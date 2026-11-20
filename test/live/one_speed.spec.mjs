// one_speed.spec.mjs — every link inside a campaign page changes the page the
// way the sidebar does, and a page lands whole.
//
// Part 1: a link in a page's words, a row on the campaign home and a card in
// All pages swap #main-content in place. A marker on the window object
// survives an in-place change and is lost by a full document load, so it is
// what tells the two apart. Back works, a form in the page still posts, and a
// link out of the campaign still loads in full.
//
// Part 2: after a page shows, where its text starts does not move.
//
// RUN: tools/live-checks.sh test/live/one_speed.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done, BASE } from './_live.mjs';

after(done);

let w;

const mark = (page) => page.evaluate(() => { window.__oneSpeed = 'same-document'; });
const marked = (page) => page.evaluate(() => window.__oneSpeed === 'same-document');
// htmx draws the swap a beat after the address changes; wait for the heading.
const heading = (page, text) => page.locator('#main-content h1', { hasText: text }).first().waitFor();

test('a link in the words, a home row and an All pages card change the page in place', async () => {
  const owner = await person({ tag: 'speed', name: 'Mara Vell', context: { viewport: { width: 1280, height: 900 } } });
  const cid = await newCampaign(owner, 'One Speed');
  const target = await newPage(owner, cid, 'Character', 'Thalrik Stone');
  const from = await newPage(owner, cid, 'Location', 'Ravenhold');
  w = { owner, cid, from, target };
  const url = `/campaigns/${cid}/entities/${target}`;
  const doc = { type: 'doc', content: [{ type: 'paragraph', content: [
    { type: 'text', text: 'Ruled by ' },
    { type: 'text', text: 'Thalrik Stone', marks: [{ type: 'link', attrs: { href: url } }] },
    { type: 'text', text: ' and visited from ' },
    { type: 'text', text: 'the campaigns list', marks: [{ type: 'link', attrs: { href: '/campaigns' } }] },
  ] }] };
  const put = await owner.api(`/campaigns/${cid}/entities/${from}/entry`, { method: 'PUT', body: { entry: JSON.stringify(doc), entry_html: '' } });
  assert.ok(put.ok, `saving the words answered ${put.status}`);

  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${from}`);
  await page.locator('#main-content .ProseMirror a', { hasText: 'Thalrik Stone' }).waitFor();
  const fromTitle = await page.title();
  await mark(page);

  // 1. The link in the words.
  await page.locator('#main-content .ProseMirror a', { hasText: 'Thalrik Stone' }).click();
  await page.waitForURL(`${BASE}${url}`);
  await heading(page, 'Thalrik Stone');
  assert.ok(await marked(page), 'the link in the words reloaded the whole document');
  assert.notEqual(await page.title(), fromTitle, 'the title did not change');
  assert.equal(await page.evaluate(() => document.getElementById('main-content').scrollTop), 0, 'the new page did not open at its top');

  // 2. Back returns to the page that was left, still without a reload.
  await page.goBack();
  await heading(page, 'Ravenhold');
  assert.ok(await marked(page), 'Back reloaded the whole document');
  assert.equal(new URL(page.url()).pathname, `/campaigns/${cid}/entities/${from}`);

  // 3. A link out of the campaign still loads in full.
  await page.locator('#main-content .ProseMirror a', { hasText: 'the campaigns list' }).click();
  await page.waitForURL(`${BASE}/campaigns`);
  assert.ok(!(await marked(page)), 'a link out of the campaign was swapped in place');

  // 4. A row under Recently updated on the campaign home.
  await owner.go(`/campaigns/${cid}`);
  const row = page.locator('#main-content a[href*="/entities/"]').first();
  await row.waitFor();
  await mark(page);
  const rowHref = await row.getAttribute('href');
  await row.click();
  await page.waitForURL((u) => u.pathname === rowHref.split('?')[0]);
  await page.locator('#main-content h1').first().waitFor();
  assert.ok(await marked(page), 'a home row reloaded the whole document');

  // 5. A card in All pages.
  await owner.go(`/campaigns/${cid}/entities`);
  const card = page.locator('#main-content a[href*="/entities/"]').filter({ hasText: /Ravenhold|Thalrik/ }).first();
  await card.waitFor();
  await mark(page);
  const cardHref = await card.getAttribute('href');
  await card.click();
  await page.waitForURL((u) => u.pathname === cardHref.split('?')[0]);
  await page.locator('#main-content h1').first().waitFor();
  assert.ok(await marked(page), 'an All pages card reloaded the whole document');

  // 6. A form in the page still posts: make a page from the new-page form
  // reached by an in-place change.
  await owner.go(`/campaigns/${cid}/entities`);
  await mark(page);
  await page.locator('#main-content a[href$="/entities/new"]').first().click();
  await page.waitForURL(`${BASE}/campaigns/${cid}/entities/new`);
  assert.ok(await marked(page), 'the new-page link reloaded the whole document');
  await page.locator('select#entity_type_id').selectOption({ index: 1 });
  const field = page.locator('#entity-form input[name=name], form input#name').first();
  await field.fill('Posted In Place');
  await Promise.all([page.waitForURL(/\/entities\/[0-9a-f-]{36}$/), field.press('Enter')]);
  await heading(page, 'Posted In Place');
  owner.assertClean();
});

test('an unsaved edit is never swapped away in place', async () => {
  const { owner, cid, from } = w;
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${from}`);
  const link = page.locator('#main-content .ProseMirror a', { hasText: 'Thalrik Stone' });
  await link.waitFor();
  // With no form on screen to ask on, the hold hands the link to a full load,
  // where the browser's own leave prompt asks. Either way the unsaved change
  // is not thrown away by a quiet swap.
  let asked = false;
  page.on('dialog', (d) => { asked = true; d.dismiss().catch(() => {}); });
  await page.evaluate(() => Chronicle.markDirty('test:one-speed'));
  await mark(page);
  await link.click();
  await page.waitForTimeout(800);
  const stayedHere = new URL(page.url()).pathname === `/campaigns/${cid}/entities/${from}`;
  assert.ok(asked || stayedHere || !(await marked(page)), 'the link swapped the page away without asking');
  await page.evaluate(() => Chronicle.markClean && Chronicle.markClean('test:one-speed')).catch(() => {});
});

