package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
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
