package connector

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// windowWaiting records the window the lookup asked for.
type windowWaiting struct{ older *int }

func (w windowWaiting) Waiting(_ context.Context, _ string, _ records.Actor, older int) (records.WaitingList, error) {
	*w.older = older
	return records.WaitingList{}, nil
}

// waiting_for_review takes `older` (a number or its text), passes it to the
// lookup and says how to page; home takes `audience`; an owner's key lists
// both with their argument and a tool never takes one it didn't declare.
func TestTools_WaitingOlderAndHomeAudience(t *testing.T) {
	for _, tc := range []struct{ tool, arg string }{{"waiting_for_review", "older"}, {"home", "audience"}} {
		tl := findTool(&Connection{Privacy: aiexport.PrivacyModePermitted}, nil, tc.tool)
		if tl == nil || !tl.takes(tc.arg) {
			t.Errorf("%s does not take %s", tc.tool, tc.arg)
		}
	}
	got := -1
	s := &Server{Lookups: &records.Lookups{Waiting: windowWaiting{&got}}}
	for _, arg := range []string{`7`, `"7"`} {
		got = -1
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"waiting_for_review","arguments":{"older":` + arg + `}}}`
		out := s.Handle(context.Background(), session(aiexport.PrivacyModePermitted), []byte(body))
		var res struct {
			Content []struct{ Text string } `json:"content"`
			IsError bool                    `json:"isError"`
		}
		if err := json.Unmarshal(decode(t, out).Result, &res); err != nil {
			t.Fatal(err)
		}
		if res.IsError || got != 7 || !strings.Contains(res.Content[0].Text, "from 7 to 14 days ago") {
			t.Errorf("older %s: asked %d, error %v, text %q", arg, got, res.IsError, res.Content)
		}
	}
	out := s.Handle(context.Background(), session(aiexport.PrivacyModePermitted),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"waiting_for_review","arguments":{"days":"7"}}}`))
	if !strings.Contains(string(out.Body), "doesn't take") {
		t.Errorf("an argument the tool never declared was accepted: %s", out.Body)
	}
}
