package decks

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// AI is the card decks' part of the AI engine (aiplug, ADR-071): the `deck`
// and `card` blocks an AI can ask to write and the `decks` lookup. Every read
// and write goes through Service, so the Director-only rules, the sight rules
// the screens already obey and the one check of a card's tests are the
// service's own; this file words them for a review screen and decides nothing
// a screen would not.
type AI struct {
	deck *deckKind
	card *cardKind
	look *decksLookup
}

// PageFinder finds a page by the name an AI wrote. It is implemented in
// internal/app over the entities service, so decks imports no other plugin.
type PageFinder interface {
	// FindPage is the page's id and stored name, ok false when there is none.
	FindPage(ctx context.Context, campaignID, name string) (id, stored string, ok bool, err error)
}

// NewAI builds the AI part. on reports whether the campaign has the Card
// decks add-on switched on: the service does not ask, because its routes are
// gated where they are mounted.
func NewAI(svc Service, pages PageFinder, on func(ctx context.Context, campaignID string) bool) *AI {
	c := &aiCore{svc: svc, pages: pages, on: on}
	return &AI{deck: &deckKind{c}, card: &cardKind{c}, look: &decksLookup{c}}
}

// Kinds are the blocks, deck first so a card row of the same paste can name it.
func (a *AI) Kinds() []aiplug.Kind { return []aiplug.Kind{a.deck, a.card} }

// Lookup is the `decks` lookup, also a tool on a connected AI app.
func (a *AI) Lookup() aiplug.Lookup { return a.look }

var (
	_ aiplug.Kind    = (*deckKind)(nil)
	_ aiplug.Briefed = (*deckKind)(nil)
	_ aiplug.Staged  = (*deckKind)(nil)
	_ aiplug.Kind    = (*cardKind)(nil)
	_ aiplug.Briefed = (*cardKind)(nil)
	_ aiplug.Lookup  = (*decksLookup)(nil)
)

// aiCore is what the two kinds and the lookup share.
type aiCore struct {
	svc   Service
	pages PageFinder
	on    func(ctx context.Context, campaignID string) bool
}

// directorViewer is the actor as the service sees a Director. Plan and Apply
// reach it only after CanAuthorDmOnly, so the role is never claimed for
// someone who is not one.
func directorViewer(a aiplug.Actor) Viewer {
	return Viewer{UserID: a.UserID, MemberRole: permissions.RoleOwner, VisibilityRole: a.Role, Director: true}
}

// gate is the refusal that comes before anything is read: the add-on is off,
// or the key is not a Director's. "" lets the row go on.
func (c *aiCore) gate(ctx context.Context, campaignID string, a aiplug.Actor) string {
	if !c.on(ctx, campaignID) {
		return "Card decks are switched off in this campaign"
	}
	if !a.CanAuthorDmOnly() {
		return "only the Director changes card decks"
	}
	return ""
}

