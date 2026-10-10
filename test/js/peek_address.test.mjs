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
