// customize_grow.test.mjs — what the wall's grow and the part bar rely on:
// a script-driven fly instead of a view transition, focus going in and coming
// back, the loading marks the fly waits for, and the phone's two-row bar.
// The moves themselves run in a browser in test/e2e/customize_grow.spec.mjs.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const read = (...p) => readFileSync(join(root, ...p), 'utf8');
const shell = read('static', 'js', 'widgets', 'customize_shell.js');
const wallCSS = read('static', 'css', 'customize_wall.css');
const shellCSS = read('static', 'css', 'customize_shell.css');
const wallTpl = read('internal', 'plugins', 'campaigns', 'customize_wall.templ');
const shellTpl = read('internal', 'plugins', 'campaigns', 'customize_shell.templ');
const editor = read('static', 'js', 'widgets', 'customize_page_editor.js');
const dice = read('internal', 'plugins', 'rolls', 'static', 'js', 'dice_customize.js');

test('the grow is a script-driven fly, not a view transition started at swap time', () => {
  for (const [name, src] of [['wall templ', wallTpl], ['shell templ', shellTpl], ['shell js', shell], ['wall css', wallCSS]]) {
    assert.ok(!/transition:true/.test(src), name + ' still asks htmx for a view transition');
    assert.ok(!/view-transition|beforeTransition|czs-vt/.test(src), name + ' still holds view-transition code');
  }
  // The copy is laid out at its landing size and played from the tile with a transform.
  assert.match(shell, /function flipFrom\(r, g\)/);
  assert.match(shell, /\{ transform: flipFrom\(from, g\) \}, \{ transform: 'none' \}/);
  assert.match(shell, /cssMs\('--dur-slide', 360\) \* 0\.92/);
  assert.match(shell, /cssMs\('--dur-large', 280\) \* 0\.9/);
  // Calm only fades; Off makes no copy at all.
  assert.match(shell, /if \(lv === 'off'\) return;/);
  assert.match(wallCSS, /\.czw-fly \{[^}]*position: fixed; z-index: var\(--z-raised\)/);
});

test('the fly holds until the workshop has drawn, 1.5 s at most', () => {
  assert.match(shell, /var READY_CAP = 1500;/);
  assert.match(shell, /stage\.querySelector\('\[aria-busy="true"\]'\)/);
  assert.match(shell, /!Chronicle\.isMounted\(w\)/);
  assert.match(wallCSS, /html\[data-czs-fly="grow"\] \.czs-is-workshop \[data-czs-stage\] \{ opacity: 0; animation: none; \}/);
  assert.match(wallCSS, /html\[data-czs-fly="shrink"\] \.czs-is-wall \[data-czw\] \{ opacity: 0; \}/);
  assert.match(read('static', 'js', 'boot.js'), /Chronicle\.isMounted = function \(el\) \{ return mounted\.has\(el\); \};/);
});

test('workshops that load after they mount say so with aria-busy', () => {
  assert.match(editor, /if \(paletteState === 'loading' \|\| \(!PV\.loaded && S\.preview\)\) cv\.setAttribute\('aria-busy', 'true'\);/);
  assert.match(editor, /function fill\(\) \{\n\s+markBusy\(\);/);
  assert.match(dice, /self\.el\.setAttribute\('aria-busy', 'true'\);/);
  assert.equal((dice.match(/self\.el\.removeAttribute\('aria-busy'\)/g) || []).length, 2, 'the dice drop the mark on success and on failure');
});

test('an interrupted move cleans up after itself', () => {
  for (const t of ['htmx:responseError', 'htmx:sendError', 'htmx:timeout', 'htmx:swapError']) assert.ok(shell.includes("'" + t + "'"), t);
  assert.match(shell, /htmx\.trigger\(fly\.elt, 'htmx:abort'\)/);
  assert.match(shell, /var STALE = 10000;/);
  assert.match(shell, /window\.addEventListener\('popstate', function \(\) \{\n\s+if \(fly\) flyClear\(fly\);/);
});

test('focus lands on the workshop\'s name going in and on the tile coming back', () => {
  assert.match(shell, /if \(focusLost\(\)\) focusQuiet\(frame\.querySelector\('\.czs-catname'\)\);/);
  assert.match(shell, /if \(tile\) focusQuiet\(tile\);/);
  assert.match(shell, /el\.focus\(\{ preventScroll: true \}\)/);
  // The part's name takes focus without becoming a tab stop.
  assert.match(shellTpl, /<span class="czs-catname is-static" tabindex="-1">/);
});

test('on a phone the bar has two rows and Find is a 44-square search mark', () => {
  assert.equal((shellTpl.match(/<span class="czs-tabrow">/g) || []).length, 2, 'both bars put their tabs in a row of their own');
  assert.match(shellCSS, /\.czs-tabrow \{ display: contents; \}/);
  const phone = shellCSS.slice(shellCSS.lastIndexOf('@media (max-width: 640px)'));
  assert.match(phone, /\.czs-tabrow \{ display: flex;[^}]*flex: 0 0 100%;[^}]*overflow-x: auto;/);
  assert.match(phone, /\.czs-top \.czs-find \{ position: absolute; top: 0; right: 4px;/);
  assert.match(shellCSS, /\.czs-find, \.czs-top \.czs-find \{[^}]*width: 44px; height: 44px;/);
  // The phone's row of tabs scrolls with the edge fade, and the layout picker joins it.
  assert.match(shell, /var CUE_ROWS = '\.czs-tabs-top, \.czs-tabrow,/);
  assert.match(editor, /\(bar\.querySelector\('\.czs-tabrow'\) \|\| bar\)/);
  assert.match(wallCSS, /\.czw-more::after \{[^}]*height: 44px;/);
});
