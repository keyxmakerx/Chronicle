package mapsketch

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The page map's fixed canvas and bounds. The grid is the same on every page
// so a map the AI writes as text always lands where the editor draws it.
const (
	GridW = 40
	GridH = 26

	MaxShapes = 200
	MaxPins   = 200
	MaxName   = 80
	MaxNote   = 500
	// MaxPageName matches the longest name a page may have, so any page can
	// be linked by name.
	MaxPageName = 200
	// MaxPoints bounds one polyline; a circle wall uses circleSegments+1.
	MaxPoints = 100

	circleSegments = 24
)

// Background values.
const (
	BackgroundNone = "none"
	BackgroundDots = "dots"
	BackgroundGrid = "grid"
	BackgroundHex  = "hex"
)

// Pin styles.
const (
	StyleNumber = "number"
	StyleIcon   = "icon"
)

// Icons is the allowlist of marker icons (Font Awesome names).
var Icons = []string{"location-dot", "house", "chess-rook", "anchor", "church", "door-open", "skull", "gem", "star", "eye", "fire", "crown"}

// categoryIcons maps the older category words to icons so a line written for
// a map record still draws sensibly.
var categoryIcons = map[string]string{
	"danger": "skull", "treasure": "gem", "quest": "star", "note": "eye", "location": "location-dot",
}

// Shape kinds. The four box kinds use x,y,w,h; wall and path use pts; a label
// uses x,y and its name.
const (
	ShapeRoom   = "room"
	ShapeArea   = "area"
	ShapeWater  = "water"
	ShapeDanger = "danger"
	ShapeWall   = "wall"
	ShapePath   = "path"
	ShapeLabel  = "label"
)

// IsBoxShape reports whether t is drawn from x,y,w,h.
func IsBoxShape(t string) bool {
	switch t {
	case ShapeRoom, ShapeArea, ShapeWater, ShapeDanger:
		return true
	}
	return false
}

// IsShapeKind reports whether t is any shape kind.
func IsShapeKind(t string) bool {
	return IsBoxShape(t) || t == ShapeWall || t == ShapePath || t == ShapeLabel
}

// ValidBackground reports whether b is a background the map can draw.
func ValidBackground(b string) bool {
	switch b {
	case BackgroundNone, BackgroundDots, BackgroundGrid, BackgroundHex:
		return true
	}
	return false
}

// ValidIcon reports whether name is an allowlisted icon.
func ValidIcon(name string) bool {
	for _, i := range Icons {
		if i == name {
			return true
		}
	}
	return false
}

// Grid is the canvas size in squares.
type Grid struct {
	W int `json:"w"`
	H int `json:"h"`
}

// Shape is one drawn thing. Boxes carry X,Y,W,H in squares; walls and paths
// carry Pts (a closed wall repeats its first point last); a label carries X,Y
// and its words in Name. X and Y are always written, so a box at the corner
// reads 0, not absent; the fields a kind does not use are left out.
type Shape struct {
	ID    string       `json:"id"`
	T     string       `json:"t"`
	X     float64      `json:"x"`
	Y     float64      `json:"y"`
	W     float64      `json:"w,omitempty"`
	H     float64      `json:"h,omitempty"`
	Pts   [][2]float64 `json:"pts,omitempty"`
	Name  string       `json:"name,omitempty"`
	Color string       `json:"color,omitempty"`
}

