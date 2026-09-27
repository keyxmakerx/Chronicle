package campaigns

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// nav.go turns the owner's stored sidebar (SidebarConfig.Items) into the
// sections a sidebar is drawn from, then cuts them down for one viewer. It is
// the single place that decides what a sidebar holds: a viewer's sidebar HTML,
// the command palette's "Go to" entries and a player's pin checks all come
// from ViewNav, so a row hidden from players cannot reach a player through any
// of them.

// Built-in sections. Every sidebar has these three, in this order, ahead of
// any section the owner adds.
const (
	NavSectionPinned     = "pinned"
	NavSectionApps       = "apps"
	NavSectionCategories = "categories"
)

// NavKindCustom is the kind of a section the owner added; the built-in
// sections report their own id as their kind.
const NavKindCustom = "custom"

// Sidebar item types stored in SidebarConfig.Items.
const (
	SidebarTypeApp      = "app"
	SidebarTypeAddon    = "addon" // earlier spelling of an app item, still read as one
	SidebarTypeCategory = "category"
	SidebarTypeSection  = "section"
	SidebarTypeLink     = "link"
	// Dashboard and All Pages are placed by the sidebar itself; older configs
	// list them as items, which normalization skips.
	SidebarTypeDashboard = "dashboard"
	SidebarTypeAllPages  = "all_pages"
)

// Row kinds a NavRow reports.
const (
	NavRowApp      = "app"
	NavRowCategory = "category"
	NavRowLink     = "link"
)

// NavAccess is the least a viewer needs to open an app's page. The sidebar
// never offers a row whose page would turn the viewer away.
type NavAccess int

const (
	NavAccessAnyone   NavAccess = iota // anyone who can view the campaign
	NavAccessSignedIn                  // any signed-in viewer
	NavAccessMember                    // a member of the campaign
)

// NavApp is one app page the sidebar can list, resolved for a campaign.
type NavApp struct {
	Slug    string
	Label   string
	Icon    string
	URL     string
	Caption string // a few muted words beside the label, e.g. "Party & NPCs"
	Enabled bool   // turned on for this campaign
	Access  NavAccess
	// DefaultPinned starts the app in Pinned for a campaign whose owner has
	// never arranged the sidebar; every other app starts in Apps.
	DefaultPinned bool
}

// NavCategory is a top-level category and its sub-categories.
type NavCategory struct {
	TypeID int
	Label  string
	Icon   string
	Color  string
	URL    string
	Count  int
	Subs   []NavSubcategory
}

// NavSubcategory is a sub-category, drawn as its own row under its parent.
type NavSubcategory struct {
	TypeID int
	Label  string
	Color  string
	URL    string
	Count  int
}

// NavLayout is the owner's arrangement after normalization: the built-in
// sections, then the owner's own, each listing its items in order. It is also
// what the owner's editor starts from, so it keeps items a viewer would not
// see (hidden rows, apps turned off in Extensions).
type NavLayout struct {
	Sections []NavLayoutSection `json:"sections"`
}

// NavLayoutSection is one section of a NavLayout. Label is set for the
// owner's own sections; the built-in ones are named by the sidebar.
type NavLayoutSection struct {
	ID    string          `json:"id"`
	Label string          `json:"label,omitempty"`
	Items []NavLayoutItem `json:"items"`
}

// NavLayoutItem is one item of a NavLayout, addressed by its key. A link
// carries its own label, target and icon.
type NavLayoutItem struct {
	Key    string `json:"key"`
	Hidden bool   `json:"hidden,omitempty"` // hidden from players
	Label  string `json:"label,omitempty"`
	URL    string `json:"url,omitempty"`
	Icon   string `json:"icon,omitempty"`
}

// NavViewer is who a sidebar is drawn for.
type NavViewer struct {
	// Owner sees rows hidden from players, marked as hidden. False for an
	// owner who is viewing the campaign as a player.
	Owner bool
	// Access is what this viewer may open; apps needing more are left out.
	Access NavAccess
	// Pins are the viewer's own pinned keys, in the order pinned. Only a
	// non-owner has them; the owner pins for everyone in the layout itself.
	Pins []string
}

// NavSection is one section of a viewer's sidebar. Kind is the section's id
// for the built-in sections and NavKindCustom for the owner's own.
type NavSection struct {
	ID    string
	Kind  string
	Label string
	Rows  []NavRow
}

// NavRow is one row of a viewer's sidebar.
type NavRow struct {
	Key      string
	Kind     string
	Label    string
	Icon     string
	Color    string
	URL      string
	Caption  string
	External bool // a link leaving Chronicle; opens in a new tab
	Count    int
	Subs     []NavSubcategory
	// Hidden marks a row hidden from players. Only an owner is ever given one.
	Hidden bool
	// Personal marks a row this viewer pinned for themselves.
	Personal bool
}

