package websocket

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	gorillaWs "github.com/gorilla/websocket"

	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
)

// APIKeyAuthenticator authenticates API key tokens for WebSocket connections.
// Implemented by the syncapi service.
type APIKeyAuthenticator interface {
	// AuthenticateKeyForWS validates a raw API key and returns its campaign,
	// owner, owner-level role, and expiry (nil if the key never expires).
	AuthenticateKeyForWS(ctx context.Context, rawKey string) (campaignID, userID string, role int, expiresAt *time.Time, err error)
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

// NotesGrantAuthenticator authenticates a player's notes grant, the token an
// outside app's notes frame holds instead of a sign-in. Implemented in
// internal/app over the notes widget's grant service and the campaign's
// outside-apps switch.
type NotesGrantAuthenticator interface {
	// AuthenticateNotesGrantForWS returns the grant's campaign and player,
	// or an error when the token is unknown, revoked, idle too long, or the
	// campaign has outside apps turned off.
	AuthenticateNotesGrantForWS(ctx context.Context, token string) (campaignID, userID string, err error)
}

// NotesAppSource tags a socket opened with a notes grant. The hub sends such
// a socket note events only, since the grant reaches nothing else.
const NotesAppSource = "notes-app"

// NotesSubprotocol is the WebSocket subprotocol a notes frame asks for, and
// the one the server answers with. The grant itself travels as a second
// offered subprotocol, NotesGrantProtocolPrefix + token: a browser can't set
// headers on a WebSocket, and a token in the URL would land in access logs.
const NotesSubprotocol = "chronicle.notes"

// NotesGrantProtocolPrefix prefixes the offered subprotocol carrying the
// grant token. Grant tokens are base64url, which is valid in a subprotocol.
const NotesGrantProtocolPrefix = "chronicle.grant."

// MultiAuthenticator combines API key, notes grant and session
// authentication for WS upgrades. It checks the query parameter "token" for
// API key auth first, then a notes grant offered as a subprotocol, then
// falls back to session cookie auth. A "campaign" query parameter is
// required for session auth.
type MultiAuthenticator struct {
	apiKeyAuth  APIKeyAuthenticator
	sessionAuth SessionAuthenticator
	roleLookup  CampaignRoleLookup
	notesAuth   NotesGrantAuthenticator
}

// NewMultiAuthenticator creates an authenticator that supports both auth methods.
func NewMultiAuthenticator(apiKey APIKeyAuthenticator, session SessionAuthenticator, roles CampaignRoleLookup) *MultiAuthenticator {
	return &MultiAuthenticator{
		apiKeyAuth:  apiKey,
		sessionAuth: session,
		roleLookup:  roles,
	}
}

// SetNotesGrantAuth enables notes grant sockets. Without it a grant offered
// as a subprotocol is refused.
func (a *MultiAuthenticator) SetNotesGrantAuth(n NotesGrantAuthenticator) {
	a.notesAuth = n
}

// notesGrantToken returns the grant token a notes frame offered as a
// subprotocol, and whether it offered the notes subprotocol at all.
func notesGrantToken(r *http.Request) (token string, offered bool) {
	for _, p := range gorillaWs.Subprotocols(r) {
		if p == NotesSubprotocol {
			offered = true
		} else if t, ok := strings.CutPrefix(p, NotesGrantProtocolPrefix); ok && token == "" {
			token = t
		}
	}
	return token, offered
}

// authenticateNotesGrant checks a notes grant socket. The player must still
// be a member of the grant's campaign at upgrade time, as the grant's HTTP
// routes require on every request; a campaign named in the URL must be the
// grant's own. The key expiry stays nil: a grant has no fixed end, and
// revoking it closes the socket (Hub.RevokeNotesAppClients).
func (a *MultiAuthenticator) authenticateNotesGrant(r *http.Request, token string) (campaignID, userID, source string, role int, isDmGranted bool, expiresAt *time.Time, err error) {
	ctx := r.Context()
	if a.notesAuth == nil || a.roleLookup == nil {
		return "", "", "", 0, false, nil, fmt.Errorf("notes grant auth not configured")
	}
	campaignID, userID, err = a.notesAuth.AuthenticateNotesGrantForWS(ctx, token)
	if err != nil {
		return "", "", "", 0, false, nil, fmt.Errorf("notes grant auth: %w", err)
	}
	if want := r.URL.Query().Get("campaign"); want != "" && want != campaignID {
		return "", "", "", 0, false, nil, fmt.Errorf("notes grant is for another campaign")
	}
	role, err = a.roleLookup.GetUserCampaignRole(ctx, campaignID, userID)
	if err != nil {
		return "", "", "", 0, false, nil, fmt.Errorf("role lookup: %w", err)
	}
	if role == 0 {
		return "", "", "", 0, false, nil, fmt.Errorf("user is not a member of campaign %s", campaignID)
	}
	isDmGranted = a.lookupDmGranted(ctx, campaignID, userID)
	return campaignID, userID, NotesAppSource, role, isDmGranted, nil, nil
}

// AuthenticateWS implements the Authenticator interface.
// Priority: API key (via ?token= query param) > notes grant (subprotocol) >
// Session cookie.
func (a *MultiAuthenticator) AuthenticateWS(r *http.Request) (campaignID, userID, source string, role int, isDmGranted bool, expiresAt *time.Time, err error) {
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
			return "", "", "", 0, false, nil, fmt.Errorf("api key auth not configured")
		}
		campaignID, userID, role, expiresAt, err = a.apiKeyAuth.AuthenticateKeyForWS(ctx, token)
		if err != nil {
			return "", "", "", 0, false, nil, fmt.Errorf("api key auth: %w", err)
		}
		isDmGranted = a.lookupDmGranted(ctx, campaignID, userID)
		return campaignID, userID, foundrySource(), role, isDmGranted, expiresAt, nil
	}

	// A notes frame has no session cookie (it is a cross-site frame), so it
	// never falls through to session auth: asking for the notes subprotocol
	// commits it to the grant.
	if grant, offered := notesGrantToken(r); offered || grant != "" {
		if grant == "" || !offered {
			return "", "", "", 0, false, nil, fmt.Errorf("notes grant: incomplete subprotocol offer")
		}
		return a.authenticateNotesGrant(r, grant)
	}

	// Fall back to session cookie auth (browser clients).
	if a.sessionAuth == nil {
		return "", "", "", 0, false, nil, fmt.Errorf("no authentication provided")
	}

	userID, err = a.sessionAuth.AuthenticateSessionForWS(r)
	if err != nil {
		return "", "", "", 0, false, nil, fmt.Errorf("session auth: %w", err)
	}

	// Session auth requires a campaign parameter.
	campaignID = r.URL.Query().Get("campaign")
	if campaignID == "" {
		// Also check the Authorization header for Bearer token (alternative path).
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			rawKey := strings.TrimPrefix(authHeader, "Bearer ")
			if a.apiKeyAuth != nil {
				campaignID, userID, role, expiresAt, err = a.apiKeyAuth.AuthenticateKeyForWS(ctx, rawKey)
				if err != nil {
					return "", "", "", 0, false, nil, fmt.Errorf("bearer auth: %w", err)
				}
				isDmGranted = a.lookupDmGranted(ctx, campaignID, userID)
				return campaignID, userID, foundrySource(), role, isDmGranted, expiresAt, nil
			}
		}
		return "", "", "", 0, false, nil, fmt.Errorf("campaign parameter required for session auth")
	}

	// Look up the user's role in the campaign.
	if a.roleLookup != nil {
		role, err = a.roleLookup.GetUserCampaignRole(ctx, campaignID, userID)
		if err != nil {
			return "", "", "", 0, false, nil, fmt.Errorf("role lookup: %w", err)
		}
		if role == 0 {
			return "", "", "", 0, false, nil, fmt.Errorf("user is not a member of campaign %s", campaignID)
		}
	}

	// expiresAt stays nil: a session has no key expiry to carry.
	isDmGranted = a.lookupDmGranted(ctx, campaignID, userID)
	return campaignID, userID, "browser", role, isDmGranted, nil, nil
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
