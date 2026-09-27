// visibility_write_test.go: the required handler-level table test for the
// dm_only visibility-write rule, over {Scribe, co-DM, Owner} x body {absent,
// everyone, dm_only} x stored {everyone, dm_only}. Exercises the REAL
// handler and REAL service (over fake repositories), not access_test.go's
// fakeCalendarSvc — that harness deliberately skips business logic (see its
// doc comment), and the rule under test lives in the service.
package calendar

import (
	"context"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// visWriteStack wires the real handler + real service over fake repos,
// seeded with one event at CalendarID "cal-1" whose Visibility is stored.
// written captures the repo's UpdateEvent call (nil if the write was
// refused, or never reached because the event answered NotFound first).
func visWriteStack(roles map[string]campaigns.Role, settings, stored string) (*echo.Echo, **Event) {
	seed := &Event{ID: "evt-1", CalendarID: "cal-1", Name: "Secret War Council", Visibility: stored}
	var written *Event
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1", Visibility: "everyone"}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			if id != seed.ID {
				return nil, nil
			}
			e := *seed
			return &e, nil
		},
		updateEventFn: func(_ context.Context, evt *Event) error {
			e := *evt
			written = &e
			return nil
		},
	}
	svc := NewCalendarService(calRepo, eventRepo, &fakeEventKindRepo{}, &fakeWeatherRepo{})
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	campaignSvc := guardCampaignSvc{roles: roles, settings: settings}
	RegisterRoutes(e, NewHandler(svc), campaignSvc, guardAuthSvc{}, guardAddonSvc{enabled: true})
	return e, &written
}

// TestUpdateEventAPI_VisibilityAuthorizationMatrix is S1's required table:
// a non-author echoing the stored value is a no-op (never refused, never
// rewritten); a non-author attempting to CHANGE it is 403; and an event the
// caller cannot see at all (stored dm_only, caller not an author) is
// NotFound for every body, including a plain rename — the blind-write hole.
func TestUpdateEventAPI_VisibilityAuthorizationMatrix(t *testing.T) {
	type caller struct {
		name    string
		userID  string
		role    campaigns.Role
		granted bool
	}
	callers := []caller{
		{"Scribe", "u-scribe", campaigns.RoleScribe, false},
		{"co-DM", "u-codm", campaigns.RoleScribe, true},
		{"Owner", "u-owner", campaigns.RoleOwner, false},
	}
	bodies := []struct{ name, json string }{
		{"absent", `{"name":"Renamed"}`},
		{"everyone", `{"name":"Renamed","visibility":"everyone"}`},
		{"dm_only", `{"name":"Renamed","visibility":"dm_only"}`},
	}
	canAuthor := func(c caller) bool { return c.role >= campaigns.RoleOwner || c.granted }

	for _, stored := range []string{"everyone", "dm_only"} {
		for _, c := range callers {
			for _, body := range bodies {
				stored, c, body := stored, c, body
				t.Run(c.name+"_stored="+stored+"_body="+body.name, func(t *testing.T) {
					roles := map[string]campaigns.Role{c.userID: c.role}
					settings := ""
					if c.granted {
						settings = `{"dm_grant_ids":["` + c.userID + `"]}`
					}
					e, written := visWriteStack(roles, settings, stored)
					path := "/campaigns/camp-1/calendars/cal-1/events/evt-1"
					rec := doRequestWithBody(e, http.MethodPut, path, c.userID, body.json)

					visible := stored == "everyone" || canAuthor(c)
					if !visible {
						if rec.Code != http.StatusNotFound {
							t.Fatalf("hidden event must answer NotFound, got %d: %s", rec.Code, rec.Body.String())
						}
						if *written != nil {
							t.Fatalf("a caller who cannot see the event must never reach the repo write, got %+v", **written)
						}
						return
					}

					wantChange := body.name != "absent" && body.name != stored
					if wantChange && !canAuthor(c) {
						if rec.Code != http.StatusForbidden {
							t.Fatalf("a non-author attempting to CHANGE visibility must be 403, got %d: %s", rec.Code, rec.Body.String())
						}
						if *written != nil {
							t.Fatalf("a refused visibility change must never reach the repo write, got %+v", **written)
						}
						return
					}

					if rec.Code != http.StatusOK {
						t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
					}
					if *written == nil {
						t.Fatalf("expected the update to reach the repository")
					}
					wantStored := stored
					if body.name != "absent" {
						wantStored = body.name
					}
					if (**written).Visibility != wantStored {
						t.Errorf("stored visibility must end up %q, got %q", wantStored, (**written).Visibility)
					}
				})
			}
		}
	}
}
