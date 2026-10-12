package aiexport

import (
	"log/slog"
	"strings"
	"sync"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"

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
		converter.AddRules(mentionRule())
	})
	return converter
}

// htmlToMarkdown converts an HTML pointer to markdown, applying the
// SEC-6-AMENDED sanitize.HTMLPtr egress invariant (strips <script>,
// javascript: URLs, on* handlers via bluemonday's UGC policy) BEFORE the
// converter sees the input. Nil or empty input returns "".
//
// Returns the converter's error verbatim; callers decide whether to skip the
// body or fail the export.
//
// Every renderer that emits user HTML MUST funnel through this function; the
// renderer_test.go AST structural pin enforces it.
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
// that is always safe to emit: on error it logs the failure and returns
// convertSkipMarker instead, so one entity's bad HTML doesn't abort the
// export.
//
// Callers MUST still invoke htmlToMarkdown(...) directly and pass its two
// results here, to keep the SEC-6-AMENDED egress invariant and its AST
// structural pin intact. `kind`/`item` identify the failing field in logs.
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

// mentionRule writes a page mention as `@[Name]`, the form AI Import reads
// back as a page link, instead of a markdown link whose address means nothing
// outside Chronicle. The name is the anchor's own text, so a page renamed
// since the mention was typed still shows its old name; import then warns
// that it matches nothing rather than linking the wrong page. Any other link
// falls through to the default rule.
func mentionRule() md.Rule {
	return md.Rule{
		Filter: []string{"a"},
		Replacement: func(_ string, sel *goquery.Selection, _ *md.Options) *string {
			if _, ok := sel.Attr("data-mention-id"); !ok {
				return nil
			}
			text := strings.TrimSpace(sel.Text())
			name := strings.TrimSpace(strings.TrimPrefix(text, "@"))
			name = strings.NewReplacer("[", "", "]", "", "|", "").Replace(name)
			if name == "" {
				return md.String(text)
			}
			out := "@[" + name + "]"
			return &out
		},
	}
}
