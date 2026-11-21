// calendar_several_moons.spec.mjs — a calendar with several moons shows each
// moon on every day as its own small silhouette in a row, not as nested
// rings. The Harptos calendar has four moons on a ten-day week (narrow
// cells); a made calendar has two moons on a six-day week. A player sees the
// moons the Director has not hidden, and the row never lands on the day
// number or the event marks, at desktop or phone width.
//
// RUN: tools/live-checks.sh test/live/calendar_several_moons.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { person, newCampaign, addMember, enableAddons, done, picture, repoRoot } from './_live.mjs';

after(done);

const PHONE = { width: 390, height: 844 };
const DESKTOP = { width: 1280, height: 800 };

async function calendarFor(owner, player, tag, make) {
  const cid = await newCampaign(owner, tag);
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, '*');
  const made = await make(cid);
  assert.equal(made.status, 201, `making the calendar answered ${made.status}`);
  const cal = made.body.calendar;
  const base = `/campaigns/${cid}/calendars/${cal.id}`;
  await owner.go(`${base}/view`);
  const utc = owner.page.getByRole('button', { name: 'Use UTC time' });
  if (await utc.count()) { await utc.first().click(); await owner.go(`${base}/view`); }
  const fresh = await owner.api(base);
  return { cid, base, cal: fresh.ok && fresh.body && fresh.body.current_year ? fresh.body : cal };
}

// A made calendar: the Imperial preset's six-day week with two moons.
async function twoMoonsOnSixDays(owner, cid) {
  const file = JSON.parse(readFileSync(join(repoRoot, 'internal/plugins/calendar/presets/imperial.json'), 'utf8'));
  file.calendar.moons = [
    { name: 'The Seal', cycle_days: 30.4, phase_offset: 8, color: '#d9d4c4' },
    { name: 'Ember', cycle_days: 17.5, phase_offset: 3, color: '#e08a5a' },
  ];
  return owner.page.evaluate(async ({ cid, text }) => {
    const form = new FormData();
    form.append('file', new File([text], 'calendar.json', { type: 'application/json' }));
    const res = await Chronicle.apiFetch(`/campaigns/${cid}/calendars/import`, { method: 'POST', body: form });
    return { status: res.status, body: await res.json().catch(() => ({})) };
  }, { cid, text: JSON.stringify(file) });
}

// What the month draws: per day with a moon, how many silhouettes, rings and counts, and whether the
// row keeps clear of the day number, the event marks and the cell's edges.
const readMonth = (page) => page.evaluate(() => {
  const hit = (a, b) => a.left < b.right - 0.5 && b.left < a.right - 0.5 && a.top < b.bottom - 0.5 && b.top < a.bottom - 0.5;
  return [...document.querySelectorAll('.cal-v5 .day')].filter((d) => d.querySelector('.msil')).map((d) => {
    const row = d.querySelector('.msil'), cell = d.getBoundingClientRect();
    const sils = [...row.querySelectorAll('.m1')].filter((s) => getComputedStyle(s).display !== 'none');
    const count = [...row.querySelectorAll('.mn')].find((n) => getComputedStyle(n).display !== 'none');
    // What is drawn, not the row's box: the box keeps a touch margin around the silhouettes.
    const boxes = sils.concat(count ? [count] : []).map((e) => e.getBoundingClientRect());
    const r = { left: Math.min(...boxes.map((b) => b.left)), right: Math.max(...boxes.map((b) => b.right)), top: Math.min(...boxes.map((b) => b.top)), bottom: Math.max(...boxes.map((b) => b.bottom)) };
    const num = d.querySelector('.num'), marks = d.querySelector('.marks');
    return {
      key: d.dataset.key, several: row.classList.contains('several'),
      silhouettes: sils.length, rings: row.querySelectorAll('.rng').length, count: count ? count.textContent : '',
      tinted: sils.filter((s) => s.style.getPropertyValue('--mc')).length,
      onNumber: !!num && hit(r, num.getBoundingClientRect()),
      onMarks: !!marks && hit(r, marks.getBoundingClientRect()),
      inCell: r.left >= cell.left - 0.5 && r.right <= cell.right + 0.5,
      offCentre: Math.abs((r.left + r.right) / 2 - (cell.left + cell.right) / 2),
      top: Math.round(row.getBoundingClientRect().top - cell.top),
    };
  });
});

async function addMarks(owner, base, cal) {
  for (let d = 1; d <= 6; d++) {
    for (const name of ['Fair', 'Feast']) {
      const made = await owner.api(`${base}/events`, { method: 'POST', body: { name: `${name} ${d}`, year: cal.current_year, month: cal.current_month, day: d, all_day: true, visibility: 'everyone' } });
      assert.equal(made.status, 201, `creating an event answered ${made.status}`);
    }
  }
}

