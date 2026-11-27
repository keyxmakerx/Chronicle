package entities

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/labstack/echo/v5"

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

// SetHeroChanges wires when players may change their hero's picks. Without
// it every hero is open and no change rings the Directors' bell.
func (h *Handler) SetHeroChanges(s *HeroChangeService) { h.heroChanges = s }

// heroTarget works out the page type a new hero becomes for this viewer and
// whether they may make one. A player's hero must be a type they may claim.
func (h *Handler) heroTarget(c *echo.Context, cc *campaigns.CampaignContext) (*EntityType, heroAccess, error) {
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
	var own *Entity
	if !staff && claimingOn && target != nil {
		if own, err = h.ownedCharacter(ctx, campaignID, auth.GetUserID(c), ids); err != nil {
			return nil, heroAccess{}, err
		}
	}
	access := heroAccessFor(cc.MemberRole, claimingOn, target, own != nil)
	if access.HasHero {
		access.OwnHero = own
	}
	return target, access, nil
}

// ownedCharacter returns the user's page of one of the campaign's character
// types, or nil when they have none.
func (h *Handler) ownedCharacter(ctx context.Context, campaignID, userID string, charTypeIDs []int) (*Entity, error) {
	owned, err := h.service.ListByOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	for i := range owned {
		if slices.Contains(charTypeIDs, owned[i].EntityTypeID) {
			return &owned[i], nil
		}
	}
	return nil, nil
}

// ownsCharacter reports whether the user already owns such a page.
func (h *Handler) ownsCharacter(ctx context.Context, campaignID, userID string, charTypeIDs []int) (bool, error) {
	e, err := h.ownedCharacter(ctx, campaignID, userID, charTypeIDs)
	return e != nil, err
}

func (h *Handler) heroPlan(c *echo.Context, cc *campaigns.CampaignContext) (*HeroPlan, error) {
	if h.heroPlanner == nil {
		return &HeroPlan{Steps: []HeroStep{}}, nil
	}
	ctx := c.Request().Context()
	// A Scribe makes heroes for the table and is not limited, as a Director
	// is not; a player is, by the rules about them.
	if cc.MemberRole >= campaigns.RoleScribe && !cc.CanAuthorDmOnly() {
		ctx = WithUnlimitedPlan(ctx)
	}
	return h.heroPlanner.HeroPlan(ctx, cc.Campaign.ID, auth.GetUserID(c), cc.CanAuthorDmOnly())
}

// previewPlan is the plan as one player gets it, for a Director previewing
// the hero builder as that player. Directors only: the same lists and rules
// the player is served, with nothing of the Director's own mixed in.
func (h *Handler) previewPlan(c *echo.Context, cc *campaigns.CampaignContext, asUser string) (*HeroPlan, error) {
	if h.heroPlanner == nil {
		return &HeroPlan{Steps: []HeroStep{}}, nil
	}
	return h.heroPlanner.HeroPlan(c.Request().Context(), cc.Campaign.ID, asUser, false)
}

// mayReadHeroPlan reports whether a viewer below Director may read the
// creator plan: they may make a hero, they own a page of one of the
// campaign's character types, or a Director let them write an entry of
// their own for one of its lists.
func (h *Handler) mayReadHeroPlan(c *echo.Context, cc *campaigns.CampaignContext) (bool, error) {
	_, access, err := h.heroTarget(c, cc)
	if err != nil || access.Allowed {
		return access.Allowed, err
	}
	if cc.MemberRole < campaigns.RolePlayer {
		return false, nil
	}
	if h.charLists != nil {
		ids, err := h.charLists.CharacterTypeIDs(c.Request().Context(), cc.Campaign.ID)
		if err != nil {
			return false, err
		}
		owns, err := h.ownsCharacter(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), ids)
		if err != nil || owns {
			return owns, err
		}
	}
	return h.writesOwnEntry(c, cc)
}

// writesOwnEntry reports whether the viewer's own plan offers "make your
// own" for a list, or already holds an entry they wrote. A Director's
// permission must open the page it is used on even for a player who has no
// hero yet and may not make one.
func (h *Handler) writesOwnEntry(c *echo.Context, cc *campaigns.CampaignContext) (bool, error) {
	plan, err := h.heroPlan(c, cc)
	if err != nil || plan == nil {
		return false, err
	}
	for _, st := range plan.Steps {
		if st.CanMake {
			return true, nil
		}
		for _, ch := range st.Choices {
			if ch.Mine {
				return true, nil
			}
		}
	}
	return false, nil
}

