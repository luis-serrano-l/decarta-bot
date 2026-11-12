// Package menu defines the published menu document, parses the two input
// formats the bot accepts and validates the result.
//
// Everything a user sends ends up, verbatim, in a public git repository and on
// a public web page, so validation is strict: unknown fields are rejected,
// every string and list is bounded, and image URLs are restricted to https or
// site-relative paths.
package menu

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits on a menu document. Lengths are counted in runes.
const (
	MaxCategories       = 50
	MaxItemsPerCategory = 200
	MaxIDLen            = 64
	MaxNameLen          = 100
	MaxDescriptionLen   = 500
	MaxImageLen         = 2048
	MaxPrice            = 1_000_000
)

// Menu is the document published as static/menus/<id>.json.
type Menu struct {
	Categories []Category `json:"categories"`
}

// Category groups items under a heading. ID is a slug used by the site for
// filtering.
type Category struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Items       []Item `json:"items"`
}

// Item is a single dish. Price is a pointer so that a missing price can be
// told apart from a price of zero.
type Item struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Price       *float64 `json:"price"`
	Image       string   `json:"image,omitempty"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Validate checks m against every structural and size rule. The returned
// error names the offending element and is safe to show to the user.
func (m *Menu) Validate() error {
	if m == nil || len(m.Categories) == 0 {
		return fmt.Errorf("menu must have at least one category")
	}
	if len(m.Categories) > MaxCategories {
		return fmt.Errorf("too many categories: %d (max %d)", len(m.Categories), MaxCategories)
	}
	seen := make(map[string]struct{}, len(m.Categories))
	for i, c := range m.Categories {
		where := fmt.Sprintf("category %d", i+1)
		if utf8.RuneCountInString(c.ID) > MaxIDLen || !slugRe.MatchString(c.ID) {
			return fmt.Errorf("%s: id must be 1-%d characters of a-z, 0-9 and single hyphens", where, MaxIDLen)
		}
		if _, dup := seen[c.ID]; dup {
			return fmt.Errorf("%s: duplicate id %q", where, c.ID)
		}
		seen[c.ID] = struct{}{}
		if err := checkText(c.Name, "name", true, MaxNameLen); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if err := checkText(c.Description, "description", false, MaxDescriptionLen); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if c.Items == nil {
			return fmt.Errorf("%s: missing items", where)
		}
		if len(c.Items) > MaxItemsPerCategory {
			return fmt.Errorf("%s: too many items: %d (max %d)", where, len(c.Items), MaxItemsPerCategory)
		}
		for j, it := range c.Items {
			if err := it.validate(); err != nil {
				return fmt.Errorf("%s, item %d: %w", where, j+1, err)
			}
		}
	}
	return nil
}

func (it Item) validate() error {
	if err := checkText(it.Name, "name", true, MaxNameLen); err != nil {
		return err
	}
	if err := checkText(it.Description, "description", false, MaxDescriptionLen); err != nil {
		return err
	}
	if it.Price == nil {
		return fmt.Errorf("missing price")
	}
	if p := *it.Price; math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > MaxPrice {
		return fmt.Errorf("price must be a number between 0 and %d", MaxPrice)
	}
	return checkImage(it.Image)
}

// checkText enforces length, valid UTF-8 and the absence of control and
// formatting-override characters.
func checkText(s, field string, required bool, maxLen int) error {
	if s == "" {
		if required {
			return fmt.Errorf("missing %s", field)
		}
		return nil
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s is not valid UTF-8", field)
	}
	if n := utf8.RuneCountInString(s); n > maxLen {
		return fmt.Errorf("%s is too long: %d characters (max %d)", field, n, maxLen)
	}
	if strings.TrimSpace(s) != s {
		return fmt.Errorf("%s must not start or end with whitespace", field)
	}
	for _, r := range s {
		if unicode.IsControl(r) || isBidiOverride(r) {
			return fmt.Errorf("%s contains a control character (U+%04X)", field, r)
		}
	}
	return nil
}

// isBidiOverride reports direction-override characters, which can make text
// render differently from what a reviewer of the published JSON sees.
func isBidiOverride(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

// checkImage allows an empty value, an absolute https URL, or a site-relative
// path ("/images/x.jpg"). Everything else is rejected, including http:,
// data:, javascript:, protocol-relative ("//host/x") and path traversal.
func checkImage(s string) error {
	if s == "" {
		return nil
	}
	if utf8.RuneCountInString(s) > MaxImageLen {
		return fmt.Errorf("image is too long (max %d characters)", MaxImageLen)
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f || r == '\\' || r > unicode.MaxASCII {
			return fmt.Errorf("image must be a plain ASCII URL without spaces or backslashes")
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("image is not a valid URL")
	}
	switch {
	case u.Scheme == "https":
		if u.Hostname() == "" || u.User != nil || u.Opaque != "" {
			return fmt.Errorf("image must be an https URL with a host and no credentials")
		}
	case u.Scheme == "" && u.Host == "" && strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//"):
		for _, seg := range strings.Split(u.Path, "/") {
			if seg == ".." {
				return fmt.Errorf("image path must not contain \"..\"")
			}
		}
	default:
		return fmt.Errorf("image must be an https URL or a site-relative path starting with \"/\"")
	}
	return nil
}

// Encode renders m as the canonical, indented JSON that gets published.
// encoding/json escapes <, > and & so the output is inert if ever embedded
// in HTML.
func (m *Menu) Encode() ([]byte, error) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode menu: %w", err)
	}
	return append(b, '\n'), nil
}
