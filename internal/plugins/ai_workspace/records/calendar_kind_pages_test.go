package records

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakePageResolver knows pages by slug; hidden ones exist but the actor may
// not open them.
type fakePageResolver struct {
	pages  map[string]*entities.Entity
	hidden map[string]bool
}

func (f fakePageResolver) GetBySlug(_ context.Context, c, slug string) (*entities.Entity, error) {
	if e, ok := f.pages[slug]; ok {
		cp := *e
		cp.CampaignID = c
		return &cp, nil
	}
	return nil, errors.New("not found")
}

func (f fakePageResolver) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if !f.hidden[id] {
			out[id] = true
		}
	}
	return out, nil
}

func pageWorld() fakePageResolver {
	return fakePageResolver{
		pages: map[string]*entities.Entity{
			"selune-lore": {ID: "p-selune", Name: "Selune Lore"},
			"age-of-ash":  {ID: "p-ash", Name: "Age of Ash"},
			"secret":      {ID: "p-secret", Name: "Secret"},
		},
		hidden: map[string]bool{"p-secret": true},
	}
}

func pagedCalendar() *calendar.Calendar {
	return &calendar.Calendar{
		ID: "cal", Name: "Harptos",
		Months:   []calendar.Month{{Name: "Hammer", Days: 30}},
		Weekdays: []calendar.Weekday{{Name: "First"}},
		Moons: []calendar.Moon{
			{ID: 7, Name: "Selune", CycleDays: 30, LoreEntityID: strPtr("p-old"), LoreEntityName: "Old Lore"},
			{ID: 8, Name: "Shadow", CycleDays: 9},
		},
		Eras: []calendar.Era{
			{ID: 2, Name: "Dalereckoning", StartYear: 1, LoreEntityID: strPtr("p-old"), LoreEntityName: "Old Lore"},
			{ID: 3, Name: "Quiet Age", StartYear: 500},
			{ID: 4, Name: "Third Age", StartYear: 900},
		},
	}
}

func strPtr(s string) *string { return &s }

func TestCalendarKind_UpdateLorePages(t *testing.T) {
	// Each moon and era says something different about its page.
	r := rec(KindCalendar, ActionUpdate, "", map[string]any{
		"moons": []any{
			map[string]any{"name": "Selune", "page": "Selune Lore"},
			map[string]any{"name": "Shadow", "page": "none"},
			map[string]any{"name": "Tears", "cycle": 12, "page": "Selune Lore"},
		},
		"eras": []any{
			map[string]any{"name": "Dalereckoning", "page": "none"},
			map[string]any{"name": "Quiet Age", "page": "Age of Ash"},
			map[string]any{"name": "Third Age", "color": "#112233"},
			map[string]any{"name": "Fresh", "start_year": 1200, "page": "Age of Ash"},
		},
	}, "")
	f := &fakeStructure{cal: pagedCalendar()}
	k := CalendarKind{Svc: f, Pages: pageWorld()}
	if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" || !strings.Contains(p.Summary, "6 lore pages") {
		t.Fatalf("plan %+v", p)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}

	ms := f.edits[len(f.edits)-1].Moons
	tests := []struct {
		moon    int
		set     bool
		wantID  string // "" is a clear
		comment string
	}{
		{0, true, "p-selune", "a named page replaces the old one"},
		{1, true, "", "none clears"},
		{2, true, "p-selune", "a new moon takes its page"},
	}
	for _, tt := range tests {
		m := ms[tt.moon]
		got := ""
		if m.LoreEntityID != nil {
			got = *m.LoreEntityID
		}
		if m.LoreSet != tt.set || got != tt.wantID {
			t.Errorf("moon %s: set=%v id=%q, want set=%v id=%q (%s)", m.Name, m.LoreSet, got, tt.set, tt.wantID, tt.comment)
		}
	}

	if len(f.eraUps) != 3 || len(f.eras) != 1 {
		t.Fatalf("era updates %d creates %d", len(f.eraUps), len(f.eras))
	}
	if !f.eraUps[0].LoreEntityID.IsNull() {
		t.Errorf("none must clear the era's page: %+v", f.eraUps[0].LoreEntityID)
	}
	if id, ok := f.eraUps[1].LoreEntityID.Get(); !ok || id != "p-ash" {
		t.Errorf("a named page must replace the era's: %+v", f.eraUps[1].LoreEntityID)
	}
	if f.eraUps[2].LoreEntityID.Present() {
		t.Errorf("an era that says nothing about its page must keep it: %+v", f.eraUps[2].LoreEntityID)
	}
	if f.eras[0].LoreEntityID == nil || *f.eras[0].LoreEntityID != "p-ash" {
		t.Errorf("a new era takes its page: %+v", f.eras[0])
	}
}

