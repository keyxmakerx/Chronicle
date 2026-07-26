// calendar_v2_mobile_breakpoint_probe_test.go — the 390px render pin
// (C-CAL-MOBILE-VIEWS-FIX).
//
// WHY a browser and not another DOM assertion. Every other test in this package
// asserts on templ OUTPUT: "the desktop grid carries `hidden md:block`". That
// class of test cannot see the failure mode this dispatch chased, because the
// swap only happens if the STYLESHEET also defines the utilities the markup
// names. Step 0 demonstrated the gap concretely: `md:contents` did not exist in
// Tailwind's output before C-CAL-MOBILE-AGENDA, and rendering post-deploy HTML
// against a pre-deploy app.css silently drops the Week/Day/Timeline pills from
// the DESKTOP calendar — with every DOM assertion in this package still green.
//
// So this probe closes the loop end to end: build the real Tailwind bundle from
// the real config, render the real templ output, and ask a real browser what is
// actually visible at 390px and at 1200px.
//
// SKIPS, honestly, when it cannot do that: under `-short` (CI's mode — see
// .github/workflows/ci.yml "go test ./... -v -short"), or when either the
// Tailwind CLI or a Chromium binary is missing. A skipped run is NOT a pass;
// the skip message names exactly what was missing. To run it here:
//
//	npm install tailwindcss@3.4.17 && go test ./internal/plugins/calendar/ -run Probe
//
// Viewport technique: Chromium clamps `--window-size` to a 500px minimum on
// Linux, so a top-level window cannot reach 390px. The page under test is
// therefore loaded in a 390px-wide IFRAME inside a wide window — media queries
// inside an iframe evaluate against the iframe's own viewport, giving an exact
// 390px without a CDP driver or any npm test dependency.
package calendar

import (
	"context"
	"encoding/json"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// breakpointProbeResult is the JSON the in-page script hands back.
type breakpointProbeResult struct {
	InnerWidth   int      `json:"innerWidth"`
	DesktopGrid  bool     `json:"desktopGrid"` // 7-column month grid visible?
	MobileMonth  bool     `json:"mobileMonth"` // mini-month + agenda assembly visible?
	MiniMonth    bool     `json:"miniMonth"`   // mini-month navigator visible?
	Agenda       bool     `json:"agenda"`      // agenda list visible?
	AgendaCards  int      `json:"agendaCards"` // rendered agenda event cards
	PillsVisible []string `json:"pillsVisible"`
	PillSelected string   `json:"pillSelected"`
}

func TestProbe_MobileBreakpointSwapInRealBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short")
	}
	root := probeRepoRoot(t)
	tailwind := findTailwindCLI(root)
	if tailwind == "" {
		t.Skip("browser probe: no Tailwind CLI found (npm install tailwindcss@3.4.17, or set TAILWIND_BIN)")
	}
	chrome := findChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	css := buildTailwindCSS(t, root, tailwind)

	for _, tc := range []struct {
		name   string
		width  int
		agenda bool
		check  func(t *testing.T, r breakpointProbeResult)
	}{
		{
			// The operator-reported symptom: a phone showing the full 7-column
			// desktop grid. If this ever regresses, this is the assertion that
			// fires.
			name:  "phone/month shows the mini-month + agenda assembly, never the desktop grid",
			width: 390,
			check: func(t *testing.T, r breakpointProbeResult) {
				if r.DesktopGrid {
					t.Error("the 7-column desktop month grid must not be visible at 390px")
				}
				if !r.MobileMonth || !r.MiniMonth || !r.Agenda {
					t.Errorf("phone month view must render the mini-month navigator + agenda list; got assembly=%v mini=%v agenda=%v",
						r.MobileMonth, r.MiniMonth, r.Agenda)
				}
				if r.AgendaCards == 0 {
					t.Error("the agenda list must render event cards for a month that has events")
				}
				if got := r.PillsVisible; len(got) != 2 || got[0] != "Month" || got[1] != "Agenda" {
					t.Errorf("phone command bar must reduce to exactly Month|Agenda; got %v", got)
				}
				if r.PillSelected != "Month" {
					t.Errorf("Month must be the selected phone pill in the default state; got %q", r.PillSelected)
				}
			},
		},
		{
			// The dead-control fix: tapping Agenda must produce a visibly
			// different page, not a re-navigation to the same render.
			name:   "phone/agenda drops the navigator and selects the Agenda pill",
			width:  390,
			agenda: true,
			check: func(t *testing.T, r breakpointProbeResult) {
				if r.DesktopGrid {
					t.Error("the desktop grid must stay hidden in the agenda state")
				}
				if r.MiniMonth {
					t.Error("the agenda state must drop the mini-month navigator — otherwise the Agenda pill renders the same page as Month and is dead again")
				}
				if !r.Agenda || r.AgendaCards == 0 {
					t.Errorf("the agenda state must still render the event list; agenda=%v cards=%d", r.Agenda, r.AgendaCards)
				}
				if r.PillSelected != "Agenda" {
					t.Errorf("Agenda must be the selected phone pill in the agenda state; got %q", r.PillSelected)
				}
			},
		},
		{
			// Desktop must be untouched by all of the above — including the
			// Week/Day/Timeline pills, whose disappearance is exactly what a
			// stale stylesheet caused.
			name:  "desktop keeps the 7-column grid and the full pill row",
			width: 1200,
			check: func(t *testing.T, r breakpointProbeResult) {
				if !r.DesktopGrid {
					t.Error("the 7-column month grid must be visible at 1200px")
				}
				if r.MobileMonth {
					t.Error("the phone assembly must not be visible at 1200px")
				}
				want := []string{"Month", "Week", "Day", "Timeline"}
				if strings.Join(r.PillsVisible, ",") != strings.Join(want, ",") {
					t.Errorf("desktop command bar must show %v; got %v", want, r.PillsVisible)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runBreakpointProbe(t, chrome, css, probeMarkup(t, tc.agenda), tc.width)
			if got.InnerWidth != tc.width {
				t.Fatalf("probe viewport = %dpx; want %dpx (the iframe sizing failed, so the rest of this case proves nothing)", got.InnerWidth, tc.width)
			}
			tc.check(t, got)
		})
	}
}

