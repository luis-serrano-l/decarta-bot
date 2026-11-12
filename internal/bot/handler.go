// Package bot connects Telegram to the publish pipeline: it receives
// updates by long polling, authorises the sender, validates the submitted
// menu, shows a preview and, once the sender confirms with a button,
// rate-limits and publishes it.
//
// Logging rule for this package: a user is only ever identified by their
// menu id (the HMAC of their Telegram user ID). Raw user IDs, chat IDs,
// names and message contents are never logged.
package bot

import (
	"context"
	"crypto/rand"
	_ "embed" // for the example menu
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/luis-serrano-l/demenu_bot/internal/menu"
	"github.com/luis-serrano-l/demenu_bot/internal/menuid"
	"github.com/luis-serrano-l/demenu_bot/internal/publisher"
	"github.com/luis-serrano-l/demenu_bot/internal/ratelimit"
	"github.com/luis-serrano-l/demenu_bot/internal/store"
)

// ErrTooLarge is returned by Messenger.Download when a file exceeds the cap.
var ErrTooLarge = errors.New("file exceeds the upload limit")

// publishTimeout bounds one publish, including the conflict retry.
const publishTimeout = 45 * time.Second

// confirmTTL is how long a preview or a delete request waits for its
// button to be pressed.
const confirmTTL = 15 * time.Minute

// typingInterval is how often the "typing" indicator is refreshed; Telegram
// clears it after about five seconds.
const typingInterval = 4 * time.Second

// typingGrace is how long a reply waits for an indicator already on its
// way to Telegram.
const typingGrace = time.Second

// maxIssuesShown is how many validation problems one reply lists.
const maxIssuesShown = 5

// maxCategoriesShown bounds the category list in a preview.
const maxCategoriesShown = 12

//go:embed template.json
var templateJSON []byte

// Button is an inline button under a message. Set Data for a button that
// calls back to the bot, or URL for one that opens a link.
type Button struct {
	Text string
	Data string
	URL  string
}

// Command is an entry of the command menu Telegram shows next to the
// message box.
type Command struct {
	Name        string
	Description string
}

// Commands is registered with setMyCommands when the bot starts. /myid and
// /request_access also work but are left out of the menu to keep it short.
var Commands = []Command{
	{"start", "What this bot does"},
	{"help", "Commands and how to publish"},
	{"template", "Get an example menu.json to edit"},
	{"link", "Get the link to your menu"},
	{"status", "See what is published"},
	{"delete", "Unpublish your menu"},
}

// Callback actions carried in inline button data, as "action" or
// "action:nonce".
const (
	actTemplate = "template"
	actHow      = "how"
	actPublish  = "publish"
	actDelete   = "delete"
	actCancel   = "cancel"
)

// Messenger is the part of Telegram the handler needs. *Client implements
// it; tests use a fake.
type Messenger interface {
	// Send delivers a plain-text message to a chat, with optional rows of
	// inline buttons.
	Send(chatID int64, text string, rows ...[]Button) error
	// Edit replaces the text and buttons of a message the bot sent.
	Edit(chatID int64, messageID int, text string, rows ...[]Button) error
	// SendFile delivers content as a document with a caption.
	SendFile(chatID int64, name string, content []byte, caption string) error
	// AnswerCallback acknowledges a button press, which stops the
	// button's loading animation. text, if not empty, is shown as a toast.
	AnswerCallback(callbackID, text string) error
	// Typing shows the "typing…" indicator in a chat for a few seconds.
	Typing(chatID int64) error
	// Download fetches a file sent to the bot, returning ErrTooLarge
	// without reading more than maxBytes.
	Download(ctx context.Context, fileID string, maxBytes int64) ([]byte, error)
}

// Options configures a Handler.
type Options struct {
	SiteBaseURL    string  // public site URL without trailing slash
	AllowedUserIDs []int64 // may publish
	AdminUserIDs   []int64 // may publish, receive access requests
	MaxUploadBytes int64   // cap on a submitted menu, text or file
}

// Handler processes Telegram updates.
type Handler struct {
	opts         Options
	ids          *menuid.Generator
	pub          publisher.Publisher
	store        store.Store
	publishLimit *ratelimit.Limiter
	uploadLimit  *ratelimit.Limiter
	requestLimit *ratelimit.Limiter
	tg           Messenger
	log          *slog.Logger
	now          func() time.Time

	mu      sync.Mutex
	pending map[string]*pending // by menu id; at most one per user
	busy    map[string]struct{} // menu ids with a publish or delete in flight
}

