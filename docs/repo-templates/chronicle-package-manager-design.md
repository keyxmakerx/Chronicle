# Chronicle Package Manager Design

This document describes the Chronicle-side changes needed to pull the Foundry module
and game systems from external repositories, with full admin version management.

## Overview

Chronicle will have a built-in package manager accessible from the admin panel.
It fetches releases from GitHub repositories, allows admins to select versions,
pin to known-good releases, enable auto-updates, and see which campaigns use
which packages.

**No packages are bundled in the Chronicle binary.** Everything is pulled from
external repos on first startup or on admin demand.

---

## Architecture

### New Plugin: `internal/plugins/packages/`

Following Chronicle's standard plugin structure:

```
internal/plugins/packages/
  .ai.md              # Plugin documentation
  embed.go            # Embeds migrations/*.sql
  handler.go          # Admin handlers (thin: bind, call service, render)
  service.go          # Business logic (fetch releases, install, update)
  repository.go       # SQL queries for packages/versions tables
  model.go            # Package, PackageVersion, UpdatePolicy structs
  routes.go           # Route registration
  github_client.go    # GitHub Releases API client
  migrations/
    001_create_packages.up.sql
    001_create_packages.down.sql
  templates/
    packages.templ     # Admin package management page
    partials/
      package_card.templ
      version_picker.templ
      usage_table.templ
```

---

## Database Schema

### Migration: 001_create_packages

```sql
-- Package registry: tracks external repos and their installed state.
CREATE TABLE packages (
    id          CHAR(36) PRIMARY KEY,
    type        ENUM('system', 'foundry-module') NOT NULL,
    slug        VARCHAR(100) NOT NULL UNIQUE,
    name        VARCHAR(255) NOT NULL,
    repo_url    VARCHAR(500) NOT NULL,
    description TEXT,

    -- Version management
    installed_version   VARCHAR(50),     -- Currently installed semver (NULL = not installed)
    pinned_version      VARCHAR(50),     -- Pinned version (NULL = use latest)
    auto_update         ENUM('off', 'nightly', 'weekly', 'on_release') NOT NULL DEFAULT 'off',

    -- Tracking
    last_checked_at     DATETIME,        -- Last time we checked for updates
    last_installed_at   DATETIME,        -- Last time a version was installed
    install_path        VARCHAR(500),    -- Disk path where package is extracted

    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- Version history: all known versions from GitHub releases.
CREATE TABLE package_versions (
    id              CHAR(36) PRIMARY KEY,
    package_id      CHAR(36) NOT NULL,
    version         VARCHAR(50) NOT NULL,
    tag_name        VARCHAR(100) NOT NULL,
    release_url     VARCHAR(500) NOT NULL,     -- GitHub release page URL
    download_url    VARCHAR(500) NOT NULL,      -- ZIP asset download URL
    release_notes   TEXT,
    published_at    DATETIME NOT NULL,
    downloaded_at   DATETIME,                   -- NULL if never downloaded
    file_size       BIGINT,                     -- ZIP file size in bytes

    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (package_id) REFERENCES packages(id) ON DELETE CASCADE,
    UNIQUE KEY uk_package_version (package_id, version)
);

-- Index for efficient lookups
CREATE INDEX idx_package_versions_package ON package_versions(package_id, published_at DESC);
```

---

## Model Types

```go
// Package represents an external repository tracked by the package manager.
type Package struct {
    ID               string
    Type             PackageType // "system" or "foundry-module"
    Slug             string
    Name             string
    RepoURL          string
    Description      string
    InstalledVersion string     // "" if not installed
    PinnedVersion    string     // "" if using latest
    AutoUpdate       UpdatePolicy
    LastCheckedAt    *time.Time
    LastInstalledAt  *time.Time
    InstallPath      string
}

// PackageVersion represents a single release from GitHub.
type PackageVersion struct {
    ID           string
    PackageID    string
    Version      string
    TagName      string
    ReleaseURL   string
    DownloadURL  string
    ReleaseNotes string
    PublishedAt  time.Time
    DownloadedAt *time.Time
    FileSize     int64
}

// PackageType distinguishes system packs from Foundry modules.
type PackageType string
const (
    PackageTypeSystem       PackageType = "system"
    PackageTypeFoundryModule PackageType = "foundry-module"
)

// UpdatePolicy controls automatic update behavior.
type UpdatePolicy string
const (
    UpdateOff       UpdatePolicy = "off"
    UpdateNightly   UpdatePolicy = "nightly"
    UpdateWeekly    UpdatePolicy = "weekly"
    UpdateOnRelease UpdatePolicy = "on_release"
)

// PackageUsage shows which campaigns use a package.
type PackageUsage struct {
    CampaignID   string
    CampaignName string
    SystemID     string
    EnabledAt    time.Time
}
```

