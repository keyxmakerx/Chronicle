package entities

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// TestPagesLinkingNote_SecretsStayWithThoseWhoSeeThem: a page whose only
// link to a note sits inside an inline GM secret is not reported to a
// viewer who cannot read secrets, or the list would tell them the secret
// mentions that note.
func TestPagesLinkingNote_SecretsStayWithThoseWhoSeeThem(t *testing.T) {
	const note = "0a1b2c3d-0000-4000-8000-000000000001"
	link := `<a data-note-id="` + note + `" href="/x">Journal note</a>`
	html := func(s string) *string { return &s }
	pages := []Entity{
		{ID: "open", Name: "Open", EntryHTML: html(`<p>See ` + link + `.</p>`)},
		{ID: "secret-only", Name: "Secret", EntryHTML: html(`<p>Hm. <span data-secret="true">GM: ` + link + `</span></p>`)},
		{ID: "both", Name: "Both", EntryHTML: html(`<p>` + link + ` <span data-secret="true">and ` + link + `</span></p>`)},
	}
	var gotRole int
	repo := &mockEntityRepo{pagesLinkingNoteFn: func(_ context.Context, campaignID, noteID string, role int, _ string) ([]Entity, error) {
		gotRole = role
		if campaignID != "c1" || noteID != note {
			t.Fatalf("scoped to %q/%q", campaignID, noteID)
		}
		out := make([]Entity, len(pages))
		copy(out, pages)
		return out, nil
	}}
	svc := newTestService(repo, &mockEntityTypeRepo{})

	ids := func(es []Entity) string {
		var s []string
		for _, e := range es {
			s = append(s, e.ID)
		}
		return strings.Join(s, ",")
	}
	got, err := svc.PagesLinkingNote(context.Background(), "c1", note, int(permissions.RolePlayer), "u1", false)
	if err != nil {
		t.Fatal(err)
	}
	if ids(got) != "open,both" {
		t.Errorf("a player gets only pages that link outside secrets, got %s", ids(got))
	}
	if gotRole != int(permissions.RolePlayer) {
		t.Errorf("the viewer's role reaches the query, got %d", gotRole)
	}
	got, _ = svc.PagesLinkingNote(context.Background(), "c1", note, int(permissions.RoleOwner), "u1", true)
	if ids(got) != "open,secret-only,both" {
		t.Errorf("a viewer who reads secrets gets every page, got %s", ids(got))
	}
}

// TestPagesLinkingNoteWhere_ScopesTheCampaign pins the campaign term and the
// LIKE escaping of the pure WHERE builder.
func TestPagesLinkingNoteWhere_ScopesTheCampaign(t *testing.T) {
	where, args := pagesLinkingNoteWhere("c1", `a%b_c\d`, int(permissions.RolePlayer), "u1")
	if !strings.Contains(where, "e.campaign_id = ?") || args[0] != "c1" {
		t.Fatalf("the query must be campaign-scoped: %s %v", where, args)
	}
	if args[1] != `%data-note-id="a\%b\_c\\d"%` {
		t.Errorf("the note id is matched literally, got %v", args[1])
	}
	if !strings.Contains(where, "e.visibility") {
		t.Error("a player's query carries the visibility filter")
	}
}

// TestFindPagesLinkingNote_Integration drives the real query: only pages of
// the campaign that link the note and that the viewer may see.
func TestFindPagesLinkingNote_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := newAncestorScratchDB(t)
	ctx := context.Background()
	repo := NewEntityRepository(db)

	ownerID := ancestorDBID(t)
	mustAncestorExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		ownerID, "note-links-"+ownerID+"@example.test", "Note Links Owner", "x")
	campaign := func() (string, int) {
		id := ancestorDBID(t)
		mustAncestorExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			id, "Note Links", "note-links-"+id[:8], ownerID)
		res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`,
			id, "place", "Place", "Places")
		if err != nil {
			t.Fatalf("seed entity type: %v", err)
		}
		typeID, _ := res.LastInsertId()
		return id, int(typeID)
	}
	c1, t1 := campaign()
	c2, t2 := campaign()

	note := ancestorDBID(t)
	link := `<p>See <a data-note-id="` + note + `" href="/x">Journal note</a>.</p>`
	other := `<p>See <a data-note-id="` + ancestorDBID(t) + `" href="/x">Journal note</a>.</p>`
	now := time.Now().UTC()
	page := func(campaignID string, typeID int, name, entryHTML string, private bool) string {
		e := &Entity{
			ID: ancestorDBID(t), CampaignID: campaignID, EntityTypeID: typeID,
			Name: name, Slug: "p-" + ancestorDBID(t)[:8], IsPrivate: private,
			FieldsData: map[string]any{}, CreatedBy: ownerID, CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		mustAncestorExec(t, db, `UPDATE entities SET entry_html = ? WHERE id = ?`, entryHTML, e.ID)
		return e.ID
	}
	open := page(c1, t1, "Open page", link, false)
	hidden := page(c1, t1, "Hidden page", link, true)
	page(c1, t1, "Links another note", other, false)
	page(c2, t2, "Other campaign", link, false)
	t.Cleanup(func() {
		mustAncestorExec(t, db, `DELETE FROM entities WHERE campaign_id IN (?, ?)`, c1, c2)
		mustAncestorExec(t, db, `DELETE FROM campaigns WHERE id IN (?, ?)`, c1, c2)
		mustAncestorExec(t, db, `DELETE FROM users WHERE id = ?`, ownerID)
	})

	names := func(role int) map[string]bool {
		es, err := repo.FindPagesLinkingNote(ctx, c1, note, role, "")
		if err != nil {
			t.Fatalf("FindPagesLinkingNote: %v", err)
		}
		out := map[string]bool{}
		for _, e := range es {
			out[e.ID] = true
		}
		return out
	}
	if got := names(int(permissions.RolePlayer)); len(got) != 1 || !got[open] {
		t.Errorf("a player sees only the open page of this campaign that links the note, got %v", got)
	}
	if got := names(int(permissions.RoleOwner)); len(got) != 2 || !got[open] || !got[hidden] {
		t.Errorf("the owner also sees the hidden page, and still nothing from the other campaign, got %v", got)
	}
}
