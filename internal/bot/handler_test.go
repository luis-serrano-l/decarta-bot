package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/luis-serrano-l/demenu-bot/internal/menu"
	"github.com/luis-serrano-l/demenu-bot/internal/menuid"
	"github.com/luis-serrano-l/demenu-bot/internal/publisher"
	"github.com/luis-serrano-l/demenu-bot/internal/ratelimit"
)

const (
	testSecret = "0123456789abcdef0123456789abcdef"
	siteURL    = "https://example.github.io/demenu"

	adminID    int64 = 1000000001
	allowedID  int64 = 1000000002
	strangerID int64 = 1000000003
	// templateUserID is a second publisher, used where a test needs a chat
	// of its own.
	templateUserID int64 = 1000000004

	// testUploadLimit is the fixture's cap on submissions per 10 minutes.
	testUploadLimit = 20

	validList = "CATEGORY: Starters\n- ITEM: Soup\n  PRICE: 4.50\n"
	validJSON = `{"categories":[{"id":"starters","name":"Starters","items":[{"name":"Soup","price":4.5}]}]}`
)

// sentMessage is one message the fake Telegram client delivered.
type sentMessage struct {
	id       int
	text     string // for a file, its caption
	rows     [][]Button
	fileName string
	file     []byte
	edits    int
}

// fakeMessenger is the fake Telegram client: it records what the handler
// sends, edits and acknowledges, and serves canned file downloads.
type fakeMessenger struct {
	mu        sync.Mutex
	nextID    int
	msgs      map[int64][]*sentMessage
	touched   map[int64]*sentMessage // last message sent or edited, per chat
	files     map[string][]byte
	sendErr   error
	editErr   error
	fileErr   error
	downloads int
	typing    map[int64]int
	answered  []string // callback ids
}

func (f *fakeMessenger) add(chatID int64, m *sentMessage) {
	if f.msgs == nil {
		f.msgs = map[int64][]*sentMessage{}
		f.touched = map[int64]*sentMessage{}
	}
	f.nextID++
	m.id = f.nextID
	f.msgs[chatID] = append(f.msgs[chatID], m)
	f.touched[chatID] = m
}

func (f *fakeMessenger) Send(chatID int64, text string, rows ...[]Button) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.add(chatID, &sentMessage{text: text, rows: rows})
	return nil
}

func (f *fakeMessenger) Edit(chatID int64, messageID int, text string, rows ...[]Button) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.editErr != nil {
		return f.editErr
	}
	for _, m := range f.msgs[chatID] {
		if m.id == messageID {
			m.text, m.rows = text, rows
			m.edits++
			f.touched[chatID] = m
			return nil
		}
	}
	return errors.New("message to edit not found")
}

func (f *fakeMessenger) SendFile(chatID int64, name string, content []byte, caption string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fileErr != nil {
		return f.fileErr
	}
	f.add(chatID, &sentMessage{text: caption, fileName: name, file: content})
	return nil
}

func (f *fakeMessenger) AnswerCallback(callbackID, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answered = append(f.answered, callbackID)
	return nil
}

func (f *fakeMessenger) Typing(chatID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.typing == nil {
		f.typing = map[int64]int{}
	}
	f.typing[chatID]++
	return nil
}

func (f *fakeMessenger) Download(_ context.Context, fileID string, maxBytes int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloads++
	data, ok := f.files[fileID]
	if !ok {
		return nil, errors.New("no such file")
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}

// lastMsg returns a copy of the message most recently sent to or edited in
// the chat.
func (f *fakeMessenger) lastMsg(chatID int64) sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m := f.touched[chatID]; m != nil {
		return *m
	}
	return sentMessage{}
}

func (f *fakeMessenger) last(chatID int64) string { return f.lastMsg(chatID).text }

func (f *fakeMessenger) count(chatID int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs[chatID])
}

// button finds the newest button with this label in the chat.
func (f *fakeMessenger) button(chatID int64, label string) (messageID int, b Button, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.msgs[chatID]
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, row := range msgs[i].rows {
			for _, b := range row {
				if b.Text == label {
					return msgs[i].id, b, true
				}
			}
		}
	}
	return 0, Button{}, false
}

func labels(m sentMessage) []string {
	var out []string
	for _, row := range m.rows {
		for _, b := range row {
			out = append(out, b.Text)
		}
	}
	return out
}

type memStore struct {
	mu      sync.Mutex
	saved   map[string][]byte
	saveErr error
}

func (s *memStore) Save(id string, content []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	if s.saved == nil {
		s.saved = map[string][]byte{}
	}
	s.saved[id] = content
	return nil
}

func (s *memStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.saved, id)
	return nil
}

func (s *memStore) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.saved[id]
	return ok
}

type fixture struct {
	t    *testing.T
	h    *Handler
	tg   *fakeMessenger
	pub  *publisher.Fake
	st   *memStore
	ids  *menuid.Generator
	logs *bytes.Buffer
	now  time.Time
	cbs  int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ids, err := menuid.New(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		t:    t,
		tg:   &fakeMessenger{files: map[string][]byte{}},
		pub:  &publisher.Fake{},
		st:   &memStore{},
		ids:  ids,
		logs: &bytes.Buffer{},
		now:  time.Unix(1_700_000_000, 0),
	}
	clock := func() time.Time { return f.now }
	f.h = NewHandler(
		Options{
			SiteBaseURL:    siteURL,
			AllowedUserIDs: []int64{allowedID},
			AdminUserIDs:   []int64{adminID},
			MaxUploadBytes: 1024,
		},
		ids, f.pub, f.st,
		ratelimit.New(2, time.Hour, clock),
		ratelimit.New(testUploadLimit, 10*time.Minute, clock),
		ratelimit.New(1, time.Hour, clock),
		f.tg,
		slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	)
	f.h.now = clock
	return f
}

// send delivers a text message (or command) from the user.
func (f *fixture) send(userID int64, text string) {
	f.t.Helper()
	f.h.Handle(context.Background(), textUpdate(userID, text))
}

