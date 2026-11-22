// hero_stage_browser.test.mjs — the hero creator's "stage" look.
//
// Runs hero_stage.js and hero_creator.js (with hero_rules_lines.js) in a real
// Chromium against a stubbed, Draw-Steel-shaped plan, with the mount carrying
// data-look="stage". It pins what the look draws and does: the hero (name,
// line, four figures) in the banner or the rail, no chip row on a phone, what
// looking at an entry would change (and that looking changes nothing), the
// tuck, Save and leave and the save status, the review's hero card, that the
// hero posted is the one the classic creator posts, no figures for a system
// that has none, the site chrome made inert and given back, white words that
// stay readable on the gradient of any accent, no sideways scroll at 360, and
// a mouse that can move the hand. The classic creator's own tests are the
// proof that the plain look is untouched.
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
const DRAFT = 'chronicle:hero-draft:c1';
const file = (rel) => readFileSync(join(root, 'internal/plugins/entities/static', rel));

const ch = (name, props = {}, extra = {}) => ({ key: name.toLowerCase(), slug: name.toLowerCase(), name, summary: name + ' summary.', source: 'package', properties: props, ...extra });
const classes = ['Fury', 'Censor', 'Conduit', 'Elementalist', 'Null', 'Shadow', 'Tactician', 'Talent', 'Troubadour', 'Beastheart', 'Summoner', 'Warden', 'Zealot', 'Mage']
  .map((n) => ch(n, { starting_stamina: n === 'Tactician' ? 24 : 21 }));
// Ancestries with the figures an ancestry gives; classes with Stamina; a kit
// with a Stamina bonus; the rest are plain lists.
const PLAN = () => ({
  systemName: 'Draw Steel',
  about: [{ key: 'pronouns', label: 'Pronouns', long: false }, { key: 'backstory', label: 'Backstory', long: true }],
  steps: [
    { fieldKey: 'ancestry', label: 'Ancestry', choices: [ch('Dwarf', { speed: 5, size: '1S', stability: 1 }), ch('Human', { speed: 5, size: '1M', stability: 0 }), ch('Orc', { speed: 5, size: '1M', stability: 1 })] },
    { fieldKey: 'culture', label: 'Culture', choices: [ch('Academic'), ch('Nomad')] },
    { fieldKey: 'career', label: 'Career', choices: [ch('Aristocrat'), ch('Soldier')] },
    { fieldKey: 'class', label: 'Class', choices: classes },
    { fieldKey: 'complication', label: 'Complication', choices: [ch('Amnesia'), ch('Rival')], rules: { limited: false } },
    { fieldKey: 'kit', label: 'Kit', choices: [ch('Mountain', { stamina_bonus: 6 }), ch('Panther', { speed_bonus: 1 })] },
  ],
});
// A system whose entries carry no figure at all.
const PLAIN_PLAN = () => ({
  systemName: 'Plain Game',
  steps: [
    { fieldKey: 'ancestry', label: 'Ancestry', choices: [ch('Elf'), ch('Gnome')] },
    { fieldKey: 'class', label: 'Class', choices: [ch('Bard'), ch('Cleric')] },
  ],
});
const DRAFT_ANCESTRY = { picks: { ancestry: { name: 'Dwarf', options: [] } }, name: '', about: {} };
const DRAFT_FULL = {
  picks: { ancestry: { name: 'Dwarf', options: [] }, culture: { name: 'Academic', options: [] }, career: { name: 'Soldier', options: [] }, class: { name: 'Fury', options: [] }, complication: { name: 'Amnesia', options: [] }, kit: { name: 'Mountain', options: [] } },
  name: 'Korrin Ashvein', about: { pronouns: 'she/her' },
};

const LIGHT = ':root{--color-bg-primary:#f9fafb;--color-card-bg:#fff;--color-bg-tertiary:#f3f4f6;--color-border:#e5e7eb;--color-text-primary:#111827;--color-text-secondary:#4b5563;--dur-micro:120ms;--dur-standard:200ms;--dur-large:280ms;--ease-out:cubic-bezier(.16,1,.3,1);--z-popup:50;--z-sheet:60}';
const DARK = 'html.dark{--color-bg-primary:#111827;--color-card-bg:#1f2937;--color-bg-tertiary:#374151;--color-border:#374151;--color-text-primary:#f9fafb;--color-text-secondary:#c4c9d2}';

function page({ accent = '#6366f1', attrs = '', look = true, chrome = false, drawer = false, dark = false, extras = false }) {
  return `<!doctype html><html data-gm-term="Director"${dark ? ' class="dark"' : ''}><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>${LIGHT}${DARK}:root{--color-accent:${accent}}*,::before,::after{box-sizing:border-box}body{margin:0;font-family:sans-serif}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}${drawer ? '#tt-drawer{position:fixed;left:0;right:0;bottom:0;height:48px;background:#222;z-index:60}' : ''}</style>
<link rel="stylesheet" href="/hero_creator.css">${look ? '<link rel="stylesheet" href="/hero_stage.css">' : ''}</head><body>
${chrome ? '<nav id="sidebar"' + (chrome === 'inert' ? ' inert' : '') + '>side</nav><header id="app-topbar">top</header>' : ''}${drawer ? '<div id="tt-drawer"><button type="button" id="tt-grab">The table</button></div>' : ''}${extras ? '<a id="skip" href="#main-content">Skip to content</a><div id="notice" role="region"><button type="button">Set zone</button><button type="button">Later</button><button type="button">Never</button></div><div id="pre" inert><button type="button">Held by the page</button></div>' : ''}
${extras ? '<div id="shell"><main id="main-content" tabindex="-1">' : ''}<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true"${look ? ' data-look="stage"' : ''} ${attrs}><p class="hc-loading__text">Opening</p></div>
${extras ? '</main><footer id="foot"><a href="/elsewhere">Footer link</a></footer></div><div id="chronicle-toasts" aria-live="polite"><button type="button">Undo</button></div>' : ''}
<script>window.Chronicle = { register: (n, d) => { window.hcImpl = d; document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)); }, escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director' };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_stage.js"></script><script src="/hero_creator.js"></script></body></html>`;
}

