// _design_gallery_admin_harness.mjs — the admin gallery queue widget in a bare
// page with the site stylesheet, against an in-page fake of the admin routes,
// for the browser test and the screenshot script.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

export const entry = (id, name, extra) => Object.assign({ id, kind: 'room-theme', systemId: 'drawsteel', name, note: 'A warm room.', makerName: 'Sunniva Ostrander', summary: 'wall: stone · props: 1',
  status: 'pending', reasonCodes: ['link'], reasons: ['The text contains a web address or invite link.'], reports: [], publishedAt: '2026-11-01T10:00:00Z' }, extra);

const FAKE = `
window.__srv = { pending: [], visible: [], calls: [], refuse: '' };
window.Chronicle = {
  register: function (n, impl) { window.__impl = impl; },
  escapeHtml: function (t) { return String(t).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
  apiAction: function (url, o) {
    var s = window.__srv, m = (o && o.method) || 'GET';
    s.calls.push(m + ' ' + url);
    if (m === 'GET') {
      var st = url.indexOf('status=visible') >= 0 ? 'visible' : 'pending', list = s[st];
      return Promise.resolve(JSON.parse(JSON.stringify({ entries: list, total: list.length, page: 1, perPage: 12, status: st })));
    }
    if (s.refuse) return Promise.reject(new Error(s.refuse));
    var id = url.split('/')[3], act = url.split('/')[4];
    var e = s.pending.concat(s.visible).filter(function (x) { return x.id === id; })[0];
    s.pending = s.pending.filter(function (x) { return x.id !== id; }); s.visible = s.visible.filter(function (x) { return x.id !== id; });
    if (act === 'approve') { e.status = 'visible'; s.visible.push(e); }
    return Promise.resolve({});
  }
};`;

export async function openAdmin(browser, { width = 1100, height = 800, dark = false, seed = {} } = {}) {
  const page = await browser.newPage({ viewport: { width, height }, colorScheme: dark ? 'dark' : 'light' });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setContent('<!doctype html><html' + (dark ? ' class="dark" data-theme="dark"' : '') + '><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body class="bg-page text-fg" style="margin:0;padding:16px"><div style="max-width:56rem;margin:0 auto"><h1 class="text-2xl font-bold text-fg">Shared designs</h1><p class="text-sm text-fg-secondary mt-1 mb-4">Designs Owners shared to this server\'s gallery. Clean ones show straight away; the ones here were held for a look, or reported by other Owners.</p><div id="r"></div></div></body></html>');
  await page.addStyleTag({ content: readFileSync(join(root, 'static/css/app.css'), 'utf8') });
  await page.addScriptTag({ content: FAKE + '\nObject.assign(window.__srv, ' + JSON.stringify(seed) + ');' });
  await page.addScriptTag({ content: readFileSync(join(root, 'static/js/widgets/design_gallery_admin.js'), 'utf8') });
  await page.evaluate(() => window.__impl.init(document.getElementById('r')));
  return { page, errors };
}
