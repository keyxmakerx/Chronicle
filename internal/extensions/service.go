package extensions

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ExtensionService handles business logic for extension management.
type ExtensionService interface {
	// Admin operations (site-wide).
	Install(ctx context.Context, zipPath, userID string) (*Extension, error)
	Uninstall(ctx context.Context, extID string) error
	Update(ctx context.Context, zipPath, extID string) (*Extension, error)
	List(ctx context.Context) ([]Extension, error)
	GetByExtID(ctx context.Context, extID string) (*Extension, error)
	Rescan(ctx context.Context) (int, error)

	// Campaign operations.
	ListForCampaign(ctx context.Context, campaignID string) ([]CampaignExtension, error)
	EnableForCampaign(ctx context.Context, campaignID, extID, userID string) error
	DisableForCampaign(ctx context.Context, campaignID, extID string) error
	GetCampaignExtension(ctx context.Context, campaignID, extID string) (*CampaignExtension, error)
	PreviewContributes(ctx context.Context, extID string) (*ManifestContributes, error)

	// Provenance.
	RecordProvenance(ctx context.Context, campaignID, extensionID, tableName, recordID, recordType string) error
	ListProvenance(ctx context.Context, campaignID, extensionID string) ([]Provenance, error)

	// Extension data.
	SetData(ctx context.Context, campaignID, extensionID, namespace, key string, value json.RawMessage) error
	ListData(ctx context.Context, campaignID, extensionID, namespace string) ([]ExtensionData, error)

	// Configuration.
	SetApplier(applier ContentApplier)
}

// extensionService implements ExtensionService.
type extensionService struct {
	repo    ExtensionRepository
	extDir  string         // Root directory for extension files (e.g., "data/extensions").
	applier ContentApplier // Optional content applier for campaign enable.
}

// NewExtensionService creates a new extension service.
func NewExtensionService(repo ExtensionRepository, extDir string) ExtensionService {
	return &extensionService{
		repo:   repo,
		extDir: extDir,
	}
}

// SetApplier configures the content applier for applying extension content
// when enabled for a campaign. Called during app startup after all services
// are wired.
func (s *extensionService) SetApplier(applier ContentApplier) {
	s.applier = applier
}

// Install processes a zip upload: extracts, validates, and registers the extension.
func (s *extensionService) Install(ctx context.Context, zipPath, userID string) (*Extension, error) {
	// Extract to a temporary directory first.
	tmpDir, err := os.MkdirTemp("", "chronicle-ext-*")
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating temp dir: %w", err))
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manifest, err := ExtractZip(zipPath, tmpDir)
	if err != nil {
		return nil, apperror.NewBadRequest("invalid extension package: " + err.Error())
	}

	// Check for duplicate.
	existing, err := s.repo.FindByExtID(ctx, manifest.ID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("checking existing extension: %w", err))
	}
	if existing != nil {
		return nil, apperror.NewConflict(fmt.Sprintf("extension %q is already installed (version %s)", manifest.ID, existing.Version))
	}

	// Validate extracted content files (CSS, SVG safety).
	if err := ValidateExtractedFiles(tmpDir); err != nil {
		return nil, apperror.NewBadRequest("unsafe extension content: " + err.Error())
	}

	// Move extracted files to final location.
	destDir := filepath.Join(s.extDir, manifest.ID)
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating extension dir: %w", err))
	}
	if err := os.Rename(tmpDir, destDir); err != nil {
		// Rename may fail across filesystems; fall back to copy.
		if err := copyDir(tmpDir, destDir); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("moving extension files: %w", err))
		}
	}

	// Serialize manifest for storage.
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("serializing manifest: %w", err))
	}

	now := time.Now().UTC()
	ext := &Extension{
		ID:          generateUUID(),
		ExtID:       manifest.ID,
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Manifest:    manifestJSON,
		InstalledBy: userID,
		Status:      StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, ext); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("saving extension: %w", err))
	}

	slog.Info("extension installed",
		slog.String("ext_id", manifest.ID),
		slog.String("name", manifest.Name),
		slog.String("version", manifest.Version),
	)

	return ext, nil
}

// Uninstall removes an extension site-wide and cleans up files.
func (s *extensionService) Uninstall(ctx context.Context, extID string) error {
	ext, err := s.repo.FindByExtID(ctx, extID)
	if err != nil {
		return err
	}
	if ext == nil {
		return apperror.NewNotFound("extension not found")
	}

	// Delete from database (CASCADE handles campaign_extensions, provenance, data).
	if err := s.repo.Delete(ctx, ext.ID); err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting extension record: %w", err))
	}

	// Remove files from disk.
	extDir := filepath.Join(s.extDir, ext.ExtID)
	if err := os.RemoveAll(extDir); err != nil {
		slog.Warn("failed to remove extension files",
			slog.String("ext_id", ext.ExtID),
			slog.Any("error", err),
		)
	}

	slog.Info("extension uninstalled",
		slog.String("ext_id", ext.ExtID),
		slog.String("name", ext.Name),
	)

	return nil
}

