package syncapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// apiKeyContextKey is the Echo context key for the authenticated API key.
const apiKeyContextKey = "api_key"

// synthKeySessionID is the sentinel KeyID used for synthetic APIKeys that
// represent a session-authed caller on /api/v1/*. A real api_keys row has
// an AUTO_INCREMENT id >= 1, so ID == 0 unambiguously flags a synthetic
// identity and lets downstream middleware (RateLimit, logging) skip work
// that only makes sense for real keys.
const synthKeySessionID = 0

// GetAPIKey retrieves the authenticated API key from the request context.
// Under the multi-auth path (RequireAuthOrAPIKey), a session-authed caller
// gets a synthetic APIKey with ID == 0; callers that need to distinguish
// real keys from session callers should check for that sentinel.
func GetAPIKey(c echo.Context) *APIKey {
	key, _ := c.Get(apiKeyContextKey).(*APIKey)
	return key
}

// permissionsForCampaignRole maps a user's campaign membership role onto the
// API-key permission model so session-authed callers on /api/v1/* get the
// same downstream authorization as an equivalent Bearer request. The mapping
// is the natural least-surprising one: Owner grants everything; Scribe can
// read + write content but not configure sync endpoints; Player is read-only.
// Called by RequireAuthOrAPIKey when synthesising an APIKey from a session.
func permissionsForCampaignRole(role campaigns.Role) []APIKeyPermission {
	switch role {
	case campaigns.RoleOwner:
		return []APIKeyPermission{PermRead, PermWrite, PermSync}
	case campaigns.RoleScribe:
		return []APIKeyPermission{PermRead, PermWrite}
	case campaigns.RolePlayer:
		return []APIKeyPermission{PermRead}
	default:
		return nil
	}
}

// RequireAuthOrAPIKey authenticates /api/v1/* requests by EITHER a session
// cookie OR an Authorization: Bearer API key. This is the single source of
// truth for "who is the caller?" on the public API: in-app browser widgets
// authenticate by the same session cookie they already carry for the web UI,
// and external REST / Foundry VTT clients continue to use Bearer tokens.
//
// Flow:
//  1. If a chronicle_session cookie is present AND validates, look up the
//     caller's membership in the campaign named by the :id URL parameter.
//     Synthesise an APIKey whose CampaignID, UserID, and Permissions match
//     the session + campaign role, and expose it on the context under the
//     same apiKeyContextKey that the Bearer path uses. Downstream middleware
//     (RequireCampaignMatch, RequirePermission) then works uniformly.
//  2. Otherwise, delegate to RequireAPIKey for the traditional Bearer path.
//
// Security notes:
//   - The session cookie is SameSite=Lax (see auth.setSessionCookie), so a
//     cross-origin POST will NOT include it. Explicit CSRF tokens are
//     therefore not required here — the cookie attribute already provides
//     the CSRF defense for the multi-auth flow.
//   - Synthesised keys carry ID == synthKeySessionID so the rate limiter and
//     LogRequest path can distinguish session callers from real API keys.
//   - IP blocklist / IP allowlist / device fingerprint enforcement run only
//     on the Bearer path. Session auth trusts the upstream auth service's
//     session validation (cookie rotation, expiry, IP rate limits on login).
//
// Must be wired in place of RequireAPIKey on the /api/v1 group.
func RequireAuthOrAPIKey(authSvc auth.AuthService, campaignSvc campaigns.CampaignService, syncSvc SyncAPIService) echo.MiddlewareFunc {
	apiKey := RequireAPIKey(syncSvc)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		apiKeyChain := apiKey(next)
		return func(c echo.Context) error {
			if key, ok := tryAuthFromSession(c, authSvc, campaignSvc); ok {
				c.Set(apiKeyContextKey, key)
				return next(c)
			}
			return apiKeyChain(c)
		}
	}
}

// tryAuthFromSession attempts to resolve the caller from a session cookie.
// Returns (key, true) when a valid session maps to a campaign membership
// the URL allows, where `key` is a synthetic APIKey with permissions
// derived from the membership role. Returns (nil, false) on any negative
// outcome — "no cookie", "invalid cookie", "not a member" — so the caller
// can cleanly fall through to the Bearer path. A user who is signed in but
// isn't a member of the campaign in the URL must return false here rather
// than 403, otherwise a real API key for the same campaign would be
// blocked by the session-membership failure.
func tryAuthFromSession(c echo.Context, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) (*APIKey, bool) {
	token := auth.GetSessionTokenFromCookie(c)
	if token == "" {
		return nil, false
	}
	session, err := authSvc.ValidateSession(c.Request().Context(), token)
	if err != nil || session == nil {
		return nil, false
	}

	campaignID := c.Param("id")
	if campaignID == "" {
		// Routes that don't carry :id can still authenticate by session
		// (e.g. a future /api/v1/me endpoint); synthesise a minimal key.
		auth.SetSession(c, session)
		return &APIKey{
			ID:       synthKeySessionID,
			UserID:   session.UserID,
			IsActive: true,
		}, true
	}

	member, err := campaignSvc.GetMember(c.Request().Context(), campaignID, session.UserID)
	if err != nil || member == nil {
		// Session is valid but the user isn't a member of this campaign.
		// Fall through to the Bearer path; a site admin still lands on
		// 403 when the Bearer path also fails, which is the correct
		// outcome.
		return nil, false
	}

	auth.SetSession(c, session)
	return &APIKey{
		ID:          synthKeySessionID,
		CampaignID:  campaignID,
		UserID:      session.UserID,
		Permissions: permissionsForCampaignRole(member.Role),
		IsActive:    true,
	}, true
}

