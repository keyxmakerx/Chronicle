package pageblocks

import (
	"context"
	"encoding/json"

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
// table that has DM-only columns: they never receive the hidden columns, so
// whatever they saved would erase them. Refusing the edit is simpler and
// safer than merging.
func forViewer(doc any, a PageAccess, v Viewer) (any, bool) {
	canEdit := a.CanEdit
	if t, ok := doc.(Table); ok && !seesDMOnly(v) {
		if hasDMOnly(t) {
			canEdit = false
		}
		doc = stripDMOnly(t)
	}
	return doc, canEdit
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
	doc, canEdit := forViewer(doc, a, v)
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
	if in.Kind == KindTable && !seesDMOnly(v) {
		if found {
			prev, err := validateTable(st.Data)
			if err != nil {
				return nil, apperrorInternal(err)
			}
			if hasDMOnly(prev) {
				return nil, errForbidden("only the DM can edit a table with DM-only columns")
			}
		}
		if hasDMOnly(doc.(Table)) {
			return nil, errForbidden("only the DM can mark a column DM-only")
		}
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	saved, err := s.repo.Save(ctx, campaignID, entityID, blockID, in.Kind, raw, version, v.UserID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !saved {
		return nil, errConflict()
	}
	out, canEdit := forViewer(doc, a, v)
	return &View{Kind: in.Kind, Data: out, Version: version + 1, CanEdit: canEdit}, nil
}
