// Package systems — operator_diag_entities.go adds the data-tracing half of the
// operator diagnostics: read-only inspectors for served file CONTENT, an entity's
// stored field values, a type's field-population coverage, and the most recent
// inbound sync payloads. Together they form a three-way "where does the data die?"
// compare:
//
//	Foundry SENT  (sync.inbound)  ->  Chronicle STORED (entity.fields)  ->  schema DECLARED (entity.field-coverage)
//
// All read-only, redacted by RunDiagnostic's redactSecrets pass, admin-gated and
// audited by the AI-workflow workspace. Cross-layer reads (entity data, inbound
// sync) arrive via injected providers — systems must not import entities/syncapi,
// so the app layer wires these at startup (mirrors SetInstalledPackagesProvider).
package systems

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ── system.file-contains: does a served file contain a marker? ──────────────

// renderFileContains reads a served file (clamped to the system's dir) and reports
// whether each comma-separated marker is present. Arg: "<system-id>:<relpath>:<markers>".
// SplitN keeps colons inside the marker (e.g. a URL) intact.
func renderFileContains(arg string) string {
	var b strings.Builder
	parts := strings.SplitN(arg, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		b.WriteString("## system.file-contains\n\n_Usage: `<system-id>:<relative-path>:<marker[,marker2,...]>`_\n")
		return b.String()
	}
	systemID, relPath, markerCSV := parts[0], parts[1], parts[2]
	fmt.Fprintf(&b, "## system.file-contains %s\n\n", arg)

	var dir string
	for _, s := range LoadedHealth() {
		if s.ID == systemID {
			dir = s.Dir
			break
		}
	}
	if dir == "" {
		fmt.Fprintf(&b, "_No loaded system with id `%s`._\n", systemID)
		return b.String()
	}

	data, ok, tooLarge := readClampedFile(dir, relPath)
	if tooLarge {
		fmt.Fprintf(&b, "- `%s` is too large to scan (over the read cap).\n", relPath)
		return b.String()
	}
	if !ok {
		fmt.Fprintf(&b, "- `%s` not found in the served dir (or outside it).\n", relPath)
		return b.String()
	}
	content := string(data)
	fmt.Fprintf(&b, "loaded from `%s` — `%s` is %d bytes\n\n", dir, relPath, len(data))
	for _, m := range strings.Split(markerCSV, ",") {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if strings.Contains(content, m) {
			fmt.Fprintf(&b, "- ✓ `%s` — FOUND (%d occurrence(s))\n", m, strings.Count(content, m))
		} else {
			fmt.Fprintf(&b, "- ✗ `%s` — not found (stale/wrong build, or marker typo)\n", m)
		}
	}
	return b.String()
}

// readClampedFile reads rel under dir with the SAME traversal clamp + size cap as
// fingerprintFiles (health.go), so a hostile relative path can't escape the system
// dir. Returns (content, ok, tooLarge).
func readClampedFile(dir, rel string) (content []byte, ok bool, tooLarge bool) {
	full, in := clampedPath(dir, rel)
	if !in {
		return nil, false, false
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return nil, false, false
	}
	if info.Size() > maxFingerprintBytes {
		return nil, false, true
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, false, false
	}
	return data, true, false
}

// clampedPath joins rel under dir and confirms the result stays within dir (the
// same traversal guard fingerprintFiles uses). Returns ("", false) if it escapes.
func clampedPath(dir, rel string) (string, bool) {
	if dir == "" || rel == "" {
		return "", false
	}
	cleanDir := filepath.Clean(dir)
	full := filepath.Clean(filepath.Join(cleanDir, rel))
	if full == cleanDir || strings.HasPrefix(full, cleanDir+string(os.PathSeparator)) {
		return full, true
	}
	return "", false
}

// ── entity.fields + entity.field-coverage: stored data + schema coverage ─────

// EntityFieldDump is one entity's stored field map for the entity.fields diagnostic.
type EntityFieldDump struct {
	Found    bool
	ID       string
	Name     string
	TypeName string
	Fields   map[string]any
}

