package packages

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

func TestRoutesGuardCampaignOverrides(t *testing.T) {
	e := echo.New()
	reauth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return c.NoContent(http.StatusForbidden) }
	}
	RegisterRoutes(e.Group("/admin"), nil, reauth)

	// Hold, release and move override what an owner chose and need the
	// password again; a reminder changes nothing, so the stub never sees it
	// (a nil handler would panic, which is how this test would notice).
	for _, tt := range []struct{ method, path string }{
		{http.MethodPost, "/admin/packages/p1/campaigns/c1/hold"},
		{http.MethodDelete, "/admin/packages/p1/campaigns/c1/hold"},
		{http.MethodPut, "/admin/packages/p1/campaigns/c1/version"},
	} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: got %d, want 403 from reauth", tt.method, tt.path, rec.Code)
		}
	}
}

func TestCampaignChip(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	eightDays := now.Add(-8 * 24 * time.Hour)
	cases := []struct {
		name      string
		st        CampaignPackageState
		label     string
		hint      string
		wantAsked bool
	}{
		{"current", CampaignPackageState{EffectiveVersion: "2.8.0"}, "Up to date", "", false},
		{"owner asked", CampaignPackageState{HeldVersion: "2.9.0", HeldAt: &eightDays}, "Owner asked", "since 25 Sep 2026", true},
		{"owner chose later", CampaignPackageState{HeldVersion: "2.9.0", DismissedVersion: "2.9.0", HeldAt: &eightDays}, "Owner chose Later", "2.9.0 offered 8 days ago", true},
		{"later was for an older version", CampaignPackageState{HeldVersion: "2.9.0", DismissedVersion: "2.8.5", HeldAt: &eightDays}, "Owner asked", "since 25 Sep 2026", true},
		{"held by the admin", CampaignPackageState{HeldVersion: "2.9.0", AdminHold: true, AdminHoldAt: &eightDays}, "Held by you", "since 25 Sep 2026", false},
	}
	for _, tc := range cases {
		got := campaignChip(tc.st, now)
		if got.Label != tc.label || got.Hint != tc.hint || got.Asked != tc.wantAsked {
			t.Errorf("%s: got %+v, want label %q hint %q asked %v", tc.name, got, tc.label, tc.hint, tc.wantAsked)
		}
	}
}

func foundryRow(camps ...CampaignPackageState) PackageRow {
	return PackageRow{
		Package:         Package{ID: "m1", Type: PackageTypeFoundryModule, Name: "Chronicle Sync", InstalledVersion: "2.9.0", Status: StatusApproved},
		Campaigns:       camps,
		CampaignsKnown:  true,
		MovableVersions: []string{"2.9.0", "2.8.0"},
	}
}

