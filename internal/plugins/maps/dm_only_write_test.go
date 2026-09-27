// dm_only_write_test.go pins the maps half of the dm_only write-path fix:
// CreateMarkerAPI's dm_only downgrade checks CanAuthorDmOnly() (Owner or a
// co-DM grant), not MemberRole alone, and UpdateMarkerAPI/DeleteMarkerAPI
// refuse to touch a STORED dm_only marker for anyone who can't author
// dm_only content — the same NotFound a missing id would give — regardless
// of which fields the request carries. Every case runs the same four-role
// matrix (Owner, co-DM, Scribe, Player) through the real handler + service,
// backed by a mock repo, so both layers are exercised together.
package maps

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

func dmWriteCampaignCtx(role campaigns.Role, dmGranted bool) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  role,
		IsDmGranted: dmGranted,
	}
}

func newDMWriteRequest(method, path, body string) (echo.Context, *httptest.ResponseRecorder) {
	if body == "" {
		body = "{}"
	}
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// --- CreateMarkerAPI ---

// TestCreateMarkerAPI_DmOnlyDowngrade_ByCapability pins that the dm_only
// downgrade on create is keyed on CanAuthorDmOnly(), not MemberRole alone:
// a co-DM's dm_only marker must stay dm_only, not get silently downgraded
// to 'everyone' the way a plain Scribe's does.
func TestCreateMarkerAPI_DmOnlyDowngrade_ByCapability(t *testing.T) {
	for _, tc := range dmWriteRoles {
		t.Run(tc.name, func(t *testing.T) {
			var created *Marker
			repo := &mockMapRepo{
				getMapFn: func(_ context.Context, id string) (*Map, error) {
					return &Map{ID: id, CampaignID: "camp-1"}, nil
				},
				createMarkerFn: func(_ context.Context, mk *Marker) error {
					created = mk
					return nil
				},
			}
			h := NewHandler(NewMapService(repo))
			c, rec := newDMWriteRequest(http.MethodPost, "/campaigns/camp-1/maps/map-1/markers",
				`{"name":"Secret Door","x":10,"y":10,"icon":"fa-door-closed","color":"#000000","visibility":"dm_only"}`)
			c.SetParamNames("id", "mid")
			c.SetParamValues("camp-1", "map-1")
			c.Set("campaign_context", dmWriteCampaignCtx(tc.role, tc.dmGranted))

			if err := h.CreateMarkerAPI(c); err != nil {
				t.Fatalf("%s: CreateMarkerAPI: %v", tc.name, err)
			}
			if rec.Code != http.StatusCreated {
				t.Fatalf("%s: status = %d, want 201", tc.name, rec.Code)
			}
			if created == nil {
				t.Fatalf("%s: CreateMarker was never called", tc.name)
			}
			wantVis := "everyone"
			if tc.canAuthorDmOnly {
				wantVis = "dm_only"
			}
			if created.Visibility != wantVis {
				t.Errorf("%s: created marker visibility = %q, want %q", tc.name, created.Visibility, wantVis)
			}
		})
	}
}

// --- UpdateMarkerAPI / DeleteMarkerAPI on a STORED dm_only marker ---

func storedDMOnlyMarker() *Marker {
	return &Marker{
		ID: "mk-secret", MapID: "map-1", Name: "Secret Door",
		X: 10, Y: 10, Icon: "fa-door-closed", Color: "#000000", Visibility: "dm_only",
	}
}

func newDMOnlyMapRepo() *mockMapRepo {
	return &mockMapRepo{
		getMapFn: func(_ context.Context, id string) (*Map, error) {
			return &Map{ID: id, CampaignID: "camp-1"}, nil
		},
		getMarkerFn: func(_ context.Context, id string) (*Marker, error) {
			if id == "mk-secret" {
				return storedDMOnlyMarker(), nil
			}
			return nil, nil
		},
	}
}

// TestUpdateMarkerAPI_StoredDmOnly_RequiresCanAuthorDmOnly sends only
// {"name": ...} — no visibility field — the blind-write shape: a rename
// that never touches visibility must still be refused with the same
// NotFound a missing id would give, for any caller who cannot author
// dm_only content.
func TestUpdateMarkerAPI_StoredDmOnly_RequiresCanAuthorDmOnly(t *testing.T) {
	for _, tc := range dmWriteRoles {
		t.Run(tc.name, func(t *testing.T) {
			var updated bool
			repo := newDMOnlyMapRepo()
			repo.updateMarkerFn = func(_ context.Context, _ *Marker) error {
				updated = true
				return nil
			}
			h := NewHandler(NewMapService(repo))
			c, rec := newDMWriteRequest(http.MethodPut, "/campaigns/camp-1/maps/map-1/markers/mk-secret",
				`{"name":"Renamed Door"}`)
			c.SetParamNames("id", "mid", "mkid")
			c.SetParamValues("camp-1", "map-1", "mk-secret")
			c.Set("campaign_context", dmWriteCampaignCtx(tc.role, tc.dmGranted))

			err := h.UpdateMarkerAPI(c)
			if tc.canAuthorDmOnly {
				if err != nil {
					t.Fatalf("%s: expected success, got %v", tc.name, err)
				}
				if rec.Code != http.StatusOK {
					t.Errorf("%s: status = %d, want 200", tc.name, rec.Code)
				}
				if !updated {
					t.Errorf("%s: expected UpdateMarker to reach the repo", tc.name)
				}
			} else {
				assertAppError(t, err, http.StatusNotFound)
				if updated {
					t.Errorf("%s: UpdateMarker must not reach the repo for a caller who cannot author dm_only content", tc.name)
				}
			}
		})
	}
}

// TestDeleteMarkerAPI_StoredDmOnly_RequiresCanAuthorDmOnly is DeleteMarker's
// twin of the Update case above.
func TestDeleteMarkerAPI_StoredDmOnly_RequiresCanAuthorDmOnly(t *testing.T) {
	for _, tc := range dmWriteRoles {
		t.Run(tc.name, func(t *testing.T) {
			var deleted bool
			repo := newDMOnlyMapRepo()
			repo.deleteMarkerFn = func(_ context.Context, _ string) error {
				deleted = true
				return nil
			}
			h := NewHandler(NewMapService(repo))
			c, rec := newDMWriteRequest(http.MethodDelete, "/campaigns/camp-1/maps/map-1/markers/mk-secret", "")
			c.SetParamNames("id", "mid", "mkid")
			c.SetParamValues("camp-1", "map-1", "mk-secret")
			c.Set("campaign_context", dmWriteCampaignCtx(tc.role, tc.dmGranted))

			err := h.DeleteMarkerAPI(c)
			if tc.canAuthorDmOnly {
				if err != nil {
					t.Fatalf("%s: expected success, got %v", tc.name, err)
				}
				if rec.Code != http.StatusOK {
					t.Errorf("%s: status = %d, want 200", tc.name, rec.Code)
				}
				if !deleted {
					t.Errorf("%s: expected DeleteMarker to reach the repo", tc.name)
				}
			} else {
				assertAppError(t, err, http.StatusNotFound)
				if deleted {
					t.Errorf("%s: DeleteMarker must not reach the repo for a caller who cannot author dm_only content", tc.name)
				}
			}
		})
	}
}

// --- Control: a stored 'everyone' marker is unaffected by canAuthorDmOnly ---

func storedEveryoneMarker() *Marker {
	return &Marker{
		ID: "mk-public", MapID: "map-1", Name: "Town Square",
		X: 20, Y: 20, Icon: "fa-flag", Color: "#00ff00", Visibility: "everyone",
	}
}

func everyoneMapRepo() *mockMapRepo {
	return &mockMapRepo{
		getMapFn: func(_ context.Context, id string) (*Map, error) {
			return &Map{ID: id, CampaignID: "camp-1"}, nil
		},
		getMarkerFn: func(_ context.Context, id string) (*Marker, error) {
			if id == "mk-public" {
				return storedEveryoneMarker(), nil
			}
			return nil, nil
		},
	}
}

// TestUpdateMarkerAPI_StoredEveryone_AlwaysReachesRepo is the control for
// TestUpdateMarkerAPI_StoredDmOnly_RequiresCanAuthorDmOnly: the dm_only gate
// must never block a Scribe (or Player, or anyone else) from editing a
// marker that was never dm_only in the first place.
func TestUpdateMarkerAPI_StoredEveryone_AlwaysReachesRepo(t *testing.T) {
	for _, tc := range dmWriteRoles {
		t.Run(tc.name, func(t *testing.T) {
			var updated bool
			repo := everyoneMapRepo()
			repo.updateMarkerFn = func(_ context.Context, _ *Marker) error {
				updated = true
				return nil
			}
			h := NewHandler(NewMapService(repo))
			c, rec := newDMWriteRequest(http.MethodPut, "/campaigns/camp-1/maps/map-1/markers/mk-public",
				`{"name":"Renamed Square"}`)
			c.SetParamNames("id", "mid", "mkid")
			c.SetParamValues("camp-1", "map-1", "mk-public")
			c.Set("campaign_context", dmWriteCampaignCtx(tc.role, tc.dmGranted))

			if err := h.UpdateMarkerAPI(c); err != nil {
				t.Fatalf("%s: expected success editing a non-dm_only marker, got %v", tc.name, err)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want 200", tc.name, rec.Code)
			}
			if !updated {
				t.Errorf("%s: expected UpdateMarker to reach the repo for a non-dm_only marker", tc.name)
			}
		})
	}
}

// TestDeleteMarkerAPI_StoredEveryone_AlwaysReachesRepo is DeleteMarker's
// twin of the Update control above.
func TestDeleteMarkerAPI_StoredEveryone_AlwaysReachesRepo(t *testing.T) {
	for _, tc := range dmWriteRoles {
		t.Run(tc.name, func(t *testing.T) {
			var deleted bool
			repo := everyoneMapRepo()
			repo.deleteMarkerFn = func(_ context.Context, _ string) error {
				deleted = true
				return nil
			}
			h := NewHandler(NewMapService(repo))
			c, rec := newDMWriteRequest(http.MethodDelete, "/campaigns/camp-1/maps/map-1/markers/mk-public", "")
			c.SetParamNames("id", "mid", "mkid")
			c.SetParamValues("camp-1", "map-1", "mk-public")
			c.Set("campaign_context", dmWriteCampaignCtx(tc.role, tc.dmGranted))

			if err := h.DeleteMarkerAPI(c); err != nil {
				t.Fatalf("%s: expected success deleting a non-dm_only marker, got %v", tc.name, err)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want 200", tc.name, rec.Code)
			}
			if !deleted {
				t.Errorf("%s: expected DeleteMarker to reach the repo for a non-dm_only marker", tc.name)
			}
		})
	}
}
