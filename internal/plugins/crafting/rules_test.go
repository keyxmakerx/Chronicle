package crafting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The server keeps exactly what the rules editor keeps: every case in the
// shared fixture, whose answers come from craft_rules_logic.js itself.
func TestCleanRulesMatchesTheEditor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "js", "fixtures", "craft_rules_clean.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []struct {
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
			Want  json.RawMessage `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) == 0 {
		t.Fatal("the fixture has no cases")
	}
	for _, c := range fx.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, _ := json.Marshal(CleanRules(c.Input))
			var g, w any
			_ = json.Unmarshal(got, &g)
			_ = json.Unmarshal(c.Want, &w)
			if g, w = dropEmpty(g), dropEmpty(w); !reflect.DeepEqual(g, w) {
				gj, _ := json.Marshal(g)
				wj, _ := json.Marshal(w)
				t.Errorf("server kept\n%s\neditor keeps\n%s", gj, wj)
			}
		})
	}
}

// dropEmpty removes keys whose value is "": the editor keeps an empty name
// on a condition, the server's JSON leaves it out, and both mean the same.
func dropEmpty(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if s, ok := e.(string); ok && s == "" {
				continue
			}
			out[k] = dropEmpty(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = dropEmpty(e)
		}
		return out
	}
	return v
}

func TestCleanRulesUnreadable(t *testing.T) {
	for _, raw := range []string{"", "null", "{}", `"rules"`, "[", "42"} {
		if got := CleanRules(json.RawMessage(raw)); got == nil || len(got) != 0 {
			t.Errorf("CleanRules(%q) = %v, want an empty list", raw, got)
		}
	}
}

func TestCleanLooks(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]*Look
	}{
		{"unreadable", `[1]`, map[string]*Look{}},
		{"a mishap's moment", `{"mishap:3":{"spots":{"work":{"move":"shake"}},"words":"It cracks"},"mishap:20":{"spots":{}},"mishap:x":{"spots":{}}}`,
			map[string]*Look{"mishap:3": {Spots: map[string]LookSpot{"work": {Move: "shake"}}, Words: "It cracks"}}},
		{"known moments only", `{"success":{"spots":{"result":{"look":"bloom"}},"words":"It glows"},"party":{"spots":{}}}`,
			map[string]*Look{"success": {Spots: map[string]LookSpot{"result": {Look: "bloom"}}, Words: "It glows"}}},
		{"unknown looks and moves dropped", `{"failure":{"spots":{"heat":{"look":"lava","move":"dim"},"work":{"look":"lava"}}}}`,
			map[string]*Look{"failure": {Spots: map[string]LookSpot{"heat": {Move: "dim"}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanLooks(json.RawMessage(tt.raw)); !reflect.DeepEqual(got, tt.want) {
				gj, _ := json.Marshal(got)
				t.Errorf("got %s", gj)
			}
		})
	}
}
