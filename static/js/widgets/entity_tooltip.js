/**
 * entity_tooltip.js -- previews of linked pages.
 *
 * Pointing at (or focusing, or long-pressing on touch) any element with a
 * `data-entity-preview` URL opens the shared hover card (hovercard.js) with
 * the page's picture, type, name, up to five fields and the opening lines,
 * as the page's popup_config allows. The card's look and behaviour belong to
 * hovercard.js; this file only fetches and describes the content.
 *
 * Also exposes Chronicle.tooltip (attach/detach/show/hide/clearCache) for
 * other widgets. Previews are LRU-cached (100 entries).
 */
(function () {
  'use strict';

  var MAX_CACHE = 100;
  var cache = {};
  var cacheOrder = []; // Most recently used at the end.

  function cacheGet(url) {
    if (!(url in cache)) return null;
    var idx = cacheOrder.indexOf(url);
    if (idx !== -1) cacheOrder.splice(idx, 1);
    cacheOrder.push(url);
    return cache[url];
  }

  function cacheSet(url, data) {
    if (url in cache) {
      var idx = cacheOrder.indexOf(url);
      if (idx !== -1) cacheOrder.splice(idx, 1);
    } else if (cacheOrder.length >= MAX_CACHE) {
      delete cache[cacheOrder.shift()];
    }
    cache[url] = data;
    cacheOrder.push(url);
  }

  // The preview API's answer, as hover card content.
  function toContent(data, href) {
    var kind = data.type_name || '';
    if (data.type_label) kind += (kind ? ' · ' : '') + data.type_label;
    return {
      kind: kind,
      kindIcon: data.type_icon ? String(data.type_icon).replace(/^fa-solid\s+/, '') : '',
      title: data.name,
      locked: !!data.is_private,
      pic: data.image_path || '',
      facts: (data.attributes || []).map(function (a) { return [a.label, a.value]; }),
      text: data.entry_excerpt || '',
      link: href && href !== '#' ? { href: href, label: 'Open page' } : null
    };
  }

  // Content for a trigger: cached right away, otherwise a fetch the card
  // shows a loading line for. A failed fetch (a deleted or hidden page)
  // closes the card quietly; hovering stale links is common.
  function contentFor(trigger, url) {
    url = url || trigger.getAttribute('data-entity-preview');
    if (!url) return null;
    var href = trigger.getAttribute('href') || '';
    var hit = cacheGet(url);
    if (hit) return toContent(hit, href);
    return Chronicle.apiFetch(url)
      .then(function (res) {
        if (!res.ok) throw new Error('Preview fetch failed: ' + res.status);
        return res.json();
      })
      .then(function (data) {
        cacheSet(url, data);
        return toContent(data, href);
      })
      .catch(function (err) {
        console.warn('[Tooltip] Preview fetch failed:', err);
        return null;
      });
  }

  function start() {
    if (!Chronicle.hovercard) {
      console.error('[Tooltip] hovercard.js must load before entity_tooltip.js');
      return;
    }
    Chronicle.hovercard.bind(document, '[data-entity-preview]', function (t) { return contentFor(t); });
  }
  start();

  // Kept so a container can say it holds previewable links; the document-wide
  // binding above does the work.
  Chronicle.register('entity-tooltip', {
    init: function (el) { el._entityTooltipInit = true; },
    destroy: function (el) { delete el._entityTooltipInit; }
  });

  Chronicle.tooltip = {
    attach: function (element, previewURL) {
      if (element && previewURL) element.setAttribute('data-entity-preview', previewURL);
    },
    detach: function (element) {
      if (!element) return;
      element.removeAttribute('data-entity-preview');
      if (Chronicle.hovercard && Chronicle.hovercard.current() === element) Chronicle.hovercard.close();
    },
    show: function (element, previewURL) {
      var c = contentFor(element, previewURL);
      if (!c) return;
      if (typeof c.then === 'function') {
        Chronicle.hovercard.open(element, { title: element.textContent.trim(), loading: true });
        c.then(function (v) {
          if (Chronicle.hovercard.current() !== element) return;
          if (v) Chronicle.hovercard.update(v); else Chronicle.hovercard.close();
        });
      } else {
        Chronicle.hovercard.open(element, c);
      }
    },
    hide: function () { if (Chronicle.hovercard) Chronicle.hovercard.close(); },
    clearCache: function () { cache = {}; cacheOrder = []; }
  };
})();
