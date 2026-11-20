package systems

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// RulesPages builds what a reader sees of a rules category: its list, an
// entry's page and peek, its hover card and search hits. Every answer is
// drawn from the per-reader catalog, so who sees which entry, where a
// rewritten or old name leads and what a left-out name says are decided in
// one place (entry_catalog.go) and never again here or in a handler. A
// system other than the campaign's own has no campaign overlay: its entries
// are read straight from the package.
type RulesPages struct {
	catalog  RulesCatalog
	manifest EntryManifestResolver
	people   EntryPeople
}

// RulesCatalog is the part of the system entry service the reader's pages
// draw on. SystemEntryService satisfies it.
type RulesCatalog interface {
	Catalog(ctx context.Context, campaignID, category string, reader EntryActor) (*CatalogList, error)
	ResolveAddress(ctx context.Context, campaignID, category, slug string, printed bool, reader EntryActor) (*Resolution, error)
	ResolveName(ctx context.Context, campaignID, category, name string, reader EntryActor) (*Resolution, error)
	Notes(ctx context.Context, campaignID string, actor EntryActor, entryID int64) ([]EntryNote, error)
	OldNames(ctx context.Context, campaignID, fieldKey string, actor EntryActor) ([]EntryOldName, error)
	LeftOutList(ctx context.Context, campaignID, fieldKey string) ([]LeftOut, error)
}

// NewRulesPages wires the builder. Any argument may be nil: without the
// catalog or the manifest resolver every system reads as its package, and
// without people no page draws "In <campaign>" or a peek's hero.
func NewRulesPages(catalog RulesCatalog, manifest EntryManifestResolver, people EntryPeople) *RulesPages {
	return &RulesPages{catalog: catalog, manifest: manifest, people: people}
}

// RulesReader is who is reading, as the handler knows them.
type RulesReader struct {
	CampaignID   string
	CampaignName string
	Actor        EntryActor
	// Role is the reader's visibility role for character pages.
	Role int
}

func (r RulesReader) people() PeopleReader { return PeopleReader{UserID: r.Actor.UserID, Role: r.Role} }

// overlay reports whether sys is the campaign's own system, the one its
// entries, ties and left-out entries belong to.
func (p *RulesPages) overlay(ctx context.Context, sys System, campaignID string) bool {
	if p == nil || p.catalog == nil || p.manifest == nil {
		return false
	}
	m := p.manifest(ctx, campaignID)
	return m != nil && m.ID == sys.Info().ID
}

func findCategory(m *SystemManifest, slug string) *CategoryDef {
	for i := range m.Categories {
		if m.Categories[i].Slug == slug {
			return &m.Categories[i]
		}
	}
	return nil
}

func hiddenCategory(m *SystemManifest, slug string, r RulesReader) bool {
	return m.DirectorOnly(slug) && !r.Actor.IsDirector
}

// EntryURL is an entry's own address; printed adds the printed original's mark.
func EntryURL(campaignID, systemID, category, slug string, printed bool) string {
	u := "/campaigns/" + url.PathEscape(campaignID) + "/systems/" + url.PathEscape(systemID) + "/" +
		url.PathEscape(category) + "/" + url.PathEscape(slug)
	if printed {
		u += "?printed=1"
	}
	return u
}

func categoryURL(campaignID, systemID, category string) string {
	return "/campaigns/" + url.PathEscape(campaignID) + "/systems/" + url.PathEscape(systemID) + "/" + url.PathEscape(category)
}

func stdEntryOf(m *SystemManifest, category string, it ReferenceItem) CatalogEntry {
	return CatalogEntry{Category: category, Slug: it.ID, Name: it.Name, Summary: it.Summary,
		Description: it.Description, Properties: it.Properties, Tags: it.Tags,
		Origin: OriginStandard, Credit: m.Name, Item: &it}
}

