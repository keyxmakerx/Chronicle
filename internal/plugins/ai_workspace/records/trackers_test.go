package records

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// fakeTrackers keeps kinds and the trackers placed on pages; a kind with
// EveryTypeID 7 is on every page of category 7 (the "Character" pages).
type fakeTrackers struct {
	kinds   []TrackerSetup
	placed  map[string][]TrackerOnPage // by page id
	charTyp map[string]bool            // pages of category 7
	logs    []TrackerChange
	noBlock map[string]bool
	n       int
	members []TrackerMember
	// placedAs is the visibility each placement was first made with.
	placedAs []string
}

func newFakeTrackers() *fakeTrackers {
	return &fakeTrackers{placed: map[string][]TrackerOnPage{}, charTyp: map[string]bool{"p-orrin": true}, noBlock: map[string]bool{}}
}

// Check refuses what the plugin's own rule refuses that the tests use.
func (f *fakeTrackers) Check(in TrackerSetup) error {
	switch {
	case in.Shape != trackerCount && in.Shape != trackerSteps && in.Shape != trackerClock:
		return badRequestf("shape must be count, steps or clock")
	case in.Shape == trackerClock && (in.Max == nil || *in.Max < 2 || *in.Max > 24):
		return badRequestf("a clock needs 2 to 24 segments")
	case in.Shape == trackerSteps && (len(in.Steps) < 2 || len(in.Steps) > 20):
		return badRequestf("a steps tracker needs 2 to 20 steps")
	}
	return nil
}
func (f *fakeTrackers) Kinds(context.Context, string, Actor) ([]TrackerSetup, []TrackerCategory, error) {
	return f.kinds, []TrackerCategory{{ID: 7, Name: "Character"}}, nil
}
func (f *fakeTrackers) CreateKind(_ context.Context, _ string, _ Actor, in TrackerSetup) (*TrackerSetup, error) {
	f.n++
	k := TrackerSetup{ID: fmt.Sprintf("k%d", f.n), Name: in.Name, Shape: in.Shape, Min: in.Min, Max: in.Max,
		Steps: in.Steps, Bands: in.Bands, Visibility: in.Visibility, EveryTypeID: in.EveryTypeID, Version: 1}
	f.kinds = append(f.kinds, k)
	return &k, nil
}
func (f *fakeTrackers) UpdateKind(_ context.Context, _, id string, _ Actor, in TrackerSetup) (*TrackerSetup, error) {
	for i := range f.kinds {
		if f.kinds[i].ID == id {
			k := &f.kinds[i]
			k.Name, k.Shape, k.Min, k.Max, k.Visibility, k.EveryTypeID = in.Name, in.Shape, in.Min, in.Max, in.Visibility, in.EveryTypeID
			k.Version++
			return k, nil
		}
	}
	return nil, fmt.Errorf("no kind %s", id)
}
func (f *fakeTrackers) DeleteKind(context.Context, string, string, Actor) error { return nil }
func (f *fakeTrackers) Page(_ context.Context, _, pageID string, _ Actor) (*TrackerPage, error) {
	out := append([]TrackerOnPage(nil), f.placed[pageID]...)
	for _, k := range f.kinds {
		if k.EveryTypeID == 7 && f.charTyp[pageID] {
			has := false
			for _, t := range out {
				has = has || t.KindID == k.ID
			}
			if !has {
				out = append(out, TrackerOnPage{KindID: k.ID, Name: k.Name})
			}
		}
	}
	return &TrackerPage{Trackers: out}, nil
}
func (f *fakeTrackers) SetSeen(_ context.Context, _, id string, a Actor, in TrackerSeen) error {
	if !a.CanAuthorDmOnly() {
		return badRequestf("only a Director can change who sees a tracker")
	}
	for pg, ts := range f.placed {
		for i := range ts {
			if ts[i].ID == id {
				f.placed[pg][i].Visibility, f.placed[pg][i].Players = in.Visibility, in.Players
				return nil
			}
		}
	}
	return badRequestf("no tracker %s", id)
}
func (f *fakeTrackers) Members(context.Context, string, Actor) ([]TrackerMember, error) {
	return f.members, nil
}
func (f *fakeTrackers) Place(_ context.Context, _, pageID string, _ Actor, in TrackerPlace) error {
	f.placedAs = append(f.placedAs, in.Visibility)
	f.n++
	f.placed[pageID] = append(f.placed[pageID], TrackerOnPage{ID: fmt.Sprintf("t%d", f.n), KindID: in.KindID, Visibility: in.Visibility})
	return nil
}
func (f *fakeTrackers) Log(_ context.Context, _, _ string, _ Actor, in TrackerChange) error {
	f.logs = append(f.logs, in)
	return nil
}
func (f *fakeTrackers) Pages(context.Context, string, Actor) ([]TrackerPageRef, error) {
	var out []TrackerPageRef
	for id := range f.placed {
		out = append(out, TrackerPageRef{ID: id, Name: map[string]string{"p-orrin": "Orrin", "p-saat": "Saatraaol", "p-sivin": "Sivin"}[id]})
	}
	return out, nil
}
func (f *fakeTrackers) HasBlock(_ context.Context, _, pageID string) (bool, error) {
	return !f.noBlock[pageID], nil
}

