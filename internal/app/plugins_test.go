package app

import (
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
)

// TestPluginRegistry_RegisterAndExpose pins the metadata-registry
// surface: a registered plugin appears in RegisteredPlugins() with
// its Slug + HealthCheck intact. Doesn't boot the full App — the
// inline registerPlugin() calls in RegisterRoutes are integration-
// tested by booting Chronicle, not unit-testable in isolation.
func TestPluginRegistry_RegisterAndExpose(t *testing.T) {
	a := &App{}

	a.registerPlugin(PluginRegistration{
		Slug: foundry_vtt.PluginSlug,
		HealthCheck: func() error {
			return nil
		},
	})
	a.registerPlugin(PluginRegistration{
		Slug:        smtp.PluginSlug,
		HealthCheck: nil,
	})

	got := a.RegisteredPlugins()
	if len(got) != 2 {
		t.Fatalf("RegisteredPlugins() length = %d, want 2", len(got))
	}

	bySlug := make(map[string]PluginRegistration)
	for _, p := range got {
		bySlug[p.Slug] = p
	}
	if _, ok := bySlug[foundry_vtt.PluginSlug]; !ok {
		t.Errorf("RegisteredPlugins() missing %q (foundry_vtt.PluginSlug)", foundry_vtt.PluginSlug)
	}
	if _, ok := bySlug[smtp.PluginSlug]; !ok {
		t.Errorf("RegisteredPlugins() missing %q (smtp.PluginSlug)", smtp.PluginSlug)
	}

	if err := bySlug[foundry_vtt.PluginSlug].HealthCheck(); err != nil {
		t.Errorf("foundry_vtt HealthCheck returned %v, want nil", err)
	}
	if bySlug[smtp.PluginSlug].HealthCheck != nil {
		t.Errorf("smtp HealthCheck = non-nil; pilot expects nil to be valid")
	}
}

// TestPluginRegistry_ReturnsCopy pins the API contract that
// RegisteredPlugins() returns a copy callers can mutate without
// affecting the App's internal slice. Prevents a future caller from
// silently corrupting registry state.
func TestPluginRegistry_ReturnsCopy(t *testing.T) {
	a := &App{}
	a.registerPlugin(PluginRegistration{Slug: "x"})

	got := a.RegisteredPlugins()
	got[0].Slug = "mutated"

	again := a.RegisteredPlugins()
	if again[0].Slug != "x" {
		t.Errorf("internal slice corrupted by external mutation: got %q, want %q", again[0].Slug, "x")
	}
}

// TestPluginRegistry_HealthCheckSurface pins that HealthCheck is
// callable and can return both nil (healthy) and an error (unhealthy).
// Future NW-2.4 removable-plugin test iterates registered plugins and
// calls HealthCheck per entry; the contract is: nil = OK, non-nil =
// degraded.
func TestPluginRegistry_HealthCheckSurface(t *testing.T) {
	a := &App{}
	a.registerPlugin(PluginRegistration{
		Slug: "healthy-stub",
		HealthCheck: func() error {
			return nil
		},
	})
	a.registerPlugin(PluginRegistration{
		Slug: "unhealthy-stub",
		HealthCheck: func() error {
			return errors.New("schema not loaded")
		},
	})

	for _, p := range a.RegisteredPlugins() {
		err := p.HealthCheck()
		switch p.Slug {
		case "healthy-stub":
			if err != nil {
				t.Errorf("healthy-stub HealthCheck = %v, want nil", err)
			}
		case "unhealthy-stub":
			if err == nil {
				t.Errorf("unhealthy-stub HealthCheck = nil, want error")
			}
		}
	}
}
