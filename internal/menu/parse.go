package menu

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Parse decodes input in either accepted format (JSON if it starts with "{",
// otherwise the list format) and validates it.
func Parse(input []byte) (*Menu, error) {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 {
		return nil, single("(top level)", "empty menu", "send a menu; /template has an example")
	}
	var (
		m   *Menu
		err error
	)
	if trimmed[0] == '{' || trimmed[0] == '[' {
		m, err = ParseJSON(trimmed)
	} else {
		m, err = ParseList(string(trimmed))
	}
	if err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// ParseJSON decodes a menu document, rejecting unknown fields, wrong types
// and trailing data, and reporting each with its exact field path. It does
// not validate the result.
//
// The document is decoded generically and then walked, rather than decoded
// straight into Menu, because encoding/json reports neither array indices
// nor more than one problem.
func ParseJSON(data []byte) (*Menu, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, syntaxError(data, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		line, col := position(data, dec.InputOffset())
		return nil, single(fmt.Sprintf("line %d, column %d", line, col),
			"invalid JSON: unexpected data after the menu object",
			"delete everything after the final }")
	}

	var v issues
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, single("(top level)", "invalid JSON: the menu must be an object, not "+kind(doc),
			`wrap it as {"categories": [ ... ]}`)
	}
	unknown(&v, "", root, "categories")
	var m Menu
	if list, ok := array(&v, "categories", root["categories"]); ok {
		m.Categories = make([]Category, 0, min(len(list), MaxCategories))
		for i, raw := range list {
			if i >= MaxCategories {
				break // Validate reports the count
			}
			m.Categories = append(m.Categories, category(&v, fmt.Sprintf("categories[%d]", i), raw))
		}
		if len(list) > MaxCategories { // keep the true count for Validate without walking the excess
			m.Categories = append(m.Categories, make([]Category, len(list)-MaxCategories)...)
		}
	}
	if err := v.err(); err != nil {
		return nil, err
	}
	return &m, nil
}

func category(v *issues, p string, raw any) Category {
	obj, ok := raw.(map[string]any)
	if !ok {
		v.add(p, "must be an object, not "+kind(raw), `write it as { "id": ..., "name": ..., "items": [ ... ] }`)
		return Category{}
	}
	unknown(v, p, obj, "id", "name", "description", "items")
	c := Category{
		ID:          text(v, p+".id", obj["id"]),
		Name:        text(v, p+".name", obj["name"]),
		Description: text(v, p+".description", obj["description"]),
	}
	if list, ok := array(v, p+".items", obj["items"]); ok {
		c.Items = make([]Item, 0, min(len(list), MaxItemsPerCategory))
		for j, raw := range list {
			if j >= MaxItemsPerCategory {
				break
			}
			c.Items = append(c.Items, item(v, fmt.Sprintf("%s.items[%d]", p, j), raw))
		}
		if len(list) > MaxItemsPerCategory {
			c.Items = append(c.Items, make([]Item, len(list)-MaxItemsPerCategory)...)
		}
	}
	return c
}

func item(v *issues, p string, raw any) Item {
	obj, ok := raw.(map[string]any)
	if !ok {
		v.add(p, "must be an object, not "+kind(raw), `write it as { "name": ..., "price": ... }`)
		return Item{}
	}
	unknown(v, p, obj, "name", "description", "price", "image")
	it := Item{
		Name:        text(v, p+".name", obj["name"]),
		Description: text(v, p+".description", obj["description"]),
		Image:       text(v, p+".image", obj["image"]),
	}
	switch n := obj["price"].(type) {
	case nil: // absent or null: Validate reports the missing price
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			f = math.Inf(1) // out of float64 range: Validate rejects it
		}
		it.Price = &f
	default:
		v.add(p+".price", "price must be a number, not "+kind(n),
			"remove the quotes and any currency symbol, for example 9.5")
	}
	return it
}

// text returns raw as a string. A missing or null value is "".
func text(v *issues, path string, raw any) string {
	switch s := raw.(type) {
	case nil:
		return ""
	case string:
		return s
	default:
		v.add(path, "must be text, not "+kind(raw), `put it in double quotes, for example "Soup"`)
		return ""
	}
}

// array returns raw as a list. A missing or null value is not a list and is
// left for Validate to report.
func array(v *issues, path string, raw any) ([]any, bool) {
	switch list := raw.(type) {
	case nil:
		return nil, false
	case []any:
		return list, true
	default:
		v.add(path, "must be a list, not "+kind(raw), "put the entries between [ and ]")
		return nil, false
	}
}

// unknown reports every key of obj that is not allowed, in a stable order.
func unknown(v *issues, p string, obj map[string]any, allowed ...string) {
	var extra []string
	for k := range obj {
		if !slices.Contains(allowed, k) {
			extra = append(extra, k)
		}
	}
	slices.Sort(extra)
	for _, k := range extra {
		path := k
		if p != "" {
			path = p + "." + k
		}
		v.add(path, fmt.Sprintf("unknown field %q", clip(k)),
			`remove it; the fields allowed here are "`+strings.Join(allowed, `", "`)+`"`)
	}
}

func kind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "true/false"
	case string:
		return "text"
	case json.Number, float64:
		return "a number"
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	default:
		return "an unexpected value"
	}
}

// clip shortens user-supplied text that is echoed back in a message.
func clip(s string) string {
	const maxLen = 40
	if r := []rune(s); len(r) > maxLen {
		return string(r[:maxLen]) + "..."
	}
	return s
}

