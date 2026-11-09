package transformations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// AILayers is the transformations' part of the AI engine (aiplug, ADR-071):
// the `layer` block an AI can ask to put on or take off a hero, and the
// `layers` lookup that says what is changing a hero now. Both go through the
// Service, so the Director-only and page-edit rules are the same as on the
// sheet, and both answer nothing while the campaign has the add-on off.
type AILayers struct {
	svc   Service
	pages PageFinder
	on    func(ctx context.Context, campaignID string) bool
}

// PageFinder finds a page by the name an AI wrote. Implemented in
// internal/app over the entities service; the Service still decides whether
// the asker may see or edit it.
type PageFinder interface {
	// FindPage is the page's id and stored name, ok false when there is none.
	FindPage(ctx context.Context, campaignID, name string) (id, stored string, ok bool, err error)
}

// NewAILayers builds the AI part. on reports whether the campaign has the
// add-on switched on.
func NewAILayers(svc Service, pages PageFinder, on func(ctx context.Context, campaignID string) bool) *AILayers {
	return &AILayers{svc: svc, pages: pages, on: on}
}

var (
	_ aiplug.Kind    = (*AILayers)(nil)
	_ aiplug.Briefed = (*AILayers)(nil)
	_ aiplug.Lookup  = (*AILayers)(nil)
)

func viewerOf(a aiplug.Actor) Viewer { return Viewer{UserID: a.UserID, VisibilityRole: a.Role} }

func (*AILayers) Name() string  { return "layer" }
func (*AILayers) Label() string { return "Transformation" }
func (*AILayers) Brief() string {
	return "transformations on a hero (a vampire's bite, a curse, rising as a revenant): layers that change sheet fields without touching the hero's own"
}
func (*AILayers) Doc() string {
	return "A transformation on a hero's page: a layer that changes sheet fields while the hero's own fields stay as they are, so taking it off puts the hero back. Matched by `name` on the `page`. Keys: `page` (the hero's page name), `source` (the game system's preset it follows, like `drawsteel:revenant`; leave it out for one made by hand), `note`, and `changes`, a list of `{field, op, value, label}`: `field` is the sheet field's key as `how_to_write_changes` lists it for the category, `op` is `set` (replace the value), `add` (add a number), `append` (add an entry to a list field) or `remove` (hide the list entry with that name), and `label` names the change when the field alone says too little. Up to 40 changes; choices a preset asks for are settled in the changes you write. `action: delete` takes the layer off. A layer is never changed in place: take it off and put a new one on. `what: layers` shows a page's layers. Only the Director puts layers on, and only with Transformations switched on.\n\n```\n---\nkind: layer\nname: Revenant\npage: Mira Holt\nsource: drawsteel:revenant\nchanges:\n  - {field: ancestry, op: set, value: Revenant}\n  - {field: recoveries_max, op: add, value: -2}\n  - {field: features_json, op: append, value: {name: Tough But Withered}, label: Tough But Withered}\n---\n```"
}

// target is the page a record names, as the asker may see it, and its
// layers.
func (k *AILayers) target(ctx context.Context, campaignID string, a aiplug.Actor, page string) (id, name string, layers *PageLayers, err error) {
	if strings.TrimSpace(page) == "" {
		return "", "", nil, aiplug.BadRequestf("needs `page`, the hero's page name")
	}
	id, name, ok, err := k.pages.FindPage(ctx, campaignID, page)
	if err != nil {
		return "", "", nil, err
	}
	if !ok {
		return "", "", nil, aiplug.BadRequestf("no page called %q", page)
	}
	layers, err = k.svc.ListForPage(ctx, campaignID, id, viewerOf(a))
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return "", "", nil, aiplug.BadRequestf("no page called %q", page)
		}
		return "", "", nil, err
	}
	return id, name, layers, nil
}

// layerNamed is the page's layer of that name, or nil.
func layerNamed(p *PageLayers, name string) *Layer {
	for i := range p.Layers {
		if aiplug.SameName(p.Layers[i].Name, name) {
			return &p.Layers[i]
		}
	}
	return nil
}

// input reads a record as the layer the Service would store, checked.
func input(r aiplug.Record) (AddInput, error) {
	in := AddInput{Name: r.Name, Source: r.Str("source"), Note: r.Str("note")}
	for i, raw := range r.List("changes") {
		m, ok := raw.(map[string]any)
		if !ok {
			return in, aiplug.BadRequestf("changes: entry %d is not {field, op, value}", i+1)
		}
		c := Change{Field: fmt.Sprint(m["field"]), Op: fmt.Sprint(m["op"])}
		if l, ok := m["label"]; ok && l != nil {
			c.Label = fmt.Sprint(l)
		}
		v, err := json.Marshal(jsonable(m["value"]))
		if err != nil {
			return in, aiplug.BadRequestf("changes: entry %d's value can't be stored", i+1)
		}
		c.Value = v
		in.Changes = append(in.Changes, c)
	}
	out, err := normalise(in)
	if err != nil {
		return in, aiplug.BadRequestf("%s", apperror.SafeMessage(err))
	}
	return out, nil
}

// jsonable turns YAML's map[any]any, which encoding/json refuses, into
// map[string]any, all the way down.
func jsonable(v any) any {
	switch t := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[fmt.Sprint(k)] = jsonable(x)
		}
		return m
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = jsonable(x)
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = jsonable(x)
		}
		return out
	}
	return v
}