async function open(t, o = {}) {
  const { width = 390, height = 844, motion = 'reduce', plan = PLAN, draft = null, failWrites = false, mouse = false, dark = false } = o;
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport: { width, height }, reducedMotion: motion, hasTouch: !mouse && width < 900, isMobile: false });
  const pg = await ctx.newPage();
  const seen = { post: [] };
  await pg.addInitScript(({ draft, key, failWrites }) => {
    if (draft) localStorage.setItem(key, JSON.stringify(draft));
    if (failWrites) { Storage.prototype.setItem = function () { throw new Error('quota'); }; }
  }, { draft, key: DRAFT, failWrites });
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: file('js/hero_creator.js') });
    if (p === '/hero_stage.js') return route.fulfill({ contentType: 'text/javascript', body: file('js/hero_stage.js') });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: file('js/hero_rules_lines.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: file('css/hero_creator.css') });
    if (p === '/hero_stage.css') return route.fulfill({ contentType: 'text/css', body: file('css/hero_stage.css') });
    if (p === C + '/characters/new/plan') return ok(plan());
    if (p === C + '/characters/new' && req.method() === 'POST') { seen.post.push(JSON.parse(req.postData())); return ok({ id: 'e9', url: C + '/entities/e9' }, 201); }
    if (p.startsWith(C + '/entities/') || p === C + '/characters') return route.fulfill({ contentType: 'text/html', body: '<p>there</p>' });
    return route.fulfill({ contentType: 'text/html', body: page(o) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return { pg, seen, ctx };
}

const text = (pg, sel) => pg.locator(sel).first().innerText();
const draftOf = (pg) => pg.evaluate((k) => localStorage.getItem(k), DRAFT);
const figures = (pg) => pg.evaluate(() => Array.from(document.querySelectorAll('[data-stage] .hc-st')).map((t) => ({
  label: t.querySelector('span').textContent, value: t.querySelector('b').textContent, pv: t.querySelector('.hc-pv').textContent, only: t.classList.contains('is-pvonly'),
})));
// On a phone a neighbour card is brought to the middle by a tap; on a computer
// the card takes focus.
async function look(pg, desk, name) {
  if (desk) { await pg.locator('.hc-gcard', { hasText: name }).click(); return; }
  await pg.evaluate((n) => { const h = document.querySelector('[data-hand]'), c = [...h.children].find((x) => x.textContent.includes(n)); h.scrollLeft = c.offsetLeft - (h.clientWidth - c.offsetWidth) / 2; }, name);
}
async function choose(pg, desk, name) {
  if (desk) { await pg.locator('.hc-gcard', { hasText: name }).click(); await pg.click('.hc-right [data-act="choose"]'); return; }
  const card = pg.locator('.hc-hand .hc-card', { hasText: name }).first();
  await card.scrollIntoViewIfNeeded();
  await card.click({ position: { x: 20, y: 40 } });
  if (!(await pg.locator('.hc-sheet').count())) await card.click({ position: { x: 20, y: 40 } });
  await pg.waitForSelector('.hc-sfoot [data-act="choose"]');
  await pg.click('.hc-sfoot [data-act="choose"]');
}

for (const [label, width] of [['phone', 390], ['computer', 1100]]) {
  const desk = width >= 900;

  test(`${label}: the hero shows its name, its line and four figures, and no chip row`, { skip }, async (t) => {
    const { pg } = await open(t, { width });
    assert.equal(await pg.locator('.hc').getAttribute('data-look'), 'stage');
    assert.equal(await pg.locator('[data-stage]').count(), 1);
    assert.equal((await text(pg, '.hc-stg__nm')).trim(), 'Unnamed hero');
    assert.equal((await text(pg, '.hc-stg__ln')).trim(), 'Choose something to begin');
    const f = await figures(pg);
    assert.deepEqual(f.map((x) => x.label), ['Stamina', 'Speed', 'Size', 'Stability']);
    assert.ok(f.every((x) => x.value === '–'), 'a figure nothing gives yet is an en dash');
    assert.equal(await pg.locator('.hc-chips').count(), 0, 'the banner holds the picks, so no chip row');
    assert.equal(await pg.locator('.hc-ix').count(), 0, 'no "3 / 11" on a card');
    assert.equal(await pg.locator('.hc-sofar').count(), 0, 'the old foot block is not drawn');
    assert.equal(await pg.locator(desk ? '.hc-rail [data-stage]' : '.hc-stgw [data-stage]').count(), 1, desk ? 'the hero stands in the rail' : 'the hero sits in the banner');
    assert.equal(await pg.locator('.hc-por').first().evaluate((e) => e.tagName), 'BUTTON', 'with an About step the portrait is a button');
    assert.equal((await text(pg, '.hc-por')).trim(), '?');
    assert.equal(await pg.locator('h1').count(), 1);
    assert.equal(await pg.locator('h1').evaluate((e) => e.classList.contains('hc-sr')), !desk, 'the phone keeps its h1, hidden');
  });

  test(`${label}: looking at an entry shows what it would change and changes nothing`, { skip }, async (t) => {
    const { pg } = await open(t, { width, draft: DRAFT_ANCESTRY });
    const before = await draftOf(pg);
    assert.equal(await pg.locator('.hc-app').getAttribute('data-step'), '1', 'the draft leaves Culture next');
    // Culture gives nothing: looking at it reads "Looking at", no arrow.
    assert.equal((await text(pg, '.hc-stg__pv')).trim(), 'Looking at Academic');
    assert.ok((await figures(pg)).every((x) => x.pv === ''));
    // Go to Class and look at the entry in front.
    if (desk) await pg.click('.hc-rail .hc-row[data-k="class"]'); else { await pg.click('.hc-sbtn'); await pg.click('.hc-drape .hc-row[data-go="3"]'); }
    await pg.waitForSelector('[data-i="0"]');
    let f = await figures(pg);
    assert.equal(f[1].value, '5', 'the Dwarf already gives Speed');
    assert.equal((await text(pg, '.hc-stg__pv')).trim(), 'If you choose Fury');
    assert.deepEqual([f[0].pv, f[0].only], ['21', true], 'a figure with no value yet shows the would-be value alone');
    assert.equal(f[1].pv, '', 'a figure that would not change shows nothing');
    // Another entry in front: the preview follows.
    await look(pg, desk, 'Tactician');
    await pg.waitForFunction(() => /Tactician/.test(document.querySelector('.hc-stg__pv').textContent));
    assert.equal((await figures(pg))[0].pv, '24');
    assert.equal(await draftOf(pg), before, 'looking leaves the saved draft as it was');
    assert.equal(await pg.evaluate(() => Object.keys(document.querySelector('.hc')._heroCreator.picks).join()), 'ancestry', 'and the picks');
    // Picking Fury, then looking at Tactician: an arrow from 21 to 24.
    await choose(pg, desk, 'Fury');
    await pg.waitForFunction(() => /Fury/.test(document.querySelector('.hc-stg__ln').textContent));
    assert.equal((await text(pg, '.hc-stg__ln')).trim(), 'Dwarf Fury');
    f = await figures(pg);
    assert.equal(f[0].value, '21');
    assert.equal(f[2].value, '1S');
    assert.equal(f[3].value, '1');
  });
}

test('phone: an arrow shows a changed figure, "Looking at" a figure left as it is', { skip }, async (t) => {
  const draft = { picks: { ancestry: { name: 'Dwarf', options: [] }, class: { name: 'Fury', options: [] } }, name: '', about: {} };
  const { pg } = await open(t, { draft });
  await pg.click('.hc-sbtn'); await pg.click('.hc-drape .hc-row[data-go="3"]');
  await pg.waitForSelector('.hc-hand');
  // The picked card is in front: nothing is shown for the current pick.
  assert.equal((await text(pg, '.hc-stg__pv')).trim(), '');
  await look(pg, false, 'Censor');
  await pg.waitForFunction(() => /Censor/.test(document.querySelector('.hc-stg__pv').textContent));
  assert.equal((await text(pg, '.hc-stg__pv')).trim(), 'Looking at Censor', 'the same Stamina: nothing would change');
  await pg.locator('.hc-hand .hc-card', { hasText: 'Tactician' }).first().scrollIntoViewIfNeeded();
  await pg.evaluate(() => { const h = document.querySelector('[data-hand]'), c = [...h.children].find((x) => /Tactician/.test(x.textContent)); h.scrollLeft = c.offsetLeft - (h.clientWidth - c.offsetWidth) / 2; });
  await pg.waitForFunction(() => /Tactician/.test(document.querySelector('.hc-stg__pv').textContent));
  assert.equal((await text(pg, '.hc-stg__pv')).trim(), 'If you choose Tactician');
  const f = await figures(pg);
  assert.deepEqual([f[0].value, f[0].pv, f[0].only], ['21', '→ 24', false]);
});

test('phone: the stage tucks when the list is scrolled and opens again at the top', { skip }, async (t) => {
  const { pg } = await open(t, { draft: { picks: { ancestry: { name: 'Dwarf', options: [] }, culture: { name: 'Academic', options: [] }, career: { name: 'Soldier', options: [] } }, name: '', about: {} } });
  assert.equal(await pg.locator('.hc-app').getAttribute('data-step'), '3', 'opens on Class');
  await pg.click('[data-mode="list"]');
  const stg = pg.locator('[data-stage]');
  assert.equal(await stg.evaluate((e) => e.classList.contains('is-tuck')), false);
  const tall = await pg.locator('.hc-sstw').evaluate((e) => e.offsetHeight), big = await pg.locator('.hc-por').evaluate((e) => e.offsetWidth);
  assert.ok(tall > 30 && big === 56);
  await pg.evaluate(() => { document.querySelector('.hc-list').scrollTop = 200; });
  await pg.waitForFunction(() => document.querySelector('[data-stage]').classList.contains('is-tuck'));
  assert.equal(await pg.locator('.hc-sstw').evaluate((e) => e.offsetHeight), 0, 'the figures fold away');
  assert.equal(await pg.locator('.hc-por').evaluate((e) => e.offsetWidth), 40, 'the portrait shrinks');
  assert.equal(await pg.locator('.hc-stg__pv').evaluate((e) => e.offsetHeight), 0);
  assert.ok((await text(pg, '.hc-stg__nm')).length > 0, 'the name and line stay');
  const hit = await pg.locator('.hc-por').evaluate((e) => { const b = e.getBoundingClientRect(), s = getComputedStyle(e, '::after'); return Math.round(b.width) + 2 * -parseFloat(s.top); });
  assert.ok(hit >= 44, 'a tucked portrait still presses 44px');
  await pg.evaluate(() => { document.querySelector('.hc-list').scrollTop = 0; });
  await pg.waitForFunction(() => !document.querySelector('[data-stage]').classList.contains('is-tuck'));
  assert.equal(await pg.locator('.hc-por').evaluate((e) => e.offsetWidth), 56);
});

for (const [name, stageLook] of [['stage', true], ['classic', false]]) {
  test(`${name} look, phone: the step button keeps no hover wash after a tap, so the bars under it stay bars`, { skip }, async (t) => {
    const bg = (pg) => pg.evaluate(() => getComputedStyle(document.querySelector('.hc-sbtn')).backgroundColor);
    const touch = await open(t, { width: 360, height: 640, look: stageLook, dark: true });
    await touch.pg.click('.hc-sbtn');
    await touch.pg.waitForSelector('[data-drape]');
    await touch.pg.waitForTimeout(200);
    assert.equal(await bg(touch.pg), 'rgba(0, 0, 0, 0)', 'a finger leaves no wash');
    // A mouse over the same button still gets its wash.
    const mouse = await open(t, { width: 360, height: 640, look: stageLook, dark: true, mouse: true });
    await mouse.pg.hover('.hc-sbtn');
    await mouse.pg.waitForTimeout(200);
    assert.notEqual(await bg(mouse.pg), 'rgba(0, 0, 0, 0)', 'a mouse still gets it');
  });
}

test('Save and leave is a link to the Characters page; a try-out and a copy just Leave', { skip }, async (t) => {
  const own = await open(t, { draft: DRAFT_ANCESTRY });
  const lv = own.pg.locator('.hc-lv');
  assert.equal(await lv.evaluate((e) => e.tagName), 'A');
  assert.equal(await lv.getAttribute('href'), C + '/characters');
  assert.equal((await lv.innerText()).trim(), 'Save and leave');
  assert.ok((await lv.boundingBox()).height >= 44);
  const back = await open(t, { attrs: 'data-back-url="/campaigns/c1/entities/back"' });
  assert.equal(await back.pg.locator('.hc-lv').getAttribute('href'), '/campaigns/c1/entities/back');
  const tryout = await open(t, { attrs: 'data-tryout="true" data-user-id="u1"' });
  assert.equal((await tryout.pg.locator('.hc-lv').innerText()).trim(), 'Leave');
  assert.equal(await tryout.pg.locator('.hc-sv').count(), 0, 'nothing is saved as a draft, so no status');
  const copy = await open(t, { attrs: `data-preload='${JSON.stringify({ picks: {}, name: 'Copy' })}'` });
  assert.equal((await copy.pg.locator('.hc-lv').innerText()).trim(), 'Leave');
  assert.equal(await copy.pg.locator('.hc-sv').count(), 0);
});

test('the status says the draft is kept, or that this browser will not keep it', { skip }, async (t) => {
  const ok = await open(t, { width: 1100 });
  await ok.pg.locator('.hc-gcard', { hasText: 'Dwarf' }).click();
  await ok.pg.click('.hc-right [data-act="choose"]');
  assert.equal((await text(ok.pg, '.hc-sv')).trim(), 'Saved on this device');
  assert.equal(await ok.pg.locator('.hc-sv').evaluate((e) => e.classList.contains('is-bad')), false);
  assert.equal(await ok.pg.locator('.hc-sv').count(), 1);
  assert.equal(await ok.pg.locator('[aria-live] .hc-sv, .hc-sv[aria-live]').count(), 0, 'plain text, not a live region');
  const bad = await open(t, { width: 1100, failWrites: true });
  assert.equal((await text(bad.pg, '.hc-sv')).trim(), 'Saved on this device', 'nothing has failed before the first write');
  await bad.pg.locator('.hc-gcard', { hasText: 'Dwarf' }).click();
  await bad.pg.click('.hc-right [data-act="choose"]');
  assert.equal((await text(bad.pg, '.hc-sv')).trim(), "Not saved: this browser won't keep it");
  assert.equal(await bad.pg.locator('.hc-sv').evaluate((e) => e.classList.contains('is-bad')), true);
});

test('phone: About your hero has the Optional tag, the new line, and the portrait goes to it', { skip }, async (t) => {
  const { pg } = await open(t, { draft: DRAFT_FULL });
  assert.equal(await pg.locator('.hc-app').getAttribute('data-step'), '7', 'opens on Name and review');
  assert.equal(await pg.locator('.hc-stgw [data-stage]').count(), 0, 'only the bar sits in the banner on Name and review');
  await pg.click('.hc-sbtn'); await pg.click('.hc-drape .hc-row[data-go="6"]');
  await pg.waitForSelector('[data-about-step]');
  assert.match(await text(pg, '.hc-rev__t'), /About your hero\s+Optional/i);
  assert.equal((await text(pg, '.hc-ahd')).trim(), 'Fill in what you like now. The rest can wait for the sheet.');
  assert.equal(await pg.locator('[data-stage]').count(), 1, 'the hero is on the banner here');
  assert.match(await text(pg, '.hc-stg__ln'), /Dwarf Fury · she\/her/);
  // Typing pronouns updates the line at once, and is kept.
  await pg.fill('[data-about="pronouns"]', 'they/them');
  assert.match(await text(pg, '.hc-stg__ln'), /Dwarf Fury · they\/them/);
  await pg.click('.hc-sbtn'); await pg.click('.hc-drape .hc-row[data-go="0"]');
  await pg.click('.hc-por');
  await pg.waitForSelector('[data-about-step]');
  assert.equal(await pg.locator('.hc-app').getAttribute('data-step'), '6', 'the portrait goes to About your hero');
});

test('phone: the review\'s hero card is the hero, portrait first', { skip }, async (t) => {
  const { pg } = await open(t, { draft: DRAFT_FULL });
  const hero = pg.locator('.hc-hero');
  assert.equal((await hero.locator('.hc-por--big').innerText()).trim(), 'K');
  assert.deepEqual(await hero.locator('.hc-por--big').evaluate((e) => [e.offsetWidth, e.offsetHeight]), [96, 96]);
  assert.equal((await hero.locator('h2').innerText()).trim(), 'Korrin Ashvein');
  assert.equal((await hero.locator('.hc-line').innerText()).trim(), 'Dwarf Fury · Academic · she/her', 'the line adds the next pick it does not name');
  assert.match(await hero.innerText(), /STAMINA\s*27/i);
  // Typing a name moves the portrait's letter and the card at once.
  await pg.fill('[data-name]', 'Ysolde');
  assert.equal((await hero.locator('.hc-por--big').innerText()).trim(), 'Y');
  assert.equal((await hero.locator('h2').innerText()).trim(), 'Ysolde');
  assert.equal(await pg.locator('.hc-hero').evaluate((e) => getComputedStyle(e).backgroundImage.startsWith('linear-gradient')), true);
});

test('the hero posted is the one the classic creator posts for the same picks', { skip }, async (t) => {
  const stage = await open(t, { draft: DRAFT_FULL, width: 1100 });
  await stage.pg.click('[data-act="create"]');
  await stage.pg.waitForFunction(() => location.pathname.includes('/entities/'));
  const classic = await open(t, { draft: DRAFT_FULL, width: 1100, look: false });
  assert.equal(await classic.pg.locator('.hc').getAttribute('data-look'), null);
  assert.equal(await classic.pg.locator('.hc-stg').count(), 0, 'the plain mount draws no stage');
  await classic.pg.click('[data-act="create"]');
  await classic.pg.waitForFunction(() => location.pathname.includes('/entities/'));
  assert.equal(stage.seen.post.length, 1);
  assert.deepEqual(stage.seen.post[0], classic.seen.post[0]);
  assert.equal(stage.seen.post[0].name, 'Korrin Ashvein');
  assert.deepEqual(stage.seen.post[0].about, { pronouns: 'she/her' });
});

test('a system with no figures draws no tiles', { skip }, async (t) => {
  for (const width of [390, 1100]) {
    const { pg } = await open(t, { plan: PLAIN_PLAN, width });
    assert.equal(await pg.locator('[data-stage]').count(), 1);
    assert.equal(await pg.locator('.hc-sst, .hc-st').count(), 0);
    assert.equal((await text(pg, '.hc-stg__nm')).trim(), 'Unnamed hero');
    assert.equal(await pg.locator('.hc-por').first().evaluate((e) => e.tagName), 'SPAN', 'no About step: a plain portrait');
  }
});

test('the site chrome is inert while it is open and given back by destroy', { skip }, async (t) => {
  const { pg } = await open(t, { chrome: true, width: 1100 });
  assert.equal(await pg.locator('#sidebar').getAttribute('inert'), '');
  assert.equal(await pg.locator('#app-topbar').getAttribute('inert'), '');
  await pg.evaluate(() => { const el = document.querySelector('.hc'); window.hcImpl.destroy(el); });
  assert.equal(await pg.locator('#sidebar').getAttribute('inert'), null);
  assert.equal(await pg.locator('#app-topbar').getAttribute('inert'), null);
  // A page that set its own inert keeps it.
  const own = await open(t, { chrome: 'inert', width: 1100 });
  await own.pg.evaluate(() => { window.hcImpl.destroy(document.querySelector('.hc')); });
  assert.equal(await own.pg.locator('#sidebar').getAttribute('inert'), '');
});

test('the creator fills the window above the docked Table bar', { skip }, async (t) => {
  const { pg } = await open(t, { width: 1100, height: 720, drawer: true });
  const r = await pg.locator('.hc-app').evaluate((e) => { const b = e.getBoundingClientRect(); return { top: b.top, bottom: b.bottom, left: b.left, right: b.right, z: getComputedStyle(e).zIndex, pos: getComputedStyle(e).position }; });
  assert.equal(r.pos, 'fixed');
  assert.equal(r.top, 0);
  assert.ok(Math.abs(r.bottom - (720 - 48)) <= 1, 'it stops where the Table bar starts: ' + r.bottom);
  assert.equal(r.left, 0);
  assert.equal(r.right, 1100);
  assert.equal(r.z, '50');
  // Eight steps and the hero fit a 720-high window without the rail scrolling.
  const tall = await open(t, { width: 1280, height: 720 });
  const rail2 = await tall.pg.locator('.hc-rail').evaluate((e) => [e.scrollHeight, e.clientHeight, document.documentElement.scrollHeight, innerHeight]);
  assert.ok(rail2[0] <= rail2[1], 'eight steps and the hero fit 1280x720: ' + rail2);
  assert.ok(rail2[2] <= rail2[3], 'the page does not scroll');
});

// --- Readability on the gradient ---

// Every colour is read as the browser computed it and brought to sRGB through
// a canvas, so oklch and color-mix results are measured as drawn.
// washes says which words stand on a white wash over the gradient, and how
// strong it is: a figure's tile, the rail's current row.
async function contrasts(pg, sels, washes = {}) {
  return pg.evaluate(({ sels, washes }) => {
    const cv = document.createElement('canvas'); cv.width = cv.height = 1;
    const cx = cv.getContext('2d', { willReadFrequently: true });
    const rgb = (css) => { cx.clearRect(0, 0, 1, 1); cx.fillStyle = '#000'; cx.fillStyle = css; cx.fillRect(0, 0, 1, 1); const d = cx.getImageData(0, 0, 1, 1).data; return [d[0], d[1], d[2], d[3] / 255]; };
    const over = (f, b) => [0, 1, 2].map((i) => f[i] * f[3] + b[i] * (1 - f[3]));
    const lum = (c) => { const v = c.map((x) => { x /= 255; return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4); }); return 0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2]; };
    const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); };
    const probe = document.createElement('i'); document.querySelector('.hc').appendChild(probe);
    const ends = ['--hc-stg1', '--hc-stg2'].map((v) => { probe.style.background = 'var(' + v + ')'; return rgb(getComputedStyle(probe).backgroundColor); });
    probe.remove();
    return sels.map((s) => {
      const el = document.querySelector(s);
      if (!el) return { sel: s, missing: true };
      const cs = getComputedStyle(el), fg = rgb(cs.color);
      fg[3] *= parseFloat(cs.opacity);
      const worst = Math.min(...ends.map((e) => { let bg = e.slice(0, 3); if (washes[s]) bg = over([255, 255, 255, washes[s]], bg); return ratio(over(fg, bg), bg); }));
      return { sel: s, ratio: Math.round(worst * 100) / 100, text: el.textContent.trim().slice(0, 20) };
    });
  }, { sels, washes });
}

