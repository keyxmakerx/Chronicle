// partial_update_contract_test.go is a structural ratchet for the
// absent-means-preserve contract: an update input can only preserve an absent
// key if its fields can represent absence, so this pins representability,
// whole-tree, in two directions:
//
//  1. Every field of a CONTRACT-GOVERNED input must be able to say "absent":
//     patch.Field[T], a pointer, a map or a slice. A value-typed field cannot
//     land on one of these structs without a NAMED exception below.
//  2. The full inventory of Update*Input structs in internal/ is frozen. A
//     NEW one must be added to one list or the other, which forces its author
//     to decide — out loud — whether it is a partial update.
//
// It does not replace per-endpoint regression tests (absent preserves ·
// present replaces · explicit null clears); the convention is in
// .ai/conventions.md.
package patch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// contractGoverned lists update inputs that must be presence-aware.
var contractGoverned = map[string]string{
	"sessions.UpdateSessionInput":       "PUT /campaigns/:id/sessions/:sid — Mark Complete wiped the schedule, summary, in-world date and recurrence",
	"entities.UpdateEntityInput":        "PUT /entities/:eid and the syncapi twin — every sync push un-parented the entity; {name} alone un-privated it",
	"timeline.UpdateTimelineEventInput": "PUT .../standalone-events/:eid — a rename cleared eight fields, including per-player visibility rules",
	"maps.UpdateMarkerInput":            "PUT .../markers/:mkid — an edit or a drag cleared pin_category, visibility_rules and the Foundry pairing key",
	// CALV5 SALVAGE: restored with the domain layer, already presence-aware,
	// so it stays on the governed list rather than the allowlist.
	"calendar.UpdateEventInput": "PUT .../calendar/events/:eid — Foundry's five-key push turned off recurrence, all-day and the entity link",
	// PUT .../calendars/:calid is a settings save covering thirteen fields
	// (hours-per-day, leap-year rules, the current date/time, real-time
	// toggle...); a narrow push (e.g. rename-only) must not reset any of them.
	"calendar.UpdateCalendarInput": "PUT .../calendars/:calid — a settings-only push (e.g. rename) must not reset current date/time, leap-year rules or the hours-per-day/minute/second geometry",
	// Visibility stays required (see governedFieldExceptions); VisibilityRules
	// is the field this struct exists to protect — a visibility-flip call
	// must not silently wipe an existing per-user allow/deny list.
	"calendar.UpdateEventVisibilityInput":    "PUT .../calendar/events/:eid/visibility — a visibility-only flip (everyone <-> dm_only) must not clear an existing per-user visibility_rules blob",
	"calendar.UpdateCalendarVisibilityInput": "same shape and same reason as calendar.UpdateEventVisibilityInput — the calendar reuses the event visibility model",
	"calendar.UpdateEraInput":                "PUT .../eras/:eraID — a rename-only push must not reset the era's start/end dates, description or color",
	"calendar.UpdateEventKindInput":          "PUT .../event-kinds/:kindID — a rename-only push must not reset the kind's icon, color or default_announced",

	"systems.UpdateBookChapterInput": "PUT .../systems/:mod/book/chapters/:chapter — a rename-only push must not blank the house-rules chapter's introduction or flip its Directors-only flag, which would publish a hidden chapter to every player",

	// Each is pinned by a *_partial_update_test.go next to it.
	"maps.UpdateTokenInput":        "PUT .../tokens/:tid (web + syncapi) — a drag PUT carrying only {x, y} zeroed IsHidden, IsLocked, both HP bars and every aura/light/vision field; a hidden ambush monster went visible on the next nudge",
	"maps.UpdateDrawingInput":      "PUT .../drawings/:did (web + syncapi) — shares UpdateTokenInput's shape; a reshape-only push wiped fill, text content, font size and rotation. No shipped caller trips it today, fixed anyway under the partial-update contract",
	"maps.UpdateLayerInput":        "PUT .../layers/:lid (web + syncapi) — a SortOrder-only reorder push turned visibility and lock off for every layer. No shipped caller trips it today, fixed anyway under the partial-update contract",
	"maps.UpdateMapInput":          "PUT /campaigns/:id/maps/:mid — a rename-only push unlinked the map's image and wiped its description; ImageID/Description were already *string and STILL blindly overwritten, because a plain pointer bound from JSON can't tell absent from null either",
	"timeline.UpdateTimelineInput": "PUT /campaigns/:id/timelines/:tid — fired on EVERY settings save, not just a narrow push: the request struct has no visibility_rules/description_html member at all, so both were unconditionally blanked and canUserView() treats an absent VisibilityRules as visible to everyone",
	"tags.UpdateTagInput":          "tagService.Update — the worst finding of the 2026-09-12 toggle-truth sweep (ADR-056): Color/DmOnly were plain value types, so ANY rename necessarily also sent DmOnly's zero value and turned a DM-only tag public",
	"armory.UpdateStashInput":      "PUT /campaigns/:id/armory/stashes/:sid — a rename must not clear the stash location; only an explicit null does",
	"tags.UpdateTagRequest":        "PUT /campaigns/:id/tags/:tagId — the wire-bound twin of UpdateTagInput above; same incident, same fix",
	"maps.UpdateHexCellInput":      "PATCH .../maps/:mid/hexes/cells — a paint stroke sends only terrain, so it must not touch a hex's name or notes; a rename must not clear its terrain",
	"maps.UpdateHexLayerInput":     "PUT .../maps/:mid/hexes/layer — a push naming nothing about the anchor must not move the hexes off their picture; only an explicit null puts them back on the whole map",
	"auth.UpdateViewPrefsInput":    "PUT /account/view-prefs — each My view choice saves on its own as it is tapped, so a body naming one must not reset the other three (born governed, no incident)",
}

