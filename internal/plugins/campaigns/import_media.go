// Package campaigns — import_media.go restores the picture files a ZIP export
// carries and points every reference in the file at the restored copies.
package campaigns

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

// maxImportMediaFileBytes bounds how much of one zip entry is read into
// memory. The media service applies the instance's real upload limit
// afterwards; this only stops a hostile entry from inflating without end.
const maxImportMediaFileBytes = 100 * 1024 * 1024

// maxImportMediaFiles bounds the media files one import may carry, counted
// both as zip entries and as manifest rows. Each file is a full upload, so an
// export far past this is not one Chronicle wrote for a real campaign.
const maxImportMediaFiles = 5000

// MediaImporter restores media files from an import, recording each
// original id's new id on idMap.MediaIDs. bundle is nil for a JSON-only
// upload, which carries no file bytes.
type MediaImporter interface {
	ImportMedia(ctx context.Context, campaignID, userID string, files []ExportMediaFile, bundle *ImportMediaBundle, idMap *IDMap, report *ImportReport) error
}

// ImportMediaBundle is the media/ entries of an uploaded ZIP export.
type ImportMediaBundle struct {
	byName map[string]*zip.File
	// byStem is byName keyed without the extension, for manifests written
	// before rows carried a Filename and are matched by the file's id.
	byStem map[string]*zip.File
}

// NewImportMediaBundle indexes a zip's media/ entries by their basename.
// Entries in sub-folders or with unsafe names are ignored; more media entries
// than an import may carry is refused.
func NewImportMediaBundle(r *zip.Reader) (*ImportMediaBundle, error) {
	b := &ImportMediaBundle{byName: make(map[string]*zip.File), byStem: make(map[string]*zip.File)}
	for _, f := range r.File {
		name, ok := strings.CutPrefix(f.Name, "media/")
		if !ok || name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
			continue
		}
		if len(b.byName) >= maxImportMediaFiles {
			return nil, fmt.Errorf("the zip holds more than %d media files", maxImportMediaFiles)
		}
		b.byName[name] = f
		stem := strings.TrimSuffix(name, path.Ext(name))
		if _, taken := b.byStem[stem]; !taken {
			b.byStem[stem] = f
		}
	}
	return b, nil
}

// Len is the number of media entries in the zip.
func (b *ImportMediaBundle) Len() int {
	if b == nil {
		return 0
	}
	return len(b.byName)
}

// Read returns the bytes of the zip entry for a manifest row: by its
// Filename, or for a manifest written before Filename existed, by the entry
// named after its OriginalID. ok is false when the zip has no such entry.
func (b *ImportMediaBundle) Read(f ExportMediaFile) (data []byte, ok bool, err error) {
	return b.ReadAtMost(f, maxImportMediaFileBytes)
}

// ReadAtMost is Read for a file that may be at most limit bytes, the
// instance's upload limit: a larger entry is refused before it is inflated,
// from the size the zip declares, and again while reading, in case the
// declaration lies.
func (b *ImportMediaBundle) ReadAtMost(f ExportMediaFile, limit int64) (data []byte, ok bool, err error) {
	if b == nil {
		return nil, false, nil
	}
	if limit <= 0 || limit > maxImportMediaFileBytes {
		limit = maxImportMediaFileBytes
	}
	entry := b.byName[f.Filename]
	if entry == nil && f.OriginalID != "" {
		entry = b.byStem[f.OriginalID]
	}
	if entry == nil {
		return nil, false, nil
	}
	tooLarge := fmt.Errorf("%s is larger than the %d MB upload limit", entry.Name, limit/(1024*1024))
	if entry.UncompressedSize64 > uint64(limit) {
		return nil, true, tooLarge
	}
	rc, err := entry.Open()
	if err != nil {
		return nil, true, fmt.Errorf("open %s: %w", entry.Name, err)
	}
	defer func() { _ = rc.Close() }()
	data, err = io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, true, fmt.Errorf("read %s: %w", entry.Name, err)
	}
	if int64(len(data)) > limit {
		return nil, true, tooLarge
	}
	return data, true, nil
}

// MediaZipName is the media/ entry name the export gives a stored file:
// its basename, since the stored name carries date folders ("2026/03/<id>.png").
func MediaZipName(storedFilename string) string {
	return path.Base(strings.ReplaceAll(storedFilename, `\`, "/"))
}

// mediaIDPattern is the shape of a media file id. Only ids of this shape are
// rewritten, so a malformed manifest row cannot turn a short string into a
// search-and-replace across the whole file.
var mediaIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// remapMediaReferences points every reference to a restored media file at
// its new id: page portraits and covers, map backgrounds, token pictures,
// and /media/<id> inside any text. Media ids are UUIDs, so replacing the id
// wherever it occurs in the file touches only references to that file, in
// every field that holds one, including any added after this was written.
// An id with no restored file is left as it is. The manifest itself is
// left alone.
func remapMediaReferences(data *CampaignExport, ids map[string]string) error {
	pairs := make([]string, 0, 2*len(ids))
	for oldID, newID := range ids {
		if oldID == newID || !mediaIDPattern.MatchString(oldID) {
			continue
		}
		pairs = append(pairs, oldID, newID)
	}
	if len(pairs) == 0 {
		return nil
	}

	manifest := data.Media
	data.Media = nil
	raw, err := json.Marshal(data)
	data.Media = manifest
	if err != nil {
		return fmt.Errorf("encode export for media remap: %w", err)
	}
	var remapped CampaignExport
	if err := json.Unmarshal([]byte(strings.NewReplacer(pairs...).Replace(string(raw))), &remapped); err != nil {
		return fmt.Errorf("decode export after media remap: %w", err)
	}
	remapped.Media = manifest
	*data = remapped
	return nil
}
