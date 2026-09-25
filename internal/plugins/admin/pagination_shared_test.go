// pagination_shared_test.go pins that the Users and Campaigns admin lists
// use the same reusable components.Pagination control every other list
// page uses, instead of a hand-copied Previous/Next block. The only
// visible difference is the disabled Previous button: the hand-rolled
// version stacked a 50% opacity fade on top of the normal disabled grey
// (text-fg-muted + opacity-50); the shared control uses a single disabled
// tone (text-fg-faint) with no fade stacked on.

package admin

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestAdminUsersPage_UsesSharedPagination pins the Users list to the shared
// pagination control on page 1 of many (disabled Previous, active Next).
func TestAdminUsersPage_UsesSharedPagination(t *testing.T) {
	users := []auth.User{{ID: "u1", Email: "a@example.com", DisplayName: "A"}}
	component := AdminUsersPage(users, 50, 1, 10, "csrf")

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "Page 1 of 5") {
		t.Errorf("expected page info from the shared component; got: %s", html)
	}
	if !strings.Contains(html, `text-fg-faint cursor-not-allowed">Previous`) {
		t.Errorf("disabled Previous must use the shared component's single disabled tone (text-fg-faint), no stacked opacity fade; got: %s", html)
	}
	if strings.Contains(html, "opacity-50") {
		t.Errorf("the hand-rolled opacity-50 fade must be gone now that pagination is shared; got: %s", html)
	}
}

// TestAdminCampaignsPage_UsesSharedPagination pins the same contract for
// the Campaigns list.
func TestAdminCampaignsPage_UsesSharedPagination(t *testing.T) {
	list := []campaigns.Campaign{{ID: "c1", Name: "Ashenmoor", Slug: "ashenmoor"}}
	component := AdminCampaignsPage(list, 50, 1, 10, "csrf")

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "Page 1 of 5") {
		t.Errorf("expected page info from the shared component; got: %s", html)
	}
	if !strings.Contains(html, `text-fg-faint cursor-not-allowed">Previous`) {
		t.Errorf("disabled Previous must use the shared component's single disabled tone (text-fg-faint), no stacked opacity fade; got: %s", html)
	}
	if strings.Contains(html, "opacity-50") {
		t.Errorf("the hand-rolled opacity-50 fade must be gone now that pagination is shared; got: %s", html)
	}
}
