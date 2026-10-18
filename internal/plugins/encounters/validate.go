package encounters

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits. They bound the stored document and so a request body; the widget
// enforces the same numbers as a courtesy.
const (
	MaxNameChars     = 120
	MaxHeroes        = 12
	MaxCreatureRows  = 30
	MaxCreatureCount = 40
	MaxRowIDChars    = 64
	MaxStages        = 8
	MaxStageForms    = 10
	MaxStageArrive   = 30
	MaxNoteChars     = 200
	MaxFormNameChars = 80
	// MaxPerPage keeps one page's widget a list a DM can scan; a campaign
	// can still hold any number of encounters across its pages.
	MaxPerPage = 50
	// MaxBodyBytes is well above the largest legal encounter and low enough
	// that a junk body is refused.
	MaxBodyBytes = 64 << 10
)

// groups are the initiative groups a row may join.
var groups = map[string]bool{"A": true, "B": true, "C": true, "D": true, "E": true, "F": true, "G": true, "H": true}

// formIDPattern is the shape of a game system's form id; it keeps an id from
// carrying markup or path characters.
var formIDPattern = regexp.MustCompile(`^[a-z0-9]{1,24}$`)

func validStart(s string) bool {
	return s == StartSeen || s == StartHidden || s == StartLater
}

// validToken reports whether s is a short safe id: letters, digits, _ and -.
// Page ids (UUIDs) and the row ids the builder makes both fit; anything else
// is refused so an id can never carry markup or path characters.
func validToken(s string) bool {
	if s == "" || len(s) > MaxRowIDChars {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// cleanLine checks one line of plain text, trimmed, so stored text is always
// safe to show with textContent and to log.
func cleanLine(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) {
		return "", errInvalid(field + " is not valid text")
	}
	if utf8.RuneCountInString(s) > max {
		return "", errInvalid(fmt.Sprintf("%s is too long (at most %d characters)", field, max))
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", errInvalid(field + " contains a character that is not allowed")
		}
	}
	return s, nil
}

// validateInput checks a save body's shape. Whether the named pages exist in
// the campaign is the service's check, since it needs the directory. Rows are
// returned cleaned: trimmed names, group upper-cased, a missing start read as
// seen.
func validateInput(in SaveInput) (name string, d doc, err error) {
	name, err = cleanLine("name", in.Name, MaxNameChars)
	if err != nil {
		return "", doc{}, err
	}
	if name == "" {
		return "", doc{}, errInvalid("give the encounter a name")
	}
	if len(in.Heroes) > MaxHeroes {
		return "", doc{}, errInvalid(fmt.Sprintf("an encounter is built for at most %d heroes", MaxHeroes))
	}
	seen := map[string]bool{}
	d.Heroes = make([]string, 0, len(in.Heroes))
	for _, id := range in.Heroes {
		if !validToken(id) {
			return "", doc{}, errInvalid("a hero is not a page")
		}
		if !seen[id] {
			seen[id] = true
			d.Heroes = append(d.Heroes, id)
		}
	}
	if len(in.Creatures) > MaxCreatureRows {
		return "", doc{}, errInvalid(fmt.Sprintf("an encounter has at most %d creature rows", MaxCreatureRows))
	}
	rowIDs := map[string]bool{}
	d.Creatures = make([]Creature, 0, len(in.Creatures))
	for _, c := range in.Creatures {
		if !validToken(c.ID) || rowIDs[c.ID] {
			return "", doc{}, errInvalid("a creature row has a bad id")
		}
		rowIDs[c.ID] = true
		if c.EntityID != "" && !validToken(c.EntityID) {
			return "", doc{}, errInvalid("a creature is not a page")
		}
		if c.Name, err = cleanLine("creature name", c.Name, MaxNameChars); err != nil {
			return "", doc{}, err
		}
		if c.Name == "" {
			return "", doc{}, errInvalid("every creature row needs a name")
		}
		if c.Count < 1 || c.Count > MaxCreatureCount {
			return "", doc{}, errInvalid(fmt.Sprintf("%s: a row holds 1 to %d creatures", c.Name, MaxCreatureCount))
		}
		c.Group = strings.ToUpper(strings.TrimSpace(c.Group))
		if c.Group == "" {
			c.Group = "A"
		}
		if !groups[c.Group] {
			return "", doc{}, errInvalid(c.Name + ": initiative groups are A to H")
		}
		if c.Start == "" {
			c.Start = StartSeen
		}
		if !validStart(c.Start) {
			return "", doc{}, errInvalid(c.Name + ": a creature starts seen, hidden or later")
		}
		d.Creatures = append(d.Creatures, c)
	}
	if d.Stages, err = validateStages(in.Stages, d.Creatures); err != nil {
		return "", doc{}, err
	}
	return name, d, nil
}

