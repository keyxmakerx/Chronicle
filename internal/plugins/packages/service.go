package packages

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SettingsReader provides read access to site settings without importing
// the full settings package. Matches settings.SettingsRepository.Get.
type SettingsReader interface {
	Get(ctx context.Context, key string) (string, error)
}

// SettingsWriter provides write access to site settings.
type SettingsWriter interface {
	Set(ctx context.Context, key, value string) error
}

// defaultMaxFileSize is 50 MB in bytes — used when the setting is not configured.
const defaultMaxFileSize int64 = 50 * 1024 * 1024

// PackageService handles business logic for package management.
// It coordinates between the repository (database), GitHub API client,
// and the local filesystem to manage installed package versions.
type PackageService interface {
	// Package CRUD (admin).
	ListPackages(ctx context.Context) ([]Package, error)
	GetPackage(ctx context.Context, id string) (*Package, error)
	AddPackage(ctx context.Context, input AddPackageInput) (*Package, error)
	RemovePackage(ctx context.Context, id string) error

	// Version management.
	CheckForUpdates(ctx context.Context, packageID string) ([]PackageVersion, error)
	CheckAllForUpdates(ctx context.Context) error
	ListVersions(ctx context.Context, packageID string) ([]PackageVersion, error)
	InstallVersion(ctx context.Context, packageID, version string) error
	SetPinnedVersion(ctx context.Context, packageID, version string) error
	ClearPinnedVersion(ctx context.Context, packageID string) error
	SetAutoUpdate(ctx context.Context, packageID string, policy UpdatePolicy) error

	// Usage tracking.
	GetUsage(ctx context.Context, packageID string) ([]PackageUsage, error)

	// Auto-update worker.
	RunAutoUpdates(ctx context.Context) error
	StartAutoUpdateWorker(ctx context.Context)

	// FoundryModulePath returns the install path for the active Foundry module,
	// or empty string if none is installed.
	FoundryModulePath() string

	// FoundryModuleZipPath returns the cached ZIP file path for the active
	// Foundry module, or empty string if not available.
	FoundryModuleZipPath() string

	// --- Submission/Approval Workflow ---

	// SubmitPackage lets a campaign owner submit a repo URL for review.
	SubmitPackage(ctx context.Context, userID string, input SubmitPackageInput) (*Package, error)

	// ListMySubmissions returns packages submitted by the given user.
	ListMySubmissions(ctx context.Context, userID string) ([]Package, error)

	// ListPendingSubmissions returns all packages awaiting admin approval.
	ListPendingSubmissions(ctx context.Context) ([]Package, error)

	// CountPendingSubmissions returns the number of packages awaiting approval.
	CountPendingSubmissions(ctx context.Context) (int, error)

	// ReviewPackage approves or rejects a pending package submission.
	ReviewPackage(ctx context.Context, packageID, adminUserID string, input ReviewPackageInput) error

	// --- Lifecycle Management ---

	// DeprecatePackage marks a package as nearing end-of-life.
	DeprecatePackage(ctx context.Context, packageID, msg string) error

	// ArchivePackage hides a package from active listings.
	ArchivePackage(ctx context.Context, packageID string) error

	// UnarchivePackage restores an archived package to approved status.
	UnarchivePackage(ctx context.Context, packageID string) error

	// UpdateRepoURL changes a package's repository URL (admin only).
	UpdateRepoURL(ctx context.Context, packageID, newURL string) error

	// --- Settings ---

	// GetSecuritySettings returns the current package security configuration.
	GetSecuritySettings(ctx context.Context) (*PackageSecuritySettings, error)

	// SaveSecuritySettings persists updated security settings.
	SaveSecuritySettings(ctx context.Context, s *PackageSecuritySettings) error

	// ReconcileOrphanedInstalls detects package directories on disk that have
	// no corresponding DB record (e.g. after a database wipe).
	ReconcileOrphanedInstalls(ctx context.Context) ([]OrphanedInstall, error)

	// InstalledPackagePath returns the on-disk install path for the active
	// (installed + approved) package matching the given type and slug.
	// Returns empty string if no matching package is installed.
	InstalledPackagePath(pkgType PackageType, slug string) string
}

// packageService implements PackageService.
type packageService struct {
	repo           PackageRepository
	github         *GitHubClient
	settings       SettingsReader
	settingsWriter SettingsWriter
	mediaDir        string // Root media directory (e.g., ./media).
	onSystemInstall func() // Called after a system package is installed.
	onServeInvalidate func() // Called after install/remove to invalidate serve cache.
}

// NewPackageService creates a new package service with the given dependencies.
func NewPackageService(repo PackageRepository, github *GitHubClient, mediaDir string) PackageService {
	return &packageService{
		repo:     repo,
		github:   github,
		mediaDir: mediaDir,
	}
}

// SetOnSystemInstall wires a callback invoked after a system package is installed.
// Used to rescan the system registry so newly installed systems appear immediately.
func SetOnSystemInstall(svc PackageService, fn func()) {
	if s, ok := svc.(*packageService); ok {
		s.onSystemInstall = fn
	}
}

// SetOnServeInvalidate wires a callback invoked after a package is installed,
// updated, or removed. Used to invalidate the serve handler's path cache.
func SetOnServeInvalidate(svc PackageService, fn func()) {
	if s, ok := svc.(*packageService); ok {
		s.onServeInvalidate = fn
	}
}

