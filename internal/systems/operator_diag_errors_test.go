package systems

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// errNow / errStart are fixed reference points so every age assertion below is
// deterministic.
var (
	errNow   = time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	errStart = errNow.Add(-6 * time.Hour)
)

// withRecentErrors installs a provider for one test and restores whatever was
// there. The provider is package-level state, so a test that forgot to restore
// it would silently change every later test's answer.
func withRecentErrors(t *testing.T, fn func(limit int) RecentErrors) {
	t.Helper()
	prev := recentErrorsFn
	recentErrorsFn = fn
	t.Cleanup(func() { recentErrorsFn = prev })
}

// sampleErrors is a small, realistic ring: one route failing repeatedly, one
// panic, and one unrelated one-off — the exact shape the summary has to be able
// to collapse without hiding the one-off.
func sampleErrors() []RecentError {
	return []RecentError{
		{Time: errNow.Add(-2 * time.Minute), Status: 500, Method: "GET", Path: "/campaigns/:id/calendar", PathIsTemplate: true, Kind: "raw", Err: "sql: no rows in result set"},
		{Time: errNow.Add(-9 * time.Minute), Status: 500, Method: "GET", Path: "/campaigns/:id/calendar", PathIsTemplate: true, Kind: "raw", Err: "sql: connection refused"},
		{Time: errNow.Add(-31 * time.Minute), Status: 500, Method: "POST", Path: "/campaigns/:id/entities", PathIsTemplate: true, Kind: "panic", Err: "panic: runtime error: index out of range [3]"},
		{Time: errNow.Add(-90 * time.Minute), Status: 500, Method: "GET", Path: "/campaigns/:id/calendar", PathIsTemplate: true, Kind: "raw", Err: "sql: connection refused"},
		{Time: errNow.Add(-4 * time.Hour), Status: 503, Method: "GET", Path: "/healthz", PathIsTemplate: true, Kind: "app", Err: "redis unavailable"},
	}
}

func sampleSnapshot() RecentErrors {
	e := sampleErrors()
	return RecentErrors{Capacity: 256, Held: len(e), Total: uint64(len(e)), Entries: e}
}

// TestErrorDiagnosticsRegistered — a renderer nobody can reach is not a
// diagnostic.
func TestErrorDiagnosticsRegistered(t *testing.T) {
	cat := diagnosticCatalog()
	for _, name := range []string{"host.errors", "host.errors-summary"} {
		var found *Diagnostic
		for i := range cat {
			if cat[i].Name == name {
				found = &cat[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("%s is not registered in diagnosticCatalog()", name)
		}
		if found.Run == nil {
			t.Errorf("%s has a nil Run", name)
		}
		if found.Title == "" || found.Desc == "" {
			t.Errorf("%s: Title=%q Desc=%q — the catalog listing is how the assistant chooses", name, found.Title, found.Desc)
		}
		if found.FullDump {
			t.Errorf("%s is bounded (max %d rows) and is the first thing to run in an incident; gating it behind full_dump would hide it", name, maxErrorRows)
		}
	}
}

// TestHostErrorsUnwired is the single most important render in this file. An
// unwired provider must NOT look like a healthy server — that is the same
// "absence of evidence read as evidence" mistake the whole host.* group exists
// to prevent.
func TestHostErrorsUnwired(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  string
	}{
		{"host.errors", renderHostErrorsFrom(RecentErrors{}, false, defaultErrorRows, "", errStart, errNow)},
		{"host.errors-summary", renderHostErrorsSummaryFrom(RecentErrors{}, false, errStart, errNow)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.got, "Provider not wired") {
				t.Errorf("unwired render does not say so:\n%s", tc.got)
			}
			if !strings.Contains(tc.got, "not \"no errors have occurred\"") {
				t.Errorf("unwired render must explicitly deny that it means zero errors:\n%s", tc.got)
			}
			if !strings.Contains(tc.got, "docker logs") {
				t.Errorf("unwired render should point at the fallback that still works:\n%s", tc.got)
			}
			if strings.Contains(tc.got, "No error has been recorded") {
				t.Errorf("unwired render claimed no errors occurred — it cannot know that:\n%s", tc.got)
			}
		})
	}
}

