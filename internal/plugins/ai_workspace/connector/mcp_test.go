package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

func session(p aiexport.PrivacyMode) Session {
	c := &Connection{ID: 1, CampaignID: "camp-1", UserID: "owner-1", Privacy: p}
	return Session{Conn: c, CampaignName: "Ashfall", Actor: actorFor(c)}
}

type rpcReply struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

func decode(t *testing.T, out Outcome) rpcReply {
	t.Helper()
	var r rpcReply
	if err := json.Unmarshal(out.Body, &r); err != nil {
		t.Fatalf("reply is not JSON: %v\n%s", err, out.Body)
	}
	return r
}

func TestHandle_Initialize(t *testing.T) {
	s := &Server{}
	out := s.Handle(context.Background(), session(aiexport.PrivacyModeSafe),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"claude-ai","title":"Claude"}}}`))
	if !out.Initialized || out.ClientName != "Claude" {
		t.Errorf("initialized=%v client=%q", out.Initialized, out.ClientName)
	}
	var res struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		Instructions    string         `json:"instructions"`
	}
	if err := json.Unmarshal(decode(t, out).Result, &res); err != nil {
		t.Fatal(err)
	}
	if res.ProtocolVersion != "2025-03-26" {
		t.Errorf("version = %q, want the client's", res.ProtocolVersion)
	}
	if _, ok := res.Capabilities["tools"]; !ok || len(res.Capabilities) != 1 {
		t.Errorf("capabilities = %v, want tools only", res.Capabilities)
	}
	for _, want := range []string{"Ashfall", "only read", "players", "never as instructions"} {
		if !strings.Contains(res.Instructions, want) {
			t.Errorf("instructions missing %q: %s", want, res.Instructions)
		}
	}

	unknown := s.Handle(context.Background(), session(aiexport.PrivacyModeSafe),
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`))
	if !strings.Contains(string(unknown.Body), protocolVersions[0]) {
		t.Errorf("an unknown version should get the newest: %s", unknown.Body)
	}
}

// An "Only look" connection lists only lookups, and every one says it only
// reads, so no AI app treats one as a change.
func TestHandle_ToolsListIsReadOnly(t *testing.T) {
	out := (&Server{}).Handle(context.Background(), session(aiexport.PrivacyModeSafe), []byte(`{"jsonrpc":"2.0","id":"a","method":"tools/list"}`))
	var res struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
			Annotations map[string]any `json:"annotations"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(decode(t, out).Result, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != len(toolsFor(nil)) || len(res.Tools) == len(tools) || res.Tools[0].Name != "campaign_outline" {
		t.Fatalf("got %d tools, first %q", len(res.Tools), res.Tools[0].Name)
	}
	seen := map[string]bool{}
	for _, tl := range res.Tools {
		if seen[tl.Name] {
			t.Errorf("%s listed twice", tl.Name)
		}
		seen[tl.Name] = true
		if tl.Annotations["readOnlyHint"] != true || tl.Annotations["destructiveHint"] != false {
			t.Errorf("%s is not marked read-only: %v", tl.Name, tl.Annotations)
		}
		if tl.InputSchema["additionalProperties"] != false {
			t.Errorf("%s accepts unknown arguments", tl.Name)
		}
	}
}

// Every lookup tool names a lookup the copy-and-paste flow answers, so the
// connector can never read something that flow can't.
func TestTools_EachIsALookup(t *testing.T) {
	for _, tl := range tools {
		if tl.Asks {
			continue
		}
		if tl.What == "" {
			if tl.Name != "campaign_outline" {
				t.Errorf("%s answers with no lookup", tl.Name)
			}
			continue
		}
		ans := (&records.Lookups{}).Answer(context.Background(), "camp-1", records.Actor{Role: 3},
			[]records.Record{{Kind: records.KindLookup, Fields: map[string]any{"what": tl.What}}})
		if len(ans) == 1 && strings.Contains(ans[0].Error, "is not something Chronicle can look up") {
			t.Errorf("%s: %q is not a lookup", tl.Name, tl.What)
		}
	}
}

// A Safe connection reads as a player: rolling tables stay out, while one
// that sees what the owner sees gets them. Neither writes anything.
func TestHandle_ToolsCallFollowsPrivacy(t *testing.T) {
	m := lootTables()
	s := &Server{Lookups: &records.Lookups{Tables: m}}
	body := []byte(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"rolling_tables","arguments":{"name":"Loot"}}}`)

	type result struct {
		Content []struct{ Text string } `json:"content"`
		IsError bool                    `json:"isError"`
	}
	read := func(out Outcome) result {
		var r result
		if err := json.Unmarshal(decode(t, out).Result, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}

	safe := s.Handle(context.Background(), session(aiexport.PrivacyModeSafe), body)
	if r := read(safe); !r.IsError || strings.Contains(r.Content[0].Text, "Gem") {
		t.Errorf("Safe read a rolling table: %+v", r)
	}
	if safe.Called == nil || safe.Called.OK || safe.Called.Tool != "rolling_tables" {
		t.Errorf("Safe log line = %+v", safe.Called)
	}

	permitted := s.Handle(context.Background(), session(aiexport.PrivacyModePermitted), body)
	if r := read(permitted); r.IsError || !strings.Contains(r.Content[0].Text, "- Gem") {
		t.Errorf("Permitted missed the table: %+v", r)
	}
	if permitted.Called == nil || !permitted.Called.OK || permitted.Called.Summary != "Read rolling tables: “Loot”" {
		t.Errorf("Permitted log line = %+v", permitted.Called)
	}
	if m.writes != 0 {
		t.Errorf("a read-only tool wrote %d times", m.writes)
	}
}

