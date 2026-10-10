package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	// Register decoders for image formats.
	_ "golang.org/x/image/webp"

	"github.com/google/uuid"
	"golang.org/x/image/draw"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// StorageLimiter resolves effective storage limits for quota enforcement at
// upload time. Implemented by the settings plugin via an adapter in routes.go.
// When nil, only the static maxSize check applies.
type StorageLimiter interface {
	GetEffectiveLimits(ctx context.Context, userID, campaignID string) (maxUploadSize, maxTotalStorage int64, maxFiles int, err error)
}

// MediaService handles business logic for media file operations.
type MediaService interface {
	Upload(ctx context.Context, input UploadInput) (*MediaFile, error)
	GetByID(ctx context.Context, id string) (*MediaFile, error)
	Delete(ctx context.Context, id string) error
	FilePath(file *MediaFile) string
	ThumbnailPath(file *MediaFile, size string) string
	SetStorageLimiter(limiter StorageLimiter)

	// SetMemberChecker and SetEntityVisibilityFilter wire the ADR-058
	// decision 5 merge-eligibility machinery (see canMergeWithExisting) —
	// the same seams Handler wires for the decision 1 read-path check, but
	// wired onto the service since the merge decision is made deep inside
	// Upload, for every caller (not just /media/upload). Optional: nil
	// fails the merge closed rather than defaulting to "merge".
	SetMemberChecker(checker MemberChecker)
	SetEntityVisibilityFilter(f EntityVisibilityFilter)

	// ListCampaignMedia returns paginated media files for a campaign.
	ListCampaignMedia(ctx context.Context, campaignID string, page, perPage int) ([]MediaFile, int, error)

	// GetCampaignStats returns aggregate storage stats for a campaign.
	GetCampaignStats(ctx context.Context, campaignID string) (*CampaignMediaStats, error)

	// FindReferences returns entities that reference a media file.
	FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error)

	// DeleteCampaignMedia deletes a media file after verifying it belongs to the campaign.
	DeleteCampaignMedia(ctx context.Context, campaignID, mediaID string) error

	// DeleteCampaignFiles removes all media files belonging to a campaign from
	// both disk and database. Used during campaign deletion to prevent orphaned
	// files. Returns the number of files deleted.
	DeleteCampaignFiles(ctx context.Context, campaignID string) (int, error)

	// CleanupOrphans finds files on disk without a corresponding DB record and
	// deletes them. Returns the number of files removed.
	CleanupOrphans(ctx context.Context) (int, error)

	// BackfillContentHashes hashes the on-disk bytes of any media row
	// whose content_hash is NULL (legacy rows from before migration 26)
	// and persists the result. Iterates in batches to bound memory and
	// stops when the row count reaches zero. Returns the total number
	// of rows hashed. Safe to call repeatedly — rows that already have
	// a hash are skipped at the SELECT stage.
	BackfillContentHashes(ctx context.Context, batchSize int) (int, error)

	// ValidateMediaPath ensures the storage directory exists and is writable.
	ValidateMediaPath() error
}

// maxConcurrentUploadsPerUser limits simultaneous uploads per user to prevent
// resource exhaustion from parallel large file processing.
const maxConcurrentUploadsPerUser = 3

// minFreeDiskBytes is the minimum free disk space required after writing a file.
// Uploads are rejected if writing the file would leave less than this available.
const minFreeDiskBytes = 100 * 1024 * 1024 // 100 MB

// uploadSemaphore tracks concurrent uploads per user.
type uploadSemaphore struct {
	mu    sync.Mutex
	slots map[string]int
}

// acquire increments the user's active upload count and returns true, or
// returns false if the user has reached the concurrency limit.
func (s *uploadSemaphore) acquire(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slots[userID] >= maxConcurrentUploadsPerUser {
		return false
	}
	s.slots[userID]++
	return true
}

// release decrements the user's active upload count.
func (s *uploadSemaphore) release(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slots[userID] > 0 {
		s.slots[userID]--
	}
	if s.slots[userID] == 0 {
		delete(s.slots, userID)
	}
}

// mediaService implements MediaService.
type mediaService struct {
	repo      MediaRepository
	mediaPath string         // Root directory for file storage.
	maxSize   int64          // Maximum file size in bytes (static fallback).
	limiter   StorageLimiter // Dynamic storage limits from settings plugin. May be nil.
	sem       *uploadSemaphore

	// memberChecker and entityVisibility back the ADR-058 decision 5 merge
	// rule (canMergeWithExisting), the same seams Handler holds for the
	// decision 1 read-path check. Both optional: nil makes
	// canMergeWithExisting fail closed rather than assume "merge".
	memberChecker    MemberChecker
	entityVisibility EntityVisibilityFilter
}

// NewMediaService creates a new media service.
func NewMediaService(repo MediaRepository, mediaPath string, maxSize int64) MediaService {
	return &mediaService{
		repo:      repo,
		mediaPath: mediaPath,
		maxSize:   maxSize,
		sem:       &uploadSemaphore{slots: make(map[string]int)},
	}
}

// SetStorageLimiter sets the dynamic storage limiter for quota enforcement.
// Called after all plugins are wired to avoid initialization order issues.
func (s *mediaService) SetStorageLimiter(limiter StorageLimiter) {
	s.limiter = limiter
}

// SetMemberChecker and SetEntityVisibilityFilter wire the ADR-058 decision 5
// merge-eligibility machinery, using the same adapter instances passed to
// Handler's identically named setters.
func (s *mediaService) SetMemberChecker(checker MemberChecker) {
	s.memberChecker = checker
}

func (s *mediaService) SetEntityVisibilityFilter(f EntityVisibilityFilter) {
	s.entityVisibility = f
}

