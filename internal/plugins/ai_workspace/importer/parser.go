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

// fmOpenRe matches a `---` fence line (3+ dashes — AI output often
// emits `----`; Markdown treats 3+ as equivalent). Anchored at line
// start; trailing whitespace/CR tolerated for CRLF pastes. NOTE: a
// Markdown horizontal rule is the SAME token — splitPages tells the
// two apart by looking at what follows (see yamlKeyLineRe), so a
// `---` divider inside a page body no longer corrupts the split.
var fmOpenRe = regexp.MustCompile(`(?m)^-{3,}\s*$`)

// yamlKeyLineRe matches a line that plausibly starts a YAML mapping
// entry (`name: …`, `entity_type: …`). Used to distinguish a real
// front-matter opener (followed by keys) from a horizontal rule
// (followed by prose).
var yamlKeyLineRe = regexp.MustCompile(`^\s*[A-Za-z_][\w-]*\s*:`)

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

	// Classify each fence as opener / closer / stray by CONTENT, not
	// by blind odd/even pairing (the old scheme — one horizontal
	// rule in a body, or one missing closer, shifted the pairing and
	// corrupted every page after it; operator-reported 2026-06-12).
	//
	// A fence is an OPENER iff the first non-blank line after it
	// looks like a YAML key (`name: …`). Its CLOSER is the next
	// fence — consumed only when everything between them still looks
	// like front-matter; otherwise the block is unclosed and the
	// error stays contained to that one page (parseOnePage reports
	// it precisely). A fence followed by prose is a horizontal rule
	// and is ignored.
	openerStarts := make([]int, 0, len(fmIdx)/2+1)
	for i := 0; i < len(fmIdx); i++ {
		after := input[fmIdx[i][1]:]
		if !yamlKeyLineRe.MatchString(firstNonBlankLine(after)) {
			continue // horizontal rule / stray fence — not a page boundary
		}
		openerStarts = append(openerStarts, fmIdx[i][0])
		// Consume the matching closer when the span between reads as
		// front-matter, so the closer can't be misread as an opener.
		if i+1 < len(fmIdx) && plausibleFrontMatter(input[fmIdx[i][1]:fmIdx[i+1][0]]) {
			i++
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

// firstNonBlankLine returns the first line of s that contains a
// non-whitespace character ("" when none). Bounded scan — fence
// classification only ever needs the line right after the fence.
func firstNonBlankLine(s string) string {
	for _, line := range strings.SplitN(s, "\n", 64) {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

// plausibleFrontMatter reports whether span reads as the inside of a
// front-matter block: every non-blank line is a YAML key line, a
// list/continuation (`- …` or indented), or a comment. Used only to
// decide whether the fence after span is this block's closer — a
// false negative just means a missing-closer error surfaces on that
// page, so this errs strict rather than swallowing body text.
func plausibleFrontMatter(span string) bool {
	for _, line := range strings.Split(span, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if yamlKeyLineRe.MatchString(line) || strings.HasPrefix(t, "- ") ||
			strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		return false
	}
	return true
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

	// Validate + default the action field (V1.5 per C-AI-WORKSPACE-V1-G).
	// Empty defaults to "create" so V1-era AI prompts (no `action:`)
	// continue to parse with their previous semantics. Per-action
	// required-field validation lives below (after the name check).
	switch p.FrontMatter.Action {
	case "":
		p.FrontMatter.Action = ActionCreate
	case ActionCreate, ActionUpdate, ActionDelete:
		// ok
	default:
		p.Status = StatusParseError
		p.ParseError = fmt.Sprintf(
			"action: %q is not valid (must be one of create, update, delete)",
			p.FrontMatter.Action)
		return p
	}

	// Empty body is a soft warning, not an error. For Delete rows,
	// the body is ignored at commit; for Update / Create it would
	// produce an empty entity body which is a degraded but valid
	// state. The Delete warning surfaces an AI-prompt-quality issue
	// (wasted tokens) without blocking the commit.
	if strings.TrimSpace(p.Body) == "" {
		if p.FrontMatter.Action != ActionDelete {
			p.Warnings = append(p.Warnings, "Page body is empty")
		}
	} else if p.FrontMatter.Action == ActionDelete {
		p.Warnings = append(p.Warnings,
			"Body content is ignored for action: delete; the entity is removed by name")
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
//
// Error wording is operator-facing — kept human-readable; no raw
// library prefixes ("yaml:") reach the review screen. Technical
// detail (the underlying yaml.Unmarshal error) is preserved via the
// %v formatter for log-side debugging but stripped of its leading
// library tag.
func extractFrontMatter(s string) (body string, fm FrontMatter, rawYAML string, err error) {
	// Match: opening `---\n`, then yaml content, then `---\n` on its
	// own line. The opening fence is already known to start at
	// position 0 (caller checks HasPrefix).
	lines := strings.SplitN(s, "\n", 2)
	if len(lines) < 2 {
		return "", fm, "", fmt.Errorf(
			"front-matter block is missing its closing `---` line (the opening fence is on the only line)")
	}
	rest := lines[1]
	closeIdx := fmOpenRe.FindStringIndex(rest)
	if closeIdx == nil {
		return "", fm, "", fmt.Errorf(
			"front-matter block is missing its closing `---` line — add `---` on its own line after the YAML keys")
	}
	// If the span up to that fence doesn't read as front-matter, the
	// fence we found is a horizontal rule in the body and the REAL
	// closer is missing — say that precisely instead of letting the
	// YAML parser emit a confusing error about body prose.
	if !plausibleFrontMatter(rest[:closeIdx[0]]) {
		return "", fm, "", fmt.Errorf(
			"front-matter block is missing its closing `---` line (the next `---` found looks like a divider inside the body) — add `---` on its own line right after the YAML keys")
	}
	rawYAML = rest[:closeIdx[0]]
	body = strings.TrimSpace(rest[closeIdx[1]:])

	if err := yaml.Unmarshal([]byte(rawYAML), &fm); err != nil {
		return "", FrontMatter{}, "",
			fmt.Errorf("front-matter could not be read as YAML — %s",
				humanizeYAMLError(err))
	}
	return body, fm, rawYAML, nil
}

// humanizeYAMLError strips the `yaml:` prefix from a yaml.v3 error
// and surfaces a friendly hint when the error message matches common
// failure patterns (unindented mapping, unquoted colon-bearing
// value, etc.). Falls back to the cleaned message verbatim when no
// pattern matches.
func humanizeYAMLError(err error) string {
	msg := err.Error()
	// yaml.v3 always prefixes with "yaml:" — strip it so operator
	// doesn't see library jargon.
	msg = strings.TrimPrefix(msg, "yaml: ")
	msg = strings.TrimPrefix(msg, "yaml:")
	switch {
	case strings.Contains(msg, "mapping values are not allowed"):
		return msg + " (check that values containing `:` are quoted, e.g. `name: \"Some: Name\"`)"
	case strings.Contains(msg, "did not find expected"):
		return msg + " (check the indentation + that lists use `-` markers)"
	case strings.Contains(msg, "could not find expected ':'"):
		return msg + " (each key must be followed by `:` and a space before the value)"
	default:
		return msg
	}
}

// unknownYAMLKeys returns the top-level YAML keys that aren't in
// the FrontMatter schema. Used to populate the row's Warnings slice.
// Tolerant of comments + blank lines.
func unknownYAMLKeys(rawYAML string) []string {
	known := map[string]bool{
		"name": true, "type": true, "subcategory": true,
		"visibility": true, "tags": true, "description": true,
		// V1.5 (C-AI-WORKSPACE-V1-G) declarative action verb.
		"action": true,
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
