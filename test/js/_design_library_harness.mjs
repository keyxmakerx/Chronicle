// _design_library_harness.mjs — the shop room in Arrange, with the saved-designs
// library behind a small in-page fake of the /designs routes, so the browser
// tests (and the screenshot script) drive the real widgets against known data.
// Needs playwright-core and a Chromium (see _browser_env.mjs).

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, 'static/js/widgets', f), 'utf8');

const sells = (id, name, icon, price, quantity) => ({ id, relationType: 'sells', targetEntityId: 'e' + id, targetEntityName: name, targetEntityType: 'Goods', targetEntityIcon: 'fa-' + icon, metadata: { price, currency: 'gp', quantity } });
export const GOODS = [sells(11, 'Healing Potion', 'flask', 2, 4), sells(13, 'Fever tea', 'leaf', 1, null), sells(15, 'Toolbox', 'box', 4, 2), sells(17, 'Spellbook', 'book', 25, 1)];
const ROOM = { layout: { version: 1, roomType: 'forge', setting: 'room', size: 'm', furniture: 'normal', decorations: 'some', palette: 'oak', seeds: { room: 1, goods: 1, deco: 1 }, look: 'counter', mood: '', effects: null, pieces: [], decor: [], items: {}, portrait: null, lines: [] } };

const HOST = `<!doctype html><html data-motion="calm"><head><style>[hidden] { display: none } body { margin: 0; font-family: system-ui, sans-serif }</style></head><body>
<div id="w" data-widget="shop_room" data-can-arrange="true" data-room-endpoint="/campaigns/c1/armory/shops/s1/room" data-relations-endpoint="/rel" data-shop-name="The Gilded Kettle" data-campaign-url="/campaigns/c1" data-csrf-token="t"></div></body></html>`;

