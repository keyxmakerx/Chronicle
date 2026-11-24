package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// system_sheet_fields.go gives the categories a game system's sheets live on
// the system's own fields, and tells AI Import what the system says about
// them. A system adds its fields to the type its preset matches by name or
// preset category, so a campaign whose heroes live in "Character" and whose
// monsters live in a "Creature" made before the system was enabled never got
// them, and a field a later system version adds reaches no category unless
// something offers it. Those fields are also how GM-only values are kept from
// players, so they belong on the category, not just in the page data Foundry
// writes.

// campaignManifest is the campaign's game system: its custom upload, else its
// chosen built-in; nil when it has none.
type campaignManifest func(ctx context.Context, campaignID string) *systems.SystemManifest

// sheetFieldTypes is the slice of the entity service the sweep needs.
type sheetFieldTypes interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	ReconcileEntityTypeFields(ctx context.Context, typeID int, declared []entities.FieldDefinition) (int, error)
}

// presetTypeIDs maps each of the system's sheet presets to the campaign's
// categories it belongs on: the character preset to the player-character
// list's types (and any type already bound to that preset), the creature
// preset to the creature types.
func presetTypeIDs(ctx context.Context, m *systems.SystemManifest, types []entities.EntityType, lists entities.CharacterListReader, campaignID string) map[*systems.EntityPresetDef][]int {
	out := map[*systems.EntityPresetDef][]int{}
	for i := range m.EntityPresets {
		p := &m.EntityPresets[i]
		var ids []int
		switch p.Category {
		case presetCategoryCharacter:
			seen := map[int]bool{}
			if lists != nil {
				if pcs, err := lists.CharacterTypeIDs(ctx, campaignID); err == nil {
					for _, id := range pcs {
						seen[id] = true
						ids = append(ids, id)
					}
				}
			}
			for _, t := range types {
				if t.PresetCategory != nil && *t.PresetCategory == p.Category && !seen[t.ID] {
					ids = append(ids, t.ID)
				}
			}
		case "creature":
			ids = entities.CreatureTypeIDs(types)
		}
		if len(ids) > 0 {
			out[p] = ids
		}
	}
	return out
}

// sheetFieldsKeyPrefix starts the per-campaign site setting that records which
// of its system's sheet fields have already been offered to its categories.
const sheetFieldsKeyPrefix = "sheet_fields."

// sheetFieldsRecord is what a campaign's setting holds: the system it was
// taken for and, per preset slug, the field keys that system declared.
type sheetFieldsRecord struct {
	System  string              `json:"system"`
	Presets map[string][]string `json:"presets"`
}

// readSheetFieldsRecord is the campaign's record, or nil when it has none or
// it cannot be read: every field the categories lack is then offered.
func readSheetFieldsRecord(ctx context.Context, st pageExtrasSettings, campaignID string) *sheetFieldsRecord {
	v, err := st.Get(ctx, sheetFieldsKeyPrefix+campaignID)
	if err != nil || v == "" {
		return nil
	}
	var rec sheetFieldsRecord
	if json.Unmarshal([]byte(v), &rec) != nil {
		return nil
	}
	return &rec
}

// addSystemSheetFields gives each campaign's categories the sheet fields its
// game system declares that they lack, and runs on every boot. A system
// version can add a field however it arrives (an update in Admin, a restart
// onto a newer package, a custom upload), and the pages already hold its
// value from Foundry or the sheet, but the AI tools, the Customizer and the
// player filter only see a field the category declares. Each campaign
// records the keys it was offered, so a field the owner deleted is not
// offered again; a field new to the system is. A campaign whose categories
// could not all be updated is not recorded, so the next boot tries again.
func addSystemSheetFields(ctx context.Context, st pageExtrasSettings, camps pageExtrasCampaigns, manifest campaignManifest, ents sheetFieldTypes, lists entities.CharacterListReader) (int, error) {
	added := 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return added, fmt.Errorf("listing campaigns: %w", err)
		}
		for _, c := range list {
			added += addCampaignSheetFields(ctx, st, c.ID, manifest(ctx, c.ID), ents, lists)
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	return added, nil
}