// RequireAPIKey returns middleware that authenticates requests via API key.
// Extracts the key from the Authorization header, validates it with bcrypt,
// checks the IP blocklist, verifies IP allowlist, and records the request.
func RequireAPIKey(service SyncAPIService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ip := c.RealIP()

			// Check IP blocklist first — reject before any key processing.
			blocked, err := service.IsIPBlocked(ctx, ip)
			if err != nil {
				slog.Warn("ip blocklist check failed", slog.Any("error", err))
			}
			if blocked {
				_ = service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventIPBlocked,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
				})
				return apperror.NewForbidden("ip address blocked")
			}

			// Extract API key from Authorization header.
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				_ = service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventAuthFailure,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
					Details:   map[string]any{"reason": "missing authorization header"},
				})
				return apperror.NewUnauthorized("api key required")
			}

			rawKey := strings.TrimPrefix(authHeader, "Bearer ")
			if rawKey == authHeader {
				// No "Bearer " prefix found.
				return apperror.NewUnauthorized("invalid authorization format, use: Bearer <key>")
			}

			// Authenticate the key (prefix lookup + bcrypt verify).
			key, err := service.AuthenticateKey(ctx, rawKey)
			if err != nil {
				_ = service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventAuthFailure,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
					Details:   map[string]any{"reason": err.Error()},
				})
				return apperror.NewUnauthorized("invalid api key")
			}

			// Verify IP allowlist if configured.
			if len(key.IPAllowlist) > 0 && !isIPAllowed(ip, key.IPAllowlist) {
				_ = service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventIPBlocked,
					APIKeyID:  &key.ID,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
					Details:   map[string]any{"reason": "ip not in allowlist"},
				})
				return apperror.NewForbidden("ip address not allowed for this key")
			}

			// Device fingerprint enforcement: if the client sends X-Device-Fingerprint,
			// auto-bind on first use; reject mismatches on subsequent requests.
			// This ensures a key can only be used by a single registered device.
			// Binding uses a conditional UPDATE (WHERE fingerprint IS NULL) so
			// concurrent first requests race safely — only one wins the bind.
			deviceFP := c.Request().Header.Get("X-Device-Fingerprint")
			if deviceFP != "" {
				if key.DeviceFingerprint == nil {
					// First use — bind the device synchronously.
					if bindErr := service.BindDevice(ctx, key.ID, deviceFP); bindErr != nil {
						slog.Warn("device fingerprint binding failed",
							slog.Int("key_id", key.ID),
							slog.Any("error", bindErr),
						)
					}
				} else if *key.DeviceFingerprint != deviceFP {
					// Device mismatch — reject.
					_ = service.LogSecurityEvent(ctx, &SecurityEvent{
						EventType:  EventSuspicious,
						APIKeyID:   &key.ID,
						CampaignID: &key.CampaignID,
						IPAddress:  ip,
						UserAgent:  strPtr(c.Request().UserAgent()),
						Details:    map[string]any{"reason": "device fingerprint mismatch"},
					})
					return apperror.NewForbidden("device not authorized for this key")
				}
			}

			// Store the key in context for downstream handlers.
			c.Set(apiKeyContextKey, key)

			// Update last-used timestamp (fire-and-forget).
			// Use background context since the request context may be cancelled
			// before the goroutine completes.
			go func() {
				_ = service.UpdateKeyLastUsed(context.Background(), key.ID, ip)
			}()

			// Execute the handler and log the request.
			start := time.Now()
			err = next(c)
			duration := time.Since(start)

			statusCode := c.Response().Status
			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					statusCode = he.Code
				} else {
					statusCode = http.StatusInternalServerError
				}
			}

			// Log the request (fire-and-forget).
			var errMsg *string
			if err != nil {
				msg := err.Error()
				errMsg = &msg
			}
			go func() {
				_ = service.LogRequest(context.Background(), &APIRequestLog{
					APIKeyID:     key.ID,
					CampaignID:   key.CampaignID,
					UserID:       key.UserID,
					Method:       c.Request().Method,
					Path:         c.Request().URL.Path,
					StatusCode:   statusCode,
					IPAddress:    ip,
					UserAgent:    strPtr(c.Request().UserAgent()),
					RequestSize:  int(c.Request().ContentLength),
					ResponseSize: int(c.Response().Size),
					DurationMs:   int(duration.Milliseconds()),
					ErrorMessage: errMsg,
				})
			}()

			return err
		}
	}
}

