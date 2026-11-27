package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// fakeEntrySvc records what the kind asks of the systems service.
type fakeEntrySvc struct {
	// Of the reading and catalog halves AI records use only Tie, Untie and
	// FieldCatalog (for `in_place_of`); calling any other panics, which would
	// show a record reaching beyond its job.
	systems.EntryReading
	systems.EntryCatalog
	// catalogs are the lists FieldCatalog answers, found by their field.
	catalogs  map[string]*systems.CatalogList
	tied      map[int64]string
	untied    []int64
	tieErr    error
	rows      []systems.SystemEntry
	created   []systems.CreateSystemEntryInput
	updated   map[int64]systems.UpdateSystemEntryInput
	deleted   []int64
	checkErr  error
	lastActor systems.EntryActor
	chars     []systems.EntryCharacter
}

func (f *fakeEntrySvc) List(_ context.Context, _, _ string, _ systems.EntryActor) ([]systems.SystemEntry, error) {
	return f.rows, nil
}
func (f *fakeEntrySvc) FindByName(_ context.Context, _, field, name string, _ systems.EntryActor) (*systems.SystemEntry, error) {
	for i := range f.rows {
		if f.rows[i].FieldKey == field && strings.EqualFold(f.rows[i].Name, name) {
			return &f.rows[i], nil
		}
	}
	return nil, nil
}

