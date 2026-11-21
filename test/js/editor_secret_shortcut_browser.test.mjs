// editor_secret_shortcut_browser.test.mjs -- presses Ctrl+Shift+S in the real
// TipTap bundle and checks the secret mark lands, because the handler's
// `this` only means something against TipTap's real keymap plugin.
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

async function editorPage(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  await p.setContent('<!doctype html><html><body><div id="ed"></div></body></html>');
  await p.addScriptTag({ content: readFileSync(join(root, 'static/vendor/tiptap-bundle.min.js'), 'utf8') });
  await p.addScriptTag({ content: readFileSync(join(root, 'static/js/widgets/editor_secret.js'), 'utf8') });
  // The same extension mix editor.js builds: StarterKit without Underline,
  // Underline on its own, and the secret mark.
  await p.evaluate(() => {
    window.__ed = new TipTap.Editor({
      element: document.getElementById('ed'),
      extensions: [
        TipTap.StarterKit.configure({ link: false, underline: false, codeBlock: false }),
        TipTap.Underline,
        Chronicle.SecretMark,
      ],
      content: '<p>The vizier is the traitor.</p>',
    });
    window.__ed.commands.focus('all');
  });
  // TipTap hands DOM focus over a frame later; a key pressed before then goes to the body, not the editor.
  await p.waitForFunction(() => window.__ed.isFocused && document.activeElement === window.__ed.view.dom);
  return { p, errors };
}

const secretHTML = (p) => p.evaluate(() => window.__ed.getHTML());

test('Ctrl+Shift+S marks the selection secret and a second press clears it', { skip }, async (t) => {
  const { p, errors } = await editorPage(t);
  await p.keyboard.press('Control+Shift+S');
  assert.match(await secretHTML(p), /<span data-secret="true" class="chronicle-secret">The vizier is the traitor\.<\/span>/);
  assert.equal(await p.evaluate(() => window.__ed.isActive('secret')), true);

  await p.keyboard.press('Control+Shift+S');
  assert.doesNotMatch(await secretHTML(p), /data-secret/);
  assert.deepEqual(errors, [], 'the shortcut must not throw');
});

test('the shortcut and the toolbar command make the same mark', { skip }, async (t) => {
  const { p } = await editorPage(t);
  await p.evaluate(() => window.__ed.commands.toggleSecret());
  const viaCommand = await secretHTML(p);
  await p.evaluate(() => window.__ed.commands.toggleSecret());
  await p.keyboard.press('Control+Shift+S');
  assert.equal(await secretHTML(p), viaCommand);
});

test('the ? shortcut list names the secret key', { skip }, async (t) => {
  const { p } = await editorPage(t);
  await p.addScriptTag({ content: readFileSync(join(root, 'static/js/shortcuts_help.js'), 'utf8') });
  await p.evaluate(() => { window.__ed.commands.blur(); document.activeElement.blur(); });
  await p.keyboard.press('?');
  const rows = await p.$$eval('#shortcuts-help kbd', (els) => els.map((e) => e.textContent));
  assert.ok(rows.some((k) => /^(Ctrl|⌘)\+Shift\+S$/.test(k)), 'rows: ' + rows.join(' | '));
});
