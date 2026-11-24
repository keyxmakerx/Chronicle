package cutscenes

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// AI is the cut scenes' part of the AI engine (aiplug, ADR-071): the
// `cut-scene` block an AI can ask to write and the `cut-scenes` lookup. Every
// read and write goes through Service, so the Director-only rule and the one
// check of a scene (Normalize) are the service's own; this file words them for
// the review screen. Showing a scene is never something the AI can ask for:
// it is a live act of the Director's, on the Cut scenes page.
type AI struct {
	kind *sceneKind
	look *scenesLookup
}

// NewAI builds the AI part. on reports whether the campaign has the Cut
// scenes add-on switched on: the service does not ask, because its routes are
// gated where they are mounted.
func NewAI(svc Service, on func(ctx context.Context, campaignID string) bool) *AI {
	c := &aiCore{svc: svc, on: on}
	return &AI{kind: &sceneKind{c}, look: &scenesLookup{c}}
}

// Kinds is the one block kind.
func (a *AI) Kinds() []aiplug.Kind { return []aiplug.Kind{a.kind} }

// Lookup is the `cut-scenes` lookup, also a tool on a connected AI app.
func (a *AI) Lookup() aiplug.Lookup { return a.look }

var (
	_ aiplug.Kind    = (*sceneKind)(nil)
	_ aiplug.Briefed = (*sceneKind)(nil)
	_ aiplug.Lookup  = (*scenesLookup)(nil)
)

type aiCore struct {
	svc Service
	on  func(ctx context.Context, campaignID string) bool
}

// KindName is the block's front-matter kind.
const KindName = "cut-scene"

// directorViewer is the actor as the service sees a Director. Plan and Apply
// reach it only after CanAuthorDmOnly, so the role is never claimed for
// someone who is not one.
func directorViewer(a aiplug.Actor) Viewer { return Viewer{UserID: a.UserID, Director: true} }

func (c *aiCore) gate(ctx context.Context, campaignID string, a aiplug.Actor) string {
	if !c.on(ctx, campaignID) {
		return "Cut scenes are switched off in this campaign"
	}
	if !a.CanAuthorDmOnly() {
		return "only the Director changes cut scenes"
	}
	return ""
}