// TestHostErrorsWiredButEmpty is the other half of that distinction: a wired,
// empty ring is real good news and must read as such, while still printing the
// capacity so the reader can see the ring exists.
func TestHostErrorsWiredButEmpty(t *testing.T) {
	snap := RecentErrors{Capacity: 256, Held: 0, Total: 0}
	got := renderHostErrorsFrom(snap, true, defaultErrorRows, "", errStart, errNow)

	if strings.Contains(got, "Provider not wired") {
		t.Errorf("a wired empty ring rendered as unwired:\n%s", got)
	}
	if !strings.Contains(got, "256 slots") || !strings.Contains(got, "0 held") {
		t.Errorf("header must state capacity AND hold count so 'empty' is distinguishable from 'unwired':\n%s", got)
	}
	if !strings.Contains(got, "No error has been recorded") {
		t.Errorf("empty ring should say so plainly:\n%s", got)
	}
	if !strings.Contains(got, "EMPTY result, not an unwired one") {
		t.Errorf("empty render must name the distinction it is on the right side of:\n%s", got)
	}
	if !strings.Contains(got, "6h0m0s") {
		t.Errorf("empty render must scope its claim to the uptime it can actually speak for:\n%s", got)
	}
}

// TestHostErrorsHeaderAndLines covers the populated render: header counts, one
// line per entry, newest first, with an AGE next to each timestamp.
func TestHostErrorsHeaderAndLines(t *testing.T) {
	got := renderHostErrorsFrom(sampleSnapshot(), true, defaultErrorRows, "", errStart, errNow)

	if !strings.Contains(got, "**Ring: 256 slots · 5 held · 5 recorded since process start**") {
		t.Errorf("header line missing or reshaped:\n%s", got)
	}
	if !strings.Contains(got, "showing 5 of 5 held") {
		t.Errorf("render should say how much of the ring it is showing:\n%s", got)
	}
	// The age is what makes this readable during an incident.
	if !strings.Contains(got, "2m0s ago") {
		t.Errorf("newest entry should carry a relative age:\n%s", got)
	}
	if !strings.Contains(got, "4h0m0s ago") {
		t.Errorf("oldest entry should carry a relative age:\n%s", got)
	}
	for _, want := range []string{
		"`/campaigns/:id/calendar`",
		"sql: no rows in result set",
		"**503**",
		"redis unavailable",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render is missing %q:\n%s", want, got)
		}
	}
	// Newest first: the -2m entry must appear before the -4h one.
	newest := strings.Index(got, "sql: no rows in result set")
	oldest := strings.Index(got, "redis unavailable")
	if newest < 0 || oldest < 0 || newest > oldest {
		t.Errorf("entries are not newest-first (newest at %d, oldest at %d):\n%s", newest, oldest, got)
	}
	// The kind labels are the "is this a bug or a handled condition?" signal.
	if !strings.Contains(got, "most likely an actual bug") {
		t.Errorf("a `raw` error should be labelled as the probable-bug class:\n%s", got)
	}
	if !strings.Contains(got, "recovered panic") {
		t.Errorf("a panic entry should be labelled as one:\n%s", got)
	}
}

// TestHostErrorsPolicyNoteAlwaysPresent. The note is unconditional on purpose:
// its reader is mid-incident and will not go looking for documentation, and the
// two things it explains (4xx are absent BY DESIGN, the ring dies at restart)
// are both things that get mistaken for findings.
func TestHostErrorsPolicyNoteAlwaysPresent(t *testing.T) {
	renders := map[string]string{
		"populated": renderHostErrorsFrom(sampleSnapshot(), true, defaultErrorRows, "", errStart, errNow),
		"empty":     renderHostErrorsFrom(RecentErrors{Capacity: 256}, true, defaultErrorRows, "", errStart, errNow),
		"summary":   renderHostErrorsSummaryFrom(sampleSnapshot(), true, errStart, errNow),
	}
	for name, got := range renders {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{
				"NOT recorded: ordinary 4xx",
				"evicts oldest-first",
				"ROUTE TEMPLATES",
				"A restart empties the ring",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("policy note is missing %q:\n%s", want, got)
				}
			}
		})
	}
}

// TestHostErrorsSurfacesEviction: when the ring has wrapped, the window shown
// is incomplete and the render must SAY so. A full-looking list whose oldest
// line silently is not the first occurrence is the entity-page-walk defect
// again — a bound is fine, a bound nobody is told about is not.
func TestHostErrorsSurfacesEviction(t *testing.T) {
	snap := sampleSnapshot()
	snap.Capacity = 5
	snap.Held = 5
	snap.Total = 412

	got := renderHostErrorsFrom(snap, true, defaultErrorRows, "", errStart, errNow)
	if !strings.Contains(got, "The ring has wrapped") {
		t.Errorf("wrapped ring must be announced:\n%s", got)
	}
	if !strings.Contains(got, "407 earlier error(s) have been evicted") {
		t.Errorf("wrapped render must quantify what was lost (412 total - 5 held = 407):\n%s", got)
	}
	if !strings.Contains(got, "do NOT read the oldest line here as the first occurrence") {
		t.Errorf("wrapped render must warn against the specific wrong reading:\n%s", got)
	}

	// And the un-wrapped case must NOT cry wolf.
	if plain := renderHostErrorsFrom(sampleSnapshot(), true, defaultErrorRows, "", errStart, errNow); strings.Contains(plain, "The ring has wrapped") {
		t.Errorf("an un-wrapped ring must not claim eviction:\n%s", plain)
	}
}

