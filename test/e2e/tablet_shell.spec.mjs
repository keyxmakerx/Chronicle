// tablet_shell.spec.mjs — the app shell between a phone and a desktop: when
// the sidebar is a drawer, when the menu button shows, when a row of blocks
// folds into one column, and how a Coming up row sits in a narrow card.
//
// A harness page, as in phone_shell.spec.mjs: the real phone_shell.css, the
// real sidebar-state, Calm and Off and Coming up rules cut from input.css, and
// the real sidebar_nav.js, around the shell transcribed from layouts/app.templ
// and layouts/nav.templ and a row from app/coming_up.templ. The first test
// pins that transcription to the templates.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/tablet_shell.spec.mjs

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
const inputCSS = read('static/css/input.css');
const appTempl = read('internal/templates/layouts/app.templ');
const navJS = read('static/js/sidebar_nav.js');

// The layout's own word on when the sidebar is a drawer.
const DRAWER_QUERY = (appTempl.match(/const navDrawerQuery = "([^"]+)"/) || [])[1];
assert.ok(DRAWER_QUERY, 'navDrawerQuery not found in app.templ');

// cut returns the rules between two marks in input.css, which load here as
// plain CSS (the app ships them inside Tailwind's component layer).
function cut(from, to) {
  const a = inputCSS.indexOf(from);
  const b = inputCSS.indexOf(to, a);
  assert.ok(a > 0, `"${from.trim()}" moved in input.css`);
  assert.ok(b > a, `"${to.trim()}" no longer follows "${from.trim()}" in input.css`);
  return inputCSS.slice(a, b);
}
const layersCSS = cut(':root {\n  --z-raised:', '}\n') + '}\n';
const sidebarCSS = cut('  .sidebar-mobile-open {', '  /* Hide text labels, section headers');
const motionCSS = cut('html[data-cz-reduce] *:not(.czp *),', '\n\n/* ======');
const comingUpCSS = cut('.cu-card { container:', '/* Header widgets (app.templ');

const UTILITIES = `
  *, ::before, ::after { box-sizing: border-box; }
  html { font-size: 16px; } html, body { margin: 0; font-family: sans-serif; line-height: 1.4; } body { font-size: 14px; }
  :root { --dur-large: 280ms; }
  .flex { display: flex; } .flex-col { flex-direction: column; } .flex-1 { flex: 1 1 0%; } .block { display: block; }
  .items-center { align-items: center; } .justify-center { justify-content: center; }
  .shrink-0 { flex-shrink: 0; } .min-w-0 { min-width: 0; } .relative { position: relative; }
  .h-screen { height: 100vh; } .overflow-hidden { overflow: hidden; } .h-14 { height: 3.5rem; }
  .gap-2 { gap: .5rem; } .gap-3 { gap: .75rem; } .gap-4 { gap: 1rem; }
  .px-3 { padding-inline: .75rem; } .px-4 { padding-inline: 1rem; } .py-2 { padding-block: .5rem; } .py-1 { padding-block: .25rem; }
  .py-1\\.5 { padding-block: .375rem; }
  .fixed { position: fixed; } .inset-y-0 { top: 0; bottom: 0; } .left-0 { left: 0; } .z-\\[var\\(--z-drawer\\)\\] { z-index: var(--z-drawer); }
  .w-64 { width: 16rem; } .w-10 { width: 2.5rem; } .h-10 { height: 2.5rem; }
  .-translate-x-full { transform: translateX(-100%); }
  .grid { display: grid; } .grid-cols-1 { grid-template-columns: repeat(1, minmax(0, 1fr)); }
  .text-sm { font-size: .875rem; line-height: 1.25rem; } .text-xs { font-size: .75rem; line-height: 1rem; }
  .font-medium { font-weight: 500; } .break-words { overflow-wrap: break-word; } .rounded-lg { border-radius: .5rem; }
  .leading-none { line-height: 1; }
  .card { border: 1px solid #e5e7eb; border-radius: .5rem; background: #fff; padding: 1rem; }
  .btn-icon { padding: .5rem; border: 0; background: none; border-radius: .5rem; }
  #sidebar { background: #1f2937; color: #fff; }
  .nav-brand { height: 56px; flex: none; display: flex; align-items: center; gap: 4px; padding: 0 8px 0 14px; }
  .nav-brand-link { flex: 1; min-width: 0; }
  .nav-brand > :not(.nav-brand-bg) { position: relative; z-index: 1; }
  .nav-ibtn { width: 32px; height: 32px; flex: none; display: grid; place-items: center; border: 0; padding: 0; background: none; color: inherit; }
  .nav-ibtn-sm { width: 24px; height: 24px; }
  .nav-x-close { display: none; }
  @media (max-width: 767px) { .nav-x-close { display: grid; } .nav-sb .sidebar-pin-btn { display: none; } }
  @media (min-width: 768px) {
    .md\\:translate-x-0 { transform: translateX(0); } .md\\:static { position: static; } .md\\:z-auto { z-index: auto; }
    .md\\:shrink-0 { flex-shrink: 0; } .md\\:hidden { display: none; }
    .md\\:grid-cols-12 { grid-template-columns: repeat(12, minmax(0, 1fr)); }
    .md\\:col-span-4 { grid-column: span 4 / span 4; } .md\\:col-span-8 { grid-column: span 8 / span 8; }
  }
`;

