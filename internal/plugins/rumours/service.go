package rumours

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Service is the Rumours block's business logic. Every method takes the
// viewer and decides here what that viewer may be told and may do; handlers,
// the block and the script add no rule of their own.
//
// The rules, in one place:
//   - Everyone who can open the page reads its rumours and what the party
//     thinks. Only the Director is ever sent what is true.
//   - The Director and Scribes add rumours and set what the party thinks.
//   - Only the Director changes what is true.
//   - The Director edits or removes any rumour; a Scribe, only their own.
type Service interface {
	// View is the whole block for one viewer.
	View(ctx context.Context, campaignID, entityID string, v Viewer) (*View, error)
	// Add writes a rumour. Director and Scribes.
	Add(ctx context.Context, in AddInput) (*RumourView, error)
	// Update edits a rumour's words. Director, or the Scribe who wrote it.
	Update(ctx context.Context, campaignID, entityID, rumourID string, in UpdateRumourInput, v Viewer) (*RumourView, error)
	// SetTruth records what is true. Director only.
	SetTruth(ctx context.Context, campaignID, entityID, rumourID, truth string, v Viewer) (*RumourView, error)
	// SetBelief records what the party thinks. Director and Scribes.
	SetBelief(ctx context.Context, campaignID, entityID, rumourID, belief string, v Viewer) (*RumourView, error)
	// Remove deletes a rumour. Director, or the Scribe who wrote it.
	Remove(ctx context.Context, campaignID, entityID, rumourID string, v Viewer) error
}

type service struct {
	repo Repository
	now  func() time.Time
}

// NewService creates the Rumours service.
func NewService(repo Repository) Service {
	return &service{repo: repo, now: time.Now}
}

// whenText words the day a rumour was heard. The year appears only when it is
// not this one, so a page of recent rumours stays short.
func whenText(at, now time.Time) string {
	at, now = at.UTC(), now.UTC()
	if at.Year() == now.Year() {
		return at.Format("2 Jan")
	}
	return at.Format("2 Jan 2006")
}

// someoneAtTheTable stands in for a rumour whose speaker was left blank.
const someoneAtTheTable = "Someone at the table"

// viewOf shapes one stored rumour for one viewer. Truth is attached only for
// the Director, whatever the repository handed over, so a repository that
// ever returned it for someone else still could not leak it.
func (s *service) viewOf(x Rumour, v Viewer) RumourView {
	out := RumourView{
		ID: x.ID, Quote: x.Quote, Who: x.Source, Sure: x.Sure, When: whenText(x.CreatedAt, s.now()),
		Belief: x.Belief, CanEdit: canEdit(x, v),
	}
	if out.Who == "" {
		out.Who = someoneAtTheTable
	}
	switch {
	case x.ByDM:
	case x.CreatedBy != "" && x.CreatedBy == v.UserID:
		out.AddedBy = "you"
	case x.AuthorName != "":
		out.AddedBy = x.AuthorName
	default:
		out.AddedBy = "a player"
	}
	if x.Belief != BeliefNone {
		switch {
		case x.BeliefByDM:
			out.BeliefBy = "the DM"
		case x.BeliefName != "":
			out.BeliefBy = x.BeliefName
		default:
			out.BeliefBy = "a player"
		}
	}
	if v.IsDM() {
		t := x.Truth
		if t == "" {
			t = TruthOpen
		}
		out.Truth = &t
	}
	return out
}

// canEdit is the one place that says who may change or remove a rumour.
func canEdit(x Rumour, v Viewer) bool {
	if v.IsDM() {
		return true
	}
	return v.IsScribe() && v.UserID != "" && x.CreatedBy == v.UserID
}

func (s *service) View(ctx context.Context, campaignID, entityID string, v Viewer) (*View, error) {
	rows, total, err := s.repo.List(ctx, campaignID, entityID, v.IsDM(), MaxRumours)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	// A viewer who can neither read anything here nor add anything has no
	// use for the block, and an empty one would only be a gap on the page.
	if len(rows) == 0 && !v.CanAdd() {
		return &View{Hidden: true, Rumours: []RumourView{}}, nil
	}
	out := &View{IsDM: v.IsDM(), CanAdd: v.CanAdd(), CanMark: v.CanMark(), Total: total, Rumours: make([]RumourView, 0, len(rows))}
	for _, x := range rows {
		out.Rumours = append(out.Rumours, s.viewOf(x, v))
	}
	return out, nil
}

func validSure(n int) bool { return n >= SureMin && n <= SureMax }

// checkWords cleans and checks the words of a rumour.
func checkWords(quote, who string, sure int) (string, string, error) {
	q, ok := cleanText(quote, QuoteMax)
	if q == "" {
		if !ok {
			return "", "", apperror.NewValidation("Use plain text for the rumour.")
		}
		return "", "", apperror.NewValidation("Write what was said first.")
	}
	if !ok {
		return "", "", apperror.NewValidation("Keep the rumour to 280 characters or fewer.")
	}
	w, ok := cleanText(who, SourceMax)
	if !ok {
		return "", "", apperror.NewValidation("Keep who said it to 80 characters or fewer, in plain text.")
	}
	if !validSure(sure) {
		return "", "", apperror.NewValidation("Choose how sure they were.")
	}
	return q, w, nil
}

