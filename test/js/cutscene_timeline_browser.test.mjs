// cutscene_timeline_browser.test.mjs — the cut scene editor's timeline, in
// Chromium: one bar per layer drawn from the rows' own fields, a drag or a
// key writes back into those fields, a faded end opens the row's own
// choices, and a row added or a form swapped in by a save is redrawn. At
// phone width and under reduced motion too, and without a console error.
//
// The form is the editor's fields by name (editor_page.templ; the Go test
// TestEditorHasWhatTheTimelineReads pins that they are still there).
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const plugin = join(root, 'internal/plugins/cutscenes/static');
const TIMELINE = readFileSync(join(plugin, 'js/cutscene_timeline.js'));
const CSS = readFileSync(join(plugin, 'css/cutscenes.css'));

const opts = (name, list, on) => `<select class="csw-select" name="${name}"${name === 'bg_kind' ? ' id="csw-bg-kind"' : name === 'bg_in' ? ' id="csw-bg-in"' : ''}>${list.map(([v, t]) => `<option value="${v}"${v === on ? ' selected' : ''}>${t}</option>`).join('')}</select>`;
const TX_IN = [['', "The style's own"], ['fade', 'Fades in'], ['rise', 'Rises'], ['settle', 'Settles'], ['slide', 'Slides in']];
const TX_OUT = [['', 'Blurs away'], ['fade', 'Fades'], ['sink', 'Sinks'], ['slide', 'Slides away']];
const textRow = (say, at, secs, tin = '', tout = '') => `<div class="csw-item" data-csw-row>
<input name="tx_say" value="${say}"><input name="tx_at" type="number" min="0" max="120" step="0.1" value="${at}">
<input name="tx_seconds" type="number" min="0" max="60" step="0.1" value="${secs}">${opts('tx_in', TX_IN, tin)}${opts('tx_out', TX_OUT, tout)}</div>`;

const FORM = (title = 'The Drowned Bell') => `<form class="csw-form" data-csw-form>
<input name="name" value="Chapter two">
<section class="csw-tl" data-csw-timeline aria-labelledby="csw-tl-h" hidden><h2 class="csw-lab" id="csw-tl-h">Timeline</h2>
<div class="csw-tl-lanes" data-csw-tl-lanes></div><p class="csw-tl-empty" data-csw-tl-empty hidden>Empty</p>
<div class="csw-tl-axis" data-csw-tl-axis aria-hidden="true"></div></section>
${opts('bg_kind', [['black', 'Black'], ['none', 'Nothing']], 'black')}
${opts('bg_in', [['cut', 'At once'], ['fade', 'Fades in'], ['fall-asleep', 'Falling asleep']], 'fade')}
<input id="csw-bg-sec" name="bg_seconds" type="number" min="0.5" max="60" step="0.1" value="2">
<div id="csw-ov-list"><div class="csw-item" data-csw-row>${opts('ov_kind', [['eye', 'Eye'], ['embers', 'Embers'], ['letterbox', 'Film bars']], 'letterbox')}
<input name="ov_at" type="number" min="0" max="120" step="0.1" value="0"><input name="ov_seconds" type="number" min="0.5" max="60" step="0.1" value="1"></div></div>
<div id="csw-tx-list">${textRow('Chapter two', 1, 4, 'rise', 'fade')}${textRow(title, 2, 4, 'settle')}${textRow('Saltreach', 6, 0, 'slide')}</div>
</form>`;

const PAGE = `<!doctype html><html><head><meta charset="utf-8"><style>
:root{--color-accent:#4f46e5;--color-accent-rgb:79 70 229;--color-border:#e5e7eb;--color-border-light:#eee;--color-bg-secondary:#f3f4f6;--color-card-bg:#fff;--color-text-muted:#6b7280;--color-text-secondary:#4b5563;--color-text-primary:#111827}
body{margin:0;padding:16px;font:14px system-ui}</style><style>${CSS}</style></head><body>
<div class="csw" id="mount">${FORM()}</div>
<script src="/timeline.js"></script>
<script>window.tl = ChronicleCutsceneTimeline.mount(document.getElementById('mount'));</script></body></html>`;

async function open(t, viewport, reducedMotion) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport, reducedMotion: reducedMotion ? 'reduce' : 'no-preference' });
  const pg = await ctx.newPage();
  const errors = [];
  pg.on('pageerror', (e) => errors.push(String(e)));
  pg.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/timeline.js') return route.fulfill({ contentType: 'text/javascript', body: TIMELINE });
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await pg.goto('http://t.test/');
  return { pg, errors };
}

