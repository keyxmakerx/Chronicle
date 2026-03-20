# Data Model

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Quick reference for the database schema. Avoids reading all      -->
<!--          migration files to understand the data model.                   -->
<!-- Update: After every migration is written or applied.                     -->
<!-- ====================================================================== -->

## Entity Relationship Overview

```
User --< CampaignMember >-- Campaign
         |                      |
         +--< Note --< NoteVersion
         |                      +--< EntityType (configurable per campaign)
         |                      +--< Entity --< EntityPost
         |                      |       |---< EntityTag >-- Tag
         |                      |       |---< EntityRelation
         |                      |       |---< EntityPermission
         |                      |       |---< CalendarEvent (via entity_id)
         |                      |       |---< MapMarker (via entity_id)
         |                      |       |---< MapToken (via entity_id)
         |                      +--< Calendar --< CalendarMonth
         |                      |       |---< CalendarWeekday
         |                      |       |---< CalendarMoon
         |                      |       |---< CalendarSeason
         |                      |       |---< CalendarEvent
         |                      |       |---< CalendarEra
         |                      |       |---< CalendarEventCategory
         |                      +--< Map --< MapMarker
         |                      |      |---< MapLayer --< MapDrawing
         |                      |      |---< MapToken
         |                      |      |---< MapFog
         |                      +--< Timeline --< TimelineEvent
         |                      |       |---< TimelineEventLink >-- CalendarEvent
         |                      |       |---< TimelineEntityGroup --< TimelineEntityGroupMember
         |                      |       |---< TimelineEventConnection
         |                      +--< Session --< SessionAttendee >-- User
         |                      |       |---< SessionEntity >-- Entity
         |                      |       |---< SessionRSVPToken
         |                      +--< CampaignGroup --< CampaignGroupMember >-- User
         |                      +--< SyncMapping
         |                      +--< AuditLog
         |                      +--< SecurityEvent (site-wide)
         |                      +--< Addon --< CampaignAddon
         |                      +--< ApiKey --< ApiRequestLog
         |                      +--< Extension --< CampaignExtension
         |                                          |---< ExtensionProvenance
         |                                          |---< ExtensionData
         +--< PasswordResetToken

(--< means "has many")
```

## Tables

> All tables below are implemented. Core tables live in `db/migrations/000001_baseline.up.sql`.
> Plugin tables live in `internal/plugins/<name>/migrations/`. See ADR-028 for the
> plugin-isolated schema architecture. Historical migration numbers (000NNN) are
> preserved in column notes for traceability but those files no longer exist.

### users (implemented -- migration 000001)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID generated in Go |
| email | VARCHAR(255) | UNIQUE, NOT NULL | |
| display_name | VARCHAR(100) | NOT NULL | |
| password_hash | VARCHAR(255) | NOT NULL | argon2id |
| avatar_path | VARCHAR(500) | NULL | Uploaded image path |
| is_admin | BOOLEAN | DEFAULT false | System-level admin |
| totp_secret | VARCHAR(255) | NULL | 2FA secret |
| totp_enabled | BOOLEAN | DEFAULT false | |
| timezone | VARCHAR(50) | NULL | IANA timezone string (added 000031) |
| pending_email | VARCHAR(255) | NULL | Email change pending verification (added 000056) |
| email_verify_token | CHAR(64) | UNIQUE, NULL | Token for email change (added 000056) |
| email_verify_expires | DATETIME | NULL | Token expiry (added 000056) |
| created_at | DATETIME | NOT NULL, DEFAULT NOW() | |
| last_login_at | DATETIME | NULL | |

### campaigns (implemented -- migrations 000002, 000005, 000006, 000021)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| name | VARCHAR(200) | NOT NULL | |
| slug | VARCHAR(200) | UNIQUE, NOT NULL | URL-safe, derived from name |
| description | TEXT | NULL | |
| settings | JSON | DEFAULT '{}' | Enabled modules, theme, etc. |
| backdrop_path | VARCHAR(500) | NULL | Campaign header image (added 000005) |
| sidebar_config | JSON | DEFAULT '{}' | Sidebar ordering/visibility (added 000006) |
| is_public | BOOLEAN | DEFAULT false | Discoverable without login |
| dashboard_layout | JSON | DEFAULT NULL | Custom dashboard (added 000021) |
| created_by | CHAR(36) | FK -> users.id | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### campaign_members (implemented -- migrations 000002, 000054)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| campaign_id | CHAR(36) | PK (composite), FK -> campaigns.id ON DELETE CASCADE | |
| user_id | CHAR(36) | PK (composite), FK -> users.id ON DELETE CASCADE | |
| role | VARCHAR(20) | NOT NULL, DEFAULT 'player', CHECK IN ('owner','scribe','player') | |
| character_entity_id | VARCHAR(36) | NULL, FK -> entities.id ON DELETE SET NULL | Assigned character (added 000054) |
| joined_at | DATETIME | NOT NULL, DEFAULT NOW() | |

### ownership_transfers (implemented -- migration 000002)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | UNIQUE, FK -> campaigns.id ON DELETE CASCADE | One pending per campaign |
| from_user_id | CHAR(36) | FK -> users.id | Current owner |
| to_user_id | CHAR(36) | FK -> users.id | Target new owner |
| token | VARCHAR(128) | UNIQUE, NOT NULL | 64-byte hex token |
| expires_at | DATETIME | NOT NULL | 72h from creation |
| created_at | DATETIME | NOT NULL, DEFAULT NOW() | |

