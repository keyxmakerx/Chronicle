package campaigns

// topbar_image_test.go — C-CUSTOMIZE-RESCUE B2/B4 pins for the topbar Image
// mode. Before this fix the "Image" mode button and its upload panel did not
// exist in the templ at all — appearance_editor.js injected them into the DOM
// at runtime and saved via a full-page reload (audit §8.2 "weird block thing";
// core-tenets §T-B3). These tests pin that the Image control is now first-class
// server-rendered markup, that TopbarImageSection renders both states with the
// correct HTMX swap wiring, and that a saved image reads back into the form.

import (
	"context"
	"strings"
	"testing"
)

func renderAppearanceTab(t *testing.T, settings string) string {
	t.Helper()
	cc := &CampaignContext{
		Campaign:   &Campaign{ID: "camp-1", Name: "Test Campaign", Settings: settings},
		MemberRole: RoleOwner,
	}
	var sb strings.Builder
	if err := appearanceTab(cc, "tok").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render appearanceTab: %v", err)
	}
	return sb.String()
}

// TestAppearanceTab_ImageModeIsFirstClass proves the Image button + upload panel
// are in the server-rendered markup, not injected by JS at runtime.
func TestAppearanceTab_ImageModeIsFirstClass(t *testing.T) {
	html := renderAppearanceTab(t, "")

	if !strings.Contains(html, `data-mode="image"`) {
		t.Error(`Top Bar Style card must render a first-class data-mode="image" button (was JS-injected before the rescue)`)
	}
	if !strings.Contains(html, `id="appearance-topbar-image"`) {
		t.Error("Top Bar Style card must render the #appearance-topbar-image panel")
	}
	if !strings.Contains(html, `id="appearance-topbar-image-section"`) {
		t.Error("the image panel must contain the TopbarImageSection swap target")
	}
}

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
		var sb strings.Builder
		if err := TopbarImageSection("camp-1", "bg.png", "tok").Render(context.Background(), &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		html := sb.String()
		if !strings.Contains(html, `/media/bg.png`) {
			t.Error("set state must render the current image thumbnail")
		}
		if !strings.Contains(html, `hx-delete="/campaigns/camp-1/topbar-image"`) {
			t.Error("set state must offer an hx-delete remove")
		}
		if !strings.Contains(html, `data-topbar-image-path="bg.png"`) {
			t.Error("set state must carry the image path in data-topbar-image-path for JS state sync")
		}
	})
}

// TestAppearanceTab_TopbarImageReadsBack proves a saved topbar image renders
// back into the form (the sweep's read-back check, item (c)).
func TestAppearanceTab_TopbarImageReadsBack(t *testing.T) {
	html := renderAppearanceTab(t, `{"topbar_style":{"mode":"image","image_path":"saved.png"}}`)
	if !strings.Contains(html, `/media/saved.png`) {
		t.Error("a saved topbar image must read back into the Image panel thumbnail")
	}
	if !strings.Contains(html, `data-topbar-image-path="saved.png"`) {
		t.Error("the saved image path must round-trip into data-topbar-image-path")
	}
}
