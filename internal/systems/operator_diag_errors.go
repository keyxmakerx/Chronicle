// Package systems — operator_diag_errors.go is the "what has this server been
// getting wrong?" half of the host diagnostics. host.build says which binary is
// running and host.assets/host.embedded say what it is serving; these say what
// it has been FAILING at, without anyone needing shell access.
//
// WHY it exists. Before this, an error that fired at 2am left no trace an admin
// could reach. It went to slog, slog went to stdout, stdout went to the
// container's log driver — so the only way to answer "what broke overnight?"
// was to get onto the host, find the container, and read `docker logs`. That is
// exactly the shell archaeology the whole host.* workstream exists to remove.
// The audit plugin cannot fill the gap: it records USER ACTIONS, DB-backed and
// scoped to both a campaign and a user, so a 500 on /healthz or an anonymous
// request has neither key it needs.
//
// What these diagnostics deliberately do NOT claim. The ring is in process
// memory: a restart empties it, and each replica keeps its own. So an empty
// result has THREE different meanings that must never be flattened into one —
// nothing was recorded, nothing is recording, or the process restarted since
// the failure. Every render below states which of those it is looking at, and
// the header always prints the ring's capacity and hold count so "only 4
// errors" is visibly different from "the ring was never wired". That
// distinction is the whole reason the header exists: on 2026-08-11 an hour was
// lost to reading an absence of evidence as evidence.
package systems

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/hostinfo"
)

// defaultErrorRows is how many entries host.errors prints with no argument.
// 25 is a screenful: enough to see a pattern, small enough to paste.
const defaultErrorRows = 25

// maxErrorRows caps the argument. A bound is fine; a bound nobody is told about
// is the defect — when it clamps, the output says so.
const maxErrorRows = 100

// RecentError is one recorded server error as the diagnostics see it. It
// mirrors internal/observability.Entry rather than importing it, for the same
// dependency inversion as EmbeddedAssetSet: the app layer owns the wiring and
// internal/systems stays free of it. The duplication is four scalar fields and
// buys a package boundary that cannot cycle.
//
// Note what is NOT here: no request body, no headers, no query string, no
// client IP, no user id. Path is a route TEMPLATE whenever the router matched
// one, because Chronicle routes `/rsvp/:token` and `/join/:code` and a concrete
// path would put a live credential into output meant to be pasted into a chat
// window.
type RecentError struct {
	Time           time.Time
	Status         int
	Method         string
	Path           string
	PathIsTemplate bool
	Kind           string // "app" | "http" | "raw" | "panic"
	Err            string
}

// RecentErrors is a snapshot of the error ring plus the counters that make an
// empty result interpretable.
type RecentErrors struct {
	// Capacity is the ring's fixed size.
	Capacity int
	// Held is how many entries it currently contains.
	Held int
	// Total is every error recorded since process start, INCLUDING entries
	// since evicted. Total > Held is the only signal that history was lost,
	// and the renderer must surface it — an operator reading a full-looking
	// window that silently dropped the first failure is back to guessing.
	Total uint64
	// Entries are the requested entries, NEWEST FIRST.
	Entries []RecentError
}

// recentErrorsFn returns a snapshot of the process error ring, or nil when the
// app layer has not wired it — in which case the diagnostics SAY "provider not
// wired" instead of reporting zero errors, because "no errors" and "nobody is
// recording errors" are opposite conclusions.
var recentErrorsFn func(limit int) RecentErrors

// SetRecentErrorsProvider wires the process error ring into the diagnostics.
// Called once at startup from the app wiring. limit <= 0 must return every held
// entry (host.errors-summary groups over the whole ring).
func SetRecentErrorsProvider(fn func(limit int) RecentErrors) { recentErrorsFn = fn }

// hostErrorsDiagnostic lists recent server errors newest-first.
func hostErrorsDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "host.errors",
		Title:   "Recent server errors this process recorded (no shell, no docker logs)",
		Desc:    "THE 'what broke overnight?' check. Newest first: time + age, status, method, route template, error. Records 5xx and recovered panics only, in memory, per process — so a restart empties it. Optional argument = how many to show (default 25, max 100).",
		ArgHint: "[<count>]",
		Run:     renderHostErrors,
	}
}

// hostErrorsSummaryDiagnostic collapses the same data into one line per
// failing route. A repeating error is ONE finding; printing it 25 times buries
// the single unrelated failure sitting underneath it.
func hostErrorsSummaryDiagnostic() Diagnostic {
	return Diagnostic{
		Name:  "host.errors-summary",
		Title: "Recent server errors GROUPED by route + status (one line per failure mode)",
		Desc:  "The same ring as host.errors, grouped by route template + status with counts and first/last seen, most frequent first. Usually the better first read: it collapses a repeating error into one line so a single odd failure stays visible.",
		Run:   func(string) string { return renderHostErrorsSummary() },
	}
}

