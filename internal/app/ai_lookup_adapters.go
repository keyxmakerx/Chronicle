package app

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// aiPartyAdapter answers the AI tool's `players` lookup from the DM Screen's
// party and the next game night's roster, so the AI sees the same numbers
// the owner does, or, in a player's view, the ones players do.
type aiPartyAdapter struct {
	screen  dmscreen.Service
	types   aiEntityTypeReader
	nights  sessions.SessionService
	members campaigns.CampaignService
}

// aiEntityTypeReader reads one category, for its field definitions.
type aiEntityTypeReader interface {
	GetEntityTypeByID(ctx context.Context, id int) (*entities.EntityType, error)
}

// Party reads the screen as its owner: the lookup route is owner-only, and
// the lookup itself drops characters the chosen privacy mode can't see. In a
// player's view each hero's GM-only and owner-only sheet values are removed
// before the screen reads its meters, subtitle and conditions, as they are
// from a player's copy of the page.
func (p *aiPartyAdapter) Party(ctx context.Context, campaignID string, a records.Actor) (records.Party, error) {
	var out records.Party
	var narrow dmscreen.FieldNarrower
	if !a.CanAuthorDmOnly() {
		narrow = p.playerFields(ctx)
	}
	heroes, err := p.screen.Party(ctx, campaignID, dmscreen.Viewer{UserID: a.UserID, Role: int(campaigns.RoleOwner)}, narrow)
	if err != nil {
		return out, err
	}
	for _, h := range heroes {
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

// playerFields narrows a hero's sheet values to what another player may
// see. A category that can't be read leaves no values: nothing then says
// which are restricted.
func (p *aiPartyAdapter) playerFields(ctx context.Context) dmscreen.FieldNarrower {
	defs := map[int][]entities.FieldDefinition{}
	known := map[int]bool{}
	return func(h dmscreen.Hero) map[string]any {
		if _, done := known[h.TypeID]; !done {
			known[h.TypeID] = false
			if p.types != nil {
				if et, err := p.types.GetEntityTypeByID(ctx, h.TypeID); err == nil && et != nil {
					defs[h.TypeID], known[h.TypeID] = et.Fields, true
				}
			}
		}
		if !known[h.TypeID] {
			return nil
		}
		return entities.FilterRestrictedFields(h.Fields, defs[h.TypeID], false, false)
	}
}

// nextNight is the next game night with each member's answer by name, or
// nil when none is coming up or it can't be read. It looks as far ahead as
// the DM Screen does.
func (p *aiPartyAdapter) nextNight(ctx context.Context, campaignID string) *records.PartyNight {
	if p.members == nil || p.nights == nil {
		return nil
	}
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

// aiSystemEntriesAdapter answers the AI tool's `system-entries` lookup from
// the character pick lists, package and campaign-made entries alike.
type aiSystemEntriesAdapter struct {
	choices systems.CharacterChoiceService
}

// Entries reads the list as a Director only when the lookup's privacy mode
// lets the AI see Director-only content, so Safe never lists hidden entries.
func (a aiSystemEntriesAdapter) Entries(ctx context.Context, campaignID, field string, director bool) (string, []records.SystemEntry, error) {
	l, err := a.choices.CharacterChoiceList(systems.WithChoiceViewer(ctx, director), campaignID, field)
	if err != nil {
		return "", nil, err
	}
	out := make([]records.SystemEntry, 0, len(l.Choices))
	for _, c := range l.Choices {
		out = append(out, records.SystemEntry{Name: c.Name, Slug: c.Slug, Source: c.Source, Summary: c.Summary, Status: c.Status})
	}
	return l.SystemName, out, nil
}

// Fields lists the character fields that have entries, the keys a
// system-entry block's `entry:` takes. It reads the hero creator's plan, so
// the AI sees the same fields a player picks from.
func (a aiSystemEntriesAdapter) Fields(ctx context.Context, campaignID string, director bool) (string, []records.SystemEntryField, error) {
	plan, err := a.choices.CreatorPlan(systems.WithChoiceViewer(ctx, director), campaignID)
	if err != nil {
		return "", nil, err
	}
	out := make([]records.SystemEntryField, 0, len(plan.Steps))
	for _, st := range plan.Steps {
		out = append(out, records.SystemEntryField{Key: st.FieldKey, Label: st.Label, Count: len(st.Choices)})
	}
	return plan.SystemName, out, nil
}