// ConfigureSettings wires settings reader/writer into the package service.
// Called from routes.go after both services are initialized.
func ConfigureSettings(svc PackageService, settings SettingsReader) {
	if s, ok := svc.(*packageService); ok {
		s.settings = settings
		// If the reader also implements Set(), use it as writer too.
		if w, ok := settings.(SettingsWriter); ok {
			s.settingsWriter = w
		}
	}
}

// packagesDir returns the root directory for package storage.
func (s *packageService) packagesDir() string {
	return filepath.Join(s.mediaDir, "packages")
}

// downloadsDir returns the directory for cached ZIP downloads.
func (s *packageService) downloadsDir() string {
	return filepath.Join(s.packagesDir(), "downloads")
}

// installDir returns the extraction directory for a package version.
func (s *packageService) installDir(pkgType PackageType, slug, version string) string {
	switch pkgType {
	case PackageTypeFoundryModule:
		return filepath.Join(s.packagesDir(), "foundry-module", version)
	default:
		return filepath.Join(s.packagesDir(), "systems", slug, version)
	}
}

// --- Package CRUD ---

// ListPackages returns all registered packages.
func (s *packageService) ListPackages(ctx context.Context) ([]Package, error) {
	return s.repo.ListPackages(ctx)
}

// GetPackage returns a single package by ID.
func (s *packageService) GetPackage(ctx context.Context, id string) (*Package, error) {
	return s.repo.GetPackage(ctx, id)
}

// AddPackage registers a new GitHub repository as a package (admin action).
// Admin-created packages are auto-approved and immediately fetched.
func (s *packageService) AddPackage(ctx context.Context, input AddPackageInput) (*Package, error) {
	repoURL := strings.TrimSpace(input.RepoURL)
	if repoURL == "" {
		return nil, fmt.Errorf("repository URL is required")
	}

	// Validate URL against security policy.
	settings, _ := s.GetSecuritySettings(ctx)
	if err := ValidateRepoURL(repoURL, settings.RepoPolicy); err != nil {
		return nil, err
	}

	// Check for duplicates.
	existing, err := s.repo.FindByRepoURL(ctx, repoURL)
	if err != nil {
		return nil, fmt.Errorf("checking for duplicate: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("package already registered: %s", existing.Name)
	}

	pkgType := PackageType(input.Type)
	if pkgType == "" {
		pkgType = PackageTypeSystem
	}

	owner, repo, _ := parseRepo(repoURL)
	slug := repo
	name := input.Name
	if name == "" {
		name = repo
	}

	now := time.Now()
	pkg := &Package{
		ID:          generateUUID(),
		Type:        pkgType,
		Slug:        slug,
		Name:        name,
		RepoURL:     repoURL,
		Description: fmt.Sprintf("Package from %s/%s", owner, repo),
		// Auto-update is opt-in. The admin can switch this to nightly /
		// weekly / on-release in the package settings UI after install.
		// Defaulting to off prevents the server from making unattended
		// outbound fetches on a schedule.
		AutoUpdate:    UpdateOff,
		Status:        StatusApproved, // Admin-created = auto-approved.
		LastCheckedAt: &now,
	}

	if err := s.repo.CreatePackage(ctx, pkg); err != nil {
		return nil, fmt.Errorf("creating package record: %w", err)
	}

	// Fetch releases from GitHub and import versions.
	newVersions, err := s.fetchAndImportVersions(ctx, pkg)
	if err != nil {
		slog.Warn("failed to fetch initial releases",
			slog.String("package", pkg.Slug),
			slog.Any("error", err),
		)
		return pkg, nil
	}

	// Auto-install the latest stable version if available.
	if latest := latestStableVersion(newVersions); latest != nil {
		if err := s.InstallVersion(ctx, pkg.ID, latest.Version); err != nil {
			slog.Warn("failed to auto-install latest version",
				slog.String("package", pkg.Slug),
				slog.String("version", latest.Version),
				slog.Any("error", err),
			)
		}
	}

	return s.repo.GetPackage(ctx, pkg.ID)
}

// RemovePackage deletes a package and cleans up its installed files.
func (s *packageService) RemovePackage(ctx context.Context, id string) error {
	pkg, err := s.repo.GetPackage(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching package: %w", err)
	}
	if pkg == nil {
		return fmt.Errorf("package not found")
	}

	if pkg.InstallPath != "" {
		if err := os.RemoveAll(pkg.InstallPath); err != nil {
			slog.Warn("failed to remove package files",
				slog.String("package", pkg.Slug),
				slog.String("path", pkg.InstallPath),
				slog.Any("error", err),
			)
		}
	}

	if err := s.repo.DeletePackage(ctx, id); err != nil {
		return fmt.Errorf("deleting package: %w", err)
	}

	slog.Info("package removed",
		slog.String("id", id),
		slog.String("slug", pkg.Slug),
	)

	// Invalidate serve cache so removed packages stop being served.
	if s.onServeInvalidate != nil {
		s.onServeInvalidate()
	}

	return nil
}

// --- Version Management ---

// CheckForUpdates fetches new releases from GitHub for a single package.
func (s *packageService) CheckForUpdates(ctx context.Context, packageID string) ([]PackageVersion, error) {
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return nil, fmt.Errorf("fetching package: %w", err)
	}
	if pkg == nil {
		return nil, fmt.Errorf("package not found")
	}

	return s.fetchAndImportVersions(ctx, pkg)
}

