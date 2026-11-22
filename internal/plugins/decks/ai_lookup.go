package decks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// decksLookup answers `what: decks`. What it says depends only on the actor,
// and each branch reads through one service call and nothing else, so the
// sight rules are the ones the screens already obey and test: a Director
// reads List, Get and Moments; a player's own key reads Hand, from the
// player types; the owner's "what players see" key reads Table.
type decksLookup struct{ *aiCore }

// tableViewerID stands in for a viewer who owns no hero. Table refuses an
// empty id (it needs a member), and the owner's own id could own a hero and
// so read a page that players may not open; no user has this id, so it
// matches nothing and reads as a player who has claimed no page.
const tableViewerID = "ai-players-view"

func (*decksLookup) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "decks", Aliases: []string{"deck", "cards", "card-decks"}, Keys: "`deck` (a deck's name, optional), `from` (with `deck`, the first card to list; a long deck is read in parts)",
		Brief: "the campaign's card decks. The owner's full read lists every deck, and with `deck` its rules, cards, where each copy is and any open moment; a player's own key and the owner's what-players-see key get only what that player's own screens show",
		Tool: aiplug.Tool{
			Name: "card_decks", Title: "Card decks", Did: "Looked at the card decks",
			Desc:       "The campaign's card decks. Without `deck`, every deck with who it is for and how many cards are in the pile, in hands, on the table, used and out of the game. With `deck` (a deck's name), its rules in words, its cards (words, kind, copies, the Director's note and each test for when it comes up, with whether it fits now), where every copy is, which card comes up next and is kept for last, and any moment waiting on the Director. A deck with many cards is read in parts: the answer says which cards it holds and what `from` to ask for next. On a key set to what players see, only the cards lying face up on the table. Decks and cards are made and changed with deck and card blocks in ask_for_changes; a deck can also be removed with `action: delete` (a card cannot), and the AI never deals, draws, plays, shuffles or gathers.",
			PlayerDesc: "The cards in your own hand: your heroes' cards and the party's hand. A face-up card shows its name, kind, words and the line printed on it; a face-down card is only \"a face-down card from\" its deck. Nothing about other players' hands, the piles or the table. `deck` narrows it to one deck.",
			Args: []aiplug.Arg{{Key: "deck", Desc: "A deck's name; leave out for every deck"},
				{Key: "from", Desc: "With `deck`, the number of the first card to list; leave out to start at the first"}},
		},
	}
}

func (l *decksLookup) Answer(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) (string, string, error) {
	chip := "Card decks"
	if !l.on(ctx, campaignID) {
		return chip, "", aiplug.BadRequestf("card decks are switched off in this campaign")
	}
	want := strings.TrimSpace(r.Str("deck"))
	if want == "" {
		want = strings.TrimSpace(r.Name)
	}
	// A member's own key is checked first: it reads the member's own hand even
	// if its role were ever raised, so the narrowest reading always wins.
	switch {
	case a.ReadsOwnNotes:
		return l.ownCards(ctx, campaignID, a, want)
	case a.CanAuthorDmOnly():
		from, _, err := r.Int("from")
		if err != nil || from < 0 {
			return chip, "", aiplug.BadRequestf("`from` is the number of the first card to list")
		}
		return l.directorAnswer(ctx, campaignID, a, want, from)
	}
	return l.tableAnswer(ctx, campaignID, want)
}

// ---- a Director ---------------------------------------------------------

var (
	drawsWords    = map[string]string{"directors": "only Directors draw", "players": "chosen players draw for themselves"}
	knowsWords    = map[string]string{"nothing": "players are told nothing about the pile", "count": "players are told how many cards are left", "kinds": "players are told how many are left of each kind", "faces": "players can read every card's face"}
	othersWords   = map[string]string{"nothing": "others see nothing of a card dealt to one hero", "back": "others see a back and a count", "face": "others read a card's face once its holder shows it"}
	arrivesWords  = map[string]string{"down": "a dealt card arrives face down", "up": "a dealt card arrives face up"}
	aiFateWords   = map[string]string{FateBack: "a played card goes back to the pile", FateUsed: "a played card goes to the used pile", FateGone: "a played card is out of the game", FatePlay: "a played card stays on the table in front of its hero"}
	emptyWords    = map[string]string{"reshuffle": "an empty pile reshuffles the used pile", "stop": "an empty pile stops the draw"}
	noticeWords   = map[string]string{"slide": "a card's moment slides in for the Director", "count": "a card's moment is counted for the Director", "quiet": "a card's moment is not watched"}
	deckKindWords = map[string]string{KindHero: "a hero deck", KindParty: "a party deck", KindEvent: "an event deck"}
)

