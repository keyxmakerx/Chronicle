package entities

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// play_edit.go is the owner's half of a system's play blocks: a number the
// system declares owner-editable during play (a counter or resource, such as
// a hero's Victories or Stamina) may be written by the hero's claimed owner,
// within the block's bounds. Core never names the fields; the manifest does.

// Play edit modes and kinds as stored on FieldDefinition.Play, matching the
// systems package's names.
const (
	playEditOwner    = "owner"
	playKindCounter  = "counter"
	playKindResource = "resource"
	playNumberLimit  = 1e9 // larger than any stat; keeps stored numbers exact
)

// playOwnerNumber reports whether f is a number the claimed owner may change
// during play. A GM-only field never is, whatever its block says.
func playOwnerNumber(f *FieldDefinition) bool {
	if f == nil || f.GMOnly || f.Play == nil || f.Play.Edit != playEditOwner {
		return false
	}
	return f.Play.Kind == playKindCounter || f.Play.Kind == playKindResource
}

// playNumber reads a written number. JSON numbers arrive as float64; a
// json.Number or an integer type is taken too. Text is never a number here,
// so a stat can't be typed into as words.
func playNumber(v any) (float64, bool) {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case float32:
		n = float64(x)
	case int:
		n = float64(x)
	case int64:
		n = float64(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0, false
		}
		n = f
	default:
		return 0, false
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// checkPlayNumber checks a claimed owner's value for a play number: a number
// (or null to clear it), never below the block's min and never above its
// max_field's value (see playMax) or its max. A counter or resource moves in
// whole steps, so a fraction is refused. field is the category's fields by
// key, for the max_field's own play block.
func checkPlayNumber(entity *Entity, patch map[string]any, field map[string]*FieldDefinition, f FieldDefinition, v any) error {
	if v == nil {
		return nil
	}
	n, ok := playNumber(v)
	if !ok {
		return apperror.NewBadRequest(fmt.Sprintf("%s takes a number", fieldName(f)))
	}
	if math.Abs(n) > playNumberLimit {
		return apperror.NewBadRequest(fmt.Sprintf("%s is too large", fieldName(f)))
	}
	if n != math.Trunc(n) {
		return apperror.NewBadRequest(fmt.Sprintf("%s takes a whole number", fieldName(f)))
	}
	p := f.Play
	if p.Min != nil && n < *p.Min {
		return apperror.NewBadRequest(fmt.Sprintf("%s can't go below %s", fieldName(f), playNumberText(*p.Min)))
	}
	if limit, has := playMax(entity, patch, field, p); has && n > limit {
		return apperror.NewBadRequest(fmt.Sprintf("%s can't go above %s", fieldName(f), playNumberText(limit)))
	}
	return nil
}

// playMax is a play block's upper bound: its max_field's value, else its max.
// The same write's max counts only when the owner may set that max too and
// the value passes its own check; otherwise a write the door holds back in
// part could raise or clear the bound on the number it is bounding. A
// max_field holding no number bounds nothing rather than refusing every write.
func playMax(entity *Entity, patch map[string]any, field map[string]*FieldDefinition, p *FieldPlay) (float64, bool) {
	if p.MaxField != "" {
		if v, set := patch[p.MaxField]; set {
			if def := field[p.MaxField]; playOwnerNumber(def) && checkPlayNumber(entity, patch, nil, *def, v) == nil {
				return playNumber(v)
			}
		}
		if entity != nil {
			if n, ok := playNumber(entity.FieldsData[p.MaxField]); ok {
				return n, true
			}
		}
		return 0, false
	}
	if p.Max != nil {
		return *p.Max, true
	}
	return 0, false
}

// playNumberText writes a number as the bell and the refusals show it.
func playNumberText(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// fieldName is a field's label for a message, its key when it has none.
func fieldName(f FieldDefinition) string {
	if l := strings.TrimSpace(f.Label); l != "" {
		return l
	}
	return capitalFirst(f.Key)
}

// playBellNumber reports whether a play number rings the Directors' bell when
// its owner changes it: the system asks for it with the block's bell.
func playBellNumber(f *FieldDefinition) bool {
	return playOwnerNumber(f) && f.Play.Bell
}
