package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestReportOnlyCSP pins the script rule being watched: every page carries
// it in report-only mode, and the enforcing policy is not tightened by it.
func TestReportOnlyCSP(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	h := SecurityHeaders()(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := h(e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns", nil), rec)); err != nil {
		t.Fatal(err)
	}
	ro := rec.Header().Get("Content-Security-Policy-Report-Only")
	for _, want := range []string{"script-src 'self' 'report-sample'", "report-uri /csp-report"} {
		if !strings.Contains(ro, want) {
			t.Errorf("report-only policy %q lacks %q", ro, want)
		}
	}
	if strings.Contains(ro, "unsafe-inline") || strings.Contains(ro, "unsafe-eval") {
		t.Errorf("report-only policy %q must not allow inline code or eval", ro)
	}
	if strings.Contains(ro, "report-to") {
		t.Errorf("report-only policy %q: report-to makes Chromium skip report-uri, and it only reports over HTTPS", ro)
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "'unsafe-inline'") {
		t.Error("the enforcing policy must stay as it is until the report-only list is worked through")
	}
}

// TestParseCSPReports reads both formats and keeps only what finds the
// script again: no query strings (they can carry tokens), ids folded.
func TestParseCSPReports(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []cspViolation
	}{
		{
			"legacy inline",
			`{"csp-report":{"document-uri":"https://c.example/campaigns/0b7f3c2a-1111-4222-8333-444455556666/entities/42?token=secret#x","blocked-uri":"inline","violated-directive":"script-src","effective-directive":"script-src-elem","source-file":"https://c.example/campaigns/0b7f3c2a-1111-4222-8333-444455556666","line-number":12,"script-sample":"window.x = 1"}}`,
			[]cspViolation{{"script-src-elem", "c.example/campaigns/:id/entities/:id", "inline", "c.example/campaigns/:id", 12, "window.x = 1"}},
		},
		{
			"reporting api list",
			`[{"type":"csp-violation","body":{"documentURL":"https://c.example/admin?q=1","blockedURL":"eval","effectiveDirective":"script-src","sourceFile":"https://c.example/static/js/x.js?v=3","lineNumber":7,"sample":""}},{"type":"deprecation","body":{}}]`,
			[]cspViolation{{"script-src", "c.example/admin", "eval", "c.example/static/js/x.js", 7, ""}},
		},
		{
			"control characters and extension urls",
			`{"csp-report":{"document-uri":"https://c.example/","blocked-uri":"chrome-extension://abc/inject.js","effective-directive":"script-src-elem","script-sample":"a\nb"}}`,
			[]cspViolation{{"script-src-elem", "c.example/", "chrome-extension:", "", 0, "a b"}},
		},
		{"not json", `hello`, nil},
		{"empty object", `{}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCSPReports([]byte(tc.body))
			if len(got) != len(tc.want) {
				t.Fatalf("got %d violations %+v, want %d", len(got), got, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("violation %d:\n got %+v\nwant %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestCSPReportHandlerLogsEachViolationOnce: a page load that repeats a
// violation logs it once, and the table stops growing at its limit.
func TestCSPReportHandlerLogsEachViolationOnce(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	e := echo.New()
	h := CSPReportHandler()
	send := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, CSPReportPath, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/csp-report")
		rec := httptest.NewRecorder()
		if err := h(e.NewContext(req, rec)); err != nil {
			t.Fatal(err)
		}
		return rec.Code
	}
	report := `{"csp-report":{"document-uri":"https://c.example/campaigns/7?x=` // the query differs per load
	for i := 0; i < 3; i++ {
		if code := send(report + strings.Repeat("y", i) + `","blocked-uri":"inline","effective-directive":"script-src-attr"}}`); code != http.StatusNoContent {
			t.Fatalf("status %d, want 204", code)
		}
	}
	if n := strings.Count(buf.String(), "script would be blocked"); n != 1 {
		t.Fatalf("logged %d times, want once:\n%s", n, buf.String())
	}
	if strings.Contains(buf.String(), "x=") {
		t.Errorf("a query string reached the log:\n%s", buf.String())
	}
	if code := send(`not a report`); code != http.StatusNoContent {
		t.Errorf("a bad body answers %d, want 204", code)
	}

	tbl := &cspReportTable{seen: make(map[string]int)}
	for i := 0; i < cspReportMaxKeys+10; i++ {
		tbl.record(cspViolation{Directive: "script-src", Line: i})
	}
	if len(tbl.seen) != cspReportMaxKeys {
		t.Errorf("table holds %d violations, want the cap %d", len(tbl.seen), cspReportMaxKeys)
	}
}

// TestCSRFLetsCSPReportsThrough: browsers send reports without a token. Only
// a POST to the report path is let through; other paths still need one.
func TestCSRFLetsCSPReportsThrough(t *testing.T) {
	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, CSPReportPath, http.StatusNoContent},
		{http.MethodPost, CSPReportPath + "x", http.StatusForbidden},
		{http.MethodPut, CSPReportPath, http.StatusForbidden},
	}
	for _, tc := range cases {
		e := echo.New()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.AddCookie(&http.Cookie{Name: csrfCookieBaseName, Value: "tok"})
		rec := httptest.NewRecorder()
		h := CSRF()(func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
		err := h(e.NewContext(req, rec))
		code := rec.Code
		var appErr *apperror.AppError
		if asAppError(err, &appErr) {
			code = appErr.Code
		} else if err != nil {
			t.Fatal(err)
		}
		if code != tc.want {
			t.Errorf("%s %s: status %d, want %d", tc.method, tc.path, code, tc.want)
		}
	}
}
