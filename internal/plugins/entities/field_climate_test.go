package entities

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeClimates is the calendar's side of a climate field.
type fakeClimates struct{}

func (fakeClimates) Climates() []ClimateChoice {
	return []ClimateChoice{{ID: "temperate", Name: "Temperate"}, {ID: "cold-coast", Name: "Cold coast"}, {ID: "ley-storm", Name: "Ley storm", Magic: true}}
}
func (fakeClimates) World(context.Context, string, int, string) (ClimateChoice, bool) {
	return ClimateChoice{ID: "temperate", Name: "Temperate"}, true
}
func (fakeClimates) Today(context.Context, string, string, int, string) *ClimateToday { return nil }

func TestCheckClimateValue(t *testing.T) {
	d := FieldDefinition{Key: "climate", Label: "Climate", Type: FieldTypeClimate}
	choices := fakeClimates{}.Climates()
	tests := []struct {
		name    string
		v       any
		choices []ClimateChoice
		wantErr string
	}{
		{"one of the calendar's climates", "cold-coast", choices, ""},
		{"a magic climate", "ley-storm", choices, ""},
		{"empty means the same as around it", "", choices, ""},
		{"spaces only count as empty", "  ", choices, ""},
		{"a climate the calendar does not have", "lava-lake", choices, "Climate must be one of the calendar's climates: temperate, cold-coast, ley-storm"},
		{"anything is kept when no calendar is wired", "lava-lake", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkClimateValue(d, tt.v, tt.choices)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestCheckLinkValues_Climate(t *testing.T) {
	et := &EntityType{ID: 3, CampaignID: "camp", Fields: []FieldDefinition{{Key: "climate", Label: "Climate", Type: FieldTypeClimate}}}
	types := &mockEntityTypeRepo{findByIDFn: func(context.Context, int) (*EntityType, error) { return et, nil }}
	page := &Entity{ID: "p", CampaignID: "camp", EntityTypeID: 3}
	tests := []struct {
		name       string
		oldD, newD map[string]any
		wantErr    bool
	}{
		{"a known climate", nil, map[string]any{"climate": "cold-coast"}, false},
		{"an unknown climate", nil, map[string]any{"climate": "lava-lake"}, true},
		{"an unknown climate already stored is not re-checked", map[string]any{"climate": "lava-lake"}, map[string]any{"climate": "lava-lake"}, false},
		{"clearing it returns the place to what is around it", map[string]any{"climate": "cold-coast"}, map[string]any{"climate": ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &entityService{types: types, entities: &mockEntityRepo{}, fieldClimate: fakeClimates{}}
			err := s.checkLinkValues(context.Background(), page, tt.oldD, tt.newD)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckFieldKinds_ClimateClearsStraySettings(t *testing.T) {
	s := &entityService{types: &mockEntityTypeRepo{}}
	fields := []FieldDefinition{{Key: "climate", Label: "Climate", Type: FieldTypeClimate, LinkTo: 3, RuleID: 7, DateAgo: true, Options: []string{"a"},
		Countdowns: []FieldCountdown{{Name: "x"}}}}
	if err := s.checkFieldKinds(context.Background(), "camp", fields); err != nil {
		t.Fatal(err)
	}
	f := fields[0]
	if f.LinkTo != 0 || f.RuleID != 0 || f.DateAgo || len(f.Options) != 0 || len(f.Countdowns) != 0 {
		t.Fatalf("stray settings kept: %+v", f)
	}
}

// climateWorld is a Location category with a Climate field and a Region
// category without one: Saltreach (Cold coast) holds the town of Gull Rock,
// which holds the Harbour; the Wilds have no climate above them.
func climateWorld() (*mockEntityRepo, *mockEntityTypeRepo) {
	loc := &EntityType{ID: 1, CampaignID: "camp", Slug: "location", Fields: []FieldDefinition{{Key: "climate", Label: "Climate", Type: FieldTypeClimate}}}
	other := &EntityType{ID: 2, CampaignID: "camp", Slug: "region"}
	types := &mockEntityTypeRepo{findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
		switch id {
		case 1:
			return loc, nil
		case 2:
			return other, nil
		}
		return nil, errors.New("not found")
	}}
	salt := "salt"
	pages := map[string][]Entity{
		"gull":    {{ID: "salt", Name: "Saltreach", CampaignID: "camp", EntityTypeID: 1, FieldsData: map[string]any{"climate": "cold-coast"}}},
		"harbour": {{ID: "gull", Name: "Gull Rock", CampaignID: "camp", EntityTypeID: 1, ParentID: &salt}, {ID: "salt", Name: "Saltreach", CampaignID: "camp", EntityTypeID: 1, FieldsData: map[string]any{"climate": "cold-coast"}}},
		"stray":   {{ID: "x", Name: "Elsewhere", CampaignID: "other", EntityTypeID: 1, FieldsData: map[string]any{"climate": "tundra"}}},
		"wilds":   {{ID: "top", Name: "Top", CampaignID: "camp", EntityTypeID: 2}},
	}
	ents := &mockEntityRepo{findAncestorsFn: func(_ context.Context, id string, _ int, _ string) ([]Entity, error) {
		return pages[id], nil
	}}
	return ents, types
}

func TestClimateRegion(t *testing.T) {
	ents, types := climateWorld()
	s := &entityService{types: types, entities: ents}
	salt := "salt"
	tests := []struct {
		name        string
		e           *Entity
		wantRegion  string
		wantClimate string
	}{
		{"a page with its own climate is its own region", &Entity{ID: "salt", CampaignID: "camp", EntityTypeID: 1, FieldsData: map[string]any{"climate": "cold-coast"}}, "salt", "cold-coast"},
		{"a town takes its parent's", &Entity{ID: "gull", CampaignID: "camp", EntityTypeID: 1, ParentID: &salt}, "salt", "cold-coast"},
		{"a page deeper down takes the nearest above with one", &Entity{ID: "harbour", CampaignID: "camp", EntityTypeID: 1}, "salt", "cold-coast"},
		{"an ancestor of another campaign never counts", &Entity{ID: "stray", CampaignID: "camp", EntityTypeID: 1}, "", ""},
		{"a category with no climate field and nothing above", &Entity{ID: "wilds", CampaignID: "camp", EntityTypeID: 2}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region, climate, err := s.ClimateRegion(context.Background(), tt.e)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if region != nil {
				got = region.ID
			}
			if got != tt.wantRegion || climate != tt.wantClimate {
				t.Fatalf("region %q climate %q, want %q %q", got, climate, tt.wantRegion, tt.wantClimate)
			}
		})
	}
}

func TestClimatePlaces_WalksEveryPage(t *testing.T) {
	types := &mockEntityTypeRepo{listByCampaignFn: func(context.Context, string) ([]EntityType, error) {
		return []EntityType{
			{ID: 1, Fields: []FieldDefinition{{Key: "clim", Type: FieldTypeClimate}}},
			{ID: 2, Fields: []FieldDefinition{{Key: "notes", Type: "text"}}},
		}, nil
	}}
	var asked [][]int
	parent := "salt"
	ents := &mockEntityRepo{listByCampaignFn: func(_ context.Context, _ string, ids []int, _ int, _ string, opts ListOptions) ([]Entity, int, error) {
		asked = append(asked, ids)
		total := climatePlacesPage + 2
		if opts.Page == 1 {
			out := make([]Entity, climatePlacesPage)
			for i := range out {
				out[i] = Entity{ID: "blank", EntityTypeID: 1}
			}
			out[0] = Entity{ID: "salt", Name: "Saltreach", EntityTypeID: 1, FieldsData: map[string]any{"clim": " cold-coast "}}
			return out, total, nil
		}
		return []Entity{
			{ID: "gull", Name: "Gull Rock", EntityTypeID: 1, ParentID: &parent, FieldsData: map[string]any{"clim": "tundra"}},
			{ID: "tpl", Name: "Template", EntityTypeID: 1, IsTemplate: true, FieldsData: map[string]any{"clim": "desert"}},
		}, total, nil
	}}
	s := &entityService{types: types, entities: ents}
	got, err := s.ClimatePlaces(context.Background(), "camp")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (PlaceClimate{ID: "salt", Name: "Saltreach", Climate: "cold-coast"}) ||
		got[1] != (PlaceClimate{ID: "gull", Name: "Gull Rock", ParentID: "salt", Climate: "tundra"}) {
		t.Fatalf("places %+v", got)
	}
	if len(asked) != 2 || len(asked[0]) != 1 || asked[0][0] != 1 {
		t.Fatalf("asked %v, want two pages of category 1 only", asked)
	}
}

func TestEnsureClimateFields(t *testing.T) {
	stored := map[int]*EntityType{
		1: {ID: 1, CampaignID: "a", Slug: "location", Name: "Location", NamePlural: "Locations", Icon: "fa-map", Color: "#5b8def", Fields: []FieldDefinition{{Key: "region", Label: "Region", Type: "text", Section: "Where"}}},
		2: {ID: 2, CampaignID: "b", Slug: "location", Name: "Location", Fields: []FieldDefinition{{Key: "weather", Label: "Weather", Type: FieldTypeClimate}}},
		3: {ID: 3, CampaignID: "c", Slug: "location", Name: "Location", Fields: []FieldDefinition{{Key: "climate", Label: "Climate", Type: "text"}}},
		4: {ID: 4, CampaignID: "a", Slug: "character", Name: "Character"},
	}
	updated := map[int][]FieldDefinition{}
	types := &mockEntityTypeRepo{
		listAllFn: func(context.Context) ([]EntityType, error) {
			out := []EntityType{}
			for i := 1; i <= 4; i++ {
				out = append(out, *stored[i])
			}
			return out, nil
		},
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			cp := *stored[id]
			cp.Fields = append([]FieldDefinition(nil), stored[id].Fields...)
			return &cp, nil
		},
		updateFn: func(_ context.Context, et *EntityType) error { updated[et.ID] = et.Fields; return nil },
	}
	svc := newTestService(&mockEntityRepo{}, types)
	n, err := svc.(*entityService).EnsureClimateFields(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(updated) != 1 {
		t.Fatalf("changed %d, updated %v; want only the Location without a climate", n, updated)
	}
	f := updated[1]
	if len(f) != 2 || f[1].Key != "climate" || f[1].Type != FieldTypeClimate || f[1].Section != "Where" {
		t.Fatalf("fields %+v", f)
	}
}

// A category that fails does not stop the others, and the sweep reports it,
// so the once-per-site marker is not written and the next start tries again.
func TestEnsureClimateFieldsReportsAFailedCategory(t *testing.T) {
	stored := map[int]*EntityType{
		1: {ID: 1, CampaignID: "a", Slug: "location", Name: "Location", NamePlural: "Locations", Icon: "fa-map", Color: "#5b8def"},
		2: {ID: 2, CampaignID: "b", Slug: "location", Name: "Location", NamePlural: "Locations", Icon: "fa-map", Color: "#5b8def"},
	}
	updated := map[int]bool{}
	types := &mockEntityTypeRepo{
		listAllFn: func(context.Context) ([]EntityType, error) { return []EntityType{*stored[1], *stored[2]}, nil },
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			cp := *stored[id]
			return &cp, nil
		},
		updateFn: func(_ context.Context, et *EntityType) error {
			if et.ID == 1 {
				return errors.New("db down")
			}
			updated[et.ID] = true
			return nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, types)
	n, err := svc.(*entityService).EnsureClimateFields(context.Background())
	if err == nil {
		t.Fatal("a failed category must be reported")
	}
	if n != 1 || !updated[2] {
		t.Fatalf("changed %d, updated %v; the other Location should still get the field", n, updated)
	}
}

// climateSvc answers the reads the climate line makes: the region, whether
// the viewer may open it, and whether the page has pages under it.
type climateSvc struct {
	EntityService
	region   *Entity
	climate  string
	canView  bool
	children int
}

func (c climateSvc) ClimateRegion(context.Context, *Entity) (*Entity, string, error) {
	return c.region, c.climate, nil
}
func (c climateSvc) CheckEntityAccess(context.Context, string, int, string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: c.canView}, nil
}
func (c climateSvc) GetChildren(context.Context, string, int, string) ([]Entity, error) {
	return make([]Entity, c.children), nil
}

// todayFor records the region the line asks the weather of.
type todayFor struct {
	fakeClimates
	asked *string
}

func (f todayFor) Today(_ context.Context, _, regionID string, _ int, _ string) *ClimateToday {
	*f.asked = regionID
	return &ClimateToday{Date: "Firstfrost 8", Words: "Light rain, 6°"}
}

func TestClimateShown(t *testing.T) {
	salt := &Entity{ID: "salt", Name: "Saltreach", CampaignID: "camp"}
	gull := &Entity{ID: "gull", Name: "Gull Rock", CampaignID: "camp"}
	tests := []struct {
		name     string
		svc      climateSvc
		page     *Entity
		wantOK   bool
		wantFrom string
		own      bool
		covers   bool
	}{
		{"no climate here or above: no line", climateSvc{}, gull, false, "", false, false},
		{"its own climate, with pages inside", climateSvc{region: salt, climate: "cold-coast", children: 2}, salt, true, "", true, true},
		{"its own climate, nothing inside", climateSvc{region: salt, climate: "cold-coast"}, salt, true, "", true, false},
		{"a parent's, which the viewer may open", climateSvc{region: salt, climate: "cold-coast", canView: true}, gull, true, "Saltreach", false, false},
		{"a parent's the viewer may not open is never named", climateSvc{region: salt, climate: "cold-coast"}, gull, true, "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asked := ""
			h := &Handler{service: tt.svc, fieldClimate: todayFor{asked: &asked}}
			got, ok := h.climateShown(context.Background(), "camp", tt.page, 1, "u1")
			if ok != tt.wantOK {
				t.Fatalf("ok = %v", ok)
			}
			if !ok {
				return
			}
			c := got.Climate
			if got.Text != "Cold coast" || c.Own != tt.own || c.Covers != tt.covers || c.From != tt.wantFrom {
				t.Fatalf("shown %+v %+v", got, c)
			}
			if c.Covers != (c.Here == "Saltreach") {
				t.Errorf("covers %v names %q", c.Covers, c.Here)
			}
			if (c.FromHref != "") != (tt.wantFrom != "") {
				t.Errorf("link %q with name %q", c.FromHref, c.From)
			}
			if asked != "salt" || c.Today == nil || c.Today.Words != "Light rain, 6°" {
				t.Errorf("weather asked of %q, got %+v", asked, c.Today)
			}
		})
	}
}