// heroLookFrom reads ?look=: the stage look is the default, and only the exact
// value "classic" picks the other.
func heroLookFrom(v string) string {
	if v == HeroLookClassic {
		return HeroLookClassic
	}
	return HeroLookStage
}

// NewHero renders the hero creator (GET /campaigns/:id/characters/new).
func (h *Handler) NewHero(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	target, access, err := h.heroTarget(c, cc)
	if err != nil {
		return err
	}
	if !access.Allowed {
		if access.OwnHero != nil {
			return middleware.Render(c, http.StatusForbidden, HeroAlreadyPage(cc, access.OwnHero))
		}
		return apperror.NewForbidden(access.refusalMessage())
	}
	return middleware.Render(c, http.StatusOK, HeroCreatorPage(cc, HeroCreatorView{
		TypeName:  target.Name,
		ForSelf:   access.ForSelf,
		CSRFToken: middleware.GetCSRFToken(c),
		TermsURL:  h.termsURLIf(c.Request().Context(), cc.Campaign.ID, "/campaigns/"+cc.Campaign.ID+"/characters/new/terms"),
		Look:      heroLookFrom(c.QueryParam("look")),
	}))
}

// requireHeroPlanAccess is the gate on the creator's plan and its rules
// words. Directors also read the plan: the "Own entries" page takes its
// pick lists from it, whether or not they could make a hero themselves. So
// does a player who already has a hero, whose builder fills in that hero from
// the same lists.
func (h *Handler) requireHeroPlanAccess(c *echo.Context, cc *campaigns.CampaignContext) error {
	if cc.CanAuthorDmOnly() {
		return nil
	}
	ok, err := h.mayReadHeroPlan(c, cc)
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewForbidden("you can't create a hero in this campaign")
	}
	return nil
}

// HeroPlanAPI returns the creator's steps (GET /campaigns/:id/characters/new/plan).
func (h *Handler) HeroPlanAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.requireHeroPlanAccess(c, cc); err != nil {
		return err
	}
	// ?as= is a Director's preview as one player. The player's own pages (the
	// hero builder and creator) also ask with ?preview=player, so a Director
	// with "view as player" on gets the lists a player gets there, rules
	// applied; the Directors' tools that read the plan never ask, and keep
	// the whole list.
	as := c.QueryParam("as")
	if as == "" && c.QueryParam("preview") == "player" && campaigns.ViewingAsPlayer(c, cc) {
		as = auth.GetUserID(c)
	}
	if as != "" && cc.CanAuthorDmOnly() && len(as) <= 36 {
		plan, err := h.previewPlan(c, cc, as)
		if err != nil {
			return err
		}
		h.addHeroAbout(c, cc, plan)
		return c.JSON(http.StatusOK, plan)
	}
	plan, err := h.heroPlan(c, cc)
	if err != nil {
		return err
	}
	h.addHeroAbout(c, cc, plan)
	plan.CanPin = cc.CanAuthorDmOnly() && !plan.Practice && !plan.Tryout
	return c.JSON(http.StatusOK, plan)
}

// addHeroAbout gives the plan its "About your hero" step, from the fields of
// the page type the hero will be. A viewer who may not make one gets none.
func (h *Handler) addHeroAbout(c *echo.Context, cc *campaigns.CampaignContext, plan *HeroPlan) {
	if target, access, err := h.heroTarget(c, cc); err == nil && access.Allowed {
		plan.About = heroAbout(target)
	}
}

