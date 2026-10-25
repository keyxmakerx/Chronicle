package app

import (
	"context"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/pageblocks"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/systems"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// aiDeps is what AI Import's record kinds and lookups are built from. A
// plugin service is nil while its plugin is degraded; the derived fields are
// filled by newAIDeps.
type aiDeps struct {
	Calendar        calendar.CalendarService
	Campaigns       campaigns.CampaignService
	CharacterChoice systems.CharacterChoiceService
	DMScreen        dmscreen.Service
	Drawings        maps.DrawingService
	Encounters      encounters.Service
	Entities        entities.EntityService
	FieldRules      fieldrules.Service
	Folders         entities.FolderService
	Health          *database.PluginHealthRegistry
	Maps            maps.MapService
	Notes           notes.NoteService
	PageBlocks      pageblocks.Service
	Quests          quests.QuestService // nil while the quests plugin is degraded
	Boards          quests.BoardService
	Relations       relations.RelationService
	RollTables      rolltables.Service
	Sessions        sessions.SessionService
	SystemEntries   systems.SystemEntryService
	Systems         *systems.SystemHandler
	Tags            tags.TagService
	PageLayouts     entities.PageLayoutService // nil while its tables are degraded
	FoundryScenes   records.FoundryScenesAPI   // nil while the sync API is degraded

	// Derived once and shared by several kinds and the lookups.
	Rolls      records.EventRollAPI // the calendar's roll half; nil without one
	Tables     records.TableKind
	HouseRules *records.HouseRuleKind
	Homebrew   records.HomebrewAPI // nil while field rules are degraded
	Nights     aiGameNightsAdapter
	PageMaps   records.PageMapsAPI // nil while page blocks are degraded
}

// newAIDeps fills the derived fields from the services.
func newAIDeps(d aiDeps) *aiDeps {
	if rs, ok := d.Calendar.(calendar.EventRollService); ok {
		d.Rolls = rs
	}
	d.Tables = records.TableKind{Svc: aiRollTablesAdapter{d.RollTables}}
	campaignsSvc := d.Campaigns
	d.HouseRules = &records.HouseRuleKind{Book: d.Systems, SystemOf: func(ctx context.Context, campaignID string) string {
		c, err := campaignsSvc.GetByID(ctx, campaignID)
		if err != nil {
			return ""
		}
		return c.ParseSettings().SystemID
	}}
	if d.FieldRules != nil {
		d.Homebrew = aiHomebrewAdapter{svc: d.FieldRules}
	}
	d.Nights = aiGameNightsAdapter{svc: d.Sessions, members: d.Campaigns}
	if d.PageBlocks != nil {
		d.PageMaps = aiPageMapsAdapter{svc: d.PageBlocks}
	}
	return &d
}

// aiKindSource is one feature's record kinds. Rank places them in the
// prompt: lower comes first, and equal ranks keep registration order.
type aiKindSource struct {
	Rank  int
	Kinds func(d *aiDeps) []records.Kind
}

// aiKindSources is filled from init, so a new kind adds a file with its own
// addAIKinds call instead of editing RegisterRoutes or a shared list. Pick a
// rank between the neighbours it should sit with in the prompt.
var aiKindSources []aiKindSource

func addAIKinds(rank int, kinds func(d *aiDeps) []records.Kind) {
	aiKindSources = append(aiKindSources, aiKindSource{Rank: rank, Kinds: kinds})
}

// aiKinds lists every source's kinds in rank order. A source returns nil
// while its plugin is degraded, so its blocks are refused at review.
func aiKinds(d *aiDeps) []records.Kind {
	sources := append([]aiKindSource(nil), aiKindSources...)
	sort.SliceStable(sources, func(i, j int) bool { return sources[i].Rank < sources[j].Rank })
	var out []records.Kind
	for _, s := range sources {
		out = append(out, s.Kinds(d)...)
	}
	return out
}

// aiLookups is the read-only lookups an AI may ask for.
func aiLookups(d *aiDeps, reg *records.Registry) *records.Lookups {
	l := &records.Lookups{
		Folders:  aiFolderNames{folders: d.Folders},
		Cal:      d.Calendar,
		Weather:  d.Calendar,
		Rolls:    d.Rolls,
		Maps:     aiMapsAdapter{d.Maps},
		Pages:    d.Entities,
		Rels:     d.Relations,
		Notes:    d.Notes,
		Rules:    d.HouseRules,
		Entries:  aiSystemEntriesAdapter{choices: d.CharacterChoice},
		Party:    &aiPartyAdapter{screen: d.DMScreen, nights: d.Sessions, members: d.Campaigns},
		Tags:     d.Tags,
		Tiers:    d.Campaigns,
		Nights:   d.Nights,
		Kinds:    reg,
		PageMaps: d.PageMaps,
		Menu:     aiMenuAdapter{campaigns: d.Campaigns, types: d.Entities},
	}
	if d.Health.IsHealthy(rolltables.PluginSlug) {
		l.Tables = d.Tables.Svc
	}
	if d.Notes != nil {
		l.Journal = aiJournalAdapter{notes: d.Notes, members: d.Campaigns}
	}
	if d.PageLayouts != nil {
		l.Layouts = d.PageLayouts
	}
	if d.FoundryScenes != nil {
		l.Scenes = d.FoundryScenes
	}
	if d.Quests != nil && d.Boards != nil {
		l.Quests = aiQuestsAdapter{quests: d.Quests, boards: d.Boards, types: d.Entities}
	}
	return l
}

// The kinds that existed before kinds registered themselves, in their prompt
// order. Ranks step by 10 so a new kind can sit between any two.
func init() {
	addAIKinds(10, func(d *aiDeps) []records.Kind {
		return []records.Kind{
			records.CalendarKind{Svc: d.Calendar, Pages: d.Entities},
			records.EventKind{Svc: d.Calendar, Tiers: d.Campaigns, Pages: d.Entities, Rolls: d.Rolls, Tables: d.Tables.Svc},
			records.WeatherKind{Svc: d.Calendar},
		}
	})
	addAIKinds(40, func(d *aiDeps) []records.Kind {
		if d.Rolls == nil {
			return nil
		}
		return []records.Kind{records.RollNightKind{Cal: d.Calendar, Rolls: d.Rolls}}
	})
	// Moons set by hand: the calendar service's moon mark half.
	addAIKinds(50, func(d *aiDeps) []records.Kind {
		ms, ok := d.Calendar.(calendar.MoonMarkService)
		if !ok {
			return nil
		}
		return []records.Kind{records.MoonKind{Cal: d.Calendar, Marks: ms}}
	})
	addAIKinds(60, func(d *aiDeps) []records.Kind {
		if !d.Health.IsHealthy(rolltables.PluginSlug) {
			return nil
		}
		return []records.Kind{d.Tables}
	})
	addAIKinds(70, func(d *aiDeps) []records.Kind {
		return []records.Kind{
			records.ShopStockKind(d.Entities, d.Relations),
			records.CarriedItemKind(d.Entities, d.Relations),
			records.PinKind{Svc: aiMapsAdapter{d.Maps}, Entities: d.Entities},
			records.MapSketchKind{Svc: aiMapSketchAdapter{maps: d.Maps, drawings: d.Drawings}, Pins: aiMapsAdapter{d.Maps}},
			records.NoteKind{Svc: d.Notes, Entities: d.Entities},
			*d.HouseRules,
			records.SystemEntryKind{Svc: d.SystemEntries},
		}
	})
	addAIKinds(140, func(d *aiDeps) []records.Kind {
		if d.Homebrew == nil {
			return nil
		}
		return []records.Kind{
			records.FieldRuleKind{Svc: d.Homebrew},
			records.LocalWordKind{Svc: d.Homebrew, Entities: d.Entities},
		}
	})
	// A category field of type rule needs the rules; without them the kind
	// still adds every other kind of field.
	addAIKinds(160, func(d *aiDeps) []records.Kind {
		return []records.Kind{records.CategoryFieldKind{Types: d.Entities, Rules: d.Homebrew}}
	})
	addAIKinds(170, func(d *aiDeps) []records.Kind {
		if d.Encounters == nil {
			return nil
		}
		return []records.Kind{records.EncounterKind{Svc: aiEncountersAdapter{svc: d.Encounters}, Entities: d.Entities}}
	})
	addAIKinds(180, func(d *aiDeps) []records.Kind {
		return []records.Kind{
			records.FolderKind{Svc: aiFoldersAdapter{folders: d.Folders, types: d.Entities}},
			records.GameNightKind{Svc: d.Nights},
			records.TagKind{Svc: d.Tags, Pages: d.Entities},
		}
	})
	addAIKinds(210, func(d *aiDeps) []records.Kind {
		if d.PageMaps == nil {
			return nil
		}
		return []records.Kind{records.PageMapKind{Svc: d.PageMaps, Entities: d.Entities}}
	})
	addAIKinds(220, func(d *aiDeps) []records.Kind {
		return []records.Kind{records.GeneratorKind{Cal: d.Calendar, Tables: d.Tables, Weather: d.Calendar, Rolls: d.Rolls}}
	})
}

// aiAddonChecker is the add-on switch AI Import's feature gate reads.
type aiAddonChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID, addonSlug string) (bool, error)
}

// aiFeatureGate names, by kind, the features a campaign has switched off,
// using the same add-ons their pages are gated on: the calendar and what is
// written onto it, game nights (which need the calendar too), and maps. A
// failed check counts the feature as on, so a lookup error never blocks an
// import; the feature's own routes still gate it.
func aiFeatureGate(addons aiAddonChecker) func(ctx context.Context, campaignID string) records.OffFeatures {
	return func(ctx context.Context, campaignID string) records.OffFeatures {
		off := func(slug string) bool {
			on, err := addons.IsEnabledForCampaign(ctx, campaignID, slug)
			return err == nil && !on
		}
		out := records.OffFeatures{}
		if off(calendar.PluginSlug) {
			for _, k := range []string{records.KindCalendar, "event", "weather", "roll-night", "moon", "game-night"} {
				out[k] = "Calendar"
			}
		}
		if off(sessions.SessionsAddonSlug) {
			out["game-night"] = "Game nights"
		}
		if off("maps") {
			out["map"], out["pin"] = "Maps", "Maps"
		}
		return out
	}
}
