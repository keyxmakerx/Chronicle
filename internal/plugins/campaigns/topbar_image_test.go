package campaigns

// topbar_image_test.go pins TopbarImageSection: both states render with the
// correct HTMX swap wiring.

import (
	"context"
	"strings"
	"testing"
)

// TestTopbarImageSection_States pins both render states + their HTMX swap wiring.
func TestTopbarImageSection_States(t *testing.T) {
	t.Run("no image → upload dropzone posting to the topbar-image endpoint", func(t *testing.T) {
		var sb strings.Builder
		if err := TopbarImageSection("camp-1", "", "tok").Render(context.Background(), &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		html := sb.String()
		if !strings.Contains(html, `hx-post="/campaigns/camp-1/topbar-image"`) {
			t.Error("empty state must offer an hx-post upload to the topbar-image endpoint")
		}
		if !strings.Contains(html, `hx-target="#appearance-topbar-image-section"`) {
			t.Error("upload must swap the section in place (no reload)")
		}
		if !strings.Contains(html, `data-topbar-image-path=""`) {
			t.Error("empty state must carry an empty data-topbar-image-path for JS state sync")
		}
	})

	t.Run("image set → thumbnail + hx-delete remove", func(t *testing.T) {
		// The fixture matches what the upload path actually stores:
		// MediaUploader.UploadBackdrop returns MediaFile.Filename as
		// filepath.Join("2006/01", uuid+ext) — a value containing slashes.
		const stored = "2026/09/b7c17bb1-6563-462c-8b49-5b2e8bd57108.png"
		var sb strings.Builder
		if err := TopbarImageSection("camp-1", stored, "tok").Render(context.Background(), &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		html := sb.String()

		// /media/:id matches ONE path segment. A src carrying the stored
		// value verbatim cannot be routed by Echo and answers 404.
		if strings.Contains(html, "/media/"+stored) {
			t.Errorf("src renders the raw stored path %q under /media/, which Echo's single-segment /media/:id route cannot match — this is a 404", stored)
		}
		if !strings.Contains(html, `/media/b7c17bb1-6563-462c-8b49-5b2e8bd57108`) {
			t.Error("set state must render the image through MediaURL, which reduces the stored path to the media id")
		}
		if !strings.Contains(html, `hx-delete="/campaigns/camp-1/topbar-image"`) {
			t.Error("set state must offer an hx-delete remove")
		}
		// The data- attribute keeps the raw stored value: JS round-trips it
		// back to the server, which stores paths, not ids.
		if !strings.Contains(html, `data-topbar-image-path="`+stored+`"`) {
			t.Error("set state must carry the stored path in data-topbar-image-path for JS state sync")
		}
	})
}
