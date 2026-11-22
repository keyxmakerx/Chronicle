// calendar_several_moons.spec.mjs — a calendar with several moons shows one
// disc a day with each moon's lit shape laid over it in its own colour, three
// at most and a count for the rest. The Harptos calendar has four moons on a
// ten-day week (narrow cells); a made calendar has two moons on a six-day
// week. A player sees the moons the Director has not hidden, and the mark
// never lands on the day number, the event marks or the next day's mark, at
// desktop or phone width. Therin's pair, a pale moon and a deep red one half a
// cycle apart, is pictured in light and dark to judge the colours by eye.
//
// RUN: tools/live-checks.sh test/live/calendar_several_moons.spec.mjs

import { test, after, beforeEach } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { person, newCampaign, addMember, enableAddons, done, picture, repoRoot, once, resetErrors } from './_live.mjs';

after(done);

const PHONE = { width: 390, height: 844 };
const SMALL = { width: 320, height: 640 };
const DESKTOP = { width: 1280, height: 800 };

// One owner, one player and one campaign serve every walk here; each walk
// makes its own calendar in it, so the walks never read each other's days.
const world = once(async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const cid = await newCampaign(owner, 'Several Moons');
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, '*');
  return { owner, player, cid };
});

beforeEach(async () => { const w = await world(); resetErrors(w.owner, w.player); });