// SetCardLook is the owner's Customizer path, never an AI record's.
func (f *fakeEntrySvc) SetCardLook(context.Context, string, systems.EntryActor, int64, string, string) (*systems.SystemEntry, error) {
	panic("an AI record must not set a card look")
}
func (f *fakeEntrySvc) Grants(context.Context, string, systems.EntryActor) ([]systems.EntryGrant, error) {
	return nil, nil
}
func (f *fakeEntrySvc) Grant(context.Context, string, systems.EntryActor, systems.GrantInput) (*systems.EntryGrant, error) {
	return nil, nil
}
func (f *fakeEntrySvc) RevokeGrant(context.Context, string, systems.EntryActor, int64) error {
	return nil
}
func (f *fakeEntrySvc) CanMake(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (f *fakeEntrySvc) CheckCreate(_ context.Context, _ string, _ systems.EntryActor, _ systems.CreateSystemEntryInput) error {
	return f.checkErr
}
func (f *fakeEntrySvc) Create(_ context.Context, _ string, a systems.EntryActor, in systems.CreateSystemEntryInput) (*systems.SystemEntry, error) {
	f.lastActor = a
	f.created = append(f.created, in)
	return &systems.SystemEntry{ID: 100 + int64(len(f.created))}, nil
}
func (f *fakeEntrySvc) FieldCatalog(_ context.Context, _, field string, _ systems.EntryActor) (*systems.CatalogList, error) {
	for _, l := range f.catalogs {
		if l.FieldKey == field {
			return l, nil
		}
	}
	return nil, nil
}
func (f *fakeEntrySvc) Tie(_ context.Context, _ string, _ systems.EntryActor, id int64, slug string) (*systems.SystemEntry, error) {
	if f.tieErr != nil {
		return nil, f.tieErr
	}
	if f.tied == nil {
		f.tied = map[int64]string{}
	}
	f.tied[id] = slug
	return &systems.SystemEntry{ID: id, StandsFor: slug}, nil
}
func (f *fakeEntrySvc) Untie(_ context.Context, _ string, _ systems.EntryActor, id int64) (*systems.SystemEntry, error) {
	f.untied = append(f.untied, id)
	return &systems.SystemEntry{ID: id}, nil
}
func (f *fakeEntrySvc) UpdateTelling(ctx context.Context, campaignID string, a systems.EntryActor, _ int, id int64, in systems.UpdateSystemEntryInput) (*systems.SystemEntry, *systems.TellQuestion, error) {
	e, err := f.Update(ctx, campaignID, a, id, in)
	return e, nil, err
}

func (f *fakeEntrySvc) Update(_ context.Context, _ string, a systems.EntryActor, id int64, in systems.UpdateSystemEntryInput) (*systems.SystemEntry, error) {
	f.lastActor = a
	if f.updated == nil {
		f.updated = map[int64]systems.UpdateSystemEntryInput{}
	}
	f.updated[id] = in
	return &systems.SystemEntry{}, nil
}
func (f *fakeEntrySvc) Delete(_ context.Context, _ string, _ systems.EntryActor, id int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeEntrySvc) Choices(context.Context, string, string) ([]systems.Choice, error) {
	return nil, nil
}
func (f *fakeEntrySvc) FindCharacter(_ context.Context, _, name string) (*systems.EntryCharacter, error) {
	for _, c := range f.chars {
		if strings.EqualFold(c.Name, name) {
			return &c, nil
		}
	}
	return nil, nil
}

func TestSystemEntryKind_Plan(t *testing.T) {
	existing := systems.SystemEntry{ID: 7, FieldKey: "ancestry", Name: "Stoneborn", Properties: map[string]any{"size": "Medium"}}
	tests := []struct {
		name    string
		actor   Actor
		rec     Record
		svcErr  error
		wantErr string // substring, "" = ok
	}{
		{"create ok", owner, rec("system-entry", "create", "Dune Folk", map[string]any{"entry": "ancestry"}, "x"), nil, ""},
		{"player refused", player, rec("system-entry", "create", "Dune Folk", map[string]any{"entry": "ancestry"}, ""), nil, "Directors"},
		{"name required", owner, rec("system-entry", "create", "", map[string]any{"entry": "ancestry"}, ""), nil, "needs a name"},
		{"entry required", owner, rec("system-entry", "create", "X", nil, ""), nil, "entry:"},
		{"create over existing", owner, rec("system-entry", "create", "stoneborn", map[string]any{"entry": "ancestry"}, ""), nil, "already exists"},
		{"service validation surfaces", owner, rec("system-entry", "create", "Y", map[string]any{"entry": "rarity"}, ""), apperror.NewValidation("not a text field"), "not a text field"},
		{"update ok", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "speed": 5}, ""), nil, ""},
		{"update missing", owner, rec("system-entry", "update", "Nobody", map[string]any{"entry": "ancestry"}, ""), nil, "no ancestry entry"},
		{"update with a trait list ok", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "ancestry_points": 3,
			"purchased_traits": []any{map[string]any{"name": "Stone Skin", "cost": 2, "description": "Tough."}}}, ""), nil, ""},
		{"update bad trait list refused", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "purchased_traits": []any{"Stone Skin", map[string]any{"cost": 1}}}, ""), nil, "needs a name"},
		{"update nested detail refused", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "x": map[string]any{"a": 1}}, ""), nil, "needs a name"},
		{"update with an ability ok", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "signature_ability": map[string]any{"name": "Rune", "keywords": []any{"Magic"}, "tier1": "2 damage"}}, ""), nil, ""},
		{"update ability with an unknown part refused", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "signature_ability": map[string]any{"name": "Rune", "damage": "2"}}, ""), nil, "can only have these parts"},
		{"update with a border ok", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "card_border": "fire", "card_colour": "#c4243a"}, ""), nil, ""},
		{"update unknown border refused", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "card_border": "lava"}, ""), nil, "great-rune"},
		{"update bad colour refused", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "card_colour": "red"}, ""), nil, "six hex digits"},
		{"delete ok", owner, rec("system-entry", "delete", "Stoneborn", map[string]any{"entry": "ancestry"}, ""), nil, ""},
		{"delete missing", owner, rec("system-entry", "delete", "Nobody", map[string]any{"entry": "ancestry"}, ""), nil, "no ancestry entry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeEntrySvc{rows: []systems.SystemEntry{existing}, checkErr: tt.svcErr}
			p := SystemEntryKind{Svc: svc}.Plan(context.Background(), camp, tt.actor, tt.rec)
			if tt.wantErr == "" && p.Error != "" {
				t.Fatalf("unexpected error %q", p.Error)
			}
			if tt.wantErr != "" && !strings.Contains(p.Error, tt.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", p.Error, tt.wantErr)
			}
		})
	}
}

