package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// practice_handler.go is the web door of practice heroes: it binds the
// request, asks the service, and draws the page. Practice heroes never touch
// entities rows; the only place they meet one is a Director's yes, which makes
// the real hero through the same function the hero creator uses.

// HeroReleaser lets a hero go back to "Free to pick" for one page, the way a
// leaver's heroes do. Wired at the composition root from the entity
// repository, which owns that SQL.
type HeroReleaser interface {
	ReleaseOne(ctx context.Context, campaignID, entityID, userID string) (bool, error)
}

// SetPracticeService wires practice heroes. Without it every practice route
// answers not found and the Characters page shows no shelf.
func (h *Handler) SetPracticeService(s PracticeService) { h.practice = s }

// SetHeroReleaser wires the release of a player's current hero on a
// Director's yes.
func (h *Handler) SetHeroReleaser(r HeroReleaser) { h.heroReleaser = r }

// practiceBodyLimit bounds a creator body before it is read. A practice hero's
// stored picks are limited further, in the service.
const practiceBodyLimit = 128 * 1024

// practiceOn is the gate every practice route passes: the service is wired
// and the campaign lets players claim heroes. Otherwise the feature does not
// exist, so it answers not found rather than forbidden.
func (h *Handler) practiceOn(c *echo.Context, cc *campaigns.CampaignContext) error {
	if h.practice == nil || !h.isAddonEnabled(c.Request().Context(), cc.Campaign.ID, AddonPlayerCharacterClaiming) {
		return apperror.NewNotFound("practice heroes are not available here")
	}
	return nil
}

// practiceTarget is the page type a player's real hero would be, when one may
// be made at all. A practice hero is for someone who could have a hero, so
// the same rule decides: a type players may claim, among the campaign's
// character types.
func (h *Handler) practiceTarget(ctx context.Context, campaignID string) (*EntityType, []int, error) {
	if h.charLists == nil {
		return nil, nil, nil
	}
	ids, err := h.charLists.CharacterTypeIDs(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	types, err := h.service.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	return pickHeroType(types, ids, true), ids, nil
}

// practiceMaker checks that this viewer may make and change practice heroes:
// a member of at least Player, with the practice feature on and a hero type
// players may claim. A player who already has a hero qualifies; that is the
// point. Everything else is not found.
func (h *Handler) practiceMaker(c *echo.Context, cc *campaigns.CampaignContext) (PracticeActor, []int, error) {
	if err := h.practiceOn(c, cc); err != nil {
		return PracticeActor{}, nil, err
	}
	if cc.MemberRole < campaigns.RolePlayer {
		return PracticeActor{}, nil, apperror.NewNotFound("practice heroes are not available here")
	}
	target, ids, err := h.practiceTarget(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return PracticeActor{}, nil, err
	}
	if target == nil {
		return PracticeActor{}, nil, apperror.NewNotFound("practice heroes are not available here")
	}
	return h.practiceActor(c, cc), ids, nil
}

func (h *Handler) practiceActor(c *echo.Context, cc *campaigns.CampaignContext) PracticeActor {
	return PracticeActor{UserID: auth.GetUserID(c), Director: cc.CanAuthorDmOnly()}
}

// practiceBody is what the creator sends. Copy says the hero was started as a
// copy of the player's own: the server reads that hero again for the level and
// the name, so a client cannot claim either.
type practiceBody struct {
	Name  string              `json:"name"`
	Picks map[string]HeroPick `json:"picks"`
	Copy  bool                `json:"copy"`
}

func (h *Handler) bindPractice(c *echo.Context) (practiceBody, error) {
	var b practiceBody
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, practiceBodyLimit)
	if err := c.Bind(&b); err != nil {
		return b, apperror.NewBadRequest("invalid request")
	}
	return b, nil
}

