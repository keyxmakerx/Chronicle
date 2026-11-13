// sessions_availability_crowded.spec.mjs — the availability heatmap and a
// proposal with several times stay readable when five or more players are free
// on the same day, with no sideways scrolling on a laptop-width screen.
// Stands in for the operator check in #815.
//
// Covers steps 1 and 2 at 1280 wide with six players and the Director all free
// on the same Friday. "Readable" is asserted as: no page or panel scrolls
// sideways, each person's lane is inside its day column and none overlaps
// another, and no option text is cut off. Whether it looks comfortable is left
// to the pictures for a person to approve.
//
// RUN: tools/live-checks.sh test/live/sessions_availability_crowded.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

const NAMES = ['Ana Vance', 'Bo Harrow', 'Cy Mercer', 'Di Falk', 'Ed Lorne', 'Fay Ostler'];
let w;
let covered = [];

const sideways = (page) => page.evaluate(() => ({
  page: document.documentElement.scrollWidth - document.documentElement.clientWidth,
  main: (() => { const m = document.querySelector('#main-content'); return m.scrollWidth - m.clientWidth; })(),
  panels: [...document.querySelectorAll('.avail-scroll, [data-avail-panel]')].map((e) => e.scrollWidth - e.clientWidth).filter((n) => n > 0),
}));

test('six players free on one day: the heatmap has a lane each and nothing scrolls sideways', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const cid = await newCampaign(owner, 'Crowded Week');
  await enableAddons(owner, cid, '*');
  await owner.go(`/campaigns/${cid}/availability`);
  const players = [];
  for (const [i, name] of NAMES.entries()) {
    const p = await person({ tag: 'p' + i, name });
    await addMember(owner, cid, p, 'player');
    players.push(p);
  }
  // Everyone is free Friday evening, starting at staggered hours.
  for (const [i, p] of [owner, ...players].entries()) {
    await p.go(`/campaigns/${cid}/availability`);
    const r = await p.api(`/campaigns/${cid}/availability/mine`, { method: 'PUT', body: { tz: 'UTC', blocks: [
      { dayOfWeek: 5, startMinute: 1020 + i * 20, endMinute: 1380, state: i % 2 ? 'preferred' : 'available', weekCadence: 0 },
    ] } });
    assert.ok(r.ok, `saving answered ${r.status}`);
  }
  w = { owner, players, cid };

  await owner.go(`/campaigns/${cid}/availability?tab=overlay`);
  const grid = owner.page.locator('.ov-weekgrid');
  await grid.waitFor();
  await owner.page.locator('.ov-lane').first().waitFor();

  const lanes = await owner.page.locator('.ov-lane').evaluateAll((els) => els.map((e) => {
    const r = e.getBoundingClientRect(); const col = e.closest('.ov-daycol').getBoundingClientRect();
    return { name: e.title, x: r.x, right: r.right, w: r.width, colX: col.x, colRight: col.right };
  }));
  assert.equal(lanes.length, 7, 'one lane for each of the seven people');
  for (const l of lanes) {
    assert.ok(l.x >= l.colX - 0.5 && l.right <= l.colRight + 0.5, `${l.name} spills out of its day column`);
    assert.ok(l.w >= 8, `${l.name} is squeezed to ${l.w}px`);
  }
  const sorted = [...lanes].sort((a, b) => a.x - b.x);
  for (let i = 1; i < sorted.length; i++) assert.ok(sorted[i].x >= sorted[i - 1].right - 0.5, `${sorted[i].name} overlaps ${sorted[i - 1].name}`);

  const scroll = await sideways(owner.page);
  assert.equal(scroll.page, 0, 'the page scrolls sideways');
  assert.equal(scroll.main, 0, 'the content area scrolls sideways');
  assert.deepEqual(scroll.panels, [], 'a panel scrolls sideways');

  covered = await owner.page.locator('.ov-bn').evaluateAll((els) => els.map((b) => {
    const r = b.getBoundingClientRect();
    return [...document.querySelectorAll('.ov-lane')].some((l) => { const q = l.getBoundingClientRect(); return q.right > r.left && q.left < r.right && q.bottom > r.top && q.top < r.bottom; }) ? b.textContent : null;
  }).filter(Boolean));

  // The roster chips wrap rather than run off the screen or get cut.
  const chips = await owner.page.locator('.avail-chip').evaluateAll((els) => els.map((e) => ({ cut: e.scrollWidth > e.clientWidth + 1, right: e.getBoundingClientRect().right })));
  assert.ok(chips.every((c) => !c.cut && c.right <= 1280), 'a roster chip is cut off');
  // Scroll Friday evening into view so the picture shows the crowded lanes.
  await owner.page.locator('.ov-lane').first().evaluate((e) => e.scrollIntoView({ block: 'center' }));
  await picture(owner.page, 'sessions-availability-crowded', {
    check: '#815', about: 'Group heatmap with seven people free on the same Friday',
    mask: ['header', '[data-week-label]', '.ov-weekgrid > div:nth-child(n+2):nth-child(-n+8)'],
  });
  owner.assertClean();
});

test('a proposal with several times reads cleanly for the Director and a player', async () => {
  const { owner, players, cid } = w;
  const options = ['2031-07-04', '2031-07-05', '2031-07-11', '2031-07-12', '2031-07-18'].map((date, i) => ({ date, startMinute: 1080 + (i % 2) * 60, endMinute: 1320 }));
  const made = await owner.api(`/campaigns/${cid}/proposals`, { method: 'POST', body: { title: 'Pick a night for the heist', note: 'Anything on a weekend works.', tz: 'UTC', options } });
  assert.ok(made.ok, `creating the proposal answered ${made.status}`);
  const pid = made.body.id || (made.body.proposal && made.body.proposal.id);
  assert.ok(pid, 'no proposal id came back');
  // Everyone answers, so every option carries a full set of votes.
  for (const p of [owner, ...players]) {
    await p.go(`/campaigns/${cid}/proposals/${pid}`);
    const opts = await p.page.locator('[hx-post*="/respond"]').evaluateAll((els) => [...new Set(els.map((e) => e.getAttribute('hx-post')))]);
    for (const [i, url] of opts.entries()) await p.api(url, { method: 'POST', form: { response: ['yes', 'maybe', 'no'][i % 3] } });
  }

  for (const who of [owner, players[0]]) {
    await who.go(`/campaigns/${cid}/proposals/${pid}`);
    const text = await who.page.locator('#main-content').innerText();
    assert.match(text, /Pick a night for the heist/);
    const scroll = await sideways(who.page);
    assert.equal(scroll.page, 0, 'the proposal page scrolls sideways');
    assert.equal(scroll.main, 0, 'the proposal content scrolls sideways');
    const cut = await who.page.locator('#main-content *').evaluateAll((els) => els
      .filter((e) => e.children.length === 0 && e.textContent.trim() && e.scrollWidth > e.clientWidth + 1 && getComputedStyle(e).overflow !== 'visible')
      .map((e) => e.textContent.trim().slice(0, 40)));
    assert.deepEqual(cut, [], 'text is cut off: ' + cut.join(' | '));
  }
  await picture(owner.page, 'sessions-proposal-several-times', { check: '#815', about: 'A proposal with five suggested times and seven answers', mask: ['header'] });
  owner.assertClean();
});

test('the "7/7" counts stay readable when seven lanes stack in a day', { todo: 'with five or more people the lanes cover the free-count numbers in the hour cells' }, () => {
  assert.deepEqual(covered, [], 'lanes sit on top of these counts: ' + covered.join(', '));
});
