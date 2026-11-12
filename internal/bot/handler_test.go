package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/luis-serrano-l/demenu_bot/internal/menuid"
	"github.com/luis-serrano-l/demenu_bot/internal/publisher"
	"github.com/luis-serrano-l/demenu_bot/internal/ratelimit"
)

const (
	testSecret = "0123456789abcdef0123456789abcdef"
	siteURL    = "https://example.github.io/demenu"

	adminID    int64 = 1000000001
	allowedID  int64 = 1000000002
	strangerID int64 = 1000000003

	validList = "CATEGORY: Starters\n- ITEM: Soup\n  PRICE: 4.50\n"
	validJSON = `{"categories":[{"id":"starters","name":"Starters","items":[{"name":"Soup","price":4.5}]}]}`
)

// fakeMessenger records sent messages and serves canned file downloads.
type fakeMessenger struct {
	mu        sync.Mutex
	sent      map[int64][]string
	files     map[string][]byte
	sendErr   error
	downloads int
}

func (f *fakeMessenger) Send(chatID int64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	if f.sent == nil {
		f.sent = map[int64][]string{}
	}
	f.sent[chatID] = append(f.sent[chatID], text)
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

func (f *fakeMessenger) last(chatID int64) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.sent[chatID]
	if len(msgs) == 0 {
		return ""
	}
	return msgs[len(msgs)-1]
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

func (s *memStore) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.saved[id]
	return ok
}