// practiceCopySource reads the player's own hero as picks, through the same
// field filtering a player gets: fields marked GM-only are left out.
func (h *Handler) practiceCopySource(c *echo.Context, cc *campaigns.CampaignContext, charTypes []int) (hero *Entity, picks map[string]HeroPick, level *int, err error) {
	ctx := c.Request().Context()
	hero, err = h.ownedCharacter(ctx, cc.Campaign.ID, auth.GetUserID(c), charTypes)
	if err != nil {
		return nil, nil, nil, err
	}
	if hero == nil {
		return nil, nil, nil, apperror.NewBadRequest("you have no hero to copy")
	}
	types, err := h.service.GetEntityTypes(ctx, cc.Campaign.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	plan, err := h.practice.Plan(ctx, cc.Campaign.ID, auth.GetUserID(c))
	if err != nil {
		return nil, nil, nil, err
	}
	picks, level = practiceCopyFields(plan, hero.FieldsData, heroHiddenKeys(hero, types, cc.CanAuthorDmOnly()))
	return hero, picks, level, nil
}

// PracticeNew renders the hero creator in practice mode
// (GET /campaigns/:id/characters/practice/new). ?from=hero starts as a copy of
// the player's own hero and ?edit=<id> loads one of their practice heroes.
func (h *Handler) PracticeNew(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	actor, charTypes, err := h.practiceMaker(c, cc)
	if err != nil {
		return err
	}
	base := "/campaigns/" + cc.Campaign.ID + "/characters/practice"
	view := PracticeCreatorView{
		CampaignID: cc.Campaign.ID, CSRFToken: middleware.GetCSRFToken(c),
		PlanURL: base + "/plan", CreateURL: base,
	}
	var preload *practicePreload
	switch {
	case c.QueryParam("edit") != "":
		hero, err := h.practice.Open(c.Request().Context(), cc.Campaign.ID, c.QueryParam("edit"), actor)
		if err != nil {
			return err
		}
		// A Director may open a practice hero but only its maker changes it.
		if hero.UserID != actor.UserID {
			return apperror.NewNotFound("practice hero not found")
		}
		view.UpdateURL = base + "/" + hero.ID
		view.Editing = true
		preload = &practicePreload{Name: hero.Name, Picks: hero.Picks}
	case c.QueryParam("from") == "hero":
		hero, picks, _, err := h.practiceCopySource(c, cc, charTypes)
		if err != nil {
			return err
		}
		preload = &practicePreload{Name: hero.Name, Picks: picks, Copy: true}
	}
	if preload != nil {
		raw, err := json.Marshal(preload)
		if err != nil {
			return apperror.NewInternal(err)
		}
		view.Preload = string(raw)
	}
	return middleware.Render(c, http.StatusOK, PracticeCreatorPage(cc, view))
}

// practicePreload is what the creator starts from for a copy or an edit.
type practicePreload struct {
	Name  string              `json:"name"`
	Picks map[string]HeroPick `json:"picks"`
	Copy  bool                `json:"copy,omitempty"`
}

// PracticePlanAPI returns the practice plan
// (GET /campaigns/:id/characters/practice/plan).
func (h *Handler) PracticePlanAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if _, _, err := h.practiceMaker(c, cc); err != nil {
		return err
	}
	plan, err := h.practice.Plan(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, plan)
}

// PracticeCreate saves a new practice hero (POST /campaigns/:id/characters/practice).
func (h *Handler) PracticeCreate(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	actor, charTypes, err := h.practiceMaker(c, cc)
	if err != nil {
		return err
	}
	body, err := h.bindPractice(c)
	if err != nil {
		return err
	}
	in := PracticeInput{Name: body.Name, Picks: body.Picks}
	if body.Copy {
		hero, _, level, err := h.practiceCopySource(c, cc, charTypes)
		if err != nil {
			return err
		}
		in.CopiedFrom, in.Level = hero.Name, level
	}
	saved, err := h.practice.Create(c.Request().Context(), cc.Campaign.ID, actor, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, map[string]any{"id": saved.ID, "url": practiceURL(cc.Campaign.ID, saved.ID)})
}

// PracticeUpdate replaces a practice hero's name and picks
// (PUT /campaigns/:id/characters/practice/:pid).
func (h *Handler) PracticeUpdate(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	actor, _, err := h.practiceMaker(c, cc)
	if err != nil {
		return err
	}
	body, err := h.bindPractice(c)
	if err != nil {
		return err
	}
	saved, err := h.practice.Update(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), actor, PracticeInput{Name: body.Name, Picks: body.Picks})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"id": saved.ID, "url": practiceURL(cc.Campaign.ID, saved.ID)})
}

func practiceURL(campaignID, id string) string {
	return "/campaigns/" + campaignID + "/characters/practice/" + id
}

// PracticeShow renders a practice hero's page
// (GET /campaigns/:id/characters/practice/:pid). Only its maker and the
// Directors may open it.
func (h *Handler) PracticeShow(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	return h.practicePage(c, cc, c.Param("pid"), "")
}

