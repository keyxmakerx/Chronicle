// page_headers.spec.mjs — the page headers of app pages, Manage pages and a
// page's own title, with the real built CSS around markup transcribed from the
// templates: the title alone carries the heading look; the sentence under it
// stays in sentence case; a Manage page stays in Material under a kit; on a
// phone no name is squeezed into a column or broken inside a word; the People
// list keeps each member's role control inside the card.
//
// RUN: ./tools/tailwind.sh -i static/css/input.css -o /tmp/chronicle-app.css --minify
//      NODE_PATH="$(npm root -g)" node --test test/e2e/page_headers.spec.mjs
// (APP_CSS overrides the built stylesheet's path.)

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(repoRoot, p), 'utf8');
const appPath = process.env.APP_CSS || '/tmp/chronicle-app.css';
const haveApp = existsSync(appPath);
const appCSS = haveApp ? readFileSync(appPath, 'utf8') : '';
const annalsCSS = read('static/css/page_look_annals.css');
const kitsCSS = read('static/css/style_kits.css');
const skip = haveApp ? false : 'build the CSS first (see RUN)';

const frame = (inner, { kit = '', caps = false } = {}) => `<!doctype html>
<html lang="en" data-cz-page="annals" ${kit ? `data-cz-kit="${kit}"` : ''} ${caps ? 'style="--annals-hcase:uppercase"' : ''}>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><style>${appCSS}</style><style>${annalsCSS}</style><style>${kitsCSS}</style></head>
<body><main id="main-content" style="overflow-y:auto"><div class="max-w-screen-2xl mx-auto px-4 py-6" data-app-frame>${inner}</div></main></body></html>`;

const emptyOpening = (name = 'Calendars') => `
<header class="mb-6" data-app-opening="empty">
  <h1 class="cb-title text-2xl font-bold text-fg">${name}</h1>
  <p class="mt-2 max-w-prose text-sm leading-relaxed text-fg" data-t="sentence">A plain sentence about what lives here.</p>
  <p class="mt-2 text-xs text-fg-muted">Nothing to do yet.</p>
</header>`;

const manageHeader = `
<div class="manage-frame" data-manage-frame data-workshop>
  <header class="manage-head">
    <a href="#" class="manage-eyebrow">Campaign settings</a>
    <h1 class="manage-title cb-title">People</h1>
    <p class="manage-intro" data-t="intro">Who is in the campaign and what they can do.</p>
  </header>
  <p>body</p>
</div>`;

const pageTitle = (name) => `
<div class="flex flex-wrap items-start justify-between gap-x-3 gap-y-1"><div class="min-w-0">
  <div class="flex flex-wrap items-center gap-2">
    <h1 class="cb-title basis-full sm:basis-auto text-xl md:text-3xl font-bold text-fg" data-page-h1>${name}</h1>
    <button class="px-2">*</button><button class="px-2">pin</button><span class="text-xs">GM team only</span><button class="px-2">pen</button>
  </div></div></div>`;

const withPage = async (html, vp, fn) => {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ viewport: vp, deviceScaleFactor: 1 });
    await page.setContent(html);
    await fn(page);
  } finally { await browser.close(); }
};
const css = (page, sel, prop) => page.$eval(sel, (e, p) => getComputedStyle(e)[p], prop);

for (const [label, opts] of [['no kit', {}], ['default capitals', { caps: true }], ['Sharp', { kit: 'sharp' }], ['Sharp and capitals', { kit: 'sharp', caps: true }]]) {
  test(`empty opening, ${label}: the sentence sits under the title in sentence case with no extra rule`, { skip }, async () => {
    await withPage(frame(emptyOpening(), opts), { width: 1000, height: 700 }, async (page) => {
      const t = await page.$eval('h1', (e) => e.getBoundingClientRect());
      const s = await page.$eval('[data-t=sentence]', (e) => e.getBoundingClientRect());
      assert.ok(s.top >= t.bottom - 1, 'the sentence is below the title, not beside it');
      assert.equal(await css(page, '[data-t=sentence]', 'textTransform'), 'none');
      const h = await page.$eval('header', (e) => { const c = getComputedStyle(e); return [c.display, c.borderTopWidth, c.borderBottomWidth]; });
      assert.deepEqual(h, ['block', '0px', '0px'], 'the header carries no rule of its own');
    });
  });
  test(`Manage header, ${label}: intro in sentence case, no extra rule`, { skip }, async () => {
    await withPage(frame(manageHeader, opts), { width: 1000, height: 700 }, async (page) => {
      assert.equal(await css(page, '[data-t=intro]', 'textTransform'), 'none');
      const h = await page.$eval('header', (e) => { const c = getComputedStyle(e); return [c.borderTopWidth, c.borderBottomWidth]; });
      assert.deepEqual(h, ['0px', '0px']);
    });
  });
}

