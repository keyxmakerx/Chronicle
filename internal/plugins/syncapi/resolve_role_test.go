package syncapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- stubs for resolveRole / §1B (sync-key role decoupling) tests ---

// stubCampaignSvcForRole embeds campaigns.CampaignService; only GetMember is
// reachable from resolveRole. getMemberFn supplies the (member, err) pair so a
// test can simulate a still-owner, a demoted owner, or a removed member.
type stubCampaignSvcForRole struct {
	campaigns.CampaignService
	getMemberFn func(ctx context.Context, campaignID, userID string) (*campaigns.CampaignMember, error)
}

func (s *stubCampaignSvcForRole) GetMember(ctx context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	return s.getMemberFn(ctx, campaignID, userID)
}

// stubSyncSvcForRole embeds SyncAPIService; only LogSecurityEvent is reachable
// from resolveRole's degrade path. Captured events let tests assert the loud
// signal fired with the right reason.
type stubSyncSvcForRole struct {
	SyncAPIService
	events []*SecurityEvent
}

func (s *stubSyncSvcForRole) LogSecurityEvent(_ context.Context, e *SecurityEvent) error {
	s.events = append(s.events, e)
	return nil
}

// newRoleContext builds an Echo context carrying the given API key (nil sets
// none) for resolveRole, with a fresh ResponseRecorder to inspect headers.
func newRoleContext(key *APIKey) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/entities", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	if key != nil {
		c.Set(apiKeyContextKey, key)
	}
	return c, rec
}

// resetDegradeThrottle clears the package-level throttle so the loud-signal
// assertions are hermetic across tests / cases.
func resetDegradeThrottle() {
	degradeSignalThrottle.Range(func(k, _ any) bool {
		degradeSignalThrottle.Delete(k)
		return true
	})
}

func memberWithRole(role campaigns.Role) func(context.Context, string, string) (*campaigns.CampaignMember, error) {
	return func(_ context.Context, _, _ string) (*campaigns.CampaignMember, error) {
		return &campaigns.CampaignMember{Role: role}, nil
	}
}

func memberRemoved(_ context.Context, _, _ string) (*campaigns.CampaignMember, error) {
	return nil, errors.New("not a member")
}

// TestResolveRole_BearerKeyDecoupledFromLiveMembership is the §1B fix: a stored
// Bearer key resolves to Owner-level sync visibility regardless of whether its
// creator is still an Owner — so an ownership transfer or member removal can no
// longer silently strip private/custom entities from the sync. The lost-access
// condition is surfaced loudly (response header + security event) instead.
func TestResolveRole_BearerKeyDecoupledFromLiveMembership(t *testing.T) {
	resetDegradeThrottle()

	cases := []struct {
		name         string
		keyID        int // distinct per case so the per-key throttle never collides
		getMember    func(context.Context, string, string) (*campaigns.CampaignMember, error)
		wantRole     int
		wantDegraded bool
		wantReason   string
	}{
		{
			name:      "normally owned key — full visibility, no signal",
			keyID:     11,
			getMember: memberWithRole(campaigns.RoleOwner),
			wantRole:  int(campaigns.RoleOwner),
		},
		{
			name:         "transferred owner now Scribe — still full visibility, loud signal",
			keyID:        12,
			getMember:    memberWithRole(campaigns.RoleScribe),
			wantRole:     int(campaigns.RoleOwner),
			wantDegraded: true,
			wantReason:   "owner_demoted",
		},
		{
			name:         "creator removed — still full visibility, loud signal",
			keyID:        13,
			getMember:    memberRemoved,
			wantRole:     int(campaigns.RoleOwner),
			wantDegraded: true,
			wantReason:   "owner_removed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			syncSvc := &stubSyncSvcForRole{}
			campSvc := &stubCampaignSvcForRole{getMemberFn: tc.getMember}
			h := NewAPIHandler(syncSvc, nil, campSvc, nil)

			key := &APIKey{ID: tc.keyID, CampaignID: "camp-1", UserID: "creator-1", IsActive: true}
			c, rec := newRoleContext(key)

			got := h.resolveRole(c)
			if got != tc.wantRole {
				t.Errorf("role = %d, want %d (a degraded key must NOT lose sync visibility)", got, tc.wantRole)
			}

			gotDegraded := rec.Header().Get(keyOwnerDegradedHeader) == "1"
			if gotDegraded != tc.wantDegraded {
				t.Errorf("degrade header set = %v, want %v", gotDegraded, tc.wantDegraded)
			}

			if tc.wantDegraded {
				if len(syncSvc.events) != 1 {
					t.Fatalf("want 1 security event, got %d", len(syncSvc.events))
				}
				ev := syncSvc.events[0]
				if ev.EventType != EventKeyOwnerDegraded {
					t.Errorf("event type = %q, want %q", ev.EventType, EventKeyOwnerDegraded)
				}
				if ev.APIKeyID == nil || *ev.APIKeyID != tc.keyID {
					t.Errorf("event APIKeyID = %v, want %d", ev.APIKeyID, tc.keyID)
				}
				if ev.Details["reason"] != tc.wantReason {
					t.Errorf("reason = %v, want %q", ev.Details["reason"], tc.wantReason)
				}
			} else if len(syncSvc.events) != 0 {
				t.Errorf("want no security event for a normally-owned key, got %d", len(syncSvc.events))
			}
		})
	}
}

