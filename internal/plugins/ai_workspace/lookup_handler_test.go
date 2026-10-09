package ai_workspace

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

const lookupFixture = `---
kind: lookup
what: table
name: Loot
---
---
kind: table
name: Never written
---
- Gem`

func postLookup(t *testing.T, h *Handler, form url.Values) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/ai-workspace/import/parse", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	ownerCtx(c)
	if err := h.ParseImport(c); err != nil {
		t.Fatal(err)
	}
	return rec.Body.String()
}

// A paste with lookups is answered, never reviewed or written; the answer
// follows the privacy picked on the panel, Safe by default.
func TestImport_LookupsAreAnswered(t *testing.T) {
	m := &memTables{doc: records.TableDoc{Tables: []records.Table{{ID: "loot", Name: "Loot", Entries: []records.Entry{{Name: "Gem", Weight: 1}}}}}}
	h := recordsHandler(m)
	h.SetLookups(&records.Lookups{Tables: m})

	safe := postLookup(t, h, url.Values{"markdown_paste": {lookupFixture}})
	for _, want := range []string{"Answer for your AI", "1 lookup.", "1 other block was left out", "left out in Safe privacy", `<option value="safe" selected>`} {
		if !strings.Contains(safe, want) {
			t.Errorf("safe answer missing %q", want)
		}
	}
	if strings.Contains(safe, "rec_0_include") {
		t.Error("lookups went to the review")
	}

	permitted := postLookup(t, h, url.Values{"markdown_paste": {lookupFixture}, "lookup_privacy": {"permitted"}})
	if !strings.Contains(permitted, "- Gem") || !strings.Contains(permitted, `<option value="permitted" selected>`) {
		t.Errorf("permitted answer lacks the table:\n%s", permitted)
	}
	if len(m.doc.Tables) != 1 {
		t.Fatalf("a lookup paste wrote a table: %+v", m.doc.Tables)
	}
}

// The AI tool is one box: copy a prompt, paste (open), export.
func TestSettingsTab_OneAIToolBox(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Ashfall"}, MemberRole: campaigns.RoleOwner}
	var buf bytes.Buffer
	if err := aiToolSection(cc).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	order := []string{"AI tool", "Copy a prompt for your AI", "Paste what your AI gives you", "Export the whole campaign for AI"}
	at := -1
	for _, s := range order {
		i := strings.Index(html, s)
		if i <= at {
			t.Fatalf("%q missing or out of order", s)
		}
		at = i
	}
	for _, id := range []string{`id="ai-prompt-modal-host"`, `id="ai-import-review-host"`, `id="ai-export-modal-host"`, `x-data="{ open: 'paste' }"`} {
		if !strings.Contains(html, id) {
			t.Errorf("box missing %s", id)
		}
	}
}
