// customize_look_same_colour.test.mjs — the row of looks compares colours
// without regard to case, as the server's check of a saved look does, so a
// saved starter look is not called Custom because a colour was stored as
// #9A4A26 and the look lists #9a4a26.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');

function slice(start, end) {
  const a = src.indexOf(start);
  const b = src.indexOf(end, a);
  assert.ok(a > 0 && b > a, `${start} not found`);
  return src.slice(a, b + end.length);
}

const code = slice('function same(a, b)', '}') + slice('function sameLook(a, b)', '}');

test('colours match across case, everything else must be equal', () => {
  const run = (a, b) => vm.runInNewContext(code + `;sameLook(${JSON.stringify(a)}, ${JSON.stringify(b)})`);
  assert.equal(run('#9A4A26', '#9a4a26'), true);
  assert.equal(run('#9a4a26', '#9a4a27'), false);
  assert.equal(run('lift', 'Lift'), true, 'words compare the same way, harmlessly');
  assert.equal(run(null, null), true);
  assert.equal(run(null, '#fff'), false);
  assert.equal(run({ a: 1 }, { a: 1 }), true);
});
