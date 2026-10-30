package records

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// CraftingAPI is the crafting app as the crafting lookup and the recipe,
// material and drops kinds need it, adapted in app wiring so this package
// does not import the plugin. The service keeps its own rules: Directors
// only for everything but the recipe book, items and characters of this
// campaign, the system's characteristics, the limits.
type CraftingAPI interface {
	// Read is crafting as the Actor may see it: every part for a
	// Director, the words and the recipe book for anyone else.
	Read(ctx context.Context, campaignID string, a Actor) (*Crafting, error)
	// SaveRecipe checks a recipe and, unless check, saves it. A new recipe
	// arrives as a draft players can't see until a Director approves it.
	SaveRecipe(ctx context.Context, campaignID string, a Actor, in CraftRecipeInput, check bool) error
	DeleteRecipe(ctx context.Context, campaignID, id string, a Actor) error
	// SetTags replaces an item's material kinds; none leaves it no
	// material at all.
	SetTags(ctx context.Context, campaignID, itemID string, a Actor, tags []string) error
	// SaveDrops checks a page's drop list and, unless check, saves it.
	SaveDrops(ctx context.Context, campaignID, pageID string, a Actor, in CraftDropsInput, check bool) error
	DeleteDrops(ctx context.Context, campaignID, pageID string, a Actor) error
}

// Crafting is what the crafting lookup prints and the crafting kinds check
// against, with every id the Actor may see named.
type Crafting struct {
	// Off says the campaign has crafting switched off in Game & features.
	Off      bool
	Director bool
	// System names the game system the rules come from; "" for none.
	System string
	Dice   string
	// Stats are the characteristics a roll adds; with none, a recipe's
	// characteristic is free words.
	Stats []CraftChoice
	// Tiers names the harvest test's three outcomes.
	Tiers    []string
	Words    CraftWords
	Tags     []CraftChoice
	Methods  []CraftChoice
	Settings CraftSettings
	// Book is the recipe book as a player reads it, for an Actor who is
	// no Director.
	Book []CraftBookPage

	// The Director's parts.
	Items      []CraftItem
	Characters []CraftNamed
	// ItemsCut says the campaign has more items or characters than were
	// read.
	ItemsCut bool
	Recipes  []CraftRecipe
	Drops    []CraftDrops
	// DropsOff says drop lists and the tray can't be read on this server.
	DropsOff bool
	Tray     []CraftSpoil
	Projects []CraftProject
	Benches  []CraftBench
}

// CraftChoice is one entry of a fixed list: a key and its words.
type CraftChoice struct{ Key, Label string }

// CraftWords are the system's names for a project's goal, prerequisite and
// source.
type CraftWords struct{ Goal, Prerequisite, Source string }

// CraftSettings are the campaign's crafting defaults.
type CraftSettings struct {
	Look, Difficulty string
	Bane, Skip       bool
}

// CraftItem is an Armory item with its material kinds (tag keys).
type CraftItem struct {
	ID, Name string
	Tags     []string
}

// CraftNamed is a page picked by id and shown by name.
type CraftNamed struct{ ID, Name string }

// CraftMaterial is one thing a recipe asks for: an item, or any item of a
// kind (Tag).
type CraftMaterial struct {
	ItemID, Tag string
	Qty         int
}

// CraftRecipe is a stored recipe. Method is the template its steps were
// laid with; ByHand says the steps were changed in the editor since.
type CraftRecipe struct {
	ID, Name string
	Draft    bool
	Method   string
	ByHand   bool
	Steps    int
	// Rules counts the editor's "When… then…" rules, which an AI keeps.
	Rules                      int
	Materials                  []CraftMaterial
	ResultID                   string
	ResultQty                  int
	Goal                       int
	Stat, Prerequisite, Source string
	Cost                       float64
	Currency                   string
	// Who is "all" or "learned"; Known are the characters who learned it.
	Who   string
	Known []string
}

// CraftRecipeInput is a whole recipe to check or save, by ids. Method ""
// keeps a stored recipe's steps.
type CraftRecipeInput struct {
	ID, Name, Method           string
	Materials                  []CraftMaterial
	ResultID                   string
	ResultQty                  int
	Goal                       int
	Stat, Prerequisite, Source string
	Cost                       float64
	Currency                   string
	Who                        string
	Known                      []string
}

// CraftDropLine is one line of a drop list: taken as it is (Qty), or
// harvested with a test adding Stat that pays Yields by tier. Study marks a
// find players see as "Something unidentified" until a study adding
// StudyStat lands; Once loses it on the first miss.
type CraftDropLine struct {
	ItemID    string
	Harvest   bool
	Qty       int
	Stat      string
	Yields    [3]int
	Study     bool
	StudyStat string
	Once      bool
}