// The default, a pale yellow, a near white, and the greens, which are the
// brightest colours at any one lightness.
for (const accent of ['#6366f1', '#facc15', '#f8fafc', '#22c55e', '#00ff00']) {
  test(`white words on the gradient read at 4.5:1 with the accent ${accent}`, { skip }, async (t) => {
    // The banner on a phone, in light and dark.
    const ph = await open(t, { accent, draft: DRAFT_ANCESTRY });
    const banner = ['.hc-lv span', '.hc-sv', '.hc-stg__nm', '.hc-stg__ln', '.hc-stg__pv', '.hc-st span', '.hc-st b', '.hc-pv'];
    for (const r of await contrasts(ph.pg, banner, { '.hc-st span': 0.14, '.hc-st b': 0.14, '.hc-pv': 0.14 })) {
      assert.ok(!r.missing, r.sel + ' is drawn');
      assert.ok(r.ratio >= 4.5, `${accent} banner ${r.sel} "${r.text}" is ${r.ratio}:1`);
    }
    // A figure with nothing yet shows its would-be value alone, dimmed.
    await ph.pg.evaluate(() => document.querySelector('.hc-st').classList.add('is-pvonly'));
    const dim = await contrasts(ph.pg, ['.hc-st.is-pvonly .hc-pv'], { '.hc-st.is-pvonly .hc-pv': 0.14 });
    assert.ok(dim[0].ratio >= 4.5, `${accent} the dimmed figure is ${dim[0].ratio}:1`);
    // The rail on a computer, in dark.
    const dk = await open(t, { accent, width: 1100, dark: true, draft: DRAFT_ANCESTRY });
    const now = '.hc-rail .hc-row[aria-current="step"] .hc-lb ';
    const rail = ['.hc-rail .hc-stg__nm', '.hc-rail .hc-stg__ln', '.hc-rail .hc-st span', '.hc-rail .hc-st b', '.hc-rail .hc-row .hc-lb b', '.hc-rail .hc-row:not([aria-current]) .hc-lb span', now + 'b', now + 'span', '.hc-rail .hc-num.is-done'];
    for (const r of await contrasts(dk.pg, rail, { '.hc-rail .hc-st span': 0.14, '.hc-rail .hc-st b': 0.14, [now + 'b']: 0.18, [now + 'span']: 0.18 })) {
      assert.ok(!r.missing, r.sel + ' is drawn');
      assert.ok(r.ratio >= 4.5 || r.sel.endsWith('.is-done'), `${accent} rail ${r.sel} "${r.text}" is ${r.ratio}:1`);
    }
    // The review's hero card.
    const rv = await open(t, { accent, draft: DRAFT_FULL });
    const card = ['.hc-hero .hc-who', '.hc-hero h2', '.hc-hero .hc-line', '.hc-hero .hc-stat span', '.hc-hero .hc-stat b'];
    for (const r of await contrasts(rv.pg, card, { '.hc-hero .hc-stat span': 0.14, '.hc-hero .hc-stat b': 0.14 })) {
      assert.ok(!r.missing, r.sel + ' is drawn');
      assert.ok(r.ratio >= 4.5, `${accent} hero card ${r.sel} "${r.text}" is ${r.ratio}:1`);
    }
  });
}

