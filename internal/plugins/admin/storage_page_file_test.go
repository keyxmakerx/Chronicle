package admin

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// A page file is opened only through its page, so the storage page counts it
// and offers Delete, but links nowhere (the general media route answers 404).
func TestStorageFilesTab_PageFilesShowNoThumbnailOrLink(t *testing.T) {
	const id = "cccccccc-3333-4333-8333-cccccccccccc"
	data := StoragePageData{
		Stats: &media.StorageStats{},
		Files: []media.AdminMediaFile{{
			MediaFile: media.MediaFile{
				ID: id, UploadedBy: "u", OriginalName: "handout.png", MimeType: "image/png",
				FileSize: 2048, UsageType: media.UsagePageFile, CreatedAt: time.Now(),
			},
			UploaderName: "Ana",
		}},
		Page: 1, PerPage: 20, TotalFiles: 1,
	}
	var buf bytes.Buffer
	if err := storageFilesTab(data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, `src="/media/`+id) || strings.Contains(html, `href="/media/`+id) {
		t.Errorf("the page links or shows the page file:\n%s", html)
	}
	if !strings.Contains(html, "Page files") || !strings.Contains(html, "/admin/media/"+id) {
		t.Error("the row lost its label or its Delete button")
	}
}
