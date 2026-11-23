package entities

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// treasureOffer is the shape of Draw Steel's Items switch: a reused Type pick
// list, two fields set aside, and new fields.
func treasureOffer() FieldSwitchOffer {
	return FieldSwitchOffer{
		SystemID: "drawsteel", SystemName: "Draw Steel", PresetSlug: "drawsteel-item",
		Category: "item", Label: "treasure fields",
		Fields: []FieldDefinition{
			{Key: "type", Label: "Type", Type: "select", Options: []string{"Consumable", "Trinket", "Leveled", "Artifact"}},
			{Key: "kind", Label: "Kind", Type: "select", Options: []string{"Weapon", "Armor", "Implement", "Other"}},
			{Key: "effect", Label: "Effect", Type: "textarea"},
			{Key: "book_source", Label: "Copied from", Type: "text", GMOnly: true},
		},
		SetAside: []string{"rarity", "attunement"},
		Reuse:    []string{"type"},
	}
}

// itemFields is the Items list a fantasy campaign starts with.
func itemFields() []FieldDefinition {
	return []FieldDefinition{
		{Key: "type", Label: "Type", Type: "text"},
		{Key: "rarity", Label: "Rarity", Type: "text"},
		{Key: "attunement", Label: "Requires Attunement", Type: "checkbox"},
		{Key: "weight", Label: "Weight", Type: "text"},
	}
}

func keysOf(fs []FieldDefinition) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Key
	}
	return out
}

func TestSwitchTarget(t *testing.T) {
	item := "item"
	other := "creature"
	parent := 1
	tests := []struct {
		name  string
		types []EntityType
		want  int // 0 for none
	}{
		{"the seeded Items", []EntityType{{ID: 1, Slug: "character"}, {ID: 4, Slug: "item"}}, 4},
		{"a type bound to the category wins over the slug", []EntityType{{ID: 4, Slug: "item"}, {ID: 9, Slug: "gear", PresetCategory: &item}}, 9},
		{"a sub-category called item is not it", []EntityType{{ID: 5, Slug: "item", ParentTypeID: &parent}}, 0},
		{"an item slug bound to another preset is not it", []EntityType{{ID: 6, Slug: "item", PresetCategory: &other}}, 0},
		{"no items at all", []EntityType{{ID: 1, Slug: "character"}}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SwitchTarget(tc.types, "item")
			if (got == nil) != (tc.want == 0) || (got != nil && got.ID != tc.want) {
				t.Fatalf("SwitchTarget = %+v, want id %d", got, tc.want)
			}
		})
	}
	if SwitchTarget([]EntityType{{ID: 4, Slug: "item"}}, "") != nil {
		t.Error("an empty category matches nothing")
	}
}

func TestSwitchedFieldLists(t *testing.T) {
	offer := treasureOffer()
	current := append(itemFields(), FieldDefinition{Key: "owner_note", Label: "Owner note", Type: "text"})

	after := switchedFieldList(current, offer)
	if got, want := keysOf(after), []string{"type", "kind", "effect", "book_source", "weight", "owner_note"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after switching = %v, want %v", got, want)
	}
	if after[0].Type != "select" || len(after[0].Options) != 4 {
		t.Errorf("the shared Type takes the system's pick list, got %+v", after[0])
	}
	after[0].Options[0] = "changed"
	if offer.Fields[0].Options[0] != "Consumable" {
		t.Error("the switched list must not alias the offer's options")
	}

	// A field the owner adds after switching stays when they switch back.
	switched := append(switchedFieldList(current, offer), FieldDefinition{Key: "added_later", Label: "Later"})
	back := switchedBackFieldList(switched, current, offer)
	if got, want := keysOf(back), []string{"type", "rarity", "attunement", "weight", "owner_note", "added_later"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after switching back = %v, want %v", got, want)
	}
	if back[0].Type != "text" {
		t.Errorf("switching back restores the old Type definition, got %+v", back[0])
	}
}

