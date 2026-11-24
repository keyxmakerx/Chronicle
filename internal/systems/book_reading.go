package systems

import (
	"context"
	"html"
	"regexp"
	"sort"
	"strings"
)

// The Handbook is the package's book, but where it lists the entries of a
// kind it must list what this reader has: the catalog's entries, a rewritten
// entry once as the campaign's with the word "Rewritten", left-out entries
// gone and the left-out entries that have a sentence closing the chapter as
// their one line. Two places in a book list entries. A rules-index chapter
// is generated from a category, so for a kind it is generated again from the
// reader's catalog. A cards block is written by hand, so only a card that
// names its entry (entry: ancestries/human) is the catalog's to change;
// matching a card to an entry by its title would be matching by name, which
// the campaign's reading never does. Directors-only entries reach only the
// Director's book because the catalog never gives them to anyone else.

// rewrittenWord opens a rewritten entry's summary in the book's lists.
const rewrittenWord = "Rewritten · "

// BookKindList is the reader's catalog of a category when it is a kind of
// the campaign's own system, else nil: that category reads as the package.
type BookKindList func(category string) (*CatalogList, error)

// BookKind is the catalog a book reads a category from, for one reader.
func (p *RulesPages) BookKind(ctx context.Context, sys System, category string, r RulesReader) (*CatalogList, error) {
	if !p.overlay(ctx, sys, r.CampaignID) || kindField(sys.Info(), category) == "" {
		return nil, nil
	}
	return p.catalog.Catalog(ctx, r.CampaignID, category, r.Actor)
}

// memoKinds asks for each category once per book.
func memoKinds(kinds BookKindList) BookKindList {
	if kinds == nil {
		return func(string) (*CatalogList, error) { return nil, nil }
	}
	seen := map[string]*CatalogList{}
	return func(cat string) (*CatalogList, error) {
		if l, ok := seen[cat]; ok {
			return l, nil
		}
		l, err := kinds(cat)
		if err != nil {
			return nil, err
		}
		seen[cat] = l
		return l, nil
	}
}

var (
	bookBlockTag = regexp.MustCompile(`(?i)</?(p|div|li|ul|ol|h[1-6]|blockquote|br)\b[^>]*>`)
	bookAnyTag   = regexp.MustCompile(`<[^>]*>`)
	bookBlankRun = regexp.MustCompile(`\n\s*\n\s*`)
	bookPartNo   = regexp.MustCompile(` \(\d+\)$`)
)

// bookText turns an entry's story into the book's plain text: a campaign
// entry's story is HTML (secret spans already stripped for players), the
// package's is already text.
func bookText(s string) string {
	if !looksLikeHTML(s) && !strings.Contains(s, "<") {
		return strings.TrimSpace(s)
	}
	s = bookBlockTag.ReplaceAllString(s, "\n\n")
	s = html.UnescapeString(bookAnyTag.ReplaceAllString(s, ""))
	return strings.TrimSpace(bookBlankRun.ReplaceAllString(s, "\n\n"))
}

// listedItem is a catalog entry as the book lists it.
func listedItem(e CatalogEntry) ReferenceItem {
	it := ReferenceItem{ID: e.Slug, Category: e.Category, Name: e.Name, Summary: e.Summary,
		Description: bookText(e.Description), Properties: e.Properties, Tags: e.Tags}
	if e.Printed {
		it.ID += "?printed=1"
	}
	if e.Origin == OriginRewritten {
		it.Summary = rewrittenWord + cleanSummary(e.Summary)
	}
	return it
}

// closingItem is a left-out entry's one line.
func closingItem(e CatalogEntry, campaignName string) ReferenceItem {
	return ReferenceItem{ID: e.Slug, Category: e.Category, Name: e.Name,
		Summary: NotInLine(e.Name, e.FieldKey, campaignName, e.Sentence)}
}

// catalogIndex is a kind's index entries for one reader, in name order,
// with the lines that close the chapter.
func catalogIndex(l *CatalogList, def *CategoryDef, campaignName string) *indexEntries {
	ie := &indexEntries{lenient: true}
	if def != nil {
		ie.fields, ie.label = def.Fields, def.Name
	}
	for _, e := range l.Entries {
		ie.items = append(ie.items, listedItem(e))
	}
	sort.SliceStable(ie.items, func(i, j int) bool { return lessFold(ie.items[i].Name, ie.items[j].Name) })
	for _, e := range l.LeftOutNoted {
		ie.closing = append(ie.closing, closingItem(e, campaignName))
	}
	return ie
}

// indexLayout reads back how a generated chapter was laid out (its group,
// per-page count and the title of its last "other" page) from its pages.
func indexLayout(ch BookChapter) (cat, group, other string, per int, ok bool) {
	per = bookIndexPerPage
	for i, p := range ch.Pages {
		c, s, isIndex := pageSlice(p)
		if !isIndex {
			return "", "", "", 0, false
		}
		if i == 0 {
			cat, group = c, s.Key
			if s.Key == "" && s.To > 0 {
				per = s.To - s.From
			}
		}
		if s.Key != "" && s.Value == "" {
			other = strings.TrimSpace(bookPartNo.ReplaceAllString(p.Title, ""))
		}
	}
	return cat, group, other, per, cat != ""
}

