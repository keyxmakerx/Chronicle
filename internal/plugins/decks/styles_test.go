package decks

import "testing"

// Every value of every style must be in its list, the style keys must be
// unique, and applying a style must give back exactly the style's values.
func TestStylesUseOnlyListedValues(t *testing.T) {
	if len(deckStyles) != 7 {
		t.Fatalf("seven styles, got %d", len(deckStyles))
	}
	seen := map[string]bool{}
	for _, s := range deckStyles {
		if seen[s.Key] || s.Key == "" || s.Name == "" || s.Line == "" {
			t.Errorf("style %q: key unique and name and line set", s.Key)
		}
		seen[s.Key] = true
		for _, c := range []struct {
			f, v string
			list []string
		}{
			{"border", s.Border, borderValues}, {"shape", s.Shape, shapeValues}, {"window", s.Window, windowValues},
			{"lettering", s.Lettering, letteringValues}, {"corner", s.Corner, cornerValues}, {"marker", s.Marker, markerValues},
			{"finish", s.Finish, finishValues}, {"pattern", s.Pattern, patternValues}, {"sign", s.Sign, backSignValues},
			{"turn", s.Turn, turnValues},
		} {
			if oneOf(c.f, c.v, c.list) != nil {
				t.Errorf("style %s: %s %q is not in its list", s.Key, c.f, c.v)
			}
		}
		if s.Colour != "" && oneOf("colour", s.Colour, Palette) != nil {
			t.Errorf("style %s: colour %q is not a palette key", s.Key, s.Colour)
		}
		got := s.Look()
		if got.Border != s.Border || got.Shape != s.Shape || got.Turn != s.Turn || got.Finish != s.Finish ||
			got.Back.Pattern != s.Pattern || got.Back.Colour != s.Colour || got.Back.Sign != s.Sign || got.Lettering != s.Lettering ||
			got.Corner != s.Corner || got.Marker != s.Marker || got.Window != s.Window {
			t.Errorf("style %s does not apply as written: %+v", s.Key, got)
		}
	}
	if deckStyles[0].Key != "plain" {
		t.Error("Plain is the first style")
	}
	l := deckStyles[0].Look()
	d := defaultLook()
	if l.Border != d.Border || l.Shape != d.Shape || l.Turn != d.Turn || l.Back.Pattern != d.Back.Pattern || l.Back.Sign != d.Back.Sign {
		t.Errorf("Plain must equal the default look: %+v vs %+v", l, d)
	}
}

// The seven rows are the signed table; changing one is a change to the
// signed design, so this pins the spots most easily mistyped.
func TestStylesMatchTheSignedTable(t *testing.T) {
	iron, _ := StyleByKey("iron-omens")
	if iron.Border != "metal" || iron.Window != "pointed" || iron.Pattern != "iron" || iron.Colour != "slate" || iron.Sign != "eye" || iron.Turn != "slam" || iron.Finish != "glow" {
		t.Errorf("iron omens: %+v", iron)
	}
	road, _ := StyleByKey("road-events")
	if road.Border != "none" || road.Shape != "standard" || road.Corner != "number" || road.Pattern != "waves" || road.Turn != "sweep" {
		t.Errorf("road events: %+v", road)
	}
	if _, ok := StyleByKey("nope"); ok {
		t.Error("an unknown key finds nothing")
	}
}
