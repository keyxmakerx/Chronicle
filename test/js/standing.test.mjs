// standing.test.mjs — pins the Standing block's pure parts: how much motion
// applies, the "Why:" line (the same cases as TestStandingWhyText in Go, so the
// line reads the same after a reason is added without a reload), the text
// rules for a reason (mirroring cleanStandingText), the word a reason would
// move the standing to, search and filter, and the Director's player preview.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'entities', 'static', 'js', 'standing.js'), 'utf8');
const sandbox = { module: { exports: {} } };
vm.runInNewContext(src, sandbox);
const S = sandbox.module.exports;

test('motionOf: reduced motion is always off; otherwise the page attribute decides', () => {
  const cases = [
    { attr: null, reduced: false, want: 'full' },
    { attr: 'calm', reduced: false, want: 'calm' },
    { attr: 'off', reduced: false, want: 'off' },
    { attr: 'calm', reduced: true, want: 'off' },
    { attr: 'bogus', reduced: false, want: 'full' }
  ];
  for (const c of cases) assert.equal(S.motionOf(c.attr, c.reduced), c.want, JSON.stringify(c));
});

test('whyText: the quiet line, same cases as the Go StandingWhyText', () => {
  const cases = [
    [0, '', 'Why: no reasons shared yet'],
    [1, 'Saved the granary', 'Why: Saved the granary'],
    [1, 'Saved the granary from the fire', 'Why: Saved the granary from…'],
    [5, 'Saved the granary from the fire', 'Why: Saved the granary from… and 4 more'],
    [2, 'Brawled', 'Why: Brawled and 1 more'],
    [8, 'Brawled', 'Why: Brawled and 7 more'],
    [9, 'Brawled', 'Why: 9 reasons'],
    [120, 'x', 'Why: 120 reasons']
  ];
  for (const [total, newest, want] of cases) assert.equal(S.whyText(total, newest), want, `${total} ${newest}`);
});

test('wordFor and stepsLabel: five words, anything else reads as Neutral', () => {
  ['Hostile', 'Wary', 'Neutral', 'Warm', 'Trusted'].forEach((w, i) => assert.equal(S.wordFor(i), w));
  assert.equal(S.wordFor(-1), 'Neutral');
  assert.equal(S.wordFor(9), 'Neutral');
  assert.equal(S.wordFor(1.5), 'Neutral');
  assert.equal(S.stepsLabel({ set: true, level: 1 }), 'Standing: Wary, step 2 of 5');
  assert.equal(S.stepsLabel({ set: false, level: 2 }), 'Standing not set yet');
});

test('cleanText: trims, folds whitespace, refuses control characters, never cuts', () => {
  const cases = [
    { in: '  hello   world ', max: 90, text: 'hello world', ok: true },
    { in: '', max: 90, text: '', ok: true },
    { in: 'a\tb\nc', max: 90, text: 'a b c', ok: true },
    { in: 'a\u0000b', max: 90, text: '', ok: false },
    { in: 'a\u001bb', max: 90, text: '', ok: false },
    { in: 'é'.repeat(5), max: 5, text: 'é'.repeat(5), ok: true },
    { in: 'é'.repeat(6), max: 5, text: 'é'.repeat(6), ok: false },
    { in: '🐉'.repeat(3), max: 3, text: '🐉'.repeat(3), ok: true }
  ];
  for (const c of cases) {
    const got = S.cleanText(c.in, c.max);
    assert.deepEqual({ text: got.text, ok: got.ok }, { text: c.text, ok: c.ok }, JSON.stringify(c.in));
  }
});

test('validateReason: the message the form shows', () => {
  assert.equal(S.validateReason('Paid the debt', 'session 13'), '');
  assert.equal(S.validateReason('   ', ''), 'Say what happened first.');
  assert.equal(S.validateReason('a'.repeat(91), ''), 'Keep the reason to 90 characters or fewer.');
  assert.equal(S.validateReason('a'.repeat(90), ''), '');
  assert.equal(S.validateReason('ok', 'w'.repeat(31)), 'Keep the "when" to 30 characters or fewer.');
  assert.equal(S.validateReason('a\u0000b', ''), 'Use plain text for the reason.');
});

test('suggestLevel: a reason moves the word one step, clamped to the five', () => {
  const cases = [
    { level: 1, dir: 'up', want: 2 }, { level: 1, dir: 'down', want: 0 }, { level: 1, dir: 'none', want: 1 },
    { level: 4, dir: 'up', want: 4 }, { level: 0, dir: 'down', want: 0 }
  ];
  for (const c of cases) assert.equal(S.suggestLevel(c.level, c.dir), c.want, JSON.stringify(c));
});

test('filterReasons: search is case-insensitive and the effect filter narrows', () => {
  const list = [
    { text: 'Saved the granary', direction: 'up' },
    { text: 'Brawled in the guildhall', direction: 'down' },
    { text: 'Met at the Gate', direction: 'none' },
    { text: 'Returned the ledger', direction: 'up' }
  ];
  const texts = (q, f) => S.filterReasons(list, q, f).map((r) => r.text);
  assert.equal(texts('', 'all').length, 4);
  assert.deepEqual(texts('', 'up'), ['Saved the granary', 'Returned the ledger']);
  assert.deepEqual(texts('GATE', 'all'), ['Met at the Gate']);
  assert.deepEqual(texts('the', 'down'), ['Brawled in the guildhall']);
  assert.deepEqual(texts('zzz', 'all'), []);
  assert.equal(S.filterReasons(null, '', 'all').length, 0);
});

test('infoText and moreLabel', () => {
  assert.equal(S.infoText(3, 12, false), 'Newest first');
  assert.equal(S.infoText(3, 12, true), '3 of 12 · newest first');
  assert.equal(S.moreLabel(95), 'Show 30 more (95 left)');
  assert.equal(S.moreLabel(7), 'Show 7 more (7 left)');
});

test('visibleTotal: the Director previewing as a player does not count Director-only reasons', () => {
  const sub = { total: 120, reasons: [{ dmOnly: true }, { dmOnly: false }, { dmOnly: true }] };
  assert.equal(S.visibleTotal(sub, false), 120);
  assert.equal(S.visibleTotal(sub, true), 118);
  assert.equal(S.visibleTotal({ total: 1, reasons: [{ dmOnly: true }] }, true), 0);
});

test('escapeShouldClose: Escape closes unless a modifier is held', () => {
  assert.equal(S.escapeShouldClose({ key: 'Escape' }), true);
  assert.equal(S.escapeShouldClose({ key: 'Escape', ctrlKey: true }), false);
  assert.equal(S.escapeShouldClose({ key: 'a' }), false);
  assert.equal(S.escapeShouldClose(null), false);
});

test('replaceSubject: swaps the server answer in, or adds a subject it lacked', () => {
  const subs = [{ key: 'party', level: 1 }, { key: 'm', level: 3 }];
  assert.deepEqual(S.replaceSubject(subs, { key: 'm', level: 4 }).map((s) => s.level), [1, 4]);
  assert.equal(S.replaceSubject(subs, { key: 'x', level: 0 }).length, 3);
  assert.equal(subs.length, 2, 'the input is not changed');
});
