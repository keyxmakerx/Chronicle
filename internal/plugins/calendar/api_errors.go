// api_errors.go — typed errors for the public Foundry-facing
// calendar API. These endpoints follow the cross-repo wire
// contract pinned in
// cordinator/decisions/2026-05-17-error-catalog-wire-contract.md:
// every error response is JSON
//
//	{ "error": "<code>", "message": "<text>", "category": "<bucket>" }
//
// where `category` is one of the five buckets the Foundry module
// consumes from `update-info.mjs` / `api-client.mjs`. The shape
// is intentionally identical to the foundry_vtt plugin's *Error
// type so a future drift guard can fold both producers into one
// contract. Kept here rather than imported from foundry_vtt to
// avoid creating a calendar→foundry_vtt import edge (the only
// existing edge is the opposite direction, via campaigns).
package calendar

import (
	"fmt"
	"net/http"
)

// APIErrCategory mirrors the five-bucket enum pinned in the
// error-catalog wire contract (auth / config / not_found /
// validation / internal).
type APIErrCategory string

const (
	APIErrCategoryAuth       APIErrCategory = "auth"
	APIErrCategoryConfig     APIErrCategory = "config"
	APIErrCategoryNotFound   APIErrCategory = "not_found"
	APIErrCategoryValidation APIErrCategory = "validation"
	APIErrCategoryInternal   APIErrCategory = "internal"
)

// APIError carries enough context for both the categorized HTTP
// response (status + JSON body) and the operator's server log
// (wrapped cause). Message follows the same four-clause format
// the foundry_vtt errors use: what / detail / cause / action.
type APIError struct {
	Category APIErrCategory
	Code     string
	Message  string
	Cause    error
}

// Error implements the error interface for typed errors emitted
// by the calendar API handlers. Includes the wrapped cause in
// log output so the operator can chase the root.
func (e *APIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s/%s] %s: %v", e.Category, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s/%s] %s", e.Category, e.Code, e.Message)
}

// Unwrap exposes the cause for errors.Is / errors.As.
func (e *APIError) Unwrap() error { return e.Cause }

// HTTPStatus maps category → HTTP status. Matches the foundry_vtt
// plugin's mapping exactly so the wire contract is consistent
// across the two error producers.
func (e *APIError) HTTPStatus() int {
	switch e.Category {
	case APIErrCategoryAuth:
		return http.StatusForbidden
	case APIErrCategoryConfig:
		return http.StatusServiceUnavailable
	case APIErrCategoryNotFound:
		return http.StatusNotFound
	case APIErrCategoryValidation:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// --- constructors (four-clause Message format) ---

// APIErrInvalidToken — caller hit a calendar endpoint with a
// missing, wrong-HMAC, or rotated per-campaign token. 403.
func APIErrInvalidToken(cause error) *APIError {
	return &APIError{
		Category: APIErrCategoryAuth,
		Code:     "invalid_token",
		Message: "Chronicle calendar URL has an invalid or rotated token: " +
			"the HMAC signature failed verification or the token has been " +
			"rotated since this URL was issued. " +
			"The campaign owner rotated the install URL or the URL was " +
			"copied incorrectly. " +
			"Open campaign settings → Integrations → Foundry VTT in Chronicle, " +
			"copy the current install URL, and reinstall the module in Foundry.",
		Cause: cause,
	}
}

// APIErrCalendarNotConfigured — campaign exists but no calendar
// is configured. 404 with structured body so the Foundry
// dashboard's silent-catch (sync-dashboard.mjs:567-571) keeps
// working; Chronicle MUST return a structured 404 here, not the
// framework's generic 404.
func APIErrCalendarNotConfigured(campaignID string) *APIError {
	return &APIError{
		Category: APIErrCategoryNotFound,
		Code:     "calendar_not_configured",
		Message: fmt.Sprintf(
			"No calendar is configured for campaign %q: "+
				"the campaign exists but the operator has not yet set up a calendar. "+
				"This is normal for campaigns that don't track in-world time. "+
				"The campaign owner sets up a calendar via Chronicle → campaign settings → Calendar.",
			campaignID),
	}
}

// APIErrEventNotFound — caller referenced an event ID that
// doesn't exist (or belongs to a different campaign's calendar).
// 404.
func APIErrEventNotFound(eventID string) *APIError {
	return &APIError{
		Category: APIErrCategoryNotFound,
		Code:     "event_not_found",
		Message: fmt.Sprintf(
			"Calendar event %q does not exist on this Chronicle instance: "+
				"the event was deleted or the ID is wrong. "+
				"A concurrent client deleted it or the module's local cache is stale. "+
				"The Foundry module should resync from /calendar/events to refresh its event map.",
			eventID),
	}
}

// APIErrValidation — request payload is malformed (missing
// required field, invalid value). 422.
func APIErrValidation(detail string) *APIError {
	return &APIError{
		Category: APIErrCategoryValidation,
		Code:     "invalid_request",
		Message: fmt.Sprintf(
			"Calendar API request payload is invalid: %s. "+
				"The Foundry module sent a field that doesn't match the wire contract. "+
				"This is most likely a module bug — open an issue against keyxmakerx/chronicle-foundry-module "+
				"with the request body that triggered this.",
			detail),
	}
}

// APIErrEventConflict — caller-supplied event ID (on PUT) refers
// to an event owned by a different calendar, or another caller
// concurrently modified the row. 422 (caller-driven; not a
// Chronicle-side failure).
func APIErrEventConflict(detail string) *APIError {
	return &APIError{
		Category: APIErrCategoryValidation,
		Code:     "event_conflict",
		Message: fmt.Sprintf(
			"Calendar event update conflicted with existing state: %s. "+
				"Two clients tried to write to the same event simultaneously, "+
				"or the event was deleted while this request was in flight. "+
				"The Foundry module should refetch /calendar/events and retry.",
			detail),
	}
}

// APIErrInternal — server-side fault (DB / FS error). 500.
// Message is intentionally generic; the wrapped cause is logged.
func APIErrInternal(code string, cause error) *APIError {
	return &APIError{
		Category: APIErrCategoryInternal,
		Code:     code,
		Message: "An internal Chronicle error occurred while serving the " +
			"calendar API. The Chronicle operator's server logs contain " +
			"the underlying cause. Report this with the request timestamp " +
			"to the Chronicle operator; if you ARE the operator, the cause " +
			"is in the slog output around this request.",
		Cause: cause,
	}
}

// AsAPIError extracts a *APIError from an error chain, returning
// nil if the error isn't typed.
func AsAPIError(err error) *APIError {
	if err == nil {
		return nil
	}
	if ae, ok := err.(*APIError); ok {
		return ae
	}
	return nil
}
