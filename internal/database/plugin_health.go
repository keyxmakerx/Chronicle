// Package database provides connection setup for MariaDB and Redis.
// This file implements the plugin health registry that tracks which
// plugins have healthy schemas and serves that status to middleware,
// templates, and route registration.
package database

import (
	"sync"
	"time"
)

// PluginHealth holds the health status of a single plugin's schema.
type PluginHealth struct {
	// Healthy is true if the plugin's schema migrations ran successfully.
	Healthy bool

	// Error describes what went wrong if Healthy is false. Empty otherwise.
	Error string

	// Version is the highest applied migration version for this plugin.
	Version int

	// LatestVersion is the highest available migration version from disk.
	LatestVersion int

	// UpdatedAt is when this status was last set.
	UpdatedAt time.Time
}

// PluginHealthRegistry tracks which plugins are healthy and provides
// thread-safe reads for middleware and template rendering. Created once
// at startup and shared across the app.
type PluginHealthRegistry struct {
	mu     sync.RWMutex
	status map[string]PluginHealth
}

// NewPluginHealthRegistry creates an empty health registry.
func NewPluginHealthRegistry() *PluginHealthRegistry {
	return &PluginHealthRegistry{
		status: make(map[string]PluginHealth),
	}
}

// Register records the health status of a plugin. Called once per plugin
// during startup after migrations complete (or fail).
func (r *PluginHealthRegistry) Register(slug string, healthy bool, err error, version int, latestVersion int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	h := PluginHealth{
		Healthy:       healthy,
		Version:       version,
		LatestVersion: latestVersion,
		UpdatedAt:     time.Now(),
	}
	if err != nil {
		h.Error = err.Error()
	}
	r.status[slug] = h
}

// IsHealthy returns whether a plugin's schema is operational. Returns true
// for unknown plugins (plugins without schema migrations are always healthy).
func (r *PluginHealthRegistry) IsHealthy(slug string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.status[slug]
	if !ok {
		return true // Unknown plugin = no schema = always healthy.
	}
	return h.Healthy
}

// Get returns the full health status for a plugin. Returns nil if the plugin
// has no registered status.
func (r *PluginHealthRegistry) Get(slug string) *PluginHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.status[slug]
	if !ok {
		return nil
	}
	return &h
}

// All returns a copy of all plugin health statuses. Used by the admin
// dashboard to display plugin health.
func (r *PluginHealthRegistry) All() map[string]PluginHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	copy := make(map[string]PluginHealth, len(r.status))
	for k, v := range r.status {
		copy[k] = v
	}
	return copy
}

// DegradedPlugins returns slugs of plugins that are not healthy.
func (r *PluginHealthRegistry) DegradedPlugins() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var degraded []string
	for slug, h := range r.status {
		if !h.Healthy {
			degraded = append(degraded, slug)
		}
	}
	return degraded
}
