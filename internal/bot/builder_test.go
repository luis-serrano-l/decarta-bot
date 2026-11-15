package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/luis-serrano-l/decarta-bot/internal/menu"
)

// draftOf returns the stored draft of the user, or nil.
func (f *fixture) draftOf(userID int64) *draft {
	f.t.Helper()
	raw, err := f.drafts.Load(f.ids.ID(userID))
	if err != nil {
		return nil
	}
	var d draft
	if err := json.Unmarshal(raw, &d); err != nil {
		f.t.Fatalf("stored draft is not JSON: %v", err)
	}
	return &d
}

// published returns what the user has online, parsed.
func (f *fixture) published(userID int64) *menu.Menu {
	f.t.Helper()
	raw, _ := f.pub.Get(f.ids.ID(userID))
	m, err := menu.Parse(raw)
	if err != nil {
		f.t.Fatalf("published content is not a menu: %v", err)
	}
	return m
}

// dishes lists "Section/Dish price" for every dish in the draft.
func dishes(d *draft) []string {
	var out []string
	if d == nil {
		return nil
	}
	for _, c := range d.Menu.Categories {
		for _, it := range c.Items {
			out = append(out, fmt.Sprintf("%s/%s %s", c.Name, it.Name, priceText(it)))
		}
	}
	return out
}

func TestBuilderFirstMenu(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, "/build")
	if got := f.tg.last(allowedID); !strings.Contains(got, "What is your first section called?") {
		t.Fatalf("opening message = %q", got)
	}
	if got := labels(f.tg.lastMsg(allowedID)); !slices.Equal(got, []string{"Starters", "Mains", "Desserts", "Drinks", "Cancel"}) {
		t.Fatalf("opening buttons = %q", got)
	}

	f.press(allowedID, "Starters")
	if got := f.tg.last(allowedID); !strings.Contains(got, "✅ Section Starters added.") || !strings.Contains(got, "Adding to Starters") {
		t.Fatalf("after the chip = %q", got)
	}

	f.send(allowedID, "Gazpacho 6.50\nCroquetas 8")
	if got := f.tg.last(allowedID); !strings.Contains(got, "✅ Added 2 dishes to Starters.") ||
		!strings.Contains(got, "• Gazpacho  6.50") || !strings.Contains(got, "📂 Starters (2)") {
		t.Fatalf("after two dishes = %q", got)
	}

	f.press(allowedID, "📝 Describe last dish")
	if got := f.tg.last(allowedID); !strings.Contains(got, "description for Croquetas") {
		t.Fatalf("description prompt = %q", got)
	}
	f.send(allowedID, "Ham, six pieces")

	f.press(allowedID, "📂 Add section")
	if got := labels(f.tg.lastMsg(allowedID)); !slices.Equal(got, []string{"Mains", "Desserts", "Drinks", "⬅️ Back"}) {
		t.Fatalf("second section buttons = %q", got)
	}
	f.send(allowedID, "  Rice dishes ")
	f.send(allowedID, "Paella 14,50 €")

	want := []string{"Starters/Gazpacho 6.50", "Starters/Croquetas 8.00", "Rice dishes/Paella 14.50"}
	d := f.draftOf(allowedID)
	if got := dishes(d); !slices.Equal(got, want) {
		t.Fatalf("draft = %q, want %q", got, want)
	}
	if got := d.Menu.Categories[0].Items[1].Description; got != "Ham, six pieces" {
		t.Errorf("description = %q", got)
	}
	if f.pub.Calls() != 0 {
		t.Fatal("building a menu published something")
	}

	f.press(allowedID, "✅ Done, preview")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Your menu is valid") || !strings.Contains(got, "Items: 3") {
		t.Fatalf("preview = %q", got)
	}
	f.press(allowedID, "Publish")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Menu published") {
		t.Fatalf("after Publish = %q", got)
	}
	if got := dishes(&draft{Menu: *f.published(allowedID)}); !slices.Equal(got, want) {
		t.Errorf("published = %q, want %q", got, want)
	}
	if f.draftOf(allowedID) != nil {
		t.Error("the draft was kept after publishing")
	}
	assertNoRawIDs(t, f.logs.String())
}

