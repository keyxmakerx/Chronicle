// desk_carry_browser.test.mjs -- the line carries you. Runs
// static/js/desk_carry.js in a real Chromium against a stubbed htmx swap and
// checks the end states that matter: after the move there, and back, nothing
// is left half-moved (no travelling copy, no hiding flag, no held style, no
// animation) under full motion, Calm and Off; and that a slow answer, a
// failed one, the Back button, a second press and a modified click all end in
// the right place. Skips locally, and fails under CI, without a browser.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const SCRIPT = readFileSync(join(root, 'static/js/desk_carry.js'), 'utf8');

// What a press plays, from the pure part of the script (no browser needed).
test('modeOf: full carries, Calm fades, Off and a device that reduces motion are an ordinary link', () => {
  const sandbox = { module: { exports: {} } };
  vm.runInNewContext(SCRIPT, sandbox);
  const { modeOf, plainPress } = sandbox.module.exports;
  const base = { hasHtmx: true, motion: null, reducedMotion: false, czReduce: false, viewMotion: null };
  const cases = [
    [{}, 'carry'],
    [{ motion: 'calm' }, 'fade'],
    [{ czReduce: true }, 'fade'],
    [{ viewMotion: 'calm' }, 'fade'],
    [{ motion: 'off' }, ''],
    [{ reducedMotion: true }, ''],
    [{ hasHtmx: false }, ''],
  ];
  for (const [over, want] of cases) assert.equal(modeOf({ ...base, ...over }), want, JSON.stringify(over));
  assert.equal(plainPress({ button: 0 }), true);
  for (const m of ['metaKey', 'ctrlKey', 'shiftKey', 'altKey']) assert.equal(plainPress({ button: 0, [m]: true }), false, m);
  assert.equal(plainPress({ button: 1 }), false);
});

const DESK = `<div class="desk-page"><div class="desk" data-desk>
  <div class="desk-ln" data-line="people" id="ln"><div class="desk-row"><p class="desk-s" id="deskS">4 people: you and 3 players.</p>
    <span class="desk-acts"><a class="desk-q" id="open" data-carry href="/campaigns/c/members" onclick="window.Chronicle.deskCarry.go(this,event,0)">Open People</a></span>
    <button class="desk-chev" id="chev">v</button></div></div>
  <div class="desk-ln" data-line="trash"><div class="desk-row"><p class="desk-s">Trash is empty.</p></div></div>
</div></div>`;
const PEOPLE = `<div class="manage-frame"><header><a class="manage-eyebrow" id="back" data-manage-back href="/campaigns/c/dashboard" onclick="window.Chronicle.deskCarry.back(this,event,0)">Manage</a><h1>People</h1></header>
  <div class="manage-sentence" data-manage-sentence data-line="people"><p class="desk-s" id="pageS">4 people: you and 3 players.</p></div><div id="body">The members.</div></div>`;
const PAGES = { '/campaigns/c/dashboard': DESK, '/campaigns/c/members': PEOPLE };
const shell = (inner, motion) => `<!doctype html><html ${motion ? `data-motion="${motion}"` : ''}><head><style>
  #main-content{height:600px;overflow:auto;position:relative}
  .desk-fly{position:fixed;margin:0}
  html[data-desk-trip] #main-content > :not(link, script, style) { opacity: 0; }
  .is-glow{outline:1px solid red}
</style></head><body><main id="main-content">${inner}</main></body></html>`;

let shared = null;
after(async () => { if (shared) await (await shared).close(); });

