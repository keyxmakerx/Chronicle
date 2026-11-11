// admin_visitor_addresses.spec.mjs — after the Echo v5 change the site still
// sees each visitor's own address (not the proxy's), and files still come
// from disk: a page's picture shows, a page attachment downloads, and a
// backup downloads. Stands in for the operator check in #2099.
//
// Step 1 relies on each check's person arriving with its own X-Forwarded-For
// address from 2001:db8::/32, which the server trusts from loopback just as
// it would from the real reverse proxy; the check is that the newest login
// event and the signed-in list show that address and not 127.0.0.1 or ::1.
// What it cannot show: the real proxy's own address range (172.x or 10.x)
// and that the proxy passes the header on; both need the live site behind
// its reverse proxy.
// Step 2 covers the picture, the attachment, and the backup download (the
// newest file in Backup files, whatever the server has; a backup made by the
// server needs the dump tool and database credentials, so this takes
// whichever file is already listed, and skips if none is).
//
// RUN: tools/live-checks.sh test/live/admin_visitor_addresses.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { createReadStream } from 'node:fs';
import { siteAdmin, person, signIn, newCampaign, newPage, done, ADMIN } from './_live.mjs';

after(done);

// A 1x1 PNG.
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');

// Addresses compare by value: "2001:db8::0:1" and "2001:db8::1" are the same.
const norm = (ip) => ip.toLowerCase().split(':').map((g) => g.replace(/^0+(?=.)/, '')).join(':');

async function confirmReauth(page) {
  const box = page.locator('#reauth-modal');
  const shown = await box.getByText('Confirm Your Identity').waitFor({ state: 'visible', timeout: 4000 }).then(() => true, () => false);
  if (shown) {
    await box.locator('input[name=password]').fill(ADMIN.password);
    await box.getByRole('button', { name: 'Confirm' }).click();
    await box.waitFor({ state: 'hidden' });
  }
}

test('the newest login shows the visitor\'s own address', async () => {
  const name = 'Ines Marlow ' + Math.random().toString(36).slice(2, 7);
  const visitor = await person({ tag: 'visitor', name });
  // Sign out and back in, as the issue says, so the newest event is a login.
  await visitor.go('/account');
  await visitor.page.context().clearCookies();
  await signIn(visitor, { register: false });

  const admin = await siteAdmin();
  const { page } = admin;
  await admin.go('/admin/security?tab=log&type=login.success');
  const rows = page.locator('tbody tr', { hasText: name });
  await rows.first().waitFor();
  const seen = (await rows.first().locator('code').innerText()).trim();
  assert.equal(norm(seen), norm(visitor.ip), `the newest login shows the visitor's address, not the proxy's (saw ${seen})`);
  assert.doesNotMatch(seen, /^(127\.|::1$|172\.|10\.)/);

  // The signed-in list agrees.
  await admin.go('/admin/security?tab=sessions');
  const live = page.locator('tbody tr', { hasText: name }).first();
  await live.waitFor();
  assert.equal(norm((await live.locator('code').first().innerText()).trim()), norm(visitor.ip), 'the signed-in list shows the same address');
});

test('a page picture shows, a page attachment and a backup download', async (t) => {
  const owner = await person({ tag: 'files', name: 'Mara Vell' });
  const cid = await newCampaign(owner, 'Files from disk');
  const eid = await newPage(owner, cid, 'Character', 'Mara Vell');
  const { page } = owner;

  // The picture: upload it, set it as the page's picture, and see it drawn.
  const up = await page.evaluate(async ({ cid, eid, b64 }) => {
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
    const fd = new FormData();
    fd.append('file', new Blob([bytes], { type: 'image/png' }), 'mara.png');
    fd.append('campaign_id', cid); fd.append('usage_type', 'entity_image'); fd.append('entity_id', eid);
    const r = await Chronicle.apiFetch('/media/upload', { method: 'POST', body: fd });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) return { status: r.status, j };
    const s = await Chronicle.apiFetch(`/campaigns/${cid}/entities/${eid}/image`, { method: 'PUT', body: { image_path: j.id } });
    return { status: r.status, set: s.status, j };
  }, { cid, eid, b64: PNG.toString('base64') });
  assert.equal(up.status, 201, 'the picture uploaded: ' + JSON.stringify(up));
  assert.equal(up.set, 200, 'the picture was set on the page');
  const served = await page.evaluate(async (u) => { const r = await fetch(u); return { status: r.status, type: r.headers.get('content-type'), len: (await r.arrayBuffer()).byteLength }; }, up.j.url);
  assert.equal(served.status, 200, 'the uploaded picture is served');
  assert.match(served.type, /image\/png/);
  assert.ok(served.len > 0, 'the picture has content (the server re-encodes uploads, so the size may differ)');
  // The page list draws each page's picture on its card.
  await owner.go(`/campaigns/${cid}/entities`);
  await page.waitForFunction(() => [...document.images].some((i) => /\/media\//.test(i.currentSrc || i.src) && i.complete), null, { timeout: 8000 }).catch(() => {});
  const drawn = await page.evaluate(() => [...document.images].some((i) => /\/media\//.test(i.currentSrc || i.src) && i.complete && i.naturalWidth > 0));
  assert.ok(drawn, 'the page draws the picture (no broken image): ' + JSON.stringify(await page.evaluate(() => [...document.images].map((i) => [i.currentSrc || i.src, i.complete, i.naturalWidth]))));

  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  // The attachment: attach through the Files section, then download it.
  const input = page.locator('[data-pf-input]');
  await input.waitFor({ state: 'attached' });
  await input.setInputFiles({ name: 'handout.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n') });
  const link = page.locator('#page-files a[href$="/download"]').first();
  await link.waitFor();
  const [dl] = await Promise.all([page.waitForEvent('download'), link.click()]);
  let size = 0;
  for await (const c of createReadStream(await dl.path())) size += c.length;
  assert.ok(size > 0, 'the attachment arrived with content');
  owner.assertClean();

  // The backup: download the first file Backup files lists.
  const admin = await siteAdmin();
  await admin.go('/admin/backup');
  const btn = admin.page.getByRole('button', { name: 'Download' }).first();
  if (!(await btn.count())) { t.skip('no backup file is listed on this server'); return; }
  const [bdl] = await Promise.all([admin.page.waitForEvent('download', { timeout: 20000 }), btn.click().then(() => confirmReauth(admin.page))]);
  let bsize = 0;
  for await (const c of createReadStream(await bdl.path())) bsize += c.length;
  assert.ok(bsize > 0, 'the backup file ' + bdl.suggestedFilename() + ' arrived with content');
});
