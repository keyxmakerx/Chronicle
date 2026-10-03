package packages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

// --- fakes ---

// fakeUpdateRepo is an in-memory CampaignUpdateRepository.
type fakeUpdateRepo struct {
	rows    map[string]*CampaignUpdateRow // key campaign|package
	slugOf  map[string]string             // package id -> slug (for kept versions)
	failSet bool
}

func newFakeUpdateRepo() *fakeUpdateRepo {
	return &fakeUpdateRepo{rows: map[string]*CampaignUpdateRow{}, slugOf: map[string]string{}}
}

func ukey(c, p string) string { return c + "|" + p }

func (f *fakeUpdateRepo) Get(_ context.Context, c, p string) (*CampaignUpdateRow, error) {
	if r, ok := f.rows[ukey(c, p)]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeUpdateRepo) sorted(keep func(*CampaignUpdateRow) bool) []CampaignUpdateRow {
	var out []CampaignUpdateRow
	for _, r := range f.rows {
		if keep(r) {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CampaignID < out[j].CampaignID })
	return out
}

func (f *fakeUpdateRepo) ListByPackage(_ context.Context, p string) ([]CampaignUpdateRow, error) {
	return f.sorted(func(r *CampaignUpdateRow) bool { return r.PackageID == p }), nil
}

func (f *fakeUpdateRepo) ListHeld(_ context.Context) ([]CampaignUpdateRow, error) {
	return f.sorted(func(r *CampaignUpdateRow) bool { return r.HeldVersion != "" }), nil
}

func (f *fakeUpdateRepo) ListKeptVersions(_ context.Context) (map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	for _, r := range f.rows {
		slug := f.slugOf[r.PackageID]
		for _, v := range []string{r.Version, r.HeldVersion} {
			if v == "" {
				continue
			}
			if out[slug] == nil {
				out[slug] = map[string]bool{}
			}
			out[slug][v] = true
		}
	}
	return out, nil
}

func (f *fakeUpdateRepo) UpsertModeVersion(_ context.Context, c, p string, m UpdateMode, v string) error {
	r := f.rows[ukey(c, p)]
	if r == nil {
		r = &CampaignUpdateRow{CampaignID: c, PackageID: p}
		f.rows[ukey(c, p)] = r
	}
	r.Mode, r.Version = m, v
	return nil
}

func (f *fakeUpdateRepo) SetHeld(_ context.Context, c, p, held string) error {
	if f.failSet {
		return errors.New("db down")
	}
	r := f.rows[ukey(c, p)]
	if r == nil {
		if held == "" {
			return nil
		}
		r = &CampaignUpdateRow{CampaignID: c, PackageID: p, Mode: UpdateModeAutomatic}
		f.rows[ukey(c, p)] = r
	}
	r.HeldVersion = held
	return nil
}

func (f *fakeUpdateRepo) DeleteDefaultRows(_ context.Context) (int64, error) {
	var n int64
	for k, r := range f.rows {
		if r.Mode == UpdateModeAutomatic && r.Version == "" && r.HeldVersion == "" {
			delete(f.rows, k)
			n++
		}
	}
	return n, nil
}

// fakeUpdatePkgs is an updatePackageSource over a temp dir of version folders.
type fakeUpdatePkgs struct {
	root     string
	pkgs     []Package
	usage    map[string][]PackageUsage
	versions []string // known releases, filled by mkVersions
	mu       sync.Mutex
}

func (f *fakeUpdatePkgs) ListVersions(context.Context, string) ([]PackageVersion, error) {
	var out []PackageVersion
	for _, v := range f.versions {
		out = append(out, PackageVersion{Version: v})
	}
	return out, nil
}
func (f *fakeUpdatePkgs) lockForPackage(string) *sync.Mutex { return &f.mu }

func (f *fakeUpdatePkgs) ListPackages(context.Context) ([]Package, error) { return f.pkgs, nil }
func (f *fakeUpdatePkgs) GetPackage(_ context.Context, id string) (*Package, error) {
	for i := range f.pkgs {
		if f.pkgs[i].ID == id {
			p := f.pkgs[i]
			return &p, nil
		}
	}
	return nil, nil
}
func (f *fakeUpdatePkgs) GetUsage(_ context.Context, id string) ([]PackageUsage, error) {
	return f.usage[id], nil
}
func (f *fakeUpdatePkgs) InstallDirForVersion(_ PackageType, slug, v string) string {
	if v == "" {
		return ""
	}
	return filepath.Join(f.root, slug, v)
}

