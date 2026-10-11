package ai_workspace

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// An @[Name] that matches no page warns on the row, naming it; one that
// matches an existing page or a page in the same paste does not.
func TestImport_ReviewWarnsOnUnknownPageLinks(t *testing.T) {
	h := NewHandler(nil)
	h.SetImportLookup(&stubLookup{
		existing: map[string]*entities.Entity{"known": {ID: "e-1", CampaignID: "camp-1", Name: "Known", Slug: "known"}},
	})
	h.SetImportCommitter(importer.NewCommitter(nil))
	md := "---\nname: Maro\ntype: character\n---\nSees @[Known], @[Lyra] and @[Ghost Town].\n" +
		"---\nname: Lyra\ntype: character\n---\nHello.\n"
	body, ct := multipartBody(t, "markdown_paste", md)
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/ai-workspace/import/parse", body)
	req.Header.Set(echo.HeaderContentType, ct)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	ownerCtx(c)
	if err := h.ParseImport(c); err != nil {
		t.Fatal(err)
	}
	out := rec.Body.String()
	if !strings.Contains(out, "@[Ghost Town]") {
		t.Errorf("review should name the unknown link; got:\n%s", out)
	}
	for _, not := range []string{"@[Known]", "@[Lyra]"} {
		if strings.Contains(out, "The page link "+not) {
			t.Errorf("%s resolves and must not warn", not)
		}
	}
}
