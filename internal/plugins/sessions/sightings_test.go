package sessions

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

type fakeSightingRepo struct {
	rows     []SightingRow
	links    []SightingLink
	upserted []SightingRow
	deleted  []string
}

func (f *fakeSightingRepo) UpsertSighting(_ context.Context, s SightingRow) error {
	f.upserted = append(f.upserted, s)
	return nil
}
func (f *fakeSightingRepo) DeleteSighting(_ context.Context, sessionID, date, entityID string) error {
	f.deleted = append(f.deleted, sessionID+"|"+date+"|"+entityID)
	return nil
}
func (f *fakeSightingRepo) ListSightingsForEntity(context.Context, string, string) ([]SightingRow, error) {
	return f.rows, nil
}
func (f *fakeSightingRepo) ListSightingsForNight(_ context.Context, sessionID, date string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range f.rows {
		if r.SessionID == sessionID && r.OccurrenceDate == date {
			out[r.EntityID] = r.State
		}
	}
	return out, nil
}
func (f *fakeSightingRepo) ListLinksForEntity(context.Context, string, string) ([]SightingLink, error) {
	return f.links, nil
}

var (
	dmViewer     = permissions.RequestViewer(permissions.RoleOwner, "dm1")
	playerViewer = permissions.RequestViewer(permissions.RolePlayer, "p1")
)

func sightingDates(list []Sighting) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.SessionID + "|" + s.Date + "|" + s.Source
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSightingsFor_Merge(t *testing.T) {
	repo := &fakeSightingRepo{
		rows: []SightingRow{
			{SessionID: "weekly", SessionName: "Weekly", OccurrenceDate: "2026-10-10", State: SightingSeen, Source: SightingSourceCheckin, AddedBy: "dm1"},
			{SessionID: "weekly", SessionName: "Weekly", OccurrenceDate: "2026-10-03", State: SightingNotSeen, Source: SightingSourceCheckin, AddedBy: "dm1"},
			{SessionID: "oneoff", SessionName: "One-off", OccurrenceDate: "2026-09-01", State: SightingNotSeen, Source: SightingSourceManual},
			{SessionID: "dismissed", SessionName: "Dismissed", OccurrenceDate: "2026-03-08", State: SightingNotSeen, Source: SightingSourceCheckin},
		},
		links: []SightingLink{
			// Answered series: the series-wide link is dropped.
			{SessionID: "weekly", SessionName: "Weekly", Role: EntityRoleEncountered, ScheduledDate: "2026-01-03", IsRecurring: true},
			// A stored answer for the same one-off night wins over its link.
			{SessionID: "oneoff", SessionName: "One-off", Role: EntityRoleKey, ScheduledDate: "2026-09-01"},
			{SessionID: "met", SessionName: "Met", Role: EntityRoleKey, ScheduledDate: "2026-08-15"},
			{SessionID: "series", SessionName: "Series", Role: EntityRoleEncountered, ScheduledDate: "2026-02-01", IsRecurring: true},
			// Only a "not seen" night on this series: its link stays.
			{SessionID: "dismissed", SessionName: "Dismissed", Role: EntityRoleKey, ScheduledDate: "2026-03-01", IsRecurring: true},
			{SessionID: "named", SessionName: "Named", Role: EntityRoleMentioned, ScheduledDate: "2026-09-20"},
		},
	}
	svc := NewSightingService(repo, &mockSessionRepo{}, &mockEntityChecker{}, &stubEntityVisibility{})

	tests := []struct {
		name        string
		viewer      permissions.Viewer
		seen        []string
		suggestions []string
		notSeen     []string
		addedBy     string
	}{
		{
			name:        "dm sees everything, newest first",
			viewer:      dmViewer,
			seen:        []string{"weekly|2026-10-10|checkin", "met|2026-08-15|link", "dismissed||link", "series||link"},
			suggestions: []string{"named|2026-09-20|link"},
			notSeen:     []string{"weekly|2026-10-03|checkin", "oneoff|2026-09-01|manual", "dismissed|2026-03-08|checkin"},
			addedBy:     "dm1",
		},
		{
			name:        "player sees only nights seen",
			viewer:      playerViewer,
			seen:        []string{"weekly|2026-10-10|checkin", "met|2026-08-15|link", "dismissed||link", "series||link"},
			suggestions: []string{},
			notSeen:     []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.SightingsFor(context.Background(), "c1", "e1", tt.viewer)
			if err != nil {
				t.Fatal(err)
			}
			if g := sightingDates(got.Seen); !equalStrings(g, tt.seen) {
				t.Errorf("seen = %v, want %v", g, tt.seen)
			}
			if g := sightingDates(got.Suggestions); !equalStrings(g, tt.suggestions) {
				t.Errorf("suggestions = %v, want %v", g, tt.suggestions)
			}
			if g := sightingDates(got.NotSeen); !equalStrings(g, tt.notSeen) {
				t.Errorf("notSeen = %v, want %v", g, tt.notSeen)
			}
			if got.Seen[0].AddedBy != tt.addedBy {
				t.Errorf("addedBy = %q, want %q", got.Seen[0].AddedBy, tt.addedBy)
			}
		})
	}
}