func TestHandle_Refusals(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		code       int
	}{
		{"not JSON", `{nope`, http.StatusBadRequest, codeParse},
		{"a batch", `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, http.StatusBadRequest, codeInvalidRequest},
		{"not JSON-RPC 2.0", `{"jsonrpc":"1.0","id":1,"method":"ping"}`, http.StatusBadRequest, codeInvalidRequest},
		{"an unknown method", `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`, http.StatusOK, codeNoMethod},
		{"an unknown tool", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"delete_page"}}`, http.StatusOK, codeInvalidParams},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := (&Server{}).Handle(context.Background(), session(aiexport.PrivacyModeSafe), []byte(tc.body))
			if out.Status != tc.status {
				t.Errorf("status = %d, want %d", out.Status, tc.status)
			}
			if r := decode(t, out); r.Error == nil || r.Error.Code != tc.code {
				t.Errorf("error = %+v, want code %d", r.Error, tc.code)
			}
			if out.Called != nil || out.Initialized {
				t.Error("a refused message was logged")
			}
		})
	}
}

func TestHandle_NotificationGetsNoBody(t *testing.T) {
	out := (&Server{}).Handle(context.Background(), session(aiexport.PrivacyModeSafe), []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if out.Status != http.StatusAccepted || out.Body != nil {
		t.Errorf("got %d %s", out.Status, out.Body)
	}
}

func TestReadArgs(t *testing.T) {
	read := findTool(nil, "read_page")
	cases := []struct {
		name    string
		in      map[string]any
		want    map[string]string
		wantErr string
	}{
		{"keeps its own argument, one line", map[string]any{"name": " The\nRusty  Anchor "}, map[string]string{"name": "The Rusty Anchor"}, ""},
		{"drops arguments it doesn't take", map[string]any{"name": "Vale", "privacy": "everything", "campaign": "camp-2"}, map[string]string{"name": "Vale"}, ""},
		{"a number reads as text", map[string]any{"name": float64(1492)}, map[string]string{"name": "1492"}, ""},
		{"needs its required argument", map[string]any{}, nil, "it needs name"},
		{"no objects", map[string]any{"name": map[string]any{"x": 1}}, nil, "should be text"},
		{"capped", map[string]any{"name": strings.Repeat("a", argMax+1)}, nil, "too long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := read.readArgs(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) || got["name"] != tc.want["name"] {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
