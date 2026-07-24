// event_grid.js — V2 calendar event grid widget. Wave 1 PR 4
// (C-CAL-V2-EVENT-CARD-COMPOSITE). Owns three drag patterns + the
// event detail drawer + the visibility editor chip-row builder.
//
// Inherits the data-drawer-* attribute contract from
// subresource_grid.js (PR #364) for drawer chrome. Adds calendar-
// grid-specific behaviors:
//
//   1. Click event card → drawer opens in edit mode
//   2. Click "+" affordance on empty cell → drawer opens in create mode
//   3. Click overflow "+N more" → expand cell (full reload to a
//      day-detail view; full popover deferred to PR 5 stretch)
//   4. Drag event card → reschedule via PUT on existing
//      /campaigns/:cid/calendars/:calId/events/:eid endpoint
//   5. Visibility editor: add/remove allow + deny rule chips inline;
//      effective-audience summary computed client-side
//
// Drag-to-create (cell click-and-drag) + drag-to-resize (ribbon edge)
// are wired but minimal in this PR; PR 5 can polish.

(function () {
    'use strict';

    // dayRange normalizes a drag from startDay→endDay into an ordered span:
    // a single cell (start === end) is a single-day add; otherwise a multi-day
    // range. Pure (exposed for tests). C-CAL-INTERACTIONS.
    function dayRange(startDay, endDay) {
        var lo = Math.min(startDay, endDay), hi = Math.max(startDay, endDay);
        return { startDay: lo, endDay: hi, multi: lo !== hi };
    }
    if (typeof window !== 'undefined') window.__calDayRange = dayRange;

    // _openDrawer is the LIVE drawer-opener, refreshed by each init() so the
    // document-level drag-create (wired once) always calls the current closure.
    var _openDrawer = null;
    var _dayDragWired = false;

    // wireDayDragCreateOnce installs the drag-to-create / multiselect on the
    // month grid ONCE (document-level + live DOM queries), so it survives
    // boosted-nav re-inits without leaking listeners (the QA2/E8 class). A
    // pointerdown on an empty Scribe cell starts a day-range selection; dragging
    // across cells highlights the span; release opens the editor pre-filled
    // (single cell → single day, multi → multi-day end date).
    function wireDayDragCreateOnce() {
        if (_dayDragWired || typeof document === 'undefined') return;
        _dayDragWired = true;
        var sel = null;
        function addableCell(target) {
            if (!target || !target.closest) return null;
            var cell = target.closest('.cell-drop-target');
            if (!cell || !cell.querySelector('[data-cell-add-event]')) return null; // empty Scribe cell
            if (target.closest('[data-event-card], a, button')) return null;        // not a card / + / overflow
            return cell;
        }
        function highlight(lo, hi) {
            document.querySelectorAll('.cell-drop-target').forEach(function (c) {
                var d = parseInt(c.dataset.cellDay, 10);
                var on = !isNaN(d) && d >= lo && d <= hi;
                c.classList.toggle('ring-2', on);
                c.classList.toggle('ring-accent', on);
            });
        }
        function clearHighlight() {
            document.querySelectorAll('.cell-drop-target.ring-accent').forEach(function (c) {
                c.classList.remove('ring-2', 'ring-accent');
            });
        }
        document.addEventListener('pointerdown', function (e) {
            if (e.button !== 0) return;
            var cell = addableCell(e.target);
            if (!cell) return;
            var day = parseInt(cell.dataset.cellDay, 10);
            if (isNaN(day)) return;
            // #439 polish: suppress the browser's text-selection while drag-
            // selecting a day range — otherwise the drag highlights the day-
            // number TEXT instead of cleanly ringing the cells. preventDefault
            // stops the selection starting; userSelect:none covers the drag
            // itself. Both are scoped to the active drag — pointerup restores
            // normal selection everywhere else, so text selection elsewhere is
            // untouched.
            e.preventDefault();
            document.body.style.userSelect = 'none';
            sel = {
                year: parseInt(cell.dataset.cellYear, 10),
                month: parseInt(cell.dataset.cellMonth, 10),
                startDay: day, curDay: day,
            };
            highlight(day, day);
        });
        document.addEventListener('pointermove', function (e) {
            if (!sel) return;
            var el = document.elementFromPoint(e.clientX, e.clientY);
            var cell = el && el.closest ? el.closest('.cell-drop-target') : null;
            if (!cell) return;
            var d = parseInt(cell.dataset.cellDay, 10);
            if (isNaN(d) || d === sel.curDay) return;
            sel.curDay = d;
            var r = dayRange(sel.startDay, d);
            highlight(r.startDay, r.endDay);
        });
        document.addEventListener('pointerup', function () {
            document.body.style.userSelect = '';   // restore normal text selection
            if (!sel) return;
            var s = sel; sel = null;
            clearHighlight();
            if (!_openDrawer) return;
            var r = dayRange(s.startDay, s.curDay);
            if (r.multi) {
                // A drag ACROSS cells is the multiselect-days create — it still
                // wins over a single click and opens the drawer directly.
                _openDrawer({
                    year: s.year, month: s.month, day: r.startDay,
                    end_year: s.year, end_month: s.month, end_day: r.endDay,
                });
            }
            // A SINGLE-cell press (no drag) no longer opens the drawer: the day
            // mini-view (calendar_v2_shell.js, all roles) is now the first tier
            // on a date click, and its "Add event" button is the create path.
            // cordinator#33 item 4.
        });
    }

    function init() {
        var root = document.querySelector('[data-cal-v2-root]');
        if (!root) return;

        var calendarID = root.dataset.calV2CalendarId;
        var campaignID = root.dataset.calV2CampaignId;
        var csrfToken = root.dataset.calV2CsrfToken;
        var isScribe = root.dataset.calV2IsScribe === 'true';
        var isOwner = root.dataset.calV2IsOwner === 'true';

        if (!calendarID) return; // no calendar to interact with, any role

        var events = [];
        try {
            events = JSON.parse(root.dataset.calV2Events || '[]');
        } catch (e) {
            console.error('event_grid: invalid events payload', e);
        }

        function eventByID(id) {
            for (var i = 0; i < events.length; i++) {
                if (events[i].id === id) return events[i];
            }
            return null;
        }

        // --- Quick-edit card (C-CAL-QUICKEDIT — the "peek" small editor) ---
        // Read-only interactions: wired for EVERY calendar member, not just
        // Scribes (C-CAL-UX-PAIR §Fix 1 — event_grid.js used to bail out for
        // players before reaching any of this, killing the tap-to-view card
        // the markup already server-gates to read-only for them). Click an
        // event chip → a compact pinned card beside it: title + description
        // editable in place (Scribes), one Save (PUT through the same events
        // endpoint, body = the stored event merged with the two edited fields
        // so nothing else is touched), "Full editor" hands off to the drawer.
        // Players get the same card read-only (server-gated markup —
        // calendar_v2_quickedit.templ renders no inputs/buttons for them, so
        // the `if (qeSave) …` / `if (qeExpand) …` wiring below simply finds
        // nothing to attach to).
        var qe = document.getElementById('cal-v2-event-quickedit');
        var qeID = null, qeDirty = false;
        function qeEl(sel) { return qe ? qe.querySelector(sel) : null; }
        function qeFmtTime(ev) {
            if (ev.all_day) return 'All day';
            if (ev.start_hour == null) return '';
            var p2 = function (n) { return n < 10 ? '0' + n : '' + n; };
            return p2(ev.start_hour) + ':' + p2(ev.start_minute == null ? 0 : ev.start_minute);
        }
        function openQuickEdit(card) {
            var id = card.dataset.eventId;
            var ev = id ? eventByID(id) : null;
            // No scaffold/data → fall back to the full drawer, but only for
            // Scribes: players have no drawer (server-gated absent from the
            // DOM — eventV2Drawer, calendar_v2.templ), and openDrawer isn't
            // defined for them below this function.
            if (!qe || !ev) { if (isScribe) openDrawer(id); return; }
            qeID = id; qeDirty = false;
            var nameI = qeEl('[data-qe-name]');
            if (nameI) nameI.value = ev.name || '';
            var nameRo = qeEl('[data-qe-name-ro]');
            if (nameRo) nameRo.textContent = ev.name || 'Event';
            var descI = qeEl('[data-qe-desc]');
            if (descI) descI.value = ev.description || '';
            var descRo = qeEl('[data-qe-desc-ro]');
            if (descRo) descRo.textContent = ev.description || '';
            var meta = qeEl('[data-qe-meta]');
            if (meta) {
                var bits = [];
                bits.push('Day ' + ev.day + (ev.end_day != null && (ev.end_day !== ev.day || ev.end_month !== ev.month) ? ' → ' + ev.end_day : ''));
                var t = qeFmtTime(ev); if (t) bits.push(t);
                if (ev.category) bits.push(ev.category);
                if (ev.tier) bits.push(ev.tier);
                meta.textContent = bits.join(' · ');
            }
            var vis = qeEl('[data-qe-vis]');
            if (vis) vis.textContent = (ev.visibility && ev.visibility !== 'everyone') ? '🔒 DM only' : '';
            // Position beside the chip: below by default, flipped above /
            // clamped when the viewport runs out.
            qe.classList.remove('hidden');
            var rect = card.getBoundingClientRect();
            var pr = qe.getBoundingClientRect();
            var left = Math.min(rect.left, window.innerWidth - pr.width - 8);
            var top = rect.bottom + 6;
            if (top + pr.height > window.innerHeight - 8) top = rect.top - pr.height - 6;
            qe.style.left = Math.max(8, left) + 'px';
            qe.style.top = Math.max(8, top) + 'px';
            if (nameI && typeof nameI.focus === 'function') nameI.focus();
        }
        function closeQuickEdit(force) {
            if (!qe || qe.classList.contains('hidden')) return;
            if (qeDirty && !force && !window.confirm('Discard unsaved changes?')) return;
            qe.classList.add('hidden');
            qeID = null; qeDirty = false;
        }
        function saveQuickEdit(btn) {
            var ev = qeID ? eventByID(qeID) : null;
            if (!ev) return;
            // Merge: the stored event + the two edited fields. Sending the full
            // stored shape keeps every untouched field (dates, category, tier,
            // visibility + rules) exactly as it was — a lossless quick-save.
            var body = {};
            for (var k in ev) {
                if (Object.prototype.hasOwnProperty.call(ev, k) && ev[k] !== null && k !== 'id') body[k] = ev[k];
            }
            var nameI = qeEl('[data-qe-name]');
            if (nameI) body.name = nameI.value.trim();
            var descI = qeEl('[data-qe-desc]');
            if (descI) body.description = descI.value;
            if (!body.name) { window.Chronicle.notify('Name is required', 'error'); return; }
            if (btn) btn.disabled = true;
            window.Chronicle.apiFetch('/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + qeID, {
                method: 'PUT', body: body, headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) {
                    return resp.json().catch(function () { return {}; }).then(function (b) {
                        throw new Error((b && b.message) || 'Save failed');
                    });
                }
                closeQuickEdit(true);
                window.location.reload();
            }).catch(function (e) {
                if (btn) btn.disabled = false;
                window.Chronicle.notify((e && e.message) || 'Save failed', 'error');
            });
        }
        if (qe && qe.dataset.qeWired !== '1') {
            qe.dataset.qeWired = '1'; // per-node guard (QA2 re-init class)
            var qeClose = qeEl('[data-qe-close]');
            if (qeClose) qeClose.addEventListener('click', function () { closeQuickEdit(false); });
            var qeSave = qeEl('[data-qe-save]');
            if (qeSave) qeSave.addEventListener('click', function () { saveQuickEdit(qeSave); });
            var qeExpand = qeEl('[data-qe-expand]');
            if (qeExpand) qeExpand.addEventListener('click', function () {
                var id = qeID;
                closeQuickEdit(true);
                openDrawer(id);
            });
            qe.addEventListener('input', function () { qeDirty = true; });
            qe.addEventListener('keydown', function (e) {
                if (e.key === 'Escape') { e.stopPropagation(); closeQuickEdit(false); }
            });
        }
        // Outside click closes (root-bound so the listener dies with the page
        // on a boosted-nav swap — no document-listener leak, the E4/E5 class).
        root.addEventListener('click', function (e) {
            if (!qe || qe.classList.contains('hidden')) return;
            if (qe.contains(e.target)) return;
            if (e.target.closest && e.target.closest('[data-event-card]')) return; // re-open on another chip
            closeQuickEdit(false);
        });

        // Card click → open the quick-edit card. Every member gets this (the
        // mobile agenda cards, #544, carry the same data-event-card hook and
        // ride the same wiring here for free).
        document.querySelectorAll('[data-event-card]').forEach(function (card) {
            card.addEventListener('click', function () {
                openQuickEdit(card);
            });
        });

        // --- Everything below is Scribe+ write tooling (drawer, drag-create,
        // drag-move, cell-add). The drawer element itself is server-gated to
        // Scribes (eventV2Drawer, calendar_v2.templ) — for every other role
        // document.getElementById finds nothing and this early-return IS the
        // gate (C-CAL-UX-PAIR §Fix 1).
        var drawer = document.getElementById('event-v2-drawer');
        if (!drawer) return;

        var editingID = null; // null = create mode; string = edit mode
        var dirty = false;
        var currentEvent = null; // the stored event being edited (for the actions)
        var _lastFocus = null;   // element focused before the drawer opened (a11y restore)

        // --- Drawer: open / close / populate -------------------

        function openDrawer(idOrPrefill) {
            var prefill = {};
            if (typeof idOrPrefill === 'string') {
                editingID = idOrPrefill;
                prefill = eventByID(idOrPrefill) || {};
            } else if (idOrPrefill && typeof idOrPrefill === 'object') {
                editingID = null;
                prefill = idOrPrefill;
            } else {
                editingID = null;
            }

            dirty = false;
            _lastFocus = (document.activeElement && typeof document.activeElement.focus === 'function')
                ? document.activeElement : null;
            populateDrawer(prefill);
            drawer.classList.remove('hidden');
            // Focus-trap the dialog (C-CAL-LARGE-EDITOR: role=dialog, aria-modal).
            drawer.addEventListener('keydown', onTrapKeydown);
            var first = drawer.querySelector('[data-field="name"]') || drawer.querySelector('[data-field]');
            if (first && typeof first.focus === 'function') first.focus();

            drawer.querySelectorAll('[data-field]').forEach(function (el) {
                el.addEventListener('input', markDirty);
                el.addEventListener('change', markDirty);
            });

            // Initialize the visibility editor state from the event.
            initVisibilityEditor(prefill);

            // Attach-entity picker (2b): persists real entity_event_links.
            initEntityTies();
            // Drawer actions (C-CAL-EDITOR-EXPANSION PR1): edit-mode only.
            currentEvent = editingID ? prefill : null;
            initDrawerActions(prefill);
            // RSVP panel (C-CAL-RSVP-P1): load the live per-event RSVP controls
            // into the drawer slot in EDIT mode. A new (unsaved) event has no id,
            // so the slot keeps its "save first" hint.
            loadDrawerRSVP();
        }

        // loadDrawerRSVP lazy-loads the RSVP panel fragment for the event being
        // edited into the drawer slot. Uses the same campaign/calendar ids the
        // shell root carries; htmx.ajax activates the fragment's hx-* buttons.
        function loadDrawerRSVP() {
            var slot = drawer.querySelector('[data-rsvp-drawer-slot]');
            if (!slot) return;
            if (!editingID) return; // create mode → keep the "save first" hint
            var root = document.querySelector('[data-cal-v2-root]');
            if (!root || !window.htmx) return;
            var cid = root.dataset.calV2CampaignId;
            var calId = root.dataset.calV2CalendarId;
            if (!cid || !calId) return;
            var url = '/campaigns/' + encodeURIComponent(cid) +
                '/calendars/' + encodeURIComponent(calId) +
                '/events/' + encodeURIComponent(editingID) + '/rsvp-panel';
            window.htmx.ajax('GET', url, { target: slot, swap: 'innerHTML' });
        }

        function markDirty() { dirty = true; }

        // Focus trap (C-CAL-LARGE-EDITOR): keep Tab / Shift+Tab inside the open
        // drawer, cycling first↔last of the currently-VISIBLE focusables (so the
        // hidden time inputs / collapsed panels don't create dead tab stops).
        function drawerFocusables() {
            var sel = 'a[href], button:not([disabled]), input:not([disabled]), ' +
                'select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
            return Array.prototype.slice.call(drawer.querySelectorAll(sel)).filter(function (el) {
                return el.offsetParent !== null; // visible (not display:none / hidden ancestor)
            });
        }
        function onTrapKeydown(e) {
            if (e.key !== 'Tab') return;
            var els = drawerFocusables();
            if (!els.length) return;
            var first = els[0], last = els[els.length - 1];
            if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
            else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
        }

        function closeDrawer(force) {
            if (dirty && !force) {
                if (!window.confirm('Discard unsaved changes?')) return;
            }
            drawer.classList.add('hidden');
            drawer.removeEventListener('keydown', onTrapKeydown);
            var confirmEl = drawer.querySelector('[data-drawer-confirm]');
            if (confirmEl) confirmEl.classList.add('hidden');
            clearDrawerError();
            editingID = null;
            dirty = false;
            // Restore focus to the control that opened the drawer (a11y).
            if (_lastFocus && typeof _lastFocus.focus === 'function') {
                try { _lastFocus.focus(); } catch (e) { /* element gone after reload */ }
            }
            _lastFocus = null;
        }

        function populateDrawer(item) {
            var title = drawer.querySelector('[data-drawer-title]');
            var deleteBtn = drawer.querySelector('[data-drawer-delete]');

            if (title) title.textContent = editingID ? 'Edit event' : 'Add event';
            if (deleteBtn) {
                if (editingID && isOwner) deleteBtn.classList.remove('hidden');
                else deleteBtn.classList.add('hidden');
            }

            drawer.querySelectorAll('[data-field]').forEach(function (el) {
                var field = el.dataset.field;
                var value = item[field];
                if (el.type === 'checkbox') el.checked = Boolean(value);
                else el.value = (value === undefined || value === null) ? '' : value;
            });
            // C-CAL-INTERACTIONS: reflect a multi-day prefill (a drag-create range
            // or an existing multi-day event) — show the end-date fields + check
            // the toggle when an end date is present and differs from the start.
            syncMultiday(hasEndDate(item));
            syncRecurrenceCustom();
            // C-CAL-LARGE-EDITOR: the redesigned drawer's own controls (chips,
            // segment, time inputs, all-day, live hints) are not [data-field]s —
            // populate them explicitly from the same item.
            clearDrawerError();
            syncTypeChips(item.category || '');
            syncTierSeg(item.tier || '');
            populateTimes(item);
            updateRecurrenceSummary();
            updateFantasyHint();
            updateRtTimeHint();
        }

        // --- C-CAL-LARGE-EDITOR redesigned controls -----------------------

        // The inline validation region (data-drawer-error) surfaces the events
        // endpoint's single {error,message} 422 next to the form, not as a
        // full-page state (dispatch §3). clearDrawerError resets it each open.
        function drawerError() { return drawer.querySelector('[data-drawer-error]'); }
        function clearDrawerError() {
            var e = drawerError();
            if (e) { e.textContent = ''; e.classList.add('hidden'); }
        }
        function showDrawerError(msg) {
            var e = drawerError();
            if (!e) { window.Chronicle.notify(msg || 'Save failed', 'error'); return; }
            e.textContent = msg || 'Save failed';
            e.classList.remove('hidden');
            var body = drawer.querySelector('.overflow-y-auto');
            if (body && typeof body.scrollTo === 'function') body.scrollTo({ top: 0, behavior: 'smooth' });
        }

        function fmt2(n) { return (n < 10 ? '0' : '') + n; }

        // Type chips ↔ hidden category field. Clicking a chip selects it (or, if
        // already active, clears back to "no category" — matching the former
        // select's "— none —"). Active styling is driven inline from the chip's
        // own category color so no compiled CSS class is needed.
        function typeValueEl() { return drawer.querySelector('[data-type-value]'); }
        function syncTypeChips(slug) {
            var hidden = typeValueEl();
            if (hidden) hidden.value = slug || '';
            drawer.querySelectorAll('[data-type-chip]').forEach(function (chip) {
                var on = chip.dataset.catSlug === slug && !!slug;
                chip.setAttribute('aria-pressed', on ? 'true' : 'false');
                var color = chip.dataset.catColor || '';
                if (on && color) {
                    chip.style.borderColor = color;
                    chip.style.backgroundColor = color + '22';
                    chip.style.color = 'var(--color-text-primary)';
                } else {
                    chip.style.borderColor = '';
                    chip.style.backgroundColor = '';
                    chip.style.color = '';
                }
            });
        }

        // Tier segment ↔ hidden tier field. One active at a time; clicking the
        // active chip clears to "" (platform default). Active styling toggles
        // known-good utility classes (no new compiled CSS).
        function tierValueEl() { return drawer.querySelector('[data-tier-value]'); }
        function syncTierSeg(slug) {
            var hidden = tierValueEl();
            if (hidden) hidden.value = slug || '';
            drawer.querySelectorAll('[data-tier-chip]').forEach(function (chip) {
                var on = chip.dataset.tierSlug === slug && !!slug;
                chip.setAttribute('aria-pressed', on ? 'true' : 'false');
                chip.classList.toggle('bg-surface-alt', on);
                chip.classList.toggle('text-fg', on);
                chip.classList.toggle('shadow-sm', on);
                chip.classList.toggle('text-fg-secondary', !on);
            });
        }

        // All-day toggle shows/hides the time inputs. An all-day event carries no
        // clock time (StartHour==nil) — readDrawer omits the times + sends
        // all_day:true so the service clears them (C-CAL-LARGE-EDITOR backend).
        function alldayBtn() { return drawer.querySelector('[data-allday-toggle]'); }
        function isAllday() {
            var b = alldayBtn();
            return !!(b && b.getAttribute('aria-checked') === 'true');
        }
        function syncAllday(on) {
            var b = alldayBtn();
            if (b) b.setAttribute('aria-checked', on ? 'true' : 'false');
            var fields = drawer.querySelector('[data-time-fields]');
            if (fields) fields.classList.toggle('hidden', !!on);
        }

        // populateTimes fills the two HH:MM inputs from the event's clock fields
        // and sets the all-day state. In EDIT mode a stored event with no start
        // hour IS all-day; a fresh CREATE defaults to timed (empty inputs).
        function populateTimes(item) {
            var startEl = drawer.querySelector('[data-time-start]');
            var endEl = drawer.querySelector('[data-time-end]');
            if (startEl) startEl.value = (item.start_hour == null) ? '' :
                fmt2(item.start_hour) + ':' + fmt2(item.start_minute == null ? 0 : item.start_minute);
            if (endEl) endEl.value = (item.end_hour == null) ? '' :
                fmt2(item.end_hour) + ':' + fmt2(item.end_minute == null ? 0 : item.end_minute);
            var allDay = item.all_day === true || (editingID && item.start_hour == null);
            syncAllday(!!allDay);
        }

        // parseTimeInput turns an <input type="time"> "HH:MM" into {h,m}, or null.
        function parseTimeInput(el) {
            if (!el || !el.value) return null;
            var m = /^(\d{1,2}):(\d{2})$/.exec(el.value.trim());
            if (!m) return null;
            var h = parseInt(m[1], 10), mi = parseInt(m[2], 10);
            if (isNaN(h) || isNaN(mi) || h < 0 || h > 23 || mi < 0 || mi > 59) return null;
            return { h: h, m: mi };
        }

        // Fantasy-equivalence hint: the entered date rendered in the active
        // calendar's own prose. Era name if the year falls in one, else the bare
        // year + epoch suffix — both carried on the drawer as data attributes.
        function drawerYearLabel(year) {
            if (isNaN(year)) return '';
            var eras = [];
            try { eras = JSON.parse(drawer.dataset.calEras || '[]'); } catch (e) { eras = []; }
            for (var i = 0; i < eras.length; i++) {
                var er = eras[i];
                if (year >= er.start && (er.end == null || year <= er.end)) return er.name;
            }
            var epoch = drawer.dataset.calEpoch || '';
            return year + (epoch ? ' ' + epoch : '');
        }
        function updateFantasyHint() {
            var hint = drawer.querySelector('[data-fantasy-hint]');
            if (!hint) return;
            var monthSel = drawer.querySelector('[data-field="month"]');
            var dayEl = drawer.querySelector('[data-field="day"]');
            var yearEl = drawer.querySelector('[data-field="year"]');
            var monthName = (monthSel && monthSel.selectedIndex >= 0 && monthSel.options[monthSel.selectedIndex]) ?
                monthSel.options[monthSel.selectedIndex].text : '';
            var day = dayEl ? parseInt(dayEl.value, 10) : NaN;
            var year = yearEl ? parseInt(yearEl.value, 10) : NaN;
            if (!monthName || isNaN(day)) { hint.textContent = ''; return; }
            var yl = drawerYearLabel(year);
            hint.textContent = '= ' + monthName + ' ' + day + (yl ? ' · ' + yl : '') + ' — players see both dates.';
        }

        // Viewer-zone time hint (C-CAL-UX-PAIR §Fix 2): on a real-time calendar,
        // the entered start time is in the anchor zone; this restates it in the
        // Scribe's OWN browser zone below the time inputs, read-only. Reuses the
        // pure eventHint math calendar_v2_shell.js exposes on window.__calRtHint
        // (the same P3 dual-line pattern as the header's live clock) — no server
        // change, no new endpoint. Hidden for all-day events, incomplete dates,
        // non-real-time calendars, or when the Scribe's zone matches the anchor.
        function updateRtTimeHint() {
            var hint = drawer.querySelector('[data-rt-time-hint]');
            if (!hint) return;
            if (!window.__calRtHint) { hint.classList.add('hidden'); return; }
            var zone = window.__calRtHint.anchorZone();
            var monthEl = drawer.querySelector('[data-field="month"]');
            var dayEl = drawer.querySelector('[data-field="day"]');
            var yearEl = drawer.querySelector('[data-field="year"]');
            var month = monthEl ? parseInt(monthEl.value, 10) : NaN;
            var day = dayEl ? parseInt(dayEl.value, 10) : NaN;
            var year = yearEl ? parseInt(yearEl.value, 10) : NaN;
            var t = parseTimeInput(drawer.querySelector('[data-time-start]'));
            if (!zone || isAllday() || !t || isNaN(month) || isNaN(day) || isNaN(year)) {
                hint.classList.add('hidden');
                return;
            }
            var val = window.__calRtHint.eventHint(
                { year: year, month: month, day: day, start_hour: t.h, start_minute: t.m, all_day: false }, zone);
            if (!val) { hint.classList.add('hidden'); return; }
            hint.textContent = 'Your time: ' + val;
            hint.classList.remove('hidden');
        }

        // Plain-language recurrence summary — restates the select choice.
        function updateRecurrenceSummary() {
            var sel = drawer.querySelector('[data-recurrence-type]');
            var out = drawer.querySelector('[data-recurrence-summary]');
            if (!sel || !out) return;
            var v = sel.value, txt = '';
            if (v === 'weekly') txt = 'Repeats weekly.';
            else if (v === 'biweekly') txt = 'Repeats every 2 weeks.';
            else if (v === 'monthly') txt = 'Repeats monthly, on the same day.';
            else if (v === 'custom') {
                var n = parseInt((drawer.querySelector('[data-field="recurrence_interval"]') || {}).value, 10);
                txt = 'Repeats every ' + (isNaN(n) ? 'N' : n) + ' week' + (n === 1 ? '' : 's') + '.';
            }
            out.textContent = txt;
            out.classList.toggle('hidden', !txt);
        }

        // --- Multi-day (end-date) toggle ---------------------------------
        function multidayToggle() { return drawer.querySelector('[data-multiday-toggle]'); }
        function syncMultiday(on) {
            var t = multidayToggle(), f = drawer.querySelector('[data-multiday-fields]');
            if (t) t.checked = !!on;
            if (f) f.classList.toggle('hidden', !on);
        }
        // syncRecurrenceCustom shows the "every N weeks" input only for the
        // custom recurrence type (C-CAL-EDITOR-EXPANSION PR2).
        function syncRecurrenceCustom() {
            var sel = drawer.querySelector('[data-recurrence-type]');
            var custom = drawer.querySelector('[data-recurrence-custom]');
            if (custom) custom.classList.toggle('hidden', !sel || sel.value !== 'custom');
        }
        function hasEndDate(item) {
            return !!(item && item.end_day != null &&
                (item.end_year !== item.year || item.end_month !== item.month || item.end_day !== item.day));
        }

        function readDrawer() {
            var body = {};
            drawer.querySelectorAll('[data-field]').forEach(function (el) {
                var field = el.dataset.field;
                if (el.type === 'checkbox') body[field] = el.checked;
                else if (el.type === 'number') {
                    var n = parseInt(el.value, 10);
                    if (!isNaN(n)) body[field] = n;
                } else {
                    var v = el.value.trim();
                    if (v !== '') body[field] = v;
                }
            });
            // C-CAL-INTERACTIONS: the end-date span only persists when the
            // multi-day toggle is on — otherwise drop any stale end_* so a
            // single-day event never carries a leftover span.
            var mt = multidayToggle();
            if (!mt || !mt.checked) {
                delete body.end_year;
                delete body.end_month;
                delete body.end_day;
            }
            // Pull the visibility editor state into the body.
            var vis = readVisibilityEditor();
            body.visibility = vis.mode === 'public' ? 'everyone' : 'dm_only';
            if (vis.rules.length > 0) {
                body.visibility_rules = JSON.stringify(vis.rules);
            }
            // Recurrence (C-CAL-EDITOR-EXPANSION PR2): is_recurring is derived
            // from the Repeats select; the interval only matters for "custom".
            body.is_recurring = !!body.recurrence_type;
            if (!body.is_recurring) {
                delete body.recurrence_interval;
            }
            // C-CAL-LARGE-EDITOR: all-day + times. All-day → no clock time; drop
            // any time fields and flag all_day so the service clears the stored
            // clock (the drawer's all-day == StartHour==nil model). Otherwise pull
            // HH:MM from the two time inputs into the clock fields.
            if (isAllday()) {
                body.all_day = true;
                delete body.start_hour; delete body.start_minute;
                delete body.end_hour; delete body.end_minute;
            } else {
                body.all_day = false;
                var st = parseTimeInput(drawer.querySelector('[data-time-start]'));
                var en = parseTimeInput(drawer.querySelector('[data-time-end]'));
                if (st) { body.start_hour = st.h; body.start_minute = st.m; }
                if (en) { body.end_hour = en.h; body.end_minute = en.m; }
            }
            // Coerce integer fields — month/end_month come from <select> (string
            // value), and JSON must send real ints for the Go int bindings.
            ['year', 'month', 'day', 'end_year', 'end_month', 'end_day',
                'start_hour', 'start_minute', 'end_hour', 'end_minute', 'recurrence_interval']
                .forEach(function (k) {
                    if (body[k] === undefined || body[k] === null || body[k] === '') return;
                    var n = parseInt(body[k], 10);
                    if (isNaN(n)) delete body[k]; else body[k] = n;
                });
            return body;
        }

        // --- Attach-entity picker (2b): real entity_event_links ---------
        // Persists ties through the calendar plugin's tie endpoints; SEARCHES
        // entities via the entities plugin's own /entities/search (cross-plugin
        // stays at the API boundary). Roles come from the Go enum via
        // data-ties-roles so the vocabulary never drifts from the backend.

        var tiesSection = drawer.querySelector('[data-event-ties-section]');
        var TIE_ROLES = ((tiesSection && tiesSection.getAttribute('data-ties-roles')) ||
            'involved,present,affected,mentioned').split(',');

        function tiesEsc(s) {
            return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
                return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
            });
        }
        function tiesBase() {
            return '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + editingID + '/entities';
        }

        function initEntityTies() {
            if (!tiesSection) return;
            var list = tiesSection.querySelector('[data-ties-list]');
            var picker = tiesSection.querySelector('[data-ties-picker]');
            var hint = tiesSection.querySelector('[data-ties-hint]');
            var search = tiesSection.querySelector('[data-ties-search]');
            var results = tiesSection.querySelector('[data-ties-results]');
            if (list) list.innerHTML = '';
            if (results) { results.innerHTML = ''; results.classList.add('hidden'); }
            if (search) search.value = '';
            // A new event has no id yet — ties can only attach after it's saved.
            if (!editingID) {
                if (picker) picker.classList.add('hidden');
                if (hint) hint.classList.remove('hidden');
                return;
            }
            if (picker) picker.classList.remove('hidden');
            if (hint) hint.classList.add('hidden');
            loadTies();
            wireTiesSearch(search, results);
        }

        function loadTies() {
            window.Chronicle.apiFetch(tiesBase(), { method: 'GET' })
                .then(function (r) { return r.ok ? r.json() : []; })
                .then(function (ties) { renderTies(ties || []); })
                .catch(function () {});
        }

        function renderTies(ties) {
            var list = tiesSection.querySelector('[data-ties-list]');
            if (!list) return;
            if (!ties.length) {
                list.innerHTML = '<span class="text-xs text-fg-secondary italic">No entities attached.</span>';
                return;
            }
            list.innerHTML = '';
            ties.forEach(function (t) {
                var role = t.participation_role || 'involved';
                var opts = TIE_ROLES.map(function (r) {
                    return '<option value="' + r + '"' + (r === role ? ' selected' : '') + '>' + r + '</option>';
                }).join('');
                var chip = document.createElement('span');
                chip.className = 'inline-flex items-center gap-1 px-2 py-0.5 rounded bg-surface-2 text-xs';
                chip.innerHTML = '<span>' + tiesEsc(t.entity_name) + '</span>' +
                    '<select class="bg-transparent text-xs" data-tie-role aria-label="Participation role">' + opts + '</select>' +
                    '<button type="button" class="text-fg-secondary hover:text-danger" data-tie-remove aria-label="Remove">&times;</button>';
                chip.querySelector('[data-tie-role]').addEventListener('change', function (e) {
                    linkTie(t.entity_id, e.target.value);
                });
                chip.querySelector('[data-tie-remove]').addEventListener('click', function () {
                    unlinkTie(t.entity_id);
                });
                list.appendChild(chip);
            });
        }

        function linkTie(entityID, role) {
            window.Chronicle.apiFetch(tiesBase() + '/' + entityID, {
                method: 'PUT', body: { role: role }, headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (r) {
                if (r.ok) loadTies(); else window.Chronicle.notify('Attach failed', 'error');
            }).catch(function () { window.Chronicle.notify('Attach failed', 'error'); });
        }

        function unlinkTie(entityID) {
            window.Chronicle.apiFetch(tiesBase() + '/' + entityID, {
                method: 'DELETE', headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (r) { if (r.ok) loadTies(); }).catch(function () {});
        }

        var tiesSearchTimer = null;
        function wireTiesSearch(search, results) {
            if (!search || search.__tiesWired) return;
            search.__tiesWired = true;
            search.addEventListener('input', function () {
                var q = search.value.trim();
                if (tiesSearchTimer) clearTimeout(tiesSearchTimer);
                if (q.length < 2) { results.classList.add('hidden'); results.innerHTML = ''; return; }
                tiesSearchTimer = setTimeout(function () {
                    window.Chronicle.apiFetch('/campaigns/' + campaignID + '/entities/search?q=' + encodeURIComponent(q),
                        { method: 'GET', headers: { 'Accept': 'application/json' } })
                        .then(function (r) { return r.ok ? r.json() : { results: [] }; })
                        .then(function (data) { renderTieResults(results, (data && data.results) || []); })
                        .catch(function () {});
                }, 200);
            });
            document.addEventListener('click', function (e) {
                if (results && !results.contains(e.target) && e.target !== search) results.classList.add('hidden');
            });
        }

        function renderTieResults(results, items) {
            if (!results) return;
            if (!items.length) {
                results.innerHTML = '<div class="px-2 py-1 text-xs text-fg-secondary">No matches</div>';
                results.classList.remove('hidden');
                return;
            }
            results.innerHTML = '';
            items.slice(0, 12).forEach(function (it) {
                var id = it.id || it.entity_id;
                var name = it.name || it.entity_name || id;
                var row = document.createElement('button');
                row.type = 'button';
                row.className = 'block w-full text-left px-2 py-1 text-xs hover:bg-surface-2';
                row.textContent = name;
                row.addEventListener('click', function () {
                    linkTie(id, 'involved');
                    results.classList.add('hidden');
                    var s = tiesSection.querySelector('[data-ties-search]');
                    if (s) s.value = '';
                });
                results.appendChild(row);
            });
            results.classList.remove('hidden');
        }

        function saveDrawer() {
            clearDrawerError();
            var body = readDrawer();
            if (!body.name) {
                // Inline (dispatch §3) — no toast-only failures for validation.
                showDrawerError('Name is required.');
                var nameEl = drawer.querySelector('[data-field="name"]');
                if (nameEl && typeof nameEl.focus === 'function') nameEl.focus();
                return;
            }
            var url, method;
            if (editingID) {
                url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + editingID;
                method = 'PUT';
            } else {
                url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events';
                method = 'POST';
            }
            window.Chronicle.apiFetch(url, {
                method: method,
                body: body,
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) {
                    // Surface the endpoint's {error,message} 422/400 inline next to
                    // the form (the shapes from apperror.NewValidation), not a
                    // full-page error state (C-CAL-LARGE-EDITOR §3).
                    return resp.json().catch(function () { return {}; }).then(function (b) {
                        throw new Error((b && b.message) || 'Save failed');
                    });
                }
                closeDrawer(true);
                window.location.reload();
            }).catch(function (e) {
                showDrawerError((e && e.message) || 'Save failed');
            });
        }

        function commitDelete() {
            if (!editingID) return;
            var url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + editingID;
            window.Chronicle.apiFetch(url, {
                method: 'DELETE',
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) {
                    throw new Error('Delete failed');
                }
                closeDrawer(true);
                window.location.reload();
            }).catch(function (e) {
                window.Chronicle.notify((e && e.message) || 'Delete failed', 'error');
            });
        }

        // --- Drawer actions (C-CAL-EDITOR-EXPANSION PR1) -----------------
        // The Actions section is edit-mode only (an existing event has an id to
        // act on); listeners bind ONCE to the drawer's action controls.
        var _actionsWired = false;
        function wireDrawerActionsOnce() {
            if (_actionsWired) return;
            _actionsWired = true;
            var ceBtn = drawer.querySelector('[data-action-create-entity]');
            var cePanel = drawer.querySelector('[data-create-entity-panel]');
            if (ceBtn && cePanel) ceBtn.addEventListener('click', function () { cePanel.classList.toggle('hidden'); });
            var ceGo = drawer.querySelector('[data-create-entity-go]');
            if (ceGo) ceGo.addEventListener('click', createEntityFromEvent);

            var dupBtn = drawer.querySelector('[data-action-duplicate]');
            var dupPanel = drawer.querySelector('[data-duplicate-panel]');
            if (dupBtn && dupPanel) dupBtn.addEventListener('click', function () { dupPanel.classList.toggle('hidden'); });
            var dupGo = drawer.querySelector('[data-duplicate-go]');
            if (dupGo) dupGo.addEventListener('click', duplicateToDate);

            var plBtn = drawer.querySelector('[data-action-permalink]');
            if (plBtn) plBtn.addEventListener('click', copyPermalink);

            var wxBtn = drawer.querySelector('[data-action-weather]');
            var wxPanel = drawer.querySelector('[data-weather-panel]');
            if (wxBtn && wxPanel) wxBtn.addEventListener('click', function () { wxPanel.classList.toggle('hidden'); });
            var wxGo = drawer.querySelector('[data-weather-go]');
            if (wxGo) wxGo.addEventListener('click', setWeatherForDay);
        }

        function initDrawerActions(item) {
            wireDrawerActionsOnce();
            var actions = drawer.querySelector('[data-drawer-actions]');
            if (actions) actions.classList.toggle('hidden', !editingID); // edit-mode only
            // Collapse the inline panels each open.
            var cePanel = drawer.querySelector('[data-create-entity-panel]');
            if (cePanel) cePanel.classList.add('hidden');
            var dupPanel = drawer.querySelector('[data-duplicate-panel]');
            if (dupPanel) dupPanel.classList.add('hidden');
            var wxPanel = drawer.querySelector('[data-weather-panel]');
            if (wxPanel) wxPanel.classList.add('hidden');
            // Prefill the duplicate date to the event's date + 1 day (the operator
            // can adjust; the server validates the date).
            if (editingID && item) {
                setDupField('[data-dup-year]', item.year);
                setDupField('[data-dup-month]', item.month);
                setDupField('[data-dup-day]', (item.day || 0) + 1);
            }
        }
        function setDupField(sel, val) {
            var el = drawer.querySelector(sel);
            if (el && val != null) el.value = val;
        }

        // Create entity from event → POSTs the create-entity endpoint, refreshes
        // the ties so the new entity shows in "Linked entities", and toasts a
        // link to the new entity's page.
        function createEntityFromEvent() {
            if (!editingID) return;
            var sel = drawer.querySelector('[data-create-entity-type]');
            var typeID = sel ? parseInt(sel.value, 10) : NaN;
            if (isNaN(typeID)) { window.Chronicle.notify('Pick an entity type', 'error'); return; }
            var url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + editingID + '/create-entity';
            window.Chronicle.apiFetch(url, {
                method: 'POST', body: { entity_type_id: typeID }, headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (r) {
                if (!r.ok) return r.json().catch(function () { return {}; }).then(function (b) {
                    throw new Error((b && b.message) || 'Create failed');
                });
                return r.json();
            }).then(function (res) {
                if (!res) return;
                var panel = drawer.querySelector('[data-create-entity-panel]');
                if (panel) panel.classList.add('hidden');
                loadTies(); // the new entity is linked — reflect it in the chips
                window.Chronicle.notify(
                    'Created “' + tiesEsc(res.name) + '” — <a class="underline" href="' + res.edit_url + '">open</a>',
                    'success', { html: true, duration: 8000 });
            }).catch(function (e) {
                window.Chronicle.notify((e && e.message) || 'Create failed', 'error');
            });
        }

        // Duplicate to date → POST the create endpoint with the stored event's
        // full body (lossless, like saveQuickEdit) and the chosen date. The id +
        // any multi-day span are dropped so the copy is a fresh single-day event.
        function duplicateToDate() {
            if (!currentEvent) return;
            var y = parseInt((drawer.querySelector('[data-dup-year]') || {}).value, 10);
            var mo = parseInt((drawer.querySelector('[data-dup-month]') || {}).value, 10);
            var d = parseInt((drawer.querySelector('[data-dup-day]') || {}).value, 10);
            if (isNaN(y) || isNaN(mo) || isNaN(d)) { window.Chronicle.notify('Pick a date', 'error'); return; }
            var body = {};
            for (var k in currentEvent) {
                if (Object.prototype.hasOwnProperty.call(currentEvent, k)) body[k] = currentEvent[k];
            }
            delete body.id;
            delete body.end_year; delete body.end_month; delete body.end_day;
            body.year = y; body.month = mo; body.day = d;
            var url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events';
            window.Chronicle.apiFetch(url, {
                method: 'POST', body: body, headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (r) {
                if (!r.ok) return r.json().catch(function () { return {}; }).then(function (b) {
                    throw new Error((b && b.message) || 'Duplicate failed');
                });
                closeDrawer(true);
                window.location.reload();
            }).catch(function (e) {
                window.Chronicle.notify((e && e.message) || 'Duplicate failed', 'error');
            });
        }

        // Permalink → copy a focus URL; loading it re-opens this event's drawer
        // (the focus handler below).
        function copyPermalink() {
            if (!editingID) return;
            var link = window.location.origin + '/campaigns/' + campaignID +
                '/calendar/v2/' + calendarID + '/month?focus=' + encodeURIComponent(editingID);
            var ok = function () { window.Chronicle.notify('Link copied', 'success'); };
            var fail = function () { window.Chronicle.notify('Copy this link: ' + link, 'info', { duration: 0 }); };
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(link).then(ok).catch(fail);
            } else {
                fail();
            }
        }

        // Set weather for this event's day (C-CAL-EDITOR-EXPANSION PR2). GM world
        // tool — the markup is server-gated to CanControlWorldState. PUTs the
        // additive weatherDate so the override lands on the event's day, not the
        // calendar's current date.
        function setWeatherForDay() {
            if (!currentEvent) return;
            var sel = drawer.querySelector('[data-weather-type]');
            var wt = sel ? sel.value : '';
            if (!wt) return;
            var url = '/campaigns/' + campaignID + '/calendar/world-state' +
                (calendarID ? '?calendarId=' + encodeURIComponent(calendarID) : '');
            window.Chronicle.apiFetch(url, {
                method: 'PUT',
                body: { weather: wt, weatherDate: { year: currentEvent.year, month: currentEvent.month, day: currentEvent.day } },
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (r) {
                if (!r.ok) return r.json().catch(function () { return {}; }).then(function (b) {
                    throw new Error((b && b.message) || 'Set weather failed');
                });
                var panel = drawer.querySelector('[data-weather-panel]');
                if (panel) panel.classList.add('hidden');
                window.Chronicle.notify('Weather set for this day', 'success');
            }).catch(function (e) {
                window.Chronicle.notify((e && e.message) || 'Set weather failed', 'error');
            });
        }

        // --- Visibility editor (chip-row builder per Q-V2-7) ---

        var visEditor = drawer.querySelector('[data-visibility-editor]');
        var visRules = [];

        function initVisibilityEditor(event) {
            visRules = [];
            if (event && event.visibility_rules) {
                try {
                    var parsed = JSON.parse(event.visibility_rules);
                    if (Array.isArray(parsed)) visRules = parsed;
                } catch (e) {
                    // Malformed rules ignored; widget renders empty chip row.
                }
            }
            if (!visEditor) return;
            // Set the radio state.
            var isPublic = !event || event.visibility === 'everyone';
            var radios = visEditor.querySelectorAll('input[type="radio"][data-visibility-mode]');
            radios.forEach(function (r) {
                r.checked = (r.dataset.visibilityMode === (isPublic ? 'public' : 'specific'));
            });
            updateSpecificPanel(isPublic);
            renderChipRow();
            updateSummary();
            wireVisibilityHandlers();
        }

        function updateSpecificPanel(isPublic) {
            var panel = visEditor && visEditor.querySelector('[data-visibility-specific-panel]');
            if (!panel) return;
            panel.style.display = isPublic ? 'none' : '';
        }

        function renderChipRow() {
            var row = visEditor && visEditor.querySelector('[data-visibility-chip-row]');
            if (!row) return;
            row.innerHTML = '';
            visRules.forEach(function (rule, i) {
                row.appendChild(buildChip(rule, i));
            });
            var hidden = visEditor.querySelector('[data-visibility-rules-json]');
            if (hidden) hidden.value = JSON.stringify(visRules);
        }

        function buildChip(rule, i) {
            var span = document.createElement('span');
            var color = rule.mode === 'allow' ? 'border-green-500/40 bg-green-500/10' : 'border-amber-500/40 bg-amber-500/10';
            var iconColor = rule.mode === 'allow' ? 'text-green-500' : 'text-amber-500';
            span.className = 'chip-add inline-flex items-center gap-1 text-xs rounded px-2 py-1 border ' + color;
            span.dataset.visibilityChipIndex = String(i);

            var icon = document.createElement('span');
            icon.className = iconColor;
            icon.setAttribute('aria-hidden', 'true');
            icon.innerHTML = rule.mode === 'allow' ? '<i class="fa-solid fa-check"></i>' : '<i class="fa-solid fa-ban"></i>';
            span.appendChild(icon);

            var label = document.createElement('span');
            label.className = 'text-fg';
            label.textContent = rule.label || (rule.kind === 'user' ? '@' + rule.target : rule.target);
            span.appendChild(label);

            var remove = document.createElement('button');
            remove.type = 'button';
            remove.className = 'text-fg-secondary hover:text-fg ml-1 transition-colors duration-micro';
            remove.setAttribute('aria-label', 'Remove rule');
            remove.innerHTML = '<i class="fa-solid fa-xmark text-[10px]" aria-hidden="true"></i>';
            remove.addEventListener('click', function () {
                visRules.splice(i, 1);
                renderChipRow();
                updateSummary();
                markDirty();
            });
            span.appendChild(remove);
            return span;
        }

        function updateSummary() {
            var summaryEl = visEditor && visEditor.querySelector('[data-visibility-summary]');
            if (!summaryEl) return;
            var checked = visEditor.querySelector('input[type="radio"][data-visibility-mode]:checked');
            var isPublic = checked && checked.dataset.visibilityMode === 'public';
            if (isPublic) {
                summaryEl.textContent = 'Everyone with campaign access can see this.';
                return;
            }
            if (visRules.length === 0) {
                summaryEl.textContent = 'Nobody yet — add an allow rule to grant access.';
                return;
            }
            var allows = visRules.filter(function (r) { return r.mode === 'allow'; });
            var denies = visRules.filter(function (r) { return r.mode === 'deny'; });
            if (denies.length === 0) {
                var labels = allows.slice(0, 3).map(chipLabel);
                var extra = allows.length > 3 ? ' and ' + (allows.length - 3) + ' more' : '';
                summaryEl.textContent = labels.join(', ') + extra + ' can see this.';
            } else if (allows.length === 0) {
                summaryEl.textContent = 'Everyone except: ' + denies.map(chipLabel).join(', ');
            } else {
                summaryEl.textContent = allows.length + ' allow rule(s), ' + denies.length + ' deny rule(s). Server resolves precedence.';
            }
        }

        function chipLabel(rule) {
            return rule.label || (rule.kind === 'user' ? '@' + rule.target : rule.target);
        }

        function readVisibilityEditor() {
            if (!visEditor) return { mode: 'public', rules: [] };
            var checked = visEditor.querySelector('input[type="radio"][data-visibility-mode]:checked');
            var mode = checked ? checked.dataset.visibilityMode : 'public';
            return { mode: mode, rules: visRules };
        }

        function wireVisibilityHandlers() {
            if (!visEditor || visEditor.dataset.visibilityWired === '1') {
                // Re-init only updates state; handlers stay bound from first open.
                return;
            }
            visEditor.dataset.visibilityWired = '1';

            visEditor.querySelectorAll('input[type="radio"][data-visibility-mode]').forEach(function (r) {
                r.addEventListener('change', function () {
                    updateSpecificPanel(r.dataset.visibilityMode === 'public');
                    updateSummary();
                    markDirty();
                });
            });

            var addButtons = visEditor.querySelectorAll('[data-visibility-add]');
            addButtons.forEach(function (b) {
                b.addEventListener('click', function () {
                    openPicker(b.dataset.visibilityAdd);
                });
            });

            var picker = visEditor.querySelector('[data-visibility-picker]');
            if (picker) {
                var confirm = picker.querySelector('[data-visibility-picker-confirm]');
                var cancel = picker.querySelector('[data-visibility-picker-cancel]');
                var input = picker.querySelector('[data-visibility-picker-input]');
                if (cancel) cancel.addEventListener('click', function () { picker.classList.add('hidden'); });
                if (confirm) {
                    confirm.addEventListener('click', function () {
                        var kindEl = picker.querySelector('[data-visibility-picker-kind]');
                        var kind = kindEl ? kindEl.value : 'user';
                        var target = input ? input.value.trim() : '';
                        if (!target) return;
                        visRules.push({
                            mode: picker.dataset.pickerMode || 'allow',
                            kind: kind,
                            target: target,
                            label: kind === 'user' ? '@' + target : target,
                        });
                        if (input) input.value = '';
                        picker.classList.add('hidden');
                        renderChipRow();
                        updateSummary();
                        markDirty();
                    });
                }
            }
        }

        function openPicker(mode) {
            var picker = visEditor && visEditor.querySelector('[data-visibility-picker]');
            if (!picker) return;
            picker.classList.remove('hidden');
            picker.dataset.pickerMode = mode;
            var input = picker.querySelector('[data-visibility-picker-input]');
            if (input) input.focus();
        }

        // --- Drawer controls ---------------------

        drawer.querySelectorAll('[data-drawer-close]').forEach(function (el) {
            el.addEventListener('click', function () { closeDrawer(false); });
        });
        var backdrop = drawer.querySelector('[data-drawer-backdrop]');
        if (backdrop) backdrop.addEventListener('click', function () { closeDrawer(false); });

        var saveBtn = drawer.querySelector('[data-drawer-save]');
        if (saveBtn) saveBtn.addEventListener('click', saveDrawer);

        var delBtn = drawer.querySelector('[data-drawer-delete]');
        var confirmEl = drawer.querySelector('[data-drawer-confirm]');
        if (delBtn && confirmEl) {
            delBtn.addEventListener('click', function () {
                confirmEl.classList.remove('hidden');
                var check = confirmEl.querySelector('[data-drawer-confirm-check]');
                var doBtn = confirmEl.querySelector('[data-drawer-confirm-do]');
                if (check) check.checked = false;
                if (doBtn) doBtn.disabled = true;
            });
            var check = confirmEl.querySelector('[data-drawer-confirm-check]');
            var doBtn = confirmEl.querySelector('[data-drawer-confirm-do]');
            if (check && doBtn) check.addEventListener('change', function () { doBtn.disabled = !check.checked; });
            if (doBtn) doBtn.addEventListener('click', commitDelete);
            var cancel = confirmEl.querySelector('[data-drawer-confirm-cancel]');
            if (cancel) cancel.addEventListener('click', function () { confirmEl.classList.add('hidden'); });
        }

        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape' && !drawer.classList.contains('hidden')) closeDrawer(false);
        });

        // --- Cell + → create (Scribe+; card click → quick-edit is wired for
        // every role near the top of init(), above the drawer gate) ---

        document.querySelectorAll('[data-cell-add-event]').forEach(function (btn) {
            btn.addEventListener('click', function (e) {
                e.stopPropagation();
                var cell = btn.closest('.cell-drop-target');
                if (!cell) { openDrawer({}); return; }
                openDrawer({
                    year: parseInt(cell.dataset.cellYear, 10),
                    month: parseInt(cell.dataset.cellMonth, 10),
                    day: parseInt(cell.dataset.cellDay, 10),
                });
            });
        });

        // Add-enabled (empty Scribe) cells get the pointer cursor. The
        // drag-create selector (wired ONCE below) handles BOTH a single click
        // (single-day add — C-CAL-V2-MONTH-GRID-ALIGN-FIX #3) and a drag across
        // cells (a multi-day range = the operator's "multiselect days").
        document.querySelectorAll('.cell-drop-target').forEach(function (cell) {
            if (cell.querySelector('[data-cell-add-event]')) cell.classList.add('cursor-pointer');
        });
        // Multi-day end-date fields show/hide with their toggle.
        var mtoggle = drawer && drawer.querySelector('[data-multiday-toggle]');
        if (mtoggle) mtoggle.addEventListener('change', function () { syncMultiday(mtoggle.checked); });
        var rtypeSel = drawer && drawer.querySelector('[data-recurrence-type]');
        if (rtypeSel) rtypeSel.addEventListener('change', function () {
            syncRecurrenceCustom();
            updateRecurrenceSummary();
        });
        var rIntervalEl = drawer && drawer.querySelector('[data-field="recurrence_interval"]');
        if (rIntervalEl) rIntervalEl.addEventListener('input', updateRecurrenceSummary);

        // C-CAL-LARGE-EDITOR redesigned controls (wired ONCE — the scaffold is
        // static; these persist across opens without leaking listeners).
        // Type chips: select / toggle-off; mark dirty; sync the hidden category.
        drawer.querySelectorAll('[data-type-chip]').forEach(function (chip) {
            chip.addEventListener('click', function () {
                var cur = (typeValueEl() && typeValueEl().value) || '';
                var slug = chip.dataset.catSlug || '';
                syncTypeChips(cur === slug ? '' : slug);
                markDirty();
            });
        });
        // Tier segment: exclusive select; clicking the active chip clears to default.
        drawer.querySelectorAll('[data-tier-chip]').forEach(function (chip) {
            chip.addEventListener('click', function () {
                var cur = (tierValueEl() && tierValueEl().value) || '';
                var slug = chip.dataset.tierSlug || '';
                syncTierSeg(cur === slug ? '' : slug);
                markDirty();
            });
        });
        // All-day toggle: flip state, show/hide the time inputs, mark dirty.
        var alldayEl = drawer.querySelector('[data-allday-toggle]');
        if (alldayEl) alldayEl.addEventListener('click', function () {
            syncAllday(!isAllday());
            markDirty();
            updateRtTimeHint();
        });
        // Live fantasy-equivalence + viewer-zone time hints on any date change.
        ['[data-field="month"]', '[data-field="day"]', '[data-field="year"]'].forEach(function (s) {
            var el = drawer.querySelector(s);
            if (el) {
                el.addEventListener('input', updateFantasyHint);
                el.addEventListener('change', updateFantasyHint);
                el.addEventListener('input', updateRtTimeHint);
                el.addEventListener('change', updateRtTimeHint);
            }
        });
        // Start-time input: the RT hint tracks the entered clock time too (not
        // a [data-field], so it isn't covered by the generic dirty-tracking
        // loop above — wired here specifically for the hint).
        var timeStartEl = drawer.querySelector('[data-time-start]');
        if (timeStartEl) {
            timeStartEl.addEventListener('input', updateRtTimeHint);
            timeStartEl.addEventListener('change', updateRtTimeHint);
        }

        // Expose the live openDrawer to the document-level drag-create (wired
        // once so it never leaks listeners across boosted-nav re-inits).
        _openDrawer = openDrawer;
        wireDayDragCreateOnce();

        // The shell's "+N more" popover falls back to this when no chip is in
        // the DOM for the event — keep it pointing at the FULL drawer.
        window.calV2OpenDrawerByID = openDrawer;
        // The day mini-view's "Add event" button (Scribe+) opens the create
        // drawer prefilled with the clicked day (cordinator#33 item 4). openDrawer
        // treats an object arg as a create-mode prefill. Defined only for Scribes
        // (init returns early otherwise), matching the button's server-side gate.
        window.calV2OpenCreateDrawer = function (prefill) { openDrawer(prefill || {}); };

        // Permalink focus-open (C-CAL-EDITOR-EXPANSION PR1): if the URL carries
        // ?focus=<eventId> and that event is in the page's (visible) set, open its
        // drawer on load. Unknown/invisible id = silent no-op (eventByID is the
        // dm_only/visibility-filtered set the server rendered).
        try {
            var focusID = new URLSearchParams(window.location.search).get('focus');
            if (focusID && eventByID(focusID)) openDrawer(focusID);
        } catch (e) { /* URLSearchParams unsupported — skip */ }

        // --- Drag-to-reschedule via existing PUT endpoint --

        var dragSrc = null;
        document.querySelectorAll('[data-event-card]').forEach(function (card) {
            card.addEventListener('dragstart', function (e) {
                dragSrc = card.dataset.eventId;
                e.dataTransfer.effectAllowed = 'move';
                e.dataTransfer.setData('text/plain', dragSrc || '');
                card.classList.add('drag-ghost');
            });
            card.addEventListener('dragend', function () {
                card.classList.remove('drag-ghost');
                dragSrc = null;
            });
        });
        document.querySelectorAll('.cell-drop-target').forEach(function (cell) {
            cell.addEventListener('dragover', function (e) {
                e.preventDefault();
                e.dataTransfer.dropEffect = 'move';
                cell.classList.add('ring-2', 'ring-accent');
            });
            cell.addEventListener('dragleave', function () {
                cell.classList.remove('ring-2', 'ring-accent');
            });
            cell.addEventListener('drop', function (e) {
                e.preventDefault();
                cell.classList.remove('ring-2', 'ring-accent');
                if (!dragSrc) return;
                var event = eventByID(dragSrc);
                if (!event) return;
                var newYear = parseInt(cell.dataset.cellYear, 10);
                var newMonth = parseInt(cell.dataset.cellMonth, 10);
                var newDay = parseInt(cell.dataset.cellDay, 10);
                if (event.year === newYear && event.month === newMonth && event.day === newDay) return;

                var body = Object.assign({}, event, {
                    year: newYear,
                    month: newMonth,
                    day: newDay,
                });
                var url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + dragSrc;
                window.Chronicle.apiFetch(url, {
                    method: 'PUT',
                    body: body,
                    headers: { 'X-CSRF-Token': csrfToken },
                }).then(function (resp) {
                    if (!resp.ok) throw new Error('Move failed');
                    cell.classList.add('drop-receive-pulse');
                    setTimeout(function () { window.location.reload(); }, 250);
                }).catch(function (e) {
                    window.Chronicle.notify((e && e.message) || 'Move failed', 'error');
                });
            });
        });
    }

    // --- Drag-to-resize on Month-view multi-day ribbons (Wave 1.7A §B) ---
    //
    // Pointer-capture pattern: pointerdown on the right-edge handle
    // captures the pointer to the handle element so pointermove/up
    // events route to it even when the cursor strays off the handle.
    // stopPropagation() prevents the ribbon body's dragstart
    // (drag-to-reschedule) from firing. snap-to-day-boundary keeps
    // the resize semantically discrete (no sub-day resize on Month
    // view per dispatch §A.2).
    //
    // Browser support: setPointerCapture is universal in Chromium +
    // modern Firefox + Safari 13+. Older Safari falls back to
    // document-level pointermove tracking (graceful degradation; the
    // affordance still works but cursor-off-handle may not track as
    // smoothly). Documented as known limitation per stop-and-flag.
    function initRibbonResize() {
        var root = document.querySelector('[data-cal-v2-root]');
        if (!root) return;
        var calendarID = root.dataset.calV2CalendarId;
        var campaignID = root.dataset.calV2CampaignId;
        var csrfToken = root.dataset.calV2CsrfToken;
        var isScribe = root.dataset.calV2IsScribe === 'true';
        // Write affordance — Scribe+ only (C-CAL-UX-PAIR gate-split audit: this
        // was wired unconditionally pre-existing, reachable by players even
        // though the ribbon's [data-ribbon-resize] handle itself is now
        // server-gated too — see monthWeekRows, calendar_v2.templ).
        if (!isScribe || !calendarID) return;

        var events = [];
        try {
            events = JSON.parse(root.dataset.calV2Events || '[]');
        } catch (e) {
            console.error('event_grid resize: invalid events payload', e);
        }

        // Cache event-by-id lookup; the parent init() block defines
        // an equivalent closure but we redefine locally to keep the
        // resize handler self-contained.
        function eventByID(id) {
            for (var i = 0; i < events.length; i++) {
                if (events[i].id === id) return events[i];
            }
            return null;
        }

        // Active drag state — single resize at a time.
        var resizeState = null;

        function onPointerDown(e) {
            var handle = e.target.closest('[data-ribbon-resize="right"]');
            if (!handle) return;
            var ribbon = handle.closest('[data-event-id]');
            if (!ribbon) return;
            var eventID = ribbon.dataset.eventId;
            var ev = eventByID(eventID);
            if (!ev) return;
            // Suppress the ribbon body's drag-to-reschedule
            // (it shares the same DOM but different gesture target).
            e.stopPropagation();
            e.preventDefault();
            try {
                handle.setPointerCapture(e.pointerId);
            } catch (err) {
                // setPointerCapture unsupported (older Safari);
                // fall through to document-level tracking via the
                // pointermove handler registered globally below.
            }
            // Compute starting state.
            var ribbonRect = ribbon.getBoundingClientRect();
            var grid = ribbon.parentElement; // the .grid container
            var gridRect = grid.getBoundingClientRect();
            var cols = computeGridColumnCount(grid);
            var cellWidth = gridRect.width / cols;
            // Read existing grid-column to find the current start col + span.
            var styleParse = parseGridColumn(ribbon.style.gridColumn);
            resizeState = {
                handle: handle,
                ribbon: ribbon,
                eventID: eventID,
                event: ev,
                gridRect: gridRect,
                cellWidth: cellWidth,
                cols: cols,
                startCol: styleParse.start,
                originalSpan: styleParse.span,
                originalEndCol: styleParse.start + styleParse.span - 1,
                originalRight: ribbonRect.right,
                pointerID: e.pointerId,
                committed: false,
            };
            ribbon.classList.add('is-resizing');
        }

        function onPointerMove(e) {
            if (!resizeState) return;
            if (e.pointerId !== resizeState.pointerID) return;
            // Snap to nearest day boundary. cursorX within grid →
            // target end-column (1-indexed inclusive).
            var relX = e.clientX - resizeState.gridRect.left;
            var targetCol = Math.round(relX / resizeState.cellWidth);
            if (targetCol < resizeState.startCol) {
                targetCol = resizeState.startCol;
            }
            if (targetCol > resizeState.cols) {
                targetCol = resizeState.cols; // clamp to grid edge
            }
            var newSpan = targetCol - resizeState.startCol + 1;
            resizeState.ribbon.style.gridColumn = resizeState.startCol + ' / span ' + newSpan;
            resizeState.currentEndCol = targetCol;
        }

        function onPointerUp(e) {
            if (!resizeState) return;
            if (e.pointerId !== resizeState.pointerID) return;
            var state = resizeState;
            resizeState = null;
            state.ribbon.classList.remove('is-resizing');
            try {
                state.handle.releasePointerCapture(e.pointerId);
            } catch (err) { /* ignore */ }

            var newEndCol = state.currentEndCol || state.originalEndCol;
            if (newEndCol === state.originalEndCol) {
                // No change; nothing to commit.
                return;
            }
            // Convert end-col delta to end-day delta. The ribbon's
            // grid-column maps day-of-week-row → column; we don't
            // have access to the start day directly here, but the
            // event.end_day field gives us the existing value to
            // add the col delta to.
            var dayDelta = newEndCol - state.originalEndCol;
            var newEndDay = (state.event.end_day || state.event.day) + dayDelta;
            // Single-day conversion confirm: if resizing collapses
            // to start day, prompt before committing.
            if (newSpanCollapsesToSingleDay(state)) {
                if (!window.confirm('Convert to single-day event?')) {
                    // Revert visual.
                    state.ribbon.style.gridColumn = state.startCol + ' / span ' + state.originalSpan;
                    return;
                }
            }
            commitResize(state, newEndDay);
        }

        function newSpanCollapsesToSingleDay(state) {
            // If the new end-col equals start-col AND the original
            // event was multi-day, this is a collapse.
            return state.currentEndCol === state.startCol &&
                state.originalSpan > 1;
        }

        function commitResize(state, newEndDay) {
            var body = Object.assign({}, state.event, {
                end_year: state.event.end_year || state.event.year,
                end_month: state.event.end_month || state.event.month,
                end_day: newEndDay,
            });
            var url = '/campaigns/' + campaignID + '/calendars/' + calendarID + '/events/' + state.eventID;
            window.Chronicle.apiFetch(url, {
                method: 'PUT',
                body: body,
                headers: { 'X-CSRF-Token': csrfToken },
            }).then(function (resp) {
                if (!resp.ok) throw new Error('Resize failed');
                setTimeout(function () { window.location.reload(); }, 200);
            }).catch(function (err) {
                // Revert ribbon to original span on failure.
                state.ribbon.style.gridColumn = state.startCol + ' / span ' + state.originalSpan;
                window.Chronicle.notify((err && err.message) || 'Resize failed', 'error');
            });
        }

        // Helpers ---------------------------------------------------

        // computeGridColumnCount inspects the CSS Grid template-
        // columns to derive the column count. Falls back to 7
        // (Gregorian default) when parsing fails.
        function computeGridColumnCount(grid) {
            var style = window.getComputedStyle(grid);
            var template = style.gridTemplateColumns || '';
            // template-columns resolves to a space-separated list of
            // tracks; counting them gives the column count.
            var tracks = template.trim().split(/\s+/).filter(Boolean);
            if (tracks.length > 0) return tracks.length;
            return 7;
        }

        // parseGridColumn parses an inline "grid-column: N / span M"
        // style value into {start, span}. Returns {start: 1, span: 1}
        // when parsing fails.
        function parseGridColumn(value) {
            if (!value) return { start: 1, span: 1 };
            var m = value.match(/^(\d+)\s*\/\s*span\s*(\d+)$/i);
            if (m) return { start: parseInt(m[1], 10), span: parseInt(m[2], 10) };
            return { start: 1, span: 1 };
        }

        // Register listeners at the document level so pointer events
        // route correctly even when the cursor strays off the handle
        // (graceful fallback when setPointerCapture is unavailable).
        document.addEventListener('pointerdown', onPointerDown);
        document.addEventListener('pointermove', onPointerMove);
        document.addEventListener('pointerup', onPointerUp);
        document.addEventListener('pointercancel', onPointerUp);
    }

    // boot() binds the grid handlers once PER ROOT NODE. The V2 shell arrives
    // via hx-boost navigation (and re-renders on view/date nav), but under
    // htmx.config.allowScriptTags=false this <script> never re-runs — so we
    // bind on htmx:afterSettle/htmx:load too, and the per-root guard keeps a
    // freshly-swapped grid from double-binding (C-CAL-V2-MONTH-GRID-ALIGN-FIX
    // #3 — the day-add / card-edit handlers weren't binding after boosted nav).
    function boot() {
        var root = document.querySelector('[data-cal-v2-root]');
        if (!root || root.__eventGridInited) return;
        root.__eventGridInited = true;
        init();
        initRibbonResize();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', boot);
    } else {
        boot();
    }
    try {
        document.addEventListener('htmx:afterSettle', boot);
        document.addEventListener('htmx:load', boot);
    } catch (e) {}
})();
