/**
 * find_palette.js -- the small "Find a setting" field shared by every workshop
 *
 * Chronicle.findPalette.open({ finds, label, placeholder, onGo, onClose })
 * draws a field over a scrim, filters `finds` as the owner types and goes to
 * the chosen one. A find is { label, path, href, icon, words }: `label` and
 * `path` are shown, and `words` are extra names that match without showing,
 * so "colour" finds Look. A hit in the label ranks above one only in the path,
 * and a typed word that is exactly one of `words` counts like a hit inside the
 * label, so an entry listing "settings" is not buried under longer names.
 *
 * The Customizer and the Manage desk both use it, so Find reads and keys the
 * same in both places: arrows move, Enter goes, Escape closes and puts focus
 * back where it was. It never listens for the "/" key itself; each workshop
 * decides when "/" is its own (never while someone is typing) and calls open.
 * Only opacity and transform move, and under Calm it is a plain fade and under
 * Off it just appears.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});

  // Chronicle.escapeHtml is the one escape every script shares (boot.js).
  function esc(s) { return C.escapeHtml(s); }

  function words(s) { return String(s || '').toLowerCase(); }

  // match returns the finds that contain every typed word, best first: a hit
  // in the label first, one at the start of the label before one inside it,
  // then the rest in the order the server listed them. Exported for tests.
  function match(finds, query, limit) {
    var q = words(query).trim().split(/\s+/).filter(Boolean);
    return finds.map(function (f, i) {
      var hay = words(f.label + ' ' + f.path + ' ' + (f.words || ''));
      if (!q.every(function (w) { return hay.indexOf(w) >= 0; })) return null;
      var lab = words(f.label), rank = 2;
      if (q.length && q.every(function (w) { return lab.indexOf(w) >= 0; })) rank = lab.indexOf(q[0]) === 0 ? 0 : 1;
      else {
        var listed = words(f.words).split(/\s+/);
        if (q.length && q.every(function (w) { return listed.indexOf(w) >= 0; })) rank = 1;
      }
      return { f: f, rank: rank, i: i };
    }).filter(Boolean).sort(function (a, b) {
      return a.rank - b.rank || a.i - b.i;
    }).slice(0, limit || 40).map(function (r) { return r.f; });
  }

  // Only a same-address find with a # moves in place; anything else is a visit.
  function defaultGo(f) {
    var here = location.pathname, to = String(f.href).split('#');
    if (to[0] === here && to[1]) { location.hash = to[1]; return; }
    location.href = f.href;
  }

  var open_ = null;

  function open(opts) {
    if (open_) return open_;
    opts = opts || {};
    var finds = opts.finds || [];
    var label = opts.label || 'Find a setting';
    var opener = document.activeElement;
    var scrim = document.createElement('div');
    scrim.className = 'fnd-scrim';
    scrim.innerHTML = '<div class="fnd-box" role="dialog" aria-modal="true" aria-label="' + esc(label) + '">' +
      '<div class="fnd-q"><i class="fa-solid fa-magnifying-glass" aria-hidden="true"></i>' +
      '<input type="text" placeholder="' + esc(opts.placeholder || 'Find a setting') + '" aria-label="' + esc(label) +
      '" autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true" aria-controls="fnd-res"><kbd>Esc</kbd></div>' +
      '<div class="fnd-res" id="fnd-res" role="listbox"></div></div>';
    document.body.appendChild(scrim);
    var input = scrim.querySelector('input'), res = scrim.querySelector('.fnd-res');
    var sel = 0, shown = [];

    function draw() {
      shown = match(finds, input.value);
      sel = Math.min(sel, Math.max(0, shown.length - 1));
      res.innerHTML = shown.length ? shown.map(function (f, i) {
        var icon = String(f.icon || 'fa-sliders').replace(/^fa-solid\s+/, '');
        return '<a href="' + esc(f.href) + '" role="option" id="fnd-f-' + i + '" aria-selected="' + (i === sel) + '">' +
          '<span class="fnd-ic"><i class="fa-solid ' + esc(icon) + '" aria-hidden="true"></i></span>' +
          '<span><span class="fnd-t">' + esc(f.label) + '</span><span class="fnd-p">' + esc(f.path) + '</span></span></a>';
      }).join('') : '<p class="fnd-none">Nothing matches. Try another word.</p>';
      input.setAttribute('aria-activedescendant', shown.length ? 'fnd-f-' + sel : '');
    }

    function close() {
      if (!open_) return;
      scrim.remove();
      open_ = null;
      if (opener && opener.focus) opener.focus();
      if (opts.onClose) opts.onClose();
    }

    function go(f) {
      close();
      (opts.onGo || defaultGo)(f);
    }

    function scrollSel() {
      var a = res.querySelector('[aria-selected="true"]');
      if (a && a.scrollIntoView) a.scrollIntoView({ block: 'nearest' });
    }

    input.addEventListener('input', function () { sel = 0; draw(); });
    input.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown') { e.preventDefault(); sel = Math.min(sel + 1, shown.length - 1); draw(); scrollSel(); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); sel = Math.max(sel - 1, 0); draw(); scrollSel(); }
      else if (e.key === 'Enter') { e.preventDefault(); if (shown[sel]) go(shown[sel]); }
    });
    res.addEventListener('click', function (e) {
      var a = e.target.closest('a');
      if (!a) return;
      e.preventDefault();
      var i = Number(a.id.replace('fnd-f-', ''));
      if (shown[i]) go(shown[i]);
    });
    scrim.addEventListener('pointerdown', function (e) { if (e.target === scrim) close(); });
    scrim.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(); }
      if (e.key === 'Tab') { e.preventDefault(); input.focus(); }
    });
    draw();
    input.focus();
    open_ = { close: close, isOpen: function () { return !!open_; } };
    return open_;
  }

  C.findPalette = { open: open, match: match, isOpen: function () { return !!open_; } };
})();
