package sessions

// Reporting a member's zone must fall back to member_availability.tz when
// users.timezone is unset: the availability page's zone control only writes
// member_availability.tz (static/js/availability.js PUTs {tz, blocks} to
// /availability/mine, never PUT /account/timezone), so a roster view reading
// users.timezone alone would report a set zone as absent.

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// zoneUserDir answers users.timezone per user id; a user not in the map has none.
type zoneUserDir struct {
	byUser map[string]string
	// batchCalls and batchIDs record how the roster path reached the directory.
	batchCalls int
	batchIDs   []string
	getCalls   int
}

func (d *zoneUserDir) GetTimezonesByIDs(_ context.Context, ids []string) (map[string]string, error) {
	d.batchCalls++
	d.batchIDs = append(d.batchIDs, ids...)
	out := map[string]string{}
	for _, id := range ids {
		if tz, ok := d.byUser[id]; ok {
			out[id] = tz
		}
	}
	return out, nil
}

func (d *zoneUserDir) GetUser(_ context.Context, userID string) (*auth.User, error) {
	d.getCalls++
	u := &auth.User{ID: userID, DisplayName: "Player"}
	if tz, ok := d.byUser[userID]; ok {
		u.Timezone = &tz
	}
	return u, nil
}

// zoneHandler wires a roster plus stored availability blocks (which carry the
// zone the member set on the availability page) and account zones.
func zoneHandler(roster []string, blocks []AvailabilityBlock, accountZones map[string]string) *Handler {
	members := make([]campaigns.CampaignMember, 0, len(roster))
	for _, id := range roster {
		members = append(members, campaigns.CampaignMember{UserID: id, DisplayName: id, Role: campaigns.RolePlayer})
	}
	repo := &mockSessionRepo{
		listCampaignAvailabilityFn: func(_ context.Context, _ string) ([]AvailabilityBlock, error) {
			return blocks, nil
		},
	}
	return &Handler{
		svc:          NewSessionService(repo, nil, nil),
		memberLister: &stubMemberLister{members: members},
		userDir:      &zoneUserDir{byUser: accountZones},
	}
}

// TestCampaignRoster_ReportsTheZoneSetOnTheAvailabilityPage is the headline: a
// member whose ONLY zone is member_availability.tz must be reported as having a
// zone.
func TestCampaignRoster_ReportsTheZoneSetOnTheAvailabilityPage(t *testing.T) {
	h := zoneHandler(
		[]string{"painter", "accounter", "neither"},
		[]AvailabilityBlock{
			// "painter" set the control on the availability page and saved.
			{UserID: "painter", DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60,
				EndMinute: 22 * 60, State: AvailAvailable, TZ: "Europe/London"},
		},
		// "accounter" set theirs in account settings instead.
		map[string]string{"accounter": "America/Denver"},
	)

	got := map[string]string{}
	for _, r := range h.CampaignRoster(context.Background(), "camp-1") {
		got[r.UserID] = r.TZ
	}

	if got["painter"] != "Europe/London" {
		t.Errorf("the member who set their zone on the availability page reads as %q — the "+
			"Bench prints 'zone not set' and a repair chip for them, permanently", got["painter"])
	}
	if got["accounter"] != "America/Denver" {
		t.Errorf("the account-settings zone regressed: %q", got["accounter"])
	}
	// EMPTY STAYS A FIRST-CLASS STATE. A member with no zone anywhere must not
	// acquire a UTC guess: the consumers print a repair rather than a clock.
	if got["neither"] != "" {
		t.Errorf("a member with no zone anywhere reads as %q — absence was laundered into a "+
			"fact, which is the thing the repair chip exists to avoid", got["neither"])
	}
}

// TestCampaignRoster_AvailabilityZoneWinsOverTheAccountZone pins the precedence.
// The availability control is the one the member chose in the context of their
// availability, and it is the one they can see on that page.
func TestCampaignRoster_AvailabilityZoneWinsOverTheAccountZone(t *testing.T) {
	h := zoneHandler(
		[]string{"u1"},
		[]AvailabilityBlock{{UserID: "u1", DayOfWeek: 2, StartMinute: 0, EndMinute: 60,
			State: AvailAvailable, TZ: "Europe/London"}},
		map[string]string{"u1": "America/Denver"},
	)
	roster := h.CampaignRoster(context.Background(), "camp-1")
	if len(roster) != 1 || roster[0].TZ != "Europe/London" {
		t.Fatalf("roster = %+v, want the availability-page zone Europe/London", roster)
	}
}

// TestCampaignRoster_IgnoresAnUnusableStoredZone keeps the new read honest: a
// junk zone in the column is not a zone, and must fall through rather than be
// handed to a clock renderer.
func TestCampaignRoster_IgnoresAnUnusableStoredZone(t *testing.T) {
	h := zoneHandler(
		[]string{"u1"},
		[]AvailabilityBlock{{UserID: "u1", DayOfWeek: 2, StartMinute: 0, EndMinute: 60,
			State: AvailAvailable, TZ: "Mars/Olympus_Mons"}},
		map[string]string{"u1": "America/Denver"},
	)
	roster := h.CampaignRoster(context.Background(), "camp-1")
	if len(roster) != 1 || roster[0].TZ != "America/Denver" {
		t.Fatalf("roster = %+v, want the account zone once the stored one is unusable", roster)
	}
}

