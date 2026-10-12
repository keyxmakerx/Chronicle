// Package wire holds Chronicle's wire-contract integrity tests.
//
// This file snapshot-tests the routing table the running app would serve:
// the test builds the real App, runs the real RegisterRoutes against an
// unreachable database and an in-memory Redis, and records every route Echo
// is handed. A scan of the source could not resolve group prefixes,
// group-level middleware or routes added by loops and builders, so a route
// silently losing its auth or rate-limit middleware went unnoticed.
// Recording at registration sees exactly what the router sees, including
// those cases.
//
// Each snapshot line is: METHOD, resolved path (group prefixes included),
// auth class, handler, and the middleware chain in execution order (group
// middleware first, then per-route), tab-separated. The auth class is derived
// from the middleware names in the chain (see authClassRules), never from a
// path or a guess.
//
// What the snapshot does not cover:
//   - Middleware is identified by its function name, not its arguments, so
//     RequireRole(RoleOwner) becoming RequireRole(RoleScribe), or one
//     RequirePermission level becoming another, leaves the line unchanged.
//   - Global middleware (Echo.Use / Echo.Pre in app setup) applies to every
//     route and is not part of a route's chain here.
//   - Routes Echo registers while the App is constructed (the /static mount)
//     predate the recording hook.
//   - Routes behind a code path the unreachable-database boot never takes are
//     absent. Every plugin counts as healthy, because a plugin with no
//     recorded health is healthy by default.
//   - Closure-valued handlers and middleware are named after the function
//     that built them, so two inline closures in one function look alike.
//
// Duplicate method+path registrations are kept in registration order: the
// later one wins in Echo, so the order is part of the contract.
//
// Building the App needs the Templ-generated code, so run `templ generate`
// first, as the build does.
//
// Update the snapshot after an intentional route or middleware change:
//
//	UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...
package wire

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	// Registers the "mysql" driver; sql.Open needs it even though no
	// connection is ever established.
	_ "github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/app"
	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// routeRecord is one route as Echo was handed it at registration.
type routeRecord struct {
	Method  string   // HTTP verb.
	Path    string   // Full path, group prefixes included.
	Handler string   // Normalised name of the handler function.
	Chain   []string // Normalised middleware names, execution order.
}

// snapshotFields is the number of tab-separated fields in a snapshot line.
const snapshotFields = 5

// emptyChain is the snapshot text for a route with no middleware. A
// placeholder keeps every line at snapshotFields fields so the shape test can
// be strict.
const emptyChain = "-"

// chainSep joins middleware names in a snapshot line. The names contain
// dots, slashes and parentheses but never this sequence.
const chainSep = " > "

// authClassRule maps a middleware name to the auth class a route carries
// when its chain includes that middleware.
type authClassRule struct {
	Middleware string
	Class      string
}

// authClassRules is ordered strongest first; the first rule whose middleware
// appears in a route's chain decides the class. The class comes from what the
// chain contains, so a route that loses its auth middleware changes class in
// the snapshot instead of keeping a hand-written label.
var authClassRules = []authClassRule{
	{"plugins/auth.RequireSiteAdmin", "site-admin"},
	{"plugins/syncapi.RequireAuthOrAPIKey", "session-or-bearer"},
	{"plugins/auth.RequireAuth", "session"},
	{"plugins/campaigns.AllowPublicCampaignAccess", "public-capable"},
	{"plugins/auth.OptionalAuth", "optional-session"},
}

// authClassNone is the class of a chain with no recognised auth middleware.
// It means "no auth middleware", not "safe": a handler can still check the
// caller itself (the websocket upgrade does).
const authClassNone = "none"

// authClass derives a route's auth class from its middleware chain.
func authClass(chain []string) string {
	for _, rule := range authClassRules {
		for _, name := range chain {
			if name == rule.Middleware {
				return rule.Class
			}
		}
	}
	return authClassNone
}

// String formats a record as a snapshot line. Keep this stable: the
// snapshot file's contents depend on it.
func (r routeRecord) String() string {
	chain := emptyChain
	if len(r.Chain) > 0 {
		chain = strings.Join(r.Chain, chainSep)
	}
	return strings.Join([]string{r.Method, r.Path, authClass(r.Chain), r.Handler, chain}, "\t")
}

