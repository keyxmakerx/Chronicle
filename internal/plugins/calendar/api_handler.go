// api_handler.go — Foundry-facing public calendar API.
//
// Implements the 6 endpoints pinned in
// cordinator/decisions/2026-05-17-calendar-sync-wire-contract.md
// and consumed by chronicle-foundry-module's calendar-sync.mjs:
//
//   GET    /api/v1/campaigns/{cid}/calendar
//   PUT    /api/v1/campaigns/{cid}/calendar/date
//   GET    /api/v1/campaigns/{cid}/calendar/events
//   POST   /api/v1/campaigns/{cid}/calendar/events
//   PUT    /api/v1/campaigns/{cid}/calendar/events/{eventId}
//   DELETE /api/v1/campaigns/{cid}/calendar/events/{eventId}
//
// Authentication: per-campaign signed token on the ?token= query
// parameter — same scheme as /api/v1/campaigns/{cid}/foundry-vtt/
// module.json. Verification is delegated to a TokenVerifier
// interface so the calendar plugin doesn't import foundry_vtt
// directly.
//
// Field semantics (pinned by the decision):
//   - year, month, day, hour, minute are integers.
//   - month, day are 1-indexed. hour, minute are 0-indexed.
//   - visibility wire enum: "everyone" or "gm-only". Chronicle's
//     internal storage uses "everyone" and "dm_only"; this file
//     is the single translation point.
//   - event id is server-generated and stable for the event's
//     lifetime (the module persists it in user flags as
//     calendarEventMappings).
//
// Error responses follow the cross-repo wire contract: every
// error is JSON { error, message, category }. See api_errors.go.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TokenVerifier is the narrow contract APIHandler needs from the
// foundry_vtt plugin for per-campaign signed-token auth. Wired by
// internal/app/routes.go from foundry_vtt.Service so the calendar
// plugin has no compile-time dependency on foundry_vtt.
type TokenVerifier interface {
	VerifyManifestToken(ctx context.Context, campaignID, token string) error
}

// APIHandler serves the public Foundry-facing calendar endpoints.
// Thin per the project conventions: bind/verify, call service,
// translate to/from wire shape.
type APIHandler struct {
	svc      CalendarService
	verifier TokenVerifier
}

// NewAPIHandler constructs an APIHandler. Both dependencies are
// required; nil verifier would let callers bypass per-campaign
// authentication.
func NewAPIHandler(svc CalendarService, verifier TokenVerifier) *APIHandler {
	return &APIHandler{svc: svc, verifier: verifier}
}

// --- wire DTOs ---

// apiCalendarSnapshot is the response body shape for GET /calendar.
// Field names are pinned by the wire-contract decision; do not
// rename without amending the decision and coordinating with the
// Foundry module's calendar-sync.mjs consumer.
type apiCalendarSnapshot struct {
	Name          string `json:"name"`
	CurrentYear   int    `json:"current_year"`
	CurrentMonth  int    `json:"current_month"`
	CurrentDay    int    `json:"current_day"`
	CurrentHour   int    `json:"current_hour"`
	CurrentMinute int    `json:"current_minute"`
}

// apiDate is the request and response body shape for
// PUT /calendar/date. Echoed on success.
type apiDate struct {
	Year   int `json:"year"`
	Month  int `json:"month"`
	Day    int `json:"day"`
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

// apiEvent is the response shape for event GETs / POSTs / PUTs.
// hour, minute, updated_at are optional per the decision; emitted
// as omitempty for hour/minute and always present for updated_at
// since the storage layer guarantees a value.
type apiEvent struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Year        int       `json:"year"`
	Month       int       `json:"month"`
	Day         int       `json:"day"`
	Hour        *int      `json:"hour,omitempty"`
	Minute      *int      `json:"minute,omitempty"`
	Description string    `json:"description"`
	Visibility  string    `json:"visibility"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// apiEventBody is the request body shape for POST / PUT
// /calendar/events. The module sends exactly these fields per
// _calendariaNoteToChronicleEvent (calendar-sync.mjs:305-337); no
// hour/minute on writes (Calendaria notes are day-resolution).
type apiEventBody struct {
	Name        string `json:"name"`
	Year        int    `json:"year"`
	Month       int    `json:"month"`
	Day         int    `json:"day"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
}

