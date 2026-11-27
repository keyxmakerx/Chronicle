package audit

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 27, 19, 0, 0, 0, time.UTC)

func at(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

func TestBuildFeed_CutsSittingsByAuthorAndQuiet(t *testing.T) {
	entries := []AuditEntry{ // newest first, as the repository returns them
		{UserID: "echo", UserName: "Echo", Action: "campaign.welcome_message.updated", CreatedAt: at(50)},
		{UserID: "ginko", UserName: "Ginko", Action: ActionEntityUpdated, EntityID: "p1", CreatedAt: at(45)},
		{UserID: "echo", UserName: "Echo", Action: ActionEntityUpdated, EntityID: "p1", CreatedAt: at(30)},
		{UserID: "echo", UserName: "Echo", Action: ActionMemberRoleChanged, CreatedAt: at(10)},
		// 40 quiet minutes before this one: a sitting of its own.
		{UserID: "echo", UserName: "Echo", Action: ActionEntityCreated, EntityID: "p2", CreatedAt: at(-31)},
	}
	foundry := []FoundryEvent{
		{At: at(40), Kind: "page", ResourceID: "p1", Name: "The Drowned Bell", Action: "Foundry kept", Status: "conflict", Direction: "to_chronicle"},
		{At: at(20), Kind: "character", ResourceID: "c1", Name: "Mira", Action: "fields updated", Direction: "to_chronicle"},
	}
	f := buildFeed(entries, foundry)
	if f.Total != 7 {
		t.Fatalf("Total = %d, want 7", f.Total)
	}
	var got []string
	for _, s := range f.Sittings {
		got = append(got, s.AuthorName+":"+strconv.Itoa(len(s.Rows)))
	}
	want := "Echo:3 Ginko:1 Foundry Sync:2 Echo:1"
	if strings.Join(got, " ") != want {
		t.Fatalf("sittings = %q, want %q", strings.Join(got, " "), want)
	}
	if f.Counts[KindConflict] != 1 || f.Counts[KindFoundry] != 1 || f.Counts[KindPage] != 3 || f.Counts[KindSetting] != 1 || f.Counts[KindPlayers] != 1 {
		t.Errorf("counts = %v", f.Counts)
	}
	s := f.Sittings[0]
	if !s.Start.Equal(at(10)) || !s.End.Equal(at(50)) {
		t.Errorf("first sitting spans %v to %v", s.Start, s.End)
	}
}

func TestSittingSummary(t *testing.T) {
	person := Sitting{Author: "echo", Rows: []ActivityRow{
		{Kind: KindSetting}, {Kind: KindSetting},
		{Kind: KindPage, Entry: &AuditEntry{EntityID: "p1"}}, {Kind: KindPage, Entry: &AuditEntry{EntityID: "p1"}},
		{Kind: KindPage, Entry: &AuditEntry{EntityID: "p2"}},
		{Kind: KindPlayers},
	}}
	if got, want := sittingSummary(person), "Changed 2 settings, 2 pages and 1 change to players"; got != want {
		t.Errorf("person summary = %q, want %q", got, want)
	}
	fd := Sitting{Author: FoundryAuthor, Rows: []ActivityRow{{Kind: KindFoundry}, {Kind: KindFoundry}, {Kind: KindConflict}}}
	if got, want := sittingSummary(fd), "Brought 2 changes in from Foundry, 1 conflict settled"; got != want {
		t.Errorf("foundry summary = %q, want %q", got, want)
	}
}

type fakeFoundry struct {
	evs []FoundryEvent
	err error
}

func (f fakeFoundry) FoundryActivity(context.Context, string, int) ([]FoundryEvent, error) {
	return f.evs, f.err
}

func TestRecentActivity_FoundryFailureLeavesFoundryOut(t *testing.T) {
	repo := &mockAuditRepo{}
	svc := NewAuditService(repo)
	svc.SetFoundryActivity(fakeFoundry{err: context.DeadlineExceeded})
	if _, err := svc.RecentActivity(context.Background(), "c1"); err != nil {
		t.Fatalf("a failed Foundry read failed the page: %v", err)
	}
}

// The conflict row opens to say which side was kept, the module's reason,
// and where to compare; a Chronicle-kept conflict offers no restore.
func TestRecentActivity_RendersConflict(t *testing.T) {
	for _, tc := range []struct {
		action  string
		kept    string
		restore bool
	}{{"Foundry kept", "Foundry kept", true}, {"Chronicle kept", "Chronicle kept", false}} {
		f := buildFeed(nil, []FoundryEvent{{At: at(0), Kind: "page", ResourceID: "p1", Name: "The Drowned Bell", Action: tc.action, Status: "conflict",
			Message: "The newest edit wins. Fields the Foundry edit sent: name, entry."}})
		v := ActivityView{CampaignID: "c1", Feed: f, Now: at(5), Loc: time.UTC, Shown: sittingsStep}
		var buf bytes.Buffer
		if err := RecentActivity(v).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		html := buf.String()
		for _, want := range []string{"Foundry Sync", tc.kept, "The Drowned Bell", "changed on both sides", "Fields the Foundry edit sent: name, entry.",
			"/campaigns/c1/entities/p1?history=1", "Compare in the page", "Brought 0 changes", "1 conflict"} {
			if want == "Brought 0 changes" {
				if strings.Contains(html, want) {
					t.Errorf("%s: summary counts nothing as changes: %s", tc.action, want)
				}
				continue
			}
			if !strings.Contains(html, want) {
				t.Errorf("%s: rendered activity lacks %q", tc.action, want)
			}
		}
		if got := strings.Contains(html, "words back"); got != tc.restore {
			t.Errorf("%s: restore offered = %v, want %v", tc.action, got, tc.restore)
		}
	}
}

func TestSittingWhen(t *testing.T) {
	now := time.Date(2026, 11, 27, 22, 0, 0, 0, time.UTC)
	v := ActivityView{Now: now, Loc: time.UTC}
	at := func(h, m int) time.Time { return time.Date(2026, 11, 27, h, m, 0, 0, time.UTC) }
	tests := []struct {
		name       string
		start, end time.Time
		want       string
	}{
		{"one time", at(19, 2), at(19, 2), "Today, 7:02pm"},
		{"same half of the day", at(19, 2), at(19, 48), "Today, 7:02 to 7:48pm"},
		{"across noon", at(11, 40), at(12, 20), "Today, 11:40am to 12:20pm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sittingWhen(v, Sitting{Start: tt.start, End: tt.end}); got != tt.want {
				t.Errorf("sittingWhen = %q, want %q", got, tt.want)
			}
		})
	}
}
