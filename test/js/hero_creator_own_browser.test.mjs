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
import { bordersAttrs, pickerScripts, serveBorders, contrast, INFO } from './_card_border_fixture.mjs';

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

// Site tokens for the tests that measure colour.
const TOKENS = '*,::before,::after{box-sizing:border-box}:root{--color-accent:#4f46e5;--color-accent-link:#4338ca;--color-bg-primary:#f9fafb;--color-bg-secondary:#f3f4f6;--color-card-bg:#fff;--color-bg-tertiary:#f3f4f6;--color-border:#e5e7eb;--color-text-primary:#111827;--color-text-secondary:#4b5563;--color-accent-rgb:79 70 229;--dur-micro:120ms;--dur-standard:200ms;--ease-out:cubic-bezier(.16,1,.3,1);--ease-in:cubic-bezier(.4,0,1,1)}'
  + 'html.dark{--color-accent-link:#a5b4fc;--color-bg-primary:#111827;--color-bg-secondary:#1f2937;--color-card-bg:#1f2937;--color-bg-tertiary:#374151;--color-border:#374151;--color-text-primary:#f9fafb;--color-text-secondary:#c4c9d2}';
const html = (attrs, motion, o = {}) => `<!doctype html><html data-gm-term="Director"${motion ? ` data-motion="${motion}"` : ''}${o.dark ? ' class="dark"' : ''}><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}${o.tokens ? TOKENS : ''}</style><link rel="stylesheet" href="/hero_creator.css">${o.stage ? '<link rel="stylesheet" href="/hero_stage.css"><link rel="stylesheet" href="/hero_borders.css">' : ''}</head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true" ${attrs}><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director',
  apiFetch: (url, o) => fetch(url, { method: (o && o.method) || 'GET', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': (o && o.csrfToken) || '' }, body: o && o.body ? JSON.stringify(o.body) : undefined }) };</script>
<script src="/hero_rules_lines.js"></script>${o.scripts === false ? '' : pickerScripts}<script src="/own_entry_editor.js"></script>${o.stage ? '<script src="/hero_stage.js"></script>' : ''}<script src="/hero_creator.js"></script></body></html>`;

// A stand-in for the entry API: the player's one permission, the entries they
// have written, and what a save answers.
async function open(t, o = {}) {
  const { width = 390, attrs: baseAttrs = '', plan = PLAN(), grants = [{ id: 1, fieldKey: 'ancestry', userId: 'u1', budget: 3 }], entries = [], failWith = null, motion = null,
    lib = false, pick = true, stage = false, scripts = true, tokens = false, dark = false, reduced = true } = o;
  // lib: the page hands the form the border library; pick: and the picker's lists; scripts: and the two scripts.
  const attrs = baseAttrs + (stage ? ' data-look="stage"' : '') + (lib ? bordersAttrs({ pick }) : '');
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height: 844 }, reducedMotion: reduced ? 'reduce' : 'no-preference' });
  const errors = [];
  pg.on('pageerror', (e) => errors.push(e.message));
  const seen = { post: [], put: [], created: [], errors };
  const store = entries.slice();
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname, m = req.method();
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (serveBorders(route, p)) return;
    if (p === '/hero_stage.js') return route.fulfill({ contentType: 'text/javascript', body: read('js/hero_stage.js') });
    if (p === '/hero_stage.css') return route.fulfill({ contentType: 'text/css', body: read('css/hero_stage.css') });
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
    return route.fulfill({ contentType: 'text/html', body: html(attrs, motion, { stage, scripts, tokens, dark }) });
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
  // Said once, by the field, and tied to it.
  assert.equal(await pg.locator('.hc-sheet :text("Give it a name")').count() + await pg.locator('.hc-sfoot :text("Give it a name")').count(), 1, 'the missing name is said once');
  assert.equal(await pg.getAttribute('[data-k="name"]', 'aria-invalid'), 'true');
  assert.equal(await pg.getAttribute('[data-k="name"]', 'aria-describedby'), 'oed-n-err');
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
  if (!(await pg.locator('.hc-drape').count())) await pg.click('[data-act="steps"]');
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

// --- The Card border row ---

