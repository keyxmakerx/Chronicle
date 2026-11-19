// editor_link_target.test.mjs — pins what a link in written text is drawn
// with: an address that is not a link (one that runs code) is drawn as a link
// to nowhere, and every link opens a new tab cut off from this one. The
// address alone decides, whatever the saved text says about the link.
//
// The first half runs the rule on its own; the second draws saved text through
// the real editor in Chromium (needs playwright-core, see _browser_env.mjs).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const MENTION_JS = readFileSync(join(root, 'static/js/widgets/editor_mention.js'), 'utf8');
const TIPTAP_JS = readFileSync(join(root, 'static/vendor/tiptap-bundle.min.js'), 'utf8');

function loadLinkKind() {
  const start = MENTION_JS.indexOf('var LINK_PROTOCOLS');
  const end = MENTION_JS.indexOf('\n  }\n', MENTION_JS.indexOf('function linkKind')) + 4;
  assert.ok(start > 0 && end > start, 'linkKind not found');
  return new Function(MENTION_JS.slice(start, end) + '\nreturn linkKind;')();
}

const HERE = 'https://chronicle.test/campaigns/c1/entities/e1';

test('a page of this site is inside, however its address is written', () => {
  const kind = loadLinkKind();
  for (const href of ['/campaigns/c1/entities/e2', '#history', '?tab=posts', 'e2', '../maps/m1',
    'https://chronicle.test/campaigns/c1', 'HTTPS://CHRONICLE.TEST/x', 'https://chronicle.test']) {
    assert.equal(kind(href, HERE), 'inside', href);
  }
});

test('another website is outside, even when written to look like a path', () => {
  const kind = loadLinkKind();
  for (const href of ['https://example.com/', 'http://chronicle.test/x', 'https://chronicle.test.example.com/',
    'https://chronicle.test:8443/x', '//example.com/x', '/\\example.com/x', '/\t/example.com/x', '\\\\example.com',
    'https://chronicle.test@example.com/', 'mailto:dm@example.com', 'tel:+15550100']) {
    assert.equal(kind(href, HERE), 'outside', JSON.stringify(href));
  }
});

test('an address that runs code or is not a link is refused', () => {
  const kind = loadLinkKind();
  for (const href of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', ' javascript:alert(1)', 'java\tscript:alert(1)',
    '\u0001javascript:alert(1)', 'data:text/html,<script>alert(1)</script>', 'vbscript:msgbox(1)', 'file:///etc/passwd',
    'blob:https://chronicle.test/1', 'http://[bad']) {
    assert.equal(kind(href, HERE), 'refused', JSON.stringify(href));
  }
});

test('a link with no address is left as it is', () => {
  const kind = loadLinkKind();
  assert.equal(kind('', HERE), 'outside');
  assert.equal(kind(null, HERE), 'outside');
  assert.equal(kind(undefined, HERE), 'outside');
});

// Saved text as the editor stores it: each link says what it likes about its
// own target and rel, which the drawing must not take on trust.
function savedText(links) {
  return { type: 'doc', content: [{ type: 'paragraph', content: links.map((l) => ({
    type: 'text', text: l.text, marks: [{ type: 'link', attrs: l.attrs }] })) }] };
}

async function draw(t, links, { editable = false } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext();
  const p = await ctx.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await ctx.route('**/*', (r) => r.fulfill({ contentType: 'text/html',
    body: '<!doctype html><html><head><title>' + new URL(r.request().url()).pathname + '</title></head><body><div id="ed"></div></body></html>' }));
  await p.goto(HERE);
  await p.addScriptTag({ content: TIPTAP_JS });
  await p.addScriptTag({ content: MENTION_JS });
  await p.evaluate(({ doc, editable }) => {
    const Link = window.Chronicle.MentionLink;
    window.ed = new window.TipTap.Editor({
      element: document.getElementById('ed'),
      extensions: [window.TipTap.StarterKit.configure({ link: false }), Link.configure({ openOnClick: true, HTMLAttributes: { class: 'text-accent hover:underline' } })],
      editable,
      content: doc,
    });
  }, { doc: savedText(links), editable });
  return { p, ctx };
}
const drawn = (p) => p.$$eval('#ed a', (as) => as.map((a) => ({
  text: a.textContent, href: a.getAttribute('href'), target: a.getAttribute('target'), rel: a.getAttribute('rel') })));

const BLANK = { target: '_blank', rel: 'noopener noreferrer nofollow', class: null };

test('drawn text: a link opens a new tab cut off from this one, whatever it says', { skip }, async (t) => {
  const { p } = await draw(t, [
    { text: 'a rules site', attrs: { href: 'https://example.com/rules', ...BLANK } },
    { text: 'a bare outside link', attrs: { href: 'https://example.com/bare', target: null, rel: null, class: null } },
    { text: 'a named window', attrs: { href: '//example.com/opener', target: 'chronicle', rel: 'opener', class: null } },
  ]);
  for (const out of await drawn(p)) {
    assert.equal(out.target, '_blank', out.text);
    assert.equal(out.rel, 'noopener noreferrer nofollow', out.text);
  }
});

test('drawn text: an address that runs code is drawn as a link to nowhere', { skip }, async (t) => {
  const { p } = await draw(t, [
    { text: 'one', attrs: { href: 'javascript:document.title="ran"', ...BLANK } },
    { text: 'two', attrs: { href: 'javascript:document.title="ran"', target: null, rel: null, class: null } },
    { text: 'three', attrs: { href: ' JaVaScRiPt:document.title="ran"', target: '_self', rel: null, class: null } },
    { text: 'four', attrs: { href: 'data:text/html,<script>opener.document.title="ran"</script>', target: null, rel: null, class: null } },
  ]);
  for (const a of await drawn(p)) {
    assert.equal(a.href, '', a.text);
    assert.equal(a.target, '_blank', a.text);
    assert.equal(a.rel, 'noopener noreferrer nofollow', a.text);
  }
});
