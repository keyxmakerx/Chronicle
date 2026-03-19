// campaign_modules.go manages per-campaign custom game systems.
// Campaign owners can upload ZIP files containing manifest.json + data/*.json
// to create custom reference content for their campaign. These modules use
// GenericSystem (no Go code needed) and are stored in the media directory.
package systems

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// maxSystemZipSize limits custom system ZIP uploads to 50 MB.
const maxSystemZipSize = 50 * 1024 * 1024

// maxDataFileSize limits individual JSON data files to 10 MB.
const maxDataFileSize = 10 * 1024 * 1024

// CampaignSystemManager manages custom game systems uploaded by
// campaign owners. Each campaign can have at most one custom system.
// Modules are stored on disk and loaded into memory as GenericSystem instances.
type CampaignSystemManager struct {
	mu       sync.RWMutex
	baseDir  string                    // Root storage dir (e.g., ./media/modules).
	modules  map[string]*GenericSystem // campaignID → loaded module instance.
	manifests map[string]*SystemManifest // campaignID → manifest (even if module failed to load).
}

// NewCampaignSystemManager creates a manager that stores custom systems
// under baseDir/<campaignID>/. Discovers and loads any existing modules.
func NewCampaignSystemManager(baseDir string) *CampaignSystemManager {
	mgr := &CampaignSystemManager{
		baseDir:   baseDir,
		modules:   make(map[string]*GenericSystem),
		manifests: make(map[string]*SystemManifest),
	}

	// Discover existing uploads on startup.
	mgr.discoverAll()

	return mgr
}

// GetSystem returns the custom system for a campaign, or nil if none.
func (m *CampaignSystemManager) GetSystem(campaignID string) System { 
	m.mu.RLock()
	defer m.mu.RUnlock()
	mod := m.modules[campaignID]
	if mod == nil {
		return nil
	}
	return mod
}

// GetManifest returns the custom system manifest for a campaign, or nil.
func (m *CampaignSystemManager) GetManifest(campaignID string) *SystemManifest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.manifests[campaignID]
}

// Dir returns the absolute directory path for a campaign's custom system,
// or empty string if no custom system is installed.
func (m *CampaignSystemManager) Dir(campaignID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.manifests[campaignID] == nil {
		return ""
	}
	return filepath.Join(m.baseDir, campaignID)
}

// Install extracts a ZIP file containing a custom game system,
// validates it, stores it on disk, and loads it into memory.
// Returns the parsed manifest on success.
func (m *CampaignSystemManager) Install(campaignID string, zipData io.ReaderAt, zipSize int64) (*SystemManifest, error) {
	if zipSize > maxSystemZipSize {
		return nil, fmt.Errorf("module ZIP exceeds maximum size of %d MB", maxSystemZipSize/(1024*1024))
	}

	zr, err := zip.NewReader(zipData, zipSize)
	if err != nil {
		return nil, fmt.Errorf("invalid ZIP file: %w", err)
	}

	// First pass: validate structure and find manifest.
	var manifestFile *zip.File
	var dataFiles []*zip.File
	var widgetFiles []*zip.File
	for _, f := range zr.File {
		// Security: reject path traversal.
		if strings.Contains(f.Name, "..") {
			return nil, fmt.Errorf("invalid file path: %s", f.Name)
		}
		// Skip directories.
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name == "manifest.json" {
			manifestFile = f
		} else if strings.HasPrefix(f.Name, "data/") && strings.HasSuffix(f.Name, ".json") {
			if f.UncompressedSize64 > maxDataFileSize {
				return nil, fmt.Errorf("data file %s exceeds maximum size of %d MB", f.Name, maxDataFileSize/(1024*1024))
			}
			dataFiles = append(dataFiles, f)
		} else if strings.HasPrefix(f.Name, "widgets/") && strings.HasSuffix(f.Name, ".js") {
			if f.UncompressedSize64 > maxDataFileSize {
				return nil, fmt.Errorf("widget file %s exceeds maximum size of %d MB", f.Name, maxDataFileSize/(1024*1024))
			}
			widgetFiles = append(widgetFiles, f)
		}
		// Ignore other files silently.
	}

	if manifestFile == nil {
		return nil, fmt.Errorf("ZIP must contain a manifest.json at the root")
	}
	if len(dataFiles) == 0 {
		return nil, fmt.Errorf("ZIP must contain at least one data/*.json file")
	}

	// Parse and validate manifest.
	manifest, err := m.readManifestFromZip(manifestFile)
	if err != nil {
		return nil, err
	}

	// Validate all data files parse as ReferenceItem arrays.
	for _, df := range dataFiles {
		if err := m.validateDataFile(df); err != nil {
			return nil, fmt.Errorf("invalid data file %s: %w", df.Name, err)
		}
	}

	// Prefix custom system ID to avoid collisions with built-in modules.
	if !strings.HasPrefix(manifest.ID, "custom-") {
		manifest.ID = "custom-" + manifest.ID
	}
	// Force status to available.
	manifest.Status = StatusAvailable

	// Remove any existing module for this campaign.
	m.removeFromDisk(campaignID)

	// Extract to disk.
	sysDir := m.campaignModuleDir(campaignID)
	if err := os.MkdirAll(filepath.Join(sysDir, "data"), 0o755); err != nil {
		return nil, fmt.Errorf("creating module directory: %w", err)
	}

	// Create widgets directory if widget files are present.
	if len(widgetFiles) > 0 {
		if err := os.MkdirAll(filepath.Join(sysDir, "widgets"), 0o755); err != nil {
			return nil, fmt.Errorf("creating widgets directory: %w", err)
		}
	}

	// Write modified manifest (with custom- prefix and available status).
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "manifest.json"), manifestBytes, 0o644); err != nil {
		return nil, fmt.Errorf("writing manifest: %w", err)
	}

	// Extract data files.
	for _, df := range dataFiles {
		destPath := filepath.Join(sysDir, df.Name)
		if err := m.extractFile(df, destPath); err != nil {
			_ = os.RemoveAll(sysDir)
			return nil, fmt.Errorf("extracting %s: %w", df.Name, err)
		}
	}

	// Extract widget JS files.
	for _, wf := range widgetFiles {
		destPath := filepath.Join(sysDir, wf.Name)
		if err := m.extractFile(wf, destPath); err != nil {
			_ = os.RemoveAll(sysDir)
			return nil, fmt.Errorf("extracting widget %s: %w", wf.Name, err)
		}
	}

	// Load into memory.
	mod, err := NewGenericSystem(manifest, filepath.Join(sysDir, "data"))
	if err != nil {
		_ = os.RemoveAll(sysDir)
		return nil, fmt.Errorf("loading module: %w", err)
	}

	m.mu.Lock()
	m.modules[campaignID] = mod
	m.manifests[campaignID] = manifest
	m.mu.Unlock()

	slog.Info("custom game system installed",
		slog.String("campaign_id", campaignID),
		slog.String("system_id", manifest.ID),
		slog.String("module_name", manifest.Name),
	)

	return manifest, nil
}