// pending is an action waiting for its confirmation button. It lives in
// memory only: after a restart the button answers "expired" and the user
// sends the menu again.
type pending struct {
	action  string // actPublish or actDelete
	nonce   string // ties the action to the buttons of one message
	content []byte // canonical menu JSON, for actPublish
	expires time.Time
}

// NewHandler wires a Handler. publishLimit bounds publishes per user,
// uploadLimit bounds the (cheaper, more frequent) menu submissions that
// are checked and previewed, and requestLimit bounds /request_access
// notifications.
func NewHandler(opts Options, ids *menuid.Generator, pub publisher.Publisher, st store.Store,
	publishLimit, uploadLimit, requestLimit *ratelimit.Limiter, tg Messenger, log *slog.Logger) *Handler {
	return &Handler{
		opts: opts, ids: ids, pub: pub, store: st,
		publishLimit: publishLimit, uploadLimit: uploadLimit, requestLimit: requestLimit,
		tg: tg, log: log,
		now:     time.Now,
		pending: make(map[string]*pending),
		busy:    make(map[string]struct{}),
	}
}

// Handle processes one update. It never panics on malformed updates and
// ignores everything that is not a private message or button press from a
// human.
func (h *Handler) Handle(ctx context.Context, update tgbotapi.Update) {
	if cq := update.CallbackQuery; cq != nil {
		h.callback(ctx, cq)
		return
	}
	msg := update.Message
	if msg == nil {
		msg = update.EditedMessage // editing a submission re-submits it
	}
	if msg == nil || msg.From == nil || msg.From.IsBot || msg.Chat == nil || !msg.Chat.IsPrivate() {
		return
	}
	u := h.user(msg.From.ID, msg.Chat.ID)

	switch {
	case msg.IsCommand():
		h.command(u, msg)
	case msg.Document != nil || msg.Text != "":
		h.submit(ctx, u, msg)
	default:
		h.reply(u, "📄 Send your menu as a .json file or as a text message.\n\nUse /template for an example and /help for all commands.")
	}
}

// user is the sender of the update being handled.
type user struct {
	rawID     int64  // Telegram user ID; never logged
	chatID    int64  // equals rawID in private chats; never logged
	id        string // public menu id; the only identifier that is logged
	admin     bool
	publisher bool
}

func (h *Handler) user(rawID, chatID int64) user {
	u := user{rawID: rawID, chatID: chatID, id: h.ids.ID(rawID)}
	u.admin = slices.Contains(h.opts.AdminUserIDs, rawID)
	u.publisher = u.admin || slices.Contains(h.opts.AllowedUserIDs, rawID)
	return u
}

func (h *Handler) command(u user, msg *tgbotapi.Message) {
	switch cmd := msg.Command(); cmd {
	case "start":
		text := startText
		if !u.publisher {
			text += "\n\n🔒 You cannot publish yet. Use /request_access to ask the administrators."
		}
		h.reply(u, text, []Button{
			{Text: "Get a template", Data: actTemplate},
			{Text: "How it works", Data: actHow},
		})
	case "help":
		h.reply(u, helpText(u))
	case "status":
		h.status(u)
	case "myid":
		// Anyone may ask for their own ID: it is what a new operator puts
		// in ADMIN_USER_IDS, and what a user gives an administrator.
		h.reply(u, fmt.Sprintf("🆔 Your Telegram user ID is %d", u.rawID))
	case "template":
		h.template(u)
	case "link", "myurl", "menuurl":
		h.link(u, cmd)
	case "delete":
		h.askDelete(u)
	case "request_access":
		h.requestAccess(u, msg.From)
	default:
		h.reply(u, "❓ Unknown command. Use /help to see the available commands.")
	}
}

func (h *Handler) requirePublisher(u user, what string) bool {
	if u.publisher {
		return true
	}
	h.log.Warn("unauthorised command", "id", u.id, "command", what)
	h.reply(u, "❌ You are not authorised to publish menus. Use /request_access to ask the administrators.")
	return false
}

