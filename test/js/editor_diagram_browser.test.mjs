// editor_diagram_browser.test.mjs — runs the diagram script in a real
// Chromium (real DOMParser, the real vendored Mermaid) because cleanSvg and
// the directive stripping only mean something against the real thing.
//
// Needs playwright-core and a Chromium. It looks in the usual places
// (NODE_PATH, /opt/node-tools, PW_CHROMIUM or /opt/pw-browsers) and skips,
// saying so, when none is found.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const FIX = join(root, 'test/js/fixtures/mermaid');

function findPlaywright() {
  for (const base of [import.meta.url, '/opt/node-tools/', process.env.NODE_PATH ? process.env.NODE_PATH + '/' : null]) {
    if (!base) continue;
    try {
      const req = createRequire(base.startsWith('file:') || base.startsWith('/') ? (base.startsWith('/') ? base + 'x.js' : base) : base);
      return req('playwright-core');
    } catch (e) { /* try the next place */ }
  }
  return null;
}
function findChromium() {
  if (process.env.PW_CHROMIUM && existsSync(process.env.PW_CHROMIUM)) return process.env.PW_CHROMIUM;
  try {
    for (const d of readdirSync('/opt/pw-browsers')) {
      const c = join('/opt/pw-browsers', d, 'chrome-linux', 'chrome');
      if (d.startsWith('chromium-') && existsSync(c)) return c;
    }
  } catch (e) { /* none */ }
  return null;
}

const pw = findPlaywright();
const chromePath = findChromium();
const skip = !pw || !chromePath ? 'playwright-core or Chromium not found' : false;

const ALLOWED = ('svg g path rect circle ellipse line polyline polygon text tspan defs marker style title desc ' +
  'lineargradient radialgradient stop clippath use filter fedropshadow fegaussianblur feoffset femerge femergenode ' +
  'feflood fecomposite feblend fecolormatrix symbol').split(' ');

async function page(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const evil = [];
  await p.route(/evil\.example/, (r) => { evil.push(r.request().url()); r.abort(); });
  await p.setContent('<!doctype html><html><head></head><body></body></html>');
  await p.addScriptTag({ content: readFileSync(join(root, 'static/vendor/mermaid.min.js'), 'utf8') });
  await p.addScriptTag({ content: readFileSync(join(root, 'static/js/widgets/editor_diagram.js'), 'utf8') });
  return { p, evil };
}

