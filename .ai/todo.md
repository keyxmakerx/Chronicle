# Chronicle Backlog

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Single source of truth for what needs to be done, priorities,    -->
<!--          and what has been completed.                                     -->
<!-- Update: At the start of a session (to understand priorities), during      -->
<!--         work (to mark progress), and at session end (to reflect).        -->
<!-- Legend: [ ] Not started  [~] In progress  [x] Complete  [!] Blocked      -->
<!-- ====================================================================== -->

## 1. Bugfixes & Problems

Known broken or missing things, ordered by severity.

### Critical

- [x] **Login "invalid CSRF token" (C-AUTH-LOGIN-CSRF-FIX)** — Root cause: the CSRF cookie name is scheme-dependent (`__Host-chronicle_csrf` over HTTPS, bare over HTTP); behind a TLS-terminating proxy the derived scheme could differ between the form GET (cookie set) and the POST (validate), so `req.Cookie(name)` missed the cookie and compared the form token against a freshly-generated value → guaranteed 403. Fix: `readExistingCSRF` reads the cookie under BOTH names (resilient to scheme flips) + `schemeIsSecure` hardened to parse comma-list `X-Forwarded-Proto`. Part B: friendly no-jargon 403 (`CSRFFriendlyMessage`) and login auto-recovery — a stale/missing-token login POST bounces to `GET /login?expired=1` (HX-Redirect for HTMX, 303 otherwise), which re-issues a valid token + shows a friendly banner. Regression tests in `internal/middleware/csrf_test.go` (set→submit, both scheme-flip directions, recovery, friendly-403, API skip). ⚠️ Operator confirms a real proxied login post-merge (CI can't repro the proxy condition).

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### In flight — 2026-06-11 sweep round (agents dispatched; coordinator-verified findings)

- [x] **Document-listener leaks** in `entity_posts.js` + `relation_graph.js` (cordinator#39 F1/F2) — Agent 1, `C-SWEEP-FIXES-R1` PR 1. Shipped (PR #462).
- [x] **Public-campaign read gaps**: aliases route not in pub group; player-notes block mounts for anonymous; map blocks blank for public viewers (cordinator#39 F3/F5/F4) — Agent 1, `C-SWEEP-FIXES-R1` PR 2. Fog/layers stay auth-only. Shipped (PR #462).
- [x] **Topbar custom branding still masked** (cordinator#29) — header lacks a stacking context, so the z-index:-1 brand layer paints under `bg-surface`; fix = `isolate` on the header — Agent 2, `C-BACKLOG-BUGS-R1`. Shipped (PR #464).

### Dynamic surface + Characters page — 2026-06-22

- [x] **Dynamic-surface frame (Wave 1)** `Chronicle.surface` + admin surface demo at `/admin/design-lab` (Design Lab repurposed from the static showcase). See status.md.
- [x] **Characters ("Cast") page** `GET /campaigns/:id/characters` — party (claimed PCs) + NPCs; mini→full launch = the frame's first production adopter. `service.ListClaimed` + pure `assembleCastParty` (tested) + `characters.js` (plugin embed.FS) + sidebar link. Later consolidated + addon-gated (see below). Cordinator `plans/2026-06-22-characters-cast-page-design.md`.
- [x] **Player characters default to the big dynamic-surface widget** — `character_surface` layout block (registered, `Contexts:["template"]`, editable in the layout editor); `CharacterLayout()` is the default for PC sub-types; generic box renderers in `character_surface.js`; description box reuses the role-aware `editor` widget. Frame fix: `Provider.push` no longer clobbered by `load()`. See `entities/.ai.md` §`character_surface`.
- [x] **Consolidated addon-gated Characters page** — Players section (player-character-claiming) + NPC section (npcs, via injected `NPCSectionProvider`: feature-tag portrait row + full revealed list + DM reveal/hide); page 404s if neither addon. Standalone `/npcs` gallery now redirects to `/characters`. Cleanup follow-ups: remove dead `npcs/gallery.templ`; de-dupe the legacy "NPCs" sidebar link (now redundant with "Characters").
- [x] **Auto-premake the "Player Characters" sub-type on addon enable** — `addons.EnableForCampaign` now calls `PresetApplier.ApplyAddonEnableEffects`, which for `player-character-claiming` runs `entityService.EnsurePlayerCharacterType` (idempotent: skips if a PC type exists, else creates it with `CharacterLayout()` + claimable). Tested.
- [x] **Backfill the PC type for already-enabled campaigns** — one-time idempotent startup sweep (`internal/app/backfill.go`, wired after the preset applier in `routes.go`) replays the enable-effect for campaigns that turned the claiming addon on before it shipped. New `addons.ListCampaignsUsingAddon` (repo+service); runs through the service (no SQL), self-healing every boot. Table-driven test. Migration-safe (no schema change).
- [x] **Glossary route for client reference-renderers** — `GET /campaigns/:id/systems/:mod/rules-glossary` (`SystemHandler.RulesGlossaryAPI`) serves the system's raw `data/rules-glossary.json` (preserves authored `slug`), so `{@category term}` tokens resolve. Unblocks the Draw Steel sheet's refs.
- [x] **Draw Steel `character-sheet` → `Chronicle.surface` adopter** (Chronicle-Draw-Steel) — 11 `ds-*` box renderers (inner content only), schema omits empty boxes, headless identity banner, ability power-roll overlays via one delegated listener; mount contract unchanged; read-only. *Remaining:* operator browser-verify on a Hero entity.
- [x] **Premake no longer duplicates a system character type** — `EnsurePlayerCharacterType` skip-check broadened to `isClaimableType(&t) || isPlayerCharacterType(...)` (`service.go:~927`), so enabling claiming in a Draw Steel campaign (which already has `drawsteel-character`) skips the premake instead of creating a redundant generic "Player Characters" type (was surfacing as a duplicate category + 2nd editor sheet). Regression test added. **Superseded by the PC sub-category migration below** — the stray is re-parented in place, not deleted.
- [x] **PC sub-category migration + forward nesting (PROD)** — reworked `EnsurePlayerCharacterType` (`service.go:~920`) so the claimable "Player Character" type is **nested under the default "Characters" category** (`ParentTypeID` → the seeded `character` type) — the shape ADR-039 / cordinator `2026-06-19-pc-claim-design §3` always intended. Three idempotent cases: **migrate** a stray top-level PC type an earlier build premade by re-parenting it via `UpdateEntityType` (changes only the parent pointer → claimed characters + layout preserved; `Update` never writes `layout`); **skip** when a system character type (`drawsteel-character`) already serves as claimable; else **create** nested (top-level fallback when no `character` category). The existing startup backfill (`internal/app/backfill.go`) replays it, so **deploying heals prod in place** — no manual delete, no SQL. `ensure_pc_type_test.go`: migration + nested-create cases added (5 pass). A re-parented sub-type leaves the sidebar top level (reached via the "Characters" parent — the Lore→Timelines pattern).
- [x] **Characters nav link → top static nav** — moved from the categories drill-down zone up beside Dashboard/Calendar in `app.templ`, addon gate kept.
- [x] **Rulebook nav link for the enabled game system** — top static nav link (beside Characters) to the system's reference index (`/campaigns/{id}/systems/{slug}`), labeled with the system's name. Injector identifies the enabled system from the campaign-addons it already lists (category `system`) → `layouts.SetEnabledSystem`; new `sidebarSystemReferenceLink`. Generic across systems; shown only when one is enabled.
- [x] **PC-sheet system-binding — 4 seams (modularity; zero core changes for a new system)** — a system pack fills Chronicle's player-character category. Design: Cordinator `plans/2026-06-23-pc-sheet-system-binding-design.md` (§11 = the shipped minimal-touch shape). **Seam 1 (renderer-by-preset):** `RendererDef.PresetCategory` (`internal/systems/manifest.go`) — a renderer binds by entity-type slug **XOR** `preset_category` (validated against the manifest's own `entity_presets[].category`); registry gains `presetRenderers` (`show_renderer_registry.go`); `lookupEntityShowRenderer` resolves **slug-first-then-preset** (no shadowing); `registerManifestRenderers` (`routes.go`) routes to the right map. **Seam 2 (nest the system char type):** `EnsurePlayerCharacterType` Case 2 re-parents a system's own claimable char type (e.g. `drawsteel-character`, detected via `isClaimableType`) under "Characters" — **no rename, no field copy** (renders via its own slug renderer); generic fallback unchanged. **Seam 3 (duplicate guard):** `CreateEntityType` rejects a manual 2nd PC type (one per preset-category) with a `Conflict` pointing at the addon. **Seam 4 (page-renderer ≠ palette block):** `GetSystemWidgetBlockMetas` (`internal/systems/handler.go`) excludes widgets that are also `renderers[].widget` — kills the "drop sheet into layout → bare name" trap (replaces the slug-string filter `9a1e4d6`). No DB change. Field-adoption/rename/`CreateEntityTypeInput.Fields` (design drafts) **dropped** per §11. Tests: `show_renderer_preset_test.go`, `ensure_pc_type_test.go`. Docs: `systems/.ai.md`, `entities/.ai.md`, `docs/system-package-rendering.md`. *Remaining:* Draw Steel `0.0.10` (optional `preset_category:"character"` renderer — not required, slug renderer already covers its type); operator prod-campaign stray-duplicate migration (separate from boot).
- [~] **Rulebook v2 — interactive rules-glossary surface** (Flagship #2) — **PARKED** (built, then removed from the build at the operator's request — needs a rework; code recoverable at commit `bbe6508`). New Chronicle-core widget `static/js/widgets/rulebook.js` (`data-widget="rulebook"`, see its `.ai.md`): fetches the system's `rules-glossary`, groups by `properties.category`, renders each rule as an expanding `Chronicle.surface` box beside a category nav, with debounced cross-category search and stackable `{@term}` cross-ref overlays (`deal` transition). A thin surface adopter (frame owns chrome/motion/overlays). Mounted above the category grid in `SystemIndexContent`; **degrades invisibly** when a system ships no glossary, so zero backend/route changes. Matches mockup `04_rulebook.png`. *Remaining:* operator browser-verify; optional follow-ups — `{@term}` hovercard (MINI, via the `/tooltip` endpoint) and extending the surface to the full category catalog (currently glossary-only; spell/monster catalogs keep the grid + their own browsers).
- [ ] **PC type as a SUB-category (system-less campaigns)** — deferred design: for campaigns with no game-system character type, should the premade "Player Characters" be a sub-category of the Characters page rather than a top-level type? Pairs with the P4c subcategory wizard / template inheritance. Moot when a system provides its own character type.
- [ ] **Cast page — Draw Steel surface in the launch overlay** (the DS sheet adopter shipped; now wire it in): replace the `/preview` body with the real dynamic character sheet.
- [ ] **Cast page — session/location-derived "active" NPCs** (Option C in the design): derive the Active band from where the party is, beyond the manual `cast` tag.

### Player Character Claiming (PC-CLAIM) — staged feature

Goal: restrict claiming to a "Player Characters" sub-type via an Owner-toggleable
addon; make claims visible (who claimed what); keep Foundry auto-claim working
for player-owned PC actors.

**Deferred follow-ups (from the 2026-06-24 duplicate consolidation):**
- [x] **Widen the duplicate guard → human-readable error (PC-DUP-GUARD-2)**: folded
  into the Extension Settings framework (2026-06-24, plan
  `/root/.claude/plans/drifting-jingling-boole.md`). The player-character
  SetupProvider detects the duplicate (generic + system character type) as a
  health check, and `MergeDuplicatePlayerCharacterType` returns a human-readable
  `apperror` when the (generic → system) pair is ambiguous. Owner reconciles on
  demand from the extension settings page instead of a silent migration.
- [x] **`ApplySystemPresets` drops `preset.Fields` (PC-PRESET-FIELDS)**: DONE in two
  parts. **Create path** (commit `cf3a7c6`): `CreateEntityTypeInput.Fields` +
  `mapPresetFields`/`mapPresetFieldType` (`preset_applier.go`) — a new system type is
  created WITH its declared schema. **Reconcile path / non-destructive backfill of
  EXISTING types** (WS-5): `entities.ReconcileEntityTypeFields(typeID, declared)` +
  the pure `mergeNewFields` helper — additively appends any declared field whose Key
  is absent, never removes/reorders/overwrites (idempotent). `ApplySystemPresets` now,
  instead of skip-if-exists, indexes existing types by preset-category then name and
  **upgrades the match in place** (else creates). So enabling/updating a system fills
  EXISTING heroes' type schema (Tyne/Orrin/Saatraaol) without recreating it. Type
  schema only — never entity data. Tests: `reconcile_fields_test.go`.

- [x] **Stage 1 — claim visibility (PC-CLAIM-1)**: distinct `entity.claimed` /
  `entity.owner_changed` audit actions (audit/model.go) + activity-feed labels &
  colors (audit/activity.templ); `ClaimEntity` records the real character name
  under `entity.claimed` (was generic `entity.updated` + "claimed by <id>"), and
  `AssignOwner` records the new owner in `Details` under `entity.owner_changed`
  (`logAuditWithDetails`). Compile + audit/entities unit tests green.
- [x] **Stage 2 — addon + claimable flag (PC-CLAIM-2)**: registered
  `player-character-claiming` in `builtinAddons` (CategoryPlugin/StatusActive,
  `fa-user-check`; startup seeder upserts it, no migration). Migration `000029`
  adds `entity_types.claimable BOOLEAN NULL AFTER parent_type_id` (NULL=unset →
  legacy heuristic; TRUE/FALSE=explicit Owner choice). `Claimable *bool` plumbed
  through EntityType + the create/update input & form DTOs. **Repo de-duplicated**:
  the former 6-way SELECT/scan copy-paste is now a single `entityTypeColumns`
  const + `scanEntityType(rowScanner)` helper that *every* read routes through
  (FindByID resolves `ParentTypeName` via a cheap follow-up lookup so it shares
  the one scan path) — kills the scan-order drift risk; `claimable` added to the
  Create INSERT + Update. `isClaimableType` now honours the flag (explicit wins;
  NULL → preset/slug heuristic). `CreateEntityType` gates player-character
  sub-types (`PresetCategory=="player_character"` or `slug=="player-character"`)
  on the addon via a service-injected `AddonChecker` (`SetAddonChecker`, wired in
  `app/routes.go`), rejecting with an "enable the Player Character Claiming addon"
  apperror when off and defaulting `claimable=true` when on. Verified: `templ
  generate`, `go build ./...`, `make test-unit` (43 pkgs green, incl. table-driven
  `isClaimableType` + create-gate tests), and `make test-int` against a real
  MariaDB (new `TestEntityTypeRepository_Integration` exercises all six reads +
  INSERT/UPDATE so the claimable scan is proven live); `make migrate-up`/`down`
  round-trip clean.
- [x] **Stage 3 — UI (PC-CLAIM-3)**: the player-facing + GM-facing surfaces for
  the addon + flag that Stage 2 plumbed. **(1) Owner on the character page** — the
  show page's claim block is extracted to `claim_banner.templ` (`claimBanner`):
  "Claimed by &lt;DisplayName&gt;" when `OwnerUserID != nil` (the Show handler
  resolves the id → `CampaignMember.DisplayName` via the existing `memberLister`;
  falls back to "a player" for a stale/unresolved owner), else the existing
  unclaimed banner. **(2) GM owner overview** — `category_owner_roster.templ`
  (`claimRosterPanel`) renders on a claimable category dashboard for **Scribe+**:
  every character with its owner + a reassign `<select>` (members) and an Unclaim
  button, both PUT `…/entities/:eid/owner` via `Chronicle.apiFetch`. The Index
  handler builds the `ClaimRoster` (full type list, capped 100, + members) only
  when Scribe+ AND claimable AND addon-on, and threads it through
  `CategoryDashboardContent` *inside* `#entity-list` so the search/sort swap still
  works. **(3) Per-type toggle** — `EntityTypeCard` (and the Add-Category form)
  gain a "Players can claim entities of this type" checkbox **only when the addon
  is enabled**; the quick-edit save rides `claimable` on the PUT (bound to the
  effective `isClaimableType`), the create form posts `claimable=true` when
  checked. `EntityTypesPage`/`CreateEntityType` compute `claimingEnabled`.
  **(4) Claim-button gating** — the unclaimed banner now requires addon-enabled
  **AND** `isClaimableType(type)`. Tests: `claim_overview_test.go` (helpers +
  component renders for all three surfaces & gating states). Verified `templ
  generate`, `go build ./...`, `make test-unit` (43 pkgs green), `go vet`.
- [x] **Stage 4 — Foundry (PC-CLAIM-4)**: actor-sync detects the addon, maps
  player-owned PC-type actors → the PC sub-type and auto-claims them (NPCs/monsters
  excluded by actor type + GM ownership); surface "enable Player Character Claiming
  in Chronicle" when the addon is off. — MERGED FM #64
- [x] **May bugs verify-then-fix** — editor dark-on-dark (#8, shipped PR #465), mobile notepad z-index (#11, shipped PR #466) — Agent 3, `C-BACKLOG-BUGS-R1`. Customizer no-change save + scroll (#10) status not independently reconfirmed this pass; the Customization Hub had a full rescue since (#524, C-CUSTOMIZE-RESCUE) so it is very likely also resolved — verify before relying on this if it resurfaces.

### High

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Medium

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Recently Fixed (2026-04-25)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Recently Fixed (2026-04-12)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Low (Original)

_See `.ai/audit.md` for the full feature parity & completeness audit. Audit items now organized into Phases M0-M3 and Backlog below._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

---

## 2. Features To Do

New capabilities ordered by priority for alpha release.

### Calendar Showcase: World-State Effects (C-CAL-WORLDSTATE-EFFECTS-SYSTEM)

Synced world-state animation system — ONE `worldState` drives BOTH the Almanac
sky-band AND the hourglass time-piece. Mock-data only, `/demo/calendar/almanac`.
Spec: `docs/design/world-state-effects/` (README + BUILD-PLAN + CATALOG + prototypes).

- [x] **Wave 2 — MUST effects** (CATALOG §12):
  - [x] **2a Weather + celestial bundle** (10): clear/cloudy/rain/thunderstorm/snow/fog/
    tornado/ashfall + meteor-shower/aurora — `EFFECTS` renderers on the shared frame
    hook, hgSand sync. **Shipped (PR #391).**
  - [x] **2b Moon library** (~28): vendored Noto/Twemoji lunar sets + 12 procedural
    SVGs; `MOON_DESIGNS` registry; emoji + css-clip phase paths; named-phase popover;
    demo design picker + Randomize + Add. **Shipped (PR #394).**
  - [x] **2c Mood-tint wash** (CATALOG Part 5) — global `overlay`-blend wash over both
    surfaces as resolution step 6 (sky-band div + hourglass canvas composite over
    sand); 8 presets + custom + intensity + clear; static (no rAF), reduced-motion-safe.
    **Shipped (PR #395)** — closed the Wave 2 MUST set.
- [x] **Wave 3 — Time-control verb layer** (CATALOG Part 6, D&D narrative-chunk model):
  +1hr / +1day / long-rest / custom (smooth ~600ms time tween) / set-time / step-back
  (single-undo + ~400ms reverse-sand) / atmosphere-pause; `timepieceFill` 0–0.33 caps →
  reuse the dawn/dusk flip + reset; verbs tween on the shared rAF (`engine.addTick`),
  reduced-motion → instant snaps. Mechanics in `window.__calTimeControl` (reusable by
  the future GM Live Control Panel). NOT VCR playback. **Shipped** (`window.__calTimeControl`
  live on `main`, wired into the worldstate layer order).
- [ ] **Wave 4 — SHOULD effects** · [ ] **Wave 5 — NICE/EXOTIC long tail** (on demand).

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Timeline Showcase: FM Tuner (C-TIMELINE-V2-DESIGN-1-TUNER)

Two candidate timeline designs were built: the Tuner (this section) and the Ledger
(shipped as the alternate, see below). Mock-data only, `/demo/timeline/tuner`,
page-separated (own CSS+JS). Raw SVG + CSS transforms, NO D3 (audit §7). Spec:
`cordinator/dispatches/chronicle/C-TIMELINE-V2-DESIGN-1-TUNER.md`.

- [x] **Ledger timeline (alternate design)** — shipped as `/demo/timeline/ledger` (chronicle#460,
  2026-06-11); `/demo/calendar` is the consolidated hub. Operator design pick decided 2026-07-03
  = **Ledger** — it drove Timeline V2 W1 (the calendar's 4th V2 view, PR #519/#520).

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Apps Hub → Calendars Dashboard (E1)

Overhaul/expand the Extensions hub into a hub that opens per-app management
dashboards; first = Calendars. Audit: `reports/chronicle/2026-06-07-apps-hub-cal-dash-prep-audit.md`.

- [~] **W3/W4 SUPERSEDED** by the widget-binding framework (below) — the Calendars dashboard becomes a *consumer* of the binding registry; W1/W2 stand.

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Widget Binding Framework (the real Wave-4)

Generic host (entity/entity-type/dashboard) ↔ widget-type ↔ data-instance binding;
`entity_calendar`/`entity_worldstate`/`map_editor` are hardcoded special cases.
Audit: `cordinator/reports/chronicle/2026-06-07-widget-binding-framework-prep-audit.md`. ADR-038.

- [x] **C-CAL-V2-WORLDSTATE-BAND-FINISHING Part D — DONE via the GM-overhaul arc** (chronicle#442/#443/#456: full-band strip+sheets console, edge-docked, state-machined). Original note: re-anchor `gmControlPanelV2` from `fixed bottom-4 right-4` to a **collapsible in-band overlay** (within/over the sky-band region, z-above, animated, reduced-motion-aware) to resolve the notes-button collision; needs a relative wrapper around the `overflow-hidden` band so the expanded panel isn't clipped + gm_panel.js coordination. `CanControlWorldState`-gated (server-side, unchanged). Its own follow-on PR.
- [x] **`cal-almanac.css` reorg — DONE via chronicle#442** (`cal-almanac-render.css` split + the `css_render_split.test.mjs` guard). Original note: the worldstate widget was built demo-first, so widget-intrinsic render rules were tangled with demo-only chrome under `.cal-almanac-shell`. After the band-finishing de-scope, formally separate **widget-intrinsic render** vs **demo-only chrome** sections so the next "works in demo, blank in prod" regression can't happen. Not urgent; logged for the hygiene arc.
- [x] **V1→V2 calendar cutover — DONE via chronicle#440** (all V1 views 301 to V2; #453 made the V2 views public-capable). Original note: retire/redirect the V1 `/calendars/:calId` month/week/day/timeline views + the `/calendars` Index redirect + the app-dashboard "Open" link to the V2 shell; remove the V1 `calendar.templ` view chrome once parity is confirmed. Its own dispatch.
- [ ] **P4c** `EntityType.hosts_widget_type` flag + the **"Calendars subcategory" create wizard** (entity-type-as-host preset — "an entity IS a calendar"; pick-or-create on entity create) + surface the P1 entity-type **template** inheritance rung. *(operator's headline vision piece — its own wave.)*
- [ ] **P3b** dashboard-as-host (unify `DashboardBlockSwitch` → `BlockRegistry.Render`, lights up `host_type='dashboard'`) · **`entity.map_id` backfill→bindings + column drop** + retire the dormant `AssignMap` endpoint / `entity_map.js` change-pick handlers (now more relevant since maps writes bindings — pair it with maps cleanup).

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Worldstate Widgetization (C-CAL-WORLDSTATE-WIDGETS) — Phase 6

Graduates the showcase worldState renderers into a reusable production widget +
an entity-page block, completing "all three views entity-able". Spec:
`cordinator/dispatches/chronicle/C-CAL-WORLDSTATE-WIDGETS.md`.

- [ ] **Wave 4 — per-entity configurable attachment** (owner picks which calendar/date a
  given entity's widget binds to + config UI + persistence) — OUT of scope, post-deadline
  widget framework (same boundary the Tuner §Q draws).

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Alpha-Critical (Must Have)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Alpha-Nice-to-Have

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase K: Permissions & Competitive Gap Closers

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase L: Content Depth & Editor Power

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase M0: Data Integrity & Export Completeness ← START HERE

_Fix export/import so backups don't lose data. Highest-priority work._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase M1: Quick Wins Sprint

_High-impact, low-effort items that immediately improve the user experience._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase M2: JS Code Quality

_Consistency and reliability across all JS widgets._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase M3: Test Coverage

_Fill the biggest test gaps — zero-test plugins and incomplete service tests._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase S: Data Integrity & Admin Tooling (COMPLETE)

_Fix orphaned data, cascade gaps, and admin DB visibility. See `.ai/phases.md`._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase T: Game System Modules & Worldbuilding Tools

- [ ] **Sprint T-4b: Entity Type Template Library** — Genre presets (fantasy, sci-fi, horror, modern, historical) as JSON fixtures. Campaign creation genre selection. "Import preset" in Customization Hub.

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase U: Collaboration & Platform Maturity

- [ ] **Sprint U-3: 2FA/TOTP Support** — TOTP enrollment with QR code (`pquerna/otp`). Login redirect to TOTP input. Recovery codes (8 hashed). Admin force-disable.
- [ ] **Sprint U-4: Accessibility Audit (WCAG 2.1 AA)** — ARIA labels, focus traps, skip-to-content, color contrast 4.5:1, keyboard nav, screen reader announcements, axe-core scanning.
- [ ] **Sprint U-5: Infrastructure & Deployment** — Docker-compose full stack verification with health checks. Makefile full-stack target. `CONTRIBUTING.md`. CI against docker-compose.

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase V: Obsidian-Style Notes & Discovery (COMPLETE except V-4)

_Quick capture, backlinks, enhanced graph, editor power-ups. See `.ai/obsidian-notes-plan.md` and `.ai/competitive-gap-analysis.md`._

- [~] **Sprint V-4: Enhanced Graph View & Cover Images** — @mention links in graph ✅, entity type filtering ✅, tag filtering (deferred — needs service plumbing), local graph (N hops) ✅, clustering ✅, orphan detection ✅, PNG export ✅. Cover/banner image layout block type ✅ (migration 000004, API, block registry, upload UI). Remaining: tag-based filtering on graph (requires TagEntityLister adapter).

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase W: Polish, Ecosystem & Delight

- [ ] **Sprint W-2: Map Drawing Tools, Regions & Measurement** — Leaflet.Draw integration (freehand, polygons, circles, rectangles, text). Uses existing `map_drawings` table. Per-drawing visibility, color/opacity. Also: map regions (polygon fills/strokes/labels), measurement/distance tool, map embed layout block for entity pages.
- [ ] **Sprint W-2.5: Nested / Linked Maps** — Click marker to open sub-map. `linked_map_id` on markers. Breadcrumb navigation between map levels. Competitive gap vs World Anvil/LegendKeeper.
- [ ] **Sprint W-3: Discord Bot Integration** — Plugin at `internal/plugins/discord/`. Bot token config. Webhook session notifications. Reaction-based RSVP per ADR-012.
- [~] **Sprint W-4: Bulk Operations & Persistent Filters** — Multi-select in sidebar reorg mode done (Ctrl+click, floating action bar, bulk-move API). Remaining: multi-select on entity list page, batch tag, batch visibility, batch delete. Persistent filters per category in localStorage. Entity tag/field filtering on list pages.
- [ ] **Sprint W-5: Editor Import/Export & Additional Themes** — Markdown import/export via `goldmark`. Sepia + high-contrast themes. Custom accent color picker. Embed media blocks (video/audio URLs) in editor.
- [ ] **Sprint W-6: Timeline List View & Meter Blocks** — Simple chronological list view alongside D3 viz. Meter/tracker layout block type for numeric values (HP, spell slots) with bar/circle/dot styles.

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase N: Sidebar Navigation Overhaul (COMPLETE — ADR-032)

_Comprehensive sidebar navigation rework. Replaces folder-entity hack, adds
favorites, unified sidebar model, and large campaign support._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Backlog: Integrations Tab Redesign (COMPLETE)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Backlog: Remaining Audit Items (address opportunistically)

_Lower-priority items to pick up during related sprints or as standalone tasks._

**UI Consistency:**
- [ ] **Alert styling inconsistent** — login.templ and entities/form.templ use inline Tailwind instead of alert-success/alert-error classes.
- [ ] **Admin pagination inline** — admin/users.templ and admin/campaigns.templ have hand-rolled pagination instead of using components.Pagination.
- [ ] **Modal approach mixed** — Sessions uses dialog element; calendar/other modals use Alpine.js. Should standardize.
- [ ] **Rate limiting on mutations** — Campaign/entity/widget mutation endpoints have no rate limiting (auth + media do).
- [x] **Recurring calendar events (beyond yearly)** — shipped (chronicle#461, 2026-06-11): weekly/biweekly/monthly/custom share the sessions vocabulary; single `Event.OccursOn` expansion predicate; leap-aware monthly; migration 011. Multi-day-span recurrence + recurrence end-date UI controls remain future polish.

**Documentation:**
- [ ] **`calendar_v2` widget missing .ai.md** — the only Go widget without a documentation file (posts/.ai.md already exists).
- [ ] **12 JS widgets missing .ai.md** — calendar_widget, map_widget, relation_graph, entity_type_config, entity_type_editor, groups, permissions, shop_inventory, timeline_widget, entity_posts, notifications, shortcuts_help. (sidebar_tree, sidebar_reorg, sidebar_tag_filter, sidebar_layout_editor now have .ai.md files.)

**Player & DM Experience Gaps:**
- [ ] **Entity tag/field filtering** — Entity list only has type tabs. No filter by tag, custom field value, or visibility mode.
- [ ] **Entity print/PDF export** — No per-entity print stylesheet or PDF generation.
- [ ] **Share link for entities** — Campaign-level public mode exists but no per-entity shareable links.
- [ ] **Soft delete / entity archive** — Entities are hard-deleted only. Add `archived_at` column or trash/recycle bin pattern.
- [ ] **Map measurement tool** — Can't measure distance between markers. Leaflet supports this via plugins.
- [ ] **Map fog of war native UI** — Backend exists for Foundry sync but no Chronicle-native fog controls.
- [ ] **Initiative tracker** — No combat ordering tool for session management.
- [ ] **Session prep checklist** — No per-session task list for DM prep items.
- [ ] **NPC quick generator** — Random name/trait generator for improvisation.
- [ ] **Account deletion** — No self-service account removal option.
- [ ] **Member activity tracking** — No last-seen, activity feed, or engagement metrics.
- [ ] **Timeline search/filter** — No search within timeline events by name/text.
- [ ] **Timeline zoom-to-era** — No button to jump viewport to a specific era.
- [ ] **Entity version history UI** — Audit log exists but no "view diff / restore version" for entities.
- [ ] **Toast notification grouping** — Duplicate toasts stack separately instead of grouping.
- [ ] **Entity image gallery** — Only one image per entity; no carousel/gallery for multiple images.

### Phase P: Extension System (Content Extensions — Layer 1)

_Declarative content packs: no code execution, manifest-only. See ADR-021._

- [ ] **Sprint P-1: Extension Infrastructure** — Migration (extensions, campaign_extensions, extension_records, extension_assets tables). Extension model/repository/service. Manifest parser + validator. Zip installer with security checks (file type allowlist, path traversal prevention, SVG sanitization, size limits).
- [ ] **Sprint P-2: Admin Extension Management** — Admin UI for listing/installing/uninstalling extensions. `GET/POST/DELETE /admin/extensions`. Extension detail page showing manifest metadata. On-disk storage in `extensions/` directory.
- [ ] **Sprint P-3: Campaign Extension Enable/Disable** — Campaign settings "Extensions" tab. `GET/POST/DELETE /campaigns/:id/extensions/:ext_id`. Preview endpoint showing what enabling will do. Addon requirement checking.
- [ ] **Sprint P-4: Content Appliers** — Calendar preset applier (replaces calendar config). Entity type template applier (creates entity type). Entity preset applier (creates entities). Tag collection applier (merge). Provenance tracking in extension_records for clean uninstall.
- [ ] **Sprint P-5: Marker Icons & Themes** — Marker icon pack registration (namespaced IDs). Theme variant registration (CSS custom property overrides). Asset serving endpoint (`GET /extensions/:ext_id/assets/*path`).
- [ ] **Sprint P-6: Example Extensions** — Forgotten Realms Calendar (Harptos) pack. D&D 5e Character Sheet entity type template. Sample monster pack. Package as reference implementations for extension authors.

### Phase Q: Extension System (Widget Extensions — Layer 2)

_Browser-sandboxed JS widgets that extend the UI. See ADR-021._

- [ ] **Sprint Q-1: Widget Extension API** — `Chronicle.registerWidget(name, {mount, unmount, config})` API in boot.js. Extension widget discovery and auto-mounting. Widget config schema in manifest.
- [ ] **Sprint Q-2: Widget Extension Distribution** — Allow `.js` files in extension zips (scoped to widget registration pattern). Extension widget blocks appear in template editor palette.

### Phase R: Extension System (Logic Extensions — Layer 3/WASM)

_WASM-sandboxed backend logic via Extism/wazero. See ADR-021._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase F: Foundry Sync Enhancements & Character Integration ✓ (F-1 through F-7 COMPLETE)

_Improve Foundry VTT sync fidelity. Add system-aware character sheet sync. Build toward inventory/NPC features._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase X: System Modularity & Owner Experience

_Validate the full owner pipeline: upload custom system → enable → get presets,
tooltips, Foundry sync, widgets, character sheets. Ensure the system framework
is truly modular and self-service._

- [ ] **Sprint X-5: System-Provided Character Sheet Widgets** — Character sheets are system-authored, not Chronicle core. Each system package ships a widget JS file (via existing `ext_widget` block type from X-3) that reads entity attributes and renders a styled character sheet. Manifest gains `character_sheet` section defining `field_groups` (visual groupings like "Ability Scores", "Combat Stats") with layout hints (grid columns, row spans). D&D 5e gets classic 5e-style layout, PF2e gets PF2e-style, etc. No new block type needed — reuses system widget infrastructure. Chronicle core provides mounting point + attributes API only.

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase A-2: Armory Multi-Instance

_Support multiple named inventory collections per campaign. Current armory is a single campaign-wide view._

- [ ] **Sprint A2-2: Instance UI Polish** — Add/remove items UI on instance view, drag-and-drop reorder, instance description editing, Foundry sync per-instance.

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Entity Manager Widget

_A drag-and-droppable block for entity/category/dashboard pages showing entities from a selected category with sorting, tag filtering, folder creation, and visibility toggles._

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Needs Discussion (Deferred)

- [x] **Sessions** — direction settled and shipped: RSVP flow (email tokens, `/rsvp/:token`), then the availability scheduler (C-SCHED-P1/P2, PR #530/#534 — recurring per-member availability, DM heatmap, slot proposals, per-option responses, notifications, month view). See `internal/plugins/sessions/.ai.md`. Remaining: P3 (confirm-winner → session creation) is in flight now.
- [ ] **Journals** — Need to discuss journal direction — "Obsidian built into the site" vision, personal vs shared notes, folder structure, linking.

### Deferred to Phase S+ (or community contributions)

- [ ] **Module Builder UI** — Guided wizard that helps users create custom game system modules through the web UI. Step-by-step: name/metadata → define categories → define fields per category → paste/upload reference data → preview tooltips → export as module directory. Eliminates need to hand-write manifest.json + data files.
- [ ] Dagger Heart module (system data + Foundry adapter)
- [ ] Whiteboards / freeform canvas (Tldraw/Excalidraw)
- [ ] Offline mode / service worker caching
- [ ] Collaborative editing presence indicators
- [ ] Calendar timezone support / print-PDF export
- [ ] Map hex/square grid overlay
- [ ] Webhook support for external event notifications
- [ ] Widget inline CSS → CSS classes migration
- [ ] Reusable modal/dropdown component library
- [ ] Dice roller widget
- [ ] Encounter difficulty calculator
- [ ] Family tree / genealogy builder
- [ ] Cross-campaign search
- [ ] Mobile-optimized modals (full-screen on small screens)
- [ ] **Knowledge Graph / Mind Map addon** — Interactive graph visualization showing how campaign content is interconnected. Primary view: **Tag Graph** — nodes are tags, edges connect entities that share tags, click a tag to see all entities tagged with it, click an entity to see all its tags. Additional views: **Mention Graph** — nodes are entities, edges are @mention references between them. **Timeline Graph** — nodes are timeline events, edges show event connections and entity involvement. **Relation Graph** (existing, expand) — add tag-based clustering. Designed as a **self-hosted extension addon** — uploadable via the content extension system (Layer 2: widget extension), not built into core. Ships as a reference implementation of the widget extension API. Uses D3.js or Cytoscape.js. Data sourced from existing APIs (tags, relations, entity-names, timeline). Register as addon (`knowledge-graph` slug) with per-campaign enable/disable.

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

---

## 3. Competitive Analysis

Summary of strengths/weaknesses for strategic positioning. Full analysis in `.ai/roadmap.md`.

| Platform | Users | Key Strengths | Key Weaknesses | What Chronicle Should Learn |
|----------|-------|--------------|----------------|----------------------------|
| **WorldAnvil** | ~1.5M | 25+ templates, guided prompts, inline secrets, Chronicles (map+timeline combo), 45+ RPG systems, family trees | BBCode editor, steep learning curve, cluttered UI, aggressive paywall, privacy requires paid | Guided prompts, deep secrets system, RPG system breadth |
| **Kanka** | ~300K | Structured 20-type entities, generous free tier, deep per-role/user permissions, best calendar (-2B to +2B years), GPL source, REST API, marketplace | Summernote editor, complex permission UI, self-hosted deprioritized | Permission granularity, calendar depth, marketplace concept |
| **LegendKeeper** | Small | Best WebGL maps (regions, navigation), real-time co-editing, auto-linking, offline-first, clean UI, speed as brand | Limited entity types, no formal relations, minimal game systems | Auto-linking magic, speed obsession, map interaction depth |
| **Obsidian** | ~4M+ | Local-first markdown, 1000+ plugins, graph view, backlinks, community themes, offline, privacy by default, canvas/whiteboard | Not TTRPG-specific, no calendar/maps/timeline natively (requires plugin cobbling), single-user (no campaign sharing), no web UI | Plugin ecosystem model, graph visualization, local-first philosophy, community extensibility |

### Where Chronicle Already Wins

1. **Drag-and-drop page layout editor** — nobody else has visual page design
2. **Customizable dashboards** (campaign + per-category) — most flexible dashboard system
3. **Self-hosted as primary target** — no paywall, no forced public content
4. **Modern tech stack** — TipTap + HTMX + Templ vs BBCode/Summernote
5. **Per-entity field overrides** — unique; entities customize their own schema
6. **REST API from day one** — matches Kanka, beats WorldAnvil and LegendKeeper
7. **Extension framework** — per-campaign addon toggle
8. **Audit logging** — no competitor has this
9. **Interactive D3 timeline** with eras, clustering, minimap — exceeds Kanka, matches WorldAnvil

### Chronicle vs Obsidian

- Obsidian users cobble TTRPG workflows from community plugins (Fantasy Calendar, Leaflet, TTRPG plugin). Chronicle offers purpose-built calendar/maps/timelines/entity types as first-class features.
- Chronicle has multi-user campaign sharing built-in; Obsidian is single-user.
- Obsidian's plugin ecosystem (1000+) is aspirational — Chronicle's addon system is the foundation for similar extensibility.

---

## 4. Technical Debt (Future Refactoring)

Items identified during the 2026-03-09 codebase audit. Not urgent — document for future sessions.

### Handler File Sizes
Large handler files that could benefit from splitting if they grow further:
- [ ] `entities/handler.go` (1,983 lines) — consider splitting entity type CRUD into separate handler
- [ ] `calendar/handler.go` (1,687 lines) — consider splitting event vs calendar CRUD
- [ ] `campaigns/handler.go` (1,245 lines) — consider splitting members/settings into separate handler

### Service Interface Sizes
Interfaces with 30+ methods that could be split into role-based sub-interfaces:
- [ ] `CampaignService` (40 methods) — could split: CampaignCRUD + CampaignMembers + CampaignSettings
- [ ] `EntityService` (38 methods) — could split: EntityCRUD + EntityTypeService + EntityPermissions
- [ ] `TimelineService` (30 methods) — could split: TimelineCRUD + TimelineEvents + TimelineConnections

### Inline CSS in JS Widgets
Six widgets inject `<style>` elements dynamically. Working correctly (ID-based dedup) but could be moved to `input.css`:
- [ ] `permissions.js`, `shop_inventory.js`, `tag_picker.js`, `entity_tooltip.js`, `relations.js`, `template_editor.js`

---

## Completed Sprints

### Phase 0: Project Scaffolding (2026-02-19)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase 1: Foundation (2026-02-19)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase 2: Media & UI (2026-02-19 to 2026-02-20)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase 3: Competitor-Inspired UI Overhaul (2026-02-20)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase B: Extensions & API (2026-02-20)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase C: Notes & Terminology (2026-02-20)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase D: Campaign Customization Hub (2026-02-22 to 2026-02-24)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase E: Core UX & Discovery (2026-02-24 to 2026-02-25)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase F: Calendar & Time (2026-02-25 to 2026-02-28)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase G: Maps & Geography + Timeline (2026-02-28 to 2026-03-03)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Alpha Documentation Sprint (2026-03-03)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Code Quality Sprint (2026-03-03)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Bug Fixes & Testing Sprint (2026-03-04)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Production Fix + Mobile Nav + Widgets + Foundry Completion (2026-03-04, batch 20)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Calendar Sessions + Entity Widgets + Foundry Security (2026-03-04, batches 21-24)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Alpha Hardening Batch (2026-03-04, batch 25)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase H: Release Readiness (2026-03-04, batches 26-27)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase I Sprint 1: Campaign Export/Import (2026-03-04, batch 27)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase I Sprint 2: Timeline Phase 2B (2026-03-05, batch 28)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Phase I Sprint 3: Calendar Week View (2026-03-05, batch 29)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Sprint K-2: Per-Entity Permissions UI (2026-03-05, batch 36)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Community Bestiary Backend (2026-03-25)
- [ ] Bestiary unit tests — service tests with mocked repo (not yet written)
- [ ] Widget integration — Draw Steel monster widget to call bestiary API endpoints (external repo)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_

### Security Hardening — Audit Completion (2026-03-25)

_Completed entries archived → .ai/archive/todo-completed-2026-06-10.md_
