// brochure_browser.test.mjs — the Brochure's panel builder (internal/plugins/entities/static/js/brochure.js)
// and the hover card's seat inside an open dialog (static/js/hovercard.js).
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

const PAGE = '<h1>Title</h1><p><em>The line.</em></p><hr><p>Before any heading.</p>' +
  '<h2>One</h2><p>Words.</p>' +
  '<h2>Two</h2><h3>Part a</h3><p>A.</p><h3>Part b</h3><table><tr><th>A</th><th>B</th></tr><tr><td>x</td><td>a cell with a good many more than thirty-two characters</td></tr></table>' +
  '<h2>Three</h2><table><tr><th>A</th><td>short</td></tr></table>';

const BODY = '<section data-block="brochure"><button type="button" data-box>Open</button>' +
  '<dialog data-brochure-dialog><div class="cb-bro-body"><div data-brochure-src>' + PAGE + '</div></div></dialog></section>';

async function open(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html><body>' + BODY + '</body></html>');
  await p.addScriptTag({ content: 'window.Chronicle = { register: function () {} };' });
  await p.addScriptTag({ content: read('static/js/hovercard.js') });
  await p.addScriptTag({ content: read('static/js/peek_address.js') });
  await p.addScriptTag({ content: read('static/js/peek_panel.js') });
  // The panel's own placement rules, as input.css gives them.
  await p.addStyleTag({ content: '.peek-panel{position:fixed;z-index:70;top:0;right:0;bottom:0;width:30rem;background:#fff}.peek-panel[hidden]{display:none}' });
  await p.addScriptTag({ content: read('internal/plugins/entities/static/js/brochure.js') });
  await p.evaluate(() => Chronicle.brochure.open(document.querySelector('[data-box]')));
  return p;
}

test('the cover holds the title, the line and what comes before the first heading; a rule is dropped', { skip }, async (t) => {
  const p = await open(t);
  const cover = await p.evaluate(() => {
    const c = document.querySelector('.cb-bro-cover');
    return { text: c.textContent, hr: c.querySelectorAll('hr').length, n: c.dataset.n, of: c.dataset.of };
  });
  assert.match(cover.text, /Title/);
  assert.match(cover.text, /The line\./);
  assert.match(cover.text, /Before any heading\./);
  assert.equal(cover.hr, 0);
  assert.equal(cover.n, '1');
  assert.equal(cover.of, '4');
});

test('each second-level heading starts the next panel, numbered in order', { skip }, async (t) => {
  const p = await open(t);
  const panels = await p.evaluate(() => [...document.querySelectorAll('.cb-bro-panel')].map((x) => [x.dataset.n, x.querySelector('h2') ? x.querySelector('h2').textContent : 'cover']));
  assert.deepEqual(panels, [['1', 'cover'], ['2', 'One'], ['3', 'Two'], ['4', 'Three']]);
});

test('a wide table takes two folds, a small one does not, and every table has its own scroll box', { skip }, async (t) => {
  const p = await open(t);
  const r = await p.evaluate(() => ({
    wide: [...document.querySelectorAll('.cb-bro-panel')].map((x) => x.classList.contains('cb-bro-panel--wide')),
    wrapped: [...document.querySelectorAll('table')].every((x) => x.parentElement.classList.contains('cb-bro-tw')),
    parts: document.querySelectorAll('.cb-bro-panel:nth-child(3) .cb-bro-sec').length,
  }));
  assert.deepEqual(r.wide, [false, false, true, false]);
  assert.ok(r.wrapped);
  assert.equal(r.parts, 2);
});

test('opening twice builds the panels once', { skip }, async (t) => {
  const p = await open(t);
  await p.evaluate(() => { document.querySelector('dialog').close(); Chronicle.brochure.open(document.querySelector('[data-box]')); });
  assert.equal(await p.locator('.cb-bro-panel').count(), 4);
});

test('a hover card for a link inside an open modal dialog is seated in the dialog, and back on the body otherwise', { skip }, async (t) => {
  const p = await open(t);
  await p.evaluate(() => {
    const a = document.createElement('a');
    a.href = '#'; a.id = 'in'; a.textContent = 'in';
    document.querySelector('.cb-bro-panel').appendChild(a);
    const b = document.createElement('a');
    b.href = '#'; b.id = 'out'; b.textContent = 'out';
    document.body.appendChild(b);
  });
  assert.equal(await p.evaluate(() => { Chronicle.hovercard.open(document.getElementById('in'), { title: 'T' }); return document.getElementById('chc').parentNode.nodeName; }), 'DIALOG');
  // dialog closed: a link outside it puts the card back on the page
  assert.equal(await p.evaluate(() => { Chronicle.hovercard.close(); document.querySelector('dialog').close(); Chronicle.hovercard.open(document.getElementById('out'), { title: 'T' }); return document.getElementById('chc').parentNode.nodeName; }), 'BODY');
});

test('a page peek opened from inside an open brochure is seated in it, on top and reachable, and goes back to the body after', { skip }, async (t) => {
  const p = await open(t);
  await p.evaluate(() => {
    window.fetch = () => Promise.resolve({ ok: true, text: () => Promise.resolve('<div data-peek-root><p id="pk">Peeked</p></div>') });
    const a = document.createElement('a');
    a.href = '/campaigns/c1/entities/e1'; a.id = 'in'; a.textContent = 'in';
    document.querySelector('.cb-bro-panel').appendChild(a);
  });
  await p.evaluate(() => Chronicle.peek.open(document.getElementById('in')));
  await p.waitForSelector('#pk');
  assert.equal(await p.evaluate(() => document.getElementById('peek-panel').parentNode.nodeName), 'DIALOG');
  // The proof it is not behind the dialog: a point inside the panel lands on the panel.
  assert.equal(await p.evaluate(() => {
    const r = document.getElementById('peek-panel').getBoundingClientRect();
    const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    return !!hit && document.getElementById('peek-panel').contains(hit);
  }), true);
  // The same key that closes the peek must not also close the brochure.
  await p.keyboard.press('Escape');
  assert.equal(await p.evaluate(() => document.querySelector('dialog').open), true, 'Escape closed the brochure with the peek');
  assert.equal(await p.evaluate(() => Chronicle.peek.isOpen()), false);
  // A peek opened from the hover card (an address, no link) takes the open dialog too.
  await p.evaluate(() => Chronicle.peek.open('/campaigns/c1/entities/e2'));
  assert.equal(await p.evaluate(() => document.getElementById('peek-panel').parentNode.nodeName), 'DIALOG');
  await p.evaluate(() => document.querySelector('dialog').close());
  // The dialog's close event arrives a task later.
  await p.waitForFunction(() => document.getElementById('peek-panel').parentNode.nodeName === 'BODY');
  assert.equal(await p.evaluate(() => Chronicle.peek.isOpen()), false);
});
