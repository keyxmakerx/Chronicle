package foundry_vtt

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// fakeUpdateService is the slice of the packages update service the owner flow
// uses, recording what it was asked to do.
type fakeUpdateService struct {
	packages.CampaignUpdateService
	state    packages.CampaignPackageState
	versions []string
	err      error

	approved  string
	dismissed string
	switched  string
	gotActor  packages.ActorInfo
}

func (f *fakeUpdateService) CampaignState(context.Context, string, string) (*packages.CampaignPackageState, error) {
	st := f.state
	return &st, f.err
}
func (f *fakeUpdateService) InstalledVersions(context.Context, string) ([]string, error) {
	return f.versions, nil
}
func (f *fakeUpdateService) ApproveVersion(_ context.Context, _, _, v string, a packages.ActorInfo) (*packages.CampaignPackageState, error) {
	f.approved, f.gotActor = v, a
	return nil, f.err
}
func (f *fakeUpdateService) DismissHeld(_ context.Context, _, _, v string) (*packages.CampaignPackageState, error) {
	f.dismissed = v
	return nil, f.err
}
func (f *fakeUpdateService) SwitchVersion(_ context.Context, _, _, v string, a packages.ActorInfo) (*packages.CampaignPackageState, error) {
	f.switched, f.gotActor = v, a
	return nil, f.err
}

// versionsReader serves one module package with release notes.
type versionsReader struct {
	stubPackageReader
	versions []packages.PackageVersion
}

func (r *versionsReader) ListVersions(context.Context, string) ([]packages.PackageVersion, error) {
	return r.versions, nil
}

func newOwnerFlow(st packages.CampaignPackageState, notes ...packages.PackageVersion) (OwnerUpdates, *fakeUpdateService) {
	upd := &fakeUpdateService{state: st, versions: []string{"2.9.0", "2.8.0"}}
	rd := &versionsReader{
		stubPackageReader: stubPackageReader{pkgs: []packages.Package{{
			ID: "m1", Type: packages.PackageTypeFoundryModule, Name: "Chronicle Sync",
			InstalledVersion: "2.9.0", Status: packages.StatusApproved,
		}}},
		versions: notes,
	}
	return NewOwnerUpdates(upd, rd), upd
}

func TestOwnerUpdatesView(t *testing.T) {
	notes := packages.PackageVersion{
		Version: "2.9.0", ReleaseURL: "https://github.com/x/y/releases/tag/2.9.0",
		ReleaseNotes: "## Changes\n- Calendar notice\n- Safer deletes\n",
	}
	cases := []struct {
		name  string
		state packages.CampaignPackageState
		notes []packages.PackageVersion
		want  OwnerUpdateView
	}{
		{
			name:  "up to date",
			state: packages.CampaignPackageState{EffectiveVersion: "2.9.0"},
			want:  OwnerUpdateView{CampaignID: "c1", Package: "Chronicle Sync", Running: "2.9.0", Versions: []string{"2.9.0", "2.8.0"}},
		},
		{
			name:  "ready, with notes",
			state: packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0"},
			notes: []packages.PackageVersion{notes},
			want: OwnerUpdateView{CampaignID: "c1", Package: "Chronicle Sync", Running: "2.8.0", Ready: "2.9.0",
				Notes: []string{"Changes", "Calendar notice", "Safer deletes"}, NotesURL: notes.ReleaseURL,
				Versions: []string{"2.9.0", "2.8.0"}},
		},
		{
			name:  "ready, no notes known",
			state: packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0"},
			want:  OwnerUpdateView{CampaignID: "c1", Package: "Chronicle Sync", Running: "2.8.0", Ready: "2.9.0", Versions: []string{"2.9.0", "2.8.0"}},
		},
		{
			name:  "later pressed for this version",
			state: packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0", DismissedVersion: "2.9.0"},
			want:  OwnerUpdateView{CampaignID: "c1", Package: "Chronicle Sync", Running: "2.8.0", Ready: "2.9.0", Dismissed: true, Versions: []string{"2.9.0", "2.8.0"}},
		},
		{
			name:  "later pressed for an older version asks again",
			state: packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0", DismissedVersion: "2.8.5"},
			want:  OwnerUpdateView{CampaignID: "c1", Package: "Chronicle Sync", Running: "2.8.0", Ready: "2.9.0", Versions: []string{"2.9.0", "2.8.0"}},
		},
		{
			name:  "held by the admin offers no picker",
			state: packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0", AdminHold: true},
			want:  OwnerUpdateView{CampaignID: "c1", Package: "Chronicle Sync", Running: "2.8.0", Ready: "2.9.0", AdminHold: true},
		},
		{
			name:  "a non-web release link is not offered",
			state: packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0"},
			notes: []packages.PackageVersion{{Version: "2.9.0", ReleaseURL: "javascript:alert(1)"}},
			want:  OwnerUpdateView{CampaignID: "c1", Package: "Chronicle Sync", Running: "2.8.0", Ready: "2.9.0", Versions: []string{"2.9.0", "2.8.0"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, _ := newOwnerFlow(tc.state, tc.notes...)
			got, err := o.View(context.Background(), "c1")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, tc.want) {
				t.Errorf("view =\n%+v\nwant\n%+v", *got, tc.want)
			}
		})
	}
}

