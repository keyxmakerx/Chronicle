package records

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

type fakeEntryFields struct{}

func (fakeEntryFields) Entries(context.Context, string, string, bool) (string, []SystemEntry, error) {
	return "", nil, nil
}
func (fakeEntryFields) Fields(_ context.Context, _ string, director bool) (string, []SystemEntryField, error) {
	return "Draw Steel", []SystemEntryField{{Key: "ancestry", Label: "Heritage", Count: 12}, {Key: "kit", Label: "kit", Count: 30}}, nil
}

func TestLookups_CalendarCategoriesAndImportance(t *testing.T) {
	tiers := fakeTiers{{Slug: "major", Name: "Major"}, {Slug: "standard", Name: "Standard", IsDefault: true}, {Slug: "side-quest", Name: "Side quest"}}
	l := calendarWorld()
	l.Tiers = tiers
	l.Cal.(*fakeCal).cal.EventKinds = []calendar.EventKind{{Name: "Festival", Slug: "festival"}}

	ans := l.Answer(context.Background(), camp, owner, []Record{lookup(lookupWhatCalendar, nil)})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	for _, want := range []string{
		"### Event categories (the names `category:` takes on events)\n\nFestival",
		"### Event importance levels (the names `importance:` takes on events)",
		"- Major\n", "- Standard, the usual level", "- Side quest (`side-quest`)",
	} {
		if !strings.Contains(ans.Text, want) {
			t.Errorf("answer lacks %q:\n%s", want, ans.Text)
		}
	}
	if strings.Contains(ans.Text, "Event kinds") {
		t.Error("the old heading is still there")
	}

	// Without tiers wired the rest of the answer stands.
	l.Tiers = nil
	ans = l.Answer(context.Background(), camp, owner, []Record{lookup(lookupWhatCalendar, nil)})[0]
	if ans.Error != "" || strings.Contains(ans.Text, "importance") {
		t.Errorf("unwired tiers: %q %q", ans.Error, ans.Text)
	}

	// A player is not shown the levels.
	l.Tiers = tiers
	ans = l.Answer(context.Background(), camp, player, []Record{lookup(lookupWhatCalendar, nil)})[0]
	if strings.Contains(ans.Text, "importance") {
		t.Error("a player was shown the importance levels")
	}
}

// fakePlug is a feature's lookup plugged in from outside the engine.
type fakePlug struct {
	got  []Record
	text string
	err  error
}

func (f *fakePlug) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{What: "widgets", Aliases: []string{"widget"}, Keys: "optional `name`", Brief: "the widgets"}
}

func (f *fakePlug) Answer(_ context.Context, _ string, _ Actor, r Record) (string, string, error) {
	f.got = append(f.got, r)
	return "Widgets · 2", f.text, f.err
}

func TestLookups_Plugged(t *testing.T) {
	p := &fakePlug{text: "## Widgets (2)\n\n- Cog\n- Gear\n"}
	l := &Lookups{Plugged: []aiplug.Lookup{p}}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("widgets", nil), lookup("Widget", map[string]any{"name": "Cog"})})
	if ans[0].Error != "" || ans[0].Chip != "Widgets · 2" || !strings.Contains(ans[0].Text, "- Gear") {
		t.Errorf("by what: %+v", ans[0])
	}
	if ans[1].Error != "" || len(p.got) != 2 || p.got[1].Str("name") != "Cog" {
		t.Errorf("by alias: %+v, got %d records", ans[1], len(p.got))
	}

	// The engine words the plug's refusal like any lookup's and keeps no text.
	p.err = aiplug.BadRequestf("widgets are left out in Safe privacy")
	if got := l.Answer(context.Background(), camp, player, []Record{lookup("widgets", nil)})[0]; got.Error != "widgets are left out in Safe privacy" || got.Text != "" {
		t.Errorf("refusal: %+v", got)
	}

	// The engine's size cap still applies to a plug's answer.
	p.err, p.text = nil, "## Widgets\n\n"+strings.Repeat("- a widget with a long name\n", maxAnswerBytes/20)
	if got := l.Answer(context.Background(), camp, owner, []Record{lookup("widgets", nil)})[0]; len(got.Text) > maxAnswerBytes+200 || !strings.Contains(got.Text, "cut here") {
		t.Errorf("an oversized answer was not cut: %d bytes", len(got.Text))
	}

	// A built-in what is never handed to a plug.
	l.Plugged = []aiplug.Lookup{&fakeShadow{}}
	if got := l.Answer(context.Background(), camp, owner, []Record{lookup("tags", nil)})[0]; got.Chip == "shadow" || got.Text == "shadow" {
		t.Errorf("a plug answered a built-in what: %+v", got)
	}
	if got := (&Lookups{}).Answer(context.Background(), camp, owner, []Record{lookup("widgets", nil)})[0]; !strings.Contains(got.Error, "is not something Chronicle can look up") {
		t.Errorf("unplugged: %+v", got)
	}

	doc := (&Lookups{Plugged: []aiplug.Lookup{p}}).LookupDoc()
	w, f := strings.Index(doc, "`what: widgets`: the widgets. Keys: optional `name`."), strings.Index(doc, "`what: "+lookupWhatFormat+"`")
	if w < 0 || f < 0 || w > f {
		t.Errorf("the plugged lookup should be listed before format:\n%s", doc)
	}
}

