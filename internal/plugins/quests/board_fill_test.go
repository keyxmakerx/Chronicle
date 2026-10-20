package quests

import (
	"context"
	"testing"
)

func (e boardEnv) setStatus(id, status string) {
	e.quest.docs[id] = []byte(`{"status":"` + status + `"}`)
	e.quest.camp[id] = camp
}

func (e boardEnv) fillWith(t *testing.T, bid string, body string) {
	t.Helper()
	var p BoardPatch
	if err := jsonUnmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PatchBoard(context.Background(), camp, pageHome, bid, dm, p); err != nil {
		t.Fatal(err)
	}
}

// notices lists the quest ids on a board as v sees them, and which are auto.
func (e boardEnv) notices(t *testing.T, v Viewer) map[string]bool {
	t.Helper()
	view, err := e.svc.View(context.Background(), camp, pageHome, v)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, b := range view.Boards {
		for _, it := range b.Items {
			if it.Kind == KindNotice {
				out[it.QuestID] = it.Auto
			}
		}
	}
	return out
}

func TestFillPinsMatchingQuestsAndFiltersPerViewer(t *testing.T) {
	e := newBoardEnv()
	e.ents.add(camp, "q2", "Wolves", false)
	e.ents.add(camp, "q3", "Done deal", false)
	e.setStatus("q1", StatusNotStarted)
	e.setStatus("q2", StatusActive)
	e.setStatus("q3", StatusDone)
	e.setStatus("q-secret", StatusActive)
	bid := e.board(t, WhoAll)
	ctx := context.Background()
	// q2 is already pinned by hand, so the board must not pin it twice.
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindNotice, RefID: "q2"}); err != nil {
		t.Fatal(err)
	}
	e.fillWith(t, bid, `{"fill":["active","not_started","active"]}`)

	b, _ := e.repo.GetBoard(ctx, camp, pageHome, bid)
	if got := b.Fill; len(got) != 2 || got[0] != StatusNotStarted || got[1] != StatusActive {
		t.Fatalf("fill stored as %v, want canonical [not_started active]", got)
	}
	tests := []struct {
		name string
		v    Viewer
		want map[string]bool
	}{
		{"dm sees the hidden quest too", dm, map[string]bool{"q1": true, "q2": false, "q-secret": true}},
		{"player sees only what they may open", player, map[string]bool{"q1": true, "q2": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.notices(t, tt.v)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
			for id, auto := range tt.want {
				if a, ok := got[id]; !ok || a != auto {
					t.Errorf("%s: got auto=%v present=%v, want auto=%v", id, a, ok, auto)
				}
			}
		})
	}
}

func TestFillQuestFollowsStatus(t *testing.T) {
	e := newBoardEnv()
	e.setStatus("q1", StatusActive)
	bid := e.board(t, WhoAll)
	ctx := context.Background()
	e.fillWith(t, bid, `{"fill":["not_started","active"]}`)
	note, err := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindNote, Text: "the goat knows"})
	if err != nil {
		t.Fatal(err)
	}
	var autoID string
	for _, it := range e.repo.items {
		if it.Auto {
			autoID = it.ID
		}
	}
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, player, ItemInput{Kind: KindString, From: note.ID, To: autoID}); err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		status    string
		wantOn    bool
		wantHomes int
	}{
		{StatusActive, true, 0},
		{StatusDone, false, 1},
		{StatusDone, false, 0},
		{StatusNotStarted, true, 1},
	}
	for _, st := range steps {
		e.setStatus("q1", st.status)
		homes, err := e.svc.FillQuest(ctx, camp, "q1")
		if err != nil {
			t.Fatal(err)
		}
		if len(homes) != st.wantHomes {
			t.Errorf("%s: %d homes changed, want %d", st.status, len(homes), st.wantHomes)
		}
		if _, on := e.notices(t, dm)["q1"]; on != st.wantOn {
			t.Errorf("%s: on board %v, want %v", st.status, on, st.wantOn)
		}
	}
	for _, it := range e.repo.items {
		if it.Kind == KindString {
			t.Errorf("the string to the taken-down notice should have gone with it")
		}
	}
}

