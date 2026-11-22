package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// hero_changes_adapters.go wires when players may change their hero's picks:
// the Director's rule lives in the campaign settings, and a change rings the
// bell of the owner and every Co-DM. A Director keeps one unread row per hero
// (see hero_change_fold.go).

// notifHeroChanged is the bell type of a player's hero change. It has no
// category on the account page, so it is always delivered.
const notifHeroChanged = "hero_changed"

// notifHeroLevelRaised is the bell type of a level a player gave their own
// hero through the sync. Like notifHeroChanged it is always delivered.
const notifHeroLevelRaised = "hero_level_raised"

// heroChangeShown is how many changed fields the bell names before "and N more".
const heroChangeShown = 3

// heroChangeNames is how many names of a list the bell gives on each side,
// added and removed, before "and N more".
const heroChangeNames = 4

// heroChangeCampaigns is the slice of the campaign service the rule and the
// bell need.
type heroChangeCampaigns interface {
	GetByID(ctx context.Context, id string) (*campaigns.Campaign, error)
	UpdateHeroChanges(ctx context.Context, campaignID string, rule *campaigns.HeroChangeRule) error
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// heroChangeRules keeps the rule in the campaign settings, so it travels with
// the campaign's other settings.
type heroChangeRules struct {
	camps heroChangeCampaigns
}

func (r heroChangeRules) HeroChangeRule(ctx context.Context, campaignID string) (*campaigns.HeroChangeRule, error) {
	c, err := r.camps.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, apperror.NewNotFound("campaign not found")
	}
	return c.ParseSettings().HeroChanges, nil
}

func (r heroChangeRules) SaveHeroChangeRule(ctx context.Context, campaignID string, rule *campaigns.HeroChangeRule) error {
	return r.camps.UpdateHeroChanges(ctx, campaignID, rule)
}

// heroBellStore is the bell store as the hero bells use it: a plain row for a
// level, and one row per hero for changes.
type heroBellStore interface {
	scribeNoteNotifier
	FoldUnreadNotification(ctx context.Context, userIDs []string, campaignID, ntype, link string, fold func(kept string) (message, detail, state string)) error
}

// heroChangeBell tells the campaign's Directors what a player changed.
type heroChangeBell struct {
	camps heroChangeCampaigns
	bell  heroBellStore
}

// HeroChanged keeps one unread row per hero and Director: a change while the
// row is unread folds into it, so a burst of saves is one bell.
func (b heroChangeBell) HeroChanged(ctx context.Context, e *entities.Entity, userID string, changes []entities.HeroPickChange) error {
	c, err := b.camps.GetByID(ctx, e.CampaignID)
	if err != nil || c == nil {
		return err
	}
	members, err := b.camps.ListMembers(ctx, e.CampaignID)
	if err != nil {
		return err
	}
	directors, player := heroChangeRecipients(c, members, userID)
	if len(directors) == 0 {
		return nil
	}
	return b.bell.FoldUnreadNotification(ctx, directors, e.CampaignID, notifHeroChanged, heroLink(e),
		func(kept string) (string, string, string) {
			fields := foldHeroChanges(kept, changes)
			return player + " changed " + e.Name, heroFoldDetail(fields), encodeHeroFold(fields)
		})
}

// heroLink is the page a hero's bells point at; it is also what keeps one
// unread row per hero.
func heroLink(e *entities.Entity) string {
	return "/campaigns/" + e.CampaignID + "/entities/" + e.ID
}

// LevelRaised says only who, which hero and the new level: the page itself
// may hold GM-only text, and none of it goes in the bell.
func (b heroChangeBell) LevelRaised(ctx context.Context, e *entities.Entity, userID string, level int) error {
	return b.ring(ctx, e, userID, notifHeroLevelRaised, func(player string) (string, string) {
		return player + " raised " + e.Name + " to level " + strconv.Itoa(level), ""
	})
}

// ring sends one bell to the campaign's Directors, leaving out the player
// who made the change.
func (b heroChangeBell) ring(ctx context.Context, e *entities.Entity, userID, ntype string, say func(player string) (message, detail string)) error {
	c, err := b.camps.GetByID(ctx, e.CampaignID)
	if err != nil || c == nil {
		return err
	}
	members, err := b.camps.ListMembers(ctx, e.CampaignID)
	if err != nil {
		return err
	}
	directors, player := heroChangeRecipients(c, members, userID)
	if len(directors) == 0 {
		return nil
	}
	message, detail := say(player)
	link := heroLink(e)
	return b.bell.NotifyUsersWithDetail(ctx, directors, e.CampaignID, ntype, message, detail, link)
}

// heroChangeRecipients is the owner and every Co-DM except the player
// themselves, and the player's name as the bell reads it.
func heroChangeRecipients(c *campaigns.Campaign, members []campaigns.CampaignMember, userID string) (directors []string, player string) {
	player = "A player"
	for _, m := range members {
		if m.UserID == userID {
			if n := strings.TrimSpace(m.DisplayName); n != "" {
				player = n
			}
			continue
		}
		if m.Role == campaigns.RoleOwner || campaigns.MemberAccess(c, m) == campaigns.AccessCoDM {
			directors = append(directors, m.UserID)
		}
	}
	return directors, player
}

func pluralMore(n int) string {
	if n == 1 {
		return "1 more"
	}
	return strconv.Itoa(n) + " more"
}