// governedFieldExceptions are value-typed fields deliberately left on a
// governed struct. Each needs a reason, and the reason has to be a fact.
var governedFieldExceptions = map[string]string{
	"maps.UpdateHexCellInput.Col":          "value-typed by choice: Col and Row are the hex's identity, not data to merge. The handler refuses an entry that omits either, and the service bounds them to 0..MaxHexCoord.",
	"maps.UpdateHexCellInput.Row":          "value-typed by choice: same as maps.UpdateHexCellInput.Col.",
	"entities.UpdateEntityInput.ImagePath": "INERT — entityService.Update never reads it. That is its own defect (campaign import believes it is applying image paths through this input and is not); tracked as #613 rather than fixed under a ruling that was about a different bug. It cannot clobber anything precisely because nothing reads it.",

	// Update only assigns Name when non-empty, so it already preserves an
	// absent/blank name without needing presence-awareness.
	"maps.UpdateTokenInput.Name": "value-typed by choice: UpdateToken only assigns Name when it is non-empty, so an absent/blank name already preserved the stored one before this fix.",
	"maps.UpdateLayerInput.Name": "value-typed by choice: UpdateLayer only assigns Name when it is non-empty, so an absent/blank name already preserved the stored one before this fix.",
	// On these three, Name is required — Update rejects a blank merged name
	// with 400, failing loudly instead of silently overwriting.
	"maps.UpdateMapInput.Name":           "value-typed by choice: UpdateMap validates the merged name is non-empty and rejects the whole call with 400 when it is blank, so an absent name fails loudly rather than silently overwriting.",
	"timeline.UpdateTimelineInput.Name":  "value-typed by choice: UpdateTimeline validates the merged name is non-empty and rejects the whole call with 400 when it is blank, so an absent name fails loudly rather than silently overwriting.",
	"tags.UpdateTagInput.Name":           "value-typed by choice: tagService.Update validates the merged name is non-empty and rejects the whole call with 400 when it is blank, so an absent name fails loudly rather than silently overwriting.",
	"tags.UpdateTagRequest.Name":         "value-typed by choice: the same required-name validation applies via UpdateTagInput.Name above — this is the wire-bound twin.",
	"calendar.UpdateCalendarInput.Name":  "value-typed by choice: UpdateCalendar validates the merged name is non-empty and rejects the whole call with 400 when it is blank, so an absent name fails loudly rather than silently overwriting.",
	"calendar.UpdateEraInput.Name":       "value-typed by choice: UpdateEra validates the merged name is non-empty and rejects the whole call with 400 when it is blank, so an absent name fails loudly rather than silently overwriting.",
	"calendar.UpdateEventKindInput.Name": "value-typed by choice: UpdateEventKind validates the merged name is non-empty and rejects the whole call with 400 when it is blank, so an absent name fails loudly rather than silently overwriting.",

	// These are action endpoints ("set visibility"), not general settings
	// saves — every call states the new visibility outright, so there is no
	// absent-preserves case for it. VisibilityRules on both structs is
	// presence-aware (patch.Field), which is the field the fix is actually
	// about.
	"calendar.UpdateEventVisibilityInput.Visibility":    "value-typed by choice: SetEventVisibility is a dedicated set-visibility action, not a general update — the caller always states the event's new visibility outright.",
	"calendar.UpdateCalendarVisibilityInput.Visibility": "value-typed by choice: same reason as calendar.UpdateEventVisibilityInput.Visibility — a dedicated set-visibility action, not a general update.",
}

