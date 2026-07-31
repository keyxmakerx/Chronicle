# Chronicle Backlog

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Single source of truth for what needs to be done, priorities,    -->
<!--          and what has been completed.                                     -->
<!-- Update: At the start of a session (to understand priorities), during      -->
<!--         work (to mark progress), and at session end (to reflect).        -->
<!-- Legend: [ ] Not started  [~] In progress  [x] Complete  [!] Blocked      -->
<!-- ====================================================================== -->

## 0. calendar-v4 round 2 — the reveal pass (open)

Round 2 adds no data, no zone and no engine. Five slices; R2-1 has landed.

| slice | what it owns | state |
|---|---|---|
| **R2-1** — `C-CALV4-BENCH-R2` | the disclosure primitive + register, the page measure, the two-column RSVP panel, the glanceable entity seed | **[x] shipped 2026-07-30** |
| **R2-2a** — the day card + the editor's mechanism | click a day, unfold a card, create/edit an event against the SHIPPED event API. **Consumed R2-1's disclosure register in place** — it stayed in `calendar-bench.css` and the card's two rules were added by name inside the same reduced-motion wrapper, reusing all three `--disc-*` tokens; the speculative shared sheet was never built ([DC-6] SIGNED, first-lander clause). **[x] shipped 2026-07-31** |
| **R2-2b** — `C-CALV4-EDITOR-R2b` | the editor's full §5 chrome pass (the type rail's locked hue+pattern+glyph triple, the restricted-audience chip row, the recurrence block with its GM-side chips, the live preview column) **and drag-create**. Split from R2-2a at a stage boundary with everything green, under the dispatch's pre-authorised split. **Gated on the operator's editor stills signature ([DC-5]).** | [ ] |
| **R2-3** — the Block theater | expand any embed to a full-tier overlay. **This is where the depth R2-1 removed from the entity embed comes back.** R2-1 deliberately shipped NO substitute — no expand chip, no "show more", no second embed — because a stopgap becomes the thing R2-3 has to delete. | [ ] |
| **R2-4** — V2 sunset / the anonymous-public route move | the frozen V2 shell. R2-1 did not open `calendar_v2*` for any reason. | [ ] |
| **R2-5** — the sky header | the dashed skyband placeholder in the real-world Block. Drawing pass FIRST. Reuses R2-1's register as its base motion. | [ ] |

Carried out of R2-2a, not closed:

- [ ] **`C-CALV4-DAYPICK-A11Y`** — a day cell needs a focusable control that is
  NOT gated on the docked Ledger. Where the `ledger` layer is off, `dayPick`
  (`instrument.templ:213`) emits no radio and no `.dsel` label, so the day has
  no focusable control at all and the card is POINTER-ONLY for that viewer.
  R2-2a wired both openers (cell `click` + the radio's `change`) and refused to
  inject `tabindex` — that is a Block mutation and it would ship a focusable
  control the server never rendered. This is a WIDGET change with its own
  screen-reader pass.
- [ ] **The editor MORPH** — a shared-element growth from the card's box into
  the editor's, which is more than the register's clip-reveal gives. R2-2a ships
  the register-only version (card closes as editor opens, both from one origin)
  and ESCALATES the morph as a NAMED motion carve-out for the operator to sign,
  alongside the sky header's. Per-surface motion invention is what produced the
  skypane verdict.
- [ ] **`C-CALV4-DAYMENU`** (only if wanted at all) — the day `⋯` overflow menu's
  contents: *Copy link to this date*, *Open Almanac*, *Set campaign date*, and
  the GM `1 hidden here` row. Each needs its own audience or scope ruling; the
  hidden-count row in particular is a hidden-content count on a
  player-adjacent surface.
- [x] **CTS-6, the per-day `+N more` popover — CLOSED as SUPERSEDED.** The day
  card IS the day's full list, which is the feature that booking wanted. Closed
  with the reason rather than quietly dropped.
- [ ] **The §12 screenshot gate for R2-2a was NOT executed** — same reason as
  R2-1's. Every row of it needs a browser: the card open at 1232px light+dark
  NOT occluding the Ledger (a collision there is a STOP-AND-FLAG), at 420px and
  358px, with the `ledger` layer switched OFF, the editor in create and edit
  mode, the full player set checked for ABSENCE, and the reduced-motion capture
  showing the completed state with no partial frame.

Carried out of R2-1, not closed:

- [ ] **`C-CALV4-RIBBON-R2b`** — the PER-TILE ribbon grain. [BR2-3] refused it and
  took the ribbon whole: six controls is the sky directive's count-3 failure, and
  the mobile fold is caused by the stacked tiles rather than by their detail
  lines. It is cheap now if the operator still wants it after seeing R2-1
  shipped, because the register and the store already exist.
- [!] **The two `_tokens.css` defects, booked a FOURTH time, not patched** — the
  `color-mix(… var(--surface-card) …, var(--accent))` pink drift in light, and
  `.badge.need` at 2.59:1. They belong to the `_tokens.css` re-sign pass; patching
  per-file is what keeps them alive. R2-1's disclosure summaries use the
  `transparent 96%` form and carry no `.badge.need` at all.
- [x] **Verifier findings 1–3, fixed forward in stages 6–8 (2026-07-30).** The
  inherited `hx-vals` (the key moved to the POST URL), the closed ribbon's
  missing session clause, and a twisty comment that described a rotation the
  sheet never shipped. Each carries its own pin, and the inheritance rule is
  now written into `bench.templ`, `handler_v2.go` and the plugin's `.ai.md`
  rather than living in one reviewer's head.
- [x] **Verifier findings 2–3 of the second round, fixed forward in stage 9
  (2026-07-31).** Two false motion claims — `bench.templ`'s header denying the
  sheet has any transition, and `calendar-bench.css`:733 naming
  `tools/check-v2-motion-discipline.sh` as this sheet's enforcement when the
  guard's scope has never included `static/css/` (and :50 of the same file said
  the opposite). Both rewritten, and pinned by
  `TestBenchProse_MotionClaimsMatchTheSheet`, which derives the facts rather than
  matching a sentence.
- [ ] **The §13 screenshot gate for R2-1 was NOT executed** — no browser in the
  build environment (Playwright chromium download failed). Every geometric claim
  is derived arithmetically from the shipped sheet and pinned by tests, but the
  following want an eye on a live client: the four closed sections at 1440 / 1024
  / 640 / 390 in light and dark, GM and player; the RSVP panel's two-column
  treatment at 1440 and 1024; the `zone not set` + `Ask →` pair still present at
  390; the calendar above the ribbon at 390 plus a keyboard-tab trace of what the
  reading-order divergence costs; the reduced-motion still proving the disclosure
  is instant AND complete; and the entity embed before/after at host 420px and at
  full tier.

## 1. Bugfixes & Problems

Known broken or missing things, ordered by severity.

### Critical

