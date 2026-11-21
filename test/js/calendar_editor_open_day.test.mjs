// In Edit mode a double click, or Enter, on the only chosen day opens its card
// (the same door as the Open day button), since the day roller lives there.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const src = readFileSync(new URL('../../static/js/widgets/calendar_editor.js', import.meta.url), 'utf8');
const sandbox = { module: { exports: {} }, console, Chronicle: {}, document: { querySelector: () => null, addEventListener() {} }, window: {} };
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(src, sandbox);
const { soleChosen } = sandbox.module.exports;

test('soleChosen is true only for the one chosen day', () => {
  const cases = [
    [{ '1_1_5': true }, '1_1_5', true],
    [{ '1_1_5': true }, '1_1_6', false],
    [{ '1_1_5': true, '1_1_6': true }, '1_1_5', false],
    [{ '1_1_5': false }, '1_1_5', false],
    [{}, '1_1_5', false],
    [null, '1_1_5', false],
  ];
  for (const [sel, key, want] of cases) assert.equal(soleChosen(sel, key), want, JSON.stringify(sel) + ' ' + key);
});

test('the stage opens the day card on Enter and on a double click while editing', () => {
  assert.match(src, /e\.key === 'Enter' && soleChosen\(self\.selection, day\.dataset\.key\)[\s\S]{0,120}view\.openWing\(day\.dataset\.key\)/);
  assert.match(src, /stage\.addEventListener\('dblclick'[\s\S]{0,700}self\.view\.openWing\(key\)/);
});
