package timeline

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// aiTimelineService is the real service over a repo that ignores the role,
// as if the SQL filter did not exist, so what hides a row is the service's
// own per-viewer rules the lookup is meant to inherit.
func aiTimelineService() TimelineService {
	repo := &mockTimelineRepo{
		listFn: func(context.Context, string, int) ([]Timeline, error) {
			return []Timeline{
				{ID: "t1", CampaignID: "c", Name: "The Long War", Visibility: "everyone"},
				{ID: "t2", CampaignID: "c", Name: "The Real Plan", Visibility: "dm_only"},
			}, nil
		},
		listStandaloneEventsFn: func(_ context.Context, id string, _ int) ([]TimelineEvent, error) {
			if id == "t2" {
				return []TimelineEvent{{ID: "e3", TimelineID: "t2", Name: "Betrayal planned", Year: 1200, Month: 1, Day: 1, Visibility: "dm_only"}}, nil
			}
			return []TimelineEvent{
				{ID: "e2", TimelineID: "t1", Name: "Siege of Harrowmere", Year: 1204, Month: 3, Day: 7, Visibility: "everyone", EntityName: "Harrowmere"},
				{ID: "e1", TimelineID: "t1", Name: "Secret pact", Year: 1203, Month: 2, Day: 1, Visibility: "dm_only"},
			}, nil
		},
	}
	return NewTimelineService(repo, nil, nil, nil)
}

func TestAITimelines_Answer(t *testing.T) {
	tests := []struct {
		name     string
		actor    aiplug.Actor
		has, not []string
	}{
		{"owner sees Director-only rows, marked", aiplug.Actor{UserID: "o", Role: 3},
			[]string{"The Long War", "The Real Plan (Director only)", "Secret pact [Director only]", "Betrayal planned", "1204-03-07: Siege of Harrowmere (page: Harrowmere)"}, nil},
		{"player-safe mode and a player's key do not", aiplug.Actor{UserID: "p", Role: 1},
			[]string{"The Long War", "Siege of Harrowmere"},
			[]string{"Real Plan", "Betrayal", "Secret pact", "Director only"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewAITimelines(aiTimelineService(), nil)
			_, text, err := l.Answer(context.Background(), "c", tt.actor, aiplug.Record{})
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.has {
				if !strings.Contains(text, w) {
					t.Errorf("answer lacks %q:\n%s", w, text)
				}
			}
			for _, w := range tt.not {
				if strings.Contains(text, w) {
					t.Errorf("answer holds %q:\n%s", w, text)
				}
			}
		})
	}
}

func TestAITimelines_RefusesWhenOff(t *testing.T) {
	l := NewAITimelines(aiTimelineService(), func(context.Context, string) string { return "Timelines is switched off" })
	if _, _, err := l.Answer(context.Background(), "c", aiplug.Actor{Role: 3}, aiplug.Record{}); err == nil || !strings.Contains(err.Error(), "switched off") {
		t.Errorf("err = %v", err)
	}
}

func TestAITimelines_ToolIsNotOwnerOnly(t *testing.T) {
	in := (&AITimelines{}).LookupInfo()
	if in.What != "timelines" || in.Tool.Name != "timelines" || in.Tool.Owner || in.Tool.PlayerDesc == "" {
		t.Errorf("info = %+v", in)
	}
}
