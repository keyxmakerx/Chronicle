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

// --- DC-CLEAR-1 / DC-MOBILE-4: `clear` is measured, and it has a reader ------

test('the bottom sheet measures the Ledger it covers instead of declaring itself clear', () => {
  // The measured 390px geometry: the sheet is the full width at the foot of the
  // viewport and the narrow layout has stacked the Ledger below the grid, so
  // the two really do intersect (54,260 px² on the shipped Bench). The first cut
  // returned `clear: true` here without ever looking at the rect.
  const at = P.placeCard(
    { left: 40, top: 300, right: 90, bottom: 350, width: 50, height: 50 },
    { w: 390, h: 799 }, { w: 390, h: 799 },
    { left: 9, top: 594, width: 372, height: 146 }, {});
  assert.equal(at.sheet, true);
  assert.equal(at.clear, false, 'a sheet sitting over the docked Ledger must say so');
});

test('a bottom sheet that clears the Ledger reports clear', () => {
  const at = P.placeCard(
    { left: 40, top: 300, right: 90, bottom: 350, width: 50, height: 50 },
    { w: 340, h: 200 }, { w: 390, h: 800 },
    { left: 9, top: 100, width: 372, height: 146 }, {});
  assert.equal(at.sheet, true);
  assert.equal(at.clear, true);
});

test('clear is measured AFTER the dodge, from the box that actually landed', () => {
  // A day under the column, with room to dodge left: the pre-dodge Y-overlap is
  // true and the post-dodge intersection is false, and only the second is the
  // truth about what the viewer sees.
  const at = P.placeCard(
    { left: 860, top: 300, right: 944, bottom: 384, width: 84, height: 84 },
    { w: 340, h: 200 }, { w: 1232, h: 900 },
    { left: 900, top: 200, width: 300, height: 600 }, {});
  assert.equal(at.clear, true);
  assert.ok(at.left + 340 <= 900);
});

test('a Ledger the card misses on the Y axis alone is not an occlusion', () => {
  const at = P.placeCard(
    { left: 860, top: 40, right: 944, bottom: 124, width: 84, height: 84 },
    { w: 340, h: 100 }, { w: 1232, h: 900 },
    { left: 900, top: 600, width: 300, height: 200 }, {});
  assert.equal(at.clear, true, 'the column is 400px below the card');
});

test('no docked Ledger is not an occlusion — it is the card being the only answer', () => {
  for (const ledger of [null, undefined, { left: 900, top: 0, width: 0, height: 600 }]) {
    const at = P.placeCard(
      { left: 400, top: 300, right: 484, bottom: 384, width: 84, height: 84 },
      { w: 340, h: 200 }, { w: 1232, h: 900 }, ledger, {});
    assert.equal(at.clear, true);
  }
});

test('the occlusion reporter speaks exactly once, and never for the signed bottom sheet', () => {
  const said = [];
  const report = P.occlusionReporter((m) => said.push(m));

  assert.equal(report({ clear: true, sheet: false }), false, 'a clear placement says nothing');
  assert.equal(report({ clear: false, sheet: true }), false,
    'the mobile bottom sheet is the SIGNED treatment (DC-3 bullet 4, §12 scopes the ' +
    'STOP-AND-FLAG to 1232px) and must not train the next hand to ignore the warning');
  assert.equal(said.length, 0);

  assert.equal(report({ clear: false, sheet: false }), true, 'a covered Ledger must speak');
  assert.equal(said.length, 1);
  assert.match(said[0], /Open in the Ledger/);

  // One per session, not one per placement: the card repositions on every open.
  assert.equal(report({ clear: false, sheet: false }), false);
  assert.equal(said.length, 1);
});

test('applyPlacement writes the report onto the box — the flag has a reader', () => {
  const el = { style: {}, _cls: new Set(), _attr: {} };
  el.classList = {
    toggle: (c, on) => { if (on) el._cls.add(c); else el._cls.delete(c); },
  };
  el.setAttribute = (k, v) => { el._attr[k] = v; };

  P.applyPlacement(el, { left: 12, top: 34, width: 0, sheet: false, clear: true }, null);
  assert.equal(el.style.left, '12px');
  assert.equal(el.style.top, '34px');
  assert.equal(el._attr['data-dc-clear'], '1');
  assert.equal(el._cls.has('dcsheet'), false);
  assert.equal(el.style.width, '', 'a non-sheet must not keep a sheet width');

  P.applyPlacement(el, { left: 0, top: 500, width: 390, sheet: true, clear: false }, null);
  assert.equal(el._attr['data-dc-clear'], '0', 'the occlusion is recorded on the DOM at every width');
  assert.equal(el._cls.has('dcsheet'), true);
  assert.equal(el.style.width, '390px');

  // Crossing the breakpoint back up clears the sheet width rather than pinning
  // a desktop card to the phone's.
  P.applyPlacement(el, { left: 20, top: 40, width: 0, sheet: false, clear: true }, null);
  assert.equal(el.style.width, '');
  assert.equal(el._cls.has('dcsheet'), false);
  assert.equal(el._attr['data-dc-clear'], '1');
});

test('applyPlacement hands every placement to the reporter, clear or not', () => {
  const seen = [];
  const el = { style: {}, classList: { toggle() {} }, setAttribute() {} };
  P.applyPlacement(el, { left: 0, top: 0, width: 0, sheet: false, clear: true }, (at) => seen.push(at.clear));
  P.applyPlacement(el, { left: 0, top: 0, width: 0, sheet: false, clear: false }, (at) => seen.push(at.clear));
  assert.deepEqual(seen, [true, false]);
});

test('the one interpolated selector value is gated to the two key namespaces', () => {
  assert.equal(P.ordIsSafe('12'), true);
  assert.equal(P.ordIsSafe('i1'), true);
  assert.equal(P.ordIsSafe('1"] , [data-x'), false);
  assert.equal(P.ordIsSafe(''), false);
  assert.equal(P.ordIsSafe(null), false);
});
