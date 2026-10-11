// editor_diagram.test.mjs — pins the diagram block: what it saves and reads
// back, which sources are refused or cleaned before Mermaid sees them, and
// that the vendored library is requested only when something is drawn.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const SRC = readFileSync(join(root, 'static/js/widgets/editor_diagram.js'), 'utf8');

const same = (a, b) => assert.equal(JSON.stringify(a), JSON.stringify(b));

// A page that records every script it is asked to load. Loading one runs
// onload at once and installs a stand-in Mermaid that records its calls.
function load(opts = {}) {
  const loaded = [];
  const calls = { initialize: [], render: [] };
  const added = [];
  const fakeMermaid = {
    initialize: (c) => calls.initialize.push(c),
    render: async (id, text) => {
      calls.render.push(text);
      if (opts.fail) throw new Error(opts.fail);
      return { svg: '<svg xmlns="http://www.w3.org/2000/svg"></svg>' };
    },
  };
  const svgRoot = {
    nodeName: 'svg',
    localName: 'svg',
    namespaceURI: 'http://www.w3.org/2000/svg',
    attributes: [],
    getElementsByTagName: () => [],
    setAttribute() {},
  };
  const document = {
    currentScript: { getAttribute: (a) => (a === 'data-mermaid-src' ? opts.src || '' : null) },
    head: {
      appendChild(s) {
        loaded.push(s.src);
        window.mermaid = fakeMermaid;
        s.onload();
      },
    },
    createElement: () => ({}),
    getElementById: () => null,
    importNode: (n) => n,
    documentElement: { classList: { contains: (c) => c === 'dark' && !!opts.dark } },
  };
  const window = {
    TipTap: { Node: { create: (spec) => ({ spec }) } },
    Chronicle: { SlashCommands: { addCommand: (c) => added.push(c) } },
  };
  class DOMParser {
    parseFromString() { return { documentElement: svgRoot, getElementsByTagName: () => [] }; }
  }
  vm.runInNewContext(SRC, { window, document, DOMParser, Chronicle: window.Chronicle, TipTap: window.TipTap });
  return { D: window.Chronicle.EditorDiagram, window, loaded, calls, added };
}

test('the node saves only text and a class, and never a GM flag', () => {
  const { D } = load();
  const spec = D.extension.spec;
  assert.equal(spec.name, 'diagram');
  assert.equal(spec.code, true);
  assert.equal(spec.marks, '');
  assert.equal(spec.content, 'text*');
  same(spec.addAttributes(), {});

  const rule = spec.parseHTML()[0];
  assert.equal(rule.tag, 'pre.ce-diagram');
  assert.ok(rule.priority > 50, 'must outrank the plain code block');
  const pre = (cls) => ({ getAttribute: (a) => (a === 'class' ? cls : null) });
  // Old content that carries the GM class loads as a plain diagram, so a
  // re-save cannot bring the class back.
  for (const cls of ['ce-diagram', 'ce-diagram ce-diagram--gm']) same(rule.getAttrs(pre(cls)), {});
  same(spec.renderHTML({ node: { attrs: { gmOnly: true } } }), ['pre', { class: 'ce-diagram' }, ['code', 0]]);
  same(spec.renderHTML({ node: { attrs: {} } }), ['pre', { class: 'ce-diagram' }, ['code', 0]]);
});

test('the slash menu gets /diagram', () => {
  const { added } = load();
  assert.equal(added.length, 1);
  assert.equal(added[0].id, 'diagram');
  assert.match(added[0].keywords, /diagram/);
});

