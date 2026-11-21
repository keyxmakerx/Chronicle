// Package hexsketch reads the line-by-line text a hex map is painted in, for
// AI Import's hex-map record. It is pure: no I/O and no plugin imports, so the
// maps plugin's rules are carried here as constants and pinned against it by a
// test in the maps package. A caller turns the parts and cells it returns into
// whatever it stores or draws.
//
// Parts keep the order lines were written in, so a preview can replay them;
// cells are what the block leaves on each hex once later lines have painted
// over earlier ones.
package hexsketch

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits shared with the maps plugin. A line breaking one is reported at
// review instead of failing a whole batch of hexes at commit.
const (
	MinSide       = 4
	MaxSide       = 100
	MaxRadius     = 30
	MaxNameRunes  = 120
	MaxNotesRunes = 2000
)

// Lands are the kinds of land a hex can be painted with, in the Paint panel's
// order. The maps plugin's terrain "road" is not one: roads are lines.
var Lands = []string{
	"plains", "forest", "hills", "mountain", "water", "swamp", "desert", "snow", "town",
	"jungle", "farmland", "scrub", "badlands", "waste", "volcanic", "coast", "deep", "ice", "cavern",
}

// Lines are the words that draw a line through hexes.
var Lines = []string{"road", "river", "trail"}

// landAliases are the spellings the Paint panel shows, and a few near ones,
// keyed with spaces and hyphens removed.
var landAliases = map[string]string{
	"mountains": "mountain", "scrubland": "scrub", "wasteland": "waste", "deepsea": "deep",
	"caverns": "cavern", "cave": "cavern", "sea": "water", "lake": "water",
	"city": "town", "village": "town", "places": "town", "plain": "plains", "hill": "hills",
}

// Hex is a hex by offset coordinates: 0,0 is top-left, odd rows sit half a hex
// to the right.
type Hex struct {
	Col int `json:"col"`
	Row int `json:"row"`
}

// Part kinds.
const (
	PartLand  = "land"
	PartLine  = "line"
	PartPlace = "place"
)

// Part is one line of the body as it was understood. Hexes are in drawing
// order for a line (consecutive hexes are joined) and row by row otherwise.
// Word is the land kind or the line word; Name is the optional label of a land
// or line, which is for review only, or the name of a place.
type Part struct {
	Kind  string `json:"kind"`
	Word  string `json:"word,omitempty"`
	Name  string `json:"name,omitempty"`
	Notes string `json:"notes,omitempty"`
	Line  int    `json:"line"`
	Hexes []Hex  `json:"hexes"`
}

// Cell is what a block leaves on one hex. A nil line mask means no such line;
// a pointer to 0 is the line on the hex with no exits. Name and Notes come
// only from a place.
type Cell struct {
	Col     int    `json:"col"`
	Row     int    `json:"row"`
	Terrain string `json:"terrain,omitempty"`
	Road    *int   `json:"road,omitempty"`
	River   *int   `json:"river,omitempty"`
	Trail   *int   `json:"trail,omitempty"`
	Name    string `json:"name,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// Problem is a line that could not be read, in plain words.
type Problem struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

func (p Problem) String() string { return fmt.Sprintf("Line %d: %s", p.Line, p.Text) }

// Result is a parsed body. Cells are ordered by row, then column.
type Result struct {
	Cols     int       `json:"cols"`
	Rows     int       `json:"rows"`
	Parts    []Part    `json:"parts"`
	Cells    []Cell    `json:"cells"`
	Problems []Problem `json:"problems,omitempty"`
}

var (
	sizeRe  = regexp.MustCompile(`^\s*(\d+)\s*(?:x|X|×|by)\s*(\d+)\s*$`)
	quoteRe = regexp.MustCompile(`"([^"]*)"|“([^”]*)”`)
	commaRe = regexp.MustCompile(`\s*,\s*`)
	pointRe = regexp.MustCompile(`^(-?\d+),(-?\d+)$`)
	placeRe = regexp.MustCompile(`(?i)^place\s+(?:"([^"]*)"|“([^”]*)”)\s*(-?\d+\s*,\s*-?\d+)\s*(?::\s*(.*))?$`)
)

