# Data Model

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Quick reference for the database schema. Avoids reading all      -->
<!--          migration files to understand the data model.                   -->
<!-- Update: After every migration is written or applied.                     -->
<!-- ====================================================================== -->

> The migrations are the source of truth: `db/migrations/*.up.sql` (core, runs
> first) and each plugin's `internal/plugins/<name>/migrations/*.up.sql`
> (plugin, runs after core, in the order `cmd/server/main.go` registers them).
> This file is a derived summary — regenerate the parts that changed whenever
> a migration is added.

Core tables live in `db/migrations/`. Plugins that own tables have a
`migrations/` directory under `internal/plugins/<name>/`; every other plugin
reuses core tables and owns none of its own (ADR-028).

## Conventions

**Naming.** Tables and columns are `snake_case`, table names are plural.
Primary keys are `id`: `CHAR(36)`/`VARCHAR(36)` UUID (generated in Go) for
anything a user creates, `INT`/`BIGINT AUTO_INCREMENT` for join tables and
logs. A campaign-scoped table carries `campaign_id`. Most tables have
`created_at`/`updated_at` DATETIME columns, usually `DEFAULT CURRENT_TIMESTAMP`
/ `ON UPDATE CURRENT_TIMESTAMP`.

**Ownership.** `created_by` records the acting user on most tables and has no
FK-cascade behavior that deletes content when the user is deleted.
`entities.owner_user_id` is different: it is the nullable, player-claimable
"this is my character" pointer (`ON DELETE SET NULL`). `campaign_members.role`
(`owner`/`scribe`/`player`) is the campaign-wide authority tier;
`campaign_members.character_entity_id` links a member to the entity they've
claimed.

**Visibility model.** Two independent layers:
1. **Coarse:** `is_private` (legacy DM-only flag on `entities`) and `dm_only`
   (on `tags`, `entity_relations`) hide something from players outright.
2. **Fine-grained:** `entity_permissions` / `tag_permissions` grant `view` or
   `edit` to a `subject_type` (`role` / `user` / `group` / `public`) +
   `subject_id`. `entities.visibility` (`default`/`custom`) selects which mode
   applies. Grants are **additive only** — they can widen visibility (e.g.
   reveal a `dm_only` entity to one player via a tag grant) but never narrow
   it. `maps`, `map_markers`, `map_drawings`, `timelines`, `timeline_events`,
   `timeline_event_links`, `entity_notes` and
   `calendar_events` carry their own `visibility` enum plus a
   `visibility_rules` JSON `{allowed_users: [], denied_users: []}` for
   per-object overrides.

`entity_notes.audience` is a separate five-tier enum (`private`/`dm_only`/
`dm_scribe`/`everyone`/`custom`) — a per-note-author model layered on top of,
not a replacement for, entity-level visibility.

**Soft state.** Chronicle does not use a blanket `deleted_at` convention —
almost everything is hard-deleted via `ON DELETE CASCADE` from its parent. The
one soft-state field is `campaigns.archived_at` (NULL = active); an archived
campaign stays in the database and is excluded from active-campaign lists.

**JSON conventions.**
- Rich text: an `entry` / `content` / `body` JSON column (a TipTap/ProseMirror
  document) paired with a pre-rendered, pre-sanitized `*_html` column, which is
  what templates actually render via `templ.Raw`.
- `fields_data` / `field_overrides` on `entities` hold entity-type-specific
  custom field values.
- `settings`, `manifest`, `config_schema`, `config_json` hold free-form config
  blobs read by Go code, not queried with `JSON_EXTRACT` in practice.
- `visibility_rules` is always shaped `{allowed_users: [], denied_users: []}`
  when present.

## Core schema

Core tables live in `db/migrations/000001_baseline.up.sql` — idempotent
(every `CREATE TABLE IF NOT EXISTS`), collapsing what used to be 63 sequential
core+plugin migrations into their final shape (ADR-028) — plus incremental
numbered core migrations after it, tracked once each via `golang-migrate`'s
`schema_migrations` table.

### Users & site