func (f *fakeUpdatePkgs) mkVersions(t *testing.T, slug string, versions ...string) {
	t.Helper()
	for _, v := range versions {
		f.versions = append(f.versions, v)
		if err := os.MkdirAll(filepath.Join(f.root, slug, v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

type recAudit struct{ events []map[string]any }

func (r *recAudit) LogEvent(_ context.Context, event, _, _, _, _ string, d map[string]any) error {
	d2 := map[string]any{"event": event}
	for k, v := range d {
		d2[k] = v
	}
	r.events = append(r.events, d2)
	return nil
}

type updEnv struct {
	svc   *campaignUpdateService
	repo  *fakeUpdateRepo
	pkgs  *fakeUpdatePkgs
	audit *recAudit
}

// newUpdEnv builds one installed system package "sys" (id p1) at the given
// installed version, with the listed version folders on disk and the listed
// campaigns using it.
func newUpdEnv(t *testing.T, installed string, onDisk []string, campaigns ...string) *updEnv {
	t.Helper()
	pkgs := &fakeUpdatePkgs{root: t.TempDir(), usage: map[string][]PackageUsage{}}
	pkgs.pkgs = []Package{{ID: "p1", Type: PackageTypeSystem, Slug: "sys", InstalledVersion: installed, Status: StatusApproved}}
	pkgs.mkVersions(t, "sys", onDisk...)
	for _, c := range campaigns {
		pkgs.usage["p1"] = append(pkgs.usage["p1"], PackageUsage{CampaignID: c, CampaignName: "name-" + c})
	}
	repo := newFakeUpdateRepo()
	repo.slugOf["p1"] = "sys"
	audit := &recAudit{}
	return &updEnv{svc: newCampaignUpdateService(repo, pkgs, audit), repo: repo, pkgs: pkgs, audit: audit}
}

func ctx() context.Context { return context.Background() }

// --- tests ---

func TestParseUpdateMode(t *testing.T) {
	cases := []struct {
		in      string
		want    UpdateMode
		wantErr bool
	}{
		{"automatic", UpdateModeAutomatic, false},
		{"pinned", UpdateModePinned, false},
		{"approve_first", UpdateModeApproveFirst, false},
		{" pinned ", UpdateModePinned, false},
		{"", "", true},
		{"Automatic", "", true},
		{"preserve", "", true},
		{"ask", "", true},
	}
	for _, tc := range cases {
		got, err := ParseUpdateMode(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("ParseUpdateMode(%q) = %q, %v; want %q, err=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestReportedPackageMode(t *testing.T) {
	cases := []struct {
		name string
		pkg  *Package
		want UpdateMode
	}{
		{"nil", nil, UpdateModeAutomatic},
		{"auto-update on, no pin", &Package{AutoUpdate: UpdateNightly}, UpdateModeAutomatic},
		{"auto-update off, no pin", &Package{AutoUpdate: UpdateOff}, UpdateModePinned},
		{"pin wins over auto-update", &Package{AutoUpdate: UpdateWeekly, PinnedVersion: "1.0.0"}, UpdateModePinned},
	}
	for _, tc := range cases {
		if got := ReportedPackageMode(tc.pkg); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestDefaultsAreAutomatic(t *testing.T) {
	e := newUpdEnv(t, "2.0.0", []string{"2.0.0"}, "c1")
	st, err := e.svc.CampaignState(ctx(), "c1", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != UpdateModeAutomatic || st.Version != "" || st.EffectiveVersion != "2.0.0" || st.HeldVersion != "" {
		t.Fatalf("a campaign with no row must read as automatic on the installed version, got %+v", st)
	}
	states, err := e.svc.CampaignStates(ctx(), "c1")
	if err != nil || len(states) != 1 || states[0].Mode != UpdateModeAutomatic {
		t.Fatalf("CampaignStates = %+v, %v", states, err)
	}
	if other, _ := e.svc.CampaignStates(ctx(), "not-using"); len(other) != 0 {
		t.Fatalf("a campaign that does not use the system must not list it: %+v", other)
	}
}

func TestSetMode(t *testing.T) {
	cases := []struct {
		name        string
		in          SetUpdateModeInput
		wantErr     bool
		wantMode    UpdateMode
		wantVersion string
	}{
		{"automatic", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "automatic"}, false, UpdateModeAutomatic, ""},
		{"pinned to an installed older version", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "pinned", Version: "1.0.0"}, false, UpdateModePinned, "1.0.0"},
		{"pinned with no version uses the current one", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "pinned"}, false, UpdateModePinned, "2.0.0"},
		{"path traversal", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "pinned", Version: "../../x"}, true, "", ""},
		{"path separator", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "pinned", Version: "1.0.0/../2.0.0"}, true, "", ""},
		{"dot dot only", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "pinned", Version: ".."}, true, "", ""},
		{"valid format but not a known release", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "pinned", Version: "9.9.9"}, true, "", ""},
		{"pinned to a version that is not on disk", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "pinned", Version: "0.0.1"}, true, "", ""},
		{"approve first stays where it is", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "approve_first", Version: "1.0.0"}, false, UpdateModeApproveFirst, "2.0.0"},
		{"unknown mode", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: "sometimes"}, true, "", ""},
		{"empty mode", SetUpdateModeInput{CampaignID: "c1", PackageID: "p1"}, true, "", ""},
		{"unknown package", SetUpdateModeInput{CampaignID: "c1", PackageID: "nope", Mode: "automatic"}, true, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newUpdEnv(t, "2.0.0", []string{"1.0.0", "2.0.0"}, "c1")
			st, err := e.svc.SetMode(ctx(), tc.in, ActorInfo{UserID: "owner"})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if len(e.repo.rows) != 0 {
					t.Errorf("a refused change must write nothing: %+v", e.repo.rows)
				}
				return
			}
			if st.Mode != tc.wantMode || st.Version != tc.wantVersion {
				t.Errorf("state = %+v, want mode %q version %q", st, tc.wantMode, tc.wantVersion)
			}
			if len(e.audit.events) != 1 || e.audit.events[0]["event"] != EventCampaignUpdateModeSet {
				t.Errorf("want one mode-set audit event, got %+v", e.audit.events)
			}
		})
	}
}

