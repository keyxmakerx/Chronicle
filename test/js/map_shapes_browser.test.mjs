// map_shapes_browser.test.mjs — drives the Shapes tool in a real Chromium: the
// real Leaflet, the real annotation, shapes and drawing-tool scripts, on a
// blank map (no picture), with an in-page stand-in for the drawings API. It
// pins what a person can do by hand: place a room, area, water, wall, arrow
// and bubble from the palette, select one, move it, resize it from a handle,
// rename it or edit the bubble's text, nudge it with the keyboard and delete
// it; that only the changed fields are sent; that a refused save puts the
// shape back; that a player cannot pick anything up; and that user text is
// never parsed as markup.
//
// Needs playwright-core and a Chromium. It looks in the usual places
// (NODE_PATH, /opt/node-tools, PW_CHROMIUM or /opt/pw-browsers) and skips,
// saying so, when none is found.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const JS = join(root, 'internal/plugins/maps/static/js');

function findPlaywright() {
  for (const base of [import.meta.url, '/opt/node-tools/', process.env.NODE_PATH ? process.env.NODE_PATH + '/' : null]) {
    if (!base) continue;
    try {
      const req = createRequire(base.startsWith('file:') || base.startsWith('/') ? (base.startsWith('/') ? base + 'x.js' : base) : base);
      return req('playwright-core');
    } catch (e) { /* try the next place */ }
  }
  return null;
}
function findChromium() {
  if (process.env.PW_CHROMIUM && existsSync(process.env.PW_CHROMIUM)) return process.env.PW_CHROMIUM;
  try {
    for (const d of readdirSync('/opt/pw-browsers')) {
      const c = join('/opt/pw-browsers', d, 'chrome-linux', 'chrome');
      if (d.startsWith('chromium-') && existsSync(c)) return c;
    }
  } catch (e) { /* none */ }
  return null;
}

const pw = findPlaywright();
const chromePath = findChromium();
const skip = !pw || !chromePath ? 'playwright-core or Chromium not found' : false;

const read = (f) => readFileSync(f, 'utf8');

const PAGE_CSS = `
html, body { margin: 0; height: 100%; }
:root { --color-card-bg: #ffffff; --color-text-primary: #111827; --color-text-body: #374151; --color-text-secondary: #6b7280; --color-border: #d1d5db; --color-bg-tertiary: #f3f4f6; --mp-accent: #6366f1; --mp-float: #ffffffee; }
html.dark { --color-card-bg: #1f2937; --color-text-primary: #f9fafb; --color-text-body: #e5e7eb; --color-text-secondary: #9ca3af; --color-border: #4b5563; --color-bg-tertiary: #374151; }
#map-wrap { position: relative; width: 900px; height: 600px; background: #e5e7eb; }
#map-container { position: absolute; inset: 0; }
.mp-fl { position: absolute; z-index: 1100; background: var(--mp-float); border: 1px solid var(--color-border); border-radius: 10px; }
[hidden] { display: none !important; }
`;

// A stand-in for the drawings API: it keeps rows in memory, applies a PUT as a
// partial update, and records every call so a test can see exactly what was
// sent. __fail makes the next write answer 400.
const STUB = `
window.__store = []; window.__calls = []; window.__fail = 0; window.__notes = [];
var n = 0;
function reply(status, obj) { return Promise.resolve({ ok: status < 400, status: status, json: function () { return Promise.resolve(obj); } }); }
window.Chronicle = {
  notify: function (m) { window.__notes.push(m); },
  escapeHtml: function (s) { var d = document.createElement('div'); d.textContent = s; return d.innerHTML; },
  escapeAttr: function (s) { return String(s).replace(/"/g, '&quot;'); },
  apiFetch: function (url, o) {
    o = o || {};
    var m = o.method || 'GET';
    var body = o.body ? JSON.parse(JSON.stringify(o.body)) : undefined;
    window.__calls.push({ method: m, url: url, body: body });
    var id = url.split('/')[6];
    if (m !== 'GET' && window.__fail > 0) { window.__fail--; return reply(400, { message: 'refused' }); }
    if (m === 'GET') return reply(200, JSON.parse(JSON.stringify(window.__store)));
    if (m === 'POST') {
      var d = Object.assign({ id: 'd' + (++n), map_id: 'm1', rotation: 0, visibility: 'everyone', created_by: 'u1' }, body);
      window.__store.push(d);
      return reply(201, JSON.parse(JSON.stringify(d)));
    }
    var row = window.__store.filter(function (x) { return x.id === id; })[0];
    if (!row) return reply(404, {});
    if (m === 'PUT') { Object.assign(row, body); return reply(200, {}); }
    if (m === 'DELETE') { window.__store.splice(window.__store.indexOf(row), 1); return reply(204, {}); }
    return reply(405, {});
  }
};
`;

