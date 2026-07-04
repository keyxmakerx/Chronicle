// Tests for the C-FMC-6 auto-pin flows: install-hook + one-time
// migration. Contracts pinned:
//
//   - AutoPinOnInstall is a no-op when previousVersion is empty
//     (first-ever install — no prior state to preserve).
//   - AutoPinOnInstall is a no-op when previousVersion == newVersion
//     (re-install of the same version; defensive guard).
//   - AutoPinOnInstall pins every auto-tracking campaign to
//     previousVersion + logs one EventModuleAutoPinOnInstall per
//     campaign + one EventModuleAutoPinInstallSummary total.
//   - Pin failures for individual campaigns don't abort the fan-out;
//     the summary event still fires with the accurate affected count.
//   - AutoPinMigrate is idempotent — second call returns immediately
//     without re-running.
//   - AutoPinMigrate is a no-op when no foundry-module package is
//     registered (logs + returns nil; future boot retries).
package foundry_vtt

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// fakeRepoForAutoPin stubs the Repository interface focusing on
// CampaignsWithEmptyPin (the only method auto-pin paths exercise).
type fakeRepoForAutoPin struct {
	emptyPinCampaigns []CampaignUsage
}

func (r *fakeRepoForAutoPin) GetCampaignToken(_ context.Context, _ string) (*CampaignToken, error) {
	return nil, nil
}
func (r *fakeRepoForAutoPin) UpsertCampaignToken(_ context.Context, _ *CampaignToken) error {
	return nil
}
func (r *fakeRepoForAutoPin) BumpCampaignTokenVersion(_ context.Context, _ string) (int, error) {
	return 1, nil
}
func (r *fakeRepoForAutoPin) CampaignsUsingVersion(_ context.Context, _ string) ([]CampaignUsage, error) {
	return nil, nil
}
func (r *fakeRepoForAutoPin) CampaignsOlderThan(_ context.Context, _ string, _ func(a, b string) bool) ([]CampaignUsage, error) {
	return nil, nil
}
func (r *fakeRepoForAutoPin) CampaignsWithEmptyPin(_ context.Context) ([]CampaignUsage, error) {
	return r.emptyPinCampaigns, nil
}

// recordingSettings extends fakeSettings to record every SetFoundryModulePin call.
type recordingSettings struct {
	mu    sync.Mutex
	calls []recordedPin
	err   error // returned by SetFoundryModulePin if non-nil
	// modes maps campaignID → pin_mode. A missing entry (or a nil map)
	// returns "" — the promote default — so tests can exercise preserve
	// vs promote per campaign.
	modes map[string]string
}

type recordedPin struct {
	campaignID string
	version    string
}

