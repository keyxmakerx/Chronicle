# Build a widget

A widget is a JS module that mounts on any element with
`data-widget="<name>"`, fetches its own data and draws itself. `static/js/boot.js`
finds the mounts, on page load and after every htmx swap, and calls your code.

The browser contract is the same everywhere; only how the script ships differs:

| Ship it in | Script lives at | Loaded when | Placed by |
|---|---|---|---|
| A game system package | `widgets/<slug>.js`, declared in `manifest.json` | the campaign uses that system | owner, in the page layout editor |
| A content extension | any `.js`, declared in `manifest.json` | the campaign enables the pack | owner, in the page layout editor |
| Chronicle itself | `static/js/widgets/<name>.js` or a plugin's `static/js/` | first time its mount appears on a page | a templ file that renders the mount |

## The contract

```js
(function () {
  Chronicle.register('hello-widget', {
    init: function (el, config) {
      if (!config.campaignId) { el.textContent = 'No campaign.'; return; }
      Chronicle.apiFetch('/campaigns/' + config.campaignId + '/systems/mygame/data/spells.json')
        .then(function (r) { return r.json(); })
        .then(function (items) {
          el.innerHTML = '<ul>' + items.map(function (s) {
            return '<li>' + Chronicle.escapeHtml(s.name) + '</li>';
          }).join('') + '</ul>';
        });
    },
    destroy: function (el) { el.innerHTML = ''; }
  });
})();
```

- `Chronicle.register(name, { init, destroy })`: `name` must equal the
  mount's `data-widget` value and the manifest `slug`. `destroy` is optional;
  it runs when htmx removes the element.
- `config` holds every other `data-*` attribute, camelCased
  (`data-campaign-id` → `config.campaignId`), with `"true"`/`"false"` and
  numeric strings converted.
- `Chronicle.apiFetch(url, opts)` returns a `fetch` Response. It adds the
  CSRF header on writes and JSON-encodes a plain-object `body`.
- `Chronicle.escapeHtml(s)` before putting any data into `innerHTML`.
- Packaged widgets: write ES5 (`var`, no arrow functions), carry styles in an
  injected `<style>` tag, use CSS variables with fallbacks
  (`var(--bg-primary, #fff)`).

## In a game system package

```json
"widgets": [
  { "slug": "hello-widget", "name": "Hello", "icon": "fa-star",
    "description": "Lists spells.", "script_file": "widgets/hello-widget.js" }
]
```

Served from `/campaigns/:id/systems/<system>/widgets/<slug>.js`, versioned by
the package version. Max 10 widgets. Put shared helpers in `text_renderers`;
they load before widgets. See [game-system.md](game-system.md).

## In a content extension

```json
"contributes": {
  "widgets": [
    { "slug": "dice-roller", "name": "Dice Roller", "icon": "fa-dice-d20",
      "file": "widgets/dice-roller.js",
      "config": [{ "key": "default_dice", "label": "Default Dice", "type": "select",
                   "default": "d20", "options": ["d4", "d20"] }] }
  ]
}
```

Served from `/extensions/<id>/assets/<file>`. `config` declares the `data-*`
options the widget reads. A placed block's saved config arrives as those
attributes, but the layout editor has no form for them yet, so always fall
back to the default. Working example: `extensions/dice-roller/`.
See [content-extension.md](content-extension.md).

## Placing a packaged widget

Owner: Manage > Customize (`/campaigns/:id/customize`) > Layouts > pick a
page template > drag your widget from the **Extensions** section of the
palette. The mount gets `data-campaign-id`, `data-entity-id`, and
`data-editable="true"` for Scribes and up.

Working: the block shows your output on the entity page. Broken: an empty
block; the browser console names the widget that never registered.

## Built into Chronicle

1. Write `static/js/widgets/hello.js` (file name is the snake_case of the
   widget name) with `Chronicle.register('hello', …)`.
2. List it in `coreWidgets` in `internal/app/routes.go`:
   `{Name: "hello", Scripts: []string{"/static/js/widgets/hello.js"}}`.
   A plugin's own widget goes in its `PluginRegistration.Widgets` instead,
   with paths relative to its static mount. Helpers first, the registering
   script last.
3. Render the mount from a templ file, passing endpoints as data:
   `<div data-widget="hello" data-endpoint={ "/campaigns/" + id + "/hello" }></div>`.
4. Server side, if it needs an API: `internal/widgets/<name>/` with handler,
   service, repository and `routes.go` like `internal/widgets/tags/`, wired in
   `internal/app/routes.go`. See [built-in-plugin.md](built-in-plugin.md).

Never add a feature script to `base.templ` or a page template: boot.js loads
it on sight (ADR-063). `tools/check-widget-mounts.sh` fails on a mount with
no load path. Run `make verify` before pushing.