test('the first word decides the kind', () => {
  const { D } = load();
  const cases = [
    ['flowchart LR\nA-->B', 'flowchart'],
    ['\n\n  sequenceDiagram\n A->>B: hi', 'sequenceDiagram'],
    ['%% a comment\ngraph TD\nA-->B', 'graph'],
    ['%%{init: {"theme":"dark"}}%%\nerDiagram\nA ||--o{ B : has', 'erDiagram'],
    ['%%{init: {\n"theme":"dark"\n}}%%\nmindmap\n root', 'mindmap'],
    ['---\ntitle: T\n---\nstateDiagram-v2\n[*]-->A', 'stateDiagram-v2'],
    ['gitGraph:\n commit', 'gitGraph'],
    ['', ''],
  ];
  for (const [src, want] of cases) assert.equal(D._diagramType(src), want, JSON.stringify(src));
});

test('only the eight kinds are drawn, and click lines are dropped', () => {
  const { D } = load();
  for (const ok of ['flowchart TD\nA-->B', 'sequenceDiagram\nA->>B: x', 'classDiagram\nclass A', 'stateDiagram-v2\n[*]-->A',
    'erDiagram\nA ||--o{ B : has', 'mindmap\n root', 'timeline\n title T\n 2000 : x', 'gitGraph\n commit']) {
    assert.equal(D._prepareSource(ok).ok, true, ok);
  }
  for (const bad of ['pie title P\n "a": 1', 'gantt\n title x', 'architecture-beta\n group g(cloud)[G]', 'flowchart-elk TD\nA-->B', 'hello']) {
    const r = D._prepareSource(bad);
    assert.equal(r.ok, false, bad);
    assert.match(r.error, /diagram type/);
  }
  const r = D._prepareSource('flowchart LR\n  A-->B\n  click A call alert("x")\n  click B href "javascript:alert(1)"\n  link-->C');
  assert.equal(r.ok, true);
  assert.doesNotMatch(r.source, /click/);
  assert.match(r.source, /A-->B/);
  assert.match(r.source, /link-->C/, 'a node called link is not a link directive');
  assert.equal(D._prepareSource('classDiagram\n  link A "http://x"').source, 'classDiagram');
});

test('custom CSS, empty text and very long text are refused', () => {
  const { D } = load();
  assert.equal(D._prepareSource('   \n').empty, true);
  assert.match(D._prepareSource('%%{init: {"themeCSS": "body{display:none}"}}%%\nflowchart TD\nA-->B').error, /styles/);
  assert.match(D._prepareSource('flowchart TD\n' + 'A-->B\n'.repeat(5000)).error, /too long/);
});

test('errors are one plain sentence, never Mermaid\'s own text', () => {
  const { D } = load();
  assert.match(D._friendlyError(new Error('Parse error on line 3:\n...A-->\n-----^\nExpecting NODE')), /^Line 3 is not valid/);
  assert.match(D._friendlyError(new Error('No diagram type detected matching given configuration')), /diagram type/);
  const generic = D._friendlyError(new Error('Cannot read properties of undefined'));
  assert.match(generic, /could not be drawn/);
  assert.doesNotMatch(generic, /undefined/);
});

test('lazy load: nothing is requested until a real diagram is drawn', async () => {
  const { D, loaded } = load();
  assert.equal(loaded.length, 0, 'loading the editor script loads nothing');
  // No saved diagram in the HTML: no request.
  assert.equal(D.hydrate({ querySelectorAll: () => [] }), 0);
  assert.equal(loaded.length, 0);
  // Empty, unsupported and too-long text never reach the library.
  await D._draw('');
  await D._draw('pie\n "a": 1');
  assert.equal(loaded.length, 0);
});

test('lazy load: the first real diagram loads the vendored file once', async () => {
  const { D, loaded, calls } = load({ src: '/static/vendor/mermaid.min.js?v=abc123' });
  const a = await D._draw('flowchart LR\nA-->B');
  const b = await D._draw('sequenceDiagram\nA->>B: hi');
  assert.equal(a.ok, true);
  assert.equal(b.ok, true);
  same(loaded, ['/static/vendor/mermaid.min.js?v=abc123']);
  assert.equal(calls.render.length, 2);
});

