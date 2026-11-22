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
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// practice_handler.go is the web door of practice heroes: it binds the
// request, asks the service, and draws the page. Practice heroes never touch
// entities rows; the only place they meet one is a Director's yes, which makes
// the real hero through the same function the hero creator uses.

// SetPracticeService wires practice heroes. Without it every practice route
// answers not found and the Characters page shows no shelf.
func (h *Handler) SetPracticeService(s PracticeService) { h.practice = s }

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

// practiceMaker checks that this viewer may make and change practice heroes:
// a Player exactly (a Scribe or Director gets the same 404 a stranger does),
// with the practice feature on and a hero type players may claim. A player who
// already has a hero qualifies; that is the point.
func (h *Handler) practiceMaker(c *echo.Context, cc *campaigns.CampaignContext) (PracticeActor, error) {
	if err := h.practiceOn(c, cc); err != nil {
		return PracticeActor{}, err
	}
	if cc.MemberRole != campaigns.RolePlayer {
		return PracticeActor{}, apperror.NewNotFound("practice heroes are not available here")
	}
	target, _, err := h.practice.Target(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return PracticeActor{}, err
	}
	if target == nil {
		return PracticeActor{}, apperror.NewNotFound("practice heroes are not available here")
	}
	return h.practiceActor(c, cc), nil
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

// PracticeNew renders the hero creator in practice mode
// (GET /campaigns/:id/characters/practice/new). ?from=hero starts as a copy of
// the player's own hero and ?edit=<id> loads one of their practice heroes.
func (h *Handler) PracticeNew(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	actor, err := h.practiceMaker(c, cc)
	if err != nil {
		return err
	}
	base := "/campaigns/" + cc.Campaign.ID + "/characters/practice"
	view := PracticeCreatorView{
		CampaignID: cc.Campaign.ID, CSRFToken: middleware.GetCSRFToken(c),
		PlanURL: base + "/plan", CreateURL: base,
		TermsURL: h.termsURLIf(c.Request().Context(), cc.Campaign.ID, base+"/terms"),
		Look:     heroLookFrom(c.QueryParam("look")),
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
		src, err := h.practice.CopySource(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), cc.CanAuthorDmOnly())
		if err != nil {
			return err
		}
		preload = &practicePreload{Name: src.Hero.Name, Picks: src.Picks, Copy: true}
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
	if _, err := h.practiceMaker(c, cc); err != nil {
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
	actor, err := h.practiceMaker(c, cc)
	if err != nil {
		return err
	}
	body, err := h.bindPractice(c)
	if err != nil {
		return err
	}
	in := PracticeInput{Name: body.Name, Picks: body.Picks}
	if body.Copy {
		src, err := h.practice.CopySource(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), cc.CanAuthorDmOnly())
		if err != nil {
			return err
		}
		in.CopiedFrom, in.Level = src.Hero.Name, src.Level
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
	actor, err := h.practiceMaker(c, cc)
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
	actor, err := h.practiceMaker(c, cc)
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
	actor, err := h.practiceMaker(c, cc)
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
// (POST /campaigns/:id/characters/practice/:pid/allow, with `seen`: the
// version of the hero the Director looked at): the maker's practice hero
// becomes their real hero.
func (h *Handler) PracticeAllow(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.practiceOn(c, cc); err != nil {
		return err
	}
	var req struct {
		Seen string `form:"seen" json:"seen"`
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 8*1024)
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	ctx := c.Request().Context()
	res, err := h.practice.Promote(ctx, cc.Campaign.ID, c.Param("pid"), h.practiceActor(c, cc), req.Seen)
	if err != nil {
		return err
	}
	// The hero exists from here on, so it is on record before anything about
	// the old hero can go wrong.
	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityCreated, res.Hero.ID, res.Hero.Name)
	maker := h.practiceMakerName(ctx, cc.Campaign.ID, res.Practice.UserID)
	if res.ReleaseErr != nil {
		return h.castClaimedPageAlert(c, cc, fmt.Sprintf("%s is now %s's hero, but %s couldn't be freed. Open %s and free it.",
			res.Hero.Name, maker, res.Previous.Name, res.Previous.Name))
	}
	return h.castClaimedPage(c, cc, res.Hero.Name+" is now "+maker+"'s hero.")
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
	target, charTypes, err := h.practice.Target(ctx, cc.Campaign.ID)
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
	now := time.Now()
	for _, p := range mine {
		line := practiceSummary(p.Picks, p.Level)
		if p.CopiedFromName != "" {
			line = "A copy of " + p.CopiedFromName + ", changed " + practiceAge(now, p.UpdatedAt)
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
	var waiting []PracticeHero
	for _, p := range all {
		if p.AskStatus == PracticeAskWaiting {
			waiting = append(waiting, p)
		}
	}
	held := h.practice.HeldPicks(ctx, cc.Campaign.ID, waiting)
	v := &PracticeDirectorView{CampaignID: cc.Campaign.ID, CSRFToken: middleware.GetCSRFToken(c)}
	for _, p := range all {
		maker := strings.TrimSpace(names[p.UserID])
		if maker == "" {
			maker = "a player"
		}
		row := PracticeDirectorRow{ID: p.ID, Name: p.Name, Summary: practiceSummary(p.Picks, p.Level), Maker: maker,
			Waiting: p.AskStatus == PracticeAskWaiting, Version: strconv.FormatInt(p.Version(), 10)}
		if row.Waiting {
			if old, err := h.practice.CurrentHero(ctx, cc.Campaign.ID, p.UserID); err == nil && old != nil {
				row.OldHero = old.Name
			}
			row.Holds = practiceHoldsLine(maker, held[p.ID])
		}
		v.Rows = append(v.Rows, row)
	}
	return v
}
