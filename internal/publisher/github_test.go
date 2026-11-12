package publisher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	testToken  = "test-github-token-that-must-never-leak"
	testRepo   = "owner/site"
	testBranch = "main"
	testID     = "0Aa1Bb2Cc3Dd4Ee5F"
	testPath   = "/repos/owner/site/contents/static/menus/" + testID + ".json"
)

// fakeGitHub is a minimal stand-in for the Contents API holding one file.
type fakeGitHub struct {
	t *testing.T

	mu        sync.Mutex
	content   []byte // nil means the file does not exist
	puts      int
	deletes   int
	gets      int
	lastPut   putBody
	getStatus int // if non-zero, GET fails with this status
	putStatus int // if non-zero, PUT and DELETE fail with this status
	conflicts int // number of PUTs or DELETEs to answer with 409 before succeeding

	branchMissing bool // the repository or branch does not exist: every GET is a 404
	branchChecks  int
}

type putBody struct {
	Message string `json:"message"`
	Content string `json:"content"`
	Branch  string `json:"branch"`
	SHA     string `json:"sha"`
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
		f.t.Errorf("Authorization header = %q", got)
	}
	if got := r.Header.Get("X-GitHub-Api-Version"); got != apiVersion {
		f.t.Errorf("X-GitHub-Api-Version = %q", got)
	}
	if r.Header.Get("User-Agent") == "" {
		f.t.Error("missing User-Agent (GitHub rejects such requests)")
	}
	if strings.Contains(r.URL.String(), testToken) {
		f.t.Error("token appeared in the request URL")
	}
	if r.URL.Path == "/repos/"+testRepo+"/branches/"+testBranch && r.Method == http.MethodGet {
		f.branchChecks++
		if f.branchMissing {
			writeErr(w, http.StatusNotFound, "Branch not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"name": testBranch})
		return
	}
	if r.URL.Path != testPath {
		f.t.Errorf("path = %q, want %q", r.URL.Path, testPath)
	}

	switch r.Method {
	case http.MethodGet:
		f.gets++
		if got := r.URL.Query().Get("ref"); got != testBranch {
			f.t.Errorf("ref = %q, want %q", got, testBranch)
		}
		switch {
		case f.getStatus != 0:
			writeErr(w, f.getStatus, "get failed")
		case f.content == nil || f.branchMissing:
			writeErr(w, http.StatusNotFound, "Not Found")
		default:
			writeJSON(w, http.StatusOK, map[string]string{"type": "file", "sha": blobSHA(f.content)})
		}
	case http.MethodPut:
		f.puts++
		var body putBody
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			f.t.Errorf("bad PUT body: %v", err)
		}
		f.lastPut = body
		switch {
		case f.putStatus != 0:
			writeErr(w, f.putStatus, "put failed")
			return
		case f.conflicts > 0:
			f.conflicts--
			writeErr(w, http.StatusConflict, "is at abc but expected def")
			return
		}
		wantSHA := ""
		if f.content != nil {
			wantSHA = blobSHA(f.content)
		}
		if body.SHA != wantSHA {
			writeErr(w, http.StatusConflict, "sha mismatch")
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(body.Content)
		if err != nil {
			f.t.Errorf("content is not base64: %v", err)
		}
		status := http.StatusOK
		if f.content == nil {
			status = http.StatusCreated
		}
		f.content = decoded
		writeJSON(w, status, map[string]any{"content": map[string]string{"sha": blobSHA(decoded)}})
	case http.MethodDelete:
		f.deletes++
		var body putBody
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			f.t.Errorf("bad DELETE body: %v", err)
		}
		f.lastPut = body
		switch {
		case f.putStatus != 0:
			writeErr(w, f.putStatus, "delete failed")
		case f.conflicts > 0:
			f.conflicts--
			writeErr(w, http.StatusConflict, "is at abc but expected def")
		case f.content == nil:
			writeErr(w, http.StatusNotFound, "Not Found")
		case body.SHA != blobSHA(f.content):
			writeErr(w, http.StatusConflict, "sha mismatch")
		default:
			f.content = nil
			writeJSON(w, http.StatusOK, map[string]any{"content": nil})
		}
	default:
		f.t.Errorf("unexpected method %s", r.Method)
		writeErr(w, http.StatusMethodNotAllowed, "nope")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

func TestGitHubPublish(t *testing.T) {
	newContent := []byte("{\"categories\":[]}\n")

	tests := []struct {
		name        string
		id          string
		existing    []byte
		getStatus   int
		putStatus   int
		conflicts   int
		wantErr     string
		wantPuts    int
		wantSHASent bool
		wantStored  []byte
	}{
		{name: "creates a new file", id: testID, wantPuts: 1, wantStored: newContent},
		{name: "updates an existing file with its sha", id: testID, existing: []byte("old"), wantPuts: 1, wantSHASent: true, wantStored: newContent},
		{name: "unchanged content makes no commit", id: testID, existing: newContent, wantPuts: 0, wantStored: newContent},
		{name: "retries once on conflict", id: testID, existing: []byte("old"), conflicts: 1, wantPuts: 2, wantSHASent: true, wantStored: newContent},
		{name: "gives up after repeated conflicts", id: testID, existing: []byte("old"), conflicts: 5, wantErr: "HTTP 409", wantPuts: 2, wantSHASent: true, wantStored: []byte("old")},
		{name: "bad token", id: testID, getStatus: http.StatusUnauthorized, wantErr: "check GITHUB_TOKEN"},
		{name: "forbidden on write", id: testID, putStatus: http.StatusForbidden, wantErr: "Contents read/write", wantPuts: 1},
		{name: "server error on read", id: testID, getStatus: http.StatusBadGateway, wantErr: "HTTP 502"},
		{name: "server error on write", id: testID, putStatus: http.StatusInternalServerError, wantErr: "HTTP 500", wantPuts: 1},
		{name: "path traversal id", id: "../../README.md00", wantErr: ErrInvalidID.Error()},
		{name: "short id", id: "abc", wantErr: ErrInvalidID.Error()},
		{name: "empty id", id: "", wantErr: ErrInvalidID.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeGitHub{t: t, content: tt.existing, getStatus: tt.getStatus, putStatus: tt.putStatus, conflicts: tt.conflicts}
			srv := httptest.NewServer(fake)
			defer srv.Close()

			g, err := NewGitHub(testToken, testRepo, testBranch, WithAPIBase(srv.URL), WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}
			err = g.Publish(context.Background(), tt.id, newContent)

			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("Publish() error = %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("Publish() = nil, want error containing %q", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("Publish() error = %q, want it to contain %q", err, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), testToken) {
				t.Errorf("error leaks the token: %v", err)
			}
			if fake.puts != tt.wantPuts {
				t.Errorf("PUT requests = %d, want %d", fake.puts, tt.wantPuts)
			}
			if string(fake.content) != string(tt.wantStored) {
				t.Errorf("stored content = %q, want %q", fake.content, tt.wantStored)
			}
			if fake.puts > 0 {
				if (fake.lastPut.SHA != "") != tt.wantSHASent {
					t.Errorf("sha sent = %q, wantSHASent %v", fake.lastPut.SHA, tt.wantSHASent)
				}
				if fake.lastPut.Branch != testBranch {
					t.Errorf("branch = %q, want %q", fake.lastPut.Branch, testBranch)
				}
				if want := "menu: update " + testID; fake.lastPut.Message != want {
					t.Errorf("commit message = %q, want %q", fake.lastPut.Message, want)
				}
			}
		})
	}
}

func TestGitHubPublishTransportErrorDoesNotLeakToken(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // connection refused from here on

	g, err := NewGitHub(testToken, testRepo, testBranch, WithAPIBase(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = g.Publish(context.Background(), testID, []byte("x"))
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Errorf("error leaks the token: %v", err)
	}
}

func TestGitHubPublishHonoursContext(t *testing.T) {
	srv := httptest.NewServer(&fakeGitHub{t: t})
	defer srv.Close()
	g, err := NewGitHub(testToken, testRepo, testBranch, WithAPIBase(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Publish(ctx, testID, []byte("x")); !errors.Is(err, context.Canceled) {
		t.Errorf("Publish() error = %v, want context.Canceled", err)
	}
}

func TestNewGitHub(t *testing.T) {
	tests := []struct {
		name                string
		token, repo, branch string
		wantErr             bool
	}{
		{"ok", "t", "owner/repo", "main", false},
		{"ok with dots and slashes", "t", "my-org/my.site_repo", "release/v1.0", false},
		{"empty token", "", "owner/repo", "main", true},
		{"no owner", "t", "repo", "main", true},
		{"url instead of slug", "t", "https://github.com/owner/repo", "main", true},
		{"extra path", "t", "owner/repo/contents", "main", true},
		{"traversal in repo", "t", "owner/..", "main", true},
		{"query in repo", "t", "owner/repo?x=1", "main", true},
		{"empty branch", "t", "owner/repo", "", true},
		{"branch with space", "t", "owner/repo", "my branch", true},
		{"branch traversal", "t", "owner/repo", "a/../b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGitHub(tt.token, tt.repo, tt.branch)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGitHub() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBlobSHA(t *testing.T) {
	// Known values from `git hash-object`.
	tests := []struct{ in, want string }{
		{"", "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"},
		{"hello\n", "ce013625030ba8dba906f756967f9e9ca394464a"},
	}
	for _, tt := range tests {
		if got := blobSHA([]byte(tt.in)); got != tt.want {
			t.Errorf("blobSHA(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestFake(t *testing.T) {
	var f Fake
	var _ Publisher = &f
	if err := f.Publish(context.Background(), "a", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if got, ok := f.Get("a"); !ok || string(got) != "1" {
		t.Errorf("Get = %q, %v", got, ok)
	}
	f.Err = errors.New("boom")
	if err := f.Publish(context.Background(), "a", []byte("2")); err == nil {
		t.Error("expected error")
	}
	if got, _ := f.Get("a"); string(got) != "1" {
		t.Errorf("failed publish overwrote content: %q", got)
	}
	if f.Calls() != 2 {
		t.Errorf("Calls() = %d, want 2", f.Calls())
	}
}

func TestGitHubDelete(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		existing    []byte
		getStatus   int
		delStatus   int
		conflicts   int
		wantErr     string
		wantIs      error
		wantDeletes int
		wantGone    bool
		noBranch    bool
	}{
		{name: "wrong repository or branch is not mistaken for nothing published", id: testID, existing: []byte("old"), noBranch: true, wantErr: "check GITHUB_REPO, GITHUB_BRANCH"},
		{name: "deletes an existing file", id: testID, existing: []byte("old"), wantDeletes: 1, wantGone: true},
		{name: "nothing published", id: testID, wantIs: ErrNotFound, wantGone: true},
		{name: "retries once on conflict", id: testID, existing: []byte("old"), conflicts: 1, wantDeletes: 2, wantGone: true},
		{name: "gives up after repeated conflicts", id: testID, existing: []byte("old"), conflicts: 5, wantErr: "HTTP 409", wantDeletes: 2},
		{name: "bad token", id: testID, existing: []byte("old"), getStatus: http.StatusUnauthorized, wantErr: "check GITHUB_TOKEN"},
		{name: "forbidden on write", id: testID, existing: []byte("old"), delStatus: http.StatusForbidden, wantErr: "Contents read/write", wantDeletes: 1},
		{name: "path traversal id", id: "../../README.md00", existing: []byte("old"), wantIs: ErrInvalidID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeGitHub{t: t, content: tt.existing, getStatus: tt.getStatus, putStatus: tt.delStatus, conflicts: tt.conflicts, branchMissing: tt.noBranch}
			srv := httptest.NewServer(fake)
			defer srv.Close()

			g, err := NewGitHub(testToken, testRepo, testBranch, WithAPIBase(srv.URL), WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}
			err = g.Delete(context.Background(), tt.id)

			switch {
			case tt.wantIs != nil:
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("Delete() error = %v, want %v", err, tt.wantIs)
				}
			case tt.wantErr == "" && err != nil:
				t.Fatalf("Delete() error = %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("Delete() error = %v, want it to contain %q", err, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), testToken) {
				t.Errorf("error leaks the token: %v", err)
			}
			if fake.deletes != tt.wantDeletes {
				t.Errorf("DELETE requests = %d, want %d", fake.deletes, tt.wantDeletes)
			}
			if gone := fake.content == nil; gone != tt.wantGone {
				t.Errorf("file gone = %v, want %v", gone, tt.wantGone)
			}
			if tt.wantDeletes > 0 {
				if fake.lastPut.Message != "menu: delete "+testID || fake.lastPut.Branch != testBranch || fake.lastPut.SHA == "" {
					t.Errorf("DELETE body = %+v", fake.lastPut)
				}
			}
		})
	}
}

func TestFakeDelete(t *testing.T) {
	f := &Fake{}
	ctx := context.Background()
	if err := f.Delete(ctx, testID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete() before any publish = %v, want ErrNotFound", err)
	}
	if err := f.Publish(ctx, testID, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := f.Delete(ctx, testID); err != nil {
		t.Errorf("Delete() = %v", err)
	}
	if _, ok := f.Get(testID); ok {
		t.Error("content is still published after Delete")
	}
	if f.Deletes() != 2 {
		t.Errorf("Deletes() = %d, want 2", f.Deletes())
	}
}
