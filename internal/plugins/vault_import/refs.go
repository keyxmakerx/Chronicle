package vault_import

import (
	"net/url"
	"regexp"
	"strings"
)

// RefKind says which syntax a reference was written in.
type RefKind int

const (
	// RefWiki is [[Note]], [[Note|alias]] or [[Note#Heading]].
	RefWiki RefKind = iota
	// RefEmbed is ![[Note]] or ![[picture.png|300]].
	RefEmbed
	// RefLink is [text](path).
	RefLink
	// RefImage is ![alt](path).
	RefImage
)

// Ref is one link or picture found in a note's text. Start and End are byte
// offsets into the text that was scanned.
type Ref struct {
	Kind       RefKind
	Start, End int
	// Target is the note or file named, without #heading or ^block. For
	// Markdown links it is the URL-decoded path. Empty for a link to a heading
	// in the same note.
	Target   string
	Fragment string
	// Text is the alias, the link text or the picture's alt text.
	Text string
	// Hint is an embed's size ("300" or "300x200"); it is not alt text.
	Hint  string
	Title string
	// Remote is true for a Markdown reference that is not a path inside the
	// vault (a web address, mail link or bare #anchor). Such references are
	// left to the Markdown renderer, except that remote pictures become links.
	Remote bool
}

// fenceOpenRe finds the start of a fenced code block.
var fenceOpenRe = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")

// proseSpan is a stretch of one line that is neither inside a fenced code
// block nor an inline code span.
type proseSpan struct{ s, e int }

// proseSpans walks body and returns the stretches references may be read from.
// A [[link]] written inside code is code, and must come through untouched.
func proseSpans(body string) []proseSpan {
	var out []proseSpan
	var fence string // the opening marker while inside a fenced block
	pos := 0
	for pos <= len(body) {
		nl := strings.IndexByte(body[pos:], '\n')
		lineEnd := len(body)
		next := len(body) + 1
		if nl >= 0 {
			lineEnd = pos + nl
			next = lineEnd + 1
		}
		line := body[pos:lineEnd]
		switch {
		case fence != "":
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
		default:
			if m := fenceOpenRe.FindStringSubmatch(line); m != nil {
				fence = m[1]
			} else {
				out = append(out, codeFreeSpans(body, pos, lineEnd)...)
			}
		}
		pos = next
	}
	return out
}

// codeFreeSpans splits one line around its inline code spans. A span opens
// with a run of backticks and closes with the next run of the same length.
func codeFreeSpans(body string, s, e int) []proseSpan {
	var out []proseSpan
	start := s
	i := s
	// A run length with no partner later on the line has none further on
	// either; remembering that keeps a line of ever-longer runs linear.
	var unmatched map[int]bool
	for i < e {
		if body[i] != '`' {
			i++
			continue
		}
		j := i
		for j < e && body[j] == '`' {
			j++
		}
		closeAt := -1
		if !unmatched[j-i] {
			closeAt = matchingRun(body, j, e, j-i)
		}
		if closeAt < 0 {
			if unmatched == nil {
				unmatched = map[int]bool{}
			}
			unmatched[j-i] = true
			i = j
			continue
		}
		if i > start {
			out = append(out, proseSpan{start, i})
		}
		i = closeAt + (j - i)
		start = i
	}
	if start < e {
		out = append(out, proseSpan{start, e})
	}
	return out
}

// matchingRun finds the next run of exactly n backticks in body[from:e].
func matchingRun(body string, from, e, n int) int {
	i := from
	for i < e {
		if body[i] != '`' {
			i++
			continue
		}
		j := i
		for j < e && body[j] == '`' {
			j++
		}
		if j-i == n {
			return i
		}
		i = j
	}
	return -1
}

// ScanRefs returns every reference in body outside code, in order.
//
// Every step is linear in the length of body: a hostile note made of brackets
// must cost no more than a real one. Bracket partners and the next "]]" are
// found once per line in a single pass and looked up in O(1) afterwards.
func ScanRefs(body string) []Ref {
	var refs []Ref
	var sc scratch
	for _, sp := range proseSpans(body) {
		scanSpan(body, sp.s, sp.e, &refs, &sc)
	}
	return refs
}

// maxDestLook bounds how far past a link's "]" the destination and title are
// read, so a stream of "[a](<" cannot make every bracket rescan the line.
const maxDestLook = 1024

// maxLinkText is the longest link label read as a link.
const maxLinkText = 4096

// scratch holds the per-line lookup tables, reused across lines.
type scratch struct {
	match   []int32 // for a "[" at offset i: offset of its "]", or -1
	nextEnd []int32 // offset of the first "]" at or after i, or -1
	nextBeg []int32 // offset of the first "[" at or after i, or -1
	stack   []int32
}

func grow(b []int32, n int) []int32 {
	if cap(b) < n {
		return make([]int32, n)
	}
	return b[:n]
}

