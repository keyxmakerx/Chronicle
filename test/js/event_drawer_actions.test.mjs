// event_drawer_actions.test.mjs — the event-drawer Action set wiring contracts
// (C-CAL-EDITOR-EXPANSION PR1). Source-level seams on event_grid.js:
//   1. Duplicate-to-date POSTs the create endpoint with the STORED event's full
//      body (lossless) + the chosen date, id/span dropped.
//   2. Permalink copies a ?focus=<id> URL; on load, ?focus= opens that event's
//      drawer via the existing path (guarded to the visible set).
//   3. Create-entity POSTs the create-entity endpoint and refreshes the ties.
//   4. The action listeners wire once.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'event_grid.js'), 'utf8');

function block(name) {
  const start = src.indexOf('function ' + name);
  assert.ok(start >= 0, name + ' present');
  // grab a generous window — enough to cover the function body
  return src.slice(start, start + 1400);
}

test('duplicate-to-date POSTs the stored event body + new date (lossless)', () => {
  const b = block('duplicateToDate');
  assert.match(b, /for \(var k in currentEvent\)/, 'must copy from the STORED event (lossless)');
  assert.match(b, /delete body\.id/, 'the duplicate must not carry the source id');
  assert.match(b, /delete body\.end_year/, 'the multi-day span is dropped for the copy');
  assert.match(b, /body\.year = y;\s*body\.month = mo;\s*body\.day = d;/, 'must set the chosen new date');
  assert.match(b, /'\/campaigns\/' \+ campaignID \+ '\/calendars\/' \+ calendarID \+ '\/events'/, 'POSTs the existing create endpoint');
  assert.match(b, /method: 'POST'/, 'duplicate is a create (POST)');
});

test('permalink copies a ?focus= URL and the load handler opens that event', () => {
  const b = block('copyPermalink');
  assert.match(b, /\/calendar\/v2\/' \+ calendarID \+ '\/month\?focus=' \+ encodeURIComponent\(editingID\)/,
    'permalink targets the calendar with ?focus=<eventId>');
  assert.match(b, /navigator\.clipboard/, 'copies to the clipboard');
  // focus-open on load: ?focus= opens the drawer, guarded to the visible set.
  assert.match(src, /URLSearchParams\(window\.location\.search\)\.get\('focus'\)/, 'reads ?focus= on load');
  assert.match(src, /if \(focusID && eventByID\(focusID\)\) openDrawer\(focusID\)/,
    'opens the focused event only when it is in the visible set (silent no-op otherwise)');
});

test('create-entity POSTs the create-entity endpoint and refreshes the ties', () => {
  const b = block('createEntityFromEvent');
  assert.match(b, /events\/' \+ editingID \+ '\/create-entity'/, 'POSTs the create-entity endpoint');
  assert.match(b, /entity_type_id: typeID/, 'sends the picked entity type');
  assert.match(b, /loadTies\(\)/, 'refreshes the Linked-entities chips so the new entity shows');
  assert.match(b, /res\.edit_url/, 'the toast links to the new entity');
});

test('the action listeners wire once', () => {
  assert.match(src, /_actionsWired/, 'a once-guard gates the action listeners');
  assert.match(src, /data-action-create-entity/, 'wires the create-entity button');
  assert.match(src, /data-action-duplicate/, 'wires the duplicate button');
  assert.match(src, /data-action-permalink/, 'wires the permalink button');
});
