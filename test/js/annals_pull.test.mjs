// annals_pull.js: when following a connection plays the pull, which way it
// goes, and how rows are named for the move.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'annals_pull.js'), 'utf8');

function load() {
  const sandbox = { module: { exports: {} } };
  vm.runInNewContext(src, sandbox);
  return sandbox.module.exports;
}
const P = load();

test('modeOf: Annals with full motion slides, Calm fades, anything else is a plain load', () => {
  const full = { annals: true, hasVT: true, reducedMotion: false, motion: null, czReduce: false, viewMotion: null };
  const cases = [
    [{}, 'slide'],
    [{ motion: 'calm', viewMotion: 'calm' }, 'fade'],
    [{ czReduce: true }, 'fade'],
    [{ viewMotion: 'calm' }, 'fade'],
    [{ motion: 'off', viewMotion: 'calm' }, ''],
    [{ reducedMotion: true }, ''],
    [{ hasVT: false }, ''],
    [{ annals: false }, ''],
  ];
  for (const [over, want] of cases) {
    assert.equal(P.modeOf({ ...full, ...over }), want, JSON.stringify(over));
  }
});

test('sideOf: the page comes from the side the block is on; a middle block counts as right', () => {
  const main = { left: 256, width: 1024 };
  assert.equal(P.sideOf({ left: 1000, width: 260 }, main), 'right');
  assert.equal(P.sideOf({ left: 270, width: 260 }, main), 'left');
  assert.equal(P.sideOf({ left: 276, width: 648 }, main), 'right');
  assert.equal(P.sideOf({ left: 256, width: 1024 }, main), 'right');
});

test('isBack: only the link back to the page the last pull came from reverses', () => {
  const trail = [{ from: '/e/a', to: '/e/b' }, { from: '/e/b', to: '/e/c' }];
  assert.equal(P.isBack(trail, '/e/c', '/e/b'), true);
  assert.equal(P.isBack(trail, '/e/c', '/e/a'), false);
  assert.equal(P.isBack(trail, '/e/b', '/e/a'), false, 'not where the last pull landed');
  assert.equal(P.isBack([], '/e/b', '/e/a'), false);
});

test('rowName: a safe transition name from the linked page id, none without one', () => {
  assert.equal(P.rowName('9d79b1a5-1e08-4ed6'), 'annals-rel-9d79b1a5-1e08-4ed6');
  assert.equal(P.rowName('a b"c'), 'annals-rel-abc');
  assert.equal(P.rowName(''), '');
  assert.equal(P.rowName(undefined), '');
});

test('lastSegment: the page id from a link, ignoring query, hash and a trailing slash', () => {
  assert.equal(P.lastSegment('/campaigns/c1/entities/e9'), 'e9');
  assert.equal(P.lastSegment('/campaigns/c1/entities/e9/?tab=x#top'), 'e9');
});