// notYetSwept freezes the rest of the inventory. Being on this list is a
// statement about what was looked at, not a claim of safety. Removing a name
// means the struct became contract-governed; adding one means a new update
// input shipped and its author decided it is not a partial update.
var notYetSwept = map[string]bool{
	"packages.UpdatePolicyInput":          true,
	"packages.UpdateRepoURLInput":         true,
	"bestiary.UpdatePublicationInput":     true,
	"timeline.UpdateEntityGroupInput":     true,
	"timeline.UpdateEventVisibilityInput": true,
	"addons.UpdateAddonInput":             true,
	"entities.UpdateLayoutPresetInput":    true,
	"entities.UpdateContentTemplateInput": true,
	"entities.UpdateEntityTypeInput":      true,
	"entities.UpdatePromptInput":          true,
	"maps.UpdateTokenPositionInput":       true,
	"campaigns.UpdateCampaignInput":       true,

	// The scanner covers Update*Input and Update*Request (ADR-056). These are
	// unaudited, not verified safe — several (UpdateEntityRequest,
	// UpdateEntityTypeRequest, UpdateSMTPRequest) look like good candidates
	// for the next sweep.
	"campaigns.UpdateCampaignRequest":         true,
	"campaigns.UpdateRoleRequest":             true,
	"campaigns.UpdateSidebarConfigRequest":    true,
	"entities.UpdateEntityRequest":            true,
	"entities.UpdateEntityTypeRequest":        true,
	"entity_notes.UpdateNoteRequest":          true,
	"notes.UpdateNoteRequest":                 true,
	"posts.UpdatePostRequest":                 true,
	"relations.UpdateRelationMetadataRequest": true,
	"smtp.UpdateSMTPRequest":                  true,
}

type inputStruct struct {
	qualified string // "sessions.UpdateSessionInput"
	file      string
	fields    []inputField
}

type inputField struct {
	name       string
	typeString string
	line       int
}

// TestPartialUpdateContract_GovernedInputsCanRepresentAbsence is direction 1.
func TestPartialUpdateContract_GovernedInputsCanRepresentAbsence(t *testing.T) {
	found := scanUpdateInputs(t)
	for name, why := range contractGoverned {
		st, ok := found[name]
		if !ok {
			t.Errorf("%s is contract-governed but no longer exists in the tree (%s). If it was renamed, rename it here too; if it was deleted, delete this entry.", name, why)
			continue
		}
		for _, f := range st.fields {
			if presenceAware(f.typeString) {
				continue
			}
			key := name + "." + f.name
			if reason, exempt := governedFieldExceptions[key]; exempt {
				t.Logf("%s: value-typed by exception — %s", key, reason)
				continue
			}
			t.Errorf(
				"%s:%d — %s.%s is %s, a type with no ABSENT state.\n"+
					"  This struct is a PARTIAL update (%s), so a caller that omits this key must\n"+
					"  leave the stored value alone — and the service cannot do that if the field\n"+
					"  cannot tell 'absent' from the zero value. Use patch.Field[%s] (absent\n"+
					"  preserves, explicit null clears, a value replaces), or add a NAMED entry to\n"+
					"  governedFieldExceptions saying why this one is different.",
				st.file, f.line, name, f.name, f.typeString, why, f.typeString,
			)
		}
	}
}

