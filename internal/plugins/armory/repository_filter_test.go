package armory

import (
	"net/url"
	"strings"
	"testing"
)

// A type filter may only narrow the item types; an id outside them must match
// nothing rather than replace the item-type restriction.
func TestItemIDsWhereAndJoins_TypeFilter(t *testing.T) {
	tests := []struct {
		name      string
		typeIDs   []int
		opts      ItemListOptions
		wantSQL   string
		wantNoSQL string
		wantArgs  []any
	}{
		{"no type filter uses all item types", []int{1, 2}, ItemListOptions{}, "e.entity_type_id IN (?,?)", "1 = 0", []any{"c1", 1, 2}},
		{"item type narrows", []int{1, 2}, ItemListOptions{TypeID: 2}, "e.entity_type_id = ?", "1 = 0", []any{"c1", 2}},
		{"non-item type matches nothing", []int{1, 2}, ItemListOptions{TypeID: 99}, "1 = 0", "entity_type_id", []any{"c1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, where, args := itemIDsWhereAndJoins("c1", tc.typeIDs, tc.opts)
			if !strings.Contains(where, tc.wantSQL) {
				t.Errorf("where = %q, want it to contain %q", where, tc.wantSQL)
			}
			if strings.Contains(where, tc.wantNoSQL) {
				t.Errorf("where = %q, must not contain %q", where, tc.wantNoSQL)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tc.wantArgs)
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("args[%d] = %v, want %v", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}

// Pagination links must escape free-text filters and carry the collection.
func TestArmoryPageURL(t *testing.T) {
	tests := []struct {
		name string
		opts ItemListOptions
		want url.Values
	}{
		{"plain", ItemListOptions{Sort: "name"}, url.Values{"page": {"2"}, "sort": {"name"}}},
		{"special characters survive", ItemListOptions{Sort: "name", Search: "a&b=c #1", Tag: "x&y"},
			url.Values{"page": {"2"}, "sort": {"name"}, "q": {"a&b=c #1"}, "tag": {"x&y"}}},
		{"instance and type", ItemListOptions{Sort: "updated", TypeID: 4, InstanceID: 7},
			url.Values{"page": {"2"}, "sort": {"updated"}, "type": {"4"}, "instance": {"7"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := armoryPageURL("camp", 2, tc.opts)
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("parse %q: %v", raw, err)
			}
			got := u.Query()
			if len(got) != len(tc.want) {
				t.Fatalf("query = %v, want %v (url %q)", got, tc.want, raw)
			}
			for k, v := range tc.want {
				if got.Get(k) != v[0] {
					t.Errorf("param %s = %q, want %q (url %q)", k, got.Get(k), v[0], raw)
				}
			}
		})
	}
}
