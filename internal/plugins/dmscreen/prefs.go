package dmscreen

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// A person's screen preferences: how they arrange a docked screen (which
// cards are open, folded, on a leaf, their heights, which span two columns,
// which are hidden) and its surface and density. They are personal, so
// everything here is about one person's own row; nothing in it can reveal
// content, since the cards themselves are always drawn by the server for the
// viewer. The document is validated whole on every write so a hand-sent body
// can only ever store the shape the screen writes.

// The screens a person keeps preferences for.
const (
	ScreenDM     = "dm"     // the DM Screen
	ScreenPlayer = "player" // the players' screen
)

// Screens lists every screen with preferences, for routes and tests.
var Screens = []string{ScreenDM, ScreenPlayer}

const (
	// maxLayouts is how many named layouts a person keeps per screen, beside
	// Default, which is always there and never stored as one.
	maxLayouts = 8
	// maxLayoutName is a layout name's length in characters.
	maxLayoutName = 40
	// maxPrefsBytes caps the stored document. A full screen with eight
	// layouts is a few kilobytes; the cap only stops abuse.
	maxPrefsBytes = 16 << 10
	// maxSectionIDs caps every list of section ids; a screen has about a dozen.
	maxSectionIDs = 32
	// maxPrefsLeaves matches the dock's MAX_LEAVES.
	maxPrefsLeaves = 2
	// DefaultLayoutName is the layout that is always there. It is the
	// person's everyday screen; a saved layout may not take its name.
	DefaultLayoutName = "Default"
)

// sectionIDPattern is the shape of a section id ("world", "party", "my-hero").
// The ids reach the browser only as data attributes the dock compares, but a
// short safe shape keeps the stored document free of anything else.
var sectionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// The heights a card can have. L is the whole card and the default, so it is
// never stored.
const (
	SizeSmall  = "S"
	SizeMedium = "M"
	SizeLarge  = "L"
)

// Surfaces and densities the Style drawer offers. The empty string is the
// default (solid, roomy).
var (
	prefsSurfaces  = []string{"", "solid", "glass", "felt"}
	prefsDensities = []string{"", "roomy", "compact"}
)

// Arrangement is one way of laying out a screen.
type Arrangement struct {
	// Open lists the column's open cards in the order they were opened; the
	// column shows the newest first.
	Open []string `json:"open"`
	// Folded are cards folded to their one line.
	Folded []string `json:"folded,omitempty"`
	// Leaves are the leaves beside the column, each a list of its cards.
	Leaves [][]string `json:"leaves,omitempty"`
	// Sizes holds each card's height, S or M; a card not listed is L.
	Sizes map[string]string `json:"sizes,omitempty"`
	// Wide cards span both columns when the column is two wide.
	Wide []string `json:"wide,omitempty"`
	// Hidden cards leave the rail until shown again in arrange mode.
	Hidden []string `json:"hidden,omitempty"`
	// Cols is the column's width in columns, 1 or 2.
	Cols int `json:"cols,omitempty"`
}

// Layout is a named, saved arrangement.
type Layout struct {
	Name string `json:"name"`
	Arrangement
}

// Style is the screen's surface and density. The kit is not here: it is a
// member look pick, governed by the owner's "who can change" setting.
type Style struct {
	Surface string `json:"surface,omitempty"`
	Density string `json:"density,omitempty"`
}

// ScreenPrefs is the stored document.
type ScreenPrefs struct {
	// Current names the saved layout the screen was last switched to; empty
	// is Default.
	Current string `json:"current,omitempty"`
	// Home is Default's arrangement, the person's everyday screen. Nil means
	// the screen's own starting arrangement.
	Home *Arrangement `json:"home,omitempty"`
	// Live is what the screen shows while a saved layout is current and has
	// been changed since; nil shows that layout as saved.
	Live    *Arrangement `json:"live,omitempty"`
	Layouts []Layout     `json:"layouts,omitempty"`
	Style   Style        `json:"style"`
}