func TestSetModeKeepsPendingHoldOnlyWhileAskingFirst(t *testing.T) {
	cases := []struct {
		name     string
		to       string
		wantHeld string
	}{
		{"still asking first keeps the hold", "approve_first", "2.0.0"},
		{"switching to pinned answers it", "pinned", ""},
		{"switching to automatic answers it", "automatic", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newUpdEnv(t, "2.0.0", []string{"1.0.0", "2.0.0"}, "c1")
			e.repo.rows[ukey("c1", "p1")] = &CampaignUpdateRow{CampaignID: "c1", PackageID: "p1", Mode: UpdateModeApproveFirst, Version: "1.0.0", HeldVersion: "2.0.0"}
			if _, err := e.svc.SetMode(ctx(), SetUpdateModeInput{CampaignID: "c1", PackageID: "p1", Mode: tc.to, Version: "1.0.0"}, ActorInfo{}); err != nil {
				t.Fatal(err)
			}
			if got := e.repo.rows[ukey("c1", "p1")].HeldVersion; got != tc.wantHeld {
				t.Errorf("held = %q, want %q", got, tc.wantHeld)
			}
		})
	}
}

func TestOnInstallPerMode(t *testing.T) {
	cases := []struct {
		name         string
		row          *CampaignUpdateRow // nil = no stored choice
		prev, next   string
		wantVersion  string
		wantHeld     string
		wantMode     UpdateMode
		wantNoChange bool
	}{
		{name: "automatic follows the new version, untouched", prev: "1.0.0", next: "2.0.0", wantMode: UpdateModeAutomatic, wantNoChange: true},
		{name: "pinned with a version is untouched",
			row:  &CampaignUpdateRow{Mode: UpdateModePinned, Version: "0.9.0"},
			prev: "1.0.0", next: "2.0.0", wantMode: UpdateModePinned, wantVersion: "0.9.0"},
		{name: "pinned with no version freezes on the previous one",
			row:  &CampaignUpdateRow{Mode: UpdateModePinned},
			prev: "1.0.0", next: "2.0.0", wantMode: UpdateModePinned, wantVersion: "1.0.0"},
		{name: "ask first keeps its version and holds the new one",
			row:  &CampaignUpdateRow{Mode: UpdateModeApproveFirst, Version: "1.0.0"},
			prev: "1.0.0", next: "2.0.0", wantMode: UpdateModeApproveFirst, wantVersion: "1.0.0", wantHeld: "2.0.0"},
		{name: "ask first with no version freezes on previous and holds",
			row:  &CampaignUpdateRow{Mode: UpdateModeApproveFirst},
			prev: "1.0.0", next: "2.0.0", wantMode: UpdateModeApproveFirst, wantVersion: "1.0.0", wantHeld: "2.0.0"},
		{name: "ask first is not offered a rollback",
			row:  &CampaignUpdateRow{Mode: UpdateModeApproveFirst, Version: "3.0.0"},
			prev: "3.0.0", next: "2.0.0", wantMode: UpdateModeApproveFirst, wantVersion: "3.0.0", wantHeld: ""},
		{name: "first-ever install is a no-op",
			row:  &CampaignUpdateRow{Mode: UpdateModePinned},
			prev: "", next: "2.0.0", wantMode: UpdateModePinned, wantVersion: ""},
		{name: "reinstall of the same version is a no-op",
			row:  &CampaignUpdateRow{Mode: UpdateModeApproveFirst, Version: "2.0.0"},
			prev: "2.0.0", next: "2.0.0", wantMode: UpdateModeApproveFirst, wantVersion: "2.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newUpdEnv(t, tc.prev, []string{"0.9.0", "1.0.0", "2.0.0", "3.0.0"}, "c1")
			if tc.row != nil {
				r := *tc.row
				r.CampaignID, r.PackageID = "c1", "p1"
				e.repo.rows[ukey("c1", "p1")] = &r
			}
			pkg := e.pkgs.pkgs[0]
			if err := e.svc.OnInstall(ctx(), &pkg, tc.prev, tc.next); err != nil {
				t.Fatal(err)
			}
			if len(e.repo.rows) > 0 && e.repo.rows[ukey("c1", "p1")].HeldVersion != "" {
				t.Fatal("OnInstall must not write holds; that waits for the commit")
			}
			pkg.InstalledVersion = tc.next // the install committed
			if err := e.svc.OnInstalled(ctx(), &pkg, tc.prev, tc.next); err != nil {
				t.Fatal(err)
			}
			got := e.repo.rows[ukey("c1", "p1")]
			if tc.wantNoChange {
				if got != nil {
					t.Fatalf("an automatic campaign must get no row, got %+v", got)
				}
				return
			}
			if got.Mode != tc.wantMode || got.Version != tc.wantVersion || got.HeldVersion != tc.wantHeld {
				t.Errorf("row = %+v, want mode %q version %q held %q", got, tc.wantMode, tc.wantVersion, tc.wantHeld)
			}
		})
	}
}

