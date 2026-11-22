package decks

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// noticeSlide is the deck setting that slides the notice in.
const noticeSlide = "slide"

// The notice a Director sees when a card's moment comes (see "Cards that wait
// for their moment" in .ai.md). It is a server-rendered fragment fetched by
// the Table for Directors only; a player's browser never asks for it.

// momentsPage is what the notice fragment draws: every open moment (for the
// Table's count and its "Waiting on you" rows) and the one shown.
type momentsPage struct {
	Moments []MomentView
	// At is the index of the moment shown.
	At  int
	Now time.Time
	// Looks are the decks' looks by deck id, for the picked card.
	Looks map[string]Look
}

// cardLook is the look the picked card is drawn in: the deck's own, but never
// the tall plate, which carries no words; the notice always shows the compact
// card so the Director can read what was picked.
func (p momentsPage) cardLook(m MomentView) Look {
	l := p.Looks[m.DeckID]
	if l.Shape != "small" {
		l.Shape = "standard"
	}
	return l
}

func (p momentsPage) cur() *MomentView {
	if p.At < 0 || p.At >= len(p.Moments) {
		return nil
	}
	return &p.Moments[p.At]
}

// MomentsPanel handles GET /campaigns/:id/decks/moments/panel: the open
// moments as the Table draws them. ?at= names the moment to show. A non-Director
// is told the page is not there, before anything is read.
func (h *Handler) MomentsPanel(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	list, err := h.svc.Moments(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	p := momentsPage{Moments: list, Now: time.Now(), Looks: map[string]Look{}}
	if len(list) > 0 {
		decks, err := h.svc.List(c.Request().Context(), cid, v)
		if err != nil {
			return err
		}
		if dl, ok := decks.(DirectorList); ok {
			for _, d := range dl.Decks {
				p.Looks[d.ID] = d.Look
			}
		}
	}
	if at := c.QueryParam("at"); at != "" {
		for i, m := range list {
			if m.ID == at {
				p.At = i
			}
		}
	}
	return middleware.Render(c, http.StatusOK, MomentsPanel(p))
}

// slides says whether a moment slides the notice in: its deck asks for a
// notice and nothing about it is quiet.
func (m MomentView) slides() bool { return m.Notice == noticeSlide && !m.Quiet }

func (m MomentView) can(answer string) bool {
	for _, a := range m.Answers {
		if a == answer {
			return true
		}
	}
	return false
}

// title is the reading, said once in bold; a moment with no number behind it
// (only "the Director says so") says what came up instead.
func (m MomentView) title() string {
	if m.Reading != "" {
		return m.Reading + "."
	}
	return "A card's moment has come in " + m.DeckName + "."
}

// dealLabel words the deal button for where the card goes.
func (m MomentView) dealLabel() string {
	switch m.DeckKind {
	case KindHero:
		if m.Hero != "" {
			return "Deal to " + m.Hero
		}
		return "Deal it"
	case KindParty:
		if m.Hero != "" {
			return "Deal to " + m.Hero
		}
		return "Deal to the party"
	}
	return "Put it on the table"
}

func (m MomentView) inviteLabel() string {
	if m.Hero != "" {
		return "Let " + m.Hero + " draw"
	}
	return "Let them draw"
}

var countWordsList = []string{"no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}

func numberWord(n int) string {
	if n >= 0 && n < len(countWordsList) {
		return countWordsList[n]
	}
	return strconv.Itoa(n)
}

// pickNote says how the deck came to this card.
func (m MomentView) pickNote() string {
	if m.FitCount <= 1 {
		return "The only card that fits."
	}
	return "The deck drew this one from the " + numberWord(m.FitCount) + " that fit."
}

// ago says how long ago in a few plain words.
func ago(then, now time.Time) string {
	d := now.Sub(then)
	switch {
	case d < 2*time.Minute:
		return "a moment ago"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " minutes ago"
	case d < 2*time.Hour:
		return "an hour ago"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " hours ago"
	}
	return strconv.Itoa(int(d.Hours()/24)) + " days ago"
}

// fromLine is the quiet line under the reading: where it came from and how old
// it is when known, then how many cards fit.
func (p momentsPage) fromLine(m MomentView) string {
	var parts []string
	var read string
	switch {
	case m.ReadingSource == "layer" && m.ReadAt != nil:
		read = "With a transformation on, " + ago(*m.ReadAt, p.Now)
	case m.ReadingSource == "layer":
		read = "With a transformation on"
	case m.ReadAt != nil:
		read = "Changed " + ago(*m.ReadAt, p.Now)
	}
	if read != "" {
		parts = append(parts, read)
	}
	parts = append(parts, m.fitsIn())
	return strings.Join(parts, " · ")
}

// fitsIn says how many cards fit, and where.
func (m MomentView) fitsIn() string {
	n := m.FitCount
	noun, verb := "cards", "fit"
	if n == 1 {
		noun, verb = "card", "fits"
	}
	place := "in " + m.DeckName
	if m.DeckKind == KindHero && m.Hero != "" {
		place = "in " + possessive(m.Hero) + " deck"
	}
	return strconv.Itoa(n) + " " + noun + " " + place + " " + verb
}

// neighbour is the id of the moment a step away from the one shown, wrapping.
func (p momentsPage) neighbour(step int) string {
	n := len(p.Moments)
	if n == 0 {
		return ""
	}
	return p.Moments[((p.At+step)%n+n)%n].ID
}
