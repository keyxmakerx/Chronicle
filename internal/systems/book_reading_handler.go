package systems

import (
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// bookKinds reads a kind's entries from the reader's catalog for the book
// routes. director is the book's own view: a Director previewing the player
// view reads as a player.
func (h *SystemHandler) bookKinds(c *echo.Context, director bool) (BookKindList, string) {
	cc := campaigns.GetCampaignContext(c)
	mod := h.resolveSystem(c)
	if cc == nil || mod == nil {
		return nil, ""
	}
	r := rulesReader(c, cc)
	r.Actor.IsDirector = director
	ctx := c.Request().Context()
	pages := h.rulesPages()
	return func(cat string) (*CatalogList, error) { return pages.BookKind(ctx, mod, cat, r) }, cc.Campaign.Name
}

// forReader rewrites a filtered book for the reader asking (book_reading.go).
func (h *SystemHandler) forReader(c *echo.Context, b *Book, m *SystemManifest, director bool) (*Book, error) {
	kinds, campaignName := h.bookKinds(c, director)
	if kinds == nil {
		return b, nil
	}
	return readerBook(b, m, kinds, campaignName)
}

// readerIndex is one category's index entries for the reader: a kind's from
// their catalog, any other category's from the package file.
func (h *SystemHandler) readerIndex(c *echo.Context, sysDir string, m *SystemManifest, cat string, director bool) (*indexEntries, error) {
	kinds, campaignName := h.bookKinds(c, director)
	return indexWith(kinds, campaignName, sysDir, m, cat)
}

func indexWith(kinds BookKindList, campaignName, sysDir string, m *SystemManifest, cat string) (*indexEntries, error) {
	if kinds != nil {
		l, err := kinds(cat)
		if err != nil {
			return nil, err
		}
		if l != nil {
			return catalogIndex(l, findCategory(m, cat), campaignName), nil
		}
	}
	return loadIndexEntries(sysDir, m, cat)
}

// readerFind is the book search for the reader. The cached index serves
// every book whose generated chapters list no kind; one that lists a kind
// is searched as this reader's book.
func (h *SystemHandler) readerFind(c *echo.Context, sysDir string, m *SystemManifest, director bool, cached *bookFindIndex) (*bookFindIndex, error) {
	kinds, campaignName := h.bookKinds(c, director)
	if kinds == nil {
		return cached, nil
	}
	kinds = memoKinds(kinds)
	listsKind := false
	for cat := range cached.cats {
		l, err := kinds(cat)
		if err != nil {
			return nil, err
		}
		listsKind = listsKind || l != nil
	}
	if !listsKind {
		return cached, nil
	}
	b, err := LoadBook(sysDir, m)
	if err != nil {
		return nil, err
	}
	rb, err := readerBook(FilterBook(b, director), m, kinds, campaignName)
	if err != nil {
		return nil, err
	}
	return buildBookFindIndexWith(rb, func(cat string) (*indexEntries, error) {
		return indexWith(kinds, campaignName, sysDir, m, cat)
	}), nil
}

// sliceIsLast reports whether the page showing want is its chapter's last,
// where a kind's closing lines are listed.
func sliceIsLast(b *Book, category string, want bookIndexSlice) bool {
	for _, ch := range indexChapters(b) {
		for pi, p := range ch.Pages {
			if pc, s, ok := pageSlice(p); ok && pc == category && s == want {
				return pi == len(ch.Pages)-1
			}
		}
	}
	return false
}
