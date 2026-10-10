/**
 * Map block picker
 *
 * The entity page's empty Map block (BlockEntityMapChoose in
 * internal/plugins/maps/entity_map_block.templ) lists the campaign's maps as
 * cards. This file is the behaviour behind its inline handlers: the search
 * filters the cards and keeps a live count, a click selects a card (Pick stays
 * disabled until one is selected), and Pick or a double-click binds the map.
 * Create a map opens a name field that posts the create-and-bind route.
 *
 * Picking posts to the existing widget-binding route (POST /bindings, Scribe+,
 * the same right as editing the page). Its answer is the whole block host
 * re-rendered as the framed preview, swapped outerHTML, so this file makes no
 * permission decision and renders no map itself.
 *
 * The cards are server-rendered; map names reach the DOM only through templ's
 * escaping or textContent here, never innerHTML.
 *
 * The handlers are inline IIFEs (swap-safety: the block arrives by HTMX), so
 * this exposes window.ChronicleMapPicker and adds no delegated listener. It
 * loads on sight through the block's map-picker widget mount (ADR-063).
 */
(function () {
  // ---- Pure helpers (unit-tested in test/js/map_block_picker.test.mjs) ----

  /**
   * filterMaps: the maps whose name contains the search term, case-insensitive,
   * ignoring surrounding spaces. Order is kept; an empty term keeps them all.
   */
  function filterMaps(maps, term) {
    var t = String(term == null ? '' : term).trim().toLowerCase();
    if (!t) return maps.slice();
    return maps.filter(function (m) {
      return String(m.name || '').toLowerCase().indexOf(t) >= 0;
    });
  }

  function plural(n) { return n === 1 ? '1 map' : n + ' maps'; }

  /**
   * countLabel: "N maps" before anything is typed, "N of M maps" while a
   * search narrows the list, so the count says what the grid is showing.
   */
  function countLabel(shown, total, term) {
    var t = String(term == null ? '' : term).trim();
    if (!t) return plural(total);
    return shown + ' of ' + plural(total);
  }

  /** noMatchText: the line shown when the search hides every card. */
  function noMatchText(term) {
    return 'No maps match “' + String(term == null ? '' : term).trim() + '”.';
  }

  var api = { filterMaps: filterMaps, countLabel: countLabel, noMatchText: noMatchText };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof window === 'undefined' || typeof document === 'undefined') return;

  // ---- Browser part ----

  function rootOf(el) { return el && el.closest ? el.closest('[data-map-picker]') : null; }
  function cards(root) { return Array.prototype.slice.call(root.querySelectorAll('.mp-pick-card')); }

  // Pick names the chosen map, so the button says what it will do.
  function syncPick(root) {
    var btn = root.querySelector('[data-pick]');
    if (!btn) return;
    var sel = root.querySelector('.mp-pick-card[aria-pressed="true"]');
    btn.disabled = !sel;
    btn.textContent = sel ? 'Pick ' + (sel.getAttribute('data-map-name') || '') : 'Pick';
  }

  function select(card) {
    var root = rootOf(card);
    if (!root || root.dataset.busy) return;
    cards(root).forEach(function (c) { c.setAttribute('aria-pressed', c === card ? 'true' : 'false'); });
    syncPick(root);
  }

  function filter(input) {
    var root = rootOf(input);
    if (!root) return;
    var all = cards(root);
    var maps = all.map(function (c) { return { name: c.getAttribute('data-map-name') || '', el: c }; });
    var shown = filterMaps(maps, input.value);
    var keep = new Set(shown.map(function (m) { return m.el; }));
    all.forEach(function (c) {
      c.hidden = !keep.has(c);
      // A selection the search hides is dropped, so Pick never binds a map
      // the person can no longer see.
      if (c.hidden) c.setAttribute('aria-pressed', 'false');
    });
    var count = root.querySelector('[data-pick-count]');
    if (count) count.textContent = countLabel(shown.length, all.length, input.value);
    var none = root.querySelector('[data-pick-none]');
    if (none) {
      none.hidden = shown.length > 0;
      none.textContent = shown.length ? '' : noMatchText(input.value);
    }
    syncPick(root);
  }

  // The new preview settles in (ui-standard "Settle"). It runs as a Web
  // Animation because htmx resets the swapped element's class list when it
  // finishes settling, which would cut a CSS class animation short; so it
  // reads html[data-motion] itself: Calm fades in place, Off (which a device
  // asking for reduced motion always is) shows the end state.
  function settle(el) {
    if (!el.animate) return;
    var level = document.documentElement.getAttribute('data-motion');
    if (level === 'off') return;
    if (level === 'calm') { el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 160 }); return; }
    el.animate([{ opacity: 0, transform: 'translateY(6px)' }, { opacity: 1, transform: 'none' }],
      { duration: 280, easing: 'cubic-bezier(0.16, 1, 0.3, 1)' });
  }

  function pick(el, e) {
    var root = rootOf(el);
    if (!root || root.dataset.busy) return;
    // A double-click on a card picks that card; the Pick button picks the
    // selected one.
    var card = el.classList && el.classList.contains('mp-pick-card') ? el
      : root.querySelector('.mp-pick-card[aria-pressed="true"]');
    if (!card || card.hidden) return;
    if (e && e.preventDefault) e.preventDefault();
    if (!window.htmx) return;
    select(card);
    var hostID = root.getAttribute('data-host-block');
    var host = hostID ? document.getElementById(hostID) : null;
    if (!host) return;
    root.dataset.busy = '1';
    var btn = root.querySelector('[data-pick]');
    if (btn) { btn.disabled = true; btn.textContent = 'Picking…'; }
    window.htmx.ajax('POST', '/campaigns/' + encodeURIComponent(root.getAttribute('data-campaign-id')) + '/bindings', {
      target: '#' + hostID,
      swap: 'outerHTML',
      values: {
        host_type: 'entity',
        host_id: root.getAttribute('data-host-id'),
        widget_type: 'map',
        instance_id: card.getAttribute('data-map-id')
      }
    }).then(done, done);
    function done() {
      var next = document.getElementById(hostID);
      if (next && next !== host) {
        settle(next);
        return;
      }
      // Nothing was swapped: the request failed and the global
      // htmx:responseError toast has said why. Put the picker back so the
      // person can try again.
      delete root.dataset.busy;
      syncPick(root);
    }
  }

  // Create a map: the name field opens in place of the button that asked for
  // it, and Cancel puts the button back where focus was.
  function create(el) {
    var root = rootOf(el);
    if (!root || root.dataset.busy) return;
    var form = root.querySelector('[data-pick-new]');
    if (!form) return;
    form.hidden = false;
    var opener = root.querySelectorAll('[data-pick-new-open]');
    for (var i = 0; i < opener.length; i++) opener[i].hidden = true;
    var name = form.querySelector('input[name="name"]');
    if (name) name.focus();
  }

  function cancelCreate(el) {
    var root = rootOf(el);
    if (!root) return;
    var form = root.querySelector('[data-pick-new]');
    if (form) { form.hidden = true; form.reset(); }
    var opener = root.querySelectorAll('[data-pick-new-open]');
    for (var i = 0; i < opener.length; i++) opener[i].hidden = false;
    if (opener.length) opener[opener.length - 1].focus();
  }

  window.ChronicleMapPicker = { filter: filter, select: select, pick: pick, create: create, cancelCreate: cancelCreate };

  // The block's data-widget="map-picker" mount is what makes boot.js load this
  // file on sight; the inline handlers do the work, so the widget itself has
  // nothing to set up.
  if (window.Chronicle && Chronicle.register) {
    Chronicle.register('map-picker', { init: function () {}, destroy: function () {} });
  }
})();