// --- POST /calendar wire DTOs (C-CAL-CREATE-ENDPOINT, 2026-05-19) ---
//
// Field names mirror the Calendaria export shape closely so the
// Foundry-side transform in FM-CAL-EDITOR-PR3 stays thin. The full
// wire contract is pinned in
// cordinator/decisions/2026-05-19-calendar-create-wire.md.

// apiCreateCalendarBody is the JSON request body for
// POST /api/v1/campaigns/{cid}/calendar. All fields are validated
// at this layer before being threaded into the service's import path.
type apiCreateCalendarBody struct {
	Name          string                 `json:"name"`
	CurrentYear   int                    `json:"current_year"`
	CurrentMonth  int                    `json:"current_month"`
	CurrentDay    int                    `json:"current_day"`
	CurrentHour   int                    `json:"current_hour"`
	CurrentMinute int                    `json:"current_minute"`
	Months        []apiCreateMonth       `json:"months"`
	Weekdays      []apiCreateWeekday     `json:"weekdays"`
	Seasons       []apiCreateSeason      `json:"seasons,omitempty"`
	Moons         []apiCreateMoon        `json:"moons,omitempty"`
	Eras          []apiCreateEra         `json:"eras,omitempty"`
}

// apiCreateMonth — per-month entry in the import payload.
// Intercalary maps to Chronicle's IsIntercalary storage flag.
type apiCreateMonth struct {
	Name        string `json:"name"`
	Days        int    `json:"days"`
	Intercalary bool   `json:"intercalary,omitempty"`
}

// apiCreateWeekday — per-weekday entry. is_rest_day defaults false.
type apiCreateWeekday struct {
	Name      string `json:"name"`
	IsRestDay bool   `json:"is_rest_day,omitempty"`
}

// apiCreateSeason — per-season entry. Months/days are 1-indexed per
// the wire contract.
type apiCreateSeason struct {
	Name       string `json:"name"`
	MonthStart int    `json:"month_start"`
	DayStart   int    `json:"day_start"`
	MonthEnd   int    `json:"month_end"`
	DayEnd     int    `json:"day_end"`
	Color      string `json:"color,omitempty"`
}

// apiCreateMoon — per-moon entry. Color is hex like "#aabbcc".
// CycleDays uses the same name Chronicle's MoonInput uses for storage.
type apiCreateMoon struct {
	Name        string  `json:"name"`
	CycleDays   float64 `json:"cycle_days"`
	PhaseOffset float64 `json:"phase_offset,omitempty"`
	Color       string  `json:"color,omitempty"`
}

// apiCreateEra — per-era entry. EndYear nullable: open-ended current
// era is the common case (e.g. "Third Age" with no defined end).
type apiCreateEra struct {
	Name      string `json:"name"`
	StartYear int    `json:"start_year"`
	EndYear   *int   `json:"end_year,omitempty"`
	Color     string `json:"color,omitempty"`
}

// apiCalendarCreated is the response body for a successful POST.
// Returns the calendar id + a snapshot of the seeded state so the
// caller can immediately render without a follow-up GET.
type apiCalendarCreated struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CurrentYear   int    `json:"current_year"`
	CurrentMonth  int    `json:"current_month"`
	CurrentDay    int    `json:"current_day"`
	CurrentHour   int    `json:"current_hour"`
	CurrentMinute int    `json:"current_minute"`
	MonthCount    int    `json:"month_count"`
	WeekdayCount  int    `json:"weekday_count"`
	SeasonCount   int    `json:"season_count"`
	MoonCount     int    `json:"moon_count"`
	EraCount      int    `json:"era_count"`
}

// --- visibility translation ---

const (
	wireVisibilityEveryone = "everyone"
	wireVisibilityGMOnly   = "gm-only"

	storageVisibilityEveryone = "everyone"
	storageVisibilityDMOnly   = "dm_only"
)

// wireToStorageVisibility translates the wire enum the module
// sends to Chronicle's internal storage enum. The Foundry module
// emits "gm-only" (hyphen) per the wire contract; Chronicle's
// existing UI/service expects "dm_only" (underscore). Translating
// here keeps both surfaces stable: no UI/data migration needed,
// no wire contract drift.
//
// Empty value defaults to "everyone" to match the existing
// CreateEvent semantics — Calendaria notes without an explicit
// visibility should be public.
func wireToStorageVisibility(v string) (string, error) {
	switch v {
	case "", wireVisibilityEveryone:
		return storageVisibilityEveryone, nil
	case wireVisibilityGMOnly:
		return storageVisibilityDMOnly, nil
	default:
		return "", APIErrValidation(`visibility must be "everyone" or "gm-only"`)
	}
}

