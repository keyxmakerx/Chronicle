package entities

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

func TestSeedCharacterLists(t *testing.T) {
	tests := []struct {
		name      string
		types     []EntityType
		wantChars []int
		wantNPCs  []int
	}{
		{
			name: "default seed: character only, not location or item",
			types: []EntityType{
				{ID: 1, Slug: "character", Enabled: true},
				{ID: 2, Slug: "location", Enabled: true},
				{ID: 4, Slug: "item", PresetCategory: strp("item"), Enabled: true},
			},
			wantChars: []int{1}, wantNPCs: []int{1},
		},
		{
			name: "player-character sub-type is covered by its parent for characters",
			types: []EntityType{
				{ID: 1, Slug: "character", Enabled: true},
				{ID: 2, Slug: "player-character", PresetCategory: strp(PresetCategoryPlayerCharacter), ParentTypeID: intp(1), Enabled: true},
				{ID: 3, Slug: "villain", ParentTypeID: intp(1), Enabled: true},
			},
			wantChars: []int{1}, wantNPCs: []int{1},
		},
		{
			name: "creature and monster types are left out, system characters kept",
			types: []EntityType{
				{ID: 5, Slug: "npc", Enabled: true},
				{ID: 6, Slug: "creature", Enabled: true},
				{ID: 7, Slug: "drawsteel-monster", Enabled: true},
				{ID: 8, Slug: "dnd5e-character", Enabled: true},
				{ID: 9, Slug: "beast", PresetCategory: strp("creature"), Enabled: true},
				{ID: 10, Slug: "drawsteel-creature", PresetCategory: strp("creature"), Enabled: true},
			},
			wantChars: []int{5, 8}, wantNPCs: []int{5, 8},
		},
		{
			name: "a sub-type of a creature type stays out",
			types: []EntityType{
				{ID: 1, Slug: "creature", Enabled: true},
				{ID: 2, Slug: "undead", ParentTypeID: intp(1), Enabled: true},
			},
			wantChars: []int{}, wantNPCs: []int{},
		},
		{
			name:      "disabled type is left out",
			types:     []EntityType{{ID: 1, Slug: "character", Enabled: false}},
			wantChars: []int{}, wantNPCs: []int{},
		},
		{
			name: "a sub-type whose parent is disabled is listed itself",
			types: []EntityType{
				{ID: 1, Slug: "character", Enabled: false},
				{ID: 2, Slug: "villain", ParentTypeID: intp(1), Enabled: true},
			},
			wantChars: []int{2}, wantNPCs: []int{2},
		},
		{
			name: "parent cycle does not loop",
			types: []EntityType{
				{ID: 1, Slug: "a", ParentTypeID: intp(2), Enabled: true},
				{ID: 2, Slug: "b", ParentTypeID: intp(1), Enabled: true},
			},
			wantChars: []int{}, wantNPCs: []int{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chars, npcs := seedCharacterLists(tt.types)
			if !reflect.DeepEqual(chars, tt.wantChars) || !reflect.DeepEqual(npcs, tt.wantNPCs) {
				t.Errorf("seed = %v / %v, want %v / %v", chars, npcs, tt.wantChars, tt.wantNPCs)
			}
		})
	}
}

