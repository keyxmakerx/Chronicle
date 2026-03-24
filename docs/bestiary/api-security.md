# Community Bestiary — API & Security Specification

> **Status:** Draft
> **Author:** Chronicle Team
> **Last Updated:** 2026-03-24
> **Related:** [Bestiary Design](./design.md) | [Monster Builder](../../../Chronicle-Draw-Steel/docs/monster-builder.md)

---

## 1. API Routes

### 1.1 Public Browsing (Authenticated Users)

All routes require a valid session (logged-in user). No anonymous access.

```
GET    /bestiary                              Browse published creatures (paginated)
GET    /bestiary/search                       Filtered search
GET    /bestiary/trending                     Trending feed
GET    /bestiary/newest                       Newest feed
GET    /bestiary/top-rated                    Top rated feed (min 3 ratings)
GET    /bestiary/most-imported                Most imported feed
GET    /bestiary/:slug                        View single publication
GET    /bestiary/:slug/statblock              Raw statblock JSON
GET    /bestiary/:slug/reviews                Paginated reviews
GET    /bestiary/creators/:userId             Creator profile + publications
GET    /bestiary/favorites                    Current user's favorites
GET    /bestiary/my-creations                 Current user's publications (all states)
```

### 1.2 Authenticated Actions

```
POST   /bestiary                              Publish a creature
PUT    /bestiary/:id                          Update your publication
DELETE /bestiary/:id                          Soft-delete (archive) your publication
PATCH  /bestiary/:id/visibility               Change visibility (draft/published/unlisted/archived)
POST   /bestiary/:id/rate                     Rate 1-5 + optional review
DELETE /bestiary/:id/rate                     Remove your rating
POST   /bestiary/:id/favorite                 Toggle favorite
DELETE /bestiary/:id/favorite                 Remove favorite
POST   /bestiary/:id/import/:campaignId       Import into campaign
POST   /bestiary/:id/fork/:campaignId         Fork into campaign (creates editable copy)
POST   /bestiary/:id/flag                     Flag for moderation
```

### 1.3 Admin / Moderation

Requires site admin role.

```
GET    /admin/bestiary/flagged                Flagged publications queue
GET    /admin/bestiary/stats                  Dashboard stats
POST   /admin/bestiary/:id/moderate           Approve/archive/unflag with reason
GET    /admin/bestiary/:id/moderation-log     View moderation history
```

### 1.4 Federation API (Future — Hub Mode)

These endpoints would live on the central hub service.

```
POST   /api/v1/hub/register                   Register an instance
POST   /api/v1/hub/publish                    Publish to hub
GET    /api/v1/hub/browse                     Search hub content
GET    /api/v1/hub/creature/:hubId            Get creature from hub
POST   /api/v1/hub/import/:hubId              Import from hub
POST   /api/v1/hub/rate/:hubId                Rate on hub
GET    /api/v1/hub/creators/:creatorId        Hub creator profile
POST   /api/v1/hub/sync                       Sync ratings/downloads
```

---

## 2. Request/Response Formats

### 2.1 Publish Creature

**`POST /bestiary`**

Request:
```json
{
  "source_entity_id": "uuid-of-entity",
  "source_campaign_id": "uuid-of-campaign",
  "name": "Ashen Wyrm",
  "description": "A dragon of cinder and ash that feeds on dying fires.",
  "flavor_text": "Where the Ashen Wyrm walks, even flame fears to tread...",
  "tags": ["dragon", "fire", "boss", "horror"],
  "statblock_json": {
    "name": "Ashen Wyrm",
    "level": 8,
    "organization": "solo",
    "role": "brute",
    "ev": 192,
    "keywords": ["dragon", "beast"],
    "size": "H",
    "stamina": 240,
    "winded": 120,
    "speed": 7,
    "stability": 3,
    "characteristics": {
      "might": 4,
      "agility": 1,
      "reason": -1,
      "intuition": 2,
      "presence": 3
    },
    "immunities": [
      { "type": "Fire", "value": 5 }
    ],
    "free_strike": "12 fire damage",
    "traits": [
      {
        "name": "Ember Aura",
        "description": "Any creature that starts its turn within 2 squares of the Ashen Wyrm takes 5 fire damage."
      }
    ],
    "abilities": [
      {
        "name": "Inferno Bite",
        "type": "signature",
        "keywords": ["Attack", "Melee", "Fire"],
        "distance": "Melee",
        "target": "1 creature",
        "power_roll": "Might vs. Agility",
        "tier1_result": "8 fire damage",
        "tier2_result": "14 fire damage",
        "tier3_result": "20 fire damage; target is burning (EoT)"
      }
    ],
    "villain_actions": [
      {
        "name": "Ember Storm",
        "order": "opener",
        "description": "The Ashen Wyrm beats its wings. Each creature within 3 squares takes 8 fire damage and is pushed 3 squares.",
        "keywords": ["Area", "Fire"]
      },
      {
        "name": "Ash Cloud",
        "order": "crowd-control",
        "description": "A choking cloud of ash fills a 5-square area within 10 squares. The area is heavily obscured until end of the encounter. Creatures in the area are slowed (EoT).",
        "keywords": ["Area", "Fire"]
      },
      {
        "name": "Extinction Breath",
        "order": "ultimate",
        "description": "The Ashen Wyrm exhales a devastating cone of superheated ash. All creatures in a 6-square cone take 30 fire damage (no roll). Creatures reduced to 0 stamina by this ability are turned to ash.",
        "keywords": ["Area", "Fire"]
      }
    ]
  },
  "visibility": "published"
}
```

