// customize_look_email.test.mjs — the Emails section in customize_look.js
// offers exactly the layouts the server and internal/email draw, is covered
// by Save, Discard and Reset (SEC_KEYS), travels in the Save payload, and
// sends its test with the draft's layout and colour.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const mail = readFileSync(join(root, 'internal', 'email', 'email.go'), 'utf8');
const css = readFileSync(join(root, 'static', 'css', 'customize.css'), 'utf8');

const a = src.indexOf('var EMAIL_LAYOUTS = [');
const a2 = src.indexOf('];', a);
assert.ok(a > 0 && a2 > a, 'EMAIL_LAYOUTS marker not found');
const layouts = JSON.parse(vm.runInNewContext(src.slice(a, a2 + 2) + ';JSON.stringify(EMAIL_LAYOUTS)'));

test('EMAIL_LAYOUTS match internal/email\'s layouts, in order, Note first', () => {
  const m = mail.match(/var Layouts = \[\]Layout\{([^}]*)\}/);
  assert.ok(m, 'Layouts not found in internal/email/email.go');
  const names = m[1].split(',').map((x) => x.trim());
  const ids = names.map((n) => mail.match(new RegExp('\\b' + n + '\\s+Layout\\s*=\\s*"([^"]+)"'))[1]);
  assert.deepEqual(layouts.map((l) => l[0]), ids);
  assert.deepEqual(layouts.map((l) => l[1]), ['Note', 'Poster', 'Ticket', 'Chat']);
});

test('Emails sits in Pages and sheets and is part of Save, Discard and Reset', () => {
  assert.match(src, /\{ id:'email', name:'Emails', icon:'i-mail'/, 'SECTIONS must list Emails');
  assert.match(src, /email:\['email\.layout'\]/, 'SEC_KEYS must list email.layout');
  assert.match(src, /email:clone\(d\.email\)/, 'toInput must send the email layout');
  assert.match(src, /if \(!d\.email \|\| !d\.email\.layout\) d\.email = \{ layout:'note' \};/, 'fromServer must fill a missing layout as Note');
  assert.match(src, /data-k="email\.layout"/, 'a control must bind email.layout');
  assert.match(src, /id=\\"cz-i-mail\\"/, 'the sprite must carry the mail icon');
  assert.match(src, /if \(sec === 'brand' \|\| sec === 'email'\) return 'Back to what is saved';/, 'Reset puts the layout back to what is saved');
});

test('Send me a test posts the draft layout and colour and reports where it went', () => {
  assert.match(src, /'\/appearance\/email-test', \{ method:'POST', body:\{ layout:draft\.email\.layout, accent:draft\.colours\.accent \}/);
  assert.match(src, /case 'em-test-btn': sendTestEmail\(t\); break;/);
  assert.match(src, /toast\('Test sent to ' \+ \(r\.b\.to \|\| 'your email address'\)/);
  assert.match(src, /err\.hidden = false; err\.textContent = r\.b\.message \|\| r\.b\.error/, 'a refused test says why next to the button');
});

test('the tiles take the colour the email would, falling back the same way', () => {
  const b = src.indexOf('function emailAccent(');
  const fn = src.slice(b, src.indexOf('\n', b));
  const lum = src.slice(src.indexOf('    function hexToRgb('), src.indexOf('    function toLab('));
  const ctx = { clamp: (v, lo, hi) => Math.max(lo, Math.min(hi, v)) };
  vm.runInNewContext(lum + fn, ctx);
  assert.equal(ctx.emailAccent('#2563eb'), '#2563eb');
  assert.equal(ctx.emailAccent('#fde68a'), '#4f46e5', 'too light to read on white');
  assert.equal(ctx.emailAccent('red'), '#4f46e5');
  assert.match(mail, /DefaultAccent = "#4f46e5"/, 'the fallback matches internal/email');
  for (const id of ['note', 'poster', 'ticket', 'chat']) assert.match(css, new RegExp('\\.czp \\.em-' + id + '\\{'), id + ' tile is drawn');
});