// FieldCoverageRow is one declared field's population count across a type's entities.
type FieldCoverageRow struct {
	Key      string
	Label    string
	NonEmpty int
}

// FieldCoverage is the result of entity.field-coverage for one type.
type FieldCoverage struct {
	Found       bool
	TypeName    string
	EntityCount int
	Declared    []FieldCoverageRow
}

// EntityTypeInfo is one entity type for the entity.types discovery diagnostic.
type EntityTypeInfo struct {
	ID             int
	Name           string
	Slug           string
	PresetCategory string
	Count          int
}

// EntityHit is one search result for the entity.find discovery diagnostic.
type EntityHit struct {
	ID       string
	Name     string
	Slug     string
	TypeName string
}

// EntityDiagProvider is the injected read-only window into campaign entity data.
// Implemented by the app layer against the entities service (dependency inversion).
type EntityDiagProvider interface {
	EntityFields(ctx context.Context, campaignID, idOrSlug string) (EntityFieldDump, error)
	TypeFieldCoverage(ctx context.Context, campaignID, typeIDOrName string) (FieldCoverage, error)
	EntityTypes(ctx context.Context, campaignID string) ([]EntityTypeInfo, error)
	FindEntities(ctx context.Context, campaignID, query string) ([]EntityHit, error)
}

// SyncMappingInfo is one Foundry↔Chronicle sync link for the entity.sync-mappings
// diagnostic.
type SyncMappingInfo struct {
	ExternalSystem string
	ExternalID     string
	ChronicleType  string
	ChronicleID    string
	LastSync       string
}

// syncMappingFn returns the sync mappings for one entity (resolved id), or nil if
// the provider isn't wired. Injected from the app layer (syncapi + entity resolve).
var syncMappingFn func(ctx context.Context, campaignID, entityID string) ([]SyncMappingInfo, error)

// SetSyncMappingProvider wires the sync-mapping reader for entity.sync-mappings.
func SetSyncMappingProvider(fn func(ctx context.Context, campaignID, entityID string) ([]SyncMappingInfo, error)) {
	syncMappingFn = fn
}

var entityDiagProvider EntityDiagProvider

// SetEntityDiagProvider wires the entities read window for the entity.* diagnostics.
func SetEntityDiagProvider(p EntityDiagProvider) { entityDiagProvider = p }

// renderEntityFields dumps one entity's stored field values. Arg: "<campaignId>:<idOrSlug>".
func renderEntityFields(arg string) string {
	var b strings.Builder
	b.WriteString("## entity.fields\n\n")
	campaignID, ref, ok := splitArg2(arg)
	if !ok {
		b.WriteString("_Usage: `<campaignId>:<entityIdOrSlug>`_\n")
		return b.String()
	}
	if entityDiagProvider == nil {
		b.WriteString("_Entity provider not wired (entities plugin not injected at startup)._\n")
		return b.String()
	}
	dump, err := entityDiagProvider.EntityFields(context.Background(), campaignID, ref)
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if !dump.Found {
		fmt.Fprintf(&b, "_No entity `%s` in campaign `%s`._ Run `campaigns.list` to confirm the campaign id, then `entity.types <id>` to find the hero.\n", ref, campaignID)
		return b.String()
	}
	fmt.Fprintf(&b, "**%s** (`%s`, type: %s)\n\n", dump.Name, dump.ID, fallback(dump.TypeName, "?"))
	if len(dump.Fields) == 0 {
		b.WriteString("_No stored field values (fields_data is empty) — this is the 'renders blank' signature._\n")
		return b.String()
	}
	keys := make([]string, 0, len(dump.Fields))
	for k := range dump.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(&b, "%d stored field(s):\n", len(keys))
	for _, k := range keys {
		fmt.Fprintf(&b, "- `%s`: %s\n", k, previewValue(dump.Fields[k], 160))
	}
	return b.String()
}

