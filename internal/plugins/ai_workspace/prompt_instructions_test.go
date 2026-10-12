package ai_workspace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/prompt"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// typesOnly answers the page-type list; listing pages fails the test, since
// the standing instructions must carry no campaign content.
type typesOnly struct{ t *testing.T }

func (s typesOnly) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return []entities.EntityType{{Name: "Character", Slug: "character", Enabled: true}}, nil
}
func (s typesOnly) List(context.Context, string, int, int, string, entities.ListOptions) ([]entities.Entity, int, error) {
	s.t.Error("the instructions read campaign pages")
	return nil, 0, nil
}

// The copy button's text: every format and the lookups, plus page types,
// and plain text the browser can put on the clipboard as is.
func TestPromptInstructions(t *testing.T) {
	m := &memTables{}
	h := recordsHandler(m)
	h.SetPromptBuilder(prompt.NewService(typesOnly{t}, nil, nil))
	h.SetLookups(&records.Lookups{})
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/ai-workspace/prompt/instructions", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	ownerCtx(c)
	if err := h.PromptInstructions(c); err != nil {
		t.Fatal(err)
	}
	if ct := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content type %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"standing instructions", "kind: table (Rolling table)", "`what: format`", "slug: `character`", "## How to answer my requests"} {
		if !strings.Contains(body, want) {
			t.Errorf("instructions lack %q:\n%s", want, body)
		}
	}
}
