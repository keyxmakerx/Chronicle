package foundry_vtt

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestAutoTrackingCampaigns_ReturnsEmptyPinCampaigns pins that the service
// accessor forwards the repo's empty-pin set — the campaigns that follow
// the newest installed module version rather than pinning a specific one.
func TestAutoTrackingCampaigns_ReturnsEmptyPinCampaigns(t *testing.T) {
	want := []CampaignUsage{
		{CampaignID: "c1", CampaignName: "Alpha"},
		{CampaignID: "c2", CampaignName: "Beta"},
	}
	svc, _, _ := newAutoPinTestService(t, want)

	got, err := svc.AutoTrackingCampaigns(context.Background())
	if err != nil {
		t.Fatalf("AutoTrackingCampaigns: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("want %d auto-tracking campaigns, got %d", len(want), len(got))
	}
}

// TestAdminVersionCampaignsBlock_ShowsAutoTrackingRow pins the item-4 fix:
// even with an empty exact-pin list, the card reports the auto-tracking
// count so it never reads as "no campaign uses this module".
func TestAdminVersionCampaignsBlock_ShowsAutoTrackingRow(t *testing.T) {
	auto := []CampaignUsage{{CampaignID: "c1"}, {CampaignID: "c2"}, {CampaignID: "c3"}}

	var buf bytes.Buffer
	if err := AdminVersionCampaignsBlock("0.2.0", nil, auto, "csrf").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "3 campaign(s) auto-tracking latest") {
		t.Errorf("expected the auto-tracking count row, got:\n%s", out)
	}
}

// TestAdminVersionCampaignsBlock_EmptyStateMentionsAutoTracking pins the
// empty-state copy: no auto-tracking row when the count is zero, and the
// pinned-list empty state explains that auto-tracking is the norm so an
// empty list doesn't look like a bug.
func TestAdminVersionCampaignsBlock_EmptyStateMentionsAutoTracking(t *testing.T) {
	var buf bytes.Buffer
	if err := AdminVersionCampaignsBlock("0.2.0", nil, nil, "csrf").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "auto-tracking latest") {
		t.Errorf("no auto-tracking row expected when the count is zero, got:\n%s", out)
	}
	if !strings.Contains(out, "auto-track the latest installed version") {
		t.Errorf("empty state should explain auto-tracking is normal, got:\n%s", out)
	}
}

// TestAdminVersionCampaignsBlock_ShowsPinnedBanner is a regression guard
// that the exact-pin banner still renders after the signature change.
func TestAdminVersionCampaignsBlock_ShowsPinnedBanner(t *testing.T) {
	usage := []CampaignUsage{{CampaignID: "c1", CampaignName: "Alpha"}}

	var buf bytes.Buffer
	if err := AdminVersionCampaignsBlock("0.2.0", usage, nil, "csrf").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "1 campaign(s) pinned to 0.2.0") {
		t.Errorf("expected the exact-pin banner, got:\n%s", out)
	}
}
