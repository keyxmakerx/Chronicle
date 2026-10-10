/**
 * editor_diagram.js -- Mermaid diagrams inside editor text.
 *
 * Exposes Chronicle.EditorDiagram:
 *   .extension      TipTap node "diagram" (a block), inserted with /diagram
 *   .insert(ed)     put a starter diagram at the cursor
 *   .hydrate(root)  draw every saved diagram found in already-rendered HTML
 *                   (notes list, posts), for places that show HTML without TipTap
 *   .createView(host, opts)  a drawing surface: set(source), destroy()
 *
 * Saved HTML is <pre class="ce-diagram [ce-diagram--gm]"><code>MERMAID
 * TEXT</code></pre>. Only the text is stored, never the drawing, and it is
 * escaped like any code block, so the shared sanitizer keeps it as text and
 * a non-JS reader sees readable source. ce-diagram--gm marks a GM-only
 * diagram; the server drops it for players in sanitize.StripSecretsHTML and
 * StripSecretsJSON.
 *
 * Security: Mermaid is a vendored, version-pinned file served by Chronicle
 * (static/vendor/mermaid.min.js), loaded only when a diagram is first drawn,
 * run with securityLevel 'strict' and HTML labels off. Click and link
 * directives are removed from the text before it is drawn, and the SVG it
 * returns is cleaned again before it is put in the page.
 */
