package layouts

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// nav_state.go works out, once per request, where the viewer is in the
// campaign sidebar: which row is current (it carries the living ring and the
// open page's name), which sections are folded, and what a folded heading
// says. The server renders all of it, so a full page load paints the finished
// sidebar with nothing jumping into place; sidebar_nav.js keeps it current
// across boosted navigation by reading the marker App renders into
// #main-content.

const (
	keyNavHint  ctxKey = "layout_nav_hint"
	keyNavFolds ctxKey = "layout_nav_folds"
	keyNavState ctxKey = "layout_nav_state"
)

// NavHint names the category an open page belongs to, and the page's own
// name, for pages whose URL does not say (an entity's page).
type NavHint struct {
	TypeID   int
	PageName string
}

// WithNavHint attaches a hint to a request context before rendering. The
// template context is derived from the request context, so the hint reaches
// the sidebar without the injector knowing about it.
func WithNavHint(ctx context.Context, h NavHint) context.Context {
	return context.WithValue(ctx, keyNavHint, h)
}

// NavFolds is a viewer's remembered fold state: fold id → open.
type NavFolds map[string]bool

// NavFoldsCookie is the cookie sidebar_nav.js writes a viewer's folds to,
// scoped to one campaign's path.
const NavFoldsCookie = "chronicle_nav_folds"

