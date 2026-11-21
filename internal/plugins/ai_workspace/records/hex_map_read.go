package records

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/keyxmakerx/chronicle/internal/hexsketch"
)

// hexReadBudget is the most bytes of hex lines one answer carries; a bigger
// map is answered a band of rows at a time, never cut mid-row.
const hexReadBudget = 18 * 1024

// hexRows is a band of rows, both ends included.
type hexRows struct{ from, to int }

// parseHexRows reads `rows: 20-39` (or one row, `7`).
func parseHexRows(s string) (hexRows, bool) {
	s = strings.TrimSpace(s)
	a, b, found := strings.Cut(s, "-")
	if !found {
		b = a
	}
	from, err1 := strconv.Atoi(strings.TrimSpace(a))
	to, err2 := strconv.Atoi(strings.TrimSpace(b))
	if err1 != nil || err2 != nil || from < 0 || to < from {
		return hexRows{}, false
	}
	return hexRows{from, to}, true
}

// hexMapRead writes what a hex map holds as a `hex-map` update block, so an
// AI sees the hexes before it paints over them. st is already what the reader
// may see: the caller read it as that reader, so fogged hexes never arrive
// here. band limits the rows; nil means as many rows from the top as fit.
// shown is the number of hexes written and more is true when rows were held
// back for another ask.
func hexMapRead(name string, st HexMapState, band *hexRows) (text string, shown int, more bool) {
	cols, rows := st.Cols, st.Rows
	for _, c := range st.Cells {
		cols, rows = max(cols, c.Col+1), max(rows, c.Row+1)
	}
	cols, rows = min(max(cols, hexsketch.MinSide), hexsketch.MaxSide), min(max(rows, hexsketch.MinSide), hexsketch.MaxSide)
	if len(st.Cells) == 0 {
		return "", 0, false
	}
	from, to := 0, rows-1
	if band != nil {
		from, to = min(band.from, rows-1), min(band.to, rows-1)
	}
	body, n := hexLines(st.Cells, from, to)
	if band == nil {
		// Shrink to the rows that fit, by halving: encoding is cheap and a
		// map this size is rare.
		for len(body) > hexReadBudget && to > from {
			to = from + (to-from)/2
			body, n = hexLines(st.Cells, from, to)
		}
	}
	more = to < rows-1 || from > 0
	var b strings.Builder
	fmt.Fprintf(&b, "```\n---\nkind: hex-map\naction: update\nname: %s\nsize: %d x %d\n---\n", quote(name), cols, rows)
	b.WriteString(body)
	b.WriteString("```\n")
	if more {
		fmt.Fprintf(&b, "\nThis is rows %d to %d of %d (rows count from 0). Ask again with `rows`, like `rows: %d-%d`, for others.\n",
			from, to, rows, min(to+1, rows-1), min(to+1+(to-from), rows-1))
	}
	return b.String(), n, more
}

// hexLines writes the hexes of rows from..to as body lines: land as boxes of
// one kind, then each line as the hexes it runs through, then named hexes.
// A line is written when either end of a join is in the band, so a river is
// not broken at the band's edge.
func hexLines(cells []HexCellState, from, to int) (string, int) {
	in := func(row int) bool { return row >= from && row <= to }
	byHex := make(map[hexsketch.Hex]HexCellState, len(cells))
	n := 0
	for _, c := range cells {
		byHex[hexsketch.Hex{Col: c.Col, Row: c.Row}] = c
		if in(c.Row) {
			n++
		}
	}
	var b strings.Builder
	writeLand(&b, byHex, from, to)
	for _, word := range hexsketch.Lines {
		writeLine(&b, word, byHex, in)
	}
	writePlaces(&b, cells, in)
	return b.String(), n
}

// writeLand covers every painted hex with boxes that hold one kind of land
// only, so reading the lines back gives the same land on the same hexes.
func writeLand(b *strings.Builder, byHex map[hexsketch.Hex]HexCellState, from, to int) {
	var hs []hexsketch.Hex
	for h, c := range byHex {
		if h.Row >= from && h.Row <= to && c.Terrain != "" {
			hs = append(hs, h)
		}
	}
	sortRowCol(hs)
	done := map[hexsketch.Hex]bool{}
	land := func(h hexsketch.Hex) string {
		if done[h] || h.Row < from || h.Row > to {
			return ""
		}
		return byHex[h].Terrain
	}
	for _, h := range hs {
		if done[h] {
			continue
		}
		kind := byHex[h].Terrain
		word, ok := hexsketch.Land(kind)
		if !ok {
			done[h] = true
			fmt.Fprintf(b, "# %d,%d holds %s, which cannot be written here\n", h.Col, h.Row, oneLine(kind, 30))
			continue
		}
		c1 := h.Col
		for land(hexsketch.Hex{Col: c1 + 1, Row: h.Row}) == kind {
			c1++
		}
		r1 := h.Row
	down:
		for {
			for c := h.Col; c <= c1; c++ {
				if land(hexsketch.Hex{Col: c, Row: r1 + 1}) != kind {
					break down
				}
			}
			r1++
		}
		for r := h.Row; r <= r1; r++ {
			for c := h.Col; c <= c1; c++ {
				done[hexsketch.Hex{Col: c, Row: r}] = true
			}
		}
		if c1 == h.Col && r1 == h.Row {
			fmt.Fprintf(b, "%s %d,%d\n", word, h.Col, h.Row)
		} else {
			fmt.Fprintf(b, "%s %d,%d to %d,%d\n", word, h.Col, h.Row, c1, r1)
		}
	}
}

