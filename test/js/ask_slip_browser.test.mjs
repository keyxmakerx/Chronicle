// ask_slip_browser.test.mjs — the shared "Ask in place" slip.
//
// Runs the slip's part of static/js/theme.js in a real Chromium with its rules from
// input.css, on its own and inside the Edit name fold (page_name_fold.js),
// which is one of the boxes that use it.
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
const read = (f) => readFileSync(join(root, f), 'utf8');
// The slip is the last part of theme.js, which every page loads.
const THEME = read('static/js/theme.js');
const SLIP_JS = THEME.slice(THEME.indexOf('/**\n * The "Ask in place" slip'));

// The slip's own rules, and the motion tokens they use.
const CSS = ':root{--dur-slide:360ms;--ease-slide:ease;--dur-standard:200ms;--ease-out:ease}' +
  read('static/css/input.css').split('\n').filter((l) => l.includes('ask-slip')).join('\n');

const BOX = '<div class="box" data-box style="border:1px solid;border-radius:11px"><input data-in value="Brother Ansel"><div class="foot"><button type="button">Save</button></div></div>';

async function page(t, body, motion) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 800, height: 600 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html' + (motion ? ' data-motion="' + motion + '"' : '') + '><head><style>' + CSS + '</style></head><body>' + body + '</body></html>');
  await p.addScriptTag({ content: SLIP_JS });
  return p;
}
const settle = (p) => p.waitForTimeout(450);
const slip = (p) => p.evaluate(() => {
  const s = document.querySelector('.ask-slip');
  if (!s) return null;
  const r = s.getBoundingClientRect();
  return {
    last: s === s.parentNode.lastElementChild,
    on: s.classList.contains('is-on'),
    hidden: s.getAttribute('aria-hidden'),
    height: Math.round(r.height),
    inView: r.top >= 0 && r.bottom <= innerHeight,
    ask: s.querySelector('[data-ask]').textContent,
    detail: s.querySelector('[data-detail]').textContent,
    drop: s.querySelector('[data-drop]').textContent,
    dropClass: s.querySelector('[data-drop]').className,
    keepClass: s.querySelector('[data-keep]').className,
    focus: document.activeElement && (document.activeElement.hasAttribute('data-keep') ? 'keep' : document.activeElement.hasAttribute('data-in') ? 'input' : document.activeElement.tagName),
  };
});

test('asks under the box foot, names the loss, and a second ask means keep editing', { skip }, async (t) => {
  const p = await page(t, BOX);
  await p.focus('[data-in]');
  await p.evaluate(() => Chronicle.askSlip(document.querySelector('[data-box]'), {
    ask: 'Discard the new name?', detail: '“Ansel” hasn’t been saved.', discard: 'Discard',
    onDiscard: function () { window.dropped = true; },
  }));
  await settle(p);
  let s = await slip(p);
  assert.equal(s.last, true);
  assert.equal(s.on, true);
  assert.equal(s.hidden, 'false');
  assert.ok(s.height > 40, 'the box grew to hold it');
  assert.equal(s.ask, 'Discard the new name?');
  assert.equal(s.detail, '“Ansel” hasn’t been saved.');
  assert.equal(s.focus, 'keep');
  // The standard buttons, so the style kit's button look applies.
  assert.equal(s.keepClass, 'btn-secondary btn-sm');
  assert.equal(s.dropClass, 'btn-danger btn-sm');

  await p.evaluate(() => Chronicle.askSlip(document.querySelector('[data-box]'), {}));
  await settle(p);
  s = await slip(p);
  assert.equal(s.on, false);
  assert.equal(s.height, 0);
  assert.equal(s.focus, 'input', 'focus goes back to where the person was');
  assert.equal(await p.evaluate(() => !!window.dropped), false);
});

test('Discard runs the box’s discard and puts the slip away', { skip }, async (t) => {
  const p = await page(t, BOX);
  await p.evaluate(() => Chronicle.askSlip(document.querySelector('[data-box]'), { discard: 'Close anyway', onDiscard: function () { window.dropped = true; } }));
  await settle(p);
  assert.equal((await slip(p)).drop, 'Close anyway');
  await p.click('[data-drop]');
  assert.equal(await p.evaluate(() => window.dropped), true);
  assert.equal(await p.evaluate(() => Chronicle.askSlip.isOpen(document.querySelector('[data-box]'))), false);
});

