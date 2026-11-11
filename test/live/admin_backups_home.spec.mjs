// admin_backups_home.spec.mjs — backups and admin Home on a running site:
// Run backup works, the daily schedule saves, Home tells the truth about
// backups, the pre-update backup is listed, no dead buttons, and the backup
// download arrives. Stands in for the operator check in #1752.
//
// Covers steps 1, 2 (the setting saves and the "first daily backup" line
// shows; the next-morning run is #1235's own check), 4, 5 and 6, and the
// backup download from #2099. Left for a person: step 3 (a week of
// retention needs a week), the scheduled run "the next morning" of step 2,
// and step 7 (Check now on a package's Settings tab needs an installed
// package; a scratch site has none and cannot fetch one).
//
// It runs a real backup, so the server needs mariadb-dump (or mysqldump) and
// database access; if the run fails the page's own failure text is in the
// assertion message. It saves the daily schedule and puts the old one back.
// Run it apart from admin_backup_daily.spec.mjs: both own the one schedule.
//
// RUN: tools/live-checks.sh test/live/admin_backups_home.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { siteAdmin, person, newCampaign, ADMIN, done, picture } from './_live.mjs';

after(done);

async function confirmReauth(page) {
  const box = page.locator('#reauth-modal');
  const shown = await box.getByText('Confirm Your Identity').waitFor({ state: 'visible', timeout: 4000 }).then(() => true, () => false);
  if (shown) {
    await box.locator('input[name=password]').fill(ADMIN.password);
    await box.getByRole('button', { name: 'Confirm' }).click();
    await box.waitFor({ state: 'hidden' });
  }
}

async function readSchedule(page) {
  return page.evaluate(() => {
    const f = document.querySelector('form[action="/admin/backup/schedule"]');
    return { enabled: f.querySelector('[name=enabled]').checked, hour: f.querySelector('[name=hour]').value, keep: f.querySelector('[name=keep_days]').value };
  });
}

async function saveSchedule(page, s) {
  const f = page.locator('form[action="/admin/backup/schedule"]');
  await f.locator('[name=enabled]').setChecked(s.enabled);
  await f.locator('[name=hour]').selectOption(String(s.hour));
  await f.locator('[name=keep_days]').fill(String(s.keep));
  await Promise.all([
    page.waitForResponse((r) => r.url().includes('/admin/backup/schedule') && r.request().method() === 'POST'),
    f.getByRole('button', { name: 'Save' }).click(),
  ]);
  await page.waitForTimeout(500);
  await page.goto(page.url(), { waitUntil: 'networkidle' });
}

const ran = { ok: false };

test('Run backup finishes with "Last run succeeded"', async (t) => {
  const admin = await siteAdmin();
  const { page } = admin;
  await admin.go('/admin/backup');
  const dialog = (d) => d.accept();
  page.on('dialog', dialog);
  await page.getByRole('button', { name: 'Run backup' }).click();
  await confirmReauth(page);
  const outcome = page.getByText(/^\s*Last run (succeeded|failed|timed out)/);
  await outcome.first().waitFor({ timeout: 90000 });
  const text = await outcome.first().innerText();
  let detail = '';
  if (/failed|timed out/.test(text)) {
    await page.locator('details summary').first().click().catch(() => {});
    detail = await page.locator('details pre').first().innerText().catch(() => '');
  }
  // A server started without database credentials for the dump tool cannot
  // back up at all; that is the environment, not the page.
  if (/env_missing|DB_PASSWORD not set|not found|No such file/.test(detail)) {
    t.skip('this server cannot run a backup (' + detail.trim().slice(0, 120) + ')');
    return;
  }
  assert.match(text, /Last run succeeded/, 'the backup ran: ' + text + ' ' + detail);
  assert.doesNotMatch(detail, /set-gtid-purged/);
  ran.ok = true;
  await picture(page, 'admin-backup', {
    check: '#1752', about: 'Backups page after a successful run: Run card, Daily backup, files',
    mask: ['main .text-green-700', 'main table', '.admin-strip-figures'],
  });
  assert.ok(await page.locator('tbody tr').count() >= 1, 'the new backup is in the file list');
});

