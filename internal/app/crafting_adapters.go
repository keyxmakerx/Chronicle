package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// craftingDirectoryAdapter gives the crafting plugin the campaign's Armory
// items and characters through the Armory's own directory, so both read
// "item" and "character" the same way and crafting never reads the
// entities tables.
type craftingDirectoryAdapter struct {
	dir *armoryStashDirectoryAdapter
}

func (a *craftingDirectoryAdapter) Entity(ctx context.Context, campaignID, id string) (*crafting.EntityInfo, error) {
	ref, err := a.dir.GetEntity(ctx, campaignID, id)
	if err != nil || ref == nil {
		return nil, err
	}
	// A map handout is an item only so it can be carried; it is never a
	// material or something a recipe makes.
	return &crafting.EntityInfo{ID: ref.ID, Name: ref.Name, IsItem: ref.IsItem && ref.HandoutMapID == "", IsCharacter: ref.IsCharacter}, nil
}

func (a *craftingDirectoryAdapter) ListItems(ctx context.Context, campaignID string, role int, userID string) ([]crafting.Named, bool, error) {
	ti, err := a.dir.types(ctx, campaignID)
	if err != nil {
		return nil, false, err
	}
	refs, truncated, err := a.dir.listByTypes(ctx, ti, campaignID, ti.itemIDs, role, userID, 0)
	if err != nil {
		return nil, false, err
	}
	out := make([]crafting.Named, 0, len(refs))
	for _, r := range refs {
		if r.IsItem && r.HandoutMapID == "" {
			out = append(out, crafting.Named{ID: r.ID, Name: r.Name})
		}
	}
	return out, truncated, nil
}

func (a *craftingDirectoryAdapter) ListCharacters(ctx context.Context, campaignID string, role int, userID string) ([]crafting.Named, bool, error) {
	refs, truncated, err := a.dir.ListCharactersChecked(ctx, campaignID, role, userID)
	if err != nil {
		return nil, false, err
	}
	out := make([]crafting.Named, 0, len(refs))
	for _, r := range refs {
		out = append(out, crafting.Named{ID: r.ID, Name: r.Name})
	}
	return out, truncated, nil
}

func (a *craftingDirectoryAdapter) OwnedCharacterIDs(ctx context.Context, campaignID, userID string) (map[string]bool, error) {
	return a.dir.OwnedCharacterIDs(ctx, campaignID, userID)
}

// craftingRulesAdapter reads the crafting block of the campaign's enabled
// game system; with none enabled the defaults apply.
type craftingRulesAdapter struct {
	sys interface {
		EnabledSystem(ctx context.Context, campaignID string) systems.System
	}
}

func (a *craftingRulesAdapter) CraftingRules(ctx context.Context, campaignID string) (systems.CraftingRules, string) {
	if a.sys == nil {
		return systems.DefaultCraftingRules(), ""
	}
	s := a.sys.EnabledSystem(ctx, campaignID)
	if s == nil || s.Info() == nil {
		return systems.DefaultCraftingRules(), ""
	}
	return s.Info().CraftingRules(), s.Info().Name
}

// craftingCalendarAdapter names the default calendar the rules editor reads.
type craftingCalendarAdapter struct {
	cal headerCalendarService
}

func (a *craftingCalendarAdapter) DefaultCalendarID(ctx context.Context, campaignID string, role int, userID string) string {
	return headerSkyCalendarID(ctx, a.cal, campaignID, permissions.RequestViewer(role, userID))
}

// craftingArmoryAdapter is the Armory as crafting needs it: a hero's pack,
// and a project's materials, cost and made item moving through the stash
// service, which checks the actor may act for the character on every call.
type craftingArmoryAdapter struct {
	stash armory.StashService
}

func craftingActor(v crafting.Viewer) armory.Actor {
	return armory.Actor{UserID: v.UserID, Role: v.VisibilityRole}
}

func craftingCents(c crafting.Cost) armory.Cents {
	return armory.Cents(math.Round(c.Amount * 100))
}

