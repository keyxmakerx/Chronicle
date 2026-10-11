package entities

import (
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// htmlTagPattern matches HTML tags for stripping in entry excerpts.
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// previewExcerptLen is the excerpt length the hover card shows.
const previewExcerptLen = 150

// previewExcerpt turns an entry's HTML into the short plain-text excerpt the
// hover card shows. GM-only content is removed before the tags are, because
// once the markup is gone nothing marks which words were secret.
func previewExcerpt(entryHTML string, canSeeGM bool) string {
	if !canSeeGM {
		entryHTML = sanitize.StripSecretsHTML(entryHTML)
	}
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
