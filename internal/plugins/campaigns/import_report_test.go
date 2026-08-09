// import_report_test.go is the regression for the silent-partial-import bug
// (sweep R4 stage 16). Fix id: backend/import-silent-partial-success.
// Import has always been best-effort — a single bad row
// must not abandon a half-built campaign — but every skipped row went to
// slog.Warn only, and the handler redirected the operator to their new
// campaign as if nothing had been lost. A restore tool that reports success
// while dropping rows is worse than one that fails outright.
//
// Two halves are pinned here:
//   - the report COUNTS every dropped object and names how many of what
//     (TestImportReport_*),
//   - the import handler SURFACES a partial import instead of redirecting
//     (TestImportCampaign_PartialImportIsSurfaced), and still redirects on a
//     clean import (TestImportCampaign_CleanImportRedirects).
package campaigns

import (
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

func TestImportReport_CountsAndNamesFailures(t *testing.T) {
	r := NewImportReport()
	if r.HasFailures() {
		t.Fatal("fresh report already reports failures")
	}
	if got := r.Summary(); got != "" {
		t.Errorf("empty report Summary() = %q, want empty", got)
	}

	r.Fail("notes", "note", "Party Loot", "database is away")
	r.Fail("notes", "note", "Session 3", "database is away")
	r.Fail("notes", "note", "Session 4", "database is away")
	r.Fail("maps", "map marker", "The Inn", "map was not created")

	if got := r.Count(); got != 4 {
		t.Errorf("Count() = %d, want 4", got)
	}
	if got := r.Summary(); got != "3 notes, 1 map marker" {
		t.Errorf("Summary() = %q, want %q", got, "3 notes, 1 map marker")
	}
	if got := len(r.Failures()); got != 4 {
		t.Errorf("len(Failures()) = %d, want 4", got)
	}
	// The detail must carry enough for an operator to go looking.
	f := r.Failures()[0]
	if f.Section != "notes" || f.Kind != "note" || f.Name != "Party Loot" || f.Reason == "" {
		t.Errorf("first failure lost detail: %+v", f)
	}
}

// TestImportReport_TruncatesDetailNotCount pins that a pathological import
// keeps an exact count even once the per-row detail stops being retained.
// The count is what tells the operator whether to trust the restore.
func TestImportReport_TruncatesDetailNotCount(t *testing.T) {
	r := NewImportReport()
	const n = maxRecordedImportFailures + 25
	for i := 0; i < n; i++ {
		r.Fail("entities", "entity", "e", "boom")
	}
	if got := r.Count(); got != n {
		t.Errorf("Count() = %d, want %d — the count must stay exact past the detail cap", got, n)
	}
	if got := len(r.Failures()); got != maxRecordedImportFailures {
		t.Errorf("len(Failures()) = %d, want %d", got, maxRecordedImportFailures)
	}
	if got := r.Truncated(); got != 25 {
		t.Errorf("Truncated() = %d, want 25", got)
	}
	if !strings.Contains(r.Summary(), "and 25 more") {
		t.Errorf("Summary() = %q, want it to admit the 25 unlisted failures", r.Summary())
	}
}

// TestImportReport_NilSafe: adapters take *ImportReport, and a nil one must
// be an inert no-op rather than a panic that takes the import down.
func TestImportReport_NilSafe(t *testing.T) {
	var r *ImportReport
	r.Fail("notes", "note", "x", "y")
	if r.Count() != 0 || r.HasFailures() || r.Summary() != "" ||
		r.Failures() != nil || r.CountsByKind() != nil || r.Truncated() != 0 {
		t.Error("nil report is not inert")
	}
}

func TestImportReport_Pluralize(t *testing.T) {
	cases := []struct {
		kind string
		n    int
		want string
	}{
		{"note", 1, "note"},
		{"note", 2, "notes"},
		{"map marker", 3, "map markers"},
		{"class", 2, "classes"},
		{"entity", 2, "entities"},
		{"relay", 2, "relays"},
	}
	for _, c := range cases {
		if got := pluralize(c.kind, c.n); got != c.want {
			t.Errorf("pluralize(%q, %d) = %q, want %q", c.kind, c.n, got, c.want)
		}
	}
}

// --- Handler surfacing ---

// losingNoteImporter drops one note of every import and records it, standing
// in for any per-row failure inside a real adapter.
type losingNoteImporter struct{}

func (losingNoteImporter) ImportNotes(_ context.Context, _, _ string, data []ExportNote, _ *IDMap, report *ImportReport) error {
	for i, n := range data {
		if i == 0 {
			report.Fail("notes", "note", n.Title, "database is away")
		}
	}
	return nil
}

// importStubCampaignSvc is the slice of CampaignService that Import touches.
type importStubCampaignSvc struct {
	CampaignService
}

func (importStubCampaignSvc) Create(_ context.Context, _ string, in CreateCampaignInput) (*Campaign, error) {
	return &Campaign{ID: "new-campaign", Name: in.Name}, nil
}

// postImport drives ImportCampaign with the given envelope as an HTMX upload
// and returns the recorder.
func postImport(t *testing.T, h *ExportHandler, env *CampaignExport) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "campaign.json")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(raw); err != nil {
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
	// Matches auth.GetUserID's context key; the real middleware sets it.
	c.Set("auth_user_id", "user-1")

	if err := h.ImportCampaign(c); err != nil {
		t.Fatalf("ImportCampaign: %v", err)
	}
	return rec
}

func minimalEnvelope() *CampaignExport {
	return &CampaignExport{
		Format:   ExportFormat,
		Version:  ExportVersion,
		Campaign: ExportCampaignMeta{Name: "Test Campaign"},
		Notes: []ExportNote{
			{Title: "Party Loot"},
			{Title: "Session Recaps"},
		},
	}
}

// TestImportCampaign_PartialImportIsSurfaced is the regression: when the
// import drops objects, the operator must be told, in the response, how many
// of what were lost — not silently redirected to the new campaign.
func TestImportCampaign_PartialImportIsSurfaced(t *testing.T) {
	svc := NewExportImportService(importStubCampaignSvc{})
	svc.SetNoteImporter(losingNoteImporter{})
	rec := postImport(t, NewExportHandler(svc), minimalEnvelope())

	if got := rec.Header().Get("HX-Redirect"); got != "" {
		t.Fatalf("partial import redirected to %q — the loss was never surfaced", got)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Imported with losses", "1 note", "Party Loot", "database is away"} {
		if !strings.Contains(body, want) {
			t.Errorf("response never mentions %q; body:\n%s", want, body)
		}
	}
}

// TestImportCampaign_CleanImportRedirects keeps the happy path honest too: a
// loss-free import must NOT show the warning panel.
func TestImportCampaign_CleanImportRedirects(t *testing.T) {
	svc := NewExportImportService(importStubCampaignSvc{})
	rec := postImport(t, NewExportHandler(svc), minimalEnvelope())

	if got := rec.Header().Get("HX-Redirect"); got != "/campaigns/new-campaign" {
		t.Errorf("HX-Redirect = %q, want /campaigns/new-campaign", got)
	}
	if strings.Contains(rec.Body.String(), "Imported with losses") {
		t.Error("clean import showed the partial-import warning")
	}
}