function shell(sidebarState = '') {
  return `
  <div class="flex h-screen overflow-hidden" data-nav-drawer-query="${DRAWER_QUERY}">
    <aside id="sidebar" class="nav-sb fixed inset-y-0 left-0 z-[var(--z-drawer)] w-64 flex flex-col -translate-x-full md:translate-x-0 md:static md:z-auto md:shrink-0 ${sidebarState}">
      <div class="nav-brand">
        <a class="nav-brand-link" href="#home">The Sunken Crown</a>
        <button type="button" class="nav-ibtn nav-x-close" aria-label="Close navigation">x</button>
      </div>
      <nav id="sidebar-nav"><ul id="sidebar-nav-list">
        <li><a class="nav-row" href="#characters">Characters</a></li>
        <li><a class="nav-row" href="#places">Places</a></li>
      </ul></nav>
      <div><button type="button" class="nav-ibtn nav-ibtn-sm sidebar-pin-btn" aria-label="Pin">p</button></div>
    </aside>
    <div id="app-main" class="relative flex-1 flex flex-col overflow-hidden min-w-0">
      <header id="app-topbar" class="h-14 flex items-center px-3 gap-2 relative">
        <button id="nav-toggle" type="button" class="btn-icon shrink-0 md:hidden" aria-label="Open navigation">m</button>
        <button id="topbar-search-trigger" type="button" class="btn-icon">s</button>
      </header>
      <main id="main-content" style="padding:24px">
        <div class="block-row grid grid-cols-1 md:grid-cols-12 gap-4" id="row">
          <div class="md:col-span-4 min-w-0" id="third">Coming up</div>
          <div class="md:col-span-8 min-w-0" id="rest">Welcome</div>
        </div>
      </main>
    </div>
  </div>`;
}

// One Coming up row in a card of the given width.
function comingUp(width, title) {
  return `
  <div style="width:${width}px;margin:8px"><div class="cu-card card overflow-hidden" id="card">
    <a href="#night" class="cu-row flex items-center gap-3 px-4 py-2" id="cu">
      <span class="shrink-0 w-10 h-10 rounded-lg flex flex-col items-center justify-center leading-none" id="tile">THU</span>
      <span class="cu-words flex-1 min-w-0" id="words">
        <span class="block text-sm font-medium break-words" id="title">${title}</span>
        <span class="block text-xs">Thursday at 7 pm</span>
      </span>
      <span class="cu-end shrink-0 text-xs font-medium px-3 py-1.5 rounded-lg" id="end">Answer</span>
    </a>
  </div></div>`;
}

function page(body, attrs = '') {
  return `<!doctype html><html ${attrs}><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>${layersCSS}</style><style>${UTILITIES}</style><style>${sidebarCSS}</style><style>${comingUpCSS}</style><style>${motionCSS}</style><style>${shellCSS}</style></head>
<body>${body}</body></html>`;
}

let browser;
before(async () => { browser = await chromium.launch(); });
after(async () => { await browser?.close(); });

// still: the device asks for reduced motion, so a state is measured as it
// lands, not partway through the drawer's slide.
async function open(html, { width, touch, still = true, script = false }) {
  const ctx = await browser.newContext({
    viewport: { width, height: 900 },
    hasTouch: touch,
    isMobile: touch,
    reducedMotion: still ? 'reduce' : 'no-preference',
  });
  const p = await ctx.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  await p.setContent(html);
  if (script) await p.addScriptTag({ content: navJS });
  return { p, errors, close: () => ctx.close() };
}

