// calendar_outside_days.spec.mjs — the month's weeks read as one run of days.
// A week's cells outside the month show the neighbouring month's dates, faded,
// over the same era colour as the month's own days; next to a festival month
// they stay blank.
//
// RUN: tools/live-checks.sh test/live/calendar_outside_days.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { person, newCampaign, enableAddons, done, picture, repoRoot } from './_live.mjs';

after(done);

const THERIN = [
  { name: 'Lacrimosa', cycle_days: 24, phase_offset: 0, color: '#e5e7eb' },
  { name: 'Sanguin’mor', cycle_days: 24, phase_offset: 12, color: '#b91c1c' },
];

// The Imperial preset with 31-day months that run on from week to week, so
// each month starts one column later than the last and most weeks have cells
// outside the month. Its year ends in a festival month.
async function runOnCalendar(owner, cid) {
  const file = JSON.parse(readFileSync(join(repoRoot, 'internal/plugins/calendar/presets/imperial.json'), 'utf8'));
  file.calendar.moons = THERIN;
  file.calendar.month_starts_new_week = false;
  file.calendar.months.forEach((m) => { if (!m.is_intercalary) m.days = 31; });
  file.calendar.seasons = [];
  return owner.page.evaluate(async ({ cid, text }) => {
    const form = new FormData();
    form.append('file', new File([text], 'calendar.json', { type: 'application/json' }));
    const res = await Chronicle.apiFetch(`/campaigns/${cid}/calendars/import`, { method: 'POST', body: form });
    return { status: res.status, body: await res.json().catch(() => ({})) };
  }, { cid, text: JSON.stringify(file) });
}

// The shown month's cells in order: an outside cell gives its date (or null
// when blank), a day of the month gives 'in'.
const readCells = (page) => page.evaluate(() => [...document.querySelectorAll('.cal-v5 .stage > .month:not(.leaving) .wk > .day')].map((d) => {
  if (!d.classList.contains('out')) return 'in';
  const n = d.querySelector('.num');
  return n ? Number(n.textContent) : null;
}));
const split = (cells) => {
  const first = cells.indexOf('in'), last = cells.lastIndexOf('in');
  return { lead: cells.slice(0, first), trail: cells.slice(last + 1) };
};

test('the weeks show the neighbouring dates, faded, over the era colour', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Outside Days');
  await enableAddons(owner, cid, '*');
  const made = await runOnCalendar(owner, cid);
  assert.equal(made.status, 201, `importing the calendar answered ${made.status}`);
  const cal = made.body.calendar;
  const base = `/campaigns/${cid}/calendars/${cal.id}`;
  await owner.go(`${base}/view`);
  await page.locator('.cal-v5 .day .msil').first().waitFor();

  // Month 1 follows the festival month: its leading cells stay blank.
  const first = split(await readCells(page));
  assert.ok(first.lead.every((n) => n === null), `the cells before month 1 borrow dates from a festival month: ${first.lead}`);

  // Over three months, leading cells end at the month before's last day and
  // trailing cells count up from the 1st.
  let leads = 0, trails = 0;
  for (let i = 0; i < 3; i++) {
    const { lead, trail } = split(await readCells(page));
    if (i > 0 && lead.length) { leads++; assert.deepEqual(lead, lead.map((_, k) => 31 - lead.length + 1 + k), 'the leading cells are not the month before’s last days'); }
    if (trail.length) { trails++; assert.deepEqual(trail, trail.map((_, k) => k + 1), 'the trailing cells are not the next month’s first days'); }
    if (i === 0) {
      // The faded dates and the era colour under every cell.
      const look = await page.evaluate(() => {
        const out = document.querySelector('.cal-v5 .day.out .num'), field = document.querySelector('.cal-v5 canvas.era-field:not(.era-fade)');
        const month = document.querySelector('.cal-v5 .stage > .month');
        return {
          outOpacity: out ? Number(getComputedStyle(out).opacity) : null,
          outButton: !!document.querySelector('.cal-v5 button.day.out'),
          clip: field ? field.style.clipPath : null, monthH: month.offsetHeight, monthW: month.offsetWidth
        };
      });
      if (look.outOpacity !== null) assert.ok(look.outOpacity < 0.8, 'the neighbouring dates are not faded');
      assert.equal(look.outButton, false, 'an outside cell is a button');
      assert.ok(look.clip, 'the era colour is not drawn');
      assert.match(look.clip, new RegExp(`^polygon\\(0px 0px, ${look.monthW}px 0px, ${look.monthW}px ${look.monthH}px, 0px ${look.monthH}px\\)$`), `the era colour does not cover every cell: ${look.clip}`);
      await picture(page, 'calendar-outside-days', { check: 'calendar-look', about: 'Owner month: neighbouring dates faded over the era colour', locator: page.locator('#cal5-cal') });
    }
    await page.locator('#cal5-next').click();
    await page.waitForTimeout(700);
  }
  assert.ok(leads > 0, 'no month after the first had leading cells, so this check proves nothing');
  assert.ok(trails > 0, 'no month had trailing cells, so this check proves nothing');
  owner.assertClean();
});