// deckNamed is the campaign's deck of that name, nil when there is none. Two
// of one name is refused, so a change never lands on the wrong deck.
func (c *aiCore) deckNamed(ctx context.Context, campaignID string, v Viewer, name string) (*DirectorDeck, error) {
	view, err := c.svc.List(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	list, ok := view.(DirectorList)
	if !ok {
		return nil, aiplug.BadRequestf("the decks could not be read")
	}
	var hits []DirectorDeck
	for _, d := range list.Decks {
		if aiplug.SameName(d.Name, name) {
			hits = append(hits, d)
		}
	}
	switch len(hits) {
	case 0:
		return nil, nil
	case 1:
		return &hits[0], nil
	}
	return nil, aiplug.BadRequestf("more than one deck is called %s", aiplug.Quote(name))
}

// ---- keys ----------------------------------------------------------------

// aiKey makes a key comparable: case, underscores, hyphens and spaces do not
// matter, so `others_see`, `othersSee` and `OthersSee` are one key, and the
// workshop's own camelCase names work as well as the block's snake_case.
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

// aiAliases are spellings an AI is likely to use for a key.
var aiAliases = map[string]string{"backcolor": "backcolour"}

// aiEngineKeys are the front-matter keys the engine itself reads.
var aiEngineKeys = map[string]bool{"kind": true, "action": true, "name": true, "reason": true}

// What the AI never does, said once. A refused key is named in the plan, so
// the Director sees why the row stopped rather than a key quietly ignored.
const (
	whyDraws = "who may draw, the chosen players and their draws left are chosen in the deck workshop"
	whyNext  = "which card comes up next and which is kept for last are chosen in the deck workshop"
	whyMoves = "moving a card (draw, deal, play, shuffle, gather) is done at the table, never by the AI"
)

var aiRefused = map[string]string{
	"draws": whyDraws, "players": whyDraws, "chosenplayers": whyDraws, "drawsleft": whyDraws, "drawcount": whyDraws,
	"next": whyNext, "last": whyNext, "nextcard": whyNext, "lastcard": whyNext, "comesupnext": whyNext, "keptforlast": whyNext,
	"nextcardid": whyNext, "lastcardid": whyNext,
	"draw": whyMoves, "deal": whyMoves, "dealcard": whyMoves, "play": whyMoves, "shuffle": whyMoves, "gather": whyMoves,
	"putback": whyMoves, "takeback": whyMoves, "takeoff": whyMoves, "show": whyMoves, "approve": whyMoves, "another": whyMoves,
	"keep": whyMoves,
}

// aiNormalise returns the record with its keys normalised and aliases folded,
// refusing a key the AI may not set and a key this kind does not take. An
// unknown key is refused by name because a quietly ignored key could be a
// rule the AI believes it set.
func aiNormalise(r aiplug.Record, label string, own map[string]bool) (aiplug.Record, error) {
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
		if !own[n] && !aiEngineKeys[n] {
			return r, aiplug.BadRequestf("`%s` is not a key a %s block takes", k, label)
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

// aiText reads a text key; nil when absent. A key written with no value
// clears where clearing means something, and is refused where it does not.
func aiText(r aiplug.Record, key, show string, clearable bool) (*string, error) {
	v, ok := r.Fields[key]
	if !ok {
		return nil, nil
	}
	switch v.(type) {
	case nil:
		if !clearable {
			return nil, aiplug.BadRequestf("%s needs a value", show)
		}
		return ptr2(""), nil
	case map[string]any, map[any]any, []any:
		return nil, aiplug.BadRequestf("%s must be plain text", show)
	}
	s := r.Str(key)
	return &s, nil
}

// aiYesNo reads a yes or no; anything else is refused rather than read as no.
func aiYesNo(r aiplug.Record, key, show string) (*bool, error) {
	v, ok := r.Fields[key]
	if !ok {
		return nil, nil
	}
	switch t := v.(type) {
	case bool:
		return &t, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "yes", "on":
			return ptr2(true), nil
		case "false", "no", "off":
			return ptr2(false), nil
		}
	}
	return nil, aiplug.BadRequestf("%s must be yes or no", show)
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

// ---- staging: a card may name a deck an earlier row of the paste makes ------

// stagedDeck is what a deck row that planned cleanly will make, as much as a
// card row needs to be checked against it.
type stagedDeck struct {
	Name   string
	Kind   string
	Holder string
	Look   Look
}

type stagedDecksKey struct{}

func stagedName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// withStagedDeck returns a context that also knows the deck. The map is
// copied, so a context handed out earlier never changes.
func withStagedDeck(ctx context.Context, d stagedDeck) context.Context {
	m := map[string]stagedDeck{stagedName(d.Name): d}
	for k, v := range stagedDecks(ctx) {
		m[k] = v
	}
	return context.WithValue(ctx, stagedDecksKey{}, m)
}

func stagedDecks(ctx context.Context) map[string]stagedDeck {
	m, _ := ctx.Value(stagedDecksKey{}).(map[string]stagedDeck)
	return m
}

// ---- kind: deck -------------------------------------------------------------

type deckKind struct{ *aiCore }

func (*deckKind) Name() string  { return "deck" }
func (*deckKind) Label() string { return "Card deck" }
func (*deckKind) Brief() string {
	return "card decks: make a hero, party or event deck in one of the starting styles and change its rules and look; who draws, which card comes up next and moving cards stay with the Director at the table"
}

// aiPick is one rule or look key a deck block takes: the key as the block
// spells it and how it lands on the service's own input types.
type aiPick struct {
	key string
	set func(s *SettingsInput, l *LookInput, v string)
}

func lookBack(l *LookInput) *BackInput {
	if l.Back == nil {
		l.Back = &BackInput{}
	}
	return l.Back
}

// aiPicks are every rule and look key, in the order a summary lists them.
// Their values are checked by the service's own lists (applySettings and
// applyLook), never by a second list here.
var aiPicks = []aiPick{
	{"knows", func(s *SettingsInput, _ *LookInput, v string) { s.Knows = &v }},
	{"others_see", func(s *SettingsInput, _ *LookInput, v string) { s.OthersSee = &v }},
	{"arrives", func(s *SettingsInput, _ *LookInput, v string) { s.Arrives = &v }},
	{"fate", func(s *SettingsInput, _ *LookInput, v string) { s.Fate = &v }},
	{"empty", func(s *SettingsInput, _ *LookInput, v string) { s.Empty = &v }},
	{"notice", func(s *SettingsInput, _ *LookInput, v string) { s.Notice = &v }},
	{"shape", func(_ *SettingsInput, l *LookInput, v string) { l.Shape = &v }},
	{"border", func(_ *SettingsInput, l *LookInput, v string) { l.Border = &v }},
	{"turn", func(_ *SettingsInput, l *LookInput, v string) { l.Turn = &v }},
	{"window", func(_ *SettingsInput, l *LookInput, v string) { l.Window = &v }},
	// The look's `name` (where a card's name sits) would collide with the
	// block's own `name`, so it is `name_position`.
	{"name_position", func(_ *SettingsInput, l *LookInput, v string) { l.Name = &v }},
	{"lettering", func(_ *SettingsInput, l *LookInput, v string) { l.Lettering = &v }},
	{"corner", func(_ *SettingsInput, l *LookInput, v string) { l.Corner = &v }},
	{"marker", func(_ *SettingsInput, l *LookInput, v string) { l.Marker = &v }},
	{"finish", func(_ *SettingsInput, l *LookInput, v string) { l.Finish = &v }},
	{"back_pattern", func(_ *SettingsInput, l *LookInput, v string) { lookBack(l).Pattern = &v }},
	{"back_sign", func(_ *SettingsInput, l *LookInput, v string) { lookBack(l).Sign = &v }},
	{"back_colour", func(_ *SettingsInput, l *LookInput, v string) { lookBack(l).Colour = &v }},
	{"back_words", func(_ *SettingsInput, l *LookInput, v string) { lookBack(l).Words = &v }},
}

func isRule(key string) bool {
	switch key {
	case "knows", "others_see", "arrives", "fate", "empty", "notice", "discard":
		return true
	}
	return false
}

// deckOwn is the set of normalised keys a deck block takes.
var deckOwn = func() map[string]bool {
	m := map[string]bool{"for": true, "hero": true, "style": true, "renameto": true, "discard": true, "kinds": true}
	for _, p := range aiPicks {
		m[aiKey(p.key)] = true
	}
	return m
}()

// deckKinds maps what a block says a deck is for onto the stored kind.
var deckKinds = map[string]string{"hero": KindHero, "party": KindParty, "events": KindEvent, "event": KindEvent}

// deckEdit is what a deck block asks for, read and resolved.
type deckEdit struct {
	rename   *string
	kind     *string
	hero     *string // a page id; "" takes the deck off its hero
	heroName string
	settings *SettingsInput
	look     *LookInput
	style    *DeckStyle
	rules    []string // for the summary
	looks    []string
}

// styleNamed finds a starting style by key or by name.
func styleNamed(s string) (DeckStyle, bool) {
	want := strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(s, "-", " ")), " "))
	for _, st := range deckStyles {
		if strings.ToLower(strings.ReplaceAll(st.Key, "-", " ")) == want || strings.ToLower(st.Name) == want {
			return st, true
		}
	}
	return DeckStyle{}, false
}