const how = (width, touch) => `at ${width} wide by ${touch ? 'touch' : 'mouse'}`;

function shape(p) {
  return p.evaluate((q) => {
    const sb = document.getElementById('sidebar');
    const r = sb.getBoundingClientRect();
    const cs = getComputedStyle(sb);
    const shown = (s) => { const e = document.querySelector(s); return getComputedStyle(e).display !== 'none'; };
    return {
      says: matchMedia(q).matches,
      left: r.left, right: r.right, width: r.width, position: cs.position, visibility: cs.visibility,
      menu: shown('#nav-toggle'), close: shown('.nav-x-close'), pin: shown('.sidebar-pin-btn'),
      main: document.getElementById('app-main').getBoundingClientRect().toJSON(),
      sideways: document.documentElement.scrollWidth - innerWidth,
    };
  }, DRAWER_QUERY);
}

test('the harness classes are the ones the templates use', () => {
  for (const c of ['data-nav-drawer-query={ navDrawerQuery }', 'id="sidebar"', 'fixed inset-y-0 left-0 z-[var(--z-drawer)] w-64',
    '-translate-x-full md:translate-x-0', 'md:static md:z-auto md:shrink-0', "'sidebar-mobile-open shadow-elev-dragged'",
    "'sidebar-unpinned'", "'sidebar-collapsed'", "'sidebar-fully-hidden'", 'id="app-main"', 'id="nav-toggle"',
    'x-show="sidebarOpen && navDrawer"']) {
    assert.ok(appTempl.includes(c), `app.templ no longer has ${c}`);
  }
  const menu = appTempl.slice(appTempl.indexOf('id="nav-toggle"')).slice(0, 400);
  assert.ok(/class="[^"]*\bbtn-icon\b[^"]*\bmd:hidden"/.test(menu), 'the menu button is no longer a btn-icon hidden from md up');
  const nav = read('internal/templates/layouts/nav.templ');
  for (const c of ['class="nav-ibtn nav-x-close"', 'sidebar-pin-btn', 'id="sidebar-nav-list"']) {
    assert.ok(nav.includes(c), `nav.templ no longer has ${c}`);
  }
  for (const c of ['.nav-x-close { display: none; }', '.nav-brand > :not(.nav-brand-bg) { position: relative; z-index: 1; }']) {
    assert.ok(inputCSS.includes(c), `input.css no longer has ${c}`);
  }
  // Every row of blocks carries the class the fold rules hang on.
  const rows = [
    ['internal/plugins/campaigns/show.templ', 1], ['internal/plugins/entities/show.templ', 2],
    ['internal/plugins/entities/category_dashboard.templ', 1],
  ];
  for (const [file, want] of rows) {
    const src = read(file);
    assert.equal(src.split('md:grid-cols-12').length - 1, want, `${file} has a different number of block rows`);
    assert.equal(src.split('"block-row grid grid-cols-1 md:grid-cols-12 ').length - 1, want, `${file} has a block row without block-row`);
  }
  const cu = read('internal/app/coming_up.templ');
  for (const c of ['"cu-card card ', '"cu-row flex items-center gap-3 px-4 py-2', '"cu-words flex-1 min-w-0"', '"cu-end shrink-0 ',
    'break-words']) {
    assert.ok(cu.includes(c), `coming_up.templ no longer has ${c}`);
  }
  assert.equal(cu.split('"cu-end shrink-0 ').length - 1, 3, 'every word at the end of a Coming up row should be cu-end');
  assert.ok(!cu.includes('truncate'), 'a Coming up name is cut short again');
  assert.ok(read('internal/templates/layouts/base.templ').includes("querySelector('[data-nav-drawer-query]')"),
    'the script that sets the sidebar before first paint no longer asks whether it is a drawer');
  assert.ok(navJS.includes("querySelector('[data-nav-drawer-query]')"), 'sidebar_nav.js no longer reads the layout\'s drawer query');
});

