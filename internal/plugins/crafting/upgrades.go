package crafting

import "encoding/json"

// A bench's upgrades: each piece of the bench (a mortar, a burner) climbs a
// short ladder of levels, and the party has reached one of them. A level
// changes the brew either in the same words as a recipe's rules ("under a
// full moon, 1 in 6, it comes out fine"), which every project rolled at
// this bench then carries out, or in plain words the Director keeps for the
// table. The first level is where every bench starts.

// Upgrade ways a level is come by, as the Director tells the table.
var upgradeHow = []string{"start", "recipe", "bought", "given"}

// Upgrade limits, the editor's.
const (
	maxBenchPieces = 6
	maxLevels      = 5
	maxUpgradeName = 60
	maxUpgradeText = 240
)

// Level is one rung of a piece's ladder.
type Level struct {
	Name    string `json:"name"`
	Changes string `json:"changes,omitempty"`
	Rule    *Rule  `json:"rule,omitempty"`
	How     string `json:"how"`
	Note    string `json:"note,omitempty"`
}

// Piece is one part of a bench with its ladder; Have is the level the party
// has reached, counted from 0.
type Piece struct {
	Name   string  `json:"name"`
	Have   int     `json:"have"`
	Levels []Level `json:"levels"`
}

// Upgrades are a bench's pieces.
type Upgrades struct {
	Pieces []Piece `json:"pieces"`
}

// CleanUpgrades reads a bench's upgrades as the editor or the AI wrote
// them. Like the rules it forgives: a piece without a name or levels is
// left out, an unknown way of coming by a level is "given", and the level
// reached is kept on the ladder.
func CleanUpgrades(raw json.RawMessage) Upgrades {
	out := Upgrades{Pieces: []Piece{}}
	var m map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return out
	}
	list, _ := m["pieces"].([]any)
	for _, v := range list {
		p, ok := v.(map[string]any)
		if !ok || len(out.Pieces) >= maxBenchPieces {
			continue
		}
		piece := Piece{Name: str(p["name"], maxUpgradeName), Levels: []Level{}}
		levels, _ := p["levels"].([]any)
		for _, lv := range levels {
			l, ok := lv.(map[string]any)
			if !ok || len(piece.Levels) >= maxLevels {
				continue
			}
			level := Level{Name: str(l["name"], maxUpgradeName), Changes: str(l["changes"], maxUpgradeText), Note: str(l["note"], maxUpgradeText)}
			if level.Name == "" {
				continue
			}
			if len(piece.Levels) == 0 {
				// Where every bench starts: nothing to come by, nothing it changes.
				level.How, level.Changes = "start", ""
			} else {
				level.How = "given"
				if h, _ := l["how"].(string); oneOfStr(upgradeHow[1:], h) {
					level.How = h
				}
				if r, ok := l["rule"].(map[string]any); ok {
					if rule, ok := cleanRule(r); ok {
						// A level's rule is the bench's, never a draft nor a secret
						// the table can't see on the bench they own.
						rule.Draft, rule.Look = false, nil
						if rule.Players == "secret" {
							rule.Players = "book"
						}
						level.Rule = &rule
					}
				}
			}
			piece.Levels = append(piece.Levels, level)
		}
		if piece.Name == "" || len(piece.Levels) == 0 {
			continue
		}
		have, _ := number(p["have"])
		piece.Have = clamp(have, 0, len(piece.Levels)-1)
		out.Pieces = append(out.Pieces, piece)
	}
	return out
}

// Rules are the rules of every level the party has reached, which a project
// rolled at this bench carries out beside its recipe's own.
func (u Upgrades) Rules() []Rule {
	var out []Rule
	for _, p := range u.Pieces {
		for i := 1; i <= p.Have && i < len(p.Levels); i++ {
			if r := p.Levels[i].Rule; r != nil {
				out = append(out, *r)
			}
		}
	}
	return out
}
