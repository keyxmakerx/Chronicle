// Package calendar - event_rolls.go is a roll on an event: dice or a rolling
// table, rolled for each night the event lands on (the Bloodnight on every
// full moon), with result bands saying what players and the Director see.
// This file holds the shapes and the pure parts: reading a dice expression,
// checking a roll's settings, picking a band and shaping a night for a
// viewer. Storage is event_roll_repository.go, the rules event_rolls_service.go.
package calendar

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Where a roll's result comes from.
const (
	RollSourceDice  = "dice"
	RollSourceTable = "table"
)

// How a night's result was stored: rolled ahead (one night or the whole
// year), rolled when the night arrived, or typed by the Director.
const (
	RollNightAhead  = "ahead"
	RollNightOnDay  = "night"
	RollNightManual = "manual"
)

// Limits keep one roll small: it is a few results with a line or two each.
const (
	maxRollBands        = 12
	maxRollBandName     = 80
	maxRollBandText     = 1000
	maxRollDiceCount    = 100
	maxRollDiceSides    = 1000
	maxRollDiceModifier = 1000
	maxRollTableID      = 64
	// maxRollNightsPerCall bounds a lock, clear or year roll, as the weather
	// day writes are bounded.
	maxRollNightsPerCall = 1000
	// rollOnTheNightWindow is how far back a read rolls a night that arrived
	// unrolled. A calendar moved forward by weeks still rolls the nights it
	// skipped over, but an old calendar does not fill years of history the
	// first time someone looks.
	rollOnTheNightWindow = 31
)

// RollConfig is an event's roll, stored as JSON on calendar_event_rolls.
type RollConfig struct {
	Source string `json:"source"`
	// Dice is the expression for a dice roll: "2d10", "1d20+3", "3d6-1".
	Dice string `json:"dice,omitempty"`
	// TableID names one of the campaign's rolling tables for a table roll.
	TableID string `json:"table_id,omitempty"`
	// OnTheNight rolls a night that arrives without a result.
	OnTheNight bool `json:"on_the_night"`
	// Results are a dice roll's bands, lowest first. Every band but the last
	// has Max, the highest total it covers; the last covers the rest.
	Results []RollBand `json:"results,omitempty"`
	// TellPlayers is a table roll's "tell players on the night": a table has
	// no bands to carry it.
	TellPlayers bool `json:"tell_players,omitempty"`
}

// RollBand is one result of a dice roll.
type RollBand struct {
	Max  *int   `json:"max,omitempty"`
	Name string `json:"name"`
	// Players is what players are told on the night, when they are told.
	Players string `json:"players,omitempty"`
	// Director is for the Director alone.
	Director string `json:"director,omitempty"`
	// TellPlayers shows the night's player text once the night arrives.
	TellPlayers bool `json:"tell_players,omitempty"`
}

// RollNight is one night's stored result, as the repository reads it.
type RollNight struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
	// Total and Faces are a dice roll's; Entry a table roll's name.
	Total  *int   `json:"total,omitempty"`
	Faces  []int  `json:"faces,omitempty"`
	Entry  string `json:"entry,omitempty"`
	Source string `json:"source"`
	Locked bool   `json:"locked"`
	// Told is nil when the night follows its result's own setting.
	Told *bool `json:"told,omitempty"`
}

func (n RollNight) date() DayDate { return DayDate{Year: n.Year, Month: n.Month, Day: n.Day} }

// RollNightView is a night shaped for one viewer. A player gets the date,
// the result's name and its player text for a night they were told about,
// nothing else: no total, no lock, no Director's text.
type RollNightView struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
	// Result is the band's or table entry's name.
	Result string `json:"result,omitempty"`
	// Players is the player text; for a player it is set only when told.
	Players  string `json:"players,omitempty"`
	Director string `json:"director,omitempty"`
	Total    *int   `json:"total,omitempty"`
	Faces    []int  `json:"faces,omitempty"`
	Source   string `json:"source,omitempty"`
	Locked   *bool  `json:"locked,omitempty"`
	Told     *bool  `json:"told,omitempty"`
	// BandIndex is the band a dice total falls in, -1 for none. Director only.
	BandIndex *int `json:"band_index,omitempty"`
}

// EventRollView is everything a viewer may know about an event's roll. A
// player gets Nights only, and only the told ones up to today.
type EventRollView struct {
	EventID string          `json:"event_id"`
	Config  *RollConfig     `json:"config,omitempty"`
	Nights  []RollNightView `json:"nights"`
	// Upcoming are the event's dates in the asked year that have no result,
	// so the Director's list can show them. Director only.
	Upcoming []DayDate `json:"upcoming,omitempty"`
	// Truncated says the event's dates could not all be worked out.
	Truncated bool `json:"truncated,omitempty"`
}

// DiceExpr is a parsed dice expression.
type DiceExpr struct {
	Count, Sides, Modifier int
}

var diceRe = regexp.MustCompile(`^(\d{0,3})d(\d{1,4})(?:([+-])(\d{1,4}))?$`)

