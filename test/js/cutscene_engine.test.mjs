// cutscene_engine.test.mjs — the parts of the Cut scenes engine that decide
// what plays when: reading a scene safely, which layers are on screen at a
// moment, and when the player may close it.

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

const engine = createRequire(import.meta.url)('../../static/js/cutscene_engine.js');
const { normalise, timeline, activeAt, ending, lidOpen, eyeOpen, ESCAPE_AFTER, TEXT_IN, TEXT_OUT } = engine._pure;

const EYE_SCENE = {
  id: 's1', name: 'The Eye Opens',
  background: { kind: 'fall-asleep', seconds: 7, params: {} },
  overlays: [{ kind: 'eye', at: 7.5, seconds: 3.5, params: { color: '#3dff7a' } }],
  text: [{ say: 'Something has noticed you.', at: 11, seconds: 5, style: 'whisper' }],
  ending: { kind: 'tap' }
};

test('the kinds the engine lists are the ones it is built to draw', () => {
  assert.deepEqual(engine.kinds.background, ['fall-asleep', 'black', 'fade-to-black', 'none']);
  assert.deepEqual(engine.kinds.arrivals, ['cut', 'fade', 'fall-asleep']);
  assert.deepEqual(engine.kinds.overlays, ['eye', 'embers', 'letterbox']);
  assert.deepEqual(engine.kinds.textStyles, ['whisper', 'title', 'plain', 'caption']);
  assert.deepEqual(engine.kinds.layouts, ['middle', 'corner']);
  assert.deepEqual(engine.kinds.textArrivals, ['fade', 'rise', 'settle', 'slide']);
  assert.deepEqual(engine.kinds.textLeavings, ['fade', 'sink', 'slide']);
  // Every look has a plain name for the editor and the AI, and the default is one of them.
  for (const [kind, looks] of Object.entries(engine.looks)) {
    assert.ok(looks.length >= 3, kind);
    for (const l of looks) assert.ok(l.id && l.name && l.about, kind + ' ' + l.id);
    assert.ok(looks.some((l) => l.id === engine._pure.DEFAULT_LOOK[kind]), kind + ' default');
  }
  // Any eye takes either pupil, and each eye look names its own.
  assert.deepEqual(engine.pupils.map((p) => p.id), ['round', 'slit']);
  for (const p of engine.pupils) assert.ok(p.name && p.about, p.id);
  for (const l of engine.looks.eye) assert.ok(engine.pupils.some((p) => p.id === l.pupil), l.id + ' pupil');
});

test('the server and the engine name the same kinds', () => {
  // The Go side greps for each quoted kind; the file must carry them literally.
  const src = readFileSync(new URL('../../static/js/cutscene_engine.js', import.meta.url), 'utf8');
  for (const k of ['fall-asleep', 'black', 'fade-to-black', 'none', 'eye', 'embers', 'whisper', 'title', 'plain', 'tap', 'auto']) {
    assert.ok(src.includes(`'${k}'`), k);
  }
});

