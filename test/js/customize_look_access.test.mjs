// customize_look_access.test.mjs — the "Who can change" section in
// customize_look.js offers the access levels the server accepts (the DM
// Screen never offers Everyone), is covered by Save/Discard/Reset (SEC_KEYS),
// and travels in the Save payload.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const go = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'look_access.go'), 'utf8');

function grab(name) {
  const a = src.indexOf('var ' + name + ' = [');
  assert.ok(a > 0, name + ' marker not found');
  const b = src.indexOf('];', a);
  return JSON.parse(vm.runInNewContext(src.slice(a, b + 2) + ';JSON.stringify(' + name + ')'));
}

test('access levels use the ids and wording the server and the brief name', () => {
  const levels = grab('ACCESS_LEVELS');
  assert.deepEqual(levels, [['owner', 'Only me'], ['codm', 'Me and co-DMs'], ['everyone', 'Everyone']]);
  for (const [id] of levels) assert.match(go, new RegExp('"' + id + '"'), id + ' must exist on the server');
});

test('the DM Screen row never offers Everyone, and both parts match the server', () => {
  const parts = grab('ACCESS_PARTS');
  const byKey = Object.fromEntries(parts.map((p) => [p[0], p[2]]));
  assert.deepEqual(byKey.sheet, ['owner', 'codm', 'everyone']);
  assert.deepEqual(byKey.dmscreen, ['owner', 'codm']);
  assert.match(go, /LookPartSheet\s*=\s*"sheet"/);
  assert.match(go, /LookPartDmScreen\s*=\s*"dmscreen"/);
});

test('access is part of Save, Discard and Reset and is sent with the look', () => {
  assert.match(src, /access:\['access\.sheet', 'access\.dmscreen'\]/, 'SEC_KEYS must list the access keys');
  assert.match(src, /page:clone\(d\.page\), access:clone\(d\.access\)/, 'toInput must send access');
  assert.match(src, /if \(!d\.access\) d\.access = \{ sheet:'everyone', dmscreen:'codm' \}/, 'fromServer must default to the server defaults');
  assert.match(src, /'access\.' \+ p\[0\]/, 'each row must bind its access key');
  assert.match(src, /People change these for themselves only\. Nobody else sees their choice\./);
});
