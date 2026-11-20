// desk_blank_browser.test.mjs -- the desk's values that change where they stand
// work from the keyboard alone: Enter opens the menu on the chosen row, arrows
// move, Enter picks and saves, Escape closes and gives focus back, a failed
// save puts the old word back. The description is a field of a few lines:
// Enter makes a new line, Ctrl+Enter or leaving it saves, Escape keeps the old
// text, and emptying it shows "None". Making the campaign public turns on the
// guest preview word. Runs static/js/desk_blank.js in a real Chromium against a
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
  { v: 'false', l: 'private' },
  { v: 'true', l: 'public' },
]).replace(/"/g, '&quot;');

const PAGE = `<!doctype html><html><body>
<div data-desk>
  <div data-line="pages"><p>The campaign is
    <span role="button" tabindex="0" id="b" data-desk-blank="choice" data-label="The campaign is" data-url="/save"
      data-method="PUT" data-field="isPublic" data-bool data-value="false" data-options="${OPTIONS}"
      aria-haspopup="menu" aria-expanded="false" aria-disabled="false" aria-label="The campaign is: private"
      onclick="(function(b,e){var C=window.Chronicle=window.Chronicle||{};function go(){if(C.deskBlank)C.deskBlank.activate(b,e);}if(C.deskBlank){go();return;}var s=document.querySelector('script[data-desk-blank-js]');if(!s){s=document.createElement('script');s.src='/static/js/desk_blank.js?v=t';s.setAttribute('data-desk-blank-js','');document.head.appendChild(s);}s.addEventListener('load',go);})(this,event)" onkeydown="(function(b,e){if(e.key!=='Enter'&&e.key!==' ')return;e.preventDefault();var C=window.Chronicle=window.Chronicle||{};function go(){if(C.deskBlank)C.deskBlank.activate(b,e);}if(C.deskBlank){go();return;}var s=document.querySelector('script[data-desk-blank-js]');if(!s){s=document.createElement('script');s.src='/static/js/desk_blank.js?v=t';s.setAttribute('data-desk-blank-js','');document.head.appendChild(s);}s.addEventListener('load',go);})(this,event)">private</span></p>
    <span class="desk-acts"></span></div>
</div>
<button id="next">next</button>
</body></html>`;

const HANDLER = `onclick="(function(b,e){var C=window.Chronicle=window.Chronicle||{};function go(){if(C.deskBlank)C.deskBlank.activate(b,e);}if(C.deskBlank){go();return;}var s=document.querySelector('script[data-desk-blank-js]');if(!s){s=document.createElement('script');s.src='/static/js/desk_blank.js?v=t';s.setAttribute('data-desk-blank-js','');document.head.appendChild(s);}s.addEventListener('load',go);})(this,event)" onkeydown="(function(b,e){if(e.key!=='Enter'&&e.key!==' ')return;e.preventDefault();var C=window.Chronicle=window.Chronicle||{};function go(){if(C.deskBlank)C.deskBlank.activate(b,e);}if(C.deskBlank){go();return;}var s=document.querySelector('script[data-desk-blank-js]');if(!s){s=document.createElement('script');s.src='/static/js/desk_blank.js?v=t';s.setAttribute('data-desk-blank-js','');document.head.appendChild(s);}s.addEventListener('load',go);})(this,event)"`;

// The sheet's own markup: a choice with a chevron and hints, a description,
// and the guest word that is off while the campaign is private.
const CHOICE_OPTIONS = JSON.stringify([
  { v: 'false', l: 'Private', h: 'Members only' },
  { v: 'true', l: 'Public', h: 'Anyone with the link' },
]).replace(/"/g, '&quot;');
const SHEET = `<!doctype html><html><body>
<div data-desk>
  <ul>
    <li data-line="public"><span class="set-vals">
      <span role="button" tabindex="0" id="pub" class="desk-b set-v" data-desk-blank="choice" data-id="public" data-label="Campaign" data-url="/save"
        data-method="PUT" data-field="isPublic" data-bool data-value="false" data-options="${CHOICE_OPTIONS}"
        aria-haspopup="menu" aria-expanded="false" aria-disabled="false" aria-label="Campaign: Private" ${HANDLER}><span class="desk-t">Private</span><svg class="set-chev set-chev-down" width="12" height="12"></svg></span>
      <span class="desk-acts"></span></span></li>
    <li data-line="description"><span class="set-vals">
      <span role="button" tabindex="0" id="desc" class="desk-b set-v set-wide is-empty" data-desk-blank="text" data-id="description" data-label="Description" data-url="/save"
        data-method="PUT" data-field="description" data-maxlen="5000" data-value="" data-options="[]"
        aria-haspopup="false" aria-expanded="false" aria-disabled="false" aria-label="Description: None" ${HANDLER}><span class="desk-t">None</span></span>
      <span class="desk-acts"></span></span></li>
    <li data-line="preview"><span class="set-vals">
      <button type="button" id="guest" class="set-v set-word" data-needs-public data-off="Guests can't open a private campaign. Make it Public first." aria-disabled="true" title="Guests can't open a private campaign. Make it Public first." aria-describedby="why-preview">As a guest</button>
      </span><p class="set-why" id="why-preview">Guests can't open a private campaign. Make it Public first.</p></li>
  </ul>
</div>
<button id="next">next</button>
</body></html>`;

async function openSheet() {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const page = await browser.newPage();
  await page.route('http://t.test/**', (route) => {
    if (new URL(route.request().url()).pathname === '/static/js/desk_blank.js') {
      return route.fulfill({ contentType: 'text/javascript', body: SCRIPT });
    }
    return route.fulfill({ contentType: 'text/html', body: SHEET });
  });
  await page.goto('http://t.test/');
  await page.evaluate(() => {
    window.saves = [];
    window.Chronicle = {
      apiAction: (url, opts) => {
        window.saves.push({ url, method: opts.method, body: opts.body });
        return Promise.resolve({ status: 'ok' });
      },
      notify: () => {},
    };
  });
  return { browser, page };
}

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

test('a failed save puts the old word back', { skip }, async () => {
  const { browser, page } = await open({ failSave: true });
  try {
    await page.focus('#b');
    await page.keyboard.press('Enter');
    await page.waitForSelector('[role=menu]');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => window.saves.length === 1);
    await page.waitForFunction(() => document.getElementById('b').textContent.trim() === 'private');
    assert.equal(await page.locator('.desk-saved').count(), 0);
  } finally { await browser.close(); }
});

test('a menu row carries its hint, and the chevron survives a pick', { skip }, async () => {
  const { browser, page } = await openSheet();
  try {
    await page.focus('#pub');
    await page.keyboard.press('Enter');
    await page.waitForSelector('[role=menu]');
    assert.deepEqual(await page.locator('.desk-mh').allTextContents(), ['Members only', 'Anyone with the link']);
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => window.saves.length === 1);
    assert.equal((await page.textContent('#pub .desk-t')).trim(), 'Public');
    assert.equal(await page.locator('#pub .set-chev-down').count(), 1, 'the chevron is not wiped by the new word');
    assert.equal(await page.getAttribute('#pub', 'aria-label'), 'Campaign: Public');
  } finally { await browser.close(); }
});

