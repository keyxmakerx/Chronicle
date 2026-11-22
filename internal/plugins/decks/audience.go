package decks

import (
	"sort"
)

// Who is told about a move, live and on the bell. Both are decided here, from
// the move and the deck's settings alone, by two plain functions that tests
// run through every action: a message carries no card data, so the only thing
// it could leak is that it was sent, and who it is sent to is the rule that
// keeps that quiet.

// Audience is who a live "your decks view changed" message goes to. Directors
// always get it, so they are not listed. A message with neither Members nor
// Users is for Directors only.
type Audience struct {
	// Members is every member of the campaign.
	Members bool
	// Users are these players, sorted and without repeats.
	Users []string
}

// actEdit and actPlayers are changes that are not a move of a copy and are
// not logged: a card or deck edit, and a change to who a deck is chosen for.
const (
	actEdit    = "edited"
	actPlayers = "players"
)

// moveFacts is everything audience needs. It holds nothing about which card a
// copy is, so no card can change who is told.
type moveFacts struct {
	Action    string
	DeckKind  string
	Knows     string
	OthersSee string
	From, To  Place
	// FromOwner and ToOwner are the claimed players of the heroes holding the
	// copy before and after ("" when a place has no hero or the hero has no
	// player).
	FromOwner, ToOwner string
	// Seers are the players who may see the deck at all.
	Seers []string
	// FromViewers and ToViewers are the seers who may also open that hero's
	// page, so they read what the deck lets others see of its hand.
	FromViewers, ToViewers []string
	// TargetSeers are the players who see the pile count of the deck a card was
	// kept in; empty when that deck tells no one its count.
	TargetSeers []string
	// Before are the players who held cards of the deck before a gather.
	Before []string
}

type audienceSet struct {
	members bool
	users   map[string]bool
}

func (s *audienceSet) add(ids ...string) {
	for _, id := range ids {
		if id == "" {
			continue
		}
		if s.users == nil {
			s.users = map[string]bool{}
		}
		s.users[id] = true
	}
}

func (s *audienceSet) build() Audience {
	a := Audience{Members: s.members}
	if s.members {
		return a
	}
	for id := range s.users {
		a.Users = append(a.Users, id)
	}
	sort.Strings(a.Users)
	return a
}

// othersChange says whether a move can change what a player who does not hold
// the card reads of a hero's hand. Turning a card, asking to play it and an
// answer that leaves it in the hand change nothing they are sent; showing it
// changes what they read only where the deck lets others see faces.
func othersChange(f moveFacts) bool {
	switch f.Action {
	case ActTurn, ActAsk, ActTurnDown:
		return false
	case ActShow:
		return f.OthersSee == "face"
	}
	return true
}

// audience says which players, besides every Director, are told that a move
// changed their view. Anything that happens in the shown spot, anything a
// Director does to a pile, a note or a nudge, and anything on a deck a player
// cannot see is for Directors only.
func audience(f moveFacts) Audience {
	var s audienceSet
	switch f.Action {
	case actEdit, ActPutBack:
		return Audience{}
	case actPlayers:
		s.add(f.Seers...)
		return s.build()
	case ActKeep:
		if f.From.Zone == ZoneShown {
			return Audience{}
		}
		s.add(f.TargetSeers...)
	case ActShuffle:
		if f.DeckKind == KindParty {
			s.members = true
		} else if f.Knows != "nothing" {
			s.add(f.Seers...)
		}
		return s.build()
	case ActGather:
		s.add(f.Before...)
		if f.DeckKind == KindParty {
			s.members = true
		} else if f.Knows != "nothing" {
			s.add(f.Seers...)
		}
		return s.build()
	}
	shownSide := f.From.Zone == ZoneShown || f.To.Zone == ZoneShown
	sides := []struct {
		p       Place
		owner   string
		viewers []string
	}{{f.From, f.FromOwner, f.FromViewers}, {f.To, f.ToOwner, f.ToViewers}}
	for _, side := range sides {
		switch p := side.p; {
		case p.Zone == ZoneHand || p.Zone == ZonePlay:
			if p.HolderEntityID == "" {
				// The party's own hand, or the table: every member reads it.
				s.members = true
				continue
			}
			s.add(side.owner)
			if othersChange(f) && f.OthersSee != "nothing" {
				s.add(side.viewers...)
			}
		case p.Zone == ZoneUsed && f.DeckKind == KindParty:
			s.members = true
		case p.Zone == ZonePile && !shownSide && f.Knows != "nothing":
			s.add(f.Seers...)
		}
	}
	return s.build()
}