// fakeShadow claims the built-in `tags`; the built-in answer must win.
type fakeShadow struct{}

func (fakeShadow) LookupInfo() aiplug.LookupInfo { return aiplug.LookupInfo{What: "tags"} }
func (fakeShadow) Answer(context.Context, string, Actor, Record) (string, string, error) {
	return "shadow", "shadow", nil
}

func TestLookups_SystemEntryFields(t *testing.T) {
	l := &Lookups{Entries: fakeEntryFields{}}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("system-entries", nil)})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	for _, want := range []string{"(Draw Steel)", "- ancestry (Heritage), 12 entries", "- kit, 30 entries", "`entry:`"} {
		if !strings.Contains(ans.Text, want) {
			t.Errorf("answer lacks %q:\n%s", want, ans.Text)
		}
	}
}

func TestLookupDoc_ListsNewLookups(t *testing.T) {
	doc := (&Lookups{}).LookupDoc()
	for _, want := range []string{"`what: tags`", "`tag`", "without `field`"} {
		if !strings.Contains(doc, want) {
			t.Errorf("LookupDoc lacks %q", want)
		}
	}
}

func TestDocs_PointAtNewLookups(t *testing.T) {
	if !strings.Contains((SystemEntryKind{}).Doc(), "`what: system-entries`") {
		t.Error("system-entry doc does not point at the field lookup")
	}
	if !strings.Contains((EncounterKind{}).Doc(), "any page works") {
		t.Error("encounter doc does not say heroes take any page")
	}
}

type manyEntries []SystemEntry

func (m manyEntries) Entries(context.Context, string, string, bool) (string, []SystemEntry, error) {
	return "D&D", m, nil
}
func (manyEntries) Fields(context.Context, string, bool) (string, []SystemEntryField, error) {
	return "", nil, nil
}

// A field with more entries than one answer holds lists every name, and
// `name` narrows it to read their summaries.
func TestLookups_SystemEntriesLongList(t *testing.T) {
	var m manyEntries
	for i := 0; i < 400; i++ {
		m = append(m, SystemEntry{Name: fmt.Sprintf("Spell %03d", i), Slug: fmt.Sprintf("spell-%03d", i), Summary: strings.Repeat("A long summary. ", 10)})
	}
	l := &Lookups{Entries: m}
	ans := l.Answer(context.Background(), camp, owner, []Record{
		lookup("system-entries", map[string]any{"field": "spell"}),
		{Kind: KindLookup, Name: "spell 39", Fields: map[string]any{"what": "system-entries", "field": "spell"}},
		{Kind: KindLookup, Name: "nothing", Fields: map[string]any{"what": "system-entries", "field": "spell"}},
	})
	all := ans[0]
	if all.Error != "" || strings.Contains(all.Text, "cut here") || !strings.Contains(all.Text, "names and slugs only") ||
		!strings.Contains(all.Text, "- Spell 000 (spell-000)\n") || !strings.Contains(all.Text, "- Spell 399 (spell-399)\n") {
		t.Fatalf("long list: %d bytes\n%s", len(all.Text), all.Text[:300])
	}
	one := ans[1]
	if one.Error != "" || one.Chip != "Spell · 10" || !strings.Contains(one.Text, "A long summary.") {
		t.Fatalf("narrowed: %+v", one)
	}
	if !strings.Contains(ans[2].Error, "no spell entry's name holds") {
		t.Fatalf("no match: %+v", ans[2])
	}
}
