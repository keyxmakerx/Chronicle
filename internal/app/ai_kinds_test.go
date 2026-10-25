package app

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
	"github.com/keyxmakerx/chronicle/internal/plugins/pageblocks"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
	"github.com/keyxmakerx/chronicle/internal/widgets/entity_notes"
)

// fullCalendar stands in for the calendar service with its roll and moon
// halves; no method is called while the kinds are listed.
type fullCalendar struct {
	calendar.CalendarService
	calendar.EventRollService
	calendar.MoonMarkService
}

type stubFieldRules struct{ fieldrules.Service }
type stubPageBlocks struct{ pageblocks.Service }
type stubEncounters struct{ encounters.Service }
type stubQuestSvc struct{ quests.QuestService }
type stubBoardSvc struct{ quests.BoardService }
type stubPlayerNotes struct{ entity_notes.Service }

func aiKindNames(d aiDeps) string {
	var names []string
	for _, k := range records.NewRegistry(aiKinds(newAIDeps(d))...).Kinds() {
		names = append(names, k.Name())
	}
	return strings.Join(names, " ")
}

// TestAIKindsPromptOrder pins the prompt order of AI Import's kinds and that
// a degraded plugin's kinds are left out.
func TestAIKindsPromptOrder(t *testing.T) {
	healthy := database.NewPluginHealthRegistry()
	healthy.Register(rolltables.PluginSlug, true, nil, 1, 1)
	degraded := database.NewPluginHealthRegistry()
	degraded.Register(rolltables.PluginSlug, false, nil, 1, 1)

	tests := []struct {
		name string
		deps aiDeps
		want string
	}{
		{
			name: "everything wired",
			deps: aiDeps{Calendar: fullCalendar{}, Health: healthy, FieldRules: stubFieldRules{},
				PageBlocks: stubPageBlocks{}, Encounters: stubEncounters{}, Quests: stubQuestSvc{}, Boards: stubBoardSvc{}, PlayerNotes: stubPlayerNotes{}},
			want: "calendar event weather roll_night moon table shop-stock carried-item pin map note player-note house-rule system-entry quest notice-board field-rule local-word category-field encounter folder game-night tag menu-order page-map generator",
		},
		{
			name: "optional plugins degraded",
			deps: aiDeps{Health: degraded},
			want: "calendar event weather shop-stock carried-item pin map note house-rule system-entry category-field folder game-night tag menu-order generator",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aiKindNames(tt.deps); got != tt.want {
				t.Errorf("kinds =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// Every kind says in the prompt's opening list what it does, not only its
// label, so the AI can tell what a block is for before reading its format.
func TestAIKindsEachHaveABrief(t *testing.T) {
	healthy := database.NewPluginHealthRegistry()
	healthy.Register(rolltables.PluginSlug, true, nil, 1, 1)
	reg := records.NewRegistry(aiKinds(newAIDeps(aiDeps{Calendar: fullCalendar{}, Health: healthy, FieldRules: stubFieldRules{},
		PageBlocks: stubPageBlocks{}, Encounters: stubEncounters{}, Quests: stubQuestSvc{}, Boards: stubBoardSvc{}}))...)
	caps := reg.Capabilities()
	for _, k := range reg.Kinds() {
		if strings.Contains(caps, "`kind: "+k.Name()+"`: "+k.Label()+"\n") {
			t.Errorf("kind %s has only its label in the prompt", k.Name())
		}
	}
}
