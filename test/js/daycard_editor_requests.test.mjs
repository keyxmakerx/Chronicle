// daycard_editor_requests.test.mjs — C-CALV4-DAYCARD (R2-2a) stage 2.
//
// THE EDITOR IS A CLIENT OF THE SHIPPED EVENT API AND NOTHING ELSE. This suite
// pins what actually goes on the wire: the METHOD, the URL and the BODY, for
// create, update and delete — plus the two things a body can get wrong quietly.
//
//   1. A DROPPED AUDIENCE. PUT re-writes the whole record, so an editor that
//      omitted `visibility_rules` because it has no chip row yet would DESTROY
//      an event's restriction on the first save. The only symptom would be
//      players seeing something they should not, days later.
//   2. A ZERO THAT MEANS MIDNIGHT. An empty hour field must serialise as null,
//      not 0 — "no time" is the absence of a value, and hour 0 is a real time.
//
// The role gates themselves are markup-level and the producer's ([DC-9]); what
// is asserted here is that the module OBEYS them — it never fabricates a
// control the server did not render, and with no editor DOM it writes nothing.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './daycard_harness.mjs';

const P = boot().pure;

function openEditorOn(fx, day) {
  fx.fire('click', fx.cells[day]);
  fx.fire('click', fx.card.querySelector('[data-dc-new]'));
}

function setForm(fx, values) {
  const ed = fx.editor;
  const set = (sel, v) => { const n = ed.querySelector(sel); if (n) n.value = v; };
  set('[data-de-name]', values.name ?? '');
  set('[data-de-desc]', values.description ?? '');
  set('[data-de-year]', values.year ?? '');
  set('[data-de-month]', values.month ?? '');
  set('[data-de-day]', values.day ?? '');
}

// --- the pure body builder ---------------------------------------------------

test('an empty time field is null, never a zero that means midnight', () => {
  assert.equal(P.numOrNull(''), null);
  assert.equal(P.numOrNull('   '), null);
  assert.equal(P.numOrNull(null), null);
  assert.equal(P.numOrNull('0'), 0);
  assert.equal(P.numOrNull('18'), 18);
  assert.equal(P.numOrNull('nope'), null);
});

test('all-day clears the time fields rather than sending stale ones', () => {
  const body = P.buildEventBody({
    name: 'Feast', year: '1523', month: '1', day: '3',
    allDay: true, startHour: '18', startMinute: '30',
  }, null, {});
  assert.equal(body.all_day, true);
  assert.equal(body.start_hour, null);
  assert.equal(body.start_minute, null);
});

test('a timed event carries the in-world hour and minute the API has always had', () => {
  const body = P.buildEventBody({
    name: 'Feast', year: '1523', month: '1', day: '3',
    allDay: false, startHour: '18', startMinute: '0', endHour: '21', endMinute: '30',
  }, null, {});
  assert.equal(body.start_hour, 18);
  assert.equal(body.start_minute, 0);
  assert.equal(body.end_hour, 21);
  assert.equal(body.end_minute, 30);
});

test('a create body defaults to everyone with no rules', () => {
  const body = P.buildEventBody({ name: 'x', year: '1', month: '1', day: '1', allDay: true }, null, {});
  assert.equal(body.visibility, 'everyone');
  assert.equal(body.visibility_rules, null);
});

test('the GM-only control writes dm_only through the SHARED mapper', () => {
  const on = P.buildEventBody(
    { name: 'x', year: '1', month: '1', day: '1', allDay: true, gmOnly: true },
    null, { canOfferGMOnly: true });
  assert.equal(on.visibility, 'dm_only');
  assert.equal(on.visibility_rules, null);

  const off = P.buildEventBody(
    { name: 'x', year: '1', month: '1', day: '1', allDay: true, gmOnly: false },
    null, { canOfferGMOnly: true });
  assert.equal(off.visibility, 'everyone');
});

