// backlinks_secrets_integration_test.go drives the real entity repository and
// service against a live MariaDB to prove that a linking page's GM secret
// never reaches a viewer below Scribe through the backlinks API, in any
// field of the response — not just the snippet extraction path exercised by
// the mock-based unit test in service_test.go.
package entities

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// TestBacklinks_SecretRedaction_Integration seeds a public linking page whose
// entry_html contains a GM secret alongside a mention of a target page, then
// calls the real GetBacklinksWithSnippets for an owner, a Scribe, a player,
// and an anonymous visitor. The owner and Scribe must see the secret text
// (proving the fixture is real); the player and anonymous visitor must never
// receive it in any field of the response, matching entities/handler.go's
// BacklinksFragment / GetEntry bar of MemberRole >= RoleScribe.
func TestBacklinks_SecretRedaction_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}

	db := openTestDB(t)

	ctx := context.Background()
	entityRepo := NewEntityRepository(db)
	typeRepo := NewEntityTypeRepository(db)
	svc := NewEntityService(entityRepo, typeRepo, nil)

	ownerID := testUUID(t)
	campaignID := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		ownerID, "backlinks-secret-"+ownerID+"@example.test", "Backlinks Secret Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Backlinks Secret Test", "backlinks-secret-"+campaignID[:8], ownerID)
	t.Cleanup(func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, ownerID)
	})

	res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`,
		campaignID, "backlinks-secret-npc", "NPC", "NPCs")
	if err != nil {
		t.Fatalf("seed entity type: %v", err)
	}
	entityTypeID64, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("entity type last insert id: %v", err)
	}
	entityTypeID := int(entityTypeID64)

	now := time.Now().UTC()

	// The target page that gets mentioned. Public (default visibility, not
	// private) so it's a page any viewer, including anonymous, may reach.
	target := &Entity{
		ID: testUUID(t), CampaignID: campaignID, EntityTypeID: entityTypeID,
		Name: "Thalrik", Slug: "thalrik-" + testUUID(t)[:8],
		FieldsData: map[string]any{}, CreatedBy: ownerID, CreatedAt: now, UpdatedAt: now,
	}
	if err := entityRepo.Create(ctx, target); err != nil {
		t.Fatalf("create target entity: %v", err)
	}

	// A public page (default visibility, not private) whose entry contains a
	// GM secret AND a mention of the target.
	const secretText = "the crown is hidden beneath the chapel floor"
	html := `<p>Rumor has it <span data-secret="true">` + secretText + `</span>. See ` +
		`<a data-mention-id="` + target.ID + `" href="/e/` + target.ID + `">@Thalrik</a> for context.</p>`
	linker := &Entity{
		ID: testUUID(t), CampaignID: campaignID, EntityTypeID: entityTypeID,
		Name: "Baron Vex's Journal", Slug: "baron-vex-journal-" + testUUID(t)[:8],
		EntryHTML: &html, FieldsData: map[string]any{}, CreatedBy: ownerID, CreatedAt: now, UpdatedAt: now,
	}
	if err := entityRepo.Create(ctx, linker); err != nil {
		t.Fatalf("create linking entity: %v", err)
	}

	tests := []struct {
		name       string
		role       int
		userID     string
		canSeeGM   bool
		wantSecret bool
	}{
		{"owner sees the secret", permissions.RoleOwner, ownerID, true, true},
		{"scribe sees the secret", permissions.RoleScribe, testUUID(t), true, true},
		{"player never sees the secret", permissions.RolePlayer, testUUID(t), false, false},
		{"anonymous visitor never sees the secret", permissions.RoleNone, "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := svc.GetBacklinksWithSnippets(ctx, campaignID, target.ID, tt.role, tt.userID, tt.canSeeGM)
			if err != nil {
				t.Fatalf("GetBacklinksWithSnippets: %v", err)
			}
			if len(entries) != 1 {
				t.Fatalf("expected the public linking page to be returned, got %d entries: %+v", len(entries), entries)
			}
			if entries[0].Entity.ID != linker.ID {
				t.Fatalf("expected linking entity %s, got %s", linker.ID, entries[0].Entity.ID)
			}

			data, err := json.Marshal(entries)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			body := string(data)

			gotSecret := strings.Contains(body, secretText)
			if gotSecret != tt.wantSecret {
				t.Errorf("response contains secret text = %v, want %v\nbody: %s", gotSecret, tt.wantSecret, body)
			}
			if !tt.wantSecret && gotSecret {
				t.Fatalf("SECURITY LEAK: %s received GM secret text via backlinks: %s", tt.name, body)
			}

			// No field of the response may carry the linking page's raw
			// content, for any viewer — the API contract is id/name/type +
			// a snippet, never entry/entry_html/fields_data.
			for _, forbidden := range []string{`"entry"`, `"entry_html"`, `"fields_data"`, `"player_notes"`} {
				if strings.Contains(body, forbidden) {
					t.Errorf("backlinks response must not carry raw entity field %s: %s", forbidden, body)
				}
			}
		})
	}
}