// The viewer's own contract with the drawing module, reduced to what the
// module reads: the map, the ids, the gates, the grid, and setTool.
const BOOT = (o) => `
var map = L.map('map-container', { crs: L.CRS.Simple, minZoom: -3, maxZoom: 3, zoomSnap: 0.25, zoomControl: false, attributionControl: false, doubleClickZoom: false, zoomAnimation: false, fadeAnimation: false, markerZoomAnimation: false });
var bounds = [[0, 0], [1000, 1000]];
map.createPane('mpBase').style.zIndex = 350;
L.rectangle(bounds, { pane: 'mpBase', className: 'mp-sheet', interactive: false, weight: 1, fill: true }).addTo(map);
map.fitBounds(bounds);
var ctx = window.chronicleMap = {
  map: map, campaignID: 'c1', mapID: 'm1', imageW: 1000, imageH: 1000,
  isScribe: ${o.scribe}, canDraw: ${o.scribe}, isOwner: ${o.owner}, canDmOnly: ${o.owner}, userID: 'u1',
  getDisplay: function () { return { grid_size: 50, grid_type: 'square' }; },
  setHint: function () {},
  setTool: function (t) { ctx.tool = t; if (t === 'move') { ctx.draw.cancel(); ctx.draw.pictureSelectMode(true); } }
};
window.ChronicleMapDrawing.init(ctx);
`;

async function open(t, o = {}) {
  o = Object.assign({ scribe: true, owner: true, dark: false, rows: [] }, o);
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1000, height: 700 }, reducedMotion: o.reducedMotion ? 'reduce' : 'no-preference' });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, [], 'the page threw'));
  await p.setContent(`<!doctype html><html${o.dark ? ' class="dark"' : ''}><head><style>${read(join(root, 'static/vendor/leaflet.css'))}</style><style>${PAGE_CSS}</style></head>` +
    '<body><div id="map-wrap"><div id="map-container"></div></div></body></html>');
  await p.addScriptTag({ content: STUB });
  if (o.rows.length) await p.evaluate((rows) => { window.__store.push(...rows); }, o.rows);
  await p.addScriptTag({ content: read(join(root, 'static/vendor/leaflet.js')) });
  for (const f of ['map_annotations.js', 'map_shapes.js', 'map_drawing_tools.js']) await p.addScriptTag({ content: read(join(JS, f)) });
  await p.addScriptTag({ content: BOOT(o) });
  // The drawing module loads its rows asynchronously.
  await p.waitForFunction(() => window.chronicleMap.draw && window.chronicleMap.draw.count() >= window.__store.length);
  return p;
}

// Map percent -> page pixels.
const at = (p, x, y) => p.evaluate(([x, y]) => {
  const m = window.chronicleMap.map;
  const c = m.getContainer().getBoundingClientRect();
  const pt = m.latLngToContainerPoint(L.latLng(1000 - y / 100 * 1000, x / 100 * 1000));
  return { x: c.left + pt.x, y: c.top + pt.y };
}, [x, y]);

async function drag(p, from, to, opts = {}) {
  const a = await at(p, ...from), b = await at(p, ...to);
  await p.mouse.move(a.x, a.y);
  await p.mouse.down();
  await p.mouse.move((a.x + b.x) / 2, (a.y + b.y) / 2, { steps: 4 });
  await p.mouse.move(b.x, b.y, { steps: 4 });
  if (opts.hold) await opts.hold();
  await p.mouse.up();
}
async function click(p, x, y) {
  const a = await at(p, x, y);
  await p.mouse.click(a.x, a.y);
}

