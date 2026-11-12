package app

import (
	"context"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// shopTestRulesAdapter hands the armory the shop test of the campaign's
// enabled game system (its shop_tests block, else its crafting block), or the
// defaults when none is enabled. The armory never imports the systems package.
type shopTestRulesAdapter struct {
	sys interface {
		EnabledSystem(ctx context.Context, campaignID string) systems.System
	}
}

func (a *shopTestRulesAdapter) ShopTestRules(ctx context.Context, campaignID string) armory.ShopTestRules {
	v := systems.DefaultShopTestRules()
	if a.sys != nil {
		if s := a.sys.EnabledSystem(ctx, campaignID); s != nil && s.Info() != nil {
			v = s.Info().ShopTestRules()
		}
	}
	out := armory.ShopTestRules{
		Dice: v.Dice, Style: v.Style, Haggle: v.Haggle, Look: v.Look, Tiers: append([]int(nil), v.Tiers...),
		Edge: v.Edge, Bane: v.Bane, DCEasy: v.DC.Easy, DCUsual: v.DC.Usual, DCHard: v.DC.Hard,
		Stats: make([]armory.ShopTestStat, 0, len(v.Stats)),
	}
	for _, s := range v.Stats {
		out.Stats = append(out.Stats, armory.ShopTestStat{Key: s.Key, Label: s.Label, Score: s.Score})
	}
	return out
}

// shopRollLog adds each counter roll to the public roll log. A roll that can't
// be logged is still made and kept; the log is a courtesy to the table.
type shopRollLog struct {
	svc rolls.Service
}

func (l *shopRollLog) Rolled(ctx context.Context, campaignID string, e armory.ShopRollLogEntry) string {
	dice := make([]rolls.Die, len(e.Dice))
	for i, d := range e.Dice {
		dice[i] = rolls.Die{Sides: d.Sides, Value: d.Value}
	}
	id, err := l.svc.Record(ctx, campaignID, rolls.LogInput{
		Source: rolls.SourceShop, Mode: rolls.ModePublic, RollerID: e.RollerID, RollerName: e.RollerName,
		Label: e.Label, Formula: e.Formula, Dice: dice, Modifier: e.Modifier, Total: e.Total,
		Outcome: e.Outcome, From: e.From,
	})
	if err != nil {
		slog.Warn("rolls: a shop counter roll was not added to the roll log", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return ""
	}
	return id
}