// press taps the newest button with this label in the user's chat.
func (f *fixture) press(userID int64, label string) {
	f.t.Helper()
	msgID, b, ok := f.tg.button(userID, label)
	if !ok {
		f.t.Fatalf("no %q button in the chat; last message: %q", label, f.tg.last(userID))
	}
	f.pressData(userID, msgID, b.Data)
}

// pressData delivers a callback with explicit data, as a stale or forged
// button would.
func (f *fixture) pressData(userID int64, msgID int, data string) {
	f.t.Helper()
	f.cbs++
	f.h.Handle(context.Background(), callbackUpdate(userID, msgID, fmt.Sprintf("cb-%d", f.cbs), data))
}

// publish submits a valid menu and confirms it.
func (f *fixture) publish(userID int64) {
	f.t.Helper()
	f.send(userID, validList)
	f.press(userID, "Publish")
}

func textUpdate(userID int64, text string) tgbotapi.Update {
	msg := &tgbotapi.Message{
		From: &tgbotapi.User{ID: userID, UserName: "someone", FirstName: "Some", LastName: "One"},
		Chat: &tgbotapi.Chat{ID: userID, Type: "private"},
		Text: text,
	}
	if strings.HasPrefix(text, "/") {
		end := len(text)
		if i := strings.IndexByte(text, ' '); i > 0 {
			end = i
		}
		msg.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: end}}
	}
	return tgbotapi.Update{Message: msg}
}

func docUpdate(userID int64, fileID string, size int) tgbotapi.Update {
	return tgbotapi.Update{Message: &tgbotapi.Message{
		From:     &tgbotapi.User{ID: userID},
		Chat:     &tgbotapi.Chat{ID: userID, Type: "private"},
		Document: &tgbotapi.Document{FileID: fileID, FileName: "menu.json", FileSize: size},
	}}
}

func callbackUpdate(userID int64, msgID int, callbackID, data string) tgbotapi.Update {
	return tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID:   callbackID,
		From: &tgbotapi.User{ID: userID, UserName: "someone", FirstName: "Some", LastName: "One"},
		Message: &tgbotapi.Message{
			MessageID: msgID,
			Chat:      &tgbotapi.Chat{ID: userID, Type: "private"},
		},
		Data: data,
	}}
}

