// desk_fold_browser.test.mjs -- a desk line unfolds where it stands. Runs
// static/js/desk_fold.js in a real Chromium against stubbed fragments: the
// toggle's aria wiring, Enter/Space/Escape, the one fetch, the end state of
// an interrupted open under full, Calm and Off, and what an action inside the
// fold does (the route is called, the row leaves, the sentence is worded
// again). Skips locally, and fails under CI, without a browser.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const SCRIPT = readFileSync(join(root, 'static/js/desk_fold.js'), 'utf8');

const call = (m) => `window.Chronicle.deskFold.${m}(this,event)`;
const PAGE = (motion) => `<!doctype html><html ${motion ? `data-motion="${motion}"` : ''}><body>
<div data-desk>
 <div class="desk-ln" data-line="trash">
  <div class="desk-row" onclick="${call('row')}">
   <p class="desk-s">Trash holds 2 items for 30 days.</p>
   <span class="desk-acts"><a class="desk-q" id="full" href="/trash">Trash</a></span>
   <button type="button" class="desk-chev" id="chev" aria-expanded="false" aria-controls="desk-fold-trash" aria-label="Unfold Trash" data-title="Trash" onclick="${call('toggle')}" onkeydown="${call('key')}"></button>
  </div>
  <div class="desk-fold" id="desk-fold-trash" role="region" aria-label="Trash" hidden data-url="/campaigns/c/dashboard/lines/trash/fold" onkeydown="${call('key')}"></div>
 </div>
</div>
<button id="after">after</button>
</body></html>`;

const FOLD = `<div class="desk-fold-in">
 <div class="desk-fr" id="r1"><span class="desk-ft"><b>Old Vess Tollhouse</b></span><button type="button" class="desk-q desk-act" id="back1" data-url="/campaigns/c/trash/p1/restore" data-method="POST" data-busy="Bringing back" onclick="${call('act')}">Bring back</button></div>
 <div class="desk-fr desk-inv"><input type="email" data-desk-invite-email aria-label="Email to invite"><span class="desk-b" data-value="player" role="button" tabindex="0">player</span><button type="button" class="btn desk-send" id="send" data-url="/campaigns/c/invites" data-busy="Sending" onclick="${call('invite')}">Send invite</button></div>
 <p class="desk-st" role="status" data-desk-status></p></div>`;

// One browser for the file; each test gets its own context.
let shared = null;
after(async () => { if (shared) await (await shared).close(); });

async function open({ motion = '', foldFails = false, slowFold = 0, actFails = false } = {}) {
  shared = shared || pw.chromium.launch({ executablePath: chromePath });
  const browser = { close: async () => { await ctx.close(); } };
  const ctx = await (await shared).newContext();
  const page = await ctx.newPage();
  const seen = { fold: 0, sentence: 0, posts: [] };
  await page.route('http://t.test/**', async (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/static/js/desk_fold.js') return route.fulfill({ contentType: 'text/javascript', body: SCRIPT });
    if (u.pathname.endsWith('/fold')) {
      seen.fold++;
      if (slowFold) await new Promise((r) => setTimeout(r, slowFold));
      return foldFails ? route.fulfill({ status: 500, body: 'no' }) : route.fulfill({ contentType: 'text/html', body: seen.posts.length ? FOLD.replace(/<div class="desk-fr" id="r1">.*?<\/div>\n/s, '') : FOLD });
    }
    if (u.pathname.endsWith('/sentence')) { seen.sentence++; return route.fulfill({ contentType: 'text/html', body: '<p class="desk-s">Trash holds 1 item for 30 days.</p>' }); }
    if (route.request().method() === 'POST') {
      seen.posts.push({ path: u.pathname, body: route.request().postData() });
      return actFails ? route.fulfill({ status: 400, contentType: 'application/json', body: '{"message":"no"}' }) : route.fulfill({ contentType: 'application/json', body: '{"status":"ok"}' });
    }
    return route.fulfill({ contentType: 'text/html', body: PAGE(motion) });
  });
  await page.goto('http://t.test/');
  await page.addScriptTag({ url: 'http://t.test/static/js/desk_fold.js' });
  await page.evaluate(() => {
    window.Chronicle.apiFetch = (url, o = {}) => fetch(url, { method: o.method || 'GET', headers: { 'Content-Type': 'application/json' }, body: o.body ? JSON.stringify(o.body) : undefined });
    window.Chronicle.apiAction = (url, o) => window.Chronicle.apiFetch(url, o).then((r) => r.text().then((t) => { const j = t && t[0] === '{' ? JSON.parse(t) : {}; if (!r.ok) throw new Error(j.message || 'failed'); return j; }));
    window.Chronicle.notify = (m) => { window.notified = m; };
  });
  return { browser, page, seen };
}

const hidden = (page) => page.$eval('#desk-fold-trash', (e) => e.hidden);
const clean = (page) => page.$eval('#desk-fold-trash', (e) => ({ h: e.style.height, o: e.style.opacity, ov: e.style.overflow, anims: e.getAnimations().length }));

