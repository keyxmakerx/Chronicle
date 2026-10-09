package quests

import (
	"context"
	"errors"
	"testing"
)

type heard struct {
	kind   string
	id     string
	home   Home
	ver    int
	dmOnly bool
}

type recAnnouncer struct{ got []heard }

func (r *recAnnouncer) QuestChanged(_, entityID string, version int, dmOnly bool) {
	r.got = append(r.got, heard{kind: "quest", id: entityID, ver: version, dmOnly: dmOnly})
}

func (r *recAnnouncer) BoardsChanged(_ string, h Home, dmOnly bool) {
	r.got = append(r.got, heard{kind: "boards", home: h, dmOnly: dmOnly})
}

// stubQuests and stubBoards answer with err, so the wrappers can be tested
// on their own; methods the tests never call panic through the nil interface.
type stubQuests struct {
	QuestService
	err error
}

func (s stubQuests) Put(context.Context, string, string, Viewer, QuestPatch) (*DMQuestView, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &DMQuestView{Version: 7}, nil
}

type stubBoards struct {
	BoardService
	err error
}

func (s stubBoards) CreateItem(context.Context, string, Home, string, Viewer, ItemInput) (*ItemView, error) {
	return &ItemView{}, s.err
}

func (s stubBoards) DeleteBoard(context.Context, string, Home, string, Viewer) error { return s.err }

func TestWithAnnouncer(t *testing.T) {
	ents := newFakeEntities()
	ents.add("c1", "open", "Town", false)
	ents.add("c1", "hidden", "Secret lair", true)
	ents.add("c2", "foreign", "Elsewhere", false)
	failed := errors.New("refused")

	tests := []struct {
		name string
		call func(q QuestService, b BoardService)
		err  error
		want []heard
	}{
		{"quest saved on an open page reaches players", func(q QuestService, _ BoardService) {
			_, _ = q.Put(context.Background(), "c1", "open", Viewer{}, QuestPatch{})
		}, nil, []heard{{kind: "quest", id: "open", ver: 7}}},
		{"quest saved on a hidden page is DM only", func(q QuestService, _ BoardService) {
			_, _ = q.Put(context.Background(), "c1", "hidden", Viewer{}, QuestPatch{})
		}, nil, []heard{{kind: "quest", id: "hidden", ver: 7, dmOnly: true}}},
		{"a page of another campaign is DM only", func(q QuestService, _ BoardService) {
			_, _ = q.Put(context.Background(), "c1", "foreign", Viewer{}, QuestPatch{})
		}, nil, []heard{{kind: "quest", id: "foreign", ver: 7, dmOnly: true}}},
		{"a refused save says nothing", func(q QuestService, _ BoardService) {
			_, _ = q.Put(context.Background(), "c1", "open", Viewer{}, QuestPatch{})
		}, failed, nil},
		{"item pinned on an open page's board", func(_ QuestService, b BoardService) {
			_, _ = b.CreateItem(context.Background(), "c1", PageHome("open"), "b1", Viewer{}, ItemInput{})
		}, nil, []heard{{kind: "boards", home: PageHome("open")}}},
		{"item pinned on a hidden page's board is DM only", func(_ QuestService, b BoardService) {
			_, _ = b.CreateItem(context.Background(), "c1", PageHome("hidden"), "b1", Viewer{}, ItemInput{})
		}, nil, []heard{{kind: "boards", home: PageHome("hidden"), dmOnly: true}}},
		{"a category's boards reach every member", func(_ QuestService, b BoardService) {
			_ = b.DeleteBoard(context.Background(), "c1", TypeHome(13), "b1", Viewer{})
		}, nil, []heard{{kind: "boards", home: TypeHome(13)}}},
		{"a refused board change says nothing", func(_ QuestService, b BoardService) {
			_ = b.DeleteBoard(context.Background(), "c1", TypeHome(13), "b1", Viewer{})
		}, failed, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recAnnouncer{}
			q, b := WithAnnouncer(stubQuests{err: tt.err}, stubBoards{err: tt.err}, ents, rec)
			tt.call(q, b)
			if len(rec.got) != len(tt.want) {
				t.Fatalf("announced %+v, want %+v", rec.got, tt.want)
			}
			for i := range tt.want {
				if rec.got[i] != tt.want[i] {
					t.Errorf("announcement %d = %+v, want %+v", i, rec.got[i], tt.want[i])
				}
			}
		})
	}
}

func TestWithAnnouncer_NilLeavesServicesAlone(t *testing.T) {
	q, b := stubQuests{}, stubBoards{}
	gq, gb := WithAnnouncer(q, b, newFakeEntities(), nil)
	if gq != QuestService(q) || gb != BoardService(b) {
		t.Fatal("a nil announcer should return the services unchanged")
	}
}
