package transformations

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

var (
	dm     = Viewer{UserID: "u-dm", VisibilityRole: permissions.RoleOwner}
	player = Viewer{UserID: "u-pl", VisibilityRole: permissions.RolePlayer}
)

func newTestService() (Service, *memRepo) {
	repo := newMemRepo()
	pages := fakePages{pages: map[string]fakePage{
		"mirela":  {campaign: "c1", playersSee: true, dmEdits: true},
		"secret":  {campaign: "c1", playersSee: false, dmEdits: true},
		"locked":  {campaign: "c1", playersSee: true, dmEdits: false},
		"foreign": {campaign: "c2", playersSee: true, dmEdits: true},
	}}
	return NewService(repo, pages), repo
}

func revenant() AddInput {
	return AddInput{Name: "Revenant", Source: "drawsteel:revenant", Changes: []Change{
		{Field: "ancestry", Op: OpSet, Value: json.RawMessage(`"Revenant"`)},
		{Field: "recoveries_max", Op: OpAdd, Value: json.RawMessage(`-2`)},
		{Field: "weaknesses", Op: OpSet, Value: json.RawMessage(`{ "fire": 5 }`)},
		{Field: "features_json", Op: OpAppend, Value: json.RawMessage(`{"name":"Tough But Withered"}`), Label: "Tough But Withered"},
		{Field: "features_json", Op: OpRemove, Value: json.RawMessage(`"Staying Power"`)},
	}}
}

func code(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func TestAddAndList(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	l, err := svc.Add(ctx, "c1", "mirela", dm, revenant())
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if l.Name != "Revenant" || len(l.Changes) != 5 || string(l.Changes[2].Value) != `{"fire":5}` {
		t.Fatalf("stored layer = %+v", l)
	}
	for _, tc := range []struct {
		name    string
		v       Viewer
		canEdit bool
	}{
		{"the Director", dm, true},
		{"a player who can see the page", player, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.ListForPage(ctx, "c1", "mirela", tc.v)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(got.Layers) != 1 || got.CanEdit != tc.canEdit {
				t.Fatalf("list = %+v", got)
			}
		})
	}
}

func TestWhoMayDoWhat(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		do   func(Service) error
		want int
	}{
		{"a player can't put a layer on", func(s Service) error { _, err := s.Add(ctx, "c1", "mirela", player, revenant()); return err }, 403},
		{"the Director can't on a page they may not edit", func(s Service) error { _, err := s.Add(ctx, "c1", "locked", dm, revenant()); return err }, 403},
		{"a hidden page reads as missing to a player", func(s Service) error { _, err := s.ListForPage(ctx, "c1", "secret", player); return err }, 404},
		{"another campaign's page is missing", func(s Service) error { _, err := s.ListForPage(ctx, "c1", "foreign", dm); return err }, 404},
		{"a bad page id is missing", func(s Service) error { _, err := s.ListForPage(ctx, "c1", "../x", dm); return err }, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestService()
			if got := code(tc.do(svc)); got != tc.want {
				t.Fatalf("code = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestService()
	l, err := svc.Add(ctx, "c1", "mirela", dm, revenant())
	if err != nil {
		t.Fatal(err)
	}
	if code(svc.Remove(ctx, "c1", l.ID, player)) != 403 {
		t.Fatal("a player took a layer off")
	}
	if code(svc.Remove(ctx, "c2", l.ID, dm)) != 404 {
		t.Fatal("a layer was reachable from another campaign")
	}
	if err := svc.Remove(ctx, "c1", l.ID, dm); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(repo.rows) != 0 {
		t.Fatal("layer still stored")
	}
	if code(svc.Remove(ctx, "c1", l.ID, dm)) != 404 {
		t.Fatal("removing twice should be not found")
	}
}

func TestLayerLimit(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	for i := 0; i < MaxLayersPerPage; i++ {
		if _, err := svc.Add(ctx, "c1", "mirela", dm, revenant()); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	if code(func() error { _, err := svc.Add(ctx, "c1", "mirela", dm, revenant()); return err }()) != 422 {
		t.Fatal("a page took more than the limit")
	}
}

func TestValidation(t *testing.T) {
	ctx := context.Background()
	with := func(edit func(*AddInput)) AddInput { in := revenant(); edit(&in); return in }
	for _, tc := range []struct {
		name string
		in   AddInput
	}{
		{"no name", with(func(in *AddInput) { in.Name = "  " })},
		{"a name with a control character", with(func(in *AddInput) { in.Name = "Vamp\x00ire" })},
		{"no changes", with(func(in *AddInput) { in.Changes = nil })},
		{"a field key with markup", with(func(in *AddInput) { in.Changes[0].Field = "<b>" })},
		{"a field key starting with a digit", with(func(in *AddInput) { in.Changes[0].Field = "1speed" })},
		{"an unknown op", with(func(in *AddInput) { in.Changes[0].Op = "delete" })},
		{"add with text", with(func(in *AddInput) { in.Changes[1].Value = json.RawMessage(`"two"`) })},
		{"remove with a number", with(func(in *AddInput) { in.Changes[4].Value = json.RawMessage(`3`) })},
		{"set with a list", with(func(in *AddInput) { in.Changes[0].Value = json.RawMessage(`[1]`) })},
		{"set with null", with(func(in *AddInput) { in.Changes[0].Value = json.RawMessage(`null`) })},
		{"a source with spaces", with(func(in *AddInput) { in.Source = "draw steel" })},
		{"an oversize value", with(func(in *AddInput) {
			in.Changes[0].Value = json.RawMessage(`"` + string(make([]byte, MaxValueBytes)) + `"`)
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newTestService()
			_, err := svc.Add(ctx, "c1", "mirela", dm, tc.in)
			if code(err) != 422 && code(err) != 400 {
				t.Fatalf("err = %v, want a validation error", err)
			}
			if len(repo.rows) != 0 {
				t.Fatal("an invalid layer was stored")
			}
		})
	}
}

type recordNotifier struct{ got []string }

func (r *recordNotifier) LayersChanged(campaignID, entityID string) {
	r.got = append(r.got, campaignID+"/"+entityID)
}

// Foundry redraws a hero's effects from this signal, so it must follow
// every stored change and never a refused one.
func TestNotifier(t *testing.T) {
	ctx := context.Background()
	n := &recordNotifier{}
	pages := fakePages{pages: map[string]fakePage{
		"mirela": {campaign: "c1", playersSee: true, dmEdits: true},
	}}
	svc := NewService(newMemRepo(), pages, WithNotifier(n))
	if _, err := svc.Add(ctx, "c1", "mirela", player, revenant()); err == nil {
		t.Fatal("a player put a layer on")
	}
	l, err := svc.Add(ctx, "c1", "mirela", dm, revenant())
	if err != nil {
		t.Fatal(err)
	}
	if code(svc.Remove(ctx, "c1", l.ID, player)) != 403 {
		t.Fatal("a player took a layer off")
	}
	if err := svc.Remove(ctx, "c1", l.ID, dm); err != nil {
		t.Fatal(err)
	}
	if len(n.got) != 2 || n.got[0] != "c1/mirela" || n.got[1] != "c1/mirela" {
		t.Fatalf("notified %v, want one put on and one taken off", n.got)
	}
}