// renderHostErrors wires the live sources into the pure renderer.
func renderHostErrors(arg string) string {
	limit, argNote := parseErrorLimit(arg)
	var snap RecentErrors
	wired := recentErrorsFn != nil
	if wired {
		snap = recentErrorsFn(limit)
	}
	return renderHostErrorsFrom(snap, wired, limit, argNote, hostinfo.ProcessStart(), time.Now())
}

// renderHostErrorsSummary wires the live sources into the pure summary
// renderer. It asks for the WHOLE ring (limit 0) because a group count computed
// over a page would be a lie about frequency.
func renderHostErrorsSummary() string {
	var snap RecentErrors
	wired := recentErrorsFn != nil
	if wired {
		snap = recentErrorsFn(0)
	}
	return renderHostErrorsSummaryFrom(snap, wired, hostinfo.ProcessStart(), time.Now())
}

// parseErrorLimit turns the optional argument into a row count, returning a
// note when it had to correct the caller. A silently ignored argument is how an
// operator ends up believing they saw 100 errors when they saw 25.
func parseErrorLimit(arg string) (int, string) {
	s := strings.TrimSpace(arg)
	if s == "" {
		return defaultErrorRows, ""
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultErrorRows, fmt.Sprintf("_Argument %q is not a number; showing the default %d._", s, defaultErrorRows)
	}
	if n <= 0 {
		return defaultErrorRows, fmt.Sprintf("_Argument %d is not a positive count; showing the default %d._", n, defaultErrorRows)
	}
	if n > maxErrorRows {
		return maxErrorRows, fmt.Sprintf("_Asked for %d; clamped to the %d-row maximum._", n, maxErrorRows)
	}
	return n, ""
}

// renderHostErrorsFrom is the pure renderer. Every input is injected so the
// three states that matter — unwired, wired-and-empty, wired-and-evicting — are
// each testable, since none of them can be produced on demand in a test binary.
func renderHostErrorsFrom(snap RecentErrors, wired bool, limit int, argNote string, start, now time.Time) string {
	var out strings.Builder
	out.WriteString("## host.errors — recent server errors recorded by this process\n\n")
	if argNote != "" {
		out.WriteString(argNote + "\n\n")
	}
	if !wired {
		writeErrorsUnwired(&out)
		return out.String()
	}
	writeErrorsHeader(&out, snap, start, now)

	if len(snap.Entries) == 0 {
		fmt.Fprintf(&out, "\n**No error has been recorded in this process's %s of uptime.**\n", shortDuration(now.Sub(start)))
		out.WriteString("This is an EMPTY result, not an unwired one — the ring above exists and is holding zero entries. It means no 5xx and no panic since this process started; it does not mean no 4xx (those are not recorded, see the policy note) and it does not cover anything that happened before the last restart.\n\n")
		writeErrorsPolicyNote(&out)
		return out.String()
	}

	fmt.Fprintf(&out, "\n### Newest first — showing %d of %d held (requested %d)\n\n", len(snap.Entries), snap.Held, limit)
	for _, e := range snap.Entries {
		writeErrorLine(&out, e, now)
	}
	out.WriteString("\n")
	writeErrorsPolicyNote(&out)
	return out.String()
}

// writeErrorLine renders one entry. The AGE is printed next to the timestamp
// because the first question an operator asks a stack of errors is "was that
// just now, or is this all from last Tuesday?" — a bare RFC3339 timestamp makes
// them do that subtraction in their head, twenty-five times.
func writeErrorLine(out *strings.Builder, e RecentError, now time.Time) {
	fmt.Fprintf(out, "- `%s` _(%s)_ · **%d** %s `%s`%s · %s\n",
		e.Time.UTC().Format(time.RFC3339),
		relativeTo(e.Time, now),
		e.Status,
		orDash(e.Method),
		orDash(e.Path),
		templateMarker(e),
		kindLabel(e.Kind),
	)
	if strings.TrimSpace(e.Err) != "" {
		fmt.Fprintf(out, "  - %s\n", singleLine(e.Err))
	}
}

