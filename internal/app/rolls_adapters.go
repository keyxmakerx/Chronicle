package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/plugins/transformations"
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

// rollsCritAdapter gives the roll log the crit rules of the campaign's enabled
// game system; with none enabled the default (natural 20 and 1 on a d20) applies.
type rollsCritAdapter struct {
	sys interface {
		EnabledSystem(ctx context.Context, campaignID string) systems.System
	}
}

func (a *rollsCritAdapter) CritRules(ctx context.Context, campaignID string) []systems.CritRule {
	if a.sys == nil {
		return systems.DefaultCritRules()
	}
	s := a.sys.EnabledSystem(ctx, campaignID)
	if s == nil || s.Info() == nil {
		return systems.DefaultCritRules()
	}
	return s.Info().CritRules()
}

// rollsCharacterAdapter lets a Table roll name the hero it was made for. The
// page must be a live page of this campaign and the viewer its claimed player
// or a Director, because Foundry posts the roll as the actor linked to it. A
// public roll also needs a page every player may see: Foundry shows the
// actor's name and picture to the table, and a hidden page's actor is hidden
// there too. Its
// category is not checked: which categories count as characters is a list the
// Director keeps, and a hero outside it should still roll as itself.
type rollsCharacterAdapter struct {
	entities interface {
		GetByID(ctx context.Context, id string) (*entities.Entity, error)
	}
}

func (a *rollsCharacterAdapter) MayRollAs(ctx context.Context, campaignID string, v rolls.Viewer, characterID, mode string) (bool, error) {
	if a.entities == nil || v.UserID == "" {
		return false, nil
	}
	e, err := a.entities.GetByID(ctx, characterID)
	if err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	if e == nil || e.CampaignID != campaignID {
		return false, nil
	}
	if !v.Director && !e.IsOwnedBy(v.UserID) {
		return false, nil
	}
	if mode == rolls.ModePublic && (e.IsPrivate || (e.Visibility != "" && e.Visibility != entities.VisibilityDefault)) {
		return false, nil
	}
	return true, nil
}

// rollsPowerAdapter tells the Table the campaign's power roll (the game
// system's crafting declaration) and the characteristics on the viewer's own
// claimed hero. It reads only a character the viewer owns, so no one's sheet
// is shown to anyone else, and it withholds any number the GM marked GM-only
// unless the viewer sees GM content: the Table is one more place a page's
// values leave the server, so it filters them like every other.
type rollsPowerAdapter struct {
	sys interface {
		EnabledSystem(ctx context.Context, campaignID string) systems.System
	}
	entities interface {
		ListByOwner(ctx context.Context, campaignID, userID string) ([]entities.Entity, error)
		GetEntityTypeByID(ctx context.Context, id int) (*entities.EntityType, error)
	}
	characters interface {
		CharacterTypeIDs(ctx context.Context, campaignID string) ([]int, error)
	}
	// layers and addons are set once the transformations plugin is up; while
	// either is nil the numbers are the page's own.
	layers interface {
		ListForPage(ctx context.Context, campaignID, entityID string, v transformations.Viewer) (*transformations.PageLayers, error)
	}
	addons interface {
		IsEnabledForCampaign(ctx context.Context, campaignID, addonSlug string) (bool, error)
	}
}

func (a *rollsPowerAdapter) PowerRoll(ctx context.Context, campaignID, userID string, canSeeGM bool) (rolls.PowerRollLine, error) {
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
	// The numbers are a convenience: without them the line still rolls, so
	// any failure below leaves them out rather than failing the request.
	hero, fields := a.heroFields(ctx, campaignID, userID, canSeeGM)
	if hero == nil {
		return out, nil
	}
	out.Hero, out.HeroID = hero.Name, hero.ID
	layered := layeredNumbers(fields, a.heroLayers(ctx, campaignID, hero.ID, userID, canSeeGM))
	for i, c := range rules.Characteristics {
		if n, ok := layered(c.Key); ok {
			v := n
			out.Characteristics[i].Value = &v
		}
	}
	return out, nil
}

