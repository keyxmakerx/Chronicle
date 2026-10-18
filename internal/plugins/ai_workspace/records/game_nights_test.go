package records

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeNights struct {
	list    []GameNightRef
	created []GameNightInput
	updates map[string]GameNightInput
	recaps  map[string]string
	deleted []string
}

func (f *fakeNights) List(context.Context, string) ([]GameNightRef, error) { return f.list, nil }
func (f *fakeNights) Create(_ context.Context, _ string, _ Actor, in GameNightInput) (string, error) {
	f.created = append(f.created, in)
	return "new", nil
}
func (f *fakeNights) Update(_ context.Context, id string, in GameNightInput) error {
	f.updates[id] = in
	return nil
}
func (f *fakeNights) SetRecap(_ context.Context, id string, j, h *string) error {
	f.recaps[id] = *h
	return nil
}
func (f *fakeNights) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func nights() *fakeNights {
	return &fakeNights{
		list: []GameNightRef{
			{ID: "s12", Name: "Session 12", Date: "2026-10-17", Status: "planned"},
			{ID: "x1", Name: "One-shot", Date: "2026-09-01", Status: "completed"},
			{ID: "x2", Name: "One-shot", Date: "2026-11-01", Status: "planned"},
		},
		updates: map[string]GameNightInput{}, recaps: map[string]string{},
	}
}

func night(action, name string, fields map[string]any, body string) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	return Record{Kind: "game-night", Action: action, Name: name, Fields: fields, Body: body}
}

func TestGameNightKind_Plan(t *testing.T) {
	tests := []struct {
		name    string
		r       Record
		a       Actor
		summary string
		err     string
	}{
		{"a new night", night(ActionCreate, "Session 13", map[string]any{"date": "2026-10-24", "time": "19:30"}, ""), owner,
			"on 2026-10-24 at 19:30; invites every member", ""},
		{"a date YAML read as a timestamp", night(ActionCreate, "Session 13", map[string]any{"date": time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)}, ""), owner,
			"on 2026-10-24", ""},
		{"a recap", night(ActionUpdate, "Session 12", map[string]any{"status": "played"}, "They won."), owner,
			"marked played; replaces the recap, which every member can read", ""},
		{"same name, picked by date", night(ActionUpdate, "One-shot", map[string]any{"date": "2026-11-01", "summary": "Pirates"}, ""), owner,
			`on 2026-11-01, summary "Pirates"`, ""},
		{"same name, no date", night(ActionUpdate, "One-shot", map[string]any{"summary": "Pirates"}, ""), owner,
			"", "add `date:` to pick one"},
		{"a second night of a name on another day", night(ActionCreate, "Session 12", map[string]any{"date": "2026-10-31"}, ""), owner,
			"on 2026-10-31", ""},
		{"already there", night(ActionCreate, "Session 12", nil, ""), owner, "", "already exists"},
		{"bad date", night(ActionCreate, "Session 13", map[string]any{"date": "next Friday"}, ""), owner, "", "is not a day written YYYY-MM-DD"},
		{"bad time", night(ActionCreate, "Session 13", map[string]any{"time": "7pm"}, ""), owner, "", "is not a 24-hour time"},
		{"bad status", night(ActionUpdate, "Session 12", map[string]any{"status": "maybe"}, ""), owner, "", "is not planned, played or cancelled"},
		{"nothing to change", night(ActionUpdate, "Session 12", nil, ""), owner, "", "nothing to change"},
		{"missing", night(ActionDelete, "Session 99", nil, ""), owner, "", "no game night called"},
		{"remove", night(ActionDelete, "Session 12", nil, ""), owner, "moves it to the trash", ""},
		{"players can't", night(ActionCreate, "Session 13", nil, ""), player, "", "only the owner or a co-DM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := GameNightKind{Svc: nights()}.Plan(context.Background(), camp, tt.a, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.summary) {
				t.Fatalf("plan = %+v, want summary containing %q", p, tt.summary)
			}
		})
	}
}

// An update changes only the keys written; a body becomes the recap.
func TestGameNightKind_Apply(t *testing.T) {
	f := nights()
	k := GameNightKind{Svc: f}
	ctx := context.Background()
	if err := k.Apply(ctx, camp, owner, night(ActionUpdate, "Session 12", map[string]any{"status": "played"}, "They **won**.")); err != nil {
		t.Fatal(err)
	}
	u := f.updates["s12"]
	if u.Status == nil || *u.Status != "completed" || u.Name != nil || u.Date != nil || u.Summary != nil {
		t.Errorf("update = %+v, want the status only", u)
	}
	if !strings.Contains(f.recaps["s12"], "<strong>won</strong>") {
		t.Errorf("recap = %q", f.recaps["s12"])
	}

	if err := k.Apply(ctx, camp, owner, night(ActionCreate, "Session 13", map[string]any{"date": "2026-10-24"}, "")); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 1 || *f.created[0].Name != "Session 13" || *f.created[0].Date != "2026-10-24" {
		t.Errorf("created = %+v", f.created)
	}
	if _, ok := f.recaps["new"]; ok {
		t.Error("a night with no body must get no recap")
	}

	if err := k.Apply(ctx, camp, owner, night(ActionDelete, "One-shot", map[string]any{"date": "2026-09-01"}, "")); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "x1" {
		t.Errorf("deleted = %v, want x1", f.deleted)
	}
}

func TestGameNightKind_Export(t *testing.T) {
	out, err := GameNightKind{Svc: nights()}.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"kind: game-night\nname: \"Session 12\"\ndate: 2026-10-17\nstatus: planned\n", "status: played"} {
		if !strings.Contains(out, w) {
			t.Errorf("export lacks %q:\n%s", w, out)
		}
	}
	if out, _ := (GameNightKind{Svc: nights()}).Export(context.Background(), camp, player); out != "" {
		t.Error("a player gets no game-night export")
	}
}
