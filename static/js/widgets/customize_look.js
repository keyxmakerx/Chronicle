/**
 * customize_look.js -- the Customize page's look editor (data-widget="customize-look").
 *
 * The owner edits a draft; the example site beside the controls shows it,
 * and one Save sends the whole draft to PUT /campaigns/:id/appearance.
 * Pictures upload to POST /campaigns/:id/appearance/picture and are only
 * used once a draft holding them is saved. The colour rules (readable(),
 * fillFor(), tame()) match internal/colour, which the server applies again.
 *
 * Everything lives inside mount() so a boosted navigation away can tear the
 * editor down cleanly: listeners on document and window go through on() and
 * are removed by the returned destroy function.
 */
(function () {
  'use strict';

  var SPRITE = "<svg width=\"0\" height=\"0\" style=\"position:absolute\" aria-hidden=\"true\" focusable=\"false\"><defs><symbol id=\"cz-i-home\" viewBox=\"0 0 24 24\"><path d=\"M3.5 11.5 12 4.5l8.5 7\"/><path d=\"M5.5 10v9.5h13V10\"/><path d=\"M10 19.5v-5h4v5\"/></symbol><symbol id=\"cz-i-journal\" viewBox=\"0 0 24 24\"><path d=\"M12 6.5C10.3 5 7.4 4.5 4 5v13.5c3.4-.5 6.3 0 8 1.5 1.7-1.5 4.6-2 8-1.5V5c-3.4-.5-6.3 0-8 1.5z\"/><path d=\"M12 6.5V20\"/></symbol><symbol id=\"cz-i-cal\" viewBox=\"0 0 24 24\"><rect x=\"4\" y=\"5.5\" width=\"16\" height=\"14.5\" rx=\"2\"/><path d=\"M4 10h16M8.5 3.5v4M15.5 3.5v4M8 14h2M14 14h2M8 17h2\"/></symbol><symbol id=\"cz-i-d20\" viewBox=\"0 0 24 24\"><path d=\"M12 3 20 7.5v9L12 21l-8-4.5v-9z\"/><path d=\"M12 3 7.5 13.5h9zM4 7.5l3.5 6M20 7.5l-3.5 6M7.5 13.5 12 21l4.5-7.5\"/></symbol><symbol id=\"cz-i-map\" viewBox=\"0 0 24 24\"><path d=\"M9 4.5 3.5 6.5v13L9 17.5l6 2 5.5-2v-13L15 6.5z\"/><path d=\"M9 4.5v13M15 6.5v13\"/></symbol><symbol id=\"cz-i-users\" viewBox=\"0 0 24 24\"><circle cx=\"9\" cy=\"8.5\" r=\"3.2\"/><path d=\"M3.5 19c.6-3.2 2.8-5 5.5-5s4.9 1.8 5.5 5\"/><circle cx=\"16.5\" cy=\"9.5\" r=\"2.6\"/><path d=\"M16 14.2c2.4.1 4 1.7 4.5 4.3\"/></symbol><symbol id=\"cz-i-layers\" viewBox=\"0 0 24 24\"><path d=\"M12 4 20.5 8.5 12 13 3.5 8.5z\"/><path d=\"m3.5 12.5 8.5 4.5 8.5-4.5M3.5 16.5 12 21l8.5-4.5\"/></symbol><symbol id=\"cz-i-mappin\" viewBox=\"0 0 24 24\"><path d=\"M12 21s-6.5-5.6-6.5-11a6.5 6.5 0 0 1 13 0c0 5.4-6.5 11-6.5 11z\"/><circle cx=\"12\" cy=\"10\" r=\"2.3\"/></symbol><symbol id=\"cz-i-banner\" viewBox=\"0 0 24 24\"><path d=\"M6.5 3.5v17M6.5 4.5h11v10.5l-5.5-3-5.5 3\"/></symbol><symbol id=\"cz-i-gem\" viewBox=\"0 0 24 24\"><path d=\"M7 4.5h10l3.5 5-8.5 10-8.5-10z\"/><path d=\"M3.5 9.5h17M9.5 4.5 8 9.5l4 10 4-10-1.5-5\"/></symbol><symbol id=\"cz-i-scroll\" viewBox=\"0 0 24 24\"><path d=\"M8 4h9.5a2 2 0 0 1 0 4H17v10a2 2 0 0 1-2 2H6.5a2 2 0 0 1 0-4H8z\"/><path d=\"M8 16V6a2 2 0 0 0-4 0v1M11 9h3M11 12.5h3\"/></symbol><symbol id=\"cz-i-tack\" viewBox=\"0 0 24 24\"><path d=\"M9.5 3.5h5l-.8 5.3 3.3 3.2H7l3.3-3.2z\"/><path d=\"M12 12v8.5\"/></symbol><symbol id=\"cz-i-pencil\" viewBox=\"0 0 24 24\"><path d=\"M16.9 4.5a2.1 2.1 0 1 1 3 3L8 19.3l-4 1 1-4z\"/><path d=\"M14.5 7l2.5 2.5\"/></symbol><symbol id=\"cz-i-chev\" viewBox=\"0 0 24 24\"><path d=\"m6 9 6 6 6-6\"/></symbol><symbol id=\"cz-i-chev-r\" viewBox=\"0 0 24 24\"><path d=\"m9 6 6 6-6 6\"/></symbol><symbol id=\"cz-i-search\" viewBox=\"0 0 24 24\"><circle cx=\"11\" cy=\"11\" r=\"6.5\"/><path d=\"m20 20-4.2-4.2\"/></symbol><symbol id=\"cz-i-mask\" viewBox=\"0 0 24 24\"><path d=\"M4 5h8v7c0 3-2 6-4 6s-4-3-4-6zM12 5h8v7c0 3-2 6-4 6s-4-3-4-6z\"/></symbol><symbol id=\"cz-i-bell\" viewBox=\"0 0 24 24\"><path d=\"M6 16V11a6 6 0 0 1 12 0v5l1.5 2h-15z\"/><path d=\"M10 21h4\"/></symbol><symbol id=\"cz-i-note\" viewBox=\"0 0 24 24\"><path d=\"M5 4h14v11l-5 5H5z\"/><path d=\"M14 20v-5h5\"/></symbol><symbol id=\"cz-i-flame\" viewBox=\"0 0 24 24\"><path d=\"M12 21c-3.6 0-6-2.4-6-5.6 0-3.6 3.2-5.2 3.7-9.4 2.2 1.3 3.4 3.4 3.3 5.6 1-.5 1.7-1.5 2-2.8 1.7 1.5 3 3.6 3 6.4 0 3.4-2.4 5.8-6 5.8z\"/></symbol><symbol id=\"cz-i-check\" viewBox=\"0 0 24 24\"><path d=\"M5 12.5 10 17.5 19.5 7\"/></symbol><symbol id=\"cz-i-plus\" viewBox=\"0 0 24 24\"><path d=\"M12 5.5v13M5.5 12h13\"/></symbol><symbol id=\"cz-i-x\" viewBox=\"0 0 24 24\"><path d=\"M6.5 6.5l11 11M17.5 6.5l-11 11\"/></symbol><symbol id=\"cz-i-up\" viewBox=\"0 0 24 24\"><path d=\"M12 19V5M6 11l6-6 6 6\"/></symbol><symbol id=\"cz-i-down\" viewBox=\"0 0 24 24\"><path d=\"M12 5v14M6 13l6 6 6-6\"/></symbol><symbol id=\"cz-i-gear\" viewBox=\"0 0 24 24\"><circle cx=\"12\" cy=\"12\" r=\"3\"/><path d=\"M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.1 2.1M16.6 16.6l2.1 2.1M5.3 18.7l2.1-2.1M16.6 7.4l2.1-2.1\"/></symbol><symbol id=\"cz-i-undo\" viewBox=\"0 0 24 24\"><path d=\"M9 6.5 4.5 11 9 15.5\"/><path d=\"M4.5 11H14a5 5 0 0 1 0 10h-2.5\"/></symbol><symbol id=\"cz-i-info\" viewBox=\"0 0 24 24\"><circle cx=\"12\" cy=\"12\" r=\"8.5\"/><path d=\"M12 11v5M12 8v.01\"/></symbol><symbol id=\"cz-i-upload\" viewBox=\"0 0 24 24\"><path d=\"M12 15.5V4.5M7.5 9 12 4.5 16.5 9\"/><path d=\"M4.5 15v3.5A1.5 1.5 0 0 0 6 20h12a1.5 1.5 0 0 0 1.5-1.5V15\"/></symbol><symbol id=\"cz-i-trash\" viewBox=\"0 0 24 24\"><path d=\"M4.5 7h15M9.5 7V4.5h5V7M6.5 7l1 13h9l1-13\"/></symbol><symbol id=\"cz-i-image\" viewBox=\"0 0 24 24\"><rect x=\"3.5\" y=\"5\" width=\"17\" height=\"14\" rx=\"2\"/><circle cx=\"9\" cy=\"10\" r=\"1.6\"/><path d=\"m5 17 4.5-4.5 3 3 2.5-2.5 4 4\"/></symbol><symbol id=\"cz-i-link\" viewBox=\"0 0 24 24\"><path d=\"M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1\"/><path d=\"M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1\"/></symbol><symbol id=\"cz-i-spark\" viewBox=\"0 0 24 24\"><path d=\"M11 3.5l1.9 5.1 5.1 1.9-5.1 1.9L11 17.5l-1.9-5.1L4 10.5l5.1-1.9z\"/><path d=\"M18.5 15.5v5M16 18h5\"/></symbol><symbol id=\"cz-i-tag\" viewBox=\"0 0 24 24\"><path d=\"M3.5 12V4h8l9 9-8 8z\"/><circle cx=\"7.8\" cy=\"8.2\" r=\"1.3\"/></symbol><symbol id=\"cz-i-topbar\" viewBox=\"0 0 24 24\"><rect x=\"3.5\" y=\"4.5\" width=\"17\" height=\"15\" rx=\"2\"/><path d=\"M3.5 9.5h17M6.5 7h4\"/></symbol><symbol id=\"cz-i-sidebar\" viewBox=\"0 0 24 24\"><rect x=\"3.5\" y=\"4.5\" width=\"17\" height=\"15\" rx=\"2\"/><path d=\"M5.5 4.5H9.5v15H5.5a2 2 0 0 1-2-2v-11a2 2 0 0 1 2-2z\" fill=\"currentColor\" fill-opacity=\".4\"/></symbol><symbol id=\"cz-i-nav\" viewBox=\"0 0 24 24\"><rect x=\"3.5\" y=\"4.5\" width=\"17\" height=\"15\" rx=\"2\"/><path d=\"M9.5 4.5v15M5.8 8.5h1.6M5.8 12h1.6M5.8 15.5h1.6\"/></symbol><symbol id=\"cz-i-palette\" viewBox=\"0 0 24 24\"><path d=\"M12 3.5a8.5 8.5 0 1 0 0 17c1 0 1.7-.8 1.7-1.7 0-.5-.2-.9-.5-1.2-.3-.3-.5-.7-.5-1.2 0-.9.8-1.7 1.7-1.7h2c2.3 0 4.1-1.8 4.1-4.1 0-4-3.8-7.1-8.5-7.1z\"/><circle cx=\"7.8\" cy=\"11\" r=\"1.1\"/><circle cx=\"10.5\" cy=\"7.6\" r=\"1.1\"/><circle cx=\"15\" cy=\"8\" r=\"1.1\"/></symbol><symbol id=\"cz-i-type\" viewBox=\"0 0 24 24\"><path d=\"M3.5 18.5 8 6h1.5L14 18.5M5.2 14h7.1\"/><path d=\"M20.5 18.5v-5a2.5 2.5 0 0 0-4.7-1.2M20.5 15.2c-3-.4-4.9.3-4.9 1.8 0 1.4 1.8 1.9 3.3 1.1\"/></symbol><symbol id=\"cz-i-cursor\" viewBox=\"0 0 24 24\"><path d=\"M6 3.5 18.5 12l-5.3 1.3 2.9 5.9-2.3 1.1-2.8-5.9L7 17.5z\"/></symbol><symbol id=\"cz-i-depth\" viewBox=\"0 0 24 24\"><rect x=\"8.5\" y=\"8.5\" width=\"12\" height=\"12\" rx=\"2\"/><path d=\"M15.5 5.5H6A2.5 2.5 0 0 0 3.5 8v9.5\"/></symbol><symbol id=\"cz-i-moon\" viewBox=\"0 0 24 24\"><path d=\"M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z\"/></symbol><symbol id=\"cz-i-lock\" viewBox=\"0 0 24 24\"><rect x=\"5\" y=\"10.5\" width=\"14\" height=\"10\" rx=\"2\"/><path d=\"M8 10.5V8a4 4 0 0 1 8 0v2.5\"/></symbol><symbol id=\"cz-i-eyeoff\" viewBox=\"0 0 24 24\"><path d=\"M3.5 3.5l17 17M10.6 10.6a2 2 0 0 0 2.8 2.8M9.9 5.7A9.5 9.5 0 0 1 12 5.5c6 0 9.5 6.5 9.5 6.5a15 15 0 0 1-3.1 3.8M6.4 6.9C4 8.6 2.5 12 2.5 12S6 18.5 12 18.5a9 9 0 0 0 4-1\"/></symbol></defs></svg>";

  // refreshTopbar() redraws the live header's background, centre content
  // and name from a fresh server render of this page. The header sits
  // outside #main-content, so boosted navigation never redraws it and a
  // saved change would otherwise only show after a full reload. Swapping
  // the server's own markup keeps link sanitising and image URLs in one
  // place (Topbar()).
  function swapFrom(doc) {
    ['topbar-bg', 'topbar-content'].forEach(function (id) {
      var fresh = doc.getElementById(id), live = document.getElementById(id);
      if (!fresh || !live) return;
      // Widgets inside (the moving background) are torn down and mounted
      // again, since boot.js only scans on page load and htmx swaps.
      if (Chronicle.destroyWidget) Array.prototype.forEach.call(live.querySelectorAll('[data-widget]'), Chronicle.destroyWidget);
      live.innerHTML = fresh.innerHTML;
      if (Chronicle.mountWidgets) Chronicle.mountWidgets(live);
    });
    // The header's word colour follows its background.
    var freshBar = doc.getElementById('app-topbar'), liveBar = document.getElementById('app-topbar');
    if (freshBar && liveBar) liveBar.classList.toggle('cz-hdr-light', freshBar.classList.contains('cz-hdr-light'));
    var name = doc.querySelector('[data-topbar-name]');
    document.querySelectorAll('[data-topbar-name]').forEach(function (el) { if (name) el.textContent = name.textContent; });
  }
  function fetchPage() {
    return fetch(window.location.href, { credentials: 'same-origin', headers: { 'Accept': 'text/html' } })
      .then(function (res) { return res.ok ? res.text() : Promise.reject(res.status); })
      .then(function (html) { return new DOMParser().parseFromString(html, 'text/html'); });
  }
  function refreshTopbar() {
    return fetchPage().then(swapFrom).catch(function () { /* the next full load shows it; the save itself succeeded */ });
  }
  Chronicle.refreshTopbar = refreshTopbar;

  // refreshSite() brings the whole page in line with a fresh save: the
  // campaign's own style blocks, the look's attributes on <html>, the
  // header, and the menu's logo and name.
  function refreshSite() {
    return fetchPage().then(function (doc) {
      var head = document.head;
      head.querySelectorAll('style[data-campaign-style]').forEach(function (el) { el.remove(); });
      doc.head.querySelectorAll('style[data-campaign-style]').forEach(function (el) { head.appendChild(document.importNode(el, true)); });
      var html = document.documentElement;
      Array.prototype.slice.call(html.attributes).forEach(function (a) { if (a.name.indexOf('data-cz-') === 0) html.removeAttribute(a.name); });
      Array.prototype.slice.call(doc.documentElement.attributes).forEach(function (a) { if (a.name.indexOf('data-cz-') === 0) html.setAttribute(a.name, a.value); });
      // nav-rm is how the menu's script hears the campaign's reduce switch.
      html.classList.toggle('nav-rm', html.hasAttribute('data-cz-reduce'));
      swapFrom(doc);
      var brand = doc.querySelector('.nav-brand-link'), liveBrand = document.querySelector('.nav-brand-link');
      if (brand && liveBrand) liveBrand.innerHTML = brand.innerHTML;
      // The banner picture lives beside the link, not in it.
      var bg = doc.querySelector('.nav-brand-bg'), liveBg = document.querySelector('.nav-brand-bg');
      if (bg && liveBg) liveBg.innerHTML = bg.innerHTML;
    }).catch(function () { /* the next full load shows it; the save itself succeeded */ });
  }

  function mount(ROOT, STATE, CID, CSRF) {
    var alive = true;
    var listeners = [];
    function on(target, type, fn, opts) { target.addEventListener(type, fn, opts); listeners.push([target, type, fn, opts]); }

    /* ---------- Helpers ---------- */
    var $ = function(s, r){ return (r || ROOT).querySelector(s); };
    var $$ = function(s, r){ return Array.prototype.slice.call((r || ROOT).querySelectorAll(s)); };
    function esc(s){ return String(s == null ? '' : s).replace(/[&<>"']/g, function(c){ return { '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;' }[c]; }); }
    function IC(id, cls){ return '<svg class="i' + (cls ? ' ' + cls : '') + '" aria-hidden="true"><use href="#cz-' + id + '"/></svg>'; }
    function clone(o){ return JSON.parse(JSON.stringify(o)); }
    function same(a, b){ return JSON.stringify(a) === JSON.stringify(b); }
    function getP(o, path){ return path.split('.').reduce(function(x, k){ return x == null ? x : x[k]; }, o); }
    function setP(o, path, v){ var ks = path.split('.'), last = ks.pop(); ks.reduce(function(x, k){ return x[k]; }, o)[last] = v; }
    function clamp(v, a, b){ return Math.max(a, Math.min(b, v)); }
    var root = document.documentElement;
    // Less motion asked for by the device. The campaign's saved switch also
    // stills the page, but in here the draft's switch decides.
    var mqRM = window.matchMedia ? matchMedia('(prefers-reduced-motion: reduce)') : null;
    function reducedDevice(){ return !!(mqRM && mqRM.matches); }

    /* ---------- Colour ----------
       OKLab / OKLCH for mixing and taming, WCAG contrast for readability. */
    function hexToRgb(h){
      h = String(h).replace('#', '');
      if (h.length === 3) h = h.split('').map(function(c){ return c + c; }).join('');
      return [0, 2, 4].map(function(i){ return parseInt(h.slice(i, i + 2), 16) / 255; });
    }
    function rgbToHex(c){
      return '#' + c.map(function(v){ var n = Math.round(clamp(v, 0, 1) * 255); return (n < 16 ? '0' : '') + n.toString(16); }).join('');
    }
    function lin(c){ return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4); }
    function delin(c){ return c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055; }
    function luminance(hex){ var c = hexToRgb(hex).map(lin); return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]; }
    function contrast(a, b){ var A = luminance(a), B = luminance(b); return (Math.max(A, B) + 0.05) / (Math.min(A, B) + 0.05); }
    function toLab(rgb){
      var r = lin(rgb[0]), g = lin(rgb[1]), b = lin(rgb[2]);
      var l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
      var m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
      var s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
      return [0.2104542553 * l + 0.7936177850 * m - 0.0040720468 * s, 1.9779984951 * l - 2.4285922050 * m + 0.4505937099 * s, 0.0259040371 * l + 0.7827717662 * m - 0.8086757660 * s];
    }
    function fromLab(lab){
      var l = Math.pow(lab[0] + 0.3963377774 * lab[1] + 0.2158037573 * lab[2], 3);
      var m = Math.pow(lab[0] - 0.1055613458 * lab[1] - 0.0638541728 * lab[2], 3);
      var s = Math.pow(lab[0] - 0.0894841775 * lab[1] - 1.2914855480 * lab[2], 3);
      return [delin(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s), delin(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s), delin(-0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s)];
    }
    function oklch(hex){ var L = toLab(hexToRgb(hex)); return [L[0], Math.hypot(L[1], L[2]), (Math.atan2(L[2], L[1]) * 180 / Math.PI + 360) % 360]; }
    function lchRgb(L, C, h){ var r = h * Math.PI / 180; return fromLab([L, C * Math.cos(r), C * Math.sin(r)]); }
    function inGamut(c){ return c.every(function(v){ return v >= -0.0005 && v <= 1.0005; }); }
    // Keeps lightness and hue, gives up chroma until the colour exists in sRGB.
    function lchHex(L, C, h){
      var c = lchRgb(L, C, h);
      if (inGamut(c)) return rgbToHex(c);
      var lo = 0, hi = C;
      for (var i = 0; i < 24; i++){ var mid = (lo + hi) / 2; if (inGamut(lchRgb(L, mid, h))) lo = mid; else hi = mid; }
      return rgbToHex(lchRgb(L, lo, h));
    }
    function deltaE(a, b){ var A = toLab(hexToRgb(a)), B = toLab(hexToRgb(b)); return Math.hypot(A[0] - B[0], A[1] - B[1], A[2] - B[2]); }
    function mix(a, b, t){ var A = toLab(hexToRgb(a)), B = toLab(hexToRgb(b)); return rgbToHex(fromLab([A[0] + (B[0] - A[0]) * t, A[1] + (B[1] - A[1]) * t, A[2] + (B[2] - A[2]) * t])); }
    function rgbCh(hex){ return hexToRgb(hex).map(function(v){ return Math.round(v * 255); }).join(' '); }
    var INK = '#111827';
    // Moves a colour's lightness (never its hue) until text built on it reaches
    // the target contrast against every background given.
    function readable(hex, bgs, target, dir){
      function worst(h){ return Math.min.apply(null, bgs.map(function(b){ return contrast(h, b); })); }
      if (worst(hex) >= target) return { hex:hex, de:0 };
      var c = oklch(hex), L = c[0], step = dir === 'darker' ? -0.004 : 0.004, out = hex;
      for (var i = 0; i < 240; i++){
        L += step; if (L <= 0.02 || L >= 0.995) break;
        out = lchHex(L, c[1], c[2]);
        if (worst(out) >= target) break;
      }
      return { hex:out, de:deltaE(hex, out) };
    }
    // A colour used as a fill with words on it: white words when a small
    // deepening allows it, otherwise dark words on the colour as chosen.
    function fillFor(hex){
      if (contrast(hex, '#ffffff') >= 4.5) return { fill:hex, on:'#ffffff', de:0, flip:false };
      var dk = readable(hex, ['#ffffff'], 4.5, 'darker');
      if (dk.de <= 0.06) return { fill:dk.hex, on:'#ffffff', de:dk.de, flip:false };
      if (contrast(hex, INK) >= 4.5) return { fill:hex, on:INK, de:0, flip:true };
      var lt = readable(hex, [INK], 4.5, 'lighter');
      return lt.de < dk.de ? { fill:lt.hex, on:INK, de:lt.de, flip:true } : { fill:dk.hex, on:'#ffffff', de:dk.de, flip:false };
    }
    // No neon: lightness stays between 0.30 and 0.80, and bright colours lose
    // chroma first. Every preset already sits inside these limits.
    // Header backgrounds may go deeper than colours that carry words.
    function chromaCap(L){ return 0.25 - Math.max(0, L - 0.72) * 1.625; }
    function floorL(deep){ return deep ? 0.14 : 0.30; }
    function tame(hex, deep){
      var c = oklch(hex), L = clamp(c[0], floorL(deep), 0.80), C = Math.min(c[1], chromaCap(L));
      var out = (L === c[0] && C === c[1]) ? hex.toLowerCase() : lchHex(L, C, c[2]);
      return { hex:out, toned:deltaE(hex, out) > 0.02 };
    }
    function toneSpan(deep){ return 0.80 - (deep ? 0.16 : 0.36); }
    function fromPicker(h, t, deep){ var L = 0.80 - t / 100 * toneSpan(deep); return lchHex(L, Math.min(deep ? 0.14 : 0.19, chromaCap(L)), h); }
    function toPicker(hex, deep){ var c = oklch(hex); return { h:Math.round(c[2]) % 360, t:Math.round(clamp((0.80 - c[0]) / toneSpan(deep) * 100, 0, 100)) }; }

    // The menu keeps white words at 7:1 on any colour it is given, by moving
    // the colour toward black. Mirrors colour.MenuDark and colour.MenuTinted.
    function menuDark(hex){
      var out = String(hex).toLowerCase();
      for (var i = 0; i < 24 && contrast(out, '#ffffff') < 7; i++) out = mix(out, '#000000', 0.12);
      return out;
    }
    function menuTinted(accent){ return menuDark(mix(accent, '#0b0b12', 0.72)); }

    /* ---------- Data ---------- */
    var PRESETS = [['Indigo', '#6366f1'], ['Blue', '#3b82f6'], ['Cyan', '#06b6d4'], ['Emerald', '#10b981'], ['Amber', '#f59e0b'], ['Rose', '#f43f5e'], ['Purple', '#a855f7'], ['Orange', '#f97316']];
    var HDR_COLOURS = [['Night', '#0f172a'], ['Deep blue', '#1e2a5a'], ['Slate', '#1f2937'], ['Umber', '#3b2a1c'], ['Pine', '#14352a'], ['Moss', '#2f4a2c'], ['Oxblood', '#4a1512'], ['Plum', '#3b1d5e']];
    var LOOK_COLOUR_NAMES = { '#9a4a26':'Sienna', '#8f2d2d':'Wax red', '#8a6a1f':'Ochre', '#5b6be0':'Moonlit blue', '#2563eb':'Blue', '#64748b':'Slate', '#2f7d4f':'Moss green', '#a16207':'Lantern', '#4d7c0f':'Fern', '#c2410c':'Ember', '#b91c1c':'Garnet', '#b45309':'Brass', '#0e7490':'Glacier', '#7c3aed':'Violet', '#a21caf':'Orchid', '#94651b':'Candle gold', '#0f766e':'Console teal', '#0369a1':'Signal blue' };
    function colourName(hex){
      if (!hex) return '';
      var h = hex.toLowerCase(), p = PRESETS.concat(HDR_COLOURS).filter(function(x){ return x[1] === h; })[0];
      return p ? p[0] : (LOOK_COLOUR_NAMES[h] || 'Your colour');
    }

    // Page tones, light and dark. Cool is Chronicle's own grey scale.
    var TONES = {
      cool:  { light:{ bg:'#f9fafb', card:'#ffffff', alt:'#f3f4f6', line:'#e5e7eb', line2:'#d1d5db', text:'#111827', body:'#374151', muted:'#6b7280' },
               dark: { bg:'#111827', card:'#1f2937', alt:'#273244', line:'#374151', line2:'#4b5563', text:'#f9fafb', body:'#d1d5db', muted:'#9ca3af' } },
      warm:  { light:{ bg:'#faf8f5', card:'#ffffff', alt:'#f4f0ea', line:'#e8e1d8', line2:'#d6cdc1', text:'#1c1917', body:'#44403c', muted:'#78716c' },
               dark: { bg:'#171311', card:'#231e1b', alt:'#2e2824', line:'#3d3530', line2:'#534943', text:'#faf7f2', body:'#d6d0c8', muted:'#a8a097' } },
      paper: { light:{ bg:'#f4eddd', card:'#fbf6ea', alt:'#ece2cc', line:'#dccfb3', line2:'#c9b995', text:'#2a2118', body:'#43372a', muted:'#75654f' },
               dark: { bg:'#1a1610', card:'#25201a', alt:'#2f2920', line:'#43392b', line2:'#5a4d3a', text:'#f5ecd9', body:'#dccfb8', muted:'#a89a80' } }
    };
    var HIGH = { light:{ body:'text', muted:'body', line2:1 }, dark:{ body:'text', muted:'body', line2:1 } };
    var SIDEBARS = {
      charcoal:{ bg:'#1a1c23', text:'#cbd5e1', text2:'#8b93a1', text3:'#6b7385' },
      ink:     { bg:'#0e1424', text:'#cdd5e3', text2:'#8a95aa', text3:'#66728a' }
    };

    var BODY_FONTS = [
      ['inter', 'Inter', "'Inter', system-ui, -apple-system, sans-serif", 'Clean sans'],
      ['sourcesans', 'Source Sans 3', "'Source Sans 3', 'Segoe UI', sans-serif", 'Friendly sans'],
      ['atkinson', 'Atkinson Hyperlegible', "'Atkinson Hyperlegible', Verdana, sans-serif", 'Easiest to read'],
      ['literata', 'Literata', "'Literata', Georgia, serif", 'Book serif'],
      ['sourceserif', 'Source Serif 4', "'Source Serif 4', Georgia, serif", 'Classic serif'],
      ['lora', 'Lora', "'Lora', Georgia, serif", 'Calligraphic serif'],
      ['alegreya', 'Alegreya', "'Alegreya', Georgia, serif", 'Storybook serif'],
      ['merriweather', 'Merriweather', "'Merriweather', Georgia, serif", 'Sturdy serif']
    ];
    // Heading faces: weight 400 where the face has no bolder cut, a size nudge
    // where a face runs small or large, and letter-spacing for capitals.
    var HEAD_FONTS = [
      ['same', 'Same as text', null, 'Matches the text', 650, 1, '-.01em'],
      ['cinzel', 'Cinzel', "'Cinzel', 'Trajan Pro', Georgia, serif", 'Carved capitals', 600, .9, '.02em'],
      ['marcellus', 'Marcellus', "'Marcellus', Georgia, serif", 'Flared Roman', 400, 1, '.01em'],
      ['imfell', 'IM Fell English', "'IM Fell English', Georgia, serif", 'Old print', 400, 1.06, '0'],
      ['cormorant', 'Cormorant Garamond', "'Cormorant Garamond', Garamond, Georgia, serif", 'Elegant', 600, 1.14, '0'],
      ['fraunces', 'Fraunces', "'Fraunces', Georgia, serif", 'Soft and warm', 600, 1, '-.01em'],
      ['playfair', 'Playfair Display', "'Playfair Display', Georgia, serif", 'High contrast', 600, 1, '0'],
      ['josefin', 'Josefin Sans', "'Josefin Sans', 'Century Gothic', sans-serif", 'Art deco', 600, 1.02, '.01em'],
      ['chakra', 'Chakra Petch', "'Chakra Petch', 'Segoe UI', sans-serif", 'Starship console', 600, 1, '0']
    ];
    function bodyFont(id){ return BODY_FONTS.filter(function(f){ return f[0] === id; })[0] || BODY_FONTS[0]; }
    function headFont(id){ return HEAD_FONTS.filter(function(f){ return f[0] === id; })[0] || HEAD_FONTS[0]; }
    var SCALES = { compact:[13, 'Compact'], standard:[14, 'Standard'], roomy:[15.5, 'Roomy'] };

    var NAV_STYLES = [
      ['ring', 'Living ring', 'moving', 'Two traces of light circle the page you are on, changing length and pace on long, slow cycles.'],
      ['comet', 'Comet', 'moving', 'One bright point with a fading tail rides the border, a lap every twelve seconds.'],
      ['breathe', 'Breathing', 'moving', 'A soft wash from the right deepens and eases every few seconds. The quietest moving style.'],
      ['tide', 'Tide', 'moving', 'A band of light passes over the row now and then, like a lamp swinging past.'],
      ['rail', 'Rail and tint', 'still', 'A lit bar on the left and a tint. The clearest at a glance.'],
      ['tab', 'Folder tab', 'still', 'The row takes the page colour and joins the page, like a folder tab.'],
      ['edge', 'Edge-lit', 'still', 'Two hairlines, bright at the left and fading to the right.'],
      ['icon', 'Solid icon', 'still', 'The icon fills in and the words turn white. Nothing else changes.']
    ];
    var STILL_OF = { ring:'rail', comet:'edge', breathe:'rail', tide:'rail' };
    function navName(id){ return NAV_STYLES.filter(function(s){ return s[0] === id; })[0][1]; }
    var BTN_STYLES = [
      ['lift', 'Lift', 'Rises to meet the pointer and settles when pressed. Rounded corners.'],
      ['press', 'Press', 'A tactile key: it sinks onto its base when pressed.'],
      ['glow', 'Glow', 'A pill that gathers a soft light around itself on hover.'],
      ['ink', 'Ink', 'Crisp square corners. Ink spreads from wherever you press.']
    ];
    var BTN_RADIUS = { lift:'8px', press:'6px', glow:'999px', ink:'3px' };
    var ELEVATION = {
      flat:     { name:'Flat', lift:0, rest:{ light:'none', dark:'none' }, hover:{ light:'0 0 0 1.5px rgb(ACC / .35)', dark:'0 0 0 1.5px rgb(ACC / .45)' } },
      standard: { name:'Standard', lift:2, rest:{ light:'0 1px 2px 0 rgb(0 0 0 / .05)', dark:'0 1px 2px 0 rgb(0 0 0 / .3)' }, hover:{ light:'0 6px 16px -4px rgb(0 0 0 / .12), 0 2px 6px -2px rgb(0 0 0 / .08)', dark:'0 8px 20px -6px rgb(0 0 0 / .5), 0 2px 6px -2px rgb(0 0 0 / .35)' } },
      dramatic: { name:'Dramatic', lift:4, rest:{ light:'0 2px 6px -1px rgb(0 0 0 / .10), 0 10px 22px -12px rgb(0 0 0 / .28)', dark:'0 2px 6px -1px rgb(0 0 0 / .4), 0 12px 26px -12px rgb(0 0 0 / .7)' }, hover:{ light:'0 22px 44px -14px rgb(0 0 0 / .38), 0 6px 14px -6px rgb(0 0 0 / .18)', dark:'0 24px 48px -14px rgb(0 0 0 / .85), 0 6px 14px -6px rgb(0 0 0 / .5)' } }
    };
    var SPEEDS = {
      snappy:    { name:'Snappy', d:110, d2:170, ease:'cubic-bezier(.3,.9,.3,1)' },
      standard:  { name:'Standard', d:180, d2:260, ease:'cubic-bezier(.16,1,.3,1)' },
      leisurely: { name:'Leisurely', d:300, d2:440, ease:'cubic-bezier(.22,.8,.3,1)' }
    };
    var HDR_MODES = [['solid', 'Solid'], ['gradient', 'Gradient'], ['moving', 'Moving colour'], ['image', 'Image'], ['sky', 'Sky']];
    var HDR_HEIGHTS = [['slim', 'Slim'], ['tall', 'Tall']];
    // Corners of the menu and the choices that go with them.
    var CORNERS = [['plain', 'Logo and name', 'As today'], ['subtitle', 'Logo, name and a subtitle', 'A short line under the name'], ['banner', 'Banner picture', 'A picture behind the name']];
    var MENU_COLOURS = [['charcoal', 'Charcoal', 'Chronicle as it is today'], ['ink', 'Ink', 'A deep blue-black'], ['tinted', 'Tinted', 'A dark shade of your chrome accent'], ['own', 'Your own colour', 'Kept dark enough to read']];
    var GLOWS = [['accent', 'Follow the accent colour', 'Changes with the look'], ['own', 'Its own colour', 'Pick any colour']];
    var HDR_DIRS = { r:['to right', 'Left to right'], br:['to bottom right', 'Diagonal'], b:['to bottom', 'Top to bottom'] };
    var SCRIMS = { light:[.32, 'Light'], medium:[.5, 'Medium'], strong:[.68, 'Strong'] };

    // A look sets every style setting at once: header background, menu
    // highlight, colours, type, buttons, depth and motion. It never touches the
    // brand, the header's widgets or the reduce-motion switch.
    var LOOKS = [
      { id:'classic', name:'Classic', blurb:'Chronicle as it ships',
        header:{ bg:'solid', solid:'page', from:'#0f172a', to:'#1e2a5a', dir:'r' }, nav:{ style:'ring', strength:'calm' },
        colours:{ accent:'#6366f1', s1:null, s2:null, sidebar:'charcoal', page:'cool', contrast:'standard' },
        type:{ body:'inter', heading:'same', scale:'standard' }, buttons:{ style:'lift' }, motion:{ elevation:'standard', speed:'standard' } },
      { id:'parchment', name:'Parchment', blurb:'Sepia ink on old paper',
        header:{ bg:'solid', solid:'#3b2a1c', from:'#3b2a1c', to:'#4a1512', dir:'r' }, nav:{ style:'tab', strength:'calm' },
        colours:{ accent:'#9a4a26', s1:'#8f2d2d', s2:'#8a6a1f', sidebar:'tinted', page:'paper', contrast:'standard' },
        type:{ body:'literata', heading:'imfell', scale:'roomy' }, buttons:{ style:'press' }, motion:{ elevation:'flat', speed:'leisurely' } },
      { id:'midnight', name:'Midnight', blurb:'Moonlight on deep blue',
        header:{ bg:'gradient', solid:'#0f172a', from:'#0f172a', to:'#1e2a5a', dir:'r' }, nav:{ style:'ring', strength:'calm' },
        colours:{ accent:'#5b6be0', s1:'#2563eb', s2:'#64748b', sidebar:'ink', page:'cool', contrast:'standard' },
        type:{ body:'inter', heading:'cormorant', scale:'standard' }, buttons:{ style:'glow' }, motion:{ elevation:'dramatic', speed:'standard' } },
      { id:'forest', name:'Forest', blurb:'Moss, bark and lantern light',
        header:{ bg:'gradient', solid:'#14352a', from:'#14352a', to:'#2f4a2c', dir:'r' }, nav:{ style:'breathe', strength:'calm' },
        colours:{ accent:'#2f7d4f', s1:'#a16207', s2:'#4d7c0f', sidebar:'tinted', page:'warm', contrast:'standard' },
        type:{ body:'sourceserif', heading:'marcellus', scale:'standard' }, buttons:{ style:'lift' }, motion:{ elevation:'standard', speed:'leisurely' } },
      { id:'ember', name:'Ember', blurb:'Forge-warm, glowing coals',
        header:{ bg:'gradient', solid:'#1f2937', from:'#1f2937', to:'#4a1512', dir:'br' }, nav:{ style:'comet', strength:'calm' },
        colours:{ accent:'#c2410c', s1:'#b91c1c', s2:'#b45309', sidebar:'charcoal', page:'warm', contrast:'standard' },
        type:{ body:'merriweather', heading:'cinzel', scale:'standard' }, buttons:{ style:'press' }, motion:{ elevation:'dramatic', speed:'snappy' } },
      { id:'frost', name:'Frost', blurb:'Crisp, cold and clear',
        header:{ bg:'solid', solid:'page', from:'#0f172a', to:'#1e2a5a', dir:'r' }, nav:{ style:'edge', strength:'calm' },
        colours:{ accent:'#0e7490', s1:'#2563eb', s2:'#64748b', sidebar:'ink', page:'cool', contrast:'high' },
        type:{ body:'sourcesans', heading:'josefin', scale:'standard' }, buttons:{ style:'ink' }, motion:{ elevation:'flat', speed:'snappy' } },
      { id:'arcane', name:'Arcane', blurb:'Violet runes, candle gold',
        header:{ bg:'gradient', solid:'#3b1d5e', from:'#0f172a', to:'#3b1d5e', dir:'r' }, nav:{ style:'tide', strength:'calm' },
        colours:{ accent:'#7c3aed', s1:'#a21caf', s2:'#94651b', sidebar:'tinted', page:'cool', contrast:'standard' },
        type:{ body:'lora', heading:'fraunces', scale:'standard' }, buttons:{ style:'glow' }, motion:{ elevation:'dramatic', speed:'leisurely' } },
      { id:'starship', name:'Starship', blurb:'Clean consoles, cool light',
        header:{ bg:'solid', solid:'#0f172a', from:'#0f172a', to:'#1f2937', dir:'r' }, nav:{ style:'rail', strength:'lively' },
        colours:{ accent:'#0f766e', s1:'#b45309', s2:'#0369a1', sidebar:'ink', page:'cool', contrast:'standard' },
        type:{ body:'atkinson', heading:'chakra', scale:'compact' }, buttons:{ style:'ink' }, motion:{ elevation:'standard', speed:'snappy' } }
    ];
    function look(id){ return LOOKS.filter(function(l){ return l.id === id; })[0]; }
    var LOOK_KEYS = ['header.bg', 'header.solid', 'header.from', 'header.to', 'header.dir', 'nav.style', 'nav.strength',
      'colours.accent', 'colours.s1', 'colours.s2', 'colours.sidebar', 'colours.page', 'colours.contrast',
      'type.body', 'type.heading', 'type.scale', 'buttons.style', 'motion.elevation', 'motion.speed'];

    // The header holds up to WIDGET_SLOTS of these, in the owner's order. The
    // "later" ones are shown as coming, never offered.
    var WIDGETS = {
      links:  { name:'Quick links', desc:'' },
      text:   { name:'A line of text', desc:'One line your table sees' },
      note:   { name:'Quick note', desc:'Jot a note without leaving the page' },
      search: { name:'Search box', desc:'A wide search box instead of the icon' }
    };
    var WIDGET_ORDER = ['links', 'text', 'note', 'search'];
    var LATER = ['In-world date', 'Weather', 'Moon', 'Next game night', 'Era'];
    var WIDGET_SLOTS = 4;

    var SECTIONS = [
      { id:'brand', name:'Brand', icon:'i-tag', desc:'Your campaign’s name, logo, welcome message and backdrop. Looks never change these.' },
      { id:'header', name:'Header', icon:'i-topbar', desc:'The bar across the top of every page: its background and the widgets in it.' },
      { id:'sidebar', name:'Sidebar', icon:'i-sidebar', desc:'The menu down the left of every page: its colour, what fills its top-left corner, and the glow that shows when it is hidden.' },
      { id:'nav', name:'Navigation', icon:'i-nav', desc:'How the menu shows the page you are on.' },
      { id:'colours', name:'Colours', icon:'i-palette', desc:'The site’s own colour, the two colours your pages use, and the tones behind them.' },
      { id:'type', name:'Type', icon:'i-type', desc:'The fonts for text and headings, and how large everything reads.' },
      { id:'buttons', name:'Buttons', icon:'i-cursor', desc:'The shape of buttons and how they move when you point at them and press.' },
      { id:'motion', name:'Motion and depth', icon:'i-depth', desc:'How far cards lift, how quickly things move, and a calmer option for everyone.' }
    ];
    var SEC_KEYS = {
      brand:['brand.name', 'brand.logo', 'brand.welcome', 'brand.backdrop'],
      header:['header.bg', 'header.height', 'header.solid', 'header.from', 'header.to', 'header.dir', 'header.image', 'header.scrim', 'header.widgets', 'header.links', 'header.text'],
      sidebar:['colours.sidebar', 'sidebar.own', 'sidebar.corner', 'sidebar.subtitle', 'sidebar.banner', 'sidebar.glow', 'sidebar.glowColour'],
      nav:['nav.style', 'nav.strength', 'nav.pageName'],
      colours:['colours.accent', 'colours.s1', 'colours.s2', 'colours.page', 'colours.contrast'],
      type:['type.body', 'type.heading', 'type.scale'],
      buttons:['buttons.style'],
      motion:['motion.elevation', 'motion.speed', 'motion.reduceAll']
    };

    /* ---------- Sample pictures, painted once so the page needs no files ---------- */
    function rng(seed){ return function(){ seed |= 0; seed = seed + 0x6D2B79F5 | 0; var t = Math.imul(seed ^ seed >>> 15, 1 | seed); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; }; }
    function ridge(x, w, h, base, amp, rough, fill, r){
      x.beginPath(); x.moveTo(0, h);
      var y = base;
      for (var i = 0; i <= w; i += 6){ y = clamp(y + (r() - 0.5) * rough, base - amp, base + amp); x.lineTo(i, y); }
      x.lineTo(w, h); x.closePath(); x.fillStyle = fill; x.fill();
    }
    function paintDusk(){
      var c = document.createElement('canvas'); c.width = 1200; c.height = 420;
      var x = c.getContext('2d'), r = rng(7), i;
      var g = x.createLinearGradient(0, 0, 0, 420);
      g.addColorStop(0, '#1b1230'); g.addColorStop(0.42, '#4a2344'); g.addColorStop(0.72, '#b24a37'); g.addColorStop(1, '#f2a35c');
      x.fillStyle = g; x.fillRect(0, 0, 1200, 420);
      var s = x.createRadialGradient(780, 300, 10, 780, 300, 210);
      s.addColorStop(0, 'rgba(255,214,150,.95)'); s.addColorStop(0.25, 'rgba(255,170,100,.5)'); s.addColorStop(1, 'rgba(255,140,80,0)');
      x.fillStyle = s; x.fillRect(0, 0, 1200, 420);
      ridge(x, 1200, 420, 282, 55, 16, '#6e3446', r); ridge(x, 1200, 420, 318, 40, 14, '#4a2336', r); ridge(x, 1200, 420, 350, 26, 10, '#2c1526', r);
      x.fillStyle = '#170b16';
      for (var bx = 150; bx < 1050;){
        var bw = 18 + r() * 42, bh = 26 + r() * 70, top = 420 - 38 - bh;
        x.fillRect(bx, top, bw, bh + 38);
        if (r() > 0.7){ x.beginPath(); x.moveTo(bx, top); x.lineTo(bx + bw / 2, top - 22 - r() * 26); x.lineTo(bx + bw, top); x.fill(); }
        bx += bw + 2 + r() * 10;
      }
      x.fillRect(0, 382, 1200, 38);
      for (i = 0; i < 70; i++){ x.fillStyle = 'rgba(255,190,120,' + (0.35 + r() * 0.5).toFixed(2) + ')'; x.fillRect(160 + r() * 880, 330 + r() * 50, 2, 3); }
      for (i = 0; i < 90; i++){ x.fillStyle = 'rgba(255,220,190,' + (r() * 0.35).toFixed(2) + ')'; x.beginPath(); x.arc(r() * 1200, r() * 300, r() * 1.6, 0, 7); x.fill(); }
      return c.toDataURL('image/jpeg', 0.86);
    }
    var IMG = { dusk:'' };
    // Pictures are stored names; PICS maps each to a URL the page can show.
    var PICS = STATE.pictures || {};
    function imgURL(v){ return v && v !== 'none' ? (PICS[v] || '') : ''; }

    /* ---------- State ----------
       saved is what everyone sees; draft is what the preview shows. Save copies
       the draft over saved, Discard copies saved back over the draft. */
    function applyLook(d, id){
      var L = look(id);
      LOOK_KEYS.forEach(function(k){ setP(d, k, clone(getP(L, k))); });
      d.look = id;
      return d;
    }
    // Rows the header's quick links editor always offers; empty ones are
    // dropped on save.
    var LINK_ROWS = 4;
    // The page asks for a short welcome; one saved longer before keeps its room.
    function welcomeMax(){ return Math.max(160, (STATE.draft.brand.welcome || '').length); }
    function fromServer(d){
      d = clone(d);
      ['logo', 'backdrop'].forEach(function(k){ if (!d.brand[k]) d.brand[k] = 'none'; });
      if (!d.header.image) d.header.image = 'none';
      if (!d.sidebar.banner) d.sidebar.banner = 'none';
      d.header.links = (d.header.links || []).map(function(l){ return { label:l.label || '', url:l.url || '', icon:l.icon || '' }; });
      while (d.header.links.length < LINK_ROWS) d.header.links.push({ label:'', url:'', icon:'' });
      return d;
    }
    var CAMPAIGN = STATE.campaign || 'Your campaign';
    var saved = fromServer(STATE.draft);
    var BASE = clone(saved);
    var draft = clone(saved);
    var memo = {};          // custom colours picked during this visit, per swatch group
    var undoDraft = null;   // what Discard threw away, for its Undo
    var ui = { section:'brand', zoom:'site', page:'dash', ptheme:null, strip:true, pageAuto:true };

    function matchesLook(d, id){ var L = look(id); return LOOK_KEYS.every(function(k){ return same(getP(d, k), getP(L, k)); }); }
    function currentLook(d){
      if (matchesLook(d, d.look)) return d.look;
      var m = LOOKS.filter(function(l){ return matchesLook(d, l.id); })[0];
      return m ? m.id : null;
    }
    function lookChanges(d){ var L = look(d.look); return LOOK_KEYS.filter(function(k){ return !same(getP(d, k), getP(L, k)); }).length; }
    function changedSections(){ return SECTIONS.filter(function(s){ return SEC_KEYS[s.id].some(function(k){ return !same(getP(draft, k), getP(saved, k)); }); }).map(function(s){ return s.id; }); }
    // Reset puts a section back: style settings to the look you started from,
    // your own words, pictures and widgets to what is saved.
    function resetTarget(sec){
      var t = {};
      SEC_KEYS[sec].forEach(function(k){ t[k] = LOOK_KEYS.indexOf(k) >= 0 ? clone(getP(look(draft.look), k)) : clone(getP(saved, k)); });
      return t;
    }
    function sectionAtTarget(sec){ var t = resetTarget(sec); return Object.keys(t).every(function(k){ return same(getP(draft, k), t[k]); }); }

    /* ---------- From draft to preview tokens ---------- */
    function pageTheme(){ return root.classList.contains('dark') ? 'dark' : 'light'; }
    function previewTheme(){ return ui.ptheme || pageTheme(); }
    // sRGB alpha compositing, as the browser paints rgb(x / a) over a colour.
    function over(fg, bg, a){ var F = hexToRgb(fg), B = hexToRgb(bg); return rgbToHex(F.map(function(v, i){ return v * a + B[i] * (1 - a); })); }
    function tones(d, theme){
      var T = clone(TONES[d.colours.page][theme]);
      if (d.colours.contrast === 'high'){ T.muted = T.body; T.body = T.text; T.line = T.line2; T.line2 = mix(T.line2, T.text, 0.3); }
      return T;
    }
    // The menu: Charcoal, Ink, a dark shade of the accent, or the owner's own
    // colour. Tinted and own colours are darkened until white words keep 7:1,
    // the same rule the real menu applies (colour.MenuDark).
    function sidebarTone(d){
      var mode = d.colours.sidebar, s = clone(mode === 'ink' ? SIDEBARS.ink : SIDEBARS.charcoal);
      if (mode === 'tinted') s.bg = menuTinted(d.colours.accent);
      else if (mode === 'own') s.bg = menuDark(d.sidebar.own);
      if (d.colours.contrast === 'high'){ s.text = mix(s.text, '#ffffff', 0.55); s.text2 = mix(s.text2, '#ffffff', 0.4); s.text3 = mix(s.text3, '#ffffff', 0.35); }
      return s;
    }
    // The header picks light or dark words from its own background.
    function headerLook(d, T){
      var h = d.header, mode = h.bg === 'sky' ? 'solid' : h.bg;
      function col(c){ return c === 'page' ? T.card : c; }
      function words(cols){
        var w = Math.min.apply(null, cols.map(function(c){ return contrast(c, '#ffffff'); }));
        var k = Math.min.apply(null, cols.map(function(c){ return contrast(c, T.text); }));
        return w >= k;
      }
      var bg, light, rep = 'none';
      if (mode === 'solid'){ bg = col(h.solid); light = words([bg]); }
      else if (mode === 'image'){ bg = '#0b0f1a'; light = true; }
      else {
        var a = col(h.from), b = col(h.to);
        bg = 'linear-gradient(' + HDR_DIRS[h.dir][0] + ', ' + a + ', ' + b + ')';
        light = words([a, b]);
        if (mode === 'moving') rep = 'repeating-linear-gradient(' + (h.dir === 'b' ? '180deg' : '90deg') + ', ' + a + ' 0%, ' + b + ' 16.6667%, ' + a + ' 33.3333%)';
      }
      return light
        ? { bg:bg, rep:rep, text:'#f9fafb', text2:'#c9d1dd', chip:'rgb(255 255 255 / .07)', line:'rgb(255 255 255 / .13)' }
        : { bg:bg, rep:rep, text:T.text, text2:T.muted, chip:T.alt, line:T.line };
    }
    function derive(d, theme){
      var T = tones(d, theme), sb = sidebarTone(d), dir = theme === 'dark' ? 'lighter' : 'darker';
      var acc = d.colours.accent, s1 = d.colours.s1 || acc, s2 = d.colours.s2 || acc;
      var f = fillFor(acc), f1 = fillFor(s1), f2 = fillFor(s2);
      var link = readable(acc, [T.bg, T.card], 4.5, dir).hex;
      var s1t = readable(s1, [T.bg, T.card, over(s1, T.bg, 0.14)], 4.5, dir).hex;
      var s2t = readable(s2, [T.card, over(s2, T.card, 0.16)], 4.5, dir).hex;
      var accL = mix(acc, '#ffffff', 0.45);
      var sbAcc = readable(mix(acc, '#ffffff', 0.3), [sb.bg], 4.5, 'lighter').hex;
      var B = bodyFont(d.type.body), H = headFont(d.type.heading), fs = SCALES[d.type.scale][0];
      var same_ = H[0] === 'same', hz = same_ ? 1 : H[5];
      var E = ELEVATION[d.motion.elevation], S = SPEEDS[d.motion.speed], lively = d.nav.strength === 'lively';
      var hd = headerLook(d, T);
      return {
        '--p-bg':T.bg, '--p-card':T.card, '--p-alt':T.alt, '--p-line':T.line, '--p-line2':T.line2, '--p-text':T.text, '--p-body':T.body, '--p-muted':T.muted,
        '--p-accent':acc, '--p-accent-rgb':rgbCh(acc), '--p-accent-l-rgb':rgbCh(accL), '--p-link':link,
        '--p-fill':f.fill, '--p-fill-rgb':rgbCh(f.fill), '--p-on-fill':f.on, '--p-fill-deep':mix(f.fill, '#000000', 0.3),
        '--p-s1-rgb':rgbCh(s1), '--p-s1-fill':f1.fill, '--p-on-s1':f1.on, '--p-s1-text':s1t,
        '--p-s2-rgb':rgbCh(s2), '--p-s2-fill':f2.fill, '--p-on-s2':f2.on, '--p-s2-text':s2t,
        '--p-sb':sb.bg, '--p-sb-text':sb.text, '--p-sb-text-2':sb.text2, '--p-sb-text-3':sb.text3, '--p-sb-hi':'#ffffff', '--p-sb-line':'rgb(255 255 255 / .07)', '--p-sb-accent':sbAcc,
        '--p-font':B[2], '--p-font-h':same_ ? B[2] : H[2], '--p-hw':same_ ? '700' : String(H[4]), '--p-hw-b':same_ ? '700' : String(H[4]), '--p-hls':H[6],
        '--p-fs':fs + 'px', '--p-h1':(fs * 1.8 * hz).toFixed(2) + 'px', '--p-h2':(fs * 1.38 * hz).toFixed(2) + 'px', '--p-h3':(fs * 1.16 * hz).toFixed(2) + 'px',
        '--p-elev-rest':E.rest[theme], '--p-elev-hover':E.hover[theme].replace(/ACC/g, rgbCh(acc)), '--p-lift':E.lift + 'px',
        '--p-dur':S.d + 'ms', '--p-dur2':S.d2 + 'ms', '--p-ease':S.ease, '--p-shadow-rgb':theme === 'dark' ? '0 0 0' : '15 23 42',
        '--p-br':BTN_RADIUS[d.buttons.style],
        '--p-tint-a':lively ? '.26' : '.15', '--p-rail-w':lively ? '3px' : '2px', '--p-edge-w':lively ? '2px' : '1px', '--p-trace-a':lively ? '1' : '.85', '--p-band-a':lively ? '.28' : '.16',
        '--p-breath':lively ? '3.2s' : '5s', '--p-tide':lively ? '6s' : '9s',
        '--p-hdr-bg':hd.bg, '--p-hdr-rep':hd.rep, '--p-hdr-text':hd.text, '--p-hdr-text-2':hd.text2, '--p-hdr-chip':hd.chip, '--p-hdr-line':hd.line,
        '--p-scrim':String(SCRIMS[d.header.scrim][0])
      };
    }
    // Quiet notes for the Colours section, measured on light pages, where a
    // colour has to work hardest as words.
    function guardNote(which, d){
      var T = tones(d, 'light'), hex = d.colours[which], parts = [];
      if (!hex) return '';
      var f = fillFor(hex), name = colourName(hex);
      if (which === 'accent'){
        var link = readable(hex, [T.bg, T.card], 4.5, 'darker');
        if (f.flip) parts.push('buttons on it use dark words');
        else if (f.de > 0.03) parts.push('buttons use a slightly deeper shade');
        if (link.de > 0.03) parts.push('links use a deeper shade on light pages');
      } else if (which === 's1'){
        var t1 = readable(hex, [T.bg, T.card, over(hex, T.bg, 0.14)], 4.5, 'darker');
        if (f.flip) parts.push('headers and event chips on it use dark words');
        else if (f.de > 0.03) parts.push('headers use a slightly deeper shade');
        if (t1.de > 0.03) parts.push('coloured words use a deeper shade');
      } else {
        var t2 = readable(hex, [T.card, over(hex, T.card, 0.16)], 4.5, 'darker');
        if (f.flip) parts.push('badges on it use dark words');
        else if (f.de > 0.03) parts.push('badges use a slightly deeper shade');
        if (t2.de > 0.03) parts.push('tags use a deeper shade for their words');
      }
      if (!parts.length) return '';
      var list = parts.length === 1 ? parts[0] : parts.slice(0, -1).join(', ') + ' and ' + parts[parts.length - 1];
      return 'To keep words readable on ' + (name === 'Your colour' ? 'your colour' : name) + ', ' + list + '.';
    }

    /* ---------- Building the controls ---------- */
    // Swatch groups: which setting each writes, its list, and its extra option.
    var GROUPS = {
      accent:{ k:'colours.accent', list:PRESETS, label:'Chrome accent' },
      s1:    { k:'colours.s1', list:PRESETS, follow:true, label:'Surface A' },
      s2:    { k:'colours.s2', list:PRESETS, follow:true, label:'Surface B' },
      hsolid:{ k:'header.solid', list:HDR_COLOURS, page:true, label:'Header colour', deep:true },
      hfrom: { k:'header.from', list:HDR_COLOURS, label:'Gradient start', deep:true },
      hto:   { k:'header.to', list:HDR_COLOURS, label:'Gradient end', deep:true },
      sbown: { k:'sidebar.own', list:HDR_COLOURS, label:'Menu colour', deep:true },
      glow:  { k:'sidebar.glowColour', list:PRESETS, label:'Glow colour' }
    };
    function inList(g, v){ return GROUPS[g].list.some(function(x){ return x[1] === v; }); }
    function fld(id, label, sub, body){
      return '<div class="fld"><div class="lab" id="' + id + '-l">' + esc(label) + (sub ? '<small>' + esc(sub) + '</small>' : '') + '</div><div class="ctl">' + body + '</div></div>';
    }
    function opts(name, k, labelledby, items){
      return '<div class="opts" role="radiogroup" aria-labelledby="' + labelledby + '">' + items.map(function(it){
        return '<label class="opt"' + (it[4] ? ' data-hc="' + it[4] + '"' : '') + '><input type="radio" name="' + name + '" value="' + it[0] + '" data-k="' + k + '"' + (it[3] ? ' disabled' : '') + '><span>' +
          (it[2] ? '<i class="chip" style="--c:' + it[2] + '"></i>' : '') + esc(it[1]) + (it[5] ? '<em class="soon">' + esc(it[5]) + '</em>' : '') + '</span></label>';
      }).join('') + '</div>';
    }
    function swatches(g, labelledby){
      var G = GROUPS[g], name = 'sw-' + g, h = '<div class="sws" role="radiogroup" aria-labelledby="' + labelledby + '" data-swg="' + g + '">';
      if (G.follow) h += '<label class="sw" data-hc="sw:' + g + ':follow"><input type="radio" name="' + name + '" value="__follow" data-k="' + G.k + '" data-g="' + g + '" aria-label="Same as the chrome accent"><span class="sw-follow">' + IC('i-link') + '</span></label>';
      if (G.page) h += '<label class="sw page" data-hc="sw:' + g + ':page"><input type="radio" name="' + name + '" value="page" data-k="' + G.k + '" data-g="' + g + '" aria-label="Match the page"><span class="sw-page"></span></label>';
      G.list.forEach(function(p){
        h += '<label class="sw" data-hc="sw:' + g + ':' + p[1] + '"><input type="radio" name="' + name + '" value="' + p[1] + '" data-k="' + G.k + '" data-g="' + g + '" aria-label="' + esc(p[0]) + '"><span style="--c:' + p[1] + '"></span></label>';
      });
      h += '<label class="sw sw-cus" data-hc="sw:' + g + ':custom" hidden><input type="radio" name="' + name + '" value="__custom" data-k="' + G.k + '" data-g="' + g + '" aria-label="Your own colour"><span></span></label>';
      h += '<button type="button" class="sw-add" data-pick="' + g + '" aria-haspopup="dialog" aria-expanded="false" aria-label="Choose your own colour for ' + esc(G.label) + '">' + IC('i-plus') + '</button>';
      h += '<span class="sw-name" id="' + g + '-name"></span></div>';
      if (g === 'accent' || g === 's1' || g === 's2') h += '<p class="note" id="' + g + '-note" hidden><svg class="i" aria-hidden="true"><use href="#cz-i-info"/></svg><span></span></p>';
      return h;
    }
    var HL = '<span class="hl" aria-hidden="true"><span class="hl-tint"></span><span class="hl-rail"></span><span class="hl-wash"></span><span class="hl-band"></span><span class="hl-edge"></span>' +
      ['t', 'r', 'b', 'l'].map(function(s){ return '<span class="hl-trk ' + s + '"><i class="a"></i><i class="b"></i></span>'; }).join('') + '</span>';
    function tilesNav(kind){
      return NAV_STYLES.filter(function(s){ return s[2] === kind; }).map(function(s){
        return '<label class="tile nav-tile" data-hc="nav:' + s[0] + '"><input type="radio" name="t-nav" value="' + s[0] + '" data-k="nav.style"><span class="tb">' +
          '<span class="nt-s pv" data-nav="' + s[0] + '" data-own="' + s[0] + '" aria-hidden="true"><span class="s-row on" style="--c:#f87171"><span class="ic">' + IC('i-mappin') + '</span><span class="lb">Cities</span><span class="pn">Emberfall</span>' + HL + '</span></span>' +
          '<span class="tn">' + esc(s[1]) + '<span class="tck">' + IC('i-check') + '</span></span>' + (s[0] === 'ring' ? '<span class="tbadge">Signed default</span>' : '') + '</span></label>';
      }).join('');
    }
    function tilesBody(){
      return BODY_FONTS.map(function(f){
        return '<label class="tile ft"><input type="radio" name="t-body" value="' + f[0] + '" data-k="type.body"><span class="tb"><span class="tck">' + IC('i-check') + '</span>' +
          '<span class="ft-aa" style="font-family:' + esc(f[2]) + '" aria-hidden="true">Aa</span><span class="tn">' + esc(f[1]) + '</span><span class="tk">' + esc(f[3]) + '</span></span></label>';
      }).join('');
    }
    function tilesHead(){
      return HEAD_FONTS.map(function(f){
        return '<label class="tile ft' + (f[0] === 'same' ? ' wide' : '') + '"><input type="radio" name="t-head" value="' + f[0] + '" data-k="type.heading"><span class="tb"><span class="tck">' + IC('i-check') + '</span>' +
          '<span class="ft-aa"' + (f[2] ? ' style="font-family:' + esc(f[2]) + ';font-weight:' + f[4] + '"' : ' data-samefont') + ' aria-hidden="true">Aa</span><span class="tn">' + esc(f[1]) + '</span><span class="tk">' + esc(f[3]) + '</span></span></label>';
      }).join('');
    }
    function tilesBtn(){
      return BTN_STYLES.map(function(b){
        return '<label class="tile btn-tile" data-hc="btn:' + b[0] + '"><input type="radio" name="t-btn" value="' + b[0] + '" data-k="buttons.style"><span class="tb">' +
          '<span class="bt-s pv" data-btn="' + b[0] + '" data-own-br="' + b[0] + '" aria-hidden="true"><span class="pbtn pri"><span class="f">Save page</span></span></span>' +
          '<span class="tn">' + esc(b[1]) + '<span class="tck">' + IC('i-check') + '</span></span><span class="tk">' + esc(b[2]) + '</span></span></label>';
      }).join('');
    }
    function tilesElev(){
      return Object.keys(ELEVATION).map(function(id){
        var E = ELEVATION[id], k = { flat:'Cards sit on the page, with an outline on hover.', standard:'A soft shadow; cards lift a little on hover.', dramatic:'Deep shadows; cards rise clearly off the page.' }[id];
        return '<label class="tile el"><input type="radio" name="t-elev" value="' + id + '" data-k="motion.elevation"><span class="tb">' +
          '<span class="el-s pv" aria-hidden="true"><i data-elev-sample="' + id + '"></i></span><span class="tn">' + E.name + '<span class="tck">' + IC('i-check') + '</span></span><span class="tk">' + k + '</span></span></label>';
      }).join('');
    }
    function imgSlot(k, thumbCls, upLabel, hint){
      var id = k.replace('.', '-');
      return '<div class="img-row"><span class="thumb' + (thumbCls ? ' ' + thumbCls : '') + '" id="' + id + '-t" role="img" aria-label="Current picture"></span>' +
        '<button type="button" class="czb czb-s czb-sm" data-up="' + k + '">' + IC('i-upload') + esc(upLabel) + '</button>' +
        '<button type="button" class="czb czb-g czb-sm" data-rm="' + k + '">' + IC('i-trash') + 'Remove</button></div>' +
        '<p class="hint">' + hint + '</p><p class="err" id="' + id + '-e" role="alert" hidden></p>';
    }
    // Charcoal and Ink are fixed colours; Tinted and the owner's own are
    // painted into their chips as the draft changes (syncControls).
    function menuColourOpts(){
      return opts('sb-col', 'colours.sidebar', 'sb-col-l', MENU_COLOURS.map(function(c){
        return [c[0], c[1], c[0] === 'charcoal' ? SIDEBARS.charcoal.bg : c[0] === 'ink' ? SIDEBARS.ink.bg : '#2a2e3a', null, 'menu:' + c[0]];
      }));
    }
    function panelBody(id){
      if (id === 'brand') return (
        fld('b-name', 'Name', 'Up to 40 characters',
          '<input class="inp" id="b-name" type="text" maxlength="40" data-k="brand.name" placeholder="' + esc(CAMPAIGN) + '" aria-labelledby="b-name-l" aria-describedby="b-name-h">' +
          '<div class="row-h"><span class="hint" id="b-name-h">Shown at the top of the menu. Leave it empty to use the campaign’s name.</span><span class="cnt" id="b-name-c"></span></div>') +
        fld('brand-logo', 'Logo', 'Square works best', imgSlot('brand.logo', 'sq', 'Upload', 'Up to 1 MB. Without a logo, the menu shows your first letter on your chrome accent.')) +
        fld('b-welcome', 'Welcome message', 'Up to 160 characters',
          '<textarea class="inp" id="b-welcome" rows="3" maxlength="' + welcomeMax() + '" data-k="brand.welcome" aria-labelledby="b-welcome-l" aria-describedby="b-welcome-h"></textarea>' +
          '<div class="row-h"><span class="hint" id="b-welcome-h">Shown over the backdrop at the top of the dashboard.</span><span class="cnt" id="b-welcome-c"></span></div>') +
        fld('brand-backdrop', 'Backdrop', 'Behind the welcome', imgSlot('brand.backdrop', '', 'Upload', 'A soft shade keeps the welcome message readable over any picture. Up to 4 MB.')));
      if (id === 'header') return (
        fld('h-bg', 'Background', null,
          opts('h-bg', 'header.bg', 'h-bg-l', HDR_MODES.map(function(m){ return m[0] === 'sky' ? [m[0], m[1], null, true, 'hint:sky', 'Coming soon'] : [m[0], m[1]]; })) +
          '<div class="ctl" id="h-solid"><span class="hint" id="h-solid-l">Colour</span>' + swatches('hsolid', 'h-solid-l') + '</div>' +
          '<div class="ctl" id="h-grad"><span class="hint" id="h-from-l">From</span>' + swatches('hfrom', 'h-from-l') + '<span class="hint" id="h-to-l">To</span>' + swatches('hto', 'h-to-l') +
            '<span class="hint" id="h-dir-l">Direction</span>' + opts('h-dir', 'header.dir', 'h-dir-l', Object.keys(HDR_DIRS).map(function(k){ return [k, HDR_DIRS[k][1]]; })) +
            '<p class="hint" id="h-anim-h">The two colours drift slowly from side to side. They slow to a stop when nobody is using the page, and hold still for anyone who asks for less motion.</p></div>' +
          '<div class="ctl" id="h-img">' + imgSlot('header.image', '', 'Replace', 'Still or animated, 1.5 MB at most. The shade keeps the header’s words readable.') +
            '<span class="hint" id="h-scrim-l">Shade</span>' + opts('h-scrim', 'header.scrim', 'h-scrim-l', Object.keys(SCRIMS).map(function(k){ return [k, SCRIMS[k][1]]; })) + '</div>' +
          '<p class="hint">Words and buttons switch to white on dark backgrounds and pictures by themselves.</p>') +
        fld('h-height', 'Height', 'How tall the bar is', opts('h-height', 'header.height', 'h-height-l', HDR_HEIGHTS) +
          '<p class="hint">Slim is today’s bar. Tall gives pictures and moving colour more room.</p>') +
        fld('h-w', 'Widgets', 'Up to ' + WIDGET_SLOTS, '<p class="hint" id="w-full" aria-live="polite"></p><div class="wl" id="wl" aria-labelledby="h-w-l"></div>' +
          '<p class="hint">On a phone the bar shows the first widget and a +N button that opens the rest.</p>' +
          '<div class="tape-note"><span class="tape" aria-hidden="true"></span><p><b>Under construction.</b> These are on their way. The date and weather will only ever show today in your world, never days ahead.</p>' +
          '<div class="later-row"><span class="w-later">Sky background</span>' + LATER.map(function(n){ return '<span class="w-later">' + esc(n) + '</span>'; }).join('') + '</div></div>'));
      if (id === 'sidebar') return (
        fld('sb-col', 'Colour', 'The menu stays dark', menuColourOpts() +
          '<div class="ctl" id="sb-own"><span class="hint" id="sb-own-l">Your colour</span>' + swatches('sbown', 'sb-own-l') + '<p class="note" id="sb-own-note" hidden><svg class="i" aria-hidden="true"><use href="#cz-i-info"/></svg><span></span></p></div>' +
          '<p class="hint">The words in the menu pick their own shade. A whole look from the top of the page sets this too; changing it here makes the look Custom.</p>') +
        fld('sb-corner', 'Top-left corner', 'Above the menu', opts('sb-corner', 'sidebar.corner', 'sb-corner-l', CORNERS.map(function(c){ return [c[0], c[1]]; })) +
          '<div class="ctl" id="sb-sub-wrap"><label class="hint" for="sb-sub">Subtitle</label><input class="inp" id="sb-sub" type="text" maxlength="40" data-k="sidebar.subtitle" placeholder="The Drowned Crown, session 23" aria-describedby="sb-sub-c">' +
            '<div class="row-h"><span class="hint">One short line under the name. Up to 40 characters.</span><span class="cnt" id="sb-sub-c"></span></div></div>' +
          '<div class="ctl" id="sb-bnr-wrap">' + imgSlot('sidebar.banner', '', 'Upload', 'A picture behind the name, up to 1.5 MB. A soft shade keeps the name readable.') + '</div>' +
          '<p class="hint">Clicking the corner still goes to the dashboard. When the menu peeks out, only the logo shows.</p>') +
        fld('sb-glow', 'Peek glow', 'When the menu is hidden', opts('sb-glow', 'sidebar.glow', 'sb-glow-l', GLOWS.map(function(g){ return [g[0], g[1]]; })) +
          '<div class="ctl" id="sb-glow-own"><span class="hint" id="sb-glowc-l">Glow colour</span>' + swatches('glow', 'sb-glowc-l') + '</div>' +
          '<div class="glow-demo" aria-hidden="true"><i id="sb-glow-bar"></i><span>The light that grows when you rest the pointer on the left edge</span></div>' +
          '<p class="hint">Hide the menu with the arrow at its foot, then rest the pointer on the window’s left edge to see it.</p>'));
      if (id === 'nav') return (
        fld('n-style', 'Highlight', 'On the page you are on',
          '<div role="radiogroup" aria-labelledby="n-style-l" class="ctl"><p class="grp-l">Moving</p><div class="tiles two">' + tilesNav('moving') + '</div>' +
          '<p class="grp-l">Still</p><div class="tiles two">' + tilesNav('still') + '</div></div>' +
          '<p class="hint">Moving styles play in the preview; the samples here play when you point at them.</p>') +
        fld('n-str', 'Strength', null, opts('n-str', 'nav.strength', 'n-str-l', [['calm', 'Calm'], ['lively', 'Lively']]) +
          '<p class="hint">Calm is slower and softer. Lively is brighter and a little quicker.</p>') +
        fld('n-pn', 'Page name', null, opts('n-pn', 'nav.pageName', 'n-pn-l', [['hidden', 'Hidden'], ['row', 'In the row']]) +
          '<p class="hint">Shows the page you are on beside its menu entry, like Emberfall beside Cities.</p>') +
        fld('n-edit', 'Menu items', null, '<button type="button" class="lnk" id="n-edit">' + IC('i-pencil') + 'Edit the menu’s items</button>' +
          '<p class="hint">Reorder, pin, hide and group them with the pencil beside your campaign’s name at the top of the menu.</p>') +
        '<p class="note"><svg class="i" aria-hidden="true"><use href="#cz-i-info"/></svg><span>Members who ask their device for less motion always see the matching still style.</span></p>');
      if (id === 'colours') return (
        '<p class="sec-lead">Chrome is Chronicle’s own colour: the menu highlight, links and anything selected. The two surface colours belong to your pages: character headers, calendar events, timeline marks and tags.</p>' +
        fld('accent', 'Chrome accent', null, swatches('accent', 'accent-l')) +
        fld('s1', 'Surface A', 'Primary', swatches('s1', 's1-l')) +
        fld('s2', 'Surface B', 'Secondary', swatches('s2', 's2-l')) +
        fld('c-page', 'Page tone', null, opts('c-page', 'colours.page', 'c-page-l', [['cool', 'Cool', '#eef0f4'], ['warm', 'Warm', '#f1ebe3'], ['paper', 'Paper', '#eadfc6']])) +
        fld('c-con', 'Text contrast', null, opts('c-con', 'colours.contrast', 'c-con-l', [['standard', 'Standard'], ['high', 'High']]) +
          '<p class="hint">High makes body text and small print darker, and borders firmer.</p>') +
        '<p class="hint">Every colour keeps its words readable: Chronicle deepens or lightens a shade where words sit on it, and says so here. On dark pages and in the menu, links always use a lighter shade.</p>');
      if (id === 'type') return (
        fld('t-body', 'Text', 'For reading', '<div class="tiles two" role="radiogroup" aria-labelledby="t-body-l">' + tilesBody() + '</div>') +
        fld('t-head', 'Headings', 'Titles and names', '<div class="tiles two" role="radiogroup" aria-labelledby="t-head-l">' + tilesHead() + '</div>') +
        fld('t-scale', 'Size', null, opts('t-scale', 'type.scale', 't-scale-l', Object.keys(SCALES).map(function(k){ return [k, SCALES[k][1]]; })) +
          '<p class="hint">Compact fits more on a screen. Roomy is easier on tired eyes. Headings grow with the text.</p>'));
      if (id === 'buttons') return (
        fld('b-style', 'Style', 'Shape and motion', '<div class="tiles two" role="radiogroup" aria-labelledby="b-style-l">' + tilesBtn() + '</div>' +
          '<p class="hint">Point at a sample and press it. The buttons in the preview use your choice.</p>'));
      if (id === 'motion') return (
        fld('m-elev', 'Elevation', 'How far cards lift', '<div class="tiles rows" role="radiogroup" aria-labelledby="m-elev-l">' + tilesElev() + '</div>') +
        fld('m-speed', 'Motion', 'How quickly things move', opts('m-speed', 'motion.speed', 'm-speed-l', Object.keys(SPEEDS).map(function(k){ return [k, SPEEDS[k].name]; })) +
          '<p class="hint">Point at the cards and buttons in the preview to feel it.</p>') +
        fld('m-red', 'For everyone', null, '<label class="switch"><input type="checkbox" id="m-reduce" data-k="motion.reduceAll"><span class="trk"><span class="knob"></span></span>' +
          '<span class="sw-t"><b>Reduce motion for everyone in this campaign</b>Things fade instead of moving, the header holds still and the menu highlight stays still.</span></label>'));
      return '';
    }
    function buildUI(){
      $('#lk-row').innerHTML = LOOKS.map(function(L){
        return '<label class="lk" data-hc="look:' + L.id + '"><input type="radio" name="look" value="' + L.id + '" aria-describedby="lk-d-' + L.id + '"><span class="lk-b">' +
          '<span class="lk-scene" id="lk-s-' + L.id + '" aria-hidden="true"><span class="lk-sb"><i></i><i class="on"></i><i></i><i></i></span><span class="lk-pg"><span class="lk-hdr"></span><span class="lk-h">Aa</span><span class="lk-card"></span><span class="lk-btn"></span></span></span>' +
          '<span class="lk-n">' + esc(L.name) + '</span><span class="lk-d" id="lk-d-' + L.id + '">' + esc(L.blurb) + '</span><span class="lk-ck">' + IC('i-check') + '</span></span></label>';
      }).join('') +
      '<div class="lk lk-custom" id="lk-custom"><span class="lk-b"><span class="lk-scene" aria-hidden="true">' + IC('i-spark') + '</span><span class="lk-n">Custom</span>' +
        '<span class="lk-d" id="lk-custom-d">Change anything below and the look becomes your own.</span><span class="lk-ck">' + IC('i-check') + '</span></span></div>';
      $('#tl').innerHTML = SECTIONS.map(function(s, i){
        return '<button type="button" role="tab" class="tab" id="tab-' + s.id + '" aria-controls="panel-' + s.id + '" aria-selected="' + (i === 0) + '" tabindex="' + (i === 0 ? 0 : -1) + '">' +
          IC(s.icon) + '<span>' + esc(s.name) + '</span><span class="chg" aria-hidden="true"></span><span class="sr chg-t"></span></button>';
      }).join('');
      $('#panels').innerHTML = SECTIONS.map(function(s, i){
        return '<section class="sec" role="tabpanel" id="panel-' + s.id + '" aria-labelledby="tab-' + s.id + '"' + (i === 0 ? '' : ' hidden') + '>' +
          '<header class="sec-h"><div class="sec-t"><h2>' + esc(s.name) + '</h2><p>' + esc(s.desc) + '</p></div>' +
          '<div class="sec-r"><button type="button" class="reset" data-reset="' + s.id + '" aria-describedby="reset-c-' + s.id + '">' + IC('i-undo') + 'Reset this section</button><span class="reset-c" id="reset-c-' + s.id + '"></span></div></header>' +
          '<div class="sec-b">' + panelBody(s.id) + '</div></section>';
      }).join('');
    }

    /* ---------- Building the preview ----------
       Real chrome at a small size: the navigation (pinned rows and folding
       sections), the header, a dashboard and a
       city page. Nothing here is a picture of the site. */
    function srow(id, icon, label, count, colour, withHL){
      return '<div class="s-row" data-row="' + id + '"' + (colour ? ' style="--c:' + colour + '"' : '') + '><span class="ic">' + IC(icon) + '</span><span class="lb">' + label + '</span>' +
        (count != null ? '<span class="ct">' + count + '</span>' : '') + (withHL ? '<span class="pn"></span>' + HL : '') + '</div>';
    }
    function ssub(id, label, count, withHL){
      return '<div class="s-row sub" data-row="' + id + '" style="--c:#f87171"><span class="dot"></span><span class="lb">' + label + '</span><span class="ct">' + count + '</span>' + (withHL ? '<span class="pn"></span>' + HL : '') + '</div>';
    }
    function pbtn(kind, label, real){
      return real ? '<button type="button" class="pbtn ' + kind + '"><span class="f">' + label + '</span></button>' : '<span class="pbtn ' + kind + '"><span class="f">' + label + '</span></span>';
    }
    function siteHTML(){
      var sb = '<div class="s-sb"><div class="s-brand"><span class="s-bnr" id="d-bnr"></span><span class="s-logo" id="d-logo"></span><span class="s-btext"><span class="s-bname" id="d-bname"></span><span class="s-bsub" id="d-bsub"></span></span><span class="s-bchev">' + IC('i-chev') + '</span><span class="s-pen" id="d-pen">' + IC('i-pencil') + '</span></div><div class="s-list">' +
        srow('dash', 'i-home', 'Dashboard', null, null, true) +
        '<div class="s-gh">' + IC('i-tack') + 'Pinned</div>' +
        srow('journal', 'i-journal', 'Journal') + srow('calendar', 'i-cal', 'Calendar') + srow('sessions', 'i-d20', 'Sessions') + srow('maps', 'i-map', 'Maps') +
        '<div class="s-gh folded">Apps <span class="n">6</span>' + IC('i-chev', 'chev') + '</div>' +
        '<div class="s-gh">Categories <span class="n">8</span>' + IC('i-chev', 'chev') + '</div>' +
        srow('all', 'i-layers', 'All Pages', 226) + srow('locations', 'i-mappin', 'Locations', 64, '#f87171') +
        '<div class="s-subs">' + ssub('regions', 'Regions', 6) + ssub('cities', 'Cities', 14, true) + ssub('dungeons', 'Dungeons', 11) + '</div>' +
        srow('factions', 'i-banner', 'Factions', 18, '#fbbf24') + srow('items', 'i-gem', 'Items', 53, '#a78bfa') + srow('lore', 'i-scroll', 'Lore', 29, '#34d399') +
        '</div></div>';
      var hdr = '<div class="s-hdr"><div class="s-hdr-bg"><div class="s-hdr-drift" id="d-drift"></div><div class="s-hdr-img" id="d-himg"></div><div class="s-hdr-scrim"></div></div>' +
        '<div class="s-path" id="d-path"></div><div class="s-rail" id="d-rail"></div><div class="s-tools" id="d-tools"></div></div>';
      var dash = '<div class="s-page" data-page="dash"><div class="d-banner" id="d-banner"><div class="d-img" id="d-img"></div><div class="d-scrim"></div><p class="d-welcome" id="d-welcome"></p></div>' +
        '<div class="d-title"><div class="h1" id="d-title"></div><div class="d-act">' + pbtn('pri', 'New page') + pbtn('sec', 'Invite players') + '</div></div>' +
        '<div class="d-grid">' +
          '<div class="pcard lifts"><p class="c-h">The party</p><div class="band"><b>Sera Windrose</b><span class="czbadge">Level 6 · Ranger</span></div>' +
            '<div class="who"><span class="av">KV</span>Kaelen Voss<span>Rogue</span></div><div class="who"><span class="av b">BA</span>Brother Aldric<span>Cleric</span></div></div>' +
          '<div class="pcard lifts"><p class="c-h">Next session</p><div class="day"><span class="day-n">12</span><div class="day-t"><b>Harvestwane, Starday</b><span class="chipe a">Session 14 · The Warm Ash</span><span class="chipe b">Festival of Embers</span></div></div></div>' +
          '<div class="pcard lifts"><p class="c-h">Recent pages</p><div class="links"><span class="p-link">Emberfall</span><span class="p-link">Cinder Keep</span><span class="p-link">The Warm Ash</span></div><div class="tags"><span class="tag">#emberfall</span><span class="tag">#guild</span></div></div>' +
        '</div>' +
        '<div class="pcard"><p class="c-h">Timeline · Age of Ash</p><div class="tline">' +
          '<div class="tev"><span class="y">398 AF</span><span class="t">The Glass Wastes burn</span></div><div class="tev"><span class="y">406 AF</span><span class="t">Ashwright charter</span></div>' +
          '<div class="tev now"><span class="y">412 AF</span><span class="t">The Warm Ash opens</span></div><div class="tev"><span class="y">414 AF</span><span class="t">The salt caravan</span></div></div></div></div>';
      var city = '<div class="s-page" data-page="city" hidden><div class="e-banner" id="d-ebanner"></div>' +
        '<div class="e-head"><span class="kind">' + IC('i-mappin') + 'City</span><div class="h1">Emberfall</div><p class="lede">Capital of the Cinder Reach, built in the lee of Mount Varn where the ash falls warm.</p></div>' +
        '<div class="e-grid"><div class="e-body"><p>Emberfall grew from a waystation on the old salt road into the largest city north of the Glass Wastes. Its walls are cut from black basalt.</p>' +
          '<p>Margravine Ysolde Vane rules from <span class="p-link">Cinder Keep</span>. Her treaty with the <span class="p-link">Ashwright Guild</span> keeps the forges lit.</p>' +
          '<div class="d-act">' + pbtn('pri', 'Edit page') + pbtn('sec', 'Share with players') + '</div></div>' +
          '<div class="pcard lifts e-info"><p class="c-h">At a glance</p><dl><dt>Population</dt><dd>18,400</dd><dt>Ruler</dt><dd>Ysolde Vane</dd><dt>Founded</dt><dd>212 AF</dd></dl>' +
          '<div class="tags"><span class="tag">#cinder-reach</span><span class="tag">#capital</span></div></div></div></div>';
      return '<div class="site" aria-hidden="true">' + sb + '<div class="s-main">' + hdr + dash + city + '</div></div>';
    }
    function piecesHTML(){
      function pc(label, body){ return '<div class="pc"><div class="pc-l">' + label + '</div>' + body + '</div>'; }
      return '<div class="pieces">' +
        pc('Card · point at it', '<div class="pcard lifts pc-card" tabindex="0" role="group" aria-label="Sample card"><div class="h3">The Warm Ash</div><p>A tavern in the lower wards of Emberfall, linked from <span class="p-link">Cinder Keep</span>.</p><div class="tags"><span class="tag">#tavern</span><span class="tag">#emberfall</span></div></div>') +
        pc('Calendar day', '<div class="day big"><span class="day-n">12</span><div class="day-t"><b>Harvestwane, Starday</b><span class="chipe a">Session 14 · The Warm Ash</span><span class="chipe b">Festival of Embers</span></div></div>') +
        pc('Buttons · point and press', '<div class="pc-btns" id="pc-btns">' + pbtn('pri', 'Save page', true) + pbtn('sec', 'Share', true) + pbtn('qui', 'Cancel', true) + '</div>') +
        pc('Timeline event', '<div class="tl-ev"><span class="mk-dot"></span><div><span class="y">412 AF · Age of Ash</span><div class="h3">The Warm Ash opens</div><p>Old Brannoc pours the first ale.</p></div></div>') +
        '<div class="pc wide"><div class="pc-l">Menu rows · rest, hover, the page you are on</div><div class="mini-sb" aria-hidden="true">' +
          '<div class="s-row"><span class="ic">' + IC('i-map') + '</span><span class="lb">Maps</span></div>' +
          '<div class="s-row is-hover"><span class="ic">' + IC('i-journal') + '</span><span class="lb">Journal</span></div>' +
          '<div class="s-row on" data-mini="1" style="--c:#f87171"><span class="ic">' + IC('i-mappin') + '</span><span class="lb">Cities</span><span class="pn">Emberfall</span><span class="ct">14</span>' + HL + '</div></div></div>' +
        '</div>';
    }
    function typeHTML(){
      return '<div class="typeset"><div class="ts-meta" id="ts-meta"></div>' +
        '<div class="h1">Emberfall</div><p class="lede">Capital of the Cinder Reach, built in the lee of Mount Varn.</p>' +
        '<div class="h2">The salt road</div><p>Emberfall grew from a waystation on the old salt road into the largest city north of the Glass Wastes. Its walls are cut from black basalt, and its roofs are pitched steep so the ashfall slides off before it can settle.</p>' +
        '<div class="h3">Founding of the city</div><p>The first forge was lit in 212 AF. Within a generation the Ashwright Guild had turned a caravan stop into a market that never closes.</p></div>';
    }
    function buildPreview(){
      $('#preview').innerHTML =
        '<div class="zp" data-z="site"><div class="site-wrap" id="site-wrap">' + siteHTML() + '</div></div>' +
        '<div class="zp off" data-z="pieces" inert>' + piecesHTML() + '</div>' +
        '<div class="zp off" data-z="type" inert>' + typeHTML() + '</div>';
    }

    /* ---------- Rendering the draft ---------- */
    function setVars(el, vars){ for (var k in vars) el.style.setProperty(k, vars[k]); }
    function setBg(el, url){ var v = url ? 'url("' + url + '")' : 'none'; if (el && el._bg !== v){ el.style.backgroundImage = v; el._bg = v; } }
    function setHTML(el, html){ if (el && el._h !== html){ el.innerHTML = html; el._h = html; } }
    function setText(el, t){ if (el && el.textContent !== t) el.textContent = t; }
    function reduceAll(){ return !!draft.motion.reduceAll || reducedDevice(); }
    function effNav(style, reduce){ return reduce && STILL_OF[style] ? STILL_OF[style] : style; }
    function brandName(d){ return d.brand.name.trim() || CAMPAIGN; }
    function logoHTML(d){
      var url = imgURL(d.brand.logo);
      if (!url) return ['s-logo mono', esc(brandName(d).charAt(0).toUpperCase())];
      return ['s-logo', '<img alt="" src="' + esc(url) + '">'];
    }
    function railHTML(d){
      var h = d.header.widgets.map(function(w){
        if (w === 'era') return '<div class="rw"><span class="k">Era</span><span class="v">Age of Ash</span></div>';
        if (w === 'links') return '<div class="rw"><span class="k">Links</span><span class="v">' + esc(d.header.links.map(function(l){ return l.label.trim(); }).filter(Boolean).join(' · ') || 'No links yet') + '</span></div>';
        if (w === 'text') return '<div class="rw q"><span class="k">Note</span><span class="v">' + esc(d.header.text.trim() || 'Nothing written yet') + '</span></div>';
        if (w === 'search') return '<div class="rw srch">' + IC('i-search') + '<span>Search ' + esc(CAMPAIGN) + '…</span></div>';
        if (w === 'note') return '<div class="rw srch nt">' + IC('i-note') + '<span>Quick note</span></div>';
        return '';
      }).join('');
      return h;
    }
    function toolsHTML(d){
      var w = d.header.widgets, h = '';
      if (w.indexOf('search') < 0) h += '<span class="s-tool">' + IC('i-search') + '</span>';
      return h + '<span class="s-tool">' + IC('i-mask') + '</span><span class="s-tool">' + IC('i-bell') + '<span class="s-badge">2</span></span><span class="s-av">GM</span>';
    }
    function renderPreview(theme){
      var d = draft, pv = $('#preview'), reduce = reduceAll();
      pv.dataset.nav = effNav(d.nav.style, reduce);
      pv.dataset.navChosen = d.nav.style;
      pv.dataset.strength = d.nav.strength;
      pv.dataset.btn = d.buttons.style;
      pv.dataset.hdr = d.header.bg === 'sky' ? 'solid' : d.header.bg;
      pv.dataset.hdrh = d.header.height;
      pv.dataset.corner = d.sidebar.corner;
      pv.dataset.reduce = reduce ? '1' : '0';
      pv.dataset.theme = theme;
      pv.dataset.elev = d.motion.elevation;
      pv.dataset.speed = d.motion.speed;
      pv.dataset.pagename = d.nav.pageName;
      pv.dataset.page = ui.page;
      setText($('#d-bname'), brandName(d));
      setText($('#d-title'), brandName(d));
      var lg = logoHTML(d), le = $('#d-logo');
      if (le.className !== lg[0]) le.className = lg[0];
      setHTML(le, lg[1]);
      var burl = imgURL(d.brand.backdrop);
      setBg($('#d-img'), burl);
      $('#d-banner').classList.toggle('noimg', !burl);
      setText($('#d-welcome'), d.brand.welcome.trim());
      setBg($('#d-ebanner'), IMG.dusk);
      setBg($('#d-bnr'), d.sidebar.corner === 'banner' ? imgURL(d.sidebar.banner) : '');
      setText($('#d-bsub'), d.sidebar.corner === 'subtitle' ? d.sidebar.subtitle.trim() : '');
      setBg($('#d-himg'), imgURL(d.header.image));
      setHTML($('#d-path'), ui.page === 'dash' ? '<b>Dashboard</b>' : '<span>Locations</span>' + IC('i-chev-r') + '<span>Cities</span>' + IC('i-chev-r') + '<b>Emberfall</b>');
      setHTML($('#d-rail'), railHTML(d));
      setHTML($('#d-tools'), toolsHTML(d));
      $$('.s-page', pv).forEach(function(p){ p.hidden = p.dataset.page !== ui.page; });
      var act = ui.page === 'dash' ? 'dash' : 'cities';
      $$('.site .s-row[data-row]', pv).forEach(function(r){ r.classList.toggle('on', r.dataset.row === act); });
      // The page name sits in the active row when it names a page inside it.
      $$('.s-row .pn').forEach(function(pn){
        var row = pn.parentElement, name = '';
        if (d.nav.pageName === 'row' && row.classList.contains('on') && row.dataset.row !== 'dash') name = 'Emberfall';
        setText(pn, name);
        var ct = $('.ct', row); if (ct) ct.hidden = !!name;
      });
      var H = headFont(d.type.heading), B = bodyFont(d.type.body), sc = SCALES[d.type.scale];
      setHTML($('#ts-meta'), '<span>Headings <b>' + esc(H[0] === 'same' ? B[1] : H[1]) + '</b></span><span>Text <b>' + esc(B[1]) + '</b></span><span><b>' + sc[1] + '</b> size, ' + sc[0] + ' px text</span>');
    }
    function syncText(sel, v){ var el = $(sel); if (el && el.value !== v && document.activeElement !== el) el.value = v; }
    function customValue(g){ var v = getP(draft, GROUPS[g].k); return (v && v !== 'page' && !inList(g, v)) ? v : memo[g]; }
    function syncControls(theme){
      var d = draft, T = tones(d, theme);
      $$('input[type="radio"][data-k]').forEach(function(inp){
        var v = getP(d, inp.dataset.k), val = inp.value, on;
        if (val === '__follow') on = v == null;
        else if (val === '__custom') on = v != null && v !== 'page' && !inList(inp.dataset.g, v);
        else on = v === val;
        if (inp.checked !== on) inp.checked = on;
      });
      $('#m-reduce').checked = !!d.motion.reduceAll;
      syncText('#b-name', d.brand.name); syncText('#b-welcome', d.brand.welcome);
      setText($('#b-name-c'), d.brand.name.length + ' / 40');
      setText($('#b-welcome-c'), d.brand.welcome.length + ' / ' + welcomeMax());
      Object.keys(GROUPS).forEach(function(g){
        var G = GROUPS[g], v = getP(d, G.k), wrap = $('[data-swg="' + g + '"]'), cus = $('.sw-cus', wrap), cv = customValue(g);
        cus.hidden = !cv;
        if (cv){ $('span', cus).style.setProperty('--c', cv); cus.dataset.hc = 'sw:' + g + ':' + cv; $('input', cus).setAttribute('aria-label', 'Your own colour, ' + cv.toUpperCase()); }
        if (G.follow) $('.sw-follow', wrap).style.setProperty('--c', d.colours.accent);
        if (G.page){ var pg = $('.sw-page', wrap); pg.style.setProperty('--c', T.card); pg.style.setProperty('--c2', T.line2); }
        setText($('#' + g + '-name'), v == null ? 'Same as the chrome accent' : v === 'page' ? 'Matches the page' : colourName(v) + (inList(g, v) ? '' : ' · ' + v.toUpperCase()));
      });
      ['accent', 's1', 's2'].forEach(function(g){ var n = guardNote(g, d), el = $('#' + g + '-note'); el.hidden = !n; setText($('span', el), n); });
      var bg = d.header.bg;
      $('#h-solid').hidden = bg !== 'solid';
      $('#h-grad').hidden = bg !== 'gradient' && bg !== 'moving';
      $('#h-anim-h').hidden = bg !== 'moving';
      $('#h-img').hidden = bg !== 'image';
      var lg = logoHTML(d), lt = $('#brand-logo-t');
      if (!imgURL(d.brand.logo)){ lt.className = 'thumb sq'; setHTML(lt, IC('i-image')); lt.style.backgroundImage = ''; lt._bg = ''; }
      else { lt.className = 'thumb sq'; setHTML(lt, '<span class="' + lg[0] + '" style="width:44px;height:44px;border-radius:9px">' + lg[1] + '</span>'); }
      $('#brand-logo-t').setAttribute('aria-label', imgURL(d.brand.logo) ? 'Current logo' : 'No logo');
      // Sidebar section: the choices that go with each selection.
      var sbc = d.colours.sidebar, sbOwn = menuDark(d.sidebar.own);
      $('#sb-own').hidden = sbc !== 'own';
      $('#sb-sub-wrap').hidden = d.sidebar.corner !== 'subtitle';
      $('#sb-bnr-wrap').hidden = d.sidebar.corner !== 'banner';
      $('#sb-glow-own').hidden = d.sidebar.glow !== 'own';
      setText($('#sb-sub-c'), d.sidebar.subtitle.length + ' / 40');
      syncText('#sb-sub', d.sidebar.subtitle);
      $$('input[name="sb-col"]').forEach(function(i){
        var chip = $('.chip', i.parentNode);
        if (i.value === 'tinted') chip.style.setProperty('--c', menuTinted(d.colours.accent));
        else if (i.value === 'own') chip.style.setProperty('--c', sbOwn);
      });
      var ownNote = $('#sb-own-note'), darkened = sbOwn !== d.sidebar.own.toLowerCase();
      ownNote.hidden = !(sbc === 'own' && darkened);
      setText($('span', ownNote), 'Darkened a little so the menu stays readable.');
      $('#sb-glow-bar').style.setProperty('--g', d.sidebar.glow === 'own' ? d.sidebar.glowColour : d.colours.accent);
      [['brand.backdrop'], ['header.image'], ['sidebar.banner']].forEach(function(x){
        var id = x[0].replace('.', '-'), url = imgURL(getP(d, x[0])), t = $('#' + id + '-t');
        setBg(t, url); setHTML(t, url ? '' : IC('i-image')); t.setAttribute('aria-label', url ? 'Current picture' : 'No picture');
      });
      $$('[data-rm]').forEach(function(b){ b.disabled = getP(d, b.dataset.rm) === 'none'; });
      renderWidgets();
      var red = reducedDevice();
      $$('.nt-s').forEach(function(s){ s.dataset.nav = effNav(s.dataset.own, red); s.dataset.strength = d.nav.strength; });
      $$('.bt-s').forEach(function(s){ s.style.setProperty('--p-br', BTN_RADIUS[s.dataset.ownBr]); });
      $$('[data-elev-sample]').forEach(function(i){
        var E = ELEVATION[i.dataset.elevSample];
        i.style.boxShadow = E.hover[theme].replace(/ACC/g, rgbCh(d.colours.accent));
        i.style.transform = 'translateY(' + (-E.lift) + 'px)';
      });
      $$('[data-samefont]').forEach(function(s){ s.style.fontFamily = bodyFont(d.type.body)[2]; s.style.fontWeight = '700'; });
    }
    var lookTileTheme = null;
    function syncLookTiles(theme){
      if (lookTileTheme === theme) return;
      lookTileTheme = theme;
      LOOKS.forEach(function(L){
        var v = derive(applyLook(clone(BASE), L.id), theme), s = $('#lk-s-' + L.id);
        setVars(s, { '--t-sb':v['--p-sb'], '--t-bg':v['--p-bg'], '--t-ac':L.colours.accent, '--t-fill':v['--p-fill'], '--t-s1':v['--p-s1-fill'],
          '--t-hf':v['--p-font-h'], '--t-hw':v['--p-hw'], '--t-text':v['--p-text'], '--t-br':BTN_RADIUS[L.buttons.style], '--t-hdr':L.header.bg === 'image' ? '#0b0f1a' : v['--p-hdr-bg'] });
      });
    }
    function sectionName(id){ return SECTIONS.filter(function(s){ return s.id === id; })[0].name; }
    function listNames(ids){
      var n = ids.map(sectionName);
      return n.length === 1 ? n[0] : n.slice(0, -1).join(', ') + ' and ' + n[n.length - 1];
    }
    function resetCaption(sec){
      var L = look(draft.look).name;
      if (sec === 'brand') return 'Back to what is saved';
      if (sec === 'header') return 'Background to ' + L + ', height and widgets as saved';
      if (sec === 'sidebar') return 'Colour to ' + L + ', corner and glow as saved';
      return 'Back to the ' + L + ' look';
    }
    function syncStatus(){
      var cur = currentLook(draft), n = lookChanges(draft);
      $$('input[name="look"]').forEach(function(i){ if (i.checked !== (i.value === cur)) i.checked = i.value === cur; });
      $('#lk-custom').classList.toggle('on', !cur);
      setText($('#lk-custom-d'), cur ? 'Change anything below and the look becomes your own.' : 'Started from ' + look(draft.look).name + ', ' + n + ' change' + (n === 1 ? '' : 's'));
      var ch = changedSections();
      SECTIONS.forEach(function(s){
        var t = $('#tab-' + s.id), on = ch.indexOf(s.id) >= 0;
        t.classList.toggle('changed', on); setText($('.chg-t', t), on ? ', changed' : '');
        var b = $('[data-reset="' + s.id + '"]'); b.disabled = sectionAtTarget(s.id);
        setText($('#reset-c-' + s.id), resetCaption(s.id));
      });
      var dirty = ch.length > 0;
      if (dirty) ui.saved = false;
      $('#save').classList.toggle('dirty', dirty);
      $('#savebtn').disabled = !dirty; $('#discard').disabled = !dirty;
      setText($('#sv-t'), dirty ? 'Unsaved changes' : ui.saved ? 'All changes saved' : 'No unsaved changes');
      setText($('#sv-d'), dirty ? 'in ' + listNames(ch) : ui.saved ? 'Everyone in ' + CAMPAIGN + ' now sees this look.' : 'The preview matches what everyone sees.');
      setHTML($('#pv-cap'),
        '<span class="pill">' + IC('i-spark') + esc(cur ? look(cur).name + ' look' : 'Custom, from ' + look(draft.look).name) + '</span>' +
        (draft.motion.reduceAll ? '<span class="pill">' + IC('i-depth') + 'Reduced motion for everyone</span>' : '') +
        '<span>Click any part of the example site to change it. Save changes to show your draft to everyone.</span>');
    }
    function update(){
      var theme = previewTheme(), vars = derive(draft, theme);
      fzQueue();
      $$('.pv').forEach(function(el){ setVars(el, vars); });
      renderPreview(theme);
      syncControls(theme);
      syncLookTiles(theme);
      syncStatus();
      runMotion();
    }

    /* ---------- The living ring and the comet ----------
       Each trace is four bars, one per edge, each clipped by its own 2px track.
       A bar only ever moves by transform (translate plus a scale for its
       length), sampled from one path function so the four pieces join into a
       single trace that bends round the corners. */
    var ringAnims = new Map();
    function ringStop(hl){ var a = ringAnims.get(hl); if (a){ a.forEach(function(x){ x.cancel(); }); ringAnims.delete(hl); } hl._key = ''; }
    function ringStart(hl, kind, strength){
      ringStop(hl);
      var W = hl.offsetWidth, H = hl.offsetHeight;
      if (!W || !H) return;
      var P = 2 * (W + H), lively = strength === 'lively', TAU = Math.PI * 2, anims = [];
      var traces = kind === 'comet'
        ? [{ c:'a', T:lively ? 8000 : 12000, L0:0.16, Lv:0, pace:0, fp:1, fl:1, ph:0, at:0.3 }]
        : [{ c:'a', T:lively ? 9500 : 14000, L0:0.16, Lv:0.45, pace:0.35, fp:2, fl:3, ph:0, at:0.12 },
           { c:'b', T:lively ? 12500 : 19000, L0:0.12, Lv:0.5, pace:0.4, fp:3, fl:2, ph:0.37, at:0.58 }];
      var tracks = [{ s:'t', o:0, len:W, x:true, fwd:true }, { s:'r', o:W, len:H, x:false, fwd:true }, { s:'b', o:W + H, len:W, x:true, fwd:false }, { s:'l', o:2 * W + H, len:H, x:false, fwd:false }];
      traces.forEach(function(tr){
        function S(p){ return P * (p + tr.pace * Math.sin(TAU * tr.fp * p) / (TAU * tr.fp)); }
        function Lf(p){ return P * tr.L0 * (1 + tr.Lv * Math.sin(TAU * (tr.fl * p + tr.ph))); }
        var Lmax = P * tr.L0 * (1 + tr.Lv);
        tracks.forEach(function(tk){
          var bar = hl.querySelector('.hl-trk.' + tk.s + ' i.' + tr.c);
          if (!bar) return;
          bar.style.setProperty('--lm', Lmax.toFixed(1) + 'px');
          var lo = 0, hi = 1;
          for (var i = 0; i < 40; i++){ var m = (lo + hi) / 2; if (S(m) < tk.o) lo = m; else hi = m; }
          var pe = (lo + hi) / 2, N = 72, kf = [];
          for (var n = 0; n <= N; n++){
            var p = pe + n / N, u = S(p) - tk.o, L = Lf(p), pos = Math.min(u - L, tk.len);
            var off = tk.fwd ? pos : tk.len - pos - L;
            kf.push({ transform:(tk.x ? 'translateX(' : 'translateY(') + off.toFixed(2) + 'px) ' + (tk.x ? 'scaleX(' : 'scaleY(') + (L / Lmax).toFixed(4) + ')' });
          }
          anims.push(bar.animate(kf, { duration:tr.T, iterations:Infinity, iterationStart:((tr.at - pe) % 1 + 1) % 1, easing:'linear' }));
        });
      });
      ringAnims.set(hl, anims);
    }
    // on: play; still: show one paused frame (the option samples at rest).
    function hlMotion(hl, on, still){
      var host = hl.closest('[data-nav]'), eff = host.dataset.nav, row = hl.parentElement;
      var run = on && row.classList.contains('on');
      var key = eff + '|' + host.dataset.strength + '|' + hl.offsetWidth + 'x' + hl.offsetHeight + '|' + (run ? 'run' : 'still');
      if ((run || still) && (eff === 'ring' || eff === 'comet')){
        if (hl._key !== key){
          ringStart(hl, eff, host.dataset.strength); hl._key = key;
          if (!run) (ringAnims.get(hl) || []).forEach(function(a){ a.pause(); a.currentTime = 0; });
        }
      }
      else if (hl._key) ringStop(hl);
      $('.hl-wash', hl).classList.toggle('run', run && eff === 'breathe');
      $('.hl-band', hl).classList.toggle('run', run && eff === 'tide');
    }
    function runMotion(){
      var reduce = reduceAll();
      $$('#preview .hl').forEach(function(hl){ hlMotion(hl, !reduce && !hl.closest('.zp.off') && hl.offsetParent !== null); });
      $$('.nt-s .hl').forEach(function(hl){ var vis = hl.offsetParent !== null; hlMotion(hl, vis && !reducedDevice() && !!hl.closest('.nav-tile.play'), vis && !reducedDevice()); });
      driftSync();
    }

    /* ---------- The moving header, in the example ----------
       The same slide the real header makes: a transform on an over-wide strip,
       eased to a stop by Chronicle.restWake when nobody is using the page. */
    var drift = null;
    function driftStop(){ if (drift){ drift.rw.destroy(); drift.anim.cancel(); drift = null; } }
    function driftSync(){
      var el = $('#d-drift'), vertical = draft.header.dir === 'b';
      el.classList.toggle('v', vertical);
      var want = draft.header.bg === 'moving' && !reduceAll() && !!el.animate && !!(window.Chronicle && Chronicle.restWake);
      if (!want){ driftStop(); return; }
      if (drift && drift.vertical === vertical) return;
      driftStop();
      var anim = el.animate(
        vertical ? [{ transform:'translateY(0)' }, { transform:'translateY(-33.3333%)' }] : [{ transform:'translateX(0)' }, { transform:'translateX(-33.3333%)' }],
        { duration:36000, iterations:Infinity, easing:'linear' });
      anim.pause();
      var rw = null;
      rw = Chronicle.restWake.create({
        reduced:reduceAll,
        onLevel:function(level){ if (level > 0) anim.playbackRate = level; },
        onState:function(state){
          if (state === 'active'){ anim.playbackRate = Math.max(0.02, rw ? rw.level() : 0); anim.play(); } else anim.pause();
        }
      });
      drift = { anim:anim, rw:rw, vertical:vertical };
    }

    /* ---------- One-shot plays: show a motion setting without a pointer ---------- */
    var playTimers = [], playUntil = 0;
    function visibleIn(el){ return el.offsetParent !== null; }
    function play(what){
      playTimers.forEach(clearTimeout); playTimers = [];
      playUntil = Date.now() + 1200;
      var zone = $('#preview .zp:not(.off)');
      if (!zone) return;
      if (what === 'btn'){
        var b = $$('.pbtn.pri', zone).filter(visibleIn).slice(0, 2);
        b.forEach(function(x){ x.classList.add('hov'); });
        playTimers.push(setTimeout(function(){ b.forEach(function(x){ x.classList.add('prs'); inkBloom(x); }); }, 440));
        playTimers.push(setTimeout(function(){ b.forEach(function(x){ x.classList.remove('prs'); }); }, 660));
        playTimers.push(setTimeout(function(){ b.forEach(function(x){ x.classList.remove('hov'); }); }, 1080));
      } else {
        var c = $$('.pcard.lifts', zone).filter(visibleIn).slice(0, 1);
        c.forEach(function(x){ x.classList.add('hov'); });
        playTimers.push(setTimeout(function(){ c.forEach(function(x){ x.classList.remove('hov'); }); }, 950));
      }
    }
    function inkBloom(btn, cx, cy){
      var host = btn.closest('[data-btn]');
      if (!host || host.dataset.btn !== 'ink') return;
      var f = $('.f', btn), ink = $('.ink', f);
      if (!ink){ ink = document.createElement('span'); ink.className = 'ink'; f.insertBefore(ink, f.firstChild); }
      var r = f.getBoundingClientRect(), k = r.width / (f.offsetWidth || 1);
      var x = cx == null ? f.offsetWidth / 2 : (cx - r.left) / k, y = cy == null ? f.offsetHeight / 2 : (cy - r.top) / k;
      var dm = 2 * Math.hypot(Math.max(x, f.offsetWidth - x), Math.max(y, f.offsetHeight - y));
      ink.style.left = x + 'px'; ink.style.top = y + 'px'; ink.style.width = dm + 'px'; ink.style.height = dm + 'px';
      var still = reducedDevice() || (host.closest('#preview') && reduceAll());
      ink.animate(still
        ? [{ opacity:0.24, transform:'translate(-50%, -50%) scale(1)' }, { opacity:0, transform:'translate(-50%, -50%) scale(1)' }]
        : [{ opacity:0.3, transform:'translate(-50%, -50%) scale(0)' }, { opacity:0, transform:'translate(-50%, -50%) scale(1)' }],
        { duration:still ? 380 : 600, easing:'cubic-bezier(.2,.7,.3,1)' });
    }

    /* ---------- Header widgets ----------
       header.widgets is the enabled widgets in order. The list shows those
       first, then the ones that are off; the header holds WIDGET_SLOTS, and a
       full header disables the switches of the rest. */
    var openW = null;
    function widgetDesc(w, d){
      if (w === 'links') return d.header.links.map(function(l){ return l.label.trim(); }).filter(Boolean).join(' · ') || 'No links yet';
      if (w === 'text') return d.header.text.trim() || 'Nothing written yet';
      return WIDGETS[w].desc;
    }
    function wsetBody(w){
      if (w === 'links') return '<span class="wset-l">Links</span>' + draft.header.links.map(function(l, i){
        return '<div class="lrow"><input class="inp" id="wl-' + i + '-label" data-link="' + i + '.label" maxlength="30" placeholder="Name" aria-label="Link ' + (i + 1) + ' name">' +
          '<input class="inp" id="wl-' + i + '-url" data-link="' + i + '.url" maxlength="500" placeholder="/campaigns/… or https://…" aria-label="Link ' + (i + 1) + ' address"></div>';
      }).join('') + '<p class="hint">Everyone who can view the campaign sees these links.</p>';
      return '<label class="wset-l" for="wt-text">Text</label><input class="inp" id="wt-text" maxlength="80" data-wtext="1"><p class="hint">One line, up to 80 characters.</p>';
    }
    function renderWidgets(){
      var d = draft, on = d.header.widgets, full = on.length >= WIDGET_SLOTS, wl = $('#wl');
      var rows = on.concat(WIDGET_ORDER.filter(function(w){ return on.indexOf(w) < 0; }));
      if (openW && on.indexOf(openW) < 0){ openW = null; dropLayer('wset'); }
      var sig = on.join(',') + '|' + openW;
      if (wl._sig !== sig){
        wl._sig = sig;
        wl.innerHTML = rows.map(function(w){
          var W = WIDGETS[w], i = on.indexOf(w), isOn = i >= 0, set = isOn && (w === 'links' || w === 'text');
          return '<div class="wr' + (isOn ? '' : ' off') + '" data-w="' + w + '">' +
            '<input type="checkbox" class="wtog" id="wtog-' + w + '" data-wtog="' + w + '" aria-label="Show ' + W.name + ' in the header"' + (isOn ? ' checked' : '') + (!isOn && full ? ' disabled' : '') + '>' +
            '<label class="wr-t" for="wtog-' + w + '"><b>' + W.name + '</b><span data-wdesc="' + w + '"></span></label>' +
            '<button type="button" class="ib" data-wact="up" data-w="' + w + '" aria-label="Move ' + W.name + ' up"' + (i <= 0 ? ' disabled' : '') + '>' + IC('i-up') + '</button>' +
            '<button type="button" class="ib" data-wact="down" data-w="' + w + '" aria-label="Move ' + W.name + ' down"' + (!isOn || i === on.length - 1 ? ' disabled' : '') + '>' + IC('i-down') + '</button>' +
            (set ? '<button type="button" class="ib" data-wact="set" data-w="' + w + '" aria-expanded="' + (openW === w) + '" aria-controls="wset-' + w + '" aria-label="' + W.name + ' settings">' + IC('i-gear') + '</button>' : '') +
            '</div>' +
            (set ? '<div class="wset" id="wset-' + w + '"' + (openW === w ? '' : ' hidden') + '>' + wsetBody(w) + '</div>' : '');
        }).join('');
      }
      var fullEl = $('#w-full');
      fullEl.classList.toggle('is-full', full);
      setHTML(fullEl, full ? '<b>The header is full: ' + on.length + ' of ' + WIDGET_SLOTS + '.</b> Switch one off to add another.' : 'Showing <b>' + on.length + ' of ' + WIDGET_SLOTS + '</b>. Switch widgets on, and use the arrows to put them in order.');
      rows.forEach(function(w){ setText($('[data-wdesc="' + w + '"]'), widgetDesc(w, d)); });
      d.header.links.forEach(function(l, i){ syncText('#wl-' + i + '-label', l.label); syncText('#wl-' + i + '-url', l.url); });
      syncText('#wt-text', d.header.text);
    }
    function focusW(sel){ var b = $(sel); if (b && !b.disabled){ b.focus(); return true; } return false; }
    // A switch changes the list; the list's sig changes, so the rows are
    // rebuilt and focus is put back on the same switch.
    function widgetToggle(w, want){
      var list = draft.header.widgets.slice(), i = list.indexOf(w), name = WIDGETS[w].name;
      if (want && i < 0){
        if (list.length >= WIDGET_SLOTS){ update(); announce('The header is full: ' + WIDGET_SLOTS + ' of ' + WIDGET_SLOTS + '.'); return; }
        list.push(w);
      } else if (!want && i >= 0){
        list.splice(i, 1);
        if (openW === w){ openW = null; dropLayer('wset'); }
      } else return;
      draft.header.widgets = list;
      update();
      focusW('#wtog-' + w);
      announce(name + (want ? ' added to the header, place ' + list.length + ' of ' + WIDGET_SLOTS + '.' : ' removed from the header.'));
    }
    function widgetAct(act, w){
      if (act === 'set'){ toggleWset(w); return; }
      var list = draft.header.widgets.slice(), i = list.indexOf(w), name = WIDGETS[w].name;
      if (act === 'up' && i > 0){ list.splice(i, 1); list.splice(i - 1, 0, w); }
      else if (act === 'down' && i >= 0 && i < list.length - 1){ list.splice(i, 1); list.splice(i + 1, 0, w); }
      else return;
      draft.header.widgets = list;
      update();
      focusW('[data-wact="' + act + '"][data-w="' + w + '"]') || focusW('[data-wact="' + (act === 'up' ? 'down' : 'up') + '"][data-w="' + w + '"]');
      announce(name + ' moved to place ' + (list.indexOf(w) + 1) + '.');
    }
    function toggleWset(w){
      if (openW === w){ closeWset(true); return; }
      openW = w; renderWidgets();
      pushLayer({ id:'wset', close:closeWset });
      var first = $('#wset-' + w + ' input'); if (first) first.focus();
    }
    function closeWset(restore){
      var w = openW; if (!w) return;
      openW = null; dropLayer('wset'); renderWidgets();
      if (restore) focusW('[data-wact="set"][data-w="' + w + '"]');
    }

    /* ---------- Layers: Escape closes the topmost and returns focus ---------- */
    var layers = [];
    function pushLayer(l){ dropLayer(l.id); layers.push(l); }
    function dropLayer(id){ layers = layers.filter(function(x){ return x.id !== id; }); }

    /* ---------- Your own colour: a hue and a tone, never neon ---------- */
    var pick = null;
    function pickDeep(){ return !!(pick && GROUPS[pick.g].deep); }
    function paintPicker(hex){
      $('#pop-sw').style.setProperty('--c', hex);
      setText($('#pop-hexl'), hex.toUpperCase() + (colourName(hex) !== 'Your colour' ? ' · ' + colourName(hex) : ''));
      var h = +$('#pop-hue').value, t = +$('#pop-tone').value, deep = pickDeep(), hs = [];
      for (var i = 0; i <= 12; i++) hs.push(fromPicker(i * 30, t, deep) + ' ' + (i * 100 / 12).toFixed(1) + '%');
      $('#pop-hue').style.setProperty('--track', 'linear-gradient(to right, ' + hs.join(', ') + ')');
      $('#pop-tone').style.setProperty('--track', 'linear-gradient(to right, ' + fromPicker(h, 0, deep) + ', ' + fromPicker(h, 50, deep) + ', ' + fromPicker(h, 100, deep) + ')');
    }
    function placePop(btn){
      var pop = $('#pop'), r = btn.getBoundingClientRect(), w = pop.offsetWidth, h = pop.offsetHeight, vw = document.documentElement.clientWidth;
      var left = clamp(r.left - 8, 16, vw - w - 16), top = r.bottom + 8;
      if (top + h > window.innerHeight - 72 && r.top - h - 8 > 8) top = r.top - h - 8;
      pop.style.left = left + 'px'; pop.style.top = top + 'px';
    }
    function openPicker(btn){
      var g = btn.dataset.pick, G = GROUPS[g], cur = getP(draft, G.k);
      var start = cur && cur !== 'page' ? cur : (G.follow ? draft.colours.accent : '#1e2a5a');
      pick = { g:g, btn:btn };
      var pp = toPicker(start, pickDeep());
      $('#pop-hue').value = pp.h; $('#pop-tone').value = pp.t; $('#pop-hex').value = start.toUpperCase();
      setText($('#pop-t'), 'Your own colour: ' + G.label);
      paintPicker(start);
      $('#pop-note').hidden = true;
      var pop = $('#pop');
      pop.hidden = false; placePop(btn); void pop.offsetWidth; pop.classList.add('on');
      btn.setAttribute('aria-expanded', 'true');
      pushLayer({ id:'pop', close:closePicker });
      $('#pop-hue').focus();
    }
    function closePicker(restore){
      if (!pick) return;
      var btn = pick.btn, pop = $('#pop');
      pop.classList.remove('on'); btn.setAttribute('aria-expanded', 'false');
      dropLayer('pop'); pick = null;
      setTimeout(function(){ if (!pop.classList.contains('on')) pop.hidden = true; }, 220);
      if (restore) btn.focus();
    }
    function applyPick(hex, typed){
      if (!pick) return;
      var G = GROUPS[pick.g], t = tame(hex, G.deep);
      memo[pick.g] = t.hex; setP(draft, G.k, t.hex);
      update();
      if (typed){ var pp = toPicker(t.hex, G.deep); $('#pop-hue').value = pp.h; $('#pop-tone').value = pp.t; }
      paintPicker(t.hex);
      if (document.activeElement !== $('#pop-hex') || typed) $('#pop-hex').value = t.hex.toUpperCase();
      var note = $('#pop-note');
      note.hidden = !(typed && t.toned);
      if (typed && t.toned) setText($('span', note), 'Toned down from ' + typed.toUpperCase() + ' so it won’t glare. This is the closest calm colour.');
    }
    function useHex(){
      var v = $('#pop-hex').value.trim(); if (v.charAt(0) !== '#') v = '#' + v;
      if (!/^#[0-9a-f]{6}$/i.test(v)){ var n = $('#pop-note'); n.hidden = false; setText($('span', n), 'Type six letters or numbers after the #, like #5B6BE0.'); return; }
      applyPick(v.toLowerCase(), v);
    }

    /* ---------- Hover cards: only after the pointer rests, gone on leave ---------- */
    var hc = $('#hc'), hcFor = null, hcTimer = 0, hcX = 0, hcY = 0, hcOn = false;
    function btnName(id){ return BTN_STYLES.filter(function(b){ return b[0] === id; })[0][1]; }
    function hcText(key){
      var p = key.split(':'), t = p[0];
      if (t === 'look'){
        var L = look(p[1]), H = headFont(L.type.heading), B = bodyFont(L.type.body);
        return '<b>' + esc(L.name) + '</b><span>' + esc(L.blurb) + '. ' + esc(H[0] === 'same' ? B[1] + ' throughout' : H[1] + ' headings on ' + B[1] + ' text') + ', ' + btnName(L.buttons.style) + ' buttons, ' +
          ELEVATION[L.motion.elevation].name.toLowerCase() + ' depth, ' + SPEEDS[L.motion.speed].name.toLowerCase() + ' motion and the ' + navName(L.nav.style).toLowerCase() + ' highlight.</span>';
      }
      if (t === 'sw'){
        var g = p[1], v = p.slice(2).join(':');
        if (v === 'follow') return '<b>Same as the chrome accent</b><span>Changes whenever the chrome accent does.</span>';
        if (v === 'page') return '<b>Match the page</b><span>A light header on light pages and a dark one on dark pages.</span>';
        if (v === 'custom') return '';
        var n = '';
        if (g === 'accent' || g === 's1' || g === 's2'){ var d2 = clone(draft); d2.colours[g] = v; n = guardNote(g, d2); }
        return '<b>' + esc(colourName(v)) + ' · ' + v.toUpperCase() + '</b>' + (n ? '<span>' + esc(n) + '</span>' : '');
      }
      if (t === 'nav'){
        var s = NAV_STYLES.filter(function(x){ return x[0] === p[1]; })[0];
        return '<b>' + esc(s[1]) + '</b><span>' + esc(s[3]) + (STILL_OF[s[0]] ? ' With less motion it shows as ' + navName(STILL_OF[s[0]]).toLowerCase() + '.' : '') + '</span>';
      }
      if (t === 'btn'){ var b = BTN_STYLES.filter(function(x){ return x[0] === p[1]; })[0]; return '<b>' + esc(b[1]) + '</b><span>' + esc(b[2]) + '</span>'; }
      if (t === 'menu'){ var mc = MENU_COLOURS.filter(function(x){ return x[0] === p[1]; })[0]; return '<b>' + esc(mc[1]) + '</b><span>' + esc(mc[2]) + '</span>'; }
      if (t === 'hint') return '<b>Sky</b><span>Coming soon: the living sky over your world as it is in the story right now.</span>';
      return '';
    }
    function hcCtl(el){ return el.matches('input, button') ? el : ($('input, button', el) || el); }
    function hcArm(el, delay){ clearTimeout(hcTimer); hcTimer = setTimeout(function(){ hcShow(el); }, delay); }
    function hcShow(el){
      var html = el.dataset.hc ? hcText(el.dataset.hc) : '';
      if (!html || !document.contains(el) || el.offsetParent === null) return;
      hc.innerHTML = html; hc.hidden = false; hc.classList.remove('now');
      var r = el.getBoundingClientRect(), w = hc.offsetWidth, h = hc.offsetHeight, vw = document.documentElement.clientWidth;
      var left = clamp(r.left + r.width / 2 - w / 2, 12, vw - w - 12), top = r.bottom + 8;
      if (top + h > window.innerHeight - 8) top = r.top - h - 8;
      hc.style.left = left + 'px'; hc.style.top = top + 'px';
      void hc.offsetWidth; hc.classList.add('on'); hcOn = true;
      var c = hcCtl(el); c.setAttribute('aria-describedby', ((c.getAttribute('aria-describedby') || '').replace(/\bhc\b/g, '') + ' hc').trim());
    }
    function hcHide(){
      clearTimeout(hcTimer);
      if (hcOn){ hc.classList.add('now'); hc.classList.remove('on'); hc.hidden = true; hcOn = false; }
      if (hcFor){ var c = hcCtl(hcFor), a = (c.getAttribute('aria-describedby') || '').replace(/\bhc\b/g, '').trim(); if (a) c.setAttribute('aria-describedby', a); else c.removeAttribute('aria-describedby'); }
      hcFor = null;
    }
    on(ROOT, 'pointerover', function(e){
      if (e.pointerType === 'touch') return;
      var el = e.target.closest ? e.target.closest('[data-hc]') : null;
      if (el === hcFor) return;
      hcHide();
      if (!el) return;
      hcFor = el; hcX = e.clientX; hcY = e.clientY; hcArm(el, 520);
    });
    on(ROOT, 'pointermove', function(e){
      if (!hcFor || hcOn || e.pointerType === 'touch') return;
      if (Math.abs(e.clientX - hcX) + Math.abs(e.clientY - hcY) > 4){ hcX = e.clientX; hcY = e.clientY; hcArm(hcFor, 520); }
    });
    on(ROOT, 'pointerout', function(e){ if (hcFor && !(e.relatedTarget && hcFor.contains(e.relatedTarget))) hcHide(); });
    on(document, 'pointerdown', function(){ hcHide(); }, true);
    on(document, 'scroll', function(){ if (hcOn || hcFor) hcHide(); if (pick) placePop(pick.btn); }, { passive:true, capture:true });
    on(ROOT, 'focusin', function(e){
      var el = e.target.closest ? e.target.closest('[data-hc]') : null;
      if (!el || !e.target.matches(':focus-visible')) return;
      hcHide(); hcFor = el; hcArm(el, 700);
    });
    on(ROOT, 'focusout', function(e){ if (hcFor && !(e.relatedTarget && hcFor.contains(e.relatedTarget))) hcHide(); });

    /* ---------- Sections, zoom, page, looks ---------- */
    function announce(msg){ var el = $('#live'); if (!el) return; el.textContent = ''; setTimeout(function(){ el.textContent = msg; }, 30); }
    function settle(){ var f = $('#preview'); f.classList.remove('settle'); void f.offsetWidth; f.classList.add('settle'); }
    function setPage(p){ ui.page = p; $('#dp-' + p).checked = true; update(); }
    function setZoom(z){
      ui.zoom = z; $('#z-' + z).checked = true;
      $$('#preview .zp').forEach(function(p){ var on = p.dataset.z === z; p.classList.toggle('off', !on); p.inert = !on; });
      $('#preview').dataset.zoom = z;
      $$('input[name="dpage"]').forEach(function(i){ i.disabled = z !== 'site'; });
      if (z !== 'site'){ fzHover(null); fzSet(null); }
      runMotion();
    }
    function clearErrors(){ $$('.err').forEach(function(e){ e.hidden = true; }); }
    function selectSection(id, focus){
      var tab = $('#tab-' + id);
      if (ui.section !== id) clearErrors();
      if (ui.section !== id){
        ui.section = id;
        SECTIONS.forEach(function(s){
          var t = $('#tab-' + s.id), on = s.id === id, p = $('#panel-' + s.id);
          t.setAttribute('aria-selected', String(on)); t.tabIndex = on ? 0 : -1;
          p.hidden = !on;
          if (on){ p.classList.remove('enter'); void p.offsetWidth; p.classList.add('enter'); }
        });
        if (pick) closePicker(false);
        if (openW) closeWset(false);
        if (id === 'nav' && ui.page !== 'city') setPage('city');
        else if (id === 'brand' && ui.page !== 'dash') setPage('dash');
        else update();
        var tl = $('#tl');
        if (tl.scrollWidth > tl.clientWidth + 1) tl.scrollLeft = tab.offsetLeft - tl.offsetLeft - (tl.clientWidth - tab.offsetWidth) / 2;
      }
      if (focus) tab.focus();
      fzSection(id);
    }
    function chooseLook(id){
      applyLook(draft, id); settle(); update(); fzFlash('*');
      announce('Previewing the ' + look(id).name + ' look. Save changes to use it.');
    }
    function resetSection(sec){
      var t = resetTarget(sec);
      Object.keys(t).forEach(function(k){ setP(draft, k, clone(t[k])); });
      update(); settle();
      $('#tab-' + sec).focus();
      toast(sectionName(sec) + (sec === 'brand' ? ' is back to what is saved.' : ' is back to the ' + look(draft.look).name + ' look.'));
    }

    /* ---------- Save, Discard, Undo ---------- */
    var toastT = 0;
    function toast(msg, undo){
      var el = $('#toast');
      setText($('#toast-t'), msg); $('#toast-undo').hidden = !undo;
      el.classList.add('on'); clearTimeout(toastT);
      toastT = setTimeout(function(){ el.classList.remove('on'); }, undo ? 6500 : 3400);
    }
    // The draft in the shape PUT /campaigns/:id/appearance takes.
    function pic(v){ return v && v !== 'none' ? v : ''; }
    function toInput(d){
      var h = d.header, bg = h.bg === 'solid' ? (h.solid === 'page' ? '' : 'solid') : h.bg;
      var drift = bg === 'gradient' || bg === 'moving', sb = d.sidebar;
      return {
        look:d.look,
        brand:{ name:d.brand.name.trim(), logo:pic(d.brand.logo), welcome:d.brand.welcome, backdrop:pic(d.brand.backdrop) },
        header:{ bg:bg, height:h.height, color:bg === 'solid' ? h.solid : '', from:drift ? h.from : '', to:drift ? h.to : '',
          dir:drift ? 'to-' + h.dir : '', image:bg === 'image' ? pic(h.image) : '', scrim:h.scrim,
          widgets:h.widgets.slice(), links:h.links.filter(function(l){ return l.label.trim() || l.url.trim(); }), text:h.text },
        colours:{ accent:d.colours.accent, s1:d.colours.s1 || '', s2:d.colours.s2 || '', page:d.colours.page, contrast:d.colours.contrast },
        nav:clone(d.nav), type:clone(d.type), buttons:clone(d.buttons), motion:clone(d.motion),
        // The companion of each choice is sent only while that choice is
        // selected, so nothing stale is stored.
        sidebar:{ colour:d.colours.sidebar, own:d.colours.sidebar === 'own' ? sb.own : '', corner:sb.corner,
          subtitle:sb.corner === 'subtitle' ? sb.subtitle.trim() : '', banner:sb.corner === 'banner' ? pic(sb.banner) : '',
          glow:sb.glow, glowColour:sb.glow === 'own' ? sb.glowColour : '' }
      };
    }
    // A problem the page can name before asking the server: it opens the
    // section and says what is missing, next to the setting.
    function preSaveProblem(d){
      if (d.header.bg === 'image' && !pic(d.header.image)) return ['header', 'header-image-e', 'Choose a picture for the header, or pick another background.'];
      if (d.sidebar.corner === 'banner' && !pic(d.sidebar.banner)) return ['sidebar', 'sidebar-banner-e', 'Choose a picture for the banner, or pick another corner.'];
      var bad = d.header.links.filter(function(l){ return l.label.trim() && !l.url.trim(); })[0];
      if (bad && d.header.widgets.indexOf('links') >= 0) return ['header', null, 'The link “' + bad.label.trim() + '” needs an address.'];
      return null;
    }
    var saving = false;
    function doSave(){
      if (saving) return;
      clearErrors();
      var p = preSaveProblem(draft);
      if (p){
        selectSection(p[0]);
        if (p[1]){ var e = $('#' + p[1]); e.hidden = false; e.textContent = p[2]; }
        toast(p[2]);
        return;
      }
      var sent = clone(draft);
      saving = true; $('#savebtn').disabled = true; setText($('#sv-t'), 'Saving…');
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(CID) + '/appearance', { method:'PUT', body:toInput(sent), csrfToken:CSRF })
        .then(function(res){
          if (res.ok) return null;
          return res.json().then(function(b){ return (b && (b.message || b.error)) || 'Saving failed.'; }, function(){ return 'Saving failed.'; });
        })
        .catch(function(){ return 'Chronicle could not be reached. Your draft is still here.'; })
        .then(function(problem){
          saving = false;
          if (!alive) return;
          if (problem){ update(); toast('Not saved: ' + problem); return; }
          saved = sent; BASE = clone(sent); lookTileTheme = null;
          ui.saved = true; undoDraft = null; update();
          toast('Saved. Everyone in ' + CAMPAIGN + ' now sees this look.');
          refreshSite();
        });
    }
    function doDiscard(){ clearErrors(); undoDraft = clone(draft); draft = clone(saved); if (pick) closePicker(false); update(); settle(); toast('Changes thrown away.', true); $('#tab-' + ui.section).focus(); }
    function doUndo(){ if (!undoDraft) return; draft = undoDraft; undoDraft = null; update(); settle(); toast('Your changes are back.'); }

    /* ---------- Pictures ---------- */
    var UPLOAD_KIND = { 'brand.logo':'logo', 'brand.backdrop':'backdrop', 'header.image':'header', 'sidebar.banner':'menu' };
    var upKey = null, LIMITS = { 'brand.logo':[1, 'The logo'], 'brand.backdrop':[4, 'The backdrop'], 'header.image':[1.5, 'The header'], 'sidebar.banner':[1.5, 'The banner'] };
    function onFile(input){
      var f = input.files && input.files[0], k = upKey;
      if (!f || !k) return;
      var err = $('#' + k.replace('.', '-') + '-e'), lim = LIMITS[k];
      if (!/^image\//.test(f.type)){ err.hidden = false; err.textContent = 'That file is not a picture. Choose a PNG, JPG, GIF or WebP.'; return; }
      if (f.size > lim[0] * 1048576){ err.hidden = false; err.textContent = 'That picture is ' + (f.size / 1048576).toFixed(1) + ' MB. ' + lim[1] + ' takes pictures up to ' + lim[0] + ' MB.'; return; }
      err.hidden = true;
      // Stored now so the preview can show it; the campaign only uses it once
      // the draft holding it is saved.
      var fd = new FormData();
      fd.append('kind', UPLOAD_KIND[k]); fd.append('file', f);
      var btn = $('[data-up="' + k + '"]'); btn.disabled = true;
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(CID) + '/appearance/picture', { method:'POST', body:fd, csrfToken:CSRF })
        .then(function(res){
          return res.json().then(function(b){ return { ok:res.ok, b:b }; }, function(){ return { ok:false, b:null }; });
        })
        .catch(function(){ return { ok:false, b:{ message:'Chronicle could not be reached.' } }; })
        .then(function(r){
          if (!alive) return;
          btn.disabled = false;
          if (!r.ok || !r.b || !r.b.name){ err.hidden = false; err.textContent = (r.b && (r.b.message || r.b.error)) || 'That picture could not be uploaded.'; return; }
          PICS[r.b.name] = r.b.url;
          setP(draft, k, r.b.name); update(); announce('Picture added to your draft.');
        });
    }
    function showPencil(){
      setZoom('site');
      var p = $('#d-pen'); p.classList.remove('pulse'); void p.offsetWidth; p.classList.add('pulse');
      clearTimeout(p._t); p._t = setTimeout(function(){ p.classList.remove('pulse'); }, 3800);
      toast('Use the pencil beside your campaign’s name at the top of the menu.');
    }

    /* ---------- Events ---------- */
    on(ROOT, 'change', function(e){
      var t = e.target;
      if (t.name === 'look'){ if (t.checked) chooseLook(t.value); return; }
      if (t.name === 'zoom'){ setZoom(t.value); return; }
      if (t.name === 'dpage'){ setPage(t.value); return; }
      if (t.name === 'ptheme'){ ui.ptheme = t.value; update(); return; }
      if (t.id === 'file'){ onFile(t); return; }
      if (t.dataset.wtog){ widgetToggle(t.dataset.wtog, t.checked); return; }
      var k = t.dataset.k, v;
      if (!k) return;
      if (t.type === 'checkbox') v = t.checked;
      else if (t.type === 'radio'){ if (!t.checked) return; v = t.value === '__follow' ? null : t.value === '__custom' ? customValue(t.dataset.g) : t.value; }
      else return;
      setP(draft, k, v);
      update();
      fzFlash(k);
      if (k === 'buttons.style') play('btn');
      else if (k === 'motion.elevation' || k === 'motion.speed') play('card');
    });
    on(ROOT, 'input', function(e){
      var t = e.target;
      if (t.dataset.k && (t.type === 'text' || t.tagName === 'TEXTAREA')){ setP(draft, t.dataset.k, t.value); update(); fzFlash(t.dataset.k); }
      else if (t.dataset.link){ var p = t.dataset.link.split('.'); draft.header.links[+p[0]][p[1]] = t.value; update(); }
      else if (t.dataset.wtext){ draft.header.text = t.value; update(); }
      else if (t.id === 'pop-hue' || t.id === 'pop-tone') applyPick(fromPicker(+$('#pop-hue').value, +$('#pop-tone').value, pickDeep()));
    });
    on(ROOT, 'click', function(e){
      var t = e.target.closest ? e.target.closest('button') : null;
      if (!t) return;
      if (t.getAttribute('role') === 'tab'){ selectSection(t.id.slice(4)); return; }
      if (t.dataset.reset){ resetSection(t.dataset.reset); return; }
      if (t.dataset.pick){ if (pick && pick.btn === t) closePicker(true); else { if (pick) closePicker(false); openPicker(t); } return; }
      if (t.dataset.up){ upKey = t.dataset.up; $('#file').value = ''; $('#file').click(); return; }
      if (t.dataset.rm){ setP(draft, t.dataset.rm, 'none'); $('#' + t.dataset.rm.replace('.', '-') + '-e').hidden = true; update(); announce('Picture removed from your draft.'); return; }
      if (t.dataset.wact){ widgetAct(t.dataset.wact, t.dataset.w); return; }
      switch (t.id){
        case 'savebtn': doSave(); break;
        case 'discard': doDiscard(); break;
        case 'toast-undo': doUndo(); break;
        case 'n-edit': showPencil(); break;
        case 'pop-done': closePicker(true); break;
        case 'pop-use': useHex(); break;
        case 'pv-hide': toggleStrip(); break;
        case 'fz-all': fzSet(null); $('#tab-' + ui.section).focus(); break;
      }
    });
    on(ROOT, 'keydown', function(e){
      if (e.key === 'Escape'){
        if (hcOn){ hcHide(); e.preventDefault(); return; }
        var top = layers[layers.length - 1];
        if (top){ e.preventDefault(); top.close(true); return; }
        if (ui.focus){ e.preventDefault(); fzSet(null); }
        return;
      }
      if (e.target.id === 'pop-hex' && e.key === 'Enter'){ e.preventDefault(); useHex(); return; }
      if (e.target.getAttribute && e.target.getAttribute('role') === 'tab'){
        var ids = SECTIONS.map(function(s){ return s.id; }), i = ids.indexOf(e.target.id.slice(4)), n = null;
        if (e.key === 'ArrowDown' || e.key === 'ArrowRight') n = (i + 1) % ids.length;
        else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') n = (i - 1 + ids.length) % ids.length;
        else if (e.key === 'Home') n = 0;
        else if (e.key === 'End') n = ids.length - 1;
        if (n != null){ e.preventDefault(); selectSection(ids[n], true); }
      }
    });
    on(document, 'pointerdown', function(e){
      if (pick && !$('#pop').contains(e.target) && !pick.btn.contains(e.target)) closePicker(false);
    });
    $('#pop').addEventListener('focusout', function(e){
      var to = e.relatedTarget;
      if (pick && to && !$('#pop').contains(to) && to !== pick.btn) closePicker(false);
    });

    /* ---------- Sizes: the site's scale and the phone strip ---------- */
    // Narrow is measured on the editor itself: Chronicle's menu takes part of the window.
    function phone(){ var c = $('#cz'); return !!c && c.clientWidth <= 760; }
    function toggleStrip(){
      ui.strip = !ui.strip;
      $('#preview').hidden = !ui.strip;
      var b = $('#pv-hide'); b.textContent = ui.strip ? 'Hide' : 'Show preview'; b.setAttribute('aria-expanded', String(ui.strip));
      measure();
    }
    function measure(){
      var wrap = $('#site-wrap');
      if (wrap && wrap.clientWidth) wrap.style.setProperty('--k', (wrap.clientWidth / 880).toFixed(4));
      if (ui.focus) fzQueue();
      if (!phone() && !ui.strip) toggleStrip();
      ROOT.style.setProperty('--strip-h', phone() ? $('#pv-col').offsetHeight + 'px' : '0px');
    }

    /* ---------- Click to focus ----------
       Each section owns some parts of the example site. Pointing outlines
       them, a click opens the section and zooms in, Whole site zooms out.
       A changed setting flashes the parts it touched. */
    var FZ = {
      brand:  { dash:['.s-brand', '#d-banner'], city:['.s-brand'], tip:'Your name, logo, welcome and backdrop' },
      header: { all:['.s-hdr'], tip:'The bar across the top' },
      sidebar:{ all:['.s-sb'], tip:'The menu’s colour and top-left corner' },
      nav:    { all:['.s-list'], tip:'How the menu shows where you are' },
      colours:{ all:['.p-link', '.tag', '.chipe', '.czbadge', '.band', '.tev.now', '.av'], tip:'Links, tags, events and badges' },
      type:   { dash:['.d-title .h1', '.c-h'], city:['.e-head', '.e-body p'], tip:'Headings and text' },
      buttons:{ all:['.pbtn'], tip:'Every button' },
      motion: { all:['.pcard'], tip:'Cards and how they lift' }
    };
    // Most specific first: a button inside a card is a button.
    var FZ_HIT = [['.pbtn', 'buttons'], ['.p-link, .tag, .chipe, .czbadge, .band, .tev.now, .av', 'colours'],
      ['.h1, .c-h, .e-head, .e-body p', 'type'], ['.s-bnr, .s-bsub', 'sidebar'], ['.s-brand, #d-banner', 'brand'], ['.s-hdr', 'header'], ['.s-list', 'nav'], ['.s-sb', 'sidebar'],
      ['.pcard', 'motion'], ['.s-page', 'colours']];
    var FZ_OF = { brand:'brand', header:'header', sidebar:'sidebar', nav:'nav', colours:'colours', type:'type', buttons:'buttons', motion:'motion' };
    var fzLock = false, fzLastFlash = {}, fzRaf = 0, fzHovSec = null;
    function fzSite(){ return $('#site-wrap .site'); }
    function fzRects(sec){
      var site = fzSite(), F = FZ[sec], sel = F[ui.page] || F.all, sr = site.getBoundingClientRect(), s = sr.width / 880, out = [];
      if (!s) return out;
      sel.forEach(function(q){
        $$(q, site).forEach(function(el){
          if (el.offsetParent === null || el.closest('.s-page[hidden]')) return;
          var r = el.getBoundingClientRect(); if (!r.width || !r.height) return;
          var x = (r.left - sr.left) / s, y = (r.top - sr.top) / s, w = r.width / s, h = r.height / s;
          var x2 = Math.min(880, x + w), y2 = Math.min(520, y + h); x = Math.max(0, x); y = Math.max(0, y);
          if (x2 - x > 1 && y2 - y > 1) out.push({ x:x, y:y, w:x2 - x, h:y2 - y });
        });
      });
      return out;
    }
    function fzUnion(rs){
      var x = Infinity, y = Infinity, X = -Infinity, Y = -Infinity;
      rs.forEach(function(r){ x = Math.min(x, r.x); y = Math.min(y, r.y); X = Math.max(X, r.x + r.w); Y = Math.max(Y, r.y + r.h); });
      return { x:x, y:y, w:X - x, h:Y - y };
    }
    function fzRectsSVG(rs, pad, rad){
      return rs.map(function(r){
        return '<rect x="' + (r.x - pad).toFixed(1) + '" y="' + (r.y - pad).toFixed(1) + '" width="' + (r.w + pad * 2).toFixed(1) + '" height="' + (r.h + pad * 2).toFixed(1) + '" rx="' + rad + '"/>';
      }).join('');
    }
    function fzHoles(rs, pad){
      return 'M0 0H880V520H0Z' + rs.map(function(r){
        var x = r.x - pad, y = r.y - pad, w = r.w + pad * 2, h = r.h + pad * 2;
        return 'M' + x.toFixed(1) + ' ' + y.toFixed(1) + 'h' + w.toFixed(1) + 'v' + h.toFixed(1) + 'h' + (-w).toFixed(1) + 'Z';
      }).join('');
    }
    function fzK(){ return parseFloat($('#site-wrap').style.getPropertyValue('--k')) || 0.6; }
    // The example zooms only when the parts are small enough to be worth it.
    function fzZoom(rs){
      var site = fzSite(), k = fzK();
      if (!rs || !rs.length){ site.style.transform = ''; return; }
      var u = fzUnion(rs), pad = 22, f = Math.min(880 / (u.w + pad * 2), 520 / (u.h + pad * 2), 1.8);
      if (f < 1.12){ site.style.transform = ''; return; }
      var tx = clamp(440 - (u.x + u.w / 2) * f, 880 - 880 * f, 0), ty = clamp(260 - (u.y + u.h / 2) * f, 520 - 520 * f, 0);
      site.style.transform = 'scale(' + k + ') translate(' + tx.toFixed(1) + 'px, ' + ty.toFixed(1) + 'px) scale(' + f.toFixed(3) + ')';
    }
    // Rects are measured with no zoom applied, so a redraw never chases its own transform.
    function fzMeasure(sec){
      var site = fzSite(), t = site.style.transform, tr = site.style.transition;
      site.style.transition = 'none'; site.style.transform = '';
      var rs = fzRects(sec);
      site.style.transform = t; void site.offsetWidth; site.style.transition = tr;
      return rs;
    }
    function fzDraw(){
      var svg = $('#fz'), sec = ui.focus;
      if (!svg) return;
      if (!sec || ui.zoom !== 'site'){ svg.classList.remove('on'); $('#fz-bar').classList.remove('on'); fzZoom(null); return; }
      var rs = fzMeasure(sec);
      $('.fz-dim', svg).setAttribute('d', fzHoles(rs, 4));
      setHTML($('.fz-ring', svg), fzRectsSVG(rs, 4, 6));
      svg.classList.add('on');
      setText($('#fz-name'), sectionName(sec));
      $('#fz-bar').classList.add('on');
      fzZoom(rs);
    }
    function fzQueue(){ cancelAnimationFrame(fzRaf); fzRaf = requestAnimationFrame(function(){ fzDraw(); if (fzHovSec) fzHover(fzHovSec, true); }); }
    function fzSet(sec){
      ui.focus = sec;
      if (sec && ui.section !== sec){ fzLock = true; selectSection(sec); fzLock = false; }
      fzDraw();
      if (sec) announce('Showing ' + sectionName(sec) + ' in the example site. Press Escape for the whole site.');
    }
    // Opening a section from its tab keeps a focus going, or briefly shows its parts.
    function fzSection(id){
      if (fzLock) return;
      if (ui.focus){ ui.focus = id; fzQueue(); }
      else if (ui.zoom === 'site') fzFlash(id + '.');
    }
    function fzFlash(k){
      if (ui.zoom !== 'site' || reduceAll()) return;
      var secs = k === '*' ? [] : [k === 'colours.sidebar' ? 'sidebar' : FZ_OF[String(k).split('.')[0]]];
      secs.forEach(function(sec){
        if (!sec || Date.now() - (fzLastFlash[sec] || 0) < 900) return;
        fzLastFlash[sec] = Date.now();
        requestAnimationFrame(function(){
          var g = $('#fz .fz-flash'), rs = fzMeasure(sec);
          g.innerHTML = fzRectsSVG(rs, 4, 6);
          clearTimeout(g._t); g._t = setTimeout(function(){ g.innerHTML = ''; }, 1050);
        });
      });
    }
    function fzSecAt(el){
      if (!el || !el.closest) return null;
      for (var i = 0; i < FZ_HIT.length; i++) if (el.closest(FZ_HIT[i][0])) return FZ_HIT[i][1];
      return null;
    }
    function fzHover(sec, keep){
      var g = $('#fz .fz-hov'), tip = $('#fz-tip');
      if (!g) return;
      fzHovSec = sec;
      if (!sec || sec === ui.focus){ g.innerHTML = ''; tip.classList.remove('on'); return; }
      if (!keep) setHTML(tip, '<b>' + esc(sectionName(sec)) + '</b><span>' + esc(FZ[sec].tip) + ' · click to change</span>');
      g.innerHTML = fzRectsSVG(fzMeasure(sec), 4, 6);
      tip.classList.add('on');
    }
    function fzInit(){
      var site = fzSite(), wrap = $('#site-wrap');
      site.insertAdjacentHTML('beforeend', '<svg class="fz" id="fz" viewBox="0 0 880 520" aria-hidden="true"><path class="fz-dim" fill-rule="evenodd" d="M0 0H880V520H0Z"/><g class="fz-ring"></g><g class="fz-hov"></g><g class="fz-flash"></g></svg>');
      wrap.insertAdjacentHTML('beforeend', '<div class="fz-tip" id="fz-tip"></div>');
      wrap.classList.add('fz-ok');
      $('[data-z="site"]').insertAdjacentHTML('beforeend', '<div class="fz-bar" id="fz-bar"><span>Showing <b id="fz-name"></b></span><button type="button" class="czb czb-s czb-sm" id="fz-all">Whole site</button></div>');
      on(wrap, 'pointermove', function(e){
        if (e.pointerType === 'touch' || ui.zoom !== 'site') return;
        var sec = fzSecAt(e.target);
        if (sec !== fzHovSec) fzHover(sec);
        var tip = $('#fz-tip'), wr = wrap.getBoundingClientRect();
        tip.style.transform = 'translate(' + clamp(e.clientX - wr.left + 14, 4, wr.width - tip.offsetWidth - 4).toFixed(0) + 'px, ' + clamp(e.clientY - wr.top + 16, 4, wr.height - tip.offsetHeight - 4).toFixed(0) + 'px)';
      });
      on(wrap, 'pointerleave', function(){ fzHover(null); });
      on(wrap, 'click', function(e){
        if (ui.zoom !== 'site') return;
        var sec = fzSecAt(e.target);
        if (!sec) return;
        e.preventDefault();
        fzHover(null);
        fzSet(sec);
      });
    }

    /* ---------- Start ---------- */
    function init(){
      IMG.dusk = paintDusk();
      ROOT.insertAdjacentHTML('afterbegin', SPRITE);
      var live = document.createElement('div'); live.id = 'live'; live.className = 'sr'; live.setAttribute('aria-live', 'polite'); ROOT.appendChild(live);
      buildUI(); buildPreview();
      $$('.nav-tile').forEach(function(t){
        var inp = $('input', t);
        function playOn(){ t.classList.add('play'); runMotion(); }
        function playOff(){ t.classList.remove('play'); runMotion(); }
        t.addEventListener('pointerenter', playOn); t.addEventListener('pointerleave', playOff);
        inp.addEventListener('focus', function(){ if (inp.matches(':focus-visible')) playOn(); });
        inp.addEventListener('blur', playOff);
      });
      $$('.btn-tile').forEach(function(t){
        var b = $('.pbtn', t), inp = $('input', t);
        t.addEventListener('pointerenter', function(){ b.classList.add('hov'); });
        t.addEventListener('pointerleave', function(){ b.classList.remove('hov', 'prs'); });
        t.addEventListener('pointerdown', function(e){ b.classList.add('prs'); inkBloom(b, e.clientX, e.clientY); });
        t.addEventListener('pointerup', function(){ b.classList.remove('prs'); });
        inp.addEventListener('focus', function(){ if (inp.matches(':focus-visible')) b.classList.add('hov'); });
        inp.addEventListener('blur', function(){ b.classList.remove('hov', 'prs'); });
      });
      var pv = $('#preview');
      pv.addEventListener('pointerdown', function(e){ var b = e.target.closest('.pbtn'); if (b) inkBloom(b, e.clientX, e.clientY); });
      pv.addEventListener('keydown', function(e){ var b = e.target.closest ? e.target.closest('button.pbtn') : null; if (b && (e.key === 'Enter' || e.key === ' ') && !e.repeat){ b.classList.add('prs'); inkBloom(b); } });
      pv.addEventListener('keyup', function(e){ var b = e.target.closest ? e.target.closest('button.pbtn') : null; if (b) b.classList.remove('prs'); });
      pv.addEventListener('focusout', function(e){ var b = e.target.closest ? e.target.closest('button.pbtn') : null; if (b) b.classList.remove('prs'); });
      applyRM(); syncThemeRadio();
      fzInit();
      update(); setZoom('site'); measure();
      if (window.ResizeObserver){ ro = new ResizeObserver(function(){ measure(); runMotion(); }); ro.observe($('#site-wrap')); ro.observe($('#pv-col')); }
      on(window, 'resize', measure);
      if (mqRM && mqRM.addEventListener) on(mqRM, 'change', function(){ applyRM(); update(); });
      // Chronicle's theme toggle flips the dark class; the preview follows it
      // until the owner picks Light or Dark for the preview themselves.
      if (window.MutationObserver){ mo = new MutationObserver(themeChanged); mo.observe(root, { attributes:true, attributeFilter:['class'] }); }
    }

    /* ---------- Device motion and Chronicle's theme ---------- */
    var ro = null, mo = null, lastTheme = pageTheme();
    function applyRM(){ ROOT.classList.toggle('rm', reducedDevice()); }
    function syncThemeRadio(){ var pt = previewTheme(), r = $('#pt-' + pt); if (r && !r.checked) r.checked = true; }
    function themeChanged(){
      var th = pageTheme();
      applyRM();
      if (th === lastTheme) return;
      lastTheme = th; ui.ptheme = null; lookTileTheme = null; update(); syncThemeRadio();
    }

    init();
    return function destroy() {
      alive = false;
      listeners.forEach(function (l) { l[0].removeEventListener(l[1], l[2], l[3]); });
      if (ro) ro.disconnect();
      if (mo) mo.disconnect();
      ringAnims.forEach(function (a) { a.forEach(function (x) { x.cancel(); }); });
      driftStop();
      clearTimeout(toastT); clearTimeout(hcTimer);
      cancelAnimationFrame(fzRaf);
      playTimers.forEach(clearTimeout);
    };
  }

  Chronicle.register('customize-look', {
    init: function (el) {
      var state;
      try { state = JSON.parse(el.getAttribute('data-state') || '{}'); } catch (e) { state = null; }
      if (!state || !state.draft) { console.error('[customize-look] missing data-state'); return; }
      el._czDestroy = mount(el, state, el.getAttribute('data-campaign-id'), el.getAttribute('data-csrf'));
    },
    destroy: function (el) {
      if (el._czDestroy) el._czDestroy();
      el._czDestroy = null;
    }
  });
})();