// Uninstall removes a campaign's custom game system from disk and memory.
func (m *CampaignSystemManager) Uninstall(campaignID string) error {
	m.mu.Lock()
	delete(m.modules, campaignID)
	delete(m.manifests, campaignID)
	m.mu.Unlock()

	m.removeFromDisk(campaignID)

	slog.Info("custom game system uninstalled",
		slog.String("campaign_id", campaignID),
	)
	return nil
}

// campaignModuleDir returns the storage path for a campaign's custom system.
func (m *CampaignSystemManager) campaignModuleDir(campaignID string) string {
	return filepath.Join(m.baseDir, campaignID)
}

// removeFromDisk deletes the campaign module directory.
func (m *CampaignSystemManager) removeFromDisk(campaignID string) {
	dir := m.campaignModuleDir(campaignID)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove custom system directory",
			slog.String("dir", dir),
			slog.String("error", err.Error()),
		)
	}
}

// discoverAll scans the base directory for existing custom systems.
func (m *CampaignSystemManager) discoverAll() {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		// Directory doesn't exist yet — no custom systems.
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		campaignID := entry.Name()
		sysDir := filepath.Join(m.baseDir, campaignID)
		manifestPath := filepath.Join(sysDir, "manifest.json")

		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			slog.Warn("skipping invalid custom system",
				slog.String("campaign_id", campaignID),
				slog.String("error", err.Error()),
			)
			continue
		}

		dataDir := filepath.Join(sysDir, "data")
		mod, err := NewGenericSystem(manifest, dataDir)
		if err != nil {
			slog.Warn("failed to load custom system",
				slog.String("campaign_id", campaignID),
				slog.String("error", err.Error()),
			)
			m.manifests[campaignID] = manifest
			continue
		}

		m.modules[campaignID] = mod
		m.manifests[campaignID] = manifest
		slog.Info("loaded custom game system",
			slog.String("campaign_id", campaignID),
			slog.String("system_id", manifest.ID),
		)
	}
}

// readManifestFromZip reads and validates a manifest from a ZIP entry.
func (m *CampaignSystemManager) readManifestFromZip(f *zip.File) (*SystemManifest, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening manifest: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(io.LimitReader(rc, 1024*1024)) // 1 MB limit.
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest SystemManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if err := ValidateManifest(&manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	if len(manifest.Categories) == 0 {
		return nil, fmt.Errorf("manifest must define at least one category")
	}

	return &manifest, nil
}

// validateDataFile checks that a ZIP data file contains valid JSON
// that can be parsed as a ReferenceItem array.
func (m *CampaignSystemManager) validateDataFile(f *zip.File) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(io.LimitReader(rc, maxDataFileSize))
	if err != nil {
		return err
	}

	var items []ReferenceItem
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("not a valid ReferenceItem array: %w", err)
	}

	if len(items) == 0 {
		return fmt.Errorf("data file is empty")
	}

	return nil
}

// extractFile writes a ZIP entry to disk.
func (m *CampaignSystemManager) extractFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, io.LimitReader(rc, maxDataFileSize))
	return err
}
