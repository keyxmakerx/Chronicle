// hero_creator_browser.test.mjs — the new-hero creator under a Director's rules.
//
// Runs hero_creator.js (with the shared hero_rules_lines.js) in a real
// Chromium against a stubbed plan. It pins what a player making a hero sees:
// the "n not here" foot folding open names and reasons, a required step with
// one entry filled in and locked, a short-list note, the Ask first mark with a
// waiting line, the Director's suggestion, and what reaches the server.
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
const C = '/campaigns/c1';
const js = (n) => readFileSync(join(root, 'internal/plugins/entities/static/js', n));

const ch = (key, name, extra = {}) => ({ key, slug: key, name, summary: name + ' summary.', source: 'package', properties: {}, ...extra });
const PLAN = () => ({ systemName: 'Draw Steel', steps: [
  { fieldKey: 'ancestry', label: 'Ancestry', choices: [ch('human', 'Human')],
    rules: { limited: true, required: true, prefilled: 'Human' } },
  { fieldKey: 'class', label: 'Class', choices: [ch('fury', 'Fury'), ch('tactician', 'Tactician', { state: 'ask_first' })],
    rules: { limited: true, notHereCount: 3, notHere: [{ name: 'Censor', reason: 'Not in this story' }, { name: 'Null' }] } },
  { fieldKey: 'kit', label: 'Kit', choices: [ch('a', 'Arcane Archer'), ch('b', 'Battlemind')],
    rules: { limited: true, required: true } },
  { fieldKey: 'culture', label: 'Culture', choices: [ch('c', 'Academic')],
    rules: { limited: true, suggested: { id: 4, entryName: 'Nomad', message: 'Try the coast' } } },
] });

