// hero_tryout_browser.test.mjs — the Characters page's try-out mounts.
//
// Runs hero_tryout.js in a real Chromium on the shells the server draws. It
// pins the three places a try-out kept in the browser shows (a visitor's
// Practice section, a player's offer, a shelf row), that its text is only ever
// text, and the one request the widget can make: the player's "make it my
// hero", with the creator's own save, the creator's own draft when the server
// refuses, and the practice save for a player who already has a hero.
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
const BASE = '/campaigns/c1';
const KEY = 'chronicle:hero-tryout:c1:u9';
const js = (n) => readFileSync(join(root, 'internal/plugins/entities/static/js', n));

const REC = { v: 1, name: 'Old Marrow', picks: { ancestry: { name: 'Dwarf', options: [] }, class: { name: 'Conduit', options: [] } }, summary: 'Dwarf Conduit', savedAt: '2026-01-01T00:00:00.000Z' };
const attrs = (hasHero) => `data-campaign-id="c1" data-user-id="u9" data-base="${BASE}" data-csrf="tok" data-has-hero="${hasHero}"`;

// The shells as the server draws them (tryout_mount.templ).
const SHELL = {
  visitor: () => `<section class="pr" data-widget="hero-tryout" data-mode="visitor" ${attrs(false)}>
    <div class="pr-head"><h2>Practice</h2><span class="pr-count">kept on this device</span></div>
    <div data-tryout-slot="true"><p class="pr-words"><a class="pr-word" href="${BASE}/characters/try">Try the hero builder</a></p></div>
    <p class="pr-note">Only you can see this. It stays in this browser.</p></section>`,
  offer: (hasHero) => `<section class="pr pr-offer" data-widget="hero-tryout" data-mode="offer" hidden ${attrs(hasHero)}></section>
    <section class="pr"><div class="pr-device" data-widget="hero-tryout" data-mode="shelf" hidden ${attrs(hasHero)}></div></section>`,
};

const page = (body) => `<!doctype html><html><head><meta charset="utf-8"></head><body><main id="main-content">${body}</main>
<script>window.__went = []; window.Chronicle = { register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), go: (u) => window.__went.push(u) };</script>
<script src="/hero_tryout.js"></script></body></html>`;

async function open(t, { shell, stored = REC, reply = { status: 201, body: { id: 'h1', url: BASE + '/entities/h1' } }, init } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width: 1100, height: 900 } });
  const sent = [];
  const raw = typeof stored === 'string' ? stored : stored && JSON.stringify(stored);
  if (raw) await pg.addInitScript(([k, v]) => { if (!sessionStorage.getItem('__seeded')) { localStorage.setItem(k, v); sessionStorage.setItem('__seeded', '1'); } }, [KEY, raw]);
  if (init) await pg.addInitScript(init);
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    if (p === '/hero_tryout.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_tryout.js') });
    if (req.method() !== 'GET') {
      sent.push({ method: req.method(), path: p, csrf: req.headers()['x-csrf-token'], body: JSON.parse(req.postData()) });
      return route.fulfill({ status: reply.status, contentType: 'application/json', body: JSON.stringify(reply.body) });
    }
    return route.fulfill({ contentType: 'text/html', body: page(shell) });
  });
  await pg.goto('http://t.test/');
  return { pg, sent };
}
const stored = (pg) => pg.evaluate((k) => localStorage.getItem(k), KEY);
const text = (pg, sel) => pg.locator(sel).first().innerText();

test('a visitor with nothing built sees the way in and the note', { skip }, async (t) => {
  const { pg } = await open(t, { shell: SHELL.visitor(), stored: null });
  assert.equal((await text(pg, '.pr-words a')).trim(), 'Try the hero builder');
  assert.equal(await pg.locator('.pr-row').count(), 0);
  assert.match(await text(pg, '.pr-note'), /^Only you can see this\. It stays in this browser\./);
});

