package decks

import (
	"context"
	"net/http"
	"testing"
)

// Each case builds a deck with one card, puts its copy in a starting spot and
// has someone apply a verb. The table covers the verbs from their zones by a
// Director, the owning player, another player and people outside the
// campaign, including wrong-zone cases.
func TestMoveTable(t *testing.T) {
	type start struct {
		zone, holder string
		faceUp       bool
		asked        bool
	}
	const (
		ok       = 0
		notFound = http.StatusNotFound
		forbid   = http.StatusForbidden
		conf     = http.StatusConflict
		invalid  = http.StatusUnprocessableEntity
	)
	tests := []struct {
		name       string
		kind       string
		verb       string
		start      start
		card       CreateCardInput
		settings   *SettingsInput
		actor      func(*fixture) Viewer
		in         ActInput
		wantCode   int
		wantZone   string
		wantHolder string
		wantFaceUp *bool
	}{
		// deal
		{name: "director deals shown to a hand", verb: "deal", start: start{zone: ZoneShown}, actor: dirOf, in: ActInput{To: "hand", HeroID: "h-tam"}, wantZone: ZoneHand, wantHolder: "h-tam", wantFaceUp: ptr(false)},
		{name: "deal arrives up when the deck says so", verb: "deal", start: start{zone: ZoneShown}, settings: &SettingsInput{Arrives: ptr("up")}, actor: dirOf, in: ActInput{To: "hand", HeroID: "h-tam"}, wantZone: ZoneHand, wantHolder: "h-tam", wantFaceUp: ptr(true)},
		{name: "director deals shown to the table", verb: "deal", start: start{zone: ZoneShown}, actor: dirOf, in: ActInput{To: "table"}, wantZone: ZonePlay, wantFaceUp: ptr(true)},
		{name: "deal from the pile is the wrong zone", verb: "deal", start: start{zone: ZonePile}, actor: dirOf, in: ActInput{To: "table"}, wantCode: conf},
		{name: "deal to a page outside the campaign", verb: "deal", start: start{zone: ZoneShown}, actor: dirOf, in: ActInput{To: "hand", HeroID: "h-elsewhere"}, wantCode: invalid},
		{name: "deal to a hand needs a hero", verb: "deal", start: start{zone: ZoneShown}, actor: dirOf, in: ActInput{To: "hand"}, wantCode: invalid},
		{name: "deal needs a destination", verb: "deal", start: start{zone: ZoneShown}, actor: dirOf, wantCode: invalid},
		{name: "player cannot see a shown copy", verb: "deal", start: start{zone: ZoneShown}, actor: tamOf, in: ActInput{To: "table"}, wantCode: notFound},
		{name: "visitor cannot see a shown copy", verb: "deal", start: start{zone: ZoneShown}, actor: visitorOf, in: ActInput{To: "table"}, wantCode: notFound},
		{name: "former member cannot see a shown copy", verb: "deal", start: start{zone: ZoneShown}, actor: formerOf, in: ActInput{To: "table"}, wantCode: notFound},
		// put back and another
		{name: "director puts the shown copy back", verb: "put-back", start: start{zone: ZoneShown}, actor: dirOf, wantZone: ZonePile},
		{name: "put back from a hand is the wrong zone", verb: "put-back", start: start{zone: ZoneHand, holder: "h-tam"}, actor: dirOf, wantCode: conf},
		{name: "owner cannot put back", verb: "put-back", start: start{zone: ZoneHand, holder: "h-tam"}, actor: tamOf, wantCode: forbid},
		{name: "draw another with only one copy returns the same", verb: "another", start: start{zone: ZoneShown}, actor: dirOf, wantZone: ZoneShown},
		{name: "another from the pile is the wrong zone", verb: "another", start: start{zone: ZonePile}, actor: dirOf, wantCode: conf},
		// turn
		{name: "director turns a card in a hand", verb: "turn", start: start{zone: ZoneHand, holder: "h-tam"}, actor: dirOf, wantZone: ZoneHand, wantHolder: "h-tam", wantFaceUp: ptr(true)},
		{name: "owner cannot turn a face-up card back down", verb: "turn", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, actor: tamOf, wantCode: conf},
		{name: "a Director turns a face-up card back down", verb: "turn", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, actor: dirOf, wantZone: ZoneHand, wantHolder: "h-tam", wantFaceUp: ptr(false)},
		{name: "a player turns a face-down card up", verb: "turn", start: start{zone: ZoneHand, holder: "h-tam"}, actor: tamOf, wantZone: ZoneHand, wantHolder: "h-tam", wantFaceUp: ptr(true)},
		{name: "a player cannot play a face-down card", verb: "play", start: start{zone: ZoneHand, holder: "h-tam"}, actor: tamOf, wantCode: conf},
		{name: "a player cannot play a face-down ask-first card either", verb: "play", start: start{zone: ZoneHand, holder: "h-tam"}, card: CreateCardInput{AskFirst: true}, actor: tamOf, wantCode: conf},
		{name: "a Director plays a face-down card", verb: "play", start: start{zone: ZoneHand, holder: "h-tam"}, actor: dirOf, wantZone: ZoneUsed},
		{name: "another player's hand is a 404", verb: "turn", start: start{zone: ZoneHand, holder: "h-tam"}, actor: ilseOf, wantCode: notFound},
		{name: "a scribe is a player here", verb: "turn", start: start{zone: ZoneHand, holder: "h-tam"}, actor: scribeOf, wantCode: notFound},
		{name: "visitor turning is a 404", verb: "turn", start: start{zone: ZoneHand, holder: "h-tam"}, actor: visitorOf, wantCode: notFound},
		{name: "turn in play is the wrong zone", verb: "turn", start: start{zone: ZonePlay, holder: "h-tam", faceUp: true}, actor: tamOf, wantCode: conf},
		// play and its fates
		{name: "play with the default fate goes to used", verb: "play", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, actor: tamOf, wantZone: ZoneUsed, wantFaceUp: ptr(true)},
		{name: "play with fate back returns to the pile", verb: "play", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, card: CreateCardInput{Fate: FateBack}, actor: tamOf, wantZone: ZonePile, wantFaceUp: ptr(false)},
		{name: "play with fate gone", verb: "play", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, card: CreateCardInput{Fate: FateGone}, actor: tamOf, wantZone: ZoneGone},
		{name: "play with fate play stays in front of the hero", verb: "play", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, card: CreateCardInput{Fate: FatePlay}, actor: tamOf, wantZone: ZonePlay, wantHolder: "h-tam", wantFaceUp: ptr(true)},
		{name: "the deck's fate applies when the card has none", verb: "play", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, settings: &SettingsInput{Fate: ptr(FateGone)}, actor: tamOf, wantZone: ZoneGone},
		{name: "the card's fate beats the deck's", verb: "play", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, settings: &SettingsInput{Fate: ptr(FateGone)}, card: CreateCardInput{Fate: FateBack}, actor: tamOf, wantZone: ZonePile},
		{name: "ask first waits in the hand", verb: "play", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, card: CreateCardInput{AskFirst: true}, actor: tamOf, wantZone: ZoneHand, wantHolder: "h-tam"},
		{name: "asking twice is a conflict", verb: "play", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true, asked: true}, card: CreateCardInput{AskFirst: true}, actor: tamOf, wantCode: conf},
		{name: "a Director plays an ask-first card at once", verb: "play", start: start{zone: ZoneHand, holder: "h-tam"}, card: CreateCardInput{AskFirst: true}, actor: dirOf, wantZone: ZoneUsed},
		{name: "another player cannot play", verb: "play", start: start{zone: ZoneHand, holder: "h-tam"}, actor: ilseOf, wantCode: notFound},
		{name: "play from the pile is a 404 for a player", verb: "play", start: start{zone: ZonePile}, actor: tamOf, wantCode: notFound},
		{name: "play from the pile is the wrong zone for a Director", verb: "play", start: start{zone: ZonePile}, actor: dirOf, wantCode: conf},
		// approve and turn down
		{name: "director approves an asked play", verb: "approve", start: start{zone: ZoneHand, holder: "h-tam", asked: true}, card: CreateCardInput{AskFirst: true}, actor: dirOf, wantZone: ZoneUsed},
		{name: "approve follows fate play", verb: "approve", start: start{zone: ZoneHand, holder: "h-tam", asked: true}, card: CreateCardInput{AskFirst: true, Fate: FatePlay}, actor: dirOf, wantZone: ZonePlay, wantHolder: "h-tam"},
		{name: "approve with nothing asked", verb: "approve", start: start{zone: ZoneHand, holder: "h-tam"}, actor: dirOf, wantCode: conf},
		{name: "owner cannot approve their own ask", verb: "approve", start: start{zone: ZoneHand, holder: "h-tam", asked: true}, actor: tamOf, wantCode: forbid},
		{name: "other player approving is a 404", verb: "approve", start: start{zone: ZoneHand, holder: "h-tam", asked: true}, actor: ilseOf, wantCode: notFound},
		{name: "director turns an ask down", verb: "turn-down", start: start{zone: ZoneHand, holder: "h-tam", asked: true}, actor: dirOf, wantZone: ZoneHand, wantHolder: "h-tam"},
		{name: "owner cannot turn down", verb: "turn-down", start: start{zone: ZoneHand, holder: "h-tam", asked: true}, actor: tamOf, wantCode: forbid},
		// take off
		{name: "owner takes a card off in front of their hero", verb: "take-off", start: start{zone: ZonePlay, holder: "h-tam", faceUp: true}, actor: tamOf, wantZone: ZoneUsed},
		{name: "fate play ends in used when taken off", verb: "take-off", start: start{zone: ZonePlay, holder: "h-tam", faceUp: true}, card: CreateCardInput{Fate: FatePlay}, actor: tamOf, wantZone: ZoneUsed},
		{name: "take off with fate back", verb: "take-off", start: start{zone: ZonePlay, holder: "h-tam", faceUp: true}, card: CreateCardInput{Fate: FateBack}, actor: tamOf, wantZone: ZonePile},
		{name: "take off with fate gone", verb: "take-off", start: start{zone: ZonePlay, holder: "h-tam", faceUp: true}, card: CreateCardInput{Fate: FateGone}, actor: tamOf, wantZone: ZoneGone},
		{name: "a player cannot clear the whole table", kind: KindParty, verb: "take-off", start: start{zone: ZonePlay, faceUp: true}, actor: tamOf, wantCode: forbid},
		{name: "a Director clears the table", verb: "take-off", start: start{zone: ZonePlay, faceUp: true}, actor: dirOf, wantZone: ZoneUsed},
		{name: "in front of another hero is a 404", verb: "take-off", start: start{zone: ZonePlay, holder: "h-tam", faceUp: true}, actor: ilseOf, wantCode: notFound},
		{name: "take off from a hand is the wrong zone", verb: "take-off", start: start{zone: ZoneHand, holder: "h-tam"}, actor: tamOf, wantCode: conf},
		// discard
		{name: "discard is off by default", verb: "discard", start: start{zone: ZoneHand, holder: "h-tam"}, actor: tamOf, wantCode: forbid},
		{name: "owner discards when the deck allows", verb: "discard", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, settings: &SettingsInput{Discard: ptr(true)}, actor: tamOf, wantZone: ZoneUsed},
		{name: "discard with fate play ends in used", verb: "discard", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, settings: &SettingsInput{Discard: ptr(true)}, card: CreateCardInput{Fate: FatePlay}, actor: tamOf, wantZone: ZoneUsed},
		{name: "a player cannot discard a face-down card", verb: "discard", start: start{zone: ZoneHand, holder: "h-tam"}, settings: &SettingsInput{Discard: ptr(true)}, actor: tamOf, wantCode: conf},
		{name: "nor one whose fate sends it back to the pile", verb: "discard", start: start{zone: ZoneHand, holder: "h-tam"}, settings: &SettingsInput{Discard: ptr(true)}, card: CreateCardInput{Fate: FateBack}, actor: tamOf, wantCode: conf},
		{name: "a Director discards regardless", verb: "discard", start: start{zone: ZoneHand, holder: "h-tam"}, actor: dirOf, wantZone: ZoneUsed},
		{name: "another player discarding is a 404", verb: "discard", start: start{zone: ZoneHand, holder: "h-tam"}, settings: &SettingsInput{Discard: ptr(true)}, actor: ilseOf, wantCode: notFound},
		// take back
		{name: "director takes back from a hand", verb: "take-back", start: start{zone: ZoneHand, holder: "h-tam", faceUp: true}, actor: dirOf, wantZone: ZonePile, wantFaceUp: ptr(false)},
		{name: "director takes back from play", verb: "take-back", start: start{zone: ZonePlay, holder: "h-tam", faceUp: true}, actor: dirOf, wantZone: ZonePile},
		{name: "director takes back from the table", verb: "take-back", start: start{zone: ZonePlay, faceUp: true}, actor: dirOf, wantZone: ZonePile},
		{name: "owner cannot take back", verb: "take-back", start: start{zone: ZoneHand, holder: "h-tam"}, actor: tamOf, wantCode: forbid},
		{name: "take back from used is the wrong zone", verb: "take-back", start: start{zone: ZoneUsed, faceUp: true}, actor: dirOf, wantCode: conf},
		{name: "unknown verb", verb: "peek", start: start{zone: ZoneHand, holder: "h-tam"}, actor: dirOf, wantCode: notFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			if tt.kind == "" {
				tt.kind = KindHero
			}
			deck := fx.deck("Hero deck", tt.kind, tt.settings)
			tt.card.Name = "Card"
			cardID := fx.card(deck, tt.card)
			copyID := fx.copiesOf(cardID)[0]
			fx.place(copyID, tt.start.zone, tt.start.holder, tt.start.faceUp)
			if tt.start.asked {
				now := longAgo
				fx.repo.copies[fx.repo.index(copyID)].AskedAt = &now
			}
			res, err := fx.svc.Act(context.Background(), camp, deck, copyID, tt.verb, tt.actor(fx), tt.in)
			// A copy back in the pile has a new id, so the card's only copy is found by card.
			copyID = fx.copiesOf(cardID)[0]
			if code(err) != tt.wantCode {
				t.Fatalf("code = %d (%v), want %d", code(err), err, tt.wantCode)
			}
			if tt.wantCode != ok {
				if got := fx.copy(copyID); got.Zone != tt.start.zone || got.HolderEntityID != tt.start.holder || got.FaceUp != tt.start.faceUp {
					t.Errorf("a refused move changed the copy: %+v", got)
				}
				if len(fx.cap.got) != 0 {
					t.Errorf("a refused move was announced: %v", fx.cap.got)
				}
				return
			}
			if res == nil || res.Deck == nil {
				t.Fatal("no result")
			}
			if tt.wantZone == ZonePile && res.CopyID != "" {
				t.Errorf("a copy in the pile still named by the result: %s", res.CopyID)
			}
			got := fx.copy(copyID)
			if got.Zone != tt.wantZone || got.HolderEntityID != tt.wantHolder {
				t.Errorf("copy = %s/%q, want %s/%q", got.Zone, got.HolderEntityID, tt.wantZone, tt.wantHolder)
			}
			if tt.wantFaceUp != nil && got.FaceUp != *tt.wantFaceUp {
				t.Errorf("faceUp = %v, want %v", got.FaceUp, *tt.wantFaceUp)
			}
			if tt.verb == "play" && tt.card.AskFirst && !tt.actor(fx).Director && got.AskedAt == nil {
				t.Error("asking left no mark")
			}
			if (tt.verb == "approve" || tt.verb == "turn-down") && got.AskedAt != nil {
				t.Error("the answer left the mark")
			}
			if len(fx.cap.got) != 1 {
				t.Errorf("announced %d moves, want 1: %v", len(fx.cap.got), fx.cap.got)
			}
		})
	}
}

