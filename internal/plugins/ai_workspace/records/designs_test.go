package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeDesigns is an in-memory library; Check refuses data naming "lava", as a
// kind's validator would refuse an unknown part.
type fakeDesigns struct {
	list   []DesignRef
	sys    string
	pkg    []DesignRef
	saved  []DesignValue
	rename map[string][2]*string
}

func (f *fakeDesigns) List(context.Context, string, Actor) ([]DesignRef, error) { return f.list, nil }
func (f *fakeDesigns) Check(_ context.Context, _ string, _ Actor, v DesignValue) error {
	if strings.Contains(string(v.Data), "lava") {
		return apperror.NewBadRequest("unknown part: lava")
	}
	if v.Kind != "room-theme" {
		return apperror.NewBadRequest(v.Kind + " is not a kind of design")
	}
	return nil
}
func (f *fakeDesigns) Save(_ context.Context, _ string, _ Actor, v DesignValue) error {
	f.saved = append(f.saved, v)
	return nil
}
func (f *fakeDesigns) Package(context.Context, string, Actor) (string, []DesignRef, error) {
	return f.sys, f.pkg, nil
}
func (f *fakeDesigns) Rename(_ context.Context, _ string, _ Actor, id string, name, note *string) error {
	f.rename[id] = [2]*string{name, note}
	return nil
}

func designKind(owner bool) (DesignKind, *fakeDesigns) {
	f := &fakeDesigns{rename: map[string][2]*string{}, list: []DesignRef{{ID: "d1", Kind: "room-theme", Name: "Cinderhold Forge", Note: "warm", SystemID: "drawsteel", Maker: "Alice", Shared: true, Data: json.RawMessage(`{"version":1}`)}}}
	return DesignKind{Svc: f, Owners: fakeOwners{"u1": owner}}, f
}

var designOwner = Actor{UserID: "u1", Role: 3}

func TestDesignKindPlans(t *testing.T) {
	tests := []struct {
		name    string
		actor   Actor
		owner   bool
		rec     Record
		wantErr string
		wantSum string
	}{
		{"create", designOwner, true, Record{Action: ActionCreate, Name: "Brass Hall", Fields: map[string]any{"type": "room-theme", "data": map[string]any{"version": 1}}}, "", "saves the room-theme design \"Brass Hall\""},
		{"create with the data as JSON text", designOwner, true, Record{Action: ActionCreate, Name: "Brass Hall", Fields: map[string]any{"type": "room-theme", "data": `{"version":1}`}}, "", "saves the room-theme"},
		{"a co-dm is refused", Actor{UserID: "u1", Role: 3}, false, Record{Action: ActionCreate, Name: "x", Fields: map[string]any{"type": "room-theme", "data": map[string]any{}}}, "only the campaign's owner", ""},
		{"a player is refused", Actor{UserID: "u2", Role: 1}, true, Record{Action: ActionCreate, Name: "x", Fields: map[string]any{"type": "room-theme", "data": map[string]any{}}}, "only the campaign's owner", ""},
		{"no name", designOwner, true, Record{Action: ActionCreate, Fields: map[string]any{"type": "room-theme"}}, "needs a `name`", ""},
		{"no type", designOwner, true, Record{Action: ActionCreate, Name: "x", Fields: map[string]any{"data": map[string]any{}}}, "type:", ""},
		{"no data", designOwner, true, Record{Action: ActionCreate, Name: "x", Fields: map[string]any{"type": "room-theme"}}, "data:", ""},
		{"the kind's refusal is shown", designOwner, true, Record{Action: ActionCreate, Name: "x", Fields: map[string]any{"type": "room-theme", "data": map[string]any{"part": "lava"}}}, "unknown part: lava", ""},
		{"an unknown type is refused", designOwner, true, Record{Action: ActionCreate, Name: "x", Fields: map[string]any{"type": "poster", "data": map[string]any{}}}, "not a kind of design", ""},
		{"a name already saved", designOwner, true, Record{Action: ActionCreate, Name: "cinderhold forge", Fields: map[string]any{"type": "room-theme", "data": map[string]any{}}}, "already saved", ""},
		{"rename", designOwner, true, Record{Action: ActionUpdate, Name: "Cinderhold Forge", Fields: map[string]any{"rename": "Cinderhold Smithy"}}, "", "updates the design \"Cinderhold Forge\": name \"Cinderhold Smithy\""},
		{"update of a missing design", designOwner, true, Record{Action: ActionUpdate, Name: "Nope", Fields: map[string]any{"rename": "x"}}, "no design called", ""},
		{"update cannot change data", designOwner, true, Record{Action: ActionUpdate, Name: "Cinderhold Forge", Fields: map[string]any{"data": map[string]any{}}}, "not changed", ""},
		{"update naming nothing", designOwner, true, Record{Action: ActionUpdate, Name: "Cinderhold Forge", Fields: map[string]any{}}, "", "changes nothing"},
		{"delete is refused", designOwner, true, Record{Action: ActionDelete, Name: "Cinderhold Forge"}, "can't remove", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, _ := designKind(tt.owner)
			p := k.Plan(context.Background(), "c1", tt.actor, tt.rec)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error = %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.wantSum) {
				t.Fatalf("plan = %+v, want summary %q", p, tt.wantSum)
			}
		})
	}
}