// syntaxError turns a decoding failure into an issue located by line and
// column.
func syntaxError(data []byte, err error) error {
	var se *json.SyntaxError
	switch {
	case errors.As(err, &se):
		line, col := position(data, se.Offset-1) // Offset counts the offending byte
		return single(fmt.Sprintf("line %d, column %d", line, col), "invalid JSON: "+se.Error(),
			"look for a missing or extra comma, quote or bracket just before that point")
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		line, _ := position(data, int64(len(data)))
		return single(fmt.Sprintf("line %d", line), "invalid JSON: the document ends too early",
			"close every { with } and every [ with ]; the file may have been cut off")
	default:
		return single("(top level)", "invalid JSON: "+err.Error(), "compare your file with the one from /template")
	}
}

// position converts a byte offset into a 1-based line and column.
func position(data []byte, offset int64) (line, col int) {
	offset = max(0, min(offset, int64(len(data))))
	before := data[:offset]
	line = bytes.Count(before, []byte{'\n'}) + 1
	col = len(before) - bytes.LastIndexByte(before, '\n')
	return line, col
}

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// ParseList decodes the line-oriented format:
//
//	CATEGORY: Appetizers
//	DESCRIPTION: Start your meal
//
//	- ITEM: Bruschetta
//	  PRICE: 12.99
//	  DESCRIPTION: Classic tomato basil
//
// Blank lines are ignored and the leading "- " is optional. Category ids are
// derived from the names. Problems are reported by line number. It does not
// validate the result beyond what only the list form can locate.
func ParseList(text string) (*Menu, error) {
	var (
		m        Menu
		v        issues
		cat      *Category // current category, nil before the first CATEGORY
		item     *Item     // current item, nil right after a CATEGORY
		itemLine int
		usedID   = map[string]struct{}{}
	)
	const expected = "expected CATEGORY, ITEM, PRICE or DESCRIPTION"
	const expectedFix = `start the line with one of those words and a colon, for example "ITEM: Soup"`
	// closeItem reports an item that ended without a price, at the line
	// where the item starts.
	closeItem := func() {
		if item != nil && item.Price == nil {
			v.add(fmt.Sprintf("line %d", itemLine), fmt.Sprintf("ITEM %q has no PRICE", clip(item.Name)),
				`add a line such as "PRICE: 9.50" under it`)
		}
		item = nil
	}
	for n, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		where := fmt.Sprintf("line %d", n+1)
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		key, value, ok := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		if !ok {
			v.add(where, expected, expectedFix)
			continue
		}
		switch strings.TrimSpace(key) {
		case "CATEGORY":
			closeItem()
			if value == "" {
				v.add(where, "CATEGORY name is required", `write the name after the colon, for example "CATEGORY: Starters"`)
				continue
			}
			if len(m.Categories) >= MaxCategories {
				v.add(where, fmt.Sprintf("too many categories (max %d)", MaxCategories), "merge or remove categories")
				return nil, v.err()
			}
			m.Categories = append(m.Categories, Category{
				ID:    uniqueSlug(value, usedID),
				Name:  value,
				Items: []Item{},
			})
			cat = &m.Categories[len(m.Categories)-1]
		case "ITEM":
			closeItem()
			if cat == nil {
				v.add(where, "ITEM must come after CATEGORY", `add a line such as "CATEGORY: Starters" above it`)
				continue
			}
			if value == "" {
				v.add(where, "ITEM name is required", `write the name after the colon, for example "ITEM: Soup"`)
				continue
			}
			if len(cat.Items) >= MaxItemsPerCategory {
				v.add(where, fmt.Sprintf("too many items in category (max %d)", MaxItemsPerCategory), "split the category in two")
				return nil, v.err()
			}
			cat.Items = append(cat.Items, Item{Name: value})
			item, itemLine = &cat.Items[len(cat.Items)-1], n+1
		case "PRICE":
			if item == nil {
				v.add(where, "PRICE must come after ITEM", `add a line such as "ITEM: Soup" above it`)
				continue
			}
			p, err := strconv.ParseFloat(value, 64)
			if err != nil {
				v.add(where, "PRICE must be a number like 12.99", "use a dot for decimals and no currency symbol")
				p = 0 // keep the item priced so it is not reported twice
			}
			item.Price = &p
		case "DESCRIPTION":
			switch {
			case item != nil:
				item.Description = value
			case cat != nil:
				cat.Description = value
			default:
				v.add(where, "DESCRIPTION must come after CATEGORY or ITEM", "move it under the line it describes")
			}
		default:
			v.add(where, expected, expectedFix)
		}
	}
	closeItem()
	if err := v.err(); err != nil {
		return nil, err
	}
	if len(m.Categories) == 0 {
		return nil, single("line 1", "no categories found", `start with a line such as "CATEGORY: Starters"`)
	}
	return &m, nil
}

// uniqueSlug turns a category name into an id that is unique within used,
// appending -1, -2, ... on collisions.
func uniqueSlug(name string, used map[string]struct{}) string {
	base := strings.Trim(nonSlugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "category"
	}
	const suffixRoom = 4 // "-" plus up to three digits; MaxCategories is 50
	if len(base) > MaxIDLen-suffixRoom {
		base = strings.Trim(base[:MaxIDLen-suffixRoom], "-")
	}
	id := base
	for i := 1; ; i++ {
		if _, taken := used[id]; !taken {
			break
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
	used[id] = struct{}{}
	return id
}
