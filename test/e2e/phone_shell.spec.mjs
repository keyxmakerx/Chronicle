// phone_shell.spec.mjs — the app shell and page lists on a phone (390 by 844).
//
// A harness page, as in annals_phone_scroll.spec.mjs: the real phone_shell.css
// and the real .cta-* rules from input.css, with the header, the time zone
// banner and a page row transcribed from layouts/app.templ, the callout
// handler and entities/entity_card.templ. The first test pins that
// transcription to the templates, so a renamed class fails here instead of
// leaving the harness green over a page that changed.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/phone_shell.spec.mjs

import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(repoRoot, p), 'utf8');
const shellCSS = read('static/css/phone_shell.css');

// The banner rules, as the app ships them (inside Tailwind's component layer,
// which is left off here so these load as plain CSS).
const inputCSS = read('static/css/input.css');
const ctaStart = inputCSS.indexOf('  .cta-host:empty');
const ctaEnd = inputCSS.indexOf('@media (prefers-reduced-motion: reduce) {\n  .cta-bar');
assert.ok(ctaStart > 0 && ctaEnd > ctaStart, 'the .cta-* block moved in input.css');
const ctaCSS = inputCSS.slice(ctaStart, ctaEnd).replace(/\}\s*$/, '');

const UTILITIES = `
  html { font-size: 16px; } html, body { margin: 0; font-family: sans-serif; line-height: 1.4; } body { font-size: 14px; }
  :root { --color-border-light: #f3f4f6; }
  .hidden { display: none; } .flex { display: flex; } .block { display: block; }
  .items-center { align-items: center; } .items-start { align-items: flex-start; }
  .justify-between { justify-content: space-between; } .shrink-0 { flex-shrink: 0; }
  .min-w-0 { min-width: 0; } .ml-auto { margin-left: auto; } .relative { position: relative; }
  .gap-1 { gap: .25rem; } .gap-2 { gap: .5rem; } .gap-1\\.5 { gap: .375rem; }
  .px-3 { padding-inline: .75rem; } .px-2\\.5 { padding-inline: .625rem; } .py-1\\.5 { padding-block: .375rem; }
  .px-1\\.5 { padding-inline: .375rem; } .py-1 { padding-block: .25rem; } .px-1 { padding-inline: .25rem; }
  .p-0 { padding: 0; } .overflow-hidden { overflow: hidden; } .truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .h-14 { height: 3.5rem; } .h-28 { height: 7rem; } .mb-1 { margin-bottom: .25rem; }
  .font-semibold { font-weight: 600; } .rounded-full { border-radius: 9999px; }
  .max-w-\\[160px\\] { max-width: 160px; }
  .btn-icon { padding: .5rem; border: 0; background: none; border-radius: .5rem; }
  .card { border: 1px solid #e5e7eb; border-radius: .5rem; background: #fff; box-shadow: 0 1px 2px rgba(0,0,0,.1); }
  #app-topbar { border-bottom: 1px solid #e5e7eb; box-sizing: border-box; }
  .av { width: 28px; height: 28px; border-radius: 50%; background: #888; display: inline-block; }
  .ico { width: 16px; height: 16px; display: inline-block; background: currentColor; }
  h3 { margin: 0; font-size: .875rem; }
  @media (min-width: 768px) { .md\\:hidden { display: none; } }
`;

const NAME15 = 'The Ashen Reach';
const icon = '<span class="ico"></span>';

function header(name) {
  return `
  <header id="app-topbar" class="h-14 flex items-center px-3 gap-2 relative">
    <div class="tb-left flex items-center gap-2 min-w-0">
      <button id="nav-toggle" type="button" class="btn-icon shrink-0 md:hidden" aria-label="Open navigation">${icon}</button>
      <div class="relative min-w-0">
        <button type="button" id="name-btn" class="flex items-center gap-1.5 max-w-full px-2.5 py-1.5 font-semibold" style="border:0;background:none">
          <span class="truncate max-w-[160px]" id="name" data-topbar-name>${name}</span>
          <span class="ico shrink-0" style="width:14px;height:14px"></span>
        </button>
      </div>
    </div>
    <div class="flex items-center gap-1 shrink-0 ml-auto">
      <button id="topbar-search-trigger" type="button" class="btn-icon" title="Search (Ctrl+K)">${icon}</button>
      <div class="tt-host"><button type="button" class="btn-icon tt-tab" data-tt-tab aria-label="The table">${icon}</button></div>
      <div class="relative" data-widget="notification-bell">
        <button type="button" class="btn-icon relative tb-bell" data-notif-toggle aria-label="Notifications">${icon}</button>
      </div>
      <div class="relative">
        <button type="button" id="user-menu-btn" class="flex items-center gap-1.5 px-1.5 py-1" style="border:0;background:none" aria-label="Your account menu"><span class="av"></span></button>
        <div id="user-menu-panel" class="relative">
          <button type="button" class="dropdown-item tb-menu-notif w-full text-left" data-notif-menu>Notifications <span class="tb-menu-count" data-notif-menu-count>3</span></button>
        </div>
      </div>
    </div>
  </header>`;
}

