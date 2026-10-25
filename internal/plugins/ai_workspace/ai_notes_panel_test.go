package ai_workspace

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// memNoteAccess is the switches in memory, per campaign.
type memNoteAccess map[string]records.NoteAccess

func (m memNoteAccess) Get(_ context.Context, campaignID string) (records.NoteAccess, error) {
	return m[campaignID], nil
}
func (m memNoteAccess) Set(_ context.Context, campaignID, _ string, a records.NoteAccess) error {
	m[campaignID] = a
	return nil
}

func renderNotes(t *testing.T, n NotesView) string {
	t.Helper()
	var buf bytes.Buffer
	if err := aiNotesPanel(n).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestAINotesPanel(t *testing.T) {
	cases := []struct {
		name    string
		n       NotesView
		want    []string
		notWant []string
	}{
		{"both off", NotesView{CampaignID: "camp-1", PlayerNotes: true},
			[]string{"Notes your AI can use", "Journal", "Jot notes", "Player Notes", "/campaigns/camp-1/ai/notes", `name="co_dms"`, `name="scribes"`, "coDMs: false, scribes: false", "Only the writer can change a Player Note"},
			[]string{"left out while your AI sees what players see", "Saved."}},
		{"co-DMs on, just saved", NotesView{CampaignID: "camp-1", Access: records.NoteAccess{CoDMs: true}, PlayerNotes: true, Saved: true},
			[]string{"coDMs: true, scribes: false", "Works from your AI"}, nil},
		{"Player Notes switched off", NotesView{CampaignID: "camp-1"},
			[]string{"Journal", "Jot notes"}, []string{"Player Notes", "Player Note,"}},
		{"AI sees what players see", NotesView{CampaignID: "camp-1", PlayerNotes: true, Shut: true},
			[]string{"Notes are left out while your AI sees what players see", "aic-notes-shut"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderNotes(t, tc.n)
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(html, w) {
					t.Errorf("should not show %q", w)
				}
			}
		})
	}
}

// The panel shows only with the store wired, and sits beside copy and paste
// whether or not a key exists.
func TestAIPage_NotesPanelPlacement(t *testing.T) {
	base := ConnectView{CampaignID: "camp-1", Available: true}
	if strings.Contains(renderConnect(t, base), "Notes your AI can use") {
		t.Error("panel shown without the switches wired")
	}
	base.Notes = &NotesView{CampaignID: "camp-1", PlayerNotes: true}
	if !strings.Contains(renderConnect(t, base), `id="ai-notes"`) {
		t.Error("panel missing with the switches wired")
	}
}

func TestChangeNoteAccess(t *testing.T) {
	store := memNoteAccess{}
	audit := &auditLog{}
	h := &Handler{noteAccess: store, audit: audit, featureGate: func(context.Context, string) records.OffFeatures {
		return records.OffFeatures{records.KindPlayerNote: "Player Notes"}
	}}

	rec := post(t, h.ChangeNoteAccess, "/x", url.Values{"co_dms": {"on"}})
	if rec.Code != 200 {
		t.Fatalf("code %d: %s", rec.Code, rec.Body)
	}
	if got := store["camp-1"]; !got.CoDMs || got.Scribes {
		t.Fatalf("saved %+v, want co-DMs only", got)
	}
	html := rec.Body.String()
	for _, w := range []string{`id="ai-notes"`, "coDMs: true, scribes: false", "Saved."} {
		if !strings.Contains(html, w) {
			t.Errorf("answer missing %q", w)
		}
	}
	if strings.Contains(html, "Player Notes") {
		t.Error("Player Notes row shown with the add-on off")
	}
	if len(audit.events) != 1 || !strings.Contains(audit.events[0], "campaign.ai_note_access.changed co_dms_from=false co_dms_to=true scribes_from=false scribes_to=false") {
		t.Errorf("audit = %v", audit.events)
	}

	// Saving the same again records nothing new; unticking both turns them off.
	post(t, h.ChangeNoteAccess, "/x", url.Values{"co_dms": {"on"}})
	if len(audit.events) != 1 {
		t.Errorf("an unchanged save was audited: %v", audit.events)
	}
	post(t, h.ChangeNoteAccess, "/x", url.Values{})
	if got := store["camp-1"]; got.CoDMs || got.Scribes {
		t.Errorf("after unticking both = %+v", got)
	}

	if rec := post(t, (&Handler{}).ChangeNoteAccess, "/x", url.Values{"co_dms": {"on"}}); rec.Code != -1 || !strings.Contains(rec.Body.String(), "aren't available") {
		t.Errorf("unwired store: %d %s", rec.Code, rec.Body)
	}
}