func (h *Handler) template(u user) {
	if !h.requirePublisher(u, "template") {
		return
	}
	const caption = "📎 An example menu. Edit it and send it back, as a file or pasted as text. " +
		"I will show you a preview before anything is published."
	if err := h.tg.SendFile(u.chatID, "menu.json", templateJSON, caption); err != nil {
		// Still useful as text: it can be copied, edited and pasted back.
		h.log.Warn("could not send the template as a file", "id", u.id, "err", err)
		h.reply(u, caption)
		h.reply(u, string(templateJSON))
	}
}

func (h *Handler) link(u user, cmd string) {
	if !h.requirePublisher(u, cmd) {
		return
	}
	url := h.menuURL(u.id)
	if !h.store.Has(u.id) {
		h.reply(u, "🔗 Your menu will be at:\n"+url+
			"\n\nNothing is published yet. Send a menu first; /template has an example.")
		return
	}
	h.reply(u, "🔗 Your menu:\n"+url, openButton(url))
}

func (h *Handler) status(u user) {
	var b strings.Builder
	b.WriteString("📊 Status\n\n")
	switch {
	case u.admin:
		b.WriteString("Access: administrator\n")
	case u.publisher:
		b.WriteString("Access: publisher\n")
	default:
		b.WriteString("Access: none (use /request_access)\n")
		h.reply(u, b.String())
		return
	}
	published := h.store.Has(u.id)
	if published {
		b.WriteString("Menu: published\n" + h.menuURL(u.id) + "\n")
	} else {
		b.WriteString("Menu: not published\n")
	}
	if p := h.peek(u.id); p != nil {
		left := humanDuration(p.expires.Sub(h.now()))
		switch p.action {
		case actPublish:
			b.WriteString("Preview: waiting for you to tap Publish (" + left + " left)\n")
		case actDelete:
			b.WriteString("Delete: waiting for your confirmation (" + left + " left)\n")
		}
	}
	if u.admin {
		b.WriteString("Bot: running\n")
	}
	if published {
		h.reply(u, b.String(), openButton(h.menuURL(u.id)))
		return
	}
	h.reply(u, b.String())
}

func (h *Handler) requestAccess(u user, from *tgbotapi.User) {
	if u.publisher {
		h.reply(u, "✅ You already have access. Use /template to get started.")
		return
	}
	if ok, retry := h.requestLimit.Allow(u.id); !ok {
		h.reply(u, "⏳ Your request was already sent. You can ask again in "+humanDuration(retry)+".")
		return
	}
	if len(h.opts.AdminUserIDs) == 0 {
		h.log.Warn("access request but no administrators are configured", "id", u.id)
		h.reply(u, "❌ No administrators are configured for this bot.")
		return
	}

	// The administrators need the raw user ID to grant access; it goes to
	// them over Telegram and nowhere else.
	var b strings.Builder
	fmt.Fprintf(&b, "🔔 Access requested\n\nUser ID: %d", u.rawID)
	if from.UserName != "" {
		fmt.Fprintf(&b, "\nUsername: @%s", from.UserName)
	}
	if name := strings.TrimSpace(from.FirstName + " " + from.LastName); name != "" {
		fmt.Fprintf(&b, "\nName: %s", name)
	}
	b.WriteString("\n\nTo grant access, add the user ID to ALLOWED_USER_IDS and restart the bot.")

	notified := 0
	for _, adminID := range h.opts.AdminUserIDs {
		if err := h.tg.Send(adminID, b.String()); err != nil {
			h.log.Error("could not notify an administrator", "err", err)
			continue
		}
		notified++
	}
	h.log.Info("access requested", "id", u.id, "admins_notified", notified)
	if notified == 0 {
		h.reply(u, "❌ Could not reach the administrators. Please try again later.")
		return
	}
	h.reply(u, "✅ Your request was sent to the administrators.")
}

