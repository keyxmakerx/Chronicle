// page_layouts.go — a category's named page layouts besides its main one.
//
// One category can hold different kinds of page (Characters holds heroes and
// NPCs), so it can keep a few named layouts. Each page wears one: the one the
// Director picked for it, else the one a player's character starts on, else
// the category's main layout (entity_types.layout_json). Nothing changes for a
// category that never adds a layout.

package entities

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	// MaxPageLayouts is how many named layouts a category keeps besides its
	// main one.
	MaxPageLayouts = 8
	// maxPageLayoutName is the longest name a layout takes, in characters.
	maxPageLayoutName = 40
)

// pageLayoutKeyPattern is a layout's key: short, URL-safe and stable, since
// pages store it and the AI writes it.
var pageLayoutKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// MainLayoutKey is the key that names a category's main layout wherever a
// key is expected; a page with no choice wears it.
const MainLayoutKey = "main"

// PageLayout is one of a category's named layouts besides the main one.
type PageLayout struct {
	Key    string           `json:"key"`
	Name   string           `json:"name"`
	Layout EntityTypeLayout `json:"layout"`
}

// PageLayouts is everything about a category's layouts that is not the main
// layout itself.
type PageLayouts struct {
	// MainName is what the main layout is called; empty reads as the
	// category's singular name.
	MainName string `json:"mainName,omitempty"`
	// Claimed is the key of the layout a player's character wears when the
	// Director has not picked one; empty keeps the main layout.
	Claimed string       `json:"claimed,omitempty"`
	Extra   []PageLayout `json:"extra,omitempty"`
}

// Empty reports whether the category has nothing beyond its main layout.
func (p PageLayouts) Empty() bool {
	return p.MainName == "" && p.Claimed == "" && len(p.Extra) == 0
}

// Find returns the named layout with key, if the category has one.
func (p PageLayouts) Find(key string) (PageLayout, bool) {
	for _, l := range p.Extra {
		if l.Key == key {
			return l, true
		}
	}
	return PageLayout{}, false
}

// MainLabel is the main layout's name as people read it.
func (p PageLayouts) MainLabel(et *EntityType) string {
	if p.MainName != "" {
		return p.MainName
	}
	if et != nil && et.Name != "" {
		return et.Name
	}
	return "Main"
}

// Label is the name of the layout key names, the main one for any key the
// category does not have.
func (p PageLayouts) Label(et *EntityType, key string) string {
	if l, ok := p.Find(key); ok {
		return l.Name
	}
	return p.MainLabel(et)
}

// KeyByName finds a layout by its name, ignoring case, so the AI and the
// Director can name a layout the way it reads. The main layout answers to its
// name and to "main".
func (p PageLayouts) KeyByName(et *EntityType, name string) (string, bool) {
	n := strings.TrimSpace(name)
	if strings.EqualFold(n, MainLayoutKey) || strings.EqualFold(n, p.MainLabel(et)) {
		return MainLayoutKey, true
	}
	for _, l := range p.Extra {
		if strings.EqualFold(n, l.Name) || strings.EqualFold(n, l.Key) {
			return l.Key, true
		}
	}
	return "", false
}

// WornKey is the key of the layout a page wears: its own choice when that
// still names one of the category's layouts, else the layout a player's
// character starts on, else the main one.
func (p PageLayouts) WornKey(e *Entity, chosen string) string {
	if chosen == MainLayoutKey {
		return MainLayoutKey
	}
	if _, ok := p.Find(chosen); ok {
		return chosen
	}
	if isPlayersCharacter(e) {
		if _, ok := p.Find(p.Claimed); ok {
			return p.Claimed
		}
	}
	return MainLayoutKey
}

// LayoutFor returns the blocks of the layout key names, the main layout for
// the main key or any key the category does not have.
func (p PageLayouts) LayoutFor(et *EntityType, key string) EntityTypeLayout {
	if l, ok := p.Find(key); ok {
		return l.Layout
	}
	if et == nil {
		return EntityTypeLayout{}
	}
	return et.Layout
}

// Check validates the names, keys and choices; the blocks of each layout are
// checked by the caller with ValidateLayout, as a main layout's are.
func (p PageLayouts) Check() error {
	if len(p.Extra) > MaxPageLayouts {
		return apperror.NewBadRequest("a category can have at most 8 layouts besides its main one")
	}
	if err := checkLayoutName(p.MainName, true); err != nil {
		return err
	}
	seenKey := map[string]bool{MainLayoutKey: true}
	seenName := map[string]bool{strings.ToLower(strings.TrimSpace(p.MainName)): p.MainName != ""}
	for _, l := range p.Extra {
		if !pageLayoutKeyPattern.MatchString(l.Key) || l.Key == MainLayoutKey {
			return apperror.NewBadRequest("a layout's key must be short lowercase letters, digits and dashes")
		}
		if seenKey[l.Key] {
			return apperror.NewBadRequest("two layouts share a key")
		}
		seenKey[l.Key] = true
		if err := checkLayoutName(l.Name, false); err != nil {
			return err
		}
		n := strings.ToLower(strings.TrimSpace(l.Name))
		if seenName[n] {
			return apperror.NewBadRequest("two layouts in a category can't share a name")
		}
		seenName[n] = true
	}
	if p.Claimed != "" {
		if _, ok := p.Find(p.Claimed); !ok {
			return apperror.NewBadRequest("the layout a player's character starts on is not one of this category's layouts")
		}
	}
	return nil
}

// checkLayoutName checks one layout's name; the main one may be left empty.
func checkLayoutName(name string, mayBeEmpty bool) error {
	t := strings.TrimSpace(name)
	if t == "" {
		if mayBeEmpty {
			return nil
		}
		return apperror.NewBadRequest("a layout needs a name")
	}
	if t != name || utf8.RuneCountInString(t) > maxPageLayoutName || strings.ContainsAny(t, "\n\r\t") {
		return apperror.NewBadRequest("a layout's name is one line of at most 40 characters")
	}
	return nil
}

// PageLayoutInfo is what a page shows about the layout it wears: which one,
// and the others the Director could pick.
type PageLayoutInfo struct {
	Key     string
	Name    string
	Choices []PageLayoutChoice
}

// PageLayoutChoice is one layout a page could wear.
type PageLayoutChoice struct {
	Key  string
	Name string
}

// choices lists the main layout then the named ones, in their order.
func (p PageLayouts) choices(et *EntityType) []PageLayoutChoice {
	out := []PageLayoutChoice{{Key: MainLayoutKey, Name: p.MainLabel(et)}}
	for _, l := range p.Extra {
		out = append(out, PageLayoutChoice{Key: l.Key, Name: l.Name})
	}
	return out
}
