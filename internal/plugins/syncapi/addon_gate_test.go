package syncapi

import (
	stderrors "errors"

	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Test double ---

// fakeAddonGate stands in for the addons service. It satisfies all three
// narrow interfaces this plugin declares against it — AddonChecker (the REST
// gate), SyncAPIAddonGate (the service) and AddonEnablementStore (the
// reconciler) — because in production they are all one object and a test that
// split them could let the three disagree about what "enabled" means.
type fakeAddonGate struct {
	// enabled maps campaign ID → addon enabled. Absent means disabled.
	enabled map[string]bool
	// records maps campaign ID → "a campaign_addons row exists", enabled or
	// not. This is the distinction the reconciler turns on.
	records map[string]bool
	// err, when set, is returned by IsEnabledForCampaign (fail-closed path).
	err error

	checkCalls  []string
	enableCalls []string
}

func newFakeAddonGate() *fakeAddonGate {
	return &fakeAddonGate{enabled: map[string]bool{}, records: map[string]bool{}}
}

func (f *fakeAddonGate) IsEnabledForCampaign(_ context.Context, campaignID string, slug string) (bool, error) {
	f.checkCalls = append(f.checkCalls, campaignID+"/"+slug)
	if f.err != nil {
		return false, f.err
	}
	return f.enabled[campaignID], nil
}

func (f *fakeAddonGate) HasCampaignAddonRecord(_ context.Context, campaignID string, _ string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.records[campaignID], nil
}

func (f *fakeAddonGate) EnableForCampaignBySlug(_ context.Context, campaignID string, slug string, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.enableCalls = append(f.enableCalls, campaignID+"/"+slug)
	f.enabled[campaignID] = true
	// Enabling writes the row, so a second reconcile pass sees a decision
	// already on record — this is what makes the idempotence test real
	// rather than a restatement of the fake's own memory.
	f.records[campaignID] = true
	return nil
}

// --- Production-wiring fixture ---
//
// These tests drive RegisterAPIRoutes itself rather than hand-assembling a
// middleware chain. That is the point: the defect being fixed was not a
// broken middleware, it was a correct middleware that was never mounted on
// the group. A fixture that wires RequireSyncAPIAddon by hand would pass
// just as happily with routes.go unchanged.
//
// GET /api/v1/campaigns/:id/addons is the target because APIHandler.ListAddons
// answers 200 with an empty list when no AddonLister is injected, so the
// route is reachable end-to-end with nil collaborators and any non-200 is
// attributable to the middleware chain.

type addonGateFixture struct {
	echo   *echo.Echo
	gate   *fakeAddonGate
	rawKey string
}

// newAddonGateFixture registers the real /api/v1 routes against a single
// stored API key (id 0 is reserved for synthetic session identities, so
// keyID must be >= 1) and a session that belongs to campaignID.
func newAddonGateFixture(t *testing.T, keyID int, campaignID string) *addonGateFixture {
	t.Helper()

	rawKey := "chron_gate" + string(rune('a'+keyID%26)) + "0123456789012345678901234567890123456789012345678"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(_ context.Context, prefix string) (*APIKey, error) {
			if prefix != rawKey[:keyPrefixLen] {
				return nil, apperror.NewNotFound("key not found")
			}
			return &APIKey{
				ID:          keyID,
				KeyHash:     string(hash),
				KeyPrefix:   prefix,
				CampaignID:  campaignID,
				UserID:      "key-owner",
				Permissions: []APIKeyPermission{PermRead, PermWrite, PermSync},
				RateLimit:   60,
				IsActive:    true,
			}, nil
		},
		isIPBlockedFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
		logRequestFn:  func(_ context.Context, _ *APIRequestLog) error { return nil },
		logSecurityEventFn: func(_ context.Context, _ *SecurityEvent) error {
			return nil
		},
	}
	gate := newFakeAddonGate()
	syncSvc := NewSyncAPIService(repo)
	syncSvc.SetAddonGate(gate)

	authSvc := &fakeAuthService{
		validateSessionFn: func(_ context.Context, token string) (*auth.Session, error) {
			if token != "valid-session-token" {
				return nil, stderrors.New("invalid session")
			}
			return &auth.Session{UserID: "member-user"}, nil
		},
	}
	campSvc := &fakeCampaignService{
		getMemberFn: func(_ context.Context, cid, uid string) (*campaigns.CampaignMember, error) {
			if cid != campaignID {
				return nil, stderrors.New("not a member")
			}
			return &campaigns.CampaignMember{CampaignID: cid, UserID: uid, Role: campaigns.RoleOwner}, nil
		},
	}

	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		var appErr *apperror.AppError
		if stderrors.As(err, &appErr) {
			_ = c.JSON(appErr.Code, map[string]string{"error": appErr.Type, "message": appErr.Message})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}

	// Nil typed handlers are safe: RegisterAPIRoutes only registers their
	// method values, and the single route these tests call lives on the
	// APIHandler, which tolerates its optional collaborators being unset.
	api := NewAPIHandler(syncSvc, nil, campSvc, nil)
	RegisterAPIRoutes(e, api, nil, nil, nil, nil, nil, nil, syncSvc, gate, authSvc, campSvc)

	return &addonGateFixture{echo: e, gate: gate, rawKey: rawKey}
}

