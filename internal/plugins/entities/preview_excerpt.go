package entities

import (
	"strings"
	"unicode/utf8"
)

// previewExcerptLen is the excerpt length, in characters, the hover card shows.
const previewExcerptLen = 150

// previewExcerpt turns an entry's HTML, as the reader is served it, into the
// short plain-text excerpt the hover card shows. The caller serves the HTML
// first (servedText) because once the markup is gone nothing marks which
// words were secret or which link named a hidden page.
//
// The result is plain text with entities decoded: every consumer escapes it
// when it renders, so an undecoded "&#34;" would reach the reader literally.
func previewExcerpt(entryHTML string) string {
	plain := htmlToPlain(entryHTML)
	if utf8.RuneCountInString(plain) <= previewExcerptLen {
		return plain
	}
	truncated := string([]rune(plain)[:previewExcerptLen])
	if idx := strings.LastIndex(truncated, " "); idx > 100 {
		truncated = truncated[:idx]
	}
	return truncated + "..."
}
