package syncapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// CALV5-PLACEHOLDER: V5 deletes this file and restores the real calendar
// REST handler; every method name here matches the one it replaces, so
// nothing else has to change back. Meanwhile every route stays REGISTERED
// (routes.go is untouched) and answers 503 rather than 200-with-empty-body,
// so the Foundry module's own degradation path treats the calendar as
// unreachable rather than as a real, empty calendar. The date-beacon
// tables and GET /calendar-sync-beacon are NOT part of this: they are
// syncapi's own and outlive the calendar.
//
// TODO(#778): Foundry sync API rewiring is deliberately out of scope for the
// calendar-v5 seams change (calendar-v5-sky-seams) — the operator's
// instruction was to leave every route here answering 503 and rewire the
// Foundry module separately, later.

// CalendarAPIHandler serves the calendar REST surface for external tools
// (Foundry VTT Calendaria sync). While the calendar is rebuilt it holds the
// routes open and reports the rebuild.
type CalendarAPIHandler struct{}

// NewCalendarAPIHandler creates the rebuild-state calendar API handler.
//
// CALV5-PLACEHOLDER: it took (syncSvc SyncAPIService, calendarSvc
// calendar.CalendarService). Both return with V5. TODO(#778)
func NewCalendarAPIHandler() *CalendarAPIHandler {
	return &CalendarAPIHandler{}
}

// calendarRebuilding is the one answer every route below gives: a
// structured Chronicle error (an `error` code the module can switch on, a
// `message` a GM can read). 503, not 404 — 404 would send the module down
// its old-build compatibility path and hide the real reason from the GM.
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
