package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/luis-serrano-l/decarta-bot/internal/menu"
	"github.com/luis-serrano-l/decarta-bot/internal/store"
)

// The builder lets an owner put a menu together a piece at a time, with
// buttons, instead of typing the whole document. The menu in progress is a
// draft: one file per user on the data volume, so that a restart does not
// throw away what was typed on a phone. Publishing still goes through the
// preview and its Publish button.

// DraftTTL is how long an untouched draft is kept.
const DraftTTL = 7 * 24 * time.Hour

// maxDishesShown bounds the dish list on the screen where dishes are added.
const maxDishesShown = 40

// Builder callbacks. actBuildStart opens the builder; every button inside
// it carries "b:<rev>:<verb>[:<n>...]", where rev is the draft's revision
// when the message was drawn.
const (
	actBuildStart = "build"
	actBuild      = "b"
)

// What the builder is waiting for the user to type.
const (
	awaitNothing = ""
	awaitSection = "section" // the name of a new section
	awaitDishes  = "dishes"  // dishes for the current section
	awaitDesc    = "desc"    // a description for the Target dish, just added

	// Answers to the Edit / remove screens, about the Target dish or section.
	awaitPrice    = "price"
	awaitName     = "name"
	awaitEditDesc = "editdesc"
	awaitSecName  = "secname"
)

// dishesPerPage is how many dishes the Edit / remove list shows at once.
const dishesPerPage = 8

// maxLabelLen bounds a dish or section name used as a button label.
const maxLabelLen = 28

// sectionChips are one-tap names offered for a new section.
var sectionChips = []string{"Starters", "Mains", "Desserts", "Drinks"}

// draft is a menu being built. It is stored as JSON.
type draft struct {
	Menu menu.Menu `json:"menu"`
	// Current is the index of the section dishes are added to.
	Current int `json:"current"`
	// Await is what the next text message from the user means.
	Await string `json:"await,omitempty"`
	// Target is the section and dish that Await refers to.
	Target [2]int `json:"target"`
	// Rev changes on every save; buttons drawn for an older revision do
	// nothing but redraw, so an index in a button is never out of date.
	Rev     int       `json:"rev"`
	Updated time.Time `json:"updated"`
}

// btn is a builder button for the draft's current revision.
func (d *draft) btn(text, verb string, args ...int) Button {
	data := actBuild + ":" + strconv.Itoa(d.Rev) + ":" + verb
	for _, a := range args {
		data += ":" + strconv.Itoa(a)
	}
	return Button{Text: text, Data: data}
}

func (d *draft) current() *menu.Category {
	if d.Current < 0 || d.Current >= len(d.Menu.Categories) {
		return nil
	}
	return &d.Menu.Categories[d.Current]
}

// target returns the dish Target points at, or nil.
func (d *draft) target() *menu.Item {
	c, i := d.Target[0], d.Target[1]
	if c < 0 || c >= len(d.Menu.Categories) || i < 0 || i >= len(d.Menu.Categories[c].Items) {
		return nil
	}
	return &d.Menu.Categories[c].Items[i]
}

// lockDraft serialises work on one user's draft: updates are handled
// concurrently, and two messages sent in a row must not overwrite each
// other's changes.
func (h *Handler) lockDraft(id string) (unlock func()) {
	h.mu.Lock()
	l := h.draftLocks[id]
	if l == nil {
		l = new(sync.Mutex)
		h.draftLocks[id] = l
	}
	h.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// loadDraft returns the user's draft, or nil if there is none or it is too
// old or unreadable.
func (h *Handler) loadDraft(u user) *draft {
	raw, err := h.drafts.Load(u.id)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			h.log.Error("could not read the draft", "id", u.id, "err", err)
		}
		return nil
	}
	var d draft
	if err := json.Unmarshal(raw, &d); err != nil || h.now().Sub(d.Updated) > DraftTTL {
		if err != nil {
			h.log.Warn("dropping an unreadable draft", "id", u.id, "err", err)
		}
		h.dropDraft(u)
		return nil
	}
	if d.current() == nil {
		d.Current = max(0, len(d.Menu.Categories)-1)
	}
	return &d
}

