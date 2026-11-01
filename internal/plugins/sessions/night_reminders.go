package sessions

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// NotifNightReminder asks a player to answer whether they are coming: the
// person running the night pressing Remind, or the bell the day before.
const NotifNightReminder = "game_night_reminder"

// NightReminder is who RemindNight reached: their names for the page, and
// their ids so the handler can email the same people.
type NightReminder struct {
	Result  NudgeResult
	UserIDs []string
	Night   GameNight
}

// nightsLink is the in-app link to one night: the Game nights route, which
// opens that night on the calendar.
func nightsLink(campaignID, sessionID, date string) string {
	q := url.Values{}
	q.Set("session", sessionID)
	q.Set("date", date)
	return "/campaigns/" + url.PathEscape(campaignID) + "/game-nights?" + q.Encode()
}

// RemindNight asks players to answer one night. With a target it reminds that
// one player, who must not have given a firm answer yet; without one it
// reminds everyone who has not answered at all. The asker is never reminded,
// nor is anyone who left themselves out of the count. A night that has
// already happened takes no reminders.
func (s *sessionService) RemindNight(ctx context.Context, campaignID, sessionID, date, askerID, askerName, targetID, today string, members []NightMember) (*NightReminder, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, apperror.NewValidation("date must be a YYYY-MM-DD date")
	}
	nights, err := s.ListGameNights(ctx, campaignID, date, date, today, members)
	if err != nil {
		return nil, err
	}
	var night *GameNight
	for i := range nights {
		if nights[i].SessionID == sessionID && nights[i].Date == date {
			night = &nights[i]
			break
		}
	}
	if night == nil {
		return nil, apperror.NewNotFound("that game night isn't on that date")
	}
	if night.Past {
		return nil, apperror.NewBadRequest("this game night has already happened")
	}

	out := &NightReminder{Result: NudgeResult{Notified: []string{}}, Night: *night}
	for _, a := range night.Roster {
		if a.UserID == "" || a.UserID == askerID || a.Excluded {
			continue
		}
		if targetID != "" {
			if a.UserID != targetID {
				continue
			}
			if a.Answer == NightYes || a.Answer == NightNo {
				return nil, apperror.NewBadRequest(a.Name + " has already answered")
			}
		} else if a.Answer != NightNoAnswer {
			out.Result.Skipped++
			continue
		}
		out.UserIDs = append(out.UserIDs, a.UserID)
		out.Result.Notified = append(out.Result.Notified, a.Name)
	}
	if targetID != "" && len(out.UserIDs) == 0 {
		if targetID == askerID {
			return nil, apperror.NewBadRequest("you can't remind yourself")
		}
		return nil, apperror.NewNotFound("that player isn't on this night's list")
	}
	if len(out.UserIDs) == 0 {
		return out, nil
	}
	msg := fmt.Sprintf("%s asked you to say if you're coming to %s", askerLabel(askerName), night.Name)
	detail := nightWhenWords(night.Date, night.Time)
	if err := s.NotifyUsersWithDetail(ctx, out.UserIDs, campaignID, NotifNightReminder, msg, detail, nightsLink(campaignID, sessionID, date)); err != nil {
		return nil, err
	}
	return out, nil
}

// nightWhenWords is a night's date and start time in words, as written on
// the night: "Sat Nov 7 at 19:00", or the day alone with no time.
func nightWhenWords(date, hhmm string) string {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	s := d.Format("Mon Jan 2")
	if t, err := time.Parse("15:04", hhmm); err == nil {
		s += " at " + t.Format("3:04 PM")
	}
	return s
}