async function open({ motion = '', latency = 20, fail = false, start = '/campaigns/c/dashboard', reduced = false } = {}) {
  shared = shared || pw.chromium.launch({ executablePath: chromePath });
  const ctx = await (await shared).newContext({ reducedMotion: reduced ? 'reduce' : 'no-preference' });
  const page = await ctx.newPage();
  const seen = { ajax: [], navigations: [] };
  await page.route('http://t.test/**', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/static/js/desk_carry.js') return route.fulfill({ contentType: 'text/javascript', body: SCRIPT });
    if (route.request().resourceType() === 'document') seen.navigations.push(u.pathname);
    return route.fulfill({ contentType: 'text/html', body: shell(PAGES[u.pathname] || '', motion) });
  });
  await page.goto('http://t.test' + start);
  await page.addScriptTag({ url: 'http://t.test/static/js/desk_carry.js' });
  await page.evaluate(({ latency, fail, pages }) => {
    window.__ajax = [];
    window.__added = [];
    new MutationObserver((ms) => ms.forEach((m) => m.addedNodes.forEach((n) => { if (n.classList && n.classList.contains('desk-fly')) window.__added.push(n.className); }))).observe(document.body, { childList: true });
    // A stand-in for htmx's boosted swap: waits for the answer, then for the
    // swap delay it was asked for, swaps #main-content and pushes the address.
    window.htmx = {
      trigger: (el, name) => { window.__aborted = (window.__aborted || 0) + 1; window.__abortedName = name; },
      ajax: (method, path, ctx) => new Promise((resolve, reject) => {
        window.__ajax.push(path);
        setTimeout(() => {
          if (fail) { reject(new Error('down')); return; }
          const delay = parseInt(((ctx.swap || '').match(/swap:(\d+)ms/) || [])[1] || '0', 10);
          resolve();
          setTimeout(() => {
            if (window.__abortAjax) return;
            const main = document.getElementById('main-content');
            main.innerHTML = pages[path];
            history.pushState({}, '', path);
            main.dispatchEvent(new CustomEvent('htmx:afterSettle', { bubbles: true }));
          }, delay);
        }, latency);
      }),
    };
  }, { latency, fail, pages: PAGES });
  return { ctx, page, seen };
}

const rest = (page) => page.evaluate(() => ({
  path: location.pathname,
  trip: document.documentElement.hasAttribute('data-desk-trip'),
  fly: document.querySelectorAll('.desk-fly').length,
  anims: document.getAnimations().length,
  held: [...document.querySelectorAll('#main-content *')].filter((e) => e.style.opacity || e.style.visibility || e.style.transform).length,
  visible: [...document.querySelectorAll('#main-content .desk-s')].every((e) => getComputedStyle(e).opacity === '1' && getComputedStyle(e).visibility === 'visible'),
  bodyOpacity: [...document.getElementById('main-content').children].map((e) => getComputedStyle(e).opacity),
}));

for (const motion of ['', 'calm']) {
  test(`motion ${motion || 'full'}: there and back leave nothing half-moved; only full travels`, { skip }, async () => {
    const { ctx, page } = await open({ motion });
    try {
      await page.click('#open');
      await page.waitForFunction(() => location.pathname.endsWith('/members') && !document.documentElement.hasAttribute('data-desk-trip'), null, { timeout: 4000 });
      let s = await rest(page);
      assert.deepEqual({ ...s, path: undefined }, { path: undefined, trip: false, fly: 0, anims: 0, held: 0, visible: true, bodyOpacity: ['1'] });
      assert.equal(await page.evaluate(() => document.getElementById('pageS').textContent), '4 people: you and 3 players.');
      assert.equal(await page.evaluate(() => document.activeElement.id), 'back');
      await page.click('#back');
      await page.waitForFunction(() => location.pathname.endsWith('/dashboard') && !document.documentElement.hasAttribute('data-desk-trip'), null, { timeout: 4000 });
      s = await rest(page);
      assert.deepEqual({ ...s, path: undefined }, { path: undefined, trip: false, fly: 0, anims: 0, held: 0, visible: true, bodyOpacity: ['1'] });
      assert.equal(await page.evaluate(() => document.activeElement.id), 'chev');
      const copies = await page.evaluate(() => window.__added.length);
      assert.equal(copies > 0, motion === '', motion === '' ? 'full motion carries a copy' : 'Calm is a plain fade, no travelling copy');
    } finally { await ctx.close(); }
  });
}

