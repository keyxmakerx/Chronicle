/**
 * hero_looks.js — the Customizer's "Hero creator" page: the real hero creator
 * in a frame, with the campaign's real cards, where the owner presses a card
 * to change its border. A panel on the right (a sheet from the foot on a
 * phone) holds the card's kind and name, Previous and Next, and the shared
 * sample wall and colours (hero_border_picker.js). Changes wait in the
 * Customizer's one draft, section "herolooks":
 *
 *   { looks: [{ field, source, key, border, colour } | { field, source: 'package', key, reset: true }],
 *     _changes: [{ label, href }] }
 *
 * one item and one _changes entry per card that differs from what the server
 * served, in step order and then card order. The server's contract is
 * internal/app/customize_herolooks.go.
 *
 * The creator is made here, inside the frame, with the hooks the workshop
 * class reads (hero_workshop.js, see its header): the page owns the colouring
 * script shared by the cards and the samples, and the creator never makes a
 * hero, writes an entry or leaves the page. Everything from the draft or the
 * plan is untrusted: names go through Chronicle.escapeHtml, and a border or a
 * colour reaches markup only through the picker and borders.attrs after it has
 * been checked against the library and #rrggbb.
 *
 * Markup: heroLooksSection (hero_looks_section.templ). Styles: hero_looks.css.
 */