// ParseSize reads "30 x 20".
func ParseSize(s string) (cols, rows int, err error) {
	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, fmt.Errorf("size must be two numbers of hexes, across and down, like 30 x 20")
	}
	cols, _ = strconv.Atoi(m[1])
	rows, _ = strconv.Atoi(m[2])
	if cols < MinSide || cols > MaxSide || rows < MinSide || rows > MaxSide {
		return 0, 0, fmt.Errorf("size must be %d to %d hexes each way", MinSide, MaxSide)
	}
	return cols, rows, nil
}

// Land turns a spelling of a land kind into the stored kind. Case, spaces and
// hyphens do not matter ("Deep sea", "deep-sea" and "deepsea" are all deep).
func Land(word string) (string, bool) {
	k := strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(word))
	if a, ok := landAliases[k]; ok {
		return a, true
	}
	for _, l := range Lands {
		if l == k {
			return l, true
		}
	}
	return "", false
}

func isLineWord(w string) bool {
	for _, l := range Lines {
		if l == w {
			return true
		}
	}
	return false
}

// Parse reads a body written for a cols by rows map. A line it cannot read is
// reported and skipped; it never stops the rest.
func Parse(body string, cols, rows int) Result {
	res := Result{Cols: cols, Rows: rows}
	for i, raw := range strings.Split(body, "\n") {
		n := i + 1
		l, ok := cleanLine(raw)
		if !ok {
			continue
		}
		part, msg := parseLine(n, l, cols, rows)
		if msg != "" {
			res.Problems = append(res.Problems, Problem{Line: n, Text: msg})
			continue
		}
		res.Parts = append(res.Parts, part)
	}
	res.Cells = Fold(res.Parts)
	return res
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

func parseLine(n int, l string, cols, rows int) (Part, string) {
	if f := strings.Fields(l); len(f) > 0 && strings.EqualFold(f[0], "place") {
		return parsePlace(n, l, cols, rows)
	}
	name := ""
	rest := l
	if m := quoteRe.FindStringSubmatchIndex(l); m != nil {
		if m[2] >= 0 {
			name = strings.TrimSpace(l[m[2]:m[3]])
		} else {
			name = strings.TrimSpace(l[m[4]:m[5]])
		}
		rest = l[:m[0]] + " " + l[m[1]:]
	}
	toks := strings.Fields(commaRe.ReplaceAllString(rest, ","))
	if len(toks) == 0 {
		return Part{}, "there is nothing to draw on this line."
	}
	verb := strings.ToLower(toks[0])
	toks = toks[1:]
	in := bounds{cols, rows}

	switch {
	case verb == "fill":
		land, left, ok := takeLand(toks)
		if !ok {
			return Part{}, fillProblem(toks)
		}
		if len(left) > 0 {
			return Part{}, "fill takes just a kind of land, like fill water."
		}
		return Part{Kind: PartLand, Word: land, Name: name, Line: n, Hexes: in.all()}, ""
	case isLineWord(verb):
		return parseLinePart(n, verb, name, toks, in)
	}

	land, toks, ok := takeLand(append([]string{verb}, toks...))
	if !ok {
		return Part{}, fmt.Sprintf("%q isn't a kind of land, a line (road, river, trail), fill or place. Land is one of %s.",
			verb, strings.Join(Lands, ", "))
	}
	hexes, msg := parseLandShape(land, toks, in)
	if msg != "" {
		return Part{}, msg
	}
	return Part{Kind: PartLand, Word: land, Name: name, Line: n, Hexes: hexes}, ""
}

// takeLand reads a land kind from the front of toks, trying two words first so
// "deep sea" is read whole.
func takeLand(toks []string) (land string, rest []string, ok bool) {
	if len(toks) >= 2 {
		if l, ok := Land(toks[0] + toks[1]); ok {
			return l, toks[2:], true
		}
	}
	if len(toks) >= 1 {
		if l, ok := Land(toks[0]); ok {
			return l, toks[1:], true
		}
	}
	return "", toks, false
}

func fillProblem(toks []string) string {
	if len(toks) > 0 && isLineWord(strings.ToLower(toks[0])) {
		return lineAsLand(strings.ToLower(toks[0]))
	}
	return "fill needs a kind of land, like fill water."
}

func lineAsLand(word string) string {
	return fmt.Sprintf("%s is a line, not land: write %s c,r c,r to draw it through hexes.", word, word)
}

// bounds is the map's size, for clipping and checking points.
type bounds struct{ cols, rows int }

func (b bounds) has(h Hex) bool { return h.Col >= 0 && h.Row >= 0 && h.Col < b.cols && h.Row < b.rows }

func (b bounds) all() []Hex {
	out := make([]Hex, 0, b.cols*b.rows)
	for r := 0; r < b.rows; r++ {
		for c := 0; c < b.cols; c++ {
			out = append(out, Hex{c, r})
		}
	}
	return out
}

func (b bounds) String() string { return fmt.Sprintf("%d by %d map", b.cols, b.rows) }

// readPoints reads c,r points, skipping the word "to". The word it could not
// read is returned so the message can name it.
func readPoints(toks []string) (pts []Hex, hadTo bool, bad string) {
	for _, t := range toks {
		if strings.EqualFold(t, "to") {
			hadTo = true
			continue
		}
		m := pointRe.FindStringSubmatch(t)
		if m == nil {
			return nil, hadTo, t
		}
		c, e1 := strconv.Atoi(m[1])
		r, e2 := strconv.Atoi(m[2])
		if e1 != nil || e2 != nil {
			return nil, hadTo, t
		}
		pts = append(pts, Hex{c, r})
	}
	return pts, hadTo, ""
}

func unreadable(tok string) string {
	return fmt.Sprintf("I could not read %q; points are written c,r, like 4,7.", tok)
}

func parseLandShape(land string, toks []string, in bounds) ([]Hex, string) {
	if len(toks) == 0 {
		return nil, fmt.Sprintf("%s needs a hex, like %s 4,7.", land, land)
	}
	switch strings.ToLower(toks[0]) {
	case "around":
		return parseAround(toks[1:], in)
	case "along":
		pts, _, bad := readPoints(toks[1:])
		if bad != "" {
			return nil, unreadable(bad)
		}
		if len(pts) < 2 {
			return nil, "along needs at least two points, like along 2,3 8,3 12,6."
		}
		for _, p := range pts {
			if !in.has(p) {
				return nil, outside(p, in)
			}
		}
		var hexes []Hex
		for _, h := range runThrough(pts) {
			if in.has(h) {
				hexes = append(hexes, h)
			}
		}
		return hexes, ""
	}
	pts, hadTo, bad := readPoints(toks)
	if bad != "" {
		return nil, unreadable(bad)
	}
	switch {
	case len(pts) == 1 && !hadTo:
		if !in.has(pts[0]) {
			return nil, outside(pts[0], in)
		}
		return pts[:1], ""
	case len(pts) == 2 && hadTo:
		hexes := box(pts[0], pts[1], in)
		if len(hexes) == 0 {
			return nil, fmt.Sprintf("%d,%d to %d,%d is all outside the %s.", pts[0].Col, pts[0].Row, pts[1].Col, pts[1].Row, in)
		}
		return hexes, ""
	case len(pts) == 2:
		return nil, fmt.Sprintf("write a box as %s %d,%d to %d,%d, or use along for a run of hexes.", land, pts[0].Col, pts[0].Row, pts[1].Col, pts[1].Row)
	default:
		return nil, "give one hex, a box (c,r to c,r), around c,r radius n, or along with several points."
	}
}

func outside(p Hex, in bounds) string {
	return fmt.Sprintf("%d,%d is outside the %s.", p.Col, p.Row, in)
}

func parseAround(toks []string, in bounds) ([]Hex, string) {
	const usage = "around needs a hex and a radius, like around 10,8 radius 3."
	if len(toks) != 3 || !strings.EqualFold(toks[1], "radius") {
		return nil, usage
	}
	pts, _, bad := readPoints(toks[:1])
	if bad != "" {
		return nil, unreadable(bad)
	}
	rad, err := strconv.Atoi(toks[2])
	if err != nil || rad < 0 {
		return nil, usage
	}
	if rad > MaxRadius {
		return nil, fmt.Sprintf("a radius of %d is too wide; the most is %d.", rad, MaxRadius)
	}
	c := toCube(pts[0])
	var hexes []Hex
	for x := -rad; x <= rad; x++ {
		for y := max(-rad, -x-rad); y <= min(rad, -x+rad); y++ {
			h := fromCube(cube{c.x + x, c.y + y, c.z - x - y})
			if in.has(h) {
				hexes = append(hexes, h)
			}
		}
	}
	if len(hexes) == 0 {
		return nil, fmt.Sprintf("that circle is all outside the %s.", in)
	}
	sortHexes(hexes)
	return hexes, ""
}

// box is every hex whose column and row lie between the two corners, clipped
// to the map.
func box(a, b Hex, in bounds) []Hex {
	c0, c1 := max(0, min(a.Col, b.Col)), min(in.cols-1, max(a.Col, b.Col))
	r0, r1 := max(0, min(a.Row, b.Row)), min(in.rows-1, max(a.Row, b.Row))
	var out []Hex
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			out = append(out, Hex{c, r})
		}
	}
	return out
}

