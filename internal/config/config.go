// Package config loads and validates the bot's configuration from
// environment variables. Invalid or missing security-relevant settings are
// fatal: the bot refuses to start rather than fall back to a weak default.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/luis-serrano-l/demenu-bot/internal/menuid"
)

// Defaults for optional settings.
const (
	DefaultGitHubBranch   = "main"
	DefaultStoragePath    = "./data"
	DefaultPort           = 8080
	DefaultMaxUploadBytes = 256 * 1024
	DefaultPublishLimit   = 5
	DefaultPublishWindow  = time.Hour

	// maxUploadCeiling is Telegram's own download limit for bots (20 MB);
	// a larger cap could never take effect.
	maxUploadCeiling = 20 * 1024 * 1024
)

// Config is the validated configuration.
type Config struct {
	TelegramToken string // TELEGRAM_BOT_TOKEN
	HashSecret    string // HASH_SECRET

	GitHubToken  string // GITHUB_TOKEN
	GitHubRepo   string // GITHUB_REPO, "owner/name"
	GitHubBranch string // GITHUB_BRANCH

	SiteBaseURL string // HUGO_BASE_URL, without trailing slash

	AllowedUserIDs []int64 // ALLOWED_USER_IDS: may publish
	AdminUserIDs   []int64 // ADMIN_USER_IDS: may publish, receive access requests

	StoragePath    string        // STORAGE_PATH
	Port           int           // PORT
	MaxUploadBytes int64         // MAX_UPLOAD_BYTES
	PublishLimit   int           // PUBLISH_RATE_LIMIT
	PublishWindow  time.Duration // PUBLISH_RATE_WINDOW
}

// Secrets returns the values that must never appear in logs.
func (c *Config) Secrets() []string {
	return []string{c.TelegramToken, c.GitHubToken, c.HashSecret}
}

// Load reads configuration through getenv (normally os.Getenv). It returns
// every problem found, joined, so a misconfigured deployment can be fixed in
// one pass. Error messages name variables but never include secret values.
func Load(getenv func(string) string) (*Config, error) {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	get := func(key string) string { return strings.TrimSpace(getenv(key)) }

	c := &Config{
		TelegramToken: get("TELEGRAM_BOT_TOKEN"),
		HashSecret:    get("HASH_SECRET"),
		GitHubToken:   get("GITHUB_TOKEN"),
		GitHubRepo:    get("GITHUB_REPO"),
		GitHubBranch:  orDefault(get("GITHUB_BRANCH"), DefaultGitHubBranch),
		StoragePath:   orDefault(get("STORAGE_PATH"), DefaultStoragePath),
	}

	if c.TelegramToken == "" {
		fail("TELEGRAM_BOT_TOKEN is required")
	}
	if c.HashSecret == "" {
		fail("HASH_SECRET is required (generate one with: openssl rand -hex 32)")
	} else if _, err := menuid.New(c.HashSecret); err != nil {
		fail("HASH_SECRET is too weak: it must be at least %d characters and not a published default (generate one with: openssl rand -hex 32)", menuid.MinSecretLen)
	}
	if c.GitHubToken == "" {
		fail("GITHUB_TOKEN is required")
	}
	if c.GitHubRepo == "" {
		fail(`GITHUB_REPO is required (for example "luis-serrano-l/demenu")`)
	}

	var err error
	if c.SiteBaseURL, err = parseBaseURL(get("HUGO_BASE_URL")); err != nil {
		fail("HUGO_BASE_URL: %w", err)
	}
	if c.AllowedUserIDs, err = parseUserIDs(get("ALLOWED_USER_IDS")); err != nil {
		fail("ALLOWED_USER_IDS: %w", err)
	}
	if c.AdminUserIDs, err = parseUserIDs(get("ADMIN_USER_IDS")); err != nil {
		fail("ADMIN_USER_IDS: %w", err)
	}

	port, err := parseInt(get("PORT"), DefaultPort, 1, 65535)
	if err != nil {
		fail("PORT: %w", err)
	}
	c.Port = int(port)

	if c.MaxUploadBytes, err = parseInt(get("MAX_UPLOAD_BYTES"), DefaultMaxUploadBytes, 1, maxUploadCeiling); err != nil {
		fail("MAX_UPLOAD_BYTES: %w", err)
	}

	limit, err := parseInt(get("PUBLISH_RATE_LIMIT"), DefaultPublishLimit, 1, 1000)
	if err != nil {
		fail("PUBLISH_RATE_LIMIT: %w", err)
	}
	c.PublishLimit = int(limit)

	c.PublishWindow = DefaultPublishWindow
	if v := get("PUBLISH_RATE_WINDOW"); v != "" {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil:
			fail(`PUBLISH_RATE_WINDOW: must be a duration such as "1h" or "30m"`)
		case d < time.Second || d > 30*24*time.Hour:
			fail("PUBLISH_RATE_WINDOW: must be between 1s and 720h")
		default:
			c.PublishWindow = d
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// parseBaseURL requires an absolute https URL (http is tolerated only for
// localhost, for developing against `hugo server`).
func parseBaseURL(v string) (string, error) {
	if v == "" {
		return "", errors.New("is required (the public URL of the menu site)")
	}
	u, err := url.Parse(v)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("must be an absolute URL without credentials, query or fragment")
	}
	host := u.Hostname()
	local := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return "", errors.New("must use https (http is accepted for localhost only)")
	}
	return strings.TrimRight(v, "/"), nil
}

// parseUserIDs parses a comma-separated list of positive Telegram user IDs.
// Unlike earlier versions it does not skip malformed entries: silently
// dropping an ID from an access list is how lock-outs and surprises happen.
func parseUserIDs(v string) ([]int64, error) {
	if v == "" {
		return nil, nil
	}
	var ids []int64
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("must be a comma-separated list of positive numeric Telegram user IDs")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func parseInt(v string, def, lo, hi int64) (int64, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("must be an integer between %d and %d", lo, hi)
	}
	return n, nil
}
