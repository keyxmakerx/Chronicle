# Community Bestiary — Design Document

> **Status:** Draft
> **Author:** Chronicle Team
> **Last Updated:** 2026-03-24
> **Related:** [Monster Builder](../../../Chronicle-Draw-Steel/docs/monster-builder.md) | [API & Security](./api-security.md) | [Foundry Sync](../../../Chronicle-Draw-Steel/docs/foundry-creature-sync.md)

---

## 1. Overview

The Community Bestiary is a social sharing hub for homebrew creatures. It follows a **local-first, federation-ready** architecture: users on a Chronicle instance can publish, browse, rate, and import each other's creatures immediately. A future central hub (potential SaaS product) can aggregate content across instances.

**Inspiration:** D&D Beyond's public homebrew library — browsable cards, ratings, creator profiles, one-click import, trending/newest feeds.

### Goals

- Server-local sharing: any user can publish creatures visible to all users on the instance
- Browse/search with filtering by level, organization, role, keywords, tags
- Rating system (1–5 stars) with optional text reviews
- Favorites/bookmarks for personal libraries
- One-click import into any campaign the user has access to
- Moderation tools for instance admins (flag, review, archive)
- Federation-ready API contract for future central hub

### Non-Goals (Phase 1)

- Cross-instance sharing (Phase 2: central hub)
- Monetization/premium tiers (Phase 2)
- Creature collections/packs (future)
- Comments/discussion threads (use reviews for now)

---

## 2. Architecture

### 2.1 Local-First Model

```
┌───────────────────────────────────────────────┐
│           Chronicle Instance                   │
│                                               │
│  ┌─────────────┐     ┌─────────────────────┐  │
│  │   Monster    │     │  Community Bestiary  │  │
│  │   Builder    │────▶│  (bestiary addon)    │  │
│  │  (DS widget) │     │                     │  │
│  └─────────────┘     │  ┌───────────────┐   │  │
│                      │  │ Publications  │   │  │
│  ┌─────────────┐     │  │ Ratings       │   │  │
│  │  Campaign    │◀────│  │ Favorites     │   │  │
│  │  Entities    │     │  │ Imports       │   │  │
│  └─────────────┘     │  │ Moderation    │   │  │
│                      │  └───────────────┘   │  │
│                      └─────────────────────┘  │
└───────────────────────────────────────────────┘
```

### 2.2 Federation-Ready Model (Future)

```
┌──────────────────┐     ┌──────────────────────────┐     ┌──────────────────┐
│  Chronicle        │     │    Chronicle Hub (SaaS)   │     │  Chronicle        │
│  Instance A       │     │                          │     │  Instance B       │
│                   │     │  ┌────────────────────┐  │     │                   │
│  publish ────────────▶  │  │ Central Registry   │  │  ◀──────── publish    │
│                   │     │  │ • Aggregated search │  │     │                   │
│  import  ◀───────────── │  │ • Global ratings    │  │ ──────────▶ import   │
│                   │     │  │ • Creator profiles  │  │     │                   │
│                   │     │  │ • Moderation        │  │     │                   │
│                   │     │  │ • Analytics         │  │     │                   │
│                   │     │  └────────────────────┘  │     │                   │
└──────────────────┘     └──────────────────────────┘     └──────────────────┘
```

### 2.3 Integration with Chronicle Core

The bestiary is a **new Chronicle addon/plugin** (like calendar, maps, notes). It:

- Registers as addon slug `bestiary` with status `available`
- Is enabled per-campaign (but publications are instance-wide, not campaign-scoped)
- Uses its own DB tables (not the entities table)
- Provides its own routes under `/bestiary/*` and `/admin/bestiary/*`
- Reuses Chronicle's existing auth, IDOR protection, and middleware patterns

**Key distinction:** Publications are **instance-scoped**, not campaign-scoped. A user publishes from a campaign, but the publication is visible to all authenticated users on the instance. This is different from entities which are always campaign-scoped.

---

## 3. Data Model

### 3.1 Entity Relationship Diagram

```
users ──────┐
            │ creator_id
            ▼
    bestiary_publications ◀──── bestiary_ratings (user_id, publication_id)
            │
            │                   bestiary_favorites (user_id, publication_id)
            │
            ├──── bestiary_imports (publication_id → campaign entities)
            │
            └──── bestiary_moderation_log (publication_id, moderator_id)
```

