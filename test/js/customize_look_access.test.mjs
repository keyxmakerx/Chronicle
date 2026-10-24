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

test('every shareable part matches the server, and the DM Screen never offers Everyone', () => {
  const parts = grab('ACCESS_PARTS');
  const byKey = Object.fromEntries(parts.map((p) => [p[0], p[2]]));
  for (const k of ['sheet', 'page', 'hover']) assert.deepEqual(byKey[k], ['owner', 'codm', 'everyone'], k);
  assert.deepEqual(byKey.dmscreen, ['owner', 'codm']);
  for (const [k, c] of [['Sheet', 'sheet'], ['Page', 'page'], ['Hover', 'hover'], ['DmScreen', 'dmscreen']]) {
    assert.match(go, new RegExp('LookPart' + k + '\\s*=\\s*"' + c + '"'), c + ' must exist on the server');
  }
});

test('page and hover start with the owner, like the server, so nothing changes for old campaigns', () => {
  const a0 = src.indexOf('var ACCESS_DEFAULT = ');
  const def = vm.runInNewContext('JSON.stringify(' + src.slice(a0 + 'var ACCESS_DEFAULT = '.length, src.indexOf(';', a0)) + ')');
  assert.deepEqual(JSON.parse(def), { sheet: 'everyone', page: 'owner', hover: 'owner', dmscreen: 'codm' });
});

test('access is part of Save, Discard and Reset and is sent with the look', () => {
  assert.match(src, /page:\['page\.look', 'page\.kit', 'access\.page', 'access\.dmscreen'\]/);
  assert.match(src, /hover:\['hover\.look', 'hover\.timelinePopup', 'access\.hover'\]/);
  assert.match(src, /sheet:\['sheet\.style', 'access\.sheet'\]/);
  assert.match(src, /page:clone\(d\.page\), access:clone\(d\.access\)/, 'toInput must send access');
  assert.match(src, /d\.access = Object\.assign\(\{\}, ACCESS_DEFAULT, d\.access \|\| \{\}\)/, 'fromServer must fill the server defaults');
  assert.match(src, /'access\.' \+ key/, 'each row must bind its access key');
  assert.match(src, /Who else may pick their own\?/);
  assert.doesNotMatch(src, /id:'access'/, 'the standalone access section is gone');
});

test('the access panels sit at the end of the sheet, page and hover sections', () => {
  for (const [id, call] of [['sheet', "accessFld('sheet'"], ['page', "accessFld('page'"], ['hover', "accessFld('hover'"]]) {
    assert.match(src, new RegExp("id === '" + id + "'\\) return html \\+ " + call.replace(/[()']/g, (c) => '\\' + c)), id);
  }
  assert.match(src, /accessFld\('dmscreen'/, 'the DM Screen kit control sits with the page kit');
});

test('quick setups set every row the way the brief names them', () => {
  const quick = grab('QUICK').map((q) => [q[0], q[3]]);
  assert.deepEqual(Object.fromEntries(quick), {
    locked: { sheet: 'owner', page: 'owner', hover: 'owner', dmscreen: 'owner' },
    table: { sheet: 'everyone', page: 'codm', hover: 'owner', dmscreen: 'codm' },
    open: { sheet: 'everyone', page: 'everyone', hover: 'everyone', dmscreen: 'codm' },
  });
  for (const [, m] of quick) assert.notEqual(m.dmscreen, 'everyone');
});

test('viewing as follows the server rule: co-DM is a scribe or above', () => {
  const a = src.indexOf('function mayPickAs(');
  const b = src.indexOf('\n    }', a) + 6;
  const mayPickAs = vm.runInNewContext('(' + src.slice(a, b).replace('function mayPickAs', 'function') + ')');
  const cases = [
    ['owner', 'everyone', false], ['scribe', 'owner', false], ['scribe', 'codm', true], ['scribe', 'everyone', true],
    ['player', 'owner', false], ['player', 'codm', false], ['player', 'everyone', true],
  ];
  for (const [role, setting, want] of cases) assert.equal(mayPickAs(role, setting), want, role + ' ' + setting);
});