func TestBuilderInput(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*fixture) // after /build and the Starters chip
		text       string
		wantReply  []string
		wantDishes []string
	}{
		{"one dish", nil, "Soup 4.50", []string{"✅ Added 1 dish to Starters."}, []string{"Starters/Soup 4.50"}},
		{"bad line adds nothing and names the line", nil, "Soup 4.50\nBread",
			[]string{"Line 2: I could not find a price", `"Croquetas 8"`, "Nothing was changed. Send it again."}, nil},
		{"single bad line has no line number", nil, "Bread",
			[]string{"• I could not find a price"}, nil},
		{"the prompt stays open after a mistake", func(f *fixture) { f.send(allowedID, "Bread") }, "Bread 1",
			[]string{"✅ Added 1 dish"}, []string{"Starters/Bread 1.00"}},
		{"section name too long", func(f *fixture) { f.press(allowedID, "📂 Add section") }, strings.Repeat("a", 101),
			[]string{"name is too long", "Nothing was changed"}, nil},
		{"description too long", func(f *fixture) {
			f.send(allowedID, "Soup 4")
			f.press(allowedID, "📝 Describe last dish")
		}, strings.Repeat("a", 501), []string{"description is too long"}, []string{"Starters/Soup 4.00"}},
		{"too large", nil, strings.Repeat("a", 1025), []string{"too large", "1 KB"}, nil},
		{"a whole menu is still previewed", nil, validList, []string{"Your menu is valid"}, nil},
		{"after Back, text is not for the builder", func(f *fixture) { f.press(allowedID, "📂 Add section"); f.press(allowedID, "⬅️ Back") },
			"Soup 4.50", []string{"does not look like a menu"}, nil},
		{"after Keep for later, text is not for the builder", func(f *fixture) {
			f.press(allowedID, "Cancel")
			f.press(allowedID, "💾 Keep for later")
		}, "Soup 4.50", []string{"does not look like a menu"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.send(allowedID, "/build")
			f.press(allowedID, "Starters")
			if tt.setup != nil {
				tt.setup(f)
			}
			f.send(allowedID, tt.text)
			got := f.tg.last(allowedID)
			for _, want := range tt.wantReply {
				if !strings.Contains(got, want) {
					t.Errorf("reply = %q, want it to contain %q", got, want)
				}
			}
			if got := dishes(f.draftOf(allowedID)); !slices.Equal(got, tt.wantDishes) {
				t.Errorf("draft = %q, want %q", got, tt.wantDishes)
			}
			if f.pub.Calls() != 0 {
				t.Error("something was published")
			}
			assertNoRawIDs(t, f.logs.String())
		})
	}
}

func TestBuilderEditedMessageIsIgnored(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, "/build")
	f.press(allowedID, "Starters")
	f.send(allowedID, "Soup 4.50")
	u := textUpdate(allowedID, "Soup 4.50")
	f.h.Handle(context.Background(), tgbotapi.Update{EditedMessage: u.Message})
	if got := dishes(f.draftOf(allowedID)); len(got) != 1 {
		t.Errorf("draft = %q, want the dish once", got)
	}
}

func TestBuilderStartsFromPublishedMenu(t *testing.T) {
	f := newFixture(t)
	f.publish(allowedID)
	f.send(allowedID, "/build")
	if got := f.tg.last(allowedID); !strings.Contains(got, "the menu that is online now") || !strings.Contains(got, "• Soup  4.50") {
		t.Fatalf("panel = %q", got)
	}
	f.press(allowedID, "➕ Add dish")
	f.send(allowedID, "Salad 7")
	f.press(allowedID, "✅ Done, preview")
	if got := f.tg.last(allowedID); !strings.Contains(got, "It replaces the menu that is online now") {
		t.Fatalf("preview = %q", got)
	}
	f.press(allowedID, "Publish")
	if got, want := dishes(&draft{Menu: *f.published(allowedID)}), []string{"Starters/Soup 4.50", "Starters/Salad 7.00"}; !slices.Equal(got, want) {
		t.Errorf("published = %q, want %q", got, want)
	}
}

