package dmscreen

import (
	"crypto/sha256"
	"encoding/hex"
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
	Type    string            `json:"type"`
	Text    string            `json:"text"`
	Attrs   map[string]any    `json:"attrs"`
	Marks   []json.RawMessage `json:"marks"`
	Content []proseNode       `json:"content"`
}

// isPlainProse reports whether entry is empty or a document of paragraphs
// holding only unmarked text: exactly what proseFromPlain writes. Anything
// else (headings, lists, marks, images, mentions, hard breaks, an unreadable
// body) is rich, and the screen must not save over it.
func isPlainProse(entry string) bool {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return true
	}
	var doc proseNode
	if err := json.Unmarshal([]byte(entry), &doc); err != nil || doc.Type != "doc" || len(doc.Marks) > 0 {
		return false
	}
	for _, p := range doc.Content {
		if p.Type != "paragraph" || len(p.Marks) > 0 || len(p.Attrs) > 0 {
			return false
		}
		for _, t := range p.Content {
			if t.Type != "text" || len(t.Marks) > 0 || len(t.Attrs) > 0 || len(t.Content) > 0 {
				return false
			}
		}
	}
	return true
}

// BodyVersion names a stored body for the lost-update check.
func BodyVersion(entry string) string {
	sum := sha256.Sum256([]byte(entry))
	return hex.EncodeToString(sum[:8])
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
		switch n.Type {
		case "text":
			cur.WriteString(n.Text)
		case "hardBreak":
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