test('daily setting, Home, restore list and dead buttons', async () => {
  const admin = await siteAdmin();
  const { page } = admin;
  page.on('dialog', (d) => d.accept());
  await admin.go('/admin/backup');
  const original = await readSchedule(page);
  try {
    // Step 2: tick Back up every day, pick an hour, Save.
    const zone = (await page.getByText(/^At \(/).first().innerText()).match(/At \(([^,)]+)/)[1];
    let hour = new Date().getUTCHours();
    try { hour = Number(new Intl.DateTimeFormat('en-GB', { timeZone: zone, hour: '2-digit', hourCycle: 'h23' }).format(new Date())); } catch { /* zone name not an IANA name; UTC stands */ }
    await saveSchedule(page, { enabled: true, hour: (hour + 1) % 24, keep: 7 });
    assert.deepEqual(await readSchedule(page), { enabled: true, hour: String((hour + 1) % 24), keep: '7' }, 'the setting saved and shows');

    // Step 4: Home tells the truth.
    await admin.go('/admin');
    const needs = page.locator('.card', { has: page.getByRole('heading', { name: 'Needs you' }) });
    assert.doesNotMatch(await needs.innerText(), /Daily backups are off/, 'Needs you no longer says daily backups are off');
    const tile = page.locator('a', { hasText: 'Backups & restore' }).filter({ has: page.locator('p.font-mono') }).first();
    if (ran.ok) {
      assert.match(await tile.innerText(), /Under 1 hour|1 hour/i, 'the tile shows the new backup\'s age: ' + await tile.innerText());
      assert.match(await page.locator('.admin-strip-figures').innerText(), /Last backup .* ago/, 'the bottom strip says Last backup … ago');
    }

    // Step 5: the pre-update backup is listed on Restore, with its badge.
    await admin.go('/admin/restore');
    await page.getByText('Before update', { exact: true }).first().waitFor();

    // Step 6: Leave only on campaigns the admin joined; no Remove Admin on your own row.
    const owner = await person({ tag: 'backups', name: 'Mara Vell' });
    const cname = 'Leave check ' + owner.email.split('@')[0].slice(-8);
    await newCampaign(owner, cname);
    await admin.go('/admin/campaigns?q=' + encodeURIComponent(cname));
    const row = page.locator('tbody tr', { hasText: cname });
    await row.waitFor();
    assert.equal(await row.getByRole('button', { name: /^Leave/ }).count(), 0, 'no Leave on a campaign the admin has not joined');
    await row.getByRole('button', { name: /Join as/ }).click();
    await row.getByRole('menuitem', { name: 'Join as player' }).click();
    await confirmReauth(page);
    await page.locator('tbody tr', { hasText: cname }).getByRole('button', { name: /^Leave/ }).waitFor();
    // Put it back: leave again, once the refreshed list has settled.
    await page.waitForLoadState('networkidle');
    await page.locator('tbody tr', { hasText: cname }).getByRole('button', { name: /^Leave/ }).click();
    await page.locator('tbody tr', { hasText: cname }).getByRole('button', { name: /^Leave/ }).waitFor({ state: 'detached' });
    await admin.go('/admin/users?q=' + encodeURIComponent(ADMIN.email));
    const me = page.locator('tbody tr', { hasText: ADMIN.email });
    await me.waitFor();
    assert.equal(await me.getByRole('button', { name: 'Remove Admin' }).count(), 0, 'your own row has no Remove Admin');
  } finally {
    await admin.go('/admin/backup');
    await saveSchedule(page, original);
  }
});

test('a backup file downloads', async () => {
  const admin = await siteAdmin();
  const { page } = admin;
  await admin.go('/admin/backup');
  const btn = page.getByRole('button', { name: 'Download' }).first();
  await btn.waitFor();
  const dl = page.waitForEvent('download', { timeout: 20000 });
  await btn.click();
  await confirmReauth(page);
  const file = await dl;
  const { createReadStream } = await import('node:fs');
  const path = await file.path();
  let size = 0;
  for await (const c of createReadStream(path)) size += c.length;
  assert.ok(size > 0, 'the downloaded file ' + file.suggestedFilename() + ' has content');
});
