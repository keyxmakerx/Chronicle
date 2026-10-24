package encounters

import (
	"context"
	"encoding/json"
	"log/slog"

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
	repo    Repository
	pages   PageDirectory
	results ResultDirectory
}

// NewService builds the service. results may be nil when the Tests block is
// unavailable: versions still save and play, but none can be picked by a
// result.
func NewService(repo Repository, pages PageDirectory, results ResultDirectory) Service {
	return &service{repo: repo, pages: pages, results: results}
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
	rows := allRows(d)
	ids := append([]string{}, d.Heroes...)
	for _, c := range rows {
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
	for _, c := range rows {
		if _, ok := names[c.EntityID]; c.EntityID != "" && !ok {
			return errInvalid(c.Name + " is not a page in this campaign")
		}
	}
	return nil
}

// pickTests lists the tests the versions' picks name.
func pickTests(versions []Version) []string {
	var ids []string
	for _, v := range versions {
		if v.Pick != nil {
			ids = append(ids, v.Pick.Test)
		}
	}
	return ids
}

// checkPicks refuses a pick whose test is not a test of this campaign the
// DM may see, or whose result is not one of that test's results.
func (s *service) checkPicks(ctx context.Context, campaignID string, d doc, v Viewer) error {
	ids := pickTests(d.Versions)
	if len(ids) == 0 {
		return nil
	}
	if s.results == nil {
		return errInvalid("tests are not available, so a version can't be picked by a result")
	}
	outs, err := s.results.Outcomes(ctx, campaignID, ids, v)
	if err != nil {
		return apperrorInternal(err)
	}
	for _, ver := range d.Versions {
		if ver.Pick == nil {
			continue
		}
		o, ok := outs[ver.Pick.Test]
		if !ok {
			return errInvalid(ver.Name + ": the test that picks it is not in this campaign")
		}
		if _, ok := o.Results[ver.Pick.Result]; !ok {
			return errInvalid(ver.Name + ": the result that picks it is not one of " + o.Name + "'s results")
		}
	}
	return nil
}

// outcomes reads the tests the given versions' picks name. A failure to read
// them must not take the encounter down with it, so it reads as unknown
// (nil): the picks are kept as stored rather than cleared, so a save made
// while the tests cannot be read never drops them.
func (s *service) outcomes(ctx context.Context, campaignID string, ids []string, v Viewer) map[string]TestOutcome {
	if s.results == nil || len(ids) == 0 {
		return map[string]TestOutcome{}
	}
	outs, err := s.results.Outcomes(ctx, campaignID, ids, v)
	if err != nil {
		slog.Warn("encounters: cannot read the tests that pick versions",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	if outs == nil {
		outs = map[string]TestOutcome{}
	}
	return outs
}

// readVersions names each pick's test and result, and clears a pick whose
// test or result has gone, the way an unlinked row keeps its name: the
// version stays and the DM picks it by hand. It returns which version the
// recorded results choose. With outs nil (the tests could not be read) the
// picks stay as stored, unnamed, and there is no answer to give.
func readVersions(in []Version, outs map[string]TestOutcome) ([]Version, *Picked) {
	if len(in) == 0 {
		return []Version{}, nil
	}
	if outs == nil {
		out := make([]Version, 0, len(in))
		for _, ver := range in {
			ver.Add = nonNilRows(ver.Add)
			if ver.Remove == nil {
				ver.Remove = []string{}
			}
			if ver.Counts == nil {
				ver.Counts = []CountChange{}
			}
			if ver.Pick != nil {
				ver.Pick = &Pick{Test: ver.Pick.Test, Result: ver.Pick.Result}
			}
			out = append(out, ver)
		}
		return out, nil
	}
	picked := &Picked{Open: []string{}}
	open := map[string]bool{}
	out := make([]Version, 0, len(in))
	for _, ver := range in {
		ver.Add = nonNilRows(ver.Add)
		if ver.Remove == nil {
			ver.Remove = []string{}
		}
		if ver.Counts == nil {
			ver.Counts = []CountChange{}
		}
		if ver.Pick != nil {
			o, ok := outs[ver.Pick.Test]
			label, has := o.Results[ver.Pick.Result]
			if !ok || !has {
				ver.Pick = nil
			} else {
				ver.Pick = &Pick{Test: ver.Pick.Test, Result: ver.Pick.Result, TestName: o.Name, ResultLabel: label}
				switch {
				case o.Recorded == ver.Pick.Result && picked.Version == "":
					picked.Version, picked.Test, picked.Result = ver.ID, o.Name, label
				case o.Recorded == "" && !open[ver.Pick.Test]:
					open[ver.Pick.Test] = true
					picked.Open = append(picked.Open, o.Name)
				}
			}
		}
		out = append(out, ver)
	}
	return out, picked
}

func nonNilRows(in []Creature) []Creature {
	if in == nil {
		return []Creature{}
	}
	return in
}

// CarryVersions keeps an encounter's versions across a save that replaces
// its base rows: a removal or count change of a row that is gone is dropped,
// and the rest of the version, its added rows and its pick, is kept.
func CarryVersions(in []Version, rows []Creature) []Version {
	ids := make(map[string]bool, len(rows))
	for _, r := range rows {
		ids[r.ID] = true
	}
	out := make([]Version, 0, len(in))
	for _, ver := range in {
		remove := []string{}
		for _, id := range ver.Remove {
			if ids[id] {
				remove = append(remove, id)
			}
		}
		counts := []CountChange{}
		for _, cc := range ver.Counts {
			if ids[cc.Row] {
				counts = append(counts, cc)
			}
		}
		ver.Remove, ver.Counts = remove, counts
		if ver.Pick != nil {
			ver.Pick = &Pick{Test: ver.Pick.Test, Result: ver.Pick.Result}
		}
		out = append(out, ver)
	}
	return out
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
	var ids, tests []string
	for i, r := range rows {
		d, err := decode(r)
		if err != nil {
			return nil, err
		}
		docs[i] = d
		ids = append(ids, r.EntityID)
		ids = append(ids, d.Heroes...)
		for _, c := range allRows(d) {
			if c.EntityID != "" {
				ids = append(ids, c.EntityID)
			}
		}
		tests = append(tests, pickTests(d.Versions)...)
	}
	outs := s.outcomes(ctx, campaignID, tests, v)
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
			Heroes: []Hero{}, Creatures: []Creature{}, Stages: []Stage{}, Versions: []Version{}, Version: r.Version, UpdatedAt: r.UpdatedAt,
		}
		for _, id := range docs[i].Heroes {
			if n, ok := names[id]; ok {
				e.Heroes = append(e.Heroes, Hero{ID: id, Name: n})
			}
		}
		unlink := func(c Creature) Creature {
			if _, ok := names[c.EntityID]; c.EntityID != "" && !ok {
				c.EntityID = ""
			}
			return c
		}
		for _, c := range docs[i].Creatures {
			e.Creatures = append(e.Creatures, unlink(c))
		}
		for _, ver := range docs[i].Versions {
			add := make([]Creature, 0, len(ver.Add))
			for _, c := range ver.Add {
				add = append(add, unlink(c))
			}
			ver.Add = add
			e.Versions = append(e.Versions, ver)
		}
		e.Versions, e.Picked = readVersions(e.Versions, outs)
		e.Stages = readStages(docs[i].Stages, allRows(doc{Creatures: e.Creatures, Versions: e.Versions}))
		out[i] = e
	}
	return out, nil
}

// readStages drops what a stage points at that is gone, the way an unlinked
// row keeps its name: a form change needs a row with a page, an arrival needs
// a row, and a Stamina trigger on a missing row falls back to manual. A stage
// emptied this way is kept so the DM sees it and decides.
// CarryStages keeps an encounter's stages across a save that replaces its
// rows: references to rows that are gone are dropped, and a stage left with
// nothing to do is dropped too, since a save would refuse it.
func CarryStages(in []Stage, rows []Creature) []Stage {
	out := []Stage{}
	for _, st := range readStages(in, rows) {
		if len(st.Forms) > 0 || len(st.Arrive) > 0 {
			out = append(out, st)
		}
	}
	return out
}

func readStages(in []Stage, rows []Creature) []Stage {
	byID := make(map[string]Creature, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]Stage, 0, len(in))
	for _, st := range in {
		if _, ok := byID[st.When.Row]; st.When.Kind == WhenStamina && !ok {
			st.When = StageWhen{Kind: WhenManual}
		}
		forms := []StageForm{}
		for _, f := range st.Forms {
			if r, ok := byID[f.Row]; ok && r.EntityID != "" {
				forms = append(forms, f)
			}
		}
		arrive := []string{}
		for _, id := range st.Arrive {
			if _, ok := byID[id]; ok {
				arrive = append(arrive, id)
			}
		}
		st.Forms, st.Arrive = forms, arrive
		out = append(out, st)
	}
	return out
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
	out := &PageList{Encounters: list, CanEdit: a.CanEdit, HeroTypeIDs: []int{}, CreatureTypeIDs: []int{}}
	// Best effort: without the types the pickers are empty, and a creature
	// can still be added by name.
	if pt, err := s.pages.PickerTypes(ctx, campaignID); err != nil {
		slog.Warn("encounters: picker types unavailable", slog.String("campaign_id", campaignID), slog.Any("error", err))
	} else {
		if pt.Heroes != nil {
			out.HeroTypeIDs = pt.Heroes
		}
		if pt.Creatures != nil {
			out.CreatureTypeIDs = pt.Creatures
		}
	}
	return out, nil
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
	if err := s.checkPicks(ctx, campaignID, d, v); err != nil {
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
	if err := s.checkPicks(ctx, campaignID, d, v); err != nil {
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
