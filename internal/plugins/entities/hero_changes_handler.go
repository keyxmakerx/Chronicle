package entities

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// hero_changes_handler.go is the Directors' side of when players may change
// their hero's picks: the section on the Characters page that sets the times
// and reopens one hero. The player's side is the lock the hero builder shows
// and the fields door enforces (hero_changes.go).

// HeroChangesView is the Directors' section; nil for everyone else.
type HeroChangesView struct {
	CampaignID string
	CSRFToken  string
	// Ruled is false when players may change their picks any time.
	Ruled   bool
	Summary string
	// The form starts from the stored rule, or from suggested numbers.
	AfterMaking bool
	MakingDays  int
	AfterLevel  bool
	FromLevel   int
	LevelDays   int
	// Rows is each claimed hero with its state, shown only under a rule.
	Rows []HeroChangeRow
	// Open keeps the form unfolded after a save that was refused.
	Open bool
}

// HeroChangeRow is one claimed hero in the Directors' list.
type HeroChangeRow struct {
	ID     string
	Name   string
	Player string
	State  string
	Open   bool
}

// heroChangeDefaults are the numbers the form suggests before a rule exists:
// a week after making, and a week after each new level.
const (
	heroChangeDefaultDays  = 7
	heroChangeDefaultLevel = 2
)

// heroRuleSummary says the rule in one sentence, as the Directors read it.
func heroRuleSummary(rule *campaigns.HeroChangeRule) string {
	if rule == nil {
		return "Players can change their hero's picks any time."
	}
	var parts []string
	if rule.AfterMakingDays > 0 {
		parts = append(parts, "for the first "+dayWord(rule.AfterMakingDays)+" after making it")
	}
	if rule.AfterLevel {
		p := "for " + dayWord(rule.AfterLevelDays) + " after each new level"
		if rule.FromLevel > 2 {
			p += fmt.Sprintf(" from level %d on", rule.FromLevel)
		}
		parts = append(parts, p)
	}
	return "Players can change their hero's picks " + strings.Join(parts, ", and ") + "."
}

// heroLockState is a hero's lock as the Directors' list says it.
func heroLockState(l HeroLock, now time.Time) string {
	switch {
	case l.Open && l.Until != nil:
		s := "Open " + dayWord(l.DaysLeft(now)) + " more"
		if l.Reopened {
			s += ", reopened by a Director"
		}
		return s
	case l.Open:
		return "Open"
	case l.NextLevel > 0:
		return fmt.Sprintf("Locked until level %d", l.NextLevel)
	}
	return "Locked"
}

