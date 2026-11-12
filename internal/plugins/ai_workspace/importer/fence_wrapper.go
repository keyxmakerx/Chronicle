package importer

import (
	"regexp"
	"strings"
)

// fenceLineRe matches a Markdown code-fence line and captures its backticks.
var fenceLineRe = regexp.MustCompile("^ {0,3}(`{3,})[^`]*$")

// unwrapFences removes the code fence a chat AI puts around a block so it can
// be copied, and the chat that follows it. Without this the closing fence and
// a line like "Let me know!" would be saved as the last words of the page.
//
// A fence counts as a wrapper only when the next line that isn't blank is a
// front-matter `---` line, so a code block inside a page body is never
// touched. It closes at the first fence line with at least as many backticks,
// as Markdown does. An unclosed wrapper is left alone.
func unwrapFences(input string) string {
	if !strings.Contains(input, "```") {
		return input
	}
	lines := strings.Split(input, "\n")
	var out []string
	wrapped := false
	for i := 0; i < len(lines); i++ {
		m := fenceLineRe.FindStringSubmatch(strings.TrimRight(lines[i], "\r"))
		if m == nil || !nextLineOpensFrontMatter(lines, i+1) {
			out = append(out, lines[i])
			continue
		}
		closer := -1
		for j := i + 1; j < len(lines); j++ {
			if c := fenceLineRe.FindStringSubmatch(strings.TrimRight(lines[j], "\r")); c != nil &&
				len(c[1]) >= len(m[1]) && strings.TrimSpace(strings.TrimLeft(lines[j], " ")) == c[1] {
				closer = j
				break
			}
		}
		if closer < 0 {
			out = append(out, lines[i])
			continue
		}
		out = append(out, lines[i+1:closer]...)
		wrapped = true
		i = closer
		// Chat between this wrapper and the next block is not page text, but
		// anything holding a `---` line might be, so it stays.
		next := closer + 1
		for next < len(lines) && !fenceLineRe.MatchString(strings.TrimRight(lines[next], "\r")) {
			next++
		}
		if span := strings.Join(lines[closer+1:next], "\n"); !fmOpenRe.MatchString(span) {
			i = next - 1
		}
	}
	if !wrapped {
		return input
	}
	return strings.Join(out, "\n")
}

// nextLineOpensFrontMatter reports whether the first non-blank line from
// index i is a `---` fence.
func nextLineOpensFrontMatter(lines []string, i int) bool {
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		return fmOpenRe.MatchString(strings.TrimRight(lines[i], "\r"))
	}
	return false
}
