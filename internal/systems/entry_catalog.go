package systems

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// The catalog is the one per-reader list of a rules category. The hero
// creator draws on it through PickList; the category page, entry page and
// peek, rules and global search, hover cards and the Handbook's kind lists
// draw on it through RulesPages; so their counts always agree. For a reader it
// holds the standard entries not replaced for them and not left out, plus
// the campaign entries they may see (systemEntryService.visible decides,
// secret spans already stripped for players). A tied campaign entry replaces
// its standard entry for exactly the readers who may see it, so a
// Directors-only stand-in is the Director's draft and players still read the
// printed one. Nothing is ever matched by name.

// ToldWindow is how long a change the players were told about stays marked.
const ToldWindow = 14 * 24 * time.Hour

// Where an entry's words come from.
const (
	// OriginCampaign is an entry the campaign wrote beside the package.
	OriginCampaign = "campaign"
	// OriginRewritten is a campaign entry that takes a standard entry's place.
	OriginRewritten = "rewritten"
	// OriginStandard is the package's own entry, credited to the system.
	OriginStandard = "standard"
)

// What resolving an address or a name gave.
const (
	// ResolvedEntry: this is the entry to show.
	ResolvedEntry = "entry"
	// ResolvedRedirect: go to Resolution.Slug instead (Entry is its target).
	ResolvedRedirect = "redirect"
	// ResolvedLeftOut: the campaign does not offer it; Entry still carries
	// its content, so a hero who already is one can read the traits.
	ResolvedLeftOut = "left_out"
	// ResolvedPrinted: the Director's printed original of a rewritten entry.
	ResolvedPrinted = "printed"
	// ResolvedNotFound: nothing this reader may see answers.
	ResolvedNotFound = "not_found"
)

// Why a redirect leads elsewhere.
const (
	ViaRewritten = "rewritten"
	ViaOldName   = "old_name"
	ViaOldSlug   = "old_slug"
)