// A campaign that cannot be frozen must fail the install, so it is refused
// instead of silently moving onto the new version.
func TestOnInstallFailsClosedWhenACampaignCannotBeFrozen(t *testing.T) {
	e := newUpdEnv(t, "1.0.0", []string{"2.0.0"}, "c1", "c2") // 1.0.0 folder is gone
	e.repo.rows[ukey("c1", "p1")] = &CampaignUpdateRow{CampaignID: "c1", PackageID: "p1", Mode: UpdateModeApproveFirst}
	e.repo.rows[ukey("c2", "p1")] = &CampaignUpdateRow{CampaignID: "c2", PackageID: "p1", Mode: UpdateModeApproveFirst, Version: "1.0.0"}
	pkg := e.pkgs.pkgs[0]
	if err := e.svc.OnInstall(ctx(), &pkg, "1.0.0", "2.0.0"); err == nil {
		t.Fatal("expected the install to be refused")
	}
	// A refused install leaves no hold and no audit row behind, not even for
	// the campaign that could have been held.
	for k, r := range e.repo.rows {
		if r.HeldVersion != "" {
			t.Errorf("%s holds %q after a refused install", k, r.HeldVersion)
		}
	}
	if len(e.audit.events) != 0 {
		t.Errorf("a refused install must write no audit rows, got %+v", e.audit.events)
	}
}

func TestOnInstalledWritesHoldAndAuditAfterCommit(t *testing.T) {
	e := newUpdEnv(t, "2.0.0", []string{"1.0.0", "2.0.0"}, "c1")
	e.repo.rows[ukey("c1", "p1")] = &CampaignUpdateRow{CampaignID: "c1", PackageID: "p1", Mode: UpdateModeApproveFirst, Version: "1.0.0"}
	pkg := e.pkgs.pkgs[0]
	if err := e.svc.OnInstalled(ctx(), &pkg, "1.0.0", "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if e.repo.rows[ukey("c1", "p1")].HeldVersion != "2.0.0" {
		t.Error("hold missing after commit")
	}
	if len(e.audit.events) != 1 || e.audit.events[0]["event"] != EventCampaignUpdateHeld {
		t.Errorf("want one held audit event, got %+v", e.audit.events)
	}
}