### smtp_settings (implemented -- migration 000003)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, DEFAULT 1, CHECK (id = 1) | Singleton row |
| host | VARCHAR(255) | NOT NULL, DEFAULT '' | SMTP server host |
| port | INT | NOT NULL, DEFAULT 587 | SMTP port |
| username | VARCHAR(255) | NOT NULL, DEFAULT '' | SMTP username |
| password_encrypted | VARBINARY(512) | NULL | AES-256-GCM encrypted |
| from_address | VARCHAR(255) | NOT NULL, DEFAULT '' | Sender email |
| from_name | VARCHAR(100) | NOT NULL, DEFAULT 'Chronicle' | Sender display name |
| encryption | VARCHAR(20) | NOT NULL, DEFAULT 'starttls' | 'starttls', 'ssl', 'none' |
| enabled | BOOLEAN | NOT NULL, DEFAULT FALSE | |
| updated_at | DATETIME | NOT NULL, DEFAULT NOW() ON UPDATE | |

### entity_types (implemented -- migrations 000004, 000007, 000013, 000021)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| campaign_id | CHAR(36) | FK -> campaigns.id | |
| slug | VARCHAR(100) | NOT NULL | 'character', 'location' |
| name | VARCHAR(100) | NOT NULL | Display name |
| name_plural | VARCHAR(100) | NOT NULL | 'Characters', 'Locations' |
| icon | VARCHAR(50) | DEFAULT 'fa-file' | FA or RPG Awesome class |
| color | VARCHAR(7) | DEFAULT '#6b7280' | Hex color for badges |
| fields | JSON | DEFAULT '[]' | Field definitions array |
| layout_json | JSON | DEFAULT '{"sections":[]}' | Profile page layout (added 000007) |
| description | TEXT | NULL | Category description (added 000013) |
| pinned_entity_ids | JSON | DEFAULT '[]' | Pinned pages (added 000013) |
| dashboard_layout | JSON | DEFAULT NULL | Custom category dashboard (added 000021) |
| sort_order | INT | DEFAULT 0 | Sidebar order |
| is_default | BOOLEAN | DEFAULT false | Ships pre-configured |
| enabled | BOOLEAN | DEFAULT true | |
| UNIQUE(campaign_id, slug) | | | |

### entities (implemented -- migrations 000004, 000014, 000016, 000023, 000048)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id, NOT NULL | |
| entity_type_id | INT | FK -> entity_types.id, NOT NULL | |
| name | VARCHAR(200) | NOT NULL | |
| slug | VARCHAR(200) | NOT NULL | |
| entry | JSON | NULL | TipTap/ProseMirror JSON doc |
| entry_html | LONGTEXT | NULL | Pre-rendered HTML |
| player_notes | JSON | NULL | Player-facing ProseMirror JSON (migration 000016) |
| player_notes_html | TEXT | NULL | Pre-rendered HTML from player_notes (migration 000016) |
| image_path | VARCHAR(500) | NULL | Header image |
| parent_id | CHAR(36) | FK -> entities.id, NULL | Nesting |
| type_label | VARCHAR(100) | NULL | Freeform subtype ("City") |
| is_private | BOOLEAN | DEFAULT false | GM-only (legacy, see visibility) |
| is_template | BOOLEAN | DEFAULT false | |
| fields_data | JSON | DEFAULT '{}' | Type-specific field values |
| field_overrides | JSON | DEFAULT NULL | Per-entity field customization (added 000014) |
| popup_config | JSON | DEFAULT NULL | Hover preview toggle config (added 000023) |
| visibility | ENUM('default','custom') | DEFAULT 'default' | Permission mode (added 000048) |
| created_by | CHAR(36) | FK -> users.id | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |
| UNIQUE(campaign_id, slug) | | | |
| FULLTEXT(name) | | | For search |

### media_files (implemented -- migration 000005)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | NULL, FK -> campaigns.id ON DELETE SET NULL | |
| uploaded_by | CHAR(36) | NOT NULL, FK -> users.id ON DELETE CASCADE | |
| filename | VARCHAR(500) | NOT NULL | UUID-based stored filename |
| original_name | VARCHAR(500) | NOT NULL | User's original filename |
| mime_type | VARCHAR(100) | NOT NULL | Validated MIME type |
| file_size | BIGINT | NOT NULL | Size in bytes |
| usage_type | VARCHAR(50) | DEFAULT 'attachment' | 'attachment', 'avatar', etc. |
| thumbnail_paths | JSON | NULL | Generated thumbnail paths |
| created_at | TIMESTAMP | NOT NULL, DEFAULT NOW() | |

### entity_posts (implemented -- migration 000050)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| entity_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| campaign_id | CHAR(36) | FK -> campaigns.id ON DELETE CASCADE | |
| name | VARCHAR(200) | NOT NULL | |
| entry | JSON | NULL | TipTap JSON |
| entry_html | LONGTEXT | NULL | Pre-rendered |
| is_private | BOOLEAN | DEFAULT false | |
| sort_order | INT | DEFAULT 0 | |
| created_by | CHAR(36) | FK -> users.id | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### tags (implemented -- migrations 000009, 000038)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id | |
| name | VARCHAR(100) | NOT NULL | |
| slug | VARCHAR(100) | NOT NULL | |
| color | VARCHAR(7) | DEFAULT '#6b7280' | |
| parent_id | CHAR(36) | FK -> tags.id, NULL | Nested tags |
| dm_only | BOOLEAN | DEFAULT false | GM-only visibility (added 000038) |
| UNIQUE(campaign_id, slug) | | | |

### entity_tags (implemented)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| entity_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| tag_id | CHAR(36) | FK -> tags.id ON DELETE CASCADE | |
| PRIMARY KEY (entity_id, tag_id) | | | |

### entity_relations (implemented -- migrations 000010, 000046, 000052)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| source_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| target_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| type | VARCHAR(100) | NOT NULL | 'ally', 'enemy', 'parent', etc. |
| reverse_type | VARCHAR(100) | NULL | Auto-created reverse label |
| metadata | JSON | NULL | Relation-specific data (added 000046) |
| dm_only | BOOLEAN | DEFAULT false | GM-only visibility (added 000052) |
| created_at | DATETIME | NOT NULL | |

