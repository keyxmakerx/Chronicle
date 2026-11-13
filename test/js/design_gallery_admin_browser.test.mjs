// design_gallery_admin_browser.test.mjs — the admin gallery queue widget
// (static/js/widgets/design_gallery_admin.js) in a real Chromium: it lists what
// is held, shows the reasons and reports, and Approve, Reject and Remove post to
// the admin routes and update the list; a refusal shows inline.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { skip } from './_browser_env.mjs';
import { launch } from './_design_library_harness.mjs';
import { openAdmin, entry } from './_design_gallery_admin_harness.mjs';

async function admin(t, opts) {
  const browser = await launch();
  t.after(() => browser.close());
  const r = await openAdmin(browser, opts);
  t.after(() => assert.deepEqual(r.errors, []));
  return r.page;
}
const calls = (p) => p.evaluate(() => window.__srv.calls);

test('the queue shows reasons and reports, and Approve moves the entry on', { skip }, async (t) => {
  const reports = [{ reason: 'adult', note: 'not for kids', createdAt: '2026-11-02T10:00:00Z' }, { reason: 'spam', createdAt: '2026-11-02T11:00:00Z' }];
  const p = await admin(t, { seed: { pending: [entry('e1', 'Forge of Embers', { reports }), entry('e2', 'Second')], visible: [entry('e3', 'Live one', { status: 'visible', reasons: [] })] } });
  await p.waitForSelector('article');
  const first = await p.locator('article').first().innerText();
  assert.match(first, /Forge of Embers/);
  assert.match(first, /web address or invite link/);
  assert.match(first, /Reports \(2\)/);
  assert.match(first, /Adult or vulgar: not for kids/);
  await p.click('[data-ga="approve"][data-id="e1"]');
  await p.waitForFunction(() => document.querySelectorAll('article').length === 1);
  assert.ok((await calls(p)).includes('POST /admin/design-gallery/e1/approve'));
  assert.match(await p.locator('[data-ga-msg]').innerText(), /now in the gallery/);
  await p.click('[data-ga="reject"][data-id="e2"]');
  await p.waitForSelector('text=Nothing is waiting for review.');
  assert.ok((await calls(p)).includes('POST /admin/design-gallery/e2/reject'));
  await p.click('[data-ga="tab"][data-v="visible"]');
  await p.waitForSelector('[data-ga="remove"]');
  await p.click('[data-ga="remove"][data-id="e3"]');
  await p.waitForFunction(() => document.querySelectorAll('article').length === 1);
  await p.click('[data-ga="remove"][data-id="e1"]');
  await p.waitForSelector('text=No designs are shown in the gallery yet.');
  assert.ok((await calls(p)).includes('POST /admin/design-gallery/e3/remove'));
});

test('a decision the server refuses is shown inline', { skip }, async (t) => {
  const p = await admin(t, { width: 400, seed: { pending: [entry('e1', 'Forge')], refuse: 'this entry is visible now, so that decision no longer applies' } });
  await p.waitForSelector('article');
  assert.equal(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await p.click('[data-ga="approve"]');
  await p.waitForFunction(() => /no longer applies/.test(document.querySelector('[data-ga-err]').textContent));
});

test('who made it is shown, and a refusal can be undone from Not approved', { skip }, async (t) => {
  const e1 = entry('e1', 'Forge', { publisherName: 'Alice Real-Name', makerName: 'The Smith', dataName: 'Inner Forge' });
  const p = await admin(t, { seed: { pending: [e1] } });
  await p.waitForSelector('article');
  const text = await p.locator('article').innerText();
  assert.match(text, /shown as The Smith \(account: Alice Real-Name\)/);
  assert.match(text, /Named inside the design: Inner Forge/);
  await p.click('[data-ga="reject"]');
  await p.waitForSelector('text=Nothing is waiting for review.');
  await p.click('[data-ga="tab"][data-v="rejected"]');
  await p.waitForSelector('[data-ga="approve"]');
  assert.match(await p.locator('article').innerText(), /Approve after all/);
  assert.ok((await calls(p)).some((c) => /status=rejected/.test(c)));
  await p.click('[data-ga="approve"]');
  await p.waitForSelector('text=Nothing has been turned down.');
  assert.ok((await calls(p)).includes('POST /admin/design-gallery/e1/approve'));
});