// Key prefixes: an item is addressed as "<prefix>:<ref>" in a layout, in the
// page's markup and in a viewer's pins.
const (
	navKeyApp      = "app"
	navKeyCategory = "cat"
	navKeyLink     = "link"
)

// NavAppKey returns the key of an app item.
func NavAppKey(slug string) string { return navKeyApp + ":" + slug }

// NavCategoryKey returns the key of a category item.
func NavCategoryKey(id int) string { return navKeyCategory + ":" + strconv.Itoa(id) }

// NavLinkKey returns the key of a link item.
func NavLinkKey(id string) string { return navKeyLink + ":" + id }

// Formats for ids, app slugs and link icons. Ids and slugs end up in keys and
// markup; an icon is spliced into a class list, so only a Font Awesome name
// is accepted there.
var (
	navIDPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
	navSlugPattern = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	navIconPattern = regexp.MustCompile(`^fa-[a-z0-9-]{1,40}$`)
)

// isBuiltInNavSection reports whether id names one of the three built-in
// sections, which the owner cannot add, rename or remove.
func isBuiltInNavSection(id string) bool {
	return id == NavSectionPinned || id == NavSectionApps || id == NavSectionCategories
}

// navSectionAccepts reports whether a row of the given kind may sit in the
// section: Apps holds apps and links, Categories holds categories, and Pinned
// and the owner's sections hold anything.
func navSectionAccepts(section, kind string) bool {
	switch section {
	case NavSectionApps:
		return kind == NavRowApp || kind == NavRowLink
	case NavSectionCategories:
		return kind == NavRowCategory
	default:
		return true
	}
}

// navHomeSection is where a row goes when its stored section is missing or
// cannot hold it.
func navHomeSection(kind string) string {
	if kind == NavRowCategory {
		return NavSectionCategories
	}
	return NavSectionApps
}

// sanitizeNavIcon returns icon when it is a Font Awesome name and "" otherwise.
func sanitizeNavIcon(icon string) string {
	icon = strings.TrimSpace(icon)
	if navIconPattern.MatchString(icon) {
		return icon
	}
	return ""
}

// maxNavLabelLen caps a section's or link's label; both are drawn on one line.
const maxNavLabelLen = 100

// validateSidebarItems checks an owner's items before they are stored and
// returns them tidied: known types only, well-formed ids and slugs, section
// references that exist, bounded labels, link targets that are safe to render
// for every visitor (anonymous ones included, on a public campaign) and link
// icons that are Font Awesome names, since an icon lands in a class list.
func validateSidebarItems(items []SidebarItem) ([]SidebarItem, error) {
	sections := make(map[string]bool)
	for _, it := range items {
		if it.Type != SidebarTypeSection {
			continue
		}
		id := strings.TrimSpace(it.ID)
		switch {
		case !navIDPattern.MatchString(id):
			return nil, apperror.NewBadRequest("each section needs an id of up to 40 letters, digits, dashes or underscores")
		case isBuiltInNavSection(id):
			return nil, apperror.NewBadRequest(fmt.Sprintf("%q is a built-in section and cannot be added again", id))
		case sections[id]:
			return nil, apperror.NewBadRequest(fmt.Sprintf("two sections share the id %q", id))
		}
		sections[id] = true
	}

	out := make([]SidebarItem, 0, len(items))
	for _, it := range items {
		it.ID = strings.TrimSpace(it.ID)
		it.Label = strings.TrimSpace(it.Label)
		it.Section = strings.TrimSpace(it.Section)
		if utf8.RuneCountInString(it.Label) > maxNavLabelLen {
			return nil, apperror.NewBadRequest(fmt.Sprintf("labels can be at most %d characters", maxNavLabelLen))
		}
		switch it.Type {
		case SidebarTypeDashboard, SidebarTypeAllPages, SidebarTypeSection:
		case SidebarTypeApp, SidebarTypeAddon:
			if !navSlugPattern.MatchString(it.Slug) {
				return nil, apperror.NewBadRequest("an app item needs a valid slug")
			}
		case SidebarTypeCategory:
			if it.TypeID <= 0 {
				return nil, apperror.NewBadRequest("a category item needs a category id")
			}
		case SidebarTypeLink:
			if it.ID != "" && !navIDPattern.MatchString(it.ID) {
				return nil, apperror.NewBadRequest("a link id can only hold letters, digits, dashes or underscores")
			}
			if it.URL != "" {
				if err := validateNavLinkURL(it.Label, it.URL); err != nil {
					return nil, err
				}
			}
			it.Icon = sanitizeNavIcon(it.Icon)
		default:
			return nil, apperror.NewBadRequest(fmt.Sprintf("unknown sidebar item type %q", it.Type))
		}
		if it.Section != "" && !isBuiltInNavSection(it.Section) && !sections[it.Section] {
			return nil, apperror.NewBadRequest(fmt.Sprintf("an item names a section that does not exist: %q", it.Section))
		}
		out = append(out, it)
	}
	return out, nil
}

