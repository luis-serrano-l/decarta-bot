// Package bot connects Telegram to the publish pipeline: it receives
// updates by long polling, authorises the sender, validates the submitted
// menu, rate-limits and publishes it.
//
// Logging rule for this package: a user is only ever identified by their
// menu id (the HMAC of their Telegram user ID). Raw user IDs, chat IDs,
// names and message contents are never logged.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
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

// Messenger is the part of Telegram the handler needs. *Client implements
// it; tests use a fake.
type Messenger interface {
	// Send delivers a plain-text message to a chat.
	Send(chatID int64, text string) error
	// Download fetches a file sent to the bot, returning ErrTooLarge
	// without reading more than maxBytes.
	Download(ctx context.Context, fileID string, maxBytes int64) ([]byte, error)
}

// Options configures a Handler.
type Options struct {
	SiteBaseURL    string  // public site URL without trailing slash
	AllowedUserIDs []int64 // may publish
	AdminUserIDs   []int64 // may publish, use admin commands, receive access requests
	MaxUploadBytes int64   // cap on a submitted menu, text or file
}

// Handler processes Telegram updates.
type Handler struct {
	opts         Options
	ids          *menuid.Generator
	pub          publisher.Publisher
	store        store.Store
	publishLimit *ratelimit.Limiter
	requestLimit *ratelimit.Limiter
	tg           Messenger
	log          *slog.Logger
}

// NewHandler wires a Handler. publishLimit bounds publishes per user;
// requestLimit bounds /request_access notifications per user.
func NewHandler(opts Options, ids *menuid.Generator, pub publisher.Publisher, st store.Store,
	publishLimit, requestLimit *ratelimit.Limiter, tg Messenger, log *slog.Logger) *Handler {
	return &Handler{
		opts: opts, ids: ids, pub: pub, store: st,
		publishLimit: publishLimit, requestLimit: requestLimit,
		tg: tg, log: log,
	}
}