test('WITHOUT the capability the stored visibility round-trips untouched', () => {
  // A plain Scribe has no GM-only control, and the server would downgrade a
  // dm_only they sent anyway. What must NOT happen is the editor silently
  // un-hiding a GM's event by sending "everyone" on their behalf.
  const body = P.buildEventBody(
    { name: 'x', year: '1', month: '1', day: '1', allDay: true },
    { id: 'ev-9', visibility: 'dm_only', visibility_rules: null },
    { canOfferGMOnly: false });
  assert.equal(body.visibility, 'dm_only');
});

test('a RESTRICTED event keeps its audience even for a viewer who can set dm_only', () => {
  // This is the defect the whole round-trip exists to prevent. The chip row is
  // stage-3 chrome, so an event whose mode is `specific` has no control here —
  // and its rules must survive the save byte for byte.
  const rules = '{"allowed_users":["u-gm","u-nissa"]}';
  const body = P.buildEventBody(
    { name: 'x', year: '1', month: '1', day: '1', allDay: true, gmOnly: false },
    { id: 'ev-9', visibility: 'everyone', visibility_rules: rules },
    { canOfferGMOnly: true });
  assert.equal(body.visibility, 'everyone');
  assert.equal(body.visibility_rules, rules);
});

test('a RECURRING event keeps repeating after a title-only save', () => {
  // DC2-RECUR-DATALOSS. `is_recurring` is a value-typed bool on the shipped
  // PUT and the service writes it UNGUARDED by design ("false IS the value,
  // not 'absent'"), so an omitted key is not a preserved field — it is a WRITE
  // of false. The pointer siblings around it ARE nil-preserved, so the row
  // would be left is_recurring=false with recurrence_type still populated:
  // the exact half-state C-CAL-RECURRING-PARTIAL-STATE-CLEANUP cleaned up once.
  const body = P.buildEventBody(
    { name: 'Renamed', year: '1523', month: '1', day: '3', allDay: true },
    {
      id: 'ev-9', visibility: 'everyone',
      is_recurring: true, recurrence_type: 'yearly', recurrence_interval: 1,
    },
    { canOfferGMOnly: true });
  assert.equal(body.is_recurring, true,
    'the save un-repeated a recurring event the editor never offered a control for');
  assert.equal(body.recurrence_type, 'yearly');
  assert.equal(body.recurrence_interval, 1);
});

test('a NON-recurring event is sent as non-recurring, explicitly', () => {
  const body = P.buildEventBody(
    { name: 'x', year: '1', month: '1', day: '1', allDay: true },
    { id: 'ev-9', visibility: 'everyone', is_recurring: false }, {});
  assert.equal(body.is_recurring, false);
  assert.ok(!('recurrence_type' in body), 'a nil stored type must not become an empty one');
  assert.ok(!('recurrence_interval' in body));
});

test('CREATE mode sends no recurrence at all — false is then the true value', () => {
  const body = P.buildEventBody({ name: 'x', year: '1', month: '1', day: '1', allDay: true }, null, {});
  assert.ok(!('is_recurring' in body), 'a new event has no stored recurrence to preserve');
  assert.ok(!('recurrence_type' in body));
});

test('the body carries no field the shipped request struct does not bind', () => {
  const body = P.buildEventBody(
    { name: 'x', year: '1', month: '1', day: '1', allDay: true },
    {
      id: 'ev-9', visibility: 'everyone', description_html: '<p>x</p>', entity_id: 'e-1',
      is_recurring: true, recurrence_type: 'weekly', recurrence_interval: 2,
    }, {});
  const allowed = new Set([
    'name', 'description', 'description_html', 'entity_id',
    'year', 'month', 'day', 'all_day', 'category',
    'start_hour', 'start_minute', 'end_hour', 'end_minute',
    'end_year', 'end_month', 'end_day',
    'visibility', 'visibility_rules',
    // Bound by PUT …/events/:eid and by POST …/events alike; carried because
    // an ABSENT is_recurring is a write of false (DC2-RECUR-DATALOSS).
    'is_recurring', 'recurrence_type', 'recurrence_interval',
  ]);
  for (const k of Object.keys(JSON.parse(JSON.stringify(body)))) {
    assert.ok(allowed.has(k), 'unexpected request field ' + k + ' — a route that grows a ' +
      'field it was not named for is a lie in a route name');
  }
});

