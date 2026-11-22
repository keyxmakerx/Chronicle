package decks

import (
	"reflect"
	"testing"
)

// The people in the table below: a's hero is "ha", b may open ha's page, c may
// not (and sees only the deck), and a Director always hears everything, so no
// row lists one. "members" is every member of the campaign.
func TestAudienceForEveryAction(t *testing.T) {
	hand := func(face, open bool) Place {
		return Place{Zone: ZoneHand, HolderEntityID: "ha", FaceUp: face, Open: open}
	}
	partyHand := Place{Zone: ZoneHand}
	pile, shown := Place{Zone: ZonePile}, Place{Zone: ZoneShown}
	used, gone := Place{Zone: ZoneUsed, FaceUp: true}, Place{Zone: ZoneGone, FaceUp: true}
	onTable := Place{Zone: ZonePlay, FaceUp: true}
	inFront := Place{Zone: ZonePlay, HolderEntityID: "ha", FaceUp: true}
	none := Audience{}
	everyone := Audience{Members: true}
	users := func(u ...string) Audience { return Audience{Users: u} }

	rows := []struct {
		name                       string
		action, kind, knows, other string
		from, to                   Place
		want                       Audience
		target                     []string
		before                     []string
	}{
		// A Director's draw, put back and another: the shown spot, Directors only.
		{"director draw", ActDraw, KindHero, "count", "back", pile, shown, none, nil, nil},
		{"director draw, party deck", ActDraw, KindParty, "faces", "face", pile, shown, none, nil, nil},
		{"put back", ActPutBack, KindEvent, "count", "back", shown, pile, none, nil, nil},
		{"put back, party", ActPutBack, KindParty, "count", "back", shown, pile, none, nil, nil},
		// A player's draw into a hero's hand.
		{"player draw, count and backs", ActDraw, KindHero, "count", "back", pile, hand(false, false), users("a", "b", "c"), nil, nil},
		{"player draw, no count, backs", ActDraw, KindHero, "nothing", "back", pile, hand(false, false), users("a", "b"), nil, nil},
		{"player draw, nothing known or seen", ActDraw, KindHero, "nothing", "nothing", pile, hand(false, false), users("a"), nil, nil},
		// A deal. The shown spot is part of the pile to a player, so a deal out
		// of it tells whoever reads the pile's count, exactly as a deal straight
		// from the pile does.
		{"deal to a hand, backs seen", ActDeal, KindHero, "count", "back", shown, hand(false, false), users("a", "b", "c"), nil, nil},
		{"deal to a hand, others see nothing", ActDeal, KindHero, "count", "nothing", shown, hand(false, false), users("a", "b", "c"), nil, nil},
		{"deal to a hand, no count, backs seen", ActDeal, KindHero, "nothing", "back", shown, hand(false, false), users("a", "b"), nil, nil},
		{"deal to a hand, nothing known or seen", ActDeal, KindHero, "nothing", "nothing", shown, hand(false, false), users("a"), nil, nil},
		{"deal straight from the pile", ActDeal, KindHero, "count", "nothing", pile, hand(false, false), users("a", "b", "c"), nil, nil},
		{"deal to the table, count known", ActDeal, KindEvent, "count", "nothing", shown, onTable, everyone, nil, nil},
		{"deal to a hand, face up", ActDeal, KindEvent, "nothing", "face", shown, hand(true, false), users("a", "b"), nil, nil},
		{"deal to the table", ActDeal, KindEvent, "nothing", "nothing", shown, onTable, everyone, nil, nil},
		{"deal to the party's hand", ActDeal, KindParty, "nothing", "nothing", shown, partyHand, everyone, nil, nil},
		// Turning is private to the holder; in the party's hand it is the party's.
		{"turn, backs seen", ActTurn, KindHero, "count", "back", hand(false, false), hand(true, false), users("a"), nil, nil},
		{"turn, faces seen", ActTurn, KindHero, "kinds", "face", hand(false, false), hand(true, false), users("a"), nil, nil},
		{"turn in the party's hand", ActTurn, KindParty, "count", "back", partyHand, Place{Zone: ZoneHand, FaceUp: true}, everyone, nil, nil},
		// Showing is a second step that others read only where faces are seen.
		{"show, others see faces", ActShow, KindHero, "count", "face", hand(true, false), hand(true, true), users("a", "b"), nil, nil},
		{"show, others see backs", ActShow, KindHero, "count", "back", hand(true, false), hand(true, true), users("a"), nil, nil},
		{"show, others see nothing", ActShow, KindHero, "count", "nothing", hand(true, false), hand(true, true), users("a"), nil, nil},
		// A play, by where it lands.
		{"play to used", ActPlay, KindHero, "count", "back", hand(true, false), used, users("a", "b"), nil, nil},
		{"play to used, party deck", ActPlay, KindParty, "count", "back", partyHand, used, everyone, nil, nil},
		{"play, fate gone", ActPlay, KindHero, "nothing", "back", hand(true, false), gone, users("a", "b"), nil, nil},
		{"play, fate back", ActPlay, KindHero, "count", "back", hand(true, false), pile, users("a", "b", "c"), nil, nil},
		{"play, fate back, no count", ActPlay, KindHero, "nothing", "nothing", hand(true, false), pile, users("a"), nil, nil},
		{"play, fate play", ActPlay, KindHero, "count", "back", hand(true, false), inFront, users("a", "b"), nil, nil},
		// An ask waits in place: the holder, or the party.
		{"ask", ActAsk, KindHero, "count", "face", hand(true, false), hand(true, false), users("a"), nil, nil},
		{"ask in the party's hand", ActAsk, KindParty, "count", "back", Place{Zone: ZoneHand, FaceUp: true}, Place{Zone: ZoneHand, FaceUp: true}, everyone, nil, nil},
		{"approved to used", ActApprove, KindHero, "count", "back", hand(true, false), used, users("a", "b"), nil, nil},
		{"approved to the table", ActApprove, KindHero, "count", "back", hand(true, false), inFront, users("a", "b"), nil, nil},
		{"turned down", ActTurnDown, KindHero, "count", "face", hand(true, false), hand(true, false), users("a"), nil, nil},
		// Leaving play and the hand.
		{"taken off from in front", ActTakeOff, KindHero, "count", "back", inFront, used, users("a", "b"), nil, nil},
		{"taken off the table", ActTakeOff, KindEvent, "count", "back", onTable, used, everyone, nil, nil},
		{"discarded", ActDiscard, KindHero, "count", "back", hand(true, false), used, users("a", "b"), nil, nil},
		{"discarded, party deck", ActDiscard, KindParty, "count", "back", partyHand, used, everyone, nil, nil},
		{"taken back", ActTakeBack, KindHero, "count", "back", hand(false, false), pile, users("a", "b", "c"), nil, nil},
		{"taken back, no count", ActTakeBack, KindHero, "nothing", "nothing", hand(true, false), pile, users("a"), nil, nil},
		{"taken back from play", ActTakeBack, KindHero, "kinds", "face", inFront, pile, users("a", "b", "c"), nil, nil},
		// Keep: from the shown spot is Directors only; from a hand it is the
		// holder's, and the target deck's players where it tells its count.
		{"kept from shown", ActKeep, KindEvent, "count", "back", shown, gone, none, []string{"c"}, nil},
		{"kept from a hand", ActKeep, KindHero, "count", "back", hand(false, false), gone, users("a", "b", "c"), []string{"c"}, nil},
		{"kept from a hand, target silent", ActKeep, KindHero, "count", "nothing", hand(true, false), gone, users("a"), nil, nil},
		// Shuffle and gather act on the whole deck.
		{"shuffle", ActShuffle, KindHero, "count", "back", used, pile, users("a", "b", "c"), nil, nil},
		{"shuffle, no count", ActShuffle, KindHero, "nothing", "back", used, pile, none, nil, nil},
		{"shuffle, party deck", ActShuffle, KindParty, "nothing", "back", used, pile, everyone, nil, nil},
		{"gather", ActGather, KindHero, "count", "back", used, pile, users("a", "b", "c"), nil, []string{"a"}},
		{"gather, no count", ActGather, KindHero, "nothing", "back", used, pile, users("a"), nil, []string{"a"}},
		{"gather, party deck", ActGather, KindParty, "nothing", "back", used, pile, everyone, nil, nil},
		// A change to the deck itself that players read tells whoever sees it.
		{"deck changed", actDeck, KindHero, "count", "back", Place{}, Place{}, users("a", "b", "c"), nil, nil},
	}
	covered := map[string]bool{}
	for _, row := range rows {
		covered[row.action] = true
		t.Run(row.name, func(t *testing.T) {
			f := moveFacts{Action: row.action, DeckKind: row.kind, Knows: row.knows, OthersSee: row.other, From: row.from, To: row.to,
				Seers: []string{"a", "b", "c"}, TargetSeers: row.target, Before: row.before}
			if row.from.HolderEntityID == "ha" {
				f.FromOwner, f.FromViewers = "a", []string{"a", "b"}
			}
			if row.to.HolderEntityID == "ha" {
				f.ToOwner, f.ToViewers = "a", []string{"a", "b"}
			}
			got := audience(f)
			if !reflect.DeepEqual(got, row.want) {
				t.Errorf("told %+v, want %+v", got, row.want)
			}
		})
	}
	for _, a := range []string{ActDraw, ActDeal, ActTurn, ActShow, ActPlay, ActAsk, ActApprove, ActTurnDown, ActTakeOff, ActDiscard,
		ActTakeBack, ActPutBack, ActKeep, ActShuffle, ActGather, actDeck} {
		if !covered[a] {
			t.Errorf("no row for the action %q", a)
		}
	}
}

