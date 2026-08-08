// bench_date_verbs.test.mjs — C-CALV4-GAMEREADY §2, [GR-SIGN-A] SIGNED and
// [GR-4].
//
// WHAT GOES ON THE WIRE, AND NOTHING ELSE. The two step verbs and the
// Owner-only Set date all write through the EXISTING
// `PUT /campaigns/:id/calendar/world-state`, with the payload `gm_panel.js`
// already sends. This suite pins the METHOD, the URL and the BODY, because the
// endpoint's contract is the one thing a client can get wrong silently: an
// `advance` key that drifted would answer 200 and change no date, and a GM at
// a table would believe it worked.
//
// THE ROLLOVER IS DELIBERATELY NOT TESTED HERE, and that is the assertion.
// `{advance:{days:±1}}` is all the client sends — month ends, year ends and
// leap geometry are decided ONCE, server-side, exactly as they are for V2's
// console. A month-end case in this file would mean the client had grown its
// own copy of the calendar's arithmetic, which is how two surfaces start
// disagreeing about when a year turns over.
//
// PERMISSION IS ABSENCE IS THE PRODUCER'S, AND THIS PINS THE OTHER HALF: with
// no verb row in the DOM the module writes nothing at all. It never fabricates
// a control the server did not render.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './daycard_harness.mjs';

const OWNER = { canStep: true, canSet: true, year: 1523, month: 1, day: 14 };

function clickVerb(fx, sel) {
  fx.fire('click', fx.verbRow.querySelector(sel));
}

test('+1 day sends exactly {advance:{days:1}} to the existing world-state PUT', () => {
  const fx = boot({ dateVerbs: OWNER });
  clickVerb(fx, '[data-bench-date-step="1"]');

  assert.equal(fx.calls.length, 1, 'one verb click is one request');
  const call = fx.calls[0];
  assert.equal(call.method, 'PUT');
  assert.equal(call.url, '/campaigns/camp-1/calendar/world-state?calendarId=cal-1',
    'the verbs act on the Block they sit under, via the handler\'s own ?calendarId=');
  // Field-by-field, not deepEqual: the body is minted inside the module's own
  // `vm` realm, so its Object prototype is not this realm's and a structural
  // comparison fails on provenance rather than on content.
  assert.equal(call.body.advance.days, 1);
  assert.equal(call.body.advance.hours, 0);
  assert.equal(call.body.advance.minutes, 0);
  assert.equal(Object.keys(call.body).length, 1, 'a step verb sends `advance` and nothing else');
});

test('−1 day is the same endpoint with the sign flipped — the undo for a fat-finger +1', () => {
  const fx = boot({ dateVerbs: OWNER });
  clickVerb(fx, '[data-bench-date-step="-1"]');

  assert.equal(fx.calls.length, 1);
  assert.equal(fx.calls[0].body.advance.days, -1);
  assert.equal(fx.calls[0].body.advance.hours, 0);
  assert.equal(fx.calls[0].body.advance.minutes, 0);
});

test('the client sends NO clock quantities of its own', () => {
  const fx = boot({ dateVerbs: OWNER });
  clickVerb(fx, '[data-bench-date-step="1"]');
  const body = fx.calls[0].body;
  assert.equal(body.advance.hours, 0);
  assert.equal(body.advance.minutes, 0);
  assert.equal(body.time, undefined, 'a step verb must not also set an absolute time');
  assert.equal(body.weather, undefined, 'weather is C-CALV4-GM-CONSOLE\'s, not this slice\'s');
  assert.equal(body.moodTint, undefined, 'mood is C-CALV4-GM-CONSOLE\'s, not this slice\'s');
  assert.equal(body.triggerEvent, undefined, 'event triggers are C-CALV4-GM-CONSOLE\'s');
});

test('Set date sends the three fields as an absolute {time:{…}}', () => {
  const fx = boot({ dateVerbs: OWNER });
  fx.verbRow.querySelector('[data-bench-date-year]').value = '1524';
  fx.verbRow.querySelector('[data-bench-date-month]').value = '3';
  fx.verbRow.querySelector('[data-bench-date-day]').value = '9';
  clickVerb(fx, '[data-bench-date-set]');

  assert.equal(fx.calls.length, 1);
  assert.equal(fx.calls[0].method, 'PUT');
  const t = fx.calls[0].body.time;
  assert.equal(t.year, 1524);
  assert.equal(t.month, 3);
  assert.equal(t.day, 9);
  assert.equal(Object.keys(fx.calls[0].body).length, 1, 'Set date sends `time` and nothing else');
  assert.equal(t.hour, undefined, 'v4 has no clock, so Set date sets no hour');
  assert.equal(t.minute, undefined);
});

test('a co-DM fixture has no Set date control at all, so nothing can send one', () => {
  const fx = boot({ dateVerbs: { canStep: true, canSet: false } });
  assert.equal(fx.verbRow.querySelector('[data-bench-date-set]'), null,
    'permission is ABSENCE — a co-DM gets no Set date element, not a disabled one');
  clickVerb(fx, '[data-bench-date-step="1"]');
  assert.equal(fx.calls.length, 1, 'the step verbs still work at the co-DM floor');
});

test('with no verb row in the DOM the module writes nothing', () => {
  const fx = boot();
  assert.equal(fx.verbRow, null, 'a viewer below both floors gets no row at all');
  // A click anywhere that is not a verb must not reach the world-state
  // endpoint. Opening a day card is the busiest click on this page.
  fx.fire('click', fx.cells[3]);
  const worldState = fx.calls.filter((c) => c.url.indexOf('world-state') >= 0);
  assert.equal(worldState.length, 0);
});

test('a successful write RELOADS rather than hand-patching the nameplate', async () => {
  const fx = boot({ dateVerbs: OWNER });
  clickVerb(fx, '[data-bench-date-step="1"]');
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(fx.reloads(), 1,
    'the date is printed by the Block\'s own projection, and this module may read ' +
    'the Block\'s DOM but never mutate it — a reload is the honest re-read');
});

test('a refused write says so and re-arms the control instead of reloading', async () => {
  const fx = boot({
    dateVerbs: OWNER,
    responses: {
      PUT: { ok: false, json: () => Promise.resolve({ message: 'world-state control requires Owner or co-DM access' }) },
    },
  });
  clickVerb(fx, '[data-bench-date-step="1"]');
  for (let i = 0; i < 6; i++) await Promise.resolve();

  assert.equal(fx.reloads(), 0, 'a refused write must not reload a page that did not change');
  const say = fx.verbRow.querySelector('[data-bench-date-say]');
  assert.match(say.textContent, /Owner or co-DM/,
    'the server\'s own sentence is printed, never a cheerful invention');
  assert.equal(fx.verbRow.querySelector('[data-bench-date-step="1"]').disabled, false,
    're-armed, so the GM can try again');
});