### audit_log (implemented)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id ON DELETE CASCADE | |
| user_id | CHAR(36) | FK -> users.id | Actor |
| action | VARCHAR(50) | NOT NULL | 'create', 'update', 'delete' |
| entity_type | VARCHAR(50) | NOT NULL | 'entity', 'campaign', etc. |
| entity_id | VARCHAR(36) | NULL | Target ID |
| entity_name | VARCHAR(200) | NULL | Target name (for display) |
| details | JSON | NULL | Extra context |
| created_at | DATETIME | NOT NULL | |

### addons (implemented -- migrations 000015, 000027)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| slug | VARCHAR(100) | UNIQUE, NOT NULL | URL-safe identifier |
| name | VARCHAR(200) | NOT NULL | Display name |
| description | TEXT | NULL | |
| version | VARCHAR(50) | DEFAULT '0.1.0' | |
| category | ENUM | NOT NULL | 'module', 'widget', 'integration', 'plugin' (added in 000027) |
| status | ENUM | NOT NULL, DEFAULT 'planned' | 'active', 'planned', 'deprecated' |
| icon | VARCHAR(100) | DEFAULT 'fa-puzzle-piece' | Font Awesome icon |
| author | VARCHAR(200) | NULL | Creator/maintainer |
| config_schema | JSON | NULL | Optional JSON schema for addon config |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### campaign_addons (implemented -- migration 000015)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| campaign_id | CHAR(36) | FK -> campaigns.id ON DELETE CASCADE | |
| addon_id | INT | FK -> addons.id ON DELETE CASCADE | |
| enabled | BOOLEAN | DEFAULT true | |
| config_json | JSON | NULL | Per-campaign addon config |
| enabled_at | TIMESTAMP | DEFAULT NOW() | |
| enabled_by | CHAR(36) | NULL | User who enabled it |
| UNIQUE(campaign_id, addon_id) | | | |

### api_keys (implemented -- migration 000016)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id ON DELETE CASCADE | |
| user_id | CHAR(36) | FK -> users.id ON DELETE CASCADE | Key owner |
| name | VARCHAR(200) | NOT NULL | Display name |
| key_prefix | VARCHAR(8) | NOT NULL | First 8 chars (for identification) |
| key_hash | VARCHAR(255) | NOT NULL | bcrypt hash of full key |
| permissions | JSON | NOT NULL | ['read', 'write', 'sync'] |
| ip_allowlist | JSON | NULL | Optional IP whitelist |
| rate_limit | INT | DEFAULT 60 | Requests per minute |
| is_active | BOOLEAN | DEFAULT true | |
| expires_at | DATETIME | NULL | Optional expiry |
| last_used_at | DATETIME | NULL | |
| created_at | DATETIME | NOT NULL | |

### notes (implemented -- migrations 000017, 000022, 000051)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id ON DELETE CASCADE, NOT NULL | |
| user_id | CHAR(36) | FK -> users.id ON DELETE CASCADE, NOT NULL | Note creator |
| entity_id | CHAR(36) | NULL | NULL = campaign-wide note |
| parent_id | CHAR(36) | FK -> notes.id, NULL | Folder nesting (added 000051) |
| is_folder | BOOLEAN | DEFAULT false | Folder vs note (added 000051) |
| title | VARCHAR(200) | NOT NULL, DEFAULT '' | |
| content | JSON | NOT NULL | Block array [{type, value/items}] |
| entry | JSON | DEFAULT NULL | ProseMirror JSON (added 000022) |
| entry_html | TEXT | DEFAULT NULL | Pre-rendered HTML (added 000022) |
| color | VARCHAR(7) | DEFAULT '#374151' | Accent color |
| pinned | BOOLEAN | DEFAULT false | |
| is_shared | BOOLEAN | DEFAULT false | Visible to campaign members (added 000022) |
| last_edited_by | CHAR(36) | DEFAULT NULL | Last user who saved (added 000022) |
| locked_by | CHAR(36) | DEFAULT NULL | Current lock holder (added 000022) |
| locked_at | DATETIME | DEFAULT NULL | Lock acquisition time (added 000022) |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |
| INDEX idx_notes_locked (locked_by, locked_at) | | | |
| INDEX idx_notes_shared (campaign_id, is_shared) | | | |

### note_versions (implemented -- migration 000022)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| note_id | CHAR(36) | FK -> notes.id ON DELETE CASCADE, NOT NULL | |
| user_id | CHAR(36) | NOT NULL | User who triggered the save |
| title | VARCHAR(200) | NOT NULL, DEFAULT '' | Snapshot of title |
| content | JSON | NOT NULL | Snapshot of block content |
| entry | JSON | DEFAULT NULL | Snapshot of ProseMirror JSON |
| entry_html | TEXT | DEFAULT NULL | Snapshot of rendered HTML |
| created_at | DATETIME | NOT NULL, DEFAULT NOW() | |
| INDEX idx_note_versions_note (note_id, created_at DESC) | | | |

### password_reset_tokens (implemented -- migration 000020)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| user_id | CHAR(36) | FK -> users.id ON DELETE CASCADE | |
| token_hash | VARCHAR(64) | UNIQUE, NOT NULL | SHA-256 hash |
| expires_at | DATETIME | NOT NULL | 1 hour from creation |
| used_at | DATETIME | NULL | Single-use |
| created_at | DATETIME | NOT NULL | |

### storage_settings (implemented)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, DEFAULT 1, CHECK (id = 1) | Singleton row |
| max_upload_size | BIGINT | DEFAULT 10485760 | Per-file limit (bytes) |
| max_total_storage | BIGINT | DEFAULT 1073741824 | Per-campaign limit (bytes) |
| allowed_types | JSON | NOT NULL | Allowed MIME types |
| updated_at | DATETIME | NOT NULL | |

