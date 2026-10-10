// move_dialog_test.go pins the Move box's wording and shape: it folds open in
// place (no full-screen dialog), and the main button says "Ask the GM" exactly
// when the move waits for approval.
package armory

import (
	"context"
	"strings"
	"testing"
)

func TestMoveDialogRender(t *testing.T) {
	dests := []MoveDestination{
		{Endpoint: Endpoint{Kind: "stash", ID: "s1"}, Label: "Party stash", Group: "Stashes"},
		{Endpoint: Endpoint{Kind: "character", ID: "c2"}, Label: "Mira", Group: "Characters"},
	}
	base := MoveDialogView{
		CampaignID: "camp", Kind: "item", ItemID: "i1", ItemName: "Rope",
		From: Endpoint{Kind: "character", ID: "c1"}, FromName: "Bren", Max: 3, Destinations: dests,
	}
	tests := []struct {
		name    string
		mutate  func(v *MoveDialogView)
		want    []string
		notWant []string
	}{
		{
			name:   "player outside downtime asks the GM",
			mutate: func(v *MoveDialogView) { v.Immediate = false },
			want:   []string{"data-move-box", ">Ask the GM<", "Your GM has to approve this", `data-busy-label="Asking…"`, "Party stash", "Mira"},
			notWant: []string{
				`role="dialog"`, "fixed inset-0", ">Move<",
			},
		},
		{
			name:    "downtime open moves at once",
			mutate:  func(v *MoveDialogView) { v.Immediate = true },
			want:    []string{">Move<", "Downtime is open, so this happens now.", `data-busy-label="Moving…"`},
			notWant: []string{">Ask the GM<"},
		},
		{
			name:    "a single item has no quantity stepper",
			mutate:  func(v *MoveDialogView) { v.Max = 1 },
			notWant: []string{`id="ag-q"`},
			want:    []string{"data-move-go"},
		},
		{
			name:    "money asks for an amount",
			mutate:  func(v *MoveDialogView) { v.Kind = "money"; v.ItemID = ""; v.MaxMoney = 1500 },
			want:    []string{`id="ag-amount"`, "Move money"},
			notWant: []string{`id="ag-q"`},
		},
		{
			name:    "nowhere to go",
			mutate:  func(v *MoveDialogView) { v.Destinations = nil },
			want:    []string{"nowhere to move this to yet"},
			notWant: []string{"data-move-go"},
		},
		{
			name:    "nothing left",
			mutate:  func(v *MoveDialogView) { v.Max = 0 },
			want:    []string{"none of this left to move"},
			notWant: []string{"data-move-go"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := base
			tt.mutate(&v)
			var sb strings.Builder
			if err := MoveDialog(&v, "csrf").Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("output missing %q", w)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(out, w) {
					t.Errorf("output should not contain %q", w)
				}
			}
		})
	}
}