// renderFieldCoverage shows how many of a type's declared fields are actually
// populated across its entities. Arg: "<campaignId>:<typeIdOrName>".
func renderFieldCoverage(arg string) string {
	var b strings.Builder
	b.WriteString("## entity.field-coverage\n\n")
	campaignID, ref, ok := splitArg2(arg)
	if !ok {
		b.WriteString("_Usage: `<campaignId>:<entityTypeIdOrName>`_\n")
		return b.String()
	}
	if entityDiagProvider == nil {
		b.WriteString("_Entity provider not wired (entities plugin not injected at startup)._\n")
		return b.String()
	}
	cov, err := entityDiagProvider.TypeFieldCoverage(context.Background(), campaignID, ref)
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if !cov.Found {
		fmt.Fprintf(&b, "_No entity type `%s` in campaign `%s`._\n", ref, campaignID)
		return b.String()
	}
	fmt.Fprintf(&b, "type **%s** — %d entit(y/ies), %d declared field(s):\n\n", cov.TypeName, cov.EntityCount, len(cov.Declared))
	if cov.EntityCount == 0 {
		b.WriteString("_No entities of this type._\n")
		return b.String()
	}
	// Sort: emptiest first (those are the suspects).
	rows := append([]FieldCoverageRow(nil), cov.Declared...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].NonEmpty < rows[j].NonEmpty })
	for _, r := range rows {
		pct := r.NonEmpty * 100 / cov.EntityCount
		mark := "✓"
		if r.NonEmpty == 0 {
			mark = "✗"
		}
		fmt.Fprintf(&b, "- %s `%s` — %d/%d (%d%%)\n", mark, fallback(r.Label, r.Key), r.NonEmpty, cov.EntityCount, pct)
	}
	return b.String()
}

// renderEntityTypes lists a campaign's entity types (id, slug, preset, count) so
// the operator can discover the right type ref for entity.field-coverage without
// knowing IDs. Arg: "<campaignId>".
func renderEntityTypes(arg string) string {
	var b strings.Builder
	b.WriteString("## entity.types\n\n")
	campaignID := strings.TrimSpace(arg)
	if campaignID == "" {
		b.WriteString("_Usage: `<campaignId>` (run `campaigns.list` first to find it)._\n")
		return b.String()
	}
	if entityDiagProvider == nil {
		b.WriteString("_Entity provider not wired (entities plugin not injected at startup)._\n")
		return b.String()
	}
	types, err := entityDiagProvider.EntityTypes(context.Background(), campaignID)
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if len(types) == 0 {
		fmt.Fprintf(&b, "_No entity types in campaign `%s` (check the id with `campaigns.list`)._\n", campaignID)
		return b.String()
	}
	for _, t := range types {
		preset := ""
		if t.PresetCategory != "" {
			preset = " [preset:" + t.PresetCategory + "]"
		}
		fmt.Fprintf(&b, "- `%d` **%s** (%s)%s — %d entit(y/ies)\n", t.ID, t.Name, t.Slug, preset, t.Count)
	}
	return b.String()
}

// renderEntityFind searches a campaign's entities by name/slug so the operator can
// discover an entity id without the URL. Arg: "<campaignId>:<nameQuery>".
func renderEntityFind(arg string) string {
	var b strings.Builder
	b.WriteString("## entity.find\n\n")
	campaignID, query, ok := splitArg2(arg)
	if !ok {
		b.WriteString("_Usage: `<campaignId>:<nameQuery>`_\n")
		return b.String()
	}
	if entityDiagProvider == nil {
		b.WriteString("_Entity provider not wired._\n")
		return b.String()
	}
	hits, err := entityDiagProvider.FindEntities(context.Background(), campaignID, query)
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if len(hits) == 0 {
		fmt.Fprintf(&b, "_No entities matching `%s` in campaign `%s`._\n", query, campaignID)
		return b.String()
	}
	for _, h := range hits {
		fmt.Fprintf(&b, "- `%s` — **%s** (%s) [%s]\n", h.ID, h.Name, h.Slug, fallback(h.TypeName, "?"))
	}
	return b.String()
}

