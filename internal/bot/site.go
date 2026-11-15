package bot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// A published menu is a commit; the page changes only when the site has
// been rebuilt, about a minute later. The handler watches for that moment
// so that it can tell the owner the menu is really online.

// How the handler watches the site after a publish: the first look, the
// pause between looks, and when it gives up.
const (
	liveFirstCheck = 20 * time.Second
	liveInterval   = 10 * time.Second
	liveTimeout    = 5 * time.Minute
	liveCheckLimit = 15 * time.Second // one request
)

// SiteChecker reports whether the public site serves a menu yet.
type SiteChecker interface {
	// Serves reports whether the site's copy of the menu id is content.
	Serves(ctx context.Context, id string, content []byte) (bool, error)
}

// HTTPSite is a SiteChecker that fetches the menu file from the site.
type HTTPSite struct {
	base   string
	client *http.Client
}

// NewHTTPSite checks the site at baseURL, given without a trailing slash.
func NewHTTPSite(baseURL string) *HTTPSite {
	return &HTTPSite{base: baseURL, client: &http.Client{Timeout: liveCheckLimit}}
}

// Serves fetches menus/<id>.json and compares it with content. A missing
// file is "not yet", not an error.
func (s *HTTPSite) Serves(ctx context.Context, id string, content []byte) (bool, error) {
	// The query defeats caches between here and the site. The host is the
	// operator's own site, from configuration, and the id is base62.
	url := s.base + "/menus/" + id + ".json?t=" + strconv.FormatInt(time.Now().UnixNano(), 36)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("site: build request: %w", err)
	}
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := s.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("site: fetch: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing to do about it
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	case resp.StatusCode != http.StatusOK:
		return false, fmt.Errorf("site: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(len(content))+1024))
	if err != nil {
		return false, fmt.Errorf("site: read: %w", err)
	}
	return bytes.Equal(bytes.TrimSpace(body), bytes.TrimSpace(content)), nil
}

// supersede makes any watch running for id stop without a word, and
// returns the generation a new watch must carry.
func (h *Handler) supersede(id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.watchGen[id]++
	return h.watchGen[id]
}

func (h *Handler) superseded(id string, gen int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.watchGen[id] != gen
}

// watchLive looks at the site until it serves content, then tells the
// user their menu is online; after liveFor it says the page is slow
// instead. A newer publish or a delete ends the watch silently.
func (h *Handler) watchLive(u user, content []byte) {
	gen := h.supersede(u.id)
	url := h.menuURL(u.id)
	h.watches.Add(1)
	go func() {
		defer h.watches.Done()
		start := time.Now()
		for wait := h.liveWait; ; wait = h.liveEvery {
			time.Sleep(wait)
			if h.superseded(u.id, gen) {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), liveCheckLimit)
			ok, err := h.opts.Site.Serves(ctx, u.id, content)
			cancel()
			if err != nil {
				h.log.Debug("could not check the site", "id", u.id, "err", err)
			}
			if h.superseded(u.id, gen) {
				return
			}
			if ok {
				h.log.Info("menu online", "id", u.id, "seconds", int(time.Since(start).Seconds()))
				h.reply(u, "✅ Your menu is online!\n\n🔗 "+url+
					"\n\nUse /link anytime to get this link again, or /qr for a QR code.",
					openButton(url), []Button{{Text: "Get QR code", Data: actQR}})
				return
			}
			if time.Since(start) >= h.liveFor {
				h.log.Warn("menu not online in time", "id", u.id)
				h.reply(u, "⌛ Your menu was published, but the page is taking longer than usual to update.\n\n"+
					"Open the link in a few minutes:\n🔗 "+url, openButton(url))
				return
			}
		}
	}()
}