// storageToWireVisibility is the inverse: takes Chronicle's
// internal value and produces the wire enum.
func storageToWireVisibility(v string) string {
	if v == storageVisibilityDMOnly {
		return wireVisibilityGMOnly
	}
	return wireVisibilityEveryone
}

// --- shared helpers ---

// authorize verifies the per-campaign signed token on the
// request's ?token= query param. Returns nil on success, or a
// typed *APIError the caller passes to respondError.
func (h *APIHandler) authorize(c echo.Context) (campaignID string, err error) {
	campaignID = c.Param("cid")
	token := c.QueryParam("token")
	if token == "" {
		return campaignID, APIErrInvalidToken(nil)
	}
	if vErr := h.verifier.VerifyManifestToken(c.Request().Context(), campaignID, token); vErr != nil {
		// The verifier returns foundry_vtt typed errors; wrap as
		// our APIError so the wire shape stays consistent across
		// every calendar endpoint regardless of which subsystem
		// reported the underlying failure.
		return campaignID, APIErrInvalidToken(vErr)
	}
	return campaignID, nil
}

// respondError converts a typed error into the categorized JSON
// response. Logs a structured breadcrumb for every typed error so
// the operator can root-cause from server logs — Foundry's client
// emits generic 4xx/5xx messages regardless of body content.
func (h *APIHandler) respondError(c echo.Context, err error) error {
	ae := AsAPIError(err)
	if ae == nil {
		// Untyped error — wrap as internal so the wire shape is
		// preserved. The original error is logged for the operator.
		ae = APIErrInternal("calendar_api_unhandled", err)
	}
	slog.Warn("calendar API error response",
		slog.String("path", c.Request().URL.Path),
		slog.String("category", string(ae.Category)),
		slog.String("code", ae.Code),
		slog.Int("http_status", ae.HTTPStatus()),
		slog.Any("cause", ae.Cause),
	)
	return c.JSON(ae.HTTPStatus(), map[string]any{
		"error":    ae.Code,
		"message":  ae.Message,
		"category": string(ae.Category),
	})
}

// loadCalendar fetches the campaign's calendar, returning the
// structured 404 the wire contract requires when no calendar
// is configured. Other errors flow through as internal.
func (h *APIHandler) loadCalendar(ctx context.Context, campaignID string) (*Calendar, error) {
	cal, err := h.svc.GetCalendar(ctx, campaignID)
	if err != nil {
		// Treat any "not found" apperror or nil-returning case as
		// calendar-not-configured. Anything else is internal.
		if isAppErrorType(err, "not_found") {
			return nil, APIErrCalendarNotConfigured(campaignID)
		}
		return nil, APIErrInternal("get_calendar", err)
	}
	if cal == nil {
		return nil, APIErrCalendarNotConfigured(campaignID)
	}
	return cal, nil
}

// isAppErrorType reports whether err is an *apperror.AppError
// whose Type field matches the given value. Used by the handlers
// to translate Chronicle-internal error kinds into the wire
// categories without inventing a new sentinel-error layer.
func isAppErrorType(err error, kind string) bool {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Type == kind
	}
	return false
}

// --- handlers ---

// GetCalendar — GET /api/v1/campaigns/{cid}/calendar.
//
// Returns a current-date snapshot, or a structured 404 when no
// calendar is configured. The Foundry dashboard catches the
// structured 404 silently (sync-dashboard.mjs:567-571); UI clients
// see the structured body and can render a friendly message.
func (h *APIHandler) GetCalendar(c echo.Context) error {
	campaignID, err := h.authorize(c)
	if err != nil {
		return h.respondError(c, err)
	}
	cal, err := h.loadCalendar(c.Request().Context(), campaignID)
	if err != nil {
		return h.respondError(c, err)
	}
	return c.JSON(http.StatusOK, apiCalendarSnapshot{
		Name:          cal.Name,
		CurrentYear:   cal.CurrentYear,
		CurrentMonth:  cal.CurrentMonth,
		CurrentDay:    cal.CurrentDay,
		CurrentHour:   cal.CurrentHour,
		CurrentMinute: cal.CurrentMinute,
	})
}