// The in-page fake. It keeps what the routes would keep and records every
// call, so a test can say what the widget asked for.
const FAKE = `
window.__srv = { system: 'drawsteel', pkg: { systemId: 'drawsteel', systemName: 'Draw Steel', you: 'Alice', designs: [] }, mine: [], shared: [], hits: [], preview: null, previewErr: '', calls: [], n: 0,
  share: { code: 'chronicle-design:v1:eyJraW5kIjoicm9vbS10aGVtZSJ9', linkOn: false, linkUrl: '', owners: [] } };
function done(v) { return Promise.resolve(JSON.parse(JSON.stringify(v == null ? {} : v))); }
function refuse(m) { return Promise.reject(new Error(m)); }
window.Chronicle = {
  register: function (n, impl) { if (n === 'shop_room') impl.init(document.getElementById('w')); },
  escapeHtml: function (t) { return String(t).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
  apiFetch: function (url, o) {
    var body = /\\/room$/.test(url) ? ${JSON.stringify(ROOM)} : /\\/buyers$/.test(url) ? { canBuyNow: false, buyers: [] } : ${JSON.stringify(GOODS)};
    return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(body); } });
  },
  apiAction: function (url, o) {
    var s = window.__srv, m = (o && o.method) || 'GET', b = o && o.body, p = url.replace('/campaigns/c1', '').split('?')[0], q = url.split('q=')[1];
    s.calls.push(m + ' ' + p + (b ? ' ' + JSON.stringify(b) : ''));
    var id = (p.match(/^\\/designs\\/([^/]+)/) || [])[1], d = s.mine.filter(function (x) { return x.id === id; })[0];
    if (m === 'GET' && p === '/designs') return done({ designs: s.mine, systemId: s.system });
    if (m === 'GET' && p === '/designs/package') return s.pkgErr ? refuse(s.pkgErr) : done(s.pkg);
    if (m === 'POST' && /^\\/designs\\/package\\/[^/]+\\/copy$/.test(p)) {
      var pd = s.pkg.designs.filter(function (x) { return x.id === p.split('/')[3]; })[0];
      if (!pd) return refuse('that design was not found');
      var pc = { id: 'd' + (++s.n), kind: pd.kind, name: pd.name, note: pd.note || '', systemId: s.system, makerName: pd.makerName || s.pkg.systemName, data: pd.data, origin: 'package', shared: false }; s.mine.unshift(pc); return done(pc);
    }
    if (m === 'POST' && /^\\/designs\\/[^/]+\\/suggestion$/.test(p)) {
      if (s.suggestErr) return refuse(s.suggestErr);
      var sd = s.mine.filter(function (x) { return x.id === p.split('/')[2]; })[0];
      return done({ filename: 'drawsteel-design-' + sd.name.toLowerCase().replace(/[^a-z0-9]+/g, '-') + '.json', suggestion: { format: 'chronicle-design-suggestion:v1', system: s.system, designs: [{ kind: sd.kind, name: sd.name, maker: b.credit ? s.pkg.you : undefined, file: 'designs/x.json' }], data: { 'designs/x.json': sd.data }, messageForMaintainer: b.note || undefined } });
    }
    if (m === 'GET' && p === '/designs/shared-with-me') return done({ designs: s.shared });
    if (m === 'POST' && p === '/designs') { var n = { id: 'd' + (++s.n), kind: b.kind, name: b.name, note: '', systemId: s.system, makerName: 'Alice', data: b.data, origin: 'made', shared: false }; s.mine.unshift(n); return done(n); }
    if (m === 'GET' && p === '/designs/owners') return done({ owners: s.hits.filter(function (h) { return h.displayName.toLowerCase().indexOf(decodeURIComponent(q).toLowerCase()) >= 0; }) });
    if (m === 'POST' && p === '/designs/code/preview') return s.previewErr ? refuse(s.previewErr) : done(s.preview);
    if (m === 'POST' && p === '/designs/code/take') { var c = { id: 'd' + (++s.n), kind: 'room-theme', name: s.preview.name, note: '', systemId: s.system, makerName: s.preview.makerName, data: s.takeData, origin: 'code', shared: false }; s.mine.unshift(c); return done(c); }
    if (m === 'GET' && /^\\/designs\\/shared\\/[^/]+\\/preview$/.test(p)) return done(s.preview);
    if (m === 'POST' && /^\\/designs\\/shared\\/[^/]+\\/take$/.test(p)) { var t = { id: 'd' + (++s.n), kind: 'room-theme', name: s.preview.name, note: '', systemId: s.system, makerName: s.preview.makerName, data: s.takeData, origin: 'shared', shared: false }; s.mine.unshift(t); return done(t); }
    var gal = s.gal = s.gal || { own: [], cards: [], status: 'visible', reasons: [] }, gp = p.split('/');
    if (p === '/designs/gallery/mine' && m === 'GET') return done({ entries: gal.own });
    if (p === '/designs/gallery' && m === 'GET') { s.calls.push('QUERY ' + url.split('?')[1]); return done({ entries: gal.cards, total: gal.total == null ? gal.cards.length : gal.total, page: Number((url.split('page=')[1] || '1').split('&')[0]), perPage: 12, systemId: s.system }); }
    if (gp[2] === 'gallery' && gp[4] === 'copy') { var cc = gal.cards.filter(function (x) { return x.id === gp[3]; })[0]; var cp = { id: 'd' + (++s.n), kind: cc.kind, name: cc.name, note: '', systemId: s.system, makerName: cc.makerName, data: s.takeData, origin: 'gallery', shared: false }; s.mine.unshift(cp); return done(cp); }
    if (gp[2] === 'gallery' && gp[4] === 'report') return gal.reportErr ? refuse(gal.reportErr) : done({});
    if (gp[2] === 'gallery' && m === 'DELETE') { gal.own = gal.own.filter(function (x) { return x.id !== gp[3]; }); return done({}); }
    if (gp[3] === 'gallery' && (m === 'POST' || m === 'PUT')) {
      if (gal.refuse) return refuse(gal.refuse);
      var src = s.mine.filter(function (x) { return x.id === gp[2]; })[0], old = gal.own.filter(function (x) { return x.sourceDesignId === gp[2]; })[0];
      var ent = old || { id: 'g' + (++s.n), sourceDesignId: gp[2], kind: src.kind, systemId: src.systemId, reasons: [] };
      ent.name = b.name; ent.note = b.note || ''; ent.makerName = b.makerName || 'Alice'; ent.status = gal.status; ent.reasons = gal.status === 'visible' ? [] : gal.reasons;
      if (!old) gal.own.unshift(ent);
      return done(ent);
    }
    if (!d) return refuse('that design was not found');
    if (m === 'PATCH') { if (b.name != null) d.name = b.name; return done(d); }
    if (m === 'DELETE' && p === '/designs/' + id) { s.mine = s.mine.filter(function (x) { return x !== d; }); return done({}); }
    if (m === 'GET' && p === '/designs/' + id + '/share') return done(s.share);
    if (m === 'PUT' && /\\/share\\/link$/.test(p)) { s.share.linkOn = b.on; s.share.linkUrl = b.on ? '/designs/link/tok' + (++s.n) : ''; d.shared = b.on || s.share.owners.length > 0; return done(s.share); }
    if (m === 'POST' && /\\/share\\/owners$/.test(p)) { if (s.refuseOwner) return refuse(s.refuseOwner); s.share.owners.push({ userId: b.userId, name: b.userId }); d.shared = true; return done(s.share); }
    if (m === 'DELETE' && /\\/share\\/owners\\//.test(p)) { s.share.owners = s.share.owners.filter(function (x) { return x.userId !== decodeURIComponent(p.split('/').pop()); }); return done(s.share); }
    if (m === 'DELETE' && /\\/share$/.test(p)) { s.share.linkOn = false; s.share.linkUrl = ''; s.share.owners = []; d.shared = false; return done(s.share); }
    return refuse('unexpected ' + m + ' ' + p);
  }
};`;

// openArranged returns a page with the shop open in Arrange at the counter room
// section; seed sets the fake's state before the widget loads its library.
export async function openArranged(browser, { width = 1200, height = 800, seed = {} } = {}) {
  const page = await browser.newPage({ viewport: { width, height } });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setContent(HOST);
  await page.addScriptTag({ content: FAKE + '\nObject.assign(window.__srv, ' + JSON.stringify(seed) + ');' });
  for (const f of ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room_counter.js', 'design_library.js', 'design_library_gallery.js', 'design_library_suggest.js', 'shop_room_builder.js', 'shop_room.js']) await page.addScriptTag({ content: read(f) });
  await page.waitForSelector('#w:not([hidden]) .shr-front');
  await page.click('.shr-front');
  await page.waitForSelector('.src-svg [data-k="keeper"]');
  await page.click('[data-mode="arr"]');
  await page.waitForSelector('.shr-cb [data-dl] .dl');
  return { page, errors };
}

export async function launch() {
  return pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
}
