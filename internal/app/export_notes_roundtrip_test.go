// export_notes_roundtrip_test.go proves campaign export/import carries shared
// notes. Before sweep R4 stage 15 neither adapter existed and neither setter
// was called, so `Notes` was always empty in the envelope and always empty on
// the way back in: the backup silently dropped every shared note in the
// campaign. The round trip below fails (zero notes exported, zero recreated)
// with either adapter removed or either wiring call dropped.
package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// fakeNoteService is an in-memory notes.NoteService. It embeds the interface
// so only the methods the export/import adapters actually call need bodies;
// any other call would panic loudly rather than silently no-op.
type fakeNoteService struct {
	notes.NoteService

	shared []notes.Note // What ListSharedByCampaign returns.

	created []*notes.Note     // What ImportNotes created, in order.
	byID    map[string]*notes.Note
	nextID  int
}

func newFakeNoteService(shared ...notes.Note) *fakeNoteService {
	return &fakeNoteService{shared: shared, byID: map[string]*notes.Note{}}
}

func (f *fakeNoteService) ListSharedByCampaign(_ context.Context, _ string) ([]notes.Note, error) {
	return f.shared, nil
}

func (f *fakeNoteService) Create(_ context.Context, campaignID, userID string, req notes.CreateNoteRequest) (*notes.Note, error) {
	f.nextID++
	n := notes.Note{
		ID:         string(rune('a'+f.nextID-1)) + "-new",
		CampaignID: campaignID,
		UserID:     userID,
		EntityID:   req.EntityID,
		IsFolder:   req.IsFolder,
		Title:      req.Title,
		Content:    req.Content,
		Color:      req.Color,
		IsShared:   req.IsShared,
	}
	stored := &n
	f.created = append(f.created, stored)
	f.byID[n.ID] = stored
	return stored, nil
}

func (f *fakeNoteService) Update(_ context.Context, id, _ string, req notes.UpdateNoteRequest) (*notes.Note, error) {
	n, ok := f.byID[id]
	if !ok {
		t := notes.Note{}
		return &t, nil
	}
	if req.Entry != nil {
		n.Entry = req.Entry
	}
	if req.EntryHTML != nil {
		n.EntryHTML = req.EntryHTML
	}
	if req.Pinned != nil {
		n.Pinned = *req.Pinned
	}
	if req.ParentID != nil {
		n.ParentID = req.ParentID
	}
	return n, nil
}

// stubCampaignSvc is the minimum campaigns.CampaignService the export/import
// service touches: GetByID for the export envelope, Create for the import.
type stubCampaignSvc struct {
	campaigns.CampaignService
	campaign campaigns.Campaign
}

func (s *stubCampaignSvc) GetByID(context.Context, string) (*campaigns.Campaign, error) {
	c := s.campaign
	return &c, nil
}

func (s *stubCampaignSvc) Create(_ context.Context, _ string, in campaigns.CreateCampaignInput) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: "imported-campaign", Name: in.Name}, nil
}

func strptr(s string) *string { return &s }

