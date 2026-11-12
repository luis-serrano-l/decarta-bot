package menu

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func price(p float64) *float64 { return &p }

// valid returns a minimal valid menu that each test case then breaks.
func valid() *Menu {
	return &Menu{Categories: []Category{{
		ID:   "starters",
		Name: "Starters",
		Items: []Item{{
			Name:  "Bruschetta",
			Price: price(12.99),
		}},
	}}}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Menu)
		wantErr string // substring; empty means valid
	}{
		{"minimal", func(*Menu) {}, ""},
		{"empty category is allowed", func(m *Menu) { m.Categories[0].Items = []Item{} }, ""},
		{"zero price", func(m *Menu) { m.Categories[0].Items[0].Price = price(0) }, ""},
		{"unicode text", func(m *Menu) { m.Categories[0].Items[0].Name = "Crème brûlée 🍮" }, ""},
		{"name at limit", func(m *Menu) { m.Categories[0].Name = strings.Repeat("é", MaxNameLen) }, ""},
		{"description at limit", func(m *Menu) { m.Categories[0].Items[0].Description = strings.Repeat("d", MaxDescriptionLen) }, ""},
		{"categories at limit", func(m *Menu) { m.Categories = manyCategories(MaxCategories) }, ""},
		{"items at limit", func(m *Menu) { m.Categories[0].Items = manyItems(MaxItemsPerCategory) }, ""},

		{"no categories", func(m *Menu) { m.Categories = nil }, "at least one category"},
		{"too many categories", func(m *Menu) { m.Categories = manyCategories(MaxCategories + 1) }, "too many categories"},
		{"too many items", func(m *Menu) { m.Categories[0].Items = manyItems(MaxItemsPerCategory + 1) }, "too many items"},
		{"nil items", func(m *Menu) { m.Categories[0].Items = nil }, "categories[0].items: missing items"},

		{"empty id", func(m *Menu) { m.Categories[0].ID = "" }, "categories[0].id: missing id"},
		{"uppercase id", func(m *Menu) { m.Categories[0].ID = "Starters" }, "id must be"},
		{"id with space", func(m *Menu) { m.Categories[0].ID = "main courses" }, "id must be"},
		{"id with markup", func(m *Menu) { m.Categories[0].ID = `"><script>` }, "id must be"},
		{"id leading hyphen", func(m *Menu) { m.Categories[0].ID = "-a" }, "id must be"},
		{"id too long", func(m *Menu) { m.Categories[0].ID = strings.Repeat("a", MaxIDLen+1) }, "id must be"},
		{"duplicate id", func(m *Menu) { m.Categories = append(m.Categories, m.Categories[0]) }, "duplicate id"},

		{"missing category name", func(m *Menu) { m.Categories[0].Name = "" }, "missing name"},
		{"category name too long", func(m *Menu) { m.Categories[0].Name = strings.Repeat("a", MaxNameLen+1) }, "name is too long"},
		{"category description too long", func(m *Menu) { m.Categories[0].Description = strings.Repeat("a", MaxDescriptionLen+1) }, "description is too long"},
		{"missing item name", func(m *Menu) { m.Categories[0].Items[0].Name = "" }, "categories[0].items[0].name: missing name"},
		{"item name too long", func(m *Menu) { m.Categories[0].Items[0].Name = strings.Repeat("a", MaxNameLen+1) }, "name is too long"},
		{"item description too long", func(m *Menu) { m.Categories[0].Items[0].Description = strings.Repeat("a", MaxDescriptionLen+1) }, "description is too long"},
		{"newline in name", func(m *Menu) { m.Categories[0].Items[0].Name = "a\nb" }, "control character"},
		{"NUL in description", func(m *Menu) { m.Categories[0].Items[0].Description = "a\x00b" }, "control character"},
		{"bidi override", func(m *Menu) { m.Categories[0].Items[0].Name = "a" + string(rune(0x202E)) + "b" }, "control character"},
		{"invalid utf-8", func(m *Menu) { m.Categories[0].Items[0].Name = "a\xffb" }, "not valid UTF-8"},
		{"padded name", func(m *Menu) { m.Categories[0].Items[0].Name = " a" }, "whitespace"},

		{"missing price", func(m *Menu) { m.Categories[0].Items[0].Price = nil }, "categories[0].items[0].price: missing price"},
		{"negative price", func(m *Menu) { m.Categories[0].Items[0].Price = price(-1) }, "price must be"},
		{"huge price", func(m *Menu) { m.Categories[0].Items[0].Price = price(MaxPrice + 1) }, "price must be"},
		{"NaN price", func(m *Menu) { m.Categories[0].Items[0].Price = price(math.NaN()) }, "price must be"},
		{"Inf price", func(m *Menu) { m.Categories[0].Items[0].Price = price(math.Inf(1)) }, "price must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid()
			tt.mutate(m)
			checkErr(t, m.Validate(), tt.wantErr)
		})
	}
}