func TestSubmitAndConfirm(t *testing.T) {
	tests := []struct {
		name          string
		update        func(f *fixture) tgbotapi.Update
		setup         func(f *fixture)
		wantPreview   bool   // the submission is valid and gets Publish/Cancel buttons
		wantReply     string // substring of the last message after pressing Publish, if offered
		wantPublished bool
		wantPubCalls  int
		wantSaved     bool
		wantDownloads int
	}{
		{
			name:        "admin publishes the list format",
			update:      func(*fixture) tgbotapi.Update { return textUpdate(adminID, validList) },
			wantPreview: true, wantReply: "Menu published",
			wantPublished: true, wantPubCalls: 1, wantSaved: true,
		},
		{
			name:        "allowed user publishes JSON pasted as text",
			update:      func(*fixture) tgbotapi.Update { return textUpdate(allowedID, validJSON) },
			wantPreview: true, wantReply: "Menu published",
			wantPublished: true, wantPubCalls: 1, wantSaved: true,
		},
		{
			name: "allowed user publishes a JSON file",
			update: func(f *fixture) tgbotapi.Update {
				f.tg.files["file-1"] = []byte(validJSON)
				return docUpdate(allowedID, "file-1", len(validJSON))
			},
			wantPreview: true, wantReply: "Menu published",
			wantPublished: true, wantPubCalls: 1, wantSaved: true, wantDownloads: 1,
		},
		{
			name: "edited message re-submits",
			update: func(*fixture) tgbotapi.Update {
				u := textUpdate(allowedID, validList)
				return tgbotapi.Update{EditedMessage: u.Message}
			},
			wantPreview: true, wantReply: "Menu published",
			wantPublished: true, wantPubCalls: 1, wantSaved: true,
		},
		{
			name:      "stranger is refused before anything else happens",
			update:    func(*fixture) tgbotapi.Update { return textUpdate(strangerID, validList) },
			wantReply: "not authorised",
		},
		{
			name: "stranger's file is never downloaded",
			update: func(f *fixture) tgbotapi.Update {
				f.tg.files["file-1"] = []byte(validJSON)
				return docUpdate(strangerID, "file-1", len(validJSON))
			},
			wantReply: "not authorised",
		},
		{
			name: "oversized text is rejected",
			update: func(*fixture) tgbotapi.Update {
				return textUpdate(allowedID, "CATEGORY: A\n"+strings.Repeat("x", 1100))
			},
			wantReply: "too large. The limit is 1 KB",
		},
		{
			name: "file with oversized declared size is rejected without download",
			update: func(f *fixture) tgbotapi.Update {
				f.tg.files["big"] = []byte(validJSON)
				return docUpdate(allowedID, "big", 5000)
			},
			wantReply: "too large",
		},
		{
			name: "file larger than its declared size is rejected after capped download",
			update: func(f *fixture) tgbotapi.Update {
				f.tg.files["liar"] = bytes.Repeat([]byte("x"), 5000)
				return docUpdate(allowedID, "liar", 10)
			},
			wantReply: "too large", wantDownloads: 1,
		},
		{
			name:      "download failure",
			update:    func(*fixture) tgbotapi.Update { return docUpdate(allowedID, "missing", 10) },
			wantReply: "Could not download", wantDownloads: 1,
		},
		{
			name:      "chat is not mistaken for a menu",
			update:    func(*fixture) tgbotapi.Update { return textUpdate(allowedID, "hello bot") },
			wantReply: "does not look like a menu",
		},
		{
			name:      "invalid list names the line",
			update:    func(*fixture) tgbotapi.Update { return textUpdate(allowedID, "CATEGORY: A\nsoup 4.50") },
			wantReply: "1. line 2\n   expected CATEGORY, ITEM, PRICE or DESCRIPTION\n   Fix: ",
		},
		{
			name: "unknown JSON field names the path",
			update: func(*fixture) tgbotapi.Update {
				return textUpdate(allowedID, `{"categories":[{"id":"a","name":"A","items":[],"admin":true}]}`)
			},
			wantReply: "1. categories[0].admin\n   unknown field \"admin\"\n   Fix: remove it",
		},
		{
			name: "unsafe image URL",
			update: func(*fixture) tgbotapi.Update {
				return textUpdate(allowedID, `{"categories":[{"id":"a","name":"A","items":[{"name":"x","price":1,"image":"javascript:alert(1)"}]}]}`)
			},
			wantReply: "1. categories[0].items[0].image\n   image must be",
		},
		{
			name:        "publisher failure is reported without internals",
			setup:       func(f *fixture) { f.pub.Err = errors.New("github: PUT menu file: HTTP 500: secret internals") },
			update:      func(*fixture) tgbotapi.Update { return textUpdate(allowedID, validList) },
			wantPreview: true, wantReply: "could not be published right now",
			wantPubCalls: 1,
		},
		{
			name:        "local copy failure does not fail the publish",
			setup:       func(f *fixture) { f.st.saveErr = errors.New("disk full") },
			update:      func(*fixture) tgbotapi.Update { return textUpdate(allowedID, validList) },
			wantPreview: true, wantReply: "Menu published",
			wantPublished: true, wantPubCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			update := tt.update(f)
			f.h.Handle(context.Background(), update)

			sender := update.Message
			if sender == nil {
				sender = update.EditedMessage
			}
			userID := sender.From.ID
			id := f.ids.ID(userID)

			// Nothing may be published by the submission itself.
			if f.pub.Calls() != 0 {
				t.Fatalf("published before confirmation (%d calls)", f.pub.Calls())
			}
			_, _, offered := f.tg.button(userID, "Publish")
			if offered != tt.wantPreview {
				t.Fatalf("Publish button offered = %v, want %v; reply: %q", offered, tt.wantPreview, f.tg.last(userID))
			}
			if offered {
				if got := labels(f.tg.lastMsg(userID)); !slices.Equal(got, []string{"Publish", "Cancel"}) {
					t.Errorf("preview buttons = %q, want Publish and Cancel", got)
				}
				f.press(userID, "Publish")
			}

			if got := f.tg.last(userID); !strings.Contains(got, tt.wantReply) {
				t.Errorf("reply = %q, want it to contain %q", got, tt.wantReply)
			}
			if strings.Contains(f.tg.last(userID), "secret internals") {
				t.Error("reply leaks the publisher's error")
			}
			content, published := f.pub.Get(id)
			if published != tt.wantPublished {
				t.Errorf("published = %v, want %v", published, tt.wantPublished)
			}
			if published {
				if !strings.Contains(string(content), `"name": "Soup"`) {
					t.Errorf("published content is not the canonical menu: %s", content)
				}
				if want := siteURL + "/menu/" + id; !strings.Contains(f.tg.last(userID), want) {
					t.Errorf("reply %q does not contain the menu URL %q", f.tg.last(userID), want)
				}
			}
			if f.pub.Calls() != tt.wantPubCalls {
				t.Errorf("publisher calls = %d, want %d", f.pub.Calls(), tt.wantPubCalls)
			}
			if f.st.Has(id) != tt.wantSaved {
				t.Errorf("saved locally = %v, want %v", f.st.Has(id), tt.wantSaved)
			}
			if f.tg.downloads != tt.wantDownloads {
				t.Errorf("downloads = %d, want %d", f.tg.downloads, tt.wantDownloads)
			}
			assertNoRawIDs(t, f.logs.String())
		})
	}
}

func TestPreviewSummarisesTheMenu(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, "CATEGORY: Starters\n- ITEM: Soup\n  PRICE: 4.5\n- ITEM: Bread\n  PRICE: 2\nCATEGORY: Mains\n- ITEM: Fish\n  PRICE: 18.9\nCATEGORY: Drinks")
	got := f.tg.last(allowedID)
	for _, want := range []string{
		"• Starters (2 items)", "• Mains (1 item)", "• Drinks (0 items)",
		"Categories: 3", "Items: 3", "Prices: 2.00 to 18.90", "Publish it?",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("preview does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "replaces") {
		t.Errorf("first preview talks about replacing a menu:\n%s", got)
	}
	if f.tg.typing[allowedID] == 0 {
		t.Error("no typing indicator was shown while the menu was checked")
	}

	// A single price is not shown as a range; a second menu warns that it
	// replaces the first.
	f.press(allowedID, "Publish")
	f.send(allowedID, validList)
	got = f.tg.last(allowedID)
	if !strings.Contains(got, "Price: 4.50") || !strings.Contains(got, "replaces the menu that is online now") {
		t.Errorf("second preview:\n%s", got)
	}
}

func TestPublishedMessage(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, validList)
	previewID := f.tg.lastMsg(allowedID).id
	typingBefore := f.tg.typing[allowedID]
	f.press(allowedID, "Publish")

	m := f.tg.lastMsg(allowedID)
	url := siteURL + "/menu/" + f.ids.ID(allowedID)
	if m.id != previewID || m.edits != 1 {
		t.Errorf("the preview was not edited in place (message %d, %d edits)", m.id, m.edits)
	}
	if !strings.Contains(m.text, "about a minute") {
		t.Errorf("message does not say when the page updates: %q", m.text)
	}
	if len(m.rows) != 1 || len(m.rows[0]) != 1 || m.rows[0][0] != (Button{Text: "Open my menu", URL: url}) {
		t.Errorf("buttons = %+v, want a single \"Open my menu\" URL button to %s", m.rows, url)
	}
	if f.tg.typing[allowedID] <= typingBefore {
		t.Error("no typing indicator was shown while publishing")
	}
	if !slices.Equal(f.tg.answered, []string{"cb-1"}) {
		t.Errorf("answered callbacks = %q, want cb-1", f.tg.answered)
	}
}

