/**
 * hero_bench.js -- "On the bench", the first container in a hero's pack.
 *
 * The Armory's hero panel draws the empty box (character_panel.templ
 * benchBox); this widget fills it from the Crafting app's own answer for the
 * hero: a tile for each project being made, dashed, filling from the bottom as
 * points grow, with the accent edge when a roll is waiting. Pressing the box
 * drapes the Crafting block down from it over the rest of the page. On a
 * pointer device, resting on the box shows the drape as a peek that leaves
 * with the pointer; a press keeps it open. Escape, the close button or a
 * second press closes it. Touch has no hover, so a tap is the whole path.
 *
 * Who may see the box is decided on the server (the Crafting block's
 * CanCraftFor); this file only draws what the service already let through.
 * The panel reloads itself after a move, which remounts the widget, so which
 * heroes have the drape pinned open is kept for the page.
 */
(function () {
  'use strict';

  var pinned = {};
  var MAX_TILES = 4;
  var PEEK_IN = 140, PEEK_OUT = 200;

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return {'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c];
    });
  }
  function plural(n, one, many) { return n + ' ' + (n === 1 ? one : many); }
  function hoverable() {
    return !!(window.matchMedia && window.matchMedia('(hover: hover) and (pointer: fine)').matches);
  }
  function still() {
    return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) ||
      document.documentElement.classList.contains('nav-rm');
  }

  /* How many recipes the hero has everything for right now: the Crafting
     block's own blockers() says none stand in the way. Without the block's
     script the count is left out rather than guessed. */
  function couldMake(D) {
    var B = window.CraftingBlock;
    if (!B || !B.blockers || !D.recipes || !D.pack) return null;
    var n = 0;
    D.recipes.forEach(function (r) {
      try { if (!B.blockers(r, D.pack, D.character.name).length) n++; } catch (e) { /* skip a recipe it can't read */ }
    });
    return n;
  }

  function tiles(D) {
    var active = (D.projects || []).filter(function (p) { return p.status === 'active'; });
    var ready = 0;
    var out = active.slice(0, MAX_TILES).map(function (p) {
      var ok = p.rollsReady > 0 && !p.waits;
      if (ok) ready += p.rollsReady;
      var pct = p.goal > 0 ? Math.max(0, Math.min(100, Math.round(p.points / p.goal * 100))) : 0;
      return '<span class="hb-tile' + (ok ? ' is-ready' : '') + '" style="--hb-fill:' + pct + '%">' +
        '<span class="hb-name">' + esc(p.name) + '</span>' +
        '<span class="hb-pts">' + Math.min(p.points, p.goal) + ' / ' + p.goal + '</span></span>';
    });
    active.slice(MAX_TILES).forEach(function (p) { if (p.rollsReady > 0 && !p.waits) ready += p.rollsReady; });
    if (active.length > MAX_TILES) out.push('<span class="hb-tile is-more"><span class="hb-name">+' + (active.length - MAX_TILES) + ' more</span></span>');
    var could = couldMake(D);
    if (!active.length) {
      out.push('<span class="hb-tile is-slot"><span class="hb-name">Nothing being made.</span>' +
        (could ? '<span class="hb-pts">' + could + ' you could make.</span>' : '') + '</span>');
    }
    var head = active.length ? active.length + ' being made' : '';
    if (ready) head += (head ? ' · ' : '') + plural(ready, 'roll ready', 'rolls ready');
    return {html: out.join(''), head: head, ready: ready};
  }

  function mount(el, cfg) {
    var cid = String(cfg.campaignId || ''), eid = String(cfg.entityId || '');
    var face = el.querySelector('[data-hb-face]');
    var list = el.querySelector('[data-hb-tiles]');
    var count = el.querySelector('[data-hb-n]');
    var hint = el.querySelector('[data-hb-hint]');
    var wrap = el.closest('.hp-wrap');
    var drape = wrap && wrap.querySelector('[data-hb-drape]');
    if (!face || !list || !drape) return null;

    var dead = false, shown = false, peeking = false, inT = 0, outT = 0, blockEl = null, closeBtn = null;

    function load() {
      var url = '/campaigns/' + encodeURIComponent(cid) + '/crafting/characters/' + encodeURIComponent(eid);
      return Chronicle.apiFetch(url).then(function (res) {
        if (!res.ok) throw new Error('status ' + res.status);
        return res.json();
      }).then(function (D) {
        if (dead) return;
        var t = tiles(D);
        list.innerHTML = t.html;
        if (count) count.textContent = t.head;
        el.classList.toggle('has-ready', t.ready > 0);
      }).catch(function () {
        if (dead) return;
        list.innerHTML = '<span class="hp-empty">Crafting didn’t load.</span>';
      });
    }

    function ensureBlock() {
      if (blockEl) return;
      closeBtn = document.createElement('button');
      closeBtn.type = 'button';
      closeBtn.className = 'hb-close';
      closeBtn.setAttribute('aria-label', 'Close crafting');
      closeBtn.innerHTML = '<i class="fa-solid fa-xmark" aria-hidden="true"></i>';
      closeBtn.addEventListener('click', function () { setOpen(false, true); face.focus({preventScroll: true}); });
      blockEl = document.createElement('div');
      blockEl.setAttribute('data-widget', 'crafting-block');
      blockEl.setAttribute('data-campaign-id', cid);
      blockEl.setAttribute('data-entity-id', eid);
      drape.appendChild(closeBtn);
      drape.appendChild(blockEl);
      Chronicle.mountWidget(blockEl);
    }

    /* The drape hangs from the bench box, at the pack's full width. */
    function seat() { drape.style.top = (el.offsetTop + el.offsetHeight + 6) + 'px'; }

    /* open (true) shows the drape; pin says a press, not a peek, opened it. */
    function setOpen(open, pin) {
      clearTimeout(inT); clearTimeout(outT);
      if (open) {
        peeking = !pin;
        if (pin) pinned[eid] = true;
        if (shown) { face.setAttribute('aria-expanded', pin ? 'true' : 'false'); return; }
        shown = true;
        ensureBlock();
        drape.hidden = false;
        seat();
        drape.classList.remove('is-leaving');
        // A frame after unhiding, so the drape's own transition plays.
        if (!still()) { drape.classList.remove('is-in'); void drape.offsetWidth; }
        drape.classList.add('is-in');
        face.setAttribute('aria-expanded', pin ? 'true' : 'false');
        el.classList.add('is-open');
        if (hint) hint.textContent = 'Close crafting';
        return;
      }
      if (!shown) return;
      shown = false; peeking = false;
      delete pinned[eid];
      face.setAttribute('aria-expanded', 'false');
      el.classList.remove('is-open');
      if (hint) hint.textContent = 'Open crafting';
      var done = function () { if (!shown) drape.hidden = true; drape.classList.remove('is-leaving'); };
      drape.classList.remove('is-in');
      if (still()) done(); else { drape.classList.add('is-leaving'); setTimeout(done, 180); }
      load();
    }

    function onFace() {
      var on = face.getAttribute('aria-expanded') === 'true';
      setOpen(!on, !on);
    }
    function onEnter(e) {
      if (e.pointerType && e.pointerType !== 'mouse') return;
      if (!hoverable() || pinned[eid]) return;
      clearTimeout(outT);
      if (!shown) inT = setTimeout(function () { setOpen(true, false); }, PEEK_IN);
    }
    function onLeave(e) {
      if (e.pointerType && e.pointerType !== 'mouse') return;
      clearTimeout(inT);
      if (shown && peeking && !pinned[eid]) outT = setTimeout(function () { setOpen(false, false); }, PEEK_OUT);
    }
    function onKey(e) {
      if (e.key !== 'Escape' || !shown || e.defaultPrevented) return;
      if (drape.querySelector('.cra [aria-expanded="true"]')) return; // the block closes its own menu first
      setOpen(false, true);
      face.focus({preventScroll: true});
    }

    face.addEventListener('click', onFace);
    el.addEventListener('pointerenter', onEnter);
    el.addEventListener('pointerleave', onLeave);
    drape.addEventListener('pointerenter', onEnter);
    drape.addEventListener('pointerleave', onLeave);
    document.addEventListener('keydown', onKey);
    window.addEventListener('resize', seat);

    load();
    if (pinned[eid]) setOpen(true, true);

    return {
      destroy: function () {
        dead = true;
        clearTimeout(inT); clearTimeout(outT);
        face.removeEventListener('click', onFace);
        el.removeEventListener('pointerenter', onEnter);
        el.removeEventListener('pointerleave', onLeave);
        drape.removeEventListener('pointerenter', onEnter);
        drape.removeEventListener('pointerleave', onLeave);
        document.removeEventListener('keydown', onKey);
        window.removeEventListener('resize', seat);
        if (blockEl && Chronicle.destroyWidget) Chronicle.destroyWidget(blockEl);
      }
    };
  }

  Chronicle.register('hero-bench', {
    init: function (el, cfg) { el._hb = mount(el, cfg || {}); },
    destroy: function (el) { if (el._hb) { el._hb.destroy(); el._hb = null; } }
  });
})();