// saveDraft stores d under a new revision. On failure the user is told and
// false is returned.
func (h *Handler) saveDraft(u user, d *draft) bool {
	d.Rev++
	d.Updated = h.now()
	raw, err := json.Marshal(d)
	if err == nil {
		err = h.drafts.Save(u.id, raw)
	}
	if err != nil {
		h.log.Error("could not save the draft", "id", u.id, "err", err)
		h.reply(u, "❌ I could not save that change. Please try again in a moment.")
		return false
	}
	return true
}

func (h *Handler) dropDraft(u user) {
	if err := h.drafts.Delete(u.id); err != nil {
		h.log.Warn("could not remove the draft", "id", u.id, "err", err)
	}
}

// build opens the builder: on the user's draft if there is one, else on a
// copy of their published menu, else on an empty menu.
func (h *Handler) build(u user) {
	if !h.requirePublisher(u, "build") {
		return
	}
	defer h.lockDraft(u.id)()

	d, note := h.loadDraft(u), ""
	switch {
	case d != nil:
		note = "Here is the draft you were working on."
	default:
		d = &draft{}
		if m := h.publishedMenu(u); m != nil {
			d.Menu = *m
			note = "This is the menu that is online now. Change it here; nothing goes online until you publish."
		}
		h.log.Info("draft started", "id", u.id, "categories", len(d.Menu.Categories))
	}
	if len(d.Menu.Categories) == 0 {
		d.Await = awaitSection
	} else {
		d.Await = awaitNothing
	}
	if !h.saveDraft(u, d) {
		return
	}
	h.show(u, 0, d, note)
}

// publishedMenu reads the local copy of the user's published menu, or
// returns nil.
func (h *Handler) publishedMenu(u user) *menu.Menu {
	raw, err := h.store.Load(u.id)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			h.log.Warn("could not read the published copy", "id", u.id, "err", err)
		}
		return nil
	}
	m, err := menu.Parse(raw)
	if err != nil {
		h.log.Warn("the published copy is not a valid menu", "id", u.id, "err", err)
		return nil
	}
	return m
}

// show draws the screen that matches the draft's state: in the message
// msgID, or as a new message when msgID is 0.
func (h *Handler) show(u user, msgID int, d *draft, note string) {
	text, rows := h.screen(d)
	if note != "" {
		text = note + "\n\n" + text
	}
	if msgID == 0 {
		h.reply(u, text, rows...)
		return
	}
	h.edit(u, msgID, text, rows...)
}

// typeHint tells the owner how to answer: Telegram has no text fields, so
// every answer is an ordinary message.
const typeHint = "✍️ Write it in the message box below 👇 and send it"

const backToMenu = "⬅️ Back to menu"

// screen is the text and buttons for the draft's state.
func (h *Handler) screen(d *draft) (string, [][]Button) {
	back := [][]Button{{d.btn(backToMenu, "back")}}
	switch d.Await {
	case awaitSection:
		return sectionPrompt(d)
	case awaitDishes:
		if d.current() != nil {
			return addScreen(d)
		}
	case awaitDesc, awaitEditDesc:
		if it := d.target(); it != nil {
			if d.Await == awaitEditDesc && it.Description != "" {
				back = append([][]Button{{d.btn("🗑 Remove the description", "idx", d.Target[0], d.Target[1])}}, back...)
			}
			return fmt.Sprintf("📝 A short description for %s\n\n%s, for example:\nCold tomato soup with olive oil", it.Name, typeHint), back
		}
	case awaitPrice:
		if it := d.target(); it != nil {
			return fmt.Sprintf("💶 New price for %s (now %s)\n\n%s, for example:\n7.50", it.Name, priceText(*it), typeHint), back
		}
	case awaitName:
		if it := d.target(); it != nil {
			return fmt.Sprintf("✏️ New name for %s\n\n%s.", it.Name, typeHint), back
		}
	case awaitSecName:
		if c := d.Target[0]; c >= 0 && c < len(d.Menu.Categories) {
			return fmt.Sprintf("✏️ New name for the section %s\n\n%s.", d.Menu.Categories[c].Name, typeHint), back
		}
	}
	return home(d)
}

