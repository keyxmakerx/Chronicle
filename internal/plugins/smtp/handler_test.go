package smtp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// failingSMTPService fails saves so the handler's error rendering is exercised.
type failingSMTPService struct {
	SMTPService
	saveErr error
}

func (f *failingSMTPService) GetSettings(context.Context) (*SMTPSettings, error) {
	return &SMTPSettings{Host: "mail.example.com", Port: 587}, nil
}

func (f *failingSMTPService) UpdateSettings(context.Context, UpdateSMTPRequest) error {
	return f.saveErr
}

func TestUpdateSettings_ErrorRendering(t *testing.T) {
	tests := []struct {
		name         string
		htmx         bool
		wantFullPage bool
	}{
		{"htmx gets the form fragment", true, false},
		{"plain request gets the full page", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/admin/smtp", strings.NewReader("host=x"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.htmx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			h := NewHandler(&failingSMTPService{saveErr: apperror.NewBadRequest("port must be between 1 and 65535")})
			if err := h.UpdateSettings(c); err != nil {
				t.Fatalf("UpdateSettings returned error: %v", err)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "port must be between 1 and 65535") {
				t.Errorf("error message missing from response: %s", body)
			}
			// Only the full page wraps the form in its swap container.
			if got := strings.Contains(body, `id="smtp-form-container"`); got != tc.wantFullPage {
				t.Errorf("full page markup present = %v, want %v", got, tc.wantFullPage)
			}
		})
	}
}