func TestSystemEntryKind_Apply(t *testing.T) {
	ctx := context.Background()
	t.Run("create maps keys, details and director flag", func(t *testing.T) {
		svc := &fakeEntrySvc{}
		r := rec("system-entry", "create", "Dune Folk", map[string]any{
			"entry": "ancestry", "summary": "Desert nomads.", "director": true, "size": "Medium", "speed": 6, "kind": "system-entry", "action": "create",
			"reason": "The players met them in the desert.",
		}, "  Lore text.  ")
		if err := (SystemEntryKind{Svc: svc}).Apply(ctx, camp, owner, r); err != nil {
			t.Fatal(err)
		}
		in := svc.created[0]
		if in.FieldKey != "ancestry" || in.Name != "Dune Folk" || in.Summary != "Desert nomads." ||
			in.Description != "Lore text." || in.Visibility != systems.EntryVisibilityDirectors {
			t.Fatalf("create input = %+v", in)
		}
		if len(in.Properties) != 2 || in.Properties["size"] != "Medium" {
			t.Fatalf("reserved keys leaked into properties: %+v", in.Properties)
		}
		if !svc.lastActor.IsDirector {
			t.Fatal("owner not passed as director")
		}
	})
	t.Run("update sends only what the block names, merging details", func(t *testing.T) {
		svc := &fakeEntrySvc{rows: []systems.SystemEntry{{ID: 7, FieldKey: "ancestry", Name: "Stoneborn", Properties: map[string]any{"size": "Medium"}}}}
		r := rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "speed": 5, "rename_to": "Deepkin"}, "")
		if err := (SystemEntryKind{Svc: svc}).Apply(ctx, camp, owner, r); err != nil {
			t.Fatal(err)
		}
		in := svc.updated[7]
		if v, _ := in.Name.Get(); v != "Deepkin" {
			t.Fatalf("name = %q", v)
		}
		if in.Summary.Present() || in.Description.Present() || in.Visibility.Present() {
			t.Fatalf("absent keys were sent: %+v", in)
		}
		props, _ := in.Properties.Get()
		if props["size"] != "Medium" || props["speed"] != 5 {
			t.Fatalf("properties not merged: %+v", props)
		}
	})
	t.Run("delete", func(t *testing.T) {
		svc := &fakeEntrySvc{rows: []systems.SystemEntry{{ID: 7, FieldKey: "ancestry", Name: "Stoneborn"}}}
		r := rec("system-entry", "delete", "Stoneborn", map[string]any{"entry": "ancestry"}, "")
		if err := (SystemEntryKind{Svc: svc}).Apply(ctx, camp, owner, r); err != nil || len(svc.deleted) != 1 || svc.deleted[0] != 7 {
			t.Fatalf("deleted = %v err %v", svc.deleted, err)
		}
	})
	t.Run("player cannot apply", func(t *testing.T) {
		svc := &fakeEntrySvc{}
		r := rec("system-entry", "create", "X", map[string]any{"entry": "ancestry"}, "")
		if err := (SystemEntryKind{Svc: svc}).Apply(ctx, camp, player, r); err == nil || len(svc.created) != 0 {
			t.Fatalf("player apply: err %v created %d", err, len(svc.created))
		}
	})
}

func TestSystemEntryKind_Export(t *testing.T) {
	svc := &fakeEntrySvc{rows: []systems.SystemEntry{
		{FieldKey: "ancestry", Name: "Open", Visibility: systems.EntryVisibilityEveryone},
		{FieldKey: "kit", Name: "Hidden", Visibility: systems.EntryVisibilityDirectors},
	}}
	out, err := (SystemEntryKind{Svc: svc}).Export(context.Background(), camp, owner)
	if err != nil || !strings.Contains(out, "ancestry: Open") || !strings.Contains(out, "kit: Hidden (Directors only)") {
		t.Fatalf("export = %q err %v", out, err)
	}
	if out, _ := (SystemEntryKind{Svc: svc}).Export(context.Background(), camp, player); out != "" {
		t.Fatalf("player export leaked: %q", out)
	}
}

