package websocket

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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

// CampaignRoleLookup resolves a user's role in a campaign.
// Implemented by the campaigns service.
type CampaignRoleLookup interface {
	// GetUserCampaignRole returns the user's role in the campaign (0 if not a member).
	GetUserCampaignRole(ctx context.Context, campaignID, userID string) (int, error)
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
func (a *MultiAuthenticator) AuthenticateWS(r *http.Request) (campaignID, userID, source string, role int, err error) {
	ctx := r.Context()

	// Try API key auth first (Foundry VTT uses this).
	token := r.URL.Query().Get("token")
	if token != "" {
		if a.apiKeyAuth == nil {
			return "", "", "", 0, fmt.Errorf("api key auth not configured")
		}
		campaignID, userID, role, err = a.apiKeyAuth.AuthenticateKeyForWS(ctx, token)
		if err != nil {
			return "", "", "", 0, fmt.Errorf("api key auth: %w", err)
		}
		return campaignID, userID, "foundry", role, nil
	}

	// Fall back to session cookie auth (browser clients).
	if a.sessionAuth == nil {
		return "", "", "", 0, fmt.Errorf("no authentication provided")
	}

	userID, err = a.sessionAuth.AuthenticateSessionForWS(r)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("session auth: %w", err)
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
					return "", "", "", 0, fmt.Errorf("bearer auth: %w", err)
				}
				return campaignID, userID, "foundry", role, nil
			}
		}
		return "", "", "", 0, fmt.Errorf("campaign parameter required for session auth")
	}

	// Look up the user's role in the campaign.
	if a.roleLookup != nil {
		role, err = a.roleLookup.GetUserCampaignRole(ctx, campaignID, userID)
		if err != nil {
			return "", "", "", 0, fmt.Errorf("role lookup: %w", err)
		}
		if role == 0 {
			return "", "", "", 0, fmt.Errorf("user is not a member of campaign %s", campaignID)
		}
	}

	return campaignID, userID, "browser", role, nil
}
