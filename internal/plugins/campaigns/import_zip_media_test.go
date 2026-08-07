// import_zip_media_test.go pins the ruling on "Export ZIP (with media)"
// (sweep R4 stage 17).
//
// Fix id: promises/export-zip-media-dropped.
//
// The export side was never broken: the zip really does contain campaign.json
// plus a media/ folder of real bytes. Two things made it read as a promise
// that round-trips:
//
//  1. The import form's accept attribute was ".json,application/json", so the
//     file picker would not even offer the .zip the operator had just been
//     told to make. The handler could parse a zip; the UI could not deliver
//     one.
//  2. Every media entry in an accepted zip was dropped with a single
//     slog.Info and the operator was redirected to a campaign whose images
//     were all broken, with nothing on screen saying so.
//
// The ruling taken here is "stop promising, book the rest by name" rather
// than "make it round-trip": restoring media correctly means remapping every
// old media ID across entity image_path, map image_id, token image_path and
// every /media/<id> in entry_html, and a half-done remap restores the files
// while leaving every image broken — a new quiet lie in place of the old.
// That work is booked whole as C-IMPORT-MEDIA-RESTORE.
//
// So what is pinned here is honesty: the zip is accepted, and its unrestored
// media is counted and named in the response.
package campaigns

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// buildImportZip wraps an envelope plus n fake media files into a bundle in
// the exact shape ExportCampaign produces.
func buildImportZip(t *testing.T, env *CampaignExport, mediaFiles int) []byte {
	t.Helper()
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("campaign.json")
	if err != nil {
		t.Fatalf("create campaign.json: %v", err)
	}
	if _, err := f.Write(raw); err != nil {
		t.Fatalf("write campaign.json: %v", err)
	}
	for i := 0; i < mediaFiles; i++ {
		e, err := zw.Create("media/" + string(rune('a'+i)) + ".png")
		if err != nil {
			t.Fatalf("create media entry: %v", err)
		}
		if _, err := e.Write([]byte("FAKE-IMAGE-BYTES")); err != nil {
			t.Fatalf("write media entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// postImportBlob uploads arbitrary bytes under the given filename.
func postImportBlob(t *testing.T, h *ExportHandler, filename string, blob []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(blob); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/campaigns/import", &body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("auth_user_id", "user-1")

	if err := h.ImportCampaign(c); err != nil {
		t.Fatalf("ImportCampaign: %v", err)
	}
	return rec
}

// TestImportCampaign_ZipMediaIsCountedAndNamed is the regression: importing a
// media-bearing zip must tell the operator how many media files did not come
// back, not redirect them to a campaign full of broken images.
func TestImportCampaign_ZipMediaIsCountedAndNamed(t *testing.T) {
	svc := NewExportImportService(importStubCampaignSvc{})
	blob := buildImportZip(t, &CampaignExport{
		Format:   ExportFormat,
		Version:  ExportVersion,
		Campaign: ExportCampaignMeta{Name: "Test Campaign"},
	}, 3)

	rec := postImportBlob(t, NewExportHandler(svc), "campaign.zip", blob)

	if got := rec.Header().Get("HX-Redirect"); got != "" {
		t.Fatalf("zip import with dropped media redirected to %q — the loss was never surfaced", got)
	}
	body := rec.Body.String()
	for _, want := range []string{"Imported with losses", "3 media files", "restore them by hand"} {
		if !strings.Contains(body, want) {
			t.Errorf("response never mentions %q; body:\n%s", want, body)
		}
	}
}

// TestImportCampaign_ZipWithoutMediaIsClean keeps the counter honest in the
// other direction: a zip carrying no media must not manufacture a warning.
func TestImportCampaign_ZipWithoutMediaIsClean(t *testing.T) {
	svc := NewExportImportService(importStubCampaignSvc{})
	blob := buildImportZip(t, &CampaignExport{
		Format:   ExportFormat,
		Version:  ExportVersion,
		Campaign: ExportCampaignMeta{Name: "Test Campaign"},
	}, 0)

	rec := postImportBlob(t, NewExportHandler(svc), "campaign.zip", blob)
	if got := rec.Header().Get("HX-Redirect"); got != "/campaigns/new-campaign" {
		t.Errorf("HX-Redirect = %q, want /campaigns/new-campaign", got)
	}
}

// TestImportForm_AcceptsZip pins the UI half. The handler has always been
// able to parse a zip; the form's accept attribute meant the operator could
// not hand it one, so "Export ZIP (with media)" round-tripped to nothing at
// all — not even the structural data.
func TestImportForm_AcceptsZip(t *testing.T) {
	var buf bytes.Buffer
	if err := ImportCampaignPage("csrf-token").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render import page: %v", err)
	}
	html := buf.String()

	accept := ""
	if i := strings.Index(html, "accept=\""); i >= 0 {
		rest := html[i+len("accept=\""):]
		if j := strings.Index(rest, "\""); j >= 0 {
			accept = rest[:j]
		}
	}
	if !strings.Contains(accept, ".zip") {
		t.Errorf("accept=%q does not offer .zip; the ZIP export cannot be uploaded at all", accept)
	}
	// And it must not over-promise what a zip import does.
	if !strings.Contains(html, "not") || !strings.Contains(html, "re-attached") {
		t.Error("the import form does not warn that media files are not re-attached")
	}
}

// TestImportReport_FailNCountsBatchOnce pins the batch record used for media:
// one detail row, an exact count, and a plural summary. Without it a
// 300-file archive would bury every interesting failure under 300 identical
// lines and blow past the detail cap.
func TestImportReport_FailNCountsBatchOnce(t *testing.T) {
	r := NewImportReport()
	r.Fail("notes", "note", "Party Loot", "database is away")
	r.FailN("media", "media file", "", "not re-attached", 12)

	if got := r.Count(); got != 13 {
		t.Errorf("Count() = %d, want 13", got)
	}
	if got := len(r.Failures()); got != 2 {
		t.Errorf("len(Failures()) = %d, want 2 — a batch must be one detail row", got)
	}
	if got := r.Summary(); got != "12 media files, 1 note" {
		t.Errorf("Summary() = %q, want %q", got, "12 media files, 1 note")
	}
	if r.FailN("media", "media file", "", "x", 0); r.Count() != 13 {
		t.Error("FailN with n=0 must be a no-op")
	}
}
