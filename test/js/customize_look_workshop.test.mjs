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

test('Escape anywhere in the workshop and Done put the panel away, except in a text field', () => {
  assert.match(src, /case 'lk-x': closePanel\(\); break;/);
  // On the document, not the widget, so Escape works with focus in the bar or
  // on nothing; a field, a picker or a frame's menu or dialog keeps it first.
  const a = src.indexOf("on(document, 'keydown', function(e){\n      if (e.key !== 'Escape'");
  assert.ok(a > 0, 'Escape is not heard across the workshop');
  const body = src.slice(a, src.indexOf('});', a));
  assert.match(body, /t\.closest\('\[role="dialog"\], \[role="menu"\], \.czs-drawer'\)/);
  assert.match(body, /if \(top\)\{ e\.preventDefault\(\); top\.close\(true\); return; \}/);
  assert.match(body, /t\.matches\('textarea, select, \[contenteditable="true"\], input:not\(\[type="radio"\]\)/);
  assert.ok(body.indexOf('top.close') < body.indexOf("t.matches('textarea"), 'a picker closes before a field keeps Escape');
  assert.match(body, /if \(ui\.panel\)\{ e\.preventDefault\(\); closePanel\(\); \}/);
});

test('the pencil marks are buttons that open their part from the keyboard', () => {
  assert.match(src, /'<button type="button" class="rb' \+ [^;]*data-mark="' \+ sec \+ '" aria-label="' \+ esc\(lab\) \+ '"/);
  assert.match(src, /var lab = 'Change the ' \+ sectionName\(sec\)\.toLowerCase\(\);/);
  assert.match(src, /if \(t\.dataset\.mark\)\{ openMark\(t\.dataset\.mark, e\.detail === 0\); return; \}/);
  // The layer they sit in is no longer hidden from assistive tech.
  assert.match(src, /<div class="rm-brush" id="rm-brush"><\/div>/);
  assert.match(css, /\.czp \.rb\{[^}]*pointer-events:auto;/);
  assert.match(css, /\.czp \.rb:focus-visible\{ outline:2px solid var\(--focus\)/);
  // A redraw keeps focus on the mark, and a kit's frame over the example takes them away.
  assert.match(src, /var m = \$\('\[data-mark="' \+ keep \+ '"\]', brush\); if \(m\) m\.focus/);
  assert.match(src, /if \(ui\.zoom === 'site' && !ui\.focus && !\(kf && kf\.active\(\)\)\)\{/);
});

test('Whole site is out of reach while it is hidden', () => {
  assert.match(src, /\$\('#fz-bar'\)\.inert = true;\n/);
  assert.match(src, /\$\('#fz-bar'\)\.classList\.remove\('on'\); \$\('#fz-bar'\)\.inert = true;/);
  assert.match(src, /\$\('#fz-bar'\)\.classList\.add\('on'\); \$\('#fz-bar'\)\.inert = false;/);
});

test('a pencil mark keeps clear of every word, looking further out when it must', () => {
  assert.match(src, /var words = fzTextBoxes\(\)\.map\(function\(b\)\{ return \{ x:b\.x \* k - 4, y:b\.y \* k - 3, w:b\.w \* k \+ 8, h:b\.h \* k \+ 6 \}; \}\)/);
  assert.match(src, /for \(var d = 6; \(!small \|\| small\.c\) && d <= 48; d \+= 6\)\{/);
});

test('on a phone the dock wraps onto centred rows, every tool 44 tall, and Whole site has a 44 touch area', () => {
  const phone = css.slice(css.indexOf('/* ---------- Phone: the example fills the width'));
  assert.match(phone, /\.czp \.cpe-dock\{ flex-wrap:wrap; justify-content:center; overflow:visible;/);
  assert.match(phone, /\.czp \.cpe-dk\{ min-height:44px;/);
  assert.match(phone, /\.czp \.fz-bar \.czb::after\{[^}]*height:44px;/);
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
