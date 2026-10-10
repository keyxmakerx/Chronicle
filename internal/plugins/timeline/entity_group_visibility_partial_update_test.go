// entity_group_visibility_partial_update_test.go pins the partial-update
// contract for the two timeline inputs that used to write every column:
// entity-group edits (name/color) and event-link visibility.
package timeline

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestUpdateEntityGroup_PartialUpdate(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantName  string
		wantColor string
		wantErr   bool
	}{
		{"empty body preserves both", `{}`, "Heroes", "#112233", false},
		{"name only keeps color", `{"name":"Villains"}`, "Villains", "#112233", false},
		{"color only keeps name", `{"color":"#ffffff"}`, "Heroes", "#ffffff", false},
		{"explicit null on NOT NULL columns preserves", `{"name":null,"color":null}`, "Heroes", "#112233", false},
		{"blank name is refused, not stored", `{"name":""}`, "", "", true},
		{"bad color is refused", `{"color":"red"}`, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var written *EntityGroup
			repo := &mockTimelineRepo{
				listEntityGroupsFn: func(context.Context, string) ([]EntityGroup, error) {
					return []EntityGroup{
						{ID: 6, TimelineID: "tl-1", Name: "other", Color: "#000000", SortOrder: 1},
						{ID: 7, TimelineID: "tl-1", Name: "Heroes", Color: "#112233", SortOrder: 4},
					}, nil
				},
				updateEntityGroupFn: func(_ context.Context, g *EntityGroup) error { written = g; return nil },
			}
			var in UpdateEntityGroupInput
			if err := json.Unmarshal([]byte(tt.body), &in); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			err := newTestTimelineService(repo).UpdateEntityGroup(context.Background(), "tl-1", 7, in)
			if tt.wantErr {
				assertAppError(t, err, 422)
				if written != nil {
					t.Fatal("a refused update must not reach the repo")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if written.Name != tt.wantName || written.Color != tt.wantColor {
				t.Errorf("wrote %q/%q, want %q/%q", written.Name, written.Color, tt.wantName, tt.wantColor)
			}
			if written.SortOrder != 4 {
				t.Errorf("sort order = %d, want the stored 4 preserved", written.SortOrder)
			}
		})
	}
}

func TestUpdateEntityGroup_UnknownGroupIsNotFound(t *testing.T) {
	repo := &mockTimelineRepo{}
	err := newTestTimelineService(repo).UpdateEntityGroup(context.Background(), "tl-1", 99,
		UpdateEntityGroupInput{Name: patch.Of("x")})
	assertAppError(t, err, 404)
}

// TestUpdateEventLinkVisibility_PartialUpdate decodes real bodies and checks
// which fields reach the repo with which presence state.
func TestUpdateEventLinkVisibility_PartialUpdate(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantOvr      string // "absent", "null", or the value
		wantRules    string
		wantRepoCall bool
	}{
		{"override only leaves rules absent", `{"visibility_override":"dm_only"}`, "dm_only", "absent", true},
		{"rules only leaves override absent", `{"visibility_rules":"{\"allowed_users\":[\"u1\"]}"}`, "absent", `{"allowed_users":["u1"]}`, true},
		{"explicit nulls clear both", `{"visibility_override":null,"visibility_rules":null}`, "null", "null", true},
		{"empty override is a value, not absent", `{"visibility_override":""}`, "", "absent", true},
	}
	describe := func(f patch.Field[string]) string {
		switch {
		case !f.Present():
			return "absent"
		case f.IsNull():
			return "null"
		}
		v, _ := f.Get()
		return v
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotOvr, gotRules string
			called := false
			repo := &mockTimelineRepo{
				updateEventLinkVisFn: func(_ context.Context, _, _ string, o, r patch.Field[string]) error {
					called = true
					gotOvr, gotRules = describe(o), describe(r)
					return nil
				},
			}
			var in UpdateEventVisibilityInput
			if err := json.Unmarshal([]byte(tt.body), &in); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			if err := newTestTimelineService(repo).UpdateEventLinkVisibility(context.Background(), "tl-1", "evt-1", in); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if called != tt.wantRepoCall || gotOvr != tt.wantOvr || gotRules != tt.wantRules {
				t.Errorf("repo saw override=%q rules=%q (called=%v), want override=%q rules=%q", gotOvr, gotRules, called, tt.wantOvr, tt.wantRules)
			}
		})
	}
}
