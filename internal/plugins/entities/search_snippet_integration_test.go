package entities

// search_snippet_integration_test.go drives the real repository and service
// against MariaDB for the Search page's two groups. What it proves: for every
// kind of reader, a snippet never quotes text that reader may not see, a page
// hidden from them is neither listed nor quoted, and a word that exists only
// inside a secret sentence finds nothing at all (search matches the reader-safe
// search_text, so no role, the owner included, is shown a page by a secret).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestSearchGroups_NeverQuoteWhatTheReaderCannotSee_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewEntityRepository(db)
	types := NewEntityTypeRepository(db)
	svc := NewEntityService(repo, types, nil)

	ownerID, grantedID, strangerID := testUUID(t), testUUID(t), testUUID(t)
	campaignID := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		ownerID, "search-snip-"+ownerID+"@example.test", "Search Snippet Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Search Snippet Test", "search-snip-"+campaignID[:8], ownerID)
	t.Cleanup(func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, ownerID)
	})
	res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`,
		campaignID, "search-snip-npc", "NPC", "NPCs")
	if err != nil {
		t.Fatalf("seed entity type: %v", err)
	}
	typeID64, _ := res.LastInsertId()
	typeID := int(typeID64)

	now := time.Now().UTC()
	mk := func(name, entry string, private bool) *Entity {
		html := entry
		e := &Entity{
			ID: testUUID(t), CampaignID: campaignID, EntityTypeID: typeID, Name: name,
			Slug:      strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + testUUID(t)[:6],
			EntryHTML: &html, FieldsData: map[string]any{}, IsPrivate: private,
			CreatedBy: ownerID, CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
		return e
	}

	const secret = "the Duskharrow cult meets beneath the quay"
	named := mk("Tidewrights", `<p>A guild of harbour engineers.</p>`, false)
	ledger := mk("Harbour ledger", `<p>The Tidewrights owe three seasons of berth fees. <span data-secret="true">`+secret+`</span></p>`, false)
	vault := mk("Hidden vault", `<p>The Tidewrights keep their real ledger here.</p>`, true)
	granted := mk("Granted note", `<p>Only one player may read how the Tidewrights pay.</p>`, false)
	mustExec(t, db, `UPDATE entities SET visibility = 'custom' WHERE id = ?`, granted.ID)
	mustExec(t, db, `INSERT INTO entity_permissions (entity_id, subject_type, subject_id, permission) VALUES (?, 'user', ?, 'view')`, granted.ID, grantedID)

	// A page found by another name, and one by its descriptor, belong to the
	// named group even though their text mentions the words too; a page with
	// no descriptor must not drop out of the mention group.
	aliased := mk("Salt wardens", `<p>The Tidewrights hire them to watch the lower gates.</p>`, false)
	mustExec(t, db, `INSERT INTO entity_aliases (entity_id, alias) VALUES (?, ?)`, aliased.ID, "The Tidewrights' wardens")
	labelled := mk("Quay office", `<p>Tidewrights clerks sit here.</p>`, false)
	mustExec(t, db, `UPDATE entities SET type_label = ? WHERE id = ?`, "Tidewrights hall", labelled.ID)

	type reader struct {
		name   string
		role   int
		userID string
		// pages this reader may see among the four.
		sees map[string]bool
	}
	readers := []reader{
		{"owner", permissions.RoleOwner, ownerID, map[string]bool{named.ID: true, aliased.ID: true, labelled.ID: true, ledger.ID: true, vault.ID: true, granted.ID: true}},
		// A co-GM is a player-level member with a DM grant, whose visibility
		// role is the owner's (CampaignContext.VisibilityRole).
		{"co-GM", permissions.RoleOwner, testUUID(t), map[string]bool{named.ID: true, aliased.ID: true, labelled.ID: true, ledger.ID: true, vault.ID: true, granted.ID: true}},
		{"scribe", permissions.RoleScribe, strangerID, map[string]bool{named.ID: true, aliased.ID: true, labelled.ID: true, ledger.ID: true}},
		{"player", permissions.RolePlayer, strangerID, map[string]bool{named.ID: true, aliased.ID: true, labelled.ID: true, ledger.ID: true}},
		{"player with a grant", permissions.RolePlayer, grantedID, map[string]bool{named.ID: true, aliased.ID: true, labelled.ID: true, ledger.ID: true, granted.ID: true}},
		{"guest", permissions.RoleNone, "", map[string]bool{named.ID: true, aliased.ID: true, labelled.ID: true, ledger.ID: true}},
	}

	search := func(t *testing.T, r reader, query, scope string) (ids []string, body string) {
		t.Helper()
		opts := DefaultListOptions()
		opts.PerPage = 20
		opts.SearchScope = scope
		found, _, err := svc.Search(ctx, campaignID, query, 0, r.role, r.userID, opts)
		if err != nil {
			t.Fatalf("search %q/%s: %v", query, scope, err)
		}
		var idList []string
		for _, e := range found {
			idList = append(idList, e.ID)
		}
		snips := svc.SearchSnippets(ctx, campaignID, query, idList, r.role, r.userID)
		out := map[string]any{"ids": idList, "snippets": snips}
		b, _ := json.Marshal(out)
		return idList, string(b)
	}

	for _, r := range readers {
		t.Run(r.name, func(t *testing.T) {
			// The mention group lists exactly the other pages this reader may
			// see, and never the page named for the words.
			ids, body := search(t, r, "tidewrights", SearchScopeMention)
			inNamed := map[string]bool{named.ID: true, aliased.ID: true, labelled.ID: true}
			want := 0
			for id := range r.sees {
				if !inNamed[id] {
					want++
				}
			}
			if len(ids) != want {
				t.Errorf("mention group has %d pages, want %d: %s", len(ids), want, body)
			}
			for _, id := range ids {
				if !r.sees[id] {
					t.Errorf("SECURITY: %s was listed a page they may not see (%s)", r.name, id)
				}
				if inNamed[id] {
					t.Errorf("a page found by its name, another name or descriptor is in the named group only (%s)", id)
				}
			}
			if strings.Contains(body, "Duskharrow") || strings.Contains(body, secret) {
				t.Errorf("SECURITY: a snippet for %s quotes secret text: %s", r.name, body)
			}
			if r.sees[ledger.ID] && !strings.Contains(body, "berth fees") {
				t.Errorf("the ledger's visible sentence should be quoted: %s", body)
			}

			// The named group is the page whose name has the words.
			nids, _ := search(t, r, "tidewrights", SearchScopeNamed)
			if len(nids) != 3 {
				t.Errorf("named group = %v, want the page named for it, the one with another name and the one with the descriptor", nids)
			}
			for _, id := range nids {
				if !inNamed[id] {
					t.Errorf("named group holds a page that only mentions the words: %s", id)
				}
			}

			// A word that lives only inside a secret sentence finds nothing, in
			// either group, for every reader including the owner.
			for _, scope := range []string{SearchScopeNamed, SearchScopeMention, ""} {
				got, b := search(t, r, "duskharrow", scope)
				if len(got) != 0 {
					t.Errorf("a secret-only word found pages (scope %q) for %s: %s", scope, r.name, b)
				}
			}
		})
	}
}
