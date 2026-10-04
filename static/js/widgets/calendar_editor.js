/**
 * calendar_editor.js — the Owner/co-Director editing surface layered on top
 * of calendar_view.js. Loaded only when the
 * page's mount div carries data-can-edit="true" (view.templ), which is
 * true for MemberRole >= RoleOwner OR CanAuthorDmOnly() — see view.go's
 * CalendarViewData.CanEdit doc comment.
 *
 * This file does not mount as its own Chronicle widget (there is only one
 * data-widget="calendar_view" element on the page); it augments the
 * already-mounted calendar_view instance, found on `el.calendarView` (or
 * via the 'calendarv5:ready' event if this script's <script defer> tag
 * somehow ran first).
 *
 * GATING NOTE — read before changing any of the checks below: CanEdit
 * (the gate this whole file loads behind) covers Owner OR a granted
 * co-Director, and is itself defined as Owner OR CanAuthorDmOnly() — the
 * two are equal for every viewer who can reach this file at all. One write
 * this file makes is narrower than that: deleting an event (DELETE
 * .../events/:eid, routes.go) is gated RoleOwner strictly, not
 * CanAuthorDmOnly, so its footer button checks `role >= ROLE_OWNER` instead
 * of the broader canEdit/canAuthorDmOnly — a co-Director who can reach this
 * file will still see that one affordance 403 if this gating is loosened
 * without a matching routes.go change. Creating an event kind, toggling a
 * moon's hidden flag and managing eras used to be narrower the same way;
 * their routes are now CanAuthorDmOnly too, so they gate on
 * `view.canAuthorDmOnly` like every other co-Director write in this file.
 */
