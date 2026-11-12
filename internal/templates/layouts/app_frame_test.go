package layouts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// appPagesOnTheSharedOpening lists the templates whose app page has moved onto
// AppFrame and AppOpening. Each app's pull request adds its own.
var appPagesOnTheSharedOpening = []string{
	"internal/plugins/maps/maps.templ",
	"internal/plugins/timeline/timeline.templ",
}

func TestAppPages_UseTheSharedFrameAndOpening(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	for _, f := range appPagesOnTheSharedOpening {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, want := range []string{"layouts.AppFrame()", "layouts.AppOpening("} {
			if !strings.Contains(src, want) {
				t.Errorf("%s no longer uses %s", f, want)
			}
		}
	}
}
