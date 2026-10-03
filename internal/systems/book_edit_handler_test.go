package systems

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// bookEditHarness serves the real book and editor routes over a fixture
// package installed as the campaign's own system, with a fake repository.
type bookEditHarness struct {
	e    *echo.Echo
	repo *fakeBookRepo
	dir  string
}

func newBookEditHarness(t *testing.T, files map[string]string) *bookEditHarness {
	t.Helper()
	if files == nil {
		files = testBookFiles()
	}
	base := writeBook(t, nil)
	sysDir := filepath.Join(base, testCampaign)
	for name, body := range files {
		p := filepath.Join(sysDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &SystemManifest{ID: "customds", Name: "Draw Steel", Widgets: []WidgetDef{{Slug: "rulebook-frontpage"}}}
	mgr := &CampaignSystemManager{
		baseDir:   base,
		modules:   map[string]*GenericSystem{testCampaign: {manifest: m}},
		manifests: map[string]*SystemManifest{testCampaign: m},
	}
	repo := &fakeBookRepo{}
	h := NewSystemHandler()
	h.SetCampaignSystems(mgr)
	h.SetBookEdits(newTestService(repo))

	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		_ = c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	mg := e.Group("/campaigns/:id/systems/:mod", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: c.Param("id"), Name: "Test"}}
			switch c.Request().Header.Get("X-Test-Role") {
			case "owner":
				cc.MemberRole = campaigns.RoleOwner
			case "dmgrant":
				cc.MemberRole, cc.IsDmGranted = campaigns.RolePlayer, true
			case "scribe":
				cc.MemberRole = campaigns.RoleScribe
			case "player":
				cc.MemberRole = campaigns.RolePlayer
			}
			c.Set("campaign_context", cc)
			return next(c)
		}
	})
	mg.GET("/book", h.BookAPI)
	registerBookEditorRoutes(mg, h)
	return &bookEditHarness{e: e, repo: repo, dir: sysDir}
}

func (h *bookEditHarness) do(role, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/campaigns/"+testCampaign+"/systems/customds"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HX-Request", "true") // the page route then renders only its fragment
	req.Header.Set("X-Test-Role", role)
	rec := httptest.NewRecorder()
	h.e.ServeHTTP(rec, req)
	return rec
}

const okPage = `{"page":{"title":"T","blocks":[{"text":"hello"}]}}`

// editorRoutes is every editor route with a body that would succeed for an editor.
var editorRoutes = []struct{ method, path, body string }{
	{"GET", "/book/edit", ""},
	{"GET", "/book/source", ""},
	{"GET", "/book/export", ""},
	{"POST", "/book/chapters", `{"title":"House"}`},
	{"PUT", "/book/chapters/house_00000001", `{"title":"Renamed"}`},
	{"DELETE", "/book/chapters/house_00000001", ""},
	{"POST", "/book/chapters/basics/pages", okPage},
	{"PUT", "/book/chapters/basics/pages/p0", okPage},
	{"DELETE", "/book/chapters/basics/pages/p0", ""},
	{"POST", "/book/chapters/basics/pages/p0/keep", ""},
}

// TestBookEditor_OnlyDirectorsMayEdit: players and scribes get 403 on every
// editor route, the page included, and nothing is written.
func TestBookEditor_OnlyDirectorsMayEdit(t *testing.T) {
	for _, role := range []string{"player", "scribe", "nobody"} {
		for _, r := range editorRoutes {
			t.Run(role+" "+r.method+" "+r.path, func(t *testing.T) {
				h := newBookEditHarness(t, nil)
				rec := h.do(role, r.method, r.path, r.body)
				if rec.Code != http.StatusForbidden {
					t.Errorf("status = %d, want 403: %s", rec.Code, rec.Body)
				}
				if h.repo.writes != 0 || len(h.repo.pages) != 0 || len(h.repo.chapters) != 0 {
					t.Error("a refused request wrote something")
				}
			})
		}
	}
}