### 3.2 Table: `bestiary_publications`

The core table storing published creature statblocks.

| Column | Type | Nullable | Notes |
|---|---|---|---|
| `id` | CHAR(36) PK | no | UUID |
| `creator_id` | CHAR(36) FK→users | no | Publishing user |
| `source_entity_id` | CHAR(36) | yes | Original entity (null if imported from hub) |
| `source_campaign_id` | CHAR(36) | yes | Campaign it was published from |
| `system_id` | VARCHAR(100) | no | "drawsteel" — enables multi-system bestiary |
| `name` | VARCHAR(200) | no | Creature name |
| `slug` | VARCHAR(200) UNIQUE | no | URL-safe slug, auto-generated from name |
| `description` | TEXT | yes | Short description / elevator pitch |
| `flavor_text` | TEXT | yes | Lore / narrative text |
| `artwork_media_id` | CHAR(36) | yes | FK to Chronicle media system |
| `statblock_json` | JSON | no | Complete creature definition (validated on write) |
| `version` | INT | no | Incremented on update, starts at 1 |
| `tags` | JSON | yes | User-defined tags for discovery |
| `organization` | VARCHAR(50) | yes | Denormalized from statblock for filtering |
| `role` | VARCHAR(50) | yes | Denormalized from statblock for filtering |
| `level` | INT | yes | Denormalized from statblock for filtering |
| `downloads` | INT | no | Import count, default 0 |
| `rating_sum` | INT | no | Sum of all ratings (1–5), default 0 |
| `rating_count` | INT | no | Number of ratings, default 0 |
| `favorites` | INT | no | Favorite count, default 0 |
| `visibility` | ENUM | no | draft, published, unlisted, archived, flagged |
| `flagged_count` | INT | no | Number of user flags, default 0 |
| `reviewed_by` | CHAR(36) | yes | Admin who last reviewed |
| `reviewed_at` | DATETIME | yes | When last reviewed |
| `hub_id` | VARCHAR(100) | yes | ID on central hub (NULL = local only) |
| `hub_synced_at` | DATETIME | yes | Last sync to hub |
| `created_at` | DATETIME | no | |
| `updated_at` | DATETIME | no | |

**Indexes:**
- `idx_system_visibility` on (system_id, visibility)
- `idx_level_org_role` on (level, organization, role)
- `idx_creator` on (creator_id)
- `FULLTEXT idx_search` on (name, description)
- `idx_slug` UNIQUE on (slug)

### 3.3 Table: `bestiary_ratings`

One rating per user per publication.

| Column | Type | Notes |
|---|---|---|
| `id` | CHAR(36) PK | UUID |
| `publication_id` | CHAR(36) FK | |
| `user_id` | CHAR(36) FK | |
| `rating` | TINYINT | 1–5, CHECK constraint |
| `review_text` | TEXT | Optional text review |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |

**Constraints:** UNIQUE (user_id, publication_id)

### 3.4 Table: `bestiary_favorites`

Simple bookmark table.

| Column | Type | Notes |
|---|---|---|
| `user_id` | CHAR(36) | Composite PK |
| `publication_id` | CHAR(36) | Composite PK |
| `created_at` | DATETIME | |

### 3.5 Table: `bestiary_imports`

Tracks which publications were imported into which campaigns.

| Column | Type | Notes |
|---|---|---|
| `id` | CHAR(36) PK | UUID |
| `publication_id` | CHAR(36) FK | |
| `user_id` | CHAR(36) FK | Importing user |
| `campaign_id` | CHAR(36) FK | Target campaign |
| `entity_id` | CHAR(36) FK | Created entity |
| `imported_at` | DATETIME | |

**Constraints:** UNIQUE (publication_id, campaign_id) — prevent duplicate imports

### 3.6 Table: `bestiary_moderation_log`

Audit trail for all moderation actions.

| Column | Type | Notes |
|---|---|---|
| `id` | CHAR(36) PK | UUID |
| `publication_id` | CHAR(36) FK | |
| `moderator_id` | CHAR(36) FK | Admin user |
| `action` | ENUM | approve, flag, unflag, archive, restore |
| `reason` | TEXT | Moderator's reason |
| `created_at` | DATETIME | |

