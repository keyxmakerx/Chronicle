package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// The other map stubs let every map through: their tests are about writes
// and roles, not about which maps a Player was given.
func (s *stubMapSvcOwnerGate) CanSeeMap(context.Context, string, string, int, string) (bool, error) {
	return true, nil
}
func (s *stubMapSvcLinkedMap) CanSeeMap(context.Context, string, string, int, string) (bool, error) {
	return true, nil
}
func (s *stubMapSvcMarkerScope) CanSeeMap(context.Context, string, string, int, string) (bool, error) {
	return true, nil
}

// stubMapSvcGiven holds two maps; map-1 was given to user "mira".
type stubMapSvcGiven struct {
	maps.MapService
}

var givenMaps = []maps.Map{{ID: "map-1", CampaignID: "camp-1", Name: "Saltreach"}, {ID: "map-2", CampaignID: "camp-1", Name: "Imix"}}

func (stubMapSvcGiven) GetMap(_ context.Context, id string) (*maps.Map, error) {
	for i := range givenMaps {
		if givenMaps[i].ID == id {
			m := givenMaps[i]
			return &m, nil
		}
	}
	return nil, apperror.NewNotFound("map not found")
}
func (stubMapSvcGiven) ListMaps(context.Context, string) ([]maps.Map, error) {
	return append([]maps.Map(nil), givenMaps...), nil
}
func (stubMapSvcGiven) ForViewerList(_ context.Context, ms []maps.Map, _ int) ([]maps.Map, error) {
	return ms, nil
}
func (stubMapSvcGiven) ForViewer(_ context.Context, m *maps.Map, _ int) (*maps.Map, error) {
	return m, nil
}
func (stubMapSvcGiven) PlayerImageVersion(context.Context, *maps.Map) (string, error) { return "", nil }
func (stubMapSvcGiven) ListMarkers(context.Context, string, string, int, string) ([]maps.Marker, error) {
	return nil, nil
}
func (s stubMapSvcGiven) MapFilterFor(ctx context.Context, _ string, role int, userID string) (maps.MapFilter, error) {
	if maps.SeesAllMaps(role) {
		return maps.AllMaps(), nil
	}
	ok, _ := s.CanSeeMap(ctx, "camp-1", "map-1", role, userID)
	if !ok {
		return maps.MapFilter{}, nil
	}
	return onlyMap1(), nil
}
func (stubMapSvcGiven) CanSeeMap(_ context.Context, _, mapID string, role int, userID string) (bool, error) {
	return maps.SeesAllMaps(role) || (userID == "mira" && mapID == "map-1"), nil
}

// onlyMap1 builds a filter that keeps map-1 through the real service, so the
// test holds the production MapFilter type.
func onlyMap1() maps.MapFilter {
	s := maps.NewMapService(nil)
	s.SetMapGrants(miraGrants{})
	f, _ := s.MapFilterFor(context.Background(), "camp-1", 1, "mira")
	return f
}

type miraGrants struct{}

func (miraGrants) GivenMapIDs(context.Context, string, string) (map[string]bool, error) {
	return map[string]bool{"map-1": true}, nil
}
func (miraGrants) MapHolders(context.Context, string, string) ([]string, error) { return nil, nil }

// A Player's browser session reaches the sync API as itself, so it lists and
// opens only the maps given to a character of theirs; a Scribe sees them all.
func TestMapAPI_PlayerSeesOnlyGivenMaps(t *testing.T) {
	cases := []struct {
		name     string
		role     campaigns.Role
		user     string
		wantList []string
		wantGet  map[string]bool // map id -> found
	}{
		{"given player", campaigns.RolePlayer, "mira", []string{"map-1"}, map[string]bool{"map-1": true, "map-2": false}},
		{"other player", campaigns.RolePlayer, "bram", nil, map[string]bool{"map-1": false, "map-2": false}},
		{"scribe", campaigns.RoleScribe, "sam", []string{"map-1", "map-2"}, map[string]bool{"map-1": true, "map-2": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewMapAPIHandler(nil, stubMapSvcGiven{}, &stubDrawingSvcOwnerGate{}, &stubCampaignSvcOwnerGate{role: tc.role})
			key := &APIKey{CampaignID: "camp-1", UserID: tc.user}

			c, rec := newMapAPIContext(http.MethodGet, "/api/v1/campaigns/camp-1/maps", key)
			if err := h.ListMaps(c); err != nil {
				t.Fatal(err)
			}
			var got []maps.Map
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.wantList) {
				t.Fatalf("listed %d maps, want %v", len(got), tc.wantList)
			}
			for i, id := range tc.wantList {
				if got[i].ID != id {
					t.Fatalf("listed %s, want %s", got[i].ID, id)
				}
			}

			for id, found := range tc.wantGet {
				c, _ := newMapAPIContext(http.MethodGet, "/api/v1/campaigns/camp-1/maps/"+id, key)
				c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "mapID", Value: id}})
				_, err := h.requireMapInCampaign(c)
				var ae *apperror.AppError
				if found && err != nil {
					t.Errorf("%s: %v, want found", id, err)
				}
				if !found && (!errors.As(err, &ae) || ae.Code != http.StatusNotFound) {
					t.Errorf("%s: %v, want 404", id, err)
				}
			}
		})
	}
}
