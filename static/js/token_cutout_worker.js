/**
 * token_cutout_worker.js -- the token maker's smart cutout, off the page's
 * thread. A cutout takes several seconds of computing, so it runs here and
 * the maker stays responsive.
 *
 * The maker downloads the model and the WASM runtime itself (to show their
 * progress) and hands both over; this loads onnxruntime-web from the given
 * same-origin URL, opens the model, and turns a 1024 by 1024 picture into a
 * 1024 by 1024 mask, 0 (background) to 255 (figure).
 *
 *   { type: 'init', ort, mjs, wasm, model }   -> { type: 'ready' } or { type: 'error' }
 *   { type: 'run', pixels }                   -> { type: 'mask', mask } or { type: 'error' }
 */
/* global ort */
(function () {
  'use strict';

  var SIZE = 1024;
  var session = null;
  var STATIC_URL = /^\/static\/[A-Za-z0-9_.\-\/]+\.(js|mjs)(\?v=[A-Za-z0-9._\-]+)?$/;

  function fail(e) { self.postMessage({ type: 'error', message: String((e && e.message) || e) }); }

  function init(m) {
    if (!STATIC_URL.test(m.ort) || !STATIC_URL.test(m.mjs) || m.ort.indexOf('..') !== -1 || m.mjs.indexOf('..') !== -1) throw new Error('bad runtime url');
    importScripts(m.ort);
    // The page is not cross-origin isolated, so one thread; the loader comes
    // from Chronicle and the WASM is the copy the maker already downloaded.
    ort.env.wasm.numThreads = 1;
    ort.env.wasm.wasmPaths = { mjs: new URL(m.mjs, self.location.href).href };
    ort.env.wasm.wasmBinary = m.wasm;
    return ort.InferenceSession.create(new Uint8Array(m.model), { executionProviders: ['wasm'] }).then(function (s) {
      session = s;
      self.postMessage({ type: 'ready' });
    });
  }

  // IS-Net takes the picture as three planes scaled to -0.5..0.5 and gives
  // back how much each pixel is figure, which is stretched to 0..255.
  function run(m) {
    var px = m.pixels, N = SIZE * SIZE, x = new Float32Array(3 * N);
    for (var i = 0; i < N; i++) {
      x[i] = px[i * 4] / 255 - 0.5;
      x[N + i] = px[i * 4 + 1] / 255 - 0.5;
      x[2 * N + i] = px[i * 4 + 2] / 255 - 0.5;
    }
    var feeds = {};
    feeds[session.inputNames[0]] = new ort.Tensor('float32', x, [1, 3, SIZE, SIZE]);
    return session.run(feeds, [session.outputNames[0]]).then(function (r) {
      var o = r[session.outputNames[0]].data, lo = Infinity, hi = -Infinity, k;
      for (k = 0; k < N; k++) { if (o[k] < lo) lo = o[k]; if (o[k] > hi) hi = o[k]; }
      var span = hi - lo || 1, mask = new Uint8Array(N);
      for (k = 0; k < N; k++) mask[k] = Math.round((o[k] - lo) / span * 255);
      self.postMessage({ type: 'mask', mask: mask }, [mask.buffer]);
    });
  }

  self.onmessage = function (e) {
    var m = e.data || {};
    try {
      var p = m.type === 'init' ? init(m) : m.type === 'run' && session ? run(m) : Promise.reject(new Error('not ready'));
      p.catch(fail);
    } catch (err) { fail(err); }
  };
})();