// CheckAllForUpdates fetches releases for all registered packages.
func (s *packageService) CheckAllForUpdates(ctx context.Context) error {
	packages, err := s.repo.ListPackages(ctx)
	if err != nil {
		return fmt.Errorf("listing packages: %w", err)
	}

	for i := range packages {
		if _, err := s.fetchAndImportVersions(ctx, &packages[i]); err != nil {
			slog.Warn("failed to check for updates",
				slog.String("package", packages[i].Slug),
				slog.Any("error", err),
			)
		}
	}
	return nil
}

// ListVersions returns all known versions for a package, newest first.
func (s *packageService) ListVersions(ctx context.Context, packageID string) ([]PackageVersion, error) {
	return s.repo.ListVersions(ctx, packageID)
}

// InstallVersion downloads and extracts a specific version of a package.
func (s *packageService) InstallVersion(ctx context.Context, packageID, version string) error {
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return fmt.Errorf("fetching package: %w", err)
	}
	if pkg == nil {
		return fmt.Errorf("package not found")
	}

	ver, err := s.repo.GetVersion(ctx, packageID, version)
	if err != nil {
		return fmt.Errorf("fetching version: %w", err)
	}
	if ver == nil {
		return fmt.Errorf("version %s not found for package %s", version, pkg.Slug)
	}

	if ver.DownloadURL == "" {
		return fmt.Errorf("no download URL for version %s", version)
	}

	dlDir := s.downloadsDir()
	if err := os.MkdirAll(dlDir, 0o755); err != nil {
		return fmt.Errorf("creating downloads directory: %w", err)
	}

	destDir := s.installDir(pkg.Type, pkg.Slug, version)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating install directory: %w", err)
	}

	zipName := fmt.Sprintf("%s-%s.zip", pkg.Slug, version)
	zipPath := filepath.Join(dlDir, zipName)
	_, err = s.github.DownloadAsset(ctx, ver.DownloadURL, zipPath)
	if err != nil {
		return fmt.Errorf("downloading version %s: %w", version, err)
	}

	if err := extractZip(zipPath, destDir); err != nil {
		_ = os.RemoveAll(destDir)
		return fmt.Errorf("extracting version %s: %w", version, err)
	}

	// Run content validation on extracted files.
	secSettings, _ := s.GetSecuritySettings(ctx)
	if err := ValidatePackageContents(destDir, secSettings.ValidateManifest, secSettings.ScanContent, secSettings.MaxFileSize); err != nil {
		_ = os.RemoveAll(destDir)
		return fmt.Errorf("content validation failed for %s: %w", version, err)
	}

	if err := s.repo.MarkVersionDownloaded(ctx, ver.ID); err != nil {
		slog.Warn("failed to mark version as downloaded",
			slog.String("version_id", ver.ID),
			slog.Any("error", err),
		)
	}

	// For Foundry module packages, update the version in module.json to match
	// the installed version tag. The GitHub release may contain a stale version
	// string in the manifest. Manifest/download URLs are rewritten at serve
	// time, so we only need to fix the version field here.
	if pkg.Type == PackageTypeFoundryModule {
		if err := rewriteModuleJSONVersion(destDir, version); err != nil {
			slog.Warn("failed to update module.json version",
				slog.String("version", version),
				slog.Any("error", err),
			)
		}
	}

	// For system packages, rewrite manifest.json version to match the release
	// tag. The manifest embedded in the GitHub release may have a stale version.
	if pkg.Type == PackageTypeSystem {
		if err := rewriteSystemManifestVersion(destDir, version); err != nil {
			slog.Warn("failed to update system manifest.json version",
				slog.String("version", version),
				slog.Any("error", err),
			)
		}
	}

	now := time.Now()
	pkg.InstalledVersion = version
	pkg.InstallPath = destDir
	pkg.LastInstalledAt = &now
	if err := s.repo.UpdatePackage(ctx, pkg); err != nil {
		return fmt.Errorf("updating package record: %w", err)
	}

	slog.Info("package version installed",
		slog.String("package", pkg.Slug),
		slog.String("version", version),
		slog.String("path", destDir),
	)

	// Notify system registry to rescan after a system package install.
	if pkg.Type != PackageTypeFoundryModule && s.onSystemInstall != nil {
		s.onSystemInstall()
	}

	// Invalidate serve cache so the new install path is picked up.
	if s.onServeInvalidate != nil {
		s.onServeInvalidate()
	}

	return nil
}

// SetPinnedVersion pins a package to a specific version, preventing auto-updates.
func (s *packageService) SetPinnedVersion(ctx context.Context, packageID, version string) error {
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return fmt.Errorf("fetching package: %w", err)
	}
	if pkg == nil {
		return fmt.Errorf("package not found")
	}

	ver, err := s.repo.GetVersion(ctx, packageID, version)
	if err != nil {
		return fmt.Errorf("fetching version: %w", err)
	}
	if ver == nil {
		return fmt.Errorf("version %s not found", version)
	}

	pkg.PinnedVersion = version
	return s.repo.UpdatePackage(ctx, pkg)
}

// ClearPinnedVersion removes the version pin, allowing auto-updates again.
func (s *packageService) ClearPinnedVersion(ctx context.Context, packageID string) error {
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return fmt.Errorf("fetching package: %w", err)
	}
	if pkg == nil {
		return fmt.Errorf("package not found")
	}

	pkg.PinnedVersion = ""
	return s.repo.UpdatePackage(ctx, pkg)
}