func parseLinePart(n int, word, name string, toks []string, in bounds) (Part, string) {
	if len(toks) > 0 {
		switch strings.ToLower(toks[0]) {
		case "around", "along", "radius":
			return Part{}, lineAsLand(word)
		}
	}
	pts, _, bad := readPoints(toks)
	if bad != "" {
		return Part{}, unreadable(bad)
	}
	if len(pts) == 0 {
		return Part{}, fmt.Sprintf("a %s needs points, like %s 2,3 8,3 12,6.", word, word)
	}
	for _, p := range pts {
		if !in.has(p) {
			return Part{}, outside(p, in) + " The line is left out."
		}
	}
	var hexes []Hex
	for _, h := range runThrough(pts) {
		if in.has(h) {
			hexes = append(hexes, h)
		}
	}
	return Part{Kind: PartLine, Word: word, Name: name, Line: n, Hexes: hexes}, ""
}

func parsePlace(n int, l string, cols, rows int) (Part, string) {
	m := placeRe.FindStringSubmatch(l)
	if m == nil {
		if quoteRe.MatchString(l) {
			return Part{}, `a place needs one hex after its name, like place "Oakhold" 12,7: a market town.`
		}
		return Part{}, `a place needs its name in quotes, like place "Oakhold" 12,7.`
	}
	name := strings.TrimSpace(m[1] + m[2])
	notes := strings.TrimSpace(m[4])
	if name == "" {
		return Part{}, "a place needs a name."
	}
	if msg := checkText("the name", name, MaxNameRunes); msg != "" {
		return Part{}, msg
	}
	if msg := checkText("the notes", notes, MaxNotesRunes); msg != "" {
		return Part{}, msg
	}
	pts, _, bad := readPoints([]string{commaRe.ReplaceAllString(strings.TrimSpace(m[3]), ",")})
	if bad != "" {
		return Part{}, unreadable(bad)
	}
	in := bounds{cols, rows}
	if !in.has(pts[0]) {
		return Part{}, outside(pts[0], in) + " The place is left out."
	}
	return Part{Kind: PartPlace, Name: name, Notes: notes, Line: n, Hexes: pts[:1]}, ""
}

