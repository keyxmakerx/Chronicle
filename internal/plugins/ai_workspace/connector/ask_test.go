package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
		if tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": fmt.Sprint(i), "changes": numbered(i)}))); tr.IsError {
			t.Fatalf("request %d refused: %+v", i+1, tr)
		}
	}
	tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "one more", "changes": numbered(requestsWaiting)})))
	if !tr.IsError || !strings.Contains(tr.Content[0].Text, "already waiting") {
		t.Errorf("reply = %+v", tr)
	}
	if len(repo.reqs) != requestsWaiting {
		t.Errorf("stored %d", len(repo.reqs))
	}
	// Answering one makes room.
	if err := s.Asks.(*Service).TurnDown(context.Background(), "camp-1", "owner-1", 1); err != nil {
		t.Fatal(err)
	}
	if tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "now", "changes": numbered(requestsWaiting + 1)}))); tr.IsError {
		t.Errorf("after one was answered: %+v", tr)
	}
}

// numbered is goodBlocks made distinct, so requests don't count as resends.
func numbered(i int) string { return goodBlocks + fmt.Sprintf("\n\nNet %d.", i) }

// A resend of changes still waiting stores nothing new and says so without
// an error, so the owner isn't left two copies and the AI doesn't retry.
func TestHandle_AskForChangesResendWhileWaiting(t *testing.T) {
	s, sess, repo := askRig(t)
	first := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "A market", "changes": goodBlocks})))
	if first.IsError {
		t.Fatalf("first = %+v", first)
	}
	out := s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "Resend: a market", "changes": goodBlocks}))
	tr := toolText(t, out)
	if tr.IsError || !strings.Contains(tr.Content[0].Text, "Already waiting") || !strings.Contains(tr.Content[0].Text, "don't send it again") {
		t.Errorf("resend reply = %+v", tr)
	}
	if len(repo.reqs) != 1 {
		t.Errorf("stored %d requests, want 1", len(repo.reqs))
	}
	if out.Called == nil || out.Called.Note != "already waiting, not sent again" || !out.Called.OK {
		t.Errorf("log line = %+v", out.Called)
	}
	// Once answered, the same changes may be asked for again.
	if err := s.Asks.(*Service).TurnDown(context.Background(), "camp-1", "owner-1", 1); err != nil {
		t.Fatal(err)
	}
	if tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "again", "changes": goodBlocks}))); tr.IsError || !strings.Contains(tr.Content[0].Text, "Sent to") {
		t.Errorf("after an answer: %+v", tr)
	}
	if len(repo.reqs) != 2 {
		t.Errorf("stored %d requests, want 2", len(repo.reqs))
	}
}

