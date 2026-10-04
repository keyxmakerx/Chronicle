// shop_request_handler.go answers waiting purchase requests from the stashes
// page. It is the sibling of the move approve/decline handlers and answers the
// same way (an HTMX signal that reloads the page, or a redirect); the service
// enforces that only Owner visibility may answer.
package armory

import (
	"strconv"

	"github.com/labstack/echo/v4"
)

// requestID reads the :rid path parameter.
func requestID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("rid"), 10, 64)
	if err != nil || id < 1 {
		return 0, notFound("request")
	}
	return id, nil
}

// ApproveRequest handles POST /armory/purchase-requests/:rid/approve.
func (h *ShopBuyHandler) ApproveRequest(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	id, err := requestID(c)
	if err != nil {
		return err
	}
	req, err := h.svc.ApproveRequest(c.Request().Context(), cc.Campaign.ID, a, id)
	if err != nil {
		return err
	}
	if req.Status == PurchaseFailed {
		return doneWithTone(c, cc, "The request could not go through: "+req.Reason, "error")
	}
	return done(c, cc, "Approved.")
}

// DeclineRequest handles POST /armory/purchase-requests/:rid/decline.
func (h *ShopBuyHandler) DeclineRequest(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	id, err := requestID(c)
	if err != nil {
		return err
	}
	if _, err := h.svc.DeclineRequest(c.Request().Context(), cc.Campaign.ID, a, id); err != nil {
		return err
	}
	return done(c, cc, "Turned down.")
}