// UpdateScreenPrefsInput is a PUT body. It is partial: an absent key keeps
// what is stored, an explicit null clears it (Default, the starting
// arrangement, no layouts, the default style), and a value replaces it. The
// screen sends only what changed, so a style change from one tab cannot undo
// a layout another tab saved.
type UpdateScreenPrefsInput struct {
	Current patch.Field[string]      `json:"current"`
	Home    patch.Field[Arrangement] `json:"home"`
	Live    patch.Field[Arrangement] `json:"live"`
	Layouts patch.Field[[]Layout]    `json:"layouts"`
	Style   patch.Field[Style]       `json:"style"`
}

// mergePrefs lays in over cur and validates the result, which is what is
// stored. Validation reads the merged document, so an absent key is never
// mistaken for an empty one.
func mergePrefs(cur ScreenPrefs, in UpdateScreenPrefsInput) (ScreenPrefs, error) {
	out := cur
	// Get gives the zero value for an explicit null, which is each key's
	// cleared state: Default, no layouts, the default style.
	if in.Current.Present() {
		out.Current, _ = in.Current.Get()
	}
	out.Home = in.Home.Ptr(cur.Home)
	out.Live = in.Live.Ptr(cur.Live)
	if in.Layouts.Present() {
		out.Layouts, _ = in.Layouts.Get()
	}
	if in.Style.Present() {
		out.Style, _ = in.Style.Get()
	}
	return cleanPrefs(out)
}

// cleanPrefs validates a document and returns it in its stored shape:
// repeated ids dropped, default values left out, a current layout that no
// longer exists read as Default.
func cleanPrefs(p ScreenPrefs) (ScreenPrefs, error) {
	var out ScreenPrefs
	var err error
	if p.Home != nil {
		h, err := cleanArrangement(*p.Home)
		if err != nil {
			return out, err
		}
		out.Home = &h
	}
	if len(p.Layouts) > maxLayouts {
		return out, apperror.NewBadRequest(fmt.Sprintf("a screen keeps %d layouts at most", maxLayouts))
	}
	seen := map[string]bool{}
	for _, l := range p.Layouts {
		name, err := cleanLayoutName(l.Name)
		if err != nil {
			return out, err
		}
		key := strings.ToLower(name)
		if seen[key] {
			return out, apperror.NewBadRequest(fmt.Sprintf("two layouts are called %q", name))
		}
		seen[key] = true
		a, err := cleanArrangement(l.Arrangement)
		if err != nil {
			return out, err
		}
		out.Layouts = append(out.Layouts, Layout{Name: name, Arrangement: a})
	}
	// A layout deleted from another tab leaves the screen on Default rather
	// than refusing every later save.
	if cur := strings.TrimSpace(p.Current); cur != "" {
		for _, l := range out.Layouts {
			if strings.EqualFold(l.Name, cur) {
				out.Current = l.Name
			}
		}
	}
	if out.Current != "" && p.Live != nil {
		l, err := cleanArrangement(*p.Live)
		if err != nil {
			return out, err
		}
		out.Live = &l
	}
	if out.Style, err = cleanStyle(p.Style); err != nil {
		return out, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return out, apperror.NewInternal(err)
	}
	if len(b) > maxPrefsBytes {
		return out, apperror.NewBadRequest("the screen's layouts are too large to keep")
	}
	return out, nil
}

// cleanLayoutName trims a layout name and checks it: 1 to 40 characters, no
// control characters, and never Default's name.
func cleanLayoutName(name string) (string, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n == 0 {
		return "", apperror.NewBadRequest("a layout needs a name")
	}
	if n > maxLayoutName {
		return "", apperror.NewBadRequest(fmt.Sprintf("a layout name is %d characters at most", maxLayoutName))
	}
	if !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", apperror.NewBadRequest("a layout name can't hold control characters")
	}
	if strings.EqualFold(name, DefaultLayoutName) {
		return "", apperror.NewBadRequest("Default is always there, so a saved layout needs another name")
	}
	return name, nil
}

