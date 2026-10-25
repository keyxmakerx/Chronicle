package records

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// EncountersAPI is the encounters service as the encounter kind needs it,
// adapted in app wiring so this package does not import the plugin. The
// service keeps its own rules (DM only, pages in the campaign, limits); the
// kind only turns the AI's block into its input.
type EncountersAPI interface {
	ListForPage(ctx context.Context, campaignID, pageID string, a Actor) ([]EncounterRef, error)
	Create(ctx context.Context, campaignID, pageID string, a Actor, in EncounterInput) error
	Update(ctx context.Context, campaignID, id string, a Actor, in EncounterInput, version int) error
	Delete(ctx context.Context, campaignID, id string, a Actor) error
	// All lists every encounter the operator may see, with its page's name,
	// for the export.
	All(ctx context.Context, campaignID string, a Actor) ([]EncounterRef, error)
}

// EncounterRef is a stored encounter.
type EncounterRef struct {
	ID, Name, PageName string
	Ready              bool
	Version            int
	Heroes             []string // hero page names
	HeroIDs            []string
	Creatures          []EncounterCreature
	// Versions are the other ways the fight can play, each in full.
	Versions []EncounterVersion
	// Playing is the version the recorded test results pick now; empty
	// when none does and the base creatures play. Open names the tests
	// that pick a version and have no result yet.
	Playing string
	Open    []string
}

// EncounterCreature is one creature row. EntityID is empty for a creature
// that has no page.
type EncounterCreature struct {
	EntityID, Name string
	Count          int
	Group, Start   string
}

// EncounterInput is a whole encounter to save.
type EncounterInput struct {
	Name      string
	Ready     bool
	HeroIDs   []string
	Creatures []EncounterCreature
	// Versions replaces the fight's versions when SetVersions is true; an
	// update without it keeps the ones the DM planned.
	Versions    []EncounterVersionInput
	SetVersions bool
}

// EncounterKind writes the fights a DM plans on a page, matched by name
// within that page. It never reveals anything: a start state is a plan the
// DM carries out in Foundry.
type EncounterKind struct {
	Svc      EncountersAPI
	Entities EntityLookup
	// Tests finds the test results that pick versions; nil refuses a
	// version that names a test.
	Tests TestsAPI
	// Monsters is what Foundry reported it holds, for the review's warning
	// about a creature Foundry can't place; nil leaves the warning general.
	Monsters FoundryMonstersAPI
}

func (EncounterKind) Name() string  { return "encounter" }
func (EncounterKind) Label() string { return "Encounter" }
func (EncounterKind) Doc() string {
	return "A fight planned on a page, matched by `name` within its `page` (the page's name; an Adventures trial or any page). Keys: `heroes` (names of the pages for the heroes the fight is built for; any page works, so a Character page linked to a player is fine, not only a Hero page), `ready` (true puts it at the top of Foundry's list for game night), `rename_to`. `creatures` is a list, one line per kind of creature: an optional count, the creature's page name, then `group A` to `group H` (its initiative group) and how it starts: `starts seen` (the default), `starts hidden` (placed hidden; only the DM reveals it) or `arrives later` (left off the scene until the DM places it). A creature with no page yet goes in by name. Name a creature as Foundry names it (`what: foundry-monsters` lists them) so Foundry can place it; a creature Foundry has needs no page, and a Creature page matched to one of Foundry's monsters is that monster (the same lookup lists the matches), so it never gets a second stat block. `versions` (at most 6) are other ways the same fight plays, such as one per montage result: each has a `name`, its whole `creatures` list written the same way, and optionally `test` (a test's name; `what: tests` lists them) and `result` (one of its results), so recording that result in Chronicle picks the version and Foundry shows it. On update, a key you leave out keeps what is there; `creatures`, `heroes` and `versions` replace the whole list.\n\n```\n---\nkind: encounter\npage: The Siege of Harrowmere\nname: Ramparts fight\nheroes: [Ash, Bryn, Corvin, Dela]\nready: true\ncreatures:\n  - Goblin War Chief, group A, starts hidden\n  - 2 Hobgoblin Soldier, group B\n  - 4 Skeleton Archer, group C, arrives later\nversions:\n  - name: The walls held\n    test: Hold the walls\n    result: Success\n    creatures:\n      - Goblin War Chief, group A, starts hidden\n      - 2 Hobgoblin Soldier, group B\n---\n```"
}

