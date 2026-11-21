// hero_creator_own_browser.test.mjs — a player writes their own entry inside
// the hero creator.
//
// Runs hero_creator.js with the shared own_entry_editor.js in a real Chromium
// against a stubbed plan and entry API. It pins the dashed "Make your own"
// card (only while a Director's permission is unspent, never in practice or
// try-out), the editor in the phone's sheet and the computer's right pane, its
// checks, the locked points budget and its meter, Save draft and Hand it in,
// the server's errors shown in place, the Director's note, the entry at the
// front of the hand and its choice into the hero.
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
const read = (n) => readFileSync(join(root, 'internal/plugins/entities/static', n));

const pkg = (key, name, props = {}) => ({ key, slug: key, name, summary: name + ' summary.', source: 'package', properties: props });
const ANCESTRY = () => [
  pkg('human', 'Human', { size: '1M', speed: 5, ancestry_points: 3, signature_traits: [{ name: 'Resolve', description: 'Keep going.' }],
    purchased_traits: [{ name: 'Grounded', cost: 1, description: 'Hard to move.' }, { name: 'Tough', cost: 2, description: 'Hard to break.' }] }),
  pkg('elf', 'Elf', { size: '1M', speed: 5, ancestry_points: 3, signature_traits: [{ name: 'Glamor', description: 'Shine.' }],
    purchased_traits: [{ name: 'Swift', cost: 1, description: 'Quick.' }] }),
];
const PLAN = (extra = {}, choices = ANCESTRY()) => ({ systemName: 'Draw Steel', steps: [
  { fieldKey: 'ancestry', label: 'Ancestry', choices, canMake: true, ...extra },
  { fieldKey: 'class', label: 'Class', choices: [pkg('fury', 'Fury')] },
] });

const html = (attrs, motion) => `<!doctype html><html data-gm-term="Director"${motion ? ` data-motion="${motion}"` : ''}><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style><link rel="stylesheet" href="/hero_creator.css"></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true" ${attrs}><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director',
  apiFetch: (url, o) => fetch(url, { method: (o && o.method) || 'GET', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': (o && o.csrfToken) || '' }, body: o && o.body ? JSON.stringify(o.body) : undefined }) };</script>
<script src="/hero_rules_lines.js"></script><script src="/own_entry_editor.js"></script><script src="/hero_creator.js"></script></body></html>`;

