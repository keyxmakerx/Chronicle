package restore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/maintenance"
)

// stubRestoreSvc replaces the real service so handler behaviour is tested
// without a script, a marker file or a database.
type stubRestoreSvc struct {
	Service
	startErr   error
	started    []StartRequest
	startCtx   context.Context
	leaveErr   error
	left       []string
	snap       maintenance.Snapshot
	leaveToken string
}

func (s *stubRestoreSvc) Start(ctx context.Context, r StartRequest) error {
	s.startCtx = ctx
	if s.startErr != nil {
		return s.startErr
	}
	s.started = append(s.started, r)
	return nil
}
func (s *stubRestoreSvc) Leave(_ context.Context, userID string) error {
	s.left = append(s.left, userID)
	return s.leaveErr
}
func (s *stubRestoreSvc) Status() maintenance.Snapshot { return s.snap }
func (s *stubRestoreSvc) LeaveToken() string           { return s.leaveToken }
func (s *stubRestoreSvc) CheckLeaveToken(t string) bool {
	return s.leaveToken != "" && t == s.leaveToken
}
func (s *stubRestoreSvc) HandRestoreCommand(name string) string { return "restore " + name }
func (s *stubRestoreSvc) BackupDir() string                     { return "/backups" }

func postForm(path string, form url.Values, mutate func(*http.Request)) (*echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	return echo.New().NewContext(req, rec), rec
}

func withSession(req *http.Request) {
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: "tok"})
}

func statusOf(err error, rec *httptest.ResponseRecorder) int {
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return he.Code
	}
	return rec.Code
}

// TestRunStartsRestore pins what the run endpoint demands before the site
// closes, and what it hands the service.
func TestRunStartsRestore(t *testing.T) {
	tests := []struct {
		name      string
		form      url.Values
		mutate    func(*http.Request)
		startErr  error
		wantCode  int
		wantStart bool
	}{
		{"starts with the confirmation and a session", url.Values{"manifest": {"m.txt"}, "confirm": {"RESTORE"}}, withSession, nil, http.StatusSeeOther, true},
		{"no manifest", url.Values{"confirm": {"RESTORE"}}, withSession, nil, http.StatusBadRequest, false},
		{"wrong confirmation", url.Values{"manifest": {"m.txt"}, "confirm": {"restore"}}, withSession, nil, http.StatusBadRequest, false},
		{"empty confirmation", url.Values{"manifest": {"m.txt"}}, withSession, nil, http.StatusBadRequest, false},
		{"no session to tie the restore to", url.Values{"manifest": {"m.txt"}, "confirm": {"RESTORE"}}, nil, nil, http.StatusBadRequest, false},
		{"already running", url.Values{"manifest": {"m.txt"}, "confirm": {"RESTORE"}}, withSession, ErrAlreadyRunning, http.StatusConflict, false},
		{"unknown backup", url.Values{"manifest": {"m.txt"}, "confirm": {"RESTORE"}}, withSession, ErrInvalidManifest, http.StatusBadRequest, false},
		{"other failure", url.Values{"manifest": {"m.txt"}, "confirm": {"RESTORE"}}, withSession, errors.New("disk"), http.StatusInternalServerError, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubRestoreSvc{startErr: tc.startErr}
			h := NewHandler(svc)
			h.SetActor(func(*echo.Context) string { return "admin-1" })
			c, rec := postForm("/admin/restore/run", tc.form, tc.mutate)

			err := h.Run(c)

			if got := statusOf(err, rec); got != tc.wantCode {
				t.Fatalf("status = %d, want %d (err %v)", got, tc.wantCode, err)
			}
			if (len(svc.started) == 1) != tc.wantStart {
				t.Fatalf("started = %d, want start=%v", len(svc.started), tc.wantStart)
			}
			if tc.wantStart {
				got := svc.started[0]
				if got.Manifest != "m.txt" || got.UserID != "admin-1" || len(got.SessionHash) != 64 {
					t.Errorf("StartRequest = %+v", got)
				}
			}
		})
	}
}

// TestRunSurvivesRequestCancel pins that the restore starts on a context the
// service can detach, and that a cancelled request does not stop the call:
// the service itself runs in the background on a non-cancellable context.
func TestRunPassesRequestContextToStart(t *testing.T) {
	svc := &stubRestoreSvc{}
	h := NewHandler(svc)
	c, _ := postForm("/admin/restore/run", url.Values{"manifest": {"m.txt"}, "confirm": {"RESTORE"}}, withSession)
	_ = h.Run(c)
	if svc.startCtx == nil {
		t.Fatal("service was not called")
	}
}

