package records

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// fakeAccess is the owner's switches and the campaign's writers.
type fakeAccess struct {
	access  NoteAccess
	writers map[string]NoteWriter
}

func (f *fakeAccess) NoteAccess(context.Context, string) (NoteAccess, error) { return f.access, nil }
func (f *fakeAccess) NoteWriter(_ context.Context, _, userID string) (NoteWriter, error) {
	return f.writers[userID], nil
}

func TestNoteAccess_Allows(t *testing.T) {
	tests := []struct {
		a      NoteAccess
		writer string
		want   bool
	}{
		{NoteAccess{}, WriterCoDM, false},
		{NoteAccess{CoDMs: true}, WriterCoDM, true},
		{NoteAccess{CoDMs: true}, WriterScribe, false},
		{NoteAccess{Scribes: true}, WriterScribe, true},
		{NoteAccess{CoDMs: true, Scribes: true}, WriterPlayer, false},
		{NoteAccess{CoDMs: true, Scribes: true}, "", false},
	}
	for _, tc := range tests {
		if got := tc.a.Allows(tc.writer); got != tc.want {
			t.Errorf("%+v.Allows(%q) = %v, want %v", tc.a, tc.writer, got, tc.want)
		}
	}
}

// TestNoteKind_SharedNotes: a note another member shared is changed only
// when the owner's switch for its writer is on, and then title and text
// only; never removed, never a player's, never one the owner can't view.
func TestNoteKind_SharedNotes(t *testing.T) {
	writers := map[string]NoteWriter{
		"mira": {Name: "Mira", Kind: WriterCoDM},
		"sam":  {Name: "Sam", Kind: WriterScribe},
		"tam":  {Name: "Tam", Kind: WriterPlayer},
	}
	base := func() []notes.Note {
		return []notes.Note{
			{ID: "codm", CampaignID: camp, UserID: "mira", Title: "Watch on the north road", SharedWithGM: true},
			{ID: "scribe", CampaignID: camp, UserID: "sam", Title: "Session log", IsShared: true},
			{ID: "player", CampaignID: camp, UserID: "tam", Title: "My secret", SharedWithGM: true},
			{ID: "private", CampaignID: camp, UserID: "mira", Title: "Mira private"},
			{ID: "mine", CampaignID: camp, UserID: "owner", Title: "Session log"},
		}
	}
	upd := func(name string, f map[string]any) Record { return rec("note", ActionUpdate, name, f, "New text.") }
	by := func(who string) map[string]any { return map[string]any{"by": who} }
	tests := []struct {
		name    string
		access  NoteAccess
		r       Record
		wantErr string
		updated string
	}{
		{"co-DM note, switch off", NoteAccess{}, upd("Watch on the north road", nil), "co-DMs share with you are read-only", ""},
		{"co-DM note, switch on", NoteAccess{CoDMs: true}, upd("Watch on the north road", nil), "", "codm"},
		{"co-DM switch doesn't open Scribes", NoteAccess{CoDMs: true}, upd("Session log", by("Sam")), "Scribes share with you are read-only", ""},
		{"Scribe note, switch on", NoteAccess{Scribes: true}, upd("Session log", by("sam")), "", "scribe"},
		{"my own note wins without by", NoteAccess{Scribes: true}, upd("Session log", nil), "", "mine"},
		{"never a player's", NoteAccess{CoDMs: true, Scribes: true}, upd("My secret", nil), "notes players write are read-only", ""},
		{"never a private note", NoteAccess{CoDMs: true}, upd("Mira private", nil), "you have no note", ""},
		{"never removed", NoteAccess{CoDMs: true}, rec("note", ActionDelete, "Watch on the north road", nil, ""), "only Mira can remove", ""},
		{"by never removes my own of the same title", NoteAccess{Scribes: true}, rec("note", ActionDelete, "Session log", by("Sam"), ""), "only Sam can remove", ""},
		{"by with no such writer removes nothing", NoteAccess{Scribes: true}, rec("note", ActionDelete, "Session log", by("Nobody"), ""), "you have no note", ""},
		{"by someone else matches nothing", NoteAccess{CoDMs: true}, upd("Watch on the north road", by("Sam")), "you have no note", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeNotes{notes: base()}
			k := NoteKind{Svc: f, Entities: fakeEntities{}, Access: &fakeAccess{access: tc.access, writers: writers}}
			err := k.Apply(context.Background(), camp, owner, tc.r)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				if len(f.updated)+len(f.deleted) != 0 {
					t.Fatal("a refused block reached the service")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(f.updated, ",") != tc.updated {
				t.Fatalf("updated %v, want %s", f.updated, tc.updated)
			}
		})
	}
}

