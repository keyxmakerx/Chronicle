// topbar_tray_browser.test.mjs — the header's widget slot never hides a
// widget with no way to reach it. Below md the first widget sits in the bar
// and the "+N" button opens the rest; at any width, a slot too narrow for even
// the first widget moves them all to the tray and the button shows, so a
// tablet-wide window with the sidebar open still has the search box.
//
// Runs the slot's rules from static/css/input.css in a real Chromium against
// markup shaped like app.templ's topbarCenterContent. Needs playwright-core
// and a Chromium (see _browser_env.mjs).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const INPUT = readFileSync(join(root, 'static/css/input.css'), 'utf8');

// The slot's own block, from its comment to the end of its container query.
function slotCSS() {
  const start = INPUT.indexOf('.tb-slot { container: tbslot');
  const end = INPUT.indexOf('}\n}', INPUT.indexOf('@container tbslot', start)) + 3;
  assert.ok(start > 0 && end > start, 'the header slot rules are in input.css');
  return INPUT.slice(start, end);
}

// The Tailwind utilities the markup leans on, as Tailwind writes them.
const UTILITIES = '.hidden{display:none}.flex{display:flex}@media (min-width:768px){.md\\:flex{display:flex}}';

function page(widgets) {
  const first = '<div class="tb-first flex"><button id="w0" data-search-open>Search the campaign</button></div>';
  const rest = widgets.slice(1).map((w, i) => '<div class="tb-rest hidden md:flex"><a id="w' + (i + 1) + '">' + w + '</a></div>').join('');
  const solo = widgets.length === 1 ? ' tb-solo' : '';
  return '<div id="topbar-content" class="tb-slot" style="width:var(--slot)"><div class="flex">' + first + rest +
    '<button id="more" class="tb-more' + solo + '"><span class="tb-n">+' + (widgets.length - 1) + '</span><span class="tb-n-all">+' + widgets.length + '</span></button>' +
    '<div id="topbar-tray" class="flex" style="display:none"><div class="tb-first-copy"><button>Search</button></div></div>' +
    '</div></div>';
}

async function shown(t, { width, slot, widgets }) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width, height: 400 } });
  await p.setContent('<!doctype html><html><head><style>' + UTILITIES + slotCSS() + ':root{--slot:' + slot + '}</style></head><body>' + page(widgets) + '</body></html>');
  return p.evaluate(() => {
    const vis = (id) => { const e = document.getElementById(id); return !!e && e.getBoundingClientRect().width > 0; };
    const more = document.getElementById('more');
    const label = [...more.children].find((c) => c.getBoundingClientRect().width > 0);
    return { first: vis('w0'), rest: vis('w1'), more: vis('more'), count: label ? label.textContent : '' };
  });
}

test('desktop with room: every widget in the bar, no button', { skip }, async (t) => {
  assert.deepEqual(await shown(t, { width: 1280, slot: '40rem', widgets: ['search', 'links'] }),
    { first: true, rest: true, more: false, count: '' });
});

test('tablet width with the sidebar open: the slot is too narrow, so the button holds them all', { skip }, async (t) => {
  assert.deepEqual(await shown(t, { width: 820, slot: '6rem', widgets: ['search', 'links'] }),
    { first: false, rest: false, more: true, count: '+2' });
});

test('a lone search box in a narrow slot at tablet width still gets the button', { skip }, async (t) => {
  assert.deepEqual(await shown(t, { width: 820, slot: '6rem', widgets: ['search'] }),
    { first: false, rest: false, more: true, count: '+1' });
});

test('phone with room: the first widget in the bar, the button for the rest', { skip }, async (t) => {
  assert.deepEqual(await shown(t, { width: 390, slot: '12rem', widgets: ['search', 'links'] }),
    { first: true, rest: false, more: true, count: '+1' });
});

test('phone with room and one widget: no button', { skip }, async (t) => {
  assert.deepEqual(await shown(t, { width: 390, slot: '12rem', widgets: ['search'] }),
    { first: true, rest: false, more: false, count: '' });
});
