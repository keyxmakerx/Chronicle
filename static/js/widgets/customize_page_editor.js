/**
 * customize_page_editor.js: the Customizer's page layout, a category's Page
 * layout tab (entity_type_customize_tab.templ).
 *
 * It draws the category's page layout on one of the category's own pages:
 * each widget's content is that page's, drawn by the server
 * (POST .../layout-preview) and shown inert, so nothing here can open, edit
 * or save the page itself. The owner adds, moves and sizes widgets in
 * sections of columns or of tiles (internal/tilegrid), and every change goes
 * into the one Customize draft as the category's `layout`, through
 * Chronicle.customizeCategoryWrite (customize_categories.js). Save applies it
 * with the checks any layout save runs.
 *
 * Old container widgets (two columns, tabs, section) stay one unit with their
 * contents. Pages cannot draw them, so the editor offers to unpack them.
 */
(function () {
  'use strict';

  var COL_PRESETS = [[12], [8, 4], [4, 8], [6, 6], [4, 4, 4]];
  var PRESET_NAMES = { '12': 'One column', '8,4': 'Wide and narrow', '4,8': 'Narrow and wide', '6,6': 'Two even', '4,4,4': 'Three even' };
  var SECTION_KINDS = [
    ['tiles', 'Tiles', 'Widgets on a grid in set sizes', 'fa-table-cells'],
    ['12', 'One column', 'Full width, for long text', 'fa-square'],
    ['8,4', 'Wide and narrow', 'Story with facts beside it', 'fa-table-columns'],
    ['6,6', 'Two even', 'Side by side', 'fa-table-columns'],
    ['4,4,4', 'Three even', 'Short things in a row', 'fa-table-columns']
  ];
  var HEIGHTS = [['', 'Fit'], ['sm', 'Short'], ['md', 'Medium'], ['lg', 'Tall'], ['xl', 'Extra tall']];
  var HEIGHT_PX = { sm: 150, md: 300, lg: 500, xl: 700 };
  // The page's width on each screen: the real page's widest, a tablet, a phone.
  var FRAME_W = { desktop: 1180, tablet: 780, phone: 0 };
  // The server's limits (ValidateLayout, tilegrid), checked here so a change
  // Save would refuse is refused at once, with the reason.
  var MAX_ROWS = 20, MAX_BLOCKS = 10, MAX_TILES = 40, MAX_BYTES = 100 * 1024;
  // Drag timing: how long the pointer
  // rests before the gap moves, how long widgets take to glide, and how far
  // the pointer must go before the gap moves again.
  var SETTLE = 110, GLIDE = 340, NUDGE = 12;
  var GROUPS = ['The page', 'Links and lists', 'Maps and time', 'Characters and play', 'From packages', 'More'];
  var BASICS = ['title', 'image', 'entry', 'attributes', 'details', 'tags', 'divider', 'text_block', 'heading', 'callout'];
  // Where an old container keeps its widgets, so they can be listed and unpacked.
  var CONTAINER_KEYS = { two_column: ['left', 'right'], three_column: ['columns'], tabs: ['tabs'], section: ['blocks'] };

  function parse(s, dflt) {
    try { var v = JSON.parse(s || ''); return v == null ? dflt : v; } catch (e) { return dflt; }
  }
  function clone(v) { return v == null ? v : JSON.parse(JSON.stringify(v)); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function fa(cls) { return /^fa-[a-z0-9-]+$/.test(String(cls || '')) ? cls : 'fa-cube'; }
  function ic(cls) { return '<i class="fa-solid ' + fa(cls) + '" aria-hidden="true"></i>'; }
  function humanize(t) { t = String(t || '').replace(/_/g, ' '); return t.charAt(0).toUpperCase() + t.slice(1); }
  var seq = 0;
  function uid(p) { seq++; return p + '-' + Date.now().toString(36) + seq.toString(36) + Math.random().toString(36).slice(2, 6); }
  // stable is JSON with sorted keys, so two equal settings give one key.
  function stable(v) {
    if (Array.isArray(v)) return '[' + v.map(stable).join(',') + ']';
    if (v && typeof v === 'object') return '{' + Object.keys(v).sort().map(function (k) { return JSON.stringify(k) + ':' + stable(v[k]); }).join(',') + '}';
    return JSON.stringify(v);
  }
  function sizeOf(s) { var p = String(s || '1x1').split('x'); return { w: Number(p[0]) || 1, h: Number(p[1]) || 1 }; }

  // canon is a layout in the shape the server stores it, so the draft can be
  // compared with what is saved and sent as is.
  function canon(L) {
    function blk(b) {
      var o = { id: String(b.id), type: String(b.type) };
      if (b.config && typeof b.config === 'object' && Object.keys(b.config).length) o.config = b.config;
      return o;
    }
    var rows = (L && Array.isArray(L.rows) ? L.rows : []).map(function (r) {
      if (r.kind === 'tiles') {
        return { id: String(r.id), kind: 'tiles', columns: [], tiles: (r.tiles || []).map(function (t) { var o = blk(t); o.size = String(t.size); return o; }) };
      }
      return { id: String(r.id), columns: (r.columns || []).map(function (c) {
        return { id: String(c.id), width: Number(c.width) || 12, blocks: (c.blocks || []).map(blk) };
      }) };
    });
    return { rows: rows };
  }
  function colsRow(widths, blocks) {
    return { id: uid('r'), columns: widths.map(function (w, i) { return { id: uid('c'), width: w, blocks: (blocks && blocks[i]) || [] }; }) };
  }
  function tilesRow(tiles) { return { id: uid('r'), kind: 'tiles', columns: [], tiles: tiles || [] }; }
  function reId(r) {
    r.id = uid('r');
    if (r.kind === 'tiles') (r.tiles || []).forEach(function (t) { t.id = uid('t'); });
    else (r.columns || []).forEach(function (c) { c.id = uid('c'); (c.blocks || []).forEach(function (b) { b.id = uid('b'); }); });
  }
  function isDm(item) { return !!(item.config && item.config.visibility === 'dm_only'); }
  function cfgOf(item) { if (!item.config || typeof item.config !== 'object') item.config = {}; return item.config; }

  // containerKids lists the widgets an old container holds, in page order.
  function containerKids(b) {
    var c = b.config || {}, out = [];
    function add(list) { (Array.isArray(list) ? list : []).forEach(function (k) { if (k && k.type) out.push(k); }); }
    if (b.type === 'two_column') { add(c.left); add(c.right); }
    else if (b.type === 'three_column') (Array.isArray(c.columns) ? c.columns : []).forEach(add);
    else if (b.type === 'tabs') (Array.isArray(c.tabs) ? c.tabs : []).forEach(function (t) { add(t && t.blocks); });
    else if (b.type === 'section') add(c.blocks);
    return out;
  }

  // inertFrom turns a block's HTML from the server into a node that shows but
  // does nothing: no scripts, styles, frames, handlers, widget mounts, htmx
  // or Alpine attributes, or ids that could collide with the editor's page.
  function inertFrom(html) {
    var doc = new DOMParser().parseFromString('<div>' + String(html || '') + '</div>', 'text/html');
    var root = doc.body.firstElementChild;
    if (!root) return null;
    Array.prototype.slice.call(root.querySelectorAll('script, style, link, iframe, object, embed, base, meta, template, noscript')).forEach(function (n) { n.remove(); });
    Array.prototype.slice.call(root.querySelectorAll('*')).forEach(function (n) {
      Array.prototype.slice.call(n.attributes).forEach(function (a) {
        var k = a.name.toLowerCase();
        if (k.indexOf('on') === 0 || k.indexOf('hx-') === 0 || k.indexOf('data-hx') === 0 || k.indexOf('x-') === 0 ||
            k.charAt(0) === '@' || k.charAt(0) === ':' || k === 'id' || k === 'autofocus' || k === 'data-widget' ||
            k === 'contenteditable' || k === 'tabindex' || k === 'for' || k === 'form' || k === 'formaction' || k === 'action') {
          n.removeAttribute(a.name);
        }
        if ((k === 'href' || k === 'src') && /^\s*javascript:/i.test(a.value)) n.removeAttribute(a.name);
      });
    });
    var shows = root.textContent.replace(/\s+/g, '') !== '' || !!root.querySelector('img, svg, canvas, video, picture');
    return shows ? document.importNode(root, true) : null;
  }

  /* ---------- Example content ----------
   * Until a category has a page to draw with, or where the chosen page has
   * nothing for a widget, the widget shows an example of what it holds, so
   * the owner arranges a page that looks like a page. Every string here is
   * fixed text; nothing from a page or the layout goes into it. */
  function exHead(icon, name) { return '<div class="cpe-ex-h cb-head">' + ic(icon) + '<span>' + name + '</span></div>'; }
  function exRows(rows) { return '<dl class="cpe-ex-dl">' + rows.map(function (r) { return '<dt>' + r[0] + '</dt><dd>' + r[1] + '</dd>'; }).join('') + '</dl>'; }
  function exList(items) {
    return '<div class="cpe-ex-list">' + items.map(function (x) {
      return '<div class="cpe-ex-li"><span class="cpe-ex-av" style="background:' + x[2] + '">' + x[0].charAt(0) + '</span><b>' + x[0] + '</b><span>' + x[1] + '</span></div>';
    }).join('') + '</div>';
  }
  var EX_SCENE = '<svg viewBox="0 0 320 180" preserveAspectRatio="xMidYMid slice" aria-hidden="true"><path d="M0 120 L60 70 L110 105 L170 45 L230 95 L280 60 L320 85 V180 H0Z" fill="rgb(255 255 255 / 0.16)"/><path d="M0 150 L80 115 L150 140 L220 110 L320 135 V180 H0Z" fill="rgb(0 0 0 / 0.25)"/><circle cx="252" cy="38" r="16" fill="rgb(255 236 179 / 0.85)"/></svg>';
  var EX_MAP = '<svg viewBox="0 0 320 180" preserveAspectRatio="xMidYMid slice" aria-hidden="true"><rect width="320" height="180" fill="#e8dfc4"/><path d="M0 120 C60 100 90 140 160 118 S260 90 320 104" stroke="#8fb3c9" stroke-width="10" fill="none"/><circle cx="160" cy="70" r="34" fill="none" stroke="#5b5146" stroke-width="5"/><rect x="148" y="58" width="24" height="24" fill="#fff" stroke="#5b5146" stroke-width="2"/><circle cx="96" cy="44" r="7" fill="#6366f1"/><circle cx="236" cy="132" r="7" fill="#6366f1"/><circle cx="160" cy="104" r="7" fill="#dc2626"/></svg>';
  var EX = {
    title: function () { return '<div class="cpe-ex-title cb-title">Saltreach</div><div class="cpe-ex-sub"><span class="cpe-ex-chip">Location</span><span>Port city on the Grey Coast</span></div>'; },
    image: function () { return '<div class="cpe-ex-img">' + EX_SCENE + '</div>'; },
    cover_image: function () { return '<div class="cpe-ex-img cpe-ex-cover">' + EX_SCENE + '</div>'; },
    entry: function (w, h) {
      return exHead('fa-align-left', 'Story') + '<p class="cpe-ex-p">Saltreach clings to the cliffs above the harbour, its streets stacked like shelves. Every ship that comes in pays the tide-toll, and the Harbour Watch decides who pays twice.</p>' +
        (h > 1 || w > 2 ? '<p class="cpe-ex-p">Under the old lighthouse, the smugglers keep a door the Watch pretends not to see.</p>' : '');
    },
    attributes: function () { return exHead('fa-list', 'Attributes') + exRows([['Population', '12,400'], ['Ruler', 'Harbourmaster Vell'], ['Danger', 'Moderate']]); },
    details: function () { return exHead('fa-info-circle', 'Details') + exRows([['Created', '3 days ago'], ['Updated', 'Today'], ['Category', 'Locations']]); },
    tags: function () { return '<div class="cpe-ex-tags"><span>coast</span><span>trade</span><span>smugglers</span><span>session 4</span></div>'; },
    relations: function () { return exHead('fa-link', 'Relations') + exList([['Harbourmaster Vell', 'rules', '#6366f1'], ['The Tide Guild', 'based here', '#0d9488'], ['Brother Halvard', 'was born here', '#b45309']]); },
    children: function (w) {
      var t = [['Trial 1', 'The Siege'], ['Trial 2', 'The Riddle Tavern'], ['Trial 3', 'The Drowned Bell']].slice(0, w > 2 ? 3 : 2);
      return exHead('fa-sitemap', 'Sub-pages') + '<div class="cpe-ex-book">' + t.map(function (x) { return '<div><span>' + x[0] + '</span><b>' + x[1] + '</b></div>'; }).join('') + '</div>';
    },
    posts: function () { return exHead('fa-layer-group', 'Posts') + '<div class="cpe-ex-post"><b>The toll riots</b><span>Three summers ago the Watch doubled the toll overnight…</span></div><div class="cpe-ex-post"><b>Rumours at the docks</b><span>A ship came in with no crew and a full hold.</span></div>'; },
    backlinks: function () { return exHead('fa-arrow-turn-down', 'Mentioned in') + exList([['Session 4 notes', 'yesterday', '#64748b'], ['The Tide Guild', 'page', '#0d9488']]); },
    writing_prompts: function () { return exHead('fa-feather', 'Prompts') + '<ul class="cpe-ex-ul"><li>Who keeps the lighthouse now?</li><li>What does the toll pay for?</li></ul>'; },
    read_aloud: function () { return exHead('fa-bullhorn', 'Read aloud') + '<blockquote class="cpe-ex-read">Salt wind and gull cries. Below you, a hundred masts sway in the harbour, and every one of them flies the Watch’s grey flag.</blockquote>'; },
    page_steps: function (w, h) {
      if (w * h === 1) return exHead('fa-list-ol', 'Steps') + '<div class="cpe-ex-ring"><b>1 of 3</b><span>played</span></div>';
      return exHead('fa-list-ol', 'Steps') + '<ol class="cpe-ex-steps"><li class="done"><i>✓</i>Arrive at the toll gate</li><li><i>2</i>Meet the harbourmaster</li><li><i>3</i>Find the smugglers’ door</li></ol>';
    },
    page_table: function () {
      return exHead('fa-table', 'Table') + '<table class="cpe-ex-tbl"><thead><tr><th>Who</th><th>Wants</th><th class="dm">Secret</th></tr></thead><tbody>' +
        '<tr><td>Harbourmaster Vell</td><td>The toll paid</td><td class="dm">Takes smugglers’ coin</td></tr><tr><td>Old Maudry</td><td>Her son home</td><td class="dm">He’s in the hold</td></tr></tbody></table>';
    },
    page_encounters: function () { return exHead('fa-dragon', 'Encounters') + exList([['Watch patrol', 'Medium · 4 foes', '#dc2626'], ['Dock rats', 'Easy · swarm', '#64748b']]); },
    where_it_is: function () { return exHead('fa-map-location-dot', 'Where it is') + exList([['Common room', 'you are here', '#6366f1'], ['The Gilded Flagon', 'one step out', '#0d9488'], ['Port Varrow', 'the city', '#b45309']]); },
    last_seen: function () { return exHead('fa-eye', 'Last seen') + '<div class="cpe-ex-sub"><b>The Drowned Bell</b><span>5 weeks ago</span><span class="cpe-ex-chip">6 nights</span></div>'; },
    contents: function () { return exHead('fa-list-ul', 'Contents') + '<div class="cpe-ex-sub"><b>History</b><span>The Docks</span><span>Who rules</span><span class="cpe-ex-chip">+4</span></div>'; },
    standing: function () { return exHead('fa-scale-balanced', 'Standing') + '<div class="cpe-ex-sub"><b>Wary</b><span>Why: Brawled in the guildhall and 2 more</span></div>'; },
    rumours: function () { return exHead('fa-ear-listen', 'Rumours') + exList([['She sank her own ship for the insurance.', 'Old Bram · Swears by it', '#64748b'], ['Dessa Quill took the ship’s treasure.', 'Mara · Heard it', '#64748b']]); },
    family_tree: function () { return exHead('fa-sitemap', 'Family tree') + exList([['Ione Fenwick', 'parent', '#6366f1'], ['Maud Fenwick', 'you are here', '#0d9488'], ['Tam Ash', 'child', '#b45309']]); },
    divider: function () { return '<hr class="cpe-ex-hr">'; },
    map_full: function () { return '<div class="cpe-ex-img cpe-ex-map">' + EX_MAP + '</div>'; },
    local_graph: function () { return '<div class="cpe-ex-img cpe-ex-map">' + EX_MAP + '</div>'; },
    calendar_preview: function () { return exHead('fa-calendar-days', 'Calendar') + exList([['Festival of Tides', 'in 3 days', '#6366f1'], ['Toll day', 'next week', '#b45309']]); },
    timeline_preview: function () { return exHead('fa-timeline', 'Timeline') + exList([['The lighthouse falls', '412 AS', '#64748b'], ['The Watch is founded', '430 AS', '#6366f1']]); },
    entity_notes: function () { return exHead('fa-sticky-note', 'Player notes') + '<p class="cpe-ex-p">“Don’t trust the harbourmaster.” (Ilse)</p>'; },
    text_block: function () { return '<p class="cpe-ex-p">Your own text goes here, the same on every page.</p>'; }
  };
  // The examples that are fact panels, which keep their edge in Annals.
  var EX_FACTS = ['attributes', 'details'];
  // exampleNode builds the example for a widget at a tile's size (columns
  // count as wide), or a quiet skeleton for widgets with no example.
  function exampleNode(type, name, icon, w, h) {
    var f = Object.prototype.hasOwnProperty.call(EX, type) ? EX[type] : null;
    var box = document.createElement('div');
    // Examples mark the shared block parts as real blocks do, so the page
    // looks (Annals) draw them as they draw a page.
    box.className = 'cpe-ex cb-card' + (EX_FACTS.indexOf(type) >= 0 ? ' cb-facts' : '');
    if (f) { box.innerHTML = f(w || 4, h || 2); return box; }
    // The name and icon come from the canvas's attributes, so DOM calls only.
    var head = document.createElement('div');
    head.className = 'cpe-ex-h cb-head';
    var i = document.createElement('i');
    i.className = 'fa-solid ' + fa(icon);
    i.setAttribute('aria-hidden', 'true');
    var s = document.createElement('span');
    s.textContent = name;
    head.appendChild(i);
    head.appendChild(s);
    var sk = document.createElement('div');
    sk.className = 'cpe-ex-skel';
    for (var k = 0; k < 3; k++) sk.appendChild(document.createElement('i'));
    box.appendChild(head);
    box.appendChild(sk);
    return box;
  }

  function groupOf(m) {
    var t = m.type;
    if (m.widgetSlug || t === 'ext_widget') return 'From packages';
    if (BASICS.indexOf(t) >= 0) return 'The page';
    if (/map|calendar|timeline|era|weather|world/.test(t)) return 'Maps and time';
    if (/character|inventory|item|claim|party|shop|quest|stash|creature|npc/.test(t)) return 'Characters and play';
    if (/relation|backlink|sub_?page|child|list|post|note|link|grid|feed|tag/.test(t)) return 'Links and lists';
    return 'More';
  }

  // lookNow is the page look the campaign's pages use now: the draft's, when
  // the Look area changed it, else the one this page was drawn with.
  function lookNow() {
    var d = Chronicle.customizeDraft ? Chronicle.customizeDraft() : {};
    var p = d && d.look && d.look.draft && d.look.draft.page;
    if (p && (p.look === 'annals' || p.look === 'standard')) return p.look;
    return document.documentElement.getAttribute('data-cz-page') === 'annals' ? 'annals' : 'standard';
  }
  // moving says whether the page allows moves: Calm fades only, and Off or a
  // device asking for less motion keep things still.
  function moving() {
    var h = document.documentElement;
    if (h.hasAttribute('data-motion') || h.hasAttribute('data-cz-reduce')) return false;
    return !(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  // fades says whether fades may play: everything but Off and a device
  // asking for less motion.
  function fades() {
    if (document.documentElement.getAttribute('data-motion') === 'off') return false;
    return !(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  function mount(el) {
    var S = parse(el.getAttribute('data-state'), {});
    var KEY = String(S.key || '');
    var CID = String(S.campaignId || '');
    // What the pages show now. With nothing saved they draw a built-in
    // arrangement; the editor starts from it, and Save stores it once changed.
    var shown = canon(S.layout);
    var saved = S.unsaved ? { rows: [] } : shown;
    var W = {};               // palette entries by key (type, or ext_widget:slug)
    var paletteState = 'loading';
    var UI = { device: 'desktop', viewAs: 'dm', sel: null, insp: 'add', galQ: '', galTiles: false, sheet: false, reading: false, look: lookNow(), addAt: null, peekKeep: false };
    // The page the layout is drawn with, and each widget's look on it, by
    // pvKey. A look is a node, or null when the widget draws nothing there.
    var PV = { entityId: '', pages: [], looks: {}, idx: {}, n: 0, pending: {}, busy: false, failed: false, loaded: false, timer: 0 };
    var OPTS = { types: null, maps: null };
    var alive = true, listeners = [], DRAG = null;
    function on(t, type, fn, o) { t.addEventListener(type, fn, o); listeners.push([t, type, fn, o]); }
    function $(sel, root) { return (root || el).querySelector(sel); }
    function $$(sel, root) { return Array.prototype.slice.call((root || el).querySelectorAll(sel)); }
    function toast(msg, bad, undoFn) { if (Chronicle.customizeToast) Chronicle.customizeToast(msg, bad, undoFn); }

    /* ---------- The draft ---------- */
    function current() {
      var d = Chronicle.customizeDraft ? Chronicle.customizeDraft() : {};
      var it = d && d.categories && d.categories.items && d.categories.items[KEY];
      return canon(it && it.layout ? it.layout : shown);
    }
    var L = current();
    // edit applies fn to a copy of the layout and, when it changed, puts the
    // copy in the draft as one Undo step. fn returns false to call it off.
    function edit(fn) {
      var next = clone(L);
      if (fn(next) === false) { render(); return false; }
      next = canon(next);
      if (JSON.stringify(next) === JSON.stringify(L)) { render(); return true; }
      // A page with no sections draws the built-in arrangement instead, so an
      // empty layout would not mean what it shows here.
      if (!next.rows.length) { toast('A page needs at least one section. Add another before removing this one.', true); render(); return false; }
      if (JSON.stringify(next).length > MAX_BYTES) { toast('This layout is too big to save. Remove some widgets or shorten their text.', true); render(); return false; }
      if (!Chronicle.customizeCategoryWrite) {
        toast('This change could not reach your draft. Reload Customize and try again.', true);
        render();
        return false;
      }
      L = next;
      var same = JSON.stringify(L) === JSON.stringify(saved);
      var before = UI.glideBefore || snap();
      if (UI.glideFrom && UI.sel) before[UI.sel.id] = UI.glideFrom;
      Chronicle.customizeCategoryWrite(KEY, { layout: same ? undefined : clone(L) }, { step: true });
      render();
      glide(before);
      return true;
    }
    function undo() { if (Chronicle.customizeUndo) Chronicle.customizeUndo(); }

    /* ---------- The page look ----------
     * The look bar previews the page in Standard or Annals. Annals is drawn
     * by page_look_annals.css from <html data-cz-page>, so the preview sets
     * that mark while the editor is open and puts back what the page came
     * with when it closes. Which look pages use is chosen in Look. */
    var LOOK0 = document.documentElement.getAttribute('data-cz-page') === 'annals' ? 'annals' : 'standard';
    function showLook(look) {
      var h = document.documentElement;
      if (look === 'annals') h.setAttribute('data-cz-page', 'annals');
      else h.removeAttribute('data-cz-page');
    }

    /* ---------- Glide ---------- */
    // snap notes where each widget is; glide then moves each one from there
    // to where it is drawn now, so a change slides rather than jumps.
    function snap() {
      var out = {};
      $$('[data-page] [data-id]').forEach(function (n) { out[n.getAttribute('data-id')] = n.getBoundingClientRect(); });
      return out;
    }
    function glide(before) {
      if (!moving() || !before) return;
      var frame = $('.cpe-frame'), z = frame && parseFloat(frame.style.zoom) || 1;
      $$('[data-page] [data-id]').forEach(function (n) {
        var a = before[n.getAttribute('data-id')];
        if (!a || n.classList.contains('dragging')) return;
        var b = n.getBoundingClientRect(), dx = (a.left - b.left) / z, dy = (a.top - b.top) / z;
        if (Math.abs(dx) < 1 && Math.abs(dy) < 1) return;
        n.animate([{ transform: 'translate(' + dx + 'px,' + dy + 'px)' }, { transform: 'none' }], { duration: GLIDE, easing: 'cubic-bezier(.2,.8,.2,1)' });
      });
    }

    /* ---------- Finding things ---------- */
    function findRow(rid, lay) {
      var rows = (lay || L).rows;
      for (var i = 0; i < rows.length; i++) if (rows[i].id === rid) return { row: rows[i], i: i };
      return null;
    }
    function findItem(id, lay) {
      var rows = (lay || L).rows;
      for (var i = 0; i < rows.length; i++) {
        var r = rows[i];
        if (r.kind === 'tiles') {
          for (var t = 0; t < r.tiles.length; t++) if (r.tiles[t].id === id) return { kind: 'tile', row: r, ri: i, list: r.tiles, i: t, item: r.tiles[t] };
        } else {
          for (var c = 0; c < r.columns.length; c++) {
            var bl = r.columns[c].blocks;
            for (var b = 0; b < bl.length; b++) if (bl[b].id === id) return { kind: 'block', row: r, ri: i, col: r.columns[c], list: bl, i: b, item: bl[b] };
          }
        }
      }
      return null;
    }
    function findSel(lay) {
      if (!UI.sel) return null;
      return UI.sel.kind === 'sec' ? findRow(UI.sel.id, lay) : findItem(UI.sel.id, lay);
    }
    function allItems(lay) {
      var out = [];
      (lay || L).rows.forEach(function (r) {
        if (r.kind === 'tiles') r.tiles.forEach(function (t) { out.push(t); });
        else r.columns.forEach(function (c) { c.blocks.forEach(function (b) { out.push(b); }); });
      });
      return out;
    }
    function hasType(type, lay) { return allItems(lay).some(function (b) { return b.type === type; }); }

    /* ---------- The palette ---------- */
    function keyOf(type, cfg) { return type === 'ext_widget' && cfg && cfg.widget_slug ? 'ext_widget:' + cfg.widget_slug : type; }
    function meta(type, cfg) {
      var m = W[keyOf(type, cfg)];
      if (m) return m;
      if (CONTAINER_KEYS[type]) return { key: type, type: type, name: humanize(type), icon: 'fa-object-group', desc: 'An old container widget.', sizes: [], container: true, fields: [] };
      return { key: type, type: type, name: paletteState === 'ready' ? humanize(type) : 'Widget', icon: 'fa-puzzle-piece', desc: 'This widget is not offered in this campaign right now, so pages skip it.', sizes: [], fields: [], missing: paletteState === 'ready' };
    }
    function loadPalette() {
      if (!S.blockTypes) { paletteState = 'failed'; render(); return; }
      Chronicle.apiFetch(S.blockTypes).then(function (r) {
        if (!r.ok) throw new Error('block types ' + r.status);
        return r.json();
      }).then(function (list) {
        if (!alive) return;
        (Array.isArray(list) ? list : (list && list.data) || []).forEach(function (b) {
          if (!b || !b.type) return;
          if (b.type === 'ext_widget' && !b.widget_slug) return;
          var k = b.widget_slug ? 'ext_widget:' + b.widget_slug : b.type;
          W[k] = {
            key: k, type: b.type, widgetSlug: b.widget_slug || '', name: b.label || humanize(b.type), icon: b.icon || 'fa-cube',
            desc: b.description || '', sizes: Array.isArray(b.tile_sizes) ? b.tile_sizes : [], container: !!b.container,
            singleton: !!b.singleton, retired: !!b.retired, fields: Array.isArray(b.config_fields) ? b.config_fields : []
          };
          W[k].group = groupOf(W[k]);
        });
        paletteState = 'ready';
        render();
      }).catch(function () {
        if (!alive) return;
        paletteState = 'failed';
        render();
      });
    }
    function newItem(m, asTile) {
      var cfg = m.widgetSlug ? { widget_slug: m.widgetSlug } : {};
      return asTile ? { id: uid('t'), type: m.type, size: m.sizes[0], config: cfg } : { id: uid('b'), type: m.type, config: cfg };
    }
    // refuseSingleton says why a second copy of a one-per-page widget can't go in.
    function refuseSingleton(m, lay) {
      if (m.singleton && hasType(m.type, lay)) { toast(m.name + ' can only be on a page once.', true); return true; }
      return false;
    }

    /* ---------- Drawing widgets with a real page ---------- */
    // pvKey names a widget's look: the same type and settings draw the same
    // wherever they sit. Who sees it and its height don't change the content.
    function pvKey(item) {
      var c = clone(item.config || {});
      delete c.visibility; delete c.minHeight;
      return item.type + '|' + stable(c);
    }
    function pvRef(key) {
      if (!PV.idx[key]) { PV.n++; PV.idx[key] = 'k' + PV.n; PV.byRef = PV.byRef || {}; PV.byRef['k' + PV.n] = key; }
      return PV.idx[key];
    }
    function queuePreview() {
      clearTimeout(PV.timer);
      PV.timer = setTimeout(loadPreview, 120);
    }
    function loadPreview() {
      if (!alive || PV.busy || !S.preview) return;
      var want = [], seen = {};
      allItems().forEach(function (b) {
        if (CONTAINER_KEYS[b.type] || meta(b.type, b.config).missing) return;
        var k = pvKey(b);
        if (seen[k] || Object.prototype.hasOwnProperty.call(PV.looks, k)) return;
        seen[k] = true;
        want.push({ id: pvRef(k), type: b.type, config: b.config || {} });
      });
      if (PV.loaded && !want.length) return;
      if (PV.loaded && !PV.entityId) return;
      PV.busy = true;
      var forPage = PV.entityId;
      Chronicle.apiFetch(S.preview, { method: 'POST', body: { entityId: forPage, blocks: want } }).then(function (r) {
        if (!r.ok) throw new Error('preview ' + r.status);
        return r.json();
      }).then(function (res) {
        PV.busy = false;
        if (!alive || forPage !== PV.entityId) { queuePreview(); return; }
        PV.failed = false;
        PV.loaded = true;
        PV.pages = Array.isArray(res.pages) ? res.pages : [];
        PV.entityId = res.entityId || '';
        var html = res.blocks || {};
        want.forEach(function (w) {
          var key = PV.byRef[w.id];
          PV.looks[key] = Object.prototype.hasOwnProperty.call(html, w.id) ? inertFrom(html[w.id]) : null;
        });
        if (!forPage) render(); else fill();
        queuePreview();
      }).catch(function () {
        PV.busy = false;
        if (!alive) return;
        PV.failed = true;
        PV.loaded = true;
        render();
      });
    }
    function pageName() {
      var p = PV.pages.filter(function (x) { return x.id === PV.entityId; })[0];
      return p ? p.name : '';
    }
    // fill puts each widget's look into the canvas, or says why there is none.
    function fill() {
      $$('[data-pv]').forEach(function (box) {
        var key = PV.byRef && PV.byRef[box.getAttribute('data-pv')];
        if (box.getAttribute('data-filled') === '1') return;
        var m = meta(box.getAttribute('data-type'), null);
        var name = box.getAttribute('data-name') || m.name;
        var has = key && Object.prototype.hasOwnProperty.call(PV.looks, key);
        if (has && PV.looks[key]) {
          box.textContent = '';
          box.appendChild(PV.looks[key].cloneNode(true));
          box.setAttribute('data-filled', '1');
          box.classList.remove('is-wait');
          return;
        }
        if (!PV.failed && ((PV.loaded && !PV.entityId) || has)) {
          // No page to draw with, or nothing on this one: show an example.
          var z = sizeOf(box.getAttribute('data-size') || '4x2');
          box.textContent = '';
          box.appendChild(exampleNode(box.getAttribute('data-type'), name, box.getAttribute('data-icon'), z.w, z.h));
          var tag = document.createElement('span');
          tag.className = 'cpe-ex-tag';
          tag.textContent = 'Example';
          box.appendChild(tag);
          box.setAttribute('data-filled', '1');
          box.classList.remove('is-wait');
          return;
        }
        var why;
        if (PV.failed) why = 'Couldn’t draw it here. It still shows on pages.';
        else { box.classList.add('is-wait'); return; }
        box.classList.remove('is-wait');
        // Built with DOM calls: the name and icon come back from attributes.
        var card = document.createElement('div');
        card.className = 'cpe-ph-card';
        var icon = document.createElement('i');
        icon.className = 'fa-solid ' + fa(box.getAttribute('data-icon'));
        icon.setAttribute('aria-hidden', 'true');
        var words = document.createElement('span');
        var b = document.createElement('b');
        b.textContent = name;
        var sub = document.createElement('span');
        sub.textContent = why;
        words.appendChild(b);
        words.appendChild(sub);
        card.appendChild(icon);
        card.appendChild(words);
        box.textContent = '';
        box.appendChild(card);
        box.setAttribute('data-filled', '1');
      });
    }
    function showWith(id) {
      if (id === PV.entityId) return;
      PV.entityId = id;
      PV.looks = {};
      render();
      queuePreview();
    }

    /* ---------- The canvas ---------- */
    function gridCols() { return UI.device === 'phone' ? 2 : 4; }
    function live(item, m) {
      return '<div class="cpe-live" inert data-pv="' + pvRef(pvKey(item)) + '" data-type="' + esc(item.type) + '" data-name="' + esc(m.name) + '" data-icon="' + esc(fa(m.icon)) + '"' + (item.size ? ' data-size="' + esc(item.size) + '"' : '') + '>' +
        '<div class="cpe-wait"><i></i><i></i><i></i></div></div>';
    }
    function marks(item, m, extra) {
      var sel = UI.sel && UI.sel.id === item.id;
      return (isDm(item) ? '<span class="cpe-dm">' + ic('fa-lock') + ' DM team only</span>' : '') +
        '<span class="cpe-badge' + (sel ? ' on' : '') + '">' + esc(m.name) + (extra || '') + '</span>';
    }
    function blockHTML(b) {
      var m = meta(b.type, b.config), sel = UI.sel && UI.sel.id === b.id;
      var base = 'cpe-blk' + (sel ? ' sel' : '');
      if (isDm(b) && UI.viewAs === 'player') {
        if (UI.reading) return '';
        return '<div class="' + base + ' ghosted" data-id="' + esc(b.id) + '" data-kind="block" tabindex="0">' + ic('fa-eye-slash') + ' ' + esc(m.name) + ': players don’t see this</div>';
      }
      var h = HEIGHT_PX[(b.config || {}).minHeight] || 0;
      var inner;
      if (m.container || CONTAINER_KEYS[b.type]) {
        var kids = containerKids(b);
        inner = '<div class="cpe-ph-card is-old">' + ic('fa-object-group') + '<span><b>' + esc(m.name) + ' (old container)</b><span>' +
          (kids.length ? 'Holds ' + esc(kids.map(function (k) { return meta(k.type, k.config).name; }).join(', ')) + '. ' : 'Empty. ') +
          'Pages can’t draw it. Select it to unpack its widgets.</span></span></div>';
      } else if (m.missing) {
        inner = '<div class="cpe-ph-card is-old">' + ic(m.icon) + '<span><b>' + esc(m.name) + '</b><span>' + esc(m.desc) + '</span></span></div>';
      } else {
        inner = live(b, m);
      }
      return '<div class="' + base + '" data-id="' + esc(b.id) + '" data-kind="block" tabindex="0" aria-label="' + esc(m.name) + '"' +
        (h ? ' style="min-height:' + h + 'px"' : '') + '>' + marks(b, m) + inner + '</div>';
    }
    function tileHTML(t, cols) {
      var m = meta(t.type, t.config), s = sizeOf(t.size), w = Math.min(s.w, cols), sel = UI.sel && UI.sel.id === t.id;
      if (isDm(t) && UI.viewAs === 'player') {
        if (UI.reading) return '';
        return '<div class="cpe-tile ghosted' + (sel ? ' sel' : '') + '" data-id="' + esc(t.id) + '" data-kind="tile" tabindex="0" style="grid-column:span ' + w + '">' + ic('fa-eye-slash') + ' ' + esc(m.name) + ': hidden</div>';
      }
      return '<div class="cpe-tile' + (sel ? ' sel' : '') + '" data-id="' + esc(t.id) + '" data-kind="tile" tabindex="0" aria-label="' + esc(m.name + ', ' + s.w + ' by ' + s.h) + '" style="grid-column:span ' + w + ';grid-row:span ' + s.h + '">' +
        (isDm(t) ? '<span class="cpe-dm">' + ic('fa-lock') + ' DM team only</span>' : '') + tileBar(t, m, s) + (m.missing ? '<div class="cpe-ph-card is-old">' + ic(m.icon) + '<span><b>' + esc(m.name) + '</b><span>' + esc(m.desc) + '</span></span></div>' : live(t, m)) +
        (m.sizes.length > 1 ? '<span class="cpe-rsz" data-rsz="' + esc(t.id) + '" title="Drag to resize"></span>' : '') + '</div>';
    }
    // addTileHTML is the last tile of a section. It opens no picker of its
    // own: it marks the section and turns the panel to its widget list, so
    // there is one place to choose from.
    function addTileHTML(r) {
      if (UI.viewAs === 'player' || UI.reading || r.tiles.length >= MAX_TILES) return '';
      var on = UI.addAt === r.id;
      return '<button type="button" class="cpe-addt' + (on ? ' on' : '') + '" data-act="tat" data-rid="' + esc(r.id) + '" aria-pressed="' + on + '" style="grid-column:span ' + (r.tiles.length ? 1 : 2) + '">' +
        ic(on ? 'fa-arrow-right' : 'fa-plus') + '<b>' + (on ? 'Pick one in the panel' : 'Add a widget') + '</b>' +
        '<span>' + (on ? 'It goes in here. Click again to stop.' : 'Choose from the list beside the page') + '</span></button>';
    }
    // aimAdd marks a tiles section as where the next widget picked in the
    // panel goes, and turns the panel to the list; again, it stops.
    function aimAdd(rid) {
      if (UI.addAt === rid) { UI.addAt = null; render(); return; }
      UI.addAt = rid;
      UI.sel = null;
      UI.insp = 'add';
      UI.galTiles = true;
      UI.sheet = true;
      render();
      var q = document.getElementById('cpe-q'), body = $('.cpe-insp-body');
      if (body) { body.scrollTop = 0; body.classList.remove('cpe-point'); void body.offsetWidth; body.classList.add('cpe-point'); }
      if (q) q.focus({ preventScroll: true });
    }
    function addTile(rid, k) {
      var m = W[k], item = null;
      UI.addAt = null;
      if (!m) return;
      if (!m.sizes.length) { toast(m.name + ' can’t be a tile. Drag it into a columns section instead.', true); render(); return; }
      if (edit(function (lay) {
        if (refuseSingleton(m, lay)) return false;
        var f = findRow(rid, lay);
        if (!f || f.row.kind !== 'tiles' || f.row.tiles.length >= MAX_TILES) return false;
        item = newItem(m, true);
        f.row.tiles.push(item);
        UI.sel = { kind: 'tile', id: item.id };
        UI.insp = 'sel';
      }) && item) { glow(item.id); toast('Added ' + m.name + '.'); }
    }
    // tileBar is the tile's own handle: its name and size, and move and
    // remove buttons, so a tile is arranged where it sits.
    function tileBar(t, m, s) {
      var f = findItem(t.id), i = f ? f.i : 0, n = f ? f.list.length : 1, id = esc(t.id), nm = esc(m.name);
      return '<div class="cpe-tbar"><span class="cpe-tbar-n">' + nm + '</span><span class="cpe-tbar-s">' + s.w + '×' + s.h + '</span>' +
        '<button type="button" data-act="tmv" data-id="' + id + '" data-v="-1" aria-label="Move ' + nm + ' earlier"' + (i === 0 ? ' disabled' : '') + '>' + ic('fa-chevron-left') + '</button>' +
        '<button type="button" data-act="tmv" data-id="' + id + '" data-v="1" aria-label="Move ' + nm + ' later"' + (i >= n - 1 ? ' disabled' : '') + '>' + ic('fa-chevron-right') + '</button>' +
        '<button type="button" data-act="trm" data-id="' + id + '" aria-label="Remove ' + nm + ' from the layout">' + ic('fa-xmark') + '</button></div>';
    }
    function rowHTML(r) {
      var sel = UI.sel && UI.sel.kind === 'sec' && UI.sel.id === r.id;
      if (r.kind === 'tiles') {
        var cols = gridCols();
        var tiles = r.tiles.map(function (t) { return tileHTML(t, cols); }).join('');
        return '<section class="cpe-sec' + (sel ? ' sel' : '') + '" data-rid="' + esc(r.id) + '"><button type="button" class="cpe-sec-tag" data-selsec="' + esc(r.id) + '">' + ic('fa-table-cells') + ' Tiles · ' + cols + ' across</button>' +
          '<div class="cpe-tiles" data-tiles="' + esc(r.id) + '" style="grid-template-columns:repeat(' + cols + ',minmax(0,1fr))">' +
          tiles + addTileHTML(r) + '</div></section>';
      }
      var stacked = UI.device === 'phone';
      var tpl = stacked ? '1fr' : r.columns.map(function (c) { return 'minmax(0,' + c.width + 'fr)'; }).join(' ');
      var colsH = r.columns.map(function (c) {
        var blocks = c.blocks.map(blockHTML).join('');
        return '<div class="cpe-col" data-cid="' + esc(c.id) + '" data-rid="' + esc(r.id) + '">' + (blocks || '<div class="cpe-empty">Drop a widget here</div>') + '</div>';
      }).join('');
      var divs = '';
      if (!stacked && r.columns.length > 1) {
        var s = 0, n = r.columns.length;
        for (var k = 0; k < n - 1; k++) {
          s += r.columns[k].width;
          divs += '<div class="cpe-div" data-div="' + k + '" data-rid="' + esc(r.id) + '" style="left:' + divLeft(s, k, n) + '" title="Drag to change the widths"><span class="cpe-chip"></span></div>';
        }
      }
      var label = r.columns.length === 1 ? 'full width' : r.columns.map(function (c) { return c.width; }).join(' | ');
      return '<section class="cpe-sec' + (sel ? ' sel' : '') + '" data-rid="' + esc(r.id) + '"><button type="button" class="cpe-sec-tag" data-selsec="' + esc(r.id) + '">' + ic('fa-table-columns') + ' Columns · ' + label + '</button>' +
        '<div class="cpe-cols" style="grid-template-columns:' + tpl + '">' + colsH + divs + '</div></section>';
    }
    // divLeft places the line after `s` twelfths, past k gaps of 24px.
    function divLeft(s, k, n) { return 'calc((100% - ' + ((n - 1) * 24) + 'px) * ' + (s / 12) + ' + ' + (k * 24 + 12) + 'px)'; }
    function insHTML(at, last) {
      return '<div class="cpe-ins' + (last ? ' always' : '') + '" data-at="' + at + '"><button type="button" data-act="addsec" data-at="' + at + '">' + ic('fa-plus') + ' Add section</button></div>';
    }
    function canvasHTML() {
      var h = '<div class="cpe-pagehead">' + ic('fa-file-lines') + '<span>';
      if (PV.failed) h += 'The page couldn’t be drawn here, so widgets show their names.';
      else if (!PV.loaded) h += 'Loading a page to show the layout with…';
      else if (!PV.entityId) h += 'No ' + esc((S.plural || 'pages').toLowerCase()) + ' yet, so widgets show example content. Your pages will fill them with their own.';
      else h += 'Shown with ' + esc(pageName()) + ', one of your ' + esc((S.plural || 'pages').toLowerCase());
      h += '</span>' + (UI.viewAs === 'player' ? '<span class="cpe-pill dm">' + ic('fa-eye') + ' As a player sees it</span>' : '<span class="cpe-pill">' + ic('fa-eye') + ' As you see it</span>') + '</div>';
      var room = L.rows.length < MAX_ROWS;
      room = room && !UI.reading;
      L.rows.forEach(function (r, i) { h += (room ? insHTML(i) : '') + rowHTML(r); });
      if (!UI.reading) h += room ? insHTML(L.rows.length, true) : '<p class="cpe-hint cpe-full">A page holds ' + MAX_ROWS + ' sections at most. Remove one to add another.</p>';
      return '<div class="cpe-frame ' + UI.device + '"><div class="cpe-page" data-page>' + h + '</div></div>';
    }

    /* ---------- Toolbar ---------- */
    function toolbarHTML() {
      var hist = Chronicle.customizeHistory ? Chronicle.customizeHistory() : { undo: false, redo: false };
      var seg = function (label, act, cur, opts) {
        return '<div class="cpe-seg" role="group" aria-label="' + label + '">' + opts.map(function (d) {
          return '<button type="button" data-act="' + act + '" data-v="' + d[0] + '" aria-pressed="' + (cur === d[0]) + '" aria-label="' + d[2] + '"' + (d[3] ? ' title="' + d[3] + '"' : '') + '>' + ic(d[1]) + '<span class="' + (act === 'dev' ? 'cpe-hide-md' : 'cpe-hide-sm') + '">' + d[2] + '</span></button>';
        }).join('') + '</div>';
      };
      var pages = PV.pages.length > 1 ? '<label class="cpe-lab cpe-hide-sm" for="cpe-page">Show with</label><select class="cpe-inp" id="cpe-page" data-act="page">' +
        PV.pages.map(function (p) { return '<option value="' + esc(p.id) + '"' + (p.id === PV.entityId ? ' selected' : '') + '>' + esc(p.name) + '</option>'; }).join('') + '</select>' : '';
      return '<div class="cpe-toolbar">' +
        '<button class="cpe-icon-btn" type="button" data-act="undo" title="Undo (Ctrl+Z)" aria-label="Undo"' + (hist.undo ? '' : ' disabled') + '>' + ic('fa-rotate-left') + '</button>' +
        '<button class="cpe-icon-btn" type="button" data-act="redo" title="Redo (Ctrl+Shift+Z)" aria-label="Redo"' + (hist.redo ? '' : ' disabled') + '>' + ic('fa-rotate-right') + '</button><span class="cpe-vr"></span>' +
        seg('Screen size', 'dev', UI.device, [['desktop', 'fa-desktop', 'Computer', 'Computer'], ['tablet', 'fa-tablet-screen-button', 'Tablet', 'Tablet'], ['phone', 'fa-mobile-screen', 'Phone', 'Phone']]) +
        seg('See it as', 'as', UI.viewAs, [['dm', 'fa-user-shield', 'You'], ['player', 'fa-user', 'A player']]) +
        seg('Page look', 'look', UI.look, [['standard', 'fa-file-lines', 'Standard', 'Preview the Standard look. Pages use the look chosen in Look.'], ['annals', 'fa-scroll', 'Annals', 'Preview the Annals look. Pages use the look chosen in Look.']]) +
        '<span class="cpe-read"><button type="button" class="toggle" role="switch" data-act="read" aria-checked="' + UI.reading + '" aria-label="See the page as readers do"><span class="toggle-knob"></span></button><span class="cpe-hide-md">See the page as readers do</span></span>' +
        '<span class="cpe-sp"></span>' + pages +
        (UI.reading ? '</div>' : '<button class="btn-secondary btn-sm" type="button" data-act="start" title="Start from a layout in the Library" aria-label="Start from…">' + ic('fa-swatchbook') + '<span class="cpe-hide-md">Start from…</span></button>' +
        '<button class="btn-secondary btn-sm" type="button" data-act="keep" title="Keep a copy of this layout in the Library" aria-label="Keep in Library">' + ic('fa-bookmark') + '<span class="cpe-hide-md">Keep in Library</span></button></div>');
    }

    /* ---------- Inspector ---------- */
    function inspectorHTML() {
      var found = findSel();
      if (!found) { UI.sel = null; if (UI.insp === 'sel') UI.insp = 'add'; }
      var body = UI.insp === 'sel' && found ? selectedHTML(found) : galleryHTML();
      return '<aside class="cpe-insp' + (UI.sheet ? ' open' : '') + '" aria-label="Widgets and settings"><div class="cpe-insp-tabs">' +
        '<button type="button" class="cpe-handle" data-act="sheet" aria-label="Show or hide the panel"></button><div class="cpe-seg" role="tablist">' +
        '<button type="button" role="tab" data-act="insp" data-v="add" aria-selected="' + !(UI.insp === 'sel' && found) + '">' + ic('fa-plus') + ' Add widgets</button>' +
        '<button type="button" role="tab" data-act="insp" data-v="sel" aria-selected="' + !!(UI.insp === 'sel' && found) + '"' + (found ? '' : ' disabled') + '>' + ic('fa-sliders') + ' Selected</button></div></div>' +
        '<div class="cpe-insp-body">' + body + '</div></aside>';
    }
    function galleryHTML() {
      if (paletteState === 'loading') return '<div class="cpe-note">Loading widgets…</div>';
      if (paletteState === 'failed') return '<div class="cpe-note is-bad"><b>Widgets didn’t load</b><span>Reload the page to try again. Your layout is safe.</span></div>';
      var q = UI.galQ.toLowerCase(), groups = {};
      Object.keys(W).forEach(function (k) {
        var m = W[k];
        if (m.retired || m.container) return;
        if (UI.galTiles && !m.sizes.length) return;
        if (q && (m.name + ' ' + m.desc + ' ' + m.group).toLowerCase().indexOf(q) < 0) return;
        (groups[m.group] = groups[m.group] || []).push(k);
      });
      var h = '<div class="cpe-search">' + ic('fa-magnifying-glass') + '<input class="cpe-inp" id="cpe-q" data-act="q" placeholder="Search widgets" value="' + esc(UI.galQ) + '" autocomplete="off" aria-label="Search widgets"></div>' +
        '<div class="cpe-row-set"><span class="cpe-hint">Only ones that can be tiles</span><button type="button" class="toggle" role="switch" data-act="galtiles" aria-checked="' + UI.galTiles + '" aria-label="Only ones that can be tiles"><span class="toggle-knob"></span></button></div>' +
        (UI.addAt && findRow(UI.addAt) ? '<p class="cpe-hint cpe-aim">' + ic('fa-arrow-left') + ' Click a widget to put it in the tiles section you marked.</p>'
          : '<p class="cpe-hint">Drag a widget onto the page, or click it to add it ' + (UI.sel ? 'next to what you selected.' : 'at the end.') + '</p>');
      var any = false;
      GROUPS.forEach(function (g) {
        if (!groups[g]) return;
        any = true;
        h += '<div class="cpe-gal-group"><h4>' + esc(g) + '</h4><div class="cpe-gal">' + groups[g].map(function (k) {
          var m = W[k], once = m.singleton && hasType(m.type);
          return '<div class="cpe-gwrap"><button type="button" class="cpe-gw" data-gw="' + esc(k) + '" title="' + esc(m.desc) + '"' + (once ? ' disabled' : '') + '>' +
            '<span class="cpe-gw-ic">' + ic(m.icon) + '</span><span class="cpe-gw-nm">' + esc(m.name) + '</span>' +
            '<span class="cpe-gw-sz">' + (once ? 'Already on the page' : m.sizes.length ? esc(m.sizes.map(function (s) { return s.replace('x', '×'); }).join(' · ')) : 'Columns only') + '</span></button>' +
            '<button type="button" class="cpe-eye" data-act="peek" data-k="' + esc(k) + '" aria-label="Preview ' + esc(m.name) + '" title="Preview">' + ic('fa-eye') + '</button></div>';
        }).join('') + '</div></div>';
      });
      if (!any) h += '<div class="cpe-note"><b>No widgets match “' + esc(UI.galQ) + '”</b><span>Try a shorter word' + (UI.galTiles ? ', or turn off “Only ones that can be tiles”' : '') + '.</span></div>';
      return h;
    }
    function sizePic(s) {
      var z = sizeOf(s), h = '';
      for (var r = 0; r < Math.max(2, Math.min(z.h, 6)); r++) for (var c = 0; c < 4; c++) h += '<i class="' + (c < z.w && r < z.h ? 'f' : '') + '"></i>';
      return '<span class="cpe-szpic">' + h + '</span>';
    }
    function selectedHTML(f) {
      if (UI.sel.kind === 'sec') return sectionHTML(f.row, f.i);
      var it = f.item, m = meta(it.type, it.config), cfg = it.config || {};
      var h = '<div class="cpe-title"><span class="cpe-title-ic">' + ic(m.icon) + '</span><div><h3>' + esc(m.name) + '</h3>' + (m.desc ? '<p>' + esc(m.desc) + '</p>' : '') + '</div></div>';
      if (m.container || CONTAINER_KEYS[it.type]) {
        var kids = containerKids(it);
        h += '<div class="cpe-grp"><span class="cpe-lab">An old container</span><p class="cpe-hint">Pages can’t draw containers, so its widgets don’t show. Unpack it to put ' +
          (kids.length ? 'its ' + kids.length + (kids.length === 1 ? ' widget' : ' widgets') : 'it') + ' straight into this column.</p>' +
          '<div class="cpe-acts"><button class="btn-primary btn-sm" type="button" data-act="unpack">' + ic('fa-box-open') + ' Unpack</button></div></div>';
      }
      h += '<div class="cpe-grp"><span class="cpe-lab">Who sees it</span><div class="cpe-seg">' +
        '<button type="button" data-act="vis" data-v="" aria-pressed="' + !isDm(it) + '">' + ic('fa-users') + ' Everyone</button>' +
        '<button type="button" data-act="vis" data-v="dm_only" aria-pressed="' + isDm(it) + '">' + ic('fa-lock') + ' DM team only</button></div>' +
        '<span class="cpe-hint">This sets it for every page in ' + esc(S.plural || 'this category') + '. The DM team is you and the members you gave DM access.</span></div>';
      if (f.kind === 'tile') {
        h += '<div class="cpe-grp"><span class="cpe-lab">Size</span><div class="cpe-sizes">' + m.sizes.map(function (s) {
          return '<button type="button" data-act="size" data-v="' + esc(s) + '" aria-pressed="' + (it.size === s) + '">' + sizePic(s) + esc(s.replace('x', ' × ')) + '</button>';
        }).join('') + '</div><span class="cpe-hint">Or pull the tile’s corner on the page. It shows more as it grows, and never scrolls inside.</span></div>';
      } else if (!m.missing) {
        h += '<div class="cpe-grp"><span class="cpe-lab">Height</span><div class="cpe-seg cpe-seg-wrap">' + HEIGHTS.map(function (x) {
          return '<button type="button" data-act="height" data-v="' + x[0] + '" aria-pressed="' + ((cfg.minHeight || '') === x[0]) + '">' + x[1] + '</button>';
        }).join('') + '</div><span class="cpe-hint">Fit grows with what’s inside. The others keep at least that much room.</span></div>';
      }
      h += fieldsHTML(m, cfg);
      if (it.type === 'attributes' && S.fieldsUrl) {
        h += '<div class="cpe-grp"><span class="cpe-lab">Which fields</span><p class="cpe-hint">This shows the fields the category asks for' +
          (S.fields ? ': ' + Number(S.fields) + (Number(S.fields) === 1 ? ' field' : ' fields') : '') + '.</p>' +
          '<div class="cpe-acts"><a class="btn-secondary btn-sm" href="' + esc(S.fieldsUrl) + '">' + ic('fa-list-ul') + ' Edit fields</a></div></div>';
      }
      var room = f.kind === 'tile' || f.list.length < MAX_BLOCKS;
      h += '<div class="cpe-grp"><span class="cpe-lab">Place</span><div class="cpe-acts">' +
        '<button class="btn-secondary btn-sm" type="button" data-act="mv" data-v="-1"' + (f.i === 0 ? ' disabled' : '') + '>' + ic('fa-arrow-up') + ' Earlier</button>' +
        '<button class="btn-secondary btn-sm" type="button" data-act="mv" data-v="1"' + (f.i === f.list.length - 1 ? ' disabled' : '') + '>' + ic('fa-arrow-down') + ' Later</button>' +
        '<button class="btn-secondary btn-sm" type="button" data-act="dup"' + (m.singleton || m.missing || !room ? ' disabled' : '') + '>' + ic('fa-clone') + ' Duplicate</button></div>' +
        '<span class="cpe-hint">On a phone, or without a mouse, these move it instead of dragging. <kbd>Alt</kbd> + <kbd>↑</kbd> <kbd>↓</kbd> work too.</span></div>';
      h += '<div class="cpe-grp"><div class="cpe-acts"><button class="btn-danger btn-sm" type="button" data-act="rm">' + ic('fa-trash-can') + ' Remove from the layout</button></div>' +
        '<span class="cpe-hint">Removing it from the layout never deletes what pages wrote in it. Put it back and it all shows again.</span></div>';
      return h;
    }
    // fieldsHTML draws a widget's own settings from its config_fields.
    function fieldsHTML(m, cfg) {
      if (!m.fields.length) return '';
      return m.fields.map(function (fd, n) {
        var id = 'cpe-f' + n, key = String(fd.key || ''), v = cfg[key] !== undefined ? cfg[key] : (fd.default !== undefined ? fd.default : '');
        var inp;
        if (fd.type === 'number') {
          inp = '<input class="cpe-inp" type="number" id="' + id + '" data-cfg="' + esc(key) + '" value="' + esc(v) + '"' + (fd.min != null ? ' min="' + Number(fd.min) + '"' : '') + (fd.max != null ? ' max="' + Number(fd.max) + '"' : '') + '>';
        } else if (fd.type === 'textarea') {
          inp = '<textarea class="cpe-inp" rows="3" id="' + id + '" data-cfg="' + esc(key) + '">' + esc(v) + '</textarea>';
        } else if (fd.type === 'select' || fd.type === 'entity_type' || fd.type === 'map') {
          var opts = fd.type === 'select' ? (fd.options || []).map(function (o) { return [o.value, o.label]; }) : null;
          var list = fd.type === 'entity_type' ? OPTS.types : fd.type === 'map' ? OPTS.maps : null;
          if (!opts) {
            if (list === null) needOptions(fd.type);
            opts = [['', fd.type === 'map' ? 'Choose a map' : 'Choose a category']].concat((list || []).map(function (o) { return [o.id, o.name]; }));
            if (list === false) opts = [['', 'Couldn’t load the list']];
          }
          inp = '<select class="cpe-inp" id="' + id + '" data-cfg="' + esc(key) + '">' + opts.map(function (o) {
            return '<option value="' + esc(o[0]) + '"' + (String(o[0]) === String(v) ? ' selected' : '') + '>' + esc(o[1]) + '</option>';
          }).join('') + '</select>';
        } else {
          inp = '<input class="cpe-inp" type="text" id="' + id + '" data-cfg="' + esc(key) + '" value="' + esc(v) + '">';
        }
        return '<div class="cpe-grp"><label class="cpe-lab" for="' + id + '">' + esc(fd.label || humanize(key)) + '</label>' + inp + '</div>';
      }).join('');
    }
    function needOptions(type) {
      var k = type === 'map' ? 'maps' : 'types';
      if (OPTS[k] !== null || OPTS['_' + k]) return;
      OPTS['_' + k] = true;
      var url = '/api/v1/campaigns/' + encodeURIComponent(CID) + (type === 'map' ? '/maps' : '/entity-types');
      Chronicle.apiFetch(url).then(function (r) { if (!r.ok) throw new Error(String(r.status)); return r.json(); }).then(function (res) {
        var arr = Array.isArray(res) ? res : (res && res.data) || [];
        OPTS[k] = arr.map(function (o) { return { id: o.id, name: o.name || '(unnamed)' }; });
        if (alive) render();
      }).catch(function () { OPTS[k] = false; if (alive) render(); });
    }
    function sectionHTML(r, i) {
      var h;
      if (r.kind === 'tiles') {
        h = '<div class="cpe-title"><span class="cpe-title-ic">' + ic('fa-table-cells') + '</span><div><h3>Tiles section</h3><p>Widgets sit on a grid in set sizes, like home-screen widgets. Four across on a computer, two on a phone. They keep their order and fill gaps.</p></div></div>';
        h += '<div class="cpe-grp"><span class="cpe-lab">In this section</span><p class="cpe-hint">' + r.tiles.length + (r.tiles.length === 1 ? ' tile' : ' tiles') + ' of ' + MAX_TILES + '. Drag them to change the order, or pull a corner to resize.</p></div>';
      } else {
        var key = r.columns.map(function (c) { return c.width; }).join(',');
        h = '<div class="cpe-title"><span class="cpe-title-ic">' + ic('fa-table-columns') + '</span><div><h3>Columns section</h3><p>For long text, tables and read-aloud. Each column grows to fit. Drag the line between columns to change their widths.</p></div></div>';
        h += '<div class="cpe-grp"><span class="cpe-lab">Columns</span><div class="cpe-presets">' + COL_PRESETS.map(function (p) {
          var k = p.join(',');
          return '<button type="button" data-act="preset" data-v="' + k + '" aria-pressed="' + (k === key) + '"><span class="cpe-pv">' + p.map(function (w) { return '<i style="flex:' + w + '"></i>'; }).join('') + '</span>' + PRESET_NAMES[k] + '</button>';
        }).join('') + '</div>' + (PRESET_NAMES[key] ? '' : '<span class="cpe-hint">Now ' + esc(key.replace(/,/g, ' | ')) + ', set by dragging.</span>') +
          '<span class="cpe-hint">Fewer columns move their widgets into the last one; nothing is removed.</span></div>';
      }
      h += '<div class="cpe-grp"><span class="cpe-lab">Place</span><div class="cpe-acts">' +
        '<button class="btn-secondary btn-sm" type="button" data-act="mvsec" data-v="-1"' + (i === 0 ? ' disabled' : '') + '>' + ic('fa-arrow-up') + ' Up</button>' +
        '<button class="btn-secondary btn-sm" type="button" data-act="mvsec" data-v="1"' + (i === L.rows.length - 1 ? ' disabled' : '') + '>' + ic('fa-arrow-down') + ' Down</button>' +
        '<button class="btn-secondary btn-sm" type="button" data-act="dupsec"' + (L.rows.length >= MAX_ROWS ? ' disabled' : '') + '>' + ic('fa-clone') + ' Duplicate</button></div></div>';
      h += '<div class="cpe-grp"><div class="cpe-acts"><button class="btn-danger btn-sm" type="button" data-act="rm">' + ic('fa-trash-can') + ' Remove section</button></div>' +
        '<span class="cpe-hint">Its widgets leave the layout too. Undo brings them back.</span></div>';
      return h;
    }

    /* ---------- Drawing ---------- */
    function notices() {
      if (!S.systemDrawn) return '';
      return '<div class="cpe-banner">' + ic('fa-circle-info') + '<span>Your game system draws ' + esc((S.plural || 'these pages').toLowerCase()) + ' with its own sheet, so pages don’t use this layout right now. Changes here wait until they do.</span></div>';
    }
    function render() {
      if (!alive) return;
      var cv = $('.cpe-canvas'), top = cv ? cv.scrollTop : 0, ib = $('.cpe-insp-body'), itop = ib ? ib.scrollTop : 0;
      var ae = document.activeElement, fid = ae && el.contains(ae) ? ae.id : '', pos = fid === 'cpe-q' ? ae.selectionStart : null;
      // The canvas carries the player-view mark while it shows a player's
      // view, so marked Director-only parts inside drawn widgets hide too.
      el.innerHTML = '<div class="cpe' + (UI.reading ? ' reading' : '') + '">' + notices() + '<div class="cpe-editor"><div class="cpe-main">' + toolbarHTML() +
        '<div class="cpe-canvas"' + (UI.viewAs === 'player' ? ' data-view-as-player' : '') + '>' + canvasHTML() + '</div></div>' + (UI.reading ? '' : inspectorHTML()) + '</div></div>';
      cv = $('.cpe-canvas'); if (cv) cv.scrollTop = top;
      ib = $('.cpe-insp-body'); if (ib && UI.keepInsp) ib.scrollTop = itop;
      UI.keepInsp = false;
      if (fid) { var f = document.getElementById(fid); if (f) { f.focus(); if (pos != null && f.setSelectionRange) f.setSelectionRange(pos, pos); } }
      fill();
      fit();
      queuePreview();
    }
    // fit makes the editor reach the bottom of the window, so the canvas and
    // the panel scroll on their own.
    function fit() {
      var box = $('.cpe-editor');
      if (!box) return;
      var top = box.getBoundingClientRect().top + window.scrollY;
      box.style.height = Math.max(520, window.innerHeight - top - 8) + 'px';
      // The page is drawn at the width it has on that screen and scaled to
      // fit, so text wraps where it will on the real page.
      var cv = $('.cpe-canvas'), frame = $('.cpe-frame');
      if (!cv || !frame) return;
      var want = FRAME_W[UI.device] || 0, room = cv.clientWidth - 56;
      // Never below 85%, where small widget text stops being readable;
      // past that the page is drawn narrower than the real one instead.
      var z = want && room < want ? Math.max(0.85, room / want) : 1;
      frame.style.width = z < 1 ? Math.min(want, Math.floor(room / z)) + 'px' : '';
      frame.style.zoom = z < 1 ? String(z) : '';
    }

    /* ---------- Changing the layout ---------- */
    // readMode shows the page as its readers get it: no frames, labels,
    // handles or panel. The frame fades across, the one change Calm allows.
    function readMode(on) {
      UI.reading = !!on;
      UI.addAt = null;
      if (on) { UI.sel = null; UI.insp = 'add'; }
      closePop();
      render();
      fadeFrame();
    }
    // fadeFrame fades the page in after it changes how it is drawn.
    function fadeFrame() {
      var f = $('.cpe-frame');
      if (f && f.animate && fades()) f.animate([{ opacity: 0.35 }, { opacity: 1 }], { duration: 200, easing: 'cubic-bezier(.2,.8,.2,1)' });
    }
    function select(sel) {
      UI.sel = sel;
      UI.addAt = null;
      UI.insp = sel ? 'sel' : 'add';
      if (sel) UI.sheet = true;
      render();
    }
    function glow(id) {
      var n = id && el.querySelector('[data-id="' + CSS.escape(id) + '"], [data-rid="' + CSS.escape(id) + '"].cpe-sec');
      if (n) { n.classList.add('glow'); n.scrollIntoView({ block: 'nearest', behavior: 'smooth' }); }
    }
    // addWidget puts a widget next to what is selected, or at the end: in the
    // last tiles section when it can be a tile, else in a new full-width section.
    function addWidget(k) {
      var m = W[k], item = null;
      if (!m) return;
      var ok = edit(function (lay) {
        if (refuseSingleton(m, lay)) return false;
        var f = findSel(lay), tileable = m.sizes.length > 0, isSec = UI.sel && UI.sel.kind === 'sec';
        if (f && !isSec && f.kind === 'tile' && tileable && f.list.length < MAX_TILES) { item = newItem(m, true); f.list.splice(f.i + 1, 0, item); }
        else if (f && !isSec && f.kind === 'block' && f.list.length < MAX_BLOCKS) { item = newItem(m, false); f.list.splice(f.i + 1, 0, item); }
        else if (f && isSec && f.row.kind === 'tiles' && tileable && f.row.tiles.length < MAX_TILES) { item = newItem(m, true); f.row.tiles.push(item); }
        else if (f && isSec && f.row.kind !== 'tiles' && f.row.columns[f.row.columns.length - 1].blocks.length < MAX_BLOCKS) { item = newItem(m, false); f.row.columns[f.row.columns.length - 1].blocks.push(item); }
        else {
          var tr = null;
          lay.rows.forEach(function (r) { if (r.kind === 'tiles' && r.tiles.length < MAX_TILES) tr = r; });
          if (tileable && tr) { item = newItem(m, true); tr.tiles.push(item); }
          else if (lay.rows.length >= MAX_ROWS) { toast('This page has no room left. Remove a section first.', true); return false; }
          else { item = newItem(m, false); lay.rows.push(colsRow([12], [[item]])); }
        }
        UI.sel = { kind: item.size ? 'tile' : 'block', id: item.id };
        UI.insp = 'sel';
      });
      if (ok && item) { glow(item.id); toast('Added ' + m.name + '.'); }
    }
    function addSection(kind, at) {
      var row = kind === 'tiles' ? tilesRow([]) : colsRow(kind.split(',').map(Number), []);
      if (edit(function (lay) {
        if (lay.rows.length >= MAX_ROWS) { toast('A page holds ' + MAX_ROWS + ' sections at most.', true); return false; }
        lay.rows.splice(at, 0, row);
        UI.sel = { kind: 'sec', id: row.id };
        UI.insp = 'sel';
      })) glow(row.id);
    }
    function moveSel(d) {
      var f = findSel();
      if (!f || UI.sel.kind === 'sec') return;
      var j = f.i + d;
      if (j < 0 || j >= f.list.length) return;
      edit(function (lay) { var x = findSel(lay); x.list.splice(x.i, 1); x.list.splice(j, 0, x.item); });
    }
    function moveSection(d) {
      edit(function (lay) {
        var x = findSel(lay), j = x.i + d;
        if (j < 0 || j >= lay.rows.length) return false;
        lay.rows.splice(x.i, 1);
        lay.rows.splice(j, 0, x.row);
      });
    }
    function removeSel() {
      var f = findSel();
      if (!f) return;
      var name = UI.sel.kind === 'sec' ? (f.row.kind === 'tiles' ? 'the tiles section' : 'the columns section') : meta(f.item.type, f.item.config).name;
      if (edit(function (lay) {
        var x = findSel(lay);
        if (UI.sel.kind === 'sec') lay.rows.splice(x.i, 1); else x.list.splice(x.i, 1);
        UI.sel = null; UI.insp = 'add';
      })) toast('Removed ' + name + ' from the layout.', false, undo);
    }
    function duplicateSel() {
      var nid = null;
      edit(function (lay) {
        var x = findSel(lay);
        if (UI.sel.kind === 'sec') {
          if (lay.rows.length >= MAX_ROWS) return false;
          var c = clone(x.row);
          reId(c);
          var dup = false;
          (c.kind === 'tiles' ? c.tiles : [].concat.apply([], c.columns.map(function (col) { return col.blocks; }))).forEach(function (b) {
            var m = meta(b.type, b.config);
            if (m.singleton) dup = true;
          });
          if (dup) { toast('This section holds a widget that can only be on a page once.', true); return false; }
          lay.rows.splice(x.i + 1, 0, c);
          nid = c.id;
          UI.sel = { kind: 'sec', id: c.id };
          return;
        }
        var m = meta(x.item.type, x.item.config);
        if (m.singleton) { toast(m.name + ' can only be on a page once.', true); return false; }
        if (x.kind === 'block' && x.list.length >= MAX_BLOCKS) { toast('A column holds ' + MAX_BLOCKS + ' widgets at most.', true); return false; }
        if (x.kind === 'tile' && x.list.length >= MAX_TILES) { toast('A tiles section holds ' + MAX_TILES + ' tiles at most.', true); return false; }
        var cp = clone(x.item);
        cp.id = uid(x.kind === 'tile' ? 't' : 'b');
        x.list.splice(x.i + 1, 0, cp);
        nid = cp.id;
        UI.sel = { kind: x.kind, id: cp.id };
      });
      glow(nid);
    }
    // unpack puts an old container's widgets into its column, where it stood.
    function unpack() {
      edit(function (lay) {
        var x = findSel(lay);
        if (!x || x.kind !== 'block') return false;
        var kids = containerKids(x.item).map(function (k) { return { id: uid('b'), type: String(k.type), config: k.config && typeof k.config === 'object' ? k.config : {} }; });
        if (x.list.length - 1 + kids.length > MAX_BLOCKS) { toast('That column would hold more than ' + MAX_BLOCKS + ' widgets. Move some out first.', true); return false; }
        x.list.splice.apply(x.list, [x.i, 1].concat(kids));
        UI.sel = kids.length ? { kind: 'block', id: kids[0].id } : null;
        UI.insp = kids.length ? 'sel' : 'add';
      });
    }
    function setColumns(widths) {
      edit(function (lay) {
        var r = findSel(lay).row, all = r.columns;
        var next = widths.map(function (w, i) { return { id: all[i] ? all[i].id : uid('c'), width: w, blocks: all[i] ? all[i].blocks : [] }; });
        for (var k = widths.length; k < all.length; k++) next[widths.length - 1].blocks = next[widths.length - 1].blocks.concat(all[k].blocks);
        if (next[widths.length - 1].blocks.length > MAX_BLOCKS) { toast('The last column would hold more than ' + MAX_BLOCKS + ' widgets. Move some out first.', true); return false; }
        r.columns = next;
      });
    }
    function setConfig(key, raw, fd) {
      edit(function (lay) {
        var x = findSel(lay), c = cfgOf(x.item), v = raw;
        if (fd && fd.type === 'number') {
          v = parseInt(raw, 10);
          if (isNaN(v)) v = '';
          else {
            if (fd.min != null && v < fd.min) v = Number(fd.min);
            if (fd.max != null && v > fd.max) v = Number(fd.max);
          }
        }
        if (v === '' || v == null) delete c[key]; else c[key] = v;
      });
    }

    /* ---------- Menus ---------- */
    var popEl = null;
    function closePop() {
      if (popEl) {
        var gone = popEl;
        popEl = null;
        gone.style.pointerEvents = 'none';
        if (fades() && gone.animate) gone.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 120, easing: 'cubic-bezier(.4,0,1,1)' }).onfinish = function () { gone.remove(); };
        else gone.remove();
      }
      document.removeEventListener('pointerdown', popOff, true);
    }
    // Small menus close on a click off them; the peek is a panel, so it
    // closes only on its X, Escape or the eye that opened it.
    function popOff(e) { if (popEl && !popEl.contains(e.target) && !popEl.hasAttribute('data-keep') && !popEl.hasAttribute('data-panel')) closePop(); }
    function openPop(anchor, html, align) {
      closePop();
      popEl = document.createElement('div');
      popEl.className = 'cpe-pop';
      popEl.innerHTML = html;
      document.body.appendChild(popEl);
      var r = anchor.getBoundingClientRect(), w = popEl.offsetWidth, hgt = popEl.offsetHeight;
      var left = align === 'right' ? r.right - w : r.left + r.width / 2 - w / 2;
      popEl.style.left = Math.max(12, Math.min(left, window.innerWidth - w - 12)) + 'px';
      var top = r.bottom + 6;
      if (top + hgt > window.innerHeight - 12) top = Math.max(12, r.top - hgt - 6);
      popEl.style.top = top + 'px';
      popEl.addEventListener('keydown', function (e) { if (e.key === 'Escape') { closePop(); anchor.focus(); } });
      setTimeout(function () { document.addEventListener('pointerdown', popOff, true); }, 0);
      var first = popEl.querySelector('button, input');
      if (first) first.focus();
      return popEl;
    }
    // peek opens a widget's example beside the panel. Its lists, chips and
    // steps answer clicks, so the owner can try what it does; the switch
    // keeps it open while they carry on arranging.
    var TRY = '.cpe-ex-li, .cpe-ex-steps li, .cpe-ex-chip, .cpe-ex-tags span, .cpe-ex-book div, .cpe-ex-post, .cpe-ex-tbl tbody tr';
    function peek(btn, k) {
      var m = W[k];
      if (!m) return;
      if (popEl && popEl.getAttribute('data-panel') === k) { closePop(); return; }
      var z = sizeOf(m.sizes[0] || '4x2');
      var p = openPop(btn, '<div class="cpe-peek"><div class="cpe-peek-h"><span class="cpe-title-ic">' + ic(m.icon) + '</span><div><b>' + esc(m.name) + '</b><span>' +
        (m.sizes.length ? 'As a tile, ' + esc(m.sizes.map(function (x) { return x.replace('x', '×'); }).join(', ')) : 'In columns') + '</span></div>' +
        '<button type="button" class="cpe-icon-btn" data-peekx aria-label="Close the preview">' + ic('fa-xmark') + '</button></div>' +
        (m.desc ? '<p class="cpe-hint">' + esc(m.desc) + '</p>' : '') +
        '<div class="cpe-peek-box" style="width:' + Math.min(4, z.w) * 116 + 'px;min-height:' + z.h * 104 + 'px"></div>' +
        '<div class="cpe-row-set"><span class="cpe-hint">Keep it open and try it</span><button type="button" class="toggle" role="switch" data-peekkeep aria-checked="' + UI.peekKeep + '" aria-label="Keep it open and try it"><span class="toggle-knob"></span></button></div>' +
        '<p class="cpe-hint">Example content. Your pages fill it with their own.</p></div>');
      p.classList.add('cpe-peek-pop');
      p.setAttribute('data-panel', k);
      if (UI.peekKeep) p.setAttribute('data-keep', '');
      $('.cpe-peek-box', p).appendChild(exampleNode(m.type, m.name, m.icon, z.w, z.h));
      // Beside the panel, level with the widget, rather than over the list.
      var panel = $('.cpe-insp'), pr = panel && panel.getBoundingClientRect(), br = btn.getBoundingClientRect();
      if (pr && window.innerWidth > 1100) {
        p.style.left = Math.max(12, pr.left - p.offsetWidth - 12) + 'px';
        p.style.top = Math.max(12, Math.min(br.top - 20, window.innerHeight - p.offsetHeight - 12)) + 'px';
      }
      // It grows in beside the panel; Calm only fades it in.
      p.style.transformOrigin = 'right center';
      if (fades() && p.animate) p.animate(moving() ? [{ opacity: 0, transform: 'scale(.97)' }, { opacity: 1, transform: 'none' }] : [{ opacity: 0 }, { opacity: 1 }], { duration: 200, easing: 'cubic-bezier(.2,.8,.2,1)' });
      p.addEventListener('click', function (e) {
        if (e.target.closest('[data-peekx]')) { closePop(); btn.focus(); return; }
        var sw = e.target.closest('[data-peekkeep]');
        if (sw) {
          UI.peekKeep = !UI.peekKeep;
          sw.setAttribute('aria-checked', String(UI.peekKeep));
          if (UI.peekKeep) p.setAttribute('data-keep', ''); else p.removeAttribute('data-keep');
          return;
        }
        var part = e.target.closest(TRY);
        if (part && p.contains(part)) part.classList.toggle('on');
      });
    }
    function sectionMenu(btn, at) {
      var p = openPop(btn, '<div class="cpe-pop-list">' + SECTION_KINDS.map(function (o) {
        return '<button type="button" data-sk="' + o[0] + '">' + ic(o[3]) + '<span><b>' + o[1] + '</b><span>' + o[2] + '</span></span></button>';
      }).join('') + '</div>');
      p.addEventListener('click', function (e) {
        var b = e.target.closest('[data-sk]');
        if (!b) return;
        closePop();
        addSection(b.getAttribute('data-sk'), at);
      });
    }
    function presetsMenu(btn) {
      var p = openPop(btn, '<div class="cpe-pop-list"><p class="cpe-hint">Loading layouts…</p></div>', 'right');
      Chronicle.apiFetch(S.presets).then(function (r) { if (!r.ok) throw new Error(String(r.status)); return r.json(); }).then(function (list) {
        if (popEl !== p) return;
        list = Array.isArray(list) ? list : (list && list.data) || [];
        p.firstChild.innerHTML = list.length ? list.map(function (x, i) {
          return '<button type="button" data-pr="' + i + '">' + ic(x.icon || 'fa-table-columns') + '<span><b>' + esc(x.name) + '</b>' +
            (x.is_builtin ? '' : ' <span class="cpe-pill">Yours</span>') + (x.description ? '<span>' + esc(x.description) + '</span>' : '') + '</span></button>';
        }).join('') : '<p class="cpe-hint">No layouts in the Library yet. Keep this one there to start others from it.</p>';
        p.addEventListener('click', function (e) {
          var b = e.target.closest('[data-pr]');
          if (!b) return;
          var x = list[Number(b.getAttribute('data-pr'))];
          closePop();
          var fresh = canon(parse(x && x.layout_json, null));
          if (!fresh.rows.length) { toast('That layout is empty or damaged, so nothing changed.', true); return; }
          fresh.rows.forEach(reId);
          if (edit(function (lay) { lay.rows = fresh.rows; UI.sel = null; UI.insp = 'add'; })) toast('Now using “' + x.name + '”.', false, undo);
        });
      }).catch(function () {
        if (popEl === p) p.firstChild.innerHTML = '<p class="cpe-hint">The Library didn’t load. Try again in a moment.</p>';
      });
    }
    function keepMenu(btn) {
      var nm = (S.plural || 'Page') + ' layout';
      var p = openPop(btn, '<div class="cpe-keep"><label class="cpe-lab" for="cpe-keep-n">Name for this layout</label>' +
        '<input class="cpe-inp" id="cpe-keep-n" maxlength="100" value="' + esc(nm) + '">' +
        '<span class="cpe-hint">A copy goes in the Library now, where any category can start from it. It doesn’t wait for Save.</span>' +
        '<div class="cpe-acts cpe-end"><button class="btn-primary btn-sm" type="button" data-keep>Keep it</button></div></div>', 'right');
      var inp = p.querySelector('#cpe-keep-n');
      inp.select();
      function go() {
        var name = inp.value.trim() || nm;
        closePop();
        Chronicle.apiFetch(S.presets, { method: 'POST', body: { name: name, description: 'Kept from ' + (S.plural || 'a category') + '.', layout_json: JSON.stringify(L), icon: 'fa-table-columns' } })
          .then(function (r) { if (!r.ok) throw new Error(String(r.status)); toast('Kept “' + name + '” in the Library.'); })
          .catch(function () { toast('That layout wasn’t kept. Try again in a moment.', true); });
      }
      p.querySelector('[data-keep]').addEventListener('click', go);
      inp.addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); go(); } });
    }

    /* ---------- Drag to move (widgets on the page, and new ones from the panel) ---------- */
    function pressStart(e, src) {
      if (e.button !== 0) return;
      var sx = e.clientX, sy = e.clientY, started = false;
      if (e.pointerType === 'mouse') e.preventDefault();
      function mv(ev) {
        if (!started && Math.hypot(ev.clientX - sx, ev.clientY - sy) > 5) { started = true; dragBegin(src, ev); }
        if (started) dragMove(ev);
      }
      function up() {
        off();
        if (started) dragEnd();
        else if (src.from === 'gal' && UI.addAt) addTile(UI.addAt, src.key);
        else if (src.from === 'gal') addWidget(src.key);
        else select({ kind: src.from, id: src.id });
      }
      function off() {
        document.removeEventListener('pointermove', mv);
        document.removeEventListener('pointerup', up);
        document.removeEventListener('pointercancel', cancel);
        document.removeEventListener('keydown', escKey);
      }
      function cancel() { off(); if (started) dragCancel(); }
      function escKey(ev) { if (ev.key === 'Escape') cancel(); }
      document.addEventListener('pointermove', mv);
      document.addEventListener('pointerup', up);
      document.addEventListener('pointercancel', cancel);
      document.addEventListener('keydown', escKey);
    }
    function dragBegin(src, ev) {
      if (!(popEl && popEl.hasAttribute('data-keep'))) closePop();
      var r = src.el.getBoundingClientRect(), g;
      if (src.from === 'gal') {
        g = document.createElement('div');
        g.className = 'cpe-ghost cpe-ph-card';
        g.innerHTML = ic(src.m.icon) + '<span><b></b></span>';
        g.querySelector('b').textContent = src.m.name;
        g.style.width = '220px';
      } else {
        g = src.el.cloneNode(true);
        g.classList.add('cpe-ghost');
        g.classList.remove('sel');
        g.style.width = r.width + 'px';
        g.style.height = Math.min(r.height, 260) + 'px';
        src.el.classList.add('dragging');
      }
      document.body.appendChild(g);
      DRAG = { src: src, ghost: g, ox: src.from === 'gal' ? 20 : ev.clientX - r.left, oy: src.from === 'gal' ? 18 : ev.clientY - r.top, target: null, key: '' };
      document.body.classList.add('cpe-dragging');
      if (window.getSelection) window.getSelection().removeAllRanges();
      if (window.innerWidth <= 1100) { UI.sheet = false; var ins = $('.cpe-insp'); if (ins) ins.classList.remove('open'); }
    }
    function clearMarks() {
      $$('.cpe-ph').forEach(function (p) { p.remove(); });
      $$('.drop-ok').forEach(function (p) { p.classList.remove('drop-ok'); });
      $$('.no-drop').forEach(function (p) { p.classList.remove('no-drop'); var w = $('.cpe-why', p); if (w) w.remove(); });
      $$('.cpe-ins.hot').forEach(function (p) { p.classList.remove('hot'); var b = $('button', p); if (b) b.innerHTML = ic('fa-plus') + ' Add section'; });
      $$('.cpe-empty').forEach(function (p) { p.style.display = ''; });
      $$('.near').forEach(function (p) { p.classList.remove('near'); });
    }
    function refuse(sec, msg) {
      sec.classList.add('no-drop');
      var w = document.createElement('span');
      w.className = 'cpe-why';
      w.textContent = msg;
      sec.appendChild(w);
    }
    function dragMove(ev) {
      var D = DRAG;
      if (!D) return;
      D.last = [ev.clientX, ev.clientY];
      D.ghost.style.left = (ev.clientX - D.ox) + 'px';
      D.ghost.style.top = (ev.clientY - D.oy) + 'px';
      var cv = $('.cpe-canvas'), cr = cv.getBoundingClientRect();
      if (ev.clientY > cr.bottom - 40) cv.scrollTop += 12; else if (ev.clientY < cr.top + 40) cv.scrollTop -= 12;
      var under = document.elementFromPoint(ev.clientX, ev.clientY);
      if (!under || !el.contains(under)) { clearTimeout(D.wait); D.want = ''; if (D.key) { D.key = ''; D.target = null; clearMarks(); } return; }
      // Over the gap itself the drop stays where it is, so the tiles it moved
      // can't move it back.
      if (under.closest('.cpe-ph')) { clearTimeout(D.wait); D.want = D.key; return; }
      // Moving the gap moves the widgets under a still pointer; until the
      // pointer itself moves on, the gap stays.
      if (D.key && D.at && Math.hypot(ev.clientX - D.at[0], ev.clientY - D.at[1]) < NUDGE) { clearTimeout(D.wait); D.want = D.key; return; }
      var m = D.src.m, tileable = m.sizes.length > 0, t = null, key = '';
      var srcList = D.src.from !== 'gal' ? findItem(D.src.id) : null;
      var ins = under.closest('.cpe-ins'), col = under.closest('.cpe-col'), tiles = under.closest('.cpe-tiles'), sec = under.closest('.cpe-sec');
      if (ins) { t = { kind: 'newsec', at: Number(ins.getAttribute('data-at')) }; key = 'n' + t.at; }
      else if (col) {
        var kids = $$(':scope > .cpe-blk', col).filter(function (k) { return k !== D.src.el; }), idx = kids.length;
        for (var i = 0; i < kids.length; i++) { var kr = kids[i].getBoundingClientRect(); if (ev.clientY < kr.top + kr.height / 2) { idx = i; break; } }
        var c = findRow(col.getAttribute('data-rid')).row.columns.filter(function (x) { return x.id === col.getAttribute('data-cid'); })[0];
        var into = srcList && srcList.col && srcList.col.id === c.id;
        if (!into && c.blocks.length >= MAX_BLOCKS) { t = { kind: 'bad', el: sec, msg: 'This column is full (' + MAX_BLOCKS + ' widgets).' }; key = 'x' + c.id; }
        else { t = { kind: 'col', rid: col.getAttribute('data-rid'), cid: c.id, index: idx, el: col, before: kids[idx] || null }; key = 'c' + c.id + ':' + idx; }
      } else if (tiles) {
        var row = findRow(tiles.getAttribute('data-tiles')).row, same = srcList && srcList.row.id === row.id;
        if (!tileable) { t = { kind: 'bad', el: sec, msg: m.name + ' can’t be a tile. Drop it in a columns section.' }; key = 'x' + row.id; }
        else if (!same && row.tiles.length >= MAX_TILES) { t = { kind: 'bad', el: sec, msg: 'This tiles section is full (' + MAX_TILES + ' tiles).' }; key = 'x' + row.id; }
        else {
          var ks = $$(':scope > .cpe-tile', tiles).filter(function (k) { return k !== D.src.el; }), ix = ks.length, best = 1e9;
          ks.forEach(function (k, j) {
            var kr2 = k.getBoundingClientRect(), cx = kr2.left + kr2.width / 2, cy = kr2.top + kr2.height / 2, d = Math.hypot(ev.clientX - cx, (ev.clientY - cy) * 1.4);
            if (d < best) { best = d; ix = ev.clientX < cx ? j : j + 1; }
          });
          t = { kind: 'tiles', rid: row.id, index: ix, el: tiles, before: ks[ix] || $(':scope > .cpe-addt', tiles) }; key = 't' + row.id + ':' + ix;
        }
      }
      if (key === D.key) { clearTimeout(D.wait); D.want = key; return; }
      // A new spot takes only once the pointer rests on it a moment, so
      // passing over tiles doesn't shuffle them.
      if (key === D.want) return;
      D.want = key;
      clearTimeout(D.wait);
      D.wait = setTimeout(function () { if (DRAG === D) { D.at = D.last; mark(D, t, key); } }, t && t.kind !== 'bad' && D.key ? SETTLE : 0);
    }
    function mark(D, t, key) {
      var m = D.src.m, tileable = m.sizes.length > 0, srcList = D.src.from !== 'gal' ? findItem(D.src.id) : null;
      var before = snap();
      D.key = key;
      D.target = t && t.kind !== 'bad' ? t : null;
      clearMarks();
      if (!t) return;
      if (t.kind === 'bad') { refuse(t.el, t.msg); return; }
      if (t.kind === 'newsec') {
        var ie = $('.cpe-ins[data-at="' + t.at + '"]');
        ie.classList.add('hot');
        $('button', ie).innerHTML = ic('fa-plus') + ' New ' + (tileable ? 'tiles' : 'full-width') + ' section here';
        return;
      }
      var ph = document.createElement('div');
      var empty = $('.cpe-empty', t.el);
      if (empty) empty.style.display = 'none';
      if (t.kind === 'col') { ph.className = 'cpe-ph line'; t.el.classList.add('drop-ok'); }
      else {
        var sz = sizeOf(D.src.from === 'tile' && srcList ? srcList.item.size : m.sizes[0]);
        ph.className = 'cpe-ph';
        ph.style.gridColumn = 'span ' + Math.min(sz.w, gridCols());
        ph.style.gridRow = 'span ' + sz.h;
      }
      t.el.insertBefore(ph, t.before);
      // The widgets either side of the gap light up, and the ones it pushed
      // glide to their new places.
      [ph.previousElementSibling, ph.nextElementSibling].forEach(function (n) {
        if (n && n !== D.src.el && n.matches('.cpe-tile, .cpe-blk')) n.classList.add('near');
      });
      glide(before);
    }
    function dragDone() {
      var D = DRAG;
      if (!D) return null;
      DRAG = null;
      clearTimeout(D.wait);
      document.body.classList.remove('cpe-dragging');
      D.ghost.remove();
      clearMarks();
      if (D.src.el) D.src.el.classList.remove('dragging');
      return D;
    }
    function dragCancel() { if (DRAG) { dragDone(); render(); } }
    function dragEnd() {
      var ph = $('.cpe-ph:not(.line)');
      UI.glideFrom = ph ? ph.getBoundingClientRect() : null;
      // Where things are on screen, gap and all, so the drop glides from there.
      UI.glideBefore = snap();
      var D = dragDone();
      if (!D || !alive) return;
      var t = D.target;
      if (!t) { UI.glideFrom = UI.glideBefore = null; render(); return; }
      var moved = null, m = D.src.m;
      var ok = edit(function (lay) {
        var item;
        if (D.src.from === 'gal') {
          if (refuseSingleton(m, lay)) return false;
          item = newItem(m, false);
        } else {
          var f = findItem(D.src.id, lay);
          f.list.splice(f.i, 1);
          item = f.item;
        }
        if (t.kind === 'newsec') {
          if (lay.rows.length >= MAX_ROWS) { toast('A page holds ' + MAX_ROWS + ' sections at most.', true); return false; }
          var row = m.sizes.length ? tilesRow([{ id: item.id, type: item.type, size: item.size && m.sizes.indexOf(item.size) >= 0 ? item.size : m.sizes[0], config: item.config }])
            : colsRow([12], [[{ id: item.id, type: item.type, config: item.config }]]);
          lay.rows.splice(t.at, 0, row);
        } else if (t.kind === 'col') {
          var c = findRow(t.rid, lay).row.columns.filter(function (x) { return x.id === t.cid; })[0];
          c.blocks.splice(t.index, 0, { id: item.id, type: item.type, config: item.config });
        } else {
          var tr = findRow(t.rid, lay).row;
          var sz = item.size && m.sizes.indexOf(item.size) >= 0 ? item.size : m.sizes[0];
          tr.tiles.splice(t.index, 0, { id: item.id, type: item.type, size: sz, config: item.config });
        }
        moved = item.id;
        var nf = findItem(moved, lay);
        UI.sel = { kind: nf.kind, id: moved };
        UI.insp = 'sel';
      });
      UI.glideFrom = null;
      UI.glideBefore = null;
      if (ok && moved) { glow(moved); if (D.src.from === 'gal') toast('Added ' + m.name + '.'); }
    }

    /* ---------- Pull a tile's corner to resize ---------- */
    function resizeStart(e) {
      e.stopPropagation();
      e.preventDefault();
      var id = e.target.getAttribute('data-rsz'), tileEl = e.target.closest('.cpe-tile'), grid = tileEl.parentElement;
      var f = findItem(id), m = meta(f.item.type, f.item.config), start = sizeOf(f.item.size), cols = gridCols();
      var gr = grid.getBoundingClientRect(), gap = 12, colW = (gr.width - gap * (cols - 1)) / cols, rowH = 104 + gap;
      var sx = e.clientX, sy = e.clientY, cur = f.item.size;
      document.body.classList.add('cpe-dragging');
      var lab = document.createElement('span');
      lab.className = 'cpe-size';
      tileEl.appendChild(lab);
      tileEl.classList.add('resizing');
      function show(sz) {
        var z = sizeOf(sz), before = snap();
        tileEl.style.gridColumn = 'span ' + Math.min(z.w, cols);
        tileEl.style.gridRow = 'span ' + z.h;
        lab.textContent = z.w + ' × ' + z.h;
        glide(before);
      }
      show(cur);
      function mv(ev) {
        var ww = Math.max(1, Math.min(cols, start.w + Math.round((ev.clientX - sx) / (colW + gap))));
        var hh = Math.max(1, Math.min(6, start.h + Math.round((ev.clientY - sy) / rowH)));
        var best = cur, bd = 1e9;
        m.sizes.forEach(function (s) { var z = sizeOf(s), d = Math.abs(Math.min(z.w, cols) - ww) * 2 + Math.abs(z.h - hh); if (d < bd) { bd = d; best = s; } });
        if (best !== cur) { cur = best; show(cur); }
      }
      function up() {
        document.removeEventListener('pointermove', mv);
        document.removeEventListener('pointerup', up);
        document.body.classList.remove('cpe-dragging');
        if (cur !== f.item.size) edit(function (lay) { findItem(id, lay).item.size = cur; UI.sel = { kind: 'tile', id: id }; UI.insp = 'sel'; });
        else select({ kind: 'tile', id: id });
      }
      document.addEventListener('pointermove', mv);
      document.addEventListener('pointerup', up);
    }

    /* ---------- Drag the line between columns ---------- */
    function dividerStart(e) {
      e.stopPropagation();
      e.preventDefault();
      var d = e.target.closest('.cpe-div'), k = Number(d.getAttribute('data-div')), rid = d.getAttribute('data-rid');
      var row = findRow(rid).row, box = d.parentElement, br = box.getBoundingClientRect();
      var widths = row.columns.map(function (c) { return c.width; }), chip = $('.cpe-chip', d);
      d.classList.add('active');
      document.body.classList.add('cpe-dragging');
      var before = 0;
      for (var i = 0; i < k; i++) before += widths[i];
      var after = before + widths[k] + widths[k + 1];
      function apply(w) {
        box.style.gridTemplateColumns = w.map(function (x) { return 'minmax(0,' + x + 'fr)'; }).join(' ');
        var s = 0, n = w.length;
        $$('.cpe-div', box).forEach(function (dv, j) { s += w[j]; dv.style.left = divLeft(s, j, n); });
        chip.textContent = w.join(' | ');
      }
      apply(widths);
      function mv(ev) {
        var edge = Math.round((ev.clientX - br.left) / br.width * 12);
        edge = Math.max(before + 2, Math.min(after - 2, edge));
        var w = widths.slice();
        w[k] = edge - before;
        w[k + 1] = after - edge;
        if (w.join() !== widths.join()) { widths = w; apply(widths); }
      }
      function up() {
        document.removeEventListener('pointermove', mv);
        document.removeEventListener('pointerup', up);
        d.classList.remove('active');
        document.body.classList.remove('cpe-dragging');
        var old = row.columns.map(function (c) { return c.width; });
        if (old.join() !== widths.join()) edit(function (lay) { findRow(rid, lay).row.columns.forEach(function (c, j) { c.width = widths[j]; }); UI.sel = { kind: 'sec', id: rid }; UI.insp = 'sel'; });
        else render();
      }
      document.addEventListener('pointermove', mv);
      document.addEventListener('pointerup', up);
    }

    /* ---------- Wiring ---------- */
    on(el, 'click', function (e) {
      var b = e.target.closest('[data-act]');
      if (b && el.contains(b) && b.tagName !== 'SELECT' && b.tagName !== 'INPUT') {
        var act = b.getAttribute('data-act'), v = b.getAttribute('data-v');
        switch (act) {
          case 'undo': undo(); return;
          case 'redo': if (Chronicle.customizeRedo) Chronicle.customizeRedo(); return;
          case 'dev': UI.device = v; render(); return;
          case 'as': UI.viewAs = v; render(); return;
          case 'look': UI.look = v === 'annals' ? 'annals' : 'standard'; showLook(UI.look); render(); fadeFrame(); return;
          case 'read': readMode(!UI.reading); return;
          case 'insp': UI.insp = v; render(); return;
          case 'sheet': UI.sheet = !UI.sheet; $('.cpe-insp').classList.toggle('open', UI.sheet); return;
          case 'galtiles': UI.galTiles = !UI.galTiles; render(); return;
          case 'addsec': e.stopPropagation(); sectionMenu(b, Number(b.getAttribute('data-at'))); return;
          case 'start': presetsMenu(b); return;
          case 'peek': e.stopPropagation(); peek(b, b.getAttribute('data-k')); return;
          case 'keep': keepMenu(b); return;
          case 'vis': UI.keepInsp = true; edit(function (lay) { var c = cfgOf(findSel(lay).item); if (v) c.visibility = v; else delete c.visibility; }); return;
          case 'size': UI.keepInsp = true; edit(function (lay) { findSel(lay).item.size = v; }); return;
          case 'height': UI.keepInsp = true; edit(function (lay) { var c = cfgOf(findSel(lay).item); if (v) c.minHeight = v; else delete c.minHeight; }); return;
          case 'mv': UI.keepInsp = true; moveSel(Number(v)); return;
          case 'tmv': UI.sel = { kind: 'tile', id: b.getAttribute('data-id') }; UI.insp = 'sel'; moveSel(Number(v)); return;
          case 'trm': UI.sel = { kind: 'tile', id: b.getAttribute('data-id') }; removeSel(); return;
          case 'tat': e.stopPropagation(); aimAdd(b.getAttribute('data-rid')); return;
          case 'mvsec': UI.keepInsp = true; moveSection(Number(v)); return;
          case 'dup': case 'dupsec': duplicateSel(); return;
          case 'rm': removeSel(); return;
          case 'unpack': unpack(); return;
          case 'preset': UI.keepInsp = true; setColumns(v.split(',').map(Number)); return;
        }
      }
      if (UI.reading) return;
      var tag = e.target.closest('[data-selsec]');
      if (tag) { e.stopPropagation(); select({ kind: 'sec', id: tag.getAttribute('data-selsec') }); return; }
      if (e.target.closest('.cpe-canvas') && !e.target.closest('.cpe-blk, .cpe-tile, .cpe-ins, .cpe-div')) {
        var sec = e.target.closest('.cpe-sec');
        if (sec) select({ kind: 'sec', id: sec.getAttribute('data-rid') });
        else if (UI.sel) select(null);
      }
    });
    on(el, 'pointerdown', function (e) {
      if (UI.reading) return;
      if (e.target.closest('.cpe-tbar button, .cpe-addt')) return;
      if (e.target.closest('.cpe-rsz')) { resizeStart(e); return; }
      if (e.target.closest('.cpe-div')) { dividerStart(e); return; }
      var g = e.target.closest('.cpe-gw');
      if (g && !g.disabled) { var gm = W[g.getAttribute('data-gw')]; if (gm) pressStart(e, { from: 'gal', key: gm.key, m: gm, el: g }); return; }
      var it = e.target.closest('.cpe-blk, .cpe-tile');
      if (it && el.contains(it)) {
        var f = findItem(it.getAttribute('data-id'));
        if (f) pressStart(e, { from: f.kind, id: f.item.id, m: meta(f.item.type, f.item.config), el: it });
      }
    });
    on(el, 'keydown', function (e) {
      if (UI.reading) return;
      if (e.target.closest && e.target.closest('input, textarea, select')) return;
      var g = e.target.closest && e.target.closest('.cpe-gw');
      if (g && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); if (UI.addAt) addTile(UI.addAt, g.getAttribute('data-gw')); else addWidget(g.getAttribute('data-gw')); return; }
      var it = e.target.closest && e.target.closest('.cpe-blk, .cpe-tile');
      if (it && e.key === 'Enter') { e.preventDefault(); select({ kind: it.getAttribute('data-kind'), id: it.getAttribute('data-id') }); return; }
      if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown') && UI.sel && UI.sel.kind !== 'sec') {
        e.preventDefault();
        var id = UI.sel.id;
        moveSel(e.key === 'ArrowUp' ? -1 : 1);
        var n = el.querySelector('[data-id="' + CSS.escape(id) + '"]');
        if (n) n.focus();
      }
      if (e.key === 'Escape' && UI.sel && !popEl) select(null);
    });
    // Escape closes the peek wherever focus is, since clicks elsewhere don't.
    on(document, 'keydown', function (e) {
      if (e.key === 'Escape' && popEl && popEl.hasAttribute('data-panel') && !DRAG) closePop();
    });
    on(el, 'input', function (e) {
      if (e.target.id === 'cpe-q') { UI.galQ = e.target.value; render(); }
    });
    on(el, 'change', function (e) {
      var t = e.target;
      if (t.id === 'cpe-page') { showWith(t.value); return; }
      var key = t.getAttribute('data-cfg');
      if (key == null) return;
      var f = findSel();
      if (!f || !f.item) return;
      var fd = meta(f.item.type, f.item.config).fields.filter(function (x) { return x.key === key; })[0];
      UI.keepInsp = true;
      setConfig(key, t.value, fd);
    });
    on(document, 'chronicle:customize-draft', function () {
      L = current();
      if (UI.sel && !findSel()) { UI.sel = null; UI.insp = 'add'; }
      if (UI.addAt && !findRow(UI.addAt)) UI.addAt = null;
      render();
    });
    on(window, 'resize', fit);

    render();
    showLook(UI.look);
    loadPalette();
    return function destroy() {
      alive = false;
      clearTimeout(PV.timer);
      closePop();
      if (DRAG) { DRAG.ghost.remove(); DRAG = null; document.body.classList.remove('cpe-dragging'); }
      showLook(LOOK0);
      listeners.forEach(function (l) { l[0].removeEventListener(l[1], l[2], l[3]); });
    };
  }

  Chronicle.register('customize-page-editor', {
    init: function (el) { el._cpeDestroy = mount(el); },
    destroy: function (el) {
      if (el._cpeDestroy) el._cpeDestroy();
      el._cpeDestroy = null;
    }
  });

})();