// SetAutoUpdate changes the auto-update policy for a package.
func (s *packageService) SetAutoUpdate(ctx context.Context, packageID string, policy UpdatePolicy) error {
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return fmt.Errorf("fetching package: %w", err)
	}
	if pkg == nil {
		return fmt.Errorf("package not found")
	}

	pkg.AutoUpdate = policy
	return s.repo.UpdatePackage(ctx, pkg)
}

// GetUsage returns which campaigns are using a given package. It looks up
// the package by ID, then queries campaign_addons for campaigns that have
// enabled an addon matching the package's addon slug.
//
// For system packages, the addon slug is the manifest ID (e.g., "drawsteel"),
// NOT the package slug (e.g., "Chronicle-Draw-Steel"). We read the manifest
// to resolve the correct addon slug. For Foundry module packages, the addon
// slug is always "sync-api" (the builtin sync API addon).
func (s *packageService) GetUsage(ctx context.Context, packageID string) ([]PackageUsage, error) {
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return nil, fmt.Errorf("getting package for usage: %w", err)
	}
	if pkg == nil {
		return []PackageUsage{}, nil
	}

	addonSlug := resolveAddonSlug(pkg)
	return s.repo.GetUsageByCampaign(ctx, addonSlug)
}

// resolveAddonSlug maps a package to its corresponding addon slug in the
// campaign_addons table. Package slugs (e.g., "Chronicle-Draw-Steel") don't
// match addon slugs (e.g., "drawsteel"), so we resolve based on package type.
func resolveAddonSlug(pkg *Package) string {
	switch pkg.Type {
	case PackageTypeSystem:
		// System packages register addons using the manifest ID.
		if id := getManifestIDFromDir(pkg.InstallPath); id != "" {
			return id
		}
		return pkg.Slug
	case PackageTypeFoundryModule:
		// The Foundry sync module always corresponds to the "sync-api" addon.
		return "sync-api"
	default:
		return pkg.Slug
	}
}

// getManifestIDFromDir reads the "id" field from a system package's
// manifest.json. Returns empty string on any error (missing file, bad JSON).
func getManifestIDFromDir(installPath string) string {
	if installPath == "" {
		slog.Warn("getManifestIDFromDir: empty install path")
		return ""
	}
	manifestPath := filepath.Join(installPath, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		slog.Warn("getManifestIDFromDir: cannot read manifest",
			slog.String("path", manifestPath),
			slog.Any("error", err),
		)
		return ""
	}
	var m struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		slog.Warn("getManifestIDFromDir: invalid manifest JSON",
			slog.String("path", manifestPath),
			slog.Any("error", err),
		)
		return ""
	}
	if m.ID == "" {
		slog.Warn("getManifestIDFromDir: manifest has no id field",
			slog.String("path", manifestPath),
		)
	}
	return m.ID
}

// --- Auto-Update Worker ---

// RunAutoUpdates checks all approved packages with auto-update enabled and
// installs new versions according to their update policy schedule.
// Skips packages that are not approved or are deprecated.
func (s *packageService) RunAutoUpdates(ctx context.Context) error {
	packages, err := s.repo.ListPackages(ctx)
	if err != nil {
		return fmt.Errorf("listing packages: %w", err)
	}

	now := time.Now()
	for i := range packages {
		pkg := &packages[i]

		// Only auto-update approved packages.
		if pkg.Status != StatusApproved {
			continue
		}

		if pkg.PinnedVersion != "" {
			continue
		}

		shouldCheck := false
		switch pkg.AutoUpdate {
		case UpdateNightly:
			shouldCheck = pkg.LastCheckedAt == nil || now.Sub(*pkg.LastCheckedAt) >= 24*time.Hour
		case UpdateWeekly:
			shouldCheck = pkg.LastCheckedAt == nil || now.Sub(*pkg.LastCheckedAt) >= 7*24*time.Hour
		case UpdateOnRelease:
			shouldCheck = pkg.LastCheckedAt == nil || now.Sub(*pkg.LastCheckedAt) >= 1*time.Hour
		case UpdateOff:
			continue
		}

		if !shouldCheck {
			continue
		}

		newVersions, err := s.fetchAndImportVersions(ctx, pkg)
		if err != nil {
			slog.Warn("auto-update check failed",
				slog.String("package", pkg.Slug),
				slog.Any("error", err),
			)
			continue
		}

		if latest := latestStableVersion(newVersions); latest != nil {
			if latest.Version != pkg.InstalledVersion {
				if err := s.InstallVersion(ctx, pkg.ID, latest.Version); err != nil {
					slog.Warn("auto-update install failed",
						slog.String("package", pkg.Slug),
						slog.String("version", latest.Version),
						slog.Any("error", err),
					)
				} else {
					slog.Info("auto-updated package",
						slog.String("package", pkg.Slug),
						slog.String("from", pkg.InstalledVersion),
						slog.String("to", latest.Version),
					)
				}
			}
		}
	}
	return nil
}

