package entities

// The campaign home's "The party" block: each hero's name, ancestry, class and
// level, and who plays them. The home lives in the campaigns plugin, which may
// not read pages, so its placeholder loads this fragment. It reads what the
// Characters page's party band reads (claimed characters of the types the
// owner listed) and shows only the identity facts, and only those a player may
// read: a field marked GM-only or owner-only is never shown.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// partyMaxHeroes bounds the block so a large table cannot make the home long;
// the Characters page holds the whole party.
const partyMaxHeroes = 12

// partyFieldKeys are the field keys read for each fact, in the order a game
// system is most likely to name them: the same idea is "race", "species" or
// "ancestry" depending on the system.
var (
	partyAncestryKeys = []string{"ancestry", "race", "species", "heritage"}
	partyClassKeys    = []string{"class", "calling", "career"}
	partyLevelKeys    = []string{"level"}
)

// partyHero is one row of the block, already worded.
type partyHero struct {
	Name string
	Href string
	// Line is "High Elf Elementalist", whatever of ancestry and class is known.
	Line string
	// Level is "Level 3", or "" when the hero has none or it is not public.
	Level string
	// Player is who plays the hero; "" for a visitor, who sees heroes only.
	Player string
	// You marks the viewer's own hero.
	You bool
}

// partyView is what the block draws.
type partyView struct {
	CampaignID string
	Heroes     []partyHero
	// More counts heroes left out by partyMaxHeroes.
	More int
}

// partyFacts reads a hero's ancestry, class and level. Only a field whose
// type does not mark it GM-only or owner-only is read, so a system that keeps
// a hero's class private shows no class here either. fields is the page's
// effective field list (its type's fields with the page's own changes).
func partyFacts(data map[string]any, fields []FieldDefinition) (ancestry, class, level string) {
	open := map[string]bool{}
	for _, f := range fields {
		if !f.GMOnly && !f.OwnerOnly {
			open[f.Key] = true
		}
	}
	read := func(keys []string) string {
		for _, k := range keys {
			if !open[k] {
				continue
			}
			if v := partyText(data[k]); v != "" {
				return v
			}
		}
		return ""
	}
	return read(partyAncestryKeys), read(partyClassKeys), read(partyLevelKeys)
}

// partyText is a field value as one short line of text: a string, or a whole
// number. Anything else (a list, a map) is left out rather than printed as Go
// syntax.
func partyText(v any) string {
	switch t := v.(type) {
	case string:
		return strings.Join(strings.Fields(t), " ")
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	}
	return ""
}

// partyLine joins ancestry and class into the hero's second line.
func partyLine(ancestry, class string) string {
	return strings.TrimSpace(ancestry + " " + class)
}

// buildParty words the block for a viewer. types maps a page type's id to its
// type; a hero whose type is unknown still shows, with no facts.
func buildParty(campaignID string, members []CastMember, types map[int]*EntityType) partyView {
	view := partyView{CampaignID: campaignID}
	for i, m := range members {
		if i >= partyMaxHeroes {
			view.More = len(members) - partyMaxHeroes
			break
		}
		e := m.Entity
		var fields []FieldDefinition
		if t := types[e.EntityTypeID]; t != nil {
			fields = MergeFields(t.Fields, e.FieldOverrides)
			if e.FieldOverrides != nil {
				fields = append(fields, e.FieldOverrides.Added...)
			}
		}
		ancestry, class, level := partyFacts(e.FieldsData, fields)
		h := partyHero{
			Name:   e.Name,
			Href:   "/campaigns/" + campaignID + "/entities/" + e.ID,
			Line:   partyLine(ancestry, class),
			Player: m.OwnerName,
			You:    m.IsViewer,
		}
		if level != "" {
			h.Level = "Level " + level
		}
		view.Heroes = append(view.Heroes, h)
	}
	return view
}

// PartyBlock draws the block, or an empty answer that removes the placeholder
// when there are no heroes to show.
// GET /campaigns/:id/party
func (h *Handler) PartyBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	campaignID := cc.Campaign.ID
	if !h.isAddonEnabled(ctx, campaignID, AddonPlayerCharacterClaiming) {
		return c.HTML(http.StatusOK, "")
	}
	role := int(cc.VisibilityRole())
	// The Director previewing as a player reads what a player reads.
	if campaigns.ViewingAsPlayer(c, cc) {
		role = int(campaigns.RolePlayer)
	}
	var charTypes []int
	if h.charLists != nil {
		var err error
		if charTypes, err = h.charLists.CharacterTypeIDs(ctx, campaignID); err != nil {
			slog.Warn("party block: character types failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
			return c.HTML(http.StatusOK, "")
		}
	}
	members := filterCastByType(h.buildCastParty(ctx, cc, auth.GetUserID(c), role), charTypes)
	if len(members) == 0 {
		return c.HTML(http.StatusOK, "")
	}
	return middleware.Render(c, http.StatusOK, partyBlock(buildParty(campaignID, members, h.typesByID(ctx, campaignID))))
}

// typesByID maps the campaign's page types by id; a failed read gives an empty
// map, so the block still names the heroes.
func (h *Handler) typesByID(ctx context.Context, campaignID string) map[int]*EntityType {
	out := map[int]*EntityType{}
	types, err := h.service.GetEntityTypes(ctx, campaignID)
	if err != nil {
		slog.Warn("party block: page types failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return out
	}
	for i := range types {
		out[types[i].ID] = &types[i]
	}
	return out
}
