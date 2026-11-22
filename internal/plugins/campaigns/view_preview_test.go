package campaigns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// previewRequest runs gate (AllowPublicCampaignAccess or RequireCampaignAccess)
// for a signed-in user and returns the context the handler saw (nil if the
// gate stopped the request) and the recorder.
func previewRequest(t *testing.T, gate func(CampaignService) echo.MiddlewareFunc, svc CampaignService, method string, cookies []*http.Cookie, hdr map[string]string) (*CampaignContext, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, "/campaigns/camp-1", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	auth.SetSession(c, &auth.Session{UserID: "u1"})
	var got *CampaignContext
	err := gate(svc)(func(c *echo.Context) error {
		got = GetCampaignContext(c)
		return nil
	})(c)
	if err != nil {
		rec.WriteHeader(apperror.SafeCode(err))
	}
	return got, rec
}

func guestCookie() []*http.Cookie {
	return []*http.Cookie{{Name: viewAsGuestCookie, Value: "camp-1"}}
}

func TestAllowPublicAccess_GuestPreview(t *testing.T) {
	public := &Campaign{ID: "camp-1", IsPublic: true}
	private := &Campaign{ID: "camp-1", IsPublic: false}
	tests := []struct {
		name        string
		campaign    *Campaign
		role        Role
		dmGrant     bool
		method      string
		cookies     []*http.Cookie
		wantVisitor bool
	}{
		{"owner, public, GET: visitor", public, RoleOwner, false, http.MethodGet, guestCookie(), true},
		{"owner, public, HEAD: visitor", public, RoleOwner, false, http.MethodHead, guestCookie(), true},
		{"owner, public, POST: real", public, RoleOwner, false, http.MethodPost, guestCookie(), false},
		{"owner, private, GET: real", private, RoleOwner, false, http.MethodGet, guestCookie(), false},
		{"owner, public, no cookie: real", public, RoleOwner, false, http.MethodGet, nil, false},
		{"owner, cookie not 1: real", public, RoleOwner, false, http.MethodGet, []*http.Cookie{{Name: viewAsGuestCookie, Value: "0"}}, false},
		{"co-DM via grant: visitor", public, RolePlayer, true, http.MethodGet, guestCookie(), true},
		{"plain player with the cookie: no effect", public, RolePlayer, false, http.MethodGet, guestCookie(), false},
		{"scribe with the cookie: no effect", public, RoleScribe, false, http.MethodGet, guestCookie(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			camp := *tt.campaign
			if tt.dmGrant {
				camp.Settings = `{"dm_grant_ids":["u1"]}`
			}
			svc := &stubPublicSvc{campaign: &camp, member: &CampaignMember{UserID: "u1", Role: tt.role}}
			cc, _ := previewRequest(t, AllowPublicCampaignAccess, svc, tt.method, tt.cookies, nil)
			if cc == nil {
				t.Fatal("no context")
			}
			if tt.wantVisitor {
				if cc.MemberRole != RoleNone || cc.IsMember || cc.IsDmGranted || cc.IsSiteAdmin || cc.VisitUntil != nil || cc.CanAuthorDmOnly() {
					t.Errorf("not a plain visitor: %+v", cc)
				}
				if cc.VisibilityRole() != int(RoleNone) {
					t.Errorf("visibility role = %d, want 0", cc.VisibilityRole())
				}
				if cc.Preview != PreviewGuest || !cc.PreviewerCanDM || cc.PreviewerRole != tt.role {
					t.Errorf("chrome fields wrong: %+v", cc)
				}
				if cc.HomeAudience() != HomeGuest {
					t.Errorf("home audience = %s, want guest", cc.HomeAudience())
				}
				return
			}
			if cc.Preview != "" || cc.PreviewerCanDM || cc.PreviewerRole != RoleNone {
				t.Errorf("no preview was due, got %+v", cc)
			}
			if cc.MemberRole != tt.role || !cc.IsMember {
				t.Errorf("real context lost: %+v", cc)
			}
		})
	}
}