func TestCancelPublishesNothing(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, validList)
	msgID, publish, _ := f.tg.button(allowedID, "Publish")
	f.press(allowedID, "Cancel")

	m := f.tg.lastMsg(allowedID)
	if !strings.Contains(m.text, "Cancelled") || len(m.rows) != 0 {
		t.Errorf("after Cancel: text = %q, buttons = %v", m.text, labels(m))
	}
	// A client that still shows the old buttons cannot publish with them.
	f.pressData(allowedID, msgID, publish.Data)
	if got := f.tg.last(allowedID); !strings.Contains(got, "expired") {
		t.Errorf("stale Publish: reply = %q", got)
	}
	if f.pub.Calls() != 0 {
		t.Errorf("publisher calls = %d, want 0", f.pub.Calls())
	}
}

func TestPublishButtonWorksOnce(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, validList)
	msgID, publish, _ := f.tg.button(allowedID, "Publish")
	f.pressData(allowedID, msgID, publish.Data)
	f.pressData(allowedID, msgID, publish.Data)
	if f.pub.Calls() != 1 {
		t.Errorf("publisher calls = %d, want 1", f.pub.Calls())
	}
	if got := f.tg.last(allowedID); !strings.Contains(got, "expired or was already used") {
		t.Errorf("second press: reply = %q", got)
	}
}

func TestNewerPreviewReplacesOlder(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, validList)
	oldMsg, oldPublish, _ := f.tg.button(allowedID, "Publish")
	f.send(allowedID, strings.ReplaceAll(validList, "Soup", "Stew"))

	f.pressData(allowedID, oldMsg, oldPublish.Data)
	if f.pub.Calls() != 0 {
		t.Fatal("the superseded preview was published")
	}
	f.press(allowedID, "Publish")
	content, _ := f.pub.Get(f.ids.ID(allowedID))
	if !strings.Contains(string(content), "Stew") {
		t.Errorf("published %s, want the newer menu", content)
	}
}

func TestPreviewExpires(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, validList)
	f.now = f.now.Add(confirmTTL + time.Second)
	f.press(allowedID, "Publish")
	if got := f.tg.last(allowedID); !strings.Contains(got, "expired") {
		t.Errorf("reply = %q", got)
	}
	if f.pub.Calls() != 0 {
		t.Error("an expired preview was published")
	}
}

func TestButtonsAreBoundToTheirUser(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, validList)
	msgID, publish, _ := f.tg.button(allowedID, "Publish")

	// Another publisher replays the callback data.
	f.pressData(adminID, msgID, publish.Data)
	if got := f.tg.last(adminID); !strings.Contains(got, "expired") {
		t.Errorf("other publisher: reply = %q", got)
	}
	// Someone without access does the same.
	f.pressData(strangerID, msgID, publish.Data)
	if got := f.tg.last(strangerID); !strings.Contains(got, "not authorised") {
		t.Errorf("stranger: reply = %q", got)
	}
	if f.pub.Calls() != 0 {
		t.Errorf("publisher calls = %d, want 0", f.pub.Calls())
	}
	// The owner's preview is untouched.
	f.press(allowedID, "Publish")
	if _, ok := f.pub.Get(f.ids.ID(allowedID)); !ok {
		t.Error("the owner could no longer publish")
	}
	assertNoRawIDs(t, f.logs.String())
}

func TestForgedCallbackData(t *testing.T) {
	for _, data := range []string{"", "publish", "publish:", "publish:nope", "delete:", "cancel:x", "bogus:1", strings.Repeat(":", 64)} {
		f := newFixture(t)
		f.publish(allowedID)
		f.send(allowedID, strings.ReplaceAll(validList, "Soup", "Stew")) // a live preview
		f.pressData(allowedID, 1, data)
		if f.pub.Calls() != 1 || f.pub.Deletes() != 0 {
			t.Errorf("data %q: publishes = %d, deletes = %d, want 1 and 0", data, f.pub.Calls(), f.pub.Deletes())
		}
	}
}

func TestPublishFailureKeepsThePreview(t *testing.T) {
	f := newFixture(t)
	f.pub.Err = errors.New("github: down")
	f.send(allowedID, validList)
	f.press(allowedID, "Publish")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Tap Publish to try again") {
		t.Fatalf("reply = %q", got)
	}
	f.pub.Err = nil
	f.press(allowedID, "Publish")
	if _, ok := f.pub.Get(f.ids.ID(allowedID)); !ok {
		t.Error("retrying with the same button did not publish")
	}
}

// slowPublisher blocks each Publish until released, to test what happens
// while one is in flight.
type slowPublisher struct {
	*publisher.Fake
	started chan struct{}
	release chan struct{}
}

func (s *slowPublisher) Publish(ctx context.Context, id string, content []byte) error {
	s.started <- struct{}{}
	<-s.release
	return s.Fake.Publish(ctx, id, content)
}

func TestTapsDuringAPublishAreIgnored(t *testing.T) {
	f := newFixture(t)
	slow := &slowPublisher{Fake: f.pub, started: make(chan struct{}), release: make(chan struct{})}
	f.h.pub = slow
	f.pub.Err = errors.New("github: down")

	f.send(allowedID, validList)
	msgID, publish, _ := f.tg.button(allowedID, "Publish")
	_, cancel, _ := f.tg.button(allowedID, "Cancel")
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.h.Handle(context.Background(), callbackUpdate(allowedID, msgID, "cb-slow", publish.Data))
	}()
	<-slow.started

	// Impatient taps while the first is still publishing change nothing:
	// in particular the buttons are not replaced by "expired".
	f.pressData(allowedID, msgID, publish.Data)
	f.pressData(allowedID, msgID, cancel.Data)
	if got := f.tg.lastMsg(allowedID); !slices.Equal(labels(got), []string{"Publish", "Cancel"}) {
		t.Fatalf("after taps during a publish: %q with buttons %q", got.text, labels(got))
	}
	// Ignored taps are still acknowledged, or their buttons would spin.
	if len(f.tg.answered) != 3 {
		t.Errorf("answered %d callbacks, want all 3 taps acknowledged", len(f.tg.answered))
	}

	slow.release <- struct{}{}
	<-done
	if got := f.tg.last(allowedID); !strings.Contains(got, "Tap Publish to try again") {
		t.Fatalf("after the failure: %q", got)
	}

	// The failed publish left its button usable.
	f.pub.Err = nil
	go func() { <-slow.started; slow.release <- struct{}{} }()
	f.press(allowedID, "Publish")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Menu published") {
		t.Errorf("retry: %q", got)
	}
	if f.pub.Calls() != 2 {
		t.Errorf("publisher calls = %d, want 2", f.pub.Calls())
	}
}