// A stand-in for the entry API: the player's one permission, the entries they
// have written, and what a save answers.
async function open(t, o = {}) {
  const { width = 390, attrs = '', plan = PLAN(), grants = [{ id: 1, fieldKey: 'ancestry', userId: 'u1', budget: 3 }], entries = [], failWith = null, motion = null } = o;
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height: 844 }, reducedMotion: 'reduce' });
  const seen = { post: [], put: [], created: [] };
  const store = entries.slice();
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname, m = req.method();
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: read('js/hero_creator.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: read('css/hero_creator.css') });
    if (p === '/own_entry_editor.js') return route.fulfill({ contentType: 'text/javascript', body: read('js/own_entry_editor.js') });
    if (p === '/own_entry_editor.css') return route.fulfill({ contentType: 'text/css', body: read('css/own_entry_editor.css') });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: read('js/hero_rules_lines.js') });
    if (p === C + '/characters/new/plan') return ok(plan);
    if (p === C + '/system-entries/grants') return ok({ grants });
    if (p === C + '/system-entries' && m === 'GET') return ok({ entries: store });
    if (p === C + '/system-entries' && m === 'POST') {
      const b = JSON.parse(req.postData());
      seen.post.push(b);
      if (failWith) return ok({ message: failWith }, 409);
      const e = { id: 41, fieldKey: b.fieldKey, slug: 'ashborn', name: b.name, summary: b.summary, description: b.description, properties: b.properties,
        status: b.status === 'sent' ? 'sent' : 'draft', visibility: 'everyone', createdBy: 'u1' };
      store.push(e);
      return ok({ entry: e }, 201);
    }
    if (p.startsWith(C + '/system-entries/') && m === 'PUT') {
      const b = JSON.parse(req.postData());
      seen.put.push({ path: p, body: b });
      const e = store.find((x) => '/system-entries/' + x.id === p.slice(C.length));
      if (e) { Object.assign(e, b.name ? b : {}); if (b.status) e.status = b.status; return ok({ entry: e }); }
      return ok({});
    }
    if (p === C + '/characters/new' && m === 'POST') { seen.created.push(JSON.parse(req.postData())); return ok({ id: 'e9', url: C + '/entities/e9' }, 201); }
    return route.fulfill({ contentType: 'text/html', body: html(attrs, motion) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return { pg, seen, store };
}
const text = (pg, sel) => pg.locator(sel).first().innerText();
const writeDraft = async (pg, name = 'Ashborn') => {
  await pg.fill('[data-k="name"]', name);
  await pg.fill('[data-k="summary"]', 'Born of ash and ember.');
};

test('the make card shows only while a permission is unspent, and never in practice or try-out', { skip }, async (t) => {
  const a = await open(t, { plan: PLAN({ canMake: false }) });
  assert.equal(await a.pg.locator('[data-act="ownopen"]').count(), 0, 'no permission, no card');
  const b = await open(t, { attrs: 'data-practice="true"' });
  assert.equal(await b.pg.locator('[data-act="ownopen"]').count(), 0, 'practice is not where the one permission is spent');
  const c = await open(t, { attrs: 'data-tryout="true"' });
  assert.equal(await c.pg.locator('[data-act="ownopen"]').count(), 0, 'a guest never gets it');
  const d = await open(t);
  const card = d.pg.locator('.hc-hand .hc-card--own');
  assert.equal(await card.count(), 1);
  assert.match(await card.innerText(), /Make your own ancestry/);
  assert.match(await card.innerText(), /Your Director allows one/);
  assert.match(await card.innerText(), /3 points of traits[\s\S]*Points to spend\s+3, set by your Director/);
  assert.equal(await d.pg.locator('.hc-hand .hc-card[data-i]').count(), 2, 'the card is not one of the entries');
  assert.equal(await d.pg.locator('.hc-hand > *').last().getAttribute('data-act'), 'ownopen', 'it ends the hand');
  assert.equal(await d.pg.locator('a[href*="characters/entries"]').count(), 0, 'the link out is gone');
});

test('the make card is also a row in List view and a card in the grid on a computer', { skip }, async (t) => {
  const { pg } = await open(t, { width: 1280 });
  assert.equal(await pg.locator('.hc-gcard--own').count(), 1);
  assert.equal(await pg.locator('.hc-gcard[data-i]').count(), 2);
  await pg.click('.hc-mode [data-mode="list"]');
  assert.equal(await pg.locator('.hc-lrow--own').count(), 1);
  assert.match(await text(pg, '.hc-lrow--own'), /Your Director allows one/);
  await pg.fill('[data-find]', 'zzz');
  assert.equal(await pg.locator('.hc-lrow--own').count(), 0, 'not a match for a search');
});

test('on a phone the editor opens in the sheet, checks the name and keeps the points locked', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pg.click('.hc-hand .hc-card--own');
  await pg.waitForSelector('.hc-sheet [data-oed-form]');
  assert.match(await text(pg, '.hc-sheet .oed-title'), /Make your own ancestry/);
  assert.match(await text(pg, '.hc-sheet .oed-mark'), /Your Director allows one/);
  assert.match(await text(pg, '.hc-sheet .oed-ro'), /Ancestry points\s*3, locked/);
  assert.equal(await pg.locator('.oed-step').count(), 0, 'a Director-set budget has no stepper');
  // The facts the system's own ancestries share are the lines to fill in.
  const facts = await pg.locator('.oed-fact input[data-part="label"]').evaluateAll((els) => els.map((e) => e.value));
  assert.deepEqual(facts.sort(), ['Size', 'Speed']);
  // Handing in nameless says so, in place, and sends nothing.
  await pg.click('.hc-sfoot [data-oed="send"]');
  assert.equal(await pg.locator('[data-ferr="name"]').isVisible(), true);
  assert.match(await text(pg, '.hc-sfoot [data-error]'), /Give it a name/);
  assert.equal(seen.post.length, 0);
});

test('the trait list is the meter: it fills, counts and refuses with the creator\'s words', { skip }, async (t) => {
  const { pg } = await open(t);
  await pg.click('.hc-hand .hc-card--own');
  await pg.waitForSelector('[data-oed-form]');
  await pg.click('[data-list="purchased_traits"] [data-oed="additem"]');
  await pg.fill('[data-list="purchased_traits"] .oed-item__name', 'Ember Skin');
  await pg.click('[data-list="purchased_traits"] [data-cost="2"]');
  await pg.click('[data-list="purchased_traits"] [data-oed="additem"]');
  const names = pg.locator('[data-list="purchased_traits"] .oed-item__name');
  await names.nth(1).fill('Cinder Step');
  await pg.locator('[data-list="purchased_traits"] [data-cost="2"]').nth(1).click();
  assert.equal(await text(pg, '[data-left]'), '3');
  await pg.click('.oed-tcard[data-t="0"] .oed-tbuy');
  assert.equal(await text(pg, '[data-left]'), '1');
  assert.equal(await pg.locator('.oed-pips i.is-on').count(), 2);
  assert.match(await text(pg, '.oed-tcard[data-t="1"] .oed-tbuy'), /Needs 2 points, 1 left/);
  assert.equal(await pg.locator('.oed-tcard[data-t="1"]').getAttribute('class').then((c) => /is-no/.test(c)), true);
  await pg.click('.oed-tcard[data-t="0"] .oed-tbuy');
  assert.equal(await text(pg, '[data-left]'), '3', 'putting one back gives the points back');
  await pg.fill('[data-k="name"]', 'Ashborn');
  assert.match(await text(pg, '.oed-checks'), /Ready. A hero can spend all 3 points/);
});

