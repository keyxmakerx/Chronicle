// customize_look_sheet.test.mjs — the Character sheets setting in
// customize_look.js offers exactly the styles the server accepts, is
// covered by Save/Discard/Reset (SEC_KEYS), and travels in the Save payload.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const go = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'appearance.go'), 'utf8');

const a = src.indexOf('var SHEET_STYLES = [');
const a2 = src.indexOf('];', a);
assert.ok(a > 0 && a2 > a, 'SHEET_STYLES marker not found');
const styles = JSON.parse(vm.runInNewContext(src.slice(a, a2 + 2) + ';JSON.stringify(SHEET_STYLES)'));

test('SHEET_STYLES match the server allow-list, in order', () => {
  const m = go.match(/AppearanceSheetStyles\s*=\s*\[\]string\{([^}]*)\}/);
  assert.ok(m, 'AppearanceSheetStyles not found in appearance.go');
  const ids = [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
  assert.deepEqual(styles.map((s) => s[0]), ids);
  assert.equal(ids[0], 'modern');
  styles.forEach((s) => assert.ok(s[1].length > 0, s[0] + ' needs a display name'));
});

test('the sheet style is part of Save, Discard and Reset', () => {
  assert.match(src, /sheet:\['sheet\.style'\]/, 'SEC_KEYS must list sheet.style');
  assert.match(src, /hover:clone\(d\.hover\), sheet:clone\(d\.sheet\)/, 'toInput must send sheet');
  assert.match(src, /if \(!d\.sheet\) d\.sheet = \{ style:'modern' \}/, 'fromServer must default to modern');
  assert.match(src, /data-k="sheet\.style"|'sheet\.style', 'sh-style-l'/, 'a control must bind sheet.style');
});

test('the preview has a character sheet page drawn in the draft style', () => {
  const templ = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'customize_look.templ'), 'utf8');
  assert.match(templ, /name="dpage" value="sheet" id="dp-sheet"/, 'the page picker must offer the sheet');
  assert.match(src, /data-page="sheet" hidden><div class="pv-sheet" id="d-sheet" data-sheet data-sheet-preview/, 'the sample must be a preview sheet root');
  assert.match(src, /sh\.setAttribute\('data-sheet-style', d\.sheet\.style\)/, 'the sample must wear the draft style');
  assert.match(src, /id === 'sheet' && ui\.page !== 'sheet'\) setPage\('sheet'\)/, 'opening Character sheets must show the sheet');
});

// The engine's applyStyle, run against a stub page: the saved pick on <html>
// styles a campaign's sheets, but never the Customize preview's sample.
const engine = readFileSync(join(root, 'static', 'js', 'sheet_motion.js'), 'utf8');
function slice(start, end) {
  const s = engine.indexOf(start);
  const e = engine.indexOf(end, s);
  assert.ok(s > 0 && e > s, `${start} not found`);
  return engine.slice(s, e + end.length);
}
const applyCode = slice('var STYLES = [', ';') + slice('var DEFAULT_STYLE', ';') +
  slice('function styleFor(', '\n  }') + slice('function applyStyle(root)', '\n  }');

function el(attrs) {
  return {
    a: { ...attrs },
    getAttribute(k) { return k in this.a ? this.a[k] : null; },
    setAttribute(k, v) { this.a[k] = String(v); },
    hasAttribute(k) { return k in this.a; },
  };
}
function apply(htmlPick, rootAttrs) {
  const root = el(rootAttrs);
  vm.runInNewContext(applyCode + ';applyStyle(root);', {
    R: el(htmlPick ? { 'data-cz-sheet': htmlPick } : {}), root,
    ensureFonts() {}, bakeOnce() {}, BAKED: {},
  });
  return root.getAttribute('data-sheet-style');
}

test('a campaign sheet takes the saved style; the preview sample keeps the draft', () => {
  assert.equal(apply('ledger', { 'data-sheet': '' }), 'ledger');
  assert.equal(apply(null, { 'data-sheet': '' }), 'modern');
  assert.equal(apply('ledger', { 'data-sheet': '', 'data-sheet-preview': '', 'data-sheet-style': 'neon' }), 'neon');
  assert.equal(apply('ledger', { 'data-sheet': '', 'data-sheet-preview': '', 'data-sheet-style': 'bogus' }), 'modern');
});