test('Manage page under a kit stays in Material: no kit palette, no kit title rule', { skip }, async () => {
  const bg = (html, kit) => withPage(frame(html, { kit }), { width: 1000, height: 700 }, async (page) => {
    bg.out = {
      bg: await css(page, '#main-content', 'backgroundColor'),
      rule: await page.$eval('h1', (e) => getComputedStyle(e, '::after').borderTopWidth),
      weight: await css(page, 'h1', 'fontWeight'),
    };
  }).then(() => bg.out);
  const material = await bg(manageHeader, '');
  for (const kit of ['sharp', 'ornate', 'flowing', 'lumen', 'sketchbook']) {
    assert.deepEqual(await bg(manageHeader, kit), material, `${kit} dresses a Manage page`);
  }
  const dressed = await bg(emptyOpening(), 'sharp');
  assert.notEqual(dressed.bg, (await bg(emptyOpening(), '')).bg, 'the kit still dresses an app page');
});

const LONG = ['Seraphina Nightwhisper the Unyielding', 'Saltreach Harbour', 'Nightwhisper'];
for (const width of [360, 412]) {
  test(`phone ${width}: page names keep whole words and most of the width`, { skip }, async () => {
    for (const name of LONG) {
      await withPage(frame(pageTitle(name)), { width, height: 800 }, async (page) => {
        const r = await page.$eval('h1', (e) => {
          const box = e.getBoundingClientRect();
          const frameW = e.closest('[data-app-frame]').getBoundingClientRect().width - 32;
          // A word is split when a line ends inside it: compare each word's
          // client rects; a split word has more than one rect on separate lines.
          const walker = document.createTreeWalker(e, NodeFilter.SHOW_TEXT);
          const words = [];
          const text = walker.nextNode();
          const re = /\S+/g; let m;
          while ((m = re.exec(text.data))) {
            const range = document.createRange();
            range.setStart(text, m.index); range.setEnd(text, m.index + m[0].length);
            const tops = new Set([...range.getClientRects()].map((q) => Math.round(q.top)));
            if (tops.size > 1) words.push(m[0]);
          }
          return { w: box.width, frameW, split: words };
        });
        assert.ok(r.w >= 0.6 * r.frameW, `"${name}" is ${r.w}px of ${r.frameW}px`);
        assert.deepEqual(r.split, [], `"${name}" breaks inside a word`);
      });
    }
  });
  test(`phone ${width}: app opening names keep whole words`, { skip }, async () => {
    for (const name of ['Calendars', 'Maps', 'Timelines', 'Armory', 'Shops', 'Player Characters']) {
      await withPage(frame(emptyOpening(name), { caps: true }), { width, height: 800 }, async (page) => {
        const r = await page.$eval('h1', (e) => {
          const text = e.firstChild; const range = document.createRange();
          range.setStart(text, 0); range.setEnd(text, text.data.length);
          return { w: e.getBoundingClientRect().width, lines: new Set([...range.getClientRects()].map((q) => Math.round(q.top))).size, words: text.data.split(' ').length };
        });
        assert.ok(r.lines <= r.words, `"${name}" breaks inside a word (${r.lines} lines)`);
      });
    }
  });
}

test('phone 360: "Create hero" wraps under the sentence, not over it', { skip }, async () => {
  assert.ok(read('internal/templates/layouts/app_opening.templ').includes('min-w-0 flex-[1_1_16rem] flex-wrap items-center gap-x-4'), 'the harness row is the template row');
  const html = frame(`<header class="mb-6 flex flex-wrap items-center gap-x-4 gap-y-2" data-app-opening="full">
    <h1 class="cb-title text-2xl font-bold text-fg">Characters</h1>
    <div class="flex min-w-0 flex-[1_1_16rem] flex-wrap items-center gap-x-4 gap-y-2 text-sm text-fg-secondary"><span data-t="s">The cast of Ashen, and everyone who has a sheet in the campaign.</span></div>
    <div class="ml-auto"><a href="#" class="btn-primary text-sm gap-2" data-t="a">Create hero</a></div></header>`, { caps: true });
  await withPage(html, { width: 360, height: 800 }, async (page) => {
    const s = await page.$eval('[data-t=s]', (e) => e.getBoundingClientRect());
    const a = await page.$eval('[data-t=a]', (e) => e.getBoundingClientRect());
    const overlap = !(a.right <= s.left || a.left >= s.right || a.bottom <= s.top || a.top >= s.bottom);
    assert.equal(overlap, false, 'the button sits on the sentence');
    const t = await page.$eval('h1', (e) => { const r = document.createRange(); r.selectNodeContents(e.firstChild); return new Set([...r.getClientRects()].map((q) => Math.round(q.top))).size; });
    assert.equal(t, 1, 'the title is squeezed onto several lines');
  });
});