test('normalise clamps, checks and fills in what a scene leaves out', () => {
  const cases = [
    {
      name: 'empty input plays nothing and offers Close',
      in: null,
      want: { background: { kind: 'none', seconds: 7, params: {} }, overlays: [], text: [], ending: { kind: 'tap', after: 0 } }
    },
    {
      name: 'numbers are clamped to the documented ranges',
      in: { background: { kind: 'fade-to-black', seconds: 900 }, overlays: [{ kind: 'embers', at: -4, seconds: 0.01 }], text: [{ say: 'x', at: 500, seconds: 0.1 }], ending: { kind: 'auto', after: 1e9 } },
      check: (s) => {
        assert.equal(s.background.seconds, 60);
        assert.equal(s.overlays[0].at, 0);
        assert.equal(s.overlays[0].seconds, 0.5);
        assert.equal(s.text[0].at, 120);
        assert.equal(s.text[0].seconds, 0.5);
        assert.equal(s.ending.after, 60);
      }
    },
    {
      name: 'unknown kinds never break an older client',
      in: { background: { kind: 'lightning' }, overlays: [{ kind: 'dragon' }, { kind: 'eye' }], text: [{ say: 'hi', style: 'shout' }], ending: { kind: 'explode' } },
      check: (s) => {
        assert.equal(s.background.kind, 'none');
        assert.deepEqual(s.overlays.map((o) => o.kind), ['eye']);
        assert.equal(s.text[0].style, 'plain');
        assert.equal(s.ending.kind, 'tap');
      }
    },
    {
      name: 'colours must be #rrggbb, else the kind default',
      in: { overlays: [{ kind: 'eye', params: { color: 'red' } }, { kind: 'eye', params: { color: '#ABCDEF' } }, { kind: 'embers', params: { color: 'javascript:1' } }] },
      check: (s) => assert.deepEqual(s.overlays.map((o) => o.params.color), ['#3dff7a', '#abcdef', '#ff9a3c'])
    },
    {
      name: 'looks fall back to the default, never to nothing',
      in: { background: { kind: 'fall-asleep', params: { look: 'nope' } }, overlays: [{ kind: 'eye', params: { look: 'almond' } }, { kind: 'eye', params: { look: '<b>' } }] },
      check: (s) => {
        assert.equal(s.background.params.look, engine._pure.DEFAULT_LOOK['fall-asleep']);
        assert.equal(s.overlays[0].params.look, 'almond');
        assert.equal(s.overlays[1].params.look, engine._pure.DEFAULT_LOOK.eye);
      }
    },
    {
      name: 'an eye draws its look\'s own pupil unless it names a known one; nothing else has one',
      in: { overlays: [{ kind: 'eye' }, { kind: 'eye', params: { look: 'almond' } }, { kind: 'eye', params: { look: 'gather' } }, { kind: 'eye', params: { pupil: 'slit' } },
        { kind: 'eye', params: { look: 'almond', pupil: 'round' } }, { kind: 'eye', params: { look: 'almond', pupil: 'cat' } }, { kind: 'embers', params: { pupil: 'slit' } }] },
      check: (s) => {
        assert.deepEqual(s.overlays.map((o) => o.params.pupil), ['round', 'slit', 'slit', 'slit', 'round', 'slit', undefined]);
      }
    },
    {
      name: 'words are plain text, trimmed, capped and never empty',
      in: { text: [{ say: '  <img src=x onerror=alert(1)>  ' }, { say: '   ' }, { say: 'a'.repeat(400) }, { say: 7 }] },
      check: (s) => {
        assert.equal(s.text.length, 3);
        assert.equal(s.text[0].say, '<img src=x onerror=alert(1)>');
        assert.equal(s.text[1].say.length, 280);
        assert.equal(s.text[2].say, '7');
      }
    },
    {
      name: 'at most 8 overlays and 12 lines',
      in: { overlays: Array(20).fill({ kind: 'embers' }), text: Array(20).fill({ say: 'x' }) },
      check: (s) => { assert.equal(s.overlays.length, 8); assert.equal(s.text.length, 12); }
    },
    {
      name: 'a line of 0 seconds or less stays to the end',
      in: { text: [{ say: 'x', seconds: 0 }, { say: 'y', seconds: -3 }] },
      check: (s) => assert.deepEqual(s.text.map((t) => t.seconds), [0, 0])
    },
    {
      name: 'times keep tenths of a second',
      in: { background: { kind: 'fade-to-black', seconds: 6.66 }, overlays: [{ kind: 'eye', at: 7.25, seconds: 3.44 }], ending: { kind: 'auto' } },
      check: (s) => {
        assert.equal(s.background.seconds, 6.7);
        assert.equal(s.overlays[0].at, 7.3);
        assert.equal(s.overlays[0].seconds, 3.4);
        assert.equal(s.ending.after, 3);
      }
    }
  ];
  for (const c of cases) {
    const got = normalise(c.in);
    if (c.want) for (const k of Object.keys(c.want)) assert.deepEqual(got[k], c.want[k], c.name + ': ' + k);
    if (c.check) c.check(got);
  }
});

test('timeline knows when the last layer has finished arriving', () => {
  const cases = [
    { name: 'the eye scene ends with its last line', scene: EYE_SCENE, last: 16, closeAt: null },
    { name: 'a line that stays counts once it has arrived', scene: { ...EYE_SCENE, text: [{ say: 'x', at: 11, seconds: 0 }] }, last: 11 + TEXT_IN, closeAt: null },
    { name: 'black is there at once', scene: { background: { kind: 'black', seconds: 30 } }, last: 0, closeAt: null },
    { name: 'auto closes after the last layer', scene: { background: { kind: 'fade-to-black', seconds: 4 }, ending: { kind: 'auto', after: 3 } }, last: 4, closeAt: 7 },
    { name: 'an overlay counts once built', scene: { background: { kind: 'none' }, overlays: [{ kind: 'embers', at: 2, seconds: 5 }] }, last: 7, closeAt: null }
  ];
  for (const c of cases) {
    const tl = timeline(c.scene);
    assert.equal(tl.last, c.last, c.name);
    assert.equal(tl.closeAt, c.closeAt, c.name);
  }
});

