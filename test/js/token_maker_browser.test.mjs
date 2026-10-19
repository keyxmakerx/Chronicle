// token_maker_browser.test.mjs — the token maker (Chronicle.tokenMaker) in a
// real Chromium, loaded the way the layout does it: boot.js's stub fetches
// token_maker.js on the first open, and it fetches the frames, its styles and
// the previews sprite from the URLs the layout lists. The picker shows still
// previews from the sprite and plays only a moving frame the pointer rests on;
// Save uploads a PNG and writes the page's token field; closing with unsaved
// work asks first; Calm and Off open it without growing and hold frames still;
// and the sprite is still one tile per design, in the engine's order.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, extname } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, f), 'utf8');
const MEDIA_ID = '0b7d5c1e-3f0a-4c55-9a4e-2f1f7c9d8e21';

// The slip's rules and the motion tokens the maker and the slip use.
const CSS = ':root{--dur-slide:360ms;--ease-slide:ease;--dur-standard:200ms;--dur-micro:120ms;--ease-out:ease}' +
  read('static/css/input.css').split('\n').filter((l) => l.includes('ask-slip')).join('\n');

const HTML = (motion) => `<!doctype html><html${motion ? ` data-motion="${motion}" data-view-motion="calm"` : ''}><head><meta charset="utf-8">
<style>${CSS}html,body{margin:0}.pic{position:relative;width:320px;height:224px;margin:40px}</style></head><body>
<div class="pic"><img src="/media/pic.svg" alt="" style="width:100%;height:100%;object-fit:cover">
<button type="button" id="chip" data-token-maker data-campaign-id="c1" data-entity-id="e1" data-name="Brother Ansel" data-art="/media/pic.svg">Make token</button></div>
<template id="chronicle-token-maker-src" data-src="/static/js/token_maker.js?v=t" data-engine="/static/js/token_frames.js?v=t" data-css="/static/css/token_maker.css?v=t" data-previews="/static/img/token_frames.webp?v=t"></template>
<script src="/static/js/theme.js"></script><script src="/static/js/motion_rest.js"></script>
<script src="/static/vendor/htmx.min.js" defer></script><script src="/static/js/boot.js" defer></script>
</body></html>`;

// A painted figure the maker can frame: same origin, so the canvas stays readable.
const PIC = '<svg xmlns="http://www.w3.org/2000/svg" width="600" height="800"><rect width="600" height="800" fill="#6d8fb3"/>' +
  '<circle cx="300" cy="260" r="120" fill="#e8c39e"/><rect x="150" y="380" width="300" height="420" rx="60" fill="#8a2f2f"/></svg>';

const TYPES = { '.js': 'application/javascript', '.css': 'text/css', '.webp': 'image/webp' };

async function page(t, motion, viewport = { width: 1280, height: 860 }) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  p.fetched = []; p.uploads = []; p.puts = [];
  await p.route('**/*', async (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: HTML(motion) });
    if (u.pathname === '/media/pic.svg') return route.fulfill({ contentType: 'image/svg+xml', body: PIC });
    if (u.pathname === '/media/upload' && req.method() === 'POST') {
      p.uploads.push({ type: req.headers()['content-type'] || '', body: req.postDataBuffer() });
      return route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: MEDIA_ID, url: '/media/' + MEDIA_ID }) });
    }
    if (u.pathname === '/campaigns/c1/entities/e1/fields' && req.method() === 'PUT') {
      p.puts.push(JSON.parse(req.postData()));
      return route.fulfill({ contentType: 'application/json', body: '{}' });
    }
    const file = join(root, u.pathname);
    if (!u.pathname.startsWith('/static/') || !existsSync(file)) return route.fulfill({ status: 404, body: '' });
    p.fetched.push(u.pathname);
    return route.fulfill({ contentType: TYPES[extname(file)] || 'application/octet-stream', body: readFileSync(file) });
  });
  await p.goto('http://chronicle.test/');
  await p.waitForFunction(() => window.Chronicle && window.Chronicle.tokenMaker);
  return p;
}
async function openMaker(p) {
  await p.click('#chip');
  await p.waitForSelector('.tkm.is-open [data-tkm-save]:not([disabled])', { timeout: 60000 });
}

test('nothing of the maker loads with the page; the first open fetches it all', { skip }, async (t) => {
  const p = await page(t, 'off');
  const maker = (f) => /token_(maker|frames)/.test(f);
  assert.deepEqual(p.fetched.filter(maker), []);
  await openMaker(p);
  await p.waitForFunction(() => performance.getEntriesByType('resource').some((r) => r.name.includes('token_frames.webp')));
  assert.deepEqual(p.fetched.filter(maker).sort(), ['/static/css/token_maker.css', '/static/img/token_frames.webp', '/static/js/token_frames.js', '/static/js/token_maker.js']);
  const head = await p.evaluate(() => ({ title: document.getElementById('tkm-title').textContent, dialog: document.querySelector('.tkm').getAttribute('role'), modal: document.querySelector('.tkm').getAttribute('aria-modal') }));
  assert.deepEqual(head, { title: 'Token for Brother Ansel', dialog: 'dialog', modal: 'true' });
});

