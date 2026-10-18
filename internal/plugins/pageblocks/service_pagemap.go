package pageblocks

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/mapsketch"
)

// SketchPreview is what the editor gets back for text it typed: the map the
// text makes, and a plain sentence for everything that was left out or did not
// match. Nothing is saved.
type SketchPreview struct {
	Data     mapsketch.Doc `json:"data"`
	Warnings []string      `json:"warnings"`
}

// loadPageMap reads the stored map, or the empty one. A block that holds
// another kind of content is refused rather than overwritten.
func (s *service) loadPageMap(ctx context.Context, campaignID, entityID, blockID string) (doc mapsketch.Doc, st Stored, found bool, err error) {
	st, found, err = s.repo.Get(ctx, campaignID, entityID, blockID)
	if err != nil {
		return mapsketch.Doc{}, Stored{}, false, apperrorInternal(err)
	}
	if !found {
		return mapsketch.NewDoc(), Stored{}, false, nil
	}
	if st.Kind != KindPageMap {
		return mapsketch.Doc{}, Stored{}, false, errInvalid("this block holds a different kind of content")
	}
	// Validated again on read, like every kind, so a hand-edited row cannot
	// reach the wire.
	doc, err = validatePageMap(st.Data)
	if err != nil {
		return mapsketch.Doc{}, Stored{}, false, apperrorInternal(err)
	}
	return doc, st, true, nil
}

// pageMapForViewer returns d as v may see it: DM-only markers removed for a
// viewer below Owner visibility, and a link to a page v cannot open removed
// (the marker stays, as a note), with the linked page's name and path added to
// the links that remain. hidden reports that something was removed, which is
// what makes the map read-only for v. It copies; the stored document is never
// altered.
func (s *service) pageMapForViewer(ctx context.Context, campaignID string, d mapsketch.Doc, v Viewer) (out mapsketch.Doc, hidden bool, err error) {
	out = d
	out.Pins = make([]mapsketch.Pin, 0, len(d.Pins))
	for _, p := range d.Pins {
		if p.DM && !seesDMOnly(v) {
			hidden = true
			continue
		}
		out.Pins = append(out.Pins, p)
	}
	ids := pinEntityIDs(out)
	if len(ids) == 0 {
		return out, hidden, nil
	}
	links, err := s.entities.FilterViewable(ctx, campaignID, ids, v)
	if err != nil {
		return mapsketch.Doc{}, false, apperrorInternal(err)
	}
	for i := range out.Pins {
		p := &out.Pins[i]
		if p.EntityID == "" {
			continue
		}
		l, ok := links[p.EntityID]
		if !ok {
			p.EntityID = ""
			hidden = true
			continue
		}
		p.EntityName, p.EntityURL = l.Name, l.URL
	}
	return out, hidden, nil
}

// checkPageMapWrite enforces, for a save by v, that v neither erases nor
// creates what they cannot see, and links only to pages they can open.
func (s *service) checkPageMapWrite(ctx context.Context, campaignID string, v Viewer, prevData []byte, found bool, next mapsketch.Doc) error {
	var prev mapsketch.Doc
	if found {
		var err error
		if prev, err = validatePageMap(prevData); err != nil {
			return apperrorInternal(err)
		}
	}
	if !seesDMOnly(v) {
		if hasDMPins(prev) {
			return errForbidden("only the DM can edit a map with DM-only markers")
		}
		if hasDMPins(next) {
			return errForbidden("only the DM can mark a marker DM-only")
		}
	}
	// A save from someone who cannot see a stored link would drop it, since
	// they never received it.
	if ids := pinEntityIDs(prev); len(ids) > 0 {
		links, err := s.entities.FilterViewable(ctx, campaignID, ids, v)
		if err != nil {
			return apperrorInternal(err)
		}
		if len(links) < len(ids) {
			return errForbidden("this map links to pages you can't open, so you can't edit it")
		}
	}
	if ids := pinEntityIDs(next); len(ids) > 0 {
		links, err := s.entities.FilterViewable(ctx, campaignID, ids, v)
		if err != nil {
			return apperrorInternal(err)
		}
		if len(links) < len(ids) {
			return errInvalid("a marker links to a page that doesn't exist or that you can't open")
		}
	}
	return nil
}

// sketchText writes d (already filtered for the viewer) as the line grammar,
// naming each linked page by the name it is written with.
func sketchText(d mapsketch.Doc) string {
	pins := make([]mapsketch.Pin, len(d.Pins))
	copy(pins, d.Pins)
	for i := range pins {
		if pins[i].EntityID != "" {
			pins[i].Page = pins[i].EntityName
		}
	}
	d.Pins = pins
	return mapsketch.Format(d)
}

func (s *service) PageMapText(ctx context.Context, campaignID, entityID, blockID string, v Viewer) (string, int, error) {
	if !validToken(blockID) {
		return "", 0, errNotFound("block")
	}
	if _, err := s.access(ctx, campaignID, entityID, v); err != nil {
		return "", 0, err
	}
	doc, st, _, err := s.loadPageMap(ctx, campaignID, entityID, blockID)
	if err != nil {
		return "", 0, err
	}
	fd, _, err := s.pageMapForViewer(ctx, campaignID, doc, v)
	if err != nil {
		return "", 0, err
	}
	return sketchText(fd), st.Version, nil
}

