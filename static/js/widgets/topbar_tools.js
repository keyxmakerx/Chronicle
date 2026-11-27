/**
 * topbar_tools.js — the Tools tab and shelf on a phone's top bar.
 *
 * Mount: <div data-widget="topbar-tools"> drawn by layouts/topbar_tools.templ,
 * only for a viewer with two or more tools. Under 768 wide the bar shows one
 * Tools tab; tapping it hangs a shelf from the bar with every tool and its
 * name. The bar's real tool buttons (AI, the Customize brush, Table, the
 * Store, DM Screen, the eye, the bell)
 * stay in the page with their hosts, only hidden by phone_shell.css, so an
 * entry here presses the real button and the tool opens exactly as it does
 * from the bar. Counts are mirrored from the real badges, never recounted.
 *
 * "Choose what stays on the bar" switches the shelf into a choosing state:
 * up to two tools are kept, shown on the bar again as their real buttons,
 * and remembered per browser. Without storage nothing is remembered and
 * nothing breaks.
 *
 * The shelf closes on a tap outside, on Escape (focus goes back to the tab),
 * on a boosted navigation, and when the window grows past a phone. It drapes
 * from the bar on transform and opacity only; Calm fades and Off just shows
 * (phone_shell.css).
 */
(function () {
  'use strict';
  window.Chronicle = window.Chronicle || {};

  var KEY = 'chronicle-topbar-kept';
  var MAX_KEPT = 2;
  // The tools that can stay on the bar. Find is always there already.
  var KEEPABLE = ['ai', 'brush', 'table', 'store', 'dms', 'pv', 'bell'];
  var REAL = { ai: '[data-ai-chip]', brush: '[data-brush-toggle]', table: '[data-tt-tab]', store: '[data-store-toggle]', dms: '[data-dms-toggle]', bell: '[data-notif-toggle]' };
  var BADGE = { ai: '[data-ai-chip-n]', table: '[data-tt-count]', bell: '[data-notif-badge] [data-count]' };

  // The kept tools out of what storage held: known tools that are on this
  // bar, once each, at most two.
  function readKept(raw, available) {
    var list;
    try { list = JSON.parse(raw || '[]'); } catch (e) { return []; }
    if (!Array.isArray(list)) return [];
    var out = [];
    list.forEach(function (id) {
      if (KEEPABLE.indexOf(id) >= 0 && available.indexOf(id) >= 0 && out.indexOf(id) < 0 && out.length < MAX_KEPT) out.push(id);
    });
    return out;
  }

  // Pressing a tool in the choosing state keeps it, or puts it back. A third
  // one is refused (null), so the caller can say why.
  function toggleKept(kept, id) {
    if (KEEPABLE.indexOf(id) < 0) return null;
    var at = kept.indexOf(id);
    if (at >= 0) return kept.filter(function (k) { return k !== id; });
    if (kept.length >= MAX_KEPT) return null;
    return kept.concat([id]);
  }

  // A badge's words ("3", "9+") as a number; nothing or noise is 0.
  function countOf(text) {
    var n = parseInt(String(text || '').replace(/[^0-9]/g, ''), 10);
    return isNaN(n) ? 0 : n;
  }

  function label(n) { return n > 9 ? '9+' : String(n); }

  // What the tab carries: the counts of the tools it hides. A kept tool
  // shows its own badge on the bar, so it is left out.
  function tabCount(counts, kept) {
    var sum = 0;
    Object.keys(counts).forEach(function (id) { if (kept.indexOf(id) < 0) sum += counts[id] || 0; });
    return sum;
  }

  // What the hint line says while choosing.
  function hintFor(n) {
    if (n >= MAX_KEPT) return 'Two kept. Tap one to put it back.';
    if (n === 1) return 'One kept. You can keep one more.';
    return 'Pick up to two tools to keep on the bar.';
  }

  window.Chronicle._topbarTools = { readKept: readKept, toggleKept: toggleKept, countOf: countOf, label: label, tabCount: tabCount, hintFor: hintFor, KEY: KEY };

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }

  function load() { try { return window.localStorage.getItem(KEY); } catch (e) { return null; } }
  function save(list) { try { window.localStorage.setItem(KEY, JSON.stringify(list)); } catch (e) { /* not remembered */ } }

  window.Chronicle.register('topbar-tools', {
    init: function (host) {
      var bar = document.getElementById('app-topbar');
      var tab = $('[data-tb-tab]', host);
      var shelf = $('[data-tb-shelf]', host);
      var quiet = $('[data-tb-choose]', host);
      var hint = $('[data-tb-hint]', host);
      var tabN = $('[data-tb-tab-n]', host);
      if (!bar || !tab || !shelf || !quiet) return;

      var entries = {};
      $$('[data-tb-for]', shelf).forEach(function (b) { entries[b.getAttribute('data-tb-for')] = b; });
      var isOpen = false, choosing = false, timer = 0, queued = 0, kept = [];

      function toolEl(id) { return $('[data-tb-tool="' + id + '"]', bar); }
      function realButton(id) {
        var el = toolEl(id);
        if (!el) return null;
        return id === 'pv' ? el : $(REAL[id], el);
      }
      function badgeEl(id) {
        var el = toolEl(id);
        return el && BADGE[id] ? $(BADGE[id], el) : null;
      }
      function countFor(id) {
        var b = badgeEl(id);
        if (!b || b.hidden || b.hasAttribute('hidden')) return 0;
        return countOf(b.getAttribute('data-count') || b.textContent);
      }

      // Hidden tools are pressed through the tab on the way back, so focus
      // never falls to nowhere when a tool closes and hands it back.
      function wireFocus(id) {
        var btn = realButton(id);
        if (!btn) return;
        // Only on a phone, where the button is hidden: on a wider bar the
        // button is on show and takes its own focus.
        if (kept.indexOf(id) >= 0) { delete btn.focus; } else {
          btn.focus = function (o) {
            if (wide && wide.matches) HTMLElement.prototype.focus.call(btn, o); else tab.focus(o);
          };
        }
      }

      function applyKept() {
        KEEPABLE.forEach(function (id) {
          var el = toolEl(id);
          if (el) el.classList.toggle('tb-kept', kept.indexOf(id) >= 0);
          wireFocus(id);
        });
        refresh();
      }

      // Entries follow their real tools: present, counted, kept.
      function refresh() {
        queued = 0;
        var counts = {};
        Object.keys(entries).forEach(function (id) {
          var e = entries[id];
          if (id === 'find') { e.hidden = choosing || !document.getElementById('topbar-search-trigger'); return; }
          var el = toolEl(id);
          // A member's AI waits hidden until the plugin allows it.
          e.hidden = !el || !realButton(id) || el.hasAttribute('hidden');
          var n = e.hidden ? 0 : countFor(id);
          counts[id] = n;
          var chip = $('[data-tb-n]', e);
          if (chip) {
            chip.hidden = n === 0;
            chip.textContent = n ? label(n) : '';
            var src = badgeEl(id);
            if (src && src.getAttribute('data-tone')) chip.setAttribute('data-tone', src.getAttribute('data-tone'));
            else chip.removeAttribute('data-tone');
          }
          var isKept = kept.indexOf(id) >= 0;
          e.classList.toggle('is-kept', isKept);
          var state = $('[data-tb-state]', e);
          if (state) state.hidden = !(choosing && isKept);
          if (choosing) {
            e.setAttribute('aria-pressed', isKept ? 'true' : 'false');
            if (!isKept && kept.length >= MAX_KEPT) e.setAttribute('aria-disabled', 'true'); else e.removeAttribute('aria-disabled');
          } else {
            e.removeAttribute('aria-pressed');
            e.removeAttribute('aria-disabled');
          }
        });
        var n = tabCount(counts, kept);
        tabN.hidden = n === 0;
        tabN.textContent = n ? label(n) : '';
        tab.setAttribute('aria-label', n ? 'Tools, ' + n + ' waiting' : 'Tools');
        hint.hidden = !choosing;
        hint.textContent = choosing ? hintFor(kept.length) : '';
        quiet.textContent = choosing ? 'Done' : 'Choose what stays on the bar';
        shelf.classList.toggle('is-choosing', choosing);
      }

      // Mutation bursts (the bell's list, the Table's log) coalesce into one pass.
      function soon() { if (!queued) queued = setTimeout(refresh, 0); }

      function duration() {
        var d = parseFloat(getComputedStyle(shelf).transitionDuration) || 0;
        return d * 1000;
      }

      function open() {
        if (isOpen) return;
        isOpen = true;
        clearTimeout(timer);
        refresh();
        shelf.hidden = false;
        tab.setAttribute('aria-expanded', 'true');
        // Painted once closed-looking, so the transition has a start.
        void shelf.offsetWidth;
        shelf.classList.add('is-open');
      }

      function close(toTab) {
        if (!isOpen) return;
        isOpen = false;
        choosing = false;
        tab.setAttribute('aria-expanded', 'false');
        shelf.classList.remove('is-open');
        refresh();
        var ms = duration();
        clearTimeout(timer);
        if (ms <= 0) shelf.hidden = true;
        else timer = setTimeout(function () { if (!isOpen) shelf.hidden = true; }, ms + 30);
        if (toTab) tab.focus();
      }

      // The real button does the work, a tick after the shelf's own tap has
      // finished, so a tool that closes on an outside click does not see this
      // tap as one.
      function press(id) {
        setTimeout(function () {
          var btn = id === 'find' ? document.getElementById('topbar-search-trigger') : realButton(id);
          if (btn) btn.click();
        }, 0);
      }

      tab.addEventListener('click', function () { if (isOpen) close(false); else open(); });

      shelf.addEventListener('click', function (ev) {
        var b = ev.target.closest('[data-tb-for]');
        if (!b || !shelf.contains(b)) return;
        var id = b.getAttribute('data-tb-for');
        if (choosing) {
          var next = toggleKept(kept, id);
          if (next) { kept = next; save(kept); applyKept(); }
          return;
        }
        close(false);
        press(id);
      });

      quiet.addEventListener('click', function () {
        choosing = !choosing;
        refresh();
      });

      function onPointerDown(ev) { if (isOpen && !host.contains(ev.target)) close(false); }
      function onKey(ev) {
        if (ev.key !== 'Escape' || !isOpen || ev.defaultPrevented) return;
        close(true);
      }
      function onNav(ev) { if (!ev.detail || ev.type !== 'htmx:afterSwap' || ev.detail.boosted) close(false); }
      document.addEventListener('pointerdown', onPointerDown, true);
      document.addEventListener('keydown', onKey);
      // The brush can be put away or brought back on the page.
      document.addEventListener('chronicle:brush', soon);
      ['htmx:afterSwap', 'htmx:pushedIntoHistory', 'htmx:historyRestore'].forEach(function (t) { document.addEventListener(t, onNav); });

      var wide = window.matchMedia ? window.matchMedia('(min-width: 768px)') : null;
      function onWide() { if (wide.matches) close(false); }
      if (wide && wide.addEventListener) wide.addEventListener('change', onWide);

      var watch = null;
      if (window.MutationObserver) {
        watch = new MutationObserver(soon);
        ['ai', 'table', 'bell'].forEach(function (id) {
          var el = toolEl(id);
          if (el) watch.observe(el, { subtree: true, childList: true, attributes: true, characterData: true, attributeFilter: ['hidden', 'data-count', 'data-tone'] });
        });
      }

      kept = readKept(load(), KEEPABLE.filter(function (id) { return !!entries[id]; }));
      applyKept();

      host._tbDestroy = function () {
        clearTimeout(timer);
        clearTimeout(queued);
        document.removeEventListener('pointerdown', onPointerDown, true);
        document.removeEventListener('keydown', onKey);
        document.removeEventListener('chronicle:brush', soon);
        ['htmx:afterSwap', 'htmx:pushedIntoHistory', 'htmx:historyRestore'].forEach(function (t) { document.removeEventListener(t, onNav); });
        if (wide && wide.removeEventListener) wide.removeEventListener('change', onWide);
        if (watch) watch.disconnect();
      };
    },
    destroy: function (host) { if (host && host._tbDestroy) host._tbDestroy(); }
  });
})();
