package menu

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Parse decodes input in either accepted format (JSON if it starts with "{",
// otherwise the list format) and validates it.
func Parse(input []byte) (*Menu, error) {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 {
		return nil, errors.New("empty menu")
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

// ParseJSON decodes a menu document, rejecting unknown fields and trailing
// data. It does not validate the result.
func ParseJSON(data []byte) (*Menu, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var m Menu
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid JSON: unexpected data after the menu object")
	}
	return &m, nil
}

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// ParseList decodes the line-oriented format shown by /template:
//
//	CATEGORY: Appetizers
//	DESCRIPTION: Start your meal
//
//	- ITEM: Bruschetta
//	  PRICE: 12.99
//	  DESCRIPTION: Classic tomato basil
//
// Blank lines are ignored and the leading "- " is optional. Category ids are
// derived from the names. It does not validate the result.
func ParseList(text string) (*Menu, error) {
	var (
		m      Menu
		cat    *Category // current category, nil before the first CATEGORY
		item   *Item     // current item, nil right after a CATEGORY
		usedID = map[string]struct{}{}
	)
	for n, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		key, value, ok := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		if !ok {
			return nil, fmt.Errorf("line %d: expected CATEGORY, ITEM, PRICE or DESCRIPTION", n+1)
		}
		switch strings.TrimSpace(key) {
		case "CATEGORY":
			if value == "" {
				return nil, fmt.Errorf("line %d: CATEGORY name is required", n+1)
			}
			if len(m.Categories) >= MaxCategories {
				return nil, fmt.Errorf("line %d: too many categories (max %d)", n+1, MaxCategories)
			}
			m.Categories = append(m.Categories, Category{
				ID:    uniqueSlug(value, usedID),
				Name:  value,
				Items: []Item{},
			})
			cat, item = &m.Categories[len(m.Categories)-1], nil
		case "ITEM":
			if cat == nil {
				return nil, fmt.Errorf("line %d: ITEM must come after CATEGORY", n+1)
			}
			if value == "" {
				return nil, fmt.Errorf("line %d: ITEM name is required", n+1)
			}
			if len(cat.Items) >= MaxItemsPerCategory {
				return nil, fmt.Errorf("line %d: too many items in category (max %d)", n+1, MaxItemsPerCategory)
			}
			cat.Items = append(cat.Items, Item{Name: value})
			item = &cat.Items[len(cat.Items)-1]
		case "PRICE":
			if item == nil {
				return nil, fmt.Errorf("line %d: PRICE must come after ITEM", n+1)
			}
			p, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return nil, fmt.Errorf("line %d: PRICE must be a number like 12.99", n+1)
			}
			item.Price = &p
		case "DESCRIPTION":
			switch {
			case item != nil:
				item.Description = value
			case cat != nil:
				cat.Description = value
			default:
				return nil, fmt.Errorf("line %d: DESCRIPTION must come after CATEGORY or ITEM", n+1)
			}
		default:
			return nil, fmt.Errorf("line %d: expected CATEGORY, ITEM, PRICE or DESCRIPTION", n+1)
		}
	}
	if len(m.Categories) == 0 {
		return nil, errors.New("no categories found")
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