// CraftDrops is a page's drop list. AlsoID names a page whose list drops
// first.
type CraftDrops struct {
	PageID, Page, Kind string
	AlsoID, Also       string
	Lines              []CraftDropLine
	Version            int
}

// CraftDropsInput is a whole drop list to check or save; Version 0 makes a
// new one.
type CraftDropsInput struct {
	AlsoID  string
	Lines   []CraftDropLine
	Version int
}

// CraftSpoil is one thing waiting in the spoils tray, each line in words.
type CraftSpoil struct {
	Source       string
	Copy, Copies int
	Origin       string
	Lines        []string
}

// CraftProject is a hero's project.
type CraftProject struct {
	Hero, Name, Makes, Status, Waits string
	Points, Goal, Rolls              int
}

// CraftBench is a bench and the kind of bench it is.
type CraftBench struct{ Name, Kind string }

// CraftBookPage is a recipe as a player reads it in their recipe book.
type CraftBookPage struct {
	Name, Kind, Makes          string
	MakesQty, Goal             int
	Needs                      []string
	Roll, Prerequisite, Source string
	Cost                       string
	Lines                      []string
}

// ---- reading names the AI writes ----

// craftKey folds a word for matching: "Remnant part" and remnant_part are
// the same kind.
func craftKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "_", " "))), " ")
}

// choice finds a fixed-list entry by its key or its words.
func choice(list []CraftChoice, s string) (CraftChoice, bool) {
	k := craftKey(s)
	for _, c := range list {
		if craftKey(c.Key) == k || craftKey(c.Label) == k {
			return c, true
		}
	}
	return CraftChoice{}, false
}

func choiceWords(list []CraftChoice) string {
	parts := make([]string, len(list))
	for i, c := range list {
		parts[i] = c.Label
	}
	return strings.Join(parts, ", ")
}

func choiceLabel(list []CraftChoice, key string) string {
	for _, c := range list {
		if c.Key == key {
			return c.Label
		}
	}
	return key
}

// errPending marks an item the paste makes, which can only be checked once
// its page is saved.
type errPending struct{ name string }

func (e errPending) Error() string { return e.name }

// pendingPrefix marks a stand-in id for an item the paste makes, so the
// review can name it; Apply refuses a record that still holds one.
const pendingPrefix = "pending:"

func pendingID(name string) string { return pendingPrefix + name }

// item finds the Armory item called name. An item this paste makes is
// pending until it saves; a name two items share is refused, since the
// AI can't say which it means.
func (c *Crafting) item(name string, made map[string]bool) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", badRequestf("an item's name is missing")
	}
	id := ""
	for _, it := range c.Items {
		if sameName(it.Name, name) {
			if id != "" {
				return "", badRequestf("two Armory items are called %s; rename one so the AI can name it", quote(name))
			}
			id = it.ID
		}
	}
	if id != "" {
		return id, nil
	}
	if made[entities.Slugify(name)] {
		return "", errPending{name}
	}
	if c.ItemsCut {
		return "", badRequestf("no Armory item called %s among the ones Chronicle read (the campaign has more)", quote(name))
	}
	return "", badRequestf("no Armory item called %s (an item is a page in one of the Armory's item categories)", quote(name))
}

func (c *Crafting) itemName(id string) string {
	if strings.HasPrefix(id, pendingPrefix) {
		return id[len(pendingPrefix):]
	}
	for _, it := range c.Items {
		if it.ID == id {
			return it.Name
		}
	}
	return ""
}

// itemWords names an item for a summary, saying so when it isn't among
// those read.
func (c *Crafting) itemWords(id string) string {
	if n := c.itemName(id); n != "" {
		return n
	}
	if c.ItemsCut {
		return "an item Chronicle didn't read"
	}
	return "an item no longer in the Armory"
}

func (c *Crafting) itemTags(id string) []string {
	for _, it := range c.Items {
		if it.ID == id {
			return it.Tags
		}
	}
	return nil
}

func (c *Crafting) character(name string) (string, error) {
	for _, ch := range c.Characters {
		if sameName(ch.Name, name) {
			return ch.ID, nil
		}
	}
	return "", badRequestf("no character called %s", quote(name))
}

func (c *Crafting) characterName(id string) string {
	for _, ch := range c.Characters {
		if ch.ID == id {
			return ch.Name
		}
	}
	return ""
}

// stat reads a characteristic by key or words; with none listed it is free
// words, which the service checks.
func (c *Crafting) stat(key, s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || isNone(s) {
		return "", nil
	}
	if len(c.Stats) == 0 {
		return s, nil
	}
	if ch, ok := choice(c.Stats, s); ok {
		return ch.Key, nil
	}
	return "", badRequestf("%s: %q is not one of the system's characteristics (%s)", key, s, choiceWords(c.Stats))
}

