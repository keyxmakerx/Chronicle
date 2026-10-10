package layouts

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// hxOnAttr matches an hx-on / hx-on:* / hx-on::* attribute (or its data-
// prefixed form) being assigned, not prose that merely names the feature.
var hxOnAttr = regexp.MustCompile(`hx-on(:{1,2}[\w.:-]*)?\s*=`)

// TestNoHxOnAttributes: boot.js sets htmx.config.allowEval=false and htmx runs
// every hx-on handler through eval, so such an attribute is silently dead.
// Use an Alpine x-on:htmx:... listener or an inline onclick IIFE instead.
func TestNoHxOnAttributes(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	for _, dir := range []string{"internal", "static"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(path) {
			case ".templ", ".js", ".html":
			default:
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(b), "\n") {
				if hxOnAttr.MatchString(line) {
					t.Errorf("%s:%d uses an hx-on attribute, which is dead while htmx eval is off", path, i+1)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
