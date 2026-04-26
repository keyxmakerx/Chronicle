package packages

import (
	"log/slog"
	"net/http"
	"strconv"

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

	return middleware.HTMXRedirect(c, "/admin/packages")
}

// RemovePackage deletes a package (DELETE /admin/packages/:id).
func (h *Handler) RemovePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.RemovePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return middleware.HTMXRedirect(c, "/admin/packages")
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

	return middleware.HTMXRedirect(c, "/admin/packages")
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

	return middleware.HTMXRedirect(c, "/admin/packages")
}

// ClearPinnedVersion unpins a package (DELETE /admin/packages/:id/pin).
func (h *Handler) ClearPinnedVersion(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.ClearPinnedVersion(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return middleware.HTMXRedirect(c, "/admin/packages")
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

	return middleware.HTMXRedirect(c, "/admin/packages")
}

// CheckForUpdates triggers an update check (POST /admin/packages/:id/check).
func (h *Handler) CheckForUpdates(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if _, err := h.service.CheckForUpdates(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return middleware.HTMXRedirect(c, "/admin/packages")
}

// RepackFoundryZip re-bakes the cached Foundry-module zip's manifest URLs to
// the current BASE_URL (POST /admin/packages/:id/repack). Operator-facing
// escape hatch when BASE_URL changes after a Foundry module was installed —
// without this, the served zip would keep handing out stale URLs until the
// next upstream release triggered a reinstall.
func (h *Handler) RepackFoundryZip(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.RepackFoundryZip(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return middleware.HTMXRedirect(c, "/admin/packages")
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

	return middleware.HTMXRedirect(c, "/admin/packages")
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

	return middleware.HTMXRedirect(c, "/admin/packages")
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

	return middleware.HTMXRedirect(c, "/admin/packages")
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

	return middleware.HTMXRedirect(c, "/admin/packages")
}

// ArchivePackage hides a package (POST /admin/packages/:id/archive).
func (h *Handler) ArchivePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.ArchivePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return middleware.HTMXRedirect(c, "/admin/packages")
}

// UnarchivePackage restores an archived package (DELETE /admin/packages/:id/archive).
func (h *Handler) UnarchivePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.UnarchivePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return middleware.HTMXRedirect(c, "/admin/packages")
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

// SaveSecuritySettings persists settings (POST /admin/packages/settings).
func (h *Handler) SaveSecuritySettings(c echo.Context) error {
	ctx := c.Request().Context()

	maxFileSizeMB, _ := strconv.ParseInt(c.FormValue("max_file_size_mb"), 10, 64)
	if maxFileSizeMB < 1 {
		maxFileSizeMB = 50
	}

	ownerPolicy := c.FormValue("owner_upload_policy")
	if ownerPolicy != OwnerUploadAutoApprove && ownerPolicy != OwnerUploadRequireApproval && ownerPolicy != OwnerUploadDisabled {
		ownerPolicy = OwnerUploadAutoApprove
	}

	settings := &PackageSecuritySettings{
		RepoPolicy:        c.FormValue("repo_policy"),
		RequireApproval:   c.FormValue("require_approval") == "true",
		MaxFileSize:       maxFileSizeMB * 1024 * 1024,
		ValidateManifest:  c.FormValue("validate_manifest") == "true",
		ScanContent:       c.FormValue("scan_content") == "true",
		OwnerUploadPolicy: ownerPolicy,
	}

	if err := h.service.SaveSecuritySettings(ctx, settings); err != nil {
		slog.Error("failed to save security settings", slog.Any("error", err))
		return err
	}

	slog.Info("security settings updated")

	return c.Redirect(http.StatusSeeOther, "/admin/packages/settings")
}
