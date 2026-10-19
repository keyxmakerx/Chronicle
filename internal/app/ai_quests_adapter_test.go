package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
)

type questPutRecorder struct {
	quests.QuestService
	view *quests.DMQuestView
	got  quests.QuestPatch
	as   quests.Viewer
}

func (q *questPutRecorder) Get(context.Context, string, string, quests.Viewer) (any, error) {
	return q.view, nil
}

func (q *questPutRecorder) Put(_ context.Context, _, _ string, v quests.Viewer, p quests.QuestPatch) (*quests.DMQuestView, error) {
	q.got, q.as = p, v
	return q.view, nil
}

// TestAIQuestsAdapterWriteKeepsWhatTheBlockDoesNotName pins the merge: a
// block naming only the reward keeps the rest of the notice, and a step with
// stored text keeps its id.
func TestAIQuestsAdapterWriteKeepsWhatTheBlockDoesNotName(t *testing.T) {
	rec := &questPutRecorder{view: &quests.DMQuestView{Version: 3,
		Notice: quests.Notice{Title: "The Missing Goat", Kicker: "Help wanted", Reward: "5 gp", Body: []string{"Gone."}},
		Steps:  []quests.Step{{ID: "s1", Text: "Ask the farmer"}}}}
	a := aiQuestsAdapter{quests: rec}
	reward := "20 gp"
	steps := []records.QuestStep{{Text: "Ask the farmer", Done: true}, {Text: "Follow the tracks"}}
	if err := a.WriteQuest(context.Background(), "c", "goat", records.Actor{UserID: "owner", Role: 3}, records.QuestInput{Reward: &reward, Steps: &steps}); err != nil {
		t.Fatal(err)
	}
	if v, _ := rec.got.Version.Get(); v != 3 {
		t.Errorf("version %d, want the stored 3", v)
	}
	n, ok := rec.got.Notice.Get()
	if !ok || n.Title != "The Missing Goat" || n.Kicker != "Help wanted" || n.Reward != "20 gp" || len(n.Body) != 1 {
		t.Errorf("notice %+v", n)
	}
	if rec.got.Status.Present() {
		t.Error("status was not named, so it must be left as it is")
	}
	st, _ := rec.got.Steps.Get()
	if len(st) != 2 || st[0].ID != "s1" || !st[0].Done || st[1].ID != "" {
		t.Errorf("steps %+v", st)
	}
	if !rec.as.IsDM || rec.as.UserID != "owner" {
		t.Errorf("acted as %+v, want the operator as DM", rec.as)
	}
}
