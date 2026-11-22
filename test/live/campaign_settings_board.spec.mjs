// campaign_settings_board.spec.mjs — Campaign settings as the owner sees it:
// the banner and the tabs on every page, the Overview board's flips and
// feature chips saving where they stand, the Campaign tab's fields and the
// card that follows them, Connections and Data, old addresses landing on
// their new tab, the Danger zone's delete asking for the name, and every
// control 44 tall on a phone. Players and scribes never get the tabs.
//
// Set LIVE_SHOTS to a folder to also save pictures of the four tabs there,
// light and dark, on a desktop and a phone.
//
// RUN: tools/live-checks.sh test/live/campaign_settings_board.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { person, newCampaign, addMember, uniqueEmail, done, BASE } from './_live.mjs';

after(done);

const SHOTS = process.env.LIVE_SHOTS || '';

// Waits for the write `act` sends to a path ending in `path`, and returns it.
async function written(page, method, path, act) {
  const [res] = await Promise.all([
    page.waitForResponse((r) => r.request().method() === method && new URL(r.url()).pathname.endsWith(path)),
    act(),
  ]);
  return res;
}

test('the board flips who can see the campaign and switches a feature where they stand', async () => {
  const owner = await person({ tag: 'board-owner', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Saltreach docks');
  const page = owner.page;

  await owner.go(`/campaigns/${cid}/dashboard`);
  await page.locator('[data-settings-mast]').waitFor();
  assert.equal(await page.locator('.stg-name').textContent(), 'Saltreach docks');
  assert.equal(await page.locator('.stg-tab[aria-current="page"]').textContent(), 'Overview');
  assert.deepEqual(await page.locator('.stg-tab').allTextContents(), ['Overview', 'Campaign', 'People', 'Game', 'Connections', 'Data']);

  // The Campaign flip: the word changes at once, the server keeps it, and the
  // banner's pill follows without a reload.
  const access = page.locator('#line-access');
  assert.equal(await access.getAttribute('aria-pressed'), 'false');
  const res = await written(page, 'PUT', '/details', () => access.click());
  assert.ok(res.ok(), `the flip's save answered ${res.status()}`);
  assert.equal(await access.getAttribute('aria-pressed'), 'true');
  assert.equal(await access.locator('[data-sb-word]').textContent(), 'Public');
  assert.equal((await page.locator('.stg-mast [data-settings-access]').textContent()).trim(), 'Public');
  const stored = await owner.api(`/campaigns/${cid}`, { headers: { Accept: 'application/json' } });
  await owner.go(`/campaigns/${cid}/dashboard`);
  assert.equal(await page.locator('#line-access').getAttribute('aria-pressed'), 'true', 'Public is kept: ' + JSON.stringify(stored.status));

  // The keyboard reaches the New pages flip and Space flips it.
  const pages = page.locator('#line-pages');
  const before = await pages.getAttribute('aria-pressed');
  await pages.focus();
  const res2 = await written(page, 'PUT', '/default-visibility', () => page.keyboard.press('Space'));
  assert.ok(res2.ok());
  assert.notEqual(await pages.getAttribute('aria-pressed'), before);

  // A feature chip switches its add-on, and the count follows.
  const chip = page.locator('[data-sb-chip]').first();
  const was = await chip.getAttribute('aria-pressed');
  const id = await chip.getAttribute('data-sb-chip');
  const res3 = await written(page, 'PUT', `/addons/${id}/toggle`, () => chip.click());
  assert.ok(res3.ok(), `the chip's toggle answered ${res3.status()}`);
  const list = (await owner.api(`/campaigns/${cid}/addons`)).body || [];
  const addon = list.find((a) => String(a.addon_id) === id);
  assert.ok(addon, 'the chip names an add-on');
  assert.equal(String(addon.enabled), was === 'true' ? 'false' : 'true', 'the add-on switched');
  owner.assertClean();
});

test('the Campaign tab saves the name and the description when left, and the card follows', async () => {
  const owner = await person({ tag: 'board-campaign', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Lastwell');
  const page = owner.page;
  await owner.go(`/campaigns/${cid}/settings`);
  assert.equal(await page.locator('.stg-tab[aria-current="page"]').textContent(), 'Campaign');

  const name = page.locator('#sb-f-name');
  await name.fill('Lastwell Trials');
  // The first key loads the field's script, so the card follows a moment later.
  await page.waitForFunction(() => document.querySelector('[data-sb-bind="name"]').textContent === 'Lastwell Trials', null, { timeout: 3000 });
  assert.equal(await page.locator('.stg-name').textContent(), 'Lastwell Trials', 'the banner follows the typing');
  const res = await written(page, 'PUT', '/details', () => name.press('Enter'));
  assert.ok(res.ok());
  await page.locator('#line-name .desk-saved').waitFor();

  const desc = page.locator('#sb-f-description');
  await desc.fill('Five trials under the old well.');
  const res2 = await written(page, 'PUT', '/details', () => page.locator('#sb-f-name').focus());
  assert.ok(res2.ok());

  // Escape puts the stored words back without saving.
  await name.fill('Something else');
  await name.press('Escape');
  assert.equal(await name.inputValue(), 'Lastwell Trials');

  await owner.go(`/campaigns/${cid}/settings`);
  assert.equal(await page.locator('#sb-f-name').inputValue(), 'Lastwell Trials');
  assert.equal(await page.locator('#sb-f-description').inputValue(), 'Five trials under the old well.');

  // Who can see it: a press on the other card saves at once.
  const pub = page.locator('#line-public [data-v="true"]');
  const res3 = await written(page, 'PUT', '/details', () => pub.click());
  assert.ok(res3.ok());
  assert.equal(await pub.getAttribute('aria-pressed'), 'true');
  assert.equal((await page.locator('.sb-pcard [data-settings-access]').textContent()).trim(), 'Public');
  owner.assertClean();
});

test('Connections, Data and the old addresses; deleting asks for the name', async () => {
  const owner = await person({ tag: 'board-data', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Gone soon');
  const page = owner.page;

  await owner.go(`/campaigns/${cid}/settings?tab=api-keys`);
  assert.equal(new URL(page.url()).pathname, `/campaigns/${cid}/connections`);
  assert.equal(await page.locator('.stg-tab[aria-current="page"]').textContent(), 'Connections');
  await page.locator('#keys').waitFor();

  await owner.go(`/campaigns/${cid}/settings?tab=activity`);
  assert.equal(new URL(page.url()).pathname, `/campaigns/${cid}/data`);
  assert.equal(await page.locator('.stg-tab[aria-current="page"]').textContent(), 'Data');

  // A tool page under a tab keeps that tab lit.
  await owner.go(`/campaigns/${cid}/trash`);
  assert.equal(await page.locator('.stg-tab[aria-current="page"]').textContent(), 'Data');

  await owner.go(`/campaigns/${cid}/data`);
  await page.locator('#line-delete summary').click();
  const button = page.getByRole('button', { name: 'Delete the campaign' });
  assert.ok(await button.isDisabled(), 'delete is off until the name is typed');
  await page.locator('#sb-delete-name').fill('Gone');
  assert.ok(await button.isDisabled());
  await page.locator('#sb-delete-name').fill('Gone soon');
  const res = await written(page, 'DELETE', `/campaigns/${cid}`, () => button.click());
  assert.ok(res.ok(), `delete answered ${res.status()}`);
  await page.waitForURL((u) => u.pathname === '/campaigns');
  owner.assertClean();
});

test('on a phone every control and field is 44 tall; the keys pages keep the tabs; the log speaks in words', async () => {
  const owner = await person({ tag: 'board-phone', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Narrowmere');
  const page = owner.page;
  const inv = await owner.api(`/campaigns/${cid}/invite-box/email`, { method: 'POST', form: { email: uniqueEmail('board-invitee'), role: 'player' } });
  assert.ok(inv.status < 400, `the invite answered ${inv.status}`);

  await owner.go(`/campaigns/${cid}/api-keys`);
  assert.equal(await page.locator('.stg-tab[aria-current="page"]').textContent(), 'Connections', 'the full keys page sits under Connections');

  await page.setViewportSize({ width: 390, height: 844 });
  for (const tab of ['dashboard', 'settings', 'connections', 'data', 'api-keys']) {
    await owner.go(`/campaigns/${cid}/${tab}`);
    if (tab === 'connections') {
      // A key, so its Active switch and Revoke button are measured, and the
      // create form open, so its fields are.
      await page.locator('#int-key-name').waitFor({ state: 'attached' });
      await page.getByRole('button', { name: 'Create New Key' }).click();
      await page.locator('#int-key-name').fill('Phone key');
      await page.locator('#integrations-keys form[hx-post] button[type="submit"]').click();
      await page.locator('#integrations-keys').getByText('API Key Created').waitFor();
      await owner.go(`/campaigns/${cid}/${tab}`);
      await page.getByRole('button', { name: 'Create New Key' }).click();
      await page.locator('#int-key-name').waitFor();
    }
    if (tab === 'data') {
      for (const s of await page.locator('.sb-step > summary, .sb-fold > summary').all()) await s.click();
      await page.locator('#activity-content').scrollIntoViewIfNeeded();
      await page.getByText('invited someone').first().waitFor();
      assert.equal(await page.locator('#activity-content').getByText('member.invited').count(), 0, 'the log never shows a raw key');
    }
    // Links, buttons, folds and form fields. A check box, and a file picker
    // drawn as a drop area, are answered through their label, so the label
    // is what is measured.
    const short = await page.evaluate(() => {
      const out = [];
      const sel = ['a', 'button', 'summary', 'select', 'textarea', 'input:not([type="hidden"])']
        .flatMap((t) => [`.sb ${t}`, `.stg ${t}`, `.keys-touch ${t}`]).join(', ');
      for (const el of document.querySelectorAll(sel)) {
        const box = el.matches('input[type="checkbox"], input[type="radio"], input[type="file"]') ? (el.closest('label') || el) : el;
        const r = box.getBoundingClientRect();
        if (r.width && r.height && r.height < 44) out.push(`${Math.round(r.height)}px ${el.tagName.toLowerCase()} "${(el.textContent || el.getAttribute('aria-label') || el.name || '').trim().slice(0, 30)}"`);
      }
      return out;
    });
    assert.deepEqual(short, [], `${tab}: controls under 44 tall on a phone`);
  }

  // A key made on the full keys page shows inside the frame, not over it.
  await page.locator('#name').fill('Second key');
  await page.getByRole('button', { name: 'Generate Key' }).click();
  await page.getByText('API Key Created').waitFor();
  assert.equal(await page.locator('.stg-tab[aria-current="page"]').textContent(), 'Connections', 'the new key keeps the tabs');
  owner.assertClean();
});

test('players and scribes get no tabs and no settings pages', async () => {
  const owner = await person({ tag: 'board-gate-owner', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Gate');
  const scribe = await person({ tag: 'board-scribe', name: 'Sam Scribe' });
  const player = await person({ tag: 'board-player', name: 'Pia Player' });
  await addMember(owner, cid, scribe, 'scribe');
  await addMember(owner, cid, player, 'player');
  for (const who of [scribe, player]) {
    for (const path of ['dashboard', 'settings', 'connections', 'data']) {
      const res = await who.go(`/campaigns/${cid}/${path}`);
      assert.equal(res.status(), 403, `${who.name} on ${path}`);
    }
    await who.go(`/campaigns/${cid}`);
    assert.equal(await who.page.locator('.stg-bar').count(), 0, `${who.name} sees no tabs`);
  }
});

test('pictures of the tabs', { skip: !SHOTS }, async () => {
  const owner = await person({ tag: 'board-shots', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Saltreach docks');
  const page = owner.page;
  for (const [view, size] of [['desktop', { width: 1280, height: 1000 }], ['phone', { width: 390, height: 844 }]]) {
    for (const scheme of ['light', 'dark']) {
      await page.setViewportSize(size);
      await page.emulateMedia({ colorScheme: scheme, reducedMotion: 'reduce' });
      for (const tab of ['dashboard', 'settings', 'connections', 'data']) {
        await page.goto(`${BASE}/campaigns/${cid}/${tab}`, { waitUntil: 'networkidle' });
        await page.evaluate((s) => {
          document.documentElement.classList.toggle('dark', s === 'dark');
          try { localStorage.setItem('theme', s); } catch (e) { /* none */ }
        }, scheme);
        await page.waitForTimeout(200);
        await page.screenshot({ path: join(SHOTS, `settings-${tab}-${view}-${scheme}.png`), fullPage: true });
      }
    }
  }
});
