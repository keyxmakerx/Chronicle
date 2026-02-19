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
                                |
                                +--< EntityType (configurable per campaign)
                                +--< Entity --< EntityPost
                                |       |---< EntityTag >-- Tag
                                |       |---< EntityRelation
                                |       |---< EntityPermission
                                +--< CampaignRole
                                +--< Map --< MapPin
                                +--< AuditLog

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

### campaigns (implemented -- migrations 000002, 000005, 000006)
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| name | VARCHAR(200) | NOT NULL | |
| slug | VARCHAR(200) | UNIQUE, NOT NULL | URL-safe, derived from name |
| description | TEXT | NULL | |
| settings | JSON | DEFAULT '{}' | Enabled modules, theme, etc. |
| backdrop_path | VARCHAR(500) | NULL | Campaign header image (added 000005) |
| sidebar_config | JSON | DEFAULT '{}' | Sidebar ordering/visibility (added 000006) |
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

### entity_types (implemented -- migrations 000004, 000007)
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
| sort_order | INT | DEFAULT 0 | Sidebar order |
| is_default | BOOLEAN | DEFAULT false | Ships pre-configured |
| enabled | BOOLEAN | DEFAULT true | |
| UNIQUE(campaign_id, slug) | | | |

### entities (implemented -- migration 000004)
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

### entity_posts
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

### tags
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| campaign_id | CHAR(36) | FK -> campaigns.id | |
| name | VARCHAR(100) | NOT NULL | |
| slug | VARCHAR(100) | NOT NULL | |
| color | VARCHAR(7) | DEFAULT '#6b7280' | |
| parent_id | CHAR(36) | FK -> tags.id, NULL | Nested tags |
| UNIQUE(campaign_id, slug) | | | |

### entity_tags
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| entity_id | CHAR(36) | FK -> entities.id ON DELETE CASCADE | |
| tag_id | CHAR(36) | FK -> tags.id ON DELETE CASCADE | |
| PRIMARY KEY (entity_id, tag_id) | | | |

## MariaDB-Specific Notes

- **JSON columns:** MariaDB validates JSON on write. Use `JSON_EXTRACT()` for
  queries, but prefer loading full JSON into Go and processing there.
- **UUIDs:** Stored as CHAR(36). Generated in Go with `uuid.New()`.
- **Full-text search:** Use `FULLTEXT` index on `entities.name`. Query with
  `MATCH(name) AGAINST(? IN BOOLEAN MODE)`.
- **Timestamps:** Use `DATETIME` (not TIMESTAMP which has 2038 limit). Use
  `parseTime=true` in DSN for automatic Go time.Time scanning.

## Indexes (Planned)

- `users`: UNIQUE on email
- `campaigns`: INDEX on created_by, UNIQUE on slug
- `entity_types`: UNIQUE on (campaign_id, slug)
- `entities`: INDEX on (campaign_id, entity_type_id), UNIQUE on (campaign_id, slug), FULLTEXT on name
- `tags`: UNIQUE on (campaign_id, slug)

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
