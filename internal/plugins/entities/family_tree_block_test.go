package entities

import (
	"context"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func renderFamily(t *testing.T, tree *FamilyTree, dm bool) string {
	t.Helper()
	var sb strings.Builder
	if err := BlockFamilyTree("c1", "maud", "Maud", tree, dm).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// The rendered block is the last place a secret could slip through: for a
// player it holds no Director-only mark, name or id, in the markup or in the
// JSON the script reads; for the Director the same data carries the DM mark.
func TestFamilyBlockRendersOnlyWhatTheViewerMayHave(t *testing.T) {
	// The Edit tab, its save bar and its endpoints are the Director's: none of
	// them reaches a player's markup.
	editing := []string{"data-fm-edit", "data-fm-savebar", "data-fm-undo", "maud/family", "family/candidates", `data-edit-url="/`, `data-pick-url="/`}
	secrets := append([]string{"Corwin Mere", "Veiled Mother", "Old Steward", "Great Aunt Ysmere", `"dm":true`, "data-dm-only"}, editing...)
	tests := []struct {
		name     string
		role     int
		dm       bool
		contains []string
		absent   []string
	}{
		{
			name: "player", role: int(permissions.RolePlayer),
			contains: []string{`class="fm"`, `data-dm="0"`, "Ione", "Tam", "6 relatives", "open the family tree", `aria-label="Family tree"`},
			absent:   secrets,
		},
		{
			name: "director", role: int(permissions.RoleOwner), dm: true,
			contains: []string{`data-dm="1"`, "Corwin Mere", `"dm":true`, "data-dm-only", "hidden from players", "GM",
				"data-fm-edit", "data-fm-savebar", `data-edit-url="/campaigns/c1/entities/maud/family"`, `data-pick-url="/campaigns/c1/entities/maud/family/candidates"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := secretsFixture().service().Tree(context.Background(), "c1", "maud", tt.role, "u1")
			if err != nil {
				t.Fatal(err)
			}
			if tt.dm {
				// the count is the Director's own, larger, tree
				tt.contains = append(tt.contains, familyCountLabel(tree))
			}
			// Unescaped, so the JSON in data-json reads as it does in the browser.
			out := html.UnescapeString(renderFamily(t, tree, tt.dm))
			for _, w := range tt.contains {
				if !strings.Contains(out, w) {
					t.Errorf("block lacks %q", w)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(out, a) {
					t.Errorf("block contains %q", a)
				}
			}
		})
	}
}

func TestFamilyBlockEmptyAndCut(t *testing.T) {
	alone := &FamilyTree{PageID: "maud", People: []FamilyPerson{{ID: "maud", Name: "Maud", Read: true}}}
	if out := strings.TrimSpace(renderFamily(t, alone, false)); strings.Contains(out, "fm-") || strings.Contains(out, "Relations") {
		t.Errorf("a player on a page with no family gets nothing, got %q", out)
	}
	// The Director still gets the block on an empty page: Edit is how family
	// is first added (the script draws the hint in the strip).
	dmOut := renderFamily(t, alone, true)
	for _, w := range []string{`data-widget="family-tree"`, "data-fm-edit", "data-fm-savebar", "0 relatives"} {
		if !strings.Contains(dmOut, w) {
			t.Errorf("the Director's empty block lacks %q", w)
		}
	}
	cut := &FamilyTree{PageID: "maud", CutPeople: true, People: []FamilyPerson{{ID: "maud", Name: "Maud"}, {ID: "k", Name: "Kid", Parents: []FamilyParent{{ID: "maud", Kind: FamilyKindBirth}}}}}
	out := renderFamily(t, cut, false)
	for _, w := range []string{"At least 1 relatives", "Showing the nearest 60 relatives; the rest are on their own pages."} {
		if !strings.Contains(out, w) {
			t.Errorf("a cut family must say so: lacks %q", w)
		}
	}
}

// A name that looks like markup stays text: the script uses textContent, and
// the JSON script is escaped by templ.
func TestFamilyBlockEscapesNames(t *testing.T) {
	tree := &FamilyTree{PageID: "maud", People: []FamilyPerson{
		{ID: "maud", Name: "Maud", Read: true},
		{ID: "x", Name: `<img src=x onerror=alert(1)>`, Parents: []FamilyParent{{ID: "maud", Kind: FamilyKindBirth}}},
	}}
	out := renderFamily(t, tree, false)
	if strings.Contains(out, "<img src=x") {
		t.Errorf("a name reached the markup unescaped: %s", out)
	}
}

// Every handler the shell names must exist in the script, and none may be a
// templ script helper or a delegated listener.
func TestFamilyBlockHandlersExistAndAreInline(t *testing.T) {
	tree, _ := secretsFixture().service().Tree(context.Background(), "c1", "maud", int(permissions.RoleOwner), "u")
	out := renderFamily(t, tree, true)
	js, err := StaticAssetsFS.ReadFile("static/js/family_tree.js")
	if err != nil {
		t.Fatal(err)
	}
	const marker = "window.ChronicleFamilyTree."
	names := map[string]bool{}
	for _, m := range regexp.MustCompile(regexp.QuoteMeta(marker)+`([A-Za-z]+)\(`).FindAllStringSubmatch(out, -1) {
		names[m[1]] = true
	}
	// Three for everyone, and edit, undo and save for the Director's shell.
	if len(names) != 6 {
		t.Fatalf("the Director's shell should name toggle, close, siblings, edit, undo and save, got %v", names)
	}
	if player := renderFamily(t, tree, false); strings.Count(player, marker) != 3 {
		t.Errorf("a player's shell names only toggle, close and siblings, got %d handlers", strings.Count(player, marker))
	}
	for n := range names {
		if !regexp.MustCompile(`\b` + n + `: function`).Match(js) {
			t.Errorf("family_tree.js has no handler %q", n)
		}
	}
	if strings.Contains(out, "<script>function") || strings.Contains(string(js), "document.addEventListener('click'") {
		t.Error("handlers must be inline IIFEs, never templ scripts or delegated document listeners")
	}
	if !strings.Contains(out, `data-widget="family-tree"`) {
		t.Error("the block must mount as a widget so its script loads on sight")
	}
}

// familyBrowserFixtures is where the browser spec reads the block's markup.
// The Browser tests job has no Go toolchain, so the markup is committed there
// and this test keeps it identical to what the real component renders.
const familyBrowserFixtures = "../../../test/e2e/fixtures/family_tree"

var assetVersion = regexp.MustCompile(`\?v=[0-9a-f]+`)

// TestFamilyBlocksMatchBrowserFixtures fails when the committed markup the
// browser spec drives (test/e2e/family_tree_edit.spec.mjs) no longer matches
// the real block. UPDATE_FAMILY_FIXTURES=1 rewrites it.
func TestFamilyBlocksMatchBrowserFixtures(t *testing.T) {
	update := os.Getenv("UPDATE_FAMILY_FIXTURES") == "1"
	for name, role := range map[string]int{"director": int(permissions.RoleOwner), "player": int(permissions.RolePlayer)} {
		tree, err := houseFixture().service().Tree(context.Background(), "c1", "maud", role, "u1")
		if err != nil {
			t.Fatal(err)
		}
		// The asset version changes with every build, so it is pinned here.
		got := assetVersion.ReplaceAllString(renderFamily(t, tree, role == int(permissions.RoleOwner)), "?v=fixture") + "\n"
		path := filepath.Join(familyBrowserFixtures, name+".html")
		if update {
			if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v (run with UPDATE_FAMILY_FIXTURES=1)", path, err)
		}
		if string(want) != got {
			t.Errorf("%s is out of date with the Family tree block; run: UPDATE_FAMILY_FIXTURES=1 go test ./internal/plugins/entities/ -run TestFamilyBlocksMatchBrowserFixtures", path)
		}
	}
}
