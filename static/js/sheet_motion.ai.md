# sheet_motion.js

The motion engine for a game system's character sheet (`window.Chronicle.sheetMotion`).

**What it does.** For each `[data-sheet]` root it sets `data-sheet-style` (from
`html[data-cz-sheet]`, else `modern`), loads that style's Google Fonts once,
bakes the paper grain (and the rune stone or brass iron) into a bitmap, and
moves the sheet's parts: panels pulled from behind a part
(`[data-sheet-open]` -> `<template data-sheet-panel>`), folds
(`<details data-sheet-fold>`), and changed values (`land(el, oldText)`).
It does nothing on a page with no `[data-sheet]`.

**Where the look lives.** Shared pieces: `static/css/paper.css`. The twelve
styles: `static/css/sheet_styles.css`. The move contract and per-style presets:
`static/css/sheet_motion.css`. The attribute contract for system authors is in
`docs/system-package-rendering.md` ("Sheet styles and motion").

**Rules it keeps**
- Everything is scoped under `[data-sheet]`; `data-move` elsewhere (the give box)
  is untouched.
- Timing is never written here: moves read the site's `--dur-*` tokens, and the
  engine reads the real transition time with `tms()` before timing a timer.
- Calm and Off come from `html[data-motion]`, `data-view-motion="calm"` and
  `data-cz-reduce`: Calm fades and opens folds at once, Off jumps to the end.
- Only transform, opacity and clip-path move; a moving layer carries
  `.is-moving` (`will-change`) only while it moves.

**Tested.** `test/js/sheet_motion.test.mjs` covers the panel state machine,
Calm/Off, effect dispatch by `--change-move`, style resolution and the
no-sheet case through the exported pure helpers.
