package app

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

// aiPartyAdapter answers the AI tool's `players` lookup from the DM Screen's
// party and the next game night's roster, so the AI sees the same numbers
// the owner does.
type aiPartyAdapter struct {
	screen  dmscreen.Service
	nights  sessions.SessionService
	members campaigns.CampaignService
}

// Party reads the screen as its owner: the lookup route is owner-only, and
// the lookup itself drops characters the chosen privacy mode can't see.
func (p *aiPartyAdapter) Party(ctx context.Context, campaignID string, a records.Actor) (records.Party, error) {
	var out records.Party
	view, err := p.screen.Build(ctx, campaignID, dmscreen.Viewer{UserID: a.UserID, Role: int(campaigns.RoleOwner)})
	if err != nil {
		return out, err
	}
	for _, h := range view.Party {
		hero := records.PartyHero{ID: h.ID, Name: h.Name, Player: h.PlayerName, Subtitle: h.Subtitle, Conditions: h.Conditions}
		for _, m := range h.Meters {
			pm := records.PartyMeter{Label: m.Label, Current: m.Current}
			if m.HasMax {
				pm.Max = m.Max
			}
			hero.Meters = append(hero.Meters, pm)
		}
		out.Heroes = append(out.Heroes, hero)
	}
	out.Night = p.nextNight(ctx, campaignID)
	return out, nil
}

// nextNight is the next game night with each member's answer by name, or
// nil when none is coming up or it can't be read. It looks as far ahead as
// the DM Screen does.
func (p *aiPartyAdapter) nextNight(ctx context.Context, campaignID string) *records.PartyNight {
	list, err := p.members.ListMembers(ctx, campaignID)
	if err != nil {
		return nil
	}
	members := make([]sessions.NightMember, 0, len(list))
	for _, m := range list {
		members = append(members, sessions.NightMember{UserID: m.UserID, Name: m.DisplayName})
	}
	todayT := time.Now().UTC().Add(-14 * time.Hour)
	today := todayT.Format("2006-01-02")
	nights, err := p.nights.ListGameNights(ctx, campaignID, today, todayT.AddDate(0, 0, 60).Format("2006-01-02"), today, members)
	if err != nil {
		return nil
	}
	for _, n := range nights {
		if n.Past {
			continue
		}
		night := &records.PartyNight{Name: n.Name, When: nightWhen(n.Date, n.Time), Answers: map[string]string{}}
		for _, r := range n.Roster {
			night.Answers[r.Name] = r.Answer
		}
		return night
	}
	return nil
}