const tzBanner = (withAccept) => `
  <div class="cta-host"><div class="cta-bar" data-cta="timezone" role="status">
    <i class="cta-ico">c</i>
    <span class="cta-msg"><span class="cta-long">Chronicle does not know your timezone, so times are not shown in your local time</span><span class="cta-short">Times aren't in your time zone yet</span></span>
    <button type="button" class="cta-go" data-cta-tz-accept ${withAccept ? '' : 'hidden'}>Use <span data-cta-zone>London time</span></button><a class="cta-alt" href="/account">Choose</a>
    <button type="button" class="cta-x" data-cta-dismiss aria-label="Dismiss"><i>x</i></button>
  </div></div>`;

const row = (name, kind) => `
  <a href="#" class="ec-card card p-0 overflow-hidden block group">
    <div class="ec-band h-14 flex items-center justify-center"><i>i</i></div>
    <div class="ec-body px-3 py-2.5">
      <div class="ec-head flex items-start justify-between mb-1">
        <h3 class="font-semibold truncate">${name}</h3>
        <span class="lock" data-lock>L</span>
      </div>
      <div class="ec-kind flex items-center gap-1.5">
        <span class="ec-pill inline-flex items-center gap-1 px-1.5 rounded-full" style="background-color:#336;color:#fff"><i>i</i>${kind}</span>
      </div>
      <div class="ec-tags flex"><span>tag</span></div>
    </div>
  </a>`;

function page(body) {
  return `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>${UTILITIES}</style><style>${ctaCSS}</style><style>${shellCSS}</style></head>
<body>${body}</body></html>`;
}

let browser;
before(async () => { browser = await chromium.launch(); });
after(async () => { await browser?.close(); });

async function open(html, { width = 390, touch = true } = {}) {
  const ctx = await browser.newContext({
    viewport: { width, height: 844 },
    hasTouch: touch,
    isMobile: touch,
  });
  const p = await ctx.newPage();
  await p.setContent(html);
  return { p, close: () => ctx.close() };
}

test('the harness classes are the ones the templates use', () => {
  const app = read('internal/templates/layouts/app.templ');
  for (const c of ['tb-bell', 'tb-menu-notif', 'tb-menu-count', 'tb-esc', 'data-notif-menu', 'id="user-menu-btn"', 'id="topbar-search-trigger"', 'data-tt-tab']) {
    assert.ok(app.includes(c), `app.templ no longer has ${c}`);
  }
  const card = read('internal/plugins/entities/entity_card.templ');
  for (const c of ['ec-card', 'ec-band', 'ec-body', 'ec-head', 'ec-kind', 'ec-pill', 'ec-tags']) {
    assert.ok(card.includes(c), `entity_card.templ no longer has ${c}`);
  }
  const callout = read('internal/plugins/sessions/callout_handler.go');
  assert.ok(callout.includes('cta-long') && callout.includes('cta-short'));
  assert.ok(read('internal/templates/layouts/base.templ').includes('/static/css/phone_shell.css'));
});

test('every header control has a 44 by 44 hit area on touch', async () => {
  const { p, close } = await open(page(header(NAME15)));
  const boxes = await p.$$eval('#app-topbar button', (els) =>
    els.filter((e) => e.offsetParent !== null || getComputedStyle(e).position === 'fixed')
      .map((e) => ({ id: e.id || e.className, r: e.getBoundingClientRect().toJSON() })));
  await close();
  const wanted = boxes.filter((b) => !b.id.includes('tb-menu-notif') && !b.id.includes('name-btn'));
  assert.ok(wanted.length >= 4, `only ${wanted.length} header controls found`);
  for (const b of wanted) {
    assert.ok(b.r.width >= 44 && b.r.height >= 44, `${b.id} is ${b.r.width} by ${b.r.height}`);
  }
});

test('a desktop pointer keeps the header controls as they were', async () => {
  const { p, close } = await open(page(header('Name')), { width: 1280, touch: false });
  const r = await p.$eval('#nav-toggle,#topbar-search-trigger', (e) => e.getBoundingClientRect().toJSON());
  const h = await p.$eval('#app-topbar', (e) => e.getBoundingClientRect().height);
  await close();
  assert.ok(r.height < 44, `a mouse header button grew to ${r.height}`);
  assert.equal(h, 56);
});

test('the campaign name shows whole at 390 and the bell steps into the account menu', async () => {
  const { p, close } = await open(page(header(NAME15)));
  const name = await p.$eval('#name', (e) => ({ s: e.scrollWidth, c: e.clientWidth }));
  const bell = await p.$eval('.tb-bell', (e) => getComputedStyle(e).display);
  const menuRow = await p.$eval('.tb-menu-notif', (e) => getComputedStyle(e).display);
  const overflow = await p.evaluate(() => document.documentElement.scrollWidth - innerWidth);
  await close();
  assert.equal(NAME15.length, 15);
  assert.equal(name.s, name.c, `the name is clipped: ${name.s} wide in ${name.c}`);
  assert.equal(bell, 'none');
  assert.notEqual(menuRow, 'none');
  assert.equal(overflow, 0, 'the header scrolls sideways');
});