func TestPublishRateLimit(t *testing.T) {
	f := newFixture(t) // limit: 2 per hour
	id := f.ids.ID(allowedID)

	// Invalid submissions, previews and cancels do not consume the budget.
	for i := 0; i < 5; i++ {
		f.send(allowedID, "CATEGORY: A\nnot a menu")
		f.send(allowedID, validList)
		f.press(allowedID, "Cancel")
	}
	for i := 0; i < 2; i++ {
		f.publish(allowedID)
		if got := f.tg.last(allowedID); !strings.Contains(got, "Menu published") {
			t.Fatalf("publish %d: reply = %q", i+1, got)
		}
	}

	// The third is previewed but not published, and says when to retry.
	f.send(allowedID, strings.ReplaceAll(validList, "Soup", "Stew"))
	f.press(allowedID, "Publish")
	got := f.tg.last(allowedID)
	if !strings.Contains(got, "publish again in 1 hour") || !strings.HasPrefix(got, "⏳") {
		t.Errorf("third publish: reply = %q, want a friendly rate-limit message with the wait", got)
	}
	if f.pub.Calls() != 2 {
		t.Errorf("publisher calls = %d, want 2", f.pub.Calls())
	}

	// Another user has an independent budget.
	f.publish(adminID)
	if got := f.tg.last(adminID); !strings.Contains(got, "Menu published") {
		t.Errorf("other user: reply = %q", got)
	}

	// The window slides, and the same button still works after the wait
	// even though it is longer than the usual preview lifetime.
	f.now = f.now.Add(time.Hour + time.Second)
	f.press(allowedID, "Publish")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Menu published") {
		t.Errorf("after the window: reply = %q", got)
	}
	if content, _ := f.pub.Get(id); !strings.Contains(string(content), "Stew") {
		t.Errorf("published %s, want the menu that was waiting", content)
	}
	assertNoRawIDs(t, f.logs.String())
}

func TestUploadRateLimit(t *testing.T) {
	f := newFixture(t)
	f.tg.files["doc"] = []byte(validJSON)

	// Chat and commands are not uploads; valid and invalid menus both are.
	for i := 0; i < testUploadLimit; i++ {
		f.send(allowedID, "hello bot")
		f.send(allowedID, "/status")
		if i%2 == 0 {
			f.send(allowedID, validList)
		} else {
			f.send(allowedID, "CATEGORY: A\nnot a menu")
		}
	}
	if _, _, ok := f.tg.button(allowedID, "Publish"); !ok {
		t.Fatal("uploads within the limit were not previewed")
	}
	downloads, typing := f.tg.downloads, f.tg.typing[allowedID]

	// One more is refused before it is downloaded or parsed, with the wait.
	f.h.Handle(context.Background(), docUpdate(allowedID, "doc", len(validJSON)))
	got := f.tg.lastMsg(allowedID)
	if !strings.HasPrefix(got.text, "⏳") || !strings.Contains(got.text, "send another in 10 minutes") || len(got.rows) != 0 {
		t.Errorf("reply = %q with buttons %q, want a friendly limit message with the wait", got.text, labels(got))
	}
	if f.tg.downloads != downloads || f.tg.typing[allowedID] != typing {
		t.Error("a rate-limited upload was still downloaded or checked")
	}

	// The preview already shown can still be published, other users are
	// unaffected, and the window slides.
	f.press(allowedID, "Publish")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Menu published") {
		t.Errorf("publishing the earlier preview: reply = %q", got)
	}
	f.send(adminID, validList)
	if _, _, ok := f.tg.button(adminID, "Publish"); !ok {
		t.Errorf("another user was limited: %q", f.tg.last(adminID))
	}
	f.now = f.now.Add(10*time.Minute + time.Second)
	f.send(allowedID, validList)
	if got := f.tg.last(allowedID); !strings.Contains(got, "Publish it?") {
		t.Errorf("after the window: reply = %q", got)
	}
	assertNoRawIDs(t, f.logs.String())
}

func TestValidationRepliesListAtMostFiveProblems(t *testing.T) {
	f := newFixture(t)
	f.h.opts.MaxUploadBytes = 64 * 1024
	f.send(allowedID, `{"categories":[{"id":"a","name":"A","items":[`+strings.Repeat(`{"name":"x"},`, 6)+`{"name":"x"}]}]}`)
	got := f.tg.last(allowedID)
	for _, want := range []string{
		"7 problems. Here are the first 5:",
		"1. categories[0].items[0].price\n   missing price\n   Fix: add a number, for example \"price\": 9.5",
		"5. categories[0].items[4].price",
		"send the menu again to see the rest",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reply does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "6. ") || strings.Contains(got, "items[5]") {
		t.Errorf("reply lists more than five problems:\n%s", got)
	}

	f.send(allowedID, `{"categories":[{"id":"a","name":"A","items":[{"name":"x"}]}]}`)
	if got := f.tg.last(allowedID); !strings.Contains(got, "has 1 problem:") || !strings.Contains(got, "Nothing was published") {
		t.Errorf("single problem reply:\n%s", got)
	}
}

