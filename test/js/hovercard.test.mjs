// hovercard.test.mjs — contracts for the one hover card (static/js/hovercard.js):
// every caller's text is escaped, only safe links are followed, and the look
// falls back to Paper when the campaign has not chosen one (data-cz-hover,
// written by layouts.AppearanceAttrs).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'hovercard.js'), 'utf8');

/** Loads hovercard.js with <html data-cz-hover=attr>; returns Chronicle.hovercard. */
function load(attr) {
  const noop = () => {};
  const document = {
    addEventListener: noop,
    documentElement: { getAttribute: (k) => (k === 'data-cz-hover' ? attr ?? null : null) },
  };
  const window = { addEventListener: noop };
  const sandbox = { window, document };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return window.Chronicle.hovercard;
}

test('look: Paper unless the campaign picked another known look', () => {
  assert.equal(load(undefined).look(), 'paper');
  assert.equal(load('night').look(), 'night');
  assert.equal(load('compact').look(), 'compact');
  assert.equal(load('plain').look(), 'plain');
  assert.equal(load('bogus').look(), 'paper');
});

test('html escapes every field a caller passes', () => {
  const h = load().html({
    kind: '<b>NPC</b>', title: '"Varrin" <script>', text: 'a & b',
    facts: [['<i>Role</i>', 'Watch <captain>']], foot: '<hr>', pic: '/x.png" onerror="alert(1)',
  });
  assert.ok(!h.includes('<script>'), h);
  assert.ok(!h.includes('<b>NPC'), h);
  assert.ok(!h.includes('<i>Role'), h);
  assert.ok(!h.includes('<hr>'), h);
  assert.ok(!h.includes('" onerror'), h);
  assert.ok(h.includes('&quot;Varrin&quot; &lt;script&gt;'), h);
  assert.ok(h.includes('a &amp; b'), h);
});

test('links: same-site paths and http(s) only', () => {
  const hc = load();
  const link = (href) => hc.html({ title: 'T', link: { href, label: 'Open' } });
  assert.ok(link('/campaigns/1/entities/2').includes('href="/campaigns/1/entities/2"'));
  assert.ok(link('https://example.com/a').includes('href="https://example.com/a"'));
  assert.ok(!link('javascript:alert(1)').includes('href='));
  assert.ok(!link('//evil.example').includes('href='));
  assert.ok(!link('data:text/html,x').includes('href='));
});

test('a card with no link or foot has no footer', () => {
  const h = load().html({ kind: 'Rule', title: 'Edge', text: '+2 to a power roll.' });
  assert.ok(!h.includes('chc__foot'), h);
  assert.ok(h.includes('chc__title'), h);
});
