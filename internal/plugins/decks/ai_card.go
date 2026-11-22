package decks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

type cardKind struct{ *aiCore }

func (*cardKind) Name() string  { return "card" }
func (*cardKind) Label() string { return "Card" }
func (*cardKind) Brief() string {
	return "the cards of a deck: name, words, kind, copies, what happens to a card after use, a note for the Director and the tests that say when a card comes up; matched by name within a deck"
}

// cardOwn is the set of normalised keys a card block takes. The deck's kind
// is `card_kind` because the block's own `kind:` already says this is a card.
var cardOwn = map[string]bool{
	"deck": true, "words": true, "cardkind": true, "corner": true, "rare": true, "copies": true, "afteruse": true,
	"askfirst": true, "note": true, "picture": true, "renameto": true, "when": true,
}

// deckTarget is the deck a card block names: one that exists, or one an
// earlier row of the paste makes (id is "" then).
type deckTarget struct {
	id     string
	deck   Deck
	staged bool
}

func (c *aiCore) target(ctx context.Context, campaignID string, v Viewer, name string) (deckTarget, error) {
	if strings.TrimSpace(name) == "" {
		return deckTarget{}, aiplug.BadRequestf("a card needs `deck`, the name of the deck it belongs to")
	}
	hit, err := c.deckNamed(ctx, campaignID, v, name)
	if err != nil {
		return deckTarget{}, err
	}
	if hit != nil {
		return deckTarget{id: hit.ID, deck: Deck{ID: hit.ID, CampaignID: campaignID, Name: hit.Name, Kind: hit.Kind,
			HolderEntityID: hit.HolderEntityID, Settings: hit.Settings, Look: hit.Look}}, nil
	}
	if sd, ok := stagedDecks(ctx)[stagedName(name)]; ok {
		return deckTarget{staged: true, deck: Deck{CampaignID: campaignID, Name: sd.Name, Kind: sd.Kind,
			HolderEntityID: sd.Holder, Look: sd.Look}}, nil
	}
	return deckTarget{}, aiplug.BadRequestf("no deck called %q", name)
}

// cardWork is a card block planned: the review's plan and, when it can be
// applied, the one service call that writes it.
type cardWork struct {
	plan   aiplug.Plan
	deckID string
	cardID string
	create *CreateCardInput
	update *UpdateCardInput
}

func refuseCard(msg string) cardWork { return cardWork{plan: aiplug.Plan{Error: msg}} }

// cardEdit is what a card block asks for; a nil field was not written.
type cardEdit struct {
	rename  *string
	words   *string
	kind    *string // the deck's kind key; "" takes the card's kind off
	corner  *string
	rare    *bool
	copies  *int
	fate    *string
	ask     *bool
	note    *string
	picture *string
	when    *[]WhenTest
}

// resolveKind finds the deck's kind by key or by name.
func resolveKind(l Look, deck, s string) (string, error) {
	if s == "" {
		return "", nil
	}
	for _, k := range l.Kinds {
		if k.Key == s {
			return k.Key, nil
		}
	}
	var hit []KindDef
	for _, k := range l.Kinds {
		if aiplug.SameName(k.Name, s) {
			hit = append(hit, k)
		}
	}
	if len(hit) == 1 {
		return hit[0].Key, nil
	}
	names := make([]string, 0, len(l.Kinds))
	for _, k := range l.Kinds {
		names = append(names, k.Name)
	}
	have := "it has no card kinds yet; add them with `kinds` on a deck block"
	if len(names) > 0 {
		have = "its kinds are " + strings.Join(names, ", ")
	}
	return "", aiplug.BadRequestf("card_kind: %q is not one of %s's kinds; %s", s, deck, have)
}

