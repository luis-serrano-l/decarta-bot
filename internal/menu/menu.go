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

// Validate checks m against every structural and size rule. It returns a
// *ValidationError listing every problem found, each with the path of the
// offending field and a way to fix it.
func (m *Menu) Validate() error {
	var v issues
	switch {
	case m == nil || len(m.Categories) == 0:
		v.add("categories", "the menu must have at least one category",
			`add a category with "id", "name" and "items"`)
		return v.err()
	case len(m.Categories) > MaxCategories:
		v.add("categories", fmt.Sprintf("too many categories: %d (max %d)", len(m.Categories), MaxCategories),
			"merge or remove categories")
		return v.err()
	}
	seen := make(map[string]struct{}, len(m.Categories))
	for i, c := range m.Categories {
		p := fmt.Sprintf("categories[%d]", i)
		_, dup := seen[c.ID]
		switch {
		case c.ID == "":
			v.add(p+".id", "missing id", `add a short id such as "starters"`)
		case utf8.RuneCountInString(c.ID) > MaxIDLen || !slugRe.MatchString(c.ID):
			v.add(p+".id", fmt.Sprintf("id must be 1-%d characters of a-z, 0-9 and single hyphens", MaxIDLen),
				fmt.Sprintf("use something like %q", slug(c.Name, c.ID)))
		case dup:
			v.add(p+".id", fmt.Sprintf("duplicate id %q", c.ID), "give every category a different id")
		}
		seen[c.ID] = struct{}{}
		checkText(&v, p, "name", c.Name, true, MaxNameLen)
		checkText(&v, p, "description", c.Description, false, MaxDescriptionLen)
		switch {
		case c.Items == nil:
			v.add(p+".items", "missing items", `add "items": [ ... ]; the list may be empty`)
		case len(c.Items) > MaxItemsPerCategory:
			v.add(p+".items", fmt.Sprintf("too many items: %d (max %d)", len(c.Items), MaxItemsPerCategory),
				"split the category in two")
		}
		for j, it := range c.Items {
			if j >= MaxItemsPerCategory {
				break
			}
			it.validate(&v, fmt.Sprintf("%s.items[%d]", p, j))
		}
	}
	return v.err()
}

func (it Item) validate(v *issues, p string) {
	checkText(v, p, "name", it.Name, true, MaxNameLen)
	checkText(v, p, "description", it.Description, false, MaxDescriptionLen)
	switch {
	case it.Price == nil:
		v.add(p+".price", "missing price", `add a number, for example "price": 9.5`)
	case math.IsNaN(*it.Price) || math.IsInf(*it.Price, 0) || *it.Price < 0 || *it.Price > MaxPrice:
		v.add(p+".price", fmt.Sprintf("price must be a number between 0 and %d", MaxPrice),
			"use a plain number such as 9.5, without a currency symbol")
	}
	if err := checkImage(it.Image); err != nil {
		v.add(p+".image", err.Error(),
			`use an https:// URL or a path such as "/images/dish.jpg", or remove the field`)
	}
}

// checkText enforces length, valid UTF-8 and the absence of control and
// formatting-override characters on the text field p.field.
func checkText(v *issues, p, field, s string, required bool, maxLen int) {
	path := p + "." + field
	if s == "" {
		if required {
			v.add(path, "missing "+field, fmt.Sprintf("add a non-empty %q", field))
		}
		return
	}
	if !utf8.ValidString(s) {
		v.add(path, field+" is not valid UTF-8", "save the file with UTF-8 encoding and send it again")
		return
	}
	if n := utf8.RuneCountInString(s); n > maxLen {
		v.add(path, fmt.Sprintf("%s is too long: %d characters (max %d)", field, n, maxLen),
			fmt.Sprintf("shorten it to %d characters or fewer", maxLen))
	}
	if strings.TrimSpace(s) != s {
		v.add(path, field+" must not start or end with whitespace", "remove the spaces at the start and end")
	}
	for _, r := range s {
		if unicode.IsControl(r) || isBidiOverride(r) {
			v.add(path, fmt.Sprintf("%s contains a control character (U+%04X)", field, r),
				"retype the text on one line, without invisible characters")
			return
		}
	}
}

// slug suggests a valid id from a category name, falling back to the
// rejected id itself.
func slug(name, id string) string {
	for _, src := range []string{name, id} {
		s := strings.Trim(nonSlugRe.ReplaceAllString(strings.ToLower(src), "-"), "-")
		if len(s) > MaxIDLen {
			s = strings.Trim(s[:MaxIDLen], "-")
		}
		if s != "" {
			return s
		}
	}
	return "category"
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

// Summary is what a person needs to recognise a menu before publishing it.
type Summary struct {
	Categories []CategorySummary
	Items      int
	// MinPrice and MaxPrice are meaningful only when Items > 0.
	MinPrice, MaxPrice float64
}

// CategorySummary is one category's name and size.
type CategorySummary struct {
	Name  string
	Items int
}

// Summary describes a validated menu.
func (m *Menu) Summary() Summary {
	var s Summary
	for _, c := range m.Categories {
		s.Categories = append(s.Categories, CategorySummary{Name: c.Name, Items: len(c.Items)})
		for _, it := range c.Items {
			if it.Price == nil {
				continue
			}
			if s.Items == 0 || *it.Price < s.MinPrice {
				s.MinPrice = *it.Price
			}
			if s.Items == 0 || *it.Price > s.MaxPrice {
				s.MaxPrice = *it.Price
			}
			s.Items++
		}
	}
	return s
}
