package websocket

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
)

// APIKeyAuthenticator authenticates API key tokens for WebSocket connections.
// Implemented by the syncapi service.
type APIKeyAuthenticator interface {
	// AuthenticateKey validates a raw API key and returns the key's campaign ID,
	// owner user ID, and whether the owner has owner-level campaign access.
	AuthenticateKeyForWS(ctx context.Context, rawKey string) (campaignID, userID string, role int, err error)
}

// SessionAuthenticator authenticates browser sessions for WebSocket connections.
// Implemented by the auth service.
type SessionAuthenticator interface {
	// AuthenticateSessionForWS validates a session cookie and returns user identity.
	AuthenticateSessionForWS(r *http.Request) (userID string, err error)
}

// CampaignRoleLookup resolves a user's role and DM-grant status in a campaign.
// Implemented by the campaigns service.
type CampaignRoleLookup interface {
	// GetUserCampaignRole returns the user's role in the campaign (0 if not a member).
	GetUserCampaignRole(ctx context.Context, campaignID, userID string) (int, error)
	// IsUserDmGranted returns true if the campaign Owner has granted this
	// user dm_only visibility via CampaignSettings.DmGrantIDs. Lets the
	// hub deliver RequiresDM messages to trusted non-Owner members.
	IsUserDmGranted(ctx context.Context, campaignID, userID string) (bool, error)
}

// MultiAuthenticator combines API key and session authentication for WS upgrades.
// It checks the query parameter "token" for API key auth first, then falls back
// to session cookie auth. A "campaign" query parameter is required for session auth.
type MultiAuthenticator struct {
	apiKeyAuth  APIKeyAuthenticator
	sessionAuth SessionAuthenticator
	roleLookup  CampaignRoleLookup
}

// NewMultiAuthenticator creates an authenticator that supports both auth methods.
func NewMultiAuthenticator(apiKey APIKeyAuthenticator, session SessionAuthenticator, roles CampaignRoleLookup) *MultiAuthenticator {
	return &MultiAuthenticator{
		apiKeyAuth:  apiKey,
		sessionAuth: session,
		roleLookup:  roles,
	}
}

// AuthenticateWS implements the Authenticator interface.
// Priority: API key (via ?token= query param) > Session cookie.
func (a *MultiAuthenticator) AuthenticateWS(r *http.Request) (campaignID, userID, source string, role int, isDmGranted bool, err error) {
	ctx := r.Context()

	// foundrySource picks "foundry-module" when the Foundry module
	// self-identifies via ?client=foundry-module on its WS upgrade URL;
	// otherwise we keep the legacy "foundry" tag for any external API-
	// key client. The "foundry-module" source is what feeds the Hub's
	// presence tracker — only the module is treated as authoritative
	// for the /foundry-presence pill, not generic Foundry-style API
	// key callers (CLI scripts, integration tests, etc.).
	foundrySource := func() string {
		if r.URL.Query().Get("client") == foundry_vtt.ModuleSource {
			return foundry_vtt.ModuleSource
		}
		return "foundry"
	}

	// Try API key auth first (Foundry VTT uses this).
	token := r.URL.Query().Get("token")
	if token != "" {
		if a.apiKeyAuth == nil {
			return "", "", "", 0, false, fmt.Errorf("api key auth not configured")
		}
		campaignID, userID, role, err = a.apiKeyAuth.AuthenticateKeyForWS(ctx, token)
		if err != nil {
			return "", "", "", 0, false, fmt.Errorf("api key auth: %w", err)
		}
		isDmGranted = a.lookupDmGranted(ctx, campaignID, userID)
		return campaignID, userID, foundrySource(), role, isDmGranted, nil
	}

	// Fall back to session cookie auth (browser clients).
	if a.sessionAuth == nil {
		return "", "", "", 0, false, fmt.Errorf("no authentication provided")
	}

	userID, err = a.sessionAuth.AuthenticateSessionForWS(r)
	if err != nil {
		return "", "", "", 0, false, fmt.Errorf("session auth: %w", err)
	}

	// Session auth requires a campaign parameter.
	campaignID = r.URL.Query().Get("campaign")
	if campaignID == "" {
		// Also check the Authorization header for Bearer token (alternative path).
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			rawKey := strings.TrimPrefix(authHeader, "Bearer ")
			if a.apiKeyAuth != nil {
				campaignID, userID, role, err = a.apiKeyAuth.AuthenticateKeyForWS(ctx, rawKey)
				if err != nil {
					return "", "", "", 0, false, fmt.Errorf("bearer auth: %w", err)
				}
				isDmGranted = a.lookupDmGranted(ctx, campaignID, userID)
				return campaignID, userID, foundrySource(), role, isDmGranted, nil
			}
		}
		return "", "", "", 0, false, fmt.Errorf("campaign parameter required for session auth")
	}

	// Look up the user's role in the campaign.
	if a.roleLookup != nil {
		role, err = a.roleLookup.GetUserCampaignRole(ctx, campaignID, userID)
		if err != nil {
			return "", "", "", 0, false, fmt.Errorf("role lookup: %w", err)
		}
		if role == 0 {
			return "", "", "", 0, false, fmt.Errorf("user is not a member of campaign %s", campaignID)
		}
	}

	isDmGranted = a.lookupDmGranted(ctx, campaignID, userID)
	return campaignID, userID, "browser", role, isDmGranted, nil
}

// lookupDmGranted resolves the user's IsDmGranted flag, swallowing lookup
// errors as "not granted" — auth has already succeeded and a stale grant
// flag is a soft failure (worst case the user temporarily can't see
// dm_only messages until they reconnect). Logging would belong on the
// adapter side; the websocket package stays free of slog noise.
func (a *MultiAuthenticator) lookupDmGranted(ctx context.Context, campaignID, userID string) bool {
	if a.roleLookup == nil || campaignID == "" || userID == "" {
		return false
	}
	granted, err := a.roleLookup.IsUserDmGranted(ctx, campaignID, userID)
	if err != nil {
		return false
	}
	return granted
}
