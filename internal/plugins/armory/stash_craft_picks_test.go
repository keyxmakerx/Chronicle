package armory

import (
	"reflect"
	"testing"
)

func TestCraftPicksNeverSpendGoneOff(t *testing.T) {
	off := Copy{ID: 1, Quantity: 2, Memory: Memory{Off: true}}
	fine := Copy{ID: 2, Quantity: 2, Memory: Memory{Quality: "fine"}}
	tests := []struct {
		name       string
		fit        []Copy
		held, qty  int
		pick       []CopyPick
		want       []CopyPick
		wantUsable int
	}{
		{"plain covers it", []Copy{off}, 5, 3, nil, nil, 3},
		{"fresh copies make up the rest", []Copy{off, fine}, 5, 3, nil, []CopyPick{{ID: 2, N: 2}}, 3},
		{"a gone-off pick is ignored", []Copy{off, fine}, 4, 2, []CopyPick{{ID: 1}}, []CopyPick{{ID: 2, N: 2}}, 2},
		{"a fresh pick goes first", []Copy{off, fine}, 6, 1, []CopyPick{{ID: 2, N: 1}}, []CopyPick{{ID: 2, N: 1}}, 4},
		{"only gone-off ones left", []Copy{off}, 2, 1, nil, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, usable := craftPicks(tt.fit, tt.held, tt.qty, tt.pick)
			if !reflect.DeepEqual(got, tt.want) || usable != tt.wantUsable {
				t.Errorf("got %+v usable %d, want %+v usable %d", got, usable, tt.want, tt.wantUsable)
			}
			// Whatever the picks, the plan never takes a gone-off copy.
			if usable >= tt.qty {
				for _, c := range planTake(tt.fit, tt.held, tt.qty, got) {
					if c.Memory.Off {
						t.Errorf("planned to spend a gone-off copy: %+v", c)
					}
				}
			}
		})
	}
}