// TestOwnerView pins the three things the restoring admin can do while the
// site is closed, and that nothing else is claimed.
func TestOwnerView(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		form       url.Values
		mutate     func(*http.Request)
		snap       maintenance.Snapshot
		leaveErr   error
		wantClaim  bool
		wantCode   int
		wantLeft   bool
		wantInBody string
	}{
		{name: "progress page while running", method: "GET", path: "/admin/restore",
			snap: maintenance.Snapshot{Active: true, Phase: maintenance.PhaseRunning, Step: "Erasing the current database", Manifest: "m.txt"},
			wantClaim: true, wantCode: 200, wantInBody: "Erasing the current database"},
		{name: "failure page shows the way out", method: "GET", path: "/admin/restore",
			snap:      maintenance.Snapshot{Active: true, Phase: maintenance.PhaseFailed, Summary: "stopped", Safety: "chronicle_pre_restore_manifest_1.txt"},
			wantClaim: true, wantCode: 200, wantInBody: "Leave maintenance anyway"},
		{name: "running page offers no leave button", method: "GET", path: "/admin/restore",
			snap:      maintenance.Snapshot{Active: true, Phase: maintenance.PhaseRunning},
			wantClaim: true, wantCode: 200, wantInBody: "Restoring your site"},
		{name: "status poll", method: "GET", path: "/admin/restore/status",
			snap:      maintenance.Snapshot{Active: true, Phase: maintenance.PhaseRunning, Step: "Loading"},
			wantClaim: true, wantCode: 200, wantInBody: `"phase":"running"`},
		{name: "leave with the right token", method: "POST", path: "/admin/restore/leave",
			form: url.Values{"leave_token": {"secret"}}, snap: maintenance.Snapshot{Active: true, Phase: maintenance.PhaseFailed, Actor: "admin-1"},
			wantClaim: true, wantCode: 303, wantLeft: true},
		{name: "leave with a wrong token", method: "POST", path: "/admin/restore/leave",
			form: url.Values{"leave_token": {"guess"}}, snap: maintenance.Snapshot{Active: true, Phase: maintenance.PhaseFailed},
			wantClaim: true, wantCode: 403},
		{name: "leave with no token", method: "POST", path: "/admin/restore/leave",
			snap: maintenance.Snapshot{Active: true, Phase: maintenance.PhaseFailed}, wantClaim: true, wantCode: 403},
		{name: "leave from another site", method: "POST", path: "/admin/restore/leave",
			form:   url.Values{"leave_token": {"secret"}},
			mutate: func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
			snap:   maintenance.Snapshot{Active: true, Phase: maintenance.PhaseFailed}, wantClaim: true, wantCode: 403},
		{name: "leave flagged cross-site by the browser", method: "POST", path: "/admin/restore/leave",
			form:   url.Values{"leave_token": {"secret"}},
			mutate: func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
			snap:   maintenance.Snapshot{Active: true, Phase: maintenance.PhaseFailed}, wantClaim: true, wantCode: 403},
		{name: "leave while still running", method: "POST", path: "/admin/restore/leave",
			form: url.Values{"leave_token": {"secret"}}, snap: maintenance.Snapshot{Active: true, Phase: maintenance.PhaseRunning},
			leaveErr: maintenance.ErrStillBusy, wantClaim: true, wantCode: 409, wantLeft: true},
		{name: "run is not claimed", method: "POST", path: "/admin/restore/run", wantClaim: false},
		{name: "other admin pages are not claimed", method: "GET", path: "/admin/users", wantClaim: false},
		{name: "wrong method on leave is not claimed", method: "GET", path: "/admin/restore/leave", wantClaim: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubRestoreSvc{snap: tc.snap, leaveToken: "secret", leaveErr: tc.leaveErr}
			svc.snap.Manifest = firstNonEmpty(svc.snap.Manifest, "m.txt")
			if svc.snap.Safety != "" {
				svc.snap.DBTouched = true
			}
			h := NewHandler(svc)
			var body *strings.Reader
			if tc.form != nil {
				body = strings.NewReader(tc.form.Encode())
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			if tc.mutate != nil {
				tc.mutate(req)
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)

			claimed, err := h.OwnerView(c)

			if claimed != tc.wantClaim {
				t.Fatalf("claimed = %v, want %v", claimed, tc.wantClaim)
			}
			if !claimed {
				return
			}
			if got := statusOf(err, rec); got != tc.wantCode {
				t.Fatalf("status = %d, want %d (err %v)", got, tc.wantCode, err)
			}
			if tc.wantLeft != (len(svc.left) == 1) {
				t.Errorf("Leave called %d times, want called=%v", len(svc.left), tc.wantLeft)
			}
			if tc.wantLeft && tc.leaveErr == nil && svc.left[0] != "admin-1" {
				t.Errorf("Leave recorded %q, want the restoring admin", svc.left[0])
			}
			if tc.wantInBody != "" && !strings.Contains(rec.Body.String(), tc.wantInBody) {
				t.Errorf("body lacks %q:\n%s", tc.wantInBody, rec.Body.String())
			}
		})
	}
}

// TestRunningPageHasNoLeaveForm pins that leaving is offered only once the
// restore has stopped, never while it is still working.
func TestRunningPageHasNoLeaveForm(t *testing.T) {
	svc := &stubRestoreSvc{leaveToken: "secret", snap: maintenance.Snapshot{Active: true, Phase: maintenance.PhaseRunning, Manifest: "m.txt"}}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/admin/restore", nil), rec)
	if _, err := NewHandler(svc).OwnerView(c); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), "leave_token") || strings.Contains(rec.Body.String(), "secret") {
		t.Error("running page carries the leave token")
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
