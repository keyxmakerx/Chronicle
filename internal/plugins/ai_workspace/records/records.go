// Package records lets AI Import reach the campaign features that are not
// pages: calendar events, day weather, rolling tables, shop stock, carried
// items, map pins, house rules, the operator's own notes, and Chronicle's
// generators. A page whose front matter carries `kind:` (anything but
// "page") is a Record; each Kind validates it at review time (Plan) and
// writes it at commit time (Apply) through the owning plugin's service.
//
// Every Kind gets the operator as an Actor and acts with exactly what the
// normal UI would allow that operator: the import routes are owner-gated,
// and notes are narrowed further to the operator's own (see notes.go).
package records

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
)

// Actions a record may carry; the same verbs as page rows.
const (
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
)

// Record is one non-page block from the pasted markdown.
type Record struct {
	Index  int
	Kind   string
	Action string
	Name   string
	Fields map[string]any // the whole front matter, keys lower-cased
	Body   string         // markdown after the front matter
	// Generated is the browser's generator output (JSON), set at commit
	// for generator rows only. Untrusted: kinds validate it like any input.
	Generated string
	// Links resolves @[Page] in Body to page links. Nil leaves the text as
	// written (e.g. when planning).
	Links *importer.PageLinks
}

// Actor is the operator running the import.
type Actor struct {
	UserID string
	Role   int // visibility role (campaigns.CampaignContext.VisibilityRole)
}

// Viewer is the operator as the services' permission model sees them.
func (a Actor) Viewer() permissions.Viewer { return permissions.RequestViewer(a.Role, a.UserID) }

// CanAuthorDmOnly mirrors the UI's Owner-or-co-DM rule for hidden content.
func (a Actor) CanAuthorDmOnly() bool { return a.Viewer().SkipsPerUserRules() }

// Titled is a kind whose records have no name of their own; Title gives
// the review row's heading.
type Titled interface {
	Title(r Record) string
}

// Plan is what the review screen shows for one record before commit.
type Plan struct {
	Summary  string   // one plain line: what will happen
	Error    string   // blocks the row
	Warnings []string // shown, never block
	// Client is JSON handed to the browser (generator rows only): what
	// to run, and over which calendar.
	Client string
}

// Kind is one feature AI Import can write.
type Kind interface {
	Name() string  // front-matter value, e.g. "event"
	Label() string // review-screen label, e.g. "Calendar event"
	// Doc is the prompt's description of this kind's keys, with an example.
	Doc() string
	Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan
	Apply(ctx context.Context, campaignID string, a Actor, r Record) error
	// Export lists what exists today in the same format, so an AI can
	// update or remove it by name. "" when there is nothing.
	Export(ctx context.Context, campaignID string, a Actor) (string, error)
}

// Registry holds the kinds wired at startup, in prompt order.
type Registry struct {
	kinds []Kind
	byKey map[string]Kind
}

// NewRegistry registers kinds; a nil kind (its service is not wired) is skipped.
func NewRegistry(kinds ...Kind) *Registry {
	r := &Registry{byKey: map[string]Kind{}}
	for _, k := range kinds {
		if k == nil {
			continue
		}
		r.kinds = append(r.kinds, k)
		r.byKey[k.Name()] = k
	}
	return r
}

// Kinds lists the registered kinds in order.
func (r *Registry) Kinds() []Kind {
	if r == nil {
		return nil
	}
	return r.kinds
}

// Get returns the kind for a front-matter value (case-insensitive, "-"/"_" alike).
func (r *Registry) Get(name string) (Kind, bool) {
	if r == nil {
		return nil, false
	}
	k, ok := r.byKey[normKind(name)]
	return k, ok
}

func normKind(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "_", "-")
}

// Plan validates a record, refusing unknown kinds and bad actions up front.
func (r *Registry) Plan(ctx context.Context, campaignID string, a Actor, rec Record) (Kind, Plan) {
	k, ok := r.Get(rec.Kind)
	if !ok {
		return nil, Plan{Error: fmt.Sprintf("kind: %q is not something AI Import can change", rec.Kind)}
	}
	switch rec.Action {
	case ActionCreate, ActionUpdate, ActionDelete:
	default:
		return k, Plan{Error: fmt.Sprintf("action: %q is not valid (must be create, update or delete)", rec.Action)}
	}
	return k, k.Plan(ctx, campaignID, a, rec)
}

// PlanAll plans records in order. A row that makes the calendar lets the
// rows after it be checked against that calendar, since commit applies
// them in the same order.
func (r *Registry) PlanAll(ctx context.Context, campaignID string, a Actor, recs []Record) ([]Kind, []Plan) {
	kinds := make([]Kind, len(recs))
	plans := make([]Plan, len(recs))
	for i, rec := range recs {
		kinds[i], plans[i] = r.Plan(ctx, campaignID, a, rec)
		if ck, ok := kinds[i].(CalendarKind); ok && rec.Action == ActionCreate && plans[i].Error == "" {
			if _, cal := ck.plan(ctx, campaignID, a, rec); cal != nil {
				ctx = withPendingCalendar(ctx, cal)
			}
		}
	}
	return kinds, plans
}

