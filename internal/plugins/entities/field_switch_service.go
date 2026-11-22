package entities

import (
	"context"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// FieldSwitchOffers returns the switch the campaign's enabled game system
// offers this category, or nil. The app layer provides it from the system's
// manifest.
type FieldSwitchOffers func(ctx context.Context, campaignID string, et *EntityType) *FieldSwitchOffer

// fieldSwitchTypes is the slice of the entity type store the switch reads.
type fieldSwitchTypes interface {
	FindByID(ctx context.Context, id int) (*EntityType, error)
}

// FieldSwitchService runs a game system's one-time field switch for a
// category: the offer, accepting it, "Not now", switching back, and the
// values it kept aside. Every write is the owner's (the routes require it).
type FieldSwitchService struct {
	repo   FieldSwitchRepository
	types  fieldSwitchTypes
	offers FieldSwitchOffers
}

// NewFieldSwitchService wires the switch to its store, the category store and
// the offer source. A nil offers source offers nothing.
func NewFieldSwitchService(repo FieldSwitchRepository, types fieldSwitchTypes, offers FieldSwitchOffers) *FieldSwitchService {
	return &FieldSwitchService{repo: repo, types: types, offers: offers}
}

// offerFor is the category's offer, or nil.
func (s *FieldSwitchService) offerFor(ctx context.Context, campaignID string, et *EntityType) *FieldSwitchOffer {
	if s == nil || s.offers == nil || et == nil {
		return nil
	}
	return s.offers(ctx, campaignID, et)
}

// View is what the owner sees for a category: nil when no system offers it a
// switch. A failed read shows nothing rather than failing the page.
func (s *FieldSwitchService) View(ctx context.Context, campaignID string, et *EntityType) *FieldSwitchView {
	offer := s.offerFor(ctx, campaignID, et)
	if offer == nil {
		return nil
	}
	sw, err := s.repo.Get(ctx, et.ID)
	if err != nil {
		slog.Warn("field switch: reading the answer failed", slog.Int("entity_type_id", et.ID), slog.Any("error", err))
		return nil
	}
	return switchView(et, *offer, sw)
}

// State is a category's stored answer, or nil.
func (s *FieldSwitchService) State(ctx context.Context, typeID int) (*FieldSwitch, error) {
	return s.repo.Get(ctx, typeID)
}

// category loads a category and checks it belongs to the campaign.
func (s *FieldSwitchService) category(ctx context.Context, campaignID string, typeID int) (*EntityType, error) {
	et, err := s.types.FindByID(ctx, typeID)
	if err != nil {
		return nil, err
	}
	if et.CampaignID != campaignID {
		return nil, apperror.NewNotFound("category not found")
	}
	return et, nil
}

// Accept switches a category to its system's fields. It refuses when no
// switch is offered or the category has already switched.
func (s *FieldSwitchService) Accept(ctx context.Context, campaignID string, typeID int, userID string) (int, error) {
	et, err := s.category(ctx, campaignID, typeID)
	if err != nil {
		return 0, err
	}
	offer := s.offerFor(ctx, campaignID, et)
	if offer == nil {
		return 0, apperror.NewBadRequest("this category has no game system fields to switch to")
	}
	return s.AcceptOffer(ctx, et, *offer, userID)
}

// AcceptOffer switches et to offer's fields. The preset applier calls it for
// a category that has no pages when the system is turned on, so a new
// campaign starts with the system's fields and is never asked.
func (s *FieldSwitchService) AcceptOffer(ctx context.Context, et *EntityType, offer FieldSwitchOffer, userID string) (int, error) {
	sw, err := s.repo.Get(ctx, et.ID)
	if err != nil {
		return 0, err
	}
	if sw != nil && sw.State == FieldSwitchSwitched {
		return 0, apperror.NewConflict("this category already uses these fields")
	}
	// The list to go back to is the one from before the first switch; after
	// a switch back the stored list is the current one again.
	next := &FieldSwitch{
		EntityTypeID:      et.ID,
		CampaignID:        et.CampaignID,
		SystemID:          offer.SystemID,
		PresetSlug:        offer.PresetSlug,
		State:             FieldSwitchSwitched,
		OldFields:         cloneFieldDefs(et.Fields),
		OldPresetCategory: et.PresetCategory,
		UpdatedBy:         userID,
	}
	category := offer.Category
	return s.repo.Switch(ctx, next, switchedFieldList(et.Fields, offer), &category, acceptMove(et.Fields, offer))
}

// RecordSwitched notes that a category made from the system's preset already
// has its fields, so the offer never shows for it. There is nothing to go
// back to.
func (s *FieldSwitchService) RecordSwitched(ctx context.Context, et *EntityType, offer FieldSwitchOffer) error {
	return s.repo.SaveState(ctx, &FieldSwitch{
		EntityTypeID: et.ID, CampaignID: et.CampaignID,
		SystemID: offer.SystemID, PresetSlug: offer.PresetSlug, State: FieldSwitchSwitched,
	})
}

// Back puts a switched category's old fields and their kept values back. The
// system's values are kept aside the same way, so switching again restores
// them.
func (s *FieldSwitchService) Back(ctx context.Context, campaignID string, typeID int, userID string) (int, error) {
	et, err := s.category(ctx, campaignID, typeID)
	if err != nil {
		return 0, err
	}
	sw, err := s.repo.Get(ctx, et.ID)
	if err != nil {
		return 0, err
	}
	if sw == nil || sw.State != FieldSwitchSwitched || sw.OldFields == nil {
		return 0, apperror.NewConflict("this category has nothing to switch back to")
	}
	offer := s.offerFor(ctx, campaignID, et)
	if offer == nil {
		// The system is off or no longer offers the switch: go back by the
		// fields the category has that the old list lacks.
		offer = &FieldSwitchOffer{SystemID: sw.SystemID, PresetSlug: sw.PresetSlug, Fields: fieldsNotIn(et.Fields, sw.OldFields)}
	}
	next := *sw
	next.State = FieldSwitchBack
	next.UpdatedBy = userID
	return s.repo.Switch(ctx, &next, switchedBackFieldList(et.Fields, sw.OldFields, *offer), sw.OldPresetCategory, backMove(sw.OldFields, *offer))
}

// fieldsNotIn is the fields of a whose keys b lacks.
func fieldsNotIn(a, b []FieldDefinition) []FieldDefinition {
	had := fieldKeySet(b)
	var out []FieldDefinition
	for _, f := range a {
		if !had[f.Key] {
			out = append(out, f)
		}
	}
	return out
}

// Dismiss records "Not now": the list stops offering; Fields settings still do.
func (s *FieldSwitchService) Dismiss(ctx context.Context, campaignID string, typeID int, userID string) error {
	et, err := s.category(ctx, campaignID, typeID)
	if err != nil {
		return err
	}
	offer := s.offerFor(ctx, campaignID, et)
	if offer == nil {
		return apperror.NewBadRequest("this category has no game system fields to switch to")
	}
	sw, err := s.repo.Get(ctx, et.ID)
	if err != nil {
		return err
	}
	if sw != nil {
		// Already answered: "Not now" only quiets an unanswered offer.
		return nil
	}
	return s.repo.SaveState(ctx, &FieldSwitch{
		EntityTypeID: et.ID, CampaignID: campaignID,
		SystemID: offer.SystemID, PresetSlug: offer.PresetSlug, State: FieldSwitchDismissed, UpdatedBy: userID,
	})
}

// SetAside is what the switch kept aside from one page, for the GM's fold:
// nil unless the page's category has switched.
func (s *FieldSwitchService) SetAside(ctx context.Context, et *EntityType, entityID string) []SetAsideValue {
	if s == nil || et == nil {
		return nil
	}
	kept, err := s.repo.SetAside(ctx, entityID)
	if err != nil {
		slog.Warn("field switch: reading kept values failed", slog.String("entity_id", entityID), slog.Any("error", err))
		return nil
	}
	if len(kept) == 0 {
		return nil
	}
	sw, err := s.repo.Get(ctx, et.ID)
	if err != nil || sw == nil {
		return nil
	}
	var theirs []FieldDefinition
	if offer := s.offerFor(ctx, et.CampaignID, et); offer != nil {
		theirs = offer.Fields
	}
	return setAsideValues(kept, sw.OldFields, et.Fields, theirs)
}
