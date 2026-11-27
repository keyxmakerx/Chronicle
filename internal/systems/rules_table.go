package systems

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities/cardborders"
)

// "At the table" draws an entry's figures and lists the same way whether the
// entry is the package's or the campaign's. A package keeps a structured
// value beside a generated scalar twin (`<key>_display`) for consumers that
// can only print text; a campaign entry stores the structured value alone.
// So where the manifest names a twin and the entry holds the structured
// value, the structured value is drawn and the twin ignored. A campaign
// entry has no manifest of its own: its remaining details are drawn the
// same way after the category's fields.

// Detail kinds, in the order a reader meets them.
const (
	detailFigures = "figures" // short scalars, inline: "Size 1M  Speed 5"
	detailText    = "text"    // a longer scalar, as a labelled paragraph
	detailWords   = "words"   // a list of words, as a comma line
	detailItems   = "items"   // named items, as hairline rows
	detailParts   = "parts"   // an object without a name, as its readable parts
)

// maxFigureRunes is the longest scalar still drawn inline as a figure.
const maxFigureRunes = 32

// textRun is a span of prose; Bold marks **text** in package prose.
type textRun struct {
	Text string
	Bold bool
}

// para is one paragraph of runs.
type para []textRun

// figure is one labelled value.
type figure struct {
	Label string
	Value string
}

// part is one labelled piece of an object, drawn as prose.
type part struct {
	Label string
	Text  []para
}

// nestedObject is an object inside an item, such as a trait's ability,
// drawn under the item's own text like a kit's ability: its name as a small
// heading unless it repeats the item's, its flavor as the lead line, then
// its parts as label and value rows.
type nestedObject struct {
	Name  string
	Lead  []para
	Parts []part
}

// tableItem is one named row: a trait, a feature, an ability.
type tableItem struct {
	Name   string
	Anchor string
	// Cost and CostShort are "2 points" and "2"; empty when the item has none.
	Cost      string
	CostShort string
	// Lead is the item's flavor, read before its text in the quiet colour.
	Lead   []para
	Text   []para
	Parts  []part
	Nested []nestedObject
	// Held marks a trait the peek's hero holds; NoLongerOffered a trait of
	// a left-out entry its reader's hero still has.
	Held            bool
	NoLongerOffered bool
}

// tableDetail is one block of "At the table".
type tableDetail struct {
	Kind    string
	Label   string
	Aside   string
	Figures []figure
	Text    []para
	Words   string
	Items   []tableItem
	Lead    []para
	Parts   []part
}

var (
	boldPattern   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	paraSplit     = regexp.MustCompile(`\n\s*\n|\r?\n`)
	htmlBlockLike = regexp.MustCompile(`(?i)<(p|div|br|ul|ol|li|strong|em|b|i|h[1-6]|table|blockquote|span)\b`)
)

// richParas turns package prose into paragraphs: reference markers become
// their display words and **text** becomes bold, so no raw marker ever
// reaches a reader. Nothing in the result is markup.
func richParas(s string) []para {
	s = strings.TrimSpace(flattenRefMarkup(s))
	if s == "" {
		return nil
	}
	var out []para
	for _, chunk := range paraSplit.Split(s, -1) {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		var p para
		last := 0
		for _, m := range boldPattern.FindAllStringSubmatchIndex(chunk, -1) {
			if m[0] > last {
				p = append(p, textRun{Text: chunk[last:m[0]]})
			}
			p = append(p, textRun{Text: chunk[m[2]:m[3]], Bold: true})
			last = m[1]
		}
		if last < len(chunk) {
			p = append(p, textRun{Text: chunk[last:]})
		}
		out = append(out, p)
	}
	return out
}

// looksLikeHTML says whether stored prose is markup rather than plain text.
func looksLikeHTML(s string) bool { return htmlBlockLike.MatchString(s) }

// sentenceCase keeps the first word and lowers the others that are plain
// capitalised words, so "Free Strike" reads "Free strike" and "EV" stays.
func sentenceCase(s string) string {
	words := strings.Fields(s)
	for i := 1; i < len(words); i++ {
		w := words[i]
		r, size := utf8.DecodeRuneInString(w)
		rest := w[size:]
		if unicode.IsUpper(r) && rest != "" && rest == strings.ToLower(rest) {
			words[i] = string(unicode.ToLower(r)) + rest
		}
	}
	return strings.Join(words, " ")
}

