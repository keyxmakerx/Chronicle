package calendar

import (
	"io/fs"
	"testing"
)

// TestStaticAssetsFS_ContainsCalendarWidget pins the embed.FS contract:
// after NW-2.2 Chunk F's migration, calendar_widget.js MUST be served
// from the plugin's embedded static dir, not from central static/. This
// test catches a future contributor accidentally removing the embed
// directive or renaming the file.
//
// Per cordinator/decisions/2026-05-25-plugin-static-assets.md.
func TestStaticAssetsFS_ContainsCalendarWidget(t *testing.T) {
	f, err := StaticAssetsFS.Open("static/js/calendar_widget.js")
	if err != nil {
		t.Fatalf("expected embedded calendar_widget.js, got: %v", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("could not stat embedded file: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("embedded calendar_widget.js is empty; expected non-zero size")
	}
}

// TestStaticAssetsFS_RootIsStaticDir pins that the FS root contains
// the expected "static" directory. App.mountPluginStatic uses
// echo.MustSubFS(StaticAssetsFS, "static") to strip this prefix; if
// the dir name changes the URL mount breaks silently.
func TestStaticAssetsFS_RootIsStaticDir(t *testing.T) {
	entries, err := fs.ReadDir(StaticAssetsFS, ".")
	if err != nil {
		t.Fatalf("could not read FS root: %v", err)
	}
	foundStatic := false
	for _, e := range entries {
		if e.IsDir() && e.Name() == "static" {
			foundStatic = true
			break
		}
	}
	if !foundStatic {
		t.Errorf("StaticAssetsFS root does not contain 'static' dir; mount via MustSubFS will break")
	}
}
