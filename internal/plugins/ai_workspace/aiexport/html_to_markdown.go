package aiexport

import (
	"log/slog"
	"strings"
	"sync"

	md "github.com/JohannesKaufmann/html-to-markdown"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// converter holds the configured html-to-markdown converter. Built once
// via sync.Once so every render call reuses the same plugin chain
// without re-allocating. The converter is safe for concurrent use.
var (
	converter     *md.Converter
	converterOnce sync.Once
)

// getConverter returns the lazily-initialised markdown converter.
// The default options strip empty tags + collapse whitespace +
// preserve fenced code blocks, which is what we want for AI-
// consumable output.
func getConverter() *md.Converter {
	converterOnce.Do(func() {
		converter = md.NewConverter("", true, nil)
	})
	return converter
}

// htmlToMarkdown converts an HTML pointer to markdown, applying the
// SEC-6-AMENDED sanitize.HTMLPtr egress invariant BEFORE the converter
// sees the input. A nil pointer returns "" (no body). An empty string
// returns "" (no body). Otherwise:
//
//  1. sanitize.HTMLPtr strips <script>, javascript: URLs, on* handlers
//     using bluemonday's UGC policy (same allowlist that protects
//     /api/v1/* egress per internal/plugins/syncapi/egress_sanitize.go).
//  2. The converter walks the sanitized HTML tree and emits markdown.
//  3. Surrounding whitespace is trimmed so consecutive section renders
//     don't accumulate blank lines.
//
// Returns the converter's error verbatim so callers can decide whether
// to skip the body or fail the export. The defensive default is to
// skip (one entity's bad HTML shouldn't poison the whole document).
//
// IMPORTANT: every renderer that emits user HTML MUST funnel through
// this function. The renderer_test.go AST structural pin enforces it.
func htmlToMarkdown(p *string) (string, error) {
	if p == nil || *p == "" {
		return "", nil
	}
	clean := sanitize.HTMLPtr(p)
	if clean == nil || *clean == "" {
		return "", nil
	}
	got, err := getConverter().ConvertString(*clean)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(got), nil
}

// convertSkipMarker is emitted in place of a single field whose HTML could
// not be converted to markdown. It keeps the surrounding item (and the rest
// of the document) intact while telling the owner + downstream AI that one
// field's content was dropped, rather than silently vanishing.
const convertSkipMarker = "_[content omitted: could not be converted to markdown]_"

// bodyOrSkip turns an htmlToMarkdown (got, err) result into a body string
// that is always safe to emit. On a conversion error it logs the failure
// (labelled for triage) and returns convertSkipMarker instead of the error —
// honoring this file's "one entity's bad HTML shouldn't poison the whole
// document" contract, which the per-category renderers previously broke by
// propagating the error and aborting the entire export.
//
// Callers MUST still invoke htmlToMarkdown(...) directly and pass its two
// results here, so the SEC-6-AMENDED egress invariant (sanitize.HTMLPtr runs
// before the converter) and its AST structural pin both stay intact. `kind`
// and `item` identify the failing field in logs (e.g. "entity body",
// "Lyra Vance"). A nil error passes `got` through unchanged.
func bodyOrSkip(kind, item, got string, err error) string {
	if err != nil {
		slog.Warn("aiexport: skipped unconvertible HTML field",
			slog.String("kind", kind),
			slog.String("item", item),
			slog.Any("error", err))
		return convertSkipMarker
	}
	return got
}
