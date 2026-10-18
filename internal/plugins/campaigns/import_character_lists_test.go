package campaigns

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestRemapCharacterLists pins how an import carries the character and NPC
// lists: every id moves to its new category, ids the file never had are
// dropped, a category that failed to come back clears both lists, and a file
// without lists leaves them unset.
func TestRemapCharacterLists(t *testing.T) {
	exported := map[int]bool{1: true, 2: true, 3: true}
	cases := []struct {
		name      string
		settings  string
		typeIDs   map[int]int
		wantChars []int
		wantNPCs  []int
		wantState listsState
	}{
		{"both lists move to the new ids", `{"character_type_ids":[1],"npc_type_ids":[2,3]}`,
			map[int]int{1: 11, 2: 12, 3: 13}, []int{11}, []int{12, 13}, listsRemapped},
		{"an empty choice stays empty", `{"character_type_ids":[],"npc_type_ids":[2]}`,
			map[int]int{1: 11, 2: 12, 3: 13}, []int{}, []int{12}, listsRemapped},
		{"an id already stale in the source is dropped", `{"character_type_ids":[1,99],"npc_type_ids":[2]}`,
			map[int]int{1: 11, 2: 12, 3: 13}, []int{11}, []int{12}, listsRemapped},
		{"a category that did not come back clears both", `{"character_type_ids":[1],"npc_type_ids":[3]}`,
			map[int]int{1: 11, 2: 12}, nil, nil, listsLost},
		{"settings without the lists", `{"system_id":"drawsteel"}`,
			map[int]int{1: 11}, nil, nil, listsAbsent},
		{"no settings", ``, map[int]int{1: 11}, nil, nil, listsAbsent},
		{"unreadable settings", `{`, map[int]int{1: 11}, nil, nil, listsAbsent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chars, npcs, state := remapCharacterLists(json.RawMessage(tc.settings), exported, tc.typeIDs)
			if state != tc.wantState || !reflect.DeepEqual(chars, tc.wantChars) || !reflect.DeepEqual(npcs, tc.wantNPCs) {
				t.Fatalf("got %v %v %v, want %v %v %v", chars, npcs, state, tc.wantChars, tc.wantNPCs, tc.wantState)
			}
		})
	}
}
