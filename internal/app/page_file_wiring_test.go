package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// Without the page rule the files service refuses every request, and without
// the routes there is no Files section, so both calls are pinned in source.
func TestRoutes_WiresPageFiles(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{
		"media.NewPageFileService(media.NewPageFileRepository(a.DB), mediaService, &pageAccessAdapter{svc: entityService})",
		"media.RegisterPageFileRoutes(e, pageFileHandler,",
		"pageFileHandler.SetSecurityLogger(securityService)",
	} {
		if !strings.Contains(string(src), call) {
			t.Errorf("routes.go no longer calls %s", call)
		}
	}
}

// pageRuleSvc is the slice of the entities service the adapter reads.
type pageRuleSvc struct {
	entities.EntityService
	viewable   bool
	canEdit    bool
	viewErr    error
	accessErr  error
	accessCall bool
	gotCamp    string
	gotRole    int
}

func (p *pageRuleSvc) FilterViewableEntityIDs(_ context.Context, campaignID string, ids []string, role int, _ string) (map[string]bool, error) {
	p.gotCamp, p.gotRole = campaignID, role
	if p.viewErr != nil {
		return nil, p.viewErr
	}
	return map[string]bool{ids[0]: p.viewable}, nil
}

func (p *pageRuleSvc) CheckEntityAccess(context.Context, string, int, string) (*entities.EffectivePermission, error) {
	p.accessCall = true
	if p.accessErr != nil {
		return nil, p.accessErr
	}
	return &entities.EffectivePermission{CanView: true, CanEdit: p.canEdit}, nil
}

// A page file follows its page: the adapter asks the page's own visibility
// rule scoped to the campaign (which leaves out the Trash), asks the edit rule
// only of a page already found visible, and never turns an error into access.
func TestPageAccessAdapter(t *testing.T) {
	tests := []struct {
		name     string
		svc      *pageRuleSvc
		want     bool // CanView
		wantEdit bool
		wantErr  bool
		wantAsk  bool // the edit rule was consulted
	}{
		{"visible and editable", &pageRuleSvc{viewable: true, canEdit: true}, true, true, false, true},
		{"visible, read only", &pageRuleSvc{viewable: true}, true, false, false, true},
		{"not visible: the edit rule is never asked", &pageRuleSvc{canEdit: true}, false, false, false, false},
		{"visibility lookup fails", &pageRuleSvc{viewErr: errors.New("db")}, false, false, true, false},
		{"edit lookup fails", &pageRuleSvc{viewable: true, accessErr: errors.New("db")}, false, false, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&pageAccessAdapter{svc: tc.svc}).PageAccess(context.Background(), "camp", "page", 2, "u1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error %v", err, tc.wantErr)
			}
			if got.CanView != tc.want || got.CanEdit != tc.wantEdit {
				t.Errorf("access = %+v, want view %v edit %v", got, tc.want, tc.wantEdit)
			}
			if tc.svc.accessCall != tc.wantAsk {
				t.Errorf("edit rule consulted = %v, want %v", tc.svc.accessCall, tc.wantAsk)
			}
			if tc.svc.gotCamp != "camp" || tc.svc.gotRole != 2 {
				t.Errorf("visibility asked with campaign %q role %d", tc.svc.gotCamp, tc.svc.gotRole)
			}
		})
	}
}