// Bell kinds. The words are the bell's notification types.
const (
	BellDealt     = "deck_card_dealt"
	BellTakenBack = "deck_card_taken_back"
	BellAsked     = "deck_card_asked"
	BellAnswered  = "deck_ask_answered"
)

// BellEntry is one notice for the bell.
type BellEntry struct {
	Users   []string
	Kind    string
	Message string
	Link    string
}

// bellFacts is what bellFor needs. Card is the card's name; bellFor says it
// only to someone who could already read it.
type bellFacts struct {
	Action     string
	CampaignID string
	DeckID     string
	DeckName   string
	Card       string
	HeroName   string
	From, To   Place
	// FromOwner and ToOwner are the players of the heroes before and after.
	FromOwner, ToOwner string
	Actor              string
	Directors          []string
}

// handLink opens the Table on the Hand leaf.
func handLink(campaignID string) string { return "/campaigns/" + campaignID + "?table=hand" }

// bellFor says what goes on the bell for a move, and to whom. A card dealt to
// a hero tells that hero's player, naming only the deck. A card taken back
// tells them too, naming the card only if it was face up to them. An ask tells
// the Directors; an answer tells the asker. Nothing else is rung, and nobody
// is rung about what they did themselves.
func bellFor(f bellFacts) []BellEntry {
	not := func(ids []string) []string {
		var out []string
		for _, id := range ids {
			if id != "" && id != f.Actor {
				out = append(out, id)
			}
		}
		return out
	}
	switch f.Action {
	case ActDeal:
		if f.To.Zone == ZoneHand && f.To.HolderEntityID != "" {
			if u := not([]string{f.ToOwner}); len(u) > 0 {
				return []BellEntry{{Users: u, Kind: BellDealt, Message: "A new card from " + f.DeckName, Link: handLink(f.CampaignID)}}
			}
		}
	case ActTakeBack:
		if (f.From.Zone == ZoneHand || f.From.Zone == ZonePlay) && f.From.HolderEntityID != "" {
			if u := not([]string{f.FromOwner}); len(u) > 0 {
				msg := "A card was taken back from " + f.DeckName
				if f.From.FaceUp && f.Card != "" {
					msg += ": " + f.Card
				}
				return []BellEntry{{Users: u, Kind: BellTakenBack, Message: msg, Link: handLink(f.CampaignID)}}
			}
		}
	case ActAsk:
		if u := not(f.Directors); len(u) > 0 {
			who := f.HeroName
			if who == "" {
				who = "A player"
			}
			return []BellEntry{{Users: u, Kind: BellAsked, Message: who + " asks to play " + f.Card + " from " + f.DeckName,
				Link: "/campaigns/" + f.CampaignID + "/decks/workshop/" + f.DeckID}}
		}
	case ActApprove, ActTurnDown:
		if f.From.Zone == ZoneHand && f.From.HolderEntityID != "" {
			if u := not([]string{f.FromOwner}); len(u) > 0 {
				msg := "Your Director let you play " + f.Card
				if f.Action == ActTurnDown {
					msg = "Your Director turned down your ask to play " + f.Card
				}
				return []BellEntry{{Users: u, Kind: BellAnswered, Message: msg, Link: handLink(f.CampaignID)}}
			}
		}
	}
	return nil
}
