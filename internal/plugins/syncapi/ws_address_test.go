package syncapi

import (
	"context"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// wsAddressService is a live-feed key restricted to one address, with one
// address on the admin blocklist.
func wsAddressService(t *testing.T) (SyncAPIService, string) {
	t.Helper()
	rawKey := "chron_wsaddr0123456789012345678901234567890123456789012345678901"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(_ context.Context, prefix string) (*APIKey, error) {
			if prefix != rawKey[:keyPrefixLen] {
				return nil, apperror.NewNotFound("key not found")
			}
			return &APIKey{
				ID: 78, KeyHash: string(hash), KeyPrefix: prefix, CampaignID: "camp-ws", UserID: "ws-owner",
				IsActive: true, IPAllowlist: []string{"203.0.113.7", "198.51.100.0/24"},
			}, nil
		},
		isIPBlockedFn: func(_ context.Context, ip string) (bool, error) { return ip == "198.51.100.9", nil },
	}
	svc := NewSyncAPIService(repo)
	gate := newFakeAddonGate()
	gate.enabled["camp-ws"] = true
	svc.SetAddonGate(gate)
	svc.SetMemberChecker(wsOwnerMember)
	return svc, rawKey
}

// The live socket applies the same address rules as every /api/v1 request:
// a key restricted to some addresses opens the feed only from them, and an
// address an admin blocked opens it from nowhere.
func TestAuthenticateKeyForWS_AddressRules(t *testing.T) {
	svc, rawKey := wsAddressService(t)
	for _, tc := range []struct {
		ip     string
		wantOK bool
	}{
		{"203.0.113.7", true},
		{"198.51.100.20", true},
		{"192.0.2.1", false},
		{"198.51.100.9", false},
		{"", false},
	} {
		_, _, _, _, err := svc.AuthenticateKeyForWS(context.Background(), rawKey, tc.ip)
		if (err == nil) != tc.wantOK {
			t.Errorf("from %q: err = %v, want allowed %v", tc.ip, err, tc.wantOK)
		}
	}
}