// validateStages checks the stages against the rows saved with them, so a
// stage can never point at a row this save does not have. A stage must do
// something: a form change or an arrival.
func validateStages(in []Stage, rows []Creature) ([]Stage, error) {
	if len(in) > MaxStages {
		return nil, errInvalid(fmt.Sprintf("an encounter has at most %d stages", MaxStages))
	}
	byID := make(map[string]Creature, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	ids := map[string]bool{}
	out := make([]Stage, 0, len(in))
	for _, s := range in {
		if !validToken(s.ID) || ids[s.ID] {
			return nil, errInvalid("a stage has a bad id")
		}
		ids[s.ID] = true
		switch s.When.Kind {
		case WhenManual:
			s.When = StageWhen{Kind: WhenManual}
		case WhenStamina:
			if _, ok := byID[s.When.Row]; !ok {
				return nil, errInvalid("a stage watches a creature row that is not in this encounter")
			}
			if s.When.Percent < 1 || s.When.Percent > 99 {
				return nil, errInvalid("a stage's Stamina percent is 1 to 99")
			}
			s.When = StageWhen{Kind: WhenStamina, Row: s.When.Row, Percent: s.When.Percent}
		case WhenRound:
			if s.When.Round < 2 || s.When.Round > 99 {
				return nil, errInvalid("a stage's round is 2 to 99")
			}
			s.When = StageWhen{Kind: WhenRound, Round: s.When.Round}
		default:
			return nil, errInvalid("a stage starts manually, at a Stamina level or at a round")
		}
		if len(s.Forms) > MaxStageForms {
			return nil, errInvalid(fmt.Sprintf("a stage changes at most %d forms", MaxStageForms))
		}
		forms := make([]StageForm, 0, len(s.Forms))
		for _, f := range s.Forms {
			row, ok := byID[f.Row]
			if !ok {
				return nil, errInvalid("a form change names a creature row that is not in this encounter")
			}
			if row.EntityID == "" {
				return nil, errInvalid(row.Name + " has no page, so it has no other forms")
			}
			if f.Form != "" && !formIDPattern.MatchString(f.Form) {
				return nil, errInvalid("a form change has a bad form id")
			}
			var err error
			if f.FormName, err = cleanLine("form name", f.FormName, MaxFormNameChars); err != nil {
				return nil, err
			}
			forms = append(forms, f)
		}
		s.Forms = forms
		if len(s.Arrive) > MaxStageArrive {
			return nil, errInvalid(fmt.Sprintf("a stage brings in at most %d rows", MaxStageArrive))
		}
		arrive := make([]string, 0, len(s.Arrive))
		seen := map[string]bool{}
		for _, id := range s.Arrive {
			if _, ok := byID[id]; !ok {
				return nil, errInvalid("a stage brings in a creature row that is not in this encounter")
			}
			if seen[id] {
				return nil, errInvalid("a stage lists the same row twice")
			}
			seen[id] = true
			arrive = append(arrive, id)
		}
		s.Arrive = arrive
		if len(s.Forms) == 0 && len(s.Arrive) == 0 {
			return nil, errInvalid("a stage needs a form change or an arrival")
		}
		if s.How == "" {
			s.How = HowReveal
		}
		if s.How != HowReveal && s.How != HowQuiet {
			return nil, errInvalid("players see a form change quietly or revealed")
		}
		var err error
		if s.Note, err = cleanLine("stage note", s.Note, MaxNoteChars); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
