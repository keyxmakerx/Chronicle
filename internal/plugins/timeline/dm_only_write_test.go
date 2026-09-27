// dm_only_write_test.go pins the timeline half of the dm_only write-path
// fix: CreateStandaloneEventAPI's dm_only downgrade checks CanAuthorDmOnly()
// (Owner or a co-DM grant), not MemberRole alone, and
// UpdateStandaloneEventAPI/DeleteStandaloneEventAPI refuse to touch a
// STORED dm_only event for anyone who can't author dm_only content — the
// same NotFound a missing id would give — regardless of which fields the
// request carries. Every case runs the same four-role matrix (Owner, co-DM,
// Scribe, Player) through the real handler + service, backed by a mock
// repo, so both layers are exercised together.
package timeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// dmWriteRoles is the four-way matrix every case in this file runs: an
// Owner and a co-DM (Player + DmGrant) can author dm_only content; a plain
// Scribe or Player cannot.
var dmWriteRoles = []struct {
	name            string
	role            campaigns.Role
	dmGranted       bool
	canAuthorDmOnly bool
}{
	{"owner", campaigns.RoleOwner, false, true},
	{"co-DM (DM grant)", campaigns.RolePlayer, true, true},
	{"scribe", campaigns.RoleScribe, false, false},
	{"player", campaigns.RolePlayer, false, false},
}

func dmWriteTimelineCtx(role campaigns.Role, dmGranted bool) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  role,
		IsDmGranted: dmGranted,
	}
}

