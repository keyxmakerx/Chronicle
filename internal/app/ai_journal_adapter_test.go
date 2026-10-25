package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
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
