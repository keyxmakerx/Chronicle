package entities

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// StandingService is the Standing block's business logic. Every method takes
// the viewer's role and decides here what that viewer may be told: a player
// gets the word and the reasons marked for everyone, only for subjects that
// have a standing, and nothing at all when the page's switch is off. Handlers
// and the block add no rule of their own.
type StandingService interface {
	// View is the whole block for one viewer.
	View(ctx context.Context, campaignID, entityID string, role int, userID string) (*StandingView, error)
	// Subject is one subject's standing for one viewer; NotFound when the
	// viewer may not see it (the same answer as one that does not exist).
	Subject(ctx context.Context, campaignID, entityID, subject string, role int, userID string) (*StandingSubjectView, error)
	// AddReason adds a reason and moves the word. Director only.
	AddReason(ctx context.Context, in AddStandingReasonInput) (*StandingSubjectView, error)
	// RemoveReason removes a reason. Director only.
	RemoveReason(ctx context.Context, campaignID, entityID, subject, reasonID string, role int, userID string) (*StandingSubjectView, error)
	// SetPlayersSee sets the page's switch. Director only.
	SetPlayersSee(ctx context.Context, campaignID, entityID string, role int, userID string, see bool) error
}

// StandingViewerGate says which pages a viewer may open, so a character's name
// reaches a player only if they could open that character's page.
// EntityService satisfies it.
type StandingViewerGate interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

type standingService struct {
	repo StandingRepository
	gate StandingViewerGate
}

// NewStandingService creates the Standing service.
func NewStandingService(repo StandingRepository, gate StandingViewerGate) StandingService {
	return &standingService{repo: repo, gate: gate}
}

func requireStandingDM(role int) error {
	if !permissions.CanSeeDmOnly(role) {
		return apperror.NewForbidden("only the Director can change a standing")
	}
	return nil
}

// standingKey turns a wire subject ("party" or a page id) into the stored key.
// Anything else is refused before it reaches SQL.
func standingKey(subject string) (string, error) {
	if subject == StandingPartyWire {
		return StandingPartyKey, nil
	}
	if _, err := uuid.Parse(subject); err != nil {
		return "", apperror.NewBadRequest("unknown subject")
	}
	return subject, nil
}

func standingWire(key string) string {
	if key == StandingPartyKey {
		return StandingPartyWire
	}
	return key
}

// reasonViews turns stored reasons into a viewer's list. The DM-only filter is
// applied again here even though the repository already skipped them for a
// player, so a repository that ever returned one still could not leak it.
func reasonViews(rs []StandingReason, dm bool) []StandingReasonView {
	out := make([]StandingReasonView, 0, len(rs))
	for _, r := range rs {
		if r.DMOnly && !dm {
			continue
		}
		out = append(out, StandingReasonView{ID: r.ID, Direction: r.Direction, Text: r.Text, When: r.When, DMOnly: r.DMOnly && dm})
	}
	return out
}

func subjectView(key, name string, row *StandingRow, reasons []StandingReason, total int, dm bool) StandingSubjectView {
	v := StandingSubjectView{Key: standingWire(key), Name: name, Level: StandingLevelDefault, Word: StandingWord(StandingLevelDefault), Reasons: []StandingReasonView{}}
	if row != nil {
		v.Set = true
		v.Level = row.Level
		v.Word = StandingWord(row.Level)
		v.Reasons = reasonViews(reasons, dm)
		v.Total = total
		if len(v.Reasons) > v.Total {
			v.Total = len(v.Reasons)
		}
	}
	return v
}