// submit is the first half of the publish flow: authorise, bound the size,
// parse and validate, then show a preview with Publish and Cancel buttons.
// Nothing is published until the button is pressed (see confirmPublish).
func (h *Handler) submit(ctx context.Context, u user, msg *tgbotapi.Message) {
	if !h.requirePublisher(u, "submit") {
		return
	}
	if msg.Document == nil && !looksLikeMenu(msg.Text) {
		h.reply(u, "🤔 That does not look like a menu.\n\nSend a .json file, or paste the menu as text. /template gives you an example to start from.")
		return
	}
	// Checked before anything is downloaded or parsed. Looser than the
	// publish limit: fixing a typo and sending again is the normal case.
	if ok, retry := h.uploadLimit.Allow(u.id); !ok {
		h.log.Warn("upload rate limit reached", "id", u.id)
		h.reply(u, "⏳ You have sent a lot of menus in a short time, so I need you to wait a little.\n\n"+
			"You can send another in "+humanDuration(retry)+". Nothing was changed.")
		return
	}

	stop := h.working(ctx, u)
	data, problem := h.payload(ctx, u, msg)
	var (
		m       *menu.Menu
		content []byte
		err     error
	)
	if problem == "" {
		if m, err = menu.Parse(data); err == nil {
			content, err = m.Encode()
		}
	}
	stop()

	var invalid *menu.ValidationError
	switch {
	case problem != "":
		h.reply(u, problem)
		return
	case errors.As(err, &invalid):
		h.log.Info("menu rejected", "id", u.id, "bytes", len(data), "issues", invalid.Total)
		h.reply(u, issuesText(invalid))
		return
	case err != nil:
		h.log.Error("could not process menu", "id", u.id, "err", err)
		h.reply(u, "❌ Something went wrong on our side. Please try again later.")
		return
	}

	nonce := h.setPending(u.id, actPublish, content)
	h.log.Info("menu previewed", "id", u.id, "bytes", len(content), "categories", len(m.Categories))
	h.reply(u, previewText(m.Summary(), h.store.Has(u.id)), []Button{
		{Text: "Publish", Data: actPublish + ":" + nonce},
		{Text: "Cancel", Data: actCancel + ":" + nonce},
	})
}

// payload returns the submitted bytes, having enforced the upload cap, or
// a message for the user explaining why there is nothing to process.
func (h *Handler) payload(ctx context.Context, u user, msg *tgbotapi.Message) (data []byte, problem string) {
	limit := h.opts.MaxUploadBytes
	tooLarge := fmt.Sprintf("❌ That is too large. The limit is %s.", humanBytes(limit))

	if msg.Document == nil {
		if int64(len(msg.Text)) > limit {
			h.log.Warn("upload rejected: too large", "id", u.id)
			return nil, tooLarge
		}
		return []byte(msg.Text), ""
	}

	// FileSize is reported by Telegram and lets us refuse before any
	// download; Download enforces the cap again on the actual bytes.
	if int64(msg.Document.FileSize) > limit {
		h.log.Warn("upload rejected: too large", "id", u.id)
		return nil, tooLarge
	}
	data, err := h.tg.Download(ctx, msg.Document.FileID, limit)
	switch {
	case errors.Is(err, ErrTooLarge):
		h.log.Warn("upload rejected: too large", "id", u.id)
		return nil, tooLarge
	case err != nil:
		h.log.Error("could not download the file", "id", u.id, "err", err)
		return nil, "❌ Could not download your file. Please send it again."
	}
	return data, ""
}

func (h *Handler) askDelete(u user) {
	if !h.requirePublisher(u, "delete") {
		return
	}
	nonce := h.setPending(u.id, actDelete, nil)
	h.reply(u, "🗑 Unpublish your menu?\n\n"+h.menuURL(u.id)+
		"\n\nThe page will stop working for your customers. You can publish again at any time.",
		[]Button{
			{Text: "Yes, delete it", Data: actDelete + ":" + nonce},
			{Text: "Cancel", Data: actCancel + ":" + nonce},
		})
}

// callback handles a press on an inline button.
func (h *Handler) callback(ctx context.Context, cq *tgbotapi.CallbackQuery) {
	// Acknowledge first, whatever happens next: until then the button
	// shows a loading animation.
	if err := h.tg.AnswerCallback(cq.ID, ""); err != nil {
		h.log.Debug("could not answer a callback", "err", err)
	}
	// Message is absent when the button's message is too old for Telegram
	// to include; there is then no chat to reply in.
	if cq.From == nil || cq.From.IsBot || cq.Message == nil || cq.Message.Chat == nil || !cq.Message.Chat.IsPrivate() {
		return
	}
	u := h.user(cq.From.ID, cq.Message.Chat.ID)
	msgID := cq.Message.MessageID

	action, nonce, _ := strings.Cut(cq.Data, ":")
	switch action {
	case actTemplate:
		h.template(u)
	case actHow:
		h.reply(u, howText)
	case actPublish, actDelete, actCancel:
		// One confirmed action per user at a time. Updates are handled
		// concurrently, so without this a second tap during a slow publish
		// would find the draft taken and wipe the buttons needed to retry,
		// and a delete could interleave with a publish.
		if !h.begin(u.id) {
			return
		}
		defer h.end(u.id)
		h.confirm(ctx, u, msgID, action, nonce)
	default:
		h.log.Warn("unknown callback action", "id", u.id)
	}
}

