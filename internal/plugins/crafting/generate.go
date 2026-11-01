package crafting

import (
	"context"
	"hash/fnv"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// The recipe generator is Chronicle's own, like its name and weather
// generators: no AI, and the same seed and rerolls give the same list. It
// builds recipes from the campaign's own materials and the bench's own
// methods. A goal comes only from the game system's published goals, or
// from the Director's own recipes; with neither, the row says the goal is
// the Director's to set.

// MakeKind is a kind of thing the generator writes recipes for.
type MakeKind struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// For matches a published goal's "for" ("consumable", "weapon").
	For string `json:"for"`
	// Templates are the methods its recipes use; "" means no bench steps,
	// so heroes make the project roll alone.
	Templates []string `json:"-"`
	Nouns     []string `json:"-"`
	// Kinds are the material kinds it draws from when the Director picks
	// none.
	Kinds []string `json:"kinds"`
}

// MakeKinds are what the generator can write. Weapons and gear have no
// bench steps yet: the only bench Chronicle draws is the apothecary's.
var MakeKinds = []MakeKind{
	{Key: "potions", Label: "Potions", For: "consumable", Templates: []string{"steady", "volatile"},
		Nouns: []string{"Draught", "Tonic", "Elixir", "Tincture", "Philter"}, Kinds: []string{"herb", "liquid", "fungus", "magic_essence"}},
	{Key: "salves", Label: "Salves", For: "consumable", Templates: []string{"steady"},
		Nouns: []string{"Salve", "Balm", "Poultice", "Unguent", "Paste"}, Kinds: []string{"herb", "resin", "fungus", "body_part"}},
	{Key: "weapons", Label: "Weapons", For: "weapon", Templates: []string{""},
		Nouns: []string{"Blade", "Spear", "Axe", "Maul", "Bow"}, Kinds: []string{"metal", "wood", "bone", "hide"}},
	{Key: "gear", Label: "Gear", For: "gear", Templates: []string{""},
		Nouns: []string{"Charm", "Kit", "Lantern", "Cloak", "Satchel"}, Kinds: []string{"cloth", "hide", "wood", "gem"}},
}

func makeKind(k string) *MakeKind {
	for i := range MakeKinds {
		if MakeKinds[i].Key == k {
			return &MakeKinds[i]
		}
	}
	return nil
}

// Generator limits.
const (
	maxGenerate    = 12
	maxSeedLen     = 40
	maxRerolls     = 999
	defaultGenSize = 6
)

// GenerateInput is what to make. Rerolls holds how many times each row was
// rolled again, so one row changes and the rest stay.
type GenerateInput struct {
	Make    string   `json:"make"`
	Kinds   []string `json:"kinds"`
	Count   int      `json:"count"`
	Echelon int      `json:"echelon"`
	Table   string   `json:"table"`
	Seed    string   `json:"seed"`
	Rerolls []int    `json:"rerolls"`
}

// GeneratedMaterial is one line of a generated recipe, with its name.
type GeneratedMaterial struct {
	Material
	Name string `json:"name"`
}

// GeneratedRecipe is one row. Nothing is saved until the Director keeps it.
type GeneratedRecipe struct {
	Name      string              `json:"name"`
	Template  string              `json:"template"`
	Materials []GeneratedMaterial `json:"materials"`
	Steps     []Step              `json:"steps"`
	// Goal is 0 when neither the system nor the Director's recipes give one.
	Goal int `json:"goal"`
	// GoalFrom is "system", "recipes" or "" (set it yourself).
	GoalFrom string `json:"goalFrom"`
	// ResultID is an Armory item already named like the recipe, "" when
	// keeping it makes a new one.
	ResultID string `json:"resultId,omitempty"`
	// Missing are the kinds the row asks "any" of because the campaign has
	// no item of that kind yet.
	Missing []string `json:"missing"`
	// Taken says a recipe of this name already exists.
	Taken bool `json:"taken,omitempty"`
}

// GenerateResult is the list with the seed that made it.
type GenerateResult struct {
	Seed string            `json:"seed"`
	Rows []GeneratedRecipe `json:"rows"`
	// Makes lists what can be made, for the inputs.
	Makes []MakeKind `json:"makes"`
}

// genFacts is what one list is built from.
type genFacts struct {
	items   []MaterialItem
	byKind  map[string][]MaterialItem
	tags    map[string][]string
	names   []string // a rolling table's rows, when one was picked
	taken   map[string]bool
	byName  map[string]string // lower-cased item name to id
	goals   []systems.CraftingGoal
	ownGoal int
}

