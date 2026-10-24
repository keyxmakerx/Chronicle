package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeEncounters is EncountersAPI over a map of page id -> encounters.
type fakeEncounters struct {
	byPage  map[string][]EncounterRef
	created []EncounterInput
	updated []EncounterInput
	version int
	deleted []string
}

func (f *fakeEncounters) ListForPage(_ context.Context, _, pageID string, _ Actor) ([]EncounterRef, error) {
	return f.byPage[pageID], nil
}
func (f *fakeEncounters) Create(_ context.Context, _, _ string, _ Actor, in EncounterInput) error {
	f.created = append(f.created, in)
	return nil
}
func (f *fakeEncounters) Update(_ context.Context, _, _ string, _ Actor, in EncounterInput, version int) error {
	f.updated, f.version = append(f.updated, in), version
	return nil
}
func (f *fakeEncounters) Delete(_ context.Context, _, id string, _ Actor) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeEncounters) All(context.Context, string, Actor) ([]EncounterRef, error) {
	var out []EncounterRef
	for _, l := range f.byPage {
		out = append(out, l...)
	}
	return out, nil
}

func encounterPages() fakeEntities {
	mk := func(id, name string) *entities.Entity {
		return &entities.Entity{ID: id, CampaignID: camp, Name: name, Slug: entities.Slugify(name)}
	}
	out := fakeEntities{}
	for _, e := range []*entities.Entity{
		mk("page1", "The Siege of Harrowmere"), mk("h1", "Ash"), mk("h2", "Bryn"),
		mk("gob", "Goblin War Chief"), mk("hob", "Hobgoblin Soldier"),
	} {
		out[e.Slug] = e
	}
	return out
}

func encounterRecord(action string, fields map[string]any) Record {
	f := map[string]any{"page": "The Siege of Harrowmere"}
	for k, v := range fields {
		f[k] = v
	}
	return Record{Kind: "encounter", Action: action, Name: "Ramparts fight", Fields: f}
}

func TestParseCreature(t *testing.T) {
	tests := []struct {
		in      any
		want    EncounterCreature
		wantErr string
	}{
		{in: "Goblin War Chief, group A, starts hidden", want: EncounterCreature{Name: "Goblin War Chief", Count: 1, Group: "A", Start: "hidden"}},
		{in: "2 Hobgoblin Soldier, group b", want: EncounterCreature{Name: "Hobgoblin Soldier", Count: 2, Group: "B", Start: "seen"}},
		{in: "4x Skeleton Archer, Group  C, arrives later", want: EncounterCreature{Name: "Skeleton Archer", Count: 4, Group: "C", Start: "later"}},
		{in: "Rat", want: EncounterCreature{Name: "Rat", Count: 1, Group: "A", Start: "seen"}},
		{in: map[string]any{"name": "Rat", "count": 3, "group": "D", "start": "hidden"}, want: EncounterCreature{Name: "Rat", Count: 3, Group: "D", Start: "hidden"}},
		{in: "Rat, group Z", wantErr: "not a group"},
		{in: "Rat, revealed", wantErr: "not a group"},
		{in: "41 Rat", wantErr: "1 to 40"},
		{in: ", group A", wantErr: "no name"},
		{in: nil, wantErr: "empty"},
	}
	for _, tc := range tests {
		got, err := parseCreature(tc.in)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%v: want error %q, got %v", tc.in, tc.wantErr, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%v: got %+v %v, want %+v", tc.in, got, err, tc.want)
		}
	}
}

