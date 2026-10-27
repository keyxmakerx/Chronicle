/**
 * craft_rules_looks.js -- the How it looks tab of the crafting rules editor
 *
 * Pick a moment (every brew's success, progress, failure and breakthrough,
 * or one of the recipe's rules that players see), press play, and drag
 * effects onto the spots on the bench, or tap an effect and then a spot.
 * The effects are the item look tool's (ItemLookFX.LOOKS), so one shelf
 * serves items and crafting alike; the spots are its MOMENT_SPOTS, and the
 * bench is CraftRulesStage. What is saved is only spot → {look, move} and a
 * few words, cleaned by CraftRules.cleanLook. Exposes window.CraftRulesLooks.
 */
(function () {
  'use strict';
  var R = window.CraftRules, FX = window.ItemLookFX, B = window.CraftRulesStage, E = window.CraftRulesEditor;
  if (!R || !FX || !B || !E) return;
  var esc = R.esc, fa = E.fa;
  var MOVE_ICON = {shake: 'wave-square', dim: 'circle-half-stroke'};
  var HAND_P = {full: 0.5, half: 0.25, crescent: 0.125, dark: 0, vanished: 0};
  var EVERY = [['success', 'Success', 'it’s made'], ['progress', 'Progress', 'more work to do'], ['failure', 'Failure', 'a botch'], ['breakthrough', 'Breakthrough', 'a natural 19 or 20']];

  /* The window's moons on the preview day. */
  function moonsFor(S) {
    var W = S.W, day = W.day(S.at);
    return W.moons.map(function (m) {
      if (m.manual) { var s = W.hand[m.name]; return {color: m.color, p: HAND_P[s] || 0, gone: s === 'vanished'}; }
      return {color: m.color, p: R.moonOn(W, m, day).p};
    });
  }

  function mount(root, S) {
    var cur = 'success', armed = null, drag = null, undo = null, tm = null, stage = null, minis = [], dead = false;
    function rec() { return S.rec(); }
    function ruleAt(id) { return id.indexOf('rule:') === 0 ? rec().rules[+id.slice(5)] : null; }
    function groups() {
      return [['Every brew', EVERY], ['Your rules', rec().rules.map(function (q, i) {
        return q.players === 'secret' || q.draft ? null : ['rule:' + i, (i + 1) + ' · ' + R.thenTitle(q.then), R.whenTitle(S.W, q.when[0])];
      }).filter(Boolean)]];
    }
    function valid(id) { return groups().some(function (g) { return g[1].some(function (o) { return o[0] === id; }); }); }
    function look(id) {
      var q = ruleAt(id);
      if (q) { if (!q.look) { q.look = R.defLook(S.W, q); q._def = R.clone(q.look); } return q.look; }
      var r = rec(); if (!r.looks[id]) r.looks[id] = {spots: {}, words: ''};
      return r.looks[id];
    }
    function def(id) { var q = ruleAt(id); return q ? q._def : rec().defaults[id]; }
    function makes(id) { if (id === 'success' || id === 'breakthrough') return true; var q = ruleAt(id); return !!(q && R.makes(q.then)); }
    function nameOf(id) { var n = ''; groups().forEach(function (g) { g[1].forEach(function (o) { if (o[0] === id) n = o[1]; }); }); return n; }
    function spot(k) { return B.SPOTS.filter(function (s) { return s.k === k; })[0] || {k: k, name: k}; }
    function dots(l) {
      return Object.keys((l || {}).spots || {}).map(function (k) {
        var s = l.spots[k], L = s.look && FX.LOOKS[s.look];
        return L ? '<i style="background:' + L.color + '"></i>' : s.move ? '<i class="mv"></i>' : '';
      }).join('');
    }
    function rail() {
      return groups().map(function (g) {
        if (!g[1].length) return g[0] === 'Your rules' ? '<p class="crr-grp">Your rules</p><p class="crr-hint crr-railnote">No rule that players see yet.</p>' : '';
        return '<p class="crr-grp">' + esc(g[0]) + '</p>' + g[1].map(function (o) {
          return '<button type="button" class="crr-ob" data-out="' + o[0] + '" aria-pressed="' + (o[0] === cur) + '"><span class="crr-pl">' + fa('play') + '</span><span class="crr-on"><b>' + esc(o[1]) +
            '</b><small>' + esc(o[2]) + '</small></span><span class="crr-dl">' + dots(o[0].indexOf('rule:') === 0 ? ruleAt(o[0]).look : rec().looks[o[0]]) + '</span></button>';
        }).join('');
      }).join('') + (rec().rules.some(function (q) { return q.players === 'secret'; }) ? '<p class="crr-hint crr-railnote">' + fa('lock') + ' Rules set to “Only you” have no moment: players never see them fire.</p>' : '');
    }
    function spotList() {
      var l = look(cur);
      return B.SPOTS.map(function (s) {
        var v = l.spots[s.k] || {}, L = v.look && FX.LOOKS[v.look];
        return '<li class="' + (L || v.move ? 'is-full' : '') + '"><span class="crr-dot" style="' + (L ? 'background:' + L.color + ';border-style:solid' : '') + '"></span><span class="crr-sn"><b>' + esc(s.name) + '</b><em>' + esc(s.when) + '</em></span><span class="crr-sv">' +
          (L ? '<span class="crr-chipv">' + esc(L.name) + '<button type="button" data-clear="look" data-spot="' + s.k + '" aria-label="Remove ' + esc(L.name) + ' from ' + esc(s.name) + '">' + fa('xmark') + '</button></span>' : '') +
          (v.move ? '<span class="crr-chipv is-mv">' + esc(R.MOVES[v.move].name) + '<button type="button" data-clear="move" data-spot="' + s.k + '" aria-label="Remove ' + esc(R.MOVES[v.move].name) + ' from ' + esc(s.name) + '">' + fa('xmark') + '</button></span>' : '') +
          (!L && !v.move ? '<span class="crr-hint">empty</span>' : '') + '</span></li>';
      }).join('');
    }
    function spots() {
      var l = look(cur);
      return B.SPOTS.map(function (s) {
        var v = l.spots[s.k] || {}, L = v.look && FX.LOOKS[v.look];
        return '<button type="button" class="crr-spot' + (L || v.move ? ' is-full' : '') + (s.y < 220 || s.k === 'work' ? ' lab-up' : '') + '" data-spot="' + s.k + '" style="left:' + (s.x / 9.6) + '%;top:' + (s.y / 5.4) + '%" aria-label="' +
          esc(s.name + (L ? ': ' + L.name : '') + (v.move ? ', ' + R.MOVES[v.move].name : '')) + '"><i style="' + (L ? 'background:' + L.color + ';box-shadow:0 0 12px 3px ' + L.color : '') + '"></i>' +
          (v.move ? '<u>' + fa(MOVE_ICON[v.move]) + '</u>' : '') + '<span class="crr-slab">' + esc(s.name) + '</span></button>';
      }).join('');
    }
    function aiData() { var o = {}; if (cur.indexOf('rule:') === 0) o.rule = +cur.slice(5) + 1; else o.moment = cur; o.look = R.lookData(look(cur)); return o; }
    function $(s) { return root.querySelector(s); }
    function paint() {
      if (!valid(cur)) cur = 'success';
      var l = look(cur);
      $('.crr-orail').innerHTML = rail(); $('.crr-spots').innerHTML = spots(); $('.crr-spotlist').innerHTML = spotList();
      $('.crr-stg-cap').textContent = nameOf(cur);
      $('.crr-stg-tag').textContent = S.W.day(S.at).label + ' · ' + rec().name;
      var w = $('[data-words]'); if (document.activeElement !== w) w.value = l.words || '';
      $('.crr-fxai').innerHTML = E.hl(aiData());
      stage.set({spots: l.spots, words: l.words, makes: makes(cur)}); stage.liquid = rec().liquid || '#9b1c2c'; stage.setMoons(moonsFor(S));
      $('.crr-spots').classList.toggle('is-armed', !!armed);
      Array.prototype.forEach.call(root.querySelectorAll('.crr-lk2'), function (b) { var on = b.getAttribute('data-k') === armed; b.classList.toggle('is-armed', on); b.setAttribute('aria-pressed', String(on)); });
      $('[data-act="reset"]').hidden = JSON.stringify(R.lookData(def(cur)) || null) === JSON.stringify(R.lookData(l));
    }
    function changed() { S.changed('looks'); }
    function put(k, key) {
      var l = look(cur), s = l.spots[k] || (l.spots[k] = {});
      if (R.MOVES[key]) s.move = key; else s.look = key;
      armed = null; paint(); stage.play(); changed();
      S.say(spot(k).name + ': ' + (R.MOVES[key] ? R.MOVES[key].name : FX.LOOKS[key].name) + '.');
    }
    function toast(msg, fn) {
      var t = $('.crr-fxtoast');
      t.innerHTML = '<span>' + esc(msg) + '</span>' + (fn ? '<button type="button" class="quiet-word" data-act="undo">Undo</button>' : '');
      t.hidden = false; undo = fn; clearTimeout(tm);
      tm = setTimeout(function () { if (!dead) { t.hidden = true; undo = null; } }, 5000);
    }
    function shelf() {
      var h = '<div class="crr-looks">';
      Object.keys(FX.LOOKS).forEach(function (k) {
        h += '<button type="button" class="crr-lk2" data-k="' + k + '" aria-pressed="false" aria-label="' + esc(FX.LOOKS[k].name) + ': drag onto a spot, or tap it and then a spot"><canvas></canvas><b>' + esc(FX.LOOKS[k].name) + '</b></button>';
      });
      Object.keys(R.MOVES).forEach(function (k) {
        h += '<button type="button" class="crr-lk2 is-mv" data-k="' + k + '" aria-pressed="false" aria-label="' + esc(R.MOVES[k].name) + ': drag onto a spot, or tap it and then a spot"><span class="crr-mvi">' + fa(MOVE_ICON[k]) +
          '</span><b>' + esc(R.MOVES[k].name) + '</b><small>' + esc(R.MOVES[k].note) + '</small></button>';
      });
      return h + '</div>';
    }
    root.innerHTML = '<div class="crr-pane crr-fxpane"><div class="crr-sech"><h4>How it looks</h4><small>Pick a moment, press play, then drag effects onto the bench. They’re the same effects as item looks.</small></div>' +
      '<div class="crr-fxgrid"><nav class="crr-orail" aria-label="Moments"></nav><div class="crr-fxmain"><div class="crr-stage"><canvas role="img" aria-label="The bench, playing the chosen moment"></canvas>' +
      '<span class="crr-stg-tag"></span><span class="crr-stg-cap"></span><div class="crr-spots"></div><div class="crr-fxtoast" role="status" hidden></div></div>' +
      '<div class="crr-fxbar"><button type="button" class="btn btn-primary" data-act="play">' + fa('play') + '<span>Play it</span></button>' +
      '<button type="button" class="btn btn-secondary btn-sm" data-act="sound" aria-pressed="' + !!FX.Sound.on + '">' + fa(FX.Sound.on ? 'volume-high' : 'volume-xmark') + '<span>' + (FX.Sound.on ? 'Sound on' : 'Sound off') + '</span></button>' +
      '<span class="crr-grow"></span><button type="button" class="quiet-word" data-act="reset" hidden>' + fa('rotate-left') + '&nbsp;Back to the default</button></div>' +
      '<div class="crr-shelf"><div class="crr-shh"><h4>Effects</h4><span class="crr-hint">Drag one onto a spot on the bench. On a phone, tap it, then tap a spot.</span></div>' + shelf() + '</div>' +
      '<div class="crr-fxedit"><label class="crr-ctl"><span>Words on the bench</span><input class="crr-in" data-words maxlength="' + R.LIMITS.words + '"></label><ul class="crr-spotlist"></ul></div>' +
      '<div class="crr-aicap crr-block">What the AI writes</div><pre class="crr-aij crr-block crr-fxai"></pre></div></div></div>';
    stage = new B.Stage(root.querySelector('canvas'));
    var sp = $('.crr-spots');
    stage.onTick = function (st) { sp.classList.toggle('is-playing', st.mode === 'play'); };
    Array.prototype.forEach.call(root.querySelectorAll('.crr-lk2:not(.is-mv)'), function (b) { minis.push(new FX.Mini(b.querySelector('canvas'), FX.LOOKS[b.getAttribute('data-k')])); });
    paint();

    function spotAt(x, y) {
      var hit = null, best = 1e9;
      Array.prototype.forEach.call(root.querySelectorAll('.crr-spot'), function (s) {
        var r = s.getBoundingClientRect(), d = Math.hypot(x - (r.left + r.width / 2), y - (r.top + r.height / 2));
        if (d < Math.max(40, r.width) && d < best) { best = d; hit = s.getAttribute('data-spot'); }
      });
      return hit;
    }
    function onClick(e) {
      var t = e.target;
      var ob = t.closest('[data-out]'); if (ob) { cur = ob.getAttribute('data-out'); armed = null; paint(); stage.play(); return; }
      var cl = t.closest('[data-clear]');
      if (cl) {
        var l = look(cur), k = cl.getAttribute('data-spot'), f = cl.getAttribute('data-clear'), was = l.spots[k][f];
        delete l.spots[k][f]; if (!l.spots[k].look && !l.spots[k].move) delete l.spots[k];
        paint(); changed();
        toast('Removed ' + (f === 'move' ? R.MOVES[was].name : FX.LOOKS[was].name) + ' from ' + spot(k).name.toLowerCase() + '.', function () { (l.spots[k] = l.spots[k] || {})[f] = was; paint(); changed(); });
        return;
      }
      var s2 = t.closest('.crr-spot');
      if (s2) {
        var key = s2.getAttribute('data-spot'); if (armed) { put(key, armed); return; }
        var l2 = look(cur), v = l2.spots[key];
        if (v && (v.look || v.move)) { var keep = R.clone(v); delete l2.spots[key]; paint(); changed(); toast('Emptied ' + spot(key).name.toLowerCase() + '.', function () { l2.spots[key] = keep; paint(); changed(); }); }
        else stage.play();
        return;
      }
      var lk = t.closest('.crr-lk2');
      if (lk && !lk.getAttribute('data-dragged')) { var kk = lk.getAttribute('data-k'); armed = armed === kk ? null : kk; paint(); if (armed) S.say('Now tap a spot on the bench for ' + (R.MOVES[kk] ? R.MOVES[kk].name : FX.LOOKS[kk].name) + '.'); return; }
      var a = t.closest('[data-act]'); if (!a || !root.contains(a)) return;
      switch (a.getAttribute('data-act')) {
        case 'play': stage.play(); break;
        case 'sound':
          FX.Sound.on = !FX.Sound.on; a.setAttribute('aria-pressed', String(FX.Sound.on));
          a.innerHTML = fa(FX.Sound.on ? 'volume-high' : 'volume-xmark') + '<span>' + (FX.Sound.on ? 'Sound on' : 'Sound off') + '</span>';
          if (FX.Sound.on) { FX.Sound.ensure(); stage.play(); }
          break;
        case 'reset':
          var d = def(cur), q = ruleAt(cur);
          if (q) q.look = R.clone(d); else rec().looks[cur] = R.clone(d || {spots: {}, words: ''});
          paint(); stage.play(); changed(); break;
        case 'undo': if (undo) undo(); $('.crr-fxtoast').hidden = true; break;
      }
    }
    function onInput(e) {
      if (!e.target.hasAttribute('data-words')) return;
      look(cur).words = e.target.value.slice(0, R.LIMITS.words);
      $('.crr-fxai').innerHTML = E.hl(aiData()); stage.out.words = look(cur).words; changed();
    }
    function onKey(e) { if (e.key === 'Escape' && armed) { e.stopPropagation(); armed = null; paint(); } }
    /* Drag: a ghost follows the pointer, the spot under it lights up, and
       letting go places the effect there. */
    function onDown(e) {
      var lk = e.target.closest('.crr-lk2'); if (!lk || e.button) return;
      drag = {k: lk.getAttribute('data-k'), x: e.clientX, y: e.clientY, el: lk, moved: false}; lk.removeAttribute('data-dragged');
    }
    function onMove(e) {
      if (!drag) return;
      if (!drag.moved && Math.hypot(e.clientX - drag.x, e.clientY - drag.y) < 6) return;
      if (!drag.moved) {
        drag.moved = true; drag.el.setAttribute('data-dragged', '1'); drag.el.classList.add('is-lifted');
        var g = document.createElement('div'), L = FX.LOOKS[drag.k];
        g.className = 'crr-ghost'; g.innerHTML = (L ? '<i style="background:' + L.color + '"></i>' : fa(MOVE_ICON[drag.k])) + '<b>' + esc(L ? L.name : R.MOVES[drag.k].name) + '</b>';
        document.body.appendChild(g); drag.g = g; sp.classList.add('is-armed');
      }
      e.preventDefault();
      drag.g.style.left = e.clientX + 'px'; drag.g.style.top = e.clientY + 'px';
      var h = spotAt(e.clientX, e.clientY);
      Array.prototype.forEach.call(root.querySelectorAll('.crr-spot'), function (s) { s.classList.toggle('is-hot', s.getAttribute('data-spot') === h); });
    }
    function onUp(e) {
      if (!drag) return;
      var d = drag; drag = null; if (!d.moved) return;
      d.el.classList.remove('is-lifted'); if (d.g) d.g.remove();
      var h = e.type === 'pointercancel' ? null : spotAt(e.clientX, e.clientY);
      Array.prototype.forEach.call(root.querySelectorAll('.crr-spot'), function (s) { s.classList.remove('is-hot'); });
      sp.classList.remove('is-armed');
      if (h) put(h, d.k);
      setTimeout(function () { d.el.removeAttribute('data-dragged'); }, 0);
    }
    root.addEventListener('click', onClick);
    root.addEventListener('input', onInput);
    root.addEventListener('keydown', onKey);
    root.addEventListener('pointerdown', onDown);
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    window.addEventListener('pointercancel', onUp);
    return {
      select: function (id) { cur = id; armed = null; paint(); stage.play(); },
      refresh: function () { paint(); },
      destroy: function () {
        dead = true; clearTimeout(tm); stage.destroy(); minis.forEach(function (m) { m.destroy(); });
        if (drag && drag.g) drag.g.remove();
        root.removeEventListener('click', onClick); root.removeEventListener('input', onInput); root.removeEventListener('keydown', onKey); root.removeEventListener('pointerdown', onDown);
        window.removeEventListener('pointermove', onMove); window.removeEventListener('pointerup', onUp); window.removeEventListener('pointercancel', onUp);
      }
    };
  }

  window.CraftRulesLooks = {mount: mount, moonsFor: moonsFor};
})();
