package packages

import "github.com/labstack/echo/v4"

// ActivityRecorder is the slice of the admin change log this plugin writes to.
// It is declared here, not imported from the admin plugin, so the plugins stay
// independent; app wiring supplies an adapter that stamps the signed-in admin.
// Recording is best-effort: implementations must not fail the request.
type ActivityRecorder interface {
	RecordAdminChange(c echo.Context, action, targetType, targetID, label string)
}

// SetActivityRecorder wires the admin change log. Optional: nil disables it.
func (h *Handler) SetActivityRecorder(r ActivityRecorder) {
	h.activity = r
}

// recordActivity logs an admin change after it succeeded.
func (h *Handler) recordActivity(c echo.Context, action, targetType, targetID, label string) {
	if h.activity == nil {
		return
	}
	h.activity.RecordAdminChange(c, action, targetType, targetID, label)
}