// TestCampaignExportImport_SharedNotesRoundTrip is the regression: a campaign
// with shared notes must export them and import them back.
func TestCampaignExportImport_SharedNotesRoundTrip(t *testing.T) {
	entry := `{"type":"doc"}`
	src := newFakeNoteService(
		notes.Note{
			ID: "folder-1", CampaignID: "c1", Title: "Session Recaps",
			IsFolder: true, IsShared: true, Color: "#374151",
		},
		notes.Note{
			ID: "note-1", CampaignID: "c1", Title: "Party Loot",
			ParentID: strptr("folder-1"), IsShared: true, Pinned: true,
			Color: "#ff0000", Entry: &entry, EntryHTML: strptr("<p>gold</p>"),
			Content: []notes.Block{{
				Type:  "checklist",
				Items: []notes.ChecklistItem{{Text: "sell the gems", Checked: true}},
			}},
		},
		notes.Note{
			ID: "note-2", CampaignID: "c1", Title: "About the Duke",
			EntityID: strptr("entity-77"), IsShared: true, Color: "#00ff00",
		},
	)

	exportSvc := campaigns.NewExportImportService(&stubCampaignSvc{
		campaign: campaigns.Campaign{ID: "c1", Name: "Test Campaign"},
	})
	exportSvc.SetNoteExporter(&noteExportAdapter{svc: src})

	env, err := exportSvc.Export(context.Background(), "c1")
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if len(env.Notes) != 3 {
		t.Fatalf("exported %d notes, want 3 — shared notes are missing from the export envelope", len(env.Notes))
	}

	// The envelope must survive a JSON round trip: this is what lands on disk.
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var reloaded campaigns.CampaignExport
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if len(reloaded.Notes) != 3 {
		t.Fatalf("reloaded %d notes, want 3", len(reloaded.Notes))
	}

	byTitle := map[string]campaigns.ExportNote{}
	for _, n := range reloaded.Notes {
		byTitle[n.Title] = n
	}
	loot, ok := byTitle["Party Loot"]
	if !ok {
		t.Fatal("Party Loot note missing from export")
	}
	if !loot.Pinned {
		t.Error("Party Loot lost its pinned flag")
	}
	if loot.Entry == nil || *loot.Entry != entry {
		t.Error("Party Loot lost its rich-text entry")
	}
	if len(loot.Content) == 0 || !json.Valid(loot.Content) {
		t.Error("Party Loot lost its checklist blocks")
	}
	if loot.ParentIndex == nil {
		t.Fatal("Party Loot lost its folder membership")
	}
	if got := reloaded.Notes[*loot.ParentIndex].Title; got != "Session Recaps" {
		t.Errorf("Party Loot parent index points at %q, want Session Recaps", got)
	}
	if !byTitle["Session Recaps"].IsFolder {
		t.Error("Session Recaps lost its folder flag")
	}

	// --- Import side ---
	dst := newFakeNoteService()
	importSvc := campaigns.NewExportImportService(&stubCampaignSvc{})
	importSvc.SetNoteImporter(&noteImportAdapter{svc: dst})

	if _, err := importSvc.Import(context.Background(), "user-1", &reloaded); err != nil {
		t.Fatalf("import: %v", err)
	}

	if len(dst.created) != 3 {
		t.Fatalf("imported %d notes, want 3 — the importer dropped shared notes", len(dst.created))
	}
	var imported *notes.Note
	for _, n := range dst.created {
		if n.Title == "Party Loot" {
			imported = n
		}
	}
	if imported == nil {
		t.Fatal("Party Loot was not recreated on import")
	}
	if !imported.IsShared {
		t.Error("imported note is no longer shared; it would be invisible to the party")
	}
	if !imported.Pinned {
		t.Error("imported note lost its pinned flag")
	}
	if imported.Entry == nil || *imported.Entry != entry {
		t.Error("imported note lost its rich-text entry")
	}
	if len(imported.Content) != 1 || imported.Content[0].Type != "checklist" {
		t.Errorf("imported note lost its checklist blocks: %+v", imported.Content)
	}
	if imported.ParentID == nil {
		t.Error("imported note was not re-filed into its folder")
	}
}

// TestNoteExportAdapter_SkipsUnmappableParent confirms a shared note filed
// under a folder that is NOT itself shared exports at top level rather than
// carrying a dangling parent index.
func TestNoteExportAdapter_SkipsUnmappableParent(t *testing.T) {
	src := newFakeNoteService(notes.Note{
		ID: "note-1", Title: "Orphan", IsShared: true,
		ParentID: strptr("private-folder"),
	})
	a := &noteExportAdapter{svc: src}
	out, err := a.ExportNotes(context.Background(), "c1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("export notes: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d notes, want 1", len(out))
	}
	if out[0].ParentIndex != nil {
		t.Errorf("ParentIndex = %v, want nil for an unshared parent", *out[0].ParentIndex)
	}
}
