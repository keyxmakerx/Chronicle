/**
 * cutscene_engine.js — plays a Cut scene: a full-screen takeover drawn with
 * code, in three independent layers (the background, animations on top, and
 * words), described by the scene JSON the cutscenes plugin stores. How a
 * layer arrives and leaves, and where words sit, are choices of their own,
 * so any transition combines with any background, animation or text style.
 *
 * Standalone on purpose: no dependencies and no Chronicle globals, so the
 * Foundry module vendors this file unchanged and both play the same scene the
 * same way. ES5 so it runs anywhere either app does.
 *
 *   var run = ChronicleCutscene.play(host, scene, { reducedMotion, onClose, label });
 *   run.stop();
 *
 * onClose(why) gets 'tap', 'auto', 'escape' or 'stopped'. With contained:
 * true the scene fills `host` instead of the window and leaves focus and the
 * page alone: the Director's preview and the sign-off mockups use it.
 * An auto ending closes `after` seconds once the last layer has arrived.
 *
 * Everything drawn per frame goes to one canvas; soft shapes are drawn small
 * on offscreen canvases and scaled up, so a phone holds the frame rate. The
 * words are DOM text set with textContent only: a scene's words are never
 * markup.
 */
(function (root) {
  'use strict';

  // ---------- The vocabulary ----------

  // The server refuses anything outside these lists; a Go test greps this file
  // for each one so the two cannot drift.
  var KINDS = {
    background: ['fall-asleep', 'black', 'fade-to-black', 'none'],
    overlays: ['eye', 'embers', 'letterbox'],
    textStyles: ['whisper', 'title', 'plain', 'caption'],
    endings: ['tap', 'auto']
  };

  // How the background arrives, apart from what it is. Today's backdrop is
  // black, and its three arrivals keep their old kind names on the wire
  // (ARRIVAL_KIND), so a player that predates the split plays a new scene
  // the same way. A blank text arrival or leaving is the style's own, and a
  // blank layout is the middle, so a scene written before these existed
  // normalises to exactly what it was.
  var ARRIVALS = ['cut', 'fade', 'fall-asleep'];
  var ARRIVAL_KIND = { cut: 'black', fade: 'fade-to-black', 'fall-asleep': 'fall-asleep' };
  var TEXT = {
    layouts: ['middle', 'corner'],
    arrivals: ['fade', 'rise', 'settle', 'slide'],
    leavings: ['fade', 'sink', 'slide']
  };

  // Looks are params of a kind, so a scene that names none plays the default
  // and an unknown one falls back to it rather than drawing nothing.
  var LOOKS = {
    'fall-asleep': [
      { id: 'ink', name: 'Ink in water', about: 'Darkness bleeds in from the edges like ink spreading through water, in curling soft tendrils that meet in the middle.' },
      { id: 'eyelids', name: 'Heavy eyelids', about: 'Soft dark patches breathe in from the edges while eyelids droop and flutter open twice before they give in.' },
      { id: 'drift', name: 'The page drifts off', about: 'The screen itself blurs, loses its colour and sinks, while darkness closes in like tunnel vision and soft dark spots float across it.' }
    ],
    eye: [
      { id: 'rift', name: 'Burning seam', about: 'A line of green fire burns across the dark and tears open into a ragged eye with a round pupil that narrows onto you; as it opens, emerald fire blooms around it and a haze of heat veils the eye.', pupil: 'round' },
      { id: 'almond', name: 'Lids part', about: 'A thin seam of light cracks, hesitates, then the lids part on a burning iris with a slit pupil; as they open, emerald fire blooms around the eye and a haze of heat veils it.', pupil: 'slit' },
      { id: 'gather', name: 'Sparks gather', about: 'Sparks drift out of the dark into a ring of green fire; lines of light draw the eye around it, the pupil splits open and emerald fire blooms around it under a haze of heat.', pupil: 'slit' }
    ]
  };
  var DEFAULT_LOOK = { 'fall-asleep': 'ink', eye: 'rift' };
  // Any eye can take either pupil; one that names none draws its look's own
  // (the `pupil` of its LOOKS entry).
  var PUPILS = [
    { id: 'round', name: 'Round', about: 'A round black pupil that widens and narrows.' },
    { id: 'slit', name: 'Dragon slit', about: 'A tall reptilian slit that narrows and flares with the fire, in a scaled, veined iris.' }
  ];
  var DEFAULT_EYE = '#3dff7a';
  var DEFAULT_EMBER = '#ff9a3c';

  // A trapped screen is hostile: the close mark and Esc work from this many
  // seconds in, whatever the scene says.
  var ESCAPE_AFTER = 2;
  // How long a line of words takes to arrive and to leave.
  var TEXT_IN = 1.4;
  var TEXT_OUT = 1.1;

  // ---------- Normalising a scene ----------

  // Times are clamped and kept to tenths of a second, as the server stores them.
  function num(v, lo, hi, def) {
    var n = typeof v === 'number' ? v : parseFloat(v);
    if (!isFinite(n)) return def;
    return Math.round(Math.min(hi, Math.max(lo, n)) * 10) / 10;
  }
  function inList(v, list) { return list.indexOf(v) >= 0; }
  function colour(v, def) { return typeof v === 'string' && /^#[0-9a-fA-F]{6}$/.test(v) ? v.toLowerCase() : def; }
  function pupil(v, lk) {
    for (var i = 0; i < PUPILS.length; i++) if (PUPILS[i].id === v) return v;
    var all = LOOKS.eye;
    for (var j = 0; j < all.length; j++) if (all[j].id === lk) return all[j].pupil;
    return 'round';
  }
  function look(kind, v) {
    var all = LOOKS[kind] || [];
    for (var i = 0; i < all.length; i++) if (all[i].id === v) return v;
    return DEFAULT_LOOK[kind] || '';
  }

  // normalise returns a scene the player can trust: numbers clamped, colours
  // checked, unknown overlay kinds dropped, an unknown background drawn as
  // none and an unknown text style as plain, so an older client never fails
  // on a newer scene. The server clamps the same ranges.
  function normalise(raw) {
    var s = raw && typeof raw === 'object' ? raw : {};
    var bg = s.background && typeof s.background === 'object' ? s.background : {};
    var bgKind = inList(bg.kind, KINDS.background) ? bg.kind : 'none';
    // A black backdrop named with a separate arrival is stored under the
    // arrival's old kind name, so the two spellings play the same.
    if (bgKind !== 'none' && inList(bg['in'], ARRIVALS)) bgKind = ARRIVAL_KIND[bg['in']];
    var bgParams = bg.params && typeof bg.params === 'object' ? bg.params : {};
    var out = {
      id: String(s.id == null ? '' : s.id),
      name: String(s.name == null ? '' : s.name),
      background: { kind: bgKind, seconds: num(bg.seconds, 0.5, 60, 7), params: {} },
      overlays: [],
      text: [],
      ending: { kind: 'tap', after: 0 }
    };
    if (bgKind === 'fall-asleep') out.background.params.look = look('fall-asleep', bgParams.look);
    (Array.isArray(s.overlays) ? s.overlays : []).slice(0, 8).forEach(function (o) {
      if (!o || !inList(o.kind, KINDS.overlays)) return;
      var p = o.params && typeof o.params === 'object' ? o.params : {};
      var n = { kind: o.kind, at: num(o.at, 0, 120, 0), seconds: num(o.seconds, 0.5, 60, 3), params: {} };
      // Film bars are always black; only the eye and embers take a colour.
      if (o.kind !== 'letterbox') n.params.color = colour(p.color, o.kind === 'eye' ? DEFAULT_EYE : DEFAULT_EMBER);
      if (o.kind === 'eye') {
        n.params.look = look('eye', p.look);
        n.params.pupil = pupil(p.pupil, n.params.look);
      }
      out.overlays.push(n);
    });
    (Array.isArray(s.text) ? s.text : []).slice(0, 12).forEach(function (t) {
      if (!t) return;
      var say = String(t.say == null ? '' : t.say).replace(/\s+/g, ' ').trim().slice(0, 280);
      if (!say) return;
      var secs = num(t.seconds, 0, 60, 5);
      // 0 or less means "stays to the end"; anything else is at least half a second.
      if (secs > 0 && secs < 0.5) secs = 0.5;
      var line = { say: say, at: num(t.at, 0, 120, 0), seconds: secs, style: inList(t.style, KINDS.textStyles) ? t.style : 'plain' };
      // Only a choice that differs from the default is kept, so an older
      // scene normalises to the same object it always did.
      if (inList(t.layout, TEXT.layouts) && t.layout !== 'middle') line.layout = t.layout;
      if (inList(t['in'], TEXT.arrivals)) line['in'] = t['in'];
      if (inList(t.out, TEXT.leavings)) line.out = t.out;
      out.text.push(line);
    });
    var e = s.ending && typeof s.ending === 'object' ? s.ending : {};
    if (e.kind === 'auto') out.ending = { kind: 'auto', after: num(e.after, 0, 60, 3) };
    return out;
  }

  // ---------- The timeline ----------

  // timeline lists each layer's start, the moment it is fully built and the
  // moment it is gone (Infinity for one that stays). `last` is when the last
  // layer has finished arriving, which is when a tap scene offers Close; an
  // auto scene closes `after` seconds later.
  function timeline(scene) {
    // normalise is idempotent, so a scene is never trusted as already clean.
    var s = normalise(scene);
    var layers = [];
    var bg = s.background;
    var bgBuilt = bg.kind === 'black' || bg.kind === 'none' ? 0 : bg.seconds;
    layers.push({ type: 'background', index: 0, kind: bg.kind, start: 0, built: bgBuilt, end: Infinity });
    s.overlays.forEach(function (o, i) {
      layers.push({ type: 'overlay', index: i, kind: o.kind, start: o.at, built: o.at + o.seconds, end: Infinity });
    });
    s.text.forEach(function (t, i) {
      var end = t.seconds > 0 ? t.at + t.seconds : Infinity;
      layers.push({ type: 'text', index: i, kind: t.style, start: t.at, built: Math.min(end, t.at + TEXT_IN), end: end });
    });
    var last = 0;
    layers.forEach(function (l) { last = Math.max(last, l.end === Infinity ? l.built : l.end); });
    return {
      scene: s,
      layers: layers,
      last: last,
      closeAt: s.ending.kind === 'auto' ? last + s.ending.after : null
    };
  }

  // activeAt says which layers are on screen at t seconds and how far along:
  // `progress` is 0..1 through the build-up, `leaving` 0..1 through a line's
  // fade out.
  function activeAt(tl, t) {
    var out = [];
    tl.layers.forEach(function (l) {
      if (t < l.start) return;
      if (l.type === 'background' && l.kind === 'none') return;
      if (l.end !== Infinity && t >= l.end + (l.type === 'text' ? TEXT_OUT : 0)) return;
      var span = l.type === 'text' ? TEXT_IN : l.built - l.start;
      var progress = span > 0 ? Math.min(1, (t - l.start) / span) : 1;
      var leaving = l.end !== Infinity && t > l.end ? Math.min(1, (t - l.end) / TEXT_OUT) : 0;
      out.push({ type: l.type, index: l.index, kind: l.kind, progress: progress, leaving: leaving });
    });
    return out;
  }

  // ending says what the player may do at t: close at all (after the escape
  // delay), see the quiet Close word (tap scenes, once everything arrived), or
  // be closed by the scene itself (auto scenes).
  function ending(tl, t) {
    var kind = tl.scene.ending.kind;
    return {
      canClose: t >= ESCAPE_AFTER,
      showClose: kind === 'tap' && t >= Math.max(tl.last, ESCAPE_AFTER),
      tapCloses: kind === 'tap' && t >= Math.max(tl.last, ESCAPE_AFTER),
      autoClose: kind === 'auto' && t >= Math.max(tl.closeAt, ESCAPE_AFTER)
    };
  }

  // ---------- Small maths ----------

  function clamp01(x) { return x < 0 ? 0 : x > 1 ? 1 : x; }
  function smooth(a, b, x) { var v = clamp01((x - a) / (b - a)); return v * v * (3 - 2 * v); }
  function easeOut(x) { x = clamp01(x); return 1 - (1 - x) * (1 - x) * (1 - x); }
  function easeInOut(x) { x = clamp01(x); return x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2; }
  function lerp(a, b, x) { return a + (b - a) * x; }
  function hash(n) { var x = Math.sin(n * 127.1 + 311.7) * 43758.5453; return x - Math.floor(x); }
  // Value noise, 0..1: cheap, smooth and deterministic.
  function noise(x) {
    var i = Math.floor(x), f = x - i, u = f * f * (3 - 2 * f);
    return hash(i) * (1 - u) + hash(i + 1) * u;
  }
  function noise2(x, y) {
    var i = Math.floor(x), j = Math.floor(y), fx = x - i, fy = y - j;
    var ux = fx * fx * (3 - 2 * fx), uy = fy * fy * (3 - 2 * fy);
    var a = hash(i + j * 57), b = hash(i + 1 + j * 57), c = hash(i + (j + 1) * 57), d = hash(i + 1 + (j + 1) * 57);
    return lerp(lerp(a, b, ux), lerp(c, d, ux), uy);
  }
  // A seeded generator, so a scene lays out the same way every time it plays.
  function rng(seed) {
    var a = seed >>> 0;
    return function () {
      a = (a + 0x6D2B79F5) >>> 0;
      var t = a;
      t = Math.imul(t ^ (t >>> 15), t | 1);
      t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }
  function seedOf(str) {
    var h = 2166136261;
    for (var i = 0; i < str.length; i++) { h ^= str.charCodeAt(i); h = Math.imul(h, 16777619); }
    return h >>> 0;
  }
  function rgb(hex) {
    var n = parseInt(hex.slice(1), 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }
  function mix(c, d, x) { return [Math.round(lerp(c[0], d[0], x)), Math.round(lerp(c[1], d[1], x)), Math.round(lerp(c[2], d[2], x))]; }
  function css(c, a) { return 'rgba(' + c[0] + ',' + c[1] + ',' + c[2] + ',' + (a == null ? 1 : Math.round(a * 1000) / 1000) + ')'; }

  // keyed interpolates [p, value] keyframes with a smooth step between each,
  // for movements that hesitate (lids that fight to stay open).
  function keyed(keys, p) {
    if (p <= keys[0][0]) return keys[0][1];
    for (var i = 1; i < keys.length; i++) {
      if (p <= keys[i][0]) {
        var a = keys[i - 1], b = keys[i];
        return lerp(a[1], b[1], smooth(0, 1, (p - a[0]) / (b[0] - a[0])));
      }
    }
    return keys[keys.length - 1][1];
  }

  // The lids droop, snap back open twice, then give in.
  var LID_KEYS = [[0, 1], [0.2, 0.9], [0.33, 0.4], [0.39, 0.8], [0.55, 0.2], [0.62, 0.52], [0.79, 0.07], [0.85, 0.2], [0.97, 0], [1, 0]];
  function lidOpen(p) { return keyed(LID_KEYS, p); }

  // How open each eye look is, 0..1, through its build-up u.
  function eyeOpen(lk, u) {
    if (lk === 'almond') return keyed([[0, 0], [0.14, 0], [0.3, 0.17], [0.5, 0.15], [0.86, 1], [1, 1]], u);
    if (lk === 'rift') return keyed([[0, 0], [0.32, 0], [0.78, 1], [1, 1]], u);
    return 1;
  }

  var pure = {
    KINDS: KINDS, LOOKS: LOOKS, PUPILS: PUPILS, DEFAULT_LOOK: DEFAULT_LOOK, ESCAPE_AFTER: ESCAPE_AFTER, TEXT_IN: TEXT_IN, TEXT_OUT: TEXT_OUT,
    normalise: normalise, timeline: timeline, activeAt: activeAt, ending: ending,
    lidOpen: lidOpen, eyeOpen: eyeOpen, noise: noise, rng: rng
  };

  // ---------- Below here: drawing (browser only) ----------
  var play = typeof document === 'undefined' ? function () { throw new Error('ChronicleCutscene.play needs a browser'); } : (function () {

  // ---------- Style ----------

  var STYLE_ID = 'cs-cut-style';
  var STYLE_TEXT = [
    '.cs-cut{position:fixed;inset:0;z-index:var(--cs-cut-z,2147483000);container-type:size;color:#eef2ee;outline:none;touch-action:none;overscroll-behavior:contain;-webkit-tap-highlight-color:transparent;-webkit-user-select:none;user-select:none;contain:strict;transition:opacity .5s ease-in}',
    '.cs-cut.cs-cut-boxed{position:absolute;z-index:5}',
    '.cs-cut.cs-cut-leave{opacity:0}',
    '.cs-cut.cs-cut-quick{transition-duration:.25s}',
    '.cs-cut-veil{position:absolute;inset:0;pointer-events:none}',
    '.cs-cut-canvas{position:absolute;inset:0;width:100%;height:100%;display:block}',
    '.cs-cut-grain{position:absolute;inset:-64px;pointer-events:none;opacity:0;transition:opacity 3s ease-out;animation:cs-cut-grain .5s steps(4) infinite}',
    '@keyframes cs-cut-grain{0%{transform:translate(0,0)}25%{transform:translate(-23px,17px)}50%{transform:translate(19px,-29px)}75%{transform:translate(-11px,-7px)}100%{transform:translate(0,0)}}',
    '.cs-cut-words{position:absolute;left:0;right:0;display:flex;flex-direction:column;align-items:center;gap:.65em;box-sizing:border-box;padding:0 max(22px,env(safe-area-inset-right)) 0 max(22px,env(safe-area-inset-left));text-align:center;pointer-events:none}',
    '.cs-cut-words.cs-cut-mid{top:50%;transform:translateY(-50%)}',
    '.cs-cut-line{margin:0;max-width:min(32ch,100%);font-family:var(--font-heading,Georgia),Georgia,"Times New Roman",serif;color:var(--cs-cut-ink,#e9f3ea);text-wrap:balance;overflow-wrap:break-word;line-height:1.4}',
    '.cs-cut-whisper{font-style:italic;font-size:clamp(18px,4.6vw,30px);font-size:clamp(18px,4.6cqw,30px);letter-spacing:.035em;text-shadow:0 0 18px var(--cs-cut-haze,rgba(61,255,122,.35)),0 0 2px rgba(0,0,0,.9)}',
    '.cs-cut-w{display:inline-block;opacity:0;animation:cs-cut-whisper 1.9s cubic-bezier(.2,.7,.2,1) forwards}',
    '@keyframes cs-cut-whisper{0%{opacity:0;filter:blur(9px);transform:translateY(7px)}60%{opacity:.95}100%{opacity:1;filter:blur(0);transform:none}}',
    '.cs-cut-title{font-size:clamp(24px,7vw,54px);font-size:clamp(24px,7cqw,54px);letter-spacing:.2em;text-indent:.2em;text-transform:uppercase;font-weight:600;line-height:1.15;text-shadow:0 0 28px var(--cs-cut-haze,rgba(61,255,122,.45)),0 0 3px rgba(0,0,0,.9);animation:cs-cut-title 2.6s cubic-bezier(.16,.8,.2,1) forwards}',
    '@keyframes cs-cut-title{0%{opacity:0;filter:blur(12px);transform:scale(1.07)}45%{opacity:1}100%{opacity:1;filter:blur(0);transform:none}}',
    '.cs-cut-plain{font-size:clamp(16px,4vw,23px);font-size:clamp(16px,4cqw,23px);text-shadow:0 0 2px rgba(0,0,0,.9);animation:cs-cut-plain 1.3s ease-out forwards}',
    '@keyframes cs-cut-plain{from{opacity:0;transform:translateY(5px)}to{opacity:1;transform:none}}',
    '.cs-cut-line.cs-cut-gone{animation:cs-cut-gone 1.1s ease-in forwards}',
    '@keyframes cs-cut-gone{from{opacity:1}to{opacity:0;filter:blur(7px);transform:translateY(-5px)}}',
    // Small, widely spaced capitals: a chapter number, or the line under a place.
    '.cs-cut-caption{font-size:clamp(11px,2.3vw,15px);font-size:clamp(11px,2.3cqw,15px);letter-spacing:.42em;text-indent:.42em;text-transform:uppercase;line-height:1.3;text-shadow:0 0 2px rgba(0,0,0,.9);animation:cs-cut-plain 1.3s ease-out forwards}',
    // The corner card: small words at the bottom left, clear of film bars.
    '.cs-cut-words.cs-cut-corner{top:auto;right:auto;bottom:max(16%,calc(env(safe-area-inset-bottom) + 24px));max-width:72%;align-items:flex-start;gap:.3em;text-align:left;padding-left:max(6%,22px,env(safe-area-inset-left))}',
    '.cs-cut-corner .cs-cut-line{max-width:min(30ch,100%);line-height:1.25;text-indent:0}',
    '.cs-cut-corner .cs-cut-title{font-size:clamp(15px,3.2vw,26px);font-size:clamp(15px,3.2cqw,26px);letter-spacing:.08em}',
    '.cs-cut-corner .cs-cut-whisper,.cs-cut-corner .cs-cut-plain{font-size:clamp(13px,2.4vw,19px);font-size:clamp(13px,2.4cqw,19px)}',
    // Arrivals and leavings chosen apart from the style replace its own.
    '.cs-cut-line.cs-cut-in-fade{animation:cs-cut-fade 1.2s ease-out forwards}',
    '.cs-cut-line.cs-cut-in-rise{animation:cs-cut-rise 1.3s cubic-bezier(.2,.7,.2,1) forwards}',
    '@keyframes cs-cut-rise{from{opacity:0;transform:translateY(.6em)}to{opacity:1;transform:none}}',
    '.cs-cut-line.cs-cut-in-settle{animation:cs-cut-settle 1.9s cubic-bezier(.16,.8,.2,1) forwards}',
    // Drawn together by transform, never letter-spacing: spacing widens the
    // line, so a long title would wrap, then jump back to one line mid-move.
    '@keyframes cs-cut-settle{0%{opacity:0;filter:blur(8px);transform:scaleX(1.3)}55%{opacity:1}100%{opacity:1;filter:blur(0);transform:none}}',
    '.cs-cut-corner .cs-cut-line.cs-cut-in-settle{transform-origin:left center}',
    '.cs-cut-line.cs-cut-in-slide{animation:cs-cut-slide 1.1s cubic-bezier(.2,.8,.2,1) forwards}',
    '@keyframes cs-cut-slide{from{opacity:0;transform:translateX(-.9em)}to{opacity:1;transform:none}}',
    '.cs-cut-line.cs-cut-gone.cs-cut-out-fade{animation:cs-cut-fadeout 1s ease-in forwards}',
    '.cs-cut-line.cs-cut-gone.cs-cut-out-sink{animation:cs-cut-sink 1s ease-in forwards}',
    '@keyframes cs-cut-sink{from{opacity:1}to{opacity:0;transform:translateY(.5em)}}',
    '.cs-cut-line.cs-cut-gone.cs-cut-out-slide{animation:cs-cut-slideout .9s ease-in forwards}',
    '@keyframes cs-cut-slideout{from{opacity:1}to{opacity:0;transform:translateX(-.9em)}}',
    '.cs-cut-calm .cs-cut-line,.cs-cut-calm .cs-cut-w{animation-name:cs-cut-fade;animation-timing-function:ease-out}',
    '.cs-cut-calm .cs-cut-line.cs-cut-gone{animation-name:cs-cut-fadeout}',
    '@keyframes cs-cut-fade{from{opacity:0}to{opacity:1}}',
    '@keyframes cs-cut-fadeout{from{opacity:1}to{opacity:0}}',
    '.cs-cut-x{position:absolute;top:max(10px,env(safe-area-inset-top));right:max(10px,env(safe-area-inset-right));width:44px;height:44px;margin:0;padding:0;border:0;border-radius:50%;background:rgba(255,255,255,.07);color:rgba(255,255,255,.72);display:flex;align-items:center;justify-content:center;cursor:pointer;opacity:0;visibility:hidden;transition:opacity .5s ease-out,visibility 0s .5s,background-color .12s,color .12s}',
    '.cs-cut-x svg{width:18px;height:18px}',
    '.cs-cut-x.cs-cut-on{opacity:1;visibility:visible;transition:opacity .5s ease-out,background-color .12s,color .12s}',
    '.cs-cut-x:hover{background:rgba(255,255,255,.16);color:#fff}',
    '.cs-cut-x:active{transform:scale(.96)}',
    '.cs-cut-end{position:absolute;left:50%;bottom:max(16px,calc(env(safe-area-inset-bottom) + 10px));transform:translateX(-50%);min-height:44px;min-width:88px;margin:0;padding:0 20px;border:0;border-radius:22px;background:none;color:rgba(255,255,255,.6);font:500 15px/44px system-ui,-apple-system,"Segoe UI",sans-serif;letter-spacing:.08em;cursor:pointer;opacity:0;visibility:hidden;transition:opacity 1s ease-out,visibility 0s 1s,color .12s,background-color .12s}',
    '.cs-cut-end.cs-cut-on{opacity:1;visibility:visible;transition:opacity 1s ease-out,color .12s,background-color .12s}',
    '.cs-cut-end:hover{color:#fff;background:rgba(255,255,255,.08)}',
    '.cs-cut-x:focus-visible,.cs-cut-end:focus-visible{outline:2px solid rgba(255,255,255,.85);outline-offset:2px}',
    '.cs-cut-tapclose{cursor:pointer}'
  ].join('\n');

  function injectStyle(doc) {
    if (doc.getElementById(STYLE_ID)) return;
    var st = doc.createElement('style');
    st.id = STYLE_ID;
    st.textContent = STYLE_TEXT;
    (doc.head || doc.documentElement).appendChild(st);
  }

  function prefersReduced() {
    try {
      var m = document.documentElement.getAttribute('data-motion');
      if (m === 'calm' || m === 'off') return true;
      return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    } catch (e) { return false; }
  }

  function canvasOf(w, h) {
    var c = document.createElement('canvas');
    c.width = Math.max(1, Math.round(w));
    c.height = Math.max(1, Math.round(h));
    return c;
  }

  // The film grain is one small noise tile, made once per page and moved by
  // CSS, so it costs the main thread nothing per frame.
  var grainURL = null;
  function grain() {
    if (grainURL !== null) return grainURL;
    try {
      var c = canvasOf(128, 128), g = c.getContext('2d'), d = g.createImageData(128, 128), r = rng(7);
      for (var i = 0; i < d.data.length; i += 4) {
        var v = Math.floor(r() * 255);
        d.data[i] = d.data[i + 1] = d.data[i + 2] = v;
        d.data[i + 3] = 255;
      }
      g.putImageData(d, 0, 0);
      grainURL = c.toDataURL('image/png');
    } catch (e) { grainURL = ''; }
    return grainURL;
  }

  // ---------- Sprites ----------
  // Soft shapes are drawn once per colour and stamped, which is far cheaper
  // than building gradients every frame.

  var spriteCache = {};
  function sprites(hex) {
    if (spriteCache[hex]) return spriteCache[hex];
    var col = rgb(hex), hot = mix(col, [255, 255, 255], 0.7), white = [255, 255, 255];
    var glow = canvasOf(256, 256), g = glow.getContext('2d');
    var gr = g.createRadialGradient(128, 128, 0, 128, 128, 128);
    gr.addColorStop(0, css(hot, 0.85));
    gr.addColorStop(0.1, css(col, 0.55));
    gr.addColorStop(0.3, css(col, 0.2));
    gr.addColorStop(0.6, css(col, 0.06));
    gr.addColorStop(1, css(col, 0));
    g.fillStyle = gr; g.fillRect(0, 0, 256, 256);

    var ember = canvasOf(32, 32), e = ember.getContext('2d');
    var eg = e.createRadialGradient(16, 16, 0, 16, 16, 16);
    eg.addColorStop(0, css(white, 1));
    eg.addColorStop(0.2, css(hot, 0.95));
    eg.addColorStop(0.45, css(col, 0.5));
    eg.addColorStop(1, css(col, 0));
    e.fillStyle = eg; e.fillRect(0, 0, 32, 32);

    spriteCache[hex] = { glow: glow, ember: ember };
    return spriteCache[hex];
  }

  // stamp draws a sprite centred on x, y, rotated by a, w by h.
  function stamp(g, img, x, y, w, h, a, alpha) {
    if (alpha <= 0.003) return;
    g.globalAlpha = alpha > 1 ? 1 : alpha;
    if (a) {
      g.save();
      g.translate(x, y);
      g.rotate(a);
      g.drawImage(img, -w / 2, -h / 2, w, h);
      g.restore();
    } else {
      g.drawImage(img, x - w / 2, y - h / 2, w, h);
    }
    g.globalAlpha = 1;
  }

  // ---------- The background ----------

  // A low-resolution buffer the soft darkness is drawn into; scaled up to the
  // screen it blurs for free.
  function Buffer(scale, blur) {
    this.scale = scale;
    this.blur = blur;
    this.c = canvasOf(2, 2);
    this.g = this.c.getContext('2d');
    // A canvas filter blurs every draw call on its own, so the whole buffer
    // is blurred once, into a second one.
    this.filter = typeof this.g.filter === 'string';
    if (this.filter) { this.c2 = canvasOf(2, 2); this.g2 = this.c2.getContext('2d'); }
  }
  Buffer.prototype.size = function (w, h) {
    this.w = Math.max(24, Math.round(w * this.scale));
    this.h = Math.max(24, Math.round(h * this.scale));
    this.c.width = this.w; this.c.height = this.h;
    if (this.c2) { this.c2.width = this.w; this.c2.height = this.h; }
  };
  Buffer.prototype.out = function () {
    if (!this.filter) return this.c;
    this.g2.filter = 'none';
    this.g2.clearRect(0, 0, this.w, this.h);
    this.g2.filter = 'blur(' + this.blur + 'px)';
    this.g2.drawImage(this.c, 0, 0);
    return this.c2;
  };

  // The edge spots of the falling-asleep looks: where each starts on the
  // border, how far in it creeps and when it sets off.
  function edgeSpots(r, n) {
    var out = [];
    for (var i = 0; i < n; i++) {
      var side = i % 4, along = (i + r()) / n * 4 % 1;
      out.push({
        side: side,
        along: 0.08 + 0.84 * ((along + r() * 0.25) % 1),
        size: 0.22 + r() * 0.2,
        depth: 0.08 + r() * 0.26,
        delay: r() * 0.4,
        phase: r() * 6.28,
        speed: 0.6 + r() * 0.8,
        drift: r() * 100
      });
    }
    return out;
  }
  function edgePoint(s, w, h) {
    var m = Math.min(w, h) * 0.06;
    if (s.side === 0) return { x: s.along * w, y: -m, nx: 0, ny: 1 };
    if (s.side === 1) return { x: w + m, y: s.along * h, nx: -1, ny: 0 };
    if (s.side === 2) return { x: (1 - s.along) * w, y: h + m, nx: 0, ny: -1 };
    return { x: -m, y: (1 - s.along) * h, nx: 1, ny: 0 };
  }
  // Dark spots are stamped from two prepared sprites (soft, and ink with a
  // darker rim); building a gradient for each would cost far more.
  var spotSprites = null;
  function spotSprite(rim) {
    if (!spotSprites) {
      spotSprites = [false, true].map(function (withRim) {
        var c = canvasOf(64, 64), g = c.getContext('2d'), gr = g.createRadialGradient(32, 32, 0, 32, 32, 32);
        if (withRim) {
          gr.addColorStop(0, 'rgba(0,0,0,0.82)');
          gr.addColorStop(0.78, 'rgba(0,0,0,1)');
          gr.addColorStop(1, 'rgba(0,0,0,0)');
        } else {
          // Close to a Gaussian, so no spot ever shows an edge.
          gr.addColorStop(0, 'rgba(0,0,0,1)');
          gr.addColorStop(0.25, 'rgba(0,0,0,0.86)');
          gr.addColorStop(0.5, 'rgba(0,0,0,0.52)');
          gr.addColorStop(0.75, 'rgba(0,0,0,0.18)');
          gr.addColorStop(1, 'rgba(0,0,0,0)');
        }
        g.fillStyle = gr;
        g.fillRect(0, 0, 64, 64);
        return c;
      });
    }
    return spotSprites[rim ? 1 : 0];
  }
  function softSpot(g, x, y, r, a, rim) {
    if (r < 0.5 || a <= 0.004) return;
    g.globalAlpha = a > 1 ? 1 : a;
    g.drawImage(spotSprite(rim), x - r, y - r, r * 2, r * 2);
    g.globalAlpha = 1;
  }


  function inkBlooms(r) {
    var out = [];
    for (var i = 0; i < 13; i++) {
      var tds = [], k = 2 + Math.floor(r() * 2);
      for (var j = 0; j < k; j++) tds.push({ a: (r() - 0.5) * 1.6, curl: (r() - 0.5) * 3.2, len: 0.3 + r() * 0.45, seed: r() * 100, w: 0.8 + r() * 0.7 });
      out.push({ side: i % 4, along: 0.05 + r() * 0.9, delay: r() * 0.45, size: 0.1 + r() * 0.12, tendrils: tds });
    }
    return out;
  }
  function floaters(r, n) {
    var out = [];
    for (var i = 0; i < n; i++) {
      // Each spot is a loose cluster of blobs, so none reads as a circle.
      var parts = [];
      for (var k = 0; k < 4; k++) parts.push({ dx: (r() - 0.5) * 1.6, dy: (r() - 0.5) * 1.2, k: 0.45 + r() * 0.6 });
      out.push({ x: 0.15 + r() * 0.7, y: 0.12 + r() * 0.7, size: 0.06 + r() * 0.08, delay: 0.15 + r() * 0.45, s: r() * 100, parts: parts });
    }
    return out;
  }

  function Background(bg, seed, rm, veil) {
    var r = rng(seed);
    this.kind = bg.kind;
    this.look = bg.params.look || '';
    this.rm = rm;
    this.veil = veil;
    this.veilSet = '';
    if (this.kind !== 'fall-asleep' || rm) return;
    if (this.look === 'ink') {
      this.buf = new Buffer(0.25, 1.5);
      this.blooms = inkBlooms(r);
    } else if (this.look === 'eyelids') {
      this.buf = new Buffer(1 / 6, 2.2);
      this.spots = edgeSpots(r, 18);
    } else {
      this.buf = new Buffer(1 / 6, 2.4);
      this.spots = edgeSpots(r, 10);
      this.floaters = floaters(r, 8);
    }
  }
  Background.prototype.resize = function (w, h) {
    this.w = w; this.h = h;
    if (this.buf) this.buf.size(w, h);
  };
  Background.prototype.setVeil = function (v) {
    if (!this.veil || v === this.veilSet) return;
    this.veilSet = v;
    if (v === 'none') { this.veil.style.display = 'none'; return; }
    this.veil.style.display = '';
    this.veil.style.webkitBackdropFilter = v;
    this.veil.style.backdropFilter = v;
  };
  Background.prototype.draw = function (ctx, t, p) {
    var W = ctx.canvas.width, H = ctx.canvas.height;
    if (this.kind === 'none') return;
    if (this.kind === 'black') { ctx.fillStyle = '#000'; ctx.fillRect(0, 0, W, H); return; }
    if (this.kind === 'fade-to-black' || this.rm) {
      ctx.fillStyle = 'rgba(0,0,0,' + easeInOut(p) + ')';
      ctx.fillRect(0, 0, W, H);
      return;
    }
    // Once it is fully dark there is nothing left to draw but black.
    if (p >= 1) { this.setVeil('none'); ctx.fillStyle = '#000'; ctx.fillRect(0, 0, W, H); return; }
    var b = this.buf, g = b.g;
    g.setTransform(1, 0, 0, 1, 0, 0);
    g.globalCompositeOperation = 'source-over';
    g.clearRect(0, 0, b.w, b.h);
    if (this.look === 'ink') this.ink(g, b.w, b.h, t, p);
    else if (this.look === 'eyelids') this.lids(g, b.w, b.h, t, p);
    else this.drift(g, b.w, b.h, t, p);
    ctx.imageSmoothingEnabled = true;
    if ('imageSmoothingQuality' in ctx) ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(b.out(), 0, 0, W, H);
    var last = smooth(0.82, 1, p);
    if (last > 0) { ctx.fillStyle = 'rgba(0,0,0,' + last + ')'; ctx.fillRect(0, 0, W, H); }
  };

  // Shared by the looks: soft dark patches that set off from the border,
  // creep inward on a wandering line and breathe as they go.
  Background.prototype.edges = function (g, w, h, t, p, reach, strength) {
    var m = Math.min(w, h);
    this.spots.forEach(function (s) {
      var q = easeInOut(clamp01((p - s.delay) / (1 - s.delay * 0.7)));
      if (q <= 0) return;
      var e = edgePoint(s, w, h);
      var d = s.depth * q * reach * m;
      var wob = (noise(t * 0.18 + s.drift) - 0.5) * m * 0.3;
      var x = e.x + e.nx * d - e.ny * wob, y = e.y + e.ny * d + e.nx * wob;
      var r = s.size * m * (0.45 + 1.9 * q) * (1 + 0.1 * Math.sin(t * s.speed * 1.4 + s.phase));
      var a = Math.min(1, 0.3 + 0.8 * q) * (0.82 + 0.18 * Math.sin(t * s.speed + s.phase * 2)) * strength;
      softSpot(g, x, y, r, a, false);
    });
  };

  Background.prototype.lids = function (g, w, h, t, p) {
    var R = Math.sqrt(w * w + h * h) / 2, cx = w / 2, cy = h / 2;
    var inner = R * Math.max(0, 0.9 - 0.95 * p);
    var vg = g.createRadialGradient(cx, cy, inner, cx, cy, inner + R * 0.85);
    vg.addColorStop(0, 'rgba(0,0,0,0)');
    vg.addColorStop(1, 'rgba(0,0,0,' + lerp(0.4, 1, p) + ')');
    g.fillStyle = vg; g.fillRect(0, 0, w, h);
    this.edges(g, w, h, t, p, 1.6, 1);
    // A tremble while the lids fight to stay open.
    var o = clamp01(lidOpen(p) + (noise(t * 3.1) - 0.5) * 0.03 * (1 - p));
    var gap = Math.pow(o, 1.35) * 1.15 * h;
    lid(g, w, h, cy, gap, -1);
    lid(g, w, h, cy, gap * 0.8, 1);
  };
  // One eyelid: a shadow that reaches past its edge, then the lid itself.
  function lid(g, w, h, cy, gap, dir) {
    var pad = w * 0.12, edge = dir < 0 ? -pad : h + pad;
    var passes = [[0.7, 0.3], [0.88, 0.55], [1, 1]];
    for (var i = 0; i < passes.length; i++) {
      var gp = gap * passes[i][0];
      g.fillStyle = 'rgba(4,1,2,' + passes[i][1] + ')';
      g.beginPath();
      g.moveTo(-pad, edge);
      g.lineTo(w + pad, edge);
      g.lineTo(w + pad, cy + dir * gp * 0.5);
      g.quadraticCurveTo(w / 2, cy + dir * gp * 1.5, -pad, cy + dir * gp * 0.5);
      g.closePath();
      g.fill();
    }
  }

  Background.prototype.ink = function (g, w, h, t, p) {
    var m = Math.min(w, h);
    g.fillStyle = 'rgba(0,0,0,' + (0.3 * smooth(0.25, 1, p)) + ')';
    g.fillRect(0, 0, w, h);
    this.blooms.forEach(function (bl) {
      var q = clamp01((p - bl.delay) / (1 - bl.delay * 0.8));
      if (q <= 0) return;
      var f = easeOut(q), e = edgePoint(bl, w, h);
      softSpot(g, e.x, e.y, m * bl.size * (0.6 + 2.2 * f), 0.5 + 0.45 * f, true);
      softSpot(g, e.x + e.nx * m * bl.size * f, e.y + e.ny * m * bl.size * f, m * bl.size * (0.4 + 1.4 * f), 0.4 * f, false);
      bl.tendrils.forEach(function (td) {
        // Many small overlapping stamps along a curling path, with feathery
        // wisps off its sides, read as one continuous tendril of ink.
        var steps = 22, x = e.x, y = e.y, ang = Math.atan2(e.ny, e.nx) + td.a;
        var reach = f * steps, step = td.len * m / steps * 1.15;
        for (var j = 0; j < reach; j++) {
          ang += (noise(td.seed + j * 0.22 + t * 0.04) - 0.5) * td.curl * 0.34;
          var part = Math.min(1, reach - j);
          x += Math.cos(ang) * step * part;
          y += Math.sin(ang) * step * part;
          var taper = 1 - j / steps * 0.5;
          var rad = m * 0.085 * td.w * taper * (0.55 + 0.75 * f) * (1 + 0.1 * Math.sin(t * 0.7 + j * 0.4 + td.seed));
          softSpot(g, x, y, rad * (0.5 + 0.5 * part), 0.3 * part, false);
          if (j % 2 === 1) {
            // Billows off alternate sides, so the plume's edge curls.
            var side = Math.sin(j * 0.9 + td.seed + t * 0.15) * 0.9, px = -Math.sin(ang), py = Math.cos(ang);
            softSpot(g, x + px * rad * side, y + py * rad * side, rad * 0.8, 0.24 * part, false);
          }
        }
      });
    });
  };

  Background.prototype.drift = function (g, w, h, t, p) {
    // The real page beneath loses focus in waves, as eyes do when they fight
    // sleep, while its colour drains and it dims.
    if (this.veil) {
      var nod = keyed([[0, 0], [0.24, 0.6], [0.32, 0.22], [0.5, 0.92], [0.58, 0.5], [0.74, 1], [1, 1]], p);
      var blur = Math.round((6.5 * nod * smooth(0, 0.3, p) + 2 * p) * 4) / 4;
      var sat = Math.round((1 - 0.95 * smooth(0.04, 0.6, p)) * 50) / 50;
      var bri = Math.round((1 - 0.5 * smooth(0.12, 0.95, p)) * 50) / 50;
      this.setVeil('blur(' + blur + 'px) saturate(' + sat + ') brightness(' + bri + ')');
    }
    var R = Math.sqrt(w * w + h * h) / 2, cx = w / 2, cy = h / 2, m = Math.min(w, h);
    // Tunnel vision: black everywhere, with a soft, uneven hole that closes.
    g.fillStyle = '#000';
    g.fillRect(0, 0, w, h);
    g.globalCompositeOperation = 'destination-out';
    var close = Math.pow(p, 0.8);
    for (var k = 0; k < 5; k++) {
      var ox = (noise(t * 0.11 + k * 13) - 0.5) * m * 0.22, oy = (noise(t * 0.09 + k * 29) - 0.5) * m * 0.18;
      var r = R * lerp(1.55, 0.04, close) * (0.78 + 0.44 * noise(t * 0.21 + k * 7));
      var gr = g.createRadialGradient(cx + ox, cy + oy, 0, cx + ox, cy + oy, r);
      gr.addColorStop(0, 'rgba(0,0,0,1)');
      gr.addColorStop(0.55, 'rgba(0,0,0,0.93)');
      gr.addColorStop(1, 'rgba(0,0,0,0)');
      g.fillStyle = gr;
      g.fillRect(cx + ox - r, cy + oy - r, r * 2, r * 2);
    }
    g.globalCompositeOperation = 'source-over';
    this.edges(g, w, h, t, p, 1.2, 0.8);
    // Dark spots float in what is left of sight, sinking slowly.
    this.floaters.forEach(function (f) {
      var a = smooth(f.delay, f.delay + 0.25, p) * 0.62;
      if (a <= 0) return;
      var x = (f.x + (noise(t * 0.06 + f.s) - 0.5) * 0.25) * w;
      var y = (f.y + (noise(t * 0.05 + f.s + 50) - 0.5) * 0.2 + t * 0.006) * h;
      var rad = m * f.size * (1 + 0.6 * p);
      f.parts.forEach(function (q, k) {
        var br = 1 + 0.18 * Math.sin(t * (0.5 + 0.2 * k) + f.s + k);
        softSpot(g, x + q.dx * rad * (1 + 0.2 * Math.sin(t * 0.3 + k)), y + q.dy * rad, rad * q.k * br, a * 0.55, false);
      });
    });
  };

  // ---------- Embers ----------

  // A few hundred sparks at most, each stamped from one sprite.
  function Embers(hex, rm) {
    this.sp = sprites(hex);
    this.rm = rm;
    this.list = [];
    this.acc = 0;
  }
  Embers.prototype.step = function (dt, rate, emit) {
    if (this.rm) return;
    this.acc = Math.min(4, this.acc + rate * dt);
    while (this.acc >= 1) {
      this.acc -= 1;
      if (this.list.length < 180) this.list.push(emit());
    }
    var keep = [];
    for (var i = 0; i < this.list.length; i++) {
      var e = this.list[i];
      e.age += dt;
      if (e.age >= e.life) continue;
      e.vx += (noise(e.s + e.age * 0.9) - 0.5) * 70 * dt;
      e.vx *= 1 - 0.6 * dt;
      e.vy -= e.lift * dt;
      e.x += e.vx * dt;
      e.y += e.vy * dt;
      keep.push(e);
    }
    this.list = keep;
  };
  Embers.prototype.draw = function (ctx, alpha) {
    if (!this.list.length) return;
    ctx.globalCompositeOperation = 'lighter';
    for (var i = 0; i < this.list.length; i++) {
      var e = this.list[i], k = e.age / e.life;
      var a = Math.pow(Math.sin(Math.PI * k), 0.7) * e.a * (0.65 + 0.35 * noise(e.s + e.age * 9)) * alpha;
      var size = e.size * (1 - k * 0.55) * 4;
      stamp(ctx, this.sp.ember, e.x, e.y, size, size, 0, a);
    }
    ctx.globalCompositeOperation = 'source-over';
  };
  function ember(x, y, vx, vy, life, size, lift) {
    return { x: x, y: y, vx: vx, vy: vy, life: life, size: size, lift: lift, age: 0, a: 0.6 + Math.random() * 0.4, s: Math.random() * 100 };
  }

  // The embers overlay: sparks rising off the bottom of the screen.
  function EmberLayer(ov, rm) {
    this.col = rgb(ov.params.color);
    this.e = new Embers(ov.params.color, rm);
    this.rm = rm;
  }
  EmberLayer.prototype.layout = function (w, h) { this.w = w; this.h = h; };
  EmberLayer.prototype.draw = function (ctx, t, u, after, dt, alpha) {
    var w = this.w, h = this.h;
    var heat = smooth(0, 1, u);
    var gr = ctx.createLinearGradient(0, h, 0, h * 0.55);
    gr.addColorStop(0, css(this.col, 0.16 * heat * alpha));
    gr.addColorStop(1, css(this.col, 0));
    ctx.globalCompositeOperation = 'lighter';
    ctx.fillStyle = gr;
    ctx.fillRect(0, h * 0.55, w, h * 0.45);
    ctx.globalCompositeOperation = 'source-over';
    this.e.step(dt, (8 + 34 * heat) * Math.max(0.6, w / 900), function () {
      return ember(Math.random() * w, h + 8, (Math.random() - 0.5) * 30, -(30 + Math.random() * 70), 2.5 + Math.random() * 4, 0.8 + Math.random() * 2.2, 4 + Math.random() * 8);
    });
    this.e.draw(ctx, alpha);
  };

  // Film bars: black bands slide in from the top and bottom over the layer's
  // build-up and stay, framing whatever is under them like a widescreen film.
  // Under reduced motion they are simply there, fading in.
  var BAR = 0.12;
  function Letterbox(rm) { this.rm = rm; }
  Letterbox.prototype.layout = function (w, h) { this.w = w; this.h = h; };
  Letterbox.prototype.draw = function (ctx, t, u, after, dt, alpha) {
    var w = this.w, h = this.h, bar = Math.ceil(h * BAR);
    var off = this.rm ? 0 : Math.round((1 - easeOut(u)) * bar);
    ctx.globalAlpha = alpha;
    ctx.fillStyle = '#000';
    ctx.fillRect(0, -off, w, bar);
    ctx.fillRect(0, h - bar + off, w, bar);
    ctx.globalAlpha = 1;
  };

  var WHITE = [255, 255, 255], BLACK = [0, 0, 0];

  // ---------- Fire ----------
  // The eye's fire is a field of turbulent noise worked out per pixel on a
  // small buffer and scaled up, so its cost follows the buffer, not the
  // screen. Noise here reads a lookup table: the field samples it tens of
  // thousands of times a frame, which sin-hash noise could not afford.

  var NT = null;
  function qn(x, y) {
    if (!NT) {
      NT = new Float32Array(65536);
      var r = rng(4231);
      for (var k = 0; k < 65536; k++) NT[k] = r();
    }
    var xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi;
    var X0 = xi & 255, Y0 = (yi & 255) << 8, X1 = (X0 + 1) & 255, Y1 = ((yi + 1) & 255) << 8;
    var a = NT[Y0 + X0], b = NT[Y0 + X1], c = NT[Y1 + X0], d = NT[Y1 + X1];
    var u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf);
    return a + (b - a) * u + (c - a) * v + (a - b - c + d) * u * v;
  }

  // Pixels are written as whole words, so the colour table is laid out for
  // the machine's byte order.
  var LITTLE = new Uint8Array(new Uint32Array([1]).buffer)[0] === 1;
  function word(r, g, b, a) {
    return LITTLE ? ((a << 24) | (b << 16) | (g << 8) | r) >>> 0 : ((r << 24) | (g << 16) | (b << 8) | a) >>> 0;
  }
  // A ramp of [heat, colour, alpha] stops turned into 256 packed pixels.
  function rampTable(stops) {
    var out = new Uint32Array(256);
    for (var i = 0; i < 256; i++) {
      var v = i / 255, k = 1;
      while (k < stops.length - 1 && v > stops[k][0]) k++;
      var a = stops[k - 1], b = stops[k], f = clamp01((v - a[0]) / (b[0] - a[0]));
      var c = mix(a[1], b[1], f);
      out[i] = word(c[0], c[1], c[2], Math.round(255 * lerp(a[2], b[2], f)));
    }
    return out;
  }

  // The eye's outline as a distance: negative inside, growing outward, in
  // units of the eye's width. a and b are its half width and half height;
  // pe 0.5 is a circle, higher is the pointed almond of a lid.
  function outline(X, Y, a, b, pe) {
    var ax = X < 0 ? -X : X, ay = Y < 0 ? -Y : Y, hb = 0, dx = 0;
    if (ax < a) { var q = 1 - (ax * ax) / (a * a); hb = b * Math.pow(q, pe) * (Y > 0 ? 0.86 : 1); } else dx = ax - a;
    var dy = ay - hb;
    if (dx > 0) return Math.sqrt(dx * dx + (dy > 0 ? dy * dy : 0));
    return dy;
  }

  // One small buffer of field values over a region round the eye, in the
  // eye's own units, cut to what is on screen.
  function Field(cols, x0, x1, y0, y1) {
    this.cols = cols;
    this.rx0 = x0; this.rx1 = x1; this.ry0 = y0; this.ry1 = y1;
    this.c = canvasOf(2, 2);
    this.g = this.c.getContext('2d');
  }
  Field.prototype.layout = function (cx, cy, W, vw, vh, q) {
    var cell = W * (this.rx1 - this.rx0) / Math.max(24, Math.round(this.cols * q));
    // Only the part of the region that is on screen is worked out.
    var L = Math.max(cx + this.rx0 * W, 0), R = Math.min(cx + this.rx1 * W, vw);
    var T = Math.max(cy + this.ry0 * W, 0), B = Math.min(cy + this.ry1 * W, vh);
    var nx = Math.max(2, Math.ceil((R - L) / cell)), ny = Math.max(2, Math.ceil((B - T) / cell));
    this.x = L; this.y = T; this.w = nx * cell; this.h = ny * cell;
    this.nx = nx; this.ny = ny;
    this.c.width = nx; this.c.height = ny;
    this.img = this.g.createImageData(nx, ny);
    this.px = new Uint32Array(this.img.data.buffer);
    this.X = new Float32Array(nx);
    this.Y = new Float32Array(ny);
    for (var i = 0; i < nx; i++) this.X[i] = (L + (i + 0.5) * cell - cx) / W;
    for (var j = 0; j < ny; j++) this.Y[j] = (T + (j + 0.5) * cell - cy) / W;
  };
  Field.prototype.put = function () { this.g.putImageData(this.img, 0, 0); };

  // Fire: flames rise off the eye's outline, tallest above it, leaning out
  // at the sides and curling as they climb. Each pixel's heat is how far up
  // its flame it sits, broken by domain-warped noise carried up through the
  // flame, so tongues lick, split and thin to smoke. A bright bed along the
  // outline never goes out.
  function Fire(hex) {
    var col = rgb(hex), deep = mix(col, BLACK, 0.7), hot = mix(col, WHITE, 0.4);
    this.lut = rampTable([
      [0, mix(col, BLACK, 0.85), 0],
      [0.06, deep, 0.07],
      [0.16, deep, 0.32],
      [0.3, mix(col, BLACK, 0.4), 0.65],
      [0.5, col, 0.92],
      [0.7, hot, 1],
      [0.86, mix(col, WHITE, 0.72), 1],
      [1, mix(col, WHITE, 0.94), 1]
    ]);
    this.smoke = rampTable([[0, deep, 0], [1, mix(col, BLACK, 0.6), 0.16]]);
    this.f = new Field(200, -1.4, 1.4, -1.3, 0.62);
    this.glow = canvasOf(2, 2);
    this.gg = this.glow.getContext('2d');
    this.q = 1;
    this.cost = 0;
    this.frames = 0;
    this.key = '';
    this.tg = null;
  }
  Fire.prototype.layout = function (cx, cy, W, vw, vh) {
    this.args = [cx, cy, W, vw, vh];
    this.f.layout(cx, cy, W, vw, vh, this.q);
    this.glow.width = Math.max(2, Math.round(this.f.nx / 4));
    this.glow.height = Math.max(2, Math.round(this.f.ny / 4));
    this.tg = new Float32Array(this.f.nx);
    this.key = '';
  };
  // render works the field out for shape s ({a, b, pe, H, heat}) at time t.
  Fire.prototype.render = function (t, s) {
    var key = t + '|' + s.a + '|' + s.b + '|' + s.H + '|' + s.heat;
    if (key === this.key) return;
    this.key = key;
    var t0 = Date.now();
    var f = this.f, X = f.X, Y = f.Y, px = f.px, lut = this.lut, smk = this.smoke, nx = f.nx, ny = f.ny;
    var a = s.a, b = s.b, pe = s.pe, H = s.H, heat = s.heat, i, j;
    var rise = t * 1.1, sway = t * 0.55;
    // Some stretches of the outline burn higher for a while.
    for (i = 0; i < nx; i++) this.tg[i] = 0.4 + 0.75 * qn(X[i] * 5 + 40.5, t * 0.45) + 0.45 * qn(X[i] * 1.7 + 12.5, t * 0.21);
    // Short flames are small ones: the noise tightens as they shrink, so a
    // low fire breaks into many small licks instead of a few stiff columns.
    var vk = 0.5 / Math.max(0.12, H), uk = Math.sqrt(vk);
    var top = H * 3.1 + b, o = 0;
    for (j = 0; j < ny; j++) {
      var y = Y[j];
      for (i = 0; i < nx; i++, o++) {
        var x = X[i];
        if (y < -top || x > a + top || x < -a - top) { px[o] = 0; continue; }
        var d = outline(x, y, a, b, pe);
        if (d < -0.07) { px[o] = 0; continue; }
        // Tall above the eye, short beneath it, where flames only peek out;
        // anywhere over the outline counts as above, so a thin seam burns
        // along its whole length.
        var ax = x < 0 ? -x : x, ex = ax > a * 0.8 ? ax - a * 0.8 : 0;
        var up = 0.5 - 0.5 * y / (Math.sqrt(ex * ex * 0.5 + y * y) + 0.0001);
        // Tallest over the middle, so the fire rises as one plume round the
        // eye rather than a wall.
        var hk = H * (0.07 + 0.93 * up * up) * this.tg[i] * (1 - 0.7 * smooth(0.12, 0.9, ax));
        if (d > hk * 1.9) { px[o] = 0; continue; }
        var h01 = d / hk;
        var dd = d > 0 ? d : 0;
        // Flow lines: up through the flame and out at the sides, swaying
        // more the higher a tongue reaches.
        var u = (x - (x < 0 ? -1 : 1) * dd * 0.18 * (1 - up)) * uk, v = (y * 0.8 - dd * 0.35) * vk;
        u += (qn(u * 2.6 + 9.1, v * 2.2 + sway) - 0.5) * 0.16 * (h01 > 0 ? h01 : 0);
        var n = qn(u * 8, v * 3.4 + rise) * 0.6 + qn(u * 16 + 5.3, v * 7 + rise * 1.9) * 0.4;
        var hh = h01 > 0 ? h01 : 0, val = (1 - hh) * 1.05 + (n - 0.5) * (1 + 2.8 * hh) - 0.08;
        if (y > 0) val -= y * 2.6;
        var bed = d / 0.016;
        val = Math.max(val, Math.exp(-bed * bed) * (0.75 + 1.2 * (n - 0.5)));
        if (d < -0.02) val *= smooth(-0.07, -0.02, d);
        val *= heat;
        if (val > 0.004) {
          var k = Math.round(val * 165);
          px[o] = lut[k > 255 ? 255 : k];
        } else if (h01 > 0.55 && heat > 0) {
          // Thin smoke off the tips, lit faintly by the fire below.
          var sm = smooth(0.55, 1, h01) * (1 - smooth(1.2, 1.9, h01)) * smooth(0.5, 0.7, n) * heat;
          px[o] = sm > 0.01 ? smk[sm >= 1 ? 255 : Math.round(sm * 255)] : 0;
        } else px[o] = 0;
      }
    }
    f.put();
    this.gg.clearRect(0, 0, this.glow.width, this.glow.height);
    this.gg.drawImage(f.c, 0, 0, this.glow.width, this.glow.height);
    // A slow device gets a coarser field rather than a dropped frame.
    var ms = Date.now() - t0;
    this.cost = this.frames ? this.cost * 0.9 + ms * 0.1 : ms;
    this.frames++;
    if (this.frames > 12 && this.cost > 7 && this.q > 0.55) {
      this.q *= 0.8;
      this.frames = 0;
      this.layout.apply(this, this.args);
    }
  };
  Fire.prototype.draw = function (ctx, alpha, glow) {
    var f = this.f;
    ctx.globalCompositeOperation = 'lighter';
    ctx.imageSmoothingEnabled = true;
    if ('imageSmoothingQuality' in ctx) ctx.imageSmoothingQuality = 'low';
    ctx.globalAlpha = alpha;
    ctx.drawImage(f.c, f.x, f.y, f.w, f.h);
    // The flames light the air round them: the field shrunk and drawn back
    // up, which blurs it for free.
    ctx.globalAlpha = alpha * glow;
    ctx.drawImage(this.glow, f.x - f.w * 0.04, f.y - f.h * 0.04, f.w * 1.08, f.h * 1.08);
    ctx.globalAlpha = 1;
    ctx.globalCompositeOperation = 'source-over';
  };

  // Haze: a luminous mist that hangs over the eye and drifts slowly upward,
  // so the eye reads as seen through rising heat.
  function Haze(hex) {
    var col = rgb(hex);
    this.lut = rampTable([[0, mix(col, BLACK, 0.3), 0], [0.4, col, 0.4], [1, mix(col, WHITE, 0.55), 0.85]]);
    this.f = new Field(64, -0.66, 0.66, -0.42, 0.38);
    this.key = '';
  }
  Haze.prototype.layout = function (cx, cy, W, vw, vh) { this.f.layout(cx, cy, W, vw, vh, 1); this.key = ''; };
  Haze.prototype.render = function (t, s) {
    // The mist moves slowly, so twenty updates a second are plenty.
    var key = Math.round(t * 20) + '|' + s.a + '|' + s.b;
    if (key === this.key) return;
    this.key = key;
    var f = this.f, X = f.X, Y = f.Y, px = f.px, lut = this.lut, o = 0;
    var dr = t * 0.07, sw = t * 0.05;
    for (var j = 0; j < f.ny; j++) {
      for (var i = 0; i < f.nx; i++, o++) {
        var x = X[i], y = Y[j];
        var d = outline(x, y, s.a, s.b, s.pe);
        var m = (1 - smooth(-0.05, 0.05 + (y < 0 ? -y * 0.35 : 0), d)) * (y < 0 ? 1 : 1 - smooth(0, 0.3, y) * 0.5);
        if (m <= 0) { px[o] = 0; continue; }
        var w = qn(x * 2.5 + sw + 5, y * 4 + dr);
        var n = qn(x * 4 + w * 1.8 - sw, y * 6.5 + dr * 2.2 + w * 1.6) * 0.6 + qn(x * 9 + w * 2.4, y * 12 + dr * 3.5) * 0.4;
        var v = (0.16 + 0.84 * smooth(0.36, 0.78, n)) * m;
        px[o] = lut[Math.round(v * 255)];
      }
    }
    f.put();
  };
  Haze.prototype.draw = function (ctx, alpha) {
    var f = this.f;
    if (alpha <= 0.003) return;
    ctx.globalCompositeOperation = 'lighter';
    ctx.imageSmoothingEnabled = true;
    ctx.globalAlpha = alpha;
    ctx.drawImage(f.c, f.x, f.y, f.w, f.h);
    ctx.globalAlpha = 1;
    ctx.globalCompositeOperation = 'source-over';
  };

  // ---------- The eye ----------

  function Eye(ov, seed, rm) {
    var r = rng(seed);
    this.lk = ov.params.look || DEFAULT_LOOK.eye;
    this.pupil = pupil(ov.params.pupil, this.lk);
    this.hex = ov.params.color;
    this.col = rgb(this.hex);
    this.hot = mix(this.col, WHITE, 0.7);
    this.deep = mix(this.col, BLACK, 0.55);
    this.dark = mix(this.col, BLACK, 0.87);
    this.sp = sprites(this.hex);
    this.rm = rm;
    this.sparks = [];
    for (var i = 0; i < 130; i++) this.sparks.push({ a: r() * 6.283, r: 0.8 + r() * 1.6, d: r(), s: 0.5 + r() * 1.1, dir: r() < 0.5 ? -1 : 1 });
    this.e = new Embers(this.hex, rm);
    this.fire = new Fire(this.hex);
    this.haze = new Haze(this.hex);
    this.box = canvasOf(2, 2);
    this.g = this.box.getContext('2d');
  }
  Eye.prototype.layout = function (cx, cy, W, dpr, vw, vh) {
    this.cx = cx; this.cy = cy; this.W = W; this.dpr = dpr;
    this.bw = W * 1.75; this.bh = W * 1.45;
    this.box.width = Math.round(this.bw * dpr);
    this.box.height = Math.round(this.bh * dpr);
    this.fire.layout(cx, cy, W, vw, vh);
    this.haze.layout(cx, cy, W, vw, vh);
  };

  // shape is the outline the fire burns round at build-up u, in the eye's
  // units, with how tall its flames stand (H) and how hot they burn. Each
  // look keeps its own slow way of forming with only a low fire; the fire
  // blooms as the eye opens, surges, then settles to a burn that breathes.
  Eye.prototype.shape = function (t, u, after) {
    var lk = this.lk, open = eyeOpen(lk, u), a, b, pe = 0.8, low, bloom;
    if (lk === 'gather') {
      var lineP = smooth(0.5, 0.8, u);
      a = lerp(0.225, 0.54, lineP); b = lerp(0.225, 0.263, lineP); pe = lerp(0.5, 0.8, lineP);
      low = 0.4 * smooth(0.3, 0.58, u); bloom = smooth(0.62, 1, u);
    } else if (lk === 'almond') {
      a = 0.5; b = Math.max(0.006, 0.235 * open);
      low = 0.25 * smooth(0.04, 0.16, u); bloom = smooth(0.42, 0.92, u);
    } else {
      a = 0.5 * easeOut(smooth(0, 0.3, u)); b = Math.max(0.008, 0.235 * open);
      low = 0.6 * smooth(0, 0.08, u); bloom = smooth(0.34, 0.85, u);
    }
    var surge = smooth(0.8, 1, u) * Math.exp(-Math.max(0, after) / 1.8), e = easeOut(bloom);
    return {
      a: a, b: b, pe: pe,
      H: (0.1 + 0.42 * e) * (1 + 0.3 * surge) * (1 + 0.05 * Math.sin(t * 0.8)),
      heat: Math.min(1.15, Math.max(low, e) * (1 + 0.15 * surge))
    };
  };

  function bez(a, b, c, d, s) {
    var m = 1 - s;
    return m * m * m * a + 3 * m * m * s * b + 3 * m * s * s * c + s * s * s * d;
  }
  // The points along one lid (dir -1 top, 1 bottom) of an eye hw wide each
  // side and h open; ragged lids wander with noise.
  function lidPoints(hw, h, dir, n, rag, t, seed) {
    var pts = [], k = dir < 0 ? 1.33 : 1.12;
    for (var j = 0; j <= n; j++) {
      var s = j / n;
      var x = bez(-hw, -hw * 0.42, hw * 0.42, hw, s);
      var y = bez(0, dir * h * k, dir * h * k, 0, s);
      if (rag) y += (noise2(j * 0.9 + seed, t * 2.2) - 0.5) * rag * Math.sin(Math.PI * s);
      pts.push({ x: x, y: y, s: s });
    }
    return pts;
  }
  function trace(g, top, bottom) {
    g.beginPath();
    g.moveTo(top[0].x, top[0].y);
    for (var i = 1; i < top.length; i++) g.lineTo(top[i].x, top[i].y);
    for (var j = bottom.length - 1; j >= 0; j--) g.lineTo(bottom[j].x, bottom[j].y);
    g.closePath();
  }

  Eye.prototype.draw = function (ctx, t, u, after, dt, alpha) {
    var W = this.W, lk = this.lk, rm = this.rm;
    if (rm) { t = 3.3; u = 1; after = 4; }
    var I = lk === 'gather' ? smooth(0.2, 0.75, u) : lk === 'rift' ? smooth(0.03, 0.4, u) : smooth(0, 0.55, u);
    var beat = Math.pow(0.5 + 0.5 * Math.sin(t * 1.7), 10);
    // Under reduced motion nothing flickers: the eye glows, still.
    var flick = rm ? 0.93 : 0.86 + 0.14 * noise(t * 6.3);
    var cx = this.cx, cy = this.cy, s = this.shape(t, u, after);
    // The light the eye and its fire throw onto the dark around them,
    // wavering with the flames.
    var lit = Math.max(I * 0.6, s.heat) * (rm ? 1 : 0.88 + 0.12 * qn(t * 3.1, 17.5));
    ctx.globalCompositeOperation = 'lighter';
    stamp(ctx, this.sp.glow, cx, cy - W * 0.25 * s.heat, W * 7.5, W * 6, 0, 0.17 * lit * alpha);
    stamp(ctx, this.sp.glow, cx, cy - W * 0.02, W * 3 * (1 + 0.1 * beat), W * 2.1 * (1 + 0.1 * beat), 0, (0.3 + 0.15 * beat) * I * flick * alpha);
    ctx.globalCompositeOperation = 'source-over';
    // The fire burns behind the eye.
    if (s.heat > 0.003) {
      // The whole fire brightens and dims a little as it burns.
      if (!rm) s.heat *= 0.92 + 0.08 * qn(t * 4.7, 3.3);
      this.fire.render(t, s);
      this.fire.draw(ctx, rm ? alpha * 0.8 : alpha, 0.4);
    }
    this.paint(t, u, after, I, beat, flick);
    var box = this.box, dpr = this.dpr, X = (cx - this.bw / 2) * dpr, Y = (cy - this.bh * 0.6) * dpr;
    ctx.save();
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.globalAlpha = alpha;
    if (rm) {
      ctx.drawImage(box, X, Y);
    } else {
      // Heat shimmer: the eye is laid down in thin strips, each nudged
      // sideways, more strongly above it where the heat rises.
      var strip = Math.max(2, Math.round(3 * dpr)), amp = 1.6 * dpr * I, bwp = box.width, bhp = box.height;
      for (var y = 0; y < bhp; y += strip) {
        var yc = y / dpr, wgt = 0.3 + 0.7 * (1 - y / bhp);
        var dx = (Math.sin(yc * 0.09 - t * 7) + 0.5 * Math.sin(yc * 0.031 + t * 3.3)) * amp * wgt;
        var sh = Math.min(strip, bhp - y);
        ctx.drawImage(box, 0, y, bwp, sh, X + dx, Y + y, bwp, sh);
      }
    }
    ctx.restore();
    // A haze of heat veils the eye once it is open.
    var hz = lk === 'gather' ? smooth(0.75, 1, u) : smooth(0.3, 1, eyeOpen(lk, u));
    if (hz > 0) {
      this.haze.render(t, s);
      this.haze.draw(ctx, alpha * 0.3 * hz);
    }
    // Embers rise off the flames, drift on the hot air and fade.
    var k = Math.max(0.5, W / 420);
    var rate = lk === 'rift' && u < 0.36 ? 26 * smooth(0.05, 0.2, u) : 30 * s.heat;
    this.e.step(dt, rate * k, function () {
      if (lk === 'rift' && u < 0.36) {
        var end = (Math.random() < 0.5 ? -1 : 1) * W * 0.5 * easeOut(smooth(0, 0.3, u));
        return ember(cx + end, cy, (Math.random() - 0.5) * 140, -(40 + Math.random() * 90), 0.8 + Math.random() * 1.4, 0.8 + Math.random() * 1.4, 30);
      }
      var x = (Math.random() * 2 - 1) * s.a * 1.05, prof = Math.max(0, 1 - x * x / (s.a * s.a));
      var y = -s.b * Math.pow(prof, s.pe) - Math.random() * s.H * (0.3 + 0.7 * prof);
      return ember(cx + x * W, cy + y * W, x * 50 * k + (Math.random() - 0.5) * 30, -(30 + Math.random() * 60) * k, 1.4 + Math.random() * 2.6, 0.7 + Math.random() * 1.8, (6 + Math.random() * 10) * k);
    });
    this.e.draw(ctx, alpha);
  };

  // paint draws the eye itself into its own box, centred at (bw/2, 0.6 bh).
  Eye.prototype.paint = function (t, u, after, I, beat, flick) {
    var g = this.g, dpr = this.dpr, W = this.W, hw = W / 2, hO = W * 0.235;
    var R = W * (this.lk === 'gather' ? 0.225 : 0.2);
    var lk = this.lk, sp = this.sp, col = this.col, hot = this.hot, open = eyeOpen(lk, u);
    g.setTransform(1, 0, 0, 1, 0, 0);
    g.globalCompositeOperation = 'source-over';
    g.globalAlpha = 1;
    g.clearRect(0, 0, this.box.width, this.box.height);
    g.setTransform(dpr, 0, 0, dpr, this.bw * dpr / 2, this.bh * dpr * 0.6);
    var h = hO * open, i, top = null, bottom = null;
    if (lk !== 'gather' && open > 0.002) {
      var rag = lk === 'rift' ? W * 0.04 * Math.min(1, open * 2.5) : 0;
      top = lidPoints(hw, h, -1, 28, rag, t, 3);
      bottom = lidPoints(hw, h, 1, 28, rag, t, 41);
    }

    var ignite = smooth(0.3, 0.58, u), lineP = smooth(0.5, 0.8, u);
    var lh = hO * 1.12, lw = hw * 1.08;
    if (lk === 'gather' && lineP > 0) {
      // The eye's white is dark smoke inside the lines of light.
      var smoke = g.createRadialGradient(0, 0, R * 0.8, 0, 0, lw);
      smoke.addColorStop(0, css(this.dark, 0.95 * lineP));
      smoke.addColorStop(1, css(BLACK, 0.85 * lineP));
      g.fillStyle = smoke;
      trace(g, lidPoints(lw, lh, -1, 24, 0, t, 0), lidPoints(lw, lh, 1, 24, 0, t, 0));
      g.fill();
    }

    g.globalCompositeOperation = 'lighter';
    if (lk === 'gather') {
      // Sparks spiral in out of the dark and become the ring.
      for (i = 0; i < this.sparks.length; i++) {
        var s = this.sparks[i], v = clamp01((u - s.d * 0.32) / 0.4);
        if (v <= 0 || v >= 1) continue;
        var e = easeInOut(v), rr = lerp(W * s.r, R, e), a = s.a + (1 - e) * 2.4 * s.dir;
        var size = W * 0.03 * s.s * (0.5 + v), fa = smooth(0, 0.15, v) * (1 - smooth(0.85, 1, v));
        stamp(g, sp.ember, Math.cos(a) * rr, Math.sin(a) * rr, size, size, 0, fa);
        // A short tail behind each spark shows which way it is turning.
        for (var tl2 = 1; tl2 <= 3; tl2++) {
          var e2 = easeInOut(Math.max(0, v - tl2 * 0.025)), r2 = lerp(W * s.r, R, e2), a2 = s.a + (1 - e2) * 2.4 * s.dir;
          stamp(g, sp.ember, Math.cos(a2) * r2, Math.sin(a2) * r2, size * (1 - tl2 * 0.2), size * (1 - tl2 * 0.2), 0, fa * (0.5 - tl2 * 0.12));
        }
      }
    }
    // The seam of light before the lids part.
    if (lk === 'almond') {
      var seam = smooth(0, 0.12, u) * (1 - 0.7 * smooth(0.3, 0.55, u));
      var reach = smooth(0, 0.16, u);
      stamp(g, sp.glow, 0, 0, W * 1.2 * reach, W * 0.1, 0, seam);
      g.fillStyle = css(hot, seam);
      g.beginPath();
      g.ellipse(0, 0, Math.max(0.1, hw * reach), W * 0.0035 + h * 0.2, 0, 0, 6.283);
      g.fill();
    } else if (lk === 'rift' && u < 0.6) {
      var sl = W * easeOut(smooth(0, 0.3, u)), fade = 1 - smooth(0.4, 0.6, u);
      if (sl > 1) {
        var pts = [];
        for (i = 0; i <= 24; i++) {
          var x = -sl / 2 + sl * i / 24;
          pts.push([x, (noise2(x * 0.05, t * 3) - 0.5) * W * 0.03]);
        }
        stamp(g, sp.glow, 0, 0, sl * 1.2, W * 0.14, 0, 0.9 * fade);
        g.lineJoin = 'round';
        [[W * 0.035, css(col, 0.35 * fade)], [W * 0.009, css(hot, 0.95 * fade)]].forEach(function (st) {
          g.lineWidth = st[0];
          g.strokeStyle = st[1];
          g.beginPath();
          g.moveTo(pts[0][0], pts[0][1]);
          for (var k = 1; k < pts.length; k++) g.lineTo(pts[k][0], pts[k][1]);
          g.stroke();
        });
      }
    }

    // The eye's body.
    g.globalCompositeOperation = 'source-over';
    if (lk === 'gather') {
      if (ignite > 0) {
        g.save();
        g.globalAlpha = ignite;
        var sc = lerp(1.14, 1, easeOut(ignite));
        g.scale(sc, sc);
        g.beginPath();
        g.arc(0, 0, R, 0, 6.283);
        g.clip();
        this.iris(g, R, t, u, after, flick, beat, this.pupil, false);
        g.restore();
      }
      // The ring forming as the sparks arrive.
      g.globalCompositeOperation = 'lighter';
      g.strokeStyle = css(hot, smooth(0.25, 0.5, u) * (1 - 0.5 * ignite) * flick);
      g.lineWidth = W * 0.012;
      g.beginPath();
      g.arc(0, 0, R, 0, 6.283);
      g.stroke();
      // Lines of light draw the lids outward from the middle.
      if (lineP > 0) {
        var lt = lidPoints(lw, lh, -1, 32, 0, t, 0), lb = lidPoints(lw, lh, 1, 32, 0, t, 0);
        var from = Math.round(16 - 16 * lineP), to = Math.round(16 + 16 * lineP);
        [[W * 0.07, css(col, 0.12 * lineP)], [W * 0.026, css(col, 0.35 * lineP)], [W * 0.008, css(hot, 0.95 * lineP * flick)]].forEach(function (st) {
          g.lineWidth = st[0];
          g.strokeStyle = st[1];
          g.lineCap = 'round';
          [lt, lb].forEach(function (pts) {
            g.beginPath();
            g.moveTo(pts[from].x, pts[from].y);
            for (var k = from + 1; k <= to; k++) g.lineTo(pts[k].x, pts[k].y);
            g.stroke();
          });
        });
      }
      g.globalCompositeOperation = 'source-over';
      return;
    }
    if (!top) return;
    g.save();
    trace(g, top, bottom);
    g.clip();
    this.iris(g, R, t, u, after, flick, beat, this.pupil, true);
    // The lids shade the eye they cover.
    var sh = g.createLinearGradient(0, -h * 1.1, 0, h * 1.1);
    sh.addColorStop(0, 'rgba(0,0,0,0.8)');
    sh.addColorStop(0.32, 'rgba(0,0,0,0)');
    sh.addColorStop(0.78, 'rgba(0,0,0,0)');
    sh.addColorStop(1, 'rgba(0,0,0,0.55)');
    g.fillStyle = sh;
    g.fillRect(-hw, -h * 1.4, W, h * 2.8);
    g.restore();
    // The lid edges burn.
    g.globalCompositeOperation = 'lighter';
    g.lineJoin = 'round';
    trace(g, top, bottom);
    g.lineWidth = W * 0.032;
    g.strokeStyle = css(col, 0.32 * flick);
    g.stroke();
    g.lineWidth = W * 0.0075;
    g.strokeStyle = css(hot, 0.9 * flick);
    g.stroke();
    g.globalCompositeOperation = 'source-over';
  };

  // iris paints the burning iris and its pupil at the origin, radius R.
  Eye.prototype.iris = function (g, R, t, u, after, flick, beat, pupil, lidded) {
    var col = this.col, hot = this.hot, deep = this.deep, dark = this.dark, W = this.W, i;
    if (lidded) {
      var body = g.createRadialGradient(0, 0, R * 0.7, 0, 0, W * 0.55);
      body.addColorStop(0, css(dark, 1));
      body.addColorStop(1, 'rgba(0,0,0,1)');
      g.fillStyle = body;
      g.fillRect(-W * 0.6, -W * 0.5, W * 1.2, W);
    }
    var base = g.createRadialGradient(0, 0, 0, 0, 0, R);
    base.addColorStop(0, css(hot, 1));
    base.addColorStop(0.3, css(col, 1));
    base.addColorStop(0.68, css(deep, 1));
    base.addColorStop(0.9, css(dark, 1));
    base.addColorStop(1, 'rgba(0,0,0,1)');
    g.fillStyle = base;
    g.beginPath();
    g.arc(0, 0, R, 0, 6.283);
    g.fill();
    // Plasma: fibres that flare and fade, and currents circling the pupil.
    g.globalCompositeOperation = 'lighter';
    g.lineCap = 'round';
    // Bright knots of plasma drift round inside the iris.
    for (i = 0; i < 9; i++) {
      var oa = i * 0.7 + t * (0.25 + 0.08 * i) * (i % 2 ? 1 : -1), orr = R * (0.32 + 0.42 * noise(i * 4 + t * 0.5));
      stamp(g, this.sp.glow, Math.cos(oa) * orr, Math.sin(oa) * orr, R * 0.9, R * 0.9, 0, 0.22 * noise(i * 7 + t * 1.4) * flick);
    }
    for (i = 0; i < 90; i++) {
      var a = i / 90 * 6.283 + t * 0.04 + (hash(i) - 0.5) * 0.12;
      var n = noise2(i * 0.53, t * 1.2), r0 = R * (0.17 + 0.1 * hash(i + 3)), r1 = R * (0.4 + 0.55 * n);
      var bend = Math.sin(t * 0.8 + i * 0.7) * 0.18, mid = (r0 + r1) * 0.53;
      g.strokeStyle = css(i % 4 === 0 ? hot : col, (0.12 + 0.42 * n) * flick);
      g.lineWidth = R * (0.012 + 0.026 * hash(i + 9));
      g.beginPath();
      g.moveTo(Math.cos(a) * r0, Math.sin(a) * r0);
      g.quadraticCurveTo(Math.cos(a + bend) * mid, Math.sin(a + bend) * mid, Math.cos(a + bend * 1.6) * r1, Math.sin(a + bend * 1.6) * r1);
      g.stroke();
    }
    g.lineWidth = R * 0.035;
    for (i = 0; i < 8; i++) {
      var st = t * (0.6 + 0.25 * i) * (i % 2 ? 1 : -1) + i * 1.3;
      g.strokeStyle = css(hot, (0.12 + 0.18 * noise(i * 5 + t)) * flick);
      g.beginPath();
      g.arc(0, 0, R * (0.3 + 0.075 * i), st, st + 0.5 + noise(i * 3 + t * 0.7) * 1.4);
      g.stroke();
    }
    g.strokeStyle = css(col, 0.45 * flick);
    g.lineWidth = R * 0.06;
    g.beginPath();
    g.arc(0, 0, R * 0.96, 0, 6.283);
    g.stroke();
    g.globalCompositeOperation = 'source-over';
    if (pupil === 'slit') this.scales(g, R, t, flick);
    // The pupil, which widens as it notices you and breathes after.
    var idle = smooth(0, 2, after);
    var dil, split = 1;
    if (this.lk === 'almond') dil = keyed([[0, 0.15], [0.86, 0.15], [0.94, 1], [1, 0.85]], u) - 0.3 * smooth(0, 4, after);
    else if (this.lk === 'rift') dil = keyed([[0, 1], [0.78, 1], [1, 0.2]], u);
    else { dil = keyed([[0, 0.9], [0.82, 0.9], [1, 0.35]], u); split = smooth(0.58, 0.84, u); }
    dil += (0.1 * Math.sin(after * 0.9) + 0.08 * beat) * idle;
    // White-hot round the pupil, where the power comes from.
    g.globalCompositeOperation = 'lighter';
    stamp(g, this.sp.glow, 0, 0, R * 1.5, R * 1.5, 0, (0.45 + 0.25 * beat) * flick);
    g.globalCompositeOperation = 'source-over';
    var halo = g.createRadialGradient(0, 0, 0, 0, 0, R * 0.62);
    halo.addColorStop(0, 'rgba(0,0,0,0.6)');
    halo.addColorStop(1, 'rgba(0,0,0,0)');
    g.fillStyle = halo;
    g.fillRect(-R, -R, R * 2, R * 2);
    g.beginPath();
    if (pupil === 'round') {
      g.arc(0, 0, R * (0.13 + 0.36 * clamp01(dil)), 0, 6.283);
    } else {
      // A dragon's slit: tall, sharp at both ends, and breathing with the
      // fire, narrowing as it flares and opening as it settles.
      var pw = R * (0.035 + 0.15 * clamp01(dil)) * (0.78 + 0.44 * qn(t * 1.6, 41.3)), ph = R * 0.97 * split;
      if (ph < 0.3) return;
      g.moveTo(0, -ph);
      g.bezierCurveTo(pw * 1.55, -ph * 0.5, pw * 1.55, ph * 0.5, 0, ph);
      g.bezierCurveTo(-pw * 1.55, ph * 0.5, -pw * 1.55, -ph * 0.5, 0, -ph);
    }
    g.fillStyle = '#000';
    g.fill();
    g.globalCompositeOperation = 'lighter';
    g.strokeStyle = css(hot, 0.5 * flick);
    g.lineWidth = R * 0.025;
    g.stroke();
    stamp(g, this.sp.ember, -R * 0.34, -R * 0.38, R * 0.32, R * 0.26, 0, 0.5);
    g.globalCompositeOperation = 'source-over';
  };

  // scales lays a reptile's iris over the plasma: a hot seam of light down
  // the middle where the slit opens, dark veins that branch out across it,
  // a crazing of scale-like cells round the rim, and flecks that glint.
  // Fixed per eye, so it reads as anatomy rather than flicker.
  Eye.prototype.scales = function (g, R, t, flick) {
    var col = this.col, hot = this.hot, dark = this.dark, i, k;
    g.globalCompositeOperation = 'lighter';
    stamp(g, this.sp.glow, 0, 0, R * 0.8, R * 2.1, 0, 0.32 * flick);
    g.globalCompositeOperation = 'source-over';
    g.lineCap = 'round';
    g.lineJoin = 'round';
    g.strokeStyle = css(dark, 0.5);
    for (i = 0; i < 26; i++) {
      var a = i / 26 * 6.283 + (hash(i + 70) - 0.5) * 0.2, r = R * (0.3 + 0.08 * hash(i + 71));
      var x = Math.cos(a) * r, y = Math.sin(a) * r;
      g.lineWidth = R * (0.009 + 0.011 * hash(i + 72));
      g.beginPath();
      g.moveTo(x, y);
      for (k = 1; k <= 5; k++) {
        a += (hash(i * 7 + k) - 0.5) * 0.5;
        r += R * 0.12;
        x = Math.cos(a) * r; y = Math.sin(a) * r;
        g.lineTo(x, y);
        // Every other vein forks once on its way out.
        if (k === 3 && i % 2) {
          var fa = a + (hash(i + k) < 0.5 ? -0.35 : 0.35);
          g.moveTo(x, y);
          g.lineTo(Math.cos(fa) * (r + R * 0.16), Math.sin(fa) * (r + R * 0.16));
          g.moveTo(x, y);
        }
      }
      g.stroke();
    }
    // Scale-like cells: short arcs in two staggered rings near the rim.
    g.lineWidth = R * 0.016;
    g.strokeStyle = css(dark, 0.5);
    for (k = 0; k < 2; k++) {
      var n = 22 + k * 6, rr = R * (0.74 + 0.12 * k);
      for (i = 0; i < n; i++) {
        var s0 = (i + k * 0.5) / n * 6.283;
        g.beginPath();
        g.arc(Math.cos(s0) * rr * 0.12, Math.sin(s0) * rr * 0.12, rr * 0.9, s0 - 0.11, s0 + 0.11);
        g.stroke();
      }
    }
    g.globalCompositeOperation = 'lighter';
    for (i = 0; i < 14; i++) {
      var fa2 = hash(i + 90) * 6.283, fr = R * (0.38 + 0.5 * hash(i + 91)), tw = 0.5 + 0.5 * Math.sin(t * (1.1 + hash(i + 92)) + i);
      stamp(g, this.sp.ember, Math.cos(fa2) * fr, Math.sin(fa2) * fr, R * 0.09, R * 0.09, 0, 0.35 * tw * flick);
    }
    g.strokeStyle = css(mix(col, hot, 0.5), 0.35 * flick);
    g.lineWidth = R * 0.02;
    g.beginPath();
    g.arc(0, 0, R * 0.99, 0, 6.283);
    g.stroke();
    g.globalCompositeOperation = 'source-over';
  };

  // ---------- Words ----------

  // Lines are plain text nodes: a scene's words are never read as markup.
  // boxes holds one container per layout; a line goes to its layout's box,
  // in scene order among the lines already there.
  function Words(boxes, scene, still) {
    this.boxes = boxes;
    this.still = !!still;
    this.lines = scene.text.map(function (t) { return { spec: t, box: boxes[t.layout || 'middle'] || boxes.middle, node: null, gone: false }; });
  }
  // Where each shown line sits, by layout: its box's place on screen plus
  // its offset in the box, so the line's own arrival transform is left out.
  Words.prototype.places = function () {
    var at = [], tops = [];
    for (var i = 0; i < this.lines.length; i++) {
      var l = this.lines[i];
      if (!l.node) continue;
      var k = tops.indexOf(l.box);
      if (k < 0) { tops.push(l.box, l.box.getBoundingClientRect().top); k = tops.length - 2; }
      at[i] = tops[k + 1] + l.node.offsetTop;
    }
    return at;
  };
  // A box centres its lines as a group, so a line joining or leaving moves
  // the others. They glide to their new place rather than jump, on top of
  // their own arrival; under reduced motion they are simply there.
  Words.prototype.glide = function (before) {
    if (this.still || !before) return;
    var after = this.places();
    for (var i = 0; i < this.lines.length; i++) {
      var l = this.lines[i], dy = before[i] - after[i];
      if (!l.node || before[i] == null || Math.abs(dy) < 1 || !l.node.animate) continue;
      try {
        l.node.animate([{ transform: 'translateY(' + dy + 'px)' }, { transform: 'translateY(0)' }],
          { duration: 280, easing: 'cubic-bezier(.2,.7,.2,1)', composite: 'add' });
      } catch (e) { /* no Web Animations: the line keeps its old jump */ }
    }
  };
  Words.prototype.update = function (act) {
    var on = {}, i, before = null;
    for (i = 0; i < act.length; i++) if (act[i].type === 'text') on[act[i].index] = act[i];
    for (i = 0; i < this.lines.length; i++) {
      var c = this.lines[i];
      if ((on[i] && !c.node && !c.gone) || (!on[i] && c.node && c.gone)) { before = this.places(); break; }
    }
    for (i = 0; i < this.lines.length; i++) {
      var l = this.lines[i], a = on[i];
      if (a && !l.node && !l.gone) {
        l.node = this.make(l.spec);
        var next = null;
        for (var j = i + 1; j < this.lines.length; j++) if (this.lines[j].node && this.lines[j].box === l.box) { next = this.lines[j].node; break; }
        l.box.insertBefore(l.node, next);
      }
      if (a && l.node && a.leaving > 0 && !l.gone) {
        l.gone = true;
        l.node.className += ' cs-cut-gone' + (l.spec.out ? ' cs-cut-out-' + l.spec.out : '');
      }
      if (!a && l.node && l.gone) {
        l.box.removeChild(l.node);
        l.node = null;
      }
    }
    this.glide(before);
  };
  Words.prototype.make = function (spec) {
    var p = document.createElement('p');
    p.className = 'cs-cut-line cs-cut-' + spec.style + (spec['in'] ? ' cs-cut-in-' + spec['in'] : '');
    if (spec.style === 'whisper' && !spec['in']) {
      // A whisper's own arrival is a word at a time.
      spec.say.split(' ').forEach(function (w, k) {
        if (k) p.appendChild(document.createTextNode(' '));
        var s = document.createElement('span');
        s.className = 'cs-cut-w';
        s.style.animationDelay = (k * 0.12) + 's';
        s.textContent = w;
        p.appendChild(s);
      });
    } else {
      p.textContent = spec.say;
    }
    return p;
  };

  // ---------- Playing ----------

  var CLOSE_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg>';

  function backdropOK() {
    try {
      return !!(window.CSS && CSS.supports && (CSS.supports('backdrop-filter', 'blur(1px)') || CSS.supports('-webkit-backdrop-filter', 'blur(1px)')));
    } catch (e) { return false; }
  }

  function play(host, raw, opts) {
    var doc = document;
    host = host || doc.body;
    injectStyle(doc);
    var tl = timeline(normalise(raw)), scene = tl.scene;
    var rm = opts.reducedMotion != null ? !!opts.reducedMotion : prefersReduced();
    var boxed = !!opts.contained;
    var seed = seedOf(scene.id + '|' + scene.name);
    var onClose = typeof opts.onClose === 'function' ? opts.onClose : function () {};

    var el = doc.createElement('div');
    el.className = 'cs-cut' + (boxed ? ' cs-cut-boxed' : '') + (rm ? ' cs-cut-calm' : '');
    el.setAttribute('role', 'dialog');
    if (!boxed) el.setAttribute('aria-modal', 'true');
    el.setAttribute('aria-label', opts.label || scene.name || 'Cut scene');
    el.tabIndex = -1;
    var tint = null;
    scene.overlays.forEach(function (o) { if (!tint && o.kind === 'eye') tint = rgb(o.params.color); });
    if (tint) {
      el.style.setProperty('--cs-cut-ink', css(mix(tint, WHITE, 0.8)));
      el.style.setProperty('--cs-cut-haze', css(tint, 0.4));
    }
    var veil = null;
    if (scene.background.kind === 'fall-asleep' && scene.background.params.look === 'drift' && !rm && backdropOK()) {
      veil = doc.createElement('div');
      veil.className = 'cs-cut-veil';
      el.appendChild(veil);
    }
    var canvas = doc.createElement('canvas');
    canvas.className = 'cs-cut-canvas';
    canvas.setAttribute('aria-hidden', 'true');
    el.appendChild(canvas);
    var grainEl = null;
    if (!rm && grain()) {
      grainEl = doc.createElement('div');
      grainEl.className = 'cs-cut-grain';
      grainEl.setAttribute('aria-hidden', 'true');
      grainEl.style.backgroundImage = 'url(' + grain() + ')';
      el.appendChild(grainEl);
    }
    var wordsEl = doc.createElement('div');
    wordsEl.className = 'cs-cut-words';
    wordsEl.setAttribute('aria-live', 'polite');
    el.appendChild(wordsEl);
    var cornerEl = doc.createElement('div');
    cornerEl.className = 'cs-cut-words cs-cut-corner';
    cornerEl.setAttribute('aria-live', 'polite');
    el.appendChild(cornerEl);
    var xBtn = doc.createElement('button');
    xBtn.type = 'button';
    xBtn.className = 'cs-cut-x';
    xBtn.setAttribute('aria-label', 'Close');
    xBtn.innerHTML = CLOSE_SVG;
    el.appendChild(xBtn);
    var endBtn = doc.createElement('button');
    endBtn.type = 'button';
    endBtn.className = 'cs-cut-end';
    endBtn.textContent = 'Close';
    el.appendChild(endBtn);
    host.appendChild(el);

    var ctx = canvas.getContext('2d');
    var bg = new Background(scene.background, seed, rm, veil);
    var layers = scene.overlays.map(function (o, i) {
      if (o.kind === 'eye') return new Eye(o, seed + i + 1, rm);
      return o.kind === 'letterbox' ? new Letterbox(rm) : new EmberLayer(o, rm);
    });
    var words = new Words({ middle: wordsEl, corner: cornerEl }, scene, rm);
    var hasEye = scene.overlays.some(function (o) { return o.kind === 'eye'; });
    var w = 0, h = 0, dpr = 1;

    function layout() {
      w = el.clientWidth || window.innerWidth;
      h = el.clientHeight || window.innerHeight;
      dpr = Math.min(2, window.devicePixelRatio || 1);
      canvas.width = Math.max(1, Math.round(w * dpr));
      canvas.height = Math.max(1, Math.round(h * dpr));
      bg.resize(w, h);
      var W = Math.min(w * 0.72, h * 0.5, 560);
      // The eye makes room only for words in the middle; a corner card sits clear of it.
      var cy = hasEye && scene.text.some(function (x) { return !x.layout; }) ? h * 0.4 : h * 0.47;
      layers.forEach(function (l) { if (l instanceof Eye) l.layout(w / 2, cy, W, dpr, w, h); else l.layout(w, h); });
      if (hasEye) {
        wordsEl.classList.remove('cs-cut-mid');
        wordsEl.style.top = Math.round(cy + W * 0.235 * 1.3 + Math.max(26, h * 0.05)) + 'px';
      } else {
        wordsEl.classList.add('cs-cut-mid');
        wordsEl.style.top = '';
      }
    }

    var t = 0, prev = null, raf = 0, done = false, resting = false, grainOn = false;
    var xOn = false, endOn = false, sized = false;
    var prevFocus = boxed ? null : doc.activeElement;

    function draw(dt) {
      if (!sized) { layout(); sized = true; }
      var act = activeAt(tl, t);
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      ctx.globalCompositeOperation = 'source-over';
      ctx.globalAlpha = 1;
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      var bgp = 0;
      for (var i = 0; i < act.length; i++) {
        var a = act[i];
        if (a.type === 'background') { bgp = a.progress; bg.draw(ctx, t, a.progress); }
      }
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      for (i = 0; i < act.length; i++) {
        if (act[i].type !== 'overlay') continue;
        var ov = scene.overlays[act[i].index], since = t - ov.at - ov.seconds;
        // Under reduced motion a layer simply fades in over its build-up.
        layers[act[i].index].draw(ctx, t, act[i].progress, since, dt, rm ? smooth(0, 1, act[i].progress) : 1);
      }
      words.update(act);
      if (grainEl && !grainOn && (bgp > 0.3 || scene.background.kind === 'black')) { grainOn = true; grainEl.style.opacity = '0.07'; }
      var en = ending(tl, t);
      if (en.canClose && !xOn) { xOn = true; xBtn.classList.add('cs-cut-on'); }
      if (en.showClose && !endOn) {
        endOn = true;
        endBtn.classList.add('cs-cut-on');
        el.classList.add('cs-cut-tapclose');
        if (!boxed && doc.activeElement === el) endBtn.focus({ preventScroll: true });
      }
      if (en.autoClose) { close('auto'); return; }
      // Long after everything has arrived the last frame holds still, so an
      // unattended screen stops drawing.
      if (t > tl.last + 60) resting = true;
    }

    function frame(now) {
      raf = 0;
      if (done) return;
      var dt = prev === null ? 0 : Math.min(0.1, (now - prev) / 1000);
      prev = now;
      t += dt;
      draw(dt);
      if (!done && !resting) raf = window.requestAnimationFrame(frame);
    }

    var relayout = 0;
    function onResize() {
      if (relayout) return;
      relayout = window.requestAnimationFrame(function () {
        relayout = 0;
        if (done) return;
        layout();
        if (resting) draw(0);
      });
    }

    function onKey(ev) {
      if (done) return;
      if (boxed && !el.contains(ev.target)) return;
      if (ev.key === 'Escape' || ev.key === 'Esc') {
        ev.preventDefault();
        ev.stopPropagation();
        if (t >= ESCAPE_AFTER) close('escape');
        return;
      }
      if (ev.key === 'Tab' && !boxed) {
        // Focus stays inside the scene while it is up.
        var can = [xBtn, endBtn].filter(function (b) { return b.classList.contains('cs-cut-on'); });
        ev.preventDefault();
        if (!can.length) { el.focus({ preventScroll: true }); return; }
        var at = can.indexOf(doc.activeElement);
        var next = ev.shiftKey ? (at <= 0 ? can.length - 1 : at - 1) : (at + 1) % can.length;
        can[next].focus({ preventScroll: true });
        return;
      }
      // Page shortcuts never fire under the scene.
      if (el.contains(ev.target)) ev.stopPropagation();
    }

    function onClick(ev) {
      if (done) return;
      var tgt = ev.target;
      if (xBtn.contains(tgt)) { if (t >= ESCAPE_AFTER) close('escape'); return; }
      if (endBtn.contains(tgt) || ending(tl, t).tapCloses) close('tap');
    }

    // While the scene covers the window the page under it is out of reach:
    // inert for the keyboard and screen readers, and its shortcuts stand down.
    var inerted = [], marked = false;
    function coverPage() {
      if (boxed) return;
      var html = doc.documentElement;
      if (!html.hasAttribute('data-full-surface')) { html.setAttribute('data-full-surface', ''); marked = true; }
      if (host !== doc.body) return;
      Array.prototype.forEach.call(doc.body.children, function (c) {
        if (c === el || c.tagName === 'SCRIPT' || c.tagName === 'STYLE' || c.hasAttribute('inert') || /(^| )cs-cut( |$)/.test(c.className)) return;
        c.setAttribute('inert', '');
        inerted.push(c);
      });
    }
    function uncoverPage() {
      inerted.forEach(function (c) { c.removeAttribute('inert'); });
      inerted = [];
      if (marked) doc.documentElement.removeAttribute('data-full-surface');
    }

    var ro = null;
    function close(why) {
      if (done) return;
      done = true;
      if (raf) window.cancelAnimationFrame(raf);
      if (relayout) window.cancelAnimationFrame(relayout);
      doc.removeEventListener('keydown', onKey, true);
      window.removeEventListener('resize', onResize);
      window.removeEventListener('orientationchange', onResize);
      if (ro) ro.disconnect();
      uncoverPage();
      el.classList.add('cs-cut-leave');
      if (why === 'stopped') el.classList.add('cs-cut-quick');
      setTimeout(function () { if (el.parentNode) el.parentNode.removeChild(el); }, why === 'stopped' ? 300 : 560);
      if (prevFocus && prevFocus.focus && doc.contains(prevFocus)) {
        try { prevFocus.focus({ preventScroll: true }); } catch (e) { /* the page moved on */ }
      }
      try { onClose(why); } catch (e) { /* the caller's problem, not the scene's */ }
    }

    doc.addEventListener('keydown', onKey, true);
    el.addEventListener('click', onClick);
    window.addEventListener('resize', onResize);
    window.addEventListener('orientationchange', onResize);
    if (window.ResizeObserver) { ro = new ResizeObserver(onResize); ro.observe(el); }
    coverPage();
    if (!boxed) el.focus({ preventScroll: true });
    layout();
    sized = true;
    draw(0);
    raf = window.requestAnimationFrame(frame);

    return {
      stop: function () { close('stopped'); },
      time: function () { return t; },
      element: el
    };
  }

  return play;
  })();

  var api = {
    play: function (host, scene, opts) { return play(host, scene, opts || {}); },
    kinds: {
      background: KINDS.background.slice(), arrivals: ARRIVALS.slice(), overlays: KINDS.overlays.slice(), textStyles: KINDS.textStyles.slice(),
      layouts: TEXT.layouts.slice(), textArrivals: TEXT.arrivals.slice(), textLeavings: TEXT.leavings.slice()
    },
    looks: LOOKS,
    pupils: PUPILS,
    _pure: pure
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (root) root.ChronicleCutscene = api;
})(typeof window !== 'undefined' ? window : null);