// --- the wire ---------------------------------------------------------------

test('`+ New event` POSTs to the shipped create route, dated from the card day', async () => {
  const fx = boot();
  openEditorOn(fx, 3);
  assert.equal(fx.editor.popoverOpen, true, 'the editor never opened');
  assert.equal(fx.card.classList.contains('dcopen'), false,
    'the card must close as the editor opens ([DC-7])');
  // The date is PRE-FILLED from the day the card was opened on.
  assert.equal(fx.editor.querySelector('[data-de-year]').value, '1523');
  assert.equal(fx.editor.querySelector('[data-de-month]').value, '1');
  assert.equal(fx.editor.querySelector('[data-de-day]').value, '3');

  setForm(fx, { name: 'Council', year: '1523', month: '1', day: '3' });
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));

  assert.equal(fx.calls.length, 1);
  assert.equal(fx.calls[0].method, 'POST');
  assert.equal(fx.calls[0].url, '/campaigns/camp-1/calendars/cal-1/events');
  assert.equal(fx.calls[0].body.name, 'Council');
  assert.equal(fx.calls[0].body.day, 3);
});

test('the type picker is seeded from the PAGE payload, not from an Owner-only route', () => {
  const fx = boot();
  openEditorOn(fx, 3);
  const opts = fx.editor.querySelector('[data-de-category]').children;
  assert.equal(opts.length, 3, 'the palette did not reach the picker');
  assert.equal(opts[1].getAttribute('value'), 'quest');
  // …and no request was made to fetch it. GET .../event-categories sits behind
  // an OWNER floor, so a Scribe reaching for it would 403.
  assert.equal(fx.calls.filter((c) => /event-categories/.test(c.url)).length, 0);
});

test('a row Edit door GETs the one new read route, then PUTs the update', async () => {
  const stored = {
    id: 'ev-1', calendar_id: 'cal-1', name: 'Council of Wards',
    year: 1523, month: 1, day: 3, all_day: true,
    visibility: 'everyone', visibility_rules: '{"allowed_users":["u-gm"]}',
  };
  const fx = boot({
    responses: { 'GET /campaigns/camp-1/calendars/cal-1/events/ev-1': { ok: true, json: () => Promise.resolve(stored) } },
  });
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-edit]'));
  await new Promise((r) => setImmediate(r));

  assert.equal(fx.calls[0].method, 'GET');
  assert.equal(fx.calls[0].url, '/campaigns/camp-1/calendars/cal-1/events/ev-1');
  assert.equal(fx.editor.popoverOpen, true);
  assert.equal(fx.editor.querySelector('[data-de-name]').value, 'Council of Wards');
  assert.equal(fx.editor.querySelector('[data-de-delete]').hidden, false,
    'Delete appears only in edit mode, and only for a viewer who has the Owner floor');

  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  const put = fx.calls[1];
  assert.equal(put.method, 'PUT');
  assert.equal(put.url, '/campaigns/camp-1/calendars/cal-1/events/ev-1');
  assert.equal(put.body.visibility_rules, '{"allowed_users":["u-gm"]}',
    'the update dropped the audience the editor never showed anybody');
});

// --- EDIT-MODE-ID-FALLBACK-3: the door's id decides PUT-vs-POST -------------

test('an edit session with no id REFUSES; it never falls through to create', () => {
  // Primitive fields only: the mappers run in a vm realm, so their objects
  // carry a foreign prototype and deepEqual would compare identities.
  assert.equal(P.writeTarget('create', '').method, 'POST');
  assert.equal(P.writeTarget('create', '').eventID, '');
  assert.equal(P.writeTarget('edit', 'ev-2').method, 'PUT');
  assert.equal(P.writeTarget('edit', 'ev-2').eventID, 'ev-2');
  // THE FORBIDDEN FALL-THROUGH: `mode === 'edit' && eventID` used to collapse to
  // falsy here and send a POST, creating a DUPLICATE event whose only symptom is
  // a second row nobody connects to the Save they pressed.
  for (const missing of ['', null, undefined, 0]) {
    assert.equal(P.writeTarget('edit', missing), null,
      'an edit with no id must refuse, not create');
  }
  // A create never inherits a stale id either.
  assert.equal(P.writeTarget('create', 'ev-9').eventID, '');
});