// NormalizeNav reads the stored items into sections. apps lists every app the
// campaign could show, turned on or not, and cats every top-level category;
// items naming anything else are dropped, as are repeats.
//
// Items saved before sections existed carry no section. They land where the
// old sidebar drew them: an app or link under a section heading stays in that
// section, any other app or link was in the list at the top (now Pinned), and
// a category stays in Categories. Apps that are turned on but missing join
// Apps, or their default section when the owner never arranged anything, and
// categories that are missing join Categories, so nothing is ever left out.
func NormalizeNav(items []SidebarItem, apps []NavApp, cats []NavCategory) NavLayout {
	knownApps := make(map[string]bool, len(apps))
	for _, a := range apps {
		knownApps[a.Slug] = true
	}
	knownCats := make(map[int]bool, len(cats))
	for _, c := range cats {
		knownCats[c.TypeID] = true
	}

	layout := NavLayout{Sections: []NavLayoutSection{
		{ID: NavSectionPinned, Items: []NavLayoutItem{}},
		{ID: NavSectionApps, Items: []NavLayoutItem{}},
		{ID: NavSectionCategories, Items: []NavLayoutItem{}},
	}}
	index := map[string]int{NavSectionPinned: 0, NavSectionApps: 1, NavSectionCategories: 2}
	for _, it := range items {
		if it.Type != SidebarTypeSection {
			continue
		}
		id := strings.TrimSpace(it.ID)
		if !navIDPattern.MatchString(id) || isBuiltInNavSection(id) {
			continue
		}
		if _, dup := index[id]; dup {
			continue
		}
		index[id] = len(layout.Sections)
		layout.Sections = append(layout.Sections, NavLayoutSection{
			ID: id, Label: strings.TrimSpace(it.Label), Items: []NavLayoutItem{},
		})
	}

	seen := make(map[string]bool, len(items))
	under := "" // the owner's section whose heading came last, for items saved without a section
	for i, it := range items {
		var entry NavLayoutItem
		var kind string
		switch it.Type {
		case SidebarTypeSection:
			if id := strings.TrimSpace(it.ID); !isBuiltInNavSection(id) {
				if _, ok := index[id]; ok {
					under = id
				}
			}
			continue
		case SidebarTypeApp, SidebarTypeAddon:
			if !knownApps[it.Slug] {
				continue
			}
			kind, entry.Key = NavRowApp, NavAppKey(it.Slug)
		case SidebarTypeCategory:
			if !knownCats[it.TypeID] {
				continue
			}
			kind, entry.Key = NavRowCategory, NavCategoryKey(it.TypeID)
		case SidebarTypeLink:
			id := strings.TrimSpace(it.ID)
			if !navIDPattern.MatchString(id) {
				// A link stored without an id still gets a key, so it can be
				// shown and re-saved; the editor writes the id back.
				id = "lnk-" + strconv.Itoa(i)
			}
			kind, entry.Key = NavRowLink, NavLinkKey(id)
			entry.Label = strings.TrimSpace(it.Label)
			entry.URL = strings.TrimSpace(it.URL)
			entry.Icon = sanitizeNavIcon(it.Icon)
		default:
			continue
		}
		if seen[entry.Key] {
			continue
		}
		seen[entry.Key] = true
		entry.Hidden = !it.Visible

		section := strings.TrimSpace(it.Section)
		switch {
		case section != "":
			if _, ok := index[section]; !ok || !navSectionAccepts(section, kind) {
				section = navHomeSection(kind)
			}
		case kind == NavRowCategory:
			section = NavSectionCategories
		case under != "":
			section = under
		default:
			section = NavSectionPinned
		}
		s := &layout.Sections[index[section]]
		s.Items = append(s.Items, entry)
	}

	arranged := len(items) > 0
	for _, a := range apps {
		key := NavAppKey(a.Slug)
		if !a.Enabled || seen[key] {
			continue
		}
		seen[key] = true
		section := NavSectionApps
		if !arranged && a.DefaultPinned {
			section = NavSectionPinned
		}
		s := &layout.Sections[index[section]]
		s.Items = append(s.Items, NavLayoutItem{Key: key})
	}
	for _, c := range cats {
		key := NavCategoryKey(c.TypeID)
		if seen[key] {
			continue
		}
		seen[key] = true
		s := &layout.Sections[index[NavSectionCategories]]
		s.Items = append(s.Items, NavLayoutItem{Key: key})
	}
	return layout
}