func TestHandle_HowToWriteChanges(t *testing.T) {
	s, sess, _ := askRig(t)
	var gotUser, gotCampaign string
	s.Instructions = func(_ context.Context, c *Connection, _ string) (string, error) {
		gotUser, gotCampaign = c.UserID, c.CampaignID
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
	s.Instructions = func(context.Context, *Connection, string) (string, error) { return "", errors.New("db down") }
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
	if _, err := svc.WaitingRequest(ctx, "camp-2", "owner-1", q.ID); err == nil {
		t.Error("another campaign opened the request")
	}
	got, err := svc.WaitingRequest(ctx, "camp-1", "owner-1", q.ID)
	if err != nil || got.Body != goodBlocks {
		t.Fatalf("got %+v, %v", got, err)
	}
	if err := svc.Answered(ctx, "camp-1", "owner-1", q.ID, "x"); err == nil {
		t.Error("a request nobody claimed was settled")
	}
	if _, err := svc.ClaimRequest(ctx, "camp-1", "owner-1", q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ClaimRequest(ctx, "camp-1", "owner-1", q.ID); err == nil {
		t.Error("a claimed request was claimed again")
	}
	if err := svc.Answered(ctx, "camp-1", "owner-1", q.ID, "2 applied, 0 left out"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WaitingRequest(ctx, "camp-1", "owner-1", q.ID); err == nil {
		t.Error("an answered request opened again")
	}
	// It was applied, so turning it down must not say nothing changed.
	if err := svc.TurnDown(ctx, "camp-1", "owner-1", q.ID); !isConflict(err) {
		t.Errorf("turn down after apply = %v, want a conflict", err)
	}
	if w, _ := svc.Waiting(ctx, "camp-1", "owner-1"); len(w) != 0 {
		t.Errorf("still waiting: %+v", w)
	}
}

func isConflict(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusConflict
}

// A commit that fails gives the request back, so it can be applied or turned
// down; a request still waiting turns down once.
func TestService_ReleaseAndTurnDown(t *testing.T) {
	s, sess, _ := askRig(t)
	svc := s.Asks.(*Service)
	ctx := context.Background()
	q, err := svc.Ask(ctx, sess.Conn, "x", goodBlocks)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ClaimRequest(ctx, "camp-1", "owner-1", q.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.TurnDown(ctx, "camp-1", "owner-1", q.ID); !isConflict(err) {
		t.Errorf("turn down while applying = %v, want a conflict", err)
	}
	if err := svc.ReleaseRequest(ctx, "camp-1", "owner-1", q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WaitingRequest(ctx, "camp-1", "owner-1", q.ID); err != nil {
		t.Errorf("released request not waiting: %v", err)
	}
	if err := svc.TurnDown(ctx, "camp-1", "owner-1", q.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.TurnDown(ctx, "camp-1", "owner-1", q.ID); !isConflict(err) {
		t.Errorf("second turn down = %v, want a conflict", err)
	}
}

// The campaign check's problems go back to the AI, so it can fix its own
// request, and nothing waits for the owner. A check that fails only skips
// itself, since the owner's review checks everything again.
func TestAsk_CampaignCheck(t *testing.T) {
	tests := []struct {
		name     string
		problems []string
		did      string
		err      error
		wantText string
		wantKept int
	}{
		{"problems refuse the request", []string{`block 1 (Coral Court): the Places category has no folder "Docks"`},
			"", nil, `Not sent: Chronicle couldn't use it: block 1 (Coral Court): the Places category has no folder "Docks".`, 0},
		{"many problems are cut short", []string{"a", "b", "c", "d", "e", "f", "g"}, "", nil, "a; b; c; d; e; and 2 more.", 0},
		{"a page problem names the page in the log", []string{"block 1 (Nobody Here): Cannot update"}, "Asked to change Nobody Here, refused: no such page", nil, "Cannot update.", 0},
		{"no problems", nil, "", nil, "Sent to the owner for review", 1},
		{"a failed check is skipped", nil, "", errors.New("db down"), "Sent to the owner for review", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, sess, repo := askRig(t)
			var gotCampaign, gotChanges string
			s.Asks.(*Service).SetCheck(func(_ context.Context, conn *Connection, changes string) ([]string, string, error) {
				campaignID := conn.CampaignID
				gotCampaign, gotChanges = campaignID, changes
				return tt.problems, tt.did, tt.err
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
			if tt.wantKept == 0 && len(tt.problems) > 0 {
				wantSummary := "Asked for changes, refused: “Coral Court”"
				if tt.did != "" {
					wantSummary = tt.did
				}
				if out.Called == nil || out.Called.OK || out.Called.Summary != wantSummary || !strings.Contains(out.Called.Note, tt.problems[0]) {
					t.Errorf("log line = %+v, want a refusal", out.Called)
				}
			}
		})
	}
}

// A request of more blocks than one review should hold is refused before
// anything is checked against the campaign.
func TestAsk_TooManyBlocks(t *testing.T) {
	s, sess, repo := askRig(t)
	checked := false
	s.Asks.(*Service).SetCheck(func(context.Context, *Connection, string) ([]string, string, error) {
		checked = true
		return nil, "", nil
	})
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
