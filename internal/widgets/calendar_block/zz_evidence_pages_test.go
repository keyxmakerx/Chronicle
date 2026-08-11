// zz_evidence_pages_test.go — SCRATCH, decision-evidence lane 2.
//
// NOT A TEST. A generator, inert unless EVIDENCE_PAGES names a directory. It
// writes the two evidence pages the capture driver photographs, and it builds
// them exactly the way moon_reach_probe_test.go builds its census page: the
// REAL templ output of Block(), the REAL shipped calendar-block.css inlined
// (the <link> cannot resolve over file://), and the host width the app shell's
// own arithmetic gives that viewport — mrHostWidth(viewport, sidebar).
//
//	EVIDENCE_PAGES=/tmp/x go test ./internal/widgets/calendar_block/ -run EvidencePages
package calendar_block

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var evLinkRe = regexp.MustCompile(`<link[^>]*>`)

func TestEvidencePages(t *testing.T) {
	out := os.Getenv("EVIDENCE_PAGES")
	if out == "" {
		t.Skip("set EVIDENCE_PAGES=<dir> to write the decision-evidence pages")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	css := blockCSS(t)

	write := func(name string, viewport, sidebar, pad int, d BlockData) {
		t.Helper()
		host := mrHostWidth(viewport, sidebar)
		markup := evLinkRe.ReplaceAllString(render(t, d), "")
		page := `<!doctype html><html><head><meta charset="utf-8">` +
			`<meta name="viewport" content="width=device-width,initial-scale=1">` +
			`<style>html,body{margin:0;background:#fff;` +
			`font-family:system-ui,-apple-system,"Segoe UI",sans-serif}` +
			fmt.Sprintf(`.probe-host{display:block;max-width:none;width:%dpx;margin:0 %dpx}`,
				host, pad) +
			css + `</style></head><body>` +
			`<div class="probe-host">` + markup + `</div>` +
			`</body></html>`
		p := filepath.Join(out, name)
		if err := os.WriteFile(p, []byte(page), 0o644); err != nil { //nolint:gosec // scratch artefact
			t.Fatalf("write %s: %v", name, err)
		}
		t.Logf("%s — viewport %dpx, sidebar %dpx, host %dpx", name, viewport, sidebar, host)
	}

	// THE SAME FIXTURE ON BOTH PAGES, so the only difference between the two
	// placement shots is the density the container query resolves — which is
	// the whole question. fxMoonSevenDay is the Gregorian calendar with the
	// moon data actually on it (moon_reach_probe_test.go).
	write("phone-underline.html", 390, mrSidebarPx, mrPadNarrow, fxMoonSevenDay(t))
	write("desktop-named.html", 1280, mrSidebarPx, mrPadWide, fxMoonSevenDay(t))
}
