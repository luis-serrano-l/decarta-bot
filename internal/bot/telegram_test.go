package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
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

const fakeToken = "0000000000:TEST_telegram-token_not-real"

// fakeTelegram implements just enough of the Bot API for Client.
type fakeTelegram struct {
	t *testing.T

	mu           sync.Mutex
	authFailures int               // getMe calls to reject before succeeding
	updates      []tgbotapi.Update // delivered once by the first getUpdates
	files        map[string][]byte // file_path -> content
	sent         map[int64][]string
	webhookGone  bool

	nextMsgID  int
	nextUpdate int
	queued     []tgbotapi.Update   // delivered by later getUpdates calls
	autoPress  string              // label of the button to press as soon as it is offered
	commands   []string            // registered with setMyCommands
	markups    map[int64][]string  // reply_markup of each message or edit, per chat
	docs       map[int64][]sentDoc // files sent with sendDocument
	answered   int                 // answerCallbackQuery calls
	typing     int                 // sendChatAction calls
}

type sentDoc struct {
	name    string
	content []byte
	caption string
}

// record stores a message or an edit and, if it offers the autoPress
// button, queues the callback a user's tap would produce.
func (f *fakeTelegram) record(chatID int64, msgID int, text, markup string) {
	if f.sent == nil {
		f.sent = map[int64][]string{}
		f.markups = map[int64][]string{}
	}
	f.sent[chatID] = append(f.sent[chatID], text)
	f.markups[chatID] = append(f.markups[chatID], markup)

	var kb tgbotapi.InlineKeyboardMarkup
	if markup == "" || json.Unmarshal([]byte(markup), &kb) != nil {
		return
	}
	for _, row := range kb.InlineKeyboard {
		for _, b := range row {
			if b.Text != f.autoPress || b.CallbackData == nil {
				continue
			}
			f.nextUpdate++
			f.queued = append(f.queued, tgbotapi.Update{
				UpdateID:      1000 + f.nextUpdate,
				CallbackQuery: callbackUpdate(chatID, msgID, fmt.Sprintf("cb-%d", f.nextUpdate), *b.CallbackData).CallbackQuery,
			})
		}
	}
}

