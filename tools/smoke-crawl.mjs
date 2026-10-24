// smoke-crawl.mjs — a broad "does anything blow up" pass over a running
// Chronicle. It signs in (registering the account if it is new), makes a
// campaign with a page of every category, then:
//   1. walks every same-origin link it can reach, and
//   2. hits every GET route registered in internal/plugins/*/routes*.go,
// and fails on any 5xx answer, uncaught page error, or 4xx on a sub-request a
// page made itself. It finds crashes, not wrong behaviour; the spec tests in
// test/e2e and test/js stay the place for that.
//
// Point it at a scratch instance only: it creates an account, a campaign and
// pages. Links that log out, delete, leave or export are never followed, and
// only GET requests are sent apart from the sign-in and seed forms.
//
// RUN: SMOKE_BASE=http://localhost:8080 NODE_PATH="$(npm root -g)" node tools/smoke-crawl.mjs
//   SMOKE_EMAIL / SMOKE_PASSWORD  account to use (default smoke@example.com)
//   SMOKE_MAX                     page cap for the link walk (default 400)
// Requests to other hosts (CDN fonts and icons) are ignored, so it also runs
// offline.

import { createRequire } from 'node:module';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const BASE = (process.env.SMOKE_BASE || 'http://localhost:8080').replace(/\/$/, '');
const EMAIL = process.env.SMOKE_EMAIL || 'smoke@example.com';
const PASSWORD = process.env.SMOKE_PASSWORD || 'smoke-pass-123';
const MAX = Number(process.env.SMOKE_MAX) || 400;
const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..');

const SKIP = /logout|\/delete|\/remove|\/leave|\/revoke|\/export|download|accept-transfer|\/sync\/|\/media\/|\.(zip|json|png|jpe?g|webp|css|js|svg)$/i;

const browser = await chromium.launch();
const page = await (await browser.newContext()).newPage();
page.setDefaultTimeout(20000);
const problems = [];
let at = '';
page.on('pageerror', (e) => problems.push(`${at}: page error: ${e.message}`));
page.on('response', (r) => {
  if (!r.url().startsWith(BASE)) return;
  // A page that loads fine but asks the server for something missing is a
  // bug; a 4xx on the page itself is usually just a route the walk guessed.
  const own = r.request().isNavigationRequest();
  if (r.status() >= 500 || (!own && r.status() >= 400)) {
    problems.push(`${at}: ${r.status()} ${r.request().method()} ${r.url().replace(BASE, '')}`);
  }
});

async function submit(locator) {
  await Promise.all([page.waitForLoadState('networkidle'), locator.press('Enter')]);
  await page.waitForTimeout(500);
}

async function signIn() {
  at = 'sign-in';
  await page.goto(BASE + '/login');
  await page.fill('#email', EMAIL);
  await page.fill('#password', PASSWORD);
  await submit(page.locator('#password'));
  if (!page.url().includes('/login')) return;
  await page.goto(BASE + '/register');
  await page.fill('#email', EMAIL);
  await page.fill('#display_name', 'Smoke Test');
  await page.fill('#password', PASSWORD);
  await page.fill('#confirm', PASSWORD);
  await submit(page.locator('#confirm'));
  if (/\/(login|register)/.test(page.url())) throw new Error('could not sign in or register ' + EMAIL);
}

async function seedCampaign() {
  at = 'seed';
  await page.goto(BASE + '/campaigns/new');
  const name = page.locator('form input[name=name]').first();
  await name.fill('Smoke ' + new Date().toISOString().slice(0, 16));
  await submit(name);
  const id = (page.url().match(/\/campaigns\/([0-9a-f-]{36})/) || [])[1];
  if (!id) throw new Error('campaign was not created; landed on ' + page.url());
  // A new campaign starts with most add-ons off, and their pages answer 404;
  // switch every one on so the walk reaches them. Game systems are chosen
  // per campaign, not toggled, so they are left alone. Uses the page's own
  // Chronicle.apiFetch for the CSRF token.
  const enabled = await page.evaluate(async (cid) => {
    const res = await Chronicle.apiFetch(`/campaigns/${cid}/addons`);
    const list = res.ok ? await res.json() : [];
    let n = 0;
    for (const a of list || []) {
      if (a.enabled || a.addon_category === 'system') continue;
      const body = new FormData(); body.append('action', 'enable');
      const r = await Chronicle.apiFetch(`/campaigns/${cid}/addons/${a.addon_id}/toggle`, { method: 'PUT', body });
      if (r.ok) n++;
    }
    return n;
  }, id);
  console.log(`smoke-crawl: campaign ${id}, ${enabled} add-on(s) switched on`);
  await page.goto(`${BASE}/campaigns/${id}/entities/new`);
  const types = await page.$$eval('a[href*="entities/new?type="]', (as) => [...new Set(as.map((a) => a.href))]);
  let entity = '';
  for (const href of types) {
    await page.goto(href);
    const n = page.locator('form input[name=name]').first();
    if (!(await n.count())) continue;
    await n.fill('Smoke page');
    await submit(n);
    entity = entity || (page.url().match(/\/entities\/([0-9a-f-]{36})/) || [])[1] || '';
  }
  return { id, entity };
}