func (k *cardKind) parse(ctx context.Context, campaignID string, v Viewer, t deckTarget, r aiplug.Record) (cardEdit, error) {
	var e cardEdit
	var err error
	if e.rename, err = aiText(r, "renameto", "rename_to", false); err != nil {
		return e, err
	}
	if e.rename != nil && *e.rename == "" {
		e.rename = nil
	}
	for _, f := range []struct {
		key, show string
		dst       **string
	}{{"words", "words", &e.words}, {"corner", "corner", &e.corner}, {"afteruse", "after_use", &e.fate},
		{"note", "note", &e.note}, {"picture", "picture", &e.picture}} {
		if *f.dst, err = aiText(r, f.key, f.show, true); err != nil {
			return e, err
		}
	}
	if s, err := aiText(r, "cardkind", "card_kind", true); err != nil {
		return e, err
	} else if s != nil {
		key, err := resolveKind(t.deck.Look, t.deck.Name, *s)
		if err != nil {
			return e, err
		}
		e.kind = &key
	}
	if e.rare, err = aiYesNo(r, "rare", "rare"); err != nil {
		return e, err
	}
	if e.ask, err = aiYesNo(r, "askfirst", "ask_first"); err != nil {
		return e, err
	}
	if r.Has("copies") {
		n, ok, err := r.Int("copies")
		if err != nil {
			return e, err
		}
		if !ok {
			return e, aiplug.BadRequestf("copies needs a whole number")
		}
		e.copies = &n
	}
	if r.Has("when") {
		tests, err := k.readWhen(ctx, campaignID, v, t, r.Fields["when"])
		if err != nil {
			return e, err
		}
		e.when = &tests
	}
	return e, nil
}

// readWhen turns the block's list of tests into the stored shape and has the
// service check it. The AI may say who by page name and a tracker or tag by
// name; those become ids first, then the same strict decoder the HTTP route
// uses refuses any key a kind of test does not have, and ValidateWhen, the
// one check, judges the names. Nothing about a test's rules is repeated here.
func (k *cardKind) readWhen(ctx context.Context, campaignID string, v Viewer, t deckTarget, raw any) ([]WhenTest, error) {
	if raw == nil {
		return nil, nil
	}
	items, ok := jsonable(raw).([]any)
	if !ok {
		if m, isMap := jsonable(raw).(map[string]any); isMap {
			items, ok = []any{m}, true
		}
	}
	if !ok {
		return nil, aiplug.BadRequestf("when must be a list of tests")
	}
	var opts *WhenOptionsView
	options := func() (WhenOptionsView, error) {
		if opts == nil {
			o, err := k.svc.WhenOptions(ctx, campaignID, v)
			if err != nil {
				return WhenOptionsView{}, err
			}
			opts = &o
		}
		return *opts, nil
	}
	// Only the tests the service will accept are resolved: a fifth is refused
	// by the decoder's own count.
	for i := 0; i < len(items) && i < MaxTests; i++ {
		m, isMap := items[i].(map[string]any)
		if !isMap {
			return nil, aiplug.BadRequestf("test %d: each test is a map like {test: tag, who: holder, tag: Cursed, has: true}", i+1)
		}
		if err := k.resolveTest(ctx, campaignID, i+1, m, options); err != nil {
			return nil, err
		}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return nil, aiplug.BadRequestf("when can't be read")
	}
	tests, err := ParseWhen(b)
	if err != nil {
		return nil, err
	}
	return k.svc.ValidateWhen(ctx, campaignID, t.deck, tests)
}

// resolveTest turns the names an AI wrote in one test into ids, in place.
func (k *cardKind) resolveTest(ctx context.Context, campaignID string, n int, m map[string]any, options func() (WhenOptionsView, error)) error {
	if who, ok := m["who"].(string); ok {
		switch w := strings.TrimSpace(who); strings.ToLower(w) {
		case WhoHolder, WhoAny:
			m["who"] = strings.ToLower(w)
		default:
			id, _, found, err := k.pages.FindPage(ctx, campaignID, w)
			if err != nil {
				return err
			}
			if !found {
				return aiplug.BadRequestf("test %d: no page called %q", n, w)
			}
			m["who"] = id
		}
	}
	switch m["test"] {
	case TestTag:
		name, ok := m["tag"].(string)
		if !ok {
			return nil
		}
		o, err := options()
		if err != nil {
			return err
		}
		var hits []int
		for _, g := range o.Tags {
			if aiplug.SameName(g.Name, name) {
				hits = append(hits, g.ID)
			}
		}
		switch len(hits) {
		case 0:
			return aiplug.BadRequestf("test %d: no tag called %q", n, name)
		case 1:
			m["tag"] = hits[0]
		default:
			return aiplug.BadRequestf("test %d: more than one tag is called %q", n, name)
		}
	case TestTracker:
		name, ok := m["kind"].(string)
		if !ok {
			return nil
		}
		o, err := options()
		if err != nil {
			return err
		}
		// A name no tracker has is left as written: it may be a tracker's id,
		// and the one check refuses it if it is neither.
		var hits []string
		for _, tk := range o.Trackers {
			if aiplug.SameName(tk.Name, name) {
				hits = append(hits, tk.ID)
			}
		}
		switch len(hits) {
		case 0:
		case 1:
			m["kind"] = hits[0]
		default:
			return aiplug.BadRequestf("test %d: more than one tracker is called %q", n, name)
		}
	}
	return nil
}

