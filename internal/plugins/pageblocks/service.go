package pageblocks

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/mapsketch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Service owns the block content rules.
type Service interface {
	// Get returns the block as viewer v may see it. kindHint says which
	// kind to answer with for a block that has no saved row; a saved row's
	// own kind wins. A page that is missing or hidden from v is NotFound.
	Get(ctx context.Context, campaignID, entityID, blockID, kindHint string, v Viewer) (*View, error)
	// Put validates and saves a block, returning the saved view.
	Put(ctx context.Context, campaignID, entityID, blockID string, v Viewer, in SaveInput) (*View, error)

	// PageMapText is the page map as the line grammar (mapsketch.Grammar), as
	// viewer v sees it: DM-only markers and links to pages v cannot open are
	// left out. version is the one a save must carry. A block with no saved
	// map reads as empty text at version 0.
	PageMapText(ctx context.Context, campaignID, entityID, blockID string, v Viewer) (text string, version int, err error)
	// EditorSketchText is PageMapText for the editor's "edit as text": it also
	// requires that v may edit the page and may save everything on the map.
	EditorSketchText(ctx context.Context, campaignID, entityID, blockID string, v Viewer) (text string, version int, err error)
	// PreviewSketch parses text and resolves its page names for an editor,
	// without saving. Warnings are the lines left out and the names that
	// matched no page v can open.
	PreviewSketch(ctx context.Context, campaignID, entityID, blockID string, v Viewer, text string) (*SketchPreview, error)
	// SavePageMapText parses text, resolves its page names, validates, and
	// replaces the whole map with the same permission rules as Put. A line
	// that cannot be drawn is left out and reported in Result.Problems, as are
	// page names that matched nothing (those lack the "Line N: " prefix).
	SavePageMapText(ctx context.Context, campaignID, entityID, blockID string, v Viewer, text string) (mapsketch.Result, error)
	// FirstPageMapBlock finds the first page_map block in the layout of the
	// page's category.
	FirstPageMapBlock(ctx context.Context, campaignID, entityID string) (blockID string, ok bool, err error)
}

type service struct {
	repo     Repository
	entities EntityDirectory
}

// NewService builds the service.
func NewService(repo Repository, entities EntityDirectory) Service {
	return &service{repo: repo, entities: entities}
}

// access is the one place that decides a page exists in the campaign and the
// viewer may see it. A hidden page is reported exactly like a missing one so
// its existence is not confirmed.
func (s *service) access(ctx context.Context, campaignID, entityID string, v Viewer) (PageAccess, error) {
	a, err := s.entities.Access(ctx, campaignID, entityID, v)
	if err != nil {
		return PageAccess{}, apperrorInternal(err)
	}
	if !a.Exists || !a.CanView {
		return PageAccess{}, errNotFound("page")
	}
	return a, nil
}

// seesDMOnly is the rule DM-only blocks use on the page: Owner visibility,
// which an owner and a member with DM access both have.
func seesDMOnly(v Viewer) bool { return v.VisibilityRole >= permissions.RoleOwner }

// load reads the stored document, or the empty one for kindHint.
func (s *service) load(ctx context.Context, campaignID, entityID, blockID, kindHint string) (kind string, doc any, version int, err error) {
	st, found, err := s.repo.Get(ctx, campaignID, entityID, blockID)
	if err != nil {
		return "", nil, 0, apperrorInternal(err)
	}
	if !found {
		if !validKind(kindHint) {
			return "", nil, 0, errInvalid("unknown block kind")
		}
		return kindHint, emptyDoc(kindHint), 0, nil
	}
	// The stored document passed validation when it was saved; validating
	// again on read keeps a hand-edited row from reaching the wire.
	doc, err = validateDoc(st.Kind, st.Data)
	if err != nil {
		return "", nil, 0, apperrorInternal(err)
	}
	return st.Kind, doc, st.Version, nil
}

