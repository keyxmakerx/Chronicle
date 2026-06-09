// calendar_permissions.test.mjs — C-CAL-DASHBOARD-W5b. Pins the pure mappers in
// calendar_permissions.js: editor mode + chip rules ↔ the stored visibility
// payload. The DOM driver is guarded behind `typeof document`, so booting with
// only `window` exposes window.__calPerm without touching the DOM.
//
// NOTE: the mappers run in a vm realm, so their returned objects have a foreign
// prototype — assert on primitive fields / JSON.parse (in this realm), not
// deepEqual on the cross-realm objects directly.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'calendar_permissions.js');

function load() {
  const sandbox = {};
  sandbox.window = sandbox; // window defined, document undefined → DOM driver skipped
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'calendar_permissions.js' });
  return sandbox.window.__calPerm;
}

const P = load();

test('gmonly maps to dm_only with no rules', () => {
  const out = P.buildVisibilityPayload('gmonly', [{ mode: 'allow', kind: 'user', target: 'u1' }]);
  assert.equal(out.visibility, 'dm_only');
  assert.equal(out.visibility_rules, null);
});

test('public maps to everyone with no rules', () => {
  const out = P.buildVisibilityPayload('public', []);
  assert.equal(out.visibility, 'everyone');
  assert.equal(out.visibility_rules, null);
});

test('specific maps allow/deny user chips to allowed/denied_users', () => {
  const out = P.buildVisibilityPayload('specific', [
    { mode: 'allow', kind: 'user', target: 'u1' },
    { mode: 'allow', kind: 'user', target: 'u2' },
    { mode: 'deny', kind: 'user', target: 'u3' },
  ]);
  assert.equal(out.visibility, 'everyone');
  assert.deepEqual(JSON.parse(out.visibility_rules), { allowed_users: ['u1', 'u2'], denied_users: ['u3'] });
});

test('specific with no chips writes null rules (bulk-replace clears)', () => {
  const out = P.buildVisibilityPayload('specific', []);
  assert.equal(out.visibility, 'everyone');
  assert.equal(out.visibility_rules, null);
});

test('non-user (role) chips do not persist to the user-scoped model', () => {
  const out = P.buildVisibilityPayload('specific', [{ mode: 'allow', kind: 'role', target: 'player' }]);
  assert.equal(out.visibility_rules, null);
});

test('rulesToChips parses stored allow/deny into chips', () => {
  const chips = P.rulesToChips('{"allowed_users":["u1"],"denied_users":["u2"]}');
  assert.deepEqual(JSON.parse(JSON.stringify(chips)), [
    { mode: 'allow', kind: 'user', target: 'u1', label: 'u1' },
    { mode: 'deny', kind: 'user', target: 'u2', label: 'u2' },
  ]);
  assert.equal(P.rulesToChips('').length, 0);
  assert.equal(P.rulesToChips('{bad json').length, 0);
});

test('round-trips: stored rules → chips → payload reproduces the rules', () => {
  const stored = '{"allowed_users":["u1","u2"],"denied_users":["u9"]}';
  const out = P.buildVisibilityPayload('specific', P.rulesToChips(stored));
  assert.deepEqual(JSON.parse(out.visibility_rules), JSON.parse(stored));
});