test('phone: All pages toolbar fits at 360, 390 and 412; New page keeps its icon', { skip }, async () => {
  const idx = read('internal/plugins/entities/index.templ');
  const start = idx.indexOf('<div class="flex w-full items-center gap-3 sm:w-auto">');
  assert.ok(start > 0, 'the toolbar row is full width on a phone');
  const row = `<div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 mb-6">
    <div><h1 class="text-2xl font-bold">All pages</h1></div>
    <div class="flex w-full items-center gap-3 sm:w-auto">
      <form class="relative min-w-0 flex-1 sm:flex-none" role="search"><input type="search" class="input pl-9 pr-3 py-2 w-full sm:w-56"/></form>
      <div class="flex shrink-0 items-center gap-1 bg-surface-alt rounded-md p-0.5"><button class="px-2 py-1">g</button><button class="px-2 py-1">t</button></div>
      <a href="#" class="btn-primary shrink-0" aria-label="New page" data-t="new"><i class="fa-solid fa-plus text-xs"></i> <span class="max-[399px]:hidden">New page</span></a>
    </div></div>`;
  for (const width of [360, 390, 412]) {
    await withPage(frame(row), { width, height: 800 }, async (page) => {
      const r = await page.$eval('[data-t=new]', (e) => e.getBoundingClientRect());
      assert.ok(r.right <= width, `New page ends at ${r.right} of ${width}`);
      assert.ok(await page.$eval('#main-content', (e) => e.scrollWidth <= e.clientWidth), 'the page slides sideways');
    });
  }
});

test('phone 360: category toolbar shows search, sort and the view switch without sliding', { skip }, async () => {
  const html = frame(`<div class="flex flex-wrap items-center justify-between gap-x-2 gap-y-2 mb-4 border-b border-edge pb-3">
    <div class="contents sm:flex sm:items-center sm:gap-2">
      <div class="relative w-full sm:w-auto"><input class="input pl-8 pr-3 py-1.5 w-full sm:w-48 text-sm" data-t="q"/></div>
      <select class="input py-1.5 text-sm w-auto" data-t="sort"><option>Recently Created</option></select>
    </div>
    <div class="flex items-center gap-1 bg-surface-alt rounded-md p-0.5" data-t="sw"><button class="px-2 py-1">g</button><button class="px-2 py-1">t</button><button class="px-2 py-1">r</button></div></div>`);
  await withPage(html, { width: 360, height: 800 }, async (page) => {
    const box = (t) => page.$eval(`[data-t=${t}]`, (e) => e.getBoundingClientRect());
    const q = await box('q'), s = await box('sort'), w = await box('sw');
    assert.ok(q.width > 250, 'search takes its own full line');
    assert.ok(s.right <= 360 && w.right <= 360, 'sort and the switch are on screen');
    assert.ok(s.top >= q.bottom && Math.abs(s.top - w.top) < 40, 'sort and the switch share the second line');
  });
});

test('phone 360: each member is a block and the role control is inside the card', { skip }, async () => {
  const tmpl = read('internal/plugins/campaigns/members.templ');
  const cls = (re) => { const m = tmpl.match(re); assert.ok(m, String(re)); return m[1]; };
  const tableC = cls(/<table class="([^"]*max-sm:block[^"]*)"/);
  const theadC = cls(/<thead class="([^"]*)"/);
  const tbodyC = cls(/<tbody class="([^"]*max-sm:block[^"]*)"/);
  const trC = cls(/<tr class="(max-sm:flex[^"]*)"/);
  const nameC = cls(/<td class="(px-3 sm:px-6 py-4 max-sm:basis-full[^"]*)"/);
  const roleC = cls(/<td class="(px-3 sm:px-6 py-4 max-sm:p-0)"/);
  const actC = cls(/<td class="(px-3 sm:px-6 py-4 text-right max-sm:ml-auto[^"]*)"/);
  const innerC = cls(/<div class="(flex flex-wrap items-center justify-end[^"]*)">\s*\/\/ Role change/);
  const row = `<tr class="${trC}"><td class="${nameC}"><div class="flex items-center gap-3"><span class="ag-av s32">MV</span><div class="min-w-0"><div class="text-sm font-medium">Mara Vell</div><div class="text-xs break-all sm:break-normal">mara.vell.long.address@example.invalid</div></div></div></td>
    <td class="hidden sm:table-cell px-6 py-4">None</td>
    <td class="${roleC}"><span class="inline-block px-2 py-1 text-xs rounded-full">Player</span></td>
    <td class="hidden sm:table-cell px-6 py-4">Jan 1</td>
    <td class="${actC}"><div class="${innerC}"><form class="inline-flex items-center gap-1"><select class="input text-xs py-1 w-auto" data-t="role"><option>Co-GM</option></select><button class="text-xs">Save</button></form><form><button class="text-xs" data-t="rm">Remove</button></form></div></td></tr>`;
  const html = frame(`<div class="card overflow-x-auto" data-t="card"><table class="${tableC}"><thead class="${theadC}"><tr><th>Name</th></tr></thead><tbody class="${tbodyC}">${row}${row}</tbody></table></div>`);
  await withPage(html, { width: 360, height: 800 }, async (page) => {
    const card = await page.$eval('[data-t=card]', (e) => { const r = e.getBoundingClientRect(); return { l: r.left, r: r.right, sw: e.scrollWidth, cw: e.clientWidth }; });
    for (const t of ['role', 'rm']) {
      const b = await page.$eval(`[data-t=${t}]`, (e) => { const r = e.getBoundingClientRect(); return { l: r.left, r: r.right }; });
      assert.ok(b.l >= card.l && b.r <= card.r, `${t} is outside the card (${b.r} > ${card.r})`);
    }
    assert.ok(card.sw <= card.cw, 'the card slides sideways');
  });
});
