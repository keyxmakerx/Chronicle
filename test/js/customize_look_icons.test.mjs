// customize_look_icons.test.mjs — the Icons section of customize_look.js
// offers exactly the choices the server accepts, sits between Buttons and
// Motion in Colours and type, is covered by Save/Discard/Reset (SEC_KEYS),
// travels in the Save payload, and is not part of a whole look: starting from
// a look never resets the icon choices.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const go = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'appearance.go'), 'utf8');

// Evaluates a one-statement `var NAME = [...];` out of the widget source.
function list(name) {
  const a = src.indexOf('var ' + name + ' = [');
  const b = src.indexOf('];', a);
  assert.ok(a > 0 && b > a, name + ' marker not found');
  return JSON.parse(vm.runInNewContext(src.slice(a, b + 2) + ';JSON.stringify(' + name + ')'));
}
const serverList = (name) => {
  const m = go.match(new RegExp(name + '\\s*=\\s*\\[\\]string\\{([^}]*)\\}'));
  assert.ok(m, name + ' not found in appearance.go');
  return [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
};

test('the amounts and colours match the server allow-lists, in order, with the signed names', () => {
  const amounts = list('ICON_AMOUNTS');
  assert.deepEqual(amounts.map((x) => x[0]), serverList('AppearanceIconAmounts'));
  assert.deepEqual(amounts.map((x) => x[1]), ['Few', 'Beside names', 'Everywhere']);
  const colours = list('ICON_COLOURS');
  assert.deepEqual(colours.map((x) => x[0]), serverList('AppearanceIconColours'));
  assert.deepEqual(colours.map((x) => x[1]), ['Ink', 'Accent', 'Category']);
  assert.equal(colours[2][2].length, 3, 'Category shows three dots');
});

test('Icons sits after Buttons and before Motion in Colours and type', () => {
  const room = src.match(/\['style', 'Colours and type', 'i-palette', (\[[^\]]*\])/);
  assert.ok(room, 'the style room not found');
  assert.deepEqual(JSON.parse(room[1].replace(/'/g, '"')), ['colours', 'type', 'buttons', 'icons', 'motion']);
  assert.match(src, /\{ id:'icons', name:'Icons', icon:'i-icons', desc:'Chronicle shows few icons\. Bring more back for this campaign here\.' \}/);
});

test('the choices are part of Save, Discard and Reset and of the Save payload', () => {
  assert.match(src, /icons:\['icons\.amount', 'icons\.colour'\]/, 'SEC_KEYS must list both');
  assert.match(src, /icons:clone\(d\.icons\)/, 'toInput must send icons');
  assert.match(src, /d\.icons = Object\.assign\(\{ amount:'few', colour:'ink' \}, d\.icons \|\| \{\}\)/, 'a missing choice reads as the default');
  assert.match(src, /data-k="icons\.colour"/);
  assert.match(src, /opts\('i-amount', 'icons\.amount'/);
  assert.match(src, /fld\('i-amount', 'How many icons'/);
  assert.match(src, /fld\('i-colour', 'Icon colour'/);
  assert.match(src, /One choice colours every icon that stands for a category\./);
});

test('a whole look never carries the icon choices', () => {
  const a = src.indexOf('var LOOK_KEYS = [');
  const b = src.indexOf('];', a);
  const keys = JSON.parse(vm.runInNewContext(src.slice(a, b + 2) + ';JSON.stringify(LOOK_KEYS)'));
  assert.ok(keys.length > 10);
  assert.deepEqual(keys.filter((k) => k.startsWith('icons.')), []);
  const looks = src.slice(src.indexOf('var LOOKS = ['), a);
  assert.ok(!/icons\s*:/.test(looks), 'no look sets icons');
});

test('the preview shows the draft: the choices ride on the preview element', () => {
  assert.match(src, /pv\.dataset\.icons = d\.icons\.amount;/);
  assert.match(src, /pv\.dataset\.icolour = d\.icons\.colour;/);
});