// Generate writes a list of recipes and saves nothing.
func (s *service) Generate(ctx context.Context, campaignID string, v Viewer, in GenerateInput) (*GenerateResult, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	in, mk, err := cleanGenerate(in)
	if err != nil {
		return nil, err
	}
	cat, err := s.Catalog(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	rules, _ := s.systemRules(ctx, campaignID)
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	f := genFacts{items: cat.Items, byKind: map[string][]MaterialItem{}, tags: map[string][]string{}, taken: map[string]bool{},
		byName: map[string]string{}, goals: rules.Goals, ownGoal: middleGoal(all)}
	for _, it := range cat.Items {
		f.tags[it.ID] = it.Tags
		f.byName[strings.ToLower(it.Name)] = it.ID
		for _, t := range it.Tags {
			f.byKind[t] = append(f.byKind[t], it)
		}
	}
	for _, r := range all {
		f.taken[strings.ToLower(r.Name)] = true
	}
	if in.Table != "" {
		for _, t := range cat.Tables {
			if strings.EqualFold(t.Name, in.Table) {
				for _, row := range t.Rows {
					if row = strings.TrimSpace(row); row != "" && len([]rune(row)) <= maxNameLen && !hasControl(row) {
						f.names = append(f.names, row)
					}
				}
			}
		}
	}
	return &GenerateResult{Seed: in.Seed, Rows: generateRows(in, mk, f), Makes: MakeKinds}, nil
}

// cleanGenerate fills in the defaults and refuses what can't be used.
func cleanGenerate(in GenerateInput) (GenerateInput, *MakeKind, error) {
	if in.Make == "" {
		in.Make = MakeKinds[0].Key
	}
	mk := makeKind(in.Make)
	if mk == nil {
		return in, nil, errInvalid("the generator can make potions, salves, weapons or gear")
	}
	kinds, err := cleanKinds(in.Kinds)
	if err != nil {
		return in, nil, err
	}
	if len(kinds) == 0 {
		kinds = mk.Kinds
	}
	in.Kinds = kinds
	if in.Count == 0 {
		in.Count = defaultGenSize
	}
	// Compared here rather than through clamp so the bound sits beside the
	// allocations below, where code scanning can see it.
	if in.Count < 1 {
		in.Count = 1
	} else if in.Count > maxGenerate {
		in.Count = maxGenerate
	}
	in.Echelon = clamp(in.Echelon, 1, 4)
	in.Seed = strings.TrimSpace(in.Seed)
	if len(in.Seed) > maxSeedLen || hasControl(in.Seed) {
		return in, nil, errInvalid("the seed is a short word or number")
	}
	if in.Seed == "" {
		return in, nil, errInvalid("the generator needs a seed")
	}
	rr := make([]int, in.Count)
	for i := range rr {
		if i < len(in.Rerolls) {
			rr[i] = clamp(in.Rerolls[i], 0, maxRerolls)
		}
	}
	in.Rerolls = rr
	return in, mk, nil
}

// rowRand is the dice for one row: the seed, the row and how often it was
// rolled again, so a reroll leaves the other rows as they were.
func rowRand(seed string, row, reroll int) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(seed))
	h.Write([]byte{0})
	h.Write([]byte(strconv.Itoa(row)))
	h.Write([]byte{0})
	h.Write([]byte(strconv.Itoa(reroll)))
	sum := h.Sum64()
	return rand.New(rand.NewPCG(sum, sum^0x9e3779b97f4a7c15))
}

func generateRows(in GenerateInput, mk *MakeKind, f genFacts) []GeneratedRecipe {
	rows := make([]GeneratedRecipe, 0, in.Count)
	used := map[string]bool{}
	for i := 0; i < in.Count; i++ {
		rows = append(rows, generateRow(rowRand(in.Seed, i, in.Rerolls[i]), in, mk, f, used))
	}
	return rows
}