test('EDIT mode writes to the id on the DOOR, not to the id the server echoed back', async () => {
  // The server's record used to win the single line that decides PUT-vs-POST,
  // which was correct by luck rather than by design: the write went wherever
  // the response said, not to the row the viewer clicked.
  const fx = boot({
    responses: {
      'GET /campaigns/camp-1/calendars/cal-1/events/ev-2': {
        ok: true,
        json: () => Promise.resolve({
          id: 'ev-5', calendar_id: 'cal-1', name: 'Barrow scouting',
          year: 1523, month: 1, day: 3, all_day: true, visibility: 'everyone',
        }),
      },
    },
  });
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-edit="ev-2"]'));
  await new Promise((r) => setImmediate(r));

  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  const put = fx.calls.find((c) => c.method === 'PUT');
  assert.ok(put, 'an edit must PUT');
  assert.equal(put.url, '/campaigns/camp-1/calendars/cal-1/events/ev-2',
    'the write targets the row that was clicked');
});

test('DELETE follows the same door id as the save, so the two cannot disagree', async () => {
  const fx = boot({
    responses: {
      'GET /campaigns/camp-1/calendars/cal-1/events/ev-2': {
        ok: true,
        json: () => Promise.resolve({
          id: 'ev-5', calendar_id: 'cal-1', name: 'Barrow scouting',
          year: 1523, month: 1, day: 3, all_day: true, visibility: 'everyone',
        }),
      },
    },
  });
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-edit="ev-2"]'));
  await new Promise((r) => setImmediate(r));

  assert.equal(fx.editor.querySelector('[data-de-delete]').hidden, false);
  fx.fire('click', fx.editor.querySelector('[data-de-delete]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.find((c) => c.method === 'DELETE').url,
    '/campaigns/camp-1/calendars/cal-1/events/ev-2');
});

test('an EDIT session with no id REFUSES the save rather than POSTing a duplicate', async () => {
  // A row whose door carries no id never reaches the read route at all — and if
  // an edit session somehow arrives without one, Save must stop. The failure it
  // forbids is silent: a POST creates a SECOND event and the only symptom is an
  // extra row nobody connects to the Save they pressed.
  const fx = boot();
  fx.fire('click', fx.cells[3]);
  const door = fx.card.querySelector('[data-dc-edit]');
  door.setAttribute('data-dc-edit', '');
  fx.fire('click', door);
  await new Promise((r) => setImmediate(r));

  assert.equal(fx.calls.length, 0, 'an id-less door does not even read');
  assert.equal(fx.editor.popoverOpen !== true, true, 'and it opens no editor');
});

test('the edit door round-trips the recurrence the route now hands it', async () => {
  // The whole chain, on the wire: the record carries recurrence (handler.go's
  // eventEditorRecord), the module stores it, and the PUT sends it back. The
  // pure-builder tests above pin the body; this pins that the record's keys
  // actually reach the builder rather than being dropped on the way in.
  const stored = {
    id: 'ev-1', calendar_id: 'cal-1', name: 'Midsummer', year: 1523, month: 1, day: 3,
    all_day: true, visibility: 'everyone',
    is_recurring: true, recurrence_type: 'yearly', recurrence_interval: 1,
  };
  const fx = boot({
    responses: { 'GET /campaigns/camp-1/calendars/cal-1/events/ev-1': { ok: true, json: () => Promise.resolve(stored) } },
  });
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-edit]'));
  await new Promise((r) => setImmediate(r));

  fx.editor.querySelector('[data-de-name]').value = 'Midsummer Renamed';
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));

  const put = fx.calls[1];
  assert.equal(put.method, 'PUT');
  assert.equal(put.body.name, 'Midsummer Renamed');
  assert.equal(put.body.is_recurring, true,
    'renaming a recurring event through the day-card editor un-repeated it');
  assert.equal(put.body.recurrence_type, 'yearly');
  assert.equal(put.body.recurrence_interval, 1);
});

