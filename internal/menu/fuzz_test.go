package menu

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"
)

// FuzzParse feeds Parse arbitrary bytes, as any Telegram user can. Whatever
// the input, Parse must not panic, must explain a rejection as a
// ValidationError, and must accept only menus whose published form is
// stable: encoding an accepted menu and parsing that again gives the same
// menu and the same bytes.
func FuzzParse(f *testing.F) {
	seeds := []string{
		templateList,
		"CATEGORY: A\nITEM: x\nPRICE: 1",
		"category: A\r\n- item: x\r\n  Price: 1\r\n  description: d",
		"CATEGORY: Салаты\nCATEGORY: Салаты",
		"CATEGORY: A\n- ITEM: x\n  PRICE: NaN",
		"CATEGORY: A\n- ITEM: x\n  PRICE: 1e400",
		`{"categories":[{"id":"a","name":"A","description":"d","items":[{"name":"x","description":"d","price":1.5,"image":"/img/x.jpg"}]}]}`,
		`{"categories":[{"id":"a","name":"A","items":[{"name":"<b>&</b>","price":0.1,"image":"https://e.com/x.jpg"}]}]}`,
		`{"categories":[{"id":"a","name":"A","items":null}]} {}`,
		`[]`,
		"",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	if tmpl, err := os.ReadFile("../bot/template.json"); err == nil {
		f.Add(tmpl)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		m, err := Parse(input)
		if err != nil {
			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("Parse() error is not a *ValidationError: %T: %v", err, err)
			}
			if len(invalid.Issues) == 0 {
				t.Fatal("Parse() rejected the menu without naming a problem")
			}
			return
		}

		published, err := m.Encode()
		if err != nil {
			t.Fatalf("Encode() of an accepted menu: %v", err)
		}
		again, err := Parse(published)
		if err != nil {
			t.Fatalf("the published form of an accepted menu is rejected: %v\n%s", err, published)
		}
		if !reflect.DeepEqual(m, again) {
			t.Fatalf("the menu changed when parsed from its published form:\n%s", published)
		}
		republished, err := again.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(published, republished) {
			t.Fatalf("publishing twice gives different bytes:\n%s\n%s", published, republished)
		}
	})
}