// StartAutoUpdateWorker runs a background loop that checks for updates hourly.
func (s *packageService) StartAutoUpdateWorker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	slog.Info("package auto-update worker started")

	for {
		select {
		case <-ctx.Done():
			slog.Info("package auto-update worker stopped")
			return
		case <-ticker.C:
			if err := s.RunAutoUpdates(ctx); err != nil {
				slog.Error("auto-update run failed", slog.Any("error", err))
			}
		}
	}
}

// FoundryModulePath returns the install path for the active Foundry module
// package, or empty string if none is installed.
func (s *packageService) FoundryModulePath() string {
	ctx := context.Background()
	packages, err := s.repo.ListPackages(ctx)
	if err != nil {
		return ""
	}
	for _, pkg := range packages {
		if pkg.Type == PackageTypeFoundryModule && pkg.InstallPath != "" && pkg.Status == StatusApproved {
			return pkg.InstallPath
		}
	}
	return ""
}

// FoundryModuleZipPath returns the cached ZIP file path for the active
// Foundry module package. Foundry VTT downloads this ZIP when installing
// the module via the manifest URL. Returns empty string if unavailable.
func (s *packageService) FoundryModuleZipPath() string {
	ctx := context.Background()
	packages, err := s.repo.ListPackages(ctx)
	if err != nil {
		return ""
	}
	for _, pkg := range packages {
		if pkg.Type == PackageTypeFoundryModule && pkg.InstalledVersion != "" && pkg.Status == StatusApproved {
			zipName := fmt.Sprintf("%s-%s.zip", pkg.Slug, pkg.InstalledVersion)
			zipPath := filepath.Join(s.downloadsDir(), zipName)
			if _, err := os.Stat(zipPath); err == nil {
				return zipPath
			}
		}
	}
	return ""
}

// InstalledPackagePath returns the on-disk install path for the active
// (installed + approved) package matching the given type and slug.
// Returns empty string if no matching package is installed.
func (s *packageService) InstalledPackagePath(pkgType PackageType, slug string) string {
	ctx := context.Background()
	packages, err := s.repo.ListPackages(ctx)
	if err != nil {
		return ""
	}
	for _, pkg := range packages {
		if pkg.Type == pkgType && pkg.Slug == slug && pkg.InstallPath != "" && pkg.Status == StatusApproved {
			return pkg.InstallPath
		}
	}
	return ""
}

// --- Submission/Approval Workflow ---

// SubmitPackage lets a campaign owner submit a repo URL for admin review.
// The submission always lands in the admin review queue as StatusPending,
// regardless of any settings. Install only happens after an admin explicitly
// approves the package via ReviewPackage. This prevents any non-admin user
// from triggering an outbound network fetch through this route.
//
// Submissions are also gated by OwnerUploadPolicy: when set to "disabled",
// this method refuses the submission outright before any DB write.
func (s *packageService) SubmitPackage(ctx context.Context, userID string, input SubmitPackageInput) (*Package, error) {
	repoURL := strings.TrimSpace(input.RepoURL)
	if repoURL == "" {
		return nil, fmt.Errorf("repository URL is required")
	}

	// Validate URL against security policy.
	secSettings, _ := s.GetSecuritySettings(ctx)
	if err := ValidateRepoURL(repoURL, secSettings.RepoPolicy); err != nil {
		return nil, err
	}

	// Honor the owner upload policy. "disabled" rejects the submission
	// before any DB write; the other values let it through to the admin
	// review queue.
	if secSettings.OwnerUploadPolicy == OwnerUploadDisabled {
		return nil, fmt.Errorf("owner submissions are disabled")
	}

	// Check for duplicates.
	existing, err := s.repo.FindByRepoURL(ctx, repoURL)
	if err != nil {
		return nil, fmt.Errorf("checking for duplicate: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("this repository has already been submitted: %s", existing.Name)
	}

	pkgType := PackageType(input.Type)
	if pkgType == "" {
		pkgType = PackageTypeSystem
	}

	// Try to parse owner/repo for GitHub URLs (non-GitHub URLs might not parse).
	owner, repo, _ := parseRepo(repoURL)
	slug := repo
	if slug == "" {
		// For non-GitHub URLs, derive slug from URL path.
		parts := strings.Split(strings.TrimSuffix(repoURL, "/"), "/")
		if len(parts) > 0 {
			slug = strings.TrimSuffix(parts[len(parts)-1], ".git")
		}
	}
	name := input.Name
	if name == "" {
		name = slug
	}

	now := time.Now()
	description := "Submitted by user"
	if owner != "" && repo != "" {
		description = fmt.Sprintf("Package from %s/%s", owner, repo)
	}

	pkg := &Package{
		ID:          generateUUID(),
		Type:        pkgType,
		Slug:        slug,
		Name:        name,
		RepoURL:     repoURL,
		Description: description,
		// Auto-update is opt-in (see AddPackage for rationale).
		AutoUpdate:    UpdateOff,
		SubmittedBy:   userID,
		Status:        StatusPending,
		LastCheckedAt: &now,
	}

	if err := s.repo.CreatePackage(ctx, pkg); err != nil {
		return nil, fmt.Errorf("creating package record: %w", err)
	}

	slog.Info("package submitted",
		slog.String("slug", pkg.Slug),
		slog.String("submitted_by", userID),
	)

	return s.repo.GetPackage(ctx, pkg.ID)
}

// ListMySubmissions returns packages submitted by the given user.
func (s *packageService) ListMySubmissions(ctx context.Context, userID string) ([]Package, error) {
	return s.repo.ListBySubmitter(ctx, userID)
}

// ListPendingSubmissions returns all packages awaiting admin approval.
func (s *packageService) ListPendingSubmissions(ctx context.Context) ([]Package, error) {
	return s.repo.ListByStatus(ctx, StatusPending)
}

// CountPendingSubmissions returns the number of packages awaiting approval.
func (s *packageService) CountPendingSubmissions(ctx context.Context) (int, error) {
	return s.repo.CountPendingSubmissions(ctx)
}

// ReviewPackage approves or rejects a pending package submission.
func (s *packageService) ReviewPackage(ctx context.Context, packageID, adminUserID string, input ReviewPackageInput) error {
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return fmt.Errorf("fetching package: %w", err)
	}
	if pkg == nil {
		return fmt.Errorf("package not found")
	}
	if pkg.Status != StatusPending {
		return fmt.Errorf("package is not pending review (current status: %s)", pkg.Status)
	}

	switch input.Action {
	case "approve":
		// Re-validate the URL before approving (could have been tampered).
		secSettings, _ := s.GetSecuritySettings(ctx)
		if err := ValidateRepoURL(pkg.RepoURL, secSettings.RepoPolicy); err != nil {
			return fmt.Errorf("URL no longer passes validation: %w", err)
		}

		if err := s.repo.UpdateStatus(ctx, packageID, StatusApproved, adminUserID, input.Note); err != nil {
			return fmt.Errorf("approving package: %w", err)
		}

		slog.Info("package approved",
			slog.String("package", pkg.Slug),
			slog.String("approved_by", adminUserID),
		)

		// Fetch versions and install in background.
		s.fetchAndInstallLatest(ctx, pkg)

	case "reject":
		if err := s.repo.UpdateStatus(ctx, packageID, StatusRejected, adminUserID, input.Note); err != nil {
			return fmt.Errorf("rejecting package: %w", err)
		}

		slog.Info("package rejected",
			slog.String("package", pkg.Slug),
			slog.String("rejected_by", adminUserID),
			slog.String("reason", input.Note),
		)

	default:
		return fmt.Errorf("invalid review action: %s (expected 'approve' or 'reject')", input.Action)
	}

	return nil
}