func TestCampaignsTabForTheFoundryModule(t *testing.T) {
	now := time.Now()
	row := foundryRow(
		CampaignPackageState{CampaignID: "c1", CampaignName: "Shattered Coast", EffectiveVersion: "2.9.0"},
		CampaignPackageState{CampaignID: "c2", CampaignName: "Ember Vale", EffectiveVersion: "2.8.0", HeldVersion: "2.9.0", HeldAt: &now},
		CampaignPackageState{CampaignID: "c3", CampaignName: "The Long Night", EffectiveVersion: "2.7.4", HeldVersion: "2.9.0", AdminHold: true, AdminHoldAt: &now},
	)
	data := PackagesPageData{CSRFToken: "tok", Now: now, CanRemind: true}
	out := renderToString(t, campaignUpdatesPanel(row, data))

	for _, want := range []string{
		"Shattered Coast", "Up to date",
		"Ember Vale", "Owner asked",
		"The Long Night", "Held by you",
		// per-campaign menu
		"Hold on 2.8.0", "Stop holding", "Move to a version",
		`hx-post="/admin/packages/m1/campaigns/c2/hold"`,
		`hx-delete="/admin/packages/m1/campaigns/c3/hold"`,
		`hx-put="/admin/packages/m1/campaigns/c2/version"`,
		`hx-post="/admin/packages/m1/campaigns/c2/remind"`,
		"Each owner is asked", // the explanation keeps the contract in view
	} {
		if want == "Each owner is asked" {
			want = "each owner is asked"
		}
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	// Remind only for an owner who has something waiting, and not for a
	// campaign that is up to date or held.
	if n := strings.Count(out, "/remind\""); n != 1 {
		t.Errorf("Remind owner should be offered once (only Ember Vale), got %d", n)
	}
	if strings.Count(out, "<dialog") != 3 {
		t.Errorf("each campaign has its own Move dialog")
	}
}

func TestCampaignsTabOmitsRemindWithoutANotificationPath(t *testing.T) {
	now := time.Now()
	row := foundryRow(CampaignPackageState{CampaignID: "c2", CampaignName: "Ember Vale", EffectiveVersion: "2.8.0", HeldVersion: "2.9.0", HeldAt: &now})
	out := renderToString(t, campaignUpdatesPanel(row, PackagesPageData{CSRFToken: "tok", Now: now}))
	if strings.Contains(out, "Remind owner") {
		t.Error("Remind owner must not be offered when nothing can deliver it")
	}
}

func TestCampaignsTabFallsBackToUsageForOtherPackages(t *testing.T) {
	sys := PackageRow{Package: Package{ID: "ds", Type: PackageTypeSystem, Name: "Draw Steel"}, UsageKnown: true,
		Usage: []PackageUsage{{CampaignID: "c1", CampaignName: "Shattered Coast", EnabledAt: time.Now()}}}
	data := PackagesPageData{Query: parsePackagesQuery("", "", "", "ds", PanelTabCampaigns), CSRFToken: "tok", Now: time.Now(), Rows: []PackageRow{sys}}
	data.Selected = &data.Rows[0]
	out := renderToString(t, packagePanel(data))
	if strings.Contains(out, "Hold on") || strings.Contains(out, "Owner asked") {
		t.Errorf("a game system keeps today's plain campaign list: %s", out)
	}
	if !strings.Contains(out, "Shattered Coast") {
		t.Error("the usage list must still render")
	}
}

func TestUpdatesTabCountsOwnersAsked(t *testing.T) {
	row := foundryRow(
		CampaignPackageState{CampaignID: "c1", CampaignName: "A", EffectiveVersion: "2.8.0"},
		CampaignPackageState{CampaignID: "c2", CampaignName: "B", EffectiveVersion: "2.8.0"},
		CampaignPackageState{CampaignID: "c3", CampaignName: "C", EffectiveVersion: "2.7.0", AdminHold: true},
	)
	row.InstalledVersion = "2.8.0"
	row.Newer = &PackageVersion{Version: "2.9.0"}
	data := PackagesPageData{CSRFToken: "tok", Now: time.Now(), Updates: []PackageRow{row}}
	out := renderToString(t, updatesTab(data))
	for _, want := range []string{"2 owners will be asked", "1 held by you",
		"Each campaign keeps its current version until its owner presses Update."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}

	// A system's update row has no owner counts and the plain confirm.
	sys := PackageRow{Package: Package{ID: "ds", Type: PackageTypeSystem, Name: "Draw Steel", InstalledVersion: "1"}, Newer: &PackageVersion{Version: "2"}}
	out = renderToString(t, updatesTab(PackagesPageData{CSRFToken: "tok", Now: time.Now(), Updates: []PackageRow{sys}}))
	if strings.Contains(out, "owners will be asked") || strings.Contains(out, "keeps its current version") {
		t.Errorf("game systems keep today's wording: %s", out)
	}
}

func TestInstallConfirmText(t *testing.T) {
	if got := installConfirmText(PackageTypeFoundryModule, "2.9.0"); got != "Install version 2.9.0? Each campaign keeps its current version until its owner presses Update." {
		t.Errorf("foundry confirm = %q", got)
	}
	if got := installConfirmText(PackageTypeSystem, "1.0.0"); got != "Install version 1.0.0?" {
		t.Errorf("system confirm = %q", got)
	}
}

// --- handlers ---

type fakeCampaignUpdates struct {
	CampaignUpdateService
	state   CampaignPackageState
	hold    *bool
	moved   string
	actor   ActorInfo
	moveErr error
}

func (f *fakeCampaignUpdates) SetAdminHold(_ context.Context, _, _ string, hold bool, a ActorInfo) (*CampaignPackageState, error) {
	f.hold, f.actor = &hold, a
	return &f.state, nil
}
func (f *fakeCampaignUpdates) SwitchVersion(_ context.Context, _, _, v string, a ActorInfo) (*CampaignPackageState, error) {
	f.moved, f.actor = v, a
	return &f.state, f.moveErr
}
func (f *fakeCampaignUpdates) CampaignState(context.Context, string, string) (*CampaignPackageState, error) {
	st := f.state
	return &st, nil
}

type fakePackageGetter struct{ PackageService }

func (fakePackageGetter) GetPackage(_ context.Context, id string) (*Package, error) {
	return &Package{ID: id, Name: "Chronicle Sync", Type: PackageTypeFoundryModule}, nil
}

type recordingReminder struct {
	campaign, version string
	err               error
}

func (r *recordingReminder) RemindOwner(_ context.Context, _ *Package, c, v string, _ ActorInfo) error {
	r.campaign, r.version = c, v
	return r.err
}

func adminCtx(method string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "/x", strings.NewReader("version=2.8.0"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id", "cid")
	c.SetParamValues("m1", "c1")
	auth.SetSession(c, &auth.Session{UserID: "admin-1"})
	return c, rec
}

func TestAdminCampaignActionsActOnTheOwnersBehalf(t *testing.T) {
	upd := &fakeCampaignUpdates{}
	h := &Handler{service: fakePackageGetter{}, updates: upd}

	c, rec := adminCtx(http.MethodPost)
	if err := h.HoldCampaign(c); err != nil || upd.hold == nil || !*upd.hold || !upd.actor.Admin || upd.actor.UserID != "admin-1" {
		t.Fatalf("hold: err %v, hold %v, actor %+v", err, upd.hold, upd.actor)
	}
	if rec.Header().Get("HX-Redirect") == "" {
		t.Error("a write answers by sending the admin back to the page")
	}

	c, _ = adminCtx(http.MethodDelete)
	if err := h.ReleaseCampaign(c); err != nil || upd.hold == nil || *upd.hold {
		t.Fatalf("release: err %v, hold %v", err, upd.hold)
	}

	c, _ = adminCtx(http.MethodPut)
	if err := h.MoveCampaign(c); err != nil || upd.moved != "2.8.0" || !upd.actor.Admin {
		t.Fatalf("move: err %v, moved %q, actor %+v", err, upd.moved, upd.actor)
	}

	upd.moveErr = apperror.NewBadRequest("version 9.9.9 is not a known release of this package")
	c, _ = adminCtx(http.MethodPut)
	if err := h.MoveCampaign(c); err == nil {
		t.Error("a refused move must surface its error")
	}
}

func TestAdminCampaignActionsNeedTheUpdateService(t *testing.T) {
	h := &Handler{service: fakePackageGetter{}}
	for name, fn := range map[string]func(echo.Context) error{
		"hold": h.HoldCampaign, "release": h.ReleaseCampaign, "move": h.MoveCampaign, "remind": h.RemindCampaignOwner,
	} {
		c, _ := adminCtx(http.MethodPost)
		var ae *apperror.AppError
		if err := fn(c); !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
			t.Errorf("%s without the service: err = %v, want not found", name, err)
		}
	}
}

func TestAdminCampaignActionsNeedASession(t *testing.T) {
	h := &Handler{service: fakePackageGetter{}, updates: &fakeCampaignUpdates{}}
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	c := echo.New().NewContext(req, httptest.NewRecorder())
	var ae *apperror.AppError
	if err := h.HoldCampaign(c); !errors.As(err, &ae) || ae.Code != http.StatusUnauthorized {
		t.Errorf("err = %v, want unauthorized", err)
	}
}

func TestRemindReadsTheWaitingVersionFromTheServer(t *testing.T) {
	rem := &recordingReminder{}
	upd := &fakeCampaignUpdates{state: CampaignPackageState{HeldVersion: "2.9.0"}}
	h := &Handler{service: fakePackageGetter{}, updates: upd, reminder: rem}

	c, _ := adminCtx(http.MethodPost)
	if err := h.RemindCampaignOwner(c); err != nil {
		t.Fatal(err)
	}
	if rem.campaign != "c1" || rem.version != "2.9.0" {
		t.Errorf("reminder got %q %q; the version must come from the stored hold, not the request", rem.campaign, rem.version)
	}

	// Nothing waiting: there is nothing to remind about.
	upd.state = CampaignPackageState{}
	rem.campaign = ""
	c, _ = adminCtx(http.MethodPost)
	var ae *apperror.AppError
	if err := h.RemindCampaignOwner(c); !errors.As(err, &ae) || ae.Code != http.StatusConflict || rem.campaign != "" {
		t.Errorf("err = %v, reminded %q; want conflict and no reminder", err, rem.campaign)
	}

	// No way to reach owners wired: not offered.
	h.reminder = nil
	c, _ = adminCtx(http.MethodPost)
	if err := h.RemindCampaignOwner(c); !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Errorf("err = %v, want not found", err)
	}
}

// A campaign id typed into a request must not create state for something the
// package does not apply to.
func TestAdminActionsRefuseACampaignThePackageDoesNotUse(t *testing.T) {
	e := newUpdEnv(t, "2.0.0", []string{"1.0.0", "2.0.0"}, "real")
	var ae *apperror.AppError
	if _, err := e.svc.SetAdminHold(ctx(), "typed-in", "p1", true, ActorInfo{Admin: true}); !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Errorf("hold on an unknown campaign: err = %v, want not found", err)
	}
	if _, err := e.svc.SwitchVersion(ctx(), "typed-in", "p1", "1.0.0", ActorInfo{Admin: true}); !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Errorf("move of an unknown campaign: err = %v, want not found", err)
	}
	if len(e.repo.rows) != 0 {
		t.Errorf("a refused request must leave no row behind: %+v", e.repo.rows)
	}
}
