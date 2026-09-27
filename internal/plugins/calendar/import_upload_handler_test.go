// import_upload_handler_test.go covers the two multipart routes
// (PreviewImportAPI / CreateFromImportAPI) end to end against the HTTP
// layer, using the same newAccessTestRouter harness and fakeCalendarSvc as
// access_test.go's route-gate tests — those cover authorization on these
// routes, this file covers the request/response shape a real multipart
// upload takes through them.
package calendar

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newMultipartCalendarUpload builds a multipart/form-data request body with
// a "file" field carrying raw and, optionally, current_year/month/day form
// fields alongside it (CreateFromImportAPI reads those with formIntPtr).
func newMultipartCalendarUpload(t *testing.T, raw []byte, form map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", "calendar.json")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(raw); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	for k, v := range form {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("WriteField(%q): %v", k, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, w.FormDataContentType()
}

func doMultipartRequest(e *echo.Echo, method, path, userID string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", contentType)
	if userID != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userID})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestPreviewImportAPI_MissingFileIsBadRequest(t *testing.T) {
	e, _ := newAccessTestRouter(false, true, map[string]campaigns.Role{"u-owner": campaigns.RoleOwner})
	rec := doRequest(e, http.MethodPost, "/campaigns/camp-1/calendars/import/preview", "u-owner")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 with no file uploaded, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPreviewImportAPI_UploadedFileIsParsedAndReturned(t *testing.T) {
	e, _ := newAccessTestRouter(false, true, map[string]campaigns.Role{"u-owner": campaigns.RoleOwner})
	raw := calendariaSeasonFixture([]int{30, 31}, []fixtureSeason{{Name: "Only Season", DayStart: 1, DayEnd: 30}})
	body, ct := newMultipartCalendarUpload(t, raw, nil)

	rec := doMultipartRequest(e, http.MethodPost, "/campaigns/camp-1/calendars/import/preview", "u-owner", body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// fakeCalendarSvc.PreviewImport doesn't actually parse the upload (it
	// returns a fixed stub — access_test.go's doc comment on
	// fakeCalendarSvc explains why): this asserts the handler wired the
	// uploaded bytes through to the service and rendered its result, not
	// that parsing itself is correct (import_clamp_test.go and
	// export_import_events_test.go cover the parser).
	if !strings.Contains(rec.Body.String(), "Previewed Calendar") {
		t.Errorf("expected the service's parsed calendar name in the response, got: %s", rec.Body.String())
	}
}

func TestCreateFromImportAPI_CreatesAndReturnsCalendarWithWarnings(t *testing.T) {
	e, _ := newAccessTestRouter(false, true, map[string]campaigns.Role{"u-owner": campaigns.RoleOwner})
	raw := calendariaSeasonFixture([]int{30}, []fixtureSeason{{Name: "Only Season", DayStart: 1, DayEnd: 30}})
	body, ct := newMultipartCalendarUpload(t, raw, map[string]string{
		"current_year": "5", "current_month": "1", "current_day": "1",
	})

	rec := doMultipartRequest(e, http.MethodPost, "/campaigns/camp-1/calendars/import", "u-owner", body, ct)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"calendar"`) {
		t.Errorf("expected the response to carry the created calendar under \"calendar\", got: %s", rec.Body.String())
	}
}
