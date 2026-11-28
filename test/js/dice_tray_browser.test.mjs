// dice_tray_browser.test.mjs — the Dice tray card's shared mat, against the real table.js (which
// hands each live roll to the card), the real tray scripts and the real dice engine, in a real
// Chromium. A roll is delivered the way the Table's socket delivers it (table.receive(view, true)).
//
// What it pins: a public roll from another member lands on the mat with their name tag; a private,
// Director-only or blind roll never does, however it reaches this browser; Others' dice Off hides
// other members' dice and keeps your own; the level switch changes the drawing (3D dice at Detailed,
// flat dice at the others); and with motion Off the result shows at once and nothing is thrown.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally, and fails under CI,
// when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, normalize, extname } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const js = (p) => readFileSync(join(root, 'internal/plugins/rolls/static/js', p));
const MIME = { '.js': 'text/javascript', '.css': 'text/css' };

const page = `<!doctype html><html><head><meta charset="utf-8">
<link rel="stylesheet" href="/static/css/dice_tray.css"><link rel="stylesheet" href="/static/css/paper.css"><style>body{margin:0}#card{width:420px}</style></head><body>
<header id="app-topbar"><div class="tt-host" data-widget="table" data-decks="false" data-campaign-id="c1" data-user-id="u1"><button type="button" class="tt-tab" data-tt-tab>Table<span class="hdr-n" data-tt-count hidden></span></button></div></header>
<div class="dms-dock"><div id="card" data-widget="dice-tray" data-campaign-id="c1"></div></div>
<script>
window.__sets = [];
window.__posts = [];
window.__thrown = 0;
window.Chronicle = (function () {
  var defs = {};
  function mountOne(el) { var d = defs[el.getAttribute('data-widget')]; if (d && !el.__mounted) { el.__mounted = true; d.init(el); } }
  return {
    tableDice: new Proxy({}, { get: function (_, k) { return k === 'throwInto' ? function () { window.__thrown++; return Promise.resolve(); } : k === 'motion' ? function () { return 'off'; } : function () { return ''; }; } }),
    escapeHtml: function (v) { return String(v).replace(/[&<>"']/g, function (c) { return '&#' + c.charCodeAt(0) + ';'; }); },
    motionMs: function (n, f) { return f; }, reducedMotion: function () { return false; },
    apiFetch: function (url, opts) {
      if (opts && opts.method === 'POST') {
        window.__posts.push(opts.body);
        return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(window.__reply); } });
      }
      return Promise.resolve({ ok: false, status: 404, json: function () { return Promise.resolve(null); }, text: function () { return Promise.resolve(''); } });
    },
    register: function (n, d) { defs[n] = d; document.querySelectorAll('[data-widget="' + n + '"]').forEach(mountOne); },
    mountWidgets: function (r) { r.querySelectorAll('[data-widget]').forEach(mountOne); }
  };
})();
document.querySelector('.dms-dock')._dmsTray = { get: function () { return {}; }, set: function (k, v) { window.__sets.push([k, v]); } };
</script>
<script src="/js/roll_dice.js"></script><script src="/js/table.js"></script>
<script src="/js/dice_tray_art.js"></script><script src="/js/dice_tray_stage.js"></script><script src="/js/dice_tray_core.js"></script><script src="/js/dice_tray.js"></script>
</body></html>`;

// A roll as the roll feed delivers it.
const roll = (o) => Object.assign({
  id: 'r' + Math.random().toString(36).slice(2), origin: 'chronicle', source: 'table', rollerId: 'u2', rollerName: 'Tyne',
  label: '2d10+1', formula: '2d10+1', dice: [{ sides: 10, value: 4 }, { sides: 10, value: 6 }], modifier: 1, total: 11, mode: 'public',
  rollerDice: { set: 'jewelled', colour: 'onyx', critSucc: '', critFail: '', own: true }, createdAt: new Date().toISOString()
}, o);

async function open(t, ctxOpts) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--ignore-gpu-blocklist'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext(Object.assign({ viewport: { width: 900, height: 900 } }, ctxOpts || {}));
  const pg = await ctx.newPage();
  const errors = [];
  pg.on('pageerror', (e) => errors.push(String(e) + ' ' + e.stack));
  t.after(() => assert.deepEqual(errors, [], 'no script errors'));
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    // The engine, its sets and the libraries are served from the repo at their site paths.
    if (p.startsWith('/static/') || p.startsWith('/js/')) {
      const file = p.startsWith('/js/') ? join(root, 'internal/plugins/rolls/static', p) : join(root, p);
      if (normalize(file).startsWith(root) && existsSync(file) && statSync(file).isFile()) return route.fulfill({ contentType: MIME[extname(file)] || 'application/octet-stream', body: readFileSync(file) });
      return route.fulfill({ status: 404, body: '' });
    }
    return route.fulfill({ contentType: 'text/html', body: page });
  });
  await pg.goto('http://t.test/');
  await pg.waitForFunction(() => document.querySelector('.tt-host')._table && document.querySelector('#card')._dtr);
  return pg;
}

