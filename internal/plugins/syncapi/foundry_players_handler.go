// foundry_players_handler.go — the GM's Foundry client reports who is in the
// world (POST /sync/players), and the owner's Foundry and People pages read
// it back as HTMX fragments. See foundry_players.go.

package syncapi

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SetFoundryPlayers wires the players store. Without it the report endpoint
// answers 404 and the pages show no players table.
func (h *SyncHistoryHandler) SetFoundryPlayers(repo FoundryPlayerRepository) {
	h.players = repo
}

// ReportPlayers replaces the campaign's Foundry player list with the GM
// client's report. Only an owner or DM-access key may send it: a player's
// own client never holds a key, and the list names who is linked to whom.
// POST /api/v1/campaigns/:id/sync/players
func (h *SyncHistoryHandler) ReportPlayers(c echo.Context) error {
	if h.players == nil {
		return apperror.NewNotFound("player reports are not available")
	}
	if !h.isDMEquivalent(c) {
		return apperror.NewForbidden("reporting Foundry players requires owner or DM access")
	}
	var req reportPlayersRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	ctx := c.Request().Context()
	campaignID := c.Param("id")
	members, err := h.campaignSvc.ListMembers(ctx, campaignID)
	if err != nil {
		return err
	}
	ids := make(map[string]bool, len(members))
	for _, m := range members {
		ids[m.UserID] = true
	}
	players, err := cleanPlayers(req.Players, ids, h.now())
	if err != nil {
		return err
	}
	if err := h.players.Replace(ctx, campaignID, players); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]int{"stored": len(players)})
}

// playersView loads the table for the owner's pages.
func (h *SyncHistoryHandler) playersView(ctx context.Context, cc *campaigns.CampaignContext) (FoundryPlayersView, error) {
	v := FoundryPlayersView{CampaignID: cc.Campaign.ID, Now: h.now()}
	if h.players == nil {
		return v, nil
	}
	players, err := h.players.List(ctx, cc.Campaign.ID)
	if err != nil {
		return v, err
	}
	members, err := h.campaignSvc.ListMembers(ctx, cc.Campaign.ID)
	if err != nil {
		return v, err
	}
	refs := make([]MemberRef, 0, len(members))
	for _, m := range members {
		name := m.DisplayName
		if name == "" {
			name = m.Email
		}
		refs = append(refs, MemberRef{UserID: m.UserID, Name: name,
			Role: campaigns.MemberAccessLabel(cc.Campaign, m), Owner: m.Role == campaigns.RoleOwner})
	}
	v.Rows = buildPlayerRows(players, refs, v.Now)
	v.ReportedAt = latestReport(players)
	return v, nil
}

// PlayersFragment is the Players in Foundry card body.
// GET /campaigns/:id/foundry/players
func (h *SyncHistoryHandler) PlayersFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.playersView(c.Request().Context(), cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, FoundryPlayersFragment(v))
}

// MemberStatusFragment fills the People table's "In Foundry" cells with
// out-of-band swaps, one per member.
// GET /campaigns/:id/foundry/member-status
func (h *SyncHistoryHandler) MemberStatusFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.playersView(c.Request().Context(), cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, FoundryMemberStatusOOB(v))
}

// problemsShown is how many recent failures the Foundry page lists.
const problemsShown = 5

// ProblemsFragment lists the latest failed syncs in plain words.
// GET /campaigns/:id/foundry/problems
func (h *SyncHistoryHandler) ProblemsFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	events, err := h.repo.List(c.Request().Context(), cc.Campaign.ID, SyncHistoryFilter{FailedOnly: true, Limit: problemsShown})
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, FoundryProblemsFragment(cc.Campaign.ID, events, h.now()))
}

// RegisterFoundryPageRoutes mounts the owner's Foundry page fragments. Owner
// only: the page is, and what Foundry reports about people is the owner's
// diagnostic, never shown to players.
func RegisterFoundryPageRoutes(e *echo.Echo, h *SyncHistoryHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id", auth.RequireAuth(authSvc), campaigns.RequireCampaignAccess(campaignSvc))
	owner := campaigns.RequireRole(campaigns.RoleOwner)
	cg.GET("/foundry/players", h.PlayersFragment, owner)
	cg.GET("/foundry/member-status", h.MemberStatusFragment, owner)
	cg.GET("/foundry/problems", h.ProblemsFragment, owner)
}

// prunePlayers drops lists Foundry has not refreshed in a while.
func prunePlayers(ctx context.Context, repo FoundryPlayerRepository, now time.Time) (int64, error) {
	if repo == nil {
		return 0, nil
	}
	return repo.PruneOlderThan(ctx, now.Add(-foundryPlayersRetention))
}