const FIELD = '[data-hcw-f]';
const mineWith = (props, extra = {}) => {
  const e = { id: 41, fieldKey: 'ancestry', slug: 'ashborn', name: 'Ashborn', summary: 'Born of ash.', description: '', status: 'draft', createdBy: 'u1',
    properties: { size: '1M', speed: 5, ancestry_points: 3, ...props }, ...extra };
  const plan = PLAN({ canMake: false }, [{ key: '41', slug: 'ashborn', name: 'Ashborn', summary: 'Born of ash.', source: 'campaign', status: 'draft', mine: true, entryId: 41, properties: e.properties }].concat(ANCESTRY()));
  return { plan, entries: [e] };
};
// Opens a new entry's form (phone sheet), or edits the player's one.
const newForm = async (o) => { const r = await open(o.t, o.o); await r.pg.click(o.o && o.o.width > 900 ? '.hc-gcard--own' : '.hc-hand .hc-card--own'); await r.pg.waitForSelector(FIELD); return r; };
const editForm = async (t, props, o = {}) => {
  const r = await open(t, { ...mineWith(props), ...o });
  await r.pg.locator('.hc-hand .hc-card[data-i]').first().click();
  await r.pg.waitForSelector('.hc-sheet [data-act="ownedit"]');
  await r.pg.click('.hc-sheet [data-act="ownedit"]');
  await r.pg.waitForSelector('.hc-sheet [data-oed-form]');
  return r;
};
const row = (pg) => pg.evaluate(() => {
  const f = document.querySelector('[data-hcw-f]');
  const sm = f.querySelector('[data-frow] .hc-gcard'), live = f.querySelector('[data-flive]');
  return { name: f.querySelector('[data-fname]').textContent, col: f.querySelector('[data-fcol]').textContent, word: f.querySelector('[data-fword]').textContent,
    expanded: f.querySelector('[data-hcw="fold"]').getAttribute('aria-expanded'),
    rowTrim: sm.getAttribute('data-trim'), rowK: sm.style.getPropertyValue('--k').trim(), liveTrim: live.getAttribute('data-trim'), liveK: live.style.getPropertyValue('--k').trim(),
    liveName: f.querySelector('[data-flname]').textContent, liveSum: f.querySelector('[data-flsum]').textContent, liveKind: f.querySelector('[data-flive] .hc-top').textContent };
});
const shown = (pg, sel) => pg.locator(sel).first().isVisible();

test('Card border: one closed row right after "A few words about it", reading None, unfolding in place and folding back', { skip }, async (t) => {
  const { pg, seen } = await newForm({ t, o: { lib: true, stage: true } });
  const after = await pg.evaluate(() => document.querySelector('#oed-s').closest('.oed-field').nextElementSibling.matches('[data-hcw-f]'));
  assert.ok(after, 'right after the few words');
  assert.match(await text(pg, FIELD + ' .oed-k'), /^Card border$/);
  let r = await row(pg);
  assert.deepEqual([r.name, r.col, r.word, r.expanded], ['None', 'A plain card', 'Change', 'false']);
  assert.equal(await shown(pg, FIELD + ' .hcw-pk'), false, 'closed, the picker is out of reach');
  await pg.click(FIELD + ' [data-hcw="fold"]');
  r = await row(pg);
  assert.deepEqual([r.word, r.expanded], ['Done', 'true']);
  assert.equal(await shown(pg, FIELD + ' .hcw-t[data-b="fire"]'), true);
  assert.equal(await pg.locator(FIELD + ' .hcw-t').count(), 17, 'None and sixteen samples');
  assert.equal(await pg.locator(FIELD + ' .hcw-t[data-b=""]').getAttribute('aria-pressed'), 'true');
  assert.equal(await pg.locator(FIELD + ' [data-lay="field"]').count(), 1);
  await pg.click(FIELD + ' [data-hcw="fold"]');
  r = await row(pg);
  assert.deepEqual([r.word, r.expanded], ['Change', 'false']);
  assert.equal(await shown(pg, FIELD + ' .hcw-t[data-b="fire"]'), false);
  assert.deepEqual(seen.errors, []);
});

