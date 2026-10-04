package ai_workspace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

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
