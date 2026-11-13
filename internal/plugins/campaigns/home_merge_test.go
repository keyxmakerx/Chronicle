package campaigns

import "testing"

func blk(id, typ string) DashboardBlock { return DashboardBlock{ID: id, Type: typ} }

func row(id string, cols ...DashboardColumn) DashboardRow { return DashboardRow{ID: id, Columns: cols} }

func col(id string, w int, bs ...DashboardBlock) DashboardColumn {
	return DashboardColumn{ID: id, Width: w, Blocks: bs}
}

// blockIDs lists every block id in reading order.
func blockIDs(l *DashboardLayout) []string {
	var out []string
	for _, r := range l.Rows {
		for _, c := range r.Columns {
			for _, b := range c.Blocks {
				out = append(out, b.ID)
			}
		}
	}
	return out
}

func TestMergeDirectorExtras(t *testing.T) {
	everyone := &DashboardLayout{Rows: []DashboardRow{
		row("r-welcome", col("c-w", 12, blk("b-welcome", "welcome_banner"))),
		row("r-main", col("c-m", 12, blk("b-recent", "recent_pages"), blk("b-pins", "pinned_pages"))),
	}}
	tests := []struct {
		name string
		own  *DashboardLayout
		want []string
	}{
		{"empty director home is just Everyone", &DashboardLayout{}, []string{"b-welcome", "b-recent", "b-pins"}},
		{"a starter copy adds nothing but its extra, kept under the banner",
			&DashboardLayout{Rows: []DashboardRow{
				row("r-welcome", col("c-w", 12, blk("b-welcome", "welcome_banner"))),
				row("r-setup", col("c-s", 12, blk("b-setup", "text_block"))),
				row("r-main", col("c-m", 12, blk("b-recent", "recent_pages"))),
			}},
			[]string{"b-welcome", "b-setup", "b-recent", "b-pins"}},
		{"an old copy with different ids is still recognised",
			&DashboardLayout{Rows: []DashboardRow{row("old", col("oc", 12, blk("old-recent", "recent_pages"), blk("old-feed", "activity_feed")))}},
			[]string{"b-welcome", "b-recent", "b-pins", "old-feed"}},
		{"an extra with a colliding row id keeps unique ids",
			&DashboardLayout{Rows: []DashboardRow{row("r-main", col("c-m", 12, blk("b-feed", "activity_feed")))}},
			[]string{"b-welcome", "b-recent", "b-pins", "b-feed"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeDirectorExtras(everyone, tc.own)
			ids := blockIDs(got)
			if len(ids) != len(tc.want) {
				t.Fatalf("blocks = %v, want %v", ids, tc.want)
			}
			for i := range ids {
				if ids[i] != tc.want[i] {
					t.Fatalf("blocks = %v, want %v", ids, tc.want)
				}
			}
			seen := map[string]bool{}
			for _, r := range got.Rows {
				if seen[r.ID] {
					t.Fatalf("duplicate row id %q", r.ID)
				}
				seen[r.ID] = true
			}
		})
	}
	if len(everyone.Rows) != 2 || len(everyone.Rows[1].Columns[0].Blocks) != 2 {
		t.Fatal("the Everyone layout was modified")
	}
}
