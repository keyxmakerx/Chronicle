// copies.go is what single copies of an item remember: marks ("moonlit"), a
// quality (fine or poor), who made or harvested them and when, and when they
// go off. A holder's count of an item stays one number; a Copy is part of it
// that remembers something, and the rest of the count is plain. Plain copies
// are always used, given and moved first, unless someone picks a remembered
// one, so a hero never loses a fine potion to a careless move.
package armory

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Where a copy is held.
const (
	HolderCharacter = EndpointCharacter
	HolderStash     = EndpointStash
)

// Copy limits.
const (
	maxCopyMarks = 6
	maxCopyMark  = 40
	maxCopyWords = 80
)

// Quality words a copy may carry.
const (
	QualityFine = "fine"
	QualityPoor = "poor"
)

// Memory is what one copy remembers. Every field is optional; a Memory with
// nothing in it is a plain copy and is never stored.
type Memory struct {
	Marks   []string `json:"marks,omitempty"`
	Quality string   `json:"quality,omitempty"`
	// Quirk is the words of a quirk the copy came out with.
	Quirk string `json:"quirk,omitempty"`
	// How is "made" or "harvested"; By and On name who and when, in words.
	How string `json:"how,omitempty"`
	By  string `json:"by,omitempty"`
	On  string `json:"on,omitempty"`
	// OffDay is the calendar day (its absolute count) it goes off, 0 never;
	// OffOn names it in words, and Becomes is the item it turns into then.
	OffDay      int64  `json:"offDay,omitempty"`
	OffOn       string `json:"offOn,omitempty"`
	Becomes     string `json:"becomes,omitempty"`
	BecomesName string `json:"becomesName,omitempty"`
	// Off is set at the respite after OffDay for a copy that becomes
	// nothing: it stays in the pack, marked gone off.
	Off bool `json:"off,omitempty"`
}