func TestAllowPublicAccess_GuestPreviewNonMemberUnchanged(t *testing.T) {
	svc := &stubPublicSvc{campaign: &Campaign{ID: "camp-1", IsPublic: true}, memberErr: apperror.NewNotFound("no")}
	cc, _ := previewRequest(t, AllowPublicCampaignAccess, svc, http.MethodGet, guestCookie(), nil)
	if cc == nil || cc.Preview != "" || cc.PreviewerCanDM || cc.MemberRole != RoleNone {
		t.Errorf("a non-member sending the cookie must get the ordinary visitor context: %+v", cc)
	}
}

func TestRequireCampaignAccess_GuestPreview(t *testing.T) {
	public := &Campaign{ID: "camp-1", IsPublic: true}
	svc := &stubPublicSvc{campaign: public, member: &CampaignMember{UserID: "u1", Role: RoleOwner}}
	html := map[string]string{"Accept": "text/html"}
	tests := []struct {
		name       string
		gate       func(CampaignService) echo.MiddlewareFunc
		method     string
		cookies    []*http.Cookie
		hdr        map[string]string
		campaign   *Campaign
		role       Role
		wantStatus int
		wantBody   string
	}{
		{"GET page is refused with a way back", RequireCampaignAccess, http.MethodGet, guestCookie(), html, public, RoleOwner, 403, "A guest can&#39;t open this page."},
		{"GET page, even-if-archived gate", RequireCampaignAccessEvenIfArchived, http.MethodGet, guestCookie(), html, public, RoleOwner, 403, "A guest can&#39;t open this page."},
		{"HTMX fragment just gets the 403", RequireCampaignAccess, http.MethodGet, guestCookie(), map[string]string{"HX-Request": "true", "Accept": "text/html"}, public, RoleOwner, 403, ""},
		{"JSON read gets the 403", RequireCampaignAccess, http.MethodGet, guestCookie(), nil, public, RoleOwner, 403, ""},
		{"POST is never affected", RequireCampaignAccess, http.MethodPost, guestCookie(), html, public, RoleOwner, 200, ""},
		{"no cookie passes", RequireCampaignAccess, http.MethodGet, nil, html, public, RoleOwner, 200, ""},
		{"private campaign passes (no downgrade there)", RequireCampaignAccess, http.MethodGet, guestCookie(), html, &Campaign{ID: "camp-1"}, RoleOwner, 200, ""},
		{"a player's cookie has no effect", RequireCampaignAccess, http.MethodGet, guestCookie(), html, public, RolePlayer, 200, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &stubPublicSvc{campaign: tt.campaign, member: &CampaignMember{UserID: "u1", Role: tt.role}}
			_ = svc
			cc, rec := previewRequest(t, tt.gate, s, tt.method, tt.cookies, tt.hdr)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == 200 && cc == nil {
				t.Error("handler should have run")
			}
			if tt.wantStatus != 200 && cc != nil {
				t.Error("handler must not run on a refusal")
			}
			if tt.wantBody != "" {
				body := rec.Body.String()
				for _, want := range []string{tt.wantBody, "You are viewing this campaign as a guest.", `name="as" value="self"`, "/campaigns/camp-1/view-as"} {
					if !strings.Contains(body, want) && !strings.Contains(body, strings.ReplaceAll(want, "&#39;", "&#34;")) && !strings.Contains(body, strings.ReplaceAll(want, "&#39;", "'")) {
						t.Errorf("refusal page missing %q", want)
					}
				}
			}
		})
	}
}

// viewAsCall posts to ViewAs behind the same capability gate the route has.
func viewAsCall(t *testing.T, cc *CampaignContext, form url.Values, cookies []*http.Cookie, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/view-as", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set(contextKeyCampaign, cc)
	h := &Handler{}
	err := RequireCapability(func(cc *CampaignContext) bool { return cc.CanAuthorDmOnly() }, "no")(h.ViewAs)(c)
	if err != nil {
		rec.WriteHeader(apperror.SafeCode(err))
	}
	return rec
}

