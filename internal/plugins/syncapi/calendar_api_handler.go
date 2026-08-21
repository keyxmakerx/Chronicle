package syncapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// CALV5-PLACEHOLDER: this file replaces a 1601-line calendar REST surface while
// the calendar plugin is rebuilt (V5).
//
// WHY 503 AND NOT AN EMPTY BODY. The other side of these routes is a SHIPPED
// PRODUCT — the Chronicle Sync module for Foundry VTT — pointed at a live game
// world. A handler that answered 200 with `{}` or `[]` would tell the module
// that the campaign HAS a calendar and it is empty, and the module would
// faithfully apply that emptiness: it would find no events for the back-catalog
// window, and its structure comparison would read a zero-month calendar as a
// mismatch against the GM's real Calendaria world. "Unreachable" is a state the
// module already handles correctly and fails open on; "empty" is one it
// believes.
//
// So every route below stays REGISTERED (routes.go is untouched — the module's
// requests reach a real endpoint rather than a 404 that it would read as an old
// Chronicle build) and answers 503 with a structured, machine-readable body.
// The module's existing degradation path tolerates this; see the module's
// API-CONTRACT.md error-shape section.
//
// The date-beacon tables and GET /calendar-sync-beacon are deliberately NOT
// part of this: they are syncapi's own, they outlive the calendar, and V5
// reuses the same seen/applied model.
//
// V5 DELETES THIS FILE and restores the real handler. Every method name here
// matches the one it replaces, so nothing else has to change back.

// CalendarAPIHandler serves the calendar REST surface for external tools
// (Foundry VTT Calendaria sync). While the calendar is rebuilt it holds the
// routes open and reports the rebuild.
type CalendarAPIHandler struct{}

// NewCalendarAPIHandler creates the rebuild-state calendar API handler.
//
// CALV5-PLACEHOLDER: it took (syncSvc SyncAPIService, calendarSvc
// calendar.CalendarService). Both return with V5.
func NewCalendarAPIHandler() *CalendarAPIHandler {
	return &CalendarAPIHandler{}
}

// calendarRebuilding is the one answer every route below gives.
//
// The shape matches the module's expectations for a structured Chronicle error
// (an `error` code it can switch on, a `message` a GM can read). 503 is chosen
// over 404 deliberately: 404 means "this Chronicle is too old to have the
// endpoint", which would send the module down its old-build compatibility path
// and hide the real reason from the GM.
func calendarRebuilding(c echo.Context) error {
	return c.JSON(http.StatusServiceUnavailable, map[string]string{
		"error": "calendar_rebuilding",
		"message": "Chronicle's calendar is being rebuilt and is temporarily " +
			"unavailable. Calendar sync is paused; maps, actors, items and notes " +
			"are unaffected.",
	})
}

// ListCalendars is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) ListCalendars(c echo.Context) error { return calendarRebuilding(c) }

// GetCalendar is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetCalendar(c echo.Context) error { return calendarRebuilding(c) }

// GetCurrentDate is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetCurrentDate(c echo.Context) error { return calendarRebuilding(c) }

// ConfirmDate is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) ConfirmDate(c echo.Context) error { return calendarRebuilding(c) }

// GetSeasons is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetSeasons(c echo.Context) error { return calendarRebuilding(c) }

// GetMoons is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetMoons(c echo.Context) error { return calendarRebuilding(c) }

// GetEras is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetEras(c echo.Context) error { return calendarRebuilding(c) }

// GetEventCategories is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetEventCategories(c echo.Context) error { return calendarRebuilding(c) }

// GetStructure is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetStructure(c echo.Context) error { return calendarRebuilding(c) }

// GetWeather is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetWeather(c echo.Context) error { return calendarRebuilding(c) }

// GetWorldState is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetWorldState(c echo.Context) error { return calendarRebuilding(c) }

// GetCycles is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetCycles(c echo.Context) error { return calendarRebuilding(c) }

// GetFestivals is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetFestivals(c echo.Context) error { return calendarRebuilding(c) }

// ListEvents is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) ListEvents(c echo.Context) error { return calendarRebuilding(c) }

// GetEvent is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) GetEvent(c echo.Context) error { return calendarRebuilding(c) }

// CreateEvent is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) CreateEvent(c echo.Context) error { return calendarRebuilding(c) }

// UpdateEvent is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateEvent(c echo.Context) error { return calendarRebuilding(c) }

// DeleteEvent is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) DeleteEvent(c echo.Context) error { return calendarRebuilding(c) }

// SetDate is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) SetDate(c echo.Context) error { return calendarRebuilding(c) }

// AdvanceDate is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) AdvanceDate(c echo.Context) error { return calendarRebuilding(c) }

// AdvanceTime is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) AdvanceTime(c echo.Context) error { return calendarRebuilding(c) }

// UpdateCalendarSettings is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateCalendarSettings(c echo.Context) error {
	return calendarRebuilding(c)
}

// UpdateMonths is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateMonths(c echo.Context) error { return calendarRebuilding(c) }

// UpdateWeekdays is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateWeekdays(c echo.Context) error { return calendarRebuilding(c) }

// UpdateMoons is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateMoons(c echo.Context) error { return calendarRebuilding(c) }

// UpdateEras is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateEras(c echo.Context) error { return calendarRebuilding(c) }

// UpdateSeasons is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateSeasons(c echo.Context) error { return calendarRebuilding(c) }

// UpdateEventCategories is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateEventCategories(c echo.Context) error {
	return calendarRebuilding(c)
}

// SetWeather is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) SetWeather(c echo.Context) error { return calendarRebuilding(c) }

// UpdateCycles is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateCycles(c echo.Context) error { return calendarRebuilding(c) }

// UpdateFestivals is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) UpdateFestivals(c echo.Context) error { return calendarRebuilding(c) }

// ExportCalendar is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) ExportCalendar(c echo.Context) error { return calendarRebuilding(c) }

// ImportCalendar is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) ImportCalendar(c echo.Context) error { return calendarRebuilding(c) }

// CreateCalendar is unavailable while the calendar is rebuilt.
func (h *CalendarAPIHandler) CreateCalendar(c echo.Context) error { return calendarRebuilding(c) }
