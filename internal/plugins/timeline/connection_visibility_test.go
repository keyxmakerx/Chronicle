package timeline

import (
	"context"
	"testing"
)

// A connection reaches a viewer only when both of its ends are events that
// viewer may see; the event list passed in is already filtered for them.
func TestListConnections_OnlyBetweenVisibleEvents(t *testing.T) {
	// Source types are opaque here; "linked" stands in for a linked event.
	const own, linked = "standalone", "linked"
	label := "The Board pays the Rats"
	conns := []EventConnection{
		{ID: 1, SourceType: own, SourceID: "a", TargetType: own, TargetID: "b"},
		{ID: 2, SourceType: own, SourceID: "a", TargetType: linked, TargetID: "secret", Label: &label},
		{ID: 3, SourceType: linked, SourceID: "secret", TargetType: own, TargetID: "b"},
		{ID: 4, SourceType: linked, SourceID: "a", TargetType: own, TargetID: "b"},
	}
	visible := []EventLink{{EventID: "a", Source: own}, {EventID: "b", Source: own}}
	withSecret := append(append([]EventLink{}, visible...), EventLink{EventID: "secret", Source: linked})

	tests := []struct {
		name   string
		events []EventLink
		want   []int
	}{
		{"player drops every connection touching a hidden event", visible, []int{1}},
		{"director sees them all but a mismatched source type", withSecret, []int{1, 2, 3}},
		{"no visible events, no connections", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewTimelineService(&mockTimelineRepo{
				listConnectionsFn: func(context.Context, string) ([]EventConnection, error) { return conns, nil },
			}, nil, nil, nil)
			got, err := svc.ListConnections(context.Background(), "tl-1", tc.events)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d connections, want %d", len(got), len(tc.want))
			}
			for i, c := range got {
				if c.ID != tc.want[i] {
					t.Errorf("connection %d: got id %d, want %d", i, c.ID, tc.want[i])
				}
			}
		})
	}
}