test('Delete uses the shipped DELETE and nothing else', async () => {
  const stored = { id: 'ev-1', calendar_id: 'cal-1', name: 'x', year: 1523, month: 1, day: 3, all_day: true, visibility: 'everyone' };
  const fx = boot({
    responses: { 'GET /campaigns/camp-1/calendars/cal-1/events/ev-1': { ok: true, json: () => Promise.resolve(stored) } },
  });
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-edit]'));
  await new Promise((r) => setImmediate(r));
  fx.fire('click', fx.editor.querySelector('[data-de-delete]'));
  await new Promise((r) => setImmediate(r));

  const del = fx.calls[fx.calls.length - 1];
  assert.equal(del.method, 'DELETE');
  assert.equal(del.url, '/campaigns/camp-1/calendars/cal-1/events/ev-1');
});

test('a save that fails says so IN the editor rather than closing on a lie', async () => {
  const fx = boot({ responses: { POST: { ok: false, json: () => Promise.resolve({}) } } });
  openEditorOn(fx, 3);
  setForm(fx, { name: 'Council', year: '1523', month: '1', day: '3' });
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.editor.querySelector('[data-de-err]').hidden, false);
  assert.equal(fx.editor.popoverOpen, true, 'a failed save must not close the editor');
  assert.equal(fx.reloads(), 0);
});

test('a titleless save never reaches the wire', async () => {
  const fx = boot();
  openEditorOn(fx, 3);
  setForm(fx, { name: '   ', year: '1523', month: '1', day: '3' });
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.length, 0);
  assert.equal(fx.editor.querySelector('[data-de-err]').hidden, false);
});

// --- the role gates, obeyed rather than computed -----------------------------

test('with NO editor DOM the module offers no door and writes nothing', async () => {
  // This is a PLAYER's page: bench.templ rendered no editor scaffold and no
  // data-dc-can-edit, so there is nothing for the module to open.
  const fx = boot({ canEdit: false });
  fx.fire('click', fx.cells[3]);
  assert.equal(fx.editor, null);
  assert.equal(fx.card.querySelector('[data-dc-edit]'), null,
    'a player’s rows are read-only text, not disabled buttons');
  assert.equal(fx.card.querySelector('[data-dc-new]'), null);
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.length, 0);
});

test('without the dm_only control the module never sends dm_only', async () => {
  const fx = boot({ canGMOnly: false });
  openEditorOn(fx, 3);
  assert.equal(fx.editor.querySelector('[data-de-gmonly]'), null,
    'the capability gate is markup-level and this fixture is without it');
  setForm(fx, { name: 'Council', year: '1523', month: '1', day: '3' });
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls[0].body.visibility, 'everyone');
});

test('without the Owner floor there is no Delete element to reveal', async () => {
  const stored = { id: 'ev-1', calendar_id: 'cal-1', name: 'x', year: 1523, month: 1, day: 3, all_day: true, visibility: 'everyone' };
  const fx = boot({
    canDelete: false,
    responses: { 'GET /campaigns/camp-1/calendars/cal-1/events/ev-1': { ok: true, json: () => Promise.resolve(stored) } },
  });
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-edit]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.editor.querySelector('[data-de-delete]'), null);
});

test('the editor closes on Escape under the register, and the card does not reopen', () => {
  const fx = boot();
  openEditorOn(fx, 3);
  // Drain the CARD's own close first — it left when the editor arrived, and its
  // 160ms timer is the same register rule. What is measured below is the
  // editor's.
  fx.flush();
  fx.fire('keydown', fx.editor, { key: 'Escape' });
  assert.equal(fx.editor.classList.contains('dcopen'), false);
  assert.equal(fx.timers.length, 1, 'exactly one close timer, at --disc-close');
  assert.equal(fx.timers[0].ms, 160);
  fx.flush();
  assert.equal(fx.editor.popoverOpen, false);
  assert.equal(fx.card.popoverOpen, false, 'the card left when the editor arrived and stays gone');
});