// cookieStates reads what the response did to the two preview cookies:
// "on", "off" (cleared) or "" (untouched).
func cookieStates(rec *httptest.ResponseRecorder) (player, guest string) {
	for _, ck := range rec.Result().Cookies() {
		state := "off"
		if ck.Value == "camp-1" {
			state = "on"
		}
		switch ck.Name {
		case viewAsPlayerCookie:
			player = state
		case viewAsGuestCookie:
			guest = state
		}
	}
	return
}

// The guest-code preview lowers the DM team's reads to a visitor holding a
// look-around pass, on a private campaign too (a pass opens one), and has no
// effect for anyone else or on a write.
func TestAllowPublicAccess_GuestCodePreview(t *testing.T) {
	public := &Campaign{ID: "camp-1", IsPublic: true}
	private := &Campaign{ID: "camp-1", IsPublic: false}
	code := []*http.Cookie{{Name: viewAsGuestCodeCookie, Value: "camp-1"}}
	both := append([]*http.Cookie{{Name: viewAsGuestCookie, Value: "camp-1"}}, code...)
	tests := []struct {
		name     string
		campaign *Campaign
		role     Role
		method   string
		cookies  []*http.Cookie
		want     string
	}{
		{"owner, public: pass holder", public, RoleOwner, http.MethodGet, code, PreviewGuestCode},
		{"owner, private: pass holder", private, RoleOwner, http.MethodGet, code, PreviewGuestCode},
		{"owner, POST: real", private, RoleOwner, http.MethodPost, code, ""},
		{"code wins over a stale guest cookie", public, RoleOwner, http.MethodGet, both, PreviewGuestCode},
		{"another campaign's cookie: real", private, RoleOwner, http.MethodGet, []*http.Cookie{{Name: viewAsGuestCodeCookie, Value: "camp-2"}}, ""},
		{"plain player with the cookie: no effect", private, RolePlayer, http.MethodGet, code, ""},
		{"scribe with the cookie: no effect", public, RoleScribe, http.MethodGet, code, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			camp := *tt.campaign
			svc := &stubPublicSvc{campaign: &camp, member: &CampaignMember{UserID: "u1", Role: tt.role}}
			cc, _ := previewRequest(t, AllowPublicCampaignAccess, svc, tt.method, tt.cookies, nil)
			if cc == nil {
				t.Fatal("no context")
			}
			if cc.Preview != tt.want {
				t.Fatalf("preview = %q, want %q", cc.Preview, tt.want)
			}
			if tt.want == "" {
				if cc.MemberRole != tt.role || !cc.IsMember || cc.VisitUntil != nil {
					t.Errorf("real context lost: %+v", cc)
				}
				return
			}
			if cc.MemberRole != RoleNone || cc.IsMember || cc.IsDmGranted || cc.IsSiteAdmin || cc.CanAuthorDmOnly() || cc.VisibilityRole() != int(RoleNone) {
				t.Errorf("not a visitor: %+v", cc)
			}
			if cc.VisitUntil == nil || !cc.VisitUntil.After(time.Now().Add(6*24*time.Hour)) {
				t.Errorf("the preview's pass should run a week, got %v", cc.VisitUntil)
			}
			if !cc.PreviewerCanDM || cc.PreviewerRole != tt.role || cc.HomeAudience() != HomeGuest {
				t.Errorf("chrome fields wrong: %+v", cc)
			}
		})
	}
}

