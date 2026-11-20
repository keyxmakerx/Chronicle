// rulebook_editor_entry_browser.test.mjs — a card tied to its rules entry
// (entry: ancestries/human) keeps the tie through a house edit. The editor's
// form has no field for the tie, so a save that rebuilt the card from the
// form alone would drop it, and the Handbook would then show players the
// printed card again.
//
// Runs the editor in a real Chromium with the server stubbed. Needs
// playwright-core and a Chromium (see _browser_env.mjs); skips locally, and
// fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const EDITOR = readFileSync(join(root, 'static/js/widgets/rulebook_editor.js'), 'utf8');

const SOURCE = {
  title: 'Rulebook', systemName: 'Draw Steel',
  parts: [{ title: 'Heroes', chapters: [{ id: 'ancestries', title: 'Ancestries', pages: [{
    key: 'p0', state: 'package',
    page: { title: 'Peoples', blocks: [
      { type: 'cards', items: [
        { title: 'Human', summary: 'Everywhere.', text: 'Printed words.', entry: 'ancestries/human' },
        { title: 'A card about nothing', summary: 'Stays.' },
      ] },
      { type: 'flaps', items: [{ title: 'A flap', text: 'Open me.' }] },
    ] },
  }] }] }],
};

test('a tied card keeps its entry when its text is edited and saved', { skip }, async (t) => {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  await p.setContent('<!doctype html><html><body><div id="ed"></div></body></html>');
  await p.evaluate((source) => {
    window.puts = [];
    window.Chronicle = {
      register: function (name, def) { window.def = def; },
      escapeHtml: function (s) {
        return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
      },
      apiFetch: function (url, opts) {
        var method = (opts && opts.method) || 'GET';
        var body = method === 'GET' ? source : { entry: { key: 'p0', state: 'edited' } };
        if (method === 'PUT') window.puts.push({ url: url, body: JSON.parse(JSON.stringify(opts.body)) });
        return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(body); } });
      },
    };
  }, SOURCE);
  await p.addScriptTag({ content: EDITOR });
  await p.evaluate(() => window.def.init(document.getElementById('ed'), { sourceUrl: '/campaigns/c1/systems/ds/book/source' }));

  const field = p.locator('[data-path="blocks.0.items.0.text"]');
  await field.waitFor();
  await field.fill('Printed words, with a typo fixed.');
  await p.waitForFunction(() => window.puts.length > 0, null, { timeout: 5000 });

  const saved = await p.evaluate(() => window.puts[window.puts.length - 1]);
  assert.equal(saved.url, '/campaigns/c1/systems/ds/book/chapters/ancestries/pages/p0');
  const [cards, flaps] = saved.body.page.blocks;
  assert.deepEqual(cards.items[0], { title: 'Human', summary: 'Everywhere.', text: 'Printed words, with a typo fixed.', entry: 'ancestries/human' });
  assert.equal('entry' in cards.items[1], false, 'an untied card stays untied');
  // The book allows entry on cards only, so a flap never carries one.
  assert.equal('entry' in flaps.items[0], false);
  assert.deepEqual(errors, []);
});
