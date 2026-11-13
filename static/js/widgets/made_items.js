/**
 * made_items.js -- the Dark painted made-item pictures (window.MadeItems)
 *
 * Each item was built as a solid, lit, photographed and turned into brush
 * strokes offline (tools/made-items). A picture is served as an atlas of four
 * tiles: the item painted in neutral greys, then how much of each pixel takes
 * the recipe's body, trim and detail colour. The browser multiplies the greys
 * by the recipe's colours once per item and colour set and keeps the result as
 * an image URL, so two potions brewed from different herbs look different
 * while every picture still comes from one file.
 *
 * Sizes: "s" (256, lists and tiles), "l" (512, an opened item) and "shelf"
 * (128 on a clear background, shop shelves). Items that move have a strip of
 * 64 frames that plays only where one item is shown large, one at a time,
 * never in a list, and not at all when the viewer asks for less motion or to
 * save data: phones stuttered on a grid of moving pictures.
 *
 * Only an item key and #rrggbb colours ever reach the markup.
 */
window.MadeItems = (function () {
  'use strict';
  var BASE = '/static/img/made/';
  var HEX = /^#[0-9a-f]{6}$/i;
  var LOOP = 2400, FRAMES = 64;

  /* key: [name, body, trim, detail, body grey, trim grey, detail grey, moves].
     The greys are the neutral each role was painted in; a recolour is the
     painted grey times colour / grey. */
  var ITEMS = {
    sword: ["Longsword", '#aab3bd', '#b98a3a', '#3f2a22', 177, 143, 90, 0],
    dagger: ["Dagger", '#8f99a6', '#c9a23f', '#7a3fd0', 151, 162, 97, 0],
    spear: ["Spear", '#9aa4ae', '#7b5331', '#b0302c', 162, 91, 90, 0],
    mace: ["Flanged mace", '#6f7884', '#8a6238', '#2c2622', 119, 105, 90, 0],
    axe: ["War axe", '#93a0ad', '#7b5331', '#5d2f24', 158, 91, 90, 0],
    hammer: ["Smith's hammer", '#6f7884', '#8a6238', '#3c2a22', 119, 105, 90, 0],
    staff: ["Staff", '#5f4630', '#b9c0c8', '#4f8fe0', 90, 191, 133, 0],
    bow: ["Bow", '#8a5a32', '#3b2a24', '#e6dcc8', 100, 90, 200, 0],
    crossbow: ["Crossbow", '#7a4f2c', '#55595f', '#d8d2c2', 90, 90, 200, 0],
    arrows: ["Quiver of arrows", '#6b4a2e', '#b98a3a', '#e6e0d0', 90, 143, 200, 0],
    wand: ["Wand", '#3b2a24', '#c7ccd6', '#3fd0b0', 90, 200, 161, 0],
    flail: ["Flail", '#59606a', '#6e4a2c', '#2c2622', 95, 90, 90, 0],
    shield: ["Kite shield", '#2f4f7a', '#aab3bd', '#d8c070', 90, 177, 190, 0],
    gauntlet: ["Gauntlet", '#9aa4ae', '#b98a3a', '#5a2a2a', 162, 143, 90, 0],
    helm: ["Helm", '#9aa4ae', '#b98a3a', '#9b2c2c', 162, 143, 90, 0],
    breastplate: ["Breastplate", '#aab3bd', '#c9a23f', '#3b7fd4', 177, 162, 116, 0],
    cloak: ["Cloak", '#3c5a44', '#c9a23f', '#c0392b', 90, 162, 96, 0],
    ring: ["Signet ring", '#c9a23f', '#c9a23f', '#1f7a4a', 162, 162, 90, 0],
    crown: ["Crown", '#d4af4a', '#d4af4a', '#a3262e', 175, 175, 90, 0],
    mask: ["Mask", '#e4dccb', '#c9a23f', '#7a1f2a', 200, 162, 90, 0],
    pouch: ["Belt pouch", '#7a5534', '#b98a3a', '#4a3220', 92, 143, 90, 0],
    amulet: ["Amulet", '#c9a23f', '#c9a23f', '#c0392b', 162, 162, 96, 0],
    pickaxe: ["Pickaxe", '#59606a', '#8a6238', '#3a2b24', 95, 105, 90, 0],
    tongs: ["Forge tongs", '#4d535b', '#8a6238', '#ff8a2a', 90, 105, 162, 1],
    mortar: ["Mortar and pestle", '#8f8b84', '#7b5331', '#6f9a3a', 139, 91, 130, 0],
    lantern: ["Lantern", '#3f4349', '#b98a3a', '#ffb347', 90, 143, 189, 1],
    key: ["Key", '#8a6f3a', '#c9a23f', '#c0392b', 113, 162, 96, 0],
    spool: ["Needle and thread", '#a3262e', '#b08a5a', '#c7ccd6', 90, 144, 200, 0],
    anvil: ["Anvil", '#5d646e', '#6e4a2c', '#2c2622', 99, 90, 90, 0],
    compass: ["Compass", '#b98a3a', '#b98a3a', '#b0302c', 143, 143, 90, 0],
    tome: ["Tome", '#5a2a2a', '#b98a3a', '#3fa67a', 90, 143, 130, 0],
    scroll: ["Scroll", '#b98a3a', '#5a3a26', '#a3262e', 143, 90, 90, 0],
    orb: ["Seeing orb", '#4a3f8f', '#b9c0c8', '#b9a6ff', 90, 191, 182, 1],
    candle: ["Ritual candle", '#e8dcc0', '#8a6f3a', '#7fd0ff', 200, 113, 189, 1],
    runestone: ["Runestone", '#6a6e72', '#8a6238', '#5dd6ff', 109, 105, 182, 1],
    hourglass: ["Hourglass", '#9fc7c9', '#6e4a2c', '#e0b04a', 187, 90, 179, 0],
    censer: ["Censer", '#b98a3a', '#d4af4a', '#ff7a2a', 143, 175, 153, 1],
    crystal: ["Crystal cluster", '#7a4fd0', '#55504a', '#d08ae0', 107, 90, 169, 0],
    bread: ["Loaf", '#b5772e', '#8a6238', '#ecd9a0', 129, 105, 200, 0],
    tankard: ["Tankard", '#8a5a32', '#55595f', '#f1e6c8', 100, 90, 200, 0],
    pie: ["Pie", '#c98f3c', '#7a8088', '#7a1f3a', 151, 127, 90, 1],
    roast: ["Roast leg", '#9a4f24', '#e6dcc8', '#4f7a2a', 97, 200, 100, 0],
    stew: ["Hot meal", '#b5622a', '#7a5534', '#d8b45a', 116, 92, 181, 1],
    bottle: ["Bottle", '#2f5a3c', '#b08a5a', '#a3262e', 90, 144, 90, 0],
    ingot: ["Ingots", '#c9a23f', '#c9a23f', '#c9a23f', 162, 162, 162, 0],
    coins: ["Coins", '#c9a23f', '#d8d8de', '#b0703c', 162, 200, 125, 0],
    chest: ["Chest", '#6e4a2c', '#55595f', '#c9a23f', 90, 90, 162, 0],
    lute: ["Lute", '#b5772e', '#5a3a26', '#e6dcc8', 129, 90, 200, 0],
    fang: ["Fang", '#e8e0cc', '#8fe04a', '#6a2a30', 200, 183, 90, 1],
    claw: ["Talon", '#2c2a2e', '#5a6b3a', '#b0703c', 90, 96, 125, 0],
    horn: ["Horn", '#7a6a58', '#8a6f3a', '#ffcf5a', 109, 113, 200, 1],
    scale: ["Scale", '#2f7a6a', '#1f4f5a', '#ff8a3a', 98, 90, 164, 1],
    feather: ["Feather", '#1f3f8a', '#e8e0cc', '#3fd0b0', 90, 200, 161, 0],
    eye: ["Watching eye", '#e6dccb', '#7a2a3a', '#d0a020', 200, 90, 160, 0],
    core: ["Elemental core", '#d0401f', '#4a4440', '#ffb347', 103, 90, 189, 1],
    wisp: ["Bound wisp", '#7fd6e6', '#8a8f98', '#e6ffff', 190, 143, 200, 1],
    heart: ["Crystal heart", '#5e1a24', '#3f141c', '#ff6a8a', 90, 90, 154, 1],
    tentacle: ["Tendril", '#4a3a6a', '#c9b8d8', '#5dff9a', 90, 193, 195, 1],
    carapace: ["Carapace plate", '#3a2f4f', '#c9b890', '#3fd0e0', 90, 185, 166, 0],
    knot: ["Nameless thing", '#8a8f98', '#5a4a7a', '#b07bff', 143, 90, 154, 1],
    pelt: ["Shifting pelt", '#b0703c', '#6a4a32', '#2c1f1a', 125, 90, 90, 0],
    skull: ["Horned skull", '#ddd3bd', '#6a5a48', '#5dff9a', 200, 93, 195, 1],
    meat: ["Cuts of meat", '#8e2a2a', '#b9a583', '#5d7a3a', 90, 167, 106, 0],
    hand: ["Skeletal hand", '#b7a888', '#c9a23f', '#7dffc0', 169, 162, 200, 1],
    geode: ["Stone eye", '#5a5148', '#2f7f96', '#8fe8ff', 90, 106, 200, 1],
    unicorn: ["Unicorn horn", '#f1ece2', '#c9d0d8', '#ffe9a8', 200, 200, 200, 1],
    starfrag: ["Star fragment", '#4a4e58', '#5a4a3c', '#a8d8ff', 90, 90, 200, 1],
    ink: ["Vial of bubbling ink", '#cfdcd8', '#8a5a34', '#4a35c8', 200, 100, 90, 1],
    conch: ["Singing shell", '#e2c9a6', '#f2a79c', '#8fe0ff', 200, 188, 200, 1],
    mandrake: ["Mandrake root", '#8a6a48', '#b8a47a', '#3f6a34', 112, 165, 90, 0],
    egg: ["Dragon egg", '#3f5a4a', '#c2a25a', '#ff9a3a', 90, 163, 173, 1],
    wing: ["Pixie wing", '#bfe6ff', '#3a2f4a', '#ffe08a', 200, 90, 200, 1],
    potion: ["Healing potion", '#d8e4e0', '#5e1616', '#c8182e', 200, 90, 90, 1],
    ore: ["Lump of ore", '#5c5a58', '#e2ded6', '#d4a23f', 90, 200, 166, 0],
    bolt: ["Bolt of cloth", '#1f6a6a', '#c9a23f', '#8a6238', 90, 162, 105, 0],
    herbs: ["Bundle of herbs", '#6f8a4a', '#b8a47a', '#8a6fc0', 123, 165, 128, 0],
    boots: ["Boots", '#6b4a2e', '#b98a3a', '#3a2a20', 90, 143, 90, 0],
    torch: ["Burning torch", '#6e4a2c', '#3f4349', '#ff9a3a', 90, 90, 173, 1],
    bomb: ["Bomb", '#6a6c72', '#b8a47a', '#ffb347', 108, 165, 189, 1],
    gems: ["Cut gems", '#1f9a5a', '#35204a', '#c0283c', 110, 90, 90, 0]
  };

  /* The six shapes made items were drawn as before, and the painted item each
     becomes; and back, for the illustrated and lifelike drawings. */
  var FROM_SHAPE = {potion: 'bottle', vial: 'ink', blade: 'sword', shield: 'shield', ring: 'ring', bundle: 'pouch'};
  var TO_SHAPE = {
    sword: 'blade', dagger: 'blade', spear: 'blade', axe: 'blade', bottle: 'potion', potion: 'potion', ink: 'vial', shield: 'shield',
    ring: 'ring', amulet: 'ring', crown: 'ring'
  };

  /* The shop room's item icons that have a painted item to stand in for them
     on a shelf. Icons with none keep their flat drawing. */
  var FROM_ICON = {
    hammer: 'hammer', 'shield-halved': 'shield', vest: 'breastplate', flask: 'bottle', vial: 'ink', 'bottle-droplet': 'bottle',
    'mortar-pestle': 'mortar', ring: 'ring', gem: 'crystal', crown: 'crown', eye: 'eye', key: 'key', mask: 'mask', compass: 'compass',
    scroll: 'scroll', skull: 'skull', 'wand-sparkles': 'wand', book: 'tome', 'book-open': 'tome', 'bag-shopping': 'pouch',
    hourglass: 'hourglass', feather: 'feather', lightbulb: 'lantern', 'bread-slice': 'bread', sword: 'sword', dagger: 'dagger',
    axe: 'axe', bow: 'bow', 'beer-mug': 'tankard', tentacle: 'tentacle', shell: 'conch', star: 'starfrag', 'boxes-stacked': 'chest',
    fire: 'torch', leaf: 'herbs', 'spa': 'herbs'
  };

  function has(k) { return typeof k === 'string' && Object.prototype.hasOwnProperty.call(ITEMS, k); }
  /* key is the painted item a look's shape names, old shapes included. */
  function key(shape) { return has(shape) ? shape : (FROM_SHAPE[shape] || null); }
  function oldShape(shape) { return has(shape) ? (TO_SHAPE[shape] || 'bundle') : shape; }
  function colours(k, spec) {
    var d = ITEMS[k], s = spec || {};
    function c(v, i) { return typeof v === 'string' && HEX.test(v) ? v.toLowerCase() : d[i]; }
    return [c(s.body, 1), c(s.trim, 2), c(s.detail, 3)];
  }

  /* html is the picture's markup: an image that fills in once its colours
     are worked out. o.move adds the moving strip over it where the item moves. */
  function html(spec, size, o) {
    o = o || {};
    var k = key(spec && spec.shape);
    if (!k) return '';
    var z = o.zoom || (size > 128 ? 'l' : 's'), c = colours(k, spec).join(',');
    var px = size || 64, lab = o.label ? ' alt="' + String(o.label).replace(/[&<>"]/g, '') + '"' : ' alt="" aria-hidden="true"';
    var img = '<img class="mi-pic' + (o.cls ? ' ' + o.cls : '') + '" data-mi="' + k + '" data-mi-z="' + z + '" data-mi-c="' + c + '" width="' + px + '" height="' + px + '"' + lab + ' decoding="async">';
    if (!o.move || !ITEMS[k][7] || !motionOK()) return img;
    return '<span class="mi-move">' + img + '<canvas data-mi-move="' + k + '" data-mi-c="' + c + '" width="256" height="256" aria-hidden="true"></canvas></span>';
  }
  /* For a shelf inside an SVG scene: an <image> that fills in the same way. */
  function svgImage(k, x, y, w, spec) {
    if (!has(k)) return '';
    return '<image data-mi="' + k + '" data-mi-z="shelf" data-mi-c="' + colours(k, spec).join(',') + '" x="' + x.toFixed(1) + '" y="' + y.toFixed(1) + '" width="' + w.toFixed(1) + '" height="' + w.toFixed(1) + '" preserveAspectRatio="xMidYMax meet"/>';
  }

  // ---------- Recolouring ----------

  var atlases = {}, made = {};
  function atlas(url) {
    if (!atlases[url]) atlases[url] = new Promise(function (ok, no) {
      var im = new Image();
      im.onload = function () { ok(im); };
      im.onerror = function () { delete atlases[url]; no(new Error('made item picture did not load')); };
      im.src = url;
    });
    return atlases[url];
  }
  function rgb(h) { var n = parseInt(h.slice(1), 16); return [n >> 16, (n >> 8) & 255, n & 255]; }
  /* recolour multiplies each tile of neutral greys by its colours. src is the
     atlas, (sx, sy) the top left of one picture's four tiles, t their size. */
  function recolour(ctx, src, sx, sy, t, k, c, dx, dy) {
    var w = document.createElement('canvas'); w.width = w.height = t * 2;
    var wc = w.getContext('2d', {willReadFrequently: true});
    wc.drawImage(src, sx, sy, t * 2, t * 2, 0, 0, t * 2, t * 2);
    var px = wc.getImageData(0, 0, t * 2, t * 2).data, out = ctx.createImageData(t, t), o = out.data, d = ITEMS[k];
    var f = [0, 1, 2].map(function (i) { var v = rgb(c[i]), g = d[4 + i]; return [v[0] / g - 1, v[1] / g - 1, v[2] / g - 1]; });
    var row = t * 8;
    for (var y = 0; y < t; y++) for (var x = 0; x < t; x++) {
      var n = y * row + x * 4, b = n + t * 4, tr = n + t * row, de = tr + t * 4, j = (y * t + x) * 4;
      var mb = px[b] / 255, mt = px[tr] / 255, md = px[de] / 255;
      for (var ch = 0; ch < 3; ch++) o[j + ch] = px[n + ch] * (1 + mb * f[0][ch] + mt * f[1][ch] + md * f[2][ch]);
      o[j + 3] = px[n + 3];
    }
    ctx.putImageData(out, dx, dy);
  }
  /* picture resolves to an image URL for one item, size and colour set. */
  function picture(k, z, c) {
    var id = z + '/' + k + '/' + c.join(',');
    if (made[id]) return made[id];
    made[id] = atlas(BASE + z + '/' + k + '.webp').then(function (im) {
      var t = im.naturalWidth / 2, cv = document.createElement('canvas'); cv.width = cv.height = t;
      recolour(cv.getContext('2d'), im, 0, 0, t, k, c, 0, 0);
      return new Promise(function (ok) { cv.toBlob(function (b) { ok(b ? URL.createObjectURL(b) : cv.toDataURL()); }); });
    });
    made[id].catch(function () { delete made[id]; });
    return made[id];
  }
  var ready = {};
  function fill(el) {
    var k = el.getAttribute('data-mi'), z = el.getAttribute('data-mi-z'), c = (el.getAttribute('data-mi-c') || '').split(',');
    if (!has(k) || ['s', 'l', 'shelf'].indexOf(z) < 0 || c.length !== 3 || !c.every(function (h) { return HEX.test(h); })) return;
    var id = z + '/' + k + '/' + c.join(','), svg = el.namespaceURI === 'http://www.w3.org/2000/svg';
    function put(u) { if (svg) el.setAttribute('href', u); else el.src = u; el.classList.add('mi-in'); }
    if (el.getAttribute('data-mi-at') === id) return;
    el.setAttribute('data-mi-at', id);
    if (ready[id]) { put(ready[id]); return; }
    picture(k, z, c).then(function (u) { ready[id] = u; if (el.getAttribute('data-mi-at') === id) put(u); }, function () { /* the box stays empty, as before */ });
  }
  function scan(root) {
    if (!root || !root.querySelectorAll) return;
    if (root.matches && root.matches('[data-mi]')) fill(root);
    root.querySelectorAll('[data-mi]').forEach(fill);
    root.querySelectorAll('canvas[data-mi-move]').forEach(watch);
    if (root.matches && root.matches('canvas[data-mi-move]')) watch(root);
  }

  // ---------- Moving items ----------

  function motionOK() {
    try {
      if (window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches) return false;
      if (navigator.connection && navigator.connection.saveData) return false;
    } catch (e) { /* assume it may move */ }
    return true;
  }
  var strips = {}, current = null, seen = null;
  /* strip recolours every frame once into one canvas of 256 tiles. Each
     frame in the file is the neutral picture beside its three masks at half
     size, which keeps the file inside what phones decode comfortably. */
  function strip(k, c) {
    var id = k + '/' + c.join(',');
    if (!strips[id]) strips[id] = atlas(BASE + 'move/' + k + '.webp').then(function (im) {
      var t = 256, h = t / 2, cols = 8, cv = document.createElement('canvas'), x = cv.getContext('2d');
      cv.width = cols * t; cv.height = Math.ceil(FRAMES / cols) * t;
      var w = document.createElement('canvas'); w.width = t * 2; w.height = t;
      var wc = w.getContext('2d', {willReadFrequently: true}), d = ITEMS[k], out = x.createImageData(t, t), o = out.data;
      var f = [0, 1, 2].map(function (i) { var v = rgb(c[i]), g = d[4 + i]; return [v[0] / g - 1, v[1] / g - 1, v[2] / g - 1]; });
      for (var i = 0; i < FRAMES; i++) {
        var fx = (i % cols), fy = Math.floor(i / cols);
        wc.drawImage(im, fx * t * 2, fy * t, t * 2, t, 0, 0, t * 2, t);
        var px = wc.getImageData(0, 0, t * 2, t).data, row = t * 8;
        for (var y = 0; y < t; y++) for (var xx = 0; xx < t; xx++) {
          var n = y * row + xx * 4, my = y >> 1, mx = xx >> 1;
          var mb = px[my * row + (t + mx) * 4] / 255, mt = px[my * row + (t + h + mx) * 4] / 255, md = px[(h + my) * row + (t + mx) * 4] / 255, j = (y * t + xx) * 4;
          for (var ch = 0; ch < 3; ch++) o[j + ch] = px[n + ch] * (1 + mb * f[0][ch] + mt * f[1][ch] + md * f[2][ch]);
          o[j + 3] = 255;
        }
        x.putImageData(out, fx * t, fy * t);
      }
      delete atlases[BASE + 'move/' + k + '.webp'];
      return cv;
    });
    strips[id].catch(function () { delete strips[id]; });
    return strips[id];
  }
  function watch(cv) {
    if (cv.__mi) return;
    cv.__mi = true;
    if (!('IntersectionObserver' in window)) return;
    seen = seen || new IntersectionObserver(function (es) {
      es.forEach(function (e) { if (e.isIntersecting) start(e.target); else if (current && current.cv === e.target) stop(); });
    });
    seen.observe(cv);
  }
  /* Only one item moves at a time: starting one stops the last. */
  function start(cv) {
    if (current && current.cv === cv) return;
    stop();
    var k = cv.getAttribute('data-mi-move'), c = (cv.getAttribute('data-mi-c') || '').split(',');
    if (!has(k) || c.length !== 3 || !c.every(function (h) { return HEX.test(h); })) return;
    var me = current = {cv: cv, raf: 0};
    strip(k, c).then(function (s) {
      if (current !== me || !cv.isConnected) return;
      var x = cv.getContext('2d'), t0 = performance.now();
      cv.classList.add('mi-on');
      (function tick(now) {
        if (current !== me) return;
        me.raf = requestAnimationFrame(tick);
        if (document.hidden) return;
        // Drawn every display frame, each picture blended into the next, so
        // 20 pictures a second move without steps.
        var p = ((now - t0) % LOOP) / LOOP * FRAMES, i = Math.floor(p), a = p - i, j = (i + 1) % FRAMES;
        x.globalAlpha = 1; x.drawImage(s, (i % 8) * 256, Math.floor(i / 8) * 256, 256, 256, 0, 0, 256, 256);
        x.globalAlpha = a; x.drawImage(s, (j % 8) * 256, Math.floor(j / 8) * 256, 256, 256, 0, 0, 256, 256);
      })(t0);
    }, function () { /* the still stays */ });
  }
  function stop() {
    if (!current) return;
    cancelAnimationFrame(current.raf);
    current.cv.classList.remove('mi-on');
    current = null;
  }

  if (typeof MutationObserver !== 'undefined' && typeof document !== 'undefined') {
    var boot = function () {
      scan(document.body);
      new MutationObserver(function (ms) {
        ms.forEach(function (m) {
          if (m.type === 'attributes') { fill(m.target); return; }
          m.addedNodes.forEach(function (n) { if (n.nodeType === 1) scan(n); });
          m.removedNodes.forEach(function (n) { if (current && n.nodeType === 1 && (n === current.cv || (n.contains && n.contains(current.cv)))) stop(); });
        });
      }).observe(document.body, {childList: true, subtree: true, attributes: true, attributeFilter: ['data-mi-c', 'data-mi']});
    };
    if (document.body) boot(); else document.addEventListener('DOMContentLoaded', boot);
  }

  return {has: has, key: key, forIcon: function (ic) { var k = FROM_ICON[ic]; return has(k) ? k : null; }, oldShape: oldShape, html: html, svgImage: svgImage, scan: scan, keys: function () { return Object.keys(ITEMS); },
    item: function (k) { var d = ITEMS[k]; return d ? {name: d[0], body: d[1], trim: d[2], detail: d[3], moves: !!d[7]} : null; }};
})();