// briefs is one plain line per kind for the prompt's opening "what you
// can do" list, which every prompt carries; Docs has the full formats.
var briefs = map[string]string{
	KindCalendar:   "make the campaign calendar, or change its months, weekdays, seasons, moons, year label, eras and current date",
	"event":        "calendar events on the campaign calendar, one-off or repeating",
	"weather":      "one day's weather on the calendar",
	"table":        "rolling tables and their entries",
	"shop-stock":   "what a shop sells, its price and how many",
	"carried-item": "items a character carries",
	"pin":          "pins on the campaign's maps",
	"note":         "my own notes only: my Journal notes and my jots on pages, never notes anyone else wrote",
	"house-rule":   "house-rules chapters in the campaign's rulebook",
	"system-entry": "the campaign's own ancestries, kits, cultures, classes and other pick-list entries for its game system",
	"generator":    "run Chronicle's own generators (weather for a range of days, festivals and other events, sky events, names into a rolling table) instead of inventing the result",
}

// Capabilities is the prompt's opening list of what a block can change,
// one line per wired kind.
func (r *Registry) Capabilities() string {
	var b strings.Builder
	for _, k := range r.Kinds() {
		brief := briefs[k.Name()]
		if brief == "" {
			brief = k.Label()
		}
		b.WriteString("- `kind: " + k.Name() + "`: " + brief + "\n")
	}
	return b.String()
}

// Docs is the prompt section describing every kind.
func (r *Registry) Docs() string {
	var b strings.Builder
	for _, k := range r.Kinds() {
		b.WriteString("### kind: " + k.Name() + " (" + k.Label() + ")\n\n")
		b.WriteString(strings.TrimSpace(k.Doc()))
		b.WriteString("\n\n")
	}
	return b.String()
}

// ExportAll concatenates every kind's export. A failing kind is noted, not fatal.
func (r *Registry) ExportAll(ctx context.Context, campaignID string, a Actor) string {
	var b strings.Builder
	for _, k := range r.Kinds() {
		s, err := k.Export(ctx, campaignID, a)
		if err != nil {
			b.WriteString("<!-- " + k.Label() + ": could not be listed -->\n\n")
			continue
		}
		if strings.TrimSpace(s) == "" {
			continue
		}
		b.WriteString("## " + k.Label() + "s\n\n" + s + "\n")
	}
	return b.String()
}

// ---- field helpers: front matter arrives as loosely typed YAML ----

// Str returns a string field ("" when absent).
func (r Record) Str(key string) string {
	v, ok := r.Fields[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case int:
		return strconv.Itoa(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// Has reports whether the key was written at all.
func (r Record) Has(key string) bool { _, ok := r.Fields[key]; return ok }

// Int reads a whole number; ok is false when absent, and err when not a number.
func (r Record) Int(key string) (n int, ok bool, err error) {
	f, ok, err := r.Float(key)
	if !ok || err != nil {
		return 0, ok, err
	}
	if f != math.Trunc(f) {
		return 0, true, badRequestf("%s: %v is not a whole number", key, f)
	}
	return int(f), true, nil
}

// Float reads a number; ok is false when absent, and err when not a number.
func (r Record) Float(key string) (float64, bool, error) {
	v, ok := r.Fields[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	switch t := v.(type) {
	case int:
		return float64(t), true, nil
	case int64:
		return float64(t), true, nil
	case uint64:
		return float64(t), true, nil
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, true, badRequestf("%s is not a number", key)
		}
		return t, true, nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, true, badRequestf("%s: %q is not a number", key, t)
		}
		return f, true, nil
	}
	return 0, true, badRequestf("%s is not a number", key)
}

// Bool reads yes/no; ok is false when absent.
func (r Record) Bool(key string) (bool, bool) {
	v, ok := r.Fields[key]
	if !ok {
		return false, false
	}
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "yes" || s == "on", true
	}
	return false, true
}

// List reads a YAML list (or a single value) as items.
func (r Record) List(key string) []any {
	v, ok := r.Fields[key]
	if !ok || v == nil {
		return nil
	}
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{v}
}

// badRequestf is a refusal worded for the operator.
func badRequestf(format string, args ...any) error {
	return apperror.NewBadRequest(fmt.Sprintf(format, args...))
}

// sameName compares names the way people read them.
func sameName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// quote renders a YAML-safe scalar for exports.
func quote(s string) string { return strconv.Quote(s) }

// planError is the text a review row shows for err: an AppError's own
// message (never its type prefix or internal cause), else a plain line.
func planError(err error) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code < 500 {
		return ae.Message
	}
	return "could not be checked; try again in a moment"
}