func (h *Handler) confirm(ctx context.Context, u user, msgID int, action, nonce string) {
	switch action {
	case actPublish:
		h.confirmPublish(ctx, u, msgID, nonce)
	case actDelete:
		h.confirmDelete(ctx, u, msgID, nonce)
	case actCancel:
		if h.take(u.id, "", nonce) == nil {
			h.edit(u, msgID, expiredText)
			return
		}
		h.log.Info("action cancelled", "id", u.id)
		h.edit(u, msgID, "✖️ Cancelled. Nothing was changed.")
	}
}

// begin marks id as having a confirmed action in flight; it reports false
// if one already is.
func (h *Handler) begin(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, running := h.busy[id]; running {
		return false
	}
	h.busy[id] = struct{}{}
	return true
}

func (h *Handler) end(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.busy, id)
}

const expiredText = "⌛ This button has expired or was already used. Send the menu or the command again."

// confirmPublish is the second half of the publish flow: rate-limit,
// publish, keep a local copy, and turn the preview into a confirmation.
func (h *Handler) confirmPublish(ctx context.Context, u user, msgID int, nonce string) {
	if !h.requirePublisher(u, "publish") {
		return
	}
	// Taking the draft out makes a double tap publish once.
	p := h.take(u.id, actPublish, nonce)
	if p == nil {
		h.edit(u, msgID, expiredText)
		return
	}

	// Only confirmed, valid menus count against the limit, so previews and
	// fixing typos are free.
	if ok, retry := h.publishLimit.Allow(u.id); !ok {
		// Keep the draft until the wait is over, so the same button works.
		p.expires = h.now().Add(retry + confirmTTL)
		h.restore(u.id, p)
		h.log.Warn("publish rate limit reached", "id", u.id)
		h.reply(u, "⏳ You have published a lot in a short time, so I need you to wait a little.\n\n"+
			"You can publish again in "+humanDuration(retry)+". Your preview is kept until then: just tap Publish again.")
		return
	}

	stop := h.working(ctx, u)
	pctx, cancel := context.WithTimeout(ctx, publishTimeout)
	err := h.pub.Publish(pctx, u.id, p.content)
	cancel()
	stop()
	if err != nil {
		h.log.Error("publish failed", "id", u.id, "err", err)
		if !h.restore(u.id, p) {
			h.reply(u, "❌ That menu could not be published right now. Your newer preview is still waiting.")
			return
		}
		h.reply(u, "❌ Your menu is valid but could not be published right now. Tap Publish to try again in a moment.")
		return
	}
	if err := h.store.Save(u.id, p.content); err != nil {
		// The menu is live; the local copy is a convenience.
		h.log.Warn("could not save the local copy", "id", u.id, "err", err)
	}

	h.log.Info("menu published", "id", u.id, "bytes", len(p.content))
	url := h.menuURL(u.id)
	h.edit(u, msgID, "✅ Menu published!\n\n🔗 "+url+
		"\n\nThe page updates in about a minute. Use /link anytime to get this link again.",
		openButton(url))
}

