// Package observability holds Chronicle's in-process self-observation: state
// the server records ABOUT ITSELF so an operator can ask what happened without
// shelling into the container.
//
// WHY it exists. Until now a Chronicle error that fired at 2am left no trace an
// admin could reach. It went to slog, slog went to stdout, and stdout went to
// the container's log driver — so answering "what broke overnight?" required
// shell access to the host, `docker logs`, and knowing which container. The
// audit plugin cannot serve this purpose: it records USER ACTIONS, is
// DB-backed, and is scoped to a campaign AND a user, so a 500 on /healthz or an
// anonymous request has neither of the keys it needs. This package is the
// missing piece — a small, bounded, in-memory record of recent server errors,
// readable through the admin diagnostics catalog.
//
// It is a LEAF: standard library only, no Chronicle imports. That is deliberate
// and load-bearing. The writers are internal/app (the central error handler)
// and internal/middleware (panic recovery); the reader is internal/systems (the
// diagnostics catalog) via provider injection. A leaf with no Chronicle imports
// cannot create a cycle with any of them.
//
// What it deliberately is NOT: durable, complete, or a log. The ring lives in
// process memory, so a restart empties it and a multi-replica deployment gives
// each replica its own. It is a recent-errors window for an operator standing
// in front of a running server, not an audit trail — and the diagnostics say so
// rather than letting a reader mistake an empty ring for a quiet night.
package observability

import (
	"strings"
	"sync"
	"time"
)

// DefaultCapacity is how many entries the process-wide ring holds. 256 is sized
// against the recording POLICY, not against traffic: only 5xx and explicitly
// unexpected errors are stored (see ShouldRecord), so 256 is a deep history of
// real failures rather than a few seconds of noise. It is also small enough
// that the whole ring is a fixed ~50 KB allocated once at startup.
const DefaultCapacity = 256

// maxErrLen bounds a stored error string. An error from the database driver can
// carry an entire failed statement including bound values; the diagnostics
// render through redactSecrets, but the rule this project follows is to avoid
// holding the secret in the first place. 300 characters is enough to identify
// what failed and far too short to carry a payload. Truncation is marked so a
// reader never mistakes a clipped message for the whole one.
const maxErrLen = 300

// Kind classifies WHERE an error came from, which is often more diagnostic than
// the status code: a 500 raised deliberately by a service (KindApp) and a 500
// that is a raw unwrapped error escaping a handler (KindRaw) look identical on
// the wire and mean completely different things to whoever fixes it.
type Kind string

const (
	// KindApp is an *apperror.AppError — a domain error the code raised on
	// purpose, carrying its own status and message.
	KindApp Kind = "app"
	// KindHTTP is an *echo.HTTPError — raised by the framework or by a
	// handler using Echo's own error type.
	KindHTTP Kind = "http"
	// KindRaw is an error that was NEITHER of the above. It reached the
	// central handler unwrapped, got the default 500, and is the class most
	// likely to be an actual bug rather than a handled condition.
	KindRaw Kind = "raw"
	// KindPanic is a recovered panic. It never reaches the central error
	// handler at all (see RecordPanic), so it is recorded separately.
	KindPanic Kind = "panic"
)

// Entry is one recorded error. It is deliberately a summary, not a transcript:
// no request body, no headers, no query string, no client IP, no user id. The
// fields here are what an operator needs to answer "what is failing, how often,
// and since when" — and nothing that would turn a diagnostic paste into a data
// disclosure.
type Entry struct {
	// Time is when the error was recorded, in the server's clock.
	Time time.Time
	// Status is the HTTP status that was (or will be) sent to the client.
	Status int
	// Method is the HTTP method. Cheap, non-identifying, and it separates a
	// failing GET from a failing POST on the same route.
	Method string
	// Path is a route TEMPLATE ("/campaigns/:id/entities/:slug") whenever the
	// router matched one — see PathFor for why the concrete path is not
	// stored. It is also what makes host.errors-summary able to collapse a
	// thousand failures on one route into one line.
	Path string
	// PathIsTemplate distinguishes a template from the fallback concrete path,
	// so the renderer can say which it is showing instead of leaving the
	// reader to guess whether ":id" is literal.
	PathIsTemplate bool
	// Kind is the error's provenance (see the Kind constants).
	Kind Kind
	// Err is the error's message, truncated to maxErrLen.
	Err string
}