async function calendarFor(owner, cid, make) {
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

const SEAL_AND_EMBER = [
  { name: 'The Seal', cycle_days: 30.4, phase_offset: 8, color: '#d9d4c4' },
  { name: 'Ember', cycle_days: 17.5, phase_offset: 3, color: '#e08a5a' },
];
const THERIN = [
  { name: 'Lacrimosa', cycle_days: 24, phase_offset: 0, color: '#e5e7eb' },
  { name: 'Sanguin’mor', cycle_days: 24, phase_offset: 12, color: '#b91c1c' },
];

// A made calendar: the Imperial preset's six-day week with two moons.
async function twoMoonsOnSixDays(owner, cid, moons = SEAL_AND_EMBER) {
  const file = JSON.parse(readFileSync(join(repoRoot, 'internal/plugins/calendar/presets/imperial.json'), 'utf8'));
  file.calendar.moons = moons;
  return owner.page.evaluate(async ({ cid, text }) => {
    const form = new FormData();
    form.append('file', new File([text], 'calendar.json', { type: 'application/json' }));
    const res = await Chronicle.apiFetch(`/campaigns/${cid}/calendars/import`, { method: 'POST', body: form });
    return { status: res.status, body: await res.json().catch(() => ({})) };
  }, { cid, text: JSON.stringify(file) });
}

// What the month draws: per day with a moon, the discs, lit shapes and count, and whether the mark
// keeps clear of the day number, the event marks, the cell's edges and every other day's mark.
const readMonth = (page) => page.evaluate(() => {
  const hit = (a, b) => a.left < b.right - 0.5 && b.left < a.right - 0.5 && a.top < b.bottom - 0.5 && b.top < a.bottom - 0.5;
  const all = [...document.querySelectorAll('.cal-v5 .day .msil')].map((m) => ({ m, r: m.getBoundingClientRect() }));
  return [...document.querySelectorAll('.cal-v5 .day')].filter((d) => d.querySelector('.msil')).map((d) => {
    const mark = d.querySelector('.msil'), cell = d.getBoundingClientRect();
    const count = mark.querySelector('.mn');
    const r = mark.getBoundingClientRect();
    const num = d.querySelector('.num'), marks = d.querySelector('.marks');
    return {
      key: d.dataset.key, over: mark.classList.contains('over'),
      discs: mark.querySelectorAll('.sil.ov .db').length, shapes: mark.querySelectorAll('.sil.ov .dl').length,
      count: count ? count.textContent : '',
      tinted: [...mark.querySelectorAll('.dl')].filter((s) => s.style.getPropertyValue('--mc')).length,
      onNumber: !!num && hit(r, num.getBoundingClientRect()),
      onMarks: !!marks && hit(r, marks.getBoundingClientRect()),
      onNeighbour: all.some((o) => o.m !== mark && hit(r, o.r)),
      inCell: r.left >= cell.left - 0.5 && r.right <= cell.right + 0.5,
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

test('four moons on a ten-day week are one disc with three lit shapes and a +1, for the owner and a player', async () => {
  const { owner, player, cid: campaign } = await world();
  const { base, cal } = await calendarFor(owner, campaign, (cid) => owner.api(`/campaigns/${cid}/calendars/presets/harptos`, { method: 'POST', body: {} }));
  const { page } = owner;
  await addMarks(owner, base, cal);
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();

  const wide = await readMonth(page);
  assert.ok(wide.length >= 10, 'the month drew too few moon marks');
  for (const d of wide) {
    assert.ok(d.over && d.discs === 1, `${d.key} is not one shared disc`);
    // A dark moon has no lit shape, so a day lays up to three.
    assert.ok(d.shapes <= 3, `${d.key} lays ${d.shapes} lit shapes on the disc`);
    assert.equal(d.count, '+1', `${d.key} counts ${d.count || 'nothing'} for the fourth moon`);
    assert.equal(d.tinted, d.shapes, `${d.key} has a moon with no colour of its own`);
    assert.ok(!d.onNumber, `${d.key}: the moons sit on the day number`);
    assert.ok(!d.onMarks, `${d.key}: the moons sit on the event marks`);
    assert.ok(d.inCell, `${d.key}: the moons spill out of the cell`);
  }
  assert.ok(wide.some((d) => d.shapes === 3), 'no day of the month lays three lit shapes');
  await picture(page, 'calendar-several-moons-desktop-light', { check: '#2482', about: 'Owner month, four moons on a ten-day week, desktop', locator: page.locator('#cal5-cal') });

  await page.emulateMedia({ colorScheme: 'dark' });
  await picture(page, 'calendar-several-moons-desktop-dark', { check: '#2482', about: 'The same month in dark', locator: page.locator('#cal5-cal') });
  await page.emulateMedia({ colorScheme: 'light' });

  await page.setViewportSize(PHONE);
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();
  const narrow = await readMonth(page);
  for (const d of narrow) {
    // The mark is one disc wide, so a ten-day week's phone cell holds it with its count.
    assert.ok(d.over && d.discs === 1 && d.shapes <= 3, `${d.key}: a phone draws ${d.shapes} shapes on ${d.discs} discs`);
    assert.equal(d.count, '+1', `${d.key}: a phone counts ${d.count || 'nothing'}`);
    assert.ok(!d.onNumber && !d.onMarks, `${d.key}: the moons sit on the number or the event marks on a phone`);
    assert.ok(d.inCell, `${d.key}: the moons spill out of the cell on a phone`);
  }
  assert.ok(narrow.every((d) => !d.onNeighbour), 'a day\'s moons run under the next day\'s on a phone');
  await picture(page, 'calendar-several-moons-phone-light', { check: '#2482', about: 'Owner month, four moons, phone width', locator: page.locator('#cal5-cal') });

  // The narrowest phone: the count sits under the disc, so a day's mark stays inside its own cell.
  await page.setViewportSize(SMALL);
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();
  for (const d of await readMonth(page)) {
    assert.equal(d.count, '+1', `${d.key}: the narrowest phone counts ${d.count || 'nothing'}`);
    assert.ok(!d.onNeighbour, `${d.key}: the moons run under the next day's at 320px`);
    assert.ok(!d.onNumber && !d.onMarks && d.inCell, `${d.key}: the moons collide at 320px`);
  }
  await picture(page, 'calendar-several-moons-small-phone-light', { check: '#2532', about: 'Owner month, four moons on a ten-day week, 320px wide', locator: page.locator('#cal5-cal') });
  await page.setViewportSize(DESKTOP);

  // A player sees the same moons; when the Director hides one, the player
  // sees three and the Director still sees all four.
  await player.go(`${base}/view`);
  await player.page.locator('.cal-v5 .day .msil').first().waitFor();
  assert.ok((await readMonth(player.page)).every((d) => d.shapes <= 3 && d.count === '+1'), 'a player does not see the same four moons');

  const moons = cal.moons || (await owner.api(base)).body.moons;
  assert.equal(moons.length, 4);
  const hid = await owner.api(`${base}/moons/${moons[1].id}/hidden`, { method: 'PUT', body: { hidden: true } });
  assert.ok(hid.ok, `hiding a moon answered ${hid.status}`);
  await player.go(`${base}/view`);
  await player.page.locator('.cal-v5 .day .msil').first().waitFor();
  assert.ok((await readMonth(player.page)).every((d) => d.shapes <= 3 && d.count === ''), 'the player still sees the hidden moon');
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();
  assert.ok((await readMonth(page)).every((d) => d.shapes <= 3 && d.count === '+1'), 'the Director lost a moon by hiding it from players');
  owner.assertClean();
  player.assertClean();
});

test('two moons on a six-day week are one disc with two tinted shapes a day, clear of the day number', async () => {
  const { owner, cid: campaign } = await world();
  const { base, cal } = await calendarFor(owner, campaign, (cid) => twoMoonsOnSixDays(owner, cid));
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
      assert.ok(d.over && d.discs === 1 && d.shapes <= 2 && d.count === '' && d.tinted === d.shapes, `${d.key} at ${width.width}px: ${JSON.stringify(d)}`);
      assert.ok(!d.onNumber && !d.onMarks && d.inCell, `${d.key} at ${width.width}px: the moons collide`);
    }
    assert.ok(days.some((d) => d.shapes === 2), `no day lays both moons' lit shapes at ${width.width}px`);
    await picture(page, `calendar-two-moons-${width.width < 600 ? 'phone' : 'desktop'}`, { check: '#2482', about: `Owner month, two moons on a six-day week, ${width.width}px`, locator: page.locator('#cal5-cal') });
  }
  owner.assertClean();
});

test('a pale moon and a deep red one keep their own colours, in light and dark', async () => {
  const { owner, cid: campaign } = await world();
  const { base } = await calendarFor(owner, campaign, (cid) => twoMoonsOnSixDays(owner, cid, THERIN));
  const { page } = owner;
  for (const width of [DESKTOP, PHONE]) {
    await page.setViewportSize(width);
    await owner.go(`${base}/view`);
    await page.locator('.cal-v5 .day .msil').first().waitFor();
    const days = await readMonth(page);
    // Half a cycle apart, one moon is always waxing while the other wanes; the disc is never empty.
    assert.ok(days.every((d) => d.over && d.discs === 1 && d.shapes >= 1 && d.tinted === d.shapes), `a day lost its moons at ${width.width}px`);
    assert.ok(days.every((d) => !d.onNumber && !d.onNeighbour && d.inCell), `the moons collide at ${width.width}px`);
    const size = width.width < 600 ? 'phone' : 'desktop';
    for (const scheme of ['light', 'dark']) {
      await page.emulateMedia({ colorScheme: scheme });
      await picture(page, `calendar-pale-and-red-moons-${size}-${scheme}`, { check: '#2532', about: `Owner month, a pale moon and a deep red moon half a cycle apart, ${width.width}px, ${scheme}`, locator: page.locator('#cal5-cal') });
    }
    await page.emulateMedia({ colorScheme: 'light' });
  }
  owner.assertClean();
});

test('an open day keeps its moon, whether or not its event marks fly into the card', async () => {
  const { owner, cid: campaign } = await world();
  // Full motion, so a day with events packs its marks into the card.
  await owner.page.emulateMedia({ reducedMotion: 'no-preference' });
  await owner.page.setViewportSize(DESKTOP);
  const { base, cal } = await calendarFor(owner, campaign, (cid) => twoMoonsOnSixDays(owner, cid, THERIN));
  const { page } = owner;
  await addMarks(owner, base, cal);
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();
  const moonOf = (key) => page.evaluate((k) => {
    const day = document.querySelector(`.cal-v5 .day[data-key="${k}"]`);
    const m = day && day.querySelector('.msil');
    if (!m) return null;
    const cs = getComputedStyle(m);
    return { packed: day.classList.contains('packed'), opacity: cs.opacity, visibility: cs.visibility };
  }, key);
  const y = cal.current_year, mo = cal.current_month;
  // Day 2 has event marks (they fly into the card); day 10 has none.
  for (const [d, withEvents] of [[2, true], [10, false]]) {
    const key = `${y}_${mo}_${d}`;
    await page.locator(`.cal-v5 .day[data-key="${key}"]`).click();
    await page.waitForTimeout(900);
    const m = await moonOf(key);
    assert.ok(m, `day ${d} drew no moon`);
    if (withEvents) assert.ok(m.packed, 'a day with events did not pack its marks, so this check proves nothing');
    assert.equal(m.visibility, 'visible', `day ${d}'s moon is hidden while its card is open`);
    assert.equal(m.opacity, '1', `day ${d}'s moon faded while its card is open`);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
  }
  await page.emulateMedia({ reducedMotion: 'reduce' });
  owner.assertClean();
});