// ValidateMediaPath ensures the media storage directory exists and is writable.
// Call at startup to surface configuration issues early instead of failing on
// the first upload with a 500 error.
func (s *mediaService) ValidateMediaPath() error {
	if err := os.MkdirAll(s.mediaPath, 0750); err != nil {
		return fmt.Errorf("media path %q: cannot create directory: %w", s.mediaPath, err)
	}
	// Write and remove a test file to verify write permissions.
	testFile := filepath.Join(s.mediaPath, ".write-test")
	if err := os.WriteFile(testFile, []byte("ok"), 0640); err != nil {
		return fmt.Errorf("media path %q: not writable: %w", s.mediaPath, err)
	}
	_ = os.Remove(testFile)
	slog.Info("media storage path validated", slog.String("path", s.mediaPath))
	return nil
}

// Upload validates, stores, and records a new media file.
func (s *mediaService) Upload(ctx context.Context, input UploadInput) (*MediaFile, error) {
	// Limit concurrent uploads per user to prevent resource exhaustion.
	if !s.sem.acquire(input.UploadedBy) {
		return nil, apperror.NewBadRequest("too many concurrent uploads; please wait and try again")
	}
	defer s.sem.release(input.UploadedBy)

	// Validate MIME type.
	if !AllowedMimeTypes[input.MimeType] {
		return nil, apperror.NewBadRequest("unsupported file type: " + input.MimeType)
	}

	// A note picture is served only to readers of a note that holds it, so it
	// needs a campaign to scope that to, and must be a picture.
	if input.UsageType == UsageNoteImage && (input.CampaignID == "" || !strings.HasPrefix(input.MimeType, "image/")) {
		return nil, apperror.NewBadRequest("a note picture must be an image in a campaign")
	}

	// Per-campaign dedup: hash the original bytes (before sanitization
	// re-encodes them, which is non-deterministic). A hash already used in
	// this campaign returns that record instead of writing a duplicate.
	// Skipped when there's no campaign context (avatars, backdrops,
	// system-level files) — the dedup index is keyed on
	// (campaign_id, content_hash), and each campaign owns its own media space.
	contentHash := ""
	if len(input.FileBytes) > 0 {
		sum := sha256.Sum256(input.FileBytes)
		contentHash = hex.EncodeToString(sum[:])
		// Note pictures are never merged: each upload is its own file, so who
		// may open it never depends on who else uploaded the same bytes.
		if input.CampaignID != "" && input.UsageType != UsageNoteImage {
			if existing, err := s.repo.FindByContentHash(ctx, input.CampaignID, contentHash); err != nil {
				// Non-fatal — continue with a fresh upload rather than
				// blocking the user on a transient DB hiccup.
				slog.Warn("media dedup: lookup failed, proceeding with new upload",
					slog.String("campaign_id", input.CampaignID),
					slog.Any("error", err),
				)
			} else if existing != nil {
				// ADR-058 decision 5: a content-hash match is not
				// automatically safe to merge. The new upload has no
				// destination page yet, so merging is safe only if the
				// uploader can already see every page that references the
				// matched file — otherwise a hidden page's artwork becomes
				// reachable the moment they attach their "new" file to a
				// visible page.
				mergeable, refs, mergeErr := s.canMergeWithExisting(ctx, input.CampaignID, existing.ID, input.UploadedBy)
				switch {
				case mergeErr != nil:
					// Fail closed: an uncomputable answer falls through to
					// a fresh upload exactly like a genuine "no" — the two
					// must behave identically from the uploader's side.
					slog.Warn("media dedup: merge-eligibility check failed, refusing merge",
						slog.String("campaign_id", input.CampaignID),
						slog.String("existing_id", existing.ID),
						slog.Any("error", mergeErr),
					)
				case mergeable:
					slog.Info("media dedup: returning existing file for duplicate upload",
						slog.String("campaign_id", input.CampaignID),
						slog.String("existing_id", existing.ID),
						slog.String("hash", contentHash),
					)
					// Safe to hand back the "where is this used" refs in
					// full, unfiltered: mergeable already proved the
					// uploader can see every one of these pages.
					existing.MatchedExisting = true
					existing.UsedBy = refs
					return existing, nil
				default:
					// Not mergeable: the uploader cannot see at least one
					// page already using this content. Do not merge, and
					// say nothing here or in the response that
					// distinguishes this from an ordinary upload —
					// confirming a match would let the uploader fingerprint
					// a hidden page's artwork (ADR-055 rule 3). Fall
					// through to store a genuinely separate row.
					slog.Info("media dedup: merge refused, uploader cannot see every referencing page; storing a separate file",
						slog.String("campaign_id", input.CampaignID),
						slog.String("existing_id", existing.ID),
						slog.String("uploader_id", input.UploadedBy),
					)
				}
			}
		}
	}

	// Validate file size. When the dynamic limiter is wired, its live
	// setting is the source of truth (checkQuotas below enforces it) —
	// skip the static check here so the env-var ceiling can't silently
	// override the admin panel's saved limit. Tests without a limiter
	// still get the static fallback.
	if s.limiter == nil {
		maxUpload := s.maxSize
		if input.FileSize > maxUpload {
			return nil, apperror.NewBadRequest(fmt.Sprintf("file too large; maximum size is %d MB", maxUpload/(1024*1024)))
		}
	}

	// Enforce dynamic storage limits from site settings if available.
	if s.limiter != nil {
		if err := s.checkQuotas(ctx, input); err != nil {
			return nil, err
		}
	}

	// Runs with or without a limiter: the cap is what keeps one member from
	// holding the campaign's space in pictures nobody else can remove.
	if input.UsageType == UsageNoteImage {
		if err := s.checkNotePictureCap(ctx, input); err != nil {
			return nil, err
		}
	}

	// Validate magic bytes match declared MIME type.
	if !validateMagicBytes(input.FileBytes, input.MimeType) {
		return nil, apperror.NewBadRequest("file content does not match declared type")
	}

	// Re-encode images to strip ALL metadata (EXIF, IPTC, XMP) and
	// destroy any polyglot payloads. The decode-then-encode pipeline
	// produces a clean file containing only pixel data (CDR approach).
	// Audio files are stored as-is (no re-encoding needed).
	if strings.HasPrefix(input.MimeType, "image/") {
		sanitizedBytes, effectiveMime, err := sanitizeImage(input.FileBytes, input.MimeType)
		if err != nil {
			return nil, apperror.NewBadRequest("image sanitization failed: " + err.Error())
		}
		input.FileBytes = sanitizedBytes
		input.FileSize = int64(len(sanitizedBytes))
		input.MimeType = effectiveMime
	}

	// Generate UUID filename in date-based directory.
	id := generateUUID()
	now := time.Now().UTC()
	dir := filepath.Join(s.mediaPath, now.Format("2006/01"))
	ext := MimeToExtension[input.MimeType]
	filename := id + ext

	// Create directory with restrictive permissions.
	if err := os.MkdirAll(dir, 0750); err != nil {
		slog.Error("failed to create media directory",
			slog.String("dir", dir),
			slog.String("campaign_id", input.CampaignID),
			slog.Any("error", err),
		)
		return nil, apperror.NewInternal(fmt.Errorf("creating media directory: %w", err))
	}

	// Verify sufficient disk space before writing to prevent filling the filesystem.
	if err := checkDiskSpace(dir, input.FileSize); err != nil {
		return nil, err
	}

	// Write sanitized file to disk with restrictive permissions.
	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, input.FileBytes, 0640); err != nil {
		slog.Error("failed to write media file to disk",
			slog.String("path", fullPath),
			slog.String("campaign_id", input.CampaignID),
			slog.String("mime_type", input.MimeType),
			slog.Int64("size", input.FileSize),
			slog.Any("error", err),
		)
		return nil, apperror.NewInternal(fmt.Errorf("writing media file: %w", err))
	}

	// Build file record.
	var campaignPtr *string
	if input.CampaignID != "" {
		campaignPtr = &input.CampaignID
	}

	file := &MediaFile{
		ID:           id,
		CampaignID:   campaignPtr,
		UploadedBy:   input.UploadedBy,
		Filename:     filepath.Join(now.Format("2006/01"), filename),
		OriginalName: input.OriginalName,
		MimeType:     input.MimeType,
		FileSize:     input.FileSize,
		// contentHash was computed above (sha256 of original bytes,
		// before sanitization). Persisting it here makes future dedup
		// lookups for this same content short-circuit immediately.
		ContentHash:    contentHash,
		UsageType:      input.UsageType,
		ThumbnailPaths: make(map[string]string),
		CreatedAt:      now,
	}

	// Generate thumbnails for images (using sanitized bytes).
	if file.IsImage() && input.MimeType != "image/gif" {
		thumbSizes := map[string]int{"300": 300, "800": 800}
		for sizeLabel, maxDim := range thumbSizes {
			thumbFilename, err := s.generateThumbnail(input.FileBytes, dir, id, ext, maxDim)
			if err != nil {
				slog.Warn("thumbnail generation failed",
					slog.String("file_id", id),
					slog.String("size", sizeLabel),
					slog.Any("error", err),
				)
				continue
			}
			file.ThumbnailPaths[sizeLabel] = filepath.Join(now.Format("2006/01"), thumbFilename)
		}
	}

	// Save to database.
	if err := s.repo.Create(ctx, file); err != nil {
		// Clean up all disk files (main + thumbnails) on DB failure.
		// Errors are intentionally ignored — cleanup is best-effort.
		_ = os.Remove(fullPath)
		for _, thumbFile := range file.ThumbnailPaths {
			_ = os.Remove(filepath.Join(s.mediaPath, thumbFile))
		}
		slog.Error("failed to save media record to database",
			slog.String("file_id", id),
			slog.String("campaign_id", input.CampaignID),
			slog.Any("error", err),
		)
		return nil, apperror.NewInternal(fmt.Errorf("saving media record: %w", err))
	}

	slog.Info("media file uploaded",
		slog.String("id", id),
		slog.String("mime_type", input.MimeType),
		slog.Int64("size", input.FileSize),
	)
	return file, nil
}