// humanKey names a stored key for a reader: "purchased_traits" reads
// "Purchased traits", "tier1" reads "Tier 1".
func humanKey(k string) string {
	k = strings.TrimSuffix(k, "_display")
	var b strings.Builder
	prevDigit := false
	for i, r := range strings.ReplaceAll(k, "_", " ") {
		isDigit := unicode.IsDigit(r)
		if isDigit && !prevDigit && i > 0 && b.Len() > 0 && !strings.HasSuffix(b.String(), " ") {
			b.WriteByte(' ')
		}
		prevDigit = isDigit
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(out)
	return string(unicode.ToUpper(r)) + out[size:]
}

// fieldLabel is the label a manifest field gives, in sentence case, with a
// twin's "display" dropped.
func fieldLabel(f FieldDef) string {
	l := strings.TrimSpace(f.Label)
	if l == "" {
		return humanKey(f.Key)
	}
	l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(l, " Display"), " display"))
	return sentenceCase(l)
}

// singular names one of a plural label: "Signature traits" with one item
// reads "Signature trait".
func singular(label string) string {
	if strings.HasSuffix(label, "ss") || !strings.HasSuffix(label, "s") {
		return label
	}
	return strings.TrimSuffix(label, "s")
}

// scalarText renders a scalar for a reader; ok is false for anything else.
func scalarText(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(flattenRefMarkup(t)), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case int:
		return strconv.Itoa(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case json.Number:
		return t.String(), true
	case bool:
		if t {
			return "Yes", true
		}
		return "No", true
	}
	return "", false
}

// buyBudget is the points a hero spends on an entry's list: the first
// positive number under a key ending in "_points", as the hero creator reads it.
func buyBudget(props map[string]any) (string, float64) {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if n, ok := props[k].(float64); ok && n > 0 && strings.HasSuffix(k, "_points") {
			return k, n
		}
	}
	return "", 0
}

func points(n float64) string {
	s := strconv.FormatFloat(n, 'f', -1, 64)
	if n == 1 {
		return s + " point"
	}
	return s + " points"
}

// namedItems reads a list of objects that each carry a name; ok is false
// when the value is not such a list.
func namedItems(v any) ([]map[string]any, bool) {
	list, isList := v.([]any)
	if !isList || len(list) == 0 {
		return nil, false
	}
	out := make([]map[string]any, 0, len(list))
	for _, it := range list {
		m, isObj := it.(map[string]any)
		if !isObj {
			return nil, false
		}
		if name, _ := m["name"].(string); strings.TrimSpace(name) == "" {
			return nil, false
		}
		out = append(out, m)
	}
	return out, true
}

// wordList reads a list of scalars as one comma line.
func wordList(v any) (string, bool) {
	list, isList := v.([]any)
	if !isList || len(list) == 0 {
		return "", false
	}
	words := make([]string, 0, len(list))
	for _, it := range list {
		s, ok := scalarText(it)
		if !ok {
			return "", false
		}
		if s != "" {
			words = append(words, s)
		}
	}
	return strings.Join(words, ", "), len(words) > 0
}

// partOrder is the order an object's common parts read in; any other key
// follows alphabetically.
var partOrder = []string{"flavor", "type", "action_type", "cost", "keywords", "distance", "target",
	"trigger", "power_roll", "tier1", "tier2", "tier3", "effect", "special"}

// textKeys hold an item's own prose, read as its text rather than a part.
var textKeys = []string{"description", "text"}

