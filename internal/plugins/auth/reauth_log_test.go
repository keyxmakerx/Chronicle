package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// reauthStubService answers ConfirmReauth with err; nothing else is called.
type reauthStubService struct {
	AuthService
	err error
}

func (s *reauthStubService) ConfirmReauth(context.Context, string, string) error { return s.err }

type recordingSecurityLog struct{ events []string }

func (r *recordingSecurityLog) LogEvent(_ context.Context, eventType, _, _, _, _ string, _ map[string]any) error {
	r.events = append(r.events, eventType)
	return nil
}

// Every password re-confirmation is logged, the wrong ones included, so the
// security dashboard shows someone guessing behind a signed-in session.
func TestReauthConfirm_LogsOutcome(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantEvent string
		wantCode  int
	}{
		{"right password", nil, "reauth_confirmed", http.StatusOK},
		{"wrong password", apperror.NewUnauthorized("incorrect password"), "reauth_failed", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := &recordingSecurityLog{}
			h := NewHandler(&reauthStubService{err: tt.err}, 0)
			h.SetSecurityLogger(logs)
			e := echo.New()
			form := url.Values{"password": {"pw"}}
			req := httptest.NewRequest(http.MethodPost, "/account/reauth", strings.NewReader(form.Encode()))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(contextKeySession, &Session{UserID: "u-1"})

			err := h.ReauthConfirm(c)
			code := rec.Code
			if err != nil {
				if ae, ok := err.(*apperror.AppError); ok {
					code = ae.Code
				}
			}
			if code != tt.wantCode {
				t.Fatalf("code %d, want %d (err %v)", code, tt.wantCode, err)
			}
			if len(logs.events) != 1 || logs.events[0] != tt.wantEvent {
				t.Fatalf("events %v, want [%s]", logs.events, tt.wantEvent)
			}
		})
	}
}
