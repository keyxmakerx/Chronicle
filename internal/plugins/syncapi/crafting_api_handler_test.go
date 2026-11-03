package syncapi

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeCraftingSvc records what the handler hands the service.
type fakeCraftingSvc struct {
	calls []string
	body  string
}

func (f *fakeCraftingSvc) rec(s string, body json.RawMessage) (any, error) {
	f.calls = append(f.calls, s)
	f.body = string(body)
	return map[string]string{"call": s}, nil
}
func (f *fakeCraftingSvc) Character(_ context.Context, cid, key, acting, char string) (any, error) {
	return f.rec("character:"+cid+":"+key+":"+acting+":"+char, nil)
}
func (f *fakeCraftingSvc) StartProject(_ context.Context, _, _, acting, char string, body json.RawMessage) (any, error) {
	return f.rec("start:"+acting+":"+char, body)
}
func (f *fakeCraftingSvc) RollProject(_ context.Context, _, _, acting, pid string, body json.RawMessage) (any, error) {
	return f.rec("roll:"+acting+":"+pid, body)
}
func (f *fakeCraftingSvc) CancelProject(_ context.Context, _, _, acting, pid string) error {
	_, err := f.rec("cancel:"+acting+":"+pid, nil)
	return err
}
func (f *fakeCraftingSvc) Spoils(_ context.Context, _, _, acting string) (any, error) {
	return f.rec("spoils:"+acting, nil)
}
func (f *fakeCraftingSvc) TakeSpoil(_ context.Context, _, _, acting string, id int64, body json.RawMessage) (any, error) {
	return f.rec("take:"+acting+":"+strconv.FormatInt(id, 10), body)
}
func (f *fakeCraftingSvc) StudySpoil(_ context.Context, _, _, acting string, id int64, body json.RawMessage) (any, error) {
	return f.rec("study:"+acting+":"+strconv.FormatInt(id, 10), body)
}
func (f *fakeCraftingSvc) Experiment(_ context.Context, _, _, acting, char string, body json.RawMessage) (any, error) {
	return f.rec("experiment:"+acting+":"+char, body)
}
func (f *fakeCraftingSvc) BreakDown(_ context.Context, _, _, acting, char string, body json.RawMessage) (any, error) {
	return f.rec("break:"+acting+":"+char, body)
}
func (f *fakeCraftingSvc) Help(_ context.Context, _, _, acting, pid string, body json.RawMessage) (any, error) {
	return f.rec("help:"+acting+":"+pid, body)
}
func (f *fakeCraftingSvc) FightEnded(_ context.Context, _, _, acting string, body json.RawMessage) (any, error) {
	return f.rec("fight:"+acting, body)
}

// newCraftingRouteFixture drives RegisterAPIRoutes with the crafting routes
// mounted, so the real middleware chain (key, permission, addon gate) runs.
func newCraftingRouteFixture(t *testing.T, perms []APIKeyPermission, addons slugChecker) (*echo.Echo, *fakeCraftingSvc, string) {
	t.Helper()
	const campaignID = "camp-1"
	rawKey := "chron_craft0123456789012345678901234567890123456789012345678901"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(_ context.Context, prefix string) (*APIKey, error) {
			if prefix != rawKey[:keyPrefixLen] {
				return nil, apperror.NewNotFound("key not found")
			}
			return &APIKey{ID: 8, KeyHash: string(hash), KeyPrefix: prefix, CampaignID: campaignID,
				UserID: "gm-user", Permissions: perms, RateLimit: 60, IsActive: true}, nil
		},
		isIPBlockedFn:      func(context.Context, string) (bool, error) { return false, nil },
		logRequestFn:       func(context.Context, *APIRequestLog) error { return nil },
		logSecurityEventFn: func(context.Context, *SecurityEvent) error { return nil },
	}
	syncSvc := NewSyncAPIService(repo)
	campSvc := &fakeCampaignService{getMemberFn: func(_ context.Context, cid, uid string) (*campaigns.CampaignMember, error) {
		return &campaigns.CampaignMember{CampaignID: cid, UserID: uid, Role: campaigns.RoleOwner}, nil
	}, getByIDFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
		return &campaigns.Campaign{ID: id}, nil
	}}
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		var ae *apperror.AppError
		if stderrors.As(err, &ae) {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
	svc := &fakeCraftingSvc{}
	api := NewAPIHandler(syncSvc, nil, campSvc, nil)
	RegisterAPIRoutes(e, api, nil, nil, nil, nil, nil, nil, nil, nil, syncSvc, addons, &fakeAuthService{}, campSvc,
		WithCrafting(NewCraftingAPIHandler(svc, "craft-addon")))
	return e, svc, rawKey
}