---

## 4. Statblock JSON Schema

The `statblock_json` column stores the complete creature definition. Schema validation happens on publish.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["name", "level", "organization", "role", "stamina", "speed", "characteristics"],
  "properties": {
    "name": { "type": "string", "maxLength": 200 },
    "level": { "type": "integer", "minimum": 1, "maximum": 20 },
    "organization": {
      "type": "string",
      "enum": ["minion", "horde", "platoon", "elite", "leader", "solo", "swarm"]
    },
    "role": {
      "type": "string",
      "enum": ["brute", "controller", "defender", "harrier", "hexer"]
    },
    "ev": { "type": "integer", "minimum": 1 },
    "keywords": {
      "type": "array",
      "items": { "type": "string", "maxLength": 50 },
      "maxItems": 20
    },
    "faction": { "type": "string", "maxLength": 100 },
    "size": {
      "type": "string",
      "enum": ["T", "S", "M", "L", "H", "G"]
    },
    "stamina": { "type": "integer", "minimum": 1 },
    "winded": { "type": "integer", "minimum": 0 },
    "speed": { "type": "integer", "minimum": 0 },
    "stability": { "type": "integer", "minimum": 0 },
    "characteristics": {
      "type": "object",
      "required": ["might", "agility", "reason", "intuition", "presence"],
      "properties": {
        "might": { "type": "integer", "minimum": -5, "maximum": 10 },
        "agility": { "type": "integer", "minimum": -5, "maximum": 10 },
        "reason": { "type": "integer", "minimum": -5, "maximum": 10 },
        "intuition": { "type": "integer", "minimum": -5, "maximum": 10 },
        "presence": { "type": "integer", "minimum": -5, "maximum": 10 }
      }
    },
    "immunities": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "type": { "type": "string" },
          "value": { "type": "integer" }
        }
      },
      "maxItems": 10
    },
    "free_strike": { "type": "string", "maxLength": 500 },
    "traits": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["name", "description"],
        "properties": {
          "name": { "type": "string", "maxLength": 100 },
          "description": { "type": "string", "maxLength": 2000 }
        }
      },
      "maxItems": 20
    },
    "abilities": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["name", "type"],
        "properties": {
          "name": { "type": "string", "maxLength": 100 },
          "type": {
            "type": "string",
            "enum": ["signature", "action", "maneuver", "triggered"]
          },
          "keywords": {
            "type": "array",
            "items": { "type": "string" },
            "maxItems": 10
          },
          "distance": { "type": "string", "maxLength": 100 },
          "target": { "type": "string", "maxLength": 200 },
          "power_roll": { "type": "string", "maxLength": 100 },
          "tier1_result": { "type": "string", "maxLength": 1000 },
          "tier2_result": { "type": "string", "maxLength": 1000 },
          "tier3_result": { "type": "string", "maxLength": 1000 },
          "effect": { "type": "string", "maxLength": 2000 },
          "trigger": { "type": "string", "maxLength": 200 },
          "vp_cost": { "type": "integer", "minimum": 0, "maximum": 10 }
        }
      },
      "maxItems": 30
    },
    "villain_actions": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["name", "order", "description"],
        "properties": {
          "name": { "type": "string", "maxLength": 100 },
          "order": {
            "type": "string",
            "enum": ["opener", "crowd-control", "ultimate"]
          },
          "description": { "type": "string", "maxLength": 2000 },
          "keywords": {
            "type": "array",
            "items": { "type": "string" },
            "maxItems": 10
          },
          "distance": { "type": "string", "maxLength": 100 },
          "target": { "type": "string", "maxLength": 200 },
          "power_roll": { "type": "string", "maxLength": 100 },
          "tier1_result": { "type": "string", "maxLength": 1000 },
          "tier2_result": { "type": "string", "maxLength": 1000 },
          "tier3_result": { "type": "string", "maxLength": 1000 },
          "effect": { "type": "string", "maxLength": 2000 }
        }
      },
      "maxItems": 3
    }
  }
}
```

**Size limits:**
- Maximum `statblock_json` size: 100KB
- Maximum abilities: 30
- Maximum villain actions: 3
- Maximum traits: 20
- Maximum keywords: 20
- All text fields have explicit maxLength

---

## 5. Publication Lifecycle

### 5.1 States

```
                  ┌──────────┐
                  │  draft    │ ← User creates, not visible to others
                  └────┬─────┘
                       │ publish
                       ▼
                  ┌──────────┐
          ┌──────│ published │◀──── update (increments version)
          │      └────┬─────┘
          │           │
     flag │ (3+ flags)│ creator archives
          │           │
          ▼           ▼
     ┌──────────┐ ┌──────────┐
     │  flagged  │ │ archived │
     └────┬─────┘ └────┬─────┘
          │             │
    admin │ review      │ creator restores
          │             │
          ▼             ▼
     ┌──────────┐ ┌──────────┐
     │ archived │ │ published│
     │(by admin)│ └──────────┘
     └──────────┘

  Special state:
  ┌──────────┐
  │ unlisted │ ← Accessible by direct link, not in search results
  └──────────┘
