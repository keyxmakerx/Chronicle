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

func (p openedPage) base() string { return "/campaigns/" + p.CampaignID + "/decks" }

// handNote is the line under a hand: which decks have a card still face down,
// by deck name alone. Nothing about the card is in it.
func handNote(spots []HandSpot, party []HandParty) []string {
	var order []string
	count := map[string]int{}
	add := func(cards []HandCard) {
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
	for _, sp := range spots {
		add(sp.Hand)
	}
	for _, pt := range party {
		add(pt.Hand)
	}
	out := make([]string, 0, len(order))
	for _, name := range order {
		if n := count[name]; n == 1 {
			out = append(out, "A new card from "+name+". Tap it to turn it over.")
		} else {
			out = append(out, strconv.Itoa(n)+" new cards from "+name+". Tap one to turn it over.")
		}
	}
	return out
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
	return strconv.Itoa(*d.Pile) + " left"
}

func (d HandDraw) drawsLine() string {
	switch {
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
	out := ""
	for _, k := range d.Kinds {
		name := "No kind"
		if def, ok := kindOf(d.Look, k.KindKey); ok {
			name = def.Name
		}
		if out != "" {
			out += ", "
		}
		out += name + " " + strconv.Itoa(k.Count)
	}
	return out
}
