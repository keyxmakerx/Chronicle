package entities

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// FieldTypeClimate is a place's climate, one of the calendar's climates by
// id. A page with none takes its nearest parent's, so a region's towns
// share its weather; a page with no climate above it keeps the calendar's
// world climate and shows no climate line at all, so a campaign that never
// sets one sees nothing new. The page that holds the climate is the place's
// region: the calendar keeps weather per region.
const FieldTypeClimate = "climate"

// FieldClimate is the calendar's side of a climate field. It belongs to the
// calendar plugin and is wired in internal/app; unset, a climate field
// stores any value and shows no weather.
type FieldClimate interface {
	// Climates is every climate a place can take, in the picker's order.
	Climates() []ClimateChoice
	// World is the climate a place with none keeps; false when the campaign
	// has no calendar this viewer can see.
	World(ctx context.Context, campaignID string, role int, userID string) (ClimateChoice, bool)
	// Today is today's weather in a region as this viewer may see it; nil
	// for a player when the party is not in that region today.
	Today(ctx context.Context, campaignID, regionID string, role int, userID string) *ClimateToday
}

// ClimateChoice is one climate a place can take.
type ClimateChoice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Magic bool   `json:"magic,omitempty"`
}

// ClimateToday is a region's weather today, in words.
type ClimateToday struct {
	Date string `json:"date"`
	// Words is "Light rain, 6°"; empty when the day has no weather yet.
	Words     string `json:"words,omitempty"`
	PartyHere bool   `json:"partyHere,omitempty"`
}

// ClimateShown is how a climate field reads beyond its name.
type ClimateShown struct {
	// Own is set when the page holds the climate itself.
	Own bool `json:"own"`
	// From and FromHref name the page the climate comes from, only when the
	// viewer may open it.
	From     string        `json:"from,omitempty"`
	FromHref string        `json:"fromHref,omitempty"`
	Today    *ClimateToday `json:"today,omitempty"`
	// Covers says pages under this one follow its climate; Here is this
	// page's name for that line.
	Covers bool   `json:"covers,omitempty"`
	Here   string `json:"here,omitempty"`
}

// SetFieldClimate wires the calendar's climates into the fields API.
func (h *Handler) SetFieldClimate(fc FieldClimate) { h.fieldClimate = fc }

// SetFieldClimate lets field saves check a climate is one the calendar has.
func (s *entityService) SetFieldClimate(fc FieldClimate) { s.fieldClimate = fc }

// climateKey is the key of a definition list's climate field, "" for none.
// Only the first counts, so a page has one climate.
func climateKey(defs []FieldDefinition) string {
	for _, d := range defs {
		if d.Type == FieldTypeClimate {
			return d.Key
		}
	}
	return ""
}

// climateChoice finds id among the choices.
func climateChoice(choices []ClimateChoice, id string) (ClimateChoice, bool) {
	for _, c := range choices {
		if c.ID == id {
			return c, true
		}
	}
	return ClimateChoice{}, false
}

// checkClimateValue refuses a climate the calendar does not list. Empty is
// always fine: it means "the same as around it".
func checkClimateValue(d FieldDefinition, v any, choices []ClimateChoice) error {
	id := strings.TrimSpace(fieldText(v))
	if id == "" || choices == nil {
		return nil
	}
	if _, ok := climateChoice(choices, id); ok {
		return nil
	}
	ids := make([]string, len(choices))
	for i, c := range choices {
		ids[i] = c.ID
	}
	return apperror.NewBadRequest(fmt.Sprintf("%s must be one of the calendar's climates: %s", d.Label, strings.Join(ids, ", ")))
}

// PlaceClimate is a page with a climate of its own, for the calendar.
type PlaceClimate struct {
	ID       string
	Name     string
	ParentID string
	Climate  string
}

// ownClimate is the climate a page holds itself, "" for none.
func (s *entityService) ownClimate(ctx context.Context, e *Entity, types map[int]*EntityType) string {
	et, ok := types[e.EntityTypeID]
	if !ok {
		et, _ = s.types.FindByID(ctx, e.EntityTypeID)
		types[e.EntityTypeID] = et
	}
	if et == nil {
		return ""
	}
	key := climateKey(et.Fields)
	if key == "" {
		return ""
	}
	return strings.TrimSpace(fieldText(e.FieldsData[key]))
}

// ClimateRegion is the page whose climate covers e (e itself or its nearest
// real parent with one) and that climate. Nil when no page above has one.
// Parents are read unfiltered: which climate applies is not a secret, and
// callers name the region only to viewers who may open it.
func (s *entityService) ClimateRegion(ctx context.Context, e *Entity) (*Entity, string, error) {
	types := map[int]*EntityType{}
	if c := s.ownClimate(ctx, e, types); c != "" {
		return e, c, nil
	}
	parents, err := s.entities.FindAncestors(ctx, e.ID, permissions.RoleOwner, "")
	if err != nil {
		return nil, "", fmt.Errorf("finding climate region: %w", err)
	}
	for i := range parents {
		p := &parents[i]
		if p.CampaignID != e.CampaignID {
			continue
		}
		if c := s.ownClimate(ctx, p, types); c != "" {
			return p, c, nil
		}
	}
	return nil, "", nil
}

// climatePlacesPage is how many pages one walk step reads.
const climatePlacesPage = 200

