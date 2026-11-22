package records

import (
	"context"
	"strings"
	"testing"
)

func seenFixture() (TrackerKind, *fakeTrackers) {
	k, svc := trackerFixture()
	svc.kinds = []TrackerSetup{{ID: "k1", Name: "Hunger", Shape: trackerCount, Visibility: trackerShown}}
	return k, svc
}

func seenRec(action string, f map[string]any) Record {
	return rec("tracker", action, "Hunger", f, "")
}

func TestTrackerKind_ShownTo(t *testing.T) {
	tests := []struct {
		name    string
		fields  map[string]any
		wantVis string
		wantIDs []string // chosen players' ids
		wantMay []string
		summary string
	}{
		{"one player by display name", map[string]any{"page": "Sivin", "shown_to": []any{"Pat"}}, trackerChosen, []string{"u-pat"}, nil, "shown to Pat on Sivin"},
		{"a single name as a string", map[string]any{"page": "Sivin", "shown_to": "pat"}, trackerChosen, []string{"u-pat"}, nil, "shown to Pat"},
		{"a hero page's name reaches its player", map[string]any{"page": "Sivin", "shown_to": []any{"  SIVIN "}}, trackerChosen, []string{"u-pat"}, nil, "shown to Pat"},
		{"a claimed hero's name reaches its player", map[string]any{"page": "Sivin", "shown_to": []any{"old brannoc"}}, trackerChosen, []string{"u-sam"}, nil, "shown to Sam"},
		{"two players, one may change", map[string]any{"page": "Sivin", "shown_to": []any{"Pat", "Sam"}, "may_change": []any{"Pat"}}, trackerChosen, []string{"u-pat", "u-sam"}, []string{"u-pat"}, "shown to Pat and Sam; Pat may change it"},
		{"may_change as a string", map[string]any{"page": "Sivin", "shown_to": []any{"Pat", "Sam"}, "may_change": "Brannoc"}, trackerChosen, []string{"u-pat", "u-sam"}, []string{"u-sam"}, "Sam may change it"},
		{"hidden", map[string]any{"page": "Sivin", "shown_to": "directors"}, trackerHidden, nil, nil, "hidden from players on Sivin"},
		{"everyone", map[string]any{"page": "Sivin", "shown_to": "Everyone"}, trackerShown, nil, nil, "shown to everyone on Sivin"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k, svc := seenFixture()
			r := seenRec(ActionUpdate, tc.fields)
			p := k.Plan(context.Background(), camp, owner, r)
			if p.Error != "" || !strings.Contains(p.Summary, tc.summary) {
				t.Fatalf("plan %+v, want %q", p, tc.summary)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			got := svc.placed["p-sivin"]
			if len(got) != 1 || got[0].Visibility != tc.wantVis || len(got[0].Players) != len(tc.wantIDs) {
				t.Fatalf("placed %+v", got)
			}
			for i, id := range tc.wantIDs {
				may := false
				for _, m := range tc.wantMay {
					may = may || m == id
				}
				if got[0].Players[i].UserID != id || got[0].Players[i].CanChange != may {
					t.Fatalf("players %+v", got[0].Players)
				}
			}
			// A new placement is made hidden first, never open to more people
			// than the block asks for.
			if len(svc.placedAs) != 1 || svc.placedAs[0] != trackerHidden {
				t.Fatalf("placed as %v", svc.placedAs)
			}
		})
	}
}

