// Package middleware provides HTTP middleware for Chronicle.
// ratelimit.go implements sliding-window rate limiters stored in memory:
// RateLimit keys by IP (auth endpoints, uploads), UserRateLimit by a
// caller-supplied identity (authenticated-only routes).
package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// rateLimitEntry tracks request counts for a single IP within a time window.
type rateLimitEntry struct {
	count       int
	windowStart time.Time
}

// RateLimit returns middleware that limits requests per IP to maxRequests
// within the given window duration. Returns 429 when exceeded.
func RateLimit(maxRequests int, window time.Duration) echo.MiddlewareFunc {
	var mu sync.Mutex
	entries := make(map[string]*rateLimitEntry)

	// Background cleanup of expired entries every minute.
	go func() {
		for {
			time.Sleep(time.Minute)
			mu.Lock()
			now := time.Now()
			for ip, entry := range entries {
				if now.Sub(entry.windowStart) > window*2 {
					delete(entries, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			now := time.Now()

			mu.Lock()
			entry, exists := entries[ip]
			if !exists || now.Sub(entry.windowStart) > window {
				entries[ip] = &rateLimitEntry{count: 1, windowStart: now}
				mu.Unlock()
				return next(c)
			}

			entry.count++
			if entry.count > maxRequests {
				mu.Unlock()
				return c.JSON(http.StatusTooManyRequests, map[string]string{
					"error":   "Too Many Requests",
					"message": "Rate limit exceeded. Please try again later.",
				})
			}
			mu.Unlock()
			return next(c)
		}
	}
}

// userRateLimitEntry tracks request counts for a single identity key within
// a time window.
type userRateLimitEntry struct {
	count       int
	windowStart time.Time
}

// UserRateLimit limits requests to maxRequests per window, keyed by
// identity(c). The caller supplies identity so this package never imports
// plugin code; an empty identity skips limiting. Over the limit it answers
// with the app's normal 429 (apperror.NewTooManyRequests) and Retry-After.
func UserRateLimit(identity func(echo.Context) string, maxRequests int, window time.Duration) echo.MiddlewareFunc {
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
			key := identity(c)
			if key == "" {
				return next(c)
			}

			now := time.Now()

			mu.Lock()
			entry, exists := entries[key]
			if !exists || now.Sub(entry.windowStart) > window {
				entries[key] = &userRateLimitEntry{count: 1, windowStart: now}
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
				c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))
				return apperror.NewTooManyRequests("Rate limit exceeded. Please try again shortly.")
			}
			mu.Unlock()
			return next(c)
		}
	}
}
