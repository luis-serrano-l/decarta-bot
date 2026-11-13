package bot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// DefaultTelegramAPI is the public Bot API root.
	DefaultTelegramAPI = "https://api.telegram.org"

	pollTimeoutSeconds = 50
	httpTimeout        = 70 * time.Second // must exceed the long-poll timeout
	maxWorkers         = 8
	maxMessageUnits    = 4000 // Telegram rejects messages over 4096 UTF-16 code units
	initialBackoff     = 2 * time.Second
	maxBackoff         = time.Minute
)

// Client talks to the Telegram Bot API by long polling. Webhooks are not
// supported: polling needs no inbound connectivity, so the bot exposes
// nothing but a health endpoint.
type Client struct {
	token   string
	apiBase string
	http    *http.Client
	log     *slog.Logger
	backoff time.Duration

	api       *tgbotapi.BotAPI
	connected atomic.Bool
}

// NewClient returns a Client. apiBase is normally DefaultTelegramAPI; tests
// point it at a local server.
func NewClient(token, apiBase string, log *slog.Logger) *Client {
	return &Client{
		token:   token,
		apiBase: strings.TrimRight(apiBase, "/"),
		http:    &http.Client{Timeout: httpTimeout},
		log:     log,
		backoff: initialBackoff,
	}
}

// Connected reports whether the bot has authenticated with Telegram.
func (c *Client) Connected() bool { return c.connected.Load() }

// Run authenticates, then delivers updates to handle until ctx is
// cancelled. Authentication failures (bad token, no network) are retried
// with backoff rather than treated as fatal, so a container with a
// misconfigured token stays up, visibly unconnected, instead of crash
// looping.
func (c *Client) Run(ctx context.Context, handle func(context.Context, tgbotapi.Update)) error {
	if err := c.connect(ctx); err != nil {
		return err
	}

	// Polling and webhooks are mutually exclusive on Telegram's side, so
	// clear any webhook left over from an earlier deployment.
	if _, err := c.api.Request(tgbotapi.DeleteWebhookConfig{}); err != nil {
		c.log.Warn("telegram: could not clear webhook", "err", scrub(err))
	}

	// The command menu is a convenience; the bot works without it.
	if _, err := c.api.Request(tgbotapi.NewSetMyCommands(menuCommands()...)); err != nil {
		c.log.Warn("telegram: could not register the command menu", "err", scrub(err))
	}

	cfg := tgbotapi.NewUpdate(0)
	cfg.Timeout = pollTimeoutSeconds
	cfg.AllowedUpdates = []string{"message", "edited_message", "callback_query"}
	updates := c.api.GetUpdatesChan(cfg)
	c.log.Info("telegram: polling for updates")

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxWorkers)
	defer wg.Wait()
	for {
		select {
		case <-ctx.Done():
			c.api.StopReceivingUpdates()
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				defer func() {
					if r := recover(); r != nil {
						// One bad update must not take the bot down.
						c.log.Error("panic while handling an update", "panic", fmt.Sprint(r))
					}
				}()
				handle(ctx, update)
			}()
		}
	}
}

func (c *Client) connect(ctx context.Context) error {
	endpoint := c.apiBase + "/bot%s/%s"
	backoff := c.backoff
	for {
		api, err := tgbotapi.NewBotAPIWithClient(c.token, endpoint, c.http)
		if err == nil {
			c.api = api
			c.connected.Store(true)
			c.log.Info("telegram: authorised", "bot", api.Self.UserName)
			return nil
		}
		c.log.Error("telegram: cannot authenticate; check TELEGRAM_BOT_TOKEN and connectivity",
			"err", scrub(err), "retry_in", backoff.String())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

// Send implements Messenger.
func (c *Client) Send(chatID int64, text string, rows ...[]Button) error {
	msg := tgbotapi.NewMessage(chatID, clipMessage(text))
	if len(rows) > 0 {
		msg.ReplyMarkup = keyboard(rows)
	}
	if _, err := c.api.Send(msg); err != nil {
		return fmt.Errorf("telegram: send message: %w", scrub(err))
	}
	return nil
}

// Edit implements Messenger.
func (c *Client) Edit(chatID int64, messageID int, text string, rows ...[]Button) error {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, clipMessage(text))
	if len(rows) > 0 {
		markup := keyboard(rows)
		edit.ReplyMarkup = &markup
	}
	// Request, not Send: Send expects a Message back, Request only checks ok.
	if _, err := c.api.Request(edit); err != nil {
		return fmt.Errorf("telegram: edit message: %w", scrub(err))
	}
	return nil
}

// SendFile implements Messenger.
func (c *Client) SendFile(chatID int64, name string, content []byte, caption string) error {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FileBytes{Name: name, Bytes: content})
	doc.Caption = caption
	if _, err := c.api.Send(doc); err != nil {
		return fmt.Errorf("telegram: send file: %w", scrub(err))
	}
	return nil
}

