// availability_labels.test.mjs — guard for C-CAL-BETA-RESCUE #3:
// "the reserve/proposal flow renders 'all h's'".
//
// STATUS: could NOT be reproduced in the current code. hourLabel() (availability.js
// :42) and every one of its callers — the my-availability axis, the team-overlay
// hour axis, the per-member lane titles, and the DM slot-builder chips — render
// correct 12-hour clock strings ("12 AM" … "11 PM", "6 PM–10 PM"). No path emits a
// literal 'h'. The server-side proposal/RSVP labels are separately unit-tested
// (proposals_service_test.go: TimeLabel == "7:00 PM – 9:00 PM").
//
// This suite pins that contract so a future regression that reintroduces a stray
// 'h' (e.g. a 24h "18h" style, or a moment-style "hh:mm" leaking in) is caught.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot, wait } from './availability_harness.mjs';

const EXPECTED_AXIS = [
  '12 AM', '1 AM', '2 AM', '3 AM', '4 AM', '5 AM', '6 AM', '7 AM', '8 AM', '9 AM', '10 AM', '11 AM',
  '12 PM', '1 PM', '2 PM', '3 PM', '4 PM', '5 PM', '6 PM', '7 PM', '8 PM', '9 PM', '10 PM', '11 PM',
];
// A "bad" label is one carrying a standalone 'h' or a run of h's — the "all h's"
// symptom (24h "18h" suffix, "hh:mm" moment layout, etc.).
const hasStrayH = (s) => /\bh\b|hh/i.test(s);

test('the my-availability grid hour axis renders correct 12-hour clock labels', async () => {
  const { doc } = boot();
  await wait(30);
  const cells = doc.querySelectorAll('[data-avail-panel="mine"] .avail-grid .hcell');
  const labels = cells.map((c) => c.textContent);
  assert.deepEqual(labels, EXPECTED_AXIS);
  assert.ok(!labels.some(hasStrayH), 'no axis label contains a stray "h"');
});

test('the team-overlay hour axis renders correct 12-hour clock labels', async () => {
  const { doc } = boot();
  await wait(20);
  doc.querySelector('[data-avail-tab="overlay"]').click();
  await wait(40);
  const grid = doc.querySelector('[data-ov-grid] .ov-weekgrid');
  // Layout: [corner, 7 day headers, hour-axis, 7 day columns]; index 8 is the axis.
  const axis = grid.children[8];
  const labels = axis.children.map((c) => c.textContent);
  assert.deepEqual(labels, EXPECTED_AXIS);
  assert.ok(!labels.some(hasStrayH), 'no axis label contains a stray "h"');
});

test('per-member lane titles read as a 12-hour clock range, never "h"', async () => {
  const { doc } = boot();
  await wait(20);
  doc.querySelector('[data-avail-tab="overlay"]').click();
  await wait(40);
  // The default overlay gives Alice a lane on day 2, 18:00–22:00.
  const lanes = doc.querySelectorAll('[data-ov-grid] .ov-lane');
  assert.ok(lanes.length >= 1, 'at least one member lane rendered');
  const title = lanes[0].getAttribute('title');
  assert.match(title, /\d{1,2} (AM|PM)/, 'lane title carries a clock label');
  assert.ok(!hasStrayH(title), `lane title must not contain a stray "h": ${title}`);
});

test('DM slot-builder chip labels a picked slot as a 12-hour clock range', async () => {
  const { doc } = boot();
  await wait(20);
  doc.querySelector('[data-avail-tab="overlay"]').click();
  await wait(40);
  doc.querySelector('[data-ov-build-toggle]').click();
  await wait(30);
  const col = doc.querySelector('[data-ov-grid] .ov-daycol');
  assert.ok(col, 'a day column is available for the builder');
  // Give the column a real height so the drag maps to 18:00–22:00 (26px/hour).
  col.getBoundingClientRect = () => ({ top: 0, left: 0, right: 100, bottom: 624, width: 100, height: 624 });
  col.__fire('mousedown', { clientX: 5, clientY: 18 * 26 });
  col.__fire('mouseup', { clientX: 5, clientY: 22 * 26 });
  await wait(20);
  const chip = doc.querySelector('[data-ov-builder] .ov-slotchip');
  assert.ok(chip, 'a slot chip rendered after the drag');
  const text = chip.textContent;
  assert.match(text, /6 PM.*10 PM/, `chip reads a clock range, got: ${text}`);
  assert.ok(!hasStrayH(text), `chip label must not contain a stray "h": ${text}`);
});
