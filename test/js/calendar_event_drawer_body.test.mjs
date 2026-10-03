// calendar_event_drawer_body.test.mjs — pins which parts of a rule the
// drawer sends on save: an update is partial, so a rule that is unchanged,
// hidden from this viewer, or never existed must be left out, and only a
// real edit or a removal may name recurrence_rule.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const read = (f) => readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', f), 'utf8');

globalThis.window = globalThis;
globalThis.document = { createElement: () => ({ style: {}, setAttribute() {}, appendChild() {}, addEventListener() {} }) };
globalThis.Chronicle = {};
globalThis.module = { exports: {} };
vm.runInThisContext(read('calendar_rule.js'));
delete globalThis.module;
Chronicle.calendarDate = { dayIndex: (c, y, m, d) => y * 1000 + m * 40 + d };
vm.runInThisContext(read('calendar_event_drawer.js'));
const Drawer = Chronicle.calendarEventDrawer.Drawer;
const Rl = Chronicle.calendarRule;

const cal = { current_year: 1, hours_per_day: 24, minutes_per_hour: 60, moons: [{ id: 1, name: 'Luna' }], seasons: [] };
const fullMoon = { match: [{ kind: 'moon_phase', moon_id: 9, phase: 'full' }] }; // moon 9 was deleted
const other = { match: [{ kind: 'weekday', weekday: 2 }] };

// A drawer with just enough surface for _body: fields by selector.
function make(state, extra) {
  const fields = {
    '#cal5-edT': { value: 'Feast' }, '#cal5-edD': { value: '' },
    '#cal5-esY': { value: '1' }, '#cal5-esM': { value: '1' }, '#cal5-esD': { value: '1' },
    '#cal5-eeY': { value: '1' }, '#cal5-eeM': { value: '1' }, '#cal5-eeD': { value: '1' },
    '#cal5-edErr': { textContent: '', hidden: true },
  };
  const d = Object.create(Drawer.prototype);
  d.view = { canAuthorDmOnly: false };
  d.cal = cal;
  d.CalDate = Chronicle.calendarDate;
  d.el = { querySelector: (s) => fields[s] || null };
  d._env = () => ({ cal });
  d.state = Object.assign({ isNew: false, allDay: true, kindId: null, ev: {}, vis: 'everyone', rep: { type: '', end: 'never' }, rule: {} }, state);
  Object.assign(d, extra);
  return d;
}

test('a new plain event sends no recurrence_rule', () => {
  const b = make({ isNew: true })._body().body;
  assert.equal('recurrence_rule' in b, false);
  assert.equal(b.is_recurring, false);
});

test('a rule turned into a plain event clears it with null', () => {
  const d = make({ rep: { type: '', end: 'never', hadRule: true } });
  assert.equal(d._body().body.recurrence_rule, null);
});

test('a rule hidden from this viewer is left out entirely', () => {
  const d = make({ rep: { type: 'rule', locked: true, end: 'never' } });
  const b = d._body().body;
  for (const k of ['recurrence_rule', 'is_recurring', 'recurrence_type']) assert.equal(k in b, false, k);
});

test('an unchanged rule is left out, even one naming a deleted moon', () => {
  const form = Rl.fromRule(fullMoon, cal);
  const d = make({ rep: { type: 'rule', end: 'never', hadRule: true }, rule: { form, original: Rl.clone(form) } });
  const b = d._body().body;
  assert.ok(b, 'save is not blocked');
  assert.equal('recurrence_rule' in b, false);
  assert.equal(b.recurrence_type, 'rule');
});

test('a changed rule is sent as a value', () => {
  const orig = Rl.fromRule(other, cal);
  const form = Rl.clone(orig);
  form.every = 2;
  const d = make({ rep: { type: 'rule', end: 'never', hadRule: true }, rule: { form, original: orig } });
  assert.deepEqual(d._body().body.recurrence_rule, Rl.toRule(form));
});
