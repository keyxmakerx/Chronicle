package vault_import

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// FrontMatter carries the keys Chronicle's AI-workspace import reads from a
// page's YAML header, under the same names. Obsidian properties that mean
// nothing to Chronicle (cssclasses, dates, aliases) are ignored without noise.
type FrontMatter struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"`
	Subcategory string   `yaml:"subcategory"`
	Visibility  string   `yaml:"visibility"`
	Tags        []string `yaml:"tags"`
	Description string   `yaml:"description"`
}

// maxFrontMatterBytes keeps a runaway header from being handed to the YAML
// parser; real property blocks are a few hundred bytes.
const maxFrontMatterBytes = 64 << 10

// SplitFrontMatter separates a leading `---` YAML block from a note's body.
// It follows Obsidian: the block must start on the very first line, and ends at
// the next `---` (or `...`) line. A note without one, or with one that is not
// closed, is all body. err is set when a block was found but is not valid YAML;
// the body is still returned, header removed, so the text is never lost.
func SplitFrontMatter(src string) (fm FrontMatter, body string, hadBlock bool, err error) {
	src = strings.TrimPrefix(src, "\xef\xbb\xbf")
	norm := strings.ReplaceAll(src, "\r\n", "\n")
	if !strings.HasPrefix(norm, "---\n") {
		return fm, norm, false, nil
	}
	rest := norm[len("---\n"):]
	end := -1
	pos := 0
	for pos <= len(rest) {
		nl := strings.IndexByte(rest[pos:], '\n')
		line := rest[pos:]
		next := len(rest) + 1
		if nl >= 0 {
			line = rest[pos : pos+nl]
			next = pos + nl + 1
		}
		if t := strings.TrimRight(line, " \t"); t == "---" || t == "..." {
			end = pos
			body = ""
			if next <= len(rest) {
				body = rest[next:]
			}
			break
		}
		pos = next
	}
	if end < 0 || end > maxFrontMatterBytes {
		return fm, norm, false, nil
	}
	raw := rest[:end]
	body = strings.TrimLeft(body, "\n")

	var loose map[string]any
	if yerr := yaml.Unmarshal([]byte(raw), &loose); yerr != nil {
		return fm, body, true, yerr
	}
	fm = frontMatterFrom(loose)
	return fm, body, true, nil
}

// frontMatterFrom reads the known keys leniently: Obsidian writes tags as a
// list, a comma string or a single word, and numbers or dates where a name is
// expected, none of which should reject the note.
func frontMatterFrom(m map[string]any) FrontMatter {
	var fm FrontMatter
	str := func(k string) string {
		v, _ := m[k].(string)
		return strings.TrimSpace(v)
	}
	fm.Name = str("name")
	if fm.Name == "" {
		fm.Name = str("title")
	}
	fm.Type = strings.ToLower(str("type"))
	fm.Subcategory = str("subcategory")
	fm.Visibility = strings.ToLower(str("visibility"))
	fm.Description = str("description")
	switch v := m["tags"].(type) {
	case string:
		for _, t := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
			fm.Tags = append(fm.Tags, strings.TrimPrefix(t, "#"))
		}
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				fm.Tags = append(fm.Tags, strings.TrimPrefix(strings.TrimSpace(s), "#"))
			}
		}
	}
	return fm
}
