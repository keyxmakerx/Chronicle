package sessions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

var (
	aiOwner  = aiplug.Actor{UserID: "owner", Role: 3}
	aiPlayer = aiplug.Actor{UserID: "pl", Role: 1}
)

type fakeNights struct {
	list    []aiNight
	created []aiNightInput
	updates map[string]aiNightInput
	recaps  map[string]string
	deleted []string
}

func (f *fakeNights) List(context.Context, string) ([]aiNight, error) { return f.list, nil }
func (f *fakeNights) Create(_ context.Context, _ string, _ aiplug.Actor, in aiNightInput) (string, error) {
	f.created = append(f.created, in)
	return "new", nil
}
func (f *fakeNights) Update(_ context.Context, _, id string, in aiNightInput) error {
	f.updates[id] = in
	return nil
}
func (f *fakeNights) SetRecap(_ context.Context, id string, _, h *string) error {
	f.recaps[id] = *h
	return nil
}
func (f *fakeNights) Delete(_ context.Context, _, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func aiNights() *fakeNights {
	return &fakeNights{
		list: []aiNight{
			{ID: "s12", Name: "Session 12", Date: "2026-10-17", Status: "planned"},
			{ID: "x1", Name: "One-shot", Date: "2026-09-01", Status: "completed"},
			{ID: "x2", Name: "One-shot", Date: "2026-11-01", Status: "planned"},
		},
		updates: map[string]aiNightInput{}, recaps: map[string]string{},
	}
}

// fakeText stands in for the engine's renderer, which owns markdown, links
// and GM-only markers; the AI part only has to hand the body over.
type fakeText struct{}

func (fakeText) HTML(body string) (*string, error) { h := "<p>" + body + "</p>"; return &h, nil }
func (fakeText) Entry(body string) (*string, *string, error) {
	j, h := `{"type":"doc"}`, "<p>"+body+"</p>"
	return &j, &h, nil
}

func night(action, name string, fields map[string]any, body string) aiplug.Record {
	if fields == nil {
		fields = map[string]any{}
	}
	return aiplug.Record{Kind: "game-night", Action: action, Name: name, Fields: fields, Body: body, Text: fakeText{}}
}

func TestAIGameNights_Plan(t *testing.T) {
	tests := []struct {
		name    string
		r       aiplug.Record
		a       aiplug.Actor
		summary string
		err     string
	}{
		{"a new night", night(aiplug.ActionCreate, "Session 13", map[string]any{"date": "2026-10-24", "time": "19:30"}, ""), aiOwner,
			"on 2026-10-24 at 19:30; invites every member", ""},
		{"a date YAML read as a timestamp", night(aiplug.ActionCreate, "Session 13", map[string]any{"date": time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)}, ""), aiOwner,
			"on 2026-10-24", ""},
		{"a recap", night(aiplug.ActionUpdate, "Session 12", map[string]any{"status": "played"}, "They won."), aiOwner,
			"marked played; replaces the recap, which every member can read", ""},
		{"same name, picked by date", night(aiplug.ActionUpdate, "One-shot", map[string]any{"date": "2026-11-01", "summary": "Pirates"}, ""), aiOwner,
			`on 2026-11-01, summary "Pirates"`, ""},
		{"same name, no date", night(aiplug.ActionUpdate, "One-shot", map[string]any{"summary": "Pirates"}, ""), aiOwner,
			"", "add `date:` to pick one"},
		{"a second night of a name on another day", night(aiplug.ActionCreate, "Session 12", map[string]any{"date": "2026-10-31"}, ""), aiOwner,
			"on 2026-10-31", ""},
		{"already there", night(aiplug.ActionCreate, "Session 12", nil, ""), aiOwner, "", "already exists"},
		{"bad date", night(aiplug.ActionCreate, "Session 13", map[string]any{"date": "next Friday"}, ""), aiOwner, "", "is not a day written YYYY-MM-DD"},
		{"bad time", night(aiplug.ActionCreate, "Session 13", map[string]any{"time": "7pm"}, ""), aiOwner, "", "is not a 24-hour time"},
		{"bad status", night(aiplug.ActionUpdate, "Session 12", map[string]any{"status": "maybe"}, ""), aiOwner, "", "is not planned, played or cancelled"},
		{"nothing to change", night(aiplug.ActionUpdate, "Session 12", nil, ""), aiOwner, "", "nothing to change"},
		{"missing", night(aiplug.ActionDelete, "Session 99", nil, ""), aiOwner, "", "no game night called"},
		{"remove", night(aiplug.ActionDelete, "Session 12", nil, ""), aiOwner, "moves it to the trash", ""},
		{"players can't", night(aiplug.ActionCreate, "Session 13", nil, ""), aiPlayer, "", "only the owner or a co-DM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := (&AIGameNights{store: aiNights()}).Plan(context.Background(), "c", tt.a, tt.r)
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
func TestAIGameNights_Apply(t *testing.T) {
	f := aiNights()
	k := &AIGameNights{store: f}
	ctx := context.Background()
	if err := k.Apply(ctx, "c", aiOwner, night(aiplug.ActionUpdate, "Session 12", map[string]any{"status": "played"}, "They won.")); err != nil {
		t.Fatal(err)
	}
	u := f.updates["s12"]
	if u.Status == nil || *u.Status != StatusCompleted || u.Name != nil || u.Date != nil || u.Summary != nil {
		t.Errorf("update = %+v, want the status only", u)
	}
	if f.recaps["s12"] != "<p>They won.</p>" {
		t.Errorf("recap = %q, want the body through the engine's Text", f.recaps["s12"])
	}

	if err := k.Apply(ctx, "c", aiOwner, night(aiplug.ActionCreate, "Session 13", map[string]any{"date": "2026-10-24"}, "")); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 1 || *f.created[0].Name != "Session 13" || *f.created[0].Date != "2026-10-24" {
		t.Errorf("created = %+v", f.created)
	}
	if _, ok := f.recaps["new"]; ok {
		t.Error("a night with no body must get no recap")
	}

	if err := k.Apply(ctx, "c", aiOwner, night(aiplug.ActionDelete, "One-shot", map[string]any{"date": "2026-09-01"}, "")); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "x1" {
		t.Errorf("deleted = %v, want x1", f.deleted)
	}

	// A body with no renderer is refused, never stored as raw markdown.
	r := night(aiplug.ActionUpdate, "Session 12", nil, "raw **text**")
	r.Text = nil
	if err := k.Apply(ctx, "c", aiOwner, r); err == nil {
		t.Error("a body was applied without the engine's Text")
	}
}

func TestAIGameNights_Export(t *testing.T) {
	out, err := (&AIGameNights{store: aiNights()}).Export(context.Background(), "c", aiOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"kind: game-night\nname: \"Session 12\"\ndate: 2026-10-17\nstatus: planned\n", "status: played"} {
		if !strings.Contains(out, w) {
			t.Errorf("export lacks %q:\n%s", w, out)
		}
	}
	if out, _ := (&AIGameNights{store: aiNights()}).Export(context.Background(), "c", aiPlayer); out != "" {
		t.Error("a player gets no game-night export")
	}
}

func TestAIGameNights_Lookup(t *testing.T) {
	f := aiNights()
	f.list[0].Time = "19:30"
	f.list[0].Summary = "The siege of Harrowmere"
	k := &AIGameNights{store: f}
	chip, text, err := k.Answer(context.Background(), "c", aiOwner, aiplug.Record{})
	if err != nil {
		t.Fatal(err)
	}
	if chip != "Game nights · 3" {
		t.Errorf("chip = %q", chip)
	}
	for _, want := range []string{
		"## Game nights (3)",
		"- Session 12: 2026-10-17 19:30, planned. The siege of Harrowmere",
		"- One-shot: 2026-09-01, played",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("answer lacks %q:\n%s", want, text)
		}
	}
	if _, text, err := k.Answer(context.Background(), "c", aiPlayer, aiplug.Record{}); err == nil || !strings.Contains(err.Error(), "choose Permitted") || text != "" {
		t.Errorf("a player got %q, %v", text, err)
	}
	in := k.LookupInfo()
	if in.What != "game-nights" || in.Tool.Name != "game_nights" || in.Tool.Owner {
		t.Errorf("lookup info = %+v", in)
	}
}

// ringSvc is the sessions service as serviceNights calls it: who had
// answered the night, and the bells rung.
type ringSvc struct {
	SessionService
	responded []string
	bells     []string
	moves     int
}

func (r *ringSvc) MoveSession(context.Context, string, UpdateSessionInput) (*Session, []string, error) {
	r.moves++
	return nil, r.responded, nil
}
func (r *ringSvc) DeleteSession(context.Context, string) ([]string, error) { return r.responded, nil }
func (r *ringSvc) GetSession(_ context.Context, id string) (*Session, error) {
	return &Session{ID: id, Name: "Session 12"}, nil
}
func (r *ringSvc) NotifyUsers(_ context.Context, ids []string, campaignID, ntype, msg, link string) error {
	r.bells = append(r.bells, strings.Join(ids, ",")+"|"+campaignID+"|"+ntype+"|"+msg+"|"+link)
	return nil
}

// An approved AI change to a night tells the people who had answered, the
// way the owner's own move or cancel does, so their answer is checked again
// rather than left on a date that no longer holds.
func TestServiceNights_RingsWhoAnswered(t *testing.T) {
	date := "2026-10-18"
	tests := []struct {
		name      string
		responded []string
		act       func(serviceNights) error
		want      []string
	}{
		{"a move asks who answered to check again", []string{"u1", "u2"},
			func(g serviceNights) error {
				return g.Update(context.Background(), "c", "s12", aiNightInput{Date: &date})
			},
			[]string{`u1,u2|c|` + NotifSessionMoved + `|"Session 12" was moved. Please check your answer.|/campaigns/c/sessions/s12`}},
		{"a removal tells who answered it is cancelled", []string{"u1"},
			func(g serviceNights) error { return g.Delete(context.Background(), "c", "s12") },
			[]string{`u1|c|` + NotifSessionCancelled + `|"Session 12" was cancelled.|/campaigns/c/game-nights`}},
		{"nobody answered, no bell", nil,
			func(g serviceNights) error {
				return g.Update(context.Background(), "c", "s12", aiNightInput{Date: &date})
			},
			nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &ringSvc{responded: tt.responded}
			if err := tt.act(serviceNights{svc: svc}); err != nil {
				t.Fatal(err)
			}
			if strings.Join(svc.bells, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("bells = %q, want %q", svc.bells, tt.want)
			}
		})
	}
}