Response (201 Created):
```json
{
  "id": "uuid",
  "slug": "ashen-wyrm",
  "name": "Ashen Wyrm",
  "version": 1,
  "visibility": "published",
  "url": "/bestiary/ashen-wyrm",
  "created_at": "2026-03-24T12:00:00Z"
}
```

### 2.2 Browse/Search Response

**`GET /bestiary/search?q=dragon&level_min=5&sort=trending`**

```json
{
  "results": [
    {
      "id": "uuid",
      "slug": "ashen-wyrm",
      "name": "Ashen Wyrm",
      "description": "A dragon of cinder and ash...",
      "level": 8,
      "organization": "solo",
      "role": "brute",
      "keywords": ["dragon", "beast"],
      "tags": ["dragon", "fire", "boss"],
      "creator": {
        "id": "user-uuid",
        "display_name": "stormDM",
        "avatar_url": "/media/avatars/user-uuid.jpg"
      },
      "rating_average": 4.6,
      "rating_count": 47,
      "downloads": 234,
      "favorites": 89,
      "version": 2,
      "created_at": "2026-03-21T15:30:00Z",
      "updated_at": "2026-03-23T10:00:00Z"
    }
  ],
  "total": 42,
  "page": 1,
  "per_page": 20,
  "total_pages": 3
}
```

### 2.3 Import Response

**`POST /bestiary/:id/import/:campaignId`**

```json
{
  "entity_id": "new-entity-uuid",
  "campaign_id": "target-campaign-uuid",
  "publication_id": "source-publication-uuid",
  "creature_name": "Ashen Wyrm",
  "entity_url": "/campaigns/target-campaign-uuid/entities/new-entity-uuid"
}
```

### 2.4 Rate Request

**`POST /bestiary/:id/rate`**

```json
{
  "rating": 5,
  "review_text": "Ran this against 5 level 7 heroes, perfect balance. The villain actions flow beautifully."
}
```

---

## 3. Security Specification

### 3.1 Authentication & Authorization

| Endpoint Pattern | Required Auth | Required Role |
|---|---|---|
| `GET /bestiary/*` | Session (logged in) | Any authenticated user |
| `POST /bestiary` | Session | Any authenticated user |
| `PUT /bestiary/:id` | Session | Publication creator only |
| `DELETE /bestiary/:id` | Session | Publication creator only |
| `PATCH /bestiary/:id/visibility` | Session | Publication creator only |
| `POST /bestiary/:id/rate` | Session | Any user (not creator) |
| `POST /bestiary/:id/import/:cid` | Session | Scribe+ role in target campaign |
| `POST /bestiary/:id/fork/:cid` | Session | Scribe+ role in target campaign |
| `POST /bestiary/:id/flag` | Session | Any authenticated user |
| `GET /admin/bestiary/*` | Session | Site admin |
| `POST /admin/bestiary/*` | Session | Site admin |

### 3.2 IDOR Prevention

All resource-scoped operations verify ownership:

```go
// Publication modification — verify creator
func requirePublicationCreator(ctx echo.Context, pub *BestiaryPublication, userID string) error {
    if pub.CreatorID != userID {
        return echo.NewHTTPError(http.StatusForbidden, "you can only modify your own publications")
    }
    return nil
}

// Campaign import — verify campaign role
func requireCampaignScribe(ctx echo.Context, campaignID, userID string) error {
    role := getUserCampaignRole(campaignID, userID)
    if role < RoleScribe {
        return echo.NewHTTPError(http.StatusForbidden, "scribe or owner role required")
    }
    return nil
}

// Self-rating prevention
func preventSelfRating(pub *BestiaryPublication, userID string) error {
    if pub.CreatorID == userID {
        return echo.NewHTTPError(http.StatusForbidden, "cannot rate your own publication")
    }
    return nil
}
```

