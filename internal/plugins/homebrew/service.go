package homebrew

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Service is the Workshop's recipe store. Every method is for Directors:
// players meet a recipe only through the widgets it places, which keep their
// own audience rules.
type Service interface {
	Catalog(v Viewer) (Catalog, error)
	List(ctx context.Context, campaignID string, v Viewer) ([]RecipeSummary, error)
	Get(ctx context.Context, campaignID, id string, v Viewer) (*Recipe, error)
	Create(ctx context.Context, campaignID string, v Viewer, in RecipeInput) (*Recipe, error)
	Update(ctx context.Context, campaignID, id string, v Viewer, in RecipeInput) (*Recipe, error)
	Delete(ctx context.Context, campaignID, id string, v Viewer) error
}

type service struct {
	repo  Repository
	reg   *Registry
	newID func() string
}

// NewService builds the service over a repository and the registry the app
// filled at start.
func NewService(repo Repository, reg *Registry) Service {
	return &service{repo: repo, reg: reg, newID: uuid.NewString}
}

func requireDirector(v Viewer) error {
	if !v.Director {
		return errForbidden("only a Director can use the Homebrew Workshop")
	}
	return nil
}

func (s *service) Catalog(v Viewer) (Catalog, error) {
	if err := requireDirector(v); err != nil {
		return Catalog{}, err
	}
	return s.reg.Catalog(), nil
}

func (s *service) List(ctx context.Context, campaignID string, v Viewer) ([]RecipeSummary, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	rs, err := s.repo.List(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	out := make([]RecipeSummary, 0, len(rs))
	for _, r := range rs {
		out = append(out, RecipeSummary{ID: r.ID, Name: r.Name, Parts: len(r.Doc.Parts), Rules: len(r.Doc.Rules), Version: r.Version, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

func (s *service) get(ctx context.Context, campaignID, id string) (Recipe, error) {
	if !validToken(id) {
		return Recipe{}, errNotFound("recipe")
	}
	r, ok, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return Recipe{}, errInternal(err)
	}
	if !ok {
		return Recipe{}, errNotFound("recipe")
	}
	return r, nil
}

func (s *service) Get(ctx context.Context, campaignID, id string, v Viewer) (*Recipe, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	r, err := s.get(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// check validates a body into the recipe it describes.
func (s *service) check(ctx context.Context, campaignID string, in RecipeInput) (string, Doc, error) {
	name, err := cleanName("name", in.Name)
	if err != nil {
		return "", Doc{}, err
	}
	doc, err := validateDoc(ctx, s.reg, campaignID, in.Doc)
	if err != nil {
		return "", Doc{}, err
	}
	return name, doc, nil
}

func dupName(err error, name string) error {
	if errors.Is(err, errDuplicate) {
		return errInvalid("this campaign already has a recipe called " + name)
	}
	return errInternal(err)
}

func (s *service) Create(ctx context.Context, campaignID string, v Viewer, in RecipeInput) (*Recipe, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	name, doc, err := s.check(ctx, campaignID, in)
	if err != nil {
		return nil, err
	}
	n, err := s.repo.Count(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	if n >= MaxRecipes {
		return nil, errInvalid(fmt.Sprintf("a campaign holds at most %d recipes", MaxRecipes))
	}
	r := Recipe{ID: s.newID(), CampaignID: campaignID, Name: name, Doc: doc, CreatedBy: v.UserID}
	if err := s.repo.Create(ctx, r); err != nil {
		return nil, dupName(err, name)
	}
	return s.Get(ctx, campaignID, r.ID, v)
}

func (s *service) Update(ctx context.Context, campaignID, id string, v Viewer, in RecipeInput) (*Recipe, error) {
	if err := requireDirector(v); err != nil {
		return nil, err
	}
	cur, err := s.get(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if in.Version != cur.Version {
		return nil, errConflict()
	}
	name, doc, err := s.check(ctx, campaignID, in)
	if err != nil {
		return nil, err
	}
	cur.Name, cur.Doc = name, doc
	ok, err := s.repo.Update(ctx, cur, in.Version)
	if err != nil {
		return nil, dupName(err, name)
	}
	if !ok {
		return nil, errConflict()
	}
	return s.Get(ctx, campaignID, id, v)
}

func (s *service) Delete(ctx context.Context, campaignID, id string, v Viewer) error {
	if err := requireDirector(v); err != nil {
		return err
	}
	if !validToken(id) {
		return errNotFound("recipe")
	}
	ok, err := s.repo.Delete(ctx, campaignID, id)
	if err != nil {
		return errInternal(err)
	}
	if !ok {
		return errNotFound("recipe")
	}
	return nil
}