func TestLongFieldNameDoesNotFloodTheReply(t *testing.T) {
	f := newFixture(t)
	f.h.opts.MaxUploadBytes = 64 * 1024
	key := strings.Repeat("k", 9000)
	f.send(allowedID, `{"categories":[{"id":"a","name":"A","items":[],"`+key+`":1}],"extra":2}`)
	got := f.tg.last(allowedID)
	if len(got) > 1000 || !strings.Contains(got, "1. extra") || !strings.Contains(got, "2. categories[0].kkk") || !strings.Contains(got, "Fix: remove it") {
		t.Errorf("reply is %d bytes and may have lost the other problems:\n%.300s", len(got), got)
	}
}

func TestClipMessageCountsUTF16(t *testing.T) {
	for _, text := range []string{
		strings.Repeat("a", 5000),
		strings.Repeat("😀", 3000), // 2 UTF-16 units each
		strings.Repeat("ñ", 4000),
		"short",
	} {
		got := clipMessage(text)
		units := 0
		for _, r := range got {
			units += utf16.RuneLen(r)
		}
		if units > 4096 || !utf8.ValidString(got) {
			t.Errorf("clipped to %d UTF-16 units (valid UTF-8: %v), want at most 4096", units, utf8.ValidString(got))
		}
	}
	if clipMessage("short") != "short" || clipMessage(strings.Repeat("ñ", 4000)) != strings.Repeat("ñ", 4000) {
		t.Error("a message within the limit was changed")
	}
}

func TestStart(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, "/start")
	m := f.tg.lastMsg(allowedID)
	if n := strings.Count(m.text, "\n") + 1; n != 3 {
		t.Errorf("/start is %d lines, want 3:\n%s", n, m.text)
	}
	if got := labels(m); !slices.Equal(got, []string{"Get a template", "How it works"}) {
		t.Errorf("buttons = %q", got)
	}

	f.press(allowedID, "How it works")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Nothing is public until you tap Publish") {
		t.Errorf("How it works: %q", got)
	}

	f.press(allowedID, "Get a template")
	file := f.tg.lastMsg(allowedID)
	if file.fileName != "menu.json" || !bytes.Equal(file.file, templateJSON) {
		t.Errorf("template: file name = %q, %d bytes", file.fileName, len(file.file))
	}
	if len(f.tg.answered) != 2 {
		t.Errorf("answered %d callbacks, want 2", len(f.tg.answered))
	}

	// Someone without access gets the same explanation, plus what to do.
	f.send(strangerID, "/start")
	if got := f.tg.last(strangerID); !strings.Contains(got, startText) || !strings.Contains(got, "/request_access") {
		t.Errorf("stranger /start: %q", got)
	}
	f.press(strangerID, "Get a template")
	if got := f.tg.lastMsg(strangerID); got.file != nil || !strings.Contains(got.text, "not authorised") {
		t.Errorf("stranger template: %+v", got)
	}
	f.press(strangerID, "How it works")
	if got := f.tg.last(strangerID); !strings.Contains(got, "How it works") {
		t.Errorf("stranger How it works: %q", got)
	}
}

// The example sent by /template must itself be publishable, unchanged and
// within the default upload limit, and already in canonical form.
func TestTemplateIsAValidMenu(t *testing.T) {
	m, err := menu.Parse(templateJSON)
	if err != nil {
		t.Fatalf("the template is not a valid menu: %v", err)
	}
	canonical, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, templateJSON) {
		t.Errorf("template.json is not in canonical form; want:\n%s", canonical)
	}

	f := newFixture(t)
	f.h.opts.MaxUploadBytes = 256 * 1024
	f.send(allowedID, "/template")
	f.tg.files["edited"] = f.tg.lastMsg(allowedID).file
	f.h.Handle(context.Background(), docUpdate(allowedID, "edited", len(templateJSON)))
	f.press(allowedID, "Publish")
	if got, _ := f.pub.Get(f.ids.ID(allowedID)); !bytes.Equal(got, templateJSON) {
		t.Errorf("sending the template back published:\n%s", got)
	}
}

func TestTemplateFallsBackToText(t *testing.T) {
	f := newFixture(t)
	f.tg.fileErr = errors.New("telegram: send file: Bad Request")
	f.send(allowedID, "/template")
	if got := f.tg.last(allowedID); got != string(templateJSON) {
		t.Errorf("fallback reply = %q", got)
	}
}

func TestDelete(t *testing.T) {
	f := newFixture(t)
	id := f.ids.ID(allowedID)
	f.publish(allowedID)

	// The command alone deletes nothing.
	f.send(allowedID, "/delete")
	m := f.tg.lastMsg(allowedID)
	if !strings.Contains(m.text, siteURL+"/menu/"+id) || !slices.Equal(labels(m), []string{"Yes, delete it", "Cancel"}) {
		t.Fatalf("confirmation = %q with buttons %q", m.text, labels(m))
	}
	if f.pub.Deletes() != 0 || !f.st.Has(id) {
		t.Fatal("deleted before confirmation")
	}

	// Cancel keeps the menu.
	f.press(allowedID, "Cancel")
	if _, ok := f.pub.Get(id); !ok || f.pub.Deletes() != 0 {
		t.Fatal("Cancel deleted the menu")
	}

	// A failure keeps the menu and the button.
	f.send(allowedID, "/delete")
	f.pub.Err = errors.New("github: DELETE menu file: HTTP 500: secret internals")
	f.press(allowedID, "Yes, delete it")
	if got := f.tg.last(allowedID); !strings.Contains(got, "could not be unpublished") || strings.Contains(got, "secret") {
		t.Errorf("failure reply = %q", got)
	}
	f.pub.Err = nil

	typingBefore := f.tg.typing[allowedID]
	f.press(allowedID, "Yes, delete it")
	m = f.tg.lastMsg(allowedID)
	if !strings.Contains(m.text, "Menu unpublished") || !strings.Contains(m.text, "about a minute") || len(m.rows) != 0 {
		t.Errorf("after delete: %q with buttons %q", m.text, labels(m))
	}
	if _, ok := f.pub.Get(id); ok || f.st.Has(id) {
		t.Error("the menu is still published or stored")
	}
	if f.tg.typing[allowedID] <= typingBefore {
		t.Error("no typing indicator was shown while deleting")
	}

	// Deleting again finds nothing; /link no longer offers the page.
	f.send(allowedID, "/delete")
	f.press(allowedID, "Yes, delete it")
	if got := f.tg.last(allowedID); !strings.Contains(got, "no published menu") {
		t.Errorf("second delete: %q", got)
	}
	f.send(allowedID, "/link")
	if got := f.tg.lastMsg(allowedID); !strings.Contains(got.text, "Nothing is published yet") || len(got.rows) != 0 {
		t.Errorf("/link after delete: %q", got.text)
	}
	assertNoRawIDs(t, f.logs.String())
}