test('Save draft sends the entry as written, with the locked points, and it leads the hand as a draft', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pg.click('.hc-hand .hc-card--own');
  await pg.waitForSelector('[data-oed-form]');
  await writeDraft(pg);
  await pg.locator('.oed-fact input[data-part="value"]').first().fill('1L');
  await pg.click('[data-list="purchased_traits"] [data-oed="additem"]');
  await pg.fill('[data-list="purchased_traits"] .oed-item__name', 'Ember Skin');
  await pg.click('[data-list="purchased_traits"] [data-cost="3"]');
  await pg.click('.hc-sfoot [data-oed="draft"]');
  await pg.waitForSelector('.hc-sheet', { state: 'detached' });
  assert.equal(seen.post.length, 1);
  const b = seen.post[0];
  assert.equal(b.fieldKey, 'ancestry');
  assert.equal(b.name, 'Ashborn');
  assert.equal(b.summary, 'Born of ash and ember.');
  assert.equal(b.status, undefined, 'a draft is not handed in');
  assert.equal(b.properties.ancestry_points, 3);
  assert.deepEqual(b.properties.purchased_traits, [{ name: 'Ember Skin', cost: 3 }]);
  assert.equal(b.properties.size, '1L');
  // It is the first card, marked Draft; the make card is gone, the permission is spent.
  const first = pg.locator('.hc-hand .hc-card[data-i]').first();
  assert.match(await first.innerText(), /Ashborn/);
  assert.match(await first.innerText(), /Draft/);
  assert.equal(await pg.locator('.hc-card--own').count(), 0);
  assert.equal(await pg.locator('.hc-hand .hc-card[data-i]').count(), 3);
});

test('handed in, it waits for the Director and can still be chosen into the hero', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pg.click('.hc-hand .hc-card--own');
  await pg.waitForSelector('[data-oed-form]');
  await writeDraft(pg);
  await pg.click('.hc-sfoot [data-oed="send"]');
  await pg.waitForSelector('.hc-sheet', { state: 'detached' });
  assert.equal(seen.post[0].status, 'sent');
  const first = pg.locator('.hc-hand .hc-card[data-i]').first();
  assert.match(await first.innerText(), /Waiting for your Director/);
  await first.click();
  await pg.waitForSelector('.hc-sheet [data-act="ownedit"]');
  assert.match(await text(pg, '.hc-sheet .hc-ownbar'), /Waiting for your Director, and you can still pick it/);
  await pg.click('.hc-sfoot [data-act="choose"]');
  await pg.waitForFunction(() => /Ashborn/.test(document.querySelector('.hc-chips').innerText));
  // On to the name, and the hero is made with the entry; it is then kept to the hero.
  await pg.click('.hc-chip.is-slot, .hc-sbtn');
  await pg.evaluate(() => document.querySelector('.hc-app') && 0);
  const go = pg.locator('[data-act="steps"]');
  await go.click();
  await pg.click('.hc-drape .hc-row:last-child');
  await pg.fill('[data-name]', 'Tess');
  await pg.click('[data-act="create"]');
  await pg.waitForTimeout(250);
  assert.deepEqual(seen.created[0].picks.ancestry, { name: 'Ashborn', options: [] });
  assert.deepEqual(seen.put.find((p) => p.path.endsWith('/system-entries/41')).body, { forEntityId: 'e9' });
});

test('a server refusal shows in the editor and keeps what was written', { skip }, async (t) => {
  const { pg } = await open(t, { failWith: 'an entry called Ashborn already exists' });
  await pg.click('.hc-hand .hc-card--own');
  await pg.waitForSelector('[data-oed-form]');
  await writeDraft(pg);
  await pg.click('.hc-sfoot [data-oed="send"]');
  await pg.waitForSelector('.hc-sfoot [data-error]:not([hidden])');
  assert.match(await text(pg, '.hc-sfoot [data-error]'), /already exists/);
  assert.equal(await pg.locator('[data-k="name"]').inputValue(), 'Ashborn');
  assert.equal(await pg.locator('.hc-sheet').count(), 1, 'the sheet stays open');
  assert.equal(await pg.locator('.hc-sfoot [data-oed="send"]').isDisabled(), false, 'and it can be tried again');
});

