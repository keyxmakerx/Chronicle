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
 * Every change redraws the room in place; players never load this file's UI.
 */
(function () {
  'use strict';

  var C = null, cur = null, mode = 'lists', armed = '', placedMap = {}, noteT = 0;
  var LBL = {
    shape: { 'open-air': 'Open-air stall' },
    opening: { none: 'No window', 'window-left': 'Window, left', 'window-right': 'Window, right', 'window-centre': 'Window, centre', 'round-window': 'Round window', doorway: 'Doorway' },
    counter: { 'stall-table': 'Stall table' },
    prop: { 'floating-books': 'Floating books', 'steam-cart': 'Steam cart' }
  };
  var SKEL = [['shape', 'Back wall'], ['opening', 'Opening'], ['display', 'Display'], ['counter', 'Counter']];
  var LOOK = [['wall', 'Wall'], ['light', 'Light'], ['outside', 'Outside']];
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
    return '<div class="shr-field"><span>Starting from</span><span class="shr-chips" role="group" aria-label="Starting from">' +
      C.starters.map(function (s, i) { return '<button type="button" class="shr-chip" data-cb-start="' + i + '" aria-pressed="false">' + esc(s.name) + '</button>'; }).join('') + '</span>' +
      '<button type="button" class="shr-link" data-cb-reset="1" hidden>Back to this shop type’s starter</button></div>';
  }
  function html() {
    var O = C.options, place = mode === 'place', h = '';
    h += '<span class="shr-seg" role="group" aria-label="How to build">' +
      '<button type="button" data-cb-mode="lists" aria-pressed="' + !place + '">Pick from lists</button><button type="button" data-cb-mode="place" aria-pressed="' + place + '">Place it yourself</button></span>';
    if (place) {
      h += '<p class="shr-note">Dashed slots are on the room. Drag a prop to its slot, or press a prop and then its slot. Press the window to move it: left, centre, right or none.</p>' +
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
    return h + '<p class="shr-note" role="status" aria-live="polite" data-cb-say></p>';
  }
  function say(t) { var e = cur && cur.box.querySelector('[data-cb-say]'); if (e) e.textContent = t || ''; }

  // Keeps every control in step with the theme, without rebuilding them (a colour picker stays open while it is dragged).
  function sync() {
    var b = cur.box, t = cur.api.theme(), j = JSON.stringify(t);
    Array.prototype.forEach.call(b.querySelectorAll('[data-cb-path]'), function (c) {
      var v = pathGet(t, c.getAttribute('data-cb-path')); if (String(c.value) !== String(v)) c.value = v;
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
    notes(); hilite();
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
    if (d.cbSide) { var pn = b.closest('.shr-panel'); if (pn) pn.classList.toggle('cbl'); }
  }
  function onInput(e) {
    var c = e.target, path = c.getAttribute && c.getAttribute('data-cb-path'), pr = c.getAttribute && c.getAttribute('data-cb-prop');
    if (path) { var v = c.type === 'range' ? +c.value : c.value; change(function (t) { pathSet(t, path, v); }, c.type === 'range' || c.type === 'color'); }
    else if (pr && e.type === 'change') change(function (t) { var i = t.props.indexOf(pr); if (c.checked && i < 0) t.props.push(pr); else if (!c.checked && i >= 0) t.props.splice(i, 1); });
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
    box.className = 'shr-cb'; box.innerHTML = html();
    if (!box._cb) { box._cb = 1; box.addEventListener('click', onClick); box.addEventListener('input', onInput); box.addEventListener('change', onInput); box.addEventListener('dragstart', onDrag); box.addEventListener('dragend', function () { armed = ''; sync(); }); }
    var pn = box.closest('.shr-panel'); if (pn) pn.classList.toggle('cbp', mode === 'place');
    sync();
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

  window.ShopRoomBuilder = {
    mount: mount, opening: opening,
    // The counter room has just been drawn: remember which props found a spot, and relight the slots once it is in the page.
    placed: function (m) { placedMap = m || {}; clearTimeout(noteT); noteT = setTimeout(function () { notes(); hilite(); }, 0); }
  };
})();
