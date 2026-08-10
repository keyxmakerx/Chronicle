// bench_door_labels_test.go — A DOOR IS LABELLED WITH THE ROOM IT OPENS.
//
// THE DEFECT. The Bench's header rendered two controls a few pixels apart:
//
//	<a href=…/calendars/<id>/settings>Builder →</a>      (owners)
//	<a href=…/calendars/builder>+ New calendar</a>       (owners)
//
// The first lands on `CalendarSettingsPage` — a page whose own <h1> reads
// "Calendar Settings" — and the second lands on the builder wizard. Two doors,
// one lie: the one labelled "Builder" is not the builder, and the builder is
// the one beside it labelled something else. The identical href is already
// called "Settings" in the other two places the Bench prints it (the Block's
// management strip and each subordinate calendar row), so the header was also
// the only place the same destination had two names.
//
// WHY THE FIX IS THE STRING AND NOT THE HREF. Re-pointing the header control at
// the builder would give owners two adjacent controls that both open the
// wizard, and would DELETE the only header-level door to the primary calendar's
// settings — the page where months, weekdays, moons, seasons, eras, categories,
// weather, cycles and festivals are edited. The destination is right and useful;
// the word on it was wrong.
//
// WHAT THIS GUARD ASSERTS, in both directions, over every anchor the Bench
// renders rather than over the one that was wrong:
//
//  1. AN ANCHOR THAT SAYS "BUILDER" GOES TO THE BUILDER. Any link whose visible
//     text names the builder must have `/calendars/builder` in its href.
//  2. AN ANCHOR THAT GOES TO SETTINGS DOES NOT SAY "BUILDER". The converse, so
//     that renaming the destination page or adding a third door cannot
//     reintroduce the same mismatch from the other side.
//
// Both walk the parsed document (theater_test.go's helpers) because the claim
// pairs an element's TEXT with its HREF, and neither half is a substring the
// other can be found by.
package calendar

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// benchDoorText is an element's visible text, collapsed to single spaces.
func benchDoorText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			walk(k)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(sb.String()), " ")
}

// benchDoors returns every <a> in the rendered Bench as (text, href) pairs.
func benchDoors(t *testing.T, markup string) [][2]string {
	t.Helper()
	doc := theaterParse(t, markup)
	var out [][2]string
	theaterWalk(doc, func(n *html.Node) {
		if n.Data != "a" {
			return
		}
		href, ok := theaterAttr(n, "href")
		if !ok {
			return
		}
		out = append(out, [2]string{benchDoorText(n), href})
	})
	return out
}

// TestBenchDoors_ABuilderLinkGoesToTheBuilder is the whole of it.
func TestBenchDoors_ABuilderLinkGoesToTheBuilder(t *testing.T) {
	// The owner's Bench, because both doors are owner-only and a player's page
	// carries neither — asserting against a player would pass vacuously.
	doors := benchDoors(t, renderBench(t, benchFxData(true, true)))
	if len(doors) == 0 {
		t.Fatal("the rendered Bench has no links at all — every claim below would " +
			"pass vacuously")
	}

	const builderRoute = "/calendars/builder"
	sawBuilderDoor := false

	for _, d := range doors {
		text, href := d[0], d[1]
		lower := strings.ToLower(text)

		// (1) SAYS BUILDER → IS THE BUILDER.
		if strings.Contains(lower, "builder") {
			if !strings.Contains(href, builderRoute) {
				t.Errorf("a link reading %q points at %q, which is not the builder. "+
					"That href is the calendar SETTINGS page — its own <h1> reads "+
					"\"Calendar Settings\" — and the Bench already calls the identical "+
					"href \"Settings\" in the management strip and in every subordinate "+
					"row. A door is labelled with the room it opens", text, href)
				continue
			}
			sawBuilderDoor = true
		}

		// (2) GOES TO SETTINGS → DOES NOT SAY BUILDER. The converse arm, so the
		// mismatch cannot come back from the other side (a renamed destination,
		// or a third door added later).
		if strings.HasSuffix(href, "/settings") && strings.Contains(lower, "builder") {
			t.Errorf("the settings door reads %q — %q is the settings page, and the "+
				"builder is the separate control beside it", text, href)
		}
	}

	// The builder must still be REACHABLE from the Bench. Fixing a mislabelled
	// door by deleting the honest one beside it would satisfy both arms above.
	if !sawBuilderDoor {
		hasBuilderHref := false
		for _, d := range doors {
			if strings.Contains(d[1], builderRoute) {
				hasBuilderHref = true
			}
		}
		if !hasBuilderHref {
			t.Error("no link on the owner's Bench reaches the builder at all — the two " +
				"claims above are satisfied by a page with no builder door, and that is " +
				"not the fix")
		}
	}
}