// httpVerbs is the set of method names recognised as Echo route
// registrations. The shape test validates snapshot lines against it, and
// other wire tests scan source for `recv.METHOD(...)` calls with it.
var httpVerbs = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"PATCH":   true,
	"DELETE":  true,
	"HEAD":    true,
	"OPTIONS": true,
}

// closureSuffix matches the compiler's numbered closure suffixes
// (".func3", ".func3.1"). The numbers shift whenever unrelated code is added
// above a closure, so keeping them would make the snapshot churn on
// refactors that change nothing about routing.
var closureSuffix = regexp.MustCompile(`\.func\d+(\.\d+)*`)

// internalPathMarker splits Chronicle's import path from the package-relative
// name that is kept in the snapshot.
const internalPathMarker = "/internal/"

// funcName returns a stable, repo-relative name for a handler or middleware
// function value.
func funcName(f any) string {
	fn := runtime.FuncForPC(reflect.ValueOf(f).Pointer())
	if fn == nil {
		return "<unknown>"
	}
	return normaliseFuncName(fn.Name())
}

// normaliseFuncName strips the module prefix, the method-value "-fm" suffix
// and numbered closure suffixes from a runtime function name.
func normaliseFuncName(name string) string {
	if i := strings.Index(name, internalPathMarker); i >= 0 {
		name = name[i+len(internalPathMarker):]
	}
	name = strings.TrimSuffix(name, "-fm")
	return closureSuffix.ReplaceAllString(name, "")
}

// buildRouteRecords runs the real App's route registration and returns every
// route Echo was handed.
//
// No network is needed: the MariaDB handle is lazy and points at a closed
// port, so startup reconcilers fail fast and log (they are written to
// tolerate that), and Redis is in-memory. Everything that writes to disk is
// pointed at a temp directory.
func buildRouteRecords(t *testing.T) []routeRecord {
	t.Helper()

	// Startup reconcilers log their expected failures against the dead
	// database; that noise would bury a real test failure.
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	db, err := sql.Open("mysql", "wire:wire@tcp(127.0.0.1:1)/wire")
	if err != nil {
		t.Fatalf("open lazy db handle: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tmp := t.TempDir()
	cfg := &config.Config{
		Env:            "development",
		BaseURL:        "http://localhost:8080",
		LogLevel:       "error",
		TrustedProxies: middleware.DefaultTrustedProxies,
		ExtensionsPath: filepath.Join(tmp, "extensions"),
		BackupDir:      filepath.Join(tmp, "backups"),
		// Wiring requires a script path but nothing runs it at registration.
		BackupScriptPath:  filepath.Join(tmp, "backup.sh"),
		RestoreScriptPath: filepath.Join(tmp, "restore.sh"),
		Auth: config.AuthConfig{
			SecretKey:  "wire-contract-test-secret-key-0000000",
			SessionTTL: 720 * time.Hour,
		},
		Upload: config.UploadConfig{
			MaxSize:           10 * 1024 * 1024,
			MediaPath:         filepath.Join(tmp, "media"),
			SigningSecretFile: filepath.Join(tmp, "signing-secret"),
			ServeRateLimit:    300,
		},
	}

	// A fresh registry reports every plugin healthy, so no plugin's routes
	// are skipped as degraded.
	a, err := app.New(cfg, db, rdb, database.NewPluginHealthRegistry(), nil)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	// Stops the background jobs RegisterRoutes starts, so they do not outlive
	// the test and write into the removed temp dir.
	t.Cleanup(a.ShutdownCancel)

	var records []routeRecord
	a.Echo.OnAddRouteHandler = func(_ string, route echo.Route, handler echo.HandlerFunc, mws []echo.MiddlewareFunc) {
		// Group.Use makes Echo add a catch-all 404 per group so the group's
		// middleware runs on unmatched paths. They are not routes anyone
		// calls, and their chain repeats the group's real routes.
		if route.Method == echo.RouteNotFound {
			return
		}
		chain := make([]string, len(mws))
		for i, mw := range mws {
			chain[i] = funcName(mw)
		}
		records = append(records, routeRecord{
			Method:  route.Method,
			Path:    route.Path,
			Handler: funcName(handler),
			Chain:   chain,
		})
	}
	a.RegisterRoutes()

	// Path then method groups a resource's verbs together for review. The
	// stable sort keeps registration order among duplicates, where the later
	// registration is the one that serves.
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Path != records[j].Path {
			return records[i].Path < records[j].Path
		}
		return records[i].Method < records[j].Method
	})
	return records
}