```

### 5.2 Publish Flow

1. User builds creature in Monster Builder widget
2. User clicks "Publish to Bestiary"
3. Client validates statblock (all E-rules pass)
4. Client sends `POST /bestiary` with statblock JSON
5. Server validates:
   - User is authenticated
   - Statblock passes JSON schema validation
   - Statblock JSON ≤ 100KB
   - All text fields sanitized (HTML escaped, markdown allowed)
   - User hasn't exceeded publish rate limit (10/hour)
   - Slug is unique (auto-generated, with numeric suffix on collision)
6. Server creates publication with visibility = `published`
7. Server denormalizes level/organization/role for search indexes
8. Returns publication ID and slug

### 5.3 Import Flow

1. User browsing bestiary clicks "Import to Campaign"
2. Client presents campaign picker (campaigns where user has Scribe+ role)
3. Client sends `POST /bestiary/:id/import/:campaignId`
4. Server validates:
   - Publication exists and is published/unlisted
   - User has Scribe or Owner role in target campaign
   - Publication hasn't already been imported to this campaign
5. Server creates entity in campaign:
   - Entity type = `drawsteel-creature` preset
   - Custom fields populated from statblock JSON
   - Entity name = creature name
   - `abilities_json` and `villain_actions_json` populated
6. Server creates import tracking record
7. Server increments download count on publication
8. Returns created entity ID

### 5.4 Fork Flow

"Fork & Edit" creates a copy of someone else's creature in your campaign for modification:

1. Same as import flow, but:
   - Entity gets "(Fork)" suffix in name
   - Fork is tracked as a separate import with `is_fork = true`
   - User can then modify freely and optionally re-publish as a new publication

---

## 6. Search & Discovery

### 6.1 Search Endpoint

`GET /bestiary/search`

**Query parameters:**
| Param | Type | Notes |
|---|---|---|
| `q` | string | Full-text search on name + description |
| `level_min` | int | Minimum creature level |
| `level_max` | int | Maximum creature level |
| `organization` | string | Filter by organization |
| `role` | string | Filter by role |
| `keywords` | string | Comma-separated keyword filter |
| `tags` | string | Comma-separated tag filter |
| `creator_id` | string | Filter by creator |
| `system_id` | string | Filter by game system (default: campaign's active system) |
| `sort` | string | trending, newest, top_rated, most_imported |
| `page` | int | Pagination (default 1) |
| `per_page` | int | Results per page (default 20, max 50) |

### 6.2 Trending Algorithm

"Trending" uses a time-decayed popularity score:

```
score = (downloads_recent * 3 + ratings_recent * 2 + favorites_recent * 1) / age_hours^0.5
```

Where `_recent` = last 7 days. Recalculated hourly via background job or on-demand with caching.

### 6.3 Featured Feeds

| Feed | Sort | Notes |
|---|---|---|
| Trending | Time-decayed score | Homepage default |
| Newest | created_at DESC | Fresh content |
| Top Rated | rating_sum/rating_count DESC | Minimum 3 ratings |
| Most Imported | downloads DESC | All-time imports |
| Staff Picks | Manual curation | Admin-tagged publications |

---

## 7. Rating System

### 7.1 Rating Rules

- One rating per user per publication (upsert on re-rate)
- Rating: 1–5 stars (integer)
- Optional text review (max 2000 chars, sanitized)
- Creator cannot rate their own publication
- Rating is immediately reflected in publication's aggregated score
- Review text visible to all authenticated users

### 7.2 Aggregation

Stored on `bestiary_publications`:
- `rating_sum` — Sum of all ratings
- `rating_count` — Number of ratings

Average: `rating_sum / rating_count` (computed at read time, not stored)

Updated atomically on rate/unrate:
```sql
UPDATE bestiary_publications
SET rating_sum = rating_sum + :new_rating - COALESCE(:old_rating, 0),
    rating_count = rating_count + CASE WHEN :old_rating IS NULL THEN 1 ELSE 0 END
