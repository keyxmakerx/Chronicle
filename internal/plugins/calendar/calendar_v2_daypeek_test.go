// calendar_v2_daypeek_test.go — C-CAL-WORLDSTATE-PRODUCTION-PORT 2b-2.
// The day popover gains a read-only worldState peek (moon/weather/celestial)
// for the clicked day, fetched from the existing #401 GET seed endpoint.
package calendar

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDayPopover_HasWorldStatePeekContainer: the popover carries the peek
// container the shell JS fills.
func TestDayPopover_HasWorldStatePeekContainer(t *testing.T) {
	var sb strings.Builder
	if err := dayDetailPopoverV2().Render(context.Background(), &sb); err != nil {
		t.Fatalf("render popover: %v", err)
	}
	if !strings.Contains(sb.String(), "data-day-popover-worldstate") {
		t.Errorf("day popover missing the worldState peek container")
	}
}

// TestV2Root_CarriesDateContext: the root exposes year+month so the popover
// JS can build the per-day world-state URL.
func TestV2Root_CarriesDateContext(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/calendar/calendar_v2.templ")
	for _, want := range []string{"data-cal-v2-year=", "data-cal-v2-month="} {
		if !strings.Contains(src, want) {
			t.Errorf("calendar_v2 root missing %q (popover needs the date context)", want)
		}
	}
}

// TestShellJS_HasWorldStatePeekWiring: the popover fetches the #401 seed for
// the clicked day and renders moon/weather/celestial — no new endpoint.
func TestShellJS_HasWorldStatePeekWiring(t *testing.T) {
	js := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_v2_shell.js")
	for _, marker := range []string{
		"renderWorldStatePeek",            // the peek entry point
		"/calendar/world-state?year=",     // reuses the existing #401 GET seed
		"data-day-popover-worldstate",     // target container
		"m.namedPhase",                    // renders the moon phase label
		"ws.weather.type",                 // renders weather
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("shell JS missing worldState-peek wiring: %q", marker)
		}
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}
