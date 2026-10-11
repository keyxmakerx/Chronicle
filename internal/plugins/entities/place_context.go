package entities

import (
	"context"
	"fmt"

	"github.com/a-h/templ"
)

// place_context.go carries the extra listings a handler already loaded (and
// already filtered for this viewer) to the templates that draw them, the same
// way page_extras.go carries a page's sub-pages. Templates never query.

type pagePlacesKey struct{}

// pagePlaces is the "Also listed under" line of one page.
type pagePlaces struct {
	// Enabled is false when listings are not wired, which hides the line.
	Enabled bool
	// CanEdit shows the add and remove controls. The server checks again on
	// every write; this only decides what is drawn.
	CanEdit bool
	// Links are the places this viewer may see.
	Links []PlaceLink
	// RealParentID is the page's home, which the picker never offers.
	RealParentID string
}

func withPagePlaces(ctx context.Context, p pagePlaces) context.Context {
	return context.WithValue(ctx, pagePlacesKey{}, p)
}

func pagePlacesOf(ctx context.Context) pagePlaces {
	p, _ := ctx.Value(pagePlacesKey{}).(pagePlaces)
	return p
}

type treePlacesKey struct{}

// withTreePlaces carries the listings to draw in a tree (the sidebar's and the
// category page's), already limited to what this viewer may see.
func withTreePlaces(ctx context.Context, links []PlaceLink) context.Context {
	return context.WithValue(ctx, treePlacesKey{}, links)
}

func treePlacesOf(ctx context.Context) []PlaceLink {
	l, _ := ctx.Value(treePlacesKey{}).([]PlaceLink)
	return l
}

// placesCall builds an inline handler that calls Chronicle.PagePlaces.<method>
// with the element it sits on. method is fixed text, never request data. It is
// inline (not a templ script) so it survives an HTMX swap of the line.
func placesCall(method string) templ.ComponentScript {
	return inlineHandler("page_places_"+method,
		`(function(el){if(window.Chronicle&&Chronicle.PagePlaces)Chronicle.PagePlaces.`+method+`(el);})(this)`)
}

// placeSidebarKey is the unique key of one listing row in the sidebar tree: a
// page can appear under several parents, so the page id alone no longer names
// a row.
func placeSidebarKey(l PlaceLink) string {
	return l.EntityID + "~" + l.ParentID
}

// placeIDList is the ids a page is listed under, for the picker to skip.
func placeIDList(links []PlaceLink) string {
	out := ""
	for i, l := range links {
		if i > 0 {
			out += ","
		}
		out += l.ParentID
	}
	return out
}

func placesEndpoint(campaignID, entityID string) string {
	return fmt.Sprintf("/campaigns/%s/entities/%s/places", campaignID, entityID)
}
