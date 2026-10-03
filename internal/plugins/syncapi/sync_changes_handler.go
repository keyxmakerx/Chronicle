package syncapi

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

const (
	syncChangesDefaultLimit = 500
	syncChangesMaxLimit     = 1000
)

// SyncChangesHandler serves GET /sync/changes.
type SyncChangesHandler struct {
	repo        SyncChangeRepository
	campaignSvc campaigns.CampaignService
}

// NewSyncChangesHandler creates the change feed handler.
func NewSyncChangesHandler(repo SyncChangeRepository, campaignSvc campaigns.CampaignService) *SyncChangesHandler {
	return &SyncChangesHandler{repo: repo, campaignSvc: campaignSvc}
}

type syncChangesResponse struct {
	Changes       []SyncChange `json:"changes"`
	Next          int64        `json:"next"`
	HasMore       bool         `json:"hasMore"`
	ResetRequired bool         `json:"resetRequired"`
}

// isDMEquivalent mirrors MapAPIHandler.canAuthorDmOnly: campaign Owner or a
// co-DM grant. Fails closed on any lookup error.
func (h *SyncChangesHandler) isDMEquivalent(c echo.Context) bool {
	key := GetAPIKey(c)
	if key == nil {
		return false
	}
	ctx := c.Request().Context()
	if member, err := h.campaignSvc.GetMember(ctx, key.CampaignID, key.UserID); err == nil && member.Role >= campaigns.RoleOwner {
		return true
	}
	granted, err := h.campaignSvc.IsUserDmGranted(ctx, key.CampaignID, key.UserID)
	return err == nil && granted
}

// ListChanges returns change ids after a cursor. DM-equivalent callers only:
// even the ids of private records are not for players.
// GET /api/v1/campaigns/:id/sync/changes?since=N&limit=500
func (h *SyncChangesHandler) ListChanges(c echo.Context) error {
	if !h.isDMEquivalent(c) {
		return apperror.NewForbidden("the change feed requires owner or co-DM access")
	}
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	since := int64(0)
	if v := c.QueryParam("since"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return apperror.NewBadRequest("since must be a non-negative integer")
		}
		since = n
	}
	limit := syncChangesDefaultLimit
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > syncChangesMaxLimit {
		limit = syncChangesMaxLimit
	}

	pruned, err := h.repo.PrunedThrough(ctx, campaignID)
	if err != nil {
		return err
	}
	if since < pruned {
		// The gap cannot be replayed. Hand back the head so the client can
		// resume from it after its full resync.
		head, err := h.repo.Head(ctx, campaignID)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, syncChangesResponse{Changes: []SyncChange{}, Next: head, ResetRequired: true})
	}

	// One extra row tells us whether more remain without a COUNT.
	rows, err := h.repo.List(ctx, campaignID, since, limit+1)
	if err != nil {
		return err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	next := since
	if len(rows) > 0 {
		next = rows[len(rows)-1].Seq
	}
	return c.JSON(http.StatusOK, syncChangesResponse{Changes: rows, Next: next, HasMore: hasMore})
}