test('cleanSvg keeps real Mermaid drawings whole and only uses allowed parts', { skip }, async (t) => {
  const { p } = await page(t);
  const files = readdirSync(FIX).filter((f) => f.endsWith('.svg'));
  assert.ok(files.length >= 8, 'a saved drawing for each kind');
  for (const f of files) {
    const raw = readFileSync(join(FIX, f), 'utf8');
    const r = await p.evaluate((raw) => {
      const D = Chronicle.EditorDiagram;
      const doc = new DOMParser().parseFromString(raw, 'image/svg+xml');
      const clean = D._cleanSvg(raw);
      const count = (n, sel) => n.querySelectorAll(sel).length;
      return {
        tags: [...new Set([...clean.querySelectorAll('*')].map((e) => e.localName))],
        counts: ['path', 'rect', 'text', 'g', 'circle', 'polygon', 'marker'].map((s) => [s, count(doc, s), count(clean, s)]),
        styleKept: count(clean, 'style') === count(doc, 'style'),
        bad: [...clean.querySelectorAll('*')].flatMap((e) => [...e.attributes].filter((a) => /^on/i.test(a.name) || /url\(\s*["']?\s*(?!#)|image-set|@import|javascript:|\\/i.test(a.value)).map((a) => a.name)),
        role: clean.getAttribute('role'),
      };
    }, raw);
    for (const t of r.tags) assert.ok(ALLOWED.includes(t.toLowerCase()), f + ': unexpected element ' + t);
    for (const [name, before, after] of r.counts) assert.equal(after, before, f + ': ' + name + ' count changed');
    assert.equal(r.styleKept, true, f + ': Mermaid\'s own style was dropped');
    assert.deepEqual(r.bad, [], f);
    assert.equal(r.role, 'img');
  }
});

test('cleanSvg removes everything outside the allow-lists', { skip }, async (t) => {
  const { p, evil } = await page(t);
  const r = await p.evaluate(async () => {
    const D = Chronicle.EditorDiagram;
    const evilSvg = '<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" onload="window.__pwn=1" viewBox="0 0 10 10">' +
      '<script>window.__pwn=2</script>' +
      '<style>@import url(https://evil.example/a.css);</style>' +
      '<style>.a{background:image-set("https://evil.example/b.png" 1x)}</style>' +
      '<style>.a{background:\\75rl(https://evil.example/c.png)}</style>' +
      '<style>.ok{fill:url(#grad)}</style>' +
      '<foreignObject><div xmlns="http://www.w3.org/1999/xhtml"><img src="https://evil.example/d.png"/></div></foreignObject>' +
      '<image href="https://evil.example/e.png"/>' +
      '<a href="https://evil.example/f"><rect id="inA" width="5" height="5"/></a>' +
      '<use href="https://evil.example/g#x"/><use id="u1" href="#inA"/>' +
      '<rect id="r1" width="5" height="5" style="fill:url(https://evil.example/h.png)" onclick="window.__pwn=3" fill="url(#grad)"/>' +
      '<rect id="r2" style="background:image-set(url(https://evil.example/i.png) 1x)"/>' +
      '<rect id="r3" style="fill:\\75rl(https://evil.example/j.png)"/>' +
      '<rect id="r4" style="@import \'https://evil.example/k.css\'"/>' +
      '<rect id="r5" style="width:expression(alert(1))"/>' +
      '<rect id="r6" data-id="keep" aria-label="keep" foo="bar" srcset="https://evil.example/l.png"/>' +
      '<animate attributeName="href" to="javascript:alert(1)"/>' +
      '<set attributeName="onclick" to="alert(1)"/>' +
      '<text id="t1">hi</text></svg>';
    const clean = D._cleanSvg(evilSvg);
    const holder = document.createElement('div');
    holder.appendChild(clean);
    document.body.appendChild(holder);
    await new Promise((res) => setTimeout(res, 500));
    const ser = new XMLSerializer().serializeToString(clean);
    const has = (id) => !!clean.querySelector('#' + id);
    return {
      pwn: window.__pwn || null,
      ser,
      tags: [...new Set([...clean.querySelectorAll('*')].map((e) => e.localName))],
      inA: has('inA'), u1Href: clean.querySelector('#u1') && clean.querySelector('#u1').getAttribute('href'),
      r1: { style: clean.querySelector('#r1').getAttribute('style'), onclick: clean.querySelector('#r1').getAttribute('onclick'), fill: clean.querySelector('#r1').getAttribute('fill') },
      r2: clean.querySelector('#r2').getAttribute('style'), r3: clean.querySelector('#r3').getAttribute('style'),
      r4: clean.querySelector('#r4').getAttribute('style'), r5: clean.querySelector('#r5').getAttribute('style'),
      r6: ['data-id', 'aria-label', 'foo', 'srcset'].map((a) => clean.querySelector('#r6').getAttribute(a)),
      styles: [...clean.querySelectorAll('style')].map((s) => s.textContent),
      onload: clean.getAttribute('onload'),
    };
  });
  assert.equal(r.pwn, null);
  assert.doesNotMatch(r.ser, /evil\.example|script|foreignObject|<image|<animate|<set|onload|onclick|srcset|expression|image-set|\\75/);
  for (const t of r.tags) assert.ok(ALLOWED.includes(t.toLowerCase()), 'unexpected element ' + t);
  assert.equal(r.inA, true, 'a link is unwrapped, its drawing kept');
  assert.equal(r.u1Href, '#inA', 'an internal use reference stays');
  assert.equal(r.r1.style, null);
  assert.equal(r.r1.onclick, null);
  assert.equal(r.r1.fill, 'url(#grad)', 'an internal fragment reference stays');
  assert.deepEqual([r.r2, r.r3, r.r4, r.r5], [null, null, null, null]);
  assert.deepEqual(r.r6, ['keep', 'keep', null, null]);
  assert.deepEqual(r.styles, ['.ok{fill:url(#grad)}']);
  assert.deepEqual(evil, [], 'nothing left for evil.example');
});

test('config in the text cannot reach Mermaid, and nothing leaves for evil.example', { skip }, async (t) => {
  const { p, evil } = await page(t);
  const hostile = {
    jsonEscapedThemeCSS: '%%{init: {"\\u0074hemeCSS": "body{background:url(https://evil.example/a.png)}"}}%%\nflowchart LR\nA-->B',
    plainThemeCSS: '%%{init: {"themeCSS": "body{background:url(https://evil.example/b.png)}"}}%%\nflowchart LR\nA-->B',
    fontFamilyUrl: '%%{init: {"themeVariables": {"fontFamily": "x;background-image:url(https://evil.example/c.png)"}}}%%\nflowchart LR\nA-->B',
    frontMatter: '---\nconfig:\n  themeCSS: "body{background:url(https://evil.example/d.png)}"\n  fontFamily: "a;background:url(https://evil.example/e.png)"\n---\nflowchart LR\nA-->B',
    multilineDirective: '%%{\n init: {\n "theme": "base", "fontFamily": "url(https://evil.example/f.png)"\n }\n}%%\nflowchart LR\nA-->B',
    directiveAfterBlank: '\n\n%%{init: {"fontFamily": "image-set(https://evil.example/n.png 1x)"}}%%\n%%{init: {"theme":"forest"}}%%\nsequenceDiagram\nA->>B: hi',
    nestedDirective: '%%{%%{init: {"fontFamily":"url(https://evil.example/o.png)"}}%%init: {}}%%\nflowchart LR\nA-->B',
    classDefImageSet: 'flowchart LR\nA-->B\nclassDef bad fill:#fff,background-image:image-set("https://evil.example/g.png" 1x),stroke:red\nclass A bad',
    classDefUrl: 'flowchart LR\nA-->B\nstyle A fill:url(https://evil.example/h.png),stroke:red',
    linkStyleUrl: 'flowchart LR\nA-->B\nlinkStyle 0 stroke:url(https://evil.example/j.png)',
    clickHref: 'flowchart LR\nA-->B\nclick A href "https://evil.example/k" _blank\nclick B call alert(1)',
    htmlLabel: 'flowchart LR\nA["<img src=https://evil.example/l.png onerror=alert(1)>x"]-->B["<a href=https://evil.example/m>y</a>"]',
  };
  const out = await p.evaluate(async (hostile) => {
    const D = Chronicle.EditorDiagram;
    const res = {};
    for (const [k, src] of Object.entries(hostile)) {
      const prep = D._prepareSource(src);
      const d = await D._draw(src);
      let ser = '';
      if (d.ok) { const h = document.createElement('div'); h.appendChild(d.svg); document.body.appendChild(h); ser = new XMLSerializer().serializeToString(d.svg); }
      res[k] = { prepared: prep.ok ? prep.source : null, refused: prep.ok ? null : prep.error, ok: d.ok, leaks: /evil\.example|image-set|themeCSS/.test(ser) };
    }
    await new Promise((r) => setTimeout(r, 1500));
    return res;
  }, hostile);
  assert.match(out.jsonEscapedThemeCSS.refused, /styles/);
  assert.match(out.plainThemeCSS.refused, /styles/);
  assert.match(out.frontMatter.refused, /styles/);
  // A directive nested in another cannot be stripped into a valid text, so it is refused.
  assert.equal(out.nestedDirective.ok, false);
  for (const k of ['fontFamilyUrl', 'multilineDirective', 'directiveAfterBlank']) {
    assert.equal(out[k].refused, null, k);
    assert.doesNotMatch(out[k].prepared, /%%|evil\.example|fontFamily/, k + ': directive left in the text');
  }
  for (const [k, v] of Object.entries(out)) assert.equal(v.leaks, false, k + ' leaked into the drawing');
  assert.deepEqual(evil, [], 'nothing left for evil.example');
});

test('a diagram marked GM-only is never drawn by hydrate', { skip }, async (t) => {
  const { p } = await page(t);
  const r = await p.evaluate(async () => {
    const d = document.createElement('div');
    d.innerHTML = '<pre class="ce-diagram ce-diagram--gm"><code>flowchart LR\nSecret--&gt;Base</code></pre><pre class="ce-diagram"><code>flowchart LR\nOpen--&gt;Map</code></pre>';
    document.body.appendChild(d);
    const n = Chronicle.EditorDiagram.hydrate(d);
    await new Promise((res) => setTimeout(res, 1500));
    return { n, svgs: d.querySelectorAll('svg').length, gmPre: d.querySelectorAll('pre.ce-diagram--gm').length, text: d.querySelector('svg') ? d.querySelector('svg').textContent : '' };
  });
  assert.equal(r.n, 1);
  assert.equal(r.svgs, 1);
  assert.equal(r.gmPre, 1);
  assert.doesNotMatch(r.text, /Secret/);
});

test('hydrated views are released when their element leaves the page', { skip }, async (t) => {
  const { p } = await page(t);
  const r = await p.evaluate(async () => {
    const D = Chronicle.EditorDiagram;
    const d = document.createElement('div');
    d.innerHTML = '<pre class="ce-diagram"><code>flowchart LR\nA--&gt;B</code></pre>';
    document.body.appendChild(d);
    D.hydrate(d);
    await new Promise((res) => setTimeout(res, 1200));
    const before = D._live();
    d.remove();
    document.dispatchEvent(new Event('htmx:afterSwap'));
    return { before, after: D._live() };
  });
  assert.equal(r.before, 1);
  assert.equal(r.after, 0);
});
