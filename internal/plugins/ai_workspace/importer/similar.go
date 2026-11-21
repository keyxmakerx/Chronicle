// similar.go finds the page a "make a new page" request is much like, so the
// review can say so before the owner approves a twin. Everything here is
// cheap and deterministic on purpose: a normalised-name comparison and a
// word-set overlap, with no scoring model to explain or tune.

package importer

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	htmltok "golang.org/x/net/html"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

const (
	// bodyJaccardMin is how much of the combined vocabulary two bodies must
	// share to count as the same words. Below it, two pages on one subject
	// (a rumour and the NPC it is about) stay separate pages.
	bodyJaccardMin = 0.8
	// bodyWordsMin keeps a one-line body from matching on its few words:
	// overlap only means something once there is a vocabulary to overlap.
	bodyWordsMin = 20
	// similarCandidates bounds the pages weighed per request.
	similarCandidates = 30
)

// PageFinder is the slice of the entities service the search needs. Search
// runs with the viewer's own role, so a page the viewer cannot see is never a
// candidate and never named in the review.
type PageFinder interface {
	Search(ctx context.Context, campaignID, query string, typeID int, role int, userID string, opts entities.ListOptions) ([]entities.Entity, int, error)
}

// SimilarPage is the existing page a create request is much like.
type SimilarPage struct {
	ID   string
	Name string
	// Differ is how many distinct words one body has that the other lacks;
	// meaningful only when HasBody.
	Differ  int
	HasBody bool
	// CanUpdate is whether turning the request into a change to this page
	// can work: a change finds its page by the slug of the name it is given.
	CanUpdate bool
	// GMOnly is what replacing the page's text would lose, so the review can
	// ask for the same tick it asks for on any other change.
	GMOnly GMOnlyParts
	// Have and Sent are the two texts as lines, for "See both". Have is the
	// page's reader-safe text, so GM-only passages are not copied into a
	// comparison; GMOnlyMark stands where one was left out.
	Have []string
	Sent []string
	// Pending is set when the match is a page another row or request means to
	// add, not one the campaign has: there is no page to change, so the
	// review offers only "See both". Elsewhere is then true for a page in a
	// different waiting request and false for one in this same review.
	Pending   bool
	Elsewhere bool
}

// GMOnlyMark stands in a reader-safe text where a GM-only part was left out.
// It is a private-use character so it is never a word and never real text;
// the review turns it into a plain sentence.
const GMOnlyMark = "\ue000"

// PendingPage is a page some other row or waiting request means to add, in
// the AI's own words. Index is its row in the review being built, or -1 for
// a page of another request.
type PendingPage struct {
	Index int
	Name  string
	Body  string
}

// suffixes an AI or an export tacks on to make a "different" name for the
// same page. Only whole trailing groups are dropped, never a word inside.
var (
	trailGroup = regexp.MustCompile(`(?i)\s*[\(\[]\s*(?:imported|import|copy(?:\s*\d+)?|new|duplicate|draft|updated|revised|v?\d+)\s*[\)\]]\s*$`)
	trailWord  = regexp.MustCompile(`(?i)\s+[-–—:]?\s*(?:imported|copy(?:\s*\d+)?|duplicate)\s*$`)
)

// NormalizeTitle lowercases a name, drops a trailing "(Imported)" or "(copy)"
// style suffix and all punctuation, and collapses spaces, so "New Player
// Guide (Imported)" and "new player guide" read the same.
func NormalizeTitle(name string) string {
	s := strings.TrimSpace(name)
	for {
		t := trailGroup.ReplaceAllString(s, "")
		t = trailWord.ReplaceAllString(t, "")
		if t == s || strings.TrimSpace(t) == "" {
			break
		}
		s = t
	}
	return strings.Join(words(s), " ")
}

// words splits on anything that is not a letter or digit and lowercases.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

// titleStrength is how alike two names are once normalised: 2 for the same,
// 1 when one holds every word of the other (and has at least two, so "Guide"
// does not match every guide), 0 otherwise. Different numbers never match
// ("Session 3" and "Session 4" share no containment).
func titleStrength(a, b string) int {
	na, nb := NormalizeTitle(a), NormalizeTitle(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 2
	}
	wa, wb := strings.Fields(na), strings.Fields(nb)
	if len(wa) > len(wb) {
		wa, wb = wb, wa
	}
	if len(wa) < 2 {
		return 0
	}
	in := map[string]bool{}
	for _, w := range wb {
		in[w] = true
	}
	for _, w := range wa {
		if !in[w] {
			return 0
		}
	}
	return 1
}

func wordSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range words(s) {
		out[w] = true
	}
	return out
}

// bodyOverlap returns the Jaccard overlap of two bodies' distinct words and
// how many words only one of them has. ok is false when either body is too
// short to compare.
func bodyOverlap(a, b string) (jaccard float64, differ int, ok bool) {
	sa, sb := wordSet(a), wordSet(b)
	if len(sa) < bodyWordsMin || len(sb) < bodyWordsMin {
		return 0, 0, false
	}
	both := 0
	for w := range sa {
		if sb[w] {
			both++
		}
	}
	union := len(sa) + len(sb) - both
	return float64(both) / float64(union), union - both, true
}

// bestWord is the longest word of a name, the query that finds its look-alikes
// without reading the whole campaign. Names are matched on every word later;
// this only has to bring them into the candidate list.
func bestWord(name string) string {
	best := ""
	for _, w := range strings.Fields(NormalizeTitle(name)) {
		if len([]rune(w)) > len([]rune(best)) {
			best = w
		}
	}
	return best
}

