package systems

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// withLoadedSystems swaps the package-level globalLoader for a loader seeded with
// the given modules for the duration of fn, restoring the original afterward.
// Lets the LoadedHealth()-backed diagnostics (system.versions / system.files) run
// against a known fixture without touching the real registry.
func withLoadedSystems(t *testing.T, mods map[string]*loadedSystem, fn func()) {
	t.Helper()
	prev := globalLoader
	l := NewSystemLoader("")
	for id, ls := range mods {
		l.modules[id] = ls
	}
	globalLoader = l
	defer func() { globalLoader = prev }()
	fn()
}

// --- OperatorDiagnosticsAPI handler (real echo.Context via httptest) ----------

// newDiagCtx builds an echo.Context for GET /admin/diagnostics with the given
// raw query string ("" for none), returning the context + the recorder to assert
// status/body/headers against.
func newDiagCtx(query string) (echo.Context, *httptest.ResponseRecorder) {
	target := "/admin/diagnostics"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	return echo.New().NewContext(req, rec), rec
}

func TestOperatorDiagnosticsAPI_Catalog(t *testing.T) {
	h := NewSystemHandler()
	c, rec := newDiagCtx("")
	if err := h.OperatorDiagnosticsAPI(c); err != nil {
		t.Fatalf("no-name request should not error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("content-type = %q, want text/markdown", ct)
	}
	body := rec.Body.String()
	// The body is the catalog menu — it names every diagnostic.
	for _, d := range diagnosticCatalog() {
		if !strings.Contains(body, d.Name) {
			t.Errorf("catalog body missing diagnostic %q", d.Name)
		}
	}
}

func TestOperatorDiagnosticsAPI_NamedDiagnostic(t *testing.T) {
	h := NewSystemHandler()
	c, rec := newDiagCtx("name=system.versions")
	if err := h.OperatorDiagnosticsAPI(c); err != nil {
		t.Fatalf("known diagnostic should not error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("content-type = %q, want text/markdown", ct)
	}
	if !strings.Contains(rec.Body.String(), "system.versions") {
		t.Errorf("body should contain the diagnostic heading, got %q", rec.Body.String())
	}
}

func TestOperatorDiagnosticsAPI_UnknownDiagnostic(t *testing.T) {
	h := NewSystemHandler()
	c, rec := newDiagCtx("name=does.not.exist")
	err := h.OperatorDiagnosticsAPI(c)
	if err == nil {
		t.Fatal("unknown diagnostic should return an error (404), got nil")
	}
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error should be an *apperror.AppError, got %T", err)
	}
	if ae.Code != http.StatusNotFound {
		t.Errorf("error code = %d, want 404", ae.Code)
	}
	// The handler must NOT have written a success body itself — Echo's error
	// handler renders the 404.
	if rec.Code == http.StatusOK && rec.Body.Len() > 0 {
		t.Errorf("handler wrote a body for the unknown-name case: %q", rec.Body.String())
	}
}

// --- redactSecrets additional cases ------------------------------------------

func TestRedactSecrets_MultiLineOnlySecretLine(t *testing.T) {
	in := strings.Join([]string{
		"loaded_version: 0.13.0",
		"DB_PASSWORD=hunter2",
		"dir: /app/media/packages/systems/drawsteel",
	}, "\n")
	got := redactSecrets(in)
	if strings.Contains(got, "hunter2") {
		t.Errorf("secret value survived redaction: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("expected the secret line to be redacted: %q", got)
	}
	// The surrounding, non-secret lines must be untouched.
	if !strings.Contains(got, "loaded_version: 0.13.0") {
		t.Errorf("non-secret line above was mangled: %q", got)
	}
	if !strings.Contains(got, "dir: /app/media/packages/systems/drawsteel") {
		t.Errorf("non-secret line below was mangled: %q", got)
	}
}

func TestRedactSecrets_CaseInsensitive(t *testing.T) {
	for _, in := range []string{
		"PASSWORD=hunter2",
		"password=hunter2",
		"Api_Key: sk-XYZ",
		"AUTHORIZATION: Bearer eyJ",
	} {
		if got := redactSecrets(in); !strings.Contains(got, "[REDACTED]") {
			t.Errorf("redactSecrets(%q) should be case-insensitive, got %q", in, got)
		}
	}
}

func TestRedactSecrets_NoSeparatorNotRedacted(t *testing.T) {
	// A bare keyword with no [:=] separator + value is prose, not a credential.
	for _, in := range []string{
		"the password field is required",
		"rotate your api key regularly",
		"token",
	} {
		if got := redactSecrets(in); strings.Contains(got, "[REDACTED]") {
			t.Errorf("redactSecrets(%q) should NOT redact (no separator), got %q", in, got)
		}
	}
}

func TestRedactSecrets_Sha256HashSurvives(t *testing.T) {
	// A fingerprint line "sha256: <hex>" is exactly the diagnostic payload and must
	// never be mistaken for a secret.
	in := "- `manifest.json` — 42 · `deadbeefcafe1234` · 2026-06-26T00:00:00Z\nsha256: deadbeefcafe1234"
	got := redactSecrets(in)
	if strings.Contains(got, "[REDACTED]") {
		t.Errorf("sha256 hash line was wrongly redacted: %q", got)
	}
	if !strings.Contains(got, "deadbeefcafe1234") {
		t.Errorf("hash value was lost: %q", got)
	}
}

// --- fingerprintFiles edge cases ---------------------------------------------

func TestFingerprintFiles_DirectoryRelPathNotExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	// "widgets" is a directory, not a served file — must be Exists=false (the
	// !info.IsDir() guard), with no size/hash.
	got := fingerprintFiles(dir, []string{"widgets"})
	if len(got) != 1 {
		t.Fatalf("expected 1 fingerprint, got %d", len(got))
	}
	if got[0].Exists {
		t.Errorf("a directory relPath must be Exists=false, got %+v", got[0])
	}
	if got[0].SHA256 != "" || got[0].Size != 0 {
		t.Errorf("a directory must carry no size/hash, got %+v", got[0])
	}
}

func TestFingerprintFiles_EmptyArgs(t *testing.T) {
	dir := t.TempDir()
	// Empty rel-path list → empty (but non-nil) result.
	if got := fingerprintFiles(dir, nil); len(got) != 0 {
		t.Errorf("nil relPaths should yield 0 fingerprints, got %d", len(got))
	}
	// An empty-string relPath is skipped by the `rel != ""` guard → Exists=false.
	got := fingerprintFiles(dir, []string{""})
	if len(got) != 1 || got[0].Exists {
		t.Errorf("empty relPath should yield one Exists=false entry, got %+v", got)
	}
	// An empty dir argument likewise produces a non-existent fingerprint.
	got = fingerprintFiles("", []string{"manifest.json"})
	if len(got) != 1 || got[0].Exists {
		t.Errorf("empty dir should yield one Exists=false entry, got %+v", got)
	}
}

func TestFingerprintFiles_IdenticalContentSameHash(t *testing.T) {
	dir := t.TempDir()
	content := []byte("identical-bytes\n")
	if err := os.WriteFile(filepath.Join(dir, "a.js"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.js"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	fps := fingerprintFiles(dir, []string{"a.js", "b.js"})
	if len(fps) != 2 {
		t.Fatalf("expected 2 fingerprints, got %d", len(fps))
	}
	if !fps[0].Exists || !fps[1].Exists {
		t.Fatalf("both files should exist: %+v", fps)
	}
	if fps[0].SHA256 != fps[1].SHA256 {
		t.Errorf("identical content must hash identically: %q vs %q", fps[0].SHA256, fps[1].SHA256)
	}
}

// --- system.files diagnostic (missing / empty / present id) -------------------

func newDrawsteelFixture(t *testing.T) (string, map[string]*loadedSystem) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"id":"drawsteel"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "widgets", "character-sheet.js"), []byte("// sheet"), 0o644); err != nil {
		t.Fatal(err)
	}
	mods := map[string]*loadedSystem{
		"drawsteel": {
			manifest: &SystemManifest{
				ID: "drawsteel", Name: "Draw Steel", Version: "0.13.0",
				Widgets: []WidgetDef{{Slug: "character-sheet", ScriptFile: "widgets/character-sheet.js"}},
			},
			dir:    dir,
			source: "package",
		},
	}
	return dir, mods
}