func (s *recordingSettings) GetFoundryModulePin(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (s *recordingSettings) SetFoundryModulePin(_ context.Context, campaignID, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.calls = append(s.calls, recordedPin{campaignID: campaignID, version: version})
	return nil
}

// pin_mode stubs added in C-FMC-ADMIN-UX-AUDIT Chunk 1. Existing
// auto_pin_test.go cases predate Chunk 2's hook behavior change, so
// the stubs return zero/no-op values — the test cases don't exercise
// pin_mode yet. New Chunk 1 tests (pin_mode_test.go) exercise the
// settings adapter contract via a dedicated stub instead of reusing
// this one.
func (s *recordingSettings) GetFoundryModulePinMode(_ context.Context, campaignID string) (string, error) {
	if s.modes != nil {
		return s.modes[campaignID], nil
	}
	return "", nil
}
func (s *recordingSettings) SetFoundryModulePinMode(_ context.Context, _, _ string) error {
	return nil
}

func (s *recordingSettings) CampaignExists(_ context.Context, _ string) (bool, error) {
	return true, nil
}

// newAutoPinTestService wires the service struct with the fakes
// needed for auto-pin testing. Bypasses NewService since we don't
// need tokens/pkgs/baseURL for the auto-pin code paths.
func newAutoPinTestService(t *testing.T, emptyPinCampaigns []CampaignUsage) (*service, *recordingSettings, *fakeEventLogger) {
	t.Helper()
	settings := &recordingSettings{}
	events := &fakeEventLogger{}
	svc := &service{
		repo:     &fakeRepoForAutoPin{emptyPinCampaigns: emptyPinCampaigns},
		settings: settings,
		events:   events,
	}
	return svc, settings, events
}

// TestAutoPinOnInstall_NoopOnFirstInstall — empty previousVersion
// means no prior state to preserve. The hook returns 0 affected
// without touching any campaigns or logging events.
func TestAutoPinOnInstall_NoopOnFirstInstall(t *testing.T) {
	svc, settings, events := newAutoPinTestService(t, []CampaignUsage{
		{CampaignID: "camp-1", CampaignName: "C1"},
	})
	n, err := svc.AutoPinOnInstall(context.Background(), "", "v0.1.10", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 affected on first install, got %d", n)
	}
	if len(settings.calls) != 0 {
		t.Errorf("no pins should be written on first install, got %d", len(settings.calls))
	}
	if len(events.events) != 0 {
		t.Errorf("no events should be logged on first install, got %d", len(events.events))
	}
}

// TestAutoPinOnInstall_NoopOnSameVersion — previousVersion ==
// newVersion is a re-install. No state transition; no auto-pin.
// Defensive guard against InstallVersion re-firing the hook
// without an actual version change.
func TestAutoPinOnInstall_NoopOnSameVersion(t *testing.T) {
	svc, settings, _ := newAutoPinTestService(t, []CampaignUsage{
		{CampaignID: "camp-1", CampaignName: "C1"},
	})
	n, err := svc.AutoPinOnInstall(context.Background(), "v0.1.10", "v0.1.10", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 affected on same-version re-install, got %d", n)
	}
	if len(settings.calls) != 0 {
		t.Errorf("no pins should be written on same-version re-install, got %d", len(settings.calls))
	}
}

// TestAutoPinOnInstall_PreserveModeFreezes — campaigns explicitly in
// "preserve" mode are pinned to the previous version on install.
// previousVersion=v0.1.10, newVersion=v0.1.11, 3 preserve-mode
// campaigns. All three get pinned to v0.1.10; per-campaign events
// fire + one summary event.
func TestAutoPinOnInstall_PreserveModeFreezes(t *testing.T) {
	campaigns := []CampaignUsage{
		{CampaignID: "camp-1", CampaignName: "Imix"},
		{CampaignID: "camp-2", CampaignName: "Test"},
		{CampaignID: "camp-3", CampaignName: "Third"},
	}
	svc, settings, events := newAutoPinTestService(t, campaigns)
	settings.modes = map[string]string{
		"camp-1": PinModePreserve, "camp-2": PinModePreserve, "camp-3": PinModePreserve,
	}
	n, err := svc.AutoPinOnInstall(context.Background(), "v0.1.10", "v0.1.11", "admin-1", "1.2.3.4", "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 affected, got %d", n)
	}
	if len(settings.calls) != 3 {
		t.Fatalf("expected 3 SetFoundryModulePin calls, got %d", len(settings.calls))
	}
	for _, call := range settings.calls {
		if call.version != "v0.1.10" {
			t.Errorf("expected pin to v0.1.10 (the previous version), got %s for %s",
				call.version, call.campaignID)
		}
	}

	// Audit log: 3 per-campaign events + 1 summary event.
	var perCampaign, summary int
	for _, e := range events.events {
		switch e.eventType {
		case EventModuleAutoPinOnInstall:
			perCampaign++
		case EventModuleAutoPinInstallSummary:
			summary++
		}
	}
	if perCampaign != 3 {
		t.Errorf("expected 3 per-campaign events, got %d", perCampaign)
	}
	if summary != 1 {
		t.Errorf("expected 1 summary event, got %d", summary)
	}
}

