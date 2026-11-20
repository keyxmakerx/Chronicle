// motion_tokens.test.mjs — the hover card, the sidebar Apps fold and the All
// apps drawer time themselves from the shared duration tokens, and menus built
// on keyframes fade under Calm.
//
// These are source checks: input.css holds Tailwind directives, so it cannot be
// loaded into a page as it is. They pin that no raw millisecond figure is left
// in those three moves (Motion speed retimes only tokens), and the shape of the
// Calm rule: it is scoped to Calm, behind a no-reduced-motion query, and the
// device-reduced-motion and Off rules for the three-dots menu are untouched.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, f), 'utf8');

test('the hover card peeks on the micro token', () => {
  const css = read('static/css/paper.css');
  const rule = /\.chc \{[^}]*\}/.exec(css)[0];
  assert.match(rule, /transition: opacity var\(--dur-micro\) var\(--ease-out\), transform var\(--dur-micro\) var\(--ease-out\);/);
  assert.doesNotMatch(rule, /\d+ms/);
  // The card is hidden after its fade, so that wait follows the token too.
  assert.match(read('static/js/hovercard.js'), /motionMs\('--dur-micro', 120\)/);
});

test('the sidebar fold and the drawer read their durations from tokens', () => {
  const nav = read('static/js/sidebar_nav.js');
  assert.doesNotMatch(nav, /\b(FOLD_MS|FADE_OUT_MS)\b/);
  assert.match(nav, /function foldMs\(\) \{ return tokMs\('--dur-large', 280\); \}/);
  assert.match(nav, /function fadeOutMs\(\) \{ return tokMs\('--dur-micro', 120\); \}/);

  const drawer = read('static/js/app_drawer.js');
  assert.doesNotMatch(drawer, /\b(GROW_IN|GROW_OUT|FOLD_MS)\b/);
  assert.match(drawer, /function growIn\(\) \{ return tokMs\('--dur-standard', 200\); \}/);
  assert.match(drawer, /function growOut\(\) \{ return tokMs\('--dur-micro', 120\); \}/);
  assert.match(drawer, /function foldMs\(\) \{ return tokMs\('--dur-standard', 200\); \}/);
  for (const src of [nav, drawer]) {
    // Every animation's duration is a call, never a bare number.
    for (const m of src.matchAll(/duration: ([^,}]+)/g)) {
      if (/^\d+$/.test(m[1].trim())) {
        // The only literal left is the undo toast's seven seconds.
        assert.equal(m[1].trim(), '7000');
      }
    }
  }
});

test('keyframe menus fade under Calm and keep the reduced-motion jump', () => {
  const css = read('static/css/input.css');
  const calm = /@media \(prefers-reduced-motion: no-preference\) \{\s*html\[data-motion="calm"\] :is\(\.ph-more-menu, \.pl-pop, \.pl-chip\):not\(\.czp \*\) \{\s*animation-name: calm-fade;\s*animation-duration: var\(--dur-micro\) !important;/;
  assert.match(css, calm);
  assert.match(css, /@keyframes calm-fade \{ from \{ opacity: 0; \} to \{ opacity: 1; \} \}/);
  // The Calm fade comes after the blanket Calm rule it overrides.
  assert.ok(css.indexOf('@keyframes calm-fade') > css.indexOf('html[data-view-motion="calm"] *:not(.czp *)'));
  // A device that reduces motion, and Off, still show the menu at once.
  assert.match(css, /@media \(prefers-reduced-motion: reduce\) \{ \.ph-more-menu, \.ph-claim \.btn-primary\.is-loading i \{ animation: none !important; \} \}/);
  assert.match(css, /\.nav-rm:not\(\[data-motion="calm"\]\) \.ph-more-menu \{ animation: none !important; \}/);
  assert.doesNotMatch(css, /^\.nav-rm \.ph-more-menu \{ animation: none/m);
  // The blanket device rule is untouched.
  assert.match(css, /@media \(prefers-reduced-motion: reduce\) \{\s*\*, \*::before, \*::after \{\s*animation-duration: 0\.01ms !important;/);
});