func TestValueMoveRoundTrip(t *testing.T) {
	offer := treasureOffer()
	old := itemFields()
	accept := acceptMove(old, offer)
	back := backMove(old, offer)

	tests := []struct {
		name         string
		page         map[string]any
		wantSwitched map[string]any
		wantKept     map[string]any
	}{
		{
			name:         "rarity and attunement are kept aside, a fitting Type stays",
			page:         map[string]any{"type": "Consumable", "rarity": "Rare", "attunement": true, "weight": "1 lb"},
			wantSwitched: map[string]any{"type": "Consumable", "weight": "1 lb"},
			wantKept:     map[string]any{"rarity": "Rare", "attunement": true},
		},
		{
			name:         "a Type the pick list lacks is kept aside too",
			page:         map[string]any{"type": "Wondrous item", "rarity": "Uncommon"},
			wantSwitched: map[string]any{},
			wantKept:     map[string]any{"type": "Wondrous item", "rarity": "Uncommon"},
		},
		{
			name:         "a page with none of the fields does not change",
			page:         map[string]any{"weight": "2 lb"},
			wantSwitched: map[string]any{"weight": "2 lb"},
			wantKept:     map[string]any{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, kept, _ := accept.apply(tc.page, nil)
			if !reflect.DeepEqual(d, tc.wantSwitched) || !reflect.DeepEqual(kept, tc.wantKept) {
				t.Fatalf("accept: page %v kept %v, want page %v kept %v", d, kept, tc.wantSwitched, tc.wantKept)
			}
			// New fields start empty: accepting fills nothing in.
			for _, k := range []string{"kind", "effect", "book_source"} {
				if _, ok := d[k]; ok {
					t.Errorf("accept filled %s", k)
				}
			}
			d2, kept2, _ := back.apply(d, kept)
			if !reflect.DeepEqual(d2, tc.page) || len(kept2) != 0 {
				t.Errorf("back: page %v kept %v, want the page as it was %v and nothing kept", d2, kept2, tc.page)
			}
		})
	}
}

func TestValueMoveKeepsWhatWasTypedAfterTheSwitch(t *testing.T) {
	offer := treasureOffer()
	old := itemFields()
	page := map[string]any{"type": "Wondrous item", "rarity": "Rare"}

	d, kept, _ := acceptMove(old, offer).apply(page, nil)
	// The Director fills the treasure fields after switching.
	d["type"] = "Trinket"
	d["kind"] = "Other"

	d, kept, _ = backMove(old, offer).apply(d, kept)
	if !reflect.DeepEqual(d, map[string]any{"type": "Wondrous item", "rarity": "Rare"}) {
		t.Errorf("back: page = %v", d)
	}
	if !reflect.DeepEqual(kept, map[string]any{"type": "Trinket", "kind": "Other"}) {
		t.Errorf("back keeps the treasure values aside, got %v", kept)
	}

	// Switching again brings the treasure values back, and the old Type goes aside again.
	d, kept, changed := acceptMove(old, offer).apply(d, kept)
	if !changed {
		t.Fatal("switching again should move values")
	}
	if !reflect.DeepEqual(d, map[string]any{"type": "Trinket", "kind": "Other"}) {
		t.Errorf("switch again: page = %v", d)
	}
	if !reflect.DeepEqual(kept, map[string]any{"type": "Wondrous item", "rarity": "Rare"}) {
		t.Errorf("switch again: kept = %v", kept)
	}
}

