// Package ratelimit implements a small in-memory sliding-window limiter.
//
// State is per process and is lost on restart, which is acceptable here: the
// limiter bounds how often an authorised user can trigger a commit (and a
// site rebuild) in the companion repository, it is not a security boundary.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter allows at most limit events per key in any window-long interval.
// It is safe for concurrent use.
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[string][]time.Time
}

// New returns a Limiter. A nil now defaults to time.Now. A limit below 1
// denies everything.
func New(limit int, window time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{
		limit:  limit,
		window: window,
		now:    now,
		hits:   make(map[string][]time.Time),
	}
}

// Allow records an event for key and reports whether it is within the limit.
// When it is not, retryAfter is how long until the next event would be
// allowed. Denied events are not recorded.
func (l *Limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)

	hits := l.hits[key]
	live := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			live = append(live, t)
		}
	}

	if l.limit < 1 {
		return false, l.window
	}
	if len(live) >= l.limit {
		l.hits[key] = live
		return false, live[0].Add(l.window).Sub(now)
	}
	l.hits[key] = append(live, now)
	l.sweep(cutoff)
	return true, 0
}

// Undo takes back the most recent event recorded for key, for a caller whose
// allowed action then failed and should not count. It does nothing if key
// has no recorded events.
func (l *Limiter) Undo(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	hits := l.hits[key]
	switch len(hits) {
	case 0:
	case 1:
		delete(l.hits, key)
	default:
		l.hits[key] = hits[:len(hits)-1]
	}
}

// sweep drops keys whose events have all expired so the map cannot grow
// without bound. Called with mu held.
func (l *Limiter) sweep(cutoff time.Time) {
	for k, hits := range l.hits {
		if len(hits) == 0 || !hits[len(hits)-1].After(cutoff) {
			delete(l.hits, k)
		}
	}
}