// navSectionTitle names a section as the sidebar shows it.
func navSectionTitle(s NavLayoutSection) string {
	switch s.ID {
	case NavSectionPinned:
		return "Pinned"
	case NavSectionApps:
		return "Apps"
	case NavSectionCategories:
		return "Categories"
	}
	if s.Label == "" {
		return "Untitled"
	}
	return s.Label
}

// ViewNav cuts a layout down to what one viewer's sidebar shows. A row hidden
// from players is kept only for an owner, and marked. An app is kept only
// when it is turned on and the viewer may open its page, and a link only when
// its target is safe to follow. A non-owner's own pins then move their rows
// into Pinned, after the campaign's pins, so no row is ever shown twice.
func ViewNav(layout NavLayout, apps []NavApp, cats []NavCategory, viewer NavViewer) []NavSection {
	appBySlug := make(map[string]NavApp, len(apps))
	for _, a := range apps {
		appBySlug[a.Slug] = a
	}
	catByID := make(map[int]NavCategory, len(cats))
	for _, c := range cats {
		catByID[c.TypeID] = c
	}

	out := make([]NavSection, 0, len(layout.Sections))
	for _, s := range layout.Sections {
		kind := s.ID
		if !isBuiltInNavSection(s.ID) {
			kind = NavKindCustom
		}
		sec := NavSection{ID: s.ID, Kind: kind, Label: navSectionTitle(s)}
		for _, it := range s.Items {
			if it.Hidden && !viewer.Owner {
				continue
			}
			row, ok := navRowFor(it, appBySlug, catByID, viewer)
			if !ok {
				continue
			}
			row.Hidden = it.Hidden
			sec.Rows = append(sec.Rows, row)
		}
		out = append(out, sec)
	}
	if !viewer.Owner && len(viewer.Pins) > 0 && len(out) > 0 && out[0].ID == NavSectionPinned {
		applyNavPins(out, viewer.Pins)
	}
	return out
}

// navRowFor resolves one layout item into a row, or reports false when the
// viewer should not see it at all.
func navRowFor(it NavLayoutItem, apps map[string]NavApp, cats map[int]NavCategory, viewer NavViewer) (NavRow, bool) {
	prefix, ref, ok := strings.Cut(it.Key, ":")
	if !ok {
		return NavRow{}, false
	}
	switch prefix {
	case navKeyApp:
		a, found := apps[ref]
		if !found || !a.Enabled || viewer.Access < a.Access || a.URL == "" {
			return NavRow{}, false
		}
		return NavRow{
			Key: it.Key, Kind: NavRowApp, Label: a.Label, Icon: a.Icon,
			URL: a.URL, Caption: a.Caption,
		}, true
	case navKeyCategory:
		id, err := strconv.Atoi(ref)
		if err != nil {
			return NavRow{}, false
		}
		c, found := cats[id]
		if !found {
			return NavRow{}, false
		}
		return NavRow{
			Key: it.Key, Kind: NavRowCategory, Label: c.Label, Icon: c.Icon,
			Color: c.Color, URL: c.URL, Count: c.Count, Subs: c.Subs,
		}, true
	case navKeyLink:
		target, safe := sanitize.SafeLinkURL(it.URL)
		if !safe || it.Label == "" {
			return NavRow{}, false
		}
		lower := strings.ToLower(target)
		return NavRow{
			Key: it.Key, Kind: NavRowLink, Label: it.Label, Icon: it.Icon, URL: target,
			External: strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"),
		}, true
	}
	return NavRow{}, false
}

// applyNavPins moves each pinned row, in pin order, from its section to the
// end of Pinned. Keys the viewer cannot see, or that the campaign already
// pinned, are skipped, which is what keeps another viewer's or a hidden
// row's key from ever surfacing here.
func applyNavPins(sections []NavSection, pins []string) {
	pinned := &sections[0]
	inPinned := make(map[string]bool, len(pinned.Rows))
	for _, r := range pinned.Rows {
		inPinned[r.Key] = true
	}
	for _, key := range pins {
		if inPinned[key] {
			continue
		}
		for si := 1; si < len(sections); si++ {
			rows := sections[si].Rows
			idx := -1
			for ri := range rows {
				if rows[ri].Key == key {
					idx = ri
					break
				}
			}
			if idx < 0 {
				continue
			}
			row := rows[idx]
			sections[si].Rows = append(rows[:idx:idx], rows[idx+1:]...)
			row.Personal = true
			pinned.Rows = append(pinned.Rows, row)
			inPinned[key] = true
			break
		}
	}
}
