package app

// decks_homebrew_draw.go is the Workshop's deck part and the action that
// draws from it for a hero. The part points at one of the campaign's decks
// and never copies it. The action does what a Director does at the table, a
// draw to the shown spot and then a deal of each card to the hero's hand,
// through the decks service, so the hand, the bell, card.drawn and every
// guard behave exactly as they do for a draw by hand.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// maxDeckIDChars bounds the id a part stores; a deck id is a UUID.
const maxDeckIDChars = 64

// The decks service's copy verbs the draw uses, as its routes name them.
const (
	deckVerbDeal    = "deal"
	deckVerbPutBack = "put-back"
)

// deckPartData is a deck part's data: the deck's id, which the actions use,
// and its name as the campaign spells it, kept for display and for a message
// that can still name the deck once the id means nothing (a recipe moved to
// another campaign, or the deck deleted).
type deckPartData struct {
	DeckID string `json:"deckId"`
	Name   string `json:"name,omitempty"`
}

// drawArgs is deck.draw's args. Count is how many cards, 1 by default.
type drawArgs struct {
	Count *int `json:"count,omitempty"`
}

// checkDeckPart accepts only a deck of this campaign and refreshes its name,
// so the stored name follows a renamed deck on every save.
func (s *decksStory) checkDeckPart(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
	var d deckPartData
	if err := strictJSON(raw, &d); err != nil {
		return nil, err
	}
	d.DeckID = strings.TrimSpace(d.DeckID)
	d.Name = strings.Join(strings.Fields(d.Name), " ")
	if d.DeckID == "" {
		return nil, apperror.NewValidation("pick one of the campaign's decks")
	}
	if len(d.DeckID) > maxDeckIDChars {
		return nil, apperror.NewValidation("is not one of the campaign's decks")
	}
	svc, err := s.ready(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	// Only Directors save recipes, so the lookup is a Director's and sees
	// every deck of the campaign; another campaign's deck is not found.
	deck, err := deckByID(ctx, svc, campaignID, decks.Viewer{Director: true, VisibilityRole: permissions.RoleOwner}, d.DeckID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperror.NewValidation(goneDeckText(d.Name, "is not one of this campaign's decks; choose it again"))
		}
		return nil, err
	}
	d.Name = deck.Name
	return json.Marshal(d)
}

// checkDrawArgs keeps a draw within what a Director's draw takes at once.
func checkDrawArgs(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var a drawArgs
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	if a.Count == nil {
		one := 1
		a.Count = &one
	}
	if *a.Count < 1 || *a.Count > decks.MaxDrawCount {
		return nil, apperror.NewValidation(fmt.Sprintf("draw a whole number of cards from 1 to %d", decks.MaxDrawCount))
	}
	return json.Marshal(a)
}

// draw deals cards from the part's deck to the hero whose play or press set
// the rule off. It runs only when a Director presses Run (the action Waits),
// as that Director.
func (s *decksStory) draw(ctx context.Context, campaignID string, a homebrew.Act) (string, error) {
	if a.Part == nil {
		return "", errors.New("deck.draw: no part")
	}
	var part deckPartData
	if err := json.Unmarshal(a.Part.Data, &part); err != nil {
		return "", fmt.Errorf("deck.draw part: %w", err)
	}
	var args drawArgs
	if err := json.Unmarshal(a.Args, &args); err != nil {
		return "", fmt.Errorf("deck.draw args: %w", err)
	}
	count := 1
	if args.Count != nil {
		count = *args.Count
	}
	if a.Scope == "" {
		return "", apperror.NewValidation("There is no hero to draw for: this rule was set off from no hero's page. Deal the cards from the deck yourself.")
	}
	svc, v, err := s.asDirectorHere(ctx, campaignID, a.By)
	if err != nil {
		return "", err
	}
	deck, err := deckByID(ctx, svc, campaignID, v, part.DeckID)
	if err != nil {
		if isNotFound(err) {
			return "", apperror.NewValidation(goneDeckText(part.Name, "is no longer one of this campaign's decks; choose it again in the Workshop"))
		}
		return "", err
	}
	// The Director's own draw: among the cards that fit now, at most five in
	// the shown spot, and the deck's own empty-pile rule and message.
	drawn, err := svc.Draw(ctx, campaignID, deck.ID, v, decks.DrawInput{Count: count})
	if err != nil {
		return "", err
	}
	hero := heroName(s.heroNames(ctx, svc, campaignID, v), a.Scope)
	for i, copyID := range drawn.CopyIDs {
		// Each deal is the Director's deal: the hand, the bell, card.drawn
		// and who may see the face follow the deck's own rules.
		if _, err := svc.Act(ctx, campaignID, deck.ID, copyID, deckVerbDeal, v, decks.ActInput{To: "hand", HeroID: a.Scope}); err != nil {
			s.putBack(ctx, svc, campaignID, deck.ID, v, drawn.CopyIDs[i:])
			if i == 0 {
				return "", err
			}
			return "", apperror.NewConflict(fmt.Sprintf("Dealt %s from %s to %s's hand, then stopped: %s", cardWord(i), deck.Name, hero, plainDeckError(err)))
		}
	}
	line := fmt.Sprintf("Dealt %s from %s to %s's hand.", cardWord(len(drawn.CopyIDs)), deck.Name, hero)
	if len(drawn.CopyIDs) < count {
		line = strings.TrimSuffix(line, ".") + fmt.Sprintf("; the pile held only %d.", len(drawn.CopyIDs))
	}
	return line, nil
}

// putBack returns copies a draw took but could not deal, so a failed rule
// leaves nothing waiting in the shown spot.
func (s *decksStory) putBack(ctx context.Context, svc decks.Service, campaignID, deckID string, v decks.Viewer, copyIDs []string) {
	for _, id := range copyIDs {
		if _, err := svc.Act(ctx, campaignID, deckID, id, deckVerbPutBack, v, decks.ActInput{}); err != nil {
			slog.Warn("decks: a drawn card could not be put back", slog.String("campaign_id", campaignID), slog.String("deck_id", deckID), slog.Any("error", err))
		}
	}
}

// heroNames maps hero pages to their names for the result line; an empty map
// when the roster cannot be read, which names the hero "a hero".
func (s *decksStory) heroNames(ctx context.Context, svc decks.Service, campaignID string, v decks.Viewer) map[string]string {
	out := map[string]string{}
	r, err := svc.Roster(ctx, campaignID, v)
	if err != nil {
		return out
	}
	for _, h := range r.Heroes {
		out[h.ID] = h.Name
	}
	return out
}

// deckByID reads one deck as a Director.
func deckByID(ctx context.Context, svc decks.Service, campaignID string, v decks.Viewer, id string) (decks.DirectorDeck, error) {
	view, err := svc.Get(ctx, campaignID, id, v)
	if err != nil {
		return decks.DirectorDeck{}, err
	}
	d, ok := view.(decks.DirectorDeck)
	if !ok {
		return decks.DirectorDeck{}, apperror.NewValidation("Card decks are not available right now.")
	}
	return d, nil
}

// goneDeckText names a deck the part can no longer reach by the name it kept.
func goneDeckText(name, rest string) string {
	if name == "" {
		return "That deck " + rest + "."
	}
	return "The deck " + name + " " + rest + "."
}

// plainDeckError is an owner's own message, never a raw internal one.
func plainDeckError(err error) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code < 500 {
		return ae.Message
	}
	return "something went wrong."
}

func cardWord(n int) string {
	if n == 1 {
		return "1 card"
	}
	return fmt.Sprintf("%d cards", n)
}
