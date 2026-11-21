// own_entries_browser.test.mjs — the Director's marks on the own-entries page.
//
// Runs own_entries.js in a real Chromium against a stubbed server. It pins
// what a Director sees and what reaches the server: every entry of a list
// with its mark, a quiet mark saved at once, a limiting mark checked first
// and saved with its reason, waiting Ask-first requests allowed or answered
// with a note, and a player's page with none of it.
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

const PLAN = { systemName: 'Draw Steel', steps: [
  { fieldKey: 'class', label: 'Class', choices: [
    { slug: 'fury', name: 'Fury', summary: 'Rage.', source: 'Draw Steel', properties: {} },
    { slug: 'tactician', name: 'Tactician', summary: 'Orders.', source: 'Draw Steel', properties: {} },
  ] },
] };
const entry = (key, name, extra = {}) => ({ key, source: 'package', slug: key, name, summary: name + ' summary.', everyone: null, players: [], state: 'open', ruleCount: 0, ...extra });
const RULES = () => ({
  fieldKey: 'class', systemName: 'Draw Steel',
  groups: [
    { source: 'campaign', key: '*', label: 'This campaign’s own', everyone: null, players: [] },
  ],
  entries: [
    entry('fury', 'Fury'),
    entry('tactician', 'Tactician', { everyone: { state: 'ask_first', reason: '' }, state: 'ask_first' }),
  ],
  members: [{ userId: 'u2', name: 'Pat' }], orphans: [], warnings: [],
});
const REQUEST = { id: 7, fieldKey: 'class', fieldLabel: 'Class', source: 'package', key: 'tactician', entryName: 'Tactician', userId: 'u2', playerName: 'Pat Lee', entityId: 'e1', entityName: 'Tess', status: 'waiting' };

const html = (director) => `<!doctype html><html><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style></head><body>
<div class="hc hc--tool" data-widget="own-entries" data-own-entries="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-entries-url="${C}/system-entries"
  data-members-url="${C}/members" data-director="${director}" data-user="u1" data-field="class" data-csrf="tok"><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])) };</script>
<script src="/own_entry_editor.js"></script><script src="/own_entries.js"></script></body></html>`;

async function open(t, { director = true, checkCount = 0, requests = [REQUEST], putStatus = 200 } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width: 1100, height: 900 } });
  const seen = { put: [], check: [], post: [] };
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname, m = req.method();
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/own_entry_editor.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'internal/plugins/entities/static/js/own_entry_editor.js')) });
    if (p === '/own_entry_editor.css') return route.fulfill({ contentType: 'text/css', body: '' });
    if (p === '/own_entries.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'internal/plugins/entities/static/js/own_entries.js')) });
    if (p === C + '/characters/new/plan') return ok(PLAN);
    if (p === C + '/system-entries') return ok({ entries: [] });
    if (p === C + '/system-entries/grants') return ok({ grants: [] });
    if (p === C + '/members') return ok([{ user_id: 'u2', display_name: 'Pat Lee' }]);
    if (p === C + '/choice-requests') return ok({ requests, count: requests.length });
    if (p === C + '/choice-rules/entries') return ok(RULES());
    if (p === C + '/choice-rules/check') {
      seen.check.push(JSON.parse(req.postData()));
      return ok({ count: checkCount, heroes: checkCount ? [{ heroId: 'e1', heroName: 'Tess', playerId: 'u2', playerName: 'Pat', fieldKey: 'class', value: 'Fury', kind: 'not_offered' }] : [] });
    }
    if (p === C + '/choice-rules' && m === 'PUT') {
      seen.put.push(JSON.parse(req.postData()));
      return ok(putStatus === 200 ? { rules: [] } : { message: 'Players can’t see Fury yet.' }, putStatus);
    }
    if (p.startsWith(C + '/choice-requests/') && m === 'POST') {
      seen.post.push({ p, body: JSON.parse(req.postData() || '{}') });
      return p.endsWith('/allow') ? ok({ heroUpdated: true }) : route.fulfill({ status: 204 });
    }
    return route.fulfill({ contentType: 'text/html', body: html(director) });
  });
  await pg.goto('http://t.test/');
  return { pg, seen };
}

