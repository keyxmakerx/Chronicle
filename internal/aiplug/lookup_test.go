package aiplug

import (
	"fmt"
	"strings"
	"testing"
)

func repeatLines(n, width int) []string {
	out := make([]string, n)
	for i := range out {
		head := fmt.Sprintf("line %04d ", i)
		out[i] = head + strings.Repeat("x", max(width-len(head), 0))
	}
	return out
}

// LinePieces cuts by count and by size, and every line lands in exactly one
// piece, in order.
func TestLinePieces(t *testing.T) {
	tests := []struct {
		name       string
		lines      []string
		budget     int
		wantPieces int
	}{
		{"empty", nil, 1000, 0},
		{"short list is one piece", repeatLines(10, 20), 1000, 1},
		{"cut by count", repeatLines(MaxAnswerLines*2+1, 10), 1 << 20, 3},
		{"cut by size", repeatLines(100, 99), 1000, 10},
		{"a line over the budget stands alone", []string{"a", strings.Repeat("y", 50), "b"}, 10, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			starts := LinePieces(tc.lines, tc.budget)
			if len(starts) != tc.wantPieces {
				t.Fatalf("%d pieces, want %d", len(starts), tc.wantPieces)
			}
			covered := 0
			for p := 1; p <= len(starts); p++ {
				start, end, ok := PieceRange(starts, len(tc.lines), p)
				if !ok || start != covered || end <= start {
					t.Fatalf("piece %d = [%d,%d) ok=%v after %d lines", p, start, end, ok, covered)
				}
				size := 0
				for _, l := range tc.lines[start:end] {
					size += len(l) + 1
				}
				if end-start > MaxAnswerLines || (end-start > 1 && size > tc.budget) {
					t.Errorf("piece %d holds %d lines, %d bytes", p, end-start, size)
				}
				covered = end
			}
			if covered != len(tc.lines) {
				t.Errorf("pieces cover %d of %d lines", covered, len(tc.lines))
			}
			if _, _, ok := PieceRange(starts, len(tc.lines), len(starts)+1); ok && len(starts) > 0 {
				t.Errorf("a piece past the end was given")
			}
		})
	}
}

// WriteLinesPiece writes a list that fits as WriteLines does, and keeps every
// piece of a long-lined one under the answer cap after its opening.
func TestWriteLinesPiece(t *testing.T) {
	opening := "## A list\n\n" + strings.Repeat("o", 2000) + "\n"
	tests := []struct {
		name     string
		lines    []string
		piece    int
		want     []string
		notWant  []string
		wantSame bool
	}{
		{"fits", repeatLines(5, 20), 1, nil, []string{"piece"}, true},
		{"first of a long-lined list", repeatLines(300, 120), 1, []string{"Lines 1 to", "(piece 1 of", "`piece: 2`"}, nil, false},
		{"last of a long-lined list", repeatLines(300, 120), 2, []string{"of 300 (piece 2 of 2)", "line 0299 "}, []string{"`piece: 3`"}, false},
		{"past the end", repeatLines(300, 120), 3, []string{"There is no piece 3: this list has 2 pieces."}, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			b.WriteString(opening)
			WriteLinesPiece(&b, tc.lines, tc.piece)
			got := b.String()
			if len(got) > MaxAnswerBytes {
				t.Errorf("%d bytes, over the cap", len(got))
			}
			if tc.wantSame {
				var w strings.Builder
				w.WriteString(opening)
				WriteLines(&w, tc.lines)
				if got != w.String() {
					t.Errorf("a list that fits was not written as WriteLines writes it")
				}
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q", w)
				}
			}
		})
	}
}
