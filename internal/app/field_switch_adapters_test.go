package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// switchManifest is a system whose item preset reaches campaigns only through
// the owner's yes, beside an ordinary preset.
func switchManifest() *systems.SystemManifest {
	return &systems.SystemManifest{ID: "drawsteel", Name: "Draw Steel", EntityPresets: []systems.EntityPresetDef{
		{Slug: "drawsteel-creature", Name: "Creature", NamePlural: "Creatures", Category: "creature",
			Fields: []systems.FieldDef{{Key: "level", Label: "Level", Type: "number"}}},
		{Slug: "drawsteel-item", Name: "Item", NamePlural: "Items", Category: "item",
			Fields: []systems.FieldDef{
				{Key: "type", Label: "Type", Type: "enum", Options: []string{"Consumable", "Trinket"}},
				{Key: "kind", Label: "Kind", Type: "enum", Options: []string{"Weapon", "Other"}},
			},
			Switch: &systems.PresetSwitchDef{Label: "treasure fields", SetAside: []string{"rarity"}, Reuse: []string{"type"}}},
	}}
}

// fakeSwitcher records what the applier asked of the field switch.
type fakeSwitcher struct {
	states   map[int]*entities.FieldSwitch
	stateErr error
	accepted []int
	recorded []int
	offer    entities.FieldSwitchOffer
}

func (f *fakeSwitcher) State(_ context.Context, id int) (*entities.FieldSwitch, error) {
	if f.stateErr != nil {
		return nil, f.stateErr
	}
	return f.states[id], nil
}

func (f *fakeSwitcher) AcceptOffer(_ context.Context, et *entities.EntityType, offer entities.FieldSwitchOffer, _ string) (int, error) {
	f.accepted = append(f.accepted, et.ID)
	f.offer = offer
	return 0, nil
}

func (f *fakeSwitcher) RecordSwitched(_ context.Context, et *entities.EntityType, _ entities.FieldSwitchOffer) error {
	f.recorded = append(f.recorded, et.ID)
	return nil
}

// switchEntities adds page counts to the preset fake.
type switchEntities struct {
	*fakePresetEntities
	counts map[int]int
}

func (s switchEntities) CountByType(context.Context, string, int, string) (map[int]int, error) {
	return s.counts, nil
}

func TestApplyPresets_SwitchPresetWaitsForTheOwner(t *testing.T) {
	item := "item"
	seeded := []entities.EntityType{{ID: 4, Name: "Item", Slug: "item"}}
	tests := []struct {
		name         string
		types        []entities.EntityType
		pages        int
		state        *entities.FieldSwitch
		create       bool
		noSwitcher   bool
		wantAccepted bool
		wantRecorded bool
		wantCreated  int
		wantMerged   bool // the preset's fields were merged into the type
	}{
		{name: "turning the system on, Items has pages: only the offer", types: seeded, pages: 3, create: true},
		{name: "turning the system on, Items is empty: switched with no offer", types: seeded, create: true, wantAccepted: true},
		{name: "a package update never switches, even an empty Items", types: seeded},
		{name: "a package update leaves an unswitched Items alone", types: seeded, pages: 2},
		{name: "Not now is kept when the system is turned on again", types: seeded, create: true,
			state: &entities.FieldSwitch{State: entities.FieldSwitchDismissed}},
		{name: "switched back is kept when the system is turned on again", types: seeded, create: true,
			state: &entities.FieldSwitch{State: entities.FieldSwitchBack}},
		{name: "a switched Items takes a package update's new fields",
			types: []entities.EntityType{{ID: 4, Name: "Item", Slug: "item", PresetCategory: &item}}, pages: 5,
			state: &entities.FieldSwitch{State: entities.FieldSwitchSwitched}, wantMerged: true},
		{name: "a campaign with no Items gets them made with the fields", types: nil, create: true,
			wantCreated: 1, wantRecorded: true},
		{name: "no Items and an update: nothing is made", types: nil},
		{name: "an Item sub-category is the GM's own, never made over",
			types: []entities.EntityType{{ID: 8, Name: "Item", Slug: "item", ParentTypeID: new(int)}}, create: true},
		{name: "without the switch wired the preset is skipped", types: seeded, create: true, noSwitcher: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := &fakePresetEntities{types: tc.types}
			ents := switchEntities{fakePresetEntities: base, counts: map[int]int{4: tc.pages}}
			sw := &fakeSwitcher{states: map[int]*entities.FieldSwitch{4: tc.state}}
			p := newPresetApplier(ents)
			if !tc.noSwitcher {
				p.withFieldSwitch(sw)
			}
			if _, err := p.applyManifestPresets(context.Background(), "c1", "drawsteel", switchManifest(), tc.create, nil); err != nil {
				t.Fatal(err)
			}
			if (len(sw.accepted) > 0) != tc.wantAccepted {
				t.Errorf("accepted = %v, want %v", sw.accepted, tc.wantAccepted)
			}
			if (len(sw.recorded) > 0) != tc.wantRecorded {
				t.Errorf("recorded = %v, want %v", sw.recorded, tc.wantRecorded)
			}
			var itemCreated int
			for _, c := range base.created {
				if c.PresetCategory == "item" {
					itemCreated++
				}
			}
			if itemCreated != tc.wantCreated {
				t.Errorf("item types created = %d, want %d", itemCreated, tc.wantCreated)
			}
			merged := false
			for _, id := range base.reconciled {
				if id == 4 || id == 8 {
					merged = true
				}
			}
			if merged != tc.wantMerged {
				t.Errorf("merged into the item type = %v, want %v (reconciled %v)", merged, tc.wantMerged, base.reconciled)
			}
		})
	}
}

