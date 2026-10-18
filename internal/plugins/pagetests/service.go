package pagetests

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Service owns the test rules. Writing is the DM's alone; reading a page's
// tests needs only the page, but anyone below the DM is told only about
// tests marked shown, and only their name and recorded result.
type Service interface {
	// ListForPage returns a page's tests as the viewer may see them.
	ListForPage(ctx context.Context, campaignID, entityID string, v Viewer) (*PageList, error)
	// Create adds a test to a page.
	Create(ctx context.Context, campaignID, entityID string, v Viewer, in SaveInput) (*Test, error)
	// Update replaces a test's setup; the tally and the record are kept.
	Update(ctx context.Context, campaignID, id string, v Viewer, in SaveInput) (*Test, error)
	// Tally moves one side of a test's tally by one step.
	Tally(ctx context.Context, campaignID, id string, v Viewer, in TallyInput) (*Test, error)
	// Record records a result with today's calendar date, or clears it.
	Record(ctx context.Context, campaignID, id string, v Viewer, in RecordInput) (*Test, error)
	// Delete removes a test.
	Delete(ctx context.Context, campaignID, id string, v Viewer) error
	// Choices lists the campaign's tests for the encounter builder.
	Choices(ctx context.Context, campaignID string, v Viewer) ([]Choice, error)
	// Outcomes returns, for the tests among ids the DM may see, their name,
	// results and recorded result, keyed by id.
	Outcomes(ctx context.Context, campaignID string, ids []string, v Viewer) (map[string]Outcome, error)
}

type service struct {
	repo  Repository
	pages PageDirectory
	cal   Calendar
	now   func() time.Time
}

// NewService builds the service. cal may be nil, and a record then carries
// no date.
func NewService(repo Repository, pages PageDirectory, cal Calendar) Service {
	return &service{repo: repo, pages: pages, cal: cal, now: time.Now}
}

func requireDM(v Viewer) error {
	if !v.IsDM() {
		return errForbidden("tests are changed by the DM")
	}
	return nil
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

// decode reads a stored document. It passed validation when saved; a row
// that no longer parses is an internal error, never sent half-read.
func decode(r Row) (doc, error) {
	var d doc
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return doc{}, apperrorInternal(err)
	}
	if d.Results == nil {
		d.Results = []Result{}
	}
	return d, nil
}

// present is a test for the DM: everything, with the result on offer.
func present(r Row, d doc, pageName string) Test {
	return Test{
		ID: r.ID, Page: Page{ID: r.EntityID, Name: pageName}, Name: r.Name, Kind: d.Kind, Shown: r.Shown,
		Needed: d.Needed, Limit: d.Limit, Successes: d.Successes, Failures: d.Failures,
		Results: d.Results, Recorded: d.Recorded, Offer: offer(d), Version: r.Version, UpdatedAt: r.UpdatedAt,
	}
}

// presentToPlayer is a shown test for anyone below the DM: its name, kind
// and recorded result, and no other result, total or count.
func presentToPlayer(r Row, d doc) Test {
	t := Test{ID: r.ID, Page: Page{ID: r.EntityID}, Name: r.Name, Kind: d.Kind, Shown: true, Results: []Result{}}
	if d.Recorded != nil {
		t.Recorded = &Recorded{Result: d.Recorded.Result, Label: d.Recorded.Label, DateLabel: d.Recorded.DateLabel}
	}
	return t
}

func (s *service) ListForPage(ctx context.Context, campaignID, entityID string, v Viewer) (*PageList, error) {
	a, err := s.access(ctx, campaignID, entityID, v)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListForPage(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	out := &PageList{Tests: []Test{}, CanEdit: v.IsDM() && a.CanEdit}
	for _, r := range rows {
		if !v.IsDM() && !r.Shown {
			continue
		}
		d, err := decode(r)
		if err != nil {
			return nil, err
		}
		if v.IsDM() {
			out.Tests = append(out.Tests, present(r, d, ""))
		} else {
			out.Tests = append(out.Tests, presentToPlayer(r, d))
		}
	}
	return out, nil
}

// load reads a test the DM may change: the DM, and its page editable.
func (s *service) load(ctx context.Context, campaignID, id string, v Viewer) (Row, doc, error) {
	if err := requireDM(v); err != nil {
		return Row{}, doc{}, err
	}
	if !validToken(id) {
		return Row{}, doc{}, errNotFound("test")
	}
	r, found, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return Row{}, doc{}, apperrorInternal(err)
	}
	if !found {
		return Row{}, doc{}, errNotFound("test")
	}
	a, err := s.pages.Access(ctx, campaignID, r.EntityID, v)
	if err != nil {
		return Row{}, doc{}, apperrorInternal(err)
	}
	if !a.Exists || !a.CanView {
		return Row{}, doc{}, errNotFound("test")
	}
	if !a.CanEdit {
		return Row{}, doc{}, errForbidden("you can't edit this page")
	}
	d, err := decode(r)
	if err != nil {
		return Row{}, doc{}, err
	}
	return r, d, nil
}

