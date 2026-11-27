/**
 * page_tabs.js -- the tabbed panel under a game-system-rendered page.
 *
 * The server draws the tabs and every piece in its pane
 * (internal/plugins/entities/page_tabs.templ); each piece then loads itself.
 * This widget switches tabs, keeps each tab's count from the rows its piece
 * drew (data-pt-count, or a pane's data-pt-grp groups), and gives the bar one
 * action word that presses the current piece's own add button
 * (data-pt-act) and wears its label, so "New Note" reads "Cancel" while a
 * note is being written.
 *
 * It opens on the first tab with something in it. Pieces arrive one by one,
 * so the choice is remade as they load, but only until the viewer picks a tab
 * or the first few seconds pass: the panel never changes tab under someone.
 * A page refresh that replaces the panel mounts a new copy straight after
 * destroying this one; the tab the viewer was on is kept for it.
 *
 * A game-system sheet can offer a dock: while its own tab row is on screen,
 * the panel's tabs join that row and its panes open in the sheet's pane
 * area, and the panel itself is hidden. A piece the sheet names a slot for
 * (data-sheet-dock-piece, the Items piece on a hero's Equipment tab) goes
 * into that slot and has no tab. When the sheet takes its dock away (a
 * phone-width sheet, a redraw) every node comes back here as it was. The
 * contract is in docs/system-package-rendering.md; a sheet without a dock
 * keeps the panel as it is.
 */