// TestCampaignMemberZones_OneReadPerRoster pins the shape WG-4 requires: the
// availability zones are read ONCE for the whole roster, not once per member.
func TestCampaignMemberZones_OneReadPerRoster(t *testing.T) {
	reads := 0
	repo := &mockSessionRepo{
		listCampaignAvailabilityFn: func(_ context.Context, _ string) ([]AvailabilityBlock, error) {
			reads++
			return []AvailabilityBlock{
				{UserID: "u1", TZ: "Europe/London", DayOfWeek: 1, StartMinute: 0, EndMinute: 60, State: AvailAvailable},
				{UserID: "u2", TZ: "Europe/London", DayOfWeek: 1, StartMinute: 0, EndMinute: 60, State: AvailAvailable},
				{UserID: "u3", TZ: "Europe/London", DayOfWeek: 1, StartMinute: 0, EndMinute: 60, State: AvailAvailable},
			}, nil
		},
	}
	members := []campaigns.CampaignMember{
		{UserID: "u1", DisplayName: "A", Role: campaigns.RolePlayer},
		{UserID: "u2", DisplayName: "B", Role: campaigns.RolePlayer},
		{UserID: "u3", DisplayName: "C", Role: campaigns.RolePlayer},
	}
	h := &Handler{
		svc:          NewSessionService(repo, nil, nil),
		memberLister: &stubMemberLister{members: members},
		userDir:      &zoneUserDir{},
	}
	h.CampaignRoster(context.Background(), "camp-1")
	if reads != 1 {
		t.Errorf("the campaign's availability zones were read %d times for a 3-member roster; "+
			"asking per member turns a roster render into an N+1 (WG-4)", reads)
	}
}

// TestOverlayMembers_BatchesAccountZones pins the N+1 fix: however many members
// lack an availability-page zone, the account zones are read in ONE batch call
// that names only those members, never per member, and precedence holds
// (availability zone, then a valid account zone, then unset).
func TestOverlayMembers_BatchesAccountZones(t *testing.T) {
	tests := []struct {
		name        string
		roster      []string
		blocks      []AvailabilityBlock
		account     map[string]string
		wantTZ      map[string]string
		wantBatch   int
		wantBatched []string
	}{
		{
			name:        "availability zone wins and is not batched",
			roster:      []string{"a", "b"},
			blocks:      []AvailabilityBlock{{UserID: "a", DayOfWeek: 1, StartMinute: 0, EndMinute: 60, State: AvailAvailable, TZ: "Europe/London"}},
			account:     map[string]string{"a": "America/Denver", "b": "America/Denver"},
			wantTZ:      map[string]string{"a": "Europe/London", "b": "America/Denver"},
			wantBatch:   1,
			wantBatched: []string{"b"},
		},
		{
			name:        "many members without a zone, still one call",
			roster:      []string{"a", "b", "c", "d"},
			account:     map[string]string{"a": "UTC", "c": "Asia/Tokyo"},
			wantTZ:      map[string]string{"a": "UTC", "b": "", "c": "Asia/Tokyo", "d": ""},
			wantBatch:   1,
			wantBatched: []string{"a", "b", "c", "d"},
		},
		{
			name:        "invalid account zone is unset",
			roster:      []string{"a"},
			account:     map[string]string{"a": "Mars/Olympus_Mons"},
			wantTZ:      map[string]string{"a": ""},
			wantBatch:   1,
			wantBatched: []string{"a"},
		},
		{
			name:   "everyone has an availability zone, no batch call",
			roster: []string{"a"},
			blocks: []AvailabilityBlock{{UserID: "a", DayOfWeek: 1, StartMinute: 0, EndMinute: 60, State: AvailAvailable, TZ: "Europe/London"}},
			wantTZ: map[string]string{"a": "Europe/London"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := zoneHandler(tc.roster, tc.blocks, tc.account)
			dir := h.userDir.(*zoneUserDir)
			got := h.overlayMembers(context.Background(), "camp-1")
			for _, m := range got {
				if m.TZ != tc.wantTZ[m.UserID] {
					t.Errorf("%s TZ = %q, want %q", m.UserID, m.TZ, tc.wantTZ[m.UserID])
				}
			}
			if dir.batchCalls != tc.wantBatch {
				t.Errorf("batch calls = %d, want %d", dir.batchCalls, tc.wantBatch)
			}
			if dir.getCalls != 0 {
				t.Errorf("per-member GetUser calls = %d, want 0", dir.getCalls)
			}
			if len(dir.batchIDs) != len(tc.wantBatched) {
				t.Errorf("batched ids = %v, want %v", dir.batchIDs, tc.wantBatched)
			}
		})
	}
}
