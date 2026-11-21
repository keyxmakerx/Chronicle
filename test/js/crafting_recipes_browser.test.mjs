// crafting_recipes_browser.test.mjs — how a new recipe begins in the
// Crafting recipe list: internal/plugins/crafting/static/js/crafting_recipes.js.
//
// The list starts with one typed line. A name and Enter add the recipe, an
// empty line adds nothing, there is no "New recipe" button that saves an
// unnamed one, and keys typed in the line never act as shortcuts.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const library = { ...JSON.parse(readFileSync(join(root, 'test', 'js', 'fixtures', 'craft_book_pages.json'), 'utf8')).library, pictures: [] };
const TYPES = { '.css': 'text/css', '.js': 'application/javascript', '.woff2': 'font/woff2', '.svg': 'image/svg+xml' };
const SCRIPTS = ['/static/js/widgets/craft_pictures.js', '/internal/plugins/crafting/static/js/crafting_lib.js', '/internal/plugins/crafting/static/js/crafting_recipes.js'];

const PAGE = `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/app.css">
<link rel="stylesheet" href="/internal/plugins/crafting/static/css/crafting.css"><style>body{margin:0;padding:16px}</style></head>
<body><div id="mount"></div>${SCRIPTS.map((s) => `<script src="${s}"></script>`).join('')}</body></html>`;

function recipe(name, i) {
  return { id: 'r' + i, name, template: '', bench: library.benchKinds[0].key, materials: [], steps: [], result: { qty: 1 }, goal: 30, stat: '', prerequisite: '', source: '',
    cost: { amount: 0, currency: '' }, who: 'all', known: [], colors: {}, rules: [], looks: {}, extras: {}, version: 1 };
}

// refuse: names the fake server answers 409 to, as a taken name would be.
async function open(t, { width = 1200, refuse = [] } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: PAGE });
    const file = join(root, u.pathname);
    if (existsSync(file)) return route.fulfill({ contentType: TYPES[u.pathname.slice(u.pathname.lastIndexOf('.'))] || 'application/octet-stream', body: readFileSync(file) });
    return route.fulfill({ status: 404, body: '' });
  });
  page.setDefaultTimeout(30000);
  await page.goto('http://crafting.test/', { waitUntil: 'load' });
  await page.evaluate(({ library, refuse }) => {
    window.posted = []; window.statuses = [];
    const recipes = [];
    const app = {
      lib: library, recipes, cid: 'c', calendarId: '', items: {}, itemList: [], benches: [], skills: [], tables: [], characters: [],
      status: (s, m) => window.statuses.push([s, m || '']),
      toast() {}, confirm: () => Promise.resolve(true), reloadFix() {}, show() {},
      api(method, url, body) {
        if (method === 'GET') return Promise.resolve({ placed: true });
        if (method === 'POST' && url === '/recipes') {
          window.posted.push(body);
          if (refuse.includes(body.name)) return Promise.reject(Object.assign(new Error('there is already a recipe with that name'), { status: 409 }));
          return Promise.resolve({ id: 'r' + window.posted.length, name: body.name, template: '', bench: body.bench, materials: [], steps: [], result: { qty: 1 }, goal: 30, stat: '', prerequisite: '', source: '',
            cost: { amount: 0, currency: '' }, who: 'all', known: [], colors: {}, rules: [], looks: {}, extras: {}, version: 1 });
        }
        return Promise.resolve({});
      },
    };
    window.CraftingRecipes.mount(document.getElementById('mount'), app);
  }, { library, refuse });
  return { page, errors };
}
const names = (page) => page.$$eval('.cra-ritem b', (els) => els.map((e) => e.textContent));

test('the list has a typed line and no button that saves an unnamed recipe', { skip, timeout: 120000 }, async (t) => {
  const { page, errors } = await open(t);
  assert.equal(await page.$$eval('.cra-newrow, [data-act="new"]', (e) => e.length), 0);
  assert.equal(await page.getAttribute('.cra-addin', 'placeholder'), 'Name a recipe, then press Enter');
  assert.equal(await page.getAttribute('.cra-addin', 'aria-label'), 'Name a new recipe');
  assert.deepEqual(errors, []);
});

test('a name and Enter add the recipe, and the line stays ready for the next', { skip, timeout: 120000 }, async (t) => {
  const { page, errors } = await open(t);
  await page.focus('.cra-addin');
  await page.keyboard.type('Moonpetal salve');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.cra-ritem');
  assert.deepEqual(await names(page), ['Moonpetal salve']);
  assert.equal(await page.inputValue('.cra-addin'), '');
  assert.equal(await page.evaluate(() => document.activeElement.classList.contains('cra-addin')), true, 'focus stays on the line');
  assert.match(await page.textContent('.cra-rmore'), /Add materials/);
  await page.keyboard.type('  Silvered blade  ');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.querySelectorAll('.cra-ritem').length === 2);
  assert.deepEqual(await page.evaluate(() => window.posted.map((b) => b.name)), ['Moonpetal salve', 'Silvered blade']);
  assert.deepEqual(errors, []);
});

test('an empty or blank line adds nothing', { skip, timeout: 120000 }, async (t) => {
  const { page } = await open(t);
  await page.focus('.cra-addin');
  await page.keyboard.press('Enter');
  await page.keyboard.type('   ');
  await page.keyboard.press('Enter');
  assert.equal(await page.evaluate(() => window.posted.length), 0);
  assert.equal(await page.$$eval('.cra-ritem', (e) => e.length), 0);
});

test('a refused name stays typed so it can be fixed in place', { skip, timeout: 120000 }, async (t) => {
  const { page } = await open(t, { refuse: ['Taken'] });
  await page.focus('.cra-addin');
  await page.keyboard.type('Taken');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => window.statuses.some((s) => s[0] === 'error'));
  assert.equal(await page.inputValue('.cra-addin'), 'Taken');
  assert.equal(await page.$$eval('.cra-ritem', (e) => e.length), 0);
  assert.equal(await page.evaluate(() => document.activeElement.classList.contains('cra-addin')), true);
});

test('keys typed in the line are text, never shortcuts', { skip, timeout: 120000 }, async (t) => {
  const { page } = await open(t);
  await page.focus('.cra-addin');
  await page.keyboard.type('nd?/');
  assert.equal(await page.inputValue('.cra-addin'), 'nd?/');
  assert.equal(await page.evaluate(() => window.posted.length), 0);
});

test('the line is usable at phone width', { skip, timeout: 120000 }, async (t) => {
  const { page } = await open(t, { width: 360 });
  const box = await page.locator('.cra-addin').boundingBox();
  assert.ok(box.height >= 44, 'a touch-sized line: ' + box.height);
  assert.ok(box.x >= 0 && box.x + box.width <= 360, 'inside the screen');
  // The list itself must fit; the preview column's width is the page's, not this line's.
  assert.equal(await page.evaluate(() => document.querySelector('.cra-rlist').getBoundingClientRect().right <= innerWidth), true, 'the list fits the screen');
});