func styleKeys() string {
	keys := make([]string, 0, len(deckStyles))
	for _, s := range deckStyles {
		keys = append(keys, s.Key)
	}
	return strings.Join(keys, ", ")
}

// kindDefs reads the `kinds` list strictly: a key a kind does not have is
// refused by name.
func kindDefs(v any) ([]KindDef, error) {
	items, ok := jsonable(v).([]any)
	if v != nil && !ok {
		return nil, aiplug.BadRequestf("kinds must be a list of {key, name, colour, sign}")
	}
	out := make([]KindDef, 0, len(items))
	for i, it := range items {
		raw, err := json.Marshal(it)
		if err != nil {
			return nil, aiplug.BadRequestf("kinds: entry %d can't be read", i+1)
		}
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		var kd KindDef
		if err := dec.Decode(&kd); err != nil {
			if rest, found := strings.CutPrefix(err.Error(), "json: unknown field "); found {
				return nil, aiplug.BadRequestf("kinds: entry %d has no key %s; a kind has key, name, colour and sign", i+1, rest)
			}
			return nil, aiplug.BadRequestf("kinds: entry %d must be {key, name, colour, sign}", i+1)
		}
		out = append(out, kd)
	}
	return out, nil
}

// parse reads a deck block's keys. The hero page is found by name; every
// other value is only carried, for the service's own lists to check.
func (k *deckKind) parse(ctx context.Context, campaignID string, r aiplug.Record) (deckEdit, error) {
	var e deckEdit
	var err error
	if e.rename, err = aiText(r, "renameto", "rename_to", false); err != nil {
		return e, err
	}
	if e.rename != nil && *e.rename == "" {
		e.rename = nil
	}
	if s, err := aiText(r, "for", "for", false); err != nil {
		return e, err
	} else if s != nil {
		kind, ok := deckKinds[aiKey(*s)]
		if !ok {
			return e, aiplug.BadRequestf("for: %q is not hero, party or events", *s)
		}
		e.kind = &kind
	}
	if h, err := aiText(r, "hero", "hero", true); err != nil {
		return e, err
	} else if h != nil {
		if name := strings.TrimSpace(*h); name == "" || strings.EqualFold(name, "none") {
			e.hero = ptr2("")
		} else {
			id, stored, ok, err := k.pages.FindPage(ctx, campaignID, name)
			if err != nil {
				return e, err
			}
			if !ok {
				return e, aiplug.BadRequestf("no page called %q for the hero", name)
			}
			e.hero, e.heroName = &id, stored
		}
	}
	var in SettingsInput
	var look LookInput
	for _, p := range aiPicks {
		v, err := aiText(r, aiKey(p.key), p.key, false)
		if err != nil {
			return e, err
		}
		if v == nil {
			continue
		}
		p.set(&in, &look, *v)
		if isRule(p.key) {
			e.rules = append(e.rules, p.key+" "+*v)
		} else {
			e.looks = append(e.looks, p.key+" "+*v)
		}
	}
	if d, err := aiYesNo(r, "discard", "discard"); err != nil {
		return e, err
	} else if d != nil {
		in.Discard = d
		e.rules = append(e.rules, fmt.Sprintf("discard %t", *d))
	}
	if r.Has("kinds") {
		kinds, err := kindDefs(r.Fields["kinds"])
		if err != nil {
			return e, err
		}
		look.Kinds = kinds
		e.looks = append(e.looks, fmt.Sprintf("%d card %s", len(kinds), plural(len(kinds), "kind", "kinds")))
	}
	if s, err := aiText(r, "style", "style", false); err != nil {
		return e, err
	} else if s != nil {
		st, ok := styleNamed(*s)
		if !ok {
			return e, aiplug.BadRequestf("style: %q is not one of %s", *s, styleKeys())
		}
		e.style = &st
	}
	if in != (SettingsInput{}) {
		e.settings = &in
	}
	if look.Shape != nil || look.Border != nil || look.Turn != nil || look.Window != nil || look.Name != nil || look.Lettering != nil ||
		look.Corner != nil || look.Marker != nil || look.Finish != nil || look.Back != nil || look.Kinds != nil {
		e.look = &look
	}
	return e, nil
}