func TestSetAsideValues(t *testing.T) {
	kept := map[string]any{"attunement": true, "rarity": "Rare", "zzz": "x", "type": ""}
	got := setAsideValues(kept, itemFields(), treasureOffer().Fields)
	want := []SetAsideValue{
		{Key: "type", Label: "Type", Value: "empty"},
		{Key: "rarity", Label: "Rarity", Value: "Rare"},
		{Key: "attunement", Label: "Requires Attunement", Value: "Yes"},
		{Key: "zzz", Label: "zzz", Value: "x"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("setAsideValues = %+v, want %+v", got, want)
	}
	if setAsideValues(nil) != nil {
		t.Error("nothing kept lists nothing")
	}
	if got := setAsideText("on", "checkbox"); got != "Yes" {
		t.Errorf("a ticked box saved as text = %q", got)
	}
}

func TestFieldSwitchViewWording(t *testing.T) {
	et := &EntityType{NamePlural: "Items", Fields: itemFields()}
	offer := treasureOffer()
	tests := []struct {
		name        string
		sw          *FieldSwitch
		fields      []FieldDefinition
		summary     string
		wantState   string
		wantBack    bool
		wantAdds    string
		wantRemoves string
	}{
		{
			name: "unanswered", fields: itemFields(),
			wantAdds:    "Adds Type, Kind, Effect and Copied from.",
			wantRemoves: "Rarity and Requires Attunement come off the pages, and so does a Type that isn’t Consumable, Trinket, Leveled or Artifact. What they held is kept in a fold only you see.",
		},
		{
			name: "a summary replaces the label list", fields: itemFields(), summary: "Type, Kind and the Effect",
			wantAdds:    "Adds Type, Kind and the Effect.",
			wantRemoves: "Rarity and Requires Attunement come off the pages, and so does a Type that isn’t Consumable, Trinket, Leveled or Artifact. What they held is kept in a fold only you see.",
		},
		{
			name: "only a Type, nothing to set aside", fields: []FieldDefinition{{Key: "type", Label: "Type"}},
			wantAdds:    "Adds Type, Kind, Effect and Copied from.",
			wantRemoves: "A Type that isn’t Consumable, Trinket, Leveled or Artifact comes off the pages. What it held is kept in a fold only you see.",
		},
		{
			name: "one field comes off", fields: []FieldDefinition{{Key: "rarity", Label: "Rarity"}},
			wantAdds:    "Adds Type, Kind, Effect and Copied from.",
			wantRemoves: "Rarity comes off the pages. What it held is kept in a fold only you see.",
		},
		{
			name: "nothing comes off", fields: []FieldDefinition{{Key: "weight", Label: "Weight"}},
			wantAdds: "Adds Type, Kind, Effect and Copied from.",
		},
		{
			name: "switched by the owner can go back", fields: itemFields(),
			sw:        &FieldSwitch{State: FieldSwitchSwitched, OldFields: itemFields()},
			wantState: FieldSwitchSwitched, wantBack: true,
			wantAdds:    "Adds Type, Kind, Effect and Copied from.",
			wantRemoves: "Rarity and Requires Attunement come off the pages, and so does a Type that isn’t Consumable, Trinket, Leveled or Artifact. What they held is kept in a fold only you see.",
		},
		{
			name: "made with the fields has nothing to go back to", fields: itemFields(),
			sw:          &FieldSwitch{State: FieldSwitchSwitched},
			wantState:   FieldSwitchSwitched,
			wantAdds:    "Adds Type, Kind, Effect and Copied from.",
			wantRemoves: "Rarity and Requires Attunement come off the pages, and so does a Type that isn’t Consumable, Trinket, Leveled or Artifact. What they held is kept in a fold only you see.",
		},
		{
			name: "Not now", fields: itemFields(),
			sw:          &FieldSwitch{State: FieldSwitchDismissed},
			wantState:   FieldSwitchDismissed,
			wantAdds:    "Adds Type, Kind, Effect and Copied from.",
			wantRemoves: "Rarity and Requires Attunement come off the pages, and so does a Type that isn’t Consumable, Trinket, Leveled or Artifact. What they held is kept in a fold only you see.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := offer
			o.Summary = tc.summary
			e := *et
			e.Fields = tc.fields
			v := switchView(&e, o, tc.sw)
			if v.State != tc.wantState || v.CanSwitchBack != tc.wantBack {
				t.Errorf("state %q back %v, want %q %v", v.State, v.CanSwitchBack, tc.wantState, tc.wantBack)
			}
			if got := v.AddsLine(); got != tc.wantAdds {
				t.Errorf("AddsLine = %q, want %q", got, tc.wantAdds)
			}
			if got := v.RemovesLine(tc.fields); got != tc.wantRemoves {
				t.Errorf("RemovesLine = %q, want %q", got, tc.wantRemoves)
			}
		})
	}
	v := switchView(et, offer, nil)
	if got := v.Question("Items"); got != "Use Draw Steel’s treasure fields for Items?" {
		t.Errorf("Question = %q", got)
	}
	if !strings.Contains(v.BackLine(), "treasure fields come off") {
		t.Errorf("BackLine = %q", v.BackLine())
	}
}