func TestViewAs_GuestCode(t *testing.T) {
	privOwner := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: RoleOwner, IsMember: true}
	player := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: RolePlayer, IsMember: true}
	gCookie := &http.Cookie{Name: viewAsGuestCookie, Value: "camp-1"}
	codeState := func(rec *httptest.ResponseRecorder) string {
		for _, ck := range rec.Result().Cookies() {
			if ck.Name == viewAsGuestCodeCookie {
				if ck.MaxAge < 0 {
					return "off"
				}
				if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
					return "loose"
				}
				return "on"
			}
		}
		return ""
	}
	rec := viewAsCall(t, privOwner, url.Values{"as": {"guestcode"}, "next": {"home"}}, []*http.Cookie{gCookie}, false)
	if rec.Code != 303 || rec.Header().Get("Location") != "/campaigns/camp-1" || codeState(rec) != "on" {
		t.Errorf("a private campaign's owner starts it: %d %q %q", rec.Code, rec.Header().Get("Location"), codeState(rec))
	}
	if _, g := cookieStates(rec); g != "off" {
		t.Errorf("starting it clears the guest preview, guest=%q", g)
	}
	rec = viewAsCall(t, privOwner, url.Values{"as": {"self"}}, []*http.Cookie{{Name: viewAsGuestCodeCookie, Value: "camp-1"}}, false)
	if codeState(rec) != "off" {
		t.Errorf("self ends it, got %q", codeState(rec))
	}
	if rec := viewAsCall(t, player, url.Values{"as": {"guestcode"}}, nil, false); rec.Code != 403 {
		t.Errorf("a player is refused, got %d", rec.Code)
	}
}

