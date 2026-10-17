package maps

import (
	"context"
	"fmt"
	"strings"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// Presentation helpers for the "Where it is" block (where_is_block.templ).

// whereBigThumbSize is the media thumbnail edge for the enlarged map; the card
// and the peek use the smaller linkThumbSize.
const whereBigThumbSize = "800"

// defaultAspect is the picture ratio assumed for a map whose size is unknown,
// the shape the cards are drawn in.
const defaultAspect = 240.0 / 168.0

// whereDescMax bounds the map description carried on a card, so a long
// description does not bloat the page; the full text is on the map's page.
const whereDescMax = 220

// whereAspect is the map picture's width over height as a CSS number, so the
// pin spot can be laid over the picture itself rather than over its frame.
func whereAspect(m *Map) string {
	if m.ImageWidth > 0 && m.ImageHeight > 0 {
		return fmt.Sprintf("%.4f", float64(m.ImageWidth)/float64(m.ImageHeight))
	}
	return fmt.Sprintf("%.4f", defaultAspect)
}

// whereFitStyle sets the picture ratio on the card's picture box.
func whereFitStyle(m *Map) templ.SafeCSS {
	return templ.SafeCSS("--ar:" + whereAspect(m) + ";")
}

// whereSpotStyle places a pin or its outline at a percent position, clamped to
// the picture.
func whereSpotStyle(x, y float64) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("left:%.2f%%;top:%.2f%%;", clampPct(x), clampPct(y)))
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// whereBigSrc is the enlarged map's picture: the player copy when the viewer's
// map carries one, else a larger signed thumbnail of the original.
func whereBigSrc(ctx context.Context, m *Map) string {
	if m == nil {
		return ""
	}
	if m.PlayerImageURL != "" {
		return m.PlayerImageURL
	}
	if m.ImageID == nil || *m.ImageID == "" {
		return ""
	}
	return layouts.MediaThumbURL(ctx, *m.ImageID, whereBigThumbSize)
}

// whereDesc is the map's description trimmed to one short passage.
func whereDesc(m *Map) string {
	if m.Description == nil {
		return ""
	}
	s := strings.Join(strings.Fields(*m.Description), " ")
	r := []rune(s)
	if len(r) <= whereDescMax {
		return s
	}
	return strings.TrimSpace(string(r[:whereDescMax])) + "..."
}

// whereCountLabel is the heading's count of maps on the ladder.
func whereCountLabel(n int) string {
	return mapCountLabel(n)
}

// wherePlayerCount is how many steps a player would see.
func wherePlayerCount(steps []WhereStep) int {
	n := 0
	for _, s := range steps {
		if !s.DMOnly {
			n++
		}
	}
	return n
}

// whereHasDMOnly reports whether any step is hidden from players.
func whereHasDMOnly(steps []WhereStep) bool {
	return wherePlayerCount(steps) != len(steps)
}

// whereNextName is the name of the map one step wider than step i, or "".
func whereNextName(steps []WhereStep, i int) string {
	if i+1 < len(steps) {
		return steps[i+1].Map.Name
	}
	return ""
}

// whereNextPage is the page of the map one step wider than step i, or "".
func whereNextPage(campaignID string, steps []WhereStep, i int) string {
	if i+1 < len(steps) {
		return mapPageURL(campaignID, steps[i+1].Map.ID)
	}
	return ""
}

// whereChildName is the name of the map one step closer than step i, or "".
func whereChildName(steps []WhereStep, i int) string {
	if i > 0 {
		return steps[i-1].Map.Name
	}
	return ""
}

// whereAriaLabel is a card's spoken name.
func whereAriaLabel(s WhereStep, first bool) string {
	var b strings.Builder
	b.WriteString(s.Map.Name)
	if first {
		b.WriteString(", this place")
	}
	if s.DMOnly {
		b.WriteString(", hidden from players")
	}
	b.WriteString(". Open larger map")
	return b.String()
}

// whereHiddenLine is the Director's note that players see a shorter ladder,
// naming the maps they are kept from.
func whereHiddenLine(steps []WhereStep) string {
	var names []string
	for _, s := range steps {
		if s.DMOnly {
			names = append(names, s.Map.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	n := len(names)
	fewer := "one step fewer"
	if n > 1 {
		fewer = fmt.Sprintf("%d steps fewer", n)
	}
	return joinNames(names) + " " + hiddenVerb(n) + " hidden from players, so they see " + fewer + "."
}

func hiddenVerb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

// joinNames joins names as "A", "A and B" or "A, B and C".
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// whereCall is one of the block's handlers in static/js/where_it_is.js, as an
// inline IIFE so it works after an HTMX swap of the page (a templ script helper
// or delegated listener would not). fn is always one of that file's fixed
// names, never input.
func whereCall(fn string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "maps_where_" + fn,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleWhereIt)window.ChronicleWhereIt.` + fn + `(el,e);` +
			`})(this,event)`,
	}
}

// whereWalkCall is whereCall for the panel's arrows, which pass a direction
// (-1 closer, 1 wider).
func whereWalkCall(dir int) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     fmt.Sprintf("maps_where_walk_%d", dir),
		Function: "",
		Call: fmt.Sprintf(`(function(el,e){`+
			`if(window.ChronicleWhereIt)window.ChronicleWhereIt.walk(el,%d);`+
			`})(this,event)`, dir),
	}
}
