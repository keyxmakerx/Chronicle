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

const html = (term) => `<!doctype html><html data-gm-term="${term}"><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true"><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => document.documentElement.getAttribute('data-gm-term') || 'GM' };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, { created = { id: 'e9', url: C + '/entities/e9' }, term = 'Director' } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width: 1100, height: 900 } });
  const seen = { post: [] };
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_creator.js') });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_rules_lines.js') });
    if (p === C + '/characters/new/plan') return ok(PLAN());
    if (p === C + '/characters/new' && req.method() === 'POST') { seen.post.push(JSON.parse(req.postData())); return ok(created, 201); }
    return route.fulfill({ contentType: 'text/html', body: html(term) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-page');
  return { pg, seen };
}
const text = (pg, sel) => pg.locator(sel).first().innerText();
const next = (pg) => pg.click('[data-act="next"]');

test('a required step with one entry is filled in and locked', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.match(await text(pg, '.hc-head'), /Everyone starts with this/);
  assert.match(await text(pg, '.hc-row'), /Everyone starts with this/);
  assert.match(await text(pg, '.hc-chosen'), /Human.*everyone starts with this/);
  await pg.click('.hc-row');
  assert.equal(await pg.locator('[data-act="choose"]').isDisabled(), true);
  assert.equal(await pg.locator('.hc-note').count(), 0, 'no short-list note for a locked step');
});

test('not here folds open names and reasons at the list foot', { skip }, async (t) => {
  const { pg } = await open(t);
  await next(pg);
  assert.equal(await pg.locator('.hc-row').count(), 2, 'not-here entries are not listed');
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
  await next(pg);
  await pg.click('.hc-nh .hc-link');
  assert.match(await text(pg, '.hc-nh-list'), /Your DM says: “Not in this story”/);
  await pg.locator('.hc-row', { hasText: 'Tactician' }).click();
  await pg.click('[data-act="choose"]');
  assert.match(await text(pg, '.hc-notes'), /Tactician is waiting for your DM/);
  assert.doesNotMatch(await pg.locator('.hc-page').innerText(), /Director/);
});

test('an Ask first pick is marked, waits for the Director and is sent with the hero', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await next(pg);
  assert.match(await text(pg, '.hc-row:nth-of-type(1)'), /Fury/);
  assert.match(await pg.locator('.hc-row', { hasText: 'Tactician' }).innerText(), /Ask first/);
  await pg.locator('.hc-row', { hasText: 'Tactician' }).click();
  assert.match(await text(pg, '[data-act="choose"]'), /^Ask for Tactician/);
  await pg.click('[data-act="choose"]');
  assert.match(await text(pg, '.hc-notes'), /Tactician is waiting for your Director/);
  assert.match(await text(pg, '.hc-chosen'), /Tactician.*waiting for your Director/);
  await next(pg);
  assert.match(await text(pg, '.hc-notes'), /Your Director has set a short list for this/);
  await next(pg);
  assert.match(await text(pg, '.hc-notes'), /asked for another culture instead of Nomad: “Try the coast”/);
  await next(pg);
  assert.match(await text(pg, '.hc-sum'), /Tactician\s*waiting for your Director/);
  await pg.fill('[data-name]', 'Tess');
  await pg.click('[data-act="create"]');
  await pg.waitForTimeout(100);
  assert.deepEqual(seen.post[0].picks.class, { name: 'Tactician', options: [] });
  assert.deepEqual(seen.post[0].picks.ancestry, { name: 'Human', options: [] }, 'the locked step is sent');
});
