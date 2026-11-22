/**
 * hero_borders.js — puts a border from Chronicle's library on a hero creator
 * card and gives it the entry's colour.
 *
 * A border is a nine-slice picture (styles: hero_borders.css). Where a border
 * has parts that take the entry's colour (carved lines, runes, glowing cracks,
 * a vine), its picture leaves them dark and a second picture, the mask, holds
 * how much colour each point takes. The two are put together here on a canvas,
 * once for each border and colour on the page, and the result replaces the
 * plain picture on every card that wears it. Scales are dyed whole instead and
 * keep their brass bosses. Until a coloured picture is ready, and whenever
 * making one fails, the card shows the plain picture.
 *
 * window.ChronicleHeroBorders(lib, root) returns { attrs, destroy }.
 *   lib   the library as the page hands it over, by key:
 *         { img, mask, over, corner, cornerMask, ground, dye, peak, gain, glow }
 *   root  the element the cards are drawn inside
 *
 * Loaded before the scripts that use it.
 */
(function () {
  'use strict';

  var HEX = /^#[0-9a-f]{6}$/i;

  function rgbOf(hex) { return [parseInt(hex.slice(1, 3), 16), parseInt(hex.slice(3, 5), 16), parseInt(hex.slice(5, 7), 16)]; }
  // A picture's address as it may be written into a style attribute: only the
  // characters a served path or a made picture's address uses.
  var SAFE = /^[\w\-./?=%:]+$/;
  function cssUrl(u) { return 'url(&quot;' + u + '&quot;)'; }

  function load(src) {
    return new Promise(function (ok, fail) {
      var im = new Image();
      im.onload = function () { ok(im); };
      im.onerror = function () { fail(new Error('picture not loaded')); };
      im.src = src;
    });
  }

  function sheet(w, h) { var c = document.createElement('canvas'); c.width = w; c.height = h; return c; }

  // A lit part of a modelled border glows: its renderer passed the colour
  // through a film tone curve (ACES), which lifts a strong colour's weaker
  // channels toward white. The same curve here gives the colour that part
  // would have been drawn in, for any entry; strength is how hard it is lit.
  // The result is scaled so white stays white.
  var FILM_IN = [0.59719, 0.35458, 0.04823, 0.07600, 0.90834, 0.01566, 0.02840, 0.13383, 0.83777];
  var FILM_OUT = [1.60475, -0.53108, -0.07367, -0.10208, 1.10813, -0.00605, -0.00327, -0.07276, 1.07602];
  function mix(m, v) { return [m[0] * v[0] + m[1] * v[1] + m[2] * v[2], m[3] * v[0] + m[4] * v[1] + m[5] * v[2], m[6] * v[0] + m[7] * v[1] + m[8] * v[2]]; }
  function lit(rgb, strength) {
    function shown(c) {
      var light = c.map(function (v) { v /= 255; return (v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)) * strength / 0.6; });
      var film = mix(FILM_IN, light).map(function (v) { return (v * (v + 0.0245786) - 0.000090537) / (v * (0.983729 * v + 0.4329510) + 0.238081); });
      return mix(FILM_OUT, film).map(function (v) { v = Math.min(1, Math.max(0, v)); return v <= 0.0031308 ? v * 12.92 : 1.055 * Math.pow(v, 1 / 2.4) - 0.055; });
    }
    var white = shown([255, 255, 255]), got = shown(rgb);
    return [255 * got[0] / white[0], 255 * got[1] / white[1], 255 * got[2] / white[2]];
  }

  // Adds the colour to the picture wherever the mask says, and by how much.
  // peak sets how bright the colour's strongest channel is drawn (0 leaves the
  // colour as it is), so a dark entry colour still lights a rune; gain is the
  // mask's own scale; glow, where a border has it, is the strength its lit
  // parts glow at.
  function inlay(base, mask, colour, peak, gain, glow) {
    var w = base.naturalWidth, h = base.naturalHeight, c = sheet(w, h), x = c.getContext('2d', { willReadFrequently: true });
    x.drawImage(mask, 0, 0, w, h);
    var m = x.getImageData(0, 0, w, h).data;
    x.clearRect(0, 0, w, h);
    x.drawImage(base, 0, 0);
    var img = x.getImageData(0, 0, w, h), d = img.data, rgb = rgbOf(colour);
    if (glow) rgb = lit(rgb, glow);
    var k = (peak ? peak * 255 / Math.max(rgb[0], rgb[1], rgb[2], 13) : 1) * (gain || 1) / 255;
    var r = rgb[0] * k, g = rgb[1] * k, b = rgb[2] * k;
    for (var i = 0; i < d.length; i += 4) {
      var a = m[i];
      if (a) { d[i] += a * r; d[i + 1] += a * g; d[i + 2] += a * b; }
    }
    x.putImageData(img, 0, 0);
    return c;
  }

  // Dyes a picture painted without colour of its own, keeping its light and
  // shade, then lays the parts that stay as they are (over) back on top.
  function dye(base, over, colour) {
    var w = base.naturalWidth, h = base.naturalHeight, c = sheet(w, h), x = c.getContext('2d');
    x.drawImage(base, 0, 0);
    x.globalCompositeOperation = 'color'; x.fillStyle = colour; x.fillRect(0, 0, w, h);
    x.globalCompositeOperation = 'soft-light'; x.globalAlpha = 0.25; x.fillRect(0, 0, w, h); x.globalAlpha = 1;
    x.globalCompositeOperation = 'multiply'; x.fillStyle = '#a9a9a9'; x.fillRect(0, 0, w, h);
    x.globalCompositeOperation = 'destination-in'; x.drawImage(base, 0, 0);
    x.globalCompositeOperation = 'source-over';
    if (over) x.drawImage(over, 0, 0, w, h);
    return c;
  }

  function toUrl(c) {
    return new Promise(function (ok, fail) {
      c.toBlob(function (b) { if (b) ok(URL.createObjectURL(b)); else fail(new Error('picture not made')); });
    });
  }

  function create(lib, root) {
    lib = lib || {};
    var made = {}, asked = {}, urls = [], queue = [], busy = false, gone = false;

    // The pictures of one border in one colour: { img, corner }, each present
    // only when that part takes colour. Resolves to null when nothing does.
    function colour(b, col) {
      if (b.dye) return load(b.img).then(function (base) { return (b.over ? load(b.over) : Promise.resolve(null)).then(function (ov) { return toUrl(dye(base, ov, col)); }); }).then(function (u) { return { img: u }; });
      var jobs = [];
      if (b.mask) jobs.push(Promise.all([load(b.img), load(b.mask)]).then(function (p) { return toUrl(inlay(p[0], p[1], col, b.peak, b.gain, b.glow)); }).then(function (u) { return ['img', u]; }));
      if (b.cornerMask) jobs.push(Promise.all([load(b.corner), load(b.cornerMask)]).then(function (p) { return toUrl(inlay(p[0], p[1], col, b.peak, b.gain, b.glow)); }).then(function (u) { return ['corner', u]; }));
      if (!jobs.length) return Promise.resolve(null);
      return Promise.all(jobs).then(function (parts) { var o = {}; parts.forEach(function (p) { o[p[0]] = p[1]; }); return o; });
    }

    // One picture is made at a time, so a step with many borders never holds
    // the page up for more than one of them.
    function next() {
      if (busy || gone || !queue.length) return;
      busy = true;
      var job = queue.shift(), b = lib[job.key];
      colour(b, job.col).then(function (out) {
        if (!out) return;
        if (gone) { Object.keys(out).forEach(function (k) { URL.revokeObjectURL(out[k]); }); return; }
        Object.keys(out).forEach(function (k) { urls.push(out[k]); });
        made[job.id] = out;
        wear(job.key, job.col, out);
      }).catch(function () { /* the plain picture stays */ }).then(function () {
        busy = false;
        if (!queue.length) return;
        if (window.requestIdleCallback) window.requestIdleCallback(next, { timeout: 200 }); else window.setTimeout(next, 16);
      });
    }

    // Puts a finished picture on every card already on the page that wears it.
    function wear(key, col, out) {
      if (!root) return;
      Array.prototype.forEach.call(root.querySelectorAll('[data-trim="' + key + '"]'), function (el) {
        if (String(el.style.getPropertyValue('--k')).trim().toLowerCase() !== col) return;
        if (out.img) el.style.setProperty('--trim-img', 'url("' + out.img + '")');
        if (out.corner) el.style.setProperty('--trim-corner', 'url("' + out.corner + '")');
      });
    }

    // The attributes that make a card wear its border: look is { border, colour }
    // from the plan. An unknown border or a colour that is not six hex digits
    // leaves the card plain.
    function attrs(look) {
      var b = look && lib[look.border], col = look && String(look.colour || '').toLowerCase();
      if (!b || !HEX.test(col) || !/^[a-z0-9-]+$/.test(look.border) || !SAFE.test(b.img) || (b.corner && !SAFE.test(b.corner)) || (b.ground && !SAFE.test(b.ground))) return '';
      var id = look.border + ' ' + col, out = made[id] || {};
      if ((b.dye || b.mask || b.cornerMask) && !asked[id]) {
        asked[id] = true;
        queue.push({ id: id, key: look.border, col: col });
        next();
      }
      var style = '--k:' + col + ';--trim-img:' + cssUrl(out.img || b.img);
      if (b.corner) style += ';--trim-corner:' + cssUrl(out.corner || b.corner);
      if (b.ground) style += ';--trim-ground:' + cssUrl(b.ground);
      return ' data-trim="' + look.border + '" style="' + style + '"';
    }

    function destroy() {
      gone = true; queue.length = 0;
      urls.forEach(function (u) { URL.revokeObjectURL(u); });
      urls.length = 0;
    }

    return { attrs: attrs, destroy: destroy };
  }

  window.ChronicleHeroBorders = create;
})();