// writeErrorsHeader prints the ring's own state FIRST, so a reader can size
// what follows before reading it. Without this line "4 errors" is ambiguous
// between a quiet server and a broken diagnostic.
func writeErrorsHeader(out *strings.Builder, snap RecentErrors, start, now time.Time) {
	fmt.Fprintf(out, "**Ring: %d slots · %d held · %d recorded since process start** (process up %s, started %s)\n",
		snap.Capacity, snap.Held, snap.Total,
		shortDuration(now.Sub(start)), start.UTC().Format(time.RFC3339))
	if snap.Total > uint64(snap.Held) {
		fmt.Fprintf(out, "\n> **The ring has wrapped — %d earlier error(s) have been evicted and are gone.** What follows is the newest %d only, so do NOT read the oldest line here as the first occurrence. If the counts matter, `host.errors-summary` groups the whole ring at once.\n",
			snap.Total-uint64(snap.Held), snap.Held)
	}
}

// writeErrorsUnwired is the "nobody is recording" render. It is emphatic on
// purpose: this is the one output that must never be mistaken for good news.
func writeErrorsUnwired(out *strings.Builder) {
	out.WriteString("**Provider not wired** — the app layer did not inject the error ring at startup, so no errors can be listed.\n\n")
	out.WriteString("**This is not \"no errors have occurred\".** It means nothing in this binary is reporting them to this diagnostic. Errors are still going to the process log; read them with `docker logs <chronicle>` until the wiring is restored. Expected wiring: `systems.SetRecentErrorsProvider(...)` in the app's route registration.\n")
}

// writeErrorsPolicyNote states the recording policy and the ring's limits every
// time. It is unconditional and repeated rather than documented once elsewhere,
// because the reader of this output is mid-incident and will not go looking:
// silently omitting 4xx, or silently losing history at restart, is precisely
// the kind of thing that gets read as a finding when it is a design decision.
func writeErrorsPolicyNote(out *strings.Builder) {
	out.WriteString("### What is and is not in here\n\n")
	out.WriteString("- **Recorded: 5xx responses and recovered panics.** Nothing else.\n")
	out.WriteString("- **NOT recorded: ordinary 4xx** (404, 401, 403, 422, …). The ring evicts oldest-first, so admitting the routine 4xx background of any public server — crawlers, stale bookmarks, expired CSRF tokens — would let a 404 storm quietly evict the one 500 you came here for. An absence of 4xx here says nothing about whether 4xx are happening.\n")
	out.WriteString("- **Paths are ROUTE TEMPLATES** (`/campaigns/:id/...`), not the URLs that were requested. Chronicle routes `/rsvp/:token` and `/join/:code`, so a concrete path could carry a live credential into output meant to be pasted elsewhere. A path shown without `:` placeholders and marked _(raw path)_ is one the router did not match.\n")
	out.WriteString("- **In memory, this process only.** A restart empties the ring, and each replica keeps its own — so an empty result may just mean the process restarted after the failure. Anything older than the uptime above is only in the process log.\n")
	out.WriteString("- **Full detail lives in the log.** Panic stack traces in particular are logged but deliberately not stored here; use `docker logs <chronicle>` with a timestamp from a line above to find one.\n")
}

// renderHostErrorsSummaryFrom is the pure summary renderer.
func renderHostErrorsSummaryFrom(snap RecentErrors, wired bool, start, now time.Time) string {
	var out strings.Builder
	out.WriteString("## host.errors-summary — recent server errors grouped by route + status\n\n")
	if !wired {
		writeErrorsUnwired(&out)
		return out.String()
	}
	writeErrorsHeader(&out, snap, start, now)

	groups := groupErrors(snap.Entries)
	if len(groups) == 0 {
		fmt.Fprintf(&out, "\n**No error has been recorded in this process's %s of uptime.**\n", shortDuration(now.Sub(start)))
		out.WriteString("This is an EMPTY result, not an unwired one — the ring above exists and is holding zero entries.\n\n")
		writeErrorsPolicyNote(&out)
		return out.String()
	}

	fmt.Fprintf(&out, "\n### %d distinct failure mode(s) across %d held entries — most frequent first\n\n", len(groups), snap.Held)
	for _, g := range groups {
		writeErrorGroup(&out, g, now)
	}
	out.WriteString("\n")
	writeErrorsPolicyNote(&out)
	return out.String()
}

// errorGroup is one failure mode: a route+status pair and everything observed
// under it. Distinct messages are counted rather than listed — a group whose
// count is 300 but whose message count is 1 is a stuck route, while 300 entries
// with 300 distinct messages is something else entirely, and that ratio is the
// most useful single number in the whole summary.
type errorGroup struct {
	Status    int
	Path      string
	Templated bool
	Methods   []string
	Kinds     []string
	Count     int
	First     time.Time
	Last      time.Time
	SampleErr string // the message from the most recent entry in the group
	Distinct  int    // how many distinct messages the group contains
}