func trackerFixture() (TrackerKind, *fakeTrackers) {
	svc := newFakeTrackers()
	svc.members = []TrackerMember{
		{UserID: "u-pat", Name: "Pat", Character: "Sivin"},
		{UserID: "u-sam", Name: "Sam", Character: "Brannoc"},
		{UserID: "u-twin1", Name: "Alex"}, {UserID: "u-twin2", Name: "alex"},
		{UserID: "u-dm", Name: "Dee", Director: true},
	}
	pages := fakeEntities{
		"orrin":     {ID: "p-orrin", CampaignID: camp, Name: "Orrin"},
		"saatraaol": {ID: "p-saat", CampaignID: camp, Name: "Saatraaol"},
		"sivin":     {ID: "p-sivin", CampaignID: camp, Name: "Sivin"},
	}
	return TrackerKind{Svc: svc, Entities: pages}, svc
}

func planApply(t *testing.T, k TrackerKind, r Record) Plan {
	t.Helper()
	p := k.Plan(context.Background(), camp, owner, r)
	if p.Error != "" {
		t.Fatalf("plan: %s", p.Error)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return p
}

// A secret counter: made hidden, put on one hero's page, then stepped with
// a note on that page's own tracker.
func TestTrackerKind_SecretCounter(t *testing.T) {
	k, svc := trackerFixture()
	p := planApply(t, k, rec("tracker", ActionCreate, "Counsel", map[string]any{
		"min": 0, "max": 5, "bands": []any{map[string]any{"0": "No counsel"}, "3: Advice comes unbidden"}, "pages": []any{"Orrin"},
	}, ""))
	if !strings.Contains(p.Summary, "hidden from players") || !strings.Contains(p.Summary, "a count from 0 to 5") {
		t.Fatalf("summary %q", p.Summary)
	}
	kd := svc.kinds[0]
	if kd.Visibility != trackerHidden || len(kd.Bands) != 2 || kd.Bands[1].At != 3 || kd.Bands[1].Text != "Advice comes unbidden" {
		t.Fatalf("kind %+v", kd)
	}
	if got := svc.placed["p-orrin"]; len(got) != 1 || got[0].Visibility != trackerHidden {
		t.Fatalf("placed %+v", got)
	}

	p = planApply(t, k, rec("tracker", ActionUpdate, "counsel", map[string]any{"page": "Orrin", "add": 1, "note": "Counselled at the gate"}, ""))
	if !strings.Contains(p.Summary, "Orrin +1 (Counselled at the gate)") {
		t.Fatalf("summary %q", p.Summary)
	}
	if len(svc.logs) != 1 || svc.logs[0].TrackerID != svc.placed["p-orrin"][0].ID || svc.logs[0].Op != trackerAdd || svc.logs[0].Amount != 1 {
		t.Fatalf("log %+v", svc.logs)
	}
	if svc.kinds[0].Version != 1 {
		t.Fatal("a change of value rewrote the kind")
	}
}

// A kind on every page of a category logs on a page's virtual tracker by
// kind, without placing it; a page without the kind gets it placed first.
func TestTrackerKind_EveryPageAndPlaceOnLog(t *testing.T) {
	k, svc := trackerFixture()
	planApply(t, k, rec("tracker", ActionCreate, "Respites", map[string]any{"every": "character"}, ""))
	if svc.kinds[0].EveryTypeID != 7 {
		t.Fatalf("kind %+v", svc.kinds[0])
	}
	planApply(t, k, rec("tracker", ActionUpdate, "Respites", map[string]any{"page": "Orrin", "set": 2}, ""))
	if l := svc.logs[0]; l.KindID != svc.kinds[0].ID || l.TrackerID != "" || l.Op != trackerSet {
		t.Fatalf("log %+v", l)
	}
	if len(svc.placed["p-orrin"]) != 0 {
		t.Fatal("placed a kind its category already carries")
	}
	planApply(t, k, rec("tracker", ActionUpdate, "Respites", map[string]any{"page": "Saatraaol", "add": -1}, ""))
	if len(svc.placed["p-saat"]) != 1 || svc.logs[1].TrackerID == "" {
		t.Fatalf("placed %+v log %+v", svc.placed["p-saat"], svc.logs[1])
	}
}

func TestTrackerKind_Refusals(t *testing.T) {
	k, svc := trackerFixture()
	svc.kinds = []TrackerSetup{{ID: "k9", Name: "Doom", Shape: trackerClock, Max: func() *int { n := 6; return &n }(), Visibility: trackerHidden, InUse: 2}}
	tests := []struct {
		name   string
		a      Actor
		action string
		tr     string
		fields map[string]any
		want   string
	}{
		{"player", player, ActionCreate, "New", nil, "owner or a co-DM"},
		{"already there", owner, ActionCreate, "doom", nil, "use action: update"},
		{"update missing", owner, ActionUpdate, "Gone", map[string]any{"page": "Orrin", "add": 1}, "use action: create"},
		{"bad shape", owner, ActionCreate, "New", map[string]any{"shape": "dial"}, "shape must be"},
		{"clock too big", owner, ActionCreate, "New", map[string]any{"shape": "clock", "max": 40}, "segments"},
		{"one step", owner, ActionCreate, "New", map[string]any{"shape": "steps", "steps": []any{"Calm: nothing yet"}}, "2 to 20 steps"},
		{"add and set", owner, ActionUpdate, "Doom", map[string]any{"page": "Orrin", "add": 1, "set": 2}, "not both"},
		{"add nothing", owner, ActionUpdate, "Doom", map[string]any{"page": "Orrin", "add": 0}, "must move"},
		{"page without a change", owner, ActionUpdate, "Doom", map[string]any{"page": "Orrin"}, "use `pages`"},
		{"unknown page", owner, ActionUpdate, "Doom", map[string]any{"page": "Nobody", "add": 1}, "no page called"},
		{"unknown category", owner, ActionCreate, "New", map[string]any{"every": "Vehicles"}, "no category"},
		{"bad band", owner, ActionCreate, "New", map[string]any{"bands": []any{"high: very"}}, "whole number"},
		{"remove in use", owner, ActionDelete, "Doom", nil, "still on 2 pages"},
		{"nothing to do", owner, ActionUpdate, "Doom", nil, "say what to change"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := k.Plan(context.Background(), camp, tc.a, rec("tracker", tc.action, tc.tr, tc.fields, ""))
			if !strings.Contains(p.Error, tc.want) {
				t.Fatalf("error %q, want it to say %q", p.Error, tc.want)
			}
		})
	}
}