---

## Service Interface

```go
type PackageService interface {
    // Package CRUD
    ListPackages(ctx context.Context) ([]Package, error)
    GetPackage(ctx context.Context, id string) (*Package, error)
    AddPackage(ctx context.Context, repoURL string) (*Package, error)
    RemovePackage(ctx context.Context, id string) error

    // Version management
    CheckForUpdates(ctx context.Context, packageID string) ([]PackageVersion, error)
    CheckAllForUpdates(ctx context.Context) error
    ListVersions(ctx context.Context, packageID string) ([]PackageVersion, error)
    InstallVersion(ctx context.Context, packageID, version string) error
    SetPinnedVersion(ctx context.Context, packageID, version string) error
    ClearPinnedVersion(ctx context.Context, packageID string) error
    SetAutoUpdate(ctx context.Context, packageID string, policy UpdatePolicy) error

    // Usage tracking
    GetUsage(ctx context.Context, packageID string) ([]PackageUsage, error)

    // Auto-update worker
    RunAutoUpdates(ctx context.Context) error
}
```

---

## GitHub Client

```go
// GitHubClient fetches release information from GitHub repositories.
type GitHubClient struct {
    httpClient *http.Client
}

// ListReleases fetches all releases for a repository.
// Parses repo URL to extract owner/repo: "https://github.com/owner/repo"
func (c *GitHubClient) ListReleases(ctx context.Context, repoURL string) ([]GitHubRelease, error)

// DownloadAsset downloads a release asset (ZIP file) to disk.
func (c *GitHubClient) DownloadAsset(ctx context.Context, downloadURL, destPath string) error

// GitHubRelease represents a GitHub release.
type GitHubRelease struct {
    TagName     string
    Name        string
    Body        string // Release notes (markdown)
    PublishedAt time.Time
    Assets      []GitHubAsset
}

// GitHubAsset represents a downloadable file in a release.
type GitHubAsset struct {
    Name        string
    DownloadURL string
    Size        int64
    ContentType string
}
```

**GitHub API usage:**
- `GET https://api.github.com/repos/{owner}/{repo}/releases` — list releases
- No authentication required for public repos (60 req/hour rate limit)
- Optional: support `GITHUB_TOKEN` env var for higher rate limits (5000 req/hour)

---

## Admin Routes

```go
func RegisterRoutes(admin *echo.Group, h *Handler) {
    g := admin.Group("/packages")

    g.GET("", h.ListPackages)                        // Package management page
    g.POST("", h.AddPackage)                         // Add a new package repo
    g.DELETE("/:id", h.RemovePackage)                // Remove a package

    g.GET("/:id/versions", h.ListVersions)           // List available versions (HTMX)
    g.PUT("/:id/version", h.InstallVersion)          // Install specific version
    g.PUT("/:id/pin", h.SetPinnedVersion)            // Pin to a version
    g.DELETE("/:id/pin", h.ClearPinnedVersion)       // Unpin (use latest)
    g.PUT("/:id/auto-update", h.SetAutoUpdate)       // Set auto-update policy
    g.POST("/:id/check", h.CheckForUpdates)          // Manual update check

    g.GET("/:id/usage", h.GetUsage)                  // Which campaigns use this (HTMX)
}
```

---

## Installation Flow

### Adding a Package

1. Admin enters GitHub repo URL (e.g., `https://github.com/keyxmakerx/chronicle-systems`)
2. Service fetches releases from GitHub API
3. Determines package type from releases:
   - If ZIP contains `module.json` → `foundry-module`
   - If ZIP contains `manifest.json` + `data/` → `system`
4. Creates `packages` record
5. Imports all versions into `package_versions`
6. Installs latest version (or pinned version) automatically

### Installing a Version