// catalog is the reader's list of a category, from the catalog for the
// campaign's system and from the package for any other.
func (p *RulesPages) catalogList(ctx context.Context, sys System, category string, r RulesReader) (*CatalogList, error) {
	if p.overlay(ctx, sys, r.CampaignID) {
		return p.catalog.Catalog(ctx, r.CampaignID, category, r.Actor)
	}
	m := sys.Info()
	out := &CatalogList{Category: category, SystemName: m.Name, Entries: []CatalogEntry{}, LeftOutNoted: []CatalogEntry{}}
	dp := sys.DataProvider()
	if hiddenCategory(m, category, r) || dp == nil {
		return out, nil
	}
	items, err := dp.List(category)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		out.Entries = append(out.Entries, stdEntryOf(m, category, it))
	}
	sort.SliceStable(out.Entries, func(a, b int) bool {
		return strings.ToLower(out.Entries[a].Name) < strings.ToLower(out.Entries[b].Name)
	})
	return out, nil
}

func (p *RulesPages) resolve(ctx context.Context, sys System, category, slug string, printed bool, r RulesReader) (*Resolution, error) {
	if p.overlay(ctx, sys, r.CampaignID) {
		return p.catalog.ResolveAddress(ctx, r.CampaignID, category, slug, printed, r.Actor)
	}
	m := sys.Info()
	dp := sys.DataProvider()
	if hiddenCategory(m, category, r) || dp == nil {
		return &Resolution{Kind: ResolvedNotFound}, nil
	}
	it, err := dp.Get(category, slug)
	if err != nil || it == nil {
		return &Resolution{Kind: ResolvedNotFound}, nil
	}
	e := stdEntryOf(m, category, *it)
	return &Resolution{Kind: ResolvedEntry, Entry: &e}, nil
}

func (p *RulesPages) resolveName(ctx context.Context, sys System, category, name string, r RulesReader) (*Resolution, error) {
	if p.overlay(ctx, sys, r.CampaignID) {
		return p.catalog.ResolveName(ctx, r.CampaignID, category, name, r.Actor)
	}
	l, err := p.catalogList(ctx, sys, category, r)
	if err != nil {
		return nil, err
	}
	for i := range l.Entries {
		if strings.EqualFold(l.Entries[i].Name, strings.TrimSpace(name)) {
			return &Resolution{Kind: ResolvedEntry, Entry: &l.Entries[i]}, nil
		}
	}
	return &Resolution{Kind: ResolvedNotFound}, nil
}

// whoseWords is the line under an entry's name saying whose words these are.
type whoseWords struct {
	Text string
	// Tone is "rewritten" (the accent edge), "credit" (quiet) or "mine" (a
	// player's own draft).
	Tone string
}

// directorMark is an amber Directors-only line, with an optional link.
type directorMark struct {
	Text     string
	LinkText string
	Href     string
}

// noteLine is one dated line the players were told.
type noteLine struct {
	At   time.Time
	Line string
}

// contentsLink is one section in the contents rail.
type contentsLink struct {
	ID    string
	Label string
}

// EntryView is an entry's page, or its peek, as one reader has it.
type EntryView struct {
	CampaignID   string
	CampaignName string
	SystemID     string
	Category     CategoryDef
	CategoryURL  string
	// Redirect is where the handler sends the reader instead (303).
	Redirect string
	URL      string
	Kind     string
	Name     string
	Entry    CatalogEntry
	// LeftOut is a left-out entry's page: only LeftOutLine, and the traits
	// for a reader whose hero still is one.
	LeftOut     bool
	LeftOutLine string
	// Printed is the Director's printed original of a rewritten entry.
	Printed    bool
	Whose      *whoseWords
	Marks      []directorMark
	Summary    string
	Story      []para
	StoryHTML  string
	InCampaign *peopleView
	Table      []tableDetail
	Notes      []noteLine
	Hero       *heroView
	IsDirector bool
	Contents   []contentsLink
}

