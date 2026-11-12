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

// ErrNotFound is returned by Delete when there is no published menu for
// the id.
var ErrNotFound = errors.New("publisher: no published menu")

// Publisher makes menus available and takes them down again.
// Implementations must be safe for concurrent use.
type Publisher interface {
	// Publish makes content the menu for id, replacing any previous
	// version.
	Publish(ctx context.Context, id string, content []byte) error
	// Delete unpublishes the menu for id. It returns ErrNotFound if there
	// is none.
	Delete(ctx context.Context, id string) error
}

// Fake is an in-memory Publisher for tests.
type Fake struct {
	// Err, if set, is returned by every Publish and Delete call.
	Err error

	mu        sync.Mutex
	published map[string][]byte
	calls     int
	deletes   int
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

// Delete removes the content published under id, or returns f.Err.
func (f *Fake) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.published[id]; !ok {
		return ErrNotFound
	}
	delete(f.published, id)
	return nil
}

// Deletes returns how many times Delete was invoked, including failures.
func (f *Fake) Deletes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deletes
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