func TestOwnerUpdatesNoModule(t *testing.T) {
	cases := []struct {
		name string
		pkgs []packages.Package
	}{
		{"none registered", nil},
		{"registered, nothing installed", []packages.Package{{ID: "m1", Type: packages.PackageTypeFoundryModule, Status: packages.StatusApproved}}},
		{"installed but not approved", []packages.Package{{ID: "m1", Type: packages.PackageTypeFoundryModule, InstalledVersion: "1", Status: packages.StatusArchived}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := NewOwnerUpdates(&fakeUpdateService{}, &stubPackageReader{pkgs: tc.pkgs})
			v, err := o.View(context.Background(), "c1")
			if err != nil || v != nil {
				t.Errorf("View = %v, %v; want nil, nil", v, err)
			}
			if err := o.Update(context.Background(), "c1", "2.9.0", packages.ActorInfo{}); err == nil {
				t.Error("Update with no module must fail")
			}
		})
	}
}

func TestOwnerUpdatesActionsPassTheVersionThrough(t *testing.T) {
	o, upd := newOwnerFlow(packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0"})
	actor := packages.ActorInfo{UserID: "u1"}
	if err := o.Update(context.Background(), "c1", "2.9.0", actor); err != nil || upd.approved != "2.9.0" || upd.gotActor.Admin {
		t.Errorf("Update: err %v, approved %q, actor %+v", err, upd.approved, upd.gotActor)
	}
	if err := o.Later(context.Background(), "c1", "2.9.0"); err != nil || upd.dismissed != "2.9.0" {
		t.Errorf("Later: err %v, dismissed %q", err, upd.dismissed)
	}
	if err := o.Switch(context.Background(), "c1", "2.8.0", actor); err != nil || upd.switched != "2.8.0" {
		t.Errorf("Switch: err %v, switched %q", err, upd.switched)
	}
	upd.err = errors.New("boom")
	if err := o.Update(context.Background(), "c1", "2.9.0", actor); err == nil {
		t.Error("a service error must reach the handler")
	}
}

// --- handlers ---

type handlerEnv struct {
	h   *Handler
	upd *fakeUpdateService
}

func newHandlerEnv(st packages.CampaignPackageState) handlerEnv {
	o, upd := newOwnerFlow(st, packages.PackageVersion{Version: "2.9.0", ReleaseNotes: "- Calendar notice"})
	h := NewHandler(nil)
	h.SetOwnerUpdates(o)
	return handlerEnv{h: h, upd: upd}
}

func (e handlerEnv) call(t *testing.T, method, target string, role campaigns.Role, form url.Values, fn func(echo.Context) error) (*httptest.ResponseRecorder, error) {
	t.Helper()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "c1", Name: "Shattered Coast"},
		MemberRole: role,
	})
	auth.SetSession(c, &auth.Session{UserID: "u1"})
	return rec, fn(c)
}

var waiting = packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0"}