const deliver = (pg, view) => pg.evaluate((v) => document.querySelector('.tt-host')._table.receive(v, true), view);
const tray = (pg) => pg.evaluate(() => document.querySelector('#card')._dtr.tray.state());
// Dice count on the mat at its current level, either kind.
const dice = (s) => s.three + s.flat;
const settle = (pg, want) => pg.waitForFunction((n) => { const s = document.querySelector('#card')._dtr.tray.state(); return s.three + s.flat >= n; }, want, { timeout: 20000 });

test('a public roll from another member lands on the mat with their name tag', { skip }, async (t) => {
  const pg = await open(t);
  await deliver(pg, roll({ rollerName: 'Tyne' }));
  await settle(pg, 2);
  const s = await tray(pg);
  assert.equal(dice(s), 2, 'both dice are on the mat');
  assert.ok(s.tags.includes('Tyne'), 'the roller\'s name tag is on the mat: ' + s.tags);
  assert.equal(s.level, 'detailed', 'a new tray starts at Detailed');
  await pg.waitForFunction(() => /Tyne rolled 2d10\+1: 4 \+ 6 \+ 1 = 11/.test(document.querySelector('.dtr-result').textContent), null, { timeout: 20000 });
});

test('a private, Director-only or blind roll never appears on the mat', { skip }, async (t) => {
  const pg = await open(t);
  // gm and self are what the Director's or the roller's own browser can hold; blind is a Director's
  // full copy, and the stub is what the roller gets. None may be drawn, and none may reach the result line.
  for (const v of [roll({ mode: 'gm' }), roll({ mode: 'self' }), roll({ mode: 'blind' }), roll({ mode: 'blind', hidden: true, dice: undefined, total: undefined }),
    roll({ mode: 'gm', mine: true, rollerId: 'u1' }), roll({ mode: undefined })]) await deliver(pg, v);
  await pg.waitForTimeout(1200);
  const s = await tray(pg);
  assert.equal(dice(s), 0, 'no die was drawn');
  assert.deepEqual(s.tags, [], 'no name tag was drawn');
  assert.equal(s.result, '', 'no result line was written');
  // Nothing of the roll is anywhere in the card's markup either.
  assert.ok(!(await pg.evaluate(() => document.querySelector('#card').textContent)).includes('Tyne'));
  // A public one still lands, so the test is not passing on a mat that draws nothing at all.
  await deliver(pg, roll({ rollerName: 'Kell', rollerId: 'u3' }));
  await settle(pg, 2);
  assert.ok((await tray(pg)).tags.includes('Kell'));
});

test('Others\' dice Off hides other members\' dice and keeps your own', { skip }, async (t) => {
  const pg = await open(t);
  await deliver(pg, roll({ rollerName: 'Tyne' }));
  await deliver(pg, roll({ rollerName: 'Orrin', rollerId: 'u1', mine: true, dice: [{ sides: 20, value: 17 }], formula: 'd20', modifier: 0, total: 17 }));
  await settle(pg, 3);
  assert.deepEqual((await tray(pg)).tags.sort(), ['Tyne', 'You']);
  await pg.click('[data-dtr-set="trayOthers"][data-v="off"]');
  await pg.waitForFunction(() => { const s = document.querySelector('#card')._dtr.tray.state(); return s.three + s.flat === 1; }, null, { timeout: 20000 });
  assert.deepEqual((await tray(pg)).tags, ['You'], 'only your own dice stay');
  assert.deepEqual(await pg.evaluate(() => window.__sets.slice(-1)[0]), ['trayOthers', 'off'], 'the choice is saved with the screen prefs');
  // A new roll from another member stays off the mat while Off.
  await deliver(pg, roll({ rollerName: 'Tyne' }));
  await pg.waitForTimeout(800);
  assert.equal(dice(await tray(pg)), 1);
  await pg.click('[data-dtr-set="trayOthers"][data-v="on"]');
  await settle(pg, 3);
  assert.deepEqual((await tray(pg)).tags.sort(), ['Tyne', 'You']);
  // Another member's roll on the result line goes when Off is chosen.
  await deliver(pg, roll({ id: 'r-late', rollerName: 'Tyne' }));
  await pg.waitForFunction(() => /^Tyne rolled/.test(document.querySelector('.dtr-result').textContent), null, { timeout: 20000 });
  await pg.click('[data-dtr-set="trayOthers"][data-v="off"]');
  assert.equal((await tray(pg)).result, '', 'Off clears another member\'s result line');
});