// styledLook is the look a block asks for: the style fills it first and the
// single look keys written beside it override.
func (e deckEdit) styledLook() *LookInput {
	if e.style == nil {
		return e.look
	}
	base := e.style.LookInput()
	if e.look == nil {
		return &base
	}
	o := *e.look
	pick := func(dst **string, v *string) {
		if v != nil {
			*dst = v
		}
	}
	pick(&base.Shape, o.Shape)
	pick(&base.Border, o.Border)
	pick(&base.Turn, o.Turn)
	pick(&base.Window, o.Window)
	pick(&base.Name, o.Name)
	pick(&base.Lettering, o.Lettering)
	pick(&base.Corner, o.Corner)
	pick(&base.Marker, o.Marker)
	pick(&base.Finish, o.Finish)
	if o.Back != nil {
		b := *base.Back
		pick(&b.Pattern, o.Back.Pattern)
		pick(&b.Sign, o.Back.Sign)
		pick(&b.Colour, o.Back.Colour)
		pick(&b.Words, o.Back.Words)
		base.Back = &b
	}
	base.Kinds = o.Kinds
	return &base
}

// deckWork is a deck block planned: what the review shows and, when it can
// be applied, the one service call that writes it.
type deckWork struct {
	plan   aiplug.Plan
	id     string
	create *CreateDeckInput
	update *UpdateDeckInput
	staged *stagedDeck
}

