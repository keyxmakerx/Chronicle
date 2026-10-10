package records

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// KindLookup is the front-matter kind of a read-only question an AI asks
// Chronicle ("which events fall in Deepwinter?"). Lookups never reach a
// Kind: they are answered, not reviewed, and nothing is written.
const KindLookup = "lookup"

// Answers are size-capped so a lookup can never paste the whole world back:
// each answer, all answers together, and the lines in one list.
const (
	maxLookups      = 20
	maxAnswerBytes  = 24 * 1024
	maxAnswersBytes = 120 * 1024
	maxAnswerLines  = 300
	maxCalMonths    = 24
	maxPagesWalk    = 50 // pages of 100, as the AI export walks them
)

// PagesAPI lists campaign pages as a viewer of the given role sees them
// (entities.EntityService).
type PagesAPI interface {
	List(ctx context.Context, campaignID string, typeID int, role int, userID string, opts entities.ListOptions) ([]entities.Entity, int, error)
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
}

// SystemEntry is one game-system pick-list entry (an ancestry, a kit…).
type SystemEntry struct{ Name, Slug, Source, Summary string }

// SystemEntriesAPI lists the campaign system's entries for one character
// field, package and campaign-made alike. director includes entries kept
// from players.
type SystemEntriesAPI interface {
	Entries(ctx context.Context, campaignID, field string, director bool) (system string, list []SystemEntry, err error)
}

// PartyAPI is the party as the DM Screen shows it, plus the next game night.
type PartyAPI interface {
	Party(ctx context.Context, campaignID string, a Actor) (Party, error)
}

// Party is the claimed player characters right now.
type Party struct {
	Heroes []PartyHero
	Night  *PartyNight
}

// PartyHero is one claimed character with the meters its system declares.
type PartyHero struct {
	ID, Name, Player, Subtitle string
	Meters                     []PartyMeter
	Conditions                 []string
}

// PartyMeter is one number on a hero; Max is "" when it has none.
type PartyMeter struct{ Label, Current, Max string }

// PartyNight is the next game night and each player's answer, keyed by the
// player's name ("yes", "maybe", "no", or "" for no answer yet).
type PartyNight struct {
	Name, When string
	Answers    map[string]string
}

// Lookups answers `kind: lookup` blocks. Every answer is read as the Actor
// sees it, so the caller picks the privacy: Safe passes a player-role Actor
// (as the export does), which drops hidden pages, events, pins, tables and
// Director-only entries. Notes are only ever the Actor's own. Any service
// may be nil; its lookups then say so.
type Lookups struct {
	Cal     CalendarAPI
	Weather WeatherSettingsAPI
	Tables  RollTablesAPI
	Maps    MapsAPI
	Pages   PagesAPI
	Rels    RelationsAPI
	Notes   NotesAPI
	Rules   *HouseRuleKind
	Entries SystemEntriesAPI
	Party   PartyAPI
}

// LookupAnswer is one lookup's result: the chip shown on screen and the
// markdown handed back to the AI. Error replaces Text when it failed.
type LookupAnswer struct {
	Chip  string
	Text  string
	Error string
}

// lookupWhats is every `what:` a lookup may ask, in the prompt's order, with
// its keys for the prompt's description.
var lookupWhats = []struct{ what, keys, brief string }{
	{"pages", "optional `type` (e.g. Character)", "page names, by type"},
	{"page", "`name`", "one page: its text, tags and links"},
	{lookupWhatCalendar, "none", "the calendar itself: months, weekdays, seasons, moons with today's phase and the next new and full moon, eras, festivals, today's date, and its climate"},
	{"weather-kinds", "none", "every climate, built-in weather label and sky effect the calendar can animate, and the owner's own kinds of weather"},
	{"events", "`from` and `to` (e.g. `Deepwinter 1 1492`), or `year` and optional `month`", "calendar events in a date range"},
	{"weather", "`from` and `to`, or `year` and `month`", "each day's weather in a date range"},
	{"table", "optional `name`", "rolling tables, or one table's entries"},
	{"pins", "optional `map`", "maps, or one map's pins"},
	{"stock", "`shop`", "what a shop sells"},
	{"inventory", "`character`", "what a character carries"},
	{"notes", "none", "the titles of my own notes"},
	{"house-rules", "none", "the house-rules chapters"},
	{"system-entries", "`field` (e.g. ancestry, kit, race)", "the game system's entries for a character field"},
	{"players", "none", "each player's character right now: level and class, meters, conditions, what they carry, and whether they are coming to the next game night"},
}

