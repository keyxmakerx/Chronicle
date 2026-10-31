# AI Documentation Index

Start at the root `CLAUDE.md` ("Where things live" covers open work and when to
edit a doc). Read a plugin's or widget's own `.ai.md` when working on it.
Older cross-repo rulings are in the frozen Cordinator repo's `decisions/`,
which code comments still cite.

## Reference files

| File | What it covers |
|------|----------------|
| `architecture.md` | System design, three-tier extension model, request flow, dependency graph |
| `ui-standard.md` | How Chronicle looks and behaves: what is on a page, opening and closing, motion, look, sign-off. Read before any UI change |
| `conventions.md` | Code patterns, CI guards, security rules, cross-plugin import rules |
| `decisions.md` | Architecture Decision Records. Append-only; code cites ADR numbers, so never renumber |
| `tech-stack.md` | Technology versions and configs |
| `data-model.md` | Schema overview; the migrations are the source of truth |
| `api-routes.md` | Where routes are listed (`internal/wire/routes_snapshot.txt`, `docs/api/openapi.yaml`) |
| `glossary.md` | TTRPG and Chronicle terminology |
| `troubleshooting.md` | Non-obvious problems and fixes, including test-environment ones |
| `plugin-development.md` | Building WASM extensions |
| `designs/` | `2026-09-13-media-renovation.md`: the media plan; slices and open decisions in #733 |

`status.md` and `todo.md` are pointers to the issues.

## Per-plugin and per-widget docs

Each has an `.ai.md` (purpose, files, routes, rules, footguns).

- **Plugins** (`internal/plugins/<name>/`): addons, admin, ai_workspace (and
  ai_workspace/aiexport), armory, audit, auth, backup, bestiary, calendar,
  campaigns, crafting, designlab, dmscreen, encounters, entities, fieldrules, foundry_vtt, homebrew, maps, media, npcs,
  packages, pageblocks, quests, restore, rolltables, rumours, sessions, settings, smtp, syncapi,
  systemstate, timeline, trackers, vault_import, widgetbindings.
  calendar's V5 rebuild (#741) is landing in parts — its `.ai.md` says
  which parts have shipped and which are still open.
- **Widgets** (`internal/widgets/<name>/`): attributes, editor, entity_notes,
  mentions, notes, posts, relations, sky, tags, title.
- **Infrastructure:** `internal/database/`, `internal/extensions/`,
  `internal/systems/` (one doc for all game systems, which are external
  packages), `internal/websocket/`.
- **Front-end scripts** (`static/js/`): `boot`, `hovercard`, `motion_rest`,
  `notifications`, `sheet_motion`, `shortcuts_help`, `sidebar_tag_filter`, `sidebar_tree`, and
  under `widgets/`: `dynamic_surface`, `entity_posts`, `entity_tooltip`,
  `entity_type_config`, `entity_type_editor`, `groups`, `header_motion`,
  `header_sky`, `image_upload`, `layout_editor`, `permissions`,
  `relation_graph`, `shop_inventory`, `template_editor`. A plugin's own
  widget scripts sit in `internal/plugins/<slug>/static/js/` with their
  `.ai.md` (timeline: `timeline_viz`, `timeline_widget`; quests: `quest_board`;
  pageblocks: `page_blocks`; encounters: `encounters`; maps: `map_widget`).
- **Examples:** `extensions/example-wasm-go/`, `extensions/example-wasm-rust/`.

## Templates

- `templates/module-ai.md.tmpl`: start a new plugin or widget `.ai.md` from this.
- `templates/decision-record.md.tmpl`: the ADR format.

## Other documentation

- `docs/`: operator docs (deployment, restore drill, diagnostics), the OpenAPI spec, package authoring.
- `tools/`: the CI guard scripts.
- Root `README.md`: the project overview for humans.
