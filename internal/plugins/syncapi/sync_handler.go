package syncapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// SyncHandler serves sync mapping CRUD endpoints under the REST API.
type SyncHandler struct {
	syncMappingSvc SyncMappingService
}

// NewSyncHandler creates a new sync handler.
func NewSyncHandler(syncMappingSvc SyncMappingService) *SyncHandler {
	return &SyncHandler{syncMappingSvc: syncMappingSvc}
}

// ListMappings returns all sync mappings for a campaign.
// GET /api/v1/campaigns/:id/sync/mappings?limit=50&offset=0
func (h *SyncHandler) ListMappings(c echo.Context) error {
	campaignID := c.Param("id")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	// Clamp pagination parameters to safe ranges.
	if limit < 0 {
		limit = 0
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	mappings, total, err := h.syncMappingSvc.ListMappings(c.Request().Context(), campaignID, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list sync mappings")
	}

	if mappings == nil {
		mappings = []SyncMapping{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":   mappings,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetMapping returns a single sync mapping by ID.
// GET /api/v1/campaigns/:id/sync/mappings/:mappingID
func (h *SyncHandler) GetMapping(c echo.Context) error {
	mappingID := c.Param("mappingID")
	mapping, err := h.syncMappingSvc.GetMapping(c.Request().Context(), mappingID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "sync mapping not found")
	}

	// Verify campaign scope.
	if mapping.CampaignID != c.Param("id") {
		return echo.NewHTTPError(http.StatusNotFound, "sync mapping not found")
	}

	return c.JSON(http.StatusOK, mapping)
}

// CreateMapping creates a new sync mapping.
// POST /api/v1/campaigns/:id/sync/mappings
func (h *SyncHandler) CreateMapping(c echo.Context) error {
	campaignID := c.Param("id")

	var input CreateSyncMappingInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	mapping, err := h.syncMappingSvc.CreateMapping(c.Request().Context(), campaignID, input)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, mapping)
}

// DeleteMapping removes a sync mapping.
// DELETE /api/v1/campaigns/:id/sync/mappings/:mappingID
func (h *SyncHandler) DeleteMapping(c echo.Context) error {
	mappingID := c.Param("mappingID")

	// Verify campaign scope.
	mapping, err := h.syncMappingSvc.GetMapping(c.Request().Context(), mappingID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "sync mapping not found")
	}
	if mapping.CampaignID != c.Param("id") {
		return echo.NewHTTPError(http.StatusNotFound, "sync mapping not found")
	}

	if err := h.syncMappingSvc.DeleteMapping(c.Request().Context(), mappingID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete sync mapping")
	}

	return c.NoContent(http.StatusNoContent)
}

// LookupMapping finds a sync mapping by Chronicle or external identity.
// GET /api/v1/campaigns/:id/sync/lookup?chronicle_type=entity&chronicle_id=X
// GET /api/v1/campaigns/:id/sync/lookup?external_system=foundry&external_id=X
func (h *SyncHandler) LookupMapping(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	chronicleType := c.QueryParam("chronicle_type")
	chronicleID := c.QueryParam("chronicle_id")
	externalSystem := c.QueryParam("external_system")
	externalID := c.QueryParam("external_id")

	// Look up by Chronicle identity.
	if chronicleType != "" && chronicleID != "" {
		if externalSystem == "" {
			externalSystem = "foundry"
		}
		mapping, err := h.syncMappingSvc.GetMappingByChronicle(ctx, campaignID, chronicleType, chronicleID, externalSystem)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "sync mapping not found")
		}
		return c.JSON(http.StatusOK, mapping)
	}

	// Look up by external identity.
	if externalSystem != "" && externalID != "" {
		mapping, err := h.syncMappingSvc.GetMappingByExternal(ctx, campaignID, externalSystem, externalID)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "sync mapping not found")
		}
		return c.JSON(http.StatusOK, mapping)
	}

	return echo.NewHTTPError(http.StatusBadRequest, "provide either chronicle_type+chronicle_id or external_system+external_id")
}

// PullMappings returns sync mappings modified since a given timestamp.
// GET /api/v1/campaigns/:id/sync/pull?since=2024-01-01T00:00:00Z&limit=100
func (h *SyncHandler) PullMappings(c echo.Context) error {
	campaignID := c.Param("id")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))

	sinceStr := c.QueryParam("since")
	var since time.Time
	if sinceStr != "" {
		var err error
		since, err = time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid since timestamp (use RFC3339)")
		}
	}

	resp, err := h.syncMappingSvc.PullModified(c.Request().Context(), campaignID, since, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to pull sync mappings")
	}

	return c.JSON(http.StatusOK, resp)
}
