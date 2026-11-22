package decks

import (
	"strconv"
)

// handPage is what the Hand leaf's fragment draws.
type handPage struct {
	CampaignID string
	View       *HandView
}

func (p handPage) base() string { return "/campaigns/" + p.CampaignID + "/decks" }

// stripPage is what a hero page's Cards part draws.
type stripPage struct {
	CampaignID string
	View       *HeroHandView
}

func (p stripPage) base() string { return "/campaigns/" + p.CampaignID + "/decks" }

// openedPage is what an opened card draws.
type openedPage struct {
	CampaignID string
	Card       *OpenedCard
}

// newLine says a deck has a card waiting face down in a hand: the head names
// the deck and the tail says what to do. Nothing about the card is in it.
type newLine struct{ Head, Tail string }

// newLines reads rows of a hand for cards still face down, by deck name alone,
// in the order the decks first appear.
func newLines(rows ...[]HandCard) []newLine {
	var order []string
	count := map[string]int{}
	for _, cards := range rows {
		for _, c := range cards {
			if c.Copy.FaceUp {
				continue
			}
			if count[c.DeckName] == 0 {
				order = append(order, c.DeckName)
			}
			count[c.DeckName]++
		}
	}
	out := make([]newLine, 0, len(order))
	for _, name := range order {
		if n := count[name]; n == 1 {
			out = append(out, newLine{"A new card from " + name, "Tap it to turn it over."})
		} else {
			out = append(out, newLine{strconv.Itoa(n) + " new cards from " + name, "Tap one to turn it over."})
		}
	}
	return out
}

// handNote is what the hand's status line announces: every deck with a card
// still face down, across the viewer's hands and the party's.
func handNote(spots []HandSpot, party []HandParty) []string {
	var rows [][]HandCard
	for _, sp := range spots {
		rows = append(rows, sp.Hand)
	}
	for _, pt := range party {
		rows = append(rows, pt.Hand)
	}
	lines := newLines(rows...)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.Head+". "+l.Tail)
	}
	return out
}

// pileLine is the caption under a pile a player may see: how many are left
// when the deck lets them know, and only that it is the pile when it does not.
func pileLine(pile *int) string {
	switch {
	case pile == nil:
		return "The pile"
	case *pile == 0:
		return "None left"
	}
	return strconv.Itoa(*pile) + " left"
}

// pileEmpty says a pile the player may count has nothing left, so its back is
// drawn as a faint place where the pile was.
func pileEmpty(pile *int) bool { return pile != nil && *pile == 0 }

// stateLine is what an opened card says about itself under the card.
func (c *OpenedCard) stateLine() string {
	switch {
	case c.Copy.Waiting && c.Shown:
		return "Waiting for your Director. Shown to the table."
	case c.Copy.Waiting:
		return "Waiting for your Director"
	case c.Shown:
		return "Shown to the table"
	}
	return ""
}

// position is "2 of 5" for the turn-over button of a face-down card.
func position(i, n int) string { return strconv.Itoa(i+1) + " of " + strconv.Itoa(n) }

// openLabel names the button that opens a face-up card.
func openLabel(c HandCard) string {
	if c.Copy.Face == nil {
		return "Open this card"
	}
	return "Open " + accessibleName(c.Look, *c.Copy.Face) + ", " + c.DeckName
}

// faceOrZero is the card's face; callers have checked it is face up.
func faceOrZero(c PlayerCopy) Face {
	if c.Face == nil {
		return Face{}
	}
	return *c.Face
}

// deckLine is a deck's pile as the deck lets the viewer read it.
func (d HandDraw) knownLine() string {
	if d.Pile == nil {
		return ""
	}
	return countWords(*d.Pile, "card in the pile", "cards in the pile")
}

func (d HandDraw) drawsLine() string {
	switch {
	case d.Invited:
		return "Your Director invites you to draw."
	case d.DrawsLeft == nil:
		return "You can draw as often as you like."
	case *d.DrawsLeft == 1:
		return "1 draw left"
	case *d.DrawsLeft <= 0:
		return "No draws left"
	}
	return strconv.Itoa(*d.DrawsLeft) + " draws left"
}

// kindsLine names the pile's kinds and how many of each, when the deck lets
// the viewer know them.
func (d HandDraw) kindsLine() string {
	out, named := "", false
	for _, k := range d.Kinds {
		name := "No kind"
		if def, ok := kindOf(d.Look, k.KindKey); ok {
			name, named = def.Name, true
		}
		if out != "" {
			out += ", "
		}
		out += name + " " + strconv.Itoa(k.Count)
	}
	// A deck with no kinds has nothing to add to the count of the pile.
	if !named {
		return ""
	}
	return out
}