(function () {
var CARRY_MS = 3000;
var carried = null;
// A live refresh compares the page part by part as the server sent it, so
// the panel is whole while one runs and docks again once it has landed.
var refreshing = false;
document.addEventListener('chronicle:page-refreshing', function () { refreshing = true; }, true);
document.addEventListener('chronicle:page-refreshed', function () { refreshing = false; }, true);

Chronicle.register('page-tabs', {
  init: function (el) {
    var AUTO_MS = 4000;
    var tabs = Array.prototype.slice.call(el.querySelectorAll('[data-pt-tab]'));
    var press = el.querySelector('[data-pt-press]');
    var pressLabel = el.querySelector('[data-pt-press-label]');
    var bar = el.querySelector('.pt-tabs');
    var auto = true, queued = false;
    var autoTimer = setTimeout(function () { auto = false; }, AUTO_MS);

    // Panes are found once: docked, they live in the sheet, not in el.
    var panes = {};
    tabs.forEach(function (t) { panes[t.getAttribute('data-pt-tab')] = el.querySelector('[data-pt-pane="' + t.getAttribute('data-pt-tab') + '"]'); });
    function paneOf(tab) { return panes[tab.getAttribute('data-pt-tab')] || null; }
    var allPanes = tabs.map(paneOf).filter(Boolean);

    // While docked: the sheet's root, its slots, and which piece went into a
    // slot of its own (by key). open is the panel's tab open in the sheet.
    var dock = null, open = null, reopen = null, stripTab = null;
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
        // An empty tab shows no number.
        var shown = n > 0 ? String(n) : '';
        if (badge && badge.textContent !== shown) badge.textContent = shown;
        tab.classList.toggle('is-empty', n === 0);
        var isBlank = !settling(pane) && blank(pane);
        var empty = pane.querySelector('[data-pt-empty]');
        if (empty) empty.hidden = !isBlank;
        else tab.hidden = isBlank;
        if (!first && n > 0 && !tab.hidden) first = tab;
      });
      // Docked, the sheet's own tab is open unless the viewer picked one of
      // these; nothing here picks a tab for them.
      if (dock) { syncPress(); return; }
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
        // A piece docked in a slot of the sheet's stays shown there.
        if (p && !(dock && dock.pieces[key(t)])) p.hidden = !on;
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
      if (!press || !pressLabel) return;
      press.hidden = !label;
      if (pressLabel.textContent !== label) pressLabel.textContent = label;
    }

    // Listeners sit on the tabs, the button and the panes themselves, since
    // docking moves them out of el.
    function onTab(ev) {
      var tab = ev.currentTarget;
      stopAuto();
      if (dock) openDocked(tab); else select(tab, false);
    }
    function onPress() {
      var t = target();
      if (t) { stopAuto(); t.click(); }
    }
    // Arrow keys, Home and End move between tabs, as a tab list does. Docked,
    // the sheet's row moves through every tab, these included.
    function onKey(ev) {
      if (dock) return;
      var tab = ev.currentTarget;
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
      if (src && src.setAttribute && src !== el && allPanes.indexOf(src) === -1) src.setAttribute('data-pt-done', '');
      schedule();
    }
    // Typing inside a piece is the viewer at work; the panel holds its tab.
    function onFocus(ev) { if (!ev.target.closest('[data-pt-tab]')) stopAuto(); }

    // ---- The sheet's dock ----
    function say(node, name) { node.dispatchEvent(new CustomEvent(name, { bubbles: true })); }
    function key(tab) { return tab.getAttribute('data-pt-tab'); }
    function letGo() {
      open = null;
      tabs.forEach(function (t) {
        if (dock && dock.pieces[key(t)]) return;
        t.setAttribute('aria-selected', 'false');
        t.tabIndex = -1;
        var p = paneOf(t);
        if (p) p.hidden = true;
      });
      syncPress();
    }
    function openDocked(tab) {
      if (dock.pieces[key(tab)]) return;
      select(tab, false);
      open = key(tab);
      say(tab, 'chronicle:sheet-dock-open');
    }
    function dockTo(root) {
      if (dock || !root || !root.isConnected) return;
      var tabSlot = root.querySelector('[data-sheet-dock-tabs]');
      var paneSlot = root.querySelector('[data-sheet-dock-panes]');
      if (!tabSlot || !paneSlot || !bar || !tabs.length) return;
      var cur = current();
      stripTab = cur ? key(cur) : null;
      var wrap = document.createElement('div');
      wrap.className = 'pt-docked';
      if (press) wrap.appendChild(press);
      dock = { root: root, wrap: wrap, pieces: {} };
      tabs.forEach(function (t) {
        var k = key(t), p = paneOf(t);
        var slot = root.querySelector('[data-sheet-dock-piece="' + k + '"]');
        if (slot && p) {
          // The piece shows in the sheet's own pane; it is no tab panel there.
          dock.pieces[k] = slot;
          p.hidden = false;
          p.classList.add('is-piece');
          p.removeAttribute('role');
          p.removeAttribute('tabindex');
          slot.appendChild(p);
          slot.setAttribute('data-docked', '');
          return;
        }
        tabSlot.appendChild(t);
        if (p) wrap.appendChild(p);
      });
      paneSlot.appendChild(wrap);
      el.hidden = true;
      letGo();
      var back = reopen;
      reopen = null;
      var again = tabs.filter(function (t) { return key(t) === back; })[0];
      if (again && !again.hidden && !dock.pieces[back]) openDocked(again);
      schedule();
    }
    function undock() {
      if (!dock) return;
      reopen = open;
      var d = dock;
      dock = null;
      open = null;
      tabs.forEach(function (t) {
        var k = key(t), p = paneOf(t);
        bar.appendChild(t);
        if (!p) return;
        if (d.pieces[k]) {
          p.classList.remove('is-piece');
          p.setAttribute('role', 'tabpanel');
          p.setAttribute('tabindex', '0');
          d.pieces[k].removeAttribute('data-docked');
        }
        el.appendChild(p);
      });
      if (press) el.appendChild(press);
      if (d.wrap.parentNode) d.wrap.parentNode.removeChild(d.wrap);
      el.hidden = false;
      var want = tabs.filter(function (t) { return key(t) === (reopen || stripTab) && !t.hidden; })[0];
      if (want) select(want, false); else { tabs.forEach(function (t) { t.setAttribute('aria-selected', 'false'); }); update(); }
    }
    function sheetOpenDock() {
      var s = page.querySelector('[data-sheet][data-sheet-dock="open"]');
      if (s && !refreshing) dockTo(s);
    }
    function onDock(ev) { if (!refreshing && page.contains(ev.target)) dockTo(ev.target); }
    // A sheet whose root is already off the page says it from its mount.
    function onUndock(ev) { if (dock && (ev.target === dock.root || !dock.root.isConnected || ev.target.contains(dock.root))) undock(); }
    function onSheetTab(ev) { if (dock && ev.target === dock.root) letGo(); }
    function onRefreshing() { if (dock) undock(); }
    function onRefreshed() { if (el.isConnected) sheetOpenDock(); }

    // The sheet above slides left while a paper is out beside it; the strip
    // takes the same shift so the two stay one column. The sheet stays the
    // authority: this only copies what its folio says. A game system's sheet
    // draws its folio after this widget mounts, and may draw it again, so the
    // folio is looked for whenever the page's content changes until one is
    // there and still on the page.
    var folio = null, fo = null;
    function follow() {
      // A narrow sheet (under 820px wide) does not slide, so neither does the strip.
      var sheet = folio && folio.closest('[data-sheet]');
      var on = !!folio && folio.classList.contains('is-shifted') && !(sheet && sheet.clientWidth < 820);
      if (on) el.style.setProperty('--shift', folio.style.getPropertyValue('--shift') || '0px');
      el.classList.toggle('is-shifted', on);
    }
    function findFolio() {
      if (folio && folio.isConnected) return;
      if (fo) { fo.disconnect(); fo = null; }
      folio = document.querySelector('[data-sheet-folio]');
      if (folio) {
        fo = new MutationObserver(follow);
        fo.observe(folio, { attributes: true, attributeFilter: ['class', 'style'] });
      }
      follow();
    }
    var page = el.closest('#main-content') || document.body;
    var finder = new MutationObserver(findFolio);
    finder.observe(page, { childList: true, subtree: true });
    findFolio();

    tabs.forEach(function (t) { t.addEventListener('click', onTab); t.addEventListener('keydown', onKey); });
    if (press) press.addEventListener('click', onPress);
    var watched = [el].concat(allPanes);
    var mo = new MutationObserver(schedule);
    watched.forEach(function (n) {
      if (n !== el) { n.addEventListener('htmx:afterRequest', onLoaded); n.addEventListener('focusin', onFocus); }
      mo.observe(n, { childList: true, subtree: true, characterData: true });
    });
    el.addEventListener('htmx:afterRequest', onLoaded);
    el.addEventListener('focusin', onFocus);
    var docEvents = [['chronicle:sheet-dock', onDock], ['chronicle:sheet-undock', onUndock], ['chronicle:sheet-tab', onSheetTab],
      ['chronicle:page-refreshing', onRefreshing], ['chronicle:page-refreshed', onRefreshed]];
    docEvents.forEach(function (p) { document.addEventListener(p[0], p[1]); });

    el._ptOff = function () {
      // Everything goes home first, so the page's swap takes the pieces with
      // the panel rather than leaving them in the sheet.
      undock();
      var cur = current();
      var was = reopen;
      carried = was || (!auto && cur) ? { path: location.pathname, tab: was || key(cur), docked: !!was, at: Date.now() } : null;
      clearTimeout(autoTimer);
      mo.disconnect();
      if (fo) fo.disconnect();
      finder.disconnect();
      tabs.forEach(function (t) { t.removeEventListener('click', onTab); t.removeEventListener('keydown', onKey); });
      if (press) press.removeEventListener('click', onPress);
      watched.forEach(function (n) { n.removeEventListener('htmx:afterRequest', onLoaded); n.removeEventListener('focusin', onFocus); });
      docEvents.forEach(function (p) { document.removeEventListener(p[0], p[1]); });
    };
    update();
    var c = carried;
    carried = null;
    if (c && c.path === location.pathname && Date.now() - c.at < CARRY_MS) {
      var keep = el.querySelector('[data-pt-tab="' + c.tab + '"]');
      if (keep && !keep.hidden) { stopAuto(); select(keep, false); }
      if (c.docked) { stopAuto(); reopen = c.tab; }
    }
    sheetOpenDock();
  },
  // A page refresh calls this only when the panel's markup came back the
  // same (page_live.js): the tabs stay as the reader left them, and the
  // widgets inside are refreshed one by one.
  update: function () {},
  destroy: function (el) {
    if (el._ptOff) el._ptOff();
  }
});
})();