func TestSystemEntryKind_ForCharacter(t *testing.T) {
	ctx := context.Background()
	wren := systems.EntryCharacter{ID: "ent-wren", Name: "Wren", OwnerUserID: "u-tyne"}
	tests := []struct {
		name    string
		fields  map[string]any
		wantErr string
		wantSum string
	}{
		{"named character", map[string]any{"entry": "ancestry", "for": "wren"}, "", "shown only to wren's player"},
		{"unknown character", map[string]any{"entry": "ancestry", "for": "Nobody"}, "no single character page", ""},
		{"both director and for", map[string]any{"entry": "ancestry", "for": "Wren", "director": true}, "either", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeEntrySvc{chars: []systems.EntryCharacter{wren}}
			p := SystemEntryKind{Svc: svc}.Plan(ctx, camp, owner, rec("system-entry", "create", "Fairy", tt.fields, ""))
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error = %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.wantSum) {
				t.Fatalf("plan = %+v, want summary with %q", p, tt.wantSum)
			}
		})
	}

	t.Run("create keeps it to the character", func(t *testing.T) {
		svc := &fakeEntrySvc{chars: []systems.EntryCharacter{wren}}
		r := rec("system-entry", "create", "Fairy", map[string]any{"entry": "ancestry", "for": "Wren"}, "")
		if err := (SystemEntryKind{Svc: svc}).Apply(ctx, camp, owner, r); err != nil {
			t.Fatal(err)
		}
		in := svc.created[0]
		if in.Visibility != systems.EntryVisibilityCharacter || in.ForEntityID != "ent-wren" {
			t.Fatalf("create input = %+v", in)
		}
		if _, isDetail := in.Properties["for"]; isDetail {
			t.Fatal("`for` stored as a detail")
		}
	})

	t.Run("update moves it to the character", func(t *testing.T) {
		svc := &fakeEntrySvc{chars: []systems.EntryCharacter{wren}, rows: []systems.SystemEntry{{ID: 9, FieldKey: "ancestry", Name: "Fairy"}}}
		r := rec("system-entry", "update", "Fairy", map[string]any{"entry": "ancestry", "for": "Wren"}, "")
		if err := (SystemEntryKind{Svc: svc}).Apply(ctx, camp, owner, r); err != nil {
			t.Fatal(err)
		}
		in := svc.updated[9]
		if v, _ := in.Visibility.Get(); v != systems.EntryVisibilityCharacter {
			t.Fatalf("visibility = %q", v)
		}
		if v, _ := in.ForEntityID.Get(); v != "ent-wren" {
			t.Fatalf("forEntityId = %q", v)
		}
	})
}

// The notes the AI reads name both properties and every border key.
func TestSystemEntryKind_DocCardBorders(t *testing.T) {
	doc := SystemEntryKind{}.Doc()
	for _, want := range []string{"card_border", "card_colour", "carved-stone (Carved stone)", "vine (Vine and wood)", "glass-midnight (Midnight glass)", "mechanical (Boiler works)", "kind: system-entry"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the kind's notes lack %q", want)
		}
	}
	if strings.Index(doc, "card_border") > strings.Index(doc, "kind: system-entry") {
		t.Error("the example should stay last")
	}
}

// ancestries is a Director's reading of the ancestry field: two printed
// entries offered, one already replaced by Rootkin, one left out.
func ancestries() map[string]*systems.CatalogList {
	return map[string]*systems.CatalogList{"ancestries": {
		Category: "ancestries", FieldKey: "ancestry", SystemName: "Draw Steel",
		Entries: []systems.CatalogEntry{
			{Slug: "high-elf", Name: "High Elf", Origin: systems.OriginStandard},
			{Slug: "dwarf", Name: "Dwarf", Origin: systems.OriginStandard},
			{Slug: "rootkin", Name: "Rootkin", Origin: systems.OriginRewritten, StandsFor: "wode-elf", StandsForName: "Wode Elf", EntryID: 8},
			// A campaign entry sharing a printed name is never the target.
			{Slug: "orc", Name: "Orc", Origin: systems.OriginCampaign, EntryID: 11},
		},
		LeftOut: []systems.CatalogEntry{{Slug: "revenant", Name: "Revenant", Origin: systems.OriginStandard, LeftOut: true}},
	}}
}