func refuse(msg string) deckWork { return deckWork{plan: aiplug.Plan{Error: msg}} }

func (k *deckKind) work(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) deckWork {
	if msg := k.gate(ctx, campaignID, a); msg != "" {
		return refuse(msg)
	}
	if r.Action == aiplug.ActionDelete {
		return refuse("The AI does not remove a deck. Delete it in the deck workshop.")
	}
	rec, err := aiNormalise(r, "deck", deckOwn)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	if r.Name == "" {
		return refuse("a deck needs a name")
	}
	e, err := k.parse(ctx, campaignID, rec)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	v := directorViewer(a)
	hit, err := k.deckNamed(ctx, campaignID, v, r.Name)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	if r.Action == aiplug.ActionUpdate {
		return k.workUpdate(e, hit, r)
	}
	return k.workCreate(e, hit, r)
}

func (k *deckKind) workCreate(e deckEdit, hit *DirectorDeck, r aiplug.Record) deckWork {
	if hit != nil {
		return refuse("a deck called " + aiplug.Quote(r.Name) + " already exists; use action: update")
	}
	if e.kind == nil {
		return refuse("a deck needs `for`: hero, party or events")
	}
	kind := *e.kind
	switch {
	case kind == KindHero && (e.hero == nil || *e.hero == ""):
		return refuse("a hero deck needs `hero`, the page name of the hero it is for")
	case kind != KindHero && e.hero != nil && *e.hero != "":
		return refuse("only a hero deck belongs to a hero page")
	}
	// The service's own words, from the functions its Create runs.
	name, err := plainText("the deck's name", r.Name, MaxNameChars)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	if err := validKind(kind); err != nil {
		return refuse(aiplug.PlanError(err))
	}
	if _, err := applySettings(defaultSettings(kind), e.settings); err != nil {
		return refuse(aiplug.PlanError(err))
	}
	li := e.styledLook()
	look, err := applyLook(defaultLook(), li)
	if err != nil {
		return refuse(aiplug.PlanError(err))
	}
	in := CreateDeckInput{Name: name, Kind: kind, Settings: e.settings, Look: li}
	holder := ""
	if e.hero != nil {
		holder = *e.hero
		in.HolderEntityID = holder
	}
	s := fmt.Sprintf("makes the %s deck %s", deckNoun(kind), aiplug.Quote(name))
	if e.heroName != "" {
		s += " for " + e.heroName
	}
	if e.style != nil {
		s += ", in the " + e.style.Name + " style"
	}
	if len(e.rules) > 0 {
		s += "; rules: " + strings.Join(e.rules, ", ")
	}
	if len(e.looks) > 0 {
		s += "; look: " + strings.Join(e.looks, ", ")
	}
	return deckWork{plan: aiplug.Plan{Summary: s}, create: &in,
		staged: &stagedDeck{Name: name, Kind: kind, Holder: holder, Look: look}}
}

