package dmscreen

import (
	"encoding/json"
	"html"
	"strings"
	"unicode/utf8"
)

// maxNoteChars bounds one save; the screen's note is a scratchpad, not a
// manuscript, and the full editor is one click away.
const maxNoteChars = 20000

// maxNightTitleChars keeps "<night>: DM notes" inside the notes title limit.
const maxNightTitleChars = 150

// noteNames picks the label the Notes tab shows and the title the stored note
// carries. The note is one per campaign and follows the next game night: when
// the night changes, the next save retitles it. With no night it falls back to
// a name that doesn't pretend to belong to one.
func noteNames(n *NightView) (label, title string) {
	if n == nil || strings.TrimSpace(n.Name) == "" {
		return "DM Screen notes", "DM Screen notes"
	}
	name := strings.TrimSpace(n.Name)
	if utf8.RuneCountInString(name) > maxNightTitleChars {
		name = string([]rune(name)[:maxNightTitleChars])
	}
	return "Kept with " + name, name + ": DM notes"
}

// proseNode is the slice of a ProseMirror node this package reads.
type proseNode struct {
	Type    string      `json:"type"`
	Text    string      `json:"text"`
	Content []proseNode `json:"content"`
}

// blockTypes are the nodes that end a line of plain text.
var blockTypes = map[string]bool{
	"paragraph": true, "heading": true, "blockquote": true, "codeBlock": true,
	"listItem": true, "taskItem": true, "horizontalRule": true,
}

// plainFromProse flattens a note's ProseMirror JSON to plain text, one line
// per block. Rich content written in the full editor (lists, headings, marks)
// shows as its text; the formatting is lost on the next save from the screen,
// which writes plain paragraphs. Unparseable or empty input gives "".
func plainFromProse(entry string) string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return ""
	}
	var doc proseNode
	if err := json.Unmarshal([]byte(entry), &doc); err != nil {
		return ""
	}
	var lines []string
	var cur strings.Builder
	var walk func(n proseNode)
	flush := func() {
		lines = append(lines, cur.String())
		cur.Reset()
	}
	walk = func(n proseNode) {
		switch {
		case n.Type == "text":
			cur.WriteString(n.Text)
		case n.Type == "hardBreak":
			cur.WriteString("\n")
		}
		for _, c := range n.Content {
			walk(c)
		}
		// A list item holds a paragraph; ending the line there too would
		// double it, so only leaf-level blocks flush.
		if blockTypes[n.Type] && !hasBlockChild(n) {
			flush()
		}
	}
	walk(doc)
	if cur.Len() > 0 {
		flush()
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func hasBlockChild(n proseNode) bool {
	for _, c := range n.Content {
		if blockTypes[c.Type] || hasBlockChild(c) {
			return true
		}
	}
	return false
}

// proseFromPlain turns text into a ProseMirror document, one paragraph per
// line, and the matching HTML the notes list and search read.
func proseFromPlain(text string) (entry, entryHTML string) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(text, "\n")
	paras := make([]map[string]any, 0, len(lines))
	var sb strings.Builder
	for _, l := range lines {
		p := map[string]any{"type": "paragraph"}
		if l != "" {
			p["content"] = []map[string]any{{"type": "text", "text": l}}
		}
		paras = append(paras, p)
		sb.WriteString("<p>" + html.EscapeString(l) + "</p>")
	}
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": paras})
	return string(b), sb.String()
}