func (f *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if path, ok := strings.CutPrefix(r.URL.Path, "/file/bot"+fakeToken+"/"); ok {
		data, found := f.files[path]
		if !found {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
		return
	}
	method, ok := strings.CutPrefix(r.URL.Path, "/bot"+fakeToken+"/")
	if !ok {
		f.t.Errorf("unexpected request path %q", r.URL.Path)
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			f.t.Errorf("bad multipart form: %v", err)
		}
	} else if err := r.ParseForm(); err != nil {
		f.t.Errorf("bad form: %v", err)
	}
	chatID, _ := strconv.ParseInt(r.Form.Get("chat_id"), 10, 64)
	message := func(id int) map[string]any {
		return map[string]any{"message_id": id, "chat": map[string]any{"id": chatID, "type": "private"}}
	}

	switch method {
	case "getMe":
		if f.authFailures > 0 {
			f.authFailures--
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"ok":false,"error_code":401,"description":"Unauthorized"}`)
			return
		}
		f.ok(w, map[string]any{"id": 42, "is_bot": true, "username": "test_bot"})
	case "deleteWebhook":
		f.webhookGone = true
		f.ok(w, true)
	case "getUpdates":
		pending := f.updates
		f.updates = nil
		if pending == nil {
			pending, f.queued = f.queued, nil
		}
		if pending == nil {
			pending = []tgbotapi.Update{}
			// Emulate (a very short) long poll so the client does not spin.
			f.mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			f.mu.Lock()
		}
		f.ok(w, pending)
	case "sendMessage":
		f.nextMsgID++
		f.record(chatID, f.nextMsgID, r.Form.Get("text"), r.Form.Get("reply_markup"))
		f.ok(w, message(f.nextMsgID))
	case "editMessageText":
		msgID, _ := strconv.Atoi(r.Form.Get("message_id"))
		f.record(chatID, msgID, r.Form.Get("text"), r.Form.Get("reply_markup"))
		f.ok(w, message(msgID))
	case "sendDocument":
		file, header, err := r.FormFile("document")
		if err != nil {
			f.t.Errorf("sendDocument without a document: %v", err)
			http.Error(w, "no document", http.StatusBadRequest)
			return
		}
		content, _ := io.ReadAll(file)
		if f.docs == nil {
			f.docs = map[int64][]sentDoc{}
		}
		f.docs[chatID] = append(f.docs[chatID], sentDoc{header.Filename, content, r.Form.Get("caption")})
		f.nextMsgID++
		f.ok(w, message(f.nextMsgID))
	case "setMyCommands":
		var cmds []tgbotapi.BotCommand
		if err := json.Unmarshal([]byte(r.Form.Get("commands")), &cmds); err != nil {
			f.t.Errorf("setMyCommands: bad commands %q: %v", r.Form.Get("commands"), err)
		}
		f.commands = nil
		for _, c := range cmds {
			f.commands = append(f.commands, c.Command)
		}
		f.ok(w, true)
	case "answerCallbackQuery":
		f.answered++
		f.ok(w, true)
	case "sendChatAction":
		if r.Form.Get("action") != "typing" {
			f.t.Errorf("sendChatAction: action = %q, want typing", r.Form.Get("action"))
		}
		f.typing++
		f.ok(w, true)
	case "getFile":
		path := r.Form.Get("file_id") + ".json"
		f.ok(w, map[string]any{"file_id": r.Form.Get("file_id"), "file_size": len(f.files[path]), "file_path": path})
	default:
		f.t.Errorf("unexpected Bot API method %q", method)
		http.NotFound(w, r)
	}
}

func (f *fakeTelegram) ok(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func (f *fakeTelegram) lastSent(chatID int64) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.sent[chatID]
	if len(msgs) == 0 {
		return ""
	}
	return msgs[len(msgs)-1]
}

// syncBuffer is a bytes.Buffer safe for the concurrent writes slog makes.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestEndToEnd drives the whole path with only the network edges faked:
// Telegram long poll -> handler -> validation -> preview -> Publish button
// -> rate limit -> publisher -> reply. The fake presses Publish as soon as
// it is offered. The first getMe calls are rejected to exercise the
// reconnect loop.
func TestEndToEnd(t *testing.T) {
	tg := &fakeTelegram{
		t:            t,
		authFailures: 2,
		autoPress:    "Publish",
		files:        map[string][]byte{"doc-1.json": []byte(validJSON)},
		updates: []tgbotapi.Update{
			{UpdateID: 1, Message: textUpdate(adminID, validList).Message},
			{UpdateID: 2, Message: docUpdate(allowedID, "doc-1", len(validJSON)).Message},
			{UpdateID: 3, Message: textUpdate(strangerID, validList).Message},
			{UpdateID: 4, Message: textUpdate(strangerID, "/start").Message},
			{UpdateID: 5, Message: textUpdate(templateUserID, "/template").Message},
		},
	}
	srv := httptest.NewServer(tg)
	defer srv.Close()

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	ids, err := menuid.New(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	pub := &publisher.Fake{}
	client := NewClient(fakeToken, srv.URL, logger)
	client.backoff = time.Millisecond
	handler := NewHandler(
		Options{SiteBaseURL: siteURL, AllowedUserIDs: []int64{allowedID, templateUserID}, AdminUserIDs: []int64{adminID}, MaxUploadBytes: 1024},
		ids, pub, &memStore{},
		ratelimit.New(5, time.Hour, nil), ratelimit.New(20, time.Minute, nil), ratelimit.New(1, time.Hour, nil),
		client, logger,
	)

	if client.Connected() {
		t.Error("Connected() = true before Run")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx, handler.Handle) }()

	waitFor(t, "both menus to be published and every user to get a reply", func() bool {
		tg.mu.Lock()
		docs, stranger := len(tg.docs[templateUserID]), len(tg.sent[strangerID])
		tg.mu.Unlock()
		return strings.Contains(tg.lastSent(adminID), "Menu published") &&
			strings.Contains(tg.lastSent(allowedID), "Menu published") &&
			stranger == 2 && docs == 1
	})
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() = %v, want nil on cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}

	if !client.Connected() {
		t.Error("Connected() = false after a successful run")
	}
	if !tg.webhookGone {
		t.Error("the webhook was not cleared before polling")
	}
	for _, userID := range []int64{adminID, allowedID} {
		id := ids.ID(userID)
		content, ok := pub.Get(id)
		if !ok || !strings.Contains(string(content), `"name": "Soup"`) {
			t.Errorf("user %s: published = %v, content = %q", id, ok, content)
		}
		if want := siteURL + "/menu/" + id; !strings.Contains(tg.lastSent(userID), want) {
			t.Errorf("reply %q does not contain %q", tg.lastSent(userID), want)
		}
	}
	if _, ok := pub.Get(ids.ID(strangerID)); ok {
		t.Error("an unauthorised user's menu was published")
	}

	tg.mu.Lock()
	defer tg.mu.Unlock()
	if got := strings.Join(tg.sent[strangerID], "\n"); !strings.Contains(got, "not authorised") {
		t.Errorf("stranger replies = %q", got)
	}
	if want := []string{"start", "help", "template", "link", "status", "delete"}; !slices.Equal(tg.commands, want) {
		t.Errorf("registered commands = %q, want %q", tg.commands, want)
	}
	// Over the wire: /start carries its two callback buttons, the final
	// message a URL button, and the template arrives as a file.
	if got := strings.Join(tg.markups[strangerID], ""); !strings.Contains(got, `"callback_data":"template"`) || !strings.Contains(got, `"callback_data":"how"`) {
		t.Errorf("/start buttons = %s", got)
	}
	markups := tg.markups[adminID]
	if want := `"url":"` + siteURL + "/menu/" + ids.ID(adminID) + `"`; !strings.Contains(markups[len(markups)-1], want) {
		t.Errorf("final markup %s does not contain %s", markups[len(markups)-1], want)
	}
	if doc := tg.docs[templateUserID][0]; doc.name != "menu.json" || !bytes.Equal(doc.content, templateJSON) || doc.caption == "" {
		t.Errorf("template document = %q, %d bytes, caption %q", doc.name, len(doc.content), doc.caption)
	}
	if tg.answered != 2 {
		t.Errorf("answered %d callbacks, want 2", tg.answered)
	}
	if tg.typing < 4 {
		t.Errorf("typing shown %d times, want at least once per check and per publish", tg.typing)
	}

	out := logs.String()
	if !strings.Contains(out, "cannot authenticate") {
		t.Errorf("the failed authentication was not logged:\n%s", out)
	}
	if strings.Contains(out, fakeToken) || strings.Contains(out, "TEST_telegram") {
		t.Errorf("logs contain the bot token:\n%s", out)
	}
	assertNoRawIDs(t, out)
}

func TestRunStopsWhileAuthenticationKeepsFailing(t *testing.T) {
	srv := httptest.NewServer(&fakeTelegram{t: t, authFailures: 1 << 30})
	defer srv.Close()

	logs := &syncBuffer{}
	client := NewClient(fakeToken, srv.URL, slog.New(slog.NewTextHandler(logs, nil)))
	client.backoff = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := client.Run(ctx, func(context.Context, tgbotapi.Update) { t.Error("handler called") })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run() = %v, want context.DeadlineExceeded", err)
	}
	if client.Connected() {
		t.Error("Connected() = true although authentication never succeeded")
	}
	if strings.Contains(logs.String(), fakeToken) {
		t.Errorf("logs contain the bot token:\n%s", logs.String())
	}
}

func TestConnectErrorDoesNotLeakToken(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // connection refused: the error is a *url.Error carrying the token

	logs := &syncBuffer{}
	client := NewClient(fakeToken, srv.URL, slog.New(slog.NewTextHandler(logs, nil)))
	client.backoff = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = client.Run(ctx, func(context.Context, tgbotapi.Update) {})

	out := logs.String()
	if !strings.Contains(out, "cannot authenticate") {
		t.Fatalf("expected an authentication error in the logs:\n%s", out)
	}
	if strings.Contains(out, fakeToken) || strings.Contains(out, "/bot") {
		t.Errorf("logs contain the token-bearing URL:\n%s", out)
	}
}

func TestFetchCapped(t *testing.T) {
	const limit = 100
	body := func(n int) []byte { return bytes.Repeat([]byte("x"), n) }

	mux := http.NewServeMux()
	for _, n := range []int{0, limit - 1, limit, limit + 1, 10 * limit} {
		mux.HandleFunc(fmt.Sprintf("/sized/%d", n), func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(body(n))
		})
		// Chunked: no Content-Length, so only the read cap protects us.
		mux.HandleFunc(fmt.Sprintf("/chunked/%d", n), func(w http.ResponseWriter, _ *http.Request) {
			fl := w.(http.Flusher)
			for sent := 0; sent < n; sent += 10 {
				_, _ = w.Write(body(min(10, n-sent)))
				fl.Flush()
			}
		})
	}
	mux.HandleFunc("/missing", http.NotFound)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		path    string
		wantLen int
		wantErr error  // matched with errors.Is
		wantMsg string // or by substring
	}{
		{"/sized/0", 0, nil, ""},
		{"/sized/99", 99, nil, ""},
		{"/sized/100", 100, nil, ""},
		{"/sized/101", 0, ErrTooLarge, ""},
		{"/sized/1000", 0, ErrTooLarge, ""},
		{"/chunked/100", 100, nil, ""},
		{"/chunked/101", 0, ErrTooLarge, ""},
		{"/chunked/1000", 0, ErrTooLarge, ""},
		{"/missing", 0, nil, "HTTP 404"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			data, err := fetchCapped(context.Background(), srv.Client(), srv.URL+tt.path, limit)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
			case tt.wantMsg != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
					t.Fatalf("error = %v, want it to contain %q", err, tt.wantMsg)
				}
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			}
			if len(data) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(data), tt.wantLen)
			}
		})
	}
}

func TestFetchCappedErrorsOmitTheURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()

	for _, u := range []string{
		base + "/file/bot" + fakeToken + "/x.json",    // connection refused
		"http://[::1]:namedport/file/bot" + fakeToken, // unparsable
	} {
		_, err := fetchCapped(context.Background(), http.DefaultClient, u, 10)
		if err == nil {
			t.Fatalf("expected an error for %q", u)
		}
		if strings.Contains(err.Error(), fakeToken) || strings.Contains(err.Error(), "/file/bot") {
			t.Errorf("error leaks the download URL: %v", err)
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