func dirOf(fx *fixture) Viewer     { return fx.dir }
func tamOf(fx *fixture) Viewer     { return fx.tam }
func ilseOf(fx *fixture) Viewer    { return fx.ilse }
func scribeOf(fx *fixture) Viewer  { return fx.scribe }
func formerOf(fx *fixture) Viewer  { return fx.former }
func visitorOf(fx *fixture) Viewer { return fx.visitor }

func TestAnotherNeverReturnsTheSameCopyWhenOthersExist(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Event", KindEvent, nil)
	a := fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(2)})
	res, err := fx.svc.Draw(context.Background(), camp, deck, fx.dir, DrawInput{})
	fx.must(err)
	first := res.CopyID
	for i := 0; i < 5; i++ {
		res, err = fx.svc.Act(context.Background(), camp, deck, first, "another", fx.dir, ActInput{})
		fx.must(err)
		if res.CopyID == first {
			t.Fatalf("another drew the same copy %s", first)
		}
		first = res.CopyID
	}
	shown := 0
	for _, c := range fx.repo.copies {
		if c.Zone == ZoneShown {
			shown++
		}
	}
	if shown != 1 {
		t.Errorf("%d copies shown, want exactly 1", shown)
	}
	_ = a
}

func TestLostRaceIsAConflictAndSilent(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Hero deck", KindHero, nil)
	c := fx.card(deck, CreateCardInput{Name: "A"})
	cp := fx.copiesOf(c)[0]
	fx.place(cp, ZoneHand, "h-tam", false)
	fx.repo.failMoves = true
	_, err := fx.svc.Act(context.Background(), camp, deck, cp, "turn", fx.tam, ActInput{})
	if code(err) != http.StatusConflict {
		t.Fatalf("code = %d, want 409", code(err))
	}
	if len(fx.cap.got) != 0 {
		t.Errorf("announced a move that did not happen: %v", fx.cap.got)
	}
}