// index fills the tables for body[s:e].
func (sc *scratch) index(body string, s, e int) {
	n := e - s
	sc.match = grow(sc.match, n+1)
	sc.nextEnd = grow(sc.nextEnd, n+1)
	sc.nextBeg = grow(sc.nextBeg, n+1)
	sc.stack = sc.stack[:0]
	for i := range sc.match {
		sc.match[i] = -1
	}
	for i := 0; i < n; i++ {
		switch body[s+i] {
		case '\\':
			i++
		case '[':
			sc.stack = append(sc.stack, int32(i))
		case ']':
			if k := len(sc.stack); k > 0 {
				sc.match[sc.stack[k-1]] = int32(i)
				sc.stack = sc.stack[:k-1]
			}
		}
	}
	sc.nextEnd[n], sc.nextBeg[n] = -1, -1
	for i := n - 1; i >= 0; i-- {
		sc.nextEnd[i], sc.nextBeg[i] = sc.nextEnd[i+1], sc.nextBeg[i+1]
		switch body[s+i] {
		case ']':
			sc.nextEnd[i] = int32(i)
		case '[':
			sc.nextBeg[i] = int32(i)
		}
	}
}

func scanSpan(body string, s, e int, refs *[]Ref, sc *scratch) {
	sc.index(body, s, e)
	i := s
	for i < e {
		c := body[i]
		if c == '\\' {
			i += 2
			continue
		}
		if c != '[' && c != '!' {
			i++
			continue
		}
		if strings.HasPrefix(body[i:e], "![[") || strings.HasPrefix(body[i:e], "[[") {
			embed := body[i] == '!'
			open := i + 2
			if embed {
				open = i + 3
			}
			// The inner text must hold no bracket: the first "]" after the
			// opening must start the closing "]]", with no "[" before it.
			if open <= e {
				end := int(sc.nextEnd[open-s])
				beg := int(sc.nextBeg[open-s])
				if end >= 0 && end+1 < e-s && body[s+end+1] == ']' && (beg < 0 || beg > end) {
					r := parseWiki(body[open:s+end], embed)
					r.Start, r.End = i, s+end+2
					// [[]] names nothing; leave it as the text it is.
					if r.Target != "" || r.Fragment != "" {
						*refs = append(*refs, r)
					}
					i = r.End
					continue
				}
			}
			i++
			continue
		}
		image := c == '!'
		br := i
		if image {
			if i+1 >= e || body[i+1] != '[' {
				i++
				continue
			}
			br = i + 1
		}
		if cl := int(sc.match[br-s]); cl >= 0 {
			if r, ok := parseMDRef(body, i, br, s+cl, e, image); ok {
				*refs = append(*refs, r)
				i = r.End
				continue
			}
		}
		i++
	}
}

// parseWiki reads the inside of [[...]]: target, alias and heading.
func parseWiki(inner string, embed bool) Ref {
	// A table cell writes the alias bar as \| to keep it from splitting the cell.
	inner = strings.ReplaceAll(inner, `\|`, "|")
	r := Ref{Kind: RefWiki}
	if embed {
		r.Kind = RefEmbed
	}
	target, alias, hasAlias := strings.Cut(inner, "|")
	if name, frag, ok := strings.Cut(target, "#"); ok {
		target = name
		r.Fragment = strings.TrimSpace(frag)
	}
	r.Target = strings.TrimSpace(target)
	if hasAlias {
		alias = strings.TrimSpace(alias)
		if embed && sizeHintRe.MatchString(alias) {
			r.Hint = alias
		} else {
			r.Text = alias
		}
	}
	return r
}

var sizeHintRe = regexp.MustCompile(`^\d{1,4}(x\d{1,4})?$`)

var schemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// parseMDRef reads [text](dest "title") or ![alt](dest). open is the index of
// the "[" and start the index of the whole reference (the "!" for a picture).
func parseMDRef(body string, start, open, closeIdx, e int, image bool) (Ref, bool) {
	// closeIdx is the "]" matching open, found once per line by scratch.index.
	// Text longer than maxLinkText is not a link label; the cap keeps nested
	// brackets from rescanning the same text for each level.
	if closeIdx+1 >= e || body[closeIdx+1] != '(' || closeIdx-open > maxLinkText {
		return Ref{}, false
	}
	text := body[open+1 : closeIdx]
	// A link whose text holds a picture is left alone so the picture, scanned
	// next, is the reference that gets rewritten.
	if !image && strings.Contains(text, "![") {
		return Ref{}, false
	}
	lookEnd := e
	if closeIdx+1+maxDestLook < lookEnd {
		lookEnd = closeIdx + 1 + maxDestLook
	}
	dest, title, end, ok := parseDest(body, closeIdx+1, lookEnd)
	if !ok {
		return Ref{}, false
	}
	r := Ref{Kind: RefLink, Start: start, End: end, Text: text, Title: title}
	if image {
		r.Kind = RefImage
	}
	r.Remote = true
	switch {
	case dest == "", strings.HasPrefix(dest, "#"), strings.HasPrefix(dest, "//"), schemeRe.MatchString(dest):
	default:
		path, frag, _ := strings.Cut(dest, "#")
		if dec, err := url.PathUnescape(path); err == nil {
			path = dec
		}
		path, _, _ = strings.Cut(path, "?")
		r.Remote = false
		r.Target = path
		r.Fragment = frag
	}
	if r.Remote {
		r.Target = dest
	}
	return r, true
}

