/**
 * shop_room_builder.js — the counter room builder, for the shop's owner in the
 * room's Arrange panel (a "Counter room" section shown while the look is
 * Counter). The room it edits is the theme the counter look draws from
 * (shop_room_counter.js: skeleton, theme, props); the widget saves that theme
 * in the room layout's `counter` field with the rest of Arrange.
 *
 * Two ways in, one theme: pick from lists (starting-from chips, selects,
 * swatches, a warmth slider, prop checkboxes), or place it by hand (dashed
 * slots on the room; drag a prop chip to its slot, or press the chip and then
 * the slot; press the window to move it). The widget hands over an api:
 *   theme()        the theme in force, normalised (the saved one, else the starter)
 *   custom()       whether the owner has made one
 *   set(t, soft)   draw and keep t; soft waits a moment, for sliders and pickers
 *   reset()        drop the saved theme, back to the shop type's starter
 *   place(v)       read or set place-by-hand mode
 *   campaign()     { url, csrf } for the saved-designs library (design_library.js)
 * The owner's own picture (one of the campaign's pictures, on the back wall or
 * through the window) is chosen from the campaign's pictures or uploaded here.
 * Every change redraws the room in place; players never load this file's UI.
 */
(function () {
  'use strict';

  var C = null, cur = null, mode = 'lists', armed = '', placedMap = {}, noteT = 0;
  var LBL = {
    shape: { 'open-air': 'Open-air stall', hold: 'Ship’s hold', hollow: 'Hollow tree', 'void': 'Open void', ribs: 'Great ribs', works: 'Gearworks' },
    opening: { none: 'No window', 'window-left': 'Window, left', 'window-right': 'Window, right', 'window-centre': 'Window, centre', 'round-window': 'Round window', doorway: 'Doorway' },
    counter: { 'stall-table': 'Stall table', log: 'Fallen log', slab: 'Stone slab' },
    wall: { white: 'White nowhere', lair: 'Dragon’s lair' },
    outside: { live: 'The campaign’s sky, live' },
    scene: { 'sky-islands': 'Sky islands', underdark: 'Underdark caverns', astral: 'Astral sea' },
    prop: { 'floating-books': 'Floating books', 'steam-cart': 'Steam cart' }
  };
  // The server takes at most this many props in one theme.
  var MAX_PROPS = 24, FULL = 'A room holds up to 24 props. Take one out first.';
  var SKEL = [['shape', 'Back wall'], ['opening', 'Opening'], ['display', 'Display'], ['counter', 'Counter']];
  var LOOK = [['wall', 'Wall'], ['light', 'Light'], ['outside', 'Outside'], ['scene', 'View outside']];
  var COLS = [['wood', 'Wood'], ['cloth', 'Cloth'], ['accent', 'Accent'], ['trim', 'Trim']];
  var SW = { wood: ['#6b4527', '#4a3220', '#7a5230', '#3a2a20', '#8a6a3a', '#4a3a5a'], cloth: ['#5a2024', '#3a5a3a', '#4a2a6a', '#2a4a6a', '#8a2a2e', '#5a3a22'],
    accent: ['#e0a83a', '#ff9a50', '#3fae6a', '#8ab4ff', '#d23a4a', '#c4b0ff'], trim: ['#c89a4a', '#9aa0a8', '#d9b25a', '#b87a4a', '#e8e0d0', '#7a8a9a'] };

  function esc(t) { t = String(t == null ? '' : t); return window.Chronicle && window.Chronicle.escapeHtml ? window.Chronicle.escapeHtml(t) : t.replace(/[^\w #.,:'’-]/g, ''); }
  function nice(v) { v = String(v).replace(/-/g, ' '); return v.charAt(0).toUpperCase() + v.slice(1); }
  function lbl(group, v) { return (LBL[group] && LBL[group][v]) || nice(v); }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }
  function pathGet(o, p) { return p.split('.').reduce(function (a, k) { return a && a[k]; }, o); }
  function pathSet(o, p, v) { var a = p.split('.'), l = a.pop(); a.reduce(function (x, k) { return x[k]; }, o)[l] = v; }
  function slotOf(p) { return C.options.propAt[p].at; }
  function slotName(slot) { return C.options.slots[slot][4]; }

  var CSS = [
    '.shr-cb{display:grid;gap:10px}.shr-cb[hidden]{display:none}',
    '.shr-cb details{border-top:1px solid var(--color-border,#e5e7eb);padding-top:6px}',
    '.shr-cb summary{cursor:pointer;font-size:.72rem;text-transform:uppercase;letter-spacing:.06em;color:var(--color-text-muted,#6b7280);padding:4px 0}',
    '.shr-cb summary:focus-visible,.shr-cb button:focus-visible,.shr-cb select:focus-visible,.shr-cb input:focus-visible{outline:2px solid var(--shr-acc);outline-offset:2px}',
    '.shr-cb details>.shr-field,.shr-cb details>.shr-rng,.shr-cb details>.shr-cbk{margin:8px 0}',
    '.shr-cb .shr-chip{min-height:32px}.shr-cb .shr-chip[draggable=true]{cursor:grab}',
    '.shr-cb .shr-chip.arm{outline:2px dashed var(--shr-acc);outline-offset:2px}.shr-cb .shr-chip.on{background:color-mix(in srgb,var(--shr-acc) 14%,transparent)}',
    '.shr-cb .shr-chip button{border:0;background:none;color:inherit;font:inherit;cursor:pointer;padding:0 2px;line-height:1;min-height:24px}',
    '.shr-cbsw{display:flex;flex-wrap:wrap;gap:6px;align-items:center}',
    '.shr-cbsw button{width:28px;height:28px;border-radius:50%;border:2px solid var(--color-border,#e5e7eb);padding:0;cursor:pointer}',
    '.shr-cbsw button[aria-pressed=true]{border-color:var(--color-text-primary,#111827);box-shadow:0 0 0 2px var(--shr-acc)}',
    '.shr-cbsw input[type=color]{width:34px;height:30px;padding:0;border:1px solid var(--color-input-border,#d1d5db);border-radius:6px;background:var(--color-input-bg,#fff);cursor:pointer}',
    '.shr-cbk{display:grid;grid-template-columns:1fr 1fr;gap:2px 8px}.shr-cbk label{display:flex;gap:6px;align-items:center;min-height:32px;font-size:.8rem;color:var(--color-text-body,#374151)}',
    '.shr-cbk small{color:var(--color-text-secondary,#6b7280)}',
    '.shr-cb .shr-seg button{min-height:32px}',
    '.shr-cbpic{width:100%;aspect-ratio:12/7;object-fit:cover;border-radius:6px;border:1px solid var(--color-border,#e5e7eb);display:block;background:var(--color-bg-secondary,#f3f4f6)}',
    '.shr-cbgrid{display:grid;grid-template-columns:repeat(3,1fr);gap:6px;margin-top:6px}.shr-cbgrid[hidden]{display:none}',
    '.shr-cbgrid button{padding:0;border:1px solid var(--color-border,#e5e7eb);border-radius:6px;overflow:hidden;aspect-ratio:3/2;cursor:pointer;background:var(--color-bg-secondary,#f3f4f6);min-height:44px}',
    '.shr-cbgrid button[aria-pressed=true]{outline:2px solid var(--shr-acc);outline-offset:1px}.shr-cbgrid img{width:100%;height:100%;object-fit:cover;display:block}',
    '.shr-cbpicf{display:grid;gap:8px}.shr-cbpicf .shr-chips{display:flex;flex-wrap:wrap;gap:4px 12px}',
    '.shr-cbup{position:relative;cursor:pointer}.shr-cbup input{position:absolute;inset:0;opacity:0;cursor:pointer;width:100%}',
    // The panel can sit on the left, so the right-hand slots are not covered, and on a phone it drops to the foot while placing.
    '.shr-panel.cbl{right:auto;left:0;border-left:0;border-right:1px solid var(--color-border,#e5e7eb);box-shadow:18px 0 40px -24px rgb(0 0 0/.45);transform:translateX(-105%)}.shr-panel.cbl.on{transform:none}',
    '@media (max-width:640px){.shr-panel.cbp{top:auto;height:56%;border-left:0;border-top:1px solid var(--color-border,#e5e7eb)}.shr-panel.cbp.cbl{left:0;right:0}.shr-panel.cbp .cbside{display:none}}'
  ].join('\n');
  function style() {
    if (document.getElementById('shr-cb-style')) return;
    var st = document.createElement('style'); st.id = 'shr-cb-style'; st.textContent = CSS; document.head.appendChild(st);
  }

  /* ---- The controls, drawn as HTML and wired by delegation on the box ---- */
  function selectRow(path, label, vals, group) {
    var id = 'shr-cb-' + path.replace('.', '-');
    return '<label class="shr-field" for="' + id + '"><span>' + label + '</span><select id="' + id + '" data-cb-path="' + path + '">' +
      vals.map(function (v) { return '<option value="' + esc(v) + '">' + esc(lbl(group, v)) + '</option>'; }).join('') + '</select></label>';
  }
  function colourRow(name, label, swatches) {
    var sw = swatches ? SW[name].map(function (c) { return '<button type="button" style="background:' + c + '" data-cb-sw="' + name + ':' + c + '" aria-label="' + label + ' ' + c + '" aria-pressed="false"></button>'; }).join('') : '';
    return '<div class="shr-field" role="group" aria-label="' + label + ' colour"><span>' + label + '</span><span class="shr-cbsw">' + sw +
      '<input type="color" data-cb-path="theme.' + name + '" aria-label="Pick a ' + label.toLowerCase() + ' colour"></span></div>';
  }
  function warmthRow() {
    return '<label class="shr-rng"><span>Warmth</span><input type="range" min="0" max="1" step="0.05" data-cb-path="theme.warmth" aria-label="Warmth of the light"><output data-cb-warm></output></label>';
  }
  function group(title, body, open) { return '<details' + (open === false ? '' : ' open') + '><summary>' + title + '</summary>' + body + '</details>'; }
  function startChips() {
    return '<div class="shr-field"><span>Starting from</span><small class="dl-g" style="font-size:.7rem;text-transform:uppercase;letter-spacing:.05em;color:var(--color-text-muted,#6b7280)">Built in</small><span class="shr-chips" role="group" aria-label="Built in">' +
      C.starters.map(function (s, i) { return '<button type="button" class="shr-chip" data-cb-start="' + i + '" aria-pressed="false">' + esc(s.name) + '</button>'; }).join('') + '</span>' +
      '<button type="button" class="shr-link" data-cb-reset="1" hidden>Back to this shop type’s starter</button>' +
      '<div data-dl></div></div>';
  }
  function html() {
    var O = C.options, place = mode === 'place', h = '';
    h += '<span class="shr-seg" role="group" aria-label="How to build">' +
      '<button type="button" data-cb-mode="lists" aria-pressed="' + !place + '">Pick from lists</button><button type="button" data-cb-mode="place" aria-pressed="' + place + '">Place it yourself</button></span>';
    if (place) {
      h += '<p class="shr-note">Dashed slots are on the room. Drag a prop to its slot, or press a prop and then its slot. Press the window to move it: left, centre, right or none. Drag the shelves or a placed prop to move it, or press it and use the arrow keys.</p>' +
        '<div class="shr-cbmv" data-cb-mv></div>' +
        '<button type="button" class="shr-link cbside" data-cb-side="1">Move this panel to the other side</button>';
    }
    h += startChips();
    h += group('Skeleton', SKEL.filter(function (r) { return !(place && r[0] === 'opening'); }).map(function (r) { return selectRow('skeleton.' + r[0], r[1], O.skeleton[r[0]], r[0]); }).join(''));
    h += group(place ? 'Look' : 'Theme', LOOK.map(function (r) { return selectRow('theme.' + r[0], r[1], O.theme[r[0]], r[0]); }).join('') + warmthRow() +
      COLS.map(function (r) { return colourRow(r[0], r[1], !place); }).join(''));
    if (place) {
      h += group('Props', '<span class="shr-chips" role="group" aria-label="Props">' + O.props.map(function (p) {
        return '<span class="shr-chip" data-cb-tray="' + p + '" draggable="true"><button type="button" data-cb-arm="' + p + '" aria-pressed="false" aria-label="' + esc(lbl('prop', p)) + ': press, then press its dashed slot">' + esc(lbl('prop', p)) + '</button>' +
          '<button type="button" data-cb-drop="' + p + '" aria-label="Remove ' + esc(lbl('prop', p)) + '" hidden>×</button></span>';
      }).join('') + '</span>');
    } else {
      h += group('Props', '<div class="shr-cbk">' + O.props.map(function (p) {
        return '<label><input type="checkbox" data-cb-prop="' + p + '"><span>' + esc(lbl('prop', p)) + '</span><small data-cb-note="' + p + '"></small></label>';
      }).join('') + '</div>');
    }
    h += group('Your own picture', '<div class="shr-cbpicf" data-cb-pic></div>', false);
    return h + '<p class="shr-note" role="status" aria-live="polite" data-cb-say></p>';
  }

  /* ---- The owner's own picture: one of the campaign's pictures, on the back
     wall or as the view through the opening. The room keeps only the
     picture's id; the server checks it is this campaign's. ---- */
  var PIC_ON = [['wall', 'On the back wall'], ['outside', 'Through the window']];
  var PIC_FIT = [['fill', 'Fill'], ['fit', 'Whole picture'], ['repeat', 'Repeat']];
  var PIC_PAGE = 24, picKey = null, pics = { items: [], page: 0, total: 0, open: false, busy: false };
  function campaignId() { var c = cur.api.campaign && cur.api.campaign(); var m = c && /^\/campaigns\/([^/?#]+)/.exec(c.url || ''); return m ? decodeURIComponent(m[1]) : ''; }
  function seg(label, attr, list, val) {
    return '<span class="shr-seg" role="group" aria-label="' + label + '">' + list.map(function (o) { return '<button type="button" ' + attr + '="' + o[0] + '" aria-pressed="' + (o[0] === val) + '">' + o[1] + '</button>'; }).join('') + '</span>';
  }
  function picGrid() {
    if (!pics.open) return '<div class="shr-cbgrid" data-cb-grid hidden></div>';
    var p = cur.api.theme().theme.picture, sel = p ? p.id : '';
    var cells = pics.items.map(function (m) {
      return '<button type="button" data-cb-use="' + esc(m.id) + '" aria-pressed="' + (m.id === sel) + '" aria-label="Use ' + esc(m.original_name || 'this picture') + '"><img src="' + esc(m.thumbnail_url || m.url) + '" alt="" loading="lazy"></button>';
    }).join('');
    var more = pics.page * PIC_PAGE < pics.total ? '<button type="button" class="shr-link" data-cb-more="1">More pictures</button>' : '';
    var none = !pics.busy && !pics.items.length ? '<p class="shr-note">This campaign has no pictures yet. Upload one.</p>' : '';
    return '<div class="shr-cbgrid" data-cb-grid>' + cells + '</div>' + none + more;
  }
  function syncPic(force) {
    var box = cur && cur.box.querySelector('[data-cb-pic]'); if (!box) return;
    var p = cur.api.theme().theme.picture || null, key = JSON.stringify(p) + '|' + pics.open + '|' + pics.items.length + '|' + pics.busy;
    if (!force && key === picKey) return;
    picKey = key;
    var up = '<label class="shr-link shr-cbup">Upload a picture<input type="file" accept="image/png,image/jpeg,image/webp,image/gif" data-cb-up aria-label="Upload a picture for the room"></label>';
    var h = '';
    if (p) {
      h += '<img class="shr-cbpic" src="/media/' + esc(p.id) + '/thumb/800" alt="The room’s picture">' +
        seg('Where the picture goes', 'data-cb-picon', PIC_ON, p.on) + seg('How it fills that space', 'data-cb-picfit', PIC_FIT, p.fit) +
        '<span class="shr-chips"><button type="button" class="shr-link" data-cb-pics="1" aria-expanded="' + pics.open + '">' + (pics.open ? 'Hide the pictures' : 'Choose another') + '</button>' + up +
        '<button type="button" class="shr-link" data-cb-picoff="1">Take the picture away</button></span>';
    } else {
      h += '<p class="shr-note">Use one of this campaign’s pictures as the back wall, or as the view through the window.</p>' +
        '<span class="shr-chips"><button type="button" class="shr-link" data-cb-pics="1" aria-expanded="' + pics.open + '">' + (pics.open ? 'Hide the pictures' : 'Choose from this campaign’s pictures') + '</button>' + up + '</span>';
    }
    box.innerHTML = h + picGrid();
  }
  function loadPics(next) {
    var c = cur.api.campaign && cur.api.campaign(); if (!c || !window.Chronicle || pics.busy) return;
    pics.busy = true; syncPic(true);
    var page = next ? pics.page + 1 : 1;
    window.Chronicle.apiFetch(c.url + '/media/list?perPage=' + PIC_PAGE + '&page=' + page).then(function (r) {
      if (!r.ok) throw new Error(String(r.status));
      return r.json();
    }).then(function (d) {
      var got = (d && (d.items || d.media || d.data)) || (Array.isArray(d) ? d : []);
      got = got.filter(function (m) { return m && /^image\//.test(m.mime_type || '') && /^[0-9a-f-]{36}$/i.test(m.id || ''); });
      pics.items = next ? pics.items.concat(got) : got; pics.page = page; pics.total = +(d && d.total) || pics.items.length;
    }).catch(function () { say('The campaign’s pictures could not be loaded. Try again.'); pics.open = false; })
      .then(function () { pics.busy = false; syncPic(true); });
  }
  function usePic(id) {
    change(function (t) { var p = t.theme.picture; t.theme.picture = { id: id, on: p ? p.on : 'wall', fit: p ? p.fit : 'fill' }; });
    say('The room uses that picture now. Save room to keep it.');
  }
  function upload(file) {
    var id = campaignId(); if (!file || !id || !window.Chronicle) return;
    var fd = new FormData(); fd.append('file', file); fd.append('usage_type', 'attachment'); fd.append('campaign_id', id);
    say('Uploading the picture…');
    window.Chronicle.apiFetch('/media/upload', { method: 'POST', body: fd }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (b) { if (!r.ok) throw new Error(b && b.message || 'The picture did not upload.'); return b; });
    }).then(function (m) {
      if (!m || !/^[0-9a-f-]{36}$/i.test(m.id || '')) throw new Error('The picture did not upload. Try again.');
      if (pics.open) pics.items.unshift({ id: m.id, original_name: file.name, mime_type: m.mime_type || 'image/jpeg', thumbnail_url: m.thumbnail_url, url: m.url });
      usePic(m.id);
    }).catch(function (e) { say(e && e.message ? e.message : 'The picture did not upload. Try again.'); });
  }
  function say(t) { var e = cur && cur.box.querySelector('[data-cb-say]'); if (e) e.textContent = t || ''; }

  // Keeps every control in step with the theme, without rebuilding them (a colour picker stays open while it is dragged).
  function sync() {
    var b = cur.box, t = cur.api.theme(), j = JSON.stringify(t);
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-path]'), function (c) {
      var v = pathGet(t, c.getAttribute('data-cb-path'));
      // A word the theme leaves out (no scene is the town) shows as the list's first.
      if (v == null && c.tagName === 'SELECT') v = c.options[0] ? c.options[0].value : '';
      if (String(c.value) !== String(v)) c.value = v;
    });
    var w = b.querySelector('[data-cb-warm]'); if (w) w.textContent = Math.round(t.theme.warmth * 100) + '%';
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-sw]'), function (e) { var p = e.getAttribute('data-cb-sw').split(':'); e.setAttribute('aria-pressed', String(t.theme[p[0]] === p[1])); });
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-start]'), function (e) { e.setAttribute('aria-pressed', String(JSON.stringify(C.normalise(C.starters[+e.getAttribute('data-cb-start')])) === j)); });
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-prop]'), function (e) { e.checked = t.props.indexOf(e.getAttribute('data-cb-prop')) >= 0; });
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-tray]'), function (e) {
      var p = e.getAttribute('data-cb-tray'), on = t.props.indexOf(p) >= 0;
      e.classList.toggle('on', on); e.classList.toggle('arm', armed === p); e.querySelector('[data-cb-arm]').setAttribute('aria-pressed', String(armed === p)); e.querySelector('[data-cb-drop]').hidden = !on;
    });
    var r = b.querySelector('[data-cb-reset]'); if (r) r.hidden = !cur.api.custom();
    var sm = b.closest('.shr-sec'), sh = sm && sm.querySelector('.shr-sh small'); if (sh) sh.textContent = t.name;
    syncPic(); syncMv(); notes(); hilite();
  }
  // A prop that found no free spot in this skeleton says so, and is never squeezed in.
  function notes() {
    if (!cur) return;
    var t = cur.api.theme();
    Array.prototype.forEach.call(cur.box.querySelectorAll('[data-cb-note]'), function (e) { var p = e.getAttribute('data-cb-note'); e.textContent = t.props.indexOf(p) >= 0 && placedMap[p] === false ? '(no room here)' : ''; });
    Array.prototype.forEach.call(cur.box.querySelectorAll('[data-cb-tray]'), function (e) { var p = e.getAttribute('data-cb-tray'); e.title = t.props.indexOf(p) >= 0 && placedMap[p] === false ? 'In the theme, but this skeleton has no free spot for it' : ''; });
  }
  function slots() { return cur ? cur.box.closest('.shr').querySelectorAll('.cm-slot[data-slot]') : []; }
  function hilite() {
    if (!cur) return;
    Array.prototype.forEach.call(slots(), function (r) { r.classList.toggle('cm-on', !!armed && r.getAttribute('data-slot') === slotOf(armed)); });
  }

  /* ---- Changing the theme ---- */
  function change(fn, soft) { var t = clone(cur.api.theme()); fn(t); cur.api.set(C.normalise(t), soft); sync(); }
  function place(slot, p) {
    if (!cur || !p || !C.options.propAt[p]) return;
    if (slotOf(p) !== slot) { say('The ' + lbl('prop', p).toLowerCase() + ' belongs at: ' + slotName(slotOf(p)).toLowerCase() + '.'); return; }
    if (cur.api.theme().props.indexOf(p) >= 0) { say('The ' + lbl('prop', p).toLowerCase() + ' is already in the room.'); return; }
    if (cur.api.theme().props.length >= MAX_PROPS) { say(FULL); return; }
    armed = '';
    change(function (t) { t.props.push(p); });
    say('Placed the ' + lbl('prop', p).toLowerCase() + '.');
  }
  function onClick(e) {
    var b = e.target.closest && e.target.closest('button'); if (!b) return;
    var d = b.dataset;
    if (d.cbMode) { mode = d.cbMode; armed = ''; cur.api.place(mode === 'place'); mount(cur.box, cur.api); return; }
    if (d.cbStart !== undefined) { cur.api.set(C.normalise(clone(C.starters[+d.cbStart]))); sync(); say('Started from ' + C.starters[+d.cbStart].name + '.'); return; }
    if (d.cbReset) { cur.api.reset(); sync(); say('Back to the starter for this shop type.'); return; }
    if (d.cbSw) { var p = d.cbSw.split(':'); change(function (t) { t.theme[p[0]] = p[1]; }); return; }
    if (d.cbArm) { armed = armed === d.cbArm ? '' : d.cbArm; sync(); say(armed ? 'Now press the dashed slot “' + slotName(slotOf(armed)) + '”, or drag the chip there.' : ''); return; }
    if (d.cbDrop) { change(function (t) { var i = t.props.indexOf(d.cbDrop); if (i >= 0) t.props.splice(i, 1); }); return; }
    if (d.cbSide) { var pn = b.closest('.shr-panel'); if (pn) pn.classList.toggle('cbl'); return; }
    if (d.cbPics) { pics.open = !pics.open; if (pics.open && !pics.page) loadPics(false); else syncPic(true); return; }
    if (d.cbMore) { loadPics(true); return; }
    if (d.cbUse) { usePic(d.cbUse); return; }
    if (d.cbPicon || d.cbPicfit) { change(function (t) { if (!t.theme.picture) return; if (d.cbPicon) t.theme.picture.on = d.cbPicon; else t.theme.picture.fit = d.cbPicfit; }); return; }
    if (d.cbUnmove !== undefined && sel) { var k = sel; setMove(k, [0, 0, 1]); say(mvName(k) + ' is back where it was.'); return; }
    if (d.cbPicoff) { change(function (t) { delete t.theme.picture; }); say('The picture is gone from the room. Save room to keep that.'); }
  }
  function onInput(e) {
    if (e.target && e.target.hasAttribute && e.target.hasAttribute('data-cb-size')) { if (sel) { var v = mvOf(sel); v[2] = +e.target.value; setMove(sel, v, true); } return; }
    if (e.target && e.target.hasAttribute && e.target.hasAttribute('data-cb-up')) { if (e.type === 'change' && e.target.files && e.target.files[0]) upload(e.target.files[0]); return; }
    var c = e.target, path = c.getAttribute && c.getAttribute('data-cb-path'), pr = c.getAttribute && c.getAttribute('data-cb-prop');
    if (path) { var v = c.type === 'range' ? +c.value : c.value; change(function (t) { pathSet(t, path, v); }, c.type === 'range' || c.type === 'color'); }
    else if (pr && e.type === 'change') {
      if (c.checked && cur.api.theme().props.length >= MAX_PROPS) { c.checked = false; say(FULL); return; }
      change(function (t) { var i = t.props.indexOf(pr); if (c.checked && i < 0) t.props.push(pr); else if (!c.checked && i >= 0) t.props.splice(i, 1); });
    }
  }

  // Opening: the window in place mode moves left, centre, right, then away.
  function opening() {
    if (!cur) return;
    var order = ['window-left', 'window-centre', 'window-right', 'none'], i = order.indexOf(cur.api.theme().skeleton.opening);
    change(function (t) { t.skeleton.opening = order[(i + 1) % order.length]; });
    say('The opening is now: ' + lbl('opening', cur.api.theme().skeleton.opening).toLowerCase() + '.');
  }

  function mount(box, api) {
    C = window.ShopRoomCounter; style();
    if (cur && cur.box !== box) armed = '';
    cur = { box: box, api: api };
    mode = api.place() ? 'place' : 'lists';
    box.className = 'shr-cb'; box.innerHTML = html(); picKey = null;
    if (!box._cb) { box._cb = 1; box.addEventListener('click', onClick); box.addEventListener('input', onInput); box.addEventListener('change', onInput); box.addEventListener('dragstart', onDrag); box.addEventListener('dragend', function () { armed = ''; sync(); }); }
    var pn = box.closest('.shr-panel'); if (pn) pn.classList.toggle('cbp', mode === 'place');
    sync();
    // The owner's saved designs sit with the starting points; the library file loads on sight and is optional.
    var dl = box.querySelector('[data-dl]');
    if (dl && window.DesignLibrary && api.campaign) {
      window.DesignLibrary.mount(dl, {
        kind: 'room-theme', campaign: api.campaign, say: say, theme: function () { return clone(cur.api.theme()); },
        apply: function (data) { cur.api.set(C.normalise(clone(data))); sync(); }
      });
    }
  }
  function onDrag(e) {
    var c = e.target.closest && e.target.closest('[data-cb-tray]'); if (!c) return;
    armed = c.getAttribute('data-cb-tray');
    try { e.dataTransfer.setData('text/plain', armed); e.dataTransfer.effectAllowed = 'copy'; } catch (x) { /* a synthetic drag has no data */ }
    sync();
  }

  /* Slots live in the room's drawing, outside the panel, so these listen on the
     document and act only on the open builder's own room. */
  function slotAt(e) { var s = e.target.closest && e.target.closest('[data-slot]'); return s && cur && cur.box.isConnected && cur.box.closest('.shr').contains(s) ? s : null; }
  document.addEventListener('dragover', function (e) { var s = slotAt(e); if (s && armed && s.getAttribute('data-slot') === slotOf(armed)) e.preventDefault(); });
  document.addEventListener('drop', function (e) { var s = slotAt(e); if (!s) return; e.preventDefault(); place(s.getAttribute('data-slot'), armed); });
  document.addEventListener('click', function (e) { var s = slotAt(e); if (!s) return; if (armed) place(s.getAttribute('data-slot'), armed); else say('Press a prop first, then its slot.'); });
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    var s = slotAt(e); if (!s) return; e.preventDefault();
    if (armed) place(s.getAttribute('data-slot'), armed); else say('Press a prop first, then its slot.');
  });

  /* ---- Moving the shelves and props: the room says which piece was picked
     or where it was dragged; the panel sizes it or puts it back. ---- */
  var sel = '';
  function mvName(k) { return k === 'display' ? 'The shelves' : lbl('prop', k); }
  function mvOf(k) { var m = cur.api.theme().moves || {}; return m[k] ? m[k].slice() : [0, 0, 1]; }
  function setMove(k, v, soft) {
    change(function (t) { t.moves = Object.assign({}, t.moves); t.moves[k] = v; if (v[0] === 0 && v[1] === 0 && v[2] === 1) delete t.moves[k]; }, soft);
  }
  function syncMv() {
    var el = cur && cur.box.querySelector('[data-cb-mv]'); if (!el) return;
    if (sel && sel !== 'display' && cur.api.theme().props.indexOf(sel) < 0) sel = '';
    if (!sel) { el.innerHTML = ''; el._k = ''; return; }
    var v = mvOf(sel);
    // Drawn once per piece, then only its values change, so a slider being dragged is never replaced.
    if (el._k === sel) {
      var r = el.querySelector('[data-cb-size]'), o = el.querySelector('output'), b = el.querySelector('[data-cb-unmove]');
      if (r && document.activeElement !== r) r.value = v[2]; if (o) o.textContent = Math.round(v[2] * 100) + '%'; if (b) b.hidden = !(v[0] || v[1] || v[2] !== 1);
      return;
    }
    el._k = sel;
    el.innerHTML = '<div class="shr-field" role="group" aria-label="' + esc(mvName(sel)) + '"><span>' + esc(mvName(sel)) + '</span>' +
      '<label class="shr-rng"><span>Size</span><input type="range" min="0.5" max="2" step="0.05" value="' + v[2] + '" data-cb-size aria-label="Size of ' + esc(mvName(sel).toLowerCase()) + '"><output>' + Math.round(v[2] * 100) + '%</output></label>' +
      '<button type="button" class="shr-link" data-cb-unmove' + (v[0] || v[1] || v[2] !== 1 ? '' : ' hidden') + '>Put it back where it was</button></div>';
  }
  function picked(k) { if (!cur || mode !== 'place') return; sel = k; syncMv(); }
  function moved(k, v) { if (!cur) return; sel = k; setMove(k, v); say(mvName(k) + ' moved. Save room to keep it.'); }

  window.ShopRoomBuilder = {
    mount: mount, opening: opening, picked: picked, moved: moved,
    // The counter room has just been drawn: remember which props found a spot, and relight the slots once it is in the page.
    placed: function (m) { placedMap = m || {}; clearTimeout(noteT); noteT = setTimeout(function () { notes(); hilite(); }, 0); }
  };
})();
