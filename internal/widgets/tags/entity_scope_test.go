// entity_scope_test.go pins that the per-entity tag write only reaches a page
// in the URL's campaign that the caller can see. The tag ids are checked
// against the campaign, but the entity id comes from the URL.
package tags

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type scopeTagService struct {
	TagService
	wrote *[]string
}

func (s scopeTagService) SetEntityTagsVisible(_ context.Context, entityID, _ string, _ []int, _ bool) error {
	*s.wrote = append(*s.wrote, entityID)
	return nil
}

func (s scopeTagService) GetEntityTags(_ context.Context, _ string, _ bool) ([]Tag, error) {
	return []Tag{}, nil
}

func TestSetEntityTags_ScopedToCampaign(t *testing.T) {
	gate := tagFakeEntityGate{
		campaignOf: map[string]string{"own-ent": "camp-1", "hidden-ent": "camp-1", "foreign-ent": "camp-2"},
		canView:    map[string]bool{"own-ent": true, "hidden-ent": false, "foreign-ent": true},
	}
	tests := []struct {
		name     string
		eid      string
		gate     EntityGate
		wantCode int // 0 = success
	}{
		{"page in this campaign is written", "own-ent", gate, 0},
		{"page hidden from the caller is 404", "hidden-ent", gate, http.StatusNotFound},
		{"another campaign's page is 404", "foreign-ent", gate, http.StatusNotFound},
		{"unknown page is 404", "missing-ent", gate, http.StatusNotFound},
		{"no gate wired fails closed", "own-ent", nil, http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var wrote []string
			h := NewHandler(scopeTagService{wrote: &wrote})
			if tc.gate != nil {
				h.SetEntityGate(tc.gate)
			}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1/entities/"+tc.eid+"/tags", strings.NewReader(`{"tagIds":[1]}`))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("camp-1", tc.eid)
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe, IsMember: true,
			})
			auth.SetSession(c, &auth.Session{UserID: "scribe-1"})

			err := h.SetEntityTags(c)
			if tc.wantCode == 0 {
				if err != nil || rec.Code != http.StatusOK || len(wrote) != 1 {
					t.Fatalf("err=%v code=%d wrote=%v, want 200 and one write", err, rec.Code, wrote)
				}
				return
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != tc.wantCode {
				t.Fatalf("err = %v, want app error %d", err, tc.wantCode)
			}
			if len(wrote) != 0 {
				t.Errorf("tags written for %s: %v", tc.eid, wrote)
			}
		})
	}
}
