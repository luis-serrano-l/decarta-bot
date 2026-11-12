package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		connected bool
		wantCode  int
		wantBody  string
	}{
		{"healthy before telegram connects", http.MethodGet, "/health", false, http.StatusOK, `{"status":"ok","telegram":"connecting"}`},
		{"healthy and connected", http.MethodGet, "/health", true, http.StatusOK, `{"status":"ok","telegram":"connected"}`},
		{"POST is not allowed", http.MethodPost, "/health", true, http.StatusMethodNotAllowed, ""},
		{"root is not served", http.MethodGet, "/", true, http.StatusNotFound, ""},
		{"old default menu endpoint is gone", http.MethodGet, "/api/menu.json", true, http.StatusNotFound, ""},
		{"old user menu endpoint is gone", http.MethodGet, "/api/user/0000001abcdef/menu.json", true, http.StatusNotFound, ""},
		{"old debug endpoint is gone", http.MethodGet, "/api/debug/user/0000001abcdef", true, http.StatusNotFound, ""},
		{"old webhook endpoint is gone", http.MethodPost, "/webhook", true, http.StatusNotFound, ""},
		{"CORS preflight gets no CORS headers", http.MethodOptions, "/health", true, http.StatusMethodNotAllowed, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := healthHandler(func() bool { return tt.connected })
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("Origin", "https://evil.example")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			for name := range rec.Header() {
				if strings.HasPrefix(name, "Access-Control-") {
					t.Errorf("unexpected CORS header %s", name)
				}
			}
		})
	}
}

func TestProbe(t *testing.T) {
	healthy := httptest.NewServer(healthHandler(func() bool { return false }))
	defer healthy.Close()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	}))
	defer failing.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closedPort := port(t, closed.URL)
	closed.Close()

	tests := []struct {
		name string
		port string
		want int
	}{
		{"healthy", port(t, healthy.URL), 0},
		{"server error", port(t, failing.URL), 1},
		{"nothing listening", closedPort, 1},
		{"not a port", "8080/../x", 1},
		{"out of range", "70000", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := probe(tt.port); got != tt.want {
				t.Errorf("probe() = %d, want %d", got, tt.want)
			}
		})
	}
}

func port(t *testing.T, url string) string {
	t.Helper()
	_, p, err := net.SplitHostPort(strings.TrimPrefix(url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