func TestEncounterKind_Plan(t *testing.T) {
	existing := EncounterRef{ID: "e1", Name: "Ramparts fight", PageName: "The Siege of Harrowmere", Version: 4, HeroIDs: []string{"h1"},
		Creatures: []EncounterCreature{{EntityID: "gob", Name: "Goblin War Chief", Count: 1, Group: "A", Start: "hidden"}}}
	creatures := []any{"Goblin War Chief, group A, starts hidden", "2 Hobgoblin Soldier, group B", "4 Skeleton Archer, group C, arrives later"}
	tests := []struct {
		name     string
		actor    Actor
		action   string
		fields   map[string]any
		existing bool
		made     []string // slugs of pages this paste makes
		wantErr  string
		wantSum  string
		wantWarn string
	}{
		{name: "player refused", actor: player, action: ActionCreate, wantErr: "owner or a co-DM"},
		{name: "needs a page", action: ActionCreate, fields: map[string]any{"page": ""}, wantErr: "needs `page`"},
		{name: "create", action: ActionCreate, fields: map[string]any{"creatures": creatures, "heroes": []any{"Ash", "Bryn"}},
			wantSum: "7 creatures, 1 start hidden, 4 arrive later, for 2 heroes", wantWarn: `"Skeleton Archer" is not a page yet`},
		{name: "create twice", action: ActionCreate, existing: true, wantErr: "use action: update"},
		{name: "page made in this paste", action: ActionCreate, fields: map[string]any{"page": "New Trial"}, made: []string{"new-trial"}, wantSum: "0 creatures"},
		{name: "page nowhere", action: ActionCreate, fields: map[string]any{"page": "New Trial"}, wantErr: `no page called "New Trial"; add it to this paste`},
		{name: "update keeps creatures", action: ActionUpdate, existing: true, fields: map[string]any{"ready": true}, wantSum: "1 creature, 1 start hidden, for 1 hero"},
		{name: "update missing", action: ActionUpdate, wantErr: "no encounter called"},
		{name: "update on a missing page", action: ActionUpdate, fields: map[string]any{"page": "Nowhere"}, wantErr: "no page called"},
		{name: "delete", action: ActionDelete, existing: true, wantSum: "removes the encounter"},
		{name: "bad line", action: ActionCreate, fields: map[string]any{"creatures": []any{"Rat, group Q"}}, wantErr: "not a group"},
		{name: "missing hero", action: ActionCreate, fields: map[string]any{"heroes": []any{"Zed"}}, wantErr: `no hero page called "Zed"`},
		{name: "hero made in this paste", action: ActionCreate, fields: map[string]any{"heroes": []any{"Zed"}}, made: []string{"zed"}, wantSum: "0 creatures, for 1 hero"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeEncounters{byPage: map[string][]EncounterRef{}}
			if tc.existing {
				svc.byPage["page1"] = []EncounterRef{existing}
			}
			a := tc.actor
			if a.UserID == "" {
				a = owner
			}
			k := EncounterKind{Svc: svc, Entities: encounterPages()}
			r := encounterRecord(tc.action, tc.fields)
			r.Made = map[string]bool{}
			for _, s := range tc.made {
				r.Made[s] = true
			}
			p := k.Plan(context.Background(), camp, a, r)
			if tc.wantErr != "" {
				if !strings.Contains(p.Error, tc.wantErr) {
					t.Fatalf("want error %q, got %+v", tc.wantErr, p)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("unexpected error %q", p.Error)
			}
			if tc.wantSum != "" && p.Summary != tc.wantSum {
				t.Errorf("summary %q, want %q", p.Summary, tc.wantSum)
			}
			if tc.wantWarn != "" && !strings.Contains(strings.Join(p.Warnings, "|"), tc.wantWarn) {
				t.Errorf("warnings %q, want %q", p.Warnings, tc.wantWarn)
			}
		})
	}
}

