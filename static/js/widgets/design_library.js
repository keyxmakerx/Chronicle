/**
 * design_library.js — the saved-designs library for the Owner: "My designs" and
 * "Shared with me" beside a builder's starting points, saving the current
 * design, and the Share sheet (share code, link, named owners). It is generic
 * over a design kind; the shop room builder (shop_room_builder.js) is its first
 * host, and mounts it with:
 *   kind        'room-theme'
 *   campaign()  { url: '/campaigns/<id>', csrf }
 *   theme()     the current design's data
 *   apply(data) draw data as the current design (unsaved until the host saves)
 *   say(text)   announce in the host's status line
 * The same file serves the page a design link opens (widget `design_library`).
 * Everything here is Owner-only on the server; the widget just never offers it
 * to anyone else, because the builder is not mounted for players.
 *
 * The pure helpers (parseCode, systemName, previewNotes, ...) are on
 * DesignLibrary.util so test/js can exercise them without a page.
 */
(function () {
  'use strict';

  var PREFIX = 'chronicle-design:v1:';
  var KIND_LABEL = { 'room-theme': 'Room theme', 'crafting-circle': 'Magic circle', 'crafting-item-look': 'Item look', 'crafting-bench': 'Crafting bench' };
  var SYSTEMS = { dnd5e: 'D&D 5e', drawsteel: 'Draw Steel', pathfinder2e: 'Pathfinder 2e', daggerheart: 'Daggerheart' };

  /* ---- Pure helpers ---- */
  function nice(v) { v = String(v || '').replace(/[-_]+/g, ' ').trim(); return v.charAt(0).toUpperCase() + v.slice(1); }
  function kindLabel(k) { return KIND_LABEL[k] || nice(k) || 'Design'; }
  function systemName(id) {
    if (!id) return 'no game system';
    if (SYSTEMS[id]) return SYSTEMS[id];
    return id.indexOf('custom:') === 0 ? 'a custom game system' : nice(id);
  }
  // A pasted code is read here only to fail fast and to name what is wrong; the
  // server reads it again and is the authority.
  function decodeBase64Url(s) {
    s = s.replace(/-/g, '+').replace(/_/g, '/');
    while (s.length % 4) s += '=';
    var bin = atob(s);
    try {
      if (typeof TextDecoder !== 'undefined') {
        var b = new Uint8Array(bin.length);
        for (var i = 0; i < bin.length; i++) b[i] = bin.charCodeAt(i);
        return new TextDecoder('utf-8', { fatal: true }).decode(b);
      }
      return decodeURIComponent(escape(bin));
    } catch (e) { throw new Error('not text'); }
  }
  function parseCode(text) {
    var code = String(text || '').replace(/\s+/g, '');
    if (!code) return { ok: false, error: 'Paste a design code first.' };
    if (code.indexOf(PREFIX) !== 0) return { ok: false, error: 'That is not a Chronicle design code. It should start with ' + PREFIX };
    var enc = code.slice(PREFIX.length).replace(/=+$/, '');
    if (enc.length > 22000) return { ok: false, error: 'That design code is too large.' };
    var body;
    try { body = JSON.parse(decodeBase64Url(enc)); } catch (e) { return { ok: false, error: 'That design code is damaged. Copy it again in full.' }; }
    if (!body || typeof body !== 'object' || Array.isArray(body) || !body.kind || !String(body.name || '').trim()) {
      return { ok: false, error: 'That design code is damaged. Copy it again in full.' };
    }
    return { ok: true, code: code, body: { kind: String(body.kind), name: String(body.name), makerName: String(body.maker_name || ''), systemId: String(body.system_id || '') } };
  }
  // What the preview says beyond the design's name: the game-system note first,
  // then each thing this campaign lacks, as the server worded it.
  function previewNotes(p, campaignSystem) {
    var out = [];
    if (p && p.sameSystem === false) {
      out.push({ tone: 'warn', text: 'Made for ' + systemName(p.systemId) + '. This campaign uses ' + systemName(campaignSystem) + '. Parts it uses may be named differently here.' });
    }
    ((p && p.lacks) || []).forEach(function (l) { out.push({ tone: 'warn', text: String(l) }); });
    return out;
  }
  function shareUrl(origin, linkUrl) { return linkUrl ? String(origin || '').replace(/\/+$/, '') + linkUrl : ''; }
  // The campaigns of a search hit that run the design's game system.
  function onSystem(owner, systemId) {
    return ((owner && owner.campaigns) || []).filter(function (c) { return c.systemId === systemId; });
  }
  function esc(t) {
    t = String(t == null ? '' : t);
    return window.Chronicle && window.Chronicle.escapeHtml ? window.Chronicle.escapeHtml(t) : t.replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; });
  }

  /* ---- State: one library view at a time (the builder mounts one) ---- */
  var H = null;        // the host that mounted us
  var el = null;       // the element we draw the groups in
  var S = { mine: [], shared: [], system: '', loaded: false, sel: '', mode: '', msg: '', err: '', draft: '', paste: '',
    preview: null, previewFrom: null, sheet: false, tab: 'code', share: null, q: '', hits: [], busy: false, shareErr: '', shareMsg: '' };
  var searchT = 0, opener = null;

  function base() { return H && H.campaign ? H.campaign().url : ''; }
  function call(method, path, body) {
    return window.Chronicle.apiAction(base() + path, { method: method, body: body, csrfToken: H && H.campaign ? H.campaign().csrf : undefined });
  }
  function say(t) { S.msg = t || ''; S.err = ''; if (H && H.say) H.say(t || ''); paintMsg(); }
  function fail(e) { S.err = (e && e.message) || 'Something went wrong. Try again.'; S.msg = ''; paintMsg(); }
  function mineById(id) { for (var i = 0; i < S.mine.length; i++) if (S.mine[i].id === id) return S.mine[i]; return null; }

  var CSS = [
    '.dl{display:grid;grid-template-columns:minmax(0,1fr);gap:10px;margin-top:6px}.dl small.dl-g{display:block;font-size:.7rem;text-transform:uppercase;letter-spacing:.05em;color:var(--color-text-muted,#6b7280);margin:2px 0 4px}',
    '.dl .shr-chips{display:flex;flex-wrap:wrap;gap:6px}.dl .shr-chip{min-height:32px}.dl .shr-chip em{font-style:normal;font-size:.7rem;color:var(--color-text-secondary,#6b7280)}',
    '.dl button:focus-visible,.dl input:focus-visible,.dl textarea:focus-visible,.dl-sheet button:focus-visible,.dl-sheet input:focus-visible,.dl-sheet textarea:focus-visible{outline:2px solid var(--shr-acc,#4f46e5);outline-offset:2px}',
    '.dl-detail{border:1px solid var(--color-border,#e5e7eb);border-radius:8px;padding:8px 10px;background:color-mix(in srgb,var(--shr-acc,#4f46e5) 6%,var(--color-card-bg,#fff));display:grid;gap:6px}',
    '.dl-detail b{font-size:.84rem;color:var(--color-text-primary,#111827)}.dl-detail span.dl-sub{font-size:.76rem;color:var(--color-text-secondary,#6b7280)}',
    '.dl-acts{display:flex;flex-wrap:wrap;gap:4px 12px}.dl-acts button,.dl-link{border:0;background:none;padding:4px 0;min-height:28px;font:inherit;font-size:.8rem;font-weight:600;cursor:pointer;color:var(--shr-acc,#4f46e5)}',
    '.dl-acts button.dl-bad,.dl-bad{color:var(--color-danger,#b91c1c)}',
    '.dl-row{display:flex;gap:6px;align-items:stretch}.dl-row .dl-in{flex:1 1 auto;width:0}.dl-in{box-sizing:border-box;width:100%;min-width:0;border:1px solid var(--color-input-border,#d1d5db);border-radius:6px;padding:6px 8px;background:var(--color-input-bg,#fff);color:var(--color-text-primary,#111827);font:inherit}',
    '.dl-btn{border:1px solid var(--color-border,#e5e7eb);border-radius:8px;padding:6px 12px;min-height:34px;background:var(--color-card-bg,#fff);color:var(--color-text-body,#374151);font:inherit;font-size:.82rem;font-weight:600;cursor:pointer}',
    '.dl-btn.pri{background:var(--shr-acc,#4f46e5);border-color:var(--shr-acc,#4f46e5);color:#fff}.dl-btn:disabled{opacity:.5;cursor:default}',
    '.dl-lab{font-size:.72rem;text-transform:uppercase;letter-spacing:.06em;color:var(--color-text-muted,#6b7280)}',
    '.dl textarea,.dl-sheet textarea{width:100%;min-width:0;box-sizing:border-box;border:1px solid var(--color-input-border,#d1d5db);border-radius:6px;padding:6px 8px;background:var(--color-input-bg,#fff);color:var(--color-text-primary,#111827);font:.72rem/1.4 ui-monospace,Menlo,Consolas,monospace;resize:vertical;overflow-wrap:anywhere}',
    '.dl-err{margin:0;font-size:.78rem;color:var(--color-danger,#b91c1c)}.dl-ok{margin:0;font-size:.78rem;color:var(--color-text-secondary,#6b7280)}',
    '.dl-card{border:1px solid var(--color-border,#e5e7eb);border-radius:10px;padding:10px;background:var(--color-card-bg,#fff);display:grid;gap:8px}',
    '.dl-card h4{margin:0;font-size:.82rem;display:flex;justify-content:space-between;align-items:center}.dl-card .nm{font-size:1rem;font-weight:700;color:var(--color-text-primary,#111827);overflow-wrap:anywhere}',
    '.dl-tag{display:inline-block;font-size:.66rem;font-weight:700;letter-spacing:.05em;text-transform:uppercase;border:1px solid var(--shr-acc,#4f46e5);color:var(--shr-acc,#4f46e5);border-radius:4px;padding:1px 6px;margin-right:6px}',
    '.dl-note{border-left:3px solid #d99a2b;background:#fff6e0;color:#5c3a00;padding:6px 10px;font-size:.78rem;border-radius:0 6px 6px 0}',
    '.dl-sheet{position:absolute;left:0;right:0;bottom:0;z-index:40;max-height:88%;display:flex;flex-direction:column;background:var(--color-card-bg,#fff);color:var(--color-text-body,#374151);border-top:1px solid var(--color-border,#e5e7eb);border-radius:14px 14px 0 0;box-shadow:0 -18px 40px -16px rgb(0 0 0/.5)}',
    '.dl-sh{display:flex;justify-content:space-between;align-items:center;gap:8px;padding:10px 14px;border-bottom:1px solid var(--color-border,#e5e7eb)}.dl-sh h3{margin:0;font-size:.95rem;overflow-wrap:anywhere}',
    '.dl-sb{padding:10px 14px;overflow:auto;display:grid;gap:10px}.dl-sf{padding:10px 14px;border-top:1px solid var(--color-border,#e5e7eb);display:flex;gap:10px;align-items:center;flex-wrap:wrap;font-size:.76rem;color:var(--color-text-secondary,#6b7280)}',
    '.dl-tabs{display:grid;grid-auto-flow:column;grid-auto-columns:1fr;border:1px solid var(--color-border,#e5e7eb);border-radius:8px;overflow:hidden}',
    '.dl-tabs button{border:0;background:transparent;color:var(--color-text-secondary,#6b7280);padding:8px 4px;min-height:40px;font:inherit;font-size:.78rem;cursor:pointer}',
    '.dl-tabs button[aria-selected=true]{background:var(--color-text-primary,#111827);color:var(--color-bg-primary,#fff);font-weight:600}',
    '.dl-chipx{display:inline-flex;align-items:center;gap:4px;border:1px solid var(--color-border,#e5e7eb);border-radius:999px;padding:2px 4px 2px 10px;font-size:.8rem}.dl-chipx button{border:0;background:none;color:inherit;cursor:pointer;min-width:28px;min-height:28px;font-size:1rem;line-height:1}',
    '.dl-hits{list-style:none;margin:0;padding:0;border-top:1px solid var(--color-border,#e5e7eb)}.dl-hits li{display:flex;justify-content:space-between;align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid var(--color-border,#e5e7eb)}',
    '.dl-hits b{font-size:.84rem;display:block}.dl-hits span{display:block;font-size:.74rem;color:var(--color-text-secondary,#6b7280)}',
    '.dl-sw{display:flex;align-items:center;gap:8px}.dl-sw button{min-height:36px}.dl-sw button[aria-checked=true]{background:var(--shr-acc,#4f46e5);color:#fff;border-color:var(--shr-acc,#4f46e5)}',
    '@media (max-width:640px){.dl-sheet{max-height:92%}}'
  ].join('\n');
  function style() {
    if (document.getElementById('dl-style')) return;
    var st = document.createElement('style'); st.id = 'dl-style'; st.textContent = CSS; document.head.appendChild(st);
  }

  /* ---- Drawing the groups ---- */
  function originLine(d) {
    var by = d.makerName ? ' by ' + d.makerName : '';
    var how = d.origin === 'code' ? 'Added from a code' + by : d.origin === 'shared' ? 'Shared' + by + (d.originRef ? ', from ' + d.originRef : '') : 'Made in this campaign';
    return kindLabel(d.kind) + ' · ' + how;
  }
  function chip(id, name, pressed, extra) {
    return '<button type="button" class="shr-chip" data-dl-pick="' + esc(id) + '" aria-pressed="' + pressed + '">' + esc(name) + (extra ? ' <em>' + esc(extra) + '</em>' : '') + '</button>';
  }
  function detail() {
    var d = mineById(S.sel);
    if (!d) return '';
    var h = '<div class="dl-detail" role="group" aria-label="' + esc(d.name) + '"><b>' + esc(d.name) + (d.shared ? ' <span class="dl-tag">Shared</span>' : '') + '</b><span class="dl-sub">' + esc(originLine(d)) + (d.note ? '. ' + esc(d.note) : '') + '</span>';
    if (S.mode === 'rename') {
      h += '<div class="dl-row"><input class="dl-in" id="dl-rename" data-dl-key="rename" maxlength="60" aria-label="New name for ' + esc(d.name) + '" value="' + esc(S.draft) + '"><button type="button" class="dl-btn pri" data-dl-do="rename-ok">Rename</button><button type="button" class="dl-btn" data-dl-do="cancel">Cancel</button></div>';
    } else if (S.mode === 'delete') {
      h += '<p class="dl-ok">Delete “' + esc(d.name) + '”? Copies others already took stay with them.</p><div class="dl-acts"><button type="button" class="dl-bad" data-dl-do="delete-ok">Delete</button><button type="button" data-dl-do="cancel">Keep it</button></div>';
    } else {
      h += '<div class="dl-acts"><button type="button" data-dl-do="rename">Rename</button><button type="button" data-dl-do="share">Share…</button>' +
        (d.shared ? '<button type="button" class="dl-bad" data-dl-do="stop">Stop sharing</button>' : '') + '<button type="button" class="dl-bad" data-dl-do="delete">Delete</button></div>';
    }
    return h + '</div>';
  }
  function card() {
    var v = S.preview; if (!v) return '';
    var p = v.p, h = '<div class="dl-card" role="region" aria-label="Import preview"><h4>Import preview <button type="button" class="dl-link" data-dl-do="cancel-preview">Cancel</button></h4>' +
      '<div><div class="nm">' + esc(p.name) + '</div>' + (p.makerName ? '<span class="dl-sub">by ' + esc(p.makerName) + '</span>' : '') + '</div>' +
      '<div><span class="dl-tag">' + esc(kindLabel(p.kind)) + '</span><span class="dl-sub">Made for ' + esc(systemName(p.systemId)) + '</span></div>';
    if (p.note) h += '<span class="dl-sub">' + esc(p.note) + '</span>';
    h += '<span class="dl-sub">Only choices from known parts. No pictures come with it.</span>';
    previewNotes(p, S.system).forEach(function (n) { h += '<div class="dl-note" role="note">' + esc(n.text) + '</div>'; });
    return h + '<div class="dl-row"><button type="button" class="dl-btn pri" data-dl-do="take"' + (S.busy ? ' disabled' : '') + '>Add to my designs</button><button type="button" class="dl-btn" data-dl-do="cancel-preview">Cancel</button></div></div>';
  }
  function html() {
    var h = '<div class="dl"><div><small class="dl-g">My designs</small><span class="shr-chips" role="group" aria-label="My designs">';
    h += S.mine.length ? S.mine.map(function (d) { return chip(d.id, d.name, d.id === S.sel, ''); }).join('') :
      '<span class="dl-ok">' + (S.loaded ? 'Nothing saved yet. Name this room below to keep it.' : 'Loading…') + '</span>';
    h += '</span></div>' + detail();
    h += '<div><small class="dl-g">Shared with me</small><span class="shr-chips" role="group" aria-label="Shared with me">';
    h += S.shared.length ? S.shared.map(function (d) { return chip('s:' + d.id, d.name, S.preview && S.preview.id === d.id, d.makerName ? 'from ' + d.makerName : ''); }).join('') :
      '<span class="dl-ok">' + (S.loaded ? 'Nothing shared with you for this game system.' : '') + '</span>';
    h += '</span></div>';
    h += '<div><label class="dl-lab" for="dl-save">Save as my design</label><div class="dl-row"><input class="dl-in" id="dl-save" data-dl-key="save" maxlength="60" placeholder="Name this room" value="' + esc(S.mode === '' ? S.draft : '') + '">' +
      '<button type="button" class="dl-btn pri" data-dl-do="save"' + (S.busy ? ' disabled' : '') + '>Save as my design</button></div></div>';
    h += '<div><label class="dl-lab" for="dl-paste">Paste a code</label><textarea id="dl-paste" data-dl-key="paste" rows="3" cols="1" spellcheck="false" placeholder="' + PREFIX + '…">' + esc(S.paste) + '</textarea>' +
      '<div class="dl-row" style="margin-top:6px"><button type="button" class="dl-btn" data-dl-do="preview"' + (S.busy ? ' disabled' : '') + '>Preview</button></div></div>';
    h += card();
    return h + '<p class="dl-err" role="alert" data-dl-err></p><p class="dl-ok" role="status" aria-live="polite" data-dl-msg></p></div>';
  }
  function paintMsg() {
    if (!el) return;
    var e = el.querySelector('[data-dl-err]'), m = el.querySelector('[data-dl-msg]');
    if (e) e.textContent = S.err; if (m) m.textContent = S.msg;
  }
  // Redraws, keeping the caret in a text box the owner is typing in.
  function paint() {
    if (!el || !el.isConnected) return;
    var a = document.activeElement, key = a && el.contains(a) && a.getAttribute('data-dl-key'), pos = key && a.selectionStart;
    var pick = a && el.contains(a) && a.getAttribute('data-dl-pick'), act = a && el.contains(a) && a.getAttribute('data-dl-do');
    el.innerHTML = html();
    paintMsg();
    var again = key ? el.querySelector('[data-dl-key="' + key + '"]') : pick ? el.querySelector('[data-dl-pick="' + pick + '"]') :
      act ? el.querySelector('[data-dl-do="' + act + '"]') : null;
    if (again) { again.focus(); if (key && again.setSelectionRange && pos != null) try { again.setSelectionRange(pos, pos); } catch (x) { /* a field with no caret */ } }
  }

  /* ---- Loading and acting ---- */
  function load() {
    var q = '?kind=' + encodeURIComponent(H.kind);
    return Promise.all([call('GET', '/designs' + q), call('GET', '/designs/shared-with-me' + q)]).then(function (r) {
      S.mine = r[0].designs || []; S.system = r[0].systemId || ''; S.shared = r[1].designs || []; S.loaded = true;
      if (S.sel && !mineById(S.sel)) S.sel = '';
      paint();
    }).catch(function (e) { S.loaded = true; fail(e); paint(); });
  }
  function busy(on) { S.busy = on; if (el) Array.prototype.forEach.call(el.querySelectorAll('[data-dl-do="save"],[data-dl-do="preview"],[data-dl-do="take"]'), function (b) { b.disabled = on; }); }
  function pick(id) {
    S.mode = ''; S.err = ''; S.preview = null;
    if (id.indexOf('s:') === 0) { return previewShared(id.slice(2)); }
    var d = mineById(id); if (!d) return;
    S.sel = d.id; S.draft = '';
    H.apply(d.data); say('Using “' + d.name + '”. Save room to keep it.'); paint();
  }
  function previewShared(id) {
    busy(true);
    return call('GET', '/designs/shared/' + encodeURIComponent(id) + '/preview').then(function (p) {
      S.preview = { p: p, source: 'shared', id: id }; S.sel = ''; busy(false); paint();
    }).catch(function (e) { busy(false); fail(e); });
  }
  function save() {
    var name = (el.querySelector('[data-dl-key="save"]').value || '').trim();
    if (!name) { S.err = 'Give the design a name first.'; paintMsg(); return; }
    var data = H.theme(); data.name = name.slice(0, 60);
    busy(true);
    call('POST', '/designs', { kind: H.kind, name: name, data: data }).then(function (d) {
      S.draft = ''; S.sel = d.id; busy(false); say('Saved “' + d.name + '” to your designs.'); return load();
    }).catch(function (e) { busy(false); fail(e); });
  }
  function rename() {
    var d = mineById(S.sel), name = ((el.querySelector('[data-dl-key="rename"]') || {}).value || '').trim();
    if (!d) return;
    if (!name) { S.err = 'A design needs a name.'; paintMsg(); return; }
    call('PATCH', '/designs/' + encodeURIComponent(d.id), { name: name }).then(function (u) {
      S.mode = ''; S.draft = ''; say('Renamed to “' + u.name + '”.'); return load();
    }).catch(fail);
  }
  function remove() {
    var d = mineById(S.sel); if (!d) return;
    call('DELETE', '/designs/' + encodeURIComponent(d.id)).then(function () {
      S.mode = ''; S.sel = ''; say('Deleted “' + d.name + '”.'); return load();
    }).catch(fail);
  }
  function stop() {
    var d = mineById(S.sel); if (!d) return;
    call('DELETE', '/designs/' + encodeURIComponent(d.id) + '/share').then(function () {
      S.share = null; say('Stopped sharing “' + d.name + '”. Copies already taken stay with their owners.'); return load();
    }).catch(fail);
  }
  function preview() {
    var box = el.querySelector('[data-dl-key="paste"]'); S.paste = box ? box.value : S.paste;
    var r = parseCode(S.paste);
    if (!r.ok) { S.err = r.error; S.msg = ''; S.preview = null; paint(); return; }
    busy(true);
    call('POST', '/designs/code/preview', { code: r.code }).then(function (p) {
      S.preview = { p: p, source: 'code', code: r.code }; S.sel = ''; busy(false); S.err = ''; paint();
    }).catch(function (e) { S.preview = null; busy(false); fail(e); });
  }
  function take() {
    var v = S.preview; if (!v) return;
    busy(true);
    var req = v.source === 'code' ? call('POST', '/designs/code/take', { code: v.code }) : call('POST', '/designs/shared/' + encodeURIComponent(v.id) + '/take');
    req.then(function (d) {
      S.preview = null; S.paste = v.source === 'code' ? '' : S.paste; S.sel = d.id; busy(false);
      H.apply(d.data); say('Added “' + d.name + '” to your designs and used it here. Save room to keep it.'); return load();
    }).catch(function (e) { busy(false); fail(e); });
  }

  function onClick(e) {
    var b = e.target.closest && e.target.closest('button'); if (!b || !el.contains(b)) return;
    if (b.hasAttribute('data-dl-pick')) { pick(b.getAttribute('data-dl-pick')); return; }
    var act = b.getAttribute('data-dl-do'); if (!act) return;
    var d = mineById(S.sel);
    switch (act) {
      case 'save': save(); break;
      case 'rename': S.mode = 'rename'; S.draft = d ? d.name : ''; paint(); break;
      case 'rename-ok': rename(); break;
      case 'delete': S.mode = 'delete'; paint(); break;
      case 'delete-ok': remove(); break;
      case 'cancel': S.mode = ''; S.draft = ''; S.err = ''; paint(); break;
      case 'stop': stop(); break;
      case 'share': opener = b; openSheet(); break;
      case 'preview': preview(); break;
      case 'take': take(); break;
      case 'cancel-preview': S.preview = null; S.err = ''; paint(); break;
    }
  }
  function onInput(e) {
    var k = e.target.getAttribute && e.target.getAttribute('data-dl-key');
    if (k === 'paste') S.paste = e.target.value; else if (k === 'save' || k === 'rename') S.draft = e.target.value;
  }
  function onKey(e) {
    if (e.key === 'Enter' && e.target.getAttribute) {
      var k = e.target.getAttribute('data-dl-key');
      if (k === 'save') { e.preventDefault(); save(); } else if (k === 'rename') { e.preventDefault(); rename(); }
    }
  }

  /* ---- The Share sheet ---- */
  var sheetEl = null;
  function curDesign() { return mineById(S.sel); }
  function host() { return (el && el.closest('.shr-panel')) || el; }
  function openSheet() {
    var d = curDesign(); if (!d) return;
    S.sheet = true; S.tab = 'code'; S.share = null; S.q = ''; S.hits = []; S.shareErr = ''; S.shareMsg = '';
    paintSheet(true);
    call('GET', '/designs/' + encodeURIComponent(d.id) + '/share').then(function (st) { S.share = st; paintSheet(); }).catch(function (e) { S.shareErr = e.message; paintSheet(); });
  }
  function closeSheet() {
    S.sheet = false;
    if (sheetEl) { sheetEl.remove(); sheetEl = null; }
    if (opener && opener.isConnected) opener.focus(); else { var b = el && el.querySelector('[data-dl-do="share"]'); if (b) b.focus(); }
    load();
  }
  function tabBody(d) {
    var st = S.share;
    if (!st) return '<p class="dl-ok">Loading…</p>';
    if (S.tab === 'code') {
      return '<p class="dl-ok">A short text code. Send it to anyone; it also works on another Chronicle server. Pasting it shows what it uses and who made it before anything is added.</p>' +
        '<label class="dl-lab" for="dl-code">Share code</label><textarea id="dl-code" readonly rows="4" cols="1" spellcheck="false" data-dl-sel="1">' + esc(st.code) + '</textarea>' +
        '<div class="dl-row"><button type="button" class="dl-btn pri" data-dl-sh="copy-code">Copy share code</button></div>';
    }
    if (S.tab === 'link') {
      var url = shareUrl(location.origin, st.linkUrl);
      return '<p class="dl-ok">Any owner on this server whose campaign uses ' + esc(systemName(d.systemId)) + ' can open the link and copy this design into their campaign.</p>' +
        '<div class="dl-sw"><span id="dl-sw-l">Anyone with the link</span><button type="button" role="switch" class="dl-btn" aria-checked="' + !!st.linkOn + '" aria-labelledby="dl-sw-l" data-dl-sh="link">' + (st.linkOn ? 'On' : 'Off') + '</button></div>' +
        (st.linkOn ? '<label class="dl-lab" for="dl-url">Link</label><div class="dl-row"><input id="dl-url" class="dl-in" readonly value="' + esc(url) + '" data-dl-sel="1"><button type="button" class="dl-btn pri" data-dl-sh="copy-link">Copy link</button></div>' +
          '<p class="dl-ok">Turning it off kills this link. Turning it on again makes a new one.</p>' : '');
    }
    var chips = st.owners.length ? st.owners.map(function (o) {
      return '<span class="dl-chipx">' + esc(o.name || 'Owner') + '<button type="button" data-dl-sh="remove" data-uid="' + esc(o.userId) + '" aria-label="Remove ' + esc(o.name || 'owner') + '">×</button></span>';
    }).join('') : '<span class="dl-ok">Nobody is named yet.</span>';
    var hits = S.hits.map(function (o) {
      var ok = onSystem(o, d.systemId).length > 0, camps = o.campaigns.map(function (c) { return esc(c.name) + ' · ' + esc(systemName(c.systemId)); }).join(', ');
      var named = st.owners.some(function (x) { return x.userId === o.userId; });
      return '<li><div><b>' + esc(o.displayName) + '</b><span>' + camps + '</span>' + (ok ? '' : '<span class="dl-err">No campaign on ' + esc(systemName(d.systemId)) + '</span>') + '</div>' +
        '<button type="button" class="dl-btn" data-dl-sh="add" data-uid="' + esc(o.userId) + '"' + (ok && !named ? '' : ' disabled') + ' aria-label="Add ' + esc(o.displayName) + '">' + (named ? 'Added' : 'Add') + '</button></li>';
    }).join('');
    return '<div class="shr-chips" role="group" aria-label="Named owners" style="display:flex;flex-wrap:wrap;gap:6px">' + chips + '</div>' +
      '<label class="dl-lab" for="dl-q">Add an owner on this server</label><input id="dl-q" class="dl-in" data-dl-q="1" placeholder="Search by display name" autocomplete="off" value="' + esc(S.q) + '">' +
      (S.q.length >= 2 && !S.hits.length ? '<p class="dl-ok">No owner found for “' + esc(S.q) + '”.</p>' : '') + (hits ? '<ul class="dl-hits">' + hits + '</ul>' : '');
  }
  function sheetHtml(d) {
    var tabs = [['code', 'Share code'], ['link', 'Anyone with the link'], ['owners', 'Named owners']];
    return '<div class="dl-sh"><h3 id="dl-sh-t" tabindex="-1">Share “' + esc(d.name) + '”</h3><button type="button" class="dl-btn" data-dl-sh="close">Close</button></div>' +
      '<div class="dl-sb"><div class="dl-tabs" role="tablist" aria-label="How to share">' + tabs.map(function (t) {
        return '<button type="button" role="tab" id="dl-tab-' + t[0] + '" aria-selected="' + (S.tab === t[0]) + '" aria-controls="dl-pane" data-dl-sh="tab" data-tab="' + t[0] + '" tabindex="' + (S.tab === t[0] ? 0 : -1) + '">' + t[1] + '</button>';
      }).join('') + '</div><div id="dl-pane" role="tabpanel" aria-labelledby="dl-tab-' + S.tab + '" style="display:grid;gap:10px">' + tabBody(d) + '</div>' +
      '<p class="dl-err" role="alert">' + esc(S.shareErr) + '</p><p class="dl-ok" role="status" aria-live="polite">' + esc(S.shareMsg) + '</p></div>' +
      '<div class="dl-sf"><button type="button" class="dl-link dl-bad" data-dl-sh="stop">Stop sharing</button><span>Copies already taken stay with their owners.</span></div>';
  }
  // Redrawn whole, so the owner search keeps its caret by id.
  function paintSheet(focus) {
    var d = curDesign(); if (!S.sheet || !d) return;
    var a = document.activeElement, id = a && sheetEl && sheetEl.contains(a) && a.id, pos = id && a.selectionStart, tab = a && a.getAttribute && a.getAttribute('data-tab');
    if (!sheetEl) {
      sheetEl = document.createElement('div'); sheetEl.className = 'dl-sheet'; sheetEl.setAttribute('role', 'dialog'); sheetEl.setAttribute('aria-modal', 'true'); sheetEl.setAttribute('aria-labelledby', 'dl-sh-t');
      sheetEl.addEventListener('click', onSheetClick); sheetEl.addEventListener('input', onSheetInput); sheetEl.addEventListener('keydown', onSheetKey); sheetEl.addEventListener('focusin', function (e) { if (e.target.getAttribute('data-dl-sel') === '1' && e.target.select) e.target.select(); });
      host().appendChild(sheetEl);
    }
    sheetEl.innerHTML = sheetHtml(d);
    var act = a && sheetEl.contains(a) && a.getAttribute('data-dl-sh'), uid = a && a.getAttribute && a.getAttribute('data-uid');
    var again = tab ? sheetEl.querySelector('[data-tab="' + tab + '"]') : id ? sheetEl.querySelector('#' + id) :
      act ? sheetEl.querySelector('[data-dl-sh="' + act + '"]' + (uid ? '[data-uid="' + uid + '"]' : '')) : null;
    // A control that went away (a removed chip) hands focus to the heading, so the keyboard stays in the sheet.
    if (!again && (focus || (a && sheetEl.contains(a)) || document.activeElement === document.body)) again = sheetEl.querySelector('#dl-sh-t');
    if (again) { again.focus(); if (id && again.setSelectionRange && pos != null) try { again.setSelectionRange(pos, pos); } catch (x) { /* no caret */ } }
  }

  function copy(text, field) {
    function done(ok) { S.shareErr = ok ? '' : 'Copying was blocked. Select the text and copy it by hand.'; S.shareMsg = ok ? 'Copied.' : ''; paintSheet(); }
    if (navigator.clipboard && navigator.clipboard.writeText) return navigator.clipboard.writeText(text).then(function () { done(true); }, function () { done(false); });
    var f = sheetEl && sheetEl.querySelector(field), ok = false;
    try { if (f) { f.select(); ok = document.execCommand('copy'); } } catch (x) { ok = false; }
    done(ok);
  }
  function sharing(p) {
    var d = curDesign(); if (!d) return;
    S.shareErr = ''; S.shareMsg = '';
    return p(d).then(function (st) { S.share = st; paintSheet(); }).catch(function (e) { S.shareErr = e.message; paintSheet(); });
  }
  function onSheetClick(e) {
    var b = e.target.closest && e.target.closest('button'); if (!b) return;
    var act = b.getAttribute('data-dl-sh'), uid = b.getAttribute('data-uid'), id = function (d) { return '/designs/' + encodeURIComponent(d.id) + '/share'; };
    if (act === 'close') closeSheet();
    else if (act === 'tab') { S.tab = b.getAttribute('data-tab'); S.shareErr = ''; S.shareMsg = ''; paintSheet(); }
    else if (act === 'copy-code') copy(S.share.code, '#dl-code');
    else if (act === 'copy-link') copy(shareUrl(location.origin, S.share.linkUrl), '#dl-url');
    else if (act === 'link') sharing(function (d) { return call('PUT', id(d) + '/link', { on: !S.share.linkOn }); });
    else if (act === 'add') sharing(function (d) { return call('POST', id(d) + '/owners', { userId: uid }).then(function (st) { S.shareMsg = 'Added.'; return st; }); });
    else if (act === 'remove') sharing(function (d) { return call('DELETE', id(d) + '/owners/' + encodeURIComponent(uid)); });
    else if (act === 'stop') sharing(function (d) { return call('DELETE', id(d)).then(function (st) { S.shareMsg = 'Sharing stopped. Copies already taken stay with their owners.'; return st; }); });
  }
  function onSheetInput(e) {
    if (!e.target.hasAttribute('data-dl-q')) return;
    S.q = e.target.value; clearTimeout(searchT);
    var q = S.q.trim();
    if (q.length < 2) { S.hits = []; return; }
    searchT = setTimeout(function () {
      call('GET', '/designs/owners?q=' + encodeURIComponent(q)).then(function (r) { if (S.q.trim() === q) { S.hits = r.owners || []; paintSheet(); } }).catch(function (x) { S.shareErr = x.message; paintSheet(); });
    }, 250);
  }
  // Escape closes; Tab stays inside the sheet, and arrows move between tabs.
  function onSheetKey(e) {
    if (e.key === 'Escape') { e.stopPropagation(); e.preventDefault(); closeSheet(); return; }
    var t = e.target.getAttribute && e.target.getAttribute('data-tab');
    if (t && (e.key === 'ArrowRight' || e.key === 'ArrowLeft')) {
      var order = ['code', 'link', 'owners'], i = order.indexOf(t) + (e.key === 'ArrowRight' ? 1 : -1);
      S.tab = order[(i + order.length) % order.length]; paintSheet(); var n = sheetEl.querySelector('[data-tab="' + S.tab + '"]'); if (n) n.focus(); e.preventDefault(); return;
    }
    if (e.key !== 'Tab') return;
    var f = sheetEl.querySelectorAll('button:not([disabled]),input,textarea'), first = f[0], last = f[f.length - 1];
    if (e.shiftKey && document.activeElement === first) { last.focus(); e.preventDefault(); }
    else if (!e.shiftKey && document.activeElement === last) { first.focus(); e.preventDefault(); }
  }

  /* ---- Mounting ---- */
  function mount(box, host_) {
    style();
    if (H !== host_ && H && H.kind !== host_.kind) S.mine = [];
    H = host_; el = box;
    if (sheetEl) { sheetEl.remove(); sheetEl = null; }
    if (!box._dl) { box._dl = 1; box.addEventListener('click', onClick); box.addEventListener('input', onInput); box.addEventListener('keydown', onKey); }
    box.innerHTML = html(); paintMsg();
    // The panel redraws on some changes; the sheet is put back with it.
    if (S.sheet && curDesign()) paintSheet();
    load();
  }

  /* ---- The page a design link opens ---- */
  function linkPage(root) {
    var token = root.getAttribute('data-token') || '', pv = null, out = '';
    function view() {
      var h = '<h1 class="text-xl font-bold text-fg">A shared design</h1>';
      if (out) h += '<p role="status" class="text-sm">' + esc(out) + '</p>';
      else if (pv) {
        h += '<div class="dl-card" style="margin-top:12px"><div class="nm">' + esc(pv.name) + '</div>' + (pv.makerName ? '<span class="dl-sub">by ' + esc(pv.makerName) + '</span>' : '') +
          '<div><span class="dl-tag">' + esc(kindLabel(pv.kind)) + '</span><span class="dl-sub">Made for ' + esc(systemName(pv.systemId)) + '</span></div>';
        if (!pv.targets.length) h += '<div class="dl-note" role="note">None of your campaigns use ' + esc(systemName(pv.systemId)) + ', so you can’t copy this design.</div>';
        else {
          h += '<label class="dl-lab" for="dl-target">Copy into which campaign?</label><select id="dl-target" class="dl-in">' + pv.targets.map(function (t) { return '<option value="' + esc(t.id) + '">' + esc(t.name) + '</option>'; }).join('') + '</select>' +
            '<div id="dl-lacks"></div><div class="dl-row"><button type="button" class="dl-btn pri" id="dl-copy">Add to my designs</button></div>';
        }
        h += '<p class="dl-err" role="alert" id="dl-lerr"></p></div>';
      } else h += '<p class="text-sm text-fg-secondary" role="status">Loading…</p>';
      root.innerHTML = h;
      var sel = root.querySelector('#dl-target');
      if (sel) { sel.addEventListener('change', lacks); lacks(); root.querySelector('#dl-copy').addEventListener('click', copyIn); }
    }
    function lacks() {
      var sel = root.querySelector('#dl-target'), box = root.querySelector('#dl-lacks'), t = pv.targets.filter(function (x) { return x.id === sel.value; })[0];
      box.innerHTML = ((t && t.lacks) || []).map(function (l) { return '<div class="dl-note" role="note">' + esc(l) + '</div>'; }).join('');
    }
    function copyIn() {
      var sel = root.querySelector('#dl-target'), err = root.querySelector('#dl-lerr');
      window.Chronicle.apiAction('/designs/link/' + encodeURIComponent(token) + '/take', { method: 'POST', body: { campaignId: sel.value } }).then(function (d) {
        out = '“' + d.name + '” was added to your designs. Open the shop room’s Arrange panel to use it.'; view();
      }).catch(function (e) { err.textContent = e.message; });
    }
    style(); view();
    window.Chronicle.apiAction('/designs/link/' + encodeURIComponent(token), { headers: { Accept: 'application/json' } }).then(function (p) { pv = p; view(); }).catch(function (e) { out = e.message; view(); });
  }
  if (window.Chronicle && window.Chronicle.register) {
    window.Chronicle.register('design_library', { init: function (root) { linkPage(root); }, destroy: function (root) { root.innerHTML = ''; } });
  }

  window.DesignLibrary = {
    mount: mount,
    util: { PREFIX: PREFIX, parseCode: parseCode, systemName: systemName, kindLabel: kindLabel, previewNotes: previewNotes, shareUrl: shareUrl, onSystem: onSystem }
  };
})();
