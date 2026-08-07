// pagination_total_order_test.go — C-SWEEP-R3 / backend/entity-pagination-
// nondeterministic-order.
//
// Every LIMIT ? OFFSET ? query in this plugin must sort on a total order.
// The leading sort columns (name, updated_at, created_at, sort_order) are all
// non-unique — only uq_entities_campaign_slug is — so tied rows come back in
// whatever order the chosen plan emits, and that choice is not stable across
// the statements of one walk. Measured on MariaDB 10.11 with 50,000 entities
// sharing one updated_at: page-by-page `ORDER BY e.updated_at DESC LIMIT 100
// OFFSET n` returned 563 entities twice and 563 not at all (the server used a
// priority-queue sort while LIMIT+OFFSET was small and a full filesort once it
// was not, and the two disagree about the tied window). Appending the primary
// key drove that to 0 dup / 0 miss.
//
// Both tests below are source/pure-unit level so they run under `-short` with
// no database — an integration test would need a 50k-row seed and a MariaDB.

package entities

import (
	"os"
	"strings"
	"testing"
)

// TestListOptions_OrderByClause_TotalOrder pins that every sort the list page
// exposes (?sort=name|updated|created|manual, plus the unknown/empty default)
// ends with the unique `e.id` tiebreaker.
func TestListOptions_OrderByClause_TotalOrder(t *testing.T) {
	tests := []struct {
		sort string
		want string
	}{
		{"name", "ORDER BY e.name ASC, e.id ASC"},
		{"updated", "ORDER BY e.updated_at DESC, e.id ASC"},
		{"created", "ORDER BY e.created_at DESC, e.id ASC"},
		{"manual", "ORDER BY e.sort_order ASC, e.name ASC, e.id ASC"},
		{"", "ORDER BY e.name ASC, e.id ASC"},
		{"bogus", "ORDER BY e.name ASC, e.id ASC"},
	}
	for _, tt := range tests {
		t.Run(tt.sort, func(t *testing.T) {
			got := ListOptions{Sort: tt.sort}.OrderByClause()
			if got != tt.want {
				t.Errorf("OrderByClause() = %q, want %q", got, tt.want)
			}
			// Restate the invariant the exact-match above encodes, so a future
			// edit that changes the leading columns still has to keep the
			// tiebreaker rather than just re-baselining the string.
			if !strings.HasSuffix(got, "e.id ASC") {
				t.Errorf("OrderByClause() = %q: OFFSET paging needs a unique final sort key (e.id)", got)
			}
		})
	}
}

// TestPaginatedQueries_HaveUniqueTiebreaker is a structural pin over
// repository.go: every `LIMIT ? OFFSET ?` query must either delegate its
// ordering to OrderByClause (pinned above) or carry `e.id` in its own literal
// ORDER BY. A new paginated read that forgets the tiebreaker fails here.
func TestPaginatedQueries_HaveUniqueTiebreaker(t *testing.T) {
	data, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatalf("read repository.go: %v", err)
	}
	src := string(data)

	const marker = "LIMIT ? OFFSET ?"
	found := 0
	for cursor := 0; ; {
		rel := strings.Index(src[cursor:], marker)
		if rel < 0 {
			break
		}
		pos := cursor + rel
		cursor = pos + len(marker)
		found++

		// Window: from the SELECT that opens this query through a little past
		// the marker, so a clause passed as an fmt.Sprintf argument after the
		// format string (ListByCampaign's opts.OrderByClause()) is included.
		start := strings.LastIndex(src[:pos], "SELECT ")
		if start < 0 {
			t.Fatalf("paginated query at byte %d has no preceding SELECT", pos)
		}
		end := pos + len(marker) + 200
		if end > len(src) {
			end = len(src)
		}
		window := src[start:end]

		if strings.Contains(window, "OrderByClause()") {
			continue // ordering pinned by TestListOptions_OrderByClause_TotalOrder
		}
		ob := strings.LastIndex(window[:pos-start], "ORDER BY")
		if ob < 0 {
			t.Errorf("paginated query at byte %d has no ORDER BY: OFFSET paging over an unordered result duplicates and skips rows", pos)
			continue
		}
		if !strings.Contains(window[ob:pos-start], "e.id") {
			t.Errorf("paginated query at byte %d orders by %q with no unique tiebreaker: append e.id",
				pos, strings.TrimSpace(window[ob:pos-start]))
		}
	}

	// Guard the guard: if the paginated queries move or get renamed, this test
	// must not quietly pass by matching nothing.
	if found < 2 {
		t.Fatalf("expected at least 2 %q queries in repository.go, found %d", marker, found)
	}
}
