package campaigns

import (
	"encoding/json"
	"strconv"
)

// DirectorHome is the home the DM team sees: the Everyone home plus whatever
// the Director's own saved home holds that the Everyone home does not.
//
// Why a merge and not "the Director's saved home wins": a new campaign saves
// the Director a copy of the starter front page, and a saved Director layout
// used to shadow the Everyone home completely, so a widget added for everyone
// never reached the DM team. Merging at read time fixes that without
// rewriting any stored layout; the Director's saved home keeps its data and
// is read as "my extras on top of Everyone".
//
// A Director block counts as already on the Everyone home when it shares an
// id with one there, or has the same type and settings (an older copy whose
// ids differ). Everyone's version then stands, so a block the Director once
// copied follows later edits to the Everyone home instead of freezing. Every
// other Director block is an extra and is placed after the Everyone row that
// the Director's layout kept it behind, so a setup note under the welcome
// banner stays under it.
func (c *Campaign) DirectorHome() *DashboardLayout {
	everyone, _ := c.HomeLayout(HomeDefault)
	own := c.GetRoleDashboardJSON(HomeDirector)
	if own == nil {
		return everyone
	}
	return mergeDirectorExtras(everyone, own)
}

// homeEntry is one row of the merged home. It is a pointer target so extras
// can be anchored behind the exact Everyone row they followed.
type homeEntry struct {
	row DashboardRow
}

// blockSignature identifies a block by what it is and how it is set up, for
// recognising an old copy whose ids differ from the Everyone block's.
func blockSignature(b DashboardBlock) string {
	cfg, _ := json.Marshal(b.Config)
	return b.Type + "|" + string(cfg)
}

// mergeDirectorExtras returns everyone with the blocks of own that everyone
// lacks added in. Neither input is modified.
func mergeDirectorExtras(everyone, own *DashboardLayout) *DashboardLayout {
	entries := make([]*homeEntry, 0, len(everyone.Rows)+len(own.Rows))
	byBlockID := map[string]*homeEntry{}
	bySig := map[string]*homeEntry{}
	tileRows := map[string]*homeEntry{}
	usedRowIDs := map[string]bool{}
	usedColIDs := map[string]bool{}
	for _, r := range everyone.Rows {
		e := &homeEntry{row: r}
		entries = append(entries, e)
		usedRowIDs[r.ID] = true
		if r.Kind != "" {
			tileRows[r.ID] = e
		}
		for _, col := range r.Columns {
			usedColIDs[col.ID] = true
			for _, b := range col.Blocks {
				byBlockID[b.ID] = e
				if _, ok := bySig[blockSignature(b)]; !ok {
					bySig[blockSignature(b)] = e
				}
			}
		}
	}

	indexOf := func(target *homeEntry) int {
		for i, e := range entries {
			if e == target {
				return i
			}
		}
		return -1
	}
	// anchor is the entry new extras go behind; nil means nothing of the
	// Director's has matched yet, so extras go at the bottom.
	var anchor *homeEntry
	unique := func(id string, used map[string]bool) string {
		out := id
		for n := 2; used[out]; n++ {
			out = id + "-dm" + strconv.Itoa(n)
		}
		used[out] = true
		return out
	}
	insert := func(row DashboardRow) {
		e := &homeEntry{row: row}
		at := len(entries)
		if anchor != nil {
			at = indexOf(anchor) + 1
		}
		entries = append(entries, nil)
		copy(entries[at+1:], entries[at:])
		entries[at] = e
		anchor = e
	}

	for _, r := range own.Rows {
		if r.Kind != "" {
			if e, ok := tileRows[r.ID]; ok {
				anchor = e
				continue
			}
			r.ID = unique(r.ID, usedRowIDs)
			insert(r)
			continue
		}
		var extraCols []DashboardColumn
		for _, col := range r.Columns {
			var extra []DashboardBlock
			for _, b := range col.Blocks {
				if e, ok := byBlockID[b.ID]; ok {
					anchor = e
					continue
				}
				if e, ok := bySig[blockSignature(b)]; ok {
					anchor = e
					continue
				}
				extra = append(extra, b)
			}
			if len(extra) > 0 {
				extraCols = append(extraCols, DashboardColumn{ID: col.ID, Width: col.Width, Blocks: extra})
			}
		}
		if len(extraCols) == 0 {
			continue
		}
		// A lone surviving column of a split row widens to the full row
		// rather than leaving a gap where its neighbour was.
		if len(extraCols) == 1 && len(r.Columns) > 1 {
			extraCols[0].Width = 12
		}
		for i := range extraCols {
			extraCols[i].ID = unique(extraCols[i].ID, usedColIDs)
		}
		insert(DashboardRow{ID: unique(r.ID, usedRowIDs), Columns: extraCols})
	}

	out := &DashboardLayout{Rows: make([]DashboardRow, 0, len(entries))}
	for _, e := range entries {
		out.Rows = append(out.Rows, e.row)
	}
	return out
}