// matchRank is how alike two pages are, 0 for not alike: a name match
// outranks a body match, and a body match adds its overlap. One rule serves
// existing pages and pending ones so the review and the AI are told the same.
func matchRank(name, text, otherName, otherText string) float64 {
	rank := 0.0
	switch titleStrength(name, otherName) {
	case 2:
		rank = 3
	case 1:
		rank = 2
	}
	if j, _, ok := bodyOverlap(text, otherText); ok && j >= bodyJaccardMin {
		rank += j
	}
	return rank
}

// FindSimilar returns the best existing page a create request is much like,
// or nil. A candidate counts by name (same or containing, after the suffix is
// dropped) or by body (Jaccard over bodyJaccardMin); a name match outranks a
// body match. Candidates come from a name search, so a page whose name shares
// no word with the request is found only when the names are alike by
// normalisation, not by body alone. A page the request already names exactly
// is the classifier's conflict and is left to it.
func FindSimilar(ctx context.Context, finder PageFinder, campaignID string, v Viewer, p ParsedPage, rollTables map[string]string) *SimilarPage {
	if finder == nil || p.Status == StatusParseError || p.FrontMatter.IsEra() {
		return nil
	}
	q := bestWord(p.Name)
	if len([]rune(q)) < 2 {
		return nil
	}
	found, _, err := finder.Search(ctx, campaignID, q, 0, v.Role, v.UserID, entities.ListOptions{PerPage: similarCandidates, SearchScope: entities.SearchScopeNamed})
	if err != nil {
		return nil
	}
	own := entities.Slugify(p.Name)
	sent := markdownText(p.Body)
	var best *entities.Entity
	bestRank := 0.0
	for i := range found {
		e := &found[i]
		if e.Slug == own || e.IsTemplate {
			continue
		}
		if rank := matchRank(p.Name, sent, e.Name, entityText(e)); rank > bestRank {
			best, bestRank = e, rank
		}
	}
	if best == nil {
		return nil
	}
	have := entityText(best)
	out := &SimilarPage{ID: best.ID, Name: best.Name, CanUpdate: entities.Slugify(best.Name) == best.Slug}
	if _, d, ok := bodyOverlap(sent, have); ok {
		out.Differ, out.HasBody = d, true
	}
	out.Have, out.Sent = textLines(have), textLines(sent)
	if out.CanUpdate {
		out.GMOnly = GMOnlyPartsForMarkdown(best, p.Body, rollTables)
	}
	return out
}

// entityText is a page's reader-safe words: its HTML with GM-only passages
// removed, as plain lines. Reading the stripped text keeps hidden passages
// out of both the word counts and the side-by-side; the mark left in their
// place lets the side-by-side say a part is not shown instead of showing a
// sentence with a hole in it.
func entityText(e *entities.Entity) string {
	if e == nil || e.EntryHTML == nil {
		return ""
	}
	return htmlText(sanitize.RedactSecretsHTML(*e.EntryHTML, GMOnlyMark))
}

// htmlText turns HTML into text with a line break after each block, so the
// paragraphs can be compared as lines.
func htmlText(src string) string {
	var b strings.Builder
	z := htmltok.NewTokenizer(strings.NewReader(src))
	for {
		switch z.Next() {
		case htmltok.ErrorToken:
			return b.String()
		case htmltok.TextToken:
			b.Write(z.Text())
		case htmltok.StartTagToken, htmltok.EndTagToken, htmltok.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "p", "li", "br", "div", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote":
				b.WriteByte('\n')
			}
		}
	}
}

// markdownText is the AI's body as the plain lines a reader would see, so it
// is compared with a page's text and not with its markup. A body that will
// not convert falls back to its own words.
func markdownText(body string) string {
	h, err := MarkdownToHTML(body)
	if err != nil {
		return body
	}
	return htmlText(h)
}

// textLines splits text into trimmed, non-empty lines.
func textLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// pendingBest is the page in others the request is most like, by the same
// rule as matchRank, skipping the request's own row. A page whose name is the
// request's own slug is still a match here: two rows that would make the same
// page are the clearest case.
func pendingBest(p ParsedPage, self int, sent string, others []PendingPage) *PendingPage {
	var best *PendingPage
	bestRank := 0.0
	for i := range others {
		o := &others[i]
		if o.Index == self && self >= 0 {
			continue
		}
		if rank := matchRank(p.Name, sent, o.Name, markdownText(o.Body)); rank > bestRank {
			best, bestRank = o, rank
		}
	}
	return best
}

// FindSimilarPending returns the page among others that a create request is
// much like, or nil: the review's check for two pages in one request, or in
// two waiting requests, that would make the same page. It reads nothing from
// the campaign, so there is no viewer to hide anything from; the pages are
// the AI's own text, which the owner can read in full on the same screen.
func FindSimilarPending(p ParsedPage, self int, others []PendingPage) *SimilarPage {
	if p.Status == StatusParseError || p.FrontMatter.IsEra() || len(others) == 0 {
		return nil
	}
	sent := markdownText(p.Body)
	best := pendingBest(p, self, sent, others)
	if best == nil {
		return nil
	}
	have := markdownText(best.Body)
	out := &SimilarPage{Name: best.Name, Pending: true, Elsewhere: best.Index < 0}
	if _, d, ok := bodyOverlap(sent, have); ok {
		out.Differ, out.HasBody = d, true
	}
	out.Have, out.Sent = textLines(have), textLines(sent)
	return out
}