// Update replaces an installed extension with a new version.
func (s *extensionService) Update(ctx context.Context, zipPath, extID string) (*Extension, error) {
	existing, err := s.repo.FindByExtID(ctx, extID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, apperror.NewNotFound("extension not found")
	}

	// Extract to temp.
	tmpDir, err := os.MkdirTemp("", "chronicle-ext-update-*")
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating temp dir: %w", err))
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manifest, err := ExtractZip(zipPath, tmpDir)
	if err != nil {
		return nil, apperror.NewBadRequest("invalid extension package: " + err.Error())
	}

	if manifest.ID != extID {
		return nil, apperror.NewBadRequest(fmt.Sprintf("manifest ID %q does not match extension %q", manifest.ID, extID))
	}

	if err := ValidateExtractedFiles(tmpDir); err != nil {
		return nil, apperror.NewBadRequest("unsafe extension content: " + err.Error())
	}

	// Replace files on disk.
	destDir := filepath.Join(s.extDir, manifest.ID)
	if err := os.RemoveAll(destDir); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("removing old extension files: %w", err))
	}
	if err := os.Rename(tmpDir, destDir); err != nil {
		if err := copyDir(tmpDir, destDir); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("moving extension files: %w", err))
		}
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("serializing manifest: %w", err))
	}

	if err := s.repo.UpdateVersion(ctx, existing.ID, manifest.Version, manifestJSON); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating extension version: %w", err))
	}

	existing.Version = manifest.Version
	existing.Manifest = manifestJSON

	slog.Info("extension updated",
		slog.String("ext_id", manifest.ID),
		slog.String("old_version", existing.Version),
		slog.String("new_version", manifest.Version),
	)

	return existing, nil
}

// List returns all installed extensions.
func (s *extensionService) List(ctx context.Context) ([]Extension, error) {
	return s.repo.List(ctx)
}

// GetByExtID returns an extension by manifest ID.
func (s *extensionService) GetByExtID(ctx context.Context, extID string) (*Extension, error) {
	ext, err := s.repo.FindByExtID(ctx, extID)
	if err != nil {
		return nil, err
	}
	if ext == nil {
		return nil, apperror.NewNotFound("extension not found")
	}
	return ext, nil
}

// Rescan discovers extensions from the filesystem that aren't in the database.
func (s *extensionService) Rescan(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(s.extDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, apperror.NewInternal(fmt.Errorf("reading extensions dir: %w", err))
	}

	discovered := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(s.extDir, entry.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // No manifest, skip.
		}

		manifest, err := ParseManifest(data)
		if err != nil {
			slog.Warn("skipping invalid extension",
				slog.String("dir", entry.Name()),
				slog.Any("error", err),
			)
			continue
		}

		// Check if already registered.
		existing, _ := s.repo.FindByExtID(ctx, manifest.ID)
		if existing != nil {
			continue
		}

		// Validate content files.
		extDir := filepath.Join(s.extDir, entry.Name())
		if err := ValidateExtractedFiles(extDir); err != nil {
			slog.Warn("skipping unsafe extension",
				slog.String("dir", entry.Name()),
				slog.Any("error", err),
			)
			continue
		}

		manifestJSON, _ := json.Marshal(manifest)
		now := time.Now().UTC()
		ext := &Extension{
			ID:          generateUUID(),
			ExtID:       manifest.ID,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Description: manifest.Description,
			Manifest:    manifestJSON,
			InstalledBy: "system",
			Status:      StatusActive,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := s.repo.Create(ctx, ext); err != nil {
			slog.Warn("failed to register discovered extension",
				slog.String("ext_id", manifest.ID),
				slog.Any("error", err),
			)
			continue
		}

		discovered++
		slog.Info("discovered extension from filesystem",
			slog.String("ext_id", manifest.ID),
			slog.String("name", manifest.Name),
		)
	}

	return discovered, nil
}

// --- Campaign Operations ---

// ListForCampaign returns all extensions with their enabled status for a campaign.
func (s *extensionService) ListForCampaign(ctx context.Context, campaignID string) ([]CampaignExtension, error) {
	return s.repo.ListForCampaign(ctx, campaignID)
}