test('no sideways scroll at 360 wide, and no figure label is cut', { skip }, async (t) => {
  const { pg } = await open(t, { width: 360, draft: DRAFT_ANCESTRY });
  const fits = () => pg.evaluate(() => ({
    page: document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    app: document.querySelector('.hc-app').scrollWidth <= document.querySelector('.hc-app').clientWidth,
    tiles: [...document.querySelectorAll('.hc-st')].every((e) => e.scrollWidth <= e.clientWidth + 0.5 && e.querySelector('span').scrollWidth <= e.querySelector('span').clientWidth + 0.5),
  }));
  assert.deepEqual(await fits(), { page: true, app: true, tiles: true }, 'the culture hand');
  await pg.click('[data-mode="list"]');
  assert.deepEqual(await fits(), { page: true, app: true, tiles: true }, 'the list');
  await pg.click('.hc-sbtn'); await pg.click('.hc-drape .hc-row[data-go="6"]');
  await pg.waitForSelector('[data-about-step]');
  assert.deepEqual(await fits(), { page: true, app: true, tiles: true }, 'About your hero');
  await pg.click('.hc-sbtn'); await pg.click('.hc-drape .hc-row[data-go="7"]');
  await pg.waitForSelector('[data-rev]');
  const rv = await pg.evaluate(() => ({ page: document.documentElement.scrollWidth <= document.documentElement.clientWidth, rev: document.querySelector('.hc-rev').scrollWidth <= document.querySelector('.hc-rev').clientWidth }));
  assert.deepEqual(rv, { page: true, rev: true }, 'Name and review');
});

