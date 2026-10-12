# Building for Chronicle

Short guides to every way you can add to Chronicle. Each says what the thing
is, the files it needs, a minimal working example, and how to install and test it.

| You want to… | Build a | Needs Go? | Guide |
|---|---|---|---|
| Add a game's rules, reference data, character fields and rulebook | Game system package | No | [game-system.md](game-system.md) |
| Add an interactive block to pages (a tracker, a stat block, a roller) | Widget | No (packaged) / Yes (built-in) | [widget.md](widget.md) |
| Add entity types, tags or widgets to campaigns as an installable pack | Content extension | No | [content-extension.md](content-extension.md) |
| Run your own server-side logic (Rust, Go, any WASM language) | WASM plugin | No | [wasm-plugin.md](wasm-plugin.md) |
| Connect an outside tool (a VTT, a bot, a script) | App on the REST/WebSocket API | No | [api-app.md](api-app.md) |
| Add a whole feature to Chronicle itself | Built-in plugin | Yes | [built-in-plugin.md](built-in-plugin.md) |

Who installs what:

- **Site admin**: game system packages (Admin > Game systems & modules) and
  content extensions (Admin > Extensions). Both ask for your password again.
- **Campaign owner**: turns a game system or content pack on for one campaign,
  creates API keys, and places widgets on pages.
- **Contributor**: built-in plugins and widgets ship in a Chronicle release.

Real examples to copy:

- [Chronicle-Draw-Steel](https://github.com/keyxmakerx/Chronicle-Draw-Steel): a full system package with widgets and a rulebook.
- [Chronicle-DnD-5.5e](https://github.com/keyxmakerx/Chronicle-DnD-5.5e): a data-only system package.
- [Chronicle-Foundry-Module](https://github.com/keyxmakerx/Chronicle-Foundry-Module): an app built on the API.
- `extensions/` in this repo: a widget extension (`dice-roller`) and WASM plugins (`example-wasm-go`, `example-wasm-rust`).
