package maps

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// allGiven gives every map of the campaign to every user, for tests about
// other rules (shadows, fog, pins) that view as a Player.
type allGiven struct{ repo MapRepository }

func (g allGiven) GivenMapIDs(ctx context.Context, campaignID, _ string) (map[string]bool, error) {
	ms, err := g.repo.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, m := range ms {
		out[m.ID] = true
	}
	return out, nil
}

func (allGiven) MapHolders(context.Context, string, string) ([]string, error) { return nil, nil }

// givenAllMapService is NewMapService with every map given to everyone.
func givenAllMapService(repo MapRepository) MapService {
	s := NewMapService(repo)
	s.SetMapGrants(allGiven{repo})
	return s
}

// fakeGrants gives the listed maps to the listed users.
type fakeGrants struct {
	given   map[string][]string // user -> maps
	holders map[string][]string // map -> users
	err     error
}

func (f fakeGrants) GivenMapIDs(_ context.Context, _, userID string) (map[string]bool, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]bool{}
	for _, id := range f.given[userID] {
		out[id] = true
	}
	return out, nil
}

func (f fakeGrants) MapHolders(_ context.Context, _, mapID string) ([]string, error) {
	return f.holders[mapID], f.err
}

func TestMapFilterFor(t *testing.T) {
	grants := fakeGrants{given: map[string][]string{"mira": {"m1"}}}
	cases := []struct {
		name   string
		grants MapGrants
		role   int
		user   string
		want   map[string]bool
		err    bool
	}{
		{"scribe sees every map", grants, permissions.RoleScribe, "s", map[string]bool{"m1": true, "m2": true}, false},
		{"owner sees every map", nil, permissions.RoleOwner, "o", map[string]bool{"m1": true, "m2": true}, false},
		{"player sees the map given to them", grants, permissions.RolePlayer, "mira", map[string]bool{"m1": true, "m2": false}, false},
		{"another player sees none", grants, permissions.RolePlayer, "bram", map[string]bool{"m1": false, "m2": false}, false},
		{"anonymous visitor sees none", grants, permissions.RoleNone, "", map[string]bool{"m1": false, "m2": false}, false},
		{"unwired, players see none", nil, permissions.RolePlayer, "mira", map[string]bool{"m1": false, "m2": false}, false},
		{"a failed lookup is an error, never every map", fakeGrants{err: errors.New("db")}, permissions.RolePlayer, "mira", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMapService(&mockMapRepo{})
			if tc.grants != nil {
				s.SetMapGrants(tc.grants)
			}
			f, err := s.MapFilterFor(context.Background(), "camp-1", tc.role, tc.user)
			if tc.err {
				if err == nil {
					t.Fatal("want an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for id, want := range tc.want {
				if got := f.Sees(id); got != want {
					t.Errorf("Sees(%s) = %v, want %v", id, got, want)
				}
			}
			kept := f.Keep([]Map{{ID: "m1"}, {ID: "m2"}})
			n := 0
			for _, w := range tc.want {
				if w {
					n++
				}
			}
			if len(kept) != n {
				t.Errorf("Keep kept %d maps, want %d", len(kept), n)
			}
		})
	}
}

// Every route under /maps/:mid answers a map the viewer was not given exactly
// as a missing map, and lets a given map through.
func TestRequireMapVisible(t *testing.T) {
	s := NewMapService(&mockMapRepo{})
	s.SetMapGrants(fakeGrants{given: map[string][]string{"mira": {"m1"}}})
	cases := []struct {
		name string
		role campaigns.Role
		user string
		mid  string
		svc  MapService
		pass bool
	}{
		{"given map", campaigns.RolePlayer, "mira", "m1", s, true},
		{"map not given", campaigns.RolePlayer, "mira", "m2", s, false},
		{"other player", campaigns.RolePlayer, "bram", "m1", s, false},
		{"scribe", campaigns.RoleScribe, "sam", "m2", s, true},
		{"no map in the route", campaigns.RolePlayer, "bram", "", s, true},
		{"unwired service hides from players", campaigns.RolePlayer, "mira", "m1", nil, false},
		{"unwired service still lets a scribe in", campaigns.RoleScribe, "sam", "m1", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: tc.role}
			reached := false
			next := func(*echo.Context) error { reached = true; return nil }
			svc := tc.svc
			mw := requireMapVisible(func() MapService { return svc })
			c := newGivenTestContext(t, cc, tc.user, tc.mid)
			err := mw(next)(c)
			if reached != tc.pass {
				t.Fatalf("reached=%v, want %v (err %v)", reached, tc.pass, err)
			}
			if !tc.pass {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
					t.Fatalf("want 404, got %v", err)
				}
			}
		})
	}
}

// newGivenTestContext is a request inside the campaign as user, on map mid.
// The user id key is the auth plugin's own.
func newGivenTestContext(t *testing.T, cc *campaigns.CampaignContext, user, mid string) *echo.Context {
	t.Helper()
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/x", nil), httptest.NewRecorder())
	vals := echo.PathValues{{Name: "id", Value: "camp-1"}}
	if mid != "" {
		vals = append(vals, echo.PathValue{Name: "mid", Value: mid})
	}
	c.SetPathValues(vals)
	c.Set("campaign_context", cc)
	if user != "" {
		c.Set("auth_user_id", user)
	}
	return c
}
