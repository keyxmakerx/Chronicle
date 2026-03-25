package bestiary

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// userRateLimitEntry tracks request counts for a single user within a time window.
type userRateLimitEntry struct {
	count       int
	windowStart time.Time
}

// UserRateLimit returns middleware that limits requests per authenticated user
// to maxRequests within the given window duration. Returns 429 with a
// Retry-After header when exceeded. Unauthenticated requests are rejected.
func UserRateLimit(maxRequests int, window time.Duration) echo.MiddlewareFunc {
	var mu sync.Mutex
	entries := make(map[string]*userRateLimitEntry)

	// Background cleanup of expired entries every minute.
	go func() {
		for {
			time.Sleep(time.Minute)
			mu.Lock()
			now := time.Now()
			for key, entry := range entries {
				if now.Sub(entry.windowStart) > window*2 {
					delete(entries, key)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID := auth.GetUserID(c)
			if userID == "" {
				return next(c) // Let auth middleware handle unauthenticated.
			}

			now := time.Now()

			mu.Lock()
			entry, exists := entries[userID]
			if !exists || now.Sub(entry.windowStart) > window {
				entries[userID] = &userRateLimitEntry{count: 1, windowStart: now}
				mu.Unlock()
				return next(c)
			}

			entry.count++
			if entry.count > maxRequests {
				retryAfter := int(window.Seconds()) - int(now.Sub(entry.windowStart).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				mu.Unlock()
				c.Response().Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				return c.JSON(http.StatusTooManyRequests, map[string]string{
					"error":   "BESTIARY_RATE_LIMIT",
					"message": fmt.Sprintf("Rate limit exceeded. Try again in %d seconds.", retryAfter),
				})
			}
			mu.Unlock()

			return next(c)
		}
	}
}
