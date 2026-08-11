package app

// error_handler_record_test.go — the errorHandler's in-memory error recording.
//
// Two things are under test and the second matters more than the first: that
// errors reach the ring host.errors reads, and that ADDING that recording
// changed nothing about what the client receives. A diagnostic that alters
// production error responses is a regression, not an observability win, so
// every assertion about the ring is paired with one about the wire.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/observability"
)

// runHandlerWithRoute drives errorHandler for one error against a request whose
// router match is routeTemplate, returning the recorder. SetPath is what makes
// c.Path() answer a template, exactly as the real router does.
func runHandlerWithRoute(t *testing.T, err error, method, routeTemplate, rawPath string, hdrs map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, rawPath, nil)
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(routeTemplate)
	(&App{}).errorHandler(err, c)
	return rec
}

// newestRecorded returns the newest ring entry, failing the test if the ring
// did not grow past the baseline the caller captured.
func newestRecorded(t *testing.T, baseline uint64) observability.Entry {
	t.Helper()
	s := observability.Recent(1)
	if s.Total <= baseline {
		t.Fatalf("nothing was recorded (Total stayed at %d)", s.Total)
	}
	if len(s.Entries) == 0 {
		t.Fatal("ring reports a recording but returned no entries")
	}
	return s.Entries[0]
}

// TestErrorHandlerRecords5xx: the case the whole feature exists for.
func TestErrorHandlerRecords5xx(t *testing.T) {
	before := observability.Recent(0).Total
	rec := runHandlerWithRoute(t, apperror.NewInternalMessage("boom", errors.New("underlying")),
		http.MethodGet, "/campaigns/:id/calendar", "/campaigns/abc/calendar", nil)

	e := newestRecorded(t, before)
	if e.Status != http.StatusInternalServerError {
		t.Errorf("recorded status = %d, want 500", e.Status)
	}
	if e.Method != http.MethodGet {
		t.Errorf("recorded method = %q, want GET", e.Method)
	}
	if e.Path != "/campaigns/:id/calendar" || !e.PathIsTemplate {
		t.Errorf("recorded path = %q (templated=%v), want the route template", e.Path, e.PathIsTemplate)
	}
	if e.Kind != observability.KindApp {
		t.Errorf("recorded kind = %q, want %q for an *apperror.AppError", e.Kind, observability.KindApp)
	}
	// The response itself must be exactly what it was before recording existed.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("response status = %d, want 500 — recording must not change the wire", rec.Code)
	}
}

// TestErrorHandlerDoesNotRecord4xx is the eviction guard at the integration
// level: a 404 storm reaching this handler must leave the ring untouched.
func TestErrorHandlerDoesNotRecord4xx(t *testing.T) {
	before := observability.Recent(0).Total
	for i := 0; i < 50; i++ {
		runHandlerWithRoute(t, echo.NewHTTPError(http.StatusNotFound, "not found"),
			http.MethodGet, "", "/wp-admin/setup-config.php", nil)
	}
	runHandlerWithRoute(t, apperror.NewUnauthorized("auth required"), http.MethodGet, "/x", "/x", nil)
	runHandlerWithRoute(t, apperror.NewForbidden("nope"), http.MethodGet, "/x", "/x", nil)

	if after := observability.Recent(0).Total; after != before {
		t.Errorf("Total went %d -> %d: 4xx are being recorded, and a 404 storm will evict the 500 an operator came for", before, after)
	}
}

// TestErrorHandlerKindClassification pins the provenance labels. On the wire a
// deliberate 500 and an unwrapped error escaping a handler are identical; to
// whoever has to fix it they are not, and `raw` is the one that is a bug.
func TestErrorHandlerKindClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want observability.Kind
	}{
		{"domain error", apperror.NewInternalMessage("boom", nil), observability.KindApp},
		{"echo HTTPError", echo.NewHTTPError(http.StatusBadGateway, "upstream"), observability.KindHTTP},
		{"unwrapped error", errors.New("something escaped a handler"), observability.KindRaw},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := observability.Recent(0).Total
			runHandlerWithRoute(t, tt.err, http.MethodPost, "/x", "/x", nil)
			if got := newestRecorded(t, before).Kind; got != tt.want {
				t.Errorf("kind = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestErrorHandlerDoesNotRecordCommittedResponses: the very first thing the
// handler does is bail on a committed response. Recording is placed after that
// guard, so a committed response must still record nothing — otherwise the ring
// would fill with errors nobody was ever shown.
func TestErrorHandlerDoesNotRecordCommittedResponses(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/x")
	_ = c.String(http.StatusOK, "already sent") // commits the response

	before := observability.Recent(0).Total
	(&App{}).errorHandler(apperror.NewInternalMessage("too late", nil), c)
	if after := observability.Recent(0).Total; after != before {
		t.Errorf("Total went %d -> %d: an already-committed response was recorded", before, after)
	}
}

// TestErrorHandlerRawPathOnlyWhenUnmatched: the privacy rule at the integration
// level. Chronicle routes /rsvp/:token, so a matched route must store the
// template and never the live token in the URL.
func TestErrorHandlerNeverRecordsTokenInPath(t *testing.T) {
	before := observability.Recent(0).Total
	runHandlerWithRoute(t, apperror.NewInternalMessage("rsvp exploded", nil),
		http.MethodGet, "/rsvp/:token", "/rsvp/live-token-value-9f3c", nil)

	e := newestRecorded(t, before)
	if strings.Contains(e.Path, "live-token-value") {
		t.Errorf("a live token reached the error ring via the path: %q", e.Path)
	}
	if e.Path != "/rsvp/:token" {
		t.Errorf("recorded path = %q, want the template", e.Path)
	}
}

// TestErrorHandlerResponsesUnchanged re-asserts the pre-existing contract from
// the handler's other test file for the paths recording touches, so a future
// change to the recording block cannot quietly move a status code or a header.
func TestErrorHandlerResponsesUnchanged(t *testing.T) {
	t.Run("API 5xx still returns JSON with the same status", func(t *testing.T) {
		rec := runHandlerWithRoute(t, apperror.NewInternalMessage("boom", nil), http.MethodGet, "/api/v1/x", "/api/v1/x",
			map[string]string{"Accept": "application/json"})
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type = %q, want JSON", ct)
		}
	})

	t.Run("HTMX 5xx still retargets the body", func(t *testing.T) {
		rec := runHandlerWithRoute(t, apperror.NewInternalMessage("boom", nil), http.MethodGet, "/x", "/x",
			map[string]string{"HX-Request": "true"})
		if rec.Header().Get("HX-Retarget") != "body" {
			t.Errorf("HX-Retarget = %q, want body", rec.Header().Get("HX-Retarget"))
		}
	})

	t.Run("HTMX 4xx still toasts and does not swap", func(t *testing.T) {
		rec := runHandlerWithRoute(t, apperror.NewValidation("bad input"), http.MethodPost, "/x", "/x",
			map[string]string{"HX-Request": "true"})
		if rec.Header().Get("HX-Trigger") == "" {
			t.Error("HX-Trigger missing")
		}
		if rec.Header().Get("HX-Reswap") != "none" {
			t.Errorf("HX-Reswap = %q, want none", rec.Header().Get("HX-Reswap"))
		}
	})
}