// fakeSwitchRepo applies a switch to in-memory pages the way the SQL
// repository does in its transaction.
type fakeSwitchRepo struct {
	rows   map[int]*FieldSwitch
	types  map[int]*EntityType
	pages  map[string]map[string]any
	kept   map[string]map[string]any
	getErr error
	// beforeSwitch runs once at the start of the next Switch, standing in
	// for another request that commits between the service's reads and the
	// repository's lock.
	beforeSwitch func()
}

func newFakeSwitchRepo(et *EntityType, pages map[string]map[string]any) *fakeSwitchRepo {
	return &fakeSwitchRepo{rows: map[int]*FieldSwitch{}, types: map[int]*EntityType{et.ID: et}, pages: pages, kept: map[string]map[string]any{}}
}

func (f *fakeSwitchRepo) Get(_ context.Context, id int) (*FieldSwitch, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if r, ok := f.rows[id]; ok {
		c := *r
		return &c, nil
	}
	return nil, nil
}

func (f *fakeSwitchRepo) SaveState(_ context.Context, sw *FieldSwitch) error {
	c := *sw
	f.rows[sw.EntityTypeID] = &c
	return nil
}

func (f *fakeSwitchRepo) Switch(_ context.Context, sw *FieldSwitch, base switchBase, fields []FieldDefinition, cat *string, move valueMove) (int, error) {
	if hook := f.beforeSwitch; hook != nil {
		f.beforeSwitch = nil
		hook()
	}
	state := ""
	if r, ok := f.rows[sw.EntityTypeID]; ok {
		state = r.State
	}
	stored, _ := json.Marshal(f.types[sw.EntityTypeID].Fields)
	if same, err := sameFieldList(stored, base.Fields); err != nil || !same || state != base.State {
		return 0, errSwitchMoved
	}
	n := 0
	for id, page := range f.pages {
		d, kept, changed := move.apply(page, f.kept[id])
		if changed {
			n++
		}
		f.pages[id] = d
		f.kept[id] = kept
	}
	et := f.types[sw.EntityTypeID]
	et.Fields = fields
	et.PresetCategory = cat
	c := *sw
	f.rows[sw.EntityTypeID] = &c
	return n, nil
}

func (f *fakeSwitchRepo) SetAside(_ context.Context, id string) (map[string]any, error) {
	return f.kept[id], nil
}

func (f *fakeSwitchRepo) ForCampaign(_ context.Context, campaignID string) (map[int]*FieldSwitch, error) {
	out := map[int]*FieldSwitch{}
	for id, r := range f.rows {
		if r.CampaignID == campaignID {
			c := *r
			out[id] = &c
		}
	}
	return out, nil
}

func (f *fakeSwitchRepo) SetAsideForCampaign(context.Context, string) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	for id, k := range f.kept {
		if len(k) > 0 {
			out[id] = k
		}
	}
	return out, nil
}

func (f *fakeSwitchRepo) Clear(_ context.Context, id int) error {
	delete(f.rows, id)
	return nil
}

func (f *fakeSwitchRepo) RestoreSetAside(_ context.Context, id string, _ int, v map[string]any) error {
	f.kept[id] = v
	return nil
}

type fakeSwitchTypes struct{ repo *fakeSwitchRepo }

func (t fakeSwitchTypes) FindByID(_ context.Context, id int) (*EntityType, error) {
	if et, ok := t.repo.types[id]; ok {
		c := *et
		return &c, nil
	}
	return nil, apperror.NewNotFound("no such category")
}

