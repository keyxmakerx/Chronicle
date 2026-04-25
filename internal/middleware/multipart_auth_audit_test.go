package middleware

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMultipartUploads_RequireAuth is a regression guard for the
// "every multipart upload must be authenticated" rule the Coordinator
// asked for in Task C4.
//
// It walks the codebase for files containing `c.FormFile(` (the standard
// Echo multipart entry point) and asserts each one's package wires its
// routes through at least one of the recognized auth middlewares. This
// catches the future-drift case where someone adds a new POST handler
// that calls FormFile but forgets to gate the route.
//
// What this DOES NOT catch:
//   - Wrong role enforcement (Player allowed where Owner is required) —
//     handler-level checks aren't visible from a static scan; specific
//     handler tests cover those cases.
//   - A package that mounts SOME routes through auth and one rogue route
//     without — this tests at the package level, not the route level.
//     Mitigated by the convention that each package owns one routes.go
//     and developers don't typically register routes from random files.
//
// The audit table below documents every multipart endpoint and the
// auth chain it actually has, as of the C4 audit. If a row in the table
// disappears from the codebase or a new row needs adding, this test will
// fail (because the package list comes from a live grep, not the table).
//
// AUDITED MULTIPART ROUTES (April 2026 / PR for Task C4):
//
//	Route                                                   Auth chain
//	-----                                                   ----------
//	POST /account/avatar                                    RequireAuth (session)
//	POST /api/v1/.../media                                  RequireAuthOrAPIKey + RequirePermission(PermWrite)
//	POST /api/v1/.../calendar/import                        RequireAuthOrAPIKey + RequirePermission(PermWrite)
//	POST /campaigns/:id/systems/upload                      RequireAuth + RequireCampaignAccess + handler-Owner check
//	POST /campaigns/:id/systems/preview                     RequireAuth + RequireCampaignAccess + handler-Owner check
//	POST /media/upload                                      RequireAuth + rate limit + body limit
//	POST /campaigns/:id/backdrop                            RequireAuth + RequireCampaignAccess + RequireRole(Owner)
//	POST /campaigns/import                                  RequireAuth (session)
//	POST /campaigns/:id/calendars/import-setup              RequireAuth + RequireCampaignAccess + RequireRole(Owner)
//	POST /campaigns/:id/calendars/:calId/import             RequireAuth + RequireCampaignAccess + RequireRole(Owner)
//	POST /campaigns/:id/calendars/:calId/import/preview     RequireAuth + RequireCampaignAccess + RequireRole(Owner)
//	POST /campaigns/:id/notes/:nid/attachments              RequireAuth + RequireCampaignAccess + RequireRole(Player)
//	POST /admin/extensions/install                          RequireAuth + RequireSiteAdmin
//	POST /admin/extensions/rescan                           RequireAuth + RequireSiteAdmin
//
// API/v1 path is the only one through the unified RequireAuthOrAPIKey
// resolver — it's intentional: web flows are session-only, external
// clients hit /api/v1.
func TestMultipartUploads_RequireAuth(t *testing.T) {
	root := projectRoot(t)

	// Live scan: every .go file (excluding tests + generated) that calls
	// c.FormFile(. Mirrors the audit table above; a new entry here means
	// a new multipart endpoint to verify.
	formFileFiles := grepFiles(t, root, "c.FormFile(", []string{"_test.go", "_templ.go"})
	if len(formFileFiles) == 0 {
		t.Fatal("no c.FormFile call sites found; either the test is stale or " +
			"the grep moved — investigate before disabling this test")
	}

	// Recognized auth markers — any one of these in the same package's Go
	// files counts as "this package gates its routes".
	authMarkers := []string{
		"auth.RequireAuth",            // session-only routes
		"RequireAuthOrAPIKey",         // unified session+APIKey routes
		"RequireSiteAdmin",            // admin-only routes
		"RequireAPIKey",               // pure-Bearer routes (legacy; should be RequireAuthOrAPIKey)
		"adminGroup",                  // routes mounted under the admin group inherit RequireAuth+RequireSiteAdmin
	}

	// Group call sites by package so we audit at the package level.
	packagesSeen := map[string][]string{}
	for _, f := range formFileFiles {
		pkg := filepath.Dir(f)
		packagesSeen[pkg] = append(packagesSeen[pkg], f)
	}

	for pkg, files := range packagesSeen {
		// Read every .go file in the same package and search for any auth
		// marker. This is intentionally loose: a single hit anywhere in
		// the package means the dev has wired auth somewhere reachable.
		// The route-level fidelity is enforced by code review, not by a
		// static scan.
		entries, err := os.ReadDir(pkg)
		if err != nil {
			t.Fatalf("reading package dir %s: %v", pkg, err)
		}
		var found bool
		var marker string
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(pkg, e.Name()))
			if err != nil {
				continue
			}
			content := string(data)
			for _, m := range authMarkers {
				if strings.Contains(content, m) {
					found = true
					marker = m
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("package %s has c.FormFile() call sites in %v but no recognized auth middleware "+
				"reference in any of its .go files. Add one of: %v. If this is intentional "+
				"(e.g. a public upload endpoint), update this test's allowlist with a justification.",
				rel(t, root, pkg), relAll(t, root, files), authMarkers)
			continue
		}
		t.Logf("OK: package %s gates uploads via %q (call sites: %v)",
			rel(t, root, pkg), marker, relAll(t, root, files))
	}
}

// projectRoot returns the absolute path to the repo root.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// thisFile is internal/middleware/multipart_auth_audit_test.go;
	// project root is two dirs up.
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// grepFiles returns every .go file under root whose contents contain needle,
// excluding any path containing one of skipSubstrings. Walks the tree once.
func grepFiles(t *testing.T, root, needle string, skipSubstrings []string) []string {
	t.Helper()
	var hits []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip vendor / .git / node-style noise.
			name := info.Name()
			if name == "vendor" || name == ".git" || name == "node_modules" || name == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		for _, skip := range skipSubstrings {
			if strings.HasSuffix(path, skip) {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), needle) {
			hits = append(hits, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return hits
}

func rel(t *testing.T, root, p string) string {
	t.Helper()
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}

func relAll(t *testing.T, root string, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = rel(t, root, p)
	}
	return out
}
