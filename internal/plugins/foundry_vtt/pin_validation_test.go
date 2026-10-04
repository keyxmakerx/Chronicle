package foundry_vtt

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// pinPkgReader serves one foundry package with a fixed set of known
// versions, resolving install dirs under root for valid versions only.
type pinPkgReader struct {
	root  string
	known []string
}

func (r *pinPkgReader) ListPackages(_ context.Context) ([]packages.Package, error) {
	return []packages.Package{{ID: "p1", Slug: "mod", Type: packages.PackageTypeFoundryModule, InstalledVersion: "1.0.0"}}, nil
}
func (r *pinPkgReader) GetPackage(_ context.Context, _ string) (*packages.Package, error) {
	return nil, nil
}
func (r *pinPkgReader) InstallDirForVersion(_ packages.PackageType, _, v string) string {
	if !packages.ValidVersionString(v) {
		return ""
	}
	return filepath.Join(r.root, v)
}
func (r *pinPkgReader) ListVersions(_ context.Context, _ string) ([]packages.PackageVersion, error) {
	var out []packages.PackageVersion
	for _, v := range r.known {
		out = append(out, packages.PackageVersion{Version: v})
	}
	return out, nil
}

func TestSetPinnedVersion_Validation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 2.0.0 is known to the catalog but has no folder.
	rd := &pinPkgReader{root: root, known: []string{"1.0.0", "2.0.0"}}

	tests := []struct {
		name       string
		version    string
		wantErr    bool
		wantValid  bool // apperror validation error
		wantStored string
	}{
		{"known and installed", "1.0.0", false, false, "1.0.0"},
		{"empty clears", "", false, false, ""},
		{"traversal", "../x", true, true, ""},
		{"embedded traversal", "1.0/../2.0", true, true, ""},
		{"dot dot", "..", true, true, ""},
		{"slash", "a/b", true, true, ""},
		{"backslash", `a\b`, true, true, ""},
		{"too long", strings.Repeat("1", 51), true, true, ""},
		{"well-formed but unknown", "9.9.9", true, false, ""},
		{"known but not on disk", "2.0.0", true, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeSettings{}
			svc := &service{pkgs: rd, settings: fs}
			err := svc.SetPinnedVersion(context.Background(), "c1", tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantValid {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusUnprocessableEntity {
					t.Errorf("want apperror validation error, got %#v", err)
				}
			}
			if fs.pin != tt.wantStored {
				t.Errorf("stored pin = %q, want %q", fs.pin, tt.wantStored)
			}
		})
	}
}

func TestResolveCampaignManifest_InvalidStoredPin(t *testing.T) {
	for _, pin := range []string{"../x", "..", "1.0/../2.0", `a\b`, strings.Repeat("1", 51)} {
		t.Run(pin, func(t *testing.T) {
			svc := &service{
				pkgs:     &pinPkgReader{root: t.TempDir()},
				settings: &fakeSettings{pin: pin},
			}
			_, err := svc.resolveCampaignManifest(context.Background(), "c1")
			var fe *Error
			if !errors.As(err, &fe) || fe.Code != "pinned_version_not_installed" {
				t.Fatalf("want pinned_version_not_installed, got %v", err)
			}
		})
	}
}
