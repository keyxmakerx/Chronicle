// page_write_scope_test.go pins that the alias and bulk-tag writes only reach
// pages in the URL's campaign that the caller can see: the entity ids come
// from the request, and the services write by id alone.
package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// scopeFakeEntitySvc records which entities had aliases written.
type scopeFakeEntitySvc struct {
	visFakeEntitySvc
	wrote *[]string
}

func (s scopeFakeEntitySvc) SetAliases(_ context.Context, entityID string, _ []string) error {
	*s.wrote = append(*s.wrote, entityID)
	return nil
}

// scopeFakeTagFetcher records which entities had tags written.
type scopeFakeTagFetcher struct {
	EntityTagFetcher
	wrote *[]string
}

func (f scopeFakeTagFetcher) GetEntityTags(_ context.Context, _ string, _ bool) ([]EntityTagInfo, error) {
	return nil, nil
}

func (f scopeFakeTagFetcher) SetEntityTags(_ context.Context, entityID, _ string, _ []int) error {
	*f.wrote = append(*f.wrote, entityID)
	return nil
}

func scopeFakeEntities() visFakeEntitySvc {
	return visFakeEntitySvc{
		campaignOf: map[string]string{"own-ent": "camp-1", "hidden-ent": "camp-1", "foreign-ent": "camp-2"},
		canView:    map[string]bool{"own-ent": true, "hidden-ent": false, "foreign-ent": true},
	}
}

func scopeContext(method, target, body, eid string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if eid != "" {
		c.SetParamNames("id", "eid")
		c.SetParamValues("camp-1", eid)
	}
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe, IsMember: true,
	})
	auth.SetSession(c, &auth.Session{UserID: "scribe-1"})
	return c, rec
}

func TestSetAliasesAPI_ScopedToCampaign(t *testing.T) {
	tests := []struct {
		name      string
		eid       string
		wantWrite bool
	}{
		{"page in this campaign is written", "own-ent", true},
		{"page hidden from the caller is 404", "hidden-ent", false},
		{"another campaign's page is 404", "foreign-ent", false},
		{"unknown page is 404", "missing-ent", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var wrote []string
			h := NewHandler(scopeFakeEntitySvc{visFakeEntitySvc: scopeFakeEntities(), wrote: &wrote})
			c, rec := scopeContext(http.MethodPut, "/campaigns/camp-1/entities/"+tc.eid+"/aliases", `{"aliases":["x"]}`, tc.eid)
			err := h.SetAliasesAPI(c)
			if tc.wantWrite {
				if err != nil || rec.Code != http.StatusOK || len(wrote) != 1 {
					t.Fatalf("err=%v code=%d wrote=%v, want 200 and one write", err, rec.Code, wrote)
				}
				return
			}
			assertAppError(t, err, http.StatusNotFound)
			if len(wrote) != 0 {
				t.Errorf("aliases written for %s: %v", tc.eid, wrote)
			}
		})
	}
}

func TestBulkTagsAPI_SkipsPagesOutsideCampaign(t *testing.T) {
	for _, action := range []string{"add", "remove", "set"} {
		t.Run(action, func(t *testing.T) {
			var wrote []string
			h := NewHandler(scopeFakeEntities())
			h.SetTagFetcher(scopeFakeTagFetcher{wrote: &wrote})
			body := `{"entity_ids":["own-ent","hidden-ent","foreign-ent","missing-ent"],"tag_ids":[1],"action":"` + action + `"}`
			c, rec := scopeContext(http.MethodPost, "/campaigns/camp-1/entities/bulk-tags", body, "")
			if err := h.BulkTagsAPI(c); err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(wrote) != 1 || wrote[0] != "own-ent" {
				t.Errorf("wrote tags for %v, want only own-ent", wrote)
			}
			if !strings.Contains(rec.Body.String(), `"processed":1`) {
				t.Errorf("body = %s, want processed 1", rec.Body.String())
			}
		})
	}
}