// snapshotPath returns the path to the curated expected-routes file.
func snapshotPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "internal", "wire", "routes_snapshot.txt")
}

// repoRoot returns the Chronicle repo root inferred from the test's cwd.
func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// internal/wire — repo root is two dirs up.
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

// splitSnapshotLine splits a snapshot line into its fields. ok is false when
// the line does not have exactly snapshotFields of them.
func splitSnapshotLine(line string) (fields []string, ok bool) {
	fields = strings.Split(line, "\t")
	return fields, len(fields) == snapshotFields
}

// keyedLines indexes snapshot lines by "METHOD path #n", where n counts
// repeated registrations of the same method and path in file order. Keying by
// route rather than whole line lets the diff say "this route's chain
// changed" instead of reporting an unrelated removal and addition.
func keyedLines(lines []string) (keys []string, byKey map[string]string) {
	byKey = make(map[string]string, len(lines))
	seen := map[string]int{}
	for _, l := range lines {
		f, ok := splitSnapshotLine(l)
		if !ok {
			// A malformed line keys on itself so it still shows up in the diff.
			keys = append(keys, l)
			byKey[l] = l
			continue
		}
		base := f[0] + " " + f[1]
		seen[base]++
		key := fmt.Sprintf("%s #%d", base, seen[base])
		keys = append(keys, key)
		byKey[key] = l
	}
	return keys, byKey
}

// chainOf returns the middleware names of a snapshot line.
func chainOf(line string) []string {
	f, ok := splitSnapshotLine(line)
	if !ok || f[4] == emptyChain {
		return nil
	}
	return strings.Split(f[4], chainSep)
}

// subtract returns the names in a that are not in b, keeping order.
func subtract(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, n := range b {
		in[n] = true
	}
	var out []string
	for _, n := range a {
		if !in[n] {
			out = append(out, n)
		}
	}
	return out
}

// describeDrift renders the difference between the committed snapshot lines
// (want) and the live ones (got): routes added, routes removed, and routes
// whose auth class, handler or middleware chain changed. A changed route
// names the exact middleware gained or lost so a reviewer need not diff the
// long lines by eye.
func describeDrift(want, got []string) string {
	wantKeys, wantBy := keyedLines(want)
	gotKeys, gotBy := keyedLines(got)

	var added, removed, changed []string
	for _, k := range gotKeys {
		if _, ok := wantBy[k]; !ok {
			added = append(added, "  + "+gotBy[k])
		}
	}
	for _, k := range wantKeys {
		wl := wantBy[k]
		gl, ok := gotBy[k]
		switch {
		case !ok:
			removed = append(removed, "  - "+wl)
		case wl != gl:
			var b strings.Builder
			fmt.Fprintf(&b, "  ~ %s\n", k)
			wf, _ := splitSnapshotLine(wl)
			gf, _ := splitSnapshotLine(gl)
			if len(wf) == snapshotFields && len(gf) == snapshotFields {
				if wf[2] != gf[2] {
					fmt.Fprintf(&b, "      auth class: %s -> %s\n", wf[2], gf[2])
				}
				if wf[3] != gf[3] {
					fmt.Fprintf(&b, "      handler:    %s -> %s\n", wf[3], gf[3])
				}
			}
			wc, gc := chainOf(wl), chainOf(gl)
			for _, n := range subtract(wc, gc) {
				fmt.Fprintf(&b, "      middleware removed: %s\n", n)
			}
			for _, n := range subtract(gc, wc) {
				fmt.Fprintf(&b, "      middleware added:   %s\n", n)
			}
			if len(subtract(wc, gc)) == 0 && len(subtract(gc, wc)) == 0 && strings.Join(wc, chainSep) != strings.Join(gc, chainSep) {
				b.WriteString("      middleware order or repeat count changed\n")
			}
			fmt.Fprintf(&b, "      was: %s\n      now: %s", wl, gl)
			changed = append(changed, b.String())
		}
	}

	var msg strings.Builder
	msg.WriteString("wire-contract drift detected.\n\n")
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&msg, "%s (%d):\n%s\n\n", title, len(items), strings.Join(items, "\n"))
	}
	section("Routes whose auth class, handler or middleware chain changed", changed)
	section("Routes added since snapshot", added)
	section("Routes removed since snapshot", removed)
	return msg.String()
}

