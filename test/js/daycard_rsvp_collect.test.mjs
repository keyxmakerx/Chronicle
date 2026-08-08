// daycard_rsvp_collect.test.mjs — C-CALV4-GAMEREADY §4 [GR-6] and §5 [GR-10].
//
// THE FINDING. "Collect RSVPs" is the operator's OWN stated go/no-go gate for
// starting their game, and it was reachable from exactly one place in the entire
// product: the LEGACY V2 event drawer, wired by event_grid.js, which
// calendar_v2.templ is the only page to load — and the V2 shell is a committed
// deletion. `daycard.templ` contained ZERO occurrences of "rsvp". Meanwhile
// every downstream RSVP surface, the Bench session tile AND /schedule, is gated
// on that flag: a campaign that could not reach the switch got a player-facing
// page telling them they owed an answer and giving them no buttons to answer
// with.
//
// WHAT THIS SUITE PINS, and each row is one clause of the ruling:
//
//   the control is ABSENT FROM THE DOM below RoleScribe (the audience rule is
//     "permission is absence", and this module must never fabricate a control
//     the producer did not render);
//   it is DISABLED IN CREATE MODE with the V2 drawer's own hint VERBATIM,
//     because collect_rsvps cannot ride the create payload;
//   it is LIVE IN EDIT MODE and reflects the stored flag;
//   it PUTs the already-shipped route with the already-shipped body;
//   it does NOT reload the page, unlike save and delete;
//   a REFUSED write puts the checkbox back;
//   the honest mail sentence is PRINTED FROM THE RESPONSE, never invented.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './daycard_harness.mjs';

const COLLECT_URL = '/campaigns/camp-1/calendars/cal-1/events/ev-1/rsvp-collection';

// The exact sentence rsvp_handler.go's mailNotConfiguredLine holds. It is
// asserted here as a STRING because this side of the wire cannot import a Go
// constant — the Go guard (TestRSVPCollect_ReportsMailState) asserts the server
// emits the CONSTANT rather than a copy, so between the two there is no seam a
// reworded sentence could slip through.
const MAIL_NOT_CONFIGURED =
  'Email is not configured on this server — answers still work in-app; nobody was emailed.';

function openEditor(fx, day) {
  fx.fire('click', fx.cells[day]);
  fx.fire('click', fx.card.querySelector('[data-dc-new]'));
}

async function openEditDoor(fx, stored) {
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-edit]'));
  await new Promise((r) => setImmediate(r));
  return stored;
}

function storedEvent(extra) {
  return {
    id: 'ev-1', calendar_id: 'cal-1', name: 'Council of Wards',
    year: 1523, month: 1, day: 3, all_day: true, visibility: 'everyone',
    ...(extra || {}),
  };
}

function bootEditing(stored, responses) {
  return boot({
    responses: {
      'GET /campaigns/camp-1/calendars/cal-1/events/ev-1': {
        ok: true, json: () => Promise.resolve(stored),
      },
      ...(responses || {}),
    },
  });
}

// --- the audience rule ------------------------------------------------------

test('below RoleScribe the control is ABSENT FROM THE DOM, and nothing writes', async () => {
  // The producer renders no markup for a viewer without the floor, so this is
  // the state a Player's page is genuinely in — not a hidden control, not a
  // disabled one. `permission is absence`.
  const fx = boot({ canCollectRSVPs: false });
  assert.equal(fx.editor.querySelector('[data-de-rsvp-toggle]'), null,
    'a viewer below the route’s own RoleScribe floor must receive no collect control at all');
  assert.equal(fx.editor.querySelector('[data-de-rsvp]'), null);

  openEditor(fx, 3);
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.filter((c) => c.url.includes('rsvp-collection')).length, 0,
    'with no control rendered the module must issue no collection write');
});

// --- create mode ------------------------------------------------------------

test('in CREATE mode the control is disabled and carries the V2 drawer’s hint verbatim', () => {
  const fx = boot();
  openEditor(fx, 4);
  const box = fx.editor.querySelector('[data-de-rsvp-toggle]');
  assert.equal(box.disabled, true,
    'there is no event id to collect against yet, so the control is disabled BY SEQUENCE');
  assert.equal(box.checked, false);
  assert.equal(fx.editor.querySelector('[data-de-rsvp-hint]').textContent,
    'Save the event first, then invite the party',
    'the create-mode hint is the V2 drawer’s own wording, reused rather than reinvented');
});

// --- edit mode --------------------------------------------------------------

test('in EDIT mode the control is live and reflects the STORED flag', async () => {
  for (const stored of [true, false]) {
    const fx = bootEditing(storedEvent({ collect_rsvps: stored }));
    await openEditDoor(fx);
    const box = fx.editor.querySelector('[data-de-rsvp-toggle]');
    assert.equal(box.disabled, false, 'an event that exists can be collected against');
    assert.equal(box.checked, stored,
      'the control must show what the server holds, not a default');
  }
});