test('the picker shows still previews from the sprite, one group at a time', { skip }, async (t) => {
  const p = await page(t, 'off');
  await openMaker(p);
  const out = await p.evaluate(() => {
    const F = window.ChronicleTokenFrames, tiles = [...document.querySelectorAll('[data-tkm-frames] .tkm-fr')];
    const g = F.groups.filter((x) => x.id === document.querySelector('[data-tkm-groups] [aria-pressed="true"]').getAttribute('data-g'))[0];
    return {
      groups: document.querySelectorAll('[data-tkm-groups] button').length, groupCount: F.groups.length,
      tiles: tiles.map((b) => b.getAttribute('data-v')), want: g.ids.concat(['none']),
      canvases: document.querySelectorAll('[data-tkm-frames] canvas').length,
      sprite: tiles.every((b) => b.querySelector('.tkm-pv').style.backgroundImage.includes('/static/img/token_frames.webp')),
    };
  });
  assert.equal(out.groups, out.groupCount);
  assert.deepEqual(out.tiles, out.want);
  assert.equal(out.canvases, 0, 'no frame in the picker is drawn live');
  assert.ok(out.sprite, 'every tile shows its part of the sprite');
});

test('a moving frame plays in its tile only while the pointer rests on it', { skip }, async (t) => {
  const p = await page(t, null);
  await openMaker(p);
  await p.click('[data-tkm-groups] [data-g="elements"]');
  await p.hover('.tkm-fr[data-v="fire"]');
  await p.waitForSelector('.tkm-fr[data-v="fire"] canvas.tkm-hot', { timeout: 5000 });
  assert.equal(await p.evaluate(() => document.querySelectorAll('[data-tkm-frames] canvas').length), 1);
  await p.mouse.move(5, 5);
  await p.waitForFunction(() => !document.querySelector('[data-tkm-frames] canvas'));
  // A still frame keeps its preview: nothing is drawn for it.
  await p.click('[data-tkm-groups] [data-g="stone"]');
  await p.hover('.tkm-fr[data-v="granite"]');
  await p.waitForTimeout(500);
  assert.equal(await p.evaluate(() => document.querySelectorAll('[data-tkm-frames] canvas').length), 0);
});

test('save uploads a 512 pixel PNG and puts it in the page\'s token field', { skip }, async (t) => {
  const p = await page(t, 'off');
  await openMaker(p);
  await p.click('[data-tkm-groups] [data-g="magic"]');
  await p.click('.tkm-fr[data-v="arcane"]');
  await p.click('[data-tkm-colour="#2f5fc8"]');
  await p.evaluate(() => document.addEventListener('chronicle:token-saved', (e) => { window.__saved = e.detail; }));
  await p.click('[data-tkm-save]');
  await p.waitForFunction(() => !document.querySelector('.tkm'), null, { timeout: 30000 });
  assert.equal(p.uploads.length, 1);
  const up = p.uploads[0], text = up.body.toString('latin1');
  assert.match(up.type, /^multipart\/form-data/);
  assert.match(text, /name="usage_type"\r\n\r\nentity_image/);
  assert.match(text, /name="campaign_id"\r\n\r\nc1/);
  assert.match(text, /filename="token.png"\r\nContent-Type: image\/png/);
  const png = up.body.subarray(up.body.indexOf(Buffer.from([0x89, 0x50, 0x4e, 0x47])));
  assert.deepEqual([png.readUInt32BE(16), png.readUInt32BE(20)], [512, 512]);
  assert.equal(png[25], 6, 'the PNG keeps its transparency (RGBA)');
  assert.deepEqual(p.puts, [{ fields_patch: { token_image: MEDIA_ID } }]);
  const after = await p.evaluate(() => ({ saved: window.__saved, focus: document.activeElement && document.activeElement.id, dirty: Chronicle.isDirty(), kept: localStorage.getItem('chronicle.tokenMaker') }));
  assert.deepEqual(after.saved, { entityId: 'e1', mediaId: MEDIA_ID, url: '/media/' + MEDIA_ID });
  assert.equal(after.focus, 'chip', 'focus goes back to what opened it');
  assert.equal(after.dirty, false);
  assert.deepEqual(JSON.parse(after.kept), { frame: 'arcane', colour: '#2f5fc8', shape: 'round' });
});

test('a caller that saves the token itself gets the media id, and the page field is left alone', { skip }, async (t) => {
  const p = await page(t, 'off');
  await p.evaluate(() => {
    window.__got = []; window.__closed = 0;
    Chronicle.tokenMaker.open({ campaignId: 'c1', entityId: '', name: 'New creature', art: '/media/pic.svg', from: document.getElementById('chip'),
      save: (id) => { window.__got.push(id); }, onClose: () => { window.__closed++; } });
  });
  await p.waitForSelector('.tkm.is-open [data-tkm-save]:not([disabled])', { timeout: 60000 });
  await p.click('[data-tkm-save]');
  await p.waitForFunction(() => !document.querySelector('.tkm'), null, { timeout: 30000 });
  assert.deepEqual(await p.evaluate(() => [window.__got, window.__closed]), [[MEDIA_ID], 1]);
  assert.equal(p.uploads.length, 1);
  assert.deepEqual(p.puts, []);
});