func (f *addonGateFixture) bearerGET(path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+f.rawKey)
	rec := httptest.NewRecorder()
	f.echo.ServeHTTP(rec, req)
	return rec
}

func (f *addonGateFixture) sessionGET(path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: "valid-session-token"})
	rec := httptest.NewRecorder()
	f.echo.ServeHTTP(rec, req)
	return rec
}

// errorTypeOf reads the machine-readable error classifier out of a response
// body, or "" when the body is not an error envelope.
func errorTypeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		return ""
	}
	return body.Error
}

// --- REST gate ---

// TestSyncAPIAddon_BearerRejectedWhenDisabled pins that a Bearer key must
// not reach any campaign-scoped endpoint while the campaign's "Sync API"
// toggle is off, and specifically with a 403/"sync_api_disabled" — not a
// 404, which Chronicle's own Foundry module would read as "too old to have
// that endpoint" and hide the real cause.
func TestSyncAPIAddon_BearerRejectedWhenDisabled(t *testing.T) {
	f := newAddonGateFixture(t, 101, "camp-1")
	f.gate.enabled["camp-1"] = false

	rec := f.bearerGET("/api/v1/campaigns/camp-1/addons")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("bearer GET with addon disabled: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if got := errorTypeOf(t, rec); got != "sync_api_disabled" {
		t.Errorf("error type = %q, want %q; body = %s", got, "sync_api_disabled", rec.Body.String())
	}
}

