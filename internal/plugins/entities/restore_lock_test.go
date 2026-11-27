package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// restorePage is the page a restore test acts on, seen by an editor.
type restorePage struct {
	hideAllMentions
	entity *Entity
}

func (r restorePage) GetByID(_ context.Context, _ string) (*Entity, error) { return r.entity, nil }
func (restorePage) CheckEntityAccess(_ context.Context, _ string, _ int, _ string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: true, CanEdit: true}, nil
}

// restoreSafety records whether a restore was written.
type restoreSafety struct {
	PageSafetyService
	version  *EntityVersion
	restored bool
}

func (s *restoreSafety) Version(_ context.Context, _, _ string) (*EntityVersion, error) {
	return s.version, nil
}
func (s *restoreSafety) RestoreVersion(_ context.Context, _, _ string) error {
	s.restored = true
	return nil
}

// A restore is held to the editor's lock: a Scribe may not put text back
// over hidden parts, nor put back text that has hidden parts of its own.
func TestRestoreVersion_HiddenPartsLock(t *testing.T) {
	str := func(s string) *string { return &s }
	secret := `<p>open <span data-secret="true">the duke lies</span></p>`
	mention := `<p><a data-mention-id="x" href="/p">Hidden Page</a></p>`
	plain := `<p>plain text</p>`

	cases := []struct {
		name        string
		role        campaigns.Role
		current     string
		version     string
		wantRestore bool
	}{
		{"scribe, current text has a secret", campaigns.RoleScribe, secret, plain, false},
		{"scribe, version has a secret", campaigns.RoleScribe, plain, secret, false},
		{"scribe, current text mentions a hidden page", campaigns.RoleScribe, mention, plain, false},
		{"scribe, nothing hidden", campaigns.RoleScribe, plain, plain, true},
		{"owner, secrets on both sides", campaigns.RoleOwner, secret, secret, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			safety := &restoreSafety{version: &EntityVersion{EntryHTML: str(tc.version)}}
			h := &PageSafetyHandler{svc: safety, entities: restorePage{entity: &Entity{ID: "p", CampaignID: "c", EntryHTML: str(tc.current)}}}
			c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil), httptest.NewRecorder())
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c"}, {Name: "eid", Value: "p"}, {Name: "vid", Value: "v1"}})
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c"}, MemberRole: tc.role})
			err := h.RestoreVersion(c)
			if safety.restored != tc.wantRestore {
				t.Fatalf("restored = %v, want %v (err %v)", safety.restored, tc.wantRestore, err)
			}
			if !tc.wantRestore {
				if ae, ok := err.(*apperror.AppError); !ok || ae.Code != http.StatusForbidden {
					t.Errorf("want 403, got %v", err)
				}
			}
		})
	}
}