func craftingCall(e *echo.Echo, rawKey, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

const craftBase = "/api/v1/campaigns/camp-1/crafting"

func craftAddons() slugChecker { return slugChecker{SyncAPIAddonSlug: true, "craft-addon": true} }

// Each route hands the service the key holder, the member it names and the
// body as sent; a bad line id or body never reaches the service.
func TestCraftingRoutes_HandlerContract(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
		wantCall string
	}{
		{"a hero's crafting as a named player", "GET", craftBase + "/characters/c1?actingUserId=p1", "", 200, "character:camp-1:gm-user:p1:c1"},
		{"a hero's crafting as the key holder", "GET", craftBase + "/characters/c1", "", 200, "character:camp-1:gm-user::c1"},
		{"start a project", "POST", craftBase + "/characters/c1/projects", `{"actingUserId":"p1","recipeId":"r1"}`, 201, "start:p1:c1"},
		{"roll a project", "POST", craftBase + "/projects/pj1/roll", `{"actingUserId":"p1","work":"clean"}`, 200, "roll:p1:pj1"},
		{"set a project aside", "DELETE", craftBase + "/projects/pj1?actingUserId=p1", "", 204, "cancel:p1:pj1"},
		{"the Director's tray", "GET", craftBase + "/spoils", "", 200, "spoils:"},
		{"take a line", "POST", craftBase + "/spoil-lines/12/take", `{"actingUserId":"p1","characterId":"c1"}`, 200, "take:p1:12"},
		{"study a line", "POST", craftBase + "/spoil-lines/12/study", `{"actingUserId":"p1","characterId":"c1"}`, 200, "study:p1:12"},
		{"try something new", "POST", craftBase + "/characters/c1/experiment", `{"actingUserId":"p1","items":[{"itemId":"i1","qty":2}]}`, 200, "experiment:p1:c1"},
		{"break an item down", "POST", craftBase + "/characters/c1/break-down", `{"actingUserId":"p1","recipeId":"r1","copy":4}`, 200, "break:p1:c1"},
		{"help a party member", "POST", craftBase + "/projects/pj1/help", `{"actingUserId":"p1","characterId":"c1","version":3}`, 200, "help:p1:pj1"},
		{"a fight the GM confirmed", "POST", craftBase + "/fights", `{"name":"Bog","creatures":[{"pageId":"pg","down":2}]}`, 200, "fight:"},
		{"an empty body is an empty object", "POST", craftBase + "/spoil-lines/12/take", ``, 200, "take::12"},
		{"a line id that isn't a number", "POST", craftBase + "/spoil-lines/abc/take", `{}`, 404, ""},
		{"a line id below one", "POST", craftBase + "/spoil-lines/0/study", `{}`, 404, ""},
		{"malformed json", "POST", craftBase + "/fights", `{`, 400, ""},
		{"a body over the cap", "POST", craftBase + "/fights", `{"name":"` + strings.Repeat("x", maxCraftingBodyBytes) + `"}`, 400, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, svc, key := newCraftingRouteFixture(t, allPerms, craftAddons())
			rec := craftingCall(e, key, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
			if tt.wantCall == "" {
				if len(svc.calls) != 0 {
					t.Fatalf("service was called: %v", svc.calls)
				}
				return
			}
			if len(svc.calls) != 1 || svc.calls[0] != tt.wantCall {
				t.Fatalf("calls = %v, want %q", svc.calls, tt.wantCall)
			}
			if tt.body != "" && tt.method == "POST" && svc.body != tt.body {
				t.Fatalf("body = %q, want it as sent", svc.body)
			}
		})
	}
}

// A read key reads but never writes, and the crafting addon gates every
// route.
func TestCraftingRoutes_Gates(t *testing.T) {
	readOnly := []APIKeyPermission{PermRead}
	off := slugChecker{SyncAPIAddonSlug: true}
	tests := []struct {
		name     string
		perms    []APIKeyPermission
		addons   slugChecker
		method   string
		path     string
		body     string
		wantCode int
	}{
		{"read key reads a hero", readOnly, craftAddons(), "GET", craftBase + "/characters/c1", "", 200},
		{"read key reads the tray", readOnly, craftAddons(), "GET", craftBase + "/spoils", "", 200},
		{"read key cannot take", readOnly, craftAddons(), "POST", craftBase + "/spoil-lines/1/take", `{}`, 403},
		{"read key cannot study", readOnly, craftAddons(), "POST", craftBase + "/spoil-lines/1/study", `{}`, 403},
		{"read key cannot start", readOnly, craftAddons(), "POST", craftBase + "/characters/c1/projects", `{}`, 403},
		{"read key cannot roll", readOnly, craftAddons(), "POST", craftBase + "/projects/p/roll", `{}`, 403},
		{"read key cannot set aside", readOnly, craftAddons(), "DELETE", craftBase + "/projects/p", "", 403},
		{"read key cannot experiment", readOnly, craftAddons(), "POST", craftBase + "/characters/c1/experiment", `{}`, 403},
		{"read key cannot break down", readOnly, craftAddons(), "POST", craftBase + "/characters/c1/break-down", `{}`, 403},
		{"read key cannot help", readOnly, craftAddons(), "POST", craftBase + "/projects/p/help", `{}`, 403},
		{"crafting off refuses a hero's turn", allPerms, off, "POST", craftBase + "/projects/p/help", `{}`, 403},
		{"read key cannot end a fight", readOnly, craftAddons(), "POST", craftBase + "/fights", `{}`, 403},
		{"crafting off refuses reads", allPerms, off, "GET", craftBase + "/characters/c1", "", 403},
		{"crafting off refuses writes", allPerms, off, "POST", craftBase + "/fights", `{}`, 403},
		{"another campaign is refused", allPerms, craftAddons(), "GET", "/api/v1/campaigns/other/crafting/spoils", "", 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, svc, key := newCraftingRouteFixture(t, tt.perms, tt.addons)
			rec := craftingCall(e, key, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
			if tt.wantCode != 200 && len(svc.calls) != 0 {
				t.Fatalf("service reached despite %d: %v", rec.Code, svc.calls)
			}
		})
	}
}
