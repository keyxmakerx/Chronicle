package app

// decks_homebrew.go puts card decks in the Homebrew Workshop: two events a
// deck fires after a move is stored (a card is played, a card is drawn) and
// three actions a rule can end in. A number on a sheet or a tracker moves at
// once; anything bigger waits for a Director, so every deck action is marked
// Waits and none of them deals, draws or plays a card for anyone.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// The deck events and actions a recipe names.
const (
	cardPlayedEvent = "card.played"
	cardDrawnEvent  = "card.drawn"
	deckAddCopies   = "deck.add-copies"
	deckTakeOut     = "deck.take-out"
	deckShuffleBack = "deck.shuffle-back"
)

// decksStoryBudget bounds one firing: a move carries no request context that
// should outlive its reply.
const decksStoryBudget = 10 * time.Second

func init() {
	addWorkshopSource("card decks", func(reg *homebrew.Registry, d workshopDeps) error {
		if d.Decks == nil || !d.Health.IsHealthy(decks.PluginSlug) {
			return nil
		}
		return registerDecksHomebrew(reg, d.Decks)
	})
}

// decksStory is what a deck tells after a move and what the deck actions
// reach the decks service through. The services it needs are bound once they
// exist, because the deck service is built before the Homebrew one; until
// both are bound it does nothing.
type decksStory struct {
	// enabled says whether card decks are switched on for a campaign.
	enabled func(ctx context.Context, campaignID string) bool
	// spawn runs a firing off the move's own goroutine; tests run it inline.
	spawn func(func())

	mu       sync.RWMutex
	decks    decks.Service
	homebrew homebrew.Service
}

func newDecksStory(enabled func(ctx context.Context, campaignID string) bool) *decksStory {
	return &decksStory{enabled: enabled, spawn: func(f func()) { go f() }}
}

// bindDecks sets the deck service the actions act through.
func (s *decksStory) bindDecks(svc decks.Service) {
	s.mu.Lock()
	s.decks = svc
	s.mu.Unlock()
}

// bindHomebrew sets the Workshop's service the events are fired into; it stays
// unbound while the Homebrew plugin is degraded, which makes every event a
// no-op.
func (s *decksStory) bindHomebrew(svc homebrew.Service) {
	s.mu.Lock()
	s.homebrew = svc
	s.mu.Unlock()
}

func (s *decksStory) services() (decks.Service, homebrew.Service) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.decks, s.homebrew
}

var _ decks.Story = (*decksStory)(nil)

func (s *decksStory) CardDrawn(_ context.Context, campaignID string, e decks.CardEvent) {
	s.fire(campaignID, cardDrawnEvent, e)
}

func (s *decksStory) CardPlayed(_ context.Context, campaignID string, e decks.CardEvent) {
	s.fire(campaignID, cardPlayedEvent, e)
}

// cardPayload is what a card event carries for Match: names to compare, ids
// for anyone who wants them. It never says which copy moved.
type cardPayload struct {
	DeckID string `json:"deckId"`
	Deck   string `json:"deck"`
	CardID string `json:"cardId"`
	Card   string `json:"card"`
}

// fire never blocks and never fails: the move that caused it is stored and
// answered for, and must not wait on, or fail from, a rule. It runs on its
// own short context, logs what goes wrong, and survives a panic in a rule.
func (s *decksStory) fire(campaignID, event string, e decks.CardEvent) {
	_, hb := s.services()
	if hb == nil || campaignID == "" {
		return
	}
	s.spawn(func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("decks: a rule panicked", slog.Any("panic", r), slog.String("campaign_id", campaignID), slog.String("event", event))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), decksStoryBudget)
		defer cancel()
		// Card decks are an add-on: a campaign that switched them off fires
		// nothing from the cards it still has.
		if s.enabled != nil && !s.enabled(ctx, campaignID) {
			return
		}
		payload, err := json.Marshal(cardPayload{DeckID: e.DeckID, Deck: e.DeckName, CardID: e.CardID, Card: e.CardName})
		if err != nil {
			slog.Warn("decks: could not describe a card event", slog.String("campaign_id", campaignID), slog.Any("error", err))
			return
		}
		// A player's move sets off the Director's own rule, so it runs with a
		// Director's standing, as a player's tracker change does; the player
		// stays the one named as having set it off. Scope is the hero's page,
		// so a tracker action lands on the hero who played or drew.
		v := homebrew.Viewer{UserID: e.By.UserID, Director: true, VisibilityRole: permissions.RoleOwner}
		// Once: a player can draw and play the same card all night, and a rule
		// that waits for the Director is asked once, not once per play.
		if err := hb.Fire(ctx, campaignID, v, homebrew.Fired{Event: event, Scope: e.HeroID, Payload: payload, Once: true}); err != nil {
			slog.Warn("decks: a rule could not be started", slog.String("campaign_id", campaignID), slog.String("event", event), slog.Any("error", err))
		}
	})
}