### security_events (implemented -- migration 000024)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | BIGINT | PK, AUTO_INCREMENT | |
| event_type | VARCHAR(50) | NOT NULL | login.success, login.failed, admin.*, etc. |
| user_id | CHAR(36) | NULL | NULL for failed logins with unknown email |
| actor_id | CHAR(36) | NULL | Admin who initiated action |
| ip_address | VARCHAR(45) | NOT NULL, DEFAULT '' | Client IP |
| user_agent | TEXT | NULL | |
| details | JSON | NULL | Flexible metadata |
| created_at | DATETIME | NOT NULL | |

### calendars (implemented -- migrations 000027, 000028, 000030, 000031)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| campaign_id | VARCHAR(36) | UNIQUE, FK -> campaigns.id ON DELETE CASCADE | One per campaign |
| mode | VARCHAR(20) | NOT NULL, DEFAULT 'fantasy' | 'fantasy' or 'reallife' (added 000031) |
| name | VARCHAR(255) | NOT NULL, DEFAULT 'Campaign Calendar' | |
| description | TEXT | NULL | |
| epoch_name | VARCHAR(100) | NULL | e.g., "Third Age" |
| current_year | INT | NOT NULL, DEFAULT 1 | In-game year |
| current_month | INT | NOT NULL, DEFAULT 1 | In-game month |
| current_day | INT | NOT NULL, DEFAULT 1 | In-game day |
| hours_per_day | INT | NOT NULL, DEFAULT 24 | Configurable time system (added 000030) |
| minutes_per_hour | INT | NOT NULL, DEFAULT 60 | Configurable time system (added 000030) |
| seconds_per_minute | INT | NOT NULL, DEFAULT 60 | Configurable time system (added 000030) |
| current_hour | INT | NOT NULL, DEFAULT 0 | In-game hour (added 000030) |
| current_minute | INT | NOT NULL, DEFAULT 0 | In-game minute (added 000030) |
| leap_year_every | INT | NOT NULL, DEFAULT 0 | 0 = no leap years (added 000028) |
| leap_year_offset | INT | NOT NULL, DEFAULT 0 | Offset for calculation (added 000028) |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### calendar_months (implemented -- migrations 000027, 000028)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| calendar_id | VARCHAR(36) | FK -> calendars.id ON DELETE CASCADE | |
| name | VARCHAR(100) | NOT NULL | e.g., "Winterveil" |
| days | INT | NOT NULL, DEFAULT 30 | |
| sort_order | INT | NOT NULL, DEFAULT 0 | |
| is_intercalary | TINYINT(1) | NOT NULL, DEFAULT 0 | Festival/leap month |
| leap_year_days | INT | NOT NULL, DEFAULT 0 | Extra days in leap years (added 000028) |

### calendar_weekdays (implemented -- migration 000027)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| calendar_id | VARCHAR(36) | FK -> calendars.id ON DELETE CASCADE | |
| name | VARCHAR(100) | NOT NULL | |
| sort_order | INT | NOT NULL, DEFAULT 0 | |

### calendar_moons (implemented -- migration 000027)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| calendar_id | VARCHAR(36) | FK -> calendars.id ON DELETE CASCADE | |
| name | VARCHAR(100) | NOT NULL | |
| cycle_days | FLOAT | NOT NULL, DEFAULT 29.5 | Lunar cycle length |
| phase_offset | FLOAT | NOT NULL, DEFAULT 0 | Phase offset |
| color | VARCHAR(7) | NOT NULL, DEFAULT '#c0c0c0' | |

### calendar_seasons (implemented -- migrations 000027, 000028)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| calendar_id | VARCHAR(36) | FK -> calendars.id ON DELETE CASCADE | |
| name | VARCHAR(100) | NOT NULL | |
| start_month | INT | NOT NULL | |
| start_day | INT | NOT NULL | |
| end_month | INT | NOT NULL | |
| end_day | INT | NOT NULL | |
| description | TEXT | NULL | |
| color | VARCHAR(7) | NOT NULL, DEFAULT '#6b7280' | Visual indicator (added 000028) |
| weather_effect | VARCHAR(200) | NULL | Weather description for season (added 000033) |

### calendar_eras (implemented -- migration 000033)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| calendar_id | VARCHAR(36) | FK -> calendars.id ON DELETE CASCADE | |
| name | VARCHAR(200) | NOT NULL | Era name (e.g. "First Age") |
| start_year | INT | NOT NULL | First year of era |
| end_year | INT | NULL | Last year of era (NULL = ongoing) |
| description | TEXT | NULL | |
| color | VARCHAR(20) | NOT NULL, DEFAULT '#6366f1' | |
| sort_order | INT | NOT NULL, DEFAULT 0 | |

