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

test('the body carries no field the shipped request struct does not bind', () => {
  const body = P.buildEventBody(
    { name: 'x', year: '1', month: '1', day: '1', allDay: true },
    { id: 'ev-9', visibility: 'everyone', description_html: '<p>x</p>', entity_id: 'e-1' }, {});
  const allowed = new Set([
    'name', 'description', 'description_html', 'entity_id',
    'year', 'month', 'day', 'all_day', 'category',
    'start_hour', 'start_minute', 'end_hour', 'end_minute',
    'end_year', 'end_month', 'end_day',
    'visibility', 'visibility_rules',
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