func (c *Crafting) statLabel(key string) string { return choiceLabel(c.Stats, key) }

func (c *Crafting) tagLabel(key string) string { return choiceLabel(c.Tags, key) }

func (c *Crafting) tagLabels(keys []string) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = c.tagLabel(k)
	}
	return strings.Join(parts, ", ")
}

func (c *Crafting) recipe(name string) *CraftRecipe {
	for i := range c.Recipes {
		if sameName(c.Recipes[i].Name, name) {
			return &c.Recipes[i]
		}
	}
	return nil
}

func (c *Crafting) dropsOf(pageID string) *CraftDrops {
	for i := range c.Drops {
		if c.Drops[i].PageID == pageID {
			return &c.Drops[i]
		}
	}
	return nil
}

// droppedBy names the pages whose drop lists hold an item, for a
// material's "where it comes from".
func (c *Crafting) droppedBy(itemID string) []string {
	var out []string
	for _, d := range c.Drops {
		for _, l := range d.Lines {
			if l.ItemID == itemID {
				out = append(out, d.Page)
				break
			}
		}
	}
	return out
}

// usedBy names the recipes that ask for an item by name or for any item
// of one of its kinds.
func (c *Crafting) usedBy(itemID string, tags []string) []string {
	var out []string
	for _, r := range c.Recipes {
		for _, m := range r.Materials {
			if (m.ItemID != "" && m.ItemID == itemID) || (m.Tag != "" && containsStr(tags, m.Tag)) {
				out = append(out, r.Name)
				break
			}
		}
	}
	return out
}

var (
	craftCountRe = regexp.MustCompile(`^(\d+)\s*[x×]?\s+(.+)$`)
	harvestRe    = regexp.MustCompile(`(?i)^harvest\s+(.+?)(?:\s+with\s+(.+?))?\s*:\s*(\d+)\s*/\s*(\d+)\s*/\s*(\d+)$`)
	studyRe      = regexp.MustCompile(`(?i)^study(?:\s+with\s+(.+?))?(?:\s*,\s*(one chance))?$`)
)

// countName splits "2 Troll tooth" into 2 and the name; no number is 1.
func countName(s string) (int, string) {
	s = strings.TrimSpace(s)
	if m := craftCountRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, strings.TrimSpace(m[2])
	}
	return 1, s
}

// anyKind reads "any herb" as the kind it names; ok is false for an item's
// name.
func (c *Crafting) anyKind(s string) (string, bool, error) {
	low := strings.ToLower(strings.TrimSpace(s))
	if !strings.HasPrefix(low, "any ") {
		return "", false, nil
	}
	word := strings.TrimSpace(s[4:])
	if ch, ok := choice(c.Tags, word); ok {
		return ch.Key, true, nil
	}
	return "", true, badRequestf("%q is not a kind of material; the kinds are %s", word, choiceWords(c.Tags))
}

// material reads one material line: "2 Troll tooth", "1 any herb", or
// {item, qty} / {any, qty}.
func (c *Crafting) material(raw any, made map[string]bool) (CraftMaterial, error) {
	var m CraftMaterial
	var text string
	switch t := raw.(type) {
	case map[string]any:
		q := 1
		if v, ok := t["qty"]; ok {
			n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
			if err != nil {
				return m, badRequestf("materials: qty %v is not a whole number", v)
			}
			q = n
		}
		if k, ok := t["any"]; ok {
			text = fmt.Sprintf("%d any %v", q, k)
		} else {
			text = fmt.Sprintf("%d %v", q, t["item"])
		}
	case nil:
		return m, badRequestf("materials: a line is empty")
	default:
		text = fmt.Sprint(t)
	}
	q, name := countName(text)
	m.Qty = q
	if q < 1 || q > 99 {
		return m, badRequestf("materials: %s asks for %d; write 1 to 99", quote(name), q)
	}
	tag, isAny, err := c.anyKind(name)
	if err != nil {
		return m, err
	}
	if isAny {
		m.Tag = tag
		return m, nil
	}
	m.ItemID, err = c.item(name, made)
	return m, err
}

