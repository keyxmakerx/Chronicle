package decks

// story.go is how a move in a deck reaches the Homebrew Workshop's rules
// without this plugin importing another: decks declares what it would like to
// tell, internal/app implements it.

import (
	"context"
	"log/slog"
)

// CardEvent is what a rule may know about a move: names and ids of the deck
// and card, the hero page the card is with, and who did it. It never carries
// which copy moved, so a rule cannot tell one face-down copy from another.
type CardEvent struct {
	DeckID   string
	DeckName string
	CardID   string
	CardName string
	// HeroID is the hero page the card is with after a draw, or was in the
	// hand of when played; "" for the table or the party.
	HeroID string
	// By is whoever made the move: the player who drew or played, or the
	// Director who dealt, drew or let a played card through.
	By Viewer
}

// Story hears moves once they are stored. It is told after the move's
// transaction has committed and the deck's lock is released, so it can neither
// fail nor hold up the move; whatever it does with the event is its own
// business, and nothing about the event is sent to any browser from here.
type Story interface {
	// CardDrawn is told once per card that arrives where people play it from:
	// a hero's hand, the party's hand or the table. A Director's draw to the
	// shown spot, "another" and a put back are the Director looking and tell
	// nothing; the deal that follows does.
	CardDrawn(ctx context.Context, campaignID string, e CardEvent)
	// CardPlayed is told when a card is really played: for an "ask first"
	// card that is when a Director lets it through, not when the player asks.
	CardPlayed(ctx context.Context, campaignID string, e CardEvent)
}

// WithStory sets who is told of draws, deals and plays. Without it, or with
// nil, nothing is told and moves behave exactly as before.
func WithStory(st Story) Option { return func(s *service) { s.story = st } }

// tell hands one event to the story. A panic in the listener is caught and
// logged here, because the move it follows has already been stored and
// answered for; the listener's error handling is its own.
func (s *service) tell(ctx context.Context, campaignID string, played bool, d Deck, card Card, heroID string, by Viewer) {
	if s.story == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("decks: the story panicked", slog.Any("panic", r), slog.String("campaign_id", campaignID))
		}
	}()
	e := CardEvent{DeckID: d.ID, DeckName: d.Name, CardID: card.ID, CardName: card.Name, HeroID: heroID, By: by}
	if played {
		s.story.CardPlayed(ctx, campaignID, e)
		return
	}
	s.story.CardDrawn(ctx, campaignID, e)
}