func TestViewAs(t *testing.T) {
	camp := &Campaign{ID: "camp-1", IsPublic: true}
	owner := &CampaignContext{Campaign: camp, MemberRole: RoleOwner, IsMember: true}
	coDM := &CampaignContext{Campaign: camp, MemberRole: RolePlayer, IsMember: true, IsDmGranted: true}
	player := &CampaignContext{Campaign: camp, MemberRole: RolePlayer, IsMember: true}
	visitor := &CampaignContext{Campaign: camp}
	privOwner := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: RoleOwner, IsMember: true}
	pCookie := &http.Cookie{Name: viewAsPlayerCookie, Value: "camp-1"}
	gCookie := &http.Cookie{Name: viewAsGuestCookie, Value: "camp-1"}

	tests := []struct {
		name         string
		cc           *CampaignContext
		form         url.Values
		cookies      []*http.Cookie
		wantStatus   int
		wantLocation string
		wantPlayer   string
		wantGuest    string
	}{
		{"player, with next=home", owner, url.Values{"as": {"player"}, "next": {"home"}}, nil, 303, "/campaigns/camp-1", "on", ""},
		{"guest, with next=home", owner, url.Values{"as": {"guest"}, "next": {"home"}}, nil, 303, "/campaigns/camp-1", "", "on"},
		{"self, with next=home", owner, url.Values{"as": {"self"}, "next": {"home"}}, []*http.Cookie{pCookie}, 303, "/campaigns/camp-1", "off", ""},
		{"player clears the guest preview", owner, url.Values{"as": {"player"}, "next": {"home"}}, []*http.Cookie{gCookie}, 303, "/campaigns/camp-1", "on", "off"},
		{"guest clears the player preview", owner, url.Values{"as": {"guest"}, "next": {"home"}}, []*http.Cookie{pCookie}, 303, "/campaigns/camp-1", "off", "on"},
		{"arrange the players' dashboard", owner, url.Values{"as": {"self"}, "next": {"arrange"}, "home": {"player"}}, []*http.Cookie{pCookie}, 303, "/campaigns/camp-1/customize/home/player", "off", ""},
		{"arrange the guests' dashboard", owner, url.Values{"as": {"self"}, "next": {"arrange"}, "home": {"guest"}}, []*http.Cookie{gCookie}, 303, "/campaigns/camp-1/customize/home/guest", "", "off"},
		{"guest on a private campaign: Home's note, and no cookie left behind", privOwner, url.Values{"as": {"guest"}, "next": {"home"}}, []*http.Cookie{pCookie}, 303, "/campaigns/camp-1?dashboard=guests", "off", ""},
		{"guest on a private campaign clears a stale guest cookie", privOwner, url.Values{"as": {"guest"}}, []*http.Cookie{gCookie}, 303, "/campaigns/camp-1?dashboard=guests", "", "off"},
		{"co-DM may preview", coDM, url.Values{"as": {"guest"}, "next": {"home"}}, nil, 303, "/campaigns/camp-1", "", "on"},
		{"co-DM may not arrange", coDM, url.Values{"as": {"self"}, "next": {"arrange"}, "home": {"player"}}, nil, 403, "", "", ""},
		{"a player is refused", player, url.Values{"as": {"guest"}}, nil, 403, "", "", ""},
		{"a visitor is refused", visitor, url.Values{"as": {"guest"}}, nil, 403, "", "", ""},
		{"bad as", owner, url.Values{"as": {"admin"}}, nil, 400, "", "", ""},
		{"missing as", owner, url.Values{}, nil, 400, "", "", ""},
		{"bad next", owner, url.Values{"as": {"self"}, "next": {"https://evil.test"}}, nil, 400, "", "", ""},
		{"arrange needs as=self", owner, url.Values{"as": {"player"}, "next": {"arrange"}, "home": {"player"}}, nil, 400, "", "", ""},
		{"arrange needs a home", owner, url.Values{"as": {"self"}, "next": {"arrange"}}, nil, 400, "", "", ""},
		{"arrange home is an enum", owner, url.Values{"as": {"self"}, "next": {"arrange"}, "home": {"director"}}, nil, 400, "", "", ""},
		{"home without arrange", owner, url.Values{"as": {"self"}, "next": {"home"}, "home": {"player"}}, nil, 400, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := viewAsCall(t, tt.cc, tt.form, tt.cookies, false)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantLocation != "" && rec.Header().Get("Location") != tt.wantLocation {
				t.Errorf("location = %q, want %q", rec.Header().Get("Location"), tt.wantLocation)
			}
			p, g := cookieStates(rec)
			if p != tt.wantPlayer || g != tt.wantGuest {
				t.Errorf("cookies player=%q guest=%q, want %q %q", p, g, tt.wantPlayer, tt.wantGuest)
			}
		})
	}

	t.Run("no next: HTMX refreshes", func(t *testing.T) {
		rec := viewAsCall(t, owner, url.Values{"as": {"guest"}}, nil, true)
		if rec.Code != 204 || rec.Header().Get("HX-Refresh") != "true" {
			t.Errorf("status %d, HX-Refresh %q", rec.Code, rec.Header().Get("HX-Refresh"))
		}
	})
	t.Run("next=home: HTMX redirects", func(t *testing.T) {
		rec := viewAsCall(t, owner, url.Values{"as": {"player"}, "next": {"home"}}, nil, true)
		if rec.Header().Get("HX-Redirect") != "/campaigns/camp-1" {
			t.Errorf("HX-Redirect = %q", rec.Header().Get("HX-Redirect"))
		}
	})
	t.Run("guest cookie is server-only and SameSite Lax", func(t *testing.T) {
		rec := viewAsCall(t, owner, url.Values{"as": {"guest"}}, nil, false)
		for _, ck := range rec.Result().Cookies() {
			if ck.Name == viewAsGuestCookie && (!ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.MaxAge != 0) {
				t.Errorf("guest cookie = %+v", ck)
			}
		}
	})
}

func TestSameCampaignReferer(t *testing.T) {
	tests := []struct{ ref, want string }{
		{"http://example.test/campaigns/camp-1/entities?x=1", "/campaigns/camp-1/entities?x=1"},
		{"http://example.test/campaigns/camp-1", "/campaigns/camp-1"},
		{"http://evil.test/campaigns/camp-1/x", "/campaigns/camp-1"},
		{"http://example.test/campaigns/camp-10/x", "/campaigns/camp-1"},
		{"http://example.test/elsewhere", "/campaigns/camp-1"},
		{"", "/campaigns/camp-1"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodPost, "http://example.test/x", nil)
		if tt.ref != "" {
			req.Header.Set("Referer", tt.ref)
		}
		if got := sameCampaignReferer(req, "camp-1"); got != tt.want {
			t.Errorf("referer %q -> %q, want %q", tt.ref, got, tt.want)
		}
	}
}

