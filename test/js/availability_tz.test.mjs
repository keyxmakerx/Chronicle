// availability_tz.test.mjs — C-TZ-CONSOLIDATION: pins that the availability
// scheduler's "Your timezone" select renders from the server-embedded
// data-common-tz JSON (internal/timeutil.CommonZonesJSON) rather than a
// hand-rolled list baked into the JS file. A regression here (someone
// reintroducing a local COMMON_TZ array) would silently re-fork the list this
// dispatch consolidated.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './availability_harness.mjs';

test('timezone select renders exactly the server-embedded data-common-tz list', () => {
  const zones = ['UTC', 'Asia/Kathmandu', 'America/New_York', 'Pacific/Auckland'];
  const { root } = boot({ commonTZ: zones });
  const select = root.querySelector('.avail-toolbar select');
  assert.ok(select, 'expected a timezone <select> in the toolbar');
  const values = select.children.map((o) => o.value);
  for (const z of zones) {
    assert.ok(values.includes(z), `expected ${z} (from data-common-tz) among the select's options`);
  }
});

test('a zone not in data-common-tz is never offered', () => {
  const zones = ['UTC', 'Asia/Kathmandu'];
  const { root } = boot({ commonTZ: zones });
  const select = root.querySelector('.avail-toolbar select');
  const values = select.children.map((o) => o.value);
  assert.ok(!values.includes('Europe/Moscow'), 'a zone absent from data-common-tz must not appear');
});

test('missing data-common-tz degrades to no base options without throwing', () => {
  const { root } = boot({ commonTZ: null });
  const select = root.querySelector('.avail-toolbar select');
  assert.ok(select, 'render must not throw when data-common-tz is absent');
  // The stored zone (data-tz=America/New_York) is still unshifted in even
  // with an empty base list, so the select is never completely empty.
  const values = select.children.map((o) => o.value);
  assert.ok(values.includes('America/New_York'));
});

test('malformed data-common-tz JSON degrades gracefully', () => {
  // commonTZRaw sets the attribute to this literal string BEFORE the app is
  // constructed, so the malformed value is actually what loadCommonTZ parses
  // (unlike mutating the attribute after boot, which the constructor never re-reads).
  const { root } = boot({ commonTZRaw: '{not valid json' });
  const select = root.querySelector('.avail-toolbar select');
  assert.ok(select, 'a malformed attribute at boot time must not crash render');
  // Falls back to an empty base list; the stored zone is still unshifted in.
  const values = select.children.map((o) => o.value);
  assert.ok(values.includes('America/New_York'));
});