For **system packages**:
1. Download ZIP from GitHub release
2. Extract to `media/packages/systems/<slug>/<version>/`
3. For each system directory in the ZIP:
   - Validate manifest.json
   - Validate data/*.json files
   - Load as GenericSystem (NO `custom-` prefix — these are managed packages)
4. Register in global system registry
5. Update `packages.installed_version` and `packages.install_path`

For **foundry-module packages**:
1. Download ZIP from GitHub release
2. Extract to `media/packages/foundry-module/<version>/`
3. Rewrite `module.json` URLs to point at this Chronicle instance
4. Update `packages.installed_version` and `packages.install_path`
5. Foundry module serving routes now serve from this path

### Version Pinning

- When `pinned_version` is set, auto-updates skip this package
- Admin can still manually install any version
- Pinning is useful for:
  - Staying on a known-good version after a buggy release
  - Testing before upgrading
  - Campaign-specific compatibility requirements

---

## Auto-Update Worker

A background goroutine runs on a configurable schedule:

```go
func (s *packageService) StartAutoUpdateWorker(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.runScheduledUpdates(ctx)
        }
    }
}

func (s *packageService) runScheduledUpdates(ctx context.Context) {
    packages, _ := s.repo.ListPackages(ctx)
    now := time.Now()

    for _, pkg := range packages {
        if pkg.PinnedVersion != "" {
            continue // Pinned, skip
        }

        shouldCheck := false
        switch pkg.AutoUpdate {
        case UpdateNightly:
            shouldCheck = pkg.LastCheckedAt == nil || now.Sub(*pkg.LastCheckedAt) >= 24*time.Hour
        case UpdateWeekly:
            shouldCheck = pkg.LastCheckedAt == nil || now.Sub(*pkg.LastCheckedAt) >= 7*24*time.Hour
        case UpdateOnRelease:
            shouldCheck = true // Always check (webhook-based would be better)
        }

        if shouldCheck {
            newVersions, _ := s.CheckForUpdates(ctx, pkg.ID)
            if len(newVersions) > 0 {
                latest := newVersions[0] // Sorted by published_at DESC
                if latest.Version != pkg.InstalledVersion {
                    s.InstallVersion(ctx, pkg.ID, latest.Version)
                    slog.Info("auto-updated package",
                        slog.String("package", pkg.Slug),
                        slog.String("from", pkg.InstalledVersion),
                        slog.String("to", latest.Version),
                    )
                }
            }
        }
    }
}
```

---

## Chronicle Code Changes Required

### 1. Remove Bundled Foundry Module

**Files to modify:**
- `internal/app/app.go` — Remove `serveFoundryModuleManifest()`, `serveFoundryModuleZip()`, and `/foundry-module/*` static routes. Replace with proxy to package manager's installed version.
- `Dockerfile` — Remove `COPY --from=builder /src/foundry-module /app/foundry-module`

**New serving logic:**
```go
// In app.go setupRoutes() or similar:
e.GET("/foundry-module/module.json", app.servePackagedFoundryManifest)
e.GET("/foundry-module/chronicle-sync.zip", app.servePackagedFoundryZip)
e.Static("/foundry-module", packageService.FoundryModulePath())
```

Where `packageService.FoundryModulePath()` returns the installed version's extracted directory.

### 2. Remove Bundled System Data

**Files to modify:**
- `cmd/server/main.go` — Remove `_ "github.com/keyxmakerx/chronicle/internal/systems/dnd5e"` blank import
- `cmd/server/main.go` — Change `systems.Init("internal/systems")` to scan the package manager's install directory
- Delete data files: `internal/systems/dnd5e/data/`, `internal/systems/pathfinder2e/data/`, `internal/systems/drawsteel/data/`
- Delete manifests: `internal/systems/*/manifest.json`

**Keep:**
- `internal/systems/dnd5e/dnd5e.go` — Custom tooltip renderer. Registers by system ID, matches regardless of where data was loaded from.
- All Go code in `internal/systems/` (system.go, manifest.go, registry.go, loader.go, etc.)

### 3. System Loading from Package Manager

**Option A (recommended): Scan package install directory**
```go
// In main.go:
// Load systems from package manager's install directory
packageSystemsDir := filepath.Join(config.Upload.MediaPath, "packages", "systems")
if err := systems.Init(packageSystemsDir); err != nil {
    slog.Warn("no packaged systems found", slog.Any("error", err))
}
```

The existing `SystemLoader.DiscoverAll()` already scans subdirectories for `manifest.json`.
It would just need to handle the nested structure:
```
media/packages/systems/
  chronicle-systems/     # Package slug
    v1.0.0/              # Installed version
      dnd5e/
        manifest.json
        data/
      drawsteel/
        manifest.json
        data/