// Pin is one marker. Page is the linked page's name as the text wrote it;
// EntityID is filled by whoever resolves names, and Text is the note shown
// when there is no page (or the viewer cannot see it).
type Pin struct {
	ID       string  `json:"id"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Name     string  `json:"name,omitempty"`
	Style    string  `json:"style"`
	Icon     string  `json:"icon,omitempty"`
	EntityID string  `json:"entityId,omitempty"`
	Page     string  `json:"page,omitempty"`
	Text     string  `json:"text,omitempty"`
	DM       bool    `json:"dm,omitempty"`
	Color    string  `json:"color,omitempty"`
	// EntityName and EntityURL are read-only: the server fills them when it
	// serves a linked page the viewer can see, and never stores them.
	EntityName string `json:"entityName,omitempty"`
	EntityURL  string `json:"entityUrl,omitempty"`
}

// Doc is a page map.
type Doc struct {
	Grid       Grid    `json:"grid"`
	Background string  `json:"background"`
	Shapes     []Shape `json:"shapes"`
	Pins       []Pin   `json:"pins"`
}

// NewDoc is the empty page map.
func NewDoc() Doc {
	return Doc{Grid: Grid{W: GridW, H: GridH}, Background: BackgroundNone, Shapes: []Shape{}, Pins: []Pin{}}
}

// Result is what Parse made of a text: the document, counts of what was
// kept, and a plain sentence ("Line N: ...") for every line left out.
type Result struct {
	Doc        Doc
	Markers    int
	Shapes     int
	Background string
	Problems   []string
}

// Parse reads the page-map line grammar (see Grammar). A bad line is
// reported and left out; the rest is kept.
func Parse(text string) Result {
	doc := NewDoc()
	var problems []string
	fail := func(n int, msg string) { problems = append(problems, fmt.Sprintf("Line %d: %s", n, msg)) }
	for i, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		n := i + 1
		l, ok := cleanLine(raw)
		if !ok {
			continue
		}
		fields := strings.Fields(l)
		switch strings.ToLower(fields[0]) {
		case "grid":
			// The page map's grid is fixed; the line is accepted so a text
			// written for a map record still parses.
			continue
		case "background":
			if len(fields) != 2 || !ValidBackground(strings.ToLower(fields[1])) {
				fail(n, "background must be none, dots, grid or hex.")
				continue
			}
			doc.Background = strings.ToLower(fields[1])
			continue
		}
		e, msg := parsePageLine(n, l)
		if msg == "" {
			msg = checkPageEntry(&e)
		}
		if msg != "" {
			fail(n, msg)
			continue
		}
		if e.Shape == "pin" {
			if len(doc.Pins) >= MaxPins {
				fail(n, fmt.Sprintf("the map already has %d markers; this one is left out.", MaxPins))
				continue
			}
			doc.Pins = append(doc.Pins, Pin{
				ID: "p" + strconv.Itoa(len(doc.Pins)+1), X: e.Points[0][0], Y: e.Points[0][1],
				Name: e.Name, Style: e.Style, Icon: e.Icon, Page: e.Page, Text: e.Note, DM: e.DM, Color: e.Color,
			})
			continue
		}
		if len(doc.Shapes) >= MaxShapes {
			fail(n, fmt.Sprintf("the map already has %d shapes; this one is left out.", MaxShapes))
			continue
		}
		sh := entryToShape(e)
		sh.ID = "s" + strconv.Itoa(len(doc.Shapes)+1)
		doc.Shapes = append(doc.Shapes, sh)
	}
	return Result{Doc: doc, Markers: len(doc.Pins), Shapes: len(doc.Shapes), Background: doc.Background, Problems: problems}
}

var pageVerbs = []string{"background", "grid", "room", "area", "water", "danger", "wall", "path", "road", "label", "pin"}

// parsePageLine reads one shape or pin line with the page-map words added.
func parsePageLine(n int, l string) (Entry, string) {
	s := Entry{Line: n}
	head := l
	first := strings.ToLower(strings.Fields(l)[0])
	isPin := first == "pin"
	if isPin {
		// The note starts at the first colon outside quotes, so a name or a
		// page name may hold one.
		if c := colonOutsideQuotes(l); c >= 0 {
			s.Note = strings.TrimSpace(l[c+1:])
			head = l[:c]
		}
		if m := pageRe.FindStringSubmatchIndex(head); m != nil {
			if m[2] >= 0 {
				s.Page = strings.TrimSpace(head[m[2]:m[3]])
			} else {
				s.Page = strings.TrimSpace(head[m[4]:m[5]])
			}
			head = head[:m[0]] + " " + head[m[1]:]
		}
	}
	if m := quoteRe.FindStringSubmatchIndex(head); m != nil {
		if m[2] >= 0 {
			s.Name = strings.TrimSpace(head[m[2]:m[3]])
		} else {
			s.Name = strings.TrimSpace(head[m[4]:m[5]])
		}
		head = head[:m[0]] + " " + head[m[1]:]
	}
	if m := colorRe.FindStringSubmatchIndex(head); m != nil {
		c, problem := NormalizeColor(head[m[2]:m[3]])
		if problem != "" {
			return s, problem
		}
		s.Color = c
		head = head[:m[0]] + " " + head[m[1]:]
	}
	fields := strings.Fields(head)
	if len(fields) == 0 {
		return s, "there is nothing to draw on this line."
	}
	s.Verb = strings.ToLower(fields[0])
	s.Points = readPoints(head)
	if isPin {
		if len(s.Points) < 1 {
			return s, "a pin needs a point."
		}
		s.Points = s.Points[:1]
		s.Shape = "pin"
		return s, readPinWords(&s, fields[1:], head)
	}
	if msg, ok := classify(&s, head); !ok {
		if msg == "" {
			msg = fmt.Sprintf("%q isn't one of %s.", fields[0], strings.Join(pageVerbs, ", "))
		}
		return s, msg
	}
	return s, ""
}

// readPinWords reads number, icon <name>, dm and the older category words.
func readPinWords(s *Entry, words []string, head string) string {
	for i := 0; i < len(words); i++ {
		switch strings.ToLower(words[i]) {
		case "number":
			s.Style = StyleNumber
		case "dm":
			s.DM = true
		case "icon":
			if i+1 >= len(words) {
				return "icon needs a name, like icon skull."
			}
			i++
			name := strings.ToLower(words[i])
			if !ValidIcon(name) {
				return fmt.Sprintf("icon %q isn't one of %s.", words[i], strings.Join(Icons, ", "))
			}
			s.Style, s.Icon = StyleIcon, name
		}
	}
	if s.Style == "" {
		s.Style, s.Icon = StyleIcon, "location-dot"
		if m := pinCatRe.FindStringSubmatch(head); m != nil {
			s.Icon = categoryIcons[strings.ToLower(m[1])]
		}
	}
	return ""
}

// checkPageEntry bounds one parsed line to the page map's canvas and limits.
func checkPageEntry(e *Entry) string {
	if msg := CheckEntry(e, GridW, GridH); msg != "" {
		return msg
	}
	if len([]rune(e.Name)) > MaxName {
		return fmt.Sprintf("the name is longer than %d characters.", MaxName)
	}
	if len([]rune(e.Page)) > MaxPageName {
		return fmt.Sprintf("the page name is longer than %d characters.", MaxPageName)
	}
	if len([]rune(e.Note)) > MaxNote {
		return fmt.Sprintf("the note is longer than %d characters.", MaxNote)
	}
	if len(e.Points) > MaxPoints {
		return fmt.Sprintf("a line holds at most %d points.", MaxPoints)
	}
	return ""
}

// entryToShape turns a parsed line into the stored shape. A wall given as a
// box or a circle is stored as a closed polyline, so the editor has one wall
// kind to draw and move.
func entryToShape(e Entry) Shape {
	sh := Shape{Name: e.Name, Color: e.Color}
	switch e.Shape {
	case "box":
		a, b := e.Points[0], e.Points[1]
		x1, x2 := math.Min(a[0], b[0]), math.Max(a[0], b[0])
		y1, y2 := math.Min(a[1], b[1]), math.Max(a[1], b[1])
		if e.Verb == "wall" {
			sh.T = ShapeWall
			sh.Pts = [][2]float64{{x1, y1}, {x2, y1}, {x2, y2}, {x1, y2}, {x1, y1}}
			sh.Name = e.Name
			return sh
		}
		sh.T, sh.X, sh.Y, sh.W, sh.H = e.Verb, x1, y1, x2-x1, y2-y1
	case "circle":
		c := e.Points[0]
		for i := 0; i < circleSegments; i++ {
			ang := 2 * math.Pi * float64(i) / circleSegments
			sh.Pts = append(sh.Pts, [2]float64{round2(c[0] + e.Radius*math.Cos(ang)), round2(c[1] + e.Radius*math.Sin(ang))})
		}
		sh.Pts = append(sh.Pts, sh.Pts[0])
		sh.T = ShapeWall
	case "line":
		sh.T = ShapePath
		if e.Verb == "wall" {
			sh.T = ShapeWall
		}
		sh.Pts = e.Points
	case "label":
		sh.T, sh.X, sh.Y = ShapeLabel, e.Points[0][0], e.Points[0][1]
	}
	return sh
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// colonOutsideQuotes finds the first ':' that is not inside "..." or “...”.
func colonOutsideQuotes(s string) int {
	var close rune
	for i, r := range s {
		switch {
		case close != 0:
			if r == close {
				close = 0
			}
		case r == '"':
			close = '"'
		case r == '“':
			close = '”'
		case r == ':':
			return i
		}
	}
	return -1
}

// Format writes d as the line grammar, so a text a person or the AI edits
// starts from what is on the page. Names and notes are flattened to one line
// and a double quote inside a name becomes a single one, since the grammar
// has no escape; everything else round-trips through Parse.
func Format(d Doc) string {
	var b strings.Builder
	if d.Background != "" && d.Background != BackgroundNone {
		b.WriteString("background " + d.Background + "\n")
	}
	for _, s := range d.Shapes {
		b.WriteString(formatShape(s))
		b.WriteByte('\n')
	}
	for _, p := range d.Pins {
		b.WriteString(formatPin(p))
		b.WriteByte('\n')
	}
	return b.String()
}

func oneLine(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)
	return strings.TrimSpace(s)
}

func quoted(s string) string {
	return `"` + quoteSafe(s) + `"`
}

func quoteSafe(s string) string {
	return strings.NewReplacer(`"`, "'", "“", "'", "”", "'").Replace(oneLine(s))
}