// cardNameArgs is a card event's test: which deck and which card, by name so a
// recipe still works in another campaign. An empty field means any.
type cardNameArgs struct {
	Deck string `json:"deck,omitempty"`
	Card string `json:"card,omitempty"`
}

// foldName is a name as it is compared: single-spaced, lower-cased.
func foldName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// checkCardNames trims, single-spaces and caps the two optional names.
func checkCardNames(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a cardNameArgs
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	var err error
	if a.Deck, err = deckRefText("the deck's name", a.Deck, false); err != nil {
		return nil, err
	}
	if a.Card, err = deckRefText("the card's name", a.Card, false); err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

// deckRefText is calendarText for a deck or card name: one line, inner runs of
// white space collapsed, within what a deck keeps for a name.
func deckRefText(field, s string, required bool) (string, error) {
	return calendarText(field, strings.Join(strings.Fields(s), " "), decks.MaxNameChars, required)
}

// cardMatches meets a rule's When: each name it sets must equal the event's,
// without regard to case; a name left empty matches any.
func cardMatches(args, payload json.RawMessage) (bool, error) {
	var a cardNameArgs
	var p cardPayload
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return false, fmt.Errorf("card event args: %w", err)
		}
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return false, fmt.Errorf("card event payload: %w", err)
	}
	if a.Deck != "" && foldName(a.Deck) != foldName(p.Deck) {
		return false, nil
	}
	if a.Card != "" && foldName(a.Card) != foldName(p.Card) {
		return false, nil
	}
	return true, nil
}

// registerDecksHomebrew plugs card decks into the Workshop. Every action is
// Waits: it changes more than a number, so a rule that ends in one is held
// for a Director to run.
func registerDecksHomebrew(reg *homebrew.Registry, st *decksStory) error {
	names := []homebrew.Field{
		{Name: "deck", Label: "Deck", Type: homebrew.FieldText},
		{Name: "card", Label: "Card", Type: homebrew.FieldText},
	}
	regs := []func() error{
		func() error {
			return reg.RegisterEvent(homebrew.EventDef{Name: cardPlayedEvent, Label: "A card is played", Fields: names, Check: checkCardNames, Match: cardMatches})
		},
		func() error {
			return reg.RegisterEvent(homebrew.EventDef{Name: cardDrawnEvent, Label: "A card is drawn", Fields: names, Check: checkCardNames, Match: cardMatches})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: deckAddCopies, Label: "Shuffle copies of a card into a deck", Waits: true,
				Fields: append(append([]homebrew.Field{}, names...), homebrew.Field{Name: "copies", Label: "Copies", Type: homebrew.FieldWhole}),
				Check:  checkAddCopies, Run: st.addCopies})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: deckTakeOut, Label: "Take a card out of a deck for good", Waits: true,
				Fields: names, Check: checkDeckAndCard, Run: st.takeOut})
		},
		func() error {
			return reg.RegisterAction(homebrew.ActionDef{Name: deckShuffleBack, Label: "Shuffle the used pile back into a deck", Waits: true,
				Fields: names[:1], Check: checkDeckOnly, Run: st.shuffleBack})
		},
	}
	for _, r := range regs {
		if err := r(); err != nil {
			return err
		}
	}
	return nil
}

type addCopiesArgs struct {
	Deck   string `json:"deck"`
	Card   string `json:"card"`
	Copies *int   `json:"copies"`
}

func checkAddCopies(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a addCopiesArgs
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	var err error
	if a.Deck, err = deckRefText("the deck's name", a.Deck, true); err != nil {
		return nil, err
	}
	if a.Card, err = deckRefText("the card's name", a.Card, true); err != nil {
		return nil, err
	}
	if a.Copies == nil || *a.Copies < 1 || *a.Copies > decks.MaxCopiesPerCard {
		return nil, apperror.NewValidation(fmt.Sprintf("give a whole number of copies from 1 to %d", decks.MaxCopiesPerCard))
	}
	return json.Marshal(a)
}