const css = () => readFileSync(join(root, 'internal/plugins/entities/static/css/hero_creator.css'));
const html = (term) => `<!doctype html><html data-gm-term="${term}"><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style><link rel="stylesheet" href="/hero_creator.css"></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true"><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => { window.hcImpl = d; document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)); }, escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => document.documentElement.getAttribute('data-gm-term') || 'GM' };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, { created = { id: 'e9', url: C + '/entities/e9' }, term = 'Director', width = 1100 } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce' });
  const seen = { post: [] };
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_creator.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: css() });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_rules_lines.js') });
    if (p === C + '/characters/new/plan') return ok(PLAN());
    if (p === C + '/characters/new' && req.method() === 'POST') { seen.post.push(JSON.parse(req.postData())); return ok(created, 201); }
    return route.fulfill({ contentType: 'text/html', body: html(term) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return { pg, seen };
}
const text = (pg, sel) => pg.locator(sel).first().innerText();
const gcard = (pg, name) => pg.locator('.hc-gcard', { hasText: name });
const rail = (pg, key) => pg.locator('.hc-rail .hc-row[data-k="' + key + '"]');
const pick = async (pg, name) => { await gcard(pg, name).click(); await pg.click('.hc-right [data-act="choose"]'); };

test('a required step with one entry is filled in and locked', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.equal(await pg.locator('.hc-app').getAttribute('data-step'), '1', 'the page opens on the first step with something to choose');
  assert.match(await rail(pg, 'ancestry').innerText(), /Human/);
  await rail(pg, 'ancestry').click();
  assert.match(await text(pg, '.hc-intro'), /Everyone starts with this/);
  assert.match(await text(pg, '.hc-gcard'), /Everyone starts with this/);
  assert.match(await text(pg, '.hc-right [data-act="choose"]'), /Human, everyone starts with this/);
  assert.equal(await pg.locator('.hc-right [data-act="choose"]').isDisabled(), true);
  assert.equal(await pg.locator('.hc-note').count(), 0, 'no short-list note for a locked step');
});

test('not here folds open names and reasons at the list foot', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.equal(await pg.locator('.hc-gcard[data-i]').count(), 2, 'not-here entries are not listed');
  const foot = pg.locator('.hc-nh .hc-link');
  assert.equal((await foot.innerText()).trim(), '3 not here');
  assert.equal(await pg.locator('.hc-nh-list').count(), 0);
  await foot.click();
  const list = await text(pg, '.hc-nh-list');
  assert.match(list, /Censor isn’t here\. Your Director says: “Not in this story”/);
  assert.match(list, /Null isn’t here\./);
  assert.match(list, /and 1 more/);
  assert.equal(await pg.locator('.hc-nh .hc-link').getAttribute('aria-expanded'), 'true');
});

test('the campaign\'s own word for its game master is used', { skip }, async (t) => {
  const { pg } = await open(t, { term: 'DM' });
  await pg.click('.hc-nh .hc-link');
  assert.match(await text(pg, '.hc-nh-list'), /Your DM says: “Not in this story”/);
  await pick(pg, 'Tactician');
  assert.match(await rail(pg, 'class').innerText(), /waiting for your DM/);
  await rail(pg, 'class').click();
  assert.match(await text(pg, '.hc-notes'), /Tactician is waiting for your DM/);
  assert.doesNotMatch(await pg.locator('.hc-app').innerText(), /Director/);
});

test('an Ask first pick is marked, waits for the Director and is sent with the hero', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  assert.match(await text(pg, '.hc-gcard'), /Fury/);
  assert.match(await gcard(pg, 'Tactician').innerText(), /Ask first/);
  await gcard(pg, 'Tactician').click();
  assert.match(await text(pg, '.hc-right [data-act="choose"]'), /^Ask for Tactician/);
  await pg.click('.hc-right [data-act="choose"]');
  assert.match(await rail(pg, 'class').innerText(), /Tactician.*waiting for your Director/);
  await rail(pg, 'class').click();
  assert.match(await text(pg, '.hc-notes'), /Tactician is waiting for your Director/);
  await rail(pg, 'kit').click();
  assert.match(await text(pg, '.hc-notes'), /Your Director has set a short list for this/);
  await pick(pg, 'Arcane Archer');
  assert.match(await text(pg, '.hc-notes'), /asked for another culture instead of Nomad: “Try the coast”/);
  await pick(pg, 'Academic');
  assert.match(await text(pg, '.hc-sum'), /Tactician\s*waiting for your Director/);
  await pg.fill('[data-name]', 'Tess');
  await pg.click('[data-act="create"]');
  await pg.waitForTimeout(100);
  assert.deepEqual(seen.post[0].picks.class, { name: 'Tactician', options: [] });
  assert.deepEqual(seen.post[0].picks.ancestry, { name: 'Human', options: [] }, 'the locked step is sent');
  assert.deepEqual(seen.post[0].picks.kit, { name: 'Arcane Archer', options: [] });
});

test('a pick made on a phone goes into the chip row and on to the next step', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390 });
  assert.equal(await pg.locator('.hc-app').getAttribute('data-view'), 'phone');
  assert.match(await text(pg, '.hc-sbtn'), /Step 2 of 5\s*·\s*Class/, 'the locked first step is not a place to start');
  await pg.locator('.hc-card', { hasText: 'Fury' }).click();
  await pg.click('.hc-sfoot [data-act="choose"]');
  await pg.waitForFunction(() => /Step 3 of 5/.test(document.querySelector('.hc-sbtn').innerText));
  const chips = await pg.locator('.hc-chip').allInnerTexts();
  assert.ok(chips.some((c) => /class\s*fury/i.test(c)), 'the pick is a chip');
  assert.ok(chips.some((c) => /kit\s*choosing/i.test(c)), 'the step being chosen is the open slot');
});

test('unmounting lets go of the window, the document and the size watcher', { skip }, async (t) => {
  const { pg } = await open(t);
  const r = await pg.evaluate(() => {
    const el = document.querySelector('[data-widget="hero-creator"]'), c = el._heroCreator, gone = [];
    const doc = document.removeEventListener.bind(document), win = window.removeEventListener.bind(window);
    document.removeEventListener = (type, fn) => { if (fn === c.onDocDown) gone.push('document ' + type); return doc(type, fn); };
    window.removeEventListener = (type, fn) => { if (fn === c.onResize) gone.push('window ' + type); return win(type, fn); };
    let cut = false; if (c.ro) { const d = c.ro.disconnect.bind(c.ro); c.ro.disconnect = () => { cut = true; d(); }; }
    window.hcImpl.destroy(el);
    return { gone, cut, left: !!el._heroCreator };
  });
  assert.deepEqual(r.gone.sort(), ['document pointerdown', 'window resize']);
  assert.equal(r.cut, true, 'the ResizeObserver is disconnected');
  assert.equal(r.left, false);
});