const calls = (p, method) => p.evaluate((m) => window.__calls.filter((c) => c.method === m), method);
const store = (p) => p.evaluate(() => JSON.parse(JSON.stringify(window.__store)));
const last = async (p, m) => { const c = await calls(p, m); return c[c.length - 1]; };
const settle = (p) => p.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(() => setTimeout(r, 30)))));
const selected = (p) => p.evaluate(() => !!document.querySelector('.mp-shp-h'));
const barVisible = (p) => p.evaluate(() => { const b = document.querySelector('.mp-shpbar'); return !!b && !b.hidden; });
const focusedIs = (p, sel) => p.evaluate((s) => document.activeElement && document.activeElement.matches(s), sel);
const startPalette = (p, key) => p.evaluate((k) => window.chronicleMap.draw.start('pal:' + k), key);

const ROOM = { id: 'r1', map_id: 'm1', drawing_type: 'room', points: [{ x: 10, y: 10 }, { x: 30, y: 25 }], stroke_color: '#334155', stroke_width: 2, fill_alpha: 0, text_content: 'Great hall', visibility: 'everyone', created_by: 'u1' };

test('the palette places a room by dragging, snapped to the grid, and selects it with its name ready', { skip }, async (t) => {
  const p = await open(t);
  await startPalette(p, 'room');
  await drag(p, [11, 9], [31, 24]);
  await p.waitForFunction(() => window.__store.length === 1);
  const post = await last(p, 'POST');
  assert.equal(post.body.drawing_type, 'room');
  assert.deepEqual(post.body.points, [{ x: 10, y: 10 }, { x: 30, y: 25 }]);
  assert.equal(post.body.text_content, 'Room');
  assert.match(post.body.stroke_color, /^#[0-9a-f]{6}$/i);
  await settle(p);
  assert.equal(await p.evaluate(() => window.chronicleMap.tool), 'move', 'Select takes over after placing');
  assert.ok(await barVisible(p));
  assert.ok(await selected(p), 'the new shape has handles');
  assert.ok(await focusedIs(p, '.mp-shpbar input[type=text]'));
  assert.equal(await p.inputValue('.mp-shpbar input[type=text]'), 'Room');
  assert.equal(await p.locator('.mp-shp-lbl').textContent(), 'Room');
  assert.equal(await p.locator('.mp-shp-h').count(), 8);
});

test('every palette kind places the record the server and the sketch share', { skip }, async (t) => {
  const p = await open(t);
  const want = { area: 'area', water: 'water', wall: 'wall' };
  for (const [key, type] of Object.entries(want)) {
    await startPalette(p, key);
    await click(p, 50, 50);
    await p.waitForFunction((n) => window.__store.length === n, Object.keys(want).indexOf(key) + 1);
    await settle(p);
    await p.keyboard.press('Escape');
    await p.evaluate(() => window.chronicleMap.draw.escape());
    const row = (await store(p)).pop();
    assert.equal(row.drawing_type, type);
    assert.equal(row.points.length, 2);
    assert.ok(row.points[1].x > row.points[0].x && row.points[1].y > row.points[0].y);
    if (type === 'wall') assert.equal(row.text_content, undefined, 'a wall has no name');
  }
  await startPalette(p, 'arrow');
  await drag(p, [60, 60], [80, 70]);
  await p.waitForFunction(() => window.__store.length === 4);
  assert.deepEqual((await store(p))[3].points, [{ x: 60, y: 60 }, { x: 80, y: 70 }]);
  await settle(p);
  await p.evaluate(() => window.chronicleMap.draw.escape());
  await startPalette(p, 'bubble');
  await click(p, 40, 20);
  await p.waitForFunction(() => window.__store.length === 5);
  const bubble = (await store(p))[4];
  assert.equal(bubble.drawing_type, 'callout');
  assert.deepEqual(bubble.points, [{ x: 40, y: 20 }]);
  assert.equal(bubble.text_content, 'Note');
});

test('a click on the map with a box tool drops the default size', { skip }, async (t) => {
  const p = await open(t);
  await startPalette(p, 'room');
  await click(p, 21, 19);
  await p.waitForFunction(() => window.__store.length === 1);
  assert.deepEqual((await store(p))[0].points, [{ x: 20, y: 20 }, { x: 35, y: 30 }]);
});

test('a placed shape follows the map to the corner of a click near the edge', { skip }, async (t) => {
  const p = await open(t);
  await startPalette(p, 'area');
  await click(p, 97, 97);
  await p.waitForFunction(() => window.__store.length === 1);
  const pts = (await store(p))[0].points;
  assert.ok(pts[1].x <= 100 && pts[1].y <= 100, JSON.stringify(pts));
});

test('selecting a shape and dragging it sends only the new points', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  await settle(p);
  assert.ok(await selected(p), 'a clean click picks the shape up');
  assert.equal(await calls(p, 'PUT').then((c) => c.length), 0, 'selecting writes nothing');
  await drag(p, [20, 17], [37, 29]);
  await p.waitForFunction(() => window.__calls.some((c) => c.method === 'PUT'));
  const put = await last(p, 'PUT');
  assert.deepEqual(Object.keys(put.body), ['points'], 'a move is a partial update');
  assert.deepEqual(put.body.points, [{ x: 25, y: 20 }, { x: 45, y: 35 }]);
  // The record keeps its name and everything else it had.
  const row = (await store(p))[0];
  assert.equal(row.text_content, 'Great hall');
  assert.equal(row.stroke_color, '#334155');
  assert.ok(await selected(p), 'the shape stays selected after the move');
});