// ParseDice reads "NdM", "NdM+K" or "NdM-K" (N defaults to 1), within the
// roll limits.
func ParseDice(s string) (DiceExpr, error) {
	m := diceRe.FindStringSubmatch(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "")))
	if m == nil {
		return DiceExpr{}, apperror.NewValidation("dice must look like 2d10, 1d20+3 or 3d6-1")
	}
	d := DiceExpr{Count: 1}
	if m[1] != "" {
		d.Count, _ = strconv.Atoi(m[1])
	}
	d.Sides, _ = strconv.Atoi(m[2])
	if m[4] != "" {
		d.Modifier, _ = strconv.Atoi(m[4])
		if m[3] == "-" {
			d.Modifier = -d.Modifier
		}
	}
	if d.Count < 1 || d.Count > maxRollDiceCount {
		return DiceExpr{}, apperror.NewValidation(fmt.Sprintf("roll between 1 and %d dice", maxRollDiceCount))
	}
	if d.Sides < 2 || d.Sides > maxRollDiceSides {
		return DiceExpr{}, apperror.NewValidation(fmt.Sprintf("dice need between 2 and %d sides", maxRollDiceSides))
	}
	if d.Modifier > maxRollDiceModifier || d.Modifier < -maxRollDiceModifier {
		return DiceExpr{}, apperror.NewValidation("the modifier is too large")
	}
	return d, nil
}

// Range is the lowest and highest total the expression can give.
func (d DiceExpr) Range() (lo, hi int) {
	return d.Count + d.Modifier, d.Count*d.Sides + d.Modifier
}

// Roll throws the dice with crypto/rand, as Chronicle.dice does in the
// browser: a stored result is a real roll, not a seeded one.
func (d DiceExpr) Roll() (total int, faces []int, err error) {
	faces = make([]int, d.Count)
	total = d.Modifier
	for i := range faces {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(d.Sides)))
		if err != nil {
			return 0, nil, fmt.Errorf("roll dice: %w", err)
		}
		faces[i] = int(n.Int64()) + 1
		total += faces[i]
	}
	return total, faces, nil
}

// Normalize checks and tidies a roll's settings. A dice roll needs at least
// one band, bands in rising order with Max on every one but the last; a
// table roll needs a table id and keeps no bands.
func (c *RollConfig) Normalize() error {
	switch c.Source {
	case RollSourceDice:
		c.TableID = ""
		c.TellPlayers = false
		if _, err := ParseDice(c.Dice); err != nil {
			return err
		}
		c.Dice = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(c.Dice), " ", ""))
		if len(c.Results) == 0 {
			return apperror.NewValidation("a dice roll needs at least one result")
		}
		if len(c.Results) > maxRollBands {
			return apperror.NewValidation(fmt.Sprintf("a roll has at most %d results", maxRollBands))
		}
		prev := 0
		for i := range c.Results {
			b := &c.Results[i]
			b.Name = strings.TrimSpace(b.Name)
			b.Players = strings.TrimSpace(b.Players)
			b.Director = strings.TrimSpace(b.Director)
			if b.Name == "" {
				return apperror.NewValidation("every result needs a name")
			}
			if utf8.RuneCountInString(b.Name) > maxRollBandName {
				return apperror.NewValidation(fmt.Sprintf("a result's name is at most %d characters", maxRollBandName))
			}
			if utf8.RuneCountInString(b.Players) > maxRollBandText || utf8.RuneCountInString(b.Director) > maxRollBandText {
				return apperror.NewValidation(fmt.Sprintf("a result's text is at most %d characters", maxRollBandText))
			}
			last := i == len(c.Results)-1
			if last {
				b.Max = nil
				continue
			}
			if b.Max == nil {
				return apperror.NewValidation("every result but the last needs the highest total it covers")
			}
			if i > 0 && *b.Max <= prev {
				return apperror.NewValidation("results must be listed lowest first, each covering higher totals than the one before")
			}
			prev = *b.Max
		}
	case RollSourceTable:
		c.Dice = ""
		c.Results = nil
		c.TableID = strings.TrimSpace(c.TableID)
		if c.TableID == "" || len(c.TableID) > maxRollTableID {
			return apperror.NewValidation("a table roll needs one of the campaign's rolling tables")
		}
	default:
		return apperror.NewValidation(`a roll's source is "dice" or "table"`)
	}
	return nil
}

// bandFor returns the index of the band total falls in, -1 when there are
// no bands.
func (c *RollConfig) bandFor(total int) int {
	for i, b := range c.Results {
		if b.Max == nil || total <= *b.Max {
			return i
		}
	}
	return -1
}

// view shapes a stored night for a viewer. director sees everything; a
// player gets the name and player text of a told night, and nothing for any
// other (ok is false, and the caller leaves the night out).
func (c *RollConfig) view(n RollNight, director bool) (v RollNightView, ok bool) {
	v = RollNightView{Year: n.Year, Month: n.Month, Day: n.Day}
	band := -1
	tellDefault := false
	switch {
	case n.Entry != "":
		v.Result = n.Entry
		v.Players = n.Entry
		tellDefault = c.TellPlayers
	case n.Total != nil:
		band = c.bandFor(*n.Total)
		if band >= 0 {
			b := c.Results[band]
			v.Result, v.Players, v.Director = b.Name, b.Players, b.Director
			tellDefault = b.TellPlayers
		}
	}
	told := tellDefault
	if n.Told != nil {
		told = *n.Told
	}
	if !director {
		if !told || v.Players == "" {
			return RollNightView{}, false
		}
		return RollNightView{Year: n.Year, Month: n.Month, Day: n.Day, Result: v.Result, Players: v.Players}, true
	}
	locked := n.Locked
	v.Total, v.Faces, v.Source, v.Locked, v.Told, v.BandIndex = n.Total, n.Faces, n.Source, &locked, &told, &band
	return v, true
}

// encodeFaces and decodeFaces store a dice roll's faces as a short JSON
// array, so the Director can see what was thrown.
func encodeFaces(f []int) *string {
	if len(f) == 0 {
		return nil
	}
	b, _ := json.Marshal(f)
	s := string(b)
	return &s
}

func decodeFaces(s *string) []int {
	if s == nil || *s == "" {
		return nil
	}
	var f []int
	if json.Unmarshal([]byte(*s), &f) != nil {
		return nil
	}
	return f
}
