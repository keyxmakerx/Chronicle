package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/observability"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// operator_diag_adapters.go injects read-only campaign-data windows into the
// operator diagnostics (the AI Workflow Support workspace). The systems package
// must not import entities, so the app layer implements the provider interfaces
// here and wires them at startup — mirroring SetInstalledPackagesProvider.

// entityDiagAdapter implements systems.EntityDiagProvider against the entities
// service: a read-only reader for entity.fields (one entity's stored values) and
// entity.field-coverage (how populated a type's declared fields are). Read-only
// by construction — only Get/List calls, never a mutation.
type entityDiagAdapter struct {
	entities entities.EntityService
}

// EntityFields resolves an entity by ID (then by slug within the campaign) and
// returns its stored field map. Campaign scope is enforced so the diagnostic
// can't read across campaigns.
func (a entityDiagAdapter) EntityFields(ctx context.Context, campaignID, idOrSlug string) (systems.EntityFieldDump, error) {
	ent, err := a.entities.GetByID(ctx, idOrSlug)
	if err != nil || ent == nil || (campaignID != "" && ent.CampaignID != campaignID) {
		ent, err = a.entities.GetBySlug(ctx, campaignID, idOrSlug)
	}
	if err != nil {
		return systems.EntityFieldDump{}, err
	}
	if ent == nil || (campaignID != "" && ent.CampaignID != campaignID) {
		return systems.EntityFieldDump{Found: false}, nil
	}
	return systems.EntityFieldDump{
		Found:    true,
		ID:       ent.ID,
		Name:     ent.Name,
		TypeName: ent.TypeName,
		Fields:   ent.FieldsData,
	}, nil
}

// TypeFieldCoverage counts, for each declared field of a type, how many of its
// entities have a non-empty value. Entities are pulled with a generous cap; role
// 3 (owner) sees all.
func (a entityDiagAdapter) TypeFieldCoverage(ctx context.Context, campaignID, typeRef string) (systems.FieldCoverage, error) {
	et, err := a.resolveType(ctx, campaignID, typeRef)
	if err != nil {
		return systems.FieldCoverage{}, err
	}
	if et == nil {
		return systems.FieldCoverage{Found: false}, nil
	}
	list, _, err := a.entities.List(ctx, campaignID, et.ID, 3, "", entities.ListOptions{Page: 1, PerPage: 1000, Sort: "name"})
	if err != nil {
		return systems.FieldCoverage{}, err
	}
	rows := make([]systems.FieldCoverageRow, 0, len(et.Fields))
	for _, fd := range et.Fields {
		n := 0
		for i := range list {
			if nonEmptyField(list[i].FieldsData, fd.Key) {
				n++
			}
		}
		rows = append(rows, systems.FieldCoverageRow{Key: fd.Key, Label: fd.Label, NonEmpty: n})
	}
	return systems.FieldCoverage{Found: true, TypeName: et.Name, EntityCount: len(list), Declared: rows}, nil
}

// EntityTypes lists a campaign's entity types with per-type entity counts, for
// the entity.types discovery diagnostic.
func (a entityDiagAdapter) EntityTypes(ctx context.Context, campaignID string) ([]systems.EntityTypeInfo, error) {
	types, err := a.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	// role 3 (owner) so counts include all entities.
	counts, err := a.entities.CountByType(ctx, campaignID, 3, "")
	if err != nil {
		counts = map[int]int{} // counts are best-effort; still list the types
	}
	out := make([]systems.EntityTypeInfo, 0, len(types))
	for i := range types {
		preset := ""
		if types[i].PresetCategory != nil {
			preset = *types[i].PresetCategory
		}
		out = append(out, systems.EntityTypeInfo{
			ID:             types[i].ID,
			Name:           types[i].Name,
			Slug:           types[i].Slug,
			PresetCategory: preset,
			Count:          counts[types[i].ID],
		})
	}
	return out, nil
}

// FindEntities searches a campaign's entities by name/slug for the entity.find
// discovery diagnostic. role 3 (owner) so it can see all entities.
func (a entityDiagAdapter) FindEntities(ctx context.Context, campaignID, query string) ([]systems.EntityHit, error) {
	list, _, err := a.entities.Search(ctx, campaignID, query, 0, 3, "", entities.ListOptions{Page: 1, PerPage: 25, Sort: "name"})
	if err != nil {
		return nil, err
	}
	out := make([]systems.EntityHit, 0, len(list))
	for i := range list {
		out = append(out, systems.EntityHit{
			ID:       list[i].ID,
			Name:     list[i].Name,
			Slug:     list[i].Slug,
			TypeName: list[i].TypeName,
		})
	}
	return out, nil
}

// resolveType accepts a numeric type id, a slug, or a (case-insensitive) name.
func (a entityDiagAdapter) resolveType(ctx context.Context, campaignID, ref string) (*entities.EntityType, error) {
	if id, err := strconv.Atoi(ref); err == nil {
		return a.entities.GetEntityTypeByID(ctx, id)
	}
	if et, err := a.entities.GetEntityTypeBySlug(ctx, campaignID, ref); err == nil && et != nil {
		return et, nil
	}
	types, err := a.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	for i := range types {
		if strings.EqualFold(types[i].Name, ref) || strings.EqualFold(types[i].Slug, ref) {
			return &types[i], nil
		}
	}
	return nil, nil
}