func TestSystemEntryKind_InPlaceOfPlan(t *testing.T) {
	rows := []systems.SystemEntry{
		{ID: 7, FieldKey: "ancestry", Name: "Stoneborn", Status: systems.EntryStatusAccepted},
		{ID: 8, FieldKey: "ancestry", Name: "Rootkin", StandsFor: "wode-elf", StandsForSeen: `{"name":"Wode Elf"}`},
		{ID: 9, FieldKey: "ancestry", Name: "Pip's Folk", Status: systems.EntryStatusSent},
	}
	f := func(extra map[string]any) map[string]any {
		out := map[string]any{"entry": "ancestry"}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	tests := []struct {
		name     string
		actor    Actor
		rec      Record
		wantErr  string // substring, "" = ok
		wantSum  string
		wantWarn bool
	}{
		{"create in place", owner, rec("system-entry", "create", "Sylvan Elf", f(map[string]any{"in_place_of": "high elf"}), ""), "", "new ancestry entry, in place of Draw Steel's High Elf", true},
		{"create in place as a draft", owner, rec("system-entry", "create", "Sylvan Elf", f(map[string]any{"in_place_of": "High Elf", "director": true}), ""), "", "in place of Draw Steel's High Elf", false},
		{"unknown printed name", owner, rec("system-entry", "create", "Sylvan Elf", f(map[string]any{"in_place_of": "Moon Elf"}), ""), `Draw Steel has no ancestry entry called "Moon Elf"`, "", false},
		{"campaign entry is not printed", owner, rec("system-entry", "create", "Sylvan Elf", f(map[string]any{"in_place_of": "Orc"}), ""), "no ancestry entry called", "", false},
		{"left out refused", owner, rec("system-entry", "create", "Sylvan Elf", f(map[string]any{"in_place_of": "Revenant"}), ""), "left out", "", false},
		{"already replaced refused", owner, rec("system-entry", "create", "Sylvan Elf", f(map[string]any{"in_place_of": "Wode Elf"}), ""), "Rootkin already takes Wode Elf's place", "", false},
		{"field without a catalog", owner, rec("system-entry", "create", "Star", map[string]any{"entry": "career", "in_place_of": "Sailor"}, ""), "the game system has no career entry", "", false},
		{"player refused", player, rec("system-entry", "create", "Sylvan Elf", f(map[string]any{"in_place_of": "High Elf"}), ""), "Directors", "", false},
		{"update ties", owner, rec("system-entry", "update", "Stoneborn", f(map[string]any{"in_place_of": "Dwarf"}), ""), "", "changes the entry; takes the place of Dwarf", false},
		{"update already in place", owner, rec("system-entry", "update", "Rootkin", f(map[string]any{"in_place_of": "wode elf"}), ""), "", "already takes the place of Wode Elf", false},
		{"update tied elsewhere refused", owner, rec("system-entry", "update", "Rootkin", f(map[string]any{"in_place_of": "Dwarf"}), ""), "in_place_of: none", "", false},
		{"update unaccepted refused", owner, rec("system-entry", "update", "Pip's Folk", f(map[string]any{"in_place_of": "Dwarf"}), ""), "accept Pip's Folk", "", false},
		{"update unties", owner, rec("system-entry", "update", "Rootkin", f(map[string]any{"in_place_of": "none"}), ""), "", "no longer takes the place of Wode Elf", false},
		{"untie of an untied entry is no change", owner, rec("system-entry", "update", "Stoneborn", f(map[string]any{"in_place_of": "None"}), ""), "", "changes the entry", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeEntrySvc{rows: rows, catalogs: ancestries()}
			p := SystemEntryKind{Svc: svc}.Plan(context.Background(), camp, tt.actor, tt.rec)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.wantSum) {
				t.Fatalf("plan = %+v, want summary with %q", p, tt.wantSum)
			}
			if tt.name == "untie of an untied entry is no change" && p.Summary != "changes the entry" {
				t.Fatalf("summary = %q", p.Summary)
			}
			if got := len(p.Warnings) > 0; got != tt.wantWarn {
				t.Fatalf("warnings = %v, want some: %v", p.Warnings, tt.wantWarn)
			}
		})
	}
}