// Copy is a number of copies of one item, at one holder, that remember the
// same thing.
type Copy struct {
	ID       int64
	ItemID   string
	Quantity int
	Memory   Memory
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Clean keeps what a copy may remember within its limits: marks are
// trimmed, deduplicated case-blind and capped, and an unknown quality or
// way of coming by it is dropped.
func (m Memory) Clean() Memory {
	out := Memory{}
	seen := map[string]bool{}
	for _, k := range m.Marks {
		k = clip(k, maxCopyMark)
		if k == "" || seen[strings.ToLower(k)] || len(out.Marks) >= maxCopyMarks {
			continue
		}
		seen[strings.ToLower(k)] = true
		out.Marks = append(out.Marks, k)
	}
	if m.Quality == QualityFine || m.Quality == QualityPoor {
		out.Quality = m.Quality
	}
	out.Quirk = clip(m.Quirk, maxCopyWords)
	if m.How == "made" || m.How == "harvested" {
		out.How = m.How
		out.By, out.On = clip(m.By, maxCopyWords), clip(m.On, maxCopyWords)
	}
	if m.OffDay > 0 {
		out.OffDay, out.OffOn, out.Becomes = m.OffDay, clip(m.OffOn, maxCopyWords), strings.TrimSpace(m.Becomes)
		if out.Becomes != "" {
			out.BecomesName = clip(m.BecomesName, maxCopyWords)
		}
		out.Off = m.Off
	}
	return out
}

// Empty reports a memory with nothing in it: a plain copy.
func (m Memory) Empty() bool {
	return len(m.Marks) == 0 && m.Quality == "" && m.Quirk == "" && m.How == "" && m.OffDay == 0
}

// HasMark reports whether the copy carries mark, case-blind.
func (m Memory) HasMark(mark string) bool {
	for _, k := range m.Marks {
		if strings.EqualFold(k, strings.TrimSpace(mark)) {
			return true
		}
	}
	return false
}

// encodeMemory and decodeMemory are the stored form; a row that can't be
// read is a plain copy.
func encodeMemory(m Memory) string {
	b, _ := json.Marshal(m.Clean())
	return string(b)
}

func decodeMemory(s string) Memory {
	var m Memory
	if json.Unmarshal([]byte(s), &m) != nil {
		return Memory{}
	}
	return m.Clean()
}

// fitCopies caps a holder's copies of one item at its count, keeping the
// oldest: a count lowered outside the Armory (the inventory widget, a
// deleted line) takes the newest remembered copies with it. Copies come
// oldest first.
func fitCopies(copies []Copy, total int) []Copy {
	var out []Copy
	left := total
	for _, c := range copies {
		if left <= 0 {
			break
		}
		if c.Quantity > left {
			c.Quantity = left
		}
		if c.Quantity > 0 {
			out = append(out, c)
			left -= c.Quantity
		}
	}
	return out
}

// CopyPick asks for N of a remembered copy row (0 for as many as needed).
type CopyPick struct {
	ID int64
	N  int
}

// planTake decides which copies leave when take of an item's total go:
// the picked copies first (in the order given), then plain ones, then the
// oldest remembered ones. leaving is each remembered copy that goes with
// how many of it; the rest are plain. copies come oldest first and are
// first fitted to total.
func planTake(copies []Copy, total, take int, pick []CopyPick) (leaving []Copy) {
	copies = fitCopies(copies, total)
	plain := total
	for _, c := range copies {
		plain -= c.Quantity
	}
	left := take
	used := map[int64]int{}
	takeFrom := func(c Copy, n int) {
		if n > c.Quantity-used[c.ID] {
			n = c.Quantity - used[c.ID]
		}
		if n <= 0 || left <= 0 {
			return
		}
		if n > left {
			n = left
		}
		used[c.ID] += n
		left -= n
	}
	byID := map[int64]Copy{}
	for _, c := range copies {
		byID[c.ID] = c
	}
	for _, p := range pick {
		if c, ok := byID[p.ID]; ok {
			n := p.N
			if n <= 0 {
				n = c.Quantity
			}
			takeFrom(c, n)
		}
	}
	if n := min(plain, left); n > 0 {
		left -= n
	}
	for _, c := range copies {
		takeFrom(c, c.Quantity)
	}
	for _, c := range copies {
		if n := used[c.ID]; n > 0 {
			c.Quantity = n
			leaving = append(leaving, c)
		}
	}
	return leaving
}

// CopyTag is one word a copy wears in the pack: its quality, a mark, or
// that it has gone off. Kind picks the tag's colour ("", "poor", "off").
type CopyTag struct {
	Label string
	Kind  string
}

// Tags are the words a copy wears, quality first.
func (m Memory) Tags() []CopyTag {
	var out []CopyTag
	switch m.Quality {
	case QualityFine:
		out = append(out, CopyTag{Label: "Fine"})
	case QualityPoor:
		out = append(out, CopyTag{Label: "Poor", Kind: "poor"})
	}
	for _, k := range m.Marks {
		out = append(out, CopyTag{Label: strings.ToUpper(k[:1]) + k[1:]})
	}
	if m.Off {
		out = append(out, CopyTag{Label: "Gone off", Kind: "off"})
	}
	return out
}

// Words say what else a copy remembers, in a line: who made or harvested
// it and when, its quirk, and when it goes off.
func (m Memory) Words() string {
	var parts []string
	if m.How != "" && m.By != "" {
		w := "Made by "
		if m.How == "harvested" {
			w = "Harvested by "
		}
		w += m.By
		if m.On != "" {
			w += ", " + m.On
		}
		parts = append(parts, w)
	}
	if m.Quirk != "" {
		parts = append(parts, "A quirk: "+m.Quirk)
	}
	switch {
	case m.Off && m.OffOn != "":
		parts = append(parts, "at the respite after "+m.OffOn)
	case m.OffOn != "":
		w := "goes off on " + m.OffOn
		if m.BecomesName != "" {
			w += ", then becomes " + m.BecomesName
		}
		parts = append(parts, w)
	}
	return strings.Join(parts, " · ")
}

// PlainCount is how many of a holder's total remember nothing.
func PlainCount(total int, copies []Copy) int {
	for _, c := range copies {
		total -= c.Quantity
	}
	return max(total, 0)
}

// copyChoice names a copy in the Move box: "The Fine one, made by Orrin",
// "The 2 Moonlit ones".
func copyChoice(c Copy) string {
	var words []string
	for _, t := range c.Memory.Tags() {
		words = append(words, t.Label)
	}
	name := "The " + strings.Join(words, ", ") + " one"
	if len(words) == 0 {
		name = "The remembered one"
	}
	if c.Quantity > 1 {
		name = fmt.Sprintf("The %d %s ones", c.Quantity, strings.Join(words, ", "))
		if len(words) == 0 {
			name = fmt.Sprintf("The %d remembered ones", c.Quantity)
		}
	}
	switch {
	case c.Memory.How == "made" && c.Memory.By != "":
		name += ", made by " + c.Memory.By
	case c.Memory.How == "harvested" && c.Memory.By != "":
		name += ", harvested by " + c.Memory.By
	}
	return name
}
