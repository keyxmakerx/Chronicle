// mention_graph_integration_test.go drives the real repository and service
// against a live MariaDB: which @mention links a viewer is given for the
// relations graph, and that a page's name only travels with a link the
// viewer may know of.
package entities

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestMentionGraph_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}

	db := openTestDB(t)
	ctx := context.Background()
	entityRepo := NewEntityRepository(db)
	svc := NewEntityService(entityRepo, NewEntityTypeRepository(db), nil)

	ownerID := testUUID(t)
	campaignID, otherCampaignID := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		ownerID, "mention-graph-"+ownerID+"@example.test", "Mention Graph Test", "x")
	for _, id := range []string{campaignID, otherCampaignID} {
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			id, "Mention Graph Test", "mention-graph-"+id[:8], ownerID)
	}
	t.Cleanup(func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id IN (?, ?)`, campaignID, otherCampaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, ownerID)
	})

	typeFor := func(campaign string) int {
		res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon, color) VALUES (?,?,?,?,?,?)`,
			campaign, "mention-graph-place", "Place", "Places", "fa-map-pin", "#336699")
		if err != nil {
			t.Fatalf("seed entity type: %v", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("entity type last insert id: %v", err)
		}
		return int(id)
	}
	typeID, otherTypeID := typeFor(campaignID), typeFor(otherCampaignID)

	now := time.Now().UTC()
	page := func(campaign string, typeID int, name, html string) *Entity {
		e := &Entity{
			ID: testUUID(t), CampaignID: campaign, EntityTypeID: typeID,
			Name: name, Slug: strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + testUUID(t)[:8],
			FieldsData: map[string]any{}, CreatedBy: ownerID, CreatedAt: now, UpdatedAt: now,
		}
		if html != "" {
			e.EntryHTML = &html
		}
		if err := entityRepo.Create(ctx, e); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return e
	}
	link := func(e *Entity) string {
		return `<a data-mention-id="` + e.ID + `" href="/e/` + e.ID + `">@` + e.Name + `</a>`
	}

	keep := page(campaignID, typeID, "Tidewatch Keep", "")
	ledger := page(campaignID, typeID, "Smugglers Ledger", "")
	trashed := page(campaignID, typeID, "Burned Chart", "")
	hidden := page(campaignID, typeID, "Vault of Names", "")
	elsewhere := page(otherCampaignID, otherTypeID, "Far Harbour", "")
	mustExec(t, db, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, trashed.ID)
	mustExec(t, db, `UPDATE entities SET is_private = true WHERE id = ?`, hidden.ID)

	// The Watch names the Keep in plain text, the Ledger only inside DM-only
	// text, and three pages nobody but the DM (or nobody at all) may reach.
	watch := page(campaignID, typeID, "Harbour Watch",
		`<p>Posted at `+link(keep)+`. <span data-secret="true">The `+link(ledger)+` is under the floor.</span> `+
			link(trashed)+` `+link(hidden)+` `+link(elsewhere)+`</p>`)
	// A hidden page that names the Keep: its link is the DM's alone.
	plot := page(campaignID, typeID, "The Real Plot", `<p>It ends at `+link(keep)+`.</p>`)
	mustExec(t, db, `UPDATE entities SET is_private = true WHERE id = ?`, plot.ID)

	pair := func(a, b *Entity) string { return a.ID + ">" + b.ID }
	tests := []struct {
		name        string
		role        int
		userID      string
		seesSecrets bool
		wantLinks   []string
		wantPages   []*Entity
		never       []*Entity
	}{
		{
			"the DM gets every link between this campaign's live pages", permissions.RoleOwner, ownerID, true,
			[]string{pair(watch, keep), pair(watch, ledger), pair(watch, hidden), pair(plot, keep)},
			[]*Entity{watch, keep, ledger, hidden, plot},
			[]*Entity{trashed, elsewhere},
		},
		{
			"a player gets only the link written in plain text between pages they can open", permissions.RolePlayer, testUUID(t), false,
			[]string{pair(watch, keep)},
			[]*Entity{watch, keep},
			[]*Entity{ledger, trashed, hidden, elsewhere, plot},
		},
		{
			"a visitor who is not signed in gets the same", permissions.RoleNone, "", false,
			[]string{pair(watch, keep)},
			[]*Entity{watch, keep},
			[]*Entity{ledger, trashed, hidden, elsewhere, plot},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := svc.GetMentionGraph(ctx, campaignID, tt.role, tt.userID, tt.seesSecrets)
			if err != nil {
				t.Fatalf("GetMentionGraph: %v", err)
			}
			got := map[string]bool{}
			for _, l := range g.Links {
				got[l.SourceEntityID+">"+l.TargetEntityID] = true
			}
			if len(got) != len(tt.wantLinks) {
				t.Errorf("got %d links, want %d: %v", len(got), len(tt.wantLinks), got)
			}
			for _, want := range tt.wantLinks {
				if !got[want] {
					t.Errorf("link %s is missing: %v", want, got)
				}
			}
			if len(g.Pages) != len(tt.wantPages) {
				t.Errorf("got %d pages, want %d", len(g.Pages), len(tt.wantPages))
			}
			for _, want := range tt.wantPages {
				p, ok := g.Pages[want.ID]
				if !ok {
					t.Errorf("%s is missing from the pages", want.Name)
					continue
				}
				if p.Name != want.Name || p.Slug != want.Slug || p.TypeName != "Place" || p.TypeIcon != "fa-map-pin" || p.TypeColor != "#336699" {
					t.Errorf("%s is described as %+v", want.Name, p)
				}
			}

			// Nothing about a page the viewer may not know of leaves the
			// service: not its id, not its name.
			body, err := json.Marshal(g)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			for _, e := range tt.never {
				if strings.Contains(string(body), e.ID) || strings.Contains(string(body), e.Name) {
					t.Errorf("SECURITY LEAK: %s reached this viewer: %s", e.Name, body)
				}
			}
		})
	}
}