// TestResolveRole_SessionCallerKeepsLiveRole pins that session-authed callers
// (synthetic key, ID == synthKeySessionID) are unchanged by the §1B fix: their
// role still tracks live membership and no degrade signal is emitted.
func TestResolveRole_SessionCallerKeepsLiveRole(t *testing.T) {
	resetDegradeThrottle()

	cases := []struct {
		name      string
		getMember func(context.Context, string, string) (*campaigns.CampaignMember, error)
		wantRole  int
	}{
		{"session scribe stays scribe", memberWithRole(campaigns.RoleScribe), int(campaigns.RoleScribe)},
		{"session player stays player", memberWithRole(campaigns.RolePlayer), int(campaigns.RolePlayer)},
		{"session non-member resolves to none", memberRemoved, int(campaigns.RoleNone)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			syncSvc := &stubSyncSvcForRole{}
			campSvc := &stubCampaignSvcForRole{getMemberFn: tc.getMember}
			h := NewAPIHandler(syncSvc, nil, campSvc, nil)

			key := &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "user-1"}
			c, rec := newRoleContext(key)

			if got := h.resolveRole(c); got != tc.wantRole {
				t.Errorf("role = %d, want %d", got, tc.wantRole)
			}
			if rec.Header().Get(keyOwnerDegradedHeader) != "" {
				t.Errorf("session caller must not set the degrade header")
			}
			if len(syncSvc.events) != 0 {
				t.Errorf("session caller must not emit a security event, got %d", len(syncSvc.events))
			}
		})
	}
}

// TestResolveRole_NoKey returns RoleNone when no key is present on the context.
func TestResolveRole_NoKey(t *testing.T) {
	h := NewAPIHandler(&stubSyncSvcForRole{}, nil, &stubCampaignSvcForRole{}, nil)
	c, _ := newRoleContext(nil)
	if got := h.resolveRole(c); got != 0 {
		t.Errorf("role = %d, want 0", got)
	}
}

// TestResolveRole_DegradeSignalThrottled ensures the heavyweight signal (log +
// security event) fires at most once per key per interval, while the response
// header is set on every degraded request so the module can always show its
// banner.
func TestResolveRole_DegradeSignalThrottled(t *testing.T) {
	resetDegradeThrottle()

	syncSvc := &stubSyncSvcForRole{}
	campSvc := &stubCampaignSvcForRole{getMemberFn: memberRemoved}
	h := NewAPIHandler(syncSvc, nil, campSvc, nil)
	key := &APIKey{ID: 99, CampaignID: "camp-1", UserID: "creator-1"}

	for i := 0; i < 3; i++ {
		c, rec := newRoleContext(key)
		if got := h.resolveRole(c); got != int(campaigns.RoleOwner) {
			t.Fatalf("call %d: role = %d, want Owner (sync must not degrade)", i, got)
		}
		if rec.Header().Get(keyOwnerDegradedHeader) != "1" {
			t.Errorf("call %d: degrade header must be set on every degraded request", i)
		}
	}
	if len(syncSvc.events) != 1 {
		t.Errorf("want exactly 1 throttled security event across 3 requests, got %d", len(syncSvc.events))
	}
}
