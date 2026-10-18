package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

// aiGameNightsAdapter implements records.GameNightsAPI over the sessions
// service. The kind only hands it IDs it listed for the same campaign.
type aiGameNightsAdapter struct {
	svc     sessions.SessionService
	members campaigns.CampaignService
}

var _ records.GameNightsAPI = aiGameNightsAdapter{}

func (g aiGameNightsAdapter) List(ctx context.Context, campaignID string) ([]records.GameNightRef, error) {
	list, err := g.svc.ListSessions(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]records.GameNightRef, 0, len(list))
	for _, s := range list {
		ref := records.GameNightRef{ID: s.ID, Name: s.Name, Status: s.Status}
		if s.ScheduledDate != nil {
			ref.Date = *s.ScheduledDate
		}
		if s.ScheduledTime != nil {
			ref.Time = *s.ScheduledTime
		}
		if s.Summary != nil {
			ref.Summary = *s.Summary
		}
		out = append(out, ref)
	}
	return out, nil
}

// Create invites every member, as the Game nights page does when a night is
// made there. It sends no RSVP email: an import never mails people.
func (g aiGameNightsAdapter) Create(ctx context.Context, campaignID string, a records.Actor, in records.GameNightInput) (string, error) {
	var name string
	if in.Name != nil {
		name = *in.Name
	}
	s, err := g.svc.CreateSession(ctx, campaignID, sessions.CreateSessionInput{
		Name: name, Summary: in.Summary, ScheduledDate: in.Date, ScheduledTime: in.Time, CreatedBy: a.UserID,
	})
	if err != nil {
		return "", err
	}
	if in.Status != nil && *in.Status != sessions.StatusPlanned {
		if _, err := g.svc.UpdateSession(ctx, s.ID, sessions.UpdateSessionInput{Status: patch.Of(*in.Status)}); err != nil {
			return s.ID, err
		}
	}
	if members, err := g.members.ListMembers(ctx, campaignID); err == nil {
		ids := make([]string, 0, len(members))
		for _, m := range members {
			ids = append(ids, m.UserID)
		}
		_ = g.svc.InviteAll(ctx, s.ID, ids)
	}
	return s.ID, nil
}

func (g aiGameNightsAdapter) Update(ctx context.Context, id string, in records.GameNightInput) error {
	_, err := g.svc.UpdateSession(ctx, id, sessions.UpdateSessionInput{
		Name:          setIfGiven(in.Name),
		Summary:       setIfGiven(in.Summary),
		ScheduledDate: setIfGiven(in.Date),
		ScheduledTime: setIfGiven(in.Time),
		Status:        setIfGiven(in.Status),
	})
	return err
}

// setIfGiven leaves a key the AI did not write as it is; patch.FromPtr would
// clear it instead.
func setIfGiven(p *string) patch.Field[string] {
	if p == nil {
		return patch.Absent[string]()
	}
	return patch.Of(*p)
}

func (g aiGameNightsAdapter) SetRecap(ctx context.Context, id string, recapJSON, recapHTML *string) error {
	return g.svc.UpdateSessionRecap(ctx, id, recapJSON, recapHTML)
}

func (g aiGameNightsAdapter) Delete(ctx context.Context, id string) error {
	_, err := g.svc.DeleteSession(ctx, id)
	return err
}