func (h *Handler) confirmDelete(ctx context.Context, u user, msgID int, nonce string) {
	if !h.requirePublisher(u, "delete") {
		return
	}
	p := h.take(u.id, actDelete, nonce)
	if p == nil {
		h.edit(u, msgID, expiredText)
		return
	}

	stop := h.working(ctx, u)
	pctx, cancel := context.WithTimeout(ctx, publishTimeout)
	err := h.pub.Delete(pctx, u.id)
	cancel()
	stop()
	if err != nil && !errors.Is(err, publisher.ErrNotFound) {
		h.restore(u.id, p)
		h.log.Error("delete failed", "id", u.id, "err", err)
		h.reply(u, "❌ Your menu could not be unpublished right now. Tap the button to try again in a moment.")
		return
	}
	if serr := h.store.Delete(u.id); serr != nil {
		h.log.Warn("could not remove the local copy", "id", u.id, "err", serr)
	}
	if err != nil { // ErrNotFound
		h.edit(u, msgID, "ℹ️ There was no published menu to delete.")
		return
	}
	h.log.Info("menu deleted", "id", u.id)
	h.edit(u, msgID, "🗑 Menu unpublished.\n\nThe page disappears in about a minute. Send a menu at any time to publish again.")
}

// setPending records the action awaiting confirmation for id, replacing
// any earlier one (whose buttons then answer "expired"), and returns the
// nonce to put in the buttons.
func (h *Handler) setPending(id, action string, content []byte) string {
	var raw [9]byte
	_, _ = rand.Read(raw[:]) // crypto/rand.Read never fails
	nonce := base64.RawURLEncoding.EncodeToString(raw[:])

	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, p := range h.pending {
		if !p.expires.After(now) {
			delete(h.pending, k)
		}
	}
	h.pending[id] = &pending{action: action, nonce: nonce, content: content, expires: now.Add(confirmTTL)}
	return nonce
}

// take removes and returns the pending action for id if it matches nonce
// (and action, unless empty) and has not expired.
func (h *Handler) take(id, action, nonce string) *pending {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.pending[id]
	if p == nil || nonce == "" || p.nonce != nonce || (action != "" && p.action != action) {
		return nil
	}
	delete(h.pending, id)
	if !p.expires.After(h.now()) {
		return nil
	}
	return p
}

// restore puts back an action taken by take, so that its buttons work
// again. It reports false, and does nothing, if the user has started a
// newer one in the meantime.
func (h *Handler) restore(id string, p *pending) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, newer := h.pending[id]; newer {
		return false
	}
	h.pending[id] = p
	return true
}

// peek returns a copy of the live pending action for id, or nil.
func (h *Handler) peek(id string) *pending {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.pending[id]
	if p == nil || !p.expires.After(h.now()) {
		return nil
	}
	return &pending{action: p.action, expires: p.expires}
}

// working shows "typing…" in the user's chat until stop is called. stop
// waits briefly for the indicator goroutine, so that an indicator does not
// land after the reply, but never long: a slow Telegram must not hold up
// the reply or the worker.
func (h *Handler) working(ctx context.Context, u user) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(typingInterval)
		defer t.Stop()
		for {
			if err := h.tg.Typing(u.chatID); err != nil {
				h.log.Debug("could not send the typing indicator", "id", u.id, "err", err)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(typingGrace):
		}
	}
}

func (h *Handler) menuURL(id string) string {
	return h.opts.SiteBaseURL + "/menu/" + id
}

func openButton(url string) []Button {
	return []Button{{Text: "Open my menu", URL: url}}
}

func (h *Handler) reply(u user, text string, rows ...[]Button) {
	if err := h.tg.Send(u.chatID, text, rows...); err != nil {
		h.log.Error("could not send a reply", "id", u.id, "err", err)
	}
}

// edit rewrites one of the bot's own messages, dropping buttons that no
// longer apply. If Telegram refuses the edit the text is sent as a new
// message, so the user is never left without an answer.
func (h *Handler) edit(u user, msgID int, text string, rows ...[]Button) {
	if err := h.tg.Edit(u.chatID, msgID, text, rows...); err != nil {
		h.log.Warn("could not edit a message; sending a new one", "id", u.id, "err", err)
		h.reply(u, text, rows...)
	}
}

// looksLikeMenu tells a menu attempt from chat ("hello", "thanks"), which
// deserves a hint rather than a list of syntax errors.
func looksLikeMenu(text string) bool {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
		return true
	}
	upper := strings.ToUpper(t)
	return strings.Contains(upper, "CATEGORY:") || strings.Contains(upper, "ITEM:")
}