// TestBookEditor_DirectorsCanEdit walks an owner and a co-Director through
// every editor route.
func TestBookEditor_DirectorsCanEdit(t *testing.T) {
	for _, role := range []string{"owner", "dmgrant"} {
		t.Run(role, func(t *testing.T) {
			h := newBookEditHarness(t, nil)
			want := func(rec *httptest.ResponseRecorder, code int) {
				t.Helper()
				if rec.Code != code {
					t.Fatalf("status = %d, want %d: %s", rec.Code, code, rec.Body)
				}
			}

			rec := h.do(role, "GET", "/book/edit", "")
			want(rec, 200)
			for _, s := range []string{`data-widget="rulebook-editor"`, `data-source-url="/campaigns/camp-1/systems/customds/book/source"`, `data-done-url="/campaigns/camp-1/systems/customds"`, "rulebook-editor.css", "rulebook.css", "data-rulebook-fullbleed"} {
				if !strings.Contains(rec.Body.String(), s) {
					t.Errorf("editor page is missing %s:\n%s", s, rec.Body)
				}
			}
			if strings.Contains(rec.Body.String(), "<script") {
				t.Errorf("editor page must not carry inline scripts:\n%s", rec.Body)
			}

			rec = h.do(role, "GET", "/book/source", "")
			want(rec, 200)
			var src BookEditorSource
			if err := json.Unmarshal(rec.Body.Bytes(), &src); err != nil {
				t.Fatal(err)
			}
			if src.SystemName != "Draw Steel" || src.ExportURL != "/campaigns/camp-1/systems/customds/book/export" || len(src.Widgets) != 1 || src.Parts[0].Chapters[0].Pages[0].Key != "p0" {
				t.Errorf("source = %+v", src)
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" {
				t.Errorf("source must not be cached: %q", rec.Header().Get("Cache-Control"))
			}

			rec = h.do(role, "POST", "/book/chapters", `{"title":"House"}`)
			want(rec, 200)
			var created struct{ Chapter BookChapterEntry }
			_ = json.Unmarshal(rec.Body.Bytes(), &created)
			id := created.Chapter.ID
			if !strings.HasPrefix(id, "house_") {
				t.Fatalf("created chapter = %s", rec.Body)
			}
			want(h.do(role, "PUT", "/book/chapters/"+id, `{"title":"Renamed"}`), 200)
			want(h.do(role, "POST", "/book/chapters/basics/pages", okPage), 200)
			want(h.do(role, "PUT", "/book/chapters/basics/pages/p0", okPage), 200)
			want(h.do(role, "POST", "/book/chapters/basics/pages/p0/keep", ""), 200)
			want(h.do(role, "GET", "/book/export", ""), 200)
			want(h.do(role, "DELETE", "/book/chapters/basics/pages/p0", ""), 200)
			want(h.do(role, "DELETE", "/book/chapters/"+id, ""), 204)

			// Bad input is a 422 and stores nothing more.
			writes := h.repo.writes
			rec = h.do(role, "PUT", "/book/chapters/basics/pages/p0", `{"page":{"blocks":[{"type":"roll","dice":"9d9999"}]}}`)
			want(rec, 422)
			if !strings.Contains(rec.Body.String(), "dice must look like 2d10") || h.repo.writes != writes {
				t.Errorf("422 body = %s, writes %d -> %d", rec.Body, writes, h.repo.writes)
			}
			want(h.do(role, "PUT", "/book/chapters/basics/pages/p0", `not json`), 400)
		})
	}
}