test('the level switch changes the drawing: 3D dice at Detailed, flat dice at the others', { skip }, async (t) => {
  const pg = await open(t);
  await deliver(pg, roll());
  await settle(pg, 2);
  let s = await tray(pg);
  assert.equal(s.three, 2, 'Detailed throws 3D dice (a browser with no WebGL would show them flat)');
  assert.equal(s.flat, 0);
  assert.equal(await pg.locator('.dtr-mat canvas.tray3d-canvas').count(), 1);

  await pg.click('[data-dtr-set="trayLevel"][data-v="flat"]');
  await pg.waitForFunction(() => document.querySelector('#card')._dtr.tray.state().flat === 2);
  s = await tray(pg);
  assert.equal(s.three, 0, 'no 3D die at Flat paper');
  assert.equal(await pg.locator('.dtr-fd[data-look="paper"]').count(), 2, 'flat paper dice');
  assert.ok(s.tags.includes('Tyne'), 'the name tag stays');
  assert.equal(await pg.locator('.dtr-mat > canvas.dtr-matart').count(), 1, 'the flat mat is a plain canvas');

  await pg.click('[data-dtr-set="trayLevel"][data-v="simple"]');
  await pg.waitForFunction(() => document.querySelectorAll('.dtr-fd[data-look="simple"]').length === 2);
  assert.equal((await tray(pg)).three, 0);

  await pg.click('[data-dtr-set="trayLevel"][data-v="detailed"]');
  await pg.waitForFunction(() => { const s = document.querySelector('#card')._dtr.tray.state(); return s.three === 2 && s.flat === 0; }, null, { timeout: 20000 });
  assert.deepEqual(await pg.evaluate(() => window.__sets.map((x) => x[1])), ['flat', 'simple', 'detailed'], 'each choice is saved');
});

test('at Flat paper and Simple a name tag never covers its roller\'s own dice', { skip }, async (t) => {
  const pg = await open(t);
  await pg.click('[data-dtr-set="trayLevel"][data-v="flat"]');
  const six = [4, 6, 8, 10, 12, 20].map((s) => ({ sides: s, value: 2 }));
  // Rollers on different sides, one with two rows of dice.
  for (const [id, name, n] of [['u2', 'Tyne', 6], ['u3', 'Kell', 2], ['u4', 'Ash', 1], ['u5', 'Bryn', 1]]) {
    await deliver(pg, roll({ id: 'r-' + id, rollerId: id, rollerName: name, dice: six.slice(0, n), formula: n + 'd', modifier: 0, total: 2 * n }));
  }
  await pg.waitForFunction(() => document.querySelectorAll('.dtr-fd').length === 10 && document.querySelectorAll('.dtr-tag').length === 4, null, { timeout: 20000 });
  await pg.waitForTimeout(700);
  for (const look of ['flat', 'simple']) {
    if (look === 'simple') { await pg.click('[data-dtr-set="trayLevel"][data-v="simple"]'); await pg.waitForFunction(() => document.querySelectorAll('.dtr-fd[data-look="simple"]').length === 10); }
    const hits = await pg.evaluate(() => {
      const out = [];
      const tags = [...document.querySelectorAll('.dtr-tag')];
      // A roll's elements are its dice, then its tag, in order.
      let dice = [];
      for (const n of document.querySelector('.dtr-flat').children) {
        if (n.classList.contains('dtr-fd')) { dice.push(n); continue; }
        if (!n.classList.contains('dtr-tag')) continue;
        const a = n.getBoundingClientRect();
        dice.forEach((d) => { const b = d.getBoundingClientRect(); if (a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom) out.push(n.textContent); });
        dice = [];
      }
      return { out, n: tags.length };
    });
    assert.equal(hits.n, 4);
    assert.deepEqual(hits.out, [], look + ': a tag covers its own die');
  }
});

test('with motion reduced the result shows at once and no dice are thrown', { skip }, async (t) => {
  const pg = await open(t, { reducedMotion: 'reduce' });
  await deliver(pg, roll({ rollerName: 'Tyne' }));
  // At once: no wait for a throw to land.
  await pg.waitForFunction(() => /Tyne rolled 2d10\+1: 4 \+ 6 \+ 1 = 11/.test(document.querySelector('.dtr-result').textContent), null, { timeout: 6000 });
  const s = await tray(pg);
  assert.equal(dice(s), 0, 'nothing is thrown');
});

test('a roll made from the card is decided by the server, thrown in the card and kept off the page-wide throw', { skip }, async (t) => {
  const pg = await open(t, { reducedMotion: 'reduce' });
  await pg.evaluate((v) => { window.__reply = v; }, roll({ rollerName: 'Orrin', rollerId: 'u1', mine: true }));
  await pg.click('[data-dtr-go]');
  await pg.waitForFunction(() => window.__posts.length === 1);
  assert.deepEqual(await pg.evaluate(() => window.__posts[0]), { formula: '2d10+2', mode: 'public' });
  await pg.waitForFunction(() => /rolled 2d10\+1: 4 \+ 6 \+ 1 = 11/.test(document.querySelector('.dtr-result').textContent));
  // The same roll arriving on the socket afterwards is shown once, and the page does not throw it too.
  const view = await pg.evaluate(() => window.__reply);
  await deliver(pg, view);
  await pg.waitForTimeout(300);
  assert.equal(await pg.evaluate(() => window.__thrown), 0);
});
