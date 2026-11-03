package transformations

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
)

// Service owns the layer rules. Reading a page's layers needs only the page;
// putting one on or taking one off is the Director's, on a page they may
// edit.
type Service interface {
	// ListForPage returns a page's layers, oldest first.
	ListForPage(ctx context.Context, campaignID, entityID string, v Viewer) (*PageLayers, error)
	// Add puts a layer on a page.
	Add(ctx context.Context, campaignID, entityID string, v Viewer, in AddInput) (*Layer, error)
	// Remove takes a layer off, leaving the page as it was underneath.
	Remove(ctx context.Context, campaignID, id string, v Viewer) error
}

type service struct {
	repo  Repository
	pages PageDirectory
}

// NewService builds the service.
func NewService(repo Repository, pages PageDirectory) Service {
	return &service{repo: repo, pages: pages}
}

// access decides the page exists in the campaign and the viewer may see it.
// A hidden page is reported exactly like a missing one.
func (s *service) access(ctx context.Context, campaignID, entityID string, v Viewer) (PageAccess, error) {
	if !validToken(entityID) {
		return PageAccess{}, errNotFound("page")
	}
	a, err := s.pages.Access(ctx, campaignID, entityID, v)
	if err != nil {
		return PageAccess{}, apperrorInternal(err)
	}
	if !a.Exists || !a.CanView {
		return PageAccess{}, errNotFound("page")
	}
	return a, nil
}

// editable is the Director on a page they may edit.
func (s *service) editable(ctx context.Context, campaignID, entityID string, v Viewer) error {
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return err
	}
	if !v.IsDM() {
		return errForbidden("only the Director changes what is transforming a hero")
	}
	if !a.CanEdit {
		return errForbidden("you can't edit this page")
	}
	return nil
}

// present decodes a stored row. It passed validation when saved; a row that
// no longer parses is an internal error, never sent half-read.
func present(r Row) (Layer, error) {
	var d doc
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return Layer{}, apperrorInternal(err)
	}
	if d.Changes == nil {
		d.Changes = []Change{}
	}
	return Layer{ID: r.ID, EntityID: r.EntityID, Name: r.Name, Source: r.Source, Note: d.Note, Changes: d.Changes, CreatedAt: r.CreatedAt}, nil
}

func (s *service) ListForPage(ctx context.Context, campaignID, entityID string, v Viewer) (*PageLayers, error) {
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListForPage(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	out := &PageLayers{Layers: []Layer{}, CanEdit: v.IsDM() && a.CanEdit}
	for _, r := range rows {
		l, err := present(r)
		if err != nil {
			return nil, err
		}
		out.Layers = append(out.Layers, l)
	}
	return out, nil
}

func (s *service) Add(ctx context.Context, campaignID, entityID string, v Viewer, in AddInput) (*Layer, error) {
	if err := s.editable(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	in, err := normalise(in)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(doc{Note: in.Note, Changes: in.Changes})
	if err != nil {
		return nil, apperrorInternal(err)
	}
	r := Row{ID: uuid.NewString(), CampaignID: campaignID, EntityID: entityID, Name: in.Name, Source: in.Source, Data: data}
	ok, err := s.repo.Create(ctx, r, v.UserID, MaxLayersPerPage)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errInvalid("a hero can have at most 20 layers; take one off first")
	}
	stored, found, err := s.repo.Get(ctx, campaignID, r.ID)
	if err != nil || !found {
		return nil, apperrorInternal(err)
	}
	slog.Info("layer put on", slog.String("campaign_id", campaignID), slog.String("entity_id", entityID),
		slog.String("layer_id", r.ID), slog.String("source", in.Source))
	l, err := present(stored)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *service) Remove(ctx context.Context, campaignID, id string, v Viewer) error {
	if !validToken(id) {
		return errNotFound("layer")
	}
	r, found, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return apperrorInternal(err)
	}
	if !found {
		return errNotFound("layer")
	}
	if err := s.editable(ctx, campaignID, r.EntityID, v); err != nil {
		return err
	}
	ok, err := s.repo.Delete(ctx, campaignID, id)
	if err != nil {
		return apperrorInternal(err)
	}
	if !ok {
		return errNotFound("layer")
	}
	slog.Info("layer taken off", slog.String("campaign_id", campaignID), slog.String("entity_id", r.EntityID), slog.String("layer_id", id))
	return nil
}
