/**
 * own_entry_editor.js — the player's own entry, written where they build.
 *
 * A Director can let one player write one entry of their own for a pick list
 * (an ancestry, a kit). This is the editor for it, shared by the hero creator,
 * the Characters page's hero builder and the sheet's pick list, so a player
 * never has to leave the screen they are choosing on. The Directors' "Your own
 * entries" page (own_entries.js) uses the same pure parts below, so a saved
 * entry means the same thing wherever it was written.
 *
 * It talks only to the systems plugin's entry API (/campaigns/:id/system-
 * entries), which enforces everything: who may write, the locked points budget,
 * the lists' shape, draft against handed in. This file draws the form the
 * server's rules allow and sends what the player wrote.
 *
 * The parts:
 *   shapeOf, draftOf, propsOf, problems, tidy   pure: what the form holds and
 *                                               what it sends
 *   Editor                                      one editor, mounted into a
 *                                               body and a foot the host owns
 *
 * Its look comes from the host's tokens (--hc-* in the creator, the site's own
 * elsewhere) through own_entry_editor.css, which this script loads itself, so
 * a host never needs to know about it. Motion follows html[data-motion].
 *
 * Loaded before the script that uses it (the widget registry lists it first);
 * exposes window.ChronicleOwnEntry.
 */
(function () {
  'use strict';

  var DIRTY = 'own-entry-editor';
  // Read now: document.currentScript is gone once the script has run.
  var HERE = document.currentScript && document.currentScript.src || '';

  function esc(v) {
    if (window.Chronicle && Chronicle.escapeHtml) return Chronicle.escapeHtml(v);
    return String(v == null ? '' : v).replace(/[&<>"']/g, function (c) { return '&#' + c.charCodeAt(0) + ';'; });
  }
  // The campaign's own word for its game master, already HTML-safe.
  function gm() { return window.Chronicle && Chronicle.gmTerm ? Chronicle.gmTerm() : 'GM'; }
  function words(key) {
    var s = String(key).replace(/_display$/, '').replace(/[_-]+/g, ' ').trim();
    return s.charAt(0).toUpperCase() + s.slice(1);
  }
  function lower(s) { return String(s || '').toLowerCase(); }
  function an(w) { return /^[aeiou]/i.test(w) ? 'an' : 'a'; }
  function sing(label) { return lower(label).replace(/ies$/, 'y').replace(/s$/, ''); }
  // A fact's label becomes the property key the creator reads ("Size" → size).
  function keyOf(label) {
    var k = lower(label).replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
    if (!/^[a-z]/.test(k)) k = 'f_' + k;
    return k.slice(0, 40);
  }
  function isFact(v) { return typeof v === 'number' || (typeof v === 'string' && v.length > 0 && v.length <= 40); }
  // Numbers stay numbers so the creator can do sums with them ("3" points).
  function factValue(v) { v = v.trim(); return /^-?\d+(\.\d+)?$/.test(v) ? Number(v) : v; }

  // --- The card's border ----------------------------------------------------

  var CARD_B = 'card_border', CARD_C = 'card_colour';
  var HEX6 = /^#[0-9a-f]{6}$/i;
  function has(o, k) { return !!o && typeof o === 'object' && Object.prototype.hasOwnProperty.call(o, k); }
  function cardKey(k) { return k === CARD_B || k === CARD_C; }

  // The two details as the form holds them: the look when the border is in
  // the library (the stored colour when it is six hex digits, else the
  // border's own), or what was stored when it is not usable here, so a save
  // can write it back untouched. borders is the picker's border list; without
  // one the form has no such field and the caller carries both as stored.
  function lookFromProps(props, borders) {
    var b = props[CARD_B], c = props[CARD_C];
    if (b === undefined && c === undefined) return { look: null, kept: null };
    if (borders && typeof b === 'string' && has(borders, b)) {
      var base = borders[b] && HEX6.test(borders[b].colour) ? String(borders[b].colour).toLowerCase() : '#888888';
      var col = typeof c === 'string' && HEX6.test(c) ? c.toLowerCase() : base;
      return { look: { border: b, colour: col, own: col === base }, kept: null };
    }
    var kept = {};
    if (b !== undefined) kept[CARD_B] = b;
    if (c !== undefined) kept[CARD_C] = c;
    return { look: null, kept: kept };
  }

  // What a mount hands over: the library the cards are drawn from and the
  // picker's lists. Read from the nearest element that carries each, else the
  // page's. Null (no field) when either is missing or unreadable, or the two
  // scripts are not on the page.
  function readJSON(el, attr) {
    var m = (el && el.closest && el.closest('[' + attr + ']')) || document.querySelector('[' + attr + ']');
    if (!m) return null;
    try {
      var v = JSON.parse(m.getAttribute(attr));
      return v && typeof v === 'object' && !Array.isArray(v) ? v : null;
    } catch (e) { return null; }
  }
  function cardLib(el) {
    if (typeof window.ChronicleBorderPicker !== 'function' || typeof window.ChronicleHeroBorders !== 'function') return null;
    var lib = readJSON(el, 'data-borders'), pick = readJSON(el, 'data-border-picker');
    if (!lib || !pick || !pick.borders || typeof pick.borders !== 'object' || !Array.isArray(pick.groups)) return null;
    return { lib: lib, pick: pick };
  }
  function cardMap(el) { var k = cardLib(el); return k ? k.pick.borders : null; }

  // --- What the form holds -------------------------------------------------

  // The facts a system's own entries share: scalar properties present on at
  // least half of them, budget keys and display twins aside, most common
  // first, at most four. A new entry starts with these lines.
  function suggestedFacts(pkg) {
    var count = {};
    pkg.forEach(function (c) {
      Object.keys(c.properties || {}).forEach(function (k) {
        if (/(_display|_points)$/.test(k) || cardKey(k) || !isFact(c.properties[k])) return;
        count[k] = (count[k] || 0) + 1;
      });
    });
    return Object.keys(count).filter(function (k) { return count[k] * 2 >= pkg.length; })
      .sort(function (a, b) { return count[b] - count[a] || (a < b ? -1 : 1); }).slice(0, 4);
  }
  // A list of named items, as the creator reads one.
  function namedList(v) {
    return Array.isArray(v) && v.length > 0 && v.every(function (it) { return it && typeof it === 'object' && typeof it.name === 'string'; });
  }
  // A plain list of short words (a quick build), edited one per line.
  function wordList(v) {
    return Array.isArray(v) && v.length > 0 && v.every(function (w) { return typeof w === 'string'; });
  }
  // A fact too long for one line is written in a box (a benefit, a drawback).
  function longFact(v) { return typeof v === 'string' && (v.length > 120 || v.indexOf('\n') >= 0); }
  // Where the words came from is the page's credit, not a fact to edit; it
  // goes back as loaded and is not named among the kept parts.
  function creditKey(k) { return k === 'provenance' || k === 'provenance_display'; }

  // The shape a system's own entries share, so a player's or Director's entry
  // can carry the same parts: scalar facts, lists of named items that at least
  // half of them have (a list whose items cost points is one a hero buys
  // from), and the points budget, read as the creator does from a "*_points"
  // number.
  function shapeOf(pkg) {
    var lists = {}, budgets = {};
    pkg.forEach(function (c) {
      var p = c.properties || {};
      Object.keys(p).forEach(function (k) {
        if (namedList(p[k])) {
          var l = lists[k] || (lists[k] = { n: 0, buy: 0 });
          l.n++;
          if (p[k].every(function (it) { return typeof it.cost === 'number'; })) l.buy++;
        } else if (/_points$/.test(k) && typeof p[k] === 'number' && p[k] > 0) {
          var b = budgets[k] || (budgets[k] = {});
          b[p[k]] = (b[p[k]] || 0) + 1;
        }
      });
    });
    var keys = Object.keys(lists).filter(function (k) { return lists[k].n * 2 >= pkg.length; }).sort();
    // What every hero gets comes before what they buy, as in the creator.
    var shape = { facts: suggestedFacts(pkg), lists: keys.map(function (k) {
      return { key: k, label: words(k), buy: lists[k].buy * 2 >= lists[k].n };
    }).sort(function (a, b) { return a.buy - b.buy; }), budgetKey: '', budget: 0 };
    var bk = Object.keys(budgets).sort()[0];
    if (bk && shape.lists.some(function (l) { return l.buy; })) {
      shape.budgetKey = bk;
      shape.budget = +Object.keys(budgets[bk]).sort(function (a, b) { return budgets[bk][b] - budgets[bk][a]; })[0];
    }
    return shape;
  }

  // An item as the form edits it; orig is the item as loaded, whose other
  // keys go back with it while its name stays the same.
  function itemOf(it) {
    var text = typeof it.description === 'string' || it.description == null;
    return { name: String(it.name || ''), cost: typeof it.cost === 'number' ? it.cost : 1,
      description: text ? String(it.description || '') : '', orig: it };
  }
  // The keys of a loaded item the form does not edit.
  function extraKeys(orig) {
    if (!orig || typeof orig !== 'object') return [];
    return Object.keys(orig).filter(function (k) {
      return k !== 'name' && k !== 'cost' && !(k === 'description' && (typeof orig.description === 'string' || orig.description == null));
    });
  }
  // Whether an entry is a player's, not yet accepted.
  function isDraft(e) { return !!e && !!e.status && e.status !== 'accepted'; }

  // A draft holds facts as label/value lines, each shaped list by its key,
  // word lists as lines, and the points budget. A list the system doesn't
  // share is edited too (extraLists); anything else the form cannot edit
  // (an object, a yes/no, an empty list) is carried as loaded.
  function draftOf(e, f, budget0, borders) {
    var lists = {};
    f.lists.forEach(function (l) { lists[l.key] = []; });
    if (!e) {
      return { id: '', name: '', summary: '', text: '', who: 'everyone', budget: budget0 || f.budget, lists: lists, status: '', note: '', reviewNote: '', look: null,
        facts: f.facts.map(function (k) { return { label: words(k), value: '', key: k }; }), wordLists: [], extraLists: [], carried: {} };
    }
    var props = e.properties || {}, facts = [], wls = [], extra = [], carried = {}, budget = f.budget;
    var shared = {};
    f.lists.forEach(function (l) { shared[l.key] = true; });
    var lk = lookFromProps(props, borders);
    Object.keys(props).forEach(function (k) {
      var v = props[k];
      // With the Card border field the two details are its look; without it they
      // go back as stored, never as free-text facts.
      if (cardKey(k)) { if (!borders) carried[k] = v; }
      else if (creditKey(k)) carried[k] = v;
      else if (namedList(v)) {
        lists[k] = v.map(itemOf);
        if (!shared[k]) extra.push({ key: k, label: words(k), buy: v.every(function (it) { return typeof it.cost === 'number'; }) });
      } else if (k === f.budgetKey && typeof v === 'number') budget = v;
      else if (wordList(v)) wls.push({ key: k, label: words(k), text: v.join('\n'), orig: v });
      else if (typeof v === 'number' || typeof v === 'string') facts.push({ label: words(k), value: String(v), key: k, orig: v, long: longFact(v) });
      else carried[k] = v;
    });
    var who = e.visibility || 'everyone';
    // A player's draft is only theirs until accepted; accepting it offers
    // their character, or everyone, never "Directors only" by default.
    if (isDraft(e)) who = e.visibility === 'character' ? 'character' : 'everyone';
    return { id: e.id, name: e.name || '', summary: e.summary || '', text: e.description || '', who: who,
      forName: e.visibility === 'character' ? (e.forName || '') : undefined,
      look: lk.look, lookKept: borders && lk.kept || undefined, lookTouched: false,
      budget: budget0 || budget, lists: lists, facts: facts, wordLists: wls, extraLists: extra, carried: carried,
      standsFor: e.standsFor || '', standsForName: e.standsForName || '', savedWho: e.visibility || 'everyone',
      status: e.status || 'accepted', reviewNote: e.reviewNote || '', note: '', by: e.createdBy || '' };
  }
  // Every list the form edits for a draft: the system's shared lists, then
  // the entry's own.
  function listsOf(d, f) { return f.lists.concat(d.extraLists || []); }

  // propsOf builds the properties sent to Chronicle from a draft: what the
  // form could not edit first, exactly as loaded, then facts under snake_case
  // keys (numbers as numbers, an untouched value as it was), the budget, word
  // lists, and each list with only its named items, an item keeping its
  // other keys while its name is unchanged.
  function propsOf(d, f) {
    var props = {}, seen = {};
    Object.keys(d.carried || {}).forEach(function (k) { props[k] = d.carried[k]; });
    for (var i = 0; i < d.facts.length; i++) {
      var p = d.facts[i];
      if (!p.label.trim() || !p.value.trim()) continue;
      // An untouched label keeps the key it came with, so a fact the system
      // spells differently is not renamed by a save.
      var k = (p.key && words(p.key) === p.label) ? p.key : keyOf(p.label);
      if (cardKey(k)) return { error: '“' + p.label.trim() + '” is set in the Card border field, not as a fact. Rename or remove this fact.' };
      if (seen[k]) return { error: 'Two facts are both called “' + p.label.trim() + '”. Rename or remove one.' };
      seen[k] = true;
      props[k] = (p.orig !== undefined && k === p.key && p.value === String(p.orig)) ? p.orig : factValue(p.value);
    }
    // A border the form could not show goes back as stored until the writer picks.
    if (d.lookKept && !d.lookTouched) Object.keys(d.lookKept).forEach(function (k) { props[k] = d.lookKept[k]; });
    else if (d.look) {
      props[CARD_B] = d.look.border;
      if (!d.look.own) props[CARD_C] = String(d.look.colour).toLowerCase();
    }
    if (f.budgetKey) props[f.budgetKey] = d.budget;
    (d.wordLists || []).forEach(function (w) {
      var items = w.text.split('\n').map(function (x) { return x.trim(); }).filter(Boolean);
      if (!items.length) return;
      props[w.key] = w.orig && w.orig.join('\n') === items.join('\n') ? w.orig : items;
    });
    var buyOf = {};
    listsOf(d, f).forEach(function (l) { buyOf[l.key] = l.buy; });
    Object.keys(d.lists).forEach(function (key) {
      var items = d.lists[key].filter(function (it) { return it.name.trim(); }).map(function (it) {
        var o = {}, same = it.orig && String(it.orig.name || '').trim() === it.name.trim();
        if (same) extraKeys(it.orig).forEach(function (x) { o[x] = it.orig[x]; });
        o.name = it.name.trim();
        if (buyOf[key]) o.cost = it.cost;
        else if (same && typeof it.orig.cost === 'number') o.cost = it.orig.cost;
        if (it.description.trim()) o.description = it.description.trim();
        return o;
      });
      if (items.length) props[key] = items;
    });
    return { props: props };
  }

  // keptLine names the parts the form keeps but cannot edit, in one quiet
  // line, or '' when there are none.
  function keptLine(d, f) {
    var parts = Object.keys(d.carried || {}).filter(function (k) { return !creditKey(k); }).map(words);
    var traits = [];
    listsOf(d, f).forEach(function (l) {
      (d.lists[l.key] || []).forEach(function (it) {
        if (extraKeys(it.orig).length && String(it.orig.name || '').trim() === it.name.trim()) traits.push(it.name.trim());
      });
    });
    if (traits.length) parts.push('the rules attached to ' + (traits.length > 3 ? traits.slice(0, 3).join(', ') + ' and ' + (traits.length - 3) + ' more' : traits.join(', ')));
    if (!parts.length) return '';
    return 'Kept as printed, not edited here: ' + parts.join('; ') + '.';
  }

  // The writing box's HTML reduced to the few tags it can make, parsed in an
  // inert document and stripped of every attribute but data-secret, so the
  // preview never runs anything dropped into the box. The server sanitizes
  // what is saved on its own.
  var KEEP = { P: 1, BR: 1, B: 1, STRONG: 1, I: 1, EM: 1, U: 1, SPAN: 1, DIV: 1, UL: 1, OL: 1, LI: 1 };
  function tidy(html) {
    var doc = new DOMParser().parseFromString('<div>' + (html || '') + '</div>', 'text/html');
    var root = doc.body.firstChild;
    (function walk(el) {
      Array.prototype.slice.call(el.childNodes).forEach(function (n) {
        if (n.nodeType === 3) return;
        if (n.nodeType !== 1) { n.remove(); return; }
        walk(n);
        if (!KEEP[n.tagName]) {
          if (/^(SCRIPT|STYLE|IFRAME|OBJECT|EMBED|TEMPLATE|SVG|MATH)$/i.test(n.tagName)) n.remove();
          else n.replaceWith.apply(n, Array.prototype.slice.call(n.childNodes));
          return;
        }
        var secret = n.hasAttribute('data-secret');
        Array.prototype.slice.call(n.attributes).forEach(function (a) { n.removeAttribute(a.name); });
        if (secret) n.setAttribute('data-secret', 'true');
      });
    })(root);
    return root;
  }

  // The system's rules for an entry, read back. Each is [message, blocks]:
  // what the server would refuse blocks Save; a budget a hero can't spend
  // is only a warning, since a Director may mean it.
  function problems(d, f) {
    var out = [];
    if (!d.name.trim()) out.push(['Give it a name.', true, 'name']);
    listsOf(d, f).forEach(function (l) {
      var items = d.lists[l.key] || [], seen = {}, total = 0;
      items.forEach(function (it) {
        var k = lower(it.name.trim());
        if (!k) out.push(['One of the ' + lower(l.label) + ' has no name yet.', true, 'item']);
        else if (seen[k]) out.push(['“' + it.name.trim() + '” is in ' + lower(l.label) + ' twice.', true, 'item']);
        seen[k] = true;
        total += it.cost;
      });
      if (l.buy && f.budgetKey) {
        if (!items.length) out.push(['Add ' + lower(l.label) + ' for heroes to buy.', false]);
        else if (total < d.budget) out.push(['The ' + lower(l.label) + ' add up to ' + total + ' point' + (total === 1 ? '' : 's') + ', less than the ' + d.budget + ' a hero spends. Add more, or spend fewer.', false]);
      }
    });
    return out;
  }

  // --- Motion and the stylesheet -------------------------------------------

  // html[data-motion] is the site's own signal: full, calm or off. A device
  // that asks for less motion is still, as in app_drawer.js.
  function motion() {
    var h = document.documentElement, m = h.getAttribute('data-motion');
    var rm = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (m === 'off' || rm) return 'still';
    if (m === 'calm') return 'gentle';
    return 'lively';
  }

  var cssReady = null;
  // The editor's stylesheet, beside this script's own address. Resolves when it
  // has loaded (or after a moment, so a slow stylesheet never holds the form).
  function ensureCss() {
    if (cssReady) return cssReady;
    cssReady = new Promise(function (resolve) {
      var have = document.querySelector('link[data-oed-css]');
      if (have && have.sheet) { resolve(); return; }
      // The script's ?v= token is its own digest, not the stylesheet's, so the
      // address drops it and the stylesheet is revalidated instead of being
      // held for a year.
      var href = HERE ? HERE.replace(/[?#].*$/, '').replace(/\.js$/, '.css').replace(/\/js\/([^\/]*)$/, '/css/$1') : '/static/plugins/entities/css/own_entry_editor.css';
      var l = have || document.createElement('link');
      l.rel = 'stylesheet';
      l.setAttribute('data-oed-css', '');
      l.onload = l.onerror = function () { resolve(); };
      if (!have) { l.href = href; document.head.appendChild(l); }
      setTimeout(resolve, 1500);
    });
    return cssReady;
  }

  function api(url, opts) {
    if (window.Chronicle && Chronicle.apiFetch) return Chronicle.apiFetch(url, opts);
    var o = { method: opts && opts.method || 'GET', credentials: 'same-origin', headers: { Accept: 'application/json' } };
    if (opts && opts.body) { o.headers['Content-Type'] = 'application/json'; o.body = JSON.stringify(opts.body); }
    if (opts && opts.csrfToken) o.headers['X-CSRF-Token'] = opts.csrfToken;
    return fetch(url, o);
  }
  function json(r) {
    return r.json().catch(function () { return {}; }).then(function (b) { return { ok: r.ok, status: r.status, body: b }; });
  }
  function dirty(on) {
    if (!window.Chronicle) return;
    if (on && Chronicle.markDirty) Chronicle.markDirty(DIRTY);
    if (!on && Chronicle.markClean) Chronicle.markClean(DIRTY);
  }

  // --- The Card border row --------------------------------------------------

  var cardCssReady = null;
  function cssHref(name) {
    // Beside this script, in css/ where the page serves the plugin's files.
    return HERE ? HERE.replace(/[?#].*$/, '').replace(/[^\/]*$/, '').replace(/\/js\/$/, '/css/') + name : '/static/plugins/entities/css/' + name;
  }
  // The stylesheets the samples and the picker need when the page has not
  // linked them already. hero_creator.css is not one of them: its names
  // (.hc-row, .hc-btn, .hc-page) are other screens' too, so the card's few
  // base rules come with this form's own sheet where the creator's is absent.
  function ensureCardCss() {
    if (cardCssReady) return cardCssReady;
    cardCssReady = Promise.all(['hero_borders.css', 'hero_border_picker.css'].map(function (n) {
      return new Promise(function (resolve) {
        if (document.querySelector('link[href*="' + n + '"]')) { resolve(); return; }
        var l = document.createElement('link');
        l.rel = 'stylesheet';
        l.onload = l.onerror = function () { resolve(); };
        l.href = cssHref(n);
        document.head.appendChild(l);
        setTimeout(resolve, 1500);
      });
    }));
    return cardCssReady;
  }
  function creatorSheet() { return !!document.querySelector('link[href*="hero_creator.css"]'); }

  // CardRow draws and runs the field. o: root (an element that stays while the
  // form is redrawn, which the borders instance colours inside), borders (a
  // shared instance, when the host already has one that covers root), label
  // (the kind of entry), director, and the field's class names for hosts that
  // style forms their own way (field, k, hint).
  function CardRow(kit, o) {
    this.kit = kit;
    this.o = o;
    this.mine = !o.borders;
    this.borders = o.borders || window.ChronicleHeroBorders(kit.lib, o.root);
    this.picker = window.ChronicleBorderPicker(kit.pick, this.borders);
    this.open = false;
    this.cleanup = null;
    this.raf = 0;
  }
  CardRow.prototype.nameOf = function (look) {
    return look ? String(this.kit.pick.borders[look.border].name == null ? look.border : this.kit.pick.borders[look.border].name) : 'None';
  };
  CardRow.prototype.colourOf = function (look) {
    return look ? (this.picker.isOwn(look) ? 'Its own colour' : this.picker.colourName(look.colour)) : 'A plain card';
  };
  CardRow.prototype.html = function (d) {
    var p = this.picker, look = d.look, open = this.open, o = this.o;
    var cls = o.field || 'oed-field', kc = o.k || 'oed-k', hc = o.hint || 'oed-hint';
    var base = creatorSheet() ? '' : ' hcw-f--base';
    return '<div class="' + cls + ' hcw-f' + base + '" data-hcw-f><span class="' + kc + '" id="hcw-f-k">Card border</span>' +
      '<button type="button" class="hcw-f__row" data-hcw="fold" aria-expanded="' + open + '" aria-describedby="hcw-f-k">' +
      '<span class="hc" style="container-type:normal;position:static" data-frow>' + p.sample(look) + '</span>' +
      '<span class="hcw-f__tx"><b data-fname>' + esc(this.nameOf(look)) + '</b><small data-fcol>' + esc(this.colourOf(look)) + '</small></span>' +
      '<span class="hcw-f__w" data-fword>' + (open ? 'Done' : 'Change') + '</span></button>' +
      '<div class="hcw-f__fold' + (open ? ' is-open' : '') + '" data-ffold><div class="hcw-f__in"><div class="hcw-f__pad">' +
      '<div class="hcw-f__live hc" data-look="stage" style="container-type:normal;position:static"><div class="hc-gcard" data-flive><span class="hc-top"><span>' + esc(o.label) + '</span></span><h3 class="hc-disp" data-flname></h3><p class="hc-sum" data-flsum></p></div></div>' +
      p.html({ look: look, sys: undefined }, { none: 'None', lay: 'field', top: true }) + '</div></div></div>' +
      '<span class="' + hc + '">' + (o.director ? 'How this card looks in the creator.' : 'How your card looks in the creator. Your ' + gm() + ' can change it.') + '</span></div>';
  };
  // Brings a drawn row, its live card and its picker in line with the draft.
  CardRow.prototype.sync = function (box, d) {
    var f = box && box.querySelector('[data-hcw-f]');
    if (!f || !d) return;
    var p = this.picker, look = d.look, open = this.open;
    var row = f.querySelector('[data-frow] .hc-gcard'), live = f.querySelector('[data-flive]'), pk = f.querySelector('[data-pk]');
    if (row) p.paint(row, look);
    if (live) p.paint(live, look);
    if (pk) p.sync(pk, { look: look, sys: undefined });
    f.querySelector('[data-fname]').textContent = this.nameOf(look);
    f.querySelector('[data-fcol]').textContent = this.colourOf(look);
    f.querySelector('[data-fword]').textContent = open ? 'Done' : 'Change';
    f.querySelector('[data-hcw="fold"]').setAttribute('aria-expanded', String(open));
    f.querySelector('[data-ffold]').classList.toggle('is-open', open);
    var nm = box.querySelector('[data-k="name"]'), sm = box.querySelector('[data-k="summary"]');
    f.querySelector('[data-flname]').textContent = (nm && nm.value.trim()) || 'Your ' + lower(this.o.label);
    f.querySelector('[data-flsum]').textContent = (sm && sm.value.trim()) || 'What a player reads before opening it.';
  };
  // hooks: get() the draft being written, changed() after a pick.
  CardRow.prototype.attach = function (box, hooks) {
    var self = this, p = this.picker;
    this.detach();
    function now() { self.sync(box, hooks.get()); }
    function onClick(ev) {
      var b = ev.target.closest && ev.target.closest('[data-hcw="fold"]');
      if (!b || !box.contains(b)) return;
      ev.preventDefault();
      self.open = !self.open;
      now();
    }
    function onInput(ev) {
      var t = ev.target;
      if (t.matches && t.matches('[data-k="name"], [data-k="summary"]')) now();
    }
    box.addEventListener('click', onClick);
    box.addEventListener('input', onInput);
    var unbind = p.bind(box, {
      state: function () { var d = hooks.get(); return d ? { look: d.look, sys: undefined } : null; },
      change: function (pk, look, merge) {
        var d = hooks.get();
        if (!d) return;
        d.look = look ? { border: look.border, colour: String(look.colour).toLowerCase(), own: p.isOwn(look) } : null;
        d.lookTouched = true;
        // A colour being dragged repaints once a frame, the last value winning.
        if (merge) { if (!self.raf) self.raf = requestAnimationFrame(function () { self.raf = 0; now(); }); }
        else now();
        hooks.changed();
      }
    });
    this.cleanup = function () {
      box.removeEventListener('click', onClick);
      box.removeEventListener('input', onInput);
      unbind();
    };
  };
  CardRow.prototype.detach = function () {
    if (this.cleanup) { this.cleanup(); this.cleanup = null; }
    if (this.raf) { cancelAnimationFrame(this.raf); this.raf = 0; }
  };
  CardRow.prototype.destroy = function () {
    this.detach();
    if (this.mine && this.borders && this.borders.destroy) this.borders.destroy();
  };
  // A row for a host, or null (no field) when the library is missing or the
  // pieces cannot be made.
  function makeRow(el, o) {
    try {
      var kit = cardLib(el);
      return kit ? new CardRow(kit, o) : null;
    } catch (e) { return null; }
  }

  // --- The editor ----------------------------------------------------------

  function scroller(el) {
    for (var e = el && el.parentElement; e && e !== document.body; e = e.parentElement) {
      if (/(auto|scroll)/.test(getComputedStyle(e).overflowY) && e.scrollHeight > e.clientHeight) return e;
    }
    return null;
  }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }
  function sameDraft(a, b) { return JSON.stringify(a) === JSON.stringify(b); }

  // opts: campaignId, csrf, step ({fieldKey, label, choices}), entry (the
  // player's own plan choice to change, or none), forId (a hero this entry is
  // for), closeButton, onSaved(entry, mode, isNew), say(text).
  function Editor(opts) {
    this.o = opts;
    this.base = '/campaigns/' + encodeURIComponent(opts.campaignId) + '/system-entries';
    this.csrf = opts.csrf || '';
    var st = opts.step || {};
    var pkg = (st.choices || []).filter(function (c) { return c.source !== 'campaign'; });
    this.f = shapeOf(pkg);
    this.f.key = st.fieldKey;
    this.f.label = st.label;
    this.grant = null;
    this.entry = null;
    this.d = null;
    this.saved = null;
    this.ready = false;
    // Always built from fixed words and the already-escaped GM term, so it is
    // written as HTML.
    this.failed = '';
    this.busy = false;
    this.confirm = false;
    this.tried = false;
    this.tryBuy = [];
    this.fresh = '';
    this.bodyEl = null;
    this.footEl = null;
    this.listeners = [];
  }

  // Reads the player's permission (and the entry to change), then draws the
  // form. The host may mount before this finishes; it shows "Opening" first.
  Editor.prototype.start = function () {
    var self = this, reads = [api(this.base + '/grants').then(json)];
    var id = this.o.entry && this.o.entry.entryId;
    if (id) reads.push(api(this.base + '?fieldKey=' + encodeURIComponent(this.f.key)).then(json));
    this.loading = Promise.all([ensureCss(), cardMap(this.o.root) ? ensureCardCss() : null, Promise.all(reads)]).then(function (r) {
      var res = r[2];
      if (!res[0].ok || (res[1] && !res[1].ok)) throw new Error('load');
      var key = self.f.key;
      self.grant = ((res[0].body && res[0].body.grants) || []).filter(function (g) { return g.fieldKey === key; })[0] || null;
      if (res[1]) {
        self.entry = ((res[1].body && res[1].body.entries) || []).filter(function (e) { return e.id === id; })[0] || null;
        if (!self.entry) throw new Error('gone');
      }
      if (!self.entry && !self.grant) self.failed = 'Your ' + gm() + ' hasn’t let you write one here.';
      else {
        self.d = draftOf(self.entry, self.f, self.grant && self.grant.budget, self.cardMap());
        self.saved = clone(self.d);
      }
      self.ready = true;
      self.paint();
      self.focusFirst();
    }).catch(function () {
      self.failed = 'Your entry could not be opened. Check your connection and try again.';
      self.ready = true;
      self.paint();
    });
    return this.loading;
  };

  Editor.prototype.mount = function (body, foot) {
    this.unmount();
    this.bodyEl = body;
    this.footEl = foot || null;
    if (foot) foot.classList.add('oed-foot');
    var self = this;
    this.card = this.cardRow();
    if (this.card) this.card.attach(body, { get: function () { return self.d; }, changed: function () { self.touched(); } });
    function on(el, type, fn) { el.addEventListener(type, fn); self.listeners.push([el, type, fn]); }
    on(body, 'click', function (e) { self.onClick(e); });
    on(body, 'input', function (e) { self.onInput(e); });
    if (foot) on(foot, 'click', function (e) { self.onClick(e); });
    this.paint();
  };
  Editor.prototype.unmount = function () {
    this.listeners.forEach(function (l) { l[0].removeEventListener(l[1], l[2]); });
    this.listeners = [];
    if (this.card) { this.card.destroy(); this.card = null; }
    this.bodyEl = this.footEl = null;
  };

  // The picker's border list, or null when this form has no Card border field.
  Editor.prototype.cardMap = function () { return cardMap(this.bodyEl || this.o.root); };
  // The field, made once for a mounted body. A creator that already colours
  // borders for its cards, and holds this body, lends its instance, so a border
  // in a colour is made once; otherwise one is made on the body and ended with it.
  Editor.prototype.cardRow = function () {
    var body = this.bodyEl, o = this.o;
    if (!body) return null;
    if (this.card) return this.card;
    var shared = o.borders && o.bordersRoot && o.bordersRoot.contains(body) ? o.borders : null;
    return makeRow(body, { root: body, borders: shared, label: this.f.label, director: !!o.director });
  };
  Editor.prototype.destroy = function () { this.unmount(); dirty(false); };

  Editor.prototype.isDirty = function () {
    this.syncText();
    return !!this.d && !sameDraft(this.d, this.saved);
  };
  Editor.prototype.say = function (t) {
    if (this.o.say) { this.o.say(t); return; }
    var live = this.bodyEl && this.bodyEl.querySelector('.oed-live');
    if (live) { live.textContent = ''; setTimeout(function () { live.textContent = t; }, 30); }
  };
  Editor.prototype.focusFirst = function () {
    if (!this.bodyEl) return;
    // A finger keeps the keyboard down until it taps a field.
    var coarse = window.matchMedia && window.matchMedia('(pointer: coarse)').matches;
    var n = this.bodyEl.querySelector('[data-k="name"]'), t = this.bodyEl.querySelector('[data-oed-title]');
    if (n && !n.value && !coarse) n.focus({ preventScroll: true });
    else if (t) t.focus({ preventScroll: true });
  };

  // Asks before dropping what was written; done() runs once it may close.
  Editor.prototype.requestClose = function (done) {
    if (!this.isDirty()) { done(); return; }
    this.confirm = true;
    this.after = done;
    this.paintFoot();
    var keep = this.footEl && this.footEl.querySelector('[data-oed="keep"]');
    if (keep) keep.focus({ preventScroll: true });
  };

  // --- Drawing ---

  Editor.prototype.paint = function () { this.paintBody(); this.paintFoot(); };

  Editor.prototype.paintBody = function () {
    var b = this.bodyEl;
    if (!b) return;
    var sc = scroller(b), top = sc ? sc.scrollTop : 0;
    if (!this.ready) b.innerHTML = '<div class="oed oed-load" role="status">Opening…</div>';
    else if (this.failed) {
      b.innerHTML = '<div class="oed"><p class="oed-fail" role="alert">' + this.failed + '</p>' +
        (this.d ? '' : '<button type="button" class="oed-link" data-oed="retry">Try again</button>') + '</div>';
    } else {
      b.innerHTML = this.formHTML();
      if (this.card) this.card.sync(b, this.d);
    }
    if (sc) sc.scrollTop = top;
  };

  Editor.prototype.paintFoot = function () {
    var f = this.footEl;
    if (!f) return;
    var d = this.d, ok = this.ready && !this.failed && d;
    if (!ok) { f.innerHTML = this.o.closeButton ? '<button type="button" class="oed-btn oed-btn--ghost" data-oed="cancel">Close</button>' : ''; return; }
    if (this.confirm) {
      f.innerHTML = '<div class="oed-confirm" role="alertdialog" aria-label="Unsaved changes"><p>Close without saving? What you changed is lost.</p>' +
        '<div class="oed-acts"><button type="button" class="oed-btn oed-btn--danger" data-oed="discard">Don’t save</button>' +
        '<button type="button" class="oed-btn oed-btn--ghost" data-oed="keep">Keep writing</button></div></div>';
      return;
    }
    var sent = d.status === 'sent', off = this.busy ? ' disabled' : '';
    f.innerHTML = '<p class="oed-error" role="alert" hidden data-error></p>' +
      '<div class="oed-acts"><button type="button" class="oed-btn oed-btn--ghost" data-oed="draft"' + off + '>' + (this.busy === 'draft' ? 'Saving…' : sent ? 'Save' : 'Save draft') + '</button>' +
      (sent ? '' : '<button type="button" class="oed-btn" data-oed="send"' + off + '>' + (this.busy === 'send' ? 'Handing in…' : 'Hand it in') + '</button>') + '</div>' +
      '<span class="oed-hint">' + (sent ? 'Handed in. Your ' + gm() + ' accepts it or asks for changes.' : 'Your ' + gm() + ' accepts it or asks for changes') + '</span>' +
      (this.o.closeButton ? '<button type="button" class="oed-link" data-oed="cancel">Close</button>' : '');
    if (this.errText) this.showError(this.errText);
  };

  // The name field's own error, tied to the field for a screen reader.
  Editor.prototype.nameError = function (on) {
    var fe = this.bodyEl && this.bodyEl.querySelector('[data-ferr="name"]'), n = this.bodyEl && this.bodyEl.querySelector('[data-k="name"]');
    if (fe) fe.hidden = !on;
    if (!n) return;
    if (on) { n.setAttribute('aria-invalid', 'true'); n.setAttribute('aria-describedby', 'oed-n-err'); }
    else { n.removeAttribute('aria-invalid'); n.removeAttribute('aria-describedby'); }
  };
  Editor.prototype.showError = function (msg) {
    this.errText = msg;
    var e = this.footEl && this.footEl.querySelector('[data-error]');
    if (e) { e.textContent = msg; e.hidden = !msg; }
  };

  function costSeg(li, ii, c) {
    return '<span class="oed-cost" role="group" aria-label="Cost">' + [1, 2, 3].map(function (n) {
      return '<button type="button" data-oed="cost" data-cost="' + n + '" data-li="' + esc(li) + '" data-ii="' + ii + '" aria-pressed="' + (c === n) + '">' + n + ' pt' + (n > 1 ? 's' : '') + '</button>';
    }).join('') + '</span>';
  }

  Editor.prototype.stateWord = function () {
    var d = this.d;
    if (d.status === 'sent') return 'Waiting for your ' + gm();
    if (d.status === 'draft') return d.reviewNote ? 'Changes asked' : 'Draft saved';
    return '';
  };

  Editor.prototype.formHTML = function () {
    var self = this, d = this.d, f = this.f, lbl = lower(f.label), g = this.grant;
    var st = this.stateWord();
    var asked = d.reviewNote && d.status === 'draft' ? '<p class="oed-asked" role="note"><b>Your ' + gm() + ' asked:</b> ' + esc(d.reviewNote) + '</p>' : '';
    var buyL = listsOf(d, f).filter(function (l) { return l.buy; })[0];
    var h = '<div class="oed" data-oed-form>' +
      '<div class="oed-kind"><span>' + esc(f.label) + ' · yours</span><button type="button" class="oed-mark" data-tip="allow" title="Your ' + gm() + ' lets you write one ' + esc(lbl) + ' of your own.">Your ' + gm() + ' allows one</button>' +
      (st ? '<span class="oed-state">' + st + '</span>' : '') + '</div>' +
      '<h3 class="oed-title" tabindex="-1" data-oed-title>' + (d.id ? 'Your ' + esc(lbl) : 'Make your own ' + esc(lbl)) + '</h3>' +
      '<p class="oed-lede">Built like every ' + esc(lbl) + ' here' + (f.budgetKey && buyL ? ', with ' + d.budget + ' points to spend on ' + esc(lower(buyL.label)) : '') + '. Your ' + gm() + ' reads it before it counts.</p>' + asked +
      '<div class="oed-field"><label for="oed-n">Name</label><input id="oed-n" class="oed-big" data-k="name" maxlength="100" autocomplete="off" value="' + esc(d.name) + '" placeholder="For example, Ashborn">' +
      '<span class="oed-ferr" id="oed-n-err" data-ferr="name" hidden>Give it a name first.</span></div>' +
      '<div class="oed-field"><label for="oed-s">A few words about it</label><textarea id="oed-s" rows="2" data-k="summary" maxlength="300" placeholder="What a player reads before opening it">' + esc(d.summary) + '</textarea></div>' +
      (this.card ? this.card.html(d) : '') + this.factsHTML() + this.textHTML() + this.wordsHTML() + this.budgetHTML() +
      listsOf(d, f).map(function (l) { return self.listHTML(l); }).join('') + '<div data-prev>' + this.prevHTML() + '</div>';
    var kept = keptLine(d, f);
    h += (kept ? '<p class="oed-hint oed-kept">' + esc(kept) + '</p>' : '') + '<div class="oed-checks" data-checks>' + this.checksHTML() + '</div>' +
      '<p class="oed-who">Only you and your ' + gm() + ' see it until a ' + gm() + ' accepts it.</p>' +
      '<div class="oed-live oed-sr" aria-live="polite"></div></div>';
    return h;
  };

  Editor.prototype.factsHTML = function () {
    var d = this.d;
    var rows = d.facts.map(function (p, i) {
      var val = p.long
        ? '<textarea class="oed-fact-long" rows="4" aria-label="' + esc(p.label || 'Fact') + '" data-fact="' + i + '" data-part="value" maxlength="2000">' + esc(p.value) + '</textarea>'
        : '<input aria-label="Fact value" data-fact="' + i + '" data-part="value" maxlength="200" value="' + esc(p.value) + '" placeholder="Value">';
      return '<div class="oed-fact' + (this.fresh === 'f' + i ? ' is-new' : '') + '"><input aria-label="Fact name" data-fact="' + i + '" data-part="label" maxlength="40" value="' + esc(p.label) + '" placeholder="Name, e.g. Size">' + val +
        '<button type="button" class="oed-rm" data-oed="rmfact" data-i="' + i + '" aria-label="Remove ' + esc(p.label || 'this fact') + '">×</button></div>';
    }, this).join('');
    return '<div class="oed-field"><span class="oed-k">Facts</span>' + (rows ? '<div class="oed-facts">' + rows + '</div>' : '') +
      '<div><button type="button" class="oed-link" data-oed="addfact">+ Add a fact</button></div></div>';
  };

  Editor.prototype.textHTML = function () {
    var safe = tidy(this.d.text);
    safe.querySelectorAll('[data-secret]').forEach(function (s) { s.replaceWith.apply(s, Array.prototype.slice.call(s.childNodes)); });
    return '<div class="oed-field"><span class="oed-k" id="oed-text-k">More about it, if you like</span><div class="oed-tool">' +
      '<button type="button" data-oed="bold" title="Bold" aria-label="Bold"><b>B</b></button><button type="button" data-oed="italic" title="Italic" aria-label="Italic"><i>I</i></button></div>' +
      '<div class="oed-text" contenteditable="true" role="textbox" aria-multiline="true" aria-labelledby="oed-text-k" data-k="text" data-placeholder="Write it as you would a page.">' + safe.innerHTML + '</div></div>';
  };

  Editor.prototype.wordsHTML = function () {
    return (this.d.wordLists || []).map(function (w, i) {
      return '<label class="oed-field"><span class="oed-k">' + esc(w.label) + ' <small class="oed-hint">one per line</small></span>' +
        '<textarea class="oed-words" rows="' + Math.min(6, Math.max(2, w.text.split('\n').length)) + '" data-word="' + i + '">' + esc(w.text) + '</textarea></label>';
    }).join('');
  };

  // The budget is the Director's when they set one (the server holds it too);
  // otherwise it is a number the player picks.
  Editor.prototype.budgetHTML = function () {
    var f = this.f, d = this.d;
    if (!f.budgetKey) return '';
    var locked = this.grant && this.grant.budget > 0, nm = esc(words(f.budgetKey));
    if (locked) {
      return '<dl class="oed-ro"><div><dt>' + nm + '</dt><dd>' + d.budget + ', locked</dd></div></dl>';
    }
    var buy = listsOf(d, f).filter(function (l) { return l.buy; })[0];
    return '<div class="oed-field oed-bud"><span class="oed-k" id="oed-bud-k">' + nm + '</span>' +
      '<span class="oed-step" role="group" aria-labelledby="oed-bud-k"><button type="button" data-oed="minus" aria-label="One point fewer">−</button>' +
      '<output>' + d.budget + '</output><button type="button" data-oed="plus" aria-label="One point more">+</button></span>' +
      (buy ? '<span class="oed-hint">Points a hero spends on ' + esc(lower(buy.label)) + '.</span>' : '') + '</div>';
  };

  Editor.prototype.listHTML = function (l) {
    var d = this.d, items = d.lists[l.key] || [], self = this;
    var thing = sing(l.label);
    return '<div class="oed-field oed-list" data-list="' + esc(l.key) + '"><span class="oed-k">' + esc(l.label) + (l.buy ? ' <small class="oed-hint">a hero buys these</small>' : '') + '</span>' +
      items.map(function (it, i) {
        return '<div class="oed-item' + (self.fresh === l.key + ':' + i ? ' is-new' : '') + '"><div class="oed-item__row">' +
          '<input class="oed-item__name" aria-label="' + esc(thing) + ' name" data-li="' + esc(l.key) + '" data-ii="' + i + '" data-part="name" maxlength="100" value="' + esc(it.name) + '" placeholder="Name">' +
          (l.buy ? costSeg(l.key, i, it.cost) : '') +
          '<button type="button" class="oed-rm" data-oed="rmitem" data-li="' + esc(l.key) + '" data-ii="' + i + '" aria-label="Remove ' + esc(it.name || thing) + '">×</button></div>' +
          '<textarea rows="2" aria-label="What it does" data-li="' + esc(l.key) + '" data-ii="' + i + '" data-part="description" maxlength="2000" placeholder="What it does">' + esc(it.description) + '</textarea></div>';
      }).join('') +
      '<div><button type="button" class="oed-add" data-oed="additem" data-li="' + esc(l.key) + '">+ Add ' + an(thing) + ' ' + esc(thing) + '</button></div></div>';
  };

  // The first list a hero buys from, as the creator reads it, and what is
  // ticked in the try-out.
  Editor.prototype.buyOf = function () {
    var l = listsOf(this.d, this.f).filter(function (x) { return x.buy; }).sort(function (a, b) { return a.key < b.key ? -1 : 1; })[0];
    if (!l || !this.f.budgetKey) return null;
    var items = (this.d.lists[l.key] || []).map(function (it, i) { return { i: i, name: it.name.trim(), cost: it.cost, text: it.description }; }).filter(function (x) { return x.name; });
    return { l: l, items: items };
  };
  Editor.prototype.spent = function (buy) {
    var t = this.tryBuy;
    return buy.items.reduce(function (n, x) { return n + (t.indexOf(x.i) >= 0 ? x.cost : 0); }, 0);
  };
  Editor.prototype.trim = function () {
    var buy = this.buyOf(), keep = [], left = this.d.budget;
    if (buy) buy.items.forEach(function (x) { if (this.tryBuy.indexOf(x.i) >= 0 && x.cost <= left) { keep.push(x.i); left -= x.cost; } }, this);
    this.tryBuy = keep;
  };
  Editor.prototype.pips = function (spent) {
    var out = '';
    for (var i = 0; i < this.d.budget && i < 12; i++) out += '<i class="' + (i < spent ? 'is-on' : '') + '"></i>';
    return out;
  };
  Editor.prototype.tlabel = function (x, on, no, left) {
    return on ? 'Bought ✓' : no ? 'Needs ' + x.cost + (x.cost === 1 ? ' point' : ' points') + ', ' + left + ' left' : 'Buy for ' + x.cost + (x.cost === 1 ? ' point' : ' points');
  };
  Editor.prototype.prevHTML = function () {
    var buy = this.buyOf(), d = this.d;
    if (!buy) return '';
    this.trim();
    var spent = this.spent(buy), left = d.budget - spent, self = this, nm = esc(words(this.f.budgetKey));
    var locked = this.grant && this.grant.budget > 0;
    var cards = buy.items.map(function (x) {
      var on = self.tryBuy.indexOf(x.i) >= 0, no = !on && x.cost > left;
      return '<div class="oed-tcard' + (on ? ' is-on' : '') + (no ? ' is-no' : '') + '" role="listitem" data-t="' + x.i + '"><div class="oed-th"><b>' + esc(x.name) + '</b><span class="oed-pc">' + x.cost + (x.cost === 1 ? ' point' : ' points') + '</span></div>' +
        (x.text.trim() ? '<p>' + esc(x.text) + '</p>' : '') +
        '<button type="button" class="oed-tbuy" data-oed="try" data-t="' + x.i + '" aria-pressed="' + on + '"' + (no ? ' aria-disabled="true"' : '') + '>' + self.tlabel(x, on, no, left) + '</button></div>';
    }).join('');
    return '<div class="oed-prev"><span class="oed-k">How a hero buys it</span>' +
      '<div class="oed-meter"><span class="oed-bigno" data-left>' + left + '</span><span class="oed-lab2"><b>' + nm + ' left</b><span>of ' + d.budget + (locked ? ' · set by your ' + gm() : '') + '</span></span>' +
      '<span class="oed-pips" aria-hidden="true">' + this.pips(spent) + '</span></div>' +
      (cards ? '<div class="oed-buyrow" role="list">' + cards + '</div><div><button type="button" class="oed-link" data-oed="unbuy">Clear</button></div>'
        : '<p class="oed-hint">Add a trait above and it shows up here, ready to buy. Tap to try spending the points.</p>') + '</div>';
  };

  // Tapping a card in the try-out updates the meter in place, so the number
  // counts and the pips fill instead of the whole preview flickering.
  Editor.prototype.tryToggle = function (i) {
    var buy = this.buyOf();
    if (!buy) return;
    var x = buy.items.filter(function (y) { return y.i === i; })[0];
    if (!x) return;
    var at = this.tryBuy.indexOf(i), left = this.d.budget - this.spent(buy);
    if (at >= 0) this.tryBuy.splice(at, 1);
    else if (x.cost <= left) this.tryBuy.push(i);
    else {
      var c = this.bodyEl.querySelector('.oed-tcard[data-t="' + i + '"]');
      if (c && motion() !== 'still') { c.classList.remove('is-flash'); void c.offsetWidth; c.classList.add('is-flash'); }
      this.say('Needs ' + x.cost + ' points, ' + left + ' left.');
      return;
    }
    this.syncTry(buy);
  };
  Editor.prototype.syncTry = function (buy) {
    var left = this.d.budget - this.spent(buy), spent = this.d.budget - left, self = this, root = this.bodyEl;
    var big = root.querySelector('[data-left]');
    if (big && big.textContent !== String(left)) {
      big.textContent = left;
      big.classList.remove('is-pop'); void big.offsetWidth;
      if (motion() !== 'still') big.classList.add('is-pop');
    }
    root.querySelectorAll('.oed-pips i').forEach(function (p, n) { p.classList.toggle('is-on', n < spent); });
    buy.items.forEach(function (x) {
      var c = root.querySelector('.oed-tcard[data-t="' + x.i + '"]');
      if (!c) return;
      var on = self.tryBuy.indexOf(x.i) >= 0, no = !on && x.cost > left, b = c.querySelector('.oed-tbuy');
      c.classList.toggle('is-on', on); c.classList.toggle('is-no', no);
      b.setAttribute('aria-pressed', String(on));
      if (no) b.setAttribute('aria-disabled', 'true'); else b.removeAttribute('aria-disabled');
      b.textContent = self.tlabel(x, on, no, left);
    });
    this.say(left + (left === 1 ? ' point' : ' points') + ' left');
  };

  Editor.prototype.checksHTML = function () {
    var self = this, p = problems(this.d, this.f).filter(function (m) { return (!m[1] || self.tried) && !(self.tried && m[2] === 'name'); });
    if (!p.length) return (this.tried || !problems(this.d, this.f).length) && this.d.name.trim() ? '<p class="oed-ready">✓ Ready' + (this.f.budgetKey ? '. A hero can spend all ' + this.d.budget + ' points.' : '.') + '</p>' : '';
    return '<ul class="oed-problems" aria-label="Before you save">' + p.map(function (m) { return '<li class="' + (m[1] ? 'is-block' : '') + '">' + esc(m[0]) + '</li>'; }).join('') + '</ul>';
  };

  // --- Reading what is typed ---

  Editor.prototype.syncText = function () {
    var box = this.bodyEl && this.bodyEl.querySelector('.oed-text');
    if (!box || !this.d) return;
    this.d.text = (box.textContent.trim() || box.querySelector('li')) ? box.innerHTML : '';
  };

  // A change that does not add or remove a field: the checks, the preview
  // and the dirty mark follow, the form stays as it is under the cursor.
  Editor.prototype.touched = function () {
    var d = this.d;
    if (!d || !this.bodyEl) return;
    dirty(this.isDirty());
    var c = this.bodyEl.querySelector('[data-checks]');
    if (c) c.innerHTML = this.checksHTML();
    var p = this.bodyEl.querySelector('[data-prev]');
    if (p) p.innerHTML = this.prevHTML();
    if (d.name.trim()) this.nameError(false);
    if (this.errText) { this.errText = ''; this.showError(''); }
  };

  // Redraws the form after a field is added or removed, keeping the place.
  Editor.prototype.redraw = function (focusSel) {
    this.syncText();
    this.paintBody();
    this.touched();
    if (focusSel) { var all = this.bodyEl.querySelectorAll(focusSel), f = all[all.length - 1]; if (f) f.focus(); }
    this.fresh = '';
  };

  Editor.prototype.onInput = function (e) {
    var el = e.target, d = this.d;
    if (!d || !el.getAttribute) return;
    var k = el.getAttribute('data-k');
    if (k === 'text') this.syncText();
    else if (k) d[k] = el.value;
    if (el.hasAttribute('data-fact')) d.facts[+el.getAttribute('data-fact')][el.getAttribute('data-part')] = el.value;
    if (el.hasAttribute('data-li') && el.hasAttribute('data-part')) d.lists[el.getAttribute('data-li')][+el.getAttribute('data-ii')][el.getAttribute('data-part')] = el.value;
    if (el.hasAttribute('data-word')) d.wordLists[+el.getAttribute('data-word')].text = el.value;
    if (k || el.hasAttribute('data-fact') || el.hasAttribute('data-li') || el.hasAttribute('data-word')) this.touched();
  };

  Editor.prototype.onClick = function (e) {
    var t = e.target.closest && e.target.closest('[data-oed]');
    if (!t) return;
    var d = this.d, act = t.getAttribute('data-oed'), li = t.getAttribute('data-li'), ii = +t.getAttribute('data-ii'), self = this;
    switch (act) {
      case 'retry': this.failed = ''; this.ready = false; this.paint(); this.start(); break;
      case 'bold': case 'italic':
        document.execCommand(act);
        this.syncText(); this.touched();
        break;
      case 'addfact':
        this.syncText();
        d.facts.push({ label: '', value: '', key: '' });
        this.fresh = 'f' + (d.facts.length - 1);
        this.redraw('.oed-fact input[data-part="label"]');
        break;
      case 'rmfact':
        d.facts.splice(+t.getAttribute('data-i'), 1);
        this.redraw();
        break;
      case 'additem':
        this.syncText();
        (d.lists[li] = d.lists[li] || []).push({ name: '', cost: 1, description: '' });
        this.fresh = li + ':' + (d.lists[li].length - 1);
        this.redraw('.oed-list[data-list="' + li + '"] .oed-item__name');
        break;
      case 'rmitem':
        d.lists[li].splice(ii, 1);
        this.tryBuy = [];
        this.redraw('.oed-list[data-list="' + li + '"] [data-oed="additem"]');
        break;
      case 'cost':
        d.lists[li][ii].cost = +t.getAttribute('data-cost');
        t.parentNode.querySelectorAll('button').forEach(function (b) { b.setAttribute('aria-pressed', String(b === t)); });
        this.tryBuy = [];
        this.touched();
        break;
      case 'plus': case 'minus':
        d.budget = Math.max(1, Math.min(20, d.budget + (act === 'plus' ? 1 : -1)));
        t.parentNode.querySelector('output').textContent = d.budget;
        this.tryBuy = [];
        this.touched();
        break;
      case 'try': this.tryToggle(+t.getAttribute('data-t')); break;
      case 'unbuy': this.tryBuy = []; var buy = this.buyOf(); if (buy) { var p = this.bodyEl.querySelector('[data-prev]'); p.innerHTML = this.prevHTML(); } break;
      case 'draft': this.save(''); break;
      case 'send': this.save('send'); break;
      case 'cancel': this.requestClose(function () { self.close(); }); break;
      case 'keep': this.confirm = false; this.after = null; this.paintFoot(); break;
      case 'discard': var go = this.after; this.confirm = false; this.after = null; dirty(false); if (go) go(); break;
    }
  };

  // The host's close: the editor tells it so it can put its sheet away.
  Editor.prototype.close = function () { if (this.o.onClose) this.o.onClose(); };

  // --- Saving ---

  // mode is '' (save as a draft, or save changes) or 'send' (hand it in).
  Editor.prototype.save = function (mode) {
    var self = this, d = this.d, f = this.f;
    if (this.busy || !d) return;
    this.syncText();
    this.tried = true;
    var bad = problems(d, f).filter(function (m) { return m[1]; });
    if (bad.length) {
      this.touched();
      // A missing name is said once, by the name field; the foot speaks only
      // for what the field cannot.
      var rest = bad.filter(function (m) { return m[2] !== 'name'; });
      if (!d.name.trim()) {
        this.nameError(true);
        var n = this.bodyEl.querySelector('[data-k="name"]');
        if (n) n.focus();
      }
      this.showError(rest.length ? rest[0][0] : '');
      return;
    }
    var built = propsOf(d, f);
    if (built.error) { this.showError(built.error); return; }
    var text = tidy(d.text);
    var body = { name: d.name.trim(), summary: d.summary.trim(), description: text.textContent.trim() ? text.innerHTML : '', properties: built.props };
    var isNew = !d.id;
    if (mode === 'send') body.status = 'sent';
    if (isNew) { body.fieldKey = f.key; if (this.o.forId) body.forEntityId = this.o.forId; }
    this.busy = mode === 'send' ? 'send' : 'draft';
    this.showError('');
    this.paintFoot();
    api(isNew ? this.base : this.base + '/' + encodeURIComponent(d.id), { method: isNew ? 'POST' : 'PUT', body: body, csrfToken: this.csrf })
      .then(json).then(function (res) {
        self.busy = false;
        if (!res.ok || !res.body.entry) {
          self.paintFoot();
          var msg = res.body.message || res.body.error || 'The entry could not be saved. Try again.';
          self.showError(msg);
          if (res.status === 409) { var nm = self.bodyEl && self.bodyEl.querySelector('[data-k="name"]'); if (nm) nm.focus(); }
          return;
        }
        var e = res.body.entry;
        self.entry = e;
        self.d = draftOf(e, f, self.grant && self.grant.budget, self.cardMap());
        self.saved = clone(self.d);
        dirty(false);
        self.paintFoot();
        if (self.o.onSaved) self.o.onSaved(e, mode, isNew);
      }).catch(function () {
        self.busy = false;
        self.paintFoot();
        self.showError('The entry could not be saved. Check your connection and try again.');
      });
  };

  // A server entry as a plan choice, so the host can list and pick it like any
  // other: the same fields the plan carries for a player's own entry.
  function choiceOf(e) {
    var sum = e.summary || tidy(e.description).textContent.trim().slice(0, 160);
    // The border is the card's look, not a fact to list.
    var props = {}, all = e.properties || {};
    Object.keys(all).forEach(function (k) { if (!cardKey(k)) props[k] = all[k]; });
    var lk = lookFromProps(all, cardMap(null)).look;
    var ch = { key: String(e.id), slug: e.slug || '', name: e.name, summary: sum, source: 'campaign', description: e.description || '',
      properties: props, status: e.status !== 'accepted' ? e.status : '', mine: true, entryId: e.id, reviewNote: e.reviewNote || '' };
    if (lk) ch.look = { border: lk.border, colour: lk.colour };
    return ch;
  }

  Editor.choiceOf = choiceOf;

  window.ChronicleOwnEntry = {
    shapeOf: shapeOf, draftOf: draftOf, propsOf: propsOf, problems: problems, tidy: tidy, listsOf: listsOf, keptLine: keptLine,
    isDraft: isDraft, itemOf: itemOf, extraKeys: extraKeys, suggestedFacts: suggestedFacts, namedList: namedList, wordList: wordList,
    longFact: longFact, creditKey: creditKey, factValue: factValue, keyOf: keyOf, words: words, sing: sing, motion: motion,
    ensureCss: ensureCss, ensureCardCss: ensureCardCss, cardMap: cardMap, cardLib: cardLib, makeRow: makeRow, lookFromProps: lookFromProps,
    Editor: Editor, choiceOf: choiceOf
  };
})();