// canMergeWithExisting answers ADR-058 decision 5's merge question: may
// uploaderID's upload be merged into existingFileID, the row a content-hash
// match just found in campaignID? Returns the deduplicated reference list
// alongside the verdict, since a true verdict already proves the uploader
// can see every referencing page.
//
// A file no entity yet references is vacuously mergeable (decision 3).
//
// Fails closed: any error from FindReferences or the visibility filter is
// reported back as an error, refusing the merge exactly as on a genuine "no".
func (s *mediaService) canMergeWithExisting(ctx context.Context, campaignID, existingFileID, uploaderID string) (bool, []MediaRef, error) {
	refs, err := s.repo.FindReferences(ctx, campaignID, existingFileID)
	if err != nil {
		return false, nil, fmt.Errorf("finding references for merge check: %w", err)
	}
	if len(refs) == 0 {
		return true, refs, nil
	}
	if s.entityVisibility == nil {
		return false, nil, fmt.Errorf("media: entity visibility filter not configured for merge check")
	}

	// Dedup entity IDs the same way checkEntityScopedAccess does — a file
	// referenced twice by the same entity (e.g. as both image_path and
	// cover_image_path) must not be asked about twice.
	entityIDs := make([]string, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if seen[ref.EntityID] {
			continue
		}
		seen[ref.EntityID] = true
		entityIDs = append(entityIDs, ref.EntityID)
	}

	role := promotedVisibilityRole(s.memberChecker, campaignID, uploaderID)
	viewable, err := s.entityVisibility.FilterViewableEntityIDs(ctx, campaignID, entityIDs, role, uploaderID)
	if err != nil {
		return false, nil, fmt.Errorf("filtering viewable entities for merge check: %w", err)
	}

	// ALL, not "at least one" — decision 5 is the mirror of decision 1.
	// Decision 1 lets a viewer read a file if any one referencing page is
	// visible to them (a file already on screen can't be un-shown).
	// Decision 5 asks a different question: would merging teach the
	// uploader about a page they can't already see? That is only false when
	// EVERY referencing page is already visible to them.
	for _, id := range entityIDs {
		if !viewable[id] {
			return false, nil, nil
		}
	}
	return true, refs, nil
}

