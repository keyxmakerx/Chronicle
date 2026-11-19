// dates.js: one instant, four forms, read two ways by the reader's locale.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'dates.js'), 'utf8');
const D = (() => {
  const sandbox = { module: { exports: {} }, Intl, Date, Object };
  vm.runInNewContext(src, sandbox);
  return sandbox.module.exports;
})();

const ISO = '2026-11-18T03:30:00Z';
const cases = [
  // Chicago is UTC-6 in November, so 03:30Z is still the 17th there.
  ['short', 'en-US', 'America/Chicago', 'Nov 17, 2026'],
  ['short', 'en-GB', 'Europe/London', '18 Nov 2026'],
  ['day', 'en-US', 'UTC', 'Nov 18'],
  ['day', 'en-GB', 'UTC', '18 Nov'],
  ['long', 'en-US', 'UTC', 'November 18, 2026'],
  ['long', 'en-GB', 'UTC', '18 November 2026'],
  ['weekday', 'en-US', 'UTC', 'Wednesday, November 18'],
  ['weekday', 'en-GB', 'UTC', 'Wednesday 18 November'],
  ['with-time', 'en-US', 'America/Chicago', 'Nov 17, 2026, 9:30 PM'],
  ['with-time', 'en-GB', 'Europe/London', '18 Nov 2026, 3:30'],
];
for (const [form, locale, timeZone, want] of cases) {
  test(`${form} in ${locale} (${timeZone})`, () => {
    assert.equal(D.format(ISO, form, { locale, timeZone }), want);
  });
}

test('an unreadable instant or form leaves the fallback text', () => {
  assert.equal(D.format('not a date', 'short'), null);
  assert.equal(D.format(ISO, 'nope'), null);
});

test('an unknown saved zone falls back to the device zone', () => {
  assert.equal(D.format(ISO, 'long', { locale: 'en-GB', timeZone: 'Mars/Olympus' }) !== null, true);
});
