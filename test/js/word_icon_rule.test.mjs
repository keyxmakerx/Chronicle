// word_icon_rule.test.mjs: an icon that repeats a button's word is drawn only
// when the campaign asks for icons everywhere, and the sidebar never takes
// the mark, because its icons are landmarks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(root, p), 'utf8');

test('a word icon is hidden unless the campaign shows icons everywhere', () => {
  const css = read('static/css/input.css').replace(/\s+/g, ' ');
  assert.ok(css.includes(':root:not([data-cz-icons="all"]) .word-ic { display: none; }'));
});

test('the sidebar keeps its icons at every step', () => {
  assert.ok(!read('internal/templates/layouts/nav.templ').includes('word-ic'));
});

test('the Customizer says what Everywhere does', () => {
  assert.ok(read('static/js/widgets/customize_look.js').includes('Everywhere also keeps the icon on buttons and menu rows that have a word.'));
});

// The icon font's sheet loads after the site's and gives .fa-solid a display
// of its own, so a hiding rule only as strong as one class loses to it.
test('a kind icon stays hidden while icons are Few, with the icon font loaded after', { skip }, async (t) => {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const site = read('static/css/input.css');
  const font = '.fa-solid{display:var(--fa-display,inline-block)}';
  const shown = async (icons) => {
    await p.setContent(`<!doctype html><html data-cz-icons="${icons}"><style>${site}</style><style>${font}</style><body><a><i class="fa-solid fa-user kind-ic"></i>Bren</a></body></html>`);
    return p.$eval('.kind-ic', (el) => getComputedStyle(el).display);
  };
  assert.equal(await shown('few'), 'none');
  assert.equal(await shown('names'), 'inline-block');
});
