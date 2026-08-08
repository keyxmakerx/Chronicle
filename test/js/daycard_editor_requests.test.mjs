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

// AMENDED BY NAME, C-CALV4-EDITOR-R2b stage 2, under [ER-10] SIGNED.
//
// WHO TURNED IT OVER AND WHAT THE OLD CLAIM WAS. Stage 2 shipped the type
// picker as `<select data-de-category>` and this test counted its <option>
// children. The chrome replaces it with the LOCKED TRIPLE — one `.topt` per
// category carrying hue rail + stroke pattern + glyph — because a <select>
// can carry exactly one channel and the build law wants three.
//
// WHY THE NEW CLAIM IS NOT WEAKER. It asserts the same seeding fact (the
// palette came off the PAGE PAYLOAD and no request was made for it) and adds
// the three claims the old shape could not make at all: that the rail carries
// the category's own hue, that the pattern class is present, and that the
// glyph is a sibling of the rail rather than the rail's ink. Condition 1 is
// met literally — `data-de-category` did not move off the node the module
// reads, it merely became a hidden input — and condition 2 holds because the
// body's `category` value is produced by the same `.value` read as before.
//
// MUTATION-TESTED: dropping `cat.axis` from edTypeRail turns this red.
test('the type picker is seeded from the PAGE payload, not from an Owner-only route', () => {
  const fx = boot();
  openEditorOn(fx, 3);
  const opts = fx.editor.querySelector('[data-de-typerail]').children;
  assert.equal(opts.length, 3, 'the palette did not reach the picker');
  // The first option is `No type`, which every event may be.
  assert.equal(opts[0].getAttribute('data-type-pick'), '');
  assert.equal(opts[1].getAttribute('data-type-pick'), 'quest');

  // THE LOCKED TRIPLE, per option: hue on the rail, pattern as a class beside
  // it, glyph as its OWN element. A hue never travels alone on this surface.
  const quest = opts[1];
  assert.equal(quest.style.getPropertyValue('--axis'), '#ef4444');
  assert.ok(/\brail\b/.test(quest.querySelector('.rail').className));
  assert.equal(quest.querySelector('.g').textContent, '▲');
  assert.equal(quest.querySelector('.nm').textContent, 'Quest');

  // THE PATTERN CHANNEL MUST CARRY INFORMATION, NOT MERELY EXIST — stage 8,
  // fix-forward. `.rail` used to be asserted for the presence of a class, and
  // it passed while every option in the rail drew `p1`: the producer emitted no
  // pattern at all and the module's fallback supplied the same solid stroke for
  // every type. A channel that says the same thing about everything is not a
  // channel, and hue was doing the whole job — which is exactly what the build
  // law forbids. The two categories carry DIFFERENT patterns and the assertion
  // is that the rail keeps them apart.
  const social = opts[2];
  const pat = (o) => (o.querySelector('.rail').className.match(/\bp[1-8]\b/) || [])[0];
  assert.equal(pat(quest), 'p4', 'the type rail dropped the category\'s own pattern');
  assert.equal(pat(social), 'p7');
  assert.notEqual(pat(quest), pat(social),
    'two types share a stroke pattern, so hue is carrying the identity alone');

  // …and the handle the write path reads is still the one it always read.
  assert.equal(fx.editor.querySelector('[data-de-category]').tagName, 'INPUT');

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

// ── C-CALV4-EDITOR-R2b stage 2: the chrome's own contracts ────────────────

test('the recurrence unit list is corrected in all three directions', () => {
  const P = boot().pure;

  // A TEN-DAY WEEK IS NOT INVENTION AND ITS CHIP COMES OFF. Week-based
  // recurrence strides `WeekLength() × recurrenceWeeks(...)`, so `weekly` on a
  // ten-day calendar MEANS every tenday. DAYCARD §5 and the mockup both chip
  // this unit; both are wrong and the correction is this slice's.
  // `plain` round-trips through JSON because these values are built inside the
  // module's own vm realm: strict deepEqual compares prototypes, and an array
  // from another realm is never reference-equal to one from this file's.
  const plain = (v) => JSON.parse(JSON.stringify(v));
  const ten = P.recurrenceUnits(10);
  assert.deepEqual(plain(ten.map((u) => u.id)), ['week', 'month', 'year', 'day', 'moon']);
  assert.equal(ten[0].backed, true, 'the week unit is an accepted recurrence_type');
  assert.equal(ten[1].backed, true, 'monthly is an accepted recurrence_type');
  assert.equal(ten[2].backed, true, 'yearly is an accepted recurrence_type (C-CALV4-GAMEREADY §6)');
  assert.equal(ten[3].backed, false, 'there is no daily recurrence_type');
  assert.equal(ten[4].backed, false, 'there is no moon-phase recurrence_type');

  // AMENDED BY C-CALV4-GAMEREADY §6 [GR-11] — AND THE AMENDMENT IS AN
  // INVERSION, NOT A RELAXATION.
  //
  // This assertion used to read `…filter(/year/).length === 0` with the note
  // "`year` IS INVENTION AND THE DRAWING OFFERS IT UNCHIPPED. It does not ship
  // at all: an unbacked unit in a picker is a trap." That was CORRECT for as
  // long as it was true — OccursOn had four types and dropped `yearly` on the
  // floor, so offering the unit would have been the exact silent degradation
  // the chip exists to prevent.
  //
  // §6 built the type, so the ONLY reason the unit was withheld is gone, and
  // the guard now asserts the same property from the other side: the unit
  // EXISTS and is BACKED. It is not weaker — a picker offering an unbacked
  // `year` still fails here, which is what the original was protecting.
  const year = ten.filter((u) => u.id === 'year');
  assert.equal(year.length, 1, 'the year unit ships now that RecurrenceYearly expands');
  assert.equal(year[0].backed, true, 'a year unit that is not backed is the trap the chip prevents');

  // THE LABEL IS DERIVED, NEVER A LITERAL, and it is re-driven at the four
  // week lengths the stills re-drive `.wdpick` at. Chronicle's Calendar carries
  // Weekdays and a WeekLength() and NO WEEK NOUN, so the honest derived label
  // names the cycle's length — which cannot lie about the stride the way a bare
  // "week" does on a ten-day calendar, and cannot hardcode "tenday" either.
  for (const n of [5, 7, 10, 13]) {
    assert.equal(P.recurrenceUnits(n)[0].label, n + '-day week');
  }
  // A CALENDAR WITH NO WEEKDAYS HAS NO WEEK UNIT. WeekLength() 0 makes the
  // server's own stride 0 and OccursOn falls back to a single occurrence, so
  // offering the unit would be offering exactly the silent degradation the chip
  // exists to prevent.
  assert.deepEqual(plain(P.recurrenceUnits(0).map((u) => u.id)), ['month', 'year', 'day', 'moon']);
});

// C-CALV4-GAMEREADY §6, [GR-11] — the yearly unit, end to end through the two
// pure mappers the editor and the server meet in.
//
// THE INVERSE IS HALF THE GUARD. A unit that authors `yearly` but does not read
// `yearly` back opens the editor on "once" for an event that repeats, and the
// next title-only save silently un-repeats a festival — the exact half-state
// C-CAL-RECURRING-PARTIAL-STATE-CLEANUP had to unwind once already.
test('the yearly unit authors yearly and reads yearly back', () => {
  const P = boot().pure;
  const plain = (v) => JSON.parse(JSON.stringify(v));

  assert.deepEqual(plain(P.recurrenceBody({ mode: 'repeats', unit: 'year', every: 1 })),
    { is_recurring: true, recurrence_type: 'yearly', recurrence_interval: 0 });

  // THE INTERVAL IS NOT AUTHORED FROM HERE, and `every` is ignored rather than
  // forwarded. The server honours a stored yearly interval, but this editor
  // offers no `every [N]` field for the unit, so sending a number the author
  // never typed would author a rule they did not choose.
  assert.equal(P.recurrenceInterval('year'), false);
  assert.deepEqual(plain(P.recurrenceBody({ mode: 'repeats', unit: 'year', every: 4 })),
    { is_recurring: true, recurrence_type: 'yearly', recurrence_interval: 0 });

  // The inverse: a stored yearly event opens ON the year unit.
  const back = plain(P.recurrenceFromRecord({ is_recurring: true, recurrence_type: 'yearly' }, 10));
  assert.equal(back.mode, 'repeats');
  assert.equal(back.unit, 'year');

  // And a full round trip is lossless, which is what keeps a title-only save
  // from rewriting the rule.
  assert.deepEqual(
    plain(P.recurrenceBody(P.recurrenceFromRecord({ is_recurring: true, recurrence_type: 'yearly' }, 10))),
    { is_recurring: true, recurrence_type: 'yearly', recurrence_interval: 0 });
});

test('the interval maps onto the type at the producer of the request body', () => {
  const P = boot().pure;
  const plain = (v) => JSON.parse(JSON.stringify(v));
  const rec = (unit, every, mode) => plain(P.recurrenceBody({ mode: mode || 'repeats', unit, every }));

  assert.deepEqual(rec('week', 1), { is_recurring: true, recurrence_type: 'weekly', recurrence_interval: 0 });
  assert.deepEqual(rec('week', 2), { is_recurring: true, recurrence_type: 'biweekly', recurrence_interval: 0 });
  assert.deepEqual(rec('week', 3), { is_recurring: true, recurrence_type: 'custom', recurrence_interval: 3 });
  assert.deepEqual(rec('week', 9), { is_recurring: true, recurrence_type: 'custom', recurrence_interval: 9 });

  // MONTHLY HAS NO INTERVAL, and the field is ABSENT rather than chipped:
  // OccursOn's monthly branch ignores RecurrenceInterval entirely, so `every 2
  // months` would be stored, accepted and then expanded EVERY month.
  assert.equal(P.recurrenceInterval('week'), true);
  assert.equal(P.recurrenceInterval('month'), false);
  assert.deepEqual(rec('month', 4), { is_recurring: true, recurrence_type: 'monthly', recurrence_interval: 0 });

  // AN UNBACKED UNIT AUTHORS NOTHING. Null means "do not write", so the caller
  // round-trips the stored rule instead of sending a type the server would
  // silently degrade to one occurrence — which is exactly what the chip beside
  // the unit promises.
  assert.equal(rec('day', 1), null);
  assert.equal(rec('moon', 1), null);

  // ONCE CLEARS THE TYPE AND THE INTERVAL TOGETHER. A JSON null cannot clear
  // either column — service.UpdateEvent guards the pointer siblings — so the
  // empty string is the only clear the shipped PUT admits, and it lands the
  // pair consistent instead of in the half-state
  // C-CAL-RECURRING-PARTIAL-STATE-CLEANUP already had to clean up once.
  assert.deepEqual(rec('week', 3, 'once'),
    { is_recurring: false, recurrence_type: '', recurrence_interval: 0 });
});

test('the three losslessness cases, on the wire', () => {
  const P = boot().pure;
  const stored = {
    id: 'ev-1', visibility: 'everyone',
    is_recurring: true, recurrence_type: 'weekly', recurrence_interval: 1,
  };
  const base = { name: 'x', year: '1', month: '1', day: '1', allDay: true };

  // UNTOUCHED → ROUND-TRIPPED. `form.recurrence` is null when the author never
  // opened the recurrence controls, which is what keeps a title-only save from
  // rewriting a rule it never showed anybody.
  const untouched = P.buildEventBody({ ...base, recurrence: null }, stored, {});
  assert.equal(untouched.is_recurring, true);
  assert.equal(untouched.recurrence_type, 'weekly');
  assert.equal(untouched.recurrence_interval, 1);

  // AUTHORED → SENT.
  const authored = P.buildEventBody(
    { ...base, recurrence: { mode: 'repeats', unit: 'week', every: 2 } }, stored, {});
  assert.equal(authored.recurrence_type, 'biweekly');

  // EXPLICITLY ONCE → is_recurring:false AND the pair cleared together.
  const once = P.buildEventBody(
    { ...base, recurrence: { mode: 'once', unit: 'week', every: 2 } }, stored, {});
  assert.equal(once.is_recurring, false);
  assert.equal(once.recurrence_type, '');
  assert.equal(once.recurrence_interval, 0);

  // A CHIPPED UNIT ROUND-TRIPS RATHER THAN DEGRADING.
  const chipped = P.buildEventBody(
    { ...base, recurrence: { mode: 'repeats', unit: 'day', every: 1 } }, stored, {});
  assert.equal(chipped.recurrence_type, 'weekly', 'a chipped unit wrote a degrading type');
});

test('the editor renders a chip on every unbacked unit and on no backed one', () => {
  const fx = boot();
  openEditorOn(fx, 3);
  const units = fx.editor.querySelectorAll('[data-unit-pick]');
  assert.ok(units.length >= 3, 'the unit list did not build');
  // AMENDED BY C-CALV4-GAMEREADY §6 [GR-11], AND STRENGTHENED WHILE AMENDING.
  // The backed set was written out here as `id === 'week' || id === 'month'` —
  // a THIRD hand-typed copy of a fact that already lived in recurrenceUnits and
  // in model.go's constant block, and it went stale the moment `yearly` shipped.
  // It is now READ from the producer, so adding a unit can never again leave
  // this guard asserting the old world. The property is unchanged: a chip on a
  // backed unit is the mockup's defect, a chip missing from an unbacked one is
  // a silent single occurrence.
  const backedIds = new Set(boot().pure.recurrenceUnits(10).filter((u) => u.backed).map((u) => u.id));
  assert.ok(backedIds.has('year'), 'the yearly unit must be backed since §6 built RecurrenceYearly');
  for (const u of units) {
    const id = u.getAttribute('data-unit-pick');
    const chip = u.querySelector('.badge.need');
    const backed = backedIds.has(id);
    assert.equal(!!chip, !backed,
      'unit ' + id + ': a chip on a backed unit is the mockup’s defect; a chip ' +
      'missing from an unbacked one is a silent single occurrence');
    if (chip) assert.equal(chip.textContent, 'needs backend');
  }
});

test('an end date can never precede its own start, and the intercalary day sorts last', () => {
  const P = boot().pure;
  // The signed shape in miniature: three ordinary days then a festival day,
  // exactly the order the producer emits (ordinary ascending, then intercalary).
  const list = [
    { key: 'h-1', ord: '1', day: 1, weekday: 'A' },
    { key: 'h-2', ord: '2', day: 2, weekday: 'B' },
    { key: 'h-3', ord: '3', day: 3, weekday: 'A' },
    { key: 'h-i1', ord: 'i1', day: 1, label: 'Midwinter' },
  ];
  const plain = (v) => JSON.parse(JSON.stringify(v));
  const dates = P.orderedDates(list);
  assert.deepEqual(plain(dates.map((d) => d.key)), ['h-1', 'h-2', 'h-3', 'h-i1']);
  assert.equal(P.isIntercalary(dates[3]), true);

  // Advancing from every start, over the whole list, never lands before it.
  for (let i = 0; i < dates.length; i++) {
    let cur = dates[i].key;
    for (let step = 0; step < 6; step++) {
      const next = P.nextDate(dates, cur);
      if (!next) break;
      assert.ok(dates.indexOf(next) > i,
        'an end date landed before its own start — the defect the drawing lane ' +
        'found by taking `day === "ic" ? days : day` as the ordering base');
      cur = next.key;
    }
  }
  // FROM THE INTERCALARY DAY THERE ARE NO END OPTIONS AT ALL. It is the last
  // date in the month, so the field says "ends the same day" rather than
  // clamping backwards to a numbered day.
  assert.equal(P.nextDate(dates, 'h-i1'), null);
});

test('the week is DERIVED from the calendar’s own weekday names, at any length', () => {
  const P = boot().pure;
  for (const len of [5, 7, 10, 13]) {
    const names = [];
    for (let i = 0; i < len; i++) names.push('WD' + i);
    const list = [];
    for (let d = 1; d <= 30; d++) {
      list.push({ key: 'h-' + d, ord: String(d), day: d, weekday: names[(d - 1) % len] });
    }
    const shape = P.weekShape(list);
    assert.equal(shape.len, len, 'the derived week length is wrong at ' + len);
    assert.deepEqual(JSON.parse(JSON.stringify(shape.names)), names);
  }
  // A calendar that declares no weekdays has no week, and says so rather than
  // guessing one.
  assert.deepEqual(JSON.parse(JSON.stringify(P.weekShape([{ key: 'h-1', ord: '1', day: 1 }]))),
    { len: 0, names: [] });
});

test('the audience reads allow-by-default ONLY when there is no rule at all', () => {
  const P = boot().pure;
  const plain = (v) => JSON.parse(JSON.stringify(v));
  const ids = ['u-a', 'u-b', 'u-c'];

  // No rule: everyone is allowed, because the event is not restricted.
  assert.deepEqual(plain(P.audienceFromRules([], ids)), { 'u-a': true, 'u-b': true, 'u-c': true });

  // AN ALLOW LIST IS A CLOSED DOOR WITH A GUEST LIST. A member absent from it
  // is DENIED, and reading them as allowed would silently widen the audience on
  // the first save — a permission bug wearing a UI bug's clothes.
  assert.deepEqual(
    plain(P.audienceFromRules([{ mode: 'allow', kind: 'user', target: 'u-a' }], ids)),
    { 'u-a': true, 'u-b': false, 'u-c': false });

  // A deny list is an open door with a bouncer.
  assert.deepEqual(
    plain(P.audienceFromRules([{ mode: 'deny', kind: 'user', target: 'u-b' }], ids)),
    { 'u-a': true, 'u-b': false, 'u-c': true });

  // The return leg emits an ALLOW list, in the roster's own order.
  assert.deepEqual(
    plain(P.audienceToChips({ 'u-a': true, 'u-b': false, 'u-c': true }, ids)),
    [{ mode: 'allow', kind: 'user', target: 'u-a', label: 'u-a' },
     { mode: 'allow', kind: 'user', target: 'u-c', label: 'u-c' }]);
});