// whenWords says the tests as the editor does, each with whose it is:
// "Orrin's Hit points is at or below 5 (on the card) and the Director says so
// (only you)".
func whenWords(views []WhenView) string {
	parts := make([]string, 0, len(views))
	for _, w := range views {
		who := "only you"
		if w.Open {
			who = "on the card"
		}
		parts = append(parts, w.Sentence+" ("+who+")")
	}
	return strings.Join(parts, " and ")
}

func (k *cardKind) work(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) cardWork {
	if msg := k.gate(ctx, campaignID, a); msg != "" {
		return refuseCard(msg)
	}
	if r.Action == aiplug.ActionDelete {
		return refuseCard("The AI does not remove a card. Delete it in the deck workshop.")
	}
	rec, err := aiNormalise(r, "card", cardOwn)
	if err != nil {
		return refuseCard(aiplug.PlanError(err))
	}
	if r.Name == "" {
		return refuseCard("a card needs a name")
	}
	v := directorViewer(a)
	t, err := k.target(ctx, campaignID, v, rec.Str("deck"))
	if err != nil {
		return refuseCard(aiplug.PlanError(err))
	}
	e, err := k.parse(ctx, campaignID, v, t, rec)
	if err != nil {
		return refuseCard(aiplug.PlanError(err))
	}
	var hit *DirectorCard
	if !t.staged {
		view, err := k.svc.Get(ctx, campaignID, t.id, v)
		if err != nil {
			return refuseCard(aiplug.PlanError(err))
		}
		d, ok := view.(DirectorDeck)
		if !ok {
			return refuseCard("the deck could not be read")
		}
		for i := range d.Cards {
			if aiplug.SameName(d.Cards[i].Name, r.Name) {
				if hit != nil {
					return refuseCard("more than one card in " + t.deck.Name + " is called " + aiplug.Quote(r.Name))
				}
				hit = &d.Cards[i]
			}
		}
	}
	if r.Action == aiplug.ActionUpdate {
		return k.workUpdate(ctx, campaignID, v, t, hit, e, r)
	}
	return k.workCreate(ctx, campaignID, v, t, hit, e, r)
}

// checked runs the service's own card check on the card as it would be stored.
func checked(d Deck, c Card) aiplug.Plan {
	if err := checkCard(d, &c); err != nil {
		return aiplug.Plan{Error: aiplug.PlanError(err)}
	}
	return aiplug.Plan{}
}

func (k *cardKind) workCreate(ctx context.Context, campaignID string, v Viewer, t deckTarget, hit *DirectorCard, e cardEdit, r aiplug.Record) cardWork {
	if hit != nil {
		return refuseCard("a card called " + aiplug.Quote(r.Name) + " already exists in " + t.deck.Name + "; use action: update")
	}
	in := CreateCardInput{Name: r.Name, Copies: e.copies}
	c := Card{Name: r.Name, Copies: 1}
	if e.words != nil {
		in.Words, c.Words = *e.words, *e.words
	}
	if e.kind != nil {
		in.KindKey, c.KindKey = *e.kind, *e.kind
	}
	if e.corner != nil {
		in.Corner, c.Corner = *e.corner, *e.corner
	}
	if e.rare != nil {
		in.Rare = *e.rare
	}
	if e.copies != nil {
		c.Copies = *e.copies
	}
	if e.fate != nil {
		in.Fate, c.Fate = *e.fate, *e.fate
	}
	if e.ask != nil {
		in.AskFirst = *e.ask
	}
	if e.note != nil {
		in.DirectorNote, c.DirectorNote = *e.note, *e.note
	}
	if e.picture != nil {
		in.Picture = *e.picture
	}
	if p := checked(t.deck, c); p.Error != "" {
		return refuseCard(p.Error)
	}
	s := "adds " + r.Name + " to " + t.deck.Name + fmt.Sprintf(", %d %s", c.Copies, plural(c.Copies, "copy", "copies"))
	s += k.extras(e, false)
	if e.when != nil && len(*e.when) > 0 {
		in.When = WhenList(*e.when)
		views, err := k.svc.SayWhen(ctx, campaignID, t.deck, v, *e.when)
		if err != nil {
			return refuseCard(aiplug.PlanError(err))
		}
		s += "; comes up when " + whenWords(views)
	}
	return cardWork{plan: aiplug.Plan{Summary: s}, deckID: t.id, create: &in}
}