### 3.3 Input Validation

#### 3.3.1 Statblock JSON Validation

Server-side validation on every publish/update:

1. **Size check:** `len(statblock_json) <= 102400` (100KB)
2. **JSON schema validation:** Against the schema in [design.md §4](./design.md#4-statblock-json-schema)
3. **String sanitization:** All string fields in statblock HTML-escaped
4. **Markdown sanitization:** Markdown fields processed through allowlisted renderer (no raw HTML, no `<script>`, no `javascript:` URLs)
5. **Enum validation:** organization, role, size, ability types checked against allowed values
6. **Array bounds:** abilities ≤ 30, villain_actions ≤ 3, traits ≤ 20, keywords ≤ 20, immunities ≤ 10
7. **Numeric bounds:** level 1–20, characteristics -5 to +10, stamina > 0

#### 3.3.2 Text Field Validation

| Field | Max Length | Sanitization |
|---|---|---|
| `name` | 200 chars | HTML escape |
| `description` | 5000 chars | HTML escape |
| `flavor_text` | 5000 chars | HTML escape |
| `review_text` | 2000 chars | HTML escape |
| `flag reason` | 1000 chars | HTML escape |
| `moderation reason` | 2000 chars | HTML escape |
| `tags` (each) | 50 chars | HTML escape, lowercase, alphanumeric + hyphens only |

#### 3.3.3 Slug Generation

```go
func generateSlug(name string) string {
    slug := strings.ToLower(name)
    slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
    slug = strings.Trim(slug, "-")
    if len(slug) > 100 {
        slug = slug[:100]
    }
    // On collision, append -2, -3, etc.
    return ensureUnique(slug)
}
```

### 3.4 Rate Limiting

| Action | Limit | Window | Key |
|---|---|---|---|
| Publish (POST /bestiary) | 10 | 1 hour | user_id |
| Update (PUT /bestiary/:id) | 20 | 1 hour | user_id |
| Rate (POST /bestiary/:id/rate) | 30 | 1 hour | user_id |
| Import | 50 | 1 hour | user_id |
| Flag | 10 | 1 hour | user_id |
| Search | 100 | 1 minute | user_id |
| Browse pages | 200 | 1 minute | user_id |

Implementation: Token bucket per user, stored in Redis (or in-memory for single-instance). Returns `429 Too Many Requests` with `Retry-After` header.

### 3.5 Anti-Abuse

#### 3.5.1 Auto-Flag Threshold

Publications are automatically hidden (visibility → `flagged`) when:
- `flagged_count >= 3` from unique users
- Admin is notified via existing notification system

#### 3.5.2 Creator Reputation

Track per-user:
- `publications_archived_by_admin` — count of admin-archived publications
- If count ≥ 3, reduce rate limits by 50%
- If count ≥ 5, require admin approval for new publications

#### 3.5.3 Duplicate Detection

On publish, check for exact name + level + organization matches from the same creator. Warn (don't block) if potential duplicate found.

### 3.6 Data Privacy

| Data | Visibility | Notes |
|---|---|---|
| Publication content | All authenticated users | Published/unlisted only |
| Creator user ID | All authenticated users | Displayed as profile link |
| Creator display name | All authenticated users | From user profile |
| Rating + review text | All authenticated users | Associated with reviewer display name |
| Flag reasons | Admins only | Not visible to creator or other users |
| Moderation log | Admins only | Audit trail |
| Draft publications | Creator only | Not visible in search/browse |
| Source campaign ID | Creator only | Not exposed in API responses |
| Source entity ID | Creator only | Not exposed in API responses |

### 3.7 SQL Injection Prevention

All queries use parameterized statements (Go's `database/sql` with `?` placeholders). No string concatenation in queries.

Full-text search uses MySQL's `MATCH() AGAINST()` with parameterized input:
```go
query := `SELECT * FROM bestiary_publications
          WHERE visibility = 'published'
          AND MATCH(name, description) AGAINST(? IN BOOLEAN MODE)`
db.Query(query, sanitizedSearchTerm)
```

Search terms are sanitized: strip MySQL full-text operators (`+`, `-`, `*`, `~`, `<`, `>`, `(`, `)`, `@`).

### 3.8 XSS Prevention

1. **Statblock rendering:** All string values HTML-escaped before DOM insertion
2. **Markdown rendering:** Allowlisted subset only (bold, italic, lists, links). No raw HTML passthrough.
3. **Review text:** Plain text only, displayed with `textContent` not `innerHTML`
4. **Slug:** Generated from name, alphanumeric + hyphens only
5. **Tags:** Alphanumeric + hyphens only, lowercased

### 3.9 CSRF Protection

All state-changing endpoints (POST/PUT/DELETE/PATCH) require:
- Valid session cookie
- Chronicle's existing CSRF token in `X-CSRF-Token` header
- Referer/Origin header validation

### 3.10 Federation Security (Future Hub)

| Concern | Mitigation |
|---|---|
| Instance authentication | API key issued during registration, rotatable |
| Content integrity | HMAC signature on statblock JSON with instance secret |
| Hub API abuse | Per-instance rate limits, separate from per-user |
| Data poisoning | Hub validates statblock schema independently |
| Instance impersonation | Public key verification, domain validation |
| Content takedown | Hub admin can archive any publication, instance notified |

---

## 4. Error Responses

All errors follow Chronicle's standard error format:

```json
{
  "error": {
    "code": "BESTIARY_RATE_LIMIT",
    "message": "Rate limit exceeded. Try again in 42 seconds.",
    "details": {
      "retry_after": 42,
      "limit": 10,
      "window": "1h"
    }
  }
}
```

### Error Codes

| Code | HTTP Status | Description |
|---|---|---|
| `BESTIARY_NOT_FOUND` | 404 | Publication not found or not visible |
| `BESTIARY_FORBIDDEN` | 403 | Not authorized (not creator, wrong role, etc.) |
| `BESTIARY_VALIDATION_FAILED` | 422 | Statblock validation errors |
| `BESTIARY_RATE_LIMIT` | 429 | Rate limit exceeded |
| `BESTIARY_DUPLICATE_IMPORT` | 409 | Already imported to this campaign |
| `BESTIARY_DUPLICATE_RATING` | 409 | Already rated (use PUT to update) |
| `BESTIARY_SELF_RATING` | 403 | Cannot rate own publication |
| `BESTIARY_SLUG_CONFLICT` | 409 | Slug already taken (auto-resolves) |
| `BESTIARY_ARCHIVED` | 410 | Publication has been archived |
| `BESTIARY_FLAGGED` | 403 | Publication is under review |
| `BESTIARY_PAYLOAD_TOO_LARGE` | 413 | Statblock exceeds 100KB |

---

## 5. Database Migration

Migration file: `XXXXXX_add_bestiary.up.sql`

See [design.md §3](./design.md#3-data-model) for complete table definitions.

Down migration drops all bestiary tables in reverse order:
1. `bestiary_moderation_log`
2. `bestiary_imports`
3. `bestiary_favorites`
4. `bestiary_ratings`
5. `bestiary_publications`

---

## 6. Performance Considerations

### 6.1 Caching

| Data | Cache Strategy | TTL |
|---|---|---|
| Search results | In-memory LRU cache | 60 seconds |
| Trending feed | Background job refresh | 5 minutes |
| Creator profiles | In-memory | 5 minutes |
| Publication detail | None (real-time) | — |
| Rating aggregates | Stored on publication row | Real-time via atomic UPDATE |

### 6.2 Pagination

All list endpoints use keyset pagination (cursor-based) for consistent results:

```
GET /bestiary/newest?cursor=eyJjcmVhdGVkX2F0IjoiMjAyNi0wMy0yMFQxMjowMDowMFoiLCJpZCI6InV1aWQifQ==&per_page=20
```

Cursor is base64-encoded `{created_at, id}` for stable ordering.

### 6.3 Full-Text Search

MySQL FULLTEXT index on (name, description). For larger deployments, consider Elasticsearch/Meilisearch integration (future).

---

## 7. Monitoring & Observability

### 7.1 Metrics

| Metric | Type | Description |
|---|---|---|
| `bestiary_publications_total` | Counter | Total publications created |
| `bestiary_imports_total` | Counter | Total imports across all campaigns |
| `bestiary_ratings_total` | Counter | Total ratings submitted |
| `bestiary_flags_total` | Counter | Total flags submitted |
| `bestiary_search_duration_ms` | Histogram | Search query latency |
| `bestiary_rate_limit_hits` | Counter | Rate limit violations by endpoint |

### 7.2 Alerts

- Flag count spike: >10 flags in 1 hour → notify admins
- Search latency P99 > 500ms → investigate indexing
- Publication rate spike: >50 from single user (bypassed rate limit?) → investigate