func TestSystemFilesDiagnostic_PresentID(t *testing.T) {
	_, mods := newDrawsteelFixture(t)
	withLoadedSystems(t, mods, func() {
		out, ok := RunDiagnostic(diagnosticCatalog(), "system.files", "drawsteel")
		if !ok {
			t.Fatal("system.files should dispatch")
		}
		// Lists the served version + each fingerprinted file.
		if !strings.Contains(out, "0.13.0") {
			t.Errorf("present-id output should name the version: %q", out)
		}
		if !strings.Contains(out, "manifest.json") || !strings.Contains(out, "widgets/character-sheet.js") {
			t.Errorf("present-id output should list both served files: %q", out)
		}
		if strings.Contains(out, "No loaded system") {
			t.Errorf("present id should not report 'no loaded system': %q", out)
		}
	})
}

func TestSystemFilesDiagnostic_MissingID(t *testing.T) {
	_, mods := newDrawsteelFixture(t)
	withLoadedSystems(t, mods, func() {
		out, ok := RunDiagnostic(diagnosticCatalog(), "system.files", "nope")
		if !ok {
			t.Fatal("system.files should dispatch even for an unknown arg")
		}
		if !strings.Contains(out, "No loaded system with id") {
			t.Errorf("missing id should report 'no loaded system', got %q", out)
		}
	})
}

func TestSystemFilesDiagnostic_EmptyArgListsIDs(t *testing.T) {
	_, mods := newDrawsteelFixture(t)
	withLoadedSystems(t, mods, func() {
		out, ok := RunDiagnostic(diagnosticCatalog(), "system.files", "")
		if !ok {
			t.Fatal("system.files should dispatch with an empty arg")
		}
		// Empty arg prompts for an id and lists the loaded ones.
		if !strings.Contains(out, "Needs a system id") {
			t.Errorf("empty arg should prompt for a system id, got %q", out)
		}
		if !strings.Contains(out, "drawsteel") {
			t.Errorf("empty arg should list loaded ids, got %q", out)
		}
	})
}

func TestSystemVersionsDiagnostic_ListsLoaded(t *testing.T) {
	_, mods := newDrawsteelFixture(t)
	withLoadedSystems(t, mods, func() {
		out, ok := RunDiagnostic(diagnosticCatalog(), "system.versions", "")
		if !ok {
			t.Fatal("system.versions should dispatch")
		}
		if !strings.Contains(out, "drawsteel") || !strings.Contains(out, "0.13.0") {
			t.Errorf("system.versions should list the loaded system + version: %q", out)
		}
	})
}
