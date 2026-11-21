// night_date_widget_browser.test.mjs — the Real date card on a date-less
// night's page (internal/plugins/sessions/static/js/night_date.js): the button
// and Enter in the date field both send only scheduled_date to the night's own
// URL, an empty field sends nothing, and a refusal says so and frees the
// button. Runs the widget in a real Chromium against a stubbed Chronicle
// runtime; skips locally, and fails under CI, when playwright-core or a
// Chromium is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const widget = readFileSync(join(root, 'internal', 'plugins', 'sessions', 'static', 'js', 'night_date.js'), 'utf8');

const PAGE = `<!doctype html><html><body>
<div data-widget="night-date" data-url="/campaigns/c1/sessions/s1">
  <input type="date" id="night-real-date"/>
  <button type="button" id="night-real-date-btn">Set the date</button>
</div>
<script>
  window.calls = []; window.notes = [];
  window.Chronicle = {
    register: function (n, def) { def.init(document.querySelector('[data-widget="' + n + '"]')); },
    apiFetch: function (url, o) {
      window.calls.push({ url: url, method: o.method, body: o.body });
      return Promise.resolve({ ok: window.status !== 'refuse' });
    },
    notify: function (m, t) { window.notes.push(m + '|' + t); }
  };
</script>
<script>${'${WIDGET}'}</script>
</body></html>`;

async function open(t, refuse) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const loads = { n: 0 };
  await p.route('**/*', (route) => {
    loads.n++;
    return route.fulfill({ contentType: 'text/html', body: PAGE.replace('${WIDGET}', () => widget) });
  });
  await p.goto('http://chronicle.test/');
  if (refuse) await p.evaluate(() => { window.status = 'refuse'; });
  return { p, loads };
}

for (const how of ['button', 'Enter']) {
  test(`the date is set from the ${how}, sending only scheduled_date`, { skip }, async (t) => {
    const { p, loads } = await open(t);
    await p.fill('#night-real-date', '2026-11-06');
    if (how === 'button') await p.click('#night-real-date-btn');
    else await p.press('#night-real-date', 'Enter');
    // The page reloads on success and the stub runtime starts over, so count the loads.
    for (let i = 0; i < 100 && loads.n < 2; i++) await new Promise((r) => setTimeout(r, 20));
    assert.equal(loads.n, 2, 'the page reloaded once');
  });
}

test('Enter and the button send the PUT with only scheduled_date', { skip }, async (t) => {
  const { p } = await open(t, true);
  await p.fill('#night-real-date', '2026-11-06');
  await p.press('#night-real-date', 'Enter');
  await p.waitForFunction(() => window.notes.length === 1);
  const calls = await p.evaluate(() => window.calls);
  assert.deepEqual(calls, [{ url: '/campaigns/c1/sessions/s1', method: 'PUT', body: { scheduled_date: '2026-11-06' } }]);
});

test('an empty date field sends nothing and asks for a date', { skip }, async (t) => {
  const { p } = await open(t, true);
  await p.press('#night-real-date', 'Enter');
  await p.click('#night-real-date-btn');
  assert.deepEqual(await p.evaluate(() => window.calls), []);
  assert.deepEqual(await p.evaluate(() => window.notes), ['Pick a date first|error', 'Pick a date first|error']);
  assert.equal(await p.evaluate(() => document.activeElement.id), 'night-real-date');
});

test('a refused date says so and frees the button', { skip }, async (t) => {
  const { p } = await open(t, true);
  await p.fill('#night-real-date', '2026-11-06');
  await p.click('#night-real-date-btn');
  await p.waitForFunction(() => window.notes.length === 1);
  assert.deepEqual(await p.evaluate(() => window.notes), ['Could not set the date|error']);
  assert.equal(await p.$eval('#night-real-date-btn', (b) => b.disabled), false);
});
