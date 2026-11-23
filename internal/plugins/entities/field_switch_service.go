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

// FieldSwitchDraft reports whether the owner's unsaved Customizer draft
// replaces a category's field list. The campaigns plugin keeps the draft; the
// app layer answers this from it.
type FieldSwitchDraft func(ctx context.Context, campaignID, userID string, typeID int) (bool, error)

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
	draft  FieldSwitchDraft
}

// NewFieldSwitchService wires the switch to its store, the category store and
// the offer source. A nil offers source offers nothing.
func NewFieldSwitchService(repo FieldSwitchRepository, types fieldSwitchTypes, offers FieldSwitchOffers) *FieldSwitchService {
	return &FieldSwitchService{repo: repo, types: types, offers: offers}
}

// SetDraft wires the Customizer draft check. Unwired, no draft is checked.
func (s *FieldSwitchService) SetDraft(d FieldSwitchDraft) {
	s.draft = d
}

// refuseDraft refuses a switch while the owner's Customizer draft holds a
// field list for the category: publishing it replaces the whole list, so it
// would put the fields from before the switch back over it. The draft lives
// on the server, so the page's own guard cannot see it from outside the
// Customizer, and the AI's request never passes that guard at all.
func (s *FieldSwitchService) refuseDraft(ctx context.Context, et *EntityType, userID string) error {
	if s.draft == nil {
		return nil
	}
	held, err := s.draft(ctx, et.CampaignID, userID, et.ID)
	if err != nil {
		return err
	}
	if held {
		return apperror.NewConflict("Publish or discard your Customizer changes to " + et.NamePlural + " first: publishing them would put the old fields back over the switch")
	}
	return nil
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
	if err := s.refuseDraft(ctx, et, userID); err != nil {
		return 0, err
	}
	return s.AcceptOffer(ctx, et, *offer, userID)
}

// AcceptOffer switches et to offer's fields. The preset applier calls it for
// a category that has no pages when the system is turned on, so a new
// campaign starts with the system's fields and is never asked.
func (s *FieldSwitchService) AcceptOffer(ctx context.Context, et *EntityType, offer FieldSwitchOffer, userID string) (int, error) {
	// The caller's copy may predate its own earlier writes (the preset
	// applier lists categories once per run); the switch is worked out from
	// the stored list, which the repository checks again under its lock.
	et, err := s.types.FindByID(ctx, et.ID)
	if err != nil {
		return 0, err
	}
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
	return s.repo.Switch(ctx, next, baseOf(sw, et), switchedFieldList(et.Fields, offer), &category, acceptMove(et.Fields, offer))
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
		// Only the system's own list tells its fields from ones the owner
		// added since; without it every later field would go too.
		return 0, apperror.NewConflict("turn the game system back on to switch " + et.NamePlural + " back: Chronicle needs its field list to tell its fields from the ones you added")
	}
	if err := s.refuseDraft(ctx, et, userID); err != nil {
		return 0, err
	}
	next := *sw
	next.State = FieldSwitchBack
	next.UpdatedBy = userID
	return s.repo.Switch(ctx, &next, baseOf(sw, et), switchedBackFieldList(et.Fields, sw.OldFields, *offer), sw.OldPresetCategory, backMove(sw.OldFields, *offer))
}

// baseOf is what a switch is worked out from: the answer read and the
// category's field list.
func baseOf(sw *FieldSwitch, et *EntityType) switchBase {
	b := switchBase{Fields: et.Fields}
	if sw != nil {
		b.State = sw.State
	}
	return b
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

// ForCampaign is every answered category in a campaign, by category id, for
// the campaign backup.
func (s *FieldSwitchService) ForCampaign(ctx context.Context, campaignID string) (map[int]*FieldSwitch, error) {
	return s.repo.ForCampaign(ctx, campaignID)
}

// SetAsideForCampaign is every page's kept values in a campaign, by page id,
// for the campaign backup.
func (s *FieldSwitchService) SetAsideForCampaign(ctx context.Context, campaignID string) (map[string]map[string]any, error) {
	return s.repo.SetAsideForCampaign(ctx, campaignID)
}

// Column widths of entity_type_field_switches, checked before a restore so a
// hand-edited backup is refused with a reason rather than a database error.
const (
	switchIDMaxLen       = 100
	switchCategoryMaxLen = 50
)

// Restore makes a restored category's answer the one its backup carried: sw,
// or no answer when sw is nil, so the offer shows again. It writes no page
// and no field list; the import has already restored both.
func (s *FieldSwitchService) Restore(ctx context.Context, campaignID string, typeID int, sw *FieldSwitch, userID string) error {
	if _, err := s.category(ctx, campaignID, typeID); err != nil {
		return err
	}
	if sw == nil {
		return s.repo.Clear(ctx, typeID)
	}
	switch sw.State {
	case FieldSwitchSwitched, FieldSwitchDismissed, FieldSwitchBack:
	default:
		return apperror.NewBadRequest("the game system field answer is not one Chronicle knows")
	}
	if sw.SystemID == "" || sw.PresetSlug == "" || len(sw.SystemID) > switchIDMaxLen || len(sw.PresetSlug) > switchIDMaxLen {
		return apperror.NewBadRequest("the game system field answer does not name its game system")
	}
	if sw.OldPresetCategory != nil && len(*sw.OldPresetCategory) > switchCategoryMaxLen {
		return apperror.NewBadRequest("the game system field answer's old category is too long")
	}
	// A nil list stays nil: it means there is nothing to switch back to.
	var old []FieldDefinition
	if sw.OldFields != nil {
		old = cloneFieldDefs(sw.OldFields)
	}
	return s.repo.SaveState(ctx, &FieldSwitch{
		EntityTypeID: typeID, CampaignID: campaignID,
		SystemID: sw.SystemID, PresetSlug: sw.PresetSlug, State: sw.State,
		OldFields: old, OldPresetCategory: sw.OldPresetCategory,
		UpdatedBy: userID,
	})
}

// RestoreSetAside writes back the values a backup says a switch kept aside
// from one restored page. The import passes only pages it has just made in
// the campaign, so entityID is not checked again here.
func (s *FieldSwitchService) RestoreSetAside(ctx context.Context, campaignID string, typeID int, entityID string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	if _, err := s.category(ctx, campaignID, typeID); err != nil {
		return err
	}
	return s.repo.RestoreSetAside(ctx, entityID, typeID, values)
}
