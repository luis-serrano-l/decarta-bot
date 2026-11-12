package ratelimit

import (
	"sync"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestAllow(t *testing.T) {
	type step struct {
		advance   time.Duration // applied before the call
		key       string
		wantOK    bool
		wantRetry time.Duration
	}
	tests := []struct {
		name   string
		limit  int
		window time.Duration
		steps  []step
	}{
		{
			name: "allows up to the limit then denies", limit: 3, window: time.Hour,
			steps: []step{
				{0, "a", true, 0},
				{0, "a", true, 0},
				{0, "a", true, 0},
				{0, "a", false, time.Hour},
			},
		},
		{
			name: "keys are independent", limit: 1, window: time.Hour,
			steps: []step{
				{0, "a", true, 0},
				{0, "b", true, 0},
				{0, "a", false, time.Hour},
				{0, "b", false, time.Hour},
			},
		},
		{
			name: "window slides", limit: 2, window: time.Hour,
			steps: []step{
				{0, "a", true, 0},
				{30 * time.Minute, "a", true, 0},
				{29 * time.Minute, "a", false, time.Minute},
				{time.Minute, "a", true, 0},       // first hit expired exactly at the boundary
				{0, "a", false, 30 * time.Minute}, // second hit still live
				{30 * time.Minute, "a", true, 0},  // second hit expired
				{0, "a", false, 30 * time.Minute}, // third hit expires in 30m
				{24 * time.Hour, "a", true, 0},    // long idle
			},
		},
		{
			name: "denied calls do not extend the lockout", limit: 1, window: time.Hour,
			steps: []step{
				{0, "a", true, 0},
				{20 * time.Minute, "a", false, 40 * time.Minute},
				{20 * time.Minute, "a", false, 20 * time.Minute},
				{20 * time.Minute, "a", true, 0},
			},
		},
		{
			name: "zero limit denies everything", limit: 0, window: time.Hour,
			steps: []step{
				{0, "a", false, time.Hour},
				{2 * time.Hour, "a", false, time.Hour},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clock{t: time.Unix(1_700_000_000, 0)}
			l := New(tt.limit, tt.window, c.now)
			for i, s := range tt.steps {
				c.advance(s.advance)
				ok, retry := l.Allow(s.key)
				if ok != s.wantOK || retry != s.wantRetry {
					t.Fatalf("step %d: Allow(%q) = (%v, %v), want (%v, %v)",
						i, s.key, ok, retry, s.wantOK, s.wantRetry)
				}
			}
		})
	}
}

func TestSweepBoundsMemory(t *testing.T) {
	c := &clock{t: time.Unix(1_700_000_000, 0)}
	l := New(5, time.Hour, c.now)
	for _, k := range []string{"a", "b", "c"} {
		l.Allow(k)
	}
	c.advance(2 * time.Hour)
	l.Allow("d")
	if n := len(l.hits); n != 1 {
		t.Errorf("expired keys were not swept: %d keys remain", n)
	}
}

func TestConcurrent(t *testing.T) {
	const limit = 50
	l := New(limit, time.Hour, nil)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow("k"); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != limit {
		t.Errorf("allowed %d events, want exactly %d", allowed, limit)
	}
}
