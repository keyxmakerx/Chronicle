// table_hero_roll_browser.test.mjs — the Table's power roll line against the
// real table.js in a real Chromium. A characteristic showing the viewer's
// hero's number sends the hero's page id with the roll, so Foundry posts it
// as the hero's actor; the plain power roll and a quick die send none.
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
const scripts = {
  '/power_roll.js': readFileSync(join(root, 'internal/plugins/rolls/static/js/power_roll.js')),
  '/table.js': readFileSync(join(root, 'internal/plugins/rolls/static/js/table.js')),
};

const page = (power) => `<!doctype html><html><head><meta charset="utf-8"></head><body>
<header><div class="tt-host" data-widget="table" data-campaign-id="c1" data-user-id="u1"><button type="button" class="tt-tab" data-tt-tab aria-expanded="false">Table</button></div></header>
<main id="main">page</main>
<script>window.posted = []; window.Chronicle = {
  tableDice: new Proxy({}, { get: (_, k) => (k === 'motion' ? () => 'off' : () => '') }), escapeHtml: (v) => String(v), motionMs: (n, f) => f,
  apiFetch: (url, opts) => {
    const reply = (body, ok = true) => Promise.resolve({ ok, status: ok ? 200 : 404, json: () => Promise.resolve(body) });
    if (/\\/rolls\\/power/.test(url)) return reply(${JSON.stringify(power)});
    if (/\\/rolls$/.test(url) && opts && opts.method === 'POST') {
      window.posted.push(opts.body);
      return reply({ id: 'r' + window.posted.length, origin: 'chronicle', rollerName: 'Kai', label: opts.body.label, formula: opts.body.formula, dice: [{ sides: 10, value: 5 }], total: 5, modifier: 0, mode: 'public', createdAt: new Date().toISOString() });
    }
    if (/\\/rolls\\/settings/.test(url)) return reply({});
    if (/\\/rolls/.test(url)) return reply({ rolls: [], more: false });
    return reply(null, false);
  },
  register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)),
};</script><script src="/power_roll.js"></script><script src="/table.js"></script></body></html>`;

async function open(t, power) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (scripts[p]) return route.fulfill({ contentType: 'text/javascript', body: scripts[p] });
    return route.fulfill({ contentType: 'text/html', body: page(power) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForFunction(() => document.querySelector('.tt-host')._table);
  await pg.click('[data-tt-tab]');
  return pg;
}

const LINE = {
  declared: true, dice: '2d10', edge: 2, bane: -2, tiers: [11, 16], hero: 'Saatraaol', heroId: 'hero-1',
  characteristics: [{ key: 'might', label: 'Might', value: 1 }, { key: 'reason', label: 'Reason', value: null }],
};

test('a roll of the hero\'s number names the hero; one without it names none', { skip }, async (t) => {
  const pg = await open(t, LINE);
  await pg.click('[data-tt-pstat="might"]');
  await pg.waitForFunction(() => window.posted.length === 1);
  await pg.click('[data-tt-pstat="reason"]');
  await pg.waitForFunction(() => window.posted.length === 2);
  await pg.click('[data-tt-quick="d6"]');
  await pg.waitForFunction(() => window.posted.length === 3);
  const sent = await pg.evaluate(() => window.posted);
  assert.equal(sent[0].characterId, 'hero-1', 'Might shows the hero\'s +1, so the hero rolls it');
  assert.equal(sent[1].characterId, undefined, 'Reason has no number from the sheet');
  assert.equal(sent[2].characterId, undefined, 'a quick die is the player\'s own');
});

test('a viewer with no hero rolls the plain power roll and names none', { skip }, async (t) => {
  const pg = await open(t, { ...LINE, hero: '', heroId: '', characteristics: LINE.characteristics.map((c) => ({ ...c, value: null })) });
  await pg.click('[data-tt-pplain]');
  await pg.waitForFunction(() => window.posted.length === 1);
  assert.equal((await pg.evaluate(() => window.posted))[0].characterId, undefined);
});