// LookupDoc is the prompt's description of lookup blocks.
func LookupDoc() string {
	var b strings.Builder
	for _, w := range lookupWhats {
		fmt.Fprintf(&b, "- `what: %s`: %s. Keys: %s.\n", w.what, w.brief, w.keys)
	}
	return b.String()
}

// lookupRun carries one paste's answers, caching the page walk they share.
type lookupRun struct {
	l          *Lookups
	ctx        context.Context
	campaignID string
	a          Actor
	pages      []entities.Entity
	pagesErr   error
	pagesRead  bool
	// pagesCut is true when the walk stopped at maxPagesWalk with pages
	// left; answers that read pages then say so.
	pagesCut bool
}

// Answer answers each lookup in order. A failing lookup gets an Error and
// the rest still run; answers past the total cap are refused, not cut.
func (l *Lookups) Answer(ctx context.Context, campaignID string, a Actor, recs []Record) []LookupAnswer {
	run := &lookupRun{l: l, ctx: ctx, campaignID: campaignID, a: a}
	out := make([]LookupAnswer, 0, len(recs))
	total := 0
	for i, r := range recs {
		what := normKind(r.Str("what"))
		if i >= maxLookups {
			out = append(out, LookupAnswer{Chip: oneLine(what, 80), Error: fmt.Sprintf("only %d lookups are answered at once; ask again for the rest", maxLookups)})
			continue
		}
		ans := run.one(what, r)
		ans.Chip = oneLine(ans.Chip, 80)
		ans.Error = oneLine(ans.Error, 300)
		if ans.Error == "" && run.pagesCut && ans.Text != "" {
			ans.Text += fmt.Sprintf("\n(Only the first %d pages were read; this answer may miss some.)\n", maxPagesWalk*100)
		}
		if ans.Error == "" {
			ans.Text = capText(ans.Text, maxAnswerBytes)
			if total+len(ans.Text) > maxAnswersBytes {
				ans = LookupAnswer{Chip: ans.Chip, Error: "the answers are already long; ask for this one on its own"}
			}
			total += len(ans.Text)
		}
		out = append(out, ans)
	}
	return out
}

