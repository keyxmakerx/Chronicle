/**
 * calendar_sky_rail.js: the sky's hand controls on the calendar's own page.
 *
 * For the owner the header's Moons button opens a menu of the sun and each
 * moon that slides down from under the header's button row and floats over
 * the calendar, riding up and down with the docked sky (SkyPane.Dock in
 * sky_pane.js) as it folds and opens. Pressing a body in the menu, or the body itself in
 * the sky, lifts a tray with that body's look: the sun's kind of star and
 * colour; a moon's colour, how strongly the sky shows it, and whether
 * players see it. Each pick saves at once through PUT .../sun,
 * .../moons/:id/look and .../moons/:id/hidden, and the sky redraws from the
 * saved values. The menu's last entry opens the moon phases the button
 * opens for everyone else. Seeing it as a player is the editor deck's job.
 *
 * calendar_view.js loads this file on demand for the campaign owner only
 * (the look routes are Owner only), so other viewers never download it.
 * It registers Chronicle.calendarSkyRail.mount(view) and touches the view
 * only through its sky (skyDock, _skyDay), its calendar (cal, apiBase) and
 * its words (say, _announce).
 */
(function () {
  'use strict';

  var KINDS = [['red', 'Red ember'], ['orange', 'Orange'], ['yellow', 'Yellow'], ['white', 'White'], ['blue', 'Blue-white']];
  // The sky's own default strength (SkyMoon.specFor) when a moon has none.
  var DEFAULT_STRENGTH = 0.4;
  var HEX6 = /^#[0-9a-fA-F]{6}$/;

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; });
  }
  function kindColor(k) {
    var kd = window.SkyWorld && SkyWorld.SUN_KINDS && SkyWorld.SUN_KINDS[k];
    return kd ? kd[0] : '#fff1c9';
  }
  function kindName(k) {
    for (var i = 0; i < KINDS.length; i++) if (KINDS[i][0] === k) return KINDS[i][1];
    return 'Yellow';
  }
  function strengthOf(m) {
    var t = m.tint != null && m.tint !== '' ? Number(m.tint) : DEFAULT_STRENGTH;
    return t >= 0 && t <= 1 ? t : DEFAULT_STRENGTH;
  }
  function sunOf(cal) { var s = cal.sun || {}; return { kind: s.kind || 'yellow', color: HEX6.test(s.color || '') ? s.color : null }; }
  // A colour input takes only #rrggbb; a stored #rgb is widened for it.
  function hex6(c) {
    if (HEX6.test(c || '')) return c.toLowerCase();
    if (/^#[0-9a-fA-F]{3}$/.test(c || '')) return ('#' + c[1] + c[1] + c[2] + c[2] + c[3] + c[3]).toLowerCase();
    return '#cccccc';
  }
  function pct(v) { return Math.round(v * 100) + '%'; }

  function Rail(view) {
    var self = this;
    this.view = view;
    this.rail = view.el.querySelector('#cal5-skyrail');
    this.tray = view.el.querySelector('#cal5-skytray');
    this.btn = view.el.querySelector('#cal5-moonbtn');
    this.sky = view.el.querySelector('#cal5-skywrap');
    this.sel = null;
    this.shown = false;
    this.flashT = 0;
    if (!this.rail || !this.tray || !this.btn || !view.skyDock) throw new Error('sky rail: no sky to edit');
    this._click = function (e) { self.onClick(e); };
    this._input = function (e) { self.onInput(e); };
    this._change = function (e) { self.onChange(e); };
    this._key = function (e) { self.onKey(e); };
    this._sky = function (e) { self.onSky(e); };
    this._hover = function (e) { self.onHover(e); };
    // Pressing anywhere else on the page puts the menu away, as the calendar's
    // own panels do; folding or opening the sky carries it along instead.
    this._outside = function (e) {
      var t = e.target;
      if (!self.shown || self.tray.contains(t) || self.rail.contains(t) || self.btn.contains(t) || t === self.canvas || (t.closest && t.closest('#cal5-skybtn'))) return;
      self.hide();
    };
    // The menu drops from under the Moons button, whose row sits on the sky's
    // lower edge, so it rides up and down as the sky folds and opens.
    this._place = function () {
      self.rail.style.top = Math.round(self.btn.getBoundingClientRect().bottom - view.calEl.getBoundingClientRect().top + 6) + 'px';
    };
    if (window.ResizeObserver && this.sky) { this._ro = new ResizeObserver(this._place); this._ro.observe(this.sky); }
    document.addEventListener('pointerdown', this._outside, true);
    [this.rail, this.tray].forEach(function (el) {
      el.addEventListener('click', self._click);
      el.addEventListener('input', self._input);
      el.addEventListener('change', self._change);
      el.addEventListener('keydown', self._key);
    });
    this.canvas = view.skyDock.canvas;
    this.canvas.addEventListener('click', this._sky);
    this.canvas.addEventListener('mousemove', this._hover);
    this.rail.innerHTML = '<div class="srin"></div>';
    this.inner = this.rail.firstChild;
    this.btn.setAttribute('aria-controls', 'cal5-skyrail');
    this.btn.setAttribute('aria-haspopup', 'true');
    this.rail.hidden = false;
    this._place();
    this.render();
  }

  Rail.prototype.moon = function (id) {
    return (this.view.cal.moons || []).filter(function (m) { return String(m.id) === String(id); })[0] || null;
  };
  Rail.prototype.bodies = function () {
    return ['sun'].concat((this.view.cal.moons || []).map(function (m) { return String(m.id); }));
  };
  Rail.prototype.nameOf = function (b) { return b === 'sun' ? 'The sun' : (this.moon(b) || {}).name || 'Moon'; };
  Rail.prototype.colorOf = function (b) {
    if (b === 'sun') { var s = sunOf(this.view.cal); return s.color || kindColor(s.kind); }
    return hex6((this.moon(b) || {}).color);
  };

  // The menu: one chip per body, its ring filled as far as the moon's colour
  // strength, an amber edge on a moon players can't see.
  Rail.prototype.render = function () {
    var self = this, up = this.view.skyDock.bodiesUp(), list = this.bodies();
    var chips = list.map(function (b, i) {
      var m = b === 'sun' ? null : self.moon(b), on = self.sel === b;
      var p = m ? Math.round(strengthOf(m) * 100) : 100;
      var tab = self.sel ? on : i === 0;
      return '<button type="button" class="src" role="option" id="cal5-sr-' + esc(b) + '" data-sr="' + esc(b) + '" aria-selected="' + on + '" tabindex="' + (tab ? 0 : -1) + '"' +
        (m && m.hidden_from_players ? ' data-dm-only' : '') + '>' +
        '<span class="srr" style="--c:' + self.colorOf(b) + ';--p:' + p + '" aria-hidden="true"><i></i></span>' +
        '<span class="srt"><span class="srn">' + esc(self.nameOf(b)) + '</span>' +
        '<small>' + (up[b === 'sun' ? 'sun' : m.id] ? 'In the sky' : 'Below the horizon') + '</small></span></button>';
    }).join('');
    this.inner.innerHTML = '<div class="srl" role="listbox" aria-orientation="horizontal" aria-label="The sun and moons">' + chips + '</div>' +
      '<button type="button" class="srp" data-sr-phases><i class="fa-solid fa-moon" aria-hidden="true"></i><span>Moon phases</span></button>';
  };

  Rail.prototype.sunFields = function () {
    var s = sunOf(this.view.cal);
    var kinds = KINDS.map(function (k) {
      return '<button type="button" class="stc" role="radio" data-sr-kind="' + k[0] + '" aria-checked="' + (s.kind === k[0]) + '">' +
        '<span class="std" style="--c:' + kindColor(k[0]) + '" aria-hidden="true"></span>' + k[1] + '</button>';
    }).join('');
    // Black and white-rimmed suns are planned (#1659); shown so the row's shape is settled.
    var styles = '<button type="button" class="stc" role="radio" aria-checked="true"><span class="std" style="--c:' + (s.color || kindColor(s.kind)) + '" aria-hidden="true"></span>Glow</button>' +
      '<button type="button" class="stc" role="radio" aria-checked="false" disabled><span class="std black" aria-hidden="true"></span>Black<small>Planned</small></button>' +
      '<button type="button" class="stc" role="radio" aria-checked="false" disabled><span class="std rim" style="--c:' + kindColor(s.kind) + '" aria-hidden="true"></span>White rim<small>Planned</small></button>';
    var exact = s.color
      ? '<div class="stline"><input type="color" data-sr-suncolor value="' + s.color + '" aria-label="The sun’s exact colour"><code>' + s.color + '</code>' +
        '<button type="button" class="stq" data-sr-exact="off">Use the kind’s own colour</button></div>'
      : '<button type="button" class="stq" data-sr-exact="on">Pick an exact colour instead</button>';
    return '<div class="stf"><span class="stl">Kind of star</span><div class="stcs" role="radiogroup" aria-label="Kind of star">' + kinds + '</div></div>' +
      '<div class="stf"><span class="stl">Style</span><div class="stcs" role="radiogroup" aria-label="Sun style">' + styles + '</div></div>' +
      '<div class="stf"><span class="stl">Colour</span>' + exact + '</div>';
  };
  Rail.prototype.moonFields = function (m) {
    var v = strengthOf(m), id = esc(m.id);
    return '<div class="stf"><span class="stl">Colour</span><div class="stline"><input type="color" data-sr-color value="' + hex6(m.color) + '" aria-label="' + esc(m.name) + ' colour"><code>' + hex6(m.color) + '</code></div></div>' +
      '<div class="stf"><label class="stl" for="cal5-srs-' + id + '">Colour strength</label><div class="stline"><input type="range" id="cal5-srs-' + id + '" data-sr-strength min="0" max="1" step="0.05" value="' + v + '"><output>' + pct(v) + '</output></div></div>' +
      '<div class="stf"><button type="button" class="stsw" role="switch" data-sr-hidden aria-checked="' + !m.hidden_from_players + '">' +
      '<span><b>Players can see this moon</b><small>' + (m.hidden_from_players ? 'Only you see it. It carries the amber edge.' : 'It shows in every player’s sky.') + '</small></span><span class="sttrack" aria-hidden="true"></span></button></div>';
  };

  // The tray lifts over the calendar from the chosen chip; changing body
  // swaps its contents in place rather than lifting it again.
  Rail.prototype.fillTray = function () {
    var b = this.sel, m = b === 'sun' ? null : this.moon(b);
    if (!b || (b !== 'sun' && !m)) return false;
    this.tray.setAttribute('aria-label', this.nameOf(b));
    if (m && m.hidden_from_players) this.tray.setAttribute('data-dm-only', ''); else this.tray.removeAttribute('data-dm-only');
    this.tray.innerHTML = '<div class="sth"><span class="srr" style="--c:' + this.colorOf(b) + ';--p:' + (m ? Math.round(strengthOf(m) * 100) : 100) + '" aria-hidden="true"><i></i></span>' +
      '<b>' + esc(this.nameOf(b)) + '</b><span class="sts" role="status" aria-live="polite"></span>' +
      '<button type="button" class="ib" data-sr-close aria-label="Close"><i class="fa-solid fa-xmark"></i></button></div>' +
      '<div class="stb">' + (m ? this.moonFields(m) : this.sunFields()) + '</div>';
    return true;
  };
  Rail.prototype.select = function (b, focusChip) {
    if (this.view._players) return;
    if (!this.shown) this.show(false);
    var was = this.sel;
    this.sel = b;
    this.render();
    var chip = this.chip(b);
    if (!this.fillTray()) { this.close(); return; }
    if (!was) Chronicle.calendarPanel.growOpen(this.tray, chip, this.view.calEl);
    this.view.skyDock.redraw();
    if (focusChip && chip) chip.focus({ preventScroll: true });
  };
  Rail.prototype.close = function (instant) {
    var b = this.sel;
    if (!b) return;
    this.sel = null;
    this.render();
    this.view.skyDock.redraw();
    if (instant) { this.tray.classList.remove('open', 'live'); this.tray.removeAttribute('style'); this.tray.innerHTML = ''; return; }
    Chronicle.calendarPanel.growClose(this.tray, this.chip(b));
  };
  Rail.prototype.chip = function (b) { return b ? this.rail.querySelector('[data-sr="' + b + '"]') : null; };

  // The menu slides down out of the sky's edge; focusChip moves the keyboard
  // into it when the button was pressed from the keyboard.
  Rail.prototype.show = function (focusChip) {
    if (this.shown || this.view._players) return;
    this.view.closeMoonView();
    this.shown = true;
    this._place();
    this.render();
    this.rail.classList.remove('instant');
    this.rail.classList.add('open');
    this.btn.setAttribute('aria-expanded', 'true');
    if (focusChip) { var c = this.rail.querySelector('.src[tabindex="0"]'); if (c) c.focus({ preventScroll: true }); }
  };
  // instant drops it without the slide (the page is changing under it).
  Rail.prototype.hide = function (instant, focusBtn) {
    if (!this.shown) return;
    this.close(true);
    this.shown = false;
    this.rail.classList.toggle('instant', !!instant);
    this.rail.classList.remove('open');
    this.btn.setAttribute('aria-expanded', 'false');
    if (focusBtn) this.btn.focus({ preventScroll: true });
  };
  Rail.prototype.toggle = function (focusChip) { if (this.shown) this.hide(false, focusChip); else this.show(focusChip); };

  // The sky redraws from the calendar as it now stands; the rail's rings follow.
  Rail.prototype.redrawSky = function () {
    var v = this.view;
    v._skySig = null;
    v._skyDay();
    var r = this.chip(this.sel), b = this.sel;
    if (r && b) {
      var m = b === 'sun' ? null : this.moon(b), ring = r.querySelector('.srr'), head = this.tray.querySelector('.sth .srr');
      [ring, head].forEach(function (el) {
        if (!el) return;
        el.style.setProperty('--c', this.colorOf(b));
        el.style.setProperty('--p', m ? Math.round(strengthOf(m) * 100) : 100);
      }, this);
    }
  };
  Rail.prototype.flash = function (text) {
    var s = this.tray.querySelector('.sts');
    if (!s) return;
    s.innerHTML = '<i class="fa-solid fa-check"></i>' + esc(text);
    clearTimeout(this.flashT);
    this.flashT = setTimeout(function () { s.textContent = ''; }, 1600);
  };
  // put sends one change; on success apply() keeps it, on failure undo()
  // puts back what was there and the calendar says why.
  Rail.prototype.put = function (tail, body, apply, undo) {
    var self = this, v = this.view;
    return Chronicle.apiFetch(v.apiBase + tail, { method: 'PUT', body: body }).then(function (resp) {
      if (resp.ok) { apply(); self.redrawSky(); self.flash('Saved'); return; }
      return resp.json().catch(function () { return {}; }).then(function (b) {
        undo(); self.redrawSky(); self.refill();
        v.say(b && b.message ? b.message : 'That didn’t save. Nothing changed.');
      });
    }).catch(function () { undo(); self.redrawSky(); self.refill(); v.say('Could not reach the server. Nothing changed.'); });
  };
  Rail.prototype.refill = function () { if (this.sel) { this.render(); this.fillTray(); } };

  Rail.prototype.saveSun = function (next, focusSel) {
    var cal = this.view.cal, before = cal.sun ? { kind: cal.sun.kind, color: cal.sun.color } : null;
    cal.sun = { kind: next.kind, color: next.color || undefined };
    this.refill();
    if (focusSel) { var f = this.tray.querySelector(focusSel); if (f) f.focus({ preventScroll: true }); }
    this.put('/sun', { kind: next.kind, color: next.color || null }, function () {}, function () { cal.sun = before; });
  };
  // The moon's colour and strength go together, as the route takes them.
  Rail.prototype.saveMoonLook = function (m, saved) {
    var body = { color: hex6(m.color), strength: m.tint != null && m.tint !== '' ? strengthOf(m) : null };
    this.put('/moons/' + encodeURIComponent(m.id) + '/look', body,
      function () { saved.color = m.color; saved.tint = m.tint; },
      function () { m.color = saved.color; m.tint = saved.tint; });
  };

  Rail.prototype.onClick = function (e) {
    var t, v = this.view;
    if (e.target.closest('[data-sr-phases]')) { this.hide(true); v.openMoonView(); return; }
    if ((t = e.target.closest('[data-sr]'))) {
      if (this.sel === t.dataset.sr) this.close(); else this.select(t.dataset.sr, true);
      return;
    }
    if (e.target.closest('[data-sr-close]')) { this.close(); return; }
    var sun = sunOf(v.cal);
    if ((t = e.target.closest('[data-sr-kind]'))) {
      if (t.getAttribute('aria-checked') === 'true') return;
      this.saveSun({ kind: t.dataset.srKind, color: sun.color }, '[data-sr-kind="' + t.dataset.srKind + '"]');
      return;
    }
    if ((t = e.target.closest('[data-sr-exact]'))) {
      var on = t.dataset.srExact === 'on';
      this.saveSun({ kind: sun.kind, color: on ? kindColor(sun.kind) : null }, on ? '[data-sr-suncolor]' : '[data-sr-exact]');
      return;
    }
    if ((t = e.target.closest('[data-sr-hidden]'))) {
      var m = this.moon(this.sel);
      if (!m) return;
      var hidden = !m.hidden_from_players;
      m.hidden_from_players = hidden;
      this.refill();
      var sw = this.tray.querySelector('[data-sr-hidden]');
      if (sw) sw.focus({ preventScroll: true });
      this.put('/moons/' + encodeURIComponent(m.id) + '/hidden', { hidden: hidden }, function () {
        if (v._refreshMv && v.mvEl && v.mvEl.classList.contains('open')) v._refreshMv();
        v._announce(m.name + (hidden ? ' hidden from players.' : ' shown to players.'));
      }, function () { m.hidden_from_players = !hidden; });
    }
  };
  // Dragging and picking redraw the sky at once; the change saves when let go.
  Rail.prototype.onInput = function (e) {
    var t = e.target, m = this.moon(this.sel), row = t.closest('.stline');
    if (t.matches('[data-sr-color]') && m) {
      if (!this._before) this._before = { color: m.color, tint: m.tint };
      m.color = t.value;
    } else if (t.matches('[data-sr-strength]') && m) {
      if (!this._before) this._before = { color: m.color, tint: m.tint };
      m.tint = String(Number(t.value));
    } else if (t.matches('[data-sr-suncolor]')) {
      this.view.cal.sun = { kind: sunOf(this.view.cal).kind, color: t.value };
    } else return;
    var out = row && row.querySelector('output, code');
    if (out) out.textContent = t.type === 'range' ? pct(Number(t.value)) : t.value;
    this.redrawSky();
  };
  Rail.prototype.onChange = function (e) {
    var t = e.target, m = this.moon(this.sel);
    if ((t.matches('[data-sr-color]') || t.matches('[data-sr-strength]')) && m) {
      var saved = this._before || { color: m.color, tint: m.tint };
      this._before = null;
      this.saveMoonLook(m, saved);
    } else if (t.matches('[data-sr-suncolor]')) {
      this.saveSun({ kind: sunOf(this.view.cal).kind, color: t.value }, null);
    }
  };
  Rail.prototype.onKey = function (e) {
    if (e.key === 'Escape' && this.sel) { e.stopPropagation(); var b = this.sel; this.close(); var c = this.chip(b); if (c) c.focus({ preventScroll: true }); return; }
    if (e.key === 'Escape' && this.shown) { e.stopPropagation(); this.hide(false, true); return; }
    var chip = e.target.closest && e.target.closest('.src');
    if (!chip || ['ArrowRight', 'ArrowLeft', 'Home', 'End'].indexOf(e.key) < 0) return;
    e.preventDefault();
    var list = this.bodies(), i = list.indexOf(chip.dataset.sr);
    i = e.key === 'Home' ? 0 : e.key === 'End' ? list.length - 1 : (i + (e.key === 'ArrowRight' ? 1 : -1) + list.length) % list.length;
    if (this.sel) this.select(list[i], true);
    else { var n = this.chip(list[i]); if (n) { this.rail.querySelectorAll('.src').forEach(function (c) { c.tabIndex = c === n ? 0 : -1; }); n.focus(); } }
  };

  // A body clicked in the sky is chosen as if from the rail.
  Rail.prototype.at = function (e) {
    var r = this.canvas.getBoundingClientRect();
    return this.view.skyDock.bodyAt(e.clientX - r.left, e.clientY - r.top);
  };
  Rail.prototype.onSky = function (e) {
    if (this.view._players) return;
    var b = this.at(e);
    if (b == null) return;
    b = String(b);
    if (this.sel === b) this.close(); else this.select(b, true);
  };
  Rail.prototype.onHover = function (e) {
    this.canvas.style.cursor = !this.view._players && this.at(e) != null ? 'pointer' : '';
  };
  // A ring round the chosen body, drawn over each frame of the sky.
  Rail.prototype.overlay = function (ctx, st, d, y) {
    var b = this.sel, at = null;
    if (!b) return;
    if (b === 'sun') { if (st.alt > -.02) at = { x: st.sp.x, y: st.sp.y, r: Math.max(16, st.R * 1.5) }; }
    else st.moons.forEach(function (M) { if (M.up && String(M.mo.id) === b) at = { x: M.x, y: M.y, r: Math.max(13, M.r * 1.25) }; });
    if (!at) return;
    ctx.save();
    ctx.setTransform(d, 0, 0, d, 0, y * d);
    ctx.beginPath(); ctx.arc(at.x, at.y, at.r + 5, 0, Math.PI * 2);
    ctx.lineWidth = 3.5; ctx.strokeStyle = 'rgba(10,14,30,.45)'; ctx.stroke();
    ctx.lineWidth = 1.75; ctx.strokeStyle = 'rgba(255,255,255,.92)'; ctx.stroke();
    ctx.restore();
  };
  // The sky folding or opening puts the tray away; the menu rides with the sky's edge.
  Rail.prototype.folding = function () { this.close(true); };
  Rail.prototype.destroy = function () {
    var self = this;
    clearTimeout(this.flashT);
    [this.rail, this.tray].forEach(function (el) {
      el.removeEventListener('click', self._click);
      el.removeEventListener('input', self._input);
      el.removeEventListener('change', self._change);
      el.removeEventListener('keydown', self._key);
    });
    this.canvas.removeEventListener('click', this._sky);
    this.canvas.removeEventListener('mousemove', this._hover);
    document.removeEventListener('pointerdown', this._outside, true);
    if (this._ro) this._ro.disconnect();
  };

  window.Chronicle = window.Chronicle || {};
  Chronicle.calendarSkyRail = { mount: function (view) { return new Rail(view); } };
})();
