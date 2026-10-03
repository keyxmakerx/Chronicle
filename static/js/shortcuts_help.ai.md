# shortcuts_help.js

## Purpose

The "?" keyboard-shortcuts overlay: a modal card listing the app's shortcuts.
It is a static list; it binds none of the shortcuts it documents.

## Loading

A `<script defer>` tag in `layouts/base.templ`, after `command_palette.js`.
No `data-widget`; it self-initialises as an IIFE and exposes no API.

## Behaviour

- `?` (without Ctrl or Cmd) toggles the overlay, unless focus is in an
  `input`, `textarea`, `select` or contenteditable element.
- `Escape`, the Close button, or a click on the backdrop closes it.
- The command palette's "Keyboard Shortcuts" entry opens it by dispatching a
  synthetic `keydown` with `key: '?'` on `document`, so the keydown path must
  keep working for that entry.
- The modifier label is `⌘` on Mac-like platforms, `Ctrl` elsewhere
  (`navigator.platform`).

## The listed shortcuts and where each is bound

| Shown | Bound in |
|---|---|
| Mod+K | `search_modal.js` |
| Mod+Shift+P | `command_palette.js` |
| Mod+N, Mod+E, Mod+S | `keyboard_shortcuts.js` |
| Mod+Shift+N | `quick_capture.js` (also offered by the palette) |
| `/` slash commands | `widgets/editor_slash.js` |
| `?` | this file |

When a shortcut is added, removed or rebound in those files, edit the
`shortcuts` array here too; nothing derives it.

## Events

Listens to `keydown` on `document`; emits nothing.

## Gotchas

- The overlay uses inline styles for its backdrop (`z-index: 9999`) and the
  `card` and `btn-secondary` classes for the panel.
- It has no focus trap and does not restore focus on close.
- Text is hard-coded English, not localised.
