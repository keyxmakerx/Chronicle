// Tests for the C-FMC-8 admin auto-pin banner flow.
//
// Contracts pinned:
//
//  1. AutoPinOnInstall (when KV is wired) persists an AutoPinSummary
//     to the LatestAutoPinSummaryKey settings row.
//  2. GetUnreadAutoPinSummary returns the stored summary when no
//     dismissal exists; returns nil when a dismissal stamp is at or
//     after the summary timestamp.
//  3. DismissAutoPinBanner writes a Unix-second timestamp; the
//     subsequent GetUnreadAutoPinSummary returns nil.
//  4. GetUnreadAutoPinSummary returns nil when no summary has ever
//     been stored (clean install).
//  5. GetUnreadAutoPinSummary returns nil when the KV isn't wired
//     (defensive — banner is supplementary).
package foundry_vtt

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// newBannerTestService wires a service struct with just the bits
// the banner path needs: kv + events + settings + repo. Tests
// inject the in-memory fakeKVStore so the assertions can read
// state directly from the map.
func newBannerTestService(t *testing.T) (*service, *fakeKVStore, *recordingSettings, *fakeEventLogger) {
	t.Helper()
	kv := newFakeKVStore()
	settings := &recordingSettings{}
	events := &fakeEventLogger{}
	svc := &service{
		repo:     &fakeRepoForAutoPin{},
		settings: settings,
		events:   events,
		kv:       kv,
	}
	return svc, kv, settings, events
}

// TestAutoPinOnInstall_PersistsSummaryToKV — when the install hook
// fires with a real version transition, the summary lands in the
// settings KV. The banner handler reads from there.
func TestAutoPinOnInstall_PersistsSummaryToKV(t *testing.T) {
	svc, kv, _, _ := newBannerTestService(t)
	svc.repo = &fakeRepoForAutoPin{emptyPinCampaigns: []CampaignUsage{
		{CampaignID: "camp-1", CampaignName: "C1"},
	}}

	if _, err := svc.AutoPinOnInstall(context.Background(),
		"v0.1.10", "v0.1.11", "", "", ""); err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	raw, ok := kv.store[LatestAutoPinSummaryKey]
	if !ok {
		t.Fatal("AutoPinOnInstall should persist a summary to the KV store")
	}
	var summary AutoPinSummary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		t.Fatalf("summary should be valid JSON: %v", err)
	}
	if summary.PreviousVersion != "v0.1.10" {
		t.Errorf("PreviousVersion: want v0.1.10, got %q", summary.PreviousVersion)
	}
	if summary.NewVersion != "v0.1.11" {
		t.Errorf("NewVersion: want v0.1.11, got %q", summary.NewVersion)
	}
	if summary.Affected != 1 {
		t.Errorf("Affected: want 1, got %d", summary.Affected)
	}
	if summary.Timestamp == 0 {
		t.Error("Timestamp should be set to a non-zero Unix time")
	}
}

// TestGetUnreadAutoPinSummary_FreshReturnsSummary — happy path:
// summary exists, no dismissal stamp, handler returns the summary.
func TestGetUnreadAutoPinSummary_FreshReturnsSummary(t *testing.T) {
	svc, kv, _, _ := newBannerTestService(t)
	summary := AutoPinSummary{
		PreviousVersion: "v0.1.10",
		NewVersion:      "v0.1.11",
		Affected:        2,
		Timestamp:       time.Now().Unix(),
	}
	bytes, _ := json.Marshal(summary)
	kv.store[LatestAutoPinSummaryKey] = string(bytes)

	got, err := svc.GetUnreadAutoPinSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil summary")
	}
	if got.Affected != 2 {
		t.Errorf("Affected: want 2, got %d", got.Affected)
	}
}

