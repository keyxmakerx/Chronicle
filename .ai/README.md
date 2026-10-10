# AI Documentation Index

Context for AI sessions working on Chronicle. Start at the root `CLAUDE.md`
("Where things live" covers open work, GitHub Issues, and when to edit a doc);
this page maps the rest of `.ai/`, present tense throughout.

Read a plugin's or widget's own `.ai.md` when working on it (index below).
Older cross-repo rulings live in the private Cordinator repo's `decisions/`,
which code comments still cite; Cordinator is otherwise a frozen archive.

## Reference files

| File | What it covers |
|------|----------------|
| `architecture.md` | System design, the three-tier extension model, request flow, dependency graph |
| `ui-standard.md` | How Chronicle looks and behaves: what is on a page, opening and closing, motion, look, sign-off. Read before any UI change |
| `conventions.md` | Code patterns with Go/Templ/SQL examples, CI guards, security rules, cross-plugin import discipline |
| `decisions.md` | Architecture Decision Records. Append-only; code cites ADR numbers, so never renumber |
| `tech-stack.md` | Technology versions, configs, and why each was chosen |
| `data-model.md` | Schema overview: every live table and what it holds. The migrations are the source of truth |
| `api-routes.md` | Where to find routes: `internal/wire/routes_snapshot.txt` lists every route (CI-guarded) and `docs/api/openapi.yaml` describes the sync API |
| `glossary.md` | TTRPG and Chronicle terminology |
| `troubleshooting.md` | Non-obvious problems and their fixes, including the test-environment ones |
| `plugin-development.md` | Building WASM extensions |
| `designs/` | Designs. `2026-09-12-build-order.md` and `2026-09-12-header-and-nav.md` are approved and mostly unbuilt; their Apps drawer is superseded by the sidebar signed on #739 (Pinned + folding sections). `2026-09-13-media-renovation.md` is the media plan; its open decisions are tracked in #733. |

`status.md` and `todo.md` are pointers to the issues now. Finished plans,
audits and old designs were deleted; git history keeps them.

## Per-plugin and per-widget docs

Every directory below has an `.ai.md` describing its purpose, files, routes,
business rules and footguns.

- **Plugins** (`internal/plugins/<name>/`): addons, admin, ai_workspace (and
  ai_workspace/aiexport), armory, audit, auth, backup, bestiary, calendar,
  campaigns, designlab, dmscreen, entities, foundry_vtt, maps, media, npcs,
  packages, quests, restore, rolltables, sessions, settings, smtp, syncapi,
  systemstate, timeline, widgetbindings.
  calendar's V5 rebuild (#741) is landing in parts — its `.ai.md` says
  which parts have shipped and which are still open.
- **Widgets** (`internal/widgets/<name>/`): attributes, editor, entity_notes,
  mentions, notes, posts, relations, sky, tags, title.
- **Infrastructure:** `internal/database/`, `internal/extensions/`,
  `internal/systems/` (game systems are external packages installed through
  Admin → Packages, so there is one systems-infrastructure doc, not one per
  system), `internal/websocket/`.
- **Front-end scripts** (`static/js/`): `boot`, `hovercard`, `motion_rest`,
  `notifications`, `sheet_motion`, `shortcuts_help`, `sidebar_tag_filter`, `sidebar_tree`, and
  under `widgets/`: `dynamic_surface`, `entity_posts`, `entity_tooltip`,
  `entity_type_config`, `entity_type_editor`, `groups`, `header_motion`,
  `image_upload`, `layout_editor`, `map_widget`, `permissions`, `quest_board`,
  `relation_graph`, `shop_inventory`, `template_editor`. A plugin's own
  widget scripts sit in `internal/plugins/<slug>/static/js/` with their
  `.ai.md` (timeline: `timeline_viz`, `timeline_widget`).
- **Examples:** `extensions/example-wasm-go/`, `extensions/example-wasm-rust/`.

## Templates

- `templates/module-ai.md.tmpl`: start a new plugin or widget `.ai.md` from this.
- `templates/decision-record.md.tmpl`: the ADR format.

## Other documentation

- `docs/`: operator-facing docs (deployment and upgrades, the restore drill,
  admin diagnostics, the OpenAPI spec, package authoring).
- `tools/`: the CI guard scripts.
- Root `README.md`: the project overview for humans.
