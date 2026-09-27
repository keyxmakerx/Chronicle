package notes

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// memRepo is a map-backed NoteRepository for the service's access rules: it
// stores what Update writes, so a test sees the columns the SQL would hold.
func memRepo(notes ...*Note) (*mockNoteRepo, map[string]*Note, *[]string) {
	store := map[string]*Note{}
	for _, n := range notes {
		cp := *n
		cp.derive()
		store[n.ID] = &cp
	}
	var reparented []string
	repo := &mockNoteRepo{
		createFn: func(_ context.Context, n *Note) error {
			cp := *n
			cp.derive()
			store[n.ID] = &cp
			return nil
		},
		findByIDFn: func(_ context.Context, id string) (*Note, error) {
			n, ok := store[id]
			if !ok {
				return nil, apperror.NewNotFound("note not found")
			}
			cp := *n
			cp.derive()
			return &cp, nil
		},
		updateFn: func(_ context.Context, n *Note) error {
			cp := *n
			cp.derive()
			store[n.ID] = &cp
			return nil
		},
		deleteFn: func(_ context.Context, id string) error {
			delete(store, id)
			return nil
		},
		listTreeFn: func(_ context.Context, campaignID string) ([]TreeRow, error) {
			var out []TreeRow
			for _, n := range store {
				if n.CampaignID == campaignID {
					out = append(out, TreeRow{ID: n.ID, ParentID: n.ParentID, UserID: n.UserID, IsFolder: n.IsFolder})
				}
			}
			return out, nil
		},
		reparentToTopFn: func(_ context.Context, ids []string) error {
			reparented = append(reparented, ids...)
			for _, id := range ids {
				if n, ok := store[id]; ok {
					n.ParentID = nil
				}
			}
			return nil
		},
	}
	return repo, store, &reparented
}

// recordingPublisher keeps every event the service publishes.
type recordingPublisher struct{ events []NoteEvent }

func (r *recordingPublisher) PublishNoteEvent(ev NoteEvent) { r.events = append(r.events, ev) }

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }
func visp(v Visibility) *Visibility {
	return &v
}

