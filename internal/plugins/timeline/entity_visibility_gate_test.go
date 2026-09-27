// entity_visibility_gate_test.go pins ADR-055 rule 3 for the timeline
// plugin's own list methods, mirroring maps.ListMarkers' equivalent
// coverage (maps/service_test.go): ListTimelineEvents and ListEntityGroups
// must blank a linked entity's id/name/icon when the viewer isn't
// separately permitted to see that entity, an Owner/co-DM must see it
// unfiltered (and never even consult the gate), and an unwired or failing
// gate must fail closed rather than leak. These run without a database, so
// they exercise the blanking logic in ordinary `-short` CI, unlike the
// real-MariaDB proof in internal/app/timeline_entity_visibility_leak_test.go.
package timeline

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// fakeEntityVisibilityGate is a stub EntityVisibilityGate: it reports
// exactly the entity IDs in `viewable` as visible and records whether it
// was consulted at all.
type fakeEntityVisibilityGate struct {
	viewable map[string]bool
	err      error
	called   bool
}

func (g *fakeEntityVisibilityGate) FilterViewableEntityIDs(_ context.Context, _ string, _ []string, _ int, _ string) (map[string]bool, error) {
	g.called = true
	if g.err != nil {
		return nil, g.err
	}
	return g.viewable, nil
}

// --- ListTimelineEvents ---

func entityLinkedStandaloneEvent() TimelineEvent {
	entID := "ent-secret"
	return TimelineEvent{
		ID: "evt-1", TimelineID: "tl-1", Name: "Meeting With The Spy",
		Year: 1, Month: 1, Day: 1, Visibility: "everyone",
		EntityID: &entID, EntityName: "Secret Spy", EntityIcon: "fa-user-secret",
	}
}

