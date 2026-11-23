/**
 * cutscene_stage.js — plays the Cut scenes the Director shows to this player.
 * Mounted once per campaign page by the layout (outside #main-content, so it
 * outlives boosted navigation) when the Cut scenes add-on is on.
 *
 * It listens on the campaign's socket: `cutscene.show` takes over the screen
 * with the engine, `cutscene.stop` ends it. The socket replays nothing, so on
 * load and after a dropped connection it asks for this player's showings once
 * and plays one shown in the last two minutes that they have not watched (they
 * opened the page, or changed page, mid-scene).
 *
 * The engine (cutscene_engine.js) loads on first use. Chronicle.cutscenes.watch
 * lets the Table's Tonight list play a showing again.
 */
(function () {
  'use strict';

  // A showing this recent and unwatched plays when the page opens.
  var RECENT_MS = 2 * 60 * 1000;

  // pickRecent returns the showing to play now from the newest-first list of
  // this player's showings, or null.
  function pickRecent(rows, now) {
    if (!Array.isArray(rows)) return null;
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      if (!r || !r.showingId || r.watched) continue;
      var shown = Date.parse(r.shownAt), until = Date.parse(r.rewatchUntil);
      if (isNaN(shown) || now - shown > RECENT_MS || now - shown < -RECENT_MS) continue;
      if (!isNaN(until) && until <= now) continue;
      return r;
    }
    return null;
  }

  // List responses are a bare array or {data, total}.
  function rowsOf(body) {
    if (Array.isArray(body)) return body;
    return body && Array.isArray(body.data) ? body.data : [];
  }

  var api = { pickRecent: pickRecent, rowsOf: rowsOf, RECENT_MS: RECENT_MS };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof document === 'undefined') return;

  var W = window;
  var C = W.Chronicle;

  // Only the version token is read from the page; the path is fixed here, so
  // a changed attribute can never point the loader at another script.
  var ENGINE_PATH = '/static/js/cutscene_engine.js';
  var engineLoad = null;
  function loadEngine(attr) {
    if (W.ChronicleCutscene) return Promise.resolve(W.ChronicleCutscene);
    if (engineLoad) return engineLoad;
    var m = /\?v=([A-Za-z0-9._-]{1,80})$/.exec(attr || '');
    var src = ENGINE_PATH + (m ? '?v=' + encodeURIComponent(m[1]) : '');
    engineLoad = new Promise(function (resolve, reject) {
      var s = document.createElement('script');
      s.src = src;
      s.onload = function () { if (W.ChronicleCutscene) resolve(W.ChronicleCutscene); else reject(new Error('engine missing')); };
      s.onerror = function () { var e = new Error('The scene could not be loaded. Check the connection and try again.'); e.plain = true; reject(e); };
      document.head.appendChild(s);
    }).catch(function (e) { engineLoad = null; throw e; });
    return engineLoad;
  }

  function emit(name, detail) {
    try { W.dispatchEvent(new CustomEvent(name, { detail: detail })); } catch (e) { /* very old browser */ }
  }

  Chronicle.register('cutscene-stage', {
    init: function (el, config) {
      // Ids can look like numbers to boot.js's config parser.
      var campaignId = String(config.campaignId || '');
      var userId = String(config.userId || '');
      var engineSrc = el.getAttribute('data-engine-src') || '';
      if (!campaignId || !userId) return;
      var base = '/campaigns/' + encodeURIComponent(campaignId) + '/cutscenes';
      var alive = true, ws = null, wsTimer = null, wsDelay = 0;
      var current = null;
      var marked = {};

      function seen(id) {
        try { return W.sessionStorage.getItem('chronicle.cutscene.' + id) === '1'; } catch (e) { return false; }
      }
      function remember(id) {
        try { W.sessionStorage.setItem('chronicle.cutscene.' + id, '1'); } catch (e) { /* private window */ }
      }

      function markWatched(id) {
        remember(id);
        if (marked[id]) return;
        marked[id] = true;
        C.apiFetch(base + '/showings/' + encodeURIComponent(id) + '/watched', { method: 'POST' }).then(function (res) {
          if (!res.ok) throw new Error('status ' + res.status);
          emit('chronicle:cutscene-watched', { showingId: id });
        }).catch(function () { marked[id] = false; });
      }

      function play(id, scene, name) {
        if (!alive || !scene) return Promise.resolve();
        if (current) { var was = current; current = null; was.run.stop(); }
        var mine = { id: id, run: { stop: function () { mine.cancelled = true; } } };
        current = mine;
        return loadEngine(engineSrc).then(function (CS) {
          if (!alive || mine.cancelled || current !== mine) return;
          mine.run = CS.play(document.body, scene, {
            label: name ? 'Cut scene: ' + name : 'Cut scene',
            onClose: function () { if (current === mine) current = null; }
          });
          markWatched(id);
        }, function (e) {
          if (current === mine) current = null;
          throw e;
        });
      }

      // Errors a person may read carry plain: true.
      function plain(msg) { var e = new Error(msg); e.plain = true; return e; }
      function fetchShowing(id) {
        return C.apiFetch(base + '/showings/' + encodeURIComponent(id)).then(function (res) {
          if (!res.ok) throw plain(res.status === 404 ? 'This scene can no longer be watched.' : 'The scene could not be read. Try again in a moment.');
          return res.json();
        });
      }

      // watch plays a showing again from the Tonight list.
      function watch(id) {
        return fetchShowing(id).then(function (s) {
          return play(String(s.showingId || id), s.scene, s.scene && s.scene.name);
        });
      }

      // A showing that began before this page could hear it.
      function catchUp() {
        C.apiFetch(base + '/mine').then(function (res) {
          if (!res.ok) throw new Error('status ' + res.status);
          return res.json();
        }).then(function (body) {
          var rows = rowsOf(body);
          emit('chronicle:cutscene-list', { rows: rows });
          var r = pickRecent(rows, Date.now());
          if (!r || seen(r.showingId) || current) return null;
          return watch(String(r.showingId));
        }).catch(function () { /* the next show or reconnect tries again */ });
      }

      function connect() {
        if (!alive || typeof W.WebSocket !== 'function') return;
        var url = (W.location.protocol === 'https:' ? 'wss:' : 'ws:') + '//' + W.location.host + '/ws?campaign=' + encodeURIComponent(campaignId);
        var sock, opened = false;
        try { sock = new W.WebSocket(url); } catch (e) { return; }
        ws = sock;
        sock.addEventListener('open', function () {
          // A reconnect asks what it missed; the first open was covered at load.
          if (wsDelay) catchUp();
          opened = true;
          wsDelay = 0;
        });
        sock.addEventListener('message', function (ev) {
          var msg;
          try { msg = JSON.parse(ev.data); } catch (e) { return; }
          if (!msg || msg.campaignId !== campaignId || !msg.payload) return;
          var p = msg.payload;
          if (msg.type === 'cutscene.show' && p.showingId && p.scene) {
            var id = String(p.showingId);
            emit('chronicle:cutscene-shown', { showingId: id });
            play(id, p.scene, p.scene.name).catch(function () { /* stays in the Tonight list to watch */ });
          } else if (msg.type === 'cutscene.stop' && p.showingId) {
            if (current && current.id === String(p.showingId)) { var c = current; current = null; c.run.stop(); }
          }
        });
        sock.addEventListener('error', function (e) { if (e.preventDefault) e.preventDefault(); });
        sock.addEventListener('close', function () {
          if (ws !== sock) return;
          ws = null;
          // Never opened: not signed in, or the socket is refused; nothing to follow.
          if (!alive || !opened) return;
          wsDelay = Math.min(60000, (wsDelay || 1000) * 2);
          wsTimer = setTimeout(connect, wsDelay);
        });
      }

      C.cutscenes = { watch: watch };
      connect();
      catchUp();

      el._csStage = function () {
        alive = false;
        if (wsTimer) clearTimeout(wsTimer);
        if (ws) { var s = ws; ws = null; try { s.close(); } catch (e) { /* already closed */ } }
        if (current) { var c = current; current = null; c.run.stop(); }
        if (C.cutscenes && C.cutscenes.watch === watch) C.cutscenes = null;
      };
    },
    destroy: function (el) {
      if (el._csStage) el._csStage();
      el._csStage = null;
    }
  });
})();
