package entities

// era_diff.go: what an era version changes in a page's text, block by
// block, for the Eras panel's highlights. Blocks (paragraphs, headings,
// lists...) are matched by their text; a block that was edited keeps its
// formatting and has the words it added marked.

import (
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Era diff block kinds.
const (
	EraDiffSame    = "same"
	EraDiffChanged = "changed"
	EraDiffAdded   = "added"
	EraDiffRemoved = "removed"
)

// EraDiffBlock is one block of the version's text. HTML is the version's
// block (changed words in <mark class="era-w">); for a removed block it is
// the usual page's.
type EraDiffBlock struct {
	Kind string
	HTML string
}

// eraSimilar is how alike two blocks' words must be for the second to count
// as an edit of the first rather than a removal and an addition.
const eraSimilar = 0.35

// maxEraDiffBlocks bounds the block match (blocks × blocks); past it every
// block is compared in place, which a very long page can afford.
const maxEraDiffBlocks = 250_000

type eraBlock struct {
	nodes []*html.Node
	text  string
}

// EraDiff compares a page's usual text with an era version's, both
// sanitized HTML.
func EraDiff(usualHTML, versionHTML string) []EraDiffBlock {
	a, b := eraBlocks(usualHTML), eraBlocks(versionHTML)
	var out []EraDiffBlock
	var runA, runB []eraBlock
	flush := func() {
		out = append(out, pairRun(runA, runB)...)
		runA, runB = nil, nil
	}
	for _, op := range matchBlocks(a, b) {
		switch op.kind {
		case ' ':
			flush()
			out = append(out, EraDiffBlock{Kind: EraDiffSame, HTML: renderBlock(b[op.j])})
		case '-':
			runA = append(runA, a[op.i])
		case '+':
			runB = append(runB, b[op.j])
		}
	}
	flush()
	return out
}

// maxEraRunPairs bounds the pairing of one run of unmatched blocks; past it
// blocks are paired in place.
const maxEraRunPairs = 400

// pairRun lays out a run of usual blocks the version dropped (a) against a
// run of blocks it brought in (b). Blocks alike enough are paired as edits,
// in order, choosing the pairing with the most likeness, so a paragraph
// removed just before an edited one does not hide the edit.
func pairRun(a, b []eraBlock) []EraDiffBlock {
	n, m := len(a), len(b)
	sim := func(i, j int) float64 {
		if n*m > maxEraRunPairs && i != j {
			return 0
		}
		s := eraSimilarity(a[i].text, b[j].text)
		if s < eraSimilar {
			return 0
		}
		return s
	}
	// best[i][j] is the most likeness pairing a[i:] with b[j:] can reach.
	best := make([][]float64, n+1)
	for i := range best {
		best[i] = make([]float64, m+1)
	}
	sims := make([][]float64, n)
	for i := range sims {
		sims[i] = make([]float64, m)
		for j := range sims[i] {
			sims[i][j] = sim(i, j)
		}
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			v := max(best[i+1][j], best[i][j+1])
			if sims[i][j] > 0 {
				v = max(v, best[i+1][j+1]+sims[i][j])
			}
			best[i][j] = v
		}
	}
	var out []EraDiffBlock
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case sims[i][j] > 0 && best[i][j] == best[i+1][j+1]+sims[i][j]:
			out = append(out, EraDiffBlock{Kind: EraDiffChanged, HTML: markAdded(a[i].text, b[j])})
			i++
			j++
		case best[i][j] == best[i+1][j]:
			out = append(out, EraDiffBlock{Kind: EraDiffRemoved, HTML: renderBlock(a[i])})
			i++
		default:
			out = append(out, EraDiffBlock{Kind: EraDiffAdded, HTML: renderBlock(b[j])})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, EraDiffBlock{Kind: EraDiffRemoved, HTML: renderBlock(a[i])})
	}
	for ; j < m; j++ {
		out = append(out, EraDiffBlock{Kind: EraDiffAdded, HTML: renderBlock(b[j])})
	}
	return out
}

// EraDiffCount is how many blocks a diff changes, adds or removes.
func EraDiffCount(d []EraDiffBlock) int {
	n := 0
	for _, b := range d {
		if b.Kind != EraDiffSame {
			n++
		}
	}
	return n
}

type blockOp struct {
	kind byte
	i, j int
}

// matchBlocks is a longest-common-subsequence match of blocks by text.
func matchBlocks(a, b []eraBlock) []blockOp {
	n, m := len(a), len(b)
	var ops []blockOp
	if n*m > maxEraDiffBlocks {
		for i := 0; i < max(n, m); i++ {
			switch {
			case i < n && i < m && a[i].text == b[i].text:
				ops = append(ops, blockOp{' ', i, i})
			default:
				if i < n {
					ops = append(ops, blockOp{'-', i, 0})
				}
				if i < m {
					ops = append(ops, blockOp{'+', 0, i})
				}
			}
		}
		return ops
	}
	dp := make([]int32, (n+1)*(m+1))
	at := func(i, j int) int32 { return dp[i*(m+1)+j] }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i].text == b[j].text:
				dp[i*(m+1)+j] = at(i+1, j+1) + 1
			case at(i+1, j) >= at(i, j+1):
				dp[i*(m+1)+j] = at(i+1, j)
			default:
				dp[i*(m+1)+j] = at(i, j+1)
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i].text == b[j].text:
			ops = append(ops, blockOp{' ', i, j})
			i++
			j++
		case at(i+1, j) >= at(i, j+1):
			ops = append(ops, blockOp{'-', i, 0})
			i++
		default:
			ops = append(ops, blockOp{'+', 0, j})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, blockOp{'-', i, 0})
	}
	for ; j < m; j++ {
		ops = append(ops, blockOp{'+', 0, j})
	}
	return ops
}