// practicePage opens a practice hero and draws its page or, for an HTMX
// request, the fragment the page swaps.
func (h *Handler) practicePage(c *echo.Context, cc *campaigns.CampaignContext, id, notice string) error {
	if err := h.practiceOn(c, cc); err != nil {
		return err
	}
	ctx := c.Request().Context()
	actor := h.practiceActor(c, cc)
	hero, err := h.practice.Open(ctx, cc.Campaign.ID, id, actor)
	if err != nil {
		return err
	}
	// The notes under each pick are the maker's own plan, not the viewer's.
	plan, err := h.practice.Plan(ctx, cc.Campaign.ID, hero.UserID)
	if err != nil {
		return err
	}
	view := PracticePageView{
		CampaignID: cc.Campaign.ID, CSRFToken: middleware.GetCSRFToken(c), ID: hero.ID, Name: hero.Name,
		Summary: practiceSummary(hero.Picks, hero.Level), Rows: practicePickRows(plan, hero.Picks),
		Mine: hero.UserID == actor.UserID, Status: hero.AskStatus, Note: hero.AskNote,
		NeedsYes: practiceNeedsYes(plan, hero.Picks), Copied: hero.CopiedFromName, Notice: notice,
	}
	if !view.Mine {
		view.Maker = h.practiceMakerName(ctx, cc.Campaign.ID, hero.UserID)
	}
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, PracticeHeroContent(cc, view))
	}
	return middleware.Render(c, http.StatusOK, PracticePage(cc, view))
}


// practiceMakerName is a maker's display name, "a player" when unknown.
func (h *Handler) practiceMakerName(ctx context.Context, campaignID, userID string) string {
	if h.memberLister != nil {
		if members, err := h.memberLister.ListMembers(ctx, campaignID); err == nil {
			for _, m := range members {
				if m.UserID == userID && strings.TrimSpace(m.DisplayName) != "" {
					return m.DisplayName
				}
			}
		}
	}
	return "a player"
}

// PracticeDelete throws a practice hero away
// (DELETE /campaigns/:id/characters/practice/:pid). It is deleted outright:
// not the Trash, and not recorded as a change to a hero, since it never was one.
func (h *Handler) PracticeDelete(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	actor, _, err := h.practiceMaker(c, cc)
	if err != nil {
		return err
	}
	if err := h.practice.Delete(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), actor); err != nil {
		return err
	}
	back := "/campaigns/" + cc.Campaign.ID + "/characters"
	if isHTMX(c) {
		c.Response().Header().Set("HX-Redirect", back)
		return c.NoContent(http.StatusOK)
	}
	return c.NoContent(http.StatusNoContent)
}

// PracticeAsk marks a practice hero as one its maker wants to play
// (POST /campaigns/:id/characters/practice/:pid/ask).
func (h *Handler) PracticeAsk(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	actor, _, err := h.practiceMaker(c, cc)
	if err != nil {
		return err
	}
	id := c.Param("pid")
	if err := h.practice.Ask(c.Request().Context(), cc.Campaign.ID, id, actor); err != nil {
		return err
	}
	if !isHTMX(c) {
		return c.Redirect(http.StatusSeeOther, practiceURL(cc.Campaign.ID, id))
	}
	return h.practicePage(c, cc, id, "Asked. The Director will see it on the Characters page.")
}

// PracticeDecline is a Director's "not now"
// (POST /campaigns/:id/characters/practice/:pid/decline, optional note).
func (h *Handler) PracticeDecline(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.practiceOn(c, cc); err != nil {
		return err
	}
	var req struct {
		Note string `form:"note" json:"note"`
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 8*1024)
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	hero, err := h.practice.Decline(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), h.practiceActor(c, cc), req.Note)
	if err != nil {
		return err
	}
	return h.castClaimedPage(c, cc, "Told "+h.practiceMakerName(c.Request().Context(), cc.Campaign.ID, hero.UserID)+" not now.")
}

// PracticeAllow is a Director's yes
// (POST /campaigns/:id/characters/practice/:pid/allow): the maker's practice
// hero becomes their real hero.
func (h *Handler) PracticeAllow(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.practiceOn(c, cc); err != nil {
		return err
	}
	hero, made, err := h.promotePractice(c, cc, c.Param("pid"))
	if err != nil {
		return err
	}
	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityCreated, made.ID, made.Name)
	return h.castClaimedPage(c, cc, made.Name+" is now "+h.practiceMakerName(c.Request().Context(), cc.Campaign.ID, hero.UserID)+"'s hero.")
}