// A draft survives a restart: a new Handler on the same drafts store picks
// it up, until it is seven days old.
func TestBuilderResumeAndExpiry(t *testing.T) {
	tests := []struct {
		name       string
		idle       time.Duration
		wantReply  string
		wantDishes []string
	}{
		{"next day", 24 * time.Hour, "Here is the draft you were working on.", []string{"Starters/Soup 4.50"}},
		{"after a week", DraftTTL + time.Minute, "What is your first section called?", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.send(allowedID, "/build")
			f.press(allowedID, "Starters")
			f.send(allowedID, "Soup 4.50")

			restarted := newFixture(t)
			restarted.drafts.saved = f.drafts.saved
			restarted.now = f.now.Add(tt.idle)
			restarted.send(allowedID, "/build")
			if got := restarted.tg.last(allowedID); !strings.Contains(got, tt.wantReply) {
				t.Errorf("reply = %q, want it to contain %q", got, tt.wantReply)
			}
			if got := dishes(restarted.draftOf(allowedID)); !slices.Equal(got, tt.wantDishes) {
				t.Errorf("draft = %q, want %q", got, tt.wantDishes)
			}
		})
	}
}

func TestBuilderButtons(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, "/build")
	oldID, old, _ := f.tg.button(allowedID, "Starters")
	f.press(allowedID, "Starters")
	f.send(allowedID, "Soup 4.50")
	f.press(allowedID, "📂 Add section")
	f.press(allowedID, "Drinks")
	f.send(allowedID, "Water 2")

	// A button from an older message redraws instead of acting.
	f.pressData(allowedID, oldID, old.Data)
	if got := f.tg.last(allowedID); !strings.Contains(got, "out of date") {
		t.Errorf("stale button reply = %q", got)
	}
	if got := len(f.draftOf(allowedID).Menu.Categories); got != 2 {
		t.Fatalf("a stale button changed the draft: %d sections", got)
	}

	f.press(allowedID, "↔️ Change section")
	f.press(allowedID, "Starters (1)")
	f.send(allowedID, "Bread 1")
	if got, want := dishes(f.draftOf(allowedID)), []string{"Starters/Soup 4.50", "Starters/Bread 1.00", "Drinks/Water 2.00"}; !slices.Equal(got, want) {
		t.Fatalf("draft = %q, want %q", got, want)
	}

	// Malformed and unknown data does nothing.
	msgID, b, _ := f.tg.button(allowedID, "✅ Done, preview")
	rev, _, _ := strings.Cut(strings.TrimPrefix(b.Data, "b:"), ":")
	before := f.tg.count(allowedID)
	for _, data := range []string{"b:", "b:" + rev, "b:" + rev + ":nope", "b:" + rev + ":cur:x", "b:" + rev + ":cur:9", "b:" + rev + ":chip:9"} {
		f.pressData(allowedID, msgID, data)
	}
	if got := f.tg.count(allowedID); got != before {
		t.Errorf("bad button data produced %d messages", got-before)
	}

	// Keep editing from the preview goes back to the menu.
	f.press(allowedID, "✅ Done, preview")
	f.press(allowedID, "✏️ Keep editing")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Your menu (draft)") {
		t.Errorf("after Keep editing = %q", got)
	}

	f.press(allowedID, "Cancel")
	f.press(allowedID, "🗑 Discard draft")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Draft discarded") {
		t.Errorf("after Discard = %q", got)
	}
	if f.draftOf(allowedID) != nil {
		t.Error("the draft was not removed")
	}
	f.pressData(allowedID, msgID, b.Data)
	if got := f.tg.last(allowedID); !strings.Contains(got, "no longer exists") {
		t.Errorf("button without a draft = %q", got)
	}
	assertNoRawIDs(t, f.logs.String())
}