func TestSightingsFor_HiddenPageGetsNothing(t *testing.T) {
	repo := &fakeSightingRepo{links: []SightingLink{{SessionID: "s", Role: EntityRoleKey, ScheduledDate: "2026-01-01"}}}
	for name, ev := range map[string]EntityVisibilityFilter{
		"hidden page":     &stubEntityVisibility{hidden: map[string]bool{"e1": true}},
		"no filter wired": nil,
	} {
		t.Run(name, func(t *testing.T) {
			svc := NewSightingService(repo, &mockSessionRepo{}, &mockEntityChecker{}, ev)
			got, err := svc.SightingsFor(context.Background(), "c1", "e1", playerViewer)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Seen) != 0 {
				t.Errorf("seen = %v, want none", got.Seen)
			}
		})
	}
}

func TestRecordSighting(t *testing.T) {
	weekly, oneoff := RecurrenceWeekly, "2026-10-10"
	sessions := &mockSessionRepo{findByIDFn: func(_ context.Context, id string) (*Session, error) {
		switch id {
		case "weekly":
			d := "2026-10-03"
			return &Session{ID: id, CampaignID: "c1", ScheduledDate: &d, IsRecurring: true, RecurrenceType: &weekly}, nil
		case "oneoff":
			return &Session{ID: id, CampaignID: "c1", ScheduledDate: &oneoff}, nil
		case "other":
			return &Session{ID: id, CampaignID: "c2", ScheduledDate: &oneoff}, nil
		}
		return nil, apperror.NewNotFound("session not found")
	}}
	checker := &mockEntityChecker{belongsFn: func(_ context.Context, entityID, _ string) (bool, error) {
		return entityID != "foreign", nil
	}}

	tests := []struct {
		name   string
		in     RecordSightingInput
		viewer permissions.Viewer
		status int // 0 = success
		source string
	}{
		{"one-off night", RecordSightingInput{SessionID: "oneoff", Date: "2026-10-10", EntityID: "e1", State: SightingSeen, Source: SightingSourceCheckin}, dmViewer, 0, SightingSourceCheckin},
		{"a later night of a weekly series", RecordSightingInput{SessionID: "weekly", Date: "2026-10-17", EntityID: "e1", State: SightingNotSeen}, dmViewer, 0, SightingSourceManual},
		{"not a night of the series", RecordSightingInput{SessionID: "weekly", Date: "2026-10-18", EntityID: "e1", State: SightingSeen}, dmViewer, http.StatusBadRequest, ""},
		{"not the one-off's date", RecordSightingInput{SessionID: "oneoff", Date: "2026-10-11", EntityID: "e1", State: SightingSeen}, dmViewer, http.StatusBadRequest, ""},
		{"player may not record", RecordSightingInput{SessionID: "oneoff", Date: "2026-10-10", EntityID: "e1", State: SightingSeen}, playerViewer, http.StatusForbidden, ""},
		{"bad state", RecordSightingInput{SessionID: "oneoff", Date: "2026-10-10", EntityID: "e1", State: "maybe"}, dmViewer, http.StatusBadRequest, ""},
		{"link is never stored", RecordSightingInput{SessionID: "oneoff", Date: "2026-10-10", EntityID: "e1", State: SightingSeen, Source: SightingSourceLink}, dmViewer, http.StatusBadRequest, ""},
		{"bad date", RecordSightingInput{SessionID: "oneoff", Date: "10/10/2026", EntityID: "e1", State: SightingSeen}, dmViewer, http.StatusBadRequest, ""},
		{"another campaign's session", RecordSightingInput{SessionID: "other", Date: "2026-10-10", EntityID: "e1", State: SightingSeen}, dmViewer, http.StatusNotFound, ""},
		{"another campaign's page", RecordSightingInput{SessionID: "oneoff", Date: "2026-10-10", EntityID: "foreign", State: SightingSeen}, dmViewer, http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeSightingRepo{}
			svc := NewSightingService(repo, sessions, checker, &stubEntityVisibility{})
			err := svc.RecordSighting(context.Background(), "c1", tt.in, tt.viewer)
			if tt.status != 0 {
				assertAppError(t, err, tt.status)
				if len(repo.upserted) != 0 {
					t.Errorf("wrote %v on a rejected answer", repo.upserted)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(repo.upserted) != 1 || repo.upserted[0].Source != tt.source || repo.upserted[0].AddedBy != "dm1" {
				t.Errorf("upserted = %+v, want one row from dm1 with source %q", repo.upserted, tt.source)
			}
		})
	}
}

func TestClearSighting(t *testing.T) {
	d := "2026-10-10"
	sessions := &mockSessionRepo{findByIDFn: func(_ context.Context, id string) (*Session, error) {
		return &Session{ID: id, CampaignID: "c1", ScheduledDate: &d}, nil
	}}
	repo := &fakeSightingRepo{}
	svc := NewSightingService(repo, sessions, &mockEntityChecker{}, &stubEntityVisibility{})

	assertAppError(t, svc.ClearSighting(context.Background(), "c1", "s1", d, "e1", playerViewer), http.StatusForbidden)
	if err := svc.ClearSighting(context.Background(), "c1", "s1", d, "e1", dmViewer); err != nil {
		t.Fatal(err)
	}
	if !equalStrings(repo.deleted, []string{"s1|2026-10-10|e1"}) {
		t.Errorf("deleted = %v", repo.deleted)
	}
}