// PathFor implements the "a path template, not a transcript" rule, and it is a
// named function rather than an inline expression because it is a PRIVACY
// DECISION that has to be reviewable in one place.
//
// Chronicle has routes whose path segments are live credentials —
// `/rsvp/:token`, `/proposals/respond/:token`, `/calendar-rsvp/:token`,
// `/join/:code`. Storing the concrete request path would put a working invite
// or RSVP token into a buffer whose entire purpose is to be pasted into a chat
// window. So when the router matched a route, the TEMPLATE is stored: it groups
// perfectly, it names the handler, and it cannot contain a secret.
//
// The concrete path is used only when there is no template, which in practice
// means the router matched nothing — and an unmatched request is a 404, which
// the default policy does not record at all. The boolean is returned so callers
// can label which one they got.
func PathFor(routeTemplate, rawPath string) (string, bool) {
	if t := strings.TrimSpace(routeTemplate); t != "" {
		return t, true
	}
	return rawPath, false
}

// ShouldRecord is THE recording policy, named and isolated so it can be read,
// argued with, and tested on its own.
//
// The rule: record 5xx and explicitly-unexpected errors; do not record ordinary
// 4xx. The reason is eviction, not volume. The ring is fixed-size and evicts
// oldest-first, so every recorded entry costs the oldest one. 4xx is the
// routine background of any web server — a crawler probing for /wp-admin, a
// stale bookmark, an expired CSRF token, a bot hammering a login form — and
// admitting it would let a 404 storm silently evict the single 500 that an
// operator came here to find. That failure mode is worse than not recording
// 4xx at all, because the ring would still LOOK full and healthy.
//
// The unexpected flag is the escape hatch for callers that know something is
// anomalous regardless of the status they are about to send (panic recovery
// uses it). It is not a way to reintroduce 4xx wholesale: nothing in the
// request path sets it from client input.
func ShouldRecord(status int, unexpected bool) bool {
	return unexpected || status >= 500
}

// Ring is a fixed-size, oldest-first-evicting buffer of Entry, safe for
// concurrent use. It is a plain mutex rather than a lock-free structure because
// writes only happen on the error path — a server writing here often enough for
// lock contention to matter has a much larger problem than this mutex.
type Ring struct {
	mu    sync.Mutex
	buf   []Entry
	next  int    // index of the next write
	full  bool   // whether the buffer has wrapped at least once
	total uint64 // every Record that passed the policy, INCLUDING evicted ones
}

// NewRing creates a ring holding capacity entries. A capacity of zero or less
// is coerced to DefaultCapacity: a zero-length ring would silently discard
// everything, which is exactly the "looks wired, records nothing" state these
// diagnostics exist to make impossible.
func NewRing(capacity int) *Ring {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Ring{buf: make([]Entry, capacity)}
}

// Record stores an entry, evicting the oldest when the ring is full. It applies
// no policy of its own — callers filter with ShouldRecord — so that a test can
// exercise eviction directly and so the policy has exactly one home.
//
// It fills a zero Time with the current time (mirroring EventLog.Record) and
// truncates the error message. It never blocks on anything but its own mutex
// and never returns an error: recording an error must not be able to fail in a
// way that complicates the code path that is already handling a failure.
func (r *Ring) Record(e Entry) {
	if r == nil {
		return // nil-safe: an unwired ring is a no-op, never a panic
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	e.Err = truncate(e.Err, maxErrLen)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.next] = e
	r.next++
	if r.next == len(r.buf) {
		r.next = 0
		r.full = true
	}
	r.total++
}