test('four moons on a ten-day week are four silhouettes a day, no rings, for the owner and a player', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const { base, cal } = await calendarFor(owner, player, 'Several Moons', (cid) => owner.api(`/campaigns/${cid}/calendars/presets/harptos`, { method: 'POST', body: {} }));
  const { page } = owner;
  await addMarks(owner, base, cal);
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();

  const wide = await readMonth(page);
  assert.ok(wide.length >= 10, 'the month drew too few moon marks');
  for (const d of wide) {
    assert.ok(d.several, `${d.key} is not a row of silhouettes`);
    assert.equal(d.silhouettes, 4, `${d.key} shows ${d.silhouettes} silhouettes`);
    assert.equal(d.rings, 0, `${d.key} still draws rings`);
    assert.equal(d.count, '', `${d.key} counts moons that are all shown`);
    assert.equal(d.tinted, 4, `${d.key} has a moon with no colour of its own`);
    assert.ok(!d.onNumber, `${d.key}: the moons sit on the day number`);
    assert.ok(!d.onMarks, `${d.key}: the moons sit on the event marks`);
    assert.ok(d.inCell, `${d.key}: the moons spill out of the cell`);
  }
  await picture(page, 'calendar-several-moons-desktop-light', { check: '#2482', about: 'Owner month, four moons on a ten-day week, desktop', locator: page.locator('#cal5-cal') });

  await page.emulateMedia({ colorScheme: 'dark' });
  await picture(page, 'calendar-several-moons-desktop-dark', { check: '#2482', about: 'The same month in dark', locator: page.locator('#cal5-cal') });
  await page.emulateMedia({ colorScheme: 'light' });

  await page.setViewportSize(PHONE);
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();
  const narrow = await readMonth(page);
  for (const d of narrow) {
    // A ten-day week leaves a phone cell about 33px: all four moons, two lines of two, nothing left out.
    assert.equal(d.silhouettes, 4, `${d.key}: a phone shows ${d.silhouettes} silhouettes`);
    assert.equal(d.count, '', `${d.key}: a phone counts moons that are all shown`);
    assert.ok(!d.onNumber && !d.onMarks, `${d.key}: the moons sit on the number or the event marks on a phone`);
    assert.ok(d.inCell, `${d.key}: the moons spill out of the cell on a phone`);
    assert.ok(d.offCentre < 2, `${d.key}: the row is ${d.offCentre}px off centre`);
    assert.equal(d.top, 31, `${d.key}: the row starts ${d.top}px down, not at the single moon's height`);
  }
  await picture(page, 'calendar-several-moons-phone-light', { check: '#2482', about: 'Owner month, four moons, phone width', locator: page.locator('#cal5-cal') });
  await page.setViewportSize(DESKTOP);

  // A player sees the same moons; when the Director hides one, the player
  // sees three and the Director still sees all four.
  await player.go(`${base}/view`);
  await player.page.locator('.cal-v5 .day .msil').first().waitFor();
  assert.ok((await readMonth(player.page)).every((d) => d.silhouettes === 4 && d.rings === 0), 'a player does not see the same four moons');

  const moons = cal.moons || (await owner.api(base)).body.moons;
  assert.equal(moons.length, 4);
  const hid = await owner.api(`${base}/moons/${moons[1].id}/hidden`, { method: 'PUT', body: { hidden: true } });
  assert.ok(hid.ok, `hiding a moon answered ${hid.status}`);
  await player.go(`${base}/view`);
  await player.page.locator('.cal-v5 .day .msil').first().waitFor();
  assert.ok((await readMonth(player.page)).every((d) => d.silhouettes === 3), 'the player still sees the hidden moon');
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();
  assert.ok((await readMonth(page)).every((d) => d.silhouettes === 4), 'the Director lost a moon by hiding it from players');
  owner.assertClean();
  player.assertClean();
});

test('two moons on a six-day week are two tinted silhouettes a day, clear of the day number', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const { base, cal } = await calendarFor(owner, player, 'Two Moons', (cid) => twoMoonsOnSixDays(owner, cid));
  const { page } = owner;
  await addMarks(owner, base, cal);
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();
  for (const width of [DESKTOP, PHONE]) {
    await page.setViewportSize(width);
    await owner.go(`${base}/view`);
    await page.locator('.cal-v5 .day .msil').first().waitFor();
    const days = await readMonth(page);
    assert.ok(days.length >= 10, 'the month drew too few moon marks');
    for (const d of days) {
      assert.ok(d.several && d.silhouettes === 2 && d.rings === 0 && d.tinted === 2, `${d.key} at ${width.width}px: ${JSON.stringify(d)}`);
      assert.ok(!d.onNumber && !d.onMarks && d.inCell, `${d.key} at ${width.width}px: the moons collide`);
    }
    await picture(page, `calendar-two-moons-${width.width < 600 ? 'phone' : 'desktop'}`, { check: '#2482', about: `Owner month, two moons on a six-day week, ${width.width}px`, locator: page.locator('#cal5-cal') });
  }
  owner.assertClean();
});
