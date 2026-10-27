/**
 * item_look.js -- Chronicle Item Look Widget
 *
 * Shows how an item shows its magic: a small stage where the item glows on
 * its own, strikes or pours when used, and flares when made, with an optional
 * synthesised sound. The Owner picks the look from a fixed gallery beside the
 * stage and can swap its colour and sound and set the words that float up when
 * it is used; every change saves on its own. Players see the stage only, and
 * the block hides itself when the item has no look or is not an item (the
 * endpoint answers 404). Drawing and sound live in item_look_fx.js.
 *
 * Mount via:
 *   <div data-widget="item_look"
 *        data-look-endpoint="/campaigns/:id/armory/items/:eid/look"
 *        data-item-name="..." data-item-type="..." data-item-image="..."
 *        data-can-edit="true"
 *        data-csrf-token="..."></div>
 */
(function () {
  var CSS = [
    '.ilk{display:grid;grid-template-columns:minmax(0,1.5fr) minmax(0,1fr);gap:16px;align-items:start}',
    '.ilk.solo{grid-template-columns:minmax(0,1fr)}',
    '@media (max-width:760px){.ilk{grid-template-columns:minmax(0,1fr)}}',
    '.ilk-main{display:flex;flex-direction:column;gap:8px;min-width:0}',
    '.ilk-stage{position:relative;border-radius:12px;overflow:hidden;background:#15110e;box-shadow:var(--elev-resting)}',
    '.ilk-stage canvas{display:block;width:100%;aspect-ratio:16/9}',
    '.ilk-bar{display:flex;flex-wrap:wrap;gap:8px;align-items:center}',
    '.ilk-seg{display:inline-flex;gap:2px;padding:3px;border-radius:10px;border:1px solid var(--color-border);background:var(--color-bg-secondary)}',
    '.ilk-seg button{font:inherit;font-size:.85rem;color:var(--color-text-body);background:none;border:0;border-radius:7px;padding:5px 11px;cursor:pointer}',
    '.ilk-seg button:hover{background:var(--color-bg-tertiary)}',
    '.ilk-seg button[aria-pressed="true"]{background:var(--color-accent);color:#fff}',
    '.ilk-snd{margin-left:auto;font:inherit;font-size:.85rem;color:var(--color-text-muted);background:none;border:1px solid transparent;border-radius:8px;padding:5px 9px;cursor:pointer;display:inline-flex;gap:6px;align-items:center}',
    '.ilk-snd:hover{border-color:var(--color-border);color:var(--color-text-primary)}',
    '.ilk-panel{display:flex;flex-direction:column;gap:14px;padding:14px;border-radius:12px;background:var(--color-bg-secondary);box-shadow:var(--elev-resting);min-width:0}',
    '.ilk-h{font-size:.75rem;text-transform:uppercase;letter-spacing:.08em;font-weight:600;color:var(--color-accent);margin:0}',
    '.ilk-f{display:flex;flex-direction:column;gap:7px;min-width:0}',
    '.ilk-gal{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:7px}',
    '.ilk-tile{font:inherit;text-align:left;display:flex;flex-direction:column;gap:3px;padding:0 0 6px;border-radius:10px;border:1px solid transparent;background:var(--color-bg-tertiary);color:var(--color-text-body);cursor:pointer;overflow:hidden;min-width:0}',
    '.ilk-tile canvas{display:block;width:100%;height:52px}',
    '.ilk-tile b{font-size:.78rem;font-weight:600;color:var(--color-text-primary);padding:0 8px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.ilk-tile:hover{border-color:var(--color-border)}',
    '.ilk-tile[aria-pressed="true"]{border-color:var(--color-accent);box-shadow:inset 2px 0 0 var(--color-accent)}',
    '.ilk-sw{display:flex;flex-wrap:wrap;gap:6px}',
    '.ilk-sw button{width:24px;height:24px;border-radius:50%;padding:0;cursor:pointer;border:2px solid var(--color-bg-secondary);box-shadow:0 0 0 1px var(--color-border)}',
    '.ilk-sw button[aria-pressed="true"]{box-shadow:0 0 0 2px var(--color-accent)}',
    '.ilk-sw .own{background:conic-gradient(#ff7a2f,#ffd66b,#8fe07a,#8fd8ff,#a99bff,#ff7a2f)}',
    '.ilk-row{display:grid;grid-template-columns:1fr 1fr;gap:12px}',
    '.ilk-l{display:flex;flex-direction:column;gap:3px;font-size:.78rem;color:var(--color-text-muted);min-width:0}',
    '.ilk-l select,.ilk-l input{font:inherit;font-size:.87rem;color:var(--color-text-primary);background:transparent;border:0;border-bottom:1px solid var(--color-border);border-radius:0;padding:4px 2px;min-width:0}',
    '.ilk-l select:focus,.ilk-l input:focus{outline:none;border-bottom-color:var(--color-accent)}',
    '.ilk-foot{display:flex;gap:12px;align-items:baseline;justify-content:space-between;font-size:.82rem;color:var(--color-text-muted)}',
    '.ilk-foot .err{color:var(--color-danger)}',
    '.ilk-q{font:inherit;font-size:.85rem;color:var(--color-accent);background:none;border:0;padding:0;cursor:pointer}',
    '.ilk-q:hover{text-decoration:underline}',
    '.ilk-empty{font-size:.87rem;color:var(--color-text-muted)}'
  ].join('\n');
  function injectStyle() {
    if (document.getElementById('ilk-style')) return;
    var st = document.createElement('style'); st.id = 'ilk-style'; st.textContent = CSS; document.head.appendChild(st);
  }
  var SPK_ON = '<i class="fa-solid fa-volume-high" aria-hidden="true"></i>', SPK_OFF = '<i class="fa-solid fa-volume-xmark" aria-hidden="true"></i>';
  var bars = [];
  function paintSound(FX) { bars.forEach(function (b) { b.innerHTML = (FX.Sound.on ? SPK_ON + 'Sound on' : SPK_OFF + 'Sound off'); b.setAttribute('aria-pressed', FX.Sound.on ? 'true' : 'false'); }); }

  Chronicle.register('item_look', {
    init: function (el) {
      var FX = window.ItemLookFX; if (!FX) return;
      injectStyle();
      var ds = el.dataset, canEdit = ds.canEdit === 'true', saved = null, img = null;
      el.hidden = true;
      if (ds.itemImage) { img = new Image(); img.src = ds.itemImage; }
      var kind = FX.kindFor(ds.itemType, !!ds.itemImage);
      var thing = { name: ds.itemName || 'Item', kind: kind, img: img, target: kind === 'flask' ? 'ally' : 'dummy' };

      el.innerHTML = '<div class="ilk' + (canEdit ? '' : ' solo') + '"><div class="ilk-main">' +
        '<div class="ilk-stage"><canvas role="img"></canvas></div>' +
        '<div class="ilk-bar"><span class="ilk-seg" role="group" aria-label="Moment">' +
        '<button type="button" data-m="idle" aria-pressed="true">On the card</button><button type="button" data-m="use" aria-pressed="false">When used</button><button type="button" data-m="made" aria-pressed="false">When made</button></span>' +
        '<button type="button" class="ilk-snd"></button></div></div></div>';
      var root = el.firstChild, cv = root.querySelector('canvas'), seg = root.querySelector('.ilk-seg'), snd = root.querySelector('.ilk-snd');
      cv.setAttribute('aria-label', thing.name + ', its look');
      var stage = new FX.Stage(cv), prev = 'idle';
      stage.onTick = function (s) { if (prev !== 'idle' && s.mode === 'idle') paintMoment('idle'); prev = s.mode; };
      function paintMoment(m) { seg.querySelectorAll('button').forEach(function (b) { b.setAttribute('aria-pressed', b.dataset.m === m ? 'true' : 'false'); }); }
      seg.addEventListener('click', function (e) { var b = e.target.closest('button[data-m]'); if (!b) return; paintMoment(b.dataset.m); prev = b.dataset.m; stage.play(b.dataset.m); });
      bars.push(snd); paintSound(FX);
      snd.addEventListener('click', function () { FX.Sound.on = !FX.Sound.on; if (FX.Sound.on) FX.Sound.ensure(); paintSound(FX); });
      if (img) img.onload = function () { stage.set(thing, stage.look); };

      function show() {
        var L = FX.resolve(saved) || FX.resolve({ look: 'quiet' });
        thing.target = (thing.kind === 'flask' || L.heals) ? 'ally' : 'dummy';
        stage.set(thing, L);
      }

      // ---- The Owner's gallery ----
      var panel = null, minis = [], status = null, timer = null, seq = 0;
      function save() {
        clearTimeout(timer);
        status.textContent = 'Saving…'; status.className = '';
        timer = setTimeout(function () {
          var mine = ++seq, body = saved ? { look: saved.look, tint: saved.tint || '', sound: saved.sound || '', words: saved.words || '' } : { look: '' };
          Chronicle.apiFetch(ds.lookEndpoint, { method: 'PUT', body: body, csrfToken: ds.csrfToken }).then(function (r) {
            if (mine !== seq) return;
            if (!r.ok) throw new Error(String(r.status));
            status.textContent = saved ? 'Saved. Players see this look.' : 'Look removed.';
          }).catch(function () {
            if (mine !== seq) return;
            status.textContent = 'That did not save. Change anything to try again.'; status.className = 'err';
          });
        }, 350);
      }
      function buildPanel() {
        panel = document.createElement('div'); panel.className = 'ilk-panel';
        panel.innerHTML = '<div class="ilk-f"><p class="ilk-h">Look</p><div class="ilk-gal"></div></div>' +
          '<div class="ilk-f"><p class="ilk-h">Colour</p><div class="ilk-sw"></div></div>' +
          '<div class="ilk-row"><label class="ilk-l">Sound<select data-k="sound"></select></label>' +
          '<label class="ilk-l">Words when it lands<input type="text" data-k="words" maxlength="24"></label></div>' +
          '<div class="ilk-foot"><span role="status" aria-live="polite"></span><button type="button" class="ilk-q" data-k="clear">Remove look</button></div>';
        root.appendChild(panel);
        status = panel.querySelector('[role="status"]');
        var gal = panel.querySelector('.ilk-gal');
        Object.keys(FX.LOOKS).forEach(function (k) {
          var L = FX.LOOKS[k], b = document.createElement('button');
          b.type = 'button'; b.className = 'ilk-tile'; b.dataset.look = k;
          var c = document.createElement('canvas'); c.setAttribute('aria-hidden', 'true'); b.appendChild(c);
          var n = document.createElement('b'); n.textContent = L.name; b.appendChild(n);
          b.setAttribute('aria-label', L.name + ': ' + (FX.MOTIF_NAMES[L.motif] || 'glints') + ', ' + FX.SOUND_NAMES[L.sound]);
          gal.appendChild(b); minis.push(new FX.Mini(c, L));
        });
        gal.addEventListener('click', function (e) {
          var b = e.target.closest('.ilk-tile'); if (!b) return;
          saved = { look: b.dataset.look, tint: '', sound: '', words: saved ? saved.words : '' };
          paintPanel(); show(); stage.play('use'); paintMoment('use'); prev = 'use'; save();
        });
        var sel = panel.querySelector('select');
        sel.innerHTML = '<option value="">The look’s own</option>' + FX.SOUNDS.map(function (k) { return '<option value="' + k + '">' + FX.SOUND_NAMES[k] + '</option>'; }).join('');
        sel.addEventListener('change', function () { if (!saved) return; saved.sound = sel.value; show(); var L = FX.resolve(saved); FX.Sound.play(L.sound, L.pitch); save(); });
        var words = panel.querySelector('input');
        words.addEventListener('input', function () { if (!saved) return; saved.words = words.value; show(); save(); });
        panel.querySelector('.ilk-sw').addEventListener('click', function (e) {
          var b = e.target.closest('button'); if (!b || !saved) return;
          saved.tint = b.dataset.tint || ''; paintPanel(); show(); stage.play('use'); paintMoment('use'); prev = 'use'; save();
        });
        panel.querySelector('[data-k="clear"]').addEventListener('click', function () { saved = null; paintPanel(); show(); save(); });
      }
      function paintPanel() {
        panel.querySelectorAll('.ilk-tile').forEach(function (b) { b.setAttribute('aria-pressed', saved && b.dataset.look === saved.look ? 'true' : 'false'); });
        var sw = panel.querySelector('.ilk-sw');
        sw.innerHTML = '<button type="button" class="own" data-tint="" aria-label="The look’s own colour" title="The look’s own colour"></button>' +
          FX.TINTS.map(function (c) { return '<button type="button" data-tint="' + c + '" style="background:' + c + '" aria-label="Colour ' + c + '"></button>'; }).join('');
        sw.querySelectorAll('button').forEach(function (b) { b.disabled = !saved; b.setAttribute('aria-pressed', saved && (saved.tint || '') === b.dataset.tint ? 'true' : 'false'); });
        var sel = panel.querySelector('select'), words = panel.querySelector('input');
        sel.disabled = words.disabled = !saved;
        sel.value = saved ? (saved.sound || '') : '';
        if (document.activeElement !== words) words.value = saved ? (saved.words || '') : '';
        panel.querySelector('[data-k="clear"]').hidden = !saved;
        if (!saved && !status.textContent) status.textContent = 'No look yet. Pick one and players see it.';
      }

      Chronicle.apiFetch(ds.lookEndpoint).then(function (r) {
        if (!r.ok) return null;
        return r.json();
      }).catch(function () { return null; }).then(function (j) {
        if (!j) return; // not an item, hidden, or the armory is off
        saved = FX.resolve(j.look) ? j.look : null;
        if (!saved && !canEdit) return;
        el.hidden = false;
        if (canEdit) { buildPanel(); paintPanel(); }
        show();
      });

      el._ilk = { stage: stage, minis: minis, snd: snd };
    },
    destroy: function (el) {
      var s = el._ilk; if (!s) return;
      s.stage.destroy(); s.minis.forEach(function (m) { m.destroy(); });
      var i = bars.indexOf(s.snd); if (i >= 0) bars.splice(i, 1);
      el._ilk = null;
    }
  });
})();