func TestToggleViewAsPlayer_ClearsEitherPreview(t *testing.T) {
	pub := &Campaign{ID: "camp-1", IsPublic: true}
	priv := &Campaign{ID: "camp-1"}
	owner := func(c *Campaign) *CampaignContext {
		return &CampaignContext{Campaign: c, MemberRole: RoleOwner, IsMember: true}
	}
	pCookie := &http.Cookie{Name: viewAsPlayerCookie, Value: "camp-1"}
	gCookie := &http.Cookie{Name: viewAsGuestCookie, Value: "camp-1"}
	tests := []struct {
		name       string
		campaign   *Campaign
		cookies    []*http.Cookie
		wantPlayer string
		wantGuest  string
	}{
		{"nothing on: starts the player preview", pub, nil, "on", ""},
		{"player on: off", pub, []*http.Cookie{pCookie}, "off", ""},
		{"guest on: off", pub, []*http.Cookie{gCookie}, "", "off"},
		{"both stale: both off", pub, []*http.Cookie{pCookie, gCookie}, "off", "off"},
		{"guest cookie on a private campaign does nothing there: starts player, clears guest", priv, []*http.Cookie{gCookie}, "on", "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/toggle-view-mode", nil)
			for _, ck := range tt.cookies {
				req.AddCookie(ck)
			}
			req.Header.Set("HX-Request", "true")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(contextKeyCampaign, owner(tt.campaign))
			if err := (&Handler{}).ToggleViewAsPlayer(c); err != nil {
				t.Fatal(err)
			}
			p, g := cookieStates(rec)
			if p != tt.wantPlayer || g != tt.wantGuest {
				t.Errorf("player=%q guest=%q, want %q %q", p, g, tt.wantPlayer, tt.wantGuest)
			}
		})
	}
}

// The owner's Settings link in the Home title is the owner's own control: it
// is not drawn under either preview, with a picture or without.
func TestHomeTitle_SettingsLinkAbsentUnderPreviews(t *testing.T) {
	pic := "backdrop.png"
	for _, withPicture := range []bool{false, true} {
		camp := &Campaign{ID: "camp-1", Name: "Test"}
		if withPicture {
			camp.BackdropPath = &pic
		}
		owner := &CampaignContext{Campaign: camp, MemberRole: RoleOwner, IsMember: true}
		tests := []struct {
			name string
			ctx  context.Context
			cc   *CampaignContext
			want bool
		}{
			{"owner", context.Background(), owner, true},
			{"player preview", layouts.SetViewingAsPlayer(context.Background(), true), owner, false},
			{"guest preview", layouts.SetPreviewAs(context.Background(), PreviewGuest), owner.GuestPreviewContext(), false},
		}
		for _, tt := range tests {
			out := renderCardWithCtx(t, tt.ctx, dashWelcomeBanner(tt.cc))
			if got := strings.Contains(out, "/campaigns/camp-1/settings"); got != tt.want {
				t.Errorf("%s (picture=%v): settings link present = %v, want %v", tt.name, withPicture, got, tt.want)
			}
		}
	}
}

