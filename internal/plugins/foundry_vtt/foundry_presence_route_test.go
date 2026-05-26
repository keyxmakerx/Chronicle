// foundry_presence_route_test.go — NW-2.3 regression pin.
//
// Pins two invariants the relocation needs to preserve byte-for-
// behavior:
//
//  1. The endpoint is registered at `GET /campaigns/:id/foundry-presence`
//     in foundry_vtt's RegisterOwnerRoutes — NOT at any other path or
//     in any other plugin (any operator bookmark / external monitoring
//     would break otherwise).
//  2. `FoundryPresenceResponse` serialises with the JSON shape
//     `{connected: bool, never_seen: bool, last_seen?: time}` — same
//     keys the campaigns plugin emitted before the relocation.
//
// AST assertion (not a runtime HTTP test) so this stays fast + pins
// the structural invariant. Same pattern as
// internal/wire/show_banner_route_test.go + the wider AST-pin family.
package foundry_vtt

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"
)

// readSource reads name relative to the package directory. AST tests
// in this package use it to load source files for parse-and-walk.
func readSource(name string) ([]byte, error) {
	return os.ReadFile(name)
}

// TestFoundryPresenceRoute_RegisteredOnCampaignsGroup asserts the
// AST of foundry_vtt/routes.go contains a `cg.GET("/foundry-presence",
// h.GetFoundryPresenceAPI)` call inside RegisterOwnerRoutes. A future
// refactor that moves the route again (or accidentally drops it)
// fails this with a pinpointed message.
func TestFoundryPresenceRoute_RegisteredOnCampaignsGroup(t *testing.T) {
	src, err := readSource("routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "routes.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse routes.go: %v", err)
	}

	var fnBody string
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "RegisterOwnerRoutes" || fd.Body == nil {
			continue
		}
		start := fset.Position(fd.Body.Pos()).Offset
		end := fset.Position(fd.Body.End()).Offset
		fnBody = string(src[start:end])
		break
	}
	if fnBody == "" {
		t.Fatal("RegisterOwnerRoutes not found in routes.go — has it been renamed?")
	}

	// Two assertions: the path literal + the handler reference must both
	// appear inside RegisterOwnerRoutes' body. A wrapped variant
	// (e.g. cg.GET("/foundry-presence", anotherHandler)) would still
	// match the path; we also pin the handler name to catch that.
	if !strings.Contains(fnBody, `"/foundry-presence"`) {
		t.Errorf(`RegisterOwnerRoutes does not register "/foundry-presence". `+
			`The NW-2.3 relocation contract requires the URL stay at `+
			`/campaigns/:id/foundry-presence — operator bookmarks + `+
			`external monitoring depend on it. Body:\n%s`, fnBody)
	}
	if !strings.Contains(fnBody, "h.GetFoundryPresenceAPI") {
		t.Errorf("/foundry-presence path registered but not against " +
			"GetFoundryPresenceAPI. Confirm the handler the route binds to.")
	}
}

// TestFoundryPresenceResponse_JSONShape pins the wire shape of the
// response struct so a future field rename / addition shows up loudly.
// Three fields: `connected` (bool), `never_seen` (bool), `last_seen`
// (optional time). Matches the shape the campaigns plugin emitted
// pre-relocation byte-for-byte.
func TestFoundryPresenceResponse_JSONShape(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)

	t.Run("connected with last_seen", func(t *testing.T) {
		resp := FoundryPresenceResponse{
			Connected: true,
			NeverSeen: false,
			LastSeen:  &now,
		}
		got, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		want := `{"connected":true,"never_seen":false,"last_seen":"2026-05-26T12:00:00Z"}`
		if string(got) != want {
			t.Errorf("connected response shape drift:\n got %s\nwant %s", got, want)
		}
	})

	t.Run("never seen omits last_seen", func(t *testing.T) {
		resp := FoundryPresenceResponse{
			Connected: false,
			NeverSeen: true,
		}
		got, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// LastSeen is *time.Time with omitempty — nil pointer drops the
		// key entirely. The pill template differentiates "never" vs
		// "last seen Tt ago" on never_seen + presence/absence of last_seen.
		want := `{"connected":false,"never_seen":true}`
		if string(got) != want {
			t.Errorf("never-seen response shape drift:\n got %s\nwant %s", got, want)
		}
	})
}