// probeMarkup renders the command bar + view slot exactly as CalendarV2Page
// composes them, for the requested phone toggle state.
func probeMarkup(t *testing.T, agenda bool) string {
	t.Helper()
	data := designPass1Data("month", []Event{
		timedEvent("e1", "Session 12", 13, 19, 0),
		allDayEvent("e2", "Harvest Feast", 16),
		timedEvent("e3", "Ambush", 21, 9, 30),
	})
	data.MobileAgenda = agenda

	var sb strings.Builder
	if err := calendarV2Header(nil, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render command bar: %v", err)
	}
	if err := calendarV2View(nil, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render view slot: %v", err)
	}
	return sb.String()
}

// probeScript measures what is actually VISIBLE (a zero-area box counts as
// hidden, which catches `display:contents` wrappers and collapsed parents that
// a computed-style check alone would miss).
const probeScript = `
(function(){
  var vis = function(el){
    if (!el) return false;
    var r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  };
  var pills = [].slice.call(document.querySelectorAll('[role="tab"]')).filter(vis);
  var selected = pills.filter(function(p){ return p.getAttribute('aria-selected') === 'true'; });
  return {
    innerWidth: window.innerWidth,
    desktopGrid: vis(document.querySelector('[data-month-desktop-grid]')),
    mobileMonth: vis(document.querySelector('[data-mobile-month-assembly]')),
    miniMonth:   vis(document.querySelector('[data-mobile-mini-month]')),
    agenda:      vis(document.querySelector('[data-mobile-agenda]')),
    agendaCards: [].slice.call(document.querySelectorAll('[data-event-card="agenda"]')).filter(vis).length,
    pillsVisible: pills.map(function(p){ return p.textContent.trim(); }),
    pillSelected: selected.length === 1 ? selected[0].textContent.trim() : ''
  };
})()`