func (a *craftingArmoryAdapter) Pack(ctx context.Context, campaignID string, v crafting.Viewer, characterID string) (*crafting.Pack, error) {
	p, err := a.stash.CraftPack(ctx, campaignID, craftingActor(v), characterID)
	if err != nil {
		return nil, err
	}
	out := &crafting.Pack{Name: p.Name, Items: make([]crafting.PackItem, 0, len(p.Items)), PayKind: p.PayKind, Have: int64(p.Have),
		MoneyLabel: p.MoneyLabel, MoneyKey: p.MoneyKey}
	for _, it := range p.Items {
		out.Items = append(out.Items, crafting.PackItem{ItemID: it.ItemID, Name: it.Name, Qty: it.Quantity, Tags: []string{}})
	}
	return out, nil
}

func (a *craftingArmoryAdapter) Charge(p *crafting.Pack, c crafting.Cost) int64 {
	return int64(armory.CraftCharge(&armory.CraftPack{PayKind: p.PayKind, MoneyKey: p.MoneyKey}, craftingCents(c), c.Currency))
}

func (a *craftingArmoryAdapter) Spend(ctx context.Context, campaignID string, v crafting.Viewer, characterID string, items []crafting.PackItem, c crafting.Cost, label string) (string, error) {
	in := armory.CraftSpendInput{CharacterID: characterID, Cost: craftingCents(c), Currency: c.Currency, Label: label}
	for _, it := range items {
		in.Items = append(in.Items, armory.CraftItem{ItemID: it.ItemID, Quantity: it.Qty})
	}
	out, err := a.stash.CraftSpend(ctx, campaignID, craftingActor(v), in)
	if err != nil {
		return "", err
	}
	return out.Paid, nil
}

func (a *craftingArmoryAdapter) Give(ctx context.Context, campaignID string, v crafting.Viewer, characterID, itemID string, qty int, label string) error {
	return a.stash.CraftGive(ctx, campaignID, craftingActor(v), characterID, itemID, qty, label)
}

func (a *craftingArmoryAdapter) GiveSpoil(ctx context.Context, campaignID string, v crafting.Viewer, characterID, itemID string, qty int, harvested bool) error {
	return a.stash.SpoilGive(ctx, campaignID, craftingActor(v), characterID, itemID, qty, harvested)
}

// craftingSheetsAdapter reads a characteristic from a character's sheet
// fields, only for a page of the campaign asked about.
type craftingSheetsAdapter struct {
	entities entities.EntityService
}

func (a *craftingSheetsAdapter) Stat(ctx context.Context, campaignID, characterID, key string) (int, bool, error) {
	e, err := a.entities.GetByID(ctx, characterID)
	if err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
			return 0, false, nil
		}
		return 0, false, err
	}
	if e == nil || e.CampaignID != campaignID {
		return 0, false, nil
	}
	return sheetNumber(e.FieldsData[key])
}

// Skills lists the skills a character's sheet names. Systems keep them as
// text ("Alchemy, Herbalism") in a "skills" field, or as data in
// "skills_json" (a list of names, or of objects with a name); ok is false
// when the sheet has neither.
func (a *craftingSheetsAdapter) Skills(ctx context.Context, campaignID, characterID string) ([]string, bool, error) {
	e, err := a.entities.GetByID(ctx, characterID)
	if err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	if e == nil || e.CampaignID != campaignID {
		return nil, false, nil
	}
	return sheetSkills(e.FieldsData)
}

// sheetSkills reads the skills out of a sheet's fields.
func sheetSkills(fields map[string]any) ([]string, bool, error) {
	var out []string
	if raw, ok := fields["skills_json"].(string); ok && strings.TrimSpace(raw) != "" {
		var list []any
		if json.Unmarshal([]byte(raw), &list) == nil {
			for _, v := range list {
				switch x := v.(type) {
				case string:
					out = append(out, x)
				case map[string]any:
					if n, ok := x["name"].(string); ok {
						out = append(out, n)
					}
				}
			}
		}
	}
	if text, ok := fields["skills"].(string); ok {
		out = append(out, strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })...)
	}
	clean := out[:0]
	for _, k := range out {
		if k = strings.TrimSpace(k); k != "" {
			clean = append(clean, k)
		}
	}
	return clean, len(clean) > 0, nil
}

// craftingTablesAdapter is the campaign's Rolling Tables as crafting reads
// them, by name, so the crafting plugin never imports rolltables.
type craftingTablesAdapter struct{ svc rolltables.Service }