### calendar_events (implemented -- migrations 000027, 000028, 000030, 000034, 000037)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| calendar_id | VARCHAR(36) | FK -> calendars.id ON DELETE CASCADE | |
| entity_id | VARCHAR(36) | NULL, FK -> entities.id ON DELETE SET NULL | |
| name | VARCHAR(255) | NOT NULL | |
| description | TEXT | NULL | ProseMirror JSON (rich text) or plain text (legacy) |
| description_html | TEXT | NULL | Pre-rendered sanitized HTML (added 000034) |
| year | INT | NOT NULL | |
| month | INT | NOT NULL | |
| day | INT | NOT NULL | |
| start_hour | INT | NULL | Event start hour (added 000030) |
| start_minute | INT | NULL | Event start minute (added 000030) |
| end_year | INT | NULL | Multi-day event end (added 000028) |
| end_month | INT | NULL | (added 000028) |
| end_day | INT | NULL | (added 000028) |
| end_hour | INT | NULL | Event end hour (added 000030) |
| end_minute | INT | NULL | Event end minute (added 000030) |
| is_recurring | TINYINT(1) | NOT NULL, DEFAULT 0 | |
| recurrence_type | VARCHAR(20) | NULL | yearly, monthly |
| visibility | VARCHAR(20) | NOT NULL, DEFAULT 'everyone' | |
| visibility_rules | JSON | NULL | Fine-grained visibility rules (added 000037) |
| category | VARCHAR(50) | NULL | holiday, battle, quest, etc. (added 000028) |
| created_by | VARCHAR(36) | NULL | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### maps (implemented -- migrations 000029, 000045)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| campaign_id | VARCHAR(36) | FK -> campaigns.id ON DELETE CASCADE | |
| name | VARCHAR(255) | NOT NULL | |
| description | TEXT | NULL | |
| image_id | VARCHAR(36) | NULL, FK -> media_files.id ON DELETE SET NULL | Background image |
| image_width | INT | NOT NULL, DEFAULT 0 | |
| image_height | INT | NOT NULL, DEFAULT 0 | |
| grid_type | VARCHAR(20) | NULL | Grid overlay type (added 000045) |
| grid_size | INT | NULL | Grid cell size (added 000045) |
| grid_color | VARCHAR(7) | NULL | Grid line color (added 000045) |
| grid_opacity | FLOAT | NULL | Grid opacity (added 000045) |
| background_color | VARCHAR(7) | NULL | Canvas background (added 000045) |
| fog_exploration | BOOLEAN | DEFAULT false | Enable fog of war (added 000045) |
| initial_view_x | DOUBLE | NULL | Default viewport X (added 000045) |
| initial_view_y | DOUBLE | NULL | Default viewport Y (added 000045) |
| initial_zoom | INT | NULL | Default zoom level (added 000045) |
| foundry_scene_id | VARCHAR(100) | NULL | Foundry VTT scene ID (added 000045) |
| sort_order | INT | NOT NULL, DEFAULT 0 | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### map_markers (implemented -- migrations 000029, plugin 003)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| map_id | VARCHAR(36) | FK -> maps.id ON DELETE CASCADE | |
| name | VARCHAR(255) | NOT NULL | |
| description | TEXT | NULL | |
| x | DOUBLE | NOT NULL, DEFAULT 50 | Percentage 0-100 |
| y | DOUBLE | NOT NULL, DEFAULT 50 | Percentage 0-100 |
| icon | VARCHAR(100) | NOT NULL, DEFAULT 'fa-map-pin' | FA icon class |
| color | VARCHAR(7) | NOT NULL, DEFAULT '#3b82f6' | Hex color |
| pin_category | VARCHAR(50) | NULL | location, danger, treasure, quest, note (plugin migration 003) |
| entity_id | VARCHAR(36) | NULL, FK -> entities.id ON DELETE SET NULL | |
| visibility | VARCHAR(20) | NOT NULL, DEFAULT 'everyone' | everyone or dm_only |
| visibility_rules | JSON | NULL | Per-user overrides: {allowed_users, denied_users} |
| created_by | VARCHAR(36) | NULL | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### sessions (implemented -- migrations 000032, 000041, 000053)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| campaign_id | VARCHAR(36) | FK -> campaigns.id ON DELETE CASCADE | |
| name | VARCHAR(200) | NOT NULL | Session title |
| summary | TEXT | NULL | Brief description |
| notes | JSON | NULL | ProseMirror JSON (GM notes) |
| notes_html | TEXT | NULL | Pre-rendered HTML |
| recap | TEXT | NULL | Session recap (added 000053) |
| recap_html | TEXT | NULL | Pre-rendered recap HTML (added 000053) |
| scheduled_date | DATE | NULL | Real-world date |
| calendar_year | INT | NULL | In-game year |
| calendar_month | INT | NULL | In-game month |
| calendar_day | INT | NULL | In-game day |
| is_recurring | TINYINT(1) | DEFAULT 0 | Recurring session (added 000041) |
| recurrence_type | VARCHAR(20) | NULL | weekly, biweekly, monthly (added 000041) |
| recurrence_interval | INT | NULL | Interval between recurrences (added 000041) |
| recurrence_day_of_week | INT | NULL | Day of week for recurrence (added 000041) |
| recurrence_end_date | DATE | NULL | End date for recurrence (added 000041) |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'planned' | planned, completed, cancelled |
| sort_order | INT | NOT NULL, DEFAULT 0 | Manual ordering |
| created_by | VARCHAR(36) | FK -> users.id | Session creator |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### session_attendees (implemented -- migration 000032)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| session_id | VARCHAR(36) | FK -> sessions.id ON DELETE CASCADE | |
| user_id | VARCHAR(36) | FK -> users.id ON DELETE CASCADE | |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'invited' | invited, accepted, declined, tentative |
| responded_at | DATETIME | NULL | When user last RSVPed |
| UNIQUE(session_id, user_id) | | | |

### session_entities (implemented -- migration 000032)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| session_id | VARCHAR(36) | FK -> sessions.id ON DELETE CASCADE | |
| entity_id | VARCHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| role | VARCHAR(50) | NOT NULL, DEFAULT 'mentioned' | mentioned, encountered, key |
| UNIQUE(session_id, entity_id) | | | |

