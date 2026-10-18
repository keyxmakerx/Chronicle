package mapsketch

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The grid bounds of a sketch that becomes a new map. A page map is a fixed
// size and does not use these.
const (
	GridMin = 4
	GridMax = 100
)

// MaxLegacyText mirrors the maps plugin's bound on a drawing's text.
const MaxLegacyText = 500

// Verbs are the words a map-record line can start with, in the order the
// refusal lists them.
var Verbs = []string{"room", "area", "water", "danger", "wall", "path", "road", "label", "pin"}

var (
	gridRe   = regexp.MustCompile(`^\s*(\d+)\s*(?:x|X|×|by)\s*(\d+)\s*$`)
	pointRe  = regexp.MustCompile(`-?\d+(?:\.\d+)?\s*,\s*-?\d+(?:\.\d+)?`)
	radiusRe = regexp.MustCompile(`(?i)\bradius\s+(\d+(?:\.\d+)?)`)
	quoteRe  = regexp.MustCompile(`"([^"]*)"|“([^”]*)”`)
	toRe     = regexp.MustCompile(`(?i)\bto\b`)
	circleRe = regexp.MustCompile(`(?i)\bcircle\b`)
	pinCatRe = regexp.MustCompile(`(?i)\b(danger|treasure|quest|note|location)\b`)
	pageRe   = regexp.MustCompile(`(?i)\bpage\s+(?:"([^"]*)"|“([^”]*)”)`)
	colorRe  = regexp.MustCompile(`(?i)\bcolor\s+(#?[0-9a-z]+)`)
)

// Entry is one parsed line, in grid squares. The map-record parser fills the
// first group; the page-map parser also fills the second.
type Entry struct {
	Line     int
	Verb     string // a Verbs word
	Shape    string // box, circle, line, label or pin
	Name     string
	Points   [][2]float64
	Radius   float64
	Category string // pins only
	Note     string // pins only

	// Page-map only.
	Style string // pins: number or icon
	Icon  string // pins: an allowlisted icon name
	Page  string // pins: the linked page's name as written
	Color string // normalised colour name or #rrggbb
	DM    bool   // pins: visible to the DM only
}

// Plan is a parsed map-record sketch: the entries that can be drawn and a
// plain sentence for each line that cannot.
type Plan struct {
	W, H    int
	Entries []Entry
	Errs    []string
}

// ParseGridSize reads "40 x 26".
func ParseGridSize(s string) (w, h int, err error) {
	m := gridRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, errors.New("grid must be two numbers of squares, across and down, like 40 x 26")
	}
	w, _ = strconv.Atoi(m[1])
	h, _ = strconv.Atoi(m[2])
	if w < GridMin || w > GridMax || h < GridMin || h > GridMax {
		return 0, 0, fmt.Errorf("grid must be %d to %d squares each way", GridMin, GridMax)
	}
	return w, h, nil
}

// cleanLine trims a line and drops a list bullet, so a pasted markdown list
// parses; it reports false for lines that carry nothing.
func cleanLine(raw string) (string, bool) {
	l := strings.TrimSpace(raw)
	l = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(l, "- "), "* "))
	if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "```") {
		return "", false
	}
	return l, true
}

// ParseEntries reads a map-record sketch body. A line it cannot draw is
// reported and skipped; it never stops the rest of the map.
func ParseEntries(body string, w, h int) Plan {
	p := Plan{W: w, H: h}
	for i, raw := range strings.Split(body, "\n") {
		n := i + 1
		l, ok := cleanLine(raw)
		if !ok {
			continue
		}
		s, err := ParseEntry(n, l)
		if err == "" {
			err = CheckEntry(&s, w, h)
		}
		if err != "" {
			p.Errs = append(p.Errs, fmt.Sprintf("Line %d: %s", n, err))
			continue
		}
		p.Entries = append(p.Entries, s)
	}
	return p
}

