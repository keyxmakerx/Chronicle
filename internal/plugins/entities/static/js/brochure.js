/**
 * brochure.js -- the Brochure home block's full view.
 *
 * The block's small box (entities/brochure.templ) carries the viewer-safe
 * served HTML of the brochure page inside a closed native <dialog>. Opening
 * lays that HTML out as panels: the cover is the page's title, the line under
 * it and everything before the first heading; each second-level heading starts
 * the next panel. Layout, folds and phone stacking are CSS (.cb-bro in
 * input.css); this file only wraps the sections and opens the dialog.
 *
 * Page links inside keep their data-entity-preview, so the document-wide hover
 * card binding (entity_tooltip.js) previews them here too; hovercard.js seats
 * its card inside the open dialog so it is not left under the top layer.
 *
 * Loaded on sight: the block's section carries data-widget="brochure", which
 * names this script in the entities plugin's registration. Exposes
 * Chronicle.brochure.open(box); the box's inline onclick calls it, so it keeps
 * working after an HTMX swap puts the block on the page.
 */
(function () {
  'use strict';

  var C = window.Chronicle = window.Chronicle || {};
  if (C.brochure) return;

  // A table with a long cell or many columns would be squeezed into one
  // fold, so its panel takes two.
  var WIDE_CELL = 32;
  var WIDE_COLS = 5;

  function tableIsWide(table) {
    var rows = table.rows;
    for (var i = 0; i < rows.length; i++) {
      if (rows[i].cells.length >= WIDE_COLS) return true;
      for (var j = 0; j < rows[i].cells.length; j++) {
        if ((rows[i].cells[j].textContent || '').trim().length > WIDE_CELL) return true;
      }
    }
    return false;
  }

  function el(tag, cls) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    return n;
  }

  // A panel's body is split at its third-level headings; a wide panel with
  // several parts sets them side by side, as a leaflet would.
  function sections(nodes) {
    var out = [], cur = null;
    nodes.forEach(function (n) {
      if (n.nodeName === 'H3' || !cur) {
        cur = el('div', 'cb-bro-sec');
        out.push(cur);
      }
      cur.appendChild(n);
    });
    return out;
  }

  function buildPanel(title, nodes, n, total) {
    var panel = el('section', 'cb-bro-panel');
    panel.setAttribute('data-n', n);
    panel.setAttribute('data-of', total);
    panel.appendChild(title);
    var secs = sections(nodes);
    var body = el('div', 'cb-bro-secs');
    secs.forEach(function (s) { body.appendChild(s); });
    if (secs.length > 1) body.classList.add('cb-bro-secs--many');
    panel.appendChild(body);
    var tables = panel.querySelectorAll('table');
    for (var i = 0; i < tables.length; i++) {
      if (tableIsWide(tables[i])) { panel.classList.add('cb-bro-panel--wide'); break; }
    }
    return panel;
  }

  // build wraps the served HTML into panels, once.
  function build(dialog) {
    var src = dialog.querySelector('[data-brochure-src]');
    if (!src || src.getAttribute('data-built')) return;
    var cover = [], groups = [], cur = null;
    Array.prototype.slice.call(src.childNodes).forEach(function (n) {
      if (n.nodeName === 'H2') {
        cur = { title: n, nodes: [] };
        groups.push(cur);
      } else if (cur) {
        cur.nodes.push(n);
      } else if (n.nodeName !== 'HR' && !(n.nodeType === 3 && !n.textContent.trim())) {
        // A rule under the title would only repeat the cover's own edge.
        cover.push(n);
      }
    });
    var total = groups.length + 1;
    var grid = el('div', 'cb-bro-grid');
    var cp = el('section', 'cb-bro-panel cb-bro-cover');
    cp.setAttribute('data-n', 1);
    cp.setAttribute('data-of', total);
    cover.forEach(function (n) { cp.appendChild(n); });
    grid.appendChild(cp);
    groups.forEach(function (g, i) {
      grid.appendChild(buildPanel(g.title, g.nodes, i + 2, total));
    });
    src.textContent = '';
    src.appendChild(grid);
    // A table that is still too wide for its fold scrolls on its own.
    var tables = src.querySelectorAll('table');
    for (var i = 0; i < tables.length; i++) {
      var w = el('div', 'cb-bro-tw');
      tables[i].parentNode.insertBefore(w, tables[i]);
      w.appendChild(tables[i]);
    }
    src.setAttribute('data-built', '1');
  }

  function open(box) {
    var section = box && box.closest ? box.closest('[data-block="brochure"]') : null;
    var dialog = section && section.querySelector('dialog[data-brochure-dialog]');
    if (!dialog || dialog.open) return;
    build(dialog);
    if (!dialog.hasAttribute('data-bound')) {
      dialog.setAttribute('data-bound', '1');
      // A card opened inside the dialog is seated in it; it must not outlive it.
      dialog.addEventListener('close', function () {
        if (C.hovercard) C.hovercard.close();
        if (box && box.isConnected) box.focus();
      });
    }
    dialog.showModal();
    var body = dialog.querySelector('.cb-bro-body');
    if (body) body.scrollTop = 0;
  }

  C.brochure = { open: open };
  // The mount only brings this file in; the box's own onclick does the work.
  C.register('brochure', { init: function () {}, destroy: function () {} });
})();
