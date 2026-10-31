# API Routes

This file does not list routes; a hand-kept table drifts. Where to look:

- **Every route:** `internal/wire/routes_snapshot.txt`, one line per
  `(method, resolved path, auth class, handler, middleware chain)`, recorded from
  the real registration and enforced in CI by `internal/wire/wire_contract_test.go`.
  Regeneration: `.ai/conventions.md` §"CI tenet-enforcement guards".
- **Public sync API** (Foundry module and other clients): `docs/api/openapi.yaml`.
- **Auth surfaces and which middleware chain each uses:**
  `.ai/conventions.md` §"Auth surfaces".

## Tracing a route to its handler

1. Grep the snapshot for the path. The line names the handler
   (`plugins/<slug>.(*Handler).Name`), auth class and middleware order.
2. Registration: the plugin's `routes.go`; core routes are in
   `internal/app/routes.go`, which wires every plugin in.
   `internal/plugins/syncapi/routes.go` registers `/api/v1/*` separately from the web UI.
3. Handler in `handler.go`, business logic in `service.go`, SQL in `repository.go`.