test('closing a sheet with unsaved writing asks first', { skip }, async (t) => {
  const { pg } = await open(t);
  await pg.click('.hc-hand .hc-card--own');
  await pg.waitForSelector('[data-oed-form]');
  await pg.fill('[data-k="name"]', 'Ashborn');
  await pg.click('.hc-sheet [data-act="closesheet"]');
  assert.match(await text(pg, '.hc-sfoot .oed-confirm'), /Close without saving/);
  await pg.click('.hc-sfoot [data-oed="keep"]');
  assert.equal(await pg.locator('[data-k="name"]').inputValue(), 'Ashborn');
  await pg.click('.hc-sheet [data-act="closesheet"]');
  await pg.click('.hc-sfoot [data-oed="discard"]');
  await pg.waitForSelector('.hc-sheet', { state: 'detached' });
  assert.equal(await pg.locator('.hc-card--own').count(), 1, 'nothing was saved, the card is still offered');
});

test('the Director\'s note shows on a draft they handed back, and the draft can be changed', { skip }, async (t) => {
  const mine = { id: 41, fieldKey: 'ancestry', slug: 'ashborn', name: 'Ashborn', summary: 'Born of ash.', description: '', status: 'draft', reviewNote: 'Make the second trait cost 2.',
    properties: { size: '1M', speed: 5, ancestry_points: 3, purchased_traits: [{ name: 'Ember Skin', cost: 1, description: 'Warm.' }] }, createdBy: 'u1' };
  const plan = PLAN({ canMake: false }, [{ key: '41', slug: 'ashborn', name: 'Ashborn', summary: 'Born of ash.', source: 'campaign', status: 'draft', mine: true, entryId: 41, properties: mine.properties }].concat(ANCESTRY()));
  const { pg, seen } = await open(t, { plan, entries: [mine] });
  const first = pg.locator('.hc-hand .hc-card[data-i]').first();
  assert.match(await first.innerText(), /Changes asked/);
  await first.click();
  await pg.waitForSelector('.hc-sheet [data-act="ownedit"]');
  assert.match(await text(pg, '.hc-sheet .hc-note'), /Your Director asked: Make the second trait cost 2/);
  await pg.click('.hc-sheet [data-act="ownedit"]');
  await pg.waitForSelector('.hc-sheet [data-oed-form]');
  assert.match(await text(pg, '.oed-asked'), /Make the second trait cost 2/);
  assert.equal(await pg.locator('[data-k="name"]').inputValue(), 'Ashborn');
  await pg.click('[data-list="purchased_traits"] [data-cost="2"]');
  await pg.click('.hc-sfoot [data-oed="send"]');
  await pg.waitForSelector('.hc-sheet', { state: 'detached' });
  const put = seen.put.find((p) => p.path.endsWith('/system-entries/41'));
  assert.equal(put.body.status, 'sent');
  assert.deepEqual(put.body.properties.purchased_traits, [{ name: 'Ember Skin', cost: 2, description: 'Warm.' }]);
  assert.match(await pg.locator('.hc-hand .hc-card[data-i]').first().innerText(), /Waiting for your Director/);
});

test('on a computer the editor takes the right pane and a second click elsewhere asks before dropping it', { skip }, async (t) => {
  const { pg, seen } = await open(t, { width: 1280 });
  await pg.click('.hc-gcard--own');
  await pg.waitForSelector('.hc-right [data-oed-form]');
  assert.match(await text(pg, '.hc-right .oed-title'), /Make your own ancestry/);
  await pg.fill('[data-k="name"]', 'Ashborn');
  // Another card asks first because of what was typed.
  await pg.locator('.hc-gcard[data-i]').nth(1).click();
  assert.match(await text(pg, '.hc-right .oed-confirm'), /Close without saving/);
  await pg.click('.hc-right [data-oed="keep"]');
  assert.equal(await pg.locator('[data-k="name"]').inputValue(), 'Ashborn');
  await pg.click('.hc-right [data-oed="draft"]');
  await pg.waitForSelector('.hc-right [data-oed-form]', { state: 'detached' });
  assert.equal(seen.post.length, 1);
  assert.match(await text(pg, '.hc-gcard[data-i]'), /Ashborn/);
  assert.match(await text(pg, '.hc-right .hc-det'), /Ashborn/, 'the new entry is the one read on the right');
  await pg.click('.hc-right [data-act="choose"]');
  assert.match(await text(pg, '.hc-rail .hc-row[data-k="ancestry"]'), /Ashborn/);
});

test('the editor follows the Calm and Off motion settings', { skip }, async (t) => {
  const { pg } = await open(t, { motion: 'off' });
  await pg.click('.hc-hand .hc-card--own');
  await pg.waitForSelector('[data-oed-form]');
  await pg.click('[data-list="purchased_traits"] [data-oed="additem"]');
  const anim = await pg.locator('.oed-item.is-new').first().evaluate((e) => getComputedStyle(e).animationName);
  assert.equal(anim, 'none', 'Off plays nothing');
});