func (s *service) EditorSketchText(ctx context.Context, campaignID, entityID, blockID string, v Viewer) (string, int, error) {
	if !validToken(blockID) {
		return "", 0, errNotFound("block")
	}
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return "", 0, err
	}
	if !a.CanEdit {
		return "", 0, errForbidden("you can't edit this page")
	}
	doc, st, _, err := s.loadPageMap(ctx, campaignID, entityID, blockID)
	if err != nil {
		return "", 0, err
	}
	fd, hidden, err := s.pageMapForViewer(ctx, campaignID, doc, v)
	if err != nil {
		return "", 0, err
	}
	if hidden {
		return "", 0, errForbidden("this map has parts you can't see, so only someone who can see them can edit it")
	}
	return sketchText(fd), st.Version, nil
}

// resolvePages turns each marker's page name into a link the viewer may open.
// A name that matches nothing, or only a page v cannot see, is reported the
// same way and the marker keeps its note.
func (s *service) resolvePages(ctx context.Context, campaignID string, v Viewer, d *mapsketch.Doc) ([]string, error) {
	warnings := []string{}
	cache := map[string]*PageLink{}
	for i := range d.Pins {
		p := &d.Pins[i]
		if p.Page == "" {
			continue
		}
		link, done := cache[p.Page]
		if !done {
			l, ok, err := s.entities.FindByName(ctx, campaignID, p.Page, v)
			if err != nil {
				return nil, apperrorInternal(err)
			}
			if ok {
				link = &l
			}
			cache[p.Page] = link
		}
		if link == nil {
			who := "the marker"
			if p.Name != "" {
				who = fmt.Sprintf("the marker %q", p.Name)
			}
			warnings = append(warnings, fmt.Sprintf("There is no page called %q that you can open, so %s keeps its note.", p.Page, who))
			continue
		}
		p.EntityID, p.EntityName, p.EntityURL, p.Page = link.ID, link.Name, link.URL, ""
	}
	return warnings, nil
}

// parseSketch reads text and resolves its names, for an editor.
func (s *service) parseSketch(ctx context.Context, campaignID string, v Viewer, text string) (mapsketch.Result, []string, error) {
	res := mapsketch.Parse(normalizeNewlines(text))
	warnings, err := s.resolvePages(ctx, campaignID, v, &res.Doc)
	if err != nil {
		return mapsketch.Result{}, nil, err
	}
	return res, warnings, nil
}

func (s *service) PreviewSketch(ctx context.Context, campaignID, entityID, blockID string, v Viewer, text string) (*SketchPreview, error) {
	if !validToken(blockID) {
		return nil, errNotFound("block")
	}
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	if !a.CanEdit {
		return nil, errForbidden("you can't edit this page")
	}
	if _, _, _, err := s.loadPageMap(ctx, campaignID, entityID, blockID); err != nil {
		return nil, err
	}
	res, warnings, err := s.parseSketch(ctx, campaignID, v, text)
	if err != nil {
		return nil, err
	}
	if !seesDMOnly(v) && hasDMPins(res.Doc) {
		return nil, errForbidden("only the DM can mark a marker DM-only")
	}
	clean, err := checkPageMap(res.Doc)
	if err != nil {
		return nil, err
	}
	// The editor needs the typed page name of a marker that matched nothing,
	// and the name and path of one that did; neither is stored.
	for i := range clean.Pins {
		src := res.Doc.Pins[i]
		clean.Pins[i].Page, clean.Pins[i].EntityName, clean.Pins[i].EntityURL = src.Page, src.EntityName, src.EntityURL
	}
	return &SketchPreview{Data: clean, Warnings: append(append([]string{}, res.Problems...), warnings...)}, nil
}

func (s *service) SavePageMapText(ctx context.Context, campaignID, entityID, blockID string, v Viewer, text string) (mapsketch.Result, error) {
	if !validToken(blockID) {
		return mapsketch.Result{}, errNotFound("block")
	}
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return mapsketch.Result{}, err
	}
	if !a.CanEdit {
		return mapsketch.Result{}, errForbidden("you can't edit this page")
	}
	_, st, found, err := s.loadPageMap(ctx, campaignID, entityID, blockID)
	if err != nil {
		return mapsketch.Result{}, err
	}
	res, warnings, err := s.parseSketch(ctx, campaignID, v, text)
	if err != nil {
		return mapsketch.Result{}, err
	}
	// A replacement that read as nothing but problems would wipe the map for
	// text that was probably meant as something else.
	if res.Shapes+res.Markers == 0 && len(res.Problems) > 0 {
		return mapsketch.Result{}, errInvalid("nothing on the map could be read. " + res.Problems[0])
	}
	clean, err := checkPageMap(res.Doc)
	if err != nil {
		return mapsketch.Result{}, err
	}
	version := 0
	if found {
		version = st.Version
	}
	view, err := s.commit(ctx, campaignID, entityID, blockID, v, a, st.Data, found, KindPageMap, clean, version)
	if err != nil {
		return mapsketch.Result{}, err
	}
	res.Doc = view.Data.(mapsketch.Doc)
	res.Problems = append(res.Problems, warnings...)
	return res, nil
}

func (s *service) FirstPageMapBlock(ctx context.Context, campaignID, entityID string) (string, bool, error) {
	id, ok, err := s.entities.FirstPageMapBlock(ctx, campaignID, entityID)
	if err != nil {
		return "", false, apperrorInternal(err)
	}
	return id, ok, nil
}
