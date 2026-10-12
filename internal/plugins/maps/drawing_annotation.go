package maps

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The four annotation drawings. They share every rule of an ordinary drawing
// (visibility, layers, who may draw or delete, shadow and fog hiding, sync)
// and add their own shape and content limits below, checked on every write so
// a partial update cannot leave one the viewer cannot draw.
const (
	// DrawingTypeArrow is a line from points[0] (tail) to points[1] (tip); the
	// viewer sizes the head from stroke_width.
	DrawingTypeArrow = "arrow"
	// DrawingTypeHighlight is a wide freehand stroke drawn translucent and
	// multiplied over the map; fill_alpha is its opacity.
	DrawingTypeHighlight = "highlight"
	// DrawingTypeStep is a numbered disc at points[0]; text_content holds the
	// number as decimal digits, stroke_color its fill.
	DrawingTypeStep = "step"
	// DrawingTypeCallout is a speech bubble whose tail points at points[0];
	// text_content is its text.
	DrawingTypeCallout = "callout"
)

// The four diagram boxes. Together with arrow (a line with a head) and callout
// (a speech bubble) they are the vocabulary of the Shapes tool, and of the AI
// map sketch that will create the same records. A box is two opposite corners
// in map percent, stored top-left first; text_content is its optional name
// (a wall has none). What a kind looks like is the viewer's decision, so it
// follows the light and dark themes: stroke_color and fill_color are accepted
// and checked like every annotation's but do not recolour these kinds.
const (
	// DrawingTypeRoom is a walled space, drawn solid.
	DrawingTypeRoom = "room"
	// DrawingTypeArea is a zone with no walls, drawn dashed and tinted.
	DrawingTypeArea = "area"
	// DrawingTypeWater is a lake, river or sea, drawn tinted.
	DrawingTypeWater = "water"
	// DrawingTypeWall is a thick bar: a wall, fence or bridge.
	DrawingTypeWall = "wall"
)

// minBoxExtent is the smallest width or height, in map percent, a box may
// have. It keeps a shape big enough to grab and select, and still allows a
// wall one grid cell thick or less.
const minBoxExtent = 0.5

const (
	// MaxDrawingTextRunes bounds the text of a label or speech bubble. The
	// column is TEXT, so without a bound one drawing could carry 64 KB that
	// every viewer downloads.
	MaxDrawingTextRunes = 500
	// MaxStepNumber caps a numbered step; three digits still fit the disc.
	MaxStepNumber = 999
	// maxAnnotationStroke bounds stroke_width for arrows and highlighters, so
	// an arrowhead or highlighter cannot cover the whole map.
	maxAnnotationStroke = 60
	// minAnnotationFont and maxAnnotationFont bound font_size for steps and
	// bubbles; nil means the viewer's default.
	minAnnotationFont = 8
	maxAnnotationFont = 48
	// The highlighter's opacity range, and what an unset one becomes.
	minHighlightAlpha     = 0.1
	maxHighlightAlpha     = 0.7
	defaultHighlightAlpha = 0.35
)

// hexColorPattern is the only colour form the viewer writes. Annotation
// colours are written into SVG and style attributes, so anything else is
// refused rather than escaped into something the browser might still parse.
var hexColorPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// isAnnotationType reports whether t is one of the four annotation drawings.
func isAnnotationType(t string) bool {
	switch t {
	case DrawingTypeArrow, DrawingTypeHighlight, DrawingTypeStep, DrawingTypeCallout:
		return true
	}
	return false
}

// isBoxShapeType reports whether t is one of the four diagram boxes.
func isBoxShapeType(t string) bool {
	switch t {
	case DrawingTypeRoom, DrawingTypeArea, DrawingTypeWater, DrawingTypeWall:
		return true
	}
	return false
}

// validateDrawingText applies the shared text bound to a label or bubble:
// trimmed, not empty, at most MaxDrawingTextRunes, and free of control
// characters other than line breaks and tabs (a bubble wraps its text and
// may hold several lines).
func validateDrawingText(s *string) (*string, error) {
	if s == nil {
		return nil, apperror.NewBadRequest("text is required")
	}
	t := strings.TrimSpace(normalizeNewlines(*s))
	if t == "" {
		return nil, apperror.NewBadRequest("text is required")
	}
	if !utf8.ValidString(t) {
		return nil, apperror.NewBadRequest("text is not valid UTF-8")
	}
	if utf8.RuneCountInString(t) > MaxDrawingTextRunes {
		return nil, apperror.NewBadRequest("text is too long (maximum " + strconv.Itoa(MaxDrawingTextRunes) + " characters)")
	}
	for _, r := range t {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return nil, apperror.NewBadRequest("text contains a control character")
		}
	}
	return &t, nil
}