// SendPhoto implements Messenger.
func (c *Client) SendPhoto(chatID int64, name string, content []byte, caption string) error {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{Name: name, Bytes: content})
	photo.Caption = caption
	if _, err := c.api.Send(photo); err != nil {
		return fmt.Errorf("telegram: send photo: %w", scrub(err))
	}
	return nil
}

// AnswerCallback implements Messenger.
func (c *Client) AnswerCallback(callbackID, text string) error {
	if _, err := c.api.Request(tgbotapi.NewCallback(callbackID, text)); err != nil {
		return fmt.Errorf("telegram: answer callback: %w", scrub(err))
	}
	return nil
}

// Typing implements Messenger.
func (c *Client) Typing(chatID int64) error {
	if _, err := c.api.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)); err != nil {
		return fmt.Errorf("telegram: send chat action: %w", scrub(err))
	}
	return nil
}

// clipMessage keeps text within Telegram's limit, which counts UTF-16 code
// units: an emoji is two.
func clipMessage(text string) string {
	units := 0
	for i, r := range text {
		units += utf16.RuneLen(r)
		if units > maxMessageUnits {
			return text[:i] + "…"
		}
	}
	return text
}

func keyboard(rows [][]Button) tgbotapi.InlineKeyboardMarkup {
	out := make([][]tgbotapi.InlineKeyboardButton, 0, len(rows))
	for _, row := range rows {
		line := make([]tgbotapi.InlineKeyboardButton, 0, len(row))
		for _, b := range row {
			if b.URL != "" {
				line = append(line, tgbotapi.NewInlineKeyboardButtonURL(b.Text, b.URL))
			} else {
				line = append(line, tgbotapi.NewInlineKeyboardButtonData(b.Text, b.Data))
			}
		}
		out = append(out, line)
	}
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: out}
}

func menuCommands() []tgbotapi.BotCommand {
	out := make([]tgbotapi.BotCommand, len(Commands))
	for i, c := range Commands {
		out[i] = tgbotapi.BotCommand{Command: c.Name, Description: c.Description}
	}
	return out
}

// Download implements Messenger.
func (c *Client) Download(ctx context.Context, fileID string, maxBytes int64) ([]byte, error) {
	file, err := c.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("telegram: get file: %w", scrub(err))
	}
	if int64(file.FileSize) > maxBytes {
		return nil, ErrTooLarge
	}
	// The download URL embeds the bot token. It must never be logged or
	// returned in an error; fetchCapped scrubs transport errors.
	u := fmt.Sprintf("%s/file/bot%s/%s", c.apiBase, c.token, file.FilePath)
	return fetchCapped(ctx, c.http, u, maxBytes)
}

// fetchCapped GETs rawURL and returns the body, or ErrTooLarge if it is
// longer than maxBytes. At most maxBytes+1 bytes are read. Errors never
// contain rawURL.
func fetchCapped(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("telegram: download: invalid file URL")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: download: %w", scrub(err))
	}
	defer resp.Body.Close() //nolint:errcheck // nothing useful to do with a close error on a read body

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram: download: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("telegram: download: %w", scrub(err))
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}

// scrub removes the request URL from transport errors. The Bot API carries
// the token in the URL path, and *url.Error prints the URL.
func scrub(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}
