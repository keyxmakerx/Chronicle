// customize_brush_per_campaign.test.mjs — hiding the header brush is saved for
// one campaign: the widget writes to that campaign's own address and never to
// the account-wide display choices.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'customize_brush.js'), 'utf8');

const sandbox = { Chronicle: { register() {} } };
sandbox.window = sandbox;
vm.runInNewContext(src, sandbox);
const B = sandbox.Chronicle._customizeBrush;

test('the save address names the campaign', () => {
  const cases = [
    ['a campaign id', 'c-1', '/account/brush/c-1'],
    ['another campaign gets its own address', 'c-2', '/account/brush/c-2'],
    ['odd characters are escaped, never raw', 'a/b?c', '/account/brush/a%2Fb%3Fc'],
  ];
  for (const [name, id, want] of cases) assert.equal(B.brushUrl(id), want, name);
});

test('the widget no longer writes the brush to the account-wide choices', () => {
  assert.ok(!/view-prefs',\s*\{\s*method:\s*'PUT',\s*body:\s*\{\s*brush/.test(src));
  assert.ok(src.includes('brushUrl(cid)'));
});