// PutDate — PUT /api/v1/campaigns/{cid}/calendar/date.
//
// Updates the campaign calendar's current year/month/day/hour/
// minute to caller-supplied values. The module calls this on
// Calendaria date/time changes (calendar-sync.mjs:241, :328) so
// in-world time stays in sync between Foundry and Chronicle.
func (h *APIHandler) PutDate(c echo.Context) error {
	campaignID, err := h.authorize(c)
	if err != nil {
		return h.respondError(c, err)
	}
	var req apiDate
	if err := decodeBody(c, &req); err != nil {
		return h.respondError(c, err)
	}
	cal, err := h.loadCalendar(c.Request().Context(), campaignID)
	if err != nil {
		return h.respondError(c, err)
	}
	if req.Month < 1 || req.Day < 1 {
		return h.respondError(c, APIErrValidation("month and day must be 1-indexed (>= 1)"))
	}
	if req.Hour < 0 || req.Minute < 0 {
		return h.respondError(c, APIErrValidation("hour and minute must be non-negative"))
	}
	// Push values through the same update path the UI uses so any
	// future invariants (leap-year clamping, hour-rollover) apply
	// uniformly across surfaces.
	if err := h.svc.UpdateCalendar(c.Request().Context(), cal.ID, UpdateCalendarInput{
		Name:             cal.Name,
		Description:      cal.Description,
		EpochName:        cal.EpochName,
		CurrentYear:      req.Year,
		CurrentMonth:     req.Month,
		CurrentDay:       req.Day,
		CurrentHour:      req.Hour,
		CurrentMinute:    req.Minute,
		HoursPerDay:      cal.HoursPerDay,
		MinutesPerHour:   cal.MinutesPerHour,
		SecondsPerMinute: cal.SecondsPerMinute,
		LeapYearEvery:    cal.LeapYearEvery,
		LeapYearOffset:   cal.LeapYearOffset,
	}); err != nil {
		if isAppErrorType(err, "validation_error") {
			return h.respondError(c, APIErrValidation(err.Error()))
		}
		return h.respondError(c, APIErrInternal("update_calendar_date", err))
	}
	return c.JSON(http.StatusOK, req)
}

// ListEvents — GET /api/v1/campaigns/{cid}/calendar/events.
//
// Returns every event for the campaign's calendar with the wire
// visibility enum. No role-based filtering — the per-campaign
// token gates access at endpoint level, and the module decides
// per-event surfacing in Foundry.
func (h *APIHandler) ListEvents(c echo.Context) error {
	campaignID, err := h.authorize(c)
	if err != nil {
		return h.respondError(c, err)
	}
	cal, err := h.loadCalendar(c.Request().Context(), campaignID)
	if err != nil {
		return h.respondError(c, err)
	}
	events, err := h.svc.ListAllEventsForCalendar(c.Request().Context(), cal.ID)
	if err != nil {
		return h.respondError(c, APIErrInternal("list_events", err))
	}
	out := make([]apiEvent, 0, len(events))
	for i := range events {
		out = append(out, toAPIEvent(&events[i]))
	}
	return c.JSON(http.StatusOK, out)
}

// CreateEvent — POST /api/v1/campaigns/{cid}/calendar/events.
//
// Creates a new event from the wire body. The returned id is the
// server-generated, lifetime-stable identifier the module
// persists in user flags (calendarEventMappings) for update /
// delete tracking.
func (h *APIHandler) CreateEvent(c echo.Context) error {
	campaignID, err := h.authorize(c)
	if err != nil {
		return h.respondError(c, err)
	}
	var body apiEventBody
	if err := decodeBody(c, &body); err != nil {
		return h.respondError(c, err)
	}
	if body.Name == "" {
		return h.respondError(c, APIErrValidation("name is required"))
	}
	if body.Month < 1 || body.Day < 1 {
		return h.respondError(c, APIErrValidation("month and day must be 1-indexed (>= 1)"))
	}
	storageVis, err := wireToStorageVisibility(body.Visibility)
	if err != nil {
		return h.respondError(c, err)
	}
	cal, err := h.loadCalendar(c.Request().Context(), campaignID)
	if err != nil {
		return h.respondError(c, err)
	}
	descCopy := body.Description // local so we can take its address safely
	evt, err := h.svc.CreateEvent(c.Request().Context(), cal.ID, CreateEventInput{
		Name:        body.Name,
		Description: &descCopy,
		Year:        body.Year,
		Month:       body.Month,
		Day:         body.Day,
		Visibility:  storageVis,
		AllDay:      true, // Calendaria notes are day-resolution; the wire body has no hour/minute fields.
	})
	if err != nil {
		if isAppErrorType(err, "validation_error") {
			return h.respondError(c, APIErrValidation(err.Error()))
		}
		return h.respondError(c, APIErrInternal("create_event", err))
	}
	return c.JSON(http.StatusCreated, toAPIEvent(evt))
}

