// calendar_set_today.test.mjs — pins the Set today save: the PUT carries the
// calendar's name (the settings save rejects a call without one), and a
// refusal shows the server's sentence rather than its error code.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const widgets = path.join(here, '..', '..', 'static', 'js', 'widgets');
const read = (f) => readFileSync(path.join(widgets, f), 'utf8');

function load(reply) {
  const calls = [];
  const sandbox = {
    module: { exports: {} }, console,
    Chronicle: {
      register(n, def) { if (n === 'calendar_view') sandbox.def = def; },
      apiFetch(url, opts) { calls.push({ url, opts }); return Promise.resolve(reply); },
    },
    document: { activeElement: null, querySelector: () => null, addEventListener() {} },
    window: {},
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(read('chronicle_gen.js'), sandbox);
  vm.runInContext(read('calendar_view.js'), sandbox);
  return { def: sandbox.def, calls };
}

const when = { y: 1491, m: 3, d: 12, h: 14, mi: 30 };

test('Set today sends the calendar name with the new date and time', async () => {
  const { def, calls } = load({ ok: true, json: () => Promise.resolve({}) });
  const self = Object.create(def);
  self.cal = { name: 'Harptos', current_year: 1490, current_month: 1, current_day: 1 };
  self.apiBase = '/campaigns/c1/calendars/k1';
  self.renderToday = () => {}; self.goToday = () => {}; self.say = () => {};
  self._dateLabel = () => ''; self._clockWords = () => '';
  assert.equal(await self._saveToday(when), '');
  assert.equal(calls.length, 1);
  assert.equal(calls[0].opts.method, 'PUT');
  assert.deepEqual({ ...calls[0].opts.body }, {
    name: 'Harptos', current_year: 1491, current_month: 3, current_day: 12, current_hour: 14, current_minute: 30,
  });
});

test('a refused save shows the message, not the error code', async () => {
  const { def } = load({ ok: false, json: () => Promise.resolve({ error: 'validation_error', message: 'calendar name is required' }) });
  const self = Object.create(def);
  self.cal = { name: 'Harptos' };
  self.apiBase = '/campaigns/c1/calendars/k1';
  assert.equal(await self._saveToday(when), 'calendar name is required');
});

test('a refusal without a message falls back to plain words', async () => {
  const { def } = load({ ok: false, json: () => Promise.resolve({ error: 'validation_error' }) });
  const self = Object.create(def);
  self.cal = { name: 'Harptos' };
  self.apiBase = '/campaigns/c1/calendars/k1';
  assert.equal(await self._saveToday(when), 'Could not set today.');
});
