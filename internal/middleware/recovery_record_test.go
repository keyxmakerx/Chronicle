package middleware

// recovery_record_test.go — panics reach the in-memory error ring.
//
// This test exists because of an asymmetry that is easy to miss: Recovery
// writes its own 500 with c.String and returns nil, so Echo never sees an error
// and app.errorHandler is NEVER CALLED for a recovered panic. Hooking only the
// error handler would therefore have left the single most valuable error class
// invisible in host.errors while the diagnostic looked like it was working —
// exactly the "absence read as evidence" failure the host.* diagnostics exist
// to prevent. If someone later removes the RecordPanic call as redundant, this
// fails.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/observability"
)

// runPanickingRequest sends one request through Recovery into a handler that
// panics, and returns the recorder.
func runPanickingRequest(t *testing.T, routeTemplate, rawPath string, panicValue any) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, rawPath, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(routeTemplate)

	h := Recovery()(func(echo.Context) error { panic(panicValue) })
	if err := h(c); err != nil {
		t.Fatalf("Recovery returned an error instead of handling the panic: %v", err)
	}
	return rec
}

func TestRecoveryRecordsPanic(t *testing.T) {
	before := observability.Recent(0).Total
	rec := runPanickingRequest(t, "/campaigns/:id", "/campaigns/abc", "boom in the handler")

	// The response contract is unchanged: still a plain 500.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 — recording must not change what the client gets", rec.Code)
	}
	if body := rec.Body.String(); body != "Internal Server Error" {
		t.Errorf("body = %q, want the unchanged generic message", body)
	}

	s := observability.Recent(1)
	if s.Total != before+1 {
		t.Fatalf("Total went %d -> %d, want exactly one recording", before, s.Total)
	}
	e := s.Entries[0]
	if e.Kind != observability.KindPanic {
		t.Errorf("Kind = %q, want %q", e.Kind, observability.KindPanic)
	}
	if e.Status != http.StatusInternalServerError {
		t.Errorf("Status = %d, want 500 (what Recovery actually sends)", e.Status)
	}
	if e.Path != "/campaigns/:id" || !e.PathIsTemplate {
		t.Errorf("Path = %q (templated=%v), want the route template", e.Path, e.PathIsTemplate)
	}
	if !strings.Contains(e.Err, "boom in the handler") {
		t.Errorf("Err = %q, want it to carry the panic value", e.Err)
	}
	if strings.Contains(e.Err, "goroutine ") {
		t.Errorf("a stack trace was stored: %q — the stack goes to the log, not into a 256-slot ring", e.Err)
	}
}

// TestRecoveryRecordsNonStringPanic: a panic value is `any`, and the common
// real case (a runtime error) is not a string. fmt.Sprint must render it rather
// than storing something like "%!s(*errors.errorString=...)".
func TestRecoveryRecordsNonStringPanic(t *testing.T) {
	before := observability.Recent(0).Total
	var s []int
	runPanickingRequest(t, "/x", "/x", func() any {
		defer func() {}()
		return recoverIndex(s)
	}())

	e := observability.Recent(1).Entries[0]
	if observability.Recent(0).Total != before+1 {
		t.Fatal("the panic was not recorded")
	}
	if !strings.Contains(e.Err, "index out of range") {
		t.Errorf("Err = %q, want the rendered runtime error", e.Err)
	}
}

// recoverIndex provokes a real runtime error value (rather than a hand-written
// string) so the test covers what an actual bug produces.
func recoverIndex(s []int) (v any) {
	defer func() { v = recover() }()
	_ = s[3]
	return nil
}
