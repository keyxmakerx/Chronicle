package app

// page_timeline_link_test.go pins where the page time line's link goes: a
// time line that follows the calendar, opened at the page's latest event up
// to today, only when the time line add-on is on and the viewer may see one.

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

type fakeTimelinesForCalendar struct {
	tls []timeline.Timeline
	err error
}

func (f fakeTimelinesForCalendar) ListTimelinesForCalendar(context.Context, string, permissions.Viewer) ([]timeline.Timeline, error) {
	return f.tls, f.err
}

type fakeAddonOn struct {
	on  bool
	err error
}

func (f fakeAddonOn) IsEnabledForCampaign(context.Context, string, string) (bool, error) { return f.on, f.err }

func TestPageTimelineLinker_FullTimeline(t *testing.T) {
	mine := timeline.Timeline{ID: "tl-1", CampaignID: "camp-1"}
	other := timeline.Timeline{ID: "tl-x", CampaignID: "camp-2"}
	tests := []struct {
		name   string
		addon  fakeAddonOn
		tls    fakeTimelinesForCalendar
		wantID string
		wantOK bool
	}{
		{"add-on on, one time line follows the calendar", fakeAddonOn{on: true}, fakeTimelinesForCalendar{tls: []timeline.Timeline{mine}}, "tl-1", true},
		{"add-on off links to the calendar", fakeAddonOn{on: false}, fakeTimelinesForCalendar{tls: []timeline.Timeline{mine}}, "", false},
		{"add-on check fails closed", fakeAddonOn{err: errors.New("db")}, fakeTimelinesForCalendar{tls: []timeline.Timeline{mine}}, "", false},
		{"no time line the viewer may see", fakeAddonOn{on: true}, fakeTimelinesForCalendar{}, "", false},
		{"a time line of another campaign is never linked", fakeAddonOn{on: true}, fakeTimelinesForCalendar{tls: []timeline.Timeline{other, mine}}, "tl-1", true},
		{"a list failure links to the calendar", fakeAddonOn{on: true}, fakeTimelinesForCalendar{err: errors.New("db")}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &pageTimelineLinker{timelines: tt.tls, addons: tt.addon}
			id, ok := l.FullTimeline(context.Background(), "camp-1", "cal-1", permissions.RequestViewer(permissions.RolePlayer, "u-1"))
			if id != tt.wantID || ok != tt.wantOK {
				t.Errorf("FullTimeline = %q, %v; want %q, %v", id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestPageTimelineFocus(t *testing.T) {
	line := func(today int, abs ...int) *calendar.PageTimeline {
		t := &calendar.PageTimeline{Today: today}
		for i, a := range abs {
			t.Events = append(t.Events, calendar.PageTimelineEvent{ID: string(rune('a' + i)), Abs: a})
		}
		return t
	}
	tests := []struct {
		name string
		t    *calendar.PageTimeline
		want string
	}{
		{"the latest event up to today", line(50, 10, 40, 60), "b"},
		{"an event on today counts", line(40, 10, 40, 60), "b"},
		{"all ahead: the first", line(5, 10, 40), "a"},
		{"a dead subject's line has no today: the last", &calendar.PageTimeline{Today: 0, Dead: true, Events: []calendar.PageTimelineEvent{{ID: "a", Abs: 10}, {ID: "b", Abs: 40}}}, "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pageTimelineFocus(tt.t); got != tt.want {
				t.Errorf("focus = %q, want %q", got, tt.want)
			}
		})
	}
}
