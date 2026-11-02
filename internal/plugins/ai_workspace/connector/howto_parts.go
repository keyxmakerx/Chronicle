package connector

import (
	"fmt"
	"regexp"
	"strings"
)

// The owner's instructions run to tens of thousands of characters, more than
// an AI app reads in one tool answer. how_to_write_changes answers them in
// parts: the guide itself, with the sheet fields and the block formats
// replaced by an index of where to read them, then each category's fields
// on request. Each kind's format is block_format's, word for word.

// The two sections of the copy-and-paste prompt that move out of the guide.
const (
	headingCategories = "My campaign's entity types"
	headingBlocks     = "Other things you can add, change or remove"
)

// howToPartCategories is the part argument that reads the sheet fields.
const howToPartCategories = "categories"

// howToWholeLimit is the longest text answered whole: small enough to read in
// one go, so a short guide (a player's) is never split.
const howToWholeLimit = 24000

// categoryLine is a category's line in the entity types list:
// "- **Creature** (slug: `creature`)".
var categoryLine = regexp.MustCompile(`^- \*\*(.+?)\*\* \(slug: `)

// mdSection is one "## " section of the prompt: its heading ("" for the text
// before the first) and its whole text, heading line included.
type mdSection struct{ heading, text string }

func splitSections(text string) []mdSection {
	var out []mdSection
	cur := mdSection{}
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			out = append(out, cur)
			cur = mdSection{heading: strings.TrimSpace(strings.TrimPrefix(line, "## "))}
		}
		cur.text += line
	}
	return append(out, cur)
}

// categoryBlocks splits the entity types section into its opening, one
// block per category and the closing rules after the list.
func categoryBlocks(section string) (head string, cats []mdSection, tail string) {
	lines := strings.SplitAfter(section, "\n")
	i := 0
	for ; i < len(lines) && !categoryLine.MatchString(lines[i]); i++ {
		head += lines[i]
	}
	for i < len(lines) {
		m := categoryLine.FindStringSubmatch(lines[i])
		if m == nil {
			break
		}
		c := mdSection{heading: m[1], text: lines[i]}
		i++
		for ; i < len(lines) && (strings.HasPrefix(lines[i], "  ") || strings.TrimSpace(lines[i]) == ""); i++ {
			c.text += lines[i]
		}
		cats = append(cats, c)
	}
	for ; i < len(lines); i++ {
		tail += lines[i]
	}
	// The blank lines after the last category belong to the rules below it.
	if n := len(cats); n > 0 {
		cats[n-1].text = strings.TrimRight(cats[n-1].text, "\n") + "\n\n"
	}
	return head, cats, tail
}

// fieldCount is how many sheet fields a category block lists.
func fieldCount(block string) int {
	n := 0
	for _, l := range strings.Split(block, "\n") {
		if strings.HasPrefix(l, "  - ") {
			n++
		}
	}
	return n
}

// howToPart answers how_to_write_changes for part and category. ok is false
// with a reason the AI can act on when no such part exists.
func howToPart(text, part, category string) (string, bool, string) {
	part = strings.ToLower(strings.TrimSpace(part))
	if category != "" {
		part = howToPartCategories
	}
	if part != "" && part != howToPartCategories && part != "guide" {
		return "", false, fmt.Sprintf("there is no part %q; leave part out for the guide, or give part: %s", oneLine(part, 40), howToPartCategories)
	}
	sections := splitSections(text)
	var cats mdSection
	for _, s := range sections {
		if s.heading == headingCategories {
			cats = s
		}
	}
	if part == howToPartCategories {
		if cats.text == "" {
			return "", false, "this campaign's guide lists no categories"
		}
		if category == "" {
			return strings.TrimSpace(cats.text), true, ""
		}
		head, list, tail := categoryBlocks(cats.text)
		var names []string
		for _, c := range list {
			names = append(names, c.heading)
			if strings.EqualFold(c.heading, strings.TrimSpace(category)) {
				return strings.TrimSpace(head + c.text + tail), true, ""
			}
		}
		return "", false, fmt.Sprintf("there is no category called %q; the categories are %s", oneLine(category, 60), strings.Join(names, ", "))
	}
	if len(text) <= howToWholeLimit {
		return text, true, ""
	}
	var b strings.Builder
	for _, s := range sections {
		switch s.heading {
		case headingCategories:
			b.WriteString(categoriesIndex(s.text))
		case headingBlocks:
			b.WriteString(blocksIndex(s.text))
		default:
			b.WriteString(s.text)
		}
	}
	return b.String(), true, ""
}

// categoriesIndex stands in for the entity types section in the guide: the
// categories by name, and where to read their fields.
func categoriesIndex(section string) string {
	_, list, _ := categoryBlocks(section)
	var b strings.Builder
	b.WriteString("## " + headingCategories + "\n\n")
	b.WriteString("Before you write a page, read its category's sheet fields: call " + toolHowTo +
		" with `category` set to the category's name, or `part: " + howToPartCategories + "` for every category at once. " +
		"A page's sheet takes only those fields. The categories, with how many fields each has:\n\n")
	for _, c := range list {
		fmt.Fprintf(&b, "- %s (%d %s)\n", c.heading, fieldCount(c.text), plural(fieldCount(c.text), "field", "fields"))
	}
	b.WriteString("\n")
	return b.String()
}

// blocksIndex stands in for the block formats: the section's opening, then
// where each kind's format is read.
func blocksIndex(section string) string {
	intro := section
	if i := strings.Index(section, "\n### "); i >= 0 {
		intro = section[:i+1]
	}
	intro = strings.Replace(intro, "`kind:` (below)", "`kind:`", 1)
	return strings.TrimRight(intro, "\n") + "\n\nEach kind's keys, rules and an example are in block_format: call it with `block` set to the kind (the kinds are listed under \"What you can do\"). Read it before you write a kind's first block.\n\n"
}