| Table | Purpose | Notable columns |
|---|---|---|
| `users` | Accounts | `email` UNIQUE; `password_hash` (argon2id); `totp_secret`/`totp_enabled`; `is_admin`, `is_disabled`; `pending_email`/`email_verify_token` (email-change flow); `admin_nav_pins`, `view_prefs` JSON (the person's own pins and My view choices) |
| `password_reset_tokens` | Forgot-password flow | `token_hash` UNIQUE; FK→`users` CASCADE; 1h expiry |
| `security_events` | Site-wide security audit log | `event_type`, `user_id`/`actor_id` nullable; `details` JSON; indexed by type/user/ip/actor + `created_at` |
| `site_settings` | Global key/value settings | `setting_key` PK |
| `smtp_settings` | Outbound email config (singleton) | `id` CHECK = 1; `password_encrypted` AES-256-GCM |
| `user_storage_limits` | Per-user upload/storage overrides | PK `user_id`; `bypass_*` columns for temporary admin-granted bypass |
| `admin_activity` | Site-wide log of changes made from the admin area, shown on the admin Home page | `actor_user_id` deliberately not a FK, so rows outlive the account; `action`, `target_type`/`target_id`/`target_label`; `detail` JSON |

### Campaigns & membership

| Table | Purpose | Notable columns |
|---|---|---|
| `campaigns` | A worldbuilding project | `slug` UNIQUE; `settings`/`sidebar_config`/`dashboard_layout`/`owner_dashboard_layout` JSON; `is_public`; `archived_at` (soft-archive); `join_code` UNIQUE (shareable invite link) |
| `campaign_members` | Campaign ↔ user, with role | composite PK `(campaign_id, user_id)`; `role` CHECK IN (`owner`,`scribe`,`player`); `character_entity_id` FK→`entities` SET NULL; `nav_pins` JSON (the member's own pinned sidebar row keys, NULL for none) |
| `campaign_invites` | Email invitations | `token` UNIQUE; `role` CHECK IN (`player`,`scribe`); `expires_at`/`accepted_at` |
| `ownership_transfers` | Pending campaign-owner handoff | one pending per campaign (`campaign_id` UNIQUE); `token` UNIQUE; 72h expiry |
| `campaign_storage_limits` | Per-campaign upload/storage overrides | PK `campaign_id`; `bypass_*` columns |
| `campaign_groups` / `campaign_group_members` | Named member groups (a permission `subject_type`) | `UNIQUE(campaign_id, name)`; members table is a plain junction |
| `audit_log` | Per-campaign action log (create/update/delete) | `action`, `entity_type`, `entity_id`, `entity_name`, `details` JSON |

### Entity types & entities

| Table | Purpose | Notable columns |
|---|---|---|
| `entity_types` | Per-campaign content categories (Character, Location, ...) | `UNIQUE(campaign_id, slug)`; `fields`/`layout_json`/`dashboard_layout`/`pinned_entity_ids` JSON; `preset_category` (system-preset origin); `parent_type_id` self-FK (sub-types, SET NULL); `claimable` tri-state BOOLEAN (NULL = heuristic, see `isClaimableType`) |
| `entities` | The core worldbuilding content unit | `UNIQUE(campaign_id, slug)`; `entity_type_id` FK; `parent_id` self-FK **or** `parent_node_id` FK→`sidebar_nodes` (mutually exclusive nesting); `owner_user_id` FK→`users` SET NULL (claim); `map_id` FK→`maps` SET NULL (cross-plugin FK added by maps plugin migration 005, since core can't reference a plugin table); `entry`/`entry_html`, `player_notes`/`player_notes_html` JSON+HTML pairs; `fields_data`/`field_overrides`/`popup_config` JSON; `search_text` FULLTEXT (backfilled from `entry_html`+`fields_data`); `visibility` enum; `is_private` legacy flag; `FULLTEXT(name)`; `deleted_at`/`deleted_by`/`trash_root_id` (Trash, ADR-060: every read excludes `deleted_at IS NOT NULL`); `entry_rev` (text revision for save clashes) |
| `entity_versions` | A page's title and text history | FK→`entities` CASCADE; `kind` (`created`/`edit`/`restore`/`baseline`); `user_id` no FK (history outlives the account); `DATETIME(6)` times |
| `entity_aliases` | Alternate names an entity is searchable/linkable by | `UNIQUE(entity_id, alias)`; `FULLTEXT(alias)` |
| `sidebar_nodes` | Pure organizational folders in the sidebar tree (no page content) | `node_type` enum(`folder`); self-nestable via `parent_id` |
| `entity_favorites` | Per-user sidebar bookmarks | composite PK `(user_id, entity_id)` |
| `saved_filters` | Per-user saved tag-filter presets | `tag_slugs` JSON array |

### Entity content, tags & permissions

| Table | Purpose | Notable columns |
|---|---|---|
| `entity_posts` | Entity-scoped sub-pages/journal posts, shared by all members | `entry`/`entry_html`; `is_private` (DM-only post) |
| `entity_notes` | Per-author, per-entity notes with a 5-tier audience ACL | `audience` enum(`private`,`dm_only`,`dm_scribe`,`everyone`,`custom`); `shared_with` JSON (only when `audience='custom'`); `body`/`body_html` |
| `entity_permissions` | Fine-grained view/edit grants on one entity | `UNIQUE(entity_id, subject_type, subject_id)`; `subject_type` enum(`role`,`user`,`group`,`public`); `permission` enum(`view`,`edit`) |
| `entity_relations` | Typed links between two entities | `UNIQUE(source_entity_id, target_entity_id, relation_type)`; `reverse_relation_type`; `metadata` JSON; `dm_only` |
| `tags` | Per-campaign labels, nestable | `UNIQUE(campaign_id, slug)`; `dm_only` |
| `entity_tags` | Entity ↔ tag junction | composite PK |
| `tag_permissions` | Visibility grant carried by a tag — widens visibility for any entity bearing it | `UNIQUE(tag_id, subject_type, subject_id)`; same `subject_type` enum as `entity_permissions`; `created_by` has no FK (deleting the granter must not cascade-delete the grant) |

### Notes (campaign floating panel)

| Table | Purpose | Notable columns |
|---|---|---|
| `notes` | Journal notes (no page) and per-page jots, with folders and live-edit locking | `parent_id` self-FK (folder nesting); `is_folder`; `content` JSON (legacy block array) + `entry`/`entry_html`; audience in `is_shared` (party) / `shared_with` JSON user ids / `shared_with_gm`; `archived_at`; `linked_note_id` (jot → Journal note, no FK); `locked_by`/`locked_at` (edit lock) |
| `note_versions` | Snapshot history on each save | FK→`notes` CASCADE; mirrors `notes`' content columns |
| `note_attachments` | Audio + transcript attachments on a note | `duration_secs`, `transcript` LONGTEXT |
| `notes_app_grants` | A player's grant for an outside app (the Foundry notebook) to use their notes in one campaign | stores only the token's SHA-256; `origin`, `last_used_at`, `revoked_at` |

### Templates & prompts

| Table | Purpose | Notable columns |
|---|---|---|
| `content_templates` | Pre-filled editor content for entity creation | global (`is_global`, `campaign_id` NULL) or per-campaign; optional `entity_type_id` scope |
| `worldbuilding_prompts` | Guided writing prompts | same global/per-campaign, optional per-type shape as `content_templates` |
| `layout_presets` | Reusable entity-page layout configs | `layout_json` (same shape as `entity_types.layout_json`); `is_builtin` (protected from edit/delete) |

### Shop & inventory

| Table | Purpose | Notable columns |
|---|---|---|
| `shop_transactions` | Historical record of purchases/sales/transfers/gifts between entities | `transaction_type`; `price_paid` (display string) + `price_numeric` DECIMAL (aggregation); `instance_id` FK→`inventory_instances` (nullable) |
| `inventory_instances` | Named item collections ("Party Loot", a shop's stock) | `UNIQUE(campaign_id, slug)` |
| `inventory_items` | Instance ↔ entity junction with quantity | `UNIQUE(instance_id, entity_id)` |

| `shop_rooms` | A shop page's walk-in room layout | PK `shop_entity_id` FK→`entities` CASCADE; `layout` LONGTEXT (a normalized JSON document; goods stay in `entity_relations`) |
| `shop_purchase_requests` | A player's ask to buy a basket outside downtime, answered by the GM | `basket` JSON; `quoted_total` + `quoted_currency` (the price the player saw); `status` enum(`pending`,`applied`,`declined`,`failed`); `buyer_entity_id` is the character paying |

### Stashes & downtime

| Table | Purpose | Notable columns |
|---|---|---|
| `stashes` | A shared hoard of items and money, outside any character | `UNIQUE(campaign_id, name)`; `money` DECIMAL with `CHECK (money >= 0)` |
| `stash_viewers` | Which characters may see a stash | composite PK `(stash_id, character_entity_id)`; both FKs CASCADE |
| `stash_items` | Items held by a stash | composite PK `(stash_id, item_entity_id)`; `CHECK (quantity > 0)` |
| `item_moves` | Ledger of every item or money move between characters and stashes, and the GM's answers to requests | `kind` enum(`item`,`money`); `from_kind`/`to_kind` enum(`character`,`stash`); `status` enum(`applied`,`pending`,`declined`,`failed`); `requested_by`, `decided_by` carry no FK |
| `campaign_downtime` | Whether the campaign's downtime is open (players may move and buy without asking) | PK `campaign_id`; `is_open`; absent row = closed |

### Rulebook edits

| Table | Purpose | Notable columns |
|---|---|---|
| `campaign_book_chapters` | A campaign's own chapters in an installed system's Rulebook | PK `(campaign_id, system_id, chapter_id)`; `director` marks a Director-only chapter; ids start `house_` |
| `campaign_book_pages` | The campaign's edited or added pages in those chapters | `page_json`; `package_index` (0-based position of the package page it replaces, NULL for a page the campaign added); `base_hash` (hash of the package page it was copied from, so a later package change is noticed) |

### Campaign system entries

| Table | Purpose | Notable columns |
|---|---|---|
| `campaign_system_entries` | A campaign's own pick-list entries (ancestry, kit, culture, class...) for its game system, kept apart from the package's data | `UNIQUE(campaign_id, system_id, field_key, slug)`; `visibility` enum(`everyone`,`directors`); `properties` JSON (flat scalars); `description` sanitized HTML; `created_by` carries no FK; FK to `campaigns` CASCADE |

### Media, addons & extensions

| Table | Purpose | Notable columns |
|---|---|---|
| `media_files` | Uploaded file metadata | `content_hash` (sha256, per-campaign dedup — `INDEX(campaign_id, content_hash)`); `thumbnail_paths` JSON; `usage_type` |
| `page_files` | Which page a `page_file` media file is attached to (migration 45) | `media_id` PK FK→`media_files` CASCADE; `entity_id` FK→`entities` CASCADE; `campaign_id`; `gm_only` |
| `addons` | Registry of installable features (systems/widgets/integrations/plugins) | `slug` UNIQUE; `category` enum; `status` enum(`active`,`planned`,`deprecated`); seeded by the baseline migration so the registry exists even if a plugin's own schema migration fails |
| `campaign_addons` | Per-campaign addon enablement | `UNIQUE(campaign_id, addon_id)`; `config_json` (the `"setup"` key holds extension-settings wizard state, ADR-043) |
| `extensions` | Installed WASM extension manifests | `ext_id` UNIQUE; `manifest` JSON |
| `campaign_extensions` | Per-campaign extension enablement | composite PK; `applied_contents` JSON |
| `extension_provenance` | Which records an extension created, for clean removal | `(table_name, record_id)` indexed |
| `extension_data` | Extension-owned key/value data, namespaced | `UNIQUE(campaign_id, extension_id, namespace, data_key)` |
| `extension_schema_versions` / `plugin_schema_versions` | Applied-version ledgers, written by the extension and plugin migration runners respectively | composite PKs `(extension_id/plugin_slug, version)`; read only by those runners, not by product features |

## Plugin schema

Plugin migrations run after core, in the order `registeredPlugins()` lists
them in `cmd/server/main.go`. A plugin's schema failing to migrate degrades that
plugin only (`internal/database/plugin_health.go`) — it never blocks boot.

### bestiary (`internal/plugins/bestiary/migrations/`)

Community-shared creature stat blocks, instance-wide (not campaign-scoped).

| Table | Purpose | Notable columns |
|---|---|---|
| `bestiary_publications` | A published/draft stat block | `slug` UNIQUE; `statblock_json`; `visibility` enum(`draft`,`published`,`unlisted`,`archived`,`flagged`); `system_id` DEFAULT `''` (must come from the publishing campaign's own system, not a hard-coded fallback); `FULLTEXT(name, description)` |
| `bestiary_ratings` | 1–5 star rating + review text | `UNIQUE(user_id, publication_id)` |
| `bestiary_favorites` | Per-user favorites | composite PK |
| `bestiary_imports` | Which campaigns imported a publication, and the resulting entity | `UNIQUE(publication_id, campaign_id)` |
| `bestiary_flags` | Per-user abuse flags (dedups repeat flags) | composite PK `(user_id, publication_id)` |
| `bestiary_moderation_log` | Moderator action history | `action` enum(`approve`,`flag`,`unflag`,`archive`,`restore`) |

### calendar (`internal/plugins/calendar/migrations/`)

Migrations 001–018 built the old calendar; `019_calv5_clean_slate` emptied
`calendars` and `calendar_events` and dropped every other table, and
`020_calv5_schema` onward is the V5 rebuild (#741). `019` and its two
siblings are one-way (see `docs/deployment.md` §6). `calendars` and
`calendar_events` are the same tables `sessions` and `timelines` still hold
foreign keys into (migrations are append-only, so those FKs stay). What the
plugin does with them: `internal/plugins/calendar/.ai.md`.

| Table | Purpose | Notable columns |
|---|---|---|
| `calendars` | A campaign's calendar | visibility columns; real-world date anchor and time zone; `hemisphere`, `forecasts_enabled`, `month_starts_new_week`; era look: `era_colors_on`, `era_feel`, `era_intensity`, `era_speed` (migration 024) |
| `calendar_months`, `calendar_weekdays` | The year's shape | `sort_order`; months carry `is_intercalary`, `leap_year_days`; weekdays `is_rest_day` |
| `calendar_moons`, `calendar_seasons` | Moons (cycle, offset, look, `hidden_from_players`) and seasons (start/end month and day) | per calendar, FK CASCADE |
| `calendar_eras` | Named stretches of years | `start_*`/`end_*`; look: `color`, `color_2`, `style`, `feel`; `lore_entity_id`, `dm_note`, `hidden_until_begins` (migration 024) |
| `calendar_cycles`, `calendar_cycle_entries`, `calendar_festivals` | Repeating year-cycles and their entries; named festival days | `cycle_length`; festival `month`/`day` or `after_month` |
| `calendar_event_kinds` | Per-campaign kinds of event (name, icon, colour, default announcement) | `UNIQUE`-slug per `campaign_id` |
| `calendar_events` | Events | `kind_id`, `announced`, `payload` JSON; `recurrence_rule` JSON, the "repeats by rule" shape (migration 022) |
| `calendar_event_overrides` | Skip or move one occurrence of a repeating event | PK `(event_id, occurrence_year, occurrence_month, occurrence_day)`; `action` enum(`skip`,`move`) with `new_*` date |
| `calendar_weather` | The calendar's current-weather snapshot | one row per calendar |
| `calendar_weather_days` | Weather for one date, generated or set by hand | PK `(calendar_id, year, month, day)`; `source` (`manual` by default); `locked` keeps Generate from replacing it (migration 025) |
| `calendar_weather_settings` | The world's climate and how long weather lasts | PK `calendar_id`; `climate`, `continuity`, `kinds` (the owner's own kinds of weather), `forecast_days` (migration 025) |
| `entity_era_links`, `entity_event_links` | Pages tied to an era or an event | `participation_role` |

### maps (`internal/plugins/maps/migrations/`)

| Table | Purpose | Notable columns |
|---|---|---|
| `maps` | A campaign map image + viewer config | `image_id` FK→`media_files` SET NULL; `grid_*`/`background_color`/`initial_view_*`/`initial_zoom`; `foundry_scene_id` (Foundry sync); `display_settings` JSON nullable (frame/pins/kinds/grid/opening view/draw gate, validated in the service, NULL = all defaults; migration 007) |
| `map_campaign_settings` | Campaign-wide default map frame | `campaign_id` PK, FK→`campaigns` CASCADE; `frame_style` (no row = `atlas`) |
| `map_markers` | Pins on a map | `x`/`y` percentage 0–100; `entity_id` FK→`entities` SET NULL; `pin_category`; `visibility`/`visibility_rules`; `foundry_id` |
| `map_layers` | Ordered drawing/token/fog layers | `layer_type`; `is_visible`/`is_locked`/`opacity` |
| `map_drawings` | Freehand/shape/text/shadow annotations and pictures on a layer | `points` JSON; `visibility`/`visibility_rules`; `foundry_id`; `image_id` (picture's media file, no FK), `crop` JSON, `sort_order` (migration 008, pictures only) |
| `map_tokens` | Positioned tokens (often an entity's avatar) | `entity_id` FK SET NULL; `bar1/2_value/max`, `aura_*`, `light_*`, `vision_enabled/range` (Foundry-parity fields); `status_effects`/`flags` JSON; `foundry_id` |
| `map_fog` | Explored/unexplored fog-of-war polygons | `points` JSON; `is_explored` |
| `map_hex_layers` | A map's hex layer (at most one per map, on when `display_settings` grid type is hex) | PK `map_id` FK→`maps` CASCADE; `anchor_drawing_id` (a picture on the map, no FK); `fog_enabled`; `party_col`/`party_row`; `miles_per_hex`, `miles_per_day`; `version` |
| `map_hex_cells` | Painted hexes, sparse (an unpainted hex has no row) | PK `(map_id, col, row)`, FK→`maps` CASCADE; `terrain` (VARCHAR checked in Go), `piece`, `name`, `notes`, `explored`; `updated_by` (no FK) |

`entities.map_id` FKs `maps.id` (constraint added by maps migration 005, since
core migrations cannot reference a plugin table on a fresh DB).

### sessions (`internal/plugins/sessions/migrations/`)

| Table | Purpose | Notable columns |
|---|---|---|
| `sessions` | A scheduled game session | `calendar_id` FK→`calendars` SET NULL ; `notes`/`notes_html`, `recap`/`recap_html`; `scheduled_date`/`scheduled_time`/`scheduled_tz` (organizer's IANA zone, nullable); `deleted_at` (soft delete); `is_recurring`+`recurrence_*` |
| `session_entities` | Entities linked to a session | `UNIQUE(session_id, entity_id)`; `role` (mentioned/encountered/key) |
| `session_attendees` | Per-user RSVP status | `UNIQUE(session_id, user_id)`; `status` (invited/accepted/declined/tentative); `note` (≤140), `excluded_from_count`, `needs_recheck` |
| `session_occurrence_rsvps` | Per-night RSVPs for a repeating session (a non-recurring session uses `session_attendees`) | `UNIQUE(session_id, user_id, occurrence_date)`; `status` (default `invited`), `note`, `excluded_from_count`, `needs_recheck`, `responded_at` |
| `session_reschedule_suggestions` | A member's "suggest another time" answer from an RSVP email link | `session_id`, `user_id` FK CASCADE; `occurrence_date` set only for one night of a repeating series; `suggested_date`, `suggested_time`, `note` |
| `session_calendar_feed_tokens` | One private, replaceable calendar-feed link per campaign member | `UNIQUE(token)`, `UNIQUE(campaign_id, user_id)` |
| `session_calendar_feed_settings` | Owner kill switch for the feed, per campaign | PK `campaign_id`; `enabled` — no row means enabled |
| `session_rsvp_tokens` | Single-use email RSVP links | `token` UNIQUE; 7-day expiry |
| `member_availability` | Recurring per-member weekly free/busy blocks | zone-local wall-clock (`day_of_week`, `start_minute`, `end_minute`, `tz`) — never UTC, so DST doesn't shift it; `week_parity` (0=every week, 1/2=alternating tracks) |
| `availability_exceptions` | One-off date overrides to the recurring pattern | `on_date` + zone-local wall-clock, same shape as `member_availability` |
| `member_availability_status` | "Has this member answered at all?" (distinct from "answered and is free") | composite PK `(campaign_id, user_id)`; `answered_at` |
| `slot_proposals` | A DM's scheduling proposal (title + candidate slots) | `status` (open/closed) |
| `slot_proposal_options` | A candidate slot on a proposal | `starts_at_utc`/`ends_at_utc` — UTC instants (unlike the wall-clock availability tables); `ordinal` display order |
| `slot_proposal_responses` | Per-option per-user yes/no/maybe | `UNIQUE(option_id, user_id)`; own table, deliberately not `session_attendees`, to keep it out of campaign/export egress |
| `slot_proposal_tokens` | Single-use email response links, mirrors `session_rsvp_tokens` | `token` UNIQUE |
| `notifications` | Generic in-app notification store | `user_id`; `campaign_id` nullable; `type`/`payload`/`link`; `read_at` NULL = unread |

### timeline (`internal/plugins/timeline/migrations/`)

| Table | Purpose | Notable columns |
|---|---|---|
| `timelines` | A visual timeline (per campaign, optionally tied to a calendar) | `calendar_id` FK→`calendars` SET NULL (the create form's calendar picker lists the campaign's calendars; creation checks the calendar is in the same campaign); `visibility`/`visibility_rules`; `zoom_default` |
| `timeline_event_links` | Links a calendar event onto a timeline | FK→`calendar_events` CASCADE; `display_order`, `visibility_override`, `label`/`color_override` |
| `timeline_entity_groups` / `timeline_entity_group_members` | Named entity groupings shown on a timeline | plain parent + junction |
| `timeline_events` | Standalone timeline events (not calendar-linked) | `year`/`month`/`day` (+ `end_*`); `entity_id` FK SET NULL; `visibility`/`visibility_rules`; `is_recurring`+`recurrence_type` |
| `timeline_event_connections` | Drawn connector lines between two events/links | `source_id`/`target_id` + `source_type`/`target_type` (`event` or `link`, not FK-typed — polymorphic); `style` (solid/dashed/dotted/arrow) |

### syncapi (`internal/plugins/syncapi/migrations/`)

REST API for external tools (Foundry VTT). See `API-CONTRACT.md` in the
Foundry module repo for the wire contract.

| Table | Purpose | Notable columns |
|---|---|---|
| `api_keys` | Bearer tokens scoped to a campaign | `key_hash` (bcrypt), `key_prefix` UNIQUE (14 chars for new keys, 8 for older ones); `permissions`/`ip_allowlist` JSON; `device_fingerprint`/`device_bound_at`; `vtt_tag` (cosmetic, e.g. "foundry"); `module_version` (nullable, self-reported by the VTT module in `X-Chronicle-Module-Version`) |
| `api_request_log` | Per-request audit trail | `api_key_id`, `status_code`, `duration_ms`, indexed by key/campaign/created/ip/status |
| `sync_mappings` | Chronicle object ↔ external-tool object, bidirectional | `UNIQUE(campaign_id, chronicle_type, chronicle_id, external_system)`; `sync_version` (conflict detection) |
| `api_ip_blocklist` | Admin-managed IP blocks for the REST API | `expires_at` nullable (permanent if NULL) |
| `api_security_events` | Auth failures, IP blocks, device mismatches, rate-limit hits | `resolved`/`resolved_by`/`resolved_at` |
| `sync_changes` | The change feed Foundry reads on connect (`GET /sync/changes`) | `seq` autoincrement is the cursor; `resource_type`, `resource_id`, `op` enum(`created`,`updated`,`deleted`); rows are pruned by age |
| `sync_change_watermarks` | How far the feed was pruned, so a stale cursor is told to resync | PK `campaign_id`; `pruned_through` |
| `sync_calendar_date_beacons` | Per-campaign "date Foundry last saw / last applied" | PK `campaign_id`; `last_served_*` (a Bearer-authed GET was served) vs `applied_*` (Foundry confirmed it set its own date via `POST .../confirm`) — distinct claims, filled independently. |
| `sync_events` | Sync history: one row per thing that synced, either direction, read by Manage › Sync history and the module's History tab | `direction` enum(`to_chronicle`,`to_foundry`,`link`); `reported_by` enum(`chronicle`,`client`); `parent_id` groups a catch-up run's steps; names, ids, call and answer only, never page text; `was_value` is what a change replaced, in words (a date push's old date); pruned after 90 days |
| `foundry_players` | Players in Foundry: the GM's Foundry client's latest list of the world's users, read by the owner's Foundry page and People page | PK (`campaign_id`, `foundry_user_id`); `member_user_id` only when a campaign member; `foundry_name` ≤100, `last_failure` ≤200; each report replaces the campaign's rows; pruned 30 days after the last report |

### packages (`internal/plugins/packages/migrations/`)

| Table | Purpose | Notable columns |
|---|---|---|
| `packages` | Installed/available external packages (systems, Foundry modules) | `type` enum(`system`,`foundry-module`); `slug` UNIQUE; `auto_update` enum; `status` enum(`pending`,`approved`,`rejected`,`archived`,`deprecated`) — submission/review workflow; `last_error`/`last_error_at` (durable failure record, survives restarts) |
| `package_versions` | Version history from GitHub releases | `UNIQUE(package_id, version)`; `prerelease` flag |
| `campaign_package_updates` | One campaign's update choice for one package | `PRIMARY KEY(campaign_id, package_id)`; `update_mode` enum(`automatic`,`pinned`,`approve_first`); `version` (the version a game-system campaign stays on); `held_version` (a newly installed version awaiting approval, any package type). No row = automatic. The Foundry module keeps its mode and pin in the campaign settings, so only `held_version` is used on its rows |

### foundry_vtt (`internal/plugins/foundry_vtt/migrations/`)

| Table | Purpose | Notable columns |
|---|---|---|
| `foundry_vtt_campaign_tokens` | Per-campaign Foundry sync token/rotation state | PK `campaign_id`; `token_version`, `rotated_at` |

### widgetbindings (`internal/plugins/widgetbindings/migrations/`)

| Table | Purpose | Notable columns |
|---|---|---|
| `widget_bindings` | Generic host ↔ widget-type ↔ data-instance binding | **FK-free by design** — `host_id`/`instance_id` are polymorphic (e.g. host = an entity now, a dashboard later; instance = a calendar now, a map/timeline later), so no single FK target exists; referential integrity is enforced in the binding service, not the schema. `UNIQUE(campaign_id, host_type, host_id, widget_type)` |

### systemstate (`internal/plugins/systemstate/migrations/`)

| Table | Purpose | Notable columns |
|---|---|---|
| `entity_system_state` | Per-page JSON documents a game-system package's widget keeps (e.g. a negotiation tracker) | `PRIMARY KEY (entity_id, system_id, state_key)`; FKs to `entities`, `campaigns` (both `ON DELETE CASCADE`) and `users` (`updated_by`, `ON DELETE SET NULL`); `public_data` (any viewer of the page) and `gm_data` (DM team only) are separate JSON columns so a partial write replaces one half |

### quests (`internal/plugins/quests/migrations/`)

| Table | Purpose |
|-------|---------|
| `quests` | One JSON sheet per page (`entity_id` PK), `version` for edit conflicts |
| `quest_board_pages` | Board and ledger looks for a place page |
| `quest_board_type_looks` | Board and ledger looks for a category (entity type); FK cascade |
| `quest_boards` | Boards on a place page or a category: exactly one of `entity_id` / `entity_type_id` is set; `who` = dm/scribe/all, `sort_order` |
| `quest_board_items` | Pins: notice/note/page/map/string; `owner_user_id`, `by_dm`, `hidden`, `ref_id` (no FK) |

### armory (`internal/plugins/armory/migrations/`)

| Table | Purpose | Notable columns |
|---|---|---|
| `armory_item_shares` | Remembers which view grants on a hidden item came from a holder sharing it with the party, so un-sharing takes back only what sharing added | `PRIMARY KEY (character_id, item_entity_id, user_id)`; `made_grant` = the share added the grant; FKs to `campaigns` and `entities` (`ON DELETE CASCADE`) |

### rolltables (`internal/plugins/rolltables/migrations/`)

| Table | Purpose | Notable columns |
|---|---|---|
| `campaign_roll_tables` | One JSON document per campaign holding all its rolling tables, replaced as a unit | `campaign_id` PK, FK→`campaigns` CASCADE; `data` LONGTEXT; `updated_by` |

## MariaDB-specific notes

- **JSON columns:** MariaDB validates JSON on write. Prefer loading full JSON
  into Go and processing there over `JSON_EXTRACT()` queries.
- **UUIDs:** stored as `CHAR(36)`/`VARCHAR(36)`, generated in Go (`uuid.New()`
  or a hex-formatted `generateID()`).
- **Full-text search:** `FULLTEXT` indexes exist on `entities.name`,
  `entities.search_text`, `entity_aliases.alias`, and
  `bestiary_publications.(name, description)`. Query with
  `MATCH(...) AGAINST(? IN BOOLEAN MODE)`.
- **Timestamps:** `DATETIME`, not `TIMESTAMP` (2038 limit) — except a handful
  of `created_at`/`updated_at` columns predating that convention. Use
  `parseTime=true` in the DSN for automatic Go `time.Time` scanning.

## Migration structure (ADR-028)

Schema is split into two tiers — see `internal/database/plugin_schema.go` (the
plugin migration runner) and `internal/database/plugin_health.go` (the health
registry tracking which plugins have healthy schemas). Migrations are
append-only and immutable once any live database may have applied them (see
`.ai/conventions.md` §Migration Safety Rules) — a schema change is always a
*new* numbered file, never an edit.

- **Core** (`db/migrations/`): fatal on failure, runs first. A single
  idempotent baseline (`000001_baseline.up.sql`) plus incremental numbered
  migrations after it.
- **Plugin** (`internal/plugins/<name>/migrations/`): graceful degradation on
  failure. Each plugin owns its own numbered sequence and only its own
  tables; a plugin migration may reference core tables (core always runs
  first) but never another plugin's tables directly by FK across plugin
  boundaries in a way that would break ordering — see the calendar/timeline/
  sessions FK notes above for how that's handled when it's unavoidable.

## Open work

Rebuilding the calendar schema: #741.
