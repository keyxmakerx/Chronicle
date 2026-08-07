// sync_pull_cursor_test.go is the regression for the unreachable-tail bug in
// POST /api/v1/campaigns/:id/sync (sweep R4 stage 18).
//
// Fix id: backend/syncapi-pull-1000-cap-no-cursor.
//
// The pull walked the campaign's entity list internally, page 1 to
// syncMaxPullPages, and stopped. `has_more` reported the truth, but nothing
// in the request could act on it: `since` is a FILTER over the list, not a
// position in it, so the next request re-walked the same first
// syncMaxPullPages*syncPageSize entities. Any entity past that point in list
// order could never reach the VTT — not slowly, not eventually, never.
//
// The fix keeps the cap as a page size and adds a cursor. What is pinned
// here: the tail is reachable in a bounded number of requests, the cursor is
// only emitted when there is more, and a bad cursor is refused rather than
// silently restarting the walk (a silent restart looks exactly like success
// while the tail stays unreachable — the same lie in a new place).
package syncapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// bigCampaignEntityService serves a campaign of `total` entities, all
// modified recently, paginated the way the real service does.
type bigCampaignEntityService struct {
	entities.EntityService
	total int
}

func (s *bigCampaignEntityService) List(_ context.Context, campaignID string, _ int, _ int, _ string, opts entities.ListOptions) ([]entities.Entity, int, error) {
	start := (opts.Page - 1) * opts.PerPage
	if start >= s.total {
		return nil, s.total, nil
	}
	end := start + opts.PerPage
	if end > s.total {
		end = s.total
	}
	now := time.Now().UTC()
	out := make([]entities.Entity, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, entities.Entity{
			ID:         fmt.Sprintf("ent-%05d", i),
			CampaignID: campaignID,
			Name:       fmt.Sprintf("Entity %05d", i),
			UpdatedAt:  now,
			CreatedAt:  now,
		})
	}
	return out, s.total, nil
}

func newSyncContext(t *testing.T, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/sync", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set(apiKeyContextKey, &APIKey{
		ID:         synthKeySessionID,
		CampaignID: "camp-1",
		UserID:     "user-1",
		IsActive:   true,
	})
	return c, rec
}

func doSync(t *testing.T, h *APIHandler, since time.Time, cursor string) syncResponse {
	t.Helper()
	body := fmt.Sprintf(`{"since":%q,"cursor":%q}`, since.Format(time.RFC3339Nano), cursor)
	c, rec := newSyncContext(t, body)
	if err := h.Sync(c); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

// TestSyncPull_TailIsReachableThroughCursor is THE regression. The campaign
// holds more entities than one request may walk; every one of them must
// arrive across a bounded cursor walk. Before the cursor, the entities past
// the first page-budget were unreachable by any sequence of requests.
func TestSyncPull_TailIsReachableThroughCursor(t *testing.T) {
	const perRequest = syncMaxPullPages * syncPageSize // what one request walks
	const total = perRequest*2 + 37                    // two full budgets and a bit

	h := NewAPIHandler(nil, &bigCampaignEntityService{total: total}, stubCampaignSvcOwner{}, nil)
	since := time.Now().Add(-time.Hour).UTC()

	seen := map[string]bool{}
	cursor := ""
	requests := 0
	for {
		requests++
		if requests > 10 {
			t.Fatalf("cursor walk did not terminate after %d requests", requests)
		}
		resp := doSync(t, h, since, cursor)
		for _, e := range resp.Entities {
			if seen[e.ID] {
				t.Errorf("entity %s returned twice across the walk", e.ID)
			}
			seen[e.ID] = true
		}
		if !resp.HasMore {
			if resp.NextCursor != "" {
				t.Errorf("has_more=false but next_cursor=%q; the client would loop forever", resp.NextCursor)
			}
			break
		}
		if resp.NextCursor == "" {
			t.Fatal("has_more=true with an empty next_cursor — the tail is unreachable, which is the whole bug")
		}
		cursor = resp.NextCursor
	}

	if len(seen) != total {
		t.Fatalf("walk delivered %d of %d entities; %d can never sync", len(seen), total, total-len(seen))
	}
	// The last entity in list order is the one the old ceiling could never
	// reach; name it explicitly so a regression reads as what it is.
	last := fmt.Sprintf("ent-%05d", total-1)
	if !seen[last] {
		t.Errorf("the campaign's last entity (%s) never arrived", last)
	}
	if requests < 3 {
		t.Errorf("walk finished in %d requests; the per-request cap is not being applied", requests)
	}
}

// TestSyncPull_SmallCampaignNeedsNoCursor keeps the cap honest in the other
// direction: a campaign that fits in one request must not hand back a cursor
// that makes the client ask for a page that does not exist.
func TestSyncPull_SmallCampaignNeedsNoCursor(t *testing.T) {
	h := NewAPIHandler(nil, &bigCampaignEntityService{total: 42}, stubCampaignSvcOwner{}, nil)
	resp := doSync(t, h, time.Now().Add(-time.Hour).UTC(), "")

	if resp.HasMore {
		t.Error("has_more=true for a 42-entity campaign")
	}
	if resp.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty", resp.NextCursor)
	}
	if len(resp.Entities) != 42 {
		t.Errorf("pulled %d entities, want 42", len(resp.Entities))
	}
}

