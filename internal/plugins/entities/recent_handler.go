package entities

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// maxRecentIDs caps one Recent check; the sidebar keeps ten pages.
const maxRecentIDs = 50

// maxRecentIDLen is longer than any page id; a longer value is skipped.
const maxRecentIDLen = 64

// ViewableIDsAPI answers which of the given pages the viewer can still open
// (GET /campaigns/:id/entities/viewable?ids=a,b,…). The sidebar's Recent list
// lives in the browser, so it asks before drawing and drops a page that was
// hidden or deleted since the visit. A page the viewer cannot see is absent,
// exactly like one that does not exist.
func (h *Handler) ViewableIDsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var ids []string
	seen := map[string]bool{}
	for _, id := range strings.Split(c.QueryParam("ids"), ",") {
		if id != "" && len(id) <= maxRecentIDLen && !seen[id] && len(ids) < maxRecentIDs {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	result := make([]string, 0, len(ids))
	if len(ids) > 0 {
		viewable, err := h.service.FilterViewableEntityIDs(c.Request().Context(), cc.Campaign.ID, ids, cc.VisibilityRole(), auth.GetUserID(c))
		if err != nil {
			return err
		}
		for _, id := range ids {
			if viewable[id] {
				result = append(result, id)
			}
		}
	}
	return c.JSON(http.StatusOK, map[string][]string{"ids": result})
}