func TestBuilderLimitsAndAccess(t *testing.T) {
	t.Run("not a publisher", func(t *testing.T) {
		f := newFixture(t)
		f.send(strangerID, "/build")
		if got := f.tg.last(strangerID); !strings.Contains(got, "not authorised") {
			t.Errorf("reply = %q", got)
		}
		f.pressData(strangerID, 1, "b:1:chip:0")
		if f.draftOf(strangerID) != nil {
			t.Error("a stranger has a draft")
		}
	})
	t.Run("start button", func(t *testing.T) {
		f := newFixture(t)
		f.send(allowedID, "/start")
		f.press(allowedID, "🍽 Build my menu")
		if got := f.tg.last(allowedID); !strings.Contains(got, "first section") {
			t.Errorf("reply = %q", got)
		}
	})
	t.Run("section full", func(t *testing.T) {
		f := newFixture(t)
		f.h.opts.MaxUploadBytes = 1 << 20
		f.send(allowedID, "/build")
		f.press(allowedID, "Starters")
		var b strings.Builder
		for i := range menu.MaxItemsPerCategory {
			fmt.Fprintf(&b, "Dish %d 1\n", i)
		}
		f.send(allowedID, b.String())
		f.send(allowedID, "One more 1")
		if got := f.tg.last(allowedID); !strings.Contains(got, "can hold 0 more dishes") {
			t.Errorf("reply = %q", got)
		}
		if got := len(f.draftOf(allowedID).Menu.Categories[0].Items); got != menu.MaxItemsPerCategory {
			t.Errorf("section has %d dishes", got)
		}
	})
	t.Run("delete removes the draft", func(t *testing.T) {
		f := newFixture(t)
		f.publish(allowedID)
		f.send(allowedID, "/build")
		f.send(allowedID, "/delete")
		f.press(allowedID, "Yes, delete it")
		if f.draftOf(allowedID) != nil {
			t.Error("the draft outlived /delete")
		}
	})
}

// editFixture has a draft with two sections: Starters (Soup, Bread) and
// Drinks (Water), with the menu message showing.
func editFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.send(allowedID, "/build")
	f.press(allowedID, "Starters")
	f.send(allowedID, "Soup 4.50\nBread 1")
	f.press(allowedID, "📂 Add section")
	f.press(allowedID, "Drinks")
	f.send(allowedID, "Water 2")
	return f
}

func TestBuilderEdit(t *testing.T) {
	openSoup := func(f *fixture) {
		f.press(allowedID, "✏️ Edit / remove")
		f.press(allowedID, "Starters (2)")
		f.press(allowedID, "Soup  4.50")
	}
	tests := []struct {
		name       string
		steps      func(*fixture)
		wantReply  []string
		wantDishes []string
	}{
		{"change price", func(f *fixture) {
			openSoup(f)
			f.press(allowedID, "💶 Change price")
			f.send(allowedID, "5,25")
		}, []string{"✅ Soup now costs 5.25.", "📂 Starters (2 dishes)"},
			[]string{"Starters/Soup 5.25", "Starters/Bread 1.00", "Drinks/Water 2.00"}},
		{"bad price keeps the question open", func(f *fixture) {
			openSoup(f)
			f.press(allowedID, "💶 Change price")
			f.send(allowedID, "cheap")
			f.send(allowedID, "5")
		}, []string{"✅ Soup now costs 5.00."},
			[]string{"Starters/Soup 5.00", "Starters/Bread 1.00", "Drinks/Water 2.00"}},
		{"rename dish", func(f *fixture) {
			openSoup(f)
			f.press(allowedID, "✏️ Rename")
			f.send(allowedID, "Soup of the day")
		}, []string{"✅ Renamed to Soup of the day."},
			[]string{"Starters/Soup of the day 4.50", "Starters/Bread 1.00", "Drinks/Water 2.00"}},
		{"remove dish", func(f *fixture) {
			openSoup(f)
			f.press(allowedID, "🗑 Remove")
		}, []string{"🗑 Removed Soup.", "📂 Starters (1 dish)"},
			[]string{"Starters/Bread 1.00", "Drinks/Water 2.00"}},
		{"rename section", func(f *fixture) {
			f.press(allowedID, "✏️ Edit / remove")
			f.press(allowedID, "Drinks (1)")
			f.press(allowedID, "✏️ Rename section")
			f.send(allowedID, "Bebidas")
		}, []string{"✅ The section is now called Bebidas."},
			[]string{"Starters/Soup 4.50", "Starters/Bread 1.00", "Bebidas/Water 2.00"}},
		{"remove section asks first", func(f *fixture) {
			f.press(allowedID, "✏️ Edit / remove")
			f.press(allowedID, "Starters (2)")
			f.press(allowedID, "🗑 Remove section")
		}, []string{"Remove the section Starters and its 2 dishes"},
			[]string{"Starters/Soup 4.50", "Starters/Bread 1.00", "Drinks/Water 2.00"}},
		{"remove section", func(f *fixture) {
			f.press(allowedID, "✏️ Edit / remove")
			f.press(allowedID, "Starters (2)")
			f.press(allowedID, "🗑 Remove section")
			f.press(allowedID, "Yes, remove it")
		}, []string{"🗑 Removed the section Starters.", "📂 Drinks (1)"}, []string{"Drinks/Water 2.00"}},
		{"text while browsing is not a dish", func(f *fixture) {
			openSoup(f)
			f.send(allowedID, "Salad 7")
		}, []string{"does not look like a menu"},
			[]string{"Starters/Soup 4.50", "Starters/Bread 1.00", "Drinks/Water 2.00"}},
		{"back from a dish returns to the list", func(f *fixture) {
			openSoup(f)
			f.press(allowedID, "⬅️ Back")
			f.press(allowedID, "⬅️ Back to menu")
		}, []string{"Your menu (draft)", "What next?"},
			[]string{"Starters/Soup 4.50", "Starters/Bread 1.00", "Drinks/Water 2.00"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := editFixture(t)
			tt.steps(f)
			got := f.tg.last(allowedID)
			for _, want := range tt.wantReply {
				if !strings.Contains(got, want) {
					t.Errorf("reply = %q, want it to contain %q", got, want)
				}
			}
			if got := dishes(f.draftOf(allowedID)); !slices.Equal(got, tt.wantDishes) {
				t.Errorf("draft = %q, want %q", got, tt.wantDishes)
			}
			assertNoRawIDs(t, f.logs.String())
		})
	}
}

