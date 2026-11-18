// desk_blank_browser.test.mjs -- the desk's dashed words work from the
// keyboard alone: Enter opens the menu on the chosen row, arrows move, Enter
// picks and saves, Escape closes and gives focus back, a failed save puts the
// old word back. Runs static/js/desk_blank.js in a real Chromium against a
// stubbed save; skips locally, and fails under CI, without a browser.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const SCRIPT = readFileSync(join(root, 'static/js/desk_blank.js'), 'utf8');

const OPTIONS = JSON.stringify([
  { v: 'false', l: 'private', a: ' only members.' },
  { v: 'true', l: 'public', a: ' anyone.' },
]).replace(/"/g, '&quot;');

const PAGE = `<!doctype html><html><body>
<div data-desk>
  <div data-line="pages"><p>The campaign is
    <span role="button" tabindex="0" id="b" data-desk-blank="choice" data-label="The campaign is" data-url="/save"
      data-method="PUT" data-field="isPublic" data-bool data-value="false" data-options="${OPTIONS}"
      aria-haspopup="menu" aria-expanded="false" aria-disabled="false" aria-label="The campaign is: private"
      onclick="(function(b,e){var C=window.Chronicle=window.Chronicle||{};function go(){if(C.deskBlank)C.deskBlank.activate(b,e);}if(C.deskBlank){go();return;}var s=document.querySelector('script[data-desk-blank-js]');if(!s){s=document.createElement('script');s.src='/static/js/desk_blank.js?v=t';s.setAttribute('data-desk-blank-js','');document.head.appendChild(s);}s.addEventListener('load',go);})(this,event)" onkeydown="(function(b,e){if(e.key!=='Enter'&&e.key!==' ')return;e.preventDefault();var C=window.Chronicle=window.Chronicle||{};function go(){if(C.deskBlank)C.deskBlank.activate(b,e);}if(C.deskBlank){go();return;}var s=document.querySelector('script[data-desk-blank-js]');if(!s){s=document.createElement('script');s.src='/static/js/desk_blank.js?v=t';s.setAttribute('data-desk-blank-js','');document.head.appendChild(s);}s.addEventListener('load',go);})(this,event)">private</span><span data-desk-after> only members.</span></p>
    <span class="desk-acts"></span></div>
</div>
<button id="next">next</button>
</body></html>`;

async function open({ failSave = false } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const page = await browser.newPage();
  await page.route('http://t.test/**', (route) => {
    if (new URL(route.request().url()).pathname === '/static/js/desk_blank.js') {
      return route.fulfill({ contentType: 'text/javascript', body: SCRIPT });
    }
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await page.goto('http://t.test/');
  await page.evaluate(({ failSave }) => {
    window.saves = [];
    window.Chronicle = {
      apiAction: (url, opts) => {
        window.saves.push({ url, method: opts.method, body: opts.body });
        return failSave ? Promise.reject(new Error('refused')) : Promise.resolve({ status: 'ok' });
      },
      notify: () => {},
    };
  }, { failSave });
  return { browser, page };
}

// The first Enter must work even though nothing has loaded the script yet: the
// handler loads it and then opens the menu.
const focusedLabel = (page) => page.evaluate(() => document.activeElement && document.activeElement.textContent.trim());

test('Enter opens the menu with focus on the chosen row', { skip }, async () => {
  const { browser, page } = await open();
  try {
    await page.focus('#b');
    await page.keyboard.press('Enter');
    await page.waitForSelector('[role=menu]');
    assert.equal(await page.getAttribute('#b', 'aria-expanded'), 'true');
    assert.equal(await focusedLabel(page), 'private');
    assert.equal(await page.locator('[role=menuitemradio]').count(), 2);
  } finally { await browser.close(); }
});

test('arrows move, Enter picks and saves once with the right body', { skip }, async () => {
  const { browser, page } = await open();
  try {
    await page.focus('#b');
    await page.keyboard.press('Enter');
    await page.waitForSelector('[role=menu]');
    await page.keyboard.press('ArrowDown');
    assert.equal(await focusedLabel(page), 'public');
    await page.keyboard.press('ArrowDown');
    assert.equal(await focusedLabel(page), 'private', 'arrows wrap');
    await page.keyboard.press('End');
    assert.equal(await focusedLabel(page), 'public');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => window.saves.length === 1);
    const saves = await page.evaluate(() => window.saves);
    assert.deepEqual(saves, [{ url: '/save', method: 'PUT', body: { isPublic: true } }]);
    assert.equal((await page.textContent('#b')).trim(), 'public');
    assert.match(await page.textContent('[data-desk-after]'), /anyone/);
    assert.equal(await page.getAttribute('#b', 'aria-expanded'), 'false');
    await page.waitForSelector('.desk-saved');
  } finally { await browser.close(); }
});

test('Escape closes the menu, keeps the word and returns focus to it', { skip }, async () => {
  const { browser, page } = await open();
  try {
    await page.focus('#b');
    await page.keyboard.press('Space');
    await page.waitForSelector('[role=menu]');
    await page.keyboard.press('Escape');
    await page.waitForSelector('[role=menu]', { state: 'detached' });
    assert.equal(await page.evaluate(() => document.activeElement.id), 'b');
    assert.equal((await page.textContent('#b')).trim(), 'private');
    assert.equal(await page.evaluate(() => window.saves.length), 0);
  } finally { await browser.close(); }
});

test('Tab closes the menu and carries on from the word', { skip }, async () => {
  const { browser, page } = await open();
  try {
    await page.focus('#b');
    await page.keyboard.press('Enter');
    await page.waitForSelector('[role=menu]');
    await page.keyboard.press('Tab');
    await page.waitForSelector('[role=menu]', { state: 'detached' });
    assert.equal(await page.evaluate(() => document.activeElement.id), 'next');
  } finally { await browser.close(); }
});

test('a failed save puts the old word and its ending back', { skip }, async () => {
  const { browser, page } = await open({ failSave: true });
  try {
    await page.focus('#b');
    await page.keyboard.press('Enter');
    await page.waitForSelector('[role=menu]');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => window.saves.length === 1);
    await page.waitForFunction(() => document.getElementById('b').textContent.trim() === 'private');
    assert.match(await page.textContent('[data-desk-after]'), /only members/);
    assert.equal(await page.locator('.desk-saved').count(), 0);
  } finally { await browser.close(); }
});
