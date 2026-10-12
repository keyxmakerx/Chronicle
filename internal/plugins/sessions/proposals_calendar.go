package sessions

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Date polls live on the Game nights calendar: each date an open poll
// suggests shows on that day, and the day card is where members answer and
// where whoever runs the game picks the date.

// calendarPoll is one open date poll as the calendar draws it.
type calendarPoll struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Note    string `json:"note"`
	AskedBy string `json:"askedBy"`
	// CanPick is whether this viewer may pick the winning date, the same
	// rule the confirm route enforces.
	CanPick bool `json:"canPick"`
	// Members is how many people could answer, so the day card can say how
	// many haven't.
	Members int                  `json:"members"`
	TZ      string               `json:"tz"`
	Options []calendarPollOption `json:"options"`
}

// calendarPollOption is one suggested date with its answers so far.
type calendarPollOption struct {
	ID        string `json:"id"`
	Date      string `json:"date"`
	DateLabel string `json:"dateLabel"`
	TimeLabel string `json:"timeLabel"`
	StartsAt  string `json:"startsAt"`
	EndsAt    string `json:"endsAt"`
	Yes       int    `json:"yes"`
	Maybe     int    `json:"maybe"`
	No        int    `json:"no"`
	// Mine is the viewer's own answer: yes, maybe, no or "".
	Mine string `json:"mine"`
}

// calendarPollsFrom shapes poll views for the calendar. names maps a user id
// to a display name; a poll whose asker has left reads "Someone".
func calendarPollsFrom(views []ProposalView, names map[string]string, members int, canPick bool) []calendarPoll {
	out := make([]calendarPoll, 0, len(views))
	for _, v := range views {
		p := calendarPoll{
			ID:      v.Proposal.ID,
			Title:   v.Proposal.Title,
			AskedBy: names[v.Proposal.CreatedBy],
			CanPick: canPick,
			Members: members,
			TZ:      v.ViewerTZ,
			Options: make([]calendarPollOption, 0, len(v.Options)),
		}
		if p.AskedBy == "" {
			p.AskedBy = "Someone"
		}
		if v.Proposal.Note != nil {
			p.Note = *v.Proposal.Note
		}
		for _, o := range v.Options {
			p.Options = append(p.Options, calendarPollOption{
				ID:        o.Option.ID,
				Date:      o.Local.Date,
				DateLabel: o.Local.DateLabel,
				TimeLabel: o.Local.TimeLabel,
				StartsAt:  o.Local.StartsAtUTC,
				EndsAt:    o.Local.EndsAtUTC,
				Yes:       o.YesCount,
				Maybe:     o.MaybeCount,
				No:        o.NoCount,
				Mine:      o.MyResponse,
			})
		}
		out = append(out, p)
	}
	return out
}

// OpenPollsAPI returns the open date polls for the calendar, dates in the
// zone the calendar asks for (?tz=, else the viewer's own).
// GET /campaigns/:id/proposals/open
func (h *Handler) OpenPollsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	views, err := h.svc.ListOpenProposalViews(ctx, cc.Campaign.ID, userID, h.resolveViewerTZ(c, userID))
	if err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	names := map[string]string{}
	members := 0
	if h.memberLister != nil {
		if list, err := h.memberLister.ListMembers(ctx, cc.Campaign.ID); err == nil {
			members = len(list)
			for _, m := range list {
				names[m.UserID] = m.DisplayName
			}
		}
	}
	canPick := cc.MemberRole >= campaigns.RoleScribe || cc.CanAuthorDmOnly()
	return c.JSON(http.StatusOK, calendarPollsFrom(views, names, members, canPick))
}

// pollLandingDate is the day a poll opens on: the date picked, once there
// is one; else the first date it suggests from today on; else its first.
func pollLandingDate(v *ProposalView, today string) string {
	first, next := "", ""
	for _, o := range v.Options {
		if o.Option.IsWinner {
			return o.Local.Date
		}
		if first == "" {
			first = o.Local.Date
		}
		if next == "" && o.Local.Date >= today {
			next = o.Local.Date
		}
	}
	if next != "" {
		return next
	}
	return first
}