// extras are the summary's words for the keys written beside the name and the
// copies, in a fixed order.
func (k *cardKind) extras(e cardEdit, update bool) string {
	var parts []string
	if e.kind != nil {
		if *e.kind == "" {
			parts = append(parts, "no card kind")
		} else {
			parts = append(parts, "card kind "+*e.kind)
		}
	}
	if e.rare != nil {
		if *e.rare {
			parts = append(parts, "rare")
		} else if update {
			parts = append(parts, "not rare")
		}
	}
	if e.ask != nil {
		if *e.ask {
			parts = append(parts, "ask first")
		} else if update {
			parts = append(parts, "no ask first")
		}
	}
	if e.fate != nil {
		if *e.fate == "" {
			parts = append(parts, "after use: the deck's own")
		} else {
			parts = append(parts, "after use: "+*e.fate)
		}
	}
	if e.corner != nil && *e.corner != "" {
		parts = append(parts, "corner "+aiplug.Quote(*e.corner))
	}
	if e.picture != nil {
		if *e.picture == "" {
			parts = append(parts, "no picture")
		} else {
			parts = append(parts, "a picture")
		}
	}
	if e.note != nil && *e.note != "" {
		parts = append(parts, "a note for the Director")
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, "; ")
}

func (k *cardKind) workUpdate(ctx context.Context, campaignID string, v Viewer, t deckTarget, hit *DirectorCard, e cardEdit, r aiplug.Record) cardWork {
	if hit == nil {
		return refuseCard("no card called " + aiplug.Quote(r.Name) + " in " + t.deck.Name)
	}
	in := UpdateCardInput{}
	c := Card{Name: hit.Name, KindKey: hit.KindKey, Words: hit.Words, DirectorNote: hit.DirectorNote, Corner: hit.Corner,
		Copies: hit.Copies, Fate: hit.Fate}
	var parts []string
	if e.rename != nil {
		in.Name, c.Name = patch.Of(*e.rename), *e.rename
		parts = append(parts, "renamed to "+aiplug.Quote(*e.rename))
	}
	if e.words != nil {
		in.Words, c.Words = patch.Of(*e.words), *e.words
		parts = append(parts, "new words")
	}
	if e.kind != nil {
		in.KindKey, c.KindKey = patch.Of(*e.kind), *e.kind
	}
	if e.corner != nil {
		in.Corner, c.Corner = patch.Of(*e.corner), *e.corner
	}
	if e.rare != nil {
		in.Rare = patch.Of(*e.rare)
	}
	if e.copies != nil {
		in.Copies, c.Copies = patch.Of(*e.copies), *e.copies
		parts = append(parts, fmt.Sprintf("%d %s", *e.copies, plural(*e.copies, "copy", "copies")))
	}
	if e.fate != nil {
		in.Fate, c.Fate = patch.Of(*e.fate), *e.fate
	}
	if e.ask != nil {
		in.AskFirst = patch.Of(*e.ask)
	}
	if e.note != nil {
		in.DirectorNote, c.DirectorNote = patch.Of(*e.note), *e.note
	}
	if e.picture != nil {
		in.Picture = patch.Of(*e.picture)
	}
	if p := checked(t.deck, c); p.Error != "" {
		return refuseCard(p.Error)
	}
	if x := k.extras(e, true); x != "" {
		parts = append(parts, strings.TrimPrefix(x, "; "))
	}
	if e.when != nil {
		in.When = patch.Of(WhenList(*e.when))
		if len(*e.when) == 0 {
			parts = append(parts, "no tests, so it comes up whenever it is drawn")
		} else {
			views, err := k.svc.SayWhen(ctx, campaignID, t.deck, v, *e.when)
			if err != nil {
				return refuseCard(aiplug.PlanError(err))
			}
			parts = append(parts, "comes up when "+whenWords(views))
		}
	}
	if len(parts) == 0 {
		return refuseCard("nothing to change: name a key to change")
	}
	return cardWork{plan: aiplug.Plan{Summary: "changes " + hit.Name + " in " + t.deck.Name + ": " + strings.Join(parts, "; ")},
		deckID: t.id, cardID: hit.ID, update: &in}
}