// defaultCampaignlessStorageBytes caps a user's total campaign-less uploads
// (avatars, and any /media/upload posted with a blank campaign_id) when no
// per-user override raises or lowers it. Campaign-scoped uploads get their
// quota from the campaign's own limits (checkQuotas below); campaign-less
// uploads have no campaign to scope a quota to, so without this a user could
// upload an unbounded number of avatar-sized files against the same shared
// disk-space floor every other upload competes for.
const defaultCampaignlessStorageBytes int64 = 25 * 1024 * 1024

// checkQuotas enforces dynamic storage limits from the settings plugin.
// Checks per-file size, campaign storage total, and campaign file count.
// Returns a user-facing error if any quota would be exceeded. A limit of 0
// means unlimited (no cap enforced).
func (s *mediaService) checkQuotas(ctx context.Context, input UploadInput) error {
	maxUpload, maxStorage, maxFiles, err := s.limiter.GetEffectiveLimits(ctx, input.UploadedBy, input.CampaignID)
	if err != nil {
		// Quota lookup failure should not block uploads -- log and allow.
		slog.Warn("failed to resolve storage limits, allowing upload",
			slog.String("user_id", input.UploadedBy),
			slog.String("campaign_id", input.CampaignID),
			slog.Any("error", err),
		)
		return nil
	}

	// Per-file size limit from settings (overrides static maxSize).
	if maxUpload > 0 && input.FileSize > maxUpload {
		return apperror.NewBadRequest(fmt.Sprintf("file too large; maximum size is %d MB", maxUpload/(1024*1024)))
	}

	// Campaign-less uploads have no campaign to check a storage quota
	// against, so they get their own per-user total instead.
	if input.CampaignID == "" {
		return s.checkCampaignlessQuota(ctx, input, maxStorage)
	}

	usedBytes, fileCount, err := s.repo.GetCampaignUsage(ctx, input.CampaignID)
	if err != nil {
		slog.Warn("failed to query campaign usage, allowing upload",
			slog.String("campaign_id", input.CampaignID),
			slog.Any("error", err),
		)
		return nil
	}

	if maxStorage > 0 && usedBytes+input.FileSize > maxStorage {
		return apperror.NewBadRequest("campaign storage quota exceeded")
	}
	if maxFiles > 0 && fileCount+1 > maxFiles {
		return apperror.NewBadRequest("campaign file count limit reached")
	}

	return nil
}

// Per-uploader share of a campaign for note pictures. Players can add these
// without a Scribe role and only the notes that hold them can remove them, so
// one member's total is bounded to leave room for everyone else.
const (
	// maxNotePictureBytesPerUser is the ceiling; a smaller campaign quota
	// lowers it to a quarter of that quota.
	maxNotePictureBytesPerUser int64 = 100 * 1024 * 1024
	maxNotePicturesPerUser           = 200
)

// notePictureByteCap is the byte cap for one uploader: the smaller of the
// ceiling and a quarter of the campaign's storage quota (0 = unlimited quota,
// so the ceiling alone).
func notePictureByteCap(campaignMaxStorage int64) int64 {
	limit := maxNotePictureBytesPerUser
	if campaignMaxStorage > 0 && campaignMaxStorage/4 < limit {
		limit = campaignMaxStorage / 4
	}
	return limit
}

// errNotePictureSpace is what the editor shows; it says what to do, not which
// number was hit.
const errNotePictureSpace = "You've used your space for note pictures in this campaign. Remove some pictures from your notes to add more."

// checkNotePictureCap refuses a note picture once the uploader holds their
// share of the campaign. Unlike checkQuotas it fails closed when usage cannot
// be read: this is the only bound on the pictures the owner cannot remove.
func (s *mediaService) checkNotePictureCap(ctx context.Context, input UploadInput) error {
	var campaignMax int64
	if s.limiter != nil {
		// A failed lookup leaves the fixed ceiling in force.
		if _, maxStorage, _, err := s.limiter.GetEffectiveLimits(ctx, input.UploadedBy, input.CampaignID); err == nil {
			campaignMax = maxStorage
		}
	}
	usedBytes, count, err := s.repo.GetUserNoteImageUsage(ctx, input.CampaignID, input.UploadedBy)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if usedBytes+input.FileSize > notePictureByteCap(campaignMax) || count+1 > maxNotePicturesPerUser {
		return apperror.NewBadRequest(errNotePictureSpace)
	}
	return nil
}

// checkCampaignlessQuota enforces a per-user total on uploads with no
// campaign_id (avatars, and any /media/upload posted with a blank
// campaign_id). maxStorage is the per-user override GetEffectiveLimits
// already resolved above; for this bucket, 0 there means "no override
// configured" and falls back to defaultCampaignlessStorageBytes rather than
// "unlimited" -- a campaign-less upload has no campaign quota to fall back
// on the way a campaign-scoped upload does.
func (s *mediaService) checkCampaignlessQuota(ctx context.Context, input UploadInput, maxStorage int64) error {
	limit := maxStorage
	if limit <= 0 {
		limit = defaultCampaignlessStorageBytes
	}

	usedBytes, _, err := s.repo.GetUserCampaignlessUsage(ctx, input.UploadedBy)
	if err != nil {
		slog.Warn("failed to query campaign-less storage usage, allowing upload",
			slog.String("user_id", input.UploadedBy),
			slog.Any("error", err),
		)
		return nil
	}

	if usedBytes+input.FileSize > limit {
		return apperror.NewBadRequest("storage quota exceeded")
	}
	return nil
}