func (k *AILayers) Plan(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) aiplug.Plan {
	if !k.on(ctx, campaignID) {
		return aiplug.Plan{Error: "Transformations are switched off in this campaign"}
	}
	if !a.CanAuthorDmOnly() {
		return aiplug.Plan{Error: "only the Director changes what is transforming a hero"}
	}
	_, page, layers, err := k.target(ctx, campaignID, a, r.Str("page"))
	if err != nil {
		return aiplug.Plan{Error: apperror.SafeMessage(err)}
	}
	if !layers.CanEdit {
		return aiplug.Plan{Error: "you can't edit " + page}
	}
	have := layerNamed(layers, r.Name)
	switch r.Action {
	case aiplug.ActionDelete:
		if have == nil {
			return aiplug.Plan{Error: fmt.Sprintf("%s has no layer called %q", page, r.Name)}
		}
		return aiplug.Plan{Summary: fmt.Sprintf("takes %s off %s; the hero's own fields show again", have.Name, page)}
	case aiplug.ActionUpdate:
		return aiplug.Plan{Error: "a layer isn't changed in place: take it off with action: delete and put a new one on"}
	}
	in, err := input(r)
	if err != nil {
		return aiplug.Plan{Error: apperror.SafeMessage(err)}
	}
	if len(layers.Layers) >= MaxLayersPerPage {
		return aiplug.Plan{Error: fmt.Sprintf("%s already has %d layers; take one off first", page, MaxLayersPerPage)}
	}
	p := aiplug.Plan{Summary: fmt.Sprintf("puts %s on %s: %s", in.Name, page, changeWords(in.Changes))}
	if have != nil {
		p.Warnings = append(p.Warnings, page+" already has a layer called "+have.Name+"; this adds a second")
	}
	return p
}

func (k *AILayers) Apply(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	id, _, layers, err := k.target(ctx, campaignID, a, r.Str("page"))
	if err != nil {
		return err
	}
	if r.Action == aiplug.ActionDelete {
		have := layerNamed(layers, r.Name)
		if have == nil {
			return apperror.NewBadRequest("it changed while you were reviewing; check it again")
		}
		return k.svc.Remove(ctx, campaignID, have.ID, viewerOf(a))
	}
	in, err := input(r)
	if err != nil {
		return err
	}
	_, err = k.svc.Add(ctx, campaignID, id, viewerOf(a), in)
	return err
}

// Export is empty: layers are read a page at a time with `what: layers`.
func (*AILayers) Export(context.Context, string, aiplug.Actor) (string, error) { return "", nil }

func (*AILayers) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "layers", Aliases: []string{"transformations", "layer"}, Keys: "`page`",
		Brief: "what is transforming a hero now: each layer and the sheet fields it changes",
		Tool: aiplug.Tool{
			Name: "transformations", Title: "Transformations", Did: "Looked at what is transforming a hero",
			Desc: "What is transforming a hero now (a vampire's bite, a curse, rising as a revenant): each layer on the page, oldest first, and the sheet fields it changes. The sheet and read_page show the hero's own fields; these change them on top. Put one on or take one off with a layer block in ask_for_changes.",
			Args: []aiplug.Arg{{Key: "page", Desc: "The hero's page name", Required: true}},
		},
	}
}

func (k *AILayers) Answer(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) (string, string, error) {
	page := r.Str("page")
	if page == "" {
		page = r.Name
	}
	chip := "Transformations · " + page
	if !k.on(ctx, campaignID) {
		return chip, "", aiplug.BadRequestf("Transformations are switched off in this campaign")
	}
	_, name, layers, err := k.target(ctx, campaignID, a, page)
	if err != nil {
		return chip, "", err
	}
	chip = fmt.Sprintf("Transformations · %s · %d", name, len(layers.Layers))
	var b strings.Builder
	fmt.Fprintf(&b, "## What is transforming %s (%d)\n\n", name, len(layers.Layers))
	if len(layers.Layers) == 0 {
		b.WriteString("Nothing: the sheet shows the hero as they are.\n")
		return chip, b.String(), nil
	}
	b.WriteString("Layers apply oldest first, on top of the hero's own fields.\n\n")
	var lines []string
	for _, l := range layers.Layers {
		line := "- " + l.Name
		if l.Source != "" {
			line += " (" + l.Source + ")"
		}
		line += ": " + changeWords(l.Changes)
		if n := strings.TrimSpace(l.Note); n != "" {
			line += ". " + aiplug.OneLine(n, 200)
		}
		lines = append(lines, line)
	}
	aiplug.WriteLines(&b, lines)
	return chip, b.String(), nil
}

// changeWords says a layer's changes in a line: "ancestry set to Revenant,
// recoveries_max add -2, features_json append Tough But Withered".
func changeWords(cs []Change) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		v := c.Label
		if v == "" {
			v = aiplug.OneLine(strings.Trim(string(c.Value), `"`), 80)
		}
		switch c.Op {
		case OpSet:
			parts = append(parts, c.Field+" set to "+v)
		default:
			parts = append(parts, c.Field+" "+c.Op+" "+v)
		}
	}
	return strings.Join(parts, ", ")
}