// TestSyncAPIAddon_BearerAllowedWhenEnabled is the other half of the toggle:
// turning it on must leave the API exactly as it was. Without this, "reject
// everything" would satisfy the test above.
func TestSyncAPIAddon_BearerAllowedWhenEnabled(t *testing.T) {
	f := newAddonGateFixture(t, 102, "camp-1")
	f.gate.enabled["camp-1"] = true

	rec := f.bearerGET("/api/v1/campaigns/camp-1/addons")

	if rec.Code != http.StatusOK {
		t.Fatalf("bearer GET with addon enabled: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

// TestSyncAPIAddon_SessionCallerUnaffected pins that session-authed callers
// (synthetic APIKey, ID == synthKeySessionID — e.g. layout_editor.js reading
// /entity-types and /maps) are unaffected by the "Sync API" toggle, since
// that toggle governs outside access, not Chronicle's own UI. The gate must
// not even ASK the addons service about a session caller, asserted below.
func TestSyncAPIAddon_SessionCallerUnaffected(t *testing.T) {
	cases := []struct {
		name    string
		addonOn bool
	}{
		{"addon disabled", false},
		{"addon enabled", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAddonGateFixture(t, 103, "camp-1")
			f.gate.enabled["camp-1"] = tc.addonOn

			rec := f.sessionGET("/api/v1/campaigns/camp-1/addons")

			if rec.Code != http.StatusOK {
				t.Fatalf("session GET (addon enabled=%v): status = %d, want 200; body = %s",
					tc.addonOn, rec.Code, rec.Body.String())
			}
			if len(f.gate.checkCalls) != 0 {
				t.Errorf("session GET consulted the addon gate %v; a synthetic session key must short-circuit",
					f.gate.checkCalls)
			}
		})
	}
}

// TestSyncAPIAddon_FailsClosedOnCheckError: an unreadable addon state is not
// permission. Mirrors RequireAddonAPI's existing fail-closed choice.
func TestSyncAPIAddon_FailsClosedOnCheckError(t *testing.T) {
	f := newAddonGateFixture(t, 104, "camp-1")
	f.gate.enabled["camp-1"] = true
	f.gate.err = stderrors.New("db unavailable")

	rec := f.bearerGET("/api/v1/campaigns/camp-1/addons")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("bearer GET with unreadable addon state: status = %d, want 503; body = %s",
			rec.Code, rec.Body.String())
	}
}

// --- WebSocket gate ---

// wsGateService builds a service whose single stored key authenticates
// rawKey, with the supplied gate attached (or none when gate is nil). member
// wires a MembershipChecker so AuthenticateKeyForWS's owner-still-owner check
// can pass; nil leaves it unwired (fail-closed), matching the addon gate.
func wsGateService(t *testing.T, gate SyncAPIAddonGate, member MembershipChecker) (SyncAPIService, string) {
	t.Helper()
	rawKey := "chron_wsgate0123456789012345678901234567890123456789012345678901"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(_ context.Context, prefix string) (*APIKey, error) {
			if prefix != rawKey[:keyPrefixLen] {
				return nil, apperror.NewNotFound("key not found")
			}
			return &APIKey{
				ID: 77, KeyHash: string(hash), KeyPrefix: prefix,
				CampaignID: "camp-ws", UserID: "ws-owner", IsActive: true,
			}, nil
		},
	}
	svc := NewSyncAPIService(repo)
	if gate != nil {
		svc.SetAddonGate(gate)
	}
	if member != nil {
		svc.SetMemberChecker(member)
	}
	return svc, rawKey
}

// wsOwnerMember is a MembershipChecker that always reports the caller as an
// Owner of "camp-ws" — the passing case for the owner-still-owner check.
var wsOwnerMember = &fakeCampaignService{
	getMemberFn: func(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
		return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: campaigns.RoleOwner}, nil
	},
}

// TestSyncAPIAddon_WebSocketRefusedWhenDisabled pins that the WS upgrade
// (internal/websocket/auth.go calls AuthenticateKeyForWS directly, bypassing
// the REST middleware chain) also refuses a key for a campaign with the
// Sync API addon disabled.
func TestSyncAPIAddon_WebSocketRefusedWhenDisabled(t *testing.T) {
	gate := newFakeAddonGate()
	gate.enabled["camp-ws"] = false
	svc, rawKey := wsGateService(t, gate, wsOwnerMember)

	_, _, _, _, err := svc.AuthenticateKeyForWS(context.Background(), rawKey)
	if err == nil {
		t.Fatal("AuthenticateKeyForWS accepted a key for a campaign with the Sync API addon disabled")
	}
	var appErr *apperror.AppError
	if !stderrors.As(err, &appErr) || appErr.Type != "sync_api_disabled" {
		t.Errorf("error = %v, want an AppError of type sync_api_disabled", err)
	}
}