// renderEntitySyncMappings shows whether an entity is linked to an external actor.
// Arg: "<campaignId>:<entityIdOrSlug>". No mappings = nothing will ever sync to it.
func renderEntitySyncMappings(arg string) string {
	var b strings.Builder
	b.WriteString("## entity.sync-mappings\n\n")
	campaignID, ref, ok := splitArg2(arg)
	if !ok {
		b.WriteString("_Usage: `<campaignId>:<entityIdOrSlug>`_\n")
		return b.String()
	}
	if syncMappingFn == nil {
		b.WriteString("_Sync-mapping provider not wired._\n")
		return b.String()
	}
	entityID, label := resolveEntityID(campaignID, ref)
	if entityID == "" {
		fmt.Fprintf(&b, "_Couldn't resolve entity `%s` in campaign `%s` (try `entity.find`)._\n", ref, campaignID)
		return b.String()
	}
	maps, err := syncMappingFn(context.Background(), campaignID, entityID)
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if len(maps) == 0 {
		fmt.Fprintf(&b, "%s — **no sync mappings**: not linked to any external actor, so nothing will sync to it.\n", label)
		return b.String()
	}
	fmt.Fprintf(&b, "%s — %d mapping(s):\n", label, len(maps))
	for _, m := range maps {
		fmt.Fprintf(&b, "- %s `%s` ↔ %s `%s` — last sync %s\n", fallback(m.ExternalSystem, "?"), m.ExternalID, fallback(m.ChronicleType, "entity"), m.ChronicleID, fallback(m.LastSync, "never"))
	}
	return b.String()
}

// resolveEntityID resolves an id-or-slug to a chronicle entity id via the entity
// provider. Returns ("","") if unresolved. label is a human string for output.
func resolveEntityID(campaignID, ref string) (id, label string) {
	if entityDiagProvider == nil {
		return ref, "entity `" + ref + "`" // no resolver: assume ref is already an id
	}
	dump, err := entityDiagProvider.EntityFields(context.Background(), campaignID, ref)
	if err != nil || !dump.Found {
		return "", ""
	}
	return dump.ID, fmt.Sprintf("**%s** (`%s`)", dump.Name, dump.ID)
}

// ── campaigns.list: discover campaign ids ───────────────────────────────────

// CampaignInfo is one campaign for the campaigns.list discovery diagnostic.
type CampaignInfo struct {
	ID   string
	Name string
	Slug string
}

var campaignListFn func(ctx context.Context) ([]CampaignInfo, error)

// SetCampaignListProvider wires the campaigns service for the campaigns.list
// diagnostic (so the operator can resolve a campaign id by name).
func SetCampaignListProvider(fn func(ctx context.Context) ([]CampaignInfo, error)) {
	campaignListFn = fn
}

// renderCampaignList lists all campaigns (id, name, slug) — the entry point for
// the entity.* diagnostics, which need a campaign id.
func renderCampaignList(string) string {
	var b strings.Builder
	b.WriteString("## campaigns.list\n\n")
	if campaignListFn == nil {
		b.WriteString("_Campaigns provider not wired._\n")
		return b.String()
	}
	cs, err := campaignListFn(context.Background())
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if len(cs) == 0 {
		b.WriteString("_No campaigns._\n")
		return b.String()
	}
	for _, c := range cs {
		fmt.Fprintf(&b, "- `%s` — **%s** (%s)\n", c.ID, c.Name, c.Slug)
	}
	return b.String()
}

// ── sync.inbound + sync.recent: what Foundry SENT ───────────────────────────

// InboundSyncRecord is one captured inbound sync payload (what an external client
// like the Foundry module sent), held in an in-memory ring buffer by syncapi.
type InboundSyncRecord struct {
	EntityID string
	At       time.Time
	Source   string         // e.g. "fields" (UpdateEntityFields) / "entity"
	Fields   map[string]any // the field map received
}

// syncInboundFn returns recent inbound records, newest first; entityID=="" means
// across all entities. Injected from syncapi (dependency inversion).
var syncInboundFn func(entityID string, limit int) []InboundSyncRecord

// SetSyncInboundProvider wires the syncapi inbound-payload buffer for sync.inbound.
func SetSyncInboundProvider(fn func(entityID string, limit int) []InboundSyncRecord) {
	syncInboundFn = fn
}

