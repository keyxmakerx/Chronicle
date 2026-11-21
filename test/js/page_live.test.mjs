import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const { isQuiet } = createRequire(import.meta.url)('../../static/js/widgets/page_live.js');

test('isQuiet waits for every kind of work in progress', () => {
  const idle = { dirty: false, editing: false, fieldFocused: false, dialogOpen: false };
  assert.equal(isQuiet(idle), true);
  for (const k of Object.keys(idle)) {
    assert.equal(isQuiet({ ...idle, [k]: true }), false, k);
  }
});