// TestParseErrorLimit pins the argument handling, including that a bad or
// clamped argument is REPORTED. A silently ignored argument is how an operator
// ends up believing they looked at 100 errors when they looked at 25.
func TestParseErrorLimit(t *testing.T) {
	tests := []struct {
		arg      string
		want     int
		wantNote string
	}{
		{"", defaultErrorRows, ""},
		{"  ", defaultErrorRows, ""},
		{"10", 10, ""},
		{" 10 ", 10, ""},
		{"100", maxErrorRows, ""},
		{"1000", maxErrorRows, "clamped"},
		{"0", defaultErrorRows, "not a positive count"},
		{"-5", defaultErrorRows, "not a positive count"},
		{"lots", defaultErrorRows, "is not a number"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("arg=%q", tt.arg), func(t *testing.T) {
			got, note := parseErrorLimit(tt.arg)
			if got != tt.want {
				t.Errorf("parseErrorLimit(%q) = %d, want %d", tt.arg, got, tt.want)
			}
			if tt.wantNote == "" {
				if note != "" {
					t.Errorf("unexpected note %q", note)
				}
			} else if !strings.Contains(note, tt.wantNote) {
				t.Errorf("note = %q, want it to mention %q", note, tt.wantNote)
			}
		})
	}
}

// TestHostErrorsPassesLimitToProvider pins that the argument reaches the ring
// rather than being applied after the fact — a provider handed no limit would
// copy the whole ring on every run.
func TestHostErrorsPassesLimitToProvider(t *testing.T) {
	gotLimit := -999
	withRecentErrors(t, func(limit int) RecentErrors {
		gotLimit = limit
		return RecentErrors{Capacity: 256}
	})
	renderHostErrors("7")
	if gotLimit != 7 {
		t.Errorf("provider was asked for %d entries, want 7", gotLimit)
	}

	renderHostErrors("")
	if gotLimit != defaultErrorRows {
		t.Errorf("provider was asked for %d entries with no argument, want the default %d", gotLimit, defaultErrorRows)
	}
}

// TestHostErrorsSummaryAsksForWholeRing: a frequency computed over one page is
// a lie about how often something is failing.
func TestHostErrorsSummaryAsksForWholeRing(t *testing.T) {
	gotLimit := -999
	withRecentErrors(t, func(limit int) RecentErrors {
		gotLimit = limit
		return RecentErrors{Capacity: 256}
	})
	renderHostErrorsSummary()
	if gotLimit > 0 {
		t.Errorf("summary asked the provider for %d entries; it must ask for the whole ring (limit <= 0)", gotLimit)
	}
}

// TestGroupErrors is the summary's core: one line per failure mode, ordered by
// frequency, with first/last bounds that span the whole group.
func TestGroupErrors(t *testing.T) {
	groups := groupErrors(sampleErrors())
	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3 (calendar 500, entities 500, healthz 503): %+v", len(groups), groups)
	}

	// Most frequent first.
	g := groups[0]
	if g.Path != "/campaigns/:id/calendar" || g.Count != 3 {
		t.Fatalf("first group = %s ×%d, want /campaigns/:id/calendar ×3", g.Path, g.Count)
	}
	if !g.First.Equal(errNow.Add(-90 * time.Minute)) {
		t.Errorf("First = %v, want the OLDEST member (-90m), not the first one iterated", g.First)
	}
	if !g.Last.Equal(errNow.Add(-2 * time.Minute)) {
		t.Errorf("Last = %v, want the NEWEST member (-2m)", g.Last)
	}
	if g.SampleErr != "sql: no rows in result set" {
		t.Errorf("SampleErr = %q, want the message from the most RECENT member", g.SampleErr)
	}
	if g.Distinct != 2 {
		t.Errorf("Distinct = %d, want 2 — the count-vs-distinct ratio is what separates a stuck route from scattered failures", g.Distinct)
	}
	if len(g.Methods) != 1 || g.Methods[0] != "GET" {
		t.Errorf("Methods = %v, want [GET]", g.Methods)
	}

	// The one-off must survive grouping, not be buried by the loud one.
	var found bool
	for _, x := range groups {
		if x.Path == "/healthz" && x.Status == 503 && x.Count == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("the single 503 on /healthz was lost in grouping: %+v", groups)
	}
}

