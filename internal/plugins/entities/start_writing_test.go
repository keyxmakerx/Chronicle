package entities

// A page made a moment ago opens ready to write: Create sends its maker to
// the page with the "write this now" mark, and the page's text block passes
// it to the editor for anyone allowed to write there. An address without the
// mark opens the page at rest, as before.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestCreate_SendsItsMakerToThePageReadyToWrite(t *testing.T) {
	const want = "/campaigns/camp-1/entities/ent-new?write=1"
	form := url.Values{"name": {"Saltreach"}, "entity_type_id": {"7"}}.Encode()
	tests := []struct {
		name   string
		htmx   bool
		status int
		header string
	}{
		{"the form as the app sends it", true, http.StatusNoContent, "HX-Redirect"},
		{"the form without scripts", false, http.StatusSeeOther, "Location"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{service: &stubCreateCapturingSvc{}}
			req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/entities", strings.NewReader(form))
			req.Header.Set("Content-Type", echo.MIMEApplicationForm)
			if tt.htmx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test", Settings: "{}"}, MemberRole: campaigns.RoleOwner, IsMember: true,
			})
			if err := h.Create(c); err != nil {
				t.Fatalf("Create: %v", err)
			}
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if got := rec.Header().Get(tt.header); got != want {
				t.Errorf("%s = %q, want %q", tt.header, got, want)
			}
		})
	}
}

// showEntryFor draws a page whose layout places the title and the text, for
// the viewer given, at the address given.
func showEntryFor(t *testing.T, cc *campaigns.CampaignContext, userID, query string) string {
	t.Helper()
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	SetGlobalBlockRegistry(reg)
	t.Cleanup(func() { SetGlobalBlockRegistry(nil) })

	ent, et := dmOnlyFixture()
	ent.IsPrivate = false
	et.Layout = EntityTypeLayout{Rows: []TemplateRow{fullWidthRow("r",
		TemplateBlock{ID: "t", Type: "title"}, TemplateBlock{ID: "e", Type: "entry"})}}
	h := &Handler{service: &dmGrantEntitySvc{entity: ent, etype: et}}

	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1"+query, nil), rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}, {Name: "eid", Value: "e1"}})
	c.Set("campaign_context", cc)
	auth.SetSession(c, &auth.Session{UserID: userID})
	if err := h.Show(c); err != nil {
		t.Fatalf("Show: %v", err)
	}
	body := rec.Body.String()
	if n := strings.Count(body, `data-widget="editor"`); n != 1 {
		t.Fatalf("the page draws its text %d times, want once", n)
	}
	return body
}

func TestShow_TheWriteMarkOpensTheTextForThoseWhoMayWriteIt(t *testing.T) {
	member := func(role campaigns.Role) *campaigns.CampaignContext {
		return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: role, IsMember: true}
	}
	tests := []struct {
		name  string
		cc    *campaigns.CampaignContext
		query string
		want  bool
	}{
		{"the owner, straight from Create", member(campaigns.RoleOwner), "?write=1", true},
		{"a scribe, straight from Create", member(campaigns.RoleScribe), "?write=1", true},
		{"the owner opening the page any other way", member(campaigns.RoleOwner), "", false},
		{"the owner with another value on the mark", member(campaigns.RoleOwner), "?write=0", false},
		{"a player handed the marked address", member(campaigns.RolePlayer), "?write=1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := showEntryFor(t, tt.cc, "u1", tt.query)
			editable := strings.Contains(body, `data-editable="true"`)
			if got := strings.Contains(body, `data-start-editing="true"`); got != tt.want {
				t.Errorf("text opens ready to write = %v, want %v", got, tt.want)
			}
			if tt.want && !editable {
				t.Error("the text is marked to open for writing but is not editable")
			}
		})
	}
}
