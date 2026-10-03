// viewer_fragment_test.go pins the focus-view endpoint (GET /maps/:mid/viewer):
// it must give a viewer exactly what the map page gives them (same access
// check, same role filtering, same tool gates), carry no script of its own, and
// never leak across campaigns.
package maps

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// viewerRepo holds one public pin and one dm_only pin and filters them the way
// the real repository does, by the role it is asked with.
func viewerRepo() *mockMapRepo {
	return &mockMapRepo{
		getMapFn: func(_ context.Context, id string) (*Map, error) {
			if id == "m-other" {
				return &Map{ID: id, CampaignID: "camp-2", Name: "Elsewhere"}, nil
			}
			return &Map{ID: id, CampaignID: "camp-1", Name: "Vellmoor Isle"}, nil
		},
		listMarkersFn: func(_ context.Context, _ string, role int) ([]Marker, error) {
			out := []Marker{{ID: "mk-pub", Name: "Harbour Bell", Visibility: "everyone"}}
			if role >= int(campaigns.RoleOwner) {
				out = append(out, Marker{ID: "mk-dm", Name: "Smugglers Cache", Visibility: "dm_only"})
			}
			return out, nil
		},
		campaignFrame: "gilded",
	}
}

func serveHandler(t *testing.T, h echo.HandlerFunc, path string, cc *campaigns.CampaignContext, mapID string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "mid")
	c.SetParamValues("camp-1", mapID)
	c.Set("campaign_context", cc)
	err := h(c)
	return rec, err
}

var configTag = regexp.MustCompile(`(?s)<div[^>]*id="map-config".*?></div>`)

func TestViewer_MatchesMapPageForEveryRole(t *testing.T) {
	tests := []struct {
		name        string
		cc          *campaigns.CampaignContext
		wantSecret  bool
		wantScribe  string
		wantCanDmOn string
	}{
		{"player", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer}, false, `data-is-scribe="false"`, `data-can-dm-only="false"`},
		{"scribe", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}, false, `data-is-scribe="true"`, `data-can-dm-only="false"`},
		{"owner", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}, true, `data-is-scribe="true"`, `data-can-dm-only="true"`},
		{"co-DM player", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsDmGranted: true}, true, `data-is-scribe="false"`, `data-can-dm-only="true"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(newTestMapService(viewerRepo()))

			rec, err := serveHandler(t, h.Viewer, "/campaigns/camp-1/maps/m-1/viewer", tt.cc, "m-1")
			if err != nil || rec.Code != http.StatusOK {
				t.Fatalf("Viewer: code=%d err=%v", rec.Code, err)
			}
			frag := rec.Body.String()
			page, err := serveHandler(t, h.Show, "/campaigns/camp-1/maps/m-1", tt.cc, "m-1")
			if err != nil || page.Code != http.StatusOK {
				t.Fatalf("Show: code=%d err=%v", page.Code, err)
			}

			if got := strings.Contains(frag, "Smugglers Cache"); got != tt.wantSecret {
				t.Errorf("dm_only marker in fragment = %v, want %v (players must never receive it)", got, tt.wantSecret)
			}
			if !strings.Contains(frag, "Harbour Bell") {
				t.Errorf("public marker missing from fragment")
			}
			cfg := configTag.FindString(frag)
			for _, want := range []string{tt.wantScribe, tt.wantCanDmOn, `data-widget="map-viewer"`, `data-campaign-id="camp-1"`, `"frame":"gilded"`} {
				if !strings.Contains(cfg, strings.ReplaceAll(want, `"`, `&#34;`)) && !strings.Contains(cfg, want) {
					t.Errorf("config missing %s\n%s", want, cfg)
				}
			}
			if pageCfg := configTag.FindString(page.Body.String()); pageCfg != cfg {
				t.Errorf("fragment config differs from the map page's:\nfragment: %s\npage:     %s", cfg, pageCfg)
			}
			// Framed like the page, with no script of its own (htmx and innerHTML
			// would not run it; the focus view starts the viewer itself).
			if !strings.Contains(frag, `id="mp-frame"`) || !strings.Contains(frag, `data-frame="gilded"`) {
				t.Errorf("fragment must carry the resolved frame")
			}
			if strings.Contains(frag, "<script") {
				t.Errorf("fragment must not contain a script element")
			}
		})
	}
}

func TestViewer_CrossCampaignAndMissingMaps404(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	repo := viewerRepo()
	base := repo.getMapFn
	repo.getMapFn = func(ctx context.Context, id string) (*Map, error) {
		if id == "m-gone" {
			return nil, apperror.NewNotFound("map not found")
		}
		return base(ctx, id)
	}
	for _, mapID := range []string{"m-other", "m-gone"} {
		t.Run(mapID, func(t *testing.T) {
			h := NewHandler(newTestMapService(repo))
			_, err := serveHandler(t, h.Viewer, "/campaigns/camp-1/maps/"+mapID+"/viewer", cc, mapID)
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
				t.Fatalf("want 404 AppError, got %v", err)
			}
		})
	}
}

func TestViewer_RouteAccessMatchesMapPage(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		public    bool
		wantLogin bool
	}{
		{"viewer public campaign", "/campaigns/camp-1/maps/m1/viewer", true, false},
		{"viewer private campaign", "/campaigns/camp-1/maps/m1/viewer", false, true},
		{"page public campaign", "/campaigns/camp-1/maps/m1", true, false},
		{"page private campaign", "/campaigns/camp-1/maps/m1", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newGuardRouter(tt.public)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if got := isLoginRedirect(rec); got != tt.wantLogin {
				t.Fatalf("login-redirect=%v (code=%d), want %v", got, rec.Code, tt.wantLogin)
			}
			if !tt.wantLogin && rec.Code != http.StatusOK {
				t.Fatalf("code=%d, want 200", rec.Code)
			}
		})
	}
}

func TestEntityMapPreview_IsFramedAndInlineWired(t *testing.T) {
	imageID := "img-1"
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer}
	for _, frame := range []string{"atlas", "gilded", "futuristic"} {
		t.Run(frame, func(t *testing.T) {
			m := &Map{ID: "m-1", CampaignID: "camp-1", Name: "Vellmoor Isle", ImageID: &imageID}
			data := MapViewData{CampaignID: "camp-1", Map: m, Display: ResolveDisplay(m, frame)}
			out := render(t, BlockEntityMapEmbed(cc, "ent-1", data, false, "own"))
			for _, want := range []string{
				`data-frame="` + frame + `"`,
				`data-viewer-url="/campaigns/camp-1/maps/m-1/viewer"`,
				`data-page-url="/campaigns/camp-1/maps/m-1"`,
				`role="button"`, `tabindex="0"`,
				`onclick="(function(el,e){`, `onkeydown="(function(el,e){`,
				"ChronicleMapFocus.open",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("preview missing %q", want)
				}
			}
			// A picture, not a live map: no viewer markup, no marker data, no script.
			for _, bad := range []string{"map-config", "data-markers", "<script", "__templ_"} {
				if strings.Contains(out, bad) {
					t.Errorf("preview must not contain %q", bad)
				}
			}
		})
	}
}
