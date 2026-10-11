/**
 * page_tabs.js -- the tabbed panel under a game-system-rendered page.
 *
 * The server draws the tabs and every piece in its pane
 * (internal/plugins/entities/page_tabs.templ); each piece then loads itself.
 * This widget switches tabs, keeps each tab's count from the rows its piece
 * drew (data-pt-count, or a pane's data-pt-grp groups), and gives the bar one
 * action button that presses the current piece's own add button
 * (data-pt-act) and wears its label, so "New Note" reads "Cancel" while a
 * note is being written.
 *
 * It opens on the first tab with something in it. Pieces arrive one by one,
 * so the choice is remade as they load, but only until the viewer picks a tab
 * or the first few seconds pass: the panel never changes tab under someone.
 */
Chronicle.register('page-tabs', {
  init: function (el) {
    var AUTO_MS = 4000;
    var tabs = Array.prototype.slice.call(el.querySelectorAll('[data-pt-tab]'));
    var press = el.querySelector('[data-pt-press]');
    var pressLabel = el.querySelector('[data-pt-press-label]');
    var auto = true, queued = false;
    var autoTimer = setTimeout(function () { auto = false; }, AUTO_MS);

    function paneOf(tab) { return el.querySelector('[data-pt-pane="' + tab.getAttribute('data-pt-tab') + '"]'); }
    function current() { for (var i = 0; i < tabs.length; i++) if (tabs[i].getAttribute('aria-selected') === 'true') return tabs[i]; return null; }
    function visible(tab) { return !tab.hidden; }

    // A lazy load still in flight; its pane is not judged empty until it lands.
    function pending(pane) {
      var loads = pane.querySelectorAll('[hx-trigger="load"]');
      for (var i = 0; i < loads.length; i++) if (!loads[i].hasAttribute('data-pt-done')) return true;
      return false;
    }
    // A widget that has drawn nothing yet is still fetching.
    function settling(pane) { return pending(pane) || !!pane.querySelector('[data-widget]:empty'); }
    function count(tab, pane) {
      var grps = pane.querySelectorAll('[data-pt-grp]');
      if (grps.length) {
        var n = 0;
        for (var i = 0; i < grps.length; i++) {
          var g = grps[i].querySelectorAll(grps[i].getAttribute('data-pt-grp')).length;
          var none = grps[i].querySelector('[data-pt-none]');
          if (none) none.hidden = g > 0 || pending(grps[i]);
          n += g;
        }
        return n;
      }
      var sel = tab.getAttribute('data-pt-count');
      return sel ? pane.querySelectorAll(sel).length : 0;
    }
    function blank(pane) {
      var empty = pane.querySelector('[data-pt-empty]');
      var txt = '';
      for (var i = 0; i < pane.childNodes.length; i++) {
        var c = pane.childNodes[i];
        if (c !== empty) txt += c.textContent || '';
      }
      return !txt.trim() && !pane.querySelector('button, input, a[href]');
    }

    function update() {
      queued = false;
      var first = null;
      tabs.forEach(function (tab) {
        var pane = paneOf(tab);
        if (!pane) return;
        var n = count(tab, pane);
        var badge = tab.querySelector('[data-pt-n]');
        // Written only on change: the observer below would see every write.
        if (badge && badge.textContent !== String(n)) badge.textContent = String(n);
        tab.classList.toggle('is-empty', n === 0);
        var isBlank = !settling(pane) && blank(pane);
        var empty = pane.querySelector('[data-pt-empty]');
        if (empty) empty.hidden = !isBlank;
        else tab.hidden = isBlank;
        if (!first && n > 0 && !tab.hidden) first = tab;
      });
      var cur = current();
      if (cur && cur.hidden) cur = null;
      if (auto && first && first !== cur) select(first, false);
      else if (!cur) { var v = tabs.filter(visible)[0]; if (v) select(v, false); }
      else syncPress();
    }
    function schedule() {
      if (queued) return;
      queued = true;
      requestAnimationFrame(update);
    }

    function select(tab, focus) {
      tabs.forEach(function (t) {
        var on = t === tab;
        t.setAttribute('aria-selected', on ? 'true' : 'false');
        t.tabIndex = on ? 0 : -1;
        var p = paneOf(t);
        if (p) p.hidden = !on;
      });
      if (focus) tab.focus();
      syncPress();
    }

    // The one action button is the current piece's own add button, shown
    // only while that piece offers one.
    function target() {
      var tab = current();
      var sel = tab && tab.getAttribute('data-pt-act');
      var pane = tab && paneOf(tab);
      return sel && pane ? pane.querySelector(sel) : null;
    }
    function syncPress() {
      var t = target();
      var label = t ? t.textContent.replace(/\s+/g, ' ').trim() : '';
      press.hidden = !label;
      if (pressLabel.textContent !== label) pressLabel.textContent = label;
    }

    function onClick(ev) {
      var tab = ev.target.closest('[data-pt-tab]');
      if (tab && el.contains(tab)) { stopAuto(); select(tab, false); return; }
      if (ev.target.closest('[data-pt-press]')) {
        var t = target();
        if (t) { stopAuto(); t.click(); }
      }
    }
    // Arrow keys, Home and End move between tabs, as a tab list does.
    function onKey(ev) {
      var tab = ev.target.closest('[data-pt-tab]');
      if (!tab) return;
      var list = tabs.filter(visible), i = list.indexOf(tab), next = null;
      if (ev.key === 'ArrowRight') next = list[(i + 1) % list.length];
      else if (ev.key === 'ArrowLeft') next = list[(i - 1 + list.length) % list.length];
      else if (ev.key === 'Home') next = list[0];
      else if (ev.key === 'End') next = list[list.length - 1];
      if (!next) return;
      ev.preventDefault();
      stopAuto();
      select(next, true);
    }
    function stopAuto() { auto = false; clearTimeout(autoTimer); }
    function onLoaded(ev) {
      var src = ev.target;
      if (src && src.setAttribute && el.contains(src)) src.setAttribute('data-pt-done', '');
      schedule();
    }

    el.addEventListener('click', onClick);
    el.addEventListener('keydown', onKey);
    el.addEventListener('htmx:afterRequest', onLoaded);
    // Typing inside a piece is the viewer at work; the panel holds its tab.
    el.addEventListener('focusin', function (ev) { if (!ev.target.closest('[data-pt-tab]')) stopAuto(); });
    var mo = new MutationObserver(schedule);
    mo.observe(el, { childList: true, subtree: true, characterData: true });

    el._ptOff = function () {
      clearTimeout(autoTimer);
      mo.disconnect();
      el.removeEventListener('click', onClick);
      el.removeEventListener('keydown', onKey);
      el.removeEventListener('htmx:afterRequest', onLoaded);
    };
    update();
  },
  destroy: function (el) {
    if (el._ptOff) el._ptOff();
  }
});