// checkText mirrors what a hex's name and notes may hold, so a bad one is a
// review warning rather than a failed batch at commit.
func checkText(what, s string, maxRunes int) string {
	if !utf8.ValidString(s) {
		return what + " is not valid text."
	}
	if utf8.RuneCountInString(s) > maxRunes {
		return fmt.Sprintf("%s is too long; the most is %d characters.", what, maxRunes)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return what + " has characters that cannot be used."
		}
	}
	return ""
}

// Fold applies parts in order, later ones painting over earlier ones. Land
// replaces land, a place replaces name and notes, and lines add their exits to
// whatever the hex already carries, so two roads that cross both stay. Hexes
// that neighbour each other in a line's list are joined; others are not.
func Fold(parts []Part) []Cell {
	byHex := map[Hex]*Cell{}
	get := func(h Hex) *Cell {
		c, ok := byHex[h]
		if !ok {
			c = &Cell{Col: h.Col, Row: h.Row}
			byHex[h] = c
		}
		return c
	}
	for _, p := range parts {
		switch p.Kind {
		case PartLand:
			for _, h := range p.Hexes {
				get(h).Terrain = p.Word
			}
		case PartPlace:
			for _, h := range p.Hexes {
				c := get(h)
				c.Name, c.Notes = p.Name, p.Notes
			}
		case PartLine:
			for i, h := range p.Hexes {
				addExits(lineMask(get(h), p.Word), 0)
				if i == 0 {
					continue
				}
				prev := p.Hexes[i-1]
				if side := sideTo(prev, h); side >= 0 {
					addExits(lineMask(get(prev), p.Word), 1<<side)
					addExits(lineMask(get(h), p.Word), 1<<((side+3)%6))
				}
			}
		}
	}
	out := make([]Cell, 0, len(byHex))
	for _, c := range byHex {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Row != out[j].Row {
			return out[i].Row < out[j].Row
		}
		return out[i].Col < out[j].Col
	})
	return out
}

