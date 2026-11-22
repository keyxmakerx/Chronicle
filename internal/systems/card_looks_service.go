package systems

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities/cardborders"
)

// CardLookReader is what the pick list reads: the owner's looks for one field.
type CardLookReader interface {
	// CardLooks returns the stored looks of a field keyed by entry key.
	CardLooks(ctx context.Context, campaignID, systemID, fieldKey string) (map[string]StoredLook, error)
}

// CardLookEntries is the slice of the entry service the owner's choice for a
// campaign's own entry goes through, so the normal validation runs on it.
type CardLookEntries interface {
	SetCardLook(ctx context.Context, campaignID string, actor EntryActor, id int64, border, colour string) (*SystemEntry, error)
}

// EntryLookForgetter removes the looks kept for the entries derived from a
// deleted campaign entry (a subclass key is <parent slug>--<name>), so none is
// left naming a card nothing can show. The card look service implements it.
type EntryLookForgetter interface {
	ForgetDerived(ctx context.Context, campaignID, systemID, parentSlug string) (int, error)
}

// WithEntryLooks makes deleting an entry delete the looks of its derived entries.
func WithEntryLooks(f EntryLookForgetter) EntryServiceOption {
	return func(s *systemEntryService) { s.looks = f }
}

// CardLookService keeps an owner's choice of card borders. It checks a whole
// batch before writing anything, so a bad item never leaves the rest half
// applied.
type CardLookService interface {
	CardLookReader
	EntryLookForgetter
	// SetEntries plugs in the entry service. It is set after construction
	// because that service in turn takes this one as its forgetter.
	SetEntries(e CardLookEntries)
	// Apply checks every change, then writes them. An item whose card is no
	// longer on the field's list is dropped (a stale draft), not an error.
	// Who may change a campaign's card borders is the caller's to check: Apply
	// writes campaign entries as a Director.
	Apply(ctx context.Context, campaignID, userID string, changes []CardLookChange) error
}

type cardLookService struct {
	repo    CardLookRepository
	choices CharacterChoiceService
	entries CardLookEntries
}

// NewCardLookService wires the service. choices is read as a Director sees the
// list, rules aside, to find which cards exist.
func NewCardLookService(repo CardLookRepository, choices CharacterChoiceService) CardLookService {
	return &cardLookService{repo: repo, choices: choices}
}

func (s *cardLookService) SetEntries(e CardLookEntries) { s.entries = e }

func (s *cardLookService) CardLooks(ctx context.Context, campaignID, systemID, fieldKey string) (map[string]StoredLook, error) {
	return s.repo.List(ctx, campaignID, systemID, fieldKey)
}

func (s *cardLookService) ForgetDerived(ctx context.Context, campaignID, systemID, parentSlug string) (int, error) {
	return s.repo.DeleteDerived(ctx, campaignID, systemID, parentSlug)
}

// lookOf is the look a border and colour name; ok is false for no border.
func lookOf(border, colour string) (cardborders.Look, bool) {
	if border == "" {
		return cardborders.Look{}, false
	}
	return cardborders.LookFrom(map[string]any{cardborders.PropBorder: border, cardborders.PropColour: colour})
}

// systemLookOf is the look the game system itself gives a package choice,
// whether or not an owner's look has since been laid over it.
func systemLookOf(c Choice) (cardborders.Look, bool) {
	if c.LookChanged {
		if c.SystemLook == nil {
			return cardborders.Look{}, false
		}
		return *c.SystemLook, true
	}
	return cardborders.LookFrom(c.Properties)
}

var errBadLookDraft = "The card borders in the draft are not valid."

// checkCardLookChange validates one item's shape and returns it normalised
// (colour lower-cased). It reads nothing from the database.
func checkCardLookChange(c CardLookChange) (CardLookChange, error) {
	if !ValidChoiceFieldKey(c.FieldKey) {
		return c, apperror.NewValidation(errBadLookDraft + " A card names a field that does not exist.")
	}
	if c.Source != ChoiceSourcePackage && c.Source != ChoiceSourceCampaign {
		return c, apperror.NewValidation(errBadLookDraft + " A card's source must be \"package\" or \"campaign\".")
	}
	if c.Key == "" || len(c.Key) > MaxCardLookKeyLen {
		return c, apperror.NewValidation(fmt.Sprintf("%s A card's key must be between 1 and %d characters.", errBadLookDraft, MaxCardLookKeyLen))
	}
	if c.Reset {
		if c.Source != ChoiceSourcePackage || c.Border != "" || c.Colour != "" {
			return c, apperror.NewValidation(errBadLookDraft + " Only a game system's card can be reset, and a reset names no border.")
		}
		return c, nil
	}
	if c.Border != "" {
		if _, ok := cardborders.Get(c.Border); !ok {
			return c, apperror.NewValidation(errBadLookDraft + " A border is not in the library.")
		}
	}
	if c.Border == "" && c.Colour != "" {
		return c, apperror.NewValidation(errBadLookDraft + " A card with no border has no colour.")
	}
	if c.Colour != "" {
		if !cardborders.ValidColour(c.Colour) {
			return c, apperror.NewValidation(errBadLookDraft + " A colour must be written as # and six hex digits, for example #c4243a.")
		}
		c.Colour = strings.ToLower(c.Colour)
	}
	return c, nil
}