// CreateHero makes the hero from the creator's picks
// (POST /campaigns/:id/characters/new) and answers with its page address.
func (h *Handler) CreateHero(c *echo.Context) error {
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
		return apperror.NewForbidden(access.refusalMessage())
	}
	plan, err := h.heroPlan(c, cc)
	if err != nil {
		return err
	}
	fields, asks, err := buildHeroFields(plan, req.Picks)
	if err != nil {
		return err
	}
	if err := addHeroNumbers(plan, req, fields); err != nil {
		return err
	}
	about, err := heroAboutValues(heroAbout(target), req.About)
	if err != nil {
		return err
	}
	for k, v := range about {
		fields[k] = v
	}

	userID := auth.GetUserID(c)
	var owner *string
	if access.ForSelf {
		owner = &userID
	}
	// A Director's own creation follows the campaign's default for new pages;
	// a player's is never private.
	entity, err := createHeroPage(c.Request().Context(), h.service, cc.Campaign.ID, userID, req.Name, target, fields, owner,
		cc.NewPagePrivacy(patch.Absent[bool]()))
	if err != nil {
		return err
	}
	h.logAudit(c, cc.Campaign.ID, audit.ActionEntityCreated, entity.ID, entity.Name)
	resp := map[string]any{
		"id":  entity.ID,
		"url": "/campaigns/" + cc.Campaign.ID + "/entities/" + entity.ID,
	}
	// A pick of an ask-first entry waits for a Director instead of landing in
	// the hero's fields. The hero is already made, so a request that can't be
	// recorded is named for the player to ask again, not a failed creation.
	if len(asks) > 0 {
		requests, failed := h.recordAsks(c, cc.Campaign.ID, entity, asks)
		resp["requests"] = requests
		if len(failed) > 0 {
			resp["requestFailed"] = failed
		}
	}
	return c.JSON(http.StatusCreated, resp)
}

// recordAsks records each ask-first pick as a request for the hero, and names
// the fields whose request could not be recorded.
func (h *Handler) recordAsks(c *echo.Context, campaignID string, e *Entity, asks []HeroAsk) (done []HeroAskOutcome, failed []string) {
	done = []HeroAskOutcome{}
	for _, a := range asks {
		a.CampaignID, a.UserID, a.EntityID, a.EntityName = campaignID, auth.GetUserID(c), e.ID, e.Name
		if h.heroRules == nil {
			failed = append(failed, a.FieldKey)
			continue
		}
		out, err := h.heroRules.RequestChoice(c.Request().Context(), a)
		if err != nil || out == nil {
			failed = append(failed, a.FieldKey)
			continue
		}
		done = append(done, *out)
	}
	return done, failed
}

// OwnEntries renders the page for a campaign's own pick-list entries
// (GET /campaigns/:id/characters/entries): the Directors' list, or for a
// player, the entries a Director let them write. The entry API enforces the
// same rules on every write; this only keeps others off the page.
//
// ?field= opens that list; ?for= names the player's character the new entry
// is for, and the page links back to it (or to the hero creator, ?back=new).
// ?entry= opens the editor on one entry (?made=1 when a rewrite just made it)
// and ?new=1 on a new one; ?return= is the rules page to go back to after.
//
// A Director's bare address opens the page itself; Campaign settings and
// "Before you play in <campaign>" both lead here.
func (h *Handler) OwnEntries(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.heroPlanner == nil {
		return apperror.NewForbidden("only the campaign's Directors manage its own entries")
	}
	view := OwnEntriesView{Director: cc.CanAuthorDmOnly(), CSRFToken: middleware.GetCSRFToken(c), Field: c.QueryParam("field"),
		Entry: c.QueryParam("entry"), New: c.QueryParam("new") == "1", Made: c.QueryParam("made") == "1"}
	if view.Director {
		view.Return = sameCampaignRulesPath(cc.Campaign.ID, c.QueryParam("return"))
	} else {
		ok, err := h.mayReadHeroPlan(c, cc)
		if err != nil {
			return err
		}
		if !ok {
			return apperror.NewForbidden("only the campaign's Directors manage its own entries")
		}
		view.UserID = auth.GetUserID(c)
	}
	// The way back is a page of this campaign only: the character the entry
	// is for, when the viewer may edit it, or the hero creator.
	if id := c.QueryParam("for"); id != "" {
		if e, err := h.service.GetByID(c.Request().Context(), id); err == nil && e.CampaignID == cc.Campaign.ID &&
			CanEditIdentity(cc.MemberRole, e, auth.GetUserID(c)) {
			view.ForID, view.ForName = e.ID, e.Name
		}
	} else if c.QueryParam("back") == "new" {
		view.BackToCreator = true
	}
	return middleware.Render(c, http.StatusOK, OwnEntriesPage(cc, view))
}

// sameCampaignRulesPath keeps a way back only to a rules page of this
// campaign, so the editor never sends anyone off-site or elsewhere.
func sameCampaignRulesPath(campaignID, to string) string {
	prefix := "/campaigns/" + campaignID + "/systems/"
	u, err := url.Parse(to)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, prefix) || strings.Contains(u.Path, "..") {
		return ""
	}
	return u.RequestURI()
}