// TestGroupErrorsSeparatesStatusOnSamePath: two different statuses on one route
// are two different failure modes and must not be merged.
func TestGroupErrorsSeparatesStatusOnSamePath(t *testing.T) {
	groups := groupErrors([]RecentError{
		{Time: errNow, Status: 500, Method: "GET", Path: "/x", PathIsTemplate: true, Kind: "raw", Err: "a"},
		{Time: errNow, Status: 503, Method: "GET", Path: "/x", PathIsTemplate: true, Kind: "app", Err: "b"},
	})
	if len(groups) != 2 {
		t.Errorf("got %d groups, want 2 — status is part of the key: %+v", len(groups), groups)
	}
}

// TestGroupErrorsMergesMethodsOnOneRoute: a route failing for both GET and POST
// is ONE broken route, but which methods it fails for is itself a finding, so
// they are listed rather than dropped.
func TestGroupErrorsMergesMethodsOnOneRoute(t *testing.T) {
	groups := groupErrors([]RecentError{
		{Time: errNow, Status: 500, Method: "GET", Path: "/x", PathIsTemplate: true, Kind: "raw", Err: "a"},
		{Time: errNow.Add(-time.Minute), Status: 500, Method: "POST", Path: "/x", PathIsTemplate: true, Kind: "raw", Err: "a"},
	})
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1: %+v", len(groups), groups)
	}
	if strings.Join(groups[0].Methods, "/") != "GET/POST" {
		t.Errorf("Methods = %v, want both listed and sorted", groups[0].Methods)
	}
	if groups[0].Distinct != 1 {
		t.Errorf("Distinct = %d, want 1 — the same message twice is one message", groups[0].Distinct)
	}
}

// TestHostErrorsSummaryRender covers the summary's output shape.
func TestHostErrorsSummaryRender(t *testing.T) {
	got := renderHostErrorsSummaryFrom(sampleSnapshot(), true, errStart, errNow)

	if !strings.Contains(got, "3 distinct failure mode(s) across 5 held entries") {
		t.Errorf("summary should state how many modes over how many entries:\n%s", got)
	}
	if !strings.Contains(got, "**3×**") {
		t.Errorf("the repeating failure should read as one line with a count:\n%s", got)
	}
	if !strings.Contains(got, "first seen") || !strings.Contains(got, "last seen") {
		t.Errorf("each group needs first/last seen:\n%s", got)
	}
	if !strings.Contains(got, "2 distinct error messages in this group") {
		t.Errorf("a group with several distinct messages must say so, or the sample reads as the whole story:\n%s", got)
	}
	// The whole point of the summary: 3 calendar failures produce ONE bullet.
	if n := strings.Count(got, "/campaigns/:id/calendar"); n != 1 {
		t.Errorf("the repeating route appears %d times, want exactly 1 — collapsing it is the diagnostic's purpose:\n%s", n, got)
	}
}

// TestRawPathIsFlagged: an entry whose router matched nothing keeps its real
// path, and both renders must label it so nobody reads a literal id as a
// placeholder.
func TestRawPathIsFlagged(t *testing.T) {
	entries := []RecentError{
		{Time: errNow, Status: 500, Method: "GET", Path: "/some/unmatched/thing", PathIsTemplate: false, Kind: "http", Err: "boom"},
	}
	snap := RecentErrors{Capacity: 256, Held: 1, Total: 1, Entries: entries}

	if got := renderHostErrorsFrom(snap, true, defaultErrorRows, "", errStart, errNow); !strings.Contains(got, "_(raw path)_") {
		t.Errorf("host.errors did not flag a non-template path:\n%s", got)
	}
	if got := renderHostErrorsSummaryFrom(snap, true, errStart, errNow); !strings.Contains(got, "raw path — the router matched no route") {
		t.Errorf("host.errors-summary did not flag a non-template path:\n%s", got)
	}
}

// TestErrorRenderSingleLine: a wrapped multi-line error must not break the
// bullet list and make the entries under it look like its children.
func TestErrorRenderSingleLine(t *testing.T) {
	snap := RecentErrors{Capacity: 256, Held: 1, Total: 1, Entries: []RecentError{
		{Time: errNow, Status: 500, Method: "GET", Path: "/x", PathIsTemplate: true, Kind: "raw", Err: "outer:\n\tinner: broke"},
	}}
	got := renderHostErrorsFrom(snap, true, defaultErrorRows, "", errStart, errNow)
	if !strings.Contains(got, "outer:  inner: broke") {
		t.Errorf("multi-line error was not flattened:\n%s", got)
	}
}

