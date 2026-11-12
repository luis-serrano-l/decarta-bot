package menu

import (
	"reflect"
	"strings"
	"testing"
)

const templateList = `CATEGORY: Appetizers
DESCRIPTION: Start your meal

- ITEM: Bruschetta
  PRICE: 12.99
  DESCRIPTION: Classic tomato basil

- ITEM: Calamari
  PRICE: 14.99

CATEGORY: Salads

- ITEM: Caesar Salad
  PRICE: 9.99
  DESCRIPTION: Crisp romaine
`

func TestParseList(t *testing.T) {
	m, err := Parse([]byte(templateList))
	if err != nil {
		t.Fatal(err)
	}
	want := &Menu{Categories: []Category{
		{ID: "appetizers", Name: "Appetizers", Description: "Start your meal", Items: []Item{
			{Name: "Bruschetta", Price: price(12.99), Description: "Classic tomato basil"},
			{Name: "Calamari", Price: price(14.99)},
		}},
		{ID: "salads", Name: "Salads", Items: []Item{
			{Name: "Caesar Salad", Price: price(9.99), Description: "Crisp romaine"},
		}},
	}}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got %+v\nwant %+v", m, want)
	}
}

func TestParseListCases(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantIDs []string
		wantErr string
	}{
		{"without dashes", "CATEGORY: A\nITEM: x\nPRICE: 1", []string{"a"}, ""},
		{"lowercase keywords", "category: A\n- item: x\n  Price: 1\n  description: d", []string{"a"}, ""},
		{"CRLF line endings", "CATEGORY: A\r\n- ITEM: x\r\n  PRICE: 1\r\n", []string{"a"}, ""},
		{"colon in value", "CATEGORY: Soups: hot\n- ITEM: x\n  PRICE: 1", []string{"soups-hot"}, ""},
		{"duplicate names get suffixed ids", "CATEGORY: A\nCATEGORY: A\nCATEGORY: a!", []string{"a", "a-1", "a-2"}, ""},
		{"non-latin name falls back", "CATEGORY: Салаты", []string{"category"}, ""},
		{"long name is truncated to a valid id", "CATEGORY: " + strings.Repeat("ab ", 30), []string{strings.TrimSuffix(strings.Repeat("ab-", 20), "-")}, ""},

		{"empty", "   \n ", nil, "empty menu"},
		{"item before category", "- ITEM: x", nil, "ITEM must come after CATEGORY"},
		{"price before item", "CATEGORY: A\nPRICE: 1", nil, "PRICE must come after ITEM"},
		{"description first", "DESCRIPTION: x", nil, "DESCRIPTION must come after"},
		{"empty category name", "CATEGORY:", nil, "CATEGORY name is required"},
		{"empty item name", "CATEGORY: A\n- ITEM:", nil, "ITEM name is required"},
		{"bad price", "CATEGORY: A\n- ITEM: x\n  PRICE: 12,99", nil, "PRICE must be a number"},
		{"NaN price", "CATEGORY: A\n- ITEM: x\n  PRICE: NaN", nil, "categories[0].items[0].price: price must be"},
		{"missing price", "CATEGORY: A\n- ITEM: x", nil, `line 2: ITEM "x" has no PRICE`},
		{"unknown key", "CATEGORY: A\nCOLOR: red", nil, "line 2: expected"},
		{"free text", "hello there", nil, "line 1: expected"},
		{"too many categories", strings.Repeat("CATEGORY: A\n", MaxCategories+1), nil, "too many categories"},
		{"too many items", "CATEGORY: A\n" + strings.Repeat("ITEM: x\nPRICE: 1\n", MaxItemsPerCategory+1), nil, "too many items"},
		{"name too long", "CATEGORY: " + strings.Repeat("a", MaxNameLen+1), nil, "name is too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Parse([]byte(tt.in))
			checkErr(t, err, tt.wantErr)
			if err != nil {
				return
			}
			var ids []string
			for _, c := range m.Categories {
				ids = append(ids, c.ID)
			}
			if !reflect.DeepEqual(ids, tt.wantIDs) {
				t.Errorf("ids = %q, want %q", ids, tt.wantIDs)
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	const item = `{"name":"x","price":1}`
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"minimal", `{"categories":[{"id":"a","name":"A","items":[` + item + `]}]}`, ""},
		{"all fields", `{"categories":[{"id":"a","name":"A","description":"d","items":[{"name":"x","description":"d","price":1.5,"image":"/img/x.jpg"}]}]}`, ""},
		{"leading whitespace", "\n  " + `{"categories":[{"id":"a","name":"A","items":[]}]}`, ""},

		{"unknown top-level field", `{"categories":[{"id":"a","name":"A","items":[]}],"owner":1}`, `unknown field "owner"`},
		{"unknown category field", `{"categories":[{"id":"a","name":"A","items":[],"html":"x"}]}`, `unknown field "html"`},
		{"unknown item field", `{"categories":[{"id":"a","name":"A","items":[{"name":"x","price":1,"onclick":"x"}]}]}`, `unknown field "onclick"`},
		{"legacy meals document", `{"meals":[]}`, `unknown field "meals"`},
		{"trailing object", `{"categories":[{"id":"a","name":"A","items":[]}]} {}`, "unexpected data"},
		{"trailing garbage", `{"categories":[{"id":"a","name":"A","items":[]}]} x`, "unexpected data"},
		{"array", `[]`, "invalid JSON"},
		{"truncated", `{"categories":[`, "invalid JSON"},
		{"price as string", `{"categories":[{"id":"a","name":"A","items":[{"name":"x","price":"1"}]}]}`, "categories[0].items[0].price: price must be a number, not text"},
		{"name as number", `{"categories":[{"id":"a","name":1,"items":[]}]}`, "categories[0].name: must be text, not a number"},
		{"price null", `{"categories":[{"id":"a","name":"A","items":[{"name":"x","price":null}]}]}`, "missing price"},
		{"items null", `{"categories":[{"id":"a","name":"A","items":null}]}`, "missing items"},
		{"items missing", `{"categories":[{"id":"a","name":"A"}]}`, "missing items"},
		{"empty object", `{}`, "at least one category"},
		{"http image", `{"categories":[{"id":"a","name":"A","items":[{"name":"x","price":1,"image":"http://e.com/x.jpg"}]}]}`, "image must be"},
		{"script image", `{"categories":[{"id":"a","name":"A","items":[{"name":"x","price":1,"image":"javascript:alert(1)"}]}]}`, "image must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.in))
			checkErr(t, err, tt.wantErr)
		})
	}
}