// TestPartialUpdateContract_InventoryIsFrozen is direction 2.
func TestPartialUpdateContract_InventoryIsFrozen(t *testing.T) {
	found := scanUpdateInputs(t)
	var unclassified []string
	for name, st := range found {
		if _, ok := contractGoverned[name]; ok {
			continue
		}
		if notYetSwept[name] {
			continue
		}
		valueTyped := 0
		for _, f := range st.fields {
			if !presenceAware(f.typeString) {
				valueTyped++
			}
		}
		unclassified = append(unclassified, name+" ("+st.file+", "+itoa(valueTyped)+" value-typed field(s) today)")
	}
	sort.Strings(unclassified)
	for _, u := range unclassified {
		t.Errorf(
			"new update input %s is in neither list.\n"+
				"  Decide out loud: if it is a PARTIAL update, make its fields presence-aware and add\n"+
				"  it to contractGoverned; if it is a full replace, add it to notYetSwept. The count\n"+
				"  above is what the guard measured, not a verdict — a value-typed field is only a\n"+
				"  bug if some caller omits the key.", u,
		)
	}

	for name := range notYetSwept {
		if _, ok := found[name]; !ok {
			t.Errorf("notYetSwept lists %s, which no longer exists. Stale allowlist entries are how a ratchet stops ratcheting.", name)
		}
	}
	for name := range notYetSwept {
		if _, dup := contractGoverned[name]; dup {
			t.Errorf("%s is in BOTH lists; one of them is wrong", name)
		}
	}
}

// presenceAware reports whether a Go type can represent "the caller did not
// send this". Pointers, maps and slices carry nil; patch.Field carries an
// explicit presence bit.
func presenceAware(typeString string) bool {
	switch {
	case strings.HasPrefix(typeString, "*"),
		strings.HasPrefix(typeString, "[]"),
		strings.HasPrefix(typeString, "map["),
		strings.HasPrefix(typeString, "patch.Field["):
		return true
	}
	return false
}

// scanUpdateInputs parses every non-test .go file under internal/ and returns
// the Update*Input struct declarations keyed by "package.TypeName".
func scanUpdateInputs(t *testing.T) map[string]inputStruct {
	t.Helper()
	root := repoRoot(t)
	out := map[string]inputStruct{}
	fset := token.NewFileSet()

	err := filepath.Walk(filepath.Join(root, "internal"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_templ.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil // unparseable files are not this guard's business
		}
		pkg := file.Name.Name
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || !strings.HasPrefix(ts.Name.Name, "Update") ||
				(!strings.HasSuffix(ts.Name.Name, "Input") && !strings.HasSuffix(ts.Name.Name, "Request")) {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			rec := inputStruct{qualified: pkg + "." + ts.Name.Name, file: rel}
			for _, f := range st.Fields.List {
				typeStr := exprString(f.Type)
				for _, nm := range f.Names {
					rec.fields = append(rec.fields, inputField{
						name: nm.Name, typeString: typeStr, line: fset.Position(nm.Pos()).Line,
					})
				}
				if len(f.Names) == 0 { // embedded
					rec.fields = append(rec.fields, inputField{
						name: typeStr, typeString: typeStr, line: fset.Position(f.Pos()).Line,
					})
				}
			}
			out[rec.qualified] = rec
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal/: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("found no Update*Input structs at all — the scanner is broken, and a broken scanner is a guard that passes by not looking")
	}
	return out
}

// exprString renders a type expression the way the source spells it, which is
// all presenceAware needs.
func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprString(v.Elt)
	case *ast.MapType:
		return "map[" + exprString(v.Key) + "]" + exprString(v.Value)
	case *ast.IndexExpr: // patch.Field[string]
		return exprString(v.X) + "[" + exprString(v.Index) + "]"
	case *ast.InterfaceType:
		return "any"
	case *ast.FuncType:
		return "func"
	default:
		return "?"
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find the repo root (no go.mod above the test's working directory)")
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
