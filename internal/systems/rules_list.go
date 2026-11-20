package systems

import (
	"context"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// filterFrom is the list length from which a category offers its name filter.
const filterFrom = 16

// listRow is one row of a category's list: the name, then the one line, and
// at its end only what the floor allows ("Rewritten", or the credit).
type listRow struct {
	Name          string
	URL           string
	Line          string
	End           string
	DirectorsOnly bool
	// Find is the lowercase text the filter matches.
	Find string
}

// RulesListView is a category's page as one reader has it.
type RulesListView struct {
	CampaignID   string
	CampaignName string
	SystemID     string
	Category     CategoryDef
	CategoryURL  string
	// Count is the number of entries the reader has, counted from the list.
	Count int
	Rows  []listRow
	// LeftOut ends the list: the left-out entries with a sentence, and for
	// the Director every left-out one.
	LeftOut    []listRow
	Query      string
	Filterable bool
	IsDirector bool
}

// List builds a category's page; q narrows the rows by name or line.
func (p *RulesPages) List(ctx context.Context, sys System, category, q string, r RulesReader) (*RulesListView, error) {
	m := sys.Info()
	cat := findCategory(m, category)
	if cat == nil || hiddenCategory(m, category, r) {
		return nil, apperror.NewNotFound("category not found")
	}
	l, err := p.catalogList(ctx, sys, category, r)
	if err != nil {
		return nil, err
	}
	v := &RulesListView{CampaignID: r.CampaignID, CampaignName: r.CampaignName, SystemID: m.ID,
		Category: *cat, CategoryURL: categoryURL(r.CampaignID, m.ID, category),
		Count: len(l.Entries), Query: strings.TrimSpace(q), IsDirector: r.Actor.IsDirector}
	v.Filterable = v.Count >= filterFrom || v.Query != ""
	// The credit is said only where the reader could mistake whose words an
	// entry holds: in a kind the campaign has written entries of its own.
	ownWords := false
	for _, e := range l.Entries {
		if e.Origin != OriginStandard {
			ownWords = true
			break
		}
	}
	want := strings.ToLower(v.Query)
	for _, e := range l.Entries {
		row := listRow{Name: e.Name, URL: EntryURL(r.CampaignID, m.ID, category, e.Slug, e.Printed),
			Line: cleanSummary(e.Summary), DirectorsOnly: e.DirectorsOnly && r.Actor.IsDirector}
		switch {
		case e.Origin == OriginRewritten:
			row.End = "Rewritten"
		case e.Origin == OriginStandard && ownWords:
			row.End = "As " + e.Credit + " prints it"
		}
		row.Find = strings.ToLower(row.Name + " " + row.Line)
		if want != "" && !strings.Contains(row.Find, want) {
			continue
		}
		v.Rows = append(v.Rows, row)
	}
	leftOut := l.LeftOutNoted
	if r.Actor.IsDirector {
		leftOut = l.LeftOut
	}
	for _, e := range leftOut {
		row := listRow{Name: e.Name, URL: EntryURL(r.CampaignID, m.ID, category, e.Slug, false),
			Line: NotInLine(e.Name, e.FieldKey, r.CampaignName, e.Sentence),
			// One without a sentence is listed for the Director alone.
			DirectorsOnly: e.Sentence == ""}
		row.Find = strings.ToLower(row.Name + " " + row.Line)
		if want != "" && !strings.Contains(row.Find, want) {
			continue
		}
		v.LeftOut = append(v.LeftOut, row)
	}
	// The catalog keeps the package's order; a reader scans the run by name.
	sort.SliceStable(v.LeftOut, func(a, b int) bool {
		return strings.ToLower(v.LeftOut[a].Name) < strings.ToLower(v.LeftOut[b].Name)
	})
	return v, nil
}

// RulesHit is one search answer.
type RulesHit struct {
	Slug     string
	Name     string
	Category string
	Summary  string
	URL      string
	// Kind is "entry", "trait" (Name is the trait, EntryName its entry) or
	// "left_out" (Summary is the one line).
	Kind      string
	EntryName string
	// Whose is "Rewritten" on a rewritten entry.
	Whose string
}

const maxHitsPerCategory = 50

// Search finds entries of every category the reader may read. In the
// campaign's own kinds it searches the reader's list: an entry by its name,
// an old name or any of its traits' names, and a left-out name answers with
// its one line. Other categories keep the package's own search.
func (p *RulesPages) Search(ctx context.Context, sys System, query string, r RulesReader) ([]RulesHit, error) {
	query = strings.TrimSpace(query)
	m := sys.Info()
	if query == "" {
		return []RulesHit{}, nil
	}
	overlay := p.overlay(ctx, sys, r.CampaignID)
	kinds := map[string]string{}
	if overlay {
		for _, c := range m.Categories {
			if f := kindField(m, c.Slug); f != "" {
				kinds[c.Slug] = f
			}
		}
	}
	out := []RulesHit{}
	if dp := sys.DataProvider(); dp != nil {
		found, err := dp.Search(query)
		if err != nil {
			return nil, err
		}
		for _, it := range found {
			if hiddenCategory(m, it.Category, r) || kinds[it.Category] != "" {
				continue
			}
			out = append(out, RulesHit{Slug: it.ID, Name: it.Name, Category: it.Category, Summary: it.Summary,
				URL: EntryURL(r.CampaignID, m.ID, it.Category, it.ID, false), Kind: "entry"})
		}
	}
	for _, c := range m.Categories {
		field := kinds[c.Slug]
		if field == "" || hiddenCategory(m, c.Slug, r) {
			continue
		}
		hits, err := p.searchKind(ctx, sys, c.Slug, field, strings.ToLower(query), r)
		if err != nil {
			return nil, err
		}
		out = append(out, hits...)
	}
	return out, nil
}

func (p *RulesPages) searchKind(ctx context.Context, sys System, category, field, q string, r RulesReader) ([]RulesHit, error) {
	m := sys.Info()
	l, err := p.catalog.Catalog(ctx, r.CampaignID, category, r.Actor)
	if err != nil {
		return nil, err
	}
	old, err := p.catalog.OldNames(ctx, r.CampaignID, field, r.Actor)
	if err != nil {
		return nil, err
	}
	oldMatch := map[int64]bool{}
	for _, n := range old {
		if strings.Contains(strings.ToLower(n.Name), q) {
			oldMatch[n.EntryID] = true
		}
	}
	var out []RulesHit
	add := func(h RulesHit) bool {
		out = append(out, h)
		return len(out) < maxHitsPerCategory
	}
	for _, e := range l.Entries {
		h := RulesHit{Slug: e.Slug, Name: e.Name, Category: category, Summary: cleanSummary(e.Summary),
			URL: EntryURL(r.CampaignID, m.ID, category, e.Slug, e.Printed), Kind: "entry"}
		if e.Origin == OriginRewritten {
			h.Whose = "Rewritten"
		}
		if strings.Contains(strings.ToLower(e.Name), q) || (e.EntryID != 0 && oldMatch[e.EntryID]) {
			if !add(h) {
				return out, nil
			}
			continue
		}
		for _, t := range itemNames(e.Properties) {
			if strings.Contains(strings.ToLower(t), q) {
				th := h
				th.Kind, th.Name, th.EntryName = "trait", t, e.Name
				th.URL = h.URL + "#trait-" + slugFromName(t)
				if !add(th) {
					return out, nil
				}
			}
		}
	}
	outs, err := p.catalog.LeftOutList(ctx, r.CampaignID, field)
	if err != nil {
		return nil, err
	}
	dp := sys.DataProvider()
	for _, lo := range outs {
		if dp == nil {
			break
		}
		if l.Replaced(lo.Slug) {
			continue // the reader finds the rewrite above, not a left-out line
		}
		it, err := dp.Get(category, lo.Slug)
		if err != nil || it == nil || !strings.Contains(strings.ToLower(it.Name), q) {
			continue
		}
		if !add(RulesHit{Slug: it.ID, Name: it.Name, Category: category, Kind: "left_out",
			Summary: NotInLine(it.Name, field, r.CampaignName, lo.Sentence),
			URL:     EntryURL(r.CampaignID, m.ID, category, it.ID, false)}) {
			break
		}
	}
	return out, nil
}

// RulesTip is what a hover card shows for one address.
type RulesTip struct {
	Entry CatalogEntry
	URL   string
	// LeftOut marks a left-out name; Line is then its one line.
	LeftOut bool
	Line    string
	// Whose is the whose-words line for a rewritten entry.
	Whose string
}

// Tooltip resolves one address for a hover card: a rewritten or old name
// shows the entry it leads to, a left-out name its one line.
func (p *RulesPages) Tooltip(ctx context.Context, sys System, category, slug string, printed bool, r RulesReader) (*RulesTip, error) {
	m := sys.Info()
	if findCategory(m, category) == nil || hiddenCategory(m, category, r) {
		return nil, apperror.NewNotFound("item not found")
	}
	res, err := p.resolve(ctx, sys, category, slug, printed, r)
	if err != nil {
		return nil, err
	}
	if res.Kind == ResolvedNotFound || res.Entry == nil {
		return nil, apperror.NewNotFound("item not found")
	}
	e := *res.Entry
	tip := &RulesTip{Entry: e, URL: EntryURL(r.CampaignID, m.ID, category, e.Slug, e.Printed || res.Kind == ResolvedPrinted)}
	switch {
	case res.Kind == ResolvedLeftOut:
		tip.LeftOut = true
		tip.Line = NotInLine(e.Name, e.FieldKey, r.CampaignName, e.Sentence)
	case e.Origin == OriginRewritten:
		printedName := e.StandsForName
		if printedName == "" {
			printedName = e.Name
		}
		tip.Whose = "Rewritten for " + r.CampaignName + ". The printed " + printedName + " does not apply here."
	}
	return tip, nil
}
