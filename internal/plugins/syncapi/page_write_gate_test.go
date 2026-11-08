package syncapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// The sync API's page writes must refuse a Scribe's session exactly what the
// web page editor refuses it: a hidden page reads as not found, and only the
// owner or a Co-DM may hide or show a page.

type gateCampaignSvc struct {
	campaigns.CampaignService
	role      campaigns.Role
	dmGranted bool
}

func (s gateCampaignSvc) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: s.role}, nil
}

func (s gateCampaignSvc) IsUserDmGranted(context.Context, string, string) (bool, error) {
	return s.dmGranted, nil
}

// gateEntitySvc applies the default-mode rule CheckEntityAccess applies:
// below Owner, a private page is invisible and a public one is editable from
// Scribe up. Every write is recorded so a refused request can be shown to
// have changed nothing.
type gateEntitySvc struct {
	entities.EntityService
	private bool
	writes  []string
}

func (s *gateEntitySvc) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	return &entities.Entity{ID: id, CampaignID: "camp-1", IsPrivate: s.private, EntityTypeID: 1}, nil
}

func (s *gateEntitySvc) CheckEntityAccess(_ context.Context, _ string, role int, _ string) (*entities.EffectivePermission, error) {
	if role >= int(campaigns.RoleOwner) {
		return &entities.EffectivePermission{CanView: true, CanEdit: true}, nil
	}
	if s.private {
		return &entities.EffectivePermission{}, nil
	}
	return &entities.EffectivePermission{CanView: true, CanEdit: role >= int(campaigns.RoleScribe)}, nil
}

func (s *gateEntitySvc) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	return &entities.EntityType{ID: id, CampaignID: "camp-1"}, nil
}

func (s *gateEntitySvc) Update(_ context.Context, id string, _ entities.UpdateEntityInput) (*entities.Entity, error) {
	s.writes = append(s.writes, "update")
	return &entities.Entity{ID: id, CampaignID: "camp-1"}, nil
}

func (s *gateEntitySvc) MergeFields(context.Context, string, map[string]any) error {
	s.writes = append(s.writes, "fields")
	return nil
}

func (s *gateEntitySvc) TogglePrivate(context.Context, string) (bool, error) {
	s.writes = append(s.writes, "toggle")
	return !s.private, nil
}

func gateContext(method, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/api/v1/campaigns/camp-1/entities/ent-1", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "entityID")
	c.SetParamValues("camp-1", "ent-1")
	c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "u-1"})
	return c, rec
}

// gateStatus is the HTTP status a handler's result maps to.
func gateStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return -1
}

func TestSyncPageWrites_FollowWebRules(t *testing.T) {
	type call func(h *APIHandler, c echo.Context) error
	update := func(h *APIHandler, c echo.Context) error { return h.UpdateEntity(c) }
	fields := func(h *APIHandler, c echo.Context) error { return h.UpdateEntityFields(c) }
	reveal := func(h *APIHandler, c echo.Context) error { return h.ToggleEntityReveal(c) }

	cases := []struct {
		name      string
		role      campaigns.Role
		dmGranted bool
		private   bool
		do        call
		method    string
		body      string
		want      int
	}{
		{"scribe updates a hidden page", campaigns.RoleScribe, false, true, update, http.MethodPut, `{"name":"x"}`, http.StatusNotFound},
		{"scribe writes fields on a hidden page", campaigns.RoleScribe, false, true, fields, http.MethodPut, `{"fields_data":{}}`, http.StatusNotFound},
		{"scribe reveals a hidden page", campaigns.RoleScribe, false, true, reveal, http.MethodPost, `{"is_private":false}`, http.StatusForbidden},
		{"scribe hides a visible page", campaigns.RoleScribe, false, false, update, http.MethodPut, `{"is_private":true}`, http.StatusForbidden},
		{"scribe hides via reveal", campaigns.RoleScribe, false, false, reveal, http.MethodPost, `{"is_private":true}`, http.StatusForbidden},
		{"player updates a visible page", campaigns.RolePlayer, false, false, update, http.MethodPut, `{"name":"x"}`, http.StatusForbidden},
		{"scribe renames a visible page", campaigns.RoleScribe, false, false, update, http.MethodPut, `{"name":"x"}`, http.StatusOK},
		{"scribe resends the current privacy", campaigns.RoleScribe, false, false, update, http.MethodPut, `{"name":"x","is_private":false}`, http.StatusOK},
		{"scribe writes fields on a visible page", campaigns.RoleScribe, false, false, fields, http.MethodPut, `{"fields_data":{}}`, http.StatusOK},
		{"co-DM reveals a hidden page", campaigns.RoleScribe, true, true, reveal, http.MethodPost, `{"is_private":false}`, http.StatusOK},
		{"owner updates a hidden page", campaigns.RoleOwner, false, true, update, http.MethodPut, `{"is_private":false}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ents := &gateEntitySvc{private: tc.private}
			h := NewAPIHandler(&stubSyncSvcForRole{}, ents, gateCampaignSvc{role: tc.role, dmGranted: tc.dmGranted}, nil)
			c, _ := gateContext(tc.method, tc.body)
			err := tc.do(h, c)
			if got := gateStatus(err); got != tc.want {
				t.Fatalf("status = %d (err %v), want %d", got, err, tc.want)
			}
			if tc.want != http.StatusOK && len(ents.writes) > 0 {
				t.Fatalf("refused request still wrote: %v", ents.writes)
			}
		})
	}
}
