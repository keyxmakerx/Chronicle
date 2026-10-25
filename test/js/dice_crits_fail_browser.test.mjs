// dice_crits_fail_browser.test.mjs — every crit failure plays its grand and
// its echo in a real Chromium without an error. Separate from
// dice_browser.test.mjs because the crits play in real time on a software
// GPU, so CI gives them a runner of their own.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { skip, playCrits } from './_dice_page.mjs';

test('every crit failure plays its grand and its echo, without an error', { skip, timeout: 600000 }, async (t) => {
  assert.deepEqual(await playCrits(t, 'fail'), ['abyss', 'crows', 'frost', 'grasp', 'hex', 'shatter']);
});