// sceneNamed is the campaign's scene of that name, nil when there is none.
// Names are unique per campaign (case aside, as people read them).
func (c *aiCore) sceneNamed(ctx context.Context, campaignID string, v Viewer, name string) (*Scene, error) {
	list, err := c.svc.List(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	var hits []Scene
	for _, s := range list {
		if aiplug.SameName(s.Name, name) {
			hits = append(hits, s)
		}
	}
	switch len(hits) {
	case 0:
		return nil, nil
	case 1:
		return &hits[0], nil
	}
	return nil, aiplug.BadRequestf("more than one scene is called %s", aiplug.Quote(name))
}

// ---- keys ----------------------------------------------------------------

// aiKey makes a key comparable: case, underscores, hyphens and spaces do not
// matter, so `on_top`, `onTop` and `On top` are one key.
func aiKey(k string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(k) {
		if r == '_' || r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// aiAliases fold the editor's section names onto the stored layer names.
var aiAliases = map[string]string{"ontop": "overlays", "overlay": "overlays", "words": "text", "lines": "text", "end": "ending"}

var aiEngineKeys = map[string]bool{"kind": true, "action": true, "name": true, "reason": true}

var sceneOwn = map[string]bool{"renameto": true, "background": true, "overlays": true, "text": true, "ending": true}

const whyShow = "showing a scene to people is a live act of the Director's, from the Cut scenes page; the AI never shows one"

var aiRefused = map[string]string{
	"show": whyShow, "showto": whyShow, "to": whyShow, "viewers": whyShow, "audience": whyShow, "players": whyShow,
	"rewatch": whyShow, "watchagain": whyShow, "play": whyShow, "showing": whyShow, "push": whyShow, "send": whyShow,
}

// aiNormalise returns the record with its keys normalised and aliases folded,
// refusing a key the AI may not set and a key this kind does not take, by
// name: a quietly ignored key could be something the AI believes it set.
func aiNormalise(r aiplug.Record) (aiplug.Record, error) {
	raw := make([]string, 0, len(r.Fields))
	for k := range r.Fields {
		raw = append(raw, k)
	}
	sort.Strings(raw)
	for _, k := range raw {
		if why := aiRefused[aiKey(k)]; why != "" {
			return r, aiplug.BadRequestf("`%s` is not something the AI sets: %s", k, why)
		}
	}
	f := make(map[string]any, len(raw))
	seen := map[string]string{}
	for _, k := range raw {
		n := aiKey(k)
		if a, ok := aiAliases[n]; ok {
			n = a
		}
		if !sceneOwn[n] && !aiEngineKeys[n] {
			return r, aiplug.BadRequestf("`%s` is not a key a cut-scene block takes", k)
		}
		if prev, dup := seen[n]; dup {
			return r, aiplug.BadRequestf("`%s` and `%s` are the same key; write one", prev, k)
		}
		seen[n] = k
		f[n] = r.Fields[k]
	}
	r.Fields = f
	return r, nil
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

// strict decodes v into dst, refusing a key dst does not have by name.
func strict(v any, dst any, what, shape string) error {
	raw, err := json.Marshal(jsonable(v))
	if err != nil {
		return aiplug.BadRequestf("%s can't be read; write it as %s", what, shape)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if rest, found := strings.CutPrefix(err.Error(), "json: unknown field "); found {
			return aiplug.BadRequestf("%s has no key %s; write it as %s", what, rest, shape)
		}
		return aiplug.BadRequestf("%s must be written as %s", what, shape)
	}
	return nil
}

const (
	bgShape     = "{kind, seconds, look}"
	ovShape     = "a list of {kind, at, seconds, color, look, pupil}"
	textShape   = "a list of {say, at, seconds, style}"
	endingShape = "{kind: tap} or {kind: auto, after}"
)

type aiBackground struct {
	Kind    string   `json:"kind"`
	Seconds *float64 `json:"seconds"`
	Look    string   `json:"look"`
	Params  *Params  `json:"params"`
}

type aiOverlay struct {
	Kind    string   `json:"kind"`
	At      *float64 `json:"at"`
	Seconds *float64 `json:"seconds"`
	Color   string   `json:"color"`
	Colour  string   `json:"colour"`
	Look    string   `json:"look"`
	Pupil   string   `json:"pupil"`
	Params  *Params  `json:"params"`
}

// aiPupil checks a pupil an AI wrote, refusing an unknown one or one on
// anything but an eye by name, as aiLook does for looks.
func aiPupil(kind, pupil, where string) (string, error) {
	pupil = strings.ToLower(strings.TrimSpace(pupil))
	if pupil == "" {
		return "", nil
	}
	if kind != OverlayEye {
		return "", aiplug.BadRequestf("%s: %q has no pupil; only %s does", where, kind, OverlayEye)
	}
	if _, ok := pupilOf(pupil); !ok {
		return "", aiplug.BadRequestf("%s: %q is not a pupil; it is one of %s", where, pupil, keys(Pupils))
	}
	return pupil, nil
}

// aiLook checks a look an AI wrote. The server drops an unknown look and
// plays the default, which a review would never show, so the AI is refused
// by name instead and told the looks there are.
func aiLook(kind, look, where string) (string, error) {
	look = strings.ToLower(strings.TrimSpace(look))
	if look == "" {
		return "", nil
	}
	list := Looks[kind]
	if len(list) == 0 {
		return "", aiplug.BadRequestf("%s: %q has no looks to pick; only %s", where, kind, lookKinds())
	}
	if _, ok := lookOf(kind, look); !ok {
		ids := make([]string, len(list))
		for i, l := range list {
			ids[i] = l.Key
		}
		return "", aiplug.BadRequestf("%s: %q is not a look of %s; it is one of %s", where, look, kind, strings.Join(ids, ", "))
	}
	return look, nil
}

// lookKinds names the kinds that have looks, in the editor's order.
func lookKinds() string {
	var out []string
	for _, list := range [][]Kind{BackgroundKinds, OverlayKinds} {
		for _, k := range list {
			if len(Looks[k.Key]) > 0 {
				out = append(out, k.Key)
			}
		}
	}
	return strings.Join(out, " and ")
}

type aiText struct {
	Say     string   `json:"say"`
	At      *float64 `json:"at"`
	Seconds *float64 `json:"seconds"`
	Style   string   `json:"style"`
}

type aiEnding struct {
	Kind  string   `json:"kind"`
	After *float64 `json:"after"`
}

func or(p *float64, def float64) float64 {
	if p == nil {
		return def
	}
	return *p
}

// sceneEdit is what a block asks for: each layer nil when not written.
type sceneEdit struct {
	rename     *string
	background *Background
	overlays   *[]Overlay
	text       *[]TextLine
	ending     *Ending
}

// parse reads a block's layers into the scene's own types. It only reads;
// Normalize checks the result.
func parse(r aiplug.Record) (sceneEdit, error) {
	var e sceneEdit
	if v, ok := r.Fields["renameto"]; ok {
		s, isStr := v.(string)
		if !isStr || strings.TrimSpace(s) == "" {
			return e, aiplug.BadRequestf("rename_to needs the new name")
		}
		s = strings.TrimSpace(s)
		e.rename = &s
	}
	if v, ok := r.Fields["background"]; ok {
		var b aiBackground
		if s, isStr := v.(string); isStr {
			b.Kind = strings.TrimSpace(s)
		} else if err := strict(v, &b, "background", bgShape); err != nil {
			return e, err
		}
		lk := b.Look
		if lk == "" && b.Params != nil {
			lk = b.Params.Look
		}
		kind := b.Kind
		if kind == "" {
			kind = BackgroundFallAsleep
		}
		lk, err := aiLook(kind, lk, "background")
		if err != nil {
			return e, err
		}
		if b.Params != nil && b.Params.Pupil != "" {
			return e, aiplug.BadRequestf("background: only an eye on top has a pupil")
		}
		bg := Background{Kind: b.Kind, Seconds: or(b.Seconds, DefaultSeconds), Params: Params{Look: lk}}
		e.background = &bg
	}
	if v, ok := r.Fields["overlays"]; ok {
		var list []aiOverlay
		if v != nil {
			if err := strict(v, &list, "overlays", ovShape); err != nil {
				return e, err
			}
		}
		out := make([]Overlay, 0, len(list))
		for i, o := range list {
			lk := o.Look
			if lk == "" && o.Params != nil {
				lk = o.Params.Look
			}
			pu := o.Pupil
			if pu == "" && o.Params != nil {
				pu = o.Params.Pupil
			}
			if known(OverlayKinds, o.Kind) {
				var err error
				where := fmt.Sprintf("layer on top %d", i+1)
				if lk, err = aiLook(o.Kind, lk, where); err != nil {
					return e, err
				}
				if pu, err = aiPupil(o.Kind, pu, where); err != nil {
					return e, err
				}
			}
			c := o.Color
			if c == "" {
				c = o.Colour
			}
			if c == "" && o.Params != nil {
				c = o.Params.Color
			}
			out = append(out, Overlay{Kind: o.Kind, At: or(o.At, 0), Seconds: or(o.Seconds, DefaultSeconds), Params: Params{Color: c, Look: lk, Pupil: pu}})
		}
		e.overlays = &out
	}
	if v, ok := r.Fields["text"]; ok {
		var list []aiText
		if v != nil {
			if err := strict(v, &list, "text", textShape); err != nil {
				return e, err
			}
		}
		out := make([]TextLine, 0, len(list))
		for _, t := range list {
			out = append(out, TextLine{Say: t.Say, At: or(t.At, 0), Seconds: or(t.Seconds, DefaultSeconds), Style: t.Style})
		}
		e.text = &out
	}
	if v, ok := r.Fields["ending"]; ok {
		var en aiEnding
		if s, isStr := v.(string); isStr {
			en.Kind = strings.TrimSpace(s)
		} else if err := strict(v, &en, "ending", endingShape); err != nil {
			return e, err
		}
		end := Ending(en)
		e.ending = &end
	}
	return e, nil
}

// ---- kind: cut-scene ------------------------------------------------------

type sceneKind struct{ *aiCore }

func (*sceneKind) Name() string  { return KindName }
func (*sceneKind) Label() string { return "Cut scene" }
func (*sceneKind) Brief() string {
	return "cut scenes: make, change or delete a full-screen moment (a background, animations on top, words, an ending); showing one to players stays with the Director"
}

type sceneWork struct {
	plan   aiplug.Plan
	id     string
	create *Scene
	save   *Scene
	remove bool
}

func refuse(msg string) sceneWork { return sceneWork{plan: aiplug.Plan{Error: msg}} }

func (k *sceneKind) work(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) sceneWork {
	if msg := k.gate(ctx, campaignID, a); msg != "" {
		return refuse(msg)
	}
	if r.Name == "" {
		return refuse("a cut scene needs a name")
	}
	v := directorViewer(a)
	if r.Action == aiplug.ActionDelete {
		return k.workDelete(ctx, campaignID, v, r)
	}
	rec, err := aiNormalise(r)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	e, err := parse(rec)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	hit, err := k.sceneNamed(ctx, campaignID, v, r.Name)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	if r.Action == aiplug.ActionUpdate {
		return k.workUpdate(ctx, campaignID, v, e, hit, r)
	}
	return k.workCreate(e, hit, r)
}

func (k *sceneKind) workCreate(e sceneEdit, hit *Scene, r aiplug.Record) sceneWork {
	if hit != nil {
		return refuse("a scene called " + aiplug.Quote(r.Name) + " already exists; use action: update")
	}
	if e.rename != nil {
		return refuse("rename_to only changes a scene that exists; use action: update")
	}
	raw := Scene{Name: r.Name, Ending: Ending{Kind: EndingTap}}
	if e.background != nil {
		raw.Background = *e.background
	}
	if e.overlays != nil {
		raw.Overlays = *e.overlays
	}
	if e.text != nil {
		raw.Text = *e.text
	}
	if e.ending != nil {
		raw.Ending = *e.ending
	}
	sc, err := Normalize(raw)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	parts := []string{"Background: " + BackgroundWords(sc.Background)}
	parts = append(parts, layerLines(sc)...)
	parts = append(parts, "Ending: "+EndingWords(sc.Ending))
	return sceneWork{plan: aiplug.Plan{
		Summary:  "makes the cut scene " + aiplug.Quote(sc.Name) + ": " + strings.Join(parts, "; "),
		Warnings: clampNotes(raw, sc),
	}, create: &sc}
}

// layerLines says the two list layers, or that they are empty.
func layerLines(sc Scene) []string {
	var out []string
	if len(sc.Overlays) == 0 {
		out = append(out, "On top: nothing")
	}
	for _, o := range sc.Overlays {
		out = append(out, "On top: "+OverlayWords(o))
	}
	if len(sc.Text) == 0 {
		out = append(out, "Words: none")
	}
	for _, t := range sc.Text {
		out = append(out, "Words: "+TextWords(t))
	}
	return out
}

func (k *sceneKind) workUpdate(ctx context.Context, campaignID string, v Viewer, e sceneEdit, hit *Scene, r aiplug.Record) sceneWork {
	if hit == nil {
		return refuse("no cut scene called " + aiplug.Quote(r.Name))
	}
	raw := *hit
	var parts []string
	if e.rename != nil {
		if other, err := k.sceneNamed(ctx, campaignID, v, *e.rename); err != nil {
			return refuse(aiplug.PlanError(err))
		} else if other != nil && other.ID != hit.ID {
			return refuse("a scene called " + aiplug.Quote(*e.rename) + " already exists")
		}
		raw.Name = *e.rename
	}
	if e.background != nil {
		raw.Background = *e.background
	}
	if e.overlays != nil {
		raw.Overlays = *e.overlays
	}
	if e.text != nil {
		raw.Text = *e.text
	}
	if e.ending != nil {
		raw.Ending = *e.ending
	}
	sc, err := Normalize(raw)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	sc.ID = hit.ID
	if e.rename != nil {
		parts = append(parts, "renames it to "+aiplug.Quote(sc.Name))
	}
	if e.background != nil {
		parts = append(parts, "Background: "+BackgroundWords(sc.Background))
	}
	if e.overlays != nil {
		parts = append(parts, onlyPrefix(layerLines(Scene{Overlays: sc.Overlays}), "On top")...)
	}
	if e.text != nil {
		parts = append(parts, onlyPrefix(layerLines(Scene{Text: sc.Text}), "Words")...)
	}
	if e.ending != nil {
		parts = append(parts, "Ending: "+EndingWords(sc.Ending))
	}
	if len(parts) == 0 {
		return refuse("nothing to change: write a layer (background, overlays, text, ending) or rename_to")
	}
	var warns []string
	if e.overlays != nil || e.text != nil {
		warns = append(warns, "A list written here replaces the scene's whole list of that layer.")
	}
	warns = append(warns, clampNotes(raw, sc)...)
	return sceneWork{plan: aiplug.Plan{
		Summary:  "changes the cut scene " + aiplug.Quote(hit.Name) + ": " + strings.Join(parts, "; "),
		Warnings: warns,
	}, id: hit.ID, save: &sc}
}

func onlyPrefix(lines []string, prefix string) []string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return out
}

// workDelete plans removing a scene. A delete takes the name and nothing
// else, so a pasted row cannot carry a change along with the removal.
func (k *sceneKind) workDelete(ctx context.Context, campaignID string, v Viewer, r aiplug.Record) sceneWork {
	keys := make([]string, 0, len(r.Fields))
	for key := range r.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !aiEngineKeys[aiKey(key)] {
			return refuse("a delete takes a scene's name and nothing else; remove `" + key + "` from the block")
		}
	}
	hit, err := k.sceneNamed(ctx, campaignID, v, r.Name)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	if hit == nil {
		return refuse("no cut scene called " + aiplug.Quote(r.Name))
	}
	return sceneWork{plan: aiplug.Plan{
		Summary: "deletes the cut scene " + aiplug.Quote(hit.Name) + " for good",
		Warnings: []string{
			"People it was already shown to can still watch it again until their time runs out.",
			"This cannot be undone.",
		},
		Seen: aiplug.Hash(*hit),
	}, id: hit.ID, remove: true}
}

// clampNotes says which numbers Normalize brought into range, so the review
// shows a figure that changed rather than storing it silently.
func clampNotes(raw, sc Scene) []string {
	var out []string
	note := func(what string, was, is float64) {
		if was != is {
			out = append(out, fmt.Sprintf("%s was %s; it is kept as %s, the nearest allowed.", what, Secs(was), Secs(is)))
		}
	}
	if raw.Background.Seconds != 0 && sc.Background.Kind != BackgroundBlack {
		note("The background's time", raw.Background.Seconds, sc.Background.Seconds)
	}
	for i := range sc.Overlays {
		if i < len(raw.Overlays) {
			note(fmt.Sprintf("Layer on top %d's start", i+1), raw.Overlays[i].At, sc.Overlays[i].At)
			note(fmt.Sprintf("Layer on top %d's time", i+1), raw.Overlays[i].Seconds, sc.Overlays[i].Seconds)
		}
	}
	for i := range sc.Text {
		if i < len(raw.Text) {
			note(fmt.Sprintf("Words %d's start", i+1), raw.Text[i].At, sc.Text[i].At)
			if raw.Text[i].Seconds > 0 {
				note(fmt.Sprintf("Words %d's time on screen", i+1), raw.Text[i].Seconds, sc.Text[i].Seconds)
			}
		}
	}
	return out
}

func (k *sceneKind) Plan(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) aiplug.Plan {
	return k.work(ctx, campaignID, a, r).plan
}

// Apply re-plans, because the campaign may have changed since review, then
// writes through the service as a Director.
func (k *sceneKind) Apply(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) error {
	w := k.work(ctx, campaignID, a, r)
	if w.plan.Error != "" {
		return apperror.NewBadRequest(w.plan.Error)
	}
	v := directorViewer(a)
	switch {
	case w.remove:
		// The owner said yes to this scene as it was reviewed; one edited
		// since is a different removal. An empty Seen fails closed.
		if r.Seen == "" || r.Seen != w.plan.Seen {
			return apperror.NewBadRequest("this scene changed since its removal was reviewed; nothing was removed. Ask for the change again to review it as it is now.")
		}
		return k.svc.Delete(ctx, campaignID, w.id, v)
	case w.create != nil:
		_, err := k.svc.Create(ctx, campaignID, v, *w.create)
		return err
	}
	_, err := k.svc.Save(ctx, campaignID, w.id, v, *w.save)
	return err
}

// Export is empty: scenes are read with `what: cut-scenes`, which gives each
// one in this block's format.
func (*sceneKind) Export(context.Context, string, aiplug.Actor) (string, error) { return "", nil }

func (*sceneKind) Doc() string {
	return fmt.Sprintf("A cut scene: a short full-screen moment the Director shows chosen players, matched by `name`. It has three separate layers and an ending; times are seconds from the start. "+
		"`background` is `{kind, seconds, look}`: kind one of %s, seconds %s to %s (how long it takes to reach full dark; black ignores it), `look` only for fall-asleep: %s. "+
		"`overlays` (the layers drawn on top, in order, at most %d) is a list of `{kind, at, seconds, color, look, pupil}`: kind one of %s, `at` 0 to %s (when it starts), `seconds` %s to %s (how long it builds up), `color` written #rrggbb (eye default %s), `look` only for eye: %s, `pupil` only for eye: %s. "+
		"`text` (the words, at most %d lines) is a list of `{say, at, seconds, style}`: `say` plain text of 1 to %d characters (never HTML or markdown), `at` 0 to %s, `seconds` how long it stays on screen (%s to %s, or 0 to stay to the end), style one of %s. "+
		"`ending` is `{kind: tap}` (a quiet Close appears once the last layer has finished) or `{kind: auto, after: 3}` (closes by itself that many seconds after the last layer, 0 to %s). "+
		"Leaving `look` out plays the first look named; leaving `pupil` out draws the look's own (round for rift, slit for almond and gather), and `pupil: slit` makes any eye a dragon's. Numbers outside their range are brought to the nearest allowed value and the review says so; an unknown kind, style, look or pupil, a look on a kind that has none, a pupil on anything but an eye, or a colour not written #rrggbb is refused. "+
		"On update only the layers written change, and a list (`overlays`, `text`) written replaces that whole list; `rename_to` renames. `action: delete` removes the scene named by `name` and takes no other key. "+
		"The AI never shows a scene to anyone: who sees it and when is the Director's live choice on the Cut scenes page, so a block naming players, `show`, `to` or a rewatch time is refused. "+
		"Read the scenes and what each kind looks like with `what: cut-scenes`.\n\n```\n---\nkind: cut-scene\nname: The Eye Opens\nbackground: {kind: fall-asleep, seconds: 7, look: ink}\noverlays:\n  - {kind: eye, at: 7.5, seconds: 3.5, color: \"#3dff7a\", look: rift, pupil: slit}\ntext:\n  - {say: \"Something has noticed you.\", at: 11, seconds: 5, style: whisper}\nending: {kind: tap}\n---\n```",
		keys(BackgroundKinds), num(MinSeconds), num(MaxSeconds), lookList(BackgroundFallAsleep),
		MaxOverlays, keys(OverlayKinds), num(MaxAt), num(MinSeconds), num(MaxSeconds), DefaultEyeColor, lookList(OverlayEye), pupilList(),
		MaxTextLines, MaxSayChars, num(MaxAt), num(MinSeconds), num(MaxSeconds), keys(TextStyles),
		num(MaxAfter))
}

// pupilList names the eye's pupils for the block's description: "`round`
// (Round), …".
func pupilList() string {
	parts := make([]string, 0, len(Pupils))
	for _, p := range Pupils {
		parts = append(parts, fmt.Sprintf("`%s` (%s)", p.Key, p.Name))
	}
	return strings.Join(parts, ", ")
}

// lookList names a kind's looks for the block's description: "`ink` (Ink
// in water), …".
func lookList(kind string) string {
	parts := make([]string, 0, len(Looks[kind]))
	for _, l := range Looks[kind] {
		parts = append(parts, fmt.Sprintf("`%s` (%s)", l.Key, l.Name))
	}
	return strings.Join(parts, ", ")
}