test('making the campaign public turns on the guest word and hides its reason', { skip }, async () => {
  const { browser, page } = await openSheet();
  try {
    assert.equal(await page.getAttribute('#guest', 'aria-disabled'), 'true');
    await page.focus('#pub');
    await page.keyboard.press('Enter');
    await page.waitForSelector('[role=menu]');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => window.saves.length === 1);
    assert.equal(await page.getAttribute('#guest', 'aria-disabled'), 'false');
    assert.equal(await page.getAttribute('#guest', 'aria-describedby'), null);
    assert.equal(await page.evaluate(() => document.getElementById('why-preview').hidden), true);
    // And back to private: off again, with its reason.
    await page.focus('#pub');
    await page.keyboard.press('Enter');
    await page.waitForSelector('[role=menu]');
    await page.keyboard.press('ArrowUp');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => window.saves.length === 2);
    assert.equal(await page.getAttribute('#guest', 'aria-disabled'), 'true');
    assert.equal(await page.getAttribute('#guest', 'aria-describedby'), 'why-preview');
    assert.equal(await page.evaluate(() => document.getElementById('why-preview').hidden), false);
  } finally { await browser.close(); }
});

test('the description opens a field of a few lines; Enter is a new line, Ctrl+Enter saves', { skip }, async () => {
  const { browser, page } = await openSheet();
  try {
    await page.focus('#desc');
    await page.keyboard.press('Enter');
    await page.waitForSelector('textarea.desk-ta');
    assert.equal(await page.evaluate(() => document.activeElement.tagName), 'TEXTAREA');
    assert.equal(await page.evaluate(() => document.activeElement.maxLength), 5000);
    await page.keyboard.type('One');
    await page.keyboard.press('Enter');
    await page.keyboard.type('Two');
    assert.equal(await page.evaluate(() => window.saves.length), 0, 'Enter alone does not save');
    await page.keyboard.press('Control+Enter');
    await page.waitForFunction(() => window.saves.length === 1);
    assert.deepEqual(await page.evaluate(() => window.saves[0]), { url: '/save', method: 'PUT', body: { description: 'One\nTwo' } });
    assert.equal((await page.textContent('#desc')).trim(), 'One Two');
    assert.equal(await page.evaluate(() => document.activeElement.id), 'desc', 'focus returns to the value');
    assert.equal(await page.evaluate(() => document.getElementById('desc').classList.contains('is-empty')), false);
    await page.waitForSelector('.desk-saved');
  } finally { await browser.close(); }
});

test('leaving the description saves; Escape keeps the old text; emptying it shows None', { skip }, async () => {
  const { browser, page } = await openSheet();
  try {
    await page.focus('#desc');
    await page.keyboard.press('Enter');
    await page.waitForSelector('textarea.desk-ta');
    await page.keyboard.type('Draft');
    await page.keyboard.press('Escape');
    await page.waitForSelector('textarea.desk-ta', { state: 'detached' });
    assert.equal((await page.textContent('#desc')).trim(), 'None');
    assert.equal(await page.evaluate(() => window.saves.length), 0, 'Escape saves nothing');

    await page.focus('#desc');
    await page.keyboard.press('Enter');
    await page.waitForSelector('textarea.desk-ta');
    await page.keyboard.type('Kept');
    await page.focus('#next');
    await page.waitForFunction(() => window.saves.length === 1);
    assert.equal((await page.textContent('#desc')).trim(), 'Kept');

    await page.focus('#desc');
    await page.keyboard.press('Enter');
    await page.waitForSelector('textarea.desk-ta');
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Delete');
    await page.keyboard.press('Control+Enter');
    await page.waitForFunction(() => window.saves.length === 2);
    assert.deepEqual(await page.evaluate(() => window.saves[1].body), { description: '' });
    assert.equal((await page.textContent('#desc')).trim(), 'None');
    assert.equal(await page.evaluate(() => document.getElementById('desc').classList.contains('is-empty')), true);
  } finally { await browser.close(); }
});
