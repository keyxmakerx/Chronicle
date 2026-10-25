package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/widgets/entity_notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

type journalNotesStub struct {
	notes.NoteService
	visible []notes.Note
	index   []notes.IndexRow
}

func (s journalNotesStub) JournalIndex(context.Context, string, permissions.Viewer) (*notes.JournalIndex, error) {
	return &notes.JournalIndex{Notes: s.index}, nil
}
func (s journalNotesStub) ListVisible(context.Context, string, permissions.Viewer, notes.ListScope) ([]notes.Note, error) {
	return s.visible, nil
}

type journalMembersStub struct{ campaigns.CampaignService }

func (journalMembersStub) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	return []campaigns.CampaignMember{{UserID: "own", DisplayName: "Key"}, {UserID: "mira", DisplayName: "Mira"}}, nil
}

func TestAIJournalAdapter(t *testing.T) {
	sp := func(s string) *string { return &s }
	stub := journalNotesStub{
		visible: []notes.Note{
			{ID: "nights", UserID: "own", Title: "Game nights", IsFolder: true},
			{ID: "deep", UserID: "own", Title: "Deep", IsFolder: true, ParentID: sp("nights")},
			{ID: "e1", UserID: "own", Title: "Session 12", ParentID: sp("deep"), Visibility: notes.VisibilityPrivate},
			{ID: "e2", UserID: "mira", Title: "Session 12", ParentID: sp("hidden-folder"), Visibility: notes.VisibilityGM},
			{ID: "j1", UserID: "own", Title: "Knots", EntityID: sp("rope"), Visibility: notes.VisibilityPrivate},
		},
		index: []notes.IndexRow{{ID: "e1", Night: &notes.NightTag{Name: "Session 12", Date: "2026-10-09"}}},
	}
	got, err := aiJournalAdapter{notes: stub, members: journalMembersStub{}}.JournalNotes(context.Background(), "c", permissions.RequestViewer(3, "own"))
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ ID, Author, Folder, Page, Night, Audience string }
	var rows []row
	for _, n := range got {
		rows = append(rows, row{n.ID, n.Author, n.Folder, n.PageID, n.Night, n.Audience})
	}
	want := []row{
		{"e1", "Key", "Game nights / Deep", "", "Session 12, 2026-10-09", "private"},
		{"e2", "Mira", "", "", "", "gm"}, // a folder the viewer can't open is left out of the path
		{"j1", "Key", "", "rope", "", "private"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("got %+v\nwant %+v", rows, want)
	}
}

type playerNotesStub struct {
	entity_notes.Service
	notes  []entity_notes.Note
	viewer *entity_notes.ViewerContext
}

func (s playerNotesStub) ListInCampaign(_ context.Context, v entity_notes.ViewerContext) ([]entity_notes.Note, error) {
	*s.viewer = v
	return s.notes, nil
}

func TestAIJournalAdapter_PlayerNotes(t *testing.T) {
	players := []entity_notes.Note{
		{ID: "p1", EntityID: "inn", AuthorUserID: "mira", Title: "Can I buy rope?", Audience: entity_notes.AudienceDMOnly, BodyHTML: "<p>Please.</p>"},
	}
	tests := []struct {
		name    string
		on      bool
		wantIDs []string
	}{
		{"listed with the add-on on", true, []string{"p1"}},
		{"left out with it off", false, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seen entity_notes.ViewerContext
			a := aiJournalAdapter{notes: journalNotesStub{}, members: journalMembersStub{}, players: playerNotesStub{notes: players, viewer: &seen}, addons: fakeAddonSwitch{on: map[string]bool{entity_notes.AddonSlug: tc.on}}}
			got, err := a.JournalNotes(context.Background(), "c", permissions.RequestViewer(3, "own"))
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, n := range got {
				ids = append(ids, n.ID)
				if !n.PlayerNote || n.Author != "Mira" || n.PageID != "inn" || n.Audience != "dm_only" || n.Body == nil || *n.Body != "<p>Please.</p>" {
					t.Errorf("row %+v", n)
				}
			}
			if !reflect.DeepEqual(ids, tc.wantIDs) {
				t.Errorf("ids %v, want %v", ids, tc.wantIDs)
			}
			if tc.on && (seen.UserID != "own" || seen.CampaignID != "c" || !seen.IsDMGranted) {
				t.Errorf("listed as %+v, want the owner's audience", seen)
			}
		})
	}
}
