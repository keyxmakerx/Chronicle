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
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
)

// Actions a record may carry; the same verbs as page rows.
const (
	ActionCreate = aiplug.ActionCreate
	ActionUpdate = aiplug.ActionUpdate
	ActionDelete = aiplug.ActionDelete
)

// The contract types live in aiplug so a feature can implement a kind
// without importing this plugin; these names keep the engine's code short.
type (
	Record = aiplug.Record
	Actor  = aiplug.Actor
	Plan   = aiplug.Plan
	Kind   = aiplug.Kind
	Titled = aiplug.Titled
)

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
		// Keyed as Get looks it up, so a name spelled with "_" is found.
		r.byKey[normKind(k.Name())] = k
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

// RefuseGMMarkers rejects a record whose body, name or any front-matter text
// carries a GM-only marker. Record texts (events, notes, summaries,
// descriptions) reach players without the strip that page text gets, so
// honouring or keeping a marker there would publish it. The error is a 400 so
// the review and result screens show its wording.
func RefuseGMMarkers(r Record) error {
	if importer.HasSecretMarkerText(r.Body) || importer.HasSecretMarkerText(r.Name) || anyMarker(r.Fields) {
		return apperror.NewBadRequest(importer.SecretNotInTextReason)
	}
	return nil
}

// anyMarker reports whether a front-matter value, at any depth, holds a
// GM-only marker.
func anyMarker(v any) bool {
	switch t := v.(type) {
	case string:
		return importer.HasSecretMarkerText(t)
	case map[string]any:
		for _, x := range t {
			if anyMarker(x) {
				return true
			}
		}
	case []any:
		for _, x := range t {
			if anyMarker(x) {
				return true
			}
		}
	}
	return false
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
	if err := RefuseGMMarkers(rec); err != nil {
		return k, Plan{Error: importer.SecretNotInTextReason}
	}
	return k, k.Plan(ctx, campaignID, a, rec)
}