test('Off is an ordinary link: nothing is intercepted and nothing animates', { skip }, async () => {
  for (const mode of [{ motion: 'off' }, { reduced: true, motion: 'off' }]) {
    const { ctx, page, seen } = await open(mode);
    try {
      await Promise.all([page.waitForURL('**/members'), page.click('#open')]);
      assert.ok(seen.navigations.includes('/campaigns/c/members'));
    } finally { await ctx.close(); }
  }
});

test('a slow answer holds the lifted sentence, then lands; the end is clean', { skip }, async () => {
  const { ctx, page } = await open({ latency: 1200 });
  try {
    await page.click('#open');
    await page.waitForTimeout(500);
    const mid = await rest(page);
    assert.equal(mid.trip, true);
    assert.equal(mid.fly, 1, 'the sentence is lifted while the page is on its way');
    await page.waitForFunction(() => location.pathname.endsWith('/members') && !document.documentElement.hasAttribute('data-desk-trip'), null, { timeout: 5000 });
    const s = await rest(page);
    assert.deepEqual({ ...s, path: undefined }, { path: undefined, trip: false, fly: 0, anims: 0, held: 0, visible: true, bodyOpacity: ['1'] });
  } finally { await ctx.close(); }
});

test('a failed request puts everything back and goes to the link the ordinary way', { skip }, async () => {
  const { ctx, page, seen } = await open({ fail: true });
  try {
    await page.click('#open');
    await page.waitForFunction(() => location.pathname.endsWith('/members'), null, { timeout: 4000 });
    assert.ok(seen.navigations.includes('/campaigns/c/members'), 'the person still arrives');
    const s = await rest(page);
    assert.equal(s.trip, false);
    assert.equal(s.fly, 0);
  } finally { await ctx.close(); }
});

test('the Back button mid-move ends it: desk showing, nothing hidden, the late answer aborted', { skip }, async () => {
  const { ctx, page } = await open({ latency: 1500 });
  try {
    await page.evaluate(() => history.pushState({}, '', '/campaigns/c/dashboard?x=1'));
    await page.click('#open');
    await page.waitForTimeout(300);
    assert.equal((await rest(page)).trip, true);
    await page.evaluate(() => { window.__abortAjax = true; history.back(); });
    await page.waitForFunction(() => !document.documentElement.hasAttribute('data-desk-trip'), null, { timeout: 2000 });
    const s = await rest(page);
    assert.deepEqual({ ...s, path: undefined }, { path: undefined, trip: false, fly: 0, anims: 0, held: 0, visible: true, bodyOpacity: ['1'] });
    assert.equal(await page.evaluate(() => window.__abortedName), 'htmx:abort', 'the answer still in flight is told to stop');
    assert.equal(await page.evaluate(() => !!document.getElementById('open')), true, 'the desk is what is showing');
  } finally { await ctx.close(); }
});

test('a second press mid-move is ignored; a modified press is left to the browser', { skip }, async () => {
  const { ctx, page } = await open({ latency: 400 });
  try {
    await page.click('#open');
    await page.evaluate(() => document.getElementById('open')?.click());
    await page.waitForFunction(() => location.pathname.endsWith('/members') && !document.documentElement.hasAttribute('data-desk-trip'), null, { timeout: 4000 });
    assert.deepEqual(await page.evaluate(() => window.__ajax), ['/campaigns/c/members'], 'one request, one move');
  } finally { await ctx.close(); }
  const m = await open();
  try {
    await m.page.evaluate(() => document.getElementById('open').addEventListener('click', (e) => e.preventDefault()));
    await m.page.click('#open', { modifiers: ['Control'] });
    assert.deepEqual(await m.page.evaluate(() => window.__ajax), [], 'Ctrl-click is not carried');
  } finally { await m.ctx.close(); }
});