// addCampaignSheetFields is addSystemSheetFields for one campaign.
func addCampaignSheetFields(ctx context.Context, st pageExtrasSettings, campaignID string, m *systems.SystemManifest, ents sheetFieldTypes, lists entities.CharacterListReader) int {
	if m == nil {
		return 0
	}
	types, err := ents.GetEntityTypes(ctx, campaignID)
	if err != nil {
		slog.Warn("system sheet fields: entity types unavailable",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return 0
	}
	// Keys offered under another system say nothing about this one's.
	known := map[string]map[string]bool{}
	if rec := readSheetFieldsRecord(ctx, st, campaignID); rec != nil && rec.System == m.ID {
		for slug, keys := range rec.Presets {
			set := map[string]bool{}
			for _, k := range keys {
				set[k] = true
			}
			known[slug] = set
		}
	}
	added, failed := 0, false
	for p, ids := range presetTypeIDs(ctx, m, types, lists, campaignID) {
		var declared []entities.FieldDefinition
		for _, f := range mapPresetFields(p.Fields) {
			if !known[p.Slug][f.Key] {
				declared = append(declared, f)
			}
		}
		if len(declared) == 0 {
			continue
		}
		for _, id := range ids {
			n, err := ents.ReconcileEntityTypeFields(ctx, id, declared)
			if err != nil {
				failed = true
				slog.Warn("system sheet fields: adding failed",
					slog.String("campaign_id", campaignID), slog.Int("entity_type_id", id), slog.Any("error", err))
				continue
			}
			added += n
		}
	}
	if failed {
		return added
	}
	rec := sheetFieldsRecord{System: m.ID, Presets: map[string][]string{}}
	changed := false
	for _, p := range m.EntityPresets {
		keys := make([]string, 0, len(p.Fields))
		for _, f := range p.Fields {
			keys = append(keys, f.Key)
			if !known[p.Slug][f.Key] {
				changed = true
			}
		}
		rec.Presets[p.Slug] = keys
	}
	if !changed {
		return added
	}
	b, err := json.Marshal(rec)
	if err == nil {
		err = st.Set(ctx, sheetFieldsKeyPrefix+campaignID, string(b))
	}
	if err != nil {
		slog.Warn("system sheet fields: recording failed",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
	return added
}

// aiFieldNotes tells AI Import's brief what the campaign's game system says
// about a category's fields.
type aiFieldNotes struct {
	manifest campaignManifest
	lists    entities.CharacterListReader
	types    interface {
		GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	}
	// choices names the character fields that have a pick list. Nil leaves
	// pick-list fields described as plain text.
	choices interface {
		CreatorPlan(ctx context.Context, campaignID string) (*systems.CreatorPlan, error)
	}
	// linked says whether a Foundry world syncs with the campaign. Nil is
	// read as linked, the safe side: a value written here would be replaced.
	linked func(ctx context.Context, campaignID string) bool
}

// pickListNote tells the AI that a field takes a name from the campaign's
// pick list and how to read that list. The list itself stays out of the
// brief: it can be long, and Directors add their own entries to it.
func pickListNote(step systems.CreatorStep) string {
	note := "A name from the campaign's `" + step.FieldKey + "` list: read it with the `system-entries` lookup (connector tool `system_entries`, field `" + step.FieldKey + "`) before writing a hero"
	if step.Parent != "" {
		note += "; it must belong to the chosen `" + step.Parent + "`"
	}
	return note + ". The field holds the name only: a homemade one is made first with `kind: system-entry`, `entry: " + step.FieldKey + "`"
}

// pickLists is the character fields that have a pick list, by key.
func (n aiFieldNotes) pickLists(ctx context.Context, campaignID string) map[string]systems.CreatorStep {
	if n.choices == nil {
		return nil
	}
	// The keys alone are used, so the Director's view is the whole set.
	plan, err := n.choices.CreatorPlan(systems.WithChoiceViewer(ctx, true), campaignID)
	if err != nil || plan == nil {
		return nil
	}
	out := make(map[string]systems.CreatorStep, len(plan.Steps))
	for _, st := range plan.Steps {
		out[st.FieldKey] = st
	}
	return out
}

// foundryKeeps reports whether Foundry is where a character field's value
// comes from: it is read off the actor's items, or never written back. A
// value written here would be replaced at the next sync. Identity picks
// (ancestry, kit, complication...) are the exception: the Foundry module
// swaps the actor's item to match Chronicle's pick.
func foundryKeeps(f systems.FieldDef) bool {
	if f.FoundryCollection != "" {
		return !syncsBackToFoundry(f)
	}
	return f.FoundryPath != "" && f.FoundryWritable != nil && !*f.FoundryWritable
}

// syncsBackToFoundry mirrors the Foundry module's identity-field rule: a
// single item of one concrete type, other than class and subclass (they
// drive the system's advancement) and the values read off the class item.
func syncsBackToFoundry(f systems.FieldDef) bool {
	if f.FoundryCollection == "" || !f.FoundryItemSingle || len(f.FoundryItemType) != 1 {
		return false
	}
	switch f.Key {
	case "level", "heroic_resource_name":
		return false
	}
	switch f.FoundryItemType[0] {
	case "", "class", "subclass":
		return false
	}
	return true
}

// FieldNotes is the system's notes on t's fields by key, or nil.
func (n aiFieldNotes) FieldNotes(ctx context.Context, campaignID string, t entities.EntityType) map[string]string {
	if n.manifest == nil || n.types == nil {
		return nil
	}
	m := n.manifest(ctx, campaignID)
	if m == nil {
		return nil
	}
	types, err := n.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for p, ids := range presetTypeIDs(ctx, m, types, n.lists, campaignID) {
		for _, id := range ids {
			if id != t.ID {
				continue
			}
			var picks map[string]systems.CreatorStep
			var foundry bool
			if p.Category == presetCategoryCharacter {
				picks = n.pickLists(ctx, campaignID)
				// Where a campaign has no Foundry world the sheet lets a person
				// edit these by hand, so the AI writes them like any other.
				foundry = n.linked == nil || n.linked(ctx, campaignID)
			}
			// A category the system's sheet lives on can still carry the
			// starting set's fields from another game (race, alignment), so
			// those are named as not the system's own.
			own := map[string]bool{}
			for _, f := range p.Fields {
				own[f.Key] = true
			}
			system := m.Name
			if system == "" {
				system = "game system"
			}
			for _, d := range t.Fields {
				if !own[d.Key] {
					out[d.Key] = "Not a " + system + " field; fill it only when I ask for it"
				}
			}
			for _, f := range p.Fields {
				note := f.Format
				// A preset's pull list names what Foundry sends back live, such
				// as a creature's current Stamina; a write here would be
				// replaced at the next sync, or overwrite what the table tracks.
				pulled := slices.Contains(p.FoundryPullFields, f.Key)
				kept := foundry && foundryKeeps(f)
				if kept || pulled {
					if note != "" {
						note += ". "
					}
					note += "Foundry keeps this one: leave it out"
				} else if st, ok := picks[f.Key]; ok {
					if note != "" {
						note += ". "
					}
					note += pickListNote(st)
				}
				if f.FoundryItemJoin && !kept && !pulled {
					if note != "" {
						note += ". "
					}
					note += "Two or more picks are one text, comma-separated (`A, B`), not a list"
				}
				if r := rangeNote(f); r != "" && !pulled {
					if note != "" {
						note += ". "
					}
					note += r
				}
				if note != "" {
					out[f.Key] = note
				}
			}
		}
	}
	return out
}

// rangeNote tells the AI the bounds the review will hold a number to, so it
// writes a sensible figure the first time rather than having the row flagged.
func rangeNote(f systems.FieldDef) string {
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	switch {
	case f.Min != nil && f.Max != nil:
		return "A number from " + num(*f.Min) + " to " + num(*f.Max)
	case f.Min != nil:
		return "A number of at least " + num(*f.Min)
	case f.Max != nil:
		return "A number of at most " + num(*f.Max)
	}
	return ""
}
