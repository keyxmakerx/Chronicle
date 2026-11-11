// _live.mjs — shared helpers for the live checks in test/live.
//
// A live check drives a real, running Chronicle (the binary, MariaDB and
// Redis) the way a person would, so it can stand in for an operator's
// click-path check. Point it at a scratch instance only: every check makes
// its own accounts, campaigns and pages. tools/live-checks.sh starts one.
//
// Each check signs up fresh accounts with unique addresses, so files run side
// by side against one server without seeing each other's data. The one shared
// account is the site admin, made by _setup.mjs before any check runs,
// because only the first account on a fresh instance becomes a site admin.
//
// Pictures: `picture()` saves a screenshot and compares it with the one the
// last run on main saved (LIVE_BASELINE). A difference never fails the check
// itself; tools/live-pictures.mjs collects them into a report for approval.

import { createRequire } from 'node:module';
import { mkdirSync, writeFileSync, readFileSync, existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

export const BASE = (process.env.LIVE_BASE || 'http://localhost:8080').replace(/\/$/, '');
export const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
export const OUT = process.env.LIVE_OUT || join(repoRoot, 'test-results', 'live');
const BASELINE = process.env.LIVE_BASELINE || '';

export const ADMIN = { email: 'admin@live.test', password: 'live-admin-pass-123', name: 'Site Admin' };
export const PASSWORD = 'live-pass-12345';

// One look for every check, so pictures compare like with like.
const CONTEXT = {
  viewport: { width: 1280, height: 800 },
  deviceScaleFactor: 1,
  locale: 'en-US',
  timezoneId: 'UTC',
  colorScheme: 'light',
  reducedMotion: 'reduce',
};

let browserPromise = null;

/** The one browser a check file shares; closed by `done()`. */
export function browser() {
  browserPromise = browserPromise || chromium.launch();
  return browserPromise;
}

/** Close the browser; call from the file's `after()`. */
export async function done() {
  if (browserPromise) await (await browserPromise).close();
  browserPromise = null;
}

let seq = 0;
/** A unique address for a new account, so files never share people. */
export function uniqueEmail(tag) {
  seq += 1;
  return `${tag}-${process.pid}-${Date.now().toString(36)}-${seq}@live.test`;
}

/**
 * A signed-in person with their own browser context. `errors` collects
 * uncaught page errors and server errors on the page's own requests;
 * `assertClean()` fails the check if any arrived.
 */
export async function person(opts = {}) {
  // Each person arrives from their own address, as real visitors do, so the
  // sign-in and sign-up rate limits (per address) never trip across checks.
  // The server trusts loopback as a proxy, so it takes the forwarded address;
  // 2001:db8::/32 is the range set aside for documentation and tests.
  seq += 1;
  const ip = opts.ip || `2001:db8::${process.pid.toString(16)}:${seq.toString(16)}`;
  const ctx = await (await browser()).newContext({
    ...CONTEXT, ...(opts.context || {}),
    extraHTTPHeaders: { 'X-Forwarded-For': ip, ...((opts.context || {}).extraHTTPHeaders || {}) },
  });
  // Other hosts (icon and font CDNs) are cut off, so a run looks the same
  // offline, locally and in CI, and pictures never depend on a CDN's mood.
  await ctx.route((url) => !url.href.startsWith(BASE) && !url.protocol.startsWith('data'), (r) => r.abort());
  const page = await ctx.newPage();
  page.setDefaultTimeout(15000);
  const errors = [];
  page.on('pageerror', (e) => errors.push(`page error: ${e.message}`));
  page.on('response', (r) => {
    if (r.url().startsWith(BASE) && r.status() >= 500) errors.push(`${r.status()} ${r.request().method()} ${r.url().replace(BASE, '')}`);
  });
  const who = {
    ctx, page, errors, ip,
    email: opts.email || uniqueEmail(opts.tag || 'user'),
    password: opts.password || PASSWORD,
    name: opts.name || 'Live ' + (opts.tag || 'User'),
    assertClean() { assert.deepEqual(errors, [], 'the page reported errors'); },
    api: (url, o) => api(page, url, o),
    go: (path) => go(page, path),
  };
  if (opts.signIn !== false) await signIn(who, { register: opts.register !== false });
  return who;
}

/** The shared site admin, made by _setup.mjs. */
export function siteAdmin(opts = {}) {
  return person({ ...opts, ...ADMIN, tag: 'admin', register: false });
}

async function submitForm(page, field) {
  await Promise.all([page.waitForLoadState('networkidle'), field.press('Enter')]);
}

/** Sign in, registering the account first when `register` is set and it is new. */
export async function signIn(who, { register = true } = {}) {
  const { page } = who;
  await page.goto(BASE + '/login');
  await page.fill('#email', who.email);
  await page.fill('#password', who.password);
  await submitForm(page, page.locator('#password'));
  if (!page.url().includes('/login')) return;
  if (!register) throw new Error('could not sign in as ' + who.email);
  await page.goto(BASE + '/register');
  await page.fill('#email', who.email);
  await page.fill('#display_name', who.name);
  await page.fill('#password', who.password);
  await page.fill('#confirm', who.password);
  await submitForm(page, page.locator('#confirm'));
  if (/\/(login|register)/.test(page.url())) throw new Error('could not register ' + who.email + ' (landed on ' + page.url() + ')');
}

/** Navigate to a path on the instance and wait for it to settle. */
export async function go(page, path) {
  const res = await page.goto(BASE + path, { waitUntil: 'networkidle' });
  return res;
}

/**
 * A request made from inside the page, through Chronicle.apiFetch, so it
 * carries the session and CSRF token exactly as the app's own scripts do.
 * Returns { status, ok, body } with body parsed as JSON when it is JSON.
 */
export async function api(page, url, opts = {}) {
  return page.evaluate(async ({ url, opts }) => {
    let body = opts.body;
    if (opts.form) { body = new FormData(); for (const [k, v] of Object.entries(opts.form)) body.append(k, v); }
    const res = await Chronicle.apiFetch(url, { method: opts.method || 'GET', body, headers: opts.headers });
    const text = await res.text();
    let parsed = text;
    try { parsed = JSON.parse(text); } catch { /* not JSON */ }
    return { status: res.status, ok: res.ok, body: parsed };
  }, { url, opts });
}

/** Make a campaign as `who` and return its id. */
export async function newCampaign(who, name) {
  const { page } = who;
  await go(page, '/campaigns/new');
  const field = page.locator('form input[name=name]').first();
  await field.fill(name || 'Live ' + new Date().toISOString().slice(0, 16));
  await Promise.all([page.waitForURL(/\/campaigns\/[0-9a-f-]{36}/).catch(() => {}), field.press('Enter')]);
  const id = (page.url().match(/\/campaigns\/([0-9a-f-]{36})/) || [])[1];
  if (!id) throw new Error('campaign was not created; landed on ' + page.url());
  return id;
}

/** Add `member` to the campaign with a role: player, scribe or owner. */
export async function addMember(owner, campaignId, member, role = 'player') {
  const res = await api(owner.page, `/campaigns/${campaignId}/members`, {
    method: 'POST', form: { email: member.email, role },
    headers: { 'HX-Request': 'true' },
  });
  assert.ok(res.ok, `adding ${member.email} answered ${res.status}`);
  assert.ok(typeof res.body === 'string' && res.body.includes(member.name), `${member.name} is not in the member list after adding`);
}

/** Switch on campaign add-ons by slug ('*' for every non-system add-on). */
export async function enableAddons(owner, campaignId, slugs) {
  const list = (await api(owner.page, `/campaigns/${campaignId}/addons`)).body || [];
  const want = (a) => slugs === '*' ? a.addon_category !== 'system' : slugs.includes(a.addon_slug || a.slug);
  for (const a of list) {
    if (a.enabled || !want(a)) continue;
    const res = await api(owner.page, `/campaigns/${campaignId}/addons/${a.addon_id}/toggle`, { method: 'PUT', form: { action: 'enable' } });
    assert.ok(res.ok, `turning on ${a.addon_slug || a.addon_id} answered ${res.status}`);
  }
}

/**
 * Make a page in a category, picked by a word of its name ("Character" finds
 * Characters), and return the page's id.
 */
export async function newPage(who, campaignId, category, name) {
  const { page } = who;
  await go(page, `/campaigns/${campaignId}/entities/new`);
  const select = page.locator('select#entity_type_id');
  const value = await select.evaluate((el, word) => {
    const hit = [...el.options].find((o) => o.value && o.textContent.toLowerCase().includes(word.toLowerCase()));
    return hit ? hit.value : '';
  }, category);
  assert.ok(value, `no category matching "${category}"`);
  await select.selectOption(value);
  const field = page.locator('#entity-form input[name=name], form input#name').first();
  await field.fill(name);
  await Promise.all([page.waitForURL(/\/entities\/[0-9a-f-]{36}/).catch(() => {}), field.press('Enter')]);
  await page.waitForLoadState('networkidle');
  const id = (page.url().match(/\/entities\/([0-9a-f-]{36})/) || [])[1];
  if (!id) throw new Error('page was not created; landed on ' + page.url());
  return id;
}

// The admin check-up sheet peeks open for a few seconds on its own, and the
// status strip counts every session on the server, so both are always hidden
// or masked.
const STILL = `*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}
[data-checkup-sheet]{visibility:hidden!important}`;
const ALWAYS_MASK = ['.admin-strip-figures'];

/**
 * Save a picture of the page (or of `opts.locator`) for approval. `mask` is
 * a list of selectors painted over, for text that changes every run (times,
 * generated names); `hide` makes elements invisible but keeps their space. Returns 'same', 'changed' or 'new'; never throws on a
 * difference, because a changed look is for a person to approve.
 */
export async function picture(page, name, opts = {}) {
  assert.match(name, /^[a-z0-9-]+$/, 'picture names are lower-case words joined by hyphens');
  // The still-making styles come off again afterwards: a check that carries
  // on would otherwise wait for transitionend events that never fire.
  const css = STILL + (opts.hide ? opts.hide.join(',') + '{visibility:hidden!important}' : '');
  const tag = await page.addStyleTag({ content: css });
  let png;
  try {
    await page.evaluate(() => document.fonts && document.fonts.ready);
    await page.waitForTimeout(150);
    const mask = [...ALWAYS_MASK, ...(opts.mask || [])].map((s) => page.locator(s));
    const target = opts.locator || page;
    png = await target.screenshot({ mask, maskColor: '#ff00ff', fullPage: !opts.locator && !!opts.fullPage, animations: 'disabled' });
  } finally {
    await tag.evaluate((el) => el.remove()).catch(() => {});
  }
  const dir = join(OUT, 'pictures');
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, name + '.png'), png);
  const record = { name, check: opts.check || '', about: opts.about || '', status: 'new', diff: 0 };
  const before = BASELINE && join(BASELINE, name + '.png');
  if (before && existsSync(before)) {
    const result = await comparePng(page, readFileSync(before), png);
    record.diff = result.ratio;
    record.status = result.ratio > (opts.tolerance ?? 0.001) ? 'changed' : 'same';
    if (result.diffPng) writeFileSync(join(dir, name + '.diff.png'), result.diffPng);
  }
  writeFileSync(join(dir, name + '.json'), JSON.stringify(record));
  return record.status;
}