test('a box at the foot of the window scrolls just enough to show the slip', { skip }, async (t) => {
  const p = await page(t, '<div style="height:560px"></div>' + BOX + '<div style="height:20px"></div>');
  await p.evaluate(() => Chronicle.askSlip(document.querySelector('[data-box]'), { ask: 'Discard?' }));
  await p.waitForTimeout(1400);
  const s = await slip(p);
  assert.equal(s.inView, true);
  assert.ok(await p.evaluate(() => scrollY > 0));
});

test('under Off it appears at once, with no slide', { skip }, async (t) => {
  const p = await page(t, BOX, 'off');
  await p.evaluate(() => Chronicle.askSlip(document.querySelector('[data-box]'), { ask: 'Discard?' }));
  const s = await p.evaluate(() => {
    const i = document.querySelector('.ask-slip-in');
    const cs = getComputedStyle(i);
    return { transform: cs.transform, opacity: cs.opacity, height: document.querySelector('.ask-slip').getBoundingClientRect().height };
  });
  assert.equal(s.transform, 'none');
  assert.equal(s.opacity, '1');
  assert.ok(s.height > 40);
});

test('a box redrawn under the slip asks again instead of going quiet', { skip }, async (t) => {
  const p = await page(t, BOX);
  await p.evaluate(() => {
    const b = document.querySelector('[data-box]');
    Chronicle.askSlip(b, { ask: 'First' });
    b.innerHTML = '<input data-in>';
    Chronicle.askSlip(b, { ask: 'Second' });
  });
  await settle(p);
  const s = await slip(p);
  assert.equal(s.on, true);
  assert.equal(s.ask, 'Second');
});

// The Edit name fold, as name_fold.templ draws it (the parts the script reads).
const FOLD = '<button type="button" id="trigger">Edit name</button><p id="away">Elsewhere on the page</p>' +
  '<div id="page-name-fold" class="ag-fold" aria-hidden="true" inert><div class="ag-box" data-box data-name="Brother Ansel" data-structure="false">' +
  '<input data-f="name" value="Brother Ansel" oninput="Chronicle.PageHeader.nameInput(this)"><p data-err hidden></p>' +
  '<div class="ag-foot"><button type="button" onclick="Chronicle.PageHeader.nameClose()">Cancel</button>' +
  '<button type="button" data-save disabled><span data-save-label>Save</span></button></div></div></div>';

test('the Edit name fold stays open on a stray click and asks on Escape', { skip }, async (t) => {
  const p = await page(t, FOLD, 'off');
  await p.addScriptTag({ content: read('static/js/widgets/page_name_fold.js') });
  await p.evaluate(() => Chronicle.PageHeader.nameFold(document.getElementById('trigger')));
  await p.fill('[data-f="name"]', 'Ansel the Grey');
  await p.click('#away');
  const open = () => p.evaluate(() => document.getElementById('page-name-fold').getAttribute('aria-hidden'));
  assert.equal(await open(), 'false', 'a stray click never closes it');
  assert.equal(await slip(p), null, 'and does not ask either');

  await p.focus('[data-f="name"]');
  await p.keyboard.press('Escape');
  let s = await slip(p);
  assert.equal(s.on, true);
  assert.equal(s.ask, 'Discard the new name?');
  assert.equal(s.detail, '“Ansel the Grey” hasn’t been saved.');
  assert.equal(await open(), 'false');

  await p.keyboard.press('Escape');
  s = await slip(p);
  assert.equal(s.on, false, 'a second Escape keeps editing');
  assert.equal(await p.inputValue('[data-f="name"]'), 'Ansel the Grey');

  await p.keyboard.press('Escape');
  await p.click('[data-drop]');
  await p.waitForTimeout(450);
  assert.equal(await open(), 'true');
  assert.equal(await p.inputValue('[data-f="name"]'), 'Brother Ansel', 'Discard puts the saved name back');
});

test('no box that asks in place still shakes', () => {
  for (const f of ['page_name_fold.js', 'give_box.js', 'choice_picker.js', 'roll_tables.js', 'permissions.js']) {
    const src = read('static/js/widgets/' + f);
    assert.ok(!/is-nagging|perm-nudge/.test(src), f + ' still shakes');
  }
  const css = read('static/css/input.css');
  assert.ok(!/@keyframes (ag-shake|rt-shake|perm-nudge)/.test(css), 'a shake keyframe is back');
});
