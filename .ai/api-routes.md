# API Routes

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Where to find the route list and how to trace a route to its    -->
<!--          handler. Not itself a route table.                             -->
<!-- Update: When auth tiers or route groups change.                         -->
<!-- ====================================================================== -->

Chronicle registers its Echo routes across the web UI, the admin panel, and
the REST API. This file does not enumerate them — a hand-maintained table
drifts from the code within weeks. Instead:

- **The full, current route list** is `internal/wire/routes_snapshot.txt`,
  a `(method, resolved path, auth class, handler, middleware chain)` inventory
  recorded from the real route registration and enforced by `internal/wire/wire_contract_test.go` in CI
  (see `.ai/conventions.md` §"CI tenet-enforcement guards" for how to
  regenerate it after adding or removing a route).
- **The public REST API** (for external clients like the Foundry module) is
  described in `docs/api/openapi.yaml`.

## Finding a route's handler

1. Grep `internal/wire/routes_snapshot.txt` for the full path or method. The
   line names the handler function (`plugins/<slug>.(*Handler).Name`), its auth
   class and the middleware chain in execution order.
2. Open that plugin's `routes.go` (or, for core routes, `internal/app/routes.go`)
   and find the registration if you need the source.
3. The handler lives in the same plugin's `handler.go`. From there, `service.go`
   has the business logic and `repository.go` has the SQL.

## Route groups and auth tiers

Chronicle exposes four distinct auth surfaces — session-cookie web UI,
per-campaign-token legacy public API, session-or-Bearer syncapi JSON API, and
admin-session — detailed in `.ai/conventions.md` §"Auth surfaces — four
canonical shapes". That section is the authoritative reference for which
middleware chain applies to which route group; this file only points at it so
the two don't drift.

Route groups follow the plugin structure: each plugin's `routes.go` registers
its own paths, `internal/app/routes.go` is the top-level registrar that wires
every plugin in, and `internal/plugins/syncapi/routes.go` registers the
`/api/v1/*` REST surface separately from the web UI.