test('a page does not move after it shows', async () => {
  const { owner, cid, from, target } = w;
  const { page } = owner;
  // A tag and a connection, the two parts that used to arrive late.
  const tag = await owner.api(`/campaigns/${cid}/tags`, { method: 'POST', body: { name: 'Haunted', color: '#7c3aed' } });
  assert.ok(tag.ok, `making a tag answered ${tag.status}`);
  const tagged = await owner.api(`/campaigns/${cid}/entities/${from}/tags`, { method: 'PUT', body: { tagIds: [tag.body.id] } });
  assert.ok(tagged.ok, `tagging answered ${tagged.status}`);
  const rel = await owner.api(`/campaigns/${cid}/entities/${from}/relations`, { method: 'POST', body: { targetEntityId: target, relationType: 'ruled by', reverseRelationType: 'rules' } });
  assert.ok(rel.ok, `connecting answered ${rel.status}`);

  // Every frame, where the entry text starts, from the first frame the page
  // can be seen (not hidden, not faded out) until a second after it.
  await page.addInitScript(() => {
    window.__tops = [];
    const t0 = performance.now();
    (function tick() {
      const ed = document.querySelector('[data-widget="editor"]');
      if (ed) {
        let seen = true;
        for (let n = ed; n && n !== document.documentElement; n = n.parentElement) {
          const cs = getComputedStyle(n);
          if (cs.visibility === 'hidden' || parseFloat(cs.opacity) < 0.05) { seen = false; break; }
        }
        if (seen) window.__tops.push(Math.round(ed.getBoundingClientRect().top * 10) / 10);
      }
      if (performance.now() - t0 < 6000) requestAnimationFrame(tick);
    })();
  });
  await page.goto(`${BASE}/campaigns/${cid}/entities/${from}`);
  await page.locator('#main-content .ProseMirror').waitFor();
  await page.locator('[data-widget="relations"] .rel-item').first().waitFor();
  await page.locator('[data-widget="tag-picker"]').getByText('Haunted').waitFor();
  await page.waitForTimeout(1500);
  const tops = await page.evaluate(() => window.__tops);
  assert.ok(tops.length > 5, 'the entry text was never seen');
  const spread = Math.max(...tops) - Math.min(...tops);
  assert.ok(spread <= 1, `the entry text moved ${spread}px after it first showed (${[...new Set(tops)].join(', ')})`);
  owner.assertClean();
});
