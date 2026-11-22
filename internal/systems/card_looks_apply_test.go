package systems

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func pkgChange(field, key, border, colour string) CardLookChange {
	return CardLookChange{FieldKey: field, Source: ChoiceSourcePackage, Key: key, Border: border, Colour: colour}
}

func code(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

// Every item is checked before anything is written: one bad item refuses the
// whole save, the valid ones before it included.
func TestCardLookApply_RefusesHostileInput(t *testing.T) {
	long := strings.Repeat("a", MaxCardLookKeyLen+1)
	many := make([]CardLookChange, MaxCardLookChanges+1)
	for i := range many {
		many[i] = pkgChange("ancestry", "k"+strings.Repeat("x", i%50)+lookID(int64(i)), "fire", "")
	}
	tests := []struct {
		name string
		in   []CardLookChange
	}{
		{"css in the colour", []CardLookChange{pkgChange("ancestry", "devil", "fire", "red;background:url(x)")}},
		{"a colour with a bad digit", []CardLookChange{pkgChange("ancestry", "devil", "fire", "#12345g")}},
		{"a short colour", []CardLookChange{pkgChange("ancestry", "devil", "fire", "#fff")}},
		{"a border outside the library", []CardLookChange{pkgChange("ancestry", "devil", "not-a-border", "")}},
		{"a border that is css", []CardLookChange{pkgChange("ancestry", "devil", "fire;x", "")}},
		{"a field key that fails validation", []CardLookChange{pkgChange("../etc", "devil", "fire", "")}},
		{"an empty field key", []CardLookChange{pkgChange("", "devil", "fire", "")}},
		{"an unknown source", []CardLookChange{{FieldKey: "ancestry", Source: "other", Key: "devil", Border: "fire"}}},
		{"no border but a colour", []CardLookChange{pkgChange("ancestry", "devil", "", "#aabbcc")}},
		{"reset with a border", []CardLookChange{{FieldKey: "ancestry", Source: ChoiceSourcePackage, Key: "devil", Border: "fire", Reset: true}}},
		{"reset with a colour", []CardLookChange{{FieldKey: "ancestry", Source: ChoiceSourcePackage, Key: "devil", Colour: "#aabbcc", Reset: true}}},
		{"reset of a campaign entry", []CardLookChange{{FieldKey: "ancestry", Source: ChoiceSourceCampaign, Key: "1", Reset: true}}},
		{"an empty key", []CardLookChange{pkgChange("ancestry", "", "fire", "")}},
		{"a key longer than the column", []CardLookChange{pkgChange("ancestry", long, "fire", "")}},
		{"the same card twice", []CardLookChange{pkgChange("ancestry", "devil", "fire", ""), pkgChange("ancestry", "devil", "ice", "")}},
		{"501 items", many},
		{"a valid item before an invalid one writes nothing", []CardLookChange{pkgChange("ancestry", "hakaan", "fire", ""), pkgChange("ancestry", "devil", "fire", "red")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLooksEnv(t)
			err := env.looks.Apply(context.Background(), lookCamp, "dm", tt.in)
			if code(err) != http.StatusUnprocessableEntity && code(err) != http.StatusBadRequest {
				t.Fatalf("err = %v (code %d), want a validation error", err, code(err))
			}
			if env.lrepo.writes != 0 || len(env.lrepo.rows) != 0 {
				t.Errorf("a refused save wrote rows: %v", env.lrepo.rows)
			}
		})
	}
}

func TestCardLookApply_PackageEntries(t *testing.T) {
	env := newLooksEnv(t)
	ctx := context.Background()
	apply := func(cs ...CardLookChange) {
		t.Helper()
		if err := env.looks.Apply(ctx, lookCamp, "dm", cs); err != nil {
			t.Fatal(err)
		}
	}
	row := func(key string) (StoredLook, bool) {
		r, ok := env.lrepo.rows[[4]string{lookCamp, "sys", "ancestry", key}]
		return r, ok
	}

	// A mixed-case colour is stored lower-case; "no border" is a real row.
	apply(pkgChange("ancestry", "hakaan", "fire", "#ABCDEF"), pkgChange("ancestry", "devil", "", ""))
	if r, _ := row("hakaan"); r.Border != "fire" || r.Colour != "#abcdef" {
		t.Errorf("hakaan row = %+v", r)
	}
	if r, ok := row("devil"); !ok || r.Border != "" {
		t.Errorf("devil should hold an explicit no-border row: %+v %v", r, ok)
	}
	if env.lrepo.by[[4]string{lookCamp, "sys", "ancestry", "hakaan"}] != "dm" {
		t.Error("updated_by not recorded")
	}
	// The same look as the game system's own is no row; so is a reset.
	apply(pkgChange("ancestry", "hakaan", "ice", "#112233"))
	if _, ok := row("hakaan"); ok {
		t.Error("choosing the package's own look must delete the row, not store a copy")
	}
	apply(CardLookChange{FieldKey: "ancestry", Source: ChoiceSourcePackage, Key: "devil", Reset: true})
	if _, ok := row("devil"); ok {
		t.Error("reset must delete the row")
	}
	// "None" on a card the package gives no border is also its own look.
	apply(pkgChange("ancestry", "human", "", ""))
	if _, ok := row("human"); ok {
		t.Error("no border on a plain card must not store a row")
	}
	// The package's own look is judged even when a row hides it: devil's own
	// look is fire (colour defaulted), so choosing exactly that clears a row.
	apply(pkgChange("ancestry", "devil", "vine", ""))
	apply(pkgChange("ancestry", "devil", "fire", "#d9622b"))
	if _, ok := row("devil"); ok {
		t.Error("the package look written out in full must still match and delete the row")
	}
	// A derived subclass is a package entry keyed parent--child.
	apply(pkgChange("subclass", "fury--reaver", "storm", ""))
	if _, ok := env.lrepo.rows[[4]string{lookCamp, "sys", "subclass", "fury--reaver"}]; !ok {
		t.Error("a subclass's look was not stored")
	}
	// A stale draft names a card that is gone: dropped, not an error.
	before := len(env.lrepo.rows)
	apply(pkgChange("ancestry", "ghost", "fire", ""), pkgChange("subclass", "fury--nobody", "fire", ""))
	if len(env.lrepo.rows) != before {
		t.Error("a card that is not on the list must be dropped")
	}
}

func TestCardLookApply_CampaignEntries(t *testing.T) {
	env := newLooksEnv(t)
	ctx := context.Background()
	moth := env.entry(t, "Mothfolk", "vine", "")
	moth.ReviewNote = ""
	draft := env.entry(t, "Drafty", "", EntryStatusDraft)
	sent := env.entry(t, "Sent", "", EntryStatusSent)
	// An entry of another campaign.
	foreign, err := env.entries.Create(ctx, "c2", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Foreign", Properties: map[string]any{"size": "2M"}})
	if err != nil {
		t.Fatal(err)
	}
	stored := func(id int64, camp string) SystemEntry {
		e, err := env.erepo.Get(ctx, camp, id)
		if err != nil {
			t.Fatal(err)
		}
		return *e
	}
	campChange := func(id int64, border, colour string) CardLookChange {
		return CardLookChange{FieldKey: "ancestry", Source: ChoiceSourceCampaign, Key: lookID(id), Border: border, Colour: colour}
	}

	// Set a border and colour: only the two properties change.
	before := stored(moth.ID, lookCamp)
	if err := env.looks.Apply(ctx, lookCamp, "dm", []CardLookChange{campChange(moth.ID, "fire", "#ABCDEF")}); err != nil {
		t.Fatal(err)
	}
	after := stored(moth.ID, lookCamp)
	if after.Properties["card_border"] != "fire" || after.Properties["card_colour"] != "#abcdef" || after.Properties["size"] != "1M" {
		t.Errorf("properties = %v", after.Properties)
	}
	after.Properties, before.Properties = nil, nil
	after.UpdatedAt = before.UpdatedAt
	if !reflect.DeepEqual(before, after) {
		t.Errorf("something besides the look changed:\n%+v\n%+v", before, after)
	}
	// Border with no colour drops the colour; no border drops both.
	if err := env.looks.Apply(ctx, lookCamp, "dm", []CardLookChange{campChange(moth.ID, "ice", "")}); err != nil {
		t.Fatal(err)
	}
	if p := stored(moth.ID, lookCamp).Properties; p["card_border"] != "ice" || p["card_colour"] != nil {
		t.Errorf("properties = %v", p)
	}
	if err := env.looks.Apply(ctx, lookCamp, "dm", []CardLookChange{campChange(moth.ID, "", "")}); err != nil {
		t.Fatal(err)
	}
	if p := stored(moth.ID, lookCamp).Properties; p["card_border"] != nil || p["card_colour"] != nil || p["size"] != "1M" {
		t.Errorf("properties = %v", p)
	}
	// No row is ever written for a campaign entry.
	if len(env.lrepo.rows) != 0 {
		t.Errorf("a campaign entry must not get a looks row: %v", env.lrepo.rows)
	}

	// A draft or sent entry is its writer's until accepted.
	for _, e := range []*SystemEntry{draft, sent} {
		err := env.looks.Apply(ctx, lookCamp, "dm", []CardLookChange{campChange(e.ID, "fire", "")})
		if code(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "Accept this entry first") {
			t.Errorf("entry %d: err = %v", e.ID, err)
		}
		if stored(e.ID, lookCamp).Properties["card_border"] != nil {
			t.Errorf("entry %d was changed", e.ID)
		}
	}

	// An id from another campaign is not on this campaign's list: dropped,
	// and the other campaign's entry is untouched.
	if err := env.looks.Apply(ctx, lookCamp, "dm", []CardLookChange{campChange(foreign.ID, "fire", "")}); err != nil {
		t.Fatal(err)
	}
	if stored(foreign.ID, "c2").Properties["card_border"] != nil {
		t.Error("another campaign's entry was changed")
	}
	// Even called directly with the id, the entry service finds only its own campaign's.
	if _, err := env.entries.SetCardLook(ctx, lookCamp, dirActor, foreign.ID, "fire", ""); code(err) != http.StatusNotFound {
		t.Errorf("direct call with a foreign id: %v", err)
	}
	// Only a Director may change it.
	if _, err := env.entries.SetCardLook(ctx, lookCamp, playerActor, moth.ID, "fire", ""); code(err) != http.StatusForbidden {
		t.Errorf("a player's SetCardLook: %v", err)
	}
	if _, err := env.entries.SetCardLook(ctx, lookCamp, dirActor, moth.ID, "fire;x", ""); code(err) != http.StatusUnprocessableEntity {
		t.Errorf("a bad border through the entry service: %v", err)
	}
}

func TestCardLookForgetDerivedOnEntryDelete(t *testing.T) {
	env := newLooksEnv(t)
	ctx := context.Background()
	en, err := env.entries.Create(ctx, lookCamp, dirActor, CreateSystemEntryInput{FieldKey: "class", Name: "Warden"})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"warden--a", "warden--b", "wardens--a", "fury--a", "warden"} {
		env.lrepo.put(lookCamp, "subclass", k, "fire", "")
	}
	if err := env.entries.Delete(ctx, lookCamp, dirActor, en.ID); err != nil {
		t.Fatal(err)
	}
	if len(env.lrepo.rows) != 3 {
		t.Errorf("only the deleted entry's derived rows go: %v", env.lrepo.rows)
	}
	for k := range env.lrepo.rows {
		if strings.HasPrefix(k[3], "warden--") {
			t.Errorf("row %v survived", k)
		}
	}
}
