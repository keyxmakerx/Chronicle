package sessions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Game-night entries put three items in the bell. The day before a night,
// each member hears that their entry is ready, and anyone who hasn't said
// whether they are coming is asked to. Once the night is over, the
// DM is asked to check in when the entries named anyone, and a player whose
// entry the DM can't read is told that sharing it gets the people they met
// counted. A background loop sends them; session_night_bells makes each one
// go out once.

// Bell types, also the keys notifyprefs files them under.
const (
	NotifNightEntryReady = "game_night_entry_ready"
	NotifNightCheckIn    = "game_night_checkin"
	NotifNightShareEntry = "game_night_share_entry"
)

// Kinds of bell claimed per night and member.
const (
	bellBefore  = "before"
	bellCheckIn = "checkin"
	bellShare   = "share"
	// bellUnanswered is the day-before nudge to say whether you're coming.
	bellUnanswered = "unanswered"
)

const (
	// bellDayBeforeHour is the hour, in the night's own zone, on the day
	// before a night when its reminder goes out.
	bellDayBeforeHour = 9
	// nightLength is how long after its start a night with a start time
	// counts as over; a night with no time is over when its day ends.
	nightLength = 4 * time.Hour
	// bellAfterWindow bounds how late an after-the-night item still goes
	// out, so a server that was down doesn't send week-old check-ins.
	bellAfterWindow = 36 * time.Hour
	// bellTick is how often the loop looks for bells to send.
	bellTick = 10 * time.Minute
)

// NightBellMember is one campaign member as the bells need them. Role is the
// visibility role: a co-DM counts as Owner.
type NightBellMember struct {
	UserID string
	Role   int
}

// NightBellCampaigns is the campaign side of the bells, implemented by an
// adapter over the campaigns and addons services.
type NightBellCampaigns interface {
	// GameNightsOn reports whether the campaign has Game nights turned on.
	GameNightsOn(ctx context.Context, campaignID string) (bool, error)
	// BellMembers lists the campaign's members.
	BellMembers(ctx context.Context, campaignID string) ([]NightBellMember, error)
}

// NightBellSessions loads a session with its linked pages, which the
// check-in needs.
type NightBellSessions interface {
	GetSession(ctx context.Context, id string) (*Session, error)
}

// NightBellRoster reads who has answered a night, the same list the
// calendar shows.
type NightBellRoster interface {
	ListGameNights(ctx context.Context, campaignID, from, to, today string, members []NightMember) ([]GameNight, error)
}

// NightBellNotifier writes bell items, honouring people's choices.
type NightBellNotifier interface {
	NotifyUsersWithDetail(ctx context.Context, userIDs []string, campaignID, ntype, message, detail, link string) error
}

// SetBells gives the service what sending bells needs. Without it SendBells
// does nothing.
func (s *nightEntryService) SetBells(campaigns NightBellCampaigns, sessions NightBellSessions, notify NightBellNotifier) {
	s.bellCampaigns, s.bellSessions, s.notify = campaigns, sessions, notify
}

// SetBellRoster gives the service the answers the unanswered nudge reads.
// Without it that nudge is skipped and the other bells still go out.
func (s *nightEntryService) SetBellRoster(roster NightBellRoster) { s.roster = roster }

