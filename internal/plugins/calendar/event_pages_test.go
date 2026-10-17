// event_pages_test.go: the gates on reading and changing the pages tied to
// an event. Each must refuse an event the viewer cannot see before it
// touches the tie table, and the read passes the viewer's role and id down
// so the repository can leave out pages they may not see.
package calendar

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// eventPagesFixture is one visible calendar holding an open event and a
// dm_only one, with the tie table's calls recorded.
type eventPagesFixture struct {
	svc              CalendarService
	linked, unlinked []string
	listedRole       int
	listedUser       string
	listCalls        int
}

func newEventPagesFixture() *eventPagesFixture {
	f := &eventPagesFixture{}
	events := map[string]Event{
		"open":   {ID: "open", CalendarID: "cal-1", Visibility: "everyone"},
		"secret": {ID: "secret", CalendarID: "cal-1", Visibility: "dm_only"},
		"other":  {ID: "other", CalendarID: "cal-2", Visibility: "everyone"},
	}
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, Visibility: "everyone"}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			if e, ok := events[id]; ok {
				return &e, nil
			}
			return nil, nil
		},
		entitiesForEventFn: func(_ context.Context, eventID string, role int, userID string) ([]EntityTieRef, error) {
			f.listCalls++
			f.listedRole, f.listedUser = role, userID
			return []EntityTieRef{{EntityID: "ent-1", EntityName: "Highwater"}}, nil
		},
		linkEntityEventFn: func(_ context.Context, entityID, eventID, role string) error {
			f.linked = append(f.linked, entityID+"/"+eventID+"/"+role)
			return nil
		},
		unlinkEntityEventFn: func(_ context.Context, entityID, eventID string) error {
			f.unlinked = append(f.unlinked, entityID+"/"+eventID)
			return nil
		},
	}
	f.svc = newTestCalendarService(calRepo, eventRepo, nil, nil)
	return f
}

func TestEventPages(t *testing.T) {
	tests := []struct {
		name     string
		eventID  string
		viewer   permissions.Viewer
		notFound bool
	}{
		{"player reads an open event's pages", "open", playerViewer("u-1"), false},
		{"player cannot read a dm_only event's pages", "secret", playerViewer("u-1"), true},
		{"owner reads a dm_only event's pages", "secret", ownerViewer("u-o"), false},
		{"an event on another calendar is not found", "other", ownerViewer("u-o"), true},
		{"an unknown event is not found", "nope", ownerViewer("u-o"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newEventPagesFixture()
			got, err := f.svc.EventPages(context.Background(), tc.eventID, "cal-1", testCampaignA, tc.viewer)
			if tc.notFound {
				assertNotFound(t, err)
				if f.listCalls != 0 {
					t.Errorf("the tie table was read for an event the viewer cannot see")
				}
				return
			}
			if err != nil {
				t.Fatalf("EventPages: %v", err)
			}
			if len(got) != 1 || got[0].EntityID != "ent-1" {
				t.Errorf("pages = %+v", got)
			}
			if f.listedRole != tc.viewer.Role() || f.listedUser != tc.viewer.UserID() {
				t.Errorf("repository asked as role %d user %q, want %d %q", f.listedRole, f.listedUser, tc.viewer.Role(), tc.viewer.UserID())
			}
		})
	}
}

func TestLinkAndUnlinkEventPage(t *testing.T) {
	tests := []struct {
		name     string
		eventID  string
		entityID string
		viewer   permissions.Viewer
		wantErr  bool
	}{
		{"scribe ties a page to an open event", "open", "ent-1", scribeViewer("u-s"), false},
		{"scribe cannot touch a dm_only event", "secret", "ent-1", scribeViewer("u-s"), true},
		{"owner ties a page to a dm_only event", "secret", "ent-1", ownerViewer("u-o"), false},
		{"an event on another calendar is refused", "other", "ent-1", ownerViewer("u-o"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newEventPagesFixture()
			err := f.svc.LinkEventPage(context.Background(), tc.eventID, "cal-1", testCampaignA, tc.entityID, tc.viewer)
			uerr := f.svc.UnlinkEventPage(context.Background(), tc.eventID, "cal-1", testCampaignA, tc.entityID, tc.viewer)
			if tc.wantErr {
				assertNotFound(t, err)
				assertNotFound(t, uerr)
				if len(f.linked)+len(f.unlinked) != 0 {
					t.Errorf("ties changed on a refused event: %v %v", f.linked, f.unlinked)
				}
				return
			}
			if err != nil || uerr != nil {
				t.Fatalf("link: %v, unlink: %v", err, uerr)
			}
			if want := tc.entityID + "/" + tc.eventID + "/involved"; len(f.linked) != 1 || f.linked[0] != want {
				t.Errorf("linked = %v, want [%s]", f.linked, want)
			}
			if want := tc.entityID + "/" + tc.eventID; len(f.unlinked) != 1 || f.unlinked[0] != want {
				t.Errorf("unlinked = %v, want [%s]", f.unlinked, want)
			}
		})
	}
}

func TestLinkEventPage_NeedsAPage(t *testing.T) {
	f := newEventPagesFixture()
	err := f.svc.LinkEventPage(context.Background(), "open", "cal-1", testCampaignA, "  ", ownerViewer("u-o"))
	if err == nil || len(f.linked) != 0 {
		t.Fatalf("a blank entity_id must be refused, got err=%v linked=%v", err, f.linked)
	}
}