func TestDesignKindApply(t *testing.T) {
	k, f := designKind(true)
	ctx := context.Background()
	if err := k.Apply(ctx, "c1", designOwner, Record{Action: ActionCreate, Name: "Brass Hall", Fields: map[string]any{"type": "room-theme", "note": "cool", "data": map[string]any{"version": 1}}}); err != nil {
		t.Fatal(err)
	}
	if len(f.saved) != 1 || f.saved[0].Name != "Brass Hall" || f.saved[0].Note != "cool" || string(f.saved[0].Data) != `{"version":1}` {
		t.Errorf("saved = %+v", f.saved)
	}
	// A rename sends only what was written: the note stays as it is.
	if err := k.Apply(ctx, "c1", designOwner, Record{Action: ActionUpdate, Name: "Cinderhold Forge", Fields: map[string]any{"rename": "Smithy"}}); err != nil {
		t.Fatal(err)
	}
	got := f.rename["d1"]
	if got[0] == nil || *got[0] != "Smithy" || got[1] != nil {
		t.Errorf("rename = %v %v", got[0], got[1])
	}
	if err := k.Apply(ctx, "c1", Actor{UserID: "u2", Role: 1}, Record{Action: ActionCreate, Name: "x", Fields: map[string]any{"type": "room-theme", "data": map[string]any{}}}); err == nil || len(f.saved) != 1 {
		t.Error("a player's block wrote a design")
	}
}

func TestDesignsLookup(t *testing.T) {
	k, f := designKind(true)
	l := DesignsLookup{Svc: f, Owners: k.Owners}
	chip, text, err := l.Answer(context.Background(), "c1", designOwner, Record{})
	if err != nil || chip != "Saved designs" {
		t.Fatalf("chip %q err %v", chip, err)
	}
	for _, want := range []string{"## Saved designs (1)", "Cinderhold Forge", "game system drawsteel", "made by Alice", "shared", "kind: design", "type: room-theme", `data: {"version":1}`, `note: "warm"`} {
		if !strings.Contains(text, want) {
			t.Errorf("answer lacks %q:\n%s", want, text)
		}
	}
	if _, _, err := l.Answer(context.Background(), "c1", Actor{UserID: "u2", Role: 1}, Record{}); err == nil {
		t.Error("a player read the library")
	}
	if info := l.LookupInfo(); info.What != "designs" || !info.Tool.Owner {
		t.Errorf("info = %+v", info)
	}
}

func TestDesignsLookupShowsPackageReadOnly(t *testing.T) {
	k, f := designKind(true)
	f.sys = "Draw Steel"
	f.pkg = []DesignRef{{ID: "pkg-1", Kind: "room-theme", Name: "Ashgate Forge", Note: "shipped", Maker: "MCDM fan", Data: json.RawMessage(`{"version":1}`)}}
	l := DesignsLookup{Svc: f, Owners: k.Owners}
	_, text, err := l.Answer(context.Background(), "c1", designOwner, Record{})
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(text, "## From Draw Steel (1, read only)")
	if i < 0 {
		t.Fatalf("no package section:\n%s", text)
	}
	section := text[i:]
	for _, want := range []string{"Ashgate Forge", "made by MCDM fan", `{"version":1}`} {
		if !strings.Contains(section, want) {
			t.Errorf("section lacks %q", want)
		}
	}
	// A package design offers no block, so nothing can be sent back to change it.
	if strings.Contains(section, "kind: design") {
		t.Error("a package design was offered as a changeable block")
	}
	// The AI kind cannot rename a package design: its id is not a library row.
	if err := k.Apply(context.Background(), "c1", designOwner, Record{Action: ActionUpdate, Name: "Ashgate Forge", Fields: map[string]any{"rename": "x"}}); err == nil && len(f.rename) > 0 {
		t.Error("a package design was renamed")
	}
}