func TestApproveHeld(t *testing.T) {
	setup := func(t *testing.T) *updEnv {
		e := newUpdEnv(t, "2.0.0", []string{"1.0.0", "2.0.0"}, "c1")
		e.repo.rows[ukey("c1", "p1")] = &CampaignUpdateRow{CampaignID: "c1", PackageID: "p1", Mode: UpdateModeApproveFirst, Version: "1.0.0", HeldVersion: "2.0.0"}
		return e
	}
	t.Run("owner approves", func(t *testing.T) {
		e := setup(t)
		st, err := e.svc.ApproveHeld(ctx(), "c1", "p1", ActorInfo{UserID: "owner"})
		if err != nil {
			t.Fatal(err)
		}
		if st.Version != "2.0.0" || st.HeldVersion != "" || st.Mode != UpdateModeApproveFirst {
			t.Errorf("state = %+v, want on 2.0.0, nothing held, still asking first", st)
		}
		if ev := e.audit.events[len(e.audit.events)-1]; ev["event"] != EventCampaignUpdateApproved || ev["by_admin"] != false {
			t.Errorf("audit = %+v", ev)
		}
	})
	t.Run("admin approves on their behalf", func(t *testing.T) {
		e := setup(t)
		if _, err := e.svc.ApproveHeld(ctx(), "c1", "p1", ActorInfo{UserID: "admin", Admin: true}); err != nil {
			t.Fatal(err)
		}
		if ev := e.audit.events[len(e.audit.events)-1]; ev["by_admin"] != true {
			t.Errorf("audit must say it was an admin: %+v", ev)
		}
	})
	t.Run("nothing held", func(t *testing.T) {
		e := setup(t)
		e.repo.rows[ukey("c1", "p1")].HeldVersion = ""
		if _, err := e.svc.ApproveHeld(ctx(), "c1", "p1", ActorInfo{}); err == nil {
			t.Fatal("approving nothing must be refused")
		}
	})
	t.Run("not asking first", func(t *testing.T) {
		e := setup(t)
		e.repo.rows[ukey("c1", "p1")].Mode = UpdateModePinned
		if _, err := e.svc.ApproveHeld(ctx(), "c1", "p1", ActorInfo{}); err == nil {
			t.Fatal("a pinned campaign has nothing to approve")
		}
	})
	t.Run("held folder is gone, stays held", func(t *testing.T) {
		e := setup(t)
		if err := os.RemoveAll(filepath.Join(e.pkgs.root, "sys", "2.0.0")); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.ApproveHeld(ctx(), "c1", "p1", ActorInfo{}); err == nil {
			t.Fatal("must refuse to move onto a missing folder")
		}
		row := e.repo.rows[ukey("c1", "p1")]
		if row.Version != "1.0.0" || row.HeldVersion != "2.0.0" {
			t.Errorf("a refused approval must change nothing: %+v", row)
		}
	})
}

func TestServedVersion(t *testing.T) {
	cases := []struct {
		name         string
		row          *CampaignUpdateRow
		removeDir    string
		wantVersion  string
		wantFellBack bool
	}{
		{"no row serves the installed version", nil, "", "2.0.0", false},
		{"automatic serves the installed version",
			&CampaignUpdateRow{Mode: UpdateModeAutomatic, Version: "1.0.0"}, "", "2.0.0", false},
		{"pinned serves its own folder",
			&CampaignUpdateRow{Mode: UpdateModePinned, Version: "1.0.0"}, "", "1.0.0", false},
		{"ask first serves the version it is on",
			&CampaignUpdateRow{Mode: UpdateModeApproveFirst, Version: "1.0.0", HeldVersion: "2.0.0"}, "", "1.0.0", false},
		{"pinned to the installed version",
			&CampaignUpdateRow{Mode: UpdateModePinned, Version: "2.0.0"}, "", "2.0.0", false},
		{"pinned folder missing falls back to installed",
			&CampaignUpdateRow{Mode: UpdateModePinned, Version: "1.0.0"}, "1.0.0", "2.0.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newUpdEnv(t, "2.0.0", []string{"1.0.0", "2.0.0"}, "c1")
			if tc.row != nil {
				r := *tc.row
				r.CampaignID, r.PackageID = "c1", "p1"
				e.repo.rows[ukey("c1", "p1")] = &r
			}
			if tc.removeDir != "" {
				if err := os.RemoveAll(filepath.Join(e.pkgs.root, "sys", tc.removeDir)); err != nil {
					t.Fatal(err)
				}
			}
			pkg := e.pkgs.pkgs[0]
			got := e.svc.ServedVersion(ctx(), "c1", &pkg)
			wantDir := filepath.Join(e.pkgs.root, "sys", tc.wantVersion)
			if got.Version != tc.wantVersion || got.Dir != wantDir || got.FellBack != tc.wantFellBack {
				t.Errorf("got %+v, want version %q dir %q fellBack %v", got, tc.wantVersion, wantDir, tc.wantFellBack)
			}
			if tc.wantFellBack && got.Reason == "" {
				t.Error("a fallback must say why")
			}
		})
	}
}