test('opening the editor leaves the Block byte-identical too', () => {
  const fx = boot();
  const before = fx.blockHost.outerHTML;
  openEditorOn(fx, 3);
  fx.fire('keydown', fx.editor, { key: 'Escape' });
  fx.flush();
  assert.equal(fx.blockHost.outerHTML, before);
});

// --- DC-SAVE-6: one Save, one write ------------------------------------------
//
// edSave is reachable from TWO independent listeners — the delegated document
// click on [data-de-save] and the form's own submit. A real user click fires
// only one, because the click branch's preventDefault cancels the button's
// activation behaviour. That is a true fact about the browser and a terrible
// thing to rest a write on: an early return, a move to the capture phase, or a
// reorder of the branch chain would turn every Save into two events, and
// nothing would fail. A double-click before the reload lands does it today.
// The guard is therefore at the WRITE, not at the listener.

const filled = (fx) => setForm(fx, { name: 'Council', year: '1523', month: '1', day: '3' });

test('both listeners firing on one Save still produce exactly one write', async () => {
  const fx = boot();
  openEditorOn(fx, 3);
  filled(fx);

  // The synthetic case the browser does NOT protect: click and submit both
  // reaching edSave for a single user gesture. The submit listener is bound to
  // the form ELEMENT, not delegated, so it is dispatched on the node.
  const form = fx.editor.querySelector('[data-de-form]');
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  form.dispatch('submit', { target: form, preventDefault() {} });
  await new Promise((r) => setImmediate(r));

  const writes = fx.calls.filter((c) => c.method === 'POST');
  assert.equal(writes.length, 1, 'two listeners must not produce two events');
});

test('a double-click on Save before the reload lands writes once', async () => {
  const fx = boot();
  openEditorOn(fx, 3);
  filled(fx);
  const save = fx.editor.querySelector('[data-de-save]');
  fx.fire('click', save);
  fx.fire('click', save);
  fx.fire('click', save);
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.filter((c) => c.method === 'POST').length, 1);
});

test('a repeated Delete before the reload lands deletes once', async () => {
  const stored = {
    id: 'ev-1', name: 'Council of Wards', year: 1523, month: 1, day: 3,
    visibility: 'everyone', visibility_rules: null, all_day: true,
  };
  const fx = boot({
    responses: { 'GET /campaigns/camp-1/calendars/cal-1/events/ev-1': { ok: true, json: () => Promise.resolve(stored) } },
  });
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-edit]'));
  await new Promise((r) => setImmediate(r));
  const del = fx.editor.querySelector('[data-de-delete]');
  fx.fire('click', del);
  fx.fire('click', del);
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.filter((c) => c.method === 'DELETE').length, 1);
});

test('a REFUSED save can be retried — the guard must not lock the editor', async () => {
  const fx = boot({ responses: { POST: { ok: false, json: () => Promise.resolve({}) } } });
  openEditorOn(fx, 3);
  filled(fx);
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.filter((c) => c.method === 'POST').length, 1);

  // The server said no and the editor is still open. A second attempt must
  // reach the wire, or a network blip would leave the editor unable to save.
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.filter((c) => c.method === 'POST').length, 2);
});

test('a REJECTED title does not consume the write, so the fixed title saves', async () => {
  const fx = boot();
  openEditorOn(fx, 3);
  setForm(fx, { name: '  ', year: '1523', month: '1', day: '3' });
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.length, 0, 'a titleless save never reaches the wire');

  filled(fx);
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.filter((c) => c.method === 'POST').length, 1,
    'the in-flight flag is set AFTER validation, so a rejected title cannot lock the editor');
});

test('reopening the editor clears an in-flight flag left by a failed save', async () => {
  const fx = boot({ responses: { POST: { ok: false, json: () => Promise.resolve({}) } } });
  openEditorOn(fx, 3);
  filled(fx);
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  fx.fire('click', fx.editor.querySelector('[data-de-cancel]'));

  openEditorOn(fx, 5);
  filled(fx);
  fx.fire('click', fx.editor.querySelector('[data-de-save]'));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.filter((c) => c.method === 'POST').length, 2,
    'a fresh editor session is a fresh write');
});