// kindWord is the kind in small words above the name: the character field's
// own word for a kind, else the category's name made singular.
func kindWord(cat *CategoryDef, fieldKey string) string {
	if fieldKey != "" {
		return humanKey(fieldKey)
	}
	name := sentenceCase(strings.TrimSpace(cat.Name))
	if strings.Contains(name, " and ") {
		return name
	}
	switch {
	case strings.HasSuffix(name, "ies"):
		return strings.TrimSuffix(name, "ies") + "y"
	case strings.HasSuffix(name, "sses"):
		return strings.TrimSuffix(name, "es")
	}
	return singular(name)
}

// trimLeading drops a summary repeated at the start of the story.
func trimLeading(story, summary string) string {
	s, sum := strings.TrimSpace(story), strings.TrimSpace(summary)
	if sum != "" && strings.HasPrefix(s, sum) {
		return strings.TrimSpace(strings.TrimPrefix(s, sum))
	}
	return s
}

// Page builds an entry's page. A redirect answers with only Redirect set; a
// name nobody may see here is not found.
func (p *RulesPages) Page(ctx context.Context, sys System, category, slug string, printed bool, r RulesReader) (*EntryView, error) {
	m := sys.Info()
	cat := findCategory(m, category)
	if cat == nil || hiddenCategory(m, category, r) {
		return nil, apperror.NewNotFound("entry not found")
	}
	res, err := p.resolve(ctx, sys, category, slug, printed, r)
	if err != nil {
		return nil, err
	}
	switch res.Kind {
	case ResolvedNotFound:
		return nil, apperror.NewNotFound("entry not found")
	case ResolvedRedirect:
		return &EntryView{Redirect: EntryURL(r.CampaignID, m.ID, category, res.Slug, false)}, nil
	}
	v, err := p.view(ctx, sys, cat, res, r, "trait-")
	if err != nil {
		return nil, err
	}
	if err := p.fillPeople(ctx, sys, v, r); err != nil {
		return nil, err
	}
	v.Contents = contentsFor(v)
	return v, nil
}

