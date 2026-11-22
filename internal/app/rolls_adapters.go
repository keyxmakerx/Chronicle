package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/systems"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// rollsPublisher sends rolls over the WebSocket hub. The hub's per-recipient
// gate (Message.DeliversTo) drops a roll for anyone outside its audience
// before it is written to their socket.
type rollsPublisher struct {
	bus ws.EventBus
}

func (p *rollsPublisher) RollMade(campaignID string, v rolls.View, a rolls.Audience) {
	msg := ws.NewMessage(ws.MsgRollMade, campaignID, v.ID, v)
	switch {
	case a.Everyone:
		msg.Public = true
	case a.DirectorsOnly:
		msg.RequiresDM = true
	case len(a.Users) > 0:
		msg.AllowedUsers = append([]string(nil), a.Users...)
		msg.StrictAudience = a.Strict
	default:
		// An audience naming nobody reaches nobody.
		return
	}
	p.bus.Publish(msg)
}

// rollsSyncAdapter lets the sync API record Foundry's rolls in the log.
type rollsSyncAdapter struct {
	svc rolls.Service
}

func (a *rollsSyncAdapter) Report(ctx context.Context, campaignID, memberID string, in syncapi.RollReport) (string, bool, error) {
	dice := make([]rolls.Die, len(in.Dice))
	for i, d := range in.Dice {
		dice[i] = rolls.Die{Sides: d.Sides, Value: d.Value}
	}
	return a.svc.Report(ctx, campaignID, rolls.ReportInput{
		Origin: rolls.OriginFoundry, OriginID: in.OriginID, RollerID: memberID, RollerName: in.RollerName,
		Label: in.Label, Formula: in.Formula, Dice: dice, Total: in.Total, Mode: in.Mode,
	})
}

// craftingRollLog adds each crafting project roll to the roll log. A roll
// that can't be logged is still made; the Crafting app has its own record.
type craftingRollLog struct {
	svc rolls.Service
}

func (l *craftingRollLog) ProjectRolled(ctx context.Context, campaignID string, v crafting.Viewer, r crafting.ProjectRoll) {
	dice := make([]rolls.Die, len(r.Dice))
	sum := 0
	for i, n := range r.Dice {
		dice[i] = rolls.Die{Sides: r.Sides, Value: n}
		sum += n
	}
	outcome := fmt.Sprintf("%d of %d", r.To, r.Goal)
	if r.Finished {
		outcome = "Made"
	}
	err := l.svc.Log(ctx, campaignID, rolls.LogInput{
		Source: rolls.SourceCrafting, RollerID: v.UserID, RollerName: r.Hero,
		Label: "Project roll: " + r.Recipe, Formula: fmt.Sprintf("%dd%d", len(r.Dice), r.Sides),
		Dice: dice, Modifier: r.Total - sum, Total: r.Total, Outcome: outcome,
		Why:  strings.TrimSpace(fmt.Sprintf("Work on %s, %d points before the roll", r.Recipe, r.From)),
		From: "Crafting, " + r.Hero + "’s projects",
	})
	if err != nil {
		slog.Warn("rolls: a crafting roll was not added to the roll log", slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
}

// rollsPowerAdapter tells the Table the campaign's power roll (the game
// system's crafting declaration) and the characteristics on the viewer's own
// claimed hero. It reads only a character the viewer owns, so no one's sheet
// is shown to anyone else.
type rollsPowerAdapter struct {
	sys interface {
		EnabledSystem(ctx context.Context, campaignID string) systems.System
	}
	entities entities.EntityService
}

func (a *rollsPowerAdapter) PowerRoll(ctx context.Context, campaignID, userID string) (rolls.PowerRollLine, error) {
	if a.sys == nil {
		return rolls.PowerRollLine{}, nil
	}
	s := a.sys.EnabledSystem(ctx, campaignID)
	if s == nil {
		return rolls.PowerRollLine{}, nil
	}
	rules, ok := s.Info().PowerRoll()
	if !ok {
		return rolls.PowerRollLine{}, nil
	}
	out := rolls.PowerRollLine{Declared: true, Dice: rules.Dice, Edge: rules.Edge, Bane: rules.Bane, Tiers: rules.Tiers,
		Characteristics: make([]rolls.PowerStat, len(rules.Characteristics))}
	for i, c := range rules.Characteristics {
		out.Characteristics[i] = rolls.PowerStat{Key: c.Key, Label: c.Label}
	}
	if a.entities == nil || userID == "" {
		return out, nil
	}
	mine, err := a.entities.ListByOwner(ctx, campaignID, userID)
	if err != nil || len(mine) == 0 {
		// The numbers are a convenience: without them the line still rolls.
		if err != nil {
			slog.Warn("rolls: cannot read the viewer's hero", slog.Any("error", err))
		}
		return out, nil
	}
	hero := mine[0]
	out.Hero = hero.Name
	for i, c := range rules.Characteristics {
		if n, ok, _ := sheetNumber(hero.FieldsData[c.Key]); ok {
			v := n
			out.Characteristics[i].Value = &v
		}
	}
	return out, nil
}
