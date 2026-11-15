package menu

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParsePrice(t *testing.T) {
	tests := []struct {
		in      string
		want    float64
		wantErr string
	}{
		{"9", 9, ""},
		{"9.5", 9.5, ""},
		{"9.50", 9.5, ""},
		{"9,50", 9.5, ""},
		{" 6.50 ", 6.5, ""},
		{"€6.50", 6.5, ""},
		{"6,50€", 6.5, ""},
		{"6.50 €", 6.5, ""},
		{"$ 12", 12, ""},
		{"0", 0, ""},
		{"12.000", 12000, ""},
		{"9.505", 9505, ""}, // three digits after the mark are thousands
		{"1,250", 1250, ""},
		{"1.250,50", 1250.5, ""},
		{"1,250.50", 1250.5, ""},
		{"1000000", 1000000, ""},

		{"", 0, "missing"},
		{"€", 0, "missing"},
		{"free", 0, "not a price"},
		{"-5", 0, "not a price"},
		{"NaN", 0, "not a price"},
		{"1e3", 0, "not a price"},
		{"9.5055", 0, "not a price"},
		{"0.500", 0, "not a price"},
		{"1.2.3", 0, "not a price"},
		{"1000001", 0, "between 0 and 1000000"},
		{strings.Repeat("9", 400), 0, "between 0 and 1000000"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParsePrice(tt.in)
			if tt.wantErr == "" {
				if err != nil || got != tt.want {
					t.Fatalf("ParsePrice(%q) = %v, %v; want %v, nil", tt.in, got, err, tt.want)
				}
				return
			}
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Issues[0].Path != "price" ||
				!strings.Contains(invalid.Issues[0].Problem, tt.wantErr) || invalid.Issues[0].Fix == "" {
				t.Fatalf("ParsePrice(%q) error = %v, want a price issue containing %q", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestParseDishLines(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []Item
		wantErr []string // "path: problem" substrings, in order
	}{
		{"one dish", "Gazpacho 6.50", []Item{{Name: "Gazpacho", Price: price(6.5)}}, nil},
		{"name with spaces", "Patatas bravas 5", []Item{{Name: "Patatas bravas", Price: price(5)}}, nil},
		{"several lines", "Gazpacho 6.50\n\n  Croquetas 8\r\n", []Item{
			{Name: "Gazpacho", Price: price(6.5)}, {Name: "Croquetas", Price: price(8)},
		}, nil},
		{"comma and sign", "Paella 14,50 €", []Item{{Name: "Paella", Price: price(14.5)}}, nil},
		{"sign before", "Paella €14.50", []Item{{Name: "Paella", Price: price(14.5)}}, nil},
		{"separators", "- Flan: 4\n• Tarta - 5\nCafé ..... 1.50", []Item{
			{Name: "Flan", Price: price(4)}, {Name: "Tarta", Price: price(5)}, {Name: "Café", Price: price(1.5)},
		}, nil},
		{"number in name", "Menú 2 personas 30", []Item{{Name: "Menú 2 personas", Price: price(30)}}, nil},

		{"empty", " \n ", nil, []string{"line 1: there is no dish"}},
		{"no price", "Gazpacho", nil, []string{"line 1: I could not find a price"}},
		{"only a price", "6.50", nil, []string{"line 1: I could not find a price"}},
		{"price first", "6.50 Gazpacho", nil, []string{"line 1: I could not find a price"}},
		{"bad price", "Gazpacho 6.5.0", nil, []string{`line 1: "6.5.0" is not a price`}},
		{"every bad line is reported", "Gazpacho 6\nCroquetas\nFlan 4\nTarta x1", nil,
			[]string{"line 2: I could not find a price", `line 4: "x1" is not a price`}},
		{"name too long", strings.Repeat("a", MaxNameLen+1) + " 5", nil, []string{"line 1: name is too long"}},
		{"control character", "Gaz" + string(rune(0x202E)) + "pacho 5", nil, []string{"line 1: name contains a control character"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDishLines(tt.in)
			if tt.wantErr == nil {
				if err != nil || !reflect.DeepEqual(got, tt.want) {
					t.Fatalf("got %+v, %v\nwant %+v", got, err, tt.want)
				}
				return
			}
			var invalid *ValidationError
			if !errors.As(err, &invalid) || len(invalid.Issues) != len(tt.wantErr) {
				t.Fatalf("error = %v, want %d issues", err, len(tt.wantErr))
			}
			for i, want := range tt.wantErr {
				is := invalid.Issues[i]
				if line := is.Path + ": " + is.Problem; !strings.Contains(line, want) || is.Fix == "" {
					t.Errorf("issue %d = %q (fix %q), want it to contain %q", i, line, is.Fix, want)
				}
			}
		})
	}
}

func TestParseDishLinesTooMany(t *testing.T) {
	var b strings.Builder
	for i := range MaxItemsPerCategory + 1 {
		fmt.Fprintf(&b, "Dish %d 1\n", i)
	}
	if _, err := ParseDishLines(b.String()); err == nil || !strings.Contains(err.Error(), "too many dishes") {
		t.Errorf("error = %v, want too many dishes", err)
	}
}

func TestCheckNameAndDescription(t *testing.T) {
	tests := []struct {
		name    string
		check   func(string) error
		in      string
		wantErr string
	}{
		{"name ok", CheckName, "Starters", ""},
		{"name empty", CheckName, "", "name: the name is empty"},
		{"name too long", CheckName, strings.Repeat("a", MaxNameLen+1), "name: name is too long"},
		{"name with newline", CheckName, "a\nb", "name: name contains a control character"},
		{"description ok", CheckDescription, "Cold tomato soup", ""},
		{"description empty", CheckDescription, "", "description: the description is empty"},
		{"description too long", CheckDescription, strings.Repeat("a", MaxDescriptionLen+1), "description: description is too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.check(tt.in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestAddCategoryAndItems(t *testing.T) {
	var m Menu
	for i, tt := range []struct{ name, wantID string }{
		{"Starters", "starters"}, {"Starters", "starters-1"}, {"Постное", "category"},
	} {
		idx, err := m.AddCategory(tt.name)
		if err != nil || idx != i || m.Categories[idx].ID != tt.wantID || m.Categories[idx].Items == nil {
			t.Fatalf("AddCategory(%q) = %d, %v; menu %+v", tt.name, idx, err, m)
		}
	}
	if _, err := m.AddCategory(""); err == nil {
		t.Error("AddCategory(\"\") = nil error")
	}

	if err := m.AddItems(0, []Item{{Name: "Gazpacho", Price: price(6.5)}}); err != nil {
		t.Fatal(err)
	}
	if err := m.AddItems(7, []Item{{Name: "x", Price: price(1)}}); err == nil {
		t.Error("AddItems to a missing section = nil error")
	}
	if err := m.AddItems(0, make([]Item, MaxItemsPerCategory)); err == nil || !strings.Contains(err.Error(), "199 more") {
		t.Errorf("AddItems past the limit: error = %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("built menu does not validate: %v", err)
	}

	for len(m.Categories) < MaxCategories {
		if _, err := m.AddCategory("More"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.AddCategory("One too many"); err == nil || !strings.Contains(err.Error(), "50 sections") {
		t.Errorf("AddCategory past the limit: error = %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("full menu does not validate: %v", err)
	}
}