test('Alt turns snapping off while dragging', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  await p.keyboard.down('Alt');
  await drag(p, [20, 17], [22, 18]);
  await p.keyboard.up('Alt');
  await p.waitForFunction(() => window.__calls.some((c) => c.method === 'PUT'));
  const pts = (await last(p, 'PUT')).body.points;
  assert.ok(Math.abs(pts[0].x - 12) < 0.6 && Math.abs(pts[0].x - 10) > 0.5, JSON.stringify(pts));
});

test('an unselected shape lets the map pan under a drag', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  const before = await p.evaluate(() => window.chronicleMap.map.getCenter());
  await drag(p, [20, 17], [10, 5]);
  const after = await p.evaluate(() => window.chronicleMap.map.getCenter());
  assert.notDeepEqual(before, after, 'the map moved');
  assert.equal(await calls(p, 'PUT').then((c) => c.length), 0, 'the shape did not');
  assert.equal(await selected(p), false);
});

test('a corner handle resizes the box, edges stay put', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  const h = await p.locator('.mp-shp-h[data-handle="se"]').boundingBox();
  const to = await at(p, 40, 35);
  await p.mouse.move(h.x + h.width / 2, h.y + h.height / 2);
  await p.mouse.down();
  await p.mouse.move(to.x, to.y, { steps: 6 });
  await p.mouse.up();
  await p.waitForFunction(() => window.__calls.some((c) => c.method === 'PUT'));
  const put = await last(p, 'PUT');
  assert.deepEqual(Object.keys(put.body), ['points']);
  assert.deepEqual(put.body.points, [{ x: 10, y: 10 }, { x: 40, y: 35 }]);
});

test('a resize cannot flip or collapse a box', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  const h = await p.locator('.mp-shp-h[data-handle="se"]').boundingBox();
  const to = await at(p, 2, 2);
  await p.mouse.move(h.x + h.width / 2, h.y + h.height / 2);
  await p.mouse.down();
  await p.mouse.move(to.x, to.y, { steps: 6 });
  await p.mouse.up();
  await p.waitForFunction(() => window.__calls.some((c) => c.method === 'PUT'));
  const [a, b] = (await last(p, 'PUT')).body.points;
  assert.ok(b.x - a.x >= 5 && b.y - a.y >= 5, 'at least one cell each way');
});

