package decks

import "strconv"

// ruleOption is one value a rule's blank can take, in words.
type ruleOption struct{ Value, Label string }

// ruleBlank is the choice in a rule's sentence. Key is the settings key.
type ruleBlank struct {
	Key     string
	Options []ruleOption
}

// rulePart is a run of a rule's sentence: plain words or a blank.
type rulePart struct {
	Text  string
	Blank *ruleBlank
}

// rule is one sentence. It may hold more than one blank when one idea needs
// two choices; there is still exactly one blank per settings key.
type rule struct {
	Parts []rulePart
	Quiet string
}

type ruleGroup struct {
	Title string
	Rules []rule
}

func opts(pairs ...string) []ruleOption {
	var out []ruleOption
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, ruleOption{pairs[i], pairs[i+1]})
	}
	return out
}

func blank(key string, o []ruleOption) rulePart {
	return rulePart{Blank: &ruleBlank{Key: key, Options: o}}
}
func words(t string) rulePart { return rulePart{Text: t} }

// ruleGroups are the Rules tab: four groups of sentences, one blank for each
// of the deck's settings and nothing else. workshop_test.go pins the keys and
// the option values to the lists in settings.go.
func ruleGroups() []ruleGroup {
	return []ruleGroup{
		{"Drawing", []rule{
			{Parts: []rulePart{blank("draws", opts("directors", "Only the Directors", "players", "The Directors and chosen players")), words(" draw from this deck.")},
				Quiet: "Choose the players, and how many draws each has, on the Players tab."},
			{Parts: []rulePart{words("When the pile runs out, "), blank("empty", opts("stop", "stop and say so", "reshuffle", "shuffle the used pile back in")), words(".")}},
		}},
		{"What players see", []rule{
			{Parts: []rulePart{words("Players who can see this deck know "), blank("knows", opts("nothing", "nothing about it", "count", "how many cards are left", "kinds", "how many of each kind are left", "faces", "every card in it")), words(".")}},
			{Parts: []rulePart{words("A card dealt to a hero arrives "), blank("arrives", opts("down", "face down", "up", "face up")), words(", and the other players see "), blank("othersSee", opts("nothing", "nothing", "back", "its back", "face", "its face once the holder shows it")), words(".")}},
		}},
		{"After a card is used", []rule{
			{Parts: []rulePart{words("A used card goes "), blank("fate", opts(FateBack, "back into the pile", FateUsed, "to the used pile", FateGone, "out of the game", FatePlay, "into play, in front of the hero")), words(" unless the card says otherwise.")}},
			{Parts: []rulePart{words("Players "), blank("discard", opts("false", "may not", "true", "may")), words(" throw away cards in their own hand.")}},
		}},
		{"Telling you", []rule{
			{Parts: []rulePart{words("When a card's moment comes, tell me with "), blank("notice", opts("slide", "a notice that slides in", "count", "a count on the Table", "quiet", "nothing, quietly")), words(".")},
				Quiet: "This is saved now and takes effect in a later step."},
		}},
	}
}

// ruleValue reads a settings value as the text a blank holds.
func ruleValue(s Settings, key string) string {
	switch key {
	case "draws":
		return s.Draws
	case "knows":
		return s.Knows
	case "othersSee":
		return s.OthersSee
	case "arrives":
		return s.Arrives
	case "fate":
		return s.Fate
	case "empty":
		return s.Empty
	case "notice":
		return s.Notice
	case "discard":
		return strconv.FormatBool(s.Discard)
	}
	return ""
}

// ruleLabel names a blank for a screen reader.
func ruleLabel(key string) string {
	switch key {
	case "draws":
		return "Who draws from this deck"
	case "knows":
		return "What players who can see the deck know"
	case "othersSee":
		return "What the other players see of a card dealt to a hero"
	case "arrives":
		return "How a card dealt to a hero arrives"
	case "fate":
		return "Where a used card goes"
	case "empty":
		return "What happens when the pile runs out"
	case "discard":
		return "Whether players may throw away cards in their own hand"
	case "notice":
		return "How you are told when a card's moment comes"
	}
	return key
}
