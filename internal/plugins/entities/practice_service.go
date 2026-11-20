package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// practice_service.go is the rules of practice heroes: the cap, who may open
// or change one, the ask-and-answer, and what a valid pick is. It sees no
// request types. Whether someone may make practice heroes at all (their role,
// the claiming addon, a claimable hero type) is decided by the handler, which
// already knows the campaign's character types.

// practiceFullMessage is the refusal when the maker already holds the most.
const practiceFullMessage = "You have three practice heroes. Throw one away to start another."

// maxAskNoteLength bounds a Director's reason for declining.
const maxAskNoteLength = 500

// PracticeService keeps practice heroes.
type PracticeService interface {
	// Plan is the plan a practice hero is made from, for this maker.
	Plan(ctx context.Context, campaignID, userID string) (*HeroPlan, error)
	// ListMine returns the maker's practice heroes, oldest first.
	ListMine(ctx context.Context, campaignID, userID string) ([]PracticeHero, error)
	// ListAll returns every practice hero of the campaign, for Directors.
	ListAll(ctx context.Context, campaignID string, actor PracticeActor) ([]PracticeHero, error)
	// Open returns a practice hero to its maker or a Director. Anyone else is
	// told it does not exist, so its existence is not revealed.
	Open(ctx context.Context, campaignID, id string, actor PracticeActor) (*PracticeHero, error)
	// Create saves a new practice hero, or refuses past the cap.
	Create(ctx context.Context, campaignID string, actor PracticeActor, in PracticeInput) (*PracticeHero, error)
	// Update replaces the maker's own practice hero and clears any ask.
	Update(ctx context.Context, campaignID, id string, actor PracticeActor, in PracticeInput) (*PracticeHero, error)
	// Delete throws away the maker's own practice hero.
	Delete(ctx context.Context, campaignID, id string, actor PracticeActor) error
	// Ask marks the maker's practice hero as one they want to play.
	Ask(ctx context.Context, campaignID, id string, actor PracticeActor) error
	// Decline is a Director's "not now", with an optional note.
	Decline(ctx context.Context, campaignID, id string, actor PracticeActor, note string) (*PracticeHero, error)
	// Claim is a Director's yes: it takes the waiting practice hero out so it
	// can become the real hero, and returns it. Restore puts it back if making
	// the real hero failed.
	Claim(ctx context.Context, campaignID, id string, actor PracticeActor) (*PracticeHero, error)
	Restore(ctx context.Context, h *PracticeHero) error
}

type practiceService struct {
	repo    PracticeRepository
	planner HeroPlanner
}

// NewPracticeService creates the practice hero service. planner may be nil,
// in which case a practice hero has only a name.
func NewPracticeService(repo PracticeRepository, planner HeroPlanner) PracticeService {
	return &practiceService{repo: repo, planner: planner}
}

func (s *practiceService) Plan(ctx context.Context, campaignID, userID string) (*HeroPlan, error) {
	if s.planner == nil {
		return &HeroPlan{Steps: []HeroStep{}, Practice: true}, nil
	}
	// The maker's own limited plan, never the unlimited one: what the rules
	// held back is a subset of what this player may read.
	plan, err := s.planner.HeroPlan(ctx, campaignID, userID, false)
	if err != nil {
		return nil, err
	}
	return practicePlan(plan), nil
}

