package crafting

// The fixed library a recipe is built from. Every list here is also sent to
// the editor (Library), so the editor and the checks on save read the same
// words. A step kind exists only when the bench engine (craft_scene.js) can
// play it; the forge's and the kitchen's steps arrive with their benches.

// Tag is a crafting tag on an Armory item: what kind of material it is.
type Tag struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Tags are the material kinds a recipe can ask for, in the order the editor
// shows them.
var Tags = []Tag{
	{"metal", "Metal"},
	{"remnant_part", "Remnant part"},
	{"body_part", "Body part"},
	{"bone", "Bone"},
	{"hide", "Hide"},
	{"liquid", "Liquid"},
	{"herb", "Herb"},
	{"magic_item", "Magic item"},
	{"magic_essence", "Magic essence"},
}

// Tool is one tool a bench has and a step uses.
type Tool struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Tools are the tools the apothecary bench draws.
var Tools = []Tool{
	{"mortar", "mortar"},
	{"flask", "flask"},
	{"burner", "burner and bellows"},
}

// StepOption is a step's one setting, offered as a short list in the editor
// and clamped to [Min, Max] on save, which is what the engine accepts.
type StepOption struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Choices []int    `json:"choices"`
	Words   []string `json:"words,omitempty"`
	Min     int      `json:"min"`
	Max     int      `json:"max"`
	Default int      `json:"default"`
}

// StepKind is one kind of step. Item steps use one of the recipe's
// materials; Accepts limits which tags that material may carry.
type StepKind struct {
	Key     string      `json:"key"`
	Name    string      `json:"name"`
	Blurb   string      `json:"blurb"`
	Tool    string      `json:"tool,omitempty"`
	Graded  bool        `json:"graded"`
	Item    bool        `json:"item"`
	Accepts []string    `json:"accepts,omitempty"`
	Into    []string    `json:"into,omitempty"`
	Option  *StepOption `json:"option,omitempty"`
}

// StepKinds are the steps the bench engine plays, under the names the
// editor mockup gives them: simmer is "Keep it steady" and strike is "Catch
// the moment".
var StepKinds = []StepKind{
	{Key: "add", Name: "Add", Blurb: "Put an ingredient in", Item: true, Into: []string{"mortar", "flask"}},
	{Key: "grind", Name: "Grind", Blurb: "Circle the pestle", Tool: "mortar", Accepts: []string{"herb", "bone", "body_part", "remnant_part"},
		Option: &StepOption{Key: "turns", Label: "turns", Choices: []int{2, 3, 5}, Min: 1, Max: 8, Default: 3}},
	{Key: "tip", Name: "Tip into the flask", Blurb: "Drag the mortar over", Tool: "mortar"},
	{Key: "pour", Name: "Pour to the line", Blurb: "Stop at the gold line", Tool: "flask", Graded: true, Item: true, Accepts: []string{"liquid", "magic_essence"}},
	{Key: "simmer", Name: "Keep it steady", Blurb: "Hold the heat in its band", Tool: "burner", Graded: true,
		Option: &StepOption{Key: "secs", Label: "", Choices: []int{5, 10}, Words: []string{"Short", "Long"}, Min: 2, Max: 20, Default: 5}},
	{Key: "strike", Name: "Catch the moment", Blurb: "Press on the gold", Graded: true,
		Option: &StepOption{Key: "n", Label: "presses", Choices: []int{1, 2, 3}, Min: 1, Max: 6, Default: 2}},
}

// stepKind finds a step kind by key.
func stepKind(k string) *StepKind {
	for i := range StepKinds {
		if StepKinds[i].Key == k {
			return &StepKinds[i]
		}
	}
	return nil
}

// BenchKind is a kind of crafting station: what it is called and the tools
// it has. A campaign's benches are places of one of these kinds.
type BenchKind struct {
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	Tools []string `json:"tools"`
}

// BenchKinds are the stations the engine can draw today.
var BenchKinds = []BenchKind{
	{Key: "apothecary", Name: "Apothecary bench", Tools: []string{"mortar", "flask", "burner"}},
}

// TemplateStep is one step a template lays down; Want names the tags the
// material it uses should carry, so the template can pick or add one.
type TemplateStep struct {
	T     string   `json:"t"`
	Want  []string `json:"want,omitempty"`
	To    string   `json:"to,omitempty"`
	Turns int      `json:"turns,omitempty"`
	Secs  int      `json:"secs,omitempty"`
	N     int      `json:"n,omitempty"`
}

// Template fills in a recipe's steps and bench in one pick.
type Template struct {
	Key   string         `json:"key"`
	Name  string         `json:"name"`
	Bench string         `json:"bench"`
	Steps []TemplateStep `json:"steps"`
}

// Templates are the two apothecary brews: steady ones end on the heat
// gauge, volatile ones on the timing bar.
var Templates = []Template{
	{Key: "steady", Name: "Steady brew", Bench: "apothecary", Steps: []TemplateStep{
		{T: "add", Want: []string{"herb", "bone", "body_part", "remnant_part"}, To: "mortar"}, {T: "grind", Turns: 3}, {T: "tip"},
		{T: "pour", Want: []string{"liquid", "magic_essence"}}, {T: "simmer", Secs: 5}}},
	{Key: "volatile", Name: "Volatile brew", Bench: "apothecary", Steps: []TemplateStep{
		{T: "add", Want: []string{"herb", "bone", "body_part", "remnant_part"}, To: "mortar"}, {T: "grind", Turns: 2}, {T: "tip"},
		{T: "add", Want: []string{"liquid", "magic_essence"}, To: "flask"}, {T: "strike", N: 2}}},
}

// Choice is one value of a setting with its words.
type Choice struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Skins are the two looks a bench plays in; a campaign picks the default and
// players may switch.
var Skins = []Choice{{"desk", "The desk"}, {"journal", "The journal"}}

// Difficulties set how tight the bench's timing is (craft_scene.js DIFF).
var Difficulties = []Choice{{"easy", "Forgiving"}, {"std", "Standard"}, {"hard", "Exacting"}}

// Moments are the times a recipe's look plays on the bench, beside its rules.
var Moments = []string{"success", "progress", "failure", "breakthrough"}

// Look effects a moment or rule may place, from the shared item-look engine
// (item_look_fx.js LOOKS, MOMENT_SPOTS) and the rules editor's two moves.
var (
	lookNames   = setOf("ember", "frost", "storm", "bloom", "dawn", "rot", "venom", "glass", "quiet")
	lookMoves   = setOf("shake", "dim")
	momentSpots = []string{"window", "heat", "work", "bench", "result"}
)

func setOf(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func tagKnown(k string) bool {
	for _, t := range Tags {
		if t.Key == k {
			return true
		}
	}
	return false
}

func benchKind(k string) *BenchKind {
	for i := range BenchKinds {
		if BenchKinds[i].Key == k {
			return &BenchKinds[i]
		}
	}
	return nil
}

func templateKnown(k string) bool {
	for _, t := range Templates {
		if t.Key == k {
			return true
		}
	}
	return false
}

func choiceKnown(list []Choice, k string) bool {
	for _, c := range list {
		if c.Key == k {
			return true
		}
	}
	return false
}
