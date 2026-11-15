package menu

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// This file holds what the step-by-step builder needs: parsing the short
// things a person types into a chat (a price, a line per dish, a name) and
// growing a menu one piece at a time. The results still go through Validate
// before anything is published.

var (
	plainPriceRe = regexp.MustCompile(`^\d+(?:[.,]\d{1,2})?$`)
	// Groups of three digits are thousands, as in "12.000" or "1,250.50";
	// no menu prices a dish to a thousandth.
	dotGroupsRe   = regexp.MustCompile(`^[1-9]\d{0,2}(?:\.\d{3})+(?:,\d{1,2})?$`)
	commaGroupsRe = regexp.MustCompile(`^[1-9]\d{0,2}(?:,\d{3})+(?:\.\d{1,2})?$`)
)

const (
	currencySigns = "€$£¥"
	priceFix      = `write only the number, for example 9.50`
)

// ParsePrice reads a price as a person types it: "9", "9.50", "9,50",
// "12.000", with an optional currency sign before or after. The error is a
// *ValidationError with one issue at path "price".
func ParsePrice(s string) (float64, error) {
	p, is := parsePrice(s)
	if is != nil {
		return 0, &ValidationError{Issues: []Issue{*is}, Total: 1}
	}
	return p, nil
}

func parsePrice(s string) (float64, *Issue) {
	bad := func(problem string) (float64, *Issue) {
		return 0, &Issue{Path: "price", Problem: problem, Fix: priceFix}
	}
	s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), currencySigns))
	var plain string
	switch {
	case s == "":
		return bad("the price is missing")
	case plainPriceRe.MatchString(s):
		plain = strings.Replace(s, ",", ".", 1)
	case dotGroupsRe.MatchString(s):
		plain = strings.Replace(strings.ReplaceAll(s, ".", ""), ",", ".", 1)
	case commaGroupsRe.MatchString(s):
		plain = strings.ReplaceAll(s, ",", "")
	default:
		return bad(fmt.Sprintf("%q is not a price", clip(s)))
	}
	p, err := strconv.ParseFloat(plain, 64)
	if err != nil || p > MaxPrice {
		return bad(fmt.Sprintf("the price must be between 0 and %d", MaxPrice))
	}
	return p, nil
}

// ParseDishLines reads one dish per line, each a name followed by its price:
//
//	Gazpacho 6.50
//	Croquetas - 8
//
// Blank lines are ignored, and a leading "-" or "•" and any punctuation
// between the name and the price are dropped. Problems are reported by line
// number, all at once.
func ParseDishLines(text string) ([]Item, error) {
	var (
		items []Item
		v     issues
	)
	for n, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(raw), "-•*"))
		if line == "" {
			continue
		}
		where := fmt.Sprintf("line %d", n+1)
		if len(items) >= MaxItemsPerCategory {
			v.add(where, fmt.Sprintf("too many dishes in one message (max %d)", MaxItemsPerCategory),
				"send them in smaller groups")
			break
		}
		name, price, ok := splitDish(line)
		if !ok {
			v.add(where, "I could not find a price",
				`write the dish, a space, then the price, for example "Croquetas 8"`)
			continue
		}
		p, is := parsePrice(price)
		if is != nil {
			v.add(where, is.Problem, is.Fix)
			continue
		}
		if is := textIssue("name", name, MaxNameLen); is != nil {
			v.add(where, is.Problem, is.Fix)
			continue
		}
		items = append(items, Item{Name: name, Price: &p})
	}
	if err := v.err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, single("line 1", "there is no dish here",
			`write the dish, a space, then the price, for example "Croquetas 8"`)
	}
	return items, nil
}

// splitDish cuts a line into the dish name and the price text at its end.
func splitDish(line string) (name, price string, ok bool) {
	// A sign after the number ("8 €") would otherwise be the last word.
	line = strings.TrimSpace(strings.TrimRight(line, currencySigns+" "))
	i := strings.LastIndexAny(line, " \t")
	if i < 0 {
		return "", "", false
	}
	price = line[i+1:]
	if strings.Trim(price, currencySigns) == "" || !strings.ContainsAny(price, "0123456789") {
		return "", "", false
	}
	name = strings.TrimSpace(strings.TrimRight(line[:i], " \t-–—:,;.·…"))
	if name == "" {
		return "", "", false
	}
	return name, price, true
}

// CheckName reports why s cannot be the name of a category or a dish, or
// nil. The error is a *ValidationError with one issue at path "name".
func CheckName(s string) error {
	if is := textIssue("name", s, MaxNameLen); is != nil {
		return &ValidationError{Issues: []Issue{*is}, Total: 1}
	}
	return nil
}

// CheckDescription reports why s cannot be a description, or nil. The error
// is a *ValidationError with one issue at path "description".
func CheckDescription(s string) error {
	if is := textIssue("description", s, MaxDescriptionLen); is != nil {
		return &ValidationError{Issues: []Issue{*is}, Total: 1}
	}
	return nil
}

// textIssue is the first problem checkText finds in a required text field,
// with the field name as its path.
func textIssue(field, s string, maxLen int) *Issue {
	var v issues
	checkText(&v, "", field, s, true, maxLen)
	if v.total == 0 {
		return nil
	}
	is := v.list[0]
	is.Path = field
	if s == "" {
		is.Problem, is.Fix = "the "+field+" is empty", "type some text"
	}
	return &is
}

// AddCategory appends an empty category called name, deriving an id that is
// unique in m, and returns its index.
func (m *Menu) AddCategory(name string) (int, error) {
	if err := CheckName(name); err != nil {
		return 0, err
	}
	if len(m.Categories) >= MaxCategories {
		return 0, single("categories", fmt.Sprintf("the menu already has %d sections, the most it can have", MaxCategories),
			"merge or remove a section first")
	}
	used := make(map[string]struct{}, len(m.Categories))
	for _, c := range m.Categories {
		used[c.ID] = struct{}{}
	}
	m.Categories = append(m.Categories, Category{ID: uniqueSlug(name, used), Name: name, Items: []Item{}})
	return len(m.Categories) - 1, nil
}

// AddItems appends items to the category at index cat.
func (m *Menu) AddItems(cat int, items []Item) error {
	if cat < 0 || cat >= len(m.Categories) {
		return single("categories", "that section no longer exists", "choose another section")
	}
	c := &m.Categories[cat]
	if room := MaxItemsPerCategory - len(c.Items); len(items) > room {
		return single("items", fmt.Sprintf("%q can hold %d more dishes (max %d)", clip(c.Name), room, MaxItemsPerCategory),
			"put the rest in a new section")
	}
	c.Items = append(c.Items, items...)
	return nil
}
