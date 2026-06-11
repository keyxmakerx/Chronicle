// widget_listener_leaks.test.mjs — pins the document-listener-leak bug class
// (cordinator#39 findings 1+2). Two widgets had ANONYMOUS document click
// listeners added in their render path; destroy() cleared innerHTML but could
// not remove them, so every HTMX navigation leaked one handler holding a
// detached DOM tree. The contract this guards, for each widget that takes a
// document-level listener:
//   1. No ANONYMOUS document listeners — the handler must be a named/stored
//      reference (so it can be removed).
//   2. Every document.addEventListener(evt, ref) has a matching
//      document.removeEventListener(evt, ref).
//   3. destroy() participates in document-listener cleanup.
// tag_picker.js is included as the canonical-correct control (the pattern the
// fixes mirror) so the guard proves it passes on already-correct code.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const WIDGETS = [
  'static/js/widgets/entity_posts.js',   // finding 1
  'static/js/widgets/relation_graph.js', // finding 2
  'static/js/widgets/tag_picker.js',     // canonical correct pattern (control)
];

// Strip comments so prose mentioning the call names can't trip the guard.
function strip(src) {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');
}
function reEsc(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

for (const rel of WIDGETS) {
  const src = strip(readFileSync(join(root, rel), 'utf8'));

  test(`${rel}: no anonymous document listeners (unremovable → leak)`, () => {
    assert.doesNotMatch(src, /document\.addEventListener\(\s*['"][^'"]+['"]\s*,\s*function/,
      'a document listener must be a stored/named reference, never an inline function');
  });

  test(`${rel}: every document listener is removable + removed`, () => {
    const adds = [...src.matchAll(/document\.addEventListener\(\s*(['"][^'"]+['"])\s*,\s*([A-Za-z_$][\w.$]*)\s*\)/g)];
    assert.ok(adds.length > 0, 'expected at least one document.addEventListener to guard');
    for (const [, evt, ref] of adds) {
      const re = new RegExp('document\\.removeEventListener\\(\\s*' + reEsc(evt) + '\\s*,\\s*' + reEsc(ref) + '\\s*\\)');
      assert.match(src, re, `${ref} added on ${evt} must have a matching document.removeEventListener`);
    }
  });

  test(`${rel}: destroy() removes document listeners`, () => {
    const i = src.indexOf('destroy:');
    assert.ok(i >= 0, 'widget must expose a destroy()');
    assert.match(src.slice(i), /document\.removeEventListener/,
      'destroy() must remove the widget’s document listener(s)');
  });
}
