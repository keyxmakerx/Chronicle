package maps

// given.go decides which maps a viewer may know about. A map has no "who can
// see" setting of its own: a Scribe or above sees every map, and a Player (or a
// visitor to a public campaign) sees only the maps the GM has given to a
// character they own, as the map's handout item in that character's inventory.
// Every place that sends a map, its name or anything on it to a user asks here
// first; a map a viewer may not see answers as if it did not exist.

import (
	"context"
	"fmt"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// MapGrants answers which maps have been given to a user's characters and who
// holds a map. Implemented in internal/app over the armory, relations and
// entities services. Unwired, Players see no maps at all.
type MapGrants interface {
	// GivenMapIDs is the set of maps a character the user owns carries the
	// handout of.
	GivenMapIDs(ctx context.Context, campaignID, userID string) (map[string]bool, error)
	// MapHolders is every user who owns a character carrying the map's
	// handout, for live messages about the map.
	MapHolders(ctx context.Context, campaignID, mapID string) ([]string, error)
}

// SeesAllMaps reports whether a viewer of this (promoted visibility) role sees
// every map. Scribes do: they draw on maps and write the pages that pin them.
func SeesAllMaps(role int) bool {
	return role >= permissions.RoleScribe
}

// MapFilter is one viewer's answer, read once per request.
type MapFilter struct {
	all   bool
	given map[string]bool
}

// AllMaps is the filter of a viewer who sees every map.
func AllMaps() MapFilter { return MapFilter{all: true} }

// Sees reports whether the viewer may know about the map.
func (f MapFilter) Sees(mapID string) bool {
	return f.all || f.given[mapID]
}

// SeesAll reports whether the filter lets every map through.
func (f MapFilter) SeesAll() bool { return f.all }

// Keep returns the maps the viewer may see, in order.
func (f MapFilter) Keep(ms []Map) []Map {
	if f.all {
		return ms
	}
	out := make([]Map, 0, len(ms))
	for _, m := range ms {
		if f.given[m.ID] {
			out = append(out, m)
		}
	}
	return out
}

// SetMapGrants wires who has been given which map.
func (s *mapService) SetMapGrants(g MapGrants) { s.grants = g }

// MapFilterFor reads the viewer's filter. A viewer with no account owns no
// characters and so sees no maps; a failed lookup is an error, never every map.
func (s *mapService) MapFilterFor(ctx context.Context, campaignID string, role int, userID string) (MapFilter, error) {
	if SeesAllMaps(role) {
		return AllMaps(), nil
	}
	if s.grants == nil || userID == "" {
		return MapFilter{}, nil
	}
	given, err := s.grants.GivenMapIDs(ctx, campaignID, userID)
	if err != nil {
		return MapFilter{}, fmt.Errorf("given maps: %w", err)
	}
	return MapFilter{given: given}, nil
}

// CanSeeMap reports whether the viewer may know about one map.
func (s *mapService) CanSeeMap(ctx context.Context, campaignID, mapID string, role int, userID string) (bool, error) {
	f, err := s.MapFilterFor(ctx, campaignID, role, userID)
	if err != nil {
		return false, err
	}
	return f.Sees(mapID), nil
}

// MapAudience is the Players who may hear live news of a map: its holders.
// Scribes and DMs are let in by the caller. Unwired, it is nobody.
func (s *mapService) MapAudience(ctx context.Context, campaignID, mapID string) ([]string, error) {
	if s.grants == nil {
		return []string{}, nil
	}
	holders, err := s.grants.MapHolders(ctx, campaignID, mapID)
	if err != nil {
		return nil, fmt.Errorf("map holders: %w", err)
	}
	if holders == nil {
		holders = []string{}
	}
	return holders, nil
}

// errMapHidden is what a viewer gets for a map they may not see: the same
// answer as a map that does not exist.
func errMapHidden() error {
	return apperror.NewNotFound("map not found")
}

// requireMapVisible answers a viewer who may not see the route's map (:mid)
// exactly as a missing map. Routes without :mid pass. A nil service refuses
// everyone below Scribe, so a route group wired without it hides maps rather
// than showing them.
func requireMapVisible(svc func() MapService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			mapID := c.Param("mid")
			if mapID == "" {
				return next(c)
			}
			cc := campaigns.GetCampaignContext(c)
			if cc == nil {
				return apperror.NewMissingContext()
			}
			role := cc.VisibilityRole()
			if SeesAllMaps(role) {
				return next(c)
			}
			s := svc()
			if s == nil {
				return errMapHidden()
			}
			ok, err := s.CanSeeMap(c.Request().Context(), cc.Campaign.ID, mapID, role, getUserID(c))
			if err != nil {
				return err
			}
			if !ok {
				return errMapHidden()
			}
			return next(c)
		}
	}
}
