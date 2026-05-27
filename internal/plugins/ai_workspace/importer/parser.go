// parser.go splits multi-page AI-generated markdown into ParsedPage
// structs. Canonical multi-page boundary is a YAML front-matter
// opener (`---\n` at the start of a line, only when FM mode is
// active for the input). H1 is NOT a split signal when FM mode is
// active — the AI's body can legitimately contain `#` lines.
//
// When NO front-matter blocks appear anywhere in the input, the
// parser falls back to H1 splitting (each `# Heading` starts a new
// page; matches the §3.5 prompt template's instruction to AI tools
// to use `#` headings per page).
//
// Per cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md
// §3.3 (front-matter schema) + §4 Phase 4.

package importer

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// validVisibilities enumerates the allowed `visibility:` enum values
// per scoping §3.3. Anything else (e.g. "PUBLIC", "everyone") is a
// parse error per-row — the operator fixes the source or picks a
// different bulk default and reparses.
var validVisibilities = map[string]bool{
	"private": true,
	"dm_only": true,
	"public":  true,
}

// h1Re matches a Markdown H1 heading at the start of a line. Used
// for both the per-page name resolution + the fallback split when
// no front-matter exists.
var h1Re = regexp.MustCompile(`(?m)^# +(.+?)\s*$`)

// fmOpenRe matches the opening `---` line of a YAML front-matter
// block. Anchored at line start so a `---` inside a code block
// doesn't false-positive. The closing fence is found by scanning
// forward; we don't use a regex for that to keep the boundary logic
// linear.
var fmOpenRe = regexp.MustCompile(`(?m)^---\s*$`)

// Parse splits raw multi-page markdown into a slice of ParsedPage.
// Each page is independently parsed + classified; one bad page
// doesn't poison the rest. The slice preserves input order so the
// review screen renders pages top-to-bottom matching the operator's
// paste / file list.
//
// Status classification (StatusConflict / StatusNewCategory) is NOT
// done here — the parser is plugin-isolated from the entities
// service. The handler does conflict + category lookup after
// parsing.
func Parse(input string) []ParsedPage {
	now := time.Now()
	pages := splitPages(input)
	out := make([]ParsedPage, 0, len(pages))
	for i, raw := range pages {
		p := parseOnePage(raw)
		p.SourceIndex = i
		p.ParsedAt = now
		out = append(out, p)
	}
	return out
}

// splitPages divides the input into raw page strings using the FM-
// opener as the canonical boundary. Falls back to H1 split when no
// FM appears anywhere — operators sometimes paste AI output that
// has H1s without FM.
//
// A page is "opener `---` line ... closer `---` line ... body". The
// fmOpenRe regex matches BOTH opener and closer fences; we pair
// them odd/even to identify openers, then slice between consecutive
// openers (or to EOF for the last).
func splitPages(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	fmIdx := fmOpenRe.FindAllStringIndex(input, -1)
	if len(fmIdx) == 0 {
		return splitByH1(input)
	}

	// Pair fences: even-indexed matches are openers, odd-indexed
	// are closers. Collect only opener positions.
	openerStarts := make([]int, 0, len(fmIdx)/2+1)
	for i, ix := range fmIdx {
		if i%2 == 0 {
			openerStarts = append(openerStarts, ix[0])
		}
	}
	if len(openerStarts) == 0 {
		return splitByH1(input)
	}

	out := make([]string, 0, len(openerStarts))
	for i, start := range openerStarts {
		end := len(input)
		if i+1 < len(openerStarts) {
			end = openerStarts[i+1]
		}
		page := strings.TrimSpace(input[start:end])
		if page != "" {
			out = append(out, page)
		}
	}
	return out
}

// splitByH1 is the fallback splitter — used only when the input
// has no front-matter anywhere. Each `# Heading` line starts a new
// page; content before the first H1 is discarded as preamble.
func splitByH1(input string) []string {
	matches := h1Re.FindAllStringIndex(input, -1)
	if len(matches) == 0 {
		// Single page, no H1, no FM — treat the whole input as
		// one un-named page. Status classification will mark it as
		// a parse error (no name).
		return []string{input}
	}
	out := make([]string, 0, len(matches))
	for i, ix := range matches {
		start := ix[0]
		end := len(input)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		page := strings.TrimSpace(input[start:end])
		if page != "" {
			out = append(out, page)
		}
	}
	return out
}

