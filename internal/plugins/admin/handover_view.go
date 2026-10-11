package admin

import (
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// lastSeenText says how long ago an owner was last seen, in the coarse
// units the hand-over setting is chosen in.
func lastSeenText(t, now time.Time) string {
	d := now.Sub(t)
	switch days := int(d.Hours() / 24); {
	case days < 1:
		return "seen today"
	case days == 1:
		return "seen yesterday"
	case days < 60:
		return fmt.Sprintf("seen %d days ago", days)
	default:
		return fmt.Sprintf("seen %d months ago", days/30)
	}
}

// handoverSettingText is the owner's "If you're away" choice as the admin
// list shows it.
func handoverSettingText(months int) string {
	switch months {
	case 0:
		return "Hand-over: never"
	case 12:
		return "Hand-over after 1 year away"
	default:
		return fmt.Sprintf("Hand-over after %d months away", months)
	}
}

// handoverAction is the "Hand over…" action, offered only once the owner
// has been away longer than they allowed. It opens the hand-over page.
func handoverAction(c campaigns.Campaign, st campaigns.HandoverStatus) []rowAction {
	if !st.Open {
		return nil
	}
	return []rowAction{{Label: "Hand over…", MenuLabel: "Hand over…", Method: "get",
		URL: fmt.Sprintf("/admin/campaigns/%s/hand-over", c.ID), Tone: "warn"}}
}

// handoverMonthsText is the owner's allowed absence in words.
func handoverMonthsText(months int) string {
	if months == 12 {
		return "a year"
	}
	return fmt.Sprintf("%d months", months)
}

// handoverCandidates are the members who could own the campaign: anyone
// with their own account except the current owner.
func handoverCandidates(members []campaigns.CampaignMember, ownerID string) []campaigns.CampaignMember {
	var out []campaigns.CampaignMember
	for _, m := range members {
		if m.UserID != ownerID && !m.IsGuest {
			out = append(out, m)
		}
	}
	return out
}
