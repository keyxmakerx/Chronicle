// Package calendar - event_batch_handler.go serves the year track's batch
// save. The rules are in event_batch.go; this only binds, calls and answers.
package calendar

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// eventBatchRequest is the body of POST .../events/batch. Each list item has
// the shape of the single route it replaces, so a client builds one the same
// way it builds a lone write.
type eventBatchRequest struct {
	Create []createEventRequest `json:"create"`
	Update []struct {
		ID      string             `json:"id"`
		Changes updateEventRequest `json:"changes"`
	} `json:"update"`
	Delete  []string `json:"delete"`
	Weather *struct {
		Clear []DayDate         `json:"clear"`
		Set   []DayWeatherInput `json:"set"`
	} `json:"weather"`
}

// ApplyEventBatchAPI saves a draft's events and weather, or undoes one, all
// or nothing. Scribe+ on the route; the service refuses the whole batch when
// any item is more than the sender may do (delete is the Owner's, weather and
// dm_only events are the Director's).
// POST /campaigns/:id/calendars/:calid/events/batch
func (h *Handler) ApplyEventBatchAPI(c *echo.Context) error {
	bs, ok := h.svc.(EventBatchService)
	if !ok {
		return apperror.NewInternalMessage("saving a draft in one go is not available", nil)
	}
	cc := campaigns.GetCampaignContext(c)
	var req eventBatchRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	v := viewerFrom(c, cc)
	userID := auth.GetUserID(c)

	in := EventBatchInput{Delete: req.Delete}
	for _, e := range req.Create {
		in.Create = append(in.Create, e.toInput(v, userID))
	}
	for _, u := range req.Update {
		in.Update = append(in.Update, EventBatchUpdate{ID: u.ID, Input: UpdateEventInput(u.Changes)})
	}
	if req.Weather != nil {
		in.Weather = &EventBatchWeather{Clear: req.Weather.Clear, Set: req.Weather.Set}
	}
	out, err := bs.ApplyEventBatch(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, in, BatchActor{
		Viewer:    v,
		UserID:    userID,
		CanDelete: cc.MemberRole >= campaigns.RoleOwner,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
