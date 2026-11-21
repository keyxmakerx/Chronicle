package systems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxRulesTerms bounds one answer so a runaway glossary cannot become a
// runaway response.
const maxRulesTerms = 2000

// RulesTerm is one rules word as the hero builder shows it: the name, the
// kind of word it is (condition, movement, ...) and a plain-text meaning.
type RulesTerm struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Text     string `json:"text"`
}

// RulesTerms are a campaign system's rules words, keyed the way the builder
// looks them up (NormalizeTermKey). Present is false when the system has no
// Rules book glossary, so a caller can leave out the feature entirely.
type RulesTerms struct {
	Terms   map[string]RulesTerm
	Present bool
}

var (
	termSpacePattern = regexp.MustCompile(`\s+`)
	// [[term|shown]] is the book's own term markup (book_index.go); the
	// glossary file can carry either it or {@category term|shown}.
	termBracketPattern = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	// Script and style bodies are not words at all; block tags and <br> are
	// where one sentence ends and the next begins, so they become a space and
	// inline tags (<b>, <em>) vanish without splitting a word from its dot.
	termHiddenPattern = regexp.MustCompile(`(?is)<(script|style)\b.*?</(script|style)\s*>`)
	termBlockPattern  = regexp.MustCompile(`(?i)</?(p|br|div|li|ul|ol|h[1-6]|tr|td|th|table|blockquote)\b[^>]*>`)
	termTagPattern    = regexp.MustCompile(`<[^>]*>`)
	termEmphasis      = strings.NewReplacer("**", "", "__", "", "`", "")
)

// NormalizeTermKey is the one rule both ends use to match a word: the term as
// written in {@category term|label} markup, trimmed, lowercased, runs of
// whitespace turned into "-". The builder applies the same rule to the markup
// it meets, so the two never disagree about "Save Ends" and "save-ends".
func NormalizeTermKey(term string) string {
	return termSpacePattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(term)), "-")
}

// PlainTermText turns a glossary text into what a tooltip can print verbatim:
// reference markup becomes its shown words, tags and entities are dropped, and
// whitespace is collapsed. Nothing downstream renders it as HTML, so this is
// for legibility, not the safety boundary.
func PlainTermText(s string) string {
	s = flattenRefMarkup(s)
	s = termBracketPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := termBracketPattern.FindStringSubmatch(m)
		if sub[2] != "" {
			return sub[2]
		}
		return sub[1]
	})
	s = termHiddenPattern.ReplaceAllString(s, " ")
	s = termBlockPattern.ReplaceAllString(s, " ")
	s = termTagPattern.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = termEmphasis.Replace(s)
	return strings.TrimSpace(termSpacePattern.ReplaceAllString(s, " "))
}

// LoadRulesTerms reads the glossary the system's Rules book names (the
// book.yaml `glossary:` data file). A system with no book or no glossary
// answers Present=false with no error; a glossary that is declared but broken
// is an error, as it is for the book itself. Entries in a Director-only
// category are left out: the builder serves players, and a hover word must
// never carry what the Director keeps back.
func LoadRulesTerms(sysDir string, m *SystemManifest) (*RulesTerms, error) {
	out := &RulesTerms{Terms: map[string]RulesTerm{}}
	if !HasBook(sysDir) {
		return out, nil
	}
	var idx bookIndexYAML
	if err := decodeBookFile(filepath.Join(sysDir, bookDirName, bookIndexFile), &idx); err != nil {
		return nil, fmt.Errorf("%s: %w", bookIndexFile, err)
	}
	if idx.Glossary == "" {
		return out, nil
	}
	entries, err := readGlossaryEntries(sysDir, idx.Glossary)
	if err != nil {
		return nil, fmt.Errorf("%s: glossary: %w", bookIndexFile, err)
	}
	out.Present = true
	for _, e := range entries {
		name := strings.TrimSpace(e.Name)
		text := e.Summary
		if text == "" {
			text = e.Description
		}
		text = PlainTermText(text)
		if name == "" || text == "" {
			continue
		}
		cat := strings.ToLower(strings.TrimSpace(e.Category))
		if cat == "" {
			cat = "term"
		}
		if m != nil && m.DirectorOnly(cat) {
			continue
		}
		t := RulesTerm{Name: name, Category: cat, Text: text}
		// The slug is how the data names the word, which may differ from its
		// printed name; a name always wins a clash.
		if slug := NormalizeTermKey(e.Slug); slug != "" {
			if _, taken := out.Terms[slug]; !taken {
				out.Terms[slug] = t
			}
		}
		out.Terms[NormalizeTermKey(name)] = t
		if len(out.Terms) >= maxRulesTerms {
			break
		}
	}
	return out, nil
}

// glossaryEntry is one row of a glossary data file.
type glossaryEntry struct {
	Slug        string
	Name        string
	Summary     string
	Description string
	Category    string
}

// UnmarshalJSON reads properties.category only when it is text: a row whose
// properties hold other shapes must not fail the whole file.
func (g *glossaryEntry) UnmarshalJSON(b []byte) error {
	var raw struct {
		Slug        string          `json:"slug"`
		Name        string          `json:"name"`
		Summary     string          `json:"summary"`
		Description string          `json:"description"`
		Properties  json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	g.Slug, g.Name, g.Summary, g.Description = raw.Slug, raw.Name, raw.Summary, raw.Description
	var props map[string]any
	if json.Unmarshal(raw.Properties, &props) == nil {
		g.Category, _ = props["category"].(string)
	}
	return nil
}

// readGlossaryEntries reads a data/<file> glossary list.
func readGlossaryEntries(sysDir, file string) ([]glossaryEntry, error) {
	if !systemDataFilePattern.MatchString(file) {
		return nil, fmt.Errorf("%q is not a data file name", file)
	}
	data, err := os.ReadFile(filepath.Join(sysDir, "data", file))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("data/%s not found", file)
		}
		return nil, fmt.Errorf("data/%s could not be read", file)
	}
	var entries []glossaryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("data/%s is not a list of entries", file)
	}
	return entries, nil
}

// RulesTerms reads the rules words of the system the hero creator builds
// from, the same system CreatorPlan picks.
func (s *characterChoiceService) RulesTerms(ctx context.Context, campaignID string) (*RulesTerms, error) {
	for _, c := range s.candidates(ctx, campaignID) {
		if characterPresetOf(c.manifest) == nil {
			continue
		}
		return LoadRulesTerms(c.dir, c.manifest)
	}
	return &RulesTerms{Terms: map[string]RulesTerm{}}, nil
}