// Peek builds an entry's peek. A redirect is followed in place, since the
// panel fetches it; with a hero the reader may see and who is of this
// entry, it names the traits that hero holds.
func (p *RulesPages) Peek(ctx context.Context, sys System, category, slug string, printed bool, heroID string, r RulesReader) (*EntryView, error) {
	m := sys.Info()
	cat := findCategory(m, category)
	if cat == nil || hiddenCategory(m, category, r) {
		return nil, apperror.NewNotFound("entry not found")
	}
	res, err := p.resolve(ctx, sys, category, slug, printed, r)
	if err != nil {
		return nil, err
	}
	switch res.Kind {
	case ResolvedNotFound:
		return nil, apperror.NewNotFound("entry not found")
	case ResolvedRedirect:
		res = &Resolution{Kind: ResolvedEntry, Entry: res.Entry}
	}
	v, err := p.view(ctx, sys, cat, res, r, "pk-")
	if err != nil {
		return nil, err
	}
	if heroID != "" && p.people != nil && v.Entry.FieldKey != "" && !v.Printed {
		if err := p.fillHero(ctx, sys, v, heroID, r); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (p *RulesPages) view(ctx context.Context, sys System, cat *CategoryDef, res *Resolution, r RulesReader, anchors string) (*EntryView, error) {
	m := sys.Info()
	e := *res.Entry
	v := &EntryView{
		CampaignID: r.CampaignID, CampaignName: r.CampaignName, SystemID: m.ID,
		Category: *cat, CategoryURL: categoryURL(r.CampaignID, m.ID, cat.Slug),
		URL:  EntryURL(r.CampaignID, m.ID, cat.Slug, e.Slug, e.Printed || res.Kind == ResolvedPrinted),
		Kind: kindWord(cat, e.FieldKey), Name: e.Name, Entry: e, IsDirector: r.Actor.IsDirector,
	}
	if res.Kind == ResolvedLeftOut {
		v.LeftOut = true
		v.LeftOutLine = NotInLine(e.Name, e.FieldKey, r.CampaignName, e.Sentence)
		if mine, err := p.readerHasHero(ctx, sys, cat.Slug, e, r); err != nil {
			return nil, err
		} else if mine {
			v.Table = buildTable(cat, e.Properties, false, anchors)
			markTraits(v.Table, itemNames(e.Properties), func(it *tableItem) { it.NoLongerOffered = true })
		}
		return v, nil
	}
	v.Printed = res.Kind == ResolvedPrinted
	p.fillWhose(ctx, sys, v, r)
	v.Summary = cleanSummary(e.Summary)
	switch {
	case e.Entry != nil, looksLikeHTML(e.Description):
		v.StoryHTML = sanitize.HTML(flattenRefMarkup(e.Description))
	default:
		v.Story = richParas(trimLeading(e.Description, e.Summary))
	}
	v.Table = buildTable(cat, e.Properties, e.Entry != nil, anchors)
	if e.EntryID != 0 && p.catalog != nil {
		notes, err := p.catalog.Notes(ctx, r.CampaignID, r.Actor, e.EntryID)
		if err != nil {
			return nil, err
		}
		for _, n := range notes {
			v.Notes = append(v.Notes, noteLine{At: n.CreatedAt, Line: n.Line})
		}
	}
	return v, nil
}

// fillWhose writes the whose-words line and the Director's amber marks.
func (p *RulesPages) fillWhose(ctx context.Context, sys System, v *EntryView, r RulesReader) {
	e := v.Entry
	switch {
	case v.Printed:
		v.Whose = &whoseWords{Text: "As " + e.Credit + " prints it", Tone: "credit"}
		mark := directorMark{Text: "Only you can read the printed " + e.Name + ". " + r.CampaignName + " reads its own."}
		if res, err := p.resolve(ctx, sys, v.Category.Slug, e.Slug, false, r); err == nil && res.Entry != nil && res.Entry.Origin == OriginRewritten {
			mark.LinkText = "Read " + r.CampaignName + "’s " + res.Entry.Name
			mark.Href = EntryURL(r.CampaignID, sys.Info().ID, v.Category.Slug, res.Entry.Slug, false)
		}
		v.Marks = append(v.Marks, mark)
	case e.Origin == OriginRewritten:
		printed := e.StandsForName
		if printed == "" {
			printed = e.Name
		}
		v.Whose = &whoseWords{Text: "Rewritten for " + r.CampaignName + ". The printed " + printed + " does not apply here.", Tone: "rewritten"}
		if r.Actor.IsDirector {
			mark := directorMark{LinkText: "Read the printed " + printed,
				Href: EntryURL(r.CampaignID, sys.Info().ID, v.Category.Slug, e.StandsFor, true)}
			if e.DirectorsOnly {
				mark.Text = "Only Directors read this " + e.Name + ". Players still read the printed one."
			}
			v.Marks = append(v.Marks, mark)
		}
	case e.Origin == OriginStandard:
		v.Whose = &whoseWords{Text: "As " + e.Credit + " prints it", Tone: "credit"}
	case e.DirectorsOnly && r.Actor.IsDirector:
		v.Marks = append(v.Marks, directorMark{Text: "Only Directors see this " + strings.ToLower(v.Kind) + "."})
	case e.Status != "" && !r.Actor.IsDirector:
		v.Whose = &whoseWords{Text: "Your draft. Only you and the Directors see it.", Tone: "mine"}
	}
}

// contentsFor lists the page's sections; a page with fewer than three needs
// no contents rail.
func contentsFor(v *EntryView) []contentsLink {
	var out []contentsLink
	if len(v.Story) > 0 || v.StoryHTML != "" {
		out = append(out, contentsLink{ID: "story", Label: "Story"})
	}
	if !v.InCampaign.empty() {
		out = append(out, contentsLink{ID: "in-campaign", Label: "In " + v.CampaignName})
	}
	if len(v.Table) > 0 {
		out = append(out, contentsLink{ID: "at-the-table", Label: "At the table"})
	}
	if len(out) < 3 {
		return nil
	}
	return out
}

// sameEntry reports whether a resolution leads to the entry on the page.
func sameEntry(res *Resolution, e CatalogEntry) bool {
	if res == nil || res.Entry == nil {
		return false
	}
	switch res.Kind {
	case ResolvedEntry, ResolvedRedirect, ResolvedLeftOut:
		return res.Entry.Slug == e.Slug && res.Entry.Printed == e.Printed && res.Entry.Origin == e.Origin
	}
	return false
}

// matchPeople keeps the characters whose stored name resolves, for this
// reader, to the entry on the page: a hero made under an old or printed name
// still appears on the entry it now leads to.
func (p *RulesPages) matchPeople(ctx context.Context, sys System, category string, e CatalogEntry, r RulesReader) ([]EntryPerson, error) {
	if p.people == nil || e.FieldKey == "" {
		return nil, nil
	}
	all, err := p.people.People(ctx, r.CampaignID, e.FieldKey, r.people(), maxPeopleScanned)
	if err != nil {
		return nil, err
	}
	leads := map[string]bool{}
	var out []EntryPerson
	for _, c := range all {
		key := strings.ToLower(strings.TrimSpace(c.Value))
		ok, seen := leads[key]
		if !seen {
			res, err := p.resolveName(ctx, sys, category, c.Value, r)
			if err != nil {
				return nil, err
			}
			ok = sameEntry(res, e)
			leads[key] = ok
		}
		if ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (p *RulesPages) readerHasHero(ctx context.Context, sys System, category string, e CatalogEntry, r RulesReader) (bool, error) {
	if r.Actor.UserID == "" {
		return false, nil
	}
	people, err := p.matchPeople(ctx, sys, category, e, r)
	if err != nil {
		return false, err
	}
	for _, c := range people {
		if c.Mine {
			return true, nil
		}
	}
	return false, nil
}

func entityLink(campaignID string, c EntryPerson) personLink {
	base := "/campaigns/" + url.PathEscape(campaignID) + "/entities/" + url.PathEscape(c.ID)
	return personLink{Name: c.Name, URL: base, Preview: base + "/preview"}
}

func (p *RulesPages) fillPeople(ctx context.Context, sys System, v *EntryView, r RulesReader) error {
	if v.LeftOut || v.Printed {
		return nil
	}
	people, err := p.matchPeople(ctx, sys, v.Category.Slug, v.Entry, r)
	if err != nil || len(people) == 0 {
		return err
	}
	pv := &peopleView{}
	for _, c := range people {
		link := entityLink(r.CampaignID, c)
		if c.Hero {
			if len(pv.Heroes) < maxPeopleShown {
				pv.Heroes = append(pv.Heroes, link)
			} else {
				pv.MoreHeroes++
			}
			continue
		}
		if len(pv.Others) < maxPeopleShown {
			pv.Others = append(pv.Others, link)
		} else {
			pv.MoreOthers++
		}
	}
	v.InCampaign = pv
	return nil
}

func (p *RulesPages) fillHero(ctx context.Context, sys System, v *EntryView, heroID string, r RulesReader) error {
	c, err := p.people.Person(ctx, r.CampaignID, heroID, v.Entry.FieldKey, r.people())
	if err != nil || c == nil {
		return err
	}
	res, err := p.resolveName(ctx, sys, v.Category.Slug, c.Value, r)
	if err != nil || !sameEntry(res, v.Entry) {
		return err
	}
	h := &heroView{ID: c.ID, Name: c.Name, Short: shortName(c.Name)}
	h.Traits = markTraits(v.Table, c.Traits, func(it *tableItem) { it.Held = true })
	v.Hero = h
	return nil
}

// jumps are the sections a phone offers under the one line: the contents
// without the story, which follows straight on.
func (v *EntryView) jumps() []contentsLink {
	var out []contentsLink
	for _, c := range v.Contents {
		if c.ID != "story" {
			out = append(out, c)
		}
	}
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }
