package quests

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// The Director team reads every quest, board or not; Safe privacy and a
// player's key read only what a board they may open pins, and never a notice
// the Director hid or a quest on a page they cannot open.
func TestAIQuests_Answer(t *testing.T) {
	e := newBoardEnv()
	ctx := context.Background()
	qsvc := NewQuestService(e.quest, e.ents, e.maps, nil)
	for _, id := range []string{"q1", "q-secret"} {
		if _, err := qsvc.Put(ctx, camp, id, dm, parsePatch(t, `{"version":0,"status":"active"}`)); err != nil {
			t.Fatal(err)
		}
	}
	bid := e.board(t, WhoAll)
	for _, id := range []string{"q1", "q-secret"} {
		if _, err := e.svc.CreateItem(ctx, camp, pageHome, bid, dm, ItemInput{Kind: KindNotice, RefID: id}); err != nil {
			t.Fatal(err)
		}
	}
	l := NewAIQuests(qsvc, e.svc)

	tests := []struct {
		name     string
		actor    aiplug.Actor
		has, not []string
	}{
		{"owner", aiplug.Actor{UserID: "dm", Role: 3},
			[]string{"Goat: active", "Secret quest: active", "on Board all (Tavern)"}, nil},
		{"player-safe mode and a player's key", aiplug.Actor{UserID: "pl", Role: 1},
			[]string{"Board all (Tavern)"}, []string{"Secret quest"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, text, err := l.Answer(ctx, camp, tt.actor, aiplug.Record{})
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.has {
				if !strings.Contains(text, w) {
					t.Errorf("answer lacks %q:\n%s", w, text)
				}
			}
			for _, w := range tt.not {
				if strings.Contains(text, w) {
					t.Errorf("answer holds %q:\n%s", w, text)
				}
			}
		})
	}
}

func TestAIQuests_ToolIsNotOwnerOnly(t *testing.T) {
	in := (&AIQuests{}).LookupInfo()
	if in.What != "quests" || in.Tool.Name != "quests" || in.Tool.Owner || in.Tool.PlayerDesc == "" {
		t.Errorf("info = %+v", in)
	}
}