// Changing a shared note sends its title and text only, even when the block
// asks to reshare it, and the review says the share is left out.
func TestNoteKind_SharedNoteTitleAndTextOnly(t *testing.T) {
	f := &fakeNotes{notes: []notes.Note{{ID: "codm", CampaignID: camp, UserID: "mira", Title: "Road", SharedWithGM: true}}}
	k := NoteKind{Svc: f, Entities: fakeEntities{}, Access: &fakeAccess{access: NoteAccess{CoDMs: true}, writers: map[string]NoteWriter{"mira": {Name: "Mira", Kind: WriterCoDM}}}}
	r := rec("note", ActionUpdate, "Road", map[string]any{"share": "party", "rename_to": "North road"}, "Bandits at dusk.")
	p := k.Plan(context.Background(), camp, owner, r)
	if !strings.Contains(p.Summary, "Mira (co-DM)") || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "only Mira can change who sees it") {
		t.Fatalf("plan %+v", p)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	u := f.lastUpdate
	if u.Visibility != nil || u.IsShared != nil || u.SharedWith != nil || u.ParentID != nil || u.Archived != nil || u.Pinned != nil {
		t.Errorf("sent owner-only fields: %+v", u)
	}
	if u.Title == nil || *u.Title != "North road" || u.EntryHTML == nil || !strings.Contains(*u.EntryHTML, "Bandits at dusk.") {
		t.Errorf("title or text missing: %+v", u)
	}
}

// What the review saw can change before Apply: a note no longer shared, a
// switch turned off, or the writer editing it right now each refuse.
func TestNoteKind_SharedNoteRechecks(t *testing.T) {
	now := time.Now()
	other := "mira"
	tests := []struct {
		name    string
		swap    func(n *notes.Note, a *fakeAccess)
		wantErr string
	}{
		{"unshared since", func(n *notes.Note, _ *fakeAccess) { n.SharedWithGM = false }, "no longer shared"},
		{"switch turned off", func(_ *notes.Note, a *fakeAccess) { a.access.CoDMs = false }, "read-only"},
		{"writer is editing it", func(n *notes.Note, _ *fakeAccess) { n.LockedBy, n.LockedAt = &other, &now }, "is editing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeNotes{notes: []notes.Note{{ID: "codm", CampaignID: camp, UserID: "mira", Title: "Road", SharedWithGM: true}}}
			acc := &fakeAccess{access: NoteAccess{CoDMs: true}, writers: map[string]NoteWriter{"mira": {Name: "Mira", Kind: WriterCoDM}}}
			k := NoteKind{Svc: &changesAfterList{fakeNotes: f, swap: func(n *notes.Note) { tc.swap(n, acc) }}, Entities: fakeEntities{}, Access: acc}
			err := k.Apply(context.Background(), camp, owner, rec("note", ActionUpdate, "Road", nil, "x"))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			if len(f.updated) != 0 {
				t.Fatal("the update reached the service")
			}
		})
	}
}

// changesAfterList hands the list the note as it was and GetByID the note
// as swap changes it, as if it changed during review.
type changesAfterList struct {
	*fakeNotes
	swap func(n *notes.Note)
}

func (c *changesAfterList) GetByID(ctx context.Context, id string) (*notes.Note, error) {
	n, err := c.fakeNotes.GetByID(ctx, id)
	if n != nil {
		cp := *n
		c.swap(&cp)
		return &cp, err
	}
	return n, err
}

// The lookups mark a shared note the AI may change, and say how; with the
// switch off, or for a Player Note, it stays read-only.
func TestLookups_ChangeableSharedNotes(t *testing.T) {
	tests := []struct {
		name    string
		access  NoteAccess
		r       Record
		want    []string
		notWant []string
	}{
		{"list marks it", NoteAccess{CoDMs: true}, lookup("notes", nil),
			[]string{"shared with the DM; I can change its title and text)"}, []string{"Can I buy rope? (by Mira; a Player Note on The Rusty Anchor; shared with the DM; I can"}},
		{"note says how", NoteAccess{CoDMs: true}, lookup("note", map[string]any{"name": "Session 12, 2026-10-09", "by": "Mira"}),
			[]string{"Written by Mira (co-DM) and shared with me", "`by: Mira` can change its title and text"}, []string{"no block can change it"}},
		{"switch off", NoteAccess{Scribes: true}, lookup("note", map[string]any{"name": "Session 12, 2026-10-09", "by": "Mira"}),
			[]string{"no block can change it"}, []string{"can change its title and text"}},
		{"a Player Note never", NoteAccess{CoDMs: true}, lookup("note", map[string]any{"name": "Can I buy rope?"}),
			[]string{"no block can change it"}, []string{"can change its title and text"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := world()
			l.NoteAccess = &fakeAccess{access: tc.access, writers: map[string]NoteWriter{"mira": {Name: "Mira", Kind: WriterCoDM}}}
			ans := l.Answer(context.Background(), camp, owner, []Record{tc.r})
			if len(ans) != 1 || ans[0].Error != "" {
				t.Fatalf("answers %+v", ans)
			}
			for _, w := range tc.want {
				if !strings.Contains(ans[0].Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, ans[0].Text)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(ans[0].Text, w) {
					t.Errorf("answer has %q:\n%s", w, ans[0].Text)
				}
			}
		})
	}
}
