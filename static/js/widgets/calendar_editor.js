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
    // gets the gear that leads to it.
    var view = this.view;
    var gear = view.role >= ROLE_OWNER && view.campaignId && view.calendarId
      ? '<a class="btn sm quiet" id="cal5-settings" href="/campaigns/' + encodeURIComponent(view.campaignId) + '/calendars/' + encodeURIComponent(view.calendarId) + '/structure" title="Calendar settings: months, weekdays, leap years, moons and seasons"><i class="fa-solid fa-gear"></i><span>Calendar settings</span></a>'
      : '';
    strip.innerHTML = '<span class="etag">Editing</span><span class="ehint">Click or drag across days. Shift extends, Ctrl/Cmd adds. Touch: tap, or hold then drag.</span><span class="sp"></span>' + gear + '<button type="button" class="btn sm quiet" id="cal5-editdone">Done</button>';
    subw.appendChild(strip);
    $('#cal5-editdone', strip).addEventListener('click', this.setEditing.bind(this, false));
  };

  CalendarEditor.prototype.setEditing = function (on) {
    this.editing = on;
    this.view.calEl.classList.toggle('editing', on);
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
        (Chronicle.calendarEventDrawer && Chronicle.calendarEventDrawer.isOpen(view));
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
  // selected. Only the two bulk actions this API can actually perform
  // ship: bulk visibility (Hide/Reveal, gated CanAuthorDmOnly, the same
  // gate the single-event visibility route already uses) and Shift events
  // (moves every event touching the selection by N days). The mockup's
  // Paint weather / Generate… / Lock actions are not wired yet
  // (TODO(#765)); the per-day weather endpoints exist, the UI does not.
  // ------------------------------------------------------------------
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
    var canVis = this.view.canAuthorDmOnly;
    this.bbar.hidden = false;
    this.bbar.innerHTML =
      '<div class="bbl"><b>' + (n === 1 ? '1 day' : n + ' days') + ' chosen</b><span>' + this._eventsInSelection().length + ' events touched</span></div>' +
      '<div class="bba">' +
        (canVis ? '<button type="button" class="bbb" data-bb="hide"><i class="fa-solid fa-eye-slash"></i><span>Hide</span></button><button type="button" class="bbb" data-bb="reveal"><i class="fa-solid fa-eye"></i><span>Reveal</span></button>' : '') +
        '<button type="button" class="bbb" data-bb="shift" aria-expanded="false"><i class="fa-solid fa-arrows-left-right"></i><span>Shift events</span></button>' +
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
  CalendarEditor.prototype._reportBulkResult = function (oks, verb, tail) {
    var view = this.view, okCount = oks.filter(Boolean).length, failCount = oks.length - okCount;
    view.eventsByMonth = {}; // simplest correct invalidation: refetch on next paint
    view.renderMonth();
    tail = tail || '';
    if (!failCount) {
      view.say(verb + ' ' + okCount + (okCount === 1 ? ' event' : ' events') + tail + '.');
    } else {
      view.say(verb + ' ' + okCount + ' of ' + oks.length + ' events' + tail + '; ' + failCount + " couldn't be saved.");
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
      self._reportBulkResult(oks, 'Shifted', tail);
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
      var foot = '<div class="efoot"><button type="button" class="btn quiet" data-edit-event="' + esc(ev.id) + '"><i class="fa-solid fa-pencil"></i> Edit</button><span class="sp"></span>' +
        (canDelete ? '<button type="button" class="btn danger" data-delete-event="' + esc(ev.id) + '"><i class="fa-solid fa-trash"></i> Delete</button>' : '') + '</div>';
      return html.replace('<!--EVP_FOOT-->', foot);
    };
  };

  CalendarEditor.prototype._bindEvpCapture = function () {
    var self = this, view = this.view;
    view.evpEl.addEventListener('click', function (e) {
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
    if (addBtn) addBtn.insertAdjacentElement('afterend', form);
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
  };

  CalendarEditor.prototype._refreshFlap = function () {
    this.view.flapEl.innerHTML = this._eraManagerHTML();
  };

  CalendarEditor.prototype._eraManagerHTML = function () {
    var view = this.view;
    var eras = (view.cal.eras || []).slice().sort(function (a, b) {
      return (a.sort_order || 0) - (b.sort_order || 0) || a.start_year - b.start_year;
    });
    var current = view.eraForDate(view.view.y, view.view.m, 1);
    var rows = eras.map(function (e) {
      var span = e.start_year + (e.end_year != null ? '–' + e.end_year : '–present');
      return '<div class="etl-seg' + (current && e.id === current.id ? ' cur' : '') + '">' +
        '<span>' + esc(e.name) + '<small>' + esc(span) + '</small></span>' +
        '<span class="acts">' +
          '<button type="button" class="qi" data-edit-era="' + e.id + '" aria-label="Edit ' + esc(e.name) + '"><i class="fa-solid fa-pencil"></i></button>' +
          '<button type="button" class="qi" data-del-era="' + e.id + '" aria-label="Delete ' + esc(e.name) + '"><i class="fa-solid fa-trash"></i></button>' +
        '</span></div>';
    }).join('') || '<p class="none">No eras yet.</p>';

    var formFor = this._eraFormFor;
    var body = formFor !== undefined
      ? this._eraFormHTML(formFor === 'new' ? null : eras.filter(function (e) { return String(e.id) === String(formFor); })[0])
      : '<button type="button" class="addev" data-add-era><i class="fa-solid fa-plus"></i>Add an era</button>';

    // Two leaves, as calendar_view.js's era card: the bar, then the list and
    // its form, which scroll.
    return '<div class="leaf lf1"><div class="grab" aria-hidden="true"></div>' +
        '<div class="crease"><span>Eras</span><button type="button" class="x" data-close aria-label="Close">✕</button></div></div>' +
      '<div class="leaf lf2"><div class="lscroll"><div class="fb fbr"><div class="etl-track">' + rows + '</div>' + body + '</div></div></div>';
  };

  CalendarEditor.prototype._eraFormHTML = function (era) {
    var isNew = !era;
    return '<form class="ebody" id="cal5-eraform" style="margin-top:10px">' +
      '<div class="fld"><input class="etitle" name="name" placeholder="Era name" required value="' + esc(era ? era.name : '') + '"/></div>' +
      '<div class="two">' +
        '<div class="fld">Starts<div class="erow"><span class="rl">Y</span><input type="number" name="start_year" required value="' + esc(era ? era.start_year : '') + '"/><input type="number" name="start_month" min="1" placeholder="M" value="' + esc(era ? era.start_month : 1) + '"/><input type="number" name="start_day" min="1" placeholder="D" value="' + esc(era ? era.start_day : 1) + '"/></div></div>' +
        '<div class="fld">Ends<label class="check"><input type="checkbox" name="ongoing"' + (!era || era.end_year == null ? ' checked' : '') + '/> Ongoing</label>' +
          '<div class="erow" data-end-fields' + (!era || era.end_year == null ? ' hidden' : '') + '><span class="rl">Y</span><input type="number" name="end_year" value="' + esc(era && era.end_year != null ? era.end_year : '') + '"/><input type="number" name="end_month" min="1" placeholder="M" value="' + esc(era && era.end_month != null ? era.end_month : '') + '"/><input type="number" name="end_day" min="1" placeholder="D" value="' + esc(era && era.end_day != null ? era.end_day : '') + '"/></div></div>' +
      '</div>' +
      '<div class="fld"><textarea name="description" rows="2" placeholder="Optional description">' + esc(era && era.description ? era.description : '') + '</textarea></div>' +
      '<div class="efoot"><button type="button" class="btn quiet" data-cancel-era>Cancel</button><span class="sp"></span>' +
        (isNew ? '' : '<button type="button" class="btn danger" data-del-era="' + era.id + '">Delete</button>') +
        '<button type="submit" class="btn primary">' + (isNew ? 'Create' : 'Save') + '</button></div>' +
      '</form>';
  };

  CalendarEditor.prototype._bindFlapCapture = function () {
    var self = this, view = this.view;
    if (!view.canAuthorDmOnly) return;
    view.flapEl.addEventListener('click', function (e) {
      var add = e.target.closest('[data-add-era]');
      var edit = e.target.closest('[data-edit-era]');
      var del = e.target.closest('[data-del-era]');
      var cancel = e.target.closest('[data-cancel-era]');
      var ongoing = e.target.closest('[name="ongoing"]');
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
          view.say('Era deleted.');
        });
      } else if (ongoing) {
        var fields = view.flapEl.querySelector('[data-end-fields]');
        if (fields) fields.hidden = ongoing.checked;
      }
    });
    view.flapEl.addEventListener('change', function (e) {
      if (e.target.name === 'ongoing') {
        var fields = view.flapEl.querySelector('[data-end-fields]');
        if (fields) fields.hidden = e.target.checked;
      }
    });
    view.flapEl.addEventListener('submit', function (e) {
      var form = e.target.closest('#cal5-eraform');
      if (!form) return;
      e.preventDefault();
      var f = form;
      var ongoing = f.ongoing.checked;
      var body = {
        name: f.name.value.trim(),
        start_year: +f.start_year.value,
        start_month: +f.start_month.value || 1,
        start_day: +f.start_day.value || 1,
        end_year: ongoing ? null : (f.end_year.value ? +f.end_year.value : null),
        end_month: ongoing ? null : (f.end_month.value ? +f.end_month.value : null),
        end_day: ongoing ? null : (f.end_day.value ? +f.end_day.value : null),
        description: f.description.value || null,
        color: '#8b5cf6',
        sort_order: (view.cal.eras || []).length
      };
      var isNew = self._eraFormFor === 'new';
      var req = isNew
        ? Chronicle.apiFetch(view.apiBase + '/eras', { method: 'POST', body: body })
        : Chronicle.apiFetch(view.apiBase + '/eras/' + self._eraFormFor, { method: 'PUT', body: body });
      req.then(function (resp) {
        if (!resp.ok) { view.say('Could not save the era.'); return null; }
        return isNew ? resp.json() : null;
      }).then(function (created) {
        if (isNew && created) {
          view.cal.eras = (view.cal.eras || []).concat([created]);
        } else if (!isNew) {
          var existing = (view.cal.eras || []).filter(function (er) { return String(er.id) === String(self._eraFormFor); })[0];
          if (existing) { for (var k in body) existing[k] = body[k]; }
        }
        self._eraFormFor = undefined;
        self._refreshFlap();
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
  var mount = document.querySelector('[data-widget="calendar_view"]');
  if (mount && mount.dataset.canEdit === 'true') {
    if (mount.calendarView) attach(mount.calendarView);
    else mount.addEventListener('calendarv5:ready', function (e) { attach(e.detail); });
  }
  // A calendar mounted after this script ran (a Calendars page preview
  // unfolding in place) announces itself the same way. The event doesn't
  // bubble, but a capturing listener on the document still sees it.
  document.addEventListener('calendarv5:ready', function (e) {
    var m = e.target;
    if (m && m.dataset && m.dataset.canEdit === 'true') attach(e.detail);
  }, true);
})();
