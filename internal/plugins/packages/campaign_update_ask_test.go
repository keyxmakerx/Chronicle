package packages

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// askBinding is a binding for a package type that asks by default (the Foundry
// module's behaviour), with its mode and version stored outside the table.
type askBinding struct{ storedElsewhereBinding }

func (askBinding) AsksByDefault() bool { return true }

// askEnv is one module package "mod" at installed version 2.0.0 with 1.0.0 and
// 2.0.0 on disk, plus one campaign per starting situation.
type askEnv struct {
	svc  *campaignUpdateService
	repo *fakeUpdateRepo
	pkgs *fakeUpdatePkgs
	bind *askBinding
	pkg  Package
}

func newAskEnv(t *testing.T, states map[string]*CampaignPackageState) *askEnv {
	t.Helper()
	pkgs := &fakeUpdatePkgs{root: t.TempDir(), usage: map[string][]PackageUsage{}}
	pkgs.pkgs = []Package{{ID: "f1", Type: PackageTypeFoundryModule, Slug: "mod", InstalledVersion: "2.0.0", Status: StatusApproved}}
	pkgs.mkVersions(t, "mod", "1.0.0", "2.0.0")
	repo := newFakeUpdateRepo()
	svc := newCampaignUpdateService(repo, pkgs, &recAudit{})
	bind := &askBinding{storedElsewhereBinding{states: states}}
	svc.RegisterBinding(bind)
	return &askEnv{svc: svc, repo: repo, pkgs: pkgs, bind: bind, pkg: pkgs.pkgs[0]}
}

// install runs the two install steps for a move from prev to next, the way
// the install pipeline does around the commit.
func (e *askEnv) install(t *testing.T, prev, next string) {
	t.Helper()
	e.pkgs.mkVersions(t, "mod", next)
	pkg := e.pkg
	if err := e.svc.OnInstall(ctx(), &pkg, prev, next); err != nil {
		t.Fatalf("OnInstall: %v", err)
	}
	pkg.InstalledVersion = next
	e.pkgs.pkgs[0].InstalledVersion = next
	e.pkg = pkg
	if err := e.svc.OnInstalled(ctx(), &pkg, prev, next); err != nil {
		t.Fatalf("OnInstalled: %v", err)
	}
}

func appStatus(err error) int {
	if ae, ok := err.(*apperror.AppError); ok {
		return ae.Code
	}
	return 0
}

// A new version never moves a campaign: one that followed the installed
// version is frozen where it was and asked, a pinned one stays put and sees
// the waiting version, and an ask-first one is asked as before.
func TestNewVersionAsksEveryCampaign(t *testing.T) {
	e := newAskEnv(t, map[string]*CampaignPackageState{
		"auto":   {Mode: UpdateModeAutomatic},
		"pinned": {Mode: UpdateModePinned, Version: "1.0.0"},
		"asking": {Mode: UpdateModeApproveFirst, Version: "1.0.0", Explicit: true},
	})
	e.pkgs.pkgs[0].InstalledVersion = "1.0.0"
	e.pkg.InstalledVersion = "1.0.0"
	e.install(t, "1.0.0", "2.0.0")

	for id, want := range map[string]struct {
		mode    UpdateMode
		version string
	}{
		"auto":   {UpdateModeApproveFirst, "1.0.0"},
		"pinned": {UpdateModePinned, "1.0.0"},
		"asking": {UpdateModeApproveFirst, "1.0.0"},
	} {
		got := e.bind.states[id]
		if got.Mode != want.mode || got.Version != want.version {
			t.Errorf("%s: %s on %q, want %s on %q", id, got.Mode, got.Version, want.mode, want.version)
		}
		st, err := e.svc.CampaignState(ctx(), id, "f1")
		if err != nil {
			t.Fatal(err)
		}
		if st.HeldVersion != "2.0.0" || st.EffectiveVersion != "1.0.0" || st.HeldAt == nil {
			t.Errorf("%s: want 1.0.0 running with 2.0.0 waiting (since set), got %+v", id, st)
		}
	}
}