test('closing with unsaved work asks first; a second Escape goes back to it', { skip }, async (t) => {
  const p = await page(t, 'off');
  await openMaker(p);
  // Untouched, Escape just closes.
  await p.keyboard.press('Escape');
  await p.waitForFunction(() => !document.querySelector('.tkm'));
  await openMaker(p);
  await p.click('[data-tkm-shape] [data-v="square"]');
  await p.keyboard.press('Escape');
  await p.waitForSelector('.tkm .ask-slip.is-on');
  assert.equal(await p.textContent('.tkm .ask-slip [data-ask]'), 'Discard this token?');
  await p.keyboard.press('Escape');
  await p.waitForFunction(() => !document.querySelector('.tkm .ask-slip.is-on'));
  assert.ok(await p.$('.tkm.is-open'), 'the maker is still open');
  await p.click('.tkm [data-tkm-close].tkm-x');
  await p.waitForSelector('.tkm .ask-slip.is-on');
  await p.click('.tkm .ask-slip [data-drop]');
  await p.waitForFunction(() => !document.querySelector('.tkm'));
  assert.equal(await p.evaluate(() => document.activeElement.id), 'chip');
  assert.equal(p.uploads.length, 0);
});

test('Calm and Off open it in place and hold moving frames still', { skip }, async (t) => {
  for (const motion of ['calm', 'off']) {
    const p = await page(t, motion);
    await openMaker(p);
    const cls = await p.evaluate(() => document.querySelector('.tkm').className);
    assert.match(cls, motion === 'off' ? /is-still/ : /is-calm/);
    await p.click('[data-tkm-groups] [data-g="elements"]');
    await p.click('.tkm-fr[data-v="fire"]');
    await p.hover('.tkm-fr[data-v="lava"]');
    await p.waitForTimeout(500);
    const out = await p.evaluate(() => ({ hot: !!document.querySelector('.tkm-hot'), motion: document.querySelector('[data-tkm-motion]').hidden, note: document.querySelector('[data-tkm-note]').textContent }));
    assert.deepEqual([out.hot, out.motion], [false, true], motion);
    assert.match(out.note, /holds still while motion is set to Calm or Off/);
  }
});

test('on a phone the maker fills the screen and the page does not scroll sideways', { skip }, async (t) => {
  const p = await page(t, 'off', { width: 390, height: 800 });
  await openMaker(p);
  const out = await p.evaluate(() => {
    const r = document.querySelector('.tkm-p').getBoundingClientRect(), f = document.querySelector('[data-tkm-frames]');
    return { w: Math.round(r.width), h: Math.round(r.height), wide: document.documentElement.scrollWidth > innerWidth, strip: getComputedStyle(f).overflowX };
  });
  assert.deepEqual(out, { w: 390, h: 800, wide: false, strip: 'auto' });
});

// The sprite is drawn by tools/token_frames_previews.mjs; a design added,
// removed or reordered without re-running it shows the wrong previews.
test('the previews sprite has one tile per design, in the engine\'s order', { skip }, async (t) => {
  const p = await page(t, 'off');
  await p.addScriptTag({ url: '/static/js/token_frames.js' });
  const out = await p.evaluate(async () => {
    const F = window.ChronicleTokenFrames, img = new Image();
    img.src = '/static/img/token_frames.webp';
    await img.decode();
    const n = F.designs.length, cols = 8, tile = img.naturalWidth / cols, art = F.sample(), place = F.place(art), S = 24;
    const small = (src, sx, sy) => {
      const c = document.createElement('canvas'); c.width = c.height = S;
      const g = c.getContext('2d'); g.drawImage(src, sx, sy, tile, tile, 0, 0, S, S);
      return g.getImageData(0, 0, S, S).data;
    };
    const fresh = F.designs.map((d) => {
      const c = document.createElement('canvas'); c.width = c.height = tile;
      F.render(c.getContext('2d'), tile, tile, { img: art, place, frame: d.id, colour: '', shape: 'round', t: 0, still: true });
      return small(c, 0, 0);
    });
    const dist = (a, b) => { let s = 0; for (let k = 0; k < a.length; k++) s += Math.abs(a[k] - b[k]); return s; };
    const nearest = [];
    for (let i = 0; i < n; i++) {
      const got = small(img, (i % cols) * tile, Math.floor(i / cols) * tile);
      let best = -1, bd = Infinity;
      fresh.forEach((f, j) => { const d = dist(got, f); if (d < bd) { bd = d; best = j; } });
      nearest.push(F.designs[best].id);
    }
    return { size: [img.naturalWidth, img.naturalHeight], want: [cols * tile, Math.ceil(n / cols) * tile], tile, ids: F.designs.map((d) => d.id), nearest };
  });
  assert.equal(out.tile, 128);
  assert.deepEqual(out.size, out.want, 'run node tools/token_frames_previews.mjs');
  assert.deepEqual(out.nearest, out.ids, 'run node tools/token_frames_previews.mjs');
});