test('at 500 wide the bell is back and the menu row is gone', async () => {
  const { p, close } = await open(page(header('Name')), { width: 500 });
  const bell = await p.$eval('.tb-bell', (e) => getComputedStyle(e).display);
  const menuRow = await p.$eval('.tb-menu-notif', (e) => getComputedStyle(e).display);
  await close();
  assert.notEqual(bell, 'none');
  assert.equal(menuRow, 'none');
});

test('the time zone question is one line under 48 tall at 390', async () => {
  const { p, close } = await open(page(tzBanner(true)));
  const m = await p.evaluate(() => {
    const bar = document.querySelector('.cta-bar');
    const msg = document.querySelector('.cta-msg');
    const vis = (s) => { const e = document.querySelector(s); return e && getComputedStyle(e).display !== 'none'; };
    return {
      h: bar.getBoundingClientRect().height,
      clipped: msg.scrollWidth > msg.clientWidth,
      short: vis('.cta-short'), long: vis('.cta-long'), choose: vis('.cta-alt'),
      accept: document.querySelector('[data-cta-tz-accept]').getBoundingClientRect().height,
      close: document.querySelector('.cta-x').getBoundingClientRect().height,
    };
  });
  await close();
  assert.ok(m.h < 48, `the banner is ${m.h} tall`);
  assert.ok(m.short && !m.long, 'the short sentence should replace the long one');
  assert.ok(!m.clipped, 'the short sentence is cut off');
  assert.ok(!m.choose, 'Choose should step aside when the accept word is there');
  assert.ok(m.accept >= 44 && m.close >= 44, `hit areas ${m.accept} and ${m.close}`);
});

test('Choose stays when the browser gave no zone to accept', async () => {
  const { p, close } = await open(page(tzBanner(false)));
  const choose = await p.$eval('.cta-alt', (e) => getComputedStyle(e).display);
  await close();
  assert.notEqual(choose, 'none');
});

test('the wide banner keeps both sentences\' long form and both buttons', async () => {
  const { p, close } = await open(page(tzBanner(true)), { width: 900, touch: false });
  const m = await p.evaluate(() => {
    const d = (s) => getComputedStyle(document.querySelector(s)).display;
    return { long: d('.cta-long'), short: d('.cta-short'), choose: d('.cta-alt') };
  });
  await close();
  assert.notEqual(m.long, 'none');
  assert.equal(m.short, 'none');
  assert.notEqual(m.choose, 'none');
});

test('page lists draw as 44-tall rows under 640 and as cards above', async () => {
  const list = `<div id="entity-list">${row('Brother Quill', 'Character')}${row('The Ashen Reach', 'Location')}</div>`;
  const narrow = await open(page(list));
  const n = await narrow.p.evaluate(() => {
    const a = document.querySelector('.ec-card');
    const q = (s) => a.querySelector(s);
    const cs = getComputedStyle(a);
    const nameR = q('h3').getBoundingClientRect(), kindR = q('.ec-kind').getBoundingClientRect(), lockR = q('[data-lock]').getBoundingClientRect();
    return {
      h: a.getBoundingClientRect().height,
      band: getComputedStyle(q('.ec-band')).display, tags: getComputedStyle(q('.ec-tags')).display,
      shadow: cs.boxShadow, bg: cs.backgroundColor, border: cs.borderBottomWidth,
      order: nameR.left < kindR.left && kindR.left < lockR.left,
      sameLine: Math.abs((nameR.top + nameR.bottom) / 2 - (lockR.top + lockR.bottom) / 2) < 6,
      pillBg: getComputedStyle(q('.ec-pill')).backgroundColor,
    };
  });
  await narrow.close();
  assert.ok(n.h >= 44 && n.h < 46, `a row is ${n.h} tall`);
  assert.equal(n.band, 'none'); assert.equal(n.tags, 'none');
  assert.equal(n.shadow, 'none'); assert.equal(n.bg, 'rgba(0, 0, 0, 0)');
  assert.equal(n.border, '1px');
  assert.ok(n.order && n.sameLine, 'name, kind and lock are not one row in that order');
  assert.equal(n.pillBg, 'rgba(0, 0, 0, 0)', 'the kind should be muted words, not a pill');

  const wide = await open(page(list), { width: 800, touch: false });
  const w = await wide.p.$eval('.ec-card', (a) => ({ band: getComputedStyle(a.querySelector('.ec-band')).display, shadow: getComputedStyle(a).boxShadow }));
  await wide.close();
  assert.notEqual(w.band, 'none');
  assert.notEqual(w.shadow, 'none');
});