// promotePractice makes the real hero from a practice hero a Director said
// yes to.
//
// The Director's yes covers every entry the rules keep from the maker, so the
// hero is checked against the unlimited plan as that maker sees it (never
// reading anything Directors alone may read). The row is taken from the shelf
// before the page is made, so two Directors answering at once cannot make two
// heroes; if making the page fails it is put back untouched. The one-hero rule
// holds because the maker's current hero is released afterwards, the way a
// leaver's is. If that release fails the new hero stands and the error is
// loud, since both pages are then theirs and a Director must free one.
func (h *Handler) promotePractice(c *echo.Context, cc *campaigns.CampaignContext, id string) (*PracticeHero, *Entity, error) {
	ctx := c.Request().Context()
	campaignID := cc.Campaign.ID
	actor := h.practiceActor(c, cc)
	hero, err := h.practice.Open(ctx, campaignID, id, actor)
	if err != nil {
		return nil, nil, err
	}
	if !actor.Director {
		return nil, nil, apperror.NewForbidden("only the Directors answer this")
	}
	if hero.AskStatus != PracticeAskWaiting {
		return nil, nil, apperror.NewConflict("Nobody is asking to play this one now.")
	}
	target, charTypes, err := h.practiceTarget(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	if target == nil {
		return nil, nil, apperror.NewConflict("Players can't have a hero of that kind in this campaign now.")
	}
	if err := h.makerStillPlays(ctx, campaignID, hero.UserID); err != nil {
		return nil, nil, err
	}
	if h.heroPlanner == nil {
		return nil, nil, apperror.NewNotFound("practice heroes are not available here")
	}
	plan, err := h.heroPlanner.HeroPlan(WithUnlimitedPlan(ctx), campaignID, hero.UserID, false)
	if err != nil {
		return nil, nil, err
	}
	fields, _, err := buildHeroFields(plan, hero.Picks)
	if err != nil {
		return nil, nil, err
	}
	h.carryLevel(fields, hero, target)
	old, err := h.ownedCharacter(ctx, campaignID, hero.UserID, charTypes)
	if err != nil {
		return nil, nil, err
	}

	taken, err := h.practice.Claim(ctx, campaignID, id, actor)
	if err != nil {
		return nil, nil, err
	}
	owner := hero.UserID
	made, err := h.makeHero(ctx, cc, hero.UserID, hero.Name, target, fields, &owner)
	if err != nil {
		if rerr := h.practice.Restore(ctx, taken); rerr != nil {
			slog.Error("practice hero lost after a failed yes", slog.String("practice_id", id), slog.Any("error", rerr))
		}
		return nil, nil, err
	}
	if old != nil && h.heroReleaser != nil {
		if _, err := h.heroReleaser.ReleaseOne(ctx, campaignID, old.ID, hero.UserID); err != nil {
			slog.Error("releasing the old hero after a Director's yes failed; the player now owns both pages",
				slog.String("campaign_id", campaignID), slog.String("old_entity_id", old.ID),
				slog.String("new_entity_id", made.ID), slog.Any("error", err))
			return nil, nil, apperror.NewInternalMessage(
				fmt.Sprintf("%s was made, but %s could not be sent back to Free to pick. Free it from the Players list.", made.Name, old.Name), err)
		}
	}
	return taken, made, nil
}

// makerStillPlays checks the practice hero's maker is still a player here:
// the row outlives nothing but the hooks that delete it are best effort.
func (h *Handler) makerStillPlays(ctx context.Context, campaignID, userID string) error {
	if h.memberLister == nil {
		return nil
	}
	members, err := h.memberLister.ListMembers(ctx, campaignID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	for _, m := range members {
		if m.UserID == userID && m.Role >= campaigns.RolePlayer {
			return nil
		}
	}
	return apperror.NewConflict("That player is no longer in this campaign.")
}

// carryLevel sets a copied hero's level on the real hero when the page type
// has a level field to put it in.
func (h *Handler) carryLevel(fields map[string]any, hero *PracticeHero, target *EntityType) {
	if hero.Level == nil {
		return
	}
	for _, f := range target.Fields {
		if f.Key != "level" {
			continue
		}
		if f.Type == "number" {
			fields["level"] = float64(*hero.Level)
		} else {
			fields["level"] = strconv.Itoa(*hero.Level)
		}
		return
	}
}

// makeHero creates a hero page. The hero creator and a Director's yes to a
// practice hero both come through here, so a hero is the same page either way.
// createdBy is who made it; owner is who plays it, nil for a hero made for the
// table.
func (h *Handler) makeHero(ctx context.Context, cc *campaigns.CampaignContext, createdBy, name string, target *EntityType, fields map[string]any, owner *string) (*Entity, error) {
	return h.service.Create(ctx, cc.Campaign.ID, createdBy, CreateEntityInput{
		Name:         name,
		EntityTypeID: target.ID,
		IsPrivate:    cc.NewPagePrivacy(patch.Absent[bool]()),
		FieldsData:   fields,
		Playable:     true,
		OwnerUserID:  owner,
	})
}

// heroHiddenKeys are the field keys of a hero the viewer may not read: the
// ones its type or page marks GM-only, unless the viewer sees GM content.
func heroHiddenKeys(e *Entity, types []EntityType, canSeeGM bool) map[string]bool {
	hidden := map[string]bool{}
	if canSeeGM {
		return hidden
	}
	var typeFields []FieldDefinition
	for i := range types {
		if types[i].ID == e.EntityTypeID {
			typeFields = types[i].Fields
			break
		}
	}
	// RestrictionDefs, not MergeFields: a field hidden on this page is still
	// GM-only.
	for _, f := range RestrictionDefs(typeFields, e.FieldOverrides) {
		if f.GMOnly {
			hidden[f.Key] = true
		}
	}
	return hidden
}

// practiceShelf gathers the player's Practice shelf for the Characters page,
// or nil when they may not make practice heroes.
func (h *Handler) practiceShelf(c *echo.Context, cc *campaigns.CampaignContext, yours []CastMember) *PracticeShelfView {
	if h.practice == nil || cc.MemberRole != campaigns.RolePlayer || !h.isAddonEnabled(c.Request().Context(), cc.Campaign.ID, AddonPlayerCharacterClaiming) {
		return nil
	}
	ctx := c.Request().Context()
	target, charTypes, err := h.practiceTarget(ctx, cc.Campaign.ID)
	if err != nil || target == nil {
		return nil
	}
	mine, err := h.practice.ListMine(ctx, cc.Campaign.ID, auth.GetUserID(c))
	if err != nil {
		slog.Warn("cast page: practice heroes failed", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return nil
	}
	v := &PracticeShelfView{CampaignID: cc.Campaign.ID, CSRFToken: middleware.GetCSRFToken(c), Max: MaxPracticeHeroes}
	for _, m := range yours {
		if slices.Contains(charTypes, m.Entity.EntityTypeID) {
			v.HeroName = m.Entity.Name
			break
		}
	}
	for _, p := range mine {
		line := practiceSummary(p.Picks, p.Level)
		if p.CopiedFromName != "" {
			line = "A copy of " + p.CopiedFromName
		}
		v.Rows = append(v.Rows, PracticeShelfRow{ID: p.ID, Name: p.Name, Line: line})
	}
	return v
}

// practiceDirectorList gathers the Directors' list of every practice hero in
// the campaign, those asking to be played first; nil when there is none or
// the viewer is not a Director.
func (h *Handler) practiceDirectorList(c *echo.Context, cc *campaigns.CampaignContext) *PracticeDirectorView {
	if h.practice == nil || !cc.CanAuthorDmOnly() || !h.isAddonEnabled(c.Request().Context(), cc.Campaign.ID, AddonPlayerCharacterClaiming) {
		return nil
	}
	ctx := c.Request().Context()
	all, err := h.practice.ListAll(ctx, cc.Campaign.ID, h.practiceActor(c, cc))
	if err != nil {
		slog.Warn("cast page: practice heroes failed", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return nil
	}
	if len(all) == 0 {
		return nil
	}
	names := map[string]string{}
	if h.memberLister != nil {
		if members, err := h.memberLister.ListMembers(ctx, cc.Campaign.ID); err == nil {
			for _, m := range members {
				names[m.UserID] = m.DisplayName
			}
		}
	}
	_, charTypes, _ := h.practiceTarget(ctx, cc.Campaign.ID)
	v := &PracticeDirectorView{CampaignID: cc.Campaign.ID, CSRFToken: middleware.GetCSRFToken(c)}
	for _, p := range all {
		maker := strings.TrimSpace(names[p.UserID])
		if maker == "" {
			maker = "a player"
		}
		row := PracticeDirectorRow{ID: p.ID, Name: p.Name, Summary: practiceSummary(p.Picks, p.Level), Maker: maker, Waiting: p.AskStatus == PracticeAskWaiting}
		if row.Waiting {
			if old, err := h.ownedCharacter(ctx, cc.Campaign.ID, p.UserID, charTypes); err == nil && old != nil {
				row.OldHero = old.Name
			}
		}
		v.Rows = append(v.Rows, row)
	}
	return v
}
