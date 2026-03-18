package smtp

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// Handler handles HTTP requests for SMTP settings management.
// Admin-only -- all routes require site admin middleware.
type Handler struct {
	service SMTPService
}

// NewHandler creates a new SMTP handler.
func NewHandler(service SMTPService) *Handler {
	return &Handler{service: service}
}

// Settings renders the SMTP settings page (GET /admin/smtp).
func (h *Handler) Settings(c echo.Context) error {
	settings, err := h.service.GetSettings(c.Request().Context())
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, SMTPSettingsPage(settings, csrfToken, ""))
}

// UpdateSettings saves SMTP settings (PUT /admin/smtp).
func (h *Handler) UpdateSettings(c echo.Context) error {
	var req UpdateSMTPRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	if err := h.service.UpdateSettings(c.Request().Context(), req); err != nil {
		settings, _ := h.service.GetSettings(c.Request().Context())
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "failed to save settings")
		return middleware.Render(c, http.StatusOK, SMTPSettingsPage(settings, csrfToken, errMsg))
	}

	// Re-render with success feedback.
	settings, _ := h.service.GetSettings(c.Request().Context())
	csrfToken := middleware.GetCSRFToken(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, SMTPFormComponent(settings, csrfToken, "", "Settings saved successfully"))
	}
	return middleware.Render(c, http.StatusOK, SMTPSettingsPage(settings, csrfToken, ""))
}

// TestConnection tests SMTP connectivity (POST /admin/smtp/test).
func (h *Handler) TestConnection(c echo.Context) error {
	if err := h.service.TestConnection(c.Request().Context()); err != nil {
		settings, _ := h.service.GetSettings(c.Request().Context())
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "connection failed")
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, SMTPFormComponent(settings, csrfToken, errMsg, ""))
		}
		return middleware.Render(c, http.StatusOK, SMTPSettingsPage(settings, csrfToken, errMsg))
	}

	settings, _ := h.service.GetSettings(c.Request().Context())
	csrfToken := middleware.GetCSRFToken(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, SMTPFormComponent(settings, csrfToken, "", "Connection successful!"))
	}
	return middleware.Render(c, http.StatusOK, SMTPSettingsPage(settings, csrfToken, ""))
}

// SendTestEmail sends a test email to a specified address (POST /admin/smtp/send-test).
func (h *Handler) SendTestEmail(c echo.Context) error {
	to := c.FormValue("test_email")
	if to == "" {
		to = c.QueryParam("to")
	}

	if err := h.service.SendTestEmail(c.Request().Context(), to); err != nil {
		settings, _ := h.service.GetSettings(c.Request().Context())
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "send failed")
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, SMTPFormComponent(settings, csrfToken, errMsg, ""))
		}
		return middleware.Render(c, http.StatusOK, SMTPSettingsPage(settings, csrfToken, errMsg))
	}

	settings, _ := h.service.GetSettings(c.Request().Context())
	csrfToken := middleware.GetCSRFToken(c)

	successMsg := "Test email sent to " + to + "!"
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, SMTPFormComponent(settings, csrfToken, "", successMsg))
	}
	return middleware.Render(c, http.StatusOK, SMTPSettingsPage(settings, csrfToken, ""))
}
