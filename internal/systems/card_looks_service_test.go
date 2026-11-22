package systems

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeLookRepo is the looks table in memory, keyed like the real primary key.
type fakeLookRepo struct {
	mu      sync.Mutex
	rows    map[[4]string]StoredLook
	by      map[[4]string]string
	failGet error
	writes  int
}

func newFakeLookRepo() *fakeLookRepo {
	return &fakeLookRepo{rows: map[[4]string]StoredLook{}, by: map[[4]string]string{}}
}

func (f *fakeLookRepo) List(_ context.Context, camp, sys, field string) (map[string]StoredLook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGet != nil {
		return nil, f.failGet
	}
	out := map[string]StoredLook{}
	for k, v := range f.rows {
		if k[0] == camp && k[1] == sys && k[2] == field {
			out[k[3]] = v
		}
	}
	return out, nil
}

func (f *fakeLookRepo) Apply(_ context.Context, ups, dels []CardLookRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range ups {
		k := [4]string{u.CampaignID, u.SystemID, u.FieldKey, u.EntryKey}
		f.rows[k], f.by[k] = u.StoredLook, u.UpdatedBy
		f.writes++
	}
	for _, d := range dels {
		delete(f.rows, [4]string{d.CampaignID, d.SystemID, d.FieldKey, d.EntryKey})
		f.writes++
	}
	return nil
}

func (f *fakeLookRepo) DeleteDerived(_ context.Context, camp, sys, slug string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for k := range f.rows {
		if k[0] == camp && k[1] == sys && slug != "" && strings.HasPrefix(k[3], slug+"--") {
			delete(f.rows, k)
			n++
		}
	}
	return n, nil
}

func (f *fakeLookRepo) put(camp, field, key, border, colour string) {
	f.rows[[4]string{camp, "sys", field, key}] = StoredLook{Border: border, Colour: colour}
}

// looksEnv is the real pick-list stack (package files, the campaign's entries
// through the real overlay, the card look service) over in-memory stores.
type looksEnv struct {
	choices *characterChoiceService
	looks   CardLookService
	lrepo   *fakeLookRepo
	erepo   *fakeEntryRepo
	entries SystemEntryService
}

const lookCamp = "c1"

func newLooksEnv(t *testing.T) *looksEnv {
	t.Helper()
	choiceSourcesMu.Lock()
	savedOverlay, savedSources := choiceOverlay, choiceSources
	choiceSources = nil
	choiceSourcesMu.Unlock()
	t.Cleanup(func() {
		choiceSourcesMu.Lock()
		choiceOverlay, choiceSources = savedOverlay, savedSources
		choiceSourcesMu.Unlock()
	})
	m := creatorTestManifest()
	files := map[string]string{
		"sys/data/ancestries.json": `[` +
			`{"slug":"hakaan","name":"Hakaan","properties":{"card_border":"ice","card_colour":"#112233","speed":5}},` +
			`{"slug":"devil","name":"Devil","properties":{"card_border":"fire"}},` +
			`{"slug":"human","name":"Human"}]`,
		"sys/data/classes.json": `[{"slug":"fury","name":"Fury","properties":{"subclasses":["Berserker","Reaver"]}}]`,
	}
	choices := newTestChoiceService(t, files, []*SystemManifest{m}, fakeAddons{"sys": true})
	lrepo, erepo := newFakeLookRepo(), &fakeEntryRepo{}
	looks := NewCardLookService(lrepo, choices)
	choices.SetChoiceLooks(looks)
	resolver := EntryManifestResolver(func(context.Context, string) *SystemManifest { return m })
	entries := NewSystemEntryService(erepo, resolver, WithEntryCharacters(testCharacters), WithEntryLooks(looks.(EntryLookForgetter)))
	looks.SetEntries(entries)
	RegisterSystemEntryChoices(entries)
	return &looksEnv{choices: choices, looks: looks, lrepo: lrepo, erepo: erepo, entries: entries}
}

func (e *looksEnv) list(t *testing.T, camp, field string, ctx context.Context) map[string]Choice {
	t.Helper()
	l, err := e.choices.CharacterChoiceList(ctx, camp, field)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Choice{}
	for _, c := range l.Choices {
		out[c.Key] = c
	}
	return out
}

func directorCtx() context.Context {
	return WithChoiceViewerUser(WithChoiceViewer(context.Background(), true), "dm")
}
func playerCtx() context.Context {
	return WithChoiceViewerUser(WithChoiceViewer(context.Background(), false), "pl")
}

func (e *looksEnv) entry(t *testing.T, name, border string, status string) *SystemEntry {
	t.Helper()
	props := map[string]any{"size": "1M"}
	if border != "" {
		props["card_border"] = border
	}
	en, err := e.entries.Create(context.Background(), lookCamp, dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: name, Properties: props})
	if err != nil {
		t.Fatal(err)
	}
	if status != "" {
		for i := range e.erepo.rows {
			if e.erepo.rows[i].ID == en.ID {
				e.erepo.rows[i].Status = status
			}
		}
	}
	return en
}

