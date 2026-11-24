/**
 * Cut scene workshop
 *
 * The Director's Cut scenes pages (internal/plugins/cutscenes/*.templ). The
 * server draws every page and checks every scene; this file only does
 * Preview: it posts the editor's form to the preview route, which answers
 * with the scene exactly as a save would store it, loads the engine
 * (static/js/cutscene_engine.js, from the mount's data-engine-src) the first
 * time, and plays the scene. On a wide screen it plays contained, in the
 * editor's preview box (sized Computer, Tablet or Phone by the switch above
 * it, which the server draws), so the Director keeps their place; on a phone the box
 * would be too small to judge the words by, so it plays over the whole window
 * as a player would see it. A refusal is shown in the form's error line in
 * the server's own words.
 *
 * The Preview button calls window.ChronicleCutsceneWorkshop.preview from an
 * inline handler, so a form HTMX swaps in after a save keeps working.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});
  var engine = null; // the engine's loading promise, kept once it worked

  function loadEngine(src) {
    if (window.ChronicleCutscene) return Promise.resolve(window.ChronicleCutscene);
    if (engine) return engine;
    engine = new Promise(function (resolve, reject) {
      var s = document.createElement('script');
      s.src = src;
      s.async = true;
      s.onload = function () {
        if (window.ChronicleCutscene) resolve(window.ChronicleCutscene);
        else reject(new Error('missing'));
      };
      s.onerror = function () { reject(new Error('failed')); };
      document.head.appendChild(s);
    });
    // A failed load may succeed on the next press.
    engine.catch(function () { engine = null; });
    return engine;
  }

  function showError(form, msg) {
    var el = form && form.querySelector('[data-csw-err]');
    if (!el) return;
    el.textContent = msg || '';
    el.hidden = !msg;
  }

  var playing = null;
  var hideTimer = 0;

  // Narrow screens preview full-window: a box at phone width is a strip.
  function wantsBox() {
    try { return !window.matchMedia('(max-width: 639px)').matches; } catch (e) { return true; }
  }

  function preview(btn) {
    var form = btn && btn.closest('form[data-csw-form]');
    var mount = btn && btn.closest('[data-engine-src]');
    if (!form || !mount || btn.disabled) return;
    showError(form, '');
    btn.disabled = true;
    btn.setAttribute('aria-busy', 'true');
    var done = function () {
      btn.disabled = false;
      btn.removeAttribute('aria-busy');
    };
    var fetchScene = C.apiFetch(form.getAttribute('data-preview-url'), { method: 'POST', body: new FormData(form) })
      .then(function (res) {
        return res.json().catch(function () { return {}; }).then(function (body) {
          if (!res.ok) throw new Error(body.message || body.error || 'The scene could not be checked. Try again.');
          return body;
        });
      });
    Promise.all([fetchScene, loadEngine(mount.getAttribute('data-engine-src')).catch(function () {
      throw new Error('The cut scene player could not be loaded. Try again.');
    })]).then(function (got) {
      done();
      if (playing && playing.stop) playing.stop();
      clearTimeout(hideTimer);
      var box = wantsBox() ? form.querySelector('[data-csw-preview-box]') : null;
      if (box) box.hidden = false;
      var run = got[1].play(box || document.body, got[0], {
        contained: !!box,
        label: 'Preview: ' + (got[0].name || 'cut scene'),
        onClose: function () {
          if (playing === run) playing = null;
          // The box folds away once the scene has faded out of it, unless
          // another preview has started in the meantime.
          if (box) hideTimer = setTimeout(function () { if (!playing) box.hidden = true; }, 600);
          if (btn.isConnected) btn.focus();
        }
      });
      playing = run;
    }).catch(function (err) {
      done();
      showError(form, err && err.message ? err.message : 'The scene could not be previewed. Try again.');
    });
  }

  window.ChronicleCutsceneWorkshop = { preview: preview };

  if (C.register) {
    C.register('cutscene-workshop', {
      init: function () {},
      destroy: function () {
        if (playing && playing.stop) playing.stop();
        playing = null;
      }
    });
  }
})();