// --- A mouse can move the hand ---

test('phone layout with a mouse: drag the hand, flick it, roll the wheel; a drag is not a click', { skip }, async (t) => {
  const { pg } = await open(t, { mouse: true, draft: DRAFT_ANCESTRY });
  await pg.click('.hc-sbtn'); await pg.click('.hc-drape .hc-row[data-go="3"]');
  await pg.waitForSelector('.hc-hand');
  const at = () => pg.evaluate(() => +document.querySelector('[data-posn]').textContent - 1);
  assert.equal(await at(), 0);
  const box = await pg.locator('.hc-hand .hc-card').first().boundingBox();
  const y = box.y + 120, x = box.x + box.width / 2;
  // A short drag of 60px moves one card on.
  await pg.mouse.move(x, y); await pg.mouse.down(); await pg.mouse.move(x - 30, y, { steps: 3 }); await pg.mouse.move(x - 60, y, { steps: 3 }); await pg.mouse.up();
  await pg.waitForFunction(() => document.querySelector('[data-posn]').textContent === '2');
  assert.equal(await at(), 1);
  assert.equal(await pg.locator('.hc-sheet').count(), 0, 'letting go over a card is not a click on it');
  // A drag the other way, and the wheel one card at a time.
  const b2 = await pg.locator('.hc-hand .hc-card').nth(1).boundingBox();
  await pg.mouse.move(b2.x + 60, y); await pg.mouse.down(); await pg.mouse.move(b2.x + 120, y, { steps: 4 }); await pg.mouse.move(b2.x + 160, y, { steps: 4 }); await pg.mouse.up();
  await pg.waitForFunction(() => document.querySelector('[data-posn]').textContent === '1');
  assert.equal(await at(), 0);
  await pg.mouse.move(x, y); await pg.mouse.wheel(0, 120);
  await pg.waitForFunction(() => document.querySelector('[data-posn]').textContent === '2');
  assert.equal(await at(), 1, 'one card a step');
  // A plain click still opens the card in the middle.
  await pg.waitForTimeout(500);
  const mid = await pg.locator('.hc-hand .hc-card').nth(1).boundingBox();
  await pg.mouse.click(mid.x + mid.width / 2, mid.y + 60);
  await pg.waitForSelector('.hc-sheet');
});

