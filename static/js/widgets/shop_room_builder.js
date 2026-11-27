/**
 * shop_room_builder.js — the counter room builder, for the shop's owner in the
 * room's Arrange panel (a "Counter room" section shown while the look is
 * Counter). The room it edits is the theme the counter look draws from
 * (shop_room_counter.js: skeleton, theme, props); the widget saves that theme
 * in the room layout's `counter` field with the rest of Arrange.
 *
 * Two ways in, one theme: press a part (the room's parts as tabs, each choice
 * a small picture of this room dressed that way; a press on a part of the
 * room, or on one of its layers pulled apart, opens that part's tab; the
 * lists sit below: starting-from chips, selects, swatches, a warmth slider,
 * prop checkboxes), or place it by hand (dashed slots on the room; drag a
 * prop chip to its slot, or press the chip and then the slot; press the
 * window to move it; drag or nudge the shelves and props). The widget hands
 * over an api:
 *   theme()        the theme in force, normalised (the saved one, else the starter)
 *   custom()       whether the owner has made one
 *   set(t, soft)   draw and keep t; soft waits a moment, for sliders and pickers
 *   reset()        drop the saved theme, back to the shop type's starter
 *   place(v)       read or set place-by-hand mode
 *   campaign()     { url, csrf } for the saved-designs library (design_library.js)
 *   thumb(t, crop, cb, alive)  a small picture of the room dressed as t, cropped to [x, y, w, h]
 *   layers(on)     pull the room apart into its layers, or put it back together
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
    '.shr-cbparts{display:grid;gap:8px}.shr-cbtabs{display:flex;flex-wrap:wrap;gap:2px;border-bottom:1px solid var(--color-border,#e5e7eb)}',
    '.shr-cbtabs button{border:0;border-bottom:2px solid transparent;background:none;color:var(--color-text-secondary,#6b7280);font:600 .68rem/1 inherit;text-transform:uppercase;letter-spacing:.04em;padding:9px 5px;min-height:44px;cursor:pointer}',
    '.shr-cbtabs button[aria-selected=true]{color:var(--color-text-primary,#111827);border-bottom-color:var(--shr-acc)}',
    '.shr-cbtabs button:focus-visible,.shr-cbtile:focus-visible{outline:2px solid var(--shr-acc);outline-offset:2px}',
    '.shr-cbtiles{display:grid;grid-template-columns:repeat(3,1fr);gap:6px}',
    '.shr-cbtile{display:grid;gap:3px;padding:3px;border:1px solid var(--color-border,#e5e7eb);border-radius:7px;background:var(--color-bg-secondary,#f9fafb);cursor:pointer;color:var(--color-text-body,#374151);font:500 .72rem/1.2 inherit;text-align:center;min-height:44px}',
    '.shr-cbtile img{width:100%;aspect-ratio:12/7;object-fit:cover;border-radius:5px;background:linear-gradient(135deg,#2a2230,#16121c);display:block}',
    '.shr-cbtile[aria-pressed=true]{border-color:var(--shr-acc);box-shadow:0 0 0 2px var(--shr-acc)}',
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
      '<button type="button" data-cb-mode="lists" aria-pressed="' + !place + '">Press a part</button><button type="button" data-cb-mode="place" aria-pressed="' + place + '">Place it yourself</button></span>';
    if (place) {
      h += '<p class="shr-note">Dashed slots are on the room. Drag a prop to its slot, or press a prop and then its slot. Press the window to move it: left, centre, right or none. Drag the shelves or a placed prop to move it, or press it and use the arrow keys.</p>' +
        '<div class="shr-cbmv" data-cb-mv></div>' +
        '<button type="button" class="shr-link cbside" data-cb-side="1">Move this panel to the other side</button>';
    } else h += partsHtml();
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
    box.innerHTML = h + '<div data-cb-palbox></div>' + picGrid();
    pal.id = ''; syncPal();
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
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-warm]'), function (w) { w.textContent = Math.round(t.theme.warmth * 100) + '%'; });
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-tile]'), function (e) { e.setAttribute('aria-pressed', String(tileOn(t, e.getAttribute('data-cb-tile'), e.getAttribute('data-cb-v')))); });
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-sw]'), function (e) { var p = e.getAttribute('data-cb-sw').split(':'); e.setAttribute('aria-pressed', String(t.theme[p[0]] === p[1])); });
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-start]'), function (e) { e.setAttribute('aria-pressed', String(JSON.stringify(C.normalise(C.starters[+e.getAttribute('data-cb-start')])) === j)); });
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-prop]'), function (e) { e.checked = t.props.indexOf(e.getAttribute('data-cb-prop')) >= 0; });
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-tray]'), function (e) {
      var p = e.getAttribute('data-cb-tray'), on = t.props.indexOf(p) >= 0;
      e.classList.toggle('on', on); e.classList.toggle('arm', armed === p); e.querySelector('[data-cb-arm]').setAttribute('aria-pressed', String(armed === p)); e.querySelector('[data-cb-drop]').hidden = !on;
    });
    var r = b.querySelector('[data-cb-reset]'); if (r) r.hidden = !cur.api.custom();
    var sm = b.closest('.shr-sec'), sh = sm && sm.querySelector('.shr-sh small'); if (sh) sh.textContent = t.name;
    syncPic(); syncMv(); syncPal(); notes(); hilite();
    if (j + part !== paintKey) paint();
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
    if (d.cbPart) { showPart(d.cbPart, false); return; }
    if (d.cbTile !== undefined) { pickTile(d.cbTile, d.cbV); return; }
    if (d.cbLayers) { layOn = cur.api.layers ? cur.api.layers(!layOn) : false; syncLayBtn(); return; }
    if (d.cbGopic) { var dp = cur.box.querySelector('[data-cb-pic]'); dp = dp && dp.closest('details'); if (dp) { dp.open = true; dp.scrollIntoView({ block: 'nearest' }); var f = dp.querySelector('button,input'); if (f) f.focus({ preventScroll: true }); } return; }
    if (d.cbPal) { usePal(); return; }
    if (d.cbMode) { mode = d.cbMode; armed = ''; if (mode === 'place' && layOn && cur.api.layers) { layOn = cur.api.layers(false); } cur.api.place(mode === 'place'); mount(cur.box, cur.api); return; }
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
    if (!box._cb) { box._cb = 1; box.addEventListener('keydown', onTabKey); box.addEventListener('click', onClick); box.addEventListener('input', onInput); box.addEventListener('change', onInput); box.addEventListener('dragstart', onDrag); box.addEventListener('dragend', function () { armed = ''; sync(); }); }
    var pn = box.closest('.shr-panel'); if (pn) pn.classList.toggle('cbp', mode === 'place');
    paintKey = ''; layOn = !!(api.layers && mode !== 'place' && layOn); drawPart(); syncLayBtn();
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


  /* ---- Dressing by part: the room's parts as tabs, each choice a small
     picture of this room dressed that way, drawn by the room itself. A press
     on a part of the room, or on one of its layers, opens that part's tab. ---- */
  var PARTS = [
    { k: 'wall', name: 'Wall', rows: [['theme.wall', 'Surface', [470, 70, 440, 257]], ['skeleton.shape', 'Shape of the room']], cols: [['wood', 'Wood'], ['trim', 'Trim']], pic: 'Or use one of your own pictures as the wall' },
    { k: 'window', name: 'Window', rows: [['skeleton.opening', 'Opening'], ['theme.scene', 'View outside', 'win'], ['theme.outside', 'Sky', 'win']], pic: 'Or show one of your own pictures through the window' },
    { k: 'shelves', name: 'Shelves', rows: [['skeleton.display', 'Display']] },
    { k: 'counter', name: 'Counter', rows: [['skeleton.counter', 'Counter']], cols: [['cloth', 'Cloth'], ['accent', 'Accent']], crop: [0, 300, 1200, 400] },
    { k: 'light', name: 'Light', rows: [['theme.light', 'Light']], warm: true },
    { k: 'props', name: 'Props' }
  ];
  var part = 'wall', partT = 0, paintKey = '', layOn = false;
  function partBy(k) { return PARTS.filter(function (P) { return P.k === k; })[0]; }
  function partsHtml() {
    return '<div class="shr-cbparts"><div class="shr-cbtabs" role="tablist" aria-label="Parts of the room">' + PARTS.map(function (P) {
      return '<button type="button" role="tab" id="shr-cbt-' + P.k + '" data-cb-part="' + P.k + '" aria-selected="' + (P.k === part) + '" tabindex="' + (P.k === part ? 0 : -1) + '" aria-controls="shr-cbpp">' + P.name + '</button>';
    }).join('') + '</div><div class="shr-cbpp" id="shr-cbpp" role="tabpanel" tabindex="-1" data-cb-pp></div>' +
      '<button type="button" class="shr-link" data-cb-layers="1" aria-pressed="false">Pull the room apart into its layers</button></div>';
  }
  function tileOn(t, path, v) {
    if (path === 'props') return t.props.indexOf(v) >= 0;
    var now = pathGet(t, path); if (now == null) now = pathGet(C.options, path)[0];
    return String(now) === v;
  }
  function tile(t, path, v, label) {
    return '<button type="button" class="shr-cbtile" data-cb-tile="' + path + '" data-cb-v="' + esc(v) + '" aria-pressed="' + tileOn(t, path, v) + '"><img alt="" data-cb-th><span>' + esc(label) + '</span></button>';
  }
  function tiles(title, body) { return '<div class="shr-field"><span>' + title + '</span><div class="shr-cbtiles" role="group" aria-label="' + title + '">' + body + '</div></div>'; }
  function drawPart() {
    var el = cur && cur.box.querySelector('[data-cb-pp]'); if (!el) return;
    var P = partBy(part), t = cur.api.theme(), h = '';
    if (P.k === 'props') h = '<p class="shr-note">Press a prop to put it in the room or take it out. Place it yourself to choose its spot.</p>' + tiles('Props', C.options.props.map(function (p) { return tile(t, 'props', p, lbl('prop', p)); }).join(''));
    else P.rows.forEach(function (r) { var g = r[0].split('.')[1]; h += tiles(r[1], pathGet(C.options, r[0]).map(function (v) { return tile(t, r[0], v, lbl(g, v)); }).join('')); });
    if (P.pic) h += '<button type="button" class="shr-link" data-cb-gopic="1">' + P.pic + '</button>';
    if (P.cols) h += P.cols.map(function (c) { return colourRow(c[0], c[1], true); }).join('');
    if (P.warm) h += warmthRow();
    el.innerHTML = h; el.setAttribute('aria-labelledby', 'shr-cbt-' + part); paintKey = '';
  }
  function showPart(k, focusTile) {
    if (!partBy(k) || !cur) return;
    part = k;
    Array.prototype.forEach.call(cur.box.querySelectorAll('[data-cb-part]'), function (b) { var on = b.getAttribute('data-cb-part') === k; b.setAttribute('aria-selected', String(on)); b.tabIndex = on ? 0 : -1; });
    drawPart(); sync();
    var f = focusTile && cur.box.querySelector(focusTile) || cur.box.querySelector('[data-cb-part="' + k + '"]');
    if (f) { f.scrollIntoView({ block: 'nearest' }); f.focus({ preventScroll: true }); }
  }
  // The arrow keys move along the tabs, as tabs do; nothing here acts on a key typed into a field.
  function onTabKey(e) {
    var b = e.target; if (!b || b.getAttribute('role') !== 'tab') return;
    var i = PARTS.map(function (P) { return P.k; }).indexOf(b.getAttribute('data-cb-part')), n = PARTS.length;
    var j = e.key === 'ArrowRight' ? (i + 1) % n : e.key === 'ArrowLeft' ? (i + n - 1) % n : e.key === 'Home' ? 0 : e.key === 'End' ? n - 1 : -1;
    if (j < 0) return; e.preventDefault(); showPart(PARTS[j].k, false);
  }
  function pickTile(path, v) {
    var t = cur.api.theme();
    if (path === 'props') {
      var on = t.props.indexOf(v) >= 0;
      if (!on && t.props.length >= MAX_PROPS) { say(FULL); return; }
      change(function (x) { var i = x.props.indexOf(v); if (i >= 0) x.props.splice(i, 1); else x.props.push(v); });
      say((on ? 'Took the ' : 'Put the ') + lbl('prop', v).toLowerCase() + (on ? ' out.' : ' in the room.') + ' Save room to keep it.');
      return;
    }
    change(function (x) { pathSet(x, path, v); });
    say(lbl(path.split('.')[1], v) + '. Save room to keep it.');
  }
  // The room drawn with one choice changed; a view or sky choice shows the drawn view even when a picture fills the window.
  function variant(t, path, v) {
    var x = clone(t);
    if (path === 'props') { if (x.props.indexOf(v) < 0) x.props.push(v); } else pathSet(x, path, v);
    if (/^(theme\.scene|theme\.outside|skeleton\.opening)$/.test(path) && x.theme.picture && x.theme.picture.on === 'outside') delete x.theme.picture;
    return x;
  }
  // Where each window stands, so the view and sky pictures show it close up.
  var WIN = { 'window-right': [740, 50, 340, 198], 'window-left': [120, 50, 340, 198], 'window-centre': [440, 30, 410, 239], 'round-window': [440, 30, 410, 239] };
  // A prop's picture is the part of the room around its slot, at the tiles' shape; a row may name its own part of the room.
  function cropOf(P, t, path, v) {
    if (path !== 'props') {
      var row = (P.rows || []).filter(function (r) { return r[0] === path; })[0], c = row && row[2];
      if (c === 'win') return WIN[t.skeleton.opening] || null;
      return c || P.crop || null;
    }
    var at = C.options.propAt[v].at, q = C.options.slots[at], x = q[0];
    if (at !== 'over' && at !== 'air' && at !== 'lane' && t.skeleton.opening === 'window-left' && t.skeleton.shape !== 'open-air') x = 1200 - q[0] - q[2];
    var w = Math.min(1200, Math.max(q[2], q[3] * 12 / 7) + 80), h = w * 7 / 12;
    return [x + q[2] / 2 - w / 2, q[1] + q[3] / 2 - h / 2, w, h].map(Math.round);
  }
  function paint() {
    clearTimeout(partT);
    if (!cur || !cur.api.thumb || !cur.box.querySelector('[data-cb-tile]')) return;
    paintKey = JSON.stringify(cur.api.theme()) + part;
    partT = setTimeout(function () {
      if (!cur) return;
      var t = cur.api.theme(), P = partBy(part);
      Array.prototype.forEach.call(cur.box.querySelectorAll('[data-cb-tile]'), function (b) {
        var img = b.querySelector('img'), path = b.getAttribute('data-cb-tile'), v = b.getAttribute('data-cb-v');
        cur.api.thumb(variant(t, path, v), cropOf(P, t, path, v), function (u) { if (u && img.isConnected) img.src = u; }, function () { return img.isConnected; });
      });
    }, 120);
  }
  function syncLayBtn() {
    // On a phone the panel drops to the foot while the layers are out, as it does while placing, so they can be seen.
    var pn = cur && cur.box.closest('.shr-panel'); if (pn) pn.classList.toggle('cbp', mode === 'place' || layOn);
    var b = cur && cur.box.querySelector('[data-cb-layers]'); if (!b) return;
    b.setAttribute('aria-pressed', String(layOn)); b.textContent = layOn ? 'Put the room back together' : 'Pull the room apart into its layers';
  }
  function partFromRoom(k) {
    if (!cur || !cur.box.isConnected || mode === 'place') return;
    var pk = String(k).indexOf('props:') === 0 ? 'props' : String(k);
    if (!partBy(pk)) return;
    showPart(pk, pk === 'props' ? '[data-cb-tile="props"][data-cb-v="' + String(k).slice(6).replace(/[^a-z0-9-]/g, '') + '"]' : '');
  }

  /* ---- Colours from the owner's picture: five colours read from it in the
     browser, and one press dresses the room in them (wood, cloth, accent,
     trim and the light's warmth). Only the colours are saved. ---- */
  var pal = { id: '', cols: null };
  function hex2(n) { return ('0' + Math.max(0, Math.min(255, Math.round(n))).toString(16)).slice(-2); }
  function readPal(img) {
    var cv = document.createElement('canvas'), n = 40; cv.width = cv.height = n;
    var x = cv.getContext('2d'); x.drawImage(img, 0, 0, n, n);
    var d = x.getImageData(0, 0, n, n).data, box = {};
    for (var i = 0; i < d.length; i += 4) {
      if (d[i + 3] < 128) continue;
      var k = (d[i] >> 5) + ',' + (d[i + 1] >> 5) + ',' + (d[i + 2] >> 5), b = box[k] || (box[k] = [0, 0, 0, 0]);
      b[0] += d[i]; b[1] += d[i + 1]; b[2] += d[i + 2]; b[3]++;
    }
    var cols = Object.keys(box).map(function (k) { var b = box[k]; return [b[0] / b[3], b[1] / b[3], b[2] / b[3], b[3]]; }).sort(function (a, b) { return b[3] - a[3]; }), out = [];
    cols.forEach(function (c) { if (out.length < 5 && out.every(function (o) { return Math.abs(o[0] - c[0]) + Math.abs(o[1] - c[1]) + Math.abs(o[2] - c[2]) > 70; })) out.push(c); });
    return out.length ? out : null;
  }
  function lum(c) { return .3 * c[0] + .59 * c[1] + .11 * c[2]; }
  function satur(c) { var mx = Math.max(c[0], c[1], c[2]), mn = Math.min(c[0], c[1], c[2]); return mx ? (mx - mn) / mx : 0; }
  function syncPal() {
    var el = cur && cur.box.querySelector('[data-cb-palbox]'); if (!el) return;
    var p = cur.api.theme().theme.picture;
    if (!p) { el.innerHTML = ''; pal = { id: '', cols: null }; return; }
    if (pal.id === p.id) { drawPal(el); return; }
    pal = { id: p.id, cols: null }; el.innerHTML = '';
    var img = new Image(), id = p.id;
    img.onload = function () { if (pal.id !== id) return; try { pal.cols = readPal(img); } catch (e) { pal.cols = null; } var e2 = cur && cur.box.querySelector('[data-cb-palbox]'); if (e2) drawPal(e2); };
    img.src = '/media/' + encodeURIComponent(id) + '/thumb/300';
  }
  function drawPal(el) {
    if (!pal.cols) { el.innerHTML = ''; return; }
    el.innerHTML = '<div class="shr-field" role="group" aria-label="Colours from your picture"><span>Colours from your picture</span><span class="shr-cbsw">' +
      pal.cols.map(function (c) { var h = '#' + hex2(c[0]) + hex2(c[1]) + hex2(c[2]); return '<i style="display:inline-block;width:24px;height:24px;border-radius:6px;background:' + h + ';border:1px solid var(--color-border,#e5e7eb)" title="' + h + '"></i>'; }).join('') +
      '</span><button type="button" class="shr-link" data-cb-pal="1">Dress the room in these colours</button></div>';
  }
  function usePal() {
    var c = pal.cols; if (!c || !c.length) return;
    var byL = c.slice().sort(function (a, b) { return lum(a) - lum(b); }), mid = byL.slice(1, -1), acc = (mid.length ? mid : byL).slice().sort(function (a, b) { return satur(b) - satur(a); })[0];
    function h(x) { return '#' + hex2(x[0]) + hex2(x[1]) + hex2(x[2]); }
    function deep(x, k) { return [x[0] * k, x[1] * k, x[2] * k]; }
    var wood = byL[0], cloth = byL[Math.min(1, byL.length - 1)], trim = byL[byL.length - 1];
    if (lum(wood) > 110) wood = deep(wood, .55);
    var r = c.reduce(function (a, x) { return a + x[0] * x[3]; }, 0), b = c.reduce(function (a, x) { return a + x[2] * x[3]; }, 0), n = c.reduce(function (a, x) { return a + x[3]; }, 0);
    var warm = Math.round(Math.max(0, Math.min(1, .5 + (r - b) / n / 160)) * 20) / 20;
    change(function (t) { t.theme.wood = h(wood); t.theme.cloth = h(cloth); t.theme.accent = h(acc); t.theme.trim = h(trim); t.theme.warmth = warm; });
    say('The room takes its colours from your picture now. Save room to keep them.');
  }

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
  function moved(k, v, soft) { if (!cur) return; sel = k; setMove(k, v, soft); say(mvName(k) + ' moved. Save room to keep it.'); }

  window.ShopRoomBuilder = {
    mount: mount, opening: opening, picked: picked, moved: moved, part: partFromRoom,
    // The room was put back together from its layers (Esc, its own button, or leaving Arrange).
    layered: function (on) { layOn = !!on; syncLayBtn(); },
    // The counter room has just been drawn: remember which props found a spot, and relight the slots once it is in the page.
    placed: function (m) { placedMap = m || {}; clearTimeout(noteT); noteT = setTimeout(function () { notes(); hilite(); }, 0); }
  };
})();
