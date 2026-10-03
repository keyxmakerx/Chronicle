package packages

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// Handler handles admin HTTP requests for the package manager plugin.
type Handler struct {
	activity ActivityRecorder
	service  PackageService
}

// NewHandler creates a new package manager handler.
func NewHandler(service PackageService) *Handler {
	return &Handler{service: service}
}

// ListPackages renders the package management page (GET /admin/packages).
// The tab, filter, search and open package are query parameters, so every view
// is a link and survives a reload.
func (h *Handler) ListPackages(c echo.Context) error {
	q := parsePackagesQuery(
		c.QueryParam("tab"), c.QueryParam("f"), c.QueryParam("q"),
		c.QueryParam("pkg"), c.QueryParam("ptab"),
	)

	data, err := buildPackagesPage(c.Request().Context(), h.service, q, middleware.GetCSRFToken(c), time.Now())
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, PackagesPage(*data))
}

// backToPage answers a write by sending the admin back to the page they were
// on, so acting from the open panel does not close it.
func (h *Handler) backToPage(c echo.Context) error {
	return middleware.HTMXRedirect(c, packagesReturnURL(c.Request().Header.Get("HX-Current-URL")))
}

// wantsFullPage reports whether the request is a browser navigation, as
// opposed to an HTMX fragment fetch or an API call, which keep their old
// responses.
func wantsFullPage(c echo.Context) bool {
	return !middleware.IsHTMX(c) && !middleware.IsAPIRequest(c)
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

	h.recordActivity(c, "package.added", "package", pkg.ID, pkg.Name)

	return h.backToPage(c)
}

// PrunePreview renders the stale-version cleanup card as a lazy HTMX
// fragment (GET /admin/packages/prune). Dry-run only — nothing deleted.
// Errors (incl. the fail-closed unwired-provider case) degrade to an
// empty fragment rather than breaking the packages page.
func (h *Handler) PrunePreview(c echo.Context) error {
	res, err := h.service.PruneStaleVersions(c.Request().Context(), 1, true)
	if err != nil || res == nil {
		return c.NoContent(http.StatusOK)
	}
	return middleware.Render(c, http.StatusOK, PruneCard(res, middleware.GetCSRFToken(c)))
}

// PruneExecute deletes stale version folders (DELETE /admin/packages/prune).
// The service re-derives the deletion set server-side — the confirm click
// authorizes the OPERATION, never a client-supplied list.
func (h *Handler) PruneExecute(c echo.Context) error {
	res, err := h.service.PruneStaleVersions(c.Request().Context(), 1, false)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	slog.Info("stale package versions pruned via admin",
		slog.Int("removed", len(res.Removed)),
		slog.Int64("bytes_freed", res.BytesFreed),
	)
	h.recordActivity(c, "package.pruned", "package", "", "")
	return h.backToPage(c)
}

// RemovePackage deletes a package (DELETE /admin/packages/:id).
func (h *Handler) RemovePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	// Read the name first: the row is gone after the removal.
	label := h.packageLabel(ctx, id)

	if err := h.service.RemovePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	h.recordActivity(c, "package.removed", "package", id, label)

	return h.backToPage(c)
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

	h.recordActivity(c, "package.version_installed", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
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

	h.recordActivity(c, "package.version_pinned", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
}

// ClearPinnedVersion unpins a package (DELETE /admin/packages/:id/pin).
func (h *Handler) ClearPinnedVersion(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.ClearPinnedVersion(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	h.recordActivity(c, "package.version_pinned", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
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

	h.recordActivity(c, "package.auto_update", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
}

// CheckForUpdates triggers an update check (POST /admin/packages/:id/check).
func (h *Handler) CheckForUpdates(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if _, err := h.service.CheckForUpdates(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return h.backToPage(c)
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
	// The page lives in the Review tab now; the old address still works.
	if wantsFullPage(c) {
		return c.Redirect(http.StatusSeeOther, tabHref(PackagesTabReview))
	}
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

	h.recordActivity(c, "package.reviewed", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
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

	h.recordActivity(c, "package.repo_changed", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
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

	h.recordActivity(c, "package.deprecated", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
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

	h.recordActivity(c, "package.deprecated", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
}

// ArchivePackage hides a package (POST /admin/packages/:id/archive).
func (h *Handler) ArchivePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.ArchivePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	h.recordActivity(c, "package.archived", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
}

// UnarchivePackage restores an archived package (DELETE /admin/packages/:id/archive).
func (h *Handler) UnarchivePackage(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	if err := h.service.UnarchivePackage(ctx, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	h.recordActivity(c, "package.archived", "package", id, h.packageLabel(ctx, id))

	return h.backToPage(c)
}

// --- Package Security Settings ---

// GetSecuritySettings renders the settings page (GET /admin/packages/settings).
func (h *Handler) GetSecuritySettings(c echo.Context) error {
	// The page lives in the Settings tab now; the old address still works.
	if wantsFullPage(c) {
		return c.Redirect(http.StatusSeeOther, tabHref(PackagesTabSettings))
	}
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

// packageLabel returns a package's name for the change log, or "" when it
// can't be read; the id is still recorded.
func (h *Handler) packageLabel(ctx context.Context, id string) string {
	p, err := h.service.GetPackage(ctx, id)
	if err != nil || p == nil {
		return ""
	}
	return p.Name
}