test('an arrow is moved by its body and by its two ends', { skip }, async (t) => {
  const arrow = { id: 'a1', map_id: 'm1', drawing_type: 'arrow', points: [{ x: 60, y: 60 }, { x: 80, y: 70 }], stroke_color: '#2563eb', stroke_width: 4, fill_alpha: 0, visibility: 'everyone', created_by: 'u1' };
  const p = await open(t, { rows: [arrow] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 70, 65);
  await settle(p);
  assert.ok(await selected(p), 'the wide hit line picks a thin arrow up');
  assert.equal(await p.locator('.mp-shp-h').count(), 2);
  assert.equal(await p.locator('.mp-shpbar input[type=text]').isVisible(), false, 'an arrow has no text');
  const tip = await p.locator('.mp-shp-h[data-handle="b"]').boundingBox();
  const to = await at(p, 90, 40);
  await p.mouse.move(tip.x + tip.width / 2, tip.y + tip.height / 2);
  await p.mouse.down();
  await p.mouse.move(to.x, to.y, { steps: 6 });
  await p.mouse.up();
  await p.waitForFunction(() => window.__calls.some((c) => c.method === 'PUT'));
  assert.deepEqual((await last(p, 'PUT')).body, { points: [{ x: 60, y: 60 }, { x: 90, y: 40 }] });
});

test('the name is edited in the bar, previewed live and saved when the field is left', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  const input = p.locator('.mp-shpbar input[type=text]');
  assert.equal(await input.inputValue(), 'Great hall');
  await input.fill('Throne room');
  assert.equal(await p.locator('.mp-shp-lbl').textContent(), 'Throne room', 'previewed as typed');
  assert.equal(await calls(p, 'PUT').then((c) => c.length), 0, 'nothing is saved per keystroke');
  await input.press('Enter');
  await p.waitForFunction(() => window.__calls.some((c) => c.method === 'PUT'));
  assert.deepEqual((await last(p, 'PUT')).body, { text_content: 'Throne room' });
});

test('clearing a name sends null; an emptied bubble goes back instead of failing', { skip }, async (t) => {
  const bubble = { id: 'b1', map_id: 'm1', drawing_type: 'callout', points: [{ x: 50, y: 50 }], stroke_color: '#7c3aed', stroke_width: 2, fill_alpha: 0, text_content: 'Rings at low tide', visibility: 'everyone', created_by: 'u1' };
  const p = await open(t, { rows: [ROOM, bubble] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  const input = p.locator('.mp-shpbar input[type=text]');
  await input.fill('');
  await input.press('Enter');
  await p.waitForFunction(() => window.__calls.some((c) => c.method === 'PUT'));
  assert.deepEqual((await last(p, 'PUT')).body, { text_content: null });
  assert.equal(await p.locator('.mp-shp-lbl').count(), 0, 'an unnamed room draws no label');

  await p.keyboard.press('Escape');
  await p.evaluate(() => window.chronicleMap.draw.escape());
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await p.locator('.mp-ann-bubble-text').click();
  const area = p.locator('.mp-shpbar textarea');
  assert.equal(await area.inputValue(), 'Rings at low tide');
  const puts = (await calls(p, 'PUT')).length;
  await area.fill('   ');
  await area.press('Enter');
  await settle(p);
  assert.equal((await calls(p, 'PUT')).length, puts, 'a bubble cannot be saved empty');
  assert.equal(await area.inputValue(), 'Rings at low tide');
  await area.fill('Wights rise here');
  await area.press('Enter');
  await p.waitForFunction((n) => window.__calls.filter((c) => c.method === 'PUT').length > n, puts);
  assert.deepEqual((await last(p, 'PUT')).body, { text_content: 'Wights rise here' });
});

test('names and bubble text are text, never markup', { skip }, async (t) => {
  const evil = '<img src=x onerror="window.__pwned=1"><script>window.__pwned=2</script>';
  const bubble = { id: 'b1', map_id: 'm1', drawing_type: 'callout', points: [{ x: 50, y: 50 }], stroke_color: '#7c3aed', stroke_width: 2, fill_alpha: 0, text_content: evil, visibility: 'everyone', created_by: 'u1' };
  const p = await open(t, { rows: [{ ...ROOM, text_content: evil }, bubble] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  await settle(p);
  assert.equal(await p.locator('.mp-shp-lbl').textContent(), evil);
  assert.equal(await p.locator('.mp-shpbar input[type=text]').inputValue(), evil);
  assert.equal(await p.locator('.mp-ann-bubble-text').textContent(), evil);
  assert.equal(await p.evaluate(() => document.querySelectorAll('#map-wrap img, #map-wrap script').length), 0);
  assert.equal(await p.evaluate(() => window.__pwned), undefined);
  // The accessible name of a shape is text too.
  const label = await p.evaluate(() => document.querySelector('.mp-shp-room').getAttribute('aria-label'));
  assert.ok(label.includes(evil));
});

test('the keyboard reaches a shape: Tab to it, Enter picks it, arrows move, Shift resizes', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  const el = p.locator('.mp-shp-room');
  assert.equal(await el.getAttribute('tabindex'), '0');
  assert.equal(await el.getAttribute('role'), 'button');
  assert.equal(await el.getAttribute('aria-pressed'), 'false');
  await el.focus();
  await p.keyboard.press('Enter');
  await settle(p);
  assert.ok(await selected(p));
  assert.equal(await p.locator('.mp-shp-room').getAttribute('aria-pressed'), 'true');
  assert.match(await p.locator('.mp-shp-hint').textContent(), /Room selected\. Arrow keys move it/);
  assert.ok(await focusedIs(p, '.mp-shp-room'), 'focus stays on the shape');
  await p.keyboard.press('ArrowRight');
  await p.keyboard.press('ArrowRight');
  await p.keyboard.press('Shift+ArrowDown');
  // One request for the whole burst.
  await p.waitForFunction(() => window.__calls.some((c) => c.method === 'PUT'));
  await settle(p);
  const puts = await calls(p, 'PUT');
  assert.equal(puts.length, 1, 'a held key is one save');
  assert.deepEqual(puts[0].body, { points: [{ x: 20, y: 10 }, { x: 40, y: 30 }] });
  assert.ok(await focusedIs(p, '.mp-shp-room'), 'focus survives the redraw');
  await p.keyboard.press('Escape');
  await p.evaluate(() => window.chronicleMap.draw.escape());
  assert.equal(await selected(p), false, 'Escape lets go');
});

test('a click on empty map lets go of the shape; a click on the shape itself does not', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  assert.ok(await selected(p));
  await click(p, 20, 17);
  await settle(p);
  assert.ok(await selected(p), 'clicking the selected shape keeps it');
  await click(p, 70, 80);
  await settle(p);
  assert.equal(await selected(p), false);
  assert.equal(await barVisible(p), false);
});

test('typing in a field never nudges the shape', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  await p.locator('.mp-shpbar input[type=text]').focus();
  await p.keyboard.press('ArrowLeft');
  await p.keyboard.press('ArrowRight');
  await settle(p);
  await p.waitForTimeout(450);
  assert.equal(await calls(p, 'PUT').then((c) => c.length), 0);
});

test('delete asks first, then removes the shape; Done and Escape only let go', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  await p.locator('.mp-shpbar button', { hasText: 'Done' }).click();
  assert.equal(await selected(p), false);
  assert.equal(await barVisible(p), false);
  await click(p, 20, 17);
  await p.locator('.mp-shpbar .mp-shp-del').click();
  await p.locator('.mp-ann-pop .mp-cbtn-danger').click();
  await p.waitForFunction(() => window.__store.length === 0);
  assert.equal((await calls(p, 'DELETE')).length, 1);
  await settle(p);
  assert.equal(await p.locator('.mp-shp-room').count(), 0);
  assert.equal(await selected(p), false);
  assert.equal(await barVisible(p), false);
});