// EnableForCampaign enables an extension for a campaign and applies its content.
func (s *extensionService) EnableForCampaign(ctx context.Context, campaignID, extID, userID string) error {
	ext, err := s.repo.FindByExtID(ctx, extID)
	if err != nil {
		return err
	}
	if ext == nil {
		return apperror.NewNotFound("extension not found")
	}
	if ext.Status != StatusActive {
		return apperror.NewBadRequest("extension is disabled")
	}

	now := time.Now().UTC()
	ce := &CampaignExtension{
		CampaignID:      campaignID,
		ExtensionID:     ext.ID,
		Enabled:         true,
		AppliedContents: json.RawMessage("{}"),
		EnabledAt:       now,
		EnabledBy:       &userID,
	}

	if err := s.repo.EnableForCampaign(ctx, ce); err != nil {
		return err
	}

	// Apply extension content to the campaign if an applier is configured.
	if s.applier != nil {
		var manifest ExtensionManifest
		if err := json.Unmarshal(ext.Manifest, &manifest); err != nil {
			slog.Warn("failed to parse manifest for content application",
				slog.String("ext_id", ext.ExtID),
				slog.Any("error", err),
			)
			return nil // Extension is enabled even if apply fails.
		}

		if err := s.applier.Apply(ctx, campaignID, ext, &manifest); err != nil {
			slog.Warn("content application had errors",
				slog.String("ext_id", ext.ExtID),
				slog.Any("error", err),
			)
			// Don't fail the enable — extension is enabled, content partially applied.
		}
	}

	return nil
}

// DisableForCampaign disables an extension for a campaign (keeps imported data).
func (s *extensionService) DisableForCampaign(ctx context.Context, campaignID, extID string) error {
	ext, err := s.repo.FindByExtID(ctx, extID)
	if err != nil {
		return err
	}
	if ext == nil {
		return apperror.NewNotFound("extension not found")
	}

	return s.repo.DisableForCampaign(ctx, campaignID, ext.ID)
}

// GetCampaignExtension returns a campaign's extension record.
func (s *extensionService) GetCampaignExtension(ctx context.Context, campaignID, extID string) (*CampaignExtension, error) {
	ext, err := s.repo.FindByExtID(ctx, extID)
	if err != nil {
		return nil, err
	}
	if ext == nil {
		return nil, apperror.NewNotFound("extension not found")
	}

	return s.repo.GetCampaignExtension(ctx, campaignID, ext.ID)
}

// PreviewContributes returns what an extension will contribute when enabled.
func (s *extensionService) PreviewContributes(ctx context.Context, extID string) (*ManifestContributes, error) {
	ext, err := s.repo.FindByExtID(ctx, extID)
	if err != nil {
		return nil, err
	}
	if ext == nil {
		return nil, apperror.NewNotFound("extension not found")
	}

	var manifest ExtensionManifest
	if err := json.Unmarshal(ext.Manifest, &manifest); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("parsing stored manifest: %w", err))
	}

	if manifest.Contributes == nil {
		return &ManifestContributes{}, nil
	}

	return manifest.Contributes, nil
}

// --- Provenance ---

// RecordProvenance tracks that an extension created a record.
func (s *extensionService) RecordProvenance(ctx context.Context, campaignID, extensionID, tableName, recordID, recordType string) error {
	return s.repo.CreateProvenance(ctx, &Provenance{
		CampaignID:  campaignID,
		ExtensionID: extensionID,
		TableName:   tableName,
		RecordID:    recordID,
		RecordType:  recordType,
		CreatedAt:   time.Now().UTC(),
	})
}

// ListProvenance returns provenance records for a campaign+extension.
func (s *extensionService) ListProvenance(ctx context.Context, campaignID, extensionID string) ([]Provenance, error) {
	return s.repo.ListProvenance(ctx, campaignID, extensionID)
}

// --- Extension Data ---

// SetData stores a key-value pair for an extension in a campaign.
func (s *extensionService) SetData(ctx context.Context, campaignID, extensionID, namespace, key string, value json.RawMessage) error {
	return s.repo.SetData(ctx, &ExtensionData{
		CampaignID:  campaignID,
		ExtensionID: extensionID,
		Namespace:   namespace,
		DataKey:     key,
		DataValue:   value,
	})
}

// ListData returns all data for an extension+namespace in a campaign.
func (s *extensionService) ListData(ctx context.Context, campaignID, extensionID, namespace string) ([]ExtensionData, error) {
	return s.repo.ListData(ctx, campaignID, extensionID, namespace)
}

// --- Helpers ---

// generateUUID creates a new v4 UUID.
func generateUUID() string {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
