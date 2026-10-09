// import_zip_media_test.go pins how an uploaded ZIP's media reaches the
// media importer, and what the import form promises about pictures.
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

// recordingMediaImporter reads every manifest row from the bundle it is
// handed and pretends each read file was restored under a new id.
type recordingMediaImporter struct {
	got    map[string]string // original id -> bytes read
	bundle *ImportMediaBundle
}

func (r *recordingMediaImporter) ImportMedia(_ context.Context, _, _ string, files []ExportMediaFile, bundle *ImportMediaBundle, idMap *IDMap, _ *ImportReport) error {
	r.bundle = bundle
	r.got = map[string]string{}
	for _, f := range files {
		data, ok, err := bundle.Read(f)
		if ok && err == nil {
			r.got[f.OriginalID] = string(data)
			idMap.MediaIDs[f.OriginalID] = "restored-" + f.OriginalID
		}
	}
	return nil
}

// TestImportCampaign_ZipMediaReachesTheImporter: the zip's media/ entries are
// handed to the media importer, paired with their manifest rows by Filename,
// and a manifest from before Filename existed is paired by the file's id.
func TestImportCampaign_ZipMediaReachesTheImporter(t *testing.T) {
	const withName = "11111111-1111-1111-1111-111111111111"
	const legacy = "22222222-2222-2222-2222-222222222222"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	raw, _ := json.Marshal(&CampaignExport{
		Format: ExportFormat, Version: ExportVersion,
		Campaign: ExportCampaignMeta{Name: "Test Campaign"},
		Media: []ExportMediaFile{
			{OriginalID: withName, Filename: withName + ".png", MimeType: "image/png"},
			{OriginalID: legacy, MimeType: "image/png"},
		},
	})
	for name, body := range map[string]string{
		"campaign.json":              string(raw),
		"media/" + withName + ".png": "NAMED-BYTES",
		"media/" + legacy + ".png":   "LEGACY-BYTES",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	svc := NewExportImportService(importStubCampaignSvc{})
	rec := &recordingMediaImporter{}
	svc.SetMediaImporter(rec)
	resp := postImportBlob(t, NewExportHandler(svc), "campaign.zip", buf.Bytes())

	if rec.got[withName] != "NAMED-BYTES" || rec.got[legacy] != "LEGACY-BYTES" {
		t.Errorf("importer read %v, want both files' bytes", rec.got)
	}
	if got := resp.Header().Get("HX-Redirect"); got != "/campaigns/new-campaign" {
		t.Errorf("a zip whose media all came back reported losses; HX-Redirect = %q, body:\n%s", got, resp.Body.String())
	}
}

// TestImportCampaign_JSONUploadHandsNoBundle: a JSON upload has no picture
// bytes, and the importer is told so (a nil bundle) rather than handed an
// empty one it could mistake for a zip that lost its files.
func TestImportCampaign_JSONUploadHandsNoBundle(t *testing.T) {
	svc := NewExportImportService(importStubCampaignSvc{})
	rec := &recordingMediaImporter{}
	svc.SetMediaImporter(rec)
	env := minimalEnvelope()
	env.Media = []ExportMediaFile{{OriginalID: "33333333-3333-3333-3333-333333333333", MimeType: "image/png"}}
	postImport(t, NewExportHandler(svc), env)
	if rec.bundle != nil {
		t.Error("a JSON upload handed the media importer a bundle")
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

// TestImportForm_AcceptsZip pins the UI half: the form accepts the zip, and
// says a JSON file brings no pictures back.
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
	if !strings.Contains(html, "carries no pictures") {
		t.Error("the import form does not say a JSON file brings no pictures back")
	}
}

// TestImportReport_FailNCountsBatchOnce pins the batch record used for media:
// one detail row, an exact count, and a plural summary. Without it a
// 300-file archive would bury every interesting failure under 300 identical
// lines and blow past the detail cap.
func TestImportReport_FailNCountsBatchOnce(t *testing.T) {
	r := NewImportReport()
	r.Fail("notes", "note", "Party Loot", "database is away")
	r.FailN("media", "media file", "", "not in the zip", 12)

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