// Compares two PNGs in a blank page's canvas, so no image library is needed.
// A pixel counts as different when any channel moves by more than 24 of 255,
// which ignores antialiasing noise; the diff paints those pixels red.
async function comparePng(page, a, b) {
  const blank = await page.context().newPage();
  try {
    const out = await blank.evaluate(async ({ a, b }) => {
      const load = (src) => new Promise((ok, no) => { const i = new Image(); i.onload = () => ok(i); i.onerror = no; i.src = 'data:image/png;base64,' + src; });
      const [ia, ib] = await Promise.all([load(a), load(b)]);
      const w = Math.max(ia.width, ib.width); const h = Math.max(ia.height, ib.height);
      const px = (img) => { const c = new OffscreenCanvas(w, h); const x = c.getContext('2d'); x.drawImage(img, 0, 0); return x.getImageData(0, 0, w, h).data; };
      const da = px(ia); const db = px(ib);
      const c = new OffscreenCanvas(w, h); const x = c.getContext('2d'); const out = x.createImageData(w, h);
      let bad = 0;
      for (let i = 0; i < da.length; i += 4) {
        const d = Math.max(Math.abs(da[i] - db[i]), Math.abs(da[i + 1] - db[i + 1]), Math.abs(da[i + 2] - db[i + 2]), Math.abs(da[i + 3] - db[i + 3]));
        if (d > 24) { bad++; out.data.set([230, 0, 40, 255], i); } else { const g = (db[i] + db[i + 1] + db[i + 2]) / 3; out.data.set([g, g, g, 60], i); }
      }
      x.putImageData(out, 0, 0);
      const blob = await c.convertToBlob({ type: 'image/png' });
      const buf = new Uint8Array(await blob.arrayBuffer());
      let s = ''; for (let i = 0; i < buf.length; i += 0x8000) s += String.fromCharCode.apply(null, buf.subarray(i, i + 0x8000));
      return { ratio: bad / (w * h) + (ia.width !== ib.width || ia.height !== ib.height ? 1 : 0), diff: btoa(s) };
    }, { a: a.toString('base64'), b: b.toString('base64') });
    return { ratio: out.ratio, diffPng: Buffer.from(out.diff, 'base64') };
  } finally {
    await blank.close();
  }
}
