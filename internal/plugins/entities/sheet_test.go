package entities

import (
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The sheet is the DM Screen's view of an Adventures page. It answers 404, the
// same as a page that does not exist, to anyone outside the DM team and for
// any page that is not an Adventure, so it can be neither probed nor used to
// read another category without its chrome.
func TestSheet_Access(t *testing.T) {
	tests := []struct {
		name     string
		role     campaigns.Role
		dm       bool
		page     string
		wantCode int
		wantBody string
	}{
		{"player, adventures page", campaigns.RolePlayer, false, "adv-page", http.StatusNotFound, ""},
		{"player, ordinary page", campaigns.RolePlayer, false, "npc-page", http.StatusNotFound, ""},
		{"non-member, adventures page", campaigns.RoleNone, false, "adv-page", http.StatusNotFound, ""},
		{"scribe, ordinary page", campaigns.RoleScribe, false, "npc-page", http.StatusNotFound, ""},
		{"owner, ordinary page", campaigns.RoleOwner, false, "npc-page", http.StatusNotFound, ""},
		{"scribe, adventures page", campaigns.RoleScribe, false, "adv-page", http.StatusOK, "data-dms-sheet-page"},
		{"player with DM access, adventures page", campaigns.RolePlayer, true, "adv-page", http.StatusOK, "data-dms-sheet-page"},
		{"owner, page of another campaign", campaigns.RoleOwner, false, "other-campaign-page", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := advCtx(tt.role, tt.dm, http.MethodGet, "/campaigns/c1/entities/"+tt.page+"/sheet")
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", tt.page)
			h := advHandlerEnv()
			err := h.Sheet(c)
			if tt.wantCode == http.StatusNotFound {
				if !notFound(err) {
					t.Fatalf("Sheet err = %v, want a 404 (body %q)", err, rec.Body.String())
				}
				if strings.Contains(rec.Body.String(), "Crypt") || strings.Contains(rec.Body.String(), "Barkeep") {
					t.Fatalf("a refused sheet wrote page content: %q", rec.Body.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("Sheet err = %v", err)
			}
			if rec.Code != tt.wantCode || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("status %d body %.200q, want %d containing %q", rec.Code, rec.Body.String(), tt.wantCode, tt.wantBody)
			}
			if strings.Contains(rec.Body.String(), "<html") {
				t.Fatal("the sheet carries the site chrome; it is a fragment")
			}
		})
	}
}
