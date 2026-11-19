// dialog_layers_browser.test.mjs — the shared pop-up layers: Chronicle.layerHost,
// Chronicle.modal, the data-opens opener, and toasts that fit a phone and stay
// usable above a modal <dialog>.
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
const THEME = read('static/js/theme.js');
const HELPERS = THEME.slice(THEME.indexOf('/**\n * The "Ask in place" slip'));
const NOTIFY = read('static/js/notifications.js');

async function open(t, body, width) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: width || 800, height: 600 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html><head><style>.hidden{display:none}</style></head><body>' + body + '</body></html>');
  await p.addScriptTag({ content: HELPERS });
  await p.addScriptTag({ content: NOTIFY });
  return p;
}

const POPUP = '<button id="open" data-opens="pop">Open</button><button id="after">After</button>' +
  '<div id="pop" class="hidden"><button id="a">A</button><input id="b"><button id="c">C</button></div>';

test('a pop-up opened by data-opens takes focus, keeps Tab inside, closes on Escape and gives focus back', { skip }, async (t) => {
  const p = await open(t, POPUP);
  await p.click('#open');
  const at = () => p.evaluate(() => document.activeElement.id);
  assert.equal(await at(), 'a');
  assert.equal(await p.getAttribute('#pop', 'role'), 'dialog');
  assert.equal(await p.getAttribute('#pop', 'aria-modal'), 'true');
  await p.keyboard.press('Shift+Tab');
  assert.equal(await at(), 'c', 'Shift+Tab from the first control wraps to the last');
  await p.keyboard.press('Tab');
  assert.equal(await at(), 'a', 'Tab from the last control wraps to the first');
  await p.keyboard.press('Escape');
  assert.equal(await p.evaluate(() => document.getElementById('pop').classList.contains('hidden')), true);
  assert.equal(await at(), 'open', 'focus returns to the button that opened it');
});

test('Escape leaves a pop-up alone when something else already took the key', { skip }, async (t) => {
  const p = await open(t, POPUP);
  await p.click('#open');
  await p.evaluate(() => document.getElementById('b').addEventListener('keydown', (e) => { if (e.key === 'Escape') e.preventDefault(); }));
  await p.focus('#b');
  await p.keyboard.press('Escape');
  assert.equal(await p.evaluate(() => document.getElementById('pop').classList.contains('hidden')), false);
});

test('hiding the pop-up yourself still returns focus to the opener', { skip }, async (t) => {
  const p = await open(t, POPUP);
  await p.click('#open');
  await p.evaluate(() => document.getElementById('pop').classList.add('hidden'));
  await p.waitForFunction(() => document.activeElement.id === 'open');
});

test('layerHost is the topmost open modal dialog, else the body', { skip }, async (t) => {
  const p = await open(t, '<dialog id="d1"><button>x</button></dialog><dialog id="d2"><button>y</button></dialog>');
  assert.equal(await p.evaluate(() => Chronicle.layerHost() === document.body), true);
  await p.evaluate(() => { document.getElementById('d1').showModal(); document.getElementById('d2').showModal(); });
  assert.equal(await p.evaluate(() => Chronicle.layerHost().id), 'd2');
  await p.evaluate(() => document.getElementById('d2').close());
  assert.equal(await p.evaluate(() => Chronicle.layerHost().id), 'd1');
});

test('a toast raised over a modal dialog can be pressed', { skip }, async (t) => {
  const p = await open(t, '<dialog id="d"><button>x</button></dialog>');
  await p.evaluate(() => document.getElementById('d').showModal());
  await p.evaluate(() => Chronicle.notify('Saved', 'info', { duration: 0 }));
  const hit = await p.evaluate(() => {
    const b = document.querySelector('#chronicle-toasts button');
    const r = b.getBoundingClientRect();
    const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    return { inside: document.getElementById('d').contains(document.getElementById('chronicle-toasts')), onIt: b.contains(top) };
  });
  assert.deepEqual(hit, { inside: true, onIt: true });
});

test('a toast fits a 360 wide phone and its close button is 44 by 44', { skip }, async (t) => {
  const p = await open(t, '<main>page</main>', 360);
  await p.evaluate(() => Chronicle.notify('Saved', 'info', { duration: 0 }));
  const m = await p.evaluate(() => {
    const r = document.querySelector('.chronicle-toast').getBoundingClientRect();
    const b = document.querySelector('.chronicle-toast button');
    const h = b.querySelector('span').getBoundingClientRect();
    return { left: r.left, right: r.right, hitW: h.width, hitH: h.height, glyphW: b.getBoundingClientRect().width };
  });
  assert.ok(m.left >= 16 && m.right <= 344, `toast spans ${m.left}..${m.right}`);
  assert.ok(m.hitW >= 44 && m.hitH >= 44, 'close hit area is at least 44 by 44');
  assert.ok(m.glyphW < 20, 'the glyph keeps its drawn size');
});
