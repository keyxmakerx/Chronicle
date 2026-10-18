package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/mapsketch"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// PageMapsAPI is the page blocks service as the page-map kind and the page
// lookup need it, adapted in app wiring so this package does not import the
// plugin. The service keeps its own rules: who may edit, DM-only markers,
// page links the reader can see, and refusing a map that draws nothing.
type PageMapsAPI interface {
	// FirstBlock is the page's first Page map block; ok is false when the
	// page's category layout has none.
	FirstBlock(ctx context.Context, campaignID, pageID string) (blockID string, ok bool, err error)
	// Text is the stored map in the line grammar, filtered for the reader.
	Text(ctx context.Context, campaignID, pageID, blockID string, a Actor) (string, error)
	// Save replaces the whole map.
	Save(ctx context.Context, campaignID, pageID, blockID string, a Actor, text string) error
}

// noMapBlock is why a page can't take a map: the block lives in the
// category's layout, which every page of that category shares, so the
// import never adds one.
const noMapBlock = "that page has no map yet. Add a Page map block to its layout in Customize"

// PageMapKind draws the small map on a page, in the line grammar the
// page's own "From text" reads. One map per page; an update replaces it.
type PageMapKind struct {
	Svc      PageMapsAPI
	Entities EntityLookup
}

func (PageMapKind) Name() string  { return "page-map" }
func (PageMapKind) Label() string { return "Page map" }
func (PageMapKind) Doc() string {
	return "The small map drawn on a page, matched by `page` (the page's name). The page's category must have a Page map block in its layout; the import never adds one. `action: create` draws a map on a page whose map is empty; `action: update` replaces the whole map, so send every line, not just the changes (a `page` lookup shows the current lines). The body is the map, one thing per line:\n\n" +
		mapsketch.Grammar + "\n\n" +
		"```\n---\nkind: page-map\naction: update\npage: Harrowmere\n---\nbackground hex\nroom \"The keep\" 18,11 to 22,15 color teal\npin \"The gate\" 20,22 number page \"Harrowmere Gate\": Terms at dusk.\n```"
}

// Title names the row on the review screen: "Map on Harrowmere".
func (PageMapKind) Title(r Record) string {
	if p := pageMapPage(r); p != "" {
		return "Map on " + p
	}
	return ""
}

// pageMapPage is the page the record names: `page`, or `name` as a fallback
// since the map has no name of its own.
func pageMapPage(r Record) string {
	if p := r.Str("page"); p != "" {
		return p
	}
	return r.Name
}

func (k PageMapKind) page(ctx context.Context, campaignID, name string) *entities.Entity {
	e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID || !sameName(e.Name, name) {
		return nil
	}
	return e
}

// mapSummary is the review row's line: "6 markers, 3 shapes, hex background".
func mapSummary(res mapsketch.Result) string {
	s := plural(res.Markers, "marker", "markers") + ", " + plural(res.Shapes, "shape", "shapes")
	if res.Background != "" && res.Background != "none" {
		s += ", " + res.Background + " background"
	}
	return s
}

func (k PageMapKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can draw page maps"}
	}
	name := pageMapPage(r)
	if name == "" {
		return Plan{Error: "a page map needs `page`: the page it is drawn on"}
	}
	switch r.Action {
	case ActionCreate, ActionUpdate:
	default:
		return Plan{Error: "a page map can't be removed from here; open the page and clear it there"}
	}
	res := mapsketch.Parse(r.Body)
	if res.Markers+res.Shapes == 0 {
		return Plan{Error: "the map draws nothing: write its lines below the front matter", Warnings: res.Problems}
	}
	p := Plan{Summary: mapSummary(res), Warnings: res.Problems}
	e := k.page(ctx, campaignID, name)
	if e == nil {
		if r.Action == ActionCreate && r.Made[entities.Slugify(name)] {
			p.Warnings = append(p.Warnings, quote(name)+" is made in this paste; its category needs a Page map block, or this row fails")
			return p
		}
		return Plan{Error: "no page called " + quote(name)}
	}
	block, ok, err := k.Svc.FirstBlock(ctx, campaignID, e.ID)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if !ok {
		return Plan{Error: noMapBlock}
	}
	if r.Action == ActionCreate {
		cur, err := k.Svc.Text(ctx, campaignID, e.ID, block, a)
		if err != nil {
			return Plan{Error: planError(err)}
		}
		if strings.TrimSpace(cur) != "" {
			return Plan{Error: e.Name + " already has a map; use action: update to replace it"}
		}
	} else {
		p.Summary = "replaces the map: " + p.Summary
	}
	return p
}

func (k PageMapKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	name := pageMapPage(r)
	e := k.page(ctx, campaignID, name)
	if e == nil {
		return badRequestf("no page called %s", quote(name))
	}
	block, ok, err := k.Svc.FirstBlock(ctx, campaignID, e.ID)
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewBadRequest(noMapBlock)
	}
	return k.Svc.Save(ctx, campaignID, e.ID, block, a, r.Body)
}

// Export is empty: maps are read one page at a time through the `page`
// lookup, which ends with the page's map lines.
func (PageMapKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

// pageMapText is the page's map as a block for the `page` lookup, or "".
func pageMapText(ctx context.Context, svc PageMapsAPI, campaignID string, e *entities.Entity, a Actor) string {
	if svc == nil {
		return ""
	}
	block, ok, err := svc.FirstBlock(ctx, campaignID, e.ID)
	if err != nil || !ok {
		return ""
	}
	text, err := svc.Text(ctx, campaignID, e.ID, block, a)
	if err != nil || strings.TrimSpace(text) == "" {
		return ""
	}
	return fmt.Sprintf("\n\n### Map\n\nSend these lines back in a `kind: page-map` block with `action: update` to change it.\n\n```\n%s\n```\n", strings.TrimSpace(text))
}
