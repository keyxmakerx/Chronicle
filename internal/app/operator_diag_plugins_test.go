package app

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// hostPluginRows merges three registries that do not agree with each other.
// These tests pin the merge itself, because the ways it can go wrong are all
// half-truths rather than crashes — one plugin rendered as two rows, a static
// mount credited to the wrong spelling, a health record silently dropped — and
// a half-true inventory is what the host.* diagnostics exist to eliminate.

// TestHostPluginRows_MergesTheTwoSpellingsOfOnePlugin is the load-bearing case.
// foundry_vtt registers as `foundry-vtt` in the metadata registry (that
// spelling is also its static URL prefix) and as `foundry_vtt` in the schema
// runner and health registry. Keyed literally, one plugin becomes two rows: one
// apparently migration-less, one apparently unregistered — and an operator
// reading either one draws a wrong conclusion.
func TestHostPluginRows_MergesTheTwoSpellingsOfOnePlugin(t *testing.T) {
	health := database.NewPluginHealthRegistry()
	health.Register("foundry_vtt", true, nil, 3, 3)

	a := &App{
		PluginSchemas: []database.PluginSchema{{Slug: "foundry_vtt", MigrationsFS: fstest.MapFS{}}},
		PluginHealth:  health,
	}
	a.registerPlugin(PluginRegistration{Slug: "foundry-vtt", HealthCheck: func() error { return nil }})

	rows := a.hostPluginRows()
	if len(rows) != 1 {
		t.Fatalf("the two spellings must merge into ONE row, got %d: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.Slug != "foundry-vtt" {
		t.Errorf("the display slug should be the metadata spelling (it is the URL prefix), got %q", r.Slug)
	}
	if r.SchemaKey != "foundry_vtt" {
		t.Errorf("the row must carry the schema spelling as an alias, got %q", r.SchemaKey)
	}
	if !r.InMetadataRegistry || !r.DeclaresMigrations || !r.SchemaKnown {
		t.Errorf("the merged row lost a registry: %+v", r)
	}
	if r.SchemaVersion != 3 || r.SchemaLatest != 3 || !r.SchemaHealthy {
		t.Errorf("the merged row lost its health record: %+v", r)
	}
}

// TestHostPluginRows_KeepsGenuinelyDifferentPluginsApart pins that the
// normalisation is narrow. A merge key aggressive enough to fold unrelated
// plugins together would hide one behind the other.
func TestHostPluginRows_KeepsGenuinelyDifferentPluginsApart(t *testing.T) {
	a := &App{}
	a.registerPlugin(PluginRegistration{Slug: "calendar"})
	a.registerPlugin(PluginRegistration{Slug: "calendar-v2"})
	a.registerPlugin(PluginRegistration{Slug: "entities"})

	if got := len(a.hostPluginRows()); got != 3 {
		t.Errorf("three distinct plugins must stay three rows, got %d", got)
	}

	for _, tc := range []struct {
		a, b string
		same bool
	}{
		{"foundry-vtt", "foundry_vtt", true},
		{"Foundry_VTT", "foundry-vtt", true},
		{"calendar", "calendar-v2", false},
		{"maps", "map", false},
	} {
		got := normalizePluginKey(tc.a) == normalizePluginKey(tc.b)
		if got != tc.same {
			t.Errorf("normalizePluginKey(%q)==normalizePluginKey(%q) = %v, want %v", tc.a, tc.b, got, tc.same)
		}
	}
}

// TestHostPluginRows_CarriesStaticMountAndHealthResult pins the fields the
// renderer depends on: the URL prefix must be the one mountPluginStatic
// actually serves (shared through pluginStaticPrefix, not retyped), and a
// failing health callback must reach the row rather than being swallowed.
func TestHostPluginRows_CarriesStaticMountAndHealthResult(t *testing.T) {
	fsys := fstest.MapFS{"js/x.js": &fstest.MapFile{Data: []byte("//x")}}
	a := &App{}
	a.registerPlugin(PluginRegistration{Slug: "calendar", StaticFS: fsys, HealthCheck: func() error { return errors.New("calendar schema unhealthy") }})
	a.registerPlugin(PluginRegistration{Slug: "smtp"})

	bySlug := map[string]struct {
		prefix    string
		hasFS     bool
		hasCheck  bool
		healthErr string
	}{}
	for _, r := range a.hostPluginRows() {
		bySlug[r.Slug] = struct {
			prefix    string
			hasFS     bool
			hasCheck  bool
			healthErr string
		}{r.StaticURLPrefix, r.StaticFS != nil, r.HasHealthCheck, r.HealthErr}
	}

	cal := bySlug["calendar"]
	if want := pluginStaticPrefix("calendar") + "/"; cal.prefix != want {
		t.Errorf("static prefix = %q, want %q (it must come from the same helper the mount uses)", cal.prefix, want)
	}
	if !cal.hasFS {
		t.Error("the row must carry the filesystem so the renderer can count and size it")
	}
	if cal.healthErr != "calendar schema unhealthy" {
		t.Errorf("a failing health callback must reach the row, got %q", cal.healthErr)
	}

	smtp := bySlug["smtp"]
	if smtp.prefix != "" || smtp.hasFS {
		t.Errorf("a plugin with no StaticFS must claim no prefix, got %+v", smtp)
	}
	if smtp.hasCheck {
		t.Error("a plugin with no HealthCheck must not be reported as having one")
	}
}

// TestHostPluginRows_SurvivesAMissingHealthRegistry pins that the adapter works
// on a partially-constructed App. It is read by a diagnostic, and a diagnostic
// that panics when one dependency is absent is useless exactly when something
// is wrong.
func TestHostPluginRows_SurvivesAMissingHealthRegistry(t *testing.T) {
	a := &App{PluginSchemas: []database.PluginSchema{{Slug: "maps"}}}
	rows := a.hostPluginRows()
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if !rows[0].DeclaresMigrations {
		t.Error("a schema-only plugin must still report that it contributed migrations")
	}
	if rows[0].SchemaKnown {
		t.Error("with no health registry, the schema outcome is UNKNOWN — it must not be reported as observed")
	}

	if got := (&App{}).hostPluginRows(); len(got) != 0 {
		t.Errorf("an empty App must yield no rows, got %+v", got)
	}
}
