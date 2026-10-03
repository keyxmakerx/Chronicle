package entities

import (
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// DiffPart is one run of a word-level diff: unchanged, added or removed text.
type DiffPart struct {
	Op   byte // ' ' same, '+' added, '-' removed
	Text string
}

// maxDiffCells bounds the LCS table (tokens × tokens). Past it the diff
// compares whole lines instead of words, which keeps a very long page from
// costing seconds of CPU.
const maxDiffCells = 4_000_000

// lineBreakTags end a line when a page's HTML is flattened to text.
var lineBreakTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "blockquote": true, "pre": true, "hr": true,
}

// HTMLToPlainLines flattens page HTML to text, one paragraph, heading or list
// item per line, so history can say "+2 lines" and show changed words.
func HTMLToPlainLines(src string) string {
	z := html.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	newline := func() {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
	}
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.TrimSpace(b.String())
		case html.TextToken:
			// Collapse whitespace as a browser would, keeping one space at
			// either edge so "Two <b>bold</b>" stays two words.
			raw := string(z.Text())
			words := strings.Join(strings.Fields(raw), " ")
			cur := b.String()
			if words != "" && unicode.IsSpace(rune(raw[0])) && cur != "" &&
				!strings.HasSuffix(cur, " ") && !strings.HasSuffix(cur, "\n") {
				b.WriteByte(' ')
			}
			b.WriteString(words)
			if words != "" && unicode.IsSpace(rune(raw[len(raw)-1])) {
				b.WriteByte(' ')
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			if lineBreakTags[string(name)] {
				newline()
			}
		}
	}
}

// tokenize splits text into words, runs of spaces and newlines, keeping
// every character so the parts join back to the original.
func tokenize(s string) []string {
	var out []string
	start := 0
	kind := func(r rune) int {
		switch {
		case r == '\n':
			return 2
		case unicode.IsSpace(r):
			return 1
		}
		return 0
	}
	runes := []rune(s)
	for i := 1; i <= len(runes); i++ {
		if i == len(runes) || kind(runes[i]) != kind(runes[start]) || kind(runes[i]) == 2 {
			out = append(out, string(runes[start:i]))
			start = i
		}
	}
	return out
}

// lcsOps diffs two token lists and returns one op per token: ' ' kept,
// '-' only in a, '+' only in b, in reading order.
func lcsOps(a, b []string) []DiffPart {
	n, m := len(a), len(b)
	// dp[i*(m+1)+j] = LCS length of a[i:], b[j:].
	dp := make([]int32, (n+1)*(m+1))
	at := func(i, j int) int32 { return dp[i*(m+1)+j] }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				dp[i*(m+1)+j] = at(i+1, j+1) + 1
			case at(i+1, j) >= at(i, j+1):
				dp[i*(m+1)+j] = at(i+1, j)
			default:
				dp[i*(m+1)+j] = at(i, j+1)
			}
		}
	}
	ops := make([]DiffPart, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, DiffPart{' ', a[i]})
			i++
			j++
		case at(i+1, j) >= at(i, j+1):
			ops = append(ops, DiffPart{'-', a[i]})
			i++
		default:
			ops = append(ops, DiffPart{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, DiffPart{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, DiffPart{'+', b[j]})
	}
	return ops
}

// lcsDiff is lcsOps with neighbouring tokens of the same op joined.
func lcsDiff(a, b []string) []DiffPart {
	var parts []DiffPart
	for _, op := range lcsOps(a, b) {
		if l := len(parts); l > 0 && parts[l-1].Op == op.Op {
			parts[l-1].Text += op.Text
			continue
		}
		parts = append(parts, op)
	}
	return parts
}

// splitLines splits text into lines, each keeping its newline.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.SplitAfter(s, "\n")
}

// DiffWords returns a word-level diff from older to newer text, or a
// line-level one when the texts are too long for words.
func DiffWords(older, newer string) []DiffPart {
	a, b := tokenize(older), tokenize(newer)
	if len(a)*len(b) > maxDiffCells {
		a, b = splitLines(older), splitLines(newer)
		if len(a)*len(b) > maxDiffCells {
			return []DiffPart{{Op: '-', Text: older}, {Op: '+', Text: newer}}
		}
	}
	return lcsDiff(a, b)
}

// CountLineChanges returns how many lines newer adds and removes compared
// with older; a changed line counts as one of each.
func CountLineChanges(older, newer string) (added, removed int) {
	a := strings.Split(strings.TrimSpace(older), "\n")
	b := strings.Split(strings.TrimSpace(newer), "\n")
	if older == "" {
		a = nil
	}
	if newer == "" {
		b = nil
	}
	if len(a)*len(b) > maxDiffCells {
		return len(b), len(a)
	}
	for _, op := range lcsOps(a, b) {
		switch op.Op {
		case '+':
			added++
		case '-':
			removed++
		}
	}
	return added, removed
}