func (s *standingService) View(ctx context.Context, campaignID, entityID string, role int, userID string) (*StandingView, error) {
	dm := permissions.CanSeeDmOnly(role)
	see, err := s.repo.PlayersSee(ctx, entityID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	// A player on a page whose standing is the Director's reads nothing else.
	if !dm && !see {
		return &StandingView{Hidden: true}, nil
	}
	rows, err := s.repo.List(ctx, campaignID, entityID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	var charIDs, standingIDs []string
	for _, r := range rows {
		standingIDs = append(standingIDs, r.ID)
		if r.SubjectKey != StandingPartyKey {
			charIDs = append(charIDs, r.SubjectKey)
		}
	}
	var chars []StandingSubjectRef
	if dm {
		if chars, err = s.repo.Characters(ctx, campaignID, standingCharacterPick); err != nil {
			return nil, apperror.NewInternal(err)
		}
	}
	names, err := s.repo.Names(ctx, campaignID, charIDs)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !dm && len(charIDs) > 0 {
		ok, err := s.gate.FilterViewableEntityIDs(ctx, campaignID, charIDs, role, userID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		for id := range names {
			if !ok[id] {
				delete(names, id)
			}
		}
	}
	reasons, counts, err := s.repo.ReasonsFor(ctx, standingIDs, dm, standingReasonsShown)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	byKey := map[string]*StandingRow{}
	for i := range rows {
		byKey[rows[i].SubjectKey] = &rows[i]
	}
	out := &StandingView{CanEdit: dm, Subjects: []StandingSubjectView{}}
	if dm {
		out.PlayersSee = &see
	}
	add := func(key, name string) {
		row := byKey[key]
		var rs []StandingReason
		n := 0
		if row != nil {
			rs, n = reasons[row.ID], counts[row.ID]
		}
		out.Subjects = append(out.Subjects, subjectView(key, name, row, rs, n, dm))
	}
	// The party leads. A player only ever sees subjects that have a word.
	if dm || byKey[StandingPartyKey] != nil {
		add(StandingPartyKey, "The party")
	}
	seen := map[string]bool{}
	for _, r := range rows {
		name, ok := names[r.SubjectKey]
		if r.SubjectKey == StandingPartyKey || !ok {
			continue
		}
		seen[r.SubjectKey] = true
		add(r.SubjectKey, name)
	}
	if dm {
		for _, c := range chars {
			if !seen[c.Key] {
				add(c.Key, c.Name)
			}
		}
	}
	if !dm && len(out.Subjects) == 0 {
		return &StandingView{Hidden: true}, nil
	}
	return out, nil
}

// subjectName is the display name of a subject the viewer is asking about.
func (s *standingService) subjectName(ctx context.Context, campaignID, key string) (string, error) {
	if key == StandingPartyKey {
		return "The party", nil
	}
	names, err := s.repo.Names(ctx, campaignID, []string{key})
	if err != nil {
		return "", apperror.NewInternal(err)
	}
	name, ok := names[key]
	if !ok {
		return "", apperror.NewNotFound("subject not found")
	}
	return name, nil
}

func (s *standingService) Subject(ctx context.Context, campaignID, entityID, subject string, role int, userID string) (*StandingSubjectView, error) {
	key, err := standingKey(subject)
	if err != nil {
		return nil, err
	}
	dm := permissions.CanSeeDmOnly(role)
	if !dm {
		see, err := s.repo.PlayersSee(ctx, entityID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if !see {
			return nil, apperror.NewNotFound("standing not found")
		}
	}
	name, err := s.subjectName(ctx, campaignID, key)
	if err != nil {
		return nil, err
	}
	if !dm && key != StandingPartyKey {
		ok, err := s.gate.FilterViewableEntityIDs(ctx, campaignID, []string{key}, role, userID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if !ok[key] {
			return nil, apperror.NewNotFound("standing not found")
		}
	}
	row, err := s.repo.Find(ctx, entityID, key)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if row == nil && !dm {
		return nil, apperror.NewNotFound("standing not found")
	}
	var rs []StandingReason
	n := 0
	if row != nil {
		reasons, counts, err := s.repo.ReasonsFor(ctx, []string{row.ID}, dm, standingReasonsShown)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		rs, n = reasons[row.ID], counts[row.ID]
	}
	v := subjectView(key, name, row, rs, n, dm)
	return &v, nil
}

func (s *standingService) AddReason(ctx context.Context, in AddStandingReasonInput) (*StandingSubjectView, error) {
	if err := requireStandingDM(in.Role); err != nil {
		return nil, err
	}
	key, err := standingKey(in.Subject)
	if err != nil {
		return nil, err
	}
	switch in.Direction {
	case StandingUp, StandingDown, StandingNone:
	default:
		return nil, apperror.NewValidation("Choose whether it raises, lowers or changes nothing.")
	}
	text, ok := cleanStandingText(in.Text, StandingReasonMax)
	if text == "" {
		return nil, apperror.NewValidation("Say what happened first.")
	}
	if !ok {
		return nil, apperror.NewValidation("Keep the reason to 90 characters or fewer.")
	}
	when, ok := cleanStandingText(in.When, StandingWhenMax)
	if !ok {
		return nil, apperror.NewValidation("Keep the \"when\" to 30 characters or fewer.")
	}
	if _, err := s.subjectName(ctx, in.CampaignID, key); err != nil {
		return nil, err
	}
	level := StandingLevelDefault
	if in.Level != nil {
		if *in.Level < StandingLevelMin || *in.Level > StandingLevelMax {
			return nil, apperror.NewValidation("That is not one of the five words.")
		}
		level = *in.Level
	} else if cur, err := s.repo.Find(ctx, in.EntityID, key); err != nil {
		return nil, apperror.NewInternal(err)
	} else if cur != nil {
		level = cur.Level
	}
	err = s.repo.SaveReason(ctx, StandingReasonWrite{
		CampaignID: in.CampaignID, EntityID: in.EntityID, SubjectKey: key, Level: level,
		Reason: StandingReason{ID: uuid.NewString(), Direction: in.Direction, Text: text, When: when, DMOnly: in.DMOnly, CreatedBy: in.UserID},
	})
	if errors.Is(err, errStandingFull) {
		return nil, apperror.NewValidation("This standing already holds 300 reasons. Remove some before adding more.")
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return s.Subject(ctx, in.CampaignID, in.EntityID, in.Subject, in.Role, in.UserID)
}

func (s *standingService) RemoveReason(ctx context.Context, campaignID, entityID, subject, reasonID string, role int, userID string) (*StandingSubjectView, error) {
	if err := requireStandingDM(role); err != nil {
		return nil, err
	}
	key, err := standingKey(subject)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(reasonID); err != nil {
		return nil, apperror.NewNotFound("reason not found")
	}
	removed, err := s.repo.DeleteReason(ctx, entityID, key, reasonID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !removed {
		return nil, apperror.NewNotFound("reason not found")
	}
	return s.Subject(ctx, campaignID, entityID, subject, role, userID)
}

func (s *standingService) SetPlayersSee(ctx context.Context, campaignID, entityID string, role int, userID string, see bool) error {
	if err := requireStandingDM(role); err != nil {
		return err
	}
	if err := s.repo.SetPlayersSee(ctx, campaignID, entityID, userID, see); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