// normalizeNewlines turns Windows and old Mac line endings into \n, so text
// pasted from anywhere is stored one way.
func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// cleanImportedLabel fits an imported label's text to the current rules
// without refusing it: line endings normalised, other control characters
// dropped, trimmed and cut to MaxDrawingTextRunes. Empty text stays as it was
// stored, since an old label may have none.
func cleanImportedLabel(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, strings.ToValidUTF8(normalizeNewlines(*s), ""))
	t = strings.TrimSpace(t)
	if utf8.RuneCountInString(t) > MaxDrawingTextRunes {
		t = strings.TrimSpace(string([]rune(t)[:MaxDrawingTextRunes]))
	}
	return &t
}

// parseStepNumber reads a step's number from text_content: decimal digits
// only, 1..MaxStepNumber.
func parseStepNumber(s *string) (int, error) {
	if s == nil {
		return 0, apperror.NewBadRequest("a numbered step needs a number")
	}
	t := strings.TrimSpace(*s)
	if t == "" || len(t) > 3 {
		return 0, apperror.NewBadRequest("a step number must be 1 to " + strconv.Itoa(MaxStepNumber))
	}
	for _, r := range t {
		if r < '0' || r > '9' {
			return 0, apperror.NewBadRequest("a step number must be 1 to " + strconv.Itoa(MaxStepNumber))
		}
	}
	n, err := strconv.Atoi(t)
	if err != nil || n < 1 || n > MaxStepNumber {
		return 0, apperror.NewBadRequest("a step number must be 1 to " + strconv.Itoa(MaxStepNumber))
	}
	return n, nil
}

// annotationPoints parses points for an annotation and checks each lies on
// the map (0..100, as the viewer clamps them).
func annotationPoints(raw json.RawMessage) ([]pointXY, error) {
	pts, ok := parsePoints(raw)
	if !ok {
		return nil, apperror.NewBadRequest("points must be a list of {x, y}")
	}
	for _, p := range pts {
		if p.X < 0 || p.X > 100 || p.Y < 0 || p.Y > 100 {
			return nil, apperror.NewBadRequest("points must lie on the map (0 to 100)")
		}
	}
	return pts, nil
}

// validateAnnotation checks, and normalises in place, a whole annotation
// drawing as it will be stored. It runs on create and on the merged row of an
// update.
func validateAnnotation(d *Drawing) error {
	if !hexColorPattern.MatchString(d.StrokeColor) {
		return apperror.NewBadRequest("colour must be a hex colour like #2563eb")
	}
	if d.FillColor != nil && !hexColorPattern.MatchString(*d.FillColor) {
		return apperror.NewBadRequest("fill colour must be a hex colour like #2563eb")
	}
	if d.Rotation != 0 {
		return apperror.NewBadRequest("this drawing cannot be turned")
	}
	if math.IsNaN(d.StrokeWidth) || d.StrokeWidth <= 0 || d.StrokeWidth > maxAnnotationStroke {
		return apperror.NewBadRequest("line width must be above 0 and at most " + strconv.Itoa(maxAnnotationStroke))
	}
	pts, err := annotationPoints(d.Points)
	if err != nil {
		return err
	}

	switch d.DrawingType {
	case DrawingTypeArrow:
		if len(pts) != 2 {
			return apperror.NewBadRequest("an arrow needs exactly two points, tail and tip")
		}
		if pts[0] == pts[1] {
			return apperror.NewBadRequest("an arrow's tail and tip must differ")
		}
	case DrawingTypeHighlight:
		// The size of a stroke is bounded by the points ceiling every drawing
		// shares (checkPointsSize, on create and update), so only the floor is
		// checked here.
		if len(pts) < 2 {
			return apperror.NewBadRequest("a highlighter stroke needs at least two points")
		}
		if d.FillAlpha == 0 {
			d.FillAlpha = defaultHighlightAlpha
		}
		if math.IsNaN(d.FillAlpha) || d.FillAlpha < minHighlightAlpha || d.FillAlpha > maxHighlightAlpha {
			return apperror.NewBadRequest("highlighter opacity must be between 0.1 and 0.7")
		}
	case DrawingTypeStep, DrawingTypeCallout:
		if len(pts) != 1 {
			return apperror.NewBadRequest("this drawing is placed at exactly one point")
		}
	}

	// Text belongs to steps and bubbles only; the line shapes refuse it so the
	// stored row says what the viewer draws.
	switch d.DrawingType {
	case DrawingTypeStep:
		n, err := parseStepNumber(d.TextContent)
		if err != nil {
			return err
		}
		canon := strconv.Itoa(n)
		d.TextContent = &canon
	case DrawingTypeCallout:
		t, err := validateDrawingText(d.TextContent)
		if err != nil {
			return err
		}
		d.TextContent = t
	default:
		if d.TextContent != nil {
			return apperror.NewBadRequest("this drawing has no text")
		}
		if d.FontSize != nil {
			return apperror.NewBadRequest("this drawing has no text size")
		}
	}
	if d.FontSize != nil && (*d.FontSize < minAnnotationFont || *d.FontSize > maxAnnotationFont) {
		return apperror.NewBadRequest("text size must be " + strconv.Itoa(minAnnotationFont) + " to " + strconv.Itoa(maxAnnotationFont))
	}
	return nil
}

