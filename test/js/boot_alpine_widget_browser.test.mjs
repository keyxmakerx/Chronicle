// boot_alpine_widget_browser.test.mjs — a page whose Alpine components come
// from its widget's script works on a direct load and after a boosted swap:
// the mount carries x-data x-ignore, and the widget's init hands it to Alpine
// (Chronicle.startAlpine) once the components exist.
//
// Runs boot.js, the vendored htmx and Alpine in a real Chromium against a
// stubbed server. Needs playwright-core and a Chromium (see _browser_env.mjs);
// skips locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

// The widget's script arrives after Alpine has started, as an on-sight
// script does.
const WIDGET = `
window.greeter = function () { return { msg: 'hello', n: 0 }; };
Chronicle.register('ap', { init: function (el) { Chronicle.startAlpine(el); } });`;

const shell = (main) => `<!doctype html><html><head>
<script src="/htmx.js" defer></script><script src="/boot.js" defer></script>
<script src="/alpine.js" defer></script></head><body>
<nav hx-boost="true" hx-target="#main" hx-select="#main > *"><a id="go" href="/page">page</a></nav>
<main id="main">${main}</main>
<script>
window.errors = [];
window.addEventListener('error', function (e) { errors.push(String(e.message)); });
document.addEventListener('alpine:initialized', function () {
  setTimeout(function () { var s = document.createElement('script'); s.src = '/widget.js'; document.head.appendChild(s); }, 100);
});
</script></body></html>`;

const PAGE = `<div data-widget="ap" x-data x-ignore>
<div x-data="{ open: true }"><div x-data="greeter()"><span id="out" x-text="msg + ' ' + open"></span>
<button id="inc" @click="n++" x-text="n"></button></div></div></div>`;

async function open(path) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const tab = await browser.newPage();
  const consoleErrors = [];
  tab.on('console', (m) => { if (m.type() === 'error' || m.type() === 'warning') consoleErrors.push(m.text()); });
  await tab.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    const js = (f) => route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, f)) });
    if (p === '/htmx.js') return js('static/vendor/htmx.min.js');
    if (p === '/boot.js') return js('static/js/boot.js');
    if (p === '/alpine.js') return js('static/vendor/alpine.min.js');
    if (p === '/widget.js') return route.fulfill({ contentType: 'text/javascript', body: WIDGET });
    if (p === '/page') return route.fulfill({ contentType: 'text/html', body: shell(PAGE) });
    return route.fulfill({ contentType: 'text/html', body: shell('<p>home</p>') });
  });
  await tab.goto('http://t.test' + path);
  return { browser, tab, consoleErrors };
}

async function assertWorks(tab, consoleErrors) {
  await tab.waitForFunction(() => document.getElementById('out') && document.getElementById('out').textContent === 'hello true');
  await tab.click('#inc');
  await tab.click('#inc');
  assert.equal(await tab.textContent('#inc'), '2', 'the component reacts once, not twice per click');
  assert.deepEqual(consoleErrors.filter((t) => /Alpine|not defined/.test(t)), []);
}

test('a direct load starts the widget\'s Alpine components', { skip }, async () => {
  const { browser, tab, consoleErrors } = await open('/page');
  try { await assertWorks(tab, consoleErrors); } finally { await browser.close(); }
});

test('a boosted swap starts them too', { skip }, async () => {
  const { browser, tab, consoleErrors } = await open('/');
  try {
    await tab.waitForFunction(() => window.Chronicle && Chronicle.startAlpine);
    await tab.click('#go');
    await assertWorks(tab, consoleErrors);
  } finally { await browser.close(); }
});