func (s *practiceService) ListMine(ctx context.Context, campaignID, userID string) ([]PracticeHero, error) {
	list, err := s.repo.ListByUser(ctx, campaignID, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return list, nil
}

func (s *practiceService) ListAll(ctx context.Context, campaignID string, actor PracticeActor) ([]PracticeHero, error) {
	if !actor.Director {
		return nil, apperror.NewForbidden("only the Directors see every practice hero")
	}
	list, err := s.repo.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return list, nil
}

// find loads one practice hero or says it does not exist.
func (s *practiceService) find(ctx context.Context, campaignID, id string) (*PracticeHero, error) {
	h, err := s.repo.Find(ctx, campaignID, id)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if h == nil {
		return nil, apperror.NewNotFound("practice hero not found")
	}
	return h, nil
}

// findOwn is find for something only the maker may change: anyone else,
// Directors included, is told it does not exist.
func (s *practiceService) findOwn(ctx context.Context, campaignID, id string, actor PracticeActor) (*PracticeHero, error) {
	h, err := s.find(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if h.UserID != actor.UserID {
		return nil, apperror.NewNotFound("practice hero not found")
	}
	return h, nil
}

func (s *practiceService) Open(ctx context.Context, campaignID, id string, actor PracticeActor) (*PracticeHero, error) {
	h, err := s.find(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if h.UserID != actor.UserID && !actor.Director {
		return nil, apperror.NewNotFound("practice hero not found")
	}
	return h, nil
}

// check validates what the creator sent against the maker's practice plan and
// returns the hero's name and picks as they will be stored.
func (s *practiceService) check(ctx context.Context, campaignID, makerID string, in PracticeInput) (string, map[string]HeroPick, error) {
	name := strings.TrimSpace(in.Name)
	if err := apperror.ValidateRequired("name", name); err != nil {
		return "", nil, err
	}
	if err := apperror.ValidateStringLength("name", name, apperror.MaxNameLength); err != nil {
		return "", nil, err
	}
	if raw, err := json.Marshal(in.Picks); err != nil || len(raw) > maxPracticePicksBytes {
		return "", nil, apperror.NewBadRequest("those picks are too large to keep")
	}
	plan, err := s.Plan(ctx, campaignID, makerID)
	if err != nil {
		return "", nil, err
	}
	resolved, err := resolveHeroPicks(plan, in.Picks, true)
	if err != nil {
		return "", nil, err
	}
	picks := make(map[string]HeroPick, len(resolved))
	for key, r := range resolved {
		picks[key] = HeroPick{Name: r.choice.Name, Options: r.bought}
	}
	return name, picks, nil
}

func (s *practiceService) Create(ctx context.Context, campaignID string, actor PracticeActor, in PracticeInput) (*PracticeHero, error) {
	name, picks, err := s.check(ctx, campaignID, actor.UserID, in)
	if err != nil {
		return nil, err
	}
	copied := strings.TrimSpace(in.CopiedFrom)
	if len(copied) > apperror.MaxNameLength {
		copied = copied[:apperror.MaxNameLength]
	}
	h := &PracticeHero{
		ID: generateUUID(), CampaignID: campaignID, UserID: actor.UserID,
		Name: name, Picks: picks, Level: in.Level, CopiedFromName: copied,
	}
	saved, err := s.repo.Insert(ctx, h, MaxPracticeHeroes)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !saved {
		return nil, apperror.NewConflict(practiceFullMessage)
	}
	return s.find(ctx, campaignID, h.ID)
}

func (s *practiceService) Update(ctx context.Context, campaignID, id string, actor PracticeActor, in PracticeInput) (*PracticeHero, error) {
	cur, err := s.findOwn(ctx, campaignID, id, actor)
	if err != nil {
		return nil, err
	}
	name, picks, err := s.check(ctx, campaignID, actor.UserID, in)
	if err != nil {
		return nil, err
	}
	cur.Name, cur.Picks = name, picks
	ok, err := s.repo.Replace(ctx, cur)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !ok {
		return nil, apperror.NewNotFound("practice hero not found")
	}
	return s.find(ctx, campaignID, id)
}

func (s *practiceService) Delete(ctx context.Context, campaignID, id string, actor PracticeActor) error {
	if _, err := s.findOwn(ctx, campaignID, id, actor); err != nil {
		return err
	}
	if _, err := s.repo.Delete(ctx, campaignID, id, actor.UserID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *practiceService) Ask(ctx context.Context, campaignID, id string, actor PracticeActor) error {
	h, err := s.findOwn(ctx, campaignID, id, actor)
	if err != nil {
		return err
	}
	if h.AskStatus == PracticeAskWaiting {
		return nil
	}
	if _, err := s.repo.SetAsk(ctx, campaignID, id, actor.UserID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// waitingFor loads a practice hero a Director is about to answer: Directors
// only, and only one that is waiting.
func (s *practiceService) waitingFor(ctx context.Context, campaignID, id string, actor PracticeActor) (*PracticeHero, error) {
	if !actor.Director {
		return nil, apperror.NewForbidden("only the Directors answer this")
	}
	h, err := s.find(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if h.AskStatus != PracticeAskWaiting {
		return nil, apperror.NewConflict("Nobody is asking to play this one now.")
	}
	return h, nil
}

func (s *practiceService) Decline(ctx context.Context, campaignID, id string, actor PracticeActor, note string) (*PracticeHero, error) {
	h, err := s.waitingFor(ctx, campaignID, id, actor)
	if err != nil {
		return nil, err
	}
	note = strings.TrimSpace(note)
	if err := apperror.ValidateStringLength("note", note, maxAskNoteLength); err != nil {
		return nil, err
	}
	ok, err := s.repo.Decline(ctx, campaignID, id, note)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !ok {
		return nil, apperror.NewConflict("Nobody is asking to play this one now.")
	}
	h.AskStatus, h.AskNote = PracticeAskDeclined, note
	return h, nil
}

func (s *practiceService) Claim(ctx context.Context, campaignID, id string, actor PracticeActor) (*PracticeHero, error) {
	h, err := s.waitingFor(ctx, campaignID, id, actor)
	if err != nil {
		return nil, err
	}
	took, err := s.repo.TakeWaiting(ctx, campaignID, id)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !took {
		return nil, apperror.NewConflict("Nobody is asking to play this one now.")
	}
	return h, nil
}

func (s *practiceService) Restore(ctx context.Context, h *PracticeHero) error {
	if err := s.repo.Restore(ctx, h); err != nil {
		slog.Error("putting a practice hero back failed",
			slog.String("campaign_id", h.CampaignID), slog.String("practice_id", h.ID), slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("restoring practice hero: %w", err))
	}
	return nil
}

// practiceAge words how long ago a practice hero changed, for the shelf's
// quiet line. Kept coarse on purpose: the exact time sits in the title.
func practiceAge(now, then time.Time) string {
	d := now.Sub(then)
	switch {
	case d < time.Hour:
		return "just now"
	case d < 24*time.Hour:
		return "today"
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