type fixture struct {
	h    *Handler
	tg   *fakeMessenger
	pub  *publisher.Fake
	st   *memStore
	ids  *menuid.Generator
	logs *bytes.Buffer
	now  time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ids, err := menuid.New(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
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
		ratelimit.New(1, time.Hour, clock),
		f.tg,
		slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	)
	return f
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

func TestPublishFlow(t *testing.T) {
	tests := []struct {
		name          string
		update        func(f *fixture) tgbotapi.Update
		setup         func(f *fixture)
		wantReply     string // substring of the last reply to the sender
		wantPublished bool
		wantPubCalls  int
		wantSaved     bool
		wantDownloads int
	}{
		{
			name:          "admin publishes the list format",
			update:        func(*fixture) tgbotapi.Update { return textUpdate(adminID, validList) },
			wantReply:     "Menu published",
			wantPublished: true, wantPubCalls: 1, wantSaved: true,
		},
		{
			name:          "allowed user publishes JSON text",
			update:        func(*fixture) tgbotapi.Update { return textUpdate(allowedID, validJSON) },
			wantReply:     "Menu published",
			wantPublished: true, wantPubCalls: 1, wantSaved: true,
		},
		{
			name: "allowed user publishes a JSON file",
			update: func(f *fixture) tgbotapi.Update {
				f.tg.files["file-1"] = []byte(validJSON)
				return docUpdate(allowedID, "file-1", len(validJSON))
			},
			wantReply:     "Menu published",
			wantPublished: true, wantPubCalls: 1, wantSaved: true, wantDownloads: 1,
		},
		{
			name: "edited message re-submits",
			update: func(*fixture) tgbotapi.Update {
				u := textUpdate(allowedID, validList)
				return tgbotapi.Update{EditedMessage: u.Message}
			},
			wantReply:     "Menu published",
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
			name:      "invalid list",
			update:    func(*fixture) tgbotapi.Update { return textUpdate(allowedID, "hello bot") },
			wantReply: "Invalid menu",
		},
		{
			name: "unknown JSON field",
			update: func(*fixture) tgbotapi.Update {
				return textUpdate(allowedID, `{"categories":[{"id":"a","name":"A","items":[],"admin":true}]}`)
			},
			wantReply: `unknown field "admin"`,
		},
		{
			name: "unsafe image URL",
			update: func(*fixture) tgbotapi.Update {
				return textUpdate(allowedID, `{"categories":[{"id":"a","name":"A","items":[{"name":"x","price":1,"image":"javascript:alert(1)"}]}]}`)
			},
			wantReply: "image must be",
		},
		{
			name:         "publisher failure is reported without internals",
			setup:        func(f *fixture) { f.pub.Err = errors.New("github: PUT menu file: HTTP 500: secret internals") },
			update:       func(*fixture) tgbotapi.Update { return textUpdate(allowedID, validList) },
			wantReply:    "could not be published right now",
			wantPubCalls: 1,
		},
		{
			name:          "local copy failure does not fail the publish",
			setup:         func(f *fixture) { f.st.saveErr = errors.New("disk full") },
			update:        func(*fixture) tgbotapi.Update { return textUpdate(allowedID, validList) },
			wantReply:     "Menu published",
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

func TestPublishRateLimit(t *testing.T) {
	f := newFixture(t) // limit: 2 per hour
	ctx := context.Background()
	id := f.ids.ID(allowedID)

	// Invalid submissions do not consume the budget.
	for i := 0; i < 5; i++ {
		f.h.Handle(ctx, textUpdate(allowedID, "not a menu"))
	}
	for i := 0; i < 2; i++ {
		f.h.Handle(ctx, textUpdate(allowedID, validList))
		if got := f.tg.last(allowedID); !strings.Contains(got, "Menu published") {
			t.Fatalf("publish %d: reply = %q", i+1, got)
		}
	}
	f.h.Handle(ctx, textUpdate(allowedID, validList))
	if got := f.tg.last(allowedID); !strings.Contains(got, "publish limit") || !strings.Contains(got, "60 minutes") {
		t.Errorf("third publish: reply = %q, want a rate-limit message with the wait", got)
	}
	if f.pub.Calls() != 2 {
		t.Errorf("publisher calls = %d, want 2", f.pub.Calls())
	}

	// Another user has an independent budget.
	f.h.Handle(ctx, textUpdate(adminID, validList))
	if got := f.tg.last(adminID); !strings.Contains(got, "Menu published") {
		t.Errorf("other user: reply = %q", got)
	}

	// The window slides.
	f.now = f.now.Add(time.Hour + time.Second)
	f.h.Handle(ctx, textUpdate(allowedID, validList))
	if got := f.tg.last(allowedID); !strings.Contains(got, "Menu published") {
		t.Errorf("after the window: reply = %q", got)
	}
	if _, ok := f.pub.Get(id); !ok {
		t.Error("nothing published for the user")
	}
	assertNoRawIDs(t, f.logs.String())
}

func TestCommands(t *testing.T) {
	tests := []struct {
		name      string
		userID    int64
		text      string
		wantReply string
		wantNot   string
	}{
		{"start for stranger", strangerID, "/start", "/request_access", "/template"},
		{"help for allowed", allowedID, "/help", "/template", "/myid"},
		{"help for admin", adminID, "/help", "/myid", "/request_access"},
		{"status as admin", adminID, "/status", "Bot is running", ""},
		{"status as allowed", allowedID, "/status", "administrators only", ""},
		{"status as stranger", strangerID, "/status", "administrators only", ""},
		{"myid as admin", adminID, "/myid", strconv.FormatInt(adminID, 10), ""},
		{"myid as stranger", strangerID, "/myid", "administrators only", strconv.FormatInt(strangerID, 10)},
		{"template as allowed", allowedID, "/template", "CATEGORY: Appetizers", ""},
		{"template as stranger", strangerID, "/template", "not authorised", "CATEGORY"},
		{"myurl as allowed, nothing published", allowedID, "/myurl", "Nothing has been published", ""},
		{"myurl alias", allowedID, "/link", siteURL + "/menu/", ""},
		{"myurl as stranger", strangerID, "/myurl", "not authorised", siteURL},
		{"request_access when already allowed", allowedID, "/request_access", "already have access", ""},
		{"unknown command", allowedID, "/nope", "Unknown command", ""},
		{"command with bot suffix and args", adminID, "/status@demenu_bot now", "Bot is running", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.h.Handle(context.Background(), textUpdate(tt.userID, tt.text))
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

func TestMyURLAfterPublish(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.h.Handle(ctx, textUpdate(allowedID, validList))
	f.h.Handle(ctx, textUpdate(allowedID, "/myurl"))
	got := f.tg.last(allowedID)
	if want := siteURL + "/menu/" + f.ids.ID(allowedID); !strings.Contains(got, want) {
		t.Errorf("reply = %q, want it to contain %q", got, want)
	}
	if strings.Contains(got, "Nothing has been published") {
		t.Errorf("reply = %q claims nothing was published", got)
	}
}

func TestRequestAccess(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	f.h.Handle(ctx, textUpdate(strangerID, "/request_access"))
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
	f.h.Handle(ctx, textUpdate(strangerID, "/request_access"))
	if got := f.tg.last(strangerID); !strings.Contains(got, "already sent") {
		t.Errorf("second reply = %q", got)
	}
	if n := len(f.tg.sent[adminID]); n != 1 {
		t.Errorf("admin received %d notifications, want 1", n)
	}
	assertNoRawIDs(t, f.logs.String())
}

func TestRequestAccessWithoutAdmins(t *testing.T) {
	f := newFixture(t)
	f.h.opts.AdminUserIDs = nil
	f.h.Handle(context.Background(), textUpdate(strangerID, "/request_access"))
	if got := f.tg.last(strangerID); !strings.Contains(got, "No administrators") {
		t.Errorf("reply = %q", got)
	}
}

func TestIgnoredUpdates(t *testing.T) {
	private := func() *tgbotapi.Message { return textUpdate(adminID, validList).Message }
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.h.Handle(context.Background(), tt.update())
			if len(f.tg.sent) != 0 || f.pub.Calls() != 0 {
				t.Errorf("update was not ignored: sent=%v publishes=%d", f.tg.sent, f.pub.Calls())
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

func TestSendFailureIsLoggedNotFatal(t *testing.T) {
	f := newFixture(t)
	f.tg.sendErr = errors.New("telegram: send message: Forbidden")
	f.h.Handle(context.Background(), textUpdate(allowedID, validList))
	if f.pub.Calls() != 1 {
		t.Errorf("publisher calls = %d, want 1", f.pub.Calls())
	}
	if !strings.Contains(f.logs.String(), "could not send a reply") {
		t.Errorf("send failure was not logged: %s", f.logs.String())
	}
}

// assertNoRawIDs enforces the logging rule: users appear in logs only as
// their HMAC id.
func assertNoRawIDs(t *testing.T, logs string) {
	t.Helper()
	for _, raw := range []int64{adminID, allowedID, strangerID} {
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
		{time.Hour, "60 minutes"},
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