// heroChangesSection gathers the Directors' section. heroes are the claimed
// characters the page lists. nil when the viewer is not a Director or the
// feature is not wired; a failed read leaves the section out.
func (h *Handler) heroChangesSection(c *echo.Context, cc *campaigns.CampaignContext, heroes []CastMember) *HeroChangesView {
	if h.heroChanges == nil || !cc.CanAuthorDmOnly() {
		return nil
	}
	ctx := c.Request().Context()
	rule, err := h.heroChanges.Rule(ctx, cc.Campaign.ID)
	if err != nil {
		slog.Warn("cast page: hero change rule failed", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return nil
	}
	v := &HeroChangesView{
		CampaignID: cc.Campaign.ID, CSRFToken: middleware.GetCSRFToken(c),
		Ruled: rule != nil, Summary: heroRuleSummary(rule),
		MakingDays: heroChangeDefaultDays, FromLevel: heroChangeDefaultLevel, LevelDays: heroChangeDefaultDays,
	}
	if rule != nil {
		v.AfterMaking, v.AfterLevel = rule.AfterMakingDays > 0, rule.AfterLevel
		if rule.AfterMakingDays > 0 {
			v.MakingDays = rule.AfterMakingDays
		}
		if rule.AfterLevel {
			v.FromLevel, v.LevelDays = max(rule.FromLevel, heroChangeDefaultLevel), rule.AfterLevelDays
		}
	}
	if rule == nil {
		return v
	}
	var claimed []Entity
	for _, m := range heroes {
		if m.Entity.OwnerUserID != nil {
			claimed = append(claimed, m.Entity)
		}
	}
	if len(claimed) == 0 {
		return v
	}
	locks, err := h.heroChanges.Locks(ctx, cc.Campaign.ID, claimed)
	if err != nil {
		slog.Warn("cast page: hero locks failed", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return v
	}
	names := map[string]string{}
	if h.memberLister != nil {
		if members, err := h.memberLister.ListMembers(ctx, cc.Campaign.ID); err == nil {
			for _, m := range members {
				names[m.UserID] = m.DisplayName
			}
		}
	}
	now := h.heroChanges.now()
	for _, e := range claimed {
		player := strings.TrimSpace(names[*e.OwnerUserID])
		if player == "" {
			player = "a player"
		}
		l := locks[e.ID]
		v.Rows = append(v.Rows, HeroChangeRow{ID: e.ID, Name: e.Name, Player: player, State: heroLockState(l, now), Open: l.Open})
	}
	return v
}

// SaveHeroChanges stores when players may change their hero's picks.
// Directors only; anyone else is answered as if the route were absent.
//
// Route: POST /campaigns/:id/characters/hero-changes
func (h *Handler) SaveHeroChanges(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.heroChanges == nil || !cc.CanAuthorDmOnly() {
		return apperror.NewNotFound("page not found")
	}
	var req struct {
		When       string `form:"when"`
		Making     string `form:"after_making"`
		MakingDays string `form:"making_days"`
		Level      string `form:"after_level"`
		FromLevel  string `form:"from_level"`
		LevelDays  string `form:"level_days"`
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 8*1024)
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	var rule *campaigns.HeroChangeRule
	if req.When == "set" {
		rule = &campaigns.HeroChangeRule{}
		if req.Making != "" {
			rule.AfterMakingDays = formInt(req.MakingDays)
			if rule.AfterMakingDays == 0 {
				return h.castClaimedPageAlert(c, cc, "Say how many days after making a hero the picks stay open.")
			}
		}
		if req.Level != "" {
			rule.AfterLevel = true
			rule.FromLevel = formInt(req.FromLevel)
			rule.AfterLevelDays = formInt(req.LevelDays)
		}
	}
	if err := h.heroChanges.SaveRule(c.Request().Context(), cc.Campaign.ID, rule); err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusBadRequest {
			return h.castClaimedPageAlert(c, cc, apperror.SafeMessage(err)+".")
		}
		return err
	}
	return h.castClaimedPage(c, cc, "Saved. "+heroRuleSummary(rule))
}

// ReopenHeroChanges opens one claimed hero's picks for some days, whatever
// the campaign's times say. Directors only.
//
// Route: POST /campaigns/:id/characters/hero-changes/:eid/reopen
func (h *Handler) ReopenHeroChanges(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.heroChanges == nil || !cc.CanAuthorDmOnly() {
		return apperror.NewNotFound("page not found")
	}
	var req struct {
		Days string `form:"days"`
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 8*1024)
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	ctx := c.Request().Context()
	e, err := h.service.GetByID(ctx, c.Param("eid"))
	if err != nil {
		return err
	}
	if e.CampaignID != cc.Campaign.ID || e.OwnerUserID == nil {
		return apperror.NewNotFound("character not found")
	}
	n := formInt(req.Days)
	if err := h.heroChanges.Reopen(ctx, e, n, auth.GetUserID(c)); err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusBadRequest {
			return h.castClaimedPageAlert(c, cc, apperror.SafeMessage(err)+".")
		}
		return err
	}
	return h.castClaimedPage(c, cc, fmt.Sprintf("%s's picks are open for %s.", e.Name, dayWord(n)))
}

// formInt reads a whole number from a form field; 0 when it is not one.
func formInt(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
