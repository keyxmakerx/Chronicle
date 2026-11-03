package records

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

const camp = "camp-1"

var owner = Actor{UserID: "owner", Role: 3}
var player = Actor{UserID: "pl", Role: 1}

func rec(kind, action, name string, fields map[string]any, body string) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	return Record{Kind: kind, Action: action, Name: name, Fields: fields, Body: body}
}

// ---- notes: only the operator's own ----

type fakeNotes struct {
	notes      []notes.Note
	deleted    []string
	updated    []string
	lastUpdate notes.UpdateNoteRequest
}

func (f *fakeNotes) ListByUserAndCampaign(_ context.Context, _, _ string) ([]notes.Note, error) {
	return f.notes, nil
}
func (f *fakeNotes) ListVisible(_ context.Context, _ string, _ permissions.Viewer, _ notes.ListScope) ([]notes.Note, error) {
	return f.notes, nil
}
func (f *fakeNotes) GetByID(_ context.Context, id string) (*notes.Note, error) {
	for i := range f.notes {
		if f.notes[i].ID == id {
			return &f.notes[i], nil
		}
	}
	return nil, errors.New("not found")
}
func (f *fakeNotes) Create(_ context.Context, campaignID string, v permissions.Viewer, req notes.CreateNoteRequest) (*notes.Note, error) {
	n := notes.Note{ID: "new", CampaignID: campaignID, UserID: v.UserID(), Title: req.Title}
	f.notes = append(f.notes, n)
	return &n, nil
}
func (f *fakeNotes) Update(_ context.Context, id string, _ permissions.Viewer, req notes.UpdateNoteRequest) (*notes.Note, error) {
	f.updated = append(f.updated, id)
	f.lastUpdate = req
	return nil, nil
}
func (f *fakeNotes) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeEntities map[string]*entities.Entity

func (f fakeEntities) GetBySlug(_ context.Context, _, slug string) (*entities.Entity, error) {
	if e, ok := f[slug]; ok {
		return e, nil
	}
	return nil, errors.New("not found")
}

func TestNoteKind_OnlyOwnNotes(t *testing.T) {
	shared := notes.Note{ID: "theirs", CampaignID: camp, UserID: "someone-else", Title: "Plans", IsShared: true}
	mine := notes.Note{ID: "mine", CampaignID: camp, UserID: "owner", Title: "My plans"}
	tests := []struct {
		name    string
		r       Record
		wantErr string
		deleted []string
		updated []string
	}{
		{"cannot change a note shared with me", rec("note", ActionUpdate, "Plans", nil, "x"), "no note", nil, nil},
		{"cannot remove a note shared with me", rec("note", ActionDelete, "Plans", nil, ""), "no note", nil, nil},
		{"changes my own", rec("note", ActionUpdate, "My plans", nil, "new text"), "", nil, []string{"mine"}},
		{"removes my own", rec("note", ActionDelete, "my plans", nil, ""), "", []string{"mine"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeNotes{notes: []notes.Note{shared, mine}}
			k := NoteKind{Svc: f, Entities: fakeEntities{}}
			err := k.Apply(context.Background(), camp, owner, tt.r)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if strings.Join(f.deleted, ",") != strings.Join(tt.deleted, ",") || strings.Join(f.updated, ",") != strings.Join(tt.updated, ",") {
				t.Fatalf("deleted %v updated %v", f.deleted, f.updated)
			}
		})
	}
}

// A note the list says is mine but the store says is not is refused.
func TestNoteKind_RechecksStoredOwner(t *testing.T) {
	f := &fakeNotes{notes: []notes.Note{{ID: "n", CampaignID: camp, UserID: "owner", Title: "T"}}}
	k := NoteKind{Svc: &swapOwner{f}, Entities: fakeEntities{}}
	if err := k.Apply(context.Background(), camp, owner, rec("note", ActionDelete, "T", nil, "")); err == nil {
		t.Fatal("deleted a note whose stored owner is someone else")
	}
	if len(f.deleted) != 0 {
		t.Fatal("delete reached the service")
	}
}

type swapOwner struct{ *fakeNotes }

func (s *swapOwner) GetByID(ctx context.Context, id string) (*notes.Note, error) {
	n, err := s.fakeNotes.GetByID(ctx, id)
	if n != nil {
		c := *n
		c.UserID = "intruder"
		return &c, err
	}
	return n, err
}

// ---- maps: a map from another campaign is never matched ----

type fakeMaps struct {
	maps    []MapRef
	pins    []PinRef
	created []PinInput
	deleted []string
}

