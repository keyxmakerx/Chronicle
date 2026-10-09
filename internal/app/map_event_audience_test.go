// map_event_audience_test.go pins that mapEventPublisherAdapter's
// PublishMarkerEvent/PublishDrawingEvent compute the WebSocket audience from
// a marker/drawing's VisibilityRules, not just its dm_only bit, by asserting
// what lands on the wire-bound Message (AllowedUsers/DeniedUsers/RequiresDM).
//
// Companion to internal/websocket/hub_visibility_test.go, which proves the
// real Hub enforces those fields once set but constructs the Message by
// hand; this test proves the adapter actually sets them from a real
// *maps.Marker / *maps.Drawing. It does not cover whether the hub's gate
// itself is correct — that is hub_visibility_test.go's job.
package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

func strPtr(s string) *string { return &s }

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPublishMarkerEvent_ComputesAudienceFromVisibilityRules(t *testing.T) {
	cases := []struct {
		name           string
		marker         *maps.Marker
		wantRequiresDM bool
		wantAllowed    []string
		wantDenied     []string
	}{
		{
			name:   "everyone, no rules — full audience, no restriction",
			marker: &maps.Marker{ID: "m1", Visibility: "everyone"},
		},
		{
			name:           "dm_only — RequiresDM set, rules irrelevant",
			marker:         &maps.Marker{ID: "m2", Visibility: "dm_only"},
			wantRequiresDM: true,
		},
		{
			name:       "everyone with an explicit deny",
			marker:     &maps.Marker{ID: "m3", Visibility: "everyone", VisibilityRules: strPtr(`{"denied_users":["user-denied"]}`)},
			wantDenied: []string{"user-denied"},
		},
		{
			name:        "specific with an allow-list",
			marker:      &maps.Marker{ID: "m4", Visibility: "specific", VisibilityRules: strPtr(`{"allowed_users":["user-a"]}`)},
			wantAllowed: []string{"user-a"},
		},
		{
			name:        "both lists set at once",
			marker:      &maps.Marker{ID: "m5", Visibility: "specific", VisibilityRules: strPtr(`{"allowed_users":["user-a"],"denied_users":["user-b"]}`)},
			wantAllowed: []string{"user-a"},
			wantDenied:  []string{"user-b"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := &captureBus{}
			a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}}
			a.PublishMarkerEvent("updated", "camp-1", tc.marker)

			if bus.last == nil {
				t.Fatal("expected Publish to be called")
			}
			if bus.last.RequiresDM != tc.wantRequiresDM {
				t.Errorf("RequiresDM = %v, want %v", bus.last.RequiresDM, tc.wantRequiresDM)
			}
			if !stringSlicesEqual(bus.last.AllowedUsers, tc.wantAllowed) {
				t.Errorf("AllowedUsers = %v, want %v", bus.last.AllowedUsers, tc.wantAllowed)
			}
			if !stringSlicesEqual(bus.last.DeniedUsers, tc.wantDenied) {
				t.Errorf("DeniedUsers = %v, want %v", bus.last.DeniedUsers, tc.wantDenied)
			}
		})
	}
}

// TestPublishDrawingEvent_ComputesAudienceFromVisibilityRules is the
// drawing-side twin, covering Drawing.VisibilityRules (maps/drawing.go).
func TestPublishDrawingEvent_ComputesAudienceFromVisibilityRules(t *testing.T) {
	cases := []struct {
		name           string
		drawing        *maps.Drawing
		wantRequiresDM bool
		wantAllowed    []string
		wantDenied     []string
	}{
		{
			name:    "everyone, no rules — full audience, no restriction",
			drawing: &maps.Drawing{ID: "d1", Visibility: "everyone"},
		},
		{
			name:           "dm_only — RequiresDM set, rules irrelevant",
			drawing:        &maps.Drawing{ID: "d2", Visibility: "dm_only"},
			wantRequiresDM: true,
		},
		{
			name:           "a picture hidden from players is staff-only on the wire",
			drawing:        &maps.Drawing{ID: "pic", DrawingType: "image", Visibility: "dm_only", ImageID: strPtr("media-1")},
			wantRequiresDM: true,
		},
		{
			name:    "a visible picture reaches everyone",
			drawing: &maps.Drawing{ID: "pic2", DrawingType: "image", Visibility: "everyone", ImageID: strPtr("media-1")},
		},
		{
			name:       "everyone with an explicit deny",
			drawing:    &maps.Drawing{ID: "d3", Visibility: "everyone", VisibilityRules: strPtr(`{"denied_users":["user-denied"]}`)},
			wantDenied: []string{"user-denied"},
		},
		{
			name:        "specific with an allow-list",
			drawing:     &maps.Drawing{ID: "d4", Visibility: "specific", VisibilityRules: strPtr(`{"allowed_users":["user-a"]}`)},
			wantAllowed: []string{"user-a"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := &captureBus{}
			a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}}
			a.PublishDrawingEvent("updated", "camp-1", tc.drawing)

			if bus.last == nil {
				t.Fatal("expected Publish to be called")
			}
			if bus.last.RequiresDM != tc.wantRequiresDM {
				t.Errorf("RequiresDM = %v, want %v", bus.last.RequiresDM, tc.wantRequiresDM)
			}
			if !stringSlicesEqual(bus.last.AllowedUsers, tc.wantAllowed) {
				t.Errorf("AllowedUsers = %v, want %v", bus.last.AllowedUsers, tc.wantAllowed)
			}
			if !stringSlicesEqual(bus.last.DeniedUsers, tc.wantDenied) {
				t.Errorf("DeniedUsers = %v, want %v", bus.last.DeniedUsers, tc.wantDenied)
			}
		})
	}
}

// TestPublishTokenEvent_NeverCarriesAudienceLists guards the "rules is nil
// for source kinds that don't carry per-user overrides" branch of
// publishWithAudience: tokens have no VisibilityRules concept (only the
// binary IsHidden), so a token event must never end up with a spurious
// AllowedUsers/DeniedUsers list.
func TestPublishTokenEvent_NeverCarriesAudienceLists(t *testing.T) {
	bus := &captureBus{}
	a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}}
	a.PublishTokenEvent("updated", "camp-1", &maps.Token{ID: "t1", IsHidden: true})

	if bus.last == nil {
		t.Fatal("expected Publish to be called")
	}
	if !bus.last.RequiresDM {
		t.Error("hidden token should be RequiresDM=true")
	}
	if len(bus.last.AllowedUsers) != 0 || len(bus.last.DeniedUsers) != 0 {
		t.Errorf("token events should never carry audience lists; got allowed=%v denied=%v", bus.last.AllowedUsers, bus.last.DeniedUsers)
	}
}