func TestSystemEntryKind_InPlaceOfApply(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		rows       []systems.SystemEntry
		rec        Record
		tieErr     error
		wantErr    string
		wantTied   map[int64]string
		wantUntied int
		wantUpdate bool
		wantCreate bool
	}{
		{name: "create then tie the new entry",
			rec:      rec("system-entry", "create", "Sylvan Elf", map[string]any{"entry": "ancestry", "in_place_of": "High Elf"}, ""),
			wantTied: map[int64]string{101: "high-elf"}, wantCreate: true},
		{name: "a tie refused after the create says the entry was added",
			rec:    rec("system-entry", "create", "Sylvan Elf", map[string]any{"entry": "ancestry", "in_place_of": "High Elf"}, ""),
			tieErr: apperror.NewConflict("Moonkin already takes this entry's place. Untie it first."), wantErr: "Sylvan Elf was added, but it could not take High Elf's place", wantCreate: true},
		{name: "update with only in_place_of ties without a save",
			rows:     []systems.SystemEntry{{ID: 7, FieldKey: "ancestry", Name: "Stoneborn"}},
			rec:      rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "in_place_of": "Dwarf"}, ""),
			wantTied: map[int64]string{7: "dwarf"}},
		{name: "update with a change saves, then ties",
			rows:     []systems.SystemEntry{{ID: 7, FieldKey: "ancestry", Name: "Stoneborn"}},
			rec:      rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "in_place_of": "Dwarf", "summary": "Deep folk."}, ""),
			wantTied: map[int64]string{7: "dwarf"}, wantUpdate: true},
		{name: "none unties",
			rows: []systems.SystemEntry{{ID: 8, FieldKey: "ancestry", Name: "Rootkin", StandsFor: "wode-elf"}},
			rec:  rec("system-entry", "update", "Rootkin", map[string]any{"entry": "ancestry", "in_place_of": "none"}, ""), wantUntied: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeEntrySvc{rows: tt.rows, catalogs: ancestries(), tieErr: tt.tieErr}
			err := (SystemEntryKind{Svc: svc}).Apply(ctx, camp, owner, tt.rec)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(apperror.SafeMessage(err), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got := len(svc.created) == 1; got != tt.wantCreate {
				t.Fatalf("created = %+v", svc.created)
			}
			if len(svc.created) == 1 {
				if _, stored := svc.created[0].Properties["in_place_of"]; stored {
					t.Fatal("`in_place_of` stored as a detail")
				}
			}
			if got := len(svc.updated) > 0; got != tt.wantUpdate {
				t.Fatalf("updated = %+v, want an update: %v", svc.updated, tt.wantUpdate)
			}
			if len(svc.tied) != len(tt.wantTied) {
				t.Fatalf("tied = %v, want %v", svc.tied, tt.wantTied)
			}
			for id, slug := range tt.wantTied {
				if svc.tied[id] != slug {
					t.Fatalf("tied = %v, want %v", svc.tied, tt.wantTied)
				}
			}
			if len(svc.untied) != tt.wantUntied {
				t.Fatalf("untied = %v", svc.untied)
			}
		})
	}
	t.Run("player cannot tie", func(t *testing.T) {
		svc := &fakeEntrySvc{rows: []systems.SystemEntry{{ID: 7, FieldKey: "ancestry", Name: "Stoneborn"}}, catalogs: ancestries()}
		r := rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "in_place_of": "Dwarf"}, "")
		if err := (SystemEntryKind{Svc: svc}).Apply(ctx, camp, player, r); err == nil || len(svc.tied) != 0 {
			t.Fatalf("player apply: err %v tied %v", err, svc.tied)
		}
	})
}

// The notes tell the AI how to name the printed entry and how to undo it.
func TestSystemEntryKind_DocInPlaceOf(t *testing.T) {
	doc := SystemEntryKind{}.Doc()
	for _, want := range []string{"in_place_of:", "in_place_of: none", "unless `director: true`"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the kind's notes lack %q", want)
		}
	}
}

// The kind's own kit example, read as the review reads a pasted block, saves
// its signature ability as an object of parts with every part kept.
func TestSystemEntryKind_DocKitAbility(t *testing.T) {
	doc := SystemEntryKind{}.Doc()
	at := strings.Index(doc, "A kit with a signature ability:")
	if at < 0 {
		t.Fatal("doc has no kit example")
	}
	blocks := parseBlocks(doc[at:])
	if len(blocks) != 1 {
		t.Fatalf("parsed %d blocks", len(blocks))
	}
	svc := &fakeEntrySvc{}
	if err := (SystemEntryKind{Svc: svc}).Apply(context.Background(), camp, owner, blocks[0]); err != nil {
		t.Fatal(err)
	}
	props := svc.created[0].Properties
	if err := systems.ValidateEntryProperties(props); err != nil {
		t.Fatalf("example refused: %v", err)
	}
	ab, ok := props["signature_ability"].(map[string]any)
	if !ok {
		t.Fatalf("signature_ability = %#v", props["signature_ability"])
	}
	for _, part := range []string{"name", "flavor", "type", "action_type", "keywords", "distance", "target", "power_roll", "tier1", "tier2", "tier3", "effect"} {
		if _, has := ab[part]; !has {
			t.Errorf("part %q lost", part)
		}
	}
	if kw, _ := ab["keywords"].([]any); len(kw) != 4 {
		t.Errorf("keywords = %#v", ab["keywords"])
	}
}
