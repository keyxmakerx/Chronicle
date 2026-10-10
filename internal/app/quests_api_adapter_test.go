package app

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// viewerBoards records the viewer and home each call was made for.
type viewerBoards struct {
	quests.BoardService
	v    quests.Viewer
	home quests.Home
}

func (b *viewerBoards) Homes(_ context.Context, _ string, v quests.Viewer) ([]quests.HomeView, error) {
	b.v = v
	return nil, nil
}

func (b *viewerBoards) View(_ context.Context, _ string, h quests.Home, v quests.Viewer) (*quests.BoardsView, error) {
	b.v, b.home = v, h
	return &quests.BoardsView{}, nil
}

func TestSyncQuestAPIAdapterViewers(t *testing.T) {
	tests := []struct {
		name     string
		players  bool
		wantDM   bool
		wantUser string
	}{
		{"caller reads as the DM", false, true, "u1"},
		{"players view is a plain player", true, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &viewerBoards{}
			a := &syncQuestAPIAdapter{}
			a.attach(struct{ quests.QuestService }{}, b, struct{ quests.PickerService }{})
			if _, err := a.Homes(context.Background(), "c", "u1", tt.players); err != nil {
				t.Fatal(err)
			}
			if b.v.IsDM != tt.wantDM || b.v.UserID != tt.wantUser {
				t.Errorf("viewer %+v", b.v)
			}
			if tt.players && b.v != quests.PlainPlayer() {
				t.Errorf("players view %+v, want PlainPlayer", b.v)
			}
		})
	}
}

func TestSyncQuestAPIAdapterHomes(t *testing.T) {
	b := &viewerBoards{}
	a := &syncQuestAPIAdapter{}
	if _, err := a.Homes(context.Background(), "c", "u1", false); apperror.SafeCode(err) != 404 {
		t.Fatalf("before attach: %v, want 404", err)
	}
	a.attach(struct{ quests.QuestService }{}, b, struct{ quests.PickerService }{})
	if _, err := a.Boards(context.Background(), "c", "u1", syncapi.QuestHome{Kind: "category", ID: "x"}, false); apperror.SafeCode(err) != 404 {
		t.Errorf("bad category id: %v, want 404", err)
	}
	if _, err := a.Boards(context.Background(), "c", "u1", syncapi.QuestHome{Kind: "category", ID: "13"}, false); err != nil || b.home != quests.TypeHome(13) {
		t.Errorf("category home %+v err %v", b.home, err)
	}
	if _, err := a.Boards(context.Background(), "c", "u1", syncapi.QuestHome{Kind: "page", ID: "p1"}, false); err != nil || b.home != quests.PageHome("p1") {
		t.Errorf("page home %+v err %v", b.home, err)
	}
}

func TestSyncQuestAPIAdapterThumbURL(t *testing.T) {
	const id = "b7c17bb1-6563-462c-8b49-5b2e8bd57108"
	if got := (&syncQuestAPIAdapter{}).thumbURL(context.Background(), "2026/03/"+id+".jpg"); got != "/media/"+id+"/thumb/300" {
		t.Errorf("unsigned link %q", got)
	}
	signer := media.NewURLSigner("test-secret-that-is-long-enough-0123456789")
	for _, stored := range []string{id, "2026/03/" + id + ".jpg"} {
		got := (&syncQuestAPIAdapter{signer: signer}).thumbURL(context.Background(), stored)
		u, err := url.Parse(got)
		if err != nil || !strings.HasPrefix(u.Path, "/media/"+id+"/thumb/300") {
			t.Fatalf("signed link %q", got)
		}
		q := u.Query()
		if !signer.VerifyThumbAPIKeyLink(id, "300", q.Get("expires"), q.Get("sig")) {
			t.Errorf("%q: link does not verify as an API-key link", stored)
		}
	}
}