test('Card border: a choice repaints the row, the live card and the words at once; the card follows the typing', { skip }, async (t) => {
  const { pg, seen } = await newForm({ t, o: { lib: true, stage: true, tokens: true } });
  await pg.click(FIELD + ' [data-hcw="fold"]');
  let r = await row(pg);
  assert.equal(r.liveKind, 'Ancestry');
  assert.equal(r.liveName, 'Your ancestry');
  assert.equal(r.liveSum, 'What a player reads before opening it.');
  await pg.fill('[data-k="name"]', 'Ashborn');
  await pg.fill('[data-k="summary"]', 'Born of ash and ember.');
  r = await row(pg);
  assert.deepEqual([r.liveName, r.liveSum], ['Ashborn', 'Born of ash and ember.']);
  await pg.click(FIELD + ' .hcw-t[data-b="fire"]');
  r = await row(pg);
  assert.deepEqual([r.name, r.col, r.rowTrim, r.liveTrim, r.rowK, r.liveK], ['Fire', 'Its own colour', 'fire', 'fire', INFO.fire[1], INFO.fire[1]]);
  await pg.click(FIELD + ' .hcw-c[data-c="#b3243a"]');
  r = await row(pg);
  assert.deepEqual([r.name, r.col, r.rowK, r.liveK], ['Fire', 'Crimson', '#b3243a', '#b3243a']);
  assert.equal(await pg.locator(FIELD + ' .hcw-c[data-c="#b3243a"]').getAttribute('aria-pressed'), 'true');
  await pg.click(FIELD + ' .hcw-c--own');
  assert.equal((await row(pg)).col, 'Its own colour');
  await pg.click(FIELD + ' .hcw-t[data-b=""]');
  r = await row(pg);
  assert.deepEqual([r.name, r.col, r.rowTrim, r.liveTrim], ['None', 'A plain card', null, null]);
  assert.deepEqual(seen.errors, []);
});

test('Card border: saving sends card_border and card_colour; the border’s own colour sends no colour; None sends neither', { skip }, async (t) => {
  const save = async (steps) => {
    const { pg, seen } = await newForm({ t, o: { lib: true, stage: true } });
    await writeDraft(pg);
    await pg.click(FIELD + ' [data-hcw="fold"]');
    await steps(pg);
    await pg.click('.hc-sfoot [data-oed="draft"]');
    await pg.waitForSelector('.hc-sheet', { state: 'detached' });
    assert.equal(seen.post.length, 1);
    return seen.post[0].properties;
  };
  let p = await save(async (pg) => { await pg.click(FIELD + ' .hcw-t[data-b="fire"]'); await pg.click(FIELD + ' .hcw-c[data-c="#B3243A" i]'); });
  assert.equal(p.card_border, 'fire');
  assert.equal(p.card_colour, '#b3243a');
  p = await save(async (pg) => { await pg.click(FIELD + ' .hcw-t[data-b="ice"]'); });
  assert.equal(p.card_border, 'ice');
  assert.ok(!('card_colour' in p), 'its own colour is not a stored colour');
  p = await save(async () => {});
  assert.ok(!('card_border' in p) && !('card_colour' in p), 'None writes neither');
  p = await save(async (pg) => { await pg.click(FIELD + ' .hcw-t[data-b="fire"]'); await pg.click(FIELD + ' .hcw-t[data-b=""]'); });
  assert.ok(!('card_border' in p) && !('card_colour' in p), 'choosing None again writes neither');
});

test('Card border: an edit PUTs the choice, and reopening shows it with no Card border or Card colour fact', { skip }, async (t) => {
  const { pg, seen } = await editForm(t, { card_border: 'storm', card_colour: '#2FB5A4' }, { lib: true, stage: true });
  let r = await row(pg);
  assert.deepEqual([r.name, r.col, r.rowTrim, r.rowK], ['Storm', 'Teal', 'storm', '#2fb5a4']);
  const labels = await pg.locator('.oed-fact input[data-part="label"]').evaluateAll((els) => els.map((e) => e.value));
  assert.ok(!labels.some((l) => /card/i.test(l)), 'not among the facts: ' + labels);
  await pg.click(FIELD + ' [data-hcw="fold"]');
  await pg.click(FIELD + ' .hcw-t[data-b="vine"]');
  await pg.click('.hc-sfoot [data-oed="draft"]');
  await pg.waitForSelector('.hc-sheet', { state: 'detached' });
  const put = seen.put.find((x) => x.path.endsWith('/system-entries/41')).body.properties;
  assert.equal(put.card_border, 'vine');
  assert.equal(put.card_colour, '#2fb5a4', 'the colour the writer had carries to the new border');
  // The saved entry's card wears it at once, and is reopened with the same choice.
  assert.equal(await pg.locator('.hc-hand .hc-card[data-i]').first().getAttribute('data-trim'), 'vine');
  await pg.locator('.hc-hand .hc-card[data-i]').first().click();
  await pg.click('.hc-sheet [data-act="ownedit"]');
  await pg.waitForSelector(FIELD);
  r = await row(pg);
  assert.deepEqual([r.name, r.col], ['Vine and wood', 'Teal']);
  assert.ok(!(await pg.locator('.oed-fact input[data-part="label"]').evaluateAll((els) => els.some((e) => /card/i.test(e.value)))));
  assert.deepEqual(seen.errors, []);
});

