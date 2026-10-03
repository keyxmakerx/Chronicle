/*
 * sky_gl.js — the painted sky (window.SkyGL): the WebGL2 painter the signed
 * sky pane mockup (#763, #842) draws with, ported from it. One WebGL2
 * context on one hidden canvas paints every sky on the page in a single
 * full-screen pass (sky_shaders.js) and each sky copies its picture out, so
 * the page never runs out of contexts. Moons come in as MOONR sprites in one
 * atlas, the drawn layers (the bleeding blood moon, the thread between
 * conjunct moons, shapes, particles) as two textures, the land as a small
 * texture of ridge heights.
 *
 * The basic 2D sky (sky_2d.js) stays the fallback: SkyGL.paint returns false
 * without WebGL2, while the context is lost or the program is compiling,
 * after a failed compile, and for the rest of the session on a device that
 * paints the moving sky too slowly.
 */
(function () {
  'use strict';

  var SW = window.SkyWorld, LOOKS = window.SkyLooks, PAL = SW.PAL, clamp = SW.clamp, smooth01 = SW.smooth01, mod = SW.mod, D2R = SW.D2R;
  function hashN(n){ var x = Math.sin(n * 127.1 + 311.7) * 43758.5453; return x - Math.floor(x); }
  var MOONR = window.MOONR;

  /* ── The painting engine. Every sky on the page is painted by one WebGL2 context on one hidden canvas, in one
     full-screen pass, and copied into that sky's own canvas; so the page never runs out of contexts and a lost context is
     one thing to restore. The moons come in as one texture of sprites from the moon view's renderer, the drawn layers
     (moon effects, shapes, particles) as two more, and the land as a small texture of ridge heights. A failed compile is
     an error in the console, never a silent blank. ── */
  var GLX = (function(){
    var VS = '#version 300 es\nvoid main(){ vec2 p = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2)); gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0); }';
    /* A program is compiled and linked without waiting on it; finish() checks it and reads its uniforms once the driver
       is done, so the page is never held up by a compile it does not need yet. */
    function program(gl, fs, label){
      var p = gl.createProgram(), v = gl.createShader(gl.VERTEX_SHADER), f = gl.createShader(gl.FRAGMENT_SHADER);
      gl.shaderSource(v, VS); gl.compileShader(v); gl.shaderSource(f, fs); gl.compileShader(f);
      gl.attachShader(p, v); gl.attachShader(p, f); gl.linkProgram(p);
      return {p:p, v:v, f:f, src:fs, label:label, U:null};
    }
    function finish(gl, P){
      if (P.U) return;
      if (!gl.getProgramParameter(P.p, gl.LINK_STATUS) && !gl.isContextLost()){
        [[P.v, VS, ' (vertex)'], [P.f, P.src, '']].forEach(function(q){
          if (gl.getShaderParameter(q[0], gl.COMPILE_STATUS)) return;
          var log = gl.getShaderInfoLog(q[0]) || '', lines = q[1].split('\n'), m = /ERROR: \d+:(\d+)/.exec(log);
          throw new Error('Sky shader "' + P.label + q[2] + '" failed: ' + log + (m ? ' @ ' + lines[+m[1] - 1] : ''));
        });
        throw new Error('Sky program "' + P.label + '" failed to link: ' + gl.getProgramInfoLog(P.p));
      }
      var U = {}, n = gl.getProgramParameter(P.p, gl.ACTIVE_UNIFORMS);
      for (var i = 0; i < n; i++){ var info = gl.getActiveUniform(P.p, i), nm = info.name.replace(/\[0\]$/, ''); U[nm] = {loc:gl.getUniformLocation(P.p, info.name), type:info.type, size:info.size}; }
      P.U = U;
    }
    function setU(gl, P, name, v){
      var u = P.U[name]; if (!u) return;
      var t = u.type, a = typeof v === 'number' ? [v] : v;
      if (t === gl.FLOAT) gl.uniform1fv(u.loc, a);
      else if (t === gl.FLOAT_VEC2) gl.uniform2fv(u.loc, a);
      else if (t === gl.FLOAT_VEC3) gl.uniform3fv(u.loc, a);
      else if (t === gl.FLOAT_VEC4) gl.uniform4fv(u.loc, a);
      else if (t === gl.INT || t === gl.SAMPLER_2D) gl.uniform1iv(u.loc, a.map(function(x){ return x | 0; }));
    }
    function tex(gl, filter){
      var t = gl.createTexture(); gl.bindTexture(gl.TEXTURE_2D, t);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, filter); gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filter);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE); gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array(4));
      return t;
    }
    var SRC = {};
    function source(dir){ return SRC[dir] || (SRC[dir] = window.SkyShaders.common + '\n' + window.SkyShaders[dir]); }

    /* The painter on its canvas. dir: 'painted'. */
    function create(canvas, dir){
      /* failIfMajorPerformanceCaveat: a browser that would paint in software gives no context, and the basic sky draws
       instead. SkyGL.allowSoftware lifts that, for the browser checks only. */
    var gl = canvas.getContext('webgl2', {alpha:true, premultipliedAlpha:true, antialias:false, depth:false, stencil:false, preserveDrawingBuffer:false, powerPreference:'low-power', failIfMajorPerformanceCaveat:!window.SkyGLAllowSoftware});
      if (!gl) throw new Error('This sky needs WebGL2, which this browser did not provide.');
      var S = {gl:gl, dir:dir, canvas:canvas, slots:{}, slotPx:0, lost:false, warm:false, par:gl.getExtension('KHR_parallel_shader_compile')};
      function build(){
        S.P = program(gl, source(dir), dir); S.warm = false;
        S.atlas = tex(gl, gl.LINEAR); S.fx = tex(gl, gl.NEAREST); S.near = tex(gl, gl.NEAREST); S.slots = {}; S.slotPx = 0;
        S.land = tex(gl, gl.NEAREST); S.landKey = '';
        gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, true);
      }
      build();
      /* Canvases come in premultiplied; raw arrays are uploaded as they are, which Firefox requires of them. */
      function raw(up){ gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false); up(); gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, true); }
      canvas.addEventListener('webglcontextlost', function(e){ e.preventDefault(); S.lost = true; });
      canvas.addEventListener('webglcontextrestored', function(){ S.lost = false; build(); S.onRestore && S.onRestore(); });

      /* Moon sprites share one texture, a slot each. Sizes are rounded up to steps of 8 device pixels, so a sky that is
         growing reuses a few prepared sizes instead of preparing one per frame. */
      S.moonSlot = function(i, key, spriteFn, px){
        var need = Math.max(8, Math.ceil(px / 8) * 8);
        if (need > S.slotPx){
          S.slotPx = need; S.slots = {};
          gl.bindTexture(gl.TEXTURE_2D, S.atlas);
          /* Allocated as zeros, not left empty: Firefox warns and clears an empty texture itself before the first slot
             is written into it. */
          raw(function(){ gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, (need + 2) * 4, need + 2, 0, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array((need + 2) * 4 * (need + 2) * 4)); });
        }
        var k = key + ':' + need;
        if (S.slots[i] !== k){
          var cv = spriteFn(need), sp = S.slotPx + 2;
          gl.bindTexture(gl.TEXTURE_2D, S.atlas);
          /* The slot is cleared first: a smaller moon in a slot a larger one used leaves nothing of it at its edge. */
          if (!S.zero || S.zero.length !== sp * sp * 4) S.zero = new Uint8Array(sp * sp * 4);
          raw(function(){ gl.texSubImage2D(gl.TEXTURE_2D, 0, i * sp, 0, sp, sp, gl.RGBA, gl.UNSIGNED_BYTE, S.zero); });
          gl.texSubImage2D(gl.TEXTURE_2D, 0, i * sp + 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, cv);
          S.slots[i] = k;
        }
        var W = (S.slotPx + 2) * 4, H = S.slotPx + 2;
        return [(i * (S.slotPx + 2) + 1) / W, 1 / H, need / W, need / H];
      };
      S.fxUpload = function(cv){ gl.bindTexture(gl.TEXTURE_2D, S.fx); gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, cv); };
      S.nearUpload = function(cv){ gl.bindTexture(gl.TEXTURE_2D, S.near); gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, cv); };
      /* The land's ridge heights: two rows (tops, bases) of four layers, in full float, read with exact interpolation. */
      S.landUpload = function(key, data, w){
        if (S.landKey === key) return;
        gl.bindTexture(gl.TEXTURE_2D, S.land);
        raw(function(){ gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, w, 2, 0, gl.RGBA, gl.FLOAT, data); });
        S.landKey = key;
      };

      /* Ready to paint: the program built (asking the browser without waiting, where it can say) and run once on a single
         pixel, since some drivers only finish a program the first time it draws. */
      S.ready = function(noWait){
        if (S.warm) return true;
        if (S.lost || gl.isContextLost()) return false;
        if (noWait && S.par && !gl.getProgramParameter(S.P.p, S.par.COMPLETION_STATUS_KHR)) return false;
        finish(gl, S.P);
        gl.viewport(0, 0, 1, 1); gl.useProgram(S.P.p); gl.drawArrays(gl.TRIANGLES, 0, 3); gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array(4));
        S.warm = true;
        return true;
      };
      /* One sky, painted into the lower-left corner of the shared canvas, which only ever grows; the caller copies that
         corner out at once. */
      S.draw = function(uni, dpr, cssW, cssH){
        if (S.lost || gl.isContextLost() || !S.warm) return false;
        var w = Math.max(1, Math.round(cssW * dpr)), h = Math.max(1, Math.round(cssH * dpr));
        if (canvas.width < w || canvas.height < h){ canvas.width = Math.max(canvas.width, w); canvas.height = Math.max(canvas.height, h); }
        gl.viewport(0, 0, w, h);
        gl.useProgram(S.P.p);
        setU(gl, S.P, 'uRes', [w, h]); setU(gl, S.P, 'uDpr', w / cssW); setU(gl, S.P, 'uSize', [cssW, cssH]);
        gl.activeTexture(gl.TEXTURE0); gl.bindTexture(gl.TEXTURE_2D, S.atlas); setU(gl, S.P, 'uAtlas', 0);
        gl.activeTexture(gl.TEXTURE1); gl.bindTexture(gl.TEXTURE_2D, S.fx); setU(gl, S.P, 'uFx', 1);
        gl.activeTexture(gl.TEXTURE2); gl.bindTexture(gl.TEXTURE_2D, S.land); setU(gl, S.P, 'uLand', 2);
        gl.activeTexture(gl.TEXTURE3); gl.bindTexture(gl.TEXTURE_2D, S.near); setU(gl, S.P, 'uNear', 3);
        for (var k in uni) setU(gl, S.P, k, uni[k]);
        gl.disable(gl.BLEND);
        gl.drawArrays(gl.TRIANGLES, 0, 3);
        S.w = w; S.h = h;
        return true;
      };
      return S;
    }
    return {create:create};
  })();

  /* ── The land: the four ridges' heights, worked out once per size on the page (the shader's own hashes, run the same
     way) and handed to the painter as a small texture; the 2D sky draws the same ridges from them. ── */
  var LAND = (function(){
    function pcg(v){ var s = (Math.imul(v >>> 0, 747796405) + 2891336453) >>> 0; var w = Math.imul(((s >>> ((s >>> 28) + 4)) ^ s) >>> 0, 277803737) >>> 0; return ((w >>> 22) ^ w) >>> 0; }
    function h1(x){ return pcg((Math.imul((Math.floor(x) + (1 << 20)) >>> 0, 2654435761) + 7) >>> 0) / 4294967295; }
    function n1(x){ var i = Math.floor(x), f = x - i, u = f * f * f * (f * (f * 6 - 15) + 10); return h1(i) + (h1(i + 1) - h1(i)) * u; }
    function sm(a, b, x){ var t = clamp((x - a) / (b - a), 0, 1); return t * t * (3 - 2 * t); }
    function trees(x, W, H){
      var sp = Math.max(3.2, H * .036), k = Math.floor(x / sp), best = 0;
      for (var j = -2; j <= 2; j++){
        var kk = k + j, clump = n1(kk * sp / W * 9 + 4);
        if (clump < .42) continue;
        var cx = (kk + .2 + .6 * h1(kk * 1.31 + 7)) * sp, r = sp * (.75 + .7 * h1(kk * 2.17 + 3)) * Math.pow(sm(.42, .72, clump), 1.4), dx = x - cx;
        if (Math.abs(dx) < r) best = Math.max(best, (Math.sqrt(r * r - dx * dx) - r * .28) * 1.25);
      }
      return best;
    }
    function base(i, x, W, H){
      var u = x / W, r;
      if (i === 0){ r = .55 * n1(u * 6 + 1.3) + .3 * n1(u * 13 + 4.1) + .15 * n1(u * 29 + 2.2); return H * (.8 - .2 * Math.pow(r, 1.35)); }
      if (i === 1){ r = .7 * n1(u * 3.4 + 7.1) + .3 * n1(u * 8 + 3.3); return H * (.9 - .15 * r); }
      if (i === 2){ r = .75 * n1(u * 2.5 + 3.7) + .25 * n1(u * 6.5 + 9.1); return H * (.955 - .12 * r); }
      r = .7 * n1(u * 1.8 + 9.9) + .3 * n1(u * 5 + 6.6);
      return H * (1.03 - .08 * r);
    }
    var CACHE = [], made = 0;
    /* Tops (with the tree crowns) and bases for every column, as a share of the height. */
    function get(W, H, dpr){
      var key = W.toFixed(2) + 'x' + H.toFixed(2) + '@' + dpr;
      for (var c = 0; c < CACHE.length; c++) if (CACHE[c].key === key) return CACHE[c];
      var w = Math.max(2, Math.round(W * dpr)), data = new Float32Array(w * 2 * 4);
      for (var j = 0; j < w; j++){
        var x = (j + .5) / w * W;
        for (var i = 0; i < 4; i++){ var b = base(i, x, W, H); data[j * 4 + i] = (b - (i === 2 ? trees(x, W, H) : 0)) / H; data[(w + j) * 4 + i] = b / H; }
      }
      var out = {key:key, data:data, w:w, W:W, H:H};
      CACHE.unshift(out); if (CACHE.length > 8) CACHE.pop(); made++;
      return out;
    }
    return {get:get, base:base, trees:trees, made:function(){ return made; }};
  })();

  /* Lightning is a plan made from a seed, so any moment of it can be drawn again exactly: a live frame, a filmed frame
     and a still one agree. A strike flickers twice at most, strikes are seconds apart, and only the bolt and the cloud
     round it light up, so a storm never flashes more than three times a second or over a large area. */
  var PLANS = {bolt:[]};
  /* A bolt shaped like a photographed one: a main channel that wanders sharply in short kinks, with
     forks that leave it at narrow angles, run down and outward, and die out in the air; a fork may fork again. The
     main channel is laid first, then the forks, so the painter can thin them by their place in the list. Its x is
     measured from the strike point in the same unit as its height, so its kinks keep their shape on any width. */
  function jag(rnd, a, b, levels, rough){
    var pts = [a, b];
    for (var lv = 0; lv < levels; lv++){
      var nx = [pts[0]];
      for (var i = 0; i < pts.length - 1; i++){
        var p = pts[i], q = pts[i + 1], dy = q[1] - p[1];
        nx.push([(p[0] + q[0]) / 2 + (rnd() - .5) * dy * rough * (lv ? 1 : .3), (p[1] + q[1]) / 2 + (rnd() - .5) * dy * .15], q);
      }
      pts = nx;
    }
    return pts;
  }
  function boltShape(rnd, seg, br){
    var main = jag(rnd, [0, .26 + rnd() * .08], [(rnd() - .5) * .3, 1.0], 4, .68);
    for (var i = 0; i < main.length - 1; i++) seg.push(main[i].concat(main[i + 1]));
    var forks = 2 + Math.floor(rnd() * 3), left = 24;
    for (var f = 0; f < forks && left > 1; f++){
      var at = main[2 + Math.floor(rnd() * (main.length * .55))], dir = rnd() < .5 ? -1 : 1, len = .12 + rnd() * .22;
      var end = [at[0] + dir * len * (.35 + rnd() * .5), at[1] + len], pts = jag(rnd, at, end, 3, .5);
      for (i = 0; i < pts.length - 1 && left > 0; i++, left--) br.push(pts[i].concat(pts[i + 1]));
      if (left > 3 && rnd() < .6){
        var s0 = pts[2 + Math.floor(rnd() * 3)], e2 = [s0[0] + dir * (.04 + rnd() * .08), s0[1] + .05 + rnd() * .07], p2 = jag(rnd, s0, e2, 2, .5);
        for (i = 0; i < p2.length - 1 && left > 0; i++, left--) br.push(p2[i].concat(p2[i + 1]));
      }
    }
  }
  function boltPlan(k){
    var P = PLANS.bolt;
    while (P.length <= k){
      var j = P.length, prev = P[j - 1], t0 = prev ? prev.t0 + 1.2 + hashN(j * 3.1 + .4) * 3.0 : 1.2;
      var x0 = .12 + hashN(j * 5.7 + 1) * .76, seg = [], br = [], rnd = MOONR.rng(900 + j);
      var hasBolt = hashN(j * 2.3 + 9) < .82;
      if (hasBolt) boltShape(rnd, seg, br);
      P.push({t0:t0, x:x0, seg:seg, br:br, bolt:hasBolt, pulses:[[0, 1], [.2 + hashN(j) * .1, .55 + hashN(j + .5) * .25]]});
    }
    return P[k];
  }
  function flashAt(t){
    var out = {k:0, s:0, x:.5, strike:null};
    for (var k = 0; ; k++){
      var b = boltPlan(k); if (b.t0 > t) break;
      var u = t - b.t0; if (u > 1.4) continue;
      var s = 0;
      b.pulses.forEach(function(pl){ var d = u - pl[0]; if (d >= 0) s += pl[1] * Math.exp(-d / .075); });
      if (s > out.s){ out.s = s; out.x = b.x; out.strike = b; out.u = u; }
    }
    return out;
  }

  /* ── The drawn layer, laid into the sky right after the moons, so hills, clouds and rain still pass in front of it:
     moons beyond the painter's four slots, the blood moon's bleeding, the thread of light between full moons (the
     calendar's moon view's own), and the look's drawn shapes and sigils (SkyFX). ── */
  var FX = (function(){
    var TAU = Math.PI * 2;
    function hash(n){ var x = Math.sin(n * 127.1 + 311.7) * 43758.5453; return x - Math.floor(x); }
    function hexOklch(hex){
      var n = parseInt(hex.slice(1), 16), c = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map(function(v){ v /= 255; return v <= .04045 ? v / 12.92 : Math.pow((v + .055) / 1.055, 2.4); });
      var l = Math.cbrt(.4122214708 * c[0] + .5363325363 * c[1] + .0514459929 * c[2]), m = Math.cbrt(.2119034982 * c[0] + .6806995451 * c[1] + .1073969566 * c[2]), s = Math.cbrt(.0883024619 * c[0] + .2817188376 * c[1] + .6299787005 * c[2]);
      var A = 1.9779984951 * l - 2.428592205 * m + .4505937099 * s, B = .0259040371 * l + .7827717662 * m - .808675766 * s;
      return {C:Math.sqrt(A * A + B * B), h:((Math.atan2(B, A) * 180 / Math.PI) + 360) % 360};
    }
    /* A moon's colour at a set lightness: its own hue, never its lightness, so pale moons don't blow out (moon view). */
    function ok(mo, l, cm, a){ var o = mo._ok || (mo._ok = hexOklch(/^#[0-9a-fA-F]{6}$/.test(mo.color || '') ? mo.color : '#d9e1ec')); o.cc = clamp(o.C, .045, .13); return 'oklch(' + l + ' ' + (o.cc * cm).toFixed(3) + ' ' + o.h.toFixed(1) + (a === undefined ? '' : ' / ' + a) + ')'; }

    /* ── Blood: under a blood moon the sky itself bleeds. Here and there a patch of the red sky darkens and thickens,
       gathers into a hanging bead, and lets go. Most are a single small drop; now and then a short stream pours; rarely a river of
       blood runs for a couple of seconds. Blood falls heavily but not fast, like something thick, and splashes on the
       land. Like the lightning, it is a plan made from a seed, so any moment of it can be drawn again exactly. ── */
    var CLOTS = [];
    function clotPlan(upTo){
      while (!CLOTS.length || CLOTS[CLOTS.length - 1].t0 <= upTo + 6){
        var j = CLOTS.length, prev = CLOTS[j - 1], h = function(s){ return hash(j * 7.13 + s); };
        var roll = h(1.1), kind = roll < .08 ? 2 : roll < .3 ? 1 : 0;
        CLOTS.push({t0:prev ? prev.t0 + .3 + h(2.2) * 1.0 : .2, kind:kind, x:.05 + .9 * h(3.3), y:.03 + .3 * h(4.4) * h(4.9),
          gather:[1.2 + h(5.5), 2 + h(5.5), 3 + 1.5 * h(5.5)][kind], pour:[0, .6 + .6 * h(6.6), 1.5 + 1.3 * h(6.6)][kind],
          size:[.6 + 1.1 * h(7.7), 1.7 + .6 * h(7.7), 3.2 + 1.2 * h(7.7)][kind]});
      }
      return CLOTS;
    }
    /* A thick fall: it gathers speed for about half a second, then holds a heavy, unhurried pace. */
    function fallBy(tf, H){ var vt = H * .6, tau = .45; return tf <= 0 ? 0 : vt * (tf - tau * (1 - Math.exp(-tf / tau))); }
    function fallV(tf, H){ return tf <= 0 ? 0 : H * .6 * (1 - Math.exp(-tf / .45)); }
    function clots(ctx, st, t, k, sc){
      var W = st.W, H = st.H, ground = H * .84, P = clotPlan(t);
      ctx.save(); ctx.lineCap = 'round';
      for (var e = 0; e < P.length; e++){
        var C = P[e], age = t - C.t0;
        if (age < 0) break;
        var fallT = ground / (H * .6) + 1.4;
        if (age > C.gather + C.pour + fallT + .8) continue;
        var x0 = W * C.x, y0 = H * C.y, r = C.size * sc, u = Math.min(1, age / C.gather);
        var release = age - C.gather, drain = C.pour ? clamp(release / C.pour, 0, 1) : (release > 0 ? 1 : 0);
        /* The patch of sky thickens: a soft dark stain that deepens as it gathers, and fades once it has bled out. */
        var fade = release > 0 ? Math.max(0, 1 - (release - C.pour) / .8) : 1, a = u * u * (3 - 2 * u) * fade;
        if (a > 0){
          var R = r * (5 + 3 * u) * (1 - .3 * drain);
          var stain = ctx.createRadialGradient(x0, y0, 0, x0, y0, R);
          stain.addColorStop(0, 'rgba(55,0,6,' + (.9 * a * k).toFixed(3) + ')'); stain.addColorStop(.55, 'rgba(70,0,8,' + (.45 * a * k).toFixed(3) + ')'); stain.addColorStop(1, 'rgba(70,0,8,0)');
          ctx.fillStyle = stain; ctx.beginPath(); ctx.ellipse(x0, y0, R, R * .8, 0, 0, TAU); ctx.fill();
        }
        /* It gathers into a bead that sags lower and lower before it lets go. */
        var lip = y0 + r * 2.5 * u;
        if (release < 0){
          var bead = r * (.3 + .7 * u);
          if (u > .25) drop(ctx, x0, lip, bead, H * .25 * Math.max(0, u - .5), k * Math.min(1, (u - .25) * 3));
          continue;
        }
        if (!C.kind){
          var y = lip + fallBy(release, H);
          if (y < ground) drop(ctx, x0, y, r, fallV(release, H), k);
          else splash(ctx, x0, ground, r, (release - tHit(ground - lip, H)) / .6, k);
          continue;
        }
        /* A stream or a river: its head falls from the moment it lets go, its tail from the moment it runs dry. */
        var yh = lip + fallBy(release, H), yt = lip + fallBy(release - C.pour, H), yb = Math.min(yh, ground);
        if (yt < yb){
          var steps = 18, wid = r * (C.kind === 2 ? 2.4 : 1.6) * (1 - .5 * Math.max(0, release - C.pour * .6) / Math.max(.3, C.pour)), L = [], Rr = [], Hl = [];
          wid = Math.max(r * .6, wid);
          for (var q = 0; q <= steps; q++){
            var ya = yt + (yb - yt) * q / steps, f = clamp((ya - lip) / (ground - lip), 0, 1);
            var cx = x0 + Math.sin(ya * .07 + t * 5 + e) * r * .35 * f, wq = Math.max(1, wid * (1 - .45 * f)) * .5;
            L.push([cx - wq, ya]); Rr.push([cx + wq, ya]); Hl.push([cx - wq * .45, ya]);
          }
          ctx.fillStyle = 'rgba(120,4,16,' + (.95 * k).toFixed(3) + ')';
          ctx.beginPath(); L.forEach(function(P2, i){ i ? ctx.lineTo(P2[0], P2[1]) : ctx.moveTo(P2[0], P2[1]); });
          for (q = Rr.length - 1; q >= 0; q--) ctx.lineTo(Rr[q][0], Rr[q][1]);
          ctx.closePath(); ctx.fill();
          ctx.strokeStyle = 'rgba(255,110,110,' + (.35 * k).toFixed(3) + ')'; ctx.lineWidth = Math.max(.5, wid * .12);
          ctx.beginPath(); Hl.forEach(function(P2, i){ i ? ctx.lineTo(P2[0], P2[1]) : ctx.moveTo(P2[0], P2[1]); }); ctx.stroke();
          /* Its head breaks into drops. */
          for (var bd = 0; bd < 3; bd++){ var yd = yh + r * (2 + bd * 3); if (yd < ground) drop(ctx, x0 + (hash(e * 3 + bd) - .5) * r * 1.5, yd, r * (.6 - bd * .12), fallV(release, H), k); }
          if (yt > lip + 2 && yt < ground) drop(ctx, x0, yt, r * .5, fallV(release - C.pour, H), k);
        }
        var hit = tHit(ground - lip, H);
        /* While it pours, the splash keeps churning; once it runs dry, the splash settles. */
        if (release > hit) splash(ctx, x0, ground, r * (C.kind === 2 ? 2 : 1.3), release - hit < C.pour ? .15 + .08 * Math.sin(t * 23 + e) : .23 + (release - hit - C.pour) / .7, k);
      }
      ctx.restore();
    }
    /* How long a thick fall takes to cover a distance. */
    function tHit(d, H){ var lo = 0, hi = 10; for (var i = 0; i < 30; i++){ var m = (lo + hi) / 2; if (fallBy(m, H) < d) lo = m; else hi = m; } return lo; }
    function drop(ctx, x, y, r, v, k){
      var st = Math.min(r * 4, v * .03);
      ctx.fillStyle = 'rgba(145,8,22,' + (.96 * k).toFixed(3) + ')';
      ctx.beginPath(); ctx.ellipse(x, y - st * .5, r * .8, r + st * .5, 0, 0, TAU); ctx.fill();
      ctx.fillStyle = 'rgba(255,120,120,' + (.55 * k).toFixed(3) + ')';
      ctx.beginPath(); ctx.ellipse(x - r * .3, y - st * .5 - r * .3, r * .22, r * .3 + st * .15, 0, 0, TAU); ctx.fill();
    }
    function splash(ctx, x, y, r, u, k){
      if (u < 0 || u > 1) return;
      var a = (1 - u) * k;
      ctx.fillStyle = 'rgba(95,3,10,' + (.8 * a).toFixed(3) + ')';
      ctx.beginPath(); ctx.ellipse(x, y, r * (1.5 + 3 * u), r * (.35 + .5 * u), 0, 0, TAU); ctx.fill();
      for (var i = 0; i < 4; i++){ var ang = Math.PI * (1.15 + .7 * i / 3), d = r * (1 + 6 * u); ctx.beginPath(); ctx.arc(x + Math.cos(ang) * d, y + Math.sin(ang) * d * .6 - r * 4 * u * (1 - u), r * .3 * (1 - u), 0, TAU); ctx.fill(); }
    }
    /* The moon's own wet sheen: a soft light off its upper side that drifts round very slowly; it never brightens. */
    function sheen(ctx, M, t, lite){
      var ang = -2.25 + .4 * Math.sin(t / 38 * TAU), hx = M.x + Math.cos(ang) * M.r * .5, hy = M.y + Math.sin(ang) * M.r * .5;
      ctx.save(); ctx.beginPath(); ctx.arc(M.x, M.y, M.r * .97, 0, TAU); ctx.clip();
      var g = ctx.createRadialGradient(hx, hy, 0, hx, hy, M.r * .6);
      g.addColorStop(0, 'rgba(' + lite + ',.2)'); g.addColorStop(.5, 'rgba(' + lite + ',.06)'); g.addColorStop(1, 'rgba(' + lite + ',0)');
      ctx.fillStyle = g; ctx.fillRect(M.x - M.r, M.y - M.r, 2 * M.r, 2 * M.r); ctx.restore();
    }
    function bleed(ctx, st, t, rm){
      var B = st.bleed, M = B.M, H = st.H, sc = clamp(H / 150, .6, 1.5), k = clamp(B.k, 0, 1), tt = rm ? 12.4 : t;
      /* The moon's wet light is crimson: it sits in a blood-red sky. */
      sheen(ctx, M, tt, '255,96,96');
      clots(ctx, st, tt, k, sc);
    }

    /* ── The Twin Lanterns: a filament of light joins the two full moons; a soft pulse travels along it (moon view).
       In the sky it runs rim to rim, wherever the two moons are. ── */
    function conj(ctx, A, B, t, rm, night){
      var dx = B.x - A.x, dy = B.y - A.y, d = Math.hypot(dx, dy);
      if (d <= A.r + B.r + 2) return;
      var ux = dx / d, uy = dy / d, x0 = A.x + ux * A.r, y0 = A.y + uy * A.r, x1 = B.x - ux * B.r, y1 = B.y - uy * B.r;
      ctx.save(); ctx.lineCap = 'round';
      var gr = ctx.createLinearGradient(x0, y0, x1, y1); gr.addColorStop(0, ok(A.mo, .82, 1.3, .9)); gr.addColorStop(1, ok(B.mo, .82, 1.3, .9));
      if (night){ ctx.shadowColor = ok(A.mo, .75, 1.2, .8); ctx.shadowBlur = 6; }
      ctx.strokeStyle = gr; ctx.lineWidth = 1.3; ctx.beginPath(); ctx.moveTo(x0, y0); ctx.lineTo(x1, y1); ctx.stroke();
      ctx.shadowBlur = 0;
      if (!rm){
        var u = (t * 1000 % 2400) / 2400, e = u < .5 ? 2 * u * u : 1 - Math.pow(-2 * u + 2, 2) / 2, px = x0 + (x1 - x0) * e, py = y0 + (y1 - y0) * e, al = Math.sin(Math.PI * u);
        var pg = ctx.createRadialGradient(px, py, 0, px, py, 5);
        pg.addColorStop(0, 'rgba(255,250,240,' + (.95 * al).toFixed(3) + ')'); pg.addColorStop(.4, ok(B.mo, .85, 1.2, (.6 * al).toFixed(3))); pg.addColorStop(1, ok(B.mo, .85, 1.2, 0));
        ctx.fillStyle = pg; ctx.fillRect(px - 5, py - 5, 10, 10);
      }
      ctx.restore();
    }

    /* A calendar can have more moons than the painter has slots: the rest are
       drawn here, right after the painted ones, as the same MOONR sprites. */
    function extraMoons(ctx, st, dpr){
      st.moons.forEach(function(M){
        if (!M.extra || !M.up) return;
        var sz = Math.max(8, Math.ceil((M.sr || M.r) * 2 * dpr / 8) * 8), spr = window.MOONR.sprite(M.spec, M.m.p, sz, {blood:M.blood});
        ctx.save(); ctx.globalAlpha = clamp((1 - st.wx.cloud * .9) * (1 - st.wx.fog * .75), .04, 1);
        ctx.translate(M.x, M.y); ctx.rotate(M.rot); ctx.drawImage(spr, -M.r, -M.r, M.r * 2, M.r * 2); ctx.restore();
      });
    }

    /* hold: the surface's own canvases ({fx}); returns false when there is nothing to draw. */
    function draw(hold, st, dpr){
      var cj = st.moons.filter(function(M){ return M.conj && M.up && M.m.alt > 0; }), extra = st.moons.some(function(M){ return M.extra && M.up; });
      var need = extra || !!st.bleed || cj.length > 1 || (st.fxItems && st.fxItems.length) || (st.look && st.look.sigils.length);
      if (!need) return false;
      var cv = hold.fx || (hold.fx = document.createElement('canvas')), w = Math.max(1, Math.round(st.W * dpr)), h = Math.max(1, Math.round(st.H * dpr));
      if (cv.width !== w || cv.height !== h){ cv.width = w; cv.height = h; }
      var ctx = cv.getContext('2d'), rm = st.rm, t = st.t, night = st.dark > .45;
      ctx.setTransform(1, 0, 0, 1, 0, 0); ctx.clearRect(0, 0, w, h);
      ctx.setTransform(w / st.W, 0, 0, h / st.H, 0, 0);
      if (extra) extraMoons(ctx, st, dpr);
      var through = (1 - st.wx.cloud * .85) * (1 - st.wx.fog * .6);
      ctx.globalAlpha = clamp(through, .15, 1);
      if (st.bleed) bleed(ctx, st, t, rm);
      /* Moons full together are joined in a chain, west to east, by the moon view's thread of light. */
      cj.sort(function(a, b){ return a.x - b.x; });
      for (var i = 0; i + 1 < cj.length; i++) conj(ctx, cj[i], cj[i + 1], t, rm, night);
      ctx.globalAlpha = 1;
      window.SkyFX.far(ctx, st);
      return true;
    }
    return {draw:draw, hexOklch:hexOklch};
  })();

  /* An aurora's colours: the engine's hue, at the lightness and chroma curtains of light have. */
  function auroraLin(hex, top){ var o = LOOKS.okOf(hex); return LOOKS.okLin(top ? .56 : .8, top ? .16 : .19, o.h); }
  /* The sun's colour: white high up, gold low, deep orange at the horizon, through the thickening air. */
  function sunColour(altDeg){
    var a = Math.max(altDeg, -1), air = 1 / (Math.sin(Math.max(a, 0) * D2R) + .15 * Math.pow(Math.max(a, 0) + 3.885, -1.253));
    var tr = [Math.exp(-.1 * air), Math.exp(-.23 * air), Math.exp(-.55 * air)], I = 2.2;
    return [tr[0] * I, tr[1] * I * .96, tr[2] * I * .9];
  }
  function moonColour(M){ var c = M.spec && M.spec.color; return /^#[0-9a-fA-F]{6}$/.test(c || '') ? c : '#d9e1ec'; }

  /* The uniforms for one moment. st is the pane's state (sky_pane.js buildState) after SkyEvents.apply; S the painter. */
  function uniforms(st, S, dpr){
    var U = {}, L = st.L, W = st.W, H = st.H, t = st.t, wx = st.wx, sc = clamp(H / 150, .55, 1.15), lk = st.look, alt = st.altEff, P = st.P;
    U.uT = t;
    U.uView = [L.hor, L.top, sc, H < 110 ? 1 : 0];
    U.uCam = [(st.cam || 0) * D2R, 0, 0, 0];
    U.uSun = [st.sp.x, st.sp.y, st.R * .95, alt];
    U.uSunCol = sunColour(alt);
    var sunVis = (1 - wx.cloud * .82) * (1 - wx.fog * .5) * (1 - wx.dark * .5) * (1 - clamp(wx.rain, 0, 1) * .85) * (1 - (lk ? lk.sun.dim : 0));
    U.uSunK = [sunVis * smooth01(-4, .5, alt), 0, 0, lk ? lk.sun['void'] : 0];
    U.uSunT = lk && lk.sun.tint ? PAL.hexLin(lk.sun.tint).concat([.6]) : [1, 1, 1, 0];
    /* Moons: four slots, a moon's slot its place in the calendar's list. */
    var mv = [], ma = [], mk = [], mc = [], me = [], rects = {};
    /* The atlas only grows, and growing clears it, so the largest moon takes its slot first. */
    st.moons.filter(function(q){ return q.i >= 0 && q.up; }).sort(function(a, b){ return (b.sr || b.r) - (a.sr || a.r); }).forEach(function(M){
      var spec = M.spec, p = M.m.p, blood = !!M.blood;
      var key = (M.mo.id != null ? M.mo.id : M.mo.name) + ':' + window.MOONR.key(spec) + ':' + Math.round(((p % 1) + 1) % 1 * 120) % 120 + (blood ? ':b' : '');
      rects[M.i] = S.moonSlot(M.i, key, function(sz){ return window.MOONR.sprite(spec, p, sz, {blood:blood}); }, (M.sr || M.r) * 2 * dpr);
    });
    for (var i = 0; i < 4; i++){
      var M = st.moons.filter(function(q){ return q.i === i; })[0];
      if (!M || !M.up){ mv.push(0, 0, 0, 0); ma.push(0, 0, 0, 0); mk.push(0, 0, 0, 0); mc.push(0, 0, 0); me.push(0, 0, 0, 0); continue; }
      var blood = !!M.blood, rect = rects[i];
      var through = (1 - wx.cloud * .9) * (1 - wx.fog * .75);
      /* A storm hides the moon's disc, leaving its light on the cloud edges; a blood moon shows through its own thin
         weather. */
      through = blood ? Math.max(through, .85) : through * (1 - .75 * wx.storm);
      mv.push(M.x, M.y, M.r, M.rot); ma.push(rect[0], rect[1], rect[2], rect[3]);
      /* A moon under the horizon throws no glow above it; one in the world's shadow throws little. */
      mk.push(clamp(through, .04, 1), M.m.lit * (1 - (M.shadowCover || 0) * .85), blood ? .55 * (M.bloodK || 1) : 0, (1 + wx.fog * 2.2 + wx.cloud * .5 * (1 - wx.dark)) * smooth01(-4, 1, M.m.alt / D2R));
      var tl = PAL.hexLin(moonColour(M));
      if (M.warm) tl = PAL.mixLin(tl, PAL.hexLin(M.warm[0]), clamp(M.warm[1], 0, 1) * .8);
      mc.push(tl[0], tl[1], tl[2]);
      var sh = st.shadows[i]; if (sh) me.push(sh[0], sh[1], sh[2], sh[3]); else me.push(0, 0, 0, 0);
    }
    U.uMoon = mv; U.uMoonA = ma; U.uMoonK = mk; U.uMoonC = mc; U.uMoonE = me;
    /* Stars fade in through twilight; the sky turns them. */
    var starVis = smooth01(-3, -13, alt) * Math.pow(1 - clamp(wx.fog, 0, 1), 2) * (lk ? lk.stars : 1) * (1 + .45 * st.darkNight);
    U.uStars = [starVis, mod(st.sidereal + (st.cam || 0), 360), 1 + .8 * st.darkNight, st.rm ? 0 : 1];
    U.uWx = [wx.cloud, wx.dark, wx.fog, wx.wind];
    U.uPrecip = [wx.rain, wx.snow, wx.storm, lk ? lk.drift : 1];
    U.uPrecipK = [lk && lk.rainTint ? lk.rainTint[1] : 0, lk ? lk.slant : 0, lk ? lk.heavy : 0, wx.hail];
    U.uPrecipC = lk && lk.rainTint ? PAL.hexLin(lk.rainTint[0]) : [1, 1, 1];
    /* Lightning: the bolt and the cloud round it, never the whole sky. */
    var storm = Math.max(wx.storm, lk ? lk.flash.lightning : 0) * (st.motion != null ? st.motion * st.motion : 1), fl = storm > .3 && !st.rm ? flashAt(t) : {s:0}, bolt = [], nb = 0, bb = [0, 0];
    if (fl.s > .01 && fl.strike && fl.strike.bolt && fl.u < .45){
      var Sk = fl.strike, groundY = H * .86, y0 = H * .22, lx = Infinity, rx = -Infinity;
      /* The plan's unit is the strike's height, across as well as down. */
      var u = (groundY - y0) / .74, X = function(x){ return Sk.x * W + x * u; };
      Sk.seg.concat(Sk.br).forEach(function(s){
        if (nb >= 40) return;
        var a = [X(s[0]), y0 + (s[1] - .26) * u], b = [X(s[2]), y0 + (s[3] - .26) * u];
        bolt.push(a[0], a[1], b[0], b[1]); nb++; lx = Math.min(lx, a[0], b[0]); rx = Math.max(rx, a[0], b[0]);
      });
      bb = [lx - H * .3, rx + H * .3];
    }
    while (bolt.length < 160) bolt.push(0, 0, 0, 0);
    U.uFlash = [fl.x ? fl.x * W : W / 2, H * .3, clamp(fl.s, 0, 1.4) * storm, fl.strike ? Math.min(fl.strike.seg.length, 40) : 0];
    U.uBolt = bolt; U.uBoltK = [nb, nb ? clamp(fl.s, 0, 1.2) * storm : 0, bb[0], bb[1]];
    var met = [], metk = [];
    st.met.slice(0, 8).forEach(function(m){ met.push(m[0], m[1], m[2], m[3]); metk.push(m[4], m[5], m[6], m[7]); });
    while (met.length < 32){ met.push(0, 0, 0, 0); metk.push(0, 0, 0, 0); }
    U.uMet = met; U.uMetK = metk;
    /* Auroras, the two strongest: night light by night; by day a pale ribbon. */
    var aur = st.aur.slice().sort(function(a, b){ return b.k - a.k; }).slice(0, 2), night = smooth01(.25, .85, st.dark), aA = [], aB = [], c0 = [], c1 = [];
    for (i = 0; i < 2; i++){
      var au = aur[i];
      if (!au){ aA.push(0, 0, 0, 0); aB.push(0, 0, 0, 0); c0.push(0, 0, 0); c1.push(0, 0, 0); continue; }
      aA.push(clamp(au.k, 0, 1.2), au.az * D2R, (au.span || 90) * D2R, au.base); aB.push(night, au.seed || 0, 0, 0);
      var A0 = auroraLin(au.c0, false), A1 = auroraLin(au.c1, true); c0.push(A0[0], A0[1], A0[2]); c1.push(A1[0], A1[1], A1[2]);
    }
    U.uAurA = aA; U.uAurB = aB; U.uAurC0 = c0; U.uAurC1 = c1;
    /* Comets: no Chronicle event places one yet, so both slots stay empty. */
    U.uComet = [0, 0, 0, 1, 0, 0, 0, 1]; U.uCometK = [0, 1, 1, 0, 0, 1, 1, 0]; U.uCometC = [1, 1, 1, 1, 1, 1];
    /* Air: the look's layers, the three strongest, each lit by the scene. */
    var airs = (lk ? lk.air : []).concat(st.airs).filter(function(a){ return a.k > .005; }).sort(function(a, b){ return b.k - a.k; }).slice(0, 3), AA = [], AB = [], AD = [], AC = [];
    for (i = 0; i < 3; i++){
      var a2 = airs[i];
      if (!a2){ AA.push(0, 0, 0, 0); AB.push(0, 1, -1, 1); AD.push(0, 0, 0, 0); AC.push(0, 0, 0); continue; }
      AA.push(Math.min(1.2, a2.k), LOOKS.AIR[a2.t], a2.sx || 0, a2.sy || 0);
      AB.push(a2.v[0], a2.v[1], a2.cx != null ? a2.cx : -1, a2.hw || 1);
      AD.push(a2.d || 0, a2.rb ? 1 : 0, a2.e || 0, i * 3.7 + 1.3);
      var lc = LOOKS.lit(a2.c, a2.t === 'void' ? 'shade' : a2.e > .4 ? 'glow' : 'air', st.sl); AC.push(lc[0], lc[1], lc[2]);
    }
    U.uAirA = AA; U.uAirB = AB; U.uAirD = AD; U.uAirC = AC;
    /* A funnel cloud, and where its debris swirls. */
    var fu = lk && lk.funnel;
    if (fu && fu.k > .01){ var fxp = W * (.62 + .06 * Math.sin(t / 23)); U.uFunnel = [fxp, fu.k * (1 - wx.fog * .4), fu.w, 1.7]; U.uFunnelC = LOOKS.lit(fu.c, 'shade', st.sl); st.funnelAt = {x:fxp, gy:H * .86}; }
    else { U.uFunnel = [0, 0, 1, 0]; U.uFunnelC = [0, 0, 0]; st.funnelAt = null; }
    /* A blood moon's stain on the sky. */
    var bm = st.bleed;
    U.uEv = [bm ? .8 * bm.k * smooth01(-3, 8, bm.M.m.alt / D2R) * Math.max(.35, st.dark) : 0, bm ? bm.M.i : 0, 0, 0];
    U.uEvC = PAL.hexLin(bm ? bm.color : '#b8472f');
    U.uFxLight = [1, 1, 1, wx.fog];
    var dark = document.documentElement.classList.contains('dark');
    U.uInk = PAL.hexLin(dark ? '#e9ebf2' : '#2b3040'); U.uPaper = PAL.hexLin(dark ? '#2a2d34' : '#fdfcf9');
    U.uGrade = [.012, 0, st.almanac ? 1 : 0, 0];
    /* The painted palette, and moonlight on the land at night. */
    var ml = 0;
    st.moons.forEach(function(Mn){ if (Mn.up && Mn.m.alt > 0) ml = Math.max(ml, Mn.m.lit * (Mn.mo.size || 1) * smooth01(0, 12, Mn.m.alt / D2R)); });
    ml *= st.dark * (1 - wx.cloud * .8);
    U.uZen = P.zen; U.uMid = P.mid; U.uHor = P.hor; U.uGlow = P.glow; U.uAnti = P.anti; U.uCLit = P.clit; U.uCShade = P.cshade; U.uFogC = P.fog;
    U.uRim = PAL.mixLin(P.rim, [.36, .44, .66], ml * .6);
    U.uHill0 = P.h0; U.uHill1 = P.h1; U.uHill2 = P.h2; U.uHill3 = P.h3;
    U.uPal = [.55 * (1 - wx.cloud * .7), smooth01(-8, -1, alt) * (1 - smooth01(4, 10, alt)) * (1 - wx.cloud), st.dark, ml];
    return U;
  }

  /* ── The one painter every sky shares. Without WebGL2, while its context is lost, after a failed compile, or on a
     device that paints it too slowly, skies are drawn by the basic 2D sky instead. ── */
  var SLOW_KEY = 'chronicle.sky.basic';
  var PAINTER = (function(){
    var P = {S:null, err:null, waiting:false, slow:false, frames:[], listeners:[]};
    try { P.slow = window.sessionStorage.getItem(SLOW_KEY) === '1'; } catch (e) { /* no storage: measure again */ }
    function touch(){ P.listeners.forEach(function(fn){ try { fn(); } catch (e) { /* a gone sky */ } }); }
    P.get = function(){
      if (P.err || P.slow) return null;
      if (!P.S){
        if (!window.WebGL2RenderingContext || !window.SkyShaders){ P.err = 'no-webgl2'; return null; }
        var cv = document.createElement('canvas'); cv.width = 16; cv.height = 16;
        try { P.S = GLX.create(cv, 'painted'); P.S.onRestore = P.wait; cv.addEventListener('webglcontextlost', touch); }
        catch (e){ P.err = e; return null; }
        P.wait();
        return null;
      }
      return P.S.lost || !P.S.warm ? null : P.S;
    };
    /* Until the painter is ready the sky is drawn in 2D, so the first frame never waits on a compile. */
    P.wait = function(){
      if (P.waiting) return; P.waiting = true;
      function poll(){
        if (!P.S || P.S.lost){ P.waiting = false; return; }
        try { if (!P.S.ready(true)){ setTimeout(poll, 50); return; } }
        catch (e){ P.err = e; if (window.console) console.error(e); }
        P.waiting = false; touch();
      }
      setTimeout(poll, P.S.par ? 16 : 150);
    };
    /* Frame times of the painted sky. A device that takes longer than SLOW_MS a frame on average over a full window
       of moving frames, or longer than STALL_MS on two moving frames in a row, gets the basic sky for the rest of the
       session. */
    P.SLOW_MS = 28; P.WINDOW = 45; P.STALL_MS = 120; P.stalls = 0;
    function giveUp(){ P.slow = true; try { window.sessionStorage.setItem(SLOW_KEY, '1'); } catch (e) { /* not remembered */ } touch(); }
    P.timed = function(ms, moving){
      if (P.slow) return;
      if (!moving) return;
      P.stalls = ms > P.STALL_MS ? P.stalls + 1 : 0;
      if (P.stalls >= 2) return giveUp();
      P.frames.push(ms); if (P.frames.length > P.WINDOW) P.frames.shift();
      if (P.frames.length < P.WINDOW) return;
      var sum = 0; P.frames.forEach(function(v){ sum += v; });
      if (sum / P.frames.length > P.SLOW_MS) giveUp();
    };
    return P;
  })();

  /* paint(ctx, hold, st, dpr, oy): paints st with the shared painter into ctx at height oy (CSS px), the whole sky in
     the strip [oy, oy + st.H). hold keeps this sky's own drawn-layer canvases. Returns false when the painter is not
     available, so the caller draws the basic sky instead. */
  function paint(ctx, hold, st, dpr, oy, moving){
    var S = PAINTER.get(); if (!S) return false;
    var U = uniforms(st, S, dpr);
    var drew = FX.draw(hold, st, dpr);
    if (drew) S.fxUpload(hold.fx);
    U.uFxOn = drew ? 1 : 0;
    var near = window.SkyFX.near(hold, st, dpr);
    if (near) S.nearUpload(hold.nearCv);
    U.uNearOn = near ? 1 : 0;
    var land = LAND.get(st.W, st.H, dpr);
    S.landUpload(land.key, land.data, land.w);
    /* Only the painting is timed: preparing a moon's sprite is the same work the basic sky does. */
    var t0 = performance.now();
    if (!S.draw(U, dpr, st.W, st.H)) return false;
    var y = Math.round((oy || 0) * dpr);
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.drawImage(S.canvas, 0, S.canvas.height - S.h, S.w, S.h, 0, y, S.w, S.h);
    PAINTER.timed(performance.now() - t0, moving);
    return true;
  }

  window.SkyGL = {
    paint: paint,
    available: function () { return !!PAINTER.get(); },
    onChange: function (fn) { PAINTER.listeners.push(fn); },
    offChange: function (fn) { PAINTER.listeners = PAINTER.listeners.filter(function (f) { return f !== fn; }); },
    _painter: PAINTER, _uniforms: uniforms, _land: LAND, _flashAt: flashAt
  };
})();