// --- Lifecycle Management ---

// DeprecatePackage marks a package as nearing end-of-life.
func (s *packageService) DeprecatePackage(ctx context.Context, packageID, msg string) error {
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return fmt.Errorf("fetching package: %w", err)
	}
	if pkg == nil {
		return fmt.Errorf("package not found")
	}

	if err := s.repo.SetDeprecated(ctx, packageID, msg); err != nil {
		return fmt.Errorf("deprecating package: %w", err)
	}

	slog.Info("package deprecated",
		slog.String("package", pkg.Slug),
		slog.String("message", msg),
	)
	return nil
}

// ArchivePackage hides a package from active listings.
func (s *packageService) ArchivePackage(ctx context.Context, packageID string) error {
	return s.repo.UpdateStatus(ctx, packageID, StatusArchived, "", "")
}

// UnarchivePackage restores an archived package to approved status.
func (s *packageService) UnarchivePackage(ctx context.Context, packageID string) error {
	return s.repo.UpdateStatus(ctx, packageID, StatusApproved, "", "")
}

// UpdateRepoURL changes a package's repository URL (admin only).
func (s *packageService) UpdateRepoURL(ctx context.Context, packageID, newURL string) error {
	newURL = strings.TrimSpace(newURL)

	secSettings, _ := s.GetSecuritySettings(ctx)
	if err := ValidateRepoURL(newURL, secSettings.RepoPolicy); err != nil {
		return err
	}

	return s.repo.UpdateRepoURL(ctx, packageID, newURL)
}

// --- Settings ---

// GetSecuritySettings returns the current package security configuration.
// Returns safe defaults if settings are not configured or the reader is nil.
func (s *packageService) GetSecuritySettings(ctx context.Context) (*PackageSecuritySettings, error) {
	defaults := &PackageSecuritySettings{
		RepoPolicy:        RepoPolicyGitHubOnly,
		RequireApproval:   true,
		MaxFileSize:       defaultMaxFileSize,
		ValidateManifest:  true,
		ScanContent:       true,
		OwnerUploadPolicy: OwnerUploadAutoApprove,
	}

	if s.settings == nil {
		return defaults, nil
	}

	if v, err := s.settings.Get(ctx, "packages.repo_policy"); err == nil {
		defaults.RepoPolicy = v
	}
	if v, err := s.settings.Get(ctx, "packages.require_approval"); err == nil {
		defaults.RequireApproval = v != "false"
	}
	if v, err := s.settings.Get(ctx, "packages.max_file_size"); err == nil {
		if n, parseErr := strconv.ParseInt(v, 10, 64); parseErr == nil && n > 0 {
			defaults.MaxFileSize = n
		}
	}
	if v, err := s.settings.Get(ctx, "packages.validate_manifest"); err == nil {
		defaults.ValidateManifest = v != "false"
	}
	if v, err := s.settings.Get(ctx, "packages.scan_content"); err == nil {
		defaults.ScanContent = v != "false"
	}
	if v, err := s.settings.Get(ctx, "packages.owner_upload_policy"); err == nil {
		defaults.OwnerUploadPolicy = v
	}

	return defaults, nil
}

