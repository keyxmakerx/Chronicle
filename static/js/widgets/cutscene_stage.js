/**
 * cutscene_stage.js — plays the Cut scenes the Director shows to this player.
 * Mounted once per campaign page by the layout (outside #main-content, so it
 * outlives boosted navigation) when the Cut scenes add-on is on.
 *
 * It listens on the campaign's socket: `cutscene.show` takes over the screen
 * with the engine, `cutscene.stop` ends it. At a moment the player's answer
 * goes to the answer route; `cutscene.moment_answering` says who else is
 * answering and `cutscene.moment_go` lets the moment go with the lines that
 * joined the story. On the Director's own screens `cutscene.moment_answer`
 * raises `cutscene-moment` on the body, which redraws the scene page's
 * moments panel. The socket replays nothing, so on load and after a dropped
 * connection it asks for this player's showings once and plays one shown in
 * the last two minutes that they have not watched (they opened the page, or
 * changed page, mid-scene), or one still holding on a moment, with what
 * they already answered there.
 *
 * The engine (cutscene_engine.js) loads on first use. Chronicle.cutscenes.watch
 * lets the Table's Tonight list play a showing again.
 */
(function () {
  'use strict';

  // A showing this recent, unwatched and not stopped plays when the page
  // opens; a stopped one is the Director calling it off, so it waits in the
  // Watch again list instead.
  var RECENT_MS = 2 * 60 * 1000;

  // pickRecent returns the showing to play now from the newest-first list of
  // this player's showings, or null. One still holding on a moment waits on
  // this player, so it plays however long ago it began and whether or not
  // they watched it.
  function pickRecent(rows, now) {
    if (!Array.isArray(rows)) return null;
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      if (!r || !r.showingId || r.stopped) continue;
      var shown = Date.parse(r.shownAt), until = Date.parse(r.rewatchUntil);
      if (!isNaN(until) && until <= now) continue;
      if (r.holding) return r;
      if (r.watched) continue;
      if (isNaN(shown) || now - shown > RECENT_MS || now - shown < -RECENT_MS) continue;
      return r;
    }
    return null;
  }

  // whoText is the quiet line under a moment's question: who else is
  // answering. A moment one person answers is theirs once they start.
  function whoText(names, one) {
    if (!names.length) return '';
    if (one) return names[0] + ' is answering this one.';
    if (names.length === 1) return names[0] + ' is adding to the story\u2026';
    var last = names[names.length - 1];
    return names.slice(0, -1).join(', ') + ' and ' + last + ' are adding to the story\u2026';
  }

  // List responses are a bare array or {data, total}.
  function rowsOf(body) {
    if (Array.isArray(body)) return body;
    return body && Array.isArray(body.data) ? body.data : [];
  }

  var api = { pickRecent: pickRecent, rowsOf: rowsOf, whoText: whoText, RECENT_MS: RECENT_MS };
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

      // answer sends this player's answer at moment i of showing id; a
      // refusal rejects with the server's words, which a person may read.
      function answer(id, i, ans) {
        var fd = new FormData();
        fd.append('way', ans.way);
        fd.append('text', ans.text || '');
        return C.apiFetch(base + '/showings/' + encodeURIComponent(id) + '/moments/' + i + '/answer', { method: 'POST', body: fd }).then(function (res) {
          if (res.ok) return res.json();
          return res.json().catch(function () { return null; }).then(function (j) {
            throw plain(j && j.message && res.status < 500 ? j.message : "That didn't send. Try again in a moment.");
          });
        });
      }

      // play takes over the screen with a showing; past (optional) is what
      // a late screen catches up on: the moments let go and its answers.
      function play(id, scene, name, past) {
        if (!alive || !scene) return Promise.resolve();
        if (current) { var was = current; current = null; was.run.stop(); }
        var mine = { id: id, scene: scene, who: new Map(), run: { stop: function () { mine.cancelled = true; } } };
        current = mine;
        past = past || {};
        return loadEngine(engineSrc).then(function (CS) {
          if (!alive || mine.cancelled || current !== mine) return;
          mine.run = CS.play(document.body, scene, {
            label: name ? 'Cut scene: ' + name : 'Cut scene',
            released: Array.isArray(past.released) ? past.released : [],
            onAnswer: function (i, ans) { return answer(id, i, ans); },
            onClose: function () { if (current === mine) current = null; }
          });
          (Array.isArray(past.answers) ? past.answers : []).forEach(function (a) {
            if (a && typeof a.moment === 'number' && mine.run.answered) mine.run.answered(a.moment, a);
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
          return play(String(s.showingId || id), s.scene, s.scene && s.scene.name, { released: s.released, answers: s.answers });
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
          if (!r || current || (!r.holding && seen(r.showingId))) return null;
          return watch(String(r.showingId));
        }).catch(function () { /* the next show or reconnect tries again */ });
      }

      // answering puts who else is answering under the question.
      function answering(id, i, uid, name) {
        if (!current || current.id !== id || !current.run.status || !uid) return;
        // Maps, not objects: the ids come off the socket, so a key such as
        // "__proto__" must stay a plain key.
        var who = current.who.get(i);
        if (!who) { who = new Map(); current.who.set(i, who); }
        who.set(String(uid), name || 'Someone');
        var names = Array.from(who.values());
        var m = current.scene && current.scene.moments && current.scene.moments[i];
        var one = !!(m && m.to === 'one');
        current.run.status(i, whoText(names, one), one);
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
          } else if (msg.type === 'cutscene.moment_answering' && p.showingId && typeof p.moment === 'number') {
            answering(String(p.showingId), p.moment, String(p.userId || ''), String(p.name || ''));
          } else if (msg.type === 'cutscene.moment_go' && p.showingId && typeof p.moment === 'number') {
            if (current && current.id === String(p.showingId) && current.run.go) current.run.go(p.moment, Array.isArray(p.lines) ? p.lines : []);
          } else if (msg.type === 'cutscene.moment_answer' && p.showingId) {
            // The Director's moments panel listens for this on the body.
            try { document.body.dispatchEvent(new CustomEvent('cutscene-moment', { bubbles: true, detail: { showingId: String(p.showingId), sceneId: String(p.sceneId || ''), moment: p.moment } })); } catch (e) { /* very old browser */ }
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