func TestEncounterKind_Apply(t *testing.T) {
	ctx := context.Background()
	k := func(svc *fakeEncounters) EncounterKind { return EncounterKind{Svc: svc, Entities: encounterPages()} }

	svc := &fakeEncounters{byPage: map[string][]EncounterRef{}}
	err := k(svc).Apply(ctx, camp, owner, encounterRecord(ActionCreate, map[string]any{
		"ready": true, "heroes": []any{"Ash"}, "creatures": []any{"Goblin War Chief, starts hidden", "3 Kobold"},
	}))
	if err != nil || len(svc.created) != 1 {
		t.Fatalf("create: %v %+v", err, svc.created)
	}
	c := svc.created[0]
	if !c.Ready || len(c.HeroIDs) != 1 || c.HeroIDs[0] != "h1" {
		t.Errorf("create input: %+v", c)
	}
	if c.Creatures[0].EntityID != "gob" || c.Creatures[0].Start != "hidden" || c.Creatures[1].EntityID != "" || c.Creatures[1].Count != 3 {
		t.Errorf("creatures: %+v", c.Creatures)
	}

	// A hero page still missing at commit is refused rather than dropped.
	if err := k(svc).Apply(ctx, camp, owner, encounterRecord(ActionCreate, map[string]any{"heroes": []any{"Zed"}})); err == nil {
		t.Error("a missing hero must refuse the row")
	}

	// Update: left-out keys keep what is there; the stored version is sent.
	svc = &fakeEncounters{byPage: map[string][]EncounterRef{"page1": {{ID: "e1", Name: "Ramparts fight", Version: 4, Ready: true,
		HeroIDs: []string{"h1"}, Creatures: []EncounterCreature{{EntityID: "gob", Name: "Goblin War Chief", Count: 1, Group: "A", Start: "hidden"}}}}}}
	if err := k(svc).Apply(ctx, camp, owner, encounterRecord(ActionUpdate, map[string]any{"rename_to": "Wall fight", "ready": false})); err != nil {
		t.Fatal(err)
	}
	u := svc.updated[0]
	if u.Name != "Wall fight" || u.Ready || len(u.Creatures) != 1 || len(u.HeroIDs) != 1 || svc.version != 4 {
		t.Errorf("update input: %+v v%d", u, svc.version)
	}

	if err := k(svc).Apply(ctx, camp, owner, encounterRecord(ActionDelete, nil)); err != nil || svc.deleted[0] != "e1" {
		t.Fatalf("delete: %v %v", err, svc.deleted)
	}
	if err := k(svc).Apply(ctx, camp, player, encounterRecord(ActionDelete, nil)); err == nil {
		t.Error("a player must be refused at commit too")
	}
}

// The export is in the kind's own format, so it parses back to the same rows.
func TestEncounterKind_ExportRoundTrips(t *testing.T) {
	svc := &fakeEncounters{byPage: map[string][]EncounterRef{"page1": {{ID: "e1", Name: "Ramparts fight", PageName: "The Siege of Harrowmere", Ready: true,
		Heroes: []string{"Ash"}, Creatures: []EncounterCreature{{Name: "Skeleton Archer", Count: 4, Group: "C", Start: "later"}}}}}}
	out, err := EncounterKind{Svc: svc}.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: encounter", `page: "The Siege of Harrowmere"`, "ready: true", `heroes: ["Ash"]`, `"4 Skeleton Archer, group C, arrives later"`} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	c, err := parseCreature("4 Skeleton Archer, group C, arrives later")
	if err != nil || c != (EncounterCreature{Name: "Skeleton Archer", Count: 4, Group: "C", Start: "later"}) {
		t.Errorf("round trip: %+v %v", c, err)
	}
	if s, _ := (EncounterKind{Svc: svc}).Export(context.Background(), camp, player); s != "" {
		t.Error("a player's export must be empty")
	}
}

// A creature, hero or page this paste creates is not warned about at
// review: pages commit before records, so it exists when the encounter does.
func TestEncounterKind_PlanMadeInPaste(t *testing.T) {
	r := encounterRecord(ActionCreate, map[string]any{
		"page": "New Trial", "heroes": []any{"Zed"}, "creatures": []any{"2 Skeleton Archer, group C"},
	})
	r.Made = map[string]bool{"new-trial": true, "zed": true, "skeleton-archer": true}
	k := EncounterKind{Svc: &fakeEncounters{byPage: map[string][]EncounterRef{}}, Entities: encounterPages()}
	p := k.Plan(context.Background(), camp, owner, r)
	if p.Error != "" || len(p.Warnings) != 0 {
		t.Fatalf("want a clean plan, got error %q warnings %q", p.Error, p.Warnings)
	}
}
