package app

import (
	"context"
	"io/fs"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// The Adventures tab runs against the real entities visibility rules and the
// real table. Each step is a way a player, or a page that is no longer an
// Adventure, could end up named on the tab; none may.
func TestDMScreenAdventures_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	ctx := context.Background()

	sub, err := fs.Sub(dmscreen.MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range database.RunPluginMigrations(h.db, []database.PluginSchema{{Slug: dmscreen.PluginSlug, MigrationsFS: sub}}) {
		if !res.Healthy {
			t.Fatalf("%s migrations did not apply: %v", res.Slug, res.Error)
		}
	}

	owner := h.newUser("owner")
	camp, err := h.campaigns.Create(ctx, owner, campaigns.CreateCampaignInput{Name: "Tab Test"})
	if err != nil {
		t.Fatal(err)
	}
	advTypes, err := h.entities.GetEntityTypesByPresetCategory(ctx, camp.ID, entities.PresetCategoryAdventures)
	if err != nil || len(advTypes) != 1 {
		t.Fatalf("Adventures category: %v, %d found", err, len(advTypes))
	}
	types, err := h.entities.GetEntityTypes(ctx, camp.ID)
	if err != nil {
		t.Fatal(err)
	}
	var plainType int
	for _, et := range types {
		if !et.DMOnly() {
			plainType = et.ID
			break
		}
	}

	lastwell, err := h.entities.Create(ctx, camp.ID, owner, entities.CreateEntityInput{
		Name: "Lastwell", EntityTypeID: advTypes[0].ID,
		FieldsData: map[string]any{"sessions": "Sessions 3 to 6", "status": "Running"},
	})
	if err != nil {
		t.Fatal(err)
	}
	npc, err := h.entities.Create(ctx, camp.ID, owner, entities.CreateEntityInput{Name: "Barkeep", EntityTypeID: plainType})
	if err != nil {
		t.Fatal(err)
	}

	board := dmscreen.NewBoardRepository(h.db)
	adapter := &dmAdventuresAdapter{entities: h.entities}
	svc := dmscreen.NewService(dmscreen.Sources{Board: board, Adventures: adapter})
	dm := dmscreen.Viewer{UserID: owner, Role: 3}
	scribe := dmscreen.Viewer{UserID: "scribe", Role: 2}
	player := dmscreen.Viewer{UserID: "player", Role: 1}

	count := func() int {
		n, err := board.Count(ctx, camp.ID)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	tabTitles := func(v dmscreen.Viewer) []string {
		view, err := svc.Build(ctx, camp.ID, v)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		var out []string
		for _, it := range view.Adventures.Items {
			if it.Kind == dmscreen.BoardKindPage {
				out = append(out, it.Title+"|"+it.Kicker+"|"+it.Subtitle)
			}
		}
		return out
	}

	if err := svc.PinPage(ctx, camp.ID, dm, npc.ID); err == nil {
		t.Fatal("a page outside Adventures was pinned")
	}
	if err := svc.PinPage(ctx, camp.ID, player, lastwell.ID); err == nil {
		t.Fatal("a player pinned an Adventures page")
	}
	if n := count(); n != 0 {
		t.Fatalf("refused pins left %d rows", n)
	}

	for i := 0; i < 2; i++ {
		if err := svc.PinPage(ctx, camp.ID, dm, lastwell.ID); err != nil {
			t.Fatalf("pin %d: %v", i, err)
		}
	}
	if n := count(); n != 1 {
		t.Fatalf("pinning twice left %d rows, want 1", n)
	}

	want := "Lastwell|Adventure|Sessions 3 to 6 · Running"
	for name, v := range map[string]dmscreen.Viewer{"owner": dm, "scribe": scribe} {
		got := tabTitles(v)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s sees %v, want [%s]", name, got, want)
		}
	}

	// A player is refused outright, and the source returns nothing for a
	// player even when asked for the pinned id directly.
	if _, err := svc.Build(ctx, camp.ID, player); err == nil {
		t.Error("Build answered a player")
	}
	if _, err := svc.PickPages(ctx, camp.ID, player, ""); err == nil {
		t.Error("the picker answered a player")
	}
	pages, err := adapter.Pages(ctx, camp.ID, []string{lastwell.ID}, player)
	if err != nil || len(pages) != 0 {
		t.Errorf("source gave a player %v (err %v)", pages, err)
	}
	found, hasCat, err := adapter.Search(ctx, camp.ID, "Last", player, 12)
	if err != nil || len(found) != 0 || !hasCat {
		t.Errorf("search gave a player %v (hasCategory %v, err %v)", found, hasCat, err)
	}

	pick, err := svc.PickPages(ctx, camp.ID, dm, "Last")
	if err != nil || !pick.HasCategory || len(pick.Pages) != 1 || !pick.Pages[0].Pinned {
		t.Errorf("owner's picker = %+v (err %v), want Lastwell marked pinned", pick, err)
	}

	// A page in the trash leaves the tab without losing its pin.
	mustGalleryExec(t, h.db, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, lastwell.ID)
	if got := tabTitles(dm); len(got) != 0 {
		t.Errorf("a trashed page is still on the tab: %v", got)
	}
	if n := count(); n != 1 {
		t.Errorf("trashing deleted the pin: %d rows", n)
	}
	mustGalleryExec(t, h.db, `UPDATE entities SET deleted_at = NULL WHERE id = ?`, lastwell.ID)
	if got := tabTitles(dm); len(got) != 1 {
		t.Errorf("a restored page did not come back: %v", got)
	}

	// Moved out of the Adventures category it can no longer be shown here.
	mustGalleryExec(t, h.db, `UPDATE entities SET entity_type_id = ? WHERE id = ?`, plainType, lastwell.ID)
	if got := tabTitles(dm); len(got) != 0 {
		t.Errorf("a page moved out of Adventures is still on the tab: %v", got)
	}
	mustGalleryExec(t, h.db, `UPDATE entities SET entity_type_id = ? WHERE id = ?`, advTypes[0].ID, lastwell.ID)

	// A note belongs to its campaign alone.
	if err := svc.AddNote(ctx, camp.ID, scribe, "Ask about the bell"); err != nil {
		t.Fatal(err)
	}
	items, err := board.List(ctx, "some-other-campaign")
	if err != nil || len(items) != 0 {
		t.Errorf("another campaign's board = %v (err %v)", items, err)
	}

	// Deleting the page for good removes its pin with it.
	mustGalleryExec(t, h.db, `DELETE FROM entities WHERE id = ?`, lastwell.ID)
	if n := count(); n != 1 {
		t.Errorf("rows after the page was deleted = %d, want only the note", n)
	}
}
