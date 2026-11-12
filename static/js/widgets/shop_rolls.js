/**
 * shop_rolls.js — rolls at a shop's counter: Haggle and Look closer.
 *
 *   ShopRolls.open({ base, goodId, goodName, price, action, characters, characterId, returnFocus, host })
 *
 * base is the shop's API root (/campaigns/:id/armory/shops/:eid). The sheet reads the test the
 * game system and the owner set (GET base/rolls), the server rolls it (POST base/rolls), and the
 * dice are thrown to land on the server's values in the look the player picked at the Table:
 * Rune 3D (Chronicle.dice), Quick 2D (Chronicle.tableDice) or numbers only. Every roll is logged
 * at the Table by the server, not here.
 *
 * When a roll is done, document gets a "shop-rolls:result" event whose detail is
 * { goodId, action, characterId, tier, haggle, look } so the shop can show the new price or note.
 *
 * Words from the server and the owner go into the page escaped; the note and extra as textContent.
 * The pure helpers are on ShopRolls.util so test/js can run them without a page.
 */
(function () {
  'use strict';
  var W = window;

  /* ---- Pure helpers ---- */
  var LOOKS = [['rune', 'Realistic 3D'], ['quick', 'Quick 2D'], ['none', 'Numbers only']];
  var TIER_WORD = { 3: 'A great roll', 2: 'A fair roll', 1: 'A poor roll' };
  function sign(n) { n = Number(n) || 0; return (n < 0 ? '− ' : '+ ') + Math.abs(n); }
  // "2d10 + 2" from the test's dice and the stat's modifier.
  function formula(dice, mod) { return String(dice || '') + (mod ? ' ' + sign(mod) : ''); }
  // What the button says before the roll: the stat's test, or the bare dice when the system adds nothing.
  function rollLabel(t) {
    if (!t) return 'Roll';
    var what = t.label ? t.label + ' test' : 'Roll';
    return what + ' (' + formula(t.dice, t.mod) + ')';
  }
  // The sum line under the dice: "9 + 8 + 2 = 19".
  function sumLine(dice, mod, total) {
    var parts = (dice || []).map(function (d) { return String(d.value); });
    if (mod) parts.push(mod < 0 ? '(' + mod + ')' : String(mod));
    return parts.join(' + ') + ' = ' + total;
  }
  // The tier card's heading and line for each action.
  function outcome(action, r) {
    if (action === 'haggle' && r.haggle) {
      var pct = r.haggle.pct;
      if (pct < 0) return { head: r.tier === 3 ? 'A great deal' : 'A fair deal', line: 'The price is now ' + r.haggle.price + ' ' + (r.haggle.currency || '') + ' for you today.' };
      if (pct > 0) return { head: 'The keeper takes offence', line: 'The price went up to ' + r.haggle.price + ' ' + (r.haggle.currency || '') + ' for you today, and that’s the last word on it.' };
      return { head: 'No change', line: 'The keeper holds the price.' };
    }
    if (action === 'look') {
      if (r.tier >= 2) return { head: r.tier === 3 ? 'You notice everything' : 'You notice something', line: '' };
      return { head: 'Nothing more to see', line: 'If there’s more to it, it got past you.' };
    }
    return { head: TIER_WORD[r.tier] || 'Rolled', line: r.outcome || '' };
  }
  // How hard the keeper is, in a sentence: "Easy", "Usual", "Hard" for tiered tests, "DC 15" for a DC.
  function againstLine(a) {
    if (!a) return '';
    if (/^DC \d+$/.test(a)) return ' You need ' + a + '.';
    var w = { Easy: 'easy-going today', Usual: 'as stubborn as ever', Hard: 'hard to move today' }[a];
    return w ? ' The keeper is ' + w + '.' : '';
  }
  // Which look the player picked at the Table, read from the same key the Table writes.
  function lookKey(campaignId, userId) { return 'chronicle.table.look.' + campaignId + '.' + userId; }
  function pickLook(stored, hasRune, hasQuick) {
    var l = stored || 'quick';
    if (l === 'rune' && !hasRune) l = hasQuick ? 'quick' : 'none';
    if (l === 'quick' && !hasQuick) l = 'none';
    return l;
  }
  function motion() {
    var m = document.documentElement.getAttribute('data-motion');
    if (m === 'off' || m === 'calm') return m;
    return W.matchMedia && W.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'off' : 'full';
  }

  /* ---- Page ---- */
  function esc(s) { return W.Chronicle && Chronicle.escapeHtml ? Chronicle.escapeHtml(String(s == null ? '' : s)) : String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return '&#' + c.charCodeAt(0) + ';'; }); }
  function table() { return document.querySelector('.tt-host[data-widget="table"]'); }
  function store(key, val) {
    try { if (val === undefined) return W.localStorage.getItem(key); W.localStorage.setItem(key, val); } catch (e) { /* nothing remembered */ }
    return null;
  }
  function look() {
    var t = table(), k = t ? lookKey(t.getAttribute('data-campaign-id'), t.getAttribute('data-user-id')) : '';
    var C = W.Chronicle || {};
    return { key: k, now: pickLook(k ? store(k) : '', !!(C.dice && C.dice.roll), !!(C.tableDice && C.tableDice.throwInto)) };
  }

  var CSS = [
    '.srl-scrim{position:fixed;inset:0;background:rgb(8 6 4/.4);z-index:1190}',
    '.srl{position:fixed;left:50%;bottom:var(--srl-bottom,0px);transform:translateX(-50%);width:min(420px,100vw);box-sizing:border-box;max-height:min(78vh,620px);overflow:auto;z-index:1200;',
    'background:var(--color-card-bg,#fff);color:var(--color-text-primary,#111827);border-radius:18px 18px 0 0;padding:8px 16px calc(16px + env(safe-area-inset-bottom,0px));box-shadow:0 -10px 30px rgb(0 0 0/.3)}',
    '.srl-grab{width:40px;height:4px;border-radius:2px;background:var(--color-border,#e5e7eb);margin:2px auto 8px}',
    '.srl-h{display:flex;align-items:baseline;gap:8px}.srl-h h3{margin:0;flex:1;font-size:1.05rem}',
    '.srl-x{border:0;background:none;color:var(--color-text-secondary,#6b7280);min-width:44px;min-height:44px;margin:-10px -10px 0 0;font-size:1.2rem;cursor:pointer}',
    '.srl-sub{color:var(--color-text-secondary,#6b7280);font-size:.82rem;margin:2px 0 10px}',
    '.srl-btn{display:flex;align-items:center;justify-content:center;width:100%;min-height:48px;border-radius:12px;border:1px solid var(--color-border,#e5e7eb);background:var(--color-card-bg,#fff);color:inherit;font:inherit;font-weight:600;margin-top:8px;cursor:pointer}',
    '.srl-btn.pri{background:var(--color-accent,#4f46e5);border-color:var(--color-accent,#4f46e5);color:#fff}.srl-btn[disabled]{opacity:.55;cursor:default}',
    '.srl-tray{position:relative;height:84px;margin:8px 0 4px}',
    '.srl-nums{display:flex;justify-content:center;gap:10px;margin:8px 0 4px;min-height:54px}.srl-nums span{width:50px;height:50px;border-radius:12px;display:grid;place-items:center;font-weight:700;font-size:1.3rem;border:1px solid var(--color-border,#d1d5db)}',
    '.srl-sum{text-align:center;font-size:.85rem;color:var(--color-text-secondary,#6b7280)}.srl-sum b{color:var(--color-text-primary,#111827);font-size:1rem}',
    '.srl-tier{border-radius:12px;padding:10px 12px;margin-top:8px;font-size:.9rem}.srl-tier b{display:block}',
    '.srl-t3{background:color-mix(in srgb,#1b6e45 12%,transparent);color:#1b6e45}.srl-t2{background:color-mix(in srgb,#8a5a00 12%,transparent);color:#8a5a00}.srl-t1{background:color-mix(in srgb,#b3261e 12%,transparent);color:#b3261e}',
    '.dark .srl-t3{color:#6fd1a0}.dark .srl-t2{color:#f0c76a}.dark .srl-t1{color:#ff8c84}',
    '.srl-tier span{color:var(--color-text-primary,#111827)}',
    '.srl-secret{background:#f2ead8;color:#3a2c18;border-radius:10px;padding:10px 12px;font-family:Georgia,serif;margin-top:8px}.srl-secret small{display:block;font:.68rem system-ui,sans-serif;letter-spacing:.08em;text-transform:uppercase;opacity:.7;margin-bottom:2px}',
    '.srl-log{display:flex;gap:8px;align-items:center;font-size:.76rem;color:var(--color-text-secondary,#6b7280);margin-top:10px}.srl-log i{width:8px;height:8px;border-radius:50%;background:var(--color-accent,#4f46e5);flex:none}',
    '.srl-row{display:flex;gap:8px;align-items:center;flex-wrap:wrap;font-size:.82rem;margin-top:8px}.srl-row select{min-height:36px;font:inherit}',
    '.srl-seg{display:flex;border:1px solid var(--color-border,#e5e7eb);border-radius:10px;overflow:hidden;flex:1}.srl-seg button{flex:1;border:0;background:none;color:inherit;font:inherit;min-height:36px;font-size:.78rem;cursor:pointer}.srl-seg button[aria-pressed=true]{background:var(--color-text-primary,#111827);color:var(--color-card-bg,#fff);font-weight:600}',
    '.srl-err{color:#b3261e;font-size:.82rem;margin:6px 0 0}.srl-err:empty{display:none}',
    '.srl.in{animation:srl-up .26s cubic-bezier(.2,.8,.2,1)}@keyframes srl-up{from{transform:translate(-50%,100%)}to{transform:translateX(-50%)}}',
    '.srl.calm{animation:srl-fade .2s}@keyframes srl-fade{from{opacity:0}to{opacity:1}}'
  ].join('\n');
  function style() {
    if (document.getElementById('srl-style')) return;
    var st = document.createElement('style'); st.id = 'srl-style'; st.textContent = CSS; document.head.appendChild(st);
  }

  var cur = null;   // the open sheet's state
  function close() {
    if (!cur) return;
    var c = cur; cur = null;
    document.removeEventListener('keydown', c.key, true);
    c.sheet.remove(); c.scrim.remove();
    if (c.o.returnFocus && c.o.returnFocus.focus) c.o.returnFocus.focus();
  }

  function open(o) {
    close(); style();
    var scrim = document.createElement('div'); scrim.className = 'srl-scrim';
    var sheet = document.createElement('div'); sheet.className = 'srl';
    sheet.setAttribute('role', 'dialog'); sheet.setAttribute('aria-modal', 'true');
    var m = motion(); if (m === 'full') sheet.classList.add('in'); else if (m === 'calm') sheet.classList.add('calm');
    // Inside an open modal dialog (the shop room is one) anything outside it is inert, so the sheet goes in it.
    var dlgs = document.querySelectorAll('dialog[open]'), host = o.host || (dlgs.length ? dlgs[dlgs.length - 1] : document.body);
    host.appendChild(scrim); host.appendChild(sheet);
    cur = { o: o, sheet: sheet, scrim: scrim, test: null, busy: false, result: null, err: '', charId: o.characterId || ((o.characters || [])[0] || {}).id || '' };
    // Escape closes only the sheet, not the shop around it.
    cur.key = function (e) { if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(); } else if (e.key === 'Tab') trap(e); };
    document.addEventListener('keydown', cur.key, true);
    scrim.addEventListener('click', close);
    sheet.addEventListener('click', onClick);
    sheet.addEventListener('change', onChange);
    paint();
    load();
    var f = sheet.querySelector('[data-srl="roll"]:not([disabled])') || sheet.querySelector('.srl-x'); if (f) f.focus();
  }
  function trap(e) {
    var f = cur.sheet.querySelectorAll('button:not([disabled]),select'); if (!f.length) return;
    var a = f[0], z = f[f.length - 1];
    if (e.shiftKey && document.activeElement === a) { e.preventDefault(); z.focus(); }
    else if (!e.shiftKey && document.activeElement === z) { e.preventDefault(); a.focus(); }
  }
  function load() {
    var c = cur; if (!c.charId) { c.err = 'Choose a character to roll for.'; paint(); return; }
    Chronicle.apiFetch(c.o.base + '/rolls?character=' + encodeURIComponent(c.charId)).then(function (r) { return r.ok ? r.json() : r.json().then(function (j) { throw new Error(j && j.message || 'The rolls couldn’t be loaded.'); }); })
      .then(function (j) {
        if (cur !== c) return;
        var a = document.activeElement, had = !a || a === document.body || a.tagName === 'DIALOG' || (a.getAttribute && a.getAttribute('data-srl') === 'close');
        c.test = j; c.err = ''; paint();
        var f = c.sheet.querySelector('[data-srl="roll"]:not([disabled])'); if (f && had) f.focus();
      })
      .catch(function (e) { if (cur !== c) return; c.err = e.message; paint(); });
  }

  // The test for this action, with the system's dice from the test as a whole.
  function testFor(c) {
    var T = c.test && c.test.test, t = T && T[c.o.action];
    if (!t) return null;
    var o = { dice: T.dice }; for (var k in t) if (Object.prototype.hasOwnProperty.call(t, k)) o[k] = t[k];
    return o;
  }
  function paint() {
    var c = cur, o = c.o, t = testFor(c), r = c.result, h = '';
    var title = (o.action === 'haggle' ? 'Haggle over ' : 'Look closer at ') + (o.goodName || 'this');
    c.sheet.setAttribute('aria-label', title);
    h += '<div class="srl-grab" aria-hidden="true"></div><div class="srl-h"><h3>' + esc(title) + '</h3>' + (o.price ? '<span>' + esc(o.price) + '</span>' : '') + '<button type="button" class="srl-x" data-srl="close" aria-label="Close">✕</button></div>';
    if (!r) {
      var against = c.test && c.test.test ? c.test.test.against : '';
      h += '<p class="srl-sub">' + (o.action === 'haggle' ? 'A great roll takes the most off. A poor one puts the price up for today.' : 'A fair roll shows what the keeper isn’t saying. A great one shows more.') + againstLine(against) + '</p>';
      if ((o.characters || []).length > 1) h += '<div class="srl-row"><label>Rolling for <select data-srl="char">' + o.characters.map(function (ch) { return '<option value="' + esc(ch.id) + '"' + (ch.id === c.charId ? ' selected' : '') + '>' + esc(ch.name) + '</option>'; }).join('') + '</select></label></div>';
      if (t && t.missing) h += '<p class="srl-sub">' + esc(t.label) + ' isn’t a number on this sheet, so nothing is added.</p>';
      h += '<button type="button" class="srl-btn pri" data-srl="roll"' + (!t || c.busy ? ' disabled' : '') + '>' + esc(c.busy ? 'Rolling…' : rollLabel(t)) + '</button>';
    } else {
      h += '<div class="srl-tray" aria-hidden="true"></div><div class="srl-nums" hidden>' + r.dice.map(function (d) { return '<span>' + (Number(d.value) | 0) + '</span>'; }).join('') + '</div>';
      h += '<div class="srl-after" hidden><p class="srl-sum">' + esc(sumLine(r.dice, r.modifier, '')) + '<b>' + (Number(r.total) | 0) + '</b> · tier ' + (Number(r.tier) | 0) + '</p>';
      var oc = outcome(o.action, r);
      h += '<div class="srl-tier srl-t' + (Number(r.tier) | 0) + '" role="status"><b>' + esc(oc.head) + '</b>' + (oc.line ? '<span>' + esc(oc.line) + '</span>' : '') + '</div>';
      if (o.action === 'look' && r.look && r.look.note) h += '<div class="srl-secret"><small>What you notice</small><span data-srl-note></span></div>';
      if (o.action === 'look' && r.look && r.look.extra) h += '<div class="srl-secret"><small>And more</small><span data-srl-extra></span></div>';
      h += '<button type="button" class="srl-btn' + (o.action === 'haggle' && r.haggle && r.haggle.pct < 0 ? ' pri' : '') + '" data-srl="close">Done</button>';
      h += '<p class="srl-log"><i aria-hidden="true"></i>Logged at the Table for the ' + esc(W.Chronicle && Chronicle.gmTerm ? Chronicle.gmTerm() : 'Director') + '.</p></div>';
    }
    var lk = look();
    h += '<div class="srl-row"><span>Dice</span><div class="srl-seg" role="group" aria-label="How dice are shown">' + LOOKS.map(function (l) { return '<button type="button" data-srl-look="' + l[0] + '" aria-pressed="' + (lk.now === l[0]) + '">' + l[1] + '</button>'; }).join('') + '</div></div>';
    h += '<p class="srl-err" role="alert">' + esc(c.err) + '</p>';
    // A redraw keeps focus on the same control, so a keyboard user isn't thrown back to the page.
    var fa = document.activeElement, fk = fa && c.sheet.contains(fa) ? (fa.getAttribute('data-srl') ? '[data-srl="' + fa.getAttribute('data-srl') + '"]' : fa.getAttribute('data-srl-look') ? '[data-srl-look="' + fa.getAttribute('data-srl-look') + '"]' : '') : '';
    c.sheet.innerHTML = h;
    if (fk) { var back = c.sheet.querySelector(fk); if (back) back.focus(); }
    if (r && r.look) { var n = c.sheet.querySelector('[data-srl-note]'), x = c.sheet.querySelector('[data-srl-extra]'); if (n) n.textContent = r.look.note; if (x) x.textContent = r.look.extra; }
  }

  function roll() {
    var c = cur; if (c.busy) return;
    c.busy = true; c.err = ''; paint();
    Chronicle.apiFetch(c.o.base + '/rolls', { method: 'POST', body: { relationId: c.o.goodId, characterId: c.charId, action: c.o.action } })
      .then(function (r) { return r.json().then(function (j) { if (!r.ok) throw new Error(j && j.message || 'The roll didn’t go through. Try again.'); return j; }); })
      .then(function (j) {
        if (cur !== c) return;
        c.busy = false; c.result = j; paint(); show(c);
        document.dispatchEvent(new CustomEvent('shop-rolls:result', { detail: { goodId: c.o.goodId, action: c.o.action, characterId: c.charId, tier: j.tier, haggle: j.haggle || null, look: j.look || null } }));
      })
      .catch(function (e) { if (cur !== c) return; c.busy = false; c.err = e.message; paint(); });
  }
  // Throw the server's dice, then show the sum and the outcome; show them anyway if the throw can't happen.
  function show(c) {
    var s = c.sheet, after = s.querySelector('.srl-after'), nums = s.querySelector('.srl-nums'), tray = s.querySelector('.srl-tray'), done = false;
    function reveal() { if (done || cur !== c) return; done = true; nums.hidden = false; tray.hidden = true; after.hidden = false; var b = after.querySelector('[data-srl="close"]'); if (b) b.focus(); }
    setTimeout(reveal, 8000);
    var dice = c.result.dice.map(function (d) { return { sides: Number(d.sides) | 0, value: Number(d.value) | 0 }; });
    var lk = look().now, C = W.Chronicle || {};
    if (lk === 'rune') {
      Promise.resolve().then(function () {
        return C.dice.roll([{ label: c.o.goodName || 'Shop roll', dice: dice.map(function (d) { return { sides: d.sides, value: d.value, crit: 'none' }; }) }], {
          from: s, onSettle: reveal,
          describe: function (g, v) { return (v || []).join(' and ') + ', ' + c.result.total + ' in all.'; }
        });
      }).then(reveal, reveal);
    } else if (lk === 'quick') {
      C.tableDice.throwInto(tray, dice, { fromLeft: true }).then(reveal, reveal);
    } else reveal();
  }

  function onClick(e) {
    var b = e.target.closest && e.target.closest('button'); if (!b || !cur) return;
    var a = b.getAttribute('data-srl'), l = b.getAttribute('data-srl-look');
    if (a === 'close') close();
    else if (a === 'roll') roll();
    else if (l) {
      var k = look().key, ok = LOOKS.some(function (x) { return x[0] === l; });
      if (k && ok) store(k, l);
      paint();
      var again = cur.sheet.querySelector('[data-srl-look="' + (ok ? l : 'quick') + '"]'); if (again) again.focus();
      if (cur.result) show(cur);
    }
  }
  function onChange(e) {
    if (!cur || e.target.getAttribute('data-srl') !== 'char') return;
    var id = e.target.value;
    if (!(cur.o.characters || []).some(function (ch) { return ch.id === id; })) return;
    cur.charId = id; cur.test = null; paint(); load();
  }

  W.ShopRolls = {
    open: open, close: close,
    util: { againstLine: againstLine, formula: formula, rollLabel: rollLabel, sumLine: sumLine, outcome: outcome, lookKey: lookKey, pickLook: pickLook, LOOKS: LOOKS }
  };
})();