(function () {
  'use strict';

  var ROLE_OWNER = 3;

  // Shared with calendar_view.js (Chronicle.calendarDate = CalDate there) so
  // "shift events" uses the exact same date arithmetic as the grid/moon
  // phase code, rather than a second copy that could drift from it.
  var CalDate = Chronicle.calendarDate;
  // Shared with calendar_view.js's eventColorStyle so a kind's colour goes
  // through one allowlist everywhere it lands in a style="" attribute.
  var sanitizeColor = Chronicle.calendarColor;

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  // A form that grows inside a card can run past the edge of whatever is
  // scrolling it (the page, or a calendar opened out on the Calendars page),
  // so it is brought into view as it opens.
  function reducedMotionNow() { return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches); }
  function reveal(el) {
    var still = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    el.scrollIntoView({ block: 'nearest', behavior: still ? 'auto' : 'smooth' });
  }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (ch) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch];
    });
  }

  // The compact event form (below) only ever edits plain text — it has no
  // rich-text editor — so reading an existing rich description back into it
  // strips the stored HTML down to text rather than showing markup.
  function stripHtml(html) {
    if (!html) return '';
    var div = document.createElement('div');
    div.innerHTML = html;
    return div.textContent || div.innerText || '';
  }

  // existing.description is ProseMirror JSON whenever existing.description_html
  // is set (Event's doc comment, model.go) and plain text otherwise — the same
  // rule showGlance() in calendar_view.js reads. Showing the JSON verbatim in a
  // plain textarea would be unreadable and, worse, invites saving it back as
  // "plain text" that is actually still-JSON-shaped garbage.
  function descriptionText(existing) {
    if (!existing) return '';
    if (existing.description_html) return stripHtml(existing.description_html);
    return existing.description || '';
  }

  // Rebuilds description_html from the textarea's plain text on save, the same
  // paragraph-per-blank-line shape entity_notes.js's bodyToHTML uses, so the
  // sanitizer (bluemonday's UGC policy, internal/sanitize) has plain <p>/<br>
  // to accept rather than raw text with no markup at all.
  function bodyToHTML(text) {
    if (!text) return '';
    var paragraphs = text.split(/\n{2,}/);
    return paragraphs.map(function (p) {
      return '<p>' + esc(p).replace(/\n/g, '<br>') + '</p>';
    }).join('');
  }

  // calendar_view is one shared object re-initialised for each mount, so an
  // editor belongs to the mount element it was installed on, not the view.
  function attach(view) {
    if (!view || !view.el || view._editorEl === view.el) return;
    view._editorEl = view.el;
    new CalendarEditor(view).install();
  }

  function CalendarEditor(view) {
    this.view = view;
    this.editing = false;
    this.selection = {}; // dayKey -> true, edit-mode multi-select
    this.anchorKey = null; // for shift-click range select
  }

  CalendarEditor.prototype.install = function () {
    this.view._editor = this;
    this._injectEditToggle();
    this._injectHEditStrip();
    this._wrapEvpRendering();
    this._wrapMvRendering();
    this._wrapFlapRendering();
    this._bindStageCapture();
    this._bindWingCapture();
    this._bindEvpCapture();
    this._bindMvCapture();
    this._bindFlapCapture();
    this._buildBulkBar();
  };

  // ------------------------------------------------------------------
  // Edit-mode toggle + the header's edit strip (h-edit swaps in for
  // h-sub, at the same height, so the grid never moves — calendar-view.css
  // already carries the .cal.editing show/hide transition for this).
  // ------------------------------------------------------------------
  CalendarEditor.prototype._injectEditToggle = function () {
    var acts = $('.h-acts', this.view.el);
    if (!acts) return;
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'tbtn';
    btn.id = 'cal5-editbtn';
    btn.setAttribute('aria-pressed', 'false');
    btn.innerHTML = '<i class="fa-solid fa-sliders"></i><span>Edit</span>';
    // After the Sky chip, which leads the actions, as signed.
    var sky = $('#cal5-skybtn', acts);
    acts.insertBefore(btn, sky ? sky.nextSibling : acts.firstChild);
    var self = this;
    btn.addEventListener('click', function () { self.setEditing(!self.editing); });
    this.editBtn = btn;
  };

  CalendarEditor.prototype._injectHEditStrip = function () {
    var subw = $('.h-subw', this.view.el);
    if (!subw) return;
    var strip = document.createElement('div');
    strip.className = 'h-edit';
    // The structure editor is Owner only (routes.go), so only an Owner
    // gets the gear that leads to it. Weather here works without choosing
    // days: it opens on the shown month.
    var view = this.view;
    var gear = view.role >= ROLE_OWNER && view.campaignId && view.calendarId
      ? '<a class="btn sm quiet" id="cal5-settings" href="/campaigns/' + encodeURIComponent(view.campaignId) + '/calendars/' + encodeURIComponent(view.calendarId) + '/structure" title="Calendar settings: months, weekdays, leap years, moons and seasons"><i class="fa-solid fa-gear"></i><span>Calendar settings</span></a>'
      : '';
    var gen = view.canAuthorDmOnly && Chronicle.calendarWeatherSheet;
    // Era authors (Owner or co-Director) manage eras from here; the card
    // folds out of this button.
    var eras = view.canAuthorDmOnly
      ? '<button type="button" class="btn sm quiet" id="cal5-erabtn" aria-haspopup="dialog" aria-expanded="false"><i class="fa-solid fa-timeline"></i><span class="btxt"> Eras</span></button>'
      : '';
    strip.innerHTML = '<span class="etag">Editing</span><span class="ehint">Click or drag across days. Shift extends, Ctrl/Cmd adds. Touch: tap, or hold then drag.</span><span class="sp"></span>' +
      (gen ? '<button type="button" class="btn sm quiet" id="edGen" aria-haspopup="dialog" aria-label="Weather"><i class="fa-solid fa-cloud-sun"></i><span class="btxt"> Weather</span></button>' : '') +
      eras + gear + '<button type="button" class="btn sm quiet" id="cal5-editdone">Done</button>';
    subw.appendChild(strip);
    $('#cal5-editdone', strip).addEventListener('click', this.setEditing.bind(this, false));
    var self = this;
    if (eras) $('#cal5-erabtn', strip).addEventListener('click', function () { view.toggleEra(); });
    if (gen) $('#edGen', strip).addEventListener('click', function () { self._openWeather(); });
  };

  CalendarEditor.prototype.setEditing = function (on) {
    this.editing = on;
    this.view.calEl.classList.toggle('editing', on);
    // Edit mode shows each day's stored weather (.cwx) to whoever may paint it.
    this.view.calEl.classList.toggle('wxon', on && this.view.canAuthorDmOnly);
    this.editBtn.setAttribute('aria-pressed', String(on));
    this._endDrag();
    // The view owns the weekday header; re-render it so the buttons go
    // back to plain labels, then decorate again only while editing.
    this.view.renderHeader();
    if (!on) { this.selection = {}; this._syncPicks(); this._renderBar(); }
    this.view.say(on ? 'Editing on. Click or drag across days to choose them.' : 'Editing off.');
  };

  // ------------------------------------------------------------------
  // Select-many. Click picks one day, Shift-click extends from the last
  // day pressed, Ctrl/Cmd-click adds or removes. Press-and-drag across
  // days selects a range (Ctrl/Cmd adds or removes it the same way); on
  // touch a tap toggles a day and a ~350ms hold, then drag, selects a
  // range so ordinary swipes still scroll the page. A weekday button
  // picks that column of the visible month, a week handle that week.
  // Click is captured (not bubbled) so it runs before calendar_view.js's
  // own stage click handler and can stop it with stopPropagation; a
  // click that ends a drag is swallowed so it does not re-select.
  // ------------------------------------------------------------------
  var HOLD_MS = 350;
  var HOLD_SLOP = 10;

  CalendarEditor.prototype._bindStageCapture = function () {
    var self = this, stage = this.view.stageEl;
    this._drag = null;
    this._swallowClick = false;
    this._lastPointerType = 'mouse';

    stage.addEventListener('click', function (e) {
      if (!self.editing) return;
      if (self._swallowClick || self._spaceDown) { self._swallowClick = false; e.stopPropagation(); e.preventDefault(); return; }
      var wkh = e.target.closest('.wkh');
      if (wkh) { e.stopPropagation(); self._pickKeys(wkh.dataset.keys.split(',').filter(Boolean), e); return; }
      var day = e.target.closest('.day');
      if (!day || !day.dataset.key) return;
      e.stopPropagation();
      var key = day.dataset.key;
      var touch = self._lastPointerType === 'touch' && !e.shiftKey && !e.ctrlKey && !e.metaKey;
      var extend = e.shiftKey || e.ctrlKey || e.metaKey || touch;
      var keys = (e.shiftKey && self.anchorKey) ? self._rangeBetween(self.anchorKey, key) : [key];
      if (!extend) {
        // Plain click replaces the selection with just this one day.
        self.selection = {};
        self.selection[key] = true;
      } else {
        // Shift/Ctrl click (or a touch tap) extends: if every key in the
        // range/click is already selected, the gesture removes them;
        // otherwise it adds them all. (Mirrors the mockup's applySel
        // 'remove'/'add' — never a per-key toggle, which would leave a
        // range half on/half off.)
        var allOn = keys.every(function (k) { return self.selection[k]; });
        keys.forEach(function (k) {
          if (allOn) delete self.selection[k];
          else self.selection[k] = true;
        });
      }
      self.anchorKey = key;
      self._syncPicks();
      self._renderBar();
    }, true);

    stage.addEventListener('pointerdown', function (e) {
      self._lastPointerType = e.pointerType || 'mouse';
      self._swallowClick = false;
      if (!self.editing || e.shiftKey) return;
      var day = e.target.closest('.day[data-key]');
      if (!day) return;
      var isTouch = e.pointerType === 'touch';
      if (!isTouch && e.button !== 0) return;
      var mod = e.ctrlKey || e.metaKey;
      var d = self._drag = {
        id: e.pointerId, from: day.dataset.key, last: day.dataset.key,
        x: e.clientX, y: e.clientY, touch: isTouch, active: false,
        // Mouse drag replaces the selection (or edits it with Ctrl/Cmd);
        // a touch hold only ever adds to what is already picked.
        mode: isTouch ? 'add' : (mod ? 'toggle' : 'set'),
        base: (isTouch || mod) ? self._copySelection() : {}
      };
      if (isTouch) d.timer = setTimeout(function () { self._startHold(d); }, HOLD_MS);
    });

    stage.addEventListener('pointermove', function (e) {
      var d = self._drag;
      if (!d || e.pointerId !== d.id) return;
      if (d.touch && !d.active) {
        // Moving before the hold fires means the person is scrolling.
        if (Math.hypot(e.clientX - d.x, e.clientY - d.y) > HOLD_SLOP) self._endDrag();
        return;
      }
      var key = self._keyAt(e.clientX, e.clientY);
      if (!key || key === d.last) return;
      if (!d.active) {
        // First move onto another day: this is a drag, not a click. Capture
        // only now, since capturing at press would retarget the plain click.
        d.active = true;
        try { stage.setPointerCapture(e.pointerId); } catch (err) { /* capture only keeps a fast drag on the grid */ }
      }
      d.last = key;
      self._applyDrag(d);
    });

    var finish = function (e) {
      var d = self._drag;
      if (!d || (e && e.pointerId !== d.id)) return;
      var dragged = d.active;
      self._endDrag();
      if (dragged) {
        self.anchorKey = d.from;
        // Only a pointerup is followed by a click; cancel is not.
        self._swallowClick = !e || e.type === 'pointerup';
      }
    };
    stage.addEventListener('pointerup', finish);
    stage.addEventListener('pointercancel', finish);

    // Once a hold has started, stop the page scrolling under the finger.
    stage.addEventListener('touchmove', function (e) {
      if (self._drag && self._drag.touch && self._drag.active && e.cancelable) e.preventDefault();
    }, { passive: false });
    stage.addEventListener('contextmenu', function (e) {
      if (self.editing && self._drag && self._drag.touch) e.preventDefault();
    });

    this._bindKeys();
    this._wrapGridRendering();
    this.view.dowEl.addEventListener('click', function (e) {
      var b = e.target.closest('.dwb');
      if (!b || !self.editing) return;
      self._pickKeys(self._columnKeys(+b.dataset.col), e);
    });
  };

  CalendarEditor.prototype._copySelection = function () {
    var out = {}, sel = this.selection;
    Object.keys(sel).forEach(function (k) { if (sel[k]) out[k] = true; });
    return out;
  };

  // Stops any drag in flight and its hold timer (edit mode turning off,
  // pointer released, scroll detected).
  CalendarEditor.prototype._endDrag = function () {
    var d = this._drag;
    if (!d) return;
    clearTimeout(d.timer);
    this._drag = null;
  };

  CalendarEditor.prototype._startHold = function (d) {
    if (this._drag !== d || !this.editing) return;
    d.active = true;
    this._applyDrag(d);
    this.view.say('Hold and drag to choose a range.');
  };

  // Recomputes the selection from the drag's starting selection and the
  // range it has swept, so dragging back shrinks the range again.
  CalendarEditor.prototype._applyDrag = function (d) {
    var keys = this._rangeBetween(d.from, d.last), sel = d.base;
    var next = {};
    Object.keys(sel).forEach(function (k) { next[k] = true; });
    var remove = d.mode === 'toggle' && keys.every(function (k) { return sel[k]; });
    keys.forEach(function (k) { if (remove) delete next[k]; else next[k] = true; });
    this.selection = next;
    this._syncPicks();
    this._renderBar();
  };

  CalendarEditor.prototype._keyAt = function (x, y) {
    var el = document.elementFromPoint(x, y);
    var day = el && el.closest ? el.closest('.day[data-key]') : null;
    return day && this.view.stageEl.contains(day) && !day.closest('.month.leaving') ? day.dataset.key : null;
  };

  // A column/week gesture: plain replaces the selection, Shift/Ctrl/Cmd
  // adds the group or, when all of it is already picked, removes it.
  CalendarEditor.prototype._pickKeys = function (keys, e) {
    var self = this;
    if (!keys.length) return;
    if (e.shiftKey || e.ctrlKey || e.metaKey) {
      var allOn = keys.every(function (k) { return self.selection[k]; });
      keys.forEach(function (k) { if (allOn) delete self.selection[k]; else self.selection[k] = true; });
    } else {
      this.selection = {};
      keys.forEach(function (k) { self.selection[k] = true; });
    }
    this.anchorKey = keys[0];
    this._syncPicks();
    this._renderBar();
  };

  // The keyed days of the visible month in column `col`, from the rendered
  // week rows so intercalary bands and short weeks need no special case.
  CalendarEditor.prototype._columnKeys = function (col) {
    var keys = [];
    $$('.wk', this.view.stageEl).forEach(function (wk) {
      var cells = [].filter.call(wk.children, function (c) { return c.classList.contains('day'); });
      if (cells[col] && cells[col].dataset.key) keys.push(cells[col].dataset.key);
    });
    return keys;
  };

  // Weekday buttons and week handles exist only while editing and are
  // rebuilt whenever the view redraws the header or the month.
  CalendarEditor.prototype._wrapGridRendering = function () {
    var self = this, view = this.view;
    ['renderHeader', '_paintMonth'].forEach(function (name) {
      var original = view[name];
      view[name] = function () {
        var r = original.apply(this, arguments);
        if (view._editor === self) self._decorateGrid();
        return r;
      };
    });
  };

  CalendarEditor.prototype._decorateGrid = function () {
    var self = this, view = this.view;
    $$('.wkh', view.stageEl).forEach(function (h) { h.remove(); });
    var dow = view.dowEl;
    if (!this.editing) {
      dow.removeAttribute('role');
      dow.removeAttribute('aria-label');
      dow.setAttribute('aria-hidden', 'true');
      return;
    }
    dow.removeAttribute('aria-hidden');
    dow.setAttribute('role', 'group');
    dow.setAttribute('aria-label', 'Weekdays');
    [].forEach.call(dow.children, function (span, i) {
      if (span.tagName === 'BUTTON') return;
      var b = document.createElement('button');
      b.type = 'button';
      b.className = 'dwb';
      b.dataset.col = String(i);
      b.setAttribute('aria-label', 'Select every ' + (span.querySelector('.f') || span).textContent + ' this month');
      b.innerHTML = span.innerHTML;
      dow.replaceChild(b, span);
    });
    $$('.wk', view.stageEl).forEach(function (wk, i) {
      var keys = $$('.day[data-key]', wk).map(function (d) { return d.dataset.key; });
      if (!keys.length) return;
      var h = document.createElement('button');
      h.type = 'button';
      h.className = 'wkh';
      h.dataset.keys = keys.join(',');
      h.setAttribute('aria-label', 'Select week ' + (i + 1) + ' of this month');
      h.innerHTML = '<i class="fa-solid fa-grip-vertical" aria-hidden="true"></i>';
      // Last child: the first .day keeps its :first-child border rule.
      wk.appendChild(h);
    });
    this._syncPicks();
  };

  // Space toggles the focused day, Shift+arrows extend from the anchor,
  // plain arrows move focus; Escape clears a selection before anything
  // else, but yields to an open panel so Escape still closes those first.
  CalendarEditor.prototype._bindKeys = function () {
    var self = this, view = this.view, stage = view.stageEl;
    stage.addEventListener('keydown', function (e) {
      if (!self.editing) return;
      var day = e.target.closest ? e.target.closest('.day[data-key]') : null;
      if (!day) return;
      if (e.key === ' ' || e.key === 'Spacebar') {
        e.preventDefault();
        self._spaceDown = true; // the button's own Space click follows keyup and must not re-select
        var k = day.dataset.key;
        if (self.selection[k]) delete self.selection[k]; else self.selection[k] = true;
        self.anchorKey = k;
        self._syncPicks();
        self._renderBar();
        return;
      }
      var cols = parseInt(view.calEl.style.getPropertyValue('--cols'), 10) || 7;
      var step = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -cols, ArrowDown: cols }[e.key];
      if (!step) return;
      var cells = $$('.day', stage), at = cells.indexOf(day), to = cells[at + step];
      if (!to || !to.dataset.key) return;
      e.preventDefault();
      to.focus();
      if (!e.shiftKey) return;
      var from = self.anchorKey || day.dataset.key;
      self.anchorKey = from;
      self.selection = {};
      self._rangeBetween(from, to.dataset.key).forEach(function (k) { self.selection[k] = true; });
      self._syncPicks();
      self._renderBar();
    });
    stage.addEventListener('keyup', function (e) {
      if (e.key !== ' ' && e.key !== 'Spacebar') return;
      if (self._spaceDown) e.preventDefault();
      setTimeout(function () { self._spaceDown = false; }, 150);
    });

    var onEsc = function (e) {
      if (!document.contains(view.el)) { document.removeEventListener('keydown', onEsc, true); return; }
      if (e.key !== 'Escape' || !self.editing || view._editor !== self) return;
      if (!self._selectedKeys().length) return;
      var panelOpen = view.evpEl.classList.contains('open') || view.mvEl.classList.contains('open') ||
        view._pf.state !== 'closed' || view.popEl.classList.contains('open') || view.wingFor ||
        (Chronicle.calendarEventDrawer && Chronicle.calendarEventDrawer.isOpen(view)) ||
        (Chronicle.calendarWeatherSheet && Chronicle.calendarWeatherSheet.isOpen(view));
      if (panelOpen) return;
      e.stopPropagation();
      self.selection = {};
      self.anchorKey = null;
      self._syncPicks();
      self._renderBar();
    };
    // Capture phase so it runs ahead of the view's own document handler.
    document.addEventListener('keydown', onEsc, true);
  };

  CalendarEditor.prototype._rangeBetween = function (a, b) {
    var buttons = $$('.day[data-key]', this.view.stageEl);
    var keys = buttons.map(function (d) { return d.dataset.key; });
    var ia = keys.indexOf(a), ib = keys.indexOf(b);
    if (ia < 0 || ib < 0) return [b];
    var lo = Math.min(ia, ib), hi = Math.max(ia, ib);
    return keys.slice(lo, hi + 1);
  };

  CalendarEditor.prototype._syncPicks = function () {
    var self = this;
    $$('.day[data-key]', this.view.stageEl).forEach(function (d) {
      d.classList.toggle('pick', !!self.selection[d.dataset.key]);
    });
    var all = function (keys) { return keys.length > 0 && keys.every(function (k) { return self.selection[k]; }); };
    $$('.wkh', this.view.stageEl).forEach(function (h) {
      h.setAttribute('aria-pressed', String(all(h.dataset.keys.split(','))));
    });
    $$('.dwb', this.view.dowEl).forEach(function (b) {
      b.setAttribute('aria-pressed', String(all(self._columnKeys(+b.dataset.col))));
    });
  };

  CalendarEditor.prototype._selectedKeys = function () {
    var self = this;
    return Object.keys(this.selection).filter(function (k) { return self.selection[k]; });
  };

  // Every distinct event touching any selected day.
  CalendarEditor.prototype._eventsInSelection = function () {
    var view = this.view, seen = {}, out = [];
    this._selectedKeys().forEach(function (k) {
      var d = k.split('_').map(Number);
      view.eventsOnDay(d[0], d[1], d[2]).forEach(function (e) {
        if (!seen[e.id]) { seen[e.id] = true; out.push(e); }
      });
    });
    return out;
  };

  // ------------------------------------------------------------------
  // The bulk bar: rises from the grid's bottom edge as soon as a day is
  // selected. Only the bulk actions this API can actually perform
  // ship: bulk visibility (Hide/Reveal, gated CanAuthorDmOnly, the same
  // gate the single-event visibility route already uses) and Shift events
  // (moves every event touching the selection by N days), plus Weather,
  // which opens the weather calendar (calendar_weather_sheet.js) with those
  // days chosen; Lock keeps the chosen days' weather safe from a roll.
  // ------------------------------------------------------------------
  // lockState is the Lock button's face for the chosen days' stored readings
  // (null where a day has none): Unlock once every reading is locked, and
  // disabled when no chosen day has weather to lock.
  function lockState(readings) {
    var have = (readings || []).filter(Boolean);
    var allLocked = have.length > 0 && have.every(function (w) { return w.locked === true; });
    return { label: allLocked ? 'Unlock' : 'Lock', icon: allLocked ? 'fa-lock-open' : 'fa-lock', disabled: !have.length, unlocking: allLocked };
  }
  function lockButtonHTML(st) {
    return '<button type="button" class="bbb" data-bb="lock"' + (st.disabled ? ' disabled' : '') + '><i class="fa-solid ' + st.icon + '" aria-hidden="true"></i><span>' + st.label + '</span></button>';
  }

  CalendarEditor.prototype._buildBulkBar = function () {
    var wrap = document.createElement('div');
    wrap.className = 'bbw';
    wrap.innerHTML = '<div class="bbar" id="cal5-bbar" hidden></div>';
    this.view.calEl.appendChild(wrap);
    this.bbar = $('#cal5-bbar', wrap);
    var self = this;
    this.bbar.addEventListener('click', function (e) {
      var t = e.target.closest('[data-bb]');
      if (!t) return;
      var a = t.dataset.bb;
      if (a === 'none') { self.selection = {}; self._syncPicks(); self._renderBar(); }
      else if (a === 'hide' || a === 'reveal') self._bulkVisibility(a === 'hide' ? 'dm_only' : 'everyone');
      else if (a === 'shift') self._toggleShiftTray();
      else if (a === 'wx') self._openWeather();
      else if (a === 'lock') self._lockDays();
    });
  };

  CalendarEditor.prototype._renderBar = function () {
    var n = this._selectedKeys().length;
    // .bar-up tells calendar-view.css's toast rule to lift a toast clear of
    // the bar (they're both bottom-centered a few px apart) — set on .cal,
    // not .bbar itself, so a plain CSS sibling/descendant selector can react
    // to it without needing :has().
    this.view.calEl.classList.toggle('bar-up', !!n);
    if (!n) { this.bbar.hidden = true; this.bbar.innerHTML = ''; this._closeShiftTray(); return; }
    var canVis = this.view.canAuthorDmOnly, self = this;
    this.bbar.hidden = false;
    this.bbar.innerHTML =
      '<div class="bbl"><b>' + (n === 1 ? '1 day' : n + ' days') + ' chosen</b><span>' + this._eventsInSelection().length + ' events touched</span></div>' +
      '<div class="bba">' +
        (canVis ? '<button type="button" class="bbb" data-bb="hide"><i class="fa-solid fa-eye-slash"></i><span>Hide</span></button><button type="button" class="bbb" data-bb="reveal"><i class="fa-solid fa-eye"></i><span>Reveal</span></button>' : '') +
        '<button type="button" class="bbb" data-bb="shift" aria-expanded="' + !!this._shiftTray + '"><i class="fa-solid fa-arrows-left-right"></i><span>Shift events</span></button>' +
        (canVis ? (Chronicle.calendarWeatherSheet ? '<button type="button" class="bbb" data-bb="wx" aria-haspopup="dialog"><i class="fa-solid fa-cloud-sun"></i><span>Weather</span></button>' : '') +
          lockButtonHTML(lockState(this._selectedDates().map(function (d) { return (self.view.weatherByYear[d.year] || {})[d.month + '_' + d.day] || null; }))) : '') +
      '</div>' +
      '<button type="button" class="x" data-bb="none" aria-label="Choose no days">✕</button>';
  };

  // Runs one Chronicle.apiFetch()-returning promise per item and resolves
  // true/false per item instead of letting a bare Promise.all either fail
  // fast on the first network rejection or (worse) resolve "done" on a
  // 403/404/500 — apiFetch's promise only ever rejects on a network failure,
  // never on a non-2xx status (boot.js), so treating its resolution alone as
  // success is exactly how a bulk write can report success while some or all
  // of its per-event requests actually failed. Every bulk write in this file
  // goes through this so a partial failure is counted, not swallowed.
  function settleAll(promises) {
    return Promise.all(promises.map(function (p) {
      return p.then(function (resp) { return resp.ok; }).catch(function () { return false; });
    }));
  }

  // Refreshes the grid from the server (so it matches whatever actually
  // landed, even on a partial failure) and reports how many of a bulk
  // write's per-event requests succeeded. verb is past tense ("Hid",
  // "Revealed", "Shifted"); tail is an optional clause inserted before the
  // event count's trailing punctuation (e.g. " by 2 days").
  CalendarEditor.prototype._reportBulkResult = function (oks, verb, tail, note) {
    var view = this.view, okCount = oks.filter(Boolean).length, failCount = oks.length - okCount;
    view.eventsByMonth = {}; // simplest correct invalidation: refetch on next paint
    view.renderMonth();
    tail = tail || '';
    if (!failCount) {
      view.say(verb + ' ' + okCount + (okCount === 1 ? ' event' : ' events') + tail + '.' + (note || ''));
    } else {
      view.say(verb + ' ' + okCount + ' of ' + oks.length + ' events' + tail + '; ' + failCount + " couldn't be saved." + (note || ''));
    }
  };

  CalendarEditor.prototype._bulkVisibility = function (visibility) {
    var self = this, view = this.view, events = this._eventsInSelection();
    if (!events.length) { view.say('No events in the selected days.'); return; }
    var verb = visibility === 'dm_only' ? 'Hid' : 'Revealed';
    settleAll(events.map(function (e) {
      return Chronicle.apiFetch(view.apiBase + '/events/' + e.id + '/visibility', {
        method: 'PUT', body: { visibility: visibility }
      });
    })).then(function (oks) {
      self._reportBulkResult(oks, verb);
    });
  };

  CalendarEditor.prototype._toggleShiftTray = function () {
    if (this._shiftTray) { this._closeShiftTray(); return; }
    var self = this, view = this.view;
    var tray = document.createElement('div');
    tray.className = 'btray';
    tray.innerHTML = '<div class="trh">Shift events<small>Moves every event touching the chosen days by this many days.</small></div>' +
      '<div class="shn"><button type="button" data-shift="-1">−</button><output id="cal5-shiftn">+1</output><button type="button" data-shift="1">+</button>' +
      '<button type="button" class="btn primary sm" id="cal5-shiftgo">Apply</button></div>';
    view.calEl.appendChild(tray);
    this._shiftTray = tray;
    this._shiftN = 1;
    $('[data-bb="shift"]', this.bbar).setAttribute('aria-expanded', 'true');
    tray.addEventListener('click', function (e) {
      var d = e.target.closest('[data-shift]');
      if (d) { self._shiftN += parseInt(d.dataset.shift, 10); $('#cal5-shiftn', tray).textContent = (self._shiftN >= 0 ? '+' : '') + self._shiftN; }
      if (e.target.id === 'cal5-shiftgo') self._applyShift();
    });
  };

  CalendarEditor.prototype._closeShiftTray = function () {
    if (!this._shiftTray) return;
    var b = $('[data-bb="shift"]', this.bbar);
    if (b) b.setAttribute('aria-expanded', 'false');
    this._shiftTray.remove();
    this._shiftTray = null;
  };

  CalendarEditor.prototype._applyShift = function () {
    var self = this, view = this.view, cal = view.cal, delta = this._shiftN, events = this._eventsInSelection();
    if (!delta || !events.length) { this._closeShiftTray(); return; }
    // A rule event's dates come from its rule and a repeating event with
    // skips or moves is keyed by dates a shift would orphan; both are left
    // as they are, and the result says so.
    var left = events.filter(function (e) {
      if (e.recurrence_type === 'rule') return true;
      return (e.occurrences || []).some(function (o) { return o.skipped || o.moved_from; });
    });
    events = events.filter(function (e) { return left.indexOf(e) < 0; });
    var note = left.length ? ' ' + left.length + (left.length === 1 ? ' event repeats' : ' events repeat') + ' by a rule or has changed dates, and was left as it is.' : '';
    if (!events.length) {
      self._closeShiftTray();
      view.say('Nothing shifted.' + note);
      return;
    }
    settleAll(events.map(function (e) {
      var start = CalDate.addDays(cal, { y: e.year, m: e.month, d: e.day }, delta);
      var body = { year: start.y, month: start.m, day: start.d };
      if (e.end_year != null) {
        var end = CalDate.addDays(cal, { y: e.end_year, m: e.end_month, d: e.end_day }, delta);
        body.end_year = end.y; body.end_month = end.m; body.end_day = end.d;
      }
      return Chronicle.apiFetch(view.apiBase + '/events/' + e.id, { method: 'PUT', body: body });
    })).then(function (oks) {
      self._closeShiftTray();
      var tail = ' by ' + delta + (Math.abs(delta) === 1 ? ' day' : ' days');
      self._reportBulkResult(oks, 'Shifted', tail, note);
    });
  };

  // ------------------------------------------------------------------
  // Weather: the edit strip's and the bulk bar's Weather buttons open the
  // weather calendar (calendar_weather_sheet.js) on a month, with any chosen
  // days of that month still chosen. It keeps a draft; Save hands it here as
  // writes, and Undo puts back exactly what the days held before.
  // ------------------------------------------------------------------
  // loadEngine runs cb once chronicle_gen.js is on the page, injecting it
  // the first time; onFail runs if it cannot load.
  function loadEngine(src, cb, onFail) {
    if (window.ChronicleGen) { cb(); return; }
    if (!src) { onFail(); return; }
    var s = document.createElement('script');
    s.src = src;
    s.onload = function () { if (window.ChronicleGen) cb(); else onFail(); };
    s.onerror = onFail;
    document.head.appendChild(s);
  }

  // dayInput flattens a stored reading back into the write shape, keeping
  // its source, so Undo can put it back as it was.
  function dayInput(w) {
    var wind = w.wind || {}, pr = w.precipitation || {};
    return {
      year: w.year, month: w.month, day: w.day, source: w.source || 'manual',
      preset_id: w.preset_id || null, preset_label: w.preset_label || null, icon: w.icon || null, color: w.color || null,
      temperature_celsius: w.temperature_celsius != null ? w.temperature_celsius : null,
      wind_speed_kph: wind.speed_kph != null ? wind.speed_kph : null, wind_speed_tier: wind.speed_tier || null,
      wind_direction: wind.direction || null, wind_direction_degrees: wind.direction_degrees != null ? wind.direction_degrees : null,
      precipitation_type: pr.type || null, precipitation_intensity: pr.intensity != null ? pr.intensity : null,
      zone_id: w.zone_id || null, zone_name: w.zone_name || null, description: w.description || null
    };
  }

  // _selectedDates turns the selection into {year, month, day} dates.
  CalendarEditor.prototype._selectedDates = function () {
    return this._selectedKeys().map(function (k) {
      var d = k.split('_').map(Number);
      return { year: d[0], month: d[1], day: d[2] };
    });
  };

  // _snapshot records what the chosen days hold now (a reading or null), for
  // Undo. Every year a chosen day falls in is loaded first.
  // A year that fails to load rejects, so Undo never mistakes "unknown" for
  // "no weather" and clears a day it should have restored.
  function yearsOf(dates) {
    var seen = {};
    dates.forEach(function (d) { seen[d.year] = true; });
    return Object.keys(seen).map(Number);
  }

  CalendarEditor.prototype._snapshot = function (dates) {
    var view = this.view, years = yearsOf(dates);
    return Promise.all(years.map(function (y) { return view.fetchWeatherYear(y); })).then(function () {
      years.forEach(function (y) { if (!view.weatherByYear[y]) throw new Error('weather not loaded'); });
      return dates.map(function (d) {
        var w = (view.weatherByYear[d.year] || {})[d.month + '_' + d.day];
        return { date: d, prev: w ? dayInput(w) : null, locked: !!(w && w.locked) };
      });
    });
  };

  // _refreshWeather reloads the touched years and then redraws, so the grid
  // shows what the server actually holds without flashing empty meanwhile.
  CalendarEditor.prototype._refreshWeather = function (dates) {
    var view = this.view, self = this;
    // The Director's "Players see" line follows the saved weather.
    if (view.canAuthorDmOnly && view.fetchForecast) view.fetchForecast(true);
    return Promise.all(yearsOf(dates).map(function (y) {
      var old = view.weatherByYear[y];
      delete view.weatherByYear[y];
      return view.fetchWeatherYear(y).then(function () {
        if (!view.weatherByYear[y] && old) view.weatherByYear[y] = old;
      });
    })).then(function () {
      view.renderMonth();
      view.refreshWing();
      self._renderBar();
    });
  };

  // _lockDays locks (or unlocks) the chosen days that hold weather. Undo
  // sends the opposite for exactly the days that changed.
  CalendarEditor.prototype._lockDays = function () {
    var self = this, view = this.view, dates = this._selectedDates();
    if (!dates.length || this._paintBusy) return;
    this._paintBusy = true;
    var send = function (days, locked) {
      return Chronicle.apiFetch(view.apiBase + '/weather/days/lock', { method: 'POST', body: { days: days, locked: locked } }).then(function (resp) {
        if (!resp.ok) throw new Error('lock failed');
        return resp.json();
      });
    };
    this._snapshot(dates).then(function (snap) {
      // The snapshot's write shape drops the lock flag, so read the stored readings.
      snap.forEach(function (s) { s.prev = (view.weatherByYear[s.date.year] || {})[s.date.month + '_' + s.date.day] || null; });
      var have = snap.filter(function (s) { return s.prev; });
      var st = lockState(snap.map(function (s) { return s.prev; }));
      if (st.disabled) { view.say('The chosen days have no weather to lock.'); return; }
      var toggle = have.filter(function (s) { return (s.prev.locked === true) === st.unlocking; });
      var days = (toggle.length ? toggle : have).map(function (s) { return s.date; });
      var locked = !st.unlocking;
      return send(days, locked).then(function (out) {
        var n = out && typeof out.changed === 'number' ? out.changed : days.length;
        self._refreshWeather(days);
        view.say((locked ? 'Locked ' : 'Unlocked ') + (n === 1 ? '1 day' : n + ' days') + '.', { label: 'Undo', run: function () {
          if (self._paintBusy) return;
          self._paintBusy = true;
          send(days, !locked).then(function () { view.say('Undone.'); }).catch(function () {
            view.say("Couldn't undo. The calendar shows what is saved now.");
          }).then(function () { self._paintBusy = false; self._refreshWeather(days); });
        } });
      });
    }).catch(function () {
      view.say("Couldn't lock those days. Nothing changed.");
    }).then(function () {
      self._paintBusy = false;
    });
  };

  // _undoPaint clears the touched days, then writes back the readings they
  // held. Clearing first lets a generated reading return to a day just
  // painted by hand, which a generated write alone may not replace.
  CalendarEditor.prototype._undoPaint = function () {
    var self = this, view = this.view, snap = this._paintUndo;
    if (!snap || this._paintBusy) return;
    this._paintBusy = true;
    var dates = snap.map(function (s) { return s.date; });
    var restore = snap.filter(function (s) { return s.prev; }).map(function (s) { return s.prev; });
    Chronicle.apiFetch(view.apiBase + '/weather/days/clear', { method: 'POST', body: { days: dates } }).then(function (resp) {
      if (!resp.ok) throw new Error('undo clear failed');
      if (!restore.length) return resp;
      return Chronicle.apiFetch(view.apiBase + '/weather/days', { method: 'PUT', body: { days: restore } });
    }).then(function (resp) {
      if (!resp.ok) throw new Error('undo restore failed');
      // Painting over a locked day clears its lock; Undo puts that back too.
      var relock = snap.filter(function (s) { return s.prev && s.locked; }).map(function (s) { return s.date; });
      if (!relock.length) return resp;
      return Chronicle.apiFetch(view.apiBase + '/weather/days/lock', { method: 'POST', body: { days: relock, locked: true } });
    }).then(function (resp) {
      if (!resp.ok) throw new Error('undo relock failed');
      self._paintUndo = null;
      view.say('Undone.');
    }).catch(function () {
      view.say("Couldn't undo. The calendar shows what is saved now.");
    }).then(function () {
      self._paintBusy = false;
      self._refreshWeather(dates);
    });
  };

  // _openWeather opens the weather calendar on the month of the first chosen
  // day (or the shown month), once the generator, the calendar's weather
  // settings and that month's weather and events have loaded.
  CalendarEditor.prototype._openWeather = function () {
    var self = this, view = this.view, dates = this._selectedDates();
    if (this._paintBusy) return;
    var CD = CalDate, cal = view.cal;
    dates.sort(function (a, b) { return CD.dayIndex(cal, a.year, a.month, a.day) - CD.dayIndex(cal, b.year, b.month, b.day); });
    var y = dates.length ? dates[0].year : view.view.y, m = dates.length ? dates[0].month : view.view.m;
    var mc = CD.monthCount(cal);
    m = ((m - 1) % mc + mc) % mc + 1;
    var chosen = dates.filter(function (d) { return d.year === y && d.month === m; }).map(function (d) { return d.year + '_' + d.month + '_' + d.day; });
    this._closeShiftTray();
    var fail = function () { view.say('The weather generator could not load. Check your connection and try again.'); };
    loadEngine(view.engineSrc, function () {
      Promise.all([view.fetchWeatherYear(y), view.fetchMonth(y, m), Chronicle.calendarWeatherSettings(view)]).then(function (got) {
        if (!view.weatherByYear[y]) { view.say("Couldn't load the weather already on those days. Try again."); return; }
        Chronicle.calendarWeatherSheet.open(view, {
          year: y, month: m, chosen: chosen, settings: got[2],
          save: function (changes) { return self._saveWeather(changes); }
        });
      });
    }, fail);
  };

  // _saveWeather writes the weather calendar's changes (clears, then
  // readings, then locks) and offers Undo. Resolves true once all are saved;
  // on a failure part way it reloads what the server holds and resolves
  // false, so the panel stays open with its draft.
  CalendarEditor.prototype._saveWeather = function (ch) {
    var self = this, view = this.view;
    var key = function (d) { return d.year + '_' + d.month + '_' + d.day; }, seen = {}, dates = [];
    ch.clear.concat(ch.put, ch.lock, ch.unlock).forEach(function (d) {
      var k = key(d);
      if (!seen[k]) { seen[k] = true; dates.push({ year: d.year, month: d.month, day: d.day }); }
    });
    if (!dates.length) return Promise.resolve(true);
    // The per-day weather API takes at most 1000 days per call, and a draft
    // can span many months, so each list goes in batches of that size.
    var call = function (path, method, list, extra) {
      var at = 0;
      var next = function () {
        if (at >= list.length) return null;
        var body = { days: list.slice(at, at + 1000) };
        at += 1000;
        Object.keys(extra || {}).forEach(function (k) { body[k] = extra[k]; });
        return Chronicle.apiFetch(view.apiBase + path, { method: method, body: body }).then(function (resp) {
          if (!resp.ok) throw new Error('weather save failed');
          return next();
        });
      };
      return next();
    };
    this._paintBusy = true;
    var snap = null;
    return this._snapshot(dates).then(function (s) {
      snap = s;
      return call('/weather/days/clear', 'POST', ch.clear);
    }).then(function () {
      return call('/weather/days', 'PUT', ch.put);
    }).then(function () {
      return call('/weather/days/lock', 'POST', ch.lock, { locked: true });
    }).then(function () {
      return call('/weather/days/lock', 'POST', ch.unlock, { locked: false });
    }).then(function () {
      self._paintUndo = snap;
      self._refreshWeather(dates);
      view.say('Saved the weather on ' + (dates.length === 1 ? '1 day' : dates.length + ' days') + '.', { label: 'Undo', run: function () { self._undoPaint(); } });
      return true;
    }).catch(function () {
      if (snap) self._refreshWeather(dates);
      view.say(snap ? "Some of the weather couldn't be saved. The calendar shows what is saved now." : "Couldn't save the weather. Nothing changed.");
      return false;
    }).then(function (ok) {
      self._paintBusy = false;
      return ok;
    });
  };

  // ------------------------------------------------------------------
  // Day wing: bind the "Add event" button calendar_view.js already
  // rendered (it knows CanEdit from its own config, independent of this
  // file loading at all) — the event-editing entry point itself lives in
  // the detail overlay (evp) below, not a second control here.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._bindWingCapture = function () {
    var self = this, view = this.view;
    view.wingEl.addEventListener('click', function (e) {
      if (e.target.closest('[data-add-event]')) {
        e.stopPropagation();
        // New events open in the full drawer like edits do, so times, an
        // end date and repeats are there from the start; the compact form
        // is only the fallback if the drawer's script did not load.
        if (Chronicle.calendarEventDrawer) {
          var d = view.wingFor.split('_').map(Number);
          Chronicle.calendarEventDrawer.open(view, null, { y: d[0], m: d[1], d: d[2] });
        } else self._openEventForm(view.wingFor, null);
      }
    }, true);
  };

  // ------------------------------------------------------------------
  // Event detail overlay: adds an Edit / Delete footer for a viewer who
  // may write this event. Delete is Owner-only (routes.go), independent
  // of the broader canEdit/canAuthorDmOnly gate — see the file's gating
  // note at the top.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._wrapEvpRendering = function () {
    var self = this, view = this.view, original = view._evpHTML.bind(view);
    view._evpHTML = function (ev) {
      var html = original(ev);
      var canDelete = view.role >= ROLE_OWNER;
      var foot = self._occurrenceHTML(ev) + '<div class="efoot"><button type="button" class="btn quiet" data-edit-event="' + esc(ev.id) + '"><i class="fa-solid fa-pencil"></i> Edit</button><span class="sp"></span>' +
        (canDelete ? '<button type="button" class="btn danger" data-delete-event="' + esc(ev.id) + '"><i class="fa-solid fa-trash"></i> Delete</button>' : '') + '</div>';
      return html.replace('<!--EVP_FOOT-->', foot);
    };
  };

  // "Move this one / Skip this one / Undo" for the date the card was opened
  // on. An override is keyed by the date the series put the occurrence on,
  // which for a moved one is where it came from, so that date is carried on
  // the block (data-oy/om/od) and every request uses it.
  CalendarEditor.prototype._occurrenceHTML = function (ev) {
    var view = this.view, day = view.wingFor && view.wingFor.split('_').map(Number);
    if (!day || !CalDate.hasExpansion(ev)) return '';
    var occ = CalDate.occurrenceOn(ev, day[0], day[1], day[2]);
    if (!occ) return '';
    var key = occ.moved_from ? { y: occ.moved_from.year, m: occ.moved_from.month, d: occ.moved_from.day } : { y: day[0], m: day[1], d: day[2] };
    var note = occ.skipped ? 'Skipped on this date. Players don’t see it.'
      : (occ.moved_from ? 'Moved here from ' + view._dateLabel(key.y, key.m, key.d) + '.' : 'Change just this date. The others stay as they are.');
    var buttons = occ.skipped
      ? '<button type="button" class="btn" data-occ="undo">Undo</button>'
      : '<button type="button" class="btn" data-occ="move">Move this one</button>' +
        '<button type="button" class="btn" data-occ="skip">Skip this one</button>' +
        (occ.moved_from ? '<button type="button" class="btn quiet" data-occ="undo">Undo</button>' : '');
    var edit = ev.recurrence_type === 'rule' ? '<span class="sp"></span><button type="button" class="btn quiet" data-edit-event="' + esc(ev.id) + '">Edit the rule</button>' : '';
    var mo = (view.cal.months || []).map(function (m, i) {
      return '<option value="' + (i + 1) + '"' + (i + 1 === day[1] ? ' selected' : '') + '>' + esc(m.name) + '</option>';
    }).join('');
    return '<div class="occ" data-occ-ev="' + esc(ev.id) + '" data-oy="' + key.y + '" data-om="' + key.m + '" data-od="' + key.d + '">' +
      '<div class="oct">' + esc(note) + '</div>' +
      '<div class="orow">' + buttons + edit + '</div>' +
      '<div class="omv" hidden><select data-occ-f="m" aria-label="Move to month">' + mo + '</select>' +
        '<select data-occ-f="d" aria-label="Move to day">' + this._dayOptions(day[0], day[1], day[2]) + '</select>' +
        '<input type="number" data-occ-f="y" aria-label="Move to year" value="' + day[0] + '">' +
        '<button type="button" class="btn primary" data-occ="go">Move</button><button type="button" class="btn quiet" data-occ="cancel">Cancel</button></div>' +
      '<div class="ost" role="status"></div></div>';
  };

  CalendarEditor.prototype._dayOptions = function (y, m, sel) {
    var n = CalDate.monthDays(this.view.cal, m - 1, y), h = '';
    for (var d = 1; d <= n; d++) h += '<option value="' + d + '"' + (d === sel ? ' selected' : '') + '>' + d + '</option>';
    return h;
  };

  CalendarEditor.prototype._occurrenceAction = function (btn) {
    var self = this, view = this.view, box = btn.closest('.occ'), act = btn.dataset.occ;
    var st = $('.ost', box), mv = $('.omv', box);
    var say = function (msg, bad) { st.textContent = msg; st.classList.toggle('err', !!bad); };
    if (act === 'move') {
      mv.hidden = false;
      say('Pick the date for just this one. The other dates stay as they are.');
      $('[data-occ-f="m"]', mv).focus();
      return;
    }
    if (act === 'cancel') { mv.hidden = true; say(''); $('[data-occ="move"]', box).focus(); return; }
    var url = view.apiBase + '/events/' + encodeURIComponent(box.dataset.occEv) + '/occurrences/' + box.dataset.oy + '/' + box.dataset.om + '/' + box.dataset.od;
    var req, done;
    if (act === 'skip') {
      req = { method: 'PUT', body: { action: 'skip' } };
      done = 'This one is skipped. It shows struck through for you, and players don’t see it.';
    } else if (act === 'undo') {
      req = { method: 'DELETE' };
      done = 'Back on its usual date.';
    } else if (act === 'go') {
      var y = parseInt($('[data-occ-f="y"]', mv).value, 10), m = +$('[data-occ-f="m"]', mv).value, d = +$('[data-occ-f="d"]', mv).value;
      if (isNaN(y)) { say('Write the year as a number.', true); return; }
      req = { method: 'PUT', body: { action: 'move', year: y, month: m, day: d } };
      done = 'Moved to ' + view._dateLabel(y, m, d) + '. The other dates stay as they are.';
    } else return;
    var buttons = $$('button', box);
    buttons.forEach(function (b) { b.disabled = true; });
    say('');
    Chronicle.apiFetch(url, req).then(function (resp) {
      if (resp.ok) {
        var dayKey = view.wingFor;
        view.eventsByMonth = {};
        view.closeEventDetail();
        view.renderMonth();
        view.say(done);
        // Back to the day the card was for, once the month is redrawn.
        view.fetchMonth(view.view.y, view.view.m).then(function () {
          var cell = dayKey && view.stageEl.querySelector('.day[data-key="' + dayKey + '"]');
          var row = view.wingEl.querySelector('.evd');
          setTimeout(function () { var t = row || cell; if (!t) return; if (!t.hasAttribute('tabindex')) t.setAttribute('tabindex', '-1'); t.focus({ preventScroll: true }); }, reducedMotionNow() ? 0 : 400);
        });
        return;
      }
      buttons.forEach(function (b) { b.disabled = false; });
      return resp.json().then(function (j) { say((j && (j.message || j.error)) || 'That didn’t save. Try again.', true); },
        function () { say('That didn’t save. Try again.', true); });
    }, function () {
      buttons.forEach(function (b) { b.disabled = false; });
      say('That didn’t save. Check your connection and try again.', true);
    });
  };

  CalendarEditor.prototype._bindEvpCapture = function () {
    var self = this, view = this.view;
    // Days of the month being moved to follow its month and year.
    view.evpEl.addEventListener('change', function (e) {
      var t = e.target;
      if (!t.dataset || (t.dataset.occF !== 'm' && t.dataset.occF !== 'y')) return;
      var mv = t.closest('.omv'), dSel = $('[data-occ-f="d"]', mv);
      var y = parseInt($('[data-occ-f="y"]', mv).value, 10) || view.cal.current_year, m = +$('[data-occ-f="m"]', mv).value;
      var keep = +dSel.value;
      dSel.innerHTML = self._dayOptions(y, m, keep);
    });
    view.evpEl.addEventListener('click', function (e) {
      var oc = e.target.closest('[data-occ]');
      if (oc) { e.stopPropagation(); self._occurrenceAction(oc); return; }
      var ed = e.target.closest('[data-edit-event]');
      var del = e.target.closest('[data-delete-event]');
      if (ed) {
        e.stopPropagation();
        var ev = view._findEvent(ed.dataset.editEvent);
        // The full drawer edits every field; the compact form stays as the
        // fallback if its script did not load.
        if (Chronicle.calendarEventDrawer) Chronicle.calendarEventDrawer.open(view, ev, null);
        else self._openEventForm(view.wingFor, ev);
      }
      if (del) {
        e.stopPropagation();
        if (!window.confirm('Delete this event? This cannot be undone.')) return;
        Chronicle.apiFetch(view.apiBase + '/events/' + del.dataset.deleteEvent, { method: 'DELETE' }).then(function (resp) {
          if (!resp.ok) { view.say('Could not delete the event.'); return; }
          view.eventsByMonth = {};
          view.closeEventDetail();
          view.renderMonth();
          view.refreshWing();
          view.say('Event deleted.');
        });
      }
    }, true);
  };

  // ------------------------------------------------------------------
  // Event create/edit form: a compact inline form appended into the wing,
  // grouped like the mockup's editor (name, date, kind incl.
  // new-kind-in-place, visibility) without the mockup's full When/Details/
  // Who/More sectioning — the fields that map onto real Chronicle fields,
  // nothing invented.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._openEventForm = function (dayKey, existing) {
    var self = this, view = this.view;
    var d = dayKey.split('_').map(Number);
    var kinds = view.cal.event_kinds || [];
    var isNew = !existing;
    // One form at a time: pressing for the same one again brings it back
    // into view rather than stacking a second copy under it.
    var target = existing ? String(existing.id) : '';
    var already = view.wingEl.querySelector('form.ebody');
    if (already && already.dataset.eventId === target) {
      already.name.focus({ preventScroll: true });
      reveal(already);
      return;
    }
    if (already) already.remove();
    var form = document.createElement('form');
    form.className = 'ebody';
    form.dataset.eventId = target;
    form.style.marginTop = '10px';
    form.innerHTML =
      '<div class="fld"><input class="etitle" name="name" placeholder="Event name" required value="' + esc(existing ? existing.name : '') + '"/></div>' +
      '<div class="fld"><textarea name="description" rows="2" placeholder="Optional description">' + esc(descriptionText(existing)) + '</textarea></div>' +
      '<div class="fld">Kind<div class="chips" id="cal5-edkinds">' + this._kindChipsHTML(kinds, existing ? existing.kind_id : null) + '</div></div>' +
      (view.canAuthorDmOnly ? '<div id="cal5-nkf-mount"></div>' : '') +
      (view.canAuthorDmOnly ? '<div class="vis" role="group" aria-label="Visibility">' +
        '<button type="button" data-vis="everyone" aria-pressed="' + (!existing || existing.visibility !== 'dm_only') + '">Everyone</button>' +
        '<button type="button" data-vis="dm_only" aria-pressed="' + (!!existing && existing.visibility === 'dm_only') + '">Director only</button></div>' : '') +
      '<div class="efoot"><button type="button" class="btn quiet" data-cancel>Cancel</button>' +
        '<span class="sp"></span><button type="submit" class="btn primary">' + (isNew ? 'Create' : 'Save') + '</button></div>';

    form.dataset.kindId = existing && existing.kind_id != null ? String(existing.kind_id) : '';
    form.dataset.visibility = existing ? existing.visibility : 'everyone';

    form.addEventListener('click', function (e) {
      if (e.target.closest('[data-cancel]')) { form.remove(); return; }
      var chip = e.target.closest('[data-kind]');
      if (chip) { form.dataset.kindId = chip.dataset.kind; $$('#cal5-edkinds [data-kind]', form).forEach(function (b) { b.setAttribute('aria-pressed', String(b === chip)); }); }
      var vis = e.target.closest('[data-vis]');
      if (vis) { form.dataset.visibility = vis.dataset.vis; $$('.vis [data-vis]', form).forEach(function (b) { b.setAttribute('aria-pressed', String(b === vis)); }); }
      var nk = e.target.closest('[data-nkb]');
      if (nk) self._openNewKindForm(form);
    });

    form.addEventListener('submit', function (e) {
      e.preventDefault();
      // description and description_html always move together: description
      // stays the honest plain text this textarea can actually edit, and
      // description_html is rebuilt from that same text so the view (which
      // prefers description_html — calendar_view.js's _evpHTML/showGlance)
      // renders what was typed instead of a stale rich-text render left over
      // from before this save (description_html is patch.Field, so leaving
      // it out of the body would PRESERVE that old HTML, not clear it —
      // UpdateEventInput's doc comment, model.go).
      var descText = form.description.value;
      var body = {
        name: form.name.value.trim(),
        description: descText || null,
        description_html: descText ? bodyToHTML(descText) : null,
        kind_id: form.dataset.kindId ? +form.dataset.kindId : null
      };
      // This compact form has no date or time picker, so it must never
      // resend year/month/day/all_day on an EDIT — a PUT is a partial
      // update (patch.Field): omitting a key preserves the stored value,
      // while resending today's wing day or a hardcoded all_day:true would
      // silently move the event (wrong for a recurring/spanning event
      // viewed from an occurrence other than its own base date) or flip a
      // timed event to all-day on its very first edit through this UI. A
      // brand-new event has no stored date yet, so it takes the day whose
      // wing it was created from and is all-day (the only mode this form
      // supports until a time picker exists).
      if (isNew) {
        body.year = d[0]; body.month = d[1]; body.day = d[2];
        body.all_day = true;
      }
      if (view.canAuthorDmOnly) body.visibility = form.dataset.visibility;
      var req;
      if (isNew) req = Chronicle.apiFetch(view.apiBase + '/events', { method: 'POST', body: body });
      else req = Chronicle.apiFetch(view.apiBase + '/events/' + existing.id, { method: 'PUT', body: body });
      req.then(function (resp) {
        if (!resp.ok) { view.say('Could not save the event.'); return; }
        view.eventsByMonth = {};
        form.remove();
        if (!isNew) view.closeEventDetail();
        view.renderMonth();
        view.refreshWing();
        view.say(isNew ? 'Event created.' : 'Event saved.');
      });
    });

    // Right under "Add an event", ahead of the day's moons.
    var addBtn = view.wingEl.querySelector('[data-add-event]');
    if (addBtn) (addBtn.closest('.rtd-open') || addBtn).insertAdjacentElement('afterend', form);
    else (view.wingEl.querySelector('.wbr') || view.wingEl.querySelector('.wb')).appendChild(form);
    form.name.focus({ preventScroll: true });
    reveal(form);
  };

  CalendarEditor.prototype._kindChipsHTML = function (kinds, activeId) {
    var html = kinds.map(function (k) {
      var color = sanitizeColor(k.color);
      var style = color ? ('color:' + color + ';') : '';
      return '<button type="button" data-kind="' + k.id + '" aria-pressed="' + (k.id === activeId) + '" style="' + style + '">' + esc(k.icon || '') + ' ' + esc(k.name) + '</button>';
    }).join('');
    if (this.view.canAuthorDmOnly) html += '<button type="button" class="nkb" data-nkb><i class="fa-solid fa-plus"></i> New kind</button>';
    return html;
  };

  // New-kind-in-place: an inline form (name/icon/colour), POSTs
  // /calendars/event-kinds (campaign-scoped, gated CanAuthorDmOnly —
  // routes.go — so this entry point itself is only ever rendered for
  // canAuthorDmOnly, see _kindChipsHTML above).
  CalendarEditor.prototype._openNewKindForm = function (parentForm) {
    var self = this, view = this.view, mount = $('#cal5-nkf-mount', parentForm);
    if (!mount || mount.querySelector('.nkf')) return;
    var swatches = ['#ef4444', '#f97316', '#84cc16', '#22c55e', '#06b6d4', '#3b82f6', '#8b5cf6', '#ec4899'];
    mount.innerHTML = '<div class="nkf">' +
      '<div class="nkt">New kind of event</div>' +
      '<div class="fld"><input id="cal5-nk-name" placeholder="Name" required/></div>' +
      '<div class="fld"><input id="cal5-nk-icon" placeholder="Icon (a short glyph, e.g. ⚔)" maxlength="8"/></div>' +
      '<div class="swp" role="group" aria-label="Colour">' + swatches.map(function (c, i) {
        return '<button type="button" data-sw="' + c + '" style="--sw:' + c + '" aria-checked="' + (i === 0) + '"></button>';
      }).join('') + '</div>' +
      '<div class="nkfoot"><button type="button" class="btn quiet" data-nk-cancel>Cancel</button><button type="button" class="btn primary" data-nk-save>Add kind</button></div>' +
      '</div>';
    mount.dataset.color = swatches[0];
    reveal(mount);
    mount.addEventListener('click', function (e) {
      if (e.target.closest('[data-nk-cancel]')) { mount.innerHTML = ''; return; }
      var sw = e.target.closest('[data-sw]');
      if (sw) { mount.dataset.color = sw.dataset.sw; $$('.swp button', mount).forEach(function (b) { b.setAttribute('aria-checked', String(b === sw)); }); }
      if (e.target.closest('[data-nk-save]')) self._saveNewKind(mount, parentForm);
    });
  };

  CalendarEditor.prototype._saveNewKind = function (mount, parentForm) {
    var view = this.view;
    var name = $('#cal5-nk-name', mount).value.trim();
    if (!name) { $('#cal5-nk-name', mount).focus(); return; }
    var slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'kind';
    var icon = $('#cal5-nk-icon', mount).value.trim() || '●';
    Chronicle.apiFetch('/campaigns/' + view.campaignId + '/calendars/event-kinds', {
      method: 'POST',
      body: { slug: slug, name: name, icon: icon, color: mount.dataset.color, sort_order: (view.cal.event_kinds || []).length, default_announced: 'on_day' }
    }).then(function (resp) {
      if (!resp.ok) { view.say('Could not create the kind (the slug may already exist).'); return; }
      return resp.json();
    }).then(function (kind) {
      if (!kind) return;
      view.cal.event_kinds = (view.cal.event_kinds || []).concat([kind]);
      mount.innerHTML = '';
      var chipsEl = $('#cal5-edkinds', parentForm);
      if (chipsEl) chipsEl.outerHTML = '<div class="chips" id="cal5-edkinds">' + this._kindChipsHTML(view.cal.event_kinds, kind.id) + '</div>';
      parentForm.dataset.kindId = String(kind.id);
      view.renderLegend();
      view.say('Added the kind “' + name + '”.');
    }.bind(this));
  };

  // ------------------------------------------------------------------
  // Moon hidden toggle, added to the moon view's strip. Gated
  // CanAuthorDmOnly (routes.go: PUT .../moons/:id/hidden), same as
  // event-kind creation and era management below.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._wrapMvRendering = function () {
    var self = this, view = this.view, original = view._mvHTML.bind(view);
    view._mvHTML = function () {
      var html = original();
      if (!view.canAuthorDmOnly) return html;
      var moon = view.moonById(view.mvMoonId) || (view.cal.moons || [])[0];
      if (!moon) return html;
      var toggle = '<div class="sw" style="margin-top:10px"><span>Shown to players<small>Hide a moon while it is still a secret.</small></span>' +
        '<button type="button" class="tog" role="switch" aria-checked="' + !moon.hidden_from_players + '" data-toggle-moon="' + moon.id + '"></button></div>';
      return html.replace('<div class="mvstrip">', toggle + '<div class="mvstrip">');
    };
  };

  CalendarEditor.prototype._bindMvCapture = function () {
    var self = this, view = this.view;
    view.mvEl.addEventListener('click', function (e) {
      var t = e.target.closest('[data-toggle-moon]');
      if (!t) return;
      e.stopPropagation();
      var moon = view.moonById(t.dataset.toggleMoon);
      if (!moon) return;
      var hidden = !moon.hidden_from_players;
      Chronicle.apiFetch(view.apiBase + '/moons/' + moon.id + '/hidden', { method: 'PUT', body: { hidden: hidden } }).then(function (resp) {
        if (!resp.ok) { view.say('Could not change the moon’s visibility.'); return; }
        moon.hidden_from_players = hidden;
        view._refreshMv();
        view.say(moon.name + (hidden ? ' hidden from players.' : ' shown to players.'));
      });
    }, true);
  };

  // ------------------------------------------------------------------
  // Era manager: the one piece of the mockup's "structure editor" (months,
  // weekdays, leap rule, moons, eras) whose write API actually exists today
  // (routes.go: POST/PUT/DELETE .../eras[/:eraID], gated CanAuthorDmOnly).
  // Months, weekdays, the leap-year rule and a moon's own name/cycle/color
  // have NO write route at all — see the plugin's .ai.md "Honest gaps";
  // building a UI for those would write into nothing. So the flap becomes a
  // real list-manage-add editor for canAuthorDmOnly (Owner or a granted
  // co-Director); everyone else keeps calendar_view.js's read-only
  // current-era flap.
  // ------------------------------------------------------------------
  CalendarEditor.prototype._wrapFlapRendering = function () {
    var self = this, view = this.view;
    if (!view.canAuthorDmOnly) return;
    this._eraFormFor = undefined; // undefined = list mode, 'new' = create, <id> = edit
    view.showFlap = function () {
      self._eraFormFor = undefined;
      view._showFlapHTML(self._eraManagerHTML());
    };
    // The era panel's "Edit eras" lands here: edit mode on, then the
    // manager folds out of the strip's Eras button once it is showing.
    view.openEraManager = function () {
      if (!self.editing) self.setEditing(true);
      requestAnimationFrame(function () { if (view._pf.state === 'closed') view.showFlap(); });
    };
  };

  CalendarEditor.prototype._refreshFlap = function () {
    var view = this.view, flap = view.flapEl;
    flap.innerHTML = this._eraManagerHTML();
    // The card may grow for the form, but only to the calendar's own
    // bottom edge: past it the card would cover the page below. What does
    // not fit scrolls inside, under the fixed Save/Cancel bar.
    if (!window.matchMedia('(max-width:600px)').matches && flap.style.top) {
      var top = parseFloat(flap.style.top) || 0, lim = view.calEl.clientHeight - top - 8;
      var room = view._pf && view._pf.g ? view._pf.g.final.room : lim;
      flap.style.maxHeight = Math.max(220, Math.min(room, lim)) + 'px';
    }
    // The open era's tab is brought into the row's view.
    var tab = flap.querySelector('.eratab[aria-pressed="true"]');
    if (tab) {
      var row = tab.parentNode, over = tab.offsetLeft + tab.offsetWidth + 24 - row.clientWidth;
      if (over > 0) row.scrollLeft = over;
    }
  };

  // eraRangeLabel is an era's span as the manager lists it: where it
  // really ends, which for an era with no end of its own is the day before
  // the next era begins.
  function eraRangeLabel(cal, r) {
    var mo = function (t) { var m = (cal.months || [])[t.m - 1]; return m ? m.name : 'Month ' + t.m; };
    var today = CalDate.dayIndex(cal, cal.current_year, cal.current_month, cal.current_day);
    if (!r.end) return r.startK > today ? 'From ' + mo(r.start) + ' ' + r.start.y : r.start.y + '–present';
    if (r.start.y !== r.end.y) return r.start.y + '–' + r.end.y;
    if (r.start.m !== r.end.m) return mo(r.start) + ' – ' + mo(r.end) + ' ' + r.end.y;
    return r.start.d + '–' + r.end.d + ' ' + mo(r.end) + ' ' + r.end.y;
  }

  CalendarEditor.prototype._eraManagerHTML = function () {
    var view = this.view, cal = view.cal;
    var rows = Chronicle.calendarEras.sorted(cal);
    var current = view.eraForDate(view.view.y, view.view.m, 1);
    var formFor = this._eraFormFor, editing = formFor !== undefined;
    var hid = function (e) { return e.hidden_until_begins ? ' <i class="fa-solid fa-eye-slash eh" title="Hidden from players until it begins" aria-label="Hidden from players until it begins"></i>' : ''; };
    var body, foot = '';
    if (!editing) {
      // The eras, oldest first; a row opens that era's form.
      body = rows.length ? '<ul class="eral">' + rows.map(function (r) {
        var e = r.era;
        return '<li><button type="button" class="erali' + (current && e.id === current.id ? ' cur' : '') + '" data-edit-era="' + esc(e.id) + '" aria-label="Edit ' + esc(e.name) + '">' +
          '<i class="esw" style="' + view._eraSwatch(e) + '"></i><span class="en">' + esc(e.name) + hid(e) + '<small>' + esc(eraRangeLabel(cal, r)) + '</small></span>' +
          '<span class="lg" aria-hidden="true">›</span></button></li>';
      }).join('') + '</ul>' : '<p class="none">No eras yet.</p>';
      body += '<button type="button" class="addev" data-add-era><i class="fa-solid fa-plus"></i>Add an era</button>';
      // Colours, style and feel are set in the Era look part of the calendar
      // settings, which is Owner only like the rest of that page.
      if (view.role >= ROLE_OWNER && view.campaignId && view.calendarId) {
        body += '<a class="addev" href="/campaigns/' + encodeURIComponent(view.campaignId) + '/calendars/' + encodeURIComponent(view.calendarId) + '/structure#era-look"><i class="fa-solid fa-palette"></i>Colours and feel: Era look</a>';
      }
    } else {
      // While one era is open, the others stay one tap away in a row that
      // scrolls sideways.
      var row = formFor === 'new' ? null : rows.filter(function (r) { return String(r.era.id) === String(formFor); })[0];
      var era = row ? row.era : null;
      body = '<div class="eratabs" role="group" aria-label="Eras">' + rows.map(function (r) {
        var e = r.era, on = String(e.id) === String(formFor);
        return '<button type="button" class="eratab" data-edit-era="' + esc(e.id) + '" aria-pressed="' + on + '">' +
          '<i class="esw" style="' + view._eraSwatch(e) + '"></i><span>' + esc(e.name) + hid(e) + '<small>' + esc(eraRangeLabel(cal, r)) + '</small></span></button>';
      }).join('') +
        '<button type="button" class="eratab add" data-add-era aria-pressed="' + (formFor === 'new') + '"><span>+ New era</span></button></div>' +
        this._eraFormHTML(era, row && row.end);
      foot = '<div class="efoot erafoot"><button type="button" class="btn quiet" data-cancel-era>Cancel</button><span class="sp"></span>' +
        (era ? '<button type="button" class="btn danger" data-del-era="' + esc(era.id) + '">Delete</button>' : '') +
        '<button type="submit" class="btn primary" form="cal5-eraform">' + (era ? 'Save' : 'Create') + '</button></div>';
    }

    // Two leaves, as calendar_view.js's era card: the bar, then the list
    // or the form, which scroll above the form's fixed buttons.
    return '<div class="leaf lf1"><div class="grab" aria-hidden="true"></div>' +
        '<div class="crease"><span>Eras</span><button type="button" class="x" data-close aria-label="Close">✕</button></div></div>' +
      '<div class="leaf lf2"><div class="lscroll"><div class="fb fbr eram">' + body + '</div></div>' + foot + '</div>';
  };

  // eraDateFields is a Starts or Ends row: month and day as lists of this
  // calendar's own months and days, then the year, as the event editor's
  // date rows are.
  CalendarEditor.prototype._eraDateFields = function (p, t, label) {
    var cal = this.view.cal, months = cal.months || [];
    var yr = '<input type="number" name="' + p + '_year" aria-label="' + label + ' year" value="' + esc(t ? t.y : '') + '"' + (p === 'start' ? ' required' : '') + '/>';
    if (!months.length) {
      return '<input type="number" name="' + p + '_month" min="1" aria-label="' + label + ' month" value="' + esc(t ? t.m : 1) + '"/>' +
        '<input type="number" name="' + p + '_day" min="1" aria-label="' + label + ' day" value="' + esc(t ? t.d : 1) + '"/>' + yr;
    }
    var m = t ? t.m : 1;
    return '<select name="' + p + '_month" aria-label="' + label + ' month">' + months.map(function (mo, i) {
      return '<option value="' + (i + 1) + '"' + (i + 1 === m ? ' selected' : '') + '>' + esc(mo.name) + '</option>';
    }).join('') + '</select>' +
      '<select name="' + p + '_day" aria-label="' + label + ' day">' + this._eraDayOptions(t ? t.y : cal.current_year, m, t ? t.d : 1) + '</select>' + yr;
  };
  CalendarEditor.prototype._eraDayOptions = function (y, m, sel) {
    var n = CalDate.monthDays(this.view.cal, m - 1, y) || 30, h = '';
    sel = Math.min(sel || 1, n);
    for (var d = 1; d <= n; d++) h += '<option value="' + d + '"' + (d === sel ? ' selected' : '') + '>' + d + '</option>';
    return h;
  };
  // A month or year change keeps the day list to that month's length.
  CalendarEditor.prototype._eraDaysFor = function (form, p) {
    var m = form[p + '_month'], d = form[p + '_day'], y = form[p + '_year'];
    if (!m || !d || d.tagName !== 'SELECT') return;
    d.innerHTML = this._eraDayOptions(+y.value || this.view.cal.current_year, +m.value || 1, +d.value || 1);
  };

  // until is where the era ends now (its own end, or the day before the next
  // era begins): what the Ends row starts at when Ongoing is switched off.
  CalendarEditor.prototype._eraFormHTML = function (era, until) {
    var cal = this.view.cal, ongoing = !era || era.end_year == null;
    var start = era ? { y: era.start_year, m: era.start_month || 1, d: era.start_day || 1 } : { y: cal.current_year, m: cal.current_month || 1, d: cal.current_day || 1 };
    var end = null;
    if (era && era.end_year != null) {
      end = Chronicle.calendarEras.explicitEndOf(cal, era);
    }
    var tog = function (name, on, label) {
      return '<button type="button" class="tog" role="switch" aria-checked="' + !!on + '" aria-label="' + label + '" data-etog="' + name + '"></button>';
    };
    return '<form class="eraform" id="cal5-eraform" novalidate>' +
      '<label class="fld"><span class="sr">Era name</span><input class="etitle" name="name" placeholder="Era name" required maxlength="255" value="' + esc(era ? era.name : '') + '"/></label>' +
      '<section class="esec"><h4>When</h4>' +
        '<div class="erow ecap" aria-hidden="true"><span></span><span>Month</span><span>Day</span><span>Year</span></div>' +
        '<div class="erow"><span class="rl">Starts</span>' + this._eraDateFields('start', start, 'Start') + '</div>' +
        '<div class="swrow"><span>Ongoing<small>Runs until the next era begins.</small></span>' + tog('ongoing', ongoing, 'Ongoing') + '</div>' +
        '<div class="erow" data-end-fields' + (ongoing ? ' hidden' : '') + '><span class="rl">Ends</span>' + this._eraDateFields('end', end || until || start, 'End') + '</div>' +
      '</section>' +
      '<section class="esec"><h4>Details</h4>' +
        '<label class="fld">Description<textarea name="description" rows="2" placeholder="Optional. Shows in the era panel.">' + esc(era && era.description ? era.description : '') + '</textarea></label>' +
        '<div class="fld">Lore page<div class="lorepick" data-lore-pick>' + this._lorePickHTML(era ? era.lore_entity_id : null, era ? era.lore_entity_name : '') + '</div></div>' +
      '</section>' +
      '<section class="esec"><h4>Players</h4>' +
        '<div class="swrow"><span>Hide from players until it begins<small>Until then players see the era before it carry on, and nothing dated inside it.</small></span>' + tog('hidden', era && era.hidden_until_begins, 'Hide from players until it begins') + '</div>' +
        '<label class="fld">Director’s note<textarea name="dm_note" rows="2" placeholder="Only you and co-Directors ever see this">' + esc(era && era.dm_note ? era.dm_note : '') + '</textarea></label>' +
      '</section>' +
      '</form>';
  };

  // The lore page picker: a chosen page with Change/Remove, or a search box
  // over the campaign's pages. The chosen id rides on the picker itself.
  CalendarEditor.prototype._lorePickHTML = function (id, name) {
    if (id) {
      return '<input type="hidden" name="lore_entity_id" value="' + esc(id) + '"/>' +
        '<span class="lp-cur">' + (Chronicle.calendarPageGlyph || '') + esc(name || 'Linked page') + '</span>' +
        '<button type="button" class="btn sm quiet" data-lore-change>Change</button><button type="button" class="btn sm quiet" data-lore-clear>Remove</button>';
    }
    return '<input type="hidden" name="lore_entity_id" value=""/>' +
      '<input type="search" class="lp-q" data-lore-q placeholder="Search pages to link" aria-label="Search pages to link" autocomplete="off"/>' +
      '<ul class="lp-res" data-lore-res role="listbox" aria-label="Pages" hidden></ul>';
  };

  CalendarEditor.prototype._loreSearch = function (input) {
    var view = this.view, q = input.value.trim(), res = input.parentNode.querySelector('[data-lore-res]'), seq = (this._loreSeq || 0) + 1;
    this._loreSeq = seq;
    if (q.length < 2) { res.hidden = true; res.innerHTML = ''; return; }
    var self = this;
    Chronicle.apiFetch('/campaigns/' + encodeURIComponent(view.campaignId) + '/entities/search?q=' + encodeURIComponent(q))
      .then(function (resp) { return resp.ok ? resp.json() : { results: [] }; })
      .then(function (body) {
        if (self._loreSeq !== seq) return;
        // Only pages: the search also returns other kinds of result.
        var pages = (body.results || []).filter(function (r) { return r.id && /\/entities\//.test(r.url || ''); }).slice(0, 8);
        res.innerHTML = pages.length
          ? pages.map(function (r) { return '<li><button type="button" role="option" data-lore-id="' + esc(r.id) + '" data-lore-name="' + esc(r.name) + '">' + esc(r.name) + (r.type_name ? '<small>' + esc(r.type_name) + '</small>' : '') + '</button></li>'; }).join('')
          : '<li class="none">No pages match.</li>';
        res.hidden = false;
      });
  };

  CalendarEditor.prototype._bindFlapCapture = function () {
    var self = this, view = this.view;
    if (!view.canAuthorDmOnly) return;
    view.flapEl.addEventListener('click', function (e) {
      var add = e.target.closest('[data-add-era]');
      var edit = e.target.closest('[data-edit-era]');
      var del = e.target.closest('[data-del-era]');
      var cancel = e.target.closest('[data-cancel-era]');
      var pick = e.target.closest('[data-lore-id]');
      var lp = e.target.closest('[data-lore-pick]');
      if (pick && lp) { lp.innerHTML = self._lorePickHTML(pick.dataset.loreId, pick.dataset.loreName); return; }
      if (lp && e.target.closest('[data-lore-clear], [data-lore-change]')) {
        lp.innerHTML = self._lorePickHTML(null, '');
        var q = lp.querySelector('[data-lore-q]'); if (q) q.focus();
        return;
      }
      var tg = e.target.closest('[data-etog]');
      if (tg) {
        var on = tg.getAttribute('aria-checked') !== 'true';
        tg.setAttribute('aria-checked', String(on));
        if (tg.dataset.etog === 'ongoing') {
          var ef = view.flapEl.querySelector('[data-end-fields]');
          if (ef) ef.hidden = on;
        }
        return;
      }
      if (add) { self._eraFormFor = 'new'; self._refreshFlap(); }
      else if (edit) { self._eraFormFor = edit.dataset.editEra; self._refreshFlap(); }
      else if (cancel) { self._eraFormFor = undefined; self._refreshFlap(); }
      else if (del) {
        e.stopPropagation();
        if (!window.confirm('Delete this era? This cannot be undone.')) return;
        Chronicle.apiFetch(view.apiBase + '/eras/' + del.dataset.delEra, { method: 'DELETE' }).then(function (resp) {
          if (!resp.ok) { view.say('Could not delete the era.'); return; }
          view.cal.eras = (view.cal.eras || []).filter(function (er) { return String(er.id) !== String(del.dataset.delEra); });
          self._eraFormFor = undefined;
          self._refreshFlap();
          view.renderMonth();
          view.say('Era deleted.');
        });
      }
    });
    var loreTimer = 0;
    view.flapEl.addEventListener('input', function (e) {
      if (!e.target.matches('[data-lore-q]')) return;
      clearTimeout(loreTimer);
      loreTimer = setTimeout(function () { self._loreSearch(e.target); }, 220);
    });
    view.flapEl.addEventListener('change', function (e) {
      var m = /^(start|end)_(month|year)$/.exec(e.target.name || '');
      if (m && e.target.form) self._eraDaysFor(e.target.form, m[1]);
    });
    view.flapEl.addEventListener('submit', function (e) {
      var form = e.target.closest('#cal5-eraform');
      if (!form) return;
      e.preventDefault();
      var f = form;
      if (!f.name.value.trim() || !f.start_year.value) {
        view.say(!f.name.value.trim() ? 'Give the era a name.' : 'Give the era a start year.');
        (!f.name.value.trim() ? f.name : f.start_year).focus();
        return;
      }
      var isOn = function (k) { var t = f.querySelector('[data-etog="' + k + '"]'); return !!t && t.getAttribute('aria-checked') === 'true'; };
      var ongoing = isOn('ongoing');
      var body = {
        name: f.name.value.trim(),
        start_year: +f.start_year.value,
        start_month: +f.start_month.value || 1,
        start_day: +f.start_day.value || 1,
        end_year: ongoing ? null : (f.end_year.value ? +f.end_year.value : null),
        end_month: ongoing ? null : (f.end_month.value ? +f.end_month.value : null),
        end_day: ongoing ? null : (f.end_day.value ? +f.end_day.value : null),
        description: f.description.value || null,
        hidden_until_begins: isOn('hidden'),
        dm_note: f.dm_note.value.trim() || null
      };
      var isNew = self._eraFormFor === 'new';
      var prior = isNew ? null : (view.cal.eras || []).filter(function (er) { return String(er.id) === String(self._eraFormFor); })[0];
      var lore = f.lore_entity_id.value || null;
      // A lore page is sent only when it changed: the server checks a newly
      // chosen page, and an unchanged one should never fail that check.
      if (isNew || (prior && (prior.lore_entity_id || null) !== lore)) body.lore_entity_id = lore;
      if (isNew) {
        // A new era starts on the next suggested palette; the Era look
        // settings change it. Colours and order are left alone on edit.
        var pals = (Chronicle.calendarEraBlend && Chronicle.calendarEraBlend.PALETTES) || [['', '#8b5cf6', '#c4b5fd']];
        var pal = pals[(view.cal.eras || []).length % pals.length];
        body.color = pal[1];
        body.color_2 = pal[2];
        body.sort_order = (view.cal.eras || []).length;
      }
      var req = isNew
        ? Chronicle.apiFetch(view.apiBase + '/eras', { method: 'POST', body: body })
        : Chronicle.apiFetch(view.apiBase + '/eras/' + self._eraFormFor, { method: 'PUT', body: body });
      req.then(function (resp) {
        if (!resp.ok) {
          return resp.json().catch(function () { return {}; }).then(function (j) {
            view.say('Could not save the era' + (j && j.message ? ': ' + j.message : '.'));
            return false;
          });
        }
        return isNew ? resp.json() : true;
      }).then(function (created) {
        if (!created) return;
        if (isNew) {
          if (created.lore_entity_id) created.lore_entity_name = (f.querySelector('.lp-cur') || {}).textContent || '';
          view.cal.eras = (view.cal.eras || []).concat([created]);
        } else if (prior) {
          for (var k in body) prior[k] = body[k];
          if ('lore_entity_id' in body) prior.lore_entity_name = body.lore_entity_id ? (f.querySelector('.lp-cur') || {}).textContent || '' : '';
        }
        self._eraFormFor = undefined;
        self._refreshFlap();
        view.renderMonth();
        view.say(isNew ? 'Era created.' : 'Era saved.');
      });
    });
  };

  // Loaded unconditionally on every page (internal/app/routes.go's
  // pluginBodyScripts registry, outside the sidebar's hx-boost-swapped
  // region — see tools/check-page-scripts.sh for why a page templ can't
  // conditionally <script src> this instead), so THIS file — not the
  // Templ page — is what gates the whole editing surface on the viewer
  // actually being able to edit: no mount at all (most pages), or a mount
  // whose data-can-edit isn't "true" (a Player on the calendar page), both
  // no-op here, exactly as if the script had never loaded.
  //
  // MUST be the last thing in this file: calendar_view.js's widget mounts
  // (and sets mount.calendarView) synchronously during ITS OWN <script
  // defer>, which the pluginBodyScripts registry always emits before this
  // one — so by the time this line runs, mount.calendarView is already
  // set and `attach` fires immediately, not later via the
  // 'calendarv5:ready' event. Attaching before every CalendarEditor.
  // prototype method above is assigned would throw "editor.install is
  // not a function" the instant a canEdit viewer loads the page.
  // Pure helpers, exposed for test/js/calendar_paint_weather.test.mjs.
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { dayInput: dayInput, lockState: lockState, lockButtonHTML: lockButtonHTML };
  }

  var mount = document.querySelector('[data-widget="calendar_view"]');
  if (mount && mount.dataset.canEdit === 'true') {
    if (mount.calendarView) attach(mount.calendarView);
    else mount.addEventListener('calendarv5:ready', function (e) { attach(e.detail); });
  }
  // A calendar mounted after this script ran (one opened in place from its
  // card on the Calendars page) announces itself the same way. The event doesn't
  // bubble, but a capturing listener on the document still sees it.
  document.addEventListener('calendarv5:ready', function (e) {
    var m = e.target;
    if (m && m.dataset && m.dataset.canEdit === 'true') attach(e.detail);
  }, true);
})();