// readerBook rewrites a filtered book for one reader. b is FilterBook's
// copy: its chapters, pages and blocks are its own, but a block's items are
// shared with the loaded book, so they are replaced, never changed in place.
func readerBook(b *Book, m *SystemManifest, kinds BookKindList, campaignName string) (*Book, error) {
	kinds = memoKinds(kinds)
	for pi := range b.Parts {
		chapters := b.Parts[pi].Chapters[:0:0]
		for _, ch := range b.Parts[pi].Chapters {
			var err error
			if ch.Generated {
				ch, err = readerIndexChapter(ch, m, kinds, campaignName)
			} else {
				ch, err = readerCards(ch, kinds, campaignName)
			}
			if err != nil {
				return nil, err
			}
			chapters = append(chapters, ch)
		}
		b.Parts[pi].Chapters = chapters
	}
	return b, nil
}

// readerIndexChapter lays a kind's generated chapter out again from the
// reader's catalog. The chapter never goes because of what the Director
// wrote: an entry that fits no group is listed on the last, "other" page,
// and a layout that would make too many pages falls back to pages by name,
// then to one page listing every entry.
func readerIndexChapter(ch BookChapter, m *SystemManifest, kinds BookKindList, campaignName string) (BookChapter, error) {
	cat, group, other, per, ok := indexLayout(ch)
	if !ok {
		return ch, nil
	}
	l, err := kinds(cat)
	if err != nil || l == nil {
		return ch, err
	}
	ie := catalogIndex(l, findCategory(m, cat), campaignName)
	pages, problem := indexPages(ch, cat, group, other, per, ie)
	if problem != "" && group != "" {
		pages, problem = indexPages(ch, cat, "", "", per, ie)
	}
	if problem != "" || len(pages) == 0 {
		pages = []BookPage{{Title: ch.Title, Director: ch.Director,
			Blocks: []BookBlock{{Type: "index", Category: cat, Count: len(ie.items)}}}}
	}
	setIndexList(pages, ch.Pages[0].Blocks[0].List)
	ch.Pages = pages
	return ch, nil
}

// readerCards changes the cards that name their entry: a card whose entry
// this reader reads rewritten shows the campaign's, a card whose entry they
// do not have goes, and after the last such card of a kind come the
// campaign's other entries of it, then a line for each left-out entry with
// a sentence.
func readerCards(ch BookChapter, kinds BookKindList, campaignName string) (BookChapter, error) {
	type place struct{ page, block int }
	last := map[string]place{}
	placed := map[string]map[string]bool{} // category -> campaign entry slugs shown
	var order []string
	for gi := range ch.Pages {
		for bi, blk := range ch.Pages[gi].Blocks {
			if blk.Type != "cards" || !hasEntryCard(blk) {
				continue
			}
			items := make([]BookItem, 0, len(blk.Items))
			for _, it := range blk.Items {
				cat, slug, tied := strings.Cut(it.Entry, "/")
				if !tied {
					items = append(items, it)
					continue
				}
				l, err := kinds(cat)
				if err != nil {
					return ch, err
				}
				if l == nil {
					items = append(items, it)
					continue
				}
				if _, seen := last[cat]; !seen {
					order = append(order, cat)
					placed[cat] = map[string]bool{}
				}
				last[cat] = place{gi, bi}
				e := cardEntry(l, slug)
				switch {
				case e == nil:
					// Left out, or not this reader's to read.
				case e.Origin == OriginStandard:
					items = append(items, it)
				default:
					placed[cat][e.Slug] = true
					items = append(items, entryCard(*e, it.Entry))
				}
			}
			ch.Pages[gi].Blocks[bi].Items = items
		}
	}
	for _, cat := range order {
		l, _ := kinds(cat)
		at := last[cat]
		blk := &ch.Pages[at.page].Blocks[at.block]
		for _, e := range l.Entries {
			if e.Origin != OriginStandard && !placed[cat][e.Slug] {
				blk.Items = append(blk.Items, entryCard(e, cat+"/"+e.Slug))
			}
		}
		if len(l.LeftOutNoted) > 0 {
			var lines []string
			for _, e := range l.LeftOutNoted {
				lines = append(lines, NotInLine(e.Name, e.FieldKey, campaignName, e.Sentence))
			}
			lastPage := &ch.Pages[len(ch.Pages)-1]
			lastPage.Blocks = append(lastPage.Blocks, BookBlock{Type: "text", Text: strings.Join(lines, "\n\n")})
		}
	}
	// A block left with no cards is not drawn as an empty grid.
	for gi := range ch.Pages {
		blocks := ch.Pages[gi].Blocks[:0:0]
		for _, blk := range ch.Pages[gi].Blocks {
			if blk.Type == "cards" && len(blk.Items) == 0 {
				continue
			}
			blocks = append(blocks, blk)
		}
		ch.Pages[gi].Blocks = blocks
	}
	return ch, nil
}

func hasEntryCard(b BookBlock) bool {
	for _, it := range b.Items {
		if it.Entry != "" {
			return true
		}
	}
	return false
}

// cardEntry is the entry a tied card stands for in this reader's catalog:
// the campaign's rewrite of it, the standard entry itself, or nil.
func cardEntry(l *CatalogList, slug string) *CatalogEntry {
	for i := range l.Entries {
		if e := &l.Entries[i]; e.Origin == OriginRewritten && e.StandsFor == slug {
			return e
		}
	}
	for i := range l.Entries {
		if e := &l.Entries[i]; e.Origin == OriginStandard && e.Slug == slug {
			return e
		}
	}
	return nil
}

// entryCard is a campaign entry as a card.
func entryCard(e CatalogEntry, ref string) BookItem {
	it := listedItem(e)
	return BookItem{Title: it.Name, Summary: cleanSummary(it.Summary), Text: it.Description, Entry: ref}
}