// TestSyncAPIAddon_WebSocketAllowedWhenEnabled keeps the enabled path intact,
// identity and role unchanged.
func TestSyncAPIAddon_WebSocketAllowedWhenEnabled(t *testing.T) {
	gate := newFakeAddonGate()
	gate.enabled["camp-ws"] = true
	svc, rawKey := wsGateService(t, gate, wsOwnerMember)

	campaignID, userID, role, _, err := svc.AuthenticateKeyForWS(context.Background(), rawKey)
	if err != nil {
		t.Fatalf("AuthenticateKeyForWS with addon enabled: %v", err)
	}
	if campaignID != "camp-ws" || userID != "ws-owner" || role == 0 {
		t.Errorf("got (%q, %q, %d), want (camp-ws, ws-owner, nonzero)", campaignID, userID, role)
	}
}

// TestSyncAPIAddon_WebSocketRefusedWhenGateUnwired pins that an unwired gate
// refuses rather than silently assuming permission.
func TestSyncAPIAddon_WebSocketRefusedWhenGateUnwired(t *testing.T) {
	svc, rawKey := wsGateService(t, nil, wsOwnerMember)

	if _, _, _, _, err := svc.AuthenticateKeyForWS(context.Background(), rawKey); err == nil {
		t.Fatal("AuthenticateKeyForWS accepted a key with no addon gate wired")
	}
}

// --- CreateKey ---

// TestCreateKey_EnablesSyncAPIAddon pins that creating a key immediately
// enables the campaign's Sync API addon rather than leaving the new key dead
// until the next reconciler pass. This deliberately re-enables a toggle the
// owner may have switched off — they're on the API keys screen asking for a
// credential at that moment.
func TestCreateKey_EnablesSyncAPIAddon(t *testing.T) {
	repo := &mockSyncAPIRepo{
		createKeyFn: func(_ context.Context, key *APIKey) error {
			key.ID = 5
			return nil
		},
	}
	gate := newFakeAddonGate()
	gate.enabled["camp-new"] = false
	svc := NewSyncAPIService(repo)
	svc.SetAddonGate(gate)

	result, err := svc.CreateKey(context.Background(), "owner-1", CreateAPIKeyInput{
		Name:        "Foundry",
		CampaignID:  "camp-new",
		Permissions: []APIKeyPermission{PermRead, PermWrite, PermSync},
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if result == nil || result.RawKey == "" {
		t.Fatal("CreateKey returned no key")
	}
	want := []string{"camp-new/" + SyncAPIAddonSlug}
	if !stringsEqual(gate.enableCalls, want) {
		t.Errorf("enable calls = %v, want %v", gate.enableCalls, want)
	}
	if !gate.enabled["camp-new"] {
		t.Error("sync-api addon still disabled after creating an API key for the campaign")
	}
}

// TestCreateKey_SurvivesAddonEnableFailure: the key row is already committed
// and its plaintext is shown exactly once, so a failure to record the addon
// state must not destroy a credential the caller cannot recover. The failure
// is logged at ERROR with the manual remedy instead.
func TestCreateKey_SurvivesAddonEnableFailure(t *testing.T) {
	repo := &mockSyncAPIRepo{
		createKeyFn: func(_ context.Context, key *APIKey) error { key.ID = 6; return nil },
	}
	gate := newFakeAddonGate()
	gate.err = stderrors.New("campaign_addons unavailable")
	svc := NewSyncAPIService(repo)
	svc.SetAddonGate(gate)

	result, err := svc.CreateKey(context.Background(), "owner-1", CreateAPIKeyInput{
		Name:        "Foundry",
		CampaignID:  "camp-new",
		Permissions: []APIKeyPermission{PermRead},
	})
	if err != nil {
		t.Fatalf("CreateKey should not fail when the addon enable fails: %v", err)
	}
	if result == nil || result.RawKey == "" {
		t.Fatal("CreateKey returned no key")
	}
}
