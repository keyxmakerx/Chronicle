import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const chip = require('../../static/js/chip_ink.js');

// The tag picker's palette, the default grey and the category presets.
const colours = [
  '#6366f1', '#8b5cf6', '#ec4899', '#f43f5e', '#f97316', '#eab308', '#22c55e', '#14b8a6', '#06b6d4', '#3b82f6',
  '#6b7280', '#f59e0b', '#10b981', '#ef4444', '#64748b', '#b45309', '#dc2626',
];

test('every shipped colour reads on its tint in both themes', () => {
  for (const c of colours) {
    const { light, dark } = chip.ink(c);
    for (const [ink, surfaces] of [[light, chip.LIGHT], [dark, chip.DARK]]) {
      for (const s of surfaces) {
        const bg = chip.over(c, s, chip.TINT);
        const got = chip.contrast(ink, bg);
        assert.ok(got >= 4.5, `${c}: ${ink} on ${bg} is ${got.toFixed(2)}:1`);
      }
    }
  }
});

test('a colour that already reads is left alone', () => {
  assert.equal(chip.ink('#111827').light, '#111827');
  assert.equal(chip.ink('#f9fafb').dark, '#f9fafb');
});

test('anything that is not #rrggbb comes back unchanged', () => {
  for (const bad of ['', '#fff', 'red', '#6b7280;x:y']) {
    assert.deepEqual(chip.ink(bad), { light: bad, dark: bad });
  }
});

test('paint sets the tint, both word colours and the class', () => {
  const props = {};
  const el = {
    classList: { add(c) { props.cls = c; } },
    style: { setProperty(k, v) { props[k] = v; } },
  };
  chip.paint(el, '#6B7280');
  assert.equal(props.cls, 'chip-ink');
  assert.equal(el.style.backgroundColor, '#6b728022');
  assert.match(props['--chip-l'], /^#[0-9a-f]{6}$/);
  assert.match(props['--chip-d'], /^#[0-9a-f]{6}$/);
});

test('paint leaves a chip with a bad colour alone', () => {
  const el = { classList: { add() { throw new Error('touched'); } }, style: {} };
  chip.paint(el, 'nope');
  assert.equal(el.style.backgroundColor, undefined);
});
