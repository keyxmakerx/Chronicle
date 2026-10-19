package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// KindPart is the block an AI writes at the top of each paste when its answer
// comes in several, so Chronicle knows which earlier paste this one relies on.
const KindPart = "part"

// Part is one `kind: part` label: part Number of the answer called Name, and
// the earlier parts it needs. Every block after the label, up to the next
// label, belongs to it.
type Part struct {
	Name   string
	Key    string // Name as a slug: parts of one answer match on it
	Number int
	Of     int // how many parts the AI said there are; 0 when it didn't say
	Needs  []int
	Block  int // 1-based position of the label in the paste
}

// Title names the part for people: `part 2 of "Lastwell fixes"`.
func (p Part) Title() string {
	return fmt.Sprintf("part %d of %q", p.Number, p.Name)
}

// IsPart reports a `kind: part` label block.
func (p ParsedPage) IsPart() bool {
	return strings.EqualFold(strings.TrimSpace(p.FrontMatter.Kind), KindPart) && p.Status != StatusParseError
}

// Labelled is a paste with its part labels taken out. Blocks keeps every
// other block in paste order; Owner maps a block's SourceIndex to the index
// in Parts of the label it sits under (blocks above every label are absent).
type Labelled struct {
	Blocks []ParsedPage
	Parts  []Part
	Owner  map[int]int
}

// PartOf is the index in Parts of the label the block at sourceIndex sits
// under, if any.
func (l Labelled) PartOf(sourceIndex int) (int, bool) {
	i, ok := l.Owner[sourceIndex]
	return i, ok
}

// TakeParts takes the part labels out of a parsed paste. A label that can't
// be read becomes a parse error in its place, so the review lists it with
// the other unusable blocks, and the blocks under it count as unlabelled.
func TakeParts(all []ParsedPage) Labelled {
	out := Labelled{Owner: map[int]int{}}
	cur := -1
	for _, p := range all {
		if !p.IsPart() {
			out.Blocks = append(out.Blocks, p)
			if cur >= 0 {
				out.Owner[p.SourceIndex] = cur
			}
			continue
		}
		part, err := readPart(p)
		if err != "" {
			p.Status, p.ParseError = StatusParseError, err
			out.Blocks = append(out.Blocks, p)
			cur = -1
			continue
		}
		out.Parts = append(out.Parts, part)
		cur = len(out.Parts) - 1
	}
	return out
}

// numRe finds the whole numbers in a loosely written value such as
// "2 of 3" or "parts 1 and 2".
var numRe = regexp.MustCompile(`\d+`)

// readPart reads one label, or says in plain words what is wrong with it.
func readPart(p ParsedPage) (Part, string) {
	part := Part{Name: strings.TrimSpace(p.Name), Block: p.SourceIndex + 1}
	// Slugify gives a fallback for an empty name, so check the name itself.
	if part.Name != "" {
		part.Key = entities.Slugify(part.Name)
	}
	if part.Key == "" {
		return part, "A part block needs `name:`, the same name on every part of one answer."
	}
	nums := numbers(p.Fields["part"])
	if len(nums) == 0 || nums[0] < 1 {
		return part, fmt.Sprintf("The part block for %q needs `part:`, the part's number counting from 1.", part.Name)
	}
	part.Number = nums[0]
	// "part: 2 of 3" carries the count too.
	if len(nums) > 1 {
		part.Of = nums[1]
	}
	if of := numbers(p.Fields["of"]); len(of) > 0 {
		part.Of = of[0]
	}
	if part.Of > 0 && part.Of < part.Number {
		return part, fmt.Sprintf("The part block for %q says part %d of %d.", part.Name, part.Number, part.Of)
	}
	seen := map[int]bool{}
	for _, n := range numbers(p.Fields["needs"]) {
		if n < 1 || n >= part.Number {
			return part, fmt.Sprintf("Part %d of %q can only need earlier parts, not part %d.", part.Number, part.Name, n)
		}
		if !seen[n] {
			seen[n] = true
			part.Needs = append(part.Needs, n)
		}
	}
	sort.Ints(part.Needs)
	return part, ""
}

