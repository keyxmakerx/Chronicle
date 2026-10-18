// markdown_html.go converts page bodies from markdown to HTML and
// applies sanitize.HTML on the output — the SEC-6 ingress funnel every
// body the AI Workspace stores must pass through before persistence.
// The AST structural pin in committer_sanitize_test.go enforces that
// the committer routes through MarkdownToHTML rather than calling
// goldmark + sanitize separately.

package importer

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// md is the configured goldmark instance. Extensions enabled:
//   - GFM (tables, strikethrough, task lists, autolinks) — AI tools
//     use these heavily; opting in covers >95% of generated content
//
// Heading ids are deliberately off: goldmark derives them from the heading
// text, so a GM-only passage in a heading would leak into the id attribute,
// which no strip pass looks at. Nothing links to them (the editor reads the
// JSON, which has no ids).
//
// html.WithUnsafe is OFF (the default) — goldmark won't emit raw
// HTML embedded in the markdown source, only the structural HTML
// it generates from markdown tokens. Defense-in-depth on top of
// sanitize.HTML below.
//
// Built once at init time; the Converter is safe for concurrent use.
var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
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
// {gm{…}gm} passages become the editor's GM-only spans (see SecretOpen) only
// with WithGMOnlyPassages, which page and era bodies pass; a viewer below
// owner/co-DM gets ErrSecretNotAllowed rather than a silent downgrade to
// public text. Without the option a marker is refused (ErrSecretNotInText),
// because the other texts that come through here are not stripped for players.
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
	if HasSecretMarkerText(out) {
		switch {
		case o.markers:
		case o.passages:
			// Markers become GM-only text only for a viewer allowed to write
			// it, so a scribe's import cannot hide or reveal passages. No
			// resolver means no viewer, so it fails closed.
			if o.links == nil || !secretWriter(o.links.viewer) {
				return "", ErrSecretNotAllowed
			}
		default:
			return "", ErrSecretNotInText
		}
		var err error
		if out, err = convertSecretMarkers(out); err != nil {
			return "", err
		}
	}
	if o.links != nil {
		// Before sanitize, so the anchors are vetted like any other markup.
		out = linkMentions(out, o.links)
	}
	return sanitize.HTML(out), nil
}

// markdownFailReason is the row-level wording for a MarkdownToHTML error.
func markdownFailReason(err error) string {
	switch {
	case errors.Is(err, ErrSecretUnbalanced):
		return secretUnbalancedReason
	case errors.Is(err, ErrSecretInAttribute):
		return secretAttributeReason
	case errors.Is(err, ErrSecretNotInText):
		return SecretNotInTextReason
	}
	if errors.Is(err, ErrSecretNotAllowed) {
		return "Not imported: this page has GM-only passages ({gm{…}gm}), which only the campaign owner or a co-DM may import."
	}
	return "Could not parse the page's markdown body — check the heading structure."
}