// Markdown joins the answers into the text the operator copies back.
func Markdown(ans []LookupAnswer) string {
	var b strings.Builder
	for _, a := range ans {
		if a.Error != "" {
			fmt.Fprintf(&b, "## %s\n\nNot answered: %s.\n\n", a.Chip, a.Error)
			continue
		}
		b.WriteString(strings.TrimSpace(a.Text))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func (run *lookupRun) one(what string, r Record) LookupAnswer {
	var ans LookupAnswer
	var err error
	switch what {
	case "pages":
		ans, err = run.pagesIndex(r.Str("type"))
	case "page":
		ans, err = run.page(firstNonEmpty(r.Name, r.Str("page")))
	case lookupWhatCalendar:
		ans, err = run.calendarInfo()
	case "weather-kinds", "weather-kind":
		ans, err = run.weatherKinds()
	case "events":
		ans, err = run.events(r)
	case "weather":
		ans, err = run.weather(r)
	case "table", "tables":
		ans, err = run.table(r.Name)
	case "pins", "map", "maps":
		ans, err = run.pins(firstNonEmpty(r.Str("map"), r.Name))
	case "stock", "shop":
		ans, err = run.links(firstNonEmpty(r.Str("shop"), r.Name), "sells", "Stock")
	case "inventory":
		ans, err = run.links(firstNonEmpty(r.Str("character"), r.Name), "Has Item", "Inventory")
	case "notes":
		ans, err = run.notes()
	case "house-rules", "house-rule":
		ans, err = run.houseRules()
	case "system-entries", "entries":
		ans, err = run.systemEntries(firstNonEmpty(r.Str("field"), r.Str("category")))
	case "players", "party":
		ans, err = run.players()
	case "":
		return LookupAnswer{Chip: "Lookup", Error: "it needs `what:`"}
	default:
		return LookupAnswer{Chip: what, Error: fmt.Sprintf("`what: %s` is not something Chronicle can look up", what)}
	}
	if err != nil {
		if ans.Chip == "" {
			ans.Chip = what
		}
		ans.Error = planError(err)
		ans.Text = ""
	}
	return ans
}

// director reports whether answers may include what players can't see.
func (run *lookupRun) director() bool { return run.a.CanAuthorDmOnly() }

// visiblePages walks every page the Actor can see, once per paste.
func (run *lookupRun) visiblePages() ([]entities.Entity, error) {
	if run.pagesRead {
		return run.pages, run.pagesErr
	}
	run.pagesRead = true
	if run.l.Pages == nil {
		run.pagesErr = badRequestf("pages can't be looked up on this server")
		return nil, run.pagesErr
	}
	seen := map[string]bool{}
	for p := 1; p <= maxPagesWalk; p++ {
		ents, _, err := run.l.Pages.List(run.ctx, run.campaignID, 0, run.a.Role, run.a.UserID, entities.ListOptions{Page: p, PerPage: 100})
		if err != nil {
			run.pagesErr = err
			return nil, err
		}
		added := 0
		for _, e := range ents {
			if seen[e.ID] || e.CampaignID != run.campaignID {
				continue
			}
			seen[e.ID] = true
			run.pages = append(run.pages, e)
			added++
		}
		if len(ents) < 100 || added == 0 {
			break
		}
		if p == maxPagesWalk {
			run.pagesCut = true
		}
	}
	return run.pages, nil
}

// findPage is the visible page called name, or a refusal that does not say
// whether a hidden one exists.
func (run *lookupRun) findPage(name string) (*entities.Entity, error) {
	if strings.TrimSpace(name) == "" {
		return nil, badRequestf("it needs the page's name")
	}
	pages, err := run.visiblePages()
	if err != nil {
		return nil, err
	}
	slug := entities.Slugify(name)
	for i := range pages {
		if sameName(pages[i].Name, name) || pages[i].Slug == slug {
			return &pages[i], nil
		}
	}
	return nil, badRequestf("no page called %s", quote(name))
}

// parentNames maps every visible page id to its name, so a parent is only
// ever named when the Actor can see it.
func (run *lookupRun) parentNames() map[string]string {
	pages, _ := run.visiblePages()
	names := make(map[string]string, len(pages))
	for _, p := range pages {
		names[p.ID] = p.Name
	}
	return names
}

// withParent labels a page "Name (in Parent)" when its parent is visible.
func withParent(p entities.Entity, names map[string]string) string {
	if p.ParentID != nil {
		if parent := names[*p.ParentID]; parent != "" {
			return p.Name + " (in " + parent + ")"
		}
	}
	return p.Name
}

// visibleIDs is the set of page ids the Actor can see.
func (run *lookupRun) visibleIDs() map[string]bool {
	pages, _ := run.visiblePages()
	ids := make(map[string]bool, len(pages))
	for _, p := range pages {
		ids[p.ID] = true
	}
	return ids
}

// relationsOf lists a page's own links the Actor may see: Director-only
// links and links to hidden pages are dropped unless the Actor is a Director.
func (run *lookupRun) relationsOf(e *entities.Entity) ([]relations.Relation, error) {
	if run.l.Rels == nil {
		return nil, nil
	}
	rels, err := run.l.Rels.ListByEntity(run.ctx, run.campaignID, e.ID)
	if err != nil {
		return nil, err
	}
	ids := run.visibleIDs()
	out := rels[:0:0]
	for _, r := range rels {
		if r.SourceEntityID != e.ID || !ids[r.TargetEntityID] {
			continue
		}
		if r.DmOnly && !run.director() {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// ---- pages ----

func (run *lookupRun) pagesIndex(typeName string) (LookupAnswer, error) {
	pages, err := run.visiblePages()
	if err != nil {
		return LookupAnswer{Chip: "Pages"}, err
	}
	groups, order := map[string][]string{}, []string{}
	parents := run.parentNames()
	for _, p := range pages {
		label := firstNonEmpty(p.TypeNamePlural, p.TypeName, "Pages")
		if typeName != "" && !sameName(p.TypeName, typeName) && !sameName(p.TypeNamePlural, typeName) && !sameName(p.TypeSlug, typeName) {
			continue
		}
		if _, ok := groups[label]; !ok {
			order = append(order, label)
		}
		groups[label] = append(groups[label], withParent(p, parents))
	}
	n := 0
	for _, g := range groups {
		n += len(g)
	}
	chip := "Pages · " + strconv.Itoa(n)
	if typeName != "" {
		chip = typeName + " · " + strconv.Itoa(n)
		if n == 0 {
			return LookupAnswer{Chip: chip}, badRequestf("no pages of type %s", quote(typeName))
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Pages (%d)\n\n", n)
	lines := 0
	for _, label := range order {
		fmt.Fprintf(&b, "### %s (%d)\n\n", label, len(groups[label]))
		for _, name := range groups[label] {
			if lines >= maxAnswerLines {
				break
			}
			b.WriteString("- " + name + "\n")
			lines++
		}
		b.WriteString("\n")
	}
	if lines < n {
		fmt.Fprintf(&b, "…and %d more; ask with `type:` for one type.\n", n-lines)
	}
	return LookupAnswer{Chip: chip, Text: b.String()}, nil
}

// PageIndex is the prompt's compact list of page names per type, capped,
// so the AI knows what it can ask about without the whole world. A
// sub-page reads "Name (in Parent)" so the AI can place new pages with
// `parent:`; a parent the reader cannot see is not named.
func (l *Lookups) PageIndex(ctx context.Context, campaignID string, a Actor) string {
	if l == nil || l.Pages == nil {
		return ""
	}
	run := &lookupRun{l: l, ctx: ctx, campaignID: campaignID, a: a}
	pages, err := run.visiblePages()
	if err != nil || len(pages) == 0 {
		return ""
	}
	const perType = 40
	groups, order := map[string][]string{}, []string{}
	parents := run.parentNames()
	for _, p := range pages {
		label := firstNonEmpty(p.TypeNamePlural, p.TypeName, "Pages")
		if _, ok := groups[label]; !ok {
			order = append(order, label)
		}
		groups[label] = append(groups[label], withParent(p, parents))
	}
	var b strings.Builder
	for _, label := range order {
		names := groups[label]
		line := names
		if len(line) > perType {
			line = line[:perType]
		}
		fmt.Fprintf(&b, "- **%s** (%d): %s", label, len(names), strings.Join(line, ", "))
		if len(names) > perType {
			fmt.Fprintf(&b, ", and %d more", len(names)-perType)
		}
		b.WriteString("\n")
	}
	return capText(b.String(), 8*1024)
}

func (run *lookupRun) page(name string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Page · " + name}
	e, err := run.findPage(name)
	if err != nil {
		return ans, err
	}
	rels, err := run.relationsOf(e)
	if err != nil {
		return ans, err
	}
	var types []entities.EntityType
	if run.l.Pages != nil {
		types, _ = run.l.Pages.GetEntityTypes(run.ctx, run.campaignID)
	}
	opts := aiexport.Options{Privacy: aiexport.PrivacyModeSafe}
	if run.director() {
		opts.Privacy = aiexport.PrivacyModePermitted
	}
	opts.ParentNames = run.parentNames()
	md, err := aiexport.RenderEntities(run.ctx, []entities.Entity{*e}, types, nil, map[string][]relations.Relation{e.ID: rels}, opts)
	if err != nil {
		return ans, err
	}
	// RenderEntities opens with the export's own headings; one page needs none.
	md = strings.TrimPrefix(md, "# Entities\n\n")
	if i := strings.Index(md, "### "); i > 0 {
		md = md[i:]
	}
	md = strings.Replace(md, "### ", "## Page: ", 1)
	md = strings.Replace(md, " {#"+anchorOf(md)+"}", "", 1)
	md = strings.TrimSuffix(strings.TrimSpace(md), "---")
	ans.Chip = "Page · " + e.Name
	ans.Text = md
	return ans, nil
}

// anchorOf is the {#slug} anchor on the rendered page's heading, if any.
func anchorOf(md string) string {
	line, _, _ := strings.Cut(md, "\n")
	i := strings.LastIndex(line, " {#")
	if i < 0 || !strings.HasSuffix(line, "}") {
		return ""
	}
	return line[i+3 : len(line)-1]
}

// ---- shop stock and inventories ----

func (run *lookupRun) links(owner, relType, label string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: label + " · " + owner}
	e, err := run.findPage(owner)
	if err != nil {
		return ans, err
	}
	rels, err := run.relationsOf(e)
	if err != nil {
		return ans, err
	}
	var lines []string
	for _, r := range rels {
		if r.RelationType != relType {
			continue
		}
		lines = append(lines, "- "+r.TargetEntityName+linkMeta(r.Metadata, relType == "sells"))
	}
	ans.Chip = fmt.Sprintf("%s · %s · %d", label, e.Name, len(lines))
	var b strings.Builder
	if relType == "sells" {
		fmt.Fprintf(&b, "## What %s sells (%d)\n\n", e.Name, len(lines))
	} else {
		fmt.Fprintf(&b, "## What %s carries (%d)\n\n", e.Name, len(lines))
	}
	if len(lines) == 0 {
		b.WriteString("Nothing yet.\n")
	}
	writeLines(&b, lines)
	return LookupAnswer{Chip: ans.Chip, Text: b.String()}, nil
}

// linkMeta renders a stock or inventory link's numbers: ": 1 gp, 5 in
// stock" or " x2 (notes)".
func linkMeta(raw []byte, shop bool) string {
	m := decodeMeta(raw)
	num := func(k string) (string, bool) {
		v, ok := m[k]
		if !ok || v == nil {
			return "", false
		}
		if f, ok := v.(float64); ok {
			return strconv.FormatFloat(f, 'f', -1, 64), true
		}
		return fmt.Sprint(v), true
	}
	if shop {
		var parts []string
		if p, ok := num("price"); ok {
			cur := "gp"
			if c, ok := m["currency"].(string); ok && c != "" {
				cur = c
			}
			parts = append(parts, p+" "+cur)
		}
		if q, ok := num("quantity"); ok {
			parts = append(parts, q+" in stock")
		} else {
			parts = append(parts, "unlimited")
		}
		return ": " + strings.Join(parts, ", ")
	}
	s := ""
	if q, ok := num("quantity"); ok && q != "1" {
		s += " x" + q
	}
	if n, ok := m["notes"].(string); ok && strings.TrimSpace(n) != "" {
		s += " (" + strings.TrimSpace(n) + ")"
	}
	return s
}

// ---- notes, house rules, rolling tables, pins ----

func (run *lookupRun) notes() (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "My notes"}
	if run.l.Notes == nil {
		return ans, badRequestf("notes can't be looked up on this server")
	}
	own, err := NoteKind{Svc: run.l.Notes}.ownNotes(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	names := map[string]string{}
	if pages, err := run.visiblePages(); err == nil {
		for _, p := range pages {
			names[p.ID] = p.Name
		}
	}
	var lines []string
	for _, n := range own {
		where := "Journal"
		if n.EntityID != nil {
			where = "a jot on a page"
			if name, ok := names[*n.EntityID]; ok {
				where = "a jot on " + name
			}
		}
		lines = append(lines, fmt.Sprintf("- %s (%s)", firstNonEmpty(n.Title, "Untitled"), where))
	}
	ans.Chip = fmt.Sprintf("My notes · %d", len(lines))
	var b strings.Builder
	fmt.Fprintf(&b, "## My notes (%d)\n\n", len(lines))
	writeLines(&b, lines)
	ans.Text = b.String()
	return ans, nil
}

func (run *lookupRun) houseRules() (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "House rules"}
	if run.l.Rules == nil {
		return ans, badRequestf("the rulebook can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("house rules are left out in Safe privacy; choose Permitted to include them")
	}
	s, err := run.l.Rules.Export(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	n := strings.Count(s, "\n")
	ans.Chip = fmt.Sprintf("House rules · %d", n)
	ans.Text = fmt.Sprintf("## House-rules chapters (%d)\n\n%s", n, firstNonEmpty(s, "None yet.\n"))
	return ans, nil
}

func (run *lookupRun) table(name string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Rolling tables"}
	if run.l.Tables == nil {
		return ans, badRequestf("rolling tables can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("rolling tables are left out in Safe privacy; choose Permitted to include them")
	}
	doc, err := run.l.Tables.Get(run.ctx, run.campaignID)
	if err != nil {
		return ans, err
	}
	var b strings.Builder
	if name == "" {
		ans.Chip = fmt.Sprintf("Rolling tables · %d", len(doc.Tables))
		fmt.Fprintf(&b, "## Rolling tables (%d)\n\n", len(doc.Tables))
		var lines []string
		for _, t := range doc.Tables {
			lines = append(lines, fmt.Sprintf("- %s (%d entries)", t.Name, len(t.Entries)))
		}
		writeLines(&b, lines)
		ans.Text = b.String()
		return ans, nil
	}
	for _, t := range doc.Tables {
		if !sameName(t.Name, name) {
			continue
		}
		ans.Chip = fmt.Sprintf("Table · %s · %d", t.Name, len(t.Entries))
		fmt.Fprintf(&b, "## Rolling table: %s (%d entries)\n\n", t.Name, len(t.Entries))
		var lines []string
		for _, e := range t.Entries {
			line := "- " + e.Name
			if e.Brief != "" {
				line += ": " + e.Brief
			}
			lines = append(lines, line)
		}
		writeLines(&b, lines)
		ans.Text = b.String()
		return ans, nil
	}
	ans.Chip = "Table · " + name
	return ans, badRequestf("no rolling table called %s", quote(name))
}

func (run *lookupRun) pins(mapName string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Maps"}
	if run.l.Maps == nil {
		return ans, badRequestf("maps can't be looked up on this server")
	}
	ms, err := run.l.Maps.ListMaps(run.ctx, run.campaignID)
	if err != nil {
		return ans, err
	}
	var b strings.Builder
	if mapName == "" {
		ans.Chip = fmt.Sprintf("Maps · %d", len(ms))
		fmt.Fprintf(&b, "## Maps (%d)\n\n", len(ms))
		var lines []string
		for _, m := range ms {
			if m.CampaignID == run.campaignID {
				lines = append(lines, "- "+m.Name)
			}
		}
		writeLines(&b, lines)
		ans.Text = b.String()
		return ans, nil
	}
	for _, m := range ms {
		if !sameName(m.Name, mapName) || m.CampaignID != run.campaignID {
			continue
		}
		pins, err := run.l.Maps.ListPins(run.ctx, run.campaignID, m.ID, run.a.Role, run.a.UserID)
		if err != nil {
			return ans, err
		}
		ans.Chip = fmt.Sprintf("Pins · %s · %d", m.Name, len(pins))
		fmt.Fprintf(&b, "## Pins on %s (%d)\n\n", m.Name, len(pins))
		var lines []string
		for _, p := range pins {
			lines = append(lines, fmt.Sprintf("- %s (x %.1f, y %.1f)", p.Name, p.X, p.Y))
		}
		writeLines(&b, lines)
		ans.Text = b.String()
		return ans, nil
	}
	ans.Chip = "Pins · " + mapName
	return ans, badRequestf("no map called %s", quote(mapName))
}

// ---- game-system entries and players ----

func (run *lookupRun) systemEntries(field string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Game-system entries"}
	if field == "" {
		return ans, badRequestf("it needs `field:`, a character field such as ancestry, kit or race")
	}
	ans.Chip = strings.ToUpper(field[:1]) + field[1:]
	if run.l.Entries == nil {
		return ans, badRequestf("game-system entries can't be looked up on this server yet")
	}
	system, list, err := run.l.Entries.Entries(run.ctx, run.campaignID, field, run.director())
	if err != nil {
		return ans, err
	}
	ans.Chip = fmt.Sprintf("%s · %d", ans.Chip, len(list))
	var b strings.Builder
	head := ans.Chip[:strings.Index(ans.Chip, " · ")]
	if system != "" {
		head += " (" + system + ")"
	}
	fmt.Fprintf(&b, "## %s, %d entries\n\n", head, len(list))
	var lines []string
	for _, e := range list {
		line := fmt.Sprintf("- %s (%s), %s", e.Name, e.Slug, firstNonEmpty(e.Source, "package"))
		if s := strings.TrimSpace(e.Summary); s != "" {
			line += ": " + oneLine(s, 140)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		b.WriteString("None listed.\n")
	}
	writeLines(&b, lines)
	ans.Text = b.String()
	return ans, nil
}

func (run *lookupRun) players() (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Players"}
	if run.l.Party == nil {
		return ans, badRequestf("the party can't be looked up on this server")
	}
	party, err := run.l.Party.Party(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	ids := run.visibleIDs()
	var lines []string
	for _, h := range party.Heroes {
		if !ids[h.ID] {
			continue // a character hidden from players stays out unless the Actor sees it
		}
		line := "- " + h.Name
		if h.Player != "" {
			line += " (" + h.Player + ")"
		}
		var parts []string
		if h.Subtitle != "" {
			parts = append(parts, h.Subtitle)
		}
		var meters []string
		for _, m := range h.Meters {
			if m.Max != "" {
				meters = append(meters, fmt.Sprintf("%s %s of %s", m.Label, m.Current, m.Max))
			} else {
				meters = append(meters, m.Label+" "+m.Current)
			}
		}
		if len(meters) > 0 {
			parts = append(parts, strings.Join(meters, ", "))
		}
		if len(h.Conditions) > 0 {
			parts = append(parts, "Conditions: "+strings.Join(h.Conditions, ", "))
		}
		if carried := run.carried(h.ID); carried != "" {
			parts = append(parts, "Carries "+carried)
		}
		if party.Night != nil {
			parts = append(parts, nightAnswer(party.Night.Answers[h.Player]))
		}
		if len(parts) > 0 {
			line += ": " + strings.Join(parts, ". ")
		}
		lines = append(lines, line+".")
	}
	ans.Chip = fmt.Sprintf("Players · %d, live", len(lines))
	var b strings.Builder
	fmt.Fprintf(&b, "## Players, right now (%d)\n\n", len(lines))
	if party.Night != nil {
		fmt.Fprintf(&b, "Next game night: %s, %s.\n\n", party.Night.Name, party.Night.When)
	}
	if len(lines) == 0 {
		b.WriteString("No player has claimed a character yet.\n")
	}
	writeLines(&b, lines)
	ans.Text = b.String()
	return ans, nil
}

// carried is a hero's inventory on one line, or "" when it can't be read.
func (run *lookupRun) carried(id string) string {
	pages, _ := run.visiblePages()
	for i := range pages {
		if pages[i].ID != id {
			continue
		}
		rels, err := run.relationsOf(&pages[i])
		if err != nil {
			return ""
		}
		var items []string
		for _, r := range rels {
			if r.RelationType == "Has Item" {
				items = append(items, r.TargetEntityName+linkMeta(r.Metadata, false))
			}
		}
		if len(items) > 12 {
			items = append(items[:12], fmt.Sprintf("and %d more", len(items)-12))
		}
		return strings.Join(items, ", ")
	}
	return ""
}

func nightAnswer(a string) string {
	switch a {
	case "yes":
		return "Coming to the next game night"
	case "maybe":
		return "Maybe coming to the next game night"
	case "no":
		return "Can't come to the next game night"
	}
	return "Not answered for the next game night yet"
}

// ---- helpers ----

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// writeLines writes up to maxAnswerLines lines, then says how many more.
func writeLines(b *strings.Builder, lines []string) {
	for i, l := range lines {
		if i >= maxAnswerLines {
			fmt.Fprintf(b, "…and %d more.\n", len(lines)-i)
			return
		}
		b.WriteString(l + "\n")
	}
}

// capText cuts s at a line boundary under n bytes and says it was cut.
func capText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := strings.LastIndex(s[:n], "\n")
	if cut < 0 {
		cut = runeStart(s, n)
	}
	return s[:cut] + "\n…cut here; ask a narrower lookup for the rest.\n"
}

// oneLine flattens whitespace and shortens s to about n bytes.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := strings.LastIndex(s[:n], " ")
	if cut < n/2 {
		cut = runeStart(s, n)
	}
	return s[:cut] + "…"
}

// runeStart backs i up to the start of a UTF-8 character, so a cut never
// splits one.
func runeStart(s string, i int) int {
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}