func (k *deckKind) workUpdate(e deckEdit, hit *DirectorDeck, r aiplug.Record) deckWork {
	if hit == nil {
		return refuse("no deck called " + aiplug.Quote(r.Name))
	}
	in := UpdateDeckInput{Version: patch.Of(hit.Version)}
	var parts []string
	var warns []string
	if e.rename != nil {
		in.Name = patch.Of(*e.rename)
		parts = append(parts, "renames it to "+aiplug.Quote(*e.rename))
	}
	kind := hit.Kind
	if e.kind != nil {
		kind = *e.kind
		in.Kind = patch.Of(kind)
		parts = append(parts, "makes it "+say(deckKindWords, kind))
		if kind != hit.Kind {
			warns = append(warns, "Who a deck is for decides which players can see it and its used cards.")
		}
	}
	holder := hit.HolderEntityID
	if e.hero != nil {
		holder = *e.hero
		if holder == "" {
			in.HolderEntityID = patch.Null[string]()
			parts = append(parts, "takes it off its hero")
		} else {
			in.HolderEntityID = patch.Of(holder)
			parts = append(parts, "for "+e.heroName)
		}
	}
	if kind != KindHero && holder != "" {
		return refuse("only a hero deck belongs to a hero page; add `hero: none` to take it off its hero")
	}
	if e.settings != nil {
		if _, err := applySettings(hit.Settings, e.settings); err != nil {
			return refuse(aiplug.PlanError(err))
		}
		in.Settings = patch.Of(*e.settings)
		parts = append(parts, "rules: "+strings.Join(e.rules, ", "))
		if e.settings.Knows != nil || e.settings.OthersSee != nil {
			warns = append(warns, "This changes what players are told about the pile or about each other's cards.")
		}
	}
	if li := e.styledLook(); li != nil {
		if _, err := applyLook(hit.Look, li); err != nil {
			return refuse(aiplug.PlanError(err))
		}
		in.Look = patch.Of(*li)
		if e.style != nil {
			parts = append(parts, "applies the "+e.style.Name+" style")
			warns = append(warns, "A style replaces the eleven look choices; the card kinds and the back's words stay.")
		}
		if len(e.looks) > 0 {
			parts = append(parts, "look: "+strings.Join(e.looks, ", "))
		}
	}
	if len(parts) == 0 {
		return refuse("nothing to change: name a key to change")
	}
	return deckWork{plan: aiplug.Plan{Summary: "changes the deck " + aiplug.Quote(hit.Name) + ": " + strings.Join(parts, "; "), Warnings: warns},
		id: hit.ID, update: &in}
}

func deckNoun(kind string) string {
	if kind == KindEvent {
		return "event"
	}
	return kind
}

func (k *deckKind) Plan(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) aiplug.Plan {
	return k.work(ctx, campaignID, a, r).plan
}

