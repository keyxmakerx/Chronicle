package syncapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestRecordSyncHistory_NotedActor pins that a noted actor names who did the
// call only when it succeeded: a refused call must stay the key holder's, so
// a module's unchecked claim never lands in the history.
func TestRecordSyncHistory_NotedActor(t *testing.T) {
	cases := []struct {
		name    string
		handler echo.HandlerFunc
		want    string
	}{
		{"success records the noted actor", func(c echo.Context) error {
			noteSyncActor(c, "player-9")
			return c.NoContent(http.StatusOK)
		}, "player-9"},
		{"error keeps the key holder", func(c echo.Context) error {
			noteSyncActor(c, "player-9")
			return apperror.NewForbidden("not allowed")
		}, "gm-1"},
		{"no actor noted keeps the key holder", func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		}, "gm-1"},
		{"empty actor is ignored", func(c echo.Context) error {
			noteSyncActor(c, "")
			return c.NoContent(http.StatusOK)
		}, "gm-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeHistoryRepo{done: make(chan struct{}, 1)}
			key := &APIKey{ID: 4, UserID: "gm-1", CampaignID: "c1"}
			e := echo.New()
			e.POST("/api/v1/campaigns/:id/armory/shops/:eid/buy", tc.handler, func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error { c.Set(apiKeyContextKey, key); return next(c) }
			}, RecordSyncHistory(repo, nil))
			e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/c1/armory/shops/s1/buy", nil))
			select {
			case <-repo.done:
			case <-time.After(2 * time.Second):
				t.Fatal("nothing recorded")
			}
			ev := repo.inserted[0]
			if ev.UserID == nil || *ev.UserID != tc.want {
				got := "<nil>"
				if ev.UserID != nil {
					got = *ev.UserID
				}
				t.Fatalf("UserID = %s, want %s", got, tc.want)
			}
			if ev.Kind != "shop" || ev.Action != "bought" {
				t.Fatalf("classified as %q/%q", ev.Kind, ev.Action)
			}
		})
	}
}

func TestClassifyHistoryCall_ShopBuy(t *testing.T) {
	r, suffix, ok := classifyHistoryCall(http.MethodPost, "/api/v1/campaigns/:id/armory/shops/:eid/buy", http.StatusOK)
	if !ok || r.kind != "shop" || r.action != "bought" || suffix != "/armory/shops/:eid/buy" {
		t.Fatalf("got %+v %q %v", r, suffix, ok)
	}
	// A refused buy is still history: the owner needs to see it was refused.
	if _, _, ok := classifyHistoryCall(http.MethodPost, "/api/v1/campaigns/:id/armory/shops/:eid/buy", http.StatusForbidden); !ok {
		t.Fatal("refused buy not recorded")
	}
}