func TestCreate_DefaultsToPrivate(t *testing.T) {
	repo, store, _ := memRepo()
	svc := NewNoteService(repo)
	n, err := svc.Create(context.Background(), "c1", player("u-writer"), CreateNoteRequest{Title: "Doubts"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.Visibility != VisibilityPrivate {
		t.Errorf("a new note must be private to its writer, got %q", n.Visibility)
	}
	if store[n.ID].IsShared || store[n.ID].SharedWithGM {
		t.Error("a private note must store no sharing")
	}
}

func TestCreate_WithVisibility(t *testing.T) {
	repo, _, _ := memRepo()
	svc := NewNoteService(repo)
	n, err := svc.Create(context.Background(), "c1", player("u-writer"), CreateNoteRequest{Title: "For the GM", Visibility: VisibilityGM})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.Visibility != VisibilityGM || !n.SharedWithGM || n.IsShared {
		t.Errorf("want a GM-only note, got %+v", n)
	}

	if _, err := svc.Create(context.Background(), "c1", player("u-writer"), CreateNoteRequest{Title: "x", Visibility: VisibilityCustom}); err == nil {
		t.Error("custom with nobody named must be refused")
	}
	if _, err := svc.Create(context.Background(), "c1", player("u-writer"), CreateNoteRequest{Title: "x", Visibility: "everyone"}); err == nil {
		t.Error("an unknown visibility must be refused")
	}
}

func TestUpdate_Visibility(t *testing.T) {
	tests := []struct {
		name       string
		vis        Visibility
		sharedWith []string
		wantErr    bool
		check      func(t *testing.T, n *Note)
	}{
		{name: "gm", vis: VisibilityGM, check: func(t *testing.T, n *Note) {
			if !n.SharedWithGM || n.IsShared || n.SharedWith != nil {
				t.Errorf("gm must set only the GM flag, got %+v", n)
			}
		}},
		{name: "party", vis: VisibilityParty, check: func(t *testing.T, n *Note) {
			if !n.IsShared || n.SharedWithGM || n.SharedWith != nil {
				t.Errorf("party must set only is_shared, got %+v", n)
			}
		}},
		{name: "custom", vis: VisibilityCustom, sharedWith: []string{" u-a ", "u-a", "u-owner", "u-b"}, check: func(t *testing.T, n *Note) {
			if n.IsShared || n.SharedWithGM || !reflect.DeepEqual(n.SharedWith, []string{"u-a", "u-b"}) {
				t.Errorf("custom must store the cleaned list without the owner, got %+v", n)
			}
		}},
		{name: "custom with only the owner is nobody", vis: VisibilityCustom, sharedWith: []string{"u-owner"}, wantErr: true},
		{name: "private", vis: VisibilityPrivate, check: func(t *testing.T, n *Note) {
			if n.IsShared || n.SharedWithGM || n.SharedWith != nil {
				t.Errorf("private must clear all sharing, got %+v", n)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, store, _ := memRepo(&Note{ID: "n1", CampaignID: "c1", UserID: "u-owner", IsShared: true, SharedWith: []string{"old"}})
			svc := NewNoteService(repo)
			_, err := svc.Update(context.Background(), "n1", player("u-owner"), UpdateNoteRequest{Visibility: visp(tt.vis), SharedWith: tt.sharedWith})
			if tt.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			tt.check(t, store["n1"])
		})
	}
}

// TestUpdate_LegacySharing pins what older clients (the Foundry module, the
// previous floating panel) get when they send isShared/sharedWith.
func TestUpdate_LegacySharing(t *testing.T) {
	gmNote := func() *Note { return &Note{ID: "n1", CampaignID: "c1", UserID: "u-owner", SharedWithGM: true} }
	tests := []struct {
		name   string
		req    UpdateNoteRequest
		wantGM bool
		want   Visibility
	}{
		{"sharing with the party replaces a GM share", UpdateNoteRequest{IsShared: boolp(true)}, false, VisibilityParty},
		{"naming people replaces a GM share", UpdateNoteRequest{SharedWith: []string{"u-a"}}, false, VisibilityCustom},
		{"the old picker's Private (both off) clears it", UpdateNoteRequest{IsShared: boolp(false), SharedWith: []string{}}, false, VisibilityPrivate},
		{"a partial isShared=false keeps it", UpdateNoteRequest{IsShared: boolp(false)}, true, VisibilityGM},
		{"a title-only edit keeps it", UpdateNoteRequest{Title: strp("renamed")}, true, VisibilityGM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, store, _ := memRepo(gmNote())
			svc := NewNoteService(repo)
			if _, err := svc.Update(context.Background(), "n1", player("u-owner"), tt.req); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if store["n1"].SharedWithGM != tt.wantGM || store["n1"].Visibility != tt.want {
				t.Errorf("GM flag %v visibility %q, want %v %q", store["n1"].SharedWithGM, store["n1"].Visibility, tt.wantGM, tt.want)
			}
		})
	}
}

func TestUpdate_ArchiveUnpinsAndUnarchiveClears(t *testing.T) {
	repo, store, _ := memRepo(&Note{ID: "n1", CampaignID: "c1", UserID: "u-owner", Pinned: true})
	svc := NewNoteService(repo)

	n, err := svc.Update(context.Background(), "n1", player("u-owner"), UpdateNoteRequest{Archived: boolp(true)})
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if !n.Archived || store["n1"].ArchivedAt == nil {
		t.Error("archiving must set archived_at")
	}
	if store["n1"].Pinned {
		t.Error("an archived note must leave the pinned group")
	}

	if _, err := svc.Update(context.Background(), "n1", player("u-owner"), UpdateNoteRequest{Archived: boolp(false)}); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if store["n1"].ArchivedAt != nil {
		t.Error("unarchiving must clear archived_at")
	}
}

// TestUpdate_FolderTarget covers filing: only into a folder of the same
// campaign the editor can see, and never a folder into itself.
func TestUpdate_FolderTarget(t *testing.T) {
	seed := func() []*Note {
		return []*Note{
			{ID: "note", CampaignID: "c1", UserID: "u-owner"},
			{ID: "mine", CampaignID: "c1", UserID: "u-owner", IsFolder: true},
			{ID: "their-private", CampaignID: "c1", UserID: "u-other", IsFolder: true},
			{ID: "their-party", CampaignID: "c1", UserID: "u-other", IsFolder: true, IsShared: true},
			{ID: "elsewhere", CampaignID: "c2", UserID: "u-owner", IsFolder: true},
			{ID: "plain", CampaignID: "c1", UserID: "u-owner"},
			{ID: "outer", CampaignID: "c1", UserID: "u-owner", IsFolder: true},
			{ID: "inner", CampaignID: "c1", UserID: "u-owner", IsFolder: true, ParentID: strp("outer")},
		}
	}
	tests := []struct {
		name    string
		noteID  string
		target  string
		wantErr bool
	}{
		{"own folder", "note", "mine", false},
		{"someone's party folder", "note", "their-party", false},
		{"someone's private folder is invisible", "note", "their-private", true},
		{"a folder in another campaign", "note", "elsewhere", true},
		{"a note that is not a folder", "note", "plain", true},
		{"a folder into itself", "outer", "outer", true},
		{"a folder into its own child", "outer", "inner", true},
		{"a missing folder", "note", "nope", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, store, _ := memRepo(seed()...)
			svc := NewNoteService(repo)
			_, err := svc.Update(context.Background(), tt.noteID, player("u-owner"), UpdateNoteRequest{ParentID: strp(tt.target)})
			if tt.wantErr {
				if err == nil {
					t.Fatal("want the move refused")
				}
				if p := store[tt.noteID].ParentID; p != nil && *p == tt.target {
					t.Error("a refused move must not be written")
				}
				return
			}
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			if p := store[tt.noteID].ParentID; p == nil || *p != tt.target {
				t.Errorf("parent = %v, want %s", p, tt.target)
			}
		})
	}
}

// TestDelete_FolderKeepsOtherPeoplesNotes: the folder cascade may only take
// the folder owner's notes; anyone else's move to the top level first.
func TestDelete_FolderKeepsOtherPeoplesNotes(t *testing.T) {
	repo, _, reparented := memRepo(
		&Note{ID: "folder", CampaignID: "c1", UserID: "u-owner", IsFolder: true, IsShared: true},
		&Note{ID: "own-child", CampaignID: "c1", UserID: "u-owner", ParentID: strp("folder")},
		&Note{ID: "their-child", CampaignID: "c1", UserID: "u-other", ParentID: strp("folder")},
		&Note{ID: "own-sub", CampaignID: "c1", UserID: "u-owner", IsFolder: true, ParentID: strp("folder")},
		&Note{ID: "their-grandchild", CampaignID: "c1", UserID: "u-other", ParentID: strp("own-sub")},
		&Note{ID: "their-sub", CampaignID: "c1", UserID: "u-other", IsFolder: true, ParentID: strp("folder")},
		&Note{ID: "own-inside-theirs", CampaignID: "c1", UserID: "u-owner", ParentID: strp("their-sub")},
	)
	svc := NewNoteService(repo)
	if err := svc.Delete(context.Background(), "folder"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got := append([]string{}, *reparented...)
	sort.Strings(got)
	want := []string{"their-child", "their-grandchild", "their-sub"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rescued %v, want %v", got, want)
	}
}

// TestEvents_CarryAudienceNotContent pins what the live-updates layer is
// told: IDs, the page for a jot, and exactly who may hear of it.
func TestEvents_CarryAudienceNotContent(t *testing.T) {
	repo, _, _ := memRepo(&Note{ID: "n1", CampaignID: "c1", UserID: "u-owner", IsShared: true, EntityID: strp("page-1")})
	svc := NewNoteServiceWithAttachments(repo, nil)
	pub := &recordingPublisher{}
	svc.SetEventPublisher(pub)

	// Party -> private: both the old audience (everyone) and the new one
	// (the owner) must hear, so viewers who lost access drop the note.
	if _, err := svc.Update(context.Background(), "n1", player("u-owner"), UpdateNoteRequest{Visibility: visp(VisibilityPrivate)}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Private edit: the owner alone.
	if _, err := svc.Update(context.Background(), "n1", player("u-owner"), UpdateNoteRequest{Title: strp("secret")}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Share with the GM: the owner, and GMs.
	if _, err := svc.Update(context.Background(), "n1", player("u-owner"), UpdateNoteRequest{Visibility: visp(VisibilityGM)}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := svc.Delete(context.Background(), "n1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if len(pub.events) != 4 {
		t.Fatalf("got %d events, want 4", len(pub.events))
	}
	for _, ev := range pub.events {
		if ev.NoteID != "n1" || ev.CampaignID != "c1" || ev.EntityID == nil || *ev.EntityID != "page-1" {
			t.Errorf("event carries the wrong ids: %+v", ev)
		}
	}
	if !pub.events[0].Audience.Everyone {
		t.Errorf("a change away from the party must reach everyone who could see it, got %+v", pub.events[0].Audience)
	}
	if a := pub.events[1].Audience; a.Everyone || a.GMs || !reflect.DeepEqual(a.Users, []string{"u-owner"}) {
		t.Errorf("a private note's edit must reach its owner alone, got %+v", a)
	}
	if a := pub.events[2].Audience; a.Everyone || !a.GMs || !reflect.DeepEqual(a.Users, []string{"u-owner"}) {
		t.Errorf("a GM share must reach the owner and GMs, got %+v", a)
	}
	if a := pub.events[3]; a.Type != "deleted" || !a.Audience.GMs {
		t.Errorf("the delete must reach the audience the note had, got %+v", a)
	}
}