func TestCampaignsOnPackageAndHeldUpdates(t *testing.T) {
	e := newUpdEnv(t, "2.0.0", []string{"1.0.0", "2.0.0"}, "c1", "c2", "c3")
	e.repo.rows[ukey("c2", "p1")] = &CampaignUpdateRow{CampaignID: "c2", PackageID: "p1", Mode: UpdateModePinned, Version: "1.0.0"}
	e.repo.rows[ukey("c3", "p1")] = &CampaignUpdateRow{CampaignID: "c3", PackageID: "p1", Mode: UpdateModeApproveFirst, Version: "1.0.0", HeldVersion: "2.0.0"}

	states, err := e.svc.CampaignsOnPackage(ctx(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		mode    UpdateMode
		version string
		held    string
	}{
		"c1": {UpdateModeAutomatic, "2.0.0", ""},
		"c2": {UpdateModePinned, "1.0.0", ""},
		"c3": {UpdateModeApproveFirst, "1.0.0", "2.0.0"},
	}
	if len(states) != len(want) {
		t.Fatalf("got %d campaigns, want %d: %+v", len(states), len(want), states)
	}
	for _, s := range states {
		w := want[s.CampaignID]
		if s.Mode != w.mode || s.EffectiveVersion != w.version || s.HeldVersion != w.held || s.CampaignName == "" {
			t.Errorf("%s = %+v, want %+v", s.CampaignID, s, w)
		}
	}

	held, err := e.svc.HeldUpdates(ctx())
	if err != nil || len(held) != 1 || held[0].CampaignID != "c3" || held[0].HeldVersion != "2.0.0" {
		t.Fatalf("HeldUpdates = %+v, %v", held, err)
	}
}

func TestReconcile(t *testing.T) {
	e := newUpdEnv(t, "2.0.0", []string{"1.0.0", "2.0.0"}, "c1", "c2", "c3", "c4")
	rows := []CampaignUpdateRow{
		{CampaignID: "c1", Mode: UpdateModeApproveFirst, Version: "1.0.0"},                       // behind, no hold -> hold 2.0.0
		{CampaignID: "c2", Mode: UpdateModeApproveFirst, Version: "2.0.0", HeldVersion: "2.0.0"}, // caught up -> clear
		{CampaignID: "c3", Mode: UpdateModePinned, Version: "1.0.0", HeldVersion: "2.0.0"},       // not asking -> clear
		{CampaignID: "c4", Mode: UpdateModeAutomatic},                                            // says nothing -> removed
	}
	for i := range rows {
		rows[i].PackageID = "p1"
		e.repo.rows[ukey(rows[i].CampaignID, "p1")] = &rows[i]
	}

	res, err := e.svc.Reconcile(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if e.repo.rows[ukey("c1", "p1")].HeldVersion != "2.0.0" {
		t.Error("c1 should be handed the installed version")
	}
	if e.repo.rows[ukey("c2", "p1")].HeldVersion != "" || e.repo.rows[ukey("c3", "p1")].HeldVersion != "" {
		t.Error("stale holds should be cleared")
	}
	if _, ok := e.repo.rows[ukey("c4", "p1")]; ok {
		t.Error("an empty row should be removed")
	}
	if res.HeldSet != 1 || res.HeldCleared != 2 || res.RowsRemoved != 1 {
		t.Errorf("result = %+v", res)
	}

	// Idempotent: a second pass changes nothing.
	again, err := e.svc.Reconcile(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if again != (ReconcileResult{}) {
		t.Errorf("second pass should change nothing, got %+v", again)
	}
}

// Today's behaviour: with no stored choice at all, the reconciler writes
// nothing and every campaign stays automatic.
func TestReconcileLeavesExistingCampaignsUntouched(t *testing.T) {
	e := newUpdEnv(t, "2.0.0", []string{"2.0.0"}, "c1", "c2")
	res, err := e.svc.Reconcile(ctx())
	if err != nil {
		t.Fatal(err)
	}
	if res != (ReconcileResult{}) || len(e.repo.rows) != 0 {
		t.Fatalf("expected no changes, got %+v rows=%+v", res, e.repo.rows)
	}
	st, _ := e.svc.CampaignState(ctx(), "c1", "p1")
	if st.Mode != UpdateModeAutomatic || st.EffectiveVersion != "2.0.0" {
		t.Errorf("state = %+v", st)
	}
}

// --- clean-up protection ---

func TestPrune_ProtectsEveryCampaignVersion(t *testing.T) {
	svc, slugDir := prunePkgEnv(t, "0.13.4", []string{"0.0.7", "0.12.0", "0.13.0", "0.13.4"})
	svc.loadedDirsFn = func() map[string]bool { return nil }
	svc.campaignVersionsFn = func(context.Context) (map[string]map[string]bool, error) {
		return map[string]map[string]bool{"drawsteel": {"0.12.0": true, "0.13.0": true}, "other": {"0.0.7": true}}, nil
	}
	res, err := svc.PruneStaleVersions(ctx(), 1, false)
	if err != nil {
		t.Fatal(err)
	}
	got := staleVersions(res)
	if !got["0.0.7"] || len(res.Reclaimable) != 1 {
		t.Fatalf("only 0.0.7 (no campaign of this package is on it) may go, got %v", got)
	}
	for _, v := range []string{"0.12.0", "0.13.0", "0.13.4"} {
		if _, err := os.Stat(filepath.Join(slugDir, v)); err != nil {
			t.Errorf("%s must survive: %v", v, err)
		}
	}
}

func TestPrune_FailsClosedOnCampaignVersionsProblem(t *testing.T) {
	cases := []struct {
		name string
		fn   func(context.Context) (map[string]map[string]bool, error)
	}{
		{"provider errors", func(context.Context) (map[string]map[string]bool, error) { return nil, errors.New("db down") }},
		{"provider not wired", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, slugDir := prunePkgEnv(t, "0.13.4", []string{"0.0.7", "0.13.4"})
			svc.loadedDirsFn = func() map[string]bool { return nil }
			svc.campaignVersionsFn = tc.fn
			res, err := svc.PruneStaleVersions(ctx(), 1, false)
			if err == nil || res != nil {
				t.Fatalf("must refuse, got res=%v err=%v", res, err)
			}
			if _, statErr := os.Stat(filepath.Join(slugDir, "0.0.7")); statErr != nil {
				t.Error("nothing may be deleted when the campaign versions are unknown")
			}
		})
	}
}

// storedElsewhereBinding stands in for a type whose mode and version live
// outside campaign_package_updates (the Foundry module's settings): only the
// held version is kept by the service.
type storedElsewhereBinding struct {
	states     map[string]*CampaignPackageState
	failFreeze bool
	applies    int // Apply calls: a freeze must never go through Apply
}

func (b *storedElsewhereBinding) Freeze(_ context.Context, id string, _ *Package, _ UpdateMode, v string) error {
	if b.failFreeze {
		return errors.New("pin write failed")
	}
	b.states[id].Version = v // only the version; the stored mode is untouched
	return nil
}

func (b *storedElsewhereBinding) PackageType() PackageType { return PackageTypeFoundryModule }
func (b *storedElsewhereBinding) UsedBy(context.Context, string, *Package) (bool, error) {
	return true, nil
}
func (b *storedElsewhereBinding) State(_ context.Context, id string, _ *Package) (CampaignPackageState, error) {
	return *b.states[id], nil
}
func (b *storedElsewhereBinding) Campaigns(context.Context, *Package) ([]CampaignPackageState, error) {
	var out []CampaignPackageState
	for id, s := range b.states {
		c := *s
		c.CampaignID = id
		out = append(out, c)
	}
	return out, nil
}
func (b *storedElsewhereBinding) Apply(_ context.Context, id string, _ *Package, m UpdateMode, v string, _ ActorInfo) error {
	b.applies++
	b.states[id] = &CampaignPackageState{Mode: m, Version: v}
	return nil
}

func TestOnInstallWithABindingThatStoresModeElsewhere(t *testing.T) {
	pkgs := &fakeUpdatePkgs{root: t.TempDir(), usage: map[string][]PackageUsage{}}
	pkgs.pkgs = []Package{{ID: "f1", Type: PackageTypeFoundryModule, Slug: "mod", InstalledVersion: "1.0.0", Status: StatusApproved}}
	pkgs.mkVersions(t, "mod", "1.0.0", "2.0.0")
	repo := newFakeUpdateRepo()
	svc := newCampaignUpdateService(repo, pkgs, nil)
	bind := &storedElsewhereBinding{states: map[string]*CampaignPackageState{
		"auto":     {Mode: UpdateModeAutomatic},
		"preserve": {Mode: UpdateModePinned},                          // legacy preserve, no pin yet
		"pinned":   {Mode: UpdateModePinned, Version: "1.0.0"},        // has its own pin
		"asking":   {Mode: UpdateModeApproveFirst, Version: "1.0.0"},  // pinned and asking first
		"fresh":    {Mode: UpdateModeApproveFirst, Explicit: true},    // asking first, tracking latest
	}}
	svc.RegisterBinding(bind)

	pkg := pkgs.pkgs[0]
	if err := svc.OnInstall(ctx(), &pkg, "1.0.0", "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if bind.applies != 0 {
		t.Error("freezing must set only the version, never rewrite a stored mode")
	}
	pkg.InstalledVersion = "2.0.0"
	if err := svc.OnInstalled(ctx(), &pkg, "1.0.0", "2.0.0"); err != nil {
		t.Fatal(err)
	}
	wantVersion := map[string]string{"auto": "", "preserve": "1.0.0", "pinned": "1.0.0", "asking": "1.0.0", "fresh": "1.0.0"}
	for id, want := range wantVersion {
		if got := bind.states[id].Version; got != want {
			t.Errorf("%s: version %q, want %q", id, got, want)
		}
	}
	for id, want := range map[string]string{"asking": "2.0.0", "fresh": "2.0.0"} {
		if got := repo.rows[ukey(id, "f1")].HeldVersion; got != want {
			t.Errorf("%s: held %q, want %q", id, got, want)
		}
	}
	for _, id := range []string{"auto", "preserve", "pinned"} {
		if r := repo.rows[ukey(id, "f1")]; r != nil {
			t.Errorf("%s must hold nothing, got %+v", id, r)
		}
	}

	// Approving moves the campaign through the binding and clears the hold.
	if _, err := svc.ApproveHeld(ctx(), "asking", "f1", ActorInfo{Admin: true}); err != nil {
		t.Fatal(err)
	}
	if got := bind.states["asking"]; got.Version != "2.0.0" || got.Mode != UpdateModeApproveFirst {
		t.Errorf("after approval: %+v", got)
	}
	if repo.rows[ukey("asking", "f1")].HeldVersion != "" {
		t.Error("approval must clear the hold")
	}
}

// A campaign whose mode is derived from older stored data keeps the older
// best-effort rule: a failed freeze is logged, not a refused install. One set
// through the update modes fails closed.
func TestOnInstallFreezeFailureLegacyVersusExplicit(t *testing.T) {
	cases := []struct {
		name     string
		explicit bool
		wantErr  bool
	}{
		{"derived from older data: carry on", false, false},
		{"explicitly set: refuse the install", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkgs := &fakeUpdatePkgs{root: t.TempDir(), usage: map[string][]PackageUsage{}}
			pkgs.pkgs = []Package{{ID: "f1", Type: PackageTypeFoundryModule, Slug: "mod", InstalledVersion: "1.0.0", Status: StatusApproved}}
			pkgs.mkVersions(t, "mod", "1.0.0", "2.0.0")
			svc := newCampaignUpdateService(newFakeUpdateRepo(), pkgs, nil)
			svc.RegisterBinding(&storedElsewhereBinding{
				failFreeze: true,
				states:     map[string]*CampaignPackageState{"c": {Mode: UpdateModePinned, Explicit: tc.explicit}},
			})
			pkg := pkgs.pkgs[0]
			err := svc.OnInstall(ctx(), &pkg, "1.0.0", "2.0.0")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