// embeddedAssetSets reports every registered plugin's //go:embed-ed static
// filesystem for the host.embedded diagnostics. Those bytes live only inside
// the executable, so nothing an operator can run against the container
// filesystem will ever see them — this adapter is the only window onto them.
//
// It reads a.registeredPlugins THROUGH the exported accessor at CALL time, not
// at wiring time, so it does not care whether it is wired before or after the
// plugins register. It mirrors the mount in mountPluginStatic exactly, sharing
// pluginStaticPrefix so the reported URL cannot drift from the served one.
func (a *App) embeddedAssetSets() []systems.EmbeddedAssetSet {
	regs := a.RegisteredPlugins()
	out := make([]systems.EmbeddedAssetSet, 0, len(regs))
	for _, p := range regs {
		if p.StaticFS == nil {
			continue // no embedded assets; not an error
		}
		out = append(out, systems.EmbeddedAssetSet{
			Slug:      p.Slug,
			URLPrefix: pluginStaticPrefix(p.Slug) + "/",
			FS:        p.StaticFS,
		})
	}
	return out
}

// hostPluginRows merges Chronicle's plugin registries into the flat rows
// host.plugins reports. It is the app layer's job because internal/systems must
// not import internal/database or the plugin packages — the same dependency
// inversion as embeddedAssetSets.
//
// There are THREE registries and they contain different plugins:
//
//   - a.registeredPlugins (PluginRegistration): opt-in metadata. Carries the
//     StaticFS and the health hook. Four plugins today.
//   - a.PluginSchemas (database.PluginSchema): the plugins that own tables and
//     handed migrations to the runner. Nine today.
//   - a.PluginHealth: the runner's record of what those migrations DID.
//
// None of them is a manifest of what exists — every plugin is compiled in and
// registers its routes unconditionally. The renderer says so on every run;
// this function's job is only to stop the merge itself from lying.
//
// THE MERGE KEY IS NORMALISED, and that is not cosmetic: foundry_vtt registers
// as `foundry-vtt` in the metadata registry (that spelling is also its static
// URL prefix) and as `foundry_vtt` in the schema runner and health registry.
// Keyed literally, one plugin would render as two rows — one apparently
// migration-less, one apparently unregistered — which is exactly the kind of
// half-true inventory this whole workstream exists to stop. The row carries
// BOTH spellings so the reader learns the alias rather than having it hidden.
func (a *App) hostPluginRows() []systems.HostPlugin {
	rows := map[string]*systems.HostPlugin{}
	get := func(key, display string) *systems.HostPlugin {
		k := normalizePluginKey(key)
		if r, ok := rows[k]; ok {
			return r
		}
		r := &systems.HostPlugin{Slug: display}
		rows[k] = r
		return r
	}

	for _, p := range a.RegisteredPlugins() {
		r := get(p.Slug, p.Slug)
		r.Slug = p.Slug // the metadata spelling wins as the display name: it is the one in the URL prefix
		r.InMetadataRegistry = true
		r.HasHealthCheck = p.HealthCheck != nil
		if p.HealthCheck != nil {
			// Every health callback registered today is an in-memory lookup
			// against the migration health registry, so calling them is cheap
			// and read-only. If one ever needs to touch the network or the DB,
			// it should be gated rather than silently made expensive here.
			if err := p.HealthCheck(); err != nil {
				r.HealthErr = err.Error()
			}
		}
		if p.StaticFS != nil {
			r.StaticURLPrefix = pluginStaticPrefix(p.Slug) + "/"
			r.StaticFS = p.StaticFS
		}
	}

	for _, s := range a.PluginSchemas {
		r := get(s.Slug, s.Slug)
		r.SchemaKey = s.Slug
		r.DeclaresMigrations = true
	}

	if a.PluginHealth != nil {
		for slug, h := range a.PluginHealth.All() {
			r := get(slug, slug)
			if r.SchemaKey == "" {
				r.SchemaKey = slug
			}
			r.SchemaKnown = true
			r.SchemaHealthy = h.Healthy
			r.SchemaVersion = h.Version
			r.SchemaLatest = h.LatestVersion
			r.SchemaError = h.Error
		}
	}

	out := make([]systems.HostPlugin, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	return out
}

// normalizePluginKey folds the two spellings Chronicle uses for the same plugin
// (`foundry-vtt` vs `foundry_vtt`) onto one merge key. Deliberately narrow: it
// only lowercases and unifies `_`/`-`, so two genuinely different plugins
// cannot be collapsed by it.
func normalizePluginKey(slug string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(slug)), "_", "-")
}

// recentErrorSnapshot reports the process error ring for the host.errors /
// host.errors-summary diagnostics.
//
// The translation between observability.Entry and systems.RecentError is
// deliberate, not incidental: internal/systems must not import the writer side
// of the ring any more than it imports the plugins, so the app layer owns the
// only place the two shapes meet. Same dependency inversion as
// embeddedAssetSets and SetInstalledPackagesProvider.
//
// limit <= 0 means "every held entry" and is passed straight through, because
// the summary groups over the whole ring — a frequency computed over one page
// would be a lie about how often something is failing.
func recentErrorSnapshot(limit int) systems.RecentErrors {
	s := observability.Recent(limit)
	out := systems.RecentErrors{Capacity: s.Capacity, Held: s.Held, Total: s.Total}
	out.Entries = make([]systems.RecentError, 0, len(s.Entries))
	for _, e := range s.Entries {
		out.Entries = append(out.Entries, systems.RecentError{
			Time:           e.Time,
			Status:         e.Status,
			Method:         e.Method,
			Path:           e.Path,
			PathIsTemplate: e.PathIsTemplate,
			Kind:           string(e.Kind),
			Err:            e.Err,
		})
	}
	return out
}

// nonEmptyField reports whether fields_data has a meaningful value for key.
func nonEmptyField(fd map[string]any, key string) bool {
	if fd == nil {
		return false
	}
	v, ok := fd[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	default:
		return true
	}
}