func TestBookEditor_RequestBodyIsCapped(t *testing.T) {
	h := newBookEditHarness(t, nil)
	big := `{"page":{"blocks":[{"text":"` + strings.Repeat("x", maxBookFileBytes) + `"}]}}`
	if rec := h.do("owner", "POST", "/book/chapters/basics/pages", big); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestBookAPI_MergesEditsAndReportsCanEdit: the reader's book is the merged
// edition, filtered afterwards, and only editors are told they can edit.
func TestBookAPI_MergesEditsAndReportsCanEdit(t *testing.T) {
	h := newBookEditHarness(t, nil)
	h.repo.pages = []StoredBookPage{
		{ID: 1, CampaignID: testCampaign, SystemID: "customds", ChapterID: "basics", SortOrder: 1, PageJSON: `{"title":"House page","blocks":[{"text":"table rule"}]}`},
		{ID: 2, CampaignID: testCampaign, SystemID: "customds", ChapterID: "basics", SortOrder: 2, PageJSON: `{"title":"DIR-ONLY-OWN","director":true,"blocks":[{"text":"DIR-SECRET"}]}`},
	}
	tests := []struct {
		role      string
		canEdit   bool
		wantInfo  []string
		wantNoInf []string
	}{
		{"player", false, []string{"House page", "table rule"}, []string{"DIR-", "SECRET"}},
		{"scribe", false, []string{"House page"}, []string{"DIR-", "SECRET"}},
		{"owner", true, []string{"House page", "DIR-SECRET"}, nil},
		{"dmgrant", true, []string{"House page", "DIR-SECRET"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			rec := h.do(tt.role, "GET", "/book", "")
			if rec.Code != 200 {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			var got struct {
				CanEdit bool `json:"canEdit"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &got)
			if got.CanEdit != tt.canEdit {
				t.Errorf("canEdit = %v, want %v", got.CanEdit, tt.canEdit)
			}
			for _, w := range tt.wantInfo {
				if !strings.Contains(rec.Body.String(), w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tt.wantNoInf {
				if strings.Contains(rec.Body.String(), w) {
					t.Errorf("leaks %q", w)
				}
			}
		})
	}
}

func TestBookEditorPage_RendersFullPage(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}}
	var buf bytes.Buffer
	err := BookEditorPage(cc, &SystemManifest{ID: "ds", Name: "Draw Steel"}, "/campaigns/camp-1/systems/ds").Render(context.Background(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `data-widget="rulebook-editor"`) {
		t.Errorf("no mount in:\n%s", buf.String())
	}
}

// unzipBook writes an export's files under a temp dir and returns it.
func unzipBook(t *testing.T, data []byte) (dir string, names []string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		p := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, names
}

// TestBookExport_RoundTripsThroughLoadBook: the exported zip is a package
// book/ folder, and loading it gives back the edition the campaign reads.
func TestBookExport_RoundTripsThroughLoadBook(t *testing.T) {
	files := testBookFiles()
	delete(files, "book/chapters/broken.yaml") // a broken chapter has no authored form to export
	files["book/book.yaml"] = strings.Replace(files["book/book.yaml"], "[basics, broken, secret-in-player-part]", "[basics, secret-in-player-part, house-table-rules]", 1)
	files["book/chapters/house-table-rules.yaml"] = "title: Package chapter that owns the house- name\npages:\n  - blocks:\n      - text: taken\n"
	pkg := testPackage(t, files)
	ctx := context.Background()
	repo := &fakeBookRepo{}
	svc := newTestService(repo)

	if _, err := svc.SavePage(ctx, testCampaign, "u", pkg, "basics", "p0", []byte(`{"title":"Edited roll","blocks":[{"text":"House: multi\nline\ntext, yes."},{"type":"roll","dice":"2d10","label":"Roll","modifier":true,"bands":[{"max":11,"label":"Low"},{"label":"High","text":"Wins"}]}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddPage(ctx, testCampaign, "u", pkg, "basics", []byte(`{"title":"Own page","wide":true,"blocks":[{"type":"cards","items":[{"title":"A","summary":"s","text":"t"}]},{"type":"creature","name":"Imp","tagline":"Pest","look":"small","notice":["it grins"],"stats":[{"label":"Stamina","value":"7"}],"note":"flank"},{"type":"widget","widget":"rulebook-frontpage"},{"type":"example","title":"Ex","steps":["one","two"]}]}`)); err != nil {
		t.Fatal(err)
	}
	ch, err := svc.CreateChapter(ctx, testCampaign, "u", pkg, "Table rules!")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddPage(ctx, testCampaign, "u", pkg, ch.ID, []byte(`{"title":"Rule 1","director":true,"blocks":[{"type":"note","text":"Be kind."}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeletePage(ctx, testCampaign, pkg, ch.ID, ch.Pages[0].Key); err != nil { // the unwritten starter
		t.Fatal(err)
	}
	if _, err := svc.UpdateChapter(ctx, testCampaign, pkg, ch.ID, UpdateBookChapterInput{Intro: patch.Of("Our table.")}); err != nil {
		t.Fatal(err)
	}
	ch2, err := svc.CreateChapter(ctx, testCampaign, "u", pkg, "Table rules")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddPage(ctx, testCampaign, "u", pkg, ch2.ID, []byte(`{"blocks":[{"text":"second"}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeletePage(ctx, testCampaign, pkg, ch2.ID, ch2.Pages[0].Key); err != nil {
		t.Fatal(err)
	}

	data, err := svc.Export(ctx, testCampaign, pkg)
	if err != nil {
		t.Fatal(err)
	}
	dir, names := unzipBook(t, data)
	for _, n := range names {
		if !strings.HasPrefix(n, "book/") {
			t.Errorf("unexpected entry %q", n)
		}
	}
	// Both house chapters got fresh ids that clash with nothing.
	for _, want := range []string{"book/chapters/house-table-rules-2.yaml", "book/chapters/house-table-rules-3.yaml", "book/chapters/basics.yaml", "book/book.yaml"} {
		if !contains(names, want) {
			t.Errorf("export is missing %s (have %v)", want, names)
		}
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "book/chapters/house-table-rules.yaml")); !strings.Contains(string(raw), "Package chapter that owns") {
		t.Errorf("a house chapter overwrote a package chapter: %s", raw)
	}
	basics, _ := os.ReadFile(filepath.Join(dir, "book/chapters/basics.yaml"))
	for _, noise := range []string{"director: false", "modifier: false", "wide: false", `text: ""`} {
		if strings.Contains(string(basics), noise) {
			t.Errorf("export is not readable YAML, found %q:\n%s", noise, basics)
		}
	}
	if !strings.Contains(string(basics), "multi\n") && !strings.Contains(string(basics), "|") {
		t.Errorf("multi-line text should be a block scalar:\n%s", basics)
	}

	// Glossary terms live in the package's data/, which the export does not carry.
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data/rules-glossary.json"), []byte(files["data/rules-glossary.json"]), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBook(dir, &SystemManifest{ID: "ds", Name: "Draw Steel", Widgets: []WidgetDef{{Slug: "rulebook-frontpage"}}})
	if err != nil {
		t.Fatalf("exported book does not load: %v", err)
	}
	merged, err := svc.Edition(ctx, testCampaign, pkg)
	if err != nil {
		t.Fatal(err)
	}
	// The package chapter named house-table-rules and the two house chapters:
	// the loaded book carries the House rules part with the renamed ids.
	wantJSON, _ := json.Marshal(renameChapters(merged, map[string]string{ch.ID: "house-table-rules-2", ch2.ID: "house-table-rules-3"}))
	gotJSON, _ := json.Marshal(loaded)
	if string(wantJSON) != string(gotJSON) {
		t.Errorf("round trip differs\nwant %s\n got %s", wantJSON, gotJSON)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// renameChapters returns a copy of b with chapter ids replaced, to compare an
// edition with its export, where house chapters get file-safe ids.
func renameChapters(b *Book, ids map[string]string) *Book {
	out := *b
	out.Parts = nil
	for _, p := range b.Parts {
		np := p
		np.Chapters = nil
		for _, c := range p.Chapters {
			if id, ok := ids[c.ID]; ok {
				c.ID = id
			}
			np.Chapters = append(np.Chapters, c)
		}
		out.Parts = append(out.Parts, np)
	}
	return &out
}
