/**
 * import_sheet_check.js — a game system's check of the numbers the AI wrote,
 * on an AI Import review row.
 *
 * Mounts via data-widget="import-sheet-check" (reviewRowSheetCheck in
 * import_review.templ), which carries the row's sheet as it would be saved
 * (data-fields), the keys the AI wrote (data-written) and the name of the
 * row's hidden formula input (data-input). The campaign's game system adds
 * Chronicle.sheetChecks[kind] (docs/system-package-rendering.md); with none,
 * or with a check that fails, the row says so in one line rather than staying
 * silent; a check with nothing to compare leaves the mount empty.
 *
 * Each number gets a tile: "Matches the formula", or "Formula gives N" with a
 * press that takes the formula's number. The hidden input holds the check's
 * patch for the numbers taken, which the commit lays over the AI's.
 *
 * A check may also open a number already taken (start: "used"), for a number
 * the AI left blank or one that followed the formula before; the patch is in
 * the input from the first draw and a press leaves it as it was (blank, or the
 * stored figure). Nothing the director or the AI chose is replaced unless the
 * check says so on the tile and the director can press it back. An item with
 * only a `said` line has no number to offer and is shown as a plain line.
 */
(function () {
  'use strict';

  var MAX_ITEMS = 200;
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
  // number that differs, the formula's figure to offer. Entries past the cap
  // are counted in out.dropped so the row can say they were not shown.
  function items(res) {
    var list = res && Array.isArray(res.items) ? res.items : [];
    var out = [], seen = {};
    out.dropped = 0;
    out.said = [];
    for (var i = 0; i < list.length; i++) {
      var it = list[i];
      if (!it || typeof it !== 'object') continue;
      var id = str(it.id);
      if (!id || seen[id]) continue;
      if (str(it.said) && !str(it.formula)) {
        if (out.said.length < MAX_ITEMS) { seen[id] = true; out.said.push(str(it.said)); }
        continue;
      }
      if (!str(it.label)) continue;
      if (!it.matches && !str(it.formula)) continue;
      if (out.length >= MAX_ITEMS) { out.dropped++; continue; }
      seen[id] = true;
      // from says whose figure shows when the formula is not taken: the AI's,
      // the page's stored one, or none (blank).
      var from = it.blank ? 'blank' : it.from === 'stored' ? 'stored' : 'ai';
      out.push({
        id: id, label: str(it.label), ai: str(it.ai), formula: str(it.formula), matches: !!it.matches,
        start: !it.matches && it.start === 'used', from: from, taken: str(it.taken), kept: str(it.kept)
      });
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
    if (!list.length && !list.said.length) return;
    var off = list.filter(function (it) { return !it.matches; });
    var used = {};
    // Without a patch nothing could be applied, so nothing opens taken.
    var opensTaken = !!patch && off.some(function (it) { return it.start; });
    if (patch) off.forEach(function (it) { if (it.start) used[it.id] = true; });

    el.innerHTML = '';
    if (str(res.note)) el.appendChild(node('span', 'air-line', str(res.note)));
    if (list.dropped) {
      el.appendChild(node('span', 'air-line air-bad', 'Showing the first ' + list.length + ' numbers. ' +
        list.dropped + ' more were not checked, so they are saved as the AI wrote them.'));
    }
    var grid = node('div', 'air-stats');
    grid.setAttribute('role', 'group');
    grid.setAttribute('aria-label', 'Numbers beside the published formulas');
    if (list.length) el.appendChild(grid);

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
        var every = opensTaken ? off.some(function (it) { return used[it.id]; })
          : off.every(function (it) { return used[it.id]; });
        used = {};
        if (!every) off.forEach(function (it) { used[it.id] = true; });
        update();
      });
      el.appendChild(all);
    }

    // The words for putting the formula back down: what a press leaves.
    function keepWord(items) {
      var froms = items.map(function (it) { return it.from; });
      if (froms.every(function (f) { return f === 'blank'; })) return 'blank';
      if (froms.indexOf('ai') < 0) return 'old';
      return 'ai';
    }
    var KEEP_ALL = { blank: 'Leave them blank', old: 'Keep the old figures', ai: 'Keep the AI’s numbers' };
    var KEEP_ONE = { blank: 'Leave this one blank', old: 'Keep the old figure', ai: 'Keep the AI’s number' };

    function update() {
      list.forEach(function (it) {
        var tile = tiles[it.id], b = tile.querySelector('b'), small = tile.querySelector('small');
        var on = !!used[it.id];
        tile.classList.toggle('is-ok', it.matches);
        tile.classList.toggle('is-off', !it.matches && !on);
        tile.classList.toggle('is-used', on);
        var blank = !on && !it.matches && it.from === 'blank';
        b.textContent = on ? it.formula : blank ? 'blank' : it.ai;
        b.classList.toggle('is-blank', blank);
        small.textContent = it.matches ? 'Matches the formula'
          : on ? (it.taken || (it.from === 'blank' ? 'Formula used (was blank)' : 'Formula used (AI wrote ' + it.ai + ')'))
          : (it.kept || (blank ? 'Left blank. Formula gives ' + it.formula : 'Formula gives ' + it.formula));
        if (tile.tagName === 'BUTTON') {
          tile.setAttribute('aria-pressed', on ? 'true' : 'false');
          tile.title = on ? KEEP_ONE[keepWord([it])] : 'Use the formula for this one';
        }
      });
      var ids = off.filter(function (it) { return used[it.id]; }).map(function (it) { return it.id; });
      if (all) {
        var taken = off.filter(function (it) { return used[it.id]; });
        var keepNow = opensTaken ? taken.length > 0 : taken.length === off.length;
        all.textContent = keepNow ? KEEP_ALL[keepWord(taken)]
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
    list.said.forEach(function (text) { el.appendChild(node('span', 'air-line', text)); });
    update();
  }

  // STAT_KINDS are the page kinds whose sheets are stat blocks.
  var STAT_KINDS = { creature: true, character: true };

  // say puts one plain line in the mount when the check cannot run.
  function say(el, text) {
    el.innerHTML = '';
    el.appendChild(node('span', 'air-line', text));
  }

  Chronicle.register('import-sheet-check', {
    init: function (el) {
      var checks = Chronicle.sheetChecks || {};
      var kind = el.getAttribute('data-sheet-kind') || '';
      var check = checks[kind];
      if (!check || typeof check.check !== 'function') {
        // Only stat blocks carry figures a formula could check; on other rows
        // the missing check is no news.
        if (STAT_KINDS[kind]) say(el, 'This game system has no check for these numbers, so they are saved as the AI wrote them.');
        return;
      }
      var name = el.getAttribute('data-input') || '';
      var form = el.closest('form') || document;
      var input = INPUT_NAME.test(name) ? form.querySelector('input[name="' + name + '"]') : null;
      var fields = parse(el.getAttribute('data-fields'), {});
      var env = {
        campaignId: el.getAttribute('data-campaign-id') || '',
        written: parse(el.getAttribute('data-written'), []),
        // The page's fields before the change; {} on a new page.
        stored: parse(el.getAttribute('data-stored'), {})
      };
      Promise.resolve()
        .then(function () { return check.check(fields, env); })
        .then(function (res) { if (el.isConnected) draw(el, input, res); }, function (err) {
          console.warn('[import-sheet-check] The game system\'s check failed; the row is shown without it:', err);
          if (el.isConnected) say(el, 'The game system\'s check could not run, so these numbers are saved as the AI wrote them.');
        });
    },
    destroy: function (el) {
      el.innerHTML = '';
    }
  });
})();
