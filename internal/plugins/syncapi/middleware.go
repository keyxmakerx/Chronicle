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
				service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventIPBlocked,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
				})
				return echo.NewHTTPError(http.StatusForbidden, "ip address blocked")
			}

			// Extract API key from Authorization header.
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventAuthFailure,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
					Details:   map[string]any{"reason": "missing authorization header"},
				})
				return echo.NewHTTPError(http.StatusUnauthorized, "api key required")
			}

			rawKey := strings.TrimPrefix(authHeader, "Bearer ")
			if rawKey == authHeader {
				// No "Bearer " prefix found.
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization format, use: Bearer <key>")
			}

			// Authenticate the key (prefix lookup + bcrypt verify).
			key, err := service.AuthenticateKey(ctx, rawKey)
			if err != nil {
				service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventAuthFailure,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
					Details:   map[string]any{"reason": err.Error()},
				})
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid api key")
			}

			// Verify IP allowlist if configured.
			if len(key.IPAllowlist) > 0 && !isIPAllowed(ip, key.IPAllowlist) {
				service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventIPBlocked,
					APIKeyID:  &key.ID,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
					Details:   map[string]any{"reason": "ip not in allowlist"},
				})
				return echo.NewHTTPError(http.StatusForbidden, "ip address not allowed for this key")
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
				return echo.NewHTTPError(http.StatusUnauthorized, "api key required")
			}
			if !key.HasPermission(perm) {
				return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions: requires "+string(perm))
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
				return echo.NewHTTPError(http.StatusUnauthorized, "api key required")
			}
			campaignID := c.Param("id")
			if campaignID != key.CampaignID {
				return echo.NewHTTPError(http.StatusForbidden, "api key not authorized for this campaign")
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
				service.LogSecurityEvent(c.Request().Context(), &SecurityEvent{
					EventType:  EventRateLimit,
					APIKeyID:   &key.ID,
					CampaignID: &key.CampaignID,
					IPAddress:  c.RealIP(),
					UserAgent:  strPtr(c.Request().UserAgent()),
				})
				c.Response().Header().Set("Retry-After", "60")
				return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
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

