package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type promoteSpy struct {
	EntityService
	e        *Entity
	promoted int
}

func (s *promoteSpy) GetByID(context.Context, string) (*Entity, error) { return s.e, nil }
func (s *promoteSpy) SetShopGood(_ context.Context, _, _ string, on bool) error {
	if !on {
		s.promoted++
	}
	return nil
}

// Making a shop good a full page needs the Director; nobody signed out reaches it.
func TestMakeFullPage_NeedsSignIn(t *testing.T) {
	spy := &promoteSpy{e: &Entity{ID: "e1", CampaignID: "camp-1", ShopGood: true}}
	e := echo.New()
	RegisterRoutes(e, NewHandler(spy), guardCampaignSvc{public: true}, guardAuthSvc{})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/entities/e1/full-page", nil))
	if !isLoginRedirect(rec) || spy.promoted != 0 {
		t.Errorf("anonymous: code=%d promoted=%d, want a sign-in redirect and no change", rec.Code, spy.promoted)
	}
}

// The route's capability gate is the Director's alone: a Player or Scribe is
// refused before the handler runs, a Director reaches it.
func TestMakeFullPage_DirectorGate(t *testing.T) {
	tests := []struct {
		name string
		cc   *campaigns.CampaignContext
		want bool
	}{
		{"owner", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}, true},
		{"co-DM", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsDmGranted: true}, true},
		{"scribe", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}, false},
		{"player", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &promoteSpy{e: &Entity{ID: "e1", CampaignID: "camp-1", ShopGood: true}}
			h := NewHandler(spy)
			// Same gate the route uses.
			gate := campaigns.RequireCapability((*campaigns.CampaignContext).CanAuthorDmOnly, "only the Director")
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("campaign_context", tt.cc)
			c.SetPathValues(echo.PathValues{{Name: "eid", Value: "e1"}})
			_ = gate(h.MakeFullPageAPI)(c)
			if got := spy.promoted == 1; got != tt.want {
				t.Errorf("promoted = %v, want %v", got, tt.want)
			}
		})
	}
}