const row = (pg, name) => pg.locator('.oe-ent', { has: pg.locator('b', { hasText: new RegExp('^' + name) }) });

test('a Director sees every entry with its mark', { skip }, async (t) => {
  const { pg } = await open(t);
  await pg.waitForSelector('.oe-ent');
  assert.match(await pg.textContent('.oe-ent--group'), /This campaign/);
  assert.equal(await row(pg, 'Fury').locator('.oe-mark').count(), 0, 'open has no mark');
  assert.match(await row(pg, 'Tactician').locator('.oe-mark').textContent(), /Ask first/);
  assert.ok(await pg.locator('select[data-show]').count(), 'the Showing switch is there');
});

test('a quiet mark saves at once, without a check', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pg.waitForSelector('.oe-ent');
  await row(pg, 'Fury').locator('.oe-dots').click();
  await pg.getByRole('menuitemradio', { name: 'Ask first' }).click();
  await pg.waitForFunction(() => document.querySelector('.oe-ent .oe-mark'));
  assert.equal(seen.check.length, 0);
  assert.deepEqual(seen.put[0].marks, [{ source: 'package', key: 'fury', target: '', state: 'ask_first' }]);
});

test('Not here is checked first, names the heroes, and saves with its reason', { skip }, async (t) => {
  const { pg, seen } = await open(t, { checkCount: 1 });
  await pg.waitForSelector('.oe-ent');
  await row(pg, 'Fury').locator('.oe-dots').click();
  await pg.getByRole('menuitemradio', { name: 'Not here' }).click();
  await pg.waitForSelector('.oe-strip');
  assert.equal(seen.put.length, 0, 'nothing saved before the Director agrees');
  assert.match(await pg.textContent('.oe-asked'), /1 hero already made breaks this: Tess \(Pat\)\. They keep what they have\./);
  await pg.fill('[data-strip-reason]', 'No Furies this time');
  await pg.getByRole('button', { name: 'Save' }).click();
  await pg.waitForFunction(() => !document.querySelector('.oe-strip'));
  assert.deepEqual(seen.put[0].marks, [{ source: 'package', key: 'fury', target: '', state: 'not_offered', reason: 'No Furies this time' }]);
});

test('a refused mark shows the server sentence and keeps the strip', { skip }, async (t) => {
  const { pg } = await open(t, { putStatus: 422 });
  await pg.waitForSelector('.oe-ent');
  await row(pg, 'Fury').locator('.oe-dots').click();
  await pg.getByRole('menuitemradio', { name: 'Ask first' }).click();
  await pg.waitForSelector('.oe-error');
  assert.match(await pg.textContent('.oe-error'), /Players can.t see Fury yet\./);
});

test('a waiting request is allowed for the player', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pg.waitForSelector('.oe-wait');
  assert.match(await pg.textContent('.oe-wait'), /Pat Lee.*asks for.*Tactician.*Tess/s);
  await pg.getByRole('button', { name: 'Allow for Pat' }).click();
  await pg.waitForTimeout(200);
  assert.equal(seen.post[0].p, C + '/choice-requests/7/allow');
});

test('Suggest another sends the short note', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pg.waitForSelector('.oe-wait');
  await pg.getByRole('button', { name: 'Suggest another' }).click();
  await pg.fill('[data-suggest-msg]', 'Try Fury, it suits the table');
  await pg.getByRole('button', { name: 'Send' }).click();
  await pg.waitForTimeout(200);
  assert.equal(seen.post[0].p, C + '/choice-requests/7/suggest');
  assert.deepEqual(seen.post[0].body, { message: 'Try Fury, it suits the table' });
});

test('a player’s page has no marks', { skip }, async (t) => {
  const { pg } = await open(t, { director: false });
  await pg.waitForSelector('.hc-page, .hc-loading__text');
  await pg.waitForTimeout(300);
  assert.equal(await pg.locator('.oe-dots, select[data-show], .oe-wait').count(), 0);
});