// TestErrorArgNoteRendered: the correction from parseErrorLimit must reach the
// output, not just the return value.
func TestErrorArgNoteRendered(t *testing.T) {
	withRecentErrors(t, func(int) RecentErrors { return RecentErrors{Capacity: 256} })
	got := renderHostErrors("banana")
	if !strings.Contains(got, "is not a number") {
		t.Errorf("a bad argument was silently ignored:\n%s", got)
	}
}

// TestErrorDiagnosticsRunThroughCatalog exercises the real entry point with no
// provider installed — the state a binary is in before wiring runs — and
// asserts it degrades to the clear message instead of panicking on a nil
// provider.
func TestErrorDiagnosticsRunThroughCatalog(t *testing.T) {
	withRecentErrors(t, nil)
	for _, name := range []string{"host.errors", "host.errors-summary"} {
		out, ok := RunDiagnostic(diagnosticCatalog(), name, "")
		if !ok {
			t.Fatalf("RunDiagnostic(%q) reported not-found", name)
		}
		if !strings.Contains(out, "Provider not wired") {
			t.Errorf("%s with a nil provider:\n%s", name, out)
		}
	}
}

// TestRawPathCannotInjectMarkdown is the regression guard for the render half
// of the raw-path problem.
//
// A path stored on the fallback branch is attacker-chosen, net/http
// percent-DECODES it into URL.Path, and this output is markdown an operator
// pastes into a chat window or an AI assistant and then acts on. Measured
// before the fix, a request to `/a%0A%0A**INJECTED-HEADING**%0A-%20fake%20bullet`
// rendered as a real heading and a real bullet: the attacker's text escaped the
// code span AND the list, and read as this diagnostic's own output. The sibling
// Err field had been flattened from the start; Path had not.
func TestRawPathCannotInjectMarkdown(t *testing.T) {
	// Exactly what url.URL.Path holds after net/http decodes that request.
	const hostile = "/a\n\n**INJECTED-HEADING**\n- fake bullet"
	entries := []RecentError{
		{Time: errNow, Status: 500, Method: "GET", Path: hostile, PathIsTemplate: false, Kind: "panic", Err: "panic: boom"},
	}
	snap := RecentErrors{Capacity: 256, Held: 1, Total: 1, Entries: entries}

	for _, tc := range []struct {
		name string
		got  string
	}{
		{"host.errors", renderHostErrorsFrom(snap, true, defaultErrorRows, "", errStart, errNow)},
		{"host.errors-summary", renderHostErrorsSummaryFrom(snap, true, errStart, errNow)},
	} {
		// The line the path is on must stay ONE line. Anything else means the
		// value left its bullet.
		if strings.Contains(tc.got, "\n**INJECTED-HEADING**") {
			t.Errorf("%s: injected text reached column 0 as a heading:\n%s", tc.name, tc.got)
		}
		if strings.Contains(tc.got, "\n- fake bullet") {
			t.Errorf("%s: injected text reached column 0 as a list item:\n%s", tc.name, tc.got)
		}
		// Flattened, not dropped: an operator still needs to see what was
		// requested, and silently discarding it would trade one dishonesty for
		// another.
		if !strings.Contains(tc.got, "INJECTED-HEADING") {
			t.Errorf("%s: the path was dropped rather than flattened:\n%s", tc.name, tc.got)
		}
	}
}

// TestRawPathCannotEscapeCodeSpan covers the other half of the same breakout: a
// backtick has no escape sequence inside a single-backtick span, so one in a
// raw path would close the span and let the remainder render as markdown.
func TestRawPathCannotEscapeCodeSpan(t *testing.T) {
	entries := []RecentError{
		{Time: errNow, Status: 500, Method: "GET", Path: "/a`x`**bold**", PathIsTemplate: false, Kind: "http", Err: "boom"},
	}
	snap := RecentErrors{Capacity: 256, Held: 1, Total: 1, Entries: entries}

	for _, tc := range []struct {
		name string
		got  string
	}{
		{"host.errors", renderHostErrorsFrom(snap, true, defaultErrorRows, "", errStart, errNow)},
		{"host.errors-summary", renderHostErrorsSummaryFrom(snap, true, errStart, errNow)},
	} {
		// Every backtick in the output should be one the renderer wrote. The
		// path contributes none, so the count must stay even and the injected
		// pair must be gone.
		if strings.Contains(tc.got, "/a`x`") {
			t.Errorf("%s: a backtick from the path survived into the code span:\n%s", tc.name, tc.got)
		}
	}
}
