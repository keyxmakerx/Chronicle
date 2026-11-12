package sessions

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeHeroes struct {
	tracked, has bool
	err          error
}

func (f fakeHeroes) PlayingHero(context.Context, string, string) (bool, bool, error) {
	return f.tracked, f.has, f.err
}

// The block lists what a new player still has to do, ticks what is true of
// them, and removes itself once everything is done.
func TestGettingStartedBlock(t *testing.T) {
	tests := []struct {
		name     string
		heroes   HeroChecker
		answered bool
		visitor  bool
		want     []string
		not      []string
		empty    bool
	}{
		{"new player", fakeHeroes{tracked: true}, false, false,
			[]string{"Getting started", "0 of 2 done", "Choose your hero", "/campaigns/camp-1/characters", "Set my times", "/campaigns/camp-1/availability"}, nil, false},
		{"hero chosen", fakeHeroes{tracked: true, has: true}, false, false,
			[]string{"1 of 2 done", "Choose your hero", "(done)", "Set my times"}, []string{"Choose a hero"}, false},
		{"times given", fakeHeroes{tracked: true}, true, false,
			[]string{"1 of 2 done", "Choose a hero"}, []string{"Set my times"}, false},
		{"claiming off leaves the hero step out", fakeHeroes{}, false, false,
			[]string{"0 of 1 done", "Set my times"}, []string{"Choose your hero"}, false},
		{"a hero check that fails leaves the step out", fakeHeroes{err: errors.New("down")}, false, false,
			[]string{"Set my times"}, []string{"Choose your hero"}, false},
		{"everything done", fakeHeroes{tracked: true, has: true}, true, false, nil, nil, true},
		{"a visitor has nothing to do", fakeHeroes{tracked: true}, false, true, nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepo{listAnsweredUserIDsFn: func(context.Context, string) (map[string]time.Time, error) {
				if tt.answered {
					return map[string]time.Time{"u1": time.Now()}, nil
				}
				return map[string]time.Time{}, nil
			}}
			h := NewHandler(newTestSessionService(repo))
			h.SetHeroChecker(tt.heroes)
			c, rec := recapCtx(campaigns.RolePlayer, false, http.MethodGet, "")
			if tt.visitor {
				c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}})
			} else {
				auth.SetSession(c, &auth.Session{UserID: "u1"})
			}
			if err := h.GettingStartedBlock(c); err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			if tt.empty {
				if body != "" {
					t.Fatalf("want the block removed, got:\n%s", body)
				}
				return
			}
			for _, w := range tt.want {
				if !strings.Contains(body, w) {
					t.Errorf("missing %q:\n%s", w, body)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(body, n) {
					t.Errorf("should not hold %q:\n%s", n, body)
				}
			}
		})
	}
}