// ParseEntry reads one map-record line, or says in plain words why it cannot.
func ParseEntry(n int, l string) (Entry, string) {
	s := Entry{Line: n}
	rest := l
	if m := quoteRe.FindStringSubmatchIndex(l); m != nil {
		if m[2] >= 0 {
			s.Name = strings.TrimSpace(l[m[2]:m[3]])
		} else {
			s.Name = strings.TrimSpace(l[m[4]:m[5]])
		}
		rest = l[:m[0]] + " " + l[m[1]:]
	}
	// A pin's note is everything after the first colon that follows its
	// name, so the note may hold commas and numbers of its own.
	if fields := strings.Fields(rest); len(fields) > 0 && strings.EqualFold(fields[0], "pin") {
		if c := strings.Index(rest, ":"); c >= 0 {
			s.Note = strings.TrimSpace(rest[c+1:])
			rest = rest[:c]
		}
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return s, "there is nothing to draw on this line."
	}
	s.Verb = strings.ToLower(fields[0])
	s.Points = readPoints(rest)
	if s.Verb == "pin" {
		if len(s.Points) < 1 {
			return s, "a pin needs a point."
		}
		s.Points = s.Points[:1]
		if s.Name == "" {
			s.Name = "Pin"
		}
		s.Category = "location"
		if m := pinCatRe.FindStringSubmatch(rest); m != nil {
			s.Category = strings.ToLower(m[1])
		}
		s.Shape = "pin"
		return s, ""
	}
	if msg, ok := classify(&s, rest); !ok {
		if msg == "" {
			msg = fmt.Sprintf("%q isn't one of %s.", fields[0], strings.Join(Verbs, ", "))
		}
		return s, msg
	}
	return s, ""
}

// readPoints finds every x,y pair in s.
func readPoints(s string) [][2]float64 {
	var pts [][2]float64
	for _, m := range pointRe.FindAllString(s, -1) {
		xy := strings.SplitN(m, ",", 2)
		x, _ := strconv.ParseFloat(strings.TrimSpace(xy[0]), 64)
		y, _ := strconv.ParseFloat(strings.TrimSpace(xy[1]), 64)
		pts = append(pts, [2]float64{x, y})
	}
	return pts
}

// classify decides the shape of every verb but pin from its points. It
// returns ok=false with a message, or with none when the verb is not one it
// knows.
func classify(s *Entry, rest string) (msg string, ok bool) {
	switch s.Verb {
	case "room", "area", "water", "danger":
		if len(s.Points) != 2 {
			return "a " + s.Verb + " needs two corners, like 3,4 to 9,8.", false
		}
		s.Shape = "box"
	case "wall":
		switch {
		case circleRe.MatchString(rest):
			r := radiusRe.FindStringSubmatch(rest)
			if len(s.Points) != 1 || r == nil {
				return "a circle wall needs a centre and a radius, like 20,13 radius 9.", false
			}
			s.Radius, _ = strconv.ParseFloat(r[1], 64)
			s.Shape = "circle"
		case len(s.Points) == 2 && toRe.MatchString(rest):
			s.Shape = "box"
			// A wall with no thickness is a line, not a box the map
			// would refuse as too thin.
			if s.Points[0][0] == s.Points[1][0] || s.Points[0][1] == s.Points[1][1] {
				s.Shape = "line"
			}
		case len(s.Points) >= 2:
			s.Shape = "line"
		default:
			return "a wall needs two corners, like 3,4 to 9,4, or a line of points.", false
		}
	case "path", "road":
		if len(s.Points) < 2 {
			return "a " + s.Verb + " needs at least two points.", false
		}
		s.Shape = "line"
	case "label":
		if len(s.Points) < 1 {
			return "a label needs a point.", false
		}
		if s.Name == "" {
			return "a label needs its words in quotes.", false
		}
		s.Points = s.Points[:1]
		s.Shape = "label"
	default:
		return "", false
	}
	return "", true
}

// CheckEntry refuses what the map would refuse: points off the grid, boxes
// with no width or height, names longer than a drawing may hold.
func CheckEntry(s *Entry, w, h int) string {
	for _, pt := range s.Points {
		if pt[0] < 0 || pt[0] > float64(w) || pt[1] < 0 || pt[1] > float64(h) {
			return fmt.Sprintf("%s,%s is off the %d by %d grid.", fmtNum(pt[0]), fmtNum(pt[1]), w, h)
		}
	}
	if s.Shape == "box" && (s.Points[0][0] == s.Points[1][0] || s.Points[0][1] == s.Points[1][1]) {
		return "a " + s.Verb + " needs some width and height; its corners are in line."
	}
	if s.Shape == "circle" {
		c, r := s.Points[0], s.Radius
		if r <= 0 {
			return "a circle wall needs a radius above 0."
		}
		if c[0]-r < 0 || c[0]+r > float64(w) || c[1]-r < 0 || c[1]+r > float64(h) {
			return fmt.Sprintf("a circle of radius %s around %s,%s runs off the %d by %d grid.", fmtNum(r), fmtNum(c[0]), fmtNum(c[1]), w, h)
		}
	}
	if len([]rune(s.Name)) > MaxLegacyText {
		return fmt.Sprintf("the name is longer than %d characters.", MaxLegacyText)
	}
	return ""
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