test('the sidebar is drawn as a drawer exactly when the layout says it is one, and only then is there a menu button', async () => {
  const cases = [
    [390, true, true], [700, false, true], [767, false, true], [767, true, true],
    [768, false, false], [768, true, true], [820, true, true], [820, false, false],
    [1023, true, true], [1023, false, false], [1024, true, false], [1024, false, false],
    [1180, true, false], [1280, false, false],
  ];
  for (const [width, touch, drawer] of cases) {
    const { p, close } = await open(page(shell()), { width, touch });
    const m = await shape(p);
    await close();
    const at = how(width, touch);
    assert.equal(m.says, drawer, `the layout's query is ${m.says} ${at}`);
    assert.equal(m.right <= 0, drawer, `the sidebar's right edge is at ${m.right} ${at}`);
    assert.equal(m.menu, drawer, `the menu button is ${m.menu ? 'shown' : 'hidden'} ${at}`);
    assert.equal(m.width, 256, `the sidebar is ${m.width} wide ${at}`);
    if (drawer) {
      assert.equal(m.position, 'fixed', at);
      assert.equal(m.main.left, 0, `the page starts at ${m.main.left} ${at}`);
      assert.equal(m.main.width, width, `the page is ${m.main.width} wide ${at}`);
    } else {
      assert.equal(m.position, 'static', at);
      assert.equal(m.main.left, 256, `the page starts at ${m.main.left} ${at}`);
    }
    assert.equal(m.sideways, 0, `the page scrolls sideways ${at}`);
  }
});

test('on an upright tablet the sidebar comes over the page, with a way to close it, and leaves again', async () => {
  for (const width of [768, 820, 1000]) {
    const { p, close } = await open(page(shell()), { width, touch: true });
    const menu = await p.$eval('#nav-toggle', (e) => e.getBoundingClientRect().toJSON());
    assert.ok(menu.width >= 44 && menu.height >= 44, `the menu button is ${menu.width} by ${menu.height} at ${width}`);
    await p.$eval('#sidebar', (e) => e.classList.add('sidebar-mobile-open'));
    const o = await shape(p);
    assert.equal(o.left, 0, `the open drawer starts at ${o.left} at ${width}`);
    assert.equal(o.width, 256);
    assert.equal(o.main.width, width, 'the page is pushed aside by the drawer');
    assert.ok(o.close, 'the open drawer has no close control');
    assert.ok(!o.pin, 'the drawer offers to pin itself');
    // The close control is drawn small; what answers a tap is 44 by 44.
    const tap = await p.$eval('.nav-x-close', (b) => {
      const s = getComputedStyle(b, '::after');
      const r = b.getBoundingClientRect();
      const hit = (x, y) => document.elementFromPoint(x, y) === b;
      return {
        w: b.clientWidth - parseFloat(s.left) - parseFloat(s.right),
        h: b.clientHeight - parseFloat(s.top) - parseFloat(s.bottom),
        around: hit(r.left + r.width / 2, r.top - 5) && hit(r.left + r.width / 2, r.bottom + 5) && hit(r.right + 5, r.top + r.height / 2),
        drawn: r.height,
      };
    });
    assert.ok(tap.w >= 44 && tap.h >= 44, `the close control answers a tap over ${tap.w} by ${tap.h}`);
    assert.ok(tap.around, 'a tap just outside the close control misses it');
    assert.equal(tap.drawn, 32, 'the close control changed size');
    await p.$eval('#sidebar', (e) => e.classList.remove('sidebar-mobile-open'));
    const c = await shape(p);
    await close();
    assert.ok(c.right <= 0, `the closed drawer still shows, its right edge at ${c.right}`);
  }
});

test('a drawer is never folded to icons or hidden, whatever was remembered from a wider screen', async () => {
  for (const state of ['sidebar-unpinned sidebar-collapsed', 'sidebar-fully-hidden']) {
    const { p, close } = await open(page(shell(state)), { width: 820, touch: true });
    const rest = await shape(p);
    await p.$eval('#sidebar', (e) => e.classList.add('sidebar-mobile-open'));
    const o = await shape(p);
    const kids = await p.$$eval('#sidebar > *', (els) => els.map((e) => e.getBoundingClientRect().left));
    await close();
    assert.ok(rest.right <= 0, `with ${state} the closed drawer shows`);
    assert.equal(o.left, 0, `with ${state} the open drawer starts at ${o.left}`);
    assert.equal(o.width, 256, `with ${state} the open drawer is ${o.width} wide`);
    assert.equal(o.visibility, 'visible', `with ${state} the open drawer is not visible`);
    for (const left of kids) assert.ok(left >= 0, `with ${state} what is in the drawer is off the page at ${left}`);
  }
});