func TestValidateNil(t *testing.T) {
	var m *Menu
	checkErr(t, m.Validate(), "at least one category")
}

func TestCheckImage(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"", false},
		{"https://example.com/a.jpg", false},
		{"https://cdn.example.com:8443/a%20b.jpg?w=200#x", false},
		{"/images/a.jpg", false},
		{"/a", false},

		{"http://example.com/a.jpg", true},
		{"HTTP://example.com/a.jpg", true},
		{"//example.com/a.jpg", true},
		{"///example.com/a.jpg", true},
		{"ftp://example.com/a.jpg", true},
		{"javascript:alert(1)", true},
		{"JaVaScRiPt:alert(1)", true},
		{"data:image/png;base64,AAAA", true},
		{"file:///etc/passwd", true},
		{"https:example.com/a.jpg", true},
		{"https:///a.jpg", true},
		{"https://user:pass@example.com/a.jpg", true},
		{"https://user@example.com/a.jpg", true},
		{"images/a.jpg", true},
		{"./a.jpg", true},
		{"../a.jpg", true},
		{"/images/../../secret", true},
		{"/..", true},
		{`/\example.com/a.jpg`, true},
		{`\\example.com\a.jpg`, true},
		{"/images/a b.jpg", true},
		{" https://example.com/a.jpg", true},
		{"https://example.com/a.jpg\n", true},
		{"java\tscript:alert(1)", true},
		{"https://exämple.com/a.jpg", true},
		{"https://example.com/" + strings.Repeat("a", MaxImageLen), true},
		{"https://%zz", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if err := checkImage(tt.in); (err != nil) != tt.wantErr {
				t.Errorf("checkImage(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestEncode(t *testing.T) {
	m := valid()
	m.Categories[0].Items[0].Description = "Classic"
	got, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "categories": [
    {
      "id": "starters",
      "name": "Starters",
      "items": [
        {
          "name": "Bruschetta",
          "description": "Classic",
          "price": 12.99
        }
      ]
    }
  ]
}
`
	if string(got) != want {
		t.Errorf("Encode() =\n%s\nwant\n%s", got, want)
	}
	// The canonical form must survive a strict round trip.
	if _, err := Parse(got); err != nil {
		t.Errorf("round trip: %v", err)
	}
}

func TestEncodeEscapesHTML(t *testing.T) {
	m := valid()
	m.Categories[0].Items[0].Description = "<script>alert(1)</script> & more"
	got, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(string(got), "<>&") {
		t.Errorf("Encode() left HTML metacharacters unescaped:\n%s", got)
	}
	back, err := Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if back.Categories[0].Items[0].Description != m.Categories[0].Items[0].Description {
		t.Errorf("round trip changed the description: %q", back.Categories[0].Items[0].Description)
	}
}

func manyCategories(n int) []Category {
	out := make([]Category, n)
	for i := range out {
		out[i] = Category{ID: fmt.Sprintf("c%d", i), Name: "C", Items: []Item{}}
	}
	return out
}

func manyItems(n int) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{Name: "I", Price: price(1)}
	}
	return out
}

func checkErr(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case want != "" && err == nil:
		t.Fatalf("expected error containing %q, got nil", want)
	case want != "" && !strings.Contains(err.Error(), want):
		t.Fatalf("error = %q, want it to contain %q", err, want)
	}
}
