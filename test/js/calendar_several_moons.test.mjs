// calendar_several_moons.test.mjs — a day's moons on the month: one moon is
// its silhouette as ever; two or more share one disc, each moon's lit shape
// laid over it in its own colour. Which moons a viewer gets is _gridMoons' rule and
// stays as it was; only the drawing changed.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';
import { escapeHtml } from './_escape.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

function view(moons, dm = true) {
  const defs = {};
  const sandbox = {
    module: { exports: {} }, console, Math, Intl,
    Chronicle: { escapeHtml, register(name, def) { defs[name] = def; } },
    document: { activeElement: null, querySelector: () => null, addEventListener() {} },
    window: {},
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const v = Object.create(defs.calendar_view);
  const cal = {
    current_year: 1, current_month: 1, current_day: 1, leap_year_every: 0,
    months: [{ name: 'A', days: 30, sort_order: 0 }], weekdays: [{ name: 'One', sort_order: 0 }],
    moons,
  };
  sandbox.module.exports.MoonMath.index(cal);
  Object.assign(v, { cal, canAuthorDmOnly: dm });
  v.day = sandbox.module.exports.CalDate.dayIndex(cal, 1, 1, 4);
  return v;
}
const cyc = (i, over) => Object.assign({ id: i, name: 'Moon' + i, cycle_days: 29, phase_offset: i * 5 }, over);
const draw = (v) => v._dayMoonsHTML('k', v.day);
const count = (html, cls) => (html.match(new RegExp('class="' + cls + '[ "]', 'g')) || []).length;
const paths = (html) => count(html, 'dl');

test('one moon is drawn as its silhouette alone, with no overlay', () => {
  const v1 = view([cyc(1, { color: '#336699' })]);
  const html = v1._dayMoonsHTML('1-1-4', v1.day);
  assert.match(html, /^<span class="msil" data-moon-day="1-1-4"/);
  assert.equal(count(html, 'sil'), 1);
  assert.ok(!html.includes('--mc'), 'a lone moon carries no tint');
  assert.ok(!html.includes('over') && !html.includes('class="ol"'));
});

test('two moons share one disc, each lit shape carrying its own colour', () => {
  const html = draw(view([cyc(1, { color: '#336699' }), cyc(2, { color: '#cc3300' })]));
  assert.match(html, /^<span class="msil over" data-moon-day="k"/);
  assert.equal(count(html, 'sil ov'), 1, 'one disc');
  assert.equal((html.match(/<circle class="db"/g) || []).length, 1);
  assert.equal(paths(html), 2);
  assert.ok(html.indexOf('--mc:#336699') < html.indexOf('--mc:#cc3300'), 'calendar order');
  assert.ok(!html.includes('class="mn'), 'no count when nothing is left out');
});

test('a moon with no usable colour draws plain, and a hostile colour never reaches the markup', () => {
  const html = draw(view([cyc(1), cyc(2, { color: 'red;background:url(x)' })]));
  assert.equal(paths(html), 2);
  assert.equal((html.match(/--mc/g) || []).length, 1, 'only the moon with a colour is tinted');
  assert.match(html, /style="--mc:[#a-zA-Z0-9(),.% ]+"/, 'the colour is cut to the characters a colour can hold');
  assert.ok(!/style="[^"]*[;:][^"]*[;:]/.test(html), 'no second declaration can ride in on a colour');
});

test('three moons are all laid on the disc with no count', () => {
  const html = draw(view([1, 2, 3].map((i) => cyc(i, { color: '#336699' }))));
  assert.equal(paths(html), 3);
  assert.ok(!html.includes('class="mn'));
});

test('five moons lay three on the disc and count the other two', () => {
  const html = draw(view([1, 2, 3, 4, 5].map((i) => cyc(i, { color: '#336699' }))));
  assert.equal(paths(html), 3);
  assert.match(html, /<span class="mn" aria-hidden="true">\+2<\/span>/);
  assert.ok((html.match(/title="([^"]*)"/) || [])[1].includes('Moon5'), 'the title names every moon');
});

test('a hand-set moon that is gone leaves the disc without its shape', () => {
  const gone = cyc(2, { phase_mode: 'manual', marks: [{ year: 1, month: 1, day: 1, state: 'vanished', shown: true }] });
  const html = draw(view([cyc(1), gone]));
  assert.match(html, /^<span class="msil over"/);
  assert.equal(paths(html), 1);
  assert.ok(!html.includes('sil gone'));
});

test('when every drawn moon is gone the dashed ring stands in for the disc', () => {
  const goneMark = { year: 1, month: 1, day: 1, state: 'vanished', shown: true };
  const html = draw(view([cyc(1, { phase_mode: 'manual', marks: [goneMark] }), cyc(2, { phase_mode: 'manual', marks: [goneMark] })]));
  assert.match(html, /^<span class="msil over"/);
  assert.equal((html.match(/class="sil gone"/g) || []).length, 1);
  assert.equal(paths(html), 0);
});

test('the Director sees a moon players do not dimmed on its own; a player never gets that moon', () => {
  const unshown = cyc(2, { phase_mode: 'manual', marks: [{ year: 1, month: 1, day: 1, state: 'full', shown: false }] });
  const dmHtml = draw(view([cyc(1), unshown, cyc(3)]));
  assert.equal(paths(dmHtml), 3);
  assert.equal(count(dmHtml, 'dl dim'), 1);
  assert.match(dmHtml, /Moon2[^;]*\(players don’t see this\)/);
  const hidden = view([cyc(1), unshown, cyc(3)], false);
  const playerHtml = draw(hidden);
  assert.equal(paths(playerHtml), 3, 'a shown mark is a moon players read; the Director dimming is theirs alone');
  assert.ok(!playerHtml.includes('dim'));
  // A hand-set moon with no mark yet stays off a player's calendar entirely.
  const unmarked = cyc(4, { phase_mode: 'manual', marks: [] });
  const only = draw(view([cyc(1), unmarked], false));
  assert.match(only, /^<span class="msil"/, 'one moon left: the lone silhouette');
  assert.ok(!only.includes('Moon4'));
});

test('a calendar with no moons draws nothing', () => {
  assert.equal(draw(view([])), '');
});