func newTimelineDMWriteRequest(method, path, body string) (echo.Context, *httptest.ResponseRecorder) {
	if body == "" {
		body = "{}"
	}
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func dmWriteTimelineRepo() *mockTimelineRepo {
	return &mockTimelineRepo{
		getByIDFn: func(_ context.Context, id string) (*Timeline, error) {
			return &Timeline{ID: id, CampaignID: "camp-1", Name: "Test Timeline", Visibility: "everyone"}, nil
		},
	}
}

// --- CreateStandaloneEventAPI ---

// TestCreateStandaloneEventAPI_DmOnlyDowngrade_ByCapability pins that the
// dm_only downgrade on create is keyed on CanAuthorDmOnly(), not MemberRole
// alone: a co-DM's dm_only event must stay dm_only, not get silently
// downgraded to 'everyone' the way a plain Scribe's does.
func TestCreateStandaloneEventAPI_DmOnlyDowngrade_ByCapability(t *testing.T) {
	for _, tc := range dmWriteRoles {
		t.Run(tc.name, func(t *testing.T) {
			var created *TimelineEvent
			repo := dmWriteTimelineRepo()
			repo.createEventFn = func(_ context.Context, e *TimelineEvent) error {
				created = e
				return nil
			}
			h := NewHandler(newTestTimelineService(repo))
			c, rec := newTimelineDMWriteRequest(http.MethodPost, "/campaigns/camp-1/timelines/tl-1/standalone-events",
				`{"name":"Secret Meeting","year":1,"month":1,"day":1,"visibility":"dm_only"}`)
			c.SetParamNames("id", "tid")
			c.SetParamValues("camp-1", "tl-1")
			c.Set("campaign_context", dmWriteTimelineCtx(tc.role, tc.dmGranted))

			if err := h.CreateStandaloneEventAPI(c); err != nil {
				t.Fatalf("%s: CreateStandaloneEventAPI: %v", tc.name, err)
			}
			if rec.Code != http.StatusCreated {
				t.Fatalf("%s: status = %d, want 201", tc.name, rec.Code)
			}
			if created == nil {
				t.Fatalf("%s: CreateEvent was never called", tc.name)
			}
			wantVis := "everyone"
			if tc.canAuthorDmOnly {
				wantVis = "dm_only"
			}
			if created.Visibility != wantVis {
				t.Errorf("%s: created event visibility = %q, want %q", tc.name, created.Visibility, wantVis)
			}
		})
	}
}

// --- UpdateStandaloneEventAPI / DeleteStandaloneEventAPI on a STORED
// dm_only event ---

func storedDMOnlyEvent() *TimelineEvent {
	return &TimelineEvent{
		ID: "evt-secret", TimelineID: "tl-1", Name: "Secret Meeting",
		Year: 1, Month: 1, Day: 1, Visibility: "dm_only",
	}
}

func dmOnlyTimelineRepo() *mockTimelineRepo {
	repo := dmWriteTimelineRepo()
	repo.getEventFn = func(_ context.Context, id string) (*TimelineEvent, error) {
		if id == "evt-secret" {
			return storedDMOnlyEvent(), nil
		}
		return nil, nil
	}
	return repo
}

// TestUpdateStandaloneEventAPI_StoredDmOnly_RequiresCanAuthorDmOnly sends
// only {"name": ...} — no visibility field — the blind-write shape: a
// rename that never touches visibility must still be refused with the same
// NotFound a missing id would give, for any caller who cannot author
// dm_only content.
func TestUpdateStandaloneEventAPI_StoredDmOnly_RequiresCanAuthorDmOnly(t *testing.T) {
	for _, tc := range dmWriteRoles {
		t.Run(tc.name, func(t *testing.T) {
			var updated bool
			repo := dmOnlyTimelineRepo()
			repo.updateEventFn = func(_ context.Context, _ *TimelineEvent) error {
				updated = true
				return nil
			}
			h := NewHandler(newTestTimelineService(repo))
			c, rec := newTimelineDMWriteRequest(http.MethodPut, "/campaigns/camp-1/timelines/tl-1/standalone-events/evt-secret",
				`{"name":"Renamed Meeting"}`)
			c.SetParamNames("id", "tid", "eid")
			c.SetParamValues("camp-1", "tl-1", "evt-secret")
			c.Set("campaign_context", dmWriteTimelineCtx(tc.role, tc.dmGranted))

			err := h.UpdateStandaloneEventAPI(c)
			if tc.canAuthorDmOnly {
				if err != nil {
					t.Fatalf("%s: expected success, got %v", tc.name, err)
				}
				if rec.Code != http.StatusOK {
					t.Errorf("%s: status = %d, want 200", tc.name, rec.Code)
				}
				if !updated {
					t.Errorf("%s: expected UpdateEvent to reach the repo", tc.name)
				}
			} else {
				assertAppError(t, err, http.StatusNotFound)
				if updated {
					t.Errorf("%s: UpdateEvent must not reach the repo for a caller who cannot author dm_only content", tc.name)
				}
			}
		})
	}
}

// TestDeleteStandaloneEventAPI_StoredDmOnly_RequiresCanAuthorDmOnly is
// DeleteStandaloneEvent's twin of the Update case above. The route itself
// is Scribe-level (routes.go), so a plain Scribe reaches the handler in
// production too — this is exactly the case the fix closes.
func TestDeleteStandaloneEventAPI_StoredDmOnly_RequiresCanAuthorDmOnly(t *testing.T) {
	for _, tc := range dmWriteRoles {
		t.Run(tc.name, func(t *testing.T) {
			var deleted bool
			repo := dmOnlyTimelineRepo()
			repo.deleteEventFn = func(_ context.Context, _ string) error {
				deleted = true
				return nil
			}
			h := NewHandler(newTestTimelineService(repo))
			c, rec := newTimelineDMWriteRequest(http.MethodDelete, "/campaigns/camp-1/timelines/tl-1/standalone-events/evt-secret", "")
			c.SetParamNames("id", "tid", "eid")
			c.SetParamValues("camp-1", "tl-1", "evt-secret")
			c.Set("campaign_context", dmWriteTimelineCtx(tc.role, tc.dmGranted))

			err := h.DeleteStandaloneEventAPI(c)
			if tc.canAuthorDmOnly {
				if err != nil {
					t.Fatalf("%s: expected success, got %v", tc.name, err)
				}
				if rec.Code != http.StatusOK {
					t.Errorf("%s: status = %d, want 200", tc.name, rec.Code)
				}
				if !deleted {
					t.Errorf("%s: expected DeleteEvent to reach the repo", tc.name)
				}
			} else {
				assertAppError(t, err, http.StatusNotFound)
				if deleted {
					t.Errorf("%s: DeleteEvent must not reach the repo for a caller who cannot author dm_only content", tc.name)
				}
			}
		})
	}
}