// PlanAll plans records in order. A row that makes the calendar, or makes
// another one the main calendar, lets the rows after it be checked against
// that calendar, since commit applies them in the same order; a row that
// makes a folder or a map does the same for that folder or map.
func (r *Registry) PlanAll(ctx context.Context, campaignID string, a Actor, recs []Record) ([]Kind, []Plan) {
	ctx = withPendingMaps(withPendingFolders(ctx))
	kinds := make([]Kind, len(recs))
	plans := make([]Plan, len(recs))
	for i, rec := range recs {
		kinds[i], plans[i] = r.Plan(ctx, campaignID, a, rec)
		if fk, ok := kinds[i].(FolderKind); ok && rec.Action == ActionCreate && plans[i].Error == "" {
			if t, err := fk.resolve(ctx, campaignID, rec); err == nil {
				pendingFolders(ctx)[folderKey(t.cat.ID, rec.Name)] = true
			}
		}
		if _, ok := kinds[i].(MapSketchKind); ok && rec.Action == ActionCreate && plans[i].Error == "" {
			pendingMaps(ctx)[mapKey(sketchName(rec))] = true
		}
		if ck, ok := kinds[i].(CalendarKind); ok && (rec.Action == ActionCreate || rec.Has("main")) && plans[i].Error == "" {
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
	KindCalendar:     "make the campaign calendar, or change its months, weekdays, seasons, moons, year label, eras and current date",
	"event":          "calendar events on the campaign calendar, one-off or repeating",
	"weather":        "one day's weather on the calendar",
	"table":          "rolling tables and their entries",
	"shop-stock":     "what a shop sells, its price and how many",
	"carried-item":   "items a character carries",
	"pin":            "pins on the campaign's maps",
	"map":            "new maps drawn from a sketch on a grid of squares: rooms, areas, water, danger zones, walls, roads, paths, labels and pins",
	"note":           "my own notes only: my Journal notes and my jots on pages, never notes anyone else wrote",
	KindPlayerNote:   "my own Player Notes on pages only, never Player Notes anyone else wrote",
	"house-rule":     "house-rules chapters in the campaign's rulebook, written whole: pages with highlighted boxes, dice rolls, fold-out cards, flaps, worked examples, creatures and Director's notes",
	"system-entry":   "the campaign's own ancestries, kits, cultures, classes and other pick-list entries for its game system",
	"encounter":      "fights planned on a page for Foundry to load: the heroes, each creature's count, initiative group and whether it starts seen, hidden or arrives later",
	"field-rule":     "rules that work a sheet field out from another, such as a guild rank from a level",
	"local-word":     "what a place or an era calls a word, such as a coin or a rank, shown in shops and on sheets",
	"category-field": "the fields on a category's sheet, including links to another category's pages and fields worked out by a rule",
	"tag":            "the campaign's tags: make, rename, recolour, hide from players or remove them (which pages carry a tag is set on the page block)",
	"folder":         "sidebar folders inside a category (such as Gods and Cosmology inside Lore); a page is filed in one with `folder:` on its own block",
	"menu-order":     "the order of the categories in the sidebar menu (owner only)",
	"quest":          "a quest page's notice: its status, title, blurb, reward, due date and steps, and which steps players see",
	"notice-board":   "notice boards on a place or category page: who may pin to them, which quests fill them by themselves, and the quests and notes pinned by hand",
	"page-map":       "the small map drawn on a page, written as one marker or shape per line",
	"material":       "which kinds of crafting material an Armory item is (herb, bone, Remnant part and the rest), so recipes asking for \"any\" of a kind take it",
	"recipe":         "crafting recipes: the item made, its materials, the method its steps follow, the project goal, characteristic, cost, who may make it, and its \"When… then…\" rules (new recipes and new rules arrive as drafts for the Director to approve)",
	"bench":          "the campaign's crafting benches, by name and kind, which a recipe's rules can name",
	"shop-room":      "the room players step into from a shop: what kind of shop it is, its look and mood, the keeper's manner and their own lines for each moment of a visit",
	"storefront":     "how a shop's front looks on the Shops street: building, colours, awning, sign, window wares, lamp and its closed look",
	"drops":          "a creature's or other page's drop list: what it leaves in the spoils tray when defeated or searched, taken as it is or harvested with a test",
	"roll_night":     "one night of an event's yearly roll: roll it again, lock it, or choose whether players are told",
	"moon":           "a moon the Director moves by hand: the day it turns full, dark or vanished, and what players see",
	"generator":      "run Chronicle's own generators (weather for a range of days, festivals and other events, sky events, names into a rolling table) instead of inventing the result",
}

// Capabilities is the prompt's opening list of what a block can change,
// one line per wired kind.
func (r *Registry) Capabilities() string {
	var b strings.Builder
	for _, k := range r.Kinds() {
		brief := briefs[k.Name()]
		if bk, ok := k.(aiplug.Briefed); ok {
			brief = bk.Brief()
		}
		if brief == "" {
			brief = k.Label()
		}
		b.WriteString("- `kind: " + k.Name() + "`: " + brief + "\n")
	}
	return b.String()
}

// CalendarKinds are the kinds the prompt's Calendar content choice
// explains: the calendar itself and everything written onto it.
var CalendarKinds = []string{KindCalendar, "event", "weather", "generator"}

// DocsFor is Docs for the named kinds only, in that order; a name not
// wired on this server is skipped.
func (r *Registry) DocsFor(names ...string) string {
	var b strings.Builder
	for _, n := range names {
		k, ok := r.Get(n)
		if !ok {
			continue
		}
		b.WriteString("### kind: " + k.Name() + " (" + k.Label() + ")\n\n")
		b.WriteString(strings.TrimSpace(k.Doc()))
		b.WriteString("\n\n")
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

// badRequestf is a refusal worded for the operator.
func badRequestf(format string, args ...any) error { return aiplug.BadRequestf(format, args...) }

// sameName compares names the way people read them.
func sameName(a, b string) bool { return aiplug.SameName(a, b) }

// quote renders a YAML-safe scalar for exports.
func quote(s string) string { return aiplug.Quote(s) }

// planError is the text a review row shows for err.
func planError(err error) string { return aiplug.PlanError(err) }