WHERE id = :publication_id
```

---

## 8. Creator Profiles

Each user who publishes at least one creature gets a public creator profile:

**`GET /bestiary/creators/:userId`**

Response:
```json
{
  "user_id": "...",
  "display_name": "stormDM",
  "avatar_url": "...",
  "publications_count": 12,
  "total_downloads": 1456,
  "average_rating": 4.3,
  "member_since": "2026-01-15",
  "publications": [ /* paginated list */ ]
}
```

No new DB table needed — aggregated from existing data at query time (with caching).

---

## 9. Moderation

### 9.1 User Flagging

Any authenticated user can flag a publication:
- `POST /bestiary/:id/flag` with optional reason
- One flag per user per publication
- At 3 flags, publication auto-changes to `flagged` visibility (hidden from search)
- Flagged content goes to admin moderation queue

### 9.2 Admin Moderation

Admin dashboard at `/admin/bestiary/flagged`:
- View flagged publications with flag reasons
- Actions: approve (unflag), archive (remove), restore
- All actions logged in `bestiary_moderation_log`

### 9.3 Auto-Moderation (Future)

- Profanity filter on name/description/review text
- Duplicate detection (cosine similarity on statblock JSON)
- Rate limiting escalation (reduce limits for users with archived content)

---

## 10. Federation API Contract (Future Hub)

The local bestiary API is designed so a central hub can mirror the same interface:

### 10.1 Hub Registration

```
POST /api/v1/hub/register
{
  "instance_url": "https://my-chronicle.example.com",
  "instance_name": "My Chronicle",
  "admin_email": "admin@example.com",
  "public_key": "..." // For content signing
}
→ { "instance_id": "...", "api_key": "..." }
```

### 10.2 Publish to Hub

```
POST /api/v1/hub/publish
Authorization: Bearer <instance_api_key>
{
  "local_id": "...",
  "creator_display_name": "stormDM",
  "statblock_json": { ... },
  "name": "Ashen Wyrm",
  "description": "...",
  "tags": [...],
  "signature": "..." // HMAC of statblock_json with instance key
}
→ { "hub_id": "...", "url": "https://hub.chronicle.app/bestiary/ashen-wyrm" }
```

### 10.3 Browse Hub

```
GET /api/v1/hub/browse?q=dragon&level_min=5&sort=trending
→ { "results": [...], "total": 42, "page": 1 }
```

### 10.4 Import from Hub

```
POST /api/v1/hub/import/:hubId
→ { "statblock_json": { ... }, "creator": "stormDM@other-instance.com" }
```

### 10.5 Monetization Tiers (Future)

| Tier | Price | Limits |
|---|---|---|
| **Free** | $0/mo | 10 publishes/month, basic search |
| **Creator** | $5/mo | Unlimited publishes, analytics, featured placement |
| **Instance** | $15/mo | Full hub access for all users, priority sync, custom branding |

---

## 11. Open Questions

1. **Should ratings federate?** If creature X is rated 4.5 locally and 3.8 on the hub, which score shows?

2. **Content licensing:** Should published creatures default to CC-BY-4.0 (matching Draw Steel's license)? Or should creators choose?

3. **Artwork in hub:** Should the hub host artwork, or just link to instance URLs? Hosting adds cost but prevents broken images.

4. **Version sync:** If a creator updates a published creature, should importers get notified? Auto-update?

5. **Collections/packs:** Should creators be able to group creatures into themed collections (e.g., "Goblin War Band" with 5 creatures)? Future feature or MVP?
