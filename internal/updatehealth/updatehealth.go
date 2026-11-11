// Package updatehealth counts, on this server only, how often each feature an
// update changed is used and whether it fails, so a site admin hears about
// the changed features that break or that nobody has tried, and not about the
// ones that quietly work. Nothing is sent anywhere.
//
// A feature is "changed" when it is listed in Changed. The server notes when
// it first ran with each entry and counts from then; requests are matched by
// their route template, so a path carrying a token is never stored.
package updatehealth

import (
	"strings"
	"sync"
	"time"
)

// Feature is one changed feature as a person would name it, with the routes
// that serve it and a page where a site admin can try it.
type Feature struct {
	// ID is stable for the life of the entry; counts are stored under it.
	ID string
	// Name is what the Needs you row calls it, e.g. "Crafting".
	Name string
	// Routes are route templates as registered, optionally led by a method
	// ("POST /campaigns/:id/shop/buy") and optionally ending in "*" to take
	// every route under a prefix ("/campaigns/:id/crafting*").
	Routes []string
	// Try is the page the "Try it now" button opens.
	Try string
}

// delta is what has been counted since the last flush.
type delta struct {
	uses, errors int
	lastError    time.Time
	lastErrorMsg string
}

// rule is one parsed entry of Feature.Routes.
type rule struct {
	method string // "" for any method
	path   string
	prefix bool
	index  int // into Recorder.features
}

// Recorder matches finished requests to changed features and keeps the counts
// in memory until Flush hands them to the store. Recording happens on every
// request, so it is a map lookup under a mutex and never touches the database.
type Recorder struct {
	features []Feature
	rules    []rule

	mu      sync.Mutex
	pending map[string]*delta
}

// NewRecorder builds a recorder for the given features.
func NewRecorder(features []Feature) *Recorder {
	r := &Recorder{features: features, pending: map[string]*delta{}}
	for i, f := range features {
		for _, spec := range f.Routes {
			ru := rule{index: i}
			spec = strings.TrimSpace(spec)
			if m, p, ok := strings.Cut(spec, " "); ok {
				ru.method, spec = strings.ToUpper(m), strings.TrimSpace(p)
			}
			if strings.HasSuffix(spec, "*") {
				ru.prefix, spec = true, strings.TrimSuffix(spec, "*")
			}
			ru.path = spec
			r.rules = append(r.rules, ru)
		}
	}
	return r
}

// Features returns the features this recorder counts.
func (r *Recorder) Features() []Feature {
	if r == nil {
		return nil
	}
	return r.features
}

// Match reports which features a request to route template path serves.
func (r *Recorder) Match(method, path string) []int {
	if r == nil || path == "" {
		return nil
	}
	var out []int
	for _, ru := range r.rules {
		if ru.method != "" && ru.method != method {
			continue
		}
		if path == ru.path || (ru.prefix && strings.HasPrefix(path, ru.path)) {
			if len(out) == 0 || out[len(out)-1] != ru.index {
				out = append(out, ru.index)
			}
		}
	}
	return out
}

// RecordUse counts a request that worked.
func (r *Recorder) RecordUse(method, path string) {
	for _, i := range r.Match(method, path) {
		r.bump(r.features[i].ID, func(d *delta) { d.uses++ })
	}
}

// RecordError counts a request that failed on the server's side (a 5xx).
// msg is the error as the error log already keeps it; it is cut short here
// too, since it is stored.
func (r *Recorder) RecordError(method, path, msg string, at time.Time) {
	for _, i := range r.Match(method, path) {
		r.bump(r.features[i].ID, func(d *delta) {
			d.errors++
			d.lastError = at
			d.lastErrorMsg = clip(method+" "+path+": "+msg, maxErrorText)
		})
	}
}

func (r *Recorder) bump(id string, f func(*delta)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.pending[id]
	if d == nil {
		d = &delta{}
		r.pending[id] = d
	}
	f(d)
}

// take empties the pending counts and returns them.
func (r *Recorder) take() map[string]*delta {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.pending
	r.pending = map[string]*delta{}
	return out
}

// giveBack returns counts a failed flush could not store, so a database blip
// loses nothing.
func (r *Recorder) giveBack(m map[string]*delta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, d := range m {
		cur := r.pending[id]
		if cur == nil {
			r.pending[id] = d
			continue
		}
		cur.uses += d.uses
		cur.errors += d.errors
		if d.lastError.After(cur.lastError) {
			cur.lastError, cur.lastErrorMsg = d.lastError, d.lastErrorMsg
		}
	}
}

// peek copies the pending counts, so a report can include what has not been
// flushed yet.
func (r *Recorder) peek() map[string]delta {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]delta, len(r.pending))
	for id, d := range r.pending {
		out[id] = *d
	}
	return out
}

// maxErrorText bounds the stored error line; the full error is in the log.
const maxErrorText = 300

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// defaultRecorder is the process recorder the middleware and error handler
// write to. It is nil until the app wires it, and a nil recorder ignores
// everything, so recording can never fail a request.
var (
	defaultMu       sync.RWMutex
	defaultRecorder *Recorder
)

// SetDefault installs the process recorder.
func SetDefault(r *Recorder) {
	defaultMu.Lock()
	defaultRecorder = r
	defaultMu.Unlock()
}

func current() *Recorder {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultRecorder
}

// Use counts a request that worked against the process recorder.
func Use(method, path string) {
	if r := current(); r != nil {
		r.RecordUse(method, path)
	}
}

// Fail counts a 5xx against the process recorder.
func Fail(method, path, msg string) {
	if r := current(); r != nil {
		r.RecordError(method, path, msg, time.Now())
	}
}
