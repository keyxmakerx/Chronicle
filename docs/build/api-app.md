# Build an app on the Chronicle API

Any outside program (a VTT module, a Discord bot, an import script) can read
and write a campaign over REST and get live changes over a WebSocket, using a
campaign API key. The [Foundry module](https://github.com/keyxmakerx/Chronicle-Foundry-Module)
is the full-size example; its `API-CONTRACT.md` documents the endpoints it uses.

## Get a key

As the campaign owner:

1. Manage > Game & features (`/campaigns/:id/extensions`): make sure the
   **Sync API** feature is on. With it off, every key is refused with
   `sync_api_disabled`.
2. Settings > API keys (`/campaigns/:id/settings?tab=api-keys`) > create a key.
   Pick permissions: **read** (GETs), **write** (create, update, delete),
   **sync** (`/sync` endpoints). Optional: rate limit (default 300 requests a
   minute), expiry date, IP allowlist (IPs or CIDRs).
3. Copy the key (`chron_…`). It is shown once.

A key belongs to one campaign and acts as the owner who made it: it stops
working if that person stops being owner. Revoke or pause it from the same tab.

## Call the API

Base URL: `https://<your-chronicle>/api/v1/campaigns/<campaignId>`.

```bash
KEY=chron_xxx; BASE=https://chronicle.example.com/api/v1/campaigns/CAMPAIGN_ID

curl -s -H "Authorization: Bearer $KEY" "$BASE/entity-types"

curl -s -H "Authorization: Bearer $KEY" "$BASE/entities?page=1&per_page=20&q=dragon"
# {"data":[…],"total":3,"page":1,"per_page":20}

curl -s -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"name":"Bob the Bard","entity_type_id":3,"fields_data":{}}' "$BASE/entities"
# 201 + the entity
```

- Bodies are JSON with `snake_case` keys, and writes need
  `Content-Type: application/json` (media upload is multipart).
- Updates are partial: a missing key keeps its value, `null` clears it. Send
  `expected_updated_at` to get a 409 instead of overwriting someone's edit.
- Some lists are `{"data": […], "total": N}`, some are bare arrays
  (`/maps`, `/members`, `/notes`). Unwrap both.
- Errors: `{"error": "<code>", "message": "<text>"}`. 401 bad key, 403 missing
  permission, 429 over the rate limit (`Retry-After` header).
- `GET /api/version` needs no key: use it to check the server is up.

Reference: [`docs/api/openapi.yaml`](../api/openapi.yaml) (hand-written).
The exact route list is `internal/wire/routes_snapshot.txt`, which CI keeps current.

## Live changes

Open `wss://<your-chronicle>/ws?token=<key>&client=<your-app-name>`. The key
picks the campaign; no subscribe message. Each message:

```json
{"type":"entity.updated","campaignId":"…","resourceId":"…","senderId":"…","payload":{…},"seq":42}
```

Types include `entity.created|updated|deleted`, `relation.*`, `map.updated`,
`marker.*`, `token.*`, `calendar.event.*`, `calendar.date.advanced`. The
payload is the same object REST returns. Answer pings (most WebSocket
libraries do); the server drops the socket when the key expires or is revoked.

## Catching up after downtime

With a **sync** key held by an owner or co-DM:

```
GET /api/v1/campaigns/:id/sync/changes?since=0&limit=500
→ {"changes":[…],"next":1234,"hasMore":false,"resetRequired":false}
```

Repeat with `since=next` while `hasMore`. Save `next` only after applying the
changes. `resetRequired: true` means the gap was pruned: re-read everything,
then continue from `next`. Changes name the resource; fetch its current state
with the normal GET.

## Test it

Working: `curl` with your key returns the campaign's entity types. Broken:
401 (wrong key or header), 403 `sync_api_disabled` (feature off), 403
`insufficient permissions` (key lacks read/write/sync).

A browser app on another origin also needs that origin allowed in Chronicle's
CORS settings; server-side programs don't.
