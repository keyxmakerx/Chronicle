// text_edge.spec.mjs — a reading page's first line starts on the same edge as
// an app page's, at desktop and phone widths (#2253).
//
// RUN: tools/live-checks.sh test/live/text_edge.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, done, BASE } from './_live.mjs';

after(done);

// Left edge of the first heading or frame content inside the page's frame.
const edge = (page) => page.evaluate(() => {
  const f = document.querySelector('[data-app-frame]');
  if (!f) return null;
  const pad = parseFloat(getComputedStyle(f).paddingLeft) || 0;
  return Math.round(f.getBoundingClientRect().left + pad);
});

for (const width of [1440, 390]) {
  test(`reading pages start on the app page's edge at ${width} wide`, async () => {
    const owner = await person({ tag: `edge${width}`, name: 'Mara Vell', context: { viewport: { width, height: 900 } } });
    const cid = await newCampaign(owner, 'Edge Check');
    const { page } = owner;
    const seen = {};

    await owner.go(`/campaigns/${cid}`);
    seen.home = await edge(page);
    await owner.go(`/campaigns/${cid}/entities`);
    seen.allPages = await edge(page);
    await owner.go(`/campaigns/${cid}/maps`);
    seen.app = await edge(page);

    await owner.go(`/campaigns/${cid}/entities`);
    const href = await page.locator(`main a[href*="/campaigns/${cid}/entities/"]`).first().getAttribute('href').catch(() => null);
    if (href) {
      await page.goto(`${BASE}${href}`, { waitUntil: 'networkidle' });
      seen.page = await edge(page);
    }

    for (const [name, left] of Object.entries(seen)) {
      assert.equal(left, seen.app, `${name} starts at ${left}, the app page at ${seen.app}`);
    }
  });
}