test('deleteSelected is what the Delete key calls: it is true only with a shape selected', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  assert.equal(await p.evaluate(() => window.chronicleMap.draw.deleteSelected()), false);
  await click(p, 20, 17);
  assert.equal(await p.evaluate(() => window.chronicleMap.draw.deleteSelected()), true);
  assert.equal(await p.locator('.mp-ann-pop').count(), 1, 'the confirm opens');
});

test('a refused save puts the shape back where it was', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  const before = await p.locator('.mp-shp-room').boundingBox();
  await p.evaluate(() => { window.__fail = 1; });
  await drag(p, [20, 17], [50, 50]);
  await p.waitForFunction(() => window.__notes.length > 0);
  await settle(p);
  const after = await p.locator('.mp-shp-room').boundingBox();
  assert.ok(Math.abs(before.x - after.x) < 1 && Math.abs(before.y - after.y) < 1, 'back in place');
  assert.deepEqual((await store(p))[0].points, ROOM.points);
});

test('hiding a shape from players sends only visibility', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  await p.locator('.mp-shpbar input[type=checkbox]').check();
  await p.waitForFunction(() => window.__calls.some((c) => c.method === 'PUT'));
  assert.deepEqual((await last(p, 'PUT')).body, { visibility: 'dm_only' });
});

