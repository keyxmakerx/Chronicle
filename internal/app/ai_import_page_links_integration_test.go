package app

// ai_import_page_links_integration_test.go proves, against a real database,
// that two pages created by one AI import paste that link each other with
// @[Name] both end up with the editor's mention anchor in entry_html, and
// that the backlinks read finds them. A fake entity service could not show
// the second write (the relink once every page exists) or the backlink scan.
//
// Skipped under -short. Run with
// `CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/app/ -run AIImportPageLinks -v`.

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
)

func TestAIImportPageLinks_MutualLinksStoreMentionAnchors(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openGalleryTestDB(t)
	defer db.Close()
	ctx := context.Background()

	fx := newGalleryFixture(t, db)
	defer fx.cleanup()
	fx.entityType("import-npc", "NPC", "", "")
	svc := fx.entityService()

	page := func(name, body string) importer.ParsedPage {
		return importer.ParsedPage{
			Name: name, Body: body, Status: importer.StatusNew,
			FrontMatter: importer.FrontMatter{Name: name, Type: "import-npc"},
		}
	}
	dec := func(name string) importer.RowDecision {
		return importer.RowDecision{Include: true, Name: name, CategorySpec: "import-npc", ConflictMode: "rename"}
	}
	res, err := importer.NewCommitter(svc).Commit(ctx, fx.campaignID, importer.CommitInput{
		OwnerID: fx.ownerID,
		Pages: []importer.ParsedPage{
			page("Maro Vale", "Rival of @[Lyra Dawn] and friend of @[Nobody Here]."),
			page("Lyra Dawn", "Rival of @[Maro Vale|the sailor]."),
		},
		Decisions: []importer.RowDecision{dec("Maro Vale"), dec("Lyra Dawn")},
		Viewer:    importer.Viewer{Role: permissions.RoleOwner, UserID: fx.ownerID},
	})
	if err != nil || res.Created != 2 {
		t.Fatalf("commit: created=%d err=%v rows=%+v", res.Created, err, res.Rows)
	}
	maroID, lyraID := res.Rows[0].EntityID, res.Rows[1].EntityID

	html := func(id string) string {
		var h string
		if err := db.QueryRow(`SELECT entry_html FROM entities WHERE id = ?`, id).Scan(&h); err != nil {
			t.Fatalf("read entry_html: %v", err)
		}
		return h
	}
	maro, lyra := html(maroID), html(lyraID)
	if !strings.Contains(maro, `data-mention-id="`+lyraID+`"`) || !strings.Contains(maro, `data-entity-preview="/campaigns/`+fx.campaignID+`/entities/`+lyraID+`/preview"`) {
		t.Errorf("Maro's page lacks the mention of Lyra: %s", maro)
	}
	if !strings.Contains(lyra, `data-mention-id="`+maroID+`"`) || !strings.Contains(lyra, ">the sailor</a>") {
		t.Errorf("Lyra's page lacks the mention of Maro: %s", lyra)
	}
	if strings.Contains(maro, "@[") || !strings.Contains(maro, "Nobody Here") {
		t.Errorf("unknown name should stay as plain words: %s", maro)
	}

	back, err := svc.GetBacklinks(ctx, fx.campaignID, lyraID, permissions.RoleOwner, fx.ownerID)
	if err != nil {
		t.Fatalf("GetBacklinks: %v", err)
	}
	if len(back) != 1 || back[0].ID != maroID {
		t.Errorf("Lyra's backlinks = %+v, want Maro's page", back)
	}
}
