package aiexport

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubEraSource returns canned versions and records the viewer it was asked
// as, so a test can check a Safe export asks as a player.
type stubEraSource struct {
	versions map[string][]EraVersion
	err      error
	viewer   permissions.Viewer
}

func (s *stubEraSource) EraVersionsFor(_ context.Context, _ string, _ []string, v permissions.Viewer) (map[string][]EraVersion, error) {
	s.viewer = v
	return s.versions, s.err
}

func eraExportService(src EraVersionSource) *Service {
	svc := NewService(
		&stubEntityLister{
			ents: []entities.Entity{{
				ID: "e1", Name: "Stonehold", EntityTypeID: 1, TypeName: "Location",
				EntryHTML: sp("<p>Usual keep.</p>"),
			}},
			types: []entities.EntityType{{ID: 1, Name: "Location", NamePlural: "Locations"}},
		},
		nil, nil, nil, nil, nil, nil,
	)
	svc.EraVersions = src
	return svc
}

func stoneholdVersions() map[string][]EraVersion {
	return map[string][]EraVersion{"e1": {
		{EraName: "Age of Ash", Current: false, Name: sp("Ashen Keep"), EntryHTML: sp("<p>Burnt keep.</p>")},
		{EraName: "Golden Age", Current: true, Name: sp("The Golden Keep"), EntryHTML: sp("<p>Gilded keep.</p>")},
		{EraName: "Age of Dust", Current: false, EntryHTML: nil},
	}}
}

func TestService_EraVersionsInEntityExport(t *testing.T) {
	tests := []struct {
		name     string
		privacy  PrivacyMode
		src      *stubEraSource
		contains []string
		absent   []string
		wantRole int
	}{
		{
			name:    "everything emits each version as a page block",
			privacy: PrivacyModeEverything,
			src:     &stubEraSource{versions: stoneholdVersions()},
			contains: []string{
				"### Stonehold {#stonehold}",
				"Usual keep.",
				"---\nname: \"Stonehold\"\nera: \"Age of Ash\"\nknown_as: \"Ashen Keep\"\n---\n\nBurnt keep.",
				"---\nname: \"Stonehold\"\nera: \"Golden Age\"\nknown_as: \"The Golden Keep\"\n---\n\nGilded keep.",
				// A version with no name and no text still shows it exists.
				"---\nname: \"Stonehold\"\nera: \"Age of Dust\"\n---\n",
			},
			wantRole: permissions.RoleOwner,
		},
		{
			name:    "owner view emits them too",
			privacy: PrivacyModePermitted,
			src:     &stubEraSource{versions: stoneholdVersions()},
			contains: []string{
				"era: \"Age of Ash\"",
				"era: \"Golden Age\"",
			},
			wantRole: permissions.RoleOwner,
		},
		{
			name:    "safe swaps in the current era and emits nothing else",
			privacy: PrivacyModeSafe,
			src:     &stubEraSource{versions: stoneholdVersions()},
			contains: []string{
				"### The Golden Keep {#the-golden-keep}",
				"Gilded keep.",
			},
			absent: []string{
				"Usual keep.", "Burnt keep.", "Ashen Keep", "Stonehold", "era:", "known_as:", "Age of Ash",
			},
			wantRole: permissions.RolePlayer,
		},
		{
			name:    "safe with no current-era version keeps the usual page",
			privacy: PrivacyModeSafe,
			src: &stubEraSource{versions: map[string][]EraVersion{"e1": {
				{EraName: "Age of Ash", Current: false, Name: sp("Ashen Keep"), EntryHTML: sp("<p>Burnt keep.</p>")},
			}}},
			contains: []string{"### Stonehold {#stonehold}", "Usual keep."},
			absent:   []string{"Burnt keep.", "Ashen Keep", "era:"},
			wantRole: permissions.RolePlayer,
		},
		{
			name:    "safe current version without its own name keeps the usual name",
			privacy: PrivacyModeSafe,
			src: &stubEraSource{versions: map[string][]EraVersion{"e1": {
				{EraName: "Golden Age", Current: true, EntryHTML: sp("<p>Gilded keep.</p>")},
			}}},
			contains: []string{"### Stonehold {#stonehold}", "Gilded keep."},
			absent:   []string{"Usual keep."},
			wantRole: permissions.RolePlayer,
		},
		{
			name:     "no versions leaves the export as before",
			privacy:  PrivacyModeEverything,
			src:      &stubEraSource{},
			contains: []string{"### Stonehold {#stonehold}", "Usual keep."},
			absent:   []string{"era:"},
			wantRole: permissions.RoleOwner,
		},
		{
			name:     "a failed era read still exports the pages",
			privacy:  PrivacyModeEverything,
			src:      &stubEraSource{err: errors.New("db down")},
			contains: []string{"### Stonehold {#stonehold}", "Usual keep."},
			absent:   []string{"era:"},
			wantRole: permissions.RoleOwner,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := eraExportService(tt.src).Generate(context.Background(), "Ashfall", "owner-1", "camp-1", Options{
				Categories: []Category{CategoryEntities},
				Privacy:    tt.privacy,
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q in:\n%s", want, out)
				}
			}
			for _, bad := range tt.absent {
				if strings.Contains(out, bad) {
					t.Errorf("unexpected %q in:\n%s", bad, out)
				}
			}
			if tt.src.viewer.Role() != tt.wantRole {
				t.Errorf("era source asked as role %d, want %d", tt.src.viewer.Role(), tt.wantRole)
			}
		})
	}
}

// Versions are written as their editor text, so a hostile script in one is
// stripped like any page body on export.
func TestRenderEraVersion_SanitizesText(t *testing.T) {
	var b strings.Builder
	e := &entities.Entity{Name: "Stonehold"}
	renderEraVersion(&b, e, EraVersion{EraName: "Age of Ash", EntryHTML: sp("<p>ok</p><script>alert(1)</script>")})
	if strings.Contains(strings.ToLower(b.String()), "script") {
		t.Errorf("script leaked: %q", b.String())
	}
}
