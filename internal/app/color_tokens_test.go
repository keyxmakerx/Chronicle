package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A var(--color-x) whose token nobody defines always falls back, so it ignores
// the theme: a dark fallback stays dark in light mode and a light one stays
// light in dark mode. The Tailwind names (surface, edge, fg) are classes, not
// tokens; CSS and JS use the --color-* tokens those classes read.
var (
	colorTokenDef = regexp.MustCompile(`(--color-[\w-]+)\s*:|setProperty\(\s*['"](--color-[\w-]+)|['"` + "`" + `](--color-[\w-]+)['"` + "`" + `]`)
	colorTokenUse = regexp.MustCompile(`var\((--color-[\w-]+)`)
)

// knownUndefinedColorTokens are used with a fallback that suits both themes
// (a status colour, or a rule only drawn on the always-dark sidebar), or are
// a placeholder in a comment.
var knownUndefinedColorTokens = map[string]bool{
	"--color-success":          true,
	"--color-warning":          true,
	"--color-sidebar-active":   true,
	"--color-accent-surface-N": true,
}

func TestColorTokensAreDefined(t *testing.T) {
	defined := map[string]bool{}
	used := map[string][]string{}
	for _, root := range []string{filepath.Join("..", "..", "static"), filepath.Join("..", "..", "internal")} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			name := d.Name()
			if name == "app.css" || strings.HasSuffix(name, "_templ.go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			switch filepath.Ext(name) {
			case ".css", ".js", ".mjs", ".go", ".templ":
			default:
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, m := range colorTokenDef.FindAllStringSubmatch(string(b), -1) {
				for _, g := range m[1:] {
					if g != "" {
						defined[g] = true
					}
				}
			}
			for _, m := range colorTokenUse.FindAllStringSubmatch(string(b), -1) {
				used[m[1]] = append(used[m[1]], p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	var missing []string
	for tok, files := range used {
		if !defined[tok] && !knownUndefinedColorTokens[tok] {
			missing = append(missing, tok+" (in "+files[0]+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("colour tokens used but never defined; use the theme's --color-* tokens:\n  %s", strings.Join(missing, "\n  "))
	}
}
