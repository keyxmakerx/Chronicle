package packages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func intp(n int) *int { return &n }

func TestParseRetentionSettings(t *testing.T) {
	cur := DefaultRetentionSettings()
	cur.KeepNewest, cur.UnusedDays = 5, 60

	tests := []struct {
		name             string
		mode, keep, days string
		want             RetentionSettings
		wantErr          bool
	}{
		{"manual default", "manual", "2", "30", RetentionSettings{RetentionManual, 2, 30}, false},
		{"keep newest", "keep_newest", "3", "30", RetentionSettings{RetentionKeepNewest, 3, 30}, false},
		{"unused days", "unused_days", "2", "90", RetentionSettings{RetentionUnusedDays, 2, 90}, false},
		{"zero keep rejected", "keep_newest", "0", "30", cur, true},
		{"negative keep rejected", "keep_newest", "-4", "30", cur, true},
		{"absurd keep rejected", "keep_newest", "1001", "30", cur, true},
		{"absurd days rejected", "unused_days", "2", "5000", cur, true},
		{"zero days rejected", "unused_days", "2", "0", cur, true},
		{"blank active number rejected", "keep_newest", "", "30", cur, true},
		{"unknown mode rejected", "delete_all", "2", "30", cur, true},
		{"max boundary accepted", "keep_newest", "1000", "30", RetentionSettings{RetentionKeepNewest, 1000, 30}, false},
		{"bad inactive number keeps stored value", "keep_newest", "3", "abc", RetentionSettings{RetentionKeepNewest, 3, 60}, false},
		{"out-of-range inactive number keeps stored value", "unused_days", "9999", "10", RetentionSettings{RetentionUnusedDays, 5, 10}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRetentionSettings(tc.mode, tc.keep, tc.days, cur)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				var ae *apperror.AppError
				if !errors.As(err, &ae) {
					t.Errorf("error must be an apperror, got %T", err)
				}
				return
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDefaultRetentionIsManual(t *testing.T) {
	d := DefaultRetentionSettings()
	if d.Mode != RetentionManual || d.KeepNewest != 2 || d.UnusedDays != 30 {
		t.Fatalf("defaults wrong: %+v", d)
	}
	// An install with no stored settings, and one with no reader at all,
	// both resolve to manual.
	svc := NewPackageService(newFakeRepo(), newOfflineGitHubClient(), t.TempDir(), "http://x").(*packageService)
	got, _ := svc.GetRetentionSettings(context.Background())
	if got.Mode != RetentionManual {
		t.Errorf("no settings reader: mode = %q, want manual", got.Mode)
	}
}

func TestEffectiveRetentionPrecedence(t *testing.T) {
	site := RetentionSettings{RetentionUnusedDays, 2, 30}
	tests := []struct {
		name string
		pkg  Package
		site RetentionSettings
		want retentionRule
	}{
		{"site rule applies", Package{Type: PackageTypeSystem}, site, retentionRule{Mode: RetentionUnusedDays, KeepNewest: 2, UnusedDays: 30}},
		{"override beats site", Package{Type: PackageTypeSystem, RetentionKeepNewest: intp(4)}, site, retentionRule{Mode: RetentionKeepNewest, KeepNewest: 4}},
		{"override applies even when site is manual", Package{Type: PackageTypeSystem, RetentionKeepNewest: intp(1)}, DefaultRetentionSettings(), retentionRule{Mode: RetentionKeepNewest, KeepNewest: 1}},
		{"foundry ignores site rule", Package{Type: PackageTypeFoundryModule}, site, retentionRule{Mode: RetentionManual}},
		{"foundry ignores override", Package{Type: PackageTypeFoundryModule, RetentionKeepNewest: intp(1)}, site, retentionRule{Mode: RetentionManual}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveRetention(tc.site, &tc.pkg); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRetentionSummary(t *testing.T) {
	tests := []struct {
		name string
		site RetentionSettings
		pkg  Package
		want string
	}{
		{"keep newest", RetentionSettings{RetentionKeepNewest, 2, 30}, Package{}, "Old versions follow the site rule: keep the newest 2."},
		{"unused", RetentionSettings{RetentionUnusedDays, 2, 30}, Package{}, "Old versions follow the site rule: remove versions unused for 30 days."},
		{"manual", DefaultRetentionSettings(), Package{}, "only removed when you click Clean up"},
		{"own", DefaultRetentionSettings(), Package{RetentionKeepNewest: intp(3)}, "own rule: keep the newest 3."},
		{"foundry", RetentionSettings{RetentionKeepNewest, 2, 30}, Package{Type: PackageTypeFoundryModule}, "never removed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RetentionSummary(tc.site, &tc.pkg); !strings.Contains(got, tc.want) {
				t.Errorf("summary %q lacks %q", got, tc.want)
			}
		})
	}
}

func TestSelectRemovable(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	ago := func(d int) time.Time { return now.Add(-time.Duration(d) * day) }

	// Published order differs from semver on purpose: 1.0.5 is a late
	// backport and must count as newer than 2.0.0 for "newest by date".
	folders := []versionFolder{
		{Name: "1.0.0", Published: ago(300), Modified: ago(200)},
		{Name: "1.1.0", Published: ago(200), Modified: ago(100)},
		{Name: "2.0.0", Published: ago(100), Modified: ago(5)},
		{Name: "1.0.5", Published: ago(50), Modified: ago(40)},
	}
	tests := []struct {
		name              string
		installed, pinned string
		rule              retentionRule
		folders           []versionFolder
		want              []string
	}{
		{"manual nominates nothing", "2.0.0", "", retentionRule{Mode: RetentionManual}, folders, nil},
		{"keep 1 by published date", "2.0.0", "", retentionRule{Mode: RetentionKeepNewest, KeepNewest: 1}, folders, []string{"1.1.0", "1.0.0"}},
		{"keep 3", "2.0.0", "", retentionRule{Mode: RetentionKeepNewest, KeepNewest: 3}, folders, []string{"1.0.0"}},
		{"keep more than exist", "2.0.0", "", retentionRule{Mode: RetentionKeepNewest, KeepNewest: 9}, folders, nil},
		{"installed protected even when oldest", "1.0.0", "", retentionRule{Mode: RetentionKeepNewest, KeepNewest: 1}, folders, []string{"2.0.0", "1.1.0"}},
		{"pinned protected", "2.0.0", "1.0.0", retentionRule{Mode: RetentionKeepNewest, KeepNewest: 1}, folders, []string{"1.1.0"}},
		{"served protected", "2.0.0", "", retentionRule{Mode: RetentionKeepNewest, KeepNewest: 1}, []versionFolder{
			{Name: "1.0.0", Published: ago(300)}, {Name: "1.1.0", Published: ago(200), Served: true}, {Name: "2.0.0", Published: ago(100)},
		}, []string{"1.0.0"}},
		{"unused 30 days", "2.0.0", "", retentionRule{Mode: RetentionUnusedDays, UnusedDays: 30}, folders, []string{"1.0.0", "1.1.0", "1.0.5"}},
		{"unused 150 days", "2.0.0", "", retentionRule{Mode: RetentionUnusedDays, UnusedDays: 150}, folders, []string{"1.0.0"}},
		{"unused: exactly at cut-off is removed", "2.0.0", "", retentionRule{Mode: RetentionUnusedDays, UnusedDays: 40}, folders, []string{"1.0.0", "1.1.0", "1.0.5"}},
		{"unused: one day inside cut-off is kept", "2.0.0", "", retentionRule{Mode: RetentionUnusedDays, UnusedDays: 41}, folders, []string{"1.0.0", "1.1.0"}},
		{"unused: installed and pinned protected", "1.0.0", "1.1.0", retentionRule{Mode: RetentionUnusedDays, UnusedDays: 30}, folders, []string{"1.0.5"}},
		{"unused: since later than mtime wins", "2.0.0", "", retentionRule{Mode: RetentionUnusedDays, UnusedDays: 30, Since: ago(10)}, folders, nil},
		{"unused: since earlier than mtime is ignored", "2.0.0", "", retentionRule{Mode: RetentionUnusedDays, UnusedDays: 30, Since: ago(1000)}, folders, []string{"1.0.0", "1.1.0", "1.0.5"}},
		{"unused: unknown mtime kept", "2.0.0", "", retentionRule{Mode: RetentionUnusedDays, UnusedDays: 1}, []versionFolder{{Name: "1.0.0"}, {Name: "2.0.0"}}, nil},
		{"undated folder ranks newest (kept)", "2.0.0", "", retentionRule{Mode: RetentionKeepNewest, KeepNewest: 2}, []versionFolder{
			{Name: "0.9.0"}, {Name: "1.0.0", Published: ago(10)}, {Name: "2.0.0", Published: ago(5)},
		}, []string{"1.0.0"}},
		{"no dates falls back to semver", "2.0.0", "", retentionRule{Mode: RetentionKeepNewest, KeepNewest: 1}, []versionFolder{
			{Name: "1.9.0"}, {Name: "1.10.0"}, {Name: "2.0.0"},
		}, []string{"1.10.0", "1.9.0"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := selectRemovable(tc.folders, tc.installed, tc.pinned, tc.rule, now)
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) && (len(got) != 0 || len(want) != 0) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

// retentionEnv is prunePkgEnv with the folders back-dated and a published
// date per version, so the end-to-end run exercises both rules.
func retentionEnv(t *testing.T, mutate func(*Package)) (*packageService, *fakeRepo, string) {
	t.Helper()
	svc, slugDir := prunePkgEnv(t, "2.0.0", []string{"1.0.0", "1.1.0", "2.0.0"})
	repo := svc.repo.(*fakeRepo)
	svc.loadedDirsFn = func() map[string]bool { return nil }
	now := time.Now()
	for v, age := range map[string]int{"1.0.0": 90, "1.1.0": 10, "2.0.0": 1} {
		ts := now.Add(-time.Duration(age) * 24 * time.Hour)
		if err := os.Chtimes(filepath.Join(slugDir, v), ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	repo.listed = []PackageVersion{
		{Version: "1.0.0", PublishedAt: now.AddDate(0, 0, -300)},
		{Version: "1.1.0", PublishedAt: now.AddDate(0, 0, -200)},
		{Version: "2.0.0", PublishedAt: now.AddDate(0, 0, -100)},
	}
	if mutate != nil {
		mutate(repo.packages["p1"])
	}
	return svc, repo, slugDir
}

type memSettings map[string]string

func (m memSettings) Get(_ context.Context, k string) (string, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}
func (m memSettings) Set(_ context.Context, k, v string) error { m[k] = v; return nil }

func existing(t *testing.T, dir string, vs ...string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, v := range vs {
		_, err := os.Stat(filepath.Join(dir, v))
		out[v] = err == nil
	}
	return out
}

func TestRunRetention(t *testing.T) {
	tests := []struct {
		name     string
		settings memSettings
		mutate   func(*Package)
		wantGone []string
	}{
		{"default manual deletes nothing", memSettings{}, nil, nil},
		{"site keep newest 1", memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "1"}, nil, []string{"1.0.0", "1.1.0"}},
		{"site unused 30 days", memSettings{settingRetentionMode: "unused_days", settingRetentionUnusedDays: "30", settingRetentionSince: "2020-01-01T00:00:00Z"}, nil, []string{"1.0.0"}},
		{"unused days just enabled: nothing is old yet", memSettings{settingRetentionMode: "unused_days", settingRetentionUnusedDays: "30", settingRetentionSince: time.Now().Format(time.RFC3339)}, nil, nil},
		{"unused days without start stamp fails closed", memSettings{settingRetentionMode: "unused_days", settingRetentionUnusedDays: "30"}, nil, nil},
		{"unused days unparsable stamp fails closed", memSettings{settingRetentionMode: "unused_days", settingRetentionUnusedDays: "30", settingRetentionSince: "garbage"}, nil, nil},
		{"keep newest with missing number fails closed", memSettings{settingRetentionMode: "keep_newest"}, nil, nil},
		{"override still runs when stamp missing", memSettings{settingRetentionMode: "unused_days", settingRetentionUnusedDays: "30"}, func(p *Package) { p.RetentionKeepNewest = intp(2) }, []string{"1.0.0"}},
		{"override beats manual site", memSettings{}, func(p *Package) { p.RetentionKeepNewest = intp(2) }, []string{"1.0.0"}},
		{"override beats site rule", memSettings{settingRetentionMode: "unused_days", settingRetentionUnusedDays: "1", settingRetentionSince: "2020-01-01T00:00:00Z"}, func(p *Package) { p.RetentionKeepNewest = intp(3) }, nil},
		{"pinned survives", memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "1"}, func(p *Package) { p.PinnedVersion = "1.0.0" }, []string{"1.1.0"}},
		{"foundry untouched", memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "1"}, func(p *Package) { p.Type = PackageTypeFoundryModule }, nil},
		{"invalid stored rule falls back to manual", memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "0"}, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, slugDir := retentionEnv(t, tc.mutate)
			svc.settings = tc.settings
			if _, err := svc.RunRetention(context.Background()); err != nil {
				t.Fatalf("RunRetention: %v", err)
			}
			gone := map[string]bool{}
			for _, v := range tc.wantGone {
				gone[v] = true
			}
			for v, present := range existing(t, slugDir, "1.0.0", "1.1.0", "2.0.0") {
				if present == gone[v] {
					t.Errorf("version %s present=%v, want present=%v", v, present, !gone[v])
				}
			}
			if v := existing(t, slugDir, "2.0.0"); !v["2.0.0"] {
				t.Error("installed version must never be removed")
			}
			// Idempotent: a second run changes nothing.
			res, err := svc.RunRetention(context.Background())
			if err != nil || len(res.Removed) != 0 {
				t.Errorf("second run must be a no-op, got %+v err %v", res, err)
			}
		})
	}
}

func TestRunRetentionFailsClosedWithoutLoaderSignal(t *testing.T) {
	svc, _, slugDir := retentionEnv(t, nil)
	svc.loadedDirsFn = nil
	svc.settings = memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "1"}
	if _, err := svc.RunRetention(context.Background()); err == nil {
		t.Fatal("must fail closed")
	}
	if !existing(t, slugDir, "1.0.0")["1.0.0"] {
		t.Error("nothing may be deleted when failing closed")
	}
}

func TestSaveAndSetRetention(t *testing.T) {
	svc, repo, _ := retentionEnv(t, nil)
	store := memSettings{}
	svc.settings, svc.settingsWriter = store, store
	ctx := context.Background()

	if err := svc.SaveRetentionSettings(ctx, RetentionSettings{RetentionKeepNewest, 0, 30}); err == nil {
		t.Error("keep newest 0 must be rejected")
	}
	if len(store) != 0 {
		t.Error("a rejected save must store nothing")
	}
	if err := svc.SaveRetentionSettings(ctx, RetentionSettings{RetentionUnusedDays, 2, 45}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetRetentionSettings(ctx); got.Mode != RetentionUnusedDays || got.UnusedDays != 45 {
		t.Errorf("round trip failed: %+v", got)
	}

	for _, n := range []int{0, -1, 1001} {
		if err := svc.SetPackageRetention(ctx, "p1", intp(n)); err == nil {
			t.Errorf("override %d must be rejected", n)
		}
	}
	if err := svc.SetPackageRetention(ctx, "p1", intp(3)); err != nil {
		t.Fatal(err)
	}
	if p := repo.packages["p1"]; p.RetentionKeepNewest == nil || *p.RetentionKeepNewest != 3 {
		t.Error("override not stored")
	}
	if err := svc.SetPackageRetention(ctx, "p1", nil); err != nil {
		t.Fatal(err)
	}
	if repo.packages["p1"].RetentionKeepNewest != nil {
		t.Error("nil must clear the override")
	}
	if err := svc.SetPackageRetention(ctx, "nope", intp(2)); err == nil {
		t.Error("unknown package must error")
	}
	repo.packages["p1"].Type = PackageTypeFoundryModule
	if err := svc.SetPackageRetention(ctx, "p1", intp(2)); err == nil {
		t.Error("foundry module override must be refused")
	}
}

// failingGet errors on one key and serves the rest, standing in for a
// transient settings-store failure.
type failingGet struct {
	memSettings
	failKey string
}

func (f failingGet) Get(ctx context.Context, k string) (string, error) {
	if k == f.failKey {
		return "", errors.New("db down")
	}
	return f.memSettings.Get(ctx, k)
}

func TestGetRetentionSettingsFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		store    SettingsReader
		wantMode RetentionMode
	}{
		{"keep newest ok", memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "3"}, RetentionKeepNewest},
		{"keep newest number read error", failingGet{memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "3"}, settingRetentionKeepNewest}, RetentionManual},
		{"keep newest number missing", memSettings{settingRetentionMode: "keep_newest"}, RetentionManual},
		{"keep newest number invalid", memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "0"}, RetentionManual},
		{"unused number read error", failingGet{memSettings{settingRetentionMode: "unused_days", settingRetentionUnusedDays: "30"}, settingRetentionUnusedDays}, RetentionManual},
		{"mode read error", failingGet{memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "3"}, settingRetentionMode}, RetentionManual},
		{"other mode's number error is irrelevant", failingGet{memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "3", settingRetentionUnusedDays: "9"}, settingRetentionUnusedDays}, RetentionKeepNewest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewPackageService(newFakeRepo(), newOfflineGitHubClient(), t.TempDir(), "http://x").(*packageService)
			svc.settings = tc.store
			got, _ := svc.GetRetentionSettings(context.Background())
			if got.Mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", got.Mode, tc.wantMode)
			}
		})
	}
}

