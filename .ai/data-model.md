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
         |                      +--< AuditLog
         |                      +--< Addon --< CampaignAddon
         |                      +--< ApiKey --< ApiRequestLog
         +--< PasswordResetToken

(--< means "has many")
```

## Tables

> Tables marked with **(implemented)** have migrations written. Others are planned.

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

### campaign_members (implemented -- migration 000002)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| campaign_id | CHAR(36) | PK (composite), FK -> campaigns.id ON DELETE CASCADE | |
| user_id | CHAR(36) | PK (composite), FK -> users.id ON DELETE CASCADE | |
| role | VARCHAR(20) | NOT NULL, DEFAULT 'player', CHECK IN ('owner','scribe','player') | |
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

### entities (implemented -- migrations 000004, 000014, 000023)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id, NOT NULL | |
| entity_type_id | INT | FK -> entity_types.id, NOT NULL | |
| name | VARCHAR(200) | NOT NULL | |
| slug | VARCHAR(200) | NOT NULL | |
| entry | JSON | NULL | TipTap/ProseMirror JSON doc |
| entry_html | LONGTEXT | NULL | Pre-rendered HTML |
| image_path | VARCHAR(500) | NULL | Header image |
| parent_id | CHAR(36) | FK -> entities.id, NULL | Nesting |
| type_label | VARCHAR(100) | NULL | Freeform subtype ("City") |
| is_private | BOOLEAN | DEFAULT false | GM-only |
| is_template | BOOLEAN | DEFAULT false | |
| fields_data | JSON | DEFAULT '{}' | Type-specific field values |
| field_overrides | JSON | DEFAULT NULL | Per-entity field customization (added 000014) |
| popup_config | JSON | DEFAULT NULL | Hover preview toggle config (added 000023) |
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

### entity_posts (implemented)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| entity_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| name | VARCHAR(200) | NOT NULL | |
| entry | JSON | NULL | TipTap JSON |
| entry_html | LONGTEXT | NULL | Pre-rendered |
| is_private | BOOLEAN | DEFAULT false | |
| sort_order | INT | DEFAULT 0 | |
| created_by | CHAR(36) | FK -> users.id | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### tags (implemented)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id | |
| name | VARCHAR(100) | NOT NULL | |
| slug | VARCHAR(100) | NOT NULL | |
| color | VARCHAR(7) | DEFAULT '#6b7280' | |
| parent_id | CHAR(36) | FK -> tags.id, NULL | Nested tags |
| UNIQUE(campaign_id, slug) | | | |

### entity_tags (implemented)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| entity_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| tag_id | CHAR(36) | FK -> tags.id ON DELETE CASCADE | |
| PRIMARY KEY (entity_id, tag_id) | | | |

### entity_relations (implemented)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| source_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| target_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| type | VARCHAR(100) | NOT NULL | 'ally', 'enemy', 'parent', etc. |
| reverse_type | VARCHAR(100) | NULL | Auto-created reverse label |
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

### addons (implemented -- migration 000015)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| slug | VARCHAR(100) | UNIQUE, NOT NULL | URL-safe identifier |
| name | VARCHAR(200) | NOT NULL | Display name |
| description | TEXT | NULL | |
| category | VARCHAR(50) | NOT NULL | 'module', 'widget', 'integration' |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'planned' | 'active', 'planned', 'deprecated' |
| icon | VARCHAR(50) | DEFAULT 'fa-puzzle-piece' | |
| version | VARCHAR(20) | DEFAULT '1.0.0' | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### campaign_addons (implemented -- migration 000015)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| campaign_id | CHAR(36) | PK (composite), FK -> campaigns.id ON DELETE CASCADE | |
| addon_id | CHAR(36) | PK (composite), FK -> addons.id ON DELETE CASCADE | |
| enabled | BOOLEAN | NOT NULL, DEFAULT true | |
| settings | JSON | DEFAULT '{}' | Per-campaign addon config |
| enabled_at | DATETIME | NOT NULL | |

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

### notes (implemented -- migrations 000017, 000022)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id ON DELETE CASCADE, NOT NULL | |
| user_id | CHAR(36) | FK -> users.id ON DELETE CASCADE, NOT NULL | Note creator |
| entity_id | CHAR(36) | NULL | NULL = campaign-wide note |
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

### user_storage_limits (implemented)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| user_id | CHAR(36) | PK, FK -> users.id ON DELETE CASCADE | |
| max_upload_size | BIGINT | NULL | Override (NULL = use global) |
| max_total_storage | BIGINT | NULL | Override (NULL = use global) |

### campaign_storage_limits (implemented)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| campaign_id | CHAR(36) | PK, FK -> campaigns.id ON DELETE CASCADE | |
| max_upload_size | BIGINT | NULL | Override |
| max_total_storage | BIGINT | NULL | Override |

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

- `users`: UNIQUE on email
- `campaigns`: INDEX on created_by, UNIQUE on slug
- `entity_types`: UNIQUE on (campaign_id, slug)
- `entities`: INDEX on (campaign_id, entity_type_id), UNIQUE on (campaign_id, slug), FULLTEXT on name
- `tags`: UNIQUE on (campaign_id, slug)
- `notes`: INDEX on (locked_by, locked_at), INDEX on (campaign_id, is_shared)
- `note_versions`: INDEX on (note_id, created_at DESC)

## Migration Log

| # | File | Description | Date Applied |
|---|------|-------------|-------------|
| 1 | 000001_create_users | Users table with auth fields | 2026-02-19 |
| 2 | 000002_create_campaigns | Campaigns, campaign_members, ownership_transfers | 2026-02-19 |
| 3 | 000003_create_smtp_settings | SMTP settings singleton table | 2026-02-19 |
| 4 | 000004_create_entities | Entity types + entities tables | 2026-02-19 |
| 5 | 000005_create_media | Media files table + campaigns.backdrop_path | 2026-02-19 |
| 6 | 000006_sidebar_config | campaigns.sidebar_config JSON column | 2026-02-19 |
| 7 | 000007_entity_type_layout | entity_types.layout_json JSON column | 2026-02-19 |
| 8 | 000008_entity_posts | Entity sub-posts table | 2026-02-19 |
| 9 | 000009_tags | Tags + entity_tags tables | 2026-02-19 |
| 10 | 000010_relations | Entity relations table | 2026-02-19 |
| 11 | 000011_audit_log | Audit logging table | 2026-02-19 |
| 12 | 000012_storage_settings | Storage settings + per-user/campaign limits | 2026-02-19 |
| 13 | 000013_entity_type_dashboards | description + pinned_entity_ids on entity_types | 2026-02-20 |
| 14 | 000014_field_overrides | field_overrides JSON on entities | 2026-02-20 |
| 15 | 000015_addons | Addons + campaign_addons tables (11 seeds) | 2026-02-20 |
| 16 | 000016_api_keys | API keys, request log, security events, IP blocklist | 2026-02-20 |
| 17 | 000017_notes | Notes table (per-user, per-campaign) | 2026-02-20 |
| 18 | 000018_activate_notes_addon | Activates player-notes addon | 2026-02-20 |
| 19 | 000019_fix_addon_statuses | Fixes addon status mismatches | 2026-02-20 |
| 20 | 000020_password_reset_tokens | Password reset tokens table | 2026-02-22 |
| 21 | 000021_dashboard_layouts | dashboard_layout on campaigns + entity_types | 2026-02-22 |
| 22 | 000022_notes_collaboration | Shared notes, locking, versions (is_shared, locked_by/at, entry/entry_html, note_versions) | 2026-02-24 |
| 23 | 000023_entity_popup_config | popup_config JSON on entities for hover preview settings | 2026-02-24 |
