// calendar_gn_feed_url.test.mjs — the Game nights panel's calendar link
// field only ever holds this site's own feed link.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const sandbox = { module: { exports: {} }, Chronicle: { register() {} }, console };
vm.createContext(sandbox);
vm.runInContext(readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8'), sandbox);
const { gnFeedURL } = sandbox.module.exports;

test('gnFeedURL: only this site\'s own feed path is shown', () => {
  const tok = 'eV6Ihdg6aJUrpKydCFGzgv1HPHI66ya2DlFX4KC7tO8';
  const cases = [
    ['https://chronicle.example.com', '/sessions/feed/' + tok + '.ics', 'https://chronicle.example.com/sessions/feed/' + tok + '.ics'],
    ['https://chronicle.example.com/', '/sessions/feed/' + tok + '.ics', 'https://chronicle.example.com/sessions/feed/' + tok + '.ics'],
    ['https://chronicle.example.com', 'https://evil.example/sessions/feed/x.ics', ''],
    ['https://chronicle.example.com', '//evil.example/sessions/feed/x.ics', ''],
    ['https://chronicle.example.com', '/sessions/feed/../../x.ics', ''],
    ['https://chronicle.example.com', '/sessions/feed/a"b.ics', ''],
    ['https://chronicle.example.com', undefined, ''],
    ['https://chronicle.example.com', '', ''],
  ];
  for (const [origin, p, want] of cases) assert.equal(gnFeedURL(origin, p), want, String(p));
});
