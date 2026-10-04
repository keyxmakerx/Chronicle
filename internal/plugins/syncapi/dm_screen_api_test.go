package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type dmScreenRoleCampaigns struct {
	campaigns.CampaignService
	role campaigns.Role
}

func (s *dmScreenRoleCampaigns) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: s.role}, nil
}

type fakeDMScreen struct {
	gotUser, gotCampaign, gotEntity string
	gotRole                         int
	gotOpen                         *bool
}

func (f *fakeDMScreen) Screen(_ context.Context, campaignID, userID string, role int) (any, error) {
	f.gotCampaign, f.gotUser, f.gotRole = campaignID, userID, role
	return map[string]any{"campaign_id": campaignID}, nil
}

func (f *fakeDMScreen) Reveal(_ context.Context, entityID, campaignID, userID string, role int) (string, error) {
	f.gotEntity, f.gotCampaign, f.gotUser, f.gotRole = entityID, campaignID, userID, role
	return "Zaltar", nil
}

func (f *fakeDMScreen) SetDowntime(_ context.Context, campaignID, userID string, role int, open bool) (any, error) {
	f.gotCampaign, f.gotUser, f.gotRole, f.gotOpen = campaignID, userID, role, &open
	return map[string]any{"open": open, "applied": 0, "failed": 0}, nil
}

func dmScreenContext(method, body string, keyID int, params ...string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/api/v1/campaigns/camp-1/dm-screen", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	names, values := []string{"id"}, []string{"camp-1"}
	for i := 0; i+1 < len(params); i += 2 {
		names, values = append(names, params[i]), append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	c.Set(apiKeyContextKey, &APIKey{ID: keyID, CampaignID: "camp-1", UserID: "user-1", IsActive: true})
	return c, rec
}

func dmStatusOf(t *testing.T, err error) int {
	t.Helper()
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected an AppError, got %v", err)
	}
	return ae.Code
}

func TestDMScreenAPI_NotWiredIs404(t *testing.T) {
	h := NewAPIHandler(nil, nil, &dmScreenRoleCampaigns{role: campaigns.RoleOwner}, nil)
	c, _ := dmScreenContext(http.MethodGet, "", synthKeySessionID)
	if got := dmStatusOf(t, h.GetDMScreen(c)); got != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", got)
	}
}

// The provider decides access, so the handler must hand it the caller's real
// role: the live membership for a browser session, Owner for a Bearer key.
func TestDMScreenAPI_PassesCallerRole(t *testing.T) {
	cases := []struct {
		name   string
		keyID  int
		member campaigns.Role
		want   int
	}{
		{"session scribe", synthKeySessionID, campaigns.RoleScribe, 2},
		{"session player", synthKeySessionID, campaigns.RolePlayer, 1},
		{"bearer key", 42, campaigns.RoleOwner, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDMScreen{}
			h := NewAPIHandler(nil, nil, &dmScreenRoleCampaigns{role: tc.member}, nil)
			h.SetDMScreen(f)
			c, rec := dmScreenContext(http.MethodGet, "", tc.keyID)
			if err := h.GetDMScreen(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || f.gotRole != tc.want || f.gotUser != "user-1" || f.gotCampaign != "camp-1" {
				t.Fatalf("code %d role %d user %q campaign %q", rec.Code, f.gotRole, f.gotUser, f.gotCampaign)
			}
		})
	}
}

func TestDMScreenAPI_Reveal(t *testing.T) {
	f := &fakeDMScreen{}
	h := NewAPIHandler(nil, nil, &dmScreenRoleCampaigns{role: campaigns.RoleScribe}, nil)
	h.SetDMScreen(f)
	c, rec := dmScreenContext(http.MethodPost, "", synthKeySessionID, "entityID", "ent-9")
	if err := h.RevealDMScreenCharacter(c); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if f.gotEntity != "ent-9" || got["name"] != "Zaltar" || got["revealed"] != true || got["id"] != "ent-9" {
		t.Fatalf("entity %q, body %v", f.gotEntity, got)
	}
}

func TestDMScreenAPI_Downtime(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantOpen   *bool
	}{
		{"open", `{"open":true}`, http.StatusOK, ptrBool(true)},
		{"close", `{"open":false}`, http.StatusOK, ptrBool(false)},
		{"missing field is refused, not read as close", `{}`, http.StatusBadRequest, nil},
		{"not json", `open`, http.StatusBadRequest, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDMScreen{}
			h := NewAPIHandler(nil, nil, &dmScreenRoleCampaigns{role: campaigns.RoleOwner}, nil)
			h.SetDMScreen(f)
			c, rec := dmScreenContext(http.MethodPost, tc.body, synthKeySessionID)
			err := h.SetDMScreenDowntime(c)
			if tc.wantStatus != http.StatusOK {
				if got := dmStatusOf(t, err); got != tc.wantStatus {
					t.Fatalf("status = %d, want %d", got, tc.wantStatus)
				}
				if f.gotOpen != nil {
					t.Fatal("provider must not be called on a bad body")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || f.gotOpen == nil || *f.gotOpen != *tc.wantOpen {
				t.Fatalf("code %d open %v", rec.Code, f.gotOpen)
			}
		})
	}
}

func ptrBool(b bool) *bool { return &b }