// TestAutoPinOnInstall_PromoteDefaultLeavesEmpty — the NEW default.
// Campaigns with no pin_mode set (empty = promote per audit D1) are
// left alone so they keep auto-tracking the newest installed version.
// No pins written, no per-campaign events; the summary still fires.
func TestAutoPinOnInstall_PromoteDefaultLeavesEmpty(t *testing.T) {
	svc, settings, events := newAutoPinTestService(t, []CampaignUsage{
		{CampaignID: "camp-1", CampaignName: "Imix"},
		{CampaignID: "camp-2", CampaignName: "Test"},
	})
	// No modes set → "" → promote default.
	n, err := svc.AutoPinOnInstall(context.Background(), "v0.1.10", "v0.1.11", "admin-1", "1.2.3.4", "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("promote default should pin nobody, got %d affected", n)
	}
	if len(settings.calls) != 0 {
		t.Errorf("promote default should write no pins, got %d", len(settings.calls))
	}
	var perCampaign, summary int
	for _, e := range events.events {
		switch e.eventType {
		case EventModuleAutoPinOnInstall:
			perCampaign++
		case EventModuleAutoPinInstallSummary:
			summary++
		}
	}
	if perCampaign != 0 {
		t.Errorf("expected 0 per-campaign events under promote default, got %d", perCampaign)
	}
	if summary != 1 {
		t.Errorf("expected 1 summary event, got %d", summary)
	}
}

// TestAutoPinOnInstall_MixedModes — only preserve-mode campaigns freeze;
// promote-mode (and unset) campaigns are left auto-tracking.
func TestAutoPinOnInstall_MixedModes(t *testing.T) {
	svc, settings, _ := newAutoPinTestService(t, []CampaignUsage{
		{CampaignID: "camp-preserve"},
		{CampaignID: "camp-promote"},
		{CampaignID: "camp-unset"},
	})
	settings.modes = map[string]string{
		"camp-preserve": PinModePreserve,
		"camp-promote":  PinModePromote,
		// camp-unset intentionally absent → "" → promote default.
	}
	n, err := svc.AutoPinOnInstall(context.Background(), "v0.1.10", "v0.1.11", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected only the preserve-mode campaign pinned, got %d affected", n)
	}
	if len(settings.calls) != 1 || settings.calls[0].campaignID != "camp-preserve" {
		t.Fatalf("expected exactly one pin for camp-preserve, got %+v", settings.calls)
	}
}

// TestAutoPinOnInstall_PartialFailure — one campaign's pin write
// fails; the fan-out continues. The summary count reflects only
// successes.
func TestAutoPinOnInstall_PartialFailure(t *testing.T) {
	svc, settings, events := newAutoPinTestService(t, []CampaignUsage{
		{CampaignID: "camp-1"}, {CampaignID: "camp-2"},
	})
	// preserve mode so both reach the (failing) pin write.
	settings.modes = map[string]string{"camp-1": PinModePreserve, "camp-2": PinModePreserve}
	settings.err = errors.New("simulated partial failure")
	n, err := svc.AutoPinOnInstall(context.Background(), "v0.1.10", "v0.1.11", "", "", "")
	if err != nil {
		t.Fatalf("AutoPinOnInstall should not propagate per-campaign errors, got: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 affected when all pins fail, got %d", n)
	}
	// Summary event still fires with the (failing) count.
	hasSummary := false
	for _, e := range events.events {
		if e.eventType == EventModuleAutoPinInstallSummary {
			hasSummary = true
		}
	}
	if !hasSummary {
		t.Error("summary event should fire regardless of per-campaign failures")
	}
}

// --- migration tests ---

// fakeKVStore is the simplest possible SettingsKVStore. Tests
// inspect/manipulate the in-memory map directly.
type fakeKVStore struct {
	store map[string]string
}

func newFakeKVStore() *fakeKVStore { return &fakeKVStore{store: map[string]string{}} }

func (s *fakeKVStore) Get(_ context.Context, key string) (string, error) {
	if v, ok := s.store[key]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}
func (s *fakeKVStore) Set(_ context.Context, key, value string) error {
	s.store[key] = value
	return nil
}