func (k *cardKind) Plan(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) aiplug.Plan {
	return k.work(ctx, campaignID, a, r).plan
}

// Apply re-plans, then writes through the service as a Director; the service
// checks the card and its tests again.
func (k *cardKind) Apply(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) error {
	w := k.work(ctx, campaignID, a, r)
	if w.plan.Error != "" {
		return apperror.NewBadRequest(w.plan.Error)
	}
	v := directorViewer(a)
	if w.create != nil {
		_, err := k.svc.CreateCard(ctx, campaignID, w.deckID, v, *w.create)
		return err
	}
	_, err := k.svc.UpdateCard(ctx, campaignID, w.deckID, w.cardID, v, *w.update)
	return err
}

// Export is empty: a deck's cards are read with `what: decks` and its `deck`.
func (*cardKind) Export(context.Context, string, aiplug.Actor) (string, error) { return "", nil }

func (*cardKind) Doc() string {
	ops := make([]string, 0, len(opPhrases))
	for _, o := range opPhrases {
		ops = append(ops, o.Key)
	}
	shares := make([]string, 0, len(sharePhrases))
	for _, o := range sharePhrases {
		shares = append(shares, o.Key)
	}
	return fmt.Sprintf("A card in a deck, matched by `name` within the deck named by `deck` (a deck made by a `deck` block earlier in the same paste works). Keys: `words`, `card_kind` (one of the deck's kinds, by its key or its name; the block's own `kind:` already says this is a card), `corner` (at most %d characters), `rare` and `ask_first` (yes or no; ask first makes a player's play wait for the Director), `copies` (1 to %d), `after_use` (where a card goes once played: %s; empty is the deck's own rule), `note` (the Director's note, never shown to players), `picture` (a media id of an image in this campaign's media, checked when it is applied), `rename_to`, and `when`. On update only the keys written change. `when` is a list of up to %d tests that must all hold for the card to come up, and replaces the card's whole list (an empty list clears it). Each test has `test` (number, tracker, tag, layer or say), `open` (yes prints the test on the card for players to read; the default is the Director's own, which no player is ever sent) and its kind's keys. `number`: `who`, `field` (a sheet field's key), `op` (%s), then `value` or `of` with `share` (%s). `tracker`: `who`, `kind` (the tracker's name), `op`, `value`. `tag`: `who`, `tag` (the tag's name), `has`. `layer`: `who`, `name`, `on`. `say`: no other keys; it holds while the Director says so. `who` is `holder` (the hero a hero deck is for), `any` (any hero) or a page name. A test with a key its kind does not have is refused by name. The AI does not remove a card: `action: delete` is refused and a card is deleted in the deck workshop. Moving a card, who may draw and which card comes up next are not done by the AI.\n\n```\n---\nkind: card\ndeck: Omens\nname: Frostbite\ncopies: 3\ncard_kind: Bane\nwords: The cold finds every gap in the armour.\nwhen:\n  - {test: number, who: Orrin, field: hp, op: lte, value: 5, open: true}\n  - {test: say}\n---\n```",
		MaxCornerChars, MaxCopiesPerCard, strings.Join(fateValues, ", "), MaxTests, strings.Join(ops, ", "), strings.Join(shares, ", "))
}