func TestCalendarKind_UpdateKeepsLoreWhenSilent(t *testing.T) {
	f := &fakeStructure{cal: pagedCalendar()}
	k := CalendarKind{Svc: f, Pages: pageWorld()}
	r := rec(KindCalendar, ActionUpdate, "", map[string]any{
		"moons": []any{map[string]any{"name": "Selune", "cycle": 31}},
	}, "")
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	for _, m := range f.edits[0].Moons {
		if m.LoreSet {
			t.Errorf("moon %s writes its page though the block never named one", m.Name)
		}
	}
}

func TestCalendarKind_PageRefusals(t *testing.T) {
	tests := []struct {
		name  string
		pages PageResolver
		moon  map[string]any
		era   map[string]any
		want  string
	}{
		{"no such page", pageWorld(), map[string]any{"name": "Selune", "page": "Nowhere"}, nil, `moon "Selune": there is no page called "Nowhere"`},
		{"a page the operator cannot open reads the same", pageWorld(), map[string]any{"name": "Selune", "page": "Secret"}, nil, `there is no page called "Secret" that you can open`},
		{"an era's page is held to the same rule", pageWorld(), nil, map[string]any{"name": "Dalereckoning", "page": "Secret"}, `era "Dalereckoning": there is no page called "Secret"`},
		{"an empty page key", pageWorld(), map[string]any{"name": "Selune", "page": ""}, nil, "page needs a page name, or none"},
		{"pages cannot be looked up", nil, map[string]any{"name": "Selune", "page": "Selune Lore"}, nil, "pages can't be looked up"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]any{}
			if tt.moon != nil {
				fields["moons"] = []any{tt.moon}
			}
			if tt.era != nil {
				fields["eras"] = []any{tt.era}
			}
			f := &fakeStructure{cal: pagedCalendar()}
			k := CalendarKind{Svc: f, Pages: tt.pages}
			r := rec(KindCalendar, ActionUpdate, "", fields, "")
			if p := k.Plan(context.Background(), camp, owner, r); !strings.Contains(p.Error, tt.want) {
				t.Errorf("plan error = %q, want it to contain %q", p.Error, tt.want)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err == nil {
				t.Error("apply must refuse what the plan refused")
			}
			if f.applied != 0 || len(f.eraUps) != 0 {
				t.Error("nothing may be written")
			}
		})
	}
}

func TestCalendarKind_CreateWithPages(t *testing.T) {
	fields := newCalendarFields()
	fields["moons"] = []any{map[string]any{"name": "Selune", "cycle": 30, "page": "Selune Lore"}, map[string]any{"name": "Plain", "cycle": 9}}
	fields["eras"] = []any{map[string]any{"name": "Dalereckoning", "start_year": 1, "page": "Age of Ash"}}
	f := &fakeStructure{}
	k := CalendarKind{Svc: f, Pages: pageWorld()}
	r := rec(KindCalendar, ActionCreate, "Harptos", fields, "")
	if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" {
		t.Fatalf("plan %+v", p)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	ir := f.created
	if ir.Moons[0].LoreEntityID == nil || *ir.Moons[0].LoreEntityID != "p-selune" || ir.Moons[1].LoreEntityID != nil {
		t.Errorf("moons %+v", ir.Moons)
	}
	// PrepareImport clears an uploaded era's page; a resolved one must survive it.
	if ir.Eras[0].LoreEntityID == nil || *ir.Eras[0].LoreEntityID != "p-ash" {
		t.Errorf("era %+v", ir.Eras[0])
	}
}

func TestCalendarKind_ExportNamesPages(t *testing.T) {
	f := &fakeStructure{cal: pagedCalendar()}
	out, err := CalendarKind{Svc: f}.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`{name: "Selune", cycle: 30, phase_offset: 0, color: "", page: "Old Lore"}`, `{name: "Shadow", cycle: 9, phase_offset: 0, color: ""}`, `page: "Old Lore"}`} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "page:") != 2 {
		t.Errorf("only moons and eras with a page may name one:\n%s", out)
	}
}

func TestLookups_CalendarShowsPages(t *testing.T) {
	l := calendarWorld()
	c := l.Cal.(*fakeCal)
	c.cal.Moons[0].LoreEntityName = "Selune Lore"
	c.cal.Eras[0].LoreEntityName = "Age of Ash"
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup(lookupWhatCalendar, nil)})[0]
	for _, want := range []string{`lore page "Selune Lore"`, `lore page "Age of Ash"`} {
		if !strings.Contains(ans.Text, want) {
			t.Errorf("answer lacks %q:\n%s", want, ans.Text)
		}
	}
}