func newSwitchService(offered bool) (*FieldSwitchService, *fakeSwitchRepo) {
	et := &EntityType{ID: 4, CampaignID: "c1", Slug: "item", NamePlural: "Items", Fields: itemFields()}
	repo := newFakeSwitchRepo(et, map[string]map[string]any{
		"p1": {"type": "Wondrous item", "rarity": "Rare", "weight": "1 lb"},
		"p2": {"type": "Consumable"},
	})
	offers := func(context.Context, string, *EntityType) *FieldSwitchOffer {
		if !offered {
			return nil
		}
		o := treasureOffer()
		return &o
	}
	return NewFieldSwitchService(repo, fakeSwitchTypes{repo}, offers), repo
}

func switchErrCode(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func TestFieldSwitchServiceAcceptAndBack(t *testing.T) {
	ctx := context.Background()
	svc, repo := newSwitchService(true)

	n, err := svc.Accept(ctx, "c1", 4, "owner")
	if err != nil || n != 1 {
		t.Fatalf("Accept = %d, %v; want 1 page changed", n, err)
	}
	et := repo.types[4]
	if et.PresetCategory == nil || *et.PresetCategory != "item" {
		t.Errorf("a switched category takes the preset category, got %v", et.PresetCategory)
	}
	if got := keysOf(et.Fields); !reflect.DeepEqual(got, []string{"type", "kind", "effect", "book_source", "weight"}) {
		t.Errorf("fields after accept = %v", got)
	}
	if aside := svc.SetAside(ctx, et, "p1"); len(aside) != 2 {
		t.Errorf("p1's fold = %+v, want Type and Rarity", aside)
	}
	if _, err := svc.Accept(ctx, "c1", 4, "owner"); switchErrCode(err) != 409 {
		t.Errorf("accepting twice = %v, want a conflict", err)
	}

	if _, err := svc.Back(ctx, "c1", 4, "owner"); err != nil {
		t.Fatalf("Back: %v", err)
	}
	if got := keysOf(repo.types[4].Fields); !reflect.DeepEqual(got, keysOf(itemFields())) {
		t.Errorf("fields after back = %v", got)
	}
	if repo.types[4].PresetCategory != nil {
		t.Errorf("back restores the old preset category, got %v", *repo.types[4].PresetCategory)
	}
	if !reflect.DeepEqual(repo.pages["p1"], map[string]any{"type": "Wondrous item", "rarity": "Rare", "weight": "1 lb"}) {
		t.Errorf("p1 after back = %v", repo.pages["p1"])
	}
	if _, err := svc.Back(ctx, "c1", 4, "owner"); switchErrCode(err) != 409 {
		t.Errorf("switching back twice = %v, want a conflict", err)
	}
}

func TestFieldSwitchServiceRefusals(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		offered  bool
		campaign string
		call     func(*FieldSwitchService, string) error
		wantCode int
	}{
		{"accept with no offer", false, "c1", func(s *FieldSwitchService, c string) error { _, err := s.Accept(ctx, c, 4, "u"); return err }, 400},
		{"dismiss with no offer", false, "c1", func(s *FieldSwitchService, c string) error { return s.Dismiss(ctx, c, 4, "u") }, 400},
		{"accept another campaign's category", true, "c2", func(s *FieldSwitchService, c string) error { _, err := s.Accept(ctx, c, 4, "u"); return err }, 404},
		{"back another campaign's category", true, "c2", func(s *FieldSwitchService, c string) error { _, err := s.Back(ctx, c, 4, "u"); return err }, 404},
		{"back before any switch", true, "c1", func(s *FieldSwitchService, c string) error { _, err := s.Back(ctx, c, 4, "u"); return err }, 409},
		{"accept a missing category", true, "c1", func(s *FieldSwitchService, c string) error { _, err := s.Accept(ctx, c, 99, "u"); return err }, 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newSwitchService(tc.offered)
			if err := tc.call(svc, tc.campaign); switchErrCode(err) != tc.wantCode {
				t.Errorf("err = %v, want code %d", err, tc.wantCode)
			}
		})
	}
}

