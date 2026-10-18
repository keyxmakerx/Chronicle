package timeline

import (
	"strings"
	"testing"
)

// The display join must stay inside the timeline's own campaign: no DB-free
// test can run the query, so this pins the predicate in the shared JOIN text.
func TestStandaloneEventJoins_EntityJoinScopedToCampaign(t *testing.T) {
	if !strings.Contains(standaloneEventJoins, "ent.campaign_id = tl.campaign_id") {
		t.Fatalf("standaloneEventJoins must scope the entity join to the timeline's campaign:\n%s", standaloneEventJoins)
	}
}