test('Card border: a border that left the library reads None and goes back untouched; a hostile colour reaches no markup', { skip }, async (t) => {
  const { pg, seen } = await editForm(t, { card_border: 'gone-border', card_colour: '#abcdef' }, { lib: true, stage: true });
  const r = await row(pg);
  assert.deepEqual([r.name, r.col, r.rowTrim], ['None', 'A plain card', null]);
  assert.equal(await pg.locator('.oed-fact').count() >= 2, true);
  await pg.click('.hc-sfoot [data-oed="draft"]');
  await pg.waitForSelector('.hc-sheet', { state: 'detached' });
  const p = seen.put.find((x) => x.path.endsWith('/system-entries/41')).body.properties;
  assert.equal(p.card_border, 'gone-border');
  assert.equal(p.card_colour, '#abcdef');
  const bad = await editForm(t, { card_border: 'fire', card_colour: 'red;background:url(x)' }, { lib: true, stage: true });
  const markup = await bad.pg.evaluate(() => document.querySelector('[data-hcw-f]').outerHTML + document.querySelector('.hc-sheet').outerHTML);
  assert.ok(!/url\(x\)|red;background/.test(markup), 'the stored colour is not in the page');
  const br = await row(bad.pg);
  assert.deepEqual([br.name, br.col, br.rowK], ['Fire', 'Its own colour', INFO.fire[1]], 'the border’s own colour stands in');
  // Picking changes what is saved: the unusable value is then replaced.
  const again = await editForm(t, { card_border: 'gone-border', card_colour: '#abcdef' }, { lib: true, stage: true });
  await again.pg.click(FIELD + ' [data-hcw="fold"]');
  await again.pg.click(FIELD + ' .hcw-t[data-b="ice"]');
  await again.pg.click('.hc-sfoot [data-oed="draft"]');
  await again.pg.waitForFunction(() => document.querySelectorAll('.hc-sheet').length === 0);
  const p2 = again.seen.put.filter((x) => x.path.endsWith('/system-entries/41')).pop().body.properties;
  assert.deepEqual([p2.card_border, 'card_colour' in p2], ['ice', false]);
});

test('Card border: a fact the writer names Card border or Card colour is refused in plain words', { skip }, async (t) => {
  for (const label of ['Card border', 'card colour']) {
    const { pg, seen } = await newForm({ t, o: { lib: true, stage: true } });
    await pg.fill('[data-k="name"]', 'Ashborn');
    await pg.click('[data-oed="addfact"]');
    await pg.locator('.oed-fact input[data-part="label"]').last().fill(label);
    await pg.locator('.oed-fact input[data-part="value"]').last().fill('fire');
    await pg.click('.hc-sfoot [data-oed="draft"]');
    await pg.waitForSelector('.hc-sfoot [data-error]:not([hidden])');
    assert.match(await text(pg, '.hc-sfoot [data-error]'), /is set in the Card border field, not as a fact/);
    assert.equal(seen.post.length, 0);
  }
});

test('Card border: without a library on the mount, or without the picker script, there is no field and the stored values are kept', { skip }, async (t) => {
  for (const o of [{ lib: true, pick: false }, { lib: false }, { lib: true, scripts: false }]) {
    const { pg, seen } = await editForm(t, { card_border: 'fire', card_colour: '#abcdef' }, { stage: true, ...o });
    assert.equal(await pg.locator(FIELD).count(), 0, JSON.stringify(o));
    const labels = await pg.locator('.oed-fact input[data-part="label"]').evaluateAll((els) => els.map((e) => e.value));
    assert.ok(!labels.some((l) => /card/i.test(l)), 'not shown as facts either');
    await pg.click('.hc-sfoot [data-oed="draft"]');
    await pg.waitForSelector('.hc-sheet', { state: 'detached' });
    const p = seen.put.find((x) => x.path.endsWith('/system-entries/41')).body.properties;
    assert.deepEqual([p.card_border, p.card_colour], ['fire', '#abcdef'], JSON.stringify(o));
    assert.deepEqual(seen.errors, [], JSON.stringify(o));
  }
});

