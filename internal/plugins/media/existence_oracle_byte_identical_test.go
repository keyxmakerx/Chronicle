// existence_oracle_byte_identical_test.go pins that "exists but private"
// and "no such id" produce a RESPONSE indistinguishable to an anonymous
// caller — not just the same status code, but the same JSON body (Type,
// Message) and the same response headers written by Handler.Serve itself
// (setSecurityHeaders only runs after checkMediaAccess succeeds, so any
// header difference before that would leak which path was taken). It does
// NOT prove the two paths take the same TIME — see handler.go.
//
// The JSON body is reconstructed here rather than driven through
// app/app.go's real errorHandler, because media cannot import app without
// an import cycle; see error_handler_api_type_test.go in the app package
// for the pin that keeps the two in sync.
package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// serveRequestAnonymousWithRecorder is serveRequestAnonymous plus the
// ResponseRecorder, so a test can also inspect what headers Serve itself
// wrote (as opposed to what the framework's error-handling middleware
// would add afterward, which this package cannot drive without importing
// internal/app and creating a cycle).
func serveRequestAnonymousWithRecorder(t *testing.T, h *Handler, id string) (*apperror.AppError, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/media/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: id}})

	err := h.Serve(c)
	if err == nil {
		t.Fatalf("Serve(%q) with no session and no signature: expected a denial, got nil", id)
	}
	appErr, ok := err.(*apperror.AppError)
	if !ok {
		t.Fatalf("Serve(%q): expected *apperror.AppError, got %T: %v", id, err, err)
	}
	return appErr, rec
}

// errorHandlerJSONBody reconstructs the exact bytes app/app.go's
// errorHandler writes for an AppError on an API request. Duplicated here,
// not imported, because internal/app imports internal/plugins/media (for
// wiring) and the reverse import would cycle.
func errorHandlerJSONBody(t *testing.T, appErr *apperror.AppError) []byte {
	t.Helper()
	errorField := appErr.Type
	if errorField == "" {
		errorField = http.StatusText(appErr.Code)
	}
	body, err := json.Marshal(map[string]string{
		"error":   errorField,
		"message": appErr.Message,
	})
	if err != nil {
		t.Fatalf("marshal expected error body: %v", err)
	}
	return body
}

// TestServe_ExistenceOracle_ResponseByteIdentical checks not just "same
// status" but same status, same JSON body, and same headers written by
// the handler itself.
func TestServe_ExistenceOracle_ResponseByteIdentical(t *testing.T) {
	h := &Handler{
		signer:        NewURLSigner("test-secret"),
		memberChecker: &stubMemberChecker{},
		service: &idKeyedMediaService{
			files: map[string]*MediaFile{
				"file-exists-private": privateMediaFile(),
			},
		},
		entityVisibility: &fakeEntityVisibilityFilter{},
	}

	knownPrivate, recKnown := serveRequestAnonymousWithRecorder(t, h, "file-exists-private")
	unknown, recUnknown := serveRequestAnonymousWithRecorder(t, h, "totally-nonexistent-id")

	// Same status.
	if knownPrivate.Code != unknown.Code {
		t.Fatalf("status differs: known-private=%d unknown=%d", knownPrivate.Code, unknown.Code)
	}

	// Same JSON body app/app.go's errorHandler would actually write for
	// each AppError -- Type and Message both, not just Code.
	bodyKnown := errorHandlerJSONBody(t, knownPrivate)
	bodyUnknown := errorHandlerJSONBody(t, unknown)
	if string(bodyKnown) != string(bodyUnknown) {
		t.Errorf("JSON body differs: known-private=%s unknown=%s", bodyKnown, bodyUnknown)
	}

	// Same headers. Serve only calls setSecurityHeaders AFTER
	// checkMediaAccess succeeds, so on a denial (either path) it should
	// have written no headers of its own at all -- confirmed here rather
	// than assumed, so a future change that adds a header to one path but
	// not the other (e.g. logging a Retry-After only for the "found but
	// denied" case) would be caught.
	if got := recKnown.Header(); len(got) != 0 {
		t.Errorf("known-private path wrote headers before the error handler ran: %v", got)
	}
	if got := recUnknown.Header(); len(got) != 0 {
		t.Errorf("unknown-id path wrote headers before the error handler ran: %v", got)
	}
}
