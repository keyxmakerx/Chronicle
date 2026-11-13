// Package aiplug is the contract a feature implements to plug into the AI
// engine (ADR-071). A feature keeps what an AI may read and ask to change
// beside its own code, as a Kind (a block it can write) and a Lookup (a
// `what:` it can answer), and registers them at startup; the engine
// (internal/plugins/ai_workspace) parses, reviews, applies and serves them
// without knowing the feature. This package imports no plugin, so any
// plugin may implement it.
package aiplug

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Actions a record may carry; the same verbs as page rows.
const (
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
)

// Record is one non-page block from an AI's answer.
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
	// Text renders Body for storage. The engine always sets it on a record
	// it applies; nil only in a plan or a lookup, which render nothing.
	Text Text
	// Made holds the slugs of pages this paste creates or keeps, which
	// exist by the time records commit (pages commit first). Set when
	// planning, so a review doesn't warn about a page the paste makes.
	Made map[string]bool
}

// Text turns a record's markdown body into what Chronicle stores. The
// engine owns it because it resolves @[Page] links against the same paste
// and refuses GM-only markers in text players read.
type Text interface {
	// HTML is body as sanitized HTML; nil for an empty body.
	HTML(body string) (*string, error)
	// Entry is body as the page editor's document and its HTML; both nil
	// for an empty body.
	Entry(body string) (doc, html *string, err error)
}

// BodyEntry is the record's body as the page editor's document and its
// HTML; both nil for an empty body.
func (r Record) BodyEntry() (doc, html *string, err error) {
	if strings.TrimSpace(r.Body) == "" {
		return nil, nil, nil
	}
	if r.Text == nil {
		return nil, nil, errors.New("aiplug: a record with a body was applied without its Text")
	}
	return r.Text.Entry(r.Body)
}

// Actor is the person whose AI asked: the owner running an import, or the
// key's owner on a connected AI app.
type Actor struct {
	UserID string
	Role   int // visibility role (campaigns.CampaignContext.VisibilityRole)
	// ReadsOwnNotes marks a campaign member's own key. It reads at the
	// player role like an owner's "what players see" key does, but the notes
	// it reaches are the member's own plus those shared with them, which is
	// all the notes widget's visibility rule lets a player viewer open. The
	// owner's players-view key leaves it false and still gets no notes.
	ReadsOwnNotes bool
}

// Viewer is the Actor as the services' permission model sees them.
func (a Actor) Viewer() permissions.Viewer { return permissions.RequestViewer(a.Role, a.UserID) }

// CanAuthorDmOnly mirrors the UI's Owner-or-co-DM rule for hidden content.
func (a Actor) CanAuthorDmOnly() bool { return a.Viewer().SkipsPerUserRules() }

// Plan is what the review screen shows for one record before commit.
type Plan struct {
	Summary  string   // one plain line: what will happen
	Error    string   // blocks the row
	Warnings []string // shown, never block
	// Client is JSON handed to the browser (generator rows only): what
	// to run, and over which calendar.
	Client string
	// Preview is a rulebook Book (the JSON GET /book returns) of what the
	// row writes, drawn on the review screen by the rulebook reader so it
	// shows in the campaign's look. House rules only; "" for none.
	Preview string
}

// Kind is one kind of block an AI can ask to write.
type Kind interface {
	Name() string  // front-matter value, e.g. "event"
	Label() string // review-screen label, e.g. "Calendar event"
	// Doc is the prompt's description of this kind's keys, with an example.
	Doc() string
	// Plan checks a record for the review screen; it never writes.
	Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan
	// Apply writes an approved record through the feature's own service,
	// re-checking it first: the campaign may have changed since review.
	Apply(ctx context.Context, campaignID string, a Actor, r Record) error
	// Export lists what exists today in the same format, so an AI can
	// update or remove it by name. "" when there is nothing.
	Export(ctx context.Context, campaignID string, a Actor) (string, error)
}

// Briefed is a Kind with the one plain line the prompt's opening "what you
// can do" list shows for it.
type Briefed interface {
	Brief() string
}

// Staged is a kind whose row makes something the rows after it in the same
// paste may name: a calendar, a folder, a map. Commit applies rows in order,
// so the engine plans each later row with the context Stage returns for a
// row that planned without an error. Stage only notes what the row will
// make, on the context; it never writes.
type Staged interface {
	Stage(ctx context.Context, campaignID string, a Actor, r Record) context.Context
}

// Titled is a kind whose records have no name of their own; Title gives
// the review row's heading.
type Titled interface {
	Title(r Record) string
}

// ---- field helpers: front matter arrives as loosely typed YAML ----

// Visibility is the record's `visibility:` in the words records use
// (everyone or dm_only), also reading a page's words: public for everyone,
// private for Directors only.
func (r Record) Visibility() string {
	switch v := r.Str("visibility"); v {
	case "public":
		return "everyone"
	case "private":
		return "dm_only"
	default:
		return v
	}
}

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

// Date reads a day written YYYY-MM-DD, which YAML may hand over as a
// timestamp; anything else comes back as written, for the caller to check.
func (r Record) Date(key string) string {
	if t, ok := r.Fields[key].(time.Time); ok {
		return t.Format("2006-01-02")
	}
	return r.Str(key)
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
		return 0, true, BadRequestf("%s: %v is not a whole number", key, f)
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
			return 0, true, BadRequestf("%s is not a number", key)
		}
		return t, true, nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, true, BadRequestf("%s: %q is not a number", key, t)
		}
		return f, true, nil
	}
	return 0, true, BadRequestf("%s is not a number", key)
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

// ---- wording shared by every kind ----

// BadRequestf is a refusal worded for the person reviewing.
func BadRequestf(format string, args ...any) error {
	return apperror.NewBadRequest(fmt.Sprintf(format, args...))
}

// SameName compares names the way people read them.
func SameName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// Quote renders a YAML-safe scalar for exports.
func Quote(s string) string { return strconv.Quote(s) }

// PlanError is the text a review row shows for err: an AppError's own
// message (never its type prefix or internal cause), else a plain line.
func PlanError(err error) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code < 500 {
		return ae.Message
	}
	return "could not be checked; try again in a moment"
}
