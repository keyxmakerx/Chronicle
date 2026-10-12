package admin

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// A note picture belongs to the readers of its note. The storage page may
// count it, but must not show it or link to it.
func TestStorageFilesTab_NotePicturesShowNoThumbnailOrLink(t *testing.T) {
	const (
		noteID = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"
		pageID = "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb"
	)
	file := func(id, usage, name string) media.AdminMediaFile {
		return media.AdminMediaFile{
			MediaFile: media.MediaFile{
				ID: id, UploadedBy: "u", OriginalName: name, MimeType: "image/png",
				FileSize: 2048, UsageType: usage, CreatedAt: time.Now(),
			},
			UploaderName: "Ana",
		}
	}
	data := StoragePageData{
		Stats: &media.StorageStats{},
		Files: []media.AdminMediaFile{file(noteID, media.UsageNoteImage, "Note picture"), file(pageID, media.UsageEntityImage, "map.png")},
		Page:  1, PerPage: 20, TotalFiles: 2,
	}
	var buf bytes.Buffer
	if err := storageFilesTab(data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, `src="/media/`+noteID) || strings.Contains(html, `href="/media/`+noteID) {
		t.Errorf("the page links or shows the note picture:\n%s", html)
	}
	if !strings.Contains(html, noteID[:8]) || !strings.Contains(html, "Note pictures") {
		t.Error("the row is gone: disk accounting still needs it")
	}
	if !strings.Contains(html, pageID) || !strings.Contains(html, `href="/media/`+pageID) {
		t.Error("an ordinary picture lost its thumbnail or View link")
	}
}
