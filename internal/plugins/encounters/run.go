package encounters

import (
	"fmt"
	"slices"
	"strings"
)

// A run is a fight the DM team is playing out on the DM Screen's Fight tab,
// for a table that isn't using Foundry: the turn order, each creature's hit
// points and conditions. There is one per campaign. It copies the encounter's
// heroes and creature rows when it starts, so changing or deleting the
// encounter later doesn't touch a fight in progress.

// Kinds of line in a run.
const (
	LineHero     = "hero"
	LineCreature = "creature"
)

// Run limits. They bound the stored document; the widget keeps to them too.
const (
	MaxRunLines      = 60
	MaxRunConds      = 12
	MaxCondChars     = 40
	MaxLineNameChars = 80
	MaxRound         = 9999
	MinInit, MaxInit = -99, 999
	MaxHitPoints     = 99999
	// MaxRunBodyBytes is well above the largest legal run.
	MaxRunBodyBytes = 128 << 10
)

// Run is the fight as stored and as the Fight tab edits it.
type Run struct {
	EncounterID string `json:"encounterId"`
	// Name is the encounter's name when the fight started.
	Name  string `json:"name"`
	Round int    `json:"round"`
	// Turn is the id of the line whose turn it is; empty before the first
	// turn, while the DM types initiative.
	Turn  string    `json:"turn"`
	Lines []RunLine `json:"lines"`
}

// RunLine is one hero, or one creature row with a box of hit points for
// each copy.
type RunLine struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	// EntityID is the line's page, kept from the encounter. The service
	// never takes one from a save, so a run can't be made to point at a page
	// the encounter didn't name.
	EntityID string `json:"entityId"`
	Group    string `json:"group"`
	// Init is the initiative the DM typed; nil until then.
	Init *int `json:"init"`
	// Max is the line's full hit points; nil until the DM types it.
	Max *int `json:"max"`
	// HP holds each copy's current hit points; nil means full.
	HP []*int `json:"hp"`
	// Conds are the line's conditions, by name.
	Conds []string `json:"conds"`
	// Waiting is a creature that starts "later": listed, but out of the
	// turn order until the DM brings it in.
	Waiting bool `json:"waiting"`
}

// RunView is what the Fight tab receives. Run is nil when no fight is on;
// Ready lists the encounters the DM can start.
type RunView struct {
	Run        *Run        `json:"run"`
	Version    int         `json:"version"`
	Ready      []Summary   `json:"ready"`
	StatBlocks []StatBlock `json:"statBlocks"`
}

// RunSave is a save body: the whole run and the version it was read at.
type RunSave struct {
	Version int  `json:"version"`
	Run     *Run `json:"run"`
}

// RunStart is a start body.
type RunStart struct {
	EncounterID string `json:"encounterId"`
}

func inRange(field string, p *int, lo, hi int) error {
	if p != nil && (*p < lo || *p > hi) {
		return errInvalid(fmt.Sprintf("%s must be between %d and %d", field, lo, hi))
	}
	return nil
}

// validateRun checks a run's shape and returns it cleaned. Whether a line's
// page may be named is the service's check, since it compares with the
// stored run.
func validateRun(in *Run) (*Run, error) {
	if in == nil {
		return nil, errInvalid("a fight is required")
	}
	out := *in
	var err error
	if out.EncounterID != "" && !validToken(out.EncounterID) {
		return nil, errInvalid("the encounter id is not valid")
	}
	if out.Name, err = cleanLine("name", out.Name, MaxNameChars); err != nil {
		return nil, err
	}
	if out.Round < 1 || out.Round > MaxRound {
		return nil, errInvalid(fmt.Sprintf("the round must be between 1 and %d", MaxRound))
	}
	if len(out.Lines) > MaxRunLines {
		return nil, errInvalid(fmt.Sprintf("a fight holds at most %d lines", MaxRunLines))
	}
	ids := map[string]bool{}
	out.Lines = make([]RunLine, 0, len(in.Lines))
	for _, l := range in.Lines {
		c, err := validateLine(l)
		if err != nil {
			return nil, err
		}
		if ids[c.ID] {
			return nil, errInvalid("two lines share an id")
		}
		ids[c.ID] = true
		out.Lines = append(out.Lines, c)
	}
	if out.Turn != "" && !ids[out.Turn] {
		return nil, errInvalid("the turn names a line that isn't in the fight")
	}
	return &out, nil
}