// TestWireContractConformance asserts that the routes the real App registers,
// with their auth class, handler and middleware chain, exactly match the
// snapshot at internal/wire/routes_snapshot.txt.
//
// Drift in either direction (a route added, removed, or given or stripped of
// a middleware without a snapshot update) fails this test. To resolve:
//
//	UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...
//
// then commit the regenerated snapshot in the same PR that introduced the
// intentional change, with the PR description explaining why.
func TestWireContractConformance(t *testing.T) {
	records := buildRouteRecords(t)
	lines := make([]string, len(records))
	for i, r := range records {
		lines[i] = r.String()
	}
	actualSnapshot := strings.Join(lines, "\n") + "\n"

	snap := snapshotPath(t)

	if os.Getenv("UPDATE_ROUTES_SNAPSHOT") != "" {
		if err := os.WriteFile(snap, []byte(actualSnapshot), 0o644); err != nil {
			t.Fatalf("write snapshot %s: %v", snap, err)
		}
		t.Logf("UPDATE_ROUTES_SNAPSHOT set — wrote %d routes to %s", len(records), snap)
		return
	}

	want, err := os.ReadFile(snap)
	if err != nil {
		t.Fatalf("read snapshot %s: %v\n"+
			"If this is the first run, regenerate with:\n"+
			"  UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...",
			snap, err)
	}

	if string(want) == actualSnapshot {
		return
	}

	wantLines := strings.Split(strings.TrimRight(string(want), "\n"), "\n")
	var msg strings.Builder
	msg.WriteString(describeDrift(wantLines, lines))
	msg.WriteString("If the drift is intentional, regenerate the snapshot:\n")
	msg.WriteString("  UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...\n\n")
	msg.WriteString("then commit the updated routes_snapshot.txt in the same PR.\n")
	msg.WriteString("Per cordinator/decisions/2026-05-21-core-tenets.md §T-O2,\n")
	msg.WriteString("the PR description must cite the decision/audit that motivated\n")
	msg.WriteString("the route change, especially if it touches an auth surface.\n")
	t.Fatal(msg.String())
}

// TestWireContractSnapshotShape sanity-checks the snapshot file format itself
// so a corrupted snapshot doesn't masquerade as a passing test.
func TestWireContractSnapshotShape(t *testing.T) {
	snap := snapshotPath(t)
	data, err := os.ReadFile(snap)
	if err != nil {
		t.Skipf("snapshot %s not present (first run); will be created by UPDATE_ROUTES_SNAPSHOT", snap)
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		t.Fatalf("snapshot %s is empty — that's suspicious; Chronicle has many routes", snap)
	}

	knownClass := map[string]bool{authClassNone: true}
	for _, rule := range authClassRules {
		knownClass[rule.Class] = true
	}

	for i, l := range lines {
		parts, ok := splitSnapshotLine(l)
		if !ok {
			t.Errorf("snapshot line %d malformed (want %d tab-separated fields, got %d): %q",
				i+1, snapshotFields, len(parts), l)
			continue
		}
		method, path, class, handler, chain := parts[0], parts[1], parts[2], parts[3], parts[4]
		if !httpVerbs[method] {
			t.Errorf("snapshot line %d: unknown HTTP method %q", i+1, method)
		}
		if path != "" && !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "*") {
			t.Errorf("snapshot line %d: path %q doesn't start with / or *", i+1, path)
		}
		if !knownClass[class] {
			t.Errorf("snapshot line %d: unknown auth class %q", i+1, class)
		}
		if handler == "" {
			t.Errorf("snapshot line %d: empty handler", i+1)
		}
		if chain == "" {
			t.Errorf("snapshot line %d: empty chain (use %q for none)", i+1, emptyChain)
		}
		// The class is derived data; a hand-edited or stale one must not pass.
		if want := authClass(chainOf(l)); class != want {
			t.Errorf("snapshot line %d: auth class %q does not match its chain (derives %q)", i+1, class, want)
		}
	}
}