func (a craftingTablesAdapter) All(ctx context.Context, campaignID string) ([]crafting.CatalogTable, error) {
	doc, err := a.svc.Get(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]crafting.CatalogTable, 0, len(doc.Tables))
	for _, t := range doc.Tables {
		ct := crafting.CatalogTable{Name: t.Name, Rows: make([]string, 0, len(t.Entries))}
		for _, e := range t.Entries {
			ct.Rows = append(ct.Rows, tableRowText(e.Name, e.Brief))
		}
		out = append(out, ct)
	}
	return out, nil
}

// tableRowText is a row as crafting shows it: its name, then its brief.
func tableRowText(name, brief string) string {
	if brief != "" {
		return name + ". " + brief
	}
	return name
}

func (a craftingTablesAdapter) Rows(ctx context.Context, campaignID, name string) ([]crafting.TableRow, bool, error) {
	doc, err := a.svc.Get(ctx, campaignID)
	if err != nil {
		return nil, false, err
	}
	for _, t := range doc.Tables {
		if !strings.EqualFold(strings.TrimSpace(t.Name), strings.TrimSpace(name)) {
			continue
		}
		rows := make([]crafting.TableRow, 0, len(t.Entries))
		for _, e := range t.Entries {
			rows = append(rows, crafting.TableRow{Text: tableRowText(e.Name, e.Brief), Weight: e.Weight})
		}
		return rows, true, nil
	}
	return nil, false, nil
}

// sheetNumber reads a sheet value as a whole number: a number, or text
// holding one ("+2" counts).
func sheetNumber(v any) (int, bool, error) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false, nil
		}
		return int(math.Round(n)), true, nil
	case int:
		return n, true, nil
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false, nil
		}
		return int(math.Round(f)), true, nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(n), "+"), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false, nil
		}
		return int(math.Round(f)), true, nil
	}
	return 0, false, nil
}

// craftingCalendarService is the slice of the calendar a project roll reads.
type craftingCalendarService interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error)
	ListEventsForMonth(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]calendar.Event, error)
	ListDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]calendar.DayWeather, error)
}

// craftingDaysAdapter reads the default calendar's today for a project
// roll: its moons, events, rolled results, weather and season, read as the
// rules editor reads them. A recipe's rules are the Director's, so the day
// is read with the Director's sight (a trusted in-process read, ADR-049's
// allowlist); only the rule's effect, never the calendar, reaches a player,
// and the service shows today's date only to a viewer who can see the
// calendar.
type craftingDaysAdapter struct {
	cal craftingCalendarService
}

func (a *craftingDaysAdapter) Today(ctx context.Context, campaignID string) (*crafting.Day, error) {
	if a.cal == nil {
		return nil, nil
	}
	v := permissions.SystemViewer(permissions.RoleOwner)
	c, err := a.cal.GetDefaultCalendarForViewer(ctx, campaignID, v)
	if err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	if c == nil || c.CurrentMonth < 1 || c.CurrentDay < 1 {
		return nil, nil
	}
	y, m, d := c.CurrentYear, c.CurrentMonth, c.CurrentDay
	day := &crafting.Day{Label: strconv.Itoa(d) + " " + c.MonthName(m), Rolled: map[string]string{},
		Time: c.DayPart(y, m, d, c.CurrentHour, c.CurrentMinute)}
	abs := c.CurrentDayIndex()
	for i := range c.Moons {
		mo := &c.Moons[i]
		if mo.Manual() {
			day.Moons = append(day.Moons, crafting.DayMoon{Name: mo.Name, Manual: true, State: mo.StateOn(abs)})
		} else {
			day.Moons = append(day.Moons, crafting.DayMoon{Name: mo.Name, Phase: mo.MoonPhase(abs)})
		}
	}
	if s := c.SeasonForDate(m, d); s != nil {
		day.Season = s.Name
	}
	events, err := a.cal.ListEventsForMonth(ctx, c.ID, campaignID, y, m, v)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, e := range events {
		names[e.ID] = e.Name
		if eventOn(e, y, m, d) {
			day.Events = append(day.Events, e.Name)
		}
	}
	if rs, ok := a.cal.(calendar.EventRollService); ok {
		rolls, err := rs.ListMonthRolls(ctx, c.ID, campaignID, y, m, v)
		if err != nil && apperror.SafeCode(err) != http.StatusNotFound {
			return nil, err
		}
		for id, nights := range rolls {
			for _, n := range nights {
				if n.Year == y && n.Month == m && n.Day == d && n.Result != "" && names[id] != "" {
					day.Rolled[names[id]] = n.Result
				}
			}
		}
	}
	wx, err := a.cal.ListDayWeather(ctx, c.ID, campaignID, y, m, v)
	if err != nil {
		return nil, err
	}
	for _, w := range wx {
		if w.Year == y && w.Month == m && w.Day == d && w.PresetLabel != nil {
			day.Weather = *w.PresetLabel
		}
	}
	return day, nil
}

