// guest_code_preview.spec.mjs — guest view as a visitor with a guest code.
//
// The owner of a private campaign starts the guest-code view from Home's
// guests note and sees what someone who came in on a guest code sees: the
// Characters page offers the try-out, and the try-out opens the hero
// builder. Back to your view ends it.
//
// RUN: tools/live-checks.sh test/live/guest_code_preview.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, enableAddons, done, picture } from './_live.mjs';

after(done);

test('the owner sees the try-out as a visitor with a guest code, then goes back', async () => {
  const owner = await person({ tag: 'gv-owner' });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Guest code view');
  await enableAddons(owner, cid, '*');

  await owner.go(`/campaigns/${cid}?dashboard=guests`);
  await Promise.all([page.waitForURL(new RegExp(`/campaigns/${cid}$`)), page.getByRole('button', { name: 'See it as a guest with a code' }).click()]);
  await page.waitForSelector('[data-guest-preview="guestcode"]');
  assert.match(await page.locator('[data-guest-preview]').innerText(), /Viewing as a visitor with a guest code/);
  assert.equal(await page.locator('[data-guest-preview] [role="group"]').count(), 0, 'a private campaign has no no-code visitor to switch to');

  await owner.go(`/campaigns/${cid}/characters`);
  const tryIt = page.getByRole('link', { name: 'Try the hero builder' });
  await tryIt.waitFor();
  await picture(page, 'guest-code-characters');
  await Promise.all([page.waitForURL(new RegExp(`/campaigns/${cid}/characters/try$`)), tryIt.click()]);
  await page.locator('.hc-app').waitFor();
  assert.ok(await page.locator('[data-guest-preview="guestcode"]').count(), 'still in the guest-code view on the try-out');

  await Promise.all([page.waitForLoadState('load'), page.getByRole('button', { name: 'Back to your view' }).click()]);
  await page.waitForFunction(() => !document.querySelector('[data-guest-preview]'));
  await owner.go(`/campaigns/${cid}/characters`);
  assert.equal(await page.getByRole('link', { name: 'Try the hero builder' }).count(), 0, 'the owner\'s own view has no try-out');
  owner.assertClean();
});

test('on a public campaign the strip switches between the two visitors, on a phone too', async () => {
  const owner = await person({ tag: 'gv-public' });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Open Table');
  await enableAddons(owner, cid, '*');
  const pub = await owner.api(`/campaigns/${cid}`, { method: 'PUT', form: { name: 'Open Table', is_public: 'true' } });
  assert.ok(pub.ok, 'made public: ' + pub.status);
  await page.setViewportSize({ width: 390, height: 844 });

  await owner.go(`/campaigns/${cid}`);
  await Promise.all([page.waitForURL(new RegExp(`/campaigns/${cid}$`)), page.getByRole('button', { name: 'Guests' }).click()]);
  const strip = page.locator('[data-guest-preview]');
  await page.waitForSelector('[data-guest-preview="guest"]');
  await owner.go(`/campaigns/${cid}/characters`);
  assert.equal(await page.getByRole('link', { name: 'Try the hero builder' }).count(), 0, 'a visitor with no code has no try-out');
  const code = strip.getByRole('button', { name: 'Guest code' });
  for (const b of [code, strip.getByRole('button', { name: 'No code' })]) {
    const box = await b.boundingBox();
    assert.ok(box && box.x >= 0 && box.x + box.width <= 390, 'the switch fits the phone');
  }
  await Promise.all([page.waitForLoadState('load'), code.click()]);
  await page.waitForSelector('[data-guest-preview="guestcode"]');
  assert.equal(await strip.getByRole('button', { name: 'Guest code' }).getAttribute('aria-pressed'), 'true');
  await page.getByRole('link', { name: 'Try the hero builder' }).waitFor();
  await picture(page, 'guest-code-switch-phone');
  owner.assertClean();
});

test('on a phone the private note\'s actions are each tapped, never the one beside them', async () => {
  for (const width of [390, 320]) {
    const owner = await person({ tag: 'gv-phone-' + width, context: { viewport: { width, height: 800 }, hasTouch: true, isMobile: true } });
    const { page } = owner;
    const cid = await newCampaign(owner, 'Pocket table');
    await owner.go(`/campaigns/${cid}?dashboard=guests`);
    await page.getByRole('button', { name: 'See it as a guest with a code' }).waitFor();
    // Every action answers at its own middle and ends.
    const hits = await page.evaluate(() => {
      const names = ['See it as a guest with a code', 'Who can read it', 'Arrange this page'];
      const els = [...document.querySelectorAll('.cb-home-preview button, .cb-home-preview a')].filter((el) => names.includes(el.textContent.trim()));
      return els.map((el) => {
        const r = el.getBoundingClientRect(), y = r.top + r.height / 2;
        return [r.left + 2, r.left + r.width / 2, r.right - 2].map((x) => { const at = document.elementFromPoint(x, y); return at === el || el.contains(at); });
      });
    });
    assert.ok(hits.length >= 3, 'the note has its three actions');
    assert.ok(hits.every((h) => h.every(Boolean)), width + ': no action covers another ' + JSON.stringify(hits));
    await picture(page, 'guest-code-note-phone-' + width);
    await Promise.all([page.waitForURL(new RegExp(`/campaigns/${cid}$`)), page.getByRole('button', { name: 'See it as a guest with a code' }).tap()]);
    await page.waitForSelector('[data-guest-preview="guestcode"]');
    owner.assertClean();
    await owner.ctx.close();
  }
});
