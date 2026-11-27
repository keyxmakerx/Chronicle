/**
 * Cut scene workshop
 *
 * The Director's Cut scenes pages (internal/plugins/cutscenes/*.templ). The
 * server draws every page and checks every scene; this file only does
 * Preview: it posts the editor's form to the preview route, which answers
 * with the scene exactly as a save would store it, loads the engine
 * (static/js/cutscene_engine.js, from the mount's data-engine-src) the first
 * time, and plays the scene over the whole window with the same call the
 * player stage makes (static/js/widgets/cutscene_stage.js), so the Director
 * sees exactly what a player sees: the page itself dimming from its edges,
 * then the scene. Playing it in a box would hide the opening, since the
 * darkness would fall over a box that is already dark. A refusal is shown in
 * the form's error line in the server's own words.
 *
 * Preview in Foundry posts the same form to its own route, which checks the
 * scene the same way and hands it to the campaign's Foundry module to play
 * on the Director's own Foundry screen; this page only says it was sent.
 *
 * Both buttons call window.ChronicleCutsceneWorkshop from an inline handler,
 * so a form HTMX swaps in after a save keeps working.
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
      var run = got[1].play(document.body, got[0], {
        label: 'Preview: ' + (got[0].name || 'cut scene'),
        onClose: function () {
          if (playing === run) playing = null;
          if (btn.isConnected) btn.focus();
        }
      });
      playing = run;
    }).catch(function (err) {
      done();
      showError(form, err && err.message ? err.message : 'The scene could not be previewed. Try again.');
    });
  }

  function showSent(form, msg) {
    var el = form && form.querySelector('[data-csw-sent]');
    if (!el) return;
    el.textContent = msg || '';
    el.hidden = !msg;
  }

  function previewInFoundry(btn) {
    var form = btn && btn.closest('form[data-csw-form]');
    if (!form || btn.disabled) return;
    showError(form, '');
    showSent(form, '');
    btn.disabled = true;
    btn.setAttribute('aria-busy', 'true');
    var done = function () {
      btn.disabled = false;
      btn.removeAttribute('aria-busy');
    };
    C.apiFetch(form.getAttribute('data-foundry-preview-url'), { method: 'POST', body: new FormData(form) })
      .then(function (res) {
        return res.json().catch(function () { return {}; }).then(function (body) {
          if (!res.ok) throw new Error(body.message || body.error || 'The scene could not be sent to Foundry. Try again.');
          return body;
        });
      })
      .then(function () {
        done();
        showSent(form, 'Sent to your Foundry screen');
      })
      .catch(function (err) {
        done();
        showError(form, err && err.message ? err.message : 'The scene could not be sent to Foundry. Try again.');
      });
  }

  window.ChronicleCutsceneWorkshop = { preview: preview, previewInFoundry: previewInFoundry };

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