// validateBoxShape checks, and normalises in place, a diagram box as it will
// be stored. It runs on create and on the merged row of an update, so moving
// or resizing a box cannot leave one the viewer cannot select.
//
// Corners are stored top-left first whichever way the caller dragged, so every
// reader (the viewer, the sketch importer, a sync module) can take points[0]
// as the origin without sorting.
func validateBoxShape(d *Drawing) error {
	if !hexColorPattern.MatchString(d.StrokeColor) {
		return apperror.NewBadRequest("colour must be a hex colour like #2563eb")
	}
	if d.FillColor != nil && !hexColorPattern.MatchString(*d.FillColor) {
		return apperror.NewBadRequest("fill colour must be a hex colour like #2563eb")
	}
	if d.Rotation != 0 {
		return apperror.NewBadRequest("this drawing cannot be turned")
	}
	if math.IsNaN(d.StrokeWidth) || d.StrokeWidth <= 0 || d.StrokeWidth > maxAnnotationStroke {
		return apperror.NewBadRequest("line width must be above 0 and at most " + strconv.Itoa(maxAnnotationStroke))
	}
	if math.IsNaN(d.FillAlpha) || d.FillAlpha < 0 || d.FillAlpha > 1 {
		return apperror.NewBadRequest("fill opacity must be between 0 and 1")
	}
	pts, err := annotationPoints(d.Points)
	if err != nil {
		return err
	}
	if len(pts) != 2 {
		return apperror.NewBadRequest("a box needs exactly two corners")
	}
	minX, maxX := math.Min(pts[0].X, pts[1].X), math.Max(pts[0].X, pts[1].X)
	minY, maxY := math.Min(pts[0].Y, pts[1].Y), math.Max(pts[0].Y, pts[1].Y)
	if maxX-minX < minBoxExtent || maxY-minY < minBoxExtent {
		return apperror.NewBadRequest("a box must be at least " + strconv.FormatFloat(minBoxExtent, 'f', -1, 64) + " percent of the map wide and high")
	}
	normal, err := json.Marshal([]pointXY{{X: minX, Y: minY}, {X: maxX, Y: maxY}})
	if err != nil {
		return apperror.NewBadRequest("points must be a list of {x, y}")
	}
	d.Points = normal

	if d.DrawingType == DrawingTypeWall {
		if d.TextContent != nil {
			return apperror.NewBadRequest("this drawing has no text")
		}
		if d.FontSize != nil {
			return apperror.NewBadRequest("this drawing has no text size")
		}
		return nil
	}
	// A name is optional; blank means none, so clearing the field in the
	// editor and sending an empty string both read as "no name".
	if d.TextContent != nil {
		if strings.TrimSpace(*d.TextContent) == "" {
			d.TextContent = nil
		} else {
			t, err := validateDrawingText(d.TextContent)
			if err != nil {
				return err
			}
			d.TextContent = t
		}
	}
	if d.FontSize != nil && (*d.FontSize < minAnnotationFont || *d.FontSize > maxAnnotationFont) {
		return apperror.NewBadRequest("text size must be " + strconv.Itoa(minAnnotationFont) + " to " + strconv.Itoa(maxAnnotationFont))
	}
	return nil
}
