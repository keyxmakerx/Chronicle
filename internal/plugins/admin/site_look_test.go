package admin

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/sitelook"
)

// fakeSiteSettings is an in-memory SiteLookSettings.
type fakeSiteSettings struct {
	cur     sitelook.Settings
	updates int
	getErr  error
}

func (f *fakeSiteSettings) GetSiteLook(context.Context) (sitelook.Settings, error) {
	return f.cur, f.getErr
}

func (f *fakeSiteSettings) UpdateSiteLook(_ context.Context, in sitelook.Settings) (sitelook.Settings, error) {
	v, err := sitelook.Validate(in)
	if err != nil {
		return sitelook.Settings{}, err
	}
	f.cur, f.updates = v, f.updates+1
	return v, nil
}

// fakeSiteMedia records what the service stores and deletes.
type fakeSiteMedia struct {
	next    int
	stored  []string
	deleted []string
	owned   map[string]bool
	failPut bool
}

func (m *fakeSiteMedia) StoreSitePicture(_ context.Context, _ string, _ []byte, _, mime string) (string, error) {
	if m.failPut {
		return "", errors.New("disk full")
	}
	m.next++
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}[mime]
	name := "2026/10/f" + string(rune('0'+m.next)) + ext
	m.stored = append(m.stored, name)
	return name, nil
}

func (m *fakeSiteMedia) OwnsSitePicture(_ context.Context, f string) (bool, error) {
	return m.owned[f], nil
}

func (m *fakeSiteMedia) DeleteSitePicture(_ context.Context, f string) error {
	m.deleted = append(m.deleted, f)
	return nil
}

func pngBytes(w, h int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)))
	return b.Bytes()
}