func orderedKeys(m map[string]any, skip map[string]bool) []string {
	rank := map[string]int{}
	for i, k := range partOrder {
		rank[k] = i + 1
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if !skip[k] {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(a, b int) bool {
		ra, rb := rank[keys[a]], rank[keys[b]]
		switch {
		case ra != 0 && rb != 0:
			return ra < rb
		case ra != 0:
			return true
		case rb != 0:
			return false
		}
		return keys[a] < keys[b]
	})
	return keys
}

// readable renders any value as prose: a scalar as itself, a list of words
// as a comma line, an object as "Label: value; ..." and named items by name.
func readable(v any) string {
	if s, ok := scalarText(v); ok {
		return s
	}
	if s, ok := wordList(v); ok {
		return s
	}
	if items, ok := namedItems(v); ok {
		names := make([]string, 0, len(items))
		for _, it := range items {
			names = append(names, strings.TrimSpace(it["name"].(string)))
		}
		return strings.Join(names, ", ")
	}
	if m, ok := v.(map[string]any); ok {
		var bits []string
		for _, k := range orderedKeys(m, nil) {
			if s := readable(m[k]); s != "" {
				bits = append(bits, humanKey(k)+": "+s)
			}
		}
		return strings.Join(bits, "; ")
	}
	return ""
}

func objectParts(m map[string]any, skip map[string]bool) []part {
	var out []part
	for _, k := range orderedKeys(m, skip) {
		if s := readable(m[k]); s != "" {
			out = append(out, part{Label: humanKey(k), Text: richParas(s)})
		}
	}
	return out
}

// leadOf takes an object's flavor as its lead line and marks it skipped.
func leadOf(m map[string]any, skip map[string]bool) []para {
	s, ok := m["flavor"].(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	skip["flavor"] = true
	return richParas(s)
}

// itemRow turns one named object into a row; anchorPrefix keeps a peek's
// anchors apart from its page's.
func itemRow(m map[string]any, anchorPrefix string) tableItem {
	name := strings.TrimSpace(m["name"].(string))
	row := tableItem{Name: flattenRefMarkup(name), Anchor: anchorPrefix + slugFromName(name)}
	skip := map[string]bool{"name": true, "cost": true}
	if c, ok := m["cost"].(float64); ok {
		row.Cost, row.CostShort = points(c), strconv.FormatFloat(c, 'f', -1, 64)
	}
	row.Lead = leadOf(m, skip)
	for _, k := range textKeys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			row.Text = richParas(s)
			skip[k] = true
			break
		}
	}
	// An object inside the item is drawn as its own block, not one prose run.
	for _, k := range orderedKeys(m, skip) {
		obj, isObj := m[k].(map[string]any)
		if !isObj || len(obj) == 0 {
			continue
		}
		skip[k] = true
		inner := map[string]bool{"name": true}
		n := nestedObject{Lead: leadOf(obj, inner)}
		if on, _ := obj["name"].(string); strings.TrimSpace(on) != "" &&
			!strings.EqualFold(strings.TrimSpace(flattenRefMarkup(on)), row.Name) {
			n.Name = flattenRefMarkup(strings.TrimSpace(on))
		}
		n.Parts = objectParts(obj, inner)
		if n.Name != "" || len(n.Lead) > 0 || len(n.Parts) > 0 {
			row.Nested = append(row.Nested, n)
		}
	}
	row.Parts = objectParts(m, skip)
	return row
}

// tableKey is one key "At the table" draws, with its label.
type tableKey struct {
	key   string
	label string
}

// twinText is a field's one-line text where the manifest names a generated
// twin (`<key>_display`) the entry does not carry, as a campaign entry never
// does: the structured base read as prose, by the rule the table draws an
// object or a list with. ok is false when key is not a twin, the entry has
// the twin, or it has no base either.
func twinText(props map[string]any, key string) (string, bool) {
	base := strings.TrimSuffix(key, "_display")
	if base == key {
		return "", false
	}
	if v, has := props[key]; has && v != nil {
		return "", false
	}
	v, has := props[base]
	if !has || v == nil {
		return "", false
	}
	return readable(v), true
}

// tableKeys decides what to draw: the category's fields in order, each
// twin swapped for its structured base when the entry holds one, then, for
// a campaign entry, every remaining detail.
// notAFigure is the properties a campaign entry can carry that say how it is
// drawn or where its words came from, never something said at the table: the
// card look, and the provenance parts the credit folds in. A printed entry
// never lists them because its manifest fields decide what shows.
var notAFigure = map[string]bool{
	cardborders.PropBorder: true, cardborders.PropColour: true,
	"custom_fields": true, "derived_from": true, "omissions": true,
}

