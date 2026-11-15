package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/luis-serrano-l/decarta-bot/internal/menu"
)

// fakeSite is a SiteChecker that starts serving a menu after a number of
// looks, or never.
type fakeSite struct {
	mu        sync.Mutex
	liveAfter int // looks before the menu is online; negative for never
	err       error
	looks     int
	onLook    func()
}

func (s *fakeSite) Serves(_ context.Context, _ string, _ []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.looks++
	if s.onLook != nil {
		s.onLook()
	}
	return s.liveAfter >= 0 && s.looks > s.liveAfter, s.err
}

func TestPublishTellsWhenTheMenuIsOnline(t *testing.T) {
	tests := []struct {
		name      string
		site      *fakeSite
		wantFirst string
		wantLast  string
		wantOpen  bool
	}{
		{"online at once", &fakeSite{}, "⏳ Publishing your menu…", "✅ Your menu is online!", true},
		{"online after a few looks", &fakeSite{liveAfter: 3}, "I will send you a message here as soon as it is online", "✅ Your menu is online!", true},
		{"errors are not fatal", &fakeSite{liveAfter: 2, err: errors.New("boom")}, "⏳ Publishing", "✅ Your menu is online!", true},
		{"never online", &fakeSite{liveAfter: -1}, "⏳ Publishing", "taking longer than usual", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.h.opts.Site = tt.site
			f.send(allowedID, validList)
			f.press(allowedID, "Publish")
			first := f.tg.last(allowedID)
			f.h.watches.Wait()

			if !strings.Contains(first, tt.wantFirst) && !strings.Contains(f.tg.last(allowedID), tt.wantLast) {
				t.Errorf("after Publish = %q, want it to contain %q", first, tt.wantFirst)
			}
			m := f.tg.lastMsg(allowedID)
			if !strings.Contains(m.text, tt.wantLast) || !strings.Contains(m.text, siteURL+"/menu/"+f.ids.ID(allowedID)) {
				t.Errorf("last message = %q, want it to contain %q and the link", m.text, tt.wantLast)
			}
			if got := slicesContains(labels(m), "Open my menu"); got != tt.wantOpen {
				t.Errorf("Open my menu button present = %v; buttons %q", got, labels(m))
			}
			if f.pub.Calls() != 1 {
				t.Errorf("publish calls = %d", f.pub.Calls())
			}
			assertNoRawIDs(t, f.logs.String())
		})
	}
}

func slicesContains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// A delete, or a newer publish, while the site is still being watched
// must not be followed by "your menu is online" for the old one.
func TestWatchStopsWhenSuperseded(t *testing.T) {
	f := newFixture(t)
	release := make(chan struct{})
	site := &fakeSite{liveAfter: 1}
	var once sync.Once
	site.onLook = func() { once.Do(func() { <-release }) }
	f.h.opts.Site = site

	f.send(allowedID, validList)
	f.press(allowedID, "Publish")
	f.send(allowedID, "/delete")
	f.press(allowedID, "Yes, delete it")
	close(release)
	f.h.watches.Wait()

	if got := f.tg.last(allowedID); !strings.Contains(got, "Menu unpublished") {
		t.Errorf("last message = %q, want the delete confirmation", got)
	}
}

func TestHTTPSiteServes(t *testing.T) {
	const id = "0Aa1Bb2Cc3Dd4Ee5F"
	m, err := menu.Parse([]byte(validList))
	if err != nil {
		t.Fatal(err)
	}
	content, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		status  int
		body    string
		want    bool
		wantErr bool
	}{
		{"same content", http.StatusOK, string(content), true, false},
		{"same content, other trailing space", http.StatusOK, strings.TrimSpace(string(content)), true, false},
		{"older content", http.StatusOK, `{"categories":[]}`, false, false},
		{"longer content with the same start", http.StatusOK, string(content) + strings.Repeat("x", 4096), false, false},
		{"not deployed yet", http.StatusNotFound, "404", false, false},
		{"site error", http.StatusBadGateway, "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path, query string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path, query = r.URL.Path, r.URL.RawQuery
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			got, err := NewHTTPSite(srv.URL).Serves(context.Background(), id, content)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Errorf("Serves() = %v, %v; want %v, error %v", got, err, tt.want, tt.wantErr)
			}
			if path != "/menus/"+id+".json" || query == "" {
				t.Errorf("requested %s?%s, want the menu file with a cache-busting query", path, query)
			}
		})
	}

	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		if ok, err := NewHTTPSite(srv.URL).Serves(context.Background(), id, content); ok || err == nil {
			t.Errorf("Serves() = %v, %v; want false and an error", ok, err)
		}
	})
}