func generateRow(r *rand.Rand, in GenerateInput, mk *MakeKind, f genFacts, used map[string]bool) GeneratedRecipe {
	row := GeneratedRecipe{Template: mk.Templates[r.IntN(len(mk.Templates))], Missing: []string{}}
	missing := map[string]bool{}
	var mats []Material
	pick := func(kind string, named bool) {
		pool := f.byKind[kind]
		if named && len(pool) > 0 {
			it := pool[r.IntN(len(pool))]
			for _, m := range mats {
				if m.ItemID == it.ID {
					return
				}
			}
			mats = append(mats, Material{ItemID: it.ID, Qty: 1 + r.IntN(2)})
			return
		}
		if len(pool) == 0 && !missing[kind] {
			missing[kind] = true
			row.Missing = append(row.Missing, kind)
		}
		for _, m := range mats {
			if m.Tag == kind {
				return
			}
		}
		mats = append(mats, Material{Tag: kind, Qty: 1})
	}
	pick(in.Kinds[r.IntN(len(in.Kinds))], true)
	pick(in.Kinds[r.IntN(len(in.Kinds))], r.IntN(2) == 0)
	if r.IntN(3) == 0 {
		pick(in.Kinds[r.IntN(len(in.Kinds))], r.IntN(2) == 0)
	}

	ri := RecipeInput{Materials: mats}
	if row.Template != "" {
		ri, _ = LayTemplate(ri, row.Template, f.tags)
		// A method that wants a kind the campaign has none of asks "any".
		for _, m := range ri.Materials {
			if m.Tag != "" && len(f.byKind[m.Tag]) == 0 && !missing[m.Tag] {
				missing[m.Tag] = true
				row.Missing = append(row.Missing, m.Tag)
			}
		}
	}
	if len(ri.Materials) > maxMaterials {
		ri.Materials = ri.Materials[:maxMaterials]
	}
	row.Steps = ri.Steps
	if row.Steps == nil {
		row.Steps = []Step{}
	}
	names := map[string]string{}
	for _, it := range f.items {
		names[it.ID] = it.Name
	}
	for _, m := range ri.Materials {
		n := names[m.ItemID]
		if m.Tag != "" {
			n = "any " + tagLabel(m.Tag)
		}
		row.Materials = append(row.Materials, GeneratedMaterial{Material: m, Name: n})
	}

	row.Name = genName(r, mk, row.Materials, f, used)
	used[strings.ToLower(row.Name)] = true
	row.Taken = f.taken[strings.ToLower(row.Name)]
	row.ResultID = f.byName[strings.ToLower(row.Name)]
	row.Goal, row.GoalFrom = genGoal(f, mk, in.Echelon)
	return row
}

// genName names a row from a rolling table's rows when one was picked,
// else from its first material and a word for what it is, avoiding names
// already used in the list or by the campaign's recipes.
func genName(r *rand.Rand, mk *MakeKind, mats []GeneratedMaterial, f genFacts, used map[string]bool) string {
	free := func(n string) bool { return !used[strings.ToLower(n)] && !f.taken[strings.ToLower(n)] }
	if len(f.names) > 0 {
		start := r.IntN(len(f.names))
		for i := 0; i < len(f.names); i++ {
			if n := f.names[(start+i)%len(f.names)]; free(n) {
				return n
			}
		}
	}
	base := "Plain"
	if len(mats) > 0 {
		if mats[0].ItemID != "" {
			base = mats[0].Name
		} else {
			base = kindLabel(mats[0].Tag)
		}
	}
	start := r.IntN(len(mk.Nouns))
	for i := 0; i < len(mk.Nouns); i++ {
		if n := base + " " + mk.Nouns[(start+i)%len(mk.Nouns)]; free(n) {
			return n
		}
	}
	return base + " " + mk.Nouns[start]
}

// genGoal takes the system's published goal for the echelon and what is
// made, then for the echelon alone; then the middle of the Director's own
// recipes' goals; and otherwise none.
func genGoal(f genFacts, mk *MakeKind, echelon int) (int, string) {
	for _, g := range f.goals {
		if g.Echelon == echelon && g.For == mk.For {
			return g.Goal, "system"
		}
	}
	for _, g := range f.goals {
		if g.Echelon == echelon && g.For == "" {
			return g.Goal, "system"
		}
	}
	if f.ownGoal > 0 {
		return f.ownGoal, "recipes"
	}
	return 0, ""
}

// middleGoal is the median goal of the campaign's approved recipes, 0 with
// none.
func middleGoal(all []Recipe) int {
	var gs []int
	for _, r := range all {
		if !r.Draft && r.Goal > 0 {
			gs = append(gs, r.Goal)
		}
	}
	if len(gs) == 0 {
		return 0
	}
	sort.Ints(gs)
	return gs[len(gs)/2]
}

// kindLabel is a kind's label as a name starts it ("Herb").
func kindLabel(k string) string {
	for _, t := range Tags {
		if t.Key == k {
			return t.Label
		}
	}
	return k
}
