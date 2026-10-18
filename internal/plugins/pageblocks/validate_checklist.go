package pageblocks

import (
	"encoding/json"
	"fmt"
)

// Checklist limits. Sixty items is the table's row limit; ten thresholds is
// more than a story needs and keeps the DM's notes readable.
const (
	MaxChecklistItems = 60
	MaxChecklistName  = 120
	MaxSideNameChars  = 40
	MaxThresholds     = 10
	MaxThresholdNote  = 500
	MaxChecklistTitle = 120
	defaultSideA      = "Party"
	defaultSideB      = "The other side"
)

func validSide(s string) bool { return s == SideA || s == SideB }

func validateChecklist(raw json.RawMessage) (Checklist, error) {
	var d Checklist
	if err := decodeStrict(raw, &d); err != nil {
		return Checklist{}, err
	}
	title, err := cleanText("title", d.Title, MaxChecklistTitle, false)
	if err != nil {
		return Checklist{}, err
	}
	sideA, err := cleanText("side name", d.SideA, MaxSideNameChars, false)
	if err != nil {
		return Checklist{}, err
	}
	sideB, err := cleanText("side name", d.SideB, MaxSideNameChars, false)
	if err != nil {
		return Checklist{}, err
	}
	if len(d.Items) > MaxChecklistItems {
		return Checklist{}, errInvalid(fmt.Sprintf("a checklist holds at most %d items", MaxChecklistItems))
	}
	if len(d.Thresholds) > MaxThresholds {
		return Checklist{}, errInvalid(fmt.Sprintf("a checklist has at most %d thresholds", MaxThresholds))
	}
	seen := make(map[string]bool, len(d.Items)+len(d.Thresholds))
	items := make([]ChecklistItem, 0, len(d.Items))
	for _, it := range d.Items {
		if !validToken(it.ID) || seen[it.ID] {
			return Checklist{}, errInvalid("item ids must be short, unique and plain")
		}
		seen[it.ID] = true
		name, err := cleanText("item name", it.Name, MaxChecklistName, false)
		if err != nil {
			return Checklist{}, err
		}
		if it.Holder != "" && !validSide(it.Holder) {
			return Checklist{}, errInvalid("an item is held by side a, side b or nobody")
		}
		if it.EntityID != "" && !validToken(it.EntityID) {
			return Checklist{}, errInvalid("an item links to a page that doesn't exist")
		}
		// The page's name and path are added on read; a client echoing
		// them back must not get them stored.
		items = append(items, ChecklistItem{ID: it.ID, Name: name, EntityID: it.EntityID, Holder: it.Holder, DM: it.DM})
	}
	ths := make([]Threshold, 0, len(d.Thresholds))
	for _, th := range d.Thresholds {
		if !validToken(th.ID) || seen[th.ID] {
			return Checklist{}, errInvalid("threshold ids must be short, unique and plain")
		}
		seen[th.ID] = true
		if !validSide(th.Side) {
			return Checklist{}, errInvalid("a threshold counts side a or side b")
		}
		if th.Count < 1 || th.Count > MaxChecklistItems {
			return Checklist{}, errInvalid(fmt.Sprintf("a threshold's count is 1 to %d", MaxChecklistItems))
		}
		note, err := cleanText("threshold note", normalizeNewlines(th.Note), MaxThresholdNote, true)
		if err != nil {
			return Checklist{}, err
		}
		ths = append(ths, Threshold{ID: th.ID, Side: th.Side, Count: th.Count, Note: note})
	}
	return Checklist{Title: title, SideA: sideA, SideB: sideB, DMOnly: d.DMOnly, Items: items, Thresholds: ths}, nil
}

// emptyChecklist is a block with nothing saved: two sides named so the
// first thing an owner sees reads sensibly, and no items.
func emptyChecklist() Checklist {
	return Checklist{SideA: defaultSideA, SideB: defaultSideB, Items: []ChecklistItem{}, Thresholds: []Threshold{}}
}

// HeldCounts returns how many items each side holds.
func HeldCounts(d Checklist) (a, b int) {
	for _, it := range d.Items {
		switch it.Holder {
		case SideA:
			a++
		case SideB:
			b++
		}
	}
	return a, b
}

// ReachedThresholds returns the thresholds whose side holds at least their
// count, in the order the DM wrote them.
func ReachedThresholds(d Checklist) []Threshold {
	a, b := HeldCounts(d)
	var out []Threshold
	for _, th := range d.Thresholds {
		n := a
		if th.Side == SideB {
			n = b
		}
		if n >= th.Count {
			out = append(out, th)
		}
	}
	return out
}

// hasDMChecklist reports whether anything in d is the DM's alone: the whole
// block, an item, or a threshold.
func hasDMChecklist(d Checklist) bool {
	if d.DMOnly || len(d.Thresholds) > 0 {
		return true
	}
	for _, it := range d.Items {
		if it.DM {
			return true
		}
	}
	return false
}

// checklistEntityIDs lists the pages d's items link to, once each.
func checklistEntityIDs(d Checklist) []string {
	seen := map[string]bool{}
	var ids []string
	for _, it := range d.Items {
		if it.EntityID != "" && !seen[it.EntityID] {
			seen[it.EntityID] = true
			ids = append(ids, it.EntityID)
		}
	}
	return ids
}