// Stage notes the deck a clean create row will make, so a card row after it
// in the same paste is checked against it. Commit applies rows in order, so
// by then the deck exists.
func (k *deckKind) Stage(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) context.Context {
	if r.Action != aiplug.ActionCreate {
		return ctx
	}
	if w := k.work(ctx, campaignID, a, r); w.plan.Error == "" && w.staged != nil {
		return withStagedDeck(ctx, *w.staged)
	}
	return ctx
}

// Apply re-plans, because the campaign may have changed since review, then
// writes through the service as a Director.
func (k *deckKind) Apply(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) error {
	w := k.work(ctx, campaignID, a, r)
	if w.plan.Error != "" {
		return apperror.NewBadRequest(w.plan.Error)
	}
	v := directorViewer(a)
	if w.create != nil {
		_, err := k.svc.Create(ctx, campaignID, v, *w.create)
		return err
	}
	_, err := k.svc.Update(ctx, campaignID, w.id, v, *w.update)
	return err
}

// Export is empty: decks are read with `what: decks`, which also says what
// the Director may not ask the AI to change.
func (*deckKind) Export(context.Context, string, aiplug.Actor) (string, error) { return "", nil }

func (*deckKind) Doc() string {
	return fmt.Sprintf("A deck of cards, matched by `name`. Keys: `for` (hero, party or events; needed to make one), `hero` (the page name of a hero deck's hero, needed for a hero deck; `none` takes a deck off its hero), `style` (a starting look: %s; it fills the whole look first and the single look keys below then override it), `rename_to`. Rules, each with the values the deck workshop offers: `knows` (what a player who sees the deck may know: %s), `others_see` (what others see of a card dealt to one hero: %s), `arrives` (how a card dealt to a hero lands: %s), `fate` (where a played card goes: %s), `empty` (a draw from an empty pile: %s), `discard` (yes or no, may players discard from their own hand), `notice` (how loudly a card's moment tells the Director: %s). Look: `shape` (%s), `border` (%s), `turn` (%s), `window` (%s), `name_position` (%s), `lettering` (%s), `corner` (the corner mark: %s), `marker` (%s), `finish` (%s), `back_pattern` (%s), `back_sign` (%s), `back_colour` (a palette colour: %s; empty is the campaign's colour), `back_words` (at most %d characters), and `kinds`, a list of `{key, name, colour, sign}` that replaces the deck's whole list (at most %d; the key is lowercase letters, digits and hyphens, the sign is one of %s). On update only the keys written change. The AI does not choose who may draw (the `draws` rule, the chosen players and their draws left), which card comes up next or is kept for last, or move a card (draw, deal, play, shuffle, gather): a block naming any of these is refused, and those are done in the deck workshop and at the table. The AI does not remove a deck; `action: delete` is refused and a deck is deleted in the deck workshop. Cards go in `card` blocks.\n\n```\n---\nkind: deck\nname: Omens\nfor: events\nstyle: iron-omens\nknows: count\nfate: used\nborder: metal\nkinds:\n  - {key: bane, name: Bane, colour: rose}\n---\n```",
		styleKeys(), strings.Join(knowsValues, ", "), strings.Join(othersSeeValues, ", "), strings.Join(arrivesValues, ", "),
		strings.Join(fateValues, ", "), strings.Join(emptyValues, ", "), strings.Join(noticeValues, ", "),
		strings.Join(shapeValues, ", "), strings.Join(borderValues, ", "), strings.Join(turnValues, ", "), strings.Join(windowValues, ", "),
		strings.Join(nameValues, ", "), strings.Join(letteringValues, ", "), strings.Join(cornerValues, ", "), strings.Join(markerValues, ", "),
		strings.Join(finishValues, ", "), strings.Join(patternValues, ", "), strings.Join(backSignValues, ", "), strings.Join(Palette, ", "),
		MaxBackWordsChars, MaxKinds, strings.Join(signValues, ", "))
}
