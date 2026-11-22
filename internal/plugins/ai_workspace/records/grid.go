package records

import (
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// gridRow is one row of a page-like arrangement of rows, columns and blocks:
// either columns of blocks or, for a tile section, tiles only the Customizer
// arranges. It reuses the home page's column and block shapes so a kind for
// another arrangement (a category's page layout today) parses, summarises and
// writes through the same code and keeps the same "a listed block takes over
// the settings of the one it stands for" rule.
type gridRow struct {
	ID      string
	Columns []HomeColumn
	Tiles   []tilegrid.Tile
	IsTiles bool
}

// gridRules tells parseGrid what one kind accepts.
type gridRules struct {
	// valid says whether a block type may be placed.
	valid func(typ string) bool
	// notBlock is the plain-words noun for a rejected type, e.g. "a page block".
	notBlock string
	// words are the block types whose `text` is markdown kept as HTML.
	words map[string]bool
	// tileRow reads a row written as `tiles: ...`; nil refuses tile rows.
	tileRow func(ri int, v any) (gridRow, error)
	// block applies what a kind adds to one block (its own keys); kept says
	// the block took over an existing one.
	block func(raw any, spec homeSpec, b *HomeBlock, kept bool) error
}

// parseGrid reads rows of columns of blocks as a record writes them, against
// the rows it replaces. A listed block takes over the ID and settings of the
// block of its type already there, so rearranging never loses a choice the
// owner made; new IDs never collide with any ID already in was, since a block
// moved to another place keeps its old ID.
func parseGrid(items []any, was []gridRow, rules gridRules) ([]gridRow, []homeWordsChange, error) {
	pool := map[string][]*poolBlock{}
	taken := map[string]bool{}
	for _, row := range was {
		for _, t := range row.Tiles {
			taken[t.ID] = true
		}
		for _, col := range row.Columns {
			for _, b := range col.Blocks {
				taken[b.ID] = true
				pool[b.Type] = append(pool[b.Type], &poolBlock{block: b})
			}
		}
	}
	claimWordMatches(items, pool)
	fresh := func(prefix string, parts ...int) string {
		id := prefix
		for _, p := range parts {
			id += fmt.Sprintf("-%d", p)
		}
		for taken[id] {
			id += "x"
		}
		taken[id] = true
		return id
	}

	var out []gridRow
	var words []homeWordsChange
	for ri, rv := range items {
		rm, ok := rv.(map[string]any)
		if !ok {
			return nil, nil, badRequestf("row %d: write it as `columns:` and a list of columns", ri+1)
		}
		if tv, has := rm["tiles"]; has {
			if rules.tileRow == nil {
				return nil, nil, badRequestf("row %d: this can't hold a tile section", ri+1)
			}
			row, err := rules.tileRow(ri, tv)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, row)
			continue
		}
		cols, _ := rm["columns"].([]any)
		if len(cols) == 0 {
			return nil, nil, badRequestf("row %d: list its `columns`", ri+1)
		}
		row := gridRow{ID: fresh("row-ai", ri+1)}
		for ci, cv := range cols {
			cm, ok := cv.(map[string]any)
			if !ok {
				return nil, nil, badRequestf("row %d, column %d: write it as `width:` and `blocks:`", ri+1, ci+1)
			}
			w, err := wholeNumber(cm["width"])
			if err != nil || w < 1 || w > 12 {
				return nil, nil, badRequestf("row %d, column %d: `width` is a whole number from 1 to 12", ri+1, ci+1)
			}
			col := HomeColumn{ID: fresh("col-ai", ri+1, ci+1), Width: w}
			blocks, _ := cm["blocks"].([]any)
			for bi, bv := range blocks {
				spec, err := homeBlockSpec(bv)
				if err != nil {
					return nil, nil, badRequestf("row %d, column %d, block %d: %v", ri+1, ci+1, bi+1, err)
				}
				typ := spec.typ
				if !rules.valid(typ) {
					return nil, nil, badRequestf("row %d, column %d: %s is not %s", ri+1, ci+1, quote(typ), rules.notBlock)
				}
				b := HomeBlock{Type: typ}
				old := takePoolBlock(pool[typ], blockKey(ri, ci, bi))
				if old != nil {
					b.ID, b.Config = old.ID, cloneConfig(old.Config)
				} else {
					b.ID = fresh("blk-ai", ri+1, ci+1, bi+1)
				}
				if spec.hasText && !rules.words[typ] {
					return nil, nil, badRequestf("row %d, column %d: only a text_block takes `text`; %s does not", ri+1, ci+1, quote(typ))
				}
				if rules.words[typ] && spec.hasText {
					if len([]rune(spec.text)) > maxHomeText {
						return nil, nil, badRequestf("row %d, column %d: the words in a %s block run over %d characters", ri+1, ci+1, typ, maxHomeText)
					}
					// Words sent back as they were read leave the stored block
					// alone, so a lookup sent back never re-renders them.
					was := storedWords(b.Config)
					if now := normalizeWords(spec.text); now != was {
						if b.Config == nil {
							b.Config = map[string]any{}
						}
						b.Config["_md"] = spec.text // turned into HTML at Apply
						words = append(words, homeWordsChange{typ: typ, was: was, now: now})
					}
				}
				if rules.block != nil {
					if err := rules.block(bv, spec, &b, old != nil); err != nil {
						return nil, nil, badRequestf("row %d, column %d, block %d: %v", ri+1, ci+1, bi+1, err)
					}
				}
				col.Blocks = append(col.Blocks, b)
			}
			row.Columns = append(row.Columns, col)
		}
		out = append(out, row)
	}
	return out, words, nil
}