func TestPlayersNeverGetTheUpdateScreensOrActions(t *testing.T) {
	e := newHandlerEnv(waiting)
	form := url.Values{"version": {"2.9.0"}, "view": {"banner"}}
	cases := []struct {
		name string
		fn   func(echo.Context) error
		post bool
	}{
		{"banner", e.h.CampaignShowBannerHandler, false},
		{"apps row", e.h.AppsUpdateRowHandler, false},
		{"update", e.h.OwnerUpdateHandler, true},
		{"later", e.h.OwnerUpdateLaterHandler, true},
		{"switch", e.h.OwnerUpdateSwitchHandler, true},
	}
	for _, role := range []campaigns.Role{campaigns.RoleNone, campaigns.RolePlayer, campaigns.RoleScribe} {
		for _, tc := range cases {
			method, f := http.MethodGet, url.Values(nil)
			if tc.post {
				method, f = http.MethodPost, form
			}
			rec, err := e.call(t, method, "/x", role, f, tc.fn)
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
				t.Errorf("%s as role %d: err = %v, want forbidden", tc.name, role, err)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("%s as role %d leaked a body: %q", tc.name, role, rec.Body.String())
			}
		}
	}
	if e.upd.approved != "" || e.upd.dismissed != "" || e.upd.switched != "" {
		t.Error("a player's request must not reach the update service")
	}
}

func TestOwnerBannerHandler(t *testing.T) {
	cases := []struct {
		name     string
		state    packages.CampaignPackageState
		contains []string
		empty    bool
	}{
		{"waiting", waiting, []string{"Chronicle Sync 2.9.0 is ready for this campaign.", "You&rsquo;re on 2.8.0", ">Update<", ">Later<", "Calendar notice"}, false},
		{"up to date", packages.CampaignPackageState{EffectiveVersion: "2.9.0"}, nil, true},
		{"held by the admin", packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0", AdminHold: true}, nil, true},
		{"later pressed", packages.CampaignPackageState{EffectiveVersion: "2.8.0", HeldVersion: "2.9.0", DismissedVersion: "2.9.0"},
			[]string{"is waiting in", "/campaigns/c1/extensions", "Only you see this."}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newHandlerEnv(tc.state)
			rec, err := e.call(t, http.MethodGet, "/x", campaigns.RoleOwner, nil, e.h.CampaignShowBannerHandler)
			if err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			if tc.empty && strings.TrimSpace(body) != "" {
				t.Errorf("expected nothing, got %q", body)
			}
			for _, want := range tc.contains {
				if !strings.Contains(body, want) {
					t.Errorf("missing %q in\n%s", want, body)
				}
			}
			if tc.name == "later pressed" && strings.Contains(body, ">Later<") {
				t.Error("the prompt must stay hidden after Later")
			}
		})
	}
}

func TestOwnerActionsHandlers(t *testing.T) {
	e := newHandlerEnv(packages.CampaignPackageState{EffectiveVersion: "2.9.0"})
	form := url.Values{"version": {"2.9.0"}, "view": {"banner"}}

	rec, err := e.call(t, http.MethodPost, "/x", campaigns.RoleOwner, form, e.h.OwnerUpdateHandler)
	if err != nil || e.upd.approved != "2.9.0" || e.upd.gotActor.UserID != "u1" || e.upd.gotActor.Admin {
		t.Fatalf("update: err %v approved %q actor %+v", err, e.upd.approved, e.upd.gotActor)
	}
	if !strings.Contains(rec.Body.String(), "Updated to Chronicle Sync 2.9.0.") {
		t.Errorf("update must answer with the result line, got %q", rec.Body.String())
	}

	rec, err = e.call(t, http.MethodPost, "/x", campaigns.RoleOwner, url.Values{"version": {"2.8.0"}, "view": {"row"}}, e.h.OwnerUpdateSwitchHandler)
	if err != nil || e.upd.switched != "2.8.0" {
		t.Fatalf("switch: err %v switched %q", err, e.upd.switched)
	}
	if !strings.Contains(rec.Body.String(), `id="fvtt-update-row"`) {
		t.Errorf("a switch from the row must answer with the row, got %q", rec.Body.String())
	}

	// A refused action (for example an admin hold) is an error, not a quiet 200.
	e.upd.err = apperror.NewForbidden("The site admin is keeping this campaign on 2.8.0.")
	_, err = e.call(t, http.MethodPost, "/x", campaigns.RoleOwner, form, e.h.OwnerUpdateHandler)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
		t.Errorf("a refused update must surface as forbidden, got %v", err)
	}
}

func TestUpdateHandlersWithoutTheUpdateFlow(t *testing.T) {
	h := NewHandler(nil)
	e := handlerEnv{h: h}
	_, err := e.call(t, http.MethodGet, "/x", campaigns.RoleOwner, nil, h.CampaignShowBannerHandler)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Errorf("without the flow wired: err = %v, want not found", err)
	}
}
