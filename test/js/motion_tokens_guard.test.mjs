// A scripted animation takes its duration from a token (Chronicle.motionMs),
// so the owner's Motion speed retimes it. This fails on a numeric `duration:`
// inside an animate/play/anim call anywhere under static/js or a plugin's
// static/js, outside the files listed below.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..');

// A file stays here only with a reason. The Glow's 1200ms is the standard's
// own figure and is allowed everywhere.
const ALLOW = {
  'static/js/widgets/calendar_view.js': 'signed calendar looks (month roll, day card fold)',
  'static/js/widgets/shop_room.js': 'signed shop room look',
  'static/js/widgets/craft_scene.js': 'signed crafting scene look',
  'static/js/widgets/shop_room_counter.js': 'signed shop room look',
  'internal/plugins/quests/static/js/quest_board.js': 'signed quest board look',
  'internal/plugins/quests/static/js/quest_board_kit.js': 'signed quest board look',
  'internal/plugins/quests/static/js/notice_boards.js': 'signed quest board look',
  'internal/plugins/rolls/static/js/table_dice.js': 'signed dice look',
  'internal/plugins/rumours/static/js/rumours.js': 'signed rulebook/rumour page turn and pop-out',
  'static/js/widgets/dynamic_surface.js': 'the page-transition engine passes its own numbers (0 for none, a fixed fade fallback)',
  'static/js/widgets/player_view.js': 'ring flash tied to its own delay; not an everyday move',
  'internal/plugins/entities/static/js/standing.js': 'not yet moved onto tokens; separate follow-up',
  'internal/plugins/maps/static/js/map_focus.js': 'map crease and focus reveal; not yet moved onto tokens',
  'internal/plugins/maps/static/js/map_links.js': 'not yet moved onto tokens; separate follow-up',
  'internal/plugins/maps/static/js/map_block_picker.js': 'not yet moved onto tokens; separate follow-up',
  'internal/plugins/maps/static/js/where_it_is.js': 'not yet moved onto tokens; separate follow-up',
  'internal/plugins/maps/static/js/map_viewer.js': 'Leaflet flyTo seconds, not the Web Animations API',
};

function walk(dir, out) {
  if (!fs.existsSync(dir)) return;
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) { if (e.name !== 'vendor' && e.name !== 'node_modules') walk(p, out); }
    else if (/\.m?js$/.test(e.name)) out.push(p);
  }
}

function files() {
  const out = [];
  walk(path.join(ROOT, 'static/js'), out);
  const plugins = path.join(ROOT, 'internal/plugins');
  for (const d of fs.readdirSync(plugins)) walk(path.join(plugins, d, 'static/js'), out);
  return out.filter((f) => !f.includes('/vendor/') && !/\.min\.js$/.test(f));
}

// The text of each animate/play/anim call, to its matching close paren.
function calls(src) {
  const found = [];
  const re = /(?:\.animate|\bplay|\banim)\(/g;
  let m;
  while ((m = re.exec(src))) {
    let depth = 1, i = re.lastIndex;
    while (i < src.length && depth > 0) {
      const ch = src[i++];
      if (ch === '(') depth++; else if (ch === ')') depth--;
    }
    found.push({ at: m.index, text: src.slice(m.index, i) });
  }
  return found;
}

function offenders(src) {
  const bad = [];
  for (const c of calls(src)) {
    const hit = /\bduration:\s*(\d+)\b/.exec(c.text);
    if (hit && hit[1] !== '1200' && hit[1] !== '0') bad.push(src.slice(0, c.at).split('\n').length);
  }
  return bad;
}

test('scripted animations take durations from tokens', () => {
  const problems = [];
  for (const f of files()) {
    const rel = path.relative(ROOT, f);
    if (ALLOW[rel]) continue;
    const lines = offenders(fs.readFileSync(f, 'utf8'));
    if (lines.length) problems.push(rel + ': call at line ' + lines.join(', '));
  }
  assert.deepEqual(problems, [], 'use Chronicle.motionMs(token, fallback) for the duration:\n' + problems.join('\n'));
});

test('every allowlisted file exists and still has a reason to be listed', () => {
  for (const rel of Object.keys(ALLOW)) {
    assert.ok(fs.existsSync(path.join(ROOT, rel)), rel + ' is listed but missing');
    assert.ok(ALLOW[rel].length > 8, rel + ' needs a reason');
  }
});

test('Chronicle.motionMs reads ms and s tokens', async () => {
  const src = fs.readFileSync(path.join(ROOT, 'static/js/theme.js'), 'utf8');
  assert.match(src, /Chronicle\.motionMs = function/);
  const vals = { '--a': '120ms', '--b': '0.2s', '--c': 'x' };
  const win = { Chronicle: {}, getComputedStyle: () => ({ getPropertyValue: (n) => vals[n] || '' }), matchMedia: () => ({ matches: false }) };
  const doc = { addEventListener() {}, documentElement: { getAttribute() { return null; }, hasAttribute() { return false; }, classList: { contains() { return false; }, toggle() {} } } };
  new Function('window', 'document', src)(win, doc);
  assert.equal(win.Chronicle.motionMs('--a', 1), 120);
  assert.equal(win.Chronicle.motionMs('--b', 1), 200);
  assert.equal(win.Chronicle.motionMs('--c', 7), 7);
  assert.equal(win.Chronicle.motionMs('--none', 9), 9);
});
