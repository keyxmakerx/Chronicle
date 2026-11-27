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
