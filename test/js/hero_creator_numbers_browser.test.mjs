// hero_creator_numbers_browser.test.mjs — the creator's characteristics step,
// when a game system declares one: the picked class fixes some characteristics
// and offers arrays for the rest; the player chooses an array, can swap any
// two numbers, or deals one at random, and the hero is sent with them.
//
// The plan is a synthetic one, not any real package's. Runs hero_creator.js in
// a real Chromium. Needs playwright-core and a Chromium (see _browser_env.mjs);
// skips locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const C = '/campaigns/c1';
const js = (n) => readFileSync(join(root, 'internal/plugins/entities/static/js', n));
const css = () => readFileSync(join(root, 'internal/plugins/entities/static/css/hero_creator.css'));

const ch = (key, name, properties = {}) => ({ key, slug: key, name, summary: name + ' summary.', source: 'package', properties });
const FIELDS = [['might', 'Might'], ['agility', 'Agility'], ['reason', 'Reason'], ['intuition', 'Intuition'], ['presence', 'Presence']].map(([key, label]) => ({ key, label }));
const PLAN = () => ({ systemName: 'Test System',
  numbers: { characteristics: { label: 'Characteristics', step: 'class', fields: FIELDS, fixedProperty: 'primary', fixedValue: 2, arraysProperty: 'arrays' } },
  steps: [
    { fieldKey: 'class', label: 'Class', needed: true, choices: [
      ch('shadow', 'Shadow', { primary: ['Agility'], starting_stamina: 18, recoveries: 8, arrays: ['2, 2, −1, −1', '2, 1, 1, −1', '2, 1, 0, 0', '1, 1, 1, 0'] }),
      ch('censor', 'Censor', { primary: ['Might', 'Presence'], arrays: ['2, −1, −1', '1, 1, −1', '1, 0, 0'] }),
      ch('plain', 'Plain'),
    ] },
    { fieldKey: 'kit', label: 'Kit', choices: [ch('wp', 'Warrior Priest', { stamina_bonus: 9 })] },
  ] });