test('the chevron carries aria-expanded and aria-controls; Enter opens, Escape closes and gives focus back', { skip }, async () => {
  const { browser, page } = await open({ motion: 'off' });
  try {
    assert.equal(await page.getAttribute('#chev', 'aria-controls'), 'desk-fold-trash');
    assert.equal(await page.getAttribute('#chev', 'aria-expanded'), 'false');
    await page.focus('#chev');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => !document.getElementById('desk-fold-trash').hidden);
    assert.equal(await page.getAttribute('#chev', 'aria-expanded'), 'true');
    assert.equal(await page.getAttribute('#chev', 'aria-label'), 'Fold Trash');
    // A keyboard open puts focus on the first thing to do inside.
    assert.equal(await page.evaluate(() => document.activeElement.id), 'back1');
    await page.keyboard.press('Escape');
    assert.equal(await hidden(page), true);
    assert.equal(await page.evaluate(() => document.activeElement.id), 'chev');
    assert.equal(await page.getAttribute('#chev', 'aria-expanded'), 'false');
    // Space works the same as Enter.
    await page.keyboard.press('Space');
    await page.waitForFunction(() => !document.getElementById('desk-fold-trash').hidden);
  } finally { await browser.close(); }
});

test('pressing the row opens it, pressing a link or the quiet word does not, and the fold is fetched once', { skip }, async () => {
  const { browser, page, seen } = await open({ motion: 'off' });
  try {
    await page.click('.desk-s');
    await page.waitForFunction(() => !document.getElementById('desk-fold-trash').hidden);
    await page.click('.desk-s');
    assert.equal(await hidden(page), true);
    await page.click('.desk-s');
    await page.waitForFunction(() => !document.getElementById('desk-fold-trash').hidden);
    assert.equal(seen.fold, 1, 'the fragment is fetched on first open only');
    await page.click('.desk-s');
    await page.evaluate(() => document.getElementById('full').addEventListener('click', (e) => e.preventDefault()));
    await page.click('#full');
    assert.equal(await hidden(page), true, 'a link inside the row keeps its own meaning');
  } finally { await browser.close(); }
});

for (const motion of ['', 'calm', 'off']) {
  test(`motion ${motion || 'full'}: open, then close mid-way, ends closed with nothing left on the element`, { skip }, async () => {
    const { browser, page } = await open({ motion });
    try {
      await page.click('#chev');
      await page.waitForFunction(() => document.getElementById('desk-fold-trash').children.length > 0);
      await page.click('#chev'); // an immediate second press, mid open
      await page.waitForFunction(() => document.getElementById('desk-fold-trash').hidden, null, { timeout: 3000 });
      assert.deepEqual(await clean(page), { h: '', o: '', ov: '', anims: 0 });
      assert.equal(await page.getAttribute('#chev', 'aria-expanded'), 'false');
      // And the reverse: open, close, open quickly ends open.
      await page.click('#chev'); await page.click('#chev'); await page.click('#chev');
      await page.waitForFunction(() => !document.getElementById('desk-fold-trash').hidden);
      await page.waitForTimeout(500);
      assert.deepEqual(await clean(page), { h: '', o: '', ov: '', anims: 0 });
      assert.equal(await page.getAttribute('#chev', 'aria-expanded'), 'true');
    } finally { await browser.close(); }
  });
}

test('a fold that cannot be read says so and links the full page', { skip }, async () => {
  const { browser, page } = await open({ motion: 'off', foldFails: true });
  try {
    await page.click('#chev');
    await page.waitForFunction(() => !document.getElementById('desk-fold-trash').hidden);
    const t = await page.$eval('#desk-fold-trash', (e) => e.innerText);
    assert.match(t, /could not be opened right now/);
    assert.equal(await page.$eval('#desk-fold-trash a', (a) => a.getAttribute('href')), '/trash');
  } finally { await browser.close(); }
});

test('Bring back calls its route, the row leaves and the sentence is worded again', { skip }, async () => {
  const { browser, page, seen } = await open({ motion: 'off' });
  try {
    await page.click('#chev');
    await page.waitForSelector('#back1');
    await page.click('#back1');
    await page.waitForFunction(() => document.querySelector('.desk-s').textContent.includes('1 item'));
    assert.deepEqual(seen.posts.map((p) => p.path), ['/campaigns/c/trash/p1/restore']);
    assert.equal(await page.$('#r1'), null);
    assert.ok(seen.fold >= 2, 'the list is read again');
  } finally { await browser.close(); }
});

test('a refused action keeps the row and says so', { skip }, async () => {
  const { browser, page } = await open({ motion: 'off', actFails: true });
  try {
    await page.click('#chev');
    await page.click('#back1');
    await page.waitForFunction(() => window.notified);
    assert.equal(await page.$eval('#back1', (b) => b.textContent), 'Bring back');
    assert.equal(await page.$eval('.desk-s', (p) => p.textContent), 'Trash holds 2 items for 30 days.');
  } finally { await browser.close(); }
});

test('an invite needs an email, posts the role chosen, and tells the owner what it did', { skip }, async () => {
  const { browser, page, seen } = await open({ motion: 'off' });
  try {
    await page.click('#chev');
    await page.waitForSelector('#send');
    await page.click('#send');
    assert.match(await page.textContent('[data-desk-status]'), /Type their email/);
    assert.equal(seen.posts.length, 0);
    await page.fill('[data-desk-invite-email]', 'a@b.test');
    await page.keyboard.press('Enter'); // Enter in the field is not wired in this stub; the button is
    await page.click('#send');
    await page.waitForFunction(() => document.querySelector('[data-desk-status]').textContent.includes('Invite sent'));
    assert.deepEqual(JSON.parse(seen.posts[0].body), { email: 'a@b.test', role: 'player' });
    assert.equal(await page.inputValue('[data-desk-invite-email]'), '');
  } finally { await browser.close(); }
});