test('Card border: the stage look draws a just-written entry’s border; the classic look draws none and lists no such fact', { skip }, async (t) => {
  for (const stage of [true, false]) {
    const { pg, seen } = await newForm({ t, o: { lib: true, stage } });
    await writeDraft(pg);
    await pg.click(FIELD + ' [data-hcw="fold"]');
    await pg.click(FIELD + ' .hcw-t[data-b="fire"]');
    await pg.click('.hc-sfoot [data-oed="draft"]');
    await pg.waitForSelector('.hc-sheet', { state: 'detached' });
    assert.equal(seen.post[0].properties.card_border, 'fire');
    const first = pg.locator('.hc-hand .hc-card[data-i]').first();
    assert.match(await first.innerText(), /Ashborn/);
    assert.equal(await first.getAttribute('data-trim'), stage ? 'fire' : null, stage ? 'the card wears it' : 'classic draws no border');
    assert.equal(await pg.locator('[data-trim]').count() > 0, stage);
    assert.ok(!/card border|card colour/i.test(await pg.locator('.hc-app').innerText()), 'no such fact in the creator');
    assert.deepEqual(seen.errors, []);
  }
});

test('Card border: the hint names the campaign’s word for the Director', { skip }, async (t) => {
  const { pg } = await newForm({ t, o: { lib: true, stage: true } });
  assert.equal((await text(pg, FIELD + ' .oed-hint')).trim(), 'How your card looks in the creator. Your Director can change it.');
});

test('Card border: at 360 wide every sample, swatch and the row are 44 high, and nothing scrolls sideways', { skip }, async (t) => {
  const { pg } = await editForm(t, { card_border: 'fire', card_colour: '#b3243a' }, { width: 360, lib: true, stage: true, tokens: true });
  await pg.click(FIELD + ' [data-hcw="fold"]');
  const m = await pg.evaluate(() => {
    const h = (sel) => Array.from(document.querySelectorAll(sel)).map((e) => { const r = e.getBoundingClientRect(); return [Math.round(r.width), Math.round(r.height)]; });
    const sheet = document.querySelector('.hc-sheet [data-ssc]');
    return { row: h('[data-hcw="fold"]'), tiles: h('[data-hcw-f] .hcw-t'), swatches: h('[data-hcw-f] .hcw-c'),
      doc: document.documentElement.scrollWidth - document.documentElement.clientWidth, sheet: sheet.scrollWidth - sheet.clientWidth };
  });
  assert.ok(m.row[0][1] >= 44, 'row ' + m.row);
  assert.ok(m.tiles.length === 17 && m.tiles.every((x) => x[1] >= 44), 'tiles ' + JSON.stringify(m.tiles));
  assert.ok(m.swatches.length >= 12 && m.swatches.every((x) => x[0] >= 44 && x[1] >= 44), 'swatches ' + JSON.stringify(m.swatches));
  assert.ok(m.doc <= 0 && m.sheet <= 0, 'no sideways scroll ' + JSON.stringify(m));
});

test('Card border: keyboard only — Tab reaches the row, Enter unfolds it, Tab walks colours then samples, Enter picks, with a focus ring', { skip }, async (t) => {
  const { pg } = await editForm(t, { card_border: 'fire', card_colour: '#d9622b' }, { lib: true, stage: true, tokens: true });
  await pg.focus('#oed-s');
  await pg.keyboard.press('Tab');
  assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('data-hcw')), 'fold', 'Tab from the few words reaches the row');
  const ring = await pg.evaluate(() => { const s = getComputedStyle(document.activeElement); return [s.outlineStyle, parseFloat(s.outlineWidth)]; });
  assert.ok(ring[0] !== 'none' && ring[1] >= 2, 'a visible focus ring ' + ring);
  await pg.keyboard.press('Enter');
  assert.equal(await pg.getAttribute(FIELD + ' [data-hcw="fold"]', 'aria-expanded'), 'true');
  const order = [];
  for (let i = 0; i < 40; i++) {
    await pg.keyboard.press('Tab');
    const k = await pg.evaluate(() => { const a = document.activeElement; return a.matches('.hcw-c') ? 'c' : a.matches('[data-any]') ? 'any' : a.matches('.hcw-t') ? 't:' + a.getAttribute('data-b') : 'x'; });
    order.push(k);
    if (k === 't:vine') break;
  }
  const firstTile = order.findIndex((k) => k.startsWith('t:'));
  assert.ok(firstTile > 10 && order.slice(0, firstTile).every((k) => k === 'c' || k === 'any'), 'colours first: ' + order.join(' '));
  assert.equal(order[firstTile], 't:', 'then the samples, None first');
  // Enter on a focused sample picks it.
  await pg.focus(FIELD + ' .hcw-t[data-b="ice"]');
  await pg.keyboard.press('Enter');
  assert.equal(await pg.getAttribute(FIELD + ' .hcw-t[data-b="ice"]', 'aria-pressed'), 'true');
  assert.equal((await row(pg)).name, 'Ice');
});

