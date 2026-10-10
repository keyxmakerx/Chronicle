package entities

import (
	"context"

	"github.com/a-h/templ"
)

// name_fold.go holds the Go side of the Edit name fold: the inline handlers
// (built here from fixed text so nothing depends on a script inside a swapped
// fragment) and the small request-scoped values the templates read. The
// behaviour itself lives in static/js/widgets/page_name_fold.js
// (Chronicle.PageHeader).

// headerCall builds an inline handler that calls Chronicle.PageHeader.<method>
// with the element it sits on. method is fixed text, never request data.
func headerCall(method string) templ.ComponentScript {
	return inlineHandler("page_header_"+method,
		`(function(el){if(window.Chronicle&&Chronicle.PageHeader)Chronicle.PageHeader.`+method+`(el);})(this)`)
}

type pageParentKey struct{}

// pageParent is the page's current parent, shown in the Edit name fold so the
// search starts from what is saved rather than a bare id.
type pageParent struct {
	ID   string
	Name string
}

// withPageParent carries the page's parent to the Edit name fold.
func withPageParent(ctx context.Context, p pageParent) context.Context {
	return context.WithValue(ctx, pageParentKey{}, p)
}

// pageParentOf returns the parent the show handler resolved, or the zero value.
func pageParentOf(ctx context.Context) pageParent {
	p, _ := ctx.Value(pageParentKey{}).(pageParent)
	return p
}

// parentOfEntity finds the entity's immediate parent among its ancestors by id,
// not by position, so a reordered chain can never name the wrong page.
func parentOfEntity(entity *Entity, ancestors []Entity) pageParent {
	if entity == nil || entity.ParentID == nil {
		return pageParent{}
	}
	for i := range ancestors {
		if ancestors[i].ID == *entity.ParentID {
			return pageParent{ID: ancestors[i].ID, Name: ancestors[i].Name}
		}
	}
	// The parent exists but the viewer cannot see it: keep the id so an
	// unrelated save never clears it, and show no name.
	return pageParent{ID: *entity.ParentID}
}

// boolAttr is "true"/"false" for data attributes the widgets read as strings.
func boolAttr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