func TestFillStopsAndKeepsHandPinned(t *testing.T) {
	e := newBoardEnv()
	e.ents.add(camp, "q2", "Wolves", false)
	e.setStatus("q1", StatusActive)
	e.setStatus("q2", StatusActive)
	bid := e.board(t, WhoAll)
	ctx := context.Background()
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindNotice, RefID: "q2"}); err != nil {
		t.Fatal(err)
	}
	e.fillWith(t, bid, `{"fill":["active"]}`)
	e.fillWith(t, bid, `{"fill":null}`)
	got := e.notices(t, dm)
	if len(got) != 1 || got["q2"] {
		t.Fatalf("after stopping, got %v; want only the hand-pinned q2", got)
	}
}

func TestFillValidationAndRemoval(t *testing.T) {
	e := newBoardEnv()
	e.setStatus("q1", StatusActive)
	bid := e.board(t, WhoAll)
	ctx := context.Background()
	var bad BoardPatch
	_ = jsonUnmarshal([]byte(`{"fill":["open"]}`), &bad)
	if _, err := e.svc.PatchBoard(ctx, camp, pageHome, bid, dm, bad); code(err) != 422 && code(err) != 400 {
		t.Fatalf("unknown status: got %v, want a validation error", err)
	}
	if _, err := e.svc.PatchBoard(ctx, camp, pageHome, bid, player, bad); code(err) != 403 {
		t.Fatalf("player setting fill: got %v, want 403", err)
	}
	e.fillWith(t, bid, `{"fill":["active"]}`)
	var autoID string
	for _, it := range e.repo.items {
		if it.Auto {
			autoID = it.ID
		}
	}
	if err := e.svc.DeleteItem(ctx, camp, pageHome, bid, autoID, dm); err == nil {
		t.Fatal("removing a notice the board placed itself should be refused while it fills")
	}
	if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindNotice, RefID: "q1"}); err == nil {
		t.Fatal("pinning a quest the board already shows should be refused")
	}
	if _, err := e.svc.PatchItem(ctx, camp, pageHome, bid, autoID, dm, parseItemPatch(t, `{"hidden":true}`)); err != nil {
		t.Fatalf("hiding it instead: %v", err)
	}
	if _, on := e.notices(t, player)["q1"]; on {
		t.Error("a hidden auto notice must not reach players")
	}
}

func TestFillRespectsTheBoardCap(t *testing.T) {
	e := newBoardEnv()
	bid := e.board(t, WhoAll)
	for i := 0; i < MaxItemsPerBoard; i++ {
		e.repo.items = append(e.repo.items, Item{ID: "n" + string(rune('A'+i)), BoardID: bid, CampaignID: camp, Kind: KindNote, Text: "x"})
	}
	e.setStatus("q1", StatusActive)
	e.fillWith(t, bid, `{"fill":["active"]}`)
	if n := len(e.repo.items); n != MaxItemsPerBoard {
		t.Fatalf("a full board grew to %d items", n)
	}
	view, err := e.svc.View(context.Background(), camp, pageHome, dm)
	if err != nil || len(view.Boards) != 1 || !view.Boards[0].Full {
		t.Fatalf("a full filling board must say so: %+v err=%v", view, err)
	}
}

type countingBoards struct {
	BoardService
	calls []string
}

func (c *countingBoards) FillQuest(_ context.Context, _, questID string) ([]Home, error) {
	c.calls = append(c.calls, questID)
	return nil, nil
}

type putOnlyQuests struct {
	QuestService
	err error
}

func (s putOnlyQuests) Put(context.Context, string, string, Viewer, QuestPatch) (*DMQuestView, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &DMQuestView{Status: StatusDone}, nil
}

func TestWithBoardFillRunsOnlyAfterASave(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		calls int
	}{
		{"saved", nil, 1},
		{"refused", errForbidden("no"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &countingBoards{}
			q := WithBoardFill(putOnlyQuests{err: tt.err}, b)
			_, _ = q.Put(context.Background(), camp, "q1", dm, QuestPatch{})
			if len(b.calls) != tt.calls {
				t.Errorf("FillQuest called %d times, want %d", len(b.calls), tt.calls)
			}
		})
	}
}
