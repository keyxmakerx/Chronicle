package packages

import (
	"archive/zip"
	"context"
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

	// SeedOfficialPackages creates the official Chronicle repos if none exist.
	SeedOfficialPackages(ctx context.Context)
}

// packageService implements PackageService.
type packageService struct {
	repo     PackageRepository
	github   *GitHubClient
	settings SettingsReader
	mediaDir string // Root media directory (e.g., ./media).
}

// NewPackageService creates a new package service with the given dependencies.
func NewPackageService(repo PackageRepository, github *GitHubClient, mediaDir string) PackageService {
	return &packageService{
		repo:     repo,
		github:   github,
		mediaDir: mediaDir,
	}
}

// ConfigureSettings wires a settings reader into the package service.
// Called from routes.go after both services are initialized.
func ConfigureSettings(svc PackageService, settings SettingsReader) {
	if s, ok := svc.(*packageService); ok {
		s.settings = settings
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
		ID:            generateUUID(),
		Type:          pkgType,
		Slug:          slug,
		Name:          name,
		RepoURL:       repoURL,
		Description:   fmt.Sprintf("Package from %s/%s", owner, repo),
		AutoUpdate:    UpdateNightly,
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

	// Auto-install the latest version if available.
	if len(newVersions) > 0 {
		latest := newVersions[0]
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
// enabled an addon matching the package's slug.
func (s *packageService) GetUsage(ctx context.Context, packageID string) ([]PackageUsage, error) {
	pkg, err := s.repo.GetPackage(ctx, packageID)
	if err != nil {
		return nil, fmt.Errorf("getting package for usage: %w", err)
	}
	if pkg == nil {
		return []PackageUsage{}, nil
	}
	return s.repo.GetUsageByCampaign(ctx, pkg.Slug)
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

		if len(newVersions) > 0 {
			latest := newVersions[0]
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

// --- Submission/Approval Workflow ---

// SubmitPackage lets a campaign owner submit a repo URL for review.
// If admin approval is not required, the package is auto-approved and fetched.
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

	// Determine initial status based on approval setting.
	status := StatusPending
	if !secSettings.RequireApproval {
		status = StatusApproved
	}

	now := time.Now()
	description := "Submitted by user"
	if owner != "" && repo != "" {
		description = fmt.Sprintf("Package from %s/%s", owner, repo)
	}

	pkg := &Package{
		ID:            generateUUID(),
		Type:          pkgType,
		Slug:          slug,
		Name:          name,
		RepoURL:       repoURL,
		Description:   description,
		AutoUpdate:    UpdateNightly,
		SubmittedBy:   userID,
		Status:        status,
		LastCheckedAt: &now,
	}

	if err := s.repo.CreatePackage(ctx, pkg); err != nil {
		return nil, fmt.Errorf("creating package record: %w", err)
	}

	slog.Info("package submitted",
		slog.String("slug", pkg.Slug),
		slog.String("submitted_by", userID),
		slog.String("status", string(status)),
	)

	// If auto-approved, immediately fetch versions and install.
	if status == StatusApproved {
		s.fetchAndInstallLatest(ctx, pkg)
	}

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
		RepoPolicy:       RepoPolicyGitHubOnly,
		RequireApproval:  true,
		MaxFileSize:      defaultMaxFileSize,
		ValidateManifest: true,
		ScanContent:      true,
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

	return defaults, nil
}

// SeedOfficialPackages creates the official Chronicle repos if no packages
// exist yet. This runs on startup to ensure the default system pack and
// Foundry module repos are registered. Existing installations are untouched.
func (s *packageService) SeedOfficialPackages(ctx context.Context) {
	existing, err := s.repo.ListPackages(ctx)
	if err != nil {
		slog.Warn("failed to list packages for seeding", slog.Any("error", err))
		return
	}

	// Don't seed if any packages already exist.
	if len(existing) > 0 {
		return
	}

	seeds := []struct {
		name    string
		repoURL string
		pkgType PackageType
	}{
		{
			name:    "Chronicle Systems",
			repoURL: "https://github.com/keyxmakerx/chronicle-systems",
			pkgType: PackageTypeSystem,
		},
		{
			name:    "Chronicle Foundry Module",
			repoURL: "https://github.com/keyxmakerx/chronicle-foundry-module",
			pkgType: PackageTypeFoundryModule,
		},
	}

	for _, seed := range seeds {
		now := time.Now()
		pkg := &Package{
			ID:            generateUUID(),
			Type:          seed.pkgType,
			Slug:          strings.TrimPrefix(seed.repoURL, "https://github.com/keyxmakerx/"),
			Name:          seed.name,
			RepoURL:       seed.repoURL,
			Description:   "Official Chronicle package",
			AutoUpdate:    UpdateNightly,
			Status:        StatusApproved,
			LastCheckedAt: &now,
		}

		if err := s.repo.CreatePackage(ctx, pkg); err != nil {
			slog.Warn("failed to seed official package",
				slog.String("name", seed.name),
				slog.Any("error", err),
			)
			continue
		}

		slog.Info("seeded official package",
			slog.String("name", seed.name),
			slog.String("repo", seed.repoURL),
		)

		// Attempt to fetch and install (non-fatal if GitHub is unreachable).
		s.fetchAndInstallLatest(ctx, pkg)
	}
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

	if len(newVersions) > 0 {
		latest := newVersions[0]
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
	for _, rel := range releases {
		downloadURL, fileSize := pickDownloadAsset(rel.Assets)

		v := PackageVersion{
			ID:           generateUUID(),
			PackageID:    pkg.ID,
			Version:      normalizeVersion(rel.TagName),
			TagName:      rel.TagName,
			ReleaseURL:   fmt.Sprintf("https://github.com/%s/releases/tag/%s", repoPath(pkg.RepoURL), rel.TagName),
			DownloadURL:  downloadURL,
			ReleaseNotes: rel.Body,
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

// extractZip extracts a ZIP file to a destination directory.
func extractZip(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer func() { _ = zr.Close() }()

	for _, zf := range zr.File {
		if strings.Contains(zf.Name, "..") {
			return fmt.Errorf("invalid path in zip: %s", zf.Name)
		}

		destPath := filepath.Join(destDir, zf.Name)

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
