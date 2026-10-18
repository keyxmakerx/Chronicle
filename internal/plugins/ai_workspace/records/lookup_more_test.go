package records

import (
	"context"
	"strings"
	"testing"

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

func TestLookups_GameNights(t *testing.T) {
	n := nights()
	n.list[0].Time = "19:30"
	n.list[0].Summary = "The siege of Harrowmere"
	l := &Lookups{Nights: n}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("game-nights", nil)})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	for _, want := range []string{
		"## Game nights (3)",
		"- Session 12: 2026-10-17 19:30, planned. The siege of Harrowmere",
		"- One-shot: 2026-09-01, played",
	} {
		if !strings.Contains(ans.Text, want) {
			t.Errorf("answer lacks %q:\n%s", want, ans.Text)
		}
	}
	if got := l.Answer(context.Background(), camp, player, []Record{lookup("game-nights", nil)})[0]; !strings.Contains(got.Error, "owner or a co-DM") || got.Text != "" {
		t.Errorf("a player got %+v", got)
	}
	if got := (&Lookups{}).Answer(context.Background(), camp, owner, []Record{lookup("game-nights", nil)})[0]; !strings.Contains(got.Error, "can't be looked up") {
		t.Errorf("unwired: %+v", got)
	}
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
	doc := LookupDoc()
	for _, want := range []string{"`what: tags`", "`what: game-nights`", "`tag`", "without `field`"} {
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
