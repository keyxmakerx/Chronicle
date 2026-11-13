package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// viewerJournal stands in for the notes widget's own visibility rule: it
// answers only what the asking viewer may open, and records who asked, so a
// test can prove the player's own viewer, not the owner's, reached it.
type viewerJournal struct {
	all    []JournalNote
	asked  []permissions.Viewer
	opened func(v permissions.Viewer, n JournalNote) bool
}

func (j *viewerJournal) JournalNotes(_ context.Context, _ string, v permissions.Viewer) ([]JournalNote, error) {
	j.asked = append(j.asked, v)
	var out []JournalNote
	for _, n := range j.all {
		if j.opened(v, n) {
			out = append(out, n)
		}
	}
	return out, nil
}

// A player's key reads that player's own notes and what others shared with
// them, never another player's private note or a DM-only one; the owner's
// "what players see" key still gets no notes.
func TestJournal_PlayerKeyReadsOwnAndShared(t *testing.T) {
	journal := &viewerJournal{
		all: []JournalNote{
			{ID: "m1", Title: "My theory", AuthorID: "pl", Audience: "private", Body: strp("<p>Mine.</p>")},
			{ID: "m2", Title: "Party plan", AuthorID: "mira", Author: "Mira", Audience: "party", Body: strp("<p>We rest.</p>")},
			{ID: "m3", Title: "Mira's secret", AuthorID: "mira", Author: "Mira", Audience: "private"},
			{ID: "m4", Title: "For the DM", AuthorID: "mira", Author: "Mira", Audience: "gm"},
		},
		opened: func(v permissions.Viewer, n JournalNote) bool {
			return n.AuthorID == v.UserID() || n.Audience == "party"
		},
	}
	l := &Lookups{Journal: journal}
	playerKey := Actor{UserID: "pl", Role: 1, ReadsOwnNotes: true}

	tests := []struct {
		name    string
		r       Record
		a       Actor
		want    []string
		notWant []string
		errText string
	}{
		{"my_notes lists own and shared", lookup("notes", nil), playerKey,
			[]string{"My theory", "Party plan", "by Mira"}, []string{"Mira's secret", "For the DM", "; I can change"}, ""},
		{"read_note reads my own", lookup("note", map[string]any{"name": "My theory"}), playerKey,
			[]string{"Mine."}, nil, ""},
		{"read_note reads a shared one", lookup("note", map[string]any{"name": "Party plan"}), playerKey,
			[]string{"We rest."}, nil, ""},
		{"another player's private note is not found", lookup("note", map[string]any{"name": "Mira's secret"}), playerKey,
			nil, nil, "no note called"},
		{"a DM-only note is not found", lookup("note", map[string]any{"name": "For the DM"}), playerKey,
			nil, nil, "no note called"},
		{"the owner's players-view key still gets none", lookup("notes", nil), Actor{UserID: "owner", Role: 1},
			nil, nil, "your notes are left out"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans := l.Answer(context.Background(), camp, tt.a, []Record{tt.r})
			if len(ans) != 1 {
				t.Fatalf("%d answers", len(ans))
			}
			got := ans[0]
			if tt.errText != "" {
				if !strings.Contains(got.Error, tt.errText) {
					t.Fatalf("error = %q, want %q", got.Error, tt.errText)
				}
				return
			}
			if got.Error != "" {
				t.Fatalf("unexpected error %q", got.Error)
			}
			for _, w := range tt.want {
				if !strings.Contains(got.Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, got.Text)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got.Text, w) {
					t.Errorf("answer leaks %q:\n%s", w, got.Text)
				}
			}
		})
	}
	for _, v := range journal.asked {
		if v.UserID() != "pl" || v.Role() != 1 {
			t.Errorf("journal asked as %q role %d, want the player", v.UserID(), v.Role())
		}
	}
}