### timelines (implemented -- migrations 000035, 000036)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| campaign_id | VARCHAR(36) | FK -> campaigns.id ON DELETE CASCADE | |
| calendar_id | VARCHAR(36) | NULL, FK -> calendars.id ON DELETE SET NULL | Optional calendar link |
| name | VARCHAR(255) | NOT NULL | |
| description | TEXT | NULL | |
| description_html | TEXT | NULL | Pre-rendered HTML |
| color | VARCHAR(20) | NULL | Theme color |
| icon | VARCHAR(100) | NULL | FA icon |
| visibility | VARCHAR(20) | DEFAULT 'everyone' | |
| visibility_rules | JSON | NULL | Fine-grained rules |
| zoom_default | INT | NULL | Default zoom level |
| sort_order | INT | DEFAULT 0 | |
| created_by | VARCHAR(36) | NULL | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### timeline_events (implemented -- migration 000036)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| timeline_id | VARCHAR(36) | FK -> timelines.id ON DELETE CASCADE | |
| entity_id | VARCHAR(36) | NULL, FK -> entities.id ON DELETE SET NULL | |
| name | VARCHAR(255) | NOT NULL | |
| description | TEXT | NULL | |
| description_html | TEXT | NULL | |
| year | INT | NOT NULL | |
| month | INT | NULL | |
| day | INT | NULL | |
| start_hour | INT | NULL | |
| start_minute | INT | NULL | |
| end_year | INT | NULL | |
| end_month | INT | NULL | |
| end_day | INT | NULL | |
| end_hour | INT | NULL | |
| end_minute | INT | NULL | |
| is_recurring | BOOLEAN | DEFAULT false | |
| recurrence_type | VARCHAR(20) | NULL | |
| category | VARCHAR(50) | NULL | |
| visibility | VARCHAR(20) | DEFAULT 'everyone' | |
| visibility_rules | JSON | NULL | Added by 000037 |
| display_order | INT | DEFAULT 0 | |
| label | VARCHAR(100) | NULL | |
| color | VARCHAR(20) | NULL | |
| created_by | VARCHAR(36) | NULL | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### timeline_event_links (implemented -- migration 000035)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| timeline_id | VARCHAR(36) | FK -> timelines.id ON DELETE CASCADE | |
| event_id | VARCHAR(36) | FK -> calendar_events.id ON DELETE CASCADE | |
| display_order | INT | DEFAULT 0 | |
| visibility_override | VARCHAR(20) | NULL | |
| visibility_rules | JSON | NULL | |
| label | VARCHAR(100) | NULL | |
| color_override | VARCHAR(20) | NULL | |
| created_at | DATETIME | NOT NULL | |

### timeline_entity_groups (implemented -- migration 000035)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| timeline_id | VARCHAR(36) | FK -> timelines.id ON DELETE CASCADE | |
| name | VARCHAR(255) | NOT NULL | |
| color | VARCHAR(20) | NULL | |
| sort_order | INT | DEFAULT 0 | |

### timeline_entity_group_members (implemented -- migration 000035)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| group_id | INT | FK -> timeline_entity_groups.id ON DELETE CASCADE | |
| entity_id | VARCHAR(36) | FK -> entities.id ON DELETE CASCADE | |

### timeline_event_connections (implemented -- migration 000047)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| timeline_id | VARCHAR(36) | FK -> timelines.id ON DELETE CASCADE | |
| source_id | VARCHAR(36) | NOT NULL | Source event/link ID |
| target_id | VARCHAR(36) | NOT NULL | Target event/link ID |
| source_type | VARCHAR(20) | NOT NULL | 'event' or 'link' |
| target_type | VARCHAR(20) | NOT NULL | 'event' or 'link' |
| label | VARCHAR(255) | NULL | |
| color | VARCHAR(20) | NULL | |
| style | VARCHAR(20) | DEFAULT 'solid' | solid, dashed, dotted, arrow |
| created_at | DATETIME | NOT NULL | |
| UNIQUE(timeline_id, source_id, target_id) | | | |

### calendar_event_categories (implemented -- migration 000039)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| calendar_id | VARCHAR(36) | FK -> calendars.id ON DELETE CASCADE | |
| slug | VARCHAR(100) | NOT NULL | |
| name | VARCHAR(255) | NOT NULL | |
| icon | VARCHAR(100) | NULL | |
| color | VARCHAR(20) | NULL | |
| sort_order | INT | DEFAULT 0 | |
| UNIQUE(calendar_id, slug) | | | |

### session_rsvp_tokens (implemented -- migration 000041)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| token | VARCHAR(64) | NOT NULL | Single-use RSVP token |
| session_id | VARCHAR(36) | FK -> sessions.id ON DELETE CASCADE | |
| user_id | VARCHAR(36) | FK -> users.id ON DELETE CASCADE | |
| action | VARCHAR(20) | NOT NULL | accept, decline, tentative |
| used_at | DATETIME | NULL | |
| expires_at | DATETIME | NOT NULL | 7-day expiry |
| created_at | DATETIME | NOT NULL | |

### sync_mappings (implemented -- migration 000044)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| campaign_id | VARCHAR(36) | FK -> campaigns.id ON DELETE CASCADE | |
| chronicle_type | VARCHAR(50) | NOT NULL | entity, calendar_event, map, etc. |
| chronicle_id | VARCHAR(36) | NOT NULL | Chronicle object ID |
| external_system | VARCHAR(50) | NOT NULL | 'foundry' |
| external_id | VARCHAR(255) | NOT NULL | External system ID |
| sync_version | INT | DEFAULT 0 | Conflict detection counter |
| last_synced_at | DATETIME | NULL | |
| sync_direction | VARCHAR(20) | DEFAULT 'bidirectional' | |
| sync_metadata | JSON | NULL | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |
| UNIQUE(campaign_id, chronicle_type, chronicle_id, external_system) | | | |

### map_layers (implemented -- migration 000045)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| map_id | VARCHAR(36) | FK -> maps.id ON DELETE CASCADE | |
| name | VARCHAR(255) | NOT NULL | |
| layer_type | VARCHAR(50) | NOT NULL | drawing, token, fog |
| sort_order | INT | DEFAULT 0 | |
| is_visible | BOOLEAN | DEFAULT true | |
| opacity | FLOAT | DEFAULT 1.0 | |
| is_locked | BOOLEAN | DEFAULT false | |
| created_at | DATETIME | NOT NULL | |