func say(m map[string]string, v string) string {
	if w := m[v]; w != "" {
		return w
	}
	return v
}

// rulesIn says a deck's rules in a sentence.
func rulesIn(s Settings) string {
	parts := []string{say(drawsWords, s.Draws), say(knowsWords, s.Knows), say(othersWords, s.OthersSee), say(arrivesWords, s.Arrives),
		say(aiFateWords, s.Fate), say(emptyWords, s.Empty), say(noticeWords, s.Notice)}
	if s.Discard {
		parts = append(parts, "players may discard from their own hand")
	}
	return strings.Join(parts, "; ")
}

// lookIn says a deck's look as the keys a block writes.
func lookIn(l Look) string {
	parts := []string{"shape " + l.Shape, "border " + l.Border, "turn " + l.Turn, "window " + l.Window, "name_position " + l.Name,
		"lettering " + l.Lettering, "corner " + l.Corner, "marker " + l.Marker, "finish " + l.Finish,
		"back_pattern " + l.Back.Pattern, "back_sign " + l.Back.Sign}
	if l.Back.Colour != "" {
		parts = append(parts, "back_colour "+l.Back.Colour)
	}
	if l.Back.Words != "" {
		parts = append(parts, "back_words "+aiplug.Quote(l.Back.Words))
	}
	return strings.Join(parts, ", ")
}

func kindNameOf(l Look, key string) string {
	for _, k := range l.Kinds {
		if k.Key == key {
			return k.Name
		}
	}
	return ""
}

// copiesOf is how many physical cards a deck has.
func copiesOf(cards []DirectorCard) int {
	n := 0
	for _, c := range cards {
		n += c.Copies
	}
	return n
}

type deckNames struct {
	heroes  map[string]string
	members map[string]string
}

func (n deckNames) hero(id string) string {
	if name := n.heroes[id]; name != "" {
		return name
	}
	return "a hero page"
}

func (n deckNames) forWhom(d DirectorDeck) string {
	w := say(deckKindWords, d.Kind)
	if d.Kind == KindHero && d.HolderEntityID != "" {
		w += " for " + n.hero(d.HolderEntityID)
	}
	return w
}

func (l *decksLookup) directorAnswer(ctx context.Context, campaignID string, a aiplug.Actor, want string, from int) (string, string, error) {
	chip := "Card decks"
	v := directorViewer(a)
	view, err := l.svc.List(ctx, campaignID, v)
	if err != nil {
		return chip, "", err
	}
	list, ok := view.(DirectorList)
	if !ok {
		return chip, "", aiplug.BadRequestf("the decks could not be read")
	}
	names := deckNames{heroes: map[string]string{}, members: map[string]string{}}
	if ro, err := l.svc.Roster(ctx, campaignID, v); err == nil {
		for _, h := range ro.Heroes {
			names.heroes[h.ID] = h.Name
		}
		for _, m := range ro.Members {
			names.members[m.UserID] = m.Name
		}
	}
	moments, momErr := l.svc.Moments(ctx, campaignID, v)
	if want == "" {
		return l.directorList(list, names, moments, momErr)
	}
	var hit *DirectorDeck
	for i := range list.Decks {
		if aiplug.SameName(list.Decks[i].Name, want) {
			if hit != nil {
				return chip, "", aiplug.BadRequestf("more than one deck is called %s", aiplug.Quote(want))
			}
			hit = &list.Decks[i]
		}
	}
	if hit == nil {
		return chip, "", aiplug.BadRequestf("no deck called %q", want)
	}
	full, err := l.svc.Get(ctx, campaignID, hit.ID, v)
	if err != nil {
		return chip, "", err
	}
	d, ok := full.(DirectorDeck)
	if !ok {
		return chip, "", aiplug.BadRequestf("the deck could not be read")
	}
	return directorDeckText(d, names, moments, momErr, from)
}

