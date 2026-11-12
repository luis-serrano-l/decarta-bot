package menu

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// issuesOf parses in and returns the issues as "path|problem" strings.
func issuesOf(t *testing.T, in string) []string {
	t.Helper()
	_, err := Parse([]byte(in))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Parse() error = %v, want a *ValidationError", err)
	}
	var out []string
	for _, is := range ve.Issues {
		if is.Fix == "" {
			t.Errorf("issue at %s has no fix", is.Path)
		}
		out = append(out, is.Path+"|"+is.Problem)
	}
	return out
}

func TestIssuesNameExactPaths(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			"every problem is reported, in document order",
			`{"categories":[
				{"id":"a","name":"A","items":[{"name":"x","price":1},{"name":"","price":"2 EUR","colour":"red"}]},
				{"id":"Main Dishes","name":"Main Dishes","items":"none"}],
			  "owner":1}`,
			[]string{
				`owner|unknown field "owner"`,
				`categories[0].items[1].colour|unknown field "colour"`,
				`categories[0].items[1].price|price must be a number, not text`,
				`categories[1].items|must be a list, not text`,
			},
		},
		{
			"validation issues after a clean decode",
			`{"categories":[{"id":"Main Dishes","name":"Main Dishes","items":[{"name":"x"},{"name":"y","price":-1,"image":"http://e.com/y.jpg"}]},{"id":"b","name":"B"}]}`,
			[]string{
				"categories[0].id|id must be 1-64 characters of a-z, 0-9 and single hyphens",
				"categories[0].items[0].price|missing price",
				"categories[0].items[1].price|price must be a number between 0 and 1000000",
				`categories[0].items[1].image|image must be an https URL or a site-relative path starting with "/"`,
				"categories[1].items|missing items",
			},
		},
		{"category is not an object", `{"categories":["Starters"]}`, []string{"categories[0]|must be an object, not text"}},
		{"item is not an object", `{"categories":[{"id":"a","name":"A","items":[7]}]}`, []string{"categories[0].items[0]|must be an object, not a number"}},
		{"syntax error has a line and column", "{\n  \"categories\": [\n    {\"id\": \"a\" \"name\": \"A\"}\n  ]\n}",
			[]string{"line 3, column 16|invalid JSON: invalid character '\"' after object key:value pair"}},
		{"truncated document", "{\n\"categories\": [", []string{"line 2|invalid JSON: the document ends too early"}},
		{"list problems carry line numbers", "ITEM: orphan\nCATEGORY: A\n- ITEM: x\n- ITEM: y\n  PRICE: 1,5\nhello",
			[]string{
				"line 1|ITEM must come after CATEGORY",
				`line 3|ITEM "x" has no PRICE`,
				"line 5|PRICE must be a number like 12.99",
				"line 6|expected CATEGORY, ITEM, PRICE or DESCRIPTION",
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := issuesOf(t, tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("issues =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(tt.want, "\n  "))
			}
		})
	}
}

func TestIssueFixSuggestsAnID(t *testing.T) {
	m := valid()
	m.Categories[0].ID, m.Categories[0].Name = "Main Dishes!", "Main Dishes"
	var ve *ValidationError
	if err := m.Validate(); !errors.As(err, &ve) {
		t.Fatalf("Validate() = %v", err)
	}
	if got := ve.Issues[0].Fix; !strings.Contains(got, `"main-dishes"`) {
		t.Errorf("fix = %q, want it to suggest \"main-dishes\"", got)
	}
}

func TestIssuesAreCapped(t *testing.T) {
	in := `{"categories":[{"id":"a","name":"A","items":[` + strings.Repeat(`{"name":"x"},`, 150) + `{"name":"x"}]}]}`
	_, err := Parse([]byte(in))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(ve.Issues) != maxIssues || ve.Total != 151 {
		t.Errorf("kept %d of %d issues, want %d of 151", len(ve.Issues), ve.Total, maxIssues)
	}
	if !strings.HasSuffix(ve.Error(), "and 101 more") {
		t.Errorf("Error() does not mention the remainder: ...%s", ve.Error()[len(ve.Error())-40:])
	}
}

func TestOversizedListsAreNotWalked(t *testing.T) {
	in := `{"categories":[{"id":"a","name":"A","items":[` + strings.Repeat(`7,`, MaxItemsPerCategory+10) + `7]}]}`
	_, err := Parse([]byte(in))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Parse() error = %v", err)
	}
	if ve.Total != MaxItemsPerCategory {
		t.Errorf("found %d issues, want one per walked item (%d)", ve.Total, MaxItemsPerCategory)
	}
}

func TestSummary(t *testing.T) {
	m, err := Parse([]byte(templateList))
	if err != nil {
		t.Fatal(err)
	}
	want := Summary{
		Categories: []CategorySummary{{"Appetizers", 2}, {"Salads", 1}},
		Items:      3, MinPrice: 9.99, MaxPrice: 14.99,
	}
	if got := m.Summary(); !reflect.DeepEqual(got, want) {
		t.Errorf("Summary() = %+v, want %+v", got, want)
	}
	empty := &Menu{Categories: []Category{{ID: "a", Name: "A", Items: []Item{}}}}
	if got := empty.Summary(); got.Items != 0 || len(got.Categories) != 1 {
		t.Errorf("Summary() of an empty category = %+v", got)
	}
}
