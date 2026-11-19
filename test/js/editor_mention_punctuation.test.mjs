// editor_mention_punctuation.test.mjs — pins what follows a name picked with
// @. The pick leaves a space after the name so the next word can be typed;
// when the next thing typed is a comma, a full stop or another closing mark,
// that space is taken back, so the text reads "@Name," and never "@Name ,".
//
// Runs the real editor and the real picker in Chromium against a stubbed
// search (needs playwright-core, see _browser_env.mjs), wired the way
// editor.js wires them.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const MENTION_JS = readFileSync(join(root, 'static/js/widgets/editor_mention.js'), 'utf8');
// The list is placed by Chronicle.placeAtCaret, which lives in theme.js.
const THEME_JS = readFileSync(join(root, 'static/js/theme.js'), 'utf8');
const TIPTAP_JS = readFileSync(join(root, 'static/vendor/tiptap-bundle.min.js'), 'utf8');

const NBSP = ' ';
const FOUND = { results: [{ id: 'e2', name: 'Thorne', url: '/campaigns/c1/entities/e2', type_name: 'Character', type_color: '#336699' }] };

async function open(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext();
  const p = await ctx.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await ctx.route('**/*', (r) => {
    const u = new URL(r.request().url());
    if (u.pathname.endsWith('/entities/search')) return r.fulfill({ contentType: 'application/json', body: JSON.stringify(FOUND) });
    return r.fulfill({ contentType: 'text/html', body: '<!doctype html><html><body><div id="ed"></div></body></html>' });
  });
  await p.goto('https://chronicle.test/campaigns/c1/entities/e1');
  await p.addScriptTag({ content: TIPTAP_JS });
  await p.evaluate(() => {
    const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
    window.Chronicle = { apiFetch: (url, opts) => fetch(url, opts), escapeHtml: esc, escapeAttr: esc };
  });
  await p.addScriptTag({ content: THEME_JS });
  await p.addScriptTag({ content: MENTION_JS });
  await p.evaluate(() => {
    const ext = window.Chronicle.MentionExtension({ campaignId: 'c1' });
    window.ed = new window.TipTap.Editor({
      element: document.getElementById('ed'),
      extensions: [window.TipTap.StarterKit.configure({ link: false }), window.Chronicle.MentionLink.configure({ openOnClick: false })],
      editable: true,
      content: '<p></p>',
      editorProps: { handleKeyDown: (view, event) => ext.onKeyDown(null, event) },
    });
    ext.onCreate(window.ed);
    window.ed.on('update', () => ext.onUpdate(window.ed));
  });
  await p.click('#ed .ProseMirror');
  return p;
}

// Types "See @Tho" and picks the one page the search returns.
async function pick(p, by = 'Enter') {
  await p.keyboard.type('See @Tho');
  await p.waitForSelector('.mention-popup__item');
  if (by === 'Enter') await p.keyboard.press('Enter');
  else await p.locator('.mention-popup__item').first().dispatchEvent('mousedown');
  await p.waitForFunction(() => document.querySelector('#ed a[data-mention-id="e2"]'));
}
const text = (p) => p.evaluate(() => window.ed.getText());

test('a comma or a full stop typed after a picked name touches the name', { skip }, async (t) => {
  for (const mark of [',', '.']) {
    const p = await open(t);
    await pick(p);
    assert.equal(await text(p), 'See @Thorne' + NBSP, 'the pick leaves a space for the next word');
    await p.keyboard.type(mark);
    assert.equal(await text(p), 'See @Thorne' + mark);
    await p.keyboard.type(' then the tide turned');
    assert.equal(await text(p), 'See @Thorne' + mark + ' then the tide turned');
    // The name is still the link it was, with the mark outside it.
    assert.equal(await p.textContent('#ed a[data-mention-id="e2"]'), '@Thorne');
  }
});

test('the other closing marks do the same, and a name picked with the mouse too', { skip }, async (t) => {
  for (const mark of [';', ':', '!', '?', ')']) {
    const p = await open(t);
    await pick(p, 'mouse');
    await p.keyboard.type(mark);
    assert.equal(await text(p), 'See @Thorne' + mark, mark);
  }
});

test('a word typed after a picked name keeps the space', { skip }, async (t) => {
  for (const next of ['walks', '(aside)', '-', '"so"', "'s"]) {
    const p = await open(t);
    await pick(p);
    await p.keyboard.type(next);
    assert.equal(await text(p), 'See @Thorne' + NBSP + next, next);
  }
});

test('only the very next thing typed can take the space back', { skip }, async (t) => {
  const p = await open(t);
  await pick(p);
  await p.keyboard.type('and');
  await p.keyboard.type(',');
  assert.equal(await text(p), 'See @Thorne' + NBSP + 'and,');

  // A space the writer typed is theirs: a comma after it stays where it is.
  const q = await open(t);
  await q.keyboard.type('See Thorne ,');
  assert.equal(await text(q), 'See Thorne ,');
});

test('one undo gives back the mark and the space together', { skip }, async (t) => {
  const p = await open(t);
  await pick(p);
  // The editor undoes whatever was typed without a pause as one step, so
  // pause: the mark and the space taken back are then a step of their own.
  await p.waitForTimeout(700);
  await p.keyboard.type(',');
  assert.equal(await text(p), 'See @Thorne,');
  await p.keyboard.press('Control+z');
  assert.equal(await text(p), 'See @Thorne' + NBSP);
});