func sectionPrompt(d *draft) (string, [][]Button) {
	const how = "Tap a name, or ✍️ write your own in the message box below 👇 and send it."
	text := "📂 New section\n\nWhat is it called?\n" + how
	if len(d.Menu.Categories) == 0 {
		text = "🍽 Let's build your menu.\n\nA menu is made of sections, such as Starters or Drinks, with dishes in them.\n\n" +
			"📂 What is your first section called?\n" + how
	}
	var chips []Button
	for i, name := range sectionChips {
		taken := false
		for _, c := range d.Menu.Categories {
			taken = taken || strings.EqualFold(c.Name, name)
		}
		if !taken {
			chips = append(chips, d.btn(name, "chip", i))
		}
	}
	var rows [][]Button
	if len(chips) > 0 {
		rows = append(rows, chips)
	}
	if len(d.Menu.Categories) > 0 {
		rows = append(rows, []Button{d.btn(backToMenu, "back")})
	} else {
		rows = append(rows, []Button{d.btn("Cancel", "drop")})
	}
	return text, rows
}

// home is the builder's main message: the whole menu so far and what can
// be done with it.
func home(d *draft) (string, [][]Button) {
	return "🍽 Your menu (draft)\n\n" + menuText(&d.Menu) + "\n\nWhat next?", [][]Button{
		{d.btn("➕ Add dish", "add"), d.btn("📂 Add section", "sec")},
		{d.btn("✏️ Edit dishes", "ed"), d.btn("✏️ Edit sections", "eds")},
		{d.btn("✅ Done, preview", "done"), d.btn("Cancel", "cancel")},
	}
}

// addScreen is where dishes are typed: it shows the one section they go
// to, and nothing else can be done here but go back.
func addScreen(d *draft) (string, [][]Button) {
	c := d.current()
	var b strings.Builder
	b.WriteString("📂 " + c.Name)
	if len(c.Items) == 0 {
		b.WriteString("\n   No dishes yet.")
	}
	first := max(0, len(c.Items)-maxDishesShown)
	if first > 0 {
		fmt.Fprintf(&b, "\n   … %s above", dishCount(first))
	}
	for _, it := range c.Items[first:] {
		fmt.Fprintf(&b, "\n   • %s  %s", it.Name, priceText(it))
	}
	fmt.Fprintf(&b, "\n\n➕ Add a dish to %s\n%s, for example:\nGazpacho 6.50\n\nSeveral lines add several dishes.", c.Name, typeHint)

	var rows [][]Button
	if d.target() != nil && d.Target[0] == d.Current {
		rows = append(rows, []Button{d.btn("📝 Describe last dish", "desc")})
	}
	return b.String(), append(rows, []Button{d.btn(backToMenu, "back")})
}

// menuTextBudget is the size, in bytes, after which menuText stops
// listing dishes, to stay inside one Telegram message.
const menuTextBudget = 2400