// --- Lively: the flight ends with the stage redrawn ---

for (const [label, width] of [['phone', 390], ['computer', 1100]]) {
  test(`${label}, Lively: the chosen card flies to the hero and the stage is redrawn`, { skip }, async (t) => {
    const { pg } = await open(t, { width, motion: 'no-preference', draft: DRAFT_ANCESTRY });
    assert.equal(await pg.locator('.hc-app').getAttribute('data-mo'), 'lively');
    if (width < 900) { await pg.click('.hc-sbtn'); await pg.click('.hc-drape .hc-row[data-go="3"]'); } else await pg.click('.hc-rail .hc-row[data-k="class"]');
    await pg.waitForSelector(width < 900 ? '.hc-hand .hc-card' : '.hc-gcard');
    await pg.waitForTimeout(700);
    await choose(pg, width >= 900, 'Fury');
    await pg.waitForSelector('.hc-ghost', { timeout: 3000 });
    await pg.waitForFunction(() => !document.querySelector('.hc-ghost'), null, { timeout: 5000 });
    assert.equal(await pg.evaluate(() => document.querySelector('.hc')._heroCreator.busy), false, 'busy is cleared');
    await pg.waitForFunction(() => /Fury/.test(document.querySelector('.hc-stg__ln').textContent));
    assert.equal((await text(pg, '.hc-stg__ln')).trim(), 'Dwarf Fury');
    await pg.waitForFunction(() => document.querySelector('[data-stat="stamina"]').textContent === '21', null, { timeout: 3000 });
    assert.equal(await pg.locator('[data-stage]').count(), 1);
    if (width >= 900) assert.equal(await pg.locator('.hc-rail [data-stage]').count(), 1, 'the rail is redrawn with the stage in it');
  });
}

// --- Everything outside the creator is out of reach ---

// Where a Tab press lands, for as many presses as there are controls and then
// some, so a loop through the page is seen whole.
async function tabStops(pg, key, n) {
  const seen = [];
  for (let i = 0; i < n; i++) {
    await pg.keyboard.press(key);
    seen.push(await pg.evaluate(() => {
      const a = document.activeElement;
      if (!a || a === document.body) return 'body';
      if (a.closest('.hc')) return 'creator';
      if (a.closest('#tt-drawer')) return 'table';
      if (a.closest('#chronicle-toasts')) return 'toasts';
      return 'OUTSIDE ' + (a.id || a.textContent.trim());
    }));
  }
  return seen;
}

test('Tab and Shift+Tab only land on the creator, the Table bar and the toasts', { skip }, async (t) => {
  const { pg } = await open(t, { width: 1100, height: 720, chrome: true, drawer: true, extras: true });
  const inertNow = () => pg.evaluate(() => [...document.querySelectorAll('[inert]')].map((e) => e.id).sort());
  assert.deepEqual(await inertNow(), ['app-topbar', 'foot', 'notice', 'pre', 'sidebar', 'skip'].sort(), 'every sibling up the tree is inert; the page\'s own inert stays');
  assert.equal(await pg.locator('#tt-drawer').getAttribute('inert'), null, 'the Table bar stays usable');
  assert.equal(await pg.locator('#chronicle-toasts').getAttribute('inert'), null, 'the toasts stay usable');
  assert.equal(await pg.locator('#shell').getAttribute('inert'), null, 'the creator\'s own ancestors are never inert');
  for (const key of ['Tab', 'Shift+Tab']) {
    const stops = await tabStops(pg, key, 90);
    assert.deepEqual([...new Set(stops)].filter((x) => x !== 'creator' && x !== 'table' && x !== 'toasts' && x !== 'body'), [], key + ' never lands outside: ' + [...new Set(stops)]);
    assert.ok(stops.includes('creator'), key + ' reaches the creator');
    assert.ok(stops.includes('table'), key + ' reaches the Table bar');
  }
  // A bar that arrives later (HTMX) is covered too.
  await pg.evaluate(() => {
    const b = document.createElement('div'); b.id = 'late'; b.innerHTML = '<button type="button">Late</button>';
    document.getElementById('shell').insertBefore(b, document.getElementById('main-content'));
    const c = document.createElement('div'); c.id = 'latest'; c.innerHTML = '<button type="button">Later</button>';
    document.body.insertBefore(c, document.body.firstChild);
  });
  await pg.waitForFunction(() => document.getElementById('late').hasAttribute('inert') && document.getElementById('latest').hasAttribute('inert'), null, { timeout: 2000 });
  // Leaving gives back exactly what it took.
  await pg.evaluate(() => window.hcImpl.destroy(document.querySelector('.hc')));
  assert.deepEqual(await inertNow(), ['pre'], 'only the page\'s own inert is left');
  assert.equal(await pg.evaluate(() => document.documentElement.hasAttribute('data-full-surface')), false);
  // After leaving, a bar that arrives is left alone.
  await pg.evaluate(() => { const b = document.createElement('div'); b.id = 'after'; document.body.appendChild(b); });
  await pg.waitForTimeout(100);
  assert.equal(await pg.locator('#after').getAttribute('inert'), null);
});

test('the Table bar that arrives after the creator, unstyled at first, is not made inert', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390, extras: true });
  await pg.evaluate(() => {
    const d = document.createElement('div'); d.id = 'tt-drawer'; d.innerHTML = '<button type="button">Table</button>'; document.body.appendChild(d);
  });
  await pg.waitForTimeout(100);
  assert.equal(await pg.locator('#tt-drawer').getAttribute('inert'), null);
  assert.equal(await pg.locator('#notice').getAttribute('inert'), '');
});

test('the creator marks the page as covered while open, so page shortcuts stand down', { skip }, async (t) => {
  const { pg } = await open(t, { width: 1100 });
  const mark = () => pg.evaluate(() => document.documentElement.hasAttribute('data-full-surface'));
  assert.equal(await mark(), true);
  await pg.evaluate(() => window.hcImpl.destroy(document.querySelector('.hc')));
  assert.equal(await mark(), false);
});

// --- The practice and try-out pages take the look ---