test('a visitor with one built sees it as a row, with change and throw away', { skip }, async (t) => {
  const { pg, sent } = await open(t, { shell: SHELL.visitor() });
  assert.equal((await text(pg, '.pr-row .pr-main b')).trim(), 'Old Marrow');
  assert.equal((await text(pg, '.pr-row .pr-main small')).trim(), 'Dwarf Conduit');
  assert.equal((await text(pg, '.pr-row .pr-mark')).trim(), 'Practice');
  assert.equal(await pg.locator('.pr-row .pr-words a').getAttribute('href'), BASE + '/characters/try');
  assert.equal((await text(pg, '.pr-row .pr-words a')).trim(), 'Change it');
  assert.match(await text(pg, '.pr-note'), /Only you can see this/);
  assert.deepEqual(sent, []);

  await pg.click('.pr-words >> text=Throw it away');
  assert.match(await text(pg, '.pr-confirm'), /Throw away Old Marrow\? It can.t be brought back\./);
  await pg.click('.pr-confirm >> text=Keep it');
  assert.ok(await stored(pg), 'kept after "Keep it"');
  await pg.click('.pr-words >> text=Throw it away');
  await pg.click('.pr-confirm .btn-danger');
  assert.equal(await stored(pg), null, 'gone after confirming');
  assert.equal((await text(pg, '.pr-words a')).trim(), 'Try the hero builder', 'the way in is back');
  assert.deepEqual(sent, [], 'throwing it away sends nothing');
});

test('what storage holds is shown as text, never as markup', { skip }, async (t) => {
  const evil = { ...REC, name: '<img src=x onerror=1>', summary: '<b id="boom">bold</b>' };
  for (const [mode, shell] of [['visitor', SHELL.visitor()], ['offer', SHELL.offer(false)]]) {
    const { pg } = await open(t, { shell, stored: evil });
    assert.equal(await pg.locator('main img').count(), 0, mode + ': no element was made from the name');
    assert.equal(await pg.locator('#boom').count(), 0, mode + ': no element was made from the summary');
    assert.match(await pg.locator('main').innerText(), /<img src=x onerror=1>/, mode + ': the name is shown as written');
  }
});

test('storage that is not ours or is damaged shows nothing', { skip }, async (t) => {
  for (const bad of ['{"v":2,"name":"X","picks":{}}', 'not json', '{"v":1,"name":"","picks":{}}', '{"v":1,"name":"X","picks":[]}', 'null']) {
    const { pg } = await open(t, { shell: SHELL.offer(false), stored: bad });
    assert.equal(await pg.locator('[data-mode="offer"]').isVisible(), false, bad);
  }
});

test('after becoming a player: the offer, with Make it my hero and Not now', { skip }, async (t) => {
  const { pg } = await open(t, { shell: SHELL.offer(false) });
  assert.equal((await text(pg, '.pr-offer__line')).trim(), 'You built Old Marrow while you were looking around.');
  assert.equal((await text(pg, '.pr-offer .btn-primary')).trim(), 'Make Old Marrow my hero');
  assert.equal((await text(pg, '.pr-offer .pr-word')).trim(), 'Not now');
  // The same try-out is the last row of the Practice shelf.
  assert.equal((await text(pg, '.pr-device .pr-mark')).trim(), 'On this device');
  assert.equal((await text(pg, '.pr-device .pr-main b')).trim(), 'Old Marrow');
});

test('Make it my hero sends the creator\'s own save, then forgets the try-out and goes to the hero', { skip }, async (t) => {
  const { pg, sent } = await open(t, { shell: SHELL.offer(false) });
  await pg.click('.pr-offer .btn-primary');
  await pg.waitForFunction(() => window.__went.length > 0);
  assert.deepEqual(sent, [{ method: 'POST', path: BASE + '/characters/new', csrf: 'tok', body: { name: 'Old Marrow', picks: REC.picks } }]);
  assert.deepEqual(await pg.evaluate(() => window.__went), [BASE + '/entities/h1']);
  assert.equal(await stored(pg), null, 'the try-out is removed once the hero exists');
});

test('a pick that became a request to the Director is told the way the creator tells it', { skip }, async (t) => {
  const reply = { status: 201, body: { id: 'h1', url: BASE + '/entities/h1', requests: [], requestFailed: ['class'] } };
  const { pg } = await open(t, { shell: SHELL.offer(false), reply });
  await pg.click('.pr-offer .btn-primary');
  await pg.waitForSelector('.pr-offer .pr-note');
  assert.match(await text(pg, '.pr-offer .pr-note'), /Your hero is made, but a pick for class could not be sent to your Director/);
  assert.equal(await pg.locator('.pr-offer a.pr-word').getAttribute('href'), BASE + '/entities/h1');
  assert.deepEqual(await pg.evaluate(() => window.__went), [], 'it stays to say so');
  assert.equal(await stored(pg), null);
});