func TestFieldSwitchServiceDismissAndView(t *testing.T) {
	ctx := context.Background()
	svc, repo := newSwitchService(true)
	et := repo.types[4]

	if v := svc.View(ctx, "c1", et); v == nil || v.State != "" {
		t.Fatalf("an unanswered offer shows, got %+v", v)
	}
	if err := svc.Dismiss(ctx, "c1", 4, "owner"); err != nil {
		t.Fatal(err)
	}
	if v := svc.View(ctx, "c1", et); v == nil || v.State != FieldSwitchDismissed {
		t.Fatalf("after Not now the Fields tab still offers it, got %+v", v)
	}
	if !reflect.DeepEqual(repo.pages["p1"], map[string]any{"type": "Wondrous item", "rarity": "Rare", "weight": "1 lb"}) {
		t.Error("Not now changes no page")
	}
	// The switch is still there after Not now, and Not now after a switch is a no-op.
	if _, err := svc.Accept(ctx, "c1", 4, "owner"); err != nil {
		t.Fatalf("accept after Not now: %v", err)
	}
	if err := svc.Dismiss(ctx, "c1", 4, "owner"); err != nil || repo.rows[4].State != FieldSwitchSwitched {
		t.Errorf("Not now after accepting must not undo it: %v, state %s", err, repo.rows[4].State)
	}

	repo.getErr = errors.New("db down")
	if v := svc.View(ctx, "c1", et); v != nil {
		t.Error("a failed read shows no offer rather than failing the page")
	}
}

// With the system off there is no list to tell its fields from ones the
// owner added since the switch, so switching back waits for it; nothing
// changes in the meantime.
func TestFieldSwitchServiceBackWithoutTheSystem(t *testing.T) {
	ctx := context.Background()
	on, repo := newSwitchService(true)
	if _, err := on.Accept(ctx, "c1", 4, "owner"); err != nil {
		t.Fatal(err)
	}
	repo.types[4].Fields = append(repo.types[4].Fields, FieldDefinition{Key: "owner_note", Label: "Owner note", Type: "text"})
	before := keysOf(repo.types[4].Fields)
	off := NewFieldSwitchService(repo, fakeSwitchTypes{repo}, nil)
	if _, err := off.Back(ctx, "c1", 4, "owner"); switchErrCode(err) != 409 {
		t.Fatalf("Back without the system = %v, want a conflict", err)
	}
	if got := keysOf(repo.types[4].Fields); !reflect.DeepEqual(got, before) || repo.rows[4].State != FieldSwitchSwitched {
		t.Errorf("a refused switch back changed the category: fields %v, state %s", got, repo.rows[4].State)
	}
	// Turned back on, the owner's own field stays.
	if _, err := on.Back(ctx, "c1", 4, "owner"); err != nil {
		t.Fatalf("Back with the system on: %v", err)
	}
	if got := keysOf(repo.types[4].Fields); !reflect.DeepEqual(got, append(keysOf(itemFields()), "owner_note")) {
		t.Errorf("fields after back = %v", got)
	}
}

// Two presses at once: the second worked out its list and moves from what the
// first has since changed, so it is refused and every page moves once.
func TestFieldSwitchServiceRefusesAMovedBase(t *testing.T) {
	ctx := context.Background()
	original := map[string]any{"type": "Wondrous item", "rarity": "Rare", "weight": "1 lb"}
	tests := []struct {
		name      string
		switched  bool // accept before the race
		call      func(*FieldSwitchService) error
		wantState string
	}{
		{"two switch backs", true, func(s *FieldSwitchService) error { _, err := s.Back(ctx, "c1", 4, "owner"); return err }, FieldSwitchBack},
		{"two accepts", false, func(s *FieldSwitchService) error { _, err := s.Accept(ctx, "c1", 4, "owner"); return err }, FieldSwitchSwitched},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newSwitchService(true)
			if tc.switched {
				if _, err := svc.Accept(ctx, "c1", 4, "owner"); err != nil {
					t.Fatal(err)
				}
			}
			var first error
			repo.beforeSwitch = func() { first = tc.call(svc) }
			if err := tc.call(svc); switchErrCode(err) != 409 {
				t.Fatalf("the second press = %v, want a conflict", err)
			}
			if first != nil {
				t.Fatalf("the first press failed: %v", first)
			}
			if repo.rows[4].State != tc.wantState {
				t.Errorf("state = %s, want %s", repo.rows[4].State, tc.wantState)
			}
			if tc.switched && !reflect.DeepEqual(repo.pages["p1"], original) {
				t.Errorf("p1 after the race = %v, want %v", repo.pages["p1"], original)
			}
		})
	}
}

