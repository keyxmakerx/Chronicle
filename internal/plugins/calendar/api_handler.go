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
		if isAppErrorType(err, "validation") {
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
		if isAppErrorType(err, "validation") {
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
		if isAppErrorType(err, "validation") {
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
