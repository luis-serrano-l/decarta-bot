package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	goodSecret = "0123456789abcdef0123456789abcdef"
	tgToken    = "0000000000:TEST_telegram-token_not-real"
	ghToken    = "test-github-token-not-real"
)

func baseEnv() map[string]string {
	return map[string]string{
		"TELEGRAM_BOT_TOKEN": tgToken,
		"HASH_SECRET":        goodSecret,
		"GITHUB_TOKEN":       ghToken,
		"GITHUB_REPO":        "owner/site",
		"HUGO_BASE_URL":      "https://example.github.io/decarta-site/",
	}
}

func load(env map[string]string) (*Config, error) {
	return Load(func(k string) string { return env[k] })
}

func TestLoadDefaults(t *testing.T) {
	c, err := load(baseEnv())
	if err != nil {
		t.Fatal(err)
	}
	want := &Config{
		TelegramToken:  tgToken,
		HashSecret:     goodSecret,
		GitHubToken:    ghToken,
		GitHubRepo:     "owner/site",
		GitHubBranch:   "main",
		SiteBaseURL:    "https://example.github.io/decarta-site",
		StoragePath:    "./data",
		Port:           8080,
		MaxUploadBytes: 256 * 1024,
		PublishLimit:   5,
		PublishWindow:  time.Hour,
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("got  %+v\nwant %+v", c, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	env := baseEnv()
	env["GITHUB_BRANCH"] = "menus"
	env["STORAGE_PATH"] = "/data"
	env["PORT"] = "9090"
	env["MAX_UPLOAD_BYTES"] = "1024"
	env["PUBLISH_RATE_LIMIT"] = "2"
	env["PUBLISH_RATE_WINDOW"] = "30m"
	env["ALLOWED_USER_IDS"] = "1, 2 ,,3"
	env["ADMIN_USER_IDS"] = "9"
	env["HUGO_BASE_URL"] = "http://localhost:1313"

	c, err := load(env)
	if err != nil {
		t.Fatal(err)
	}
	if c.GitHubBranch != "menus" || c.StoragePath != "/data" || c.Port != 9090 ||
		c.MaxUploadBytes != 1024 || c.PublishLimit != 2 || c.PublishWindow != 30*time.Minute ||
		c.SiteBaseURL != "http://localhost:1313" {
		t.Errorf("overrides not applied: %+v", c)
	}
	if !reflect.DeepEqual(c.AllowedUserIDs, []int64{1, 2, 3}) || !reflect.DeepEqual(c.AdminUserIDs, []int64{9}) {
		t.Errorf("user ids: allowed=%v admin=%v", c.AllowedUserIDs, c.AdminUserIDs)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string // "" deletes the key
		wantErr string
	}{
		{"missing telegram token", "TELEGRAM_BOT_TOKEN", "", "TELEGRAM_BOT_TOKEN is required"},
		{"missing hash secret", "HASH_SECRET", "", "HASH_SECRET is required"},
		{"whitespace-only hash secret", "HASH_SECRET", "     ", "HASH_SECRET is required"},
		{"short hash secret", "HASH_SECRET", strings.Repeat("x", 31), "HASH_SECRET is too weak"},
		{"published default hash secret", "HASH_SECRET", "demenu-secret-key-change-in-production", "HASH_SECRET is too weak"},
		{"missing github token", "GITHUB_TOKEN", "", "GITHUB_TOKEN is required"},
		{"missing github repo", "GITHUB_REPO", "", "GITHUB_REPO is required"},
		{"missing base url", "HUGO_BASE_URL", "", "HUGO_BASE_URL: is required"},
		{"plain http base url", "HUGO_BASE_URL", "http://example.com", "must use https"},
		{"relative base url", "HUGO_BASE_URL", "/decarta-site", "absolute URL"},
		{"base url with credentials", "HUGO_BASE_URL", "https://u:p@example.com", "absolute URL"},
		{"base url with query", "HUGO_BASE_URL", "https://example.com/?x=1", "absolute URL"},
		{"non-numeric allowed id", "ALLOWED_USER_IDS", "1,abc", "ALLOWED_USER_IDS"},
		{"negative admin id", "ADMIN_USER_IDS", "-5", "ADMIN_USER_IDS"},
		{"bad port", "PORT", "http", "PORT"},
		{"port out of range", "PORT", "70000", "PORT"},
		{"zero upload cap", "MAX_UPLOAD_BYTES", "0", "MAX_UPLOAD_BYTES"},
		{"huge upload cap", "MAX_UPLOAD_BYTES", "999999999999", "MAX_UPLOAD_BYTES"},
		{"zero rate limit", "PUBLISH_RATE_LIMIT", "0", "PUBLISH_RATE_LIMIT"},
		{"bad window", "PUBLISH_RATE_WINDOW", "hourly", "PUBLISH_RATE_WINDOW"},
		{"negative window", "PUBLISH_RATE_WINDOW", "-1h", "PUBLISH_RATE_WINDOW"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := baseEnv()
			if tt.value == "" {
				delete(env, tt.key)
			} else {
				env[tt.key] = tt.value
			}
			c, err := load(env)
			if err == nil {
				t.Fatalf("Load() = %+v, want error containing %q", c, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			for _, secret := range []string{tgToken, ghToken, goodSecret, tt.value} {
				if tt.key == "HASH_SECRET" && secret != "" && len(secret) > 8 && strings.Contains(err.Error(), secret) {
					t.Errorf("error echoes a secret value: %q", err)
				}
			}
		})
	}
}

func TestLoadReportsAllProblemsWithoutSecrets(t *testing.T) {
	_, err := load(map[string]string{
		"TELEGRAM_BOT_TOKEN": tgToken,
		"GITHUB_TOKEN":       ghToken,
		"HASH_SECRET":        "too-short-secret",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"HASH_SECRET", "GITHUB_REPO", "HUGO_BASE_URL"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %s: %q", want, msg)
		}
	}
	for _, secret := range []string{tgToken, ghToken, "too-short-secret"} {
		if strings.Contains(msg, secret) {
			t.Errorf("error echoes a secret: %q", msg)
		}
	}
}

func TestSecrets(t *testing.T) {
	c, err := load(baseEnv())
	if err != nil {
		t.Fatal(err)
	}
	got := c.Secrets()
	for _, want := range []string{tgToken, ghToken, goodSecret} {
		found := false
		for _, s := range got {
			found = found || s == want
		}
		if !found {
			t.Errorf("Secrets() is missing a secret")
		}
	}
}