// eraBlockTags start a block of their own; anything else at the top level
// (text, inline tags) joins its neighbours in one block.
var eraBlockTags = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true,
	atom.H5: true, atom.H6: true, atom.Ul: true, atom.Ol: true, atom.Blockquote: true,
	atom.Pre: true, atom.Table: true, atom.Hr: true, atom.Figure: true, atom.Img: true,
	atom.Section: true, atom.Aside: true, atom.Details: true,
}

func eraBlocks(src string) []eraBlock {
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(src), ctx)
	if err != nil {
		return nil
	}
	var out []eraBlock
	var inline []*html.Node
	end := func() {
		if len(inline) > 0 {
			out = append(out, eraBlock{nodes: inline, text: blockText(inline)})
			inline = nil
		}
	}
	for _, n := range nodes {
		if n.Type == html.TextNode && strings.TrimSpace(n.Data) == "" && len(inline) == 0 {
			continue
		}
		if n.Type == html.ElementNode && eraBlockTags[n.DataAtom] {
			end()
			out = append(out, eraBlock{nodes: []*html.Node{n}, text: blockText([]*html.Node{n})})
			continue
		}
		if n.Type == html.CommentNode {
			continue
		}
		inline = append(inline, n)
	}
	end()
	return out
}

// blockText is a block's text with runs of space collapsed, so a block
// re-wrapped by the editor still matches. An image or rule, which has no
// text, is matched by its source.
func blockText(nodes []*html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
			b.WriteByte(' ')
		case html.ElementNode:
			if n.DataAtom == atom.Img {
				for _, a := range n.Attr {
					if a.Key == "src" {
						b.WriteString("[img " + a.Val + "] ")
					}
				}
			}
			if n.DataAtom == atom.Br || n.DataAtom == atom.Li || n.DataAtom == atom.Tr {
				b.WriteByte('\n')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func renderBlock(blk eraBlock) string {
	var b strings.Builder
	for _, n := range blk.nodes {
		_ = html.Render(&b, n)
	}
	return b.String()
}

// eraSimilarity is the share of words two texts have in common, in order.
func eraSimilarity(a, b string) float64 {
	wa, wb := strings.Fields(strings.ToLower(a)), strings.Fields(strings.ToLower(b))
	if len(wa)+len(wb) == 0 {
		return 1
	}
	if len(wa)*len(wb) > maxDiffCells {
		return 0
	}
	same := 0
	for _, op := range lcsOps(wa, wb) {
		if op.Op == ' ' {
			same++
		}
	}
	return 2 * float64(same) / float64(len(wa)+len(wb))
}

// markAdded renders the version block with the words it adds to usualText
// wrapped in <mark class="era-w">. Only text is wrapped; tags and attributes
// are rendered as they were.
func markAdded(usualText string, blk eraBlock) string {
	var nodes []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			nodes = append(nodes, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range blk.nodes {
		walk(n)
	}
	// Words of the version, in the order its text nodes hold them.
	type word struct {
		node       *html.Node
		start, end int // byte offsets in node.Data
	}
	var words []word
	var toks []string
	for _, n := range nodes {
		start := -1
		for i, r := range n.Data + " " {
			if unicode.IsSpace(r) || i == len(n.Data) {
				if start >= 0 {
					words = append(words, word{n, start, i})
					toks = append(toks, strings.ToLower(n.Data[start:i]))
					start = -1
				}
				continue
			}
			if start < 0 {
				start = i
			}
		}
	}
	usual := strings.Fields(strings.ToLower(usualText))
	added := make([]bool, len(toks))
	if len(usual)*len(toks) <= maxDiffCells {
		k := 0
		for _, op := range lcsOps(usual, toks) {
			switch op.Op {
			case ' ':
				k++
			case '+':
				added[k] = true
				k++
			}
		}
	}
	// Split each text node around its runs of added words, joining a run
	// across the spaces inside one node.
	for _, n := range nodes {
		type span struct{ s, e int }
		var spans []span
		for wi, w := range words {
			if w.node != n || !added[wi] {
				continue
			}
			if l := len(spans); l > 0 && strings.TrimSpace(n.Data[spans[l-1].e:w.start]) == "" && wi > 0 && added[wi-1] && words[wi-1].node == n {
				spans[l-1].e = w.end
				continue
			}
			spans = append(spans, span{w.start, w.end})
		}
		if len(spans) == 0 {
			continue
		}
		parent, next := n.Parent, n.NextSibling
		data := n.Data
		var repl []*html.Node
		pos := 0
		for _, sp := range spans {
			if sp.s > pos {
				repl = append(repl, &html.Node{Type: html.TextNode, Data: data[pos:sp.s]})
			}
			m := &html.Node{Type: html.ElementNode, Data: "mark", DataAtom: atom.Mark,
				Attr: []html.Attribute{{Key: "class", Val: "era-w"}}}
			m.AppendChild(&html.Node{Type: html.TextNode, Data: data[sp.s:sp.e]})
			repl = append(repl, m)
			pos = sp.e
		}
		if pos < len(data) {
			repl = append(repl, &html.Node{Type: html.TextNode, Data: data[pos:]})
		}
		if parent == nil {
			// A top-level text node: swap it within the block's own list.
			for i, top := range blk.nodes {
				if top == n {
					blk.nodes = append(blk.nodes[:i], append(repl, blk.nodes[i+1:]...)...)
					break
				}
			}
			continue
		}
		parent.RemoveChild(n)
		for _, r := range repl {
			parent.InsertBefore(r, next)
		}
	}
	return renderBlock(blk)
}