func TestLogKeepsNewest1000AndRecordsSeen(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Hero deck", KindHero, nil)
	c := fx.card(deck, CreateCardInput{Name: "A"})
	cp := fx.copiesOf(c)[0]
	fx.place(cp, ZoneHand, "h-tam", true)
	_, err := fx.svc.Act(context.Background(), camp, deck, cp, "take-back", fx.dir, ActInput{})
	fx.must(err)
	lines, _ := fx.repo.ListLog(context.Background(), camp, deck, 10)
	if len(lines) != 1 || !lines[0].Seen || lines[0].Action != ActTakeBack || lines[0].CardName != "A" {
		t.Fatalf("log = %+v, want one take-back line with seen", lines)
	}
	cp = fx.copiesOf(c)[0]
	fx.place(cp, ZoneHand, "h-tam", false)
	_, err = fx.svc.Act(context.Background(), camp, deck, cp, "take-back", fx.dir, ActInput{})
	fx.must(err)
	lines, _ = fx.repo.ListLog(context.Background(), camp, deck, 10)
	if lines[0].Seen {
		t.Error("a face-down take back was marked seen")
	}
	for i := 0; i < LogKept+5; i++ {
		fx.repo.writeLog(LogLine{DeckID: deck, Action: ActTurn})
	}
	if lines, _ = fx.repo.ListLog(context.Background(), camp, deck, 5000); len(lines) != LogKept {
		t.Errorf("kept %d lines, want %d", len(lines), LogKept)
	}
}
