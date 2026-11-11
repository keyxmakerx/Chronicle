// admin_site_look.spec.mjs — Admin > Site look: name, logo, look and sign-in
// background save, show on the signed-out sign-in page and the browser tab,
// bad uploads are refused with a red message, and a new campaign starts with
// the site's look while an older one does not. Stands in for the operator
// check in #1028.
//
// Covers steps 1 to 4, 6 and 7, and the Move slowly flag of step 5 (the page
// carries the drifting stage; that it settles after a few idle seconds and
// never moves under "reduce motion" is motion a person has to watch). The
// live preview of step 3 is checked for the name and the saved message; the
// preview's look is a picture for approval.
//
// The site look is shared by every check on the instance, so the page is
// read before anything changes and the same values are saved back at the end.
// Saving marks the site as configured for good, so a site that had never
// saved keeps the saved defaults (name Chronicle, plain background) rather
// than the shipped book icon; a person can reset that only in the database.
//
// RUN: tools/live-checks.sh test/live/admin_site_look.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { deflateSync } from 'node:zlib';
import { siteAdmin, person, newCampaign, done, picture } from './_live.mjs';

after(done);

// A solid-colour PNG made in memory: valid signature, header, one data
// chunk and an end chunk, with real CRCs.
function crc32(buf) {
  let c, crc = 0xffffffff;
  for (let n = 0; n < buf.length; n++) {
    c = (crc ^ buf[n]) & 0xff;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    crc = (crc >>> 8) ^ c;
  }
  return (crc ^ 0xffffffff) >>> 0;
}
function chunk(type, data) {
  const len = Buffer.alloc(4); len.writeUInt32BE(data.length);
  const body = Buffer.concat([Buffer.from(type), data]);
  const crc = Buffer.alloc(4); crc.writeUInt32BE(crc32(body));
  return Buffer.concat([len, body, crc]);
}
function png(size, [r, g, b]) {
  const head = Buffer.alloc(13);
  head.writeUInt32BE(size, 0); head.writeUInt32BE(size, 4);
  head[8] = 8; head[9] = 2; // 8-bit RGB
  const row = Buffer.concat([Buffer.from([0]), Buffer.from(Array(size).fill([r, g, b]).flat())]);
  const raw = Buffer.concat(Array(size).fill(row));
  return Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', head), chunk('IDAT', deflateSync(raw)), chunk('IEND', Buffer.alloc(0))]);
}

/** The values now on the form, in the shape `fill` takes back. */
async function readForm(page) {
  return page.evaluate(() => {
    const f = document.querySelector('form[action="/admin/site-look"]');
    const val = (n) => f.querySelector(`[name=${n}]`).value;
    const radio = (n) => (f.querySelector(`[name=${n}]:checked`) || {}).value || '';
    const box = (n) => !!f.querySelector(`[name=${n}]`).checked;
    return { name: val('name'), look: radio('look'), background: radio('background'), welcome: val('welcome'), move: box('move'), favicon: box('favicon') };
  });
}

async function fill(page, v) {
  const f = page.locator('form[action="/admin/site-look"]');
  await f.locator('[name=name]').fill(v.name);
  await f.locator(`[name=look][value="${v.look}"]`).check();
  await f.locator(`[name=background][value="${v.background}"]`).check();
  await f.locator('[name=welcome]').fill(v.welcome);
  const set = async (n, on) => { const el = f.locator(`[name=${n}]`); if (await el.isEnabled()) await el.setChecked(on); };
  await set('move', v.move);
  await set('favicon', v.favicon);
}

const save = async (page) => {
  await Promise.all([page.waitForURL(/site-look\?saved=1/), page.getByRole('button', { name: 'Save', exact: true }).click()]);
  await page.getByText(/^Saved\./).waitFor();
};

