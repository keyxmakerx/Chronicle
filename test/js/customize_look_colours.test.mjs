// customize_look_colours.test.mjs — the Look editor names the three campaign
// colours for what they paint, everywhere it names them, and the example
// site's soft focus draws its blur in screen pixels so Chromium has no
// mirrored copies to show.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const css = readFileSync(join(root, 'static', 'css', 'customize.css'), 'utf8');
const pages = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'customize_pages.go'), 'utf8');
const save = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'appearance.go'), 'utf8');

test('the three colours are named for what they paint, each with its line', () => {
  assert.match(src, /accent:\{ k:'colours\.accent', list:ACCENT_PRESETS, label:'Menus and links' \}/);
  assert.match(src, /s1: +\{ k:'colours\.s1', list:PRESETS, follow:true, label:'Headers and events' \}/);
  assert.match(src, /s2: +\{ k:'colours\.s2', list:PRESETS, follow:true, label:'Tags and marks' \}/);
  assert.match(src, /crow\('accent', GROUPS\.accent\.label, 'The menu highlight, links, buttons, anything selected'/);
  assert.match(src, /crow\('s1', GROUPS\.s1\.label, 'Page title rules, and calendar events without their own colour'/);
  assert.match(src, /crow\('s2', GROUPS\.s2\.label, 'Timeline marks and connections'/);
});

test('the old names and the long explaining paragraph are gone', () => {
  for (const old of [/chrome accent/i, /Surface A/, /Surface B/, /surface colours/i, /Chrome is Chronicle/]) {
    assert.doesNotMatch(src, old, String(old));
  }
  assert.doesNotMatch(save, /chrome accent|"surface [AB]"/, 'a save error still names the old colours');
});

test('Find a setting reaches Colours by what the colours paint', () => {
  const line = pages.split('\n').find((l) => l.includes('{"colours", "Colours",'));
  for (const w of ['menus', 'links', 'headers', 'events', 'tags', 'marks']) assert.ok(line.includes(w), w);
});

test('the colour rows put a hairline between them and fold on a phone', () => {
  assert.match(css, /\.czp \.crow \+ \.crow\{ border-top:1px solid var\(--line-2\); \}/);
  // The phone rule that folds every .fld comes after the row's own columns.
  assert.ok(css.indexOf('.czp .crow{') < css.lastIndexOf('.czp .fld{ grid-template-columns:minmax(0, 1fr); }'));
});

test('the blur layer undoes the site scale and its clip is in screen pixels', () => {
  const rule = css.slice(css.indexOf('.czp .rm-blur{'), css.indexOf('}', css.indexOf('.czp .rm-blur{')));
  assert.match(rule, /transform:scale\(calc\(1 \/ var\(--k, \.6\)\)\)/);
  assert.match(rule, /width:calc\(880px \* var\(--k, \.6\)\)/);
  const a = src.indexOf('function roomClip(');
  const b = src.indexOf('\n    }', a) + 6;
  const roomClip = vm.runInNewContext('(' + src.slice(a, b).replace('function roomClip', 'function') + ')');
  const d = roomClip([{ x: 100, y: 100, w: 200, h: 100 }], 0.5);
  const nums = d.match(/[\d.]+/g).map(Number);
  assert.equal(Math.max(...nums), 440, 'the clip reaches the scaled right edge, 880 x 0.5');
  assert.ok(nums.includes(48) && nums.includes(152), 'the hole edges are scaled with the padding');
});