// A backup's answer replaces whatever the category has, a missing answer
// clears it, and a hand-edited answer Chronicle cannot read is refused.
func TestFieldSwitchServiceRestore(t *testing.T) {
	ctx := context.Background()
	svc, repo := newSwitchService(true)
	if _, err := svc.Accept(ctx, "c1", 4, "owner"); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	if err := svc.Restore(ctx, "c1", 4, nil, "importer"); err != nil {
		t.Fatalf("Restore(nil): %v", err)
	}
	if _, ok := repo.rows[4]; ok {
		t.Errorf("restoring no answer left %+v, want the offer back", repo.rows[4])
	}

	old := itemFields()
	if err := svc.Restore(ctx, "c1", 4, &FieldSwitch{State: FieldSwitchBack, SystemID: "drawsteel", PresetSlug: "drawsteel-item", OldFields: old}, "importer"); err != nil {
		t.Fatalf("Restore(back): %v", err)
	}
	got := repo.rows[4]
	if got == nil || got.State != FieldSwitchBack || got.CampaignID != "c1" || got.UpdatedBy != "importer" || !reflect.DeepEqual(got.OldFields, old) {
		t.Errorf("restored row = %+v", got)
	}
	if err := svc.Restore(ctx, "c1", 4, &FieldSwitch{State: FieldSwitchSwitched, SystemID: "drawsteel", PresetSlug: "drawsteel-item"}, "importer"); err != nil {
		t.Fatalf("Restore(switched, nothing to go back to): %v", err)
	}
	if repo.rows[4].OldFields != nil {
		t.Errorf("an answer with nothing to go back to came back with %+v", repo.rows[4].OldFields)
	}

	long := strings.Repeat("x", 101)
	for name, sw := range map[string]*FieldSwitch{
		"unknown state":     {State: "maybe", SystemID: "drawsteel", PresetSlug: "drawsteel-item"},
		"no system":         {State: FieldSwitchDismissed, PresetSlug: "drawsteel-item"},
		"system too long":   {State: FieldSwitchDismissed, SystemID: long, PresetSlug: "drawsteel-item"},
		"old category long": {State: FieldSwitchDismissed, SystemID: "drawsteel", PresetSlug: "drawsteel-item", OldPresetCategory: &long},
	} {
		if err := svc.Restore(ctx, "c1", 4, sw, "importer"); switchErrCode(err) != 400 {
			t.Errorf("%s: Restore = %v, want a bad request", name, err)
		}
	}
	if err := svc.Restore(ctx, "other", 4, nil, "importer"); switchErrCode(err) != 404 {
		t.Errorf("another campaign's category: Restore = %v, want not found", err)
	}

	if err := svc.RestoreSetAside(ctx, "c1", 4, "p2", map[string]any{"rarity": "Common"}); err != nil {
		t.Fatalf("RestoreSetAside: %v", err)
	}
	if !reflect.DeepEqual(repo.kept["p2"], map[string]any{"rarity": "Common"}) {
		t.Errorf("p2's kept values = %v", repo.kept["p2"])
	}
	if err := svc.RestoreSetAside(ctx, "other", 4, "p2", map[string]any{"rarity": "Common"}); switchErrCode(err) != 404 {
		t.Errorf("another campaign's category: RestoreSetAside = %v, want not found", err)
	}
}