// Title names the row on the review screen: "Ramparts fight on The Siege of
// Harrowmere".
func (EncounterKind) Title(r Record) string {
	if r.Name == "" || r.Str("page") == "" {
		return ""
	}
	return r.Name + " on " + r.Str("page")
}

var (
	countRe = regexp.MustCompile(`^(\d+)\s*[x×]?\s+(.+)$`)
	groupRe = regexp.MustCompile(`(?i)^group\s+([a-h])$`)
)

// parseCreature reads one creature line: "2 Hobgoblin Soldier, group B,
// starts hidden". A map form ({name, count, group, start}) is read too.
func parseCreature(it any) (EncounterCreature, error) {
	c := EncounterCreature{Count: 1, Group: "A", Start: "seen"}
	var opts []string
	switch t := it.(type) {
	case map[string]any:
		c.Name = strings.TrimSpace(fmt.Sprint(t["name"]))
		if v, ok := t["count"]; ok {
			n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
			if err != nil {
				return c, badRequestf("%s: count %v is not a whole number", c.Name, v)
			}
			c.Count = n
		}
		if v, ok := t["group"]; ok {
			opts = append(opts, "group "+fmt.Sprint(v))
		}
		if v, ok := t["start"]; ok {
			opts = append(opts, "starts "+fmt.Sprint(v))
		}
	case nil:
		return c, badRequestf("a creature line is empty")
	default:
		parts := strings.Split(fmt.Sprint(t), ",")
		c.Name = strings.TrimSpace(parts[0])
		if m := countRe.FindStringSubmatch(c.Name); m != nil {
			c.Count, _ = strconv.Atoi(m[1])
			c.Name = strings.TrimSpace(m[2])
		}
		opts = parts[1:]
	}
	if c.Name == "" || c.Name == "<nil>" {
		return c, badRequestf("a creature line has no name")
	}
	for _, o := range opts {
		o = strings.ToLower(strings.Join(strings.Fields(o), " "))
		switch {
		case o == "":
		case groupRe.MatchString(o):
			c.Group = strings.ToUpper(groupRe.FindStringSubmatch(o)[1])
		case o == "starts seen" || o == "seen" || o == "starts visible":
			c.Start = "seen"
		case o == "starts hidden" || o == "hidden":
			c.Start = "hidden"
		case o == "arrives later" || o == "later" || o == "starts later":
			c.Start = "later"
		default:
			return c, badRequestf("%s: %q is not a group (group A to H) or a start (starts seen, starts hidden, arrives later)", c.Name, o)
		}
	}
	if c.Count < 1 || c.Count > 40 {
		return c, badRequestf("%s: a line holds 1 to 40 creatures", c.Name)
	}
	return c, nil
}

func (k EncounterKind) page(ctx context.Context, campaignID, name string) *entities.Entity {
	if name == "" {
		return nil
	}
	e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID {
		return nil
	}
	return e
}

// encounterDraft is the record read against the campaign: the pages it
// names resolved, and what is not a page yet.
type encounterDraft struct {
	page      *entities.Entity
	existing  *EncounterRef
	heroIDs   []string
	creatures []EncounterCreature
	versions  []EncounterVersionInput
	warnings  []string
	missing   []string // hero pages not found
}