- [x] **Entity-tie visibility leak on event/era tie lists (C-CAL-ENTITY-TIES-LEAK-FIX)** — `GET /campaigns/:id/calendars/:calId/events/:eid/entities` (RolePlayer-gated) returned every tied entity's NAME/type/icon/color with no visibility filtering; `EntitiesForEvent`/`EntitiesForEra` took only an ID, so a Player could read a dm_only/custom-restricted entity's name via any event or era it was tied to. Sibling `EntitiesForCalendar` was already hardened (cordinator#32 gap #1) — the original audit missed these two. Fixed by threading `role, userID` through both methods + the `CalendarRepository`/`CalendarService` interfaces and applying the same `entityVisibilityFilter`; the handler now sources viewer context via `cc.VisibilityRole()` (co-DM counts as Owner) + `auth.GetUserID(c)`. See `.ai/status.md` 2026-07-24 entry + `internal/plugins/calendar/.ai.md` §"Entity-tie visibility leak fix".
- [x] **3 residual JS-attribute XSS sinks (C-SEC-XSS-JSATTR-SWEEP-R1)** — the follow-up to #560's mandated 56-site sweep, which found these 3 and held scope. Each flowed free text unescaped into an Alpine `x-data` JS string literal: `parent.Name` (entities `parentSelector`, HIGH cross-user stored), `RecurrenceType` (sessions `editSessionModal`, HIGH cross-user stored — `UpdateSession` also never validated it), and `SystemID` (campaigns `settingsGeneralTab`, LOW self-XSS). Fixed via a plugin-local `jsEsc` at each sink (§T-B2, no cross-plugin import) + `UpdateSession` now rejects any non-nil recurrence type outside `{weekly,biweekly,monthly,custom}`. Per-sink regression tests + the service-level rejection test; no new routes/migrations/imports. See `.ai/status.md` 2026-07-19 entry + `cordinator reports/chronicle/2026-07-19-c-sec-xss-jsattr-sweep-r1.md`.
- [x] **Reflected XSS on campaign Settings `?tab` (C-SEC-XSS-SETTINGS-TAB, audit SEC-1)** — owner-targeted reflected XSS: `campaigns.Settings` passed the raw `?tab=` query param into an Alpine `x-data` expression (`settings.templ`), and because the browser HTML-decodes the attribute before Alpine evaluates it as JS, `?tab=%27%29%3Balert(1)%2F%2F` (`');alert(1)//`) executed attacker JS in the owner's session (CSP ships `'unsafe-inline' 'unsafe-eval'` for Alpine; Settings is owner-gated; CSRF token is JS-readable). Fixed in two layers: `sanitizeSettingsTab` (`settings_tabs.go`) allowlists `?tab=` against the viewer's visible-tab set → `"general"` fallback (also fixes the blank-tab-body seed), and a local `jsEsc()` escapes the value at the templ sink (defense in depth, §T-B2 no cross-plugin import). Regression tests in `settings_tab_sanitize_test.go` (exploit payload + hostile/unknown → `"general"`, role-hidden tab rejected, `jsEsc` neutralizes the breakout). `templ generate`/`go build`/`go test ./internal/plugins/campaigns/...`/`go vet` green. Cites §T-B1. Report `cordinator/reports/chronicle/2026-07-19-c-sec-xss-settings-tab.md`.
- [x] **Login "invalid CSRF token" (C-AUTH-LOGIN-CSRF-FIX)** — Root cause: the CSRF cookie name is scheme-dependent (`__Host-chronicle_csrf` over HTTPS, bare over HTTP); behind a TLS-terminating proxy the derived scheme could differ between the form GET (cookie set) and the POST (validate), so `req.Cookie(name)` missed the cookie and compared the form token against a freshly-generated value → guaranteed 403. Fix: `readExistingCSRF` reads the cookie under BOTH names (resilient to scheme flips) + `schemeIsSecure` hardened to parse comma-list `X-Forwarded-Proto`. Part B: friendly no-jargon 403 (`CSRFFriendlyMessage`) and login auto-recovery — a stale/missing-token login POST bounces to `GET /login?expired=1` (HX-Redirect for HTMX, 303 otherwise), which re-issues a valid token + shows a friendly banner. Regression tests in `internal/middleware/csrf_test.go` (set→submit, both scheme-flip directions, recovery, friendly-403, API skip). ⚠️ Operator confirms a real proxied login post-merge (CI can't repro the proxy condition).
- [x] **Restore drill never verified (C-BACKUP-RESTORE-KIT)** — the beta plan's single largest data-loss risk (`cordinator/plans/2026-07-10-beta-transition-plan.md` §2 item 0.6): backups exist but a restore had never once been proven to work. New `tools/restore-drill.sh` — one command, no flags for the happy path — finds the newest backup, restores it into a throwaway MariaDB container (never the live `chronicle-db`: no host port, no shared network/volume, reserved-name guard), and checks migrations-table plausibility + core-table row counts + a spot FK check, printing PASS/FAIL. `docs/RESTORE-DRILL.md` is the operator runbook (when to run, FAIL-reason table, a marked-dangerous "restore FOR REAL" section pointing at the existing `scripts/restore.sh`). `tools/test-restore-drill.sh` + `testdata/restore-drill/` fixtures (1 good + 3 targeted-FAIL) wired into CI so the kit can't rot silently. See `.ai/status.md` 2026-07-18 entry for full detail incl. a real bug this work caught (SQL `\t` escape not expanding via `mysql -e`) and this session's Docker-registry-blocked verification note. **⚠️ Operator: the ACTUAL drill run against a real production backup is still pending** — run `./tools/restore-drill.sh` per `docs/RESTORE-DRILL.md` to close item 0.6 for real; this entry is the tool shipping, not the drill having been run.
- [x] **Calendar embed's event chips were silently inert (C-CAL-EMBED-CHIPS-TAP, wave-8 #549 gate finding)** — the dashboard/entity-page calendar embed (`calendar.templ` `dayCell`) rendered event chips as `<button>` with `cursor-pointer` + a hover ring but no click handler in the embed context (the V1 scripts that once wired them were correctly deleted by #549's V1 sunset). Fixed by making chips real `<a href>` links to the V2 calendar shell, cursored to the tapped day via query params `ShowV2` already parses — no new route (`routes_snapshot.txt` unchanged). See `internal/plugins/calendar/.ai.md` §"Embed event chips now navigate to V2".
- [x] **Worldstate + weather-zone WS events were publisher-side dead letters (C-CAL-WORLDSTATE-WIRE)** — `calendar.worldstate.changed` and `calendar.weather.zones.changed` had live emitters but no `case` in `calendarEventPublisherAdapter.PublishCalendarEvent`, so both hit `default: return` and were discarded before reaching the bus. The operator's meteors and eclipses had NEVER crossed the wire (§2 of `cordinator/plans/2026-07-24-calendar-remodel-requirements.md`: "celestial events (meteors) unfindable/unsyncable"). Fixed additively: new `ws.MessageType` constants + adapter cases (with a whole-mapping regression pin, since a `default: return` silently swallows the next emitter too); an enriched `WorldStateChangePayload` carrying the day's celestial events with their **stable ids**, moons and weather, **split by audience** (player-safe broadcast always; a `RequiresDM` copy only when dm_only rows exist — the hub gates per-message, so one message can't be rich for the GM and redacted for the table); and `GET /api/v1/campaigns/:id/calendar/world-state` on the Bearer syncapi group so a token client can seed/enrich at all. See ADR-047 + `.ai/status.md` 2026-07-26 entry. **Foundry-module consumer half is PR #82 there** — it shipped the handler wired and waiting for exactly this.
- [x] **Every calendar event LIST query was broken against MariaDB (found during C-CAL-RSVP-P1 Step 0)** — `scanEvents` (`internal/plugins/calendar/repository.go`) supplied 36 `Scan` destinations for `eventCols`' 37 columns: `&evt.RecurrenceDayOfWeek` was never added when migration 011 introduced the column, though `eventCols`, `GetEvent`'s own inline scan, and the INSERT/UPDATE all were. Month/week/day/range/upcoming/search/ledger all route through `scanEvents` and therefore failed with `sql: expected 37 destination arguments in Scan, not 36`; only the single-row `GetEvent` worked. Invisible to CI because nothing in the repo executes real SQL (no sqlmock/testify/dockertest in `go.mod`). Fixed in the C-CAL-RSVP-P1 branch because adding `collect_rsvps` to `eventCols` sits on the same lines and would otherwise have left the mismatch in place. New `event_scan_contract_test.go` parses `repository.go` with `go/parser` and pins SELECT arity == Scan arity for BOTH read paths, so the next column cannot repeat it. **⚠️ Operator: worth an eyes-on check that events now list on the V2 calendar — no DB in the build sandbox, so this is verified structurally.**
- [x] **The SAME scan-mismatch landmine hit a THIRD query, entity-side (C-CALV4-TIEFIX-PB Bug 1)** — `EventsForEntity` (`internal/plugins/calendar/entity_ties_repository.go`) lives in a different file than `scanEvents`/`GetEvent`, so the C-CAL-RSVP-P1 fix above never touched it: it scanned 37 destinations against 39 selected columns (`eventCols`' 38 + `l.participation_role`), missing the same `&evt.RecurrenceDayOfWeek` + `&evt.CollectRSVPs` pair. Every entity-side tie read failed at runtime, and `entity_calendar_block.go:81` swallows the error and degrades silently — the entity-page calendar embed has shown no tied events since the day it shipped. Fixed; `event_scan_contract_test.go`'s guard is now file-parameterized and covers this third consumer (proven to fail pre-fix, pass post-fix). Also closed a related gap while in the file: `EventsForEntity` never joined `calendars`, so an event on a `dm_only` *calendar* was reachable through an entity tie — new `EventsForEntityFiltered` method (composed from existing repo methods, no interface widening) closes it, not yet wired into `entity_calendar_block.go` (a different slice's file). See `.ai/status.md` 2026-07-26 entry + `internal/plugins/calendar/.ai.md` §"Entity-side tie reads".
- [x] **Timeline `EventCount` was a visibility oracle (C-CALV4-TIEFIX-PB Bug 2)** — `internal/plugins/timeline/repository.go`'s `List`/`ListByCalendar` computed `EventCount` via two unconditional `COUNT(*)` subqueries while `ListEventLinks`/`ListStandaloneEvents` filtered by base visibility — a player could see e.g. "12 events" and open only 9, the difference being a count of hidden events. Fixed by folding the same conditional predicate into both subqueries. **PARTIAL fix, stated honestly, not claimed closed:** `visibility_rules` (`{allowed_users, denied_users}`) is resolved in Go (`canUserView`), invisible to SQL, so a residual per-event gap remains — booked for a future dispatch. See `.ai/status.md` 2026-07-26 entry + `internal/plugins/timeline/.ai.md` §"EventCount visibility fix" (also lists it under Known Limitations).
- [x] **Calendar events had no RSVPs (C-CAL-RSVP-P1)** — the operator's #1 calendar-remodel priority. The drawer shipped a DISABLED "Collect RSVPs" toggle whose comment ruled that RSVPs needed an event↔session link; superseded by ADR-046. Events now own first-class RSVP storage (calendar migration 013: `calendar_event_rsvps` + `calendar_event_rsvp_tokens` + `calendar_events.collect_rsvps`), a separate `RSVPService`/`RSVPRepository` pair, Player+ answer/count endpoints with Owner/co-DM-gated per-person detail, a Scribe+ collection toggle that fans out members-only visibility-gated action emails, and a public single-use token flow at `/calendar-rsvp/:token` (GET-confirm / POST-apply + CSRF double-submit). "Out this week" writes only the acting user's availability and skips hand-authored days; "suggest another time" writes a note + owner notification and deliberately does NOT mint a slot proposal. Egress pinned by `rsvp_egress_test.go`.

- [x] **Players had no way to give TEMPORARY availability (C-CAL-RSVP-P2)** — C-CAL-RSVP-P1 shipped an asymmetry: "Out this week" wrote structured *un*availability the scheduler could aggregate, while "Suggest another time" wrote only a free-text note the Director had to read and re-key. Players could say when they COULDN'T play in a schedulable way and when they COULD only in prose. The suggestion surfaces (quick-edit card + emailed token page) now take date/from/to rows alongside the optional note; those become real `available` exceptions that land in the DM week overlay and the computed best window. No new tables (`availability_exceptions.state` always accepted `available`/`preferred`) and no new routes. The composition — exceptions REPLACE a date, so a naive write would erase the member's usual hours — lives in `sessions.AddMyAvailableWindows`/`composeOfferedDay` because that invariant is the scheduler's own; enforced server-side because the write can arrive from an email link with no editor in front of it. ADR-046 amendment.

- [x] **The phone calendar's "Agenda" pill was a dead control (C-CAL-MOBILE-VIEWS-FIX)** — `mobileAgendaPill` linked to `v2ViewHref(data,"month")`, the exact URL the page was already on, beside an always-selected "Month" pill: tapping it re-navigated to a byte-identical render (no view change, no error) and it hardcoded `aria-selected="false"` while being the active surface. Replaced with a real two-state toggle on the EXISTING month route — `?agenda=1` → `CalendarV2ViewData.MobileAgenda`; the agenda state drops the mini-month navigator so the list owns the viewport, and each pill owns a distinct URL + honest selection state. **Step 0 DISPROVED the other half of the report:** a 390px headless probe of `main` shows the breakpoint swap was never broken (desktop grid `display:none` at 390px, mobile assembly rendering; reverse at 1200px) — the "full 7-column grid" in the operator screenshot IS the shipped mini-month navigator, which the signed mockup itself renders full-width, made unrecognizable by an events-less month collapsing the agenda to a one-line empty state. See `internal/plugins/calendar/.ai.md` §"Phone Month|Agenda toggle".
- [x] **Static assets shipped unversioned + uncached (C-ASSET-VERSIONING, audit SEC-3)** — Echo's `e.Static` sets no `Cache-Control` at all, so browsers applied heuristic freshness and could serve build-old CSS/JS for hours against freshly-rendered HTML ("the calendar looks old after deploy"). Demonstrated, not assumed: rendering post-C-CAL-MOBILE-AGENDA HTML against the pre-deploy `app.css` drops the Week/Day/Timeline pills from the **desktop** calendar, because `md:contents` was a brand-new Tailwind utility that build introduced. Fixed with `layouts.AssetURL` (per-file SHA-256 `?v=` token, per-build fallback, plugin embed FSes registered via `RegisterAssetFS`) + `middleware.StaticCache` (immutable for versioned, must-revalidate otherwise) + a contract test that fails any `.templ` emitting a bare `/static/` URL. All 101 call sites across 17 templates converted.

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

### Calendar v4 remodel — wave 1 (2026-07-26)

Operator directive: *"just go with a full redo of the calendar."* Master plan
`cordinator/plans/2026-07-26-calendar-v4-remodel-master-plan.md`; canon
`cordinator/decisions/2026-07-26-calendar-v4-canon-amendments.md`; signed,
immutable contract `cordinator/mockups/calendar-v4.html` + `mockups/renders/v4-*`.
Five slices run as parallel chats; the parallelism is bought by three rules —
no slice adds/removes/renames an Echo route, `internal/app/routes.go` belongs to
C-CALV4-SPINE-P2 only, and `calendar_v2.templ` / `calendar_v2_helpers.go` /
`calendar_v2_mobile_agenda.go` / `internal/widgets/calendar_v2/**` are FROZEN.

- [x] **C-CALV4-FOUNDATION-P0 — the data contract + the guard rails.** Landed
  `internal/widgets/calendar_block/data.go` (byte-identical to the coordinator
  pin) + a reflection-only shape pin; defined the three never-defined
  `--motion-*` tokens at their existing fallback values; shipped
  `tools/check-calendar-v4-lints.sh` (B1–B4) and wired it plus the dormant
  `tools/check-v2-motion-discipline.sh` into CI; deleted the vacuous
  `tools/check-templ-drift.sh`; added `make verify`; filed
  `internal/widgets/calendar_v2/.ai.md`. See `.ai/status.md` 2026-07-26 entry.
- [x] **B4 has no subjects yet.** Closed by C-CALV4-SEAM-P5 stages 11–12
  (2026-07-28): stage 11 first fixed the scanner's templ-conditional blind spot
  (a `>` inside `if c.Day > 0 {` truncated the tag, so B4 false-positived on
  correct markup and B3 false-negatived on dirty markup; self-test fixtures now
  cover conditional attributes from both sides), THEN stage 12 had
  `instrument.templ` stamp `data-cell` on dated cells and `data-row` + a `-w`
  answer key on week rows. B4 was watched firing on real markup (key withheld →
  exit 1) and passing correct markup.
- [ ] **`--motion-*` are not Tailwind utilities.** `tailwind.config.js` publishes
  `--ease-*` / `--dur-*` / `--elev-*` as utility classes but not `--motion-*`
  (they are consumed via raw `var()` only). Deliberately left alone — out of
  scope for the foundation slice. Revisit if a V2 surface wants
  `duration-motion-fast`.
- [ ] **Grandfathered `transition-all`.** Now that the motion-discipline guard
  runs in CI, the 7 pre-existing violations it steps over are visible and
  cleanable: `calendar.templ:57,73,88`, `app_dashboard.templ:131`,
  `timeline.templ:143`, `campaigns/settings.templ:219,978`. Not this wave —
  `calendar.templ` is calendar-plugin surface another slice may touch.
- [x] **If the coordinator amends `BlockData`'s identity types to strings** —
  happened: the r51 amendment (C-CALV4-SEAM-P5 stage 1, 2026-07-27) typed all
  four `string`; `data_test.go`'s reflect pins were flipped to `reflect.String`
  in the same commit, exactly as this entry prescribed, and
  `TestBlockIdentityIntFieldsAreZeroed` was INVERTED (now
  `TestBlockIdentityFieldsCarryRealIDs`), not deleted.

### Calendar v4 remodel — booked follow-ups from C-CALV4-BLOCK-P1 (2026-07-26)

W-A shipped the Block's render tier (`internal/widgets/calendar_block` +
`static/css/calendar-block.css`). These are the things it deliberately did NOT
do; each is named in the PR and in the widget's `.ai.md` §"Deliberate
divergences".

- **Wave-2 (W-B), the Ledger: DONE (2026-07-28, C-CALV4-LEDGER-P6).** Zone C is
  filled and lists by ordinal day, reassembled from the cells the grid already
  draws. Day answering is CSS-only (no route, no JS, `routes_snapshot.txt`
  byte-identical) via a generated 40+8-key ladder. The motion question is
  re-opened as a four-item BUDGET and `TestCSS_NoMotionAtAll` is now an
  allowlist; guard B1 and canon D3 both survive it, untouched and green. Pin
  amendment r52 landed three fields. See `.ai/decisions.md` ADR-048 and the
  widget's `.ai.md`.
- **Wave-2 (W-E), the Shelf: DONE (2026-07-28, C-CALV4-SHELF-P7).** Zone D is
  filled with Upcoming, Filters and the Almanac. The Block's four zones are all
  real and the calendar-v4 Block is architecturally complete. Upcoming reuses
  the Ledger's row primitive VERBATIM off the same one filtered pass; Filters
  ships the tab and not the engine (no filter store exists — W-F owns it); and
  the Almanac carries every DECLARED moon at full width, uncapped by MoonCap,
  which is what makes the grid's three-moon ceiling legitimate. Pin amendment
  r53 landed the celestial register. Three CSS-only radio groups now, still no
  JS and still no route. See ADR-048's Almanac section.
- **Wave-3 (W-F), the layer system: DONE** (`C-CALV4-LAYERS-P9`, 2026-07-29).
  The `⋯` invoker is LIVE and opens a top-layer `[popover]` switchboard with no
  JS at all; the per-viewer store is calendar migration 014's `block_layers`
  column on `calendar_active` plus `POST /campaigns/:id/calendar/prefs` (204 +
  `HX-Refresh`, no body in either direction — the wave's only new route, 721
  total); `LayerState` gained `PersistURL` under signed pin amendment **r54**
  with `HasSwitchboard == (PersistURL != "")` pinned; `HasSwitchboard` is true
  at all three producers. TWO of the three chipped zones are FILLED — `legend`
  (r52's `Mark.AxisLabel`, type axis only) and `moongraph` (r53's Almanac
  register, **no new pin field**) — and `SKY_ON()` is restored in one place.
  **DEF is still `["moons"]`:** the slice shipped the mechanism to LEAVE the
  default, which is the opposite of changing it.
  **Split OUT, each under its named follow-on:** the Filters engine
  (**C-CALV4-FILTERS-P10**), the colour-by axis picker and the legend's other
  two axes (**C-CALV4-AXIS-P11**, blocked on DATA — there is no
  `Mark.OwnerLabel` and no per-calendar label), the horizon data and the
  `Reveal through` write (**C-CALV4-HORIZON-P12**, with its own security
  review), the Tonight retarget and the `.sp2`/`.almgrid` ladder extension
  (**C-CALV4-ANSWER-EXT**, in W-B's files), and `moonstyle=words` (un-numbered:
  three of L20's four sky values reduce to layer keys; `words` names a register
  that exists nowhere in Chronicle).
- **`BlockData` field gaps** (each needs a coordinator pin change, so each is a
  stop-and-flag until then): a lead/trail label on `DayCell` (out-of-range cells
  render empty because `Day == 0` IS the out-of-range marker, while the signed
  Gregorian still shows a greyed 28/29/30); **a scope chip** — the Ledger head's
  `opts.scopeLabel` (cv4:1742); W-B bumped into it and OMITTED rather than
  invented; a campaign clock for real-world Blocks; an owner/creator surface on
  `Event`, which is why the Ledger meta line prints the type alone (CTS-5). The
  hidden-event count for the Nameplate's "N hidden" badge is **CLOSED** (r52 +
  C-CALV4-LEDGER-P6, 2026-07-28): `ViewerContext.HiddenCount`, GM-only BY
  CONSTRUCTION — set only when `IsGM`, from the same single viewer-filtered pass
  as the tie pair, and never rendered to a player in any form including a zero. The declared-moon total is **CLOSED** (r51 + C-CALV4-SEAM-P5
  stage 15, 2026-07-28): `buildMonthGeometry` populates
  `MonthGeometry.MoonsDeclared` from `Calendar.Moons`, and the Nameplate
  states "3 of 4 moons" when the declared total exceeds the grid's `moonCap`
  ceiling — three or fewer states nothing extra.
- **Coordinator re-sign, already booked (canon §D):** the `isNamed()` 470-vs-563
  instrument constant. Wave 1 RETIRED the row-height clause rather than
  inheriting or "correcting" it; `TestIsNamed_RetiredRowHeightClause` pins the
  divergence in both directions so the re-sign has something concrete to move.

### calendar-v4 remodel — booked follow-ups from C-CALV4-BENCH-P4 (2026-07-28)

Phase B, W-D: the nav Calendar tab's landing surface.

- [ ] **The anonymous-public gap.** `/apps/calendar` rides the authenticated
  group (`calendar/routes.go:179`) while `/calendar/v2` is public-capable
  (`routes.go:147-149`), so an anonymous visitor to a **public** campaign who
  clicks the nav Calendar tab lands on `/login`. Fixing it needs a route-group
  change plus a `routes_snapshot.txt` regeneration, which no wave-1 slice may
  do → its own wave-2 slice. Booked by the dispatch, not a finding.
- [ ] **The retired card grid is dead code awaiting the post-wave sweep.**
  `CalendarAppDashboardData`, `CalendarAppDashboardPage`,
  `calendarAppDashboardDetail` + its "see in action" children and
  `adaptiveCalendarWidget` are retained (Bounds) and only their tests exercise
  them. Two of them are still LIVE on the Bench (`calendarAppDashboardEmpty`,
  `calendarPermissionsModal`) and must survive the sweep; the associations
  panel's `EntitiesForCalendar` / `TimelineLister` reads have no caller at all
  now and the sweep should decide whether they earn a home on the Bench or go.
- [x] **The Bench does not draw the signed RSVP panel.** — **DONE 2026-07-28,
  `C-CALV4-RSVP-P8` Part A.** ~~Per-member
  availability lanes, the density row, the recommended window and the member
  table with per-member time zones all need stores that do not exist (no
  session entity, no RSVP table, no member time zone).~~ **CORRECTED
  2026-07-28 (C-CALV4-RSVP-P8 §2, ADR-048 §15): that booking was factually
  wrong.** Every store it named had already shipped — `calendar_event_rsvps` +
  `calendar_event_rsvp_tokens` + `calendar_events.collect_rsvps` (calendar
  migration 013), `member_availability` + `availability_exceptions` (sessions
  migration 002), `users.timezone` (core migration 000001), and session
  `scheduled_date`/`scheduled_time` (sessions migration 004). The item is kept
  rather than deleted so the miss stays visible; W-G Part A owns the panel body
  and it needed **no migration at all**, no new route and no JS.
- [ ] **`moongraph` + `horizon` are not in the Bench's layer set.** The signed
  bench render shows both on the primary Block; wave 1 renders each as a
  `needs backend` chip and nothing else, and at std tier the extra need-zones
  stack against the docked Ledger and the Shelf for no information at all.
  W-F fills them and adds the keys — one line in `benchBlockLayers`.
- [ ] **The `.phead` control cluster is two doors, not the signed three.**
  "colour by: type ⌄" is a per-viewer preference with no store (the Block books
  the identical control for the identical reason, `nameplate.templ`) and
  "+ New event" has no create route outside the V2 shell, which wave 1 may not
  add. The cluster ships Open calendar / Builder / + New calendar. W-F and the
  event-editor slice own the other two.

### calendar-v4 remodel — booked follow-ups from C-CALV4-HOST-P3 (2026-07-28)

Phase B, W-C: the Block's first production caller (the entity-page embed).

- [ ] **All four bound widget types need a LIVE-CLIENT render check.** The
  entity-page BlockHost now carries `container-type: inline-size`, declared per
  widget type (`widgetbindings.DeclareInlineSizeHost`) so calendar / worldstate
  / timeline / maps cannot be changed together by accident. The dispatch's
  warning that this would trap the maps block's `fixed inset-0` modals was
  MEASURED AND DOES NOT REPRODUCE (`contain: layout` traps them;
  `container-type` does not — Chromium 141, numbers in the slice report). What
  is still unverified is the three OTHER render paths in a real browser with a
  database, which this sandbox has none of. Once someone runs them, the
  declaration can become unconditional — one line in `block_host.go`.
- [ ] **P3b — dashboard-level widget bindings.** `BindingAffordanceFor` exists
  and carries its own `host_type`, so the affordance is no longer entity-only.
  The CAPABILITY is not added: the binding key is
  `UNIQUE(campaign_id, host_type, host_id, widget_type)` and `BlockHostID` is
  per `(widget_type, host)`, so **two differently-bound calendars on one page
  are not representable** without a schema change. Wave 1 must not promise it.
- [ ] **The entity embed still stacks the almanac sky band above the Block.**
  Kept deliberately: it is live, working, and pinned (the per-band seed-blob id
  scheme, `entity_calendar_block_test.go`), and the Block has no pictorial sky
  (the Skybox is PARKED, L12/D4). The signed entity render shows no band there,
  so whether the band stays on this surface is a design call for whoever owns
  the entity page's composition — not a rendering defect.
- [ ] **The entity page's "no calendar" rung shows a Create-calendar CTA to
  players.** Pre-existing, and now reached by one more path: a calendar the
  viewer may not see renders the same rung (deliberately indistinguishable from
  missing). Softening the CTA per role is a separate, small UX slice.

### calendar-v4 widgetization remodel — follow-ups booked by C-CALV4-SPINE-P2 (2026-07-26)

Wave plan: cordinator `plans/2026-07-26-calendar-v4-remodel-master-plan.md`. These are the
items the server spine surfaced and deliberately did NOT decide on its own.

- [x] **`BlockData` identity types** — closed by the r51 pin amendment
  (C-CALV4-SEAM-P5 stage 1, 2026-07-27): all four fields are `string`, the
  producer carries the real UUIDs, `CalendarSlug` means the slug again, and the
  `TestBlockIdentityIntFieldsAreZeroed` tripwire was inverted into
  `TestBlockIdentityFieldsCarryRealIDs`. This is also what un-gated the tie
  toggle (`HostEntity != ""` now passes for a real host entity).
- [ ] **Multi-day event spans have no field in `BlockData`.** The signed render draws span
  ribbons ("Eclipse window", span 5). Marks are placed by `Event.OccursOn`, which does not
  expand a multi-day event past its start day, and the pinned struct has no ribbon/span
  representation. Booked for the coordinator.
- [x] **Per-mark tie flag.** Closed by r51's `Mark.Tied` + C-CALV4-SEAM-P5
  stage 2 (2026-07-27): the producer emits the WHOLE viewer-visible mark set in
  both tie modes and flags each mark; the renderer stamps `tied`/`untied` and
  the CSS ink rule moved per-DAY → per-MARK. TieMode changes ink, never
  membership — measured payload cost of emitting everything in tied mode: 1
  byte (the mode attribute).
- [ ] **`EraBand.Edge` is unreachable.** The signed render splits one month between two
  eras at day 17/18; Chronicle's eras are year-granular, so a mid-month boundary is not
  expressible. The general rule ships; `TestBlockEraBandEdgeIsUnreachableToday` pins the
  current answer.
- [ ] **The signed "Needs eras" fault has no trigger.** Chronicle has no era-RELATIVE year
  numbering, so a calendar with zero eras still resolves its date. The fault MECHANISM
  ships (no months / month out of range / day out of range); the era variant lands when
  the model gains era-relative reckoning.
- [ ] **Three-counter reconciliation** (its own dispatch, needs an operator gate).
  `Calendar.AbsoluteDay` is leap-aware, `constLenDayIndex` is not, and they diverge by one
  day per elapsed leap year — a per-day moon disc drifts against its own cell by that
  amount. Ruling COMMON §6.4 froze `constLenDayIndex` for wave 1 because changing it would
  shift the weekday column of every calendar in the operator's production database.
  Measured and pinned by `TestBlockCounterDivergencePin`.
- [ ] **`UpcomingByCalendar` still leaks `visibility_rules` names.** `service.go`'s
  dashboard path filters base visibility in SQL only and never calls `filterEventsByUser`.
  The Block's own `UpcomingAcrossCalendars` does not repeat it, but the old path is still
  live on the Calendars dashboard.
- [ ] **`MoonDisc.Eclipse` has no backend.** `calendar_celestial_events` (migration 008)
  has no `moon_id`, so a stored eclipse cannot be attributed to a moon. Same class as the
  fog ruling: the field stays false.

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

- [x] **Sync chip beacon: "saw" → "applied" (C-SYNC-APPLIED-BEACON, 2026-07-18)** — `POST /calendar/date/confirm` (Bearer-only, real API keys) lets the Foundry module report back once it has actually applied a date, not just fetched one (closes the gap #548/C-SYNC-DATE-BEACON flagged as follow-up FM-SYNC-CONFIRMED-DATE). `computeSyncChipState` now prefers a fresh applied date over the served one; degrades byte-identical to today when an older module never calls confirm. Full design: status.md's 2026-07-18 entry + `syncapi/.ai.md` / `calendar/.ai.md`. **Foundry-module half ships separately** (FM-SYNC-CONFIRMED-DATE dispatch, contract in this dispatch's PR body) — until that lands, no module calls confirm and the chip behaves exactly as it does today.

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

### Duplicated curated timezone lists (found + fixed 2026-07-18)
- [x] **Three hand-curated IANA timezone dropdown lists had drifted** (account settings, calendar real-time anchor, availability scheduler) — found + consolidated same session, not left as debt. See `.ai/status.md` 2026-07-18 entry (C-TZ-CONSOLIDATION) for the fix. Noted here so a future session that spots a fourth zone-picking surface knows to add it to `internal/timeutil.CommonZones()` rather than hand-rolling a new list.

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

### calendar-v4 remodel — booked follow-ups from C-CALV4-LEDGER-P6 (2026-07-28)

Wave 2, W-B filled the docked Ledger. These are the things it deliberately did
NOT do; each is named in its report and in the widget's `.ai.md`.

- [x] **W-E (`C-CALV4-SHELF-P7`) is DONE.** It consumed `.lband` and the row
  primitive, landed r53, and restored `moonsBadgeTitle`'s *"— all of them are in
  the Almanac"* tail — gated, so a Block with no reachable Almanac (noShelf, or
  the shelf layer off) still withholds it. It did NOT add its three tabs to
  `.ltabs`: see the new open item below.
- [x] **W-F took ONE of the four things the Ledger left it, and re-booked the
  rest** (`C-CALV4-LAYERS-P9`). **The legend SHIPPED** — `Mark.AxisLabel` was
  exactly the missing datum, type axis only, joined to the count oracle with the
  GM/Nissa/Bryn fixture. The other three did not:
  - the `colour: <axis> ⌄` picker → **C-CALV4-AXIS-P11**, and it is blocked on
    DATA rather than on a decision: there is no `Mark.OwnerLabel` (r52 §5
    refused it, CTS-5 SIGNED "omit") and no per-calendar label, so two of the
    picker's three options cannot be built. P9 omitted them rather than
    fabricate a pressed state, exactly as waves 1 and 2 did. That slice carries
    the pin amendment for both labels.
  - the per-day overflow popovers (CTS-6) → **RE-BOOK OR CLOSE, no owner.** P9
    answered the shared argument (a top-layer popover need not occlude the
    docked Ledger — and its own screenshot gate proved the point twice over),
    but P6's reasoning has since strengthened AGAINST the feature: `+N more` is
    a count, `MoreCount` is overlapping rather than additive, and the docked
    Ledger plus the ANSWER ladder already give the day's full list a home.
    Promoting it would add up to 40 popovers per Block for a list already on
    screen.
  - the horizon → **C-CALV4-HORIZON-P12**, with its own security review and a
    player render at the horizon gating it. `Reveal through` is an owner-only
    WRITE to authored campaign state, on the product's most leak-sensitive axis;
    when it lands it adds the **GRID-side** fog split only — **the Ledger is
    never horizon-filtered**, and that is the argument that let the week-scale
    fog reversal be withdrawn.
- [x] **STRUCK — `.ltabs` keeps `Month` alone, and the bound STANDS** ([LYR-8]
  SIGNED, `C-CALV4-LAYERS-P9`). The candidate mechanism for re-opening ADR-048
  §13 was the per-viewer store, on the theory that it moves the default out of
  the container query. **It does not, and that is a finding rather than an
  opinion:** a store changes WHERE the single value comes from; it is still one
  value, rendered once, into one `checked` attribute, for a Block that may be at
  std on one host and full on another in the same page render. Nothing about the
  store makes one attribute two. And the second, independent reason still holds:
  Chronicle's Block renders the Shelf zone at std as well, so the three tabs
  already have exactly one home per tier and a second copy in `.ltabs` would be
  two controls for one piece of state. [S9] expected four tabs on an assumption
  Chronicle's Block does not satisfy; P7 reported that honestly rather than
  forcing it, and the item is STRUCK rather than re-booked to a fourth wave that
  would re-derive the same answer. If the operator would rather re-sign the std
  still to match what Chronicle actually renders, that is a coordinator drawing
  task, not a Block change.
- [ ] **The std-tier Shelf is a strip plus ~36px of scroller.** At 420px and
  358px the Block cannot afford a month, a filled Ledger and a 166px Shelf at
  once, so Zone D yields (its body scrolls; the Ledger's head and strip cannot)
  and takes what is left. No collision, nothing clipped, and the signed 132px
  ceiling is unchanged at both tiers — but the zone is visibly cramped at std,
  and the signed std render has no Shelf in it to compare against. Coordinator
  eye, same class as the CTS-8 item below.
- [ ] **CTS-8 needs a coordinator eye, not a fix.** The std-tier collision
  measurement was re-taken with the Ledger FULL at both production host widths
  (entity 420px, Bench 358px) and is clean — but only after `.lrows`'s signed
  176px floor was scoped to full tier, where the Block can afford it. Written at
  every tier it kept its declared size inside a 140px zone and landed 36px on
  top of the Shelf. The answer was taken INSIDE the Block's own std geometry; no
  host layer key was dropped and no pinned host file was touched. If the
  coordinator prefers a host change or a re-signed still instead, say so.
- [ ] **`.tm .z` ships as Zone C vocabulary and nothing emits it.** r52 folds
  the zone abbreviation INTO the producer's formatted `Mark.Time` string, so a
  renderer-side split would be the second copy of one fact r52 refused. The
  class is in the sheet for the wave that has a reason to separate them.
- [ ] **A `title` on a Ledger row's chip is now unreachable at named density.**
  The day's stretched selection label covers the cell, so a `.chip`'s hover
  `title` — which carried the audience label — no longer shows. The Ledger row's
  audience chip states it in full instead, which is why this is a trade and not
  a loss; noted so nobody "fixes" it by removing the label.

### calendar-v4 remodel — booked follow-ups from C-CALV4-SHELF-P7 (2026-07-28)

Wave 2, W-E filled the Shelf. With it the Block's four zones are all real. These
are the things it deliberately did NOT do; each is named in its report and in
the widget's `.ai.md`.

- [ ] **The Almanac's Tonight readout does not follow the selected day.** The
  signed anchor is `S.sel || m.today`; this ships the `m.today` half. Selection
  is CSS-only, so retargeting means emitting the whole readout ONCE PER
  SELECTABLE DAY and revealing it through W-B's generated ladder — 40 days ×
  (one line per declared moon plus a lit-list line) on every Block, on top of
  the +67% the docked Ledger already costs, plus an edit to the ladder and to
  the guard that scopes its reveal rule to two named surfaces. Every Almanac
  month cell already carries its `data-day`, so the retarget is one ladder
  extension away for whichever slice pays for it.
- [ ] **The Almanac's month lane does not ANSWER yet.** Its cells carry
  `data-day` and `data-axis="var(--text-primary)"` (guard B4, canon A7's
  datum-hue rule), but W-B's ladder pairs only `.grid` ↔ `.lrows`. Adding
  `.sp2` is a change to the generated ladder's third rule, which is W-B's file.
  Same shape as wave 1 emitting `data-day` for a consumer that did not exist.
- [ ] **`AlmanacDay.Node` is false everywhere and `.abr` ships unused.**
  `calendar.Moon` has no orbital-node column — the mockup's `nodeWindow()` keys
  on a hardcoded flag on its second fixture moon — so the node-window bracket
  would be an interval with nothing behind it. Declared in the sheet the way
  `.cell.fog` is, so the slice that acquires the data adds a field and not a
  stylesheet region.
- [ ] **No moon epithet** ([S6]). `calendar.Moon` has no epithet or description
  column; a migration would be legal and would ship a dead column with no
  authoring surface to fill it, which L5 forbids by name. Re-book with whichever
  wave gives moons an editor.
- [ ] **The `filters N` badge is withheld** ([S4]) and returns WITH the filter
  engine, joining the per-viewer count oracle at that point. A count with no
  denominator behind it is the shape `needs backend` exists to replace.
- [ ] **RE-BOOKED to `C-CALV4-FILTERS-P10`: the Filters engine** ([S2]). W-F
  built the store the engine needs — calendar migration 014's `block_layers`
  column, designed to grow one more COLUMN rather than one more table — and
  built nothing of the engine itself: there is still no filter state on
  `BlockData`, no filter query, and deliberately no `ViewerContext.Filters` pin
  field (r54 §5 refused one by name; `unused` reds a field added ahead of its
  consumer). `shelf.templ`'s panel comment is rewritten accordingly — the store
  exists, the engine does not, and the named slice owns it. The Filters TAB and
  its `needs backend` chip still ship; the `filters N` badge stays withheld
  ([S4]) until the engine joins the count oracle with it.
- [x] **CLOSED AS SUPERSEDED — the `moongraph`/`horizon` host-seed re-add**
  ([S11], and the HOST-P3 §4.2 / BENCH-P4 §5.7 bookings with it; [LYR-7]
  SIGNED). W-F filled both zones and DELIBERATELY DID NOT add either key to
  either seed. The booking's stated purpose was REACHABILITY, and the
  switchboard supplies reachability directly; L29 says the illumination graph's
  default is OFF, so seeding it would contradict the law that put it in W-F; and
  `horizon` is still chipped, so seeding it would ship a `needs backend` chip
  into a default view — the exact inverse of the DEF ruling. `benchBlockLayers()`
  and `entityBlockLayers()` still carry their five keys, DEF is still
  `["moons"]`, and no zone flag was minted.
  **[S11]'s caveat was honoured and it FIRED.** The re-add measurement was taken
  fresh rather than inherited, at 420px and 358px with a REAL legend and a REAL
  three-lane graph, and it COLLIDED: the docked Ledger's head drew over the
  Shelf's strip. Fixed per CTS-8's precedent inside the Block's own std geometry
  — the body scrolls, the month keeps every week row, the Shelf's strip stays
  pinned, and no layer key was dropped. See the P9 report §"the gate fired".
- [ ] **"Owner/scribe-configurable" is struck for wave 2** ([S3]). No signed
  artefact contains a Shelf configuration surface, and building one would be the
  fifth authoring surface L5 forbids. Re-book "who configures the Shelf, and
  where" to the wave that ships a config store, with its own design pass first.

### calendar-v4 wave 3 — booked by C-CALV4-RSVP-P8 Part A (2026-07-28)

Part A filled the Bench's signed RSVP panel. These are what it deliberately did
NOT take, each with the reason it was left.

- [ ] **W-G PART B — `GET /campaigns/:id/schedule`.** The Verdict, the
  Director's matrix, the Roster, the Painter and the Answer panel, in a new
  `.cal-schedule`-scoped stylesheet. **GATED on a coordinator drawing pass**:
  `mockups/calendar-v4-schedule.html` and the 17 starred stills do not exist,
  and the design spec's own header forbids building from it. Part A left it
  clean — **no `.sc-`-prefixed class ships, no `/schedule` route is reserved,
  and the panel head is not yet a link** (a link to a 404 is worse than no
  link). `tools/check-calendar-v4-lints.sh`'s B4 glob already covers
  `*schedule*`, and its self-test proves the resolution, so the file will be
  guarded on the day it is created.
- [x] ~~**`C-CALV4-RSVP-P8B` — "the asking email".** One endpoint, one template, a
  rate limit and a tokened link to the availability grid. It is what flips the
  `no reminder endpoint` honesty state and un-chips the Bench's `Nudge`, and it
  is the operator's own directive
  (`decisions/2026-07-28-operator-directive-availability-solicitation.md`). The
  RSVP fan-out today fires ONLY on the `collect_rsvps` OFF→ON transition.~~
  **DONE 2026-07-29** — struck rather than deleted, so the record shows what was
  booked beside what shipped. Shipped: `POST /campaigns/:id/calendar/ask`
  (Scribe+, snapshot 721→722), one email template with two sections, migration
  `015_schedule_asks` behind a persisted 6h campaign cooldown + 24h
  per-recipient floor plus a per-user 10/h in-memory limiter, and the panel's
  `Nudge` LIVE with both tile Nudges retired. **One correction to the booking,
  signed as [PB-2]:** *"a tokened link to the availability grid"* is not
  buildable — the grid is a 1,318-line client SPA over SIX authed JSON routes
  (`sessions/routes.go:43-49`), so a token would have to authenticate all six
  or mint a session from an emailed link. It ships as a plain deep link plus
  destination preservation through `/login`, and the directive's one-time-token
  pattern is honoured by the RSVP action links in the SAME email, unchanged.
- [ ] **`C-NOTIFY-PREFS` — per-user, per-channel notification preferences, plus
  the admin surface.** **NEWLY BOOKED 2026-07-29 by C-CALV4-RSVP-P8B, which is
  what made it necessary.** `grep notification_pref|email_opt|unsubscribe` over
  `internal/` and `db/` returns NOTHING. Until the asking email, every Chronicle
  email was transactional and self-triggered — password reset, invite accept,
  "you were invited to this event". **`POST /campaigns/:id/calendar/ask` is the
  first email a member can receive repeatedly because somebody ELSE pressed a
  button.** That is defensible at a 6-hour per-campaign cooldown plus a 24-hour
  per-recipient floor, and it is not defensible forever. P8B deliberately
  invented no unsubscribe link in the meantime: a dead link is worse than the
  email's honest *"you received this because you are a member of this
  campaign."*
- [ ] **A plain Scribe has the ask capability and no button.** `POST
  /campaigns/:id/calendar/ask` is `RequireRole(RoleScribe)` (matching the
  `rsvp-collection` toggle it re-sends), but the Bench renders the control under
  `IsGM` — `permissions.CanSeeDmOnly`, i.e. Owner **or** DM-granted. So a Scribe
  without a DM grant can call the endpoint and has no affordance for it. That is
  deliberate: WG-8 signed the panel's `.side` as GM-tier and P8B did not reopen
  it. **Booked, not fixed** — and the same shape `rsvp-collection` already has.
- [ ] **The propose-from-window write path.** `routes_snapshot.txt` carries no
  such route; `POST /campaigns/:id/proposals` is Scribe+ and takes explicit
  options. The derived window is real and Propose is inert beside it — the gap
  is stated in the panel's caption, not hidden.
- [ ] **Ledger #3 stays open and is Part B's.** `OverlayMember` has no
  `HasPattern`, so *"never answered"* is still indistinguishable from *"busy all
  week"*. It is the W-G spec's own single most important gap and the Bench panel
  cannot close it — the lanes have one mark, and a second one needs the mark
  vocabulary the drawing pass settles.
- [ ] **"Out just this week" is not a fourth status and did not ship.**
  `RSVPAction` already carries `out_week` and `rsvpWeekDates` resolves it, but
  the control needs a confirm popover naming the resolved week and the
  hand-set days it leaves alone — and **zero popovers exist in the calendar-v4
  surfaces**; W-F ships the first. If it ever lands: NEVER a silent success. On
  a degraded response the panel must print *"only the RSVP was recorded — your
  availability was not changed"* in `--warn`.
- [ ] **The `.mtable` head's slot picker is a NON-INTERACTIVE label.** The
  mockup draws a `popovertarget` there; same reason as above.
- [ ] **Availability ENTRY stays where it is.** `GET /campaigns/:id/availability`
  and its 1,310 lines of `static/js/availability.js` are untouched and the nav
  still points at them. When Part B's Painter ships it is reached ONLY from the
  Bench panel, and **retiring `/availability` is its own slice with its own
  fidelity gate** — `internal/app/routes.go` documents that the calendar's
  out-week adapter deliberately mirrors `availability.js`'s `fireOutWeek`, so
  that retirement owns the comment's referent too.
- [ ] **WG-5's chip-beside-every-disabled-control rule is product-wide but only
  IMPLEMENTED where wave 3 touched.** `helpers.go`'s `layersInvokerTitle` is
  still `title`-only; W-F makes that invoker live, so the state disappears on
  its own. Promote the rule to a blocking lint once both wave-3 slices land.
- [ ] **The `/apps/calendar` anonymous-public gap is still open and still its own
  slice.** Deliberately NOT folded into P8: moving that route onto a
  public-capable group in the same slice that fills the page with per-member
  roles, zones and clocks would make the security review reason about a nil
  viewer against brand-new code, on the most person-identifying surface in the
  product. Do it against settled code, with `buildBench`'s degrade ladder and the
  W5a `ListVisibleCalendars` path each proven for a nil viewer.
- [ ] **Density's per-day numerator is "the busiest hour of that column".** It is
  the only per-day reduction the per-hour aggregate supports without inventing a
  rule, and each bar states its own denominator in `title`. If the drawing pass
  ever specifies a different reduction, it is one function
  (`benchRsvpDensity`) and the count oracle moves with it.