// save writes d over r if r's version is still the stored one, and returns
// the test as saved.
func (s *service) save(ctx context.Context, r Row, name string, shown bool, d doc, v Viewer) (*Test, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	ok, err := s.repo.Update(ctx, r.CampaignID, r.ID, name, shown, raw, r.Version, v.UserID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	return s.reread(ctx, r.CampaignID, r.ID)
}

func (s *service) reread(ctx context.Context, campaignID, id string) (*Test, error) {
	saved, found, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if !found {
		return nil, errNotFound("test")
	}
	d, err := decode(saved)
	if err != nil {
		return nil, err
	}
	t := present(saved, d, "")
	return &t, nil
}

func checkVersion(want *int, r Row) error {
	if want == nil {
		return errInvalid("version is required")
	}
	if *want != r.Version {
		return errConflict()
	}
	return nil
}

func (s *service) Create(ctx context.Context, campaignID, entityID string, v Viewer, in SaveInput) (*Test, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	if err := s.editable(ctx, campaignID, entityID, v); err != nil {
		return nil, err
	}
	name, d, err := validateSetup(in)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.ListForPage(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	if len(existing) >= MaxPerPage {
		return nil, errInvalid("this page already has the most tests it can hold; put new ones on another page")
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	r := Row{ID: uuid.NewString(), CampaignID: campaignID, EntityID: entityID, Name: name, Shown: in.Shown, Data: raw}
	if err := s.repo.Create(ctx, r, v.UserID); err != nil {
		return nil, apperrorInternal(err)
	}
	return s.reread(ctx, campaignID, r.ID)
}

func (s *service) Update(ctx context.Context, campaignID, id string, v Viewer, in SaveInput) (*Test, error) {
	r, prev, err := s.load(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	if err := checkVersion(in.Version, r); err != nil {
		return nil, err
	}
	name, d, err := validateSetup(in)
	if err != nil {
		return nil, err
	}
	keepTally(&d, prev)
	return s.save(ctx, r, name, in.Shown, d, v)
}

func (s *service) Tally(ctx context.Context, campaignID, id string, v Viewer, in TallyInput) (*Test, error) {
	r, d, err := s.load(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	if err := checkVersion(in.Version, r); err != nil {
		return nil, err
	}
	if err := step(&d, in.Side, in.Step); err != nil {
		return nil, err
	}
	return s.save(ctx, r, r.Name, r.Shown, d, v)
}

func (s *service) Record(ctx context.Context, campaignID, id string, v Viewer, in RecordInput) (*Test, error) {
	r, d, err := s.load(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	if err := checkVersion(in.Version, r); err != nil {
		return nil, err
	}
	if in.Reset {
		d.Successes, d.Failures = 0, 0
	}
	if in.Result == "" {
		d.Recorded = nil
		return s.save(ctx, r, r.Name, r.Shown, d, v)
	}
	var picked *Result
	for i := range d.Results {
		if d.Results[i].ID == in.Result {
			picked = &d.Results[i]
		}
	}
	if picked == nil {
		return nil, errInvalid("that result is not one of this test's results")
	}
	rec := &Recorded{Result: picked.ID, Label: picked.Label, At: s.now().UTC()}
	if s.cal != nil {
		// A calendar that cannot be read must not stop the table recording
		// a result, so the record is kept without a date and the failure
		// logged.
		day, label, ok, err := s.cal.Today(ctx, campaignID)
		switch {
		case err != nil:
			slog.Warn("page tests: cannot read the calendar for a record",
				slog.String("campaign_id", campaignID), slog.Any("error", err))
		case ok:
			rec.Date, rec.DateLabel = &day, label
		}
	}
	d.Recorded = rec
	return s.save(ctx, r, r.Name, r.Shown, d, v)
}

func (s *service) Delete(ctx context.Context, campaignID, id string, v Viewer) error {
	r, _, err := s.load(ctx, campaignID, id, v)
	if err != nil {
		return err
	}
	if _, err := s.repo.Delete(ctx, campaignID, r.ID); err != nil {
		return apperrorInternal(err)
	}
	return nil
}

// visible keeps the rows whose page the viewer may see, and names the pages.
func (s *service) visible(ctx context.Context, campaignID string, rows []Row, v Viewer) ([]Row, map[string]string, error) {
	if len(rows) == 0 {
		return nil, map[string]string{}, nil
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.EntityID
	}
	names, err := s.pages.Names(ctx, campaignID, ids, v)
	if err != nil {
		return nil, nil, apperrorInternal(err)
	}
	out := rows[:0:0]
	for _, r := range rows {
		if _, ok := names[r.EntityID]; ok {
			out = append(out, r)
		}
	}
	return out, names, nil
}

func (s *service) Choices(ctx context.Context, campaignID string, v Viewer) ([]Choice, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListForCampaign(ctx, campaignID, MaxChoices)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	rows, names, err := s.visible(ctx, campaignID, rows, v)
	if err != nil {
		return nil, err
	}
	out := make([]Choice, 0, len(rows))
	for _, r := range rows {
		d, err := decode(r)
		if err != nil {
			return nil, err
		}
		out = append(out, Choice{ID: r.ID, Page: Page{ID: r.EntityID, Name: names[r.EntityID]}, Name: r.Name, Results: d.Results})
	}
	return out, nil
}

func (s *service) Outcomes(ctx context.Context, campaignID string, ids []string, v Viewer) (map[string]Outcome, error) {
	if err := requireDM(v); err != nil {
		return nil, err
	}
	want := make([]string, 0, len(ids))
	for _, id := range ids {
		if validToken(id) {
			want = append(want, id)
		}
	}
	out := map[string]Outcome{}
	if len(want) == 0 {
		return out, nil
	}
	rows, err := s.repo.GetMany(ctx, campaignID, want)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	rows, _, err = s.visible(ctx, campaignID, rows, v)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		d, err := decode(r)
		if err != nil {
			return nil, err
		}
		o := Outcome{Name: r.Name, Results: d.Results}
		if d.Recorded != nil {
			o.Recorded = d.Recorded.Result
		}
		out[r.ID] = o
	}
	return out, nil
}
