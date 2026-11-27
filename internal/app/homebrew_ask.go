package app

// homebrew_ask.go puts the "Ask the Director" step in the Homebrew Workshop
// and gives it what it tells people through: the campaign's members (who
// asked, who answers), the bell, the live socket and the sync API the
// Foundry module's GM card reads.

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

func init() {
	addWorkshopSource("ask the director", func(reg *homebrew.Registry, _ workshopDeps) error {
		return reg.RegisterAction(homebrew.AskDirector())
	})
}

// homebrewAskPeople names the asker and lists the Directors over the
// campaigns service. A Director is the owner or anyone holding the DM-only
// grant, the rule the request context uses.
type homebrewAskPeople struct {
	svc campaigns.CampaignService
}

var _ homebrew.AskPeople = (*homebrewAskPeople)(nil)

func (p *homebrewAskPeople) Name(ctx context.Context, campaignID, userID string) string {
	m, err := p.svc.GetMember(ctx, campaignID, userID)
	if err != nil || m == nil {
		return ""
	}
	return m.DisplayName
}

func (p *homebrewAskPeople) Directors(ctx context.Context, campaignID string) ([]string, error) {
	c, err := p.svc.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	list, err := p.svc.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	grants := map[string]bool{}
	for _, id := range c.ParseSettings().DmGrantIDs {
		grants[id] = true
	}
	var out []string
	for _, m := range list {
		if m.Role >= campaigns.RoleOwner || grants[m.UserID] {
			out = append(out, m.UserID)
		}
	}
	return out, nil
}

// homebrewAskBell writes ask notices through the notification service, which
// honours each person's notification choices.
type homebrewAskBell struct {
	notify interface {
		NotifyUsersWithDetail(ctx context.Context, userIDs []string, campaignID, ntype, message, detail, link string) error
	}
}

func (b *homebrewAskBell) Ring(ctx context.Context, campaignID string, n homebrew.Notice) error {
	return b.notify.NotifyUsersWithDetail(ctx, n.Users, campaignID, n.Kind, n.Message, n.Detail, n.Link)
}

// homebrewAskLive carries ask events over the hub to the Directors and the
// person who asked. The payload is the run's id and state only.
type homebrewAskLive struct {
	bus ws.EventBus
}

func (l *homebrewAskLive) AskChanged(campaignID string, e homebrew.AskEvent) {
	if l.bus == nil {
		return
	}
	t := ws.MsgHomebrewAnswered
	if e.State == homebrew.AskWaiting || e.State == homebrew.AskRoll {
		t = ws.MsgHomebrewAsk
	}
	msg := ws.NewMessage(t, campaignID, e.ID, e)
	// Directors pass the hub's audience rule on their own; an empty list
	// would make it Directors-only, which is right when nobody else asked.
	if e.Asker != "" {
		msg.AllowedUsers = []string{e.Asker}
	} else {
		msg.RequiresDM = true
	}
	l.bus.Publish(msg)
}

// syncHomebrewAskAdapter implements syncapi.HomebrewAskAPIService. The sync
// handler has already checked the caller is the owner or a co-DM, and the
// homebrew service checks the Director again. The routes are registered
// before the Workshop is built, so the service is attached afterwards; until
// then, or while the plugin is degraded, every call reads as not found.
type syncHomebrewAskAdapter struct {
	svc homebrew.Service
}

var _ syncapi.HomebrewAskAPIService = (*syncHomebrewAskAdapter)(nil)

func (a *syncHomebrewAskAdapter) attach(svc homebrew.Service) { a.svc = svc }

func homebrewAPIViewer(userID string) homebrew.Viewer {
	return homebrew.Viewer{UserID: userID, Director: true, VisibilityRole: permissions.RoleOwner}
}

func (a *syncHomebrewAskAdapter) Waiting(ctx context.Context, campaignID, userID string) (any, int, error) {
	if a.svc == nil {
		return nil, 0, apperror.NewNotFound("homebrew asks are not available")
	}
	list, err := a.svc.Asks(ctx, campaignID, homebrewAPIViewer(userID))
	if err != nil {
		return nil, 0, err
	}
	return list, len(list), nil
}

func (a *syncHomebrewAskAdapter) Answer(ctx context.Context, campaignID, userID, runID string, body json.RawMessage) (any, error) {
	if a.svc == nil {
		return nil, apperror.NewNotFound("homebrew asks are not available")
	}
	var in homebrew.AnswerInput
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, apperror.NewBadRequest("send the answer as JSON")
	}
	return a.svc.Answer(ctx, campaignID, runID, homebrewAPIViewer(userID), in)
}