func (f *fakeMaps) ListMaps(context.Context, string) ([]MapRef, error) { return f.maps, nil }
func (f *fakeMaps) ListPins(context.Context, string, string, int, string) ([]PinRef, error) {
	return f.pins, nil
}
func (f *fakeMaps) CreatePin(_ context.Context, _, _ string, in PinInput) error {
	f.created = append(f.created, in)
	return nil
}
func (f *fakeMaps) UpdatePin(context.Context, string, PinInput, bool) error { return nil }
func (f *fakeMaps) DeletePin(_ context.Context, id string, _ bool, _ string, _ int) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestPinKind(t *testing.T) {
	f := &fakeMaps{
		maps: []MapRef{{ID: "m-other", CampaignID: "other", Name: "Grimvale"}, {ID: "m1", CampaignID: camp, Name: "World"}},
		pins: []PinRef{{ID: "p1", Name: "Old Mill"}},
	}
	k := PinKind{Svc: f, Entities: fakeEntities{}}
	ctx := context.Background()
	if p := k.Plan(ctx, camp, owner, rec("pin", ActionCreate, "X", map[string]any{"map": "Grimvale", "x": 1, "y": 2}, "")); p.Error == "" {
		t.Fatal("matched another campaign's map")
	}
	if p := k.Plan(ctx, camp, owner, rec("pin", ActionCreate, "X", map[string]any{"map": "World", "x": 101, "y": 2}, "")); p.Error == "" {
		t.Fatal("accepted x above 100")
	}
	if p := k.Plan(ctx, camp, player, rec("pin", ActionCreate, "X", map[string]any{"map": "World", "x": 1, "y": 2, "visibility": "dm_only"}, "")); p.Error == "" {
		t.Fatal("a player added a hidden pin")
	}
	if err := k.Apply(ctx, camp, owner, rec("pin", ActionDelete, "old mill", map[string]any{"map": "World"}, "")); err != nil || len(f.deleted) != 1 {
		t.Fatalf("delete: %v %v", err, f.deleted)
	}
}

// ---- tables and the registry ----

type fakeTables struct{ doc TableDoc }

func (f *fakeTables) Get(context.Context, string) (TableDoc, error) { return f.doc, nil }
func (f *fakeTables) Put(_ context.Context, _ string, d TableDoc, _ string) error {
	f.doc = d
	return nil
}

func TestTableKind(t *testing.T) {
	ft := &fakeTables{doc: TableDoc{Tables: []Table{{ID: "loot", Name: "Loot", Entries: []Entry{{Name: "Coin"}}}}}}
	k := TableKind{Svc: ft}
	ctx := context.Background()
	if p := k.Plan(ctx, camp, player, rec("table", ActionCreate, "X", nil, "- a")); p.Error == "" {
		t.Fatal("player changed tables")
	}
	if p := k.Plan(ctx, camp, owner, rec("table", ActionCreate, "loot", nil, "- a")); p.Error == "" {
		t.Fatal("created a duplicate table")
	}
	if err := k.Apply(ctx, camp, owner, rec("table", ActionUpdate, "Loot", nil, "- Gem: shiny\n- Sword")); err != nil {
		t.Fatal(err)
	}
	if es := ft.doc.Tables[0].Entries; len(es) != 2 || es[0].Brief != "shiny" || es[0].Weight != 1 || es[1].Weight != 1 {
		t.Fatalf("entries %+v", es)
	}
	fm := rec("table", ActionUpdate, "Loot", map[string]any{"entries": []any{"Plain", map[string]any{"name": "Rare", "weight": 3}}}, "")
	if err := k.Apply(ctx, camp, owner, fm); err != nil {
		t.Fatal(err)
	}
	if es := ft.doc.Tables[0].Entries; len(es) != 2 || es[0].Weight != 1 || es[1].Weight != 3 {
		t.Fatalf("front-matter entries %+v", es)
	}
	if err := k.Apply(ctx, camp, owner, rec("table", ActionDelete, "Loot", nil, "")); err != nil || len(ft.doc.Tables) != 0 {
		t.Fatalf("delete: %v %+v", err, ft.doc)
	}
}

func TestRegistry_Plan(t *testing.T) {
	r := NewRegistry(TableKind{Svc: &fakeTables{}}, nil)
	if _, p := r.Plan(context.Background(), camp, owner, rec("spaceship", ActionCreate, "x", nil, "")); p.Error == "" {
		t.Fatal("unknown kind accepted")
	}
	if _, p := r.Plan(context.Background(), camp, owner, rec("table", "explode", "x", nil, "")); p.Error == "" {
		t.Fatal("bad action accepted")
	}
	if k, ok := r.Get("TABLE"); !ok || k.Name() != "table" {
		t.Fatal("kind lookup is not case-insensitive")
	}
	if !strings.Contains(r.Docs(), "kind: table") {
		t.Fatal("docs missing")
	}
	caps := NewRegistry(TableKind{Svc: &fakeTables{}}, NoteKind{}).Capabilities()
	for _, want := range []string{"`kind: table`: rolling tables", "`kind: note`: my own notes only", "never notes anyone else wrote"} {
		if !strings.Contains(caps, want) {
			t.Errorf("capabilities missing %q:\n%s", want, caps)
		}
	}
	if strings.Contains(caps, "kind: pin") {
		t.Error("capabilities list a kind that is not wired")
	}
}

