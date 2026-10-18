package app

import (
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
)

func TestKeepRowIDs(t *testing.T) {
	old := []encounters.Creature{{ID: "r1", EntityID: "boss"}, {ID: "ai1", EntityID: "imp"}, {ID: "r3", EntityID: "imp"}}
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"same pages keep their ids in order", []string{"imp", "boss", "imp"}, []string{"ai1", "r1", "r3"}},
		{"a new page gets an id no old row had", []string{"boss", "wolf"}, []string{"r1", "ai2"}},
		{"more rows than before", []string{"imp", "imp", "imp"}, []string{"ai1", "r3", "ai2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := make([]encounters.Creature, len(tt.in))
			for i, e := range tt.in {
				rows[i] = encounters.Creature{ID: "ai" + string(rune('1'+i)), EntityID: e}
			}
			keepRowIDs(rows, old)
			got := []string{}
			for _, r := range rows {
				got = append(got, r.ID)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ids = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCarryStagesForAIUpdate(t *testing.T) {
	stages := []encounters.Stage{
		{ID: "s1", When: encounters.StageWhen{Kind: encounters.WhenStamina, Row: "r1", Percent: 50},
			Forms: []encounters.StageForm{{Row: "r1", Form: "swarm"}}, Arrive: []string{"r3"}},
		{ID: "s2", When: encounters.StageWhen{Kind: encounters.WhenManual}, Arrive: []string{"gone"}},
	}
	rows := []encounters.Creature{{ID: "r1", EntityID: "boss"}}
	got := encounters.CarryStages(stages, rows)
	if len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("stages = %+v, want only s1", got)
	}
	if len(got[0].Arrive) != 0 || len(got[0].Forms) != 1 || got[0].When.Row != "r1" {
		t.Fatalf("s1 = %+v", got[0])
	}
}
