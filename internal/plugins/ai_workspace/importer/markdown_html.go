// markdown_html.go converts page bodies from markdown to HTML and
// applies sanitize.HTML on the output — the SEC-6 ingress funnel every
// body the AI Workspace stores must pass through before persistence.
// The AST structural pin in committer_sanitize_test.go enforces that
// the committer routes through MarkdownToHTML rather than calling
// goldmark + sanitize separately.

package importer

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// md is the configured goldmark instance. Extensions enabled:
//   - GFM (tables, strikethrough, task lists, autolinks) — AI tools
//     use these heavily; opting in covers >95% of generated content
//   - WithAutoHeadingID — heading anchors are stable for the
//     wikilink resolver to point at
//
// html.WithUnsafe is OFF (the default) — goldmark won't emit raw
// HTML embedded in the markdown source, only the structural HTML
// it generates from markdown tokens. Defense-in-depth on top of
// sanitize.HTML below.
//
// Built once at init time; the Converter is safe for concurrent use.
var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(html.WithHardWraps()),
)

// MarkdownToHTML converts page-body markdown to sanitized HTML.
// Single funnel — callers MUST route through this function rather
// than calling goldmark.Convert + sanitize.HTML separately: (1)
// goldmark parses + renders to HTML with no raw HTML passthrough,
// (2) sanitize.HTML strips any residual <script>/javascript:/on*
// handlers per the bluemonday UGC policy, so the output is safe for
// storage in EntryHTML and for direct render via templ.Raw().
//
// WithPageLinks additionally turns @[Page] / @[Page|text] into page links.
//
// Returns the empty string for empty input. Parse-failure errors are
// friendly-worded for operator UIs; the goldmark error is preserved
// via %w for log-side debugging.
func MarkdownToHTML(input string, opts ...RenderOption) (string, error) {
	if input == "" {
		return "", nil
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(input), &buf); err != nil {
		return "", fmt.Errorf("could not parse markdown body — check heading structure: %w", err)
	}
	var o renderOpts
	for _, opt := range opts {
		opt(&o)
	}
	out := buf.String()
	if o.links != nil {
		// Before sanitize, so the anchors are vetted like any other markup.
		out = linkMentions(out, o.links)
	}
	return sanitize.HTML(out), nil
}