const html = `<!doctype html><html data-gm-term="Director"><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}svg{display:block}</style><link rel="stylesheet" href="/hero_creator.css"></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true" data-practice="PRACTICE"><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => { window.hcImpl = d; document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)); }, escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director' };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, { width = 1100, practice = false } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce' });
  const seen = { post: [], errors: [] };
  pg.on('pageerror', (e) => seen.errors.push(e.message));
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_creator.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: css() });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_rules_lines.js') });
    if (p === C + '/characters/new/plan') return ok(PLAN());
    if (p === C + '/characters/new' && req.method() === 'POST') { seen.post.push(JSON.parse(req.postData())); return ok({ id: 'e9', url: C + '/entities/e9' }, 201); }
    return route.fulfill({ contentType: 'text/html', body: html.replace('PRACTICE', String(practice)) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return { pg, seen };
}
const text = (pg, sel) => pg.locator(sel).first().innerText();
const rail = (pg, key) => pg.locator('.hc-rail .hc-row[data-k="' + key + '"]');
const numsRow = (pg) => pg.locator('.hc-rail .hc-row', { hasText: 'Characteristics' });
const pick = async (pg, name) => { await pg.locator('.hc-gcard', { hasText: name }).click(); await pg.click('.hc-right [data-act="choose"]'); };
const tiles = (pg) => pg.evaluate(() => [].map.call(document.querySelectorAll('.hc-char'), (n) => n.querySelector('small').firstChild.textContent.trim() + ' ' + n.querySelector('b').textContent));

test('the step follows the picks, and asks for the class first', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  const labels = await pg.locator('.hc-rail .hc-row .hc-lb b').allInnerTexts();
  assert.deepEqual(labels, ['Class', 'Kit', 'Characteristics', 'Name and review']);
  assert.match(await numsRow(pg).innerText(), /Not placed yet/);
  await numsRow(pg).click();
  assert.match(await text(pg, '[data-nums-step]'), /Your class sets these\. Choose it first\./);
  await pg.click('[data-nums-step] [data-go="0"]');
  assert.equal(await pg.getAttribute('.hc-app', 'data-step'), '0', 'the button goes to the class step');
  assert.deepEqual(seen.errors, []);
});

test('a Shadow: Agility fixed, an array placed in order, two numbers swapped, and the hero sent with them', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pick(pg, 'Shadow');
  await numsRow(pg).click();
  const step = await text(pg, '[data-nums-step]');
  assert.match(step, /A Shadow starts with Agility 2\. Choose one array for the other 4\./);
  assert.deepEqual(await pg.locator('.hc-arr').allInnerTexts(), ['2, 2, −1, −1', '2, 1, 1, −1', '2, 1, 0, 0', '1, 1, 1, 0']);
  assert.deepEqual(await tiles(pg), ['Might ?', 'Agility 2', 'Reason ?', 'Intuition ?', 'Presence ?']);
  assert.match(await pg.locator('.hc-char.is-fixed').textContent(), /Agility , set by Shadow/, 'a screen reader hears why it is fixed');
  assert.equal(await pg.locator('.hc-char.is-fixed svg.hc-lock').count(), 1, 'and the eye sees a lock');
  const lockGap = await pg.evaluate(() => { const l = document.querySelector('.hc-char.is-fixed .hc-lock').getBoundingClientRect(), t = document.querySelector('.hc-char.is-fixed small').getBoundingClientRect(); return l.top - t.top; });
  assert.ok(lockGap < 8, 'the lock sits beside the name, even where every svg is a block: ' + lockGap);
  // A radio group is one Tab stop; the arrow keys choose within it.
  assert.deepEqual(await pg.locator('.hc-arr').evaluateAll((b) => b.map((x) => x.tabIndex)), [0, -1, -1, -1]);
  await pg.focus('.hc-arr[data-arr="0"]');
  await pg.keyboard.press('ArrowRight');
  assert.equal(await pg.getAttribute('.hc-arr[data-arr="1"]', 'aria-checked'), 'true');
  assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('data-arr')), '1');
  await pg.keyboard.press('ArrowLeft');
  await pg.keyboard.press('ArrowLeft');
  assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('data-arr')), '3', 'the arrows wrap');
  assert.deepEqual(await pg.locator('.hc-arr').evaluateAll((b) => b.map((x) => x.tabIndex)), [-1, -1, -1, 0]);

  await pg.click('.hc-arr[data-arr="2"]');
  assert.equal(await pg.getAttribute('.hc-arr[data-arr="2"]', 'aria-checked'), 'true');
  assert.deepEqual(await tiles(pg), ['Might 2', 'Agility 2', 'Reason 1', 'Intuition 0', 'Presence 0']);
  assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('data-arr')), '2', 'focus stays on the chosen array');
  assert.match(await numsRow(pg).innerText(), /Might 2 · Agility 2 · Reason 1 · Intuition 0 · Presence 0/);

  // Tap Might's 2, then Presence: they trade.
  await pg.click('[data-char="might"]');
  assert.equal(await pg.getAttribute('[data-char="might"]', 'aria-pressed'), 'true');
  assert.match(await text(pg, '[data-nums-say]'), /Now tap where the 2 from Might goes\./);
  await pg.click('[data-char="presence"]');
  assert.deepEqual(await tiles(pg), ['Might 0', 'Agility 2', 'Reason 1', 'Intuition 0', 'Presence 2']);

  await pg.locator('.hc-rail .hc-row').last().click();
  assert.match(await text(pg, '.hc-sum'), /Characteristics\s*Might 0 · Agility 2 · Reason 1 · Intuition 0 · Presence 2/i);
  await pg.fill('[data-name]', 'Brannoc');
  await pg.click('[data-act="create"]');
  await pg.waitForTimeout(100);
  assert.deepEqual(seen.post[0].characteristics, { agility: 2, might: 0, reason: 1, intuition: 0, presence: 2 });
  assert.deepEqual(seen.errors, []);
});

test('nothing placed: the review says the sheet starts at 0 and nothing is sent', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pick(pg, 'Shadow');
  await pg.locator('.hc-rail .hc-row').last().click();
  assert.match(await text(pg, '.hc-sum'), /Characteristics\s*Not placed yet\s*The sheet starts them at 0/i);
  await pg.fill('[data-name]', 'Brannoc');
  await pg.click('[data-act="create"]');
  await pg.waitForTimeout(100);
  assert.equal(seen.post[0].characteristics, undefined);
});

test('two fixed characteristics, a random deal, a class change and a class with no arrays', { skip }, async (t) => {
  const { pg } = await open(t);
  await pick(pg, 'Censor');
  await numsRow(pg).click();
  assert.match(await text(pg, '[data-nums-step]'), /A Censor starts with Might 2 and Presence 2\. Choose one array for the other 3\./);
  await pg.click('[data-act="numsdeal"]');
  const dealt = await pg.evaluate(() => document.querySelector('[data-widget="hero-creator"]')._heroCreator.numbersPlaced());
  assert.equal(dealt.might, 2); assert.equal(dealt.presence, 2);
  const rest = [dealt.agility, dealt.reason, dealt.intuition].sort((a, b) => a - b).join(',');
  assert.ok(['-1,-1,2', '-1,1,1', '0,0,1'].includes(rest), 'dealt one of the arrays: ' + rest);

  // A new class drops the old placing.
  await rail(pg, 'class').click();
  await pick(pg, 'Shadow');
  assert.match(await numsRow(pg).innerText(), /Not placed yet/);
  await rail(pg, 'class').click();
  await pick(pg, 'Plain');
  await numsRow(pg).click();
  assert.match(await text(pg, '[data-nums-step]'), /Plain lists no arrays, so the sheet starts these at 0/);
});

test('a placing is kept on the device and comes back', { skip }, async (t) => {
  const { pg } = await open(t);
  await pick(pg, 'Shadow');
  await numsRow(pg).click();
  await pg.click('.hc-arr[data-arr="3"]');
  await pg.reload();
  await pg.waitForSelector('.hc-app');
  assert.match(await numsRow(pg).innerText(), /Might 1 · Agility 2 · Reason 1 · Intuition 1 · Presence 0/);
});

test('a practice hero is not asked', { skip }, async (t) => {
  const { pg } = await open(t, { practice: true });
  const labels = await pg.locator('.hc-rail .hc-row .hc-lb b').allInnerTexts();
  assert.ok(!labels.includes('Characteristics'), labels.join(', '));
});

test('on a phone the step fits and counts in the steps', { skip }, async (t) => {
  const { pg, seen } = await open(t, { width: 390 });
  await pg.evaluate(() => { const c = document.querySelector('[data-widget="hero-creator"]')._heroCreator; c.picks.class = { name: 'Shadow', options: [] }; c.goStep(c.numIdx(), 'fwd'); });
  assert.match(await text(pg, '.hc-shead'), /Step 3 of 4 ·\s*Characteristics/);
  await pg.click('.hc-arr[data-arr="0"]');
  const over = await pg.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(over <= 0, 'no sideways scroll: ' + over);
  assert.deepEqual(seen.errors, []);
});
