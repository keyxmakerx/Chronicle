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

// MediaImporter restores media files from an import, recording each
// original id's new id on idMap.MediaIDs. bundle is nil for a JSON-only
// upload, which carries no file bytes.
type MediaImporter interface {
	ImportMedia(ctx context.Context, campaignID, userID string, files []ExportMediaFile, bundle *ImportMediaBundle, idMap *IDMap, report *ImportReport) error
}

// ImportMediaBundle is the media/ entries of an uploaded ZIP export.
type ImportMediaBundle struct {
	byName map[string]*zip.File
}

// NewImportMediaBundle indexes a zip's media/ entries by their basename.
// Entries in sub-folders or with unsafe names are ignored.
func NewImportMediaBundle(r *zip.Reader) *ImportMediaBundle {
	b := &ImportMediaBundle{byName: make(map[string]*zip.File)}
	for _, f := range r.File {
		name, ok := strings.CutPrefix(f.Name, "media/")
		if !ok || name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
			continue
		}
		b.byName[name] = f
	}
	return b
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
	if b == nil {
		return nil, false, nil
	}
	entry := b.byName[f.Filename]
	if entry == nil && f.OriginalID != "" {
		for name, e := range b.byName {
			if strings.TrimSuffix(name, path.Ext(name)) == f.OriginalID {
				entry = e
				break
			}
		}
	}
	if entry == nil {
		return nil, false, nil
	}
	rc, err := entry.Open()
	if err != nil {
		return nil, true, fmt.Errorf("open %s: %w", entry.Name, err)
	}
	defer func() { _ = rc.Close() }()
	data, err = io.ReadAll(io.LimitReader(rc, maxImportMediaFileBytes+1))
	if err != nil {
		return nil, true, fmt.Errorf("read %s: %w", entry.Name, err)
	}
	if len(data) > maxImportMediaFileBytes {
		return nil, true, fmt.Errorf("%s is larger than %d MB", entry.Name, maxImportMediaFileBytes/(1024*1024))
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
