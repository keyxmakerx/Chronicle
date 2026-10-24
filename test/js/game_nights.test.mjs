// game_nights.test.mjs — pins the Game nights list's pure parts: the word for
// a member's answer, the attending count after they answer, and that the
// calendar link field only ever holds this site's own feed link.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'sessions', 'static', 'js', 'game_nights.js'), 'utf8');
const sandbox = { module: { exports: {} } };
vm.runInNewContext(src, sandbox);
const G = sandbox.module.exports;
const J = (x) => JSON.parse(JSON.stringify(x));

test('answerWord: the three answers, nothing for the rest (as answerWord in Go)', () => {
  const cases = [
    ['accepted', 'Going'], ['tentative', 'Maybe'], ['declined', "Can't"],
    ['invited', ''], ['', ''], ['toString', ''], [undefined, '']
  ];
  for (const [status, want] of cases) assert.equal(G.answerWord(status), want, String(status));
});

test('countAfter: going counts, and a first answer adds the member', () => {
  const cases = [
    { a: 1, t: 3, prev: 'invited', next: 'accepted', want: { accepted: 2, total: 3 } },
    { a: 1, t: 3, prev: 'invited', next: 'tentative', want: { accepted: 1, total: 3 } },
    { a: 2, t: 3, prev: 'accepted', next: 'declined', want: { accepted: 1, total: 3 } },
    { a: 2, t: 3, prev: 'accepted', next: 'accepted', want: { accepted: 2, total: 3 } },
    { a: 1, t: 2, prev: 'tentative', next: 'accepted', want: { accepted: 2, total: 2 } },
    { a: 1, t: 2, prev: '', next: 'accepted', want: { accepted: 2, total: 3 } },
    { a: 1, t: 2, prev: '', next: 'declined', want: { accepted: 1, total: 3 } },
    { a: 0, t: 0, prev: 'accepted', next: 'declined', want: { accepted: 0, total: 0 } }
  ];
  for (const c of cases) assert.deepEqual(J(G.countAfter(c.a, c.t, c.prev, c.next)), c.want, JSON.stringify(c));
});

test('feedURL: only this site\'s own feed path is shown', () => {
  const tok = 'eV6Ihdg6aJUrpKydCFGzgv1HPHI66ya2DlFX4KC7tO8';
  const cases = [
    ['https://chronicle.example.com', '/sessions/feed/' + tok + '.ics', 'https://chronicle.example.com/sessions/feed/' + tok + '.ics'],
    ['https://chronicle.example.com/', '/sessions/feed/' + tok + '.ics', 'https://chronicle.example.com/sessions/feed/' + tok + '.ics'],
    ['https://chronicle.example.com', 'https://evil.example/sessions/feed/x.ics', ''],
    ['https://chronicle.example.com', '//evil.example/sessions/feed/x.ics', ''],
    ['https://chronicle.example.com', '/sessions/feed/../../x.ics', ''],
    ['https://chronicle.example.com', '/sessions/feed/a"b.ics', ''],
    ['https://chronicle.example.com', undefined, ''],
    ['https://chronicle.example.com', '', '']
  ];
  for (const [origin, p, want] of cases) assert.equal(G.feedURL(origin, p), want, String(p));
});