### map_drawings (implemented -- migration 000045)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| map_id | VARCHAR(36) | FK -> maps.id ON DELETE CASCADE | |
| layer_id | VARCHAR(36) | FK -> map_layers.id ON DELETE CASCADE | |
| drawing_type | VARCHAR(50) | NOT NULL | freehand, polygon, circle, rect, text |
| points | JSON | NOT NULL | Coordinate array |
| stroke_color | VARCHAR(7) | DEFAULT '#000000' | |
| stroke_width | INT | DEFAULT 2 | |
| fill_color | VARCHAR(7) | NULL | |
| fill_alpha | FLOAT | DEFAULT 0.5 | |
| text_content | TEXT | NULL | For text drawings |
| font_size | INT | NULL | |
| rotation | FLOAT | DEFAULT 0 | |
| visibility | VARCHAR(20) | DEFAULT 'everyone' | |
| created_by | VARCHAR(36) | NULL | |
| foundry_id | VARCHAR(100) | NULL | Foundry VTT sync ID |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### map_tokens (implemented -- migration 000045)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| map_id | VARCHAR(36) | FK -> maps.id ON DELETE CASCADE | |
| layer_id | VARCHAR(36) | FK -> map_layers.id ON DELETE SET NULL | |
| entity_id | VARCHAR(36) | NULL, FK -> entities.id ON DELETE SET NULL | |
| name | VARCHAR(255) | NOT NULL | |
| image_path | VARCHAR(500) | NULL | |
| x | DOUBLE | NOT NULL | Percentage 0-100 |
| y | DOUBLE | NOT NULL | Percentage 0-100 |
| width | INT | DEFAULT 1 | Grid cells |
| height | INT | DEFAULT 1 | Grid cells |
| rotation | FLOAT | DEFAULT 0 | |
| scale | FLOAT | DEFAULT 1.0 | |
| is_hidden | BOOLEAN | DEFAULT false | |
| is_locked | BOOLEAN | DEFAULT false | |
| vision_enabled | BOOLEAN | DEFAULT false | |
| vision_range | INT | NULL | |
| elevation | INT | DEFAULT 0 | |
| sort_order | INT | DEFAULT 0 | |
| status_effects | JSON | NULL | |
| flags | JSON | NULL | |
| foundry_id | VARCHAR(100) | NULL | Foundry VTT sync ID |
| created_by | VARCHAR(36) | NULL | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### map_fog (implemented -- migration 000045)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | VARCHAR(36) | PK | UUID |
| map_id | VARCHAR(36) | FK -> maps.id ON DELETE CASCADE | |
| points | JSON | NOT NULL | Polygon coordinates |
| is_explored | BOOLEAN | DEFAULT false | |
| created_at | DATETIME | NOT NULL | |

### entity_permissions (implemented -- migration 000048)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| entity_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| subject_type | ENUM('role','user','group') | NOT NULL | Permission target type (group added 000049) |
| subject_id | VARCHAR(36) | NOT NULL | Role name, user ID, or group ID |
| permission | ENUM('view','edit') | NOT NULL | |
| created_at | DATETIME | NOT NULL | |
| UNIQUE(entity_id, subject_type, subject_id) | | | |

### campaign_groups (implemented -- migration 000049)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id ON DELETE CASCADE | |
| name | VARCHAR(200) | NOT NULL | |
| description | TEXT | NULL | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |
| UNIQUE(campaign_id, name) | | | |

### campaign_group_members (implemented -- migration 000049)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| group_id | CHAR(36) | PK (composite), FK -> campaign_groups.id ON DELETE CASCADE | |
| user_id | CHAR(36) | PK (composite), FK -> users.id ON DELETE CASCADE | |
| joined_at | DATETIME | NOT NULL, DEFAULT NOW() | |

### extensions (implemented -- migration 000055)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| ext_id | VARCHAR(200) | UNIQUE, NOT NULL | Extension identifier |
| name | VARCHAR(200) | NOT NULL | |
| version | VARCHAR(50) | NOT NULL | |
| description | TEXT | NULL | |
| manifest | JSON | NOT NULL | Full manifest |
| installed_by | CHAR(36) | NULL | |
| status | VARCHAR(20) | DEFAULT 'active' | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### campaign_extensions (implemented -- migration 000055)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| campaign_id | CHAR(36) | PK (composite), FK -> campaigns.id ON DELETE CASCADE | |
| extension_id | INT | PK (composite), FK -> extensions.id ON DELETE CASCADE | |
| enabled | BOOLEAN | DEFAULT true | |
| applied_contents | JSON | NULL | Tracks what was applied |
| enabled_at | DATETIME | NOT NULL | |
| enabled_by | CHAR(36) | NULL | |

### extension_provenance (implemented -- migration 000055)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| campaign_id | CHAR(36) | NOT NULL | |
| extension_id | INT | NOT NULL | |
| table_name | VARCHAR(100) | NOT NULL | Target table |
| record_id | VARCHAR(36) | NOT NULL | Created record ID |
| record_type | VARCHAR(50) | NULL | Record subtype |
| created_at | DATETIME | NOT NULL | |

### extension_data (implemented -- migration 000055)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INT | PK, AUTO_INCREMENT | |
| campaign_id | CHAR(36) | NOT NULL | |
| extension_id | INT | NOT NULL | |
| namespace | VARCHAR(100) | NOT NULL | Plugin/scope identifier |
| data_key | VARCHAR(200) | NOT NULL | |
| data_value | JSON | NULL | |
| UNIQUE(campaign_id, extension_id, namespace, data_key) | | | |

### user_storage_limits (implemented -- migrations 000012, 000040)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| user_id | CHAR(36) | PK, FK -> users.id ON DELETE CASCADE | |
| max_upload_size | BIGINT | NULL | Override (NULL = use global) |
| max_total_storage | BIGINT | NULL | Override (NULL = use global) |
| bypass_max_upload | BIGINT | NULL | Temporary bypass limit (added 000040) |
| bypass_expires_at | TIMESTAMP | NULL | Bypass expiry (added 000040) |
| bypass_reason | VARCHAR(255) | NULL | Admin note (added 000040) |
| bypass_granted_by | CHAR(36) | NULL | Admin who granted bypass (added 000040) |