// CatalogEntry is one entry as a reader has it.
type CatalogEntry struct {
	Category string `json:"category"`
	// FieldKey is the character field of a kind; "" for other categories.
	FieldKey string `json:"fieldKey,omitempty"`
	// Slug is the entry's address under the category.
	Slug string `json:"slug"`
	// Printed marks a standard entry whose plain address a campaign entry of
	// the same slug holds; its own address carries ?printed=1.
	Printed     bool           `json:"printed,omitempty"`
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	Description string         `json:"description,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	// Origin is OriginCampaign, OriginRewritten or OriginStandard.
	Origin string `json:"origin"`
	// Credit is the system's name on a standard entry ("Draw Steel").
	Credit string `json:"credit,omitempty"`
	// StandsFor and StandsForName name the standard entry a rewritten one
	// replaces.
	StandsFor     string `json:"standsFor,omitempty"`
	StandsForName string `json:"standsForName,omitempty"`
	// DirectorsOnly is true when only Directors (and a draft's writer) see it.
	DirectorsOnly bool `json:"directorsOnly,omitempty"`
	// KeptFor names the one character an entry is kept to.
	KeptFor string `json:"keptFor,omitempty"`
	// LeftOut and Sentence describe a standard entry the campaign left out.
	LeftOut  bool   `json:"leftOut,omitempty"`
	Sentence string `json:"sentence,omitempty"`
	// EntryID is the campaign entry's id; 0 for a standard entry.
	EntryID int64 `json:"entryId,omitempty"`
	// Status is "draft" or "sent" on a player's entry not yet accepted.
	Status string `json:"status,omitempty"`
	// ToldRecently is true when the players were told about a change to this
	// entry within the last ToldWindow, so a hero's sheet can mark it.
	ToldRecently bool `json:"toldRecently,omitempty"`

	// Entry and Item are the underlying rows, for callers that render more.
	Entry *SystemEntry   `json:"-"`
	Item  *ReferenceItem `json:"-"`
}

// CatalogList is a category as one reader has it.
type CatalogList struct {
	Category   string `json:"category"`
	FieldKey   string `json:"fieldKey,omitempty"`
	SystemName string `json:"systemName"`
	// Entries are the entries this reader has, by name.
	Entries []CatalogEntry `json:"entries"`
	// LeftOut is every left-out standard entry, for Directors only.
	LeftOut []CatalogEntry `json:"leftOut,omitempty"`
	// LeftOutNoted are the left-out entries with a sentence, which every
	// reader's list ends with.
	LeftOutNoted []CatalogEntry `json:"leftOutNoted"`
}

// Replaced reports whether a standard slug is read rewritten in this list,
// so a caller adding left-out package content skips a slug that is both.
func (l *CatalogList) Replaced(slug string) bool {
	for i := range l.Entries {
		if l.Entries[i].Origin == OriginRewritten && l.Entries[i].StandsFor == slug {
			return true
		}
	}
	return false
}

// Resolution is what one address or one name means for a reader.
type Resolution struct {
	// Kind is one of the Resolved* values.
	Kind string `json:"kind"`
	// Entry is the entry to show, the redirect's target, or the left-out
	// entry's content; nil when not found.
	Entry *CatalogEntry `json:"entry,omitempty"`
	// Slug is the address to go to on a redirect.
	Slug string `json:"slug,omitempty"`
	// Via says why a redirect leads elsewhere (Via* values).
	Via string `json:"via,omitempty"`
}

// EntryCatalog is the per-reader list and resolver.
type EntryCatalog interface {
	// Catalog returns a category as the reader has it. A Directors-only
	// category is empty for anyone else.
	Catalog(ctx context.Context, campaignID, category string, reader EntryActor) (*CatalogList, error)
	// FieldCatalog is Catalog for the category a character field's pick list
	// draws from; nil when the campaign has no game system with that field.
	FieldCatalog(ctx context.Context, campaignID, fieldKey string, reader EntryActor) (*CatalogList, error)
	// ResolveAddress resolves a slug under a category. printed asks for the
	// printed original, which only a Director gets for a rewritten entry;
	// for anyone else it answers as if not asked.
	ResolveAddress(ctx context.Context, campaignID, category, slug string, printed bool, reader EntryActor) (*Resolution, error)
	// ResolveName resolves a name (what a hero stores) the same way: the
	// stand-in a reader sees for a standard name, an old name's entry, or
	// left out.
	ResolveName(ctx context.Context, campaignID, category, name string, reader EntryActor) (*Resolution, error)
	// ResolveNames resolves many names against one reading built once, so a
	// page that resolves every hero's stored name costs the same few queries
	// however many heroes there are. The answers are in the order of names.
	ResolveNames(ctx context.Context, campaignID, category string, names []string, reader EntryActor) ([]*Resolution, error)
	// PickList is the hero creator's list for a field: the package's
	// choices merged with the campaign's for the viewer the context marks
	// (ChoiceViewerIsDirector, ChoiceViewerUser), by the catalog's rules.
	PickList(ctx context.Context, campaignID, systemID, fieldKey string, pkg []Choice) ([]Choice, error)
}

// readingPlan is the catalog's merge, over slugs only, so the catalog (fed
// reference items) and the pick list (fed choice files) share one rule.
type readingPlan struct {
	offered    []int        // standard indexes this reader is offered, in order
	printed    map[int]bool // standard indexes whose plain slug a campaign entry holds
	leftOut    []int        // standard indexes left out
	replacedBy map[int]int  // standard index -> campaign index that replaces it
	standsFor  map[int]int  // campaign index -> standard index it replaces
}

// planReading merges standard slugs with the campaign entries one reader may
// see. A standard entry is replaced only through a visible entry's tie.
func planReading(stdSlugs []string, camp []SystemEntry, leftOut map[string]LeftOut) readingPlan {
	p := readingPlan{printed: map[int]bool{}, replacedBy: map[int]int{}, standsFor: map[int]int{}}
	bySlug := map[string]int{}
	for i, s := range stdSlugs {
		if _, dup := bySlug[s]; !dup && s != "" {
			bySlug[s] = i
		}
	}
	campSlugs := map[string]bool{}
	for j, e := range camp {
		campSlugs[e.Slug] = true
		if e.StandsFor == "" {
			continue
		}
		if i, ok := bySlug[e.StandsFor]; ok {
			if _, taken := p.replacedBy[i]; !taken {
				p.replacedBy[i] = j
				p.standsFor[j] = i
			}
		}
	}
	for i, s := range stdSlugs {
		// Replaced comes before left out. The service refuses both at once,
		// but two saves racing can leave both; the reader then has the
		// rewrite alone, and the printed entry never comes with it, not even
		// as a left-out entry carrying its words.
		if _, replaced := p.replacedBy[i]; replaced {
			continue
		}
		if _, out := leftOut[s]; out && s != "" {
			p.leftOut = append(p.leftOut, i)
			continue
		}
		p.offered = append(p.offered, i)
		if campSlugs[s] {
			p.printed[i] = true
		}
	}
	return p
}

// readingState is everything one category needs for one reader.
type readingState struct {
	category   string
	fieldKey   string
	systemName string
	hidden     bool
	std        []ReferenceItem
	camp       []SystemEntry
	leftOut    map[string]LeftOut
	told       map[int64]bool
	// oldNames is the kind's old names, loaded only by the resolvers.
	oldNames []EntryOldName
	plan     readingPlan
}

func (s *systemEntryService) readingFor(ctx context.Context, campaignID, category string, reader EntryActor) (*readingState, error) {
	st := &readingState{category: category, leftOut: map[string]LeftOut{}}
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil {
		st.plan = planReading(nil, nil, nil)
		return st, nil
	}
	st.systemName = m.Name
	if m.DirectorOnly(category) && !reader.IsDirector {
		st.hidden = true
		st.plan = planReading(nil, nil, nil)
		return st, nil
	}
	if sys := s.standardSystem(ctx, campaignID, m); sys != nil {
		if sys.Info().Name != "" {
			st.systemName = sys.Info().Name
		}
		items, err := sys.DataProvider().List(category)
		if err != nil {
			return nil, err
		}
		st.std = items
	}
	st.fieldKey = kindField(m, category)
	if st.fieldKey != "" {
		entries, err := s.repo.List(ctx, campaignID, m.ID, st.fieldKey, reader.IsDirector)
		if err != nil {
			return nil, err
		}
		if st.camp, err = s.visible(ctx, campaignID, entries, reader); err != nil {
			return nil, err
		}
		outs, err := s.repo.ListLeftOut(ctx, campaignID, m.ID, st.fieldKey)
		if err != nil {
			return nil, err
		}
		for _, l := range outs {
			st.leftOut[l.Slug] = l
		}
		if len(st.camp) > 0 {
			if st.told, err = s.repo.NotedSince(ctx, campaignID, time.Now().Add(-ToldWindow)); err != nil {
				return nil, err
			}
		}
	}
	slugs := make([]string, len(st.std))
	for i := range st.std {
		slugs[i] = st.std[i].ID
	}
	st.plan = planReading(slugs, st.camp, st.leftOut)
	return st, nil
}

func (st *readingState) stdEntry(i int) CatalogEntry {
	it := st.std[i]
	e := CatalogEntry{
		Category: st.category, FieldKey: st.fieldKey, Slug: it.ID, Printed: st.plan.printed[i],
		Name: it.Name, Summary: it.Summary, Description: it.Description, Properties: it.Properties, Tags: it.Tags,
		Origin: OriginStandard, Credit: st.systemName, Item: &it,
	}
	if l, out := st.leftOut[it.ID]; out {
		e.LeftOut, e.Sentence = true, l.Sentence
	}
	return e
}

func (st *readingState) campEntry(j int) CatalogEntry {
	row := st.camp[j]
	sum := row.Summary
	if sum == "" {
		sum = cleanSummary(row.Description)
	}
	e := CatalogEntry{
		Category: st.category, FieldKey: st.fieldKey, Slug: row.Slug,
		Name: row.Name, Summary: sum, Description: row.Description, Properties: row.Properties,
		Origin: OriginCampaign, EntryID: row.ID, KeptFor: row.ForName, Entry: &row,
		DirectorsOnly: row.Visibility == EntryVisibilityDirectors || row.Status != EntryStatusAccepted,
		ToldRecently:  st.told[row.ID],
	}
	if row.Status != EntryStatusAccepted {
		e.Status = row.Status
	}
	if row.StandsFor != "" {
		e.Origin, e.StandsFor = OriginRewritten, row.StandsFor
		if i, ok := st.plan.standsFor[j]; ok {
			e.StandsForName = st.std[i].Name
		} else {
			var snap StandardSnapshot
			_ = json.Unmarshal([]byte(row.StandsForSeen), &snap)
			e.StandsForName = snap.Name
		}
	}
	return e
}

func (s *systemEntryService) FieldCatalog(ctx context.Context, campaignID, fieldKey string, reader EntryActor) (*CatalogList, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil || !ValidChoiceFieldKey(fieldKey) || !isCharacterStringField(m, fieldKey) {
		return nil, nil
	}
	return s.Catalog(ctx, campaignID, kindCategory(m, fieldKey), reader)
}

func (s *systemEntryService) Catalog(ctx context.Context, campaignID, category string, reader EntryActor) (*CatalogList, error) {
	st, err := s.readingFor(ctx, campaignID, category, reader)
	if err != nil {
		return nil, err
	}
	out := &CatalogList{Category: category, FieldKey: st.fieldKey, SystemName: st.systemName,
		Entries: []CatalogEntry{}, LeftOutNoted: []CatalogEntry{}}
	if st.hidden {
		return out, nil
	}
	for _, i := range st.plan.offered {
		out.Entries = append(out.Entries, st.stdEntry(i))
	}
	for j := range st.camp {
		out.Entries = append(out.Entries, st.campEntry(j))
	}
	// By name; on equal names the campaign's comes first, as it holds the
	// plain address.
	sort.SliceStable(out.Entries, func(a, b int) bool {
		na, nb := strings.ToLower(out.Entries[a].Name), strings.ToLower(out.Entries[b].Name)
		if na != nb {
			return na < nb
		}
		return out.Entries[a].Origin != OriginStandard && out.Entries[b].Origin == OriginStandard
	})
	for _, i := range st.plan.leftOut {
		e := st.stdEntry(i)
		if reader.IsDirector {
			out.LeftOut = append(out.LeftOut, e)
		}
		if e.Sentence != "" {
			out.LeftOutNoted = append(out.LeftOutNoted, e)
		}
	}
	return out, nil
}

func (s *systemEntryService) ResolveAddress(ctx context.Context, campaignID, category, slug string, printed bool, reader EntryActor) (*Resolution, error) {
	st, err := s.readingFor(ctx, campaignID, category, reader)
	if err != nil {
		return nil, err
	}
	slug = strings.TrimSpace(slug)
	if st.hidden || slug == "" {
		return &Resolution{Kind: ResolvedNotFound}, nil
	}
	stdIdx := -1
	for i := range st.std {
		if st.std[i].ID == slug {
			stdIdx = i
			break
		}
	}
	if printed && stdIdx >= 0 {
		_, replaced := st.plan.replacedBy[stdIdx]
		_, out := st.leftOut[slug]
		switch {
		case replaced && reader.IsDirector:
			e := st.stdEntry(stdIdx)
			return &Resolution{Kind: ResolvedPrinted, Entry: &e}, nil
		case replaced:
			// Anyone else asking for the printed original of what they read
			// rewritten is answered as if they had not asked, even when the
			// entry is also marked left out.
		case out:
			return st.leftOutRes(stdIdx), nil
		default:
			e := st.stdEntry(stdIdx)
			return &Resolution{Kind: ResolvedEntry, Entry: &e}, nil
		}
	}
	for j := range st.camp {
		if st.camp[j].Slug == slug {
			e := st.campEntry(j)
			return &Resolution{Kind: ResolvedEntry, Entry: &e}, nil
		}
	}
	if stdIdx >= 0 {
		if j, replaced := st.plan.replacedBy[stdIdx]; replaced {
			e := st.campEntry(j)
			return &Resolution{Kind: ResolvedRedirect, Entry: &e, Slug: e.Slug, Via: ViaRewritten}, nil
		}
		if _, out := st.leftOut[slug]; out {
			return st.leftOutRes(stdIdx), nil
		}
		e := st.stdEntry(stdIdx)
		return &Resolution{Kind: ResolvedEntry, Entry: &e}, nil
	}
	// An old slug comes last, after the package: slugs never change on a
	// rename, so a link made to this entry always used its own slug, and an
	// old name's slug that a package entry holds was never this entry's
	// address. (Names are different: see resolveName.)
	if err := s.loadOldNames(ctx, campaignID, st); err != nil {
		return nil, err
	}
	for _, n := range st.oldNames {
		if slugFromName(n.Name) != slug {
			continue
		}
		if r := st.redirectTo(n.EntryID, ViaOldSlug); r != nil {
			return r, nil
		}
	}
	return &Resolution{Kind: ResolvedNotFound}, nil
}

func (s *systemEntryService) ResolveName(ctx context.Context, campaignID, category, name string, reader EntryActor) (*Resolution, error) {
	out, err := s.ResolveNames(ctx, campaignID, category, []string{name}, reader)
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

func (s *systemEntryService) ResolveNames(ctx context.Context, campaignID, category string, names []string, reader EntryActor) ([]*Resolution, error) {
	st, err := s.readingFor(ctx, campaignID, category, reader)
	if err != nil {
		return nil, err
	}
	if err := s.loadOldNames(ctx, campaignID, st); err != nil {
		return nil, err
	}
	out := make([]*Resolution, len(names))
	for i, n := range names {
		out[i] = st.resolveName(n)
	}
	return out, nil
}

// loadOldNames reads the kind's old names once for every resolution against
// this reading. Categories that are not kinds have none.
func (s *systemEntryService) loadOldNames(ctx context.Context, campaignID string, st *readingState) error {
	if st.fieldKey == "" || st.hidden || st.oldNames != nil {
		return nil
	}
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil {
		return nil
	}
	names, err := s.repo.ListOldNames(ctx, campaignID, m.ID, st.fieldKey)
	if err != nil {
		return err
	}
	st.oldNames = names
	if st.oldNames == nil {
		st.oldNames = []EntryOldName{}
	}
	return nil
}

// resolveName answers one name, in this order:
//  1. a campaign entry the reader sees with that name: in the campaign its
//     words hold the plain name;
//  2. a package entry of that name the reader reads rewritten: the tie is
//     the Director's explicit word about that name;
//  3. an old name of a campaign entry the reader sees: heroes store names,
//     so a renamed entry keeps the heroes made under its old name even when
//     a package entry has that name too (a Director who wants the printed
//     one back under it frees the old name);
//  4. the package entry of that name, left out or not.
//
// Names are matched case-insensitively first, then, only when nothing
// answers, also ignoring accents and Unicode normal form (sameNameLoose). The
// first pass is the whole rule as it stands, so no name that resolves today
// moves; the second keeps finding what the database's collation matched
// when old names were looked up there ("Eladrin" for "Éladrin").
func (st *readingState) resolveName(name string) *Resolution {
	if r := st.resolveNameBy(name, strings.EqualFold); r.Kind != ResolvedNotFound {
		return r
	}
	return st.resolveNameBy(name, sameNameLoose)
}

// nameKey is a name as utf8mb4_unicode_ci compares it, near enough for
// names: trimmed, decomposed, accents dropped, lower case.
func nameKey(s string) string {
	d := norm.NFD.String(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range d {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func sameNameLoose(a, b string) bool { return nameKey(a) == nameKey(b) }

func (st *readingState) resolveNameBy(name string, same func(a, b string) bool) *Resolution {
	name = strings.TrimSpace(name)
	if st.hidden || name == "" {
		return &Resolution{Kind: ResolvedNotFound}
	}
	for j := range st.camp {
		if same(st.camp[j].Name, name) {
			e := st.campEntry(j)
			return &Resolution{Kind: ResolvedEntry, Entry: &e}
		}
	}
	std := -1
	for i := range st.std {
		if same(st.std[i].Name, name) {
			std = i
			break
		}
	}
	if std >= 0 {
		if j, replaced := st.plan.replacedBy[std]; replaced {
			e := st.campEntry(j)
			return &Resolution{Kind: ResolvedRedirect, Entry: &e, Slug: e.Slug, Via: ViaRewritten}
		}
	}
	for _, n := range st.oldNames {
		if same(n.Name, name) {
			if r := st.redirectTo(n.EntryID, ViaOldName); r != nil {
				return r
			}
			break
		}
	}
	if std >= 0 {
		if _, out := st.leftOut[st.std[std].ID]; out {
			return st.leftOutRes(std)
		}
		e := st.stdEntry(std)
		return &Resolution{Kind: ResolvedEntry, Entry: &e}
	}
	return &Resolution{Kind: ResolvedNotFound}
}

func (st *readingState) leftOutRes(i int) *Resolution {
	e := st.stdEntry(i)
	return &Resolution{Kind: ResolvedLeftOut, Entry: &e}
}

// redirectTo leads to a campaign entry only when this reader may see it, so
// an old name never gives a hidden entry away.
func (st *readingState) redirectTo(entryID int64, via string) *Resolution {
	for j := range st.camp {
		if st.camp[j].ID == entryID {
			e := st.campEntry(j)
			return &Resolution{Kind: ResolvedRedirect, Entry: &e, Slug: e.Slug, Via: via}
		}
	}
	return nil
}

func (s *systemEntryService) PickList(ctx context.Context, campaignID, systemID, fieldKey string, pkg []Choice) ([]Choice, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil || !ValidChoiceFieldKey(fieldKey) {
		return pkg, nil
	}
	actor := EntryActor{UserID: ChoiceViewerUser(ctx), IsDirector: ChoiceViewerIsDirector(ctx)}
	entries, err := s.repo.List(ctx, campaignID, m.ID, fieldKey, actor.IsDirector)
	if err != nil {
		return nil, err
	}
	if entries, err = s.visible(ctx, campaignID, entries, actor); err != nil {
		return nil, err
	}
	leftOut := map[string]LeftOut{}
	slugs := make([]string, len(pkg))
	// Ties and left-out entries name slugs of the campaign's own system; a
	// list drawn from another system's file keeps every row.
	if systemID == m.ID {
		outs, err := s.repo.ListLeftOut(ctx, campaignID, m.ID, fieldKey)
		if err != nil {
			return nil, err
		}
		for _, l := range outs {
			leftOut[l.Slug] = l
		}
		for i := range pkg {
			slugs[i] = pkg[i].Slug
		}
	}
	plan := planReading(slugs, entries, leftOut)
	var told map[int64]bool
	if len(entries) > 0 {
		if told, err = s.repo.NotedSince(ctx, campaignID, time.Now().Add(-ToldWindow)); err != nil {
			return nil, err
		}
	}
	out := make([]Choice, 0, len(plan.offered)+len(entries))
	for _, i := range plan.offered {
		c := pkg[i]
		c.Origin = OriginStandard
		out = append(out, c)
	}
	for j, e := range entries {
		c := entryChoice(e, actor)
		c.ToldRecently = told[e.ID]
		if _, ok := plan.standsFor[j]; ok {
			c.Origin, c.StandsFor = OriginRewritten, e.StandsFor
		}
		out = append(out, c)
	}
	return out, nil
}