// eventOn reports whether an event of a month read falls on a day: a
// repeating event's own dates (skips and moves already applied), or a
// one-off event's start date, as the rules editor counts them.
func eventOn(e calendar.Event, y, m, d int) bool {
	if e.Occurrences != nil {
		for _, o := range e.Occurrences {
			if !o.Skipped && o.Year == y && o.Month == m && o.Day == d {
				return true
			}
		}
		return false
	}
	return e.Year == y && e.Month == m && e.Day == d
}

// craftingRespiteHook runs the Homebrew rules a respite sets off when the
// Director logs one from the Crafting app, as the drawer's button does.
type craftingRespiteHook struct {
	homebrew homebrew.Service // nil while the Homebrew plugin is degraded
}

func (h *craftingRespiteHook) RespiteLogged(ctx context.Context, campaignID string, v crafting.Viewer) error {
	if h.homebrew == nil {
		return nil
	}
	return h.homebrew.Fire(ctx, campaignID, homebrew.Viewer{UserID: v.UserID, Director: v.Director, VisibilityRole: v.VisibilityRole},
		homebrew.Fired{Event: respiteEvent})
}

// craftingRespiteListener gives every project a roll when the Director
// presses "Log a respite" in the Homebrew drawer, in campaigns with
// crafting on.
type craftingRespiteListener struct {
	svc     crafting.Service
	enabled func(ctx context.Context, campaignID string) bool
}

func (l *craftingRespiteListener) RespitePressed(ctx context.Context, campaignID string, by homebrew.Viewer) error {
	if l.enabled != nil && !l.enabled(ctx, campaignID) {
		return nil
	}
	return l.svc.RespitePressed(ctx, campaignID, crafting.Viewer{UserID: by.UserID, Director: by.Director, VisibilityRole: by.VisibilityRole})
}

// registerCraftingBlock adds the Crafting block a character page can carry:
// the hero's projects and the recipes they know. It renders for the
// Director and for the player who claimed the character, and for no one
// else, so a stranger's page shows no empty block.
func registerCraftingBlock(blockRegistry *entities.BlockRegistry, svc crafting.Service) {
	blockRegistry.Register(entities.BlockMeta{
		Type: crafting.BlockType, Label: "Crafting", Icon: "fa-flask",
		Description: "The hero's crafting projects and the recipes they know",
		Addon:       crafting.AddonSlug,
		Contexts:    []string{"template"},
		TileSizes:   []string{"2x2", "4x2", "1x2"},
		Singleton:   true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		if rc.CC == nil || rc.CC.Campaign == nil || rc.Entity == nil {
			return templ.NopComponent
		}
		v := crafting.Viewer{
			UserID:         rc.UserID,
			MemberRole:     int(rc.CC.MemberRole),
			VisibilityRole: rc.CC.VisibilityRole(),
			Director:       rc.CC.CanAuthorDmOnly(),
		}
		ok, err := svc.CanCraftFor(context.Background(), rc.CC.Campaign.ID, rc.Entity.ID, v)
		if err != nil {
			slog.Error("crafting: cannot tell who may craft for the page",
				slog.String("entity_id", rc.Entity.ID), slog.Any("error", err))
			return templ.NopComponent
		}
		if !ok {
			return templ.NopComponent
		}
		return crafting.BlockMount(rc.CC.Campaign.ID, rc.Entity.ID, rc.CSRFToken)
	})
}
