// hero_creator_terms_browser.test.mjs — rules words in the hero creator.
//
// Runs hero_creator.js in a real Chromium against a stubbed plan and glossary.
// It pins that a {@category term|label} marker the glossary knows becomes a
// button that opens one hover card, that a word the glossary lacks (or a
// glossary that fails, or no address at all) stays plain text, that the glossary
// key is normalised the way the contract says, and that nothing in a card is
// taken as HTML.
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
const read = (rel) => readFileSync(join(root, 'internal/plugins/entities/static', rel));

const PLAN = () => ({ systemName: 'Draw Steel', steps: [
  { fieldKey: 'kit', label: 'Kit', choices: [{ key: 'a', slug: 'a', name: 'Arcane Archer', source: 'package',
    summary: 'Fires a {@combat power roll|power roll} and {@condition Bleeding}. Also {@condition nothing-known}.',
    properties: { signature: { name: 'Exploding Arrow', effect: 'Make a {@combat Power  Roll}. <img src=x onerror="window.__xss=1">' } } }] },
] });
const GLOSSARY = { terms: {
  'power-roll': { name: 'Power Roll', category: 'combat', text: 'Roll 2d10 and add a characteristic. <b>not bold</b>' },
  'Bleeding': { name: 'Bleeding', category: 'condition', text: 'You lose Stamina each round.' },
} };

const html = (attrs) => `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/hero_creator.css"></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true" ${attrs}><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director' };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, { terms = 'glossary', width = 1100 } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce' });
  const asked = [];
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: read('js/hero_creator.js') });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: read('js/hero_rules_lines.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: read('css/hero_creator.css') });
    if (p === C + '/characters/new/plan') return route.fulfill({ contentType: 'application/json', body: JSON.stringify(PLAN()) });
    if (p === C + '/terms') {
      asked.push(p);
      if (terms === 'broken') return route.fulfill({ status: 500, body: 'no' });
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify(GLOSSARY) });
    }
    return route.fulfill({ contentType: 'text/html', body: html(terms === 'none' ? '' : `data-terms-url="${C}/terms"`) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return { pg, asked };
}

test('a known word is a button, and the card on a card front stays plain words', { skip }, async (t) => {
  const { pg, asked } = await open(t);
  assert.equal(asked.length, 1, 'the glossary is read once');
  const names = await pg.locator('.hc-right .hc-term').allInnerTexts();
  assert.deepEqual(names, ['power roll', 'Bleeding', 'Power Roll']);
  assert.equal(await pg.locator('.hc-gcard .hc-term').count(), 0, 'a card is itself a button, so its words are not');
  assert.doesNotMatch(await text(pg, '.hc-gcard .hc-sum'), /\{@/, 'markers are resolved on the card front');
  assert.match(await text(pg, '.hc-gcard .hc-sum'), /power roll and Bleeding\. Also nothing known\./);
});

test('a word the glossary keys with different case and spacing still finds its entry', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.equal(await pg.locator('.hc-right .hc-term[data-term="bleeding"]').count(), 1);
  assert.equal(await pg.locator('.hc-right .hc-term[data-term="power-roll"]').count(), 2);
});

test('hovering a word opens one hover card with the glossary text, as text', { skip }, async (t) => {
  const { pg } = await open(t);
  await pg.locator('.hc-right .hc-term[data-term="power-roll"]').first().hover();
  await pg.waitForSelector('.hc-tip');
  assert.equal(await pg.locator('.hc-tip').count(), 1);
  const tip = await text(pg, '.hc-tip');
  assert.match(tip, /Power Roll/);
  assert.match(tip, /Combat/i);
  assert.match(tip, /Roll 2d10 and add a characteristic\. <b>not bold<\/b>/, 'the text is shown, not run');
  assert.equal(await pg.locator('.hc-tip b').count(), 0);
  await pg.locator('.hc-right .hc-term[data-term="bleeding"]').hover();
  await pg.waitForFunction(() => /Bleeding/.test(document.querySelector('.hc-tip') && document.querySelector('.hc-tip').innerText));
  assert.equal(await pg.locator('.hc-tip').count(), 1, 'still one card');
});

test('a tap pins the card and Escape puts it away', { skip }, async (t) => {
  const { pg } = await open(t);
  await pg.locator('.hc-right .hc-term[data-term="bleeding"]').click();
  await pg.waitForSelector('.hc-tip.is-pinned');
  await pg.keyboard.press('Escape');
  await pg.waitForFunction(() => !document.querySelector('.hc-tip'));
});

test('markup in the rules text is never run', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.equal(await pg.locator('.hc-right img').count(), 0);
  assert.equal(await pg.evaluate(() => window.__xss), undefined);
  assert.match(await text(pg, '.hc-right'), /<img src=x/);
});

test('with no glossary address, or a glossary that fails, every word is plain', { skip }, async (t) => {
  for (const terms of ['none', 'broken']) {
    const { pg, asked } = await open(t, { terms });
    assert.equal(asked.length, terms === 'none' ? 0 : 1);
    assert.equal(await pg.locator('.hc-term').count(), 0);
    assert.match(await text(pg, '.hc-right'), /power roll and Bleeding/);
    assert.equal(await pg.locator('.hc-right [data-act="choose"]').count(), 1, 'the creator still works');
  }
});

function text(pg, sel) { return pg.locator(sel).first().innerText(); }
