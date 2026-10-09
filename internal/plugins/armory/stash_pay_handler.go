package armory

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Pay handles POST /armory/pay: form character_id, amount (the sheet's main
// unit, up to two decimals) and an optional reason. It answers JSON so a
// widget can say who was paid what.
func (h *StashHandler) Pay(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(c.FormValue("amount")), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return apperror.NewBadRequest(payAmountMessage)
	}
	out, err := h.svc.Pay(c.Request().Context(), cc.Campaign.ID, a, PayInput{
		CharacterID: c.FormValue("character_id"),
		Amount:      Cents(math.Round(f * 100)),
		Reason:      c.FormValue("reason"),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
