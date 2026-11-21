package entities

// new_page_error_test.go pins what a refused "new page" save keeps, and where
// Cancel goes: the parent the page was being added under, the descriptor,
// the template and the visibility, and a same-site way back.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type refusingCreateSvc struct {
	EntityService
	parent *Entity
}

func (s *refusingCreateSvc) Create(context.Context, string, string, CreateEntityInput) (*Entity, error) {
	return nil, errors.New("refused")
}
func (s *refusingCreateSvc) GetByID(_ context.Context, id string) (*Entity, error) {
	if s.parent != nil && id == s.parent.ID {
		return s.parent, nil
	}
	return nil, errors.New("not found")
}
func (s *refusingCreateSvc) GetEntityTypeByID(_ context.Context, id int) (*EntityType, error) {
	return &EntityType{ID: id, Name: "Item", Slug: "item"}, nil
}
func (s *refusingCreateSvc) GetEntityTypes(context.Context, string) ([]EntityType, error) {
	return []EntityType{{ID: 7, Name: "Item", Slug: "item", Enabled: true}}, nil
}

func refusedCreate(t *testing.T, parent *Entity, form url.Values) string {
	t.Helper()
	h := &Handler{service: &refusingCreateSvc{parent: parent}}
	c := newCreateContext(t, "", form.Encode(), echo.MIMEApplicationForm)
	if err := h.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return recorderBody(c)
}

func recorderBody(c *echo.Context) string {
	r, _ := c.Response().(*echo.Response)
	if r == nil {
		return ""
	}
	if rec, ok := r.ResponseWriter.(*httptest.ResponseRecorder); ok {
		return rec.Body.String()
	}
	return ""
}

func TestCreate_RefusedSaveKeepsWhatWasChosen(t *testing.T) {
	parent := &Entity{ID: "parent-1", CampaignID: "camp-1", Name: "The Dockside"}
	form := url.Values{
		"name": {"Bad Name"}, "entity_type_id": {"7"}, "parent_id": {"parent-1"},
		"type_label": {"Tavern"}, "template_id": {"3"}, "is_private": {"true"},
		"return": {"/campaigns/camp-1/entities?type=7"},
	}
	body := refusedCreate(t, parent, form)
	for _, want := range []string{
		`selectedID: &#39;parent-1&#39;`, `selectedName: &#39;The Dockside&#39;`,
		`value="Tavern"`, `data-selected-template="3"`,
		`name="is_private" value="true"`, newPageHiddenLine,
		`name="return" value="/campaigns/camp-1/entities?type=7"`,
		// Cancel returns to the parent page, not the Pages list.
		`href="/campaigns/camp-1/entities/parent-1" class="quiet-word">Cancel`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("refused form is missing %q", want)
		}
	}
}

func TestCreate_RefusedSaveIgnoresAParentFromAnotherCampaign(t *testing.T) {
	foreign := &Entity{ID: "parent-9", CampaignID: "camp-2", Name: "Elsewhere"}
	body := refusedCreate(t, foreign, url.Values{"name": {"x"}, "entity_type_id": {"7"}, "parent_id": {"parent-9"}})
	if strings.Contains(body, "Elsewhere") {
		t.Errorf("a page of another campaign must not come back as the parent")
	}
}

func TestNewPageCancelURL(t *testing.T) {
	parent := &Entity{ID: "p1"}
	tests := []struct {
		name      string
		parent    *Entity
		preselect int
		ret       string
		want      string
	}{
		{"under a page", parent, 7, "/campaigns/c/x", "/campaigns/c/entities/p1"},
		{"from a category", nil, 7, "/campaigns/c/x", "/campaigns/c/entities?type=7"},
		{"from somewhere else", nil, 0, "/campaigns/c/x", "/campaigns/c/x"},
		{"from nowhere", nil, 0, "", "/campaigns/c/entities"},
	}
	for _, tt := range tests {
		if got := newPageCancelURL("c", tt.parent, tt.preselect, tt.ret); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestOpenedFrom(t *testing.T) {
	tests := []struct {
		name, method, target, referer, want string
	}{
		{"return field", http.MethodGet, "/campaigns/c/entities/new?return=%2Fcampaigns%2Fc%2Fcalendar", "", "/campaigns/c/calendar"},
		{"protocol-relative return is dropped", http.MethodGet, "/campaigns/c/entities/new?return=%2F%2Fevil.test", "", ""},
		{"absolute return is dropped", http.MethodGet, "/campaigns/c/entities/new?return=https%3A%2F%2Fevil.test", "", ""},
		{"backslash return is dropped", http.MethodGet, "/campaigns/c/entities/new?return=%2F%5Cevil.test", "", ""},
		{"same-site referer", http.MethodGet, "/campaigns/c/entities/new", "http://example.com/campaigns/c/maps?x=1", "/campaigns/c/maps?x=1"},
		{"other-site referer", http.MethodGet, "/campaigns/c/entities/new", "http://evil.test/campaigns/c/maps", ""},
		{"the form's own address", http.MethodGet, "/campaigns/c/entities/new", "http://example.com/campaigns/c/entities/new", ""},
		{"a post does not use the referer", http.MethodPost, "/campaigns/c/entities", "http://example.com/campaigns/c/entities/new", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}
			c := echo.New().NewContext(req, httptest.NewRecorder())
			if got := openedFrom(c); got != tt.want {
				t.Errorf("openedFrom = %q, want %q", got, tt.want)
			}
		})
	}
}

var _ = campaigns.RoleOwner