func TestPlanError_ShowsOnlyTheMessage(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{apperror.NewBadRequest("no page called \"X\""), "no page called \"X\""},
		{apperror.NewInternal(errors.New("dial tcp 10.0.0.5:3306")), "could not be checked; try again in a moment"},
		{errors.New("raw driver error"), "could not be checked; try again in a moment"},
	}
	for _, c := range cases {
		if got := planError(c.err); got != c.want {
			t.Errorf("planError(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

func TestLinkKinds_ReviewWording(t *testing.T) {
	cases := []struct {
		kind        Kind
		fields      map[string]any
		title, summ string
	}{
		{ShopStockKind(nil, nil), map[string]any{"shop": "The Rusty Anchor", "item": "Rope", "price": 1, "quantity": 5}, "Rope at The Rusty Anchor", "1 gp · 5 in stock"},
		{ShopStockKind(nil, nil), map[string]any{"shop": "The Rusty Anchor", "item": "Rope"}, "Rope at The Rusty Anchor", "no price or quantity given"},
		{ShopStockKind(nil, nil), map[string]any{"shop": "The Rusty Anchor", "item": "Rope", "today": true}, "Rope at The Rusty Anchor", "on today's shelf some days"},
		{CarriedItemKind(nil, nil, nil), map[string]any{"character": "Ser Aldric", "item": "Potion", "quantity": 2}, "Potion for Ser Aldric", "2 carried"},
		{CarriedItemKind(nil, nil, nil), map[string]any{"character": "Ser Aldric", "item": "Potion"}, "Potion for Ser Aldric", "1 carried"},
	}
	for _, c := range cases {
		lk := c.kind.(*linkKind)
		r := rec(lk.name, ActionCreate, "", c.fields, "")
		if got := lk.Title(r); got != c.title {
			t.Errorf("Title = %q, want %q", got, c.title)
		}
		if got := lk.summary(r); got != c.summ {
			t.Errorf("summary = %q, want %q", got, c.summ)
		}
	}
}

func TestBodyHTML_PageLinks(t *testing.T) {
	links := importer.NewPageLinks(context.Background(), linkPages{}, "camp", importer.Viewer{})
	got, err := bodyHTML("Ask @[Lyra] about @[Nobody].", links)
	if err != nil || got == nil {
		t.Fatalf("bodyHTML: %v", err)
	}
	if !strings.Contains(*got, `data-mention-id="p-1"`) || !strings.Contains(*got, "about Nobody.") {
		t.Errorf("got %s", *got)
	}
	plain, _ := bodyHTML("Ask @[Lyra].", nil)
	if !strings.Contains(*plain, "@[Lyra]") {
		t.Errorf("without links the text stays as written: %s", *plain)
	}
}

type linkPages struct{}

func (linkPages) GetBySlug(_ context.Context, c, slug string) (*entities.Entity, error) {
	if slug == "lyra" {
		return &entities.Entity{ID: "p-1", CampaignID: c, Name: "Lyra", Slug: "lyra"}, nil
	}
	return nil, nil
}
func (linkPages) CheckEntityAccess(context.Context, string, int, string) (*entities.EffectivePermission, error) {
	return &entities.EffectivePermission{CanView: true}, nil
}

// A GM-only marker in a record body would reach players unfiltered, so every
// kind's body is refused with the plain reason, at review and at commit.
func TestRecordBodies_RefuseGMMarkers(t *testing.T) {
	const reason = "DM-only passages only work in page text"
	body := "Feast. {gm{the mayor is the cult leader}gm}"
	reg := NewRegistry(&bodyKind{})
	_, p := reg.Plan(context.Background(), camp, owner, rec("event", ActionCreate, "Feast", map[string]any{"year": 1492, "month": 1, "day": 1}, body))
	if p.Error != reason {
		t.Errorf("plan error = %q", p.Error)
	}
	k := &bodyKind{}
	links := importer.NewPageLinks(context.Background(), nil, "c", importer.Viewer{Role: 3})
	for _, l := range []*importer.PageLinks{nil, links} {
		r := rec("event", ActionCreate, "Feast", map[string]any{"year": 1492, "month": 1, "day": 1}, body)
		r.Text = BodyText(l)
		err := k.Apply(context.Background(), camp, owner, r)
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Message != reason || k.wrote != 0 {
			t.Errorf("links=%v: apply err = %v, writes = %d", l != nil, err, k.wrote)
		}
	}
	if err := RefuseGMMarkers(rec("event", ActionCreate, "x", nil, "plain }gm}")); err == nil {
		t.Error("a stray close marker was accepted")
	}
	for name, r := range map[string]Record{
		"summary":     rec("game-night", ActionCreate, "x", map[string]any{"summary": "{gm{the mayor}gm}"}, ""),
		"nested list": rec("event", ActionCreate, "x", map[string]any{"items": []any{map[string]any{"description": "a {gm{b}gm}"}}}, ""),
		"name":        rec("tag", ActionCreate, "{gm{Cult}gm}", nil, ""),
	} {
		if err := RefuseGMMarkers(r); err == nil {
			t.Errorf("%s: a marker outside the body was accepted", name)
		}
	}
	if err := RefuseGMMarkers(rec("event", ActionCreate, "Feast", map[string]any{"summary": "plain", "year": 1492}, "plain")); err != nil {
		t.Errorf("plain record refused: %v", err)
	}
}

// bodyKind stores its record's body as HTML, as a feature's kind does
// through the Text the engine hands it.
type bodyKind struct{ wrote int }

func (*bodyKind) Name() string                                          { return "event" }
func (*bodyKind) Label() string                                         { return "Body" }
func (*bodyKind) Doc() string                                           { return "" }
func (*bodyKind) Plan(context.Context, string, Actor, Record) Plan      { return Plan{} }
func (*bodyKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
func (k *bodyKind) Apply(_ context.Context, _ string, _ Actor, r Record) error {
	if _, err := r.Text.HTML(r.Body); err != nil {
		return err
	}
	k.wrote++
	return nil
}

// The calendar's names generator and table rolls reach the tables through
// AddNames and FindTable.
func TestTableKind_AddNamesAndFindTable(t *testing.T) {
	ft := &fakeTables{doc: TableDoc{Tables: []Table{{ID: "omens", Name: "Omens"}}}}
	k := TableKind{Svc: ft}
	ctx := context.Background()
	if err := k.AddNames(ctx, camp, owner, "Villagers", []string{"Ada", "Bram", "Cole"}); err != nil {
		t.Fatal(err)
	}
	if len(ft.doc.Tables) != 2 || ft.doc.Tables[1].ID != "villagers" || len(ft.doc.Tables[1].Entries) != 3 {
		t.Fatalf("tables %+v", ft.doc)
	}
	if w := ft.doc.Tables[1].Entries[0].Weight; w != 1 {
		t.Fatalf("generated name weight %v, want 1 (0 is never rolled)", w)
	}
	if err := k.AddNames(ctx, camp, owner, "villagers", []string{"Dov"}); err != nil || len(ft.doc.Tables[1].Entries) != 4 {
		t.Fatalf("second run made another table: %v %+v", err, ft.doc)
	}
	for name, want := range map[string]string{"omens": "omens", "OMENS": "omens", "Villagers": "villagers", "Dragons": ""} {
		if got, err := k.FindTable(ctx, camp, name); err != nil || got != want {
			t.Errorf("FindTable(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := (TableKind{}).FindTable(ctx, camp, "omens"); err == nil {
		t.Error("unwired tables found a table")
	}
}

// stagedKind notes each row it plans cleanly, for the rows after it.
type stagedKind struct{ namedKind }

type stagedKey struct{}

func (stagedKind) Plan(ctx context.Context, _ string, _ Actor, r Record) Plan {
	seen, _ := ctx.Value(stagedKey{}).(string)
	if r.Name == "bad" {
		return Plan{Error: "bad"}
	}
	return Plan{Summary: "after " + seen}
}

func (stagedKind) Stage(ctx context.Context, _ string, _ Actor, r Record) context.Context {
	seen, _ := ctx.Value(stagedKey{}).(string)
	return context.WithValue(ctx, stagedKey{}, seen+r.Name)
}

// PlanAll plans each row with what the rows before it staged, and a row
// that failed its plan stages nothing.
func TestPlanAll_Staged(t *testing.T) {
	reg := NewRegistry(stagedKind{namedKind("thing")})
	recs := []Record{rec("thing", ActionCreate, "a", nil, ""), rec("thing", ActionCreate, "bad", nil, ""), rec("thing", ActionCreate, "b", nil, "")}
	_, plans := reg.PlanAll(context.Background(), camp, owner, recs)
	if plans[0].Summary != "after " || plans[1].Error != "bad" || plans[2].Summary != "after a" {
		t.Errorf("plans %+v", plans)
	}
}
