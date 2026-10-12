package encounters

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Service owns the encounter rules. Every method is for the DM: a viewer
// below Owner visibility is refused before anything is read, so a planned
// fight never reaches a player through any route.
type Service interface {
	// ListForPage returns a page's encounters for its widget.
	ListForPage(ctx context.Context, campaignID, entityID string, v Viewer) (*PageList, error)
	// Get returns one encounter.
	Get(ctx context.Context, campaignID, id string, v Viewer) (*Encounter, error)
	// Create adds an encounter to a page.
	Create(ctx context.Context, campaignID, entityID string, v Viewer, in SaveInput) (*Encounter, error)
	// Update replaces an encounter; in.Version must be the stored version.
	Update(ctx context.Context, campaignID, id string, v Viewer, in SaveInput) (*Encounter, error)
	// Delete removes an encounter.
	Delete(ctx context.Context, campaignID, id string, v Viewer) error
	// Search lists the campaign's encounters for Foundry: ready ones first,
	// then the most recently changed, optionally matching q.
	Search(ctx context.Context, campaignID, q string, limit int, v Viewer) (items []Summary, total int, err error)
}

// Search limits: the default fills Foundry's list, the cap bounds one call.
const (
	DefaultSearchLimit = 50
	MaxSearchLimit     = 100
)

type service struct {
	repo  Repository
	pages PageDirectory
}

// NewService builds the service.
func NewService(repo Repository, pages PageDirectory) Service {
	return &service{repo: repo, pages: pages}
}

// isDM is the rule DM-only blocks use on a page: Owner visibility, which an
// owner and a member with DM access both have.
func isDM(v Viewer) bool { return v.VisibilityRole >= permissions.RoleOwner }

func requireDM(v Viewer) error {
	if !isDM(v) {
		return errForbidden("encounters are for the DM")
	}
	return nil
}

// access decides the page exists in the campaign and the viewer may see it.
// A hidden page is reported exactly like a missing one.
func (s *service) access(ctx context.Context, campaignID, entityID string, v Viewer) (PageAccess, error) {
	a, err := s.pages.Access(ctx, campaignID, entityID, v)
	if err != nil {
		return PageAccess{}, apperrorInternal(err)
	}
	if !a.Exists || !a.CanView {
		return PageAccess{}, errNotFound("page")
	}
	return a, nil
}

func (s *service) editable(ctx context.Context, campaignID, entityID string, v Viewer) error {
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return err
	}
	if !a.CanEdit {
		return errForbidden("you can't edit this page")
	}
	return nil
}