// GetByID retrieves a media file by ID.
func (s *mediaService) GetByID(ctx context.Context, id string) (*MediaFile, error) {
	return s.repo.FindByID(ctx, id)
}

// Delete removes a media file from disk and database.
func (s *mediaService) Delete(ctx context.Context, id string) error {
	file, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete from database first.
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// Delete main file from disk. Errors are intentionally ignored —
	// orphaned files are preferable to failing a successful DB delete.
	mainPath := filepath.Join(s.mediaPath, file.Filename)
	_ = os.Remove(mainPath)

	// Delete thumbnails.
	for _, thumbFile := range file.ThumbnailPaths {
		_ = os.Remove(filepath.Join(s.mediaPath, thumbFile))
	}

	slog.Info("media file deleted", slog.String("id", id))
	return nil
}

// FilePath returns the absolute path to a media file on disk.
func (s *mediaService) FilePath(file *MediaFile) string {
	return filepath.Join(s.mediaPath, file.Filename)
}

// BackfillContentHashes hashes the on-disk bytes of any row with a NULL
// content_hash and writes the hash back, in batches so a campaign with
// thousands of legacy files isn't loaded at once. Per-file errors are
// logged and skipped so the run makes as much progress as possible.
func (s *mediaService) BackfillContentHashes(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 100
	}
	hashed := 0
	for {
		select {
		case <-ctx.Done():
			return hashed, ctx.Err()
		default:
		}
		batch, err := s.repo.ListMissingContentHash(ctx, batchSize)
		if err != nil {
			return hashed, fmt.Errorf("backfill list batch: %w", err)
		}
		if len(batch) == 0 {
			return hashed, nil
		}
		for _, f := range batch {
			path := s.FilePath(&f)
			data, err := os.ReadFile(path)
			if err != nil {
				slog.Warn("media backfill: cannot read file, skipping",
					slog.String("media_id", f.ID),
					slog.String("path", path),
					slog.Any("error", err),
				)
				// Mark with a sentinel (distinct from any real sha256) so
				// the next batch doesn't pick this row up forever; these
				// rows simply won't dedup against each other.
				_ = s.repo.SetContentHash(ctx, f.ID, "0000000000000000000000000000000000000000000000000000000missing0")
				continue
			}
			sum := sha256.Sum256(data)
			hash := hex.EncodeToString(sum[:])
			if err := s.repo.SetContentHash(ctx, f.ID, hash); err != nil {
				slog.Warn("media backfill: set hash failed",
					slog.String("media_id", f.ID),
					slog.Any("error", err),
				)
				continue
			}
			hashed++
		}
	}
}

// ThumbnailPath returns the absolute path to a thumbnail on disk.
func (s *mediaService) ThumbnailPath(file *MediaFile, size string) string {
	if thumbFile, ok := file.ThumbnailPaths[size]; ok {
		return filepath.Join(s.mediaPath, thumbFile)
	}
	return s.FilePath(file)
}

// ListCampaignMedia returns paginated media files for a campaign.
func (s *mediaService) ListCampaignMedia(ctx context.Context, campaignID string, page, perPage int) ([]MediaFile, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage
	return s.repo.ListByCampaign(ctx, campaignID, perPage, offset)
}

// GetCampaignStats returns aggregate storage stats for a campaign.
func (s *mediaService) GetCampaignStats(ctx context.Context, campaignID string) (*CampaignMediaStats, error) {
	totalBytes, fileCount, err := s.repo.GetCampaignUsage(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return &CampaignMediaStats{
		TotalFiles: fileCount,
		TotalBytes: totalBytes,
	}, nil
}

// FindReferences returns entities that reference a media file.
func (s *mediaService) FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
	return s.repo.FindReferences(ctx, campaignID, mediaID)
}

// DeleteCampaignMedia deletes a media file after verifying it belongs to the campaign.
func (s *mediaService) DeleteCampaignMedia(ctx context.Context, campaignID, mediaID string) error {
	file, err := s.repo.FindByID(ctx, mediaID)
	if err != nil {
		return err
	}

	// Verify the file belongs to this campaign. A note picture is not the
	// campaign owner's to remove: it belongs to the note that holds it.
	if file.CampaignID == nil || *file.CampaignID != campaignID || file.IsNotePicture() {
		return apperror.NewNotFound("media file not found")
	}

	return s.Delete(ctx, mediaID)
}

// DeleteCampaignFiles removes all media files belonging to a campaign.
// Deletes physical files from disk (main + thumbnails) and then removes
// the database records. Called before campaign SQL DELETE to prevent orphaned
// media. Errors on individual files are logged but do not abort the operation.
func (s *mediaService) DeleteCampaignFiles(ctx context.Context, campaignID string) (int, error) {
	files, err := s.repo.ListFilesByCampaign(ctx, campaignID)
	if err != nil {
		return 0, fmt.Errorf("listing campaign files: %w", err)
	}

	deleted := 0
	for _, f := range files {
		if err := s.Delete(ctx, f.ID); err != nil {
			slog.Warn("failed to delete media file during campaign cleanup",
				slog.String("file_id", f.ID),
				slog.String("campaign_id", campaignID),
				slog.Any("error", err),
			)
			continue
		}
		deleted++
	}

	slog.Info("campaign media cleanup completed",
		slog.String("campaign_id", campaignID),
		slog.Int("total", len(files)),
		slog.Int("deleted", deleted),
	)
	return deleted, nil
}

// notePictureGrace is how long an unbound note picture is kept, so a picture
// uploaded into a note still being written is not collected under its author.
const notePictureGrace = 24 * time.Hour