func TestSiteLookService_Save(t *testing.T) {
	saved := sitelook.Settings{Configured: true, Logo: "2026/10/old.png", Background: sitelook.BackgroundPicture, Picture: "2026/10/oldbg.png", Look: "ember"}
	tests := []struct {
		name        string
		cur         sitelook.Settings
		sub         SiteLookSubmission
		failPut     bool
		wantErr     bool
		wantLogo    string
		wantPicture string
		wantDeleted []string
		wantStored  int
	}{
		{
			name:     "text only keeps the saved pictures",
			cur:      saved,
			sub:      SiteLookSubmission{Draft: sitelook.Settings{Name: "Hold", Look: "ember", Background: sitelook.BackgroundPicture}},
			wantLogo: "2026/10/old.png", wantPicture: "2026/10/oldbg.png",
		},
		{
			name:     "new logo replaces and deletes the old one",
			cur:      saved,
			sub:      SiteLookSubmission{Draft: sitelook.Settings{Look: "ember", Background: sitelook.BackgroundPicture}, Logo: &SiteLookUpload{Name: "l.png", Data: pngBytes(64, 64)}},
			wantLogo: "2026/10/f1.png", wantPicture: "2026/10/oldbg.png",
			wantDeleted: []string{"2026/10/old.png"}, wantStored: 1,
		},
		{
			name:     "remove logo deletes the file",
			cur:      saved,
			sub:      SiteLookSubmission{Draft: sitelook.Settings{Look: "ember", Background: sitelook.BackgroundPicture}, RemoveLogo: true},
			wantLogo: "", wantPicture: "2026/10/oldbg.png", wantDeleted: []string{"2026/10/old.png"},
		},
		{
			name:    "wide logo refused, nothing stored",
			cur:     sitelook.Settings{},
			sub:     SiteLookSubmission{Logo: &SiteLookUpload{Name: "l.png", Data: pngBytes(300, 50)}},
			wantErr: true,
		},
		{
			name:    "svg refused",
			cur:     sitelook.Settings{},
			sub:     SiteLookSubmission{Logo: &SiteLookUpload{Name: "l.svg", Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`)}},
			wantErr: true,
		},
		{
			name:    "storage failure surfaces",
			cur:     sitelook.Settings{},
			sub:     SiteLookSubmission{Logo: &SiteLookUpload{Name: "l.png", Data: pngBytes(64, 64)}},
			failPut: true, wantErr: true,
		},
		{
			name: "picture upload with plain background is tidied away",
			cur:  sitelook.Settings{},
			sub: SiteLookSubmission{Draft: sitelook.Settings{Background: sitelook.BackgroundPlain},
				Picture: &SiteLookUpload{Name: "b.png", Data: pngBytes(300, 100)}},
			wantStored: 1, wantDeleted: []string{"2026/10/f1.png"},
		},
		{
			name: "picture background with a wide upload",
			cur:  sitelook.Settings{},
			sub: SiteLookSubmission{Draft: sitelook.Settings{Background: sitelook.BackgroundPicture},
				Picture: &SiteLookUpload{Name: "b.png", Data: pngBytes(300, 100)}},
			wantPicture: "2026/10/f1.png", wantStored: 1,
		},
		{
			name: "invalid text after an upload discards the upload",
			cur:  sitelook.Settings{},
			sub: SiteLookSubmission{Draft: sitelook.Settings{Name: strings.Repeat("x", 41)},
				Logo: &SiteLookUpload{Name: "l.png", Data: pngBytes(64, 64)}},
			wantErr: true, wantStored: 1, wantDeleted: []string{"2026/10/f1.png"},
		},
		{
			name:    "picture background without a picture refused",
			cur:     sitelook.Settings{},
			sub:     SiteLookSubmission{Draft: sitelook.Settings{Background: sitelook.BackgroundPicture}},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			set := &fakeSiteSettings{cur: tc.cur}
			med := &fakeSiteMedia{failPut: tc.failPut}
			svc := NewSiteLookService(set, med)
			got, err := svc.Save(context.Background(), "admin-1", tc.sub)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if len(med.stored) != tc.wantStored {
				t.Errorf("stored %v, want %d file(s)", med.stored, tc.wantStored)
			}
			if strings.Join(med.deleted, ",") != strings.Join(tc.wantDeleted, ",") {
				t.Errorf("deleted %v, want %v", med.deleted, tc.wantDeleted)
			}
			if tc.wantErr {
				if set.updates != 0 {
					t.Errorf("a refused save still wrote settings")
				}
				return
			}
			if got.Logo != tc.wantLogo || got.Picture != tc.wantPicture {
				t.Errorf("logo/picture = %q/%q, want %q/%q", got.Logo, got.Picture, tc.wantLogo, tc.wantPicture)
			}
		})
	}
}

func TestSiteLookService_NoMediaRefusesUploadsOnly(t *testing.T) {
	set := &fakeSiteSettings{}
	svc := NewSiteLookService(set, nil)
	if _, err := svc.Save(context.Background(), "a", SiteLookSubmission{Draft: sitelook.Settings{Name: "Hold"}}); err != nil {
		t.Fatalf("text save without media: %v", err)
	}
	_, err := svc.Save(context.Background(), "a", SiteLookSubmission{Logo: &SiteLookUpload{Name: "l.png", Data: pngBytes(64, 64)}})
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
		t.Errorf("upload without media: err = %v, want a 400", err)
	}
}

// sessionAuth is an AuthService that only knows two session tokens.
type sessionAuth struct{ auth.AuthService }

func (sessionAuth) ValidateSession(_ context.Context, token string) (*auth.Session, error) {
	switch token {
	case "admin-token":
		return &auth.Session{UserID: "u-admin", Name: "Admin", IsAdmin: true}, nil
	case "player-token":
		return &auth.Session{UserID: "u-player", Name: "Player"}, nil
	}
	return nil, errors.New("no such session")
}

// siteLookTestServer wires the real admin routes (the same auth chain as
// production) to a handler backed by fakes.
func siteLookTestServer(set *fakeSiteSettings, med *fakeSiteMedia) *echo.Echo {
	e := echo.New()
	h := NewHandler(nil, nil, nil)
	h.SetSiteLookService(NewSiteLookService(set, med))
	RegisterRoutes(e, h, sessionAuth{}, nil)
	return e
}

func doReq(e *echo.Echo, method, path, token, contentType string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: token})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSiteLookRoutes_AdminOnly(t *testing.T) {
	form := url.Values{"name": {"Hold"}, "look": {"ember"}, "background": {"look"}, "move": {"1"}}.Encode()
	tests := []struct {
		name       string
		method     string
		token      string
		wantStatus int
		wantWrite  bool
	}{
		{"visitor GET is sent to sign in", "GET", "", http.StatusSeeOther, false},
		{"visitor POST is sent to sign in", "POST", "", http.StatusSeeOther, false},
		{"player GET is forbidden", "GET", "player-token", http.StatusForbidden, false},
		{"player POST is forbidden and writes nothing", "POST", "player-token", http.StatusForbidden, false},
		{"stale cookie is sent to sign in", "POST", "bogus", http.StatusSeeOther, false},
		{"admin GET shows the page", "GET", "admin-token", http.StatusOK, false},
		{"admin POST saves and redirects", "POST", "admin-token", http.StatusSeeOther, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			set := &fakeSiteSettings{}
			e := siteLookTestServer(set, &fakeSiteMedia{})
			var body []byte
			ct := ""
			if tc.method == "POST" {
				body, ct = []byte(form), "application/x-www-form-urlencoded"
			}
			rec := doReq(e, tc.method, "/admin/site-look", tc.token, ct, body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %.300s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if (set.updates > 0) != tc.wantWrite {
				t.Errorf("settings written = %v, want %v", set.updates > 0, tc.wantWrite)
			}
			if tc.wantWrite && (set.cur.Name != "Hold" || set.cur.Look != "ember" || set.cur.Background != "look" || !set.cur.Move) {
				t.Errorf("saved %+v", set.cur)
			}
		})
	}
}

func multipartBody(t *testing.T, fields map[string]string, file, filename string, data []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if file != "" {
		fw, _ := w.CreateFormFile(file, filename)
		_, _ = fw.Write(data)
	}
	_ = w.Close()
	return buf.Bytes(), w.FormDataContentType()
}

func TestSaveSiteLook_Upload(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		data       []byte
		fields     map[string]string
		wantStatus int
		wantInBody string
		wantLogo   string
	}{
		{"square logo saves", "logo_file", pngBytes(48, 48), map[string]string{"name": "Hold"}, http.StatusSeeOther, "", "2026/10/f1.png"},
		{"wide logo is explained", "logo_file", pngBytes(300, 40), map[string]string{"name": "Hold"}, http.StatusBadRequest, "square or close to it", ""},
		{"html pretending to be a logo", "logo_file", []byte("<script>alert(1)</script>"), map[string]string{"name": "Hold"}, http.StatusBadRequest, "PNG, JPEG or WebP", ""},
		{"rejected name keeps the form and says why", "", nil, map[string]string{"name": strings.Repeat("n", 41)}, http.StatusBadRequest, "40 characters", ""},
		{"welcome is escaped when the form comes back", "", nil, map[string]string{"name": strings.Repeat("n", 41), "welcome": `"><script>alert(1)</script>`}, http.StatusBadRequest, "&lt;script&gt;", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			set := &fakeSiteSettings{}
			e := siteLookTestServer(set, &fakeSiteMedia{})
			body, ct := multipartBody(t, tc.fields, tc.file, "x.png", tc.data)
			rec := doReq(e, "POST", "/admin/site-look", "admin-token", ct, body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantInBody != "" && !strings.Contains(rec.Body.String(), tc.wantInBody) {
				t.Errorf("body missing %q", tc.wantInBody)
			}
			if strings.Contains(rec.Body.String(), "<script>alert(1)</script>") {
				t.Errorf("unescaped script reached the page")
			}
			if set.cur.Logo != tc.wantLogo {
				t.Errorf("saved logo = %q, want %q", set.cur.Logo, tc.wantLogo)
			}
		})
	}
}

func TestSiteLookPage_RendersSavedValues(t *testing.T) {
	set := &fakeSiteSettings{cur: sitelook.Settings{Configured: true, Name: "Dragon <Hold>", Look: "arcane", Background: sitelook.BackgroundLook, Welcome: "Hi & welcome", LogoAsFavicon: true}}
	rec := doReq(siteLookTestServer(set, &fakeSiteMedia{}), "GET", "/admin/site-look", "admin-token", "", nil)
	body := rec.Body.String()
	for _, want := range []string{
		`Dragon &lt;Hold&gt;`, `value="arcane" checked`, `value="look" checked`, `Hi &amp; welcome`,
		`name="favicon" value="1" checked`, `maxlength="40"`, `maxlength="80"`, `Site look`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if strings.Contains(body, "Dragon <Hold>") {
		t.Error("site name rendered unescaped")
	}
}

func TestSiteLookTile(t *testing.T) {
	tests := []struct {
		name     string
		in       homeInput
		wantBig  string
		wantLine string
	}{
		{"unreadable", homeInput{}, "—", "name, logo and look outside campaigns"},
		{"never saved", homeInput{SiteName: "Chronicle"}, "Chronicle", "not set up yet, pages look as shipped"},
		{"saved", homeInput{SiteName: "Dragon Hold", SiteLookSaved: true}, "Dragon Hold", "name, logo and look outside campaigns"},
	}
	for _, tc := range tests {
		got := siteLookTile(tc.in)
		if got.Big != tc.wantBig || got.Line != tc.wantLine || got.Href != "/admin/site-look" {
			t.Errorf("%s: got %+v", tc.name, got)
		}
	}
	site := findTile(buildHomeGroups(homeInput{}), "Site look")
	if site.Href != "/admin/site-look" {
		t.Errorf("the Site group has no Site look tile: %+v", site)
	}
}

func TestSaveSiteLook_Move(t *testing.T) {
	tests := []struct {
		name string
		form url.Values
		want bool
	}{
		{"ticked over the look's colours", url.Values{"look": {"ember"}, "background": {"look"}, "move": {"1"}}, true},
		{"unticked is off", url.Values{"look": {"ember"}, "background": {"look"}}, false},
		{"ticked over plain is dropped", url.Values{"background": {"plain"}, "move": {"1"}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			set := &fakeSiteSettings{}
			rec := doReq(siteLookTestServer(set, &fakeSiteMedia{}), "POST", "/admin/site-look", "admin-token", "application/x-www-form-urlencoded", []byte(tc.form.Encode()))
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d", rec.Code)
			}
			if set.cur.Move != tc.want {
				t.Errorf("Move = %v, want %v", set.cur.Move, tc.want)
			}
		})
	}
}

// TestSiteLookPage_PreviewContract checks the markup the site-look widget
// relies on: the mount, the preview window, the panels it focuses, and the
// look colours rendered from the Go table into data attributes.
func TestSiteLookPage_PreviewContract(t *testing.T) {
	set := &fakeSiteSettings{cur: sitelook.Settings{Configured: true, Name: "Hold", Look: "ember", Background: sitelook.BackgroundLook, Move: true}}
	body := doReq(siteLookTestServer(set, &fakeSiteMedia{}), "GET", "/admin/site-look", "admin-token", "", nil).Body.String()
	for _, want := range []string{
		`data-widget="site-look"`, `data-sl-win`, `data-sl-switcher hidden`,
		`data-sl-view="signin"`, `data-sl-view="disc"`, `data-sl-view="tabs"`,
		`data-sl-focus="sl-p-id"`, `data-sl-focus="sl-p-look"`, `data-sl-focus="sl-p-bg"`,
		`id="sl-p-id"`, `id="sl-p-look"`, `id="sl-p-bg"`,
		`data-accent="#c2410c" data-from="#1f2937" data-to="#4a1512"`, `data-sl-moving="on"`, `data-sl-look="on"`,
		`name="move" value="1" checked`, `--sa:#c2410c;--hd1:#1f2937;--hd2:#4a1512`, `sl-bg-look`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	// Plain background: the box is disabled and the preview is not moving.
	set.cur = sitelook.Settings{Configured: true, Background: sitelook.BackgroundPlain}
	body = doReq(siteLookTestServer(set, &fakeSiteMedia{}), "GET", "/admin/site-look", "admin-token", "", nil).Body.String()
	if !strings.Contains(body, `name="move" value="1" disabled`) || !strings.Contains(body, `data-sl-moving="off"`) {
		t.Error("over a plain background the Move box should be disabled and the preview still")
	}
}