// TestSyncPull_ExactMultipleDoesNotOverrun pins the boundary where the
// campaign ends exactly on the request budget: no cursor, no phantom page.
func TestSyncPull_ExactMultipleDoesNotOverrun(t *testing.T) {
	h := NewAPIHandler(nil, &bigCampaignEntityService{total: syncMaxPullPages * syncPageSize},
		stubCampaignSvcOwner{}, nil)
	resp := doSync(t, h, time.Now().Add(-time.Hour).UTC(), "")

	if resp.HasMore || resp.NextCursor != "" {
		t.Errorf("has_more=%v next_cursor=%q at an exact budget boundary, want false/empty",
			resp.HasMore, resp.NextCursor)
	}
	if len(resp.Entities) != syncMaxPullPages*syncPageSize {
		t.Errorf("pulled %d entities, want %d", len(resp.Entities), syncMaxPullPages*syncPageSize)
	}
}

// TestSyncPull_BadCursorIsRefused: a malformed cursor must 400, not silently
// restart at page 1. A silent restart reports success while leaving the tail
// exactly as unreachable as before.
func TestSyncPull_BadCursorIsRefused(t *testing.T) {
	h := NewAPIHandler(nil, &bigCampaignEntityService{total: 10}, stubCampaignSvcOwner{}, nil)
	for _, bad := range []string{"not-base64!!", "b3RoZXItdjk6MQ", "c3luYy12MTowzz"} {
		body := fmt.Sprintf(`{"since":%q,"cursor":%q}`,
			time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), bad)
		c, _ := newSyncContext(t, body)
		if err := h.Sync(c); err == nil {
			t.Errorf("cursor %q was accepted; a bad cursor must be refused, not silently reset", bad)
		}
	}
}

func TestSyncCursor_RoundTripAndBounds(t *testing.T) {
	for _, page := range []int{1, 2, 11, syncMaxCursorPage} {
		got, err := decodeSyncCursor(encodeSyncCursor(page))
		if err != nil {
			t.Fatalf("decode(encode(%d)): %v", page, err)
		}
		if got != page {
			t.Errorf("decode(encode(%d)) = %d", page, got)
		}
	}
	if got, err := decodeSyncCursor(""); err != nil || got != 1 {
		t.Errorf("empty cursor = (%d, %v), want (1, nil)", got, err)
	}
	if _, err := decodeSyncCursor(encodeSyncCursor(syncMaxCursorPage + 1)); err == nil {
		t.Error("an out-of-range cursor page was accepted")
	}
	if _, err := decodeSyncCursor(encodeSyncCursor(0)); err == nil {
		t.Error("page 0 was accepted")
	}
}