test('the drawer slides in 280ms or less, and not at all under Calm, Off or the device\'s reduce setting', async () => {
  const motion = async (attrs, still) => {
    const { p, close } = await open(page(shell(), attrs), { width: 820, touch: true, still });
    const m = await p.$eval('#sidebar', (e) => {
      const cs = getComputedStyle(e);
      const props = cs.transitionProperty.split(',').map((s) => s.trim());
      const durs = cs.transitionDuration.split(',').map((s) => parseFloat(s) * (s.trim().endsWith('ms') ? 1 : 1000));
      const i = props.indexOf('transform') >= 0 ? props.indexOf('transform') : props.indexOf('all');
      return { slides: i >= 0 && durs[i % durs.length] > 1, ms: i >= 0 ? durs[i % durs.length] : 0 };
    });
    await close();
    return m;
  };
  const plain = await motion('', false);
  assert.ok(plain.slides, 'the drawer does not slide');
  assert.ok(plain.ms <= 280, `the drawer takes ${plain.ms}ms`);
  assert.ok(!(await motion('', true)).slides, 'the drawer slides although the device asks for reduced motion');
  assert.ok(!(await motion('data-view-motion="calm"', false)).slides, 'the drawer slides under Calm');
  assert.ok(!(await motion('data-cz-reduce', false)).slides, 'the drawer slides although the owner switched motion down');
  assert.ok(!(await motion('data-motion="off"', false)).slides, 'the drawer slides under Off');
});

test('shut, the tablet drawer is out of reach of the keyboard and screen readers; open, it is a dialog', async () => {
  const state = (p) => p.$eval('#sidebar', (e) => ({ inert: e.hasAttribute('inert'), role: e.getAttribute('role'), modal: e.getAttribute('aria-modal') }));
  const tablet = await open(page(shell()), { width: 820, touch: true, script: true });
  assert.deepEqual(await state(tablet.p), { inert: true, role: null, modal: null });
  await tablet.p.$eval('#sidebar', (e) => e.classList.add('sidebar-mobile-open'));
  await tablet.p.waitForFunction(() => document.getElementById('sidebar').getAttribute('role') === 'dialog');
  assert.deepEqual(await state(tablet.p), { inert: false, role: 'dialog', modal: 'true' });
  assert.equal(await tablet.p.evaluate(() => document.activeElement && document.activeElement.closest('#sidebar') !== null), true,
    'focus did not move into the open drawer');
  await tablet.p.$eval('#sidebar', (e) => e.classList.remove('sidebar-mobile-open'));
  await tablet.p.waitForFunction(() => document.getElementById('sidebar').hasAttribute('inert'));
  assert.deepEqual(tablet.errors, []);
  await tablet.close();

  // Docked, the sidebar is ordinary page content, by mouse or by touch.
  for (const [width, touch] of [[820, false], [1024, true], [1280, false]]) {
    const docked = await open(page(shell()), { width, touch, script: true });
    assert.deepEqual(await state(docked.p), { inert: false, role: null, modal: null }, how(width, touch));
    assert.deepEqual(docked.errors, []);
    await docked.close();
  }
});

test('a row of blocks is one column until the page area can hold them side by side', async () => {
  const open256 = '';
  const folded = 'sidebar-unpinned sidebar-collapsed';
  const cases = [
    // width, touch, the sidebar's state, side by side?
    [700, false, open256, false],
    [768, true, open256, false], [820, true, open256, false], [879, true, open256, false],
    [880, true, open256, true], [1000, true, open256, true],
    [820, false, open256, false], [879, false, 'sidebar-fully-hidden', false],
    [880, false, open256, false], [1000, false, open256, false], [1135, false, open256, false],
    [1136, false, open256, true], [1280, false, open256, true],
    [1000, false, folded, true], [1000, false, 'sidebar-fully-hidden', true],
    // An unpinned sidebar opened by resting the pointer on it: the page is not re-laid for that moment.
    [1000, false, 'sidebar-unpinned', true],
    [1024, true, open256, false], [1100, true, open256, false], [1136, true, open256, true], [1180, true, open256, true],
    [1100, true, folded, true],
  ];
  for (const [width, touch, state, beside] of cases) {
    const { p, close } = await open(page(shell(state)), { width, touch });
    const m = await p.evaluate(() => {
      const a = document.getElementById('third').getBoundingClientRect();
      const b = document.getElementById('rest').getBoundingClientRect();
      return { beside: b.left >= a.right && b.top < a.bottom, stacked: b.top >= a.bottom, third: a.width, rest: b.width,
        sideways: document.documentElement.scrollWidth - innerWidth };
    });
    await close();
    const at = `${how(width, touch)}${state ? ' with ' + state : ''}`;
    assert.equal(m.beside, beside, `the blocks are ${m.beside ? 'side by side' : 'not side by side'} ${at}`);
    assert.equal(m.stacked, !beside, `the blocks are ${m.stacked ? 'stacked' : 'not stacked'} ${at}`);
    // Side by side, the narrow block still has room for a line of words.
    if (beside && state !== 'sidebar-unpinned') assert.ok(m.third >= 250, `the third-width block is ${m.third} wide ${at}`);
    if (!beside) assert.equal(m.third, m.rest, `the stacked blocks differ in width ${at}`);
    assert.equal(m.sideways, 0, `the page scrolls sideways ${at}`);
  }
});

