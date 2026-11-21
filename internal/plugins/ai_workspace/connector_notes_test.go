package ai_workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// finderLookup is a stubLookup that can also search, as the real lookup does,
// by the role that asks: a page needing more than the reader's role is not
// returned.
type finderLookup struct {
	stubLookup
	pages   []entities.Entity
	needs   map[string]int // page ID → lowest role that may see it
	gotRole int
}

func (f *finderLookup) Search(_ context.Context, _, _ string, _ int, role int, _ string, _ entities.ListOptions) ([]entities.Entity, int, error) {
	f.gotRole = role
	var out []entities.Entity
	for _, e := range f.pages {
		if role >= f.needs[e.ID] {
			out = append(out, e)
		}
	}
	return out, len(out), nil
}

func guidePage(id, name, html string) entities.Entity {
	e := entities.Entity{ID: id, Name: name, Slug: entities.Slugify(name)}
	e.EntryHTML = &html
	return e
}

// The AI is told which existing page a new one matches, by the same rule the
// review uses and with the reader its own key gives it.
func TestConnectorNotes(t *testing.T) {
	const secret = "the dragon sleeps under the chapel"
	guide := guidePage("1", "New Player Guide", `<p>Welcome.</p><p><span data-secret="true">`+secret+`</span></p>`)
	twin := pageBlock("New Player Guide (Imported)", "Welcome.")
	tests := []struct {
		name    string
		conn    connector.Connection
		changes string
		needs   map[string]int
		want    []string
		absent  []string
		role    int
	}{
		{
			name: "an owner's key is told the page and how to update it", conn: connector.Connection{CampaignID: "camp-1", UserID: "owner-1", Privacy: aiexport.PrivacyModePermitted},
			changes: twin, role: int(campaigns.RoleOwner),
			want:   []string{"block 1:", `"New Player Guide (Imported)"`, `"New Player Guide"`, "use an update block with name: \"New Player Guide\""},
			absent: []string{secret, "dragon"},
		},
		{
			name: "a Safe key searches as a player and is not told a hidden page", conn: connector.Connection{CampaignID: "camp-1", UserID: "owner-1", Privacy: aiexport.PrivacyModeSafe},
			changes: twin, needs: map[string]int{"1": int(campaigns.RoleOwner)}, role: int(campaigns.RolePlayer),
			absent: []string{"New Player Guide", "closely matches"},
		},
		{
			name: "a player's key is not advised", conn: connector.Connection{CampaignID: "camp-1", UserID: "p-1", ForPlayer: true},
			changes: twin, role: 0,
		},
		{
			name: "an update or an unrelated page says nothing", conn: connector.Connection{CampaignID: "camp-1", UserID: "owner-1", Privacy: aiexport.PrivacyModePermitted},
			changes: "---\nname: New Player Guide\naction: update\n---\n\nWelcome.\n\n" + pageBlock("Harbour rumours", "Gulls."), role: int(campaigns.RoleOwner),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &finderLookup{pages: []entities.Entity{guide}, needs: tt.needs}
			h := NewHandler(nil)
			h.SetImportLookup(f)
			got := strings.Join(h.ConnectorNotes(context.Background(), &tt.conn, tt.changes), "\n")
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("notes are missing %q:\n%s", w, got)
				}
			}
			for _, w := range tt.absent {
				if strings.Contains(got, w) {
					t.Errorf("notes should not contain %q:\n%s", w, got)
				}
			}
			if tt.role != 0 && f.gotRole != tt.role {
				t.Errorf("searched as role %d, want %d", f.gotRole, tt.role)
			}
			if tt.want == nil && got != "" {
				t.Errorf("notes = %q, want none", got)
			}
		})
	}
}

func TestConnectorNotes_CapAndSlug(t *testing.T) {
	e := guidePage("1", "New Player Guide", "<p>Welcome.</p>")
	e.Slug = "old-slug"
	f := &finderLookup{pages: []entities.Entity{e}}
	h := NewHandler(nil)
	h.SetImportLookup(f)
	conn := &connector.Connection{CampaignID: "camp-1", UserID: "owner-1", Privacy: aiexport.PrivacyModePermitted}
	got := strings.Join(h.ConnectorNotes(context.Background(), conn, pageBlock("New Player Guide (Imported)", "Welcome.")), "\n")
	if !strings.Contains(got, "can't change that page by its name") || strings.Contains(got, "update block") {
		t.Errorf("a page that can't be found by name gets no update advice: %q", got)
	}

	f.pages[0].Slug = "new-player-guide"
	var many strings.Builder
	for i := 0; i < maxMatchNotes+3; i++ {
		many.WriteString(pageBlock("New Player Guide (Imported)", "Welcome.") + "\n")
	}
	notes := h.ConnectorNotes(context.Background(), conn, many.String())
	if len(notes) != maxMatchNotes+1 || !strings.Contains(notes[len(notes)-1], "and 3 more") {
		t.Errorf("notes = %d, last %q", len(notes), notes[len(notes)-1])
	}
}
