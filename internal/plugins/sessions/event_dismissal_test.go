package sessions

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestDismissEventAPI(t *testing.T) {
	tests := []struct {
		name      string
		eventID   string
		wantErr   int
		wantStore bool
	}{
		{"a Session event is stored", "e1", 0, true},
		{"an id that is not a Session event is refused and never stored", "someone-elses-event", http.StatusNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stored []string
			repo := &mockSessionRepo{dismissEventFn: func(_ context.Context, camp, ev, _ string) error {
				stored = append(stored, camp+"/"+ev)
				return nil
			}}
			h := NewHandler(NewSessionService(repo, nil, nil))
			h.SetSessionEventSource(fakeEventSource{events: []SessionEvent{{ID: "e1", Name: "Session 1"}}})
			c, rec := eventCtx(http.MethodPut, "/x", campaigns.RoleScribe, "eid", tt.eventID)
			err := h.DismissEventAPI(c)
			if tt.wantErr != 0 {
				assertAppError(t, err, tt.wantErr)
			} else if err != nil || rec.Code != http.StatusNoContent {
				t.Fatalf("err %v, status %d", err, rec.Code)
			}
			if (len(stored) == 1) != tt.wantStore {
				t.Fatalf("stored %v, want stored=%v", stored, tt.wantStore)
			}
		})
	}
}

// The strip leaves out an event that was set aside, and one whose night is
// stored by link, and keeps the rest.
func TestEventsWithoutNightAPILeavesOutDismissedAndLinked(t *testing.T) {
	repo := &mockSessionRepo{
		listDismissedEventIDsFn: func(context.Context, string) ([]string, error) { return []string{"e2"}, nil },
		listLinkedEventIDsFn:    func(context.Context, string) ([]string, error) { return []string{"e3"}, nil },
	}
	h := NewHandler(NewSessionService(repo, nil, nil))
	h.SetSessionEventSource(fakeEventSource{events: []SessionEvent{
		{ID: "e1", Name: "One", When: "1 Hammer, 1492"}, {ID: "e2", Name: "Two"}, {ID: "e3", Name: "Three"},
	}})
	c, rec := eventCtx(http.MethodGet, "/x", campaigns.RoleScribe)
	if err := h.EventsWithoutNightAPI(c); err != nil {
		t.Fatal(err)
	}
	var rows []eventNightRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0].EventID != "e1" {
		t.Fatalf("rows = %+v, err %v", rows, err)
	}
}