test('Site look saves, shows when signed out, refuses bad uploads, seeds new campaigns', async () => {
  const admin = await siteAdmin();
  const { page } = admin;
  const owner = await person({ tag: 'sitelook', name: 'Mara Vell' });
  const oldCid = await newCampaign(owner, 'Before the look');

  await admin.go('/admin/site-look');
  const original = await readForm(page);
  const stranger = await person({ signIn: false });

  try {
    // Step 2: the form and its preview are there.
    await page.locator('[data-sl-win]').waitFor();
    await page.getByRole('button', { name: 'Save', exact: true }).waitFor();

    // Step 3: pick values; the preview follows the name as it is typed.
    const NAME = 'Ember Hold';
    await fill(page, { name: NAME, look: 'ember', background: 'look', welcome: 'Welcome back, traveller', move: false, favicon: true });
    await page.locator('input[name=logo_file]').setInputFiles({ name: 'logo.png', mimeType: 'image/png', buffer: png(32, [194, 65, 12]) });
    await page.locator('[data-sl-win]').getByText(NAME).first().waitFor();
    await page.evaluate(() => window.scrollTo(0, 0));
    await picture(page, 'admin-site-look-form', {
      check: '#1028', about: 'Site look form with its preview, before saving',
      mask: ['input[name=logo_file]'],
    });
    await save(page);
    const saved = await readForm(page);
    assert.equal(saved.name, NAME);
    assert.equal(saved.look, 'ember');
    assert.equal(saved.background, 'look');

    // Step 4: signed out, the sign-in page shows the name, logo and colours.
    await stranger.go('/login');
    const sp = stranger.page;
    assert.match(await sp.title(), new RegExp(NAME));
    await sp.getByText(NAME).first().waitFor();
    await sp.getByText('Welcome back, traveller').waitFor();
    assert.ok(await sp.locator('a[aria-label="Ember Hold"] img').count() > 0, 'the logo is on the sign-in card');
    const icon = await sp.locator('link[rel=icon]').first().getAttribute('href');
    assert.match(icon, /^\/media\/|^\/static\/media|media/, 'the browser tab icon is the uploaded logo: ' + icon);
    assert.doesNotMatch(icon, /favicon\.svg/);
    const bg = await sp.evaluate(() => [...document.querySelectorAll('div')].map((d) => d.style.background).find((b) => b.includes('linear-gradient')) || '');
    assert.match(bg, /rgb\(31, 41, 55\)|#1f2937/i, 'the stage carries the look\'s colours');
    await picture(sp, 'admin-site-look-signin', {
      check: '#1028', about: 'Signed-out sign-in page in the Ember look with name, logo and welcome line',
    });

    // Step 5: Move slowly adds the drifting stage.
    await admin.go('/admin/site-look');
    await page.locator('input[name=move]').check();
    await save(page);
    await stranger.go('/login');
    assert.equal(await sp.locator('[data-widget="header-motion"]').count(), 1, 'the sign-in stage drifts');

    // Step 6: an SVG and an oversize file are refused; nothing else changes.
    await admin.go('/admin/site-look');
    const svg = Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8"/></svg>');
    await page.locator('input[name=picture_file]').setInputFiles({ name: 'p.svg', mimeType: 'image/svg+xml', buffer: svg });
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    const alert = page.getByRole('alert').first();
    await alert.waitFor();
    assert.ok((await alert.innerText()).length > 10, 'the red message explains why');
    assert.match(page.url(), /site-look$/, 'stays on the form');

    await admin.go('/admin/site-look');
    const big = Buffer.concat([png(8, [1, 2, 3]), Buffer.alloc(3 * 1024 * 1024 + 4096)]);
    await page.locator('input[name=picture_file]').setInputFiles({ name: 'big.png', mimeType: 'image/png', buffer: big });
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await page.getByRole('alert').first().waitFor();
    assert.match(await page.getByRole('alert').first().innerText(), /too large/i);
    await admin.go('/admin/site-look');
    assert.equal((await readForm(page)).name, NAME, 'nothing else changed');

    // Step 7: a new campaign starts in the site's look; the old one is as it was.
    const newCid = await newCampaign(owner, 'After the look');
    await owner.go(`/campaigns/${newCid}`);
    assert.equal(await owner.page.evaluate(() => document.documentElement.getAttribute('data-cz-heading')), 'cinzel', 'the new campaign has Ember\'s heading face');
    await owner.go(`/campaigns/${oldCid}`);
    assert.equal(await owner.page.evaluate(() => document.documentElement.getAttribute('data-cz-heading')), null, 'the older campaign is unchanged');
  } finally {
    // Put the site back as it was found.
    await admin.go('/admin/site-look');
    await fill(page, original);
    await page.locator('input[name=remove_logo]').check().catch(() => {});
    await page.locator('input[name=remove_picture]').check().catch(() => {});
    await save(page);
  }
  admin.assertClean();
});