// checkPages refuses a hero or creature page that is not a live page of
// this campaign, so a saved encounter never points across campaigns.
func (s *service) checkPages(ctx context.Context, campaignID string, d doc, v Viewer) error {
	ids := append([]string{}, d.Heroes...)
	for _, c := range d.Creatures {
		if c.EntityID != "" {
			ids = append(ids, c.EntityID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	names, err := s.pages.Names(ctx, campaignID, ids, v)
	if err != nil {
		return apperrorInternal(err)
	}
	for _, id := range d.Heroes {
		if _, ok := names[id]; !ok {
			return errInvalid("a hero is not a page in this campaign")
		}
	}
	for _, c := range d.Creatures {
		if _, ok := names[c.EntityID]; c.EntityID != "" && !ok {
			return errInvalid(c.Name + " is not a page in this campaign")
		}
	}
	return nil
}

// decode reads a stored document. It passed validation when saved; a row
// that no longer parses is an internal error, never sent half-read.
func decode(r Row) (doc, error) {
	var d doc
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return doc{}, apperrorInternal(err)
	}
	return d, nil
}

// present turns rows into encounters, naming heroes and pages from the
// directory in one call. A hero or creature page that has since gone is
// dropped from heroes and unlinked from its row, which keeps its name.
func (s *service) present(ctx context.Context, campaignID string, rows []Row, v Viewer) ([]Encounter, error) {
	docs := make([]doc, len(rows))
	var ids []string
	for i, r := range rows {
		d, err := decode(r)
		if err != nil {
			return nil, err
		}
		docs[i] = d
		ids = append(ids, r.EntityID)
		ids = append(ids, d.Heroes...)
		for _, c := range d.Creatures {
			if c.EntityID != "" {
				ids = append(ids, c.EntityID)
			}
		}
	}
	names := map[string]string{}
	if len(ids) > 0 {
		var err error
		if names, err = s.pages.Names(ctx, campaignID, ids, v); err != nil {
			return nil, apperrorInternal(err)
		}
	}
	out := make([]Encounter, len(rows))
	for i, r := range rows {
		e := Encounter{
			ID: r.ID, Page: Page{ID: r.EntityID, Name: names[r.EntityID]}, Name: r.Name, Ready: r.Ready,
			Heroes: []Hero{}, Creatures: []Creature{}, Version: r.Version, UpdatedAt: r.UpdatedAt,
		}
		for _, id := range docs[i].Heroes {
			if n, ok := names[id]; ok {
				e.Heroes = append(e.Heroes, Hero{ID: id, Name: n})
			}
		}
		for _, c := range docs[i].Creatures {
			if _, ok := names[c.EntityID]; c.EntityID != "" && !ok {
				c.EntityID = ""
			}
			e.Creatures = append(e.Creatures, c)
		}
		out[i] = e
	}
	return out, nil
}

func (s *service) presentOne(ctx context.Context, campaignID string, r Row, v Viewer) (*Encounter, error) {
	out, err := s.present(ctx, campaignID, []Row{r}, v)
	if err != nil {
		return nil, err
	}
	return &out[0], nil
}

func (s *service) ListForPage(ctx context.Context, campaignID, entityID string, v Viewer) (*PageList, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	if !validToken(entityID) {
		return nil, errNotFound("page")
	}
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListForPage(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	list, err := s.present(ctx, campaignID, rows, v)
	if err != nil {
		return nil, err
	}
	return &PageList{Encounters: list, CanEdit: a.CanEdit}, nil
}

// load reads an encounter the viewer may see: DM, and its page viewable.
func (s *service) load(ctx context.Context, campaignID, id string, v Viewer) (Row, error) {
	if err := requireDM(v); err != nil {
		return Row{}, err
	}
	if !validToken(id) {
		return Row{}, errNotFound("encounter")
	}
	r, found, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return Row{}, apperrorInternal(err)
	}
	if !found {
		return Row{}, errNotFound("encounter")
	}
	if _, err := s.access(ctx, campaignID, r.EntityID, v); err != nil {
		return Row{}, errNotFound("encounter")
	}
	return r, nil
}

func (s *service) Get(ctx context.Context, campaignID, id string, v Viewer) (*Encounter, error) {
	r, err := s.load(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	return s.presentOne(ctx, campaignID, r, v)
}

func (s *service) Create(ctx context.Context, campaignID, entityID string, v Viewer, in SaveInput) (*Encounter, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	if !validToken(entityID) {
		return nil, errNotFound("page")
	}
	if err := s.editable(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	name, d, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	if err := s.checkPages(ctx, campaignID, d, v); err != nil {
		return nil, err
	}
	existing, err := s.repo.ListForPage(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if len(existing) >= MaxPerPage {
		return nil, errInvalid("this page already has the most encounters it can hold; put new ones on another page")
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	r := Row{ID: uuid.NewString(), CampaignID: campaignID, EntityID: entityID, Name: name, Ready: in.Ready, Data: raw}
	if err := s.repo.Create(ctx, r, v.UserID); err != nil {
		return nil, apperrorInternal(err)
	}
	saved, found, err := s.repo.Get(ctx, campaignID, r.ID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, errNotFound("encounter")
	}
	return s.presentOne(ctx, campaignID, saved, v)
}

func (s *service) Update(ctx context.Context, campaignID, id string, v Viewer, in SaveInput) (*Encounter, error) {
	r, err := s.load(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	if err := s.editable(ctx, campaignID, r.EntityID, v); err != nil {
		return nil, err
	}
	if in.Version == nil {
		return nil, errInvalid("version is required")
	}
	if *in.Version != r.Version {
		return nil, errConflict()
	}
	name, d, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	if err := s.checkPages(ctx, campaignID, d, v); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ok, err := s.repo.Update(ctx, campaignID, id, name, in.Ready, raw, r.Version, v.UserID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	saved, found, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, errNotFound("encounter")
	}
	return s.presentOne(ctx, campaignID, saved, v)
}

func (s *service) Delete(ctx context.Context, campaignID, id string, v Viewer) error {
	r, err := s.load(ctx, campaignID, id, v)
	if err != nil {
		return err
	}
	if err := s.editable(ctx, campaignID, r.EntityID, v); err != nil {
		return err
	}
	if _, err := s.repo.Delete(ctx, campaignID, id); err != nil {
		return apperrorInternal(err)
	}
	return nil
}

func (s *service) Search(ctx context.Context, campaignID, q string, limit int, v Viewer) ([]Summary, int, error) {
	if err := requireDM(v); err != nil {
		return nil, 0, err
	}
	if limit < 1 {
		limit = DefaultSearchLimit
	}
	limit = min(limit, MaxSearchLimit)
	q, err := cleanLine("search", q, MaxNameChars)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.Search(ctx, campaignID, q, limit)
	if err != nil {
		return nil, 0, apperrorInternal(err)
	}
	// A page the viewer may not open hides its encounters too.
	pageIDs := make([]string, len(rows))
	for i, r := range rows {
		pageIDs[i] = r.EntityID
	}
	visible := map[string]string{}
	if len(pageIDs) > 0 {
		if visible, err = s.pages.Names(ctx, campaignID, pageIDs, v); err != nil {
			return nil, 0, apperrorInternal(err)
		}
	}
	out := make([]Summary, 0, len(rows))
	for _, r := range rows {
		if _, ok := visible[r.EntityID]; !ok {
			total--
			continue
		}
		d, err := decode(r)
		if err != nil {
			return nil, 0, err
		}
		n := 0
		for _, c := range d.Creatures {
			n += c.Count
		}
		out = append(out, Summary{
			ID: r.ID, Page: Page{ID: r.EntityID, Name: r.PageName}, Name: r.Name,
			Ready: r.Ready, CreatureCount: n, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, total, nil
}