func TestTrackerKind_WarnsWithoutTheBlock(t *testing.T) {
	k, svc := trackerFixture()
	svc.noBlock["p-orrin"] = true
	p := k.Plan(context.Background(), camp, owner, rec("tracker", ActionCreate, "Counsel", map[string]any{"pages": []any{"Orrin"}}, ""))
	if p.Error != "" || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "Trackers block") {
		t.Fatalf("plan %+v", p)
	}
}

// The export plans cleanly when sent back, and the lookup shows a hidden
// tracker's value, marked hidden, to the Director only.
func TestTrackerKind_ExportAndLookup(t *testing.T) {
	k, svc := trackerFixture()
	planApply(t, k, rec("tracker", ActionCreate, "Counsel", map[string]any{"max": 5, "pages": []any{"Orrin"}}, ""))
	out, err := k.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	recs := parseBlocks(out)
	if len(recs) != 1 || !strings.Contains(out, "pages: [Orrin]") {
		t.Fatalf("exported %q", out)
	}
	if p := k.Plan(context.Background(), camp, owner, recs[0]); p.Error != "" {
		t.Fatalf("exported block plans with %q:\n%s", p.Error, out)
	}

	svc.placed["p-orrin"][0].Name = "Counsel"
	svc.placed["p-orrin"][0].Value = 2
	svc.placed["p-orrin"][0].Log = []TrackerLogLine{{Op: "add", Amount: 1, Note: "At the gate", When: "3 Quickening 100"}}
	run := &lookupRun{ctx: context.Background(), campaignID: camp, a: owner, l: &Lookups{Trackers: svc}}
	ans, err := run.trackersLookup("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ans.Text, "- Counsel on Orrin: 2; who sees it: Director only") || !strings.Contains(ans.Text, "add 1 on 3 Quickening 100: At the gate") {
		t.Fatalf("lookup %q", ans.Text)
	}
	run.a = player
	if _, err := run.trackersLookup(""); err == nil {
		t.Fatal("a player read the trackers")
	}
}

