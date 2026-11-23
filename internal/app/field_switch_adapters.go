package app

// field_switch_adapters.go turns a game system's preset "switch" block into
// the entities plugin's field switch offer. The entities plugin never reads a
// manifest; this file is the bridge, as preset_applier.go is for presets.

import (
	"context"
	"sync/atomic"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// switchOfferFrom is the offer a preset's switch block makes.
func switchOfferFrom(m *systems.SystemManifest, preset systems.EntityPresetDef) entities.FieldSwitchOffer {
	return entities.FieldSwitchOffer{
		SystemID:   m.ID,
		SystemName: m.Name,
		PresetSlug: preset.Slug,
		Category:   preset.Category,
		Label:      preset.Switch.Label,
		Summary:    preset.Switch.Summary,
		Fields:     mapPresetFields(preset.Fields),
		SetAside:   append([]string(nil), preset.Switch.SetAside...),
		Reuse:      append([]string(nil), preset.Switch.Reuse...),
	}
}

// isSwitchPreset reports whether a preset reaches campaigns only through the
// owner's yes. A switch block without a category has nothing to switch.
func isSwitchPreset(preset systems.EntityPresetDef) bool {
	return preset.Switch != nil && preset.Category != ""
}

// switchOfferFor is the offer m makes et, or nil: et must be the campaign's
// category for one of m's switch presets.
func switchOfferFor(m *systems.SystemManifest, types []entities.EntityType, et *entities.EntityType) *entities.FieldSwitchOffer {
	if m == nil || et == nil {
		return nil
	}
	for _, preset := range m.EntityPresets {
		if !isSwitchPreset(preset) {
			continue
		}
		if target := entities.SwitchTarget(types, preset.Category); target != nil && target.ID == et.ID {
			offer := switchOfferFrom(m, preset)
			return &offer
		}
	}
	return nil
}

// enabledManifestSource names the campaign's enabled game system. The system
// handler is built after the entities plugin, so it is set once wiring
// reaches it; until then no switch is offered.
type enabledManifestSource interface {
	EnabledSystem(ctx context.Context, campaignID string) systems.System
}

// fieldSwitchOfferSource answers the entities plugin's FieldSwitchOffers from
// the campaign's enabled system.
type fieldSwitchOfferSource struct {
	sys   atomic.Pointer[enabledManifestSource]
	types interface {
		GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	}
}

// setSystems wires the enabled-system lookup.
func (s *fieldSwitchOfferSource) setSystems(sys enabledManifestSource) {
	s.sys.Store(&sys)
}

// offer implements entities.FieldSwitchOffers. A failed read offers nothing.
func (s *fieldSwitchOfferSource) offer(ctx context.Context, campaignID string, et *entities.EntityType) *entities.FieldSwitchOffer {
	sys := s.sys.Load()
	if sys == nil || et == nil {
		return nil
	}
	active := (*sys).EnabledSystem(ctx, campaignID)
	if active == nil || active.Info() == nil {
		return nil
	}
	m := active.Info()
	hasSwitch := false
	for _, p := range m.EntityPresets {
		if isSwitchPreset(p) {
			hasSwitch = true
			break
		}
	}
	if !hasSwitch {
		return nil
	}
	types, err := s.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil
	}
	return switchOfferFor(m, types, et)
}

// switchedCategoryGate answers syncapi's ItemFieldsGate: true once the
// campaign's category for the preset has switched to its fields. A failed
// read answers false, so item sync falls back to plain items.
func switchedCategoryGate(types interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
}, state interface {
	State(ctx context.Context, typeID int) (*entities.FieldSwitch, error)
}) func(ctx context.Context, campaignID string, preset *systems.EntityPresetDef) bool {
	return func(ctx context.Context, campaignID string, preset *systems.EntityPresetDef) bool {
		if preset == nil {
			return false
		}
		list, err := types.GetEntityTypes(ctx, campaignID)
		if err != nil {
			return false
		}
		target := entities.SwitchTarget(list, preset.Category)
		if target == nil {
			return false
		}
		sw, err := state.State(ctx, target.ID)
		return err == nil && sw != nil && sw.State == entities.FieldSwitchSwitched
	}
}

// switchTargets is the campaign's categories its enabled system offers a
// switch, whatever their answer.
func (s *fieldSwitchOfferSource) switchTargets(ctx context.Context, campaignID string) ([]*entities.EntityType, error) {
	sys := s.sys.Load()
	if sys == nil {
		return nil, nil
	}
	active := (*sys).EnabledSystem(ctx, campaignID)
	if active == nil || active.Info() == nil {
		return nil, nil
	}
	types, err := s.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var out []*entities.EntityType
	for _, preset := range active.Info().EntityPresets {
		if !isSwitchPreset(preset) {
			continue
		}
		if t := entities.SwitchTarget(types, preset.Category); t != nil {
			out = append(out, t)
		}
	}
	return out, nil
}

// aiFieldSwitchAdapter implements records.CategorySwitchAPI over the field
// switch service, so the campaign AI can see and ask for the switch.
type aiFieldSwitchAdapter struct {
	owner  aiMenuAdapter // the owner check the sidebar order kind uses
	offers *fieldSwitchOfferSource
	svc    *entities.FieldSwitchService
}

var _ records.CategorySwitchAPI = aiFieldSwitchAdapter{}

func (a aiFieldSwitchAdapter) IsOwner(ctx context.Context, campaignID, userID string) (bool, error) {
	return a.owner.IsOwner(ctx, campaignID, userID)
}

func (a aiFieldSwitchAdapter) Switches(ctx context.Context, campaignID string) ([]records.CategorySwitch, error) {
	targets, err := a.offers.switchTargets(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var out []records.CategorySwitch
	for _, t := range targets {
		v := a.svc.View(ctx, campaignID, t)
		if v == nil {
			continue
		}
		name := t.NamePlural
		if name == "" {
			name = t.Name
		}
		switched := v.State == entities.FieldSwitchSwitched
		out = append(out, records.CategorySwitch{
			TypeID:          t.ID,
			Category:        name,
			System:          v.Offer.SystemName,
			Label:           v.Offer.Label,
			Adds:            v.AddsLine(),
			Removes:         v.RemovesLine(t.Fields),
			Switched:        switched,
			CanSwitchBack:   v.CanSwitchBack,
			OwnerAnswered:   v.State != "",
			SwitchedAtBirth: switched && !v.CanSwitchBack,
		})
	}
	return out, nil
}

func (a aiFieldSwitchAdapter) Accept(ctx context.Context, campaignID string, typeID int, userID string) error {
	_, err := a.svc.Accept(ctx, campaignID, typeID, userID)
	return err
}

func (a aiFieldSwitchAdapter) Back(ctx context.Context, campaignID string, typeID int, userID string) error {
	_, err := a.svc.Back(ctx, campaignID, typeID, userID)
	return err
}

func init() {
	addAIKinds(185, func(d *aiDeps) []records.Kind {
		if d.FieldSwitch == nil {
			return nil
		}
		return []records.Kind{records.CategorySwitchKind{Svc: d.FieldSwitch}}
	})
}
