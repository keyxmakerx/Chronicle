// peek_address.test.mjs — the one check for a page peek address. A value that
// is not exactly a page link, preview or peek address must give '' so the
// peek panel never fetches (and inserts) anything else.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'peek_address.js'), 'utf8');
const sandbox = { window: {} };
vm.runInNewContext(src, sandbox);
const peekAddress = sandbox.window.Chronicle.peekAddress;

const P = '/campaigns/c1/entities/e-1/peek';

test('peekAddress accepts only the three page shapes', () => {
  const cases = [
    ['preview address', '/campaigns/c1/entities/e-1/preview', P],
    ['peek address', '/campaigns/c1/entities/e-1/peek', P],
    ['page link', '/campaigns/c1/entities/e-1', P],
    ['uuid ids', '/campaigns/4d7d3ca5-16dd/entities/aaaaaaaa-0000-4000/preview', '/campaigns/4d7d3ca5-16dd/entities/aaaaaaaa-0000-4000/peek'],
    ['media path trick', '/media/abc?/campaigns/c1/entities/e-1/preview', ''],
    ['media path trick with entities tail', '/media/abc?/entities/x/preview', ''],
    ['other route with the same tail', '/api/v1/entities/x/preview', ''],
    ['prefix before campaigns', '/x/campaigns/c1/entities/e-1/preview', ''],
    ['query on the address', '/campaigns/c1/entities/e-1/preview?x=1', ''],
    ['fragment on the address', '/campaigns/c1/entities/e-1/peek#x', ''],
    ['trailing newline', '/campaigns/c1/entities/e-1/peek\n', ''],
    ['backslash in an id', '/campaigns/c1/entities/e\\1/peek', ''],
    ['backslash host', '/\\evil.example/campaigns/c1/entities/e-1/peek', ''],
    ['protocol-relative', '//evil.example/campaigns/c1/entities/e-1/peek', ''],
    ['absolute url', 'https://evil.example/campaigns/c1/entities/e-1/peek', ''],
    ['javascript url', 'javascript:alert(1)', ''],
    ['dot segments', '/campaigns/../entities/e-1/peek', ''],
    ['extra segment', '/campaigns/c1/entities/e-1/peek/x', ''],
    ['empty', '', ''],
    ['not a string', null, ''],
  ];
  for (const [name, input, want] of cases) {
    assert.equal(peekAddress(input), want, name);
  }
});

const E = '/campaigns/c1/systems/drawsteel/ancestries/ashborn';

test('peekAddress accepts a rules entry with only hero and printed', () => {
  const cases = [
    ['entry page', E, E + '/peek'],
    ['entry peek', E + '/peek', E + '/peek'],
    ['with a hero', E + '?hero=d80fb105-2923', E + '/peek?hero=d80fb105-2923'],
    ['printed, rebuilt in order', E + '/peek?printed=1&hero=h1', E + '/peek?hero=h1&printed=1'],
    ['another parameter', E + '?x=1', ''],
    ['two heroes', E + '?hero=a&hero=b', ''],
    ['printed other than 1', E + '?printed=yes', ''],
    ['hero with a slash', E + '?hero=a/b', ''],
    ['a fragment', E + '#traits', ''],
    ['the book is not a category', '/campaigns/c1/systems/drawsteel/book/edit', ''],
    ['data is not a category', '/campaigns/c1/systems/drawsteel/data/kits.json', ''],
    ['a tooltip address', E + '/tooltip', ''],
    ['a category alone', '/campaigns/c1/systems/drawsteel/ancestries', ''],
    ['protocol-relative', '//evil.example' + E, ''],
  ];
  for (const [name, input, want] of cases) {
    assert.equal(peekAddress(input), want, name);
  }
});

const N = '/campaigns/c1/systems/drawsteel/named/ancestry';

test('peekAddress accepts an entry by kind and name', () => {
  const cases = [
    ['named page', N + '?name=Ashborn', N + '/peek?name=Ashborn'],
    ['named peek with a hero', N + '/peek?name=High%20Elf&hero=d80fb105', N + '/peek?name=High%20Elf&hero=d80fb105'],
    ['hero first, rebuilt in order', N + '?hero=h1&name=Orc', N + '/peek?name=Orc&hero=h1'],
    ['an apostrophe is encoded again', N + "?name=Can't", N + '/peek?name=Can%27t'],
    ['no name', N + '?hero=h1', ''],
    ['two names', N + '?name=a&name=b', ''],
    ['an empty name', N + '?name=', ''],
    ['a bad escape', N + '?name=%E0%A4%A', ''],
    ['a raw space', N + '?name=High Elf', ''],
    ['a plus', N + '?name=High+Elf', ''],
    ['another parameter', N + '?name=Orc&x=1', ''],
    ['a fragment', N + '?name=Orc#x', ''],
    ['a slash in the kind', '/campaigns/c1/systems/drawsteel/named/a/b?name=Orc', ''],
    ['named is never a category', '/campaigns/c1/systems/drawsteel/named/ancestry', ''],
    ['a name over 100', N + '?name=' + 'a'.repeat(101), ''],
  ];
  for (const [name, input, want] of cases) {
    assert.equal(peekAddress(input), want, name);
  }
});
