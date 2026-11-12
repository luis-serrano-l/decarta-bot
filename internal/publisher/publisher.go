// Package publisher delivers validated menus to wherever they are served
// from. The bot itself never serves menus.
package publisher

import (
	"context"
	"errors"
	"sync"
)

// ErrInvalidID is returned when the id is not a well-formed menu id. Callers
// always pass ids from menuid.Generator, so this indicates a programming
// error; it is checked anyway because the id becomes part of a file path.
var ErrInvalidID = errors.New("publisher: invalid menu id")

// Publisher makes content available as the menu for id, replacing any
// previous version. Implementations must be safe for concurrent use.
type Publisher interface {
	Publish(ctx context.Context, id string, content []byte) error
}

// Fake is an in-memory Publisher for tests.
type Fake struct {
	// Err, if set, is returned by every Publish call.
	Err error

	mu        sync.Mutex
	published map[string][]byte
	calls     int
}

// Publish records content under id, or returns f.Err.
func (f *Fake) Publish(_ context.Context, id string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.Err != nil {
		return f.Err
	}
	if f.published == nil {
		f.published = make(map[string][]byte)
	}
	f.published[id] = append([]byte(nil), content...)
	return nil
}

// Get returns the last content published under id.
func (f *Fake) Get(id string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.published[id]
	return b, ok
}

// Calls returns how many times Publish was invoked, including failures.
func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