func TestListTimelineEvents_BlanksEntityNameForUnviewableEntity(t *testing.T) {
	repo := &mockTimelineRepo{
		listStandaloneEventsFn: func(_ context.Context, _ string, _ int) ([]TimelineEvent, error) {
			return []TimelineEvent{entityLinkedStandaloneEvent()}, nil
		},
	}
	gate := &fakeEntityVisibilityGate{viewable: map[string]bool{}} // entity not viewable
	svc := newTestTimelineService(repo)
	svc.(*timelineService).entityGate = gate

	events, err := svc.ListTimelineEvents(context.Background(), "tl-1", "camp-1", permissions.RequestViewer(permissions.RolePlayer, "player-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gate.called {
		t.Fatal("expected the entity visibility gate to be consulted for a player")
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 event, got %d", len(events))
	}
	if events[0].EventName != "Meeting With The Spy" {
		t.Errorf("the event itself must still show; got name %q", events[0].EventName)
	}
	if events[0].EventEntityID != nil || events[0].EventEntityName != "" || events[0].EventEntityIcon != "" {
		t.Errorf("LEAK: expected entity id/name/icon blanked, got id=%v name=%q icon=%q",
			events[0].EventEntityID, events[0].EventEntityName, events[0].EventEntityIcon)
	}
}

func TestListTimelineEvents_OwnerSeesEntityNameUnfiltered(t *testing.T) {
	repo := &mockTimelineRepo{
		listStandaloneEventsFn: func(_ context.Context, _ string, _ int) ([]TimelineEvent, error) {
			return []TimelineEvent{entityLinkedStandaloneEvent()}, nil
		},
	}
	gate := &fakeEntityVisibilityGate{viewable: map[string]bool{}}
	svc := newTestTimelineService(repo)
	svc.(*timelineService).entityGate = gate

	events, err := svc.ListTimelineEvents(context.Background(), "tl-1", "camp-1", permissions.RequestViewer(permissions.RoleOwner, "owner-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.called {
		t.Error("expected the entity visibility gate NOT to be consulted for an owner")
	}
	if events[0].EventEntityName != "Secret Spy" {
		t.Errorf("expected owner to see the entity name unfiltered, got %q", events[0].EventEntityName)
	}
}

// TestListTimelineEvents_FailsClosedWhenGateUnwired pins the fail-closed
// default: an unconfigured (nil) EntityVisibilityGate must never be treated
// as "everything viewable".
func TestListTimelineEvents_FailsClosedWhenGateUnwired(t *testing.T) {
	repo := &mockTimelineRepo{
		listStandaloneEventsFn: func(_ context.Context, _ string, _ int) ([]TimelineEvent, error) {
			return []TimelineEvent{entityLinkedStandaloneEvent()}, nil
		},
	}
	svc := newTestTimelineService(repo) // no entityGate set

	events, err := svc.ListTimelineEvents(context.Background(), "tl-1", "camp-1", permissions.RequestViewer(permissions.RolePlayer, "player-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if events[0].EventEntityID != nil || events[0].EventEntityName != "" || events[0].EventEntityIcon != "" {
		t.Errorf("expected fail-closed blanking with no gate configured, got id=%v name=%q icon=%q",
			events[0].EventEntityID, events[0].EventEntityName, events[0].EventEntityIcon)
	}
}

func TestListTimelineEvents_PropagatesGateError(t *testing.T) {
	repo := &mockTimelineRepo{
		listStandaloneEventsFn: func(_ context.Context, _ string, _ int) ([]TimelineEvent, error) {
			return []TimelineEvent{entityLinkedStandaloneEvent()}, nil
		},
	}
	gate := &fakeEntityVisibilityGate{err: errors.New("boom")}
	svc := newTestTimelineService(repo)
	svc.(*timelineService).entityGate = gate

	_, err := svc.ListTimelineEvents(context.Background(), "tl-1", "camp-1", permissions.RequestViewer(permissions.RolePlayer, "player-1"))
	if err == nil {
		t.Fatal("expected a gate error to propagate, got nil")
	}
}

// --- ListEntityGroups ---

func entityLinkedGroup() EntityGroup {
	return EntityGroup{
		ID: 1, TimelineID: "tl-1", Name: "Cult",
		Members: []EntityGroupMember{
			{ID: 1, GroupID: 1, EntityID: "ent-secret", EntityName: "Secret Spy", EntityIcon: "fa-user-secret"},
		},
	}
}

func TestListEntityGroups_BlanksEntityNameForUnviewableEntity(t *testing.T) {
	repo := &mockTimelineRepo{
		listEntityGroupsFn: func(_ context.Context, _ string) ([]EntityGroup, error) {
			return []EntityGroup{entityLinkedGroup()}, nil
		},
	}
	gate := &fakeEntityVisibilityGate{viewable: map[string]bool{}}
	svc := newTestTimelineService(repo)
	svc.(*timelineService).entityGate = gate

	groups, err := svc.ListEntityGroups(context.Background(), "tl-1", "camp-1", permissions.RequestViewer(permissions.RolePlayer, "player-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gate.called {
		t.Fatal("expected the entity visibility gate to be consulted for a player")
	}
	if len(groups) != 1 || len(groups[0].Members) != 1 {
		t.Fatalf("expected exactly 1 group with 1 member, got %+v", groups)
	}
	m := groups[0].Members[0]
	if m.EntityID != "" || m.EntityName != "" || m.EntityIcon != "" {
		t.Errorf("LEAK: expected member id/name/icon blanked, got id=%q name=%q icon=%q", m.EntityID, m.EntityName, m.EntityIcon)
	}
}

func TestListEntityGroups_OwnerSeesEntityNameUnfiltered(t *testing.T) {
	repo := &mockTimelineRepo{
		listEntityGroupsFn: func(_ context.Context, _ string) ([]EntityGroup, error) {
			return []EntityGroup{entityLinkedGroup()}, nil
		},
	}
	gate := &fakeEntityVisibilityGate{viewable: map[string]bool{}}
	svc := newTestTimelineService(repo)
	svc.(*timelineService).entityGate = gate

	groups, err := svc.ListEntityGroups(context.Background(), "tl-1", "camp-1", permissions.RequestViewer(permissions.RoleOwner, "owner-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.called {
		t.Error("expected the entity visibility gate NOT to be consulted for an owner")
	}
	if groups[0].Members[0].EntityName != "Secret Spy" {
		t.Errorf("expected owner to see the entity name unfiltered, got %q", groups[0].Members[0].EntityName)
	}
}

func TestListEntityGroups_FailsClosedWhenGateUnwired(t *testing.T) {
	repo := &mockTimelineRepo{
		listEntityGroupsFn: func(_ context.Context, _ string) ([]EntityGroup, error) {
			return []EntityGroup{entityLinkedGroup()}, nil
		},
	}
	svc := newTestTimelineService(repo) // no entityGate set

	groups, err := svc.ListEntityGroups(context.Background(), "tl-1", "camp-1", permissions.RequestViewer(permissions.RolePlayer, "player-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := groups[0].Members[0]
	if m.EntityID != "" || m.EntityName != "" || m.EntityIcon != "" {
		t.Errorf("expected fail-closed blanking with no gate configured, got id=%q name=%q icon=%q", m.EntityID, m.EntityName, m.EntityIcon)
	}
}

func TestListEntityGroups_PropagatesGateError(t *testing.T) {
	repo := &mockTimelineRepo{
		listEntityGroupsFn: func(_ context.Context, _ string) ([]EntityGroup, error) {
			return []EntityGroup{entityLinkedGroup()}, nil
		},
	}
	gate := &fakeEntityVisibilityGate{err: errors.New("boom")}
	svc := newTestTimelineService(repo)
	svc.(*timelineService).entityGate = gate

	_, err := svc.ListEntityGroups(context.Background(), "tl-1", "camp-1", permissions.RequestViewer(permissions.RolePlayer, "player-1"))
	if err == nil {
		t.Fatal("expected a gate error to propagate, got nil")
	}
}
