package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

const goodBlocks = "---\nname: Coral Court\ntype: location\n---\n\n# Coral Court\n\nA market under the tide.\n\n---\nname: Old Harl\naction: update\n---\n\nHe sells nets now."

// askRig is an "Ask for changes" connection and a server that queues into
// a real service.
func askRig(t *testing.T) (*Server, Session, *memRepo) {
	t.Helper()
	svc, repo, _, _ := newTestService()
	if _, _, err := svc.Connect(context.Background(), "camp-1", "owner-1", aiexport.PrivacyModeSafe, true); err != nil {
		t.Fatal(err)
	}
	conn := *repo.conns[0]
	conn.ClientName = "Claude"
	return &Server{Asks: svc}, Session{Conn: &conn, CampaignName: "Ashfall", Actor: actorFor(&conn)}, repo
}

func callBody(name string, args map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 7, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args}})
	return b
}

type toolReply struct {
	Content []struct{ Text string } `json:"content"`
	IsError bool                    `json:"isError"`
}

func toolText(t *testing.T, out Outcome) toolReply {
	t.Helper()
	r := decode(t, out)
	if r.Error != nil {
		t.Fatalf("rpc error: %+v", r.Error)
	}
	var tr toolReply
	if err := json.Unmarshal(r.Result, &tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

// An "Ask for changes" connection adds the two tools; ask_for_changes says it
// isn't read-only but isn't destructive either, since it applies nothing.
func TestHandle_ToolsListForAskingConnection(t *testing.T) {
	s, sess, _ := askRig(t)
	out := s.Handle(context.Background(), sess, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	var res struct {
		Tools []struct {
			Name        string         `json:"name"`
			Annotations map[string]any `json:"annotations"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(decode(t, out).Result, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != len(tools) {
		t.Fatalf("got %d tools, want %d", len(res.Tools), len(tools))
	}
	for _, tl := range res.Tools {
		wantRead := tl.Name != toolAsk
		if tl.Annotations["readOnlyHint"] != wantRead || tl.Annotations["destructiveHint"] != false {
			t.Errorf("%s annotations = %v", tl.Name, tl.Annotations)
		}
	}
	init := s.Handle(context.Background(), sess, []byte(`{"jsonrpc":"2.0","id":2,"method":"initialize"}`))
	for _, want := range []string{toolAsk, "says yes to each one", toolHowTo} {
		if !strings.Contains(string(init.Body), want) {
			t.Errorf("instructions missing %q: %s", want, init.Body)
		}
	}
}

// A request waits for the owner, untouched; the AI is told nothing changed,
// and the log quotes the AI's own summary, never the blocks.
func TestHandle_AskForChangesQueues(t *testing.T) {
	s, sess, repo := askRig(t)
	out := s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{
		"summary": "A market\nand its netmaker", "changes": goodBlocks}))
	tr := toolText(t, out)
	if tr.IsError || !strings.Contains(tr.Content[0].Text, "1 to add, 1 to change") || !strings.Contains(tr.Content[0].Text, "Nothing has changed yet") {
		t.Fatalf("reply = %+v", tr)
	}
	if len(repo.reqs) != 1 {
		t.Fatalf("stored %d requests", len(repo.reqs))
	}
	q := repo.reqs[0]
	if q.Body != goodBlocks || q.Status != RequestWaiting || q.Blocks != 2 || q.Summary != "A market and its netmaker" || q.AppName != "Claude" || q.CampaignID != "camp-1" {
		t.Errorf("request = %+v", q)
	}
	if out.Called == nil || out.Called.Summary != "Asked for 2 changes: “A market and its netmaker”" || out.Called.Note != "sent for your review" || !out.Called.OK {
		t.Errorf("log line = %+v", out.Called)
	}
	if strings.Contains(out.Called.Summary, "Coral Court") {
		t.Error("the log line holds the blocks")
	}
}

func TestHandle_AskForChangesRefusals(t *testing.T) {
	cases := []struct {
		name    string
		changes string
		want    string
	}{
		{"empty", "  ", "it needs changes"},
		{"a lookup", "---\nkind: lookup\nwhat: pages\n---\n", "use the lookup tools"},
		{"a part label", "---\nkind: part\nanswer: big\npart: 1\nof: 2\n---\n\n" + goodBlocks, "part label"},
		{"a block that won't parse", "---\nname: [unclosed\n---\nbody", "block 1"},
		{"too long", strings.Repeat("a", changesMax+1), "too long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, sess, repo := askRig(t)
			out := s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "x", "changes": tc.changes}))
			tr := toolText(t, out)
			if !tr.IsError || !strings.Contains(tr.Content[0].Text, tc.want) {
				t.Errorf("reply = %+v, want an error with %q", tr, tc.want)
			}
			if len(repo.reqs) != 0 {
				t.Errorf("a refused request was stored")
			}
			if out.Called == nil || out.Called.OK {
				t.Errorf("log line = %+v", out.Called)
			}
		})
	}
}

// An "Only look" connection has no ask_for_changes at all.
func TestHandle_OnlyLookCannotAsk(t *testing.T) {
	s, sess, repo := askRig(t)
	sess.Conn.CanAsk = false
	for _, name := range []string{toolAsk, toolHowTo} {
		r := decode(t, s.Handle(context.Background(), sess, callBody(name, map[string]any{"summary": "x", "changes": goodBlocks})))
		if r.Error == nil || !strings.Contains(r.Error.Message, "no tool called") {
			t.Errorf("%s: got %+v", name, r)
		}
	}
	if _, err := s.Asks.Ask(context.Background(), sess.Conn, "x", goodBlocks); !errors.Is(err, ErrCannotAsk) {
		t.Errorf("Ask on an Only look connection: %v", err)
	}
	if len(repo.reqs) != 0 {
		t.Error("stored a request")
	}
}

// At most requestsWaiting wait at once, so an AI can't bury the owner.
func TestHandle_AskForChangesWaitingCap(t *testing.T) {
	s, sess, repo := askRig(t)
	for i := 0; i < requestsWaiting; i++ {
		if tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": fmt.Sprint(i), "changes": goodBlocks}))); tr.IsError {
			t.Fatalf("request %d refused: %+v", i+1, tr)
		}
	}
	tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "one more", "changes": goodBlocks})))
	if !tr.IsError || !strings.Contains(tr.Content[0].Text, "already waiting") {
		t.Errorf("reply = %+v", tr)
	}
	if len(repo.reqs) != requestsWaiting {
		t.Errorf("stored %d", len(repo.reqs))
	}
	// Answering one makes room.
	if err := s.Asks.(*Service).TurnDown(context.Background(), "camp-1", 1); err != nil {
		t.Fatal(err)
	}
	if tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "now", "changes": goodBlocks}))); tr.IsError {
		t.Errorf("after one was answered: %+v", tr)
	}
}

func TestHandle_HowToWriteChanges(t *testing.T) {
	s, sess, _ := askRig(t)
	var gotUser, gotCampaign string
	s.Instructions = func(_ context.Context, campaignID, _, userID string) (string, error) {
		gotUser, gotCampaign = userID, campaignID
		return "BLOCK FORMATS", nil
	}
	out := s.Handle(context.Background(), sess, callBody(toolHowTo, nil))
	tr := toolText(t, out)
	if tr.IsError || !strings.Contains(tr.Content[0].Text, "BLOCK FORMATS") || !strings.Contains(tr.Content[0].Text, "send them with "+toolAsk) {
		t.Errorf("reply = %+v", tr)
	}
	if gotUser != "owner-1" || gotCampaign != "camp-1" {
		t.Errorf("written for %q in %q", gotUser, gotCampaign)
	}
	s.Instructions = func(context.Context, string, string, string) (string, error) { return "", errors.New("db down") }
	if tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolHowTo, nil))); !tr.IsError || strings.Contains(tr.Content[0].Text, "db down") {
		t.Errorf("a failure: %+v", tr)
	}
}

// The owner's side: only a waiting request of their own campaign opens, and
// answering it is final.
func TestService_RequestAnswers(t *testing.T) {
	s, sess, _ := askRig(t)
	svc := s.Asks.(*Service)
	ctx := context.Background()
	q, err := svc.Ask(ctx, sess.Conn, "x", goodBlocks)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WaitingRequest(ctx, "camp-2", q.ID); err == nil {
		t.Error("another campaign opened the request")
	}
	got, err := svc.WaitingRequest(ctx, "camp-1", q.ID)
	if err != nil || got.Body != goodBlocks {
		t.Fatalf("got %+v, %v", got, err)
	}
	if err := svc.Answered(ctx, "camp-1", q.ID, "2 applied, 0 left out"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WaitingRequest(ctx, "camp-1", q.ID); err == nil {
		t.Error("an answered request opened again")
	}
	if err := svc.TurnDown(ctx, "camp-1", q.ID); err != nil {
		t.Fatal(err)
	}
	if w, _ := svc.Waiting(ctx, "camp-1"); len(w) != 0 {
		t.Errorf("still waiting: %+v", w)
	}
}

// The campaign check's problems go back to the AI, so it can fix its own
// request, and nothing waits for the owner. A check that fails only skips
// itself, since the owner's review checks everything again.
func TestAsk_CampaignCheck(t *testing.T) {
	tests := []struct {
		name     string
		problems []string
		err      error
		wantText string
		wantKept int
	}{
		{"problems refuse the request", []string{`block 1 (Coral Court): the Places category has no folder "Docks"`},
			nil, `Not sent: the owner couldn't use it: block 1 (Coral Court): the Places category has no folder "Docks".`, 0},
		{"many problems are cut short", []string{"a", "b", "c", "d", "e", "f", "g"}, nil, "a; b; c; d; e; and 2 more.", 0},
		{"no problems", nil, nil, "Sent to the owner for review", 1},
		{"a failed check is skipped", nil, errors.New("db down"), "Sent to the owner for review", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, sess, repo := askRig(t)
			var gotCampaign, gotChanges string
			s.Asks.(*Service).SetCheck(func(_ context.Context, campaignID, changes string) ([]string, error) {
				gotCampaign, gotChanges = campaignID, changes
				return tt.problems, tt.err
			})
			out := s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "Coral Court", "changes": goodBlocks}))
			tr := toolText(t, out)
			if len(tr.Content) == 0 || !strings.Contains(tr.Content[0].Text, tt.wantText) {
				t.Fatalf("reply %+v, want %q", tr, tt.wantText)
			}
			if tr.IsError != (tt.wantKept == 0) {
				t.Errorf("isError = %v", tr.IsError)
			}
			if len(repo.reqs) != tt.wantKept {
				t.Errorf("kept %d requests, want %d", len(repo.reqs), tt.wantKept)
			}
			if gotCampaign != "camp-1" || gotChanges != goodBlocks {
				t.Errorf("check saw campaign %q changes %q", gotCampaign, gotChanges)
			}
		})
	}
}

// A request of more blocks than one review should hold is refused before
// anything is checked against the campaign.
func TestAsk_TooManyBlocks(t *testing.T) {
	s, sess, repo := askRig(t)
	checked := false
	s.Asks.(*Service).SetCheck(func(context.Context, string, string) ([]string, error) { checked = true; return nil, nil })
	block := "---\nname: P%d\n---\n\nx\n\n"
	var b strings.Builder
	for i := 0; i <= blocksMax; i++ {
		fmt.Fprintf(&b, block, i)
	}
	tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "many", "changes": b.String()})))
	if !tr.IsError || !strings.Contains(tr.Content[0].Text, fmt.Sprintf("send at most %d", blocksMax)) || checked || len(repo.reqs) != 0 {
		t.Errorf("reply %+v, checked %v, kept %d", tr, checked, len(repo.reqs))
	}
}
