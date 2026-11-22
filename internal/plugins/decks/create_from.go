package decks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Two more ways to start a deck besides a style: copy another deck of the
// campaign, or turn one of its rolling tables into cards. Both go through the
// same checks as a deck and a card made by hand, and store the deck and every
// card in one transaction.

// CreateFromInput names the new deck and exactly one source: a deck to copy or
// a rolling table to read.
type CreateFromInput struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	HolderEntityID string `json:"holderEntityId"`
	DeckID         string `json:"deckId"`
	TableID        string `json:"tableId"`
}

// CardTests is a card that lost some of its tests in a copy.
type CardTests struct {
	Card  string `json:"card"`
	Count int    `json:"count"`
}

// FromReport says what a new deck could not take from its source. Notes are
// the same facts as sentences, ready to show once on the new deck's page.
type FromReport struct {
	// TestsLeftOff lists the cards whose tests the new deck cannot hold.
	TestsLeftOff []CardTests `json:"testsLeftOff"`
	// PicturesLeftOff names the cards whose picture can no longer be used.
	PicturesLeftOff []string `json:"picturesLeftOff"`
	// DayLines counts table lines with day conditions, made into cards without.
	DayLines int `json:"dayLines"`
	// Skipped counts table lines that could not become a card.
	Skipped int `json:"skipped"`
	// Merged counts lines whose name an earlier line had, so they added copies.
	Merged int `json:"merged"`
	// NotMade counts usable lines left out because a deck is full.
	NotMade int `json:"notMade"`
	// LinesUsed counts the table's usable lines that went into cards, which is
	// more than Made when lines of one name became one card.
	LinesUsed int `json:"linesUsed"`
	// Made is how many cards the new deck has.
	Made  int      `json:"made"`
	Notes []string `json:"notes"`
}

// TableChoices is what the New deck page offers under "From a rolling table".
type TableChoices struct {
	// Available is false while the rolling tables plugin cannot be read.
	Available bool
	Tables    []TableSummary
}

func (s *service) RollingTables(ctx context.Context, campaignID string, v Viewer) (TableChoices, error) {
	if err := requireDirector(v); err != nil {
		return TableChoices{}, err
	}
	if s.tables == nil {
		return TableChoices{}, nil
	}
	list, err := s.tables.List(ctx, campaignID)
	if err != nil {
		// A broken source reads as none to offer, not as a broken page.
		slog.Warn("decks: rolling tables could not be listed", slog.Any("error", err))
		return TableChoices{}, nil
	}
	return TableChoices{Available: true, Tables: list}, nil
}

func (s *service) CreateFrom(ctx context.Context, campaignID string, v Viewer, in CreateFromInput) (DeckView, FromReport, error) {
	if err := requireDirector(v); err != nil {
		return nil, FromReport{}, err
	}
	if (in.DeckID == "") == (in.TableID == "") {
		return nil, FromReport{}, errInvalid("choose one deck to copy or one rolling table")
	}
	name, n, err := s.checkNewDeck(ctx, campaignID, in.Name, in.Kind, in.HolderEntityID)
	if err != nil {
		return nil, FromReport{}, err
	}
	d := Deck{ID: uuid.NewString(), CampaignID: campaignID, Name: name, Kind: in.Kind, HolderEntityID: in.HolderEntityID, SortOrder: n}
	var cards []Card
	var rep FromReport
	if in.DeckID != "" {
		cards, rep, err = s.copyOf(ctx, campaignID, v, in.DeckID, &d)
	} else {
		cards, rep, err = s.fromTable(ctx, campaignID, in.TableID, &d)
	}
	if err != nil {
		return nil, FromReport{}, err
	}
	if err := s.repo.CreateDeckWithCards(ctx, d, cards); err != nil {
		switch {
		case errors.Is(err, errDuplicate):
			return nil, FromReport{}, errTaken("deck")
		case errors.Is(err, errCardDuplicate):
			return nil, FromReport{}, errInvalid("two of the cards would have the same name")
		case errors.Is(err, errTargetFull):
			return nil, FromReport{}, errInvalid(fmt.Sprintf("a deck can have at most %d cards", MaxCardsPerDeck))
		case errors.Is(err, errTargetCopies):
			return nil, FromReport{}, errInvalid(fmt.Sprintf("a deck can have at most %d copies in all", MaxCopiesPerDeck))
		}
		return nil, FromReport{}, apperrorInternal(err)
	}
	s.poked(campaignID)
	rep.Made = len(cards)
	rep.Notes = rep.sentences()
	view, err := s.reread(ctx, campaignID, d.ID, v)
	if err != nil {
		return nil, FromReport{}, err
	}
	return view, rep, nil
}

