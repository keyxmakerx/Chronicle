// import_tray.spec.mjs — AI Import keeps answers that come in parts in a tray,
// opens one review when the last part lands, and flags a paste that was
// already applied. Stands in for the operator check in #1654.
//
// Covers steps 1-4 of the issue (tray card and row, one review in order,
// "already applied" box with the row starting on No, Empty with confirm).
// The boot-time migration line in the server log is not read here; it needs
// a person with the server's log.
//
// RUN: tools/live-checks.sh test/live/import_tray.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, done, picture } from './_live.mjs';

after(done);

const part = (n, name, table, needs) => `---
kind: part
name: ${name}
part: ${n}
of: 2${needs ? '\nneeds: [1]' : ''}
---
---
kind: table
name: ${table}
---
- B
`;

async function openPaste(who, cid) {
  await who.go(`/campaigns/${cid}#copy-and-paste`);
  await who.page.locator('#ai-import-paste-area').waitFor({ state: 'visible' });
}

// The paste is typed into the box and reviewed with its button, as an owner
// would; the page waits for the server's answer before looking.
async function review(who, text) {
  const { page } = who;
  await page.fill('#ai-import-paste-area', text);
  const answered = page.waitForResponse((r) => r.url().includes('/import/parse'));
  await page.locator('#ai-tool-paste').getByRole('button', { name: 'Review', exact: true }).click();
  await answered;
}

test('parts out of order wait in the tray, the last part opens one review, a repeat is flagged, Empty clears', async () => {
  const owner = await person({ tag: 'tray', name: 'Tray Owner' });
  const cid = await newCampaign(owner, 'Tray check campaign');
  const { page } = owner;
  await openPaste(owner, cid);

  // 1. Part 2 first goes to the tray; no review opens.
  await review(owner, part(2, 'Tray check', 'Tray check two', true));
  const host = page.locator('#ai-import-review-host');
  await page.getByText('Part 2 of "Tray check" went to the tray: 1 of 2 in.').waitFor();
  await page.getByText('Paste part 1 next.').waitFor();
  const tray = page.locator('#ai-import-tray');
  await tray.getByText('Tray check', { exact: true }).waitFor();
  assert.equal(await host.getByRole('button', { name: 'Apply' }).count(), 0, 'a review opened for a part that waits');
  await picture(page, 'import-tray-waiting', { check: '#1654', about: 'AI paste box with one part in the tray', locator: page.locator('.aip-body') });

  // 2. Part 1 completes it: one review, part 1's table first.
  await review(owner, part(1, 'Tray check', 'Tray check one'));
  await host.getByRole('button', { name: 'Apply' }).waitFor();
  const text = await host.innerText();
  assert.ok(text.indexOf('Tray check one') > -1 && text.indexOf('Tray check two') > -1, 'both tables should be in the review');
  assert.ok(text.indexOf('Tray check one') < text.indexOf('Tray check two'), 'part 1 should come first');
  await host.getByRole('button', { name: 'Apply' }).click();
  await page.getByText('Import complete').waitFor();
  await page.waitForLoadState('networkidle');
  await openPaste(owner, cid);
  await page.locator('#ai-import-tray.air-tray-host').waitFor({ state: 'attached' });
  assert.equal(await page.locator('#ai-import-tray .air-tray-row').count(), 0, 'the tray row should be gone after applying');

  // 3. The same paste again is flagged. (The row-starts-on-No half is its own
  // subtest below, because a repeated table is refused rather than listed.)
  await review(owner, part(1, 'Tray check', 'Tray check one'));
  await host.getByText('You already applied this paste, on').waitFor();
  await host.getByText(/Already applied on/).first().waitFor();
  assert.equal(await host.locator('.air-yn input[value=on]:checked').count(), 0, 'a repeated row starts on Yes');
  await picture(page, 'import-tray-already-applied', { check: '#1654', about: 'Review of a paste that was applied before', locator: page.locator('.aip-body'), mask: ['.aip-body time'] });

  // 4. Empty a tray row: confirm, row goes, nothing in the campaign changes.
  await openPaste(owner, cid);
  await review(owner, part(2, 'Tray check B', 'Tray check B two', true));
  const row = page.locator('#ai-import-tray .air-tray-row');
  await row.first().waitFor();
  page.once('dialog', (d) => d.accept());
  await row.getByRole('button', { name: /Empty the tray of Tray check B/ }).click();
  await page.waitForFunction(() => !document.querySelector('#ai-import-tray .air-tray-row'));
  const pages = await owner.api(`/campaigns/${cid}/entities?q=Tray%20check%20B`);
  assert.doesNotMatch(JSON.stringify(pages.body), /Tray check B two/, 'emptying the tray changed the campaign');
  owner.assertClean();
});

test('pasting a table you already have says so in plain words and adds nothing', async () => {
  const owner = await person({ tag: 'trayrep', name: 'Tray Repeat' });
  const cid = await newCampaign(owner, 'Tray repeat campaign');
  const { page } = owner;
  const one = `---\nkind: table\nname: Repeat table\n---\n- B\n`;
  await openPaste(owner, cid);
  await review(owner, one);
  await page.locator('#ai-import-review-host').getByRole('button', { name: 'Apply' }).click();
  await page.getByText('Import complete').waitFor();
  await openPaste(owner, cid);
  await review(owner, one);
  const host = page.locator('#ai-import-review-host');
  await host.getByText(/You already have the .*Repeat table\. Nothing was added\./i).first().waitFor();
  assert.doesNotMatch(await host.innerText(), /use action/i, 'the review still shows the wording meant for the AI');
  owner.assertClean();
});