for (const [label, attrs, word] of [
  ['try-out', 'data-tryout="true" data-user-id="u1"', 'Leave'],
  ['practice copy', `data-practice="true" data-preload='${JSON.stringify({ picks: {}, name: 'Copy' })}'`, 'Leave'],
  ['practice', 'data-practice="true"', 'Save and leave'],
]) {
  test(`${label} with data-look="stage" draws the stage look, and ${word} goes where the classic back goes`, { skip }, async (t) => {
    const stage = await open(t, { attrs, width: 1100 });
    assert.equal(await stage.pg.locator('.hc').getAttribute('data-look'), 'stage');
    assert.equal(await stage.pg.locator('[data-stage]').count(), 1);
    assert.equal((await stage.pg.locator('.hc-lv').innerText()).trim(), word);
    const classic = await open(t, { attrs, width: 1100, look: false });
    assert.equal(await classic.pg.locator('[data-stage]').count(), 0, 'without the look it is the classic creator');
    const back = await classic.pg.locator('.hc-ibtn').first().getAttribute('href');
    assert.equal(await stage.pg.locator('.hc-lv').getAttribute('href'), back);
    assert.equal(await stage.pg.locator('.hc-lv').getAttribute('href'), C + '/characters');
  });
}

// --- Long words ---

const LONG_ENTRY = 'Wandering Keeper of the Seventh Lantern in the Hollow Marsh';
const longPlan = () => {
  const p = PLAN();
  p.steps[0].choices.push(ch(LONG_ENTRY, { speed: 7, size: '1M', stability: 2 }));
  return p;
};
// Nothing in the creator is wider than the box that holds it, and nothing
// scrolls sideways.
const noSpill = (pg) => pg.evaluate(() => {
  const bad = [];
  document.querySelectorAll('.hc-app *').forEach((el) => {
    const p = el.parentElement, cs = getComputedStyle(el);
    if (!p || cs.display === 'none' || cs.position === 'fixed' || cs.position === 'absolute') return;
    if (/hc-(sheet|drape|scrim|tip|live)/.test(el.className) || el.closest('.hc-hand, [data-hand], .hc-drape, .hc-sheet')) return;
    const a = el.getBoundingClientRect(), b = p.getBoundingClientRect();
    if (a.width > 0 && (a.right > b.right + 1 || a.left < b.left - 1) && getComputedStyle(p).overflowX === 'visible') bad.push((el.className || el.tagName) + ' ' + Math.round(a.left) + '-' + Math.round(a.right) + ' in ' + Math.round(b.left) + '-' + Math.round(b.right));
  });
  const d = document.documentElement;
  return { bad, page: d.scrollWidth <= d.clientWidth, app: document.querySelector('.hc-app').scrollWidth <= document.querySelector('.hc-app').clientWidth };
});

for (const width of [1280, 1100]) {
  test(`computer ${width}: the "If you choose" line stays on one line and ends in an ellipsis`, { skip }, async (t) => {
    const { pg } = await open(t, { width, height: 800, plan: longPlan, draft: DRAFT_ANCESTRY, motion: 'reduce' });
    await pg.click('.hc-rail .hc-row[data-k="ancestry"]');
    await pg.locator('.hc-gcard', { hasText: LONG_ENTRY }).click();
    await pg.waitForFunction(() => /If you choose/.test(document.querySelector('.hc-stg__pv').textContent));
    const r = await pg.evaluate(() => {
      const l = document.querySelector('.hc-stg__pv'), box = l.closest('.hc-rail').getBoundingClientRect(), b = l.getBoundingClientRect(), cs = getComputedStyle(l);
      return { overflow: cs.textOverflow, white: cs.whiteSpace, left: b.left, right: b.right, boxL: box.left, boxR: box.right, cut: l.scrollWidth > l.clientWidth, scrollLeft: l.scrollLeft, h: b.height, text: l.textContent };
    });
    assert.equal(r.overflow, 'ellipsis');
    assert.equal(r.white, 'nowrap');
    assert.ok(r.left >= r.boxL - 0.5 && r.right <= r.boxR + 0.5, 'the line sits inside the rail: ' + JSON.stringify(r));
    assert.ok(r.cut, 'the long name does not fit, so it is cut');
    assert.equal(r.scrollLeft, 0, 'cut at the end only: the start of the line shows');
    assert.ok(r.h <= 20, 'one line');
    assert.match(r.text, /^If you choose Wandering/);
    // The start of the line is at the rail's left edge, not before it.
    const fit = await noSpill(pg);
    assert.deepEqual(fit.bad, [], 'no element wider than its container');
    assert.equal(fit.page && fit.app, true);
  });
}

test('phone 360: an unbroken 99-character hero name wraps on the review card', { skip }, async (t) => {
  const name = 'W'.repeat(99);
  const { pg } = await open(t, { width: 360, height: 780, draft: { ...DRAFT_FULL, name } });
  await pg.waitForSelector('.hc-hero');
  const r = await pg.evaluate(() => {
    const h = document.querySelector('.hc-hero h2'), card = document.querySelector('.hc-hero').getBoundingClientRect(), b = h.getBoundingClientRect(), d = document.documentElement;
    return { left: b.left, right: b.right, cl: card.left, cr: card.right, sw: h.scrollWidth, cw: h.clientWidth, lines: Math.round(b.height / parseFloat(getComputedStyle(h).lineHeight)), page: d.scrollWidth <= d.clientWidth, rev: document.querySelector('.hc-rev').scrollWidth <= document.querySelector('.hc-rev').clientWidth };
  });
  assert.ok(r.left >= r.cl - 0.5 && r.right <= r.cr + 0.5, 'the name sits inside the card: ' + JSON.stringify(r));
  assert.ok(r.sw <= r.cw + 0.5, 'nothing of the name is cut');
  assert.ok(r.lines >= 2, 'it wraps');
  assert.equal(r.page && r.rev, true, 'nothing scrolls sideways');
  const fit = await noSpill(pg);
  assert.deepEqual(fit.bad, [], 'no element wider than its container');
  // The banner's name wraps too.
  assert.equal(await pg.evaluate(() => getComputedStyle(document.querySelector('.hc-hero .hc-line')).overflowWrap), 'anywhere');
});

// --- The Table bar's box ---

test('the creator follows the Table bar when its height or place changes after it drew', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390, height: 844, drawer: true });
  const bottom = () => pg.locator('.hc-app').evaluate((e) => Math.round(e.getBoundingClientRect().bottom));
  const frames = () => pg.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(() => requestAnimationFrame(r)))));
  assert.equal(await bottom(), 844 - 48);
  await pg.evaluate(() => { document.getElementById('tt-drawer').style.height = '80px'; });
  await frames();
  assert.equal(await bottom(), 844 - 80, 'a taller bar: the creator stops higher');
  await pg.evaluate(() => { document.getElementById('tt-drawer').style.height = '38px'; });
  await frames();
  assert.equal(await bottom(), 844 - 38, 'a shorter bar: the creator reaches lower');
  // A bar that slides into its place (a transform, no resize) is followed when
  // the slide ends at rest, and left alone while it is lifted.
  await pg.evaluate(() => {
    const d = document.getElementById('tt-drawer');
    d.style.height = '300px'; d.setAttribute('data-h', 'rest');
    d.style.transition = 'transform 120ms linear'; d.style.transform = 'translateY(100%)';
  });
  await frames();
  await pg.waitForFunction(() => Math.round(document.querySelector('.hc-app').getBoundingClientRect().bottom) === 844, null, { timeout: 2000 });
  await pg.evaluate(() => { const d = document.getElementById('tt-drawer'); d.setAttribute('data-h', 'lift'); d.style.transform = 'translateY(40%)'; });
  await pg.waitForTimeout(400);
  assert.equal(await bottom(), 844, 'a lifted bar covers the creator, which does not jump');
  await pg.evaluate(() => { const d = document.getElementById('tt-drawer'); d.setAttribute('data-h', 'rest'); d.style.transform = 'translateY(100%)'; });
  await pg.waitForTimeout(400);
  assert.equal(await bottom(), 844, 'back at rest: reaches the bottom again');
  await pg.evaluate(() => { const d = document.getElementById('tt-drawer'); d.style.transform = 'none'; d.style.height = '48px'; });
  await frames();
  await pg.waitForTimeout(300);
  assert.equal(await bottom(), 844 - 48);
  // Leaving stops the watching.
  await pg.evaluate(() => window.hcImpl.destroy(document.querySelector('.hc')));
});

