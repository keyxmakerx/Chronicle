// cutscene_layers.test.mjs — transitions and text layout are choices of
// their own, apart from what a layer is. A scene written before they existed
// must play exactly as it did: the fixture holds what the engine made of a
// set of such scenes before the split (normalised scene, timeline and what
// is on screen at sampled moments), and the engine must still make the same.

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const engine = createRequire(import.meta.url)('../../static/js/cutscene_engine.js');
const { normalise, timeline, activeAt, ending } = engine._pure;
const before = JSON.parse(readFileSync(new URL('./fixtures/cutscene_scenes_before_layers.json', import.meta.url), 'utf8'));

test('a scene written before the split plays exactly as it did', () => {
  assert.ok(before.length >= 7);
  for (const b of before) {
    const name = b.scene.name;
    assert.deepEqual(normalise(b.scene), b.normalised, name + ': normalised');
    const tl = timeline(b.scene);
    assert.deepEqual(tl.layers.map((l) => ({ ...l, end: l.end === Infinity ? 'stays' : l.end })), b.layers, name + ': layers');
    assert.equal(tl.last, b.last, name + ': last');
    assert.equal(tl.closeAt, b.closeAt, name + ': closeAt');
    for (const s of b.samples) {
      assert.deepEqual(activeAt(tl, s.t), s.active, `${name}: on screen at ${s.t}s`);
      assert.deepEqual(ending(tl, s.t), s.ending, `${name}: ending at ${s.t}s`);
    }
  }
});

test('a black background named with a separate arrival keeps the old kind name', () => {
  const kinds = { cut: 'black', fade: 'fade-to-black', 'fall-asleep': 'fall-asleep' };
  for (const [arrive, kind] of Object.entries(kinds)) {
    const s = normalise({ background: { kind: 'black', in: arrive, seconds: 4 } });
    assert.equal(s.background.kind, kind, arrive);
    assert.equal(s.background.in, undefined, arrive + ': the arrival lives in the kind name');
    assert.deepEqual(normalise({ background: { kind, seconds: 4 } }).background, s.background, arrive + ': both spellings are one scene');
  }
  // An arrival on any old name wins over the name, and the look survives.
  assert.equal(normalise({ background: { kind: 'fade-to-black', in: 'cut' } }).background.kind, 'black');
  assert.equal(normalise({ background: { kind: 'black', in: 'fall-asleep', params: { look: 'drift' } } }).background.params.look, 'drift');
  // Nothing has no arrival, and an unknown arrival leaves the kind alone.
  assert.equal(normalise({ background: { kind: 'none', in: 'fade' } }).background.kind, 'none');
  assert.equal(normalise({ background: { kind: 'fade-to-black', in: 'swirl' } }).background.kind, 'fade-to-black');
});

test('a line keeps a layout, arrival and leaving only when it chose one', () => {
  const s = normalise({ text: [
    { say: 'Saltreach', layout: 'corner', in: 'slide', out: 'slide', style: 'title' },
    { say: 'Plain', layout: 'middle', in: 'swirl', out: 'boom' },
    { say: 'Caption', style: 'caption', in: 'settle', out: 'sink' }
  ] });
  assert.deepEqual(s.text[0], { say: 'Saltreach', at: 0, seconds: 5, style: 'title', layout: 'corner', in: 'slide', out: 'slide' });
  assert.deepEqual(s.text[1], { say: 'Plain', at: 0, seconds: 5, style: 'plain' }, 'the middle and unknown choices are the defaults');
  assert.deepEqual(s.text[2], { say: 'Caption', at: 0, seconds: 5, style: 'caption', in: 'settle', out: 'sink' });
});

test('film bars are a layer on top with no colour, timed like any other', () => {
  const s = normalise({ overlays: [{ kind: 'letterbox', at: 0.5, seconds: 1.2, params: { color: '#ff0000' } }] });
  assert.deepEqual(s.overlays, [{ kind: 'letterbox', at: 0.5, seconds: 1.2, params: {} }]);
  const tl = timeline(s);
  assert.deepEqual(tl.layers[1], { type: 'overlay', index: 0, kind: 'letterbox', start: 0.5, built: 1.7, end: Infinity });
});