// navFoldIDPattern matches a fold id: a section id ("apps", "sec_x", "manage")
// or a sub-category fold ("sub-12").
var navFoldIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,48}$`)

// ParseNavFolds reads the fold cookie, "id:1|id:0|...". It is client-written,
// so anything malformed is skipped and the size is bounded.
func ParseNavFolds(raw string) NavFolds {
	out := NavFolds{}
	if raw == "" || len(raw) > 2048 {
		return out
	}
	for i, part := range strings.Split(raw, "|") {
		if i >= 64 {
			break
		}
		id, v, ok := strings.Cut(part, ":")
		if !ok || !navFoldIDPattern.MatchString(id) || (v != "0" && v != "1") {
			continue
		}
		out[id] = v == "1"
	}
	return out
}

// SetNavFolds stores the viewer's remembered folds in context.
func SetNavFolds(ctx context.Context, f NavFolds) context.Context {
	return context.WithValue(ctx, keyNavFolds, f)
}

// navState is what ResolveNavState works out for one render.
type navState struct {
	current string // key of the row for the open page; "" when none matches
	page    string // the open page's own name, shown in its row
	ring    string // row the living ring sits on: current, or its parent while folded away
	folds   NavFolds
	subOf   map[string]string // sub-category row key → its parent category's key
	curOpen string            // fold id ("sub-12") that the current row sits in, open by default
}

// navCandidate is a destination the current page could belong to.
type navCandidate struct {
	key   string
	url   string
	exact bool
}

// ResolveNavState stores the viewer's nav state in ctx. Call it after the
// sections, folds and active path are set.
func ResolveNavState(ctx context.Context) context.Context {
	if !InCampaign(ctx) {
		return ctx
	}
	st := navState{subOf: map[string]string{}}
	st.folds, _ = ctx.Value(keyNavFolds).(NavFolds)

	base := "/campaigns/" + GetCampaignID(ctx)
	var cands []navCandidate
	cands = append(cands, navCandidate{key: "dashboard", url: base, exact: true})
	if navShowsMe(ctx) {
		cands = append(cands, navCandidate{key: "me", url: base + "/me", exact: true})
	}
	for _, sec := range GetNavSections(ctx) {
		for _, row := range sec.Rows {
			if row.Kind != "link" {
				cands = append(cands, navCandidate{key: row.Key, url: row.URL})
			}
			for _, sub := range row.Subs {
				st.subOf[sub.Key] = row.Key
				cands = append(cands, navCandidate{key: sub.Key, url: sub.URL})
			}
		}
	}
	cands = append(cands, navCandidate{key: "all", url: base + "/entities"})
	for _, row := range NavManageRows(ctx) {
		cands = append(cands, navCandidate{key: row.Key, url: row.URL})
	}

	if hint, ok := ctx.Value(keyNavHint).(NavHint); ok && hint.TypeID > 0 {
		key := "cat:" + strconv.Itoa(hint.TypeID)
		for _, c := range cands {
			if c.key == key {
				st.current, st.page = key, hint.PageName
				break
			}
		}
	}
	if st.current == "" {
		path := GetActivePath(ctx)
		best := -1
		for _, c := range cands {
			match := path == c.url || (!c.exact && strings.HasPrefix(path, c.url+"/"))
			if match && len(c.url) > best {
				best, st.current = len(c.url), c.key
			}
		}
	}

	st.ring = st.current
	if parent, isSub := st.subOf[st.current]; isSub {
		st.curOpen = "sub-" + strings.TrimPrefix(parent, "cat:")
		if !navFoldOpen(st, st.curOpen) {
			st.ring = parent
		}
	} else if strings.HasPrefix(st.current, "cat:") {
		st.curOpen = "sub-" + strings.TrimPrefix(st.current, "cat:")
	}
	return context.WithValue(ctx, keyNavState, st)
}

func navStateOf(ctx context.Context) navState {
	st, _ := ctx.Value(keyNavState).(navState)
	return st
}

// navFoldOpen reports whether a fold is open: the viewer's remembered choice,
// else the default. Manage starts folded; a category's sub-categories start
// folded unless the open page is that category or one of them; every other
// section starts open.
func navFoldOpen(st navState, id string) bool {
	if v, ok := st.folds[id]; ok {
		return v
	}
	if id == "manage" {
		return false
	}
	if strings.HasPrefix(id, "sub-") {
		return id == st.curOpen
	}
	return true
}

// NavFoldOpen is navFoldOpen for the templates.
func NavFoldOpen(ctx context.Context, id string) bool {
	return navFoldOpen(navStateOf(ctx), id)
}

// NavCurrent returns the current row's key and the open page's name.
func NavCurrent(ctx context.Context) (key, page string) {
	st := navStateOf(ctx)
	return st.current, st.page
}

// NavIsCurrent reports whether key is the row for the open page.
func NavIsCurrent(ctx context.Context, key string) bool {
	return key != "" && navStateOf(ctx).current == key
}

// NavHasRing reports whether the living ring sits on this row.
func NavHasRing(ctx context.Context, key string) bool {
	return key != "" && navStateOf(ctx).ring == key
}

// NavSectionCurrent names the current row when it sits inside the given rows
// (itself or one of their sub-categories), for a folded heading to show where
// the viewer is. Empty when the current row is elsewhere.
func NavSectionCurrent(ctx context.Context, rows []NavRowView) string {
	cur := navStateOf(ctx).current
	if cur == "" {
		return ""
	}
	for _, r := range rows {
		if r.Key == cur {
			return r.Label
		}
		for _, s := range r.Subs {
			if s.Key == cur {
				return s.Label
			}
		}
	}
	return ""
}

// navShowsMe reports whether the viewer gets a My Characters row: players, and
// an owner or scribe previewing the campaign as a player.
func navShowsMe(ctx context.Context) bool {
	role := GetCampaignRole(ctx)
	return IsAuthenticated(ctx) && (role == 1 || (role >= 2 && IsViewingAsPlayer(ctx)))
}

// navSubFold is the fold id of a category's sub-category rows.
func navSubFold(typeID int) string { return "sub-" + strconv.Itoa(typeID) }

// navCount is a row's count as trailing text; zero shows nothing.
func navCount(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// navRowTrail is a row's own trailing text: a category's count, or an app's
// caption.
func navRowTrail(row NavRowView) string {
	if row.Kind == "category" {
		return navCount(row.Count)
	}
	return row.Caption
}

// navAllPagesCount is how many pages the viewer can open in All Pages: every
// top-level category's count (each already includes its sub-categories),
// whether or not the category has a row, since All Pages lists them all.
func navAllPagesCount(ctx context.Context, _ []NavRowView) string {
	counts := GetEntityCounts(ctx)
	total := 0
	for _, t := range GetEntityTypes(ctx) {
		if t.ParentTypeID == nil {
			total += counts[t.ID]
		}
	}
	return navCount(total)
}

// navInitial is the letter on a campaign's logo tile when it has no logo.
func navInitial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// navCurrentKey and navCurrentPage feed the marker App renders into
// #main-content.
func navCurrentKey(ctx context.Context) string {
	key, _ := NavCurrent(ctx)
	return key
}

func navCurrentPage(ctx context.Context) string {
	_, page := NavCurrent(ctx)
	return page
}