// TestGetUnreadAutoPinSummary_DismissedReturnsNil — after dismissal,
// the banner is silenced until a new install produces a fresh
// summary.
func TestGetUnreadAutoPinSummary_DismissedReturnsNil(t *testing.T) {
	svc, kv, _, _ := newBannerTestService(t)
	oldTime := time.Now().Add(-1 * time.Hour).Unix()
	summary := AutoPinSummary{
		PreviousVersion: "v0.1.10",
		NewVersion:      "v0.1.11",
		Affected:        1,
		Timestamp:       oldTime,
	}
	bytes, _ := json.Marshal(summary)
	kv.store[LatestAutoPinSummaryKey] = string(bytes)
	// Dismissal stamp AFTER the summary timestamp.
	kv.store[AutoPinBannerDismissedAtKey] = strconv.FormatInt(time.Now().Unix(), 10)

	got, err := svc.GetUnreadAutoPinSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil summary after dismissal, got: %+v", got)
	}
}

// TestGetUnreadAutoPinSummary_NewInstallAfterDismissal — admin
// dismisses a banner; a new install happens; the new summary's
// timestamp is later than the dismissal so the banner reappears.
func TestGetUnreadAutoPinSummary_NewInstallAfterDismissal(t *testing.T) {
	svc, kv, _, _ := newBannerTestService(t)
	// Old dismissal stamp.
	kv.store[AutoPinBannerDismissedAtKey] = strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10)
	// Fresh summary stamped NOW (after the dismissal).
	summary := AutoPinSummary{
		PreviousVersion: "v0.1.11",
		NewVersion:      "v0.1.12",
		Affected:        1,
		Timestamp:       time.Now().Unix(),
	}
	bytes, _ := json.Marshal(summary)
	kv.store[LatestAutoPinSummaryKey] = string(bytes)

	got, err := svc.GetUnreadAutoPinSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got == nil {
		t.Fatal("new install after dismissal should resurrect the banner")
	}
}

// TestGetUnreadAutoPinSummary_NoSummaryReturnsNil — clean install,
// no summary ever stored. Returns nil cleanly (no error).
func TestGetUnreadAutoPinSummary_NoSummaryReturnsNil(t *testing.T) {
	svc, _, _, _ := newBannerTestService(t)
	got, err := svc.GetUnreadAutoPinSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil summary on clean state, got: %+v", got)
	}
}

// TestGetUnreadAutoPinSummary_NoKVReturnsNil — defensive: tests
// that don't wire the KV should get a nil summary without panicking.
// The banner is supplementary; an unwired KV shouldn't break page
// renders.
func TestGetUnreadAutoPinSummary_NoKVReturnsNil(t *testing.T) {
	svc := &service{} // kv == nil
	got, err := svc.GetUnreadAutoPinSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil summary when KV not wired, got: %+v", got)
	}
}

// TestDismissAutoPinBanner_StampsTimestamp — dismiss writes a
// Unix-second timestamp string into the dismissal key. Subsequent
// GetUnreadAutoPinSummary returns nil for that summary.
func TestDismissAutoPinBanner_StampsTimestamp(t *testing.T) {
	svc, kv, _, _ := newBannerTestService(t)
	// Pre-populate a summary so we can verify dismissal silences it.
	oldTime := time.Now().Add(-1 * time.Minute).Unix()
	summary := AutoPinSummary{Timestamp: oldTime, Affected: 1, NewVersion: "v0.1.11", PreviousVersion: "v0.1.10"}
	bytes, _ := json.Marshal(summary)
	kv.store[LatestAutoPinSummaryKey] = string(bytes)

	before, _ := svc.GetUnreadAutoPinSummary(context.Background())
	if before == nil {
		t.Fatal("summary should be unread before dismiss")
	}

	if err := svc.DismissAutoPinBanner(context.Background()); err != nil {
		t.Fatalf("dismiss error: %v", err)
	}
	stamp, ok := kv.store[AutoPinBannerDismissedAtKey]
	if !ok {
		t.Fatal("dismiss should write the dismissal key")
	}
	if _, err := strconv.ParseInt(stamp, 10, 64); err != nil {
		t.Errorf("dismissal stamp should be a Unix-second integer, got %q", stamp)
	}

	after, _ := svc.GetUnreadAutoPinSummary(context.Background())
	if after != nil {
		t.Errorf("summary should be silenced after dismiss, got: %+v", after)
	}
}