// CleanupOrphans walks the media directory, checks each file against the
// database, and deletes any files not tracked. This handles the case where
// an upload crashes between writing the file and saving the DB record.
func (s *mediaService) CleanupOrphans(ctx context.Context) (int, error) {
	// Note pictures no note is bound to were abandoned (never saved into a
	// note, or their note is gone); keeping them would let players fill the
	// campaign's quota with files nobody can reach. The delay covers a note
	// not yet saved. A listing error only skips this step.
	removedPictures := 0
	if ids, err := s.repo.ListUnboundNotePictures(ctx, time.Now().UTC().Add(-notePictureGrace)); err != nil {
		slog.Warn("orphan cleanup: could not list unbound note pictures", slog.Any("error", err))
	} else {
		for _, id := range ids {
			if err := s.Delete(ctx, id); err != nil {
				slog.Warn("orphan cleanup: could not delete unbound note picture", slog.String("file_id", id), slog.Any("error", err))
				continue
			}
			removedPictures++
		}
	}

	knownFiles, err := s.repo.ListAllFilenames(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing known files: %w", err)
	}

	removed := 0
	err = filepath.Walk(s.mediaPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip unreadable entries.
		}
		if info.IsDir() {
			return nil
		}

		// Defense in depth: refuse to remove a symlink in the media tree —
		// unexpected enough to leave for an operator to investigate rather
		// than delete.
		if info.Mode()&os.ModeSymlink != 0 {
			slog.Warn("skipping symlink in media directory",
				slog.String("path", path),
			)
			return nil
		}
		// Also skip anything that isn't a regular file (sockets, devices,
		// named pipes).
		if !info.Mode().IsRegular() {
			return nil
		}

		// Get the relative path from mediaPath for comparison with DB filenames.
		rel, relErr := filepath.Rel(s.mediaPath, path)
		if relErr != nil {
			return nil
		}

		if !knownFiles[rel] {
			// Grace period: skip files younger than 15 minutes to avoid
			// deleting files from in-progress uploads (TOCTOU race).
			if time.Since(info.ModTime()) < 15*time.Minute {
				slog.Debug("skipping recent orphan file",
					slog.String("path", rel),
					slog.Duration("age", time.Since(info.ModTime())),
				)
				return nil
			}

			if removeErr := os.Remove(path); removeErr == nil {
				removed++
				slog.Info("removed orphan media file", slog.String("path", rel))
			} else {
				slog.Warn("failed to remove orphan media file",
					slog.String("path", rel),
					slog.Any("error", removeErr),
				)
			}
		}
		return nil
	})
	if err != nil {
		return removed, fmt.Errorf("walking media directory: %w", err)
	}

	slog.Info("orphan cleanup completed", slog.Int("removed", removed), slog.Int("note_pictures", removedPictures))
	return removed + removedPictures, nil
}

// maxImageDimension is the maximum width or height in pixels for uploaded images.
// Images larger than this are rejected to prevent decompression bomb attacks
// (e.g., a tiny PNG that decompresses to gigabytes in memory).
const maxImageDimension = 10000

// generateThumbnail creates a resized copy of an image.
func (s *mediaService) generateThumbnail(data []byte, dir, id, ext string, maxDim int) (string, error) {
	// Check image dimensions before full decode to prevent decompression bombs.
	// DecodeConfig reads only the header, using minimal memory.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("reading image config: %w", err)
	}
	if cfg.Width > maxImageDimension || cfg.Height > maxImageDimension {
		return "", fmt.Errorf("image too large: %dx%d exceeds %d limit", cfg.Width, cfg.Height, maxImageDimension)
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("decoding image: %w", err)
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Skip if already small enough.
	if w <= maxDim && h <= maxDim {
		return "", fmt.Errorf("image already smaller than %d", maxDim)
	}

	// Calculate new dimensions maintaining aspect ratio.
	newW, newH := maxDim, maxDim
	if w > h {
		newH = h * maxDim / w
	} else {
		newW = w * maxDim / h
	}

	// Resize using Catmull-Rom interpolation.
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	// Write thumbnail.
	thumbFilename := fmt.Sprintf("%s_%d%s", id, maxDim, ext)
	thumbPath := filepath.Join(dir, thumbFilename)

	f, err := os.Create(thumbPath)
	if err != nil {
		return "", fmt.Errorf("creating thumbnail file: %w", err)
	}
	defer f.Close()

	switch ext {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(f, dst, &jpeg.Options{Quality: 85})
	case ".png":
		err = png.Encode(f, dst)
	case ".gif":
		err = gif.Encode(f, dst, nil)
	default:
		// For WebP and others, encode as JPEG thumbnail.
		err = jpeg.Encode(f, dst, &jpeg.Options{Quality: 85})
	}

	if err != nil {
		_ = os.Remove(thumbPath)
		return "", fmt.Errorf("encoding thumbnail: %w", err)
	}

	return thumbFilename, nil
}

// validateMagicBytes checks that the file content's magic bytes match the
// declared MIME type. Prevents uploading files with a spoofed Content-Type header.
func validateMagicBytes(data []byte, declaredMIME string) bool {
	if len(data) < 4 {
		return false
	}
	switch declaredMIME {
	case "image/jpeg":
		return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
	case "image/png":
		return len(data) >= 8 &&
			data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
			data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A
	case "image/gif":
		return len(data) >= 6 && string(data[:3]) == "GIF"
	case "image/webp":
		return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
	// Audio formats.
	case "audio/mpeg":
		// MP3: starts with 0xFF 0xFB/0xF3/0xF2 (frame sync) or "ID3" (ID3 tag).
		return (data[0] == 0xFF && (data[1]&0xE0) == 0xE0) || string(data[:3]) == "ID3"
	case "audio/ogg":
		return string(data[:4]) == "OggS"
	case "audio/wav":
		return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE"
	case "audio/webm":
		// WebM uses Matroska container: starts with EBML header 0x1A45DFA3.
		// That header alone doesn't distinguish audio from video, or WebM
		// from a raw .mkv — every Matroska-family file shares it — so a
		// narrow structural check decides the rest.
		if len(data) < 4 || data[0] != 0x1A || data[1] != 0x45 || data[2] != 0xDF || data[3] != 0xA3 {
			return false
		}
		return isAudioOnlyWebM(data)
	default:
		return false
	}
}

