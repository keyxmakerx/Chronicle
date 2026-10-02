package restore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

type stubRestoreSvc struct {
	Service
	err     error
	running bool
}

func (s stubRestoreSvc) IsRunning() bool { return s.running }

func (s stubRestoreSvc) RunRestore(context.Context, string) (*RunResult, error) {
	return &RunResult{}, s.err
}

type recordingActivity struct{ actions []string }

func (r *recordingActivity) RecordAdminChange(_ echo.Context, action, _, _, _ string) {
	r.actions = append(r.actions, action)
}

// TestRunRecordsActivity pins that the start is logged before the restore
// runs (the database, and the log with it, is replaced by a successful run)
// and the finish is logged after it, into the restored database.
func TestRunRecordsActivity(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		running bool
		want    []string
	}{
		{"success logs start then finish", nil, false, []string{"restore.started", "restore.run"}},
		{"failure logs only the start", errors.New("boom"), false, []string{"restore.started"}},
		{"busy logs nothing", ErrAlreadyRunning, true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingActivity{}
			h := NewHandler(stubRestoreSvc{err: tc.err, running: tc.running})
			h.SetActivityRecorder(rec)

			form := url.Values{"manifest": {"m.txt"}, "confirm": {confirmationToken}}
			req := httptest.NewRequest(http.MethodPost, "/admin/restore/run", strings.NewReader(form.Encode()))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			c := echo.New().NewContext(req, httptest.NewRecorder())

			_ = h.Run(c)

			if strings.Join(rec.actions, ",") != strings.Join(tc.want, ",") {
				t.Errorf("recorded %v, want %v", rec.actions, tc.want)
			}
		})
	}
}