// lineMask is the cell's mask for a line word, made (as no exits) on first use.
func lineMask(c *Cell, word string) **int {
	var p **int
	switch word {
	case "road":
		p = &c.Road
	case "river":
		p = &c.River
	default:
		p = &c.Trail
	}
	if *p == nil {
		zero := 0
		*p = &zero
	}
	return p
}

func addExits(p **int, bits int) {
	v := **p | bits
	*p = &v
}

// Neighbors is the six hexes around h, east first and turning clockwise, the
// order a line's exit bits are numbered in.
func Neighbors(h Hex) [6]Hex {
	c := toCube(h)
	var out [6]Hex
	for i, d := range cubeDirs {
		out[i] = fromCube(cube{c.x + d.x, c.y + d.y, c.z + d.z})
	}
	return out
}

func sideTo(a, b Hex) int {
	for i, n := range Neighbors(a) {
		if n == b {
			return i
		}
	}
	return -1
}

// ---- Hex maths (odd-r offset, through cube coordinates) ----

type cube struct{ x, y, z int }

var cubeDirs = [6]cube{{1, -1, 0}, {0, -1, 1}, {-1, 0, 1}, {-1, 1, 0}, {0, 1, -1}, {1, 0, -1}}

func toCube(h Hex) cube {
	x := h.Col - (h.Row-(h.Row&1))/2
	return cube{x, -x - h.Row, h.Row}
}

func fromCube(c cube) Hex { return Hex{c.x + (c.z-(c.z&1))/2, c.z} }

func distance(a, b cube) int {
	return max(abs(a.x-b.x), abs(a.y-b.y), abs(a.z-b.z))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// jsRound rounds halves up, as the viewer's Math.round does, so a run drawn
// here is the run the map page would draw.
func jsRound(v float64) float64 { return math.Floor(v + 0.5) }

// Run lists the hexes on the straight run from a to b, both included.
func Run(a, b Hex) []Hex {
	p, q := toCube(a), toCube(b)
	n := distance(p, q)
	out := make([]Hex, 0, n+1)
	for i := 0; i <= n; i++ {
		t := 0.0
		if n > 0 {
			t = float64(i) / float64(n)
		}
		x := float64(p.x) + float64(q.x-p.x)*t
		y := float64(p.y) + float64(q.y-p.y)*t
		z := float64(p.z) + float64(q.z-p.z)*t
		rx, ry, rz := jsRound(x), jsRound(y), jsRound(z)
		dx, dy, dz := math.Abs(rx-x), math.Abs(ry-y), math.Abs(rz-z)
		if dx > dy && dx > dz {
			rx = -ry - rz
		} else if dy <= dz {
			rz = -rx - ry
		}
		out = append(out, fromCube(cube{int(rx), int(-rx - rz), int(rz)}))
	}
	return out
}

// runThrough joins the straight runs between consecutive points into one path;
// the end of a run is not repeated as the start of the next.
func runThrough(pts []Hex) []Hex {
	path := []Hex{pts[0]}
	for i := 1; i < len(pts); i++ {
		path = append(path, Run(pts[i-1], pts[i])[1:]...)
	}
	return path
}

func sortHexes(hs []Hex) {
	sort.Slice(hs, func(i, j int) bool {
		if hs[i].Row != hs[j].Row {
			return hs[i].Row < hs[j].Row
		}
		return hs[i].Col < hs[j].Col
	})
}