// groupErrors collapses entries by status + path, as the brief specifies.
// Method is NOT part of the key — a route that fails for both GET and POST is
// one broken route, not two — but every method observed is listed, because
// "only POST fails" is itself a finding.
//
// Sorted by count descending, then most-recent-last-seen, then path, so the
// order is stable for a test and useful for a human: the loudest failure first,
// ties broken by whichever is still happening.
func groupErrors(entries []RecentError) []errorGroup {
	type key struct {
		status int
		path   string
	}
	index := map[key]*errorGroup{}
	seenMethod := map[key]map[string]bool{}
	seenKind := map[key]map[string]bool{}
	seenMsg := map[key]map[string]bool{}

	for _, e := range entries {
		k := key{status: e.Status, path: e.Path}
		g, ok := index[k]
		if !ok {
			g = &errorGroup{
				Status:    e.Status,
				Path:      e.Path,
				Templated: e.PathIsTemplate,
				First:     e.Time,
				Last:      e.Time,
				SampleErr: e.Err,
			}
			index[k] = g
			seenMethod[k] = map[string]bool{}
			seenKind[k] = map[string]bool{}
			seenMsg[k] = map[string]bool{}
		}
		g.Count++
		// Entries arrive newest-first, so the FIRST message seen for a group
		// is its most recent one — keep that as the sample and let the bounds
		// widen in both directions regardless of arrival order.
		if e.Time.Before(g.First) {
			g.First = e.Time
		}
		if e.Time.After(g.Last) {
			g.Last = e.Time
			g.SampleErr = e.Err
		}
		if m := strings.TrimSpace(e.Method); m != "" && !seenMethod[k][m] {
			seenMethod[k][m] = true
			g.Methods = append(g.Methods, m)
		}
		if kd := strings.TrimSpace(e.Kind); kd != "" && !seenKind[k][kd] {
			seenKind[k][kd] = true
			g.Kinds = append(g.Kinds, kd)
		}
		if !seenMsg[k][e.Err] {
			seenMsg[k][e.Err] = true
			g.Distinct++
		}
	}

	out := make([]errorGroup, 0, len(index))
	for _, g := range index {
		sort.Strings(g.Methods)
		sort.Strings(g.Kinds)
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if !out[i].Last.Equal(out[j].Last) {
			return out[i].Last.After(out[j].Last)
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Status < out[j].Status
	})
	return out
}

// writeErrorGroup renders one failure mode.
func writeErrorGroup(out *strings.Builder, g errorGroup, now time.Time) {
	methods := "—"
	if len(g.Methods) > 0 {
		methods = strings.Join(g.Methods, "/")
	}
	marker := ""
	if !g.Templated {
		marker = " _(raw path — the router matched no route)_"
	}
	fmt.Fprintf(out, "- **%d×** · **%d** %s `%s`%s · %s\n",
		g.Count, g.Status, methods, orDash(g.Path), marker, kindLabel(strings.Join(g.Kinds, "+")))
	fmt.Fprintf(out, "  - first seen %s (%s) · last seen %s (%s)\n",
		g.First.UTC().Format(time.RFC3339), relativeTo(g.First, now),
		g.Last.UTC().Format(time.RFC3339), relativeTo(g.Last, now))
	if strings.TrimSpace(g.SampleErr) != "" {
		fmt.Fprintf(out, "  - latest message: %s\n", singleLine(g.SampleErr))
	}
	if g.Distinct > 1 {
		fmt.Fprintf(out, "  - %d distinct error messages in this group — the sample above is only the most recent.\n", g.Distinct)
	}
}

// kindLabel expands the stored provenance code into something a reader who has
// never seen this catalog can act on. `raw` in particular is the interesting
// one: a 500 the code raised deliberately and a 500 that escaped a handler
// unwrapped look identical on the wire and mean completely different things.
func kindLabel(kind string) string {
	switch kind {
	case "":
		return "_kind unrecorded_"
	case "app":
		return "`app` _(a domain AppError — raised deliberately)_"
	case "http":
		return "`http` _(an echo.HTTPError)_"
	case "raw":
		return "`raw` _(an unwrapped error reached the handler — most likely an actual bug)_"
	case "panic":
		return "`panic` _(**recovered panic** — the stack trace is in the process log, not here)_"
	default:
		return "`" + kind + "`"
	}
}

// templateMarker flags an entry whose path is a real requested path rather than
// a route template, so nobody reads a literal id as a placeholder or vice versa.
func templateMarker(e RecentError) string {
	if e.PathIsTemplate {
		return ""
	}
	return " _(raw path)_"
}

// singleLine flattens an error message onto one line. A multi-line error (a
// wrapped SQL error, say) would otherwise break the bullet list and make the
// following entries look like they belong to it.
func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

// orDash keeps an absent value from rendering as empty backticks, which reads
// as a rendering bug rather than as a gap.
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