// parseOnePage extracts front-matter + body + classifies one raw
// page string. Returns a ParsedPage with Status set to StatusNew
// on success or StatusParseError with ParseError populated.
//
// Category/conflict classification happens in the handler — this
// function is repo-free.
func parseOnePage(raw string) ParsedPage {
	p := ParsedPage{Body: strings.TrimSpace(raw)}

	// Detect + strip front-matter.
	if strings.HasPrefix(p.Body, "---") {
		body, fm, fmRaw, err := extractFrontMatter(p.Body)
		if err != nil {
			p.Status = StatusParseError
			p.ParseError = err.Error()
			return p
		}
		p.HasFrontMatter = true
		p.FrontMatter = fm
		p.Body = body
		// Warn for unknown YAML keys.
		if unknown := unknownYAMLKeys(fmRaw); len(unknown) > 0 {
			p.Warnings = append(p.Warnings,
				"Unknown front-matter key(s): "+strings.Join(unknown, ", "))
		}
	}

	// Resolve the page name.
	if p.FrontMatter.Name != "" {
		p.Name = strings.TrimSpace(p.FrontMatter.Name)
	} else if m := h1Re.FindStringSubmatch(p.Body); len(m) >= 2 {
		p.Name = strings.TrimSpace(m[1])
		if p.HasFrontMatter {
			p.Warnings = append(p.Warnings,
				"No `name:` in front-matter; using first H1 as name")
		}
	}

	// Validate visibility enum BEFORE checking name — a bad
	// visibility value is the noisier failure mode and the operator
	// notices it first.
	if v := strings.TrimSpace(p.FrontMatter.Visibility); v != "" {
		if !validVisibilities[v] {
			p.Status = StatusParseError
			p.ParseError = fmt.Sprintf(
				"visibility: %q is not valid (must be one of private, dm_only, public)", v)
			return p
		}
	}

	// Empty body is a soft warning, not an error.
	if strings.TrimSpace(p.Body) == "" {
		p.Warnings = append(p.Warnings, "Page body is empty")
	}

	if !p.HasName() {
		p.Status = StatusParseError
		p.ParseError = "Could not determine page name " +
			"(no `name:` in front-matter and no `#` H1 heading in body)"
		return p
	}

	// Status remains "new" at parser-level; handler may upgrade to
	// StatusConflict or StatusNewCategory once it has campaign
	// context.
	p.Status = StatusNew
	return p
}

// extractFrontMatter splits a page's leading `---...---` block from
// the body. Returns the body (front-matter and its fences removed),
// the parsed FrontMatter, the RAW yaml bytes (for unknown-key
// detection), or an error if the block is malformed.
func extractFrontMatter(s string) (body string, fm FrontMatter, rawYAML string, err error) {
	// Match: opening `---\n`, then yaml content, then `---\n` on its
	// own line. The opening fence is already known to start at
	// position 0 (caller checks HasPrefix).
	lines := strings.SplitN(s, "\n", 2)
	if len(lines) < 2 {
		return "", fm, "", fmt.Errorf(
			"front-matter block has no closing `---` (open fence is on the only line)")
	}
	rest := lines[1]
	closeIdx := fmOpenRe.FindStringIndex(rest)
	if closeIdx == nil {
		return "", fm, "", fmt.Errorf(
			"front-matter block has no closing `---` line")
	}
	rawYAML = rest[:closeIdx[0]]
	body = strings.TrimSpace(rest[closeIdx[1]:])

	if err := yaml.Unmarshal([]byte(rawYAML), &fm); err != nil {
		return "", FrontMatter{}, "",
			fmt.Errorf("front-matter YAML parse failed: %v", err)
	}
	return body, fm, rawYAML, nil
}

// unknownYAMLKeys returns the top-level YAML keys that aren't in
// the FrontMatter schema. Used to populate the row's Warnings slice.
// Tolerant of comments + blank lines.
func unknownYAMLKeys(rawYAML string) []string {
	known := map[string]bool{
		"name": true, "type": true, "subcategory": true,
		"visibility": true, "tags": true, "description": true,
	}
	var unknown []string
	for _, line := range strings.Split(rawYAML, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		// Top-level key is `<word>:` (with indentation 0).
		// Skip indented lines (those are values of multi-line keys).
		if line != strings.TrimLeft(line, " \t") {
			continue
		}
		colon := strings.IndexByte(t, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(t[:colon])
		if key == "" {
			continue
		}
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	return unknown
}
