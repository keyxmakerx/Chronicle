package decks

// asks.go is how a player's ask to play an "ask first" card reaches the
// Director as one ask, answered in one place. Decks declares what it needs;
// internal/app puts the ask through the Homebrew Workshop's "Ask the
// Director" step, so the DM Screen's drawer, the Foundry module and the
// deck's own Table tab all answer the same waiting ask.

import (
	"context"
	"log/slog"
)

// PlayAsk is a waiting play as the Director is asked about it.
type PlayAsk struct {
	DeckID   string
	DeckName string
	CardID   string
	CardName string
	CopyID   string
	// HeroID is the hand it waits in, "" for the party's hand; HeroName is
	// that hero's name, or "The party".
	HeroID   string
	HeroName string
	// By is the player who asked.
	By Viewer
}

// AskAnswer is a Director's answer to a waiting play. Reply is a written
// reply the player gets with a turn-down.
type AskAnswer struct {
	Yes   bool
	Reply string
}

// Asks puts a waiting play to the Director and answers it.
type Asks interface {
	// Ask is told once a play is stored as waiting. An error leaves it
	// waiting on the deck's own page alone, where it is still answered.
	Ask(ctx context.Context, campaignID string, a PlayAsk) error
	// Answer answers the ask waiting for a copy, as the Director v; the ask
	// then comes back through AnswerAsk. found is false when no ask waits
	// for it (a play from before asks went this way, or one made while they
	// couldn't), and the deck answers it itself.
	Answer(ctx context.Context, campaignID, deckID, copyID string, v Viewer, yes bool) (found bool, err error)
	// Close ends the ask waiting for a copy that left its wait some other
	// way (a Director played, discarded, took back, kept or gathered it, or
	// removed its card), so no answer is offered for a card that moved. why
	// is the line the drawer keeps.
	Close(ctx context.Context, campaignID, deckID, copyID string, v Viewer, why string) error
}

// WithAsks sets where waiting plays are asked. Without it a play waits on the
// deck's page and is answered there.
func WithAsks(a Asks) Option { return func(s *service) { s.asks = a } }

// askOpened tells Asks of a stored waiting play. It never fails the play,
// which is stored and answered for.
func (s *service) askOpened(ctx context.Context, campaignID string, d Deck, card Card, c Copy, by Viewer) {
	if s.asks == nil {
		return
	}
	defer func() {
		if p := recover(); p != nil {
			slog.Error("decks: asking the Director panicked", slog.Any("panic", p), slog.String("campaign_id", campaignID))
		}
	}()
	a := PlayAsk{DeckID: d.ID, DeckName: d.Name, CardID: card.ID, CardName: card.Name, CopyID: c.ID, HeroID: c.HolderEntityID, HeroName: "The party", By: by}
	if c.HolderEntityID != "" {
		a.HeroName = "A hero"
		if h, ok, err := s.heroes.InCampaign(ctx, campaignID, c.HolderEntityID); err == nil && ok && h.Name != "" {
			a.HeroName = h.Name
		}
	}
	if err := s.asks.Ask(ctx, campaignID, a); err != nil {
		slog.Warn("decks: a waiting play was not put to the Director; it still waits on the deck's page",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
}

// goneWhy is the drawer's line for each move that ends a card's wait without
// an answer; a move not listed leaves the ask waiting.
var goneWhy = map[string]string{
	ActPlay:     "Closed: a Director played the card.",
	ActDiscard:  "Closed: the card was discarded.",
	ActTakeBack: "Closed: a Director took the card back.",
	ActKeep:     "Closed: a Director kept the card in another deck.",
	ActGather:   "Closed: a Director gathered the deck.",
	"removed":   "Closed: the card is no longer in the deck.",
}

// asksGone closes the asks of copies whose wait ended without an answer. It
// never fails the move, which is stored and answered for. The close is the
// deck's own bookkeeping, so it goes with a Director's standing even when
// the move was a player's (their own discard of the waiting card).
func (s *service) asksGone(ctx context.Context, campaignID, deckID string, copyIDs []string, v Viewer, why string) {
	if s.asks == nil || len(copyIDs) == 0 {
		return
	}
	v.Director = true
	defer func() {
		if p := recover(); p != nil {
			slog.Error("decks: closing an ask panicked", slog.Any("panic", p), slog.String("campaign_id", campaignID))
		}
	}()
	for _, id := range copyIDs {
		if err := s.asks.Close(ctx, campaignID, deckID, id, v, why); err != nil {
			slog.Warn("decks: an ask for a card that moved was not closed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		}
	}
}

// askedCopies lists a deck's copies waiting for an answer, of one card or
// of all (""), read before a change that takes them away. A failure reads as
// none: the ask then stays, and its answer says the card moved.
func (s *service) askedCopies(ctx context.Context, campaignID, deckID, cardID string) []string {
	if s.asks == nil {
		return nil
	}
	copies, err := s.repo.ListCopies(ctx, campaignID, deckID)
	if err != nil {
		return nil
	}
	var out []string
	for _, c := range copies {
		if c.AskedAt != nil && (cardID == "" || c.CardID == cardID) {
			out = append(out, c.ID)
		}
	}
	return out
}

// AnswerAsk lets a waiting play through or turns it down, as the Director
// answering the ask. It is the deck's own approve and turn-down, without
// asking Asks again.
func (s *service) AnswerAsk(ctx context.Context, campaignID, deckID, copyID string, v Viewer, a AskAnswer) (*ActionResult, error) {
	verb := verbTurnDown
	if a.Yes {
		verb = verbApprove
	}
	return s.act(ctx, campaignID, deckID, copyID, verb, v, ActInput{}, &a)
}