// renderSyncInbound shows the recent inbound payloads for one entity.
// Arg: "<campaignId>:<entityIdOrSlug>" (the slug is resolved to an id).
func renderSyncInbound(arg string) string {
	var b strings.Builder
	b.WriteString("## sync.inbound\n\n")
	campaignID, ref, ok := splitArg2(arg)
	if !ok {
		b.WriteString("_Usage: `<campaignId>:<entityIdOrSlug>` (or `sync.recent` for the last few across all entities)._\n")
		return b.String()
	}
	if syncInboundFn == nil {
		b.WriteString("_Sync provider not wired (syncapi not injected at startup)._\n")
		return b.String()
	}
	entityID, label := resolveEntityID(campaignID, ref)
	if entityID == "" {
		fmt.Fprintf(&b, "_Couldn't resolve entity `%s` in campaign `%s` (try `entity.find`)._\n", ref, campaignID)
		return b.String()
	}
	return renderInboundRecords(&b, syncInboundFn(entityID, 10), label)
}

// renderSyncRecent shows the last few inbound payloads across all entities.
func renderSyncRecent() string {
	var b strings.Builder
	b.WriteString("## sync.recent\n\n")
	if syncInboundFn == nil {
		b.WriteString("_Sync provider not wired (syncapi not injected at startup)._\n")
		return b.String()
	}
	return renderInboundRecords(&b, syncInboundFn("", 15), "all entities")
}

func renderInboundRecords(b *strings.Builder, recs []InboundSyncRecord, scope string) string {
	if len(recs) == 0 {
		fmt.Fprintf(b, "_No inbound sync payloads captured for %s yet (none received since boot, or the buffer rolled over)._\n", scope)
		return b.String()
	}
	fmt.Fprintf(b, "%d recent inbound payload(s) for %s, newest first:\n\n", len(recs), scope)
	for _, r := range recs {
		keys := make([]string, 0, len(r.Fields))
		for k := range r.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(b, "### %s — %s (%d field(s))\n", r.At.UTC().Format(time.RFC3339), fallback(r.Source, "fields"), len(keys))
		if r.EntityID != "" {
			fmt.Fprintf(b, "entity `%s`\n", r.EntityID)
		}
		for _, k := range keys {
			fmt.Fprintf(b, "- `%s`: %s\n", k, previewValue(r.Fields[k], 120))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ── campaign-slot substitution (for the review-step campaign picker) ─────────
//
// Some diagnostics take a campaign id as the first colon-part of their arg (or as
// the whole arg). When the AI leaves that as a placeholder, the workspace review
// step offers a campaign dropdown; the operator's pick is substituted here before
// the batch runs. Keeping the arg shapes in systems (where the diagnostics live)
// means the admin UI doesn't have to know them.

// campaignSlot returns the campaign portion of a campaign-scoped diagnostic's arg.
// scoped=false for diagnostics that take no campaign. whole=true when the entire
// arg IS the campaign (entity.types); otherwise the campaign is parts[0] and rest
// is everything after the first colon.
func campaignSlot(name, arg string) (slot, rest string, whole, scoped bool) {
	switch name {
	case "entity.types":
		return strings.TrimSpace(arg), "", true, true
	case "entity.fields", "entity.field-coverage", "entity.find", "sync.inbound", "entity.sync-mappings":
		parts := strings.SplitN(arg, ":", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[0]), parts[1], false, true
		}
		return strings.TrimSpace(arg), "", false, true // malformed: treat whole as the slot
	}
	return "", "", false, false
}

// entitySlot returns the entity portion (parts[1]) of an entity-targeted
// diagnostic's arg. scoped=false for diagnostics that take no entity ref (the
// 2nd part of entity.find is a query and entity.field-coverage is a type, so
// neither is substitutable here).
func entitySlot(name, arg string) (slot string, scoped bool) {
	switch name {
	case "entity.fields", "sync.inbound", "entity.sync-mappings":
		parts := strings.SplitN(arg, ":", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1]), true
		}
		return "", true
	}
	return "", false
}