func TestResolveCharacterTypeIDs(t *testing.T) {
	types := []EntityType{
		{ID: 1, Slug: "character", Enabled: true},
		{ID: 2, Slug: "player-character", PresetCategory: strp(PresetCategoryPlayerCharacter), ParentTypeID: intp(1), Enabled: true},
		{ID: 3, Slug: "villain", ParentTypeID: intp(1), Enabled: true},
		{ID: 4, Slug: "boss", ParentTypeID: intp(3), Enabled: true},
		{ID: 5, Slug: "creature", Enabled: true},
		{ID: 6, Slug: "retired", ParentTypeID: intp(1), Enabled: false},
	}
	tests := []struct {
		name   string
		listed []int
		skipPC bool
		want   []int
	}{
		{"nothing listed shows nothing", nil, false, nil},
		{"a listed parent brings every enabled sub-type", []int{1}, false, []int{1, 2, 3, 4}},
		{"the NPC list leaves the player-character sub-type out", []int{1}, true, []int{1, 3, 4}},
		{"a type listed by name is kept even for the NPC list", []int{2}, true, []int{2}},
		{"a listed leaf shows only itself", []int{4}, false, []int{4}},
		{"the owner can list creature", []int{5}, false, []int{5}},
		{"a disabled type is not shown even when listed", []int{6}, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveCharacterTypeIDs(types, tt.listed, tt.skipPC); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeTypeSource struct {
	types  []EntityType
	counts map[int]int
}

func (f *fakeTypeSource) GetEntityTypes(context.Context, string) ([]EntityType, error) {
	return f.types, nil
}
func (f *fakeTypeSource) CountByType(context.Context, string, int, string) (map[int]int, error) {
	return f.counts, nil
}

type fakeListStore struct {
	lists CharacterLists
	ok    bool
	saves int
}

func (f *fakeListStore) Get(context.Context, string) (CharacterLists, bool, error) {
	return f.lists, f.ok, nil
}
func (f *fakeListStore) Save(_ context.Context, _ string, l CharacterLists) error {
	f.lists, f.ok = l, true
	f.saves++
	return nil
}

func listFixture() (*CharacterListService, *fakeListStore, *[]string) {
	types := &fakeTypeSource{
		types: []EntityType{
			{ID: 1, Name: "Character", Slug: "character", Enabled: true},
			{ID: 2, Name: "Villain", Slug: "villain", ParentTypeID: intp(1), Enabled: true},
			{ID: 3, Name: "Creature", Slug: "creature", PresetCategory: strp("creature"), Enabled: true},
			{ID: 4, Name: "Location", Slug: "location", Enabled: true},
		},
		counts: map[int]int{1: 4, 3: 1},
	}
	store := &fakeListStore{}
	svc := NewCharacterListService(types, store)
	var changed []string
	svc.SetChangeHook(func(id string) { changed = append(changed, id) })
	return svc, store, &changed
}

func TestCharacterListService_AddRemove(t *testing.T) {
	ctx := context.Background()
	svc, store, changed := listFixture()

	// Reads before any choice show nothing: no guess at request time.
	if ids, err := svc.NPCTypeIDs(ctx, "c1"); err != nil || len(ids) != 0 {
		t.Fatalf("unchosen NPC list = %v, %v; want empty", ids, err)
	}

	if name, err := svc.Add(ctx, "c1", CharacterListNPCs, 2); err != nil || name != "Villain" {
		t.Fatalf("add villain: %q, %v", name, err)
	}
	if !reflect.DeepEqual(store.lists.NPCTypeIDs, []int{2}) {
		t.Fatalf("npc list = %v", store.lists.NPCTypeIDs)
	}
	// Adding the parent absorbs the sub-type it now covers.
	if _, err := svc.Add(ctx, "c1", CharacterListNPCs, 1); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.lists.NPCTypeIDs, []int{1}) {
		t.Fatalf("npc list after parent = %v, want [1]", store.lists.NPCTypeIDs)
	}
	// Adding something already covered changes nothing and saves nothing.
	saves := store.saves
	if _, err := svc.Add(ctx, "c1", CharacterListNPCs, 2); err != nil || store.saves != saves {
		t.Fatalf("covered add: err=%v saves %d -> %d", err, saves, store.saves)
	}
	// The characters list is separate.
	if len(store.lists.CharacterTypeIDs) != 0 {
		t.Fatalf("characters list = %v, want untouched", store.lists.CharacterTypeIDs)
	}
	if got, _ := svc.NPCTypeIDs(ctx, "c1"); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("resolved NPC list = %v, want [1 2]", got)
	}

	if name, err := svc.Remove(ctx, "c1", CharacterListNPCs, 1); err != nil || name != "Character" {
		t.Fatalf("remove: %q, %v", name, err)
	}
	if len(store.lists.NPCTypeIDs) != 0 {
		t.Fatalf("npc list = %v, want empty", store.lists.NPCTypeIDs)
	}
	if len(*changed) != 3 {
		t.Errorf("change hook ran %d times, want 3 (two saves and the remove)", len(*changed))
	}
}

func TestCharacterListService_Rejects(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := listFixture()
	tests := []struct {
		name string
		do   func() error
		code int
	}{
		{"add a type from another campaign", func() error { _, e := svc.Add(ctx, "c1", CharacterListNPCs, 99); return e }, 400},
		{"add to an unknown list", func() error { _, e := svc.Add(ctx, "c1", "bogus", 1); return e }, 400},
		{"remove a type that is not listed", func() error { _, e := svc.Remove(ctx, "c1", CharacterListNPCs, 1); return e }, 404},
		{"remove from an unknown list", func() error { _, e := svc.Remove(ctx, "c1", "bogus", 1); return e }, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ae *apperror.AppError
			if err := tt.do(); !errors.As(err, &ae) || ae.Code != tt.code {
				t.Errorf("err = %v, want code %d", err, tt.code)
			}
		})
	}
}