test('turning it on PUTs the ALREADY-SHIPPED route and does not reload the page', async () => {
  const fx = bootEditing(storedEvent({ collect_rsvps: false }), {
    ['PUT ' + COLLECT_URL]: {
      ok: true, json: () => Promise.resolve({ collect_rsvps: true, emailed: true }),
    },
  });
  await openEditDoor(fx);

  const box = fx.editor.querySelector('[data-de-rsvp-toggle]');
  box.checked = true;
  fx.fireOn('change', box);
  await new Promise((r) => setImmediate(r));

  const put = fx.calls.find((c) => c.url === COLLECT_URL);
  assert.ok(put, 'the toggle never reached the shipped rsvp-collection endpoint');
  assert.equal(put.method, 'PUT');
  assert.equal(put.body.enabled, true, 'the shipped body is {"enabled":bool} and nothing else');
  assert.deepEqual(Object.keys(put.body), ['enabled'],
    'the collection PUT takes exactly one key; a widened body is a different endpoint');

  // Arming RSVPs changes no date, no title and no mark, so throwing the GM's
  // open editor away to re-render an identical grid would cost them their place
  // for nothing. Save and delete reload; this does not.
  assert.equal(fx.reloads(), 0, 'arming RSVPs must not reload the page out from under the GM');
});

test('turning it OFF sends enabled:false — the same route, the other way', async () => {
  const fx = bootEditing(storedEvent({ collect_rsvps: true }), {
    ['PUT ' + COLLECT_URL]: { ok: true, json: () => Promise.resolve({ collect_rsvps: false }) },
  });
  await openEditDoor(fx);
  const box = fx.editor.querySelector('[data-de-rsvp-toggle]');
  box.checked = false;
  fx.fireOn('change', box);
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.find((c) => c.url === COLLECT_URL).body.enabled, false);
});

// --- [GR-10] the honest sentence -------------------------------------------

test('with SMTP unconfigured the control prints the SERVER’S notice, not a success line', async () => {
  // THE MEASURED DEFECT, on the V2 client this replaces: with
  // mailer.IsConfigured=false the PUT returned a flat 200 and the client
  // UNCONDITIONALLY printed "RSVPs are open — the party has been invited."
  // Mail attempts were ZERO. The operator armed their gate, believed the
  // sentence, stopped checking, and found out on the day of the session.
  const fx = bootEditing(storedEvent({ collect_rsvps: false }), {
    ['PUT ' + COLLECT_URL]: {
      ok: true,
      json: () => Promise.resolve({
        collect_rsvps: true, emailed: false, notice: MAIL_NOT_CONFIGURED,
      }),
    },
  });
  await openEditDoor(fx);
  const box = fx.editor.querySelector('[data-de-rsvp-toggle]');
  box.checked = true;
  fx.fireOn('change', box);
  await new Promise((r) => setImmediate(r));

  const hint = fx.editor.querySelector('[data-de-rsvp-hint]').textContent;
  assert.equal(hint, MAIL_NOT_CONFIGURED,
    'with no mail server the control must say so — never "the party has been invited"');
  assert.ok(!/invited/i.test(hint), 'nobody was invited, so nothing may claim they were');
});

test('with SMTP working the notice is absent and the control says so plainly', async () => {
  const fx = bootEditing(storedEvent({ collect_rsvps: false }), {
    ['PUT ' + COLLECT_URL]: {
      ok: true, json: () => Promise.resolve({ collect_rsvps: true, emailed: true }),
    },
  });
  await openEditDoor(fx);
  const box = fx.editor.querySelector('[data-de-rsvp-toggle]');
  box.checked = true;
  fx.fireOn('change', box);
  await new Promise((r) => setImmediate(r));

  const hint = fx.editor.querySelector('[data-de-rsvp-hint]').textContent;
  assert.ok(!hint.includes('not configured'),
    'a working mail server must not be reported as a missing one');
  assert.match(hint, /email/i);
});

// --- the refusal ------------------------------------------------------------

test('a REFUSED write puts the checkbox back to what the server still holds', async () => {
  const fx = bootEditing(storedEvent({ collect_rsvps: false }), {
    ['PUT ' + COLLECT_URL]: { ok: false, json: () => Promise.resolve({}) },
  });
  await openEditDoor(fx);
  const box = fx.editor.querySelector('[data-de-rsvp-toggle]');
  box.checked = true;
  fx.fireOn('change', box);
  await new Promise((r) => setImmediate(r));

  assert.equal(box.checked, false,
    'a checkbox left showing a state the server refused is the same lie in a smaller font');
  assert.equal(fx.editor.querySelector('[data-de-err]').hidden, false,
    'and the refusal is said out loud');
});

// --- the recurring caution (the RSVP-OCCURRENCE booking, surfaced) ----------

test('a RECURRING event carries the shared-answers caution', async () => {
  // The RSVP table is UNIQUE (event_id, user_id) with NO occurrence column, so
  // every occurrence of a repeating session shares ONE set of answers: after
  // week one the tally shows last week's replies and nobody can reset them. The
  // fix is a schema change and is BOOKED (C-CALV4-RSVP-OCCURRENCE); the operator
  // being TOLD is what ships now.
  const fx = bootEditing(storedEvent({ collect_rsvps: true, is_recurring: true }));
  await openEditDoor(fx);
  assert.match(fx.editor.querySelector('[data-de-rsvp-hint]').textContent,
    /repeats.*one set of answers/,
    'a repeating session must warn that its answers are shared across occurrences');
});

test('a ONE-OFF event carries no such caution', async () => {
  const fx = bootEditing(storedEvent({ collect_rsvps: true, is_recurring: false }));
  await openEditDoor(fx);
  assert.ok(!/repeats/.test(fx.editor.querySelector('[data-de-rsvp-hint]').textContent),
    'a non-recurring event must not be warned about recurrence');
});
