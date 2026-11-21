package entities

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// practice_promote.go is what a Director's yes does, and the reads around it
// that need the page system: which page type a hero would be, the player's
// current hero, a copy of it. It lives in the service so the handler only
// binds, calls and draws.

// HeroReleaser lets a hero go back to "Free to pick" for one page, the way a
// leaver's heroes do. Wired at the composition root from the entity
// repository, which owns that SQL.
type HeroReleaser interface {
	ReleaseOne(ctx context.Context, campaignID, entityID, userID string) (bool, error)
}

// PracticePages is what practice heroes need of the page system: the
// campaign's page types, a person's own pages, and making a page. Satisfied by
// EntityService.
type PracticePages interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]EntityType, error)
	ListByOwner(ctx context.Context, campaignID, ownerUserID string) ([]Entity, error)
	Create(ctx context.Context, campaignID, userID string, input CreateEntityInput) (*Entity, error)
}

// PracticeCharacterTypes names the campaign's character page types. Satisfied
// by CharacterListReader.
type PracticeCharacterTypes interface {
	CharacterTypeIDs(ctx context.Context, campaignID string) ([]int, error)
}

// PracticeMembers lists a campaign's members. Satisfied by MemberLister.
type PracticeMembers interface {
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// PracticeDeps are the collaborators of a Director's yes. Any may be nil in
// tests; Promote needs them all but Releaser, and without one a player's old
// hero is simply not freed.
type PracticeDeps struct {
	Pages    PracticePages
	Types    PracticeCharacterTypes
	Members  PracticeMembers
	Releaser HeroReleaser
}

// PracticePromotion is the outcome of a Director's yes. The new hero stands in
// every case; ReleaseErr says the player's previous hero could not be freed
// afterwards, which leaves both pages theirs until a Director frees one.
type PracticePromotion struct {
	// Practice is the row that was taken off the shelf.
	Practice *PracticeHero
	// Hero is the new real hero.
	Hero *Entity
	// Previous is the hero the yes replaces; nil when the player had none.
	Previous   *Entity
	ReleaseErr error
}

// createHeroPage makes a hero page. The hero creator and a Director's yes to a
// practice hero both come through here, so a hero is the same page either way.
// createdBy is who made it; owner is who plays it, nil for a hero made for the
// table. private is decided by the caller, who knows whose creation this is.
func createHeroPage(ctx context.Context, pages interface {
	Create(ctx context.Context, campaignID, userID string, input CreateEntityInput) (*Entity, error)
}, campaignID, createdBy, name string, target *EntityType, fields map[string]any, owner *string, private bool) (*Entity, error) {
	return pages.Create(ctx, campaignID, createdBy, CreateEntityInput{
		Name:         name,
		EntityTypeID: target.ID,
		IsPrivate:    private,
		FieldsData:   fields,
		Playable:     true,
		OwnerUserID:  owner,
	})
}

// practiceTargetType is the page type a player's real hero would be, with the
// campaign's character type ids, nil when none may be made. A practice hero
// is for someone who could have a hero, so the same rule decides: a type
// players may claim, among the campaign's character types. The look-around
// try-out asks the same question without a practice service.
func practiceTargetType(ctx context.Context, types PracticeCharacterTypes, pages PracticePages, campaignID string) (*EntityType, []int, error) {
	if types == nil || pages == nil {
		return nil, nil, nil
	}
	ids, err := types.CharacterTypeIDs(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	all, err := pages.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	return pickHeroType(all, ids, true), ids, nil
}

func (s *practiceService) Target(ctx context.Context, campaignID string) (*EntityType, []int, error) {
	return practiceTargetType(ctx, s.deps.Types, s.deps.Pages, campaignID)
}

func (s *practiceService) CurrentHero(ctx context.Context, campaignID, userID string) (*Entity, error) {
	if s.deps.Types == nil || s.deps.Pages == nil {
		return nil, nil
	}
	ids, err := s.deps.Types.CharacterTypeIDs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return s.ownedHero(ctx, campaignID, userID, ids)
}

func (s *practiceService) ownedHero(ctx context.Context, campaignID, userID string, charTypeIDs []int) (*Entity, error) {
	owned, err := s.deps.Pages.ListByOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	for i := range owned {
		if slices.Contains(charTypeIDs, owned[i].EntityTypeID) {
			return &owned[i], nil
		}
	}
	return nil, nil
}

// PracticeCopy is the player's own hero read as practice picks.
type PracticeCopy struct {
	Hero  *Entity
	Picks map[string]HeroPick
	Level *int
}

func (s *practiceService) CopySource(ctx context.Context, campaignID, userID string, canSeeGM bool) (*PracticeCopy, error) {
	hero, err := s.CurrentHero(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	if hero == nil {
		return nil, apperror.NewBadRequest("you have no hero to copy")
	}
	types, err := s.deps.Pages.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	plan, err := s.Plan(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	// Fields marked GM-only are left out, as the player's own view of the
	// hero leaves them out.
	picks, level := practiceCopyFields(plan, hero.FieldsData, heroHiddenKeys(hero, types, canSeeGM))
	return &PracticeCopy{Hero: hero, Picks: picks, Level: level}, nil
}

// makerStillPlays checks the practice hero's maker is still a Player here.
// The hooks that delete a leaver's rows are best effort, and a maker who has
// since become a Scribe is no longer someone a practice hero is for.
func (s *practiceService) makerStillPlays(ctx context.Context, campaignID, userID string) error {
	if s.deps.Members == nil {
		return nil
	}
	members, err := s.deps.Members.ListMembers(ctx, campaignID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	for _, m := range members {
		if m.UserID == userID && m.Role == campaigns.RolePlayer {
			return nil
		}
	}
	return apperror.NewConflict("That player is no longer in this campaign.")
}

// carryLevel sets a copied hero's level on the real hero's fields when the
// page type has a level field to put it in.
func carryLevel(fields map[string]any, level *int, target *EntityType) {
	if level == nil {
		return
	}
	for _, f := range target.Fields {
		if f.Key != "level" {
			continue
		}
		if f.Type == "number" {
			fields["level"] = float64(*level)
		} else {
			fields["level"] = strconv.Itoa(*level)
		}
		return
	}
}

// Promote makes the real hero from a practice hero a Director said yes to.
//
// The Director's yes covers every entry the rules keep from the maker, so the
// hero is checked against the unlimited plan as that maker sees it (never
// reading anything Directors alone may read). The row is claimed first, and
// the hero is built from the row that was claimed, not one read earlier, so
// two Directors answering at once cannot make two heroes and a maker's edit
// cannot be approved unseen (seen is the state the Director looked at). If
// anything fails before the hero exists the row goes back untouched. The
// one-hero rule holds because the maker's current hero is released afterwards,
// as a leaver's is; if that fails the new hero stands and the result says so,
// since both pages are then theirs and a Director must free one.
//
// The new page is public to the party like any page a player makes: the maker
// is a Player, whose own creation is never private, whatever the campaign's
// default is for its Directors.
func (s *practiceService) Promote(ctx context.Context, campaignID, id string, actor PracticeActor, seen string) (*PracticePromotion, error) {
	if s.planner == nil || s.deps.Pages == nil || s.deps.Types == nil {
		return nil, apperror.NewNotFound("practice heroes are not available here")
	}
	hero, err := s.Open(ctx, campaignID, id, actor)
	if err != nil {
		return nil, err
	}
	if !actor.Director {
		return nil, apperror.NewForbidden("only the Directors answer this")
	}
	if hero.AskStatus != PracticeAskWaiting {
		return nil, apperror.NewConflict("Nobody is asking to play this one now.")
	}
	target, charTypes, err := s.Target(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, apperror.NewConflict("Players can't have a hero of that kind in this campaign now.")
	}
	if err := s.makerStillPlays(ctx, campaignID, hero.UserID); err != nil {
		return nil, err
	}
	plan, err := s.planner.HeroPlan(WithUnlimitedPlan(ctx), campaignID, hero.UserID, false)
	if err != nil {
		return nil, err
	}
	previous, err := s.ownedHero(ctx, campaignID, hero.UserID, charTypes)
	if err != nil {
		return nil, err
	}

	claimed, err := s.claim(ctx, campaignID, id, actor, seen)
	if err != nil {
		return nil, err
	}
	// From here until the hero exists, every failure puts the row back, and
	// not under a request that was cancelled, or the practice hero is lost.
	keep := context.WithoutCancel(ctx)
	fields, _, err := buildHeroFields(plan, claimed.Picks)
	if err != nil {
		s.restore(keep, claimed)
		return nil, err
	}
	carryLevel(fields, claimed.Level, target)
	owner := claimed.UserID
	made, err := createHeroPage(ctx, s.deps.Pages, campaignID, claimed.UserID, claimed.Name, target, fields, &owner, false)
	if err != nil {
		s.restore(keep, claimed)
		return nil, err
	}
	res := &PracticePromotion{Practice: claimed, Hero: made, Previous: previous}
	if previous != nil && s.deps.Releaser != nil {
		if _, err := s.deps.Releaser.ReleaseOne(ctx, campaignID, previous.ID, claimed.UserID); err != nil {
			slog.Error("releasing the old hero after a Director's yes failed; the player now owns both pages",
				slog.String("campaign_id", campaignID), slog.String("old_entity_id", previous.ID),
				slog.String("new_entity_id", made.ID), slog.Any("error", err))
			res.ReleaseErr = fmt.Errorf("releasing %s: %w", previous.ID, err)
		}
	}
	return res, nil
}

// maxHeldNames is how many picks a Director's line names before "and N more".
const maxHeldNames = 3

func (s *practiceService) HeldPicks(ctx context.Context, campaignID string, waiting []PracticeHero) map[string][]string {
	out := map[string][]string{}
	if s.planner == nil {
		return out
	}
	// One plan per maker, only for rows that are waiting: the line is for the
	// answer a Director is about to give, and the list stays cheap otherwise.
	// The names come from the maker's own plan, so they are entries the maker
	// may read; nothing a Director alone may read is named.
	plans := map[string]*HeroPlan{}
	for _, h := range waiting {
		if h.AskStatus != PracticeAskWaiting {
			continue
		}
		plan, ok := plans[h.UserID]
		if !ok {
			p, err := s.Plan(ctx, campaignID, h.UserID)
			if err != nil {
				slog.Warn("practice heroes: reading a maker's plan failed",
					slog.String("campaign_id", campaignID), slog.Any("error", err))
				p = nil
			}
			plan = p
			plans[h.UserID] = plan
		}
		for _, r := range practicePickRows(plan, h.Picks) {
			if r.Note != "" {
				out[h.ID] = append(out[h.ID], r.Value)
			}
		}
	}
	return out
}