// orderedWriter records Set calls in order.
type orderedWriter struct {
	memSettings
	order []string
}

func (o *orderedWriter) Set(ctx context.Context, k, v string) error {
	o.order = append(o.order, k)
	return o.memSettings.Set(ctx, k, v)
}

func TestSaveRetentionWritesModeLastAndStampsUnusedDays(t *testing.T) {
	tests := []struct {
		name      string
		start     memSettings
		save      RetentionSettings
		wantSince bool
	}{
		{"keep newest", memSettings{}, RetentionSettings{RetentionKeepNewest, 2, 30}, false},
		{"first enable of unused days stamps", memSettings{}, RetentionSettings{RetentionUnusedDays, 2, 30}, true},
		{"switch from keep newest to unused days stamps", memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "2"}, RetentionSettings{RetentionUnusedDays, 2, 30}, true},
		{"re-saving unused days keeps the stamp", memSettings{settingRetentionMode: "unused_days", settingRetentionUnusedDays: "30", settingRetentionSince: "2020-01-01T00:00:00Z"}, RetentionSettings{RetentionUnusedDays, 2, 45}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &orderedWriter{memSettings: tc.start}
			svc := NewPackageService(newFakeRepo(), newOfflineGitHubClient(), t.TempDir(), "http://x").(*packageService)
			svc.settings, svc.settingsWriter = w.memSettings, w
			if err := svc.SaveRetentionSettings(context.Background(), tc.save); err != nil {
				t.Fatal(err)
			}
			if len(w.order) == 0 || w.order[len(w.order)-1] != settingRetentionMode {
				t.Errorf("mode must be written last, order = %v", w.order)
			}
			stamped := false
			for _, k := range w.order {
				if k == settingRetentionSince {
					stamped = true
				}
			}
			if stamped != tc.wantSince {
				t.Errorf("stamped = %v, want %v (order %v)", stamped, tc.wantSince, w.order)
			}
		})
	}
}

// TestPinTakesPackageLockAndProtectsPinned: pinning takes the per-package
// lock, and a pinned version is never removed by the clean-up.
func TestPinTakesPackageLockAndProtectsPinned(t *testing.T) {
	svc, repo, slugDir := retentionEnv(t, nil)
	repo.versions["p1@1.0.0"] = &PackageVersion{Version: "1.0.0"}
	svc.settings = memSettings{settingRetentionMode: "keep_newest", settingRetentionKeepNewest: "1"}

	mu := svc.lockForPackage("p1")
	mu.Lock()
	done := make(chan error, 1)
	go func() { done <- svc.SetPinnedVersion(context.Background(), "p1", "1.0.0") }()
	select {
	case <-done:
		t.Fatal("pin completed while the package lock was held")
	case <-time.After(100 * time.Millisecond):
	}
	mu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunRetention(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !existing(t, slugDir, "1.0.0")["1.0.0"] {
		t.Error("a pinned version must survive the clean-up")
	}
}
