// daycard_payload.test.mjs — C-CALV4-DAYCARD (R2-2a). The pure mappers: the
// page payload → the index the card reads, the card's head line, the register's
// two durations, and the placement rule that keeps the docked Ledger uncovered.
//
// NOTE: the mappers run in a vm realm, so their objects carry a foreign
// prototype — assert on primitive fields, as calendar_permissions.test.mjs
// already does for the same reason.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot, PAYLOAD } from './daycard_harness.mjs';

const P = boot().pure;

test('the payload indexes by calendar and then by the dayKey namespace', () => {
  const idx = P.indexPayload(JSON.stringify(PAYLOAD));
  assert.ok(idx['cal-1'], 'the calendar is missing from the index');
  assert.equal(idx['cal-1'].slug, 'harptos');
  assert.equal(idx['cal-1'].ledgerDocked, true);
  assert.equal(idx['cal-1'].days['harptos-3'].day, 3);
  assert.equal(idx['cal-1'].days['harptos-3'].events.length, 2);
  // The EMPTY day is indexed too. A card that refused to open on a quiet day
  // would teach the operator the click is dead again — which is the complaint
  // this whole slice answers.
  assert.equal(idx['cal-1'].days['harptos-4'].events.length, 0);
});

test('a malformed, empty or absent payload yields an empty index and never throws', () => {
  for (const bad of ['', null, undefined, '{not json', '[]', '{"calendars":null}', '{}']) {
    const idx = P.indexPayload(bad);
    assert.equal(Object.keys(idx).length, 0, 'input ' + JSON.stringify(bad) + ' produced entries');
  }
});

test('the head drops the weekday segment rather than printing a dangling separator', () => {
  assert.equal(P.headText({ label: '3 Deepwinter 1523', weekday: 'Thirdday' }), '3 Deepwinter 1523 · Thirdday');
  assert.equal(P.headText({ label: 'Midwinter' }), 'Midwinter');
  assert.equal(P.headText(null), '');
});

test('durations are read from the register rather than copied as numbers', () => {
  assert.equal(P.durationMS('160ms', 999), 160);
  assert.equal(P.durationMS('0.2s', 999), 200);
  assert.equal(P.durationMS('  200ms ', 999), 200);
  assert.equal(P.durationMS('', 999), 999);
  assert.equal(P.durationMS('nonsense', 999), 999);
});

test('reduced motion is a BRANCH, not a scale: the close wait is zero, never shortened', () => {
  assert.equal(P.closeDelayMS('160ms', false), 160);
  assert.equal(P.closeDelayMS('160ms', true), 0);
  // The failure this forbids: a "reduced" branch that merely divides.
  assert.notEqual(P.closeDelayMS('160ms', true), 80);
});

test('the card opens beside the day and never covers the docked Ledger', () => {
  const anchor = { left: 400, top: 300, right: 484, bottom: 384, width: 84, height: 84 };
  const size = { w: 340, h: 200 };
  const view = { w: 1232, h: 900 };
  const ledger = { left: 900, top: 200, width: 300, height: 600 };

  const at = P.placeCard(anchor, size, view, ledger, {});
  assert.equal(at.sheet, false);
  assert.equal(at.clear, true, 'the placement reports itself as clear of the Ledger');
  assert.ok(at.top >= anchor.bottom, 'the card opens below its day');
  assert.ok(at.left + size.w <= ledger.left, 'the card overlaps the docked Ledger column');
});

test('a day under the Ledger pushes the card left rather than over it', () => {
  const anchor = { left: 860, top: 300, right: 944, bottom: 384, width: 84, height: 84 };
  const at = P.placeCard(anchor, { w: 340, h: 200 }, { w: 1232, h: 900 },
    { left: 900, top: 200, width: 300, height: 600 }, {});
  assert.equal(at.clear, true);
  assert.ok(at.left + 340 <= 900, 'the card must be clamped clear of the column it links to');
});

test('a card that cannot clear the Ledger says so rather than covering it silently', () => {
  // A pathological geometry: the column starts 40px in, so nothing 340px wide
  // fits beside it. The rule is that this is VISIBLE, not that it is impossible.
  const at = P.placeCard(
    { left: 0, top: 300, right: 84, bottom: 384, width: 84, height: 84 },
    { w: 340, h: 200 }, { w: 1232, h: 900 },
    { left: 40, top: 200, width: 300, height: 600 }, {});
  assert.equal(at.clear, false, 'an occluded Ledger must be reported, not hidden');
});

test('the card flips above its day when there is no room below', () => {
  const at = P.placeCard(
    { left: 400, top: 760, right: 484, bottom: 844, width: 84, height: 84 },
    { w: 340, h: 200 }, { w: 1232, h: 900 }, null, {});
  assert.ok(at.top + 200 <= 900, 'the card must stay inside the viewport');
  assert.ok(at.top < 760, 'with no room below, the card opens above its day');
});

test('below the mobile breakpoint the card is a full-width bottom sheet', () => {
  const at = P.placeCard(
    { left: 40, top: 300, right: 90, bottom: 350, width: 50, height: 50 },
    { w: 340, h: 240 }, { w: 358, h: 700 }, null, {});
  assert.equal(at.sheet, true);
  assert.equal(at.left, 0);
  assert.equal(at.width, 358);
  assert.equal(at.top, 700 - 240);
});

test('the one interpolated selector value is gated to the two key namespaces', () => {
  assert.equal(P.ordIsSafe('12'), true);
  assert.equal(P.ordIsSafe('i1'), true);
  assert.equal(P.ordIsSafe('1"] , [data-x'), false);
  assert.equal(P.ordIsSafe(''), false);
  assert.equal(P.ordIsSafe(null), false);
});