test('the library is configured strict, without HTML labels', async () => {
  const { D, calls } = load();
  await D._draw('flowchart LR\nA-->B');
  const c = calls.initialize[0];
  assert.equal(c.securityLevel, 'strict');
  assert.equal(c.htmlLabels, false);
  assert.equal(c.flowchart.htmlLabels, false);
  assert.equal(c.startOnLoad, false);
  assert.equal(c.theme, 'default');
});

test('the drawing follows dark mode', async () => {
  const { D, calls } = load({ dark: true });
  await D._draw('flowchart LR\nA-->B');
  assert.equal(calls.initialize[0].theme, 'dark');
});

test('a source Mermaid rejects comes back as a plain message', async () => {
  const { D } = load({ fail: 'Parse error on line 2: boom' });
  const r = await D._draw('flowchart LR\nA-->');
  assert.equal(r.ok, false);
  assert.match(r.error, /^Line 2 is not valid/);
});

test('only the vendored file can be named as the library', () => {
  for (const evil of ['https://cdn.example.com/mermaid.min.js', '/static/vendor/other.js', '//evil/x.js', '/static/vendor/mermaid.min.js?v=a b']) {
    assert.equal(load({ src: evil }).D._src, '/static/vendor/mermaid.min.js', evil);
  }
});

test('directives and front matter are stripped before drawing', () => {
  const { D } = load();
  const cases = [
    ['%%{init: {"theme":"forest","fontFamily":"x"}}%%\nflowchart LR\nA-->B', 'flowchart LR\nA-->B'],
    ['%%{\n init: {\n "fontFamily": "url(https://evil.example/f)"\n }\n}%%\nsequenceDiagram\nA->>B: hi', '\nsequenceDiagram\nA->>B: hi'],
    ['---\ntitle: T\nconfig:\n  fontFamily: "image-set(https://evil.example/i 1x)"\n---\nflowchart LR\nA-->B', 'flowchart LR\nA-->B'],
    ['\n\n%%{init: {"a":1}}%%\n%%{init: {"b":2}}%%\nerDiagram\nA ||--o{ B : has', '\n\n\n\nerDiagram\nA ||--o{ B : has'],
  ];
  for (const [src, want] of cases) {
    const r = D._prepareSource(src);
    assert.equal(r.ok, true, src);
    assert.equal(r.source.replace(/^\s+/, ''), want.replace(/^\s+/, ''));
    assert.doesNotMatch(r.source, /evil|%%\{|fontFamily/);
  }
  assert.equal(D._prepareSource('%%{init: {}}%%').empty, true);
});

test('custom CSS is refused even when written as a JSON escape', () => {
  const { D } = load();
  for (const bad of [
    '%%{init: {"\\u0074hemeCSS": "body{}"}}%%\nflowchart LR\nA-->B',
    '%%{init: {"themeCSS": "body{}"}}%%\nflowchart LR\nA-->B',
    '%%{init: {"fontFamily": "a\\u0075rl(x)"}}%%\nflowchart LR\nA-->B',
    '---\nconfig:\n  themeCSS: "x"\n---\nflowchart LR\nA-->B',
    'flowchart LR\nA-->B\n%% @import url(x)',
  ]) {
    const r = D._prepareSource(bad);
    assert.equal(r.ok, false, bad);
    assert.match(r.error, /styles/, bad);
  }
});

test('the page, not the text, sets theme and font', async () => {
  const { D, calls } = load();
  await D._draw('%%{init: {"theme":"forest","fontFamily":"url(https://evil.example/f)"}}%%\nflowchart LR\nA-->B');
  const c = calls.initialize[0];
  assert.equal(c.theme, 'default');
  assert.equal(typeof c.fontFamily, 'string');
  assert.doesNotMatch(c.fontFamily, /url|evil/);
  assert.equal(c.themeCSS, '');
  assert.doesNotMatch(calls.render[0], /%%|evil/);
});
