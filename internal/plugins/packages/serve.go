// Package packages — serve.go provides HTTP handlers for serving files from
// installed packages. Any package type (Foundry module, system, etc.) installed
// via the package manager has its files automatically servable at a public URL.
//
// Security: path traversal prevention via filepath.Clean + prefix check,
// per-IP rate limiting (applied at route level), and permissive CORS for
// external VTT clients.
package packages

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
)

// ServeHandler serves static files from installed packages.
type ServeHandler struct {
	svc     PackageService
	baseURL string // Public base URL for URL rewriting (e.g., "https://chronicle.bnuuy.haus").

	mu    sync.RWMutex
	cache map[string]string // "type/slug" -> install path
}

// NewServeHandler creates a handler for serving package files.
func NewServeHandler(svc PackageService, baseURL string) *ServeHandler {
	return &ServeHandler{
		svc:     svc,
		baseURL: strings.TrimRight(baseURL, "/"),
		cache:   make(map[string]string),
	}
}

// InvalidateCache clears the cached install paths, forcing a fresh lookup
// on the next request. Call this after install, update, or uninstall.
func (h *ServeHandler) InvalidateCache() {
	h.mu.Lock()
	h.cache = make(map[string]string)
	h.mu.Unlock()
}

// resolvePath looks up the install path for a package type+slug combo.
// Results are cached in memory to avoid repeated DB queries.
func (h *ServeHandler) resolvePath(pkgType, slug string) string {
	key := pkgType + "/" + slug

	h.mu.RLock()
	if p, ok := h.cache[key]; ok {
		h.mu.RUnlock()
		return p
	}
	h.mu.RUnlock()

	// Cache miss — query the service.
	p := h.svc.InstalledPackagePath(PackageType(pkgType), slug)

	h.mu.Lock()
	h.cache[key] = p
	h.mu.Unlock()

	return p
}

// ServePackageFile handles GET /packages/serve/:type/:slug/*
// It resolves the package install path and serves the requested file.
func (h *ServeHandler) ServePackageFile(c echo.Context) error {
	pkgType := c.Param("type")
	slug := c.Param("slug")
	filePath := c.Param("*")

	if pkgType == "" || slug == "" || filePath == "" {
		return c.NoContent(http.StatusNotFound)
	}

	installPath := h.resolvePath(pkgType, slug)
	if installPath == "" {
		return c.NoContent(http.StatusNotFound)
	}

	return h.serveFile(c, installPath, filePath)
}

// ServeFoundryAlias handles GET /foundry-module/* for backwards compatibility.
// It resolves the active Foundry module's install path automatically.
// For module.json requests, manifest and download URLs are rewritten to
// point at this Chronicle instance so Foundry can install/update the module
// without needing access to GitHub.
func (h *ServeHandler) ServeFoundryAlias(c echo.Context) error {
	filePath := c.Param("*")
	if filePath == "" {
		return c.NoContent(http.StatusNotFound)
	}

	installPath := h.resolvePath(string(PackageTypeFoundryModule), "chronicle-foundry")
	if installPath == "" {
		// Fallback: try resolving any foundry-module type package.
		installPath = h.svc.FoundryModulePath()
		if installPath == "" {
			return c.NoContent(http.StatusNotFound)
		}
	}

	// Intercept module.json to rewrite manifest/download URLs.
	if filePath == "module.json" {
		return h.serveFoundryManifest(c, installPath)
	}

	return h.serveFile(c, installPath, filePath)
}

// ServeFoundryDownload handles GET /foundry-module/download to serve the
// cached ZIP file for Foundry module installation. Foundry expects a ZIP
// at the URL specified in module.json's "download" field.
func (h *ServeHandler) ServeFoundryDownload(c echo.Context) error {
	zipPath := h.svc.FoundryModuleZipPath()
	if zipPath == "" {
		return c.NoContent(http.StatusNotFound)
	}

	info, err := os.Stat(zipPath)
	if err != nil || info.IsDir() {
		return c.NoContent(http.StatusNotFound)
	}

	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	c.Response().Header().Set("Content-Type", "application/zip")
	return c.File(zipPath)
}

// serveFoundryManifest reads module.json from disk, rewrites the manifest
// and download URLs to point at this Chronicle instance, and returns it.
func (h *ServeHandler) serveFoundryManifest(c echo.Context, installPath string) error {
	manifestPath := filepath.Join(installPath, "module.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		slog.Error("foundry manifest: invalid JSON", slog.Any("error", err))
		return c.NoContent(http.StatusInternalServerError)
	}

	if h.baseURL == "" {
		slog.Warn("foundry manifest served with empty baseURL — manifest/download URLs will be malformed; set BASE_URL in config")
	}

	// Rewrite manifest URL to point at this instance.
	manifest["manifest"] = fmt.Sprintf("%s/foundry-module/module.json", h.baseURL)

	// Rewrite download URL to serve the cached ZIP from this instance.
	manifest["download"] = fmt.Sprintf("%s/foundry-module/download", h.baseURL)

	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Cache-Control", "public, max-age=300")
	return c.JSON(http.StatusOK, manifest)
}

// serveFile safely resolves and serves a file from the given base directory.
// Prevents path traversal by cleaning the path and verifying it stays within
// the base directory.
func (h *ServeHandler) serveFile(c echo.Context, baseDir, requestedPath string) error {
	// Clean the requested path to prevent traversal.
	cleaned := filepath.Clean(requestedPath)

	// Reject any path that tries to escape the base directory.
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		slog.Warn("package serve: path traversal attempt blocked",
			slog.String("path", requestedPath),
			slog.String("ip", c.RealIP()),
		)
		return c.NoContent(http.StatusBadRequest)
	}

	fullPath := filepath.Join(baseDir, cleaned)

	// Double-check: resolved path must be under the base directory.
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		slog.Warn("package serve: resolved path outside base directory",
			slog.String("resolved", absPath),
			slog.String("base", absBase),
			slog.String("ip", c.RealIP()),
		)
		return c.NoContent(http.StatusBadRequest)
	}

	// Use Lstat (not Stat) so we can detect symlinks before following them.
	info, err := os.Lstat(fullPath)
	if err != nil || info.IsDir() {
		return c.NoContent(http.StatusNotFound)
	}

	// Reject symlinks to prevent escaping the install directory.
	if info.Mode()&os.ModeSymlink != 0 {
		return c.NoContent(http.StatusForbidden)
	}

	// Set permissive CORS for external VTT clients.
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Cache-Control", "public, max-age=3600")

	return c.File(fullPath)
}