func checkDeckAndCard(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a cardNameArgs
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	var err error
	if a.Deck, err = deckRefText("the deck's name", a.Deck, true); err != nil {
		return nil, err
	}
	if a.Card, err = deckRefText("the card's name", a.Card, true); err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func checkDeckOnly(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Deck string `json:"deck"`
	}
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	var err error
	if a.Deck, err = deckRefText("the deck's name", a.Deck, true); err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

// asDirector maps the Director who pressed Run to a decks viewer with exactly
// that standing, and refuses anyone who is not one.
func asDirector(by homebrew.Viewer) (decks.Viewer, error) {
	if !by.Director {
		return decks.Viewer{}, apperror.NewForbidden("Only a Director can do that.")
	}
	return decks.Viewer{UserID: by.UserID, VisibilityRole: by.VisibilityRole, Director: true}, nil
}

// deckScene is the deck a rule named and the card in it, read as the Director.
type deckScene struct {
	svc  decks.Service
	v    decks.Viewer
	deck decks.DirectorDeck
	card *decks.DirectorCard
}

// open resolves a deck, and a card when cardName is not "", by name without
// regard to case. A name that matches none or more than one fails with the
// sentence the drawer shows.
func (s *decksStory) open(ctx context.Context, campaignID string, by homebrew.Viewer, deckName, cardName string) (*deckScene, error) {
	svc, _ := s.services()
	if svc == nil {
		return nil, apperror.NewValidation("Card decks are not available right now.")
	}
	if s.enabled != nil && !s.enabled(ctx, campaignID) {
		return nil, apperror.NewValidation("Card decks are switched off for this campaign.")
	}
	v, err := asDirector(by)
	if err != nil {
		return nil, err
	}
	list, err := svc.List(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	all, ok := list.(decks.DirectorList)
	if !ok {
		return nil, apperror.NewValidation("Card decks are not available right now.")
	}
	var found []decks.DirectorDeck
	for _, d := range all.Decks {
		if foldName(d.Name) == foldName(deckName) {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 0:
		return nil, apperror.NewValidation(fmt.Sprintf("There is no deck called %s.", deckName))
	case 1:
	default:
		return nil, apperror.NewValidation(fmt.Sprintf("There is more than one deck called %s.", deckName))
	}
	view, err := svc.Get(ctx, campaignID, found[0].ID, v)
	if err != nil {
		return nil, err
	}
	deck, ok := view.(decks.DirectorDeck)
	if !ok {
		return nil, apperror.NewValidation("Card decks are not available right now.")
	}
	sc := &deckScene{svc: svc, v: v, deck: deck}
	if cardName == "" {
		return sc, nil
	}
	var cards []int
	for i, c := range deck.Cards {
		if foldName(c.Name) == foldName(cardName) {
			cards = append(cards, i)
		}
	}
	switch len(cards) {
	case 0:
		return nil, apperror.NewValidation(fmt.Sprintf("There is no card called %s in %s.", cardName, deck.Name))
	case 1:
		sc.card = &deck.Cards[cards[0]]
		return sc, nil
	}
	return nil, apperror.NewValidation(fmt.Sprintf("There is more than one card called %s in %s.", cardName, deck.Name))
}

// copiesOf lists where the card's copies are, as the Director's view has them.
func (sc *deckScene) copiesOf() []decks.DirectorCopy {
	var out []decks.DirectorCopy
	for _, c := range sc.deck.Copies {
		if c.CardID == sc.card.ID {
			out = append(out, c)
		}
	}
	return out
}

func copyWord(n int) string {
	if n == 1 {
		return "1 copy"
	}
	return fmt.Sprintf("%d copies", n)
}

func (s *decksStory) addCopies(ctx context.Context, campaignID string, a homebrew.Act) (string, error) {
	var args addCopiesArgs
	if err := json.Unmarshal(a.Args, &args); err != nil || args.Copies == nil {
		return "", fmt.Errorf("deck.add-copies args: %v", err)
	}
	sc, err := s.open(ctx, campaignID, a.By, args.Deck, args.Card)
	if err != nil {
		return "", err
	}
	// The card's count is every copy it has, wherever it lies; the edit sets
	// the new total and the deck adds the difference to the pile.
	have := len(sc.copiesOf())
	want := have + *args.Copies
	if want > decks.MaxCopiesPerCard {
		return "", apperror.NewValidation(fmt.Sprintf("%s has %s in %s and a card can have at most %d.", sc.card.Name, copyWord(have), sc.deck.Name, decks.MaxCopiesPerCard))
	}
	if _, err := sc.svc.UpdateCard(ctx, campaignID, sc.deck.ID, sc.card.ID, sc.v, decks.UpdateCardInput{Copies: patch.Of(want)}); err != nil {
		return "", err
	}
	return fmt.Sprintf("Shuffled %s of %s into %s; it now has %d.", copyWord(*args.Copies), sc.card.Name, sc.deck.Name, want), nil
}

func (s *decksStory) takeOut(ctx context.Context, campaignID string, a homebrew.Act) (string, error) {
	var args cardNameArgs
	if err := json.Unmarshal(a.Args, &args); err != nil {
		return "", fmt.Errorf("deck.take-out args: %v", err)
	}
	sc, err := s.open(ctx, campaignID, a.By, args.Deck, args.Card)
	if err != nil {
		return "", err
	}
	all := sc.copiesOf()
	pile, elsewhere := 0, []decks.DirectorCopy{}
	for _, c := range all {
		if c.Zone == decks.ZonePile {
			pile++
		} else {
			elsewhere = append(elsewhere, c)
		}
	}
	if pile == 0 {
		return fmt.Sprintf("No copy of %s is in the pile of %s, so nothing changed.%s", sc.card.Name, sc.deck.Name, sc.stillText(ctx, campaignID, elsewhere, " ")), nil
	}
	// Lowering the card's copies takes them from the pile first, so taking out
	// exactly the pile's copies leaves every other copy where it is. A card
	// must keep at least one copy, so when the pile held them all the card
	// itself leaves the deck.
	if len(elsewhere) == 0 {
		if _, err := sc.svc.DeleteCard(ctx, campaignID, sc.deck.ID, sc.card.ID, sc.v); err != nil {
			return "", err
		}
		return fmt.Sprintf("Took %s of %s out of %s. It had no other copies, so the card is gone from the deck.", copyWord(pile), sc.card.Name, sc.deck.Name), nil
	}
	if _, err := sc.svc.UpdateCard(ctx, campaignID, sc.deck.ID, sc.card.ID, sc.v, decks.UpdateCardInput{Copies: patch.Of(len(elsewhere))}); err != nil {
		return "", err
	}
	return fmt.Sprintf("Took %s of %s out of %s.%s", copyWord(pile), sc.card.Name, sc.deck.Name, sc.stillText(ctx, campaignID, elsewhere, " ")), nil
}

// stillText says, in a sentence led by lead, where the copies that stayed are:
// "1 is still in Orrin's hand and 2 are in the used pile."
func (sc *deckScene) stillText(ctx context.Context, campaignID string, left []decks.DirectorCopy, lead string) string {
	if len(left) == 0 {
		return ""
	}
	heroes := map[string]string{}
	for _, c := range left {
		if c.HolderEntityID != "" {
			if r, err := sc.svc.Roster(ctx, campaignID, sc.v); err == nil {
				for _, h := range r.Heroes {
					heroes[h.ID] = h.Name
				}
			}
			break
		}
	}
	var order []string
	counts := map[string]int{}
	for _, c := range left {
		var where string
		switch {
		case c.Zone == decks.ZoneHand && c.HolderEntityID != "":
			where = "in " + heroName(heroes, c.HolderEntityID) + "'s hand"
		case c.Zone == decks.ZoneHand:
			where = "in the party's hand"
		case c.Zone == decks.ZonePlay && c.HolderEntityID != "":
			where = "in play in front of " + heroName(heroes, c.HolderEntityID)
		case c.Zone == decks.ZonePlay:
			where = "on the table"
		case c.Zone == decks.ZoneShown:
			where = "in the shown spot"
		case c.Zone == decks.ZoneUsed:
			where = "in the used pile"
		default:
			where = "gone from play"
		}
		if counts[where] == 0 {
			order = append(order, where)
		}
		counts[where]++
	}
	parts := make([]string, 0, len(order))
	for i, where := range order {
		n := counts[where]
		verb := "are"
		if n == 1 {
			verb = "is"
		}
		if i == 0 {
			parts = append(parts, fmt.Sprintf("%d %s still %s", n, verb, where))
		} else {
			parts = append(parts, fmt.Sprintf("%d %s %s", n, verb, where))
		}
	}
	return lead + strings.Join(parts, " and ") + "."
}

func heroName(heroes map[string]string, id string) string {
	if n := heroes[id]; n != "" {
		return n
	}
	return "a hero"
}

func (s *decksStory) shuffleBack(ctx context.Context, campaignID string, a homebrew.Act) (string, error) {
	var args struct {
		Deck string `json:"deck"`
	}
	if err := json.Unmarshal(a.Args, &args); err != nil {
		return "", fmt.Errorf("deck.shuffle-back args: %v", err)
	}
	sc, err := s.open(ctx, campaignID, a.By, args.Deck, "")
	if err != nil {
		return "", err
	}
	used := 0
	for _, c := range sc.deck.Copies {
		if c.Zone == decks.ZoneUsed {
			used++
		}
	}
	if used == 0 {
		return fmt.Sprintf("%s has no used cards, so nothing changed.", sc.deck.Name), nil
	}
	// Shuffle moves only the used pile, so no card leaves a hand or the table.
	if _, err := sc.svc.Shuffle(ctx, campaignID, sc.deck.ID, sc.v); err != nil {
		return "", err
	}
	word := "cards"
	if used == 1 {
		word = "card"
	}
	return fmt.Sprintf("Shuffled %d used %s back into %s.", used, word, sc.deck.Name), nil
}
