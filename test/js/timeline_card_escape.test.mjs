// timeline_card_escape.test.mjs — pins that the timeline's day-zoom event
// card escapes every server value, including the linked entity's type icon,
// which lands inside a class attribute. Also pins that escapeHtml and
// escapeAttr both escape quotes and angle brackets.
//
// Runs the real boot.js escape helpers and the real timeline widget source in
// a vm sandbox, so the markup asserted is the markup a browser would parse.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const bootSrc = readFileSync(join(root, 'static/js/boot.js'), 'utf8');
const vizSrc = readFileSync(join(root, 'static/js/widgets/timeline_viz.js'), 'utf8');

/** Pull the escape helpers out of boot.js and evaluate them on a bare object. */
function loadEscapers() {
  const start = bootSrc.indexOf('  Chronicle.escapeAttr = function');
  const end = bootSrc.indexOf('Chronicle.escapeHtml = Chronicle.escapeAttr;');
  assert.ok(start > 0 && end > start, 'boot.js must define escapeAttr then alias escapeHtml to it');
  const chunk = bootSrc.slice(start, end + 'Chronicle.escapeHtml = Chronicle.escapeAttr;'.length);
  const Chronicle = {};
  vm.runInNewContext(chunk, { Chronicle });
  return Chronicle;
}

/** Boot the timeline widget and return its registered implementation. */
function loadTimeline() {
  const Chronicle = loadEscapers();
  const registry = {};
  Chronicle.register = (name, impl) => { registry[name] = impl; };
  vm.runInNewContext(vizSrc, {
    Chronicle,
    document: { addEventListener() {} },
    window: {},
  });
  assert.ok(registry['timeline-viz'], 'timeline_viz.js must register "timeline-viz"');
  return registry['timeline-viz'];
}

const MARKUP_ICON = `fa-x" data-y="z" data-w="<b>q</b>`;

test('escapeHtml and escapeAttr escape quotes and angle brackets', () => {
  const C = loadEscapers();
  for (const fn of [C.escapeHtml, C.escapeAttr]) {
    const out = fn(`a"b'c<d>e&f`);
    assert.equal(out, 'a&quot;b&#39;c&lt;d&gt;e&amp;f');
  }
  assert.equal(C.escapeHtml(null), '');
  assert.equal(C.escapeHtml(undefined), '');
});

test('timeline card escapes markup in the entity icon', () => {
  const viz = loadTimeline();
  const html = viz._cardHtml({
    event_name: 'Battle',
    event_year: 1, event_month: 2, event_day: 3,
    event_entity_name: 'Hero',
    event_entity_icon: MARKUP_ICON,
  });
  // The class attribute must stay closed: no raw quote from the value, so no
  // extra attribute and no extra element.
  const tag = html.match(/<i\b[^>]*>/);
  assert.ok(tag, `icon element must be present: ${html}`);
  assert.match(tag[0], /^<i class="[^"<>]*">$/, `icon element must carry only its class attribute: ${tag[0]}`);
  assert.ok(!html.includes('<b>'), 'the icon must not add an element');
  assert.ok(tag[0].includes('&quot;'), 'the quote must survive as an entity');
});

test('timeline card escapes markup in names and categories', () => {
  const viz = loadTimeline();
  const html = viz._cardHtml({
    label: '<b>x</b>',
    event_year: 0, event_month: 1, event_day: 1,
    event_entity_name: '<u>y</u>',
    event_entity_icon: 'fa-dragon',
    event_category: '"><em>z</em>',
  });
  assert.ok(!html.includes('<b>'));
  assert.ok(!html.includes('<u>'));
  assert.ok(!html.includes('<em>'));
  assert.ok(html.includes('<i class="fa-solid fa-dragon tl-viz-card-entity-icon"></i>'), 'a valid icon renders unchanged');
  assert.ok(html.includes('Y0 M1 D1'), 'year zero still renders');
});

test('timeline card falls back to the default icon', () => {
  const viz = loadTimeline();
  const html = viz._cardHtml({ event_year: 1, event_month: 1, event_day: 1, event_entity_name: 'Hero' });
  assert.ok(html.includes('fa-circle-dot'));
});