test('Card border: the row’s words and the hint reach 4.5:1 in light and dark', { skip }, async (t) => {
  for (const dark of [false, true]) {
    const { pg } = await editForm(t, { card_border: 'fire', card_colour: '#b3243a' }, { lib: true, stage: true, tokens: true, dark });
    const got = await pg.evaluate(() => {
      const rgb = (c) => (c.match(/[\d.]+/g) || []).slice(0, 4).map(Number);
      const bg = (el) => { for (let e = el; e; e = e.parentElement) { const c = rgb(getComputedStyle(e).backgroundColor); if (c.length === 3 || (c.length === 4 && c[3] > 0.95)) return c.slice(0, 3); } return [255, 255, 255]; };
      const pairs = {};
      [['name', '[data-fname]'], ['colour', '[data-fcol]'], ['word', '[data-fword]'], ['hint', '[data-hcw-f] .oed-hint'], ['label', '[data-hcw-f] .oed-k']].forEach(([k, sel]) => {
        const el = document.querySelector(sel);
        pairs[k] = [rgb(getComputedStyle(el).color).slice(0, 3), bg(el)];
      });
      return pairs;
    });
    for (const [k, [fg, bgc]] of Object.entries(got)) assert.ok(contrast(fg, bgc) >= 4.5, `${dark ? 'dark' : 'light'} ${k} ${contrast(fg, bgc).toFixed(2)}`);
  }
});

test('Card border: the fold follows the motion level — Off is open at once, full eases', { skip }, async (t) => {
  // Seconds; Off may be the site's 1ms floor, which is still no movement.
  const dur = (pg) => pg.evaluate(() => { const f = document.querySelector('[data-ffold]'); f.classList.add('is-open'); const d = parseFloat(getComputedStyle(f).transitionDuration); f.classList.remove('is-open'); return d; });
  const off = await newForm({ t, o: { lib: true, stage: true, motion: 'off', reduced: false } });
  assert.ok(await dur(off.pg) <= 0.001);
  await off.pg.click(FIELD + ' [data-hcw="fold"]');
  assert.equal(await off.pg.evaluate(() => getComputedStyle(document.querySelector('[data-ffold]')).gridTemplateRows !== '0px'), true, 'open at once');
  const calm = await newForm({ t, o: { lib: true, stage: true, motion: 'calm', reduced: false } });
  assert.ok(await dur(calm.pg) <= 0.001, 'Calm opens at once too');
  const full = await newForm({ t, o: { lib: true, stage: true, reduced: false } });
  assert.ok(await dur(full.pg) >= 0.15, 'full motion measures its content and eases');
});

// ROW_SHOTS=<dir> also writes the review pictures of the row (skipped otherwise).
test('Card border: review pictures', { skip: skip || !process.env.ROW_SHOTS }, async (t) => {
  const shot = async (name, o, closed) => {
    const { pg } = await newForm({ t, o: { lib: true, stage: true, tokens: true, reduced: true, ...o } });
    await pg.setViewportSize({ width: o.width, height: o.width > 900 ? 800 : 844 });
    await pg.fill('[data-k="name"]', 'Ashborn');
    await pg.fill('[data-k="summary"]', 'Born of ash and ember.');
    await pg.click(FIELD + ' [data-hcw="fold"]');
    await pg.click(FIELD + ' .hcw-t[data-b="fire"]');
    await pg.click(FIELD + ' .hcw-c[data-c="#2fb5a4"]');
    await pg.waitForTimeout(600);
    if (closed) { await pg.click(FIELD + ' [data-hcw="fold"]'); await pg.waitForTimeout(300); }
    await pg.locator(FIELD + ' .oed-k, ' + FIELD + ' .oe-k').first().evaluate((e) => e.scrollIntoView({ block: 'start' }));
    await pg.waitForTimeout(150);
    await pg.screenshot({ path: join(process.env.ROW_SHOTS, name) });
  };
  await shot('row-phone-light.png', { width: 390 });
  await shot('row-phone-dark.png', { width: 390, dark: true });
  await shot('row-desk-light.png', { width: 1280 });
  await shot('row-closed-phone.png', { width: 390 }, true);
});
