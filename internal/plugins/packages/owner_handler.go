package packages

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// OwnerHandler handles HTTP requests for the owner-facing package submission UI.
// These routes are accessible to any authenticated campaign owner, not just admins.
type OwnerHandler struct {
	service PackageService
}

// NewOwnerHandler creates a new owner-facing handler.
func NewOwnerHandler(service PackageService) *OwnerHandler {
	return &OwnerHandler{service: service}
}

// BrowseSystems renders the systems browse page (GET /systems/browse).
func (h *OwnerHandler) BrowseSystems(c echo.Context) error {
	ctx := c.Request().Context()

	pkgs, err := h.service.ListPackages(ctx)
	if err != nil {
		return err
	}

	// Filter to approved + deprecated systems only (no pending/rejected/archived).
	var visible []Package
	for _, p := range pkgs {
		if p.Status == StatusApproved || p.Status == StatusDeprecated {
			visible = append(visible, p)
		}
	}

	secSettings, _ := h.service.GetSecuritySettings(ctx)

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, SystemsBrowsePage(visible, *secSettings, csrfToken))
}

// SubmitSystemForm renders the submission form (GET /systems/submit).
func (h *OwnerHandler) SubmitSystemForm(c echo.Context) error {
	ctx := c.Request().Context()
	secSettings, _ := h.service.GetSecuritySettings(ctx)
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, SystemSubmitPage(*secSettings, csrfToken))
}

// HandleSubmitSystem processes a system submission (POST /systems/submit).
func (h *OwnerHandler) HandleSubmitSystem(c echo.Context) error {
	ctx := c.Request().Context()

	session := auth.GetSession(c)
	if session == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	var input SubmitPackageInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	_, err := h.service.SubmitPackage(ctx, session.UserID, input)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return middleware.HTMXRedirect(c, "/systems/my-submissions")
}

// MySubmissions renders the user's submitted packages (GET /systems/my-submissions).
func (h *OwnerHandler) MySubmissions(c echo.Context) error {
	ctx := c.Request().Context()

	session := auth.GetSession(c)
	if session == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	pkgs, err := h.service.ListMySubmissions(ctx, session.UserID)
	if err != nil {
		return err
	}

	return middleware.Render(c, http.StatusOK, MySubmissionsPage(pkgs))
}
