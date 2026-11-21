// calendar_day_sheet_phone.spec.mjs — on a phone the calendar's Day sheet
// scrolls by touch to its last row, and that row ends above the docked Table
// bar instead of behind it.
//
// RUN: tools/live-checks.sh test/live/calendar_day_sheet_phone.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, enableAddons, done, picture } from './_live.mjs';

after(done);

const PHONE = { viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true };

test('the day sheet scrolls by touch to its last row, above the Table bar', async () => {
  const owner = await person({ tag: 'sheet', context: PHONE });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Day Sheet Phone');
  await enableAddons(owner, cid, '*');
  const made = await owner.api(`/campaigns/${cid}/calendars/presets/harptos`, { method: 'POST', body: {} });
  assert.equal(made.status, 201, `making the calendar answered ${made.status}`);
  const base = `/campaigns/${cid}/calendars/${made.body.calendar.id}`;
  await owner.go(`${base}/view`);
  const utc = page.getByRole('button', { name: 'Use UTC time' });
  if (await utc.count()) { await utc.first().click(); await owner.go(`${base}/view`); }
  const cal = (await owner.api(base)).body;
  for (let i = 1; i <= 10; i++) {
    const ev = await owner.api(`${base}/events`, { method: 'POST', body: { name: `Long event ${i}`, year: cal.current_year, month: cal.current_month, day: 10, all_day: true, visibility: 'everyone' } });
    assert.equal(ev.status, 201, `creating an event answered ${ev.status}`);
  }
  await owner.go(`${base}/view`);

  await page.locator(".cal-v5 .day[data-key$=\"_10\"]").first().tap({ position: { x: 4, y: 4 } });
  const wing = page.locator('#cal5-wing.open');
  await wing.waitFor();

  const read = () => page.evaluate(() => {
    const w = document.getElementById('cal5-wing');
    const bar = document.querySelector('.tt-sheet');
    const last = [...w.querySelectorAll('.moonsec > *, .evd')].pop();
    return {
      scrollable: w.scrollHeight > w.clientHeight + 1, top: w.scrollTop,
      sheetBottom: w.getBoundingClientRect().bottom, lastBottom: last.getBoundingClientRect().bottom,
      barTop: bar ? bar.getBoundingClientRect().top : innerHeight,
      sheetTop: w.getBoundingClientRect().top,
    };
  });
  const before = await read();
  assert.ok(before.scrollable, 'the sheet has more than fits and scrolls');
  assert.ok(before.sheetBottom <= before.barTop + 1, `the sheet ends ${Math.round(before.sheetBottom - before.barTop)}px behind the bar`);

  // A real touch drag: finger down in the middle of the sheet, up, then back.
  const cdp = await page.context().newCDPSession(page);
  const touch = (type, y) => cdp.send('Input.dispatchTouchEvent', { type, touchPoints: type === 'touchEnd' ? [] : [{ x: 195, y }] });
  const drag = async (from, to) => {
    const step = to < from ? -20 : 20;
    await touch('touchStart', from);
    for (let y = from; step < 0 ? y >= to : y <= to; y += step) await touch('touchMove', y);
    await touch('touchEnd');
    await page.waitForTimeout(400);
  };
  await drag(600, 200);
  const down = await read();
  assert.ok(down.top > 0, 'a touch drag moved the sheet');
  assert.ok(down.lastBottom <= down.barTop + 1, `the last row ends ${Math.round(down.lastBottom - down.barTop)}px behind the bar`);
  await drag(200, 700);
  assert.equal((await read()).top, 0, 'a drag the other way scrolls back up');
  await picture(page, 'calendar-day-sheet-phone', { check: 'day-sheet', about: 'Day sheet on a phone, scrolled', locator: page.locator('body') });
  owner.assertClean();
});