test('a player sees the shapes but cannot pick one up', { skip }, async (t) => {
  const p = await open(t, { scribe: false, owner: false, rows: [ROOM] });
  await p.evaluate(() => window.chronicleMap.draw.pictureSelectMode(true));
  assert.equal(await p.locator('.mp-shp-room').count(), 1, 'the room is drawn');
  assert.equal(await p.locator('.mp-shp-lbl').textContent(), 'Great hall');
  assert.equal(await p.locator('.mp-shp-room').getAttribute('tabindex'), null);
  await click(p, 20, 17);
  await settle(p);
  assert.equal(await selected(p), false);
  assert.equal(await barVisible(p), false);
  assert.equal(await calls(p, 'PUT').then((c) => c.length), 0);
});

test('a scribe may not pick up another person\'s shape', { skip }, async (t) => {
  const p = await open(t, { owner: false, rows: [{ ...ROOM, created_by: 'someone-else' }] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  await click(p, 20, 17);
  await settle(p);
  assert.equal(await selected(p), false, 'the server would refuse a write, so none is offered');
});

test('no tool but Select picks shapes up', { skip }, async (t) => {
  const p = await open(t, { rows: [ROOM] });
  await startPalette(p, 'area');
  await p.evaluate(() => window.chronicleMap.draw.cancel());
  await p.evaluate(() => window.chronicleMap.draw.pictureSelectMode(false));
  await click(p, 20, 17);
  await settle(p);
  assert.equal(await selected(p), false);
  assert.equal(await p.locator('.mp-shp-room').getAttribute('tabindex'), null);
});

test('a blank map shows its sheet, and shapes sit on it in both themes', { skip }, async (t) => {
  for (const dark of [false, true]) {
    const p = await open(t, { dark, rows: [ROOM, { ...ROOM, id: 'w1', drawing_type: 'water', points: [{ x: 50, y: 10 }, { x: 80, y: 40 }], text_content: 'Mere' }, { ...ROOM, id: 'k1', drawing_type: 'wall', points: [{ x: 10, y: 60 }, { x: 60, y: 62.5 }], text_content: undefined }] });
    const sheet = await p.evaluate(() => { const e = document.querySelector('.mp-sheet'); const s = getComputedStyle(e); return { fill: s.fill, stroke: s.stroke }; });
    assert.equal(sheet.fill, dark ? 'rgb(31, 41, 55)' : 'rgb(255, 255, 255)');
    const looks = await p.evaluate(() => ['room', 'water', 'wall'].map((k) => { const s = getComputedStyle(document.querySelector('.mp-shp-' + k)); return [k, s.fill, s.fillOpacity]; }));
    for (const [k, fill, op] of looks) assert.ok(fill && fill !== 'none' && Number(op) > 0, `${k} is filled (${fill} ${op})`);
    const wallFill = looks.find((l) => l[0] === 'wall')[1];
    assert.equal(wallFill, dark ? 'rgb(249, 250, 251)' : 'rgb(17, 24, 39)', 'a wall follows the text colour of the theme');
  }
});

test('the placed shape appears with a short fade that reduced motion switches off', { skip }, async (t) => {
  const p = await open(t, { reducedMotion: true });
  await startPalette(p, 'room');
  await click(p, 21, 19);
  await p.waitForFunction(() => window.__store.length === 1);
  await settle(p);
  const bar = await p.evaluate(() => getComputedStyle(document.querySelector('.mp-shpbar')).animationName);
  assert.equal(bar, 'none');
  const rect = await p.evaluate(() => getComputedStyle(document.querySelector('.mp-shp-room')).animationName);
  assert.equal(rect, 'none');
});

test('a hand-made shape and a sketch-made one are the same record', { skip }, async (t) => {
  // A record as the AI map sketch will create it: only the fields the contract
  // names, no rail colours. The viewer draws it and it can be picked up.
  const sketch = { id: 's1', map_id: 'm1', drawing_type: 'area', points: [{ x: 5, y: 5 }, { x: 45, y: 35 }], stroke_color: '#000000', stroke_width: 2, fill_alpha: 0, text_content: 'Marsh', visibility: 'everyone', created_by: 'u1' };
  const p = await open(t, { rows: [sketch] });
  await p.evaluate(() => window.chronicleMap.setTool('move'));
  assert.equal(await p.locator('.mp-shp-area').count(), 1);
  await click(p, 25, 20);
  await settle(p);
  assert.ok(await selected(p));
  assert.equal(await p.locator('.mp-shpbar input[type=text]').inputValue(), 'Marsh');
});
