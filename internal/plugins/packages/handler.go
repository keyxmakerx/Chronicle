package packages

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// Handler handles admin HTTP requests for the package manager plugin.
type Handler struct {
	service PackageService
}

// NewHandler creates a new package manager handler.
func NewHandler(service PackageService) *Handler {
	return &Handler{service: service}
}

// ListPackages renders the package management page (GET /admin/packages).
func (h *Handler) ListPackages(c echo.Context) error {
	ctx := c.Request().Context()

	pkgs, err := h.service.ListPackages(ctx)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, PackagesPage(pkgs, csrfToken))
}

// AddPackage registers a new GitHub repository (POST /admin/packages).
func (h *Handler) AddPackage(c echo.Context) error {
	ctx := c.Request().Context()

	var input AddPackageInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	pkg, err := h.service.AddPackage(ctx, input)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	slog.Info("package added via admin",
		slog.String("slug", pkg.Slug),
		slog.String("repo", pkg.RepoURL),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// RemovePackage deletes a package (DELETE /admin/packages/:id).
func (h *Handler) RemovePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.RemovePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// ListVersions returns available versions for a package (GET /admin/packages/:id/versions).
// Returns an HTMX fragment for the version picker.
func (h *Handler) ListVersions(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	pkg, err := h.service.GetPackage(ctx, id)
	if err != nil {
		return err
	}
	if pkg == nil {
		return echo.NewHTTPError(http.StatusNotFound, "package not found")
	}

	versions, err := h.service.ListVersions(ctx, id)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, VersionList(pkg, versions, csrfToken))
}

// InstallVersion installs a specific version (PUT /admin/packages/:id/version).
func (h *Handler) InstallVersion(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	var input InstallVersionInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	if err := h.service.InstallVersion(ctx, id, input.Version); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// SetPinnedVersion pins a package to a version (PUT /admin/packages/:id/pin).
func (h *Handler) SetPinnedVersion(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	var input PinVersionInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	if err := h.service.SetPinnedVersion(ctx, id, input.Version); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// ClearPinnedVersion unpins a package (DELETE /admin/packages/:id/pin).
func (h *Handler) ClearPinnedVersion(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.ClearPinnedVersion(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// SetAutoUpdate changes the auto-update policy (PUT /admin/packages/:id/auto-update).
func (h *Handler) SetAutoUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	var input UpdatePolicyInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	policy := UpdatePolicy(input.Policy)
	if err := h.service.SetAutoUpdate(ctx, id, policy); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// CheckForUpdates triggers an update check (POST /admin/packages/:id/check).
func (h *Handler) CheckForUpdates(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if _, err := h.service.CheckForUpdates(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// GetUsage shows which campaigns use a package (GET /admin/packages/:id/usage).
func (h *Handler) GetUsage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	pkg, err := h.service.GetPackage(ctx, id)
	if err != nil {
		return err
	}
	if pkg == nil {
		return echo.NewHTTPError(http.StatusNotFound, "package not found")
	}

	usage, err := h.service.GetUsage(ctx, id)
	if err != nil {
		return err
	}

	return middleware.Render(c, http.StatusOK, UsageTable(pkg, usage))
}

// --- Admin Approval Workflow ---

// ListPendingSubmissions shows packages awaiting approval (GET /admin/packages/pending).
func (h *Handler) ListPendingSubmissions(c echo.Context) error {
	ctx := c.Request().Context()

	pkgs, err := h.service.ListPendingSubmissions(ctx)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, PendingSubmissionsList(pkgs, csrfToken))
}

// ReviewPackage approves or rejects a submission (POST /admin/packages/:id/review).
func (h *Handler) ReviewPackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	session := auth.GetSession(c)
	if session == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	var input ReviewPackageInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	if err := h.service.ReviewPackage(ctx, id, session.UserID, input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// UpdateRepoURL changes a package's repository URL (PUT /admin/packages/:id/repo).
func (h *Handler) UpdateRepoURL(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	var input UpdateRepoURLInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	if err := h.service.UpdateRepoURL(ctx, id, input.RepoURL); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// DeprecatePackage marks a package as EOL (POST /admin/packages/:id/deprecate).
func (h *Handler) DeprecatePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	var input DeprecateInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	if err := h.service.DeprecatePackage(ctx, id, input.Message); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// UndeprecatePackage clears deprecation (DELETE /admin/packages/:id/deprecate).
func (h *Handler) UndeprecatePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	pkg, err := h.service.GetPackage(ctx, id)
	if err != nil || pkg == nil {
		return echo.NewHTTPError(http.StatusNotFound, "package not found")
	}

	// Clear deprecation by restoring approved status.
	if err := h.service.UnarchivePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// ArchivePackage hides a package (POST /admin/packages/:id/archive).
func (h *Handler) ArchivePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.ArchivePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// UnarchivePackage restores an archived package (DELETE /admin/packages/:id/archive).
func (h *Handler) UnarchivePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.UnarchivePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/packages")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/packages")
}

// --- Package Security Settings ---

// GetSecuritySettings renders the settings page (GET /admin/packages/settings).
func (h *Handler) GetSecuritySettings(c echo.Context) error {
	ctx := c.Request().Context()

	secSettings, err := h.service.GetSecuritySettings(ctx)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, SecuritySettingsPage(*secSettings, csrfToken))
}
