// entity_scope_test.go pins that creating a relation only links two pages in
// the URL's campaign that the caller can see: both ids come from the request,
// and a listed relation shows the target page's name.
package relations

import (
	"context"
	"encoding/json"
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

type scopeRelService struct {
	RelationService
	created *int
}

func (s scopeRelService) Create(_ context.Context, campaignID, sourceID, targetID, relType, revType, _ string, _ json.RawMessage, _ ...bool) (*Relation, error) {
	*s.created++
	return &Relation{CampaignID: campaignID, SourceEntityID: sourceID, TargetEntityID: targetID}, nil
}

func TestCreateRelation_ScopedToCampaign(t *testing.T) {
	gate := relDmGrantGate{
		campaignOf: map[string]string{"own-a": "camp-1", "own-b": "camp-1", "hidden": "camp-1", "foreign": "camp-2"},
		private:    map[string]bool{"hidden": true},
	}
	tests := []struct {
		name     string
		source   string
		target   string
		gate     EntityGate
		wantCode int // 0 = created
	}{
		{"two pages in this campaign are linked", "own-a", "own-b", gate, 0},
		{"another campaign's target is 404", "own-a", "foreign", gate, http.StatusNotFound},
		{"another campaign's source is 404", "foreign", "own-a", gate, http.StatusNotFound},
		{"a target hidden from the caller is 404", "own-a", "hidden", gate, http.StatusNotFound},
		{"an unknown target is 404", "own-a", "missing", gate, http.StatusNotFound},
		{"no gate wired fails closed", "own-a", "own-b", nil, http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			created := 0
			h := NewHandler(scopeRelService{created: &created})
			if tc.gate != nil {
				h.SetEntityGate(tc.gate)
			}
			e := echo.New()
			body := `{"targetEntityId":"` + tc.target + `","relationType":"ally","reverseRelationType":"ally"}`
			req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/entities/"+tc.source+"/relations", strings.NewReader(body))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("camp-1", tc.source)
			// A Player: the hidden page needs Scribe or above to view.
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsMember: true,
			})
			auth.SetSession(c, &auth.Session{UserID: "member-1"})

			err := h.CreateRelation(c)
			if tc.wantCode == 0 {
				if err != nil || rec.Code != http.StatusCreated || created != 1 {
					t.Fatalf("err=%v code=%d created=%d, want 201 and one create", err, rec.Code, created)
				}
				return
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != tc.wantCode {
				t.Fatalf("err = %v, want app error %d", err, tc.wantCode)
			}
			if created != 0 {
				t.Errorf("relation created for %s -> %s", tc.source, tc.target)
			}
		})
	}
}
