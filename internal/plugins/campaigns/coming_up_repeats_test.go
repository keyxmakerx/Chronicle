package campaigns

import "testing"

// TestComingUpRepeats pins that a dashboard shows the Coming up card once,
// counting the retired session tracker as the same card.
func TestComingUpRepeats(t *testing.T) {
	row := func(types ...string) DashboardRow {
		col := DashboardColumn{}
		for _, ty := range types {
			col.Blocks = append(col.Blocks, DashboardBlock{Type: ty})
		}
		return DashboardRow{Columns: []DashboardColumn{col}}
	}
	tests := []struct {
		name string
		rows []DashboardRow
		want map[[3]int]bool
	}{
		{"none", []DashboardRow{row(BlockWelcomeBanner)}, map[[3]int]bool{}},
		{"one coming up", []DashboardRow{row(BlockComingUp)}, map[[3]int]bool{}},
		{"old tracker alone", []DashboardRow{row(BlockSessionTracker)}, map[[3]int]bool{}},
		{"tracker then coming up", []DashboardRow{row(BlockSessionTracker), row(BlockWelcomeBanner, BlockComingUp)}, map[[3]int]bool{{1, 0, 1}: true}},
		{"two in one column", []DashboardRow{row(BlockComingUp, BlockComingUp)}, map[[3]int]bool{{0, 0, 1}: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&DashboardLayout{Rows: tt.rows}).ComingUpRepeats()
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k := range tt.want {
				if !got[k] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
