/**
 * look_kit_frame.js — Customize › Look's example for style kits: one of the
 * owner's own pages, in a frame, wearing the kit picked in the draft.
 *
 * The hand-built example site cannot wear a real kit (style_kits.css keys it
 * on the document root and page-wide parts), so while the Pages part is
 * open customize_look.js asks this file for a frame instead. The frame is a
 * same-origin page with the kit put on its document by kit_try.js's dress(),
 * from the same tokens a saved kit is drawn with; nothing is stored and the
 * tab's own try-out is never touched. A press swaps the kit in the frame at
 * once, without reloading it.
 *
 * The frame is a picture: scaled to the example column as the example site
 * is, showing the top of the page, inert and hidden from screen readers but
 * for a text alternative. It is made the first time the Pages part opens
 * and kept after that. Above it a label in the example's label style ("Page
 * · in Ornate · not saved"); under it "Another page", which steps through
 * the owner's recently edited pages and the campaign home, and "Walk your
 * pages in this kit", which opens the page with the brush's try-out strip on
 * in that kit. The swap with the example site is a fade (customize.css);
 * Off makes it instant.
 */
(function () {
  'use strict';
  window.Chronicle = window.Chronicle || {};
  var C = window.Chronicle;
  if (C.lookKitFrame) return;

  // The page is drawn at this size and scaled down, the example site's
  // proportions (880 x 520) at a desktop width.
  var W = 1040, H = 614;

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text) e.textContent = text;
    return e;
  }

  // lookKitFrame(opts) makes the controller; opts: { pvw, preview, cid }.
  // set({ on, kit, saved, theme, unsaved }) is called on every change of the
  // draft; the frame is fetched and made on the first set with on.
  C.lookKitFrame = function (opts) {
    var url = '/campaigns/' + encodeURIComponent(opts.cid) + '/customize/kit-try';
    var data = null, failed = false, loading = false;
    var at = 0, frame = null, ready = false, want = null;
    var label = el('p', 'kf-l');
    label.setAttribute('aria-live', 'polite');
    label.hidden = true;
    var pane = el('div', 'kf');
    pane.setAttribute('role', 'img');
    pane.inert = true;
    var foot = el('div', 'kf-foot');
    var next = el('button', 'kf-q', 'Another page');
    next.type = 'button';
    var walk = el('button', 'kf-q', 'Walk your pages in this kit');
    walk.type = 'button';
    foot.appendChild(next);
    foot.appendChild(walk);
    foot.hidden = true;
    var note = el('p', 'kf-note', 'Colours and fonts you have not saved yet show in the example site, not here.');
    note.hidden = true;
    opts.preview.parentNode.insertBefore(label, opts.preview);
    opts.preview.appendChild(pane);
    opts.preview.parentNode.insertBefore(note, opts.preview.nextSibling);
    opts.preview.parentNode.insertBefore(foot, opts.preview.nextSibling);

    function kitLabel(id) {
      var ks = (data && data.kits) || [];
      for (var i = 0; i < ks.length; i++) if (ks[i].id === id) return ks[i].label;
      return id;
    }
    function page() { return data.pages[at]; }

    // The page's top fills the column's width; at phone width, where the
    // example is a short strip, the whole top fits its height instead.
    function fit() {
      if (!frame) return;
      var w = pane.clientWidth, h = pane.clientHeight;
      if (!w || !h) return;
      var k = Math.min(w / W, h / H);
      frame.style.transform = 'translateX(' + Math.max(0, Math.round((w - W * k) / 2)) + 'px) scale(' + k.toFixed(4) + ')';
    }

    function dress(f) {
      var doc;
      try { doc = f.contentDocument; } catch (e) { doc = null; }
      if (!doc || !doc.documentElement || !C.kitTry) return false;
      C.kitTry.dress(doc, { kit: want.kit, css: data.css, entrance: data.entrance });
      var dark = want.theme === 'dark';
      doc.documentElement.classList.toggle('dark', dark);
      doc.documentElement.style.backgroundColor = dark ? '#111827' : '#f9fafb';
      return true;
    }

    // load shows page at, in a fresh frame laid over the one on show, which
    // stays until the new one is dressed, so stepping never shows a blank.
    function load() {
      var f = document.createElement('iframe');
      f.setAttribute('tabindex', '-1');
      f.setAttribute('aria-hidden', 'true');
      f.setAttribute('title', '');
      f.className = 'kf-f';
      f.style.visibility = 'hidden';
      f.addEventListener('load', function () {
        if (!dress(f)) { failed = true; paint(); return; }
        if (frame && frame !== f) frame.remove();
        frame = f;
        f.style.visibility = '';
        ready = true;
        fit();
        paint();
      });
      f.src = page().href;
      pane.appendChild(f);
    }

    function paint() {
      var on = !!(want && want.on && data && ready && !failed);
      opts.preview.classList.toggle('kf-on', on);
      label.hidden = foot.hidden = !on;
      note.hidden = !(on && want.unsaved);
      if (!on) return;
      var name = page().name, k = kitLabel(want.kit);
      label.textContent = name + ' · in ' + k + (want.kit === want.saved ? ' · as saved' : ' · not saved');
      pane.setAttribute('aria-label', 'A picture of your page ' + name + ' in the ' + k + ' kit');
      next.hidden = data.pages.length < 2;
      if (frame) dress(frame);
    }

    function start() {
      loading = true;
      C.apiFetch(url + '?pages=1').then(function (r) {
        if (!r.ok) throw new Error('status ' + r.status);
        return r.json();
      }).then(function (j) {
        if (!j || !j.pages || !j.pages.length) throw new Error('no pages');
        data = j;
        load();
      }).catch(function () { failed = true; paint(); });
    }

    next.addEventListener('click', function () {
      if (!data) return;
      at = (at + 1) % data.pages.length;
      load();
    });
    walk.addEventListener('click', function () {
      if (!data || !want) return;
      var go = function () {
        C.kitTry.walk({ url: url, campaign: opts.cid, kit: want.kit, href: page().href, kits: data.kits, css: data.css, entrance: data.entrance });
      };
      if (C.customizeFlush) C.customizeFlush().then(go, go);
      else go();
    });
    if (window.ResizeObserver) new ResizeObserver(fit).observe(pane);

    return {
      set: function (o) {
        want = o;
        if (o.on && !data && !loading && !failed) start();
        paint();
      },
      active: function () { return opts.preview.classList.contains('kf-on'); }
    };
  };
})();
