// admin_backup_daily.spec.mjs — the Daily backup setting on Backups &
// restore saves and shows, and a daily backup set for an hour already past
// runs within a minute. Stands in for the operator check in #1235 (and the
// daily half of #1752).
//
// Covers steps 1 to 3. Left for a person: setting the hour they actually
// want (03:00) and removing a host cron job that also runs `make backup`,
// which only the person who runs the server can see. The run itself needs
// the dump tool and database credentials; a server without them (the scratch
// one started with no DB_PASSWORD) reports a failed daily backup, which this
// check records as a skip rather than a pass or a failure of the page.
//
// The daily run happens once a day, so a second run of this file on the
// same day sees that day's result rather than a fresh one.
//
// It saves the one site-wide schedule and puts the old values back. Run it
// apart from admin_backups_home.spec.mjs.
//
// RUN: tools/live-checks.sh test/live/admin_backup_daily.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { siteAdmin, done, picture } from './_live.mjs';

after(done);

const FORM = 'form[action="/admin/backup/schedule"]';

async function readSchedule(page) {
  return page.evaluate((sel) => {
    const f = document.querySelector(sel);
    return { enabled: f.querySelector('[name=enabled]').checked, hour: f.querySelector('[name=hour]').value, keep: f.querySelector('[name=keep_days]').value };
  }, FORM);
}

async function saveSchedule(page, s) {
  const f = page.locator(FORM);
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

/** The server's current hour, from the zone the form names. */
async function serverHour(page) {
  const label = await page.getByText(/^At \(/).first().innerText();
  const zone = label.match(/At \(([^,)]+)/)[1];
  try {
    return Number(new Intl.DateTimeFormat('en-GB', { timeZone: zone, hour: '2-digit', hourCycle: 'h23' }).format(new Date()));
  } catch { return new Date().getUTCHours(); }
}

test('the daily backup setting saves and shows, and the run follows', async (t) => {
  const admin = await siteAdmin();
  const { page } = admin;
  await admin.go('/admin/backup');
  const original = await readSchedule(page);
  try {
    // Steps 1 and 2: tick, pick this hour (already begun), keep 7 days, Save.
    const hour = await serverHour(page);
    await saveSchedule(page, { enabled: true, hour, keep: 7 });
    assert.deepEqual(await readSchedule(page), { enabled: true, hour: String(hour), keep: '7' }, 'the setting saves and shows');
    await picture(page, 'admin-backup-daily', {
      check: '#1235', about: 'Daily backup card, ticked, hour chosen, keep 7 days',
      locator: page.locator('.card', { has: page.getByRole('heading', { name: 'Daily backup' }) }),
      mask: ['.card:has(h2:text("Daily backup")) select', '.card:has(h2:text("Daily backup")) span.block', '.card:has(h2:text("Daily backup")) .border-t'],
    });

    // Step 3: within a minute or so a result appears: a finished daily backup
    // on the card, or the red failed notice at the top.
    const finished = page.getByText(/Last daily backup finished/);
    const failed = page.getByText(/The daily backup on .* failed/);
    let outcome = '';
    for (let i = 0; i < 12 && !outcome; i++) {
      await page.waitForTimeout(8000);
      await page.reload({ waitUntil: 'networkidle' });
      if (await finished.count()) outcome = 'finished';
      else if (await failed.count()) outcome = 'failed';
    }
    assert.ok(outcome, 'no daily backup result appeared within about a minute and a half');
    if (outcome === 'failed') {
      // The scheduled failure says only "exit code 1". A manual run states
      // the reason, which tells a server that cannot back up at all from a
      // real fault in the daily path.
      page.on('dialog', (d) => d.accept());
      await page.getByRole('button', { name: 'Run backup' }).click();
      await page.locator('details summary').first().waitFor({ timeout: 90000 }).catch(() => {});
      await page.locator('details summary').first().click().catch(() => {});
      const why = (await failed.first().innerText().catch(() => '')) + ' ' + await page.locator('details pre').first().innerText().catch(() => '');
      if (/env_missing|DB_PASSWORD|not found|No such file/i.test(why)) {
        t.skip('this server cannot run a backup: ' + why.slice(0, 140));
        return;
      }
      assert.fail('the daily backup failed: ' + why);
    }
    assert.ok(await page.locator('tbody tr', { hasText: 'chronicle_db_' }).count() >= 1, 'a chronicle_db_… file is in Backup files');
  } finally {
    await admin.go('/admin/backup');
    await saveSchedule(page, original);
  }
});
