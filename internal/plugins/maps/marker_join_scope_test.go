package maps

import (
	"strings"
	"testing"
)

// The display join must stay inside the pin's own campaign: no DB-free test can
// run the query, so this pins the predicate in the shared JOIN text.
func TestMarkerJoins_EntityJoinScopedToCampaign(t *testing.T) {
	if !strings.Contains(markerJoins, "ent.campaign_id = src.campaign_id") {
		t.Fatalf("markerJoins must scope the entity join to the map's campaign:\n%s", markerJoins)
	}
}
