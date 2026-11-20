// entry_told_browser.test.mjs — the "Changed recently" mark on a hero's
// entry link. The mark sits right after the link, never inside it: a sheet
// reads and animates the name's own text (the Draw Steel sheet's pick
// animation reads the textContent of the element holding the name). The
// link is described by the mark's words, and a sheet that draws the name
// again never ends up with two marks, nor keeps one for a name not told.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, f), 'utf8');

const link = (name) => {
  const href = '/campaigns/c1/systems/drawsteel/named/ancestry?name=' + encodeURIComponent(name) + '&hero=h1';
  return '<a class="sh-idname" href="' + href + '" data-entry-peek="' + href + '" data-v="origin-ancestry">' + name + '</a>';
};

test('the mark sits beside the name, once, and only while told', { skip }, async (t) => {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  await p.setContent('<!doctype html><html><body><span class="sh-idval" id="v">' + link('Ashborn') + ' <button>Change</button></span></body></html>');
  await p.evaluate(() => {
    window.asked = [];
    window.Chronicle = {
      apiFetch: function (url) {
        window.asked.push(url);
        var told = url.indexOf('Ashborn') >= 0 ? ['ancestry:Ashborn'] : [];
        return Promise.resolve({ ok: true, json: function () { return Promise.resolve({ told: told }); } });
      },
    };
  });
  await p.addScriptTag({ content: read('static/js/peek_address.js') });
  await p.addScriptTag({ content: read('static/js/entry_told.js') });
  const state = () => p.evaluate(() => {
    const a = document.querySelector('a[data-entry-peek]');
    const marks = document.querySelectorAll('.entry-told-mark');
    const words = document.querySelector('.entry-told-sr');
    return {
      name: a.textContent,
      marks: marks.length,
      beside: marks.length === 1 && a.nextElementSibling === marks[0],
      describedBy: a.getAttribute('aria-describedby') && words ? a.getAttribute('aria-describedby') === words.id : false,
      words: words ? words.textContent : '',
    };
  });
  await p.waitForFunction(() => document.querySelector('.entry-told-mark'));
  assert.deepEqual(await state(), { name: 'Ashborn', marks: 1, beside: true, describedBy: true, words: 'Changed recently' });

  // The sheet draws its identity again, as after a pick.
  await p.evaluate((html) => { document.getElementById('v').innerHTML = html + ' <button>Change</button>'; }, link('Ashborn'));
  await p.waitForTimeout(200);
  assert.deepEqual(await state(), { name: 'Ashborn', marks: 1, beside: true, describedBy: true, words: 'Changed recently' });

  // Only the link is swapped, leaving the old mark behind it.
  await p.evaluate((html) => {
    const a = document.querySelector('a[data-entry-peek]');
    a.insertAdjacentHTML('afterend', html);
    a.remove();
  }, link('Ashborn'));
  await p.waitForTimeout(200);
  assert.equal((await state()).marks, 1, 'a kept mark is reused, never doubled');

  // A name that was not told keeps no mark.
  await p.evaluate((html) => {
    const a = document.querySelector('a[data-entry-peek]');
    a.insertAdjacentHTML('afterend', html);
    a.remove();
  }, link('Dwarf'));
  await p.waitForFunction(() => window.asked.some((u) => u.indexOf('Dwarf') >= 0));
  await p.waitForTimeout(200);
  const s = await state();
  assert.equal(s.marks, 0);
  assert.equal(s.name, 'Dwarf');
  // One question per new set of names.
  assert.equal((await p.evaluate(() => window.asked.length)), 2);
  assert.deepEqual(errors, []);
});
