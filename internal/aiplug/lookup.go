package aiplug

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxAnswerLines caps the lines of one list in a lookup's answer; WriteLines
// says how many more there were.
const MaxAnswerLines = 300

// Lookup answers one `what:` an AI may ask, both in a pasted lookup block and
// as a tool on a connected AI app. The engine applies the privacy mode, the
// per-answer and per-paste size caps and the error wording around it.
type Lookup interface {
	LookupInfo() LookupInfo
	// Answer reads as the Actor. Text is markdown that starts with its own
	// `## ` heading; an error refuses the whole answer.
	Answer(ctx context.Context, campaignID string, a Actor, r Record) (chip, text string, err error)
}

// Dated is a lookup that knows the world's date, which the prompt's opening
// line gives beside the real one. WorldToday reads as the Actor; "" when
// there is none to give.
type Dated interface {
	WorldToday(ctx context.Context, campaignID string, a Actor) string
}

// LookupInfo describes a lookup to the prompt and to connected AI apps.
type LookupInfo struct {
	What    string   // the `what:` value, e.g. "game-nights"
	Aliases []string // other spellings answered the same
	Keys    string   // the keys it reads, for the prompt ("none" for none)
	Brief   string   // what it answers, for the prompt
	// Tool is how a connected AI app calls it; a zero Name lists no tool.
	Tool Tool
}

// Tool is a lookup as an MCP tool. Every lookup tool is read-only.
type Tool struct {
	Name, Title string
	Did         string // the "What it did" line, past tense
	Desc        string
	PlayerDesc  string // replaces Desc on a player's key; "" keeps Desc
	// Owner hides the tool from a player's key: Director preparation.
	Owner bool
	Args  []Arg
}

// Arg is one input a lookup tool takes. Key is the record field it fills,
// or "name" for the record's own name.
type Arg struct {
	Key, Desc string
	Required  bool
}

// OneLine flattens whitespace and shortens s to about n bytes.
func OneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := strings.LastIndex(s[:n], " ")
	if cut < n/2 {
		cut = n
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
	}
	return s[:cut] + "…"
}

// WriteLines writes lines up to MaxAnswerLines, then how many were left out.
func WriteLines(b *strings.Builder, lines []string) {
	for i, l := range lines {
		if i >= MaxAnswerLines {
			fmt.Fprintf(b, "…and %d more.\n", len(lines)-i)
			return
		}
		b.WriteString(l + "\n")
	}
}

// Pieced is a lookup that reads its record's `piece` itself, cutting its long
// lists with WriteLinesPiece. The engine then leaves its own byte paging off
// for that lookup, so one answer is never cut to a piece twice.
type Pieced interface {
	Pieced()
}

// PieceOf is the piece a record asks for: 1 when it names none (the engine has
// already refused one it can't read).
func PieceOf(r Record) int {
	n, err := strconv.Atoi(r.Str("piece"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// WriteLinesPiece writes the piece of lines asked for, MaxAnswerLines of them,
// and says how to read the next. A list that fits is written as WriteLines
// writes it.
func WriteLinesPiece(b *strings.Builder, lines []string, piece int) {
	if len(lines) <= MaxAnswerLines && piece <= 1 {
		WriteLines(b, lines)
		return
	}
	pieces := (len(lines) + MaxAnswerLines - 1) / MaxAnswerLines
	start, end := (piece-1)*MaxAnswerLines, min(piece*MaxAnswerLines, len(lines))
	if start >= len(lines) {
		fmt.Fprintf(b, "There is no piece %d: this list has %d.\n", piece, pieces)
		return
	}
	fmt.Fprintf(b, "Lines %d to %d of %d (piece %d of %d).\n\n", start+1, end, len(lines), piece, pieces)
	for _, l := range lines[start:end] {
		b.WriteString(l + "\n")
	}
	if end < len(lines) {
		fmt.Fprintf(b, "…%d more; ask again with `piece: %d` for the next.\n", len(lines)-end, piece+1)
	}
}
