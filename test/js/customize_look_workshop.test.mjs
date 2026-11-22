// customize_look_workshop.test.mjs — Look's layout: the example in the middle,
// the open part's settings in a panel on the right (Page layout's own frame),
// floating tools at the foot, Hold to see saved, and a sample beside every
// choice that was only words.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const read = (...p) => readFileSync(join(root, ...p), 'utf8');
const src = read('static', 'js', 'widgets', 'customize_look.js');
const css = read('static', 'css', 'customize.css');
const tpl = read('internal', 'plugins', 'campaigns', 'customize_look.templ');

test('the panel is Page layout\'s slide-out frame and the tools are its dock', () => {
  assert.match(tpl, /class="cpe-insp lk-panel" id="lk-panel"/);
  assert.match(tpl, /class="cpe-dock" id="lk-dock" role="toolbar"/);
  assert.match(tpl, /class="cpe-dock-wrap"/);
  // The panel is put away while closed, so keyboards never land in it.
  assert.match(tpl, /id="lk-panel"[^>]*\binert\b/);
  assert.match(src, /p\.inert = !ui\.panel/);
});

test('the dock holds Looks, light and dark, the page switch and Hold to see saved', () => {
  for (const id of ['lk-looks-btn', 'pt-light', 'pt-dark', 'dp-dash', 'dp-city', 'dp-sheet', 'lk-hold']) assert.ok(tpl.includes('id="' + id + '"'), id);
  assert.match(tpl, /Hold to see saved/);
  // The start-from-a-look strip no longer sits at the top: it lives in the panel.
  assert.ok(tpl.indexOf('id="lk-looks"') > tpl.indexOf('id="lk-panel"'));
  assert.ok(tpl.indexOf('id="lk-row"') > tpl.indexOf('id="lk-panel"'));
});

test('a part opens the panel; the blur lifts for settings that dress the whole site', () => {
  assert.match(src, /var WHOLE = \{ colours:1, type:1 \};/);
  assert.match(src, /var picking = !!sec0 && !WHOLE\[sec0\]/);
  // Pressing a part, choosing a room and landing from Find all open it.
  assert.match(src, /setLooks\(false\);\n\s+openPanel\(\);\n\s+if \(focus\) \$\('#tab-' \+ room\)\.focus\(\);/);
  assert.match(src, /setLooks\(false\);\n\s+syncZoomRow\(\);\n\s+openPanel\(\);/);
});

test('Escape and Done put the panel away, except in a text field', () => {
  assert.match(src, /case 'lk-x': closePanel\(\); break;/);
  assert.match(src, /e\.target\.matches\('textarea, input:not\(\[type="radio"\]\)/);
});

test('Hold to see saved paints the saved look without touching the draft, with Space or Enter too', () => {
  const a = src.indexOf('function paintSaved(){');
  const body = src.slice(a, src.indexOf('function holdStart', a));
  assert.match(body, /draft = saved;/);
  assert.match(body, /finally \{ draft = keep; \}/);
  assert.match(src, /if \(ui\.held\)\{ paintSaved\(\); return; \}/, 'update() must not paint the draft while held');
  assert.match(src, /\(e\.key === ' ' \|\| e\.key === 'Enter'\) && !e\.repeat/);
  assert.match(src, /b\.addEventListener\('blur', holdEnd\)/);
});

test('every choice that was words alone now carries a sample', () => {
  for (const kind of ['hbar', 'hdir', 'hscrim', 'hheight', 'corner', 'glow', 'strength', 'pagename', 'contrast', 'scale', 'iconAmount', 'speed', 'pageLook', 'kit', 'entrance', 'sheet', 'craft', 'builder', 'access']) {
    assert.ok(src.includes('      ' + kind + ':function('), kind + ' needs a sample');
    assert.ok(src.includes("'" + kind + "')"), kind + ' must be used by a choice');
  }
  // Fonts show their name in their own face too.
  assert.match(src, /<span class="tn" style="font-family:' \+ esc\(f\[2\]\)/);
});

test('the dock lifts above the save bar and the sheet on a phone sits at 58%', () => {
  assert.match(css, /\.czp \.lk-panel\{ height:58%; width:auto; \}/);
  assert.match(css, /\.lk-ws\.panel-open \.cpe-dock-wrap\{ bottom:calc\(var\(--lk-hh\) \* \.58 \+ 8px\); \}/);
  // The shared dock rule that lifts it over the save bar is Page layout's own.
  assert.match(read('static', 'css', 'customize_page_editor.css'), /\.czs-stage:has\(> \[data-czs-savebar\]:not\(\[hidden\]\)\) \.cpe-dock-wrap \{ bottom: 76px; \}/);
});