func (k EncounterKind) read(ctx context.Context, campaignID string, a Actor, r Record) (*encounterDraft, error) {
	d := &encounterDraft{page: k.page(ctx, campaignID, r.Str("page"))}
	if d.page != nil {
		list, err := k.Svc.ListForPage(ctx, campaignID, d.page.ID, a)
		if err != nil {
			return nil, err
		}
		for i := range list {
			if sameName(list[i].Name, r.Name) {
				d.existing = &list[i]
				break
			}
		}
	}
	for _, it := range r.List("heroes") {
		name := strings.TrimSpace(fmt.Sprint(it))
		if name == "" {
			continue
		}
		if p := k.page(ctx, campaignID, name); p != nil {
			d.heroIDs = append(d.heroIDs, p.ID)
		} else {
			d.missing = append(d.missing, name)
		}
	}
	for _, it := range r.List("creatures") {
		c, err := parseCreature(it)
		if err != nil {
			return nil, err
		}
		d.creatures = append(d.creatures, k.link(ctx, campaignID, a, r, c, d))
	}
	if r.Has("versions") {
		parsed, err := parseVersions(r.List("versions"))
		if err != nil {
			return nil, err
		}
		pageName := r.Str("page")
		if d.page != nil {
			pageName = d.page.Name
		}
		if d.versions, err = resolveVersions(ctx, k.Tests, campaignID, pageName, a, parsed); err != nil {
			return nil, err
		}
		for i := range d.versions {
			for j, c := range d.versions[i].Creatures {
				d.versions[i].Creatures[j] = k.link(ctx, campaignID, a, r, c, d)
			}
		}
	}
	return d, nil
}

// link points a creature line at its page, or warns, once per name, when
// it has none: Foundry then places it by name, so the warning says whether
// Foundry reported having it.
func (k EncounterKind) link(ctx context.Context, campaignID string, a Actor, r Record, c EncounterCreature, d *encounterDraft) EncounterCreature {
	if p := k.page(ctx, campaignID, c.Name); p != nil {
		c.EntityID, c.Name = p.ID, p.Name
		return c
	}
	if r.Made[entities.Slugify(c.Name)] {
		return c
	}
	w := k.byNameWarning(ctx, campaignID, a, c.Name)
	for _, have := range d.warnings {
		if have == w {
			return c
		}
	}
	d.warnings = append(d.warnings, w)
	return c
}

func (k EncounterKind) byNameWarning(ctx context.Context, campaignID string, a Actor, name string) string {
	base := quote(name) + " is not a page yet; it goes in by name"
	if k.Monsters == nil {
		return base + " with no stat block, and Foundry looks its actor up by name"
	}
	rep, err := k.Monsters.Monsters(ctx, campaignID, a)
	if err != nil || rep.ReportedAt == nil {
		return base + " with no stat block, and Foundry looks its actor up by name"
	}
	if m := rep.Find(name); m != nil {
		return base + ", and Foundry has it (" + m.Where() + ")"
	}
	return base + ", but Foundry did not report a monster by that name, so it can't place it until you add one"
}

// versionsSummary is the review row's note on versions: how many the block
// sets, or that it clears them.
func versionsSummary(vs []EncounterVersionInput, set bool) string {
	switch {
	case !set:
		return ""
	case len(vs) == 0:
		return "; no versions"
	}
	return "; " + plural(len(vs), "version", "versions")
}

func encounterSummary(cs []EncounterCreature, heroes int) string {
	n, hidden, later := 0, 0, 0
	for _, c := range cs {
		n += c.Count
		switch c.Start {
		case "hidden":
			hidden += c.Count
		case "later":
			later += c.Count
		}
	}
	parts := []string{plural(n, "creature", "creatures")}
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d start hidden", hidden))
	}
	if later > 0 {
		parts = append(parts, fmt.Sprintf("%d arrive later", later))
	}
	s := strings.Join(parts, ", ")
	if heroes > 0 {
		s += ", for " + plural(heroes, "hero", "heroes")
	}
	return s
}

