package layouts

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A heading may carry an icon, in the heading's own colour: the accent is
// for links and for a tool's selected state, so an icon in it reads as
// something to press. This walks every template's headings.
func TestHeadings_IconsAreNotInTheAccent(t *testing.T) {
	heading := regexp.MustCompile(`(?s)<h([1-6])\b[^>]*>.*?</h[1-6]>`)
	icon := regexp.MustCompile(`<i class="([^"]*)"`)
	seen := 0
	err := filepath.WalkDir(filepath.Join("..", ".."), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".templ") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, h := range heading.FindAllString(string(src), -1) {
			for _, m := range icon.FindAllStringSubmatch(h, -1) {
				seen++
				for _, class := range strings.Fields(m[1]) {
					if class == "text-accent" {
						t.Errorf("%s: a heading's icon is in the accent: %s", path, m[0])
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the templates: %v", err)
	}
	if seen < 20 {
		t.Fatalf("found only %d heading icons; the walk is not reading the templates", seen)
	}
}