func (s *cardLookService) Apply(ctx context.Context, campaignID, userID string, changes []CardLookChange) error {
	if len(changes) == 0 {
		return nil
	}
	if len(changes) > MaxCardLookChanges {
		return apperror.NewValidation("Too many card borders were changed at once.")
	}
	type ident struct{ field, source, key string }
	seen := make(map[ident]bool, len(changes))
	checked := make([]CardLookChange, 0, len(changes))
	for _, c := range changes {
		n, err := checkCardLookChange(c)
		if err != nil {
			return err
		}
		id := ident{n.FieldKey, n.Source, n.Key}
		if seen[id] {
			return apperror.NewValidation(errBadLookDraft + " A card appears twice.")
		}
		seen[id] = true
		checked = append(checked, n)
	}

	// Which cards exist is read once per field, as a Director sees the list.
	type listed struct {
		systemID string
		cards    map[ident]Choice
	}
	lists := map[string]*listed{}
	for _, c := range checked {
		if lists[c.FieldKey] != nil {
			continue
		}
		l, err := s.choices.CharacterChoiceList(WithChoiceUnlimited(WithChoiceViewer(ctx, true)), campaignID, c.FieldKey)
		if err != nil {
			return err
		}
		lt := &listed{systemID: l.SystemID, cards: make(map[ident]Choice, len(l.Choices))}
		for _, ch := range l.Choices {
			lt.cards[ident{c.FieldKey, ch.Source, ch.Key}] = ch
		}
		lists[c.FieldKey] = lt
	}

	var upserts, deletes []CardLookRow
	var entryChanges []struct {
		id     int64
		border string
		colour string
	}
	for _, c := range checked {
		lt := lists[c.FieldKey]
		card, ok := lt.cards[ident{c.FieldKey, c.Source, c.Key}]
		if !ok || lt.systemID == "" {
			continue
		}
		row := CardLookRow{CampaignID: campaignID, SystemID: lt.systemID, FieldKey: c.FieldKey, EntryKey: c.Key}
		if c.Source == ChoiceSourceCampaign {
			if card.Status != "" && card.Status != EntryStatusAccepted {
				return apperror.NewValidation("Accept this entry first: a card's border is its writer's until a Director accepts the entry.")
			}
			// The key of a campaign entry is its id; EntryID is set only on the
			// writer's own entries, so it cannot be relied on here.
			entryID, perr := strconv.ParseInt(card.Key, 10, 64)
			if perr != nil || entryID <= 0 {
				continue
			}
			entryChanges = append(entryChanges, struct {
				id     int64
				border string
				colour string
			}{entryID, c.Border, c.Colour})
			continue
		}
		if c.Reset {
			deletes = append(deletes, row)
			continue
		}
		// A choice equal to the game system's own look is no choice: storing
		// a copy would pin the card to today's package look.
		chosen, chosenOK := lookOf(c.Border, c.Colour)
		own, ownOK := systemLookOf(card)
		if chosenOK == ownOK && chosen == own {
			deletes = append(deletes, row)
			continue
		}
		row.Border, row.Colour, row.UpdatedBy = c.Border, c.Colour, userID
		upserts = append(upserts, row)
	}

	if err := s.repo.Apply(ctx, upserts, deletes); err != nil {
		return err
	}
	if len(entryChanges) == 0 {
		return nil
	}
	if s.entries == nil {
		return apperror.NewInternal(fmt.Errorf("card look service has no entry service"))
	}
	actor := EntryActor{UserID: userID, IsDirector: true}
	for _, ec := range entryChanges {
		if _, err := s.entries.SetCardLook(ctx, campaignID, actor, ec.id, ec.border, ec.colour); err != nil {
			// Package rows are already kept; re-applying the draft repeats them
			// harmlessly, so the whole draft stays for another try.
			slog.Warn("setting a campaign entry's card look failed", slog.Int64("entry_id", ec.id), slog.Any("error", err))
			return err
		}
	}
	return nil
}
