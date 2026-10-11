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
	rows      []systems.SystemEntry
	created   []systems.CreateSystemEntryInput
	updated   map[int64]systems.UpdateSystemEntryInput
	deleted   []int64
	checkErr  error
	lastActor systems.EntryActor
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
func (f *fakeEntrySvc) CheckCreate(_ context.Context, _ string, _ systems.EntryActor, _ systems.CreateSystemEntryInput) error {
	return f.checkErr
}
func (f *fakeEntrySvc) Create(_ context.Context, _ string, a systems.EntryActor, in systems.CreateSystemEntryInput) (*systems.SystemEntry, error) {
	f.lastActor = a
	f.created = append(f.created, in)
	return &systems.SystemEntry{}, nil
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
		{"update bad trait list refused", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "purchased_traits": []any{"Stone Skin"}}, ""), nil, "needs a name"},
		{"update nested detail refused", owner, rec("system-entry", "update", "Stoneborn", map[string]any{"entry": "ancestry", "x": map[string]any{"a": 1}}, ""), nil, "must be text"},
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
