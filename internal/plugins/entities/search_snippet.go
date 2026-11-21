package entities

// search_snippet.go builds the line the Search page shows under a page that
// only mentions the reader's words: a short stretch of the page's text with
// the words marked.
//
// The text it quotes is always the page's search_text, never entry_html. That
// column is the one place every reader's search is matched, and
// buildSearchText has already removed what only some readers may see (GM
// secrets, owner-only fields, mention link text). Quoting from the same column
// the page was found by means a snippet can never show a sentence the reader
// could not have matched on, and a word that exists only inside a secret
// leaves the page unfound rather than found with a hole. The reader's own
// view of the page is enforced again at read time by SearchTexts.

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"unicode"
)

// SnippetPart is one run of a snippet. The page draws Text as plain text and
// wraps the runs with Mark set; no HTML crosses the wire, so nothing in a
// page's words can become markup.
type SnippetPart struct {
	Text string `json:"text"`
	Mark bool   `json:"mark,omitempty"`
}

const (
	// snippetWidth is the longest stretch quoted, in characters: about one
	// line of the page's list on a desktop and two on a phone.
	snippetWidth = 140
	// snippetLead is how much text before the first match is kept, so the
	// words are read in their sentence.
	snippetLead = 40
)

// SearchTextReader is the slice of the entity repository that serves a
// snippet's source text. The MariaDB repository implements it.
type SearchTextReader interface {
	// SearchTexts returns each listed page's search_text, only for pages the
	// viewer may see (the same visibility rule as Search).
	SearchTexts(ctx context.Context, campaignID string, ids []string, role int, userID string) (map[string]string, error)
}

// SearchSnippets returns, for each page id, the marked line of its text that
// holds the query's words. A page with no such line (its words matched only
// in an alias or descriptor) is left out. Ids the viewer may not see are
// dropped by the repository, so the caller cannot ask for more than it was
// already shown. One query serves the whole page of results.
func (s *entityService) SearchSnippets(ctx context.Context, campaignID, query string, ids []string, role int, userID string) map[string][]SnippetPart {
	reader, ok := s.entities.(SearchTextReader)
	if !ok || len(ids) == 0 {
		return nil
	}
	terms := SearchTerms(query)
	if len(terms) == 0 {
		return nil
	}
	texts, err := reader.SearchTexts(ctx, campaignID, ids, role, userID)
	if err != nil {
		slog.Warn("entities: search snippets unavailable", slog.Any("error", err))
		return nil
	}
	out := make(map[string][]SnippetPart, len(texts))
	for id, text := range texts {
		if parts := BuildSnippet(text, terms); len(parts) > 0 {
			out[id] = parts
		}
	}
	return out
}

// SearchTexts implements SearchTextReader. It applies visibilityFilter again
// even though the ids came from a filtered search, so this method is safe on
// its own.
func (r *entityRepository) SearchTexts(ctx context.Context, campaignID string, ids []string, role int, userID string) (map[string]string, error) {
	texts := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return texts, nil
	}
	placeholders := make([]string, len(ids))
	args := []any{campaignID}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := `SELECT e.id, COALESCE(e.search_text, '') FROM entities e
	          WHERE e.campaign_id = ? AND e.id IN (` + strings.Join(placeholders, ", ") + `)`
	vis, visArgs := visibilityFilter(role, userID)
	query += vis
	args = append(args, visArgs...)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading search text: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, fmt.Errorf("scanning search text: %w", err)
		}
		texts[id] = text
	}
	return texts, rows.Err()
}

// SearchTerms are the words a snippet marks: the ones Search matched on.
// A query of four or more characters is matched word by word (FULLTEXT) and
// a shorter one, or one with no word in it, as one run (LIKE), so the same
// split is used here.
func SearchTerms(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	raw := []string{q}
	if words := strings.Fields(stripFTOperators(q)); len(q) >= 4 && len(words) > 0 {
		raw = words
	}
	terms := make([]string, 0, len(raw))
	for _, t := range raw {
		if t = strings.TrimSpace(t); t != "" {
			terms = append(terms, t)
		}
	}
	return terms
}

// BuildSnippet quotes text around the first of the terms, marking every
// occurrence in the stretch. It returns nil when no term is in the text.
// Matching ignores case rune by rune, so a position in the lowered text is the
// same position in the original.
func BuildSnippet(text string, terms []string) []SnippetPart {
	if len(terms) == 0 || text == "" {
		return nil
	}
	plain := []rune(strings.Join(strings.Fields(html.UnescapeString(text)), " "))
	lower := lowerRunes(plain)
	needles := make([][]rune, 0, len(terms))
	for _, t := range terms {
		if n := lowerRunes([]rune(t)); len(n) > 0 {
			needles = append(needles, n)
		}
	}

	first := -1
	for _, n := range needles {
		if at := indexRunes(lower, n, 0); at >= 0 && (first < 0 || at < first) {
			first = at
		}
	}
	if first < 0 {
		return nil
	}

	start := first - snippetLead
	if start < 0 {
		start = 0
	}
	// Begin on a word, not in the middle of one, unless the match itself is
	// the word being cut.
	if start > 0 {
		for start < first && plain[start-1] != ' ' {
			start++
		}
	}
	end := start + snippetWidth
	if end >= len(plain) {
		end = len(plain)
	} else {
		for end > first+1 && plain[end] != ' ' {
			end--
		}
	}

	var parts []SnippetPart
	if start > 0 {
		parts = append(parts, SnippetPart{Text: "…"})
	}
	var run []rune
	flush := func(mark bool) {
		if len(run) > 0 {
			parts = append(parts, SnippetPart{Text: string(run), Mark: mark})
			run = run[:0]
		}
	}
	for i := start; i < end; {
		if n := matchAt(lower, needles, i, end); n > 0 {
			flush(false)
			run = append(run, plain[i:i+n]...)
			flush(true)
			i += n
			continue
		}
		run = append(run, plain[i])
		i++
	}
	flush(false)
	if end < len(plain) {
		parts = append(parts, SnippetPart{Text: "…"})
	}
	return parts
}

// matchAt is the length of the longest needle found at i without running past
// end, or 0.
func matchAt(lower []rune, needles [][]rune, i, end int) int {
	best := 0
	for _, n := range needles {
		if len(n) > best && i+len(n) <= end && hasPrefixRunes(lower[i:], n) {
			best = len(n)
		}
	}
	return best
}

func lowerRunes(in []rune) []rune {
	out := make([]rune, len(in))
	for i, r := range in {
		out[i] = unicode.ToLower(r)
	}
	return out
}

func hasPrefixRunes(s, prefix []rune) bool {
	if len(prefix) > len(s) {
		return false
	}
	for i, r := range prefix {
		if s[i] != r {
			return false
		}
	}
	return true
}

func indexRunes(s, needle []rune, from int) int {
	for i := from; i+len(needle) <= len(s); i++ {
		if hasPrefixRunes(s[i:], needle) {
			return i
		}
	}
	return -1
}
