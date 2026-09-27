package notes

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// journalRepo builds a memRepo whose list and lookup methods apply the real
// CanView, so these tests see exactly what a viewer would.
func journalRepo(notes ...*Note) (*mockNoteRepo, map[string]*Note) {
	repo, store, _ := memRepo(notes...)
	visible := func(campaignID string, v permissions.Viewer) []Note {
		var out []Note
		for _, n := range store {
			cp := *n
			cp.derive()
			if cp.CanView(v, campaignID) {
				out = append(out, cp)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	}
	repo.listVisibleFn = func(_ context.Context, campaignID string, v permissions.Viewer, scope ListScope) ([]Note, error) {
		var out []Note
		for _, n := range visible(campaignID, v) {
			switch scope.Kind {
			case "campaign":
				if n.EntityID != nil {
					continue
				}
			case "jots":
				if n.EntityID == nil {
					continue
				}
			}
			out = append(out, n)
		}
		return out, nil
	}
	repo.findByIDsFn = func(_ context.Context, ids []string) ([]Note, error) {
		var out []Note
		for _, id := range ids {
			if n, ok := store[id]; ok {
				cp := *n
				cp.derive()
				out = append(out, cp)
			}
		}
		return out, nil
	}
	repo.listVisibleLinkingFn = func(_ context.Context, campaignID string, v permissions.Viewer, kind, target string) ([]Note, error) {
		attrName := linkAttr[kind]
		var out []Note
		for _, n := range visible(campaignID, v) {
			if strings.Contains(bodyOf(&n), attrName+`="`+target+`"`) {
				out = append(out, n)
			}
		}
		return out, nil
	}
	return repo, store
}

func htmlp(s string) *string { return &s }

func noteLink(id string) string { return `<a data-note-id="` + id + `">Journal note</a>` }

// The Ashkeep fixture: a GM, a player (ana) and three notes that link.
func ashkeep() []*Note {
	page := "44444444-4444-4444-4444-444444444444"
	return []*Note{
		{ID: idA, CampaignID: "c1", UserID: "u-gm", Title: "Thalrik Mourngrave", SharedWithGM: true,
			EntryHTML: htmlp(`<p>Lich under <a data-mention-id="` + page + `">@Ashkeep Ruins</a>.</p>`)},
		{ID: idB, CampaignID: "c1", UserID: "u-gm", Title: "Session 12", IsShared: true,
			EntryHTML: htmlp(`<p>No sign of ` + noteLink(idA) + ` yet.</p>`)},
		{ID: idP, CampaignID: "c1", UserID: "u-ana", Title: "Ana's doubts",
			EntryHTML: htmlp(`<p>After ` + noteLink(idB) + ` I wonder.</p>`)},
		{ID: "55555555-5555-5555-5555-555555555555", CampaignID: "c1", UserID: "u-gm", Title: "Combat reminder",
			EntityID: &page, IsShared: true, EntryHTML: htmlp(`<p>` + noteLink(idA) + `</p>`)},
		{ID: "66666666-6666-6666-6666-666666666666", CampaignID: "c1", UserID: "u-gm", Title: "Locations", IsFolder: true, IsShared: true},
	}
}

func rowByID(idx *JournalIndex, id string) *IndexRow {
	for i := range idx.Notes {
		if idx.Notes[i].ID == id {
			return &idx.Notes[i]
		}
	}
	return nil
}

func TestJournalIndex_WhatAPlayerSees(t *testing.T) {
	repo, _ := journalRepo(ashkeep()...)
	svc := NewNoteService(repo)
	idx, err := svc.JournalIndex(context.Background(), "c1", player("u-ana"))
	if err != nil {
		t.Fatalf("JournalIndex: %v", err)
	}
	if rowByID(idx, idA) != nil {
		t.Error("a player must not get the GM-only note in their index")
	}
	if rowByID(idx, "55555555-5555-5555-5555-555555555555") != nil {
		t.Error("a jot belongs to its page, not the Journal list")
	}
	s12 := rowByID(idx, idB)
	if s12 == nil {
		t.Fatal("the party note must be listed")
	}
	if strings.Contains(s12.Snippet, "Thalrik") || !strings.Contains(s12.Snippet, "a private note") {
		t.Errorf("a link to a note the player cannot see must not reveal its title: %q", s12.Snippet)
	}
	if s12.InCount != 1 {
		t.Errorf("Session 12 is linked from ana's note: inCount = %d, want 1", s12.InCount)
	}
	if s12.SharedWith != nil {
		t.Error("only a note's owner sees who else it is shared with")
	}
}

func TestJournalIndex_WhatTheGMSees(t *testing.T) {
	repo, _ := journalRepo(ashkeep()...)
	svc := NewNoteService(repo)
	idx, err := svc.JournalIndex(context.Background(), "c1", gm("u-gm"))
	if err != nil {
		t.Fatalf("JournalIndex: %v", err)
	}
	if rowByID(idx, idP) != nil {
		t.Error("the GM must not see a player's private note")
	}
	thalrik := rowByID(idx, idA)
	if thalrik == nil {
		t.Fatal("the GM sees their GM-only note")
	}
	// Linked from Session 12 and from the jot; not from ana's private note.
	if thalrik.InCount != 2 {
		t.Errorf("inCount = %d, want 2 (only links the GM can see count)", thalrik.InCount)
	}
	if !reflect.DeepEqual(thalrik.Links, []Link{{Kind: LinkPage, ID: "44444444-4444-4444-4444-444444444444", Label: "Ashkeep Ruins"}}) {
		t.Errorf("links = %+v", thalrik.Links)
	}
	if !strings.Contains(rowByID(idx, idB).Snippet, "Thalrik Mourngrave") {
		t.Error("the GM's snippet labels the link with the title they can see")
	}
	if f := rowByID(idx, "66666666-6666-6666-6666-666666666666"); f == nil || !f.IsFolder || f.Snippet != "" {
		t.Error("folders are listed, without a snippet")
	}
}

func TestSearch(t *testing.T) {
	repo, _ := journalRepo(ashkeep()...)
	svc := NewNoteService(repo)
	ctx := context.Background()

	hits, _ := svc.Search(ctx, "c1", player("u-ana"), "wonder", false)
	if len(hits) != 1 || hits[0].ID != idP || !strings.Contains(hits[0].Snippet, "wonder") {
		t.Errorf("a contents match returns the matching line: %+v", hits)
	}
	hits, _ = svc.Search(ctx, "c1", player("u-ana"), "thalrik", false)
	if len(hits) != 0 {
		t.Errorf("a title the player cannot see must not be findable, even through a link: %+v", hits)
	}
	hits, _ = svc.Search(ctx, "c1", gm("u-gm"), "reminder", true)
	if len(hits) != 1 {
		t.Errorf("with jots, a jot's title matches: %+v", hits)
	}
	if hits, _ = svc.Search(ctx, "c1", gm("u-gm"), "t", false); len(hits) != 0 {
		t.Error("a one-letter query returns nothing")
	}
}

func TestLabels_OnlyWhatTheViewerCanSee(t *testing.T) {
	repo, _ := journalRepo(ashkeep()...)
	svc := NewNoteService(repo)
	got, err := svc.Labels(context.Background(), "c1", player("u-ana"), []string{idA, idB, "not-an-id", idB, "77777777-7777-7777-7777-777777777777"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[idB] == nil || got[idB].Title != "Session 12" {
		t.Errorf("labels = %+v; want only the note ana can see", got)
	}
}

func TestBacklinksAndPageRefs(t *testing.T) {
	repo, _ := journalRepo(ashkeep()...)
	svc := NewNoteService(repo)
	ctx := context.Background()

	refs, _ := svc.Backlinks(ctx, "c1", gm("u-gm"), idA)
	var ids []string
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	if !reflect.DeepEqual(ids, []string{idB, "55555555-5555-5555-5555-555555555555"}) {
		t.Errorf("GM backlinks to Thalrik = %v", ids)
	}

	refs, _ = svc.Backlinks(ctx, "c1", player("u-ana"), idB)
	if len(refs) != 1 || refs[0].ID != idP {
		t.Errorf("ana sees her own note linking to Session 12: %+v", refs)
	}
	refs, _ = svc.Backlinks(ctx, "c1", gm("u-gm"), idB)
	if len(refs) != 0 {
		t.Errorf("the GM must not see ana's private note as a backlink: %+v", refs)
	}

	refs, _ = svc.PageRefs(ctx, "c1", gm("u-gm"), "44444444-4444-4444-4444-444444444444")
	if len(refs) != 1 || refs[0].ID != idA {
		t.Errorf("Journal notes referencing the page = %+v", refs)
	}
}

func TestBulk_OnlyTheCallersOwnNotes(t *testing.T) {
	repo, store := journalRepo(ashkeep()...)
	svc := NewNoteService(repo)
	ctx := context.Background()

	res, err := svc.Bulk(ctx, "c1", gm("u-gm"), BulkRequest{Action: "archive", IDs: []string{idA, idB, idP}})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(res.Done)
	if !reflect.DeepEqual(res.Done, []string{idA, idB}) || !reflect.DeepEqual(res.Skipped, []string{idP}) {
		t.Errorf("archive done %v skipped %v", res.Done, res.Skipped)
	}
	if store[idP].ArchivedAt != nil {
		t.Error("someone else's note must not be archived")
	}

	if _, err := svc.Bulk(ctx, "c1", gm("u-gm"), BulkRequest{Action: "visibility", IDs: []string{idA}, Visibility: VisibilityCustom}); err == nil {
		t.Error("custom with nobody named is refused for the whole request")
	}
	if _, err := svc.Bulk(ctx, "c1", gm("u-gm"), BulkRequest{Action: "shred", IDs: []string{idA}}); err == nil {
		t.Error("an unknown action is refused")
	}
	res, _ = svc.Bulk(ctx, "c1", gm("u-gm"), BulkRequest{Action: "delete", IDs: []string{idB}})
	if len(res.Done) != 1 || store[idB] != nil {
		t.Error("delete removes the caller's own note")
	}
}

// TestUpdate_OnlyContentEditsAddVersions: pinning, sharing and archiving
// leave the text alone, so the History list gains no duplicate entry.
func TestUpdate_OnlyContentEditsAddVersions(t *testing.T) {
	repo, _, _ := memRepo(&Note{ID: "n1", CampaignID: "c1", UserID: "u-owner"})
	versions := 0
	repo.createVersionFn = func(context.Context, *NoteVersion) error { versions++; return nil }
	svc := NewNoteService(repo)
	ctx := context.Background()

	_, _ = svc.Update(ctx, "n1", player("u-owner"), UpdateNoteRequest{Pinned: boolp(true)})
	_, _ = svc.Update(ctx, "n1", player("u-owner"), UpdateNoteRequest{Visibility: visp(VisibilityParty)})
	_, _ = svc.Update(ctx, "n1", player("u-owner"), UpdateNoteRequest{Archived: boolp(true)})
	if versions != 0 {
		t.Errorf("metadata edits added %d versions", versions)
	}
	_, _ = svc.Update(ctx, "n1", player("u-owner"), UpdateNoteRequest{Title: strp("new")})
	if versions != 1 {
		t.Errorf("a content edit must add one version, got %d", versions)
	}
}
