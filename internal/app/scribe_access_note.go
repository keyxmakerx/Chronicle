package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// scribe_access_note.go tells each owner, once, that their Scribes now see
// what players see: hidden pages, the GM Screen and Reveal became the owner's
// and Co-DMs' alone, and an owner who wants a Scribe to keep that access has
// to make them a Co-DM. It runs once per site, so a Scribe added later is
// never announced as a change.

// scribeAccessNoteKey is the site setting that records the note was sent.
const scribeAccessNoteKey = "reconcile.scribe_access_note"

// notifScribeAccessChanged is the bell type of the note. It has no category on
// the account page, so it is always delivered.
const notifScribeAccessChanged = "scribe_access_changed"

// scribeNoteNames is how many Scribes the note names before "and N more".
const scribeNoteNames = 5

// scribeNoteCampaigns is the slice of the campaign service the note needs.
type scribeNoteCampaigns interface {
	ListAll(ctx context.Context, opts campaigns.ListOptions) ([]campaigns.Campaign, int, error)
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// scribeNoteNotifier writes a bell notification.
type scribeNoteNotifier interface {
	NotifyUsersWithDetail(ctx context.Context, userIDs []string, campaignID, ntype, message, detail, link string) error
}

// noteScribeAccessOnce sends the note to the owners of every campaign with a
// Scribe who is not a Co-DM, unless that already happened, and returns how
// many campaigns it wrote to. A campaign that fails is logged and skipped; the
// setting is written once every campaign was attempted, so only a crash
// part-way repeats the sweep, and then some owners get the note twice.
func noteScribeAccessOnce(ctx context.Context, st pageExtrasSettings, camps scribeNoteCampaigns, bell scribeNoteNotifier) (int, error) {
	if v, err := st.Get(ctx, scribeAccessNoteKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", scribeAccessNoteKey, err)
		}
	}

	sent := 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return sent, fmt.Errorf("listing campaigns: %w", err)
		}
		for i := range list {
			c := &list[i]
			members, err := camps.ListMembers(ctx, c.ID)
			if err != nil {
				slog.Warn("scribe access note: members unavailable",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			owners, scribes := scribeNoteRecipients(c, members)
			if len(owners) == 0 || len(scribes) == 0 {
				continue
			}
			message := "Scribes in " + c.Name + " now see what players see"
			detail := "Your Scribes: " + nameList(scribes, scribeNoteNames) +
				". To keep one seeing hidden pages, make them a Co-DM on People."
			link := "/campaigns/" + c.ID + "/members"
			if err := bell.NotifyUsersWithDetail(ctx, owners, c.ID, notifScribeAccessChanged, message, detail, link); err != nil {
				slog.Warn("scribe access note: sending failed",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			sent++
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	if err := st.Set(ctx, scribeAccessNoteKey, "1"); err != nil {
		return sent, fmt.Errorf("recording %s: %w", scribeAccessNoteKey, err)
	}
	return sent, nil
}

// scribeNoteRecipients splits a campaign's members into the owners who get
// the note and the names of the Scribes whose access changed. A Co-DM keeps
// everything, so they are not listed.
func scribeNoteRecipients(c *campaigns.Campaign, members []campaigns.CampaignMember) (owners, scribes []string) {
	for _, m := range members {
		switch {
		case m.Role == campaigns.RoleOwner:
			owners = append(owners, m.UserID)
		case m.Role == campaigns.RoleScribe && campaigns.MemberAccess(c, m) != campaigns.AccessCoDM:
			name := strings.TrimSpace(m.DisplayName)
			if name == "" {
				name = "a guest"
			}
			scribes = append(scribes, name)
		}
	}
	return owners, scribes
}

// nameList joins names as "A, B and C", naming at most max and counting the
// rest.
func nameList(names []string, max int) string {
	shown := names
	more := 0
	if len(names) > max {
		shown, more = names[:max], len(names)-max
	}
	if more > 0 {
		return strings.Join(shown, ", ") + fmt.Sprintf(" and %d more", more)
	}
	if len(shown) == 1 {
		return shown[0]
	}
	return strings.Join(shown[:len(shown)-1], ", ") + " and " + shown[len(shown)-1]
}