// ClimatePlaces lists every live page of the campaign whose category has a
// climate field and that holds a climate. Templates are not places. The
// walk reads every page of those categories, never a capped few.
func (s *entityService) ClimatePlaces(ctx context.Context, campaignID string) ([]PlaceClimate, error) {
	types, err := s.types.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing categories: %w", err)
	}
	keys := map[int]string{}
	var ids []int
	for _, t := range types {
		if k := climateKey(t.Fields); k != "" {
			keys[t.ID] = k
			ids = append(ids, t.ID)
		}
	}
	var out []PlaceClimate
	if len(ids) == 0 {
		return out, nil
	}
	for page := 1; ; page++ {
		got, total, err := s.entities.ListByCampaign(ctx, campaignID, ids, permissions.RoleOwner, "",
			ListOptions{Page: page, PerPage: climatePlacesPage})
		if err != nil {
			return nil, fmt.Errorf("listing places: %w", err)
		}
		for _, e := range got {
			if e.IsTemplate {
				continue
			}
			c := strings.TrimSpace(fieldText(e.FieldsData[keys[e.EntityTypeID]]))
			if c == "" {
				continue
			}
			pc := PlaceClimate{ID: e.ID, Name: e.Name, Climate: c}
			if e.ParentID != nil {
				pc.ParentID = *e.ParentID
			}
			out = append(out, pc)
		}
		if len(got) == 0 || page*climatePlacesPage >= total {
			break
		}
	}
	return out, nil
}

// climateFieldDef is the Climate field every Location category carries.
var climateFieldDef = FieldDefinition{Key: "climate", Label: "Climate", Type: FieldTypeClimate, Section: "Basics"}

// EnsureClimateFields gives every Location category a Climate field when it
// has none, so a place's climate has a line on its page. Additive and
// idempotent: a category that already has a climate field, or any field
// keyed "climate", is left alone, and the empty field shows nothing until a
// climate is chosen. Best-effort per category; returns how many changed.
func (s *entityService) EnsureClimateFields(ctx context.Context) (int, error) {
	all, err := s.types.ListAll(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing categories: %w", err)
	}
	changed := 0
	for _, t := range all {
		if t.Slug != "location" || climateKey(t.Fields) != "" {
			continue
		}
		taken := false
		for _, f := range t.Fields {
			if f.Key == climateFieldDef.Key {
				taken = true
			}
		}
		if taken {
			continue
		}
		def := climateFieldDef
		if len(t.Fields) > 0 && t.Fields[0].Section != "" {
			def.Section = t.Fields[0].Section
		}
		n, err := s.ReconcileEntityTypeFields(ctx, t.ID, []FieldDefinition{def})
		if err != nil {
			slog.Warn("entities: climate field not added", slog.Int("entity_type_id", t.ID), slog.Any("error", err))
			continue
		}
		changed += n
	}
	return changed, nil
}

// climateShown works out a climate field for one page and viewer: the
// climate's name, where it comes from and today's weather there. ok is
// false when neither the page nor any page above it has a climate, so the
// line stays off the page.
func (h *Handler) climateShown(ctx context.Context, campaignID string, entity *Entity, role int, userID string) (FieldShown, bool) {
	region, climate, err := h.service.ClimateRegion(ctx, entity)
	if err != nil || region == nil {
		return FieldShown{}, false
	}
	name := climate
	if h.fieldClimate != nil {
		if c, ok := climateChoice(h.fieldClimate.Climates(), climate); ok {
			name = c.Name
		}
	}
	cs := &ClimateShown{Own: region.ID == entity.ID}
	if !cs.Own {
		if access, err := h.service.CheckEntityAccess(ctx, region.ID, role, userID); err == nil && access != nil && access.CanView {
			cs.From = region.Name
			cs.FromHref = fmt.Sprintf("/campaigns/%s/entities/%s", campaignID, region.ID)
		}
	} else if kids, err := h.service.GetChildren(ctx, entity.ID, role, userID); err == nil && len(kids) > 0 {
		cs.Covers, cs.Here = true, entity.Name
	}
	if h.fieldClimate != nil {
		cs.Today = h.fieldClimate.Today(ctx, campaignID, region.ID, role, userID)
	}
	return FieldShown{Text: name, Climate: cs}, true
}

// climateBook is what a climate field's picker needs: the climates, the
// world's, and what the page would take with none of its own. Nil when no
// field is a climate.
func (h *Handler) climateBook(ctx context.Context, campaignID string, entity *Entity, defs []FieldDefinition, role int, userID string) map[string]any {
	if climateKey(defs) == "" || h.fieldClimate == nil {
		return nil
	}
	out := map[string]any{"choices": h.fieldClimate.Climates()}
	if w, ok := h.fieldClimate.World(ctx, campaignID, role, userID); ok {
		out["world"] = w
	}
	// What an empty value falls back to: the nearest parent's climate.
	if entity.ParentID != nil && *entity.ParentID != "" {
		if parent, err := h.service.GetByID(ctx, *entity.ParentID); err == nil && parent != nil && parent.CampaignID == campaignID {
			if region, climate, err := h.service.ClimateRegion(ctx, parent); err == nil && region != nil {
				in := map[string]string{"climate": climate}
				if c, ok := climateChoice(h.fieldClimate.Climates(), climate); ok {
					in["name"] = c.Name
				}
				if access, err := h.service.CheckEntityAccess(ctx, region.ID, role, userID); err == nil && access != nil && access.CanView {
					in["from"] = region.Name
				}
				out["inherits"] = in
			}
		}
	}
	return out
}