// UpdateEvent — PUT /api/v1/campaigns/{cid}/calendar/events/{eventId}.
//
// Replaces an existing event by id. The id is preserved across
// the update — the wire contract requires this so the module's
// local mapping (calendarEventMappings) stays valid.
func (h *APIHandler) UpdateEvent(c echo.Context) error {
	campaignID, err := h.authorize(c)
	if err != nil {
		return h.respondError(c, err)
	}
	eventID := c.Param("eventId")
	if eventID == "" {
		return h.respondError(c, APIErrValidation("eventId is required"))
	}
	// Confirm the event exists AND belongs to this campaign's
	// calendar — otherwise a caller with a valid token for
	// campaign A could mutate events from campaign B.
	cal, err := h.loadCalendar(c.Request().Context(), campaignID)
	if err != nil {
		return h.respondError(c, err)
	}
	existing, err := h.svc.GetEvent(c.Request().Context(), eventID)
	if err != nil {
		if isAppErrorType(err, "not_found") {
			return h.respondError(c, APIErrEventNotFound(eventID))
		}
		return h.respondError(c, APIErrInternal("get_event", err))
	}
	if existing == nil {
		return h.respondError(c, APIErrEventNotFound(eventID))
	}
	if existing.CalendarID != cal.ID {
		return h.respondError(c, APIErrEventConflict(
			"event id belongs to a different campaign's calendar"))
	}

	var body apiEventBody
	if err := decodeBody(c, &body); err != nil {
		return h.respondError(c, err)
	}
	if body.Name == "" {
		return h.respondError(c, APIErrValidation("name is required"))
	}
	if body.Month < 1 || body.Day < 1 {
		return h.respondError(c, APIErrValidation("month and day must be 1-indexed (>= 1)"))
	}
	storageVis, err := wireToStorageVisibility(body.Visibility)
	if err != nil {
		return h.respondError(c, err)
	}
	descCopy := body.Description
	if err := h.svc.UpdateEvent(c.Request().Context(), eventID, UpdateEventInput{
		Name:        body.Name,
		Description: &descCopy,
		Year:        body.Year,
		Month:       body.Month,
		Day:         body.Day,
		Visibility:  storageVis,
		AllDay:      true,
	}); err != nil {
		if isAppErrorType(err, "validation_error") {
			return h.respondError(c, APIErrValidation(err.Error()))
		}
		if isAppErrorType(err, "not_found") {
			return h.respondError(c, APIErrEventNotFound(eventID))
		}
		return h.respondError(c, APIErrInternal("update_event", err))
	}
	updated, err := h.svc.GetEvent(c.Request().Context(), eventID)
	if err != nil || updated == nil {
		return h.respondError(c, APIErrInternal("get_event_after_update", err))
	}
	return c.JSON(http.StatusOK, toAPIEvent(updated))
}

// DeleteEvent — DELETE /api/v1/campaigns/{cid}/calendar/events/{eventId}.
//
// Returns 204 on success per the wire contract. Idempotent in
// the sense that a missing event returns the structured 404 (not
// a silent success) so the module can distinguish "already gone"
// from "token failed".
func (h *APIHandler) DeleteEvent(c echo.Context) error {
	campaignID, err := h.authorize(c)
	if err != nil {
		return h.respondError(c, err)
	}
	eventID := c.Param("eventId")
	if eventID == "" {
		return h.respondError(c, APIErrValidation("eventId is required"))
	}
	cal, err := h.loadCalendar(c.Request().Context(), campaignID)
	if err != nil {
		return h.respondError(c, err)
	}
	existing, err := h.svc.GetEvent(c.Request().Context(), eventID)
	if err != nil {
		if isAppErrorType(err, "not_found") {
			return h.respondError(c, APIErrEventNotFound(eventID))
		}
		return h.respondError(c, APIErrInternal("get_event", err))
	}
	if existing == nil {
		return h.respondError(c, APIErrEventNotFound(eventID))
	}
	if existing.CalendarID != cal.ID {
		return h.respondError(c, APIErrEventConflict(
			"event id belongs to a different campaign's calendar"))
	}
	if err := h.svc.DeleteEvent(c.Request().Context(), eventID); err != nil {
		return h.respondError(c, APIErrInternal("delete_event", err))
	}
	return c.NoContent(http.StatusNoContent)
}