// gridBlockWord is a block as a summary names it.
func gridBlockWord(b HomeBlock) string {
	if gridDirectorsOnly(b) {
		return b.Type + " (directors only)"
	}
	return b.Type
}

// gridDirectorsOnly says whether a block shows to the DM team alone.
func gridDirectorsOnly(b HomeBlock) bool {
	v, _ := b.Config["visibility"].(string)
	return v == "dm_only"
}

// gridSummary is an arrangement on one line: rows split by " / ", columns by
// " | ", a tile section named by what it holds.
func gridSummary(rows []gridRow) string {
	var out []string
	for _, row := range rows {
		if row.IsTiles {
			out = append(out, "[tile section: "+strings.Join(tileTypes(row), ", ")+"]")
			continue
		}
		var cols []string
		for _, c := range row.Columns {
			var types []string
			for _, b := range c.Blocks {
				types = append(types, gridBlockWord(b))
			}
			cols = append(cols, strings.Join(types, ", "))
		}
		out = append(out, strings.Join(cols, " | "))
	}
	return strings.Join(out, " / ")
}

func tileTypes(row gridRow) []string {
	var out []string
	for _, t := range row.Tiles {
		out = append(out, t.Type)
	}
	return out
}

// gridShape is every column's width, blocks and visibility, to tell a change
// from none. Words are compared separately, as they change through Config.
func gridShape(rows []gridRow) string {
	var b strings.Builder
	for _, row := range rows {
		b.WriteString("/")
		if row.IsTiles {
			b.WriteString("tiles:" + row.ID)
			continue
		}
		for _, c := range row.Columns {
			fmt.Fprintf(&b, "|%d:", c.Width)
			for _, bl := range c.Blocks {
				b.WriteString(bl.Type)
				if gridDirectorsOnly(bl) {
					b.WriteString("!")
				}
				b.WriteString(",")
			}
		}
	}
	return b.String()
}

// gridDropped lists what the old arrangement held more of than the new: block
// types, and tile sections that were not kept.
func gridDropped(was, next []gridRow) []string {
	count := func(rows []gridRow) (map[string]int, int) {
		m, tiles := map[string]int{}, 0
		for _, row := range rows {
			if row.IsTiles {
				tiles++
			}
			for _, c := range row.Columns {
				for _, b := range c.Blocks {
					m[b.Type]++
				}
			}
		}
		return m, tiles
	}
	w, _ := count(was)
	n, nt := count(next)
	var out []string
	for _, row := range was {
		for _, c := range row.Columns {
			for _, b := range c.Blocks {
				if w[b.Type] > n[b.Type] {
					out = append(out, b.Type)
					w[b.Type] = n[b.Type]
				}
			}
		}
	}
	// Tile rows are kept in order, so the ones past the count kept are gone.
	seen := 0
	for _, row := range was {
		if !row.IsTiles {
			continue
		}
		seen++
		if seen > nt {
			out = append(out, "a tile section ("+strings.Join(tileTypes(row), ", ")+")")
		}
	}
	return out
}

// writeGrid writes rows as the `rows:` list a record reads. spec writes one
// block; a tile section is written as `tiles: kept`.
func writeGrid(b *strings.Builder, rows []gridRow, spec func(HomeBlock) string) {
	b.WriteString("rows:\n")
	for _, row := range rows {
		if row.IsTiles {
			b.WriteString("  - tiles: kept\n")
			continue
		}
		b.WriteString("  - columns:\n")
		for _, c := range row.Columns {
			fmt.Fprintf(b, "      - width: %d\n        blocks:\n", c.Width)
			for _, bl := range c.Blocks {
				b.WriteString("          - " + spec(bl) + "\n")
			}
		}
	}
}