var probeAttrRe = regexp.MustCompile(`data-probe="([^"]*)"`)

// runBreakpointProbe writes the inner page + a wrapper that hosts it in a
// fixed-width iframe, runs Chromium once, and parses the probe payload the
// wrapper copies onto its own <body>.
func runBreakpointProbe(t *testing.T, chrome, css, markup string, width int) breakpointProbeResult {
	t.Helper()
	dir := t.TempDir()

	inner := `<!doctype html><html class="h-full"><head><meta charset="utf-8">` +
		`<style>` + css + `</style></head>` +
		`<body class="h-full font-sans antialiased" style="opacity:1">` +
		// Mirrors CalendarV2Page's root: a flex row whose negative margins are
		// cancelled by the app shell's padding.
		`<div class="px-5 py-4"><div class="h-full flex -mx-5 -my-4">` +
		`<div class="flex-1 flex flex-col min-w-0">` + markup + `</div></div></div>` +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`document.body.setAttribute('data-probe', JSON.stringify(` + probeScript + `));});</script>` +
		`</body></html>`
	writeProbeFile(t, filepath.Join(dir, "inner.html"), inner)

	outer := `<!doctype html><html><body style="margin:0">` +
		`<iframe id="f" src="inner.html" style="width:` + itoaProbe(width) + `px;height:900px;border:0"></iframe>` +
		`<script>addEventListener('load',function(){` +
		`var d=document.getElementById('f').contentDocument;` +
		`document.body.setAttribute('data-probe', d.body.getAttribute('data-probe')||'');});</script>` +
		`</body></html>`
	outerPath := filepath.Join(dir, "outer.html")
	writeProbeFile(t, outerPath, outer)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		// The wrapper reads the iframe's document across a file:// boundary.
		"--allow-file-access-from-files",
		"--window-size=1400,900",
		"--virtual-time-budget=5000",
		"--dump-dom", "file://"+outerPath,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}

	// The outer <body> carries the payload; the inner iframe's own copy also
	// appears in the dump, so match on the LAST occurrence-independent way:
	// both are identical JSON, so the first non-empty match is fine.
	for _, m := range probeAttrRe.FindAllStringSubmatch(string(out), -1) {
		raw := html.UnescapeString(m[1])
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var r breakpointProbeResult
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatalf("probe payload %q: %v", raw, err)
		}
		return r
	}
	t.Fatalf("no probe payload in the rendered DOM — the page script did not run:\n%s", truncateProbe(string(out)))
	return breakpointProbeResult{}
}

// buildTailwindCSS runs the real Tailwind build (real config, real content
// globs) so the probe measures the stylesheet a deploy would actually ship.
func buildTailwindCSS(t *testing.T, root, tailwind string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "app.css")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tailwind, "-i", "static/css/input.css", "-o", out, "--minify")
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tailwind build failed: %v\n%s", err, combined)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read built css: %v", err)
	}
	if !strings.Contains(string(body), `.md\:contents`) {
		t.Fatal("the built stylesheet has no .md\\:contents rule — the phone/desktop pill reduction cannot work without it")
	}
	return string(body)
}

func findTailwindCLI(root string) string {
	if p := os.Getenv("TAILWIND_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	local := filepath.Join(root, "node_modules", ".bin", "tailwindcss")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	if p, err := exec.LookPath("tailwindcss"); err == nil {
		return p
	}
	return ""
}

func findChromium() string {
	if p := os.Getenv("CHROMIUM_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	// Playwright's browser cache (the layout this dev container ships).
	for _, pattern := range []string{
		"/opt/pw-browsers/chromium-*/chrome-linux/chrome",
		filepath.Join(os.Getenv("HOME"), ".cache/ms-playwright/chromium-*/chrome-linux/chrome"),
	} {
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return matches[len(matches)-1]
		}
	}
	return ""
}

func probeRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

func writeProbeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func itoaProbe(n int) string {
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

func truncateProbe(s string) string {
	if len(s) > 2000 {
		return s[:2000] + "…"
	}
	return s
}