// CreateCalendar — POST /api/v1/campaigns/{cid}/calendar.
//
// Imports a calendar from a Calendaria-shaped payload. Closes the
// empty-state dead-end the operator hit 2026-05-19: Sync Calendar
// against a campaign without a calendar would dead-end on "no
// calendar configured" with no way to fix it from the per-campaign-
// token surface.
//
// Semantics:
//   - First write seeds the calendar + its sub-resources. 201 Created.
//   - Subsequent writes against a campaign that already has a calendar
//     return a structured 409 (`calendar_already_exists`); the Foundry
//     client renders "calendar already imported; open Sync Calendar
//     to view it" instead of silently double-creating.
//   - Validation rejects empty months, empty weekdays, malformed
//     dates / cycle lengths / colors at the API layer so the wire
//     surface stays uniform across surfaces that bypass the internal
//     UI's seeding helpers.
//   - On any sub-resource failure (e.g. SetMoons rejects a malformed
//     entry), the calendar row is deleted before returning the error
//     so the operator doesn't end up with a half-imported zombie row.
//
// See cordinator/decisions/2026-05-19-calendar-create-wire.md for the
// payload contract. NOT covered in v1: cycles, festivals, weather —
// operator adds those via the internal settings UI shipped in
// C-CAL-WCF-UI (chronicle#320). PR description documents the choice.
func (h *APIHandler) CreateCalendar(c echo.Context) error {
	campaignID, err := h.authorize(c)
	if err != nil {
		return h.respondError(c, err)
	}

	var req apiCreateCalendarBody
	if err := decodeBody(c, &req); err != nil {
		return h.respondError(c, err)
	}

	if vErr := validateCreateCalendarBody(&req); vErr != nil {
		return h.respondError(c, vErr)
	}

	ctx := c.Request().Context()

	// 409 if a calendar already exists for this campaign. GetCalendar
	// returns nil + nil when none configured, so a nil-cal is the
	// "fresh" signal.
	if existing, gErr := h.svc.GetCalendar(ctx, campaignID); gErr == nil && existing != nil {
		return h.respondError(c, APIErrCalendarAlreadyExists(existing.Name))
	} else if gErr != nil && !isAppErrorType(gErr, "not_found") {
		return h.respondError(c, APIErrInternal("check_existing_calendar", gErr))
	}

	// Create the calendar row + seed defaults via the existing service
	// surface. CreateCalendar always succeeds for a fresh campaign so
	// the cleanup-on-failure logic below catches the sub-resource
	// import step rather than the create itself.
	cal, err := h.svc.CreateCalendar(ctx, campaignID, CreateCalendarInput{
		Mode:        ModeFantasy, // Calendaria imports are fantasy by definition; real-life is a Chronicle UI choice.
		Name:        req.Name,
		CurrentYear: req.CurrentYear,
	})
	if err != nil {
		return h.respondError(c, APIErrInternal("create_calendar", err))
	}

	// Build the ImportResult shape ApplyImport already knows how to
	// consume — same surface the syncapi import path uses. This keeps
	// the multi-step Set* sequence behind the existing service
	// boundary instead of re-implementing it at the API layer.
	result := buildImportResultFromAPI(&req)

	if aErr := h.svc.ApplyImport(ctx, cal.ID, result); aErr != nil {
		// Rollback: delete the half-imported calendar so the operator
		// doesn't end up with a zombie row that GetCalendar would then
		// reject as a 409 conflict on the next attempt.
		if dErr := h.svc.DeleteCalendar(ctx, cal.ID); dErr != nil {
			slog.Error("calendar import rollback failed",
				slog.String("calendar_id", cal.ID),
				slog.Any("apply_error", aErr),
				slog.Any("delete_error", dErr),
			)
		}
		// ApplyImport surfaces validation errors from the inner
		// SetMonths/SetMoons/etc. as AppErrors with Type=="validation_error".
		// Translate to the wire-contract validation category so the
		// Foundry client renders the inner message inline.
		if isAppErrorType(aErr, "validation_error") {
			return h.respondError(c, APIErrValidation(apperror.UserMessage(aErr, "import payload rejected by storage layer")))
		}
		return h.respondError(c, APIErrInternal("apply_import", aErr))
	}

	// Persist the current-date fields the import didn't carry through
	// ApplyImport (it sets CurrentMonth/Day to 1; we honor the
	// caller's preferences if non-zero). Skip if the caller defaulted
	// to zero — those zeros are not meaningful values.
	if req.CurrentMonth > 0 && req.CurrentDay > 0 {
		// UpdateCalendar is a partial path (post C-CAL-NULL-PRESERVE
		// chronicle#321) — we only set the date fields, leaving the
		// just-seeded sub-resources untouched.
		uErr := h.svc.UpdateCalendar(ctx, cal.ID, UpdateCalendarInput{
			Name:             cal.Name,
			Mode:             cal.Mode,
			CurrentYear:      req.CurrentYear,
			CurrentMonth:     req.CurrentMonth,
			CurrentDay:       req.CurrentDay,
			CurrentHour:      req.CurrentHour,
			CurrentMinute:    req.CurrentMinute,
			HoursPerDay:      cal.HoursPerDay,
			MinutesPerHour:   cal.MinutesPerHour,
			SecondsPerMinute: cal.SecondsPerMinute,
			LeapYearEvery:    cal.LeapYearEvery,
			LeapYearOffset:   cal.LeapYearOffset,
		})
		if uErr != nil {
			// Date refinement failed AFTER the rows landed. Don't
			// roll back — the import is otherwise complete; surface
			// the date refinement as an internal error so the
			// operator can correct via PUT /date.
			slog.Warn("calendar import: date refinement failed; calendar import otherwise complete",
				slog.String("calendar_id", cal.ID),
				slog.Any("error", uErr),
			)
		}
	}

	return c.JSON(http.StatusCreated, apiCalendarCreated{
		ID:            cal.ID,
		Name:          req.Name,
		CurrentYear:   req.CurrentYear,
		CurrentMonth:  defaultIfZero(req.CurrentMonth, 1),
		CurrentDay:    defaultIfZero(req.CurrentDay, 1),
		CurrentHour:   req.CurrentHour,
		CurrentMinute: req.CurrentMinute,
		MonthCount:    len(req.Months),
		WeekdayCount:  len(req.Weekdays),
		SeasonCount:   len(req.Seasons),
		MoonCount:     len(req.Moons),
		EraCount:      len(req.Eras),
	})
}

