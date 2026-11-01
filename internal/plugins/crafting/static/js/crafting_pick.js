/**
 * crafting_pick.js -- typing a recipe (window.CraftingPick)
 *
 * The approved way to write a recipe anywhere outside the Crafting app: a
 * material is a few letters plus Enter ("2 moo" adds two Moonwater), and a
 * step is a short line ("grind 3", "pour moonwater"). Shared by "How it's
 * made" on an item's page and the Workshop's Crafting recipes part, so both
 * read the same words. Every name is escaped before it reaches the page.
 */
(function () {
  'use strict';
  var L = window.CraftingLib, CP = window.CraftPictures, esc = L.esc;

  /* "2 moo" is two of whatever "moo" finds; a bare name is one. */
  function splitQty(text) {
    var m = /^\s*(\d{1,2})\s*[x×]?\s+(.*)$/i.exec(text || '');
    if (m) return {qty: Math.max(1, Math.min(99, +m[1])), q: m[2].trim()};
    return {qty: 1, q: String(text || '').trim()};
  }

  /* What a few letters find: items whose name starts with them first, then
     items that hold them, then kinds as "Any herb". */
  function search(lib, items, q, limit) {
    q = String(q || '').trim().toLowerCase().replace(/^any\s+/, '');
    if (!q) return [];
    var starts = [], holds = [], kinds = [];
    (items || []).forEach(function (it) {
      var n = it.name.toLowerCase();
      if (n.indexOf(q) === 0) starts.push(it); else if (n.indexOf(q) > 0) holds.push(it);
    });
    (lib.tags || []).forEach(function (t) { if (t.label.toLowerCase().indexOf(q) === 0 || t.key.indexOf(q) === 0) kinds.push(t); });
    var out = starts.concat(holds).slice(0, limit || 6).map(function (it) {
      return {itemId: it.id, name: it.name, hint: (it.tags || []).map(function (k) { return L.tagLabel(lib, k); }).join(', '), pic: L.matPicture(lib, it)};
    });
    kinds.slice(0, 3).forEach(function (t) { out.push({tag: t.key, name: 'Any ' + t.label.toLowerCase(), hint: 'whatever the hero carries', pic: L.matPicture(lib, t.key)}); });
    return out;
  }

  function pic(p, size) {
    return CP && p && p.picture ? CP.svg(p.picture, p.tint, size || 20) : '<span class="crm-nopic" aria-hidden="true"></span>';
  }

  /* Wires an input as a material picker. onPick({itemId|tag, qty}) gets the
     pick; the input empties and keeps focus so the next one can be typed. */
  function material(input, opts) {
    var box = document.createElement('div'), hits = [], at = 0;
    box.className = 'crm-pull';
    box.setAttribute('role', 'listbox');
    box.id = 'crm-pull-' + Math.random().toString(36).slice(2, 8);
    box.hidden = true;
    input.setAttribute('role', 'combobox');
    input.setAttribute('aria-autocomplete', 'list');
    input.setAttribute('aria-controls', box.id);
    input.setAttribute('aria-expanded', 'false');
    input.parentNode.insertBefore(box, input.nextSibling);
    function draw() {
      var s = splitQty(input.value);
      hits = search(opts.lib(), opts.items(), s.q, 6);
      at = Math.min(at, Math.max(0, hits.length - 1));
      if (!s.q) { close(); return; }
      box.innerHTML = (hits.length ? hits.map(function (h, i) {
        return '<div class="crm-opt" role="option" id="' + box.id + '-' + i + '" data-i="' + i + '" aria-selected="' + (i === at) + '">' + pic(h.pic, 20) +
          '<span class="crm-opt-n">' + esc(h.name) + '</span><span class="crm-opt-h">' + esc(h.hint) + '</span></div>';
      }).join('') : '<p class="crm-pull-none">Nothing called “' + esc(s.q) + '”.' + (opts.onMissing ? ' <button type="button" class="crm-q" data-new>Add it to the Armory</button>' : '') + '</p>') +
        '<p class="crm-pull-hint">Enter adds the first · a number first sets how many</p>';
      box.hidden = false;
      input.setAttribute('aria-expanded', 'true');
      if (hits.length) input.setAttribute('aria-activedescendant', box.id + '-' + at); else input.removeAttribute('aria-activedescendant');
    }
    function close() { box.hidden = true; input.setAttribute('aria-expanded', 'false'); input.removeAttribute('aria-activedescendant'); }
    function take(i) {
      var h = hits[i], s = splitQty(input.value);
      if (!h) return;
      opts.onPick(h.itemId ? {itemId: h.itemId, qty: s.qty} : {tag: h.tag, qty: s.qty});
      input.value = ''; at = 0; close(); input.focus();
    }
    input.addEventListener('input', function () { at = 0; draw(); });
    input.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown' && hits.length) { e.preventDefault(); at = (at + 1) % hits.length; draw(); }
      else if (e.key === 'ArrowUp' && hits.length) { e.preventDefault(); at = (at - 1 + hits.length) % hits.length; draw(); }
      else if (e.key === 'Enter') { if (!box.hidden && hits.length) { e.preventDefault(); take(at); } }
      else if (e.key === 'Escape' && !box.hidden) { e.preventDefault(); e.stopPropagation(); close(); }
    });
    input.addEventListener('blur', function () { setTimeout(function () { if (!box.contains(document.activeElement)) close(); }, 150); });
    box.addEventListener('mousedown', function (e) { e.preventDefault(); });
    box.addEventListener('click', function (e) {
      var o = e.target.closest('[data-i]');
      if (o) { take(+o.getAttribute('data-i')); return; }
      if (e.target.closest('[data-new]') && opts.onMissing) { var s = splitQty(input.value); close(); opts.onMissing(s.q, s.qty); }
    });
    return {close: close, destroy: function () { box.remove(); }};
  }

  /* A step written as a short line: "add silverleaf", "grind 3", "tip",
     "pour moonwater", "simmer long", "strike 2". A material the recipe
     doesn't list yet is offered back as add, so the caller can list it.
     items is the campaign's item list; the recipe helpers read it by id. */
  var WORDS = {add: 'add', put: 'add', drop: 'add', grind: 'grind', crush: 'grind', tip: 'tip', pour: 'pour', simmer: 'simmer', heat: 'simmer', steady: 'simmer',
    keep: 'simmer', strike: 'strike', catch: 'strike', press: 'strike'};
  function step(text, lib, r, items) {
    var byId = {};
    (items || []).forEach(function (x) { byId[x.id] = x; });
    var t = String(text || '').trim().toLowerCase().replace(/[.!]$/, ''), m = /^(\w+)\s*(.*)$/.exec(t);
    if (!m || !WORDS[m[1]]) return {error: 'Start with add, grind, tip, pour, simmer or strike.'};
    var kind = WORDS[m[1]], rest = m[2].replace(/^(it|the|in|into)\s+/, ''), s = L.newStep(lib, r, kind, byId), k = L.stepKind(lib, kind);
    var n = parseInt(rest, 10);
    if (kind === 'grind') { if (n) s.turns = Math.max(1, Math.min(8, n)); return {step: s}; }
    if (kind === 'strike') { if (n) s.n = Math.max(1, Math.min(6, n)); return {step: s}; }
    if (kind === 'simmer') { s.secs = /long/.test(rest) ? 10 : n ? Math.max(2, Math.min(20, n)) : (/short/.test(rest) ? 5 : s.secs); return {step: s}; }
    if (kind === 'tip') return {step: s};
    // add and pour name a material.
    var to = /\b(flask)\b/.test(rest) ? 'flask' : /\bmortar\b/.test(rest) ? 'mortar' : '';
    var q = rest.replace(/\b(to|into|in)?\s*the\s+(flask|mortar)\b/, '').replace(/\b(flask|mortar)\b/, '').trim();
    if (kind === 'add' && to) s.to = to;
    if (!q) return s.material != null ? {step: s} : {error: 'Say what to ' + kind + ', like “' + kind + ' moonwater”.'};
    var idx = -1;
    r.materials.forEach(function (mt, i) { if (idx < 0 && L.matName(lib, mt, byId).toLowerCase().replace(/^any /, '').indexOf(q.replace(/^any\s+/, '')) === 0) idx = i; });
    if (idx >= 0) { s.material = idx; return {step: s}; }
    var hit = search(lib, items, q, 1)[0];
    if (!hit) return {error: 'Nothing called “' + q + '”.'};
    if (k && k.accepts && k.accepts.length && hit.itemId) {
      var it = byId[hit.itemId];
      if (it && !(it.tags || []).some(function (tg) { return k.accepts.indexOf(tg) >= 0; })) return {error: k.name + ' takes ' + k.accepts.map(function (a) { return L.tagLabel(lib, a); }).join(', ') + '. ' + hit.name + ' isn’t one.'};
    }
    return {step: s, addMaterial: hit.itemId ? {itemId: hit.itemId, qty: 1} : {tag: hit.tag, qty: 1}};
  }

  /* A step as the line a reader sees, and its kind as a small word. */
  function stepWords(lib, r, s, items) { return L.methodLine(lib, r, s, items).replace(/\.$/, ''); }

  window.CraftingPick = {splitQty: splitQty, search: search, material: material, step: step, stepWords: stepWords, pic: pic};
})();
