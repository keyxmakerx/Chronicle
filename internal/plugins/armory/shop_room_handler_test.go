package armory

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
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeRoomSvc struct {
	getRole  int
	saves    int
	layout   json.RawMessage
	getErr   error
	saveBody []byte
}

func (f *fakeRoomSvc) GetRoom(_ context.Context, _, _ string, role int, _ string) (json.RawMessage, error) {
	f.getRole = role
	return f.layout, f.getErr
}

func (f *fakeRoomSvc) SaveRoom(_ context.Context, _, _, _ string, raw []byte) (json.RawMessage, error) {
	f.saves++
	f.saveBody = raw
	return json.RawMessage(`{"version":1}`), nil
}

func roomCtx(method, body string, role campaigns.Role) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("camp-1", "shop-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: role,
	})
	return c, rec
}

// TestShopRoomPutRoleGate pins the route gate RegisterRoutes applies to PUT:
// only the Owner reaches the handler.
func TestShopRoomPutRoleGate(t *testing.T) {
	cases := []struct {
		name      string
		role      campaigns.Role
		wantSaves int
		wantErr   bool
	}{
		{"player forbidden", campaigns.RolePlayer, 0, true},
		{"scribe forbidden", campaigns.RoleScribe, 0, true},
		{"non-member forbidden", campaigns.RoleNone, 0, true},
		{"owner allowed", campaigns.RoleOwner, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeRoomSvc{}
			h := NewShopRoomHandler(svc)
			c, rec := roomCtx(http.MethodPut, `{"a":1}`, tc.role)
			err := campaigns.RequireRole(campaigns.RoleOwner)(h.Put)(c)
			if tc.wantErr {
				var ae *apperror.AppError
				if err == nil || !asApp(err, &ae) || ae.Code != http.StatusForbidden {
					t.Fatalf("want forbidden, got %v", err)
				}
			} else if err != nil || rec.Code != http.StatusOK {
				t.Fatalf("err=%v code=%d", err, rec.Code)
			}
			if svc.saves != tc.wantSaves {
				t.Errorf("saves = %d, want %d", svc.saves, tc.wantSaves)
			}
		})
	}
}

func TestShopRoomHandler_PutResponseAndCap(t *testing.T) {
	svc := &fakeRoomSvc{}
	h := NewShopRoomHandler(svc)
	c, rec := roomCtx(http.MethodPut, `{"x":1}`, campaigns.RoleOwner)
	if err := h.Put(c); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"layout":{"version":1}}` {
		t.Errorf("body = %s", got)
	}

	// An oversize body reaches the service one byte over the cap so it is
	// rejected there rather than silently truncated.
	big := strings.Repeat("a", maxShopRoomBytes+10)
	c, _ = roomCtx(http.MethodPut, big, campaigns.RoleOwner)
	_ = h.Put(c)
	if len(svc.saveBody) != maxShopRoomBytes+1 {
		t.Errorf("service saw %d bytes", len(svc.saveBody))
	}
}

func TestShopRoomHandler_Get(t *testing.T) {
	cases := []struct {
		name     string
		layout   json.RawMessage
		getErr   error
		wantBody string
		wantCode int
	}{
		{"layout present", json.RawMessage(`{"version":1}`), nil, `{"layout":{"version":1}}`, http.StatusOK},
		{"none yet is null", nil, nil, `{"layout":null}`, http.StatusOK},
		{"hidden shop", nil, apperror.NewNotFound("shop"), "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeRoomSvc{layout: tc.layout, getErr: tc.getErr}
			c, rec := roomCtx(http.MethodGet, "", campaigns.RolePlayer)
			err := NewShopRoomHandler(svc).Get(c)
			if tc.wantCode != http.StatusOK {
				var ae *apperror.AppError
				if err == nil || !asApp(err, &ae) || ae.Code != tc.wantCode {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tc.wantBody {
				t.Errorf("body = %s", got)
			}
			if svc.getRole != permissions.RolePlayer {
				t.Errorf("role passed = %d", svc.getRole)
			}
		})
	}
}

func asApp(err error, target **apperror.AppError) bool {
	return errors.As(err, target)
}
