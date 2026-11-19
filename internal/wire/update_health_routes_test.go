package wire

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/updatehealth"
)

// TestUpdateHealthRoutesExist keeps the changed-features list honest: a route
// that matches nothing would leave its feature at zero uses and send a site
// admin to try something that works.
func TestUpdateHealthRoutesExist(t *testing.T) {
	records := buildRouteRecords(t)
	rec := updatehealth.NewRecorder(updatehealth.Changed)
	for i, f := range updatehealth.Changed {
		found := false
		for _, r := range records {
			for _, m := range rec.Match(r.Method, r.Path) {
				if m == i {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("changed feature %q matches no registered route: %v", f.ID, f.Routes)
		}
	}
}
