package entities

import (
	"reflect"
	"testing"
)

func TestMergeFieldPatch(t *testing.T) {
	cases := []struct {
		name    string
		current map[string]any
		patch   map[string]any
		want    map[string]any
	}{
		{"absent keeps", map[string]any{"hp": 10, "lore": "x"}, map[string]any{"hp": 7}, map[string]any{"hp": 7, "lore": "x"}},
		{"null clears", map[string]any{"hp": 10, "lore": "x"}, map[string]any{"lore": nil}, map[string]any{"hp": 10}},
		{"new key added", map[string]any{"hp": 10}, map[string]any{"ac": 15}, map[string]any{"hp": 10, "ac": 15}},
		{"nil current", nil, map[string]any{"hp": 1}, map[string]any{"hp": 1}},
		{"empty patch keeps all", map[string]any{"hp": 10}, nil, map[string]any{"hp": 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := map[string]any{}
			for k, v := range tc.current {
				before[k] = v
			}
			got := mergeFieldPatch(tc.current, tc.patch)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
			if len(tc.current) > 0 && !reflect.DeepEqual(tc.current, before) {
				t.Errorf("current was modified: %v", tc.current)
			}
		})
	}
}