### campaign_storage_limits (implemented -- migrations 000012, 000040)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| campaign_id | CHAR(36) | PK, FK -> campaigns.id ON DELETE CASCADE | |
| max_upload_size | BIGINT | NULL | Override |
| max_total_storage | BIGINT | NULL | Override |
| bypass_max_storage | BIGINT | NULL | Temporary bypass limit (added 000040) |
| bypass_max_files | INT | NULL | Temporary file count bypass (added 000040) |
| bypass_expires_at | TIMESTAMP | NULL | Bypass expiry (added 000040) |
| bypass_reason | VARCHAR(255) | NULL | Admin note (added 000040) |
| bypass_granted_by | CHAR(36) | NULL | Admin who granted bypass (added 000040) |

## MariaDB-Specific Notes

- **JSON columns:** MariaDB validates JSON on write. Use `JSON_EXTRACT()` for
  queries, but prefer loading full JSON into Go and processing there.
- **UUIDs:** Stored as CHAR(36). Generated in Go with `uuid.New()` or custom
  `generateID()` (hex-formatted random bytes).
- **Full-text search:** Use `FULLTEXT` index on `entities.name`. Query with
  `MATCH(name) AGAINST(? IN BOOLEAN MODE)`.
- **Timestamps:** Use `DATETIME` (not TIMESTAMP which has 2038 limit). Use
  `parseTime=true` in DSN for automatic Go time.Time scanning.

## Indexes

- `users`: UNIQUE on email, UNIQUE on email_verify_token
- `campaigns`: INDEX on created_by, UNIQUE on slug
- `entity_types`: UNIQUE on (campaign_id, slug)
- `entities`: INDEX on (campaign_id, entity_type_id), UNIQUE on (campaign_id, slug), FULLTEXT on name
- `tags`: UNIQUE on (campaign_id, slug), INDEX on (campaign_id, dm_only)
- `notes`: INDEX on (locked_by, locked_at), INDEX on (campaign_id, is_shared), INDEX on parent_id
- `note_versions`: INDEX on (note_id, created_at DESC)
- `security_events`: INDEX on (event_type, created_at DESC), (user_id, created_at DESC), (ip_address, created_at DESC), (created_at DESC), (actor_id, created_at DESC)
- `calendars`: UNIQUE on campaign_id
- `calendar_months`: INDEX on (calendar_id, sort_order)
- `calendar_weekdays`: INDEX on (calendar_id, sort_order)
- `calendar_moons`: INDEX on calendar_id
- `calendar_seasons`: INDEX on calendar_id
- `calendar_eras`: INDEX on calendar_id
- `calendar_events`: INDEX on (calendar_id, year, month, day), INDEX on entity_id
- `calendar_event_categories`: UNIQUE on (calendar_id, slug)
- `maps`: INDEX on (campaign_id, sort_order)
- `map_markers`: INDEX on map_id, INDEX on entity_id
- `sessions`: INDEX on (campaign_id, status), INDEX on campaign_id
- `session_attendees`: UNIQUE on (session_id, user_id), INDEX on session_id
- `session_entities`: UNIQUE on (session_id, entity_id), INDEX on session_id, INDEX on entity_id
- `session_rsvp_tokens`: INDEX on token, INDEX on (session_id, user_id)
- `sync_mappings`: UNIQUE on (campaign_id, chronicle_type, chronicle_id, external_system), INDEX on (external_system, external_id)
- `timeline_event_connections`: UNIQUE on (timeline_id, source_id, target_id), INDEX on timeline_id
- `entity_permissions`: UNIQUE on (entity_id, subject_type, subject_id), INDEX on entity_id, INDEX on subject_id
- `campaign_groups`: UNIQUE on (campaign_id, name)
- `campaign_group_members`: INDEX on user_id
- `entity_posts`: INDEX on entity_id, INDEX on campaign_id
- `extension_provenance`: INDEX on (campaign_id, extension_id), INDEX on (table_name, record_id)
- `extension_data`: UNIQUE on (campaign_id, extension_id, namespace, data_key)

## Migration Structure (ADR-028)

Schema is split into two tiers. See `internal/database/plugin_schema.go` for the
plugin migration runner and `internal/database/plugin_health.go` for the health
registry that tracks which plugins have healthy schemas.

### Core Schema (fatal on failure)

| File | Tables |
|------|--------|
| `db/migrations/000001_baseline.up.sql` | All core tables: users, campaigns, campaign_members, ownership_transfers, smtp_settings, entity_types, entities, media_files, tags, entity_tags, entity_relations, entity_permissions, entity_posts, entity_aliases, notes, note_versions, audit_log, site_settings, security_events, password_reset_tokens, addons, campaign_addons, campaign_groups, campaign_group_members, extensions, campaign_extensions, extension_provenance, extension_data, extension_schema_versions, plugin_schema_versions, user_storage_limits, campaign_storage_limits |

### Plugin Schema (graceful degradation on failure)

| Plugin | Migration Dir | Tables |
|--------|--------------|--------|
| calendar | `internal/plugins/calendar/migrations/` | calendars, calendar_months, calendar_weekdays, calendar_moons, calendar_seasons, calendar_events, calendar_event_categories, calendar_eras |
| maps | `internal/plugins/maps/migrations/` | maps, map_markers, map_layers, map_drawings, map_tokens, map_fog |
| sessions | `internal/plugins/sessions/migrations/` | sessions, session_entities, session_attendees, session_rsvp_tokens |
| timeline | `internal/plugins/timeline/migrations/` | timelines, timeline_event_links, timeline_entity_groups, timeline_entity_group_members, timeline_events, timeline_event_connections |
| syncapi | `internal/plugins/syncapi/migrations/` | api_keys, api_request_log, sync_mappings |