// `start` on a new tracker puts it on a page at that value and logs the
// change, as a set on the tracker's own page would, but only within the
// tracker's own ends.
func TestTrackerKind_Start(t *testing.T) {
	t.Run("a count starts on its page", func(t *testing.T) {
		k, svc := trackerFixture()
		p := planApply(t, k, rec("tracker", ActionCreate, "Counsel", map[string]any{"min": 0, "max": 5, "page": "Orrin", "start": 3}, ""))
		if !strings.Contains(p.Summary, "Orrin starts at 3 (Starting value)") {
			t.Fatalf("summary %q", p.Summary)
		}
		if got := svc.placed["p-orrin"]; len(got) != 1 {
			t.Fatalf("placed %+v", got)
		}
		if len(svc.logs) != 1 || svc.logs[0].Op != trackerSet || svc.logs[0].Amount != 3 ||
			svc.logs[0].TrackerID != svc.placed["p-orrin"][0].ID || svc.logs[0].Note != "Starting value" {
			t.Fatalf("log %+v", svc.logs)
		}
	})
	t.Run("a note is kept", func(t *testing.T) {
		k, svc := trackerFixture()
		planApply(t, k, rec("tracker", ActionCreate, "Counsel", map[string]any{"page": "Orrin", "start": 1, "note": "Orrin arrives owing a favour"}, ""))
		if svc.logs[0].Note != "Orrin arrives owing a favour" {
			t.Fatalf("log %+v", svc.logs)
		}
	})
	t.Run("a start at the floor is still logged", func(t *testing.T) {
		k, svc := trackerFixture()
		planApply(t, k, rec("tracker", ActionCreate, "Counsel", map[string]any{"min": 0, "page": "Orrin", "start": 0}, ""))
		if len(svc.logs) != 1 || svc.logs[0].Amount != 0 {
			t.Fatalf("log %+v", svc.logs)
		}
	})
	t.Run("a count with no ends takes any number, negative too", func(t *testing.T) {
		k, svc := trackerFixture()
		planApply(t, k, rec("tracker", ActionCreate, "Debt", map[string]any{"page": "Orrin", "start": -4}, ""))
		if svc.logs[0].Amount != -4 {
			t.Fatalf("log %+v", svc.logs)
		}
	})
	t.Run("steps and clocks start inside their ends", func(t *testing.T) {
		k, svc := trackerFixture()
		planApply(t, k, rec("tracker", ActionCreate, "Mood", map[string]any{"shape": "steps", "steps": []any{"Calm: quiet", "Uneasy: murmurs", "Hostile: blades"}, "page": "Orrin", "start": 2}, ""))
		planApply(t, k, rec("tracker", ActionCreate, "Doom", map[string]any{"shape": "clock", "max": 6, "page": "Saatraaol", "start": 6}, ""))
		if len(svc.logs) != 2 || svc.logs[0].Amount != 2 || svc.logs[1].Amount != 6 {
			t.Fatalf("logs %+v", svc.logs)
		}
	})
	t.Run("a page this paste makes", func(t *testing.T) {
		k, svc := trackerFixture()
		r := rec("tracker", ActionCreate, "Counsel", map[string]any{"page": "Fresh page", "start": 1}, "")
		if p := k.Plan(context.Background(), camp, owner, r); !strings.Contains(p.Error, `no page called "Fresh page"`) {
			t.Fatalf("plan %+v", p)
		}
		r.Made = map[string]bool{"fresh-page": true}
		if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" {
			t.Fatalf("plan %+v", p)
		}
		_ = svc
	})

	refusals := []struct {
		name   string
		action string
		tr     string
		fields map[string]any
		want   string
	}{
		{"with set", ActionCreate, "New", map[string]any{"page": "Orrin", "start": 1, "set": 2}, "can't be combined with `add` or `set`"},
		{"with add", ActionCreate, "New", map[string]any{"page": "Orrin", "start": 1, "add": 1}, "can't be combined with `add` or `set`"},
		{"no page", ActionCreate, "New", map[string]any{"start": 1}, "with `page`"},
		{"on update", ActionUpdate, "Doom", map[string]any{"page": "Orrin", "start": 1}, "first value of a new tracker"},
		{"not a number", ActionCreate, "New", map[string]any{"page": "Orrin", "start": "high"}, "not a number"},
		{"a fraction", ActionCreate, "New", map[string]any{"page": "Orrin", "start": 1.5}, "not a whole number"},
		{"empty", ActionCreate, "New", map[string]any{"page": "Orrin", "start": nil}, "the number the tracker begins at"},
		{"below min", ActionCreate, "New", map[string]any{"min": 1, "max": 5, "page": "Orrin", "start": 0}, "outside what \"New\" allows (1 to 5)"},
		{"above max", ActionCreate, "New", map[string]any{"max": 5, "page": "Orrin", "start": 6}, "(5 or less)"},
		{"below an open max", ActionCreate, "New", map[string]any{"min": 2, "page": "Orrin", "start": 1}, "(2 or more)"},
		{"past the last step", ActionCreate, "New", map[string]any{"shape": "steps", "steps": []any{"A: a", "B: b"}, "page": "Orrin", "start": 2}, "0 to 1, one per step"},
		{"past a clock's segments", ActionCreate, "New", map[string]any{"shape": "clock", "max": 4, "page": "Orrin", "start": 5}, "0 to 4 segments"},
		{"a clock below zero", ActionCreate, "New", map[string]any{"shape": "clock", "max": 4, "page": "Orrin", "start": -1}, "0 to 4 segments"},
		{"an unknown page", ActionCreate, "New", map[string]any{"page": "Nobody", "start": 1}, "no page called"},
	}
	for _, tc := range refusals {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			k, svc := trackerFixture()
			svc.kinds = []TrackerSetup{{ID: "k9", Name: "Doom", Shape: trackerCount, Visibility: trackerHidden}}
			p := k.Plan(context.Background(), camp, owner, rec("tracker", tc.action, tc.tr, tc.fields, ""))
			if !strings.Contains(p.Error, tc.want) {
				t.Fatalf("error %q, want it to say %q", p.Error, tc.want)
			}
		})
	}
	if !strings.Contains((TrackerKind{}).Doc(), "`start`") {
		t.Error("the kind's doc does not describe `start`")
	}
}
