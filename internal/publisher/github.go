package publisher

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // git blob ids are SHA-1; used for change detection, not security
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/luis-serrano-l/demenu_bot/internal/menuid"
)

const (
	// DefaultGitHubAPI is the public GitHub REST API root.
	DefaultGitHubAPI = "https://api.github.com"

	// MenuDir is the directory in the site repository that holds menus.
	MenuDir = "static/menus"

	userAgent       = "demenu-bot"
	apiVersion      = "2022-11-28"
	maxResponseSize = 1 << 20
	maxAttempts     = 2
)

var (
	repoRe   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)
	branchRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,255}$`)
)

// GitHub publishes menus as static/menus/<id>.json in a repository through
// the Contents API. Each publish or delete is one commit on the configured
// branch.
//
// The token needs only "Contents: read and write" on that single repository
// (a fine-grained personal access token); it is sent in the Authorization
// header and never placed in a URL or an error.
type GitHub struct {
	token  string
	repo   string
	branch string
	api    string
	client *http.Client
}

// GitHubOption customises a GitHub publisher.
type GitHubOption func(*GitHub)

// WithAPIBase points the publisher at a different API root (GitHub
// Enterprise, or a test server).
func WithAPIBase(base string) GitHubOption {
	return func(g *GitHub) { g.api = strings.TrimRight(base, "/") }
}

// WithHTTPClient replaces the default client (15 s timeout).
func WithHTTPClient(c *http.Client) GitHubOption {
	return func(g *GitHub) { g.client = c }
}

// NewGitHub returns a publisher for repo ("owner/name") on branch.
func NewGitHub(token, repo, branch string, opts ...GitHubOption) (*GitHub, error) {
	if token == "" {
		return nil, errors.New("publisher: GitHub token is empty")
	}
	if !repoRe.MatchString(repo) || strings.Contains(repo, "..") {
		return nil, errors.New(`publisher: GitHub repo must look like "owner/name"`)
	}
	if !branchRe.MatchString(branch) || strings.Contains(branch, "..") {
		return nil, errors.New("publisher: invalid GitHub branch name")
	}
	g := &GitHub{
		token:  token,
		repo:   repo,
		branch: branch,
		api:    DefaultGitHubAPI,
		client: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(g)
	}
	return g, nil
}

// Publish creates or updates static/menus/<id>.json. If the file already has
// exactly this content no commit is made.
func (g *GitHub) Publish(ctx context.Context, id string, content []byte) error {
	if !menuid.Valid(id) {
		return ErrInvalidID
	}
	endpoint := fmt.Sprintf("%s/repos/%s/contents/%s/%s.json", g.api, g.repo, MenuDir, id)

	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var sha string
		sha, err = g.currentSHA(ctx, endpoint)
		if err != nil {
			return err
		}
		if sha == blobSHA(content) {
			return nil
		}
		err = g.put(ctx, endpoint, id, sha, content)
		// 409 means the branch moved between our read and write (for
		// example two publishes racing); re-read the sha and try once more.
		var se *statusError
		if !errors.As(err, &se) || se.code != http.StatusConflict {
			return err
		}
	}
	return err
}

// Delete removes static/menus/<id>.json in one commit. It returns
// ErrNotFound if the file does not exist.
func (g *GitHub) Delete(ctx context.Context, id string) error {
	if !menuid.Valid(id) {
		return ErrInvalidID
	}
	endpoint := fmt.Sprintf("%s/repos/%s/contents/%s/%s.json", g.api, g.repo, MenuDir, id)

	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var sha string
		sha, err = g.currentSHA(ctx, endpoint)
		if err != nil {
			return err
		}
		if sha == "" {
			return ErrNotFound
		}
		var payload []byte
		payload, err = json.Marshal(struct {
			Message string `json:"message"`
			Branch  string `json:"branch"`
			SHA     string `json:"sha"`
		}{Message: "menu: delete " + id, Branch: g.branch, SHA: sha})
		if err != nil {
			return fmt.Errorf("github: encode request: %w", err)
		}
		_, err = g.do(ctx, http.MethodDelete, endpoint, payload)
		// As in Publish: a 409 means the branch moved; re-read and retry once.
		var se *statusError
		if !errors.As(err, &se) || se.code != http.StatusConflict {
			return err
		}
	}
	return err
}

// currentSHA returns the blob sha of the existing file, or "" if there is
// none yet.
func (g *GitHub) currentSHA(ctx context.Context, endpoint string) (string, error) {
	u := endpoint + "?ref=" + url.QueryEscape(g.branch)
	body, err := g.do(ctx, http.MethodGet, u, nil)
	var se *statusError
	if errors.As(err, &se) && se.code == http.StatusNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var file struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &file); err != nil {
		return "", fmt.Errorf("github: unexpected response reading menu file: %w", err)
	}
	if file.Type != "file" || file.SHA == "" {
		return "", errors.New("github: menu path exists but is not a regular file")
	}
	return file.SHA, nil
}

func (g *GitHub) put(ctx context.Context, endpoint, id, sha string, content []byte) error {
	payload, err := json.Marshal(struct {
		Message string `json:"message"`
		Content string `json:"content"`
		Branch  string `json:"branch"`
		SHA     string `json:"sha,omitempty"`
	}{
		// The commit message is public: it carries the opaque id only.
		Message: "menu: update " + id,
		Content: base64.StdEncoding.EncodeToString(content),
		Branch:  g.branch,
		SHA:     sha,
	})
	if err != nil {
		return fmt.Errorf("github: encode request: %w", err)
	}
	_, err = g.do(ctx, http.MethodPut, endpoint, payload)
	return err
}

func (g *GitHub) do(ctx context.Context, method, u string, body []byte) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: %s menu file: %w", method, err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing useful to do with a close error on a read body

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("github: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &statusError{method: method, code: resp.StatusCode, message: apiMessage(data)}
	}
	return data, nil
}

// statusError is a non-2xx API response.
type statusError struct {
	method  string
	code    int
	message string
}

func (e *statusError) Error() string {
	hint := ""
	switch e.code {
	case http.StatusUnauthorized:
		hint = " (check GITHUB_TOKEN)"
	case http.StatusForbidden, http.StatusNotFound:
		hint = " (check GITHUB_REPO, GITHUB_BRANCH and that the token has Contents read/write on the repository)"
	}
	return fmt.Sprintf("github: %s menu file: HTTP %d: %s%s", e.method, e.code, e.message, hint)
}

// apiMessage extracts the "message" field of a GitHub error body, bounded so
// that an unexpected response cannot flood the logs.
func apiMessage(body []byte) string {
	var v struct {
		Message string `json:"message"`
	}
	msg := "no error message"
	if json.Unmarshal(body, &v) == nil && v.Message != "" {
		msg = v.Message
	}
	const maxLen = 200
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "..."
	}
	return msg
}

// blobSHA returns the git blob id of content, which is what the Contents API
// reports as "sha".
func blobSHA(content []byte) string {
	header := fmt.Sprintf("blob %d\x00", len(content))
	sum := sha1.Sum(append([]byte(header), content...)) //nolint:gosec // see import
	return hex.EncodeToString(sum[:])
}