// copyOf fills the new deck from a deck of this campaign: its look, its rules
// and its cards, all in the pile. Players, draws left, the next and last card,
// "the Director says so" and every copy's place stay behind. A deck of another
// campaign is the same 404 as one that does not exist, because the read is
// scoped to this campaign.
func (s *service) copyOf(ctx context.Context, campaignID string, v Viewer, srcID string, d *Deck) ([]Card, FromReport, error) {
	var rep FromReport
	src, err := s.newRequest(ctx, campaignID, v).directorDeck(srcID)
	if err != nil {
		return nil, rep, err
	}
	d.Settings = src.Settings
	// Who may draw stays behind with the chosen players: a copy that said
	// "chosen players draw" with nobody chosen would be a deck nobody can draw
	// from until the Director noticed.
	d.Settings.Draws = defaultSettings(d.Kind).Draws
	d.Look = src.Look
	d.Look.Kinds = append([]KindDef{}, src.Look.Kinds...)
	old, err := s.repo.ListCards(ctx, campaignID, src.ID)
	if err != nil {
		return nil, rep, apperrorInternal(err)
	}
	cards := make([]Card, 0, len(old))
	for _, o := range old {
		c := Card{ID: uuid.NewString(), DeckID: d.ID, CampaignID: campaignID, Name: o.Name, KindKey: o.KindKey, Words: o.Words,
			DirectorNote: o.DirectorNote, Picture: o.Picture, Corner: o.Corner, Rare: o.Rare, Copies: o.Copies, Fate: o.Fate, AskFirst: o.AskFirst}
		if err := checkCard(*d, &c); err != nil {
			return nil, rep, err
		}
		if c.Picture != "" {
			if err := s.checkPicture(ctx, campaignID, c.Picture); err != nil {
				if !isRefusal(err) {
					return nil, rep, err
				}
				c.Picture = ""
				rep.PicturesLeftOff = append(rep.PicturesLeftOff, c.Name)
			}
		}
		// Each test goes through the one check against the new deck, alone, so
		// one the deck cannot hold never costs the card its others.
		left := 0
		for _, t := range o.When {
			w, err := s.ValidateWhen(ctx, campaignID, *d, []WhenTest{t})
			if err != nil {
				if !isRefusal(err) {
					return nil, rep, err
				}
				left++
				continue
			}
			c.When = append(c.When, w...)
		}
		c.WhenOpen = derivedOpen(c.When)
		if left > 0 {
			rep.TestsLeftOff = append(rep.TestsLeftOff, CardTests{Card: c.Name, Count: left})
		}
		cards = append(cards, c)
	}
	return cards, rep, nil
}

