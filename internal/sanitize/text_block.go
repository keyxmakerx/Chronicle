package sanitize

import (
	"html"
	"regexp"
	"strings"
)

// looksLikeHTML matches a tag opening or closing; plain prose with a stray
// "<" (a < b) does not match.
var looksLikeHTML = regexp.MustCompile(`<[a-zA-Z/!]`)

// paragraphBreak is a blank line, allowing spaces on it.
var paragraphBreak = regexp.MustCompile(`\n[ \t]*\n`)

// TextBlock renders a Text Block's content. Owners type plain text into it,
// so text with no markup becomes escaped paragraphs (a blank line between
// them, single newlines kept as line breaks) instead of one run-on line.
// Content that already holds markup, written that way or seeded by
// Chronicle, goes through HTML like any other stored rich text.
func TextBlock(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	if looksLikeHTML.MatchString(input) {
		return HTML(input)
	}
	input = strings.ReplaceAll(input, "\r\n", "\n")
	var b strings.Builder
	for _, para := range paragraphBreak.Split(input, -1) {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(html.EscapeString(para), "\n", "<br>"))
		b.WriteString("</p>")
	}
	return b.String()
}