// cleanIDs checks a list of section ids and drops repeats, keeping order.
func cleanIDs(ids []string) ([]string, error) {
	if len(ids) > maxSectionIDs {
		return nil, apperror.NewBadRequest(fmt.Sprintf("a layout lists %d sections at most", maxSectionIDs))
	}
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		if !sectionIDPattern.MatchString(id) {
			return nil, apperror.NewBadRequest(fmt.Sprintf("%q is not a section", id))
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// cleanArrangement checks an arrangement. A card sits in one place: on a
// leaf wins over the column, as the dock draws it.
func cleanArrangement(a Arrangement) (Arrangement, error) {
	var out Arrangement
	var err error
	if len(a.Leaves) > maxSectionIDs {
		return out, apperror.NewBadRequest(fmt.Sprintf("a screen opens %d leaves at most", maxPrefsLeaves))
	}
	onLeaf := map[string]bool{}
	for _, l := range a.Leaves {
		ids, err := cleanIDs(l)
		if err != nil {
			return out, err
		}
		var kept []string
		for _, id := range ids {
			if !onLeaf[id] {
				onLeaf[id] = true
				kept = append(kept, id)
			}
		}
		if len(kept) > 0 {
			out.Leaves = append(out.Leaves, kept)
		}
	}
	if len(out.Leaves) > maxPrefsLeaves {
		return out, apperror.NewBadRequest(fmt.Sprintf("a screen opens %d leaves at most", maxPrefsLeaves))
	}
	open, err := cleanIDs(a.Open)
	if err != nil {
		return out, err
	}
	out.Open = []string{}
	for _, id := range open {
		if !onLeaf[id] {
			out.Open = append(out.Open, id)
		}
	}
	if out.Folded, err = cleanIDs(a.Folded); err != nil {
		return out, err
	}
	if out.Wide, err = cleanIDs(a.Wide); err != nil {
		return out, err
	}
	if out.Hidden, err = cleanIDs(a.Hidden); err != nil {
		return out, err
	}
	if len(a.Sizes) > maxSectionIDs {
		return out, apperror.NewBadRequest(fmt.Sprintf("a layout sizes %d sections at most", maxSectionIDs))
	}
	for id, sz := range a.Sizes {
		if !sectionIDPattern.MatchString(id) {
			return out, apperror.NewBadRequest(fmt.Sprintf("%q is not a section", id))
		}
		switch sz {
		case SizeSmall, SizeMedium:
			if out.Sizes == nil {
				out.Sizes = map[string]string{}
			}
			out.Sizes[id] = sz
		case SizeLarge:
		default:
			return out, apperror.NewBadRequest(fmt.Sprintf("%q is not a card height (S, M or L)", sz))
		}
	}
	switch a.Cols {
	case 0, 1:
	case 2:
		out.Cols = 2
	default:
		return out, apperror.NewBadRequest("a screen is one or two columns wide")
	}
	return out, nil
}

// cleanStyle checks the surface and density, storing the defaults as empty.
func cleanStyle(s Style) (Style, error) {
	if !oneOfStr(s.Surface, prefsSurfaces) {
		return Style{}, apperror.NewBadRequest(fmt.Sprintf("%q is not a surface", s.Surface))
	}
	if !oneOfStr(s.Density, prefsDensities) {
		return Style{}, apperror.NewBadRequest(fmt.Sprintf("%q is not a density", s.Density))
	}
	out := s
	if out.Surface == "solid" {
		out.Surface = ""
	}
	if out.Density == "roomy" {
		out.Density = ""
	}
	return out, nil
}

func oneOfStr(v string, list []string) bool {
	for _, x := range list {
		if v == x {
			return true
		}
	}
	return false
}
