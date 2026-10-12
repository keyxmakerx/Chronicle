# Build a built-in plugin

A plugin is a feature app compiled into Chronicle: its own tables, service,
routes and pages, under `internal/plugins/<slug>/`. Use this when the feature
needs Go, Chronicle's database or its pages; anything else fits a
[system package](game-system.md), [extension](content-extension.md) or
[API app](api-app.md) without a Chronicle release.

Copy `internal/plugins/rolltables/` (a small complete plugin) and read
[`.ai/architecture.md`](../../.ai/architecture.md) and
[`.ai/conventions.md`](../../.ai/conventions.md) first.

## Files

```
internal/plugins/hello/
  .ai.md            # purpose, files, routes, rules
  embed.go          # PluginSlug + embedded migrations
  model.go
  repository.go     # SQL only, behind an interface
  service.go        # business logic, no Echo types
  handler.go        # bind, call service, render
  routes.go
  *.templ           # pages and fragments (run `make templ`)
  migrations/001_hello.up.sql, 001_hello.down.sql
  static/js/        # optional widget scripts
  *_test.go         # table-driven
```

## Minimal pieces

```go
// embed.go
package hello

import "embed"

const PluginSlug = "hello"

//go:embed migrations/*.sql
var MigrationsFS embed.FS
```

```sql
-- migrations/001_hello.up.sql (idempotent; never edit once shipped)
CREATE TABLE IF NOT EXISTS hello_notes (
  id          INT AUTO_INCREMENT PRIMARY KEY,
  campaign_id CHAR(36) NOT NULL,
  body        TEXT NOT NULL,
  CONSTRAINT fk_hello_notes_campaign
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

```go
// routes.go
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id/hello",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.GET("", h.Show)
	cg.POST("", h.Create, campaigns.RequireRole(campaigns.RoleScribe))
}
```

Roles, lowest to highest: `RolePlayer`, `RoleScribe`, `RoleOwner`. For finer
rules use `campaigns.RequireCapability(func(cc) bool, msg)`, and re-check in
the handler. Return `internal/apperror` errors, never raw DB errors.

## Wire it in

1. **Migrations**: add
   `{Slug: hello.PluginSlug, MigrationsFS: mustSub(hello.MigrationsFS, database.PluginMigrationsSubdir)}`
   to `registeredPlugins()` in `cmd/server/main.go`. Plugin migrations run
   after core ones; a failed one marks the plugin unhealthy.
2. **Routes**: in `internal/app/routes.go`, build repo → service → handler and
   register only when healthy:
   ```go
   helloSvc := hello.NewService(hello.NewRepository(a.DB))
   if a.PluginHealth.IsHealthy(hello.PluginSlug) {
       hello.RegisterRoutes(e, hello.NewHandler(helloSvc), campaignService, authService)
   }
   ```
3. **Static files and widgets** (optional): `a.registerPlugin(PluginRegistration{Slug: hello.PluginSlug, StaticFS: …, Widgets: []PluginWidget{{Name: "hello", Scripts: []string{"js/hello.js"}}}})`.
   Files serve from `/static/plugins/hello/`. See [widget.md](widget.md).
4. **On/off per campaign** (optional): add a row to `builtinAddons` in
   `internal/plugins/addons/service.go` (seeded at startup, no migration), pass
   an `addons.AddonService` into `RegisterRoutes`, and add
   `addons.RequireAddon(addonSvc, "hello")` after `RequireCampaignAccess`.
   Without a row the plugin is always on.
5. **Other plugins**: reach them through service interfaces or adapters in
   `internal/app`, never their repositories. `tools/check-plugin-isolation.sh`
   enforces it.

## Test it

```bash
make templ          # after any .templ change
make test-unit
make test-int-local # repository tests against a local MariaDB
make verify         # build, vet, every tools/check-*.sh guard, unit + JS tests
make dev            # then open http://localhost:8080/campaigns/<id>/hello
```

Working: the page loads for a campaign member. Broken: 404 on the route
means the plugin is unhealthy (the startup log names its migration error); a
failing `make verify` names the guard and the rule.