func TestBuilderEditDescription(t *testing.T) {
	f := editFixture(t)
	open := func() {
		f.press(allowedID, "✏️ Edit / remove")
		f.press(allowedID, "Drinks (1)")
		f.press(allowedID, "Water  2.00")
		f.press(allowedID, "📝 Description")
	}
	open()
	if got := labels(f.tg.lastMsg(allowedID)); !slices.Equal(got, []string{"⬅️ Back"}) {
		t.Fatalf("buttons without a description = %q", got)
	}
	f.send(allowedID, "Still, 50 cl")
	if got := f.draftOf(allowedID).Menu.Categories[1].Items[0].Description; got != "Still, 50 cl" {
		t.Fatalf("description = %q", got)
	}
	f.press(allowedID, "Water  2.00")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Still, 50 cl") {
		t.Errorf("dish screen = %q", got)
	}
	f.press(allowedID, "📝 Description")
	f.press(allowedID, "🗑 Remove the description")
	if got := f.draftOf(allowedID).Menu.Categories[1].Items[0].Description; got != "" {
		t.Errorf("description after removal = %q", got)
	}
}

func TestBuilderRemoveLastSectionAndPaging(t *testing.T) {
	f := newFixture(t)
	f.send(allowedID, "/build")
	f.press(allowedID, "Starters")
	var b strings.Builder
	for i := 1; i <= 19; i++ {
		fmt.Fprintf(&b, "Dish %02d 1\n", i)
	}
	f.send(allowedID, b.String())

	// One section: Edit / remove goes straight to its dishes.
	f.press(allowedID, "✏️ Edit / remove")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Page 1 of 3") {
		t.Fatalf("list = %q", got)
	}
	f.press(allowedID, "Next ▶️")
	f.press(allowedID, "Next ▶️")
	if got := labels(f.tg.lastMsg(allowedID)); !slices.Equal(got[:4], []string{"Dish 17  1.00", "Dish 18  1.00", "Dish 19  1.00", "◀️ Previous"}) {
		t.Fatalf("last page buttons = %q", got)
	}
	f.press(allowedID, "Dish 17  1.00")
	f.press(allowedID, "⬅️ Back")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Page 3 of 3") {
		t.Errorf("back from a dish = %q", got)
	}

	f.press(allowedID, "🗑 Remove section")
	f.press(allowedID, "Yes, remove it")
	if got := f.tg.last(allowedID); !strings.Contains(got, "What is your first section called?") {
		t.Errorf("after removing the only section = %q", got)
	}
	f.press(allowedID, "Mains")
	f.press(allowedID, "✅ Done, preview")
	if got := f.tg.last(allowedID); !strings.Contains(got, "Mains (0 items)") {
		t.Errorf("preview = %q", got)
	}
}
