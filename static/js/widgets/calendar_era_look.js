/**
 * calendar_era_look.js — the structure editor's "Era look" part.
 *
 * The Owner sets how the eras' colours show behind the calendar's days:
 * colours on or off, the overall feel (Still / Slow and subtle / Lively, or
 * Custom from the fine-tune sliders), and each era's colours, style and
 * optional own feel. A small month beside the controls shows every change
 * live, painted by calendar_era_blend.js exactly as the calendar paints it.
 * Cancel puts back what was saved; Save writes through PUT .../era-look and
 * sends only the eras that changed (absent fields keep what is stored).
 *
 * Mounts on data-widget="calendar_era_look" (calendar_era_look.templ), whose
 * data-config carries the look and the eras, oldest first.
 */
(function () {
  'use strict';

  var EYE = '<i class="fa-solid fa-eye-slash" aria-hidden="true"></i>';

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function hexOr(c, d) { return /^#[0-9a-f]{6}$/i.test(c || '') ? c : /^#[0-9a-f]{3}$/i.test(c || '') ? '#' + c[1] + c[1] + c[2] + c[2] + c[3] + c[3] : d; }
  function grad(e) { var a = hexOr(e.color, '#888888'); return 'linear-gradient(90deg,' + a + ',' + hexOr(e.color_2, a) + ')'; }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }

  // The state the controls edit: the look plus each era's look fields.
  function snapshotOf(st) {
    return JSON.stringify({ look: st.look, eras: st.eras.map(function (e) { return [e.id, e.color, e.color_2, e.style, e.feel]; }) });
  }
  // changedEras lists, per era, only the look fields that differ from what
  // was saved; an unchanged era is left out of the save entirely.
  function changedEras(saved, now) {
    var byId = {};
    saved.forEach(function (e) { byId[e.id] = e; });
    var out = [];
    now.forEach(function (e) {
      var s = byId[e.id], d = { id: e.id }, any = false;
      ['color', 'color_2', 'style', 'feel'].forEach(function (k) {
        if (!s || (s[k] || null) !== (e[k] || null)) { d[k] = e[k] || null; any = true; }
      });
      if (any) out.push(d);
    });
    return out;
  }

  var EraLook = {
    init: function (el, config) {
      var B = window.Chronicle && Chronicle.calendarEraBlend;
      this.el = el;
      this.endpoint = config.endpoint;
      var cfg;
      try { cfg = JSON.parse(el.dataset.config || '{}'); } catch (e) { cfg = {}; }
      if (!B) { el.innerHTML = '<p class="calv5-err">The era look could not load. Reload the page to try again.</p>'; return; }
      this.B = B;
      this.state = { look: B.normalizeLook(cfg.look), eras: (cfg.eras || []).map(function (e) {
        return { id: e.id, name: e.name, color: hexOr(e.color, '#888888'), color_2: e.color_2 ? hexOr(e.color_2, null) : null, style: e.style === 'ink' ? 'ink' : 'gas', feel: B.PRESETS[e.feel] ? e.feel : null, secret: !!e.secret };
      }) };
      this.saved = clone(this.state);
      this.openEra = this.state.eras.length ? this.state.eras[0].id : null;
      this.pair = 0;
      this._build();
      this._bind();
      this._sync();
    },

    destroy: function () {
      if (this.painter) this.painter.destroy();
      if (this._ro) this._ro.disconnect();
      if (this._beforeUnload) window.removeEventListener('beforeunload', this._beforeUnload);
    },

    _build: function () {
      var B = this.B, st = this.state, self = this;
      var presetSeg = function (name, label) {
        return '<div class="el-seg" role="radiogroup" aria-label="' + esc(label) + '" data-seg="' + name + '">' +
          B.PRESET_KEYS.map(function (k) { return '<button type="button" role="radio" data-k="' + k + '">' + esc(B.PRESETS[k].label) + '</button>'; }).join('') + '</div>';
      };
      var eras = st.eras.map(function (e) {
        return '<details class="el-era" data-era="' + e.id + '"' + (e.id === self.openEra ? ' open' : '') + '>' +
          '<summary><i class="el-sw"></i><span class="nm">' + esc(e.name) + '<small></small></span>' + (e.secret ? '<span class="el-eye" title="Hidden from players until it begins" aria-label="Hidden from players until it begins">' + EYE + '</span>' : '') + '</summary>' +
          '<div class="el-inner">' +
            '<div class="el-mini">Colours</div>' +
            '<div class="el-pals">' + B.PALETTES.map(function (p) {
              return '<button type="button" class="el-pal" data-a="' + p[1] + '" data-b="' + p[2] + '" style="--g:linear-gradient(135deg,' + p[1] + ' 0 50%,' + p[2] + ' 50%)" aria-label="' + esc(p[0]) + ' palette" title="' + esc(p[0]) + '"></button>';
            }).join('') + '</div>' +
            '<div class="el-cols"><label>First <input type="color" data-k="color" aria-label="' + esc(e.name) + ' first colour"/></label>' +
              '<label>Second <input type="color" data-k="color_2" aria-label="' + esc(e.name) + ' second colour"/></label></div>' +
            '<div class="el-mini">Style</div>' +
            '<div class="el-seg" role="radiogroup" aria-label="' + esc(e.name) + ' style" data-seg="style">' +
              B.STYLES.map(function (s) { return '<button type="button" role="radio" data-k="' + s[0] + '">' + esc(s[1]) + '</button>'; }).join('') + '</div>' +
            '<div class="el-own"><span id="el-own-' + e.id + '">Its own feel<span class="el-hint">Overrides the calendar’s feel for this era only.</span></span>' +
              '<button type="button" class="el-switch" role="switch" aria-labelledby="el-own-' + e.id + '" data-own><span></span></button></div>' +
            presetSeg('own', e.name + ' feel') +
          '</div></details>';
      }).join('');
      this.el.innerHTML =
        '<div class="el-grid">' +
          '<div class="el-controls">' +
            '<section class="el-sec"><div class="el-row"><div><div class="el-label" id="el-show-l">Show era colours behind the days</div>' +
              '<div class="el-hint">When off, the days stay plain. Era names and the “begins” label still show.</div></div>' +
              '<button type="button" class="el-switch" role="switch" aria-labelledby="el-show-l" data-show><span></span></button></div></section>' +
            '<section class="el-sec" data-feel-sec><div class="el-label">Overall feel</div><div class="el-hint">For the whole calendar. Each era can have its own below.</div>' +
              presetSeg('preset', 'Overall feel') + '<p class="el-feelhint" data-feel-hint></p>' +
              '<details class="el-fine"><summary>Fine-tune</summary>' +
                '<label class="el-slider">Intensity <input type="range" data-fine="intensity" min="' + B.LIMITS.intensity[0] + '" max="' + B.LIMITS.intensity[1] + '" step="0.1"/> <output data-out="intensity"></output></label>' +
                '<label class="el-slider">Speed <input type="range" data-fine="speed" min="' + B.LIMITS.speed[0] + '" max="' + B.LIMITS.speed[1] + '" step="0.1"/> <output data-out="speed"></output></label>' +
              '</details></section>' +
            '<section class="el-sec"><div class="el-label">Eras</div>' +
              (st.eras.length ? '<div class="el-hint">Where two eras meet, the style of the era being entered shapes the mixing.</div>' + eras
                : '<div class="el-hint">This calendar has no eras yet. Add them on the calendar: Edit, then Eras.</div>') +
            '</section>' +
            '<p class="el-device"><i class="fa-solid fa-circle-info" aria-hidden="true"></i><span>Each viewer’s device still eases the motion to a stop when they stop using the page, uses a lighter version on a slower device, and shows still colours to anyone who has asked their device for less motion.</span></p>' +
          '</div>' +
          '<div class="el-previewcol"><div class="el-preview" aria-label="Preview">' +
            '<div class="el-ph"><span class="el-mini">Preview</span><select class="el-pairs" data-pairs aria-label="Which change of era to preview"></select></div>' +
            '<div class="el-month" data-month aria-hidden="true"></div>' +
            '<p class="el-hint" data-pnote></p>' +
          '</div></div>' +
        '</div>' +
        '<footer class="el-foot"><span class="el-dirty" data-dirty aria-live="polite"></span>' +
          '<button type="button" class="btn quiet" data-cancel>Cancel</button><button type="button" class="btn primary" data-save>Save</button></footer>';
      this._buildPreview();
    },

    // The preview month: 30 days from the third column, so a change of era
    // on day 18 wraps a row the way it would on the calendar.
    _buildPreview: function () {
      var m = this.el.querySelector('[data-month]'), html = '';
      for (var r = 0; r < 5; r++) {
        html += '<div class="el-wk">';
        for (var c = 0; c < 7; c++) { var d = r * 7 + c - 1; html += '<div class="el-day">' + (d >= 1 && d <= 30 ? d : '') + '</div>'; }
        html += '</div>';
      }
      m.innerHTML = html;
      var self = this;
      this.painter = this.B.create(m, { surface: function () { return getComputedStyle(m).backgroundColor; } });
      if ('ResizeObserver' in window) { this._ro = new ResizeObserver(function () { self._paint(); }); this._ro.observe(m); }
    },

    _pairs: function () {
      var eras = this.state.eras, out = [];
      for (var i = 0; i + 1 < eras.length; i++) out.push([eras[i], eras[i + 1]]);
      if (!out.length && eras.length) out.push([eras[0]]);
      return out;
    },

    _paint: function () {
      var m = this.el.querySelector('[data-month]'), pair = this._pairs()[this.pair] || [];
      var rows = Array.prototype.map.call(m.querySelectorAll('.el-wk'), function (r) { return { top: r.offsetTop, height: r.offsetHeight }; });
      this.painter.setLook(this.state.look);
      this.painter.setScene({
        box: { left: 0, top: 0, width: m.clientWidth, height: m.clientHeight },
        rows: rows, cols: 7, off: 2, days: 30, split: pair.length > 1 ? 18 : null,
        eras: pair.map(function (e) { return { key: e.id, color: e.color, color_2: e.color_2, style: e.style, feel: e.feel }; })
      });
      var note = this.el.querySelector('[data-pnote]');
      note.textContent = pair.length > 1 ? 'Days 1 to 17 are the ' + pair[0].name + '; the ' + pair[1].name + ' begins on day 18.' : pair.length ? 'The whole month is the ' + pair[0].name + '.' : 'Add an era to see its colours here.';
    },

    _era: function (id) { return this.state.eras.filter(function (e) { return String(e.id) === String(id); })[0]; },

    _bind: function () {
      var self = this, B = this.B, el = this.el;
      el.addEventListener('click', function (e) {
        var t = e.target, row = t.closest('.el-era'), era = row ? self._era(row.dataset.era) : null, look = self.state.look, k;
        if (t.closest('[data-show]')) { look.colors_on = !look.colors_on; }
        else if (t.closest('[data-seg="preset"] button')) {
          k = t.closest('button').dataset.k;
          look.feel = k; look.intensity = B.PRESETS[k].intensity; look.speed = B.PRESETS[k].speed;
        } else if (era && t.closest('.el-pal')) { var p = t.closest('.el-pal'); era.color = p.dataset.a; era.color_2 = p.dataset.b; }
        else if (era && t.closest('[data-seg="style"] button')) { era.style = t.closest('button').dataset.k; }
        else if (era && t.closest('[data-own]')) { era.feel = era.feel ? null : (B.PRESETS[look.feel] ? look.feel : 'subtle'); }
        else if (era && t.closest('[data-seg="own"] button')) { era.feel = t.closest('button').dataset.k; }
        else if (t.closest('[data-cancel]')) { self._cancel(); return; }
        else if (t.closest('[data-save]')) { self._save(); return; }
        else return;
        self._sync();
      });
      el.addEventListener('input', function (e) {
        var t = e.target, row = t.closest('.el-era'), look = self.state.look;
        if (t.dataset.fine) {
          // Moving a slider off a preset's numbers makes the feel Custom.
          look[t.dataset.fine] = +t.value;
          look.feel = B.matchPreset(look.intensity, look.speed);
        } else if (row && t.type === 'color') {
          self._era(row.dataset.era)[t.dataset.k] = t.value;
        } else return;
        self._sync();
      });
      el.addEventListener('change', function (e) {
        if (e.target.matches('[data-pairs]')) { self.pair = +e.target.value; self._paint(); }
      });
      // Opening an era's row previews the change into that era.
      el.addEventListener('toggle', function (e) {
        var row = e.target;
        if (!row.matches || !row.matches('.el-era') || !row.open) return;
        var idx = self.state.eras.findIndex(function (x) { return String(x.id) === row.dataset.era; });
        self.pair = Math.max(0, Math.min(self._pairs().length - 1, idx - 1));
        self.el.querySelector('[data-pairs]').value = String(self.pair);
        self._paint();
      }, true);
      // Leaving with unsaved changes asks first.
      this._beforeUnload = function (e) { if (snapshotOf(self.state) !== snapshotOf(self.saved)) { e.preventDefault(); e.returnValue = ''; } };
      window.addEventListener('beforeunload', this._beforeUnload);
    },

    // _sync pushes the state into every control and the preview; it runs
    // after any change, including Cancel.
    _sync: function () {
      var B = this.B, st = this.state, look = st.look, el = this.el;
      var setSeg = function (host, cur) { if (host) host.querySelectorAll('button').forEach(function (b) { b.setAttribute('aria-checked', String(b.dataset.k === cur)); }); };
      el.querySelector('[data-show]').setAttribute('aria-checked', String(look.colors_on));
      el.querySelector('[data-feel-sec]').classList.toggle('el-dim', !look.colors_on);
      setSeg(el.querySelector('[data-seg="preset"]'), look.feel);
      el.querySelector('[data-feel-hint]').textContent = look.feel === 'custom' ? 'Custom: set with Fine-tune below.' : B.PRESETS[look.feel].hint;
      ['intensity', 'speed'].forEach(function (k) {
        var inp = el.querySelector('[data-fine="' + k + '"]');
        if (document.activeElement !== inp) inp.value = look[k];
        el.querySelector('[data-out="' + k + '"]').textContent = k === 'intensity' ? B.wordI(look[k]) : B.wordS(look[k]);
      });
      st.eras.forEach(function (e) {
        var row = el.querySelector('.el-era[data-era="' + e.id + '"]');
        if (!row) return;
        row.querySelector('summary .el-sw').style.background = grad(e);
        row.querySelector('summary small').textContent = (e.style === 'ink' ? 'Ink in water' : 'Gas mixing') + ' · ' + (e.feel ? B.PRESETS[e.feel].label : 'calendar feel');
        row.querySelectorAll('.el-pal').forEach(function (p) { p.setAttribute('aria-pressed', String(p.dataset.a === e.color && p.dataset.b === e.color_2)); });
        row.querySelectorAll('input[type=color]').forEach(function (inp) { if (document.activeElement !== inp) inp.value = inp.dataset.k === 'color_2' ? (e.color_2 || e.color) : e.color; });
        setSeg(row.querySelector('[data-seg="style"]'), e.style);
        row.querySelector('[data-own]').setAttribute('aria-checked', String(!!e.feel));
        var own = row.querySelector('[data-seg="own"]');
        own.hidden = !e.feel;
        setSeg(own, e.feel);
        row.querySelector('.el-inner').classList.toggle('el-dim', !look.colors_on);
      });
      var sel = el.querySelector('[data-pairs]'), pairs = this._pairs();
      sel.innerHTML = pairs.map(function (p, i) { return '<option value="' + i + '">' + esc(p.map(function (e) { return e.name; }).join(' → ')) + '</option>'; }).join('');
      sel.hidden = pairs.length < 2;
      if (this.pair >= pairs.length) this.pair = 0;
      sel.value = String(this.pair);
      var dirty = snapshotOf(st) !== snapshotOf(this.saved);
      el.querySelector('[data-dirty]').textContent = this._status || (dirty ? 'Unsaved changes' : '');
      this._status = '';
      el.querySelector('[data-save]').disabled = !dirty || this._saving;
      el.querySelector('[data-cancel]').disabled = !dirty || this._saving;
      this._paint();
      if (this.painter) this.painter.wake();
    },

    _cancel: function () {
      this.state = clone(this.saved);
      this._status = 'Changes discarded.';
      this._sync();
    },

    _save: function () {
      var self = this, st = this.state;
      var body = { colors_on: st.look.colors_on, feel: st.look.feel, intensity: st.look.intensity, speed: st.look.speed, eras: changedEras(this.saved.eras, st.eras) };
      this._saving = true;
      this._status = 'Saving…';
      this._sync();
      Chronicle.apiFetch(this.endpoint, { method: 'PUT', body: body })
        .then(function (resp) {
          if (resp.ok) return null;
          return resp.json().catch(function () { return {}; }).then(function (j) { return (j && j.message) || 'The era look could not be saved.'; });
        })
        .catch(function () { return 'The era look could not be saved. Check your connection and try again.'; })
        .then(function (err) {
          self._saving = false;
          if (!err) self.saved = clone(self.state);
          self._status = err || 'Era look saved.';
          self._sync();
        });
    }
  };

  if (typeof window !== 'undefined' && window.Chronicle && Chronicle.register) Chronicle.register('calendar_era_look', EraLook);
  if (typeof module !== 'undefined' && module.exports) module.exports = { changedEras: changedEras, snapshotOf: snapshotOf };
})();