// --- The "All steps" list and the keyboard, in both looks ---

// Nine list steps, with About your hero and Name and review: eleven rows, more
// than a 360x640 phone holds under the header.
const LONG = () => ({
  systemName: 'Draw Steel',
  about: [{ key: 'pronouns', label: 'Pronouns', long: false }],
  steps: ['Ancestry', 'Culture', 'Career', 'Class', 'Subclass', 'Kit', 'Complication', 'Boon', 'Bane'].map((l) => ({ fieldKey: l.toLowerCase(), label: l, choices: [ch(l + ' one'), ch(l + ' two')] })),
});
const looks = [['stage', true], ['classic', false]];

for (const [name, look] of looks) {
  for (const [w, h] of [[360, 640], [360, 500]]) {
    test(`${name} look, ${w}x${h}: the All steps list stops at the app's foot and scrolls inside itself`, { skip }, async (t) => {
      const { pg } = await open(t, { width: w, height: h, plan: LONG, look });
      await pg.waitForSelector('.hc-sbtn');
      await pg.waitForTimeout(300);
      const rows = async () => pg.evaluate(() => {
        const d = document.querySelector('[data-drape]'), app = document.querySelector('.hc-app').getBoundingClientRect(), dr = d.getBoundingClientRect();
        const all = [...d.querySelectorAll('.hc-row')], cur = d.querySelector('[aria-current]').getBoundingClientRect();
        return { n: all.length, drapeBottom: dr.bottom, appBottom: app.bottom, win: window.innerHeight, scrolls: d.scrollHeight > d.clientHeight + 1,
          curIn: cur.top >= dr.top - 1 && cur.bottom <= dr.bottom + 1, oy: getComputedStyle(d).overflowY };
      });
      await pg.click('.hc-sbtn');
      await pg.waitForSelector('[data-drape]');
      let r = await rows();
      assert.equal(r.n, 11);
      assert.ok(r.drapeBottom <= r.appBottom + 1, `the list ends at the app's foot: ${r.drapeBottom} vs ${r.appBottom}`);
      assert.ok(r.drapeBottom <= r.win + 1);
      assert.equal(r.oy, 'auto');
      assert.equal(r.scrolls, true, 'eleven rows are more than this height holds');
      // The last row is reached by scrolling the list itself.
      const last = await pg.evaluate(() => {
        const d = document.querySelector('[data-drape]'), rows = d.querySelectorAll('.hc-row'), before = window.scrollY;
        d.scrollTop = d.scrollHeight;
        const a = rows[rows.length - 1].getBoundingClientRect(), b = d.getBoundingClientRect();
        return { inDrape: a.top >= b.top - 1 && a.bottom <= b.bottom + 1, inWindow: a.bottom <= window.innerHeight + 1, pageMoved: window.scrollY !== before };
      });
      assert.deepEqual(last, { inDrape: true, inWindow: true, pageMoved: false });
      // On the last step, the list opens with its row in view.
      await pg.click('.hc-drape .hc-row[data-go="10"]');
      await pg.waitForTimeout(400);
      await pg.click('.hc-sbtn');
      await pg.waitForSelector('[data-drape]');
      r = await rows();
      assert.equal(r.curIn, true, 'the current row is in view as the list opens');
      // Closing still removes it.
      await pg.click('.hc-sbtn');
      await pg.waitForFunction(() => !document.querySelector('[data-drape]'), null, { timeout: 2000 });
    });
  }

  test(`${name} look, 390x844: a short list does not scroll and no row is clipped`, { skip }, async (t) => {
    const { pg } = await open(t, { width: 390, height: 844, look });
    await pg.waitForSelector('.hc-sbtn');
    await pg.waitForTimeout(300);
    await pg.click('.hc-sbtn');
    await pg.waitForSelector('[data-drape]');
    const r = await pg.evaluate(() => {
      const d = document.querySelector('[data-drape]'), rows = [...d.querySelectorAll('.hc-row')], dr = d.getBoundingClientRect(), last = rows[rows.length - 1].getBoundingClientRect();
      return { scrolls: d.scrollHeight > d.clientHeight + 1, top: d.scrollTop, lastIn: last.bottom <= dr.bottom + 1, clipped: parseFloat(getComputedStyle(d).maxHeight) < d.scrollHeight };
    });
    assert.deepEqual(r, { scrolls: false, top: 0, lastIn: true, clipped: false });
  });

  test(`${name} look, 390x844 with a mouse and no touch: Tab and Shift+Tab keep the focused card in the strip`, { skip }, async (t) => {
    const many = () => ({ systemName: 'Draw Steel', steps: [{ fieldKey: 'class', label: 'Class', choices: Array.from({ length: 12 }, (_, i) => ch('Entry ' + i)) }] });
    const { pg, ctx } = await open(t, { width: 390, height: 844, plan: many, look, mouse: true });
    await pg.waitForSelector('.hc-card[data-i="0"]');
    await pg.waitForTimeout(500);
    await pg.focus('.hc-hand .hc-card[data-i="0"]');
    const where = () => pg.evaluate(() => {
      const a = document.activeElement, hand = document.querySelector('[data-hand]');
      if (!a || !hand.contains(a)) return { out: true };
      const r = a.getBoundingClientRect(), hr = hand.getBoundingClientRect();
      return { i: a.getAttribute('data-i'), inside: r.left >= hr.left - 1 && r.right <= hr.right + 1 };
    });
    const seen = [];
    for (let k = 0; k < 8; k++) { await pg.keyboard.press('Tab'); await pg.waitForTimeout(250); seen.push(await where()); }
    for (let k = 0; k < 2; k++) { await pg.keyboard.press('Shift+Tab'); await pg.waitForTimeout(250); seen.push(await where()); }
    assert.deepEqual(seen.map((s) => s.inside), seen.map(() => true), JSON.stringify(seen));
    assert.deepEqual(seen.map((s) => s.i), ['1', '2', '3', '4', '5', '6', '7', '8', '7', '6']);
    void ctx;
  });
}