// Handle processes one update. It never panics on malformed updates and
// ignores everything that is not a private message from a human.
func (h *Handler) Handle(ctx context.Context, update tgbotapi.Update) {
	msg := update.Message
	if msg == nil {
		msg = update.EditedMessage // editing a submission re-submits it
	}
	if msg == nil || msg.From == nil || msg.From.IsBot || msg.Chat == nil || !msg.Chat.IsPrivate() {
		return
	}
	u := user{
		rawID:  msg.From.ID,
		chatID: msg.Chat.ID,
		id:     h.ids.ID(msg.From.ID),
	}
	u.admin = slices.Contains(h.opts.AdminUserIDs, u.rawID)
	u.publisher = u.admin || slices.Contains(h.opts.AllowedUserIDs, u.rawID)

	switch {
	case msg.IsCommand():
		h.command(u, msg)
	case msg.Document != nil || msg.Text != "":
		h.submit(ctx, u, msg)
	default:
		h.reply(u, "📄 Send your menu as a text message or a .json file.\n\nUse /template for the format and /help for all commands.")
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

func (h *Handler) command(u user, msg *tgbotapi.Message) {
	switch cmd := msg.Command(); cmd {
	case "start", "help":
		h.reply(u, helpText(u))
	case "status":
		if !h.requireAdmin(u, cmd) {
			return
		}
		h.reply(u, "✅ Bot is running.")
	case "myid":
		if !h.requireAdmin(u, cmd) {
			return
		}
		h.reply(u, fmt.Sprintf("🆔 Your Telegram user ID is %d", u.rawID))
	case "template":
		if !h.requirePublisher(u, cmd) {
			return
		}
		h.reply(u, "📋 Edit the template below and send it back to publish your menu.\n\n"+
			"Items start with a dash; blank lines are ignored. Then use /myurl to get your link.")
		h.reply(u, templateText)
	case "myurl", "link", "menuurl":
		if !h.requirePublisher(u, cmd) {
			return
		}
		text := "🔗 Your menu URL:\n" + h.menuURL(u.id)
		if !h.store.Has(u.id) {
			text += "\n\nNothing has been published from this bot yet. Send a menu first (see /template)."
		}
		h.reply(u, text)
	case "request_access":
		h.requestAccess(u, msg.From)
	default:
		h.reply(u, "❓ Unknown command. Use /help to see the available commands.")
	}
}

func (h *Handler) requireAdmin(u user, cmd string) bool {
	if u.admin {
		return true
	}
	h.log.Warn("unauthorised command", "id", u.id, "command", cmd)
	h.reply(u, "❌ This command is for administrators only.")
	return false
}

func (h *Handler) requirePublisher(u user, what string) bool {
	if u.publisher {
		return true
	}
	h.log.Warn("unauthorised command", "id", u.id, "command", what)
	h.reply(u, "❌ You are not authorised to publish menus. Use /request_access to ask the administrators.")
	return false
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

// submit runs the publish flow: authorise, bound the size, parse and
// validate, rate-limit, publish, keep a local copy, confirm.
func (h *Handler) submit(ctx context.Context, u user, msg *tgbotapi.Message) {
	if !h.requirePublisher(u, "submit") {
		return
	}

	data, ok := h.payload(ctx, u, msg)
	if !ok {
		return
	}

	m, err := menu.Parse(data)
	if err != nil {
		h.log.Info("menu rejected", "id", u.id, "bytes", len(data))
		h.reply(u, "❌ Invalid menu:\n\n"+err.Error()+"\n\nUse /template to see the expected format.")
		return
	}
	content, err := m.Encode()
	if err != nil {
		h.log.Error("could not encode menu", "id", u.id, "err", err)
		h.reply(u, "❌ Something went wrong on our side. Please try again later.")
		return
	}

	// Only valid menus count against the limit, so fixing a typo is free.
	if ok, retry := h.publishLimit.Allow(u.id); !ok {
		h.log.Warn("publish rate limit reached", "id", u.id)
		h.reply(u, "⏳ You have reached the publish limit. Try again in "+humanDuration(retry)+".")
		return
	}

	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if err := h.pub.Publish(ctx, u.id, content); err != nil {
		h.log.Error("publish failed", "id", u.id, "err", err)
		h.reply(u, "❌ Your menu is valid but could not be published right now. Please try again later.")
		return
	}
	if err := h.store.Save(u.id, content); err != nil {
		// The menu is live; the local copy is a convenience.
		h.log.Warn("could not save the local copy", "id", u.id, "err", err)
	}

	h.log.Info("menu published", "id", u.id, "bytes", len(content), "categories", len(m.Categories))
	h.reply(u, "✅ Menu published!\n\n🔗 "+h.menuURL(u.id)+
		"\n\nIt can take a minute or two for the site to update. Use /myurl anytime to get this link again.")
}

// payload returns the submitted bytes, having enforced the upload cap. It
// replies to the user itself and returns false when there is nothing to
// process.
func (h *Handler) payload(ctx context.Context, u user, msg *tgbotapi.Message) ([]byte, bool) {
	limit := h.opts.MaxUploadBytes
	tooLarge := fmt.Sprintf("❌ That is too large. The limit is %s.", humanBytes(limit))

	if msg.Document == nil {
		if int64(len(msg.Text)) > limit {
			h.log.Warn("upload rejected: too large", "id", u.id)
			h.reply(u, tooLarge)
			return nil, false
		}
		return []byte(msg.Text), true
	}

	// FileSize is reported by Telegram and lets us refuse before any
	// download; Download enforces the cap again on the actual bytes.
	if int64(msg.Document.FileSize) > limit {
		h.log.Warn("upload rejected: too large", "id", u.id)
		h.reply(u, tooLarge)
		return nil, false
	}
	data, err := h.tg.Download(ctx, msg.Document.FileID, limit)
	switch {
	case errors.Is(err, ErrTooLarge):
		h.log.Warn("upload rejected: too large", "id", u.id)
		h.reply(u, tooLarge)
		return nil, false
	case err != nil:
		h.log.Error("could not download the file", "id", u.id, "err", err)
		h.reply(u, "❌ Could not download your file. Please try again.")
		return nil, false
	}
	return data, true
}

func (h *Handler) menuURL(id string) string {
	return h.opts.SiteBaseURL + "/menu/" + id
}

func (h *Handler) reply(u user, text string) {
	if err := h.tg.Send(u.chatID, text); err != nil {
		h.log.Error("could not send a reply", "id", u.id, "err", err)
	}
}

func helpText(u user) string {
	var b strings.Builder
	b.WriteString("👋 Demenu Bot publishes your restaurant menu as a web page.\n\n📋 Commands\n")
	b.WriteString("/help - Show this message\n")
	if u.publisher {
		b.WriteString("/template - Get a menu template to edit\n")
		b.WriteString("/myurl - Get your menu link to share with customers\n")
	} else {
		b.WriteString("/request_access - Ask the administrators for permission to publish\n")
	}
	if u.admin {
		b.WriteString("/status - Check that the bot is running\n")
		b.WriteString("/myid - Show your Telegram user ID\n")
	}
	if u.publisher {
		b.WriteString("\n📄 To publish, send your menu as a text message (see /template) or as a .json file.")
	} else {
		b.WriteString("\nOnly authorised users can publish menus.")
	}
	return b.String()
}

const templateText = `CATEGORY: Appetizers
DESCRIPTION: Start your meal

- ITEM: Bruschetta
  PRICE: 12.99
  DESCRIPTION: Classic tomato basil

- ITEM: Calamari
  PRICE: 14.99
  DESCRIPTION: Golden fried squid

CATEGORY: Salads
DESCRIPTION: Fresh options

- ITEM: Caesar Salad
  PRICE: 9.99
  DESCRIPTION: Crisp romaine
`

// humanDuration rounds up to whole minutes ("1 minute", "42 minutes").
func humanDuration(d time.Duration) string {
	mins := int(math.Ceil(d.Minutes()))
	if mins <= 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", mins)
}

func humanBytes(n int64) string {
	if n >= 1024 && n%1024 == 0 {
		return fmt.Sprintf("%d KB", n/1024)
	}
	return fmt.Sprintf("%d bytes", n)
}