// SaveSecuritySettings persists the security settings to the site_settings table.
func (s *packageService) SaveSecuritySettings(ctx context.Context, settings *PackageSecuritySettings) error {
	if s.settingsWriter == nil {
		return fmt.Errorf("settings writer not configured")
	}

	pairs := map[string]string{
		"packages.repo_policy":         settings.RepoPolicy,
		"packages.require_approval":    fmt.Sprintf("%t", settings.RequireApproval),
		"packages.max_file_size":       strconv.FormatInt(settings.MaxFileSize, 10),
		"packages.validate_manifest":   fmt.Sprintf("%t", settings.ValidateManifest),
		"packages.scan_content":        fmt.Sprintf("%t", settings.ScanContent),
		"packages.owner_upload_policy": settings.OwnerUploadPolicy,
	}

	for key, value := range pairs {
		if err := s.settingsWriter.Set(ctx, key, value); err != nil {
			return fmt.Errorf("saving %s: %w", key, err)
		}
	}

	return nil
}

// ReconcileOrphanedInstalls detects package directories on disk that have no
// corresponding DB record. This happens when the database is wiped but
// package files remain on disk.
func (s *packageService) ReconcileOrphanedInstalls(ctx context.Context) ([]OrphanedInstall, error) {
	pkgs, err := s.repo.ListPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}

	// Build set of known install paths.
	knownPaths := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		if pkg.InstallPath != "" {
			knownPaths[pkg.InstallPath] = true
		}
	}

	var orphans []OrphanedInstall

	// Scan systems directory: packages/systems/{slug}/{version}/
	systemsDir := filepath.Join(s.packagesDir(), "systems")
	if slugDirs, err := os.ReadDir(systemsDir); err == nil {
		for _, slugDir := range slugDirs {
			if !slugDir.IsDir() {
				continue
			}
			slugPath := filepath.Join(systemsDir, slugDir.Name())
			versionDirs, err := os.ReadDir(slugPath)
			if err != nil {
				continue
			}
			for _, verDir := range versionDirs {
				if !verDir.IsDir() {
					continue
				}
				fullPath := filepath.Join(slugPath, verDir.Name())
				if knownPaths[fullPath] {
					continue
				}
				orphans = append(orphans, OrphanedInstall{
					Path:    fullPath,
					Slug:    slugDir.Name(),
					Version: verDir.Name(),
					Size:    dirSize(fullPath),
				})
			}
		}
	}

	// Scan foundry-module directory: packages/foundry-module/{version}/
	foundryDir := filepath.Join(s.packagesDir(), "foundry-module")
	if versionDirs, err := os.ReadDir(foundryDir); err == nil {
		for _, verDir := range versionDirs {
			if !verDir.IsDir() {
				continue
			}
			fullPath := filepath.Join(foundryDir, verDir.Name())
			if knownPaths[fullPath] {
				continue
			}
			orphans = append(orphans, OrphanedInstall{
				Path:    fullPath,
				Slug:    "chronicle-sync",
				Version: verDir.Name(),
				Size:    dirSize(fullPath),
			})
		}
	}

	// Scan downloads directory for orphaned ZIPs.
	dlDir := s.downloadsDir()
	if files, err := os.ReadDir(dlDir); err == nil {
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			fullPath := filepath.Join(dlDir, f.Name())
			info, err := f.Info()
			if err != nil {
				continue
			}
			orphans = append(orphans, OrphanedInstall{
				Path:    fullPath,
				Slug:    "download-cache",
				Version: f.Name(),
				Size:    info.Size(),
			})
		}
	}

	return orphans, nil
}

// dirSize calculates the total size of all files in a directory tree.
func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// --- Internal Helpers ---

// fetchAndInstallLatest fetches releases and installs the latest version.
// Logs warnings on failure but does not return errors (fire-and-forget).
func (s *packageService) fetchAndInstallLatest(ctx context.Context, pkg *Package) {
	newVersions, err := s.fetchAndImportVersions(ctx, pkg)
	if err != nil {
		slog.Warn("failed to fetch releases after approval",
			slog.String("package", pkg.Slug),
			slog.Any("error", err),
		)
		return
	}

	if latest := latestStableVersion(newVersions); latest != nil {
		if err := s.InstallVersion(ctx, pkg.ID, latest.Version); err != nil {
			slog.Warn("failed to auto-install latest version",
				slog.String("package", pkg.Slug),
				slog.String("version", latest.Version),
				slog.Any("error", err),
			)
		}
	}
}

