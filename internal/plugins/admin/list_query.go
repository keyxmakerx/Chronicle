// list_query.go is the shared contract of the admin lists that have a search
// box, filter chips and pagination: how the query string is bound and
// validated, and the view model the list toolbar renders. Handlers call
// these so every list clamps and trims its input the same way.

package admin

import (
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// adminListPerPage is the page size of the searchable admin lists.
	adminListPerPage = 25
	// maxListSearchLen caps the search text so a pasted blob cannot become
	// a giant LIKE pattern.
	maxListSearchLen = 100
	// maxListPage keeps (page-1)*perPage far from integer overflow.
	maxListPage = 1_000_000
)

// listQuery is the bound, validated query string of an admin list.
type listQuery struct {
	// Q is the trimmed, length-capped search text.
	Q string
	// Filter is the raw chip value; the handler checks it against the
	// vocabulary of its own list and falls back to "all".
	Filter string
	// Page is 1-based and always >= 1.
	Page int
}

// parseListQuery binds ?q=, ?f= and ?page=. It never fails: bad input
// degrades to the default list rather than an error page, because these are
// URLs people edit and bookmark.
func parseListQuery(q, f, page string) listQuery {
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) > maxListSearchLen {
		q = strings.TrimSpace(string([]rune(q)[:maxListSearchLen]))
	}
	// A NUL or other control character is never a real search and only
	// risks surprising the driver or the logs.
	q = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, q)

	p, err := strconv.Atoi(strings.TrimSpace(page))
	if err != nil || p < 1 {
		p = 1
	}
	if p > maxListPage {
		p = maxListPage
	}
	return listQuery{Q: q, Filter: strings.TrimSpace(f), Page: p}
}

// clampPage pulls a page past the end back to the last page, so a stale
// link after a filter change shows results instead of an empty state.
func clampPage(page, total, perPage int) int {
	if perPage < 1 {
		return 1
	}
	last := (total + perPage - 1) / perPage
	if last < 1 {
		last = 1
	}
	if page > last {
		return last
	}
	if page < 1 {
		return 1
	}
	return page
}

// listChip is one filter chip. Value is what ?f= carries ("" is All).
type listChip struct {
	Label  string
	Value  string
	Count  int
	Active bool
}

// listView is everything the toolbar and pager need to render one list.
type listView struct {
	// BaseURL is the list's own path, e.g. "/admin/users".
	BaseURL string
	// RegionID is the DOM id of the swappable list region.
	RegionID string
	// Placeholder is the search box hint.
	Placeholder string
	// Noun names the rows in the empty state ("people", "campaigns").
	Noun    string
	Query   string
	Filter  string
	Chips   []listChip
	Total   int
	Page    int
	PerPage int
}

// href builds a list URL for a chip or page: only non-default params appear
// so the canonical list URL stays clean.
func (v listView) href(filter string, page int) string {
	vals := url.Values{}
	if v.Query != "" {
		vals.Set("q", v.Query)
	}
	if filter != "" {
		vals.Set("f", filter)
	}
	if page > 1 {
		vals.Set("page", strconv.Itoa(page))
	}
	if len(vals) == 0 {
		return v.BaseURL
	}
	return v.BaseURL + "?" + vals.Encode()
}

// ChipURL is the link for a chip; changing the chip resets to page 1.
func (v listView) ChipURL(filter string) string { return v.href(filter, 1) }

// ClearSearchURL is the list URL with the search text dropped but the chip kept.
func (v listView) ClearSearchURL() string {
	c := v
	c.Query = ""
	return c.href(v.Filter, 1)
}

// PagerParams is the extra query string the shared pager appends after
// ?page=N, so paging keeps the active search and chip.
func (v listView) PagerParams() string {
	vals := url.Values{}
	if v.Filter != "" {
		vals.Set("f", v.Filter)
	}
	if v.Query != "" {
		vals.Set("q", v.Query)
	}
	return vals.Encode()
}

// Searching reports whether any narrowing is active; the empty state words
// itself differently for "nothing exists" and "nothing matches".
func (v listView) Searching() bool { return v.Query != "" || v.Filter != "" }
