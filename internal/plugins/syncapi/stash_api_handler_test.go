package syncapi

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// fakeStashSvc records what the handler hands the service.
type fakeStashSvc struct {
	calls   []string
	move    StashMoveRequest
	moveID  int64
	open    bool
	ctxSrc  changesource.Source
	hasSrc  bool
	failErr error
}

func (f *fakeStashSvc) rec(ctx context.Context, s string) (any, error) {
	f.calls = append(f.calls, s)
	f.ctxSrc, f.hasSrc = changesource.From(ctx)
	if f.failErr != nil {
		return nil, f.failErr
	}
	return map[string]string{"call": s}, nil
}
func (f *fakeStashSvc) View(ctx context.Context, cid, key, acting, char string) (any, error) {
	return f.rec(ctx, "view:"+cid+":"+key+":"+acting+":"+char)
}
func (f *fakeStashSvc) Move(ctx context.Context, cid, key string, r StashMoveRequest) (any, error) {
	f.move = r
	return f.rec(ctx, "move:"+cid+":"+key)
}
func (f *fakeStashSvc) History(ctx context.Context, cid, key, acting, char, stash string) (any, error) {
	return f.rec(ctx, "history:"+acting+":"+char+":"+stash)
}
func (f *fakeStashSvc) Requests(ctx context.Context, cid, key, acting string) (any, error) {
	return f.rec(ctx, "requests:"+acting)
}
func (f *fakeStashSvc) Approve(ctx context.Context, cid, key, acting string, id int64) (any, error) {
	f.moveID = id
	return f.rec(ctx, "approve:"+acting)
}
func (f *fakeStashSvc) Decline(ctx context.Context, cid, key, acting string, id int64) (any, error) {
	f.moveID = id
	return f.rec(ctx, "decline:"+acting)
}
func (f *fakeStashSvc) Downtime(ctx context.Context, cid string) (any, error) {
	return f.rec(ctx, "downtime")
}
func (f *fakeStashSvc) SetDowntime(ctx context.Context, cid, key, acting string, open bool) (any, error) {
	f.open = open
	return f.rec(ctx, "setdowntime:"+acting)
}

// slugChecker enables addons by slug, unlike the all-or-nothing gate fake.
type slugChecker map[string]bool

func (s slugChecker) IsEnabledForCampaign(_ context.Context, _ string, slug string) (bool, error) {
	return s[slug], nil
}

type stashRouteFixture struct {
	echo   *echo.Echo
	svc    *fakeStashSvc
	rawKey string
}

// newStashRouteFixture drives RegisterAPIRoutes itself, so the test covers the
// real middleware chain (auth, permission, addon gate, change source) and not a
// hand-built copy of it.
func newStashRouteFixture(t *testing.T, perms []APIKeyPermission, addons slugChecker) *stashRouteFixture {
	t.Helper()
	const campaignID = "camp-1"
	rawKey := "chron_stash0123456789012345678901234567890123456789012345678901"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(_ context.Context, prefix string) (*APIKey, error) {
			if prefix != rawKey[:keyPrefixLen] {
				return nil, apperror.NewNotFound("key not found")
			}
			return &APIKey{ID: 7, KeyHash: string(hash), KeyPrefix: prefix, CampaignID: campaignID,
				UserID: "gm-user", Permissions: perms, RateLimit: 60, IsActive: true}, nil
		},
		isIPBlockedFn:      func(context.Context, string) (bool, error) { return false, nil },
		logRequestFn:       func(context.Context, *APIRequestLog) error { return nil },
		logSecurityEventFn: func(context.Context, *SecurityEvent) error { return nil },
	}
	syncSvc := NewSyncAPIService(repo)
	authSvc := &fakeAuthService{validateSessionFn: func(_ context.Context, token string) (*auth.Session, error) {
		if token != "sess" {
			return nil, stderrors.New("invalid session")
		}
		return &auth.Session{UserID: "member-user"}, nil
	}}
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
	svc := &fakeStashSvc{}
	api := NewAPIHandler(syncSvc, nil, campSvc, nil)
	RegisterAPIRoutes(e, api, nil, nil, nil, nil, nil, nil, nil, NewStashAPIHandler(svc, "stash-addon"), syncSvc, addons, authSvc, campSvc)
	return &stashRouteFixture{echo: e, svc: svc, rawKey: rawKey}
}

