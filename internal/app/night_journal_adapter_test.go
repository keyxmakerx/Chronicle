package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// entryNoteStore is an in-memory notes.NoteService keeping what the night
// entries adapter writes: the parent folder and whether a note is a folder.
type entryNoteStore struct {
	notes.NoteService
	byID    map[string]*notes.Note
	creates int
	failing bool
}

func (f *entryNoteStore) GetByID(_ context.Context, id string) (*notes.Note, error) {
	if n, ok := f.byID[id]; ok {
		c := *n
		return &c, nil
	}
	return nil, apperror.NewNotFound("note not found")
}

func (f *entryNoteStore) Create(_ context.Context, campaignID string, v permissions.Viewer, req notes.CreateNoteRequest) (*notes.Note, error) {
	f.creates++
	if f.failing {
		return nil, errors.New("db down")
	}
	n := &notes.Note{ID: req.ID, CampaignID: campaignID, UserID: v.UserID(), Title: req.Title, IsFolder: req.IsFolder, ParentID: req.ParentID}
	f.byID[req.ID] = n
	return n, nil
}

func TestNightJournalAdapter_CreateEntry(t *testing.T) {
	store := &entryNoteStore{byID: map[string]*notes.Note{}}
	a := &nightJournalAdapter{svc: store}
	ana := permissions.RequestViewer(permissions.RolePlayer, "ana")
	ctx := context.Background()

	if err := a.CreateEntry(ctx, "c1", ana, "e1", "Game night, Sat 10 Oct"); err != nil {
		t.Fatal(err)
	}
	e := store.byID["e1"]
	if e == nil || e.UserID != "ana" || e.ParentID == nil {
		t.Fatalf("entry = %+v, want ana's, in a folder", e)
	}
	folder := store.byID[*e.ParentID]
	if folder == nil || !folder.IsFolder || folder.Title != nightFolderTitle || folder.UserID != "ana" {
		t.Fatalf("folder = %+v", folder)
	}

	// A second night goes in the same folder, even once it is renamed.
	folder.Title = "Sessions"
	if err := a.CreateEntry(ctx, "c1", ana, "e2", "Game night, Sat 17 Oct"); err != nil {
		t.Fatal(err)
	}
	if p := store.byID["e2"].ParentID; p == nil || *p != folder.ID || store.creates != 3 {
		t.Errorf("second entry parent = %v after %d creates, want the same folder and 3", p, store.creates)
	}

	// An entry that exists is left alone.
	if err := a.CreateEntry(ctx, "c1", ana, "e1", "Again"); err != nil || store.creates != 3 {
		t.Errorf("existing entry: err %v, %d creates", err, store.creates)
	}

	// Each member has their own folder.
	bo := permissions.RequestViewer(permissions.RolePlayer, "bo")
	if err := a.CreateEntry(ctx, "c1", bo, "e3", "Game night, Sat 10 Oct"); err != nil {
		t.Fatal(err)
	}
	if *store.byID["e3"].ParentID == folder.ID {
		t.Error("two members share a folder")
	}

	store.failing = true
	if err := a.CreateEntry(ctx, "c1", ana, "e4", "Game night"); err == nil {
		t.Error("a failed create must be reported, so the night is tried again")
	}
}

func TestNightJournalAdapter_ReadableEntries(t *testing.T) {
	page := "11111111-1111-1111-1111-111111111111"
	body := `<p>Met <a data-mention-id="` + page + `">@Maudry</a>.</p>`
	store := &entryNoteStore{byID: map[string]*notes.Note{
		"mine":   {ID: "mine", CampaignID: "c1", UserID: "ana", Title: "Mine", EntryHTML: &body},
		"gm":     {ID: "gm", CampaignID: "c1", UserID: "bo", Title: "Bo's", SharedWithGM: true},
		"other":  {ID: "other", CampaignID: "c2", UserID: "ana", Title: "Elsewhere"},
		"hidden": {ID: "hidden", CampaignID: "c1", UserID: "bo", Title: "Bo's private"},
	}}
	a := &nightJournalAdapter{svc: store}
	ana := permissions.RequestViewer(permissions.RolePlayer, "ana")
	got, err := a.ReadableEntries(context.Background(), "c1", ana, []string{"mine", "gm", "other", "hidden", "deleted"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "mine" || len(got[0].PageIDs) != 1 || got[0].PageIDs[0] != page || got[0].Lines[page] != "Met @Maudry." {
		t.Errorf("ana reads %+v, want only her own entry with its page", got)
	}
	dm := permissions.RequestViewer(permissions.RoleOwner, "dm")
	got, _ = a.ReadableEntries(context.Background(), "c1", dm, []string{"mine", "gm", "hidden"})
	if len(got) != 1 || got[0].ID != "gm" {
		t.Errorf("the DM reads %+v, want only the entry shared with the GM", got)
	}
}
