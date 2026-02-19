// Package middleware provides HTTP middleware for Chronicle.
// ratelimit.go implements a per-IP rate limiter using a sliding window
// counter stored in memory. Designed for auth endpoints and upload endpoints.
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
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
