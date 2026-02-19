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

> **NOTE:** No tables exist yet. This section will be populated as migrations
> are written. The schema below is the PLANNED design from the project spec.

### users
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

### campaigns
| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | CHAR(36) | PK | UUID |
| name | VARCHAR(200) | NOT NULL | |
| slug | VARCHAR(200) | UNIQUE, NOT NULL | URL-safe, derived from name |
| description | TEXT | NULL | |
| settings | JSON | DEFAULT '{}' | Enabled modules, theme, etc. |
| created_by | CHAR(36) | FK -> users.id | |
| created_at | DATETIME | NOT NULL | |
| updated_at | DATETIME | NOT NULL | |

### entity_types
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
| sort_order | INT | DEFAULT 0 | Sidebar order |
| is_default | BOOLEAN | DEFAULT false | Ships pre-configured |
| enabled | BOOLEAN | DEFAULT true | |
| UNIQUE(campaign_id, slug) | | | |

### entities
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
| - | - | No migrations yet | - |