// heroLayers reads the hero's transformation layers as this viewer may, so
// a chip rolls the number the sheet shows. With the add-on off the sheet
// ignores layers, and so does the roll; any failure leaves the page's own
// numbers.
func (a *rollsPowerAdapter) heroLayers(ctx context.Context, campaignID, heroID, userID string, canSeeGM bool) []transformations.Layer {
	if a.layers == nil || a.addons == nil {
		return nil
	}
	on, err := a.addons.IsEnabledForCampaign(ctx, campaignID, transformations.AddonSlug)
	if err != nil || !on {
		return nil
	}
	role := permissions.RolePlayer
	if canSeeGM {
		role = permissions.RoleOwner
	}
	pl, err := a.layers.ListForPage(ctx, campaignID, heroID, transformations.Viewer{UserID: userID, VisibilityRole: role})
	if err != nil || pl == nil {
		if err != nil {
			slog.Warn("rolls: cannot read the hero's layers; rolling the page's numbers", slog.Any("error", err))
		}
		return nil
	}
	return pl.Layers
}

func isSheetNumber(s string) bool { _, ok, _ := sheetNumber(s); return ok }

// layeredNumbers applies the layers to the numbers the way the hero sheet
// does, oldest first: set replaces a field (a number, or "@key" for another
// field's value), add adds to it (an unset field counts as 0). A set to
// anything else leaves the field with no number. It returns a lookup of a
// field's number after every layer.
func layeredNumbers(fields map[string]any, layers []transformations.Layer) func(key string) (int, bool) {
	vals := map[string]float64{}
	notNumber := map[string]bool{}
	get := func(k string) (float64, bool) {
		if notNumber[k] {
			return 0, false
		}
		if v, ok := vals[k]; ok {
			return v, true
		}
		n, ok, _ := sheetNumber(fields[k])
		return float64(n), ok
	}
	for _, l := range layers {
		for _, c := range l.Changes {
			switch c.Op {
			case transformations.OpSet:
				var n float64
				var ref string
				switch {
				case json.Unmarshal(c.Value, &n) == nil:
					vals[c.Field] = n
					delete(notNumber, c.Field)
				case json.Unmarshal(c.Value, &ref) == nil && strings.HasPrefix(ref, "@"):
					v, _ := get(ref[1:])
					vals[c.Field] = v
					delete(notNumber, c.Field)
				case json.Unmarshal(c.Value, &ref) == nil && isSheetNumber(ref):
					v, _, _ := sheetNumber(ref)
					vals[c.Field] = float64(v)
					delete(notNumber, c.Field)
				default:
					delete(vals, c.Field)
					notNumber[c.Field] = true
				}
			case transformations.OpAdd:
				var n float64
				if json.Unmarshal(c.Value, &n) != nil {
					continue
				}
				base, _ := get(c.Field)
				vals[c.Field] = base + n
				delete(notNumber, c.Field)
			}
		}
	}
	return func(key string) (int, bool) {
		if notNumber[key] {
			return 0, false
		}
		if v, ok := vals[key]; ok {
			return int(math.Round(v)), true
		}
		n, ok, _ := sheetNumber(fields[key])
		return n, ok
	}
}

// heroFields picks the viewer's hero the way the hero creator does (the first
// page they own of a character type) and returns its values with GM-only
// fields removed for a viewer who may not see them. GM-only covers the
// category's fields and the fields added to just this page. When the
// category cannot be read, a player gets no values at all: a field's
// restriction is unknown then, and unknown is treated as hidden.
func (a *rollsPowerAdapter) heroFields(ctx context.Context, campaignID, userID string, canSeeGM bool) (*entities.Entity, map[string]any) {
	if a.entities == nil || a.characters == nil || userID == "" {
		return nil, nil
	}
	charTypes, err := a.characters.CharacterTypeIDs(ctx, campaignID)
	if err != nil {
		slog.Warn("rolls: cannot read the character types", slog.Any("error", err))
		return nil, nil
	}
	mine, err := a.entities.ListByOwner(ctx, campaignID, userID)
	if err != nil {
		slog.Warn("rolls: cannot read the viewer's hero", slog.Any("error", err))
		return nil, nil
	}
	var hero *entities.Entity
	for i := range mine {
		if slices.Contains(charTypes, mine[i].EntityTypeID) {
			hero = &mine[i]
			break
		}
	}
	if hero == nil {
		return nil, nil
	}
	if canSeeGM {
		return hero, hero.FieldsData
	}
	t, err := a.entities.GetEntityTypeByID(ctx, hero.EntityTypeID)
	if err != nil || t == nil {
		slog.Warn("rolls: cannot read the hero's category; leaving its numbers out", slog.Any("error", err))
		return hero, nil
	}
	return hero, entities.FilterGMOnlyFields(hero.FieldsData, entities.RestrictionDefs(t.Fields, hero.FieldOverrides), false)
}