func (s *service) Add(ctx context.Context, in AddInput) (*RumourView, error) {
	if !in.Viewer.CanAdd() {
		return nil, apperror.NewForbidden("only the Director and Scribes can add rumours")
	}
	sure := in.Sure
	if sure == 0 {
		sure = SureDefault
	}
	q, w, err := checkWords(in.Quote, in.Who, sure)
	if err != nil {
		return nil, err
	}
	x := Rumour{
		ID: uuid.NewString(), CampaignID: in.CampaignID, EntityID: in.EntityID,
		Quote: q, Source: w, Sure: sure, ByDM: in.Viewer.IsDM(), CreatedBy: in.Viewer.UserID,
	}
	switch err := s.repo.Insert(ctx, x, MaxRumours); {
	case errors.Is(err, errPageFull):
		return nil, apperror.NewValidation("This page already holds 200 rumours. Remove some before adding more.")
	case errors.Is(err, errNoPage):
		return nil, apperror.NewNotFound("page not found")
	case err != nil:
		return nil, apperror.NewInternal(err)
	}
	return s.fresh(ctx, in.CampaignID, in.EntityID, x.ID, in.Viewer)
}

// fresh re-reads a rumour for the viewer after a write, so the answer is what
// is stored and shaped for them, never what the request said.
func (s *service) fresh(ctx context.Context, campaignID, entityID, id string, v Viewer) (*RumourView, error) {
	x, err := s.repo.Get(ctx, campaignID, entityID, id, v.IsDM())
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if x == nil {
		return nil, apperror.NewNotFound("rumour not found")
	}
	out := s.viewOf(*x, v)
	return &out, nil
}

// find loads a rumour for a write. The truth column is never needed to decide
// one, so it is never read here.
func (s *service) find(ctx context.Context, campaignID, entityID, id string) (*Rumour, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, apperror.NewNotFound("rumour not found")
	}
	x, err := s.repo.Get(ctx, campaignID, entityID, id, false)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if x == nil {
		return nil, apperror.NewNotFound("rumour not found")
	}
	return x, nil
}

func (s *service) Update(ctx context.Context, campaignID, entityID, rumourID string, in UpdateRumourInput, v Viewer) (*RumourView, error) {
	if !v.CanAdd() {
		return nil, apperror.NewForbidden("only the Director and Scribes can edit rumours")
	}
	cur, err := s.find(ctx, campaignID, entityID, rumourID)
	if err != nil {
		return nil, err
	}
	if !canEdit(*cur, v) {
		return nil, apperror.NewForbidden("a Scribe can edit only the rumours they wrote")
	}
	// Validators read the merged value, not the raw input: an absent quote is
	// not an empty one.
	sure := in.Sure.Val(cur.Sure)
	q, w, err := checkWords(in.Quote.Val(cur.Quote), in.Who.Val(cur.Source), sure)
	if err != nil {
		return nil, err
	}
	ok, err := s.repo.UpdateText(ctx, campaignID, entityID, rumourID, q, w, sure)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !ok {
		return nil, apperror.NewNotFound("rumour not found")
	}
	return s.fresh(ctx, campaignID, entityID, rumourID, v)
}

func (s *service) SetTruth(ctx context.Context, campaignID, entityID, rumourID, truth string, v Viewer) (*RumourView, error) {
	if !v.IsDM() {
		return nil, apperror.NewForbidden("only the Director can say what is true")
	}
	switch truth {
	case TruthTrue, TruthFalse, TruthOpen:
	default:
		return nil, apperror.NewValidation("Choose true, false or open.")
	}
	if _, err := s.find(ctx, campaignID, entityID, rumourID); err != nil {
		return nil, err
	}
	ok, err := s.repo.SetTruth(ctx, campaignID, entityID, rumourID, truth)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !ok {
		return nil, apperror.NewNotFound("rumour not found")
	}
	return s.fresh(ctx, campaignID, entityID, rumourID, v)
}

func (s *service) SetBelief(ctx context.Context, campaignID, entityID, rumourID, belief string, v Viewer) (*RumourView, error) {
	if !v.CanMark() {
		return nil, apperror.NewForbidden("only the Director and Scribes can set what the party thinks")
	}
	switch belief {
	case BeliefTrue, BeliefFalse, BeliefUnsure:
	default:
		return nil, apperror.NewValidation("Choose true, false or unsure.")
	}
	if _, err := s.find(ctx, campaignID, entityID, rumourID); err != nil {
		return nil, err
	}
	ok, err := s.repo.SetBelief(ctx, campaignID, entityID, rumourID, belief, v.UserID, v.IsDM())
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !ok {
		return nil, apperror.NewNotFound("rumour not found")
	}
	return s.fresh(ctx, campaignID, entityID, rumourID, v)
}

func (s *service) Remove(ctx context.Context, campaignID, entityID, rumourID string, v Viewer) error {
	if !v.CanAdd() {
		return apperror.NewForbidden("only the Director and Scribes can remove rumours")
	}
	cur, err := s.find(ctx, campaignID, entityID, rumourID)
	if err != nil {
		return err
	}
	if !canEdit(*cur, v) {
		return apperror.NewForbidden("a Scribe can remove only the rumours they wrote")
	}
	ok, err := s.repo.Delete(ctx, campaignID, entityID, rumourID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok {
		return apperror.NewNotFound("rumour not found")
	}
	return nil
}