```

**Key change:** Systems loaded via the package manager must NOT get the `custom-` prefix.
The `CampaignSystemManager.Install()` adds this prefix, but package-managed systems
should use their original IDs so that:
- The dnd5e factory registration matches
- Addon enablement records remain consistent
- Foundry module system detection works

### 4. Handle the `custom-` Prefix Issue

Add a `SystemSource` concept:

```go
type SystemSource string
const (
    SourceBuiltin  SystemSource = "builtin"   // From package manager
    SourceCustom   SystemSource = "custom"    // User-uploaded per-campaign
)
```

`CampaignSystemManager.Install()` continues to prefix with `custom-` for
user-uploaded systems. Package-managed systems are loaded through `SystemLoader`
which does NOT apply any prefix.

### 5. Admin UI Integration

Add a "Packages" link to the admin sidebar navigation:

```go
// In admin/routes.go or wherever the sidebar is defined:
{ Name: "Packages", URL: "/admin/packages", Icon: "fa-box" }
```

The Foundry module admin page (`/admin/foundry`) should show:
- Currently installed module version (from package manager)
- Install URL (still served by Chronicle)
- Link to package manager for version management

---

## UI Design

### Package Management Page (`/admin/packages`)

```
┌─────────────────────────────────────────────────────────┐
│  Packages                                    [Add Package] │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  ┌─────────────────────────────────────────────────────┐ │
│  │ 📦 Chronicle Foundry Module           v0.2.0 ✓      │ │
│  │    github.com/keyxmakerx/chronicle-foundry-module    │ │
│  │    Auto-update: Nightly  │  Pinned: No               │ │
│  │    [Change Version ▼]  [Pin]  [Check Now]  [Usage]   │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                           │
│  ┌─────────────────────────────────────────────────────┐ │
│  │ 📦 Chronicle Systems                  v1.0.0 ✓      │ │
│  │    github.com/keyxmakerx/chronicle-systems           │ │
│  │    Auto-update: Weekly  │  Pinned: v1.0.0            │ │
│  │    Systems: dnd5e, drawsteel, pathfinder2e            │ │
│  │    [Change Version ▼]  [Unpin]  [Check Now]  [Usage] │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

### Version Picker (HTMX dropdown)

```
┌─────────────────────────────────┐
│ Available Versions               │
├─────────────────────────────────┤
│ ● v1.2.0  (latest) 2026-03-15  │  ← Currently installed
│   v1.1.0            2026-03-01  │
│   v1.0.0            2026-02-15  │  ← Pinned
│   v0.9.0            2026-02-01  │
│                  [Install v1.1.0]│
└─────────────────────────────────┘
```

### Usage Panel (HTMX fragment)

```
┌─────────────────────────────────┐
│ Campaigns Using This Package     │
├─────────────────────────────────┤
│ Campaign Name    │ System  │ Since │
│ Eberron Rising   │ dnd5e   │ Jan 5 │
│ Iron Dawn        │ drawsteel│ Mar 1 │
│ Golarion         │ pf2e    │ Feb 10│
└─────────────────────────────────┘
```

---

## Default Packages on First Boot

When Chronicle starts with an empty `packages` table, it should seed with
the official repos:

```go
func (s *packageService) SeedDefaults(ctx context.Context) error {
    defaults := []struct {
        RepoURL string
        Name    string
    }{
        {"https://github.com/keyxmakerx/chronicle-foundry-module", "Chronicle Foundry Module"},
        {"https://github.com/keyxmakerx/chronicle-systems", "Chronicle Systems"},
    }

    for _, d := range defaults {
        existing, _ := s.repo.FindByRepoURL(ctx, d.RepoURL)
        if existing != nil {
            continue
        }
        _, err := s.AddPackage(ctx, d.RepoURL)
        if err != nil {
            slog.Warn("failed to seed default package",
                slog.String("repo", d.RepoURL),
                slog.Any("error", err),
            )
        }
    }
    return nil
}
```

This runs on startup. If GitHub is unreachable, it logs a warning and continues.
The admin can manually add packages later.

---

## File Storage Layout

```
media/
  packages/
    foundry-module/
      v0.2.0/                    # Extracted Foundry module files
        module.json
        scripts/
        templates/
        styles/
        lang/
    systems/
      v1.0.0/                    # Extracted system packs
        dnd5e/
          manifest.json
          data/
        drawsteel/
          manifest.json
          data/
        pathfinder2e/
          manifest.json
          data/
    downloads/                   # Cached ZIP downloads
      chronicle-foundry-module-v0.2.0.zip
      chronicle-systems-v1.0.0.zip
  systems/                      # Existing: per-campaign custom uploads
    <campaignID>/
      manifest.json
      data/
```

---

## Implementation Order

1. **Database migration** — Create `packages` and `package_versions` tables
2. **GitHub client** — Fetch releases, download assets
3. **Repository** — SQL queries for package/version CRUD
4. **Service** — Business logic: add, install, check, auto-update
5. **Handler + Templates** — Admin UI with HTMX
6. **Routes** — Wire up admin routes
7. **System loading changes** — Scan package install directory instead of `internal/systems/`
8. **Foundry module serving** — Proxy to installed package version
9. **Auto-update worker** — Background goroutine
10. **Seed defaults** — Auto-add official repos on first boot
11. **Remove bundled data** — Delete `foundry-module/` and system data files from monorepo
