/**
 * import_sheet_check.js — a game system's check of the numbers the AI wrote,
 * on an AI Import review row.
 *
 * Mounts via data-widget="import-sheet-check" (reviewRowSheetCheck in
 * import_review.templ), which carries the row's sheet as it would be saved
 * (data-fields), the keys the AI wrote (data-written) and the name of the
 * row's hidden formula input (data-input). The campaign's game system adds
 * Chronicle.sheetChecks[kind] (docs/system-package-rendering.md); with none,
 * or with nothing to say, the mount stays empty and the row is unchanged.
 *
 * Each number gets a tile: "Matches the formula", or "Formula gives N" with a
 * press that takes the formula's number. The hidden input holds the check's
 * patch for the numbers taken, which the commit lays over the AI's. The
 * director decides; nothing is replaced without a press.
 */
(function () {
  'use strict';

  var MAX_ITEMS = 24;
  var INPUT_NAME = /^page_\d+_sheet_formula$/;

  function parse(s, fallback) {
    try {
      var v = JSON.parse(s || '');
      return v === null || v === undefined ? fallback : v;
    } catch (e) {
      return fallback;
    }
  }

  function str(v) {
    return v === null || v === undefined ? '' : String(v);
  }

  // items keeps the check's well-formed entries: an id and a label, and for a
  // number that differs, the formula's figure to offer.
  function items(res) {
    var list = res && Array.isArray(res.items) ? res.items : [];
    var out = [], seen = {};
    for (var i = 0; i < list.length && out.length < MAX_ITEMS; i++) {
      var it = list[i];
      if (!it || typeof it !== 'object') continue;
      var id = str(it.id);
      if (!id || seen[id] || !str(it.label)) continue;
      if (!it.matches && !str(it.formula)) continue;
      seen[id] = true;
      out.push({ id: id, label: str(it.label), ai: str(it.ai), formula: str(it.formula), matches: !!it.matches });
    }
    return out;
  }

  function node(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text !== undefined) n.textContent = text;
    return n;
  }

  function draw(el, input, res) {
    var list = items(res);
    var patch = res && typeof res.patch === 'function' ? res.patch : null;
    if (!list.length) return;
    var off = list.filter(function (it) { return !it.matches; });
    var used = {};

    el.innerHTML = '';
    if (str(res.note)) el.appendChild(node('span', 'air-line', str(res.note)));
    var grid = node('div', 'air-stats');
    grid.setAttribute('role', 'group');
    grid.setAttribute('aria-label', 'Numbers the AI wrote, beside the published formulas');
    el.appendChild(grid);

    var tiles = {};
    list.forEach(function (it) {
      var tile = node(it.matches || !patch ? 'div' : 'button', 'air-stat');
      if (tile.tagName === 'BUTTON') {
        tile.type = 'button';
        tile.setAttribute('data-id', it.id);
        tile.addEventListener('click', function () {
          if (used[it.id]) delete used[it.id]; else used[it.id] = true;
          update();
        });
      }
      tile.appendChild(node('span', '', it.label));
      tile.appendChild(node('b'));
      tile.appendChild(node('small'));
      tiles[it.id] = tile;
      grid.appendChild(tile);
    });

    var all = null;
    if (off.length && patch) {
      all = node('button', 'air-quiet air-stats-all');
      all.type = 'button';
      all.addEventListener('click', function () {
        var every = off.every(function (it) { return used[it.id]; });
        used = {};
        if (!every) off.forEach(function (it) { used[it.id] = true; });
        update();
      });
      el.appendChild(all);
    }

    function update() {
      list.forEach(function (it) {
        var tile = tiles[it.id], b = tile.querySelector('b'), small = tile.querySelector('small');
        var on = !!used[it.id];
        tile.classList.toggle('is-ok', it.matches);
        tile.classList.toggle('is-off', !it.matches && !on);
        tile.classList.toggle('is-used', on);
        b.textContent = on ? it.formula : it.ai;
        small.textContent = it.matches ? 'Matches the formula'
          : on ? 'Formula used (AI wrote ' + it.ai + ')'
          : 'Formula gives ' + it.formula;
        if (tile.tagName === 'BUTTON') {
          tile.setAttribute('aria-pressed', on ? 'true' : 'false');
          tile.title = on ? 'Keep the AI’s number' : 'Use the formula for this one';
        }
      });
      var ids = off.filter(function (it) { return used[it.id]; }).map(function (it) { return it.id; });
      if (all) {
        all.textContent = ids.length === off.length ? 'Keep the AI’s numbers'
          : off.length === 1 ? 'Use the formula' : 'Use the formula for all ' + off.length;
      }
      var value = '';
      if (ids.length && patch) {
        try {
          var p = patch(ids);
          if (p && typeof p === 'object' && Object.keys(p).length) value = JSON.stringify(p);
        } catch (e) {
          console.warn('[import-sheet-check] The game system could not work out the formula numbers:', e);
        }
      }
      if (input) input.value = value;
    }
    update();
  }

  Chronicle.register('import-sheet-check', {
    init: function (el) {
      var checks = Chronicle.sheetChecks || {};
      var check = checks[el.getAttribute('data-sheet-kind') || ''];
      if (!check || typeof check.check !== 'function') return;
      var name = el.getAttribute('data-input') || '';
      var form = el.closest('form') || document;
      var input = INPUT_NAME.test(name) ? form.querySelector('input[name="' + name + '"]') : null;
      var fields = parse(el.getAttribute('data-fields'), {});
      var env = {
        campaignId: el.getAttribute('data-campaign-id') || '',
        written: parse(el.getAttribute('data-written'), [])
      };
      Promise.resolve()
        .then(function () { return check.check(fields, env); })
        .then(function (res) { if (el.isConnected) draw(el, input, res); }, function (err) {
          console.warn('[import-sheet-check] The game system\'s check failed; the row is shown without it:', err);
        });
    },
    destroy: function (el) {
      el.innerHTML = '';
    }
  });
})();