// fetchAndImportVersions fetches releases from GitHub and upserts them into
// the database. Returns all versions sorted by published_at descending.
func (s *packageService) fetchAndImportVersions(ctx context.Context, pkg *Package) ([]PackageVersion, error) {
	releases, err := s.github.ListReleases(ctx, pkg.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("fetching releases from GitHub: %w", err)
	}

	var versions []PackageVersion
	rp := repoPath(pkg.RepoURL)
	for _, rel := range releases {
		downloadURL, fileSize := pickDownloadAsset(rel.Assets)

		// Fall back to GitHub's auto-generated source zipball when the
		// release has no uploaded assets (common for pre-releases).
		if downloadURL == "" && rp != "" {
			downloadURL = fmt.Sprintf("https://api.github.com/repos/%s/zipball/%s", rp, rel.TagName)
		}

		v := PackageVersion{
			ID:           generateUUID(),
			PackageID:    pkg.ID,
			Version:      normalizeVersion(rel.TagName),
			TagName:      rel.TagName,
			ReleaseURL:   fmt.Sprintf("https://github.com/%s/releases/tag/%s", rp, rel.TagName),
			DownloadURL:  downloadURL,
			ReleaseNotes: rel.Body,
			Prerelease:   rel.Prerelease,
			PublishedAt:  rel.PublishedAt,
			FileSize:     fileSize,
		}

		if err := s.repo.UpsertVersion(ctx, &v); err != nil {
			slog.Warn("failed to upsert version",
				slog.String("package", pkg.Slug),
				slog.String("version", v.Version),
				slog.Any("error", err),
			)
			continue
		}
		versions = append(versions, v)
	}

	now := time.Now()
	pkg.LastCheckedAt = &now
	if err := s.repo.UpdatePackage(ctx, pkg); err != nil {
		slog.Warn("failed to update last_checked_at",
			slog.String("package", pkg.Slug),
			slog.Any("error", err),
		)
	}

	return versions, nil
}

// pickDownloadAsset selects the best download URL from a release's assets.
func pickDownloadAsset(assets []GitHubAsset) (url string, size int64) {
	for _, a := range assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".zip") {
			return a.BrowserDownloadURL, a.Size
		}
	}
	if len(assets) > 0 {
		return assets[0].BrowserDownloadURL, assets[0].Size
	}
	return "", 0
}

// latestStableVersion returns the first non-prerelease version from the list.
// If all versions are pre-releases, falls back to the newest pre-release.
func latestStableVersion(versions []PackageVersion) *PackageVersion {
	for i := range versions {
		if !versions[i].Prerelease {
			return &versions[i]
		}
	}
	if len(versions) > 0 {
		return &versions[0]
	}
	return nil
}

// normalizeVersion strips a leading "v" from tag names.
func normalizeVersion(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

// repoPath extracts "owner/repo" from a GitHub URL.
func repoPath(repoURL string) string {
	owner, repo, err := parseRepo(repoURL)
	if err != nil {
		return ""
	}
	return owner + "/" + repo
}

// rewriteModuleJSONVersion updates the "version" field in a Foundry module's
// module.json to match the installed version tag from the package manager.
func rewriteModuleJSONVersion(dir, version string) error {
	manifestPath := filepath.Join(dir, "module.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read module.json: %w", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse module.json: %w", err)
	}

	manifest["version"] = version

	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal module.json: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(manifestPath, out, 0644); err != nil {
		return fmt.Errorf("write module.json: %w", err)
	}
	return nil
}

// rewriteSystemManifestVersion updates the "version" field in a system
// package's manifest.json to match the installed version tag. Same purpose
// as rewriteModuleJSONVersion but for system packages: the GitHub release
// tag is the source of truth, not the version embedded in the manifest.
func rewriteSystemManifestVersion(dir, version string) error {
	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest.json: %w", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse manifest.json: %w", err)
	}

	manifest["version"] = version

	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest.json: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(manifestPath, out, 0644); err != nil {
		return fmt.Errorf("write manifest.json: %w", err)
	}
	return nil
}

// extractZip extracts a ZIP file to a destination directory.
// If the zip contains a single top-level directory (common for GitHub source
// archives), the contents are "unwrapped" so files land directly in destDir.
func extractZip(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer func() { _ = zr.Close() }()

	// Detect single top-level directory wrapper (e.g. "owner-repo-hash/").
	prefix := detectSingleRootDir(zr.File)

	for _, zf := range zr.File {
		name := zf.Name
		if strings.Contains(name, "..") {
			return fmt.Errorf("invalid path in zip: %s", name)
		}

		// Strip the wrapper directory prefix when present.
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
			if name == "" {
				continue // skip the wrapper directory entry itself
			}
		}

		destPath := filepath.Join(destDir, name)

		// Canonicalize and verify the path is still under destDir to prevent
		// path traversal attacks (e.g., symlink tricks that bypass ".." checks).
		cleanDest := filepath.Clean(destPath)
		cleanBase := filepath.Clean(destDir) + string(os.PathSeparator)
		if !strings.HasPrefix(cleanDest, cleanBase) && cleanDest != filepath.Clean(destDir) {
			return fmt.Errorf("path traversal detected in zip: %s", name)
		}

		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return fmt.Errorf("creating directory %s: %w", destPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("creating parent dir: %w", err)
		}

		if err := extractSingleFile(zf, destPath); err != nil {
			return err
		}
	}
	return nil
}

// detectSingleRootDir checks whether all entries in the zip share a single
// top-level directory prefix (e.g. "owner-repo-abc123/"). Returns the prefix
// to strip, or "" if the zip has files at its root.
func detectSingleRootDir(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}

	var prefix string
	for _, f := range files {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) < 2 {
			// File at root level — no wrapper.
			return ""
		}
		dir := parts[0] + "/"
		if prefix == "" {
			prefix = dir
		} else if dir != prefix {
			return ""
		}
	}
	return prefix
}

// extractSingleFile extracts one file from the ZIP archive to disk.
func extractSingleFile(zf *zip.File, destPath string) error {
	rc, err := zf.Open()
	if err != nil {
		return fmt.Errorf("opening %s in zip: %w", zf.Name, err)
	}
	defer func() { _ = rc.Close() }()

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", destPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("writing %s: %w", destPath, err)
	}
	return nil
}
