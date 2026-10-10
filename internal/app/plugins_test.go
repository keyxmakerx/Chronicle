package app

import (
	"errors"
	"reflect"
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
// callable and can return both nil (healthy) and an error (unhealthy):
// the contract is nil = OK, non-nil = degraded.
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

// TestBuildWidgetManifest pins how on-sight widget registrations become the
// manifest boot.js reads: plugin-relative paths resolve under the plugin's
// static mount, site paths stay as they are, script order is kept, and a
// wiring mistake drops only the widget it names.
func TestBuildWidgetManifest(t *testing.T) {
	hash := func(p string) string { return p + "?v=h" }
	tests := []struct {
		name    string
		regs    []PluginRegistration
		want    map[string][]string
		wantErr bool
	}{
		{
			name: "no widgets",
			regs: []PluginRegistration{{Slug: "maps"}},
			want: map[string][]string{},
		},
		{
			name: "plugin-relative and site paths, in order",
			regs: []PluginRegistration{{Slug: "chart", Widgets: []PluginWidget{
				{Name: "timeline-viz", Scripts: []string{"/static/js/widgets/groups.js", "js/timeline_viz.js"}},
			}}},
			want: map[string][]string{"timeline-viz": {
				"/static/js/widgets/groups.js?v=h",
				"/static/plugins/chart/js/timeline_viz.js?v=h",
			}},
		},
		{
			name: "a name claimed twice keeps the first",
			regs: []PluginRegistration{
				{Slug: "a", Widgets: []PluginWidget{{Name: "w", Scripts: []string{"js/a.js"}}}},
				{Slug: "b", Widgets: []PluginWidget{{Name: "w", Scripts: []string{"js/b.js"}}}},
			},
			want:    map[string][]string{"w": {"/static/plugins/a/js/a.js?v=h"}},
			wantErr: true,
		},
		{
			name: "a widget with no scripts is left out",
			regs: []PluginRegistration{{Slug: "a", Widgets: []PluginWidget{
				{Name: "empty"},
				{Name: "ok", Scripts: []string{"js/ok.js"}},
			}}},
			want:    map[string][]string{"ok": {"/static/plugins/a/js/ok.js?v=h"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildWidgetManifest(tt.regs, hash)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("manifest = %v, want %v", got, tt.want)
			}
		})
	}
}