func TestTrackerKind_ShownToRefusals(t *testing.T) {
	tests := []struct {
		name   string
		a      Actor
		fields map[string]any
		want   string
	}{
		{"no page", owner, map[string]any{"shown_to": []any{"Pat"}}, "set per page"},
		{"unknown name", owner, map[string]any{"page": "Sivin", "shown_to": []any{"Zed"}}, "players who can be chosen: Alex, Pat, Sam, alex"},
		{"ambiguous name", owner, map[string]any{"page": "Sivin", "shown_to": []any{"ALEX"}}, "more than one member"},
		{"a director", owner, map[string]any{"page": "Sivin", "shown_to": []any{"Dee"}}, "Director and already sees"},
		{"may_change outside shown_to", owner, map[string]any{"page": "Sivin", "shown_to": []any{"Pat"}, "may_change": []any{"Sam"}}, "not in `shown_to`"},
		{"may_change alone", owner, map[string]any{"page": "Sivin", "may_change": []any{"Pat"}}, "goes with `shown_to`"},
		{"may_change with everyone", owner, map[string]any{"page": "Sivin", "shown_to": "everyone", "may_change": []any{"Pat"}}, "no meaning"},
		{"empty list", owner, map[string]any{"page": "Sivin", "shown_to": []any{}}, "at least one player"},
		{"a player", player, map[string]any{"page": "Sivin", "shown_to": []any{"Pat"}}, "owner or a co-DM"},
		{"unknown page", owner, map[string]any{"page": "Nowhere", "shown_to": []any{"Pat"}}, "no page called"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k, _ := seenFixture()
			p := k.Plan(context.Background(), camp, tc.a, seenRec(ActionUpdate, tc.fields))
			if !strings.Contains(p.Error, tc.want) {
				t.Fatalf("error %q, want %q", p.Error, tc.want)
			}
			for _, leak := range []string{"u-pat", "u-dm", "@"} {
				if strings.Contains(p.Error, leak) {
					t.Fatalf("error leaks %q: %s", leak, p.Error)
				}
			}
		})
	}
}

func TestTrackerKind_ShownToChangesNothing(t *testing.T) {
	k, svc := seenFixture()
	r := seenRec(ActionUpdate, map[string]any{"page": "Sivin", "shown_to": []any{"Pat"}, "may_change": []any{"Pat"}})
	planApply(t, k, r)
	p := k.Plan(context.Background(), camp, owner, r)
	if !strings.Contains(p.Summary, "changes nothing") {
		t.Fatalf("plan %q", p.Summary)
	}
	calls := len(svc.placedAs)
	if err := k.Apply(context.Background(), camp, owner, r); err != nil || len(svc.placedAs) != calls {
		t.Fatalf("apply again: %v, placements %v", err, svc.placedAs)
	}
	// Dropping may_change is a change.
	r2 := seenRec(ActionUpdate, map[string]any{"page": "Sivin", "shown_to": []any{"Pat"}})
	if p := k.Plan(context.Background(), camp, owner, r2); strings.Contains(p.Summary, "changes nothing") {
		t.Fatalf("plan %q", p.Summary)
	}
}

// A create names pages, each getting the tracker and the audience; a block may
// also carry a value change for one page.
func TestTrackerKind_ShownToOnCreateAndWithAdd(t *testing.T) {
	k, svc := trackerFixture()
	planApply(t, k, rec("tracker", ActionCreate, "Hunger", map[string]any{
		"pages": []any{"Orrin", "Sivin"}, "shown_to": []any{"Pat", "Sam"}}, ""))
	for _, pg := range []string{"p-orrin", "p-sivin"} {
		got := svc.placed[pg]
		if len(got) != 1 || got[0].Visibility != trackerChosen || len(got[0].Players) != 2 {
			t.Fatalf("%s placed %+v", pg, got)
		}
	}
	p := planApply(t, k, rec("tracker", ActionUpdate, "Hunger", map[string]any{
		"page": "Sivin", "add": 1, "note": "Skipped supper", "shown_to": "everyone"}, ""))
	if !strings.Contains(p.Summary, "Sivin +1") || !strings.Contains(p.Summary, "shown to everyone on Sivin") {
		t.Fatalf("summary %q", p.Summary)
	}
	if svc.placed["p-sivin"][0].Visibility != trackerShown || len(svc.logs) != 1 || svc.logs[0].TrackerID != svc.placed["p-sivin"][0].ID {
		t.Fatalf("placed %+v logs %+v", svc.placed["p-sivin"], svc.logs)
	}
	if svc.placed["p-orrin"][0].Visibility != trackerChosen {
		t.Fatal("the other page changed")
	}
}