(function () {
  'use strict';

  var SECTION = 'herolooks';
  var HEX = /^#[0-9a-f]{6}$/i;

  function has(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }
  function low(v) { return String(v).toLowerCase(); }
  function esc(v) { return Chronicle.escapeHtml(v); }
  function parse(s) { try { return JSON.parse(s || 'null'); } catch (e) { return null; } }
  function icon(n) { return '<i class="fa-solid fa-' + n + '" aria-hidden="true"></i>'; }
  function gm() { return window.Chronicle && Chronicle.gmTerm ? Chronicle.gmTerm() : 'Director'; }
  // The site's motion level as the creator reads it: lively, gentle (Calm) or still (Off).
  function level() { return window.ChronicleHeroCreator && window.ChronicleHeroCreator.motion ? window.ChronicleHeroCreator.motion() : 'lively'; }

  function create(el) {
    var cid = el.getAttribute('data-campaign-id') || '';
    var planUrl = el.getAttribute('data-plan-url') || '';
    var tabUrl = el.getAttribute('data-tab-url') || '';
    var lib = parse(el.getAttribute('data-borders')) || {};
    var pickLib = parse(el.getAttribute('data-border-picker')) || { groups: [], borders: {} };
    var editor = el.querySelector('[data-hl-editor]'), canvas = el.querySelector('[data-hl-canvas]'), fitEl = el.querySelector('[data-hl-fit]');
    var frame = el.querySelector('[data-hl-frame]'), cap = el.querySelector('[data-hl-cap]'), insp = el.querySelector('[data-hl-insp]');
    var dock = el.querySelector('[data-hl-dock]'), empty = el.querySelector('[data-hl-empty]');
    if (!editor || !canvas || !fitEl || !frame || !cap || !insp || !dock) return null;

    // One colouring script for the page, rooted where the cards and the
    // samples both live, so a border in a colour is made once.
    var borders = window.ChronicleHeroBorders(lib, el);
    var picker = window.ChronicleBorderPicker(pickLib, borders);

    var alive = true, timers = [], listeners = [];
    var V = { cr: null, mount: null, sel: null, userView: null, view: 'desk', nw: 1180, nh: 700, s: 1, wasSmall: null };
    // cards: identity -> { c, st, id }, in step order and then card order.
    var cards = {}, order = [], base = {}, sys = {};
    var pend = false, raf = 0, hooks;

    function on(t, type, fn, o) { t.addEventListener(type, fn, o); listeners.push([t, type, fn, o]); }
    function later(fn, ms) { var id = setTimeout(function () { if (alive) fn(); }, ms); timers.push(id); return id; }
    function $(s, r) { return (r || el).querySelector(s); }
    function $$(s, r) { return Array.prototype.slice.call((r || el).querySelectorAll(s)); }
    function small() { return canvas.clientWidth <= 640; }
    function savebar() { return document.querySelector('[data-czs-savebar]'); }

    // ---------- looks ----------
    function known(key) { return typeof key === 'string' && has(pickLib.borders || {}, key); }
    function ownColour(key) { var c = pickLib.borders[key] && pickLib.borders[key].colour; return HEX.test(c) ? low(c) : '#888888'; }
    // A look as it may be drawn: a library border in six hex digits, else null.
    function norm(look) {
      if (!look || typeof look !== 'object' || !known(look.border)) return null;
      return { border: look.border, colour: HEX.test(look.colour) ? low(look.colour) : ownColour(look.border) };
    }
    function same(a, b) { return picker.same(a, b); }
    function ident(st, c) { return JSON.stringify([st.fieldKey, c.source, c.key == null ? null : c.key]); }
    // Only a game system's card, or a campaign's own entry a Director has accepted, can be dressed here.
    function dressable(k) {
      var c = k.c;
      if (c.key == null || c.key === '') return false;
      if (c.source === 'package') return true;
      return c.source === 'campaign' && c.status !== 'draft' && c.status !== 'sent';
    }
    function lookOf(k) { return norm(k.c.look); }
    function setLook(k, look) { if (look) k.c.look = { border: look.border, colour: look.colour }; else delete k.c.look; }
    function sysOf(k) { return k.c.source === 'package' ? sys[k.id] : undefined; }

    // ---------- the plan: what the server served, and the draft over it ----------
    function record(plan) {
      cards = {}; order = []; base = {}; sys = {};
      (plan.steps || []).forEach(function (st) {
        (st.choices || []).forEach(function (c) {
          var id = ident(st, c);
          if (has(cards, id)) return;
          var k = { c: c, st: st, id: id };
          cards[id] = k; order.push(id);
          var now = norm(c.look);
          setLook(k, now);
          base[id] = now;
          if (c.source === 'package') sys[id] = norm(c.lookChanged ? c.systemLook : c.look);
        });
      });
    }

    // The look a draft item gives its card, null for none, undefined when the
    // item is not one this page would make (it is ignored).
    function itemLook(it, k) {
      if (it.reset === true) return k.c.source === 'package' ? sys[k.id] : undefined;
      if (typeof it.border !== 'string') return undefined;
      if (it.border === '') return null;
      if (!known(it.border)) return undefined;
      if (it.colour === '' || it.colour == null) return { border: it.border, colour: ownColour(it.border) };
      if (typeof it.colour !== 'string' || !HEX.test(it.colour)) return undefined;
      return { border: it.border, colour: low(it.colour) };
    }

    // Every card's look: as served, then the draft's items over it.
    function lay(sec) {
      order.forEach(function (id) { setLook(cards[id], base[id]); });
      var items = sec && typeof sec === 'object' && Array.isArray(sec.looks) ? sec.looks : [];
      items.forEach(function (it) {
        if (!it || typeof it !== 'object' || typeof it.field !== 'string') return;
        var k = cards[JSON.stringify([it.field, it.source, it.key == null ? null : it.key])];
        if (!k || !dressable(k)) return;
        var look = itemLook(it, k);
        if (look !== undefined) setLook(k, look);
      });
    }
    function draftSection() {
      var d = window.Chronicle && Chronicle.customizeDraft ? Chronicle.customizeDraft() : null;
      return d && typeof d === 'object' ? d[SECTION] : null;
    }

    function itemOf(k) {
      var look = lookOf(k), c = k.c;
      if (c.source === 'package' && same(look, sys[k.id])) return { field: k.st.fieldKey, source: 'package', key: c.key, reset: true };
      return { field: k.st.fieldKey, source: c.source, key: c.key, border: look ? look.border : '', colour: look && !picker.isOwn(look) ? look.colour : '' };
    }
    // Writes the whole section: one item for each card that differs from the server's.
    function write(merge) {
      var looks = [], changes = [];
      order.forEach(function (id) {
        var k = cards[id];
        if (!dressable(k) || same(lookOf(k), base[id])) return;
        looks.push(itemOf(k));
        changes.push({ label: 'Hero creator: ' + k.c.name, href: tabUrl });
      });
      if (!window.Chronicle || !Chronicle.customizeSet) return;
      Chronicle.customizeSet(SECTION, looks.length ? { looks: looks, _changes: changes } : null, merge ? undefined : { step: true });
    }

    // ---------- cards in the frame ----------
    var CARD = ':is(.hc-gcard, .hc-card)[data-i]';
    function cardEl(i) { return V.cr ? V.cr.root.querySelector(':is(.hc-gcard, .hc-card, .hc-lrow)[data-i="' + i + '"]') : null; }
    function entryAt(i) { var cr = V.cr; return cr && cr.plan ? cr.entries(cr.step)[i] : null; }
    function metaOf(c) { var cr = V.cr; return c && cr && cr.plan ? cards[ident(cr.cur(), c)] || null : null; }
    function selMeta() { return V.sel ? metaOf(entryAt(V.sel.i)) : null; }
    function fitHand() { var cr = V.cr; if (cr && cr.$('[data-hand]')) cr.fit(); }
    // Writes a card's look onto its element the way the creator draws it.
    function paintCard(k) {
      var cr = V.cr, i = cr && cr.plan ? cr.entries(cr.step).indexOf(k.c) : -1, n = i >= 0 ? cardEl(i) : null;
      if (!n || n.classList.contains('hc-lrow')) return;
      picker.paint(n, lookOf(k));
      fitHand();
    }
    function paintAll() {
      var cr = V.cr;
      if (!cr || !cr.plan) return;
      var list = cr.entries(cr.step);
      $$(CARD, cr.root).forEach(function (n) {
        var c = list[+n.getAttribute('data-i')], k = metaOf(c);
        if (k) picker.paint(n, lookOf(k));
      });
      fitHand();
    }

    // ---------- sizes: the frame is as large as the workshop allows ----------
    // The editor reaches the foot of the window, so the canvas and the panel
    // scroll on their own.
    function fitEditor() {
      var host = el.closest('#main-content'), hs = host ? getComputedStyle(host) : null;
      var shell = el.closest('.czs'), ss = shell ? getComputedStyle(shell) : null;
      var top = editor.getBoundingClientRect().top + window.scrollY + (host ? host.scrollTop : 0);
      var foot = Math.max(0, (hs ? parseFloat(hs.paddingBottom) || 0 : 0) + (ss ? parseFloat(ss.marginBottom) || 0 : 0));
      editor.style.height = Math.max(420, Math.round(window.innerHeight - top - 8 - foot)) + 'px';
    }
    function layout() {
      if (!alive) return;
      fitEditor();
      var sm = small();
      if (sm !== V.wasSmall) {
        V.wasSmall = sm; caption(); drawDock();
        if (V.sel) openPanel();
      }
      var cw = canvas.clientWidth, ch = canvas.clientHeight, capH = cap.offsetHeight + (sm ? 8 : 10);
      var availW = cw - (sm ? 0 : 32), availH = ch - (sm ? 8 : 12) - capH - (sm ? 0 : 72);
      var view = V.userView || (cw >= 760 ? 'desk' : 'phone'), nw, nh, s;
      if (view === 'desk') {
        nw = 1180; s = Math.min(1, availW / nw); nh = availH / s;
        if (nh < 560) { nh = 560; s = Math.min(s, availH / nh); }
      } else if (sm) {
        // A phone shows the creator at the height a player's phone gives it; the workshop scrolls to reach the rest.
        nw = cw; nh = Math.max(560, window.innerHeight - 48); s = 1;
      } else {
        // A phone on a computer: as tall as a common phone when there is room, never drawn under four fifths.
        nw = 390; nh = Math.max(620, Math.min(760, availH / 0.8)); s = Math.min(1, availH / nh);
      }
      if (!(s > 0)) s = 1;
      nw = Math.round(nw); nh = Math.round(nh);
      V.view = view; V.nw = nw; V.nh = nh; V.s = s;
      hooks.view = view; hooks.height = nh;
      el.setAttribute('data-view', view);
      fitEl.style.setProperty('--fw', Math.round(nw * s) + 'px'); fitEl.style.setProperty('--fh', Math.round(nh * s) + 'px');
      frame.style.setProperty('--nw', nw + 'px'); frame.style.setProperty('--nh', nh + 'px'); frame.style.setProperty('--s', String(s));
      var cr = V.cr;
      if (cr && cr.plan) { if (cr.measure()) cr.render(); else cr.fit(); }
      shift();
    }

    // The frame steps aside (transform only) so the pressed card clears the
    // panel. On a phone the workshop scrolls instead: its foot is padded by
    // whatever lies over it, and the pressed card comes to the top, clear of
    // the sheet.
    function shift() {
      if (!alive) return;
      var n = V.sel ? cardEl(V.sel.i) : null, x = 0, cv = canvas.getBoundingClientRect();
      fitEl.style.transform = 'none';
      if (small()) {
        var sb = savebar(), cover = V.sel ? insp.offsetHeight : (dock.parentNode.hidden ? 0 : 64) + (sb && !sb.hidden ? 76 : 0);
        canvas.style.setProperty('--hcw-cover', (cover + 16) + 'px');
        if (n && V.sel) {
          var r0 = n.getBoundingClientRect();
          if (r0.bottom - cv.top > cv.height - cover || r0.top < cv.top + 8) {
            canvas.scrollTo({ top: Math.max(0, canvas.scrollTop + r0.top - cv.top - 18), behavior: level() === 'lively' ? 'smooth' : 'auto' });
          }
        }
        return;
      }
      canvas.style.removeProperty('--hcw-cover');
      if (n) {
        var r = n.getBoundingClientRect(), edge = cv.right - 360 - 12 - 16;
        if (r.right > edge) x = r.right - edge;
        var fr = fitEl.getBoundingClientRect();
        if (V.view === 'phone' && fr.right > edge) x = Math.max(x, (fr.right - edge + fr.left - cv.left - 16) / 2);
      }
      fitEl.style.transform = x ? 'translate(' + (-Math.round(x)) + 'px, 0)' : '';
    }

    // ---------- the tools at the foot, and the line above the frame ----------
    function drawDock() {
      // A phone shows the creator as a phone does; there is no second view to switch to.
      var h = small() ? '' : '<button type="button" class="cpe-dk" data-hl="view-desk" aria-pressed="' + (V.view === 'desk') + '">Computer</button>' +
        '<button type="button" class="cpe-dk" data-hl="view-phone" aria-pressed="' + (V.view === 'phone') + '">Phone</button>';
      dock.innerHTML = h;
      dock.parentNode.hidden = !h;
    }
    function caption() {
      var t = (small() ? '' : 'The real creator, as your players see it. ') + 'Press a card to change its border.';
      if (cap.textContent !== t) cap.textContent = t;
    }
    function setView(v) { deselect(false); V.userView = v; layout(); drawDock(); }

    // ---------- pressing a card ----------
    function press(i) {
      var c = entryAt(i);
      if (!alive || !c) return;
      if (V.sel && V.sel.i === i && V.sel.step === V.cr.step) return;
      select(i);
    }
    // The arrow keys move the creator's focus; an open panel follows it.
    function follow(i) { if (alive && V.sel && V.sel.i !== i && entryAt(i)) select(i); }
    function drawn() {
      if (!alive || !V.cr) return;
      if (V.sel && V.sel.step !== V.cr.step) { deselect(false); return; }
      mark();
    }
    function mark() {
      var cr = V.cr;
      el.classList.toggle('is-picking', !!V.sel);
      if (!cr) return;
      $$('.hcw-on', cr.root).forEach(function (n) { n.classList.remove('hcw-on', 'is-reading'); n.removeAttribute('aria-expanded'); });
      var n = V.sel ? cardEl(V.sel.i) : null;
      // The card says its panel is open in a word as well as by its ring.
      if (n) { n.classList.add('hcw-on'); n.setAttribute('aria-expanded', 'true'); if (n.classList.contains('hc-card')) n.classList.add('is-reading'); }
    }
    function select(i) {
      var cr = V.cr, was = V.sel;
      if (pend) flush(true);
      V.sel = { step: cr.step, i: i };
      if (!cr.desk() && cr.$('[data-hand]')) { cr.at[cr.cur().fieldKey] = i; cr.scrollHandTo(i, !!was); cr.refreshUnder(); }
      mark();
      toMiddle(i);
      openPanel();
      later(shift, was ? 0 : 30);
      if (!was) later(toPicker, 60);
    }
    // Opening the panel hands it the keyboard, on the sample in use; Escape or
    // Done hands it back to the card. The list is moved by hand so nothing but
    // the list scrolls.
    function toPicker() {
      var t = $('.hcw-t[aria-pressed="true"]', insp) || $('.hcw-t', insp), wall = t && t.closest('[data-wall]');
      if (!t) { insp.focus({ preventScroll: true }); return; }
      if (wall && wall.scrollHeight > wall.clientHeight) wall.scrollTop = Math.max(0, t.getBoundingClientRect().top - wall.getBoundingClientRect().top + wall.scrollTop - (wall.clientHeight - t.offsetHeight) / 2);
      t.focus({ preventScroll: true });
    }
    // The pressed card comes to the middle of the creator's own list, clear of its edges.
    function toMiddle(i) {
      var cr = V.cr, grid = cr.$('[data-grid]'), n = cardEl(i);
      if (!grid || !n) return;
      var er = n.getBoundingClientRect(), gr = grid.getBoundingClientRect();
      var to = grid.scrollTop + (er.top - gr.top) / V.s - (grid.clientHeight - n.offsetHeight) / 2;
      later(function () { grid.scrollTo({ top: Math.max(0, to), behavior: level() === 'lively' ? 'smooth' : 'auto' }); }, 0);
    }
    // refocus gives the keyboard back to the card, for Escape and Done.
    function deselect(refocus) {
      if (!V.sel) return;
      if (pend) flush(true);
      var n = cardEl(V.sel.i);
      V.sel = null;
      mark();
      insp.classList.remove('open'); editor.classList.remove('panel-open');
      shift();
      if (refocus && n && n.focus) n.focus({ preventScroll: true });
    }
    function move(d) {
      var cr = V.cr, n = cr.entries(cr.step).length, i = V.sel ? V.sel.i + d : 0;
      if (i < 0 || i >= n) return;
      // A button of the panel keeps the keyboard while the cards are walked.
      var a = document.activeElement, held = a && insp.contains(a) ? a.getAttribute('data-hl') : null;
      if (cr.desk()) cr.setFocus(i, true); else select(i);
      var back = held && $('[data-hl="' + held + '"]', insp);
      if (back) back.focus({ preventScroll: true });
    }

    // ---------- the panel ----------
    function headHTML(k, extra) {
      return '<div class="hcw-who"><span class="hcw-kind">' + esc(k.st.label) + '</span><h3>' + esc(k.c.name) + '</h3></div>' +
        '<button type="button" class="cpe-insp-x" data-hl="prev" aria-label="Previous card" title="Previous card">' + icon('chevron-left') + '</button>' +
        '<button type="button" class="cpe-insp-x" data-hl="next" aria-label="Next card" title="Next card">' + icon('chevron-right') + '</button>' + (extra || '');
    }
    function drawHead(k) {
      var h = $('[data-head]', insp);
      if (!h) return;
      h.innerHTML = headHTML(k, '<button type="button" class="cpe-insp-x" data-hl="close" aria-label="Close" title="Close (Esc)">' + icon('xmark') + '</button>' +
        '<button type="button" class="hcw-word hcw-done" data-hl="close">Done</button>');
      var n = V.cr.entries(V.cr.step).length;
      // The end of a run is said and shown, and the button keeps the keyboard.
      var a = $('[data-hl="prev"]', h), b = $('[data-hl="next"]', h);
      if (a) a.setAttribute('aria-disabled', String(V.sel.i <= 0));
      if (b) b.setAttribute('aria-disabled', String(V.sel.i >= n - 1));
    }
    function stateOf(k) { return { look: lookOf(k), sys: sysOf(k) }; }
    function openPanel() {
      var k = selMeta();
      if (!k) return;
      var a = document.activeElement, held = a && insp.contains(a) ? a.getAttribute('data-hl') : null, inside = !!(a && insp.contains(a));
      var body;
      if (dressable(k)) {
        body = picker.html(stateOf(k), { none: 'None', sysLabel: (V.cr.plan.systemName || 'Game system') + '’s', lay: 'wall', split: true, top: small() });
      } else if (k.c.key == null || k.c.key === '') {
        body = '<div class="hcw-pending"><p class="hcw-note">This card’s border can’t be changed here.</p></div>';
      } else {
        body = '<div class="hcw-pending"><p class="hcw-note">This entry is not accepted yet. Its writer picks its border until a ' + gm() + ' accepts it.</p></div>';
      }
      insp.innerHTML = '<div class="cpe-insp-tabs hcw-ih"><span class="cpe-handle" aria-hidden="true"></span><div class="cpe-insp-head" data-head></div></div>' + body;
      drawHead(k);
      insp.classList.add('open'); editor.classList.add('panel-open');
      if (inside) {
        var back = held && $('[data-hl="' + held + '"]', insp);
        if (back) back.focus({ preventScroll: true }); else toPicker();
      }
    }
    // The card was changed in the panel: the card, the panel and the draft follow.
    function change(pk, look, merge) {
      var k = selMeta();
      if (!alive || !k || !dressable(k)) return;
      var now = norm(look);
      if (same(lookOf(k), now)) return;
      setLook(k, now);
      if (!merge) { flush(false); return; }
      // A colour being dragged changes thirteen samples at every step: it is held to one update a frame.
      pend = true;
      if (!raf) raf = requestAnimationFrame(function () { raf = 0; if (alive && pend) flush(true); });
    }
    function flush(merge) {
      if (raf) { cancelAnimationFrame(raf); raf = 0; }
      pend = false;
      var k = selMeta();
      if (!k) return;
      paintCard(k);
      var pk = $('[data-pk]', insp);
      if (pk) picker.sync(pk, stateOf(k));
      write(merge);
    }
    var unbind = picker.bind(insp, {
      state: function () { var k = selMeta(); return k && dressable(k) ? stateOf(k) : null; },
      change: change
    });

    // ---------- the draft changed under the page ----------
    function repaintAll() {
      paintAll();
      var k = selMeta(), pk = $('[data-pk]', insp);
      if (k && pk) picker.sync(pk, stateOf(k));
    }
    function onDraft(ev) {
      var d = ev.detail || {}, r = d.reason;
      if (!alive || !V.cr || !V.cr.plan) return;
      if (['undo', 'redo', 'discard', 'restore', 'publish'].indexOf(r) < 0) return;
      if (raf) { cancelAnimationFrame(raf); raf = 0; }
      pend = false;
      if (r === 'publish' && Array.isArray(d.applied) && d.applied.indexOf(SECTION) >= 0) { deselect(false); reload(); return; }
      lay(d.draft && typeof d.draft === 'object' ? d.draft[SECTION] : null);
      repaintAll();
    }
    // After a Save the plan is read again, so what is served (the looks, what
    // a reset gives back) is the server's, and the step and the view stay.
    function reload() {
      var get = window.Chronicle && Chronicle.apiFetch ? Chronicle.apiFetch(planUrl) : fetch(planUrl, { credentials: 'same-origin' });
      Promise.resolve(get).then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; }).then(function (plan) {
        if (!alive || !V.cr || !V.cr.plan) return;
        if (plan && Array.isArray(plan.steps)) {
          var fresh = {};
          plan.steps.forEach(function (st) { (st.choices || []).forEach(function (c) { fresh[ident(st, c)] = c; }); });
          order.forEach(function (id) {
            var k = cards[id], f = fresh[id];
            if (!f) return;
            base[id] = norm(f.look);
            if (k.c.source === 'package') sys[id] = norm(f.lookChanged ? f.systemLook : f.look);
            k.c.lookChanged = !!f.lookChanged;
            if (f.systemLook) k.c.systemLook = f.systemLook; else delete k.c.systemLook;
          });
        } else {
          // The page could not ask: what was saved is taken as served.
          order.forEach(function (id) { base[id] = lookOf(cards[id]); });
        }
        lay(draftSection());
        repaintAll();
      });
    }

    // ---------- the creator, in the frame ----------
    hooks = {
      view: 'desk', height: 700, borders: borders,
      loaded: function (cr) {
        if (!alive) return;
        V.cr = cr;
        if (!(cr.plan.steps || []).length) { showEmpty(); return; }
        record(cr.plan);
        lay(draftSection());
      },
      press: press, follow: follow, drawn: drawn
    };
    function showEmpty() {
      if (empty) empty.hidden = false;
      editor.hidden = true;
      // The creator has drawn nothing yet; it is taken down once its own load is through.
      later(function () { if (V.mount && window.Chronicle && Chronicle.destroyWidget) Chronicle.destroyWidget(V.mount); }, 0);
    }
    function makeMount() {
      var m = document.createElement('div');
      m.className = 'hc';
      [['data-widget', 'hero-creator'], ['data-look', 'stage'], ['data-workshop', ''], ['data-campaign-id', cid], ['data-plan-url', planUrl],
        ['data-create-url', ''], ['data-csrf', ''], ['data-type-name', 'character'], ['data-for-self', 'true'], ['data-borders', el.getAttribute('data-borders') || '{}']
      ].forEach(function (a) { m.setAttribute(a[0], a[1]); });
      m.innerHTML = '<div class="hc-loading" role="status"><p class="hc-loading__text">Opening the creator…</p></div>';
      m.heroWorkshop = hooks;
      frame.appendChild(m);
      V.mount = m;
      if (window.Chronicle && Chronicle.mountWidgets) Chronicle.mountWidgets(frame);
    }

    // ---------- events ----------
    on(el, 'click', function (ev) {
      var t = ev.target, x = t.closest && t.closest('[data-hl]');
      if (x && el.contains(x)) {
        ev.preventDefault();
        var name = x.getAttribute('data-hl');
        if (name === 'close') deselect(true);
        else if (name === 'prev') move(-1);
        else if (name === 'next') move(1);
        else if (name === 'view-desk') setView('desk');
        else if (name === 'view-phone') setView('phone');
        return;
      }
      // A press on the workshop's ground puts the panel away.
      if (V.sel && t.closest('[data-hl-canvas]') && !t.closest('[data-hl-frame]')) deselect(false);
    });
    // Nothing in the frame leads away: a link in the creator stays on this page.
    on(frame, 'click', function (ev) { if (ev.target.closest && ev.target.closest('a[href]')) ev.preventDefault(); }, true);
    on(frame, 'dblclick', function (ev) { ev.preventDefault(); });
    on(document, 'keydown', function (ev) {
      if (ev.key !== 'Escape' || !V.sel) return;
      var dlg = ev.target.closest && ev.target.closest('[role="dialog"], dialog');
      if (dlg && !el.contains(dlg)) return;
      ev.preventDefault();
      deselect(true);
    }, true);
    on(document, 'chronicle:customize-draft', onDraft);
    on(window, 'resize', function () { requestAnimationFrame(layout); });
    var ro = null, mo = null;
    if (window.ResizeObserver) { ro = new ResizeObserver(function () { requestAnimationFrame(layout); }); ro.observe(canvas); }
    var sb0 = savebar();
    if (sb0 && window.MutationObserver) { mo = new MutationObserver(function () { shift(); }); mo.observe(sb0, { attributes: true, attributeFilter: ['hidden'] }); }
    document.documentElement.classList.add('cpe-open');

    // ---------- start ----------
    caption(); layout(); drawDock();
    makeMount();
    requestAnimationFrame(layout);
    later(layout, 300);

    return {
      destroy: function () {
        alive = false;
        timers.forEach(clearTimeout);
        if (raf) cancelAnimationFrame(raf);
        listeners.forEach(function (l) { l[0].removeEventListener(l[1], l[2], l[3]); });
        listeners = [];
        if (ro) ro.disconnect();
        if (mo) mo.disconnect();
        unbind();
        document.documentElement.classList.remove('cpe-open');
        if (V.mount && window.Chronicle && Chronicle.destroyWidget) Chronicle.destroyWidget(V.mount);
        borders.destroy();
      }
    };
  }

  Chronicle.register('hero-looks', {
    init: function (el) { el._heroLooks = create(el); },
    destroy: function (el) {
      if (el._heroLooks) el._heroLooks.destroy();
      el._heroLooks = null;
    }
  });
})();
