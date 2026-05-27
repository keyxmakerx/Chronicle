// markdown_html.go converts page bodies from markdown to HTML +
// applies sanitize.HTML on the output. This is the ingress mirror
// of SEC-6-AMENDED's egress invariant — every body the AI Workspace
// stores MUST pass through sanitize.HTML before any persistence
// hand-off.
//
// V1 Phase 4 doesn't actually store anything (commit handler is
// Phase 5), but this file establishes the funnel + the AST
// structural pin lands in Phase 5 to enforce that Phase 5's
// committer routes through MarkdownToHTML rather than calling
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
// than calling goldmark.Convert + sanitize.HTML separately.
//
// The pipeline:
//
//  1. goldmark parses + renders to HTML (no raw HTML passthrough).
//  2. sanitize.HTML strips any residual <script> / javascript: /
//     on* handlers per the bluemonday UGC policy — same allowlist
//     SEC-6-AMENDED egress uses.
//  3. Output is safe for storage in EntryHTML (Phase 5 committer)
//     + safe for direct render via templ.Raw() in any consumer.
//
// Returns the empty string for empty input; returns the goldmark
// error verbatim on parse failure (rare — goldmark is forgiving).
func MarkdownToHTML(input string) (string, error) {
	if input == "" {
		return "", nil
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(input), &buf); err != nil {
		return "", fmt.Errorf("markdown→HTML: %w", err)
	}
	return sanitize.HTML(buf.String()), nil
}