func (l *decksLookup) directorList(list DirectorList, names deckNames, moments []MomentView, momErr error) (string, string, error) {
	open := map[string]bool{}
	for _, m := range moments {
		open[m.DeckID] = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Card decks (%d)\n\n", len(list.Decks))
	var lines []string
	for _, d := range list.Decks {
		c := d.Counts
		line := fmt.Sprintf("- %s (%s): %d in the pile, %d shown to you, %d in hands, %d on the table, %d used, %d out of the game.",
			d.Name, names.forWhom(d), c.Pile, c.Shown, c.Hand, c.Play, c.Used, c.Gone)
		if open[d.ID] {
			line += " A card's moment is waiting on you."
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		b.WriteString("None yet.\n")
	}
	aiplug.WriteLines(&b, lines)
	if momErr != nil {
		b.WriteString("\nWhether a card's moment is waiting could not be read just now.\n")
	}
	return fmt.Sprintf("Card decks · %d %s", len(list.Decks), plural(len(list.Decks), "deck", "decks")), b.String(), nil
}

// place is where one copy is, in words.
func (n deckNames) place(c DirectorCopy) string {
	switch c.Zone {
	case ZonePile:
		return "in the pile"
	case ZoneShown:
		return "shown to you"
	case ZoneHand:
		if c.HolderEntityID == "" {
			return "in the party's hand"
		}
		return "in " + possessive(n.hero(c.HolderEntityID)) + " hand"
	case ZonePlay:
		if c.HolderEntityID == "" {
			return "on the table"
		}
		return "in front of " + n.hero(c.HolderEntityID)
	case ZoneUsed:
		return "used"
	}
	return "out of the game"
}

// cardsPerAnswer is how many cards one answer lists. A card is up to six
// lines (itself, four tests, whether it fits), so this keeps the list inside
// what one answer may hold, and the rest is asked for with `from`.
const cardsPerAnswer = 40

func directorDeckText(d DirectorDeck, names deckNames, moments []MomentView, momErr error, from int) (string, string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%s)\n\n", aiplug.OneLine(d.Name, 120), names.forWhom(d))
	fmt.Fprintf(&b, "Rules: %s.\n\nLook: %s.\n", rulesIn(d.Settings), lookIn(d.Look))
	if len(d.Look.Kinds) > 0 {
		ks := make([]string, 0, len(d.Look.Kinds))
		for _, k := range d.Look.Kinds {
			ks = append(ks, fmt.Sprintf("%s (key %s, %s, %s)", k.Name, k.Key, k.Colour, k.Sign))
		}
		fmt.Fprintf(&b, "Card kinds: %s.\n", strings.Join(ks, ", "))
	}
	if len(d.Players) > 0 {
		ps := make([]string, 0, len(d.Players))
		for _, p := range d.Players {
			who := names.members[p.UserID]
			if who == "" {
				who = "a player"
			}
			if p.DrawsLeft != nil {
				who += fmt.Sprintf(" (%d %s left)", *p.DrawsLeft, plural(*p.DrawsLeft, "draw", "draws"))
			}
			ps = append(ps, who)
		}
		fmt.Fprintf(&b, "Chosen players: %s.\n", strings.Join(ps, ", "))
	}
	byID := map[string]DirectorCard{}
	for _, c := range d.Cards {
		byID[c.ID] = c
	}
	next, last := "none", "none"
	if c, ok := byID[d.NextCardID]; ok {
		next = c.Name
	}
	if c, ok := byID[d.LastCardID]; ok {
		last = c.Name
	}
	fmt.Fprintf(&b, "Comes up next: %s. Kept for last: %s.\n", next, last)

	fmt.Fprintf(&b, "\n### Cards (%d, %d copies)\n\n", len(d.Cards), copiesOf(d.Cards))
	first := max(from, 1)
	end := min(first-1+cardsPerAnswer, len(d.Cards))
	var lines []string
	for i := first - 1; i < end; i++ {
		lines = append(lines, cardLines(d, d.Cards[i])...)
	}
	switch {
	case len(d.Cards) == 0:
		b.WriteString("None yet.\n")
	case first > len(d.Cards):
		fmt.Fprintf(&b, "This deck has %d %s, so there is none from %d on.\n", len(d.Cards), plural(len(d.Cards), "card", "cards"), first)
	}
	aiplug.WriteLines(&b, lines)
	if len(lines) > 0 && (first > 1 || end < len(d.Cards)) {
		fmt.Fprintf(&b, "\nCards %d to %d of %d.", first, end, len(d.Cards))
		if end < len(d.Cards) {
			fmt.Fprintf(&b, " Ask again with `from: %d` for the next.", end+1)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n### Where the copies are\n\n")
	b.WriteString(copiesText(d, byID, names))

	for _, m := range moments {
		if m.DeckID != d.ID {
			continue
		}
		card := "a card"
		if m.Card != nil {
			card = m.Card.Name
		}
		b.WriteString("\n### The open moment\n\n")
		fmt.Fprintf(&b, "%s fits and the moment is %s", card, m.State)
		if m.Hero != "" {
			fmt.Fprintf(&b, "; it would go to %s", m.Hero)
		}
		if m.InvitedName != "" {
			fmt.Fprintf(&b, "; %s is invited to draw", m.InvitedName)
		}
		b.WriteString(".")
		if m.Reading != "" {
			b.WriteString(" " + aiplug.OneLine(m.Reading, 200))
		}
		b.WriteString("\n")
	}
	if momErr != nil {
		b.WriteString("\nWhether a card's moment is waiting could not be read just now.\n")
	}
	chip := fmt.Sprintf("Card decks · %s · %d %s", aiplug.OneLine(d.Name, 60), copiesOf(d.Cards), plural(copiesOf(d.Cards), "card", "cards"))
	return chip, b.String(), nil
}

// cardLines is one card as lines: the card, then each of its tests.
func cardLines(d DirectorDeck, c DirectorCard) []string {
	attrs := []string{fmt.Sprintf("%d %s", c.Copies, plural(c.Copies, "copy", "copies"))}
	if k := kindNameOf(d.Look, c.KindKey); k != "" {
		attrs = append([]string{"kind " + k}, attrs...)
	}
	if c.Rare {
		attrs = append(attrs, "rare")
	}
	if c.AskFirst {
		attrs = append(attrs, "ask first")
	}
	if c.Fate != "" {
		attrs = append(attrs, "after use: "+c.Fate)
	}
	if c.Picture != "" {
		attrs = append(attrs, "has a picture")
	}
	line := "- " + aiplug.OneLine(c.Name, 120) + " (" + strings.Join(attrs, ", ") + ")"
	if c.Words != "" {
		line += ": " + aiplug.OneLine(c.Words, 200)
	}
	if c.DirectorNote != "" {
		line += " Note for you: " + aiplug.OneLine(c.DirectorNote, 200)
	}
	out := []string{line}
	for _, w := range c.When {
		who := "only you"
		if w.Open {
			who = "on the card"
		}
		t := "  - comes up when " + w.Sentence + " (" + who + ")"
		if w.Broken {
			t += ", but it names something that is gone, so it can never hold"
		}
		out = append(out, t)
	}
	if len(c.When) > 0 {
		state := "  - fits now"
		if !c.FitsNow {
			state = "  - does not fit now: " + aiplug.OneLine(c.WhyNot, 160)
		}
		out = append(out, state)
	}
	return out
}

// copiesText says where every copy is: one line per place, cards by name.
func copiesText(d DirectorDeck, byID map[string]DirectorCard, names deckNames) string {
	type spot struct {
		label string
		cards map[string]int
		faces map[string]bool
	}
	spots := map[string]*spot{}
	var order []string
	for _, c := range d.Copies {
		label := names.place(c)
		s := spots[label]
		if s == nil {
			s = &spot{label: label, cards: map[string]int{}, faces: map[string]bool{}}
			spots[label] = s
			order = append(order, label)
		}
		name := byID[c.CardID].Name
		if c.Zone == ZoneHand || c.Zone == ZoneShown {
			if c.FaceUp {
				name += " (face up)"
			} else {
				name += " (face down)"
			}
		}
		s.cards[name]++
	}
	sort.Strings(order)
	var lines []string
	for _, label := range order {
		s := spots[label]
		cards := make([]string, 0, len(s.cards))
		for n, k := range s.cards {
			if k > 1 {
				n += fmt.Sprintf(" x%d", k)
			}
			cards = append(cards, n)
		}
		sort.Strings(cards)
		total := 0
		for _, k := range s.cards {
			total += k
		}
		lines = append(lines, fmt.Sprintf("- %s (%d): %s", strings.ToUpper(label[:1])+label[1:], total, strings.Join(cards, ", ")))
	}
	if len(lines) == 0 {
		return "No copies yet.\n"
	}
	var b strings.Builder
	aiplug.WriteLines(&b, lines)
	return b.String()
}

// ---- a player's own key -------------------------------------------------

// faceLine is one card as a player's screen shows it. A face-down card is
// its deck and nothing else; the answer never holds a card id, so nothing but
// the screen's own words can vary with which card it is.
func faceLine(h HandCard) string {
	if !h.Copy.FaceUp || h.Copy.Face == nil {
		return "- a face-down card from " + aiplug.OneLine(h.DeckName, 120)
	}
	f := h.Copy.Face
	line := "- " + aiplug.OneLine(f.Name, 120)
	if k := kindNameOf(h.Look, f.KindKey); k != "" {
		line += " (" + k + ")"
	}
	if f.Words != "" {
		line += ": " + aiplug.OneLine(f.Words, 300)
	}
	if f.When != "" {
		line += " " + aiplug.OneLine(f.When, 300)
	}
	return line
}

// noneNamed is the one answer for a deck the reader holds nothing from,
// whether it exists, is hidden from them or never was.
func noneNamed(want string) string {
	return "Nothing in your hand is from a deck called " + aiplug.Quote(aiplug.OneLine(want, 80)) + ".\n"
}

// ownCards reads the player's own hand through Hand and the player types it
// returns, and says nothing of other hands, the piles, the table or any
// moment. The one thing of a moment a player is told is an invitation to
// draw, which the Hand view already carries for the invited player only.
func (l *decksLookup) ownCards(ctx context.Context, campaignID string, a aiplug.Actor, want string) (string, string, error) {
	chip := "Card decks · Your cards"
	if a.UserID == "" {
		return chip, "", aiplug.BadRequestf("this key has no player behind it")
	}
	hand, err := l.svc.Hand(ctx, campaignID, Viewer{UserID: a.UserID, MemberRole: permissions.RolePlayer, VisibilityRole: a.Role})
	if err != nil {
		return chip, "", err
	}
	fromDeck := func(name string) bool { return want == "" || aiplug.SameName(name, want) }
	keep := func(h HandCard) bool { return fromDeck(h.DeckName) }
	var b strings.Builder
	title := "Your cards"
	if want != "" {
		title += " from " + aiplug.OneLine(want, 80)
	}
	fmt.Fprintf(&b, "## %s\n\n", title)
	total := 0
	var lines []string
	section := func(head string, cards []HandCard) {
		var mine []string
		for _, h := range cards {
			if keep(h) {
				mine = append(mine, faceLine(h))
			}
		}
		if len(mine) == 0 {
			return
		}
		total += len(mine)
		lines = append(lines, head)
		lines = append(lines, mine...)
	}
	for _, sp := range hand.Mine {
		who := sp.Name
		if who == "" {
			who = "your hero"
		}
		section(who+" holds:", sp.Hand)
		section("Lying in front of "+who+":", sp.Play)
	}
	for _, p := range hand.Party {
		section("The party's hand ("+aiplug.OneLine(p.DeckName, 80)+"):", p.Hand)
	}
	for _, d := range hand.Draws {
		if !d.Invited || !fromDeck(d.DeckName) {
			continue
		}
		into := ""
		if len(d.Heroes) > 0 {
			into = ", into " + possessive(d.Heroes[0].Name) + " hand"
		}
		lines = append(lines, "Your Director invites you to draw from "+aiplug.OneLine(d.DeckName, 120)+into+".")
	}
	switch {
	case len(lines) > 0:
		aiplug.WriteLines(&b, lines)
	case want != "":
		b.WriteString(noneNamed(want))
	default:
		b.WriteString("You hold no cards.\n")
	}
	return fmt.Sprintf("Card decks · %s · %d %s", title, total, plural(total, "card", "cards")), b.String(), nil
}

// ---- the owner's what-players-see key ------------------------------------

// tableAnswer is the cards lying face up on the table, which every member
// reads, through Table as a viewer who owns no hero.
func (l *decksLookup) tableAnswer(ctx context.Context, campaignID, want string) (string, string, error) {
	chip := "Card decks · On the table"
	t, err := l.svc.Table(ctx, campaignID, Viewer{UserID: tableViewerID, MemberRole: permissions.RolePlayer, VisibilityRole: permissions.RolePlayer})
	if err != nil {
		return chip, "", err
	}
	var b strings.Builder
	b.WriteString("## Cards on the table\n\n")
	var lines []string
	total := 0
	for _, d := range t.Decks {
		if want != "" && !aiplug.SameName(d.Name, want) {
			continue
		}
		for _, c := range d.Cards {
			total++
			line := "- " + aiplug.OneLine(c.Face.Name, 120) + " (" + aiplug.OneLine(d.Name, 120)
			if k := kindNameOf(d.Look, c.Face.KindKey); k != "" {
				line += ", " + k
			}
			line += ")"
			if c.Face.Words != "" {
				line += ": " + aiplug.OneLine(c.Face.Words, 300)
			}
			if c.Face.When != "" {
				line += " " + aiplug.OneLine(c.Face.When, 300)
			}
			lines = append(lines, line)
		}
	}
	switch {
	case len(lines) > 0:
		aiplug.WriteLines(&b, lines)
	case want != "":
		b.WriteString("No card of a deck called " + aiplug.Quote(aiplug.OneLine(want, 80)) + " is lying on the table.\n")
	default:
		b.WriteString("No card is lying on the table.\n")
	}
	return fmt.Sprintf("%s · %d %s", chip, total, plural(total, "card", "cards")), b.String(), nil
}
