package entities

import (
	"regexp"
	"strings"
)

// htmlTagPattern matches HTML tags for stripping in entry excerpts.
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// previewExcerptLen is the excerpt length the hover card shows.
const previewExcerptLen = 150

// previewExcerpt turns an entry's HTML, as the reader is served it, into the
// short plain-text excerpt the hover card shows. The caller serves the HTML
// first (servedText) because once the markup is gone nothing marks which
// words were secret or which link named a hidden page.
func previewExcerpt(entryHTML string) string {
	plain := htmlTagPattern.ReplaceAllString(entryHTML, "")
	plain = strings.Join(strings.Fields(plain), " ")
	if len(plain) <= previewExcerptLen {
		return plain
	}
	truncated := plain[:previewExcerptLen]
	if idx := strings.LastIndex(truncated, " "); idx > 100 {
		truncated = truncated[:idx]
	}
	return truncated + "..."
}