// issuesText lists up to maxIssuesShown problems, each with the path of
// the field and how to fix it.
func issuesText(e *menu.ValidationError) string {
	var b strings.Builder
	shown := min(len(e.Issues), maxIssuesShown)
	switch {
	case e.Total == 1:
		b.WriteString("❌ Your menu has 1 problem:")
	case e.Total > shown:
		fmt.Fprintf(&b, "❌ Your menu has %d problems. Here are the first %d:", e.Total, shown)
	default:
		fmt.Fprintf(&b, "❌ Your menu has %d problems:", e.Total)
	}
	for i, is := range e.Issues[:shown] {
		fmt.Fprintf(&b, "\n\n%d. %s\n   %s\n   Fix: %s", i+1, is.Path, is.Problem, is.Fix)
	}
	if e.Total > shown {
		b.WriteString("\n\nFix these and send the menu again to see the rest.")
	} else {
		b.WriteString("\n\nFix it and send the menu again. Nothing was published.")
	}
	return b.String()
}

// previewText summarises a valid menu: categories, item count and price
// range.
func previewText(s menu.Summary, replaces bool) string {
	var b strings.Builder
	b.WriteString("📋 Your menu is valid. Here is what I read:\n")
	for i, c := range s.Categories {
		if i == maxCategoriesShown {
			fmt.Fprintf(&b, "\n… and %d more", len(s.Categories)-i)
			break
		}
		fmt.Fprintf(&b, "\n• %s (%s)", c.Name, plural(c.Items, "item"))
	}
	fmt.Fprintf(&b, "\n\nCategories: %d\nItems: %d\n", len(s.Categories), s.Items)
	switch {
	case s.Items == 0:
	case s.MinPrice == s.MaxPrice:
		b.WriteString("Price: " + money(s.MinPrice) + "\n")
	default:
		b.WriteString("Prices: " + money(s.MinPrice) + " to " + money(s.MaxPrice) + "\n")
	}
	if replaces {
		b.WriteString("\nPublish it? It replaces the menu that is online now.")
	} else {
		b.WriteString("\nPublish it?")
	}
	return b.String()
}

func money(p float64) string { return strconv.FormatFloat(p, 'f', 2, 64) }

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

const startText = "👋 I turn your restaurant menu into a web page.\n" +
	"📄 Send me the menu as a file or as text and I show you a preview.\n" +
	"✅ Tap Publish and you get a link to share with your customers."

const howText = `💡 How it works

1. Get the example with /template and edit it, or write your own.
2. Send it here, as a .json file or pasted as text.
3. I check it. If something is wrong I name the field and how to fix it.
4. You get a summary. Nothing is public until you tap Publish.
5. The page is online in about a minute. /link gives you the address.

To change the menu, send a new one. To take it down, use /delete.

No JSON editor at hand? Plain text works too:

CATEGORY: Starters
- ITEM: Soup of the day
  PRICE: 5.00
  DESCRIPTION: Ask your waiter`

func helpText(u user) string {
	var b strings.Builder
	b.WriteString("📋 Commands\n\n")
	b.WriteString("/start - What this bot does\n")
	b.WriteString("/help - Show this message\n")
	if u.publisher {
		b.WriteString("/template - Get an example menu.json to edit\n")
		b.WriteString("/link - Get the link to your menu\n")
		b.WriteString("/status - See what is published\n")
		b.WriteString("/delete - Unpublish your menu\n")
		b.WriteString("/myid - Show your Telegram user ID\n")
		b.WriteString("\n📄 To publish, send your menu as a .json file or pasted as text. " +
			"You get a preview first; nothing goes online until you tap Publish.")
	} else {
		b.WriteString("/status - See your access\n")
		b.WriteString("/myid - Show your Telegram user ID\n")
		b.WriteString("/request_access - Ask the administrators for permission to publish\n")
		b.WriteString("\nOnly authorised users can publish menus.")
	}
	return b.String()
}

// humanDuration rounds up to whole minutes ("1 minute", "42 minutes",
// "1 hour 5 minutes").
func humanDuration(d time.Duration) string {
	mins := max(1, int(math.Ceil(d.Minutes())))
	if mins < 60 {
		return plural(mins, "minute")
	}
	if mins%60 == 0 {
		return plural(mins/60, "hour")
	}
	return plural(mins/60, "hour") + " " + plural(mins%60, "minute")
}

func humanBytes(n int64) string {
	if n >= 1024 && n%1024 == 0 {
		return fmt.Sprintf("%d KB", n/1024)
	}
	return fmt.Sprintf("%d bytes", n)
}
