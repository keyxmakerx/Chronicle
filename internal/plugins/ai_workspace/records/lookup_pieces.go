package records

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A lookup answer is capped, and a list is capped in lines. Neither cap may
// put the rest out of reach, so both can be stepped through with `piece`:
// piece 1 (the default) is the first, piece 2 the next, and the answer says
// how many there are.

// pieceRoom is what a byte piece leaves free under the answer cap for the
// piece header and footer.
const pieceRoom = 1024

// pieceKey is the lookup field that picks a piece. It is not `page`, which
// several lookups already use for a page's name.
const pieceKey = "piece"

// readPiece is the piece a lookup asks for: 1 when it names none.
func readPiece(r Record) (int, error) {
	s := strings.TrimSpace(r.Str(pieceKey))
	if s == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, badRequestf("`piece` should be a whole number from 1, like 2")
	}
	return n, nil
}

// pieceWindow is the lines of a list that piece shows: from start up to end.
func pieceWindow(piece int) (start, end int) {
	return (piece - 1) * maxAnswerLines, piece * maxAnswerLines
}

// writeLines writes the piece of a list the lookup asked for, up to
// maxAnswerLines of it, and says how to read the rest. A list that fits is
// written whole, exactly as before.
func (run *lookupRun) writeLines(b *strings.Builder, lines []string) {
	if len(lines) <= maxAnswerLines && run.piece <= 1 {
		writeLines(b, lines)
		return
	}
	run.linesPaged = true
	pieces := (len(lines) + maxAnswerLines - 1) / maxAnswerLines
	start, end := pieceWindow(run.piece)
	if start >= len(lines) {
		fmt.Fprintf(b, "There is no piece %d: this list has %d (%s).\n", run.piece, len(lines), plural(pieces, "piece", "pieces"))
		return
	}
	end = min(end, len(lines))
	fmt.Fprintf(b, "Lines %d to %d of %d (piece %d of %d).\n\n", start+1, end, len(lines), run.piece, pieces)
	for _, l := range lines[start:end] {
		b.WriteString(l + "\n")
	}
	if end < len(lines) {
		fmt.Fprintf(b, "…%d more; ask again with `piece: %d` for the next.\n", len(lines)-end, run.piece+1)
	}
}

// pieceOf cuts s into pieces of at most limit bytes, at line boundaries so no
// line is split unless one alone is over the limit, and then at a character
// boundary. It returns the piece asked for and how many there are.
func pieceOf(s string, piece, limit int) (text string, of int) {
	var pieces []string
	for len(s) > 0 {
		if len(s) <= limit {
			pieces = append(pieces, s)
			break
		}
		cut := strings.LastIndex(s[:limit], "\n") + 1
		if cut <= 0 {
			cut = limit
			for cut > 0 && !utf8.RuneStart(s[cut]) {
				cut--
			}
		}
		pieces = append(pieces, s[:cut])
		s = s[cut:]
	}
	if len(pieces) == 0 {
		pieces = []string{""}
	}
	if piece > len(pieces) {
		return "", len(pieces)
	}
	return pieces[piece-1], len(pieces)
}

// pageAnswer returns the piece of a long answer the lookup asked for, with a
// header saying where it is and a footer saying how to go on. An answer that
// fits is returned unchanged for piece 1, so nothing short is ever wrapped.
func pageAnswer(text string, piece int) (string, error) {
	body, of := pieceOf(text, piece, maxAnswerBytes-pieceRoom)
	if piece > of {
		return "", badRequestf("there is no piece %d: this answer has %d (%s)", piece, of, plural(of, "piece", "pieces"))
	}
	if of == 1 {
		return text, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "(Piece %d of %d of this answer, which is too long for one.)\n\n", piece, of)
	b.WriteString(body)
	if piece < of {
		fmt.Fprintf(&b, "\n(Ask again with `piece: %d` for the next piece.)\n", piece+1)
	}
	return b.String(), nil
}