function comingUpShape(p) {
  return p.evaluate(() => {
    const r = (id) => document.getElementById(id).getBoundingClientRect();
    const title = document.getElementById('title');
    const cs = getComputedStyle(title);
    const words = r('words'), end = r('end'), card = document.getElementById('card');
    return {
      under: end.top >= words.bottom - 1, beside: end.left >= words.right - 1 && end.top < words.bottom && end.bottom > words.top,
      inLine: Math.abs(end.left - words.left) < 1,
      cut: title.scrollWidth > title.clientWidth || cs.textOverflow === 'ellipsis' || cs.whiteSpace === 'nowrap',
      lines: Math.round(title.getBoundingClientRect().height / parseFloat(cs.lineHeight)),
      wordsW: words.width, rowW: (() => { const row = document.getElementById('cu'), c = getComputedStyle(row); return row.clientWidth - parseFloat(c.paddingLeft) - parseFloat(c.paddingRight); })(),
      spills: card.scrollWidth > card.clientWidth, endRight: end.right, cardRight: card.getBoundingClientRect().right,
    };
  });
}

test('a Coming up name is shown whole: it wraps, and in a narrow card the button drops under the words', async () => {
  const NAME = 'Session 3: Under the Eel and the long way home';
  // A third of a tablet's page, a phone's card, and the widest card that is still narrow.
  for (const width of [264, 334, 402]) {
    const narrow = await open(page(comingUp(width, NAME)), { width: 820, touch: true });
    const n = await comingUpShape(narrow.p);
    await narrow.close();
    assert.ok(!n.cut, `the name is cut short in a ${width} wide card`);
    assert.ok(n.under && n.inLine, `in a ${width} wide card the button should sit under the words, in line with them`);
    assert.ok(Math.abs(n.wordsW - (n.rowW - 52)) < 1, `in a ${width} wide card the words have ${n.wordsW} of the row's ${n.rowW}`);
    assert.ok(!n.spills && n.endRight <= n.cardRight, `the row spills out of a ${width} wide card`);
  }

  // Beside the words only where the words keep about 200.
  for (const width of [404, 520]) {
    const wide = await open(page(comingUp(width, NAME)), { width: 820, touch: true });
    const w = await comingUpShape(wide.p);
    await wide.close();
    assert.ok(!w.cut, `the name is cut short in a ${width} wide card`);
    assert.ok(w.beside, `in a ${width} wide card the button should stay beside the words`);
    assert.ok(w.wordsW >= 190, `beside the button the words have only ${w.wordsW} in a ${width} wide card`);
  }
  const long = await open(page(comingUp(264, NAME)), { width: 820, touch: true });
  assert.ok((await comingUpShape(long.p)).lines >= 2, 'the long name did not wrap');
  await long.close();

  // One unbroken word longer than the card still stays inside it.
  for (const width of [264, 520]) {
    const word = await open(page(comingUp(width, 'Thelongestnightnameanyonehasevertypedwithoutaspaceinitatall')), { width: 820, touch: true });
    const l = await comingUpShape(word.p);
    await word.close();
    assert.ok(!l.spills && l.endRight <= l.cardRight, `an unbroken name spills out of a ${width} wide card`);
    assert.ok(!l.cut, `an unbroken name is cut short in a ${width} wide card`);
  }
});