// numbers reads the whole numbers out of a YAML value: a number, a list of
// numbers, or text with numbers in it.
func numbers(v any) []int {
	switch t := v.(type) {
	case nil:
		return nil
	case int:
		return []int{t}
	case float64:
		return []int{int(t)}
	case []any:
		var out []int
		for _, e := range t {
			out = append(out, numbers(e)...)
		}
		return out
	}
	var out []int
	for _, m := range numRe.FindAllString(fmt.Sprint(v), -1) {
		if n, err := strconv.Atoi(m); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// OrderParts puts the parts of a paste in order when several were pasted
// together out of order: each answer's parts by number, answers in the order
// they first appear, unlabelled blocks above them all. It returns the paste
// rebuilt, and false when it was already in order (or has fewer than two
// labels), so the caller keeps the operator's own text.
func OrderParts(all []ParsedPage) (string, bool) {
	type section struct {
		key    string
		number int
		first  int // order the answer first appears in
		pos    int
		raws   []string
	}
	var head []string
	var secs []*section
	firstSeen := map[string]int{}
	var cur *section
	for _, p := range all {
		if p.IsPart() {
			if part, err := readPart(p); err == "" {
				if _, ok := firstSeen[part.Key]; !ok {
					firstSeen[part.Key] = len(firstSeen)
				}
				cur = &section{key: part.Key, number: part.Number, first: firstSeen[part.Key], pos: len(secs)}
				secs = append(secs, cur)
				cur.raws = append(cur.raws, p.Raw)
				continue
			}
		}
		if cur == nil {
			head = append(head, p.Raw)
		} else {
			cur.raws = append(cur.raws, p.Raw)
		}
	}
	if len(secs) < 2 {
		return "", false
	}
	sorted := append([]*section(nil), secs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].first != sorted[j].first {
			return sorted[i].first < sorted[j].first
		}
		return sorted[i].number < sorted[j].number
	})
	moved := false
	for i := range sorted {
		if sorted[i] != secs[i] {
			moved = true
		}
	}
	if !moved {
		return "", false
	}
	raws := head
	for _, s := range sorted {
		raws = append(raws, s.raws...)
	}
	return strings.Join(raws, "\n\n"), true
}

// MissingNeeds lists the parts p needs that are neither applied in the
// campaign nor earlier in this paste.
func MissingNeeds(p Part, applied map[int]bool, inPaste map[int]bool) []int {
	var out []int
	for _, n := range p.Needs {
		if !applied[n] && !inPaste[n] {
			out = append(out, n)
		}
	}
	return out
}

// PartList is "part 2" or "parts 1 and 2", for messages.
func PartList(nums []int) string {
	if len(nums) == 1 {
		return "part " + strconv.Itoa(nums[0])
	}
	words := make([]string, len(nums))
	for i, n := range nums {
		words[i] = strconv.Itoa(n)
	}
	return "parts " + strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// BlockHash is a block's fingerprint for spotting a paste applied before:
// SHA-256 of its text with line endings and trailing spaces evened out, so a
// copy that went through another editor still matches.
func BlockHash(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.Join(lines, "\n"))))
	return hex.EncodeToString(sum[:])
}

// AnswerMissing lists the parts an answer still lacks: every part any present
// part needs, and every part up to the count when the AI gave one, that is
// neither applied nor present (pasted now or waiting in the tray).
func AnswerMissing(present []Part, applied map[int]bool) []int {
	have := map[int]bool{}
	of := 0
	for _, p := range present {
		have[p.Number] = true
		of = max(of, p.Of)
	}
	want := map[int]bool{}
	for n := 1; n <= of; n++ {
		want[n] = true
	}
	for _, p := range present {
		for _, n := range p.Needs {
			want[n] = true
		}
	}
	var out []int
	for n := range want {
		if !have[n] && !applied[n] {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// PartTexts cuts a labelled paste into one text per label, label first, in
// the order of Parts. Blocks above the first label go with the first part.
func PartTexts(all []ParsedPage) []string {
	var out []string
	var head []string
	for _, p := range all {
		if p.IsPart() {
			if _, err := readPart(p); err == "" {
				out = append(out, p.Raw)
				if len(out) == 1 && len(head) > 0 {
					out[0] = strings.Join(append([]string{p.Raw}, head...), "\n\n")
				}
				continue
			}
		}
		if len(out) == 0 {
			head = append(head, p.Raw)
		} else {
			out[len(out)-1] += "\n\n" + p.Raw
		}
	}
	return out
}

// StoredTray is a part waiting in the tray, with its text as pasted.
type StoredTray struct {
	Part     Part
	Markdown string
}