func (f *stashRouteFixture) do(method, path, body string, session bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && body == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if session {
		req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: "sess"})
	} else {
		req.Header.Set("Authorization", "Bearer "+f.rawKey)
	}
	rec := httptest.NewRecorder()
	f.echo.ServeHTTP(rec, req)
	return rec
}

var allPerms = []APIKeyPermission{PermRead, PermWrite, PermSync}

func allAddons() slugChecker { return slugChecker{SyncAPIAddonSlug: true, "stash-addon": true} }

const base = "/api/v1/campaigns/camp-1/stashes"

func TestStashRoutes_HandlerContract(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
		wantCall string
	}{
		{"view passes key holder and acting member", "GET", base + "/view?characterId=c1&actingUserId=u1", "", 200, "view:camp-1:gm-user:u1:c1"},
		{"view without actingUserId", "GET", base + "/view?characterId=c1", "", 200, "view:camp-1:gm-user::c1"},
		{"view needs a character", "GET", base + "/view", "", 400, ""},
		{"history by stash", "GET", base + "/history?stashId=3&actingUserId=u1", "", 200, "history:u1::3"},
		{"requests", "GET", base + "/requests?actingUserId=u1", "", 200, "requests:u1"},
		{"downtime read", "GET", base + "/downtime", "", 200, "downtime"},
		{"approve with body", "POST", base + "/requests/12/approve", `{"actingUserId":"u9"}`, 200, "approve:u9"},
		{"decline with an empty body", "POST", base + "/requests/12/decline", ``, 200, "decline:"},
		{"approve with a bad id", "POST", base + "/requests/abc/approve", `{}`, 404, ""},
		{"approve with malformed json", "POST", base + "/requests/12/approve", `{`, 400, ""},
		{"set downtime", "PUT", base + "/downtime", `{"open":true,"actingUserId":"u1"}`, 200, "setdowntime:u1"},
		{"set downtime needs open", "PUT", base + "/downtime", `{}`, 400, ""},
		{"move", "POST", base + "/moves", `{"kind":"item"}`, 200, "move:camp-1:gm-user"},
		{"move with malformed json", "POST", base + "/moves", `nope`, 400, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newStashRouteFixture(t, allPerms, allAddons())
			rec := f.do(tt.method, tt.path, tt.body, false)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
			if tt.wantCall == "" {
				if len(f.svc.calls) != 0 {
					t.Fatalf("service was called: %v", f.svc.calls)
				}
				return
			}
			if len(f.svc.calls) != 1 || f.svc.calls[0] != tt.wantCall {
				t.Fatalf("calls = %v, want %q", f.svc.calls, tt.wantCall)
			}
		})
	}
}

func TestStashRoutes_MoveBodyDecoding(t *testing.T) {
	tests := []struct {
		name string
		body string
		want StashMoveRequest
	}{
		{"numeric stash id and numeric amount",
			`{"actingUserId":"u1","kind":"money","amount":12.5,"from":{"kind":"character","id":"c1"},"to":{"kind":"stash","id":3}}`,
			StashMoveRequest{ActingUserID: "u1", Kind: "money", Amount: "12.5", From: StashEndpoint{"character", "c1"}, To: StashEndpoint{"stash", "3"}}},
		{"string id and string amount",
			`{"kind":"money","amount":"7.25","from":{"kind":"stash","id":"3"},"to":{"kind":"character","id":"c2"}}`,
			StashMoveRequest{Kind: "money", Amount: "7.25", From: StashEndpoint{"stash", "3"}, To: StashEndpoint{"character", "c2"}}},
		{"item with quantity",
			`{"kind":"item","itemId":"i1","quantity":2,"from":{"kind":"character","id":"c1"},"to":{"kind":"stash","id":"3"}}`,
			StashMoveRequest{Kind: "item", ItemID: "i1", Quantity: 2, From: StashEndpoint{"character", "c1"}, To: StashEndpoint{"stash", "3"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newStashRouteFixture(t, allPerms, allAddons())
			if rec := f.do("POST", base+"/moves", tt.body, false); rec.Code != 200 {
				t.Fatalf("code = %d (%s)", rec.Code, rec.Body.String())
			}
			if f.svc.move != tt.want {
				t.Fatalf("decoded = %+v, want %+v", f.svc.move, tt.want)
			}
		})
	}
}

func TestStashRoutes_Gates(t *testing.T) {
	readOnly := []APIKeyPermission{PermRead}
	tests := []struct {
		name     string
		perms    []APIKeyPermission
		addons   slugChecker
		method   string
		path     string
		body     string
		wantCode int
	}{
		{"read key can read", readOnly, allAddons(), "GET", base + "/downtime", "", 200},
		{"read key cannot move", readOnly, allAddons(), "POST", base + "/moves", `{}`, 403},
		{"read key cannot approve", readOnly, allAddons(), "POST", base + "/requests/1/approve", `{}`, 403},
		{"read key cannot flip downtime", readOnly, allAddons(), "PUT", base + "/downtime", `{"open":true}`, 403},
		{"stash addon off is addon_disabled", allPerms, slugChecker{SyncAPIAddonSlug: true}, "GET", base + "/downtime", "", 403},
		{"stash addon off refuses writes too", allPerms, slugChecker{SyncAPIAddonSlug: true}, "POST", base + "/moves", `{}`, 403},
		{"sync api off is its own error", allPerms, slugChecker{"stash-addon": true}, "GET", base + "/downtime", "", 403},
		{"other campaign is refused", allPerms, allAddons(), "GET", "/api/v1/campaigns/other/stashes/downtime", "", 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newStashRouteFixture(t, tt.perms, tt.addons)
			rec := f.do(tt.method, tt.path, tt.body, false)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
			if tt.wantCode != 200 && len(f.svc.calls) != 0 {
				t.Fatalf("service reached despite %d: %v", rec.Code, f.svc.calls)
			}
		})
	}
}