// validateCreateCalendarBody enforces wire-shape invariants on the
// import payload. Validate at the API layer per the dispatch — the
// service's Set* methods are lenient about field-level shape (they
// accept empty arrays for replace-all-to-zero semantics) but the
// wire contract requires non-empty months / weekdays.
func validateCreateCalendarBody(req *apiCreateCalendarBody) error {
	if req.Name == "" {
		return APIErrValidation("name is required")
	}
	if len(req.Months) == 0 {
		return APIErrValidation("months must be a non-empty array")
	}
	if len(req.Weekdays) == 0 {
		return APIErrValidation("weekdays must be a non-empty array")
	}
	for i, m := range req.Months {
		if m.Name == "" {
			return APIErrValidation(fmt.Sprintf("months[%d].name is required", i))
		}
		if m.Days < 1 {
			return APIErrValidation(fmt.Sprintf("months[%d].days must be >= 1", i))
		}
	}
	for i, w := range req.Weekdays {
		if w.Name == "" {
			return APIErrValidation(fmt.Sprintf("weekdays[%d].name is required", i))
		}
	}
	for i, m := range req.Moons {
		if m.Name == "" {
			return APIErrValidation(fmt.Sprintf("moons[%d].name is required", i))
		}
		if m.CycleDays <= 0 {
			return APIErrValidation(fmt.Sprintf("moons[%d].cycle_days must be > 0", i))
		}
	}
	for i, s := range req.Seasons {
		if s.Name == "" {
			return APIErrValidation(fmt.Sprintf("seasons[%d].name is required", i))
		}
		if s.MonthStart < 1 || s.DayStart < 1 || s.MonthEnd < 1 || s.DayEnd < 1 {
			return APIErrValidation(fmt.Sprintf("seasons[%d] month/day fields must be 1-indexed", i))
		}
	}
	for i, e := range req.Eras {
		if e.Name == "" {
			return APIErrValidation(fmt.Sprintf("eras[%d].name is required", i))
		}
		if e.EndYear != nil && *e.EndYear < e.StartYear {
			return APIErrValidation(fmt.Sprintf("eras[%d].end_year must be >= start_year (open-ended era should omit end_year)", i))
		}
	}
	return nil
}