// Snapshot is an immutable copy of the ring's state, newest first, plus the
// counters that let a reader tell "nothing has gone wrong" apart from "nothing
// is recording" and from "so much went wrong that history was evicted".
type Snapshot struct {
	// Capacity is the ring's fixed size.
	Capacity int
	// Held is how many entries the ring currently contains (<= Capacity).
	Held int
	// Total is every error recorded since process start, including entries
	// already evicted. Total > Held is the eviction signal: it means the
	// window shown is incomplete, and the diagnostics say so.
	Total uint64
	// Entries is the requested slice of held entries, NEWEST FIRST.
	Entries []Entry
}

// Snapshot returns up to limit entries, newest first. limit <= 0 means all held
// entries — the summary diagnostic needs the whole ring to group over, while
// the listing diagnostic wants a page.
//
// It copies under the lock and returns owned memory, so a caller can render at
// its leisure while the server keeps recording.
func (r *Ring) Snapshot(limit int) Snapshot {
	if r == nil {
		return Snapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	held := r.next
	if r.full {
		held = len(r.buf)
	}
	s := Snapshot{Capacity: len(r.buf), Held: held, Total: r.total}
	if limit <= 0 || limit > held {
		limit = held
	}
	s.Entries = make([]Entry, 0, limit)
	// Walk backwards from the most recently written slot. r.next points at the
	// NEXT write, so the newest entry is at next-1, wrapping into the tail of
	// the buffer once the ring has filled.
	for i := 0; i < limit; i++ {
		idx := (r.next - 1 - i + len(r.buf)*2) % len(r.buf)
		s.Entries = append(s.Entries, r.buf[idx])
	}
	return s
}

// defaultRing is the process-wide ring. It is allocated eagerly at package init
// rather than wired at startup so that Record is safe from the very first
// request — an error during boot is precisely the error an operator most wants
// to see, and a nil-until-initialised ring would drop it.
var defaultRing = NewRing(DefaultCapacity)

// RecordHTTPError records an error the central HTTP error handler is about to
// render, applying ShouldRecord. Returns whether it was stored, which is only
// used by tests — production callers deliberately ignore it so that recording
// can never influence what the handler does next.
func RecordHTTPError(status int, method, routeTemplate, rawPath string, kind Kind, err error) bool {
	if !ShouldRecord(status, kind == KindPanic) {
		return false
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	p, templated := PathFor(routeTemplate, rawPath)
	defaultRing.Record(Entry{
		Status:         status,
		Method:         method,
		Path:           p,
		PathIsTemplate: templated,
		Kind:           kind,
		Err:            msg,
	})
	return true
}

// RecordPanic records a recovered panic.
//
// It exists separately because a recovered panic NEVER REACHES the central
// error handler: internal/middleware/recovery.go writes its 500 straight to the
// response with c.String and returns nil, so Echo sees no error and
// app.errorHandler is never called. Hooking only the error handler would
// therefore have left the single most valuable error class — the one that
// crashed a handler mid-flight — completely invisible in host.errors, while the
// diagnostic looked like it was working.
//
// The panic VALUE is recorded; the stack trace is NOT. The stack is already in
// the process log, and holding kilobytes of frames per entry would blow the
// ring's memory budget and bury the summary line an operator reads first.
func RecordPanic(method, routeTemplate, rawPath, panicValue string) {
	p, templated := PathFor(routeTemplate, rawPath)
	defaultRing.Record(Entry{
		Status:         500,
		Method:         method,
		Path:           p,
		PathIsTemplate: templated,
		Kind:           KindPanic,
		Err:            "panic: " + panicValue,
	})
}

// Recent returns a snapshot of the process-wide ring for the diagnostics.
func Recent(limit int) Snapshot { return defaultRing.Snapshot(limit) }

// truncate clips s to n bytes and MARKS that it did. An unmarked truncation is
// a lie by omission — a reader comparing two entries would see them as
// identical when only their shared prefix is. ToValidUTF8 drops the partial
// rune a byte-slice can leave at the cut, so the result is always printable.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…[truncated]"
}