async function visit(u) {
  at = u.replace(BASE, '');
  const res = await page.goto(u, { waitUntil: 'networkidle', timeout: 20000 })
    .catch((e) => { problems.push(`${at}: did not load: ${e.message.split('\n')[0]}`); return null; });
  await page.waitForTimeout(250);
  return res;
}

// Many links differ only by id; three of each shape is enough to catch a
// broken template without walking every page of a big campaign.
async function walk(start) {
  const seen = new Set();
  const shapes = {};
  const queue = [start];
  while (queue.length && seen.size < MAX) {
    const u = new URL(queue.shift()); u.hash = '';
    const key = u.toString();
    if (seen.has(key)) continue;
    seen.add(key);
    const shape = u.pathname.replace(/[0-9a-f-]{36}|\/\d+(?=\/|$)/g, '/X');
    shapes[shape] = (shapes[shape] || 0) + 1;
    if (shapes[shape] > 3) continue;
    if (!(await visit(key))) continue;
    const links = await page.$$eval('a[href]', (as) => as.map((a) => a.href)).catch(() => []);
    for (const l of links) if (l.startsWith(BASE) && !SKIP.test(l)) queue.push(l);
  }
  return seen.size;
}

function registeredGetRoutes() {
  const dir = join(repoRoot, 'internal', 'plugins');
  const out = new Set();
  for (const plugin of readdirSync(dir)) {
    let files = [];
    try { files = readdirSync(join(dir, plugin)).filter((f) => /^routes.*\.go$/.test(f)); } catch { continue; }
    for (const f of files) {
      for (const m of readFileSync(join(dir, plugin, f), 'utf8').matchAll(/\.GET\("([^"]*)"/g)) out.add(m[1]);
    }
  }
  return [...out];
}

// Routes are registered relative to their group, so each is tried at the
// prefixes groups use. Path parameters get the seeded ids or a placeholder;
// only a 5xx counts, since a placeholder is expected to 404.
async function probe(ids) {
  const fill = (p) => p
    .replace(/:id(?=\/|$)/g, ids.id)
    .replace(/:(eid|entityID|entityId)(?=\/|$)/g, ids.entity || 'x')
    .replace(/:[A-Za-z]+/g, 'x');
  const urls = new Set();
  for (const r of registeredGetRoutes()) {
    if (/^\/(login|register|logout|sync)|\/sync\//.test(r)) continue;
    const p = fill(r);
    for (const pre of ['', '/admin', `/campaigns/${ids.id}`, `/campaigns/${ids.id}/entities/${ids.entity}`]) urls.add(pre + p);
  }
  for (const u of urls) {
    for (const hx of [false, true]) {
      const res = await page.request.get(BASE + u, { headers: hx ? { 'HX-Request': 'true' } : {}, maxRedirects: 0, timeout: 15000 }).catch(() => null);
      if (!res) continue;
      if (res.status() >= 500) problems.push(`${u}: ${res.status()}${hx ? ' (htmx)' : ''}`);
      const html = !hx && res.status() === 200 && /text\/html/.test(res.headers()['content-type'] || '');
      if (html && /<html/i.test(await res.text())) await visit(BASE + u);
    }
  }
  return urls.size;
}

try {
  await signIn();
  const ids = await seedCampaign();
  // /admin answers only a site admin; on a fresh instance the first account is one.
  const walked = (await walk(`${BASE}/campaigns/${ids.id}`)) + (await walk(BASE + '/admin'));
  const probed = await probe(ids);
  const unique = [...new Set(problems)];
  console.log(`smoke-crawl: walked ${walked} pages, probed ${probed} routes, ${unique.length} problem(s)`);
  for (const p of unique) console.log('  ' + p);
  process.exitCode = unique.length ? 1 : 0;
} catch (e) {
  console.error('smoke-crawl: ' + e.message);
  process.exitCode = 2;
} finally {
  await browser.close();
}