// RequirePermission returns middleware that checks the API key has a specific permission.
func RequirePermission(perm APIKeyPermission) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := GetAPIKey(c)
			if key == nil {
				return apperror.NewUnauthorized("api key required")
			}
			if !key.HasPermission(perm) {
				return apperror.NewForbidden("insufficient permissions: requires " + string(perm))
			}
			return next(c)
		}
	}
}

// RequireCampaignMatch returns middleware that verifies the API key's campaign
// matches the :id parameter in the URL. Prevents using a key scoped to one
// campaign to access another.
func RequireCampaignMatch() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := GetAPIKey(c)
			if key == nil {
				return apperror.NewUnauthorized("api key required")
			}
			campaignID := c.Param("id")
			if campaignID != key.CampaignID {
				return apperror.NewForbidden("api key not authorized for this campaign")
			}
			return next(c)
		}
	}
}

// --- Rate Limiting ---

// rateLimiter tracks per-key request counts using a sliding window.
type rateLimiter struct {
	mu      sync.Mutex
	windows map[int]*rateLimitWindow // Keyed by API key ID.
}

// rateLimitWindow tracks requests in the current minute.
type rateLimitWindow struct {
	count   int
	resetAt time.Time
}

// globalRateLimiter is the singleton rate limiter instance.
var globalRateLimiter = &rateLimiter{
	windows: make(map[int]*rateLimitWindow),
}

// RateLimit returns middleware that enforces per-key request rate limits.
// Uses a simple fixed-window counter per minute.
//
// Synthetic session keys (ID == synthKeySessionID) skip this limiter —
// they represent an authenticated browser user, not an external client,
// and would otherwise all share the ID == 0 bucket. Browser-paced users
// are naturally rate-limited by human speed; abusive session behavior is
// the auth service's responsibility, not the API-key limiter's.
func RateLimit(service SyncAPIService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := GetAPIKey(c)
			if key == nil {
				return next(c)
			}
			if key.ID == synthKeySessionID {
				return next(c)
			}

			globalRateLimiter.mu.Lock()
			window, exists := globalRateLimiter.windows[key.ID]
			now := time.Now()

			if !exists || now.After(window.resetAt) {
				// New window.
				window = &rateLimitWindow{
					count:   0,
					resetAt: now.Add(time.Minute),
				}
				globalRateLimiter.windows[key.ID] = window
			}

			window.count++
			remaining := key.RateLimit - window.count
			globalRateLimiter.mu.Unlock()

			// Set rate limit headers.
			c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(key.RateLimit))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(remaining, 0)))

			if remaining < 0 {
				_ = service.LogSecurityEvent(c.Request().Context(), &SecurityEvent{
					EventType:  EventRateLimit,
					APIKeyID:   &key.ID,
					CampaignID: &key.CampaignID,
					IPAddress:  c.RealIP(),
					UserAgent:  strPtr(c.Request().UserAgent()),
				})
				c.Response().Header().Set("Retry-After", "60")
				return &apperror.AppError{Code: http.StatusTooManyRequests, Type: "rate_limit_exceeded", Message: "rate limit exceeded"}
			}

			return next(c)
		}
	}
}

// --- Helpers ---

// strPtr returns a pointer to a string (nil if empty).
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isIPAllowed checks if an IP is in the allowlist.
// Supports exact match and proper CIDR notation (e.g., "192.168.1.0/24").
func isIPAllowed(ip string, allowlist []string) bool {
	parsedIP := net.ParseIP(ip)
	for _, allowed := range allowlist {
		// Try exact match first.
		if allowed == ip {
			return true
		}
		// Try proper CIDR matching using the standard library.
		if strings.Contains(allowed, "/") {
			_, network, err := net.ParseCIDR(allowed)
			if err == nil && parsedIP != nil && network.Contains(parsedIP) {
				return true
			}
		}
	}
	return false
}

// RequireAddonAPI returns middleware that gates API endpoints behind addon
// enabled checks. Returns 404 JSON response when the addon is disabled,
// matching the behavior of the web RequireAddon middleware but for API context.
func RequireAddonAPI(addonChecker AddonChecker, slug string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			campaignID := c.Param("id")
			if campaignID == "" {
				return apperror.NewBadRequest("campaign ID required")
			}

			enabled, err := addonChecker.IsEnabledForCampaign(c.Request().Context(), campaignID, slug)
			if err != nil {
				// Fail closed — block access when addon status cannot be verified.
				slog.Error("addon check failed",
					slog.String("campaign_id", campaignID),
					slog.String("slug", slug),
					slog.Any("error", err),
				)
				return &apperror.AppError{Code: http.StatusServiceUnavailable, Type: "service_unavailable", Message: "temporarily unable to verify addon status"}
			}
			if !enabled {
				return apperror.NewNotFound(slug + " addon is not enabled for this campaign")
			}
			return next(c)
		}
	}
}

// AddonChecker defines the interface for checking addon enabled state.
// Implemented by the addons plugin's service.
type AddonChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID, slug string) (bool, error)
}