func TestDeleteRequiresAccess(t *testing.T) {
	f := newFixture(t)
	f.publish(allowedID)
	f.send(strangerID, "/delete")
	if got := f.tg.lastMsg(strangerID); !strings.Contains(got.text, "not authorised") || len(got.rows) != 0 {
		t.Errorf("reply = %q", got.text)
	}

	// A delete button cannot be used to confirm a publish, or the reverse.
	f.send(allowedID, "/delete")
	msgID, del, _ := f.tg.button(allowedID, "Yes, delete it")
	_, nonce, _ := strings.Cut(del.Data, ":")
	f.pressData(allowedID, msgID, actPublish+":"+nonce)
	if f.pub.Calls() != 1 || f.pub.Deletes() != 0 {
		t.Errorf("publishes = %d, deletes = %d after a mismatched action", f.pub.Calls(), f.pub.Deletes())
	}
}

func TestCommands(t *testing.T) {
	tests := []struct {
		name      string
		userID    int64
		text      string
		wantReply string
		wantNot   string
	}{
		{"help for stranger", strangerID, "/help", "/request_access", "/template"},
		{"help for allowed", allowedID, "/help", "/delete", "/request_access"},
		{"help for admin", adminID, "/help", "/template", "/request_access"},
		{"status as admin", adminID, "/status", "Access: administrator\nMenu: not published\nBot: running", ""},
		{"status as allowed", allowedID, "/status", "Access: publisher\nMenu: not published", "Bot:"},
		{"status as stranger", strangerID, "/status", "Access: none (use /request_access)", "Menu:"},
		{"myid as admin", adminID, "/myid", strconv.FormatInt(adminID, 10), ""},
		{"myid as stranger", strangerID, "/myid", strconv.FormatInt(strangerID, 10), ""},
		{"template as stranger", strangerID, "/template", "not authorised", "categories"},
		{"link as allowed, nothing published", allowedID, "/link", "Nothing is published yet", ""},
		{"myurl alias", allowedID, "/myurl", siteURL + "/menu/", ""},
		{"link as stranger", strangerID, "/link", "not authorised", siteURL},
		{"request_access when already allowed", allowedID, "/request_access", "already have access", ""},
		{"unknown command", allowedID, "/nope", "Unknown command", ""},
		{"command with bot suffix and args", adminID, "/status@demenu-bot now", "Bot: running", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.send(tt.userID, tt.text)
			got := f.tg.last(tt.userID)
			if !strings.Contains(got, tt.wantReply) {
				t.Errorf("reply = %q, want it to contain %q", got, tt.wantReply)
			}
			if tt.wantNot != "" && strings.Contains(got, tt.wantNot) {
				t.Errorf("reply = %q, must not contain %q", got, tt.wantNot)
			}
			if f.pub.Calls() != 0 {
				t.Error("a command triggered a publish")
			}
			assertNoRawIDs(t, f.logs.String())
		})
	}
}