// StartNightBells sends due bell items now and then every bellTick until ctx
// ends.
func StartNightBells(ctx context.Context, svc NightEntryService) {
	run := func() {
		if err := svc.SendBells(ctx, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("game-night bells failed", slog.Any("error", err))
		}
	}
	run()
	t := time.NewTicker(bellTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// bellWhen is when a night's bells are due: the reminder from the day
// before at bellDayBeforeHour until the start, the after items from the end
// until bellAfterWindow later.
func bellWhen(date, hhmm, tz string) (remind, start, end time.Time, ok bool) {
	start, ok = nightStart(date, hhmm, tz)
	if !ok {
		return
	}
	y, m, d := start.Date()
	remind = time.Date(y, m, d-1, bellDayBeforeHour, 0, 0, 0, start.Location())
	if hhmm == "" {
		end = time.Date(y, m, d+1, 0, 0, 0, 0, start.Location())
	} else {
		end = start.Add(nightLength)
	}
	return
}

// SendBells sends every bell item due at now that hasn't gone out. One
// campaign failing is logged and the rest still get theirs.
func (s *nightEntryService) SendBells(ctx context.Context, now time.Time) error {
	if s.bellCampaigns == nil || s.bellSessions == nil || s.notify == nil {
		return nil
	}
	day := func(n int) string { return now.UTC().AddDate(0, 0, n).Format("2006-01-02") }
	from, to := day(-3), day(2)
	list, err := s.entries.ListBellNights(ctx, from, to)
	if err != nil {
		return err
	}
	byCampaign := map[string][]Session{}
	var order []string
	for _, sess := range list {
		if _, seen := byCampaign[sess.CampaignID]; !seen {
			order = append(order, sess.CampaignID)
		}
		byCampaign[sess.CampaignID] = append(byCampaign[sess.CampaignID], sess)
	}
	for _, cid := range order {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.campaignBells(ctx, cid, byCampaign[cid], from, to, now); err != nil {
			slog.Warn("game-night bells for a campaign failed", slog.String("campaign_id", cid), slog.Any("error", err))
		}
	}
	return nil
}

func (s *nightEntryService) campaignBells(ctx context.Context, campaignID string, list []Session, from, to string, now time.Time) error {
	var members []NightBellMember
	loaded := false
	for _, sess := range list {
		for _, date := range occurrenceDates(sess, from, to) {
			remind, start, end, ok := bellWhen(date, strPtrVal(sess.ScheduledTime), strPtrVal(sess.ScheduledTZ))
			if !ok {
				continue
			}
			before := sess.Status == StatusPlanned && !now.Before(remind) && now.Before(start)
			after := !now.Before(end) && now.Before(end.Add(bellAfterWindow))
			if !before && !after {
				continue
			}
			if !loaded {
				on, err := s.bellCampaigns.GameNightsOn(ctx, campaignID)
				if err != nil || !on {
					return err
				}
				if members, err = s.bellCampaigns.BellMembers(ctx, campaignID); err != nil {
					return err
				}
				loaded = true
			}
			var err error
			if before {
				err = s.bellsBefore(ctx, sess, date, start, now, members)
				if err == nil {
					err = s.bellsUnanswered(ctx, sess, date, start, now, members)
				}
			} else {
				err = s.bellsAfter(ctx, sess, date, now, members)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// journalLink opens one note in the Journal.
func journalLink(campaignID, noteID string) string {
	return "/campaigns/" + url.PathEscape(campaignID) + "/journal/" + url.PathEscape(noteID)
}

// ownEntry returns the member's entry for the night if it still exists and
// is still theirs.
func (s *nightEntryService) ownEntry(ctx context.Context, campaignID string, v permissions.Viewer, rows []NightEntryRow) (string, error) {
	for _, r := range rows {
		if r.UserID != v.UserID() {
			continue
		}
		list, err := s.journal.ReadableEntries(ctx, campaignID, v, []string{r.NoteID})
		if err != nil {
			return "", err
		}
		if len(list) == 1 && list[0].AuthorID == v.UserID() {
			return r.NoteID, nil
		}
	}
	return "", nil
}

// bellsBefore tells each member the night is coming, making their entry
// first if it isn't there yet so the bell can open it.
func (s *nightEntryService) bellsBefore(ctx context.Context, sess Session, date string, start, now time.Time, members []NightBellMember) error {
	rows, err := s.entries.ListNightEntries(ctx, sess.ID, date)
	if err != nil {
		return err
	}
	has := map[string]bool{}
	for _, r := range rows {
		has[r.UserID] = true
	}
	for _, m := range members {
		// The member's real role and id, read from the membership: the
		// reads below see exactly what they would.
		v := permissions.RequestViewer(m.Role, m.UserID)
		if !getsEntries(v) {
			continue
		}
		claimed, err := s.entries.ClaimNightBell(ctx, sess.ID, date, m.UserID, bellBefore)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		if !has[m.UserID] {
			if err := s.makeEntry(ctx, sess.CampaignID, v, sess, date); err != nil {
				slog.Warn("making a game-night entry for the reminder failed", slog.String("session_id", sess.ID), slog.Any("error", err))
			} else {
				rows = append(rows, NightEntryRow{SessionID: sess.ID, Date: date, UserID: m.UserID, NoteID: nightEntryID(sess.CampaignID, sess.ID, date, m.UserID)})
			}
		}
		id, err := s.ownEntry(ctx, sess.CampaignID, v, rows)
		if err != nil {
			return err
		}
		msg := nightName(sess.Name) + " is " + bellDayWords(start, now) + "."
		link := nightsLink(sess.CampaignID, sess.ID, date)
		if id != "" {
			msg += " Your Journal has an entry ready for it."
			link = journalLink(sess.CampaignID, id)
		}
		if err := s.notify.NotifyUsersWithDetail(ctx, []string{m.UserID}, sess.CampaignID, NotifNightEntryReady, msg, "", link); err != nil {
			return err
		}
	}
	return nil
}

// bellsUnanswered asks each member on the night's list who hasn't answered,
// and hasn't left themselves out of the count, to say whether they are
// coming. The organizer is never asked about their own night. It files
// under the same notification row as the organizer's own Remind.
func (s *nightEntryService) bellsUnanswered(ctx context.Context, sess Session, date string, start, now time.Time, members []NightBellMember) error {
	if s.roster == nil {
		return nil
	}
	list := make([]NightMember, 0, len(members))
	for _, m := range members {
		if m.UserID != "" {
			list = append(list, NightMember{UserID: m.UserID})
		}
	}
	today := now.In(start.Location()).Format("2006-01-02")
	nights, err := s.roster.ListGameNights(ctx, sess.CampaignID, date, date, today, list)
	if err != nil {
		return err
	}
	for _, n := range nights {
		if n.SessionID != sess.ID || n.Date != date || n.Past {
			continue
		}
		for _, a := range n.Roster {
			if a.UserID == "" || a.UserID == n.OrganizerID || a.Excluded || a.Answer != NightNoAnswer {
				continue
			}
			claimed, err := s.entries.ClaimNightBell(ctx, sess.ID, date, a.UserID, bellUnanswered)
			if err != nil {
				return err
			}
			if !claimed {
				continue
			}
			msg := nightName(sess.Name) + " is " + bellDayWords(start, now) + ". Can you make it?"
			if err := s.notify.NotifyUsersWithDetail(ctx, []string{a.UserID}, sess.CampaignID, NotifNightReminder, msg, "", nightsLink(sess.CampaignID, sess.ID, date)); err != nil {
				return err
			}
		}
	}
	return nil
}

// bellDayWords is "tomorrow at 7:00 PM" or "today at 7:00 PM", in the
// night's own zone; a night with no start time drops the time.
func bellDayWords(start, now time.Time) string {
	y, m, d := now.In(start.Location()).Date()
	words := "tomorrow"
	if sy, sm, sd := start.Date(); sy == y && sm == m && sd == d {
		words = "today"
	}
	if start.Hour() != 0 || start.Minute() != 0 {
		words += " at " + start.Format("3:04 PM")
	}
	return words
}

func nightName(name string) string {
	if name == "" {
		return "Game night"
	}
	return name
}

// bellsAfter asks the DMs to check in once the entries name anyone they
// haven't answered, and tells each player whose entry no DM can read, at the
// first look after the night, that sharing it counts the people they met.
// Each goes out once.
func (s *nightEntryService) bellsAfter(ctx context.Context, sess Session, date string, now time.Time, members []NightBellMember) error {
	var dms []permissions.Viewer
	for _, m := range members {
		if v := permissions.RequestViewer(m.Role, m.UserID); m.UserID != "" && seesDM(v) {
			dms = append(dms, v)
		}
	}
	var full *Session
	for _, v := range dms {
		if full == nil {
			var err error
			if full, err = s.bellSessions.GetSession(ctx, sess.ID); err != nil {
				return err
			}
		}
		night, err := s.ForNight(ctx, full, date, v, now)
		if err != nil {
			return err
		}
		open := 0
		for _, p := range night.CheckIn {
			if p.Answer == "" {
				open++
			}
		}
		// Claimed only once there is someone to ask about, so an entry
		// shared later in the window still brings the check-in.
		if open == 0 {
			continue
		}
		claimed, err := s.entries.ClaimNightBell(ctx, sess.ID, date, v.UserID(), bellCheckIn)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		people := "people"
		if open == 1 {
			people = "person"
		}
		msg := fmt.Sprintf("%s is over. %d %s named in the entries may have met the party.", nightName(sess.Name), open, people)
		if err := s.notify.NotifyUsersWithDetail(ctx, []string{v.UserID()}, sess.CampaignID, NotifNightCheckIn, msg, "", nightsLink(sess.CampaignID, sess.ID, date)); err != nil {
			return err
		}
	}
	rows, err := s.entries.ListNightEntries(ctx, sess.ID, date)
	if err != nil {
		return err
	}
	for _, m := range members {
		v := permissions.RequestViewer(m.Role, m.UserID)
		if !getsEntries(v) || seesDM(v) {
			continue
		}
		claimed, err := s.entries.ClaimNightBell(ctx, sess.ID, date, m.UserID, bellShare)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		id, err := s.ownEntry(ctx, sess.CampaignID, v, rows)
		if err != nil {
			return err
		}
		if id == "" {
			continue
		}
		shared, err := s.anyCanRead(ctx, sess.CampaignID, dms, id)
		if err != nil {
			return err
		}
		if shared {
			continue
		}
		msg := nightName(sess.Name) + " is over. Share your entry with the GM if you want the people you met counted."
		if err := s.notify.NotifyUsersWithDetail(ctx, []string{m.UserID}, sess.CampaignID, NotifNightShareEntry, msg, "", journalLink(sess.CampaignID, id)); err != nil {
			return err
		}
	}
	return nil
}

// anyCanRead reports whether one of the viewers can read the note, which is
// whether the check-in reads it.
func (s *nightEntryService) anyCanRead(ctx context.Context, campaignID string, viewers []permissions.Viewer, noteID string) (bool, error) {
	for _, v := range viewers {
		list, err := s.journal.ReadableEntries(ctx, campaignID, v, []string{noteID})
		if err != nil {
			return false, err
		}
		if len(list) > 0 {
			return true, nil
		}
	}
	return false, nil
}