test('activeAt lists the layers on screen at a moment', () => {
  const tl = timeline(EYE_SCENE);
  const at = (t) => activeAt(tl, t).map((a) => a.type + ':' + a.index);
  assert.deepEqual(at(0), ['background:0']);
  assert.deepEqual(at(7.4), ['background:0']);
  assert.deepEqual(at(8), ['background:0', 'overlay:0']);
  assert.deepEqual(at(12), ['background:0', 'overlay:0', 'text:0']);
  // A line fades out after its seconds, then is gone; the eye stays.
  assert.deepEqual(at(16 + TEXT_OUT / 2), ['background:0', 'overlay:0', 'text:0']);
  assert.deepEqual(at(16 + TEXT_OUT + 0.01), ['background:0', 'overlay:0']);

  const bg = (t) => activeAt(tl, t).find((a) => a.type === 'background');
  assert.equal(bg(0).progress, 0);
  assert.equal(bg(3.5).progress, 0.5);
  assert.equal(bg(30).progress, 1);
  const eye = (t) => activeAt(tl, t).find((a) => a.type === 'overlay');
  assert.ok(Math.abs(eye(9.25).progress - 0.5) < 1e-9);
  const line = (t) => activeAt(tl, t).find((a) => a.type === 'text');
  assert.equal(line(15).leaving, 0);
  assert.ok(line(16 + TEXT_OUT / 2).leaving > 0.4);

  // A background of none draws nothing, so it is never active.
  assert.deepEqual(activeAt(timeline({ background: { kind: 'none' } }), 5), []);
});

test('ending: the escape hatch, the quiet Close and the self-closing scene', () => {
  const tap = timeline(EYE_SCENE);
  const cases = [
    { name: 'nothing for the first two seconds', tl: tap, t: ESCAPE_AFTER - 0.01, want: { canClose: false, showClose: false, tapCloses: false, autoClose: false } },
    { name: 'Esc and the close mark from two seconds', tl: tap, t: ESCAPE_AFTER, want: { canClose: true, showClose: false, tapCloses: false, autoClose: false } },
    { name: 'Close once everything arrived', tl: tap, t: 16, want: { canClose: true, showClose: true, tapCloses: true, autoClose: false } },
    { name: 'an empty tap scene still waits two seconds', tl: timeline({}), t: 1, want: { canClose: false, showClose: false, tapCloses: false, autoClose: false } },
    { name: 'auto never shows Close', tl: timeline({ background: { kind: 'black' }, ending: { kind: 'auto', after: 4 } }), t: 3, want: { canClose: true, showClose: false, tapCloses: false, autoClose: false } },
    { name: 'auto closes itself', tl: timeline({ background: { kind: 'black' }, ending: { kind: 'auto', after: 4 } }), t: 4, want: { canClose: true, showClose: false, tapCloses: false, autoClose: true } },
    { name: 'auto never closes before the escape delay', tl: timeline({ background: { kind: 'black' }, ending: { kind: 'auto', after: 0 } }), t: 1, want: { canClose: false, showClose: false, tapCloses: false, autoClose: false } }
  ];
  for (const c of cases) assert.deepEqual(ending(c.tl, c.t), c.want, c.name);
});

test('the lids fight sleep twice, then close; each eye ends fully open', () => {
  assert.equal(lidOpen(0), 1);
  assert.equal(lidOpen(1), 0);
  // Two moments where the lids snap well back open after drooping; a last
  // small flicker before they close does not count.
  let rises = 0, low = 1;
  for (let p = 0; p <= 1; p += 0.005) {
    const v = lidOpen(p);
    if (v < low) low = v;
    else if (v - low > 0.25) { rises++; low = v; }
  }
  assert.equal(rises, 2);
  for (const lk of ['almond', 'rift', 'gather']) {
    assert.equal(eyeOpen(lk, 1), 1, lk);
    assert.ok(eyeOpen(lk, 0) <= eyeOpen(lk, 1), lk);
  }
});

test('play is browser-only and says so in node', () => {
  assert.throws(() => engine.play(null, EYE_SCENE), /browser/);
});