// Every command in the registered menu must be handled, and described
// within Telegram's limits.
func TestRegisteredCommandsAreHandled(t *testing.T) {
	want := []string{"start", "help", "template", "link", "status", "delete"}
	var got []string
	for _, c := range Commands {
		got = append(got, c.Name)
		if c.Description == "" || len(c.Description) > 256 {
			t.Errorf("/%s: description %q is empty or too long", c.Name, c.Description)
		}
		f := newFixture(t)
		f.send(adminID, "/"+c.Name)
		if reply := f.tg.lastMsg(adminID); reply.id == 0 || strings.Contains(reply.text, "Unknown command") {
			t.Errorf("/%s is registered but not handled: %q", c.Name, reply.text)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("registered commands = %q, want %q", got, want)
	}
}

func TestLinkAndStatusAfterPublish(t *testing.T) {
	f := newFixture(t)
	url := siteURL + "/menu/" + f.ids.ID(allowedID)
	f.publish(allowedID)

	f.send(allowedID, "/link")
	m := f.tg.lastMsg(allowedID)
	if !strings.Contains(m.text, url) || strings.Contains(m.text, "Nothing is published") {
		t.Errorf("/link = %q", m.text)
	}
	if len(m.rows) != 1 || m.rows[0][0] != (Button{Text: "Open my menu", URL: url}) {
		t.Errorf("/link buttons = %+v", m.rows)
	}

	f.send(allowedID, validList) // a preview is now waiting
	f.send(allowedID, "/status")
	m = f.tg.lastMsg(allowedID)
	for _, want := range []string{"Menu: published", url, "Preview: waiting for you to tap Publish (15 minutes left)"} {
		if !strings.Contains(m.text, want) {
			t.Errorf("/status does not contain %q:\n%s", want, m.text)
		}
	}
}

func TestRequestAccess(t *testing.T) {
	f := newFixture(t)

	f.send(strangerID, "/request_access")
	if got := f.tg.last(strangerID); !strings.Contains(got, "request was sent") {
		t.Errorf("reply = %q", got)
	}
	note := f.tg.last(adminID)
	for _, want := range []string{strconv.FormatInt(strangerID, 10), "@someone", "Some One"} {
		if !strings.Contains(note, want) {
			t.Errorf("admin notification %q does not contain %q", note, want)
		}
	}

	// A second request within the window must not notify the admins again.
	f.send(strangerID, "/request_access")
	if got := f.tg.last(strangerID); !strings.Contains(got, "already sent") {
		t.Errorf("second reply = %q", got)
	}
	if n := f.tg.count(adminID); n != 1 {
		t.Errorf("admin received %d notifications, want 1", n)
	}
	assertNoRawIDs(t, f.logs.String())
}

func TestRequestAccessWithoutAdmins(t *testing.T) {
	f := newFixture(t)
	f.h.opts.AdminUserIDs = nil
	f.send(strangerID, "/request_access")
	if got := f.tg.last(strangerID); !strings.Contains(got, "No administrators") {
		t.Errorf("reply = %q", got)
	}
}

func TestIgnoredUpdates(t *testing.T) {
	private := func() *tgbotapi.Message { return textUpdate(adminID, validList).Message }
	callback := func() *tgbotapi.CallbackQuery { return callbackUpdate(adminID, 1, "cb", actTemplate).CallbackQuery }
	tests := []struct {
		name   string
		update func() tgbotapi.Update
	}{
		{"empty update", func() tgbotapi.Update { return tgbotapi.Update{} }},
		{"channel post has no sender", func() tgbotapi.Update {
			m := private()
			m.From = nil
			return tgbotapi.Update{Message: m}
		}},
		{"no chat", func() tgbotapi.Update {
			m := private()
			m.Chat = nil
			return tgbotapi.Update{Message: m}
		}},
		{"group chat", func() tgbotapi.Update {
			m := private()
			m.Chat = &tgbotapi.Chat{ID: -100123, Type: "group"}
			return tgbotapi.Update{Message: m}
		}},
		{"supergroup chat", func() tgbotapi.Update {
			m := private()
			m.Chat = &tgbotapi.Chat{ID: -100123, Type: "supergroup"}
			return tgbotapi.Update{Message: m}
		}},
		{"another bot", func() tgbotapi.Update {
			m := private()
			m.From.IsBot = true
			return tgbotapi.Update{Message: m}
		}},
		{"callback without a sender", func() tgbotapi.Update {
			c := callback()
			c.From = nil
			return tgbotapi.Update{CallbackQuery: c}
		}},
		{"callback from a bot", func() tgbotapi.Update {
			c := callback()
			c.From.IsBot = true
			return tgbotapi.Update{CallbackQuery: c}
		}},
		{"callback on a message too old to be included", func() tgbotapi.Update {
			c := callback()
			c.Message = nil
			return tgbotapi.Update{CallbackQuery: c}
		}},
		{"callback in a group", func() tgbotapi.Update {
			c := callback()
			c.Message.Chat = &tgbotapi.Chat{ID: -100123, Type: "group"}
			return tgbotapi.Update{CallbackQuery: c}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			update := tt.update()
			f.h.Handle(context.Background(), update)
			if len(f.tg.msgs) != 0 || f.pub.Calls() != 0 {
				t.Errorf("update was not ignored: sent=%v publishes=%d", f.tg.msgs, f.pub.Calls())
			}
			// Even an ignored button press is acknowledged, or the button
			// would show a loading animation forever.
			if update.CallbackQuery != nil && len(f.tg.answered) != 1 {
				t.Errorf("answered %d callbacks, want 1", len(f.tg.answered))
			}
		})
	}
}

func TestNonTextMessageGetsAHint(t *testing.T) {
	f := newFixture(t)
	u := textUpdate(allowedID, "")
	u.Message.Photo = []tgbotapi.PhotoSize{{FileID: "p"}}
	f.h.Handle(context.Background(), u)
	if got := f.tg.last(allowedID); !strings.Contains(got, "Send your menu") {
		t.Errorf("reply = %q", got)
	}
}

func TestEditFailureFallsBackToANewMessage(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, validList)
	previewID := f.tg.lastMsg(allowedID).id
	f.tg.editErr = errors.New("telegram: edit message: Bad Request: message can't be edited")
	f.press(allowedID, "Publish")
	m := f.tg.lastMsg(allowedID)
	if m.id == previewID || !strings.Contains(m.text, "Menu published") || len(m.rows) != 1 {
		t.Errorf("fallback message = %+v", m)
	}
}

func TestSendFailureIsLoggedNotFatal(t *testing.T) {
	f := newFixture(t)
	f.tg.sendErr = errors.New("telegram: send message: Forbidden")
	f.send(allowedID, validList)
	if f.pub.Calls() != 0 {
		t.Errorf("publisher calls = %d, want 0", f.pub.Calls())
	}
	if !strings.Contains(f.logs.String(), "could not send a reply") {
		t.Errorf("send failure was not logged: %s", f.logs.String())
	}
}

// assertNoRawIDs enforces the logging rule: users appear in logs only as
// their HMAC id.
func assertNoRawIDs(t *testing.T, logs string) {
	t.Helper()
	for _, raw := range []int64{adminID, allowedID, strangerID, templateUserID} {
		if strings.Contains(logs, fmt.Sprint(raw)) {
			t.Errorf("log output contains a raw Telegram user ID:\n%s", logs)
		}
	}
	for _, pii := range []string{"someone", "Some One"} {
		if strings.Contains(logs, pii) {
			t.Errorf("log output contains a user name:\n%s", logs)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "1 minute"},
		{time.Second, "1 minute"},
		{time.Minute, "1 minute"},
		{61 * time.Second, "2 minutes"},
		{time.Hour, "1 hour"},
		{65 * time.Minute, "1 hour 5 minutes"},
		{2 * time.Hour, "2 hours"},
	}
	for _, tt := range tests {
		if got := humanDuration(tt.in); got != tt.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{262144, "256 KB"},
		{1024, "1 KB"},
		{1000, "1000 bytes"},
		{1, "1 bytes"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.in); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
