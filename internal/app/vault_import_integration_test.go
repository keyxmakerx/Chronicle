// vault_import_integration_test.go imports a small Obsidian-style vault through
// the production adapters (entities, media, page files) against MariaDB and
// checks what the owner is promised: the folder tree, working page links,
// pictures stored once, files attached, and every page GM only.
//
// Skipped under -short. Run with `make test-int-local`.
package app

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/vault_import"
)

func vaultFixtureZip(t *testing.T, png []byte) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	add("Vault/Home.md", []byte("---\nname: Welcome\nvisibility: public\n---\nMeet [[Bob]] and [[People/Alice|Al]].\n\n[[Nobody]] is missing.\n\n![[map.png|300]]\n\nRules: ![[rules.pdf]]\n"))
	add("Vault/People/Bob.md", []byte("Back to [home](../Home.md).\n\n![[map.png]]\n"))
	add("Vault/People/Alice.md", []byte("Alice knows [[Bob#Past]].\n"))
	add("Vault/img/map.png", png)
	add("Vault/docs/rules.pdf", []byte("%PDF-1.4 fixture"))
	add("Vault/.obsidian/app.json", []byte("{}"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "Vault.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVaultImport_DBImportsATreeGMOnly_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	ctx := context.Background()
	owner := h.newUser("vault-owner")
	camp, err := h.campaigns.Create(ctx, owner, campaigns.CreateCampaignInput{Name: "Vault Import"})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	pageFiles := media.NewPageFileService(media.NewPageFileRepository(h.db), h.media, &pageAccessAdapter{svc: h.entities})
	svc := vault_import.NewService(vault_import.Deps{
		Pages:     vaultPagesAdapter{svc: h.entities},
		Pictures:  vaultPicturesAdapter{svc: h.media},
		Files:     vaultFilesAdapter{svc: pageFiles},
		ToJSON:    vaultEditorJSON,
		TempDir:   t.TempDir(),
		Lifecycle: ctx,
	})

	run := func() vault_import.JobView {
		p, err := svc.Preview(ctx, camp.ID, owner, vaultFixtureZip(t, roundTripPNG(t, 33)), "Vault.zip")
		if err != nil {
			t.Fatalf("preview: %v", err)
		}
		jv, err := svc.Start(camp.ID, owner, p.UploadID)
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			v, _ := svc.Job(camp.ID, jv.ID)
			if v.State != vault_import.StateRunning {
				return v
			}
			if time.Now().After(deadline) {
				t.Fatal("import did not finish")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	v := run()
	if v.State != vault_import.StateDone {
		t.Fatalf("state = %s: %s", v.State, v.Error)
	}
	if v.Summary.TextsFailed != 0 || v.Summary.PagesFailed != 0 {
		t.Fatalf("problems: %+v", v.Summary.Problems)
	}

	byName := func(name string) *entities.Entity {
		t.Helper()
		var id string
		if err := h.db.QueryRow(`SELECT id FROM entities WHERE campaign_id = ? AND name = ? AND deleted_at IS NULL ORDER BY created_at LIMIT 1`, camp.ID, name).Scan(&id); err != nil {
			t.Fatalf("page %q: %v", name, err)
		}
		e, err := h.entities.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	root, welcome, people, bob, alice := byName("Imported"), byName("Welcome"), byName("People"), byName("Bob"), byName("Alice")

	// Tree.
	parent := func(e *entities.Entity) string {
		if e.ParentID == nil {
			return ""
		}
		return *e.ParentID
	}
	if parent(root) != "" || parent(welcome) != root.ID || parent(people) != root.ID || parent(bob) != people.ID || parent(alice) != people.ID {
		t.Errorf("tree wrong: root<-%q welcome<-%q people<-%q bob<-%q alice<-%q", parent(root), parent(welcome), parent(people), parent(bob), parent(alice))
	}

	// Visibility: every page is GM only, whatever the note's front matter said,
	// and a player cannot see any of them.
	ids := []string{root.ID, welcome.ID, people.ID, bob.ID, alice.ID}
	for _, e := range []*entities.Entity{root, welcome, people, bob, alice} {
		if !e.IsPrivate {
			t.Errorf("page %q is not GM only", e.Name)
		}
	}
	seen, err := h.entities.FilterViewableEntityIDs(ctx, camp.ID, ids, int(campaigns.RolePlayer), "some-player")
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Errorf("a player can see imported pages: %v", seen)
	}

	// Links point at the real page ids, in the editor's mention markup.
	html := func(e *entities.Entity) string {
		if e.EntryHTML == nil {
			return ""
		}
		return *e.EntryHTML
	}
	if !strings.Contains(html(welcome), `data-mention-id="`+bob.ID+`"`) || !strings.Contains(html(welcome), `data-mention-id="`+alice.ID+`"`) {
		t.Errorf("Welcome lacks links to Bob and Alice: %s", html(welcome))
	}
	if !strings.Contains(html(bob), `data-mention-id="`+welcome.ID+`"`) {
		t.Errorf("Bob's Markdown link to Home was not turned into a page link: %s", html(bob))
	}
	if !strings.Contains(html(alice), `data-mention-id="`+bob.ID+`"`) {
		t.Errorf("Alice's [[Bob#Past]] was not turned into a page link: %s", html(alice))
	}
	if !strings.Contains(html(welcome), "Nobody") || strings.Contains(html(welcome), "[[Nobody") {
		t.Errorf("an unresolved link should stay as plain words: %s", html(welcome))
	}
	if welcome.Entry == nil || !strings.Contains(*welcome.Entry, `"mention"`) && !strings.Contains(*welcome.Entry, bob.ID) {
		t.Errorf("Welcome has no editor document with the link: %v", welcome.Entry)
	}

	// Pictures: stored once however often shown, placed as editor pictures.
	var pics int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM media_files WHERE campaign_id = ? AND original_name = 'map.png'`, camp.ID).Scan(&pics); err != nil {
		t.Fatal(err)
	}
	if pics != 1 {
		t.Errorf("map.png stored %d times, want once", pics)
	}
	for _, e := range []*entities.Entity{welcome, bob} {
		if !strings.Contains(html(e), `<figure class="ce-img`) || !strings.Contains(html(e), `src="/media/`) {
			t.Errorf("%s has no picture: %s", e.Name, html(e))
		}
		if e.Entry == nil || !strings.Contains(*e.Entry, "chronicleImage") {
			t.Errorf("%s's editor document lost the picture: %v", e.Name, e.Entry)
		}
	}

	// The PDF is in Welcome's Files section, GM only.
	listing, err := pageFiles.List(ctx, camp.ID, welcome.ID, media.PageFileViewer{UserID: owner, Role: int(campaigns.RoleOwner)})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Files) != 1 || !listing.Files[0].GMOnly {
		t.Errorf("Welcome's files = %+v, want rules.pdf GM only", listing.Files)
	}

	// A second import of the same vault makes a second folder.
	v2 := run()
	if v2.State != vault_import.StateDone {
		t.Fatalf("second import: %s %s", v2.State, v2.Error)
	}
	var roots int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM entities WHERE campaign_id = ? AND name LIKE 'Imported%' AND parent_id IS NULL AND deleted_at IS NULL`, camp.ID).Scan(&roots); err != nil {
		t.Fatal(err)
	}
	if roots != 2 {
		t.Errorf("Imported folders = %d, want 2", roots)
	}
}
