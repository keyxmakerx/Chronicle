/**
 * item_look.js -- Chronicle Item Look Widget
 *
 * Shows how an item shows its magic: a small stage where the item glows on
 * its own, strikes or pours when used, and flares when made, with an optional
 * synthesised sound. The Owner picks the look from a fixed gallery beside the
 * stage and can swap its colour and sound and set the words that float up when
 * it is used; every change saves on its own. The Owner can also put looks on
 * five spots of the stage (the item, its trail, the point of impact, the
 * target's body, the ground) by dragging a gallery tile onto a spot, or by
 * tapping a spot and then a look. "Made from" names the page the item was
 * made from (a Remnant, say), and the item carries that page's look: as its
 * look when it has none of its own, shown on top when it has. The same block
 * on the Remnant's page sets that look. Players see the stage only, and
 * the block hides itself when the page has no look and carries none, or is
 * hidden from them (the endpoint answers 404). Drawing and sound live in item_look_fx.js.
 *
 * Mount via:
 *   <div data-widget="item_look"
 *        data-look-endpoint="/campaigns/:id/armory/items/:eid/look"
 *        data-item-name="..." data-item-type="..." data-item-image="..."
 *        data-entity-search-endpoint="/campaigns/:id/entities/search"
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
    '.ilk-empty{font-size:.87rem;color:var(--color-text-muted)}',
    '.ilk-spots{position:absolute;inset:0;pointer-events:none;transition:opacity .25s}',
    '.ilk-spots.playing{opacity:.18}',
    '.ilk-spot{position:absolute;transform:translate(-50%,-50%);pointer-events:auto;font:inherit;width:40px;height:40px;border-radius:50%;border:2px dashed rgba(255,240,220,.55);background:rgba(20,14,10,.35);cursor:pointer;display:grid;place-items:center;padding:0;transition:transform .12s,border-color .12s}',
    '.ilk-spot i{width:13px;height:13px;border-radius:50%}',
    '.ilk-spot.full{border-style:solid;border-color:rgba(255,240,220,.85)}',
    '.ilk-spot .lab{position:absolute;top:100%;left:50%;transform:translateX(-50%);margin-top:3px;font-size:.68rem;font-weight:600;color:#f6ead2;white-space:nowrap;text-shadow:0 1px 3px #000;pointer-events:none}',
    '.ilk-spot.hot,.ilk-spot[aria-pressed="true"]{border-color:var(--color-accent);transform:translate(-50%,-50%) scale(1.2);background:rgba(20,14,10,.6)}',
    '.ilk-spot:focus-visible{outline:2px solid var(--color-accent);outline-offset:2px}',
    '@media (max-width:560px){.ilk-spot{width:32px;height:32px}.ilk-spot .lab{font-size:.6rem}}',
    '.ilk-tile.lifted{opacity:.35}',
    '.ilk-ghost{position:fixed;z-index:60;pointer-events:none;width:110px;border-radius:10px;overflow:hidden;box-shadow:var(--elev-raised,0 10px 30px rgba(0,0,0,.35));transform:translate(-50%,-60%) rotate(-3deg);background:var(--color-bg-tertiary);font-size:.78rem;font-weight:600;color:var(--color-text-primary)}',
    '.ilk-ghost canvas{display:block;width:100%;height:46px}',
    '.ilk-ghost b{display:block;padding:4px 8px 6px}',
    '.ilk-hint{font-size:.8rem;color:var(--color-text-muted);margin:0}',
    '.ilk-srow{display:grid;grid-template-columns:12px minmax(0,1fr) auto;gap:10px;align-items:center;font-size:.84rem;color:var(--color-text-body)}',
    '.ilk-srow .dot{width:12px;height:12px;border-radius:50%;border:1px dashed var(--color-text-muted)}',
    '.ilk-srow b{color:var(--color-text-primary);font-weight:600}',
    '.ilk-srow em{font-style:normal;color:var(--color-text-muted);font-size:.76rem;display:block}',
    '.ilk-from{display:flex;flex-wrap:wrap;gap:4px 12px;align-items:baseline;font-size:.87rem;color:var(--color-text-body)}',
    '.ilk-from a{color:var(--color-text-primary);font-weight:600}',
    '.ilk-res{display:flex;flex-direction:column;gap:2px}',
    '.ilk-res:empty{display:none}',
    '.ilk-res button{font:inherit;font-size:.86rem;text-align:left;color:var(--color-text-body);background:none;border:0;border-radius:8px;padding:6px 8px;cursor:pointer;display:flex;gap:8px;align-items:baseline;min-width:0}',
    '.ilk-res button:hover,.ilk-res button:focus-visible{background:var(--color-bg-tertiary);outline:none}',
    '.ilk-res button b{font-weight:600;color:var(--color-text-primary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.ilk-res button span{font-size:.76rem;color:var(--color-text-muted);white-space:nowrap}'
  ].join('\n');
  function injectStyle() {
    if (document.getElementById('ilk-style')) return;
    var st = document.createElement('style'); st.id = 'ilk-style'; st.textContent = CSS; document.head.appendChild(st);
  }
  var SPK_ON = '<i class="fa-solid fa-volume-high" aria-hidden="true"></i>', SPK_OFF = '<i class="fa-solid fa-volume-xmark" aria-hidden="true"></i>';
  var bars = [];
  /* Where each spot's marker sits, in the stage's 960 x 540 drawing space,
     and when it plays. The keys are ItemLookFX.SPOTS. */
  var SPOT_UI = {
    item:   { name: 'The item', when: 'always, quietly', x: 270, y: 300 },
    trail:  { name: 'The trail', when: 'on the way to the target', x: 495, y: 248 },
    impact: { name: 'Point of impact', when: 'the moment it lands', x: 720, y: 238 },
    body:   { name: 'On the target', when: 'lingers after the hit', x: 772, y: 318 },
    ground: { name: 'The ground', when: 'spreads under the target', x: 652, y: 392 }
  };
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
      var marks = null, armed = null;
      stage.onTick = function (s) {
        if (prev !== 'idle' && s.mode === 'idle') paintMoment('idle'); prev = s.mode;
        // The markers fade while a moment plays, so they never hide the effect.
        if (marks) marks.classList.toggle('playing', s.mode !== 'idle' && !armed);
      };
      function paintMoment(m) { seg.querySelectorAll('button').forEach(function (b) { b.setAttribute('aria-pressed', b.dataset.m === m ? 'true' : 'false'); }); }
      seg.addEventListener('click', function (e) { var b = e.target.closest('button[data-m]'); if (!b) return; paintMoment(b.dataset.m); prev = b.dataset.m; stage.play(b.dataset.m); });
      bars.push(snd); paintSound(FX);
      snd.addEventListener('click', function () { FX.Sound.on = !FX.Sound.on; if (FX.Sound.on) FX.Sound.ensure(); paintSound(FX); });
      if (img) img.onload = function () { stage.set(thing, stage.look); };

      // The item's own look fills the item and impact spots until a look is
      // put there, so placing a trail adds to the look instead of replacing
      // it. Other empty spots stay quiet.
      var BASE_SPOTS = ['item', 'impact'];
      function withBase(sp, base) {
        if (!sp || !Object.keys(sp).length) return null;
        var out = {}; Object.keys(sp).forEach(function (k) { out[k] = sp[k]; });
        BASE_SPOTS.forEach(function (k) { if (!out[k] && base) out[k] = { look: base.look, tint: base.tint }; });
        return out;
      }
      // source is the page this one was made from: {id, name, look}, where a
      // player who can't see that page gets only its look.
      var source = null;
      function srcLook() { return source && source.look && source.look.look ? source.look : null; }
      function own() { return saved && saved.look ? saved : null; }
      function show() {
        var L = FX.carried(saved, srcLook()) || FX.resolve({ look: 'quiet' });
        // Spots are the item's own; an item with no look of its own takes
        // its source's spots with its source's look.
        var base = own() || srcLook(), sp = saved && saved.spots && Object.keys(saved.spots).length ? saved.spots : (!own() && srcLook() ? srcLook().spots : null);
        L.spots = base ? FX.spotLayers(withBase(sp, base)) : null;
        thing.target = (thing.kind === 'flask' || L.heals) ? 'ally' : 'dummy';
        stage.set(thing, L);
      }

      // ---- The Owner's gallery ----
      var panel = null, minis = [], status = null, timer = null, seq = 0, hint = null;
      var HINT = 'Drag a look onto a spot on the stage, or tap a spot and then a look.';
      function save() {
        clearTimeout(timer);
        status.textContent = 'Saving…'; status.className = '';
        timer = setTimeout(function () {
          var mine = ++seq, body = saved ? { look: saved.look || '', tint: saved.tint || '', sound: saved.sound || '', words: saved.words || '' } : { look: '' };
          if (saved && saved.spots && Object.keys(saved.spots).length) body.spots = saved.spots;
          if (saved && saved.madeFrom) body.madeFrom = saved.madeFrom;
          Chronicle.apiFetch(ds.lookEndpoint, { method: 'PUT', body: body, csrfToken: ds.csrfToken }).then(function (r) {
            if (mine !== seq) return;
            if (!r.ok) throw new Error(String(r.status));
            return r.json().then(function (j) {
              if (mine !== seq) return;
              // The answer carries the source page's look, which the Owner
              // may just have picked.
              source = j && j.source ? j.source : null;
              paintPanel(); show();
              status.textContent = saved ? 'Saved. Players see this look.' : 'Look removed.';
            });
          }).catch(function () {
            if (mine !== seq) return;
            status.textContent = 'That did not save. Change anything to try again.'; status.className = 'err';
          });
        }, 350);
      }
      function buildPanel() {
        panel = document.createElement('div'); panel.className = 'ilk-panel';
        panel.innerHTML = '<div class="ilk-f"><p class="ilk-h">Look</p><div class="ilk-gal"></div></div>' +
          '<div class="ilk-f"><p class="ilk-h">Made from</p><div class="ilk-from"></div>' +
          '<label class="ilk-l">Find the page it was made from<input type="search" data-k="from" autocomplete="off" placeholder="A Remnant, a creature, a place…"></label>' +
          '<div class="ilk-res" role="group" aria-label="Pages found"></div></div>' +
          '<div class="ilk-f"><p class="ilk-h">Spots</p><p class="ilk-hint" aria-live="polite"></p><div class="ilk-slist"></div></div>' +
          '<div class="ilk-f"><p class="ilk-h">Colour</p><div class="ilk-sw"></div></div>' +
          '<div class="ilk-row"><label class="ilk-l">Sound<select data-k="sound"></select></label>' +
          '<label class="ilk-l">Words when it lands<input type="text" data-k="words" maxlength="24"></label></div>' +
          '<div class="ilk-foot"><span role="status" aria-live="polite"></span><button type="button" class="ilk-q" data-k="clear">Remove look</button></div>';
        root.appendChild(panel);
        status = panel.querySelector('[role="status"]');
        hint = panel.querySelector('.ilk-hint'); hint.textContent = HINT;
        var gal = panel.querySelector('.ilk-gal'); gal.setAttribute('role', 'group');
        buildSpots();
        Object.keys(FX.LOOKS).forEach(function (k) {
          var L = FX.LOOKS[k], b = document.createElement('button');
          b.type = 'button'; b.className = 'ilk-tile'; b.dataset.look = k;
          var c = document.createElement('canvas'); c.setAttribute('aria-hidden', 'true'); b.appendChild(c);
          var n = document.createElement('b'); n.textContent = L.name; b.appendChild(n);
          b.setAttribute('aria-label', L.name + ': ' + (FX.MOTIF_NAMES[L.motif] || 'glints') + ', ' + FX.SOUND_NAMES[L.sound]);
          gal.appendChild(b); minis.push(new FX.Mini(c, L));
          dragTile(b, k);
        });
        gal.addEventListener('click', function (e) {
          var b = e.target.closest('.ilk-tile'); if (!b) return;
          if (b.dataset.dragged) { delete b.dataset.dragged; return; }
          if (armed) { put(armed, b.dataset.look); return; }
          saved = { look: b.dataset.look, tint: '', sound: '', words: saved ? saved.words : '', spots: saved ? saved.spots : null, madeFrom: saved ? saved.madeFrom : '' };
          paintPanel(); show(); stage.play('use'); paintMoment('use'); prev = 'use'; save();
        });
        var sel = panel.querySelector('select');
        sel.innerHTML = '<option value="">The look’s own</option>' + FX.SOUNDS.map(function (k) { return '<option value="' + k + '">' + FX.SOUND_NAMES[k] + '</option>'; }).join('');
        sel.addEventListener('change', function () { if (!saved) return; saved.sound = sel.value; show(); var L = FX.carried(saved, srcLook()); if (L) FX.Sound.play(L.sound, L.pitch); save(); });
        var words = panel.querySelector('[data-k="words"]');
        words.addEventListener('input', function () { if (!saved) return; saved.words = words.value; show(); save(); });
        panel.querySelector('.ilk-sw').addEventListener('click', function (e) {
          var b = e.target.closest('button'); if (!b || !own()) return;
          saved.tint = b.dataset.tint || ''; paintPanel(); show(); stage.play('use'); paintMoment('use'); prev = 'use'; save();
        });
        panel.querySelector('[data-k="clear"]').addEventListener('click', function () { saved = null; source = null; armed = null; paintPanel(); show(); save(); });
        buildFrom();
      }
      // ---- Spots: markers over the stage, the drop targets ----
      function buildSpots() {
        marks = document.createElement('div'); marks.className = 'ilk-spots';
        FX.SPOTS.forEach(function (k) {
          var u = SPOT_UI[k], b = document.createElement('button');
          b.type = 'button'; b.className = 'ilk-spot'; b.dataset.spot = k;
          b.style.left = (u.x / 9.6) + '%'; b.style.top = (u.y / 5.4) + '%';
          b.innerHTML = '<i></i><span class="lab"></span>'; b.querySelector('.lab').textContent = u.name;
          marks.appendChild(b);
        });
        marks.addEventListener('click', function (e) {
          var b = e.target.closest('.ilk-spot'); if (!b) return;
          armed = armed === b.dataset.spot ? null : b.dataset.spot;
          paintPanel();
        });
        root.querySelector('.ilk-stage').appendChild(marks);
        panel.querySelector('.ilk-slist').addEventListener('click', function (e) {
          var b = e.target.closest('button[data-empty]'); if (b) put(b.dataset.empty, null);
        });
      }
      function put(spot, look) {
        armed = null;
        if (!saved) { if (!look) return; saved = { look: look, tint: '', sound: '', words: '' }; }
        var sp = {}; Object.keys(saved.spots || {}).forEach(function (k) { sp[k] = saved.spots[k]; });
        if (look) sp[spot] = { look: look }; else delete sp[spot];
        saved.spots = sp;
        paintPanel(); show();
        var m = look || spot !== 'item' ? 'use' : 'idle'; stage.play(m); paintMoment(m); prev = m;
        save();
      }
      function spotAt(x, y) {
        var hit = null;
        marks.querySelectorAll('.ilk-spot').forEach(function (b) { var r = b.getBoundingClientRect(); if (Math.hypot(x - r.left - r.width / 2, y - r.top - r.height / 2) < Math.max(34, r.width)) hit = b.dataset.spot; });
        return hit;
      }
      // Drag with a mouse or pen. Touch keeps scrolling the panel and uses
      // tap-a-spot-then-a-look instead.
      function dragTile(b, k) {
        var drag = null, ghost = null, gm = null;
        function hot(h) { marks.querySelectorAll('.ilk-spot').forEach(function (m) { m.classList.toggle('hot', m.dataset.spot === h); }); }
        function end(e, cancelled) {
          if (!drag) return;
          var was = drag; drag = null; b.classList.remove('lifted'); hot(null);
          if (ghost) { gm.destroy(); ghost.remove(); ghost = null; }
          if (!was.moved) return;
          // The click that follows a drag is not a pick.
          b.dataset.dragged = '1'; setTimeout(function () { delete b.dataset.dragged; }, 0);
          var h = cancelled ? null : spotAt(e.clientX, e.clientY); if (h) put(h, k);
        }
        b.addEventListener('pointerdown', function (e) {
          if (e.pointerType === 'touch' || e.button !== 0) return;
          drag = { x: e.clientX, y: e.clientY, moved: false };
          try { b.setPointerCapture(e.pointerId); } catch (er) { /* capture is a nicety */ }
        });
        b.addEventListener('pointermove', function (e) {
          if (!drag) return;
          if (!drag.moved && Math.hypot(e.clientX - drag.x, e.clientY - drag.y) > 6) {
            drag.moved = true; b.classList.add('lifted');
            ghost = document.createElement('div'); ghost.className = 'ilk-ghost'; ghost.setAttribute('aria-hidden', 'true');
            var gc = document.createElement('canvas'), gn = document.createElement('b'); gn.textContent = FX.LOOKS[k].name;
            ghost.appendChild(gc); ghost.appendChild(gn); document.body.appendChild(ghost); gm = new FX.Mini(gc, FX.LOOKS[k]);
          }
          if (drag.moved) { ghost.style.left = e.clientX + 'px'; ghost.style.top = e.clientY + 'px'; hot(spotAt(e.clientX, e.clientY)); }
        });
        b.addEventListener('pointerup', function (e) { end(e, false); });
        b.addEventListener('pointercancel', function (e) { end(e, true); });
      }
      // ---- Made from: search the campaign's pages and pick one ----
      function buildFrom() {
        var input = panel.querySelector('[data-k="from"]'), res = panel.querySelector('.ilk-res'), t = null, q = 0;
        input.addEventListener('input', function () {
          clearTimeout(t); var text = input.value.trim();
          if (text.length < 2 || !ds.entitySearchEndpoint) { res.innerHTML = ''; return; }
          t = setTimeout(function () {
            var mine = ++q;
            Chronicle.apiFetch(ds.entitySearchEndpoint + '?pages=1&q=' + encodeURIComponent(text)).then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; }).then(function (j) {
              if (mine !== q) return;
              res.innerHTML = '';
              ((j && j.results) || []).filter(function (p) { return p.id !== ds.entityId; }).slice(0, 6).forEach(function (p) {
                var b = document.createElement('button'), n = document.createElement('b'), k = document.createElement('span');
                b.type = 'button'; b.dataset.id = p.id; n.textContent = p.name; k.textContent = p.type_name || '';
                b.appendChild(n); b.appendChild(k); b._page = p; res.appendChild(b);
              });
              if (!res.children.length) res.textContent = 'No page by that name.';
            });
          }, 250);
        });
        res.addEventListener('click', function (e) {
          var b = e.target.closest('button[data-id]'); if (!b) return;
          var p = b._page;
          if (!saved) saved = { look: '', tint: '', sound: '', words: '' };
          saved.madeFrom = p.id; source = { id: p.id, name: p.name, url: p.url, look: null };
          input.value = ''; res.innerHTML = '';
          paintPanel(); show(); save();
        });
        panel.querySelector('.ilk-from').addEventListener('click', function (e) {
          var b = e.target.closest('button[data-k]'); if (!b || !saved) return;
          if (b.dataset.k === 'unfrom') { saved.madeFrom = ''; source = null; if (!saved.look) saved = null; }
          else if (b.dataset.k === 'only') { saved.look = ''; saved.tint = ''; }
          paintPanel(); show(); stage.play('use'); paintMoment('use'); prev = 'use'; save();
        });
      }
      function paintFrom() {
        var box = panel.querySelector('.ilk-from'); box.innerHTML = '';
        if (!source || !source.id) {
          box.textContent = 'Nothing. Pick the page it was made from, such as a Remnant, and it carries that page’s look.';
          return;
        }
        // The link comes from the server's answer, which builds it from the
        // page id; a page id from the page itself never becomes a link.
        var a = document.createElement(source.url && /^\/campaigns\/[\w-]+\/entities\/[\w-]+$/.test(source.url) ? 'a' : 'b');
        if (a.tagName === 'A') a.href = source.url;
        a.textContent = source.name;
        var say = document.createElement('span');
        say.textContent = !srcLook() ? 'has no look yet. Give it one on its own page.' : own() ? 'shows through on top of this look.' : 'gives this item its look.';
        box.appendChild(a); box.appendChild(say);
        if (own() && srcLook()) box.appendChild(btn('only', 'Use only its look'));
        box.appendChild(btn('unfrom', 'Clear'));
        function btn(k, text) { var b = document.createElement('button'); b.type = 'button'; b.className = 'ilk-q'; b.dataset.k = k; b.textContent = text; return b; }
      }
      function paintSpots() {
        var sp = (saved && saved.spots) || {};
        marks.querySelectorAll('.ilk-spot').forEach(function (b) {
          var k = b.dataset.spot, L = sp[k] ? FX.LOOKS[sp[k].look] : null, dot = b.querySelector('i');
          b.classList.toggle('full', !!L);
          dot.style.background = L ? L.color : 'transparent'; dot.style.boxShadow = L ? '0 0 12px 3px ' + L.color : 'none';
          b.setAttribute('aria-pressed', armed === k ? 'true' : 'false');
          b.setAttribute('aria-label', SPOT_UI[k].name + ': ' + (L ? L.name : 'empty') + (armed === k ? '. Now choose a look.' : '. Press, then choose a look.'));
        });
        var list = panel.querySelector('.ilk-slist'); list.innerHTML = '';
        FX.SPOTS.forEach(function (k) {
          var L = sp[k] ? FX.LOOKS[sp[k].look] : null, row = document.createElement('div'); row.className = 'ilk-srow';
          var dot = document.createElement('span'); dot.className = 'dot'; if (L) dot.style.cssText = 'background:' + L.color + ';border-color:' + L.color;
          var txt = document.createElement('span'), nm = document.createElement('b'), em = document.createElement('em');
          nm.textContent = SPOT_UI[k].name; em.textContent = SPOT_UI[k].when;
          var bl = own() || srcLook(), base = !L && bl && BASE_SPOTS.indexOf(k) >= 0 ? FX.LOOKS[bl.look] : null;
          txt.appendChild(nm); txt.appendChild(document.createTextNode(' · ' + (L ? L.name : base ? base.name + ', the item’s look' : 'empty'))); txt.appendChild(em);
          row.appendChild(dot); row.appendChild(txt);
          if (L) { var x = document.createElement('button'); x.type = 'button'; x.className = 'ilk-q'; x.dataset.empty = k; x.textContent = 'Empty'; x.setAttribute('aria-label', 'Empty ' + SPOT_UI[k].name); row.appendChild(x); }
          else row.appendChild(document.createElement('span'));
          list.appendChild(row);
        });
        hint.textContent = armed ? 'Now tap a look to put it on ' + SPOT_UI[armed].name.toLowerCase() + '.' : HINT;
        panel.querySelector('.ilk-gal').setAttribute('aria-label', armed ? 'Looks: choose one for ' + SPOT_UI[armed].name : 'Looks');
      }
      function paintPanel() {
        paintSpots();
        panel.querySelectorAll('.ilk-tile').forEach(function (b) { b.setAttribute('aria-pressed', saved && b.dataset.look === saved.look ? 'true' : 'false'); });
        var sw = panel.querySelector('.ilk-sw');
        sw.innerHTML = '<button type="button" class="own" data-tint="" aria-label="The look’s own colour" title="The look’s own colour"></button>' +
          FX.TINTS.map(function (c) { return '<button type="button" data-tint="' + c + '" style="background:' + c + '" aria-label="Colour ' + c + '"></button>'; }).join('');
        sw.querySelectorAll('button').forEach(function (b) { b.disabled = !own(); b.setAttribute('aria-pressed', own() && (saved.tint || '') === b.dataset.tint ? 'true' : 'false'); });
        var sel = panel.querySelector('select'), words = panel.querySelector('[data-k="words"]');
        paintFrom();
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
        if (!j) return; // hidden, or the armory is off
        saved = j.look && (FX.resolve(j.look) || j.look.madeFrom) ? j.look : null;
        source = j.source || null;
        if (!FX.carried(saved, srcLook()) && !canEdit) return;
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