func validateLine(l RunLine) (RunLine, error) {
	var err error
	if !validToken(l.ID) {
		return RunLine{}, errInvalid("a line id is not valid")
	}
	if l.Kind != LineHero && l.Kind != LineCreature {
		return RunLine{}, errInvalid("a line must be a hero or a creature")
	}
	if l.Name, err = cleanLine("a name", l.Name, MaxLineNameChars); err != nil {
		return RunLine{}, err
	}
	if l.Name == "" {
		return RunLine{}, errInvalid("every line needs a name")
	}
	l.Group = strings.ToUpper(strings.TrimSpace(l.Group))
	if l.Group != "" && !groups[l.Group] {
		return RunLine{}, errInvalid("a group must be A to H")
	}
	if err := inRange("initiative", l.Init, MinInit, MaxInit); err != nil {
		return RunLine{}, err
	}
	if err := inRange("hit points", l.Max, 0, MaxHitPoints); err != nil {
		return RunLine{}, err
	}
	if len(l.HP) < 1 || len(l.HP) > MaxCreatureCount {
		return RunLine{}, errInvalid(fmt.Sprintf("a line has 1 to %d hit point boxes", MaxCreatureCount))
	}
	if l.Kind == LineHero && len(l.HP) != 1 {
		return RunLine{}, errInvalid("a hero has one hit point box")
	}
	for _, hp := range l.HP {
		if err := inRange("hit points", hp, -MaxHitPoints, MaxHitPoints); err != nil {
			return RunLine{}, err
		}
	}
	if len(l.Conds) > MaxRunConds {
		return RunLine{}, errInvalid(fmt.Sprintf("a line holds at most %d conditions", MaxRunConds))
	}
	conds := make([]string, 0, len(l.Conds))
	for _, c := range l.Conds {
		c, err := cleanLine("a condition", c, MaxCondChars)
		if err != nil {
			return RunLine{}, err
		}
		if c != "" && !slices.Contains(conds, c) {
			conds = append(conds, c)
		}
	}
	l.Conds = conds
	return l, nil
}

// playedRows is the creature rows the fight plays: the version the recorded
// results pick, or the base rows when none does.
func playedRows(e *Encounter) []Creature {
	if e.Picked == nil || e.Picked.Version == "" {
		return e.Creatures
	}
	var v *Version
	for i := range e.Versions {
		if e.Versions[i].ID == e.Picked.Version {
			v = &e.Versions[i]
		}
	}
	if v == nil {
		return e.Creatures
	}
	counts := map[string]int{}
	for _, c := range v.Counts {
		counts[c.Row] = c.Count
	}
	var out []Creature
	for _, c := range e.Creatures {
		if slices.Contains(v.Remove, c.ID) {
			continue
		}
		if n, ok := counts[c.ID]; ok {
			c.Count = n
		}
		out = append(out, c)
	}
	return append(out, v.Add...)
}

// newRun lays out a fight from an encounter: its heroes, then its creature
// rows, each with a full hit point box per copy. "Later" rows wait.
func newRun(e *Encounter) *Run {
	r := &Run{EncounterID: e.ID, Name: e.Name, Round: 1, Lines: []RunLine{}}
	for _, h := range e.Heroes {
		r.Lines = append(r.Lines, RunLine{
			ID: "h-" + h.ID, Kind: LineHero, Name: h.Name, EntityID: h.ID,
			HP: []*int{nil}, Conds: []string{},
		})
	}
	for _, c := range playedRows(e) {
		n := min(max(c.Count, 1), MaxCreatureCount)
		r.Lines = append(r.Lines, RunLine{
			ID: c.ID, Kind: LineCreature, Name: c.Name, EntityID: c.EntityID, Group: c.Group,
			HP: make([]*int, n), Conds: []string{}, Waiting: c.Start == StartLater,
		})
	}
	if len(r.Lines) > MaxRunLines {
		r.Lines = r.Lines[:MaxRunLines]
	}
	return r
}