// A player the move does not touch is never told: on a deck that tells no one
// its count, c and d see the deck but not ha's page, so a deal to ha reaches a
// and b only.
func TestAudienceNeverNamesWhoseCardOrWhichCard(t *testing.T) {
	f := moveFacts{Action: ActDeal, DeckKind: KindHero, Knows: "nothing", OthersSee: "face", Seers: []string{"a", "b", "c", "d"},
		From: Place{Zone: ZoneShown}, To: Place{Zone: ZoneHand, HolderEntityID: "ha"}, ToOwner: "a", ToViewers: []string{"a", "b"}}
	got := audience(f)
	if !reflect.DeepEqual(got, Audience{Users: []string{"a", "b"}}) {
		t.Errorf("told %+v", got)
	}
}

func TestBellEntries(t *testing.T) {
	hand := func(face bool) Place { return Place{Zone: ZoneHand, HolderEntityID: "ha", FaceUp: face} }
	base := bellFacts{CampaignID: "c1", DeckID: "d1", DeckName: "Orrin's boons", Card: "A favour owed", HeroName: "Orrin",
		FromOwner: "a", ToOwner: "a", Actor: "dir", Directors: []string{"dir", "dir2"}}
	cases := []struct {
		name   string
		mod    func(*bellFacts)
		users  []string
		kind   string
		msg    string
		isNone bool
	}{
		{"dealt to a hero names only the deck", func(f *bellFacts) { f.Action, f.From, f.To = ActDeal, Place{Zone: ZoneShown}, hand(false) },
			[]string{"a"}, BellDealt, "A new card from Orrin's boons", false},
		{"dealt to the table rings nobody", func(f *bellFacts) {
			f.Action, f.From, f.To = ActDeal, Place{Zone: ZoneShown}, Place{Zone: ZonePlay, FaceUp: true}
		},
			nil, "", "", true},
		{"dealt to a hero nobody plays rings nobody", func(f *bellFacts) {
			f.Action, f.From, f.To, f.ToOwner = ActDeal, Place{Zone: ZoneShown}, hand(false), ""
		},
			nil, "", "", true},
		{"dealt to your own hero is not rung", func(f *bellFacts) {
			f.Action, f.From, f.To, f.Actor = ActDeal, Place{Zone: ZoneShown}, hand(false), "a"
		},
			nil, "", "", true},
		{"taken back face down does not name the card", func(f *bellFacts) { f.Action, f.From, f.To = ActTakeBack, hand(false), Place{Zone: ZonePile} },
			[]string{"a"}, BellTakenBack, "A card was taken back from Orrin's boons", false},
		{"taken back face up names the card", func(f *bellFacts) { f.Action, f.From, f.To = ActTakeBack, hand(true), Place{Zone: ZonePile} },
			[]string{"a"}, BellTakenBack, "A card was taken back from Orrin's boons: A favour owed", false},
		{"taken back from play names the card", func(f *bellFacts) {
			f.Action, f.From, f.To = ActTakeBack, Place{Zone: ZonePlay, HolderEntityID: "ha", FaceUp: true}, Place{Zone: ZonePile}
		}, []string{"a"}, BellTakenBack, "A card was taken back from Orrin's boons: A favour owed", false},
		{"an ask rings the Directors but not the one who made it", func(f *bellFacts) { f.Action, f.From, f.To = ActAsk, hand(true), hand(true); f.Actor = "dir" },
			[]string{"dir2"}, BellAsked, "Orrin asks to play A favour owed from Orrin's boons", false},
		{"approved rings the asker", func(f *bellFacts) { f.Action, f.From, f.To = ActApprove, hand(true), Place{Zone: ZoneUsed} },
			[]string{"a"}, BellAnswered, "Your Director let you play A favour owed", false},
		{"turned down rings the asker", func(f *bellFacts) { f.Action, f.From, f.To = ActTurnDown, hand(true), hand(true) },
			[]string{"a"}, BellAnswered, "Your Director turned down your ask to play A favour owed", false},
		{"a turn rings nobody", func(f *bellFacts) { f.Action, f.From, f.To = ActTurn, hand(false), hand(true) }, nil, "", "", true},
		{"a show rings nobody", func(f *bellFacts) { f.Action, f.From, f.To = ActShow, hand(true), hand(true) }, nil, "", "", true},
		{"a play rings nobody", func(f *bellFacts) { f.Action, f.From, f.To = ActPlay, hand(true), Place{Zone: ZoneUsed} }, nil, "", "", true},
		{"a draw rings nobody", func(f *bellFacts) { f.Action, f.From, f.To = ActDraw, Place{Zone: ZonePile}, Place{Zone: ZoneShown} }, nil, "", "", true},
		{"a put back rings nobody", func(f *bellFacts) { f.Action, f.From, f.To = ActPutBack, Place{Zone: ZoneShown}, Place{Zone: ZonePile} }, nil, "", "", true},
		{"a gather rings nobody", func(f *bellFacts) { f.Action = ActGather }, nil, "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := base
			c.mod(&f)
			got := bellFor(f)
			if c.isNone {
				if len(got) != 0 {
					t.Fatalf("rang %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("rang %d entries: %+v", len(got), got)
			}
			e := got[0]
			if !reflect.DeepEqual(e.Users, c.users) || e.Kind != c.kind || e.Message != c.msg {
				t.Errorf("rang %+v, want %v %q %q", e, c.users, c.kind, c.msg)
			}
			if e.Kind != BellAsked && e.Link != "/campaigns/c1?table=hand" {
				t.Errorf("link = %q", e.Link)
			}
			if e.Kind == BellAsked && e.Link != "/campaigns/c1/decks/workshop/d1" {
				t.Errorf("link = %q", e.Link)
			}
		})
	}
}