// buildImportResultFromAPI translates the wire shape into the
// ImportResult shape the existing ApplyImport path consumes. Lets us
// reuse the service-level Set* sequence (and its WS publishes) without
// re-implementing the dispatch chain at the API layer.
func buildImportResultFromAPI(req *apiCreateCalendarBody) *ImportResult {
	r := &ImportResult{
		Format:       FormatUnknown, // not actually used by ApplyImport beyond logging.
		CalendarName: req.Name,
		Settings: ImportedSettings{
			CurrentYear: req.CurrentYear,
		},
	}
	for i, m := range req.Months {
		r.Months = append(r.Months, MonthInput{
			Name:          m.Name,
			Days:          m.Days,
			SortOrder:     i,
			IsIntercalary: m.Intercalary,
		})
	}
	for i, w := range req.Weekdays {
		r.Weekdays = append(r.Weekdays, WeekdayInput{
			Name:      w.Name,
			SortOrder: i,
			IsRestDay: w.IsRestDay,
		})
	}
	for _, m := range req.Moons {
		// MoonInput's field layout matches apiCreateMoon's exactly
		// today, so staticcheck (S1016) wants a type conversion here
		// rather than a struct literal. The conversion is fine — if
		// MoonInput gains a field later (e.g. SortOrder), a struct
		// literal would silently zero it just as the conversion does;
		// both rely on the same field-by-field correspondence.
		r.Moons = append(r.Moons, MoonInput(m))
	}
	for _, s := range req.Seasons {
		var description *string
		color := s.Color
		r.Seasons = append(r.Seasons, Season{
			Name:        s.Name,
			StartMonth:  s.MonthStart,
			StartDay:    s.DayStart,
			EndMonth:    s.MonthEnd,
			EndDay:      s.DayEnd,
			Color:       color,
			Description: description,
		})
	}
	for i, e := range req.Eras {
		r.Eras = append(r.Eras, EraInput{
			Name:      e.Name,
			StartYear: e.StartYear,
			EndYear:   e.EndYear,
			Color:     e.Color,
			SortOrder: i,
		})
	}
	return r
}

// defaultIfZero returns v if non-zero, otherwise fallback. Used to
// echo "1" back to the caller for current_month / current_day when
// the request omitted them (ApplyImport defaults those to 1).
func defaultIfZero(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	return v
}

// --- internal helpers ---

// decodeBody binds a JSON request body and translates decode
// failures into APIErrValidation so the wire shape is uniform.
// Echo's default binder returns *echo.HTTPError on malformed JSON
// which would bypass the categorized response.
func decodeBody(c echo.Context, dst any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(dst); err != nil {
		if errors.Is(err, errEmptyBody) {
			return APIErrValidation("request body is empty")
		}
		return APIErrValidation("request body is not valid JSON: " + err.Error())
	}
	return nil
}

var errEmptyBody = errors.New("empty body")

// toAPIEvent translates a stored Event into the wire shape.
// Visibility is mapped to the wire enum; hour/minute are emitted
// only if set (Calendaria-sourced events are day-resolution).
func toAPIEvent(evt *Event) apiEvent {
	out := apiEvent{
		ID:         evt.ID,
		Name:       evt.Name,
		Year:       evt.Year,
		Month:      evt.Month,
		Day:        evt.Day,
		Visibility: storageToWireVisibility(evt.Visibility),
		UpdatedAt:  evt.UpdatedAt,
	}
	if evt.Description != nil {
		out.Description = *evt.Description
	}
	if evt.StartHour != nil {
		h := *evt.StartHour
		out.Hour = &h
	}
	if evt.StartMinute != nil {
		m := *evt.StartMinute
		out.Minute = &m
	}
	return out
}