func TestStashRoutes_ErrorsUseTheAPIShape(t *testing.T) {
	f := newStashRouteFixture(t, allPerms, allAddons())
	f.svc.failErr = apperror.NewNotFound("member not found")
	rec := f.do("GET", base+"/view?characterId=c1&actingUserId=stranger", "", false)
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != 404 || body["error"] == "" {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

// Writes made through the sync API carry their origin: a Bearer key is the
// external sync client, a browser session on the same routes is the web UI.
func TestStashRoutes_ChangeSourceOnContext(t *testing.T) {
	tests := []struct {
		name    string
		session bool
		want    changesource.Source
	}{
		{"bearer key", false, changesource.Source{Kind: changesource.KindFoundry, UserID: "gm-user"}},
		{"browser session", true, changesource.Source{Kind: changesource.KindWeb, UserID: "member-user"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newStashRouteFixture(t, allPerms, allAddons())
			if rec := f.do("GET", base+"/downtime", "", tt.session); rec.Code != 200 {
				t.Fatalf("code = %d (%s)", rec.Code, rec.Body.String())
			}
			if !f.svc.hasSrc || f.svc.ctxSrc != tt.want {
				t.Fatalf("source = %+v (set=%v), want %+v", f.svc.ctxSrc, f.svc.hasSrc, tt.want)
			}
		})
	}
}

func TestClassifyChange_StashAndDowntime(t *testing.T) {
	tests := []struct {
		typ      string
		wantType string
		wantOp   string
		wantOK   bool
	}{
		{"stash.moved", "stash", "updated", true},
		{"stash.requested", "stash", "created", true},
		{"stash.settled", "stash", "updated", true},
		{"stash.money_changed", "stash", "updated", true},
		{"downtime.changed", "downtime", "updated", true},
		{"stash.unknown", "", "", false},
		{"entity_note.updated", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			rt, op, ok := classifyChange(ws.MessageType(tt.typ))
			if rt != tt.wantType || op != tt.wantOp || ok != tt.wantOK {
				t.Fatalf("got %q %q %v", rt, op, ok)
			}
		})
	}
}

// A browser session must reach the service as its own user, never as the key
// it rides on, so the armory's rule that only a GM may name someone else holds.
func TestStashRoutes_SessionCallerIsTheSessionUser(t *testing.T) {
	f := newStashRouteFixture(t, allPerms, allAddons())
	if rec := f.do("GET", base+"/view?characterId=c1&actingUserId=gm-user", "", true); rec.Code != 200 {
		t.Fatalf("code = %d (%s)", rec.Code, rec.Body.String())
	}
	if len(f.svc.calls) != 1 || !strings.Contains(f.svc.calls[0], ":member-user:gm-user:") {
		t.Fatalf("calls = %v, want the session user as caller", f.svc.calls)
	}
}