func TestChoiceListCardLooks(t *testing.T) {
	tests := []struct {
		name       string
		row        StoredLook
		hasRow     bool
		wantBorder string // "" = none
		wantColour string // "" = none
		changed    bool
		sysBorder  string // the recorded package look, "" = nil
	}{
		{"no row keeps the package look", StoredLook{}, false, "ice", "#112233", false, ""},
		{"a row replaces border and colour", StoredLook{"fire", "#ABCDEF"}, true, "fire", "#abcdef", true, "ice"},
		{"no colour drops the package's colour so the border default applies", StoredLook{"vine", ""}, true, "vine", "", true, "ice"},
		{"an empty border is no border", StoredLook{"", ""}, true, "", "", true, "ice"},
		{"a border that left the library is ignored", StoredLook{"gone", "#ffffff"}, true, "ice", "#112233", false, ""},
		{"a bad stored colour is not served", StoredLook{"fire", "red;background:url(x)"}, true, "fire", "", true, "ice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLooksEnv(t)
			if tt.hasRow {
				env.lrepo.put(lookCamp, "ancestry", "hakaan", tt.row.Border, tt.row.Colour)
			}
			// Every kind of reader sees the owner's choice.
			for name, ctx := range map[string]context.Context{"director": directorCtx(), "player": playerCtx(), "visitor": WithChoiceVisitor(context.Background()), "plain": context.Background()} {
				c, ok := env.list(t, lookCamp, "ancestry", ctx)["hakaan"]
				if !ok {
					t.Fatalf("%s: hakaan missing", name)
				}
				if got, _ := c.Properties["card_border"].(string); got != tt.wantBorder {
					t.Errorf("%s: card_border = %q, want %q", name, got, tt.wantBorder)
				}
				if got, _ := c.Properties["card_colour"].(string); got != tt.wantColour {
					t.Errorf("%s: card_colour = %q, want %q", name, got, tt.wantColour)
				}
				if c.LookChanged != tt.changed {
					t.Errorf("%s: LookChanged = %v", name, c.LookChanged)
				}
				if tt.sysBorder == "" && c.SystemLook != nil || tt.sysBorder != "" && (c.SystemLook == nil || c.SystemLook.Border != tt.sysBorder || c.SystemLook.Colour != "#112233") {
					t.Errorf("%s: SystemLook = %+v, want border %q", name, c.SystemLook, tt.sysBorder)
				}
				if c.Properties["speed"] == nil {
					t.Errorf("%s: other properties must stay", name)
				}
			}
		})
	}
}

// The package's own map is shared and cached, so a changed card is a copy: the
// next list, and another campaign, still see the package's own look.
func TestChoiceListCardLooks_DoesNotMutatePackage(t *testing.T) {
	env := newLooksEnv(t)
	env.lrepo.put(lookCamp, "ancestry", "hakaan", "fire", "#ffffff")
	env.lrepo.put(lookCamp, "ancestry", "devil", "", "")
	if got := env.list(t, lookCamp, "ancestry", directorCtx())["hakaan"].Properties["card_border"]; got != "fire" {
		t.Fatalf("override not served: %v", got)
	}
	other := env.list(t, "c2", "ancestry", directorCtx())
	if c := other["hakaan"]; c.Properties["card_border"] != "ice" || c.Properties["card_colour"] != "#112233" || c.LookChanged {
		t.Errorf("a campaign with no rows must see the package look: %+v", c)
	}
	if c := other["devil"]; c.Properties["card_border"] != "fire" {
		t.Errorf("devil's package look is gone for another campaign: %+v", c.Properties)
	}
	// The cached file is read fresh each call here, so also prove the copy
	// directly on a shared map.
	shared := map[string]any{"card_border": "ice", "card_colour": "#112233"}
	l := &ChoiceList{SystemID: "sys", FieldKey: "ancestry", Choices: []Choice{{Key: "hakaan", Source: ChoiceSourcePackage, Properties: shared}}}
	env.choices.applyLooks(directorCtx(), lookCamp, l)
	if shared["card_border"] != "ice" || shared["card_colour"] != "#112233" || len(shared) != 2 {
		t.Errorf("the package's own map was changed: %v", shared)
	}
	if l.Choices[0].Properties["card_border"] != "fire" {
		t.Errorf("the choice did not get the override: %v", l.Choices[0].Properties)
	}
}

func TestChoiceListCardLooks_CampaignEntryUntouchedAndReaderFailure(t *testing.T) {
	env := newLooksEnv(t)
	en := env.entry(t, "Mothfolk", "vine", "")
	// A row under the entry's id (or slug) must never shadow the entry.
	env.lrepo.put(lookCamp, "ancestry", "mothfolk", "fire", "")
	env.lrepo.put(lookCamp, "ancestry", "12", "fire", "")
	env.lrepo.put(lookCamp, "ancestry", lookID(en.ID), "fire", "")
	c := env.list(t, lookCamp, "ancestry", directorCtx())[lookID(en.ID)]
	if c.Properties["card_border"] != "vine" || c.LookChanged {
		t.Errorf("a campaign entry must keep its own look: %+v", c)
	}
	// A failing read serves the package looks and keeps the list up.
	env.lrepo.failGet = errors.New("db down")
	h := env.list(t, lookCamp, "ancestry", directorCtx())["hakaan"]
	if h.Properties["card_border"] != "ice" || h.LookChanged {
		t.Errorf("failed read must serve the package look: %+v", h)
	}
}

func lookID(n int64) string { return strconv.FormatInt(n, 10) }

// Derived entries (subclasses) return early from the list; an owner's look
// applies to them too, keyed parent--child.
func TestChoiceListCardLooks_DerivedSubclass(t *testing.T) {
	env := newLooksEnv(t)
	env.lrepo.put(lookCamp, "subclass", "fury--berserker", "fire", "#aabbcc")
	subs := env.list(t, lookCamp, "subclass", directorCtx())
	b, ok := subs["fury--berserker"]
	if !ok {
		t.Fatalf("subclass keys: %v", subs)
	}
	if b.Properties["card_border"] != "fire" || b.Properties["card_colour"] != "#aabbcc" || !b.LookChanged || b.SystemLook != nil {
		t.Errorf("berserker: %+v", b)
	}
	if r := subs["fury--reaver"]; r.LookChanged || r.Properties["card_border"] != nil {
		t.Errorf("reaver must be untouched: %+v", r)
	}
}