test('a refusal keeps the try-out and opens the creator with it filled in', { skip }, async (t) => {
  const reply = { status: 400, body: { message: '"Conduit" is not on the class list' } };
  const { pg, sent } = await open(t, { shell: SHELL.offer(false), reply });
  await pg.click('.pr-offer .btn-primary');
  await pg.waitForFunction(() => window.__went.length > 0);
  assert.equal(sent.length, 1);
  assert.deepEqual(await pg.evaluate(() => window.__went), [BASE + '/characters/new']);
  assert.deepEqual(JSON.parse(await pg.evaluate(() => localStorage.getItem('chronicle:hero-draft:c1'))), { picks: REC.picks, name: 'Old Marrow' });
  assert.deepEqual(JSON.parse(await pg.evaluate(() => sessionStorage.getItem('chronicle:hero-handoff:c1'))), { message: '"Conduit" is not on the class list' });
  assert.ok(await stored(pg), 'the try-out is not lost');
});

test('a player who already has a hero keeps it as a practice hero', { skip }, async (t) => {
  const reply = { status: 201, body: { id: 'p1', url: BASE + '/characters/practice/p1' } };
  const { pg, sent } = await open(t, { shell: SHELL.offer(true), reply });
  assert.equal((await text(pg, '.pr-offer .btn-primary')).trim(), 'Keep Old Marrow as a practice hero');
  await pg.click('.pr-offer .btn-primary');
  await pg.waitForFunction(() => window.__went.length > 0);
  assert.deepEqual(sent, [{ method: 'POST', path: BASE + '/characters/practice', csrf: 'tok', body: { name: 'Old Marrow', picks: REC.picks } }]);
  assert.deepEqual(await pg.evaluate(() => window.__went), [BASE + '/characters/practice/p1']);
  assert.equal(await stored(pg), null);
});

test('a practice refusal opens the practice creator with the try-out and the sentence', { skip }, async (t) => {
  const reply = { status: 409, body: { message: 'You have three practice heroes. Throw one away to start another.' } };
  const { pg } = await open(t, { shell: SHELL.offer(true), reply });
  await pg.click('.pr-offer .btn-primary');
  await pg.waitForFunction(() => window.__went.length > 0);
  assert.deepEqual(await pg.evaluate(() => window.__went), [BASE + '/characters/practice/new']);
  assert.deepEqual(JSON.parse(await pg.evaluate(() => localStorage.getItem('chronicle:hero-practice:c1'))), { picks: REC.picks, name: 'Old Marrow' });
  assert.match(await pg.evaluate(() => sessionStorage.getItem('chronicle:hero-handoff:c1')), /three practice heroes/);
  assert.ok(await stored(pg));
});

test('Not now hides the offer for this session and keeps the try-out on the shelf', { skip }, async (t) => {
  const { pg, sent } = await open(t, { shell: SHELL.offer(false) });
  await pg.click('.pr-offer .pr-word');
  assert.equal(await pg.locator('.pr-offer').isVisible(), false);
  assert.equal(await pg.evaluate(() => sessionStorage.getItem('chronicle:hero-tryout-hidden:c1:u9')), '1');
  assert.ok(await stored(pg), 'the try-out is kept');
  assert.equal(await pg.locator('.pr-device').isVisible(), true, 'and still on the shelf');
  await pg.reload();
  assert.equal(await pg.locator('.pr-offer').isVisible(), false, 'still hidden after a reload in the same session');
  assert.deepEqual(sent, []);
});

test('the shelf row can be thrown away, which also clears the offer', { skip }, async (t) => {
  const { pg, sent } = await open(t, { shell: SHELL.offer(false) });
  await pg.click('.pr-device >> text=Throw it away');
  await pg.click('.pr-device .btn-danger');
  assert.equal(await stored(pg), null);
  assert.equal(await pg.locator('.pr-offer').isVisible(), false);
  assert.equal(await pg.locator('.pr-device').isVisible(), false);
  assert.deepEqual(sent, []);
});

test('a lost connection keeps the try-out and says so', { skip }, async (t) => {
  const { pg } = await open(t, { shell: SHELL.offer(false) });
  await pg.route('http://t.test/campaigns/**', (r) => r.abort());
  await pg.route('**/characters/new', (r) => r.abort());
  await pg.click('.pr-offer .btn-primary');
  await pg.waitForSelector('.pr-offer .pr-note');
  assert.match(await text(pg, '.pr-offer .pr-note'), /could not be sent/);
  assert.ok(await stored(pg));
  assert.equal(await pg.locator('.pr-offer .btn-primary').isDisabled(), false);
});
