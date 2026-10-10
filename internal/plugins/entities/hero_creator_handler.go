package entities

import (
	"context"
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SetHeroPlanner wires the source of the hero creator's steps. Without it
// the creator asks only for a name.
func (h *Handler) SetHeroPlanner(p HeroPlanner) { h.heroPlanner = p }

// heroTarget works out the page type a new hero becomes for this viewer and
// whether they may make one. A player's hero must be a type they may claim.
func (h *Handler) heroTarget(c echo.Context, cc *campaigns.CampaignContext) (*EntityType, heroAccess, error) {
	ctx := c.Request().Context()
	campaignID := cc.Campaign.ID
	if h.charLists == nil || cc.MemberRole < campaigns.RolePlayer {
		return nil, heroAccess{}, nil
	}
	ids, err := h.charLists.CharacterTypeIDs(ctx, campaignID)
	if err != nil {
		return nil, heroAccess{}, err
	}
	types, err := h.service.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, heroAccess{}, err
	}
	staff := cc.MemberRole >= campaigns.RoleScribe
	target := pickHeroType(types, ids, !staff)
	claimingOn := h.isAddonEnabled(ctx, campaignID, AddonPlayerCharacterClaiming)
	owns := false
	if !staff && claimingOn && target != nil {
		if owns, err = h.ownsCharacter(ctx, campaignID, auth.GetUserID(c), ids); err != nil {
			return nil, heroAccess{}, err
		}
	}
	return target, heroAccessFor(cc.MemberRole, claimingOn, target, owns), nil
}

// ownsCharacter reports whether the user already owns a page of one of the
// campaign's character types.
func (h *Handler) ownsCharacter(ctx context.Context, campaignID, userID string, charTypeIDs []int) (bool, error) {
	owned, err := h.service.ListByOwner(ctx, campaignID, userID)
	if err != nil {
		return false, err
	}
	for _, e := range owned {
		if slices.Contains(charTypeIDs, e.EntityTypeID) {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handler) heroPlan(c echo.Context, cc *campaigns.CampaignContext) (*HeroPlan, error) {
	if h.heroPlanner == nil {
		return &HeroPlan{Steps: []HeroStep{}}, nil
	}
	return h.heroPlanner.HeroPlan(c.Request().Context(), cc.Campaign.ID, cc.CanAuthorDmOnly())
}

// NewHero renders the hero creator (GET /campaigns/:id/characters/new).
func (h *Handler) NewHero(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	target, access, err := h.heroTarget(c, cc)
	if err != nil {
		return err
	}
	if !access.Allowed {
		return apperror.NewForbidden("you can't create a hero in this campaign")
	}
	return middleware.Render(c, http.StatusOK, HeroCreatorPage(cc, HeroCreatorView{
		TypeName:  target.Name,
		ForSelf:   access.ForSelf,
		CSRFToken: middleware.GetCSRFToken(c),
	}))
}

// HeroPlanAPI returns the creator's steps (GET /campaigns/:id/characters/new/plan).
func (h *Handler) HeroPlanAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if _, access, err := h.heroTarget(c, cc); err != nil {
		return err
	} else if !access.Allowed {
		return apperror.NewForbidden("you can't create a hero in this campaign")
	}
	plan, err := h.heroPlan(c, cc)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, plan)
}

// CreateHero makes the hero from the creator's picks
// (POST /campaigns/:id/characters/new) and answers with its page address.
func (h *Handler) CreateHero(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req HeroRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := apperror.ValidateRequired("name", req.Name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", req.Name, apperror.MaxNameLength); err != nil {
		return err
	}
	target, access, err := h.heroTarget(c, cc)
	if err != nil {
		return err
	}
	if !access.Allowed {
		return apperror.NewForbidden("you can't create a hero in this campaign")
	}
	plan, err := h.heroPlan(c, cc)
	if err != nil {
		return err
	}
	fields, err := buildHeroFields(plan, req.Picks)
	if err != nil {
		return err
	}

	userID := auth.GetUserID(c)
	input := CreateEntityInput{
		Name:         req.Name,
		EntityTypeID: target.ID,
		IsPrivate:    cc.Campaign.ParseSettings().ResolveNewEntityPrivacy(patch.Absent[bool]()),
		FieldsData:   fields,
	}
	if access.ForSelf {
		input.OwnerUserID = &userID
	}
	entity, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, userID, input)
	if err != nil {
		return err
	}
	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityCreated, entity.ID, entity.Name)
	return c.JSON(http.StatusCreated, map[string]string{
		"id":  entity.ID,
		"url": "/campaigns/" + cc.Campaign.ID + "/entities/" + entity.ID,
	})
}