// The "Dashboard for" line is the DM team's, in all three states, and nobody
// else's.
func TestShowPage_DashboardForLine(t *testing.T) {
	camp := &Campaign{ID: "camp-1", Name: "Test", IsPublic: true}
	render := func(cc *CampaignContext, ctx context.Context, guestPrivate bool) string {
		var sb strings.Builder
		if err := CampaignShowPage(cc, nil, nil, "tok", guestPrivate).Render(ctx, &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	owner := &CampaignContext{Campaign: camp, MemberRole: RoleOwner, IsMember: true}
	coDM := &CampaignContext{Campaign: camp, MemberRole: RolePlayer, IsMember: true, IsDmGranted: true}
	player := &CampaignContext{Campaign: camp, MemberRole: RolePlayer, IsMember: true}
	visitor := &CampaignContext{Campaign: camp}

	for name, cc := range map[string]*CampaignContext{"player": player, "visitor": visitor} {
		if strings.Contains(render(cc, context.Background(), false), "Dashboard for") {
			t.Errorf("a %s must not see the line", name)
		}
	}
	if out := render(owner, context.Background(), false); !strings.Contains(out, "Dashboard for") || !strings.Contains(out, `aria-pressed="true"`) || strings.Contains(out, "dashboard, as a") {
		t.Errorf("owner's own view: line, no note")
	}
	gp := owner.GuestPreviewContext()
	out := render(gp, layouts.SetPreviewAs(context.Background(), PreviewGuest), false)
	for _, want := range []string{"Dashboard for", "guests&#39; dashboard, as a visitor sees it", "Arrange this page", `value="guest"`} {
		if !strings.Contains(out, want) && !strings.Contains(out, strings.ReplaceAll(want, "&#39;", "'")) {
			t.Errorf("guest preview missing %q", want)
		}
	}
	if out := render(coDM.GuestPreviewContext(), context.Background(), false); strings.Contains(out, "Arrange this page") || !strings.Contains(out, "Dashboard for") {
		t.Errorf("a co-DM's guest preview has the line but no arrange link")
	}
	priv := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Test"}, MemberRole: RoleOwner, IsMember: true}
	out = render(priv, context.Background(), true)
	for _, want := range []string{"so only guests with a guest code can open it", "See it as a guest with a code", `value="guestcode"`, "Who can read it", "/campaigns/camp-1/dashboard#line-public", "Arrange this page"} {
		if !strings.Contains(out, want) && !strings.Contains(out, strings.ReplaceAll(want, "&#39;", "'")) {
			t.Errorf("private guest note missing %q", want)
		}
	}
}

// Only the DM team's own request for a private campaign's guests' dashboard
// gets the note, and a player preview in force wins over it.
func TestGuestsNoteAsked(t *testing.T) {
	pub := &Campaign{ID: "camp-1", IsPublic: true}
	priv := &Campaign{ID: "camp-1"}
	owner := func(c *Campaign) *CampaignContext {
		return &CampaignContext{Campaign: c, MemberRole: RoleOwner, IsMember: true}
	}
	tests := []struct {
		name   string
		cc     *CampaignContext
		query  string
		player bool
		want   bool
	}{
		{"owner, private, asked", owner(priv), "?dashboard=guests", false, true},
		{"owner, private, not asked", owner(priv), "", false, false},
		{"owner, private, another value", owner(priv), "?dashboard=players", false, false},
		{"owner, public: the real preview is the cookie's job", owner(pub), "?dashboard=guests", false, false},
		{"owner, private, player preview on", owner(priv), "?dashboard=guests", true, false},
		{"co-DM, private, asked", &CampaignContext{Campaign: priv, MemberRole: RolePlayer, IsMember: true, IsDmGranted: true}, "?dashboard=guests", false, true},
		{"player, private, asked", &CampaignContext{Campaign: priv, MemberRole: RolePlayer, IsMember: true}, "?dashboard=guests", false, false},
		{"no context", nil, "?dashboard=guests", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1"+tt.query, nil)
			if tt.player {
				req.AddCookie(&http.Cookie{Name: viewAsPlayerCookie, Value: "camp-1"})
			}
			c := echo.New().NewContext(req, httptest.NewRecorder())
			if got := GuestsNoteAsked(c, tt.cc); got != tt.want {
				t.Errorf("GuestsNoteAsked = %v, want %v", got, tt.want)
			}
		})
	}
}