// EntitySlotIsAmbiguous reports whether an entity-targeted call left its entity
// ref as a placeholder or empty — the review step should offer an entity field.
func EntitySlotIsAmbiguous(name, arg string) bool {
	slot, scoped := entitySlot(name, arg)
	if !scoped {
		return false
	}
	return slot == "" || (strings.HasPrefix(slot, "<") && strings.HasSuffix(slot, ">"))
}

// WithEntity returns arg with its entity slot replaced by entityRef, preserving
// the campaign part.
func WithEntity(name, arg, entityRef string) string {
	if _, scoped := entitySlot(name, arg); !scoped {
		return arg
	}
	camp := ""
	if parts := strings.SplitN(arg, ":", 2); len(parts) >= 1 {
		camp = parts[0]
	}
	return camp + ":" + entityRef
}

// ApplyEntityPick substitutes the operator-supplied entity ref into every call
// whose entity slot was left ambiguous.
func ApplyEntityPick(plan *BatchPlan, entityRef string) {
	if plan == nil || strings.TrimSpace(entityRef) == "" {
		return
	}
	for i := range plan.Calls {
		if EntitySlotIsAmbiguous(plan.Calls[i].Name, plan.Calls[i].Arg) {
			plan.Calls[i].Arg = WithEntity(plan.Calls[i].Name, plan.Calls[i].Arg, entityRef)
		}
	}
}

// PlanNeedsEntity reports whether any call left its entity slot ambiguous.
func PlanNeedsEntity(plan *BatchPlan) bool {
	if plan == nil {
		return false
	}
	for _, c := range plan.Calls {
		if EntitySlotIsAmbiguous(c.Name, c.Arg) {
			return true
		}
	}
	return false
}

// CampaignSlotIsAmbiguous reports whether a call is campaign-scoped but left its
// campaign as a placeholder (`<...>`) or empty — i.e. the review step should ask
// the operator to pick one.
func CampaignSlotIsAmbiguous(name, arg string) bool {
	slot, _, _, scoped := campaignSlot(name, arg)
	if !scoped {
		return false
	}
	return slot == "" || (strings.HasPrefix(slot, "<") && strings.HasSuffix(slot, ">"))
}

// WithCampaign returns arg with its campaign slot replaced by campaignID (no-op
// for non-campaign-scoped diagnostics).
func WithCampaign(name, arg, campaignID string) string {
	_, rest, whole, scoped := campaignSlot(name, arg)
	if !scoped {
		return arg
	}
	if whole {
		return campaignID
	}
	return campaignID + ":" + rest
}

// ApplyCampaignPick substitutes the operator-chosen campaign into every call whose
// campaign slot was left ambiguous. Called by the run handler when the review
// dropdown was used. The id comes from the server-rendered campaign list and all
// calls are read-only + campaign-scoped, so substitution is safe.
func ApplyCampaignPick(plan *BatchPlan, campaignID string) {
	if plan == nil || strings.TrimSpace(campaignID) == "" {
		return
	}
	for i := range plan.Calls {
		if CampaignSlotIsAmbiguous(plan.Calls[i].Name, plan.Calls[i].Arg) {
			plan.Calls[i].Arg = WithCampaign(plan.Calls[i].Name, plan.Calls[i].Arg, campaignID)
		}
	}
}

// PlanNeedsCampaign reports whether any call in the plan left its campaign slot
// ambiguous (so the review step should show the campaign picker).
func PlanNeedsCampaign(plan *BatchPlan) bool {
	if plan == nil {
		return false
	}
	for _, c := range plan.Calls {
		if CampaignSlotIsAmbiguous(c.Name, c.Arg) {
			return true
		}
	}
	return false
}

// ── small shared helpers ────────────────────────────────────────────────────

// splitArg2 parses "<a>:<b>" into trimmed non-empty parts.
func splitArg2(arg string) (a, bb string, ok bool) {
	parts := strings.SplitN(arg, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	a, bb = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	return a, bb, a != "" && bb != ""
}

// previewValue renders a field value compactly, capped at max runes, so a giant
// JSON blob (abilities_json) doesn't flood the result. redactSecrets still runs
// over the whole document afterward.
func previewValue(v any, max int) string {
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return "_(empty)_"
	}
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

func fallback(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