// The boot pass covers a campaign created since the last install, and holds
// nothing for one that is already current.
func TestReconcileFreezesAutomaticCampaigns(t *testing.T) {
	e := newAskEnv(t, map[string]*CampaignPackageState{
		"new":     {Mode: UpdateModeAutomatic},
		"current": {Mode: UpdateModePinned, Version: "2.0.0"},
	})
	if _, err := e.svc.Reconcile(ctx()); err != nil {
		t.Fatal(err)
	}
	if got := e.bind.states["new"]; got.Mode != UpdateModeApproveFirst || got.Version != "2.0.0" {
		t.Errorf("new campaign: %+v, want ask-first on the installed version", got)
	}
	for _, id := range []string{"new", "current"} {
		if r := e.repo.rows[ukey(id, "f1")]; r != nil && r.HeldVersion != "" {
			t.Errorf("%s is current, nothing may wait: %+v", id, r)
		}
	}
	// Running it again changes nothing.
	res, err := e.svc.Reconcile(ctx())
	if err != nil || res.HeldSet != 0 || res.HeldCleared != 0 {
		t.Errorf("second pass = %+v, %v", res, err)
	}
}

// Game systems keep today's behaviour: automatic follows the installed
// version and a pinned system is not offered anything.
func TestSystemsAreNotAskedByDefault(t *testing.T) {
	e := newUpdEnv(t, "2.0.0", []string{"1.0.0", "2.0.0"}, "auto", "pinned")
	if err := e.repo.UpsertModeVersion(ctx(), "pinned", "p1", UpdateModePinned, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Reconcile(ctx()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"auto", "pinned"} {
		st, err := e.svc.CampaignState(ctx(), id, "p1")
		if err != nil {
			t.Fatal(err)
		}
		if st.HeldVersion != "" {
			t.Errorf("%s: a system must not be offered a waiting version, got %q", id, st.HeldVersion)
		}
	}
	if st, _ := e.svc.CampaignState(ctx(), "auto", "p1"); st.Mode != UpdateModeAutomatic {
		t.Errorf("a system campaign with no choice must stay automatic, got %s", st.Mode)
	}
}

func TestDismissHeldIsPerVersion(t *testing.T) {
	e := newAskEnv(t, map[string]*CampaignPackageState{"c": {Mode: UpdateModeApproveFirst, Version: "1.0.0", Explicit: true}})
	e.pkgs.pkgs[0].InstalledVersion = "1.0.0"
	e.pkg.InstalledVersion = "1.0.0"
	e.install(t, "1.0.0", "2.0.0")

	cases := []struct {
		name     string
		version  string
		wantCode int
	}{
		{"not the waiting version", "1.5.0", http.StatusConflict},
		{"empty", "", http.StatusConflict},
		{"the waiting version", "2.0.0", 0},
	}
	for _, tc := range cases {
		_, err := e.svc.DismissHeld(ctx(), "c", "f1", tc.version)
		if appStatus(err) != tc.wantCode {
			t.Errorf("%s: err = %v, want status %d", tc.name, err, tc.wantCode)
		}
	}
	st, _ := e.svc.CampaignState(ctx(), "c", "f1")
	if st.DismissedVersion != "2.0.0" || st.HeldVersion != "2.0.0" {
		t.Fatalf("2.0.0 must stay waiting but dismissed, got %+v", st)
	}

	// A newer install asks again: the dismissal named 2.0.0 only.
	e.install(t, "2.0.0", "3.0.0")
	st, _ = e.svc.CampaignState(ctx(), "c", "f1")
	if st.HeldVersion != "3.0.0" || st.DismissedVersion == st.HeldVersion {
		t.Errorf("3.0.0 must be asked about again, got %+v", st)
	}
}

func TestAdminHoldBlocksTheOwner(t *testing.T) {
	e := newAskEnv(t, map[string]*CampaignPackageState{"c": {Mode: UpdateModeApproveFirst, Version: "1.0.0", Explicit: true}})
	e.pkgs.pkgs[0].InstalledVersion = "1.0.0"
	e.pkg.InstalledVersion = "1.0.0"
	e.install(t, "1.0.0", "2.0.0")
	owner, admin := ActorInfo{UserID: "u"}, ActorInfo{UserID: "a", Admin: true}

	if _, err := e.svc.SetAdminHold(ctx(), "c", "f1", true, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ApproveHeld(ctx(), "c", "f1", owner); appStatus(err) != http.StatusForbidden {
		t.Errorf("owner Update while held: err = %v, want forbidden", err)
	}
	if _, err := e.svc.SwitchVersion(ctx(), "c", "f1", "2.0.0", owner); appStatus(err) != http.StatusForbidden {
		t.Errorf("owner switch while held: err = %v, want forbidden", err)
	}
	if got := e.bind.states["c"].Version; got != "1.0.0" {
		t.Fatalf("a blocked owner must not move the campaign, it is on %s", got)
	}

	// The admin may still move it, and the hold stays.
	st, err := e.svc.SwitchVersion(ctx(), "c", "f1", "2.0.0", admin)
	if err != nil || st.EffectiveVersion != "2.0.0" || !st.AdminHold {
		t.Fatalf("admin move = %+v, %v", st, err)
	}

	// Letting go gives the owner their buttons back.
	if _, err := e.svc.SetAdminHold(ctx(), "c", "f1", false, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SwitchVersion(ctx(), "c", "f1", "1.0.0", owner); err != nil {
		t.Errorf("owner switch after the hold is released: %v", err)
	}
}

// Holding a campaign that still follows the installed version freezes it on
// that version first, so the hold means something.
func TestAdminHoldFreezesAutomaticCampaign(t *testing.T) {
	e := newAskEnv(t, map[string]*CampaignPackageState{"c": {Mode: UpdateModeAutomatic}})
	st, err := e.svc.SetAdminHold(ctx(), "c", "f1", true, ActorInfo{Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.bind.states["c"]; got.Mode != UpdateModeApproveFirst || got.Version != "2.0.0" {
		t.Errorf("held campaign must be frozen on 2.0.0, got %+v", got)
	}
	if !st.AdminHold || st.AdminHoldAt == nil {
		t.Errorf("hold not recorded: %+v", st)
	}
}

func TestSwitchVersionValidation(t *testing.T) {
	e := newAskEnv(t, map[string]*CampaignPackageState{"c": {Mode: UpdateModeApproveFirst, Version: "2.0.0", Explicit: true}})
	// A release that is known but whose folder was cleaned up, and one the
	// package never had.
	e.pkgs.versions = append(e.pkgs.versions, "0.5.0")
	cases := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"installed version", "1.0.0", false},
		{"path traversal", "../1.0.0", true},
		{"not a version string", "1.0.0 ; rm", true},
		{"empty", "", true},
		{"known but removed from disk", "0.5.0", true},
		{"never a release", "9.9.9", true},
	}
	for _, tc := range cases {
		_, err := e.svc.SwitchVersion(ctx(), "c", "f1", tc.version, ActorInfo{})
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
	if got := e.bind.states["c"].Version; got != "1.0.0" {
		t.Errorf("only the valid switch may land, campaign is on %s", got)
	}
}

// Going back keeps the newer version on offer, and moving onto the installed
// version clears the offer.
func TestSwitchVersionKeepsNewerOnOffer(t *testing.T) {
	e := newAskEnv(t, map[string]*CampaignPackageState{"c": {Mode: UpdateModePinned, Version: "2.0.0"}})
	st, err := e.svc.SwitchVersion(ctx(), "c", "f1", "1.0.0", ActorInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if st.EffectiveVersion != "1.0.0" || st.HeldVersion != "2.0.0" || st.Mode != UpdateModePinned {
		t.Errorf("after going back: %+v", st)
	}
	st, err = e.svc.SwitchVersion(ctx(), "c", "f1", "2.0.0", ActorInfo{})
	if err != nil || st.HeldVersion != "" {
		t.Errorf("after moving onto the installed version: %+v, %v", st, err)
	}
}

func TestInstalledVersions(t *testing.T) {
	e := newAskEnv(t, nil)
	e.pkgs.versions = append(e.pkgs.versions, "0.5.0", "1.0.0") // removed from disk, and a duplicate row
	e.pkgs.mkVersions(t, "mod", "10.0.0")
	got, err := e.svc.InstalledVersions(ctx(), "f1")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"10.0.0", "2.0.0", "1.0.0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("InstalledVersions = %v, want %v", got, want)
	}
	if err := os.RemoveAll(filepath.Join(e.pkgs.root, "mod", "2.0.0")); err != nil {
		t.Fatal(err)
	}
	got, _ = e.svc.InstalledVersions(ctx(), "f1")
	if want := []string{"10.0.0", "1.0.0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after clean-up removed 2.0.0: %v, want %v", got, want)
	}
}

// Approving keeps a pinned campaign pinned and clears what was waiting.
func TestApproveKeepsMode(t *testing.T) {
	e := newAskEnv(t, map[string]*CampaignPackageState{"c": {Mode: UpdateModePinned, Version: "1.0.0"}})
	if _, err := e.svc.Reconcile(ctx()); err != nil {
		t.Fatal(err)
	}
	st, err := e.svc.ApproveHeld(ctx(), "c", "f1", ActorInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != UpdateModePinned || st.EffectiveVersion != "2.0.0" || st.HeldVersion != "" {
		t.Errorf("after Update: %+v", st)
	}
	if _, err := e.svc.ApproveHeld(ctx(), "c", "f1", ActorInfo{}); appStatus(err) != http.StatusConflict {
		t.Errorf("a second Update with nothing waiting: err = %v, want conflict", err)
	}
}
