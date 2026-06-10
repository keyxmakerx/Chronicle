// event_quickedit.test.mjs — the event quick-edit card's wiring contracts
// (C-CAL-QUICKEDIT). Source-level seams on event_grid.js:
//   1. An event-chip click opens the QUICK-EDIT card, not the drawer; the
//      drawer stays reachable (Full editor → openDrawer; calV2OpenDrawerByID
//      exposed for the day popover).
//   2. The quick-save is LOSSLESS: it PUTs the stored event merged with the
//      two edited fields to the existing events endpoint.
//   3. Leak discipline (E4/E5 class): the outside-click closer binds to the
//      page root (dies with the node), never to document.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'event_grid.js'), 'utf8');

test('event-chip click opens the quick-edit card (drawer one tap away)', () => {
  assert.match(src, /card\.addEventListener\('click', function \(\) \{[\s\S]*?openQuickEdit\(card\);/,
    'chip click must route to openQuickEdit');
  assert.match(src, /data-qe-expand[\s\S]*?openDrawer\(id\)/, 'Full editor must hand off to the drawer');
  assert.match(src, /window\.calV2OpenDrawerByID = openDrawer/, 'the popover drawer hook must be exposed');
  assert.match(src, /if \(!qe \|\| !ev\) \{ openDrawer\(id\); return; \}/, 'missing scaffold/data must fall back to the drawer');
});

test('quick-save is a lossless merge through the existing events PUT', () => {
  assert.match(src, /function saveQuickEdit/, 'saveQuickEdit missing');
  assert.match(src, /for \(var k in ev\)/, 'must start from the STORED event (lossless merge)');
  assert.match(src, /'\/campaigns\/' \+ campaignID \+ '\/calendars\/' \+ calendarID \+ '\/events\/' \+ qeID,\s*\{\s*\n?\s*method: 'PUT'/,
    'must PUT to the existing per-event endpoint');
});

test('outside-click closer is root-bound (no document listener leak)', () => {
  const block = src.slice(src.indexOf('Quick-edit card'), src.indexOf('Drag-to-reschedule'));
  assert.ok(block.length > 0, 'quick-edit block present before drag-reschedule');
  assert.match(block, /root\.addEventListener\('click'/, 'outside-click must bind to root');
  assert.doesNotMatch(block, /document\.addEventListener/, 'quick-edit must not add document listeners');
});
