package ai_workspace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type memTables struct{ doc records.TableDoc }

func (m *memTables) Get(context.Context, string) (records.TableDoc, error) { return m.doc, nil }
func (m *memTables) Put(_ context.Context, _ string, d records.TableDoc, _ string) error {
	m.doc = d
	return nil
}

const recordsFixture = `---
kind: table
name: Tavern names
---
- The Rusty Anchor
- The Drowned Rat
---
kind: table
action: delete
name: Loot
---
---
kind: spaceship
name: Nope
---`

func recordsHandler(m *memTables) *Handler {
	h := NewHandler(nil)
	h.SetImportLookup(&stubLookup{})
	h.SetImportCommitter(importer.NewCommitter(nil))
	h.SetRecords(records.NewRegistry(records.TableKind{Svc: m}))
	return h
}

func ownerCtx(c echo.Context) {
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	fakeContext(c, &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Ashfall"},
		MemberRole: campaigns.RoleOwner,
	})
}

func TestImport_RecordsReview(t *testing.T) {
	m := &memTables{doc: records.TableDoc{Tables: []records.Table{{ID: "loot", Name: "Loot"}}}}
	h := recordsHandler(m)
	body, ct := multipartBody(t, "markdown_paste", recordsFixture)
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/ai-workspace/import/parse", body)
	req.Header.Set(echo.HeaderContentType, ct)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	ownerCtx(c)
	if err := h.ParseImport(c); err != nil {
		t.Fatal(err)
	}
	html := rec.Body.String()
	for _, want := range []string{
		`name="rec_0_include"`, "Rolling table", "2 entries",
		`name="rec_1_delete_confirmed"`, "will be removed for good",
		"is not something AI Import can change", "Import 2 changes",
		// A refused record is not markdown that failed to parse.
		"0 parse errors", "1 can&#39;t be imported yet", "(1 row that can&#39;t be imported yet will be skipped)",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("review missing %q", want)
		}
	}
}

// A removal is refused at commit unless its own tick was sent, whatever
// the browser allowed.
func TestImport_RecordsCommit_DeleteNeedsConfirmation(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		m := &memTables{doc: records.TableDoc{Tables: []records.Table{{ID: "loot", Name: "Loot"}}}}
		h := recordsHandler(m)
		form := url.Values{
			"markdown_source": {recordsFixture},
			"rec_0_include":   {"on"},
			"rec_1_include":   {"on"},
			"rec_2_include":   {"on"},
		}
		if confirmed {
			form.Set("rec_1_delete_confirmed", "on")
		}
		req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/ai-workspace/import/commit", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		ownerCtx(c)
		if err := h.CommitImport(c); err != nil {
			t.Fatal(err)
		}
		hasLoot := false
		for _, tb := range m.doc.Tables {
			if tb.Name == "Loot" {
				hasLoot = true
			}
		}
		if hasLoot == confirmed {
			t.Errorf("confirmed=%v: Loot still there = %v", confirmed, hasLoot)
		}
		if len(m.doc.Tables) == 0 || m.doc.Tables[len(m.doc.Tables)-1].Name != "Tavern names" {
			t.Errorf("confirmed=%v: new table not created: %+v", confirmed, m.doc)
		}
	}
}

// Safe export (the default) lists records as a player would see them, so
// the record kinds drop hidden content exactly as the other categories do.
func TestExportActor_FollowsPrivacy(t *testing.T) {
	cases := []struct {
		mode    aiexport.PrivacyMode
		dmShows bool
	}{
		{aiexport.PrivacyModeSafe, false},
		{aiexport.PrivacyModePermitted, true},
		{aiexport.PrivacyModeEverything, true},
	}
	for _, tc := range cases {
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
		ownerCtx(c)
		cc := campaigns.GetCampaignContext(c)
		if got := exportActor(c, cc, tc.mode).CanAuthorDmOnly(); got != tc.dmShows {
			t.Errorf("%v: CanAuthorDmOnly = %v, want %v", tc.mode, got, tc.dmShows)
		}
	}
	m := &memTables{doc: records.TableDoc{Tables: []records.Table{{ID: "loot", Name: "Loot", Entries: []records.Entry{{Name: "Gem", Weight: 1}}}}}}
	reg := records.NewRegistry(records.TableKind{Svc: m})
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	ownerCtx(c)
	cc := campaigns.GetCampaignContext(c)
	if out := reg.ExportAll(context.Background(), "camp-1", exportActor(c, cc, aiexport.PrivacyModeSafe)); strings.Contains(out, "Loot") {
		t.Errorf("safe export listed a rolling table:\n%s", out)
	}
	if out := reg.ExportAll(context.Background(), "camp-1", exportActor(c, cc, aiexport.PrivacyModeEverything)); !strings.Contains(out, "Loot") {
		t.Errorf("everything export dropped the rolling table:\n%s", out)
	}
}