// Matroska/WebM element IDs needed to tell an audio-only WebM stream from
// one carrying video, or from a raw (non-WebM) Matroska file. IDs are
// written in their conventional form, length-marker bits included — see
// readEBMLID.
const (
	ebmlHeaderID     = 0x1A45DFA3 // EBML header (4-byte id)
	ebmlDocTypeID    = 0x4282     // EBML\DocType, e.g. "webm" or "matroska" (2-byte id)
	ebmlSegmentID    = 0x18538067 // Segment (4-byte id)
	ebmlTracksID     = 0x1654AE6B // Segment\Tracks (4-byte id)
	ebmlTrackEntryID = 0xAE       // Tracks\TrackEntry (1-byte id)
	ebmlTrackTypeID  = 0x83       // TrackEntry\TrackType (1-byte id)

	ebmlTrackTypeVideo = 1 // Matroska TrackType value for a video track
)

// ebmlMaxScanElements bounds how many EBML element headers
// isAudioOnlyWebM will read before giving up. Skipping an element's
// *content* is O(1) — the parser advances past its declared size rather
// than reading it — so this bounds worst-case work to a constant
// regardless of file size, while comfortably covering any real file:
// Tracks must precede the Clusters that depend on it, so it always
// appears within the first handful of top-level and Segment children.
const ebmlMaxScanElements = 10_000

// ebmlElement is one parsed EBML element header: its id and where its
// content lives in the buffer.
type ebmlElement struct {
	id          uint32
	contentPos  int
	contentSize uint64
	unknownSize bool // the reserved "all data bits set" size encoding
}

// ebmlVintLen returns the length (1-8) an EBML variable-length integer's
// leading byte encodes — the position of its highest set bit, counting
// from the top — or 0 for an invalid (all-zero) leading byte.
func ebmlVintLen(first byte) int {
	if first == 0 {
		return 0
	}
	length := 1
	for mask := byte(0x80); mask != 0 && first&mask == 0; mask >>= 1 {
		length++
	}
	if length > 8 {
		return 0
	}
	return length
}

// readEBMLID reads an element id at data[pos]. Unlike a size vint, an
// id's length-marker bits are kept as part of the value — that's why
// conventional Matroska id constants (e.g. Segment = 0x18538067) already
// include them.
func readEBMLID(data []byte, pos int) (id uint32, n int, ok bool) {
	if pos < 0 || pos >= len(data) {
		return 0, 0, false
	}
	length := ebmlVintLen(data[pos])
	if length == 0 || length > 4 || pos+length > len(data) {
		return 0, 0, false
	}
	for i := 0; i < length; i++ {
		id = id<<8 | uint32(data[pos+i])
	}
	return id, length, true
}

// readEBMLSize reads an element size vint at data[pos], with the
// length-marker bit stripped to get the actual value. unknown reports
// the reserved "all remaining bits set" encoding EBML permits for a
// still-being-written (unbounded) element.
func readEBMLSize(data []byte, pos int) (size uint64, unknown bool, n int, ok bool) {
	if pos < 0 || pos >= len(data) {
		return 0, false, 0, false
	}
	first := data[pos]
	length := ebmlVintLen(first)
	if length == 0 || length > 8 || pos+length > len(data) {
		return 0, false, 0, false
	}
	marker := byte(0x80) >> uint(length-1)
	value := uint64(first &^ marker)
	for i := 1; i < length; i++ {
		value = value<<8 | uint64(data[pos+i])
	}
	maxVal := uint64(1)<<(uint(length)*7) - 1
	return value, value == maxVal, length, true
}

// readEBMLElement reads one element header (id + size) at data[pos].
func readEBMLElement(data []byte, pos int) (el ebmlElement, ok bool) {
	id, idLen, ok1 := readEBMLID(data, pos)
	if !ok1 {
		return ebmlElement{}, false
	}
	size, unknown, sizeLen, ok2 := readEBMLSize(data, pos+idLen)
	if !ok2 {
		return ebmlElement{}, false
	}
	return ebmlElement{id: id, contentPos: pos + idLen + sizeLen, contentSize: size, unknownSize: unknown}, true
}

// ebmlChildEnd resolves el's content end within parentEnd: parentEnd
// itself when el's size is unknown (can't know its real end, so treat it
// as running to the edge of what we're allowed to look at) or when the
// declared size doesn't fit (truncated/malformed input), otherwise the
// declared end.
func ebmlChildEnd(el ebmlElement, parentEnd int) int {
	if el.unknownSize || el.contentSize > uint64(parentEnd-el.contentPos) {
		return parentEnd
	}
	return el.contentPos + int(el.contentSize)
}

// ebmlWalkChildren iterates sibling elements in [pos, end), calling visit
// for each. It stops when visit returns false, when an element's header
// is malformed or its size is unknown (unknown size means the next
// sibling's start can't be located, so the safe move is to stop rather
// than guess), or when budget is exhausted — whichever comes first.
// budget is shared across an entire isAudioOnlyWebM call (including
// nested calls), so total work stays bounded regardless of nesting depth.
//
// budgetExhausted reports specifically whether the walk stopped because
// *budget hit zero, as opposed to a clean finish or a malformed/refused
// element. A caller walking Tracks/TrackEntry — where running out of
// budget mid-scan could silently skip a disqualifying track — uses this
// to fail closed instead of trusting a partial result.
func ebmlWalkChildren(data []byte, pos, end int, budget *int, visit func(ebmlElement) bool) (budgetExhausted bool) {
	for pos < end {
		if *budget <= 0 {
			return true
		}
		*budget--
		el, ok := readEBMLElement(data, pos)
		if !ok || el.contentPos > end {
			return false
		}
		if !visit(el) {
			return false
		}
		if el.unknownSize || el.contentSize > uint64(end-el.contentPos) {
			return false
		}
		next := el.contentPos + int(el.contentSize)
		if next <= pos {
			return false
		}
		pos = next
	}
	return false
}