func tableKeys(cat *CategoryDef, props map[string]any, campaign bool) []tableKey {
	var out []tableKey
	used := map[string]bool{}
	if cat != nil {
		for _, f := range cat.Fields {
			key := f.Key
			if base := strings.TrimSuffix(key, "_display"); base != key {
				if _, structured := props[base]; structured {
					key = base
				}
			}
			if used[key] {
				continue
			}
			used[key], used[f.Key] = true, true
			out = append(out, tableKey{key: key, label: fieldLabel(f)})
		}
	}
	if !campaign {
		return out
	}
	rest := make([]string, 0, len(props))
	for k := range props {
		if used[k] || notAFigure[k] {
			continue
		}
		// A twin is never drawn beside its own base.
		if base := strings.TrimSuffix(k, "_display"); base != k {
			if _, has := props[base]; has {
				continue
			}
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		out = append(out, tableKey{key: k, label: humanKey(k)})
	}
	return out
}

// buildTable draws "At the table" for an entry. Short scalars gather into
// one line of figures at the top; everything else follows in order.
func buildTable(cat *CategoryDef, props map[string]any, campaign bool, anchorPrefix string) []tableDetail {
	if len(props) == 0 {
		return nil
	}
	budgetKey, budget := buyBudget(props)
	figs := tableDetail{Kind: detailFigures}
	var rest []tableDetail
	for _, tk := range tableKeys(cat, props, campaign) {
		// Where the words came from is the credit's job, not a figure.
		if strings.TrimSuffix(tk.key, "_display") == "provenance" {
			continue
		}
		v, ok := props[tk.key]
		if !ok || v == nil {
			continue
		}
		if s, isScalar := scalarText(v); isScalar {
			if s == "" {
				continue
			}
			label := tk.label
			if tk.key == budgetKey {
				label = "Points to spend"
			}
			if utf8.RuneCountInString(s) <= maxFigureRunes && !strings.Contains(s, "\n") {
				figs.Figures = append(figs.Figures, figure{Label: label, Value: s})
			} else {
				rest = append(rest, tableDetail{Kind: detailText, Label: label, Text: richParas(s)})
			}
			continue
		}
		if items, isItems := namedItems(v); isItems {
			d := tableDetail{Kind: detailItems, Label: tk.label}
			costed := budget > 0
			for _, it := range items {
				row := itemRow(it, anchorPrefix)
				if row.Cost == "" {
					costed = false
				}
				d.Items = append(d.Items, row)
			}
			if costed {
				if strings.Contains(strings.ToLower(tk.key), "trait") {
					d.Label = "Traits to buy"
				}
				d.Aside = points(budget) + " to spend"
			} else if len(d.Items) == 1 {
				d.Label = singular(d.Label)
			}
			rest = append(rest, d)
			continue
		}
		if words, isWords := wordList(v); isWords {
			rest = append(rest, tableDetail{Kind: detailWords, Label: tk.label, Words: words})
			continue
		}
		if m, isObj := v.(map[string]any); isObj && len(m) > 0 {
			if name, _ := m["name"].(string); strings.TrimSpace(name) != "" {
				rest = append(rest, tableDetail{Kind: detailItems, Label: tk.label, Items: []tableItem{itemRow(m, anchorPrefix)}})
			} else {
				skip := map[string]bool{}
				lead := leadOf(m, skip)
				rest = append(rest, tableDetail{Kind: detailParts, Label: tk.label, Lead: lead, Parts: objectParts(m, skip)})
			}
		}
	}
	if len(figs.Figures) == 0 {
		return rest
	}
	return append([]tableDetail{figs}, rest...)
}

// markTraits marks the rows whose names are in held (any casing) and
// returns the held names in page order.
func markTraits(table []tableDetail, held []string, mark func(*tableItem)) []tableItem {
	want := map[string]bool{}
	for _, h := range held {
		want[strings.ToLower(strings.TrimSpace(h))] = true
	}
	var out []tableItem
	for i := range table {
		for j := range table[i].Items {
			it := &table[i].Items[j]
			if want[strings.ToLower(it.Name)] {
				mark(it)
				out = append(out, *it)
			}
		}
	}
	return out
}

// itemNames lists every named item an entry carries, for search.
func itemNames(props map[string]any) []string {
	var out []string
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if items, ok := namedItems(props[k]); ok {
			for _, it := range items {
				out = append(out, strings.TrimSpace(fmt.Sprint(it["name"])))
			}
		}
	}
	return out
}
