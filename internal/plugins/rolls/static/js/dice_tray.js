/**
 * dice_tray.js: the Dice tray card on the players' screen and the DM Screen
 * (data-widget="dice-tray").
 *
 * The card holds the shared game mat (every member's public rolls, each roller's dice coming in from
 * their own side) and the member's own tray, a tower or a bowl, which the rolls they make from the
 * card go through. The drawing is dice_tray_core.js; this file is the page around it: the roll
 * control, the four personal choices, and the rolls arriving from the Table.
 *
 * Rolls arrive as the page's Table receives them (it fires chronicle:roll-made for each live roll
 * from the campaign's socket), so the card adds no route and no socket of its own. What reaches this
 * browser was already narrowed by the server to what this member may see; the core then draws only
 * the public ones.
 *
 * The four choices (level, finish, tower or bowl, Others' dice) are saved with the screen's style
 * prefs through dm_arrange.js, on the dock element. A page without a dock keeps them for the card's
 * life.
 */
(function () {
  'use strict';

  var C = window.Chronicle;
  if (!C || !C.register) return;

  var DICE = [4, 6, 8, 10, 12, 20];
  var QUICK = [{ label: '2d10 + 2', c: { 10: 2 }, m: 2 }, { label: '2d10', c: { 10: 2 }, m: 0 }, { label: 'd20', c: { 20: 1 }, m: 0 }, { label: '3d6', c: { 6: 3 }, m: 0 }];
  var MAX_DICE = 10;
  var DEFAULTS = { trayLevel: 'detailed', trayFinish: 'paper', trayShape: 'tower', trayOthers: 'on' };
  // The saved keys and the values each may hold, so a stale or hand-edited copy cannot draw anything odd.
  var ALLOWED = { trayLevel: ['simple', 'flat', 'detailed'], trayFinish: ['paper', 'kit'], trayShape: ['tower', 'bowl'], trayOthers: ['on', 'off'] };

  function esc(s) { return C.escapeHtml(String(s == null ? '' : s)); }
  function seg(label, key, choices, cur) {
    return '<div class="dtr-set-row" data-dtr-row="' + key + '"><span class="dtr-lbl" id="dtr-l-' + key + '">' + esc(label) + '</span>' +
      '<span class="dtr-seg" role="group" aria-labelledby="dtr-l-' + key + '">' + choices.map(function (c) {
        return '<button type="button" data-dtr-set="' + key + '" data-v="' + c[0] + '" aria-pressed="' + (c[0] === cur) + '">' + esc(c[1]) + '</button>';
      }).join('') + '</span></div>';
  }
  function formula(counts, mod) {
    var parts = DICE.slice().reverse().filter(function (s) { return counts[s]; }).map(function (s) { return (counts[s] > 1 ? counts[s] : '') + 'd' + s; });
    var f = parts.join('+');
    if (f && mod) f += (mod > 0 ? '+' : '-') + Math.abs(mod);
    return f;
  }
  function typing(ev) {
    var t = ev.target;
    return !!t && (t.isContentEditable || /^(input|textarea|select)$/i.test(t.tagName || ''));
  }

  C.register('dice-tray', {
    init: function (el) {
      var cid = el.getAttribute('data-campaign-id') || '';
      var dock = el.closest('.dms-dock') || document.querySelector('.dms-dock');
      var kept = Object.assign({}, DEFAULTS), counts = { 10: 2 }, mod = 2, busy = false;
      // The screen's saved choices, when this card sits on a dock.
      var saved = dock && dock._dmsTray;
      if (saved) kept = Object.assign(kept, saved.get());
      Object.keys(ALLOWED).forEach(function (k) { if (ALLOWED[k].indexOf(kept[k]) < 0) kept[k] = DEFAULTS[k]; });

      el.innerHTML = '<div class="dtr-stage" data-dtr-stage></div>' +
        '<div class="dtr-roll" data-dtr-roll><span class="dtr-lbl">Roll</span>' +
          '<div class="dtr-dice">' + DICE.map(function (s) { return '<button type="button" class="dtr-die" data-s="' + s + '" aria-label="Add a d' + s + '">d' + s + '<span class="dtr-n" hidden></span></button>'; }).join('') + '</div>' +
          '<div class="dtr-quick">' + QUICK.map(function (q, i) { return '<button type="button" class="dtr-quiet" data-q="' + i + '">' + esc(q.label) + '</button>'; }).join('') + '<button type="button" class="dtr-quiet" data-clear>Clear</button></div>' +
          '<div class="dtr-pool"><span class="dtr-expr" data-dtr-expr></span>' +
            '<span class="dtr-step"><button type="button" data-m="-1" aria-label="Lower the bonus">&minus;</button><span data-dtr-mv></span><button type="button" data-m="1" aria-label="Raise the bonus">+</button></span>' +
            '<button type="button" class="dtr-go" data-dtr-go>Roll</button></div>' +
          '<p class="dtr-err" role="alert" hidden></p>' +
          '<p class="dtr-hint">Public: everyone at the table sees it. Press R here to roll, C to clear.</p></div>' +
        '<div class="dtr-set">' +
          seg('Level', 'trayLevel', [['simple', 'Simple'], ['flat', 'Flat paper'], ['detailed', 'Detailed']], kept.trayLevel) +
          seg('Finish', 'trayFinish', [['paper', 'Paper'], ['kit', 'Style kit']], kept.trayFinish) +
          seg('Your tray', 'trayShape', [['tower', 'Tower'], ['bowl', 'Bowl']], kept.trayShape) +
          seg('Others’ dice', 'trayOthers', [['on', 'On'], ['off', 'Off']], kept.trayOthers) +
        '</div>';
      var stage = el.querySelector('[data-dtr-stage]'), errEl = el.querySelector('.dtr-err'), goBtn = el.querySelector('[data-dtr-go]');
      var tray = C.diceTray.mount(stage, { level: kept.trayLevel, finish: kept.trayFinish, shape: kept.trayShape, others: kept.trayOthers !== 'off' });

      function total() { return Object.keys(counts).reduce(function (n, k) { return n + counts[k]; }, 0); }
      function paintPool() {
        var f = formula(counts, mod);
        el.querySelector('[data-dtr-expr]').textContent = f || 'No dice yet';
        el.querySelector('[data-dtr-mv]').textContent = (mod >= 0 ? '+' : '−') + Math.abs(mod);
        goBtn.disabled = !f || busy;
        Array.prototype.forEach.call(el.querySelectorAll('.dtr-die'), function (b) {
          var n = counts[b.getAttribute('data-s')] || 0, t = b.querySelector('.dtr-n');
          t.hidden = !n; t.textContent = String(n); b.classList.toggle('dtr-on', !!n);
        });
      }
      function paintSet() {
        Array.prototype.forEach.call(el.querySelectorAll('[data-dtr-set]'), function (b) {
          b.setAttribute('aria-pressed', String(kept[b.getAttribute('data-dtr-set')] === b.getAttribute('data-v')));
        });
        el.querySelector('[data-dtr-row="trayFinish"]').hidden = kept.trayLevel !== 'detailed';
      }
      function apply() {
        tray.setLevel(kept.trayLevel); tray.setFinish(kept.trayFinish); tray.setShape(kept.trayShape); tray.setOthers(kept.trayOthers !== 'off');
        paintSet();
      }
      paintPool(); paintSet();

      // The roll is made on the server, which decides every number and logs it; the tray only throws
      // what comes back. It is public, so it lands on the mat too (the Table's socket brings it as
      // well, and the mat shows a roll once).
      function roll() {
        var f = formula(counts, mod);
        if (!f || busy) return;
        busy = true; paintPool(); errEl.hidden = true;
        C.diceTray.pending(1);
        C.apiFetch('/campaigns/' + encodeURIComponent(cid) + '/rolls', { method: 'POST', body: { formula: f, mode: 'public' } }).then(function (res) {
          return res.json().catch(function () { return {}; }).then(function (data) { return { ok: res.ok, data: data }; });
        }).then(function (out) {
          if (!out.ok) { errEl.hidden = false; errEl.textContent = (out.data && (out.data.message || out.data.error)) || 'The roll didn’t go through. Try again.'; return; }
          out.data.mine = true;
          tray.own(out.data);
          tray.roll(out.data);
        }).catch(function () {
          errEl.hidden = false; errEl.textContent = 'The roll didn’t go through. Check your connection and try again.';
        }).then(function () { C.diceTray.pending(-1); busy = false; paintPool(); });
      }

      function onClick(ev) {
        var b = ev.target.closest('button');
        if (!b || !el.contains(b)) return;
        if (b === goBtn) { roll(); return; }
        var s = b.getAttribute('data-s');
        if (s) { if (total() < MAX_DICE) counts[s] = (counts[s] || 0) + 1; paintPool(); return; }
        var q = b.getAttribute('data-q');
        if (q != null) { var pick = QUICK[+q]; counts = Object.assign({}, pick.c); mod = pick.m; paintPool(); return; }
        if (b.hasAttribute('data-clear')) { counts = {}; mod = 0; paintPool(); return; }
        var m = b.getAttribute('data-m');
        if (m) { mod = Math.max(-5, Math.min(9, mod + +m)); paintPool(); return; }
        var key = b.getAttribute('data-dtr-set');
        if (key && ALLOWED[key] && ALLOWED[key].indexOf(b.getAttribute('data-v')) >= 0) {
          kept[key] = b.getAttribute('data-v');
          apply();
          if (saved) saved.set(key, kept[key]);
        }
      }
      // A right-click on a die button takes one away.
      function onMenu(ev) {
        var b = ev.target.closest('.dtr-die');
        if (b && counts[b.getAttribute('data-s')]) { ev.preventDefault(); counts[b.getAttribute('data-s')]--; paintPool(); }
      }
      // The card's own keys work only while focus is in it and never while typing; they stop here so
      // the Table's R (open the roller) does not also fire.
      function onKey(ev) {
        if (typing(ev) || ev.ctrlKey || ev.metaKey || ev.altKey) return;
        if (ev.key === 'r' || ev.key === 'R') { ev.preventDefault(); ev.stopPropagation(); roll(); }
        else if (ev.key === 'c' || ev.key === 'C') { ev.preventDefault(); ev.stopPropagation(); counts = {}; mod = 0; paintPool(); }
      }
      // A live roll from the Table's socket; the core keeps only the public ones.
      function onRoll(ev) { tray.roll(ev.detail); }
      // The screen's saved style arrived or changed (another tab, or after the first load).
      function onStyle(ev) {
        var s = ev.detail || {}, changed = false;
        Object.keys(ALLOWED).forEach(function (k) { if (ALLOWED[k].indexOf(s[k]) >= 0 && s[k] !== kept[k]) { kept[k] = s[k]; changed = true; } });
        if (changed) apply();
      }
      el.addEventListener('click', onClick);
      el.addEventListener('contextmenu', onMenu);
      el.addEventListener('keydown', onKey);
      window.addEventListener('chronicle:roll-made', onRoll);
      if (dock) dock.addEventListener('dms-style', onStyle);
      el._dtr = { tray: tray, off: function () {
        el.removeEventListener('click', onClick); el.removeEventListener('contextmenu', onMenu); el.removeEventListener('keydown', onKey);
        window.removeEventListener('chronicle:roll-made', onRoll);
        if (dock) dock.removeEventListener('dms-style', onStyle);
      } };
    },
    destroy: function (el) {
      var d = el._dtr;
      if (!d) return;
      d.off(); d.tray.destroy(); el._dtr = null;
    }
  });
})();