// ebmlReadASCII returns el's content as a string, bounded to parentEnd.
func ebmlReadASCII(data []byte, el ebmlElement, parentEnd int) string {
	end := ebmlChildEnd(el, parentEnd)
	if end <= el.contentPos || end > len(data) {
		return ""
	}
	return string(data[el.contentPos:end])
}

// ebmlReadUint returns el's content as a big-endian unsigned integer,
// bounded to parentEnd. Matroska encodes small integers (like TrackType)
// in the minimum number of bytes, always well under 8.
func ebmlReadUint(data []byte, el ebmlElement, parentEnd int) (uint64, bool) {
	end := ebmlChildEnd(el, parentEnd)
	if end <= el.contentPos || end > len(data) || end-el.contentPos > 8 {
		return 0, false
	}
	var v uint64
	for _, b := range data[el.contentPos:end] {
		v = v<<8 | uint64(b)
	}
	return v, true
}

// isAudioOnlyWebM does the narrow structural check validateMagicBytes
// needs to accept "audio/webm" only for a file that really is one: every
// track in Segment\Tracks must be audio (TrackType 2, never 1/video),
// and an EBML\DocType, if present, must say "webm" rather than
// "matroska" (a raw .mkv shares the exact same magic bytes). It does not
// decode any media — only the small header structure describing what's
// inside — so cost is bounded by ebmlMaxScanElements, not file size.
//
// A file whose Tracks element can't be found within that budget is
// rejected: this function only ever narrows what audio/webm accepts, so
// "can't tell" must fail closed rather than fall back to the old
// magic-bytes-only behavior.
//
// audioOnly starts true and only a video TrackEntry actually read flips
// it, so running out of budget partway through Tracks must never read as
// "scanned, all audio": padding can push a video entry past the budget.
// truncated latches on any budget exhaustion inside the Tracks walk and
// forces rejection.
func isAudioOnlyWebM(data []byte) bool {
	budget := ebmlMaxScanElements
	sawTracks := false
	sawTrackEntry := false
	audioOnly := true
	truncated := false
	docType := ""

	ebmlWalkChildren(data, 0, len(data), &budget, func(top ebmlElement) bool {
		switch top.id {
		case ebmlHeaderID:
			headerEnd := ebmlChildEnd(top, len(data))
			ebmlWalkChildren(data, top.contentPos, headerEnd, &budget, func(child ebmlElement) bool {
				if child.id == ebmlDocTypeID {
					docType = ebmlReadASCII(data, child, headerEnd)
				}
				return true
			})
		case ebmlSegmentID:
			segEnd := ebmlChildEnd(top, len(data))
			ebmlWalkChildren(data, top.contentPos, segEnd, &budget, func(segChild ebmlElement) bool {
				if segChild.id != ebmlTracksID {
					return true // keep looking for Tracks among Segment's children
				}
				sawTracks = true
				tracksEnd := ebmlChildEnd(segChild, segEnd)
				entriesExhausted := ebmlWalkChildren(data, segChild.contentPos, tracksEnd, &budget, func(entry ebmlElement) bool {
					if entry.id != ebmlTrackEntryID {
						return true
					}
					sawTrackEntry = true
					entryEnd := ebmlChildEnd(entry, tracksEnd)
					typeKnown := false
					fieldsExhausted := ebmlWalkChildren(data, entry.contentPos, entryEnd, &budget, func(field ebmlElement) bool {
						if field.id == ebmlTrackTypeID {
							if v, ok := ebmlReadUint(data, field, entryEnd); ok {
								typeKnown = true
								if v == ebmlTrackTypeVideo {
									audioOnly = false
								}
							}
						}
						return true
					})
					if fieldsExhausted {
						truncated = true
					}
					if !typeKnown {
						// TrackType is mandatory per spec; a TrackEntry
						// without a readable one means truncated or
						// malformed input, not a confirmed audio track —
						// fail closed rather than assume audio.
						audioOnly = false
					}
					return true
				})
				if entriesExhausted {
					// Budget ran out before every TrackEntry in Tracks was
					// visited — a later, unvisited TrackEntry could be
					// video. Not a confirmed audio-only file.
					truncated = true
				}
				return false // Tracks found; no need to keep scanning Segment
			})
		}
		return true
	})

	if !sawTracks || !sawTrackEntry || truncated {
		// A Tracks element that never actually enumerated a TrackEntry
		// (or a missing Tracks element at all) can't confirm the file is
		// audio-only — fail closed rather than trust the untouched
		// audioOnly default.
		return false
	}
	if docType != "" && docType != "webm" {
		return false
	}
	return audioOnly
}

// checkDiskSpace verifies that writing a file of the given size will leave at
// least minFreeDiskBytes of free space. Prevents media uploads from filling
// the filesystem and breaking other services.
func checkDiskSpace(path string, fileSize int64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		slog.Warn("disk space check failed, allowing upload",
			slog.String("path", path),
			slog.Any("error", err),
		)
		return nil // Don't block uploads if statfs fails.
	}
	available := int64(stat.Bavail) * int64(stat.Bsize)
	if available-fileSize < minFreeDiskBytes {
		return apperror.NewInternal(fmt.Errorf("insufficient disk space: %d bytes available, need %d + %d reserve",
			available, fileSize, minFreeDiskBytes))
	}
	return nil
}

// generateUUID returns a new random (v4) UUID string.
func generateUUID() string {
	return uuid.NewString()
}