(function () {
  'use strict';

  window.Chronicle = window.Chronicle || {};

  // The page hands over the library's versioned URL; only that one file is
  // ever loaded from it.
  var SELF = document.currentScript;
  var SRC_RE = /^\/static\/vendor\/mermaid\.min\.js(\?v=[A-Za-z0-9._-]{1,80})?$/;
  var RAW_SRC = (SELF && SELF.getAttribute && SELF.getAttribute('data-mermaid-src')) || '';
  var MERMAID_SRC = SRC_RE.test(RAW_SRC) ? RAW_SRC : '/static/vendor/mermaid.min.js';

  // The kinds writers can draw. Anything else (pie, gantt, architecture with
  // its remote icon packs, ...) is refused with a plain message.
  var TYPES = ['flowchart', 'graph', 'sequenceDiagram', 'classDiagram', 'classDiagram-v2',
    'stateDiagram', 'stateDiagram-v2', 'erDiagram', 'mindmap', 'timeline', 'gitGraph'];
  var MAX_SOURCE = 20000;
  var RENDER_DELAY = 400;

  var STARTER = 'flowchart LR\n  A[Start] --> B{Choice}\n  B -->|Yes| C[Do it]\n  B -->|No| D[Skip it]';

  var MSG_EMPTY = 'Nothing to draw yet. Write a diagram in the text.';
  var MSG_TYPE = 'Start the text with a diagram type: flowchart, sequenceDiagram, classDiagram, stateDiagram-v2, erDiagram, mindmap, timeline or gitGraph.';
  var MSG_BAD = 'This diagram could not be drawn. Check the text above.';
  var MSG_LONG = 'This diagram is too long to draw.';
  var MSG_CSS = 'Custom styles are not allowed in diagrams.';
  var MSG_READER = 'This diagram could not be drawn.';
  var MSG_LOAD = 'The diagram tool could not be loaded.';

  // --- Reading the text ----------------------------------------------------

  /* The first real word of the text: after a front-matter block, blank lines
     and %% comments or %%{...}%% directives. '' when there is none. */
  function diagramType(source) {
    var lines = String(source == null ? '' : source).replace(/\r\n?/g, '\n').split('\n');
    var i = 0;
    while (i < lines.length && !lines[i].trim()) i++;
    if (i < lines.length && lines[i].trim() === '---') {
      i++;
      while (i < lines.length && lines[i].trim() !== '---') i++;
      i++;
    }
    for (; i < lines.length; i++) {
      var t = lines[i].trim();
      if (!t) continue;
      if (t.indexOf('%%{') === 0) {
        while (t.indexOf('}%%') < 0 && i + 1 < lines.length) { i++; t = lines[i].trim(); }
        continue;
      }
      if (t.indexOf('%%') === 0) continue;
      var m = /^([A-Za-z][\w-]*)/.exec(t);
      return m ? m[1] : '';
    }
    return '';
  }

  // click / link / callback lines bind actions to nodes. Strict mode already
  // ignores them; they are dropped here so nothing depends on that alone.
  var ACTION_LINE = /^\s*(click|link|links|callback|callbacks)\s+["\w]/i;

  function prepareSource(source) {
    var text = String(source == null ? '' : source).replace(/\r\n?/g, '\n');
    if (!text.trim()) return { ok: false, empty: true, error: MSG_EMPTY };
    if (text.length > MAX_SOURCE) return { ok: false, error: MSG_LONG };
    if (/themeCSS/i.test(text) || /@import/i.test(text)) return { ok: false, error: MSG_CSS };
    if (TYPES.indexOf(diagramType(text)) < 0) return { ok: false, error: MSG_TYPE };
    var kept = text.split('\n').filter(function (l) { return !ACTION_LINE.test(l); });
    return { ok: true, source: kept.join('\n') };
  }

  /* One plain sentence for a failed draw; Mermaid's own message (with its
     ASCII pointer and token lists) is never shown. */
  function friendlyError(err) {
    var msg = String((err && err.message) || err || '');
    var m = /Parse error on line (\d+)/i.exec(msg) || /line (\d+)/i.exec(msg);
    if (m) return 'Line ' + m[1] + ' is not valid. Check it and the drawing will come back.';
    if (/No diagram type|UnknownDiagram/i.test(msg)) return MSG_TYPE;
    if (/Maximum|exceeded|too many/i.test(msg)) return MSG_LONG;
    return MSG_BAD;
  }

  // --- Cleaning the drawing -------------------------------------------------

  var DROP_TAGS = /^(script|iframe|object|embed|foreignobject|audio|video|canvas|link|meta|base|form|input|button|textarea|image)$/i;

  /* Parse Mermaid's SVG text and return an <svg> element with only drawing
     parts left: no scripts, no foreign content, no event handlers, no links
     out (a link is unwrapped, leaving its drawing), no remote references. */
  function cleanSvg(svgText, doc) {
    doc = doc || document;
    var parsed = new DOMParser().parseFromString(String(svgText || ''), 'image/svg+xml');
    var root = parsed.documentElement;
    if (!root || root.nodeName.toLowerCase() !== 'svg' || parsed.getElementsByTagName('parsererror').length) return null;
    var all = Array.prototype.slice.call(root.getElementsByTagName('*'));
    all.forEach(function (el) {
      if (!el.parentNode) return;
      var tag = (el.localName || el.nodeName).toLowerCase();
      if (DROP_TAGS.test(tag)) { el.parentNode.removeChild(el); return; }
      if (tag === 'a') {
        while (el.firstChild) el.parentNode.insertBefore(el.firstChild, el);
        el.parentNode.removeChild(el);
        return;
      }
      if (tag === 'style' && /@import|expression\s*\(|url\(\s*["']?\s*(?!#)/i.test(el.textContent || '')) {
        el.parentNode.removeChild(el);
        return;
      }
      Array.prototype.slice.call(el.attributes).forEach(function (a) {
        var n = a.name.toLowerCase();
        var v = String(a.value || '');
        if (n.indexOf('on') === 0) { el.removeAttribute(a.name); return; }
        if ((n === 'href' || n === 'xlink:href' || n === 'src') && v.charAt(0) !== '#') { el.removeAttribute(a.name); return; }
        if (/javascript:|data:text\/html|expression\s*\(/i.test(v)) el.removeAttribute(a.name);
      });
    });
    root.setAttribute('role', 'img');
    root.setAttribute('aria-label', 'Diagram');
    return doc.importNode ? doc.importNode(root, true) : root;
  }

  // --- Loading Mermaid ------------------------------------------------------

  var loadPromise = null;
  function loadMermaid() {
    if (window.mermaid) return Promise.resolve(window.mermaid);
    if (loadPromise) return loadPromise;
    loadPromise = new Promise(function (resolve, reject) {
      var s = document.createElement('script');
      s.src = MERMAID_SRC;
      s.async = true;
      s.onload = function () { window.mermaid ? resolve(window.mermaid) : reject(new Error('mermaid missing')); };
      s.onerror = function () { loadPromise = null; reject(new Error('mermaid failed to load')); };
      document.head.appendChild(s);
    });
    return loadPromise;
  }

  function isDark() {
    var h = document.documentElement;
    return !!(h && h.classList && h.classList.contains('dark'));
  }

  var counter = 0;
  var queue = Promise.resolve();

  /* Draw one source. Resolves {ok:true, svg} or {ok:false, error, empty?}.
     Draws run one at a time: Mermaid keeps global state while it renders. */
  function draw(source) {
    var prep = prepareSource(source);
    if (!prep.ok) return Promise.resolve(prep);
    var job = queue.then(function () {
      return loadMermaid().then(function (m) {
        var id = 'ce-dg-' + (++counter);
        m.initialize({
          startOnLoad: false,
          securityLevel: 'strict',
          theme: isDark() ? 'dark' : 'default',
          htmlLabels: false,
          flowchart: { htmlLabels: false, useMaxWidth: true },
          class: { htmlLabels: false },
          state: { htmlLabels: false },
          maxTextSize: MAX_SOURCE,
          maxEdges: 300,
          suppressErrorRendering: true,
          logLevel: 5
        });
        return Promise.resolve(m.render(id, prep.source)).then(function (out) {
          cleanupStray(id);
          var svg = cleanSvg(out && out.svg);
          return svg ? { ok: true, svg: svg } : { ok: false, error: MSG_BAD };
        }, function (err) {
          cleanupStray(id);
          return { ok: false, error: friendlyError(err) };
        });
      }, function () { return { ok: false, error: MSG_LOAD }; });
    });
    queue = job.then(function () {}, function () {});
    return job;
  }

  // Mermaid leaves a scratch element in <body> when a draw fails.
  function cleanupStray(id) {
    ['d' + id, id].forEach(function (x) {
      var el = document.getElementById(x);
      if (el && el.parentNode) el.parentNode.removeChild(el);
    });
  }

  // --- Drawing surface ------------------------------------------------------

  var live = [];
  var watching = false;
  function watchTheme() {
    if (watching || typeof MutationObserver === 'undefined') return;
    watching = true;
    new MutationObserver(function () {
      live.slice().forEach(function (v) { v.redraw(); });
    }).observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
  }

  /* A surface inside `host`. set(source) draws it; the last good drawing
     stays while a newer text is broken. opts.onState(error|null, empty) hears
     about each result. Follows the light/dark theme by redrawing. */
  function createView(host, opts) {
    opts = opts || {};
    var seq = 0;
    var source = '';
    var dark = isDark();
    var view = {
      set: function (src) {
        source = String(src == null ? '' : src);
        var mine = ++seq;
        return draw(source).then(function (res) {
          if (mine !== seq) return;
          if (res.ok) {
            host.textContent = '';
            host.appendChild(res.svg);
            host.classList.add('has-drawing');
            if (opts.onState) opts.onState(null, false);
          } else {
            if (res.empty) { host.textContent = ''; host.classList.remove('has-drawing'); }
            if (opts.onState) opts.onState(res.error, !!res.empty);
          }
        });
      },
      redraw: function () {
        var now = isDark();
        if (now === dark) return;
        dark = now;
        if (source.trim()) view.set(source);
      },
      destroy: function () {
        seq++;
        var i = live.indexOf(view);
        if (i >= 0) live.splice(i, 1);
      }
    };
    live.push(view);
    watchTheme();
    return view;
  }

  // --- Saved HTML (no TipTap) ----------------------------------------------

  /* Replace each saved diagram in `root` with its drawing. Mermaid is not
     requested when there is none. A diagram that cannot be drawn says so in
     one plain line. */
  function hydrate(root) {
    if (!root || !root.querySelectorAll) return 0;
    var pres = root.querySelectorAll('pre.ce-diagram');
    var n = 0;
    Array.prototype.forEach.call(pres, function (pre) {
      if (pre.getAttribute('data-ce-done')) return;
      pre.setAttribute('data-ce-done', '1');
      var src = pre.textContent || '';
      var wrap = document.createElement('div');
      wrap.className = 'ce-diagram ce-diagram--static';
      var view = document.createElement('div');
      view.className = 'ce-diagram__view';
      var err = document.createElement('p');
      err.className = 'ce-diagram__error';
      err.hidden = true;
      wrap.appendChild(view);
      wrap.appendChild(err);
      pre.parentNode.replaceChild(wrap, pre);
      n++;
      createView(view, {
        onState: function (e, empty) {
          if (empty) { wrap.hidden = true; return; }
          err.hidden = !e;
          err.textContent = e ? MSG_READER : '';
        }
      }).set(src);
    });
    return n;
  }

  // --- The editor node -------------------------------------------------------

  function classFor(attrs) {
    return 'ce-diagram' + (attrs && attrs.gmOnly ? ' ce-diagram--gm' : '');
  }
  function attrsFromPre(el) {
    var cls = ' ' + (el.getAttribute('class') || '') + ' ';
    return { gmOnly: cls.indexOf(' ce-diagram--gm ') >= 0 };
  }

  function createNodeView(props) {
    var node = props.node, editor = props.editor, getPos = props.getPos;

    var dom = document.createElement('div');
    dom.className = classFor(node.attrs);

    var bar = document.createElement('div');
    bar.className = 'ce-diagram__bar';
    bar.setAttribute('contenteditable', 'false');
    var label = document.createElement('span');
    label.className = 'ce-diagram__label';
    label.innerHTML = '<i class="fa-solid fa-diagram-project" aria-hidden="true"></i> Diagram';
    bar.appendChild(label);
    var gmBtn = document.createElement('button');
    gmBtn.type = 'button';
    gmBtn.className = 'ce-diagram__btn';
    gmBtn.innerHTML = '<i class="fa-solid fa-eye-slash" aria-hidden="true"></i> GM only';
    gmBtn.title = 'Hide this diagram from players';
    bar.appendChild(gmBtn);
    var delBtn = document.createElement('button');
    delBtn.type = 'button';
    delBtn.className = 'ce-diagram__btn';
    delBtn.innerHTML = '<i class="fa-solid fa-trash" aria-hidden="true"></i> Remove';
    delBtn.title = 'Remove this diagram';
    bar.appendChild(delBtn);
    dom.appendChild(bar);

    var grid = document.createElement('div');
    grid.className = 'ce-diagram__grid';
    dom.appendChild(grid);

    var textCol = document.createElement('div');
    textCol.className = 'ce-diagram__text';
    var src = document.createElement('pre');
    src.className = 'ce-diagram__src';
    src.setAttribute('spellcheck', 'false');
    textCol.appendChild(src);
    var err = document.createElement('p');
    err.className = 'ce-diagram__error';
    err.setAttribute('contenteditable', 'false');
    err.setAttribute('role', 'status');
    err.hidden = true;
    textCol.appendChild(err);
    grid.appendChild(textCol);

    var viewCol = document.createElement('div');
    viewCol.className = 'ce-diagram__view';
    viewCol.setAttribute('contenteditable', 'false');
    grid.appendChild(viewCol);

    var timer = null;
    var view = createView(viewCol, {
      onState: function (e, empty) {
        // Writers read the message under the text; readers get a one-line
        // note in the drawing's place (CSS), never the message itself.
        err.hidden = !e;
        err.textContent = e || '';
        dom.classList.toggle('is-broken', !!e);
        dom.classList.toggle('is-empty', !!empty);
      }
    });

    function setAttrs(patch) {
      if (typeof getPos !== 'function') return;
      var pos = getPos();
      if (typeof pos !== 'number') return;
      var next = {};
      for (var k in node.attrs) next[k] = node.attrs[k];
      for (var p in patch) next[p] = patch[p];
      editor.view.dispatch(editor.view.state.tr.setNodeMarkup(pos, undefined, next));
    }
    function paint() {
      dom.className = classFor(node.attrs) + (dom.classList.contains('is-broken') ? ' is-broken' : '') + (dom.classList.contains('is-empty') ? ' is-empty' : '');
      gmBtn.classList.toggle('is-on', !!node.attrs.gmOnly);
      gmBtn.setAttribute('aria-pressed', node.attrs.gmOnly ? 'true' : 'false');
      // Notes editors flag themselves after they are built; players have no GM-only.
      gmBtn.style.display = editor.chronicleNotePictures ? 'none' : '';
    }
    gmBtn.addEventListener('mousedown', function (e) { e.preventDefault(); });
    gmBtn.addEventListener('click', function () { setAttrs({ gmOnly: !node.attrs.gmOnly }); });
    delBtn.addEventListener('mousedown', function (e) { e.preventDefault(); });
    delBtn.addEventListener('click', function () {
      var pos = getPos();
      if (typeof pos !== 'number') return;
      editor.view.dispatch(editor.view.state.tr.delete(pos, pos + node.nodeSize));
      editor.commands.focus();
    });

    var last = null;
    function schedule(immediate) {
      var text = node.textContent;
      if (text === last) return;
      last = text;
      clearTimeout(timer);
      if (immediate) view.set(text);
      else timer = setTimeout(function () { view.set(text); }, RENDER_DELAY);
    }
    paint();
    schedule(true);

    return {
      dom: dom,
      contentDOM: src,
      update: function (updated) {
        if (updated.type !== node.type) return false;
        node = updated;
        paint();
        schedule(false);
        return true;
      },
      // The bar, the error line and the drawing are not document content.
      stopEvent: function (ev) {
        var t = ev && ev.target;
        return !!(t && t.closest && t.closest('.ce-diagram__bar, .ce-diagram__view, .ce-diagram__error'));
      },
      ignoreMutation: function (m) {
        if (m.type === 'selection') return false;
        return !src.contains(m.target);
      },
      destroy: function () { clearTimeout(timer); view.destroy(); }
    };
  }

  if (window.TipTap && TipTap.Node) {
    var DiagramNode = TipTap.Node.create({
      name: 'diagram',
      group: 'block',
      content: 'text*',
      marks: '',
      code: true,
      defining: true,
      isolating: true,
      addAttributes: function () {
        return { gmOnly: { default: false, rendered: false } };
      },
      // Ahead of the plain code block, which also claims <pre>.
      parseHTML: function () {
        return [{ tag: 'pre.ce-diagram', priority: 1000, preserveWhitespace: 'full', getAttrs: attrsFromPre }];
      },
      renderHTML: function (p) {
        return ['pre', { class: classFor(p.node.attrs) }, ['code', 0]];
      },
      addNodeView: function () { return createNodeView; },
      addKeyboardShortcuts: function () {
        // Enter already adds a line (the node is code). These two leave it:
        // Ctrl/Cmd+Enter anywhere, and arrow-down from the end of the last block.
        return {
          'Mod-Enter': function (ctx) {
            var ed = (ctx && ctx.editor) || this.editor;
            if (ed.state.selection.$from.parent.type.name !== 'diagram') return false;
            return ed.commands.exitCode();
          },
          ArrowDown: function (ctx) {
            var ed = (ctx && ctx.editor) || this.editor;
            var sel = ed.state.selection;
            var $from = sel.$from;
            if (!sel.empty || $from.parent.type.name !== 'diagram') return false;
            if ($from.parentOffset !== $from.parent.content.size) return false;
            if ($from.after() < ed.state.doc.content.size) return false;
            return ed.commands.exitCode();
          }
        };
      }
    });

    // "/diagram" in the slash menu (loaded before this file).
    if (Chronicle.SlashCommands && Chronicle.SlashCommands.addCommand) {
      Chronicle.SlashCommands.addCommand({
        id: 'diagram', label: 'Diagram', icon: 'fa-diagram-project',
        keywords: 'diagram mermaid flowchart sequence chart graph class state er mindmap timeline git',
        description: 'Draw a flowchart, sequence, class, state, ER, mindmap, timeline or git diagram',
        customHandler: function (ed) { insert(ed); }
      });
    }
    Chronicle.EditorDiagram = { extension: DiagramNode };
  } else {
    Chronicle.EditorDiagram = {};
  }

  function insert(ed) {
    if (!ed) return false;
    return ed.chain().focus().insertContent({
      type: 'diagram',
      attrs: { gmOnly: false },
      content: [{ type: 'text', text: STARTER }]
    }).run();
  }

  var api = Chronicle.EditorDiagram;
  api.insert = insert;
  api.hydrate = hydrate;
  api.createView = createView;
  // Exposed for tests.
  api._diagramType = diagramType;
  api._prepareSource = prepareSource;
  api._friendlyError = friendlyError;
  api._cleanSvg = cleanSvg;
  api._attrsFromPre = attrsFromPre;
  api._classFor = classFor;
  api._loadMermaid = loadMermaid;
  api._draw = draw;
  api._src = MERMAID_SRC;
})();
