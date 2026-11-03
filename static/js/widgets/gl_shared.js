/*
 * gl_shared.js — the page's one graphics-card painter (window.SharedGL).
 *
 * Everything on a page that paints with WebGL2 (the skies in sky_gl.js, the
 * painted crafting benches in craft_paint_stage.js) shares one context on one
 * hidden canvas: each draws into the canvas's lower-left corner and copies
 * its picture out at once. Browsers allow only a handful of contexts, and a
 * lost context is then one thing to restore.
 *
 * It also keeps the page's one verdict on whether this device paints those
 * effects too slowly. The sky and the benches each measure in their own way;
 * whichever finds the device slow says so here, and both then draw their
 * plainer versions for the rest of the session (ADR-072).
 */
window.SharedGL = window.SharedGL || (function () {
  'use strict';

  // One full-screen triangle; a fragment shader reads its pixel from gl_FragCoord.
  var VS = '#version 300 es\nvoid main(){ vec2 p = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2)); gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0); }';
  /* A program is compiled and linked without waiting on it; finish() checks it and reads its uniforms once the driver
     is done, so the page is never held up by a compile it does not need yet. */
  function program(gl, fs, label) {
    var p = gl.createProgram(), v = gl.createShader(gl.VERTEX_SHADER), f = gl.createShader(gl.FRAGMENT_SHADER);
    gl.shaderSource(v, VS); gl.compileShader(v); gl.shaderSource(f, fs); gl.compileShader(f);
    gl.attachShader(p, v); gl.attachShader(p, f); gl.linkProgram(p);
    return { p: p, v: v, f: f, src: fs, label: label, U: null };
  }
  // A failed compile is an error naming the line, never a silent blank.
  function finish(gl, P) {
    if (P.U) return;
    if (!gl.getProgramParameter(P.p, gl.LINK_STATUS) && !gl.isContextLost()) {
      [[P.v, VS, ' (vertex)'], [P.f, P.src, '']].forEach(function (q) {
        if (gl.getShaderParameter(q[0], gl.COMPILE_STATUS)) return;
        var log = gl.getShaderInfoLog(q[0]) || '', lines = q[1].split('\n'), m = /ERROR: \d+:(\d+)/.exec(log);
        throw new Error('Shader "' + P.label + q[2] + '" failed: ' + log + (m ? ' @ ' + lines[+m[1] - 1] : ''));
      });
      throw new Error('Program "' + P.label + '" failed to link: ' + gl.getProgramInfoLog(P.p));
    }
    var U = {}, n = gl.getProgramParameter(P.p, gl.ACTIVE_UNIFORMS);
    for (var i = 0; i < n; i++) { var info = gl.getActiveUniform(P.p, i), nm = info.name.replace(/\[0\]$/, ''); U[nm] = { loc: gl.getUniformLocation(P.p, info.name), type: info.type, size: info.size }; }
    P.U = U;
  }
  function setU(gl, P, name, v) {
    var u = P.U[name]; if (!u) return;
    var t = u.type, a = typeof v === 'number' ? [v] : v;
    if (t === gl.FLOAT) gl.uniform1fv(u.loc, a);
    else if (t === gl.FLOAT_VEC2) gl.uniform2fv(u.loc, a);
    else if (t === gl.FLOAT_VEC3) gl.uniform3fv(u.loc, a);
    else if (t === gl.FLOAT_VEC4) gl.uniform4fv(u.loc, a);
    else if (t === gl.INT || t === gl.SAMPLER_2D) gl.uniform1iv(u.loc, a.map(function (x) { return x | 0; }));
  }

  /* The context, made on first use. failIfMajorPerformanceCaveat: a browser that would paint in software gives no
     context, and every user draws its plainer version; window.SkyGLAllowSoftware lifts that for the browser checks
     only. preserveDrawingBuffer: pictures are copied out with drawImage, and Firefox can copy an unkept buffer as
     empty. gen counts restores, so a user rebuilds what a lost context took. */
  var C = { gl: null, canvas: null, err: null, lost: false, gen: 0, lostFns: [], restoreFns: [] };
  C.onLost = function (fn) { C.lostFns.push(fn); };
  C.onRestore = function (fn) { C.restoreFns.push(fn); };
  function each(list) { list.forEach(function (fn) { try { fn(); } catch (e) { if (window.console) console.error(e); } }); }
  function context() {
    if (C.gl) return C;
    if (C.err) return null;
    if (!window.WebGL2RenderingContext) { C.err = 'no-webgl2'; return null; }
    var cv = document.createElement('canvas'); cv.width = 16; cv.height = 16;
    var gl = null;
    try {
      gl = cv.getContext('webgl2', { alpha: true, premultipliedAlpha: true, antialias: false, depth: false, stencil: false, preserveDrawingBuffer: true,
        powerPreference: 'low-power', failIfMajorPerformanceCaveat: !window.SkyGLAllowSoftware });
    } catch (e) { gl = null; }
    if (!gl) { C.err = new Error('This needs WebGL2, which this browser did not provide.'); return null; }
    C.gl = gl; C.canvas = cv;
    cv.addEventListener('webglcontextlost', function (e) { e.preventDefault(); C.lost = true; each(C.lostFns); });
    cv.addEventListener('webglcontextrestored', function () { C.lost = false; C.gen++; each(C.restoreFns); });
    return C;
  }
  // The shared canvas only ever grows, so a user drawing w x h pixels never shrinks another's room.
  function fit(w, h) {
    var cv = C.canvas; if (!cv) return;
    if (cv.width < w || cv.height < h) { cv.width = Math.max(cv.width, w); cv.height = Math.max(cv.height, h); }
  }

  /* The slow-device verdict, kept for the session under the sky's existing key, so a device the sky already found
     slow stays found slow. */
  var SLOW_KEY = 'chronicle.sky.basic', slow = false, slowFns = [];
  try { slow = window.sessionStorage.getItem(SLOW_KEY) === '1'; } catch (e) { /* no storage: measure again */ }
  function isSlow() { return slow; }
  function markSlow() {
    if (slow) return;
    slow = true;
    try { window.sessionStorage.setItem(SLOW_KEY, '1'); } catch (e) { /* not remembered */ }
    each(slowFns);
  }
  function onSlow(fn) { slowFns.push(fn); }

  return { VS: VS, program: program, finish: finish, setU: setU, context: context, error: function () { return C.err; }, fit: fit,
    isSlow: isSlow, markSlow: markSlow, onSlow: onSlow, SLOW_KEY: SLOW_KEY };
})();
