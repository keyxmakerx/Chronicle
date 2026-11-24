package connector

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// withdrawRig is askRig with one request already waiting.
func withdrawRig(t *testing.T) (*Server, Session, *memRepo) {
	t.Helper()
	s, sess, repo := askRig(t)
	if tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolAsk, map[string]any{"summary": "A market", "changes": goodBlocks}))); tr.IsError {
		t.Fatalf("ask: %+v", tr)
	}
	if !strings.Contains(repo.reqs[0].Summary, "A market") {
		t.Fatalf("request = %+v", repo.reqs[0])
	}
	return s, sess, repo
}

// The AI takes back its own waiting request; the reason is kept as the
// outcome and on the log line, so the owner reads why.
func TestHandle_WithdrawRequest(t *testing.T) {
	s, sess, repo := withdrawRig(t)
	id := repo.reqs[0].ID
	out := s.Handle(context.Background(), sess, callBody(toolWithdraw, map[string]any{
		"request": "#" + itoa(id), "reason": "Coral Court already exists under another name"}))
	tr := toolText(t, out)
	if tr.IsError || !strings.Contains(tr.Content[0].Text, "Withdrawn") || !strings.Contains(tr.Content[0].Text, "nothing in the campaign changed") {
		t.Fatalf("reply = %+v", tr)
	}
	q := repo.reqs[0]
	if q.Status != RequestWithdrawn || q.Outcome != "Coral Court already exists under another name" || q.DecidedAt == nil {
		t.Errorf("request = %+v", q)
	}
	if out.Called == nil || !out.Called.OK || out.Called.Summary != "Withdrew its request “A market”" || !strings.Contains(out.Called.Note, "Coral Court already exists") {
		t.Errorf("log line = %+v", out.Called)
	}
	// The review screen says why, rather than "you already answered".
	if _, err := s.Asks.(*Service).WaitingRequest(context.Background(), "camp-1", "owner-1", id); err == nil || !strings.Contains(err.Error(), "withdrew that request: Coral Court") {
		t.Errorf("review error = %v", err)
	}
}

func TestHandle_WithdrawRequestRefusals(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*memRepo)
		args   func(id int64) map[string]any
		want   string
		status string
	}{
		{"no number", nil, func(int64) map[string]any { return map[string]any{"request": "the market", "reason": "x"} }, "request's number", RequestWaiting},
		{"blank reason", nil, func(id int64) map[string]any { return map[string]any{"request": itoa(id), "reason": "   "} }, "needs reason", RequestWaiting},
		{"missing reason", nil, func(id int64) map[string]any { return map[string]any{"request": itoa(id)} }, "reason", RequestWaiting},
		{"unknown request", nil, func(id int64) map[string]any { return map[string]any{"request": itoa(id + 40), "reason": "x"} }, "no request", RequestWaiting},
		{"answered", func(r *memRepo) { r.reqs[0].Status = RequestAnswered }, func(id int64) map[string]any { return map[string]any{"request": itoa(id), "reason": "x"} }, "already answered", RequestAnswered},
		{"turned down", func(r *memRepo) { r.reqs[0].Status = RequestTurnedDown }, func(id int64) map[string]any { return map[string]any{"request": itoa(id), "reason": "x"} }, "already answered", RequestTurnedDown},
		{"being applied", func(r *memRepo) { r.reqs[0].Status = RequestApplying }, func(id int64) map[string]any { return map[string]any{"request": itoa(id), "reason": "x"} }, "applying", RequestApplying},
		// Another person's AI asked for it: invisible to this key, so it reads
		// as not found and is never touched.
		{"someone else's", func(r *memRepo) { r.conns[0].UserID = "player-9" }, func(id int64) map[string]any { return map[string]any{"request": itoa(id), "reason": "x"} }, "no request", RequestWaiting},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, sess, repo := withdrawRig(t)
			if tt.setup != nil {
				tt.setup(repo)
			}
			out := s.Handle(context.Background(), sess, callBody(toolWithdraw, tt.args(repo.reqs[0].ID)))
			tr := toolText(t, out)
			if !tr.IsError || !strings.Contains(tr.Content[0].Text, tt.want) {
				t.Fatalf("reply = %+v, want an error with %q", tr, tt.want)
			}
			if repo.reqs[0].Status != tt.status {
				t.Errorf("status = %q, want %q", repo.reqs[0].Status, tt.status)
			}
		})
	}
}

// A look-only connection has no withdraw tool at all.
func TestHandle_WithdrawNeedsAskingConnection(t *testing.T) {
	s, sess, _ := withdrawRig(t)
	conn := *sess.Conn
	conn.CanAsk = false
	sess.Conn = &conn
	out := s.Handle(context.Background(), sess, callBody(toolWithdraw, map[string]any{"request": "1", "reason": "x"}))
	if r := decode(t, out); r.Error == nil {
		t.Fatalf("a look-only key could call %s", toolWithdraw)
	}
}

func TestRequestNumber(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"12", 12, true}, {" #12 ", 12, true}, {"Request 12", 12, true}, {"request #3", 3, true},
		{"0", 0, false}, {"-4", 0, false}, {"twelve", 0, false}, {"", 0, false},
	}
	for _, tt := range tests {
		if n, ok := requestNumber(tt.in); n != tt.want && tt.ok || ok != tt.ok {
			t.Errorf("requestNumber(%q) = %d, %v", tt.in, n, ok)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