const bars = (pg) => pg.$$eval('.csw-tl-lane', (ls) => ls.map((l) => {
  const bar = l.querySelector('.csw-tl-bar');
  const track = l.querySelector('.csw-tl-track').getBoundingClientRect();
  const r = bar.getBoundingClientRect();
  return {
    name: l.querySelector('.csw-tl-name').textContent,
    left: Math.round((r.left - track.left) / track.width * 1000) / 10,
    right: Math.round((r.right - track.left) / track.width * 1000) / 10,
    tags: [...bar.querySelectorAll('.csw-tl-end')].map((e) => e.textContent),
    open: bar.classList.contains('is-open')
  };
}));
const val = (pg, sel) => pg.$eval(sel, (e) => e.value);
// A drag lands within a tenth of a second of where the pointer went: the
// pointer moves in whole pixels and the field keeps tenths.
const about = async (pg, sel, want) => {
  const got = parseFloat(await val(pg, sel));
  assert.ok(Math.abs(got - want) <= 0.11, `${sel} is ${got}, want about ${want}`);
};

for (const [name, viewport] of [['computer', { width: 1100, height: 800 }], ['phone', { width: 390, height: 844 }]]) {
  test(`${name}: one bar per layer, placed from the rows' own numbers`, { skip }, async (t) => {
    const { pg, errors } = await open(t, viewport);
    await pg.waitForSelector('[data-csw-timeline]:not([hidden]) .csw-tl-bar');
    const got = await bars(pg);
    assert.deepEqual(got.map((b) => b.name), ['Background', 'Film bars', 'Chapter two', 'The Drowned Bell', 'Saltreach']);
    // Last change: Saltreach arrives 6 + 1.4; one second of room rounds the
    // timeline up to 9 s, ticked every 3.
    const ticks = await pg.$$eval('.csw-tl-tick', (ts) => ts.map((x) => x.textContent));
    assert.deepEqual(ticks, ['0 s', '3 s', '6 s', '9 s']);
    const near = (a, b) => Math.abs(a - b) < 1.5;
    assert.ok(near(got[2].left, 1 / 9 * 100) && near(got[2].right, (1 + 4 + 1.1) / 9 * 100), `Chapter two spans 1 to 6.1 s: ${JSON.stringify(got[2])}`);
    assert.ok(got[0].open && got[1].open && got[4].open, 'the background, the film bars and words with no end stay to the end');
    assert.ok(!got[2].open && !got[3].open, 'words with a time on screen end');
    if (name === 'computer') {
      assert.deepEqual(got[2].tags, ['rise', 'fade']);
      assert.deepEqual(got[3].tags, ['settle', 'blur']);
      assert.deepEqual(got[4].tags, ['slide'], 'words that stay to the end have no leaving');
      assert.deepEqual(got[1].tags, ['slide'], 'film bars show their own arrival');
    }
    const wide = await pg.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
    assert.ok(wide, 'nothing pushes the page sideways');
    assert.deepEqual(errors, []);
  });
}

test('dragging a bar moves its start in the row, and its right end stretches its time', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 1100, height: 800 });
  await pg.waitForSelector('.csw-tl-bar');
  const track = await pg.$eval('.csw-tl-lane:nth-child(3) .csw-tl-track', (e) => { const r = e.getBoundingClientRect(); return { x: r.left, w: r.width, y: r.top + r.height / 2 }; });
  const per = track.w / 9;
  // Grab Chapter two in its solid middle (3 s) and drag one second right.
  await pg.mouse.move(track.x + 3 * per, track.y);
  await pg.mouse.down();
  await pg.mouse.move(track.x + 3.5 * per, track.y, { steps: 3 });
  await pg.mouse.move(track.x + 4 * per, track.y, { steps: 3 });
  await pg.mouse.up();
  await about(pg, '#csw-tx-list [data-csw-row]:nth-child(1) [name="tx_at"]', 2);
  // Its length grip sits where its time on screen ends; pull it a second on.
  const at = parseFloat(await val(pg, '#csw-tx-list [data-csw-row]:nth-child(1) [name="tx_at"]'));
  const grip = await pg.$eval('.csw-tl-lane:nth-child(3) .csw-tl-grip-len', (e) => { const r = e.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }; });
  assert.ok(Math.abs(grip.x - (track.x + (at + 4) * per)) < 8, 'the grip is at the end of its time on screen');
  await pg.mouse.move(grip.x, grip.y);
  await pg.mouse.down();
  await pg.mouse.move(grip.x + per, grip.y, { steps: 4 });
  await pg.mouse.up();
  await about(pg, '#csw-tx-list [data-csw-row]:nth-child(1) [name="tx_seconds"]', 5);
  // The background's build grip stretches Takes.
  const bg = await pg.$eval('.csw-tl-lane:nth-child(1) .csw-tl-grip-build', (e) => { const r = e.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }; });
  await pg.mouse.move(bg.x, bg.y);
  await pg.mouse.down();
  await pg.mouse.move(bg.x + per, bg.y, { steps: 4 });
  await pg.mouse.up();
  await about(pg, '#csw-bg-sec', 3);
  // A drag past zero stops at the field's own limit.
  const t0 = await pg.$eval('.csw-tl-lane:nth-child(2) .csw-tl-track', (e) => { const r = e.getBoundingClientRect(); return { x: r.left, y: r.top + r.height / 2 }; });
  await pg.mouse.move(t0.x + 2 * per, t0.y);
  await pg.mouse.down();
  await pg.mouse.move(t0.x - 3 * per, t0.y, { steps: 4 });
  await pg.mouse.up();
  assert.equal(await val(pg, '#csw-ov-list [name="ov_at"]'), '0');
  assert.deepEqual(errors, []);
});