// fakeServiceForMigration wraps a service to also satisfy the
// Service interface's FindFoundryPackage. autoPinMigrateInline
// casts to *service and goes through s.repo + s.settings directly;
// the public AutoPinMigrate also calls svc.FindFoundryPackage so
// we need that to return something sensible.
type fakeServiceForMigration struct {
	*service
	foundryPkg *packages.Package
}

func (s *fakeServiceForMigration) FindFoundryPackage(_ context.Context) (*packages.Package, error) {
	return s.foundryPkg, nil
}

// Service interface satisfaction — embed *service handles most;
// override only the methods relevant to the migration. The other
// Service methods aren't called by AutoPinMigrate so the embedded
// *service's implementations are fine.

// TestAutoPinMigrate_Idempotent — running the migration twice in a
// row: first run pins campaigns + sets flag; second run reads flag
// + skips.
func TestAutoPinMigrate_Idempotent(t *testing.T) {
	svc, settings, _ := newAutoPinTestService(t, []CampaignUsage{
		{CampaignID: "camp-1"},
	})
	wrapped := &fakeServiceForMigration{
		service: svc,
		foundryPkg: &packages.Package{
			Type: packages.PackageTypeFoundryModule, InstalledVersion: "v0.1.10",
		},
	}
	kv := newFakeKVStore()

	// First run: pin written, flag set.
	if err := AutoPinMigrate(context.Background(), wrapped, kv); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if len(settings.calls) != 1 {
		t.Fatalf("first run should pin 1 campaign, got %d", len(settings.calls))
	}
	if kv.store[AutoPinMigrationSettingKey] != "v0.1.10" {
		t.Errorf("expected migration flag set to v0.1.10, got %q", kv.store[AutoPinMigrationSettingKey])
	}

	// Second run: should be a no-op.
	settings.mu.Lock()
	settings.calls = nil // reset to detect any new pin
	settings.mu.Unlock()
	if err := AutoPinMigrate(context.Background(), wrapped, kv); err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if len(settings.calls) != 0 {
		t.Errorf("second run should be a no-op, got %d pin calls", len(settings.calls))
	}
}

// TestAutoPinMigrate_NoFoundryPackage — graceful no-op when the
// operator hasn't set up the foundry-module package yet. Returns
// nil without setting the flag, so the next boot retries once
// the operator adds the package.
func TestAutoPinMigrate_NoFoundryPackage(t *testing.T) {
	svc, _, _ := newAutoPinTestService(t, nil)
	wrapped := &fakeServiceForMigration{
		service:    svc,
		foundryPkg: nil, // no package
	}
	kv := newFakeKVStore()
	if err := AutoPinMigrate(context.Background(), wrapped, kv); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := kv.store[AutoPinMigrationSettingKey]; ok {
		t.Error("flag should NOT be set when no foundry package — migration retries on next boot")
	}
}

// TestAutoPinMigrate_PinsAllEmptyCampaigns — happy path. Two
// campaigns with empty pin; both get pinned to the installed
// version; per-campaign migration events fire.
func TestAutoPinMigrate_PinsAllEmptyCampaigns(t *testing.T) {
	svc, settings, events := newAutoPinTestService(t, []CampaignUsage{
		{CampaignID: "camp-1", CampaignName: "Imix"},
		{CampaignID: "camp-2", CampaignName: "Test"},
	})
	wrapped := &fakeServiceForMigration{
		service: svc,
		foundryPkg: &packages.Package{
			Type: packages.PackageTypeFoundryModule, InstalledVersion: "v0.1.10",
		},
	}
	if err := AutoPinMigrate(context.Background(), wrapped, newFakeKVStore()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(settings.calls) != 2 {
		t.Fatalf("expected 2 pin calls, got %d", len(settings.calls))
	}
	migrationEvents := 0
	for _, e := range events.events {
		if e.eventType == EventModuleAutoPinMigration {
			migrationEvents++
		}
	}
	if migrationEvents != 2 {
		t.Errorf("expected 2 migration events, got %d", migrationEvents)
	}
}
