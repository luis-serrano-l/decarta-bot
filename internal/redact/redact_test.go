package redact

import (
	"bytes"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"testing"
)

const (
	tgToken = "0000000000:TEST_telegram-token_not-real"
	ghToken = "test-github-token-not-real"
)

func TestWriter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no secret", "hello world\n", "hello world\n"},
		{"telegram url", "Post \"https://api.telegram.org/bot" + tgToken + "/getMe\": EOF\n",
			"Post \"https://api.telegram.org/bot[REDACTED]/getMe\": EOF\n"},
		{"both secrets, repeated", tgToken + " " + ghToken + " " + tgToken, "[REDACTED] [REDACTED] [REDACTED]"},
		{"short secret is ignored", "abc abc", "abc abc"},
		{"empty write", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewWriter(&buf, tgToken, ghToken, "", "abc")
			n, err := w.Write([]byte(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if n != len(tt.in) {
				t.Errorf("Write() = %d, want %d", n, len(tt.in))
			}
			if buf.String() != tt.want {
				t.Errorf("got %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

// The realistic leak: a transport error from the Telegram client, logged
// through slog as an attribute.
func TestWriterWithSlogAndURLError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(NewWriter(&buf, tgToken), nil))

	err := &url.Error{Op: "Post", URL: "https://api.telegram.org/bot" + tgToken + "/getUpdates", Err: errors.New("timeout")}
	logger.Error("telegram request failed", "err", err)

	out := buf.String()
	if strings.Contains(out, tgToken) || strings.Contains(out, "TEST_telegram") {
		t.Errorf("token leaked: %s", out)
	}
	if !strings.Contains(out, Placeholder) {
		t.Errorf("expected placeholder in %s", out)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWriterPropagatesErrors(t *testing.T) {
	if _, err := NewWriter(failWriter{}, tgToken).Write([]byte("x")); err == nil {
		t.Error("expected the underlying error")
	}
}
