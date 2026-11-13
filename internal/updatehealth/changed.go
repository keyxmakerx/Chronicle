package updatehealth

// Changed is the list of features this release changed that people use.
//
// A pull request that changes such a feature adds an entry here: a new ID
// (counts are kept under it, so a changed feature changed again takes a new
// ID), the name a site admin knows it by, the routes that serve it, and a page
// to try it. An entry whose feature has passed on Site admin Home can be
// removed. internal/wire checks that every route here is a real route.
var Changed = []Feature{
	{
		ID:     "crafting-2026-11",
		Name:   "Crafting",
		Routes: []string{"/campaigns/:id/crafting*"},
		Try:    "/campaigns",
	},
	{
		ID:     "your-ai-page-2026-11",
		Name:   "The Your AI page",
		Routes: []string{"GET /campaigns/:id/ai", "GET /campaigns/:id/ai/activity"},
		Try:    "/campaigns",
	},
	{
		ID:     "admin-ai-helper-2026-11",
		Name:   "Site admin's AI helper",
		Routes: []string{"/admin/diagnostics/workspace*"},
		Try:    "/admin/diagnostics/workspace",
	},
}