// parseDest reads "(dest "title")" starting at the "(" and returns the index
// after the closing ")".
func parseDest(body string, open, e int) (dest, title string, end int, ok bool) {
	i := open + 1
	for i < e && (body[i] == ' ' || body[i] == '\t') {
		i++
	}
	if i < e && body[i] == '<' {
		j := strings.IndexByte(body[i+1:e], '>')
		if j < 0 {
			return "", "", 0, false
		}
		dest = body[i+1 : i+1+j]
		i = i + 1 + j + 1
	} else {
		depth := 0
		j := i
		for j < e {
			c := body[j]
			if c == '\\' {
				j += 2
				continue
			}
			if c == ' ' || c == '\t' {
				break
			}
			if c == '(' {
				depth++
			}
			if c == ')' {
				if depth == 0 {
					break
				}
				depth--
			}
			j++
		}
		if j > e {
			j = e
		}
		dest = body[i:j]
		i = j
	}
	for i < e && (body[i] == ' ' || body[i] == '\t') {
		i++
	}
	if i < e && (body[i] == '"' || body[i] == '\'' || body[i] == '(') {
		closer := body[i]
		if closer == '(' {
			closer = ')'
		}
		j := strings.IndexByte(body[i+1:e], closer)
		if j < 0 {
			return "", "", 0, false
		}
		title = body[i+1 : i+1+j]
		i = i + 1 + j + 1
		for i < e && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
	}
	if i >= e || body[i] != ')' {
		return "", "", 0, false
	}
	return dest, title, i + 1, true
}

// Rewrite replaces each reference with what f returns for it; a reference f
// declines (ok=false) is left exactly as written. refs must be in order and
// not overlap, which ScanRefs guarantees.
func Rewrite(body string, refs []Ref, f func(Ref) (string, bool)) string {
	var b strings.Builder
	last := 0
	for _, r := range refs {
		rep, ok := f(r)
		if !ok {
			continue
		}
		b.WriteString(body[last:r.Start])
		b.WriteString(rep)
		last = r.End
	}
	b.WriteString(body[last:])
	return b.String()
}

// escapeMD makes text safe to place inside Markdown as literal words or as
// link text: every ASCII punctuation mark the renderer could read as syntax is
// backslash-escaped.
func escapeMD(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\r' {
			b.WriteByte(' ')
			continue
		}
		if strings.ContainsRune("\\`*_{}[]<>()#+-.!|~&\"'", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

var (
	blockIDRe   = regexp.MustCompile(`[ \t]+\^[A-Za-z0-9-]+[ \t]*$`)
	calloutRe   = regexp.MustCompile(`^((?:[ \t]*>)+[ \t]*)\[!([A-Za-z0-9_-]+)\][+-]?[ \t]*(.*)$`)
	soloBlockID = regexp.MustCompile(`^\^[A-Za-z0-9-]+[ \t]*$`)
)

// Prepare rewrites the Obsidian-only syntax that has no Markdown meaning, so
// it does not show up as stray punctuation: %%comments%% vanish, ^block-ids
// vanish, and a "> [!note] Title" callout becomes a quote that opens with its
// title in bold. Code is left exactly as written.
func Prepare(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	var out strings.Builder
	var fence string
	inComment := false
	lines := strings.Split(body, "\n")
	for n, line := range lines {
		if fence != "" {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
			out.WriteString(line)
		} else if m := fenceOpenRe.FindStringSubmatch(line); m != nil {
			fence = m[1]
			out.WriteString(line)
		} else {
			line, inComment = stripComments(line, inComment)
			if soloBlockID.MatchString(line) {
				line = ""
			} else {
				line = blockIDRe.ReplaceAllString(line, "")
			}
			if m := calloutRe.FindStringSubmatch(line); m != nil {
				title := strings.TrimSpace(m[3])
				if title == "" {
					title = strings.ToUpper(m[2][:1]) + strings.ToLower(m[2][1:])
				}
				line = m[1] + "**" + escapeMD(title) + "**"
			}
			out.WriteString(line)
		}
		if n < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// stripComments removes %%...%% spans from a line, carrying an open comment
// across lines.
func stripComments(line string, inComment bool) (string, bool) {
	var b strings.Builder
	for {
		if inComment {
			i := strings.Index(line, "%%")
			if i < 0 {
				return b.String(), true
			}
			line = line[i+2:]
			inComment = false
			continue
		}
		i := strings.Index(line, "%%")
		if i < 0 {
			b.WriteString(line)
			return b.String(), false
		}
		b.WriteString(line[:i])
		line = line[i+2:]
		inComment = true
	}
}