// forViewer filters doc for v and decides canEdit.
//
// A viewer below Owner visibility who may edit the page still cannot edit a
// table that has DM-only columns, a page map that has DM-only markers, or a
// checklist with DM-only parts:
// they never receive the hidden parts, so whatever they saved would erase
// them. Refusing the edit is simpler and safer than merging. A page map that
// links to a page the viewer cannot open is refused for the same reason.
func (s *service) forViewer(ctx context.Context, campaignID string, doc any, a PageAccess, v Viewer) (any, bool, error) {
	canEdit := a.CanEdit
	switch d := doc.(type) {
	case Table:
		if !seesDMOnly(v) {
			if hasDMOnly(d) {
				canEdit = false
			}
			doc = stripDMOnly(d)
		}
	case mapsketch.Doc:
		out, hidden, err := s.pageMapForViewer(ctx, campaignID, d, v)
		if err != nil {
			return nil, false, err
		}
		if hidden {
			canEdit = false
		}
		doc = out
	case Checklist:
		out, hidden, err := s.checklistForViewer(ctx, campaignID, d, v)
		if err != nil {
			return nil, false, err
		}
		if hidden {
			canEdit = false
		}
		doc = out
	}
	return doc, canEdit, nil
}

func (s *service) Get(ctx context.Context, campaignID, entityID, blockID, kindHint string, v Viewer) (*View, error) {
	if !validToken(blockID) {
		return nil, errNotFound("block")
	}
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	kind, doc, version, err := s.load(ctx, campaignID, entityID, blockID, kindHint)
	if err != nil {
		return nil, err
	}
	doc, canEdit, err := s.forViewer(ctx, campaignID, doc, a, v)
	if err != nil {
		return nil, err
	}
	return &View{Kind: kind, Data: doc, Version: version, CanEdit: canEdit}, nil
}

func (s *service) Put(ctx context.Context, campaignID, entityID, blockID string, v Viewer, in SaveInput) (*View, error) {
	if !validToken(blockID) {
		return nil, errNotFound("block")
	}
	// Hidden pages are 404 before anything else, so a 403 never confirms one.
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	if !a.CanEdit {
		return nil, errForbidden("you can't edit this page")
	}
	if in.Version == nil || *in.Version < 0 {
		return nil, errInvalid("version is required")
	}
	if !validKind(in.Kind) {
		return nil, errInvalid("unknown block kind")
	}

	st, found, err := s.repo.Get(ctx, campaignID, entityID, blockID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	version := 0
	if found {
		version = st.Version
		if st.Kind != in.Kind {
			return nil, errInvalid("this block holds a different kind of content")
		}
	}
	if *in.Version != version {
		return nil, errConflict()
	}

	doc, err := validateDoc(in.Kind, in.Data)
	if err != nil {
		return nil, err
	}
	return s.commit(ctx, campaignID, entityID, blockID, v, a, st.Data, found, in.Kind, doc, version)
}

// commit applies the DM-only rules to a validated document, stores it as the
// successor of expected, and returns the saved view. prev is the stored data
// when found.
func (s *service) commit(ctx context.Context, campaignID, entityID, blockID string, v Viewer, a PageAccess, prev []byte, found bool, kind string, doc any, expected int) (*View, error) {
	switch kind {
	case KindTable:
		if !seesDMOnly(v) {
			if found {
				old, err := validateTable(prev)
				if err != nil {
					return nil, apperrorInternal(err)
				}
				if hasDMOnly(old) {
					return nil, errForbidden("only the DM can edit a table with DM-only columns")
				}
			}
			if hasDMOnly(doc.(Table)) {
				return nil, errForbidden("only the DM can mark a column DM-only")
			}
		}
	case KindPageMap:
		if err := s.checkPageMapWrite(ctx, campaignID, v, prev, found, doc.(mapsketch.Doc)); err != nil {
			return nil, err
		}
	case KindChecklist:
		if err := s.checkChecklistWrite(ctx, campaignID, v, prev, found, doc.(Checklist)); err != nil {
			return nil, err
		}
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	saved, err := s.repo.Save(ctx, campaignID, entityID, blockID, kind, raw, expected, v.UserID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !saved {
		return nil, errConflict()
	}
	out, canEdit, err := s.forViewer(ctx, campaignID, doc, a, v)
	if err != nil {
		return nil, err
	}
	return &View{Kind: kind, Data: out, Version: expected + 1, CanEdit: canEdit}, nil
}
