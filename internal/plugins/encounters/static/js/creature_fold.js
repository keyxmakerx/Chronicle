/*
 * creature_fold.js — a creature's numbers, folded out under its row in the
 * encounter builder and the DM Screen's Fight tab.
 *
 * The fold shows the game system's own stat block when the system has one
 * for the page's type (a panel widget, mounted as the creature page mounts
 * it); otherwise it lists the page's filled-in fields, which is what a game
 * system without a stat block widget (D&D 5.5e) stores its numbers in. Every
 * read goes through the routes the page itself uses, so a field the viewer
 * may not see never arrives.
 *
 * Chronicle.CreatureFold.build(opts) makes a fold, opts = {campaignId,
 * entityId, name, statBlocks}; the caller places it and keeps it across
 * redraws, and dispose(box) takes it away. nameButton draws the row's name
 * as the fold's switch.
 */
(function () {
  'use strict';

  window.Chronicle = window.Chronicle || {};
  var C = window.Chronicle;
  var seq = 0;

  function h(tag, cls, text) {
    var el = document.createElement(tag);
    if (cls) el.className = cls;
    if (text != null && text !== '') el.textContent = text;
    return el;
  }

  function getJSON(url) {
    return C.apiFetch(url, { method: 'GET' }).then(function (res) {
      if (!res.ok) throw new Error('HTTP ' + res.status);
      return res.json();
    });
  }

  // statBlockFor picks the system widget that draws this page type, if any.
  function statBlockFor(blocks, typeId) {
    for (var i = 0; i < (blocks || []).length; i++) {
      var b = blocks[i];
      if (b && typeof b.widget === 'string' && Array.isArray(b.typeIds) && b.typeIds.indexOf(typeId) >= 0) return b;
    }
    return null;
  }

  // shownValue is a field's value as one line, or '' for an empty or
  // structured value (a list or object would print as code, not numbers).
  function shownValue(v) {
    if (v === null || v === undefined) return '';
    if (typeof v === 'number' || typeof v === 'boolean') return String(v);
    if (typeof v === 'string') return v.trim();
    return '';
  }

  // fieldRows pairs each field the type defines with its filled value, in the
  // type's order.
  function fieldRows(fields, data) {
    var out = [];
    (fields || []).forEach(function (f) {
      if (!f || !f.key || f.type === 'section' || f.type === 'richtext') return;
      var v = shownValue((data || {})[f.key]);
      if (v) out.push({ label: String(f.label || f.key), value: v });
    });
    return out;
  }

  function pageLink(opts) {
    var a = h('a', 'cf-open', 'Open the page');
    a.href = '/campaigns/' + encodeURIComponent(opts.campaignId) + '/entities/' + encodeURIComponent(opts.entityId);
    return a;
  }

  function drawFields(box, opts) {
    return getJSON('/campaigns/' + encodeURIComponent(opts.campaignId) + '/entities/' + encodeURIComponent(opts.entityId) + '/fields')
      .then(function (v) {
        var rows = fieldRows((v && v.fields) || (v && v.type_fields), v && v.fields_data);
        box.textContent = '';
        if (!rows.length) {
          box.appendChild(h('p', 'cf-note', 'This page has no numbers filled in yet.'));
        } else {
          var dl = h('dl', 'cf-fields');
          rows.forEach(function (r) {
            var d = h('div', 'cf-field');
            d.appendChild(h('dt', '', r.label));
            d.appendChild(h('dd', '', r.value));
            dl.appendChild(d);
          });
          box.appendChild(dl);
        }
        box.appendChild(pageLink(opts));
      });
  }

  function drawStatBlock(box, opts, block) {
    box.textContent = '';
    var mount = h('div', 'cf-sb');
    mount.setAttribute('data-widget', block.widget);
    mount.setAttribute('data-campaign-id', opts.campaignId);
    mount.setAttribute('data-entity-id', opts.entityId);
    mount.setAttribute('data-system-id', block.systemId || '');
    mount.setAttribute('data-is-gm', 'true');
    box.appendChild(mount);
    box.appendChild(pageLink(opts));
    if (C.mountWidget) C.mountWidget(mount);
  }

  function fill(box, opts) {
    var blocks = opts.statBlocks || [];
    var first = blocks.length
      ? getJSON('/api/v1/campaigns/' + encodeURIComponent(opts.campaignId) + '/entities/' + encodeURIComponent(opts.entityId))
        .then(function (e) { return statBlockFor(blocks, e && e.entity_type_id); })
      : Promise.resolve(null);
    return first.then(function (block) {
      if (block) { drawStatBlock(box, opts, block); return null; }
      return drawFields(box, opts);
    }).catch(function () {
      box.textContent = '';
      box.appendChild(h('p', 'cf-note', 'Couldn’t load this creature’s numbers.'));
      box.appendChild(pageLink(opts));
    });
  }

  // build makes a fold and starts filling it. The caller places it, and
  // keeps it across its own redraws so a fold isn't fetched again.
  function build(opts) {
    var box = h('div', 'cf-fold');
    box.id = 'cf-' + (++seq);
    box.setAttribute('role', 'region');
    box.setAttribute('aria-label', (opts.name || 'Creature') + ': numbers');
    box.appendChild(h('p', 'cf-note', 'Loading\u2026'));
    fill(box, opts);
    return box;
  }

  // dispose takes a fold away, unmounting the stat block widget in it.
  function dispose(box) {
    if (!box) return;
    box.querySelectorAll('[data-widget]').forEach(function (n) { if (C.destroyWidget) C.destroyWidget(n); });
    box.remove();
  }

  // nameButton is the row's name as the fold's switch: a chevron and the
  // name, with aria-expanded and aria-controls kept in step with the fold.
  function nameButton(name, open, boxId, onToggle) {
    var b = h('button', 'cf-name');
    b.type = 'button';
    b.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (boxId) b.setAttribute('aria-controls', boxId);
    b.title = open ? 'Hide the numbers' : 'Show the numbers';
    b.appendChild(h('i', 'fa-solid fa-chevron-right cf-chev'));
    b.lastChild.setAttribute('aria-hidden', 'true');
    b.appendChild(h('b', '', name));
    b.addEventListener('click', onToggle);
    return b;
  }

  C.CreatureFold = { build: build, dispose: dispose, nameButton: nameButton, statBlockFor: statBlockFor, fieldRows: fieldRows };
})();