// A tracker a category puts on every page is made a row, hidden, before it is
// patched, so it is never open to more people than asked.
func TestTrackerKind_ShownToOnAnEveryPageTracker(t *testing.T) {
	k, svc := trackerFixture()
	svc.charTyp["p-sivin"] = true
	svc.kinds = []TrackerSetup{{ID: "k1", Name: "Hunger", Shape: trackerCount, Visibility: trackerShown, EveryTypeID: 7}}
	planApply(t, k, seenRec(ActionUpdate, map[string]any{"page": "Sivin", "shown_to": []any{"Pat"}}))
	got := svc.placed["p-sivin"]
	if len(got) != 1 || got[0].Visibility != trackerChosen || svc.placedAs[0] != trackerHidden {
		t.Fatalf("placed %+v as %v", got, svc.placedAs)
	}
}

// The export writes a chosen-players placement as a block that reads back as
// no change, and the lookup names who sees each tracker in its three ways.
func TestTrackerKind_ShownToExportAndLookup(t *testing.T) {
	k, svc := seenFixture()
	planApply(t, k, seenRec(ActionUpdate, map[string]any{"pages": []any{"Orrin", "Sivin", "Saatraaol"}}))
	planApply(t, k, seenRec(ActionUpdate, map[string]any{"page": "Sivin", "shown_to": []any{"Pat", "Sam"}, "may_change": []any{"Pat"}}))
	planApply(t, k, seenRec(ActionUpdate, map[string]any{"page": "Orrin", "shown_to": "everyone"}))
	planApply(t, k, seenRec(ActionUpdate, map[string]any{"page": "Saatraaol", "shown_to": "directors"}))

	out, err := k.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "page: Sivin\nshown_to: [Pat, Sam]\nmay_change: [Pat]\n") || strings.Count(out, "shown_to:") != 1 {
		t.Fatalf("export %q", out)
	}
	for _, r := range parseBlocks(out) {
		if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" {
			t.Fatalf("block plans with %q:\n%s", p.Error, out)
		} else if r.Has("shown_to") && !strings.Contains(p.Summary, "changes nothing") {
			t.Fatalf("read back plans %q", p.Summary)
		}
	}

	for pg, ts := range svc.placed {
		for i := range ts {
			svc.placed[pg][i].Name = "Hunger"
		}
	}
	run := &lookupRun{ctx: context.Background(), campaignID: camp, a: owner, l: &Lookups{Trackers: svc}}
	ans, err := run.trackersLookup("")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Hunger on Sivin: 0; who sees it: shown to Pat (may change it), Sam",
		"Hunger on Orrin: 0; who sees it: everyone",
		"Hunger on Saatraaol: 0; who sees it: Director only",
	} {
		if !strings.Contains(ans.Text, want) {
			t.Fatalf("lookup missing %q:\n%s", want, ans.Text)
		}
	}
	// A player's key or the Safe mode never reaches the names.
	run.a = player
	if ans, err := run.trackersLookup(""); err == nil || strings.Contains(ans.Text, "Pat") {
		t.Fatalf("a player read %q (err %v)", ans.Text, err)
	}
}

func TestTrackerKind_ShownToDoc(t *testing.T) {
	doc := (TrackerKind{}).Doc()
	for _, want := range []string{"`shown_to`", "`may_change`", "name: Hunger\npage: Sivin\nshown_to: [Sivin]"} {
		if !strings.Contains(doc, want) {
			t.Errorf("doc lacks %q", want)
		}
	}
}

// A request that sets the kind's settings and who sees a tracker keeps the
// settings only, and says so by name.
func TestTrackerKind_UndoNamesSettingsOnly(t *testing.T) {
	k, _ := seenFixture()
	r := seenRec(ActionUpdate, map[string]any{"players_see": false, "page": "Sivin", "shown_to": []any{"Pat"}})
	snap, err := k.Before(context.Background(), camp, owner, r)
	if err != nil || snap == nil || !strings.Contains(snap.Name, "settings, not who sees it") {
		t.Fatalf("snapshot %+v err %v", snap, err)
	}
	r = seenRec(ActionUpdate, map[string]any{"page": "Sivin", "shown_to": []any{"Pat"}})
	if snap, _ := k.Before(context.Background(), camp, owner, r); snap != nil {
		t.Fatalf("an audience-only change kept %+v", snap)
	}
}