func TestCharacterListService_PrunesDeletedTypes(t *testing.T) {
	ctx := context.Background()
	svc, store, _ := listFixture()
	store.lists = CharacterLists{CharacterTypeIDs: []int{1, 77}, NPCTypeIDs: []int{88}}
	store.ok = true

	chosen, err := svc.Chosen(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(chosen.CharacterTypeIDs, []int{1}) || len(chosen.NPCTypeIDs) != 0 {
		t.Errorf("chosen = %+v, want the deleted IDs gone", chosen)
	}
	// A save writes the pruned lists back.
	if _, err := svc.Add(ctx, "c1", CharacterListNPCs, 3); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.lists.CharacterTypeIDs, []int{1}) || !reflect.DeepEqual(store.lists.NPCTypeIDs, []int{3}) {
		t.Errorf("saved = %+v", store.lists)
	}
}

func TestCharacterListService_Seed(t *testing.T) {
	ctx := context.Background()
	svc, store, _ := listFixture()

	if err := svc.Seed(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	// Character covers its Villain sub-type; the creature type stays out.
	if !reflect.DeepEqual(store.lists.CharacterTypeIDs, []int{1}) || !reflect.DeepEqual(store.lists.NPCTypeIDs, []int{1}) {
		t.Fatalf("seeded = %+v", store.lists)
	}
	// An owner's later choice survives another pass.
	store.lists = CharacterLists{CharacterTypeIDs: []int{}, NPCTypeIDs: []int{3}}
	saves := store.saves
	if err := svc.Seed(ctx, "c1"); err != nil || store.saves != saves {
		t.Fatalf("second seed: err=%v saves %d -> %d, want untouched", err, saves, store.saves)
	}
}

func TestCharacterListService_Editor(t *testing.T) {
	ctx := context.Background()
	svc, store, _ := listFixture()
	store.lists = CharacterLists{NPCTypeIDs: []int{3}}
	store.ok = true

	ed, err := svc.Editor(ctx, "c1", CharacterListNPCs)
	if err != nil {
		t.Fatal(err)
	}
	if ed.Label != "NPCs" || len(ed.Chips) != 1 || ed.Chips[0].Name != "Creature" {
		t.Errorf("chips = %+v", ed.Chips)
	}
	// Creature is on the list, so it is not offered again; the sub-type follows
	// its parent and the parent carries the "with" note.
	var names []string
	for _, o := range ed.Options {
		names = append(names, o.Name)
	}
	if !reflect.DeepEqual(names, []string{"Character", "Villain", "Location"}) {
		t.Fatalf("options = %v", names)
	}
	if !ed.Options[1].IsSub || !reflect.DeepEqual(ed.Options[0].With, []string{"Villain"}) || ed.Options[0].Count != 4 {
		t.Errorf("options detail = %+v", ed.Options)
	}
}
