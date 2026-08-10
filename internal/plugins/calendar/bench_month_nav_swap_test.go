// bench_month_nav_swap_test.go — THE MONTH STEP IS A SWAP, NOT A RELOAD.
//
// THE DEFECT, AND WHY ITS OWN DOC COMMENT HID IT. benchNavTrio ships three
// plain `<a>` elements and its comment explains the choice like this:
//
//	"The hrefs are computed server-side (benchNav) and land on the same route,
//	 so a plain navigation — or the sidebar's `hx-boost` when the page is
//	 reached that way — swaps the page with no script of any kind."
//
// The parenthesis is false. `hx-boost` is declared on the SIDEBAR's own
// containers (app.templ 203 / 363 / 456 / 495) and boost does not travel with a
// navigation — it is an attribute inherited from ANCESTORS OF THE CLICKED LINK,
// and `<main id="main-content">` (app.templ:73), which is every page's whole
// ancestry inside the layout, declares none. So the trio's anchors have never
// been boosted by anything, and every Prev / Today / Next has been a full
// document load: the sidebar, the topbar, every stylesheet, every script tag
// and every scaffold on the page, re-fetched and re-parsed, to move one month.
//
// WHAT THIS TEST ASSERTS, and why it is written against the rendered ancestry
// rather than against a substring in bench.templ:
//
//  1. THE ANCHORS ARE BOOSTED — the three carry, at or above themselves, an
//     `hx-boost="true"`. This is the claim the comment made and could not back.
//  2. THE BOOST IS BOUNDED to `#main-content`. A bare `hx-boost` targets
//     `<body>` and swaps innerHTML, which re-renders the sidebar and topbar into
//     the DOM and is most of what the defect was. The element that declares the
//     boost must also name a target and a selection, both `#main-content`, and
//     that id must actually exist in the page it was rendered from — the
//     ancestry is walked, so a target naming an element that is not there fails
//     here rather than silently swapping into nothing at runtime.
//  3. THE HREFS SURVIVE. [GR-1] SIGNED rules the cursor to be a URL because a
//     URL is shareable, refreshable and back-buttonable. Boost keeps the href
//     doing all three (JS off, middle-click, copy-link), and replacing the
//     anchors with buttons or with bare `hx-get` would quietly retire that
//     ruling — so the hrefs are asserted to still be the route.
//
// The walk is `golang.org/x/net/html`, on this package's own precedent
// (theater_test.go): "no ancestor of these three elements declares X" is not a
// claim a substring search can make.
package calendar

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// benchNavSwapMarkers are the trio's three anchors, by their stable data
// markers rather than by their labels.
var benchNavSwapMarkers = []string{
	"data-bench-nav-prev", "data-bench-nav-today", "data-bench-nav-next",
}

// benchNavFindByAttr returns the first element carrying attr, or nil.
func benchNavFindByAttr(root *html.Node, attr string) *html.Node {
	var found *html.Node
	theaterWalk(root, func(n *html.Node) {
		if found != nil {
			return
		}
		if _, ok := theaterAttr(n, attr); ok {
			found = n
		}
	})
	return found
}

// benchNavNearestWith returns the nearest ancestor-or-self of n declaring attr,
// which is exactly how htmx resolves an inherited attribute at click time.
func benchNavNearestWith(n *html.Node, attr string) *html.Node {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Type != html.ElementNode {
			continue
		}
		if _, ok := theaterAttr(cur, attr); ok {
			return cur
		}
	}
	return nil
}

// benchNavHasID reports whether the document contains an element with this id.
func benchNavHasID(root *html.Node, id string) bool {
	found := false
	theaterWalk(root, func(n *html.Node) {
		if v, ok := theaterAttr(n, "id"); ok && v == id {
			found = true
		}
	})
	return found
}