// dropLine reads one drop line: "2 Troll tooth" is taken as it is,
// "harvest Troll hide with Might: 1/2/4" is a test paying each tier.
func (c *Crafting) dropLine(raw any, made map[string]bool) (CraftDropLine, error) {
	var l CraftDropLine
	if t, ok := raw.(map[string]any); ok {
		// YAML reads an unquoted `harvest Troll hide with Might: 1/2/4` as a
		// one-key map, so put the line back together.
		if len(t) != 1 {
			return c.dropLineMap(t, made)
		}
		for k, v := range t {
			if !strings.HasPrefix(strings.ToLower(k), "harvest ") {
				return c.dropLineMap(t, made)
			}
			raw = k + ": " + fmt.Sprint(v)
		}
	}
	if raw == nil {
		return l, badRequestf("lines: a line is empty")
	}
	text, mark, marked := strings.Cut(strings.TrimSpace(fmt.Sprint(raw)), ";")
	text = strings.TrimSpace(text)
	if marked {
		if err := c.studyMark(&l, mark); err != nil {
			return l, err
		}
	}
	if m := harvestRe.FindStringSubmatch(text); m != nil {
		l.Harvest = true
		for i := 0; i < 3; i++ {
			l.Yields[i], _ = strconv.Atoi(m[3+i])
		}
		var err error
		if l.Stat, err = c.stat("lines", m[2]); err != nil {
			return l, err
		}
		l.ItemID, err = c.item(m[1], made)
		return l, err
	}
	if strings.HasPrefix(strings.ToLower(text), "harvest ") {
		return l, badRequestf("lines: write a harvest as `harvest Troll hide with Might: 1/2/4`, what the three outcomes pay (%s)", strings.Join(c.Tiers, ", "))
	}
	q, name := countName(text)
	if q < 1 || q > 99 {
		return l, badRequestf("lines: %s drops %d; write 1 to 99", quote(name), q)
	}
	l.Qty = q
	var err error
	l.ItemID, err = c.item(name, made)
	return l, err
}

// studyMark reads what follows a line's `;`: "study with Reason", and
// ", one chance" when the first miss loses the find.
func (c *Crafting) studyMark(l *CraftDropLine, s string) error {
	m := studyRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return badRequestf("lines: after `;` write `study with Reason`, and `, one chance` if the first failed study loses it")
	}
	var err error
	l.Study, l.Once = true, m[2] != ""
	l.StudyStat, err = c.stat("lines", m[1])
	return err
}

// dropLineMap reads {item, qty} or {item, harvest, pays: [1, 2, 4]}, where
// harvest names the characteristic (or none); `study` (a characteristic,
// or yes) and `once` mark a find to study first.
func (c *Crafting) dropLineMap(t map[string]any, made map[string]bool) (CraftDropLine, error) {
	var l CraftDropLine
	name := strings.TrimSpace(fmt.Sprint(t["item"]))
	if t["item"] == nil || name == "" {
		return l, badRequestf("lines: a line needs `item`")
	}
	if st, ok := t["study"]; ok && st != nil && st != false {
		mark := "study"
		if s, isStr := st.(string); isStr && !isNone(s) && !strings.EqualFold(s, "yes") {
			mark += " with " + s
		} else if isStr && isNone(s) {
			mark = ""
		}
		if once, _ := t["once"].(bool); once {
			mark += ", one chance"
		}
		if mark != "" {
			if err := c.studyMark(&l, mark); err != nil {
				return l, err
			}
		}
	}
	if h, ok := t["harvest"]; ok {
		l.Harvest = true
		pays, _ := t["pays"].([]any)
		if len(pays) != 3 {
			return l, badRequestf("lines: %s's harvest needs `pays`, three amounts: %s", quote(name), strings.Join(c.Tiers, ", "))
		}
		for i, p := range pays {
			n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(p)))
			if err != nil {
				return l, badRequestf("lines: %s pays %v, not a whole number", quote(name), p)
			}
			l.Yields[i] = n
		}
		var err error
		if s, isStr := h.(string); isStr {
			if l.Stat, err = c.stat("lines", s); err != nil {
				return l, err
			}
		}
	} else {
		l.Qty = 1
		if v, ok := t["qty"]; ok {
			n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
			if err != nil || n < 1 || n > 99 {
				return l, badRequestf("lines: %s's qty is 1 to 99", quote(name))
			}
			l.Qty = n
		}
	}
	var err error
	l.ItemID, err = c.item(name, made)
	return l, err
}

// cost reads "25 gp", 25 or none.
func readCost(s string) (float64, string, error) {
	s = strings.TrimSpace(s)
	if s == "" || isNone(s) || s == "0" {
		return 0, "", nil
	}
	f := strings.Fields(s)
	n, err := strconv.ParseFloat(strings.TrimSuffix(f[0], ","), 64)
	if err != nil || n < 0 {
		return 0, "", badRequestf("cost: write it as a number and its coin, like `25 gp`")
	}
	return n, strings.Join(f[1:], " "), nil
}

// money writes a cost as the block reads it.
func money(n float64, cur string) string {
	s := strconv.FormatFloat(n, 'f', -1, 64)
	if cur != "" {
		s += " " + cur
	}
	return s
}
