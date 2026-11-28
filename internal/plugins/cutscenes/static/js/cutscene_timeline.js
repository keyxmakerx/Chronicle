/**
 * Cut scene timeline
 *
 * The bar view above the editor's rows (internal/plugins/cutscenes/
 * editor_page.templ). The rows stay the scene: every number and choice the
 * timeline shows is read from a row's own field, and every drag or pick is
 * written back into that field, so Save, Preview and an AI-written scene all
 * go through the same form and the timeline never holds a copy of its own.
 *
 * Each layer is a bar: it starts at its row's start and is drawn as long as
 * the engine shows it (static/js/cutscene_engine.js: words fade in over
 * TEXT_IN and leave over TEXT_OUT after their time on screen; the background
 * and the layers on top stay to the end). A bar's faded start is how it
 * arrives and its faded end how it leaves; pressing one opens a short menu of
 * that row's own choices. Dragging a bar moves its start, dragging its right
 * end stretches its time, and dragging the end of a faded start stretches how
 * long it takes to build. The bar itself is a slider for the keyboard.
 *
 * The workshop mounts it (cutscene_workshop.js); without script the
 * timeline stays hidden and the rows do everything.
 */
(function () {
  'use strict';

  // The engine's own text timings, so a line's bar is as long as it shows.
  var TEXT_IN = 1.4;
  var TEXT_OUT = 1.1;
  var STEP = 0.1;
  var DRAG_PX = 4;

  // The short word drawn in a bar's faded end. A row's choice that isn't
  // here shows its own value.
  var TAGS = {
    bg_in: { cut: 'cut', fade: 'fade', 'fall-asleep': 'asleep' },
    ov_kind: { eye: 'opens', embers: 'rise', letterbox: 'slide' },
    tx_in: { '': 'own', fade: 'fade', rise: 'rise', settle: 'settle', slide: 'slide' },
    tx_out: { '': 'blur', fade: 'fade', sink: 'sink', slide: 'slide' }
  };

  function num(input) {
    var v = parseFloat(input && input.value);
    return isFinite(v) ? v : 0;
  }

  function limit(input, key, def) {
    var v = parseFloat(input && input.getAttribute(key));
    return isFinite(v) ? v : def;
  }

  function round(v) { return Math.round(v / STEP) * STEP; }

  function secs(v) { return (Math.round(v * 10) / 10) + ' s'; }

  function optionText(sel) {
    var o = sel && sel.options[sel.selectedIndex];
    return o ? o.textContent.trim() : '';
  }

  function tag(sel) {
    if (!sel) return '';
    var t = TAGS[sel.name] || {};
    return Object.prototype.hasOwnProperty.call(t, sel.value) ? t[sel.value] : sel.value;
  }

  function q(root, sel) { return root ? root.querySelector(sel) : null; }

  // read turns the form's rows into bars. end is Infinity for a layer that
  // stays to the end; fadeIn and fadeOut are seconds at each end.
  function read(form) {
    var bars = [];
    var kind = q(form, '#csw-bg-kind');
    if (kind && kind.value !== 'none') {
      var bgIn = q(form, '#csw-bg-in');
      var bgSec = q(form, '#csw-bg-sec');
      bars.push({
        label: 'Background', title: 'Background: ' + optionText(kind) + ', ' + optionText(bgIn).toLowerCase(),
        start: 0, end: Infinity, fadeIn: bgIn && bgIn.value === 'cut' ? 0 : num(bgSec), fadeOut: 0,
        inSel: bgIn, buildInput: bgIn && bgIn.value === 'cut' ? null : bgSec, fixed: true
      });
    }
    var ovs = form.querySelectorAll('#csw-ov-list [data-csw-row]');
    for (var i = 0; i < ovs.length; i++) {
      var ok = q(ovs[i], '[name="ov_kind"]');
      bars.push({
        label: optionText(ok), title: 'On top ' + (i + 1) + ': ' + optionText(ok),
        start: num(q(ovs[i], '[name="ov_at"]')), end: Infinity, fadeIn: num(q(ovs[i], '[name="ov_seconds"]')), fadeOut: 0,
        tagIn: tag(ok), atInput: q(ovs[i], '[name="ov_at"]'), buildInput: q(ovs[i], '[name="ov_seconds"]')
      });
    }
    var txs = form.querySelectorAll('#csw-tx-list [data-csw-row]');
    for (var j = 0; j < txs.length; j++) {
      var say = (q(txs[j], '[name="tx_say"]') || {}).value || '';
      var at = num(q(txs[j], '[name="tx_at"]'));
      var len = num(q(txs[j], '[name="tx_seconds"]'));
      var name = say.trim() || 'Words ' + (j + 1);
      bars.push({
        label: name, title: 'Words ' + (j + 1) + ': ' + name,
        start: at, end: len > 0 ? at + len + TEXT_OUT : Infinity, fadeIn: len > 0 ? Math.min(TEXT_IN, len) : TEXT_IN, fadeOut: len > 0 ? TEXT_OUT : 0,
        inSel: q(txs[j], '[name="tx_in"]'), outSel: len > 0 ? q(txs[j], '[name="tx_out"]') : null,
        atInput: q(txs[j], '[name="tx_at"]'), lenInput: q(txs[j], '[name="tx_seconds"]'), shown: len
      });
    }
    // A moment is a point, not a span: the scene holds there, so it is drawn
    // as a marker in a lane of its own, named by its question.
    var mos = form.querySelectorAll('#csw-mo-list [data-csw-row]');
    for (var k = 0; k < mos.length; k++) {
      var ask = ((q(mos[k], '[name="mo_ask"]') || {}).value || '').trim() || 'Moment ' + (k + 1);
      var moAt = q(mos[k], '[name="mo_at"]');
      bars.push({
        label: ask, title: 'Moment ' + (k + 1) + ': ' + ask, mark: true,
        start: num(moAt), end: Infinity, fadeIn: 0, fadeOut: 0, atInput: moAt
      });
    }
    return bars;
  }

  // scale is the timeline's length and tick: the last moment anything
  // changes plus a second of room, never under six seconds, rounded up to
  // the tick that wastes least room and gives three to six ticks.
  function scale(bars) {
    var last = 0;
    bars.forEach(function (b) {
      last = Math.max(last, b.start + b.fadeIn, b.end === Infinity ? 0 : b.end);
    });
    var need = Math.max(6, Math.ceil(last + 1));
    var best = null;
    [1, 2, 3, 5, 10, 15, 30, 60].forEach(function (step) {
      var total = Math.ceil(need / step) * step;
      var ticks = total / step;
      if (ticks > 6) return;
      var fit = ticks >= 3 ? 0 : 1;
      if (!best || total < best.total || (total === best.total && fit <= best.fit)) best = { total: total, step: step, fit: fit };
    });
    return best || { total: Math.ceil(need / 60) * 60, step: Math.ceil(need / 360) * 60 };
  }

  function el(tagName, cls, text) {
    var e = document.createElement(tagName);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  // Timeline is one mount's timeline: it finds the current form each time
  // it draws, so a form an HTMX save swapped in is picked up.
  function Timeline(mount) {
    this.mount = mount;
    this.form = null;
    this.box = null;
    this.total = 6;
    this.drag = null;
    this.menu = null;
    var self = this;
    this.onInput = function (e) {
      if (self.drag || !e.target.closest('[data-csw-form]')) return;
      self.draw();
    };
    this.onDocDown = function (e) {
      if (self.menu && !self.menu.el.contains(e.target)) self.closeMenu(false);
    };
    this.rows = new MutationObserver(function () { self.draw(); });
    this.swaps = new MutationObserver(function () { self.attach(); });
    mount.addEventListener('input', this.onInput);
    mount.addEventListener('change', this.onInput);
    document.addEventListener('pointerdown', this.onDocDown, true);
    this.swaps.observe(mount, { childList: true });
    this.attach();
  }

  Timeline.prototype.attach = function () {
    var form = q(this.mount, 'form[data-csw-form]');
    if (form === this.form && this.box && this.box.isConnected) return;
    this.closeMenu(false);
    this.form = form;
    this.box = q(form, '[data-csw-timeline]');
    this.rows.disconnect();
    if (!form || !this.box) return;
    ['#csw-ov-list', '#csw-tx-list', '#csw-mo-list'].forEach(function (s) {
      var list = q(form, s);
      if (list) this.rows.observe(list, { childList: true });
    }, this);
    this.box.hidden = false;
    this.draw();
  };

  Timeline.prototype.draw = function () {
    if (!this.form || !this.box) return;
    var bars = read(this.form);
    var sc = scale(bars);
    this.total = sc.total;
    var lanes = q(this.box, '[data-csw-tl-lanes]');
    var axis = q(this.box, '[data-csw-tl-axis]');
    lanes.textContent = '';
    axis.textContent = '';
    var empty = q(this.box, '[data-csw-tl-empty]');
    if (empty) empty.hidden = bars.length > 0;
    for (var i = 0; i < bars.length; i++) lanes.appendChild(this.lane(bars[i], i));
    var drawn = lanes.querySelectorAll('.csw-tl-bar');
    for (var j = 0; j < drawn.length; j++) fit(drawn[j]);
    for (var t = 0; t <= this.total + 1e-9; t += sc.step) {
      var tick = el('span', 'csw-tl-tick', secs(t));
      tick.style.left = (t / this.total * 100) + '%';
      axis.appendChild(tick);
    }
  };

  Timeline.prototype.lane = function (b, i) {
    var lane = el('div', 'csw-tl-lane');
    var name = el('span', 'csw-tl-name', b.label);
    name.title = b.title;
    var track = el('div', 'csw-tl-track');
    var bar = el('div', 'csw-tl-bar' + (b.mark ? ' is-mark' : b.end === Infinity ? ' is-open' : ''));
    bar.setAttribute('data-bar', String(i));
    this.place(bar, b);
    if (b.atInput) {
      bar.tabIndex = 0;
      bar.setAttribute('role', 'slider');
      bar.setAttribute('aria-label', b.title + (b.mark ? ', holds at' : ', start'));
      bar.setAttribute('aria-valuemin', '0');
      bar.setAttribute('aria-valuemax', String(limit(b.atInput, 'max', 120)));
      this.value(bar, b);
    }
    if (b.fadeIn > 0 || b.inSel) {
      var fin = el(b.inSel ? 'button' : 'span', 'csw-tl-end csw-tl-in', b.inSel ? tag(b.inSel) : b.tagIn || '');
      if (b.inSel) {
        fin.type = 'button';
        fin.setAttribute('aria-haspopup', 'menu');
        fin.setAttribute('aria-label', b.title + ': arrives, ' + optionText(b.inSel));
        fin.setAttribute('data-pick', 'in');
      }
      bar.appendChild(fin);
    }
    if (b.outSel) {
      var fout = el('button', 'csw-tl-end csw-tl-out', tag(b.outSel));
      fout.type = 'button';
      fout.setAttribute('aria-haspopup', 'menu');
      fout.setAttribute('aria-label', b.title + ': leaves, ' + optionText(b.outSel));
      fout.setAttribute('data-pick', 'out');
      bar.appendChild(fout);
    }
    if (b.buildInput) bar.appendChild(this.grip('build', b.title + ': how long it builds'));
    if (b.lenInput && b.shown > 0) bar.appendChild(this.grip('len', b.title + ': time on screen'));
    this.wire(bar, b);
    track.appendChild(bar);
    lane.appendChild(name);
    lane.appendChild(track);
    return lane;
  };

  Timeline.prototype.grip = function (what, label) {
    var g = el('span', 'csw-tl-grip csw-tl-grip-' + what);
    g.setAttribute('data-grip', what);
    g.setAttribute('aria-hidden', 'true');
    g.title = label;
    return g;
  };

  // place sets a bar's position and its faded ends as shares of the
  // timeline and of the bar.
  Timeline.prototype.place = function (bar, b) {
    var end = b.end === Infinity ? this.total : Math.min(b.end, this.total);
    // A start typed below zero is refused on save; drawn at zero meanwhile,
    // so the bar never leaves its track.
    var start = Math.max(Math.min(b.start, this.total), 0);
    bar.style.left = (start / this.total * 100) + '%';
    // A marker's size is its own, centred on its moment.
    if (b.mark) return;
    var len = Math.max(end - start, 0.0001);
    bar.style.width = (len / this.total * 100) + '%';
    bar.style.setProperty('--in', (Math.min(b.fadeIn, len) / len * 100) + '%');
    bar.style.setProperty('--out', (Math.min(b.fadeOut, len) / len * 100) + '%');
  };

  // fit hides a bar's end words when the bar is too short to hold them, so
  // a short line in a long scene never prints its words over each other.
  // The row's own fields still carry the choice.
  function fit(bar) {
    var ends = bar.querySelectorAll('.csw-tl-end');
    if (!ends.length) return;
    var room = bar.clientWidth, need = 0, i;
    for (i = 0; i < ends.length; i++) { ends[i].hidden = false; need += ends[i].scrollWidth; }
    for (i = ends.length - 1; i >= 0 && need > room; i--) { need -= ends[i].scrollWidth; ends[i].hidden = true; }
  }

  Timeline.prototype.value = function (bar, b) {
    bar.setAttribute('aria-valuenow', String(round(b.start)));
    var text = (b.mark ? 'holds at ' : 'starts at ') + secs(b.start);
    if (b.shown > 0) text += ', on screen ' + secs(b.shown);
    else if (b.lenInput) text += ', stays to the end';
    bar.setAttribute('aria-valuetext', text);
  };

  // set writes a value into a row's field the way typing would, clamped to
  // the field's own limits, so the server sees what it always sees.
  function set(input, v, lo, quiet) {
    if (!input) return;
    var min = Math.max(limit(input, 'min', 0), lo == null ? -Infinity : lo);
    v = Math.min(Math.max(round(v), min), limit(input, 'max', 120));
    input.value = String(Math.round(v * 10) / 10);
    if (!quiet) {
      input.dispatchEvent(new Event('input', { bubbles: true }));
      input.dispatchEvent(new Event('change', { bubbles: true }));
    }
  }

  Timeline.prototype.wire = function (bar, b) {
    var self = this;
    bar.addEventListener('keydown', function (e) {
      if (e.target !== bar || !b.atInput) return;
      var d = { ArrowLeft: -1, ArrowDown: -1, ArrowRight: 1, ArrowUp: 1 }[e.key];
      if (e.key === 'Home') { e.preventDefault(); set(b.atInput, 0); self.refocus(bar); return; }
      if (!d) return;
      e.preventDefault();
      set(b.atInput, b.start + d * (e.shiftKey ? 1 : STEP));
      self.refocus(bar);
    });
    bar.addEventListener('click', function (e) {
      var pick = e.target.closest('[data-pick]');
      if (!pick || self.justDragged) return;
      self.openMenu(pick, pick.getAttribute('data-pick') === 'in' ? b.inSel : b.outSel);
    });
    bar.addEventListener('pointerdown', function (e) {
      if (e.button !== 0) return;
      var grip = e.target.closest('[data-grip]');
      var what = grip ? grip.getAttribute('data-grip') : (b.atInput ? 'move' : null);
      if (!what) return;
      var track = bar.parentElement.getBoundingClientRect();
      self.drag = { bar: bar, b: b, what: what, x: e.clientX, per: self.total / Math.max(track.width, 1), moved: false,
        start: b.start, build: b.buildInput ? num(b.buildInput) : 0, len: b.shown || 0, id: e.pointerId };
      self.justDragged = false;
      // Followed on the document until it takes hold: a press near an edge
      // that moves outward leaves the bar before the drag begins.
      document.addEventListener('pointermove', move);
      document.addEventListener('pointerup', finish);
      document.addEventListener('pointercancel', finish);
    });
    var move = function (e) {
      var d = self.drag;
      if (!d || d.bar !== bar || e.pointerId !== d.id) return;
      var dx = e.clientX - d.x;
      if (!d.moved) {
        if (Math.abs(dx) < DRAG_PX) return;
        d.moved = true;
        bar.setPointerCapture(e.pointerId);
        bar.classList.add('is-dragging');
      }
      var ds = dx * d.per;
      if (d.what === 'move') {
        set(b.atInput, d.start + ds, null, true);
        b.start = num(b.atInput);
        if (b.end !== Infinity) b.end = b.start + b.shown + TEXT_OUT;
      } else if (d.what === 'build') {
        set(b.buildInput, d.build + ds, null, true);
        b.fadeIn = num(b.buildInput);
      } else if (d.what === 'len') {
        set(b.lenInput, d.len + ds, 0.5, true);
        b.shown = num(b.lenInput);
        b.fadeIn = Math.min(TEXT_IN, b.shown);
        b.end = b.start + b.shown + TEXT_OUT;
      }
      self.place(bar, b);
      fit(bar);
      if (b.atInput) self.value(bar, b);
    };
    var finish = function (e) {
      var d = self.drag;
      if (!d || d.bar !== bar || e.pointerId !== d.id) return;
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', finish);
      document.removeEventListener('pointercancel', finish);
      self.drag = null;
      if (!d.moved) return;
      self.justDragged = true;
      setTimeout(function () { self.justDragged = false; }, 0);
      var input = d.what === 'move' ? b.atInput : d.what === 'build' ? b.buildInput : b.lenInput;
      set(input, num(input));
      self.draw();
      self.refocus(null, d);
    };
  };

  // refocus puts focus back on the bar a key or drag changed, after the
  // redraw replaced it.
  Timeline.prototype.refocus = function (bar, d) {
    var idx = bar ? bar.getAttribute('data-bar') : d && d.bar.getAttribute('data-bar');
    var again = idx != null && q(this.box, '[data-bar="' + idx + '"]');
    if (again && (bar || document.activeElement === document.body)) again.focus();
  };

  // openMenu lists the row's own choices under the faded end that was
  // pressed. Picking one sets the row's field; Esc or a press elsewhere
  // closes it and focus goes back.
  Timeline.prototype.openMenu = function (btn, sel) {
    if (!sel) return;
    this.closeMenu(false);
    var self = this;
    var menu = el('div', 'csw-tl-menu');
    menu.setAttribute('role', 'menu');
    menu.setAttribute('aria-label', btn.getAttribute('aria-label').replace(/, [^,]*$/, ''));
    var items = [];
    for (var i = 0; i < sel.options.length; i++) {
      var o = sel.options[i];
      var it = el('button', 'csw-tl-item', o.textContent.trim());
      it.type = 'button';
      it.setAttribute('role', 'menuitemradio');
      it.setAttribute('aria-checked', o.selected ? 'true' : 'false');
      it.tabIndex = -1;
      it.setAttribute('data-value', o.value);
      items.push(it);
      menu.appendChild(it);
    }
    menu.addEventListener('click', function (e) {
      var it = e.target.closest('[data-value]');
      if (!it) return;
      sel.value = it.getAttribute('data-value');
      sel.dispatchEvent(new Event('change', { bubbles: true }));
      self.closeMenu(true);
    });
    menu.addEventListener('keydown', function (e) {
      var at = items.indexOf(document.activeElement);
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); self.closeMenu(true); return; }
      if (e.key === 'Tab') { self.closeMenu(false); return; }
      var to = { ArrowDown: at + 1, ArrowUp: at - 1, Home: 0, End: items.length - 1 }[e.key];
      if (to == null) return;
      e.preventDefault();
      items[(to + items.length) % items.length].focus();
    });
    var box = this.box.getBoundingClientRect();
    var r = btn.getBoundingClientRect();
    menu.style.top = (r.bottom - box.top + 4) + 'px';
    this.box.appendChild(menu);
    var left = Math.min(Math.max(r.left - box.left, 0), Math.max(box.width - menu.offsetWidth, 0));
    menu.style.left = left + 'px';
    btn.setAttribute('aria-expanded', 'true');
    this.menu = { el: menu, sel: sel, idx: btn.closest('[data-bar]').getAttribute('data-bar'), pick: btn.getAttribute('data-pick') };
    var on = items.filter(function (x) { return x.getAttribute('aria-checked') === 'true'; })[0] || items[0];
    if (on) on.focus();
  };

  Timeline.prototype.closeMenu = function (focusBack) {
    var m = this.menu;
    if (!m) return;
    this.menu = null;
    if (m.el.parentNode) m.el.parentNode.removeChild(m.el);
    var btn = this.box && q(this.box, '[data-bar="' + m.idx + '"] [data-pick="' + m.pick + '"]');
    if (btn) {
      btn.removeAttribute('aria-expanded');
      if (focusBack) btn.focus();
    }
  };

  Timeline.prototype.destroy = function () {
    this.closeMenu(false);
    this.rows.disconnect();
    this.swaps.disconnect();
    this.mount.removeEventListener('input', this.onInput);
    this.mount.removeEventListener('change', this.onInput);
    document.removeEventListener('pointerdown', this.onDocDown, true);
  };

  window.ChronicleCutsceneTimeline = {
    mount: function (el) { return new Timeline(el); },
    read: read,
    scale: scale
  };
})();
