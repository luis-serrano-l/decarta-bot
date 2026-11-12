// Package redact removes secrets from log output.
//
// It is a last line of defence, not the primary one: code should not pass
// secrets to the logger in the first place. It exists because some secrets
// travel inside values we do not control. The Telegram Bot API puts the bot
// token in the request URL, so any *url.Error from that client carries it.
package redact

import (
	"bytes"
	"io"
	"sync"
)

// Placeholder replaces every occurrence of a secret.
const Placeholder = "[REDACTED]"

// minSecretLen guards against redacting trivially short strings, which would
// mangle unrelated output.
const minSecretLen = 6

// Writer wraps an io.Writer and replaces each configured secret with
// Placeholder. Every Write must carry complete log records (as the log and
// slog packages do); secrets split across Write calls are not detected.
type Writer struct {
	mu      sync.Mutex
	out     io.Writer
	secrets [][]byte
}

// NewWriter returns a Writer that scrubs secrets from everything written to
// out. Empty and very short secrets are ignored.
func NewWriter(out io.Writer, secrets ...string) *Writer {
	w := &Writer{out: out}
	for _, s := range secrets {
		if len(s) >= minSecretLen {
			w.secrets = append(w.secrets, []byte(s))
		}
	}
	return w
}

// Write implements io.Writer. It reports len(p) on success regardless of how
// many bytes the redacted form took.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	clean := p
	for _, s := range w.secrets {
		if bytes.Contains(clean, s) {
			clean = bytes.ReplaceAll(clean, s, []byte(Placeholder))
		}
	}
	if _, err := w.out.Write(clean); err != nil {
		return 0, err
	}
	return len(p), nil
}