// writeLine writes one kind of line as runs of neighbouring hexes. Only a
// join both hexes carry is written: a reader under fog is sent exits clipped
// to the hexes it sees, and a one-sided exit must not be painted back as a
// join into a hex it was never shown.
func writeLine(b *strings.Builder, word string, byHex map[hexsketch.Hex]HexCellState, in func(int) bool) {
	mask := func(c HexCellState) *int {
		switch word {
		case "road":
			return c.Road
		case "river":
			return c.River
		}
		return c.Trail
	}
	type edge struct{ a, b hexsketch.Hex }
	key := func(a, b hexsketch.Hex) edge {
		if b.Row < a.Row || (b.Row == a.Row && b.Col < a.Col) {
			a, b = b, a
		}
		return edge{a, b}
	}
	next := map[hexsketch.Hex][]hexsketch.Hex{}
	left := map[edge]bool{}
	var dots, nodes []hexsketch.Hex
	for h, c := range byHex {
		m := mask(c)
		if m == nil {
			continue
		}
		joined := false
		for side, nb := range hexsketch.Neighbors(h) {
			o, ok := byHex[nb]
			if *m&(1<<side) == 0 || !ok || mask(o) == nil || *mask(o)&(1<<((side+3)%6)) == 0 {
				continue
			}
			joined = true
			if !in(h.Row) && !in(nb.Row) {
				continue
			}
			next[h] = append(next[h], nb)
			left[key(h, nb)] = true
		}
		if !joined && in(h.Row) {
			dots = append(dots, h)
		}
	}
	for h := range next {
		sortRowCol(next[h])
		nodes = append(nodes, h)
	}
	// Ends first, so a plain river is one run from end to end.
	sortRowCol(nodes)
	sort.SliceStable(nodes, func(i, j int) bool { return len(next[nodes[i]])%2 > len(next[nodes[j]])%2 })
	for _, start := range nodes {
		for {
			path := []hexsketch.Hex{start}
			at := start
			for {
				moved := false
				for _, nb := range next[at] {
					if left[key(at, nb)] {
						delete(left, key(at, nb))
						path, at, moved = append(path, nb), nb, true
						break
					}
				}
				if !moved {
					break
				}
			}
			if len(path) == 1 {
				break
			}
			b.WriteString(word)
			for _, h := range path {
				fmt.Fprintf(b, " %d,%d", h.Col, h.Row)
			}
			b.WriteByte('\n')
		}
	}
	sortRowCol(dots)
	for _, h := range dots {
		fmt.Fprintf(b, "%s %d,%d\n", word, h.Col, h.Row)
	}
}

// writePlaces writes named hexes as `place` lines. Text a `place` line cannot
// carry (a quote in the name, notes of several lines, notes with no name) is
// written as `#` lines instead, which are read by the AI and skipped on the
// way back, so nothing is changed by sending the block back as it came.
func writePlaces(b *strings.Builder, cells []HexCellState, in func(int) bool) {
	sorted := append([]HexCellState(nil), cells...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Row != sorted[j].Row {
			return sorted[i].Row < sorted[j].Row
		}
		return sorted[i].Col < sorted[j].Col
	})
	for _, c := range sorted {
		if !in(c.Row) || (c.Name == "" && c.Notes == "") {
			continue
		}
		name, notes := strings.TrimSpace(c.Name), strings.TrimSpace(c.Notes)
		plainNotes := !strings.ContainsFunc(notes, unicode.IsControl)
		if name == "" {
			plainNotes = false
		} else if strings.ContainsAny(name, "\"“”") || strings.ContainsFunc(name, unicode.IsControl) {
			fmt.Fprintf(b, "# %d,%d is named %s; its name cannot be written as a place line\n", c.Col, c.Row, quote(name))
			plainNotes = false
		} else if notes != "" && plainNotes {
			fmt.Fprintf(b, "place \"%s\" %d,%d: %s\n", name, c.Col, c.Row, notes)
		} else {
			fmt.Fprintf(b, "place \"%s\" %d,%d\n", name, c.Col, c.Row)
		}
		if notes != "" && !plainNotes {
			fmt.Fprintf(b, "# notes on %d,%d (kept as they are unless a place line gives new ones):\n", c.Col, c.Row)
			for _, l := range strings.Split(notes, "\n") {
				fmt.Fprintf(b, "#   %s\n", strings.TrimSpace(strings.Map(func(r rune) rune {
					if unicode.IsControl(r) {
						return ' '
					}
					return r
				}, l)))
			}
		}
	}
}

func sortRowCol(hs []hexsketch.Hex) {
	sort.Slice(hs, func(i, j int) bool {
		if hs[i].Row != hs[j].Row {
			return hs[i].Row < hs[j].Row
		}
		return hs[i].Col < hs[j].Col
	})
}