func TestApplyPresets_SwitchOfferCarriesTheManifest(t *testing.T) {
	base := &fakePresetEntities{types: []entities.EntityType{{ID: 4, Name: "Item", Slug: "item"}}}
	sw := &fakeSwitcher{states: map[int]*entities.FieldSwitch{}}
	p := newPresetApplier(switchEntities{fakePresetEntities: base, counts: map[int]int{}}).withFieldSwitch(sw)
	if _, err := p.applyManifestPresets(context.Background(), "c1", "drawsteel", switchManifest(), true, nil); err != nil {
		t.Fatal(err)
	}
	o := sw.offer
	if o.SystemName != "Draw Steel" || o.Category != "item" || o.Label != "treasure fields" || len(o.Fields) != 2 {
		t.Fatalf("offer = %+v", o)
	}
	if o.Fields[0].Type != "select" || len(o.Fields[0].Options) != 2 {
		t.Errorf("the pick list reaches the stored field: %+v", o.Fields[0])
	}
}

func TestApplyPresets_SwitchStateReadFailureChangesNothing(t *testing.T) {
	base := &fakePresetEntities{types: []entities.EntityType{{ID: 4, Name: "Item", Slug: "item"}}}
	sw := &fakeSwitcher{stateErr: errors.New("db down")}
	p := newPresetApplier(switchEntities{fakePresetEntities: base, counts: map[int]int{}}).withFieldSwitch(sw)
	if _, err := p.applyManifestPresets(context.Background(), "c1", "drawsteel", switchManifest(), true, nil); err != nil {
		t.Fatal(err)
	}
	if len(sw.accepted) != 0 || len(base.reconciled) != 0 {
		t.Errorf("a failed read must change nothing: accepted %v merged %v", sw.accepted, base.reconciled)
	}
	for _, c := range base.created {
		if c.PresetCategory == "item" {
			t.Error("a failed read must not make a second Items")
		}
	}
}

func TestSwitchOfferFor(t *testing.T) {
	m := switchManifest()
	items := entities.EntityType{ID: 4, Slug: "item"}
	chars := entities.EntityType{ID: 1, Slug: "character"}
	types := []entities.EntityType{chars, items}
	if o := switchOfferFor(m, types, &items); o == nil || o.PresetSlug != "drawsteel-item" || o.SystemID != "drawsteel" {
		t.Errorf("Items is offered the treasure fields, got %+v", o)
	}
	if o := switchOfferFor(m, types, &chars); o != nil {
		t.Errorf("Characters is offered nothing, got %+v", o)
	}
	plain := &systems.SystemManifest{ID: "dnd", EntityPresets: []systems.EntityPresetDef{{Slug: "i", Category: "item"}}}
	if o := switchOfferFor(plain, types, &items); o != nil {
		t.Errorf("a preset without a switch block offers nothing, got %+v", o)
	}
}

type stateOnly map[int]*entities.FieldSwitch

func (s stateOnly) State(_ context.Context, id int) (*entities.FieldSwitch, error) { return s[id], nil }

func TestSwitchedCategoryGate(t *testing.T) {
	preset := &switchManifest().EntityPresets[1]
	types := &fakePresetEntities{types: []entities.EntityType{{ID: 4, Slug: "item"}}}
	tests := []struct {
		name  string
		state *entities.FieldSwitch
		want  bool
	}{
		{"unanswered", nil, false},
		{"Not now", &entities.FieldSwitch{State: entities.FieldSwitchDismissed}, false},
		{"switched back", &entities.FieldSwitch{State: entities.FieldSwitchBack}, false},
		{"switched", &entities.FieldSwitch{State: entities.FieldSwitchSwitched}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gate := switchedCategoryGate(types, stateOnly{4: tc.state})
			if got := gate(context.Background(), "c1", preset); got != tc.want {
				t.Errorf("gate = %v, want %v", got, tc.want)
			}
		})
	}
	if switchedCategoryGate(types, stateOnly{})(context.Background(), "c1", nil) {
		t.Error("no preset is never switched")
	}
}