// fromTable fills the new deck, in the Plain look, with a card for each usable
// line of a rolling table. Day conditions do not carry over: a card cannot yet
// wait for a day.
func (s *service) fromTable(ctx context.Context, campaignID, tableID string, d *Deck) ([]Card, FromReport, error) {
	var rep FromReport
	if s.tables == nil {
		return nil, rep, errInvalid("Rolling tables are not available right now.")
	}
	if !validTableID(tableID) {
		return nil, rep, errNotFound("table")
	}
	lines, _, ok, err := s.tables.Lines(ctx, campaignID, tableID)
	if err != nil {
		return nil, rep, apperrorInternal(err)
	}
	if !ok {
		return nil, rep, errNotFound("table")
	}
	d.Settings = defaultSettings(d.Kind)
	d.Look = defaultLook()

	// First the lines that can become cards, in order.
	type usable struct {
		card Card
		days bool
	}
	var use []usable
	for _, l := range lines {
		if l.Weight <= 0 || math.IsNaN(l.Weight) {
			rep.Skipped++
			continue
		}
		copies := int(math.Round(l.Weight))
		if copies < 1 {
			copies = 1
		}
		if copies > MaxCopiesPerCard {
			copies = MaxCopiesPerCard
		}
		c := Card{ID: uuid.NewString(), DeckID: d.ID, CampaignID: campaignID, Name: l.Name, Words: l.Brief, Copies: copies}
		if strings.TrimSpace(c.Name) == "" || checkCard(*d, &c) != nil {
			rep.Skipped++
			continue
		}
		use = append(use, usable{c, l.HasDays})
	}
	if len(use) == 0 {
		return nil, rep, errInvalid("That table has no lines that can become cards.")
	}
	// Then as many as a deck can hold. A line named like an earlier one adds its
	// copies to that card, since a deck's cards have one name each.
	var cards []Card
	at := map[string]int{}
	total := 0
	rep.LinesUsed = len(use)
	for i, u := range use {
		key := nameKey(u.card.Name)
		if j, dup := at[key]; dup {
			add := cards[j].Copies + u.card.Copies
			if add > MaxCopiesPerCard {
				add = MaxCopiesPerCard
			}
			add -= cards[j].Copies
			if total+add > MaxCopiesPerDeck {
				rep.NotMade += len(use) - i
				rep.LinesUsed = i
				break
			}
			cards[j].Copies += add
			total += add
			rep.Merged++
		} else {
			if len(cards) >= MaxCardsPerDeck || total+u.card.Copies > MaxCopiesPerDeck {
				rep.NotMade += len(use) - i
				rep.LinesUsed = i
				break
			}
			at[key] = len(cards)
			cards = append(cards, u.card)
			total += u.card.Copies
		}
		if u.days {
			rep.DayLines++
		}
	}
	return cards, rep, nil
}

// nameKey is a name as the database compares card names: without regard to
// case or accents, and with a sharp s as two. Two lines whose names differ only
// that way would be refused as one name by the cards table, so they are one
// card here.
func nameKey(name string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(name) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == 'ß' || r == 'ẞ':
			b.WriteString("ss")
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// validTableID is the shape a rolling table's id has, so nothing else is
// passed to the reader.
func validTableID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case i > 0 && (r >= '0' && r <= '9' || r == '-'):
		default:
			return false
		}
	}
	return true
}

// sentences says what was left off, one short sentence for each kind of thing.
func (r FromReport) sentences() []string {
	var out []string
	if n := len(r.TestsLeftOff); n > 0 {
		count := 0
		names := make([]string, 0, n)
		for _, t := range r.TestsLeftOff {
			count += t.Count
			names = append(names, t.Card)
		}
		out = append(out, fmt.Sprintf("%d %s left off because the new deck can't hold %s: %s.",
			count, plural(count, "condition was", "conditions were"), plural(count, "it", "them"), nameList(names)))
	}
	if n := len(r.PicturesLeftOff); n > 0 {
		out = append(out, fmt.Sprintf("%d %s left off because %s can't be used any more: %s.",
			n, plural(n, "picture was", "pictures were"), plural(n, "it", "they"), nameList(r.PicturesLeftOff)))
	}
	if n := r.DayLines; n > 0 {
		if n == 1 {
			out = append(out, "1 line only comes up on certain days. Cards can't wait for a day yet, so that card comes up any time.")
		} else {
			out = append(out, fmt.Sprintf("%d lines only come up on certain days. Cards can't wait for a day yet, so those cards come up any time.", n))
		}
	}
	if n := r.NotMade; n > 0 {
		out = append(out, fmt.Sprintf("A deck can't hold the whole table. The first %d %s made into cards and the last %d %s left out.",
			r.LinesUsed, plural(r.LinesUsed, "line was", "lines were"), n, plural(n, "line was", "lines were")))
	}
	if n := r.Merged; n > 0 {
		out = append(out, fmt.Sprintf("%d %s the same name as an earlier line, so %s added copies to that card instead of a card of %s own.",
			n, plural(n, "line has", "lines have"), plural(n, "it", "they"), plural(n, "its", "their")))
	}
	if n := r.Skipped; n > 0 {
		out = append(out, fmt.Sprintf("%d %s skipped: no name, a weight of 0, or too long for a card.", n, plural(n, "line was", "lines were")))
	}
	return out
}

// nameList names up to five cards and counts the rest.
func nameList(names []string) string {
	const show = 5
	if len(names) <= show {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:show], ", "), len(names)-show)
}

// isRefusal is a validation answer (a Director's input the check refused), as
// opposed to a failure of the system, which must stop the copy.
func isRefusal(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Type == "validation_error"
}