// NameKey is the form two names are compared in: what Format would write,
// trimmed and in lower case, so a name read back from Format's text matches
// the name it was written from.
func NameKey(s string) string {
	return strings.ToLower(strings.TrimSpace(quoteSafe(s)))
}

func pt(x, y float64) string { return fmtNum(round2(x)) + "," + fmtNum(round2(y)) }

func formatShape(s Shape) string {
	parts := []string{s.T}
	if s.Name != "" {
		parts = append(parts, quoted(s.Name))
	}
	switch {
	case IsBoxShape(s.T):
		parts = append(parts, pt(s.X, s.Y), "to", pt(s.X+s.W, s.Y+s.H))
	case s.T == ShapeLabel:
		parts = append(parts, pt(s.X, s.Y))
	default:
		for _, p := range s.Pts {
			parts = append(parts, pt(p[0], p[1]))
		}
	}
	if s.Color != "" {
		parts = append(parts, "color", s.Color)
	}
	return strings.Join(parts, " ")
}

func formatPin(p Pin) string {
	parts := []string{"pin"}
	if p.Name != "" {
		parts = append(parts, quoted(p.Name))
	}
	parts = append(parts, pt(p.X, p.Y))
	if p.Style == StyleNumber {
		parts = append(parts, "number")
	} else if p.Icon != "" {
		parts = append(parts, "icon", p.Icon)
	}
	if p.Page != "" {
		parts = append(parts, "page", quoted(p.Page))
	}
	if p.DM {
		parts = append(parts, "dm")
	}
	if p.Color != "" {
		parts = append(parts, "color", p.Color)
	}
	line := strings.Join(parts, " ")
	if t := oneLine(p.Text); t != "" {
		line += ": " + t
	}
	return line
}