func (k EncounterKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can plan encounters"}
	}
	if r.Name == "" {
		return Plan{Error: "an encounter needs a name"}
	}
	if r.Str("page") == "" {
		return Plan{Error: "an encounter needs `page`: the page it is planned on"}
	}
	d, err := k.read(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	p := Plan{Warnings: d.warnings}
	if d.page == nil {
		if r.Action != ActionCreate {
			return Plan{Error: "no page called " + quote(r.Str("page"))}
		}
		// A page neither found nor made by this paste is sure to fail at
		// commit, so the row is refused now rather than starting on Yes.
		if !r.Made[entities.Slugify(r.Str("page"))] {
			return Plan{Error: "no page called " + quote(r.Str("page")) + "; add it to this paste, or name a page that exists"}
		}
	}
	for _, h := range d.missing {
		if r.Action == ActionDelete {
			break
		}
		if !r.Made[entities.Slugify(h)] {
			return Plan{Error: "no hero page called " + quote(h) + "; add it to this paste, or take it out of `heroes`"}
		}
	}
	switch r.Action {
	case ActionCreate:
		if d.existing != nil {
			return Plan{Error: "an encounter called " + quote(r.Name) + " is already on " + d.page.Name + "; use action: update"}
		}
		p.Summary = encounterSummary(d.creatures, len(d.heroIDs)+len(d.missing)) + versionsSummary(d.versions, r.Has("versions"))
	case ActionUpdate:
		if d.existing == nil {
			return Plan{Error: "no encounter called " + quote(r.Name) + " on " + quote(r.Str("page"))}
		}
		cs, heroes := d.existing.Creatures, len(d.existing.HeroIDs)
		if r.Has("creatures") {
			cs = d.creatures
		}
		if r.Has("heroes") {
			heroes = len(d.heroIDs) + len(d.missing)
		}
		p.Summary = encounterSummary(cs, heroes) + versionsSummary(d.versions, r.Has("versions"))
		if s := r.Str("rename_to"); s != "" {
			p.Summary += "; renamed to " + quote(s)
		}
	default:
		if d.existing == nil {
			return Plan{Error: "no encounter called " + quote(r.Name) + " on " + quote(r.Str("page"))}
		}
		p.Summary = "removes the encounter"
		p.Warnings = nil
	}
	return p
}

func (k EncounterKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	d, err := k.read(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if d.page == nil {
		return badRequestf("no page called %s", quote(r.Str("page")))
	}
	if r.Action != ActionDelete && len(d.missing) > 0 {
		return badRequestf("no hero page called %s", quote(d.missing[0]))
	}
	if r.Action != ActionCreate && d.existing == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	switch r.Action {
	case ActionCreate:
		ready, _ := r.Bool("ready")
		return k.Svc.Create(ctx, campaignID, d.page.ID, a, EncounterInput{
			Name: r.Name, Ready: ready, HeroIDs: d.heroIDs, Creatures: d.creatures,
			Versions: d.versions, SetVersions: true,
		})
	case ActionUpdate:
		e := d.existing
		in := EncounterInput{Name: e.Name, Ready: e.Ready, HeroIDs: e.HeroIDs, Creatures: e.Creatures}
		if s := r.Str("rename_to"); s != "" {
			in.Name = s
		}
		if v, ok := r.Bool("ready"); ok {
			in.Ready = v
		}
		if r.Has("heroes") {
			in.HeroIDs = d.heroIDs
		}
		if r.Has("creatures") {
			in.Creatures = d.creatures
		}
		if r.Has("versions") {
			in.Versions, in.SetVersions = d.versions, true
		}
		return k.Svc.Update(ctx, campaignID, e.ID, a, in, e.Version)
	default:
		return k.Svc.Delete(ctx, campaignID, d.existing.ID, a)
	}
}

// Export lists every encounter in this kind's own format, so an AI can
// update or remove one by name.
func (k EncounterKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	list, err := k.Svc.All(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range list {
		writeEncounterBlock(&b, e)
		b.WriteString("\n")
	}
	return b.String(), nil
}