test('the keyboard moves a focused bar, and a faded end opens the row\'s own choices', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 1100, height: 800 });
  await pg.waitForSelector('.csw-tl-bar');
  await pg.focus('.csw-tl-lane:nth-child(5) .csw-tl-bar');
  await pg.keyboard.press('ArrowRight');
  await pg.keyboard.press('Shift+ArrowRight');
  assert.equal(await val(pg, '#csw-tx-list [data-csw-row]:nth-child(3) [name="tx_at"]'), '7.1');
  const focused = await pg.evaluate(() => document.activeElement.closest('.csw-tl-lane')?.querySelector('.csw-tl-name').textContent);
  assert.equal(focused, 'Saltreach', 'focus stays on the bar after it is redrawn');
  assert.match(await pg.$eval('.csw-tl-lane:nth-child(5) .csw-tl-bar', (e) => e.getAttribute('aria-valuetext')), /starts at 7\.1 s, stays to the end/);

  await pg.click('.csw-tl-lane:nth-child(4) .csw-tl-out');
  const menu = pg.locator('.csw-tl-menu[role="menu"]');
  await menu.waitFor();
  const items = await menu.locator('[role="menuitemradio"]').allTextContents();
  assert.deepEqual(items, ['Blurs away', 'Fades', 'Sinks', 'Slides away'], 'the menu is the row\'s own list');
  assert.equal(await pg.evaluate(() => document.activeElement.textContent), 'Blurs away', 'focus starts on the current choice');
  await pg.keyboard.press('ArrowDown');
  await pg.keyboard.press('ArrowDown');
  await pg.keyboard.press('Enter');
  assert.equal(await val(pg, '#csw-tx-list [data-csw-row]:nth-child(2) [name="tx_out"]'), 'sink');
  assert.equal(await menu.count(), 0, 'the menu closes on a pick');
  assert.equal(await pg.$eval('.csw-tl-lane:nth-child(4) .csw-tl-out', (e) => e.textContent), 'sink');
  assert.equal(await pg.evaluate(() => document.activeElement.classList.contains('csw-tl-out')), true, 'focus goes back to the end that opened it');

  await pg.click('.csw-tl-lane:nth-child(1) .csw-tl-in');
  await menu.waitFor();
  await pg.keyboard.press('Escape');
  assert.equal(await menu.count(), 0, 'Esc closes the menu');
  assert.equal(await val(pg, '#csw-bg-in'), 'fade', 'and changes nothing');
  await pg.click('.csw-tl-lane:nth-child(1) .csw-tl-in');
  await menu.locator('text=At once').click();
  assert.equal(await val(pg, '#csw-bg-in'), 'cut');
  const bgTags = await pg.$$eval('.csw-tl-lane:nth-child(1) .csw-tl-end', (es) => es.map((e) => e.textContent));
  assert.deepEqual(bgTags, ['cut']);
  assert.equal(await pg.$$eval('.csw-tl-lane:nth-child(1) .csw-tl-grip-build', (g) => g.length), 0, 'a background there at once has nothing to stretch');
  assert.deepEqual(errors, []);
});

test('typing in a row, adding a row and a form swapped in by a save all redraw', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 1100, height: 800 }, true);
  await pg.waitForSelector('.csw-tl-bar');
  await pg.fill('#csw-tx-list [data-csw-row]:nth-child(1) [name="tx_say"]', 'Chapter three');
  assert.equal(await pg.$eval('.csw-tl-lane:nth-child(3) .csw-tl-name', (e) => e.textContent), 'Chapter three');
  await pg.evaluate(() => {
    const list = document.getElementById('csw-tx-list');
    list.appendChild(list.firstElementChild.cloneNode(true));
  });
  await pg.waitForFunction(() => document.querySelectorAll('.csw-tl-lane').length === 6);
  await pg.selectOption('#csw-bg-kind', 'none');
  await pg.waitForFunction(() => document.querySelector('.csw-tl-name').textContent !== 'Background');
  // A save swaps the whole form; the new one gets its own timeline.
  await pg.evaluate((html) => {
    const old = document.querySelector('form[data-csw-form]');
    const box = document.createElement('div');
    box.innerHTML = html;
    old.replaceWith(box.firstElementChild);
  }, FORM('Saved title'));
  await pg.waitForFunction(() => [...document.querySelectorAll('.csw-tl-name')].some((e) => e.textContent === 'Saved title'));
  assert.equal(await pg.$$eval('[data-csw-timeline]', (b) => b.filter((x) => !x.hidden).length), 1);
  // Reduced motion: the menu appears without moving.
  await pg.click('.csw-tl-lane:nth-child(3) .csw-tl-in');
  const anim = await pg.$eval('.csw-tl-menu', (m) => getComputedStyle(m).animationName);
  assert.equal(anim, 'none');
  assert.deepEqual(errors, []);
});
