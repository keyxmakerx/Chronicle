// admin_rework_batch1.spec.mjs — the admin rework on a running site: menu,
// per-admin pins, Home, the password box, tabs and the People list. Stands in
// for the operator check in #878.
//
// Covers steps 1 to 6. The second admin of step 2 is made from Community >
// People by the site admin (which is also step 4's password box), and is made
// an ordinary member again at the end. Step 1's "click a section name and it
// folds" is not checked: the sections are deliberately always open now, and
// the menu's headings are Community, Add-ons, Security,
// Site & data and Tools, so the issue's wording is out of date. Step 2's
// "on another device" is a second browser context as the same admin. Step 4
// uses the password box that Make Admin raises; the database-update and
// delete-campaign buttons it names need a pending update or a campaign to
// delete and go through the same box. Backing up first (the issue's
// preamble) is a deploy step for a person.
//
// RUN: tools/live-checks.sh test/live/admin_rework_batch1.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { siteAdmin, person, ADMIN, done, picture } from './_live.mjs';

after(done);

const NAME = 'Theo Brandt';

/** Make the person found by `key` (their email) an admin the way a site admin does: People, Make Admin, password box. */
async function makeAdmin(admin, key, { sawBox } = {}) {
  const { page } = admin;
  await admin.go('/admin/users?q=' + encodeURIComponent(key));
  const row = page.locator('tbody tr', { hasText: key });
  await row.waitFor();
  page.once('dialog', (d) => d.accept());
  await row.getByRole('button', { name: 'Make Admin' }).click();
  const box = page.locator('#reauth-modal');
  const shown = await box.getByText('Confirm Your Identity').waitFor({ state: 'visible', timeout: 5000 }).then(() => true, () => false);
  if (sawBox) sawBox.shown = shown;
  if (shown) {
    await box.locator('input[name=password]').fill(ADMIN.password);
    await box.getByRole('button', { name: 'Confirm' }).click();
  }
  await page.locator('tbody tr', { hasText: key }).getByText('Admin', { exact: true }).waitFor();
}

test('menu, pins, home, password box, tabs and people list', async () => {
  const admin = await siteAdmin();
  const first = await person({ tag: 'rework', name: NAME });
  const seen = {};
  await makeAdmin(admin, first.email, { sawBox: seen });
  // Granting admin ends the person's sessions, so they sign in again.
  const theo = await person({ email: first.email, password: first.password, register: false });

  try {
    // Step 4: the password box came first, instead of the change just running.
    // (A recent confirmation by this admin would skip it; this admin is fresh.)
    assert.equal(seen.shown, true, 'Make Admin asked for a password first');

    // Step 1: the menu.
    await theo.go('/admin');
    const nav = theo.page.locator('#admin-nav');
    await nav.waitFor();
    const text = await nav.innerText();
    const order = ['Home', 'Community', 'Add-ons', 'Security', 'Site & data', 'Tools'].map((s) => text.toUpperCase().indexOf(s.toUpperCase()));
    assert.ok(order.every((n, i) => n >= 0 && (i === 0 || n > order[i - 1])), 'sections are in order: ' + order);
    assert.ok(!/Content|Infrastructure/.test(text), 'the old Content / Infrastructure headings are gone');
    await picture(theo.page, 'admin-menu', {
      check: '#878', about: 'Admin menu: Home, then the sections, as the issue describes', locator: nav,
    });

    // Step 2: pin People; it appears under Home, survives a reload and shows
    // on another browser of the same admin, and not on a different admin.
    const pin = nav.locator('[data-admin-pin="/admin/users"]');
    await pin.click({ force: true });
    const group = theo.page.getByRole('group', { name: 'Pinned admin pages' });
    await group.getByText('People').waitFor();
    await theo.go('/admin');
    await theo.page.getByRole('group', { name: 'Pinned admin pages' }).getByText('People').waitFor();
    const other = await person({ email: theo.email, password: theo.password, register: false });
    await other.go('/admin');
    await other.page.getByRole('group', { name: 'Pinned admin pages' }).getByText('People').waitFor();
    await admin.go('/admin');
    assert.equal(await admin.page.locator('#admin-nav [data-admin-pin="/admin/users"]').getAttribute('aria-pressed'), 'false', 'a second admin\'s menu has no such pin');

    // Step 3: Home has Needs you, tiles and the change log with the actor's name.
    await theo.go('/admin');
    await theo.page.getByRole('heading', { name: 'Needs you' }).waitFor();
    await theo.page.getByRole('heading', { name: 'Recent admin changes' }).waitFor();
    const recent = theo.page.locator('.card', { has: theo.page.getByRole('heading', { name: 'Recent admin changes' }) });
    await picture(theo.page, 'admin-home', {
      check: '#878', about: 'Admin Home: Needs you card, tiles, Recent admin changes',
      mask: ['main .card ul', 'main p.font-mono', 'main .card p.text-sm'],
    });
    await recent.getByText(new RegExp(`Site Admin made ${NAME} an admin`)).first().waitFor();

    // Step 5: tabs on Health & diagnostics and on Storage & cleanup.
    await theo.go('/admin/systems');
    for (const t of ['Parts of Chronicle', 'Database updates', 'AI helper']) {
      await theo.page.getByRole('link', { name: t }).first().waitFor();
    }
    await theo.go('/admin/storage');
    for (const t of ['Overview & files', 'Limits', 'Cleanup']) {
      await theo.page.getByRole('link', { name: t }).first().waitFor();
    }

    // Step 6: People narrows as you type; chips carry counts.
    await theo.go('/admin/users');
    const all = await theo.page.locator('tbody tr').count();
    for (const chip of ['All', 'Admins', 'Disabled']) {
      const link = theo.page.getByRole('group', { name: 'Filter' }).getByRole('link', { name: new RegExp('^' + chip) });
      assert.match(await link.innerText(), /\d/, `${chip} chip has a count`);
    }
    const search = theo.page.getByRole('searchbox');
    await search.pressSequentially('Theo Bra');
    await theo.page.waitForFunction(() => [...document.querySelectorAll('tbody tr')].every((r) => /Theo Bra/i.test(r.textContent)));
    const rows = theo.page.locator('tbody tr');
    assert.ok(await rows.count() >= 1, 'the match is listed');
    assert.ok(await rows.count() < all, 'the list narrows');
    await rows.first().getByText(NAME).waitFor();
    theo.assertClean();
  } finally {
    // Unmake the extra admin so the site's admins are as they were found.
    await admin.go('/admin/users?q=' + encodeURIComponent(first.email));
    const row = admin.page.locator('tbody tr', { hasText: first.email });
    admin.page.once('dialog', (d) => d.accept());
    await row.getByRole('button', { name: 'Remove Admin' }).click();
    const box = admin.page.locator('#reauth-modal');
    if (await box.getByText('Confirm Your Identity').isVisible().catch(() => false)) {
      await box.locator('input[name=password]').fill(ADMIN.password);
      await box.getByRole('button', { name: 'Confirm' }).click();
    }
    await admin.page.locator('tbody tr', { hasText: first.email }).getByText('Admin', { exact: true }).waitFor({ state: 'detached' }).catch(() => {});
  }
});