// TestBenchMonthNav_SteppingAMonthIsASwapAndNotAPageLoad is the whole of it.
func TestBenchMonthNav_SteppingAMonthIsASwapAndNotAPageLoad(t *testing.T) {
	markup := renderBench(t, benchFxData(true, true))
	doc := theaterParse(t, markup)

	// The page the trio is boosted INTO has to be there, or claim 2's target is
	// a string that resolves to nothing.
	const mainID = "main-content"
	if !benchNavHasID(doc, mainID) {
		t.Fatalf("the rendered Bench has no #%s — the layout's own content element "+
			"is the swap target and every claim below is about it", mainID)
	}

	for _, marker := range benchNavSwapMarkers {
		t.Run(marker, func(t *testing.T) {
			a := benchNavFindByAttr(doc, marker)
			if a == nil {
				t.Fatalf("no element carries %s — the month cursor is gone", marker)
			}
			if a.Data != "a" {
				t.Errorf("%s is a <%s>, want <a> — [GR-1] SIGNED rules the cursor to be "+
					"a URL, and a button is not one", marker, a.Data)
			}

			// (3) THE HREF SURVIVES, first, because it is the ruling the other
			// two claims must not cost.
			href, ok := theaterAttr(a, "href")
			if !ok || href == "" {
				t.Errorf("%s carries no href — [GR-1] SIGNED: the cursor is a URL so a GM "+
					"can paste the month of the siege into party chat. Boosting must keep "+
					"the href, never replace it with hx-get", marker)
			} else if !strings.Contains(href, "/apps/calendar") {
				t.Errorf("%s href = %q, want the Bench's own route", marker, href)
			}

			// (1) THE ANCHOR IS BOOSTED.
			boost := benchNavNearestWith(a, "hx-boost")
			if boost == nil {
				t.Fatalf("nothing at or above %s declares hx-boost, so clicking it is a "+
					"FULL DOCUMENT LOAD: the sidebar, the topbar, every script tag and "+
					"every scaffold re-fetched and re-parsed to move one month. The trio's "+
					"own doc comment claims \"the sidebar's hx-boost when the page is "+
					"reached that way\" covers this — it does not: boost is inherited from "+
					"a clicked link's ANCESTORS, and <main id=%q> declares none", marker, mainID)
			}
			if v, _ := theaterAttr(boost, "hx-boost"); v != "true" {
				t.Fatalf("the nearest hx-boost above %s is %q — a `false` here switches the "+
					"month step back to a document load", marker, v)
			}

			// (2) THE BOOST IS BOUNDED. Both attributes are read from the SAME
			// element the boost was found on: htmx resolves each independently by
			// walking ancestors, but declaring them apart is how a later edit
			// moves one and not the other.
			for _, attr := range []string{"hx-target", "hx-select"} {
				v, ok := theaterAttr(boost, attr)
				if !ok {
					t.Errorf("the boosted ancestor of %s declares no %s — an unbounded "+
						"hx-boost targets <body> with innerHTML, which re-renders the "+
						"sidebar and the topbar into the DOM and is most of what the "+
						"reload was", marker, attr)
					continue
				}
				if v != "#"+mainID {
					t.Errorf("%s = %q on the boosted ancestor of %s, want %q — the swap is "+
						"the layout's content element and nothing wider",
						attr, v, marker, "#"+mainID)
				}
			}
			// outerHTML, not innerHTML, and that is not a stylistic choice:
			// htmx's hx-select appends the SELECTED ELEMENTS to the fragment
			// (htmx.min.js, `if(o.select){…d.appendChild(e)…}`), so selecting
			// `#main-content` and swapping innerHTML nests a <main> inside the
			// <main> on every step and compounds. outerHTML replaces it.
			if v, _ := theaterAttr(boost, "hx-swap"); !strings.HasPrefix(v, "outerHTML") {
				t.Errorf("hx-swap = %q on the boosted ancestor of %s, want outerHTML — "+
					"hx-select=%q with an innerHTML swap puts the selected <main> INSIDE "+
					"the target <main>, and every month step nests one deeper",
					v, marker, "#"+mainID)
			}
		})
	}
}