// menuText lists every section with its dishes and prices.
func menuText(m *menu.Menu) string {
	var b strings.Builder
	for i := range m.Categories {
		c := &m.Categories[i]
		if b.Len() > menuTextBudget {
			fmt.Fprintf(&b, "\n📂 %s (%s)", c.Name, dishCount(len(c.Items)))
			continue
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("📂 " + c.Name)
		if len(c.Items) == 0 {
			b.WriteString("\n   No dishes yet.")
		}
		for j, it := range c.Items {
			if b.Len() > menuTextBudget {
				fmt.Fprintf(&b, "\n   … and %d more", len(c.Items)-j)
				break
			}
			fmt.Fprintf(&b, "\n   • %s  %s", it.Name, priceText(it))
		}
	}
	return b.String()
}

func priceText(it menu.Item) string {
	if it.Price == nil {
		return "no price"
	}
	return money(*it.Price)
}

// builderInput handles a text message as the answer to the builder's open
// question. It reports false, having done nothing, when the builder is not
// waiting for text from this user.
func (h *Handler) builderInput(u user, text string) bool {
	if !u.publisher {
		return false
	}
	defer h.lockDraft(u.id)()
	d := h.loadDraft(u)
	if d == nil || d.Await == awaitNothing {
		return false
	}
	if int64(len(text)) > h.opts.MaxUploadBytes {
		h.reply(u, fmt.Sprintf("❌ That is too large. The limit is %s.", humanBytes(h.opts.MaxUploadBytes)))
		return true
	}
	text = strings.TrimSpace(text)

	var (
		note string
		err  error
	)
	switch d.Await {
	case awaitSection:
		note, err = d.addSection(text)
	case awaitDishes:
		var items []menu.Item
		if items, err = menu.ParseDishLines(text); err == nil {
			if err = d.Menu.AddItems(d.Current, items); err == nil {
				c := d.current()
				d.Target = [2]int{d.Current, len(c.Items) - 1}
				note = fmt.Sprintf("✅ Added %s to %s.", dishCount(len(items)), c.Name)
			}
		}
	case awaitDesc:
		if err = menu.CheckDescription(text); err == nil {
			it := d.target()
			if it == nil {
				err = errors.New("the dish is gone")
				break
			}
			it.Description = text
			d.Await = awaitDishes
			note = fmt.Sprintf("✅ Description saved for %s.", it.Name)
		}
	case awaitPrice, awaitName, awaitEditDesc, awaitSecName:
		if note, err = d.applyEdit(text); err == nil {
			renamedSection := d.Await == awaitSecName
			d.Await = awaitNothing
			switch {
			case !h.saveDraft(u, d):
			case renamedSection:
				h.show(u, 0, d, note)
			default:
				// Back to the list the dish is in: changing several prices
				// in a row is the common case.
				body, rows := sectionScreen(d, d.Target[0], d.Target[1]/dishesPerPage)
				h.reply(u, note+"\n\n"+body, rows...)
			}
			return true
		}
	}
	if err != nil {
		h.reply(u, inputProblem(err, strings.Contains(text, "\n")))
		return true
	}
	if h.saveDraft(u, d) {
		h.show(u, 0, d, note)
	}
	return true
}

// errGone is an answer about a dish or section that is no longer there.
var errGone = errors.New("the dish or section is gone")

// applyEdit uses text as the answer to an Edit / remove question.
func (d *draft) applyEdit(text string) (note string, err error) {
	if d.Await == awaitSecName {
		c := d.Target[0]
		if c < 0 || c >= len(d.Menu.Categories) {
			return "", errGone
		}
		if err := menu.CheckName(text); err != nil {
			return "", err
		}
		d.Menu.Categories[c].Name = text
		return fmt.Sprintf("✅ The section is now called %s.", text), nil
	}
	it := d.target()
	if it == nil {
		return "", errGone
	}
	switch d.Await {
	case awaitPrice:
		p, err := menu.ParsePrice(text)
		if err != nil {
			return "", err
		}
		it.Price = &p
		return fmt.Sprintf("✅ %s now costs %s.", it.Name, money(p)), nil
	case awaitName:
		if err := menu.CheckName(text); err != nil {
			return "", err
		}
		it.Name = text
		return fmt.Sprintf("✅ Renamed to %s.", text), nil
	default: // awaitEditDesc
		if err := menu.CheckDescription(text); err != nil {
			return "", err
		}
		it.Description = text
		return fmt.Sprintf("✅ Description saved for %s.", it.Name), nil
	}
}

// label shortens a name for a button.
func label(s string) string {
	if r := []rune(s); len(r) > maxLabelLen {
		return string(r[:maxLabelLen-1]) + "…"
	}
	return s
}

// sectionScreen lists one page of the dishes of section c, each a button
// that opens the dish.
func sectionScreen(d *draft, c, page int) (string, [][]Button) {
	cat := &d.Menu.Categories[c]
	pages := max(1, (len(cat.Items)+dishesPerPage-1)/dishesPerPage)
	page = min(max(page, 0), pages-1)

	text := fmt.Sprintf("📂 %s (%s)\n\n", cat.Name, dishCount(len(cat.Items)))
	if len(cat.Items) == 0 {
		text += "This section has no dishes yet."
	} else {
		text += "Tap a dish to change or remove it."
	}
	if pages > 1 {
		text += fmt.Sprintf("\nPage %d of %d.", page+1, pages)
	}

	var rows [][]Button
	for i := page * dishesPerPage; i < min(len(cat.Items), (page+1)*dishesPerPage); i++ {
		rows = append(rows, []Button{d.btn(label(cat.Items[i].Name)+"  "+priceText(cat.Items[i]), "ei", c, i)})
	}
	var nav []Button
	if page > 0 {
		nav = append(nav, d.btn("◀️ Previous", "es", c, page-1))
	}
	if page < pages-1 {
		nav = append(nav, d.btn("Next ▶️", "es", c, page+1))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	return text, append(rows, []Button{d.btn(backToMenu, "back")})
}

// sectionActions shows what can be done with section c itself.
func sectionActions(d *draft, c int) (string, [][]Button) {
	cat := d.Menu.Categories[c]
	return fmt.Sprintf("📂 %s (%s)", cat.Name, dishCount(len(cat.Items))), [][]Button{
		{d.btn("✏️ Rename", "rs", c), d.btn("🗑 Remove", "xs", c)},
		{d.btn(backToMenu, "back")},
	}
}

// itemScreen shows dish i of section c and what can be done with it.
func itemScreen(d *draft, c, i int) (string, [][]Button) {
	it := d.Menu.Categories[c].Items[i]
	text := fmt.Sprintf("🍽 %s\nPrice: %s", it.Name, priceText(it))
	if it.Description != "" {
		text += "\n" + it.Description
	}
	return text, [][]Button{
		{d.btn("💶 Change price", "ip", c, i), d.btn("✏️ Rename", "in", c, i)},
		{d.btn("📝 Description", "id", c, i), d.btn("🗑 Remove", "ix", c, i)},
		{d.btn("⬅️ Back", "es", c, i/dishesPerPage)},
	}
}

// addSection appends a section called name and makes it the one dishes go
// to.
func (d *draft) addSection(name string) (note string, err error) {
	idx, err := d.Menu.AddCategory(name)
	if err != nil {
		return "", err
	}
	d.Current, d.Await = idx, awaitDishes
	d.Target = [2]int{-1, -1}
	return fmt.Sprintf("✅ Section %s added.", name), nil
}

func dishCount(n int) string {
	if n == 1 {
		return "1 dish"
	}
	return strconv.Itoa(n) + " dishes"
}

// inputProblem words a rejected answer: what is wrong, how to fix it, and
// that the question is still open. Line numbers are given only when the
// answer had several lines.
func inputProblem(err error, lines bool) string {
	var invalid *menu.ValidationError
	if !errors.As(err, &invalid) {
		return "❌ I could not use that. Send /build to see your menu and try again."
	}
	var b strings.Builder
	b.WriteString("❌ I could not use that:\n")
	shown := min(len(invalid.Issues), maxIssuesShown)
	for _, is := range invalid.Issues[:shown] {
		where := ""
		if lines && strings.HasPrefix(is.Path, "line ") {
			where = "Line " + strings.TrimPrefix(is.Path, "line ") + ": "
		}
		fmt.Fprintf(&b, "\n• %s%s. Fix: %s.", where, is.Problem, is.Fix)
	}
	if more := invalid.Total - shown; more > 0 {
		fmt.Fprintf(&b, "\n… and %d more.", more)
	}
	b.WriteString("\n\nNothing was changed. Send it again.")
	return b.String()
}

// builderPress handles a builder button. data is "<rev>:<verb>[:<n>...]".
func (h *Handler) builderPress(u user, msgID int, data string) {
	if !h.requirePublisher(u, "build") {
		return
	}
	defer h.lockDraft(u.id)()

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		h.log.Warn("malformed builder button", "id", u.id)
		return
	}
	verb := parts[1]
	args := make([]int, 0, len(parts)-2)
	for _, p := range parts[2:] {
		n, err := strconv.Atoi(p)
		if err != nil {
			h.log.Warn("malformed builder button", "id", u.id)
			return
		}
		args = append(args, n)
	}
	arg := func(i int) int {
		if i < len(args) {
			return args[i]
		}
		return -1
	}

	d := h.loadDraft(u)
	if d == nil {
		h.edit(u, msgID, "⌛ This draft no longer exists. Send /build to start again.")
		return
	}
	if parts[0] != strconv.Itoa(d.Rev) {
		// The menu changed after this message was drawn.
		h.show(u, msgID, d, "ℹ️ That button was out of date. Here is your menu now.")
		return
	}

	// Edit / remove buttons name a section and, some, a dish in it.
	c, i := arg(0), arg(1)
	hasSection := c >= 0 && c < len(d.Menu.Categories)
	hasDish := hasSection && i >= 0 && i < len(d.Menu.Categories[c].Items)

	note := ""
	switch verb {
	case "ed", "eds":
		// Text typed while browsing must not be taken for new dishes.
		d.Await = awaitNothing
		if !h.saveDraft(u, d) {
			return
		}
		switch {
		case verb == "eds":
			h.edit(u, msgID, "✏️ Edit sections\n\nWhich section?", sectionRows(d, "sx")...)
		case len(d.Menu.Categories) == 1:
			text, rows := sectionScreen(d, 0, 0)
			h.edit(u, msgID, text, rows...)
		default:
			h.edit(u, msgID, "✏️ Edit dishes\n\nWhich section is the dish in?", sectionRows(d, "es")...)
		}
		return
	case "sx":
		if !hasSection {
			return
		}
		text, rows := sectionActions(d, c)
		h.edit(u, msgID, text, rows...)
		return
	case "es":
		if !hasSection {
			return
		}
		text, rows := sectionScreen(d, c, i)
		h.edit(u, msgID, text, rows...)
		return
	case "ei":
		if !hasDish {
			return
		}
		text, rows := itemScreen(d, c, i)
		h.edit(u, msgID, text, rows...)
		return
	case "ip", "in", "id":
		if !hasDish {
			return
		}
		d.Target = [2]int{c, i}
		d.Await = map[string]string{"ip": awaitPrice, "in": awaitName, "id": awaitEditDesc}[verb]
	case "idx", "ix":
		if !hasDish {
			return
		}
		cat := &d.Menu.Categories[c]
		if verb == "idx" {
			cat.Items[i].Description = ""
			note = "✅ Description removed from " + cat.Items[i].Name + "."
		} else {
			note = "🗑 Removed " + cat.Items[i].Name + "."
			cat.Items = append(cat.Items[:i], cat.Items[i+1:]...)
		}
		d.Await, d.Target = awaitNothing, [2]int{-1, -1}
		if h.saveDraft(u, d) {
			text, rows := sectionScreen(d, c, i/dishesPerPage)
			h.edit(u, msgID, note+"\n\n"+text, rows...)
		}
		return
	case "rs":
		if !hasSection {
			return
		}
		d.Target, d.Await = [2]int{c, -1}, awaitSecName
	case "xs":
		if !hasSection {
			return
		}
		cat := d.Menu.Categories[c]
		h.edit(u, msgID, fmt.Sprintf("🗑 Remove the section %s and its %s from the draft?", cat.Name, dishCount(len(cat.Items))),
			[]Button{d.btn("Yes, remove it", "xsy", c), d.btn("⬅️ Back", "sx", c)})
		return
	case "xsy":
		if !hasSection {
			return
		}
		note = "🗑 Removed the section " + d.Menu.Categories[c].Name + "."
		d.Menu.Categories = append(d.Menu.Categories[:c], d.Menu.Categories[c+1:]...)
		d.Current = max(0, len(d.Menu.Categories)-1)
		d.Await, d.Target = awaitNothing, [2]int{-1, -1}
		if len(d.Menu.Categories) == 0 {
			d.Await = awaitSection
		}
	case "add":
		switch len(d.Menu.Categories) {
		case 0:
			d.Await = awaitSection
		case 1:
			d.Current, d.Await = 0, awaitDishes
			d.Target = [2]int{-1, -1}
		default:
			h.edit(u, msgID, "➕ Add dish\n\nWhich section is it for?", sectionRows(d, "cur")...)
			return
		}
	case "sec":
		d.Await = awaitSection
	case "chip":
		n := arg(0)
		if n < 0 || n >= len(sectionChips) {
			return
		}
		var err error
		if note, err = d.addSection(sectionChips[n]); err != nil {
			h.reply(u, inputProblem(err, false))
			return
		}
	case "back":
		d.Await = awaitNothing
		if len(d.Menu.Categories) == 0 {
			d.Await = awaitSection
		}
	case "desc":
		if d.target() == nil {
			return
		}
		d.Await = awaitDesc
	case "cur":
		n := arg(0)
		if n < 0 || n >= len(d.Menu.Categories) {
			return
		}
		d.Current, d.Await = n, awaitDishes
		d.Target = [2]int{-1, -1}
	case "done":
		h.builderDone(u, msgID, d)
		return
	case "cancel":
		h.edit(u, msgID, "Stop building?\n\nYour draft can wait for you for 7 days, or you can throw it away. "+
			"Either way, nothing changes online.",
			[]Button{d.btn("💾 Keep for later", "keep"), d.btn("🗑 Discard draft", "drop")},
			[]Button{d.btn("⬅️ Back", "back")})
		return
	case "keep":
		d.Await = awaitNothing
		if h.saveDraft(u, d) {
			h.edit(u, msgID, "💾 Draft kept. Send /build within 7 days to continue.")
		}
		return
	case "drop":
		h.dropDraft(u)
		h.log.Info("draft discarded", "id", u.id)
		h.edit(u, msgID, "🗑 Draft discarded. Nothing was changed online. Send /build to start again.")
		return
	default:
		h.log.Warn("unknown builder button", "id", u.id)
		return
	}
	if h.saveDraft(u, d) {
		h.show(u, msgID, d, note)
	}
}

// sectionRows is one button per section, each calling verb with the
// section's index, then Back.
func sectionRows(d *draft, verb string) [][]Button {
	rows := make([][]Button, 0, len(d.Menu.Categories)+1)
	for i, c := range d.Menu.Categories {
		rows = append(rows, []Button{d.btn("📂 "+label(c.Name), verb, i)})
	}
	return append(rows, []Button{d.btn(backToMenu, "back")})
}

// builderDone validates the draft and turns the builder message into the
// usual preview. The draft is kept until the menu is published.
func (h *Handler) builderDone(u user, msgID int, d *draft) {
	if len(d.Menu.Categories) == 0 {
		h.show(u, msgID, d, "ℹ️ The menu is empty. Add a section first.")
		return
	}
	var content []byte
	err := d.Menu.Validate()
	if err == nil {
		content, err = d.Menu.Encode()
	}
	var invalid *menu.ValidationError
	switch {
	case errors.As(err, &invalid):
		h.log.Info("draft rejected", "id", u.id, "issues", invalid.Total)
		h.reply(u, issuesText(invalid))
		return
	case err != nil:
		h.log.Error("could not process the draft", "id", u.id, "err", err)
		h.reply(u, "❌ Something went wrong on our side. Please try again later.")
		return
	}
	if ok, retry := h.uploadLimit.Allow(u.id); !ok {
		h.log.Warn("upload rate limit reached", "id", u.id)
		h.reply(u, "⏳ You have previewed a lot of menus in a short time, so I need you to wait a little.\n\n"+
			"You can preview again in "+humanDuration(retry)+". Your draft is safe.")
		return
	}
	nonce := h.setPending(u.id, actPublish, content)
	h.log.Info("menu previewed", "id", u.id, "bytes", len(content), "categories", len(d.Menu.Categories))
	h.edit(u, msgID, previewText(&d.Menu, h.store.Has(u.id)),
		[]Button{
			{Text: "Publish", Data: actPublish + ":" + nonce},
			{Text: "Cancel", Data: actCancel + ":" + nonce},
		},
		[]Button{d.btn("✏️ Keep editing", "back")})
}
