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
)

// apiKeyContextKey is the Echo context key for the authenticated API key.
const apiKeyContextKey = "api_key"

// GetAPIKey retrieves the authenticated API key from the request context.
func GetAPIKey(c echo.Context) *APIKey {
	key, _ := c.Get(apiKeyContextKey).(*APIKey)
	return key
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
			// Binding is synchronous to prevent race conditions where concurrent
			// requests could both see DeviceFingerprint==nil and bind different devices.
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
func RateLimit(service SyncAPIService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := GetAPIKey(c)
			if key == nil {
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

