package entities

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// runOwner runs h as camp-1's owner and returns the status and body.
func runOwner(t *testing.T, h echo.HandlerFunc, req *http.Request, params ...string) (int, string, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	var names, values []string
	for i := 0; i+1 < len(params); i += 2 {
		names, values = append(names, params[i]), append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner,
	})
	err := h(c)
	code := rec.Code
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		code = ae.Code
	} else if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return code, rec.Body.String(), rec.Header().Get("Location")
}

// A category's Customize tab opens only for this campaign's categories and
// only for the tabs the workspace has.
func TestEntityTypeCustomizeTab(t *testing.T) {
	tests := []struct {
		name, campaign, tab string
		code                int
		want                string
	}{
		{"own category's layout", "camp-1", "layout", http.StatusOK, `data-widget="customize-page-editor"`},
		{"another campaign's category", "camp-2", "layout", http.StatusNotFound, ""},
		{"a tab the workspace lacks", "camp-1", "secrets", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			et := &EntityType{ID: 7, CampaignID: tt.campaign, Name: "NPC", NamePlural: "NPCs"}
			h := &Handler{service: &stubSvcForFields{etype: et}}
			req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entity-types/7/customize-tab/"+tt.tab, nil)
			code, body, _ := runOwner(t, h.EntityTypeCustomizeTab, req, "id", "camp-1", "etid", "7", "tab", tt.tab)
			if code != tt.code {
				t.Fatalf("code = %d, want %d", code, tt.code)
			}
			if tt.want != "" && !strings.Contains(body, tt.want) {
				t.Errorf("body missing %q", tt.want)
			}
		})
	}
}

// The old categories page sends a visit to Customize's Categories area.
func TestEntityTypesPage_RedirectsToCustomize(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entity-types", nil)
	code, _, loc := runOwner(t, h.EntityTypesPage, req, "id", "camp-1")
	if code != http.StatusSeeOther || loc != "/campaigns/camp-1/customize/categories" {
		t.Errorf("got %d to %q, want 303 to Customize's Categories", code, loc)
	}
}

// The old layout and configuration pages send a visit to the category's Page
// layout tab in Customize, and only for this campaign's categories.
func TestOldLayoutPages_RedirectToCustomize(t *testing.T) {
	for _, tt := range []struct {
		name     string
		campaign string
		code     int
	}{
		{"own category", "camp-1", http.StatusSeeOther},
		{"another campaign's category", "camp-2", http.StatusNotFound},
	} {
		for _, page := range []string{"template", "config"} {
			t.Run(tt.name+" "+page, func(t *testing.T) {
				h := &Handler{service: &stubSvcForFields{etype: &EntityType{ID: 7, CampaignID: tt.campaign}}}
				fn := h.TemplateEditor
				if page == "config" {
					fn = h.EntityTypeConfig
				}
				req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entity-types/7/"+page, nil)
				code, _, loc := runOwner(t, fn, req, "id", "camp-1", "etid", "7")
				if code != tt.code {
					t.Fatalf("code = %d, want %d", code, tt.code)
				}
				if tt.code == http.StatusSeeOther && loc != "/campaigns/camp-1/customize/categories/7/layout" {
					t.Errorf("redirect to %q", loc)
				}
			})
		}
	}
}

// previewSvc lists one page of category 7 for the layout preview.
type previewSvc struct {
	stubSvcForFields
	pages []Entity
}

func (s *previewSvc) List(context.Context, string, int, int, string, ListOptions) ([]Entity, int, error) {
	return s.pages, len(s.pages), nil
}

// The layout preview refuses another campaign's category, a page outside the
// category, and an oversized request, before drawing anything.
func TestLayoutPreview_Refusals(t *testing.T) {
	many := make([]string, layoutPreviewMaxBlocks+1)
	for i := range many {
		many[i] = `{"id":"b` + strconv.Itoa(i) + `","type":"title"}`
	}
	tests := []struct {
		name     string
		category *EntityType
		page     *Entity
		body     string
		code     int
	}{
		{"another campaign's category", &EntityType{ID: 7, CampaignID: "camp-2"}, nil, `{}`, http.StatusNotFound},
		{"a page from another category", &EntityType{ID: 7, CampaignID: "camp-1"}, &Entity{ID: "e9", CampaignID: "camp-1", EntityTypeID: 8}, `{"entityId":"e9","blocks":[]}`, http.StatusNotFound},
		{"a page from another campaign", &EntityType{ID: 7, CampaignID: "camp-1"}, &Entity{ID: "e9", CampaignID: "camp-2", EntityTypeID: 7}, `{"entityId":"e9","blocks":[]}`, http.StatusNotFound},
		{"too many blocks", &EntityType{ID: 7, CampaignID: "camp-1"}, nil, `{"blocks":[` + strings.Join(many, ",") + `]}`, http.StatusBadRequest},
		{"not JSON", &EntityType{ID: 7, CampaignID: "camp-1"}, nil, `blocks`, http.StatusBadRequest},
		{"no pages yet draws nothing", &EntityType{ID: 7, CampaignID: "camp-1"}, nil, `{"blocks":[{"id":"b1","type":"title"}]}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{service: &previewSvc{stubSvcForFields: stubSvcForFields{etype: tt.category, entity: tt.page}}}
			req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/entity-types/7/layout-preview", strings.NewReader(tt.body))
			code, body, _ := runOwner(t, h.LayoutPreview, req, "id", "camp-1", "etid", "7")
			if code != tt.code {
				t.Fatalf("code = %d, want %d (%s)", code, tt.code, body)
			}
			if code == http.StatusOK && !strings.Contains(body, `"blocks":{}`) {
				t.Errorf("body = %s, want no blocks", body)
			}
		})
	}
}

// The editor's starting layout for a category with nothing saved holds every
// piece the page's fallback draws, so the first Save removes none of them,
// and it passes the same check Save runs.
func TestShownFallbackLayout(t *testing.T) {
	l := shownFallbackLayout()
	if err := ValidateLayout(l, func(string) bool { return true }, func(string) bool { return false }, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"title", "tags", "relations", "entry", "image", "attributes", "details", BlockSubPages, BlockPosts, BlockWritingPrompts, BlockBacklinks} {
		if !layoutPlacesBlock(l, want) {
			t.Errorf("fallback layout lacks %q", want)
		}
	}
}
